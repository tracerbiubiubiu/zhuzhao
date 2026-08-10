package middleware

import (
	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
)

// Casbin 路由级 RBAC 鉴权中间件
func Casbin(enforcer *casbin.SyncedEnforcer) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		path := c.Request.URL.Path
		method := c.Request.Method

		pass, err := enforcer.Enforce(role, path, method)
		if err != nil {
			response.InternalError(c, "鉴权服务异常")
			c.Abort()
			return
		}

		if !pass {
			response.Forbidden(c, "无权限访问该接口")
			c.Abort()
			return
		}

		c.Next()
	}
}
