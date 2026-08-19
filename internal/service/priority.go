package service

import "github.com/tracerbiubiubiu/zhuzhao/internal/model"

const superadminCode = "superadmin"

func effectivePriority(roles []*model.Role) int {
	if len(roles) == 0 {
		return 9999
	}
	minP := roles[0].Priority
	for _, r := range roles[1:] {
		if r.Priority < minP {
			minP = r.Priority
		}
	}
	return minP
}

func hasRoleCode(roles []*model.Role, code string) bool {
	for _, r := range roles {
		if r.Code == code {
			return true
		}
	}
	return false
}

func isSuperadmin(roles []*model.Role) bool {
	return hasRoleCode(roles, "superadmin")
}

// canManageTarget actor 能否对 target 做敏感写操作（严格更强）
func canManageTarget(actorRoles, targetRoles []*model.Role) bool {
	if isSuperadmin(actorRoles) {
		return true
	}
	ap := effectivePriority(actorRoles)
	// F-2 纵深防御：priority<1 的非法角色（历史脏数据）不授予任何管理权，
	// 否则 priority=0 的角色可越过 superadmin(priority=1) 的数字比较
	if ap < 1 {
		return false
	}
	return ap < effectivePriority(targetRoles)
}

// canAssignRole actor 能否分配 role
func canAssignRole(actorRoles []*model.Role, role *model.Role) bool {
	if role.Code == "superadmin" && !isSuperadmin(actorRoles) {
		return false
	}
	return role.Priority >= effectivePriority(actorRoles)
}

// canSetRolePriority actor 能否创建/更新出指定 priority 的角色（F-2 修复）。
// 规则：priority >= 1（不越过 superadmin 的底线），且不高于操作者自身权限档位。
// 修复前：CreateRole/UpdateRole 不校验 priority，priority=20 的自定义角色可自改为
// 0/1，再经 canManageTarget 重置管理员密码完成提权。
func canSetRolePriority(actorRoles []*model.Role, priority int) bool {
	if priority < 1 {
		return false
	}
	return priority >= effectivePriority(actorRoles)
}
