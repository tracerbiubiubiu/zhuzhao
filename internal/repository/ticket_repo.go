package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
)

const ticketSelectColumns = `
	id, type_code, title, COALESCE(description, '') AS description, priority, status,
	created_by, assigned_to, org_id, org_path::text, custom_data, sla_due_at, created_at, updated_at`

// TicketRepo 工单数据访问
type TicketRepo struct {
	db *pgxpool.Pool
}

// NewTicketRepo 创建 TicketRepo
func NewTicketRepo(db *pgxpool.Pool) *TicketRepo {
	return &TicketRepo{db: db}
}

// Create 创建工单（事务内写 org_path + ticket_events）
func (r *TicketRepo) Create(ctx context.Context, t *model.Ticket) error {
	return r.CreateTx(ctx, r.db, t)
}

// CreateTx 事务内创建工单
func (r *TicketRepo) CreateTx(ctx context.Context, exec rowExec, t *model.Ticket) error {
	const q = `
		INSERT INTO tickets (type_code, title, description, priority, status, created_by, assigned_to, org_id, org_path, custom_data)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, $9::ltree, $10)
		RETURNING id, created_at, updated_at`
	err := exec.QueryRow(ctx, q,
		t.TypeCode, t.Title, t.Description, t.Priority, t.Status,
		t.CreatedBy, t.AssignedTo, t.OrgID, t.OrgPath, t.CustomData,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if ec := mapUniqueViolation(err); ec != nil {
			return ec
		}
		return fmt.Errorf("create ticket: %w", err)
	}
	return nil
}

// GetByID 按 ID 查询工单
func (r *TicketRepo) GetByID(ctx context.Context, id int64) (*model.Ticket, error) {
	const q = `SELECT` + ticketSelectColumns + ` FROM tickets WHERE id = $1`
	return r.queryOne(ctx, q, id)
}

// List 分页查询工单列表（拼接 GetFilter 行级过滤）
func (r *TicketRepo) List(ctx context.Context, filter resource.Filter, q model.TicketListQuery) ([]*model.Ticket, int64, error) {
	page, pageSize := normalizePage(q.Page, q.PageSize)

	// 拼接 WHERE：scope filter + 业务筛选
	var conds []string
	var args []any
	if filter.Where != "" {
		args = append(args, filter.Args...)
		conds = append(conds, filter.Where)
	}
	if q.TypeCode != "" {
		args = append(args, q.TypeCode)
		conds = append(conds, fmt.Sprintf("type_code = $%d", len(args)))
	}
	if q.Status != "" {
		args = append(args, q.Status)
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	if q.Priority != nil {
		args = append(args, *q.Priority)
		conds = append(conds, fmt.Sprintf("priority = $%d", len(args)))
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + joinConds(conds)
	}

	// count
	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM tickets`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tickets: %w", err)
	}

	// list
	listArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	listSQL := `SELECT` + ticketSelectColumns + ` FROM tickets` + where +
		` ORDER BY id DESC LIMIT $` + fmt.Sprint(len(args)+1) + ` OFFSET $` + fmt.Sprint(len(args)+2)
	rows, err := r.db.Query(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list tickets: %w", err)
	}
	defer rows.Close()
	tickets, err := pgx.CollectRows(rows, scanTicketCollectableRow)
	if err != nil {
		return nil, 0, fmt.Errorf("collect tickets: %w", err)
	}
	return tickets, total, nil
}

// Update 更新工单标题/描述/优先级（patch 语义）
func (r *TicketRepo) Update(ctx context.Context, t *model.Ticket) error {
	const q = `
		UPDATE tickets SET
			title = $2,
			description = $3,
			priority = $4,
			updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at`
	err := r.db.QueryRow(ctx, q, t.ID, t.Title, t.Description, t.Priority).Scan(&t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errcode.ErrTicketNotFound
		}
		return fmt.Errorf("update ticket: %w", err)
	}
	return nil
}

// UpdateStatus 更新工单状态（状态机转换）
func (r *TicketRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	return r.UpdateStatusTx(ctx, r.db, id, status)
}

// UpdateStatusTx 事务内更新工单状态
func (r *TicketRepo) UpdateStatusTx(ctx context.Context, exec rowExec, id int64, status string) error {
	tag, err := exec.Exec(ctx, `UPDATE tickets SET status = $2, updated_at = NOW() WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("update ticket status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrTicketNotFound
	}
	return nil
}

// UpdateAssignedTo 更新处理人（分派/取消分派）
func (r *TicketRepo) UpdateAssignedTo(ctx context.Context, id int64, assignedTo *int64) error {
	return r.UpdateAssignedToTx(ctx, r.db, id, assignedTo)
}

// UpdateAssignedToTx 事务内更新处理人
func (r *TicketRepo) UpdateAssignedToTx(ctx context.Context, exec rowExec, id int64, assignedTo *int64) error {
	tag, err := exec.Exec(ctx, `UPDATE tickets SET assigned_to = $2, updated_at = NOW() WHERE id = $1`, id, assignedTo)
	if err != nil {
		return fmt.Errorf("update assigned_to: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrTicketNotFound
	}
	return nil
}

// Delete 物理删除工单（Phase 2a：admin only，关联表由 DB CASCADE 处理）
func (r *TicketRepo) Delete(ctx context.Context, id int64) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM tickets WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete ticket: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrTicketNotFound
	}
	return nil
}

// --- 评论 / 备注 ---

// CreateComment 创建工单回复/备注
func (r *TicketRepo) CreateComment(ctx context.Context, c *model.TicketComment) error {
	return r.CreateCommentTx(ctx, r.db, c)
}

// CreateCommentTx 事务内创建工单回复/备注
func (r *TicketRepo) CreateCommentTx(ctx context.Context, exec rowExec, c *model.TicketComment) error {
	const q = `INSERT INTO ticket_comments (ticket_id, user_id, content, is_internal) VALUES ($1, $2, $3, $4) RETURNING id, created_at`
	err := exec.QueryRow(ctx, q, c.TicketID, c.UserID, c.Content, c.IsInternal).Scan(&c.ID, &c.CreatedAt)
	if err != nil {
		return fmt.Errorf("create comment: %w", err)
	}
	return nil
}

// ListComments 查询工单回复列表。
// includeInternal=false 时过滤内部备注（BK-1：透明读旁观者仅见公开回复）。
func (r *TicketRepo) ListComments(ctx context.Context, ticketID int64, includeInternal bool) ([]*model.TicketComment, error) {
	where := "WHERE ticket_id = $1"
	if !includeInternal {
		where += " AND is_internal = false"
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, ticket_id, user_id, content, is_internal, created_at
		FROM ticket_comments `+where+` ORDER BY created_at ASC`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()
	comments, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.TicketComment, error) {
		var c model.TicketComment
		if err := row.Scan(&c.ID, &c.TicketID, &c.UserID, &c.Content, &c.IsInternal, &c.CreatedAt); err != nil {
			return nil, err
		}
		return &c, nil
	})
	if err != nil {
		return nil, fmt.Errorf("collect comments: %w", err)
	}
	return comments, nil
}

// --- 事件日志 ---

// CreateEvent 记录工单事件（审计用）
func (r *TicketRepo) CreateEvent(ctx context.Context, e *model.TicketEvent) error {
	return r.CreateEventTx(ctx, r.db, e)
}

// CreateEventTx 事务内记录事件
func (r *TicketRepo) CreateEventTx(ctx context.Context, exec rowExec, e *model.TicketEvent) error {
	const q = `INSERT INTO ticket_events (ticket_id, user_id, action, from_value, to_value) VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, '')) RETURNING id, created_at`
	err := exec.QueryRow(ctx, q, e.TicketID, e.UserID, e.Action, e.FromValue, e.ToValue).Scan(&e.ID, &e.CreatedAt)
	if err != nil {
		return fmt.Errorf("create event: %w", err)
	}
	return nil
}

// --- 工单类型 ---

// GetTicketType 按 code 查询工单类型
func (r *TicketRepo) GetTicketType(ctx context.Context, code string) (*model.TicketType, error) {
	var t model.TicketType
	err := r.db.QueryRow(ctx, `
		SELECT id, code, name, COALESCE(description, ''), states, transitions,
			default_sla_hours, has_custom_fields, is_active, created_at
		FROM ticket_types WHERE code = $1`, code).Scan(
		&t.ID, &t.Code, &t.Name, &t.Description, &t.States, &t.Transitions,
		&t.DefaultSLAHours, &t.HasCustomFields, &t.IsActive, &t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errcode.ErrTicketTypeNotFound
		}
		return nil, fmt.Errorf("get ticket type: %w", err)
	}
	return &t, nil
}

// ListTicketTypes 查询全部启用工单类型
func (r *TicketRepo) ListTicketTypes(ctx context.Context) ([]*model.TicketType, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, code, name, COALESCE(description, ''), states, transitions,
			default_sla_hours, has_custom_fields, is_active, created_at
		FROM ticket_types WHERE is_active = true ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list ticket types: %w", err)
	}
	defer rows.Close()
	types, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.TicketType, error) {
		var t model.TicketType
		if err := row.Scan(&t.ID, &t.Code, &t.Name, &t.Description, &t.States, &t.Transitions,
			&t.DefaultSLAHours, &t.HasCustomFields, &t.IsActive, &t.CreatedAt); err != nil {
			return nil, err
		}
		return &t, nil
	})
	if err != nil {
		return nil, fmt.Errorf("collect ticket types: %w", err)
	}
	return types, nil
}

// ListTicketTypeFields 查询工单类型字段定义
func (r *TicketRepo) ListTicketTypeFields(ctx context.Context, typeCode string) ([]*model.TicketTypeField, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, type_code, field_key, field_label, field_type, field_options, required, sort_order
		FROM ticket_type_fields WHERE type_code = $1 ORDER BY sort_order ASC, id ASC`, typeCode)
	if err != nil {
		return nil, fmt.Errorf("list ticket type fields: %w", err)
	}
	defer rows.Close()
	fields, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.TicketTypeField, error) {
		var f model.TicketTypeField
		if err := row.Scan(&f.ID, &f.TypeCode, &f.FieldKey, &f.FieldLabel, &f.FieldType, &f.FieldOptions, &f.Required, &f.SortOrder); err != nil {
			return nil, err
		}
		return &f, nil
	})
	if err != nil {
		return nil, fmt.Errorf("collect ticket type fields: %w", err)
	}
	return fields, nil
}

// --- 工单模板（2a 前移）---

// GetTicketTemplate 按 code 查询工单模板
func (r *TicketRepo) GetTicketTemplate(ctx context.Context, code string) (*model.TicketTemplate, error) {
	var t model.TicketTemplate
	err := r.db.QueryRow(ctx, `
		SELECT id, code, name, type_code, default_priority, default_fields, default_sla_minutes,
			org_id, org_path::text, created_by, created_at, updated_at
		FROM ticket_templates WHERE code = $1 AND deleted_at IS NULL`, code).Scan(
		&t.ID, &t.Code, &t.Name, &t.TypeCode, &t.DefaultPriority, &t.DefaultFields, &t.DefaultSLAMinutes,
		&t.OrgID, &t.OrgPath, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errcode.ErrNotFound
		}
		return nil, fmt.Errorf("get ticket template: %w", err)
	}
	return &t, nil
}

// ListTicketTemplates 查询工单模板列表
func (r *TicketRepo) ListTicketTemplates(ctx context.Context) ([]*model.TicketTemplate, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, code, name, type_code, default_priority, default_fields, default_sla_minutes,
			org_id, org_path::text, created_by, created_at, updated_at
		FROM ticket_templates WHERE deleted_at IS NULL ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list ticket templates: %w", err)
	}
	defer rows.Close()
	templates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.TicketTemplate, error) {
		var t model.TicketTemplate
		if err := row.Scan(&t.ID, &t.Code, &t.Name, &t.TypeCode, &t.DefaultPriority, &t.DefaultFields, &t.DefaultSLAMinutes,
			&t.OrgID, &t.OrgPath, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		return &t, nil
	})
	if err != nil {
		return nil, fmt.Errorf("collect ticket templates: %w", err)
	}
	return templates, nil
}

// --- 工单关联（2a 前移）---

// CreateRelation 建立工单关联
func (r *TicketRepo) CreateRelation(ctx context.Context, rel *model.TicketRelation) error {
	relType := rel.RelationType
	if relType == "" {
		relType = "related"
	}
	err := r.db.QueryRow(ctx, `
		INSERT INTO ticket_relations (source_ticket_id, target_ticket_id, relation_type, created_by)
		VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		rel.SourceTicketID, rel.TargetTicketID, relType, rel.CreatedBy,
	).Scan(&rel.ID, &rel.CreatedAt)
	if err != nil {
		if ec := mapUniqueViolation(err); ec != nil {
			return ec
		}
		return fmt.Errorf("create relation: %w", err)
	}
	return nil
}

// ListRelations 查询工单关联列表
func (r *TicketRepo) ListRelations(ctx context.Context, ticketID int64) ([]*model.TicketRelation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, source_ticket_id, target_ticket_id, relation_type, created_by, created_at
		FROM ticket_relations
		WHERE deleted_at IS NULL AND (source_ticket_id = $1 OR target_ticket_id = $1)
		ORDER BY created_at ASC`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("list relations: %w", err)
	}
	defer rows.Close()
	relations, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.TicketRelation, error) {
		var rel model.TicketRelation
		if err := row.Scan(&rel.ID, &rel.SourceTicketID, &rel.TargetTicketID, &rel.RelationType, &rel.CreatedBy, &rel.CreatedAt); err != nil {
			return nil, err
		}
		return &rel, nil
	})
	if err != nil {
		return nil, fmt.Errorf("collect relations: %w", err)
	}
	return relations, nil
}

// --- helpers ---

func (r *TicketRepo) queryOne(ctx context.Context, q string, args ...any) (*model.Ticket, error) {
	row := r.db.QueryRow(ctx, q, args...)
	ticket, err := scanTicketRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errcode.ErrTicketNotFound
		}
		return nil, err
	}
	return ticket, nil
}

func scanTicketRow(row pgx.Row) (*model.Ticket, error) {
	var t model.Ticket
	err := row.Scan(
		&t.ID, &t.TypeCode, &t.Title, &t.Description, &t.Priority, &t.Status,
		&t.CreatedBy, &t.AssignedTo, &t.OrgID, &t.OrgPath, &t.CustomData, &t.SLADueAt,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func scanTicketCollectableRow(row pgx.CollectableRow) (*model.Ticket, error) {
	return scanTicketRow(row)
}

// joinConds 用 AND 连接 WHERE 条件（filter.Where 可能含 OR，需括号包裹）
func joinConds(conds []string) string {
	result := ""
	for i, c := range conds {
		if i > 0 {
			result += " AND "
		}
		// scope filter 可能含 OR，用括号包裹保证优先级正确
		if containsOr(c) {
			result += "(" + c + ")"
		} else {
			result += c
		}
	}
	return result
}

func containsOr(s string) bool {
	return strings.HasPrefix(s, "(") || strings.Contains(s, " OR ") || strings.Contains(s, " or ")
}
