package middleware

import (
	"fmt"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
)

// RoleFetcher 获取用户角色（避免直接依赖 repository 包）
type RoleFetcher interface {
	GetRoleCodesByUserID(userID int64) ([]string, error)
}

type selfServiceRoute struct {
	method string
	path   string
}

// Phase 1 固定白名单；路径变更须同步 docs/modules/authz.md §2.2.1
var selfServiceRoutes = []selfServiceRoute{
	{method: "GET", path: "/api/v1/user/profile"},
	{method: "POST", path: "/api/v1/user/profile/update"},
	{method: "GET", path: "/api/v1/user/menus"},
	{method: "GET", path: "/api/v1/user/permissions"},
	{method: "POST", path: "/api/v1/auth/logout"},
	{method: "POST", path: "/api/v1/auth/password/update"},
}

// CasbinPassThrough Step 4 JWT 联调：跳过 Casbin，Step 5 替换为 CasbinAuth。
func CasbinPassThrough() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// CasbinAuth 路由级 RBAC 鉴权中间件（g 表消除 + 逐角色 enforce）
func CasbinAuth(enforcer *casbin.SyncedEnforcer, roleFetcher RoleFetcher) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetInt64("userID")

		roles, err := roleFetcher.GetRoleCodesByUserID(userID)
		if err != nil {
			response.InternalError(c, "获取用户角色失败")
			c.Abort()
			return
		}
		if len(roles) == 0 {
			response.ForbiddenError(c, errcode.ErrNoRoles)
			c.Abort()
			return
		}

		path := c.Request.URL.Path
		method := c.Request.Method
		if isSelfServiceRoute(method, path) {
			c.Set("roles", roles)
			c.Next()
			return
		}

		allowed := false
		for _, role := range roles {
			subject := fmt.Sprintf("role::%s", role)
			if ok, _ := enforcer.Enforce(subject, path, method); ok {
				allowed = true
				break
			}
		}

		if !allowed {
			response.ForbiddenError(c, errcode.ErrNoPermission)
			c.Abort()
			return
		}

		c.Set("roles", roles)
		c.Next()
	}
}

func isSelfServiceRoute(method, path string) bool {
	for _, r := range selfServiceRoutes {
		if r.method == method && r.path == path {
			return true
		}
	}
	return false
}
