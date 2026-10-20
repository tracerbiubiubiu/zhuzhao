package handler

import "github.com/gin-gonic/gin"

// rolesFromGin 从 gin context 提取角色码列表（CasbinAuth c.Set("roles", []string)）。
func rolesFromGin(c *gin.Context) []string {
	v, ok := c.Get("roles")
	if !ok {
		return nil
	}
	roles, ok := v.([]string)
	if !ok {
		return nil
	}
	return roles
}
