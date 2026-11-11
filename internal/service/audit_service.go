package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// AuditService 审计日志服务
type AuditService struct {
	auditRepo *repository.AuditLogRepo
	userRepo  *repository.UserRepo
}

func NewAuditService(auditRepo *repository.AuditLogRepo, userRepo *repository.UserRepo) *AuditService {
	return &AuditService{auditRepo: auditRepo, userRepo: userRepo}
}

// Insert 同步写入审计日志（实现 middleware.AuditLogger）。
// 除登录外，所有需鉴权路由由 middleware.AuditLog 在请求结束后调用本方法写入。
func (s *AuditService) Insert(ctx context.Context, entry middleware.AuditLogEntry) error {
	log := entryToModel(entry)
	if err := s.auditRepo.Create(ctx, log); err != nil {
		return err
	}
	return nil
}

// LogLogin 登录审计（公开路由不走 AuditLog 中间件，由 AuthService 显式调用）。
// D2-04：与中间件路径（F-5）对齐——登录请求的 ctx 随客户端断连取消，
// 直接透传会丢登录成功/失败审计（撞库攻击无 DB 证据）；改用
// WithoutCancel + 独立超时，覆盖 auth_service 全部 12 处 LogLogin 调用
func (s *AuditService) LogLogin(ctx context.Context, employeeNo, ip, userAgent string, userID *int64, username string, statusCode int) {
	entry := middleware.AuditLogEntry{
		Method:      "POST",
		Path:        "/api/v1/auth/login",
		StatusCode:  statusCode,
		IP:          ip,
		UserAgent:   userAgent,
		RequestBody: loginAuditBody(employeeNo),
		// request_id 自 request context 取（中间件已注入 reqid）——03 §3.4 同键
		// 关联口径：登录审计行此前空缺该键，反查断链
		RequestID: reqid.From(ctx),
		CreatedAt: time.Now(),
	}
	if userID != nil {
		entry.UserID = *userID
		entry.Username = username
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), loginAuditWriteTimeout)
	defer cancel()
	if err := s.Insert(writeCtx, entry); err != nil {
		slog.Error("audit log write failed", "request_id", entry.RequestID, "err", err, "path", entry.Path)
	}
}

// loginAuditWriteTimeout 登录审计写入独立超时（与 middleware.auditWriteTimeout 对齐）
const loginAuditWriteTimeout = 3 * time.Second

// LogRefreshReplay 刷新令牌重放审计（C-5/RT-1——2026-10-27 观测先行批）。
// /auth/refresh 为公开路由不走 AuditLog 中间件，20015 命中（槽位已推进而旧 RT
// 重现 = 盗用重放/迟到提交）此前仅以 401 响应可见、DB 零证据。记录≠告警：多标签
// 并发刷新/弱网迟到提交等合法重放同样命中，降噪前提（宽限窗口/family_id）只 gate
// 告警面不 gate 记录面——维持台账 C-5 原触发条件。WithoutCancel+独立超时与
// LogLogin 同纪律（D2-04：客户端断连不丢安全证据）。
func (s *AuditService) LogRefreshReplay(ctx context.Context, userID int64, username, ip, userAgent string) {
	entry := middleware.AuditLogEntry{
		Method:          "POST",
		Path:            "/api/v1/auth/refresh",
		StatusCode:      401,
		IP:              ip,
		UserAgent:       userAgent,
		RequestBody:     refreshReplayAuditBody(userID, username),
		ResponseSummary: "RT-1: refresh token replayed (slot advanced, stale RT resurfaced)",
		RequestID:       reqid.From(ctx),
		UserID:          userID,
		Username:        username,
		CreatedAt:       time.Now(),
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), loginAuditWriteTimeout)
	defer cancel()
	if err := s.Insert(writeCtx, entry); err != nil {
		slog.Error("audit log write failed", "request_id", entry.RequestID, "err", err, "path", entry.Path)
	}
}

func refreshReplayAuditBody(userID int64, username string) string {
	b, _ := json.Marshal(map[string]any{
		"user_id": userID, "username": username, "err": 20015,
	})
	return string(b)
}

// List 查询审计日志；employeeNo 非空时按工号解析为 user_id（优先于 q.UserID）。
// D2-27：按工号解析走含软删查询——用户软删后其历史审计仍应可查（原 404）
func (s *AuditService) List(ctx context.Context, q repository.AuditListQuery, employeeNo string) (*model.AuditLogListResponse, error) {
	if employeeNo != "" {
		user, err := s.userRepo.FindByEmployeeNoIncludeDeleted(ctx, employeeNo)
		if err != nil {
			return nil, err
		}
		q.UserID = &user.ID
	}

	page, pageSize := normalizeAuditPage(q.Page, q.PageSize)
	q.Page = page
	q.PageSize = pageSize

	list, total, err := s.auditRepo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	return &model.AuditLogListResponse{
		List:     list,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func entryToModel(entry middleware.AuditLogEntry) *model.AuditLog {
	log := &model.AuditLog{
		Username:    entry.Username,
		Method:      entry.Method,
		Path:        entry.Path,
		StatusCode:  entry.StatusCode,
		Duration:    entry.Duration,
		IP:          entry.IP,
		UserAgent:   entry.UserAgent,
		RequestBody: entry.RequestBody,
		// 000036 加列、中间件 capWriter 一直在采（有 limit 封顶）、repo INSERT 也写 $10——唯独此处漏拷，
		// 37150 行审计 response_summary 非空 0 行（2026-10-26 外审 #5 验证时核出）
		ResponseSummary: entry.ResponseSummary,
		RequestID:       entry.RequestID,
		CreatedAt:       entry.CreatedAt,
	}
	if entry.UserID != 0 {
		uid := entry.UserID
		log.UserID = &uid
	}
	return log
}

func loginAuditBody(employeeNo string) string {
	b, _ := json.Marshal(map[string]string{
		"employee_no": employeeNo,
		"password":    "***",
	})
	return string(b)
}

// normalizeAuditPage 审计分页规范化。
// B4-6：page 上限 10000——原无上限，超大 page 产生巨量 OFFSET 扫描
// （audit_logs 只增表，恶意/误操作可造成 DB CPU/IO 放大）。
// Phase 2 统一各模块分页上限为通用 normalizePage。
func normalizeAuditPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if page > 10000 {
		page = 10000
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
