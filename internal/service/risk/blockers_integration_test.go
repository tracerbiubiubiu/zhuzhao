//go:build integration

package risk_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
)

// 外审冻结候选三阻断（2026-11-22 独立复核确认）红绿回归：
// ① 写入口不可见与不存在同 404、可见而缺当前 L3 才 403（详设 §7）；
// ② 同号重放在同一 IAM 命令事务内复核当前对象——原操作人失去主锚/权限后
//    不得从重放读业务元数据，合法重放仍返回原收据；
// ③ 并发首次同 (source, source_item_key) 上报的确定性 seq/镜像——先按来源项
//    稳定键串行并在锁内重读，处理等序冲突与不同序，恰好一次周期/业务应用。
// 全部为合成数据（详设 §13/§14 命名），不指向任何真实组织/人员/路径。

// ── 合成夹具（仅本文件使用的撤权对照件，不污染共享 fixture） ──────────────

// seedIsolatedOrg 建独立合成组织（代码/路径带唯一后缀）。
func seedIsolatedOrg(t *testing.T, suffix string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, testPool.QueryRow(context.Background(), `
INSERT INTO organizations(code, name, path, status, is_system, is_virtual)
VALUES ($1, $2, $3::ltree, 1, false, true)
RETURNING id`, "iso-"+suffix, "合成隔离组-"+suffix, "orgiso"+suffix).Scan(&id))
	return id
}

// seedOrgMember 建合成用户并以指定成员角色入组（撤权对照专用主体）。
func seedOrgMember(t *testing.T, orgID int64, role string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, testPool.QueryRow(context.Background(), `
INSERT INTO users(username, password, real_name, status)
VALUES ($1, 'fixture', '合成对照主体', 1) RETURNING id`,
		"cc_"+role+"_"+uniqueSuffix()).Scan(&id))
	_, err := testPool.Exec(context.Background(), `
INSERT INTO user_orgs(user_id, org_id, org_member_role, ticket_scope)
VALUES ($1, $2, $3, 'group')
ON CONFLICT (user_id, org_id) DO NOTHING`, id, orgID, role)
	require.NoError(t, err)
	return id
}

// grantOrgMembership / dropOrgMembership 撤权对照的成员资格增删。
func grantOrgMembership(t *testing.T, userID, orgID int64, role string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
INSERT INTO user_orgs(user_id, org_id, org_member_role, ticket_scope)
VALUES ($1, $2, $3, 'group')
ON CONFLICT (user_id, org_id) DO NOTHING`, userID, orgID, role)
	require.NoError(t, err)
}

func dropOrgMembership(t *testing.T, userID, orgID int64) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`DELETE FROM user_orgs WHERE user_id=$1 AND org_id=$2`, userID, orgID)
	require.NoError(t, err)
}

func revisionOf(t *testing.T, riskID int64) int64 {
	t.Helper()
	var rev int64
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT revision FROM risk_items WHERE id=$1`, riskID).Scan(&rev))
	return rev
}

func eventsOf(t *testing.T, riskID int64) int {
	t.Helper()
	var n int
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM risk_events WHERE risk_id=$1`, riskID).Scan(&n))
	return n
}

// familyKeyHash 复算来源项稳定键指纹（服务层 canonicalKeyHex 同式：scope+"\n"+key）。
func familyKeyHash(scope, key string) string {
	sum := sha256.Sum256([]byte(scope + "\n" + key))
	return hex.EncodeToString(sum[:])
}

func familyItemCount(t *testing.T, scope, key string) int {
	t.Helper()
	var n int
	require.NoError(t, testPool.QueryRow(context.Background(), `
SELECT count(*) FROM risk_items ri JOIN risk_families f ON f.id=ri.family_id
WHERE f.fingerprint_scope=$1 AND f.canonical_key_hash=$2`, scope, familyKeyHash(scope, key)).Scan(&n))
	return n
}

// familyApplyEventCount 来源项业务应用事件数：首次应用建周期写 created
// （payload origin=source），后续应用写 source_observed——两者合计即业务应用次数。
func familyApplyEventCount(t *testing.T, scope, key string) int {
	t.Helper()
	var n int
	require.NoError(t, testPool.QueryRow(context.Background(), `
SELECT count(*) FROM risk_events e JOIN risk_items ri ON ri.id=e.risk_id
JOIN risk_families f ON f.id=ri.family_id
WHERE f.fingerprint_scope=$1 AND f.canonical_key_hash=$2
  AND e.event_kind IN ('created','source_observed')`,
		scope, familyKeyHash(scope, key)).Scan(&n))
	return n
}

func observationCount(t *testing.T, eventID string) int {
	t.Helper()
	var n int
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM risk_observations WHERE event_id=$1`, eventID).Scan(&n))
	return n
}

func latestSequenceOf(t *testing.T, scope, key string) string {
	t.Helper()
	var seq string
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT latest_sequence FROM risk_source_records WHERE source_ref=$1 AND source_item_key=$2`,
		scope, key).Scan(&seq))
	return seq
}

// requireMirrorMatches 断言来源镜像 hash 与指定事件的已收观察 hash 一致
// （镜像推进来自该事件的一次业务应用，而非并发噪声）。
func requireMirrorMatches(t *testing.T, scope, key, eventID string) {
	t.Helper()
	var recordHash, obsHash []byte
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT content_hash FROM risk_source_records WHERE source_ref=$1 AND source_item_key=$2`,
		scope, key).Scan(&recordHash))
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT content_hash FROM risk_observations WHERE source_ref=$1 AND event_id=$2`,
		scope, eventID).Scan(&obsHash))
	require.True(t, bytes.Equal(recordHash, obsHash), "镜像应来自事件 %s 的应用", eventID)
}

// ── 阻断①：写入口不可见=404（与不存在同形）、可见无 L3=403 ──────────────

func TestRiskWriteEntry_InvisibleIs404_VisibleWithoutL3Is403(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	suffix := uniqueSuffix()
	r := mustCreate(t, svc, "op-w1-create-"+suffix, "W1-"+suffix, orgGID, &userU)

	assessReq := func(op string, id int64) model.RiskAssessRequest {
		return model.RiskAssessRequest{OperationID: op, ID: id, ExpectedRevision: "1",
			Level: "high", Treatment: "mitigate"}
	}
	assignReq := func(op string, id int64) model.RiskAssignRequest {
		return model.RiskAssignRequest{OperationID: op, ID: id, ExpectedRevision: "1",
			OwnerUserID: userU, Reason: "合成责任确认"}
	}

	// ①-a 无任何范围成员资格的 N：可见集为空——写入口对隐藏与不存在同 404，
	// 不泄露对象存在性（详设 §7：不可见与不存在均 404）。
	for _, id := range []int64{r.Risk.ID, 987654321} {
		suffixID := suffix + "-" + strconv.FormatInt(id, 10)
		_, err := svc.Assess(ctx, assessReq("op-w1-n-"+suffixID, id), userN)
		require.Equal(t, 94001, errCodeOf(t, err), "assess 对不可见风险应与不存在同 404")
		_, err = svc.Assign(ctx, assignReq("op-w1-na-"+suffixID, id), userN)
		require.Equal(t, 94001, errCodeOf(t, err), "assign 对不可见风险应与不存在同 404")
	}

	// ①-b 可见而缺当前 L3：副锚只读 V（受影响部门成员，非管理者非 owner）→ 403；
	// owner U 具 assess 资格但 assign 为主锚管理者专属 → 403。
	_, err := svc.Assess(ctx, assessReq("op-w1-v-"+suffix, r.Risk.ID), userV)
	require.Equal(t, 70001, errCodeOf(t, err), "可见而无 L3 才 403")
	_, err = svc.Assign(ctx, assignReq("op-w1-u-"+suffix, r.Risk.ID), userU)
	require.Equal(t, 70001, errCodeOf(t, err), "owner 可见但无管理资格，assign 403")

	// ①-c 撤权正负对照（专用管理者，隔离管理组，不污染共享夹具）：
	// 先正向成功 → 撤主锚保留只读可见 → 403 → 撤全部成员资格 → 与不存在同 404。
	orgG2 := seedIsolatedOrg(t, "w1g-"+suffix)
	orgD2 := seedIsolatedOrg(t, "w1d-"+suffix)
	m2 := seedOrgMember(t, orgG2, "admin")
	affectedD2 := orgD2
	res2, err := svc.Create(ctx, model.RiskCreateRequest{
		OperationID: "op-w1-c2-" + suffix, Title: "W2-" + suffix,
		OrgID: orgG2, AffectedOrgID: &affectedD2, ExpectedRevision: "0"}, m2)
	require.NoError(t, err, "正向：主锚管理者可登记")

	assessed, err := svc.Assess(ctx, assessReq("op-w1-a2-"+suffix, res2.Risk.ID), m2)
	require.NoError(t, err, "正向：主锚管理者可评估")
	assigned, err := svc.Assign(ctx, model.RiskAssignRequest{
		OperationID: "op-w1-s2-" + suffix, ID: res2.Risk.ID,
		ExpectedRevision: strconv.FormatInt(assessed.Risk.Revision, 10),
		OwnerUserID:      userU, Reason: "合成责任确认"}, m2)
	require.NoError(t, err, "正向：主锚管理者可派责任")

	// 撤主锚（G2 admin），保留受影响部门 D2 只读成员资格 → 可见但无 L3 → 403。
	dropOrgMembership(t, m2, orgG2)
	grantOrgMembership(t, m2, orgD2, "member")
	rev := strconv.FormatInt(assigned.Risk.Revision, 10)
	_, err = svc.Assess(ctx, assessReq("op-w1-rv1-"+suffix, res2.Risk.ID), m2)
	require.Equal(t, 70001, errCodeOf(t, err), "撤主锚后仍可见：assess 403")
	_, err = svc.Assign(ctx, model.RiskAssignRequest{
		OperationID: "op-w1-rv2-" + suffix, ID: res2.Risk.ID,
		ExpectedRevision: rev, OwnerUserID: userU, Reason: "合成责任确认"}, m2)
	require.Equal(t, 70001, errCodeOf(t, err), "撤主锚后仍可见：assign 403")

	// 再撤全部成员资格 → 不可见 → 与不存在同 404。
	dropOrgMembership(t, m2, orgD2)
	_, err = svc.Assess(ctx, assessReq("op-w1-rv3-"+suffix, res2.Risk.ID), m2)
	require.Equal(t, 94001, errCodeOf(t, err), "撤全部资格后与不存在同 404")
	_, err = svc.Assign(ctx, model.RiskAssignRequest{
		OperationID: "op-w1-rv4-" + suffix, ID: res2.Risk.ID,
		ExpectedRevision: rev, OwnerUserID: userU, Reason: "合成责任确认"}, m2)
	require.Equal(t, 94001, errCodeOf(t, err), "撤全部资格后与不存在同 404")

	// 多组对照：G 组管理者对 G2 组风险无任何成员资格 → 404；两组互不可见互不越权。
	_, err = svc.Assess(ctx, assessReq("op-w1-x1-"+suffix, res2.Risk.ID), userM)
	require.Equal(t, 94001, errCodeOf(t, err), "他组管理者对 G2 风险不可见 → 404")
	_, err = svc.Assign(ctx, assignReq("op-w1-x2-"+suffix, res2.Risk.ID), userM)
	require.Equal(t, 94001, errCodeOf(t, err))
	_, err = svc.Assess(ctx, assessReq("op-w1-x3-"+suffix, r.Risk.ID), m2)
	require.Equal(t, 94001, errCodeOf(t, err), "失权后的 m2 对 G 组风险不可见 → 404")
}

// ── 阻断②：同号重放在命令事务内复核当前对象，失权不得读业务元数据 ────────

func TestRiskReplay_RechecksCurrentObjectInCommandTransaction(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	suffix := uniqueSuffix()

	// 正对照：合法重放返回原收据（同号同参），且无新业务效果；同号异参 409。
	r := mustCreate(t, svc, "op-rp-create-"+suffix, "RP-"+suffix, orgGID, &userU)
	deadline := dDayAt(t, 1)
	assessReq := model.RiskAssessRequest{OperationID: "op-rp-as-" + suffix, ID: r.Risk.ID,
		ExpectedRevision: "1", Level: "high", Treatment: "mitigate", Deadline: &deadline}
	first, err := svc.Assess(ctx, assessReq, userM)
	require.NoError(t, err)
	revAfterFirst := revisionOf(t, r.Risk.ID)
	replay, err := svc.Assess(ctx, assessReq, userM)
	require.NoError(t, err)
	require.True(t, replay.Receipt.Replayed)
	require.Equal(t, first.Receipt.OperationID, replay.Receipt.OperationID)
	require.Equal(t, first.Receipt.Status, replay.Receipt.Status)
	require.Equal(t, first.Receipt.RiskID, replay.Receipt.RiskID)
	require.Equal(t, first.Receipt.Revision, replay.Receipt.Revision)
	require.Equal(t, revAfterFirst, revisionOf(t, r.Risk.ID), "合法重放不产生新业务效果")
	_, err = svc.Assess(ctx, model.RiskAssessRequest{OperationID: assessReq.OperationID,
		ID: r.Risk.ID, ExpectedRevision: "1", Level: "low", Treatment: "mitigate"}, userM)
	require.Equal(t, 94003, errCodeOf(t, err), "仍授权者的同号异参保持 409")

	// 负对照：隔离组专用管理者 m2——assess/assign/create 三命令各持一原号，
	// 撤全部成员资格（失主锚+失可见性）后重放不得返回任何业务元数据。
	orgG2 := seedIsolatedOrg(t, "rp-"+suffix)
	m2 := seedOrgMember(t, orgG2, "admin")
	affectedG2 := orgG2
	createReq := model.RiskCreateRequest{
		OperationID: "op-rp2-create-" + suffix, Title: "RP2-" + suffix,
		Description: "合成风险-RP2", OrgID: orgG2, AffectedOrgID: &affectedG2,
		OwnerUserID: &userU, ExpectedRevision: "0"}
	r2, err := svc.Create(ctx, createReq, m2)
	require.NoError(t, err)
	assessReq2 := model.RiskAssessRequest{OperationID: "op-rp2-as-" + suffix, ID: r2.Risk.ID,
		ExpectedRevision: "1", Level: "high", Treatment: "mitigate"}
	_, err = svc.Assess(ctx, assessReq2, m2)
	require.NoError(t, err)
	assignReq2 := model.RiskAssignRequest{OperationID: "op-rp2-asg-" + suffix, ID: r2.Risk.ID,
		ExpectedRevision: "2", OwnerUserID: userU, Reason: "合成责任确认"}
	_, err = svc.Assign(ctx, assignReq2, m2)
	require.NoError(t, err)

	revBefore := revisionOf(t, r2.Risk.ID)
	evBefore := eventsOf(t, r2.Risk.ID)
	dropOrgMembership(t, m2, orgG2)
	_, err = svc.Assess(ctx, assessReq2, m2)
	require.Equal(t, 94001, errCodeOf(t, err), "失去主锚后 assess 重放不得返回收据")
	_, err = svc.Assign(ctx, assignReq2, m2)
	require.Equal(t, 94001, errCodeOf(t, err), "失去主锚后 assign 重放不得返回收据")
	_, err = svc.Create(ctx, createReq, m2)
	require.Equal(t, 94001, errCodeOf(t, err), "失去主锚后 create 重放不得返回收据")
	require.Equal(t, revBefore, revisionOf(t, r2.Risk.ID), "被拒重放不得产生业务效果")
	require.Equal(t, evBefore, eventsOf(t, r2.Risk.ID), "被拒重放不得追加事件")

	// 可见无 L3 变体：恢复为普通成员（可见性成立、管理资格不成立）→ 403，同样不返回收据。
	grantOrgMembership(t, m2, orgG2, "member")
	_, err = svc.Assess(ctx, assessReq2, m2)
	require.Equal(t, 70001, errCodeOf(t, err), "可见而缺 L3：assess 重放 403")
	_, err = svc.Assign(ctx, assignReq2, m2)
	require.Equal(t, 70001, errCodeOf(t, err), "可见而缺 L3：assign 重放 403")
	_, err = svc.Create(ctx, createReq, m2)
	require.Equal(t, 70001, errCodeOf(t, err), "可见而缺 L3：create 重放 403")
	require.Equal(t, revBefore, revisionOf(t, r2.Risk.ID))
	require.Equal(t, evBefore, eventsOf(t, r2.Risk.ID))
}

// ── 阻断③：并发首次同 source_item_key 的确定性 seq/镜像 ─────────────────

// seedConcurrencySource 建独立合成来源（fingerprint_scope 随用例唯一）——
// 并发用例不向共享 mock-vulnerability 来源贡献风险行（全包计数隔离纪律）。
func seedConcurrencySource(t *testing.T, sourceRef string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
INSERT INTO risk_sources(source_ref, display_name, fingerprint_scope, ordered, mapping_ref, allowed_org_ids, default_org_id, enabled, note)
VALUES ($1, '合成并发来源', $1, true, $2, ARRAY[$3::bigint], $3, true, '三阻断并发红绿专用合成来源')
ON CONFLICT (source_ref) DO NOTHING`, sourceRef, sourceMapping, orgGID)
	require.NoError(t, err)
}

// concurrentObservation 合成观察（同键异 event：severity value 区分内容 hash）。
func concurrentObservation(scope, suffix, eventID, key, seq, sevValue string) model.FindingObservationV1 {
	return model.FindingObservationV1{
		SourceRef:      scope,
		SourceItemKey:  key,
		EventID:        eventID + "-" + suffix,
		SourceSequence: seq,
		ObservedAt:     time.Date(2026, 11, 22, 5, 0, 0, 0, time.UTC),
		AssetRef:       &model.RiskAssetRef{Provider: "mock-cmdb", ExternalID: "asset-cc"},
		FindingKey:     "vuln-cc",
		SourceSeverity: &model.RiskSourceSeverity{Scheme: "provider-five-level", Value: sevValue},
		MappingRef:     sourceMapping,
	}
}

// reportConcurrently 8 路并发各报一条观察（详设 §13：并发反例 8 次口径）。
func reportConcurrently(t *testing.T, svc *risk.Service, obs []model.FindingObservationV1) []model.RiskReportItem {
	t.Helper()
	items := make([]model.RiskReportItem, len(obs))
	errs := make([]error, len(obs))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range obs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			res, err := svc.Report(context.Background(), model.RiskReportRequest{
				Observations: []model.FindingObservationV1{obs[i]}}, userM)
			errs[i] = err
			if res != nil {
				items[i] = res.Items[0]
			}
		}(i)
	}
	close(start)
	wg.Wait()
	for i := range errs {
		require.NoError(t, errs[i], "并发上报 %d 不应失败", i)
	}
	return items
}

func reportOne(t *testing.T, svc *risk.Service, scope, suffix, eventID, key, seq, sev string) model.RiskReportItem {
	t.Helper()
	res, err := svc.Report(context.Background(), model.RiskReportRequest{
		Observations: []model.FindingObservationV1{concurrentObservation(scope, suffix, eventID, key, seq, sev)}}, userM)
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	return res.Items[0]
}

func groupByResult(items []model.RiskReportItem) map[string][]model.RiskReportItem {
	grouped := map[string][]model.RiskReportItem{}
	for _, it := range items {
		grouped[it.Result] = append(grouped[it.Result], it)
	}
	return grouped
}

func TestRiskReport_ConcurrentFirstReportsAreDeterministic(t *testing.T) {
	seedRiskFixture(t)
	svc := newService()
	suffix := uniqueSuffix()
	scope := "mock-concurrency-" + suffix
	seedConcurrencySource(t, scope)

	t.Run("等序并发首报只应用一次", func(t *testing.T) {
		key := "cc-eq:vuln:443-" + suffix
		obs := make([]model.FindingObservationV1, 8)
		for i := range obs {
			obs[i] = concurrentObservation(scope, suffix, "eq-"+strconv.Itoa(i), key, "500", strconv.Itoa(5+i))
		}
		items := reportConcurrently(t, svc, obs)
		grouped := groupByResult(items)
		require.Len(t, grouped["accepted"], 1,
			"等序异内容并发首报只允许一条应用: %v", items)
		require.Len(t, grouped["quarantined"], 7, "其余等序输入应全部隔离: %v", items)
		for _, it := range grouped["quarantined"] {
			require.NotEmpty(t, it.SafeReason, "隔离项应可区别原因")
		}
		require.Equal(t, 1, familyItemCount(t, scope, key), "并发首报只允许一个周期")
		require.Equal(t, 1, familyApplyEventCount(t, scope, key), "只允许一次业务应用事件")
		require.Equal(t, "500", latestSequenceOf(t, scope, key))
		requireMirrorMatches(t, scope, key, grouped["accepted"][0].EventID)

		// 确定性收尾：同键旧序 stale、新序推进同周期，不新建第二代。
		stale := reportOne(t, svc, scope, suffix, "eq-late", key, "499", "9")
		require.Equal(t, "stale", stale.Result)
		advance := reportOne(t, svc, scope, suffix, "eq-next", key, "501", "8")
		require.Equal(t, "accepted", advance.Result)
		require.NotNil(t, advance.RiskRef)
		require.NotNil(t, grouped["accepted"][0].RiskRef)
		require.Equal(t, grouped["accepted"][0].RiskRef.ID, advance.RiskRef.ID)
		require.Equal(t, 1, familyItemCount(t, scope, key))
		require.Equal(t, 2, familyApplyEventCount(t, scope, key), "第二次应用入同周期历史")
		require.Equal(t, "501", latestSequenceOf(t, scope, key))
	})

	t.Run("不同序并发首报镜像确定", func(t *testing.T) {
		key := "cc-ord:vuln:443-" + suffix
		seqs := []string{"600", "600", "601", "601", "602", "602", "599", "603"}
		obs := make([]model.FindingObservationV1, len(seqs))
		for i := range seqs {
			obs[i] = concurrentObservation(scope, suffix, "ord-"+strconv.Itoa(i), key, seqs[i], strconv.Itoa(i))
		}
		items := reportConcurrently(t, svc, obs)
		grouped := groupByResult(items)
		for _, it := range items {
			require.Contains(t, []string{"accepted", "stale", "quarantined"}, it.Result,
				"非 accepted 项只能是 stale/quarantined: %v", it)
			switch it.Result {
			case "stale":
				require.Contains(t, it.SafeReason, "stale_sequence:", "stale 应可区别原因")
			case "quarantined":
				require.Contains(t, it.SafeReason, "conflict:", "等序冲突应可区别原因")
			}
		}
		accepted := grouped["accepted"]
		require.NotEmpty(t, accepted)
		seenSeq := map[string]bool{}
		var maxID int64
		for _, it := range accepted {
			require.NotNil(t, it.RiskRef)
			if maxID == 0 {
				maxID = it.RiskRef.ID
			}
			require.Equal(t, maxID, it.RiskRef.ID, "全部应用落在同一条风险周期")
			// 接受项的序号：从 event id 反查 seq，同序至多一条 accepted。
			var seq string
			require.NoError(t, testPool.QueryRow(context.Background(),
				`SELECT source_sequence FROM risk_observations WHERE event_id=$1`, it.EventID).Scan(&seq))
			require.False(t, seenSeq[seq], "同序不允许重复业务应用: %v", accepted)
			seenSeq[seq] = true
		}
		require.Contains(t, seenSeq, "603", "最大序号必被应用")
		require.Len(t, accepted, familyApplyEventCount(t, scope, key),
			"业务应用事件数与接受数一致（无幻影应用）")
		require.Equal(t, 1, familyItemCount(t, scope, key))
		require.Equal(t, "603", latestSequenceOf(t, scope, key), "镜像终态确定收敛到最大序号")
		requireMirrorMatches(t, scope, key, grouped["accepted"][len(accepted)-1].EventID)
		// 台账收据不丢不重：accepted/stale 各恰一行；等序隔离项不落观察行，
		// 以隔离记录保留凭据（结果项已断言 SafeReason=conflict:N）。
		resultByEvent := map[string]string{}
		for _, it := range items {
			resultByEvent[it.EventID] = it.Result
		}
		for _, o := range obs {
			if resultByEvent[o.EventID] == "quarantined" {
				require.Equal(t, 0, observationCount(t, o.EventID), "等序隔离项不落观察行")
				var conflicts int
				require.NoError(t, testPool.QueryRow(context.Background(),
					`SELECT count(*) FROM risk_ingest_conflicts WHERE event_id=$1`, o.EventID).Scan(&conflicts))
				require.GreaterOrEqual(t, conflicts, 1, "隔离凭据落库")
				continue
			}
			require.Equal(t, 1, observationCount(t, o.EventID), "event %s 收据不丢不重", o.EventID)
		}
	})

	t.Run("同事件并发重复只应用一次", func(t *testing.T) {
		key := "cc-dup:vuln:443-" + suffix
		obs := make([]model.FindingObservationV1, 8)
		for i := range obs {
			obs[i] = concurrentObservation(scope, suffix, "dup-same", key, "700", "7") // 同 event 同内容
		}
		items := reportConcurrently(t, svc, obs)
		grouped := groupByResult(items)
		require.Len(t, grouped["accepted"], 1, "同事件并发只应用一次: %v", items)
		require.Len(t, grouped["duplicate"], 7, "其余并发重投应返回 duplicate: %v", items)
		require.Equal(t, 1, observationCount(t, "dup-same-"+suffix), "台账只落一行")
		require.Equal(t, 1, familyItemCount(t, scope, key))
		require.Equal(t, 1, familyApplyEventCount(t, scope, key))
		require.Equal(t, "700", latestSequenceOf(t, scope, key))
		requireMirrorMatches(t, scope, key, "dup-same-"+suffix)
	})
}
