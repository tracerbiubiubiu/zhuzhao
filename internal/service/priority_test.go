package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func rolesWithPriority(codes []string, ps ...int) []*model.Role {
	roles := make([]*model.Role, 0, len(ps))
	for i, p := range ps {
		code := "custom_role"
		if i < len(codes) {
			code = codes[i]
		}
		roles = append(roles, &model.Role{Code: code, Priority: p})
	}
	return roles
}

// F-2 特殊场景：角色 priority 提权防护。
// 修复前：CreateRole/UpdateRole 不校验 priority，priority=20 的角色可自改为 1/0，
// 越过 superadmin(priority=1) 底线后经 canManageTarget 重置任意管理员密码。
func TestCanSetRolePriority(t *testing.T) {
	tests := []struct {
		name        string
		actorRoles  []*model.Role
		newPriority int
		want        bool
	}{
		{"superadmin 可创建 priority=1 的角色", rolesWithPriority([]string{"superadmin"}, 1), 1, true},
		{"superadmin 不能创建 priority<1", rolesWithPriority([]string{"superadmin"}, 1), 0, false},
		{"admin 不能创建低于自身 priority 的角色", rolesWithPriority([]string{"admin"}, 10), 9, false},
		{"admin 可创建等于自身 priority 的角色", rolesWithPriority([]string{"admin"}, 10), 10, true},
		{"viewer 只能创建 >=30 的角色", rolesWithPriority([]string{"viewer"}, 30), 25, false},
		{"多角色取最高权限挡低 priority", rolesWithPriority([]string{"viewer", "admin"}, 30, 10), 5, false},
		{"多角色取最高权限允许中间值", rolesWithPriority([]string{"viewer", "admin"}, 30, 10), 15, true},
		{"负数 priority 一律拒绝", rolesWithPriority([]string{"superadmin"}, 1), -1, false},
		{"零角色（无角色用户）一律拒绝", nil, 10, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, canSetRolePriority(tt.actorRoles, tt.newPriority))
		})
	}
}

// F-2 纵深防御：priority 非法（<1）的角色即使因存量脏数据存在，也不得获得管理权。
// 修复前：effectivePriority=0 的 actor 对 superadmin(priority=1) 的 canManageTarget 返回 true。
func TestCanManageTarget_IllegalPriorityHasNoPower(t *testing.T) {
	actor := rolesWithPriority([]string{"custom"}, 0)      // 非法存量：priority=0
	target := rolesWithPriority([]string{"superadmin"}, 1) // superadmin
	assert.False(t, canManageTarget(actor, target),
		"priority<1 的角色不得管理 superadmin（提权链阻断的纵深防御）")
}

// 既有语义回归：正常优先级管理判断
func TestCanManageTarget_NormalSemantics(t *testing.T) {
	admin := rolesWithPriority([]string{"admin"}, 10)
	viewer := rolesWithPriority([]string{"viewer"}, 30)
	superadmin := rolesWithPriority([]string{"superadmin"}, 1)

	assert.True(t, canManageTarget(admin, viewer), "admin 可管理 viewer")
	assert.False(t, canManageTarget(viewer, admin), "viewer 不可管理 admin")
	assert.False(t, canManageTarget(admin, superadmin), "admin 不可管理 superadmin")
	assert.True(t, canManageTarget(superadmin, admin), "superadmin 可管理任何人")
	assert.False(t, canManageTarget(viewer, viewer), "同级不可互相管理")
}
