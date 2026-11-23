// 分派/接单两个业务事务内的内核写入面（facts——只存事实，不判策略）。
package riskreminder

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// RecordAssignmentTx 分派事务内落分派事实（同事务：代次、指派人、分派时已发布的
// 提醒参数和接单期限）。写入成功后抑止旧代次 pending 意图（reason=reassigned）——
// 新代次的意图不受影响（「旧代次不抑制重入」，risk-module-detail §14-3）。
//
// 幂等合同（验收卡「同号重试只取得原结果」）：同 (ticket,generation) 重复调用且
// 业务事实（指派人/接单期限/规则版本）一致 → 返回原行不重写；事实不一致 → ErrConflict。
// 并发同代次插入由 ON CONFLICT 仲裁；未插入方在有效事务内重读并比较。
func RecordAssignmentTx(ctx context.Context, tx pgx.Tx, in *AssignmentInput) (*Assignment, error) {
	if in == nil || in.TicketID <= 0 || in.Generation <= 0 || in.AssigneeID <= 0 ||
		in.DispatchedBy <= 0 || in.AcceptDueAt.IsZero() || in.RuleID == "" || in.RuleVersion <= 0 {
		return nil, errcode.ErrInvalidParams
	}
	norm := normalizeAssignmentInput(in)

	existing, err := readAssignmentTx(ctx, tx, in.TicketID, in.Generation)
	switch {
	case err == nil:
		if err := compareAssignment(existing, norm); err != nil {
			return nil, err
		}
		return existing, nil // 原回执与记录各一份——不重写
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}

	created := &Assignment{
		TicketID: in.TicketID, Generation: in.Generation,
		AssigneeID: in.AssigneeID, DispatchedBy: in.DispatchedBy,
		DispatchedAt: norm.DispatchedAt, AcceptDueAt: norm.AcceptDueAt,
		RuleID: in.RuleID, RuleVersion: in.RuleVersion,
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO riskreminder_assignments
			(ticket_id, generation, assignee_id, dispatched_by, dispatched_at, accept_due_at, rule_id, rule_version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (ticket_id, generation) DO NOTHING
 RETURNING id`,
		created.TicketID, created.Generation, created.AssigneeID, created.DispatchedBy,
		created.DispatchedAt, created.AcceptDueAt, created.RuleID, created.RuleVersion,
	).Scan(&created.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) { // 并发同代次：不终止事务，重读先到者
			existing, rerr := readAssignmentTx(ctx, tx, in.TicketID, in.Generation)
			if rerr != nil {
				return nil, rerr
			}
			if err := compareAssignment(existing, norm); err != nil {
				return nil, err
			}
			return existing, nil
		}
		return nil, err
	}

	// 旧代次待发抑止（delivered 保留——已发提醒和超时历史可查）
	if _, err := tx.Exec(ctx, `
		UPDATE riskreminder_intents i
		SET status = $4::varchar, suppressed_at = now(), suppress_reason = $3::text
		FROM riskreminder_assignments a
		WHERE i.assignment_id = a.id
		  AND a.ticket_id = $1 AND a.generation < $2
		  AND i.status = $5::varchar`,
		in.TicketID, in.Generation, ReasonReassigned, StatusSuppressed, StatusPending,
	); err != nil {
		return nil, err
	}
	return created, nil
}

// RecordAcceptedTx 首次实际接单事务内保存原时间事实并抑止待发（同事务）。
// 已有 accepted_at 时不覆盖（原时间事实唯一），返回 already=true；
// 抑止范围仅本代次 pending 意图（reason=accepted），已发（delivered）保留可查。
// 「接单只停止接单提醒」——超期历史由 accepted_at 与 accept_due_at 的事实差可查。
// assignment 缺失说明调用序错误（接单先于分派记录）→ ErrNotFound。
func RecordAcceptedTx(ctx context.Context, tx pgx.Tx, ticketID, generation int64, acceptedAt time.Time, acceptedBy int64) (*Assignment, bool, error) {
	if ticketID <= 0 || generation <= 0 || acceptedBy <= 0 {
		return nil, false, errcode.ErrInvalidParams
	}
	if acceptedAt.IsZero() {
		acceptedAt = time.Now()
	}
	acceptedAt = acceptedAt.Truncate(time.Microsecond) // PG timestamptz 微秒精度（原时间事实）
	a, err := readAssignmentTx(ctx, tx, ticketID, generation)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, errcode.ErrNotFound
		}
		return nil, false, err
	}
	if a.AcceptedAt != nil {
		return a, true, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE riskreminder_assignments SET accepted_at = $2, accepted_by = $3 WHERE id = $1 AND accepted_at IS NULL`,
		a.ID, acceptedAt, acceptedBy); err != nil {
		return nil, false, err
	}
	a.AcceptedAt, a.AcceptedBy = &acceptedAt, &acceptedBy
	if _, err := tx.Exec(ctx, `
		UPDATE riskreminder_intents
		SET status = $3::varchar, suppressed_at = now(), suppress_reason = $4::text
		WHERE assignment_id = $1 AND status = $2::varchar`,
		a.ID, StatusPending, StatusSuppressed, ReasonAccepted); err != nil {
		return nil, false, err
	}
	return a, false, nil
}

// IntentsByTicket 该票全部意图（待发/已发/抑止——「已发提醒和超时记录可查」读面；
// 暴露给主会话时按其权限面收敛，本内核不做授权判定）。
func (s *Service) IntentsByTicket(ctx context.Context, ticketID int64) ([]*Intent, error) {
	rows, err := s.db.Query(ctx, `SELECT `+intentCols+`
		FROM riskreminder_intents WHERE ticket_id = $1 ORDER BY id`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectIntents(rows)
}

// ─── 内部 ───

const assignmentCols = `id, ticket_id, generation, assignee_id, dispatched_by,
	dispatched_at, accept_due_at, rule_id, rule_version, accepted_at, accepted_by`

const intentCols = `id, assignment_id, ticket_id, generation, recipient_id, due_at,
	status, source, source_event_id, rule_id, rule_version, channel, semantic_key,
	notification_id, delivered_at, suppressed_at, suppress_reason,
	attempt_count, last_attempted_at, last_error, created_at`

func readAssignmentTx(ctx context.Context, tx pgx.Tx, ticketID, generation int64) (*Assignment, error) {
	var a Assignment
	err := tx.QueryRow(ctx, `SELECT `+assignmentCols+`
		FROM riskreminder_assignments WHERE ticket_id = $1 AND generation = $2 FOR UPDATE`,
		ticketID, generation,
	).Scan(&a.ID, &a.TicketID, &a.Generation, &a.AssigneeID, &a.DispatchedBy,
		&a.DispatchedAt, &a.AcceptDueAt, &a.RuleID, &a.RuleVersion, &a.AcceptedAt, &a.AcceptedBy)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// normalizeAssignmentInput 幂等比较/入库前的规范化：时刻收敛到 PG timestamptz
// 的微秒精度（time.Time 是纳秒精度，直接比较会让带纳秒分量的同事实重试误判
// 漂移冲突——时间精度幂等）；零值分派时钟落到当前时刻。
func normalizeAssignmentInput(in *AssignmentInput) *AssignmentInput {
	norm := *in
	if norm.DispatchedAt.IsZero() {
		norm.DispatchedAt = time.Now()
	}
	norm.DispatchedAt = norm.DispatchedAt.Truncate(time.Microsecond)
	norm.AcceptDueAt = norm.AcceptDueAt.Truncate(time.Microsecond)
	return &norm
}

// compareAssignment 同代次幂等比较——只比业务事实（指派人/接单期限/规则版本）；
// 入参应先经 normalizeAssignmentInput 收敛（PG 只存微秒，比较精度须一致）；
// dispatched_at 属记录时刻，重试带新时钟不算事实漂移（原行原时刻保留）。
func compareAssignment(a *Assignment, in *AssignmentInput) error {
	if a.AssigneeID != in.AssigneeID || a.RuleID != in.RuleID || a.RuleVersion != in.RuleVersion ||
		!a.AcceptDueAt.Equal(in.AcceptDueAt) {
		return errcode.ErrConflict
	}
	return nil
}

func scanIntent(row pgx.Row) (*Intent, error) {
	var i Intent
	if err := row.Scan(&i.ID, &i.AssignmentID, &i.TicketID, &i.Generation, &i.RecipientID,
		&i.DueAt, &i.Status, &i.Source, &i.SourceEventID, &i.RuleID, &i.RuleVersion,
		&i.Channel, &i.SemanticKey, &i.NotificationID, &i.DeliveredAt, &i.SuppressedAt,
		&i.SuppressReason, &i.AttemptCount, &i.LastAttemptedAt, &i.LastError, &i.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &i, nil
}

func collectIntents(rows pgx.Rows) ([]*Intent, error) {
	var out []*Intent
	for rows.Next() {
		i, err := scanIntent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
