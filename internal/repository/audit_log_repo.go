package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/audit"
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
			ip, user_agent, request_body, request_id, created_at
		) VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), $11)`,
		log.UserID,
		log.Username,
		log.Method,
		log.Path,
		log.StatusCode,
		log.Duration,
		log.IP,
		log.UserAgent,
		log.RequestBody,
		log.RequestID,
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

// InsertPolicyEvals 批量写判定日志（B11① policy_evaluation_logs；audit.PolicyEvalStore 实现）。
// 多行单语句插入；空批次直接返回（flusher 保证非空调用，防御分支）。
func (r *AuditLogRepo) InsertPolicyEvals(ctx context.Context, rows []audit.PolicyEvalEntry) error {
	if len(rows) == 0 {
		return nil
	}
	values := make([]string, 0, len(rows))
	args := make([]interface{}, 0, len(rows)*9)
	for i, e := range rows {
		base := i * 9
		values = append(values, fmt.Sprintf(
			"($%d, $%d::text[], $%d, NULLIF($%d,''), $%d, $%d, NULLIF($%d,''), NULLIF($%d,''), $%d::timestamptz)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9))
		args = append(args,
			e.ActorID, e.ActorRoles, e.ResourceType, e.ResourceID, e.Action,
			e.Result, e.Reason, e.TraceID, e.CreatedAt)
	}
	q := `INSERT INTO policy_evaluation_logs
		(actor_id, actor_role_codes, resource_type, resource_id, action, result, reason, trace_id, created_at)
		VALUES ` + strings.Join(values, ",")
	if _, err := r.db.Exec(ctx, q, args...); err != nil {
		return fmt.Errorf("insert policy evals (%d rows): %w", len(rows), err)
	}
	return nil
}

var _ audit.PolicyEvalStore = (*AuditLogRepo)(nil)
