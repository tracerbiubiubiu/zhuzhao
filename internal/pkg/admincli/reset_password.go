// Package admincli 管理面 CLI 公共逻辑（break-glass 应急通道）。
//
// 当前仅 superadmin 密码重置一件：复用与管理端 ResetPassword 完全相同的语义
// （bcrypt hash + must_change_password=true + 全端会话吊销），差别只在授权方式
// ——不走 HTTP/会话，凭「容器执行权」授权：能 docker exec 进 app 容器者本已
// 具备宿主级权限，本包不新增任何提权面（对比 HTTP break-glass 端点会在认证
// 系统上开新攻击面，故不采用）。输出/交互（确认、录密）留在 cmd/server 包装层。
//
// C05（底座 §4.2/运维 §6.2）：重置走统一安全事务——同事务写口令/代际/回执/
// 投影意图；启用顺序受安全门约束：先确认本次重置的撤销投影已完成，才允许
// 连带启用（推进新代）。投影失败时 DB 安全事实保留、不启用，操作者按
// runbook §6.2 受权重试或等 worker 恢复。
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

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
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
	OperationID     string
	Reenabled       bool // 原处于禁用态且撤销投影已确认，本次已连带启用（推进新代）
	SessionsRevoked bool // 原重置代际的撤销投影是否确认
	AuditWritten    bool // audit_logs 是否落行（false=须人工台账补记）
	ForcedChange    bool // 恒 true：must_change_password=true，下次登录强制改密
	// PendingReason 投影待恢复原因（非空时启用被安全门拒绝，操作者恢复后重跑）
	PendingReason string
}

// ResetSuperadminPassword break-glass 重置：目标校验 → 统一安全事务重置（同事务
// 写口令/代际/回执/意图）→ 投影确认 → 连带启用（仅投影确认后）→ 审计留痕。
// 口令不进回执/指纹/日志（F021/底座 §4.4 秘密输入纪律）。
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

	hash, err := crypto.HashPassword(o.Password) // bcrypt 在锁外计算
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	userRepo := repository.NewUserRepo(o.Pool)
	deps := service.NewSessionWorkerDeps(o.Pool, userRepo, o.RDB, o.AccessTTL)
	opID := "cli-reset:" + service.ResolveOperationID("")
	var id int64
	var wasDisabled bool
	var newEpoch int64
	guardErr := repository.WithIAMGuard(ctx, o.Pool, func(gctx context.Context) error {
		var err error
		id, err = lookupSuperadmin(gctx, repository.IAMDB(gctx, o.Pool), o.Username)
		if err != nil {
			return err
		}
		var desiredActive bool
		newEpoch, _, desiredActive, err = service.ExecuteCLIPasswordReset(gctx, deps, id, opID, hash)
		if err != nil {
			return err
		}
		wasDisabled = !desiredActive
		return service.ProjectAfterCommit(gctx, deps, id, newEpoch)
	})
	if guardErr != nil && !errors.Is(guardErr, errcode.ErrServiceUnavailable) {
		return nil, guardErr
	}
	res := &ResetResult{UserID: id, Username: o.Username, OperationID: opID, ForcedChange: true}
	defer func() { res.AuditWritten = writeAuditLog(ctx, o.Pool, id, o.Username, o.Note) }()
	if guardErr != nil {
		res.PendingReason = "口令重置已提交，会话撤销投影待核查。按 runbook §6.2 核查原操作。"
		return res, nil
	}
	res.SessionsRevoked = true

	if !wasDisabled {
		return res, nil
	}
	enableEpoch, err := repository.WithIAMGuardValue(ctx, o.Pool, func(gctx context.Context) (int64, error) {
		currentID, err := lookupSuperadmin(gctx, repository.IAMDB(gctx, o.Pool), o.Username)
		if err != nil {
			return 0, err
		}
		if currentID != id {
			return 0, errcode.ErrConcurrentModification
		}
		return service.ExecuteCLIEnable(gctx, deps, id, newEpoch)
	})
	if errors.Is(err, errcode.ErrServiceUnavailable) && enableEpoch > 0 {
		res.Reenabled = true
		res.PendingReason = "账号启用已提交，新代际会话尚未确认就绪。按 runbook §6.2 核查。"
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("re-enable user: %w", err)
	}
	res.Reenabled = true
	return res, nil
}

func lookupSuperadmin(ctx context.Context, db repository.IAMDBTX, username string) (int64, error) {
	var id int64
	var isSuperadmin bool
	err := db.QueryRow(ctx, `
		SELECT u.id, EXISTS(
		    SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id
		    WHERE ur.user_id = u.id AND r.code = 'superadmin'
		      AND r.deleted_at IS NULL AND r.status = 1)
		FROM users u WHERE u.username = $1 AND u.deleted_at IS NULL`, username).Scan(&id, &isSuperadmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, checkTarget(false, false)
	}
	if err != nil {
		return 0, fmt.Errorf("lookup target: %w", err)
	}
	return id, checkTarget(true, isSuperadmin)
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
