package repository

import (
	"context"
	"encoding/json"
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

// archiveTables 归档目标表白名单（B11②；表名来自代码常量而非外部输入，防注入）。
var archiveTables = map[string]bool{
	"audit_logs":             true,
	"policy_evaluation_logs": true,
}

// ArchiveRow 归档行（id 用于删除；line 为 JSONL 序列化结果）。
type ArchiveRow struct {
	ID   int64
	Line []byte
}

// ArchiveFetchBatch 取一批超期行（created_at < cutoff），按 id 升序；line 按行 JSON 编码。
// 单表单批——调用方「导出成功后按同批 id 删行」（03 §4.2 原子语义）。
func (r *AuditLogRepo) ArchiveFetchBatch(ctx context.Context, table string, cutoff time.Time, limit int) ([]ArchiveRow, error) {
	if !archiveTables[table] {
		return nil, fmt.Errorf("archive: table %q not allowlisted", table)
	}
	q := fmt.Sprintf(`SELECT * FROM %s WHERE created_at < $1 ORDER BY id LIMIT $2`, table)
	rows, err := r.db.Query(ctx, q, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("archive fetch %s: %w", table, err)
	}
	defer rows.Close()
	out := make([]ArchiveRow, 0, limit)
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("archive scan %s: %w", table, err)
		}
		desc := rows.FieldDescriptions()
		obj := make(map[string]interface{}, len(desc))
		var id int64
		for i, d := range desc {
			name := string(d.Name)
			obj[name] = values[i]
			if name == "id" {
				if v, ok := values[i].(int64); ok {
					id = v
				}
			}
		}
		line, err := json.Marshal(obj)
		if err != nil {
			return nil, fmt.Errorf("archive marshal %s: %w", table, err)
		}
		out = append(out, ArchiveRow{ID: id, Line: line})
	}
	return out, rows.Err()
}

// ArchiveDeleteBatch 删除已导出的同批行（id 列表精确匹配）。
func (r *AuditLogRepo) ArchiveDeleteBatch(ctx context.Context, table string, ids []int64) error {
	if !archiveTables[table] {
		return fmt.Errorf("archive: table %q not allowlisted", table)
	}
	if len(ids) == 0 {
		return nil
	}
	q := fmt.Sprintf(`DELETE FROM %s WHERE id = ANY($1)`, table)
	if _, err := r.db.Exec(ctx, q, ids); err != nil {
		return fmt.Errorf("archive delete %s: %w", table, err)
	}
	return nil
}
