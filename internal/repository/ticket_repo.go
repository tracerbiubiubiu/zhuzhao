package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
)

const ticketSelectColumns = `
	id, type_code, title, COALESCE(description, '') AS description, priority, status,
	created_by, assigned_to, org_id, org_path::text, custom_data, sla_due_at,
	status_changed_at, escalation_supervisor_id, created_at, updated_at`

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
		INSERT INTO tickets (type_code, title, description, priority, status, created_by, assigned_to, org_id, org_path, custom_data, status_changed_at, escalation_supervisor_id)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, $9::ltree, $10, NOW(), $11)
		RETURNING id, created_at, updated_at, status_changed_at`
	err := exec.QueryRow(ctx, q,
		t.TypeCode, t.Title, t.Description, t.Priority, t.Status,
		t.CreatedBy, t.AssignedTo, t.OrgID, t.OrgPath, t.CustomData, t.EscalationSupervisor,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt, &t.StatusChangedAt)
	if err != nil {
		if ec := mapUniqueViolation(err); ec != nil {
			return ec
		}
		if ec := MapForeignKeyViolation(err); ec != nil {
			return ec // 并发删类型/组织（23503）→ 400，非 500
		}
		return fmt.Errorf("create ticket: %w", err)
	}
	return nil
}

// GetByID 按 ID 查询工单
func (r *TicketRepo) GetByID(ctx context.Context, id int64) (*model.Ticket, error) {
	const q = `SELECT` + ticketSelectColumns + ` FROM tickets WHERE id = $1`
	ticket, err := r.queryOne(ctx, q, id)
	if err != nil {
		return nil, err
	}
	r.enrichUserNames(ctx, []*model.Ticket{ticket})
	return ticket, nil
}

// List 分页查询工单列表（拼接 GetFilter 行级过滤）
func (r *TicketRepo) List(ctx context.Context, filter resource.Filter, q model.TicketListQuery) ([]*model.Ticket, int64, error) {
	// IW4 fail-closed 哨兵：无谓词且未显式豁免 = 漏接 L2 scope 过滤，拒绝而非静默全量。
	// 合法「全量」仅两种——Service.List 的 admin bypass / GetFilter 的 ticket_scope=all，
	// 二者都必须显式携带 Unscoped（见 00 检查单 §2.3 IW4）。
	if filter.Where == "" && !filter.Unscoped {
		return nil, 0, fmt.Errorf("ticket list: missing scope filter (obtain via registry.GetFilter, or set resource.Filter.Unscoped explicitly)")
	}
	page, pageSize := NormalizePage(q.Page, q.PageSize)

	// 拼接 WHERE：scope filter + 业务筛选（业务条件抽 ticketListConds 供单测）
	var conds []string
	var args []any
	if filter.Where != "" {
		args = append(args, filter.Args...)
		conds = append(conds, filter.Where)
	}
	// argOffset：scope filter（$1..$n）已占用的占位符数——业务条件必须从 n+1 起编，
	// 否则与 scope 占位符冲突 → PG 绑定错误 → 非 admin 任意筛选 500（2026-10-12 多账号
	// 实测发现：viewer/operator 列表筛选与工作台统计卡全 500，admin Unscoped 不受影响）
	bizConds, bizArgs := ticketListConds(q, len(args))
	conds = append(conds, bizConds...)
	args = append(args, bizArgs...)
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
	r.enrichUserNames(ctx, tickets)
	return tickets, total, nil
}

// ticketListConds 业务筛选条件拼装（scope 之外）：argOffset = scope filter 已占用的
// 占位符数，$n 从 offset+1 起编与合并后 args 严格同序（零 DB 纯函数测覆盖 offset 0/非 0）
func ticketListConds(q model.TicketListQuery, argOffset int) ([]string, []any) {
	var conds []string
	var args []any
	n := argOffset
	next := func(v any) string {
		args = append(args, v)
		n++
		return fmt.Sprintf("$%d", n)
	}
	if q.TypeCode != "" {
		conds = append(conds, "type_code = "+next(q.TypeCode))
	}
	if q.Status != "" {
		conds = append(conds, "status = "+next(q.Status))
	}
	if q.Priority != nil {
		conds = append(conds, "priority = "+next(*q.Priority))
	}
	if q.AssigneeID != nil {
		conds = append(conds, "assigned_to = "+next(*q.AssigneeID))
	}
	if q.CreatedBy != nil {
		conds = append(conds, "created_by = "+next(*q.CreatedBy))
	}
	// 标题关键字：ILIKE 子串（D2-21 转义 + ESCAPE）；纯数字追加 ID 精确匹配——
	//「粘工单号」是运维高频动作，纯标题子串永远漏命中
	if q.Keyword != "" {
		kw := next("%" + escapeLike(q.Keyword) + "%")
		if id, err := strconv.ParseInt(q.Keyword, 10, 64); err == nil {
			conds = append(conds, fmt.Sprintf("(title ILIKE %s ESCAPE '\\' OR id = %s)", kw, next(id)))
		} else {
			conds = append(conds, fmt.Sprintf("title ILIKE %s ESCAPE '\\'", kw))
		}
	}
	// 标签筛选（S10 B10-2）：可见性由外层 GetFilter 管线约束——标签不旁路 L2
	if q.Tag != "" {
		conds = append(conds, "EXISTS (SELECT 1 FROM tags tg WHERE tg.entity_type = 'ticket' AND tg.entity_id = tickets.id AND tg.tag = "+next(q.Tag)+")")
	}
	// 创建时间闭开区间（handler 已把 date 解析为 UTC 边界，to=次日 00:00）
	if q.CreatedFromAt != nil {
		conds = append(conds, "created_at >= "+next(*q.CreatedFromAt))
	}
	if q.CreatedToAt != nil {
		conds = append(conds, "created_at < "+next(*q.CreatedToAt))
	}
	return conds, args
}

// userRef 用户展示引用（2026-10-13 结构化消歧）：Name=real_name 优先空回退 username；
// No=工号独立字段（重名消歧器，前端 UserNameText 渲染「人名(工号)」三态降级）
type userRef struct {
	Name string
	No   string
}

// userRefsByIDs 批量反查用户展示引用（单次 ANY 集查询；软删用户不回填——
// 调用方按缺失降级显 ID）。回填是读侧增强：失败返回空 map 不阻断主查询。
func (r *TicketRepo) userRefsByIDs(ctx context.Context, ids []int64) map[int64]userRef {
	refs := make(map[int64]userRef, len(ids))
	if len(ids) == 0 {
		return refs
	}
	rows, err := r.db.Query(ctx,
		`SELECT id, COALESCE(NULLIF(real_name, ''), username), COALESCE(employee_no, '') FROM users WHERE id = ANY($1) AND deleted_at IS NULL`,
		ids)
	if err != nil {
		return refs
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var ref userRef
		if err := rows.Scan(&id, &ref.Name, &ref.No); err != nil {
			return refs
		}
		refs[id] = ref
	}
	if err := rows.Err(); err != nil {
		return refs
	}
	return refs
}

// enrichUserNames 批量回填创建人/处理人姓名（W4 随批件，03 S9：列表「处理人」列
// 裸 ID/N+1 消除；2026-10-13 升级结构化：name+employee_no 独立字段）。
func (r *TicketRepo) enrichUserNames(ctx context.Context, tickets []*model.Ticket) {
	if len(tickets) == 0 {
		return
	}
	ids := make([]int64, 0, len(tickets)*2)
	seen := make(map[int64]struct{}, len(tickets)*2)
	add := func(id int64) {
		if id == 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, t := range tickets {
		add(t.CreatedBy)
		if t.AssignedTo != nil {
			add(*t.AssignedTo)
		}
	}
	if len(ids) == 0 {
		return
	}
	refs := r.userRefsByIDs(ctx, ids)
	for _, t := range tickets {
		if ref, ok := refs[t.CreatedBy]; ok {
			t.CreatedByName = ref.Name
			t.CreatedByNo = ref.No
		}
		if t.AssignedTo != nil {
			// 2026-10-14 契约审计 B3：随四字段统一值类型，回填不再取址
			if ref, ok := refs[*t.AssignedTo]; ok {
				t.AssigneeName = ref.Name
				t.AssigneeNo = ref.No
			}
		}
	}
}

// Update 全量更新工单标题/描述/优先级（非事务入口；全字段非 nil = 整行覆盖语义保留）
func (r *TicketRepo) Update(ctx context.Context, t *model.Ticket) error {
	updatedAt, err := r.UpdateTx(ctx, r.db, t.ID, &t.Title, &t.Description, &t.Priority, nil)
	t.UpdatedAt = updatedAt
	return err
}

// FieldImpact 每字段在途使用计数（B1-5 随行件——全量替换前的变更代价提示）。
// 在途 = status NOT IN ('closed','rejected')；使用 = custom_data 含该键。
func (r *TicketRepo) FieldImpact(ctx context.Context, typeCode string) (map[string]int, error) {
	rows, err := r.db.Query(ctx, `
		SELECT f.field_key, COUNT(t.id)
		FROM ticket_type_fields f
		LEFT JOIN tickets t
			ON t.type_code = f.type_code
			AND t.status NOT IN ('closed', 'rejected')
			AND t.custom_data ? f.field_key
		WHERE f.type_code = $1
		GROUP BY f.field_key`, typeCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var key string
		var n int
		if err := rows.Scan(&key, &n); err != nil {
			return nil, err
		}
		out[key] = n
	}
	return out, rows.Err()
}

// LockCustomDataTx 行锁内读 custom_data 新鲜基线（分批审计 F-B1 修复 2026-10-21）：
// custom_data patch 此前基于事务外 GetByID 的基线在 Go 层合并后整列写回——两个并发
// Update 各持过期基线合并，后写者丢前写者键。本方法在更新事务内 FOR UPDATE 取基线
// （锁持至提交，同单更新串行化），合并与 G2 校验仍先行于写。0 行=已关闭（并发 close，
// BK-3 同款定性）→ ErrTicketAlreadyClosed。
func (r *TicketRepo) LockCustomDataTx(ctx context.Context, exec rowExec, ticketID int64) ([]byte, error) {
	var cd []byte
	err := exec.QueryRow(ctx,
		`SELECT custom_data FROM tickets WHERE id=$1 AND status <> 'closed' FOR UPDATE`, ticketID).Scan(&cd)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errcode.ErrTicketAlreadyClosed
	}
	if err != nil {
		return nil, fmt.Errorf("lock ticket custom_data: %w", err)
	}
	return cd, nil
}

// UpdateTx 事务内字段级更新工单（BK-3：WHERE status<>'closed' 条件更新——
// 消除「读后写」TOCTOU，并发 close 后本更新命中 0 行 → 90004；
// W4 十六批 Med：COALESCE 字段级更新——nil 字段不落 SET 覆盖，
// 两个并发 Update 各改不同字段不再互相冲掉。
// B5-4：+customData（nil 不覆盖；合并后的完整 JSONB——合并与 G2 重校验在 service 层）
func (r *TicketRepo) UpdateTx(ctx context.Context, exec rowExec, id int64, title, description *string, priority *int, customData []byte) (time.Time, error) {
	const q = `
		UPDATE tickets SET
			title = COALESCE($2, title),
			description = COALESCE($3, description),
			priority = COALESCE($4, priority),
			custom_data = COALESCE($5::jsonb, custom_data),
			updated_at = NOW()
		WHERE id = $1 AND status <> 'closed'
		RETURNING updated_at`
	var updatedAt time.Time
	err := exec.QueryRow(ctx, q, id, title, description, priority, customData).Scan(&updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, errcode.ErrTicketAlreadyClosed
		}
		return time.Time{}, fmt.Errorf("update ticket: %w", err)
	}
	return updatedAt, nil
}

// resolveClosedOrMissing 更新命中 0 行时的定性：工单不存在 → 90001；已关闭 → 90004
func (r *TicketRepo) resolveClosedOrMissing(ctx context.Context, id int64) error {
	var status string
	err := r.db.QueryRow(ctx, `SELECT status FROM tickets WHERE id = $1`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return errcode.ErrTicketNotFound
	}
	if err != nil {
		return fmt.Errorf("resolve ticket state: %w", err)
	}
	if status == "closed" {
		return errcode.ErrTicketAlreadyClosed
	}
	// 状态被并发迁移为其他值：以冲突语义拒绝（调用方重读后重试）
	return errcode.ErrConcurrentModification
}

// UpdateStatus 更新工单状态（状态机转换；fromStatus 乐观锁防并发漂移）
func (r *TicketRepo) UpdateStatus(ctx context.Context, id int64, fromStatus, toStatus string) error {
	return r.UpdateStatusTx(ctx, r.db, id, fromStatus, toStatus)
}

// UpdateStatusTx 事务内更新工单状态
// UpdateStatusTx 事务内状态迁移（CC1/CC2：WHERE status=$2 乐观锁锁定「期望旧状态」——
// 并发 Close 双写不再产生 closed→closed 脏事件；Close+Assign 竞态不再把 closed
// 工单复活为 assigned）。0 行 = 状态已漂移或工单不存在，由 resolveClosedOrMissing 定性。
func (r *TicketRepo) UpdateStatusTx(ctx context.Context, exec rowExec, id int64, fromStatus, toStatus string) error {
	// B10-4（E-2 拍定 a 案）：repo 层唯一状态写咽喉——status_changed_at 同语句
	// 同事务写入（零漂移窗口；存量路径与 S3a B3-1 转换端点全经此）
	tag, err := exec.Exec(ctx,
		`UPDATE tickets SET status = $3, updated_at = NOW(), status_changed_at = NOW() WHERE id = $1 AND status = $2`,
		id, fromStatus, toStatus)
	if err != nil {
		return fmt.Errorf("update ticket status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return r.resolveClosedOrMissing(ctx, id)
	}
	return nil
}

// UpdateAssignedToTx 事务内更新处理人（CC2：status<>'closed' 守卫——closed 工单
// 不可被分派/改派，消除并发 Close 后换处理人的窗口）
func (r *TicketRepo) UpdateAssignedToTx(ctx context.Context, exec rowExec, id int64, assignedTo *int64) error {
	tag, err := exec.Exec(ctx,
		`UPDATE tickets SET assigned_to = $2, updated_at = NOW() WHERE id = $1 AND status <> 'closed'`,
		id, assignedTo)
	if err != nil {
		return fmt.Errorf("update assigned_to: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return r.resolveClosedOrMissing(ctx, id)
	}
	return nil
}

// eventActionDeleted ticket_events.action 受控枚举值（F-S6-2-4 字面量提常量）。
// 注：与 domainevent.Action（"ticket.deleted" 点路径）异名空间——此为事件表 action 列词表。
const eventActionDeleted = "deleted"

// Delete 物理删除工单。删除权限：全局 admin bypass + 2c 组织委托（org owner/admin、
// ancestor owner，见 04 §4.2）。关联表处理：ticket_comments / ticket_relations 由
// DB ON DELETE CASCADE 清理；ticket_events 为 SET NULL（000014）——事件行存活、
// ticket_id 悬空，同事务先写 action='deleted' 事件，审计链完整可追溯操作者（HC2）。
func (r *TicketRepo) Delete(ctx context.Context, id int64, actorUserID int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO ticket_events (ticket_id, user_id, action, request_id) VALUES ($1, $2, $4, NULLIF($3, ''))`,
		id, actorUserID, reqid.From(ctx), eventActionDeleted); err != nil {
		// 23503 = 工单已不存在（重复/并发删除）→ 404，且无事件残留（BK-16 修正）
		if ec := MapForeignKeyViolation(err); ec != nil {
			return errcode.ErrTicketNotFound
		}
		return fmt.Errorf("write deleted event: %w", err)
	}
	tag, err := tx.Exec(ctx, `DELETE FROM tickets WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete ticket: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrTicketNotFound
	}
	return tx.Commit(ctx)
}

// --- 评论 / 备注 ---

// CreateComment 创建工单回复/备注（薄封装；HC1：service 层走事务版 + 评论事件）
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
	// 作者回填（2026-10-13 展示统一批；此前 CreatedBy 裸 ID）——与工单 enrich 同款
	// 批量反查；失败不阻断（调用方降级显 ID）
	ids := make([]int64, 0, len(comments))
	seen := make(map[int64]struct{}, len(comments))
	for _, c := range comments {
		if c.UserID != 0 {
			if _, ok := seen[c.UserID]; !ok {
				seen[c.UserID] = struct{}{}
				ids = append(ids, c.UserID)
			}
		}
	}
	refs := r.userRefsByIDs(ctx, ids)
	for _, c := range comments {
		if ref, ok := refs[c.UserID]; ok {
			c.CreatedByName = ref.Name
			c.CreatedByNo = ref.No
		}
	}
	return comments, nil
}

// --- 事件日志 ---

// CreateEvent 记录工单事件（审计用）
func (r *TicketRepo) CreateEvent(ctx context.Context, e *model.TicketEvent) error {
	return r.CreateEventTx(ctx, r.db, e)
}

// CreateEventTx 事务内记录事件（request_id 从 ctx 取——03 §3.4 事件↔slog↔审计同键）
func (r *TicketRepo) CreateEventTx(ctx context.Context, exec rowExec, e *model.TicketEvent) error {
	const q = `INSERT INTO ticket_events (ticket_id, user_id, action, from_value, to_value, request_id)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, '')) RETURNING id, created_at`
	err := exec.QueryRow(ctx, q, e.TicketID, e.UserID, e.Action, e.FromValue, e.ToValue, reqid.From(ctx)).Scan(&e.ID, &e.CreatedAt)
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
			COALESCE(transition_meta, '{}'::jsonb), default_sla_hours, has_custom_fields, is_active, version, created_at
		FROM ticket_types WHERE code = $1`, code).Scan(
		&t.ID, &t.Code, &t.Name, &t.Description, &t.States, &t.Transitions, &t.TransitionMeta,
		&t.DefaultSLAHours, &t.HasCustomFields, &t.IsActive, &t.Version, &t.CreatedAt,
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
			COALESCE(transition_meta, '{}'::jsonb), default_sla_hours, has_custom_fields, is_active, version, created_at
		FROM ticket_types WHERE is_active = true ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list ticket types: %w", err)
	}
	defer rows.Close()
	types, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.TicketType, error) {
		var t model.TicketType
		if err := row.Scan(&t.ID, &t.Code, &t.Name, &t.Description, &t.States, &t.Transitions, &t.TransitionMeta,
			&t.DefaultSLAHours, &t.HasCustomFields, &t.IsActive, &t.Version, &t.CreatedAt); err != nil {
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
		SELECT id, type_code, field_key, field_label, field_type, field_options, field_meta, required, COALESCE(validate_regex, ''), sort_order
		FROM ticket_type_fields WHERE type_code = $1 ORDER BY sort_order ASC, id ASC`, typeCode)
	if err != nil {
		return nil, fmt.Errorf("list ticket type fields: %w", err)
	}
	defer rows.Close()
	fields, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.TicketTypeField, error) {
		var f model.TicketTypeField
		if err := row.Scan(&f.ID, &f.TypeCode, &f.FieldKey, &f.FieldLabel, &f.FieldType, &f.FieldOptions, &f.FieldMeta, &f.Required, &f.ValidateRegex, &f.SortOrder); err != nil {
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
			version, org_id, org_path::text, created_by, created_at, updated_at
		FROM ticket_templates WHERE code = $1 AND deleted_at IS NULL`, code).Scan(
		&t.ID, &t.Code, &t.Name, &t.TypeCode, &t.DefaultPriority, &t.DefaultFields, &t.DefaultSLAMinutes, &t.Version,
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
			version, org_id, org_path::text, created_by, created_at, updated_at
		FROM ticket_templates WHERE deleted_at IS NULL ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list ticket templates: %w", err)
	}
	defer rows.Close()
	templates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.TicketTemplate, error) {
		var t model.TicketTemplate
		if err := row.Scan(&t.ID, &t.Code, &t.Name, &t.TypeCode, &t.DefaultPriority, &t.DefaultFields, &t.DefaultSLAMinutes, &t.Version,
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

// ExistsRelationBetween BK-5：双向判重——A→B 与 B→A 视为同一关联。
// 【P1-2 更新】并发唯一性现由迁移 000028 的「规范化对」部分唯一索引
// uq_ticket_relations_normalized(LEAST(src,tgt), GREATEST(src,tgt), relation_type)
// 兜底；本前置检查仅用于「顺序重复」的良好 UX（早返回 409），
// 不再承担唯一的并发正确性保证（其读取在事务外，与 INSERT 之间存在竞态窗口）。
func (r *TicketRepo) ExistsRelationBetween(ctx context.Context, source, target int64, relType string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM ticket_relations
			WHERE relation_type = $3
			  AND ((source_ticket_id = $1 AND target_ticket_id = $2)
			    OR (source_ticket_id = $2 AND target_ticket_id = $1))
		)`, source, target, relType).Scan(&exists)
	return exists, err
}

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
		// 23505：并发/顺序重复关联（uq_ticket_relations_normalized 规范化对唯一索引，
		// 迁移 000028）→ mapUniqueViolation 默认映射 errcode.ErrConflict（409），
		// 与 service 层预检的 ErrConflict 语义一致。
		if ec := mapUniqueViolation(err); ec != nil {
			return ec
		}
		if ec := MapForeignKeyViolation(err); ec != nil {
			return ec // 并发删关联工单（23503）→ 400，非 500
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
		&t.StatusChangedAt, &t.EscalationSupervisor, &t.CreatedAt, &t.UpdatedAt,
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

// ===== IW3/BK-18：类型/字段/模板管理（写侧） =====

// CreateTicketType 新建工单类型（states/transitions 由 service 层给默认值或已校验 JSON）
func (r *TicketRepo) CreateTicketType(ctx context.Context, t *model.TicketType) error {
	const q = `
		INSERT INTO ticket_types (code, name, description, states, transitions, default_sla_hours, is_active)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7)
		RETURNING id, created_at`
	err := r.db.QueryRow(ctx, q,
		t.Code, t.Name, t.Description, t.States, t.Transitions, t.DefaultSLAHours, t.IsActive,
	).Scan(&t.ID, &t.CreatedAt)
	if err != nil {
		if ec := mapUniqueViolation(err); ec != nil {
			return ec
		}
		return fmt.Errorf("create ticket type: %w", err)
	}
	return nil
}

// UpdateTicketType 更新类型（patch：COALESCE 保持；code 不可改）。
// P1-3 乐观锁：version == nil → 不做 CAS（保持旧 patch 语义）；version != nil →
// 谓词 AND version = $7，命中才写，且恒有 SET version = version + 1。
// 0 行（ErrNoRows）再定性：类型不存在 → 90003；存在但 version 不匹配 → 409 冲突。
func (r *TicketRepo) UpdateTicketType(ctx context.Context, code string, name *string, description *string, states, transitions, transitionMeta json.RawMessage, isActive *bool, version *int) (*model.TicketType, error) {
	q := `
		UPDATE ticket_types SET
			name = COALESCE($2, name),
			description = COALESCE($3, description),
			states = COALESCE($4, states),
			transitions = COALESCE($5, transitions),
			transition_meta = COALESCE($6, transition_meta),
			is_active = COALESCE($7, is_active),
			version = version + 1
		WHERE code = $1`
	args := []any{code, name, description, states, transitions, transitionMeta, isActive}
	if version != nil {
		q += ` AND version = $8`
		args = append(args, *version)
	}
	q += `
		RETURNING id, code, name, COALESCE(description, ''), states, transitions,
			COALESCE(transition_meta, '{}'::jsonb), default_sla_hours, has_custom_fields, is_active, version, created_at`
	var t model.TicketType
	err := r.db.QueryRow(ctx, q, args...).Scan(
		&t.ID, &t.Code, &t.Name, &t.Description, &t.States, &t.Transitions, &t.TransitionMeta,
		&t.DefaultSLAHours, &t.HasCustomFields, &t.IsActive, &t.Version, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// 0 行 = 类型不存在 或 version 不匹配；给最精确的语义码。
			return nil, r.resolveTypeVersionMiss(ctx, code, version)
		}
		return nil, fmt.Errorf("update ticket type: %w", err)
	}
	return &t, nil
}

// resolveTypeVersionMiss 类型更新命中 0 行时的定性：
// 类型不存在 → 90003；存在但并发更新（version 漂移）→ 409。
func (r *TicketRepo) resolveTypeVersionMiss(ctx context.Context, code string, version *int) error {
	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ticket_types WHERE code = $1)`, code).Scan(&exists); err != nil {
		return fmt.Errorf("resolve ticket type existence: %w", err)
	}
	if !exists {
		return errcode.ErrTicketTypeNotFound
	}
	if version != nil {
		return errcode.ErrConcurrentModification
	}
	// 理论上不可达（无 CAS 时存在即命中）；保守返回冲突而非静默。
	return errcode.ErrConcurrentModification
}

// DeleteTicketType 删除类型（有工单禁删；字段随类型一并删除）
func (r *TicketRepo) DeleteTicketType(ctx context.Context, code string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var cnt int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM tickets WHERE type_code = $1`, code).Scan(&cnt); err != nil {
		return fmt.Errorf("count tickets by type: %w", err)
	}
	if cnt > 0 {
		return errcode.ErrConflict // 有工单禁删，走停用
	}
	if _, err := tx.Exec(ctx, `DELETE FROM ticket_type_fields WHERE type_code = $1`, code); err != nil {
		return fmt.Errorf("delete type fields: %w", err)
	}
	tag, err := tx.Exec(ctx, `DELETE FROM ticket_types WHERE code = $1`, code)
	if err != nil {
		return fmt.Errorf("delete ticket type: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrTicketTypeNotFound
	}
	return tx.Commit(ctx)
}

// ReplaceTypeFields 全量替换类型字段集（事务内先清后插）+ has_custom_fields 同步。
//
// P1-3 乐观锁决策：以「父类型 ticket_types.version」作为唯一 CAS 谓词（同一个事务内
// 先 UPDATE ticket_types SET version = version + 1 WHERE code = $1 [AND version = $n]，
// 0 行 → 409）。理由：字段集是类型配置的一部分，父类型版本是唯一权威；字段行在
// Replace 语义下每次整体清空重插，无自然单行版本语义，故 ticket_type_fields.version
// 列（迁移 000027 添加）在本方法中不作为并发谓词，仅保持三表结构一致。
// version == nil → 不做 CAS（保持旧行为），但仍递增父版本（使后续携版本的调用
// 能观测到本次修改）。
func (r *TicketRepo) ReplaceTypeFields(ctx context.Context, code string, fields []model.TicketTypeField, version *int) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. 父类型版本 CAS（先于任何字段写入，锁父行）
	casQ := `UPDATE ticket_types SET version = version + 1 WHERE code = $1`
	casArgs := []any{code}
	if version != nil {
		casQ += ` AND version = $2`
		casArgs = append(casArgs, *version)
	}
	tag, err := tx.Exec(ctx, casQ, casArgs...)
	if err != nil {
		return fmt.Errorf("bump ticket type version: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// 0 行 = 类型不存在 或 version 不匹配；给最精确的语义码。
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ticket_types WHERE code = $1)`, code).Scan(&exists); err != nil {
			return fmt.Errorf("resolve ticket type existence: %w", err)
		}
		if !exists {
			return errcode.ErrTicketTypeNotFound
		}
		return errcode.ErrConcurrentModification
	}

	// 2. 清空后整体重插字段集
	if _, err := tx.Exec(ctx, `DELETE FROM ticket_type_fields WHERE type_code = $1`, code); err != nil {
		return fmt.Errorf("clear type fields: %w", err)
	}
	for i := range fields {
		f := &fields[i]
		if _, err := tx.Exec(ctx, `
			INSERT INTO ticket_type_fields
				(type_code, field_key, field_label, field_type, field_options, field_meta, required, validate_regex, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9)`,
			code, f.FieldKey, f.FieldLabel, f.FieldType, f.FieldOptions, f.FieldMeta, f.Required, f.ValidateRegex, f.SortOrder); err != nil {
			return fmt.Errorf("insert type field %s: %w", f.FieldKey, err)
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE ticket_types SET has_custom_fields = $2 WHERE code = $1`, code, len(fields) > 0); err != nil {
		return fmt.Errorf("sync has_custom_fields: %w", err)
	}
	return tx.Commit(ctx)
}

// ListTicketTypeFieldsAll 管理端全量读取（与只读版同列，含 validate_regex）
func (r *TicketRepo) ListTicketTypeFieldsAll(ctx context.Context, typeCode string) ([]*model.TicketTypeField, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, type_code, field_key, field_label, field_type, field_options, field_meta, required, COALESCE(validate_regex, ''), sort_order
		FROM ticket_type_fields WHERE type_code = $1 ORDER BY sort_order ASC, id ASC`, typeCode)
	if err != nil {
		return nil, fmt.Errorf("list type fields: %w", err)
	}
	defer rows.Close()
	fields, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.TicketTypeField, error) {
		var f model.TicketTypeField
		if err := row.Scan(&f.ID, &f.TypeCode, &f.FieldKey, &f.FieldLabel, &f.FieldType, &f.FieldOptions, &f.FieldMeta, &f.Required, &f.ValidateRegex, &f.SortOrder); err != nil {
			return nil, err
		}
		return &f, nil
	})
	return fields, err
}

// ListTicketTypesAdmin 管理端全量（含停用）
func (r *TicketRepo) ListTicketTypesAdmin(ctx context.Context) ([]*model.TicketType, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, code, name, COALESCE(description, ''), states, transitions,
			COALESCE(transition_meta, '{}'::jsonb), default_sla_hours, has_custom_fields, is_active, version, created_at
		FROM ticket_types ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list ticket types admin: %w", err)
	}
	defer rows.Close()
	types, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.TicketType, error) {
		var t model.TicketType
		if err := row.Scan(&t.ID, &t.Code, &t.Name, &t.Description, &t.States, &t.Transitions, &t.TransitionMeta,
			&t.DefaultSLAHours, &t.HasCustomFields, &t.IsActive, &t.Version, &t.CreatedAt); err != nil {
			return nil, err
		}
		return &t, nil
	})
	return types, err
}

// CreateTicketTemplate 新建模模板（org_path 由 service 层解析后传入）
func (r *TicketRepo) CreateTicketTemplate(ctx context.Context, t *model.TicketTemplate) error {
	const q = `
		INSERT INTO ticket_templates
			(code, name, type_code, default_priority, default_fields, default_sla_minutes, org_id, org_path, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::ltree, $9)
		RETURNING id, created_at, updated_at`
	err := r.db.QueryRow(ctx, q,
		t.Code, t.Name, t.TypeCode, t.DefaultPriority, t.DefaultFields, t.DefaultSLAMinutes,
		t.OrgID, t.OrgPath, t.CreatedBy,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if ec := mapUniqueViolation(err); ec != nil {
			return ec
		}
		return fmt.Errorf("create ticket template: %w", err)
	}
	return nil
}

// UpdateTicketTemplate 更新模板（patch：COALESCE 保持；code/type_code/org 不可改）。
// P1-3 乐观锁：version == nil → 不做 CAS（保持旧 patch 语义）；version != nil →
// 谓词 AND version = $6，命中才写，且恒有 SET version = version + 1。
// 0 行（ErrNoRows）再定性：模板不存在/已软删 → 404；存在但 version 不匹配 → 409 冲突。
func (r *TicketRepo) UpdateTicketTemplate(ctx context.Context, code string, name *string, defaultPriority *int, defaultFields json.RawMessage, defaultSLAMinutes *int, version *int) (*model.TicketTemplate, error) {
	q := `
		UPDATE ticket_templates SET
			name = COALESCE($2, name),
			default_priority = COALESCE($3, default_priority),
			default_fields = COALESCE($4, default_fields),
			default_sla_minutes = COALESCE($5, default_sla_minutes),
			version = version + 1,
			updated_at = NOW()
		WHERE code = $1 AND deleted_at IS NULL`
	args := []any{code, name, defaultPriority, defaultFields, defaultSLAMinutes}
	if version != nil {
		q += ` AND version = $6`
		args = append(args, *version)
	}
	q += `
		RETURNING id, code, name, type_code, default_priority, default_fields, default_sla_minutes,
			version, org_id, org_path::text, created_by, created_at, updated_at`
	var t model.TicketTemplate
	err := r.db.QueryRow(ctx, q, args...).Scan(
		&t.ID, &t.Code, &t.Name, &t.TypeCode, &t.DefaultPriority, &t.DefaultFields, &t.DefaultSLAMinutes, &t.Version,
		&t.OrgID, &t.OrgPath, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, r.resolveTemplateVersionMiss(ctx, code, version)
		}
		return nil, fmt.Errorf("update ticket template: %w", err)
	}
	return &t, nil
}

// resolveTemplateVersionMiss 模板更新命中 0 行时的定性：
// 模板不存在/已软删 → 404；存在但并发更新（version 漂移）→ 409。
func (r *TicketRepo) resolveTemplateVersionMiss(ctx context.Context, code string, version *int) error {
	var exists bool
	if err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM ticket_templates WHERE code = $1 AND deleted_at IS NULL)`, code).Scan(&exists); err != nil {
		return fmt.Errorf("resolve ticket template existence: %w", err)
	}
	if !exists {
		return errcode.ErrNotFound
	}
	if version != nil {
		return errcode.ErrConcurrentModification
	}
	return errcode.ErrConcurrentModification
}

// DeleteTicketTemplate 物理删除模板（无引用方）
func (r *TicketRepo) DeleteTicketTemplate(ctx context.Context, code string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM ticket_templates WHERE code = $1`, code)
	if err != nil {
		return fmt.Errorf("delete ticket template: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrNotFound
	}
	return nil
}

// UpdateEscalationSupervisorTx 事务内更新催办上级·人工指定（S10 B10-4——dd §31
// 基线档；幻影用户 FK 23503 → 400）。
func (r *TicketRepo) UpdateEscalationSupervisorTx(ctx context.Context, exec rowExec, id int64, supervisorID int64) error {
	_, err := exec.Exec(ctx,
		`UPDATE tickets SET escalation_supervisor_id = $2, updated_at = NOW() WHERE id = $1`,
		id, supervisorID)
	if ec := MapForeignKeyViolation(err); ec != nil {
		return errcode.New(ec.Code, "催办上级用户不存在")
	}
	return err
}
