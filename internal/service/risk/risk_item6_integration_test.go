//go:build integration

package risk_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
)

// dDayAt 合成业务日 17:00（UTC；详设 §14：R1 高/D 日 17:00、R2 中/D+1 日 17:00）。
func dDayAt(t *testing.T, dayOffset int) time.Time {
	t.Helper()
	base := time.Date(2026, 11, 22, 17, 0, 0, 0, time.UTC)
	return base.AddDate(0, 0, dayOffset)
}

func newService() *risk.Service {
	return risk.NewService(testPool, nil)
}

func mustCreate(t *testing.T, svc *risk.Service, op, title string, orgID int64, owner *int64) *model.RiskMutationResult {
	t.Helper()
	affected := orgDID // 副锚可见性用例统一挂受影响部门 D
	req := model.RiskCreateRequest{
		OperationID: op, Title: title, Description: "合成风险-" + title,
		OrgID: orgID, AffectedOrgID: &affected, OwnerUserID: owner, ExpectedRevision: "0",
	}
	res, err := svc.Create(context.Background(), req, userM)
	require.NoError(t, err)
	return res
}

func itemIDByOperation(t *testing.T, opID string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT result_risk_id FROM risk_operations WHERE operation_id=$1 AND command='create' ORDER BY created_at LIMIT 1`,
		opID).Scan(&id))
	return id
}

// ── 场景 A：人工登记、授权列表/详情、责任确认 ──────────────────────────

func TestRiskCreate_ManualEntryAuthorizedReadsOwnerGate(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	suffix := uniqueSuffix()

	res := mustCreate(t, svc, "op-create-"+suffix, "R1-"+suffix, orgGID, &userU)
	require.NotZero(t, res.Risk.ID)
	require.Equal(t, int64(1), res.Risk.Revision)
	require.Equal(t, "new", res.Status)
	require.Contains(t, res.Risk.HumanKey, "RISK-")
	require.NotEmpty(t, res.Receipt.OperationID)

	// 同号同参幂等：返回原结果，不新增风险行（与原请求逐字段一致）。
	affected := orgDID
	replay, err := svc.Create(ctx, model.RiskCreateRequest{
		OperationID: "op-create-" + suffix, Title: "R1-" + suffix, Description: "合成风险-R1-" + suffix,
		OrgID: orgGID, AffectedOrgID: &affected, OwnerUserID: &userU, ExpectedRevision: "0",
	}, userM)
	require.NoError(t, err)
	require.Equal(t, res.Risk.ID, replay.Risk.ID)
	var itemCount int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT count(*) FROM risk_items WHERE title=$1`, "R1-"+suffix).Scan(&itemCount))
	require.Equal(t, 1, itemCount)

	// 同号异参 → 409。
	_, err = svc.Create(ctx, model.RiskCreateRequest{
		OperationID: "op-create-" + suffix, Title: "R1-改动", Description: "异参",
		OrgID: orgGID, ExpectedRevision: "0",
	}, userM)
	require.Error(t, err)
	require.Equal(t, 94003, errCodeOf(t, err))

	// L3：非管理者、非 owner 不能在 G 登记。
	_, err = svc.Create(ctx, model.RiskCreateRequest{
		OperationID: "op-create-v-" + suffix, Title: "V 越权", OrgID: orgGID, ExpectedRevision: "0",
	}, userV)
	require.ErrorIs(t, err, errcode.ErrNoPermission)

	// 责任待确认：不落 owner 的登记保持待确认，派单准备被阻断。
	pending := mustCreate(t, svc, "op-create-pending-"+suffix, "R-pending-"+suffix, orgGID, nil)
	detail, err := svc.Detail(ctx, pending.Risk.ID, userM)
	require.NoError(t, err)
	require.Contains(t, detail.View.DispatchBlockers, "owner_unconfirmed")
	// 责任确认（主锚管理者 assign）后解除阻断。
	assigned, err := svc.Assign(ctx, model.RiskAssignRequest{
		OperationID: "op-assign-" + suffix, ID: pending.Risk.ID,
		ExpectedRevision: "1", OwnerUserID: userU, Reason: "资产台账核实",
	}, userM)
	require.NoError(t, err)
	require.Equal(t, int64(2), assigned.Risk.Revision)
	detail, err = svc.Detail(ctx, pending.Risk.ID, userM)
	require.NoError(t, err)
	require.NotContains(t, detail.View.DispatchBlockers, "owner_unconfirmed")

	// 授权读取：主锚、副锚可见；无范围成员不可见且与不存在不可区分。
	for _, actor := range []int64{userM, userV} {
		list, err := svc.List(ctx, model.RiskListQuery{Page: 1, PageSize: 50}, actor)
		require.NoError(t, err)
		require.True(t, containsRisk(list.Items, res.Risk.ID), "actor %d 应可见", actor)
	}
	empty, err := svc.List(ctx, model.RiskListQuery{Page: 1, PageSize: 50}, userN)
	require.NoError(t, err)
	require.False(t, containsRisk(empty.Items, res.Risk.ID))
	_, err = svc.Detail(ctx, res.Risk.ID, userN)
	require.Equal(t, 94001, errCodeOf(t, err))
	_, err = svc.Detail(ctx, 987654321, userN)
	require.Equal(t, 94001, errCodeOf(t, err))

	// 核实记录可读：created/assign 事件在历史中且带操作者。
	history, err := svc.History(ctx, pending.Risk.ID, 1, 50, userM)
	require.NoError(t, err)
	kinds := map[string]bool{}
	for _, ev := range history.Items {
		kinds[ev.EventKind] = true
	}
	require.True(t, kinds["created"])
	require.True(t, kinds["owner_assigned"])
}

// ── 场景 B：核实/评估、显式版本 CAS 与 durable 操作收据 ─────────────────

func TestRiskAssess_CASReceiptAndQualification(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	suffix := uniqueSuffix()
	r := mustCreate(t, svc, "op-as-create-"+suffix, "R2-"+suffix, orgGID, &userU)

	deadline := dDayAt(t, 0)
	assessed, err := svc.Assess(ctx, model.RiskAssessRequest{
		OperationID: "op-as-" + suffix, ID: r.Risk.ID, ExpectedRevision: "1",
		Level: "high", AssessmentNote: "核实依据：合成扫描结论+资产归属", Treatment: "mitigate",
		Deadline: &deadline,
	}, userM)
	require.NoError(t, err)
	require.Equal(t, "assessed", assessed.Status)
	require.Equal(t, int64(2), assessed.Risk.Revision)
	require.NotNil(t, assessed.Level)
	require.Equal(t, "high", *assessed.Level)
	require.NotNil(t, assessed.Deadline)
	require.True(t, deadline.Equal(*assessed.Deadline))
	require.NotNil(t, assessed.InitDeadline)
	require.True(t, deadline.Equal(*assessed.InitDeadline))

	// 陈旧 revision → 409，无业务变化。
	_, err = svc.Assess(ctx, model.RiskAssessRequest{
		OperationID: "op-as-stale-" + suffix, ID: r.Risk.ID, ExpectedRevision: "1",
		Level: "medium", Treatment: "mitigate",
	}, userM)
	require.ErrorIs(t, err, errcode.ErrConcurrentModification)
	var rev int64
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT revision FROM risk_items WHERE id=$1`, r.Risk.ID).Scan(&rev))
	require.Equal(t, int64(2), rev)

	// 同号同参 → 原收据；同号异参 → 409。
	replay, err := svc.Assess(ctx, model.RiskAssessRequest{
		OperationID: "op-as-" + suffix, ID: r.Risk.ID, ExpectedRevision: "1",
		Level: "high", AssessmentNote: "核实依据：合成扫描结论+资产归属", Treatment: "mitigate",
		Deadline: &deadline,
	}, userM)
	require.NoError(t, err)
	require.Equal(t, int64(2), replay.Risk.Revision)
	require.Equal(t, assessed.Receipt.OperationID, replay.Receipt.OperationID)
	_, err = svc.Assess(ctx, model.RiskAssessRequest{
		OperationID: "op-as-" + suffix, ID: r.Risk.ID, ExpectedRevision: "1",
		Level: "low", Treatment: "mitigate",
	}, userM)
	require.Equal(t, 94003, errCodeOf(t, err))

	// 非法 level → 400。
	_, err = svc.Assess(ctx, model.RiskAssessRequest{
		OperationID: "op-as-bad-" + suffix, ID: r.Risk.ID, ExpectedRevision: "2",
		Level: "critical", Treatment: "mitigate",
	}, userM)
	require.ErrorIs(t, err, errcode.ErrInvalidParams)

	// L3：owner 本人具 manage 可评估自己的风险；owner 不能评估他人风险；副锚只读不能评估。
	own, err := svc.Assess(ctx, model.RiskAssessRequest{
		OperationID: "op-as-owner-" + suffix, ID: r.Risk.ID, ExpectedRevision: "2",
		Level: "high", Treatment: "mitigate",
	}, userU)
	require.NoError(t, err)
	require.Equal(t, int64(3), own.Risk.Revision)
	rOther := mustCreate(t, svc, "op-as-create2-"+suffix, "R3-"+suffix, orgGID, &userM)
	_, err = svc.Assess(ctx, model.RiskAssessRequest{
		OperationID: "op-as-u-other-" + suffix, ID: rOther.Risk.ID, ExpectedRevision: "1",
		Level: "low", Treatment: "mitigate",
	}, userU)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	// 副锚只读成员 V：既非管理者也非 owner——评估他人风险被拒。
	rV := mustCreate(t, svc, "op-as-create3-"+suffix, "R4-"+suffix, orgGID, &userM)
	_, err = svc.Assess(ctx, model.RiskAssessRequest{
		OperationID: "op-as-v-" + suffix, ID: rV.Risk.ID, ExpectedRevision: "1",
		Level: "low", Treatment: "mitigate",
	}, userV)
	require.ErrorIs(t, err, errcode.ErrNoPermission)

	// 非法状态：已关闭风险不再接受评估。
	rClosed := mustCreate(t, svc, "op-as-create4-"+suffix, "R5-"+suffix, orgGID, nil)
	_, err = testPool.Exec(ctx, `UPDATE risk_items SET status='closed', closed_at=NOW() WHERE id=$1`, rClosed.Risk.ID)
	require.NoError(t, err)
	_, err = svc.Assess(ctx, model.RiskAssessRequest{
		OperationID: "op-as-closed-" + suffix, ID: rClosed.Risk.ID, ExpectedRevision: "1",
		Level: "low", Treatment: "mitigate",
	}, userM)
	require.Equal(t, 94002, errCodeOf(t, err))

	// 核实记录（含依据说明）写入历史——首次评估事件携带依据说明。
	history, err := svc.History(ctx, r.Risk.ID, 1, 50, userM)
	require.NoError(t, err)
	found := false
	for _, ev := range history.Items {
		if ev.EventKind == "assessed" {
			raw, _ := json.Marshal(ev.Payload)
			if strings.Contains(string(raw), "核实依据") {
				found = true
			}
		}
	}
	require.True(t, found)
}

// ── 场景 C：合成来源上报——重复只应用一次、缺失/过期/超时（依赖故障）可区别 ──

func baseObservation(suffix, eventID, seq string) model.FindingObservationV1 {
	sev := model.RiskSourceSeverity{Scheme: "provider-five-level", Value: "5"}
	return model.FindingObservationV1{
		SourceRef: sourceRef, SourceItemKey: "asset-01:vuln-01:443",
		EventID: eventID + "-" + suffix, SourceSequence: seq,
		ObservedAt: time.Date(2026, 11, 22, 3, 0, 0, 0, time.UTC),
		AssetRef:   &model.RiskAssetRef{Provider: "mock-cmdb", ExternalID: "asset-01"},
		FindingKey: "vuln-01", SourceSeverity: &sev,
		MappingRef: sourceMapping, Attributes: json.RawMessage(`{"service":"https"}`),
	}
}

func TestRiskReport_SyntheticSourceDedupStaleQuarantine(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	suffix := uniqueSuffix()

	first, err := svc.Report(ctx, model.RiskReportRequest{
		Observations: []model.FindingObservationV1{baseObservation(suffix, "evt-42", "42")}}, userM)
	require.NoError(t, err)
	item := first.Items[0]
	require.Equal(t, "accepted", item.Result)
	require.Equal(t, "applied", item.Processing)
	require.NotNil(t, item.RiskRef)
	require.NotEmpty(t, item.IngestID)

	// 稳定漏洞键与资产键身份保留。
	detail, err := svc.Detail(ctx, item.RiskRef.ID, userM)
	require.NoError(t, err)
	require.Equal(t, "vuln-01", detail.View.FindingKey)
	require.NotNil(t, detail.View.AssetRef)
	require.Equal(t, "mock-cmdb", detail.View.AssetRef.Provider)
	require.Equal(t, "asset-01", detail.View.AssetRef.ExternalID)
	require.NotNil(t, detail.View.SourceSeverity)
	require.Equal(t, "provider-five-level", detail.View.SourceSeverity.Scheme)

	// 同 event 同内容重报 → duplicate，风险不新增。
	replay, err := svc.Report(ctx, model.RiskReportRequest{
		Observations: []model.FindingObservationV1{baseObservation(suffix, "evt-42", "42")}}, userM)
	require.NoError(t, err)
	require.Equal(t, "duplicate", replay.Items[0].Result)
	var riskCount int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM risk_items ri
JOIN risk_families f ON f.id=ri.family_id WHERE f.fingerprint_scope=$1`, sourceRef).Scan(&riskCount))
	require.Equal(t, 1, riskCount)

	// 同 event 异内容 → quarantined + 冲突落库；重投同冲突返回原引用，不重复落行。
	conflict := baseObservation(suffix, "evt-42", "42")
	conflict.SourceSeverity = &model.RiskSourceSeverity{Scheme: "provider-five-level", Value: "4"}
	q1, err := svc.Report(ctx, model.RiskReportRequest{Observations: []model.FindingObservationV1{conflict}}, userM)
	require.NoError(t, err)
	require.Equal(t, "quarantined", q1.Items[0].Result)
	require.NotEmpty(t, q1.Items[0].SafeReason)
	q2, err := svc.Report(ctx, model.RiskReportRequest{Observations: []model.FindingObservationV1{conflict}}, userM)
	require.NoError(t, err)
	require.Equal(t, q1.Items[0].SafeReason, q2.Items[0].SafeReason)
	var conflictCount int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT count(*) FROM risk_ingest_conflicts WHERE event_id=$1`, "evt-42-"+suffix).Scan(&conflictCount))
	require.Equal(t, 1, conflictCount)

	// 旧序号 → stale：保留收据、不覆盖镜像、不建新风险。
	stale, err := svc.Report(ctx, model.RiskReportRequest{
		Observations: []model.FindingObservationV1{baseObservation(suffix, "evt-41", "41")}}, userM)
	require.NoError(t, err)
	require.Equal(t, "stale", stale.Items[0].Result)
	var latestSeq string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT latest_sequence FROM risk_source_records WHERE source_ref=$1 AND source_item_key='asset-01:vuln-01:443'`,
		sourceRef).Scan(&latestSeq))
	require.Equal(t, "42", latestSeq)

	// 等序异内容 → quarantined。
	equal := baseObservation(suffix, "evt-eq", "42")
	equal.SourceSeverity = &model.RiskSourceSeverity{Scheme: "provider-five-level", Value: "3"}
	eqRes, err := svc.Report(ctx, model.RiskReportRequest{Observations: []model.FindingObservationV1{equal}}, userM)
	require.NoError(t, err)
	require.Equal(t, "quarantined", eqRes.Items[0].Result)

	// 新事件更大序号 → 应用到同一条风险，不新建周期。
	advance, err := svc.Report(ctx, model.RiskReportRequest{
		Observations: []model.FindingObservationV1{baseObservation(suffix, "evt-43", "43")}}, userM)
	require.NoError(t, err)
	require.Equal(t, "accepted", advance.Items[0].Result)
	require.NotNil(t, advance.Items[0].RiskRef)
	require.Equal(t, item.RiskRef.ID, advance.Items[0].RiskRef.ID)
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT latest_sequence FROM risk_source_records WHERE source_ref=$1 AND source_item_key='asset-01:vuln-01:443'`,
		sourceRef).Scan(&latestSeq))
	require.Equal(t, "43", latestSeq)
	// 核实依据累计：观察事件入历史且风险 revision 不因观察推进（业务变更才推进）。
	history, err := svc.History(ctx, item.RiskRef.ID, 1, 50, userM)
	require.NoError(t, err)
	observed := 0
	for _, ev := range history.Items {
		if ev.EventKind == "source_observed" {
			observed++
		}
	}
	require.GreaterOrEqual(t, observed, 1)

	// 有序源缺序号 → held（缺失可区别），镜像不动。
	noSeq := baseObservation(suffix, "evt-noseq", "")
	noSeq.SourceSequence = ""
	held, err := svc.Report(ctx, model.RiskReportRequest{Observations: []model.FindingObservationV1{noSeq}}, userM)
	require.NoError(t, err)
	require.Equal(t, "held", held.Items[0].Result)
	require.NotEmpty(t, held.Items[0].SafeReason)
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT latest_sequence FROM risk_source_records WHERE source_ref=$1 AND source_item_key='asset-01:vuln-01:443'`,
		sourceRef).Scan(&latestSeq))
	require.Equal(t, "43", latestSeq)

	// mapping 不一致 → held。
	badMap := baseObservation(suffix, "evt-map", "44")
	badMap.MappingRef = "mapping:v2"
	mapRes, err := svc.Report(ctx, model.RiskReportRequest{Observations: []model.FindingObservationV1{badMap}}, userM)
	require.NoError(t, err)
	require.Equal(t, "held", mapRes.Items[0].Result)

	// 来源超授权范围 → held（范围可区别），零风险效果。
	_, err = testPool.Exec(ctx, `UPDATE risk_sources SET default_org_id=$1 WHERE source_ref=$2`, orgDID, sourceRef)
	require.NoError(t, err)
	outScope := baseObservation(suffix, "evt-scope", "45")
	scopeRes, err := svc.Report(ctx, model.RiskReportRequest{Observations: []model.FindingObservationV1{outScope}}, userM)
	require.NoError(t, err)
	require.Equal(t, "held", scopeRes.Items[0].Result)
	require.Contains(t, scopeRes.Items[0].SafeReason, "org")
	_, err = testPool.Exec(ctx, `UPDATE risk_sources SET default_org_id=$1 WHERE source_ref=$2`, orgGID, sourceRef)
	require.NoError(t, err)

	// 未登记来源 → 403，零接入效果。
	unknown := baseObservation(suffix, "evt-unknown", "1")
	unknown.SourceRef = "not-registered"
	_, err = svc.Report(ctx, model.RiskReportRequest{Observations: []model.FindingObservationV1{unknown}}, userM)
	require.Equal(t, 94004, errCodeOf(t, err))
	var obsCount int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT count(*) FROM risk_observations WHERE event_id=$1`, "evt-unknown-"+suffix).Scan(&obsCount))
	require.Equal(t, 0, obsCount)

	// 来源停用 → 403。
	_, err = testPool.Exec(ctx, `UPDATE risk_sources SET enabled=false WHERE source_ref=$1`, sourceRef)
	require.NoError(t, err)
	disabled := baseObservation(suffix, "evt-disabled", "46")
	_, err = svc.Report(ctx, model.RiskReportRequest{Observations: []model.FindingObservationV1{disabled}}, userM)
	require.Equal(t, 94004, errCodeOf(t, err))
	_, err = testPool.Exec(ctx, `UPDATE risk_sources SET enabled=true WHERE source_ref=$1`, sourceRef)
	require.NoError(t, err)

	// 批次：两条合法观察各建风险；缺字段整批 400 零效果；超上限 413。
	b1 := baseObservation(suffix, "evt-b1", "101")
	b1.SourceItemKey = "asset-02:vuln-01:443"
	b1.AssetRef = &model.RiskAssetRef{Provider: "mock-cmdb", ExternalID: "asset-02"}
	b2 := baseObservation(suffix, "evt-b2", "102")
	b2.SourceItemKey = "asset-03:vuln-01:443"
	b2.AssetRef = &model.RiskAssetRef{Provider: "mock-cmdb", ExternalID: "asset-03"}
	batch, err := svc.Report(ctx, model.RiskReportRequest{BatchID: "b-" + suffix, Observations: []model.FindingObservationV1{b1, b2}}, userM)
	require.NoError(t, err)
	require.Len(t, batch.Items, 2)
	require.Equal(t, "accepted", batch.Items[0].Result)
	require.NotEqual(t, batch.Items[0].RiskRef.ID, batch.Items[1].RiskRef.ID)

	badShape := b1
	badShape.EventID = ""
	_, err = svc.Report(ctx, model.RiskReportRequest{Observations: []model.FindingObservationV1{b1, badShape}}, userM)
	require.ErrorIs(t, err, errcode.ErrInvalidParams)
	var b1Obs int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT count(*) FROM risk_observations WHERE event_id=$1`, "evt-b1-"+suffix).Scan(&b1Obs))
	require.Equal(t, 1, b1Obs) // 整批拒绝：原 b1 仍只有首报一条

	big := make([]model.FindingObservationV1, 101)
	_, err = svc.Report(ctx, model.RiskReportRequest{Observations: big}, userM)
	require.Equal(t, 94006, errCodeOf(t, err))

	// 回执受权查询：主锚可查；无范围成员与不存在同判 404。
	receipt, err := svc.IngestReceipt(ctx, first.Items[0].IngestID, userM)
	require.NoError(t, err)
	require.Equal(t, "accepted", receipt.Result)
	require.NotNil(t, receipt.RiskRef)
	_, err = svc.IngestReceipt(ctx, first.Items[0].IngestID, userN)
	require.Equal(t, 94001, errCodeOf(t, err))
	_, err = svc.IngestReceipt(ctx, "missing-ingest", userM)
	require.Equal(t, 94001, errCodeOf(t, err))
}

// errCodeOf 提取业务错误码（断言辅助）。
func errCodeOf(t *testing.T, err error) int {
	t.Helper()
	var biz *errcode.Error
	require.ErrorAs(t, err, &biz)
	return biz.Code
}

func containsRisk(items []model.RiskView, id int64) bool {
	for _, v := range items {
		if v.Risk.ID == id {
			return true
		}
	}
	return false
}
