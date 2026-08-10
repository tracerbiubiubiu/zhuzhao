package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// AuditLogRepo 审计日志数据访问
type AuditLogRepo struct {
	db *pgxpool.Pool
}

func NewAuditLogRepo(db *pgxpool.Pool) *AuditLogRepo {
	return &AuditLogRepo{db: db}
}

func (r *AuditLogRepo) Create(ctx context.Context, log *model.AuditLog) error {
	return fmt.Errorf("not implemented")
}

func (r *AuditLogRepo) List(ctx context.Context, page, pageSize int, filters map[string]interface{}) ([]*model.AuditLog, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}
