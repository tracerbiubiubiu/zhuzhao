//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/crypto"
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
