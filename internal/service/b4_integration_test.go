//go:build integration

package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao-utils/crypto"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// B4-5 守护：GetMembers 分页（契约变更——原全量返回，响应新增 page/page_size）
func TestOrgService_GetMembersPagination(t *testing.T) {
	resetOrgTables(t)
	ctx := context.Background()
	orgRepo := repository.NewOrgRepo(testPool)
	userRepo := repository.NewUserRepo(testPool)
	svc := service.NewOrgService(orgRepo, userRepo, service.NewOrgDelegationService(testPool), newRBACServiceForTest(t))

	var rootID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, path, is_virtual, status, is_system, sort_order)
		VALUES ('gm_root', '根', 'gm_root', false, 1, false, 1) RETURNING id`).Scan(&rootID))

	// 3 名成员
	hash, err := crypto.HashPassword("p")
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		u := &model.User{Username: "gm_user", EmployeeNo: "E660001", Password: hash, Status: 1}
		u.EmployeeNo = "E66000" + string(rune('1'+i))
		require.NoError(t, userRepo.Create(ctx, u))
		require.NoError(t, orgRepo.AddMember(ctx, rootID, u.ID, i == 0))
	}

	// 第一页 2 条
	resp, err := svc.GetMembers(ctx, rootID, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(3), resp.Total, "total 应为全量成员数")
	assert.Len(t, resp.List, 2, "第一页应返回 2 条")
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 2, resp.PageSize)

	// 第二页 1 条
	resp, err = svc.GetMembers(ctx, rootID, 2, 2)
	require.NoError(t, err)
	assert.Len(t, resp.List, 1, "第二页应返回 1 条")

	// 超范围页 → 空列表（total 不变）
	resp, err = svc.GetMembers(ctx, rootID, 99, 2)
	require.NoError(t, err)
	assert.Empty(t, resp.List)
	assert.Equal(t, int64(3), resp.Total)

	// 默认参数规范化（page=0 → 1）
	resp, err = svc.GetMembers(ctx, rootID, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 20, resp.PageSize)
}

// B4-4 守护：菜单软删同事务清理 role_menus（原仅软删 menus 行——
// GetRoleMenuIDs 会向前端回显已删菜单的幽灵勾选）
func TestMenuRepo_DeleteCleansRoleMenus(t *testing.T) {
	resetOrgTables(t)
	ctx := context.Background()
	menuRepo := repository.NewMenuRepo(testPool)
	roleRepo := repository.NewRoleRepo(testPool)

	var roleID, menuID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO roles (code, name, priority) VALUES ('gm_role', '测试', 20) RETURNING id`).Scan(&roleID))
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO menus (code, name, menu_type, sort_order, visible, is_system)
		VALUES ('gm_menu', '测试页', 2, 1, true, false) RETURNING id`).Scan(&menuID))
	_, err := testPool.Exec(ctx, `INSERT INTO role_menus (role_id, menu_id) VALUES ($1, $2)`, roleID, menuID)
	require.NoError(t, err)

	// 删除菜单 → role_menus 同步清理
	require.NoError(t, menuRepo.Delete(ctx, menuID))

	var n int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM role_menus WHERE menu_id = $1`, menuID).Scan(&n))
	assert.Zero(t, n, "菜单软删后 role_menus 应被同事务清理，无幽灵勾选")

	// GetRoleMenuIDs 不再回显已删菜单
	ids, err := roleRepo.ListMenuIDsByRoleID(ctx, roleID)
	require.NoError(t, err)
	assert.Empty(t, ids)
}

// B4-4 守护：GetUserPermissions 对 admin/superadmin 通配展开——
// 即使清空超管角色的菜单勾选，权限码仍返回全量（与 Casbin matcher bypass 对齐）
func TestMenuService_GetUserPermissionsAdminWildcard(t *testing.T) {
	resetOrgTables(t)
	ctx := context.Background()
	menuRepo := repository.NewMenuRepo(testPool)
	roleRepo := repository.NewRoleRepo(testPool)
	userRepo := repository.NewUserRepo(testPool)
	svc := service.NewMenuService(menuRepo, userRepo, roleRepo)

	// 建一个按钮菜单（admin 角色不勾选它）
	var btnID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO menus (code, name, menu_type, permission, sort_order, visible, is_system)
		VALUES ('gm_btn', '测试按钮', 3, 'gm:test', 1, true, false) RETURNING id`).Scan(&btnID))

	// admin 角色：种子已绑定全部菜单——先清空其 role_menus 模拟「被清空菜单」场景
	var adminRoleID int64
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO roles (code, name, priority, is_system) VALUES ('admin', '管理员', 10, true) RETURNING id`).Scan(&adminRoleID))
	_, err := testPool.Exec(ctx, `DELETE FROM role_menus WHERE role_id = $1`, adminRoleID)
	require.NoError(t, err)

	hash, err := crypto.HashPassword("p")
	require.NoError(t, err)
	adminUser := &model.User{Username: "perm_admin", EmployeeNo: "E660010", Password: hash, Status: 1}
	require.NoError(t, userRepo.Create(ctx, adminUser))
	require.NoError(t, userRepo.SetRoles(ctx, adminUser.ID, []int64{adminRoleID}))

	// admin 用户权限码应包含 gm:test 按钮（通配展开，而非按 role_menus）
	perms, err := svc.GetUserPermissions(ctx, adminUser.ID)
	require.NoError(t, err)
	assert.Contains(t, perms, "button:gm:test",
		"admin 角色权限码应通配展开（Casbin bypass 语义），不受 role_menus 清空影响")

	// 对照：普通角色仅按勾选下发
	var viewerRoleID int64
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO roles (code, name, priority) VALUES ('gm_viewer', '访客', 30) RETURNING id`).Scan(&viewerRoleID))
	viewerUser := &model.User{Username: "perm_viewer", EmployeeNo: "E660011", Password: hash, Status: 1}
	require.NoError(t, userRepo.Create(ctx, viewerUser))
	require.NoError(t, userRepo.SetRoles(ctx, viewerUser.ID, []int64{viewerRoleID}))
	perms, err = svc.GetUserPermissions(ctx, viewerUser.ID)
	require.NoError(t, err)
	assert.NotContains(t, perms, "button:gm:test", "普通角色未勾选不应下发该权限码")
}

// B4-4 守护：CreateRole 显式 status=0 可创建禁用角色（指针化契约变更）
func TestRBACService_CreateRoleExplicitDisabled(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	svc := newRBACServiceForTest(t)

	saRole := rbacTestRole(t, "superadmin", 1, false)
	saActorID := rbacTestUser(t, "sa_user", "E660020", []int64{saRole.ID})

	// 未传 status → 默认启用
	role, err := svc.CreateRole(ctx, &model.CreateRoleRequest{
		Code: "gm_default", Name: "默认启用", Priority: 20,
	}, saActorID)
	require.NoError(t, err)
	assert.Equal(t, 1, role.Status, "未传 status 应默认启用")

	// 显式传 0 → 创建即禁用（原零值合并导致不可能）
	zero := 0
	role, err = svc.CreateRole(ctx, &model.CreateRoleRequest{
		Code: "gm_disabled", Name: "创建即禁用", Priority: 20, Status: &zero,
	}, saActorID)
	require.NoError(t, err)
	assert.Equal(t, 0, role.Status, "显式传 0 应创建禁用角色")
}

// resetOrgTables 组织/菜单/角色全清（B4 守护测试用）
func resetOrgTables(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		TRUNCATE user_orgs, user_roles, users, role_menus, menus, roles RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
}
