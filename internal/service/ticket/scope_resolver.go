package ticket

// --- 角色辅助 ---

// HasRole 检查角色列表中是否含目标角色（admin/superadmin bypass 用）
func HasRole(roles []string, target string) bool {
	for _, r := range roles {
		if r == target || r == "role::"+target {
			return true
		}
	}
	return false
}
