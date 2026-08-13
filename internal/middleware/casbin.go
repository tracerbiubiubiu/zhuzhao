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

// CasbinAuth 路由级 RBAC 鉴权中间件（g 表消除 + 逐角色 enforce）
func CasbinAuth(enforcer *casbin.SyncedEnforcer, roleFetcher RoleFetcher) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetInt64("userID")

		// 1. 获取用户角色 codes（Phase 1 只查直接角色）
		roles, err := roleFetcher.GetRoleCodesByUserID(userID)
		if err != nil {
			response.InternalError(c, "获取用户角色失败")
			c.Abort()
			return
		}
		if len(roles) == 0 {
			response.Forbidden(c, errcode.ErrNoPermission.Message)
			c.Abort()
			return
		}

		// 2. 逐角色 enforce（superadmin/admin 在 matcher 中自动 bypass）
		path := c.Request.URL.Path
		method := c.Request.Method
		allowed := false
		for _, role := range roles {
			subject := fmt.Sprintf("role::%s", role)
			if ok, _ := enforcer.Enforce(subject, path, method); ok {
				allowed = true
				break
			}
		}

		if !allowed {
			response.Forbidden(c, errcode.ErrNoPermission.Message)
			c.Abort()
			return
		}

		// 3. 存入 context 供 handler 复用
		c.Set("roles", roles)
		c.Next()
	}
}
