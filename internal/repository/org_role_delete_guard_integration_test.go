//go:build integration

// 2026-11-20 审计续批：org_roles 绑定 × 组织删除互斥（当前依赖清理语义内的
// 数据完整性收口，不引入组织生命周期变更）。
//
//   - Delete / DeleteVgWithOwnerCleanup：已认领的 org FOR UPDATE 同事务内计数
//     org_roles，仍有绑定 → ErrOrgHasOrgRoles（50014）拒删；VG 侧先拒后清，
//     owner 成员行必不丢；
//   - BindOrgRole：绑定事务内取成员写入同款 FOR SHARE 存续锁（与软删认领的
//     FOR UPDATE 互斥），杜绝「软删组织仍挂角色绑定」——绑定先提交 → 删除方
//     计数看到绑定拒删（组织存活）；删除先提交 → 绑定方复核 deleted_at →
//     ErrOrgNotFound（绑定不落库）。
//
// 不变量：org_roles 不指向软删组织（零残留）；拒绝删除的组织 owner 成员行原样。
package repository_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func insertGuardRole(t *testing.T, code string) int64 {
	t.Helper()
	var id int64
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO roles (code, name, priority, status, is_system)
		VALUES ($1, $1, 100, 1, false) RETURNING id`, code).Scan(&id)
	require.NoError(t, err)
	return id
}

func cleanupGuardRole(t *testing.T, roleID int64) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`DELETE FROM org_roles WHERE role_id = $1`, roleID)
	require.NoError(t, err)
	_, err = testPool.Exec(context.Background(),
		`DELETE FROM roles WHERE id = $1`, roleID)
	require.NoError(t, err)
}

func insertOwnerMembership(t *testing.T, orgID, userID int64) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO user_orgs (user_id, org_id, org_member_role) VALUES ($1, $2, 'owner')`, userID, orgID)
	require.NoError(t, err)
}

func orgRoleBindings(t *testing.T, orgID int64) int64 {
	t.Helper()
	var n int64
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM org_roles WHERE org_id = $1`, orgID).Scan(&n))
	return n
}

func orgAlive(t *testing.T, orgID int64) bool {
	t.Helper()
	var alive bool
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT deleted_at IS NULL FROM organizations WHERE id = $1`, orgID).Scan(&alive))
	return alive
}

func ownerMemberships(t *testing.T, orgID int64) int64 {
	t.Helper()
	var n int64
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM user_orgs WHERE org_id = $1 AND org_member_role = 'owner'`, orgID).Scan(&n))
	return n
}

// ─── 顺序正反例 ───

// 有绑定的组织拒删（50014、组织存活、绑定原样）；解绑后可删。
func TestOrgRepo_DeleteRejectsWithRoleBindings(t *testing.T) {
	resetOrgs(t)
	ctx := context.Background()
	repo := repository.NewOrgRepo(testPool)

	orgID := insertOrg(t, "role_guard", "root.role_guard", nil)
	roleID := insertGuardRole(t, "rg_role")
	defer cleanupGuardRole(t, roleID)

	require.NoError(t, repo.BindOrgRole(ctx, orgID, roleID))

	err := repo.Delete(ctx, orgID)
	require.Error(t, err, "仍有角色绑定的组织必须拒删（修复前直接软删留脏绑定）")
	var biz *errcode.Error
	require.ErrorAs(t, err, &biz)
	require.Equal(t, errcode.ErrOrgHasOrgRoles.Code, biz.Code)
	require.True(t, orgAlive(t, orgID), "被拒删的组织必须存活")
	require.Equal(t, int64(1), orgRoleBindings(t, orgID))

	_, err = testPool.Exec(ctx, `DELETE FROM org_roles WHERE org_id = $1 AND role_id = $2`, orgID, roleID)
	require.NoError(t, err)
	require.NoError(t, repo.Delete(ctx, orgID), "解绑后应可删")
	require.False(t, orgAlive(t, orgID))
}

// 虚拟组：有绑定时拒删且 owner 成员行不丢（先拒后清）；解绑后删除仍清 owner 行。
func TestOrgRepo_DeleteVgRejectKeepsOwner(t *testing.T) {
	resetOrgs(t)
	ctx := context.Background()
	repo := repository.NewOrgRepo(testPool)

	parent := insertOrg(t, "rg_parent", "root.rg_parent", nil)
	vgID := insertVirtualOrg(t, "rg_vg", "root.rg_parent.rg_vg", parent)
	uid := orgTestUser(t, "E810001")
	insertOwnerMembership(t, vgID, uid)
	roleID := insertGuardRole(t, "rg_vg_role")
	defer cleanupGuardRole(t, roleID)

	require.NoError(t, repo.BindOrgRole(ctx, vgID, roleID))

	err := repo.DeleteVgWithOwnerCleanup(ctx, vgID)
	require.Error(t, err, "仍有角色绑定的虚拟组必须拒删")
	var biz *errcode.Error
	require.ErrorAs(t, err, &biz)
	require.Equal(t, errcode.ErrOrgHasOrgRoles.Code, biz.Code)
	require.True(t, orgAlive(t, vgID), "被拒删的虚拟组必须存活")
	require.Equal(t, int64(1), ownerMemberships(t, vgID), "拒删后 owner 成员行不得丢失")
	require.Equal(t, int64(1), orgRoleBindings(t, vgID))

	_, err = testPool.Exec(ctx, `DELETE FROM org_roles WHERE org_id = $1 AND role_id = $2`, vgID, roleID)
	require.NoError(t, err)
	require.NoError(t, repo.DeleteVgWithOwnerCleanup(ctx, vgID), "解绑后应可删")
	require.False(t, orgAlive(t, vgID))
	require.Zero(t, ownerMemberships(t, vgID), "删除成功仍须清 owner 派生成员行")
}

// 删除后绑定必拒：ErrOrgNotFound、org_roles 零残留。
func TestOrgRepo_BindOrgRoleRejectedAfterDelete(t *testing.T) {
	resetOrgs(t)
	ctx := context.Background()
	repo := repository.NewOrgRepo(testPool)

	orgID := insertOrg(t, "rg_dead", "root.rg_dead", nil)
	roleID := insertGuardRole(t, "rg_dead_role")
	defer cleanupGuardRole(t, roleID)

	require.NoError(t, repo.Delete(ctx, orgID))
	err := repo.BindOrgRole(ctx, orgID, roleID)
	require.ErrorIs(t, err, errcode.ErrOrgNotFound, "软删组织必须拒绝绑定（存续锁复核 deleted_at）")
	require.Zero(t, orgRoleBindings(t, orgID))
}

// ─── 并发互斥（每方向 8 轮变异） ───

// Delete × BindOrgRole 双向交错：无论谁先认领——
//   - 绑定先提交 → 删除计数看到绑定 → 50014 拒删（组织存活、绑定在）；
//   - 删除先提交 → 绑定复核 deleted_at → ErrOrgNotFound（绑定不落库）。
//
// 不变量：org_roles 零残留指向软删组织。
func TestOrgRepo_DeleteBindRoleConcurrency(t *testing.T) {
	resetOrgs(t)
	ctx := context.Background()
	repo := repository.NewOrgRepo(testPool)

	const roundsEach = 8
	roleID := insertGuardRole(t, "rg_race_role")
	defer cleanupGuardRole(t, roleID)

	idx := 0
	for _, deleteFirst := range []bool{true, false} {
		for r := 0; r < roundsEach; r++ {
			code := fmt.Sprintf("rg_race_%d", idx)
			orgID := insertOrg(t, code, "root."+code, nil)
			idx++

			roundCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			var delErr, bindErr error
			delFn := func() { delErr = repo.Delete(roundCtx, orgID) }
			bindFn := func() { bindErr = repo.BindOrgRole(roundCtx, orgID, roleID) }
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

			alive := orgAlive(t, orgID)
			bindings := orgRoleBindings(t, orgID)

			// 不变量：绑定存在 ⇒ 组织存活（零残留）
			require.True(t, bindings == 0 || alive,
				"第 %d 轮（deleteFirst=%v）出现「软删组织仍挂角色绑定」（delete=%v bind=%v）",
				idx-1, deleteFirst, delErr, bindErr)

			if bindErr == nil {
				require.Error(t, delErr, "绑定已提交，删除必须被拒（第 %d 轮 deleteFirst=%v）", idx-1, deleteFirst)
				var biz *errcode.Error
				require.ErrorAs(t, delErr, &biz)
				require.Equal(t, errcode.ErrOrgHasOrgRoles.Code, biz.Code)
				require.True(t, alive)
				require.Equal(t, int64(1), bindings)
			} else {
				require.ErrorIs(t, bindErr, errcode.ErrOrgNotFound,
					"删除已提交，绑定必须被存续锁拒绝（第 %d 轮 deleteFirst=%v）", idx-1, deleteFirst)
				require.NoError(t, delErr)
				require.False(t, alive)
				require.Zero(t, bindings)
			}
		}
	}
}

// DeleteVgWithOwnerCleanup × BindOrgRole 双向交错——同上不变量，外加：
// 删除被拒轮次 owner 成员行原样（虚拟组拒后 owner 不丢）。
func TestOrgRepo_DeleteVgBindRoleConcurrency(t *testing.T) {
	resetOrgs(t)
	ctx := context.Background()
	repo := repository.NewOrgRepo(testPool)

	const roundsEach = 8
	roleID := insertGuardRole(t, "rg_vg_race_role")
	defer cleanupGuardRole(t, roleID)

	idx := 0
	for _, deleteFirst := range []bool{true, false} {
		for r := 0; r < roundsEach; r++ {
			code := fmt.Sprintf("rg_vg_race_%d", idx)
			parent := insertOrg(t, code+"_p", "root."+code+"_p", nil)
			vgID := insertVirtualOrg(t, code, "root."+code+"_p."+code, parent)
			uid := orgTestUser(t, fmt.Sprintf("E81%05d", idx))
			insertOwnerMembership(t, vgID, uid)
			idx++

			roundCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			var delErr, bindErr error
			delFn := func() { delErr = repo.DeleteVgWithOwnerCleanup(roundCtx, vgID) }
			bindFn := func() { bindErr = repo.BindOrgRole(roundCtx, vgID, roleID) }
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

			alive := orgAlive(t, vgID)
			bindings := orgRoleBindings(t, vgID)
			owners := ownerMemberships(t, vgID)

			require.True(t, bindings == 0 || alive,
				"第 %d 轮（deleteFirst=%v）出现「软删虚拟组仍挂角色绑定」（delete=%v bind=%v）",
				idx-1, deleteFirst, delErr, bindErr)

			if bindErr == nil {
				require.Error(t, delErr, "绑定已提交，VG 删除必须被拒（第 %d 轮 deleteFirst=%v）", idx-1, deleteFirst)
				var biz *errcode.Error
				require.ErrorAs(t, delErr, &biz)
				require.Equal(t, errcode.ErrOrgHasOrgRoles.Code, biz.Code)
				require.True(t, alive)
				require.Equal(t, int64(1), bindings)
				require.Equal(t, int64(1), owners, "拒删轮次 owner 成员行不得丢失")
			} else {
				require.ErrorIs(t, bindErr, errcode.ErrOrgNotFound,
					"删除已提交，绑定必须被存续锁拒绝（第 %d 轮 deleteFirst=%v）", idx-1, deleteFirst)
				require.NoError(t, delErr)
				require.False(t, alive)
				require.Zero(t, bindings)
				require.Zero(t, owners, "删除成功轮次 owner 派生成员行已清")
			}
		}
	}
}
