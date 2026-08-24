package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
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
// WithoutCancel + 独立超时，覆盖 auth_service 全部 11 处 LogLogin 调用
func (s *AuditService) LogLogin(ctx context.Context, employeeNo, ip, userAgent string, userID *int64, username string, statusCode int) {
	entry := middleware.AuditLogEntry{
		Method:      "POST",
		Path:        "/api/v1/auth/login",
		StatusCode:  statusCode,
		IP:          ip,
		UserAgent:   userAgent,
		RequestBody: loginAuditBody(employeeNo),
		CreatedAt:   time.Now(),
	}
	if userID != nil {
		entry.UserID = *userID
		entry.Username = username
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), loginAuditWriteTimeout)
	defer cancel()
	if err := s.Insert(writeCtx, entry); err != nil {
		slog.Error("audit log write failed", "err", err, "path", entry.Path)
	}
}

// loginAuditWriteTimeout 登录审计写入独立超时（与 middleware.auditWriteTimeout 对齐）
const loginAuditWriteTimeout = 3 * time.Second

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
		CreatedAt:   entry.CreatedAt,
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
