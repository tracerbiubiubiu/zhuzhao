//go:build integration

package risk_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
)

// B2 第 6–7 项 P1 修复回归（2026-11-23）：
// ① 人工登记携带稳定漏洞/资产身份（finding_key + asset_ref 成对，后兼容），
//    核实（assess 用现有 assessment_note+revision 留证）后可形成派单依据；
// ② 来源匹配证据纳入派单事实——DispatchBasis 为现势有效依据版本，空=不可派；
//    合成来源撤回（withdrawn/deleted）即失效，重报恢复；
// ③ 读/锁路径取依据不对来源镜像表加锁，避免 report/source→risk 锁序反转。
// 全部为合成数据（详设 §13/§14 命名），不指向任何真实组织/人员/路径。

// mustCreateIdentity 人工登记（携带成对 finding_key/asset_ref 身份）。
func mustCreateIdentity(t *testing.T, svc *risk.Service, op, title string, orgID int64, owner *int64,
	findingKey string, ref *model.RiskAssetRef) *model.RiskMutationResult {
	t.Helper()
	affected := orgDID
	res, err := svc.Create(context.Background(), model.RiskCreateRequest{
		OperationID: op, Title: title, Description: "合成风险-" + title,
		OrgID: orgID, AffectedOrgID: &affected, OwnerUserID: owner, ExpectedRevision: "0",
		FindingKey: findingKey, AssetRef: ref,
	}, userM)
	require.NoError(t, err)
	return res
}

func assessRisk(t *testing.T, svc *risk.Service, op string, id int64, expected string, level, note string) *model.RiskMutationResult {
	t.Helper()
	deadline := dDayAt(t, 0)
	res, err := svc.Assess(context.Background(), model.RiskAssessRequest{
		OperationID: op, ID: id, ExpectedRevision: expected,
		Level: level, AssessmentNote: note, Treatment: "mitigate", Deadline: &deadline,
	}, userM)
	require.NoError(t, err)
	return res
}

// currentBasis 读路径现势依据（单风险便捷封装）。
func currentBasis(t *testing.T, svc *risk.Service, id int64) string {
	t.Helper()
	facts, err := svc.CurrentDispatchFacts(context.Background(), testPool, userM,
		[]model.RiskDispatchExpected{{ID: id}})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	return facts[0].DispatchBasis
}

// lockBasis 锁路径现势依据（独立事务内取，调用方负责回滚前的完整断言）。
func lockBasis(t *testing.T, svc *risk.Service, id int64) string {
	t.Helper()
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	facts, err := svc.LockDispatchFacts(ctx, tx, []int64{id})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	return facts[0].DispatchBasis
}

func mirrorHashOf(t *testing.T, sourceRef, itemKey string) string {
	t.Helper()
	var hash string
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT encode(content_hash, 'hex') FROM risk_source_records WHERE source_ref=$1 AND source_item_key=$2`,
		sourceRef, itemKey).Scan(&hash))
	return hash
}

// seedBasisSource 本文件专属合成来源（独立 fingerprint_scope，不占用共享夹具
// 的 mock-vulnerability 家族计数——risk_item6 用例按该 scope 断言唯一风险）。
func seedBasisSource(t *testing.T) string {
	t.Helper()
	ref := "mock-basis-" + uniqueSuffix()
	_, err := testPool.Exec(context.Background(), `
INSERT INTO risk_sources(source_ref, display_name, fingerprint_scope, ordered, mapping_ref, allowed_org_ids, default_org_id, enabled, note)
VALUES ($1,'依据专用合成来源',$1,true,$2,ARRAY[$3::bigint],$3,true,'P1 修复回归隔离来源')
ON CONFLICT (source_ref) DO NOTHING`, ref, sourceMapping, orgGID)
	require.NoError(t, err)
	return ref
}

// ── 契约一：人工登记身份成对、后兼容、自由属性不伪装来源 ─────────────────

func TestRiskCreate_ManualFindingAssetPairContract(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	suffix := uniqueSuffix()

	// 成对携带 → 落库并在详情透出。
	ref := &model.RiskAssetRef{Provider: "mock-cmdb", ExternalID: "asset-pair-" + suffix}
	created := mustCreateIdentity(t, svc, "op-pair-"+suffix, "R-pair-"+suffix, orgGID, &userU,
		"CVE-2026-P1-"+suffix, ref)
	require.NotZero(t, created.Risk.ID)
	var findingKey, provider, external string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COALESCE(finding_key,''), COALESCE(asset_provider,''), COALESCE(asset_external_id,'')
		 FROM risk_items WHERE id=$1`, created.Risk.ID).Scan(&findingKey, &provider, &external))
	require.Equal(t, "CVE-2026-P1-"+suffix, findingKey)
	require.Equal(t, "mock-cmdb", provider)
	require.Equal(t, "asset-pair-"+suffix, external)
	detail, err := svc.Detail(ctx, created.Risk.ID, userM)
	require.NoError(t, err)
	require.Equal(t, "CVE-2026-P1-"+suffix, detail.View.FindingKey)
	require.NotNil(t, detail.View.AssetRef)
	require.Equal(t, "asset-pair-"+suffix, detail.View.AssetRef.ExternalID)

	// 后兼容：不带身份的既有登记不受影响。
	legacy := mustCreate(t, svc, "op-legacy-"+suffix, "R-legacy-"+suffix, orgGID, &userU)
	require.NotZero(t, legacy.Risk.ID)
	var legacyFinding *string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT finding_key FROM risk_items WHERE id=$1`, legacy.Risk.ID).Scan(&legacyFinding))
	require.Nil(t, legacyFinding)

	// 成对校验：只带 finding_key / 只带 asset_ref / asset_ref 字段空白 → 400。
	for i, tc := range []struct {
		findingKey string
		ref        *model.RiskAssetRef
	}{
		{"CVE-X", nil},
		{"", &model.RiskAssetRef{Provider: "mock-cmdb", ExternalID: "a"}},
		{"CVE-X", &model.RiskAssetRef{Provider: " ", ExternalID: "a"}},
	} {
		_, err := svc.Create(ctx, model.RiskCreateRequest{
			OperationID: "op-pair-bad-" + suffix + "-" + string(rune('a'+i)),
			Title:       "R-pair-bad", OrgID: orgGID, ExpectedRevision: "0",
			FindingKey: tc.findingKey, AssetRef: tc.ref,
		}, userM)
		require.ErrorIs(t, err, errcode.ErrInvalidParams, "case %d 应拒绝不成对身份", i)
	}

	// 禁止自由属性伪装来源：attributes 里的类来源键不落身份列。
	disguised := mustCreate(t, svc, "op-disguise-"+suffix, "R-disguise-"+suffix, orgGID, &userU)
	var attrFinding *string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT finding_key FROM risk_items WHERE id=$1`, disguised.Risk.ID).Scan(&attrFinding))
	require.Nil(t, attrFinding)

	// 同号同参幂等：重放带同身份返回原风险；同号异身份 → 409。
	replay := mustCreateIdentity(t, svc, "op-pair-"+suffix, "R-pair-"+suffix, orgGID, &userU,
		"CVE-2026-P1-"+suffix, ref)
	require.Equal(t, created.Risk.ID, replay.Risk.ID)
	_, err = svc.Create(ctx, model.RiskCreateRequest{
		OperationID: "op-pair-" + suffix, Title: "R-pair-" + suffix,
		OrgID: orgGID, OwnerUserID: &userU, ExpectedRevision: "0",
		FindingKey: "CVE-DIFFERENT", AssetRef: ref,
	}, userM)
	require.Equal(t, 94003, errCodeOf(t, err))

	// L3 不因携带身份放宽：副锚只读成员带身份登记仍被拒。
	_, err = svc.Create(ctx, model.RiskCreateRequest{
		OperationID: "op-pair-v-" + suffix, Title: "R-pair-v", OrgID: orgGID, ExpectedRevision: "0",
		FindingKey: "CVE-V", AssetRef: ref,
	}, userV)
	require.ErrorIs(t, err, errcode.ErrNoPermission)

	// 人工登记不落来源镜像/台账（origin 恒 manual，不伪装合成来源）。
	var sourceRecordCount int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT count(*) FROM risk_source_records WHERE source_item_key LIKE 'asset-pair-%'`).Scan(&sourceRecordCount))
	require.Equal(t, 0, sourceRecordCount)
}

// ── 契约二：人工核实依据（man1 版本串）随评估演进、随责任确认稳定 ─────────

func TestDispatchBasis_ManualVerifiedLifecycle(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	suffix := uniqueSuffix()

	ref := &model.RiskAssetRef{Provider: "mock-cmdb", ExternalID: "asset-man-" + suffix}
	created := mustCreateIdentity(t, svc, "op-man-"+suffix, "R-man-"+suffix, orgGID, &userU,
		"CVE-2026-MAN-"+suffix, ref)
	id := created.Risk.ID

	// 未评估：无当前评估事实 → 依据为空（不可派）。
	require.Empty(t, currentBasis(t, svc, id))

	// 获权核实留证：assess 用现有 assessment_note+revision 记录依据说明。
	assessed := assessRisk(t, svc, "op-man-as-"+suffix, id, "1", "high",
		"人工核实依据：漏洞复现+资产归属台账 "+suffix)
	require.Equal(t, int64(2), assessed.Risk.Revision)
	// 历史可回读该依据说明（留痕）。
	history, err := svc.History(ctx, id, 1, 50, userM)
	require.NoError(t, err)
	found := false
	for _, ev := range history.Items {
		if ev.EventKind == "assessed" && ev.AggregateRevision == 2 {
			found = true
		}
	}
	require.True(t, found, "assessed 事件应随 revision 2 留痕")

	manBasis := "man1:" + strconv.FormatInt(id, 10) + ":2"
	require.Equal(t, manBasis, currentBasis(t, svc, id))
	require.Equal(t, manBasis, lockBasis(t, svc, id), "读/锁两路径依据一致")

	// 责任确认（具名 owner）推进 revision 但不改变已核实依据版本。
	_, err = svc.Assign(ctx, model.RiskAssignRequest{
		OperationID: "op-man-assign-" + suffix, ID: id,
		ExpectedRevision: "2", OwnerUserID: userU, Reason: "资产责任人复核确认",
	}, userM)
	require.NoError(t, err)
	require.Equal(t, manBasis, currentBasis(t, svc, id), "owner 确认不使核实依据失效")

	// 重新核实 → 依据版本随之演进（新评估事实）。
	reassessed := assessRisk(t, svc, "op-man-as2-"+suffix, id, "3", "medium",
		"复核定级下调 "+suffix)
	require.Equal(t, int64(4), reassessed.Risk.Revision)
	require.Equal(t, "man1:"+strconv.FormatInt(id, 10)+":4", currentBasis(t, svc, id))

	// 存量兼容：不带身份的人工风险即便已评估也无依据（不可派，行为同现状）。
	legacy := mustCreate(t, svc, "op-man-legacy-"+suffix, "R-man-legacy-"+suffix, orgGID, &userU)
	assessRisk(t, svc, "op-man-legacy-as-"+suffix, legacy.Risk.ID, "1", "low", "存量样例")
	require.Empty(t, currentBasis(t, svc, legacy.Risk.ID))

	// 有身份但未评估 → 仍不可派（空依据）。
	fresh := mustCreateIdentity(t, svc, "op-man-fresh-"+suffix, "R-man-fresh-"+suffix, orgGID, &userU,
		"CVE-2026-FRESH-"+suffix, ref)
	require.Empty(t, currentBasis(t, svc, fresh.Risk.ID))
}

// ── 契约三：来源匹配依据（src1 版本串）纳入派单事实、撤回失效、重报恢复 ───

func TestDispatchBasis_SourceMatchEvidenceAndWithdrawal(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	suffix := uniqueSuffix()
	basisSource := seedBasisSource(t)
	itemKey := "asset-src-" + suffix + ":vuln-src:443"

	observe := func(eventID, seq, kind string, sevValue string) *model.RiskReportResult {
		t.Helper()
		obs := baseObservation(suffix, eventID, seq)
		obs.SourceRef = basisSource
		obs.SourceItemKey = itemKey
		obs.AssetRef = &model.RiskAssetRef{Provider: "mock-cmdb", ExternalID: "asset-src-" + suffix}
		obs.FindingKey = "vuln-src"
		if kind != "" {
			obs.ObservationKind = kind
		}
		if sevValue != "" {
			obs.SourceSeverity = &model.RiskSourceSeverity{Scheme: "provider-five-level", Value: sevValue}
		}
		res, err := svc.Report(ctx, model.RiskReportRequest{Observations: []model.FindingObservationV1{obs}}, userM)
		require.NoError(t, err)
		return res
	}

	first := observe("evt-s1", "301", "", "5")
	require.Equal(t, "accepted", first.Items[0].Result)
	riskID := first.Items[0].RiskRef.ID

	// 来源风险未评估前：无当前评估事实 → 空依据。
	require.Empty(t, currentBasis(t, svc, riskID))

	// 评估后依据 = 现势镜像内容版本（src1:hex(content_hash)）。
	assessRisk(t, svc, "op-src-as-"+suffix, riskID, "1", "high", "来源匹配核实 "+suffix)
	srcBasis := "src1:" + mirrorHashOf(t, basisSource, itemKey)
	require.Equal(t, srcBasis, currentBasis(t, svc, riskID))
	require.Equal(t, srcBasis, lockBasis(t, svc, riskID))

	// 同内容重复上报（duplicate）不更新镜像 → 依据稳定。
	dup := observe("evt-s1", "301", "", "5")
	require.Equal(t, "duplicate", dup.Items[0].Result)
	require.Equal(t, srcBasis, currentBasis(t, svc, riskID))

	// 来源重新断言（新事件新序号）→ 镜像推进 → 依据版本随之变化（需重预览）。
	observe("evt-s2", "302", "", "4")
	newBasis := "src1:" + mirrorHashOf(t, basisSource, itemKey)
	require.NotEqual(t, srcBasis, newBasis)
	require.Equal(t, newBasis, currentBasis(t, svc, riskID))

	// 合成来源撤回 → 镜像 kind=withdrawn → 依据失效（空 = 不可派）。
	withdrawn := observe("evt-s3", "303", "withdrawn", "")
	require.Equal(t, "accepted", withdrawn.Items[0].Result)
	require.NotNil(t, withdrawn.Items[0].RiskRef)
	require.Equal(t, riskID, withdrawn.Items[0].RiskRef.ID)
	require.Empty(t, currentBasis(t, svc, riskID), "撤回后不可派")
	require.Empty(t, lockBasis(t, svc, riskID))

	// 重新断言恢复可派依据（新镜像版本）。
	observe("evt-s4", "304", "", "5")
	require.Equal(t, "src1:"+mirrorHashOf(t, basisSource, itemKey), currentBasis(t, svc, riskID))
}

// ── 契约四：锁内取依据不锁来源镜像（防 report/source→risk 锁序反转） ──────

func TestDispatchBasis_LockPathDoesNotBlockOnSourceRecords(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	suffix := uniqueSuffix()
	basisSource := seedBasisSource(t)
	itemKey := "asset-lock-" + suffix + ":vuln-lock:443"

	obs := baseObservation(suffix, "evt-l1", "401")
	obs.SourceRef = basisSource
	obs.SourceItemKey = itemKey
	obs.AssetRef = &model.RiskAssetRef{Provider: "mock-cmdb", ExternalID: "asset-lock-" + suffix}
	obs.FindingKey = "vuln-lock"
	res, err := svc.Report(ctx, model.RiskReportRequest{Observations: []model.FindingObservationV1{obs}}, userM)
	require.NoError(t, err)
	riskID := res.Items[0].RiskRef.ID
	assessRisk(t, svc, "op-lock-as-"+suffix, riskID, "1", "high", "锁序样例 "+suffix)
	wantBasis := "src1:" + mirrorHashOf(t, basisSource, itemKey)

	// 另一连接持有来源镜像行锁（模拟 report 应用中的并发事务），
	// 派单锁路径取依据必须不阻塞（纯快照读，无 FOR UPDATE）。
	blocker, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer blocker.Rollback(ctx)
	tag, err := blocker.Exec(ctx,
		`SELECT 1 FROM risk_source_records WHERE source_ref=$1 AND source_item_key=$2 FOR UPDATE`,
		basisSource, itemKey)
	require.NoError(t, err)
	require.Equal(t, int64(1), tag.RowsAffected(), "夹具需真实持有镜像行锁")

	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	// 语句超时兜底：若实现误对镜像表加锁，这里会以超时错误暴露而不是挂死。
	_, err = tx.Exec(ctx, `SET LOCAL statement_timeout = '3s'`)
	require.NoError(t, err)
	facts, err := svc.LockDispatchFacts(ctx, tx, []int64{riskID})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.Equal(t, wantBasis, facts[0].DispatchBasis)
	require.NotEmpty(t, facts[0].FindingKey)
	require.NotNil(t, facts[0].AssetRef)
}
