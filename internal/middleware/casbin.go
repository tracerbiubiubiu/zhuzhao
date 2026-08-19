package middleware

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/casbin/casbin/v3"
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
)

// RoleFetcher 获取用户角色（避免直接依赖 repository 包）。
// F-7：ctx 由中间件传入（c.Request.Context()），请求取消可传播到 DB 查询
type RoleFetcher interface {
	GetRoleCodesByUserID(ctx context.Context, userID int64) ([]string, error)
}

// SelfServiceContextKey 自服务路由标记的 context key（导出供测试引用，
// 防止字符串字面量与实现脱钩后测试依然全绿）
const SelfServiceContextKey = "self_service"

// SelfService 标记路由为自服务（不需 Casbin 策略，任何已认证有角色用户可访问）。
// 在路由注册时挂载到对应 RouterGroup，替代硬编码路径白名单。
// 注意：必须注册在 CasbinAuth 之前（router_test.go 静态断言此顺序）。
func SelfService() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(SelfServiceContextKey, true)
		c.Next()
	}
}

// CasbinPassThrough Step 4 JWT 联调：跳过 Casbin，Step 5 替换为 CasbinAuth。
func CasbinPassThrough() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// CasbinAuth 路由级 RBAC 鉴权中间件（g 表消除 + 逐角色 enforce）。
// logger 为 nil 时使用 slog.Default()；enforce 错误记 Error、最终拒绝记 Warn（§2.1 修复）
func CasbinAuth(enforcer *casbin.SyncedEnforcer, roleFetcher RoleFetcher, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		logger := logger
		if logger == nil {
			logger = slog.Default()
		}

		userID := c.GetInt64("userID")

		roles, err := roleFetcher.GetRoleCodesByUserID(c.Request.Context(), userID)
		if err != nil {
			logger.Error("casbin fetch roles failed",
				slog.Any("error", err), slog.Int64("userID", userID),
				slog.String("path", c.Request.URL.Path))
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
		if _, ok := c.Get(SelfServiceContextKey); ok {
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
				// 单角色 enforce 失败继续尝试其余角色，但必须留痕
				logger.Error("casbin enforce error",
					slog.Any("error", err), slog.String("subject", subject),
					slog.String("path", path), slog.String("method", method))
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
			// 被拒请求留痕：谁在何时被拒绝了什么（安全审计刚需）
			logger.Warn("casbin denied",
				slog.Int64("userID", userID),
				slog.String("username", c.GetString("username")),
				slog.String("path", path), slog.String("method", method),
				slog.Any("roles", roles))
			response.ForbiddenError(c, errcode.ErrNoPermission)
			c.Abort()
			return
		}

		c.Set("roles", roles)
		c.Next()
	}
}
