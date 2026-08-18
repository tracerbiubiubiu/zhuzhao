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
	return effectivePriority(actorRoles) < effectivePriority(targetRoles)
}

// canAssignRole actor 能否分配 role
func canAssignRole(actorRoles []*model.Role, role *model.Role) bool {
	if role.Code == "superadmin" && !isSuperadmin(actorRoles) {
		return false
	}
	return role.Priority >= effectivePriority(actorRoles)
}
