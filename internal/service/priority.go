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

// directManagementPriority requires actual enabled direct role facts. The
// target's legacy empty-set sentinel must never become an actor capability.
func directManagementPriority(roles []*model.Role) (int, bool) {
	if len(roles) == 0 {
		return 0, false
	}
	minP := 0
	for _, role := range roles {
		if role == nil || role.Status != 1 || role.DeletedAt != nil || role.Priority < 1 {
			return 0, false
		}
		if minP == 0 || role.Priority < minP {
			minP = role.Priority
		}
	}
	return minP, true
}

// canManageTarget actor 能否对 target 做敏感写操作（严格更强）
func canManageTarget(actorRoles, targetRoles []*model.Role) bool {
	ap, qualified := directManagementPriority(actorRoles)
	if !qualified {
		return false
	}
	if isSuperadmin(actorRoles) {
		return true
	}
	// An account without any target role is the weak side for every qualified
	// actor; keep this local rather than changing effectivePriority globally.
	if len(targetRoles) == 0 {
		return true
	}
	return ap < effectivePriority(targetRoles)
}

// canAssignRole actor 能否分配 role
func canAssignRole(actorRoles []*model.Role, role *model.Role) bool {
	ap, qualified := directManagementPriority(actorRoles)
	if !qualified || role == nil {
		return false
	}
	if role.Code == "superadmin" && !isSuperadmin(actorRoles) {
		return false
	}
	return role.Priority >= ap
}

// canSetRolePriority actor 能否创建/更新出指定 priority 的角色（F-2 修复）。
// 规则：priority >= 1（不越过 superadmin 的底线），且不高于操作者自身权限档位。
// 修复前：CreateRole/UpdateRole 不校验 priority，priority=20 的自定义角色可自改为
// 0/1，再经 canManageTarget 重置管理员密码完成提权。
func canSetRolePriority(actorRoles []*model.Role, priority int) bool {
	if priority < 1 {
		return false
	}
	ap, qualified := directManagementPriority(actorRoles)
	return qualified && priority >= ap
}
