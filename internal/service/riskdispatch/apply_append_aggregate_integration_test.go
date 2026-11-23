//go:build integration

package riskdispatch

// 详设 §5「工单 priority 取有效关联风险中的最高级别；business_due_at 取最早的风险
// deadline……追加、移出、改级或改期时重新计算工单值」：已有在办票追加必须以
// 全部有效关联重算票面聚合（真实工单可读值）与组回执聚合事实。两方向各证一漂移：
// 更高级别追加（票面漂移）与更低级别追加（回执漂移）；另证 already_linked 批
// 不得以本批计划级别改写已存聚合。

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func requireTicketPriority(t *testing.T, db *pgxpool.Pool, ticketID int64) int {
	t.Helper()
	var priority int
	require.NoError(t, db.QueryRow(context.Background(),
		`SELECT priority FROM tickets WHERE id = $1`, ticketID).Scan(&priority))
	return priority
}

func requireGroupBusinessDue(t *testing.T, db *pgxpool.Pool, ticketID int64) time.Time {
	t.Helper()
	var due time.Time
	require.NoError(t, db.QueryRow(context.Background(),
		`SELECT business_due_at FROM risk_dispatch_groups WHERE ticket_id = $1`, ticketID).Scan(&due))
	return due.UTC()
}

// requireReceiptAggregate 读组回执保存的聚合事实（priority/business_due_at）。
func requireReceiptAggregate(t *testing.T, db *pgxpool.Pool, actorID int64, operationID string) (int, time.Time) {
	t.Helper()
	var priority int
	var due time.Time
	require.NoError(t, db.QueryRow(context.Background(), `
		SELECT (result->>'priority')::int, (result->>'business_due_at')::timestamptz
		FROM risk_dispatch_group_receipts
		WHERE actor_id = $1 AND operation_id = $2
	`, actorID, operationID).Scan(&priority, &due))
	return priority, due.UTC()
}

func requireUpdatedEventCount(t *testing.T, db *pgxpool.Pool, ticketID int64) int64 {
	t.Helper()
	return countRows(t, db,
		`SELECT COUNT(*) FROM ticket_events WHERE ticket_id = $1 AND action = 'updated'`, ticketID)
}

// newAggregateTestDispatcher 单批次可见性/授权的最小装配（复用共享夹具与合成桥）。
func newAggregateTestDispatcher(f dispatchFixture, defs map[string]DefinitionRef, risks ...int64) *Dispatcher {
	allowed := map[string]bool{}
	visible := map[string]bool{}
	for _, id := range risks {
		allowed[fmtKey(f.actorID, id)] = true
		visible[fmtKey(f.actorID, id)] = true
	}
	return newTestDispatcher(dispatchTestDB, &stubAuthorizer{allowed: allowed}, &stubFacts{visible: visible},
		&stubTicketCreator{}, stubTicketViews{visible: map[int64]bool{}, all: true}, defs)
}

// 更高级别追加：票面优先级必须随全部有效关联重算（原票 medium → 追加 urgent 后应为 1），
// 组到期取最早 deadline；真实变更按工单域惯例落一次 'updated' 历史。
func TestApplyAppendRecalculatesTicketPriorityAcrossActiveLinks(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "aggup")
	finding := "finding-agg-up"
	late := testDeadline
	early := testDeadline.Add(-72 * time.Hour)
	riskMedium := f.newRisk(t, db, "aggup", finding, "asset-m", f.ownerA, "medium", late, "assessed")
	riskUrgent := f.newRisk(t, db, "aggup", finding, "asset-u", f.ownerA, "urgent", early, "assessed")
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}

	// 首批 medium：票面 3、组到期 late
	d := newAggregateTestDispatcher(f, defs, riskMedium)
	op1 := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{riskMedium}, map[int64]int64{riskMedium: 1}), "op-agg-up-1")
	first, err := d.Apply(context.Background(), f.actorID, op1)
	require.NoError(t, err)
	require.Equal(t, "completed", first.Status)
	ticketID := first.Groups[0].Ticket.ID
	require.Equal(t, 3, requireTicketPriority(t, db, ticketID))
	require.WithinDuration(t, late, requireGroupBusinessDue(t, db, ticketID), time.Second)
	receiptPriority, receiptDue := requireReceiptAggregate(t, db, f.actorID, op1.OperationID)
	require.Equal(t, 3, receiptPriority)
	require.WithinDuration(t, late, receiptDue, time.Second)

	// 追加 urgent（更早到期）：票面随全部有效关联重算为 1，组到期取最早
	d2 := newAggregateTestDispatcher(f, defs, riskUrgent)
	op2 := applyPlan(t, d2, f.actorID, buildPlanInput(f, []int64{riskUrgent}, map[int64]int64{riskUrgent: 1}), "op-agg-up-2")
	second, err := d2.Apply(context.Background(), f.actorID, op2)
	require.NoError(t, err)
	require.Equal(t, "completed", second.Status)
	require.Equal(t, ticketID, second.Groups[0].Ticket.ID)
	require.Equal(t, ItemLinked, second.Groups[0].Items[0].Result)

	require.Equal(t, 1, requireTicketPriority(t, db, ticketID))
	require.WithinDuration(t, early, requireGroupBusinessDue(t, db, ticketID), time.Second)
	receiptPriority, receiptDue = requireReceiptAggregate(t, db, f.actorID, op2.OperationID)
	require.Equal(t, 1, receiptPriority)
	require.WithinDuration(t, early, receiptDue, time.Second)

	// 组事务与历史完整：零并行票、关联 2 条、追加项 CAS 翻转 + dispatch 事件、
	// 票历史只记一次真实变更（created 一次 + updated 一次）
	require.Equal(t, int64(1), countFixtureTickets(t, db, f.typeCode))
	require.Equal(t, int64(2), countTicketLinks(t, db, ticketID))
	require.Equal(t, "remediating", requireRiskStatus(t, db, riskUrgent))
	require.Equal(t, int64(2), requireRiskRevision(t, db, riskUrgent))
	require.Equal(t, int64(1), countRows(t, db,
		`SELECT COUNT(*) FROM risk_events WHERE risk_id = $1 AND event_kind = 'dispatch'`, riskUrgent))
	require.Equal(t, int64(1), countRows(t, db,
		`SELECT COUNT(*) FROM ticket_events WHERE ticket_id = $1 AND action = 'created'`, ticketID))
	require.Equal(t, int64(1), requireUpdatedEventCount(t, db, ticketID))
}

// 更低级别追加：票面保持已存的最高级别（urgent 不被 medium 批拉低），组到期不后移；
// 回执聚合事实保存全部有效关联的最高级别，而非本批计划级别。
func TestApplyAppendKeepsHighestLevelOverLessUrgentBatch(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "aggdown")
	finding := "finding-agg-down"
	early := testDeadline
	late := testDeadline.Add(72 * time.Hour)
	riskUrgent := f.newRisk(t, db, "aggdown", finding, "asset-u", f.ownerA, "urgent", early, "assessed")
	riskMedium := f.newRisk(t, db, "aggdown", finding, "asset-m", f.ownerA, "medium", late, "assessed")
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}

	d := newAggregateTestDispatcher(f, defs, riskUrgent)
	op1 := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{riskUrgent}, map[int64]int64{riskUrgent: 1}), "op-agg-down-1")
	first, err := d.Apply(context.Background(), f.actorID, op1)
	require.NoError(t, err)
	require.Equal(t, "completed", first.Status)
	ticketID := first.Groups[0].Ticket.ID
	require.Equal(t, 1, requireTicketPriority(t, db, ticketID))

	// 追加 medium：票面/组到期/回执聚合都保持 urgent 事实
	d2 := newAggregateTestDispatcher(f, defs, riskMedium)
	op2 := applyPlan(t, d2, f.actorID, buildPlanInput(f, []int64{riskMedium}, map[int64]int64{riskMedium: 1}), "op-agg-down-2")
	second, err := d2.Apply(context.Background(), f.actorID, op2)
	require.NoError(t, err)
	require.Equal(t, "completed", second.Status)
	require.Equal(t, ticketID, second.Groups[0].Ticket.ID)

	require.Equal(t, 1, requireTicketPriority(t, db, ticketID))
	require.WithinDuration(t, early, requireGroupBusinessDue(t, db, ticketID), time.Second)
	receiptPriority, receiptDue := requireReceiptAggregate(t, db, f.actorID, op2.OperationID)
	require.Equal(t, 1, receiptPriority)
	require.WithinDuration(t, early, receiptDue, time.Second)
	// 聚合未变化：不产生票历史噪音
	require.Equal(t, int64(0), requireUpdatedEventCount(t, db, ticketID))

	// 原号重投读原回执：不重放追加、不改写已存聚合
	replay, err := d2.Apply(context.Background(), f.actorID, op2)
	require.NoError(t, err)
	require.Equal(t, "completed", replay.Status)
	require.Equal(t, ticketID, replay.Groups[0].Ticket.ID)
	require.Equal(t, ItemLinked, replay.Groups[0].Items[0].Result)
	require.Equal(t, int64(2), countTicketLinks(t, db, ticketID))
	require.Equal(t, 1, requireTicketPriority(t, db, ticketID))
}

// already_linked 批（本批计划级别低于在办聚合）：回执不得以本批级别改写已存聚合事实，
// 票面保持最高级别；聚合未变化不落票历史。
func TestApplyAlreadyLinkedBatchKeepsTicketAggregate(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "aggkeep")
	finding := "finding-agg-keep"
	early := testDeadline
	late := testDeadline.Add(72 * time.Hour)
	riskUrgent := f.newRisk(t, db, "aggkeep", finding, "asset-u", f.ownerA, "urgent", early, "assessed")
	riskMedium := f.newRisk(t, db, "aggkeep", finding, "asset-m", f.ownerA, "medium", late, "assessed")
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}

	d := newAggregateTestDispatcher(f, defs, riskUrgent, riskMedium)
	op1 := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{riskUrgent, riskMedium},
		map[int64]int64{riskUrgent: 1, riskMedium: 1}), "op-agg-keep-1")
	first, err := d.Apply(context.Background(), f.actorID, op1)
	require.NoError(t, err)
	require.Equal(t, "completed", first.Status)
	ticketID := first.Groups[0].Ticket.ID
	require.Equal(t, 1, requireTicketPriority(t, db, ticketID))

	// 仅重选已挂的 medium（级别 3，低于在办聚合 1）：仍按当前权限给原票核对
	d2 := newAggregateTestDispatcher(f, defs, riskUrgent, riskMedium)
	mediumRevision := requireRiskRevision(t, db, riskMedium)
	op2 := applyPlan(t, d2, f.actorID, buildPlanInput(f, []int64{riskMedium},
		map[int64]int64{riskMedium: mediumRevision}), "op-agg-keep-2")
	second, err := d2.Apply(context.Background(), f.actorID, op2)
	require.NoError(t, err)
	require.Equal(t, "completed", second.Status)
	require.Equal(t, ticketID, second.Groups[0].Ticket.ID)
	require.Equal(t, ItemAlreadyLinked, second.Groups[0].Items[0].Result)

	require.Equal(t, 1, requireTicketPriority(t, db, ticketID))
	require.WithinDuration(t, early, requireGroupBusinessDue(t, db, ticketID), time.Second)
	receiptPriority, receiptDue := requireReceiptAggregate(t, db, f.actorID, op2.OperationID)
	require.Equal(t, 1, receiptPriority)
	require.WithinDuration(t, early, receiptDue, time.Second)
	require.Equal(t, int64(0), requireUpdatedEventCount(t, db, ticketID))
	require.Equal(t, int64(2), countTicketLinks(t, db, ticketID))
}
