package middleware

import (
	"fmt"

	"github.com/casbin/casbin/v3"
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
)

// RoleFetcher 获取用户角色（避免直接依赖 repository 包）
type RoleFetcher interface {
	GetRoleCodesByUserID(userID int64) ([]string, error)
}

const selfServiceContextKey = "self_service"

// SelfService 标记路由为自服务（不需 Casbin 策略，任何已认证有角色用户可访问）。
// 在路由注册时挂载到对应 RouterGroup，替代硬编码路径白名单。
func SelfService() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(selfServiceContextKey, true)
		c.Next()
	}
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

		// 自服务路由（由 SelfService 中间件标记）跳过 Casbin enforce
		if _, ok := c.Get(selfServiceContextKey); ok {
			c.Set("roles", roles)
			c.Next()
			return
		}

		path := c.Request.URL.Path
		method := c.Request.Method
		allowed := false
		var enforceErr error
		for _, role := range roles {
			subject := fmt.Sprintf("role::%s", role)
			ok, err := enforcer.Enforce(subject, path, method)
			if err != nil {
				enforceErr = err
				continue
			}
			if ok {
				allowed = true
				break
			}
		}

		if enforceErr != nil && !allowed {
			response.ServiceUnavailable(c)
			c.Abort()
			return
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
