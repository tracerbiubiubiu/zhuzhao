// Package admincli 管理面 CLI 公共逻辑（break-glass 应急通道）。
//
// 当前仅 superadmin 密码重置一件：复用与管理端 ResetPassword 完全相同的语义
// （bcrypt hash + must_change_password=true + 全端会话吊销），差别只在授权方式
// ——不走 HTTP/会话，凭「容器执行权」授权：能 docker exec 进 app 容器者本已
// 具备宿主级权限，本包不新增任何提权面（对比 HTTP break-glass 端点会在认证
// 系统上开新攻击面，故不采用）。输出/交互（确认、录密）留在 cmd/server 包装层。
package admincli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao-utils/crypto"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// 目标校验错误（CLI 面向操作者的中文提示，非 API 契约，不进 errcode 码表）。
var (
	ErrTargetNotFound      = errors.New("目标用户不存在（或已软删）")
	ErrTargetNotSuperadmin = errors.New("目标用户不持启用的 superadmin 角色——本工具仅限 superadmin 应急重置，普通用户请走管理端 POST /api/v1/users/password/reset")
)

// checkTarget 目标校验纯函数（拆出供单测钉死判定矩阵）。
func checkTarget(exists, isSuperadmin bool) error {
	if !exists {
		return ErrTargetNotFound
	}
	if !isSuperadmin {
		return ErrTargetNotSuperadmin
	}
	return nil
}

// ResetOptions 重置入参。Password 明文由调用方完成输入与确认（本包不回显、
// 不落日志）；Note 为操作注记（谁在什么场景执行），落 audit_logs.request_body。
type ResetOptions struct {
	Username  string
	Password  string
	Note      string
	Pool      *pgxpool.Pool
	RDB       *goredis.Client
	AccessTTL time.Duration // disabled 标记 TTL，与 HTTP 侧 ResetPassword 语义一致
}

// ResetResult 重置结果（各项独立上报，失败项不阻断整体返回——操作者按
// runbook 对提示逐项善后）。
type ResetResult struct {
	UserID          int64
	Username        string
	Reenabled       bool // 原处于禁用态，本次已连带启用
	SessionsRevoked bool // Redis 会话吊销是否成功（false=存量 token 可能残留）
	AuditWritten    bool // audit_logs 是否落行（false=须人工台账补记）
	ForcedChange    bool // 恒 true：must_change_password=true，下次登录强制改密
}

// ResetSuperadminPassword break-glass 重置：目标校验 → 重置密码 → 启用（若被禁）
// → 会话吊销 → 审计留痕。执行顺序对齐 HTTP 侧「先提交 DB、后吊销会话」协议
// （D2-05）；吊销失败不回滚 DB，带 reconcile 标记日志供对账。
func ResetSuperadminPassword(ctx context.Context, o ResetOptions) (*ResetResult, error) {
	if o.Username == "" {
		return nil, errors.New("username 为空")
	}
	if o.Password == "" {
		return nil, errors.New("password 为空")
	}
	if o.Pool == nil || o.RDB == nil {
		return nil, errors.New("DB/Redis 连接未初始化")
	}

	// ① 目标校验：存在 ∧ 持启用中的 superadmin 角色（B1-1 语义与
	// CountActiveSuperadminUsersTx 对齐：角色启用 ∧ 未软删才算数）
	var (
		id       int64
		username string
		status   int
		isSuper  bool
	)
	err := o.Pool.QueryRow(ctx, `
		SELECT u.id, u.username, u.status,
		       EXISTS(
		         SELECT 1 FROM user_roles ur
		         JOIN roles r ON r.id = ur.role_id
		         WHERE ur.user_id = u.id AND r.code = 'superadmin'
		           AND r.deleted_at IS NULL AND r.status = 1
		       )
		FROM users u WHERE u.username = $1 AND u.deleted_at IS NULL`,
		o.Username).Scan(&id, &username, &status, &isSuper)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, checkTarget(false, false)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup target: %w", err)
	}
	if err := checkTarget(true, isSuper); err != nil {
		return nil, err
	}

	// ② 重置密码（复用 repo：bcrypt + must_change_password=true；RowsAffected=0
	// 返回 ErrUserNotFound——与 ① 同快照下不会发生，防御保留）
	hash, err := crypto.HashPassword(o.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	if err := repository.NewUserRepo(o.Pool).UpdatePassword(ctx, id, hash, true); err != nil {
		return nil, fmt.Errorf("update password: %w", err)
	}

	res := &ResetResult{UserID: id, Username: username, ForcedChange: true, SessionsRevoked: true, AuditWritten: true}

	// ③ 连带启用（锁死场景常见「账号被误禁 + 密码遗忘」叠加——只改密不启用
	// 恢复不完整；启用无「最后一人」类保护需求，直接置回）
	if status != 1 {
		tag, err := o.Pool.Exec(ctx,
			`UPDATE users SET status = 1, updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id)
		if err != nil {
			return res, fmt.Errorf("re-enable user: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return res, fmt.Errorf("re-enable user: target vanished mid-flow")
		}
		res.Reenabled = true
	}

	// ④ 全端会话吊销（复用导出包装；失败不阻断——DB 已提交，残留会话须对账）
	if err := service.RevokeUserSessionsWithRetry(ctx, o.RDB, id, o.AccessTTL); err != nil {
		res.SessionsRevoked = false
	}

	// ⑤ 审计留痕（best-effort：DB 直改类操作本就绕 API 审计面，此处补应用侧
	// 一行；失败时必须显式提示操作者人工补台账）
	res.AuditWritten = writeAuditLog(ctx, o.Pool, id, username, o.Note)

	return res, nil
}

// writeAuditLog 落一行审计（method='CLI' 区别于 HTTP 中间件写入的 GET/POST 族）。
func writeAuditLog(ctx context.Context, pool *pgxpool.Pool, userID int64, username, note string) bool {
	body, err := json.Marshal(map[string]string{
		"trigger": "break-glass",
		"tool":    "admincli reset-password",
		"note":    note,
	})
	if err != nil {
		body = []byte(`{"trigger":"break-glass"}`)
	}
	// 操作人快照两列（000044 同款，2026-10-13 审查补漏）：CLI 行也走行内自描述，
	// 否则该路径归档后仅剩 username（本地账号名）不可归因
	_, err = pool.Exec(ctx, `
		INSERT INTO audit_logs (user_id, username, employee_no, real_name, method, path, status_code, duration, ip, user_agent, request_body)
		SELECT $1, $2, COALESCE(u.employee_no, ''), COALESCE(u.real_name, ''), 'CLI', 'break-glass:reset-password', 200, 0, NULL, 'admincli', $3
		FROM (VALUES (1)) AS _anchor
		LEFT JOIN users u ON u.id = $1`,
		userID, username, string(body))
	if err != nil {
		slog.Error("break-glass audit insert failed; manual ledger required",
			"user_id", userID, "username", username, "err", err)
		return false
	}
	return true
}
