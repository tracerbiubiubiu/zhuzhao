//go:build integration

package service_test

import (
	"context"
	"testing"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/crypto"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func resetRBACTables(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		TRUNCATE user_roles, users, role_menus RESTART IDENTITY CASCADE;
		DELETE FROM menus WHERE is_system = false;
		DELETE FROM roles`)
	require.NoError(t, err)
}

func rbacTestRole(t *testing.T, code string, priority int, isSystem bool) *model.Role {
	t.Helper()
	var id int64
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO roles (code, name, priority, is_system)
		VALUES ($1, $1, $2, $3) RETURNING id`, code, priority, isSystem).Scan(&id)
	require.NoError(t, err)
	return &model.Role{ID: id, Code: code, Priority: priority, IsSystem: isSystem, Status: 1}
}

func rbacTestUser(t *testing.T, username, employeeNo string, roleIDs []int64) int64 {
	t.Helper()
	hash, err := crypto.HashPassword("p")
	require.NoError(t, err)
	repo := repository.NewUserRepo(testPool)
	user := &model.User{Username: username, EmployeeNo: employeeNo, Password: hash, Status: 1}
	require.NoError(t, repo.Create(context.Background(), user))
	if len(roleIDs) > 0 {
		require.NoError(t, repo.SetRoles(context.Background(), user.ID, roleIDs))
	}
	return user.ID
}

// rbacTestEnforcer 内存 enforcer（无 adapter）：仅满足 RBACService 构造依赖；
// 目标校验失败路径在触达 enforcer 前返回，成功路径选 admin 角色跳过 LoadPolicy。
func rbacTestEnforcer(t *testing.T) *casbin.SyncedEnforcer {
	t.Helper()
	m, err := casbinmodel.NewModelFromString(`
[request_definition]
r = sub, obj, act
[policy_definition]
p = sub, obj, act
[policy_effect]
e = some(where (p.eft == allow))
[matchers]
m = r.sub == p.sub && r.obj == p.obj && r.act == p.act
`)
	require.NoError(t, err)
	e, err := casbin.NewSyncedEnforcer(m)
	require.NoError(t, err)
	return e
}

func newRBACServiceForTest(t *testing.T) *service.RBACService {
	t.Helper()
	return service.NewRBACService(
		repository.NewRoleRepo(testPool),
		repository.NewUserRepo(testPool),
		repository.NewMenuRepo(testPool),
		rbacTestEnforcer(t),
	)
}

func requireErrCode(t *testing.T, err error, want *errcode.Error) {
	t.Helper()
	require.Error(t, err)
	var biz *errcode.Error
	require.ErrorAs(t, err, &biz)
	assert.Equal(t, want.Code, biz.Code)
}

// B1-2 守护：角色写操作目标校验 403 矩阵。
// 持有 role:write 类策略的低权自定义角色（priority=25）不得破坏更强角色：
// 删除 / 降权 / 改菜单均返回 403 + 30010（ErrCannotManageHigher）。
func TestRBACService_EnsureCanManageRoleMatrix(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	svc := newRBACServiceForTest(t)

	lowRole := rbacTestRole(t, "low_mgr", 25, false)   // 操作者角色（弱）
	highRole := rbacTestRole(t, "high_mgr", 15, false) // 目标角色（强）
	actorID := rbacTestUser(t, "actor", "E600001", []int64{lowRole.ID})

	// 删除更强角色 → 30010
	err := svc.DeleteRole(ctx, highRole.ID, actorID)
	requireErrCode(t, err, errcode.ErrCannotManageHigher)

	// 更新（降权）更强角色 → 30010
	_, err = svc.UpdateRole(ctx, &model.UpdateRoleRequest{
		ID: highRole.ID, Version: 1, Name: "x", Priority: 25,
	}, actorID)
	requireErrCode(t, err, errcode.ErrCannotManageHigher)

	// 给更强角色分配菜单 → 30010
	err = svc.AssignMenus(ctx, &model.AssignMenusRequest{
		RoleID: highRole.ID, MenuIDs: nil,
	}, actorID)
	requireErrCode(t, err, errcode.ErrCannotManageHigher)

	// 同级目标（priority 相同）→ 严格更强语义，同样拒绝
	peerRole := rbacTestRole(t, "peer_mgr", 25, false)
	err = svc.DeleteRole(ctx, peerRole.ID, actorID)
	requireErrCode(t, err, errcode.ErrCannotManageHigher)
}

// B1-2 守护：非 superadmin 对系统角色分配菜单 → 403 + 40004（ErrRoleIsSystem）。
// 此前仅挡 superadmin 角色；admin 等系统角色无保护，可被低权角色清空菜单。
func TestRBACService_AssignMenus_SystemRoleRequiresSuperadmin(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	svc := newRBACServiceForTest(t)

	adminRole := rbacTestRole(t, "admin", 10, true)
	adminActorID := rbacTestUser(t, "admin_user", "E600011", []int64{adminRole.ID})

	// admin（非 superadmin）改系统角色菜单 → 40004
	err := svc.AssignMenus(ctx, &model.AssignMenusRequest{
		RoleID: adminRole.ID, MenuIDs: nil,
	}, adminActorID)
	requireErrCode(t, err, errcode.ErrRoleIsSystem)

	// superadmin 放行（canManageTarget 直通；admin 角色跳过策略写入，无需 LoadPolicy）
	saRole := rbacTestRole(t, "superadmin", 1, true)
	saActorID := rbacTestUser(t, "sa_user", "E600012", []int64{saRole.ID})
	err = svc.AssignMenus(ctx, &model.AssignMenusRequest{
		RoleID: adminRole.ID, MenuIDs: nil,
	}, saActorID)
	require.NoError(t, err, "superadmin 应可修改系统角色菜单")
}

// B1-2 守护：DeleteRole/UpdateRole 对系统角色仍返回 40004（原有行为不回归）。
func TestRBACService_SystemRoleDeleteAndUpdateRejected(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	svc := newRBACServiceForTest(t)

	adminRole := rbacTestRole(t, "admin", 10, true)
	saRole := rbacTestRole(t, "superadmin", 1, true)
	saActorID := rbacTestUser(t, "sa_user", "E600021", []int64{saRole.ID})

	err := svc.DeleteRole(ctx, adminRole.ID, saActorID)
	requireErrCode(t, err, errcode.ErrRoleIsSystem)

	_, err = svc.UpdateRole(ctx, &model.UpdateRoleRequest{
		ID: adminRole.ID, Version: 1, Name: "x", Priority: 10,
	}, saActorID)
	requireErrCode(t, err, errcode.ErrRoleIsSystem)
}
