package service

import (
	"context"
	"fmt"

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

// Record 异步记录审计日志
func (s *AuditService) Record(log *model.AuditLog) {
	// TODO: Phase 2 实现，channel + goroutine 异步写入
}

// Query 查询审计日志
func (s *AuditService) Query(ctx context.Context, page, pageSize int, filters map[string]interface{}) (interface{}, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}
