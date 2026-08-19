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
}

func NewAuditService(auditRepo *repository.AuditLogRepo) *AuditService {
	return &AuditService{auditRepo: auditRepo}
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

// LogLogin 登录审计（公开路由不走 AuditLog 中间件，由 AuthService 显式调用）
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
	if err := s.Insert(ctx, entry); err != nil {
		slog.Error("audit log write failed", "err", err, "path", entry.Path)
	}
}

// List 查询审计日志
func (s *AuditService) List(ctx context.Context, q repository.AuditListQuery) (*model.AuditLogListResponse, error) {
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

func normalizeAuditPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
