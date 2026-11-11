//go:build integration

package repository_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao-utils/crypto"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// insertRole 测试辅助：直接插角色行（绕过 Create 的 is_system=false 约束）
func insertRole(t *testing.T, code string, priority int, status int, isSystem bool) int64 {
	t.Helper()
	var id int64
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO roles (code, name, priority, status, is_system)
		VALUES ($1, $1, $2, $3, $4) RETURNING id`, code, priority, status, isSystem).Scan(&id)
	require.NoError(t, err)
	return id
}

func setRoleStatus(t *testing.T, roleID int64, status int) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `UPDATE roles SET status = $2 WHERE id = $1`, roleID, status)
	require.NoError(t, err)
}

// B1-1 守护：禁用角色（status=0）不参与鉴权链——GetRoleCodes（Casbin 中间件
// 每请求调用）、GetRoles（priority 档位）、ListRoleIDsByUserID（菜单下发）
// 三条路径都必须排除禁用角色；重新启用即恢复（casbin_rule 未清除）。
func TestRoleRepo_DisabledRoleExcluded(t *testing.T) {
	resetUsers(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepo(testPool)
	roleRepo := repository.NewRoleRepo(testPool)

	hash, err := crypto.HashPassword("p")
	require.NoError(t, err)
	user := &model.User{Username: "rbac_guard", EmployeeNo: "E700001", Password: hash}
	require.NoError(t, userRepo.Create(ctx, user))

	enabledID := insertRole(t, "guard_on", 20, 1, false)
	disabledID := insertRole(t, "guard_off", 25, 0, false)
	require.NoError(t, userRepo.SetRoles(ctx, user.ID, []int64{enabledID, disabledID}))

	// 禁用角色不参与 Casbin enforce
	codes, err := userRepo.GetRoleCodes(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"guard_on"}, codes, "GetRoleCodes 应排除禁用角色")

	// 禁用角色不计入 priority 档位（GetRoles）
	roles, err := userRepo.GetRoles(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, roles, 1)
	assert.Equal(t, "guard_on", roles[0].Code, "GetRoles 应排除禁用角色")

	// 禁用角色的菜单不下发
	roleIDs, err := roleRepo.ListRoleIDsByUserID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{enabledID}, roleIDs, "ListRoleIDsByUserID 应排除禁用角色")

	// 重新启用即恢复（策略保留，无需重配）
	setRoleStatus(t, disabledID, 1)
	codes, err = userRepo.GetRoleCodes(ctx, user.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"guard_on", "guard_off"}, codes, "重新启用后角色应恢复参与鉴权")
}

// B1-1 守护：superadmin 保护判断同样要求角色启用——禁用 superadmin 角色
// 后 IsSuperadminUser 返回 false、CountActiveSuperadminUsers 不计入。
func TestUserRepo_SuperadminChecks_DisabledRoleNotCounted(t *testing.T) {
	resetUsers(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepo(testPool)

	hash, err := crypto.HashPassword("p")
	require.NoError(t, err)
	u1 := &model.User{Username: "sa1", EmployeeNo: "E700011", Password: hash, Status: 1}
	u2 := &model.User{Username: "sa2", EmployeeNo: "E700012", Password: hash, Status: 1}
	require.NoError(t, userRepo.Create(ctx, u1))
	require.NoError(t, userRepo.Create(ctx, u2))

	// is_system=false 便于 resetUsers 清理（superadmin 判断只认 code，与 is_system 无关；
	// 避免残留系统角色与 TestUserRepo_SuperadminGuard 的自插同名角色冲突）
	saRole := insertRole(t, "superadmin", 1, 1, false)
	require.NoError(t, userRepo.SetRoles(ctx, u1.ID, []int64{saRole}))
	require.NoError(t, userRepo.SetRoles(ctx, u2.ID, []int64{saRole}))

	ok, err := userRepo.IsSuperadminUser(ctx, u1.ID)
	require.NoError(t, err)
	assert.True(t, ok)
	n, err := userRepo.CountActiveSuperadminUsers(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)

	// 禁用 superadmin 角色：两名用户均不再计入 superadmin 判断
	setRoleStatus(t, saRole, 0)
	ok, err = userRepo.IsSuperadminUser(ctx, u1.ID)
	require.NoError(t, err)
	assert.False(t, ok, "禁用 superadmin 角色后 IsSuperadminUser 应为 false")
	n, err = userRepo.CountActiveSuperadminUsers(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 0, n, "禁用 superadmin 角色后 CountActiveSuperadminUsers 应为 0")
}

// 复核批 C 回归（2026-10-12）：子页面的 menu_apis 来源行必须让 /orgs GET 授权
// 经得起 AssignMenus 全删重建——000042 首版只插 casbin 不插 menu_apis，任何一次
// UI 保存菜单授权即被抹掉（viewer 下次保存即 403 复发）。本测试按 service 层
// 真实重放路径（ListMenuAPIsByMenuIDs → AssignMenus）钉死「重建后存活」。
func TestRoleRepo_AssignMenus_OrgsReadSurvivesRebuild(t *testing.T) {
	resetUsers(t)
	ctx := context.Background()
	roleRepo := repository.NewRoleRepo(testPool)
	menuRepo := repository.NewMenuRepo(testPool)

	roleID := insertRole(t, "orgs_replay", 40, 1, false)

	// 自清（resetUsers 删非系统角色会被 role_menus FK 挡——本测试是包内首个
	// 给非系统角色挂 role_menus 的用例，残留会污染后续用例的 resetUsers）
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM role_menus WHERE role_id = $1`, roleID)
		_, _ = testPool.Exec(ctx, `DELETE FROM casbin_rule WHERE v0 = 'role::orgs_replay'`)
		_, _ = testPool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, roleID)
	})

	// 只绑 ticket_type_manage（000042 发放面：工单管理目录直属子页面）
	var typeMenuID int64
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT id FROM menus WHERE code = 'ticket_type_manage' AND deleted_at IS NULL`,
	).Scan(&typeMenuID))

	apis, err := menuRepo.ListMenuAPIsByMenuIDs(ctx, []int64{typeMenuID})
	require.NoError(t, err)
	require.NoError(t, roleRepo.AssignMenus(ctx, &model.Role{ID: roleID, Code: "orgs_replay"},
		[]int64{typeMenuID}, apis))

	// 重建后 /orgs GET 必须存活（来源 = 000042 up 补的 menu_apis 行）
	var count int
	require.NoError(t, testPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM casbin_rule
		WHERE ptype = 'p' AND v0 = 'role::orgs_replay'
		  AND v1 = '/api/v1/orgs' AND v2 = 'GET'`).Scan(&count))
	assert.Equal(t, 1, count, "AssignMenus 重建后 /orgs GET 规则应存活（menu_apis 来源行生效）")

	// 反向对照：down 000042 删来源行后，同一路径重放不再发放（down 忠实逆的语义面）
	// 自清（隔离债）：来源行是 000042 迁移种下的**共享夹具**，删后必须原地补回——否则
	// 本包共享库此后永久缺该行，后续（含新增）依赖「ticket_type_manage→/orgs GET」授权
	// 来源的用例会莫名失败。删除动作之前注册，保证 Delete 报错也不漏恢复。
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `
			INSERT INTO menu_apis (menu_id, api_path, api_method)
			VALUES ($1, '/api/v1/orgs', 'GET')
			ON CONFLICT DO NOTHING`, typeMenuID)
	})
	_, err = testPool.Exec(ctx, `
		DELETE FROM menu_apis
		WHERE api_path = '/api/v1/orgs' AND api_method = 'GET'
		  AND menu_id = $1`, typeMenuID)
	require.NoError(t, err)
	apis2, err := menuRepo.ListMenuAPIsByMenuIDs(ctx, []int64{typeMenuID})
	require.NoError(t, err)
	require.NoError(t, roleRepo.AssignMenus(ctx, &model.Role{ID: roleID, Code: "orgs_replay"},
		[]int64{typeMenuID}, apis2))
	require.NoError(t, testPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM casbin_rule
		WHERE ptype = 'p' AND v0 = 'role::orgs_replay'
		  AND v1 = '/api/v1/orgs' AND v2 = 'GET'`).Scan(&count))
	assert.Equal(t, 0, count, "来源行删除后重放不得再发放 /orgs GET")
}

// F-B2 并发回归（2026-10-21 编号落档）：两个管理员并发重绑同一角色，
// DELETE ALL + INSERT ALL 无锁时两事务交错 → 终态为两意图 UNION（互毁）。
// 修复 = 30ff6c8 AssignMenus 事务内 roles 行 FOR UPDATE（串行化 DELETE→INSERT）。
// 断言：串行 rounds 轮，每轮终态 role_menus 必须恰为某一次输入的完整单集
// （非混合）——§12-10 变异验证公约同款构件级守卫。
func TestRoleRepo_AssignMenus_ConcurrentNoMix(t *testing.T) {
	resetUsers(t)
	ctx := context.Background()
	roleRepo := repository.NewRoleRepo(testPool)
	menuRepo := repository.NewMenuRepo(testPool)

	roleID := insertRole(t, "assign_race", 40, 1, false)
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM role_menus WHERE role_id = $1`, roleID)
		_, _ = testPool.Exec(ctx, `DELETE FROM casbin_rule WHERE v0 = 'role::assign_race'`)
		_, _ = testPool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, roleID)
	})

	// 两个不相交菜单集：A = ticket_type_manage（共享夹具），B = 任一其他活跃菜单
	var menuA, menuB int64
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT id FROM menus WHERE code = 'ticket_type_manage' AND deleted_at IS NULL`).Scan(&menuA))
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT id FROM menus WHERE deleted_at IS NULL AND id <> $1 ORDER BY id LIMIT 1`, menuA).Scan(&menuB))
	apisA, err := menuRepo.ListMenuAPIsByMenuIDs(ctx, []int64{menuA})
	require.NoError(t, err)
	apisB, err := menuRepo.ListMenuAPIsByMenuIDs(ctx, []int64{menuB})
	require.NoError(t, err)

	// 交错命中是概率事件——串行多轮堆命中率（§12-10：靠轮数非并发数）
	const rounds = 4
	for round := 0; round < rounds; round++ {
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make(chan error, 2)
		for _, assign := range []struct {
			menuIDs []int64
			apis    []*model.MenuAPI
		}{{[]int64{menuA}, apisA}, {[]int64{menuB}, apisB}} {
			wg.Add(1)
			go func(mIDs []int64, mAPIs []*model.MenuAPI) {
				defer wg.Done()
				<-start
				errs <- roleRepo.AssignMenus(ctx, &model.Role{ID: roleID, Code: "assign_race"}, mIDs, mAPIs)
			}(assign.menuIDs, assign.apis)
		}
		close(start)
		wg.Wait()
		require.NoError(t, <-errs, "round %d assign A 不应失败", round)
		require.NoError(t, <-errs, "round %d assign B 不应失败", round)

		// 终态判定：恰为 {menuA} 或 {menuB} 之一（单元素）——出现两元素混合 = 交错命中
		var ids []int64
		rows, err := testPool.Query(ctx, `SELECT menu_id FROM role_menus WHERE role_id = $1 ORDER BY menu_id`, roleID)
		require.NoError(t, err)
		for rows.Next() {
			var id int64
			require.NoError(t, rows.Scan(&id))
			ids = append(ids, id)
		}
		require.NoError(t, rows.Err())
		isA := len(ids) == 1 && ids[0] == menuA
		isB := len(ids) == 1 && ids[0] == menuB
		assert.True(t, isA || isB, "round %d 终态 %v 为混合集——DELETE/INSERT 交错（F-B2 回归）", round, ids)
	}
}
