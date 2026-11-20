//go:build integration

// zz_rem_roles_guard：组织删除守卫 × 同源角色生命周期回归（2026-11-20 审计续批）。
//
// 审计指出：roles/delete 可软删仍被 org_roles 绑定的角色——之后该组织删除命中
// 50014（ErrOrgHasOrgRoles），而历史 deleted role 的解绑入口不可达
// （authorizeOrgRoleAssignment → GetRoleForOrgAssignment 只读未删角色），死锁闭环。
//
// 收口政策（沿现有角色在用政策，角色删除/组织绑定互斥）：
//   - 启用或停用但未删角色被组织绑定时拒绝角色删除（既有冲突码 ErrRoleInUse
//     40003；repo.Delete 事务内角色 FOR UPDATE 认领 + org_roles 计数，与
//     BindOrgRole 的角色 FOR SHARE 互斥——绑定先提交 → 删除被拒；删除先提交
//     → 绑定侧复核角色已删 → ErrNotFound，零残留）；
//   - 守卫生效前的旧库残留：仅含「确实软删角色」死绑定的组织，删除事务内
//     精确清理（限当前组织）；含未删角色绑定的组织维持 50014，先拒后清。
//
// 不变量：org_roles 不指向软删角色（本守卫生效后的新数据零残留）。
package service_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	casbinpkg "github.com/tracerbiubiubiu/zhuzhao/internal/casbin"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func zzRolesGuardReset(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(trustedIAMTestContext(), `
		TRUNCATE org_roles, role_menus, user_roles, user_orgs, users, organizations
		RESTART IDENTITY CASCADE;
		DELETE FROM roles`)
	require.NoError(t, err)
}

// 真实 adapter enforcer：DeleteRole 成功路径会 schedulePolicyReload →
// LoadPolicy（nil adapter 会 panic，沿用 iam_guard_transaction 集成测试先例）。
func zzRolesGuardRBAC(t *testing.T) *service.RBACService {
	t.Helper()
	enforcer, cleanup, err := casbinpkg.New(config.CasbinConfig{Model: "../../configs/casbin_model.conf"}, testPool)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	return service.NewRBACService(
		repository.NewRoleRepo(testPool),
		repository.NewUserRepo(testPool),
		repository.NewMenuRepo(testPool),
		enforcer,
	)
}

func zzRolesGuardSuperadmin(t *testing.T) int64 {
	t.Helper()
	super := rbacTestRole(t, "superadmin", 1, true)
	return rbacTestUser(t, "zzrg_sa", "ZZRG_SA", []int64{super.ID})
}

func zzRolesGuardOrg(t *testing.T, code, path string, parentID *int64) int64 {
	t.Helper()
	var id int64
	err := testPool.QueryRow(trustedIAMTestContext(), `
		INSERT INTO organizations (code, name, parent_id, path, is_virtual, status, is_system, sort_order)
		VALUES ($1, $1, $2, $3::ltree, false, 1, false, 1) RETURNING id`, code, parentID, path).Scan(&id)
	require.NoError(t, err)
	return id
}

func zzRolesGuardVg(t *testing.T, code, path string, parentID int64) int64 {
	t.Helper()
	var id int64
	err := testPool.QueryRow(trustedIAMTestContext(), `
		INSERT INTO organizations (code, name, parent_id, path, is_virtual, status, is_system, sort_order)
		VALUES ($1, $1, $2, $3::ltree, true, 1, false, 1) RETURNING id`, code, &parentID, path).Scan(&id)
	require.NoError(t, err)
	return id
}

func zzRolesGuardBind(t *testing.T, orgID, roleID int64) {
	t.Helper()
	require.NoError(t, repository.NewOrgRepo(testPool).BindOrgRole(trustedIAMTestContext(), orgID, roleID))
}

func zzRolesGuardLegacySoftDeleteRole(t *testing.T, roleID int64) {
	t.Helper()
	// 绕过 service/repo：模拟本守卫生效前旧库遗留的软删角色死绑定
	_, err := testPool.Exec(trustedIAMTestContext(),
		`UPDATE roles SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, roleID)
	require.NoError(t, err)
}

func zzRolesGuardBindings(t *testing.T, orgID int64) int64 {
	t.Helper()
	var n int64
	require.NoError(t, testPool.QueryRow(trustedIAMTestContext(),
		`SELECT COUNT(*) FROM org_roles WHERE org_id = $1`, orgID).Scan(&n))
	return n
}

func zzRolesGuardLiveBindings(t *testing.T, orgID int64) int64 {
	t.Helper()
	var n int64
	require.NoError(t, testPool.QueryRow(trustedIAMTestContext(), `
		SELECT COUNT(*) FROM org_roles obr
		JOIN roles r ON r.id = obr.role_id AND r.deleted_at IS NULL
		WHERE obr.org_id = $1`, orgID).Scan(&n))
	return n
}

func zzRolesGuardRoleBindingsByRole(t *testing.T, roleID int64) int64 {
	t.Helper()
	var n int64
	require.NoError(t, testPool.QueryRow(trustedIAMTestContext(),
		`SELECT COUNT(*) FROM org_roles WHERE role_id = $1`, roleID).Scan(&n))
	return n
}

func zzRolesGuardRoleAlive(t *testing.T, roleID int64) bool {
	t.Helper()
	var alive bool
	require.NoError(t, testPool.QueryRow(trustedIAMTestContext(),
		`SELECT deleted_at IS NULL FROM roles WHERE id = $1`, roleID).Scan(&alive))
	return alive
}

func zzRolesGuardOrgAlive(t *testing.T, orgID int64) bool {
	t.Helper()
	var alive bool
	require.NoError(t, testPool.QueryRow(trustedIAMTestContext(),
		`SELECT deleted_at IS NULL FROM organizations WHERE id = $1`, orgID).Scan(&alive))
	return alive
}

// ─── 角色删除侧：真实 service（handler roles/delete → RBACService.DeleteRole） ───

// 启用/停用但未删的角色被组织绑定时，roles/delete 拒绝（既有冲突码 40003
// ErrRoleInUse），角色与绑定原样——修复前直接软删留死绑定。
func TestZZRemRolesGuard_ServiceDeleteRoleRejectedWhileOrgBound(t *testing.T) {
	zzRolesGuardReset(t)
	ctx := trustedIAMTestContext()
	svc := zzRolesGuardRBAC(t)
	sa := zzRolesGuardSuperadmin(t)

	for _, status := range []int{1, 0} { // 启用 / 停用（未删）
		orgID := zzRolesGuardOrg(t, fmt.Sprintf("zzrg_t1_%d", status), fmt.Sprintf("root.zzrg_t1_%d", status), nil)
		role := rbacTestRole(t, fmt.Sprintf("zzrg_t1_role_%d", status), 90, false)
		zzRolesGuardBind(t, orgID, role.ID)
		if status == 0 { // 合法停用路径（UpdateRole）：停用不解除绑定，删除仍须被拒
			disabled := 0
			_, err := svc.UpdateRole(ctx, &model.UpdateRoleRequest{
				ID: role.ID, Version: 1, Name: role.Name, Priority: role.Priority, Status: &disabled,
			}, sa)
			require.NoError(t, err)
		}

		err := svc.DeleteRole(ctx, role.ID, sa)
		requireErrCode(t, err, errcode.ErrRoleInUse)
		require.True(t, zzRolesGuardRoleAlive(t, role.ID), "被拒删的角色必须存活（status=%d）", status)
		require.Equal(t, int64(1), zzRolesGuardBindings(t, orgID), "拒删不得动绑定（status=%d）", status)
	}
}

// 合法解绑后：角色删除成功（软删、绑定零残留），随后无绑定组织零错误删除。
func TestZZRemRolesGuard_UnbindThenDeleteRoleThenOrg(t *testing.T) {
	zzRolesGuardReset(t)
	ctx := trustedIAMTestContext()
	svc := zzRolesGuardRBAC(t)
	orgRepo := repository.NewOrgRepo(testPool)
	sa := zzRolesGuardSuperadmin(t)

	orgID := zzRolesGuardOrg(t, "zzrg_t2", "root.zzrg_t2", nil)
	role := rbacTestRole(t, "zzrg_t2_role", 90, false)
	zzRolesGuardBind(t, orgID, role.ID)

	requireErrCode(t, svc.DeleteRole(ctx, role.ID, sa), errcode.ErrRoleInUse)

	require.NoError(t, orgRepo.UnbindOrgRole(ctx, orgID, role.ID), "解绑入口必须可达")
	require.NoError(t, svc.DeleteRole(ctx, role.ID, sa), "解绑后应可删除角色")
	require.False(t, zzRolesGuardRoleAlive(t, role.ID))
	require.Zero(t, zzRolesGuardRoleBindingsByRole(t, role.ID))

	require.NoError(t, orgRepo.Delete(ctx, orgID), "无绑定组织应零错误删除")
	require.False(t, zzRolesGuardOrgAlive(t, orgID))
}

// ─── 组织删除侧：旧库死绑定精确清理（限当前组织），未删角色维持 50014 ───

// 两个组织各挂一条「确实软删角色」的死绑定：删 org1 成功并清理其死绑定；
// org2 的死绑定不受影响（清理严格限当前组织）。
func TestZZRemRolesGuard_OrgDeleteCleansOnlyOwnDeadBindings(t *testing.T) {
	zzRolesGuardReset(t)
	ctx := trustedIAMTestContext()
	orgRepo := repository.NewOrgRepo(testPool)

	role1 := rbacTestRole(t, "zzrg_t3_dead1", 90, false)
	role2 := rbacTestRole(t, "zzrg_t3_dead2", 90, false)
	org1 := zzRolesGuardOrg(t, "zzrg_t3_a", "root.zzrg_t3_a", nil)
	org2 := zzRolesGuardOrg(t, "zzrg_t3_b", "root.zzrg_t3_b", nil)
	zzRolesGuardBind(t, org1, role1.ID)
	zzRolesGuardBind(t, org2, role2.ID)
	zzRolesGuardLegacySoftDeleteRole(t, role1.ID)
	zzRolesGuardLegacySoftDeleteRole(t, role2.ID)

	require.NoError(t, orgRepo.Delete(ctx, org1), "仅死绑定的组织应可删（死绑定同事务精确清理）")
	require.False(t, zzRolesGuardOrgAlive(t, org1))
	require.Zero(t, zzRolesGuardBindings(t, org1), "删除成功后死绑定零残留")

	require.Equal(t, int64(1), zzRolesGuardBindings(t, org2), "清理必须限当前组织")
	require.True(t, zzRolesGuardOrgAlive(t, org2))
}

// 混合绑定（未删 + 软删）：维持 50014 且先拒后清（任何绑定不得动）；
// 解绑未删角色后删除成功，死绑定同事务清理归零。
func TestZZRemRolesGuard_OrgDeleteMixedLiveKeeps50014(t *testing.T) {
	zzRolesGuardReset(t)
	ctx := trustedIAMTestContext()
	orgRepo := repository.NewOrgRepo(testPool)

	live := rbacTestRole(t, "zzrg_t4_live", 90, false)
	dead := rbacTestRole(t, "zzrg_t4_dead", 90, false)
	orgID := zzRolesGuardOrg(t, "zzrg_t4", "root.zzrg_t4", nil)
	zzRolesGuardBind(t, orgID, live.ID)
	zzRolesGuardBind(t, orgID, dead.ID)
	zzRolesGuardLegacySoftDeleteRole(t, dead.ID)

	err := orgRepo.Delete(ctx, orgID)
	requireErrCode(t, err, errcode.ErrOrgHasOrgRoles) // 未删角色维持 50014
	require.True(t, zzRolesGuardOrgAlive(t, orgID), "被拒删的组织必须存活")
	require.Equal(t, int64(2), zzRolesGuardBindings(t, orgID), "拒删轮次不得清理任何绑定")
	require.Equal(t, int64(1), zzRolesGuardLiveBindings(t, orgID))

	require.NoError(t, orgRepo.UnbindOrgRole(ctx, orgID, live.ID))
	require.NoError(t, orgRepo.Delete(ctx, orgID), "解绑未删角色后应可删")
	require.False(t, zzRolesGuardOrgAlive(t, orgID))
	require.Zero(t, zzRolesGuardBindings(t, orgID), "删除成功后死绑定零残留")
}

// 虚拟组（同政策）：混合绑定时先拒后清（owner 成员行不丢）；解绑未删角色后
// 删除成功，死绑定清理、owner 派生成员行照常清空。
func TestZZRemRolesGuard_VgDeleteDeadCleanupAndOwnerKeep(t *testing.T) {
	zzRolesGuardReset(t)
	ctx := trustedIAMTestContext()
	orgRepo := repository.NewOrgRepo(testPool)

	parent := zzRolesGuardOrg(t, "zzrg_t5_p", "root.zzrg_t5_p", nil)
	vgID := zzRolesGuardVg(t, "zzrg_t5_vg", "root.zzrg_t5_p.zzrg_t5_vg", parent)
	uid := rbacTestUser(t, "zzrg_t5_owner", "ZZRG_T5_OWNER", nil)
	_, err := testPool.Exec(ctx,
		`INSERT INTO user_orgs (user_id, org_id, org_member_role) VALUES ($1, $2, 'owner')`, uid, vgID)
	require.NoError(t, err)

	live := rbacTestRole(t, "zzrg_t5_live", 90, false)
	dead := rbacTestRole(t, "zzrg_t5_dead", 90, false)
	zzRolesGuardBind(t, vgID, live.ID)
	zzRolesGuardBind(t, vgID, dead.ID)
	zzRolesGuardLegacySoftDeleteRole(t, dead.ID)

	err = orgRepo.DeleteVgWithOwnerCleanup(ctx, vgID)
	requireErrCode(t, err, errcode.ErrOrgHasOrgRoles)
	require.True(t, zzRolesGuardOrgAlive(t, vgID), "被拒删的虚拟组必须存活")
	require.Equal(t, int64(2), zzRolesGuardBindings(t, vgID), "拒删轮次不得清理任何绑定")
	var owners int64
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_orgs WHERE org_id = $1 AND org_member_role = 'owner'`, vgID).Scan(&owners))
	require.Equal(t, int64(1), owners, "拒删轮次 owner 成员行不得丢失")

	require.NoError(t, orgRepo.UnbindOrgRole(ctx, vgID, live.ID))
	require.NoError(t, orgRepo.DeleteVgWithOwnerCleanup(ctx, vgID), "解绑未删角色后应可删")
	require.False(t, zzRolesGuardOrgAlive(t, vgID))
	require.Zero(t, zzRolesGuardBindings(t, vgID), "删除成功后死绑定零残留")
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_orgs WHERE org_id = $1 AND org_member_role = 'owner'`, vgID).Scan(&owners))
	require.Zero(t, owners, "删除成功仍须清 owner 派生成员行")
}

// ─── 并发反序：DeleteRole × BindOrgRole 双向交错（每方向 8 轮） ───
//
//   - 绑定先提交 → 角色删除被拒（40003、角色存活、绑定在）；
//   - 删除先提交 → 绑定复核角色已删 → ErrNotFound（绑定不落库）。
//
// 不变量：org_roles 零残留指向软删角色。
func TestZZRemRolesGuard_DeleteRoleBindRace(t *testing.T) {
	zzRolesGuardReset(t)
	ctx := trustedIAMTestContext()
	svc := zzRolesGuardRBAC(t)
	orgRepo := repository.NewOrgRepo(testPool)
	sa := zzRolesGuardSuperadmin(t)

	const roundsEach = 8
	idx := 0
	for _, deleteFirst := range []bool{true, false} {
		for r := 0; r < roundsEach; r++ {
			orgID := zzRolesGuardOrg(t, fmt.Sprintf("zzrg_race_%d", idx), fmt.Sprintf("root.zzrg_race_%d", idx), nil)
			role := rbacTestRole(t, fmt.Sprintf("zzrg_race_role_%d", idx), 95, false)
			idx++

			roundCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			var delErr, bindErr error
			delFn := func() { delErr = svc.DeleteRole(roundCtx, role.ID, sa) }
			bindFn := func() { bindErr = orgRepo.BindOrgRole(roundCtx, orgID, role.ID) }
			var wg sync.WaitGroup
			wg.Add(2)
			start := make(chan struct{})
			launch := func() {
				<-start
				if deleteFirst {
					delFn()
				} else {
					bindFn()
				}
				wg.Done()
			}
			launch2 := func() {
				<-start
				if deleteFirst {
					bindFn()
				} else {
					delFn()
				}
				wg.Done()
			}
			go launch()
			go launch2()
			close(start)
			wg.Wait()
			cancel()

			alive := zzRolesGuardRoleAlive(t, role.ID)
			bindings := zzRolesGuardBindings(t, orgID)

			// 零残留不变量：绑定存在 ⇒ 角色存活
			require.True(t, bindings == 0 || alive,
				"第 %d 轮（deleteFirst=%v）出现「软删角色仍挂组织绑定」（delete=%v bind=%v）",
				idx-1, deleteFirst, delErr, bindErr)

			if bindErr == nil {
				require.Error(t, delErr, "绑定已提交，角色删除必须被拒（第 %d 轮 deleteFirst=%v）", idx-1, deleteFirst)
				requireErrCode(t, delErr, errcode.ErrRoleInUse)
				require.True(t, alive)
				require.Equal(t, int64(1), bindings)
			} else {
				require.ErrorIs(t, bindErr, errcode.ErrNotFound,
					"删除已提交，绑定必须被角色存续锁拒绝（第 %d 轮 deleteFirst=%v）", idx-1, deleteFirst)
				require.NoError(t, delErr)
				require.False(t, alive)
				require.Zero(t, bindings)
			}
		}
	}
}
