//go:build integration

package service_test

// 2c Step 8（M2c-1）集成测试：组织委托 D1–D6。
// SSOT: docs/phase2/04-org-delegation.md §3/§7。
// 角色经真实 RBACService（user_roles 表）注入——与生产 RoleFetcher 同路径。

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// dEnv 2c 委托测试环境：root 下实体 P，P 下虚拟组 VG；owner/admin/member2/member1 四类用户
type dEnv struct {
	orgSvc *service.OrgService
	deleg  *service.OrgDelegationService
	pID    int64
	vgID   int64
	owner  int64 // VG 的 owner
	admin  int64 // VG 的 admin
	mem1   int64 // VG 的 member（D4 被移除对象）
	mem2   int64 // VG 的 member（旁观）
	admin2 int64 // VG 的另一 admin（D5 被移除对象）
	super  int64 // 全局 superadmin（SAT 语义）
}

func setupDelegation(t *testing.T) *dEnv {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1e9)

	deleg := service.NewOrgDelegationService(testPool)
	rbac := service.NewRBACService(
		repository.NewRoleRepo(testPool), repository.NewUserRepo(testPool),
		repository.NewMenuRepo(testPool), rbacTestEnforcer(t))
	orgSvc := service.NewOrgService(repository.NewOrgRepo(testPool), repository.NewUserRepo(testPool), deleg, rbac)

	// 组织树：root > P(实体) > vg(虚拟组)
	var pID, vgID int64
	// path 整串在 Go 侧拼好（占位符单上下文，规避 pgx 42P08 多类型推断）
	pCode := "p2c_p_" + suffix
	vgCode := "vg_2c_" + suffix
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, parent_id, path, org_type, status, sort_order, is_system)
		VALUES ($1, '2c P', 1, $2::ltree, 3, 1, 80, false)
		RETURNING id`, pCode, "root."+pCode).Scan(&pID))
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, parent_id, path, org_type, status, sort_order, is_system)
		VALUES ($1, '2c VG', $2, $3::ltree, 4, 1, 80, false)
		RETURNING id`, vgCode, pID, "root."+pCode+"."+vgCode).Scan(&vgID))

	// 角色：本测试容器无 000002 种子，自建 superadmin/viewer（幂等；code 精确匹配
	// isGlobalOrgAdmin 判定与既有 viewer 语义）
	_, err := testPool.Exec(ctx, `
		INSERT INTO roles (code, name, priority, status, is_system) VALUES
		('superadmin', '超级管理员', 1, 1, true),
		('viewer', '只读', 40, 1, false)
		ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	var superRole, viewerRole int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT id FROM roles WHERE code='superadmin'`).Scan(&superRole))
	require.NoError(t, testPool.QueryRow(ctx, `SELECT id FROM roles WHERE code='viewer'`).Scan(&viewerRole))

	mkuser := func(name string) int64 {
		var id int64
		require.NoError(t, testPool.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO users (username, password, employee_no, status) VALUES ('%s', 'hash', '%s', 1)
			ON CONFLICT (employee_no) WHERE employee_no IS NOT NULL AND employee_no <> '' AND deleted_at IS NULL
				DO UPDATE SET password = EXCLUDED.password
			RETURNING id`, name, "E"+name)).Scan(&id))
		return id
	}
	super := mkuser("p2csup_" + suffix)
	_, err = testPool.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, super, superRole)
	require.NoError(t, err)

	mkmember := func(name, role string) int64 {
		id := mkuser(name)
		_, err := testPool.Exec(ctx, `
			INSERT INTO user_orgs (user_id, org_id, is_primary, org_member_role) VALUES ($1, $2, false, $3)`,
			id, vgID, role)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, viewerRole)
		require.NoError(t, err)
		return id
	}

	return &dEnv{
		orgSvc: orgSvc, deleg: deleg, pID: pID, vgID: vgID,
		owner:  mkmember("p2cown_"+suffix, "member"),
		admin:  mkmember("p2cadm_"+suffix, "admin"),
		mem1:   mkmember("p2cmem1_"+suffix, "member"),
		mem2:   mkmember("p2cmem2_"+suffix, "member"),
		admin2: mkmember("p2cadm2_"+suffix, "admin"),
		super:  super,
	}
}

// D1 设置虚拟组 owner：owner_user_ids 含负责人；双轨对齐 org_member_role=owner
func TestDelegation_D1_SetOwners(t *testing.T) {
	env := setupDelegation(t)
	ctx := context.Background()

	org, err := env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: env.vgID, OwnerUserIDs: []int64{env.owner}}, env.super)
	require.NoError(t, err)
	assert.Contains(t, org.OwnerUserIDs, env.owner)

	var role string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT org_member_role FROM user_orgs WHERE org_id=$1 AND user_id=$2`,
		env.vgID, env.owner).Scan(&role))
	assert.Equal(t, "owner", role, "双轨对齐：owner_user_ids 与 org_member_role 同步")

	// EffectiveOrgPriority：owner → 1；admin → 10；member/非成员 → 20
	p, err := env.deleg.EffectiveOrgPriority(ctx, env.owner, env.vgID)
	require.NoError(t, err)
	assert.Equal(t, service.OrgRoleOwnerPriority, p)

	// 移出列表降级：换 owner 后原 owner 降为 member（仍保留成员行）
	_, err = env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: env.vgID, OwnerUserIDs: []int64{env.mem2}}, env.super)
	require.NoError(t, err)
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT org_member_role FROM user_orgs WHERE org_id=$1 AND user_id=$2`,
		env.vgID, env.owner).Scan(&role))
	assert.Equal(t, "member", role, "移出 owner 列表应降级为 member（保留成员关系）")

	// 非 owner 非 global 调用 → 50010
	_, err = env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: env.vgID, OwnerUserIDs: []int64{env.mem2}}, env.mem1)
	requireErrCode(t, err, errcode.ErrNotOrgOwner)
}

// D2/D3 任命组内角色：owner 任命 admin → 200；admin 调用 → 50010；请求 owner → 400
func TestDelegation_D2D3_SetMemberRole(t *testing.T) {
	env := setupDelegation(t)
	ctx := context.Background()
	_, err := env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: env.vgID, OwnerUserIDs: []int64{env.owner}}, env.super)
	require.NoError(t, err)

	// D2：owner 任命 member → admin
	require.NoError(t, env.orgSvc.SetMemberRole(ctx, &model.SetOrgMemberRoleRequest{OrgID: env.vgID, UserID: env.mem1, OrgMemberRole: "admin"}, env.owner))
	var role string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT org_member_role FROM user_orgs WHERE org_id=$1 AND user_id=$2`, env.vgID, env.mem1).Scan(&role))
	assert.Equal(t, "admin", role)

	// D3：admin 调用 SetMemberRole → 50010（admin 不可任命）
	err = env.orgSvc.SetMemberRole(ctx, &model.SetOrgMemberRoleRequest{OrgID: env.vgID, UserID: env.mem2, OrgMemberRole: "admin"}, env.admin)
	requireErrCode(t, err, errcode.ErrNotOrgOwner)

	// owner 仅经 SetOwners：请求 owner → 400
	err = env.orgSvc.SetMemberRole(ctx, &model.SetOrgMemberRoleRequest{OrgID: env.vgID, UserID: env.mem2, OrgMemberRole: "owner"}, env.owner)
	requireErrCode(t, err, errcode.ErrInvalidParams)

	// 派生 owner（owner_user_ids）角色不可被改 → 50009
	err = env.orgSvc.SetMemberRole(ctx, &model.SetOrgMemberRoleRequest{OrgID: env.vgID, UserID: env.owner, OrgMemberRole: "member"}, env.super)
	requireErrCode(t, err, errcode.ErrCannotManageOrgMember)

	// 非成员目标 → 50007
	err = env.orgSvc.SetMemberRole(ctx, &model.SetOrgMemberRoleRequest{OrgID: env.vgID, UserID: env.super, OrgMemberRole: "admin"}, env.owner)
	requireErrCode(t, err, errcode.ErrNotOrgMember)
}

// D4/D5 移除成员：admin 移除 member → 200；admin 移除 admin → 50009；owner 移除 admin → 200；member 无权 → 70001
func TestDelegation_D4D5_RemoveMember(t *testing.T) {
	env := setupDelegation(t)
	ctx := context.Background()
	_, err := env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: env.vgID, OwnerUserIDs: []int64{env.owner}}, env.super)
	require.NoError(t, err)

	// D4：admin 移除 member
	require.NoError(t, env.orgSvc.RemoveMember(ctx, &model.OrgMemberRequest{OrgID: env.vgID, UserID: env.mem1}, env.admin))

	// D5：admin 移除另一 admin → 50009
	err = env.orgSvc.RemoveMember(ctx, &model.OrgMemberRequest{OrgID: env.vgID, UserID: env.admin2}, env.admin)
	requireErrCode(t, err, errcode.ErrCannotManageOrgMember)

	// owner 移除 admin → 200
	require.NoError(t, env.orgSvc.RemoveMember(ctx, &model.OrgMemberRequest{OrgID: env.vgID, UserID: env.admin2}, env.owner))

	// member 移除他人 → 70001
	err = env.orgSvc.RemoveMember(ctx, &model.OrgMemberRequest{OrgID: env.vgID, UserID: env.mem2}, env.mem2)
	requireErrCode(t, err, errcode.ErrNoPermission)

	// 全局管理员绕过组内校验
	require.NoError(t, env.orgSvc.RemoveMember(ctx, &model.OrgMemberRequest{OrgID: env.vgID, UserID: env.mem2}, env.super))
}

// D6 owner 删空虚拟组：200；有成员 → 50005；member 删 → 70001；实体删除仅全局
func TestDelegation_D6_OwnerDeleteVg(t *testing.T) {
	env := setupDelegation(t)
	ctx := context.Background()
	_, err := env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: env.vgID, OwnerUserIDs: []int64{env.owner}}, env.super)
	require.NoError(t, err)

	// member 尝试删 → 70001
	err = env.orgSvc.DeleteOrgDelegated(ctx, env.vgID, env.mem2)
	requireErrCode(t, err, errcode.ErrNoPermission)

	// 有成员 → 50005（与 Phase 1 一致）
	err = env.orgSvc.DeleteOrgDelegated(ctx, env.vgID, env.owner)
	requireErrCode(t, err, errcode.ErrOrgHasMembers)

	// 清空非 owner 成员后 owner 删 → 200（owner 行为派生数据，删除时自动失效）
	for _, uid := range []int64{env.admin, env.mem1, env.mem2, env.admin2} {
		require.NoError(t, env.orgSvc.RemoveMember(ctx, &model.OrgMemberRequest{OrgID: env.vgID, UserID: uid}, env.super))
	}
	require.NoError(t, env.orgSvc.DeleteOrgDelegated(ctx, env.vgID, env.owner))

	// 实体组织删除仅全局（owner 也 70001）
	err = env.orgSvc.DeleteOrgDelegated(ctx, env.pID, env.owner)
	requireErrCode(t, err, errcode.ErrNoPermission)
}

// AddMember 扩展（04 §3.4）：owner 可指定 admin 入组；admin 指定 admin → 50008
func TestDelegation_AddMemberRole(t *testing.T) {
	env := setupDelegation(t)
	ctx := context.Background()
	_, err := env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: env.vgID, OwnerUserIDs: []int64{env.owner}}, env.super)
	require.NoError(t, err)

	var outsider int64
	require.NoError(t, testPool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO users (username, password, employee_no, status) VALUES ('p2cout_%d', 'hash', 'E2COUT%d', 1)
		RETURNING id`, time.Now().UnixNano()%1e9, time.Now().UnixNano()%1e9)).Scan(&outsider))

	// owner 直接以 admin 身份拉人入组
	require.NoError(t, env.orgSvc.AddMember(ctx, &model.OrgMemberRequest{OrgID: env.vgID, UserID: outsider, OrgMemberRole: "admin"}, env.owner))
	var role string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT org_member_role FROM user_orgs WHERE org_id=$1 AND user_id=$2`, env.vgID, outsider).Scan(&role))
	assert.Equal(t, "admin", role)

	// admin 拉人指定 admin → 50008
	var outsider2 int64
	require.NoError(t, testPool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO users (username, password, employee_no, status) VALUES ('p2cout2_%d', 'hash', 'E2COUT2%d', 1)
		RETURNING id`, time.Now().UnixNano()%1e9, time.Now().UnixNano()%1e9)).Scan(&outsider2))
	err = env.orgSvc.AddMember(ctx, &model.OrgMemberRequest{OrgID: env.vgID, UserID: outsider2, OrgMemberRole: "admin"}, env.admin)
	requireErrCode(t, err, errcode.ErrCannotAssignHigherOrgMemberRole)

	// admin 拉 member → 200（D4 路径）
	require.NoError(t, env.orgSvc.AddMember(ctx, &model.OrgMemberRequest{OrgID: env.vgID, UserID: outsider2, OrgMemberRole: "member"}, env.admin))

	// member 拉人 → 70001
	var outsider3 int64
	require.NoError(t, testPool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO users (username, password, employee_no, status) VALUES ('p2cout3_%d', 'hash', 'E2COUT3%d', 1)
		RETURNING id`, time.Now().UnixNano()%1e9, time.Now().UnixNano()%1e9)).Scan(&outsider3))
	err = env.orgSvc.AddMember(ctx, &model.OrgMemberRequest{OrgID: env.vgID, UserID: outsider3}, env.mem1)
	requireErrCode(t, err, errcode.ErrNoPermission)
}

// P0 回归：RemoveMember 移除 owner 后，owner_user_ids 同步清理且残留权限失效
func TestDelegation_RemoveOwnerCleansOwnerUserIDs(t *testing.T) {
	env := setupDelegation(t)
	ctx := context.Background()
	_, err := env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: env.vgID, OwnerUserIDs: []int64{env.owner}}, env.super)
	require.NoError(t, err)

	// 全局管理员直删 owner 成员行（模拟「移除 owner」管理操作）
	require.NoError(t, env.orgSvc.RemoveMember(ctx, &model.OrgMemberRequest{OrgID: env.vgID, UserID: env.owner}, env.super))

	// 双轨另一侧必须同步清理
	var ids string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT owner_user_ids::text FROM organizations WHERE id=$1`, env.vgID).Scan(&ids))
	assert.NotContains(t, ids, fmt.Sprint(env.owner), "owner_user_ids 应同步移除被删 owner")

	// 残留权限失效：被移除者调 SetMemberRole → 非 owner → 70001（若 50010 则说明仍是 admin 档残留）
	err = env.orgSvc.SetMemberRole(ctx, &model.SetOrgMemberRoleRequest{OrgID: env.vgID, UserID: env.mem1, OrgMemberRole: "admin"}, env.owner)
	requireErrCode(t, err, errcode.ErrNoPermission)
}

// P0 同源回归：SetUserOrgs 全量覆盖把用户移出某 org 后，owner_user_ids 同步清理
func TestDelegation_SetUserOrgsCleansOwnerUserIDs(t *testing.T) {
	env := setupDelegation(t)
	ctx := context.Background()
	_, err := env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: env.vgID, OwnerUserIDs: []int64{env.owner}}, env.super)
	require.NoError(t, err)

	// 用户侧全量覆盖：把 owner 的组织清空（移出 vg 成员身份）
	require.NoError(t, env.orgSvc.SetUserOrgs(ctx, &model.SetUserOrgsRequest{
		UserID: env.owner, OrgIDs: []int64{}, PrimaryOrgID: nil,
	}))

	// owner_user_ids 必须同步清理（否则残留 effective owner）
	var ids string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT owner_user_ids::text FROM organizations WHERE id=$1`, env.vgID).Scan(&ids))
	assert.NotContains(t, ids, fmt.Sprint(env.owner), "SetUserOrgs 移出成员后应同步清理 owner_user_ids")

	// 残留权限失效：被移出者调 SetMemberRole → 非 owner/admin → 70001
	err = env.orgSvc.SetMemberRole(ctx, &model.SetOrgMemberRoleRequest{OrgID: env.vgID, UserID: env.mem1, OrgMemberRole: "admin"}, env.owner)
	requireErrCode(t, err, errcode.ErrNoPermission)
}

// P0 同源回归：删除用户后，其 owner_user_ids 引用在所有组织被清理
func TestDelegation_SoftDeleteCleansOwnerUserIDs(t *testing.T) {
	env := setupDelegation(t)
	ctx := context.Background()
	_, err := env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: env.vgID, OwnerUserIDs: []int64{env.owner}}, env.super)
	require.NoError(t, err)

	// 删除 owner 用户（repo 层，验证 SoftDeleteTx 的 owner 引用清理）
	require.NoError(t, repository.NewUserRepo(testPool).SoftDelete(ctx, env.owner))

	var ids string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT owner_user_ids::text FROM organizations WHERE id=$1`, env.vgID).Scan(&ids))
	assert.NotContains(t, ids, fmt.Sprint(env.owner), "SoftDelete 后 owner_user_ids 应清理被删用户引用")
}

// ---------- IW3/BK-12：组织 ↔ 角色绑定（org_roles 写侧） ----------

func rbacForBK12(t *testing.T) *service.RBACService {
	t.Helper()
	return service.NewRBACService(
		repository.NewRoleRepo(testPool), repository.NewUserRepo(testPool),
		repository.NewMenuRepo(testPool), rbacTestEnforcer(t))
}

var _ = fmt.Sprintf

func TestBK12_OrgRoleBinding(t *testing.T) {
	env := setupDelegation(t)
	ctx := context.Background()

	// 角色码基线：mem1 绑定 vg 前 viewer，无其他
	before, err := env.orgSvc.ListOrgRoles(ctx, env.vgID, env.super)
	require.NoError(t, err)
	assert.Empty(t, before, "初始无绑定")

	// 专用组织角色（避免与 setupDelegation 直赋的 viewer 混淆 BFS 源 1/2）
	orgRole, err := rbacForBK12(t).CreateRole(ctx, &model.CreateRoleRequest{
		Code: "bk12_org_" + fmt.Sprintf("%d", time.Now().UnixNano()%1e9), Name: "组织角色", Priority: 40}, env.super)
	require.NoError(t, err)

	// org admin 绑定 → 403（仅全局管理员：org_roles 赋出全局 Casbin 角色）
	err = env.orgSvc.BindOrgRole(ctx, &model.BindOrgRoleRequest{OrgID: env.vgID, RoleID: orgRole.ID}, env.admin)
	requireErrCode(t, err, errcode.ErrNoPermission)

	// 全局管理员绑定 ✓ → 列表含该角色 → BFS 源 2 生效：vg 成员有效角色包含
	require.NoError(t, env.orgSvc.BindOrgRole(ctx,
		&model.BindOrgRoleRequest{OrgID: env.vgID, RoleID: orgRole.ID}, env.super))
	list, err := env.orgSvc.ListOrgRoles(ctx, env.vgID, env.admin)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	rbac := rbacForBK12(t)
	codes, err := rbac.GetRoleCodesByUserID(ctx, env.mem1)
	require.NoError(t, err)
	assert.Contains(t, codes, orgRole.Code, "BFS 源 2：组织绑定角色后成员有效角色应包含")

	// 绑定系统角色 → 400
	var sysRole int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT id FROM roles WHERE code='superadmin'`).Scan(&sysRole))
	err = env.orgSvc.BindOrgRole(ctx,
		&model.BindOrgRoleRequest{OrgID: env.vgID, RoleID: sysRole}, env.super)
	requireErrCode(t, err, errcode.ErrInvalidParams)

	// 解绑 → 列表空 → BFS 失去组织角色
	require.NoError(t, env.orgSvc.UnbindOrgRole(ctx,
		&model.BindOrgRoleRequest{OrgID: env.vgID, RoleID: orgRole.ID}, env.super))
	codes, err = rbac.GetRoleCodesByUserID(ctx, env.mem1)
	require.NoError(t, err)
	assert.NotContains(t, codes, orgRole.Code, "解绑后 BFS 源 2 应失效")

	// 再解绑 → 未绑定 404 语义
	err = env.orgSvc.UnbindOrgRole(ctx,
		&model.BindOrgRoleRequest{OrgID: env.vgID, RoleID: orgRole.ID}, env.super)
	requireErrCode(t, err, errcode.ErrNotFound)
}

// ---------- P1-1 语义锁定：管理守卫只认直接角色（能力面 = 展开集，设计语义固化） ----------

func TestBK12_DirectVsExpanded(t *testing.T) {
	env := setupDelegation(t)
	ctx := context.Background()
	rbac := rbacForBK12(t)
	userRepo := repository.NewUserRepo(testPool)
	orgRole, err := rbac.CreateRole(ctx, &model.CreateRoleRequest{
		Code: "bk12fork_" + fmt.Sprintf("%d", time.Now().UnixNano()%1e9), Name: "组织绑定角色", Priority: 30}, env.super)
	require.NoError(t, err)

	// 纯组织绑定用户（有 user_orgs 成员行、无任何 user_roles 直赋）
	var pureUser int64
	require.NoError(t, testPool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO users (username, password, employee_no, status) VALUES ('bk12fork_%s', 'hash', 'EBK12FORK', 1)
		RETURNING id`, fmt.Sprintf("%d", time.Now().UnixNano()%1e9))).Scan(&pureUser))
	_, err = testPool.Exec(ctx, `
		INSERT INTO user_orgs (user_id, org_id, is_primary, org_member_role, ticket_scope)
		VALUES ($1, $2, false, 'member', 'assigned')`, pureUser, env.vgID)
	require.NoError(t, err)
	// 仅组织绑定（BFS 源 2），无任何直赋角色
	require.NoError(t, env.orgSvc.BindOrgRole(ctx,
		&model.BindOrgRoleRequest{OrgID: env.vgID, RoleID: orgRole.ID}, env.super))

	// 展开集（L1/能力面）：含绑定角色
	codes, err := rbac.GetRoleCodesByUserID(ctx, pureUser)
	require.NoError(t, err)
	assert.Contains(t, codes, orgRole.Code, "展开集应含组织绑定角色")

	// 直接角色（L3 管理守卫的数据源）：不含组织绑定角色——语义分叉为设计选择，
	// 由本测试锁定（防提权守卫不因组织绑定抬升档位）
	direct, err := userRepo.GetRoles(ctx, pureUser)
	require.NoError(t, err)
	assert.Empty(t, direct, "GetRoles 只认直接角色：组织绑定不经 user_roles")
}
