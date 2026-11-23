//go:build integration

package riskdispatch

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

var testDeadline = time.Date(2026, 11, 25, 17, 0, 0, 0, time.UTC)

func fmtKey(actorID, riskID int64) string {
	return strconv.FormatInt(actorID, 10) + "|" + strconv.FormatInt(riskID, 10)
}

func buildPlanInput(f dispatchFixture, riskIDs []int64, revisions map[int64]int64) PlanInput {
	input := PlanInput{TargetOrgID: f.targetID, TargetTypeCode: f.typeCode}
	for _, id := range riskIDs {
		input.Items = append(input.Items, RiskExpected{ID: id, Revision: revisions[id]})
	}
	return input
}

func applyPlan(t *testing.T, d *Dispatcher, actorID int64, in PlanInput, operationID string) ApplyInput {
	t.Helper()
	plan, err := d.Plan(context.Background(), actorID, in)
	require.NoError(t, err)
	planID, err := uuid.Parse(plan.PlanID)
	require.NoError(t, err)
	return ApplyInput{OperationID: operationID, PlanID: planID, PlanHash: plan.PlanHash}
}

// 夹具私有计数：tickets 按夹具独占 type_code 圈定，避免跨测试数据污染。
func countFixtureTickets(t *testing.T, db *pgxpool.Pool, typeCode string) int64 {
	t.Helper()
	return countRows(t, db, `SELECT COUNT(*) FROM tickets WHERE type_code = $1`, typeCode)
}

func countTicketLinks(t *testing.T, db *pgxpool.Pool, ticketID int64) int64 {
	t.Helper()
	return countRows(t, db, `
		SELECT COUNT(*) FROM risk_ticket_links l
		JOIN risk_dispatch_groups g ON g.id = l.group_id
		WHERE g.ticket_id = $1 AND l.ended_at IS NULL`, ticketID)
}

func TestApplyDispatchesGroupAtomicallyAndRecordsDepartmentFacts(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "atomic")
	risk1 := f.newRisk(t, db, "atomic", "finding-A", "asset-1", f.ownerA, "high", testDeadline, "assessed")
	risk2 := f.newRisk(t, db, "atomic", "finding-A", "asset-2", f.ownerA, "medium", testDeadline.Add(-24*time.Hour), "assessed")
	authorizer := &stubAuthorizer{allowed: map[string]bool{
		fmtKey(f.actorID, risk1): true, fmtKey(f.actorID, risk2): true,
	}}
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}
	d := newTestDispatcher(db, authorizer, &stubFacts{visible: map[string]bool{
		fmtKey(f.actorID, risk1): true, fmtKey(f.actorID, risk2): true,
	}}, &stubTicketCreator{}, stubTicketViews{visible: map[int64]bool{}, all: true}, defs)

	op := applyPlan(t, d, f.actorID,
		buildPlanInput(f, []int64{risk1, risk2}, map[int64]int64{risk1: 1, risk2: 1}), "op-atomic-1")
	result, err := d.Apply(context.Background(), f.actorID, op)
	require.NoError(t, err)
	require.Equal(t, "completed", result.Status)
	require.Len(t, result.Groups, 1)
	group := result.Groups[0]
	require.Equal(t, "completed", group.Status)
	require.NotEmpty(t, group.GroupOperationID)
	require.NotNil(t, group.Ticket)
	require.Equal(t, f.ownerAOrg, group.OwnerOrgAtDispatch)
	require.Equal(t, f.ownerA, group.OwnerUserID)

	// 票：created_by=owner、最高级别（high=2）、created 历史 actor=真实操作者
	var createdBy, priority int64
	var humanKey string
	require.NoError(t, db.QueryRow(context.Background(), `
		SELECT created_by, priority, human_key FROM tickets WHERE id = $1
	`, group.Ticket.ID).Scan(&createdBy, &priority, &humanKey))
	require.Equal(t, f.ownerA, createdBy)
	require.Equal(t, int64(2), priority)
	require.Equal(t, group.Ticket.Ref, humanKey)
	require.Equal(t, int64(1), countRows(t, db,
		`SELECT COUNT(*) FROM ticket_events WHERE ticket_id = $1 AND action = 'created' AND user_id = $2`,
		group.Ticket.ID, f.actorID))

	// 关联、风险状态与历史同事务落账
	require.Equal(t, int64(2), countTicketLinks(t, db, group.Ticket.ID))
	require.Equal(t, "remediating", requireRiskStatus(t, db, risk1))
	require.Equal(t, int64(2), requireRiskRevision(t, db, risk1))
	require.Equal(t, int64(1), countRows(t, db,
		`SELECT COUNT(*) FROM risk_events WHERE risk_id = $1 AND event_kind = 'dispatch'`, risk1))

	// 组行部门/到期/操作者事实
	var ownerOrg, operator int64
	var due time.Time
	require.NoError(t, db.QueryRow(context.Background(), `
		SELECT COALESCE(owner_org_at_dispatch, 0), operator_user_id, business_due_at
		FROM risk_dispatch_groups WHERE ticket_id = $1
	`, group.Ticket.ID).Scan(&ownerOrg, &operator, &due))
	require.Equal(t, f.ownerAOrg, ownerOrg)
	require.Equal(t, f.actorID, operator)
	require.WithinDuration(t, testDeadline.Add(-24*time.Hour), due, time.Second)

	// 组回执同事务
	require.Equal(t, int64(1), countRows(t, db,
		`SELECT COUNT(*) FROM risk_dispatch_group_receipts WHERE actor_id = $1 AND operation_id = $2`,
		f.actorID, op.OperationID))
	require.Equal(t, int64(1), countFixtureTickets(t, db, f.typeCode))
}

func TestApplyReplaySameOperationReturnsOriginalReceipt(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "replay")
	risk := f.newRisk(t, db, "replay", "finding-R", "asset-1", f.ownerA, "high", testDeadline, "assessed")
	tickets := &stubTicketCreator{}
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}
	newDispatcher := func() *Dispatcher {
		return newTestDispatcher(db, &stubAuthorizer{allowed: map[string]bool{fmtKey(f.actorID, risk): true}},
			&stubFacts{visible: map[string]bool{fmtKey(f.actorID, risk): true}},
			tickets, stubTicketViews{visible: map[int64]bool{}, all: true}, defs)
	}
	d := newDispatcher()
	op := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 1}), "op-replay-1")
	first, err := d.Apply(context.Background(), f.actorID, op)
	require.NoError(t, err)
	require.Equal(t, "completed", first.Status)
	require.NotNil(t, first.Groups[0].Ticket)

	second, err := newDispatcher().Apply(context.Background(), f.actorID, op)
	require.NoError(t, err)
	require.Equal(t, "completed", second.Status)
	require.Equal(t, first.Groups[0].Ticket.ID, second.Groups[0].Ticket.ID)
	require.Equal(t, first.Groups[0].GroupOperationID, second.Groups[0].GroupOperationID)
	require.Equal(t, ItemCreated, second.Groups[0].Items[0].Result)
	// 业务与收据各只有一份
	require.Equal(t, int64(1), countFixtureTickets(t, db, f.typeCode))
	require.Equal(t, int64(1), countTicketLinks(t, db, first.Groups[0].Ticket.ID))
	require.Equal(t, int64(1), countRows(t, db, `SELECT COUNT(*) FROM risk_events WHERE risk_id = $1`, risk))
	require.Equal(t, int64(1), countRows(t, db,
		`SELECT COUNT(*) FROM risk_dispatch_group_receipts WHERE operation_id = $1`, op.OperationID))
	require.Equal(t, 1, tickets.creates)
}

func TestApplyResumesOnlyUnfinishedGroups(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "resume")
	riskA := f.newRisk(t, db, "resume", "finding-A", "asset-a1", f.ownerA, "high", testDeadline, "assessed")
	riskB := f.newRisk(t, db, "resume", "finding-B", "asset-b1", f.ownerB, "high", testDeadline, "assessed")
	allowed := map[string]bool{fmtKey(f.actorID, riskA): true, fmtKey(f.actorID, riskB): true}
	visible := map[string]bool{fmtKey(f.actorID, riskA): true, fmtKey(f.actorID, riskB): true}
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}
	// B 组责任人先停用：首轮 A 成功、B 拒绝
	execMustNoError(t, db, context.Background(), `UPDATE users SET status = 2 WHERE id = $1`, f.ownerB)
	d := newTestDispatcher(db, &stubAuthorizer{allowed: allowed},
		&stubFacts{visible: visible}, &stubTicketCreator{},
		stubTicketViews{visible: map[int64]bool{}, all: true}, defs)

	op := applyPlan(t, d, f.actorID,
		buildPlanInput(f, []int64{riskA, riskB}, map[int64]int64{riskA: 1, riskB: 1}), "op-resume-1")
	first, err := d.Apply(context.Background(), f.actorID, op)
	require.NoError(t, err)
	require.Equal(t, "partial", first.Status)
	require.Len(t, first.Groups, 2)

	ticketA := first.Groups[0].Ticket.ID
	var completedKey string
	for _, group := range first.Groups {
		if group.Status == "completed" {
			completedKey = group.GroupKey
			require.Equal(t, ticketA, group.Ticket.ID)
		} else {
			require.Equal(t, "failed", group.Status)
			for _, item := range group.Items {
				require.Equal(t, ItemDenied, item.Result)
				require.Equal(t, ReasonOwnerUnavailable, item.Reason)
			}
		}
	}
	require.NotEmpty(t, completedKey)
	require.Equal(t, int64(1), countFixtureTickets(t, db, f.typeCode))
	require.Equal(t, "assessed", requireRiskStatus(t, db, riskB))

	// 责任人复职后同号重投：A 读原回执，B 只续未完成组
	execMustNoError(t, db, context.Background(), `UPDATE users SET status = 1 WHERE id = $1`, f.ownerB)
	second, err := d.Apply(context.Background(), f.actorID, op)
	require.NoError(t, err)
	require.Equal(t, "completed", second.Status)
	for _, group := range second.Groups {
		require.Equal(t, "completed", group.Status)
		require.Len(t, group.Items, 1)
		require.Equal(t, ItemCreated, group.Items[0].Result)
		if group.GroupKey == completedKey {
			require.Equal(t, ticketA, group.Ticket.ID) // 原回执原票，未重建
		} else {
			require.NotEqual(t, ticketA, group.Ticket.ID)
		}
	}
	require.Equal(t, int64(2), countFixtureTickets(t, db, f.typeCode))
	require.Equal(t, "remediating", requireRiskStatus(t, db, riskB))
}

func TestApplyAppendChecksRemainingCapacity(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "capacity")
	finding := "finding-C"
	risk1 := f.newRisk(t, db, "capacity", finding, "asset-1", f.ownerA, "high", testDeadline, "assessed")
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}
	newDispatcher := func(risks ...int64) *Dispatcher {
		allowed := map[string]bool{}
		visible := map[string]bool{}
		for _, id := range risks {
			allowed[fmtKey(f.actorID, id)] = true
			visible[fmtKey(f.actorID, id)] = true
		}
		return newTestDispatcher(db, &stubAuthorizer{allowed: allowed}, &stubFacts{visible: visible},
			&stubTicketCreator{}, stubTicketViews{visible: map[int64]bool{}, all: true}, defs)
	}
	d := newDispatcher(risk1)
	op := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{risk1}, map[int64]int64{risk1: 1}), "op-cap-1")
	first, err := d.Apply(context.Background(), f.actorID, op)
	require.NoError(t, err)
	ticketID := first.Groups[0].Ticket.ID
	var groupID int64
	require.NoError(t, db.QueryRow(context.Background(),
		`SELECT id FROM risk_dispatch_groups WHERE ticket_id = $1`, ticketID).Scan(&groupID))

	// 在办组灌到 19/20：再追 2 项整组拒绝（不部分追加、不并行开票）
	risk2 := f.newRisk(t, db, "capacity", finding, "asset-2", f.ownerA, "high", testDeadline, "assessed")
	risk3 := f.newRisk(t, db, "capacity", finding, "asset-3", f.ownerA, "high", testDeadline, "assessed")
	for i := range 18 {
		fill := f.newRisk(t, db, "capacity", finding, fmt.Sprintf("fill-%02d", i), f.ownerA, "low", testDeadline, "remediating")
		execMustNoError(t, db, context.Background(), `
			INSERT INTO risk_ticket_links (risk_id, group_id, owner_at_link) VALUES ($1, $2, $3)
		`, fill, groupID, f.ownerA)
	}
	d2 := newDispatcher(risk2, risk3)
	op2 := applyPlan(t, d2, f.actorID,
		buildPlanInput(f, []int64{risk2, risk3}, map[int64]int64{risk2: 1, risk3: 1}), "op-cap-2")
	result, err := d2.Apply(context.Background(), f.actorID, op2)
	require.NoError(t, err)
	require.Equal(t, "failed", result.Groups[0].Status)
	for _, item := range result.Groups[0].Items {
		require.Equal(t, ItemDenied, item.Result)
		require.Equal(t, ReasonGroupCapacity, item.Reason)
	}
	require.Equal(t, int64(1), countFixtureTickets(t, db, f.typeCode))
	require.Equal(t, int64(19), countRows(t, db,
		`SELECT COUNT(*) FROM risk_ticket_links WHERE group_id = $1 AND ended_at IS NULL`, groupID))

	// 只追 1 项：挂原票，不新建
	d3 := newDispatcher(risk2)
	op3 := applyPlan(t, d3, f.actorID, buildPlanInput(f, []int64{risk2}, map[int64]int64{risk2: 1}), "op-cap-3")
	appended, err := d3.Apply(context.Background(), f.actorID, op3)
	require.NoError(t, err)
	require.Equal(t, "completed", appended.Groups[0].Status)
	require.Equal(t, ticketID, appended.Groups[0].Ticket.ID)
	require.Equal(t, ItemLinked, appended.Groups[0].Items[0].Result)
	require.Equal(t, int64(1), countFixtureTickets(t, db, f.typeCode))
	require.Equal(t, int64(20), countRows(t, db,
		`SELECT COUNT(*) FROM risk_ticket_links WHERE group_id = $1 AND ended_at IS NULL`, groupID))
	// 原组部门事实不被追加改写
	var ownerOrg int64
	require.NoError(t, db.QueryRow(context.Background(),
		`SELECT COALESCE(owner_org_at_dispatch, 0) FROM risk_dispatch_groups WHERE id = $1`,
		groupID).Scan(&ownerOrg))
	require.Equal(t, f.ownerAOrg, ownerOrg)
}

func TestApplyRejectsIncompatibleLiveGroupWithoutParallelTicket(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "drift")
	finding := "finding-D"
	risk := f.newRisk(t, db, "drift", finding, "asset-1", f.ownerA, "high", testDeadline, "assessed")
	fresh := f.newRisk(t, db, "drift", finding, "asset-1b", f.ownerA, "high", testDeadline, "assessed")
	views := stubTicketViews{visible: map[int64]bool{}, all: true}
	defsV1 := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}
	defsV2 := map[string]DefinitionRef{f.typeCode: {Version: 2, Hash: strings.Repeat("b", 64)}}
	newDispatcher := func(defs map[string]DefinitionRef) *Dispatcher {
		return newTestDispatcher(db, &stubAuthorizer{allowed: map[string]bool{
			fmtKey(f.actorID, risk): true, fmtKey(f.actorID, fresh): true,
		}}, &stubFacts{visible: map[string]bool{
			fmtKey(f.actorID, risk): true, fmtKey(f.actorID, fresh): true,
		}}, &stubTicketCreator{}, views, defs)
	}
	d := newDispatcher(defsV1)
	op := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 1}), "op-drift-1")
	first, err := d.Apply(context.Background(), f.actorID, op)
	require.NoError(t, err)
	ticketID := first.Groups[0].Ticket.ID

	// 同漏洞同责任人、不同已发布定义 → 在办组键漂移：conflict，不并票不并行开票
	op2 := applyPlan(t, newDispatcher(defsV2), f.actorID,
		buildPlanInput(f, []int64{fresh}, map[int64]int64{fresh: 1}), "op-drift-2")
	second, err := newDispatcher(defsV2).Apply(context.Background(), f.actorID, op2)
	require.NoError(t, err)
	require.Equal(t, "failed", second.Groups[0].Status)
	for _, item := range second.Groups[0].Items {
		require.Equal(t, ItemConflict, item.Result)
		require.Equal(t, ReasonGroupScopeDrift, item.Reason)
	}
	require.Equal(t, int64(1), countFixtureTickets(t, db, f.typeCode))
	require.Equal(t, int64(1), countTicketLinks(t, db, ticketID))
}

func TestApplyHidesTicketRefWithoutTicketView(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "mask")
	finding := "finding-E"
	risk := f.newRisk(t, db, "mask", finding, "asset-1", f.ownerA, "high", testDeadline, "assessed")
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}
	newDispatcher := func(all bool) *Dispatcher {
		return newTestDispatcher(db, &stubAuthorizer{allowed: map[string]bool{fmtKey(f.actorID, risk): true}},
			&stubFacts{visible: map[string]bool{fmtKey(f.actorID, risk): true}},
			&stubTicketCreator{}, stubTicketViews{visible: map[int64]bool{}, all: all}, defs)
	}
	d := newDispatcher(true)
	op := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 1}), "op-mask-1")
	result, err := d.Apply(context.Background(), f.actorID, op)
	require.NoError(t, err)
	require.NotNil(t, result.Groups[0].Ticket) // 有票读权：给原票引用

	// 同一风险新派单操作且无票读权：already_linked、引用整体省略（不泄露存在性）
	again := newDispatcher(false)
	plan2, err := again.Plan(context.Background(), f.actorID,
		buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 2})) // 派单后 revision=2
	require.NoError(t, err)
	require.NotEmpty(t, plan2.Groups)
	require.Nil(t, plan2.Groups[0].Ticket) // 无读权：预览同样不给引用
	op2 := applyPlan(t, again, f.actorID,
		buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 2}), "op-mask-2")
	masked, err := again.Apply(context.Background(), f.actorID, op2)
	require.NoError(t, err)
	require.Equal(t, "completed", masked.Groups[0].Status)
	require.Nil(t, masked.Groups[0].Ticket)
	require.Equal(t, ItemAlreadyLinked, masked.Groups[0].Items[0].Result)
	// 新操作的核查结果持久保存，票和关联保持原数量，读取仍按当前票权遮蔽引用。
	require.Equal(t, int64(1), countFixtureTickets(t, db, f.typeCode))
	require.Equal(t, int64(1), countTicketLinks(t, db, result.Groups[0].Ticket.ID))
	readback, err := again.ReadOperation(context.Background(), f.actorID, op2.OperationID)
	require.NoError(t, err)
	require.Equal(t, masked, readback)
	require.Nil(t, readback.Groups[0].Items[0].Ticket)
}

func TestApplyConflictsOnRevisionDrift(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "rev")
	risk := f.newRisk(t, db, "rev", "finding-F", "asset-1", f.ownerA, "high", testDeadline, "assessed")
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}
	d := newTestDispatcher(db, &stubAuthorizer{allowed: map[string]bool{fmtKey(f.actorID, risk): true}},
		&stubFacts{visible: map[string]bool{fmtKey(f.actorID, risk): true}}, &stubTicketCreator{},
		stubTicketViews{visible: map[int64]bool{}, all: true}, defs)

	op := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 1}), "op-rev-1")
	// 预览后风险被他人推进（revision 漂移）
	execMustNoError(t, db, context.Background(),
		`UPDATE risk_items SET revision = revision + 1 WHERE id = $1`, risk)
	result, err := d.Apply(context.Background(), f.actorID, op)
	require.NoError(t, err)
	require.Equal(t, "pending", result.Status)
	require.Equal(t, "failed", result.Groups[0].Status)
	require.Equal(t, ItemConflict, result.Groups[0].Items[0].Result)
	require.Equal(t, ReasonRevisionChanged, result.Groups[0].Items[0].Reason)
	require.Equal(t, int64(0), countFixtureTickets(t, db, f.typeCode))
}

func TestApplyConflictsWhenPlanExpired(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "expired")
	risk := f.newRisk(t, db, "expired", "finding-G", "asset-1", f.ownerA, "high", testDeadline, "assessed")
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}
	d := newTestDispatcher(db, &stubAuthorizer{allowed: map[string]bool{fmtKey(f.actorID, risk): true}},
		&stubFacts{visible: map[string]bool{fmtKey(f.actorID, risk): true}},
		&stubTicketCreator{}, stubTicketViews{visible: map[int64]bool{}, all: true}, defs)

	op := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 1}), "op-expired-1")
	execMustNoError(t, db, context.Background(),
		`UPDATE risk_dispatch_plans SET created_at = now() - interval '2 minutes', expires_at = now() - interval '1 minute' WHERE id = $1`, op.PlanID)
	result, err := d.Apply(context.Background(), f.actorID, op)
	require.NoError(t, err)
	require.Equal(t, "failed", result.Groups[0].Status)
	require.Equal(t, ItemConflict, result.Groups[0].Items[0].Result)
	require.Equal(t, ReasonPlanExpired, result.Groups[0].Items[0].Reason)
	require.Equal(t, int64(0), countFixtureTickets(t, db, f.typeCode))
}

func TestApplyDeniesWholeGroupWhenInTxAuthorizationRevoked(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "revoke")
	risk := f.newRisk(t, db, "revoke", "finding-H", "asset-1", f.ownerA, "high", testDeadline, "assessed")
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}
	// 预检放行（第 1 次），守卫事务内复验拒绝（第 2 次）——预览后撤权变体
	// Plan and Apply admission succeed; revoke at the group commit recheck.
	authorizer := &stubAuthorizer{allowed: map[string]bool{fmtKey(f.actorID, risk): true}, denyCall: 3}
	d := newTestDispatcher(db, authorizer, &stubFacts{visible: map[string]bool{fmtKey(f.actorID, risk): true}},
		&stubTicketCreator{}, stubTicketViews{visible: map[int64]bool{}, all: true}, defs)

	op := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 1}), "op-revoke-1")
	result, err := d.Apply(context.Background(), f.actorID, op)
	require.NoError(t, err)
	require.Equal(t, "failed", result.Groups[0].Status)
	require.Equal(t, ItemDenied, result.Groups[0].Items[0].Result)
	require.Equal(t, ReasonDispatchForbidden, result.Groups[0].Items[0].Reason)
	require.Equal(t, int64(0), countFixtureTickets(t, db, f.typeCode))
	require.Equal(t, 3, authorizer.calls)
}

func TestApplySameOperationDifferentPlanConflicts(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "samenum")
	risk := f.newRisk(t, db, "samenum", "finding-I", "asset-1", f.ownerA, "high", testDeadline, "assessed")
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}
	d := newTestDispatcher(db, &stubAuthorizer{allowed: map[string]bool{fmtKey(f.actorID, risk): true}},
		&stubFacts{visible: map[string]bool{fmtKey(f.actorID, risk): true}},
		&stubTicketCreator{}, stubTicketViews{visible: map[int64]bool{}, all: true}, defs)

	op := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 1}), "op-same-1")
	_, err := d.Apply(context.Background(), f.actorID, op)
	require.NoError(t, err)
	op.PlanHash = strings.Repeat("f", 64)
	_, err = d.Apply(context.Background(), f.actorID, op)
	require.ErrorIs(t, err, errcode.ErrConcurrentModification)
}

func TestApplyConcurrentSameGroupCreatesSingleTicket(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "race")
	risk := f.newRisk(t, db, "race", "finding-J", "asset-1", f.ownerA, "high", testDeadline, "assessed")
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}
	newDispatcher := func() *Dispatcher {
		return newTestDispatcher(db, &stubAuthorizer{allowed: map[string]bool{fmtKey(f.actorID, risk): true}},
			&stubFacts{visible: map[string]bool{fmtKey(f.actorID, risk): true}},
			&stubTicketCreator{}, stubTicketViews{visible: map[int64]bool{}, all: true}, defs)
	}
	op1 := applyPlan(t, newDispatcher(), f.actorID, buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 1}), "op-race-1")
	op2 := applyPlan(t, newDispatcher(), f.actorID, buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 1}), "op-race-2")

	var wg sync.WaitGroup
	results := make([]ApplyResult, 2)
	errs := make([]error, 2)
	for i, op := range []ApplyInput{op1, op2} {
		wg.Add(1)
		go func(index int, input ApplyInput) {
			defer wg.Done()
			results[index], errs[index] = newDispatcher().Apply(context.Background(), f.actorID, input)
		}(i, op)
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.Equal(t, int64(1), countFixtureTickets(t, db, f.typeCode))
	require.Equal(t, int64(1), countRows(t, db,
		`SELECT COUNT(*) FROM risk_ticket_links WHERE ended_at IS NULL AND risk_id = $1`, risk))
	require.Equal(t, "remediating", requireRiskStatus(t, db, risk))
	// 恰好一组 completed，另一组不再新增效果（revision 漂移 → conflict）
	created := 0
	for _, result := range results {
		require.Len(t, result.Groups, 1)
		if result.Groups[0].Status == "completed" {
			created++
		} else {
			require.Equal(t, "failed", result.Groups[0].Status)
			require.Equal(t, ReasonRevisionChanged, result.Groups[0].Items[0].Reason)
		}
	}
	require.Equal(t, 1, created)
}

func TestPlanOmitsInvisibleRisksAndReportsBlockers(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "plan")
	riskVisible := f.newRisk(t, db, "plan", "finding-K", "asset-1", f.ownerA, "high", testDeadline, "assessed")
	riskNoOwner := f.newRisk(t, db, "plan", "finding-K", "asset-2", 0, "high", testDeadline, "assessed")
	riskInvisible := f.newRisk(t, db, "plan", "finding-K", "asset-3", f.ownerA, "high", testDeadline, "assessed")
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}
	d := newTestDispatcher(db, &stubAuthorizer{allowed: map[string]bool{
		fmtKey(f.actorID, riskVisible): true, fmtKey(f.actorID, riskNoOwner): true,
		fmtKey(f.actorID, riskInvisible): true,
	}}, &stubFacts{visible: map[string]bool{
		fmtKey(f.actorID, riskVisible): true, fmtKey(f.actorID, riskNoOwner): true,
	}}, &stubTicketCreator{}, stubTicketViews{visible: map[int64]bool{}, all: false}, defs)

	result, err := d.Plan(context.Background(), f.actorID, buildPlanInput(f,
		[]int64{riskVisible, riskNoOwner, riskInvisible},
		map[int64]int64{riskVisible: 1, riskNoOwner: 1, riskInvisible: 1}))
	require.NoError(t, err)
	// 可见 + 有 owner → 一组；可见 + 无 owner → 逐项 missing_personal_owner；不可见 → 不给原因不泄露
	require.Len(t, result.Groups, 1)
	require.Equal(t, f.ownerA, result.Groups[0].OwnerUserID)
	require.Len(t, result.Groups[0].Items, 1)
	require.Equal(t, riskVisible, result.Groups[0].Items[0].RiskID)
	require.Len(t, result.Blocked, 1)
	require.Equal(t, riskNoOwner, result.Blocked[0].RiskID)
	require.Equal(t, string(MissingPersonalOwner), result.Blocked[0].Reason)
}

func TestApplyRejectsMalformedInput(t *testing.T) {
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "badinput")
	risk := f.newRisk(t, db, "badinput", "finding-L", "asset-1", f.ownerA, "high", testDeadline, "assessed")
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}
	d := newTestDispatcher(db, &stubAuthorizer{allowed: map[string]bool{fmtKey(f.actorID, risk): true}},
		&stubFacts{visible: map[string]bool{fmtKey(f.actorID, risk): true}},
		&stubTicketCreator{}, stubTicketViews{visible: map[int64]bool{}, all: true}, defs)

	_, err := d.Apply(context.Background(), f.actorID,
		ApplyInput{OperationID: "op-x", PlanID: uuid.New(), PlanHash: "z"})
	require.ErrorIs(t, err, errcode.ErrNotFound)
	_, err = d.Plan(context.Background(), f.actorID,
		PlanInput{TargetOrgID: f.targetID, TargetTypeCode: f.typeCode})
	require.ErrorIs(t, err, errcode.ErrInvalidParams)
}
