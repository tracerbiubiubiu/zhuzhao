package service

import (
	"context"
	"fmt"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// AuditService 审计日志服务
type AuditService struct {
	auditRepo *repository.AuditLogRepo
}

func NewAuditService(auditRepo *repository.AuditLogRepo) *AuditService {
	return &AuditService{auditRepo: auditRepo}
}

// Insert 同步写入审计日志（实现 middleware.AuditLogger）
func (s *AuditService) Insert(ctx context.Context, entry middleware.AuditLogEntry) error {
	return fmt.Errorf("not implemented")
}

// Query 查询审计日志
func (s *AuditService) Query(ctx context.Context, page, pageSize int, filters map[string]interface{}) (interface{}, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}
