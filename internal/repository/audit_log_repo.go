package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// AuditListQuery 审计日志筛选
type AuditListQuery struct {
	Page     int
	PageSize int
	UserID   *int64
	Path     string
	Start    *time.Time
	End      *time.Time
}

// AuditLogRepo 审计日志数据访问
type AuditLogRepo struct {
	db *pgxpool.Pool
}

func NewAuditLogRepo(db *pgxpool.Pool) *AuditLogRepo {
	return &AuditLogRepo{db: db}
}

func (r *AuditLogRepo) Create(ctx context.Context, log *model.AuditLog) error {
	createdAt := log.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO audit_logs (
			user_id, username, method, path, status_code, duration,
			ip, user_agent, request_body, created_at
		) VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), $10)`,
		log.UserID,
		log.Username,
		log.Method,
		log.Path,
		log.StatusCode,
		log.Duration,
		log.IP,
		log.UserAgent,
		log.RequestBody,
		createdAt,
	)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

func (r *AuditLogRepo) List(ctx context.Context, q AuditListQuery) ([]*model.AuditLog, int64, error) {
	page, pageSize := normalizePage(q.Page, q.PageSize)

	where, args := buildAuditListWhere(q)
	countSQL := `SELECT COUNT(*) FROM audit_logs` + where
	var total int64
	if err := r.db.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}

	listArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	listSQL := `
		SELECT id, user_id, COALESCE(username, ''), method, path, status_code, duration,
			COALESCE(ip, ''), COALESCE(user_agent, ''), COALESCE(request_body, ''), created_at
		FROM audit_logs` + where + `
		ORDER BY created_at DESC, id DESC
		LIMIT $` + fmt.Sprint(len(args)+1) + ` OFFSET $` + fmt.Sprint(len(args)+2)

	rows, err := r.db.Query(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()

	logs, err := pgx.CollectRows(rows, scanAuditLogRow)
	if err != nil {
		return nil, 0, fmt.Errorf("collect audit logs: %w", err)
	}
	return logs, total, nil
}

func buildAuditListWhere(q AuditListQuery) (string, []any) {
	var parts []string
	var args []any
	n := 1

	if q.UserID != nil {
		parts = append(parts, fmt.Sprintf("user_id = $%d", n))
		args = append(args, *q.UserID)
		n++
	}
	if q.Path != "" {
		parts = append(parts, fmt.Sprintf("path = $%d", n))
		args = append(args, q.Path)
		n++
	}
	if q.Start != nil {
		parts = append(parts, fmt.Sprintf("created_at >= $%d", n))
		args = append(args, *q.Start)
		n++
	}
	if q.End != nil {
		parts = append(parts, fmt.Sprintf("created_at < $%d", n))
		args = append(args, q.End.Add(24*time.Hour))
		n++
	}
	if len(parts) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

func scanAuditLogRow(row pgx.CollectableRow) (*model.AuditLog, error) {
	var log model.AuditLog
	err := row.Scan(
		&log.ID,
		&log.UserID,
		&log.Username,
		&log.Method,
		&log.Path,
		&log.StatusCode,
		&log.Duration,
		&log.IP,
		&log.UserAgent,
		&log.RequestBody,
		&log.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &log, nil
}
