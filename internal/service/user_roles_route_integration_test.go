//go:build integration

package service_test

// P2-5（复检遗留）：GET /users/:id/roles 反查端点——用户页「分配角色」对话框
// 回显数据源（替代「工号精确+角色过滤」N+1 推导）。
// 语义对齐 ensureVisible（与 GetUserOrgs 同款）：目标为 superadmin 且 actor 非
// superadmin → 404；其余放行（路由级边界在 L1 Casbin——端点挂 system_user 页）。

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestUserService_GetUserRoleIDs(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	suffix := uniqueSuffix() + "p25"

	// superadmin 角色 + admin/目标/旁人三用户
	superRole := rbacTestRole(t, "superadmin_"+suffix, 1, true)
	_ = superRole
	saRole := rbacTestRole(t, "superadmin", 1, true)
	r1 := rbacTestRole(t, "p25_r1_"+suffix, 50, false)
	r2 := rbacTestRole(t, "p25_r2_"+suffix, 60, false)
	adminID := rbacTestUser(t, "p25_admin_"+suffix, "P25A"+suffix, []int64{saRole.ID})
	targetID := rbacTestUser(t, "p25_target_"+suffix, "P25T"+suffix, []int64{r1.ID, r2.ID})
	bystanderID := rbacTestUser(t, "p25_by_"+suffix, "P25B"+suffix, nil)
	superTargetID := rbacTestUser(t, "p25_superu_"+suffix, "P25S"+suffix, []int64{saRole.ID})

	userRepo := repository.NewUserRepo(testPool)
	roleRepo := repository.NewRoleRepo(testPool)
	svc := service.NewUserService(testPool, userRepo, roleRepo, nil, nil, nil)

	// 精确集合（管理视角）
	ids, err := svc.GetUserRoleIDs(ctx, targetID, adminID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{r1.ID, r2.ID}, ids)

	// 自见放行
	ids, err = svc.GetUserRoleIDs(ctx, targetID, targetID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{r1.ID, r2.ID}, ids)

	// 普通旁人查普通目标：ensureVisible 放行（语义同 GetUserOrgs——真边界在 L1）
	ids, err = svc.GetUserRoleIDs(ctx, targetID, bystanderID)
	require.NoError(t, err)
	assert.Len(t, ids, 2)

	// 普通 actor 查 superadmin 目标 → 404（防探测）
	_, err = svc.GetUserRoleIDs(ctx, superTargetID, bystanderID)
	requireErrCode(t, err, errcode.ErrUserNotFound)

	// 未绑定用户 → 空集非错误
	ids, err = svc.GetUserRoleIDs(ctx, bystanderID, adminID)
	require.NoError(t, err)
	assert.Empty(t, ids)

}
