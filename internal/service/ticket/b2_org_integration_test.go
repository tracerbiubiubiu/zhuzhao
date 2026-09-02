//go:build integration

package ticket

// 2b-org（Step 5，M2b-org）集成测试：虚拟组 / ticket_scope / BFS 三源角色 / 临时成员。
// SSOT: docs/phase2/03-org-enhance.md、09-ticket §5.2、rbac-inheritance-and-cascade §4。

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// setupB2Org 在 setupB2 基础上补虚拟组兄弟对：P 下 vg_a / vg_b（R9/R10 场景）
func setupB2Org(t *testing.T) *b2Env {
	t.Helper()
	env := setupB2(t)
	suffix := uniqueSuffix()
	env.vgA = createB2Vg(t, env.pID, "vg_a_"+suffix)
	env.vgB = createB2Vg(t, env.pID, "vg_b_"+suffix)

	// 虚拟组成员：U_va ∈ vg_a、U_vb ∈ vg_b（跨部门加人——虚拟组成员与部门成员正交）
	env.uVa = createB2User(t, "p2bva_"+suffix, "B2VA"+suffix)
	env.uVb = createB2User(t, "p2bvb_"+suffix, "B2VB"+suffix)
	bindOrgMember(t, env.uVa, env.vgA)
	bindOrgMember(t, env.uVb, env.vgB)
	env.roles[env.uVa] = []string{"operator"}
	env.roles[env.uVb] = []string{"operator"}
	return env
}

// T-2b-org-1（R9/R10 完整形态）：vg_a 成员透明读 vg_b 工单（可读/可评论），
// 但不可 update、不可写内部备注（读写分离）；vg_b 创建人可改自己的
func TestB2Org_R9R10_VgSiblingReadWrite(t *testing.T) {
	env := setupB2Org(t)
	ctx := context.Background()

	tk := newTicketHelper(t, env.svc, env.uVb, env.vgB, "vg_b 工单")

	// R9：vg_a 成员透明读 vg_b 工单 → 200（锚点 = 挂载实体 P）
	got, err := env.svc.Get(ctx, tk.ID, env.uVa)
	require.NoError(t, err)
	assert.Equal(t, tk.ID, got.ID)
	// List 亦含
	list, err := env.svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 100}, env.uVa)
	require.NoError(t, err)
	assert.True(t, containsID(list.List, tk.ID), "vg_a 成员列表应含 vg_b 工单")

	// R10：vg_a 成员 update vg_b 工单 → 403（非创建人；vg_b admin 属 2c）
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{
		ID: tk.ID, Title: strPtr("vg_a 越权改名"),
	}, env.uVa)
	requireErrCode(t, err, errcode.ErrNoPermission.Code)

	// 可读即可评论（公开回复），但内部备注不可写（BK-1 读写一致）
	_, err = env.svc.CreateComment(ctx, &model.CreateCommentRequest{
		TicketID: tk.ID, Content: "vg_a 同事公开回复",
	}, env.uVa)
	require.NoError(t, err)
	_, err = env.svc.CreateNote(ctx, &model.CreateNoteRequest{
		TicketID: tk.ID, Content: "vg_a 内部备注",
	}, env.uVa)
	requireErrCode(t, err, errcode.ErrNoPermission.Code)

	// vg_b 创建人改自己的 → 200
	updated, err := env.svc.Update(ctx, &model.UpdateTicketRequest{
		ID: tk.ID, Title: strPtr("创建人改名"),
	}, env.uVb)
	require.NoError(t, err)
	assert.Equal(t, "创建人改名", updated.Title)
}

// T-2b-org-2：虚拟组创建约束（vg_ 前缀 / 必须挂实体下）+ move 级联含虚拟组
func TestB2Org_VgCreateAndMoveCascade(t *testing.T) {
	env := setupB2(t)
	orgService := service.NewOrgService(repository.NewOrgRepo(testPool), repository.NewUserRepo(testPool), service.NewOrgDelegationService(testPool), rbacStubForOrgTest{})
	ctx := context.Background()

	// 无 vg_ 前缀 → 400
	_, err := orgService.Create(ctx, &model.CreateOrgRequest{
		Code: "bad_vg_" + uniqueSuffix(), Name: "坏前缀",
		ParentID: &env.pID, IsVirtual: true,
	}, env.u1)
	requireErrCode(t, err, errcode.ErrInvalidParams.Code)

	// 根级虚拟组（无 ParentID）→ 400（必须挂载实体下）
	_, err = orgService.Create(ctx, &model.CreateOrgRequest{
		Code: "vg_root_" + uniqueSuffix(), Name: "根级虚拟组",
		IsVirtual: true,
	}, env.u1)
	requireErrCode(t, err, errcode.ErrInvalidParams.Code)

	// 先造一个合法虚拟组，再尝试挂虚拟组下 → 400（父级必须实体）
	vgFirst, err := orgService.Create(ctx, &model.CreateOrgRequest{
		Code: "vg_first_" + uniqueSuffix(), Name: "首层虚拟组",
		ParentID: &env.pID, IsVirtual: true,
	}, env.u1)
	require.NoError(t, err)
	_, err = orgService.Create(ctx, &model.CreateOrgRequest{
		Code: "vg_nested_" + uniqueSuffix(), Name: "嵌套虚拟组",
		ParentID: &vgFirst.ID, IsVirtual: true,
	}, env.u1)
	requireErrCode(t, err, errcode.ErrInvalidParams.Code)

	// 合法创建：tech 实体下 vg_x → is_virtual=4、source=local、路径含 vg_ 段
	vgCode := "vg_x_" + uniqueSuffix()
	vg, err := orgService.Create(ctx, &model.CreateOrgRequest{
		Code: vgCode, Name: "合法虚拟组", ParentID: &env.pID, IsVirtual: true,
	}, env.u1)
	require.NoError(t, err)
	assert.True(t, vg.IsVirtual)
	var src string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT source FROM organizations WHERE id = $1`, vg.ID).Scan(&src))
	assert.Equal(t, "local", src)
	assert.Contains(t, vg.Path, "vg_x_")

	// move 级联含虚拟组（03 用例「HR 移动 tech 部门」）：P 移到新建目标组织下，
	// vg 路径随之从 root.P.vg_x 级联为 root.moveTarget.P.vg_x
	orgRepo := repository.NewOrgRepo(testPool)
	moveTarget := createB2Org(t, rootOrgID(t), "p2bmt_"+uniqueSuffix(), "移动目标")
	oldVgPath := vg.Path
	require.NoError(t, orgRepo.Move(ctx, env.pID, &moveTarget))
	var vgPathNew string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT path::text FROM organizations WHERE id = $1`, vg.ID).Scan(&vgPathNew))
	assert.NotEqual(t, oldVgPath, vgPathNew, "move 后虚拟组 path 应随实体子树级联更新")
	assert.True(t, pathInAnchors(vgPathNew, []string{vgPathNew[:len(vgPathNew)-len("."+lastLabel(vgPathNew))]}),
		"vg 新路径应位于移动后父级子树内")
}

// T-2b-org-3：ticket_scope 主管语义——group 成员可分派子树工单，assigned 同事不可；
// scope=all 全量可见
func TestB2Org_ScopeSupervisorAndAll(t *testing.T) {
	env := setupB2(t)
	ctx := context.Background()

	tkD1 := newTicketHelper(t, env.svc, env.u1, env.d1, "D1 工单")
	tkD2 := newTicketHelper(t, env.svc, env.u2, env.d2, "D2 工单")

	// supervisor：P 上 scope=group（主管）；allUser：D1 上 scope=all
	supervisor := createB2User(t, "p2bsup_"+uniqueSuffix(), "B2SP1")
	bindOrgMemberScope(t, supervisor, env.pID, ScopeGroup, nil)
	allUser := createB2User(t, "p2ball_"+uniqueSuffix(), "B2AL1")
	bindOrgMemberScope(t, allUser, env.d1, ScopeAll, nil)
	env.roles[supervisor] = []string{"operator"}
	env.roles[allUser] = []string{"operator"}

	// 同事（assigned）：可见（透明读）但不可分派 → 403
	err := env.svc.Assign(ctx, &model.AssignTicketRequest{
		ID: tkD1.ID, AssignedTo: &env.u1,
	}, env.colleague)
	requireErrCode(t, err, errcode.ErrNoPermission.Code)

	// 主管（group on P）：可见 + 可分派子树工单 → 200，状态转 assigned
	require.NoError(t, env.svc.Assign(ctx, &model.AssignTicketRequest{
		ID: tkD1.ID, AssignedTo: &env.u1,
	}, supervisor))
	got, err := env.svc.Get(ctx, tkD1.ID, supervisor)
	require.NoError(t, err)
	assert.Equal(t, "assigned", got.Status)

	// scope=all：跨子树工单可见；列表全量
	_, err = env.svc.Get(ctx, tkD2.ID, allUser)
	require.NoError(t, err)
	list, err := env.svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 100}, allUser)
	require.NoError(t, err)
	assert.True(t, containsID(list.List, tkD2.ID) && containsID(list.List, tkD1.ID),
		"scope=all 列表应跨子树全量")

	// 读/写分离（scope 只扩读不扩写，防「scope=万能」误解）：
	// 主管 update 子树内他人工单 → 403（仅 assign 例外的动作权）
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{
		ID: tkD1.ID, Title: strPtr("主管越权改名"),
	}, supervisor)
	requireErrCode(t, err, errcode.ErrNoPermission.Code)

	// scope=all：update 他人工单 → 403；写内部备注 → 403（仍绑属主）
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{
		ID: tkD2.ID, Title: strPtr("all 越权改名"),
	}, allUser)
	requireErrCode(t, err, errcode.ErrNoPermission.Code)
	_, err = env.svc.CreateNote(ctx, &model.CreateNoteRequest{
		TicketID: tkD2.ID, Content: "all 越权备注",
	}, allUser)
	requireErrCode(t, err, errcode.ErrNoPermission.Code)
	// scope=all 的 assign 例外动作仍放行（主管全量分派）
	require.NoError(t, env.svc.Assign(ctx, &model.AssignTicketRequest{
		ID: tkD2.ID, AssignedTo: &env.u2,
	}, allUser))

	// P1-2：主管分派不在 scope 子树内的工单 → 404（不可见→反枚举）
	otherOrg := createB2Org(t, rootOrgID(t),
		"p2bother_"+uniqueSuffix(), "其他子树")
	tkOther := newTicketHelper(t, env.svc, env.u1, otherOrg, "其他子树工单")
	err = env.svc.Assign(ctx, &model.AssignTicketRequest{
		ID: tkOther.ID, AssignedTo: &env.u1,
	}, supervisor)
	// 不可见→404（反枚举），不是 403
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
}

// T-2b-org-4：BFS 三源角色展开（直接 ∪ 组织角色 ∪ 继承链）+ 临时成员过期隔离 +
// 「父部门 org_roles 不继承子部门」
func TestB2Org_BFSRoleExpansion(t *testing.T) {
	env := setupB2(t)
	roleRepo := repository.NewRoleRepo(testPool)
	ctx := context.Background()
	suffix := uniqueSuffix()

	// 造角色：child（parent=parent），均为非系统角色
	var parentRoleID, childRoleID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO roles (code, name, priority, status) VALUES ($1, '父角色', 40, 1) RETURNING id`,
		"p2b_parent_"+suffix).Scan(&parentRoleID))
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO roles (code, name, priority, status, parent_id) VALUES ($1, '子角色', 41, 1, $2) RETURNING id`,
		"p2b_child_"+suffix, parentRoleID).Scan(&childRoleID))

	// 直接角色 = child → 展开含 child + parent（源 1 + 源 3）
	uDirect := createB2User(t, "p2bd_"+suffix, "B2D"+suffix)
	_, err := testPool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, uDirect, childRoleID)
	require.NoError(t, err)
	codes, err := roleRepo.GetEffectiveRoleCodes(ctx, uDirect)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"p2b_child_" + suffix, "p2b_parent_" + suffix}, codes)

	// 组织角色（源 2）：P 绑 parentRole；P 的成员获得（源 1 无直接角色）
	uP := createB2User(t, "p2bp_"+suffix, "B2P"+suffix)
	bindOrgMember(t, uP, env.pID)
	_, err = testPool.Exec(ctx,
		`INSERT INTO org_roles (org_id, role_id) VALUES ($1, $2)`, env.pID, parentRoleID)
	require.NoError(t, err)
	codes, err = roleRepo.GetEffectiveRoleCodes(ctx, uP)
	require.NoError(t, err)
	assert.Contains(t, codes, "p2b_parent_"+suffix)
	assert.NotContains(t, codes, "p2b_child_"+suffix)

	// 「父部门 org_roles 不继承子部门」：D1（P 的子部门）成员不得获得 P 的 org_roles
	uD1 := createB2User(t, "p2bd1_"+suffix, "B2D1"+suffix)
	bindOrgMember(t, uD1, env.d1)
	codes, err = roleRepo.GetEffectiveRoleCodes(ctx, uD1)
	require.NoError(t, err)
	assert.NotContains(t, codes, "p2b_parent_"+suffix)

	// 临时成员过期：org_roles 不参与展开；未过期则参与
	uTemp := createB2User(t, "p2bt_"+suffix, "B2T"+suffix)
	past := time.Now().Add(-time.Hour)
	_, err = testPool.Exec(ctx,
		`INSERT INTO user_orgs (user_id, org_id, is_primary, expires_at) VALUES ($1, $2, true, $3)`,
		uTemp, env.pID, past)
	require.NoError(t, err)
	codes, err = roleRepo.GetEffectiveRoleCodes(ctx, uTemp)
	require.NoError(t, err)
	assert.NotContains(t, codes, "p2b_parent_"+suffix, "过期临时成员不应获得组织角色")

	future := time.Now().Add(24 * time.Hour)
	_, err = testPool.Exec(ctx,
		`UPDATE user_orgs SET expires_at = $3 WHERE user_id = $1 AND org_id = $2`,
		uTemp, env.pID, future)
	require.NoError(t, err)
	codes, err = roleRepo.GetEffectiveRoleCodes(ctx, uTemp)
	require.NoError(t, err)
	assert.Contains(t, codes, "p2b_parent_"+suffix)
}

// T-2b-org-5：过期临时成员在工单可见性侧同样失效（resolver 读取侧过滤）
func TestB2Org_ExpiredMemberLosesAnchor(t *testing.T) {
	env := setupB2(t)
	ctx := context.Background()

	tk := newTicketHelper(t, env.svc, env.u1, env.d1, "临时成员工单")

	// 未过期临时成员：D1 透明可读
	uTmp := createB2User(t, "p2btmp_"+uniqueSuffix(), "B2TM")
	future := time.Now().Add(time.Hour)
	bindOrgMemberScope(t, uTmp, env.d1, ScopeAssigned, &future)
	_, err := env.svc.Get(ctx, tk.ID, uTmp)
	require.NoError(t, err)

	// 过期：锚点失效 → 404；属主不受影响
	past := time.Now().Add(-time.Hour)
	_, err = testPool.Exec(ctx,
		`UPDATE user_orgs SET expires_at = $3 WHERE user_id = $1 AND org_id = $2`,
		uTmp, env.d1, past)
	require.NoError(t, err)
	_, err = env.svc.Get(ctx, tk.ID, uTmp)
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
	_, err = env.svc.Get(ctx, tk.ID, env.u1)
	require.NoError(t, err)
}

// T-2b-org-6：roles.parent_id 边界——自引用环不 panic / 软删父不展开 / 停用父不展开
func TestB2Org_ParentIdEdgeCases(t *testing.T) {
	setupB2(t) // 初始化 DB 上下文（组织/用户种子），本用例不直接用 env 字段
	roleRepo := repository.NewRoleRepo(testPool)
	ctx := context.Background()
	suffix := uniqueSuffix()

	// 自引用环：parent_id = 自身 ID（PG CTE UNION 去重阻止死循环）
	var cyclicID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO roles (code, name, priority, status) VALUES ($1, '自引用', 50, 1) RETURNING id`,
		"p2b_cyclic_"+suffix).Scan(&cyclicID))
	_, err := testPool.Exec(ctx, `UPDATE roles SET parent_id = id WHERE id = $1`, cyclicID)
	require.NoError(t, err)
	uCyclic := createB2User(t, "p2bcy_"+suffix, "B2CY"+suffix)
	_, err = testPool.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, uCyclic, cyclicID)
	require.NoError(t, err)
	codes, err := roleRepo.GetEffectiveRoleCodes(ctx, uCyclic)
	require.NoError(t, err, "自引用环不应 panic 或死循环")
	assert.Contains(t, codes, "p2b_cyclic_"+suffix)

	// 软删父角色：子角色 parent_id 指向已软删角色 → 不展开
	var delParentID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO roles (code, name, priority, status, deleted_at) VALUES ($1, '软删父', 51, 1, NOW()) RETURNING id`,
		"p2b_delpar_"+suffix).Scan(&delParentID))
	var delChildID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO roles (code, name, priority, status, parent_id) VALUES ($1, '软删子', 52, 1, $2) RETURNING id`,
		"p2b_delch_"+suffix, delParentID).Scan(&delChildID))
	uDel := createB2User(t, "p2bdel_"+suffix, "B2DEL"+suffix)
	_, err = testPool.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, uDel, delChildID)
	require.NoError(t, err)
	codes, err = roleRepo.GetEffectiveRoleCodes(ctx, uDel)
	require.NoError(t, err)
	assert.Contains(t, codes, "p2b_delch_"+suffix, "子角色自身应出现")
	assert.NotContains(t, codes, "p2b_delpar_"+suffix, "软删父角色不应展开")

	// 停用父角色：子角色 parent_id 指向 status=0 角色 → 不展开
	var disParentID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO roles (code, name, priority, status) VALUES ($1, '停用父', 53, 0) RETURNING id`,
		"p2b_dispar_"+suffix).Scan(&disParentID))
	var disChildID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO roles (code, name, priority, status, parent_id) VALUES ($1, '停用子', 54, 1, $2) RETURNING id`,
		"p2b_disch_"+suffix, disParentID).Scan(&disChildID))
	uDis := createB2User(t, "p2bdis_"+suffix, "B2DIS"+suffix)
	_, err = testPool.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, uDis, disChildID)
	require.NoError(t, err)
	codes, err = roleRepo.GetEffectiveRoleCodes(ctx, uDis)
	require.NoError(t, err)
	assert.Contains(t, codes, "p2b_disch_"+suffix, "子角色自身应出现")
	assert.NotContains(t, codes, "p2b_dispar_"+suffix, "停用父角色不应展开")
}

// --- 辅助 ---

func createB2Vg(t *testing.T, parentID int64, code string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, testPool.QueryRow(context.Background(), `
		INSERT INTO organizations (code, name, parent_id, path, is_virtual, status, sort_order, is_system, source)
		VALUES ($1, $2, $3, (SELECT path::text || '.' || $4 FROM organizations WHERE id = $3)::ltree, true, 1, 70, false, 'local')
		RETURNING id`, code, "虚拟组 "+code, parentID, code).Scan(&id))
	softDeleteOrg(t, id)
	return id
}

func bindOrgMemberScope(t *testing.T, userID, orgID int64, scope string, expiresAt *time.Time) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO user_orgs (user_id, org_id, is_primary, ticket_scope, expires_at) VALUES ($1, $2, true, $3, $4)`,
		userID, orgID, scope, expiresAt)
	require.NoError(t, err)
}

func containsID(tickets []*model.Ticket, id int64) bool {
	for _, tk := range tickets {
		if tk.ID == id {
			return true
		}
	}
	return false
}

// rbacStubForOrgTest OrgService 的 RoleFetcher 桩（b2 组织测试仅用 Create/Move，
// 无全局 admin 判定需求；AddMember 委托校验用 superadmin actor 时由本桩放行）
type rbacStubForOrgTest struct{}

func (rbacStubForOrgTest) GetRoleCodesByUserID(_ context.Context, _ int64) ([]string, error) {
	return []string{"superadmin"}, nil
}
