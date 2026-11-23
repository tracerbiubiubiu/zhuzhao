// recording.go — 分派/接单业务事务内的配置化提醒记录面（票侧工作流的接缝）。
//
// 调用合同：
//   - 调用方持真实 IAM 命令守卫事务（WithIAMCommandGuard 的 tx），并已按既有
//     IAM→风险 scope→票 锁序持有票行锁——本面在票锁之后只按序取提醒行；
//   - RecordConfiguredAssignment 在已锁票下取 generation=1 或 MAX+1，以
//     asOf+发布 AcceptWithin 固定接单截止，复用已审 RecordAssignmentTx 落事实；
//   - RecordConfiguredAcceptance 只认当前代次、本人、钉版图上
//     待接单状态→票现势状态 的合法实际接单边——刷新/读/通知类调用不改票态，
//     永不构成接单；
//   - 返回 recorded=true 仅当实际新动作进入 recorder；同键重放返回原行
//     （recorded=false，原时间事实唯一）；全部写入走调用方事务，任一故障随
//     事务回滚，不留半状态。
package riskreminder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// ErrNotAcceptanceEdge 票现势状态不构成钉版图上的合法实际接单边（票仍在待接单
// 状态、经非法路径到达或转换未发布）——可见错误，不记接单。
var ErrNotAcceptanceEdge = errors.New("ticket state is not a legal first-acceptance edge")

// RecordConfiguredAssignment 分派事务内的配置化记录：读取钉版发布的提醒策略，
// 无发布 → (nil,false,nil) 不落任何提醒事实；有发布 → 代次=1 或 MAX+1、
// 截止=asOf+发布 AcceptWithin（固定截止，不取票面 SLA）、经已审
// RecordAssignmentTx 同事务落分派事实。同键重放：与当前代次事实
// （被指派人/操作者/固定截止/发布规则）完全一致的重提返回原行 recorded=false；
// 只有实际新动作进入 recorder。
func (s *Service) RecordConfiguredAssignment(ctx context.Context, tx pgx.Tx, ticketID, assigneeID, actorID int64, asOf time.Time) (*Assignment, bool, error) {
	if err := s.guardedCommandTx(ctx, tx); err != nil {
		return nil, false, err
	}
	if ticketID <= 0 || assigneeID <= 0 || actorID <= 0 || asOf.IsZero() {
		return nil, false, errcode.ErrInvalidParams
	}
	policy, _, err := boundAcceptancePublicationTx(ctx, s.db, tx, ticketID)
	if err != nil {
		if errors.Is(err, ErrReminderPolicyAbsent) {
			return nil, false, nil // 钉版未发布提醒——无提醒事实
		}
		return nil, false, err
	}
	dispatchedAt := asOf.Truncate(time.Microsecond)
	acceptDueAt := dispatchedAt.Add(policy.AcceptWithin)

	// 同键重放只看当前代次：与历史代次巧合同事实的重分派是新动作（新代次）。
	existing, err := readCurrentAssignmentByFactsTx(ctx, tx, ticketID, assigneeID, actorID, acceptDueAt, policy.RuleID, policy.RuleVersion)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, false, nil
	}

	var generation int64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(generation),0)+1 FROM riskreminder_assignments WHERE ticket_id=$1`,
		ticketID).Scan(&generation); err != nil {
		return nil, false, err
	}
	a, err := RecordAssignmentTx(ctx, tx, &AssignmentInput{
		TicketID: ticketID, Generation: generation,
		AssigneeID: assigneeID, DispatchedBy: actorID,
		DispatchedAt: dispatchedAt, AcceptDueAt: acceptDueAt,
		RuleID: policy.RuleID, RuleVersion: policy.RuleVersion,
	})
	if err != nil {
		return nil, false, err
	}
	return a, true, nil
}

// RecordConfiguredAcceptance 实际接单事务内的配置化记录：仅当前代次、本人
// （票面主责与分派事实一致）、钉版图上 待接单状态→票现势状态 的合法实际接单边。
// 已接单重放返回原行 recorded=false（原时间事实唯一，不覆盖）；
// 票仍在待接单状态（刷新/读/通知类调用）→ ErrNotAcceptanceEdge，不记接单。
// 无发布 → (nil,false,nil)——未配置提醒的票接单不产生任何提醒事实。
func (s *Service) RecordConfiguredAcceptance(ctx context.Context, tx pgx.Tx, ticketID, acceptorID int64, asOf time.Time) (*Assignment, bool, error) {
	if err := s.guardedCommandTx(ctx, tx); err != nil {
		return nil, false, err
	}
	if ticketID <= 0 || acceptorID <= 0 || asOf.IsZero() {
		return nil, false, errcode.ErrInvalidParams
	}
	policy, snapshot, err := boundAcceptancePublicationTx(ctx, s.db, tx, ticketID)
	if err != nil {
		if errors.Is(err, ErrReminderPolicyAbsent) {
			return nil, false, nil
		}
		return nil, false, err
	}

	var assignedTo *int64
	var status string
	err = tx.QueryRow(ctx, `SELECT assigned_to, status FROM tickets WHERE id=$1`, ticketID).
		Scan(&assignedTo, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, errcode.ErrNotFound
	}
	if err != nil {
		return nil, false, fmt.Errorf("read acceptance ticket facts: %w", err)
	}
	if assignedTo == nil || *assignedTo != acceptorID {
		return nil, false, errcode.ErrNoPermission // 接单只认票面现势主责本人
	}
	var generation int64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(generation),0) FROM riskreminder_assignments WHERE ticket_id=$1`,
		ticketID).Scan(&generation); err != nil {
		return nil, false, err
	}
	if generation == 0 {
		return nil, false, errcode.ErrNotFound // 发布存在但无分派事实——调用序错误
	}
	a, err := readAssignmentTx(ctx, tx, ticketID, generation)
	if err != nil {
		return nil, false, err
	}
	if a.AssigneeID != acceptorID {
		return nil, false, errcode.ErrNoPermission // 仅当前代次被分派本人
	}
	if a.AcceptedAt != nil {
		return a, false, nil // 接单重放：原时间事实唯一，不覆盖、不新增动作
	}
	// 合法实际接单边：票已沿已发布转换离开待接单状态。刷新/读/通知不改票态
	// （仍在待接单状态）——在此显式拒绝，不得记为接单。
	var graph map[string][]string
	if json.Unmarshal(snapshot.Type.Transitions, &graph) != nil {
		return nil, false, fmt.Errorf("reminder publication transitions unreadable")
	}
	if !slices.Contains(graph[policy.AwaitingState], status) {
		return nil, false, fmt.Errorf("%w: from %q to %q", ErrNotAcceptanceEdge, policy.AwaitingState, status)
	}
	a, already, err := RecordAcceptedTx(ctx, tx, ticketID, generation, asOf, acceptorID)
	if err != nil {
		return nil, false, err
	}
	return a, !already, nil
}

// guardedCommandTx 分派/接单记录只服务真实 IAM 命令守卫事务（与 checker 同一
// 判据：守卫激活且传入 tx 就是守卫事务本体）——裸事务/其他连接拒绝。
func (s *Service) guardedCommandTx(ctx context.Context, tx pgx.Tx) error {
	if s == nil || s.db == nil || tx == nil || !repository.IAMGuardActive(ctx) || repository.IAMDB(ctx, s.db) != tx {
		return errcode.ErrNoPermission
	}
	return nil
}

// readCurrentAssignmentByFactsTx 同键重放探测：当前代次内按完整分派事实精确匹配
// （截止时刻为 µs 精度——与 normalizeAssignmentInput 同精度，跨精度比较不误判）。
func readCurrentAssignmentByFactsTx(ctx context.Context, tx pgx.Tx, ticketID, assigneeID, actorID int64, acceptDueAt time.Time, ruleID string, ruleVersion int64) (*Assignment, error) {
	var a Assignment
	err := tx.QueryRow(ctx, `SELECT `+assignmentCols+`
		FROM riskreminder_assignments
		WHERE ticket_id=$1
		  AND generation=(SELECT COALESCE(MAX(generation),0) FROM riskreminder_assignments WHERE ticket_id=$1)
		  AND assignee_id=$2 AND dispatched_by=$3
		  AND accept_due_at=$4 AND rule_id=$5 AND rule_version=$6
		ORDER BY generation DESC LIMIT 1`,
		ticketID, assigneeID, actorID, acceptDueAt, ruleID, ruleVersion,
	).Scan(&a.ID, &a.TicketID, &a.Generation, &a.AssigneeID, &a.DispatchedBy,
		&a.DispatchedAt, &a.AcceptDueAt, &a.RuleID, &a.RuleVersion, &a.AcceptedAt, &a.AcceptedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}
