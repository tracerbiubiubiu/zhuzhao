//go:build integration

package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/crypto"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func strPtr(s string) *string { return &s }

// B2-3 守护：Update patch 语义——未传字段保持原值，传空串显式清空。
// 修复前全量覆盖语义：按文档示例（部分字段）调用会清空 employee_no（登录键）等字段。
func TestUserService_UpdatePatchSemantics(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepo(testPool)

	hash, err := crypto.HashPassword("p")
	require.NoError(t, err)
	user := &model.User{
		Username: "patch_user", EmployeeNo: "E610001", Password: hash,
		RealName: "张三", Email: "zhang@corp.com", Phone: "13800000000", Status: 1,
	}
	require.NoError(t, userRepo.Create(ctx, user))

	// Update/UpdateProfile 仅依赖 userRepo；actor==target 走 ensureCanManage 直通，
	// 不触达 roleRepo/orgService，其余依赖传 nil 即可
	svc := service.NewUserService(testPool, userRepo, nil, nil, nil, nil)

	// 部分字段请求：只传 real_name——修复前会把 employee_no/phone/email 全清空
	updated, err := svc.Update(ctx, &model.UpdateUserRequest{
		ID: user.ID, Version: user.Version,
		RealName: strPtr("张三（新）"),
	}, user.ID) // actor == target：ensureCanManage 直通
	require.NoError(t, err)
	assert.Equal(t, "张三（新）", updated.RealName, "传入字段应更新")
	assert.Equal(t, "E610001", updated.EmployeeNo, "未传字段（登录键）必须保持原值")
	assert.Equal(t, "zhang@corp.com", updated.Email, "未传字段必须保持原值")
	assert.Equal(t, "13800000000", updated.Phone, "未传字段必须保持原值")

	// 显式传空串 = 清空该字段
	updated, err = svc.Update(ctx, &model.UpdateUserRequest{
		ID: user.ID, Version: updated.Version,
		Phone: strPtr(""),
	}, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "", updated.Phone, "空串应显式清空")
	assert.Equal(t, "E610001", updated.EmployeeNo, "其他未传字段仍保持原值")
}

// B2-3 守护：UpdateProfile patch 语义（自服务接口，仅本人可改）
func TestUserService_UpdateProfilePatchSemantics(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepo(testPool)
	svc := service.NewUserService(testPool, userRepo, nil, nil, nil, nil)

	hash, err := crypto.HashPassword("p")
	require.NoError(t, err)
	user := &model.User{
		Username: "profile_user", EmployeeNo: "E610002", Password: hash,
		RealName: "李四", Email: "lisi@corp.com", Phone: "13900000000", Status: 1,
	}
	require.NoError(t, userRepo.Create(ctx, user))

	// 只传 avatar——其余字段保持原值
	updated, err := svc.UpdateProfile(ctx, user.ID, &model.UpdateProfileRequest{
		Avatar: strPtr("https://cdn.example/a.png"),
	})
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example/a.png", updated.Avatar)
	assert.Equal(t, "李四", updated.RealName, "未传字段保持原值")
	assert.Equal(t, "lisi@corp.com", updated.Email, "未传字段保持原值")
	assert.Equal(t, "13900000000", updated.Phone, "未传字段保持原值")

	// 空串显式清空 email
	updated, err = svc.UpdateProfile(ctx, user.ID, &model.UpdateProfileRequest{
		Email: strPtr(""),
	})
	require.NoError(t, err)
	assert.Equal(t, "", updated.Email, "空串应显式清空")
	assert.Equal(t, "李四", updated.RealName, "其他未传字段仍保持原值")
}

// B4-5 守护：SetUserOrgs 重复 org_id 去重后 SetRolesTx 的活跃性防御——
// 绑定已软删角色不产生幽灵绑定（见 repository 层测试）。
// 此处覆盖 service 层 UpdateStatus 自我保护与 is_system 保护（B4-3）。
func TestUserService_UpdateStatusGuards(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepo(testPool)
	svc := service.NewUserService(testPool, userRepo, nil, nil, nil, nil)

	hash, err := crypto.HashPassword("p")
	require.NoError(t, err)
	me := &model.User{Username: "me", EmployeeNo: "E640001", Password: hash, Status: 1}
	require.NoError(t, userRepo.Create(ctx, me))

	// 禁用自己 → 403（ErrForbidden）
	err = svc.UpdateStatus(ctx, &model.UpdateUserStatusRequest{UserID: me.ID, Status: 0}, me.ID)
	requireErrCode(t, err, errcode.ErrForbidden)

	// is_system 用户禁用 → 403（ErrUserIsSystem）
	sysUser := &model.User{Username: "sys", EmployeeNo: "E640002", Password: hash, Status: 1, IsSystem: true}
	require.NoError(t, userRepo.Create(ctx, sysUser))
	err = svc.UpdateStatus(ctx, &model.UpdateUserStatusRequest{UserID: sysUser.ID, Status: 0}, me.ID)
	requireErrCode(t, err, errcode.ErrUserIsSystem)
}

// B2-5 守护：AssignMenus 菜单存在性/活跃性校验——不存在或已软删的 ID → 404。
func TestRBACService_AssignMenusMenuValidation(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	svc := newRBACServiceForTest(t)

	saRole := rbacTestRole(t, "superadmin", 1, false)
	saActorID := rbacTestUser(t, "sa_user", "E610101", []int64{saRole.ID})
	// admin 角色：AssignMenus 走 skip 分支（不触发 LoadPolicy，内存 enforcer 无 adapter）
	target := rbacTestRole(t, "admin", 10, true)

	// 空菜单列表 = 清空，应成功
	err := svc.AssignMenus(ctx, &model.AssignMenusRequest{
		RoleID: target.ID, MenuIDs: nil,
	}, saActorID)
	require.NoError(t, err)

	var menuID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO menus (code, name, menu_type, parent_id, sort_order, visible, is_system)
		VALUES ('m_b25', '测试页', 2, NULL, 1, true, false) RETURNING id`).Scan(&menuID))

	err = svc.AssignMenus(ctx, &model.AssignMenusRequest{
		RoleID: target.ID, MenuIDs: []int64{menuID},
	}, saActorID)
	require.NoError(t, err, "存在的活跃菜单应成功")

	// 不存在 ID → 404
	err = svc.AssignMenus(ctx, &model.AssignMenusRequest{
		RoleID: target.ID, MenuIDs: []int64{999999},
	}, saActorID)
	requireErrCode(t, err, errcode.ErrMenuNotFound)

	// 软删菜单 → 404（修复前软删菜单可写入 role_menus 产生脏绑定）
	_, err = testPool.Exec(ctx, `UPDATE menus SET deleted_at = NOW() WHERE id = $1`, menuID)
	require.NoError(t, err)
	err = svc.AssignMenus(ctx, &model.AssignMenusRequest{
		RoleID: target.ID, MenuIDs: []int64{menuID},
	}, saActorID)
	requireErrCode(t, err, errcode.ErrMenuNotFound)
}

// B2-6 守护：影子超管读路径——非 superadmin 读 superadmin 角色的
// 详情/菜单/策略三个接口均 404（防推断）；superadmin 自身可读。
func TestRBACService_ShadowSuperadminReadPaths(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	svc := newRBACServiceForTest(t)

	adminRole := rbacTestRole(t, "admin", 10, true)
	saRole := rbacTestRole(t, "superadmin", 1, true)
	adminActor := rbacTestUser(t, "admin_user", "E610111", []int64{adminRole.ID})
	saActor := rbacTestUser(t, "sa_user", "E610112", []int64{saRole.ID})

	// admin 读 superadmin 角色 → 三个接口均 404
	_, err := svc.GetRole(ctx, saRole.ID, adminActor)
	requireErrCode(t, err, errcode.ErrRoleNotFound)
	_, err = svc.GetRoleMenuIDs(ctx, saRole.ID, adminActor)
	requireErrCode(t, err, errcode.ErrRoleNotFound)
	_, err = svc.GetRolePermissions(ctx, saRole.ID, adminActor)
	requireErrCode(t, err, errcode.ErrRoleNotFound)

	// admin 读普通角色 → 200（不受影响）
	normal := rbacTestRole(t, "viewer", 30, false)
	_, err = svc.GetRole(ctx, normal.ID, adminActor)
	require.NoError(t, err)

	// superadmin 读 superadmin → 200
	_, err = svc.GetRole(ctx, saRole.ID, saActor)
	require.NoError(t, err)
	_, err = svc.GetRoleMenuIDs(ctx, saRole.ID, saActor)
	require.NoError(t, err)
	_, err = svc.GetRolePermissions(ctx, saRole.ID, saActor)
	require.NoError(t, err)
}
