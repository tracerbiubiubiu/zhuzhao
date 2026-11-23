//go:build integration

// B2 第 7 项「个人派单与部门统计」读取/聚合接缝的独立 PG 单测。
// 口径 SSOT: docs/planning/phase6-b2-kickoff-plan.md 第 7 项 + §1 个人派单与部门统计。
//
// 权限纪律：可见性正反例一律走生产工单资源 GetFilter（pgxScopeResolver 真实 SQL），
// 不使用放行 authorizer 桩；放行桩仅用于错误传播与空谓词 fail-closed 的负例。
package riskdispatch

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// ---------- 生产等价装配（端口适配参考） ----------

// dsGetFilterAdapter 组合根适配参考形状：ticket 资源 GetFilter → 接缝端口。
// 生产装配同形（riskdispatch 不 import ticket——沿 TicketCreator 端口同款域边界）。
type dsGetFilterAdapter struct{ res *ticket.Resource }

func (a dsGetFilterAdapter) VisibleTicketsWhere(ctx context.Context, actorID int64) (string, []any, bool, error) {
	f, err := a.res.GetFilter(ctx, actorID, "list")
	if err != nil {
		return "", nil, false, err
	}
	return f.Where, f.Args, f.Unscoped, nil
}

// dsTicketRepo GetFilter 路径不消费 repo/cc/delegation；给拒绝桩（非放行），
// MatchesLiveOrgPaths 用真实 SQL 保持 livePathMatch 同语义。
type dsTicketRepo struct{ pool *pgxpool.Pool }

func (r dsTicketRepo) GetByID(context.Context, int64) (*model.Ticket, error) {
	return nil, errcode.ErrTicketNotFound
}

func (r dsTicketRepo) MatchesLiveOrgPaths(ctx context.Context, orgID int64, paths []string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM organizations o WHERE o.id = $1 AND o.path::text = ANY($2)
		 AND `+repository.OrgSourceActiveSQL("o")+`)`, orgID, paths).Scan(&ok)
	return ok, err
}

type dsDenyDelegation struct{}

func (dsDenyDelegation) IsOrgAdminOrOwner(context.Context, int64, int64) (bool, error) {
	return false, nil
}
func (dsDenyDelegation) IsAncestorOwner(context.Context, int64, int64, string) (bool, error) {
	return false, nil
}
func (dsDenyDelegation) HasOrgManagePermission(context.Context, int64) (bool, error) {
	return false, nil
}

type dsDenyCC struct{}

func (dsDenyCC) IsCC(context.Context, int64, int64) (bool, error) { return false, nil }

// newDeptStatsReader 真实装配：生产 ResolveScope + 生产 GetFilter SQL。
func newDeptStatsReader(pool *pgxpool.Pool) *DeptStats {
	res := ticket.NewResource(dsTicketRepo{pool: pool}, ticket.NewPgxScopeResolver(pool),
		dsDenyDelegation{}, dsDenyCC{})
	return NewDeptStats(dsGetFilterAdapter{res: res})
}

// ---------- 夹具 ----------

type dsFixture struct {
	tag          string
	mgmtOrg      int64
	targetOrg    int64
	deptA        int64 // 甲部门（调出前主部门）
	deptB        int64 // 乙部门（调入主部门）
	viewerOrg    int64 // 窄查看者锚点（与票目标组织无链）
	familyID     int64
	typeCode     string
	ownerU       int64
	unaffiliated int64 // 无主部门责任人
	dispatcherM  int64
	viewer       int64 // 本夹具目标组织锚点透明读查询者（隔离视角 + 真实正例）
	viewerNarrow int64 // assigned 窄查询者（真实反例）
}

func newDeptStatsFixture(t *testing.T, db *pgxpool.Pool, tag string) dsFixture {
	t.Helper()
	// 先注册清理（预清残留 + t.Cleanup）：注册点必须在建数之前。
	cleanupDeptStatsFixture(t, db, tag)
	ctx := context.Background()
	f := dsFixture{tag: tag, typeCode: "ds_remediation_" + tag}

	newOrg := func(code, name string) int64 {
		var id int64
		// path 显式传参：ltree 与 code 复用同参数位会让 PG 推断 42P08。
		require.NoError(t, db.QueryRow(ctx, `
			INSERT INTO organizations (code, name, path) VALUES ($1, $2, $3) RETURNING id
		`, "ds_"+code+"_"+tag, name, "ds_"+code+"_"+tag).Scan(&id))
		return id
	}
	f.mgmtOrg = newOrg("mgmt", "风险管理组织")
	f.targetOrg = newOrg("target", "整改目标组织")
	f.deptA = newOrg("depta", "部门甲")
	f.deptB = newOrg("deptb", "部门乙")
	f.viewerOrg = newOrg("viewer", "无关部门")

	require.NoError(t, db.QueryRow(ctx, `
		INSERT INTO risk_families (fingerprint_scope, canonical_key_hash) VALUES ($1, $2) RETURNING id
	`, "ds_"+tag, tag).Scan(&f.familyID))
	execMustNoError(t, db, ctx, `
		INSERT INTO ticket_types (code, name) VALUES ($1, $2) ON CONFLICT DO NOTHING
	`, f.typeCode, "DS type "+tag)

	newUser := func(name, real string) int64 {
		var id int64
		require.NoError(t, db.QueryRow(ctx, `
			INSERT INTO users (username, password, real_name, status) VALUES ($1, 'x', $2, 1) RETURNING id
		`, name+"_"+tag, real).Scan(&id))
		return id
	}
	f.ownerU = newUser("ds_owner", "责任人U")
	f.unaffiliated = newUser("ds_free", "无部门责任人")
	f.dispatcherM = newUser("ds_dispatcher", "派单者M")
	f.viewer = newUser("ds_view_anchor", "锚点查询者")
	f.viewerNarrow = newUser("ds_view_narrow", "窄查询者")

	// ownerU 现主部门 = 甲；无部门责任人无 user_orgs 行。
	execMustNoError(t, db, ctx, `
		INSERT INTO user_orgs (user_id, org_id, is_primary) VALUES ($1, $2, true)
	`, f.ownerU, f.deptA)
	// 窄查询者 scope=assigned 不扩展、锚点与票组织无链 → 真实不可见反例。
	execMustNoError(t, db, ctx, `
		INSERT INTO user_orgs (user_id, org_id, is_primary) VALUES ($1, $2, true)
	`, f.viewerNarrow, f.viewerOrg)
	// 查询者 = 本夹具目标组织实体成员（默认 entity_transparent_read）→ 透明读正例。
	// 隔离视角：锚点子树只含本夹具票——shared PG 里既有测试残留的在办组/票
	// （其他 target 组织）不进本用例聚合，勿用 scope=all 全量轴替代。
	execMustNoError(t, db, ctx, `
		INSERT INTO user_orgs (user_id, org_id, is_primary) VALUES ($1, $2, true)
	`, f.viewer, f.targetOrg)
	return f
}

func cleanupDeptStatsFixture(t *testing.T, db *pgxpool.Pool, tag string) {
	t.Helper()
	ctx := context.Background()
	typeCode := "ds_remediation_" + tag
	// 每条语句自带参数（占位符与参数数严格一致，未引用参数会 42P18）。
	stmts := []struct {
		sql  string
		args []any
	}{
		{`DELETE FROM risk_ticket_links WHERE group_id IN
			(SELECT id FROM risk_dispatch_groups WHERE type_code = $1)`, []any{typeCode}},
		{`DELETE FROM risk_dispatch_groups WHERE type_code = $1`, []any{typeCode}},
		{`DELETE FROM risk_items WHERE human_key LIKE 'DS-' || $1 || '-%'`, []any{tag}},
		{`DELETE FROM tickets WHERE type_code = $1`, []any{typeCode}},
		{`DELETE FROM user_orgs WHERE org_id IN
			(SELECT id FROM organizations WHERE code LIKE 'ds_%_' || $1)
		 OR user_id IN (SELECT id FROM users WHERE username LIKE '%_' || $1)`, []any{tag}},
		{`DELETE FROM users WHERE username LIKE '%_' || $1`, []any{tag}},
		{`DELETE FROM organizations WHERE code LIKE 'ds_%_' || $1`, []any{tag}},
		{`DELETE FROM ticket_types WHERE code = $1`, []any{typeCode}},
		{`DELETE FROM risk_families WHERE fingerprint_scope = 'ds_' || $1`, []any{tag}},
	}
	run := func() {
		for _, q := range stmts {
			_, err := db.Exec(ctx, q.sql, q.args...)
			if err != nil {
				t.Logf("cleanup dept stats fixture (%s): %v", tag, err)
			}
		}
	}
	run() // 先清残留
	t.Cleanup(run)
}

func (f dsFixture) insertTicket(t *testing.T, db *pgxpool.Pool, title string) int64 {
	t.Helper()
	ctx := context.Background()
	var orgPath string
	require.NoError(t, db.QueryRow(ctx,
		`SELECT path::text FROM organizations WHERE id = $1`, f.targetOrg).Scan(&orgPath))
	var id int64
	require.NoError(t, db.QueryRow(ctx, `
		INSERT INTO tickets (type_code, title, priority, status, created_by, org_id, org_path)
		VALUES ($1, $2, 1, 'open', $3, $4, $5) RETURNING id
	`, f.typeCode, title, f.ownerU, f.targetOrg, orgPath).Scan(&id))
	return id
}

func (f dsFixture) insertGroup(t *testing.T, db *pgxpool.Pool, finding string,
	owner, ownerOrgAtDispatch, ticketID int64) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	groupKey := fmt.Sprintf("%x", sha256.Sum256([]byte(finding+":"+fmt.Sprint(owner))))
	require.NoError(t, db.QueryRow(ctx, `
		INSERT INTO risk_dispatch_groups
			(finding_key, owner_user_id, owner_org_at_dispatch, management_org_id, target_org_id,
			 type_code, definition_version, approved_scope, group_key, ticket_id,
			 ticket_ref_snapshot, operator_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, 1, 'scope-1', $7, $8, $9, $10)
		RETURNING id
	`, finding, owner, ownerOrgAtDispatch, f.mgmtOrg, f.targetOrg, f.typeCode,
		groupKey, ticketID, "RD-"+finding, f.dispatcherM).Scan(&id))
	return id
}

func (f dsFixture) insertLinkedRisk(t *testing.T, db *pgxpool.Pool, seq int,
	finding string, groupID int64) int64 {
	t.Helper()
	ctx := context.Background()
	var riskID int64
	require.NoError(t, db.QueryRow(ctx, `
		INSERT INTO risk_items (human_key, family_id, status, revision, org_id, affected_org_id, owner_user_id,
			level, title, finding_key, asset_provider, asset_external_id, created_by)
		VALUES ($1, $2, 'remediating', 1, $3, $4, $5, 'high', $6, $7, 'cmdb', $8, $9)
		RETURNING id
	`, fmt.Sprintf("DS-%s-%s-%d", f.tag, finding, seq), f.familyID, f.mgmtOrg, f.targetOrg, f.ownerU,
		"risk "+finding, finding, fmt.Sprintf("asset-%d", seq), f.dispatcherM).Scan(&riskID))
	execMustNoError(t, db, ctx, `
		INSERT INTO risk_ticket_links (risk_id, group_id, owner_at_link) VALUES ($1, $2, $3)
	`, riskID, groupID, f.ownerU)
	return riskID
}

// transferUserPrimary 模拟 SetUserOrgs 全量覆盖后的终态：唯一 primary 行换到新部门。
func transferUserPrimary(t *testing.T, db *pgxpool.Pool, userID, newOrg int64) {
	t.Helper()
	execMustNoError(t, db, context.Background(), `
		DELETE FROM user_orgs WHERE user_id = $1 AND is_primary
	`, userID)
	execMustNoError(t, db, context.Background(), `
		INSERT INTO user_orgs (user_id, org_id, is_primary) VALUES ($1, $2, true)
	`, userID, newOrg)
}

func softDeleteUser(t *testing.T, db *pgxpool.Pool, userID int64) {
	t.Helper()
	execMustNoError(t, db, context.Background(),
		`UPDATE users SET deleted_at = NOW() WHERE id = $1`, userID)
}

type dsTodoWant struct {
	deptID   int64
	deptName string
	tickets  int64
	risks    int64
}

func requireTodoRows(t *testing.T, rows []DeptTodoRow, want []dsTodoWant) {
	t.Helper()
	require.Len(t, rows, len(want), "todo rows: %+v", rows)
	got := map[int64]DeptTodoRow{}
	for _, r := range rows {
		got[r.DeptID] = r
	}
	for _, w := range want {
		g := got[w.deptID]
		if w.deptName != "" { // 空串 = 不关心
			require.Equal(t, w.deptName, g.DeptName, "dept %d name", w.deptID)
		}
		require.Equal(t, w.tickets, g.OpenTickets, "dept %d tickets", w.deptID)
		require.Equal(t, w.risks, g.OpenRisks, "dept %d risks", w.deptID)
	}
}

type dsHistoryWant struct {
	ownerID   int64
	deptID    int64
	disp      int64
	tickets   int64
	risks     int64
	ownerName string
}

func requireHistoryRows(t *testing.T, rows []DeptHistoryRow, want []dsHistoryWant) {
	t.Helper()
	require.Len(t, rows, len(want), "history rows: %+v", rows)
	got := map[string]DeptHistoryRow{}
	for _, r := range rows {
		got[fmt.Sprintf("%d:%d", r.OwnerUserID, r.DeptID)] = r
	}
	for _, w := range want {
		key := fmt.Sprintf("%d:%d", w.ownerID, w.deptID)
		g := got[key]
		require.NotEmpty(t, g, "missing history row %s", key)
		require.Equal(t, w.disp, g.Dispatches, "row %s dispatches", key)
		require.Equal(t, w.tickets, g.Tickets, "row %s tickets", key)
		require.Equal(t, w.risks, g.Risks, "row %s risks", key)
		if w.ownerName != "" {
			require.Equal(t, w.ownerName, g.OwnerName, "row %s owner name", key)
		}
	}
}

// ---------- 用例 ----------

// 阶段 7 运行实测核心样例：同一人从甲部门调到乙部门后，
// 在办工单计入乙部门当前待办，原派单记录仍计甲部门历史，工单目标组织不随迁。
func TestDeptStats_CurrentTodoFollowsTransferredDept(t *testing.T) {
	db := dispatchTestDB
	tag := "transfer"
	f := newDeptStatsFixture(t, db, tag)
	stats := newDeptStatsReader(db)

	ticketID := f.insertTicket(t, db, "T-201 漏洞整改")
	groupID := f.insertGroup(t, db, "VULN-1", f.ownerU, f.deptA, ticketID)
	f.insertLinkedRisk(t, db, 1, "VULN-1", groupID)
	f.insertLinkedRisk(t, db, 2, "VULN-1", groupID)

	ctx := context.Background()
	todo, err := stats.CurrentTodoByDept(ctx, db, f.viewer)
	require.NoError(t, err)
	requireTodoRows(t, todo, []dsTodoWant{{deptID: f.deptA, deptName: "部门甲", tickets: 1, risks: 2}})

	history, err := stats.HistoryByDispatchDept(ctx, db, f.viewer)
	require.NoError(t, err)
	requireHistoryRows(t, history, []dsHistoryWant{{
		ownerID: f.ownerU, deptID: f.deptA, disp: 1, tickets: 1, risks: 2, ownerName: "责任人U",
	}})

	// U 从甲部门调入乙部门（SetUserOrgs 全量覆盖终态）。
	transferUserPrimary(t, db, f.ownerU, f.deptB)

	todo, err = stats.CurrentTodoByDept(ctx, db, f.viewer)
	require.NoError(t, err)
	requireTodoRows(t, todo, []dsTodoWant{{deptID: f.deptB, deptName: "部门乙", tickets: 1, risks: 2}})

	// 历史始终按派单成功时冻结的甲部门读取。
	history, err = stats.HistoryByDispatchDept(ctx, db, f.viewer)
	require.NoError(t, err)
	requireHistoryRows(t, history, []dsHistoryWant{{
		ownerID: f.ownerU, deptID: f.deptA, disp: 1, tickets: 1, risks: 2,
	}})

	// 工单目标组织不随迁：tickets.org_id 恒为派单目标组织。
	var ticketOrg int64
	require.NoError(t, db.QueryRow(ctx,
		`SELECT org_id FROM tickets WHERE id = $1`, ticketID).Scan(&ticketOrg))
	require.Equal(t, f.targetOrg, ticketOrg)
}

// 权限检查先于计数：同一份在办数据，真实可见性决定是否计入——
// 正例走实体透明读锚点、反例走 scope=assigned 窄查看者，均无放行桩。
func TestDeptStats_PermissionFiltersCounts(t *testing.T) {
	db := dispatchTestDB
	tag := "perm"
	f := newDeptStatsFixture(t, db, tag)
	stats := newDeptStatsReader(db)

	ticketID := f.insertTicket(t, db, "T-202 漏洞整改")
	groupID := f.insertGroup(t, db, "VULN-2", f.ownerU, f.deptA, ticketID)
	f.insertLinkedRisk(t, db, 1, "VULN-2", groupID)
	hiddenRisk := f.insertLinkedRisk(t, db, 2, "VULN-2", groupID)

	ctx := context.Background()
	execMustNoError(t, db, ctx, `UPDATE risk_items SET affected_org_id = NULL WHERE id = $1`, hiddenRisk)

	// 正例：目标组织实体透明读成员（真实 ResolveScope + GetFilter SQL）；
	// 同一份数据随后被窄查看者排除——正反同库同轮即互为非通用故障对照。
	todo, err := stats.CurrentTodoByDept(ctx, db, f.viewer)
	require.NoError(t, err)
	requireTodoRows(t, todo, []dsTodoWant{{deptID: f.deptA, deptName: "部门甲", tickets: 1, risks: 1}})

	// 反例：assigned 窄查看者非属主、非委托、非抄送、组织无链 → 零计数零行。
	todo, err = stats.CurrentTodoByDept(ctx, db, f.viewerNarrow)
	require.NoError(t, err)
	require.Empty(t, todo)
	history, err := stats.HistoryByDispatchDept(ctx, db, f.viewerNarrow)
	require.NoError(t, err)
	require.Empty(t, history)

	// 对照复查：正例视角计数稳定，排除反例行是权限排除而非数据消失。
	todo, err = stats.CurrentTodoByDept(ctx, db, f.viewer)
	require.NoError(t, err)
	requireTodoRows(t, todo, []dsTodoWant{{deptID: f.deptA, tickets: 1, risks: 1}})
}

// 调部门前后各派一单：当前待办合并现部门，历史按派单时点部门分桶。
func TestDeptStats_HistorySplitsByDispatchDept(t *testing.T) {
	db := dispatchTestDB
	tag := "split"
	f := newDeptStatsFixture(t, db, tag)
	stats := newDeptStatsReader(db)
	ctx := context.Background()

	t1 := f.insertTicket(t, db, "T-301 甲部门期派单")
	g1 := f.insertGroup(t, db, "VULN-A", f.ownerU, f.deptA, t1)
	f.insertLinkedRisk(t, db, 1, "VULN-A", g1)

	transferUserPrimary(t, db, f.ownerU, f.deptB)

	t2 := f.insertTicket(t, db, "T-302 乙部门期派单")
	g2 := f.insertGroup(t, db, "VULN-B", f.ownerU, f.deptB, t2)
	f.insertLinkedRisk(t, db, 2, "VULN-B", g2)

	todo, err := stats.CurrentTodoByDept(ctx, db, f.viewer)
	require.NoError(t, err)
	requireTodoRows(t, todo, []dsTodoWant{{deptID: f.deptB, deptName: "部门乙", tickets: 2, risks: 2}})

	history, err := stats.HistoryByDispatchDept(ctx, db, f.viewer)
	require.NoError(t, err)
	requireHistoryRows(t, history, []dsHistoryWant{
		{ownerID: f.ownerU, deptID: f.deptA, disp: 1, tickets: 1, risks: 1},
		{ownerID: f.ownerU, deptID: f.deptB, disp: 1, tickets: 1, risks: 1},
	})
}

// 未归属桶：无主部门/已软删责任人的在办票不消失，dept_id=0 归桶展示；
// 历史仍按派单时冻结部门读取，不因主体软删改写。
func TestDeptStats_UnattributedBucket(t *testing.T) {
	db := dispatchTestDB
	tag := "bucket"
	f := newDeptStatsFixture(t, db, tag)
	stats := newDeptStatsReader(db)
	ctx := context.Background()

	t1 := f.insertTicket(t, db, "T-401 无部门责任人票")
	g1 := f.insertGroup(t, db, "VULN-F", f.unaffiliated, 0, t1)
	f.insertLinkedRisk(t, db, 1, "VULN-F", g1)

	// 软删前：甲部门主责任人一单；软删后在办归 0 桶。
	t2 := f.insertTicket(t, db, "T-402 待软删责任人票")
	g2 := f.insertGroup(t, db, "VULN-G", f.ownerU, f.deptA, t2)
	f.insertLinkedRisk(t, db, 2, "VULN-G", g2)

	softDeleteUser(t, db, f.ownerU)

	todo, err := stats.CurrentTodoByDept(ctx, db, f.viewer)
	require.NoError(t, err)
	requireTodoRows(t, todo, []dsTodoWant{
		{deptID: 0, tickets: 2, risks: 2}, // 无部门责任人 + 已软删责任人
	})

	history, err := stats.HistoryByDispatchDept(ctx, db, f.viewer)
	require.NoError(t, err)
	requireHistoryRows(t, history, []dsHistoryWant{
		{ownerID: f.unaffiliated, deptID: 0, disp: 1, tickets: 1, risks: 1},
		{ownerID: f.ownerU, deptID: f.deptA, disp: 1, tickets: 1, risks: 1}, // 冻结部门不改写
	})
}

// fail-closed：可见性解析失败或未显式豁免的空谓词绝不计数（沿 IW4 哨兵纪律）。
// 此处放行形桩只作错误传播负例，正反例由 PermissionFiltersCounts 真实路径承担。
func TestDeptStats_FailClosedOnMissingPredicate(t *testing.T) {
	db := dispatchTestDB
	tag := "failclosed"
	f := newDeptStatsFixture(t, db, tag)
	ctx := context.Background()

	ticketID := f.insertTicket(t, db, "T-501 漏洞整改")
	groupID := f.insertGroup(t, db, "VULN-H", f.ownerU, f.deptA, ticketID)
	f.insertLinkedRisk(t, db, 1, "VULN-H", groupID)

	t.Run("resolver error never counts", func(t *testing.T) {
		stats := NewDeptStats(dsBrokenFilter{err: fmt.Errorf("resolver down")})
		todo, err := stats.CurrentTodoByDept(ctx, db, f.viewer)
		require.Error(t, err)
		require.Nil(t, todo)
		history, err := stats.HistoryByDispatchDept(ctx, db, f.viewer)
		require.Error(t, err)
		require.Nil(t, history)
	})
	t.Run("empty predicate without exemption is rejected", func(t *testing.T) {
		stats := NewDeptStats(dsBrokenFilter{})
		todo, err := stats.CurrentTodoByDept(ctx, db, f.viewer)
		require.ErrorIs(t, err, errcode.ErrForbidden)
		require.Nil(t, todo)
		history, err := stats.HistoryByDispatchDept(ctx, db, f.viewer)
		require.ErrorIs(t, err, errcode.ErrForbidden)
		require.Nil(t, history)
	})
	t.Run("no visible tickets yields empty rows", func(t *testing.T) {
		stats := newDeptStatsReader(db)
		// 无任何组织成员资格的查询者：锚点空、非属主/委托/抄送 → 谓词恒假非错误。
		var loner int64
		require.NoError(t, db.QueryRow(ctx, `
			INSERT INTO users (username, password, real_name, status) VALUES ($1, 'x', '孤立查询者', 1) RETURNING id
		`, "ds_loner_"+tag).Scan(&loner))
		todo, err := stats.CurrentTodoByDept(ctx, db, loner)
		require.NoError(t, err)
		require.Empty(t, todo)
	})
}

type dsBrokenFilter struct{ err error }

func (f dsBrokenFilter) VisibleTicketsWhere(context.Context, int64) (string, []any, bool, error) {
	if f.err != nil {
		return "", nil, false, f.err
	}
	return "", nil, false, nil // 未豁免的空谓词：非法形状，接缝必须 fail-closed
}
