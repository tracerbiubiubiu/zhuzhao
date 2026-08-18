package router

import (
	"net/http"

	"log/slog"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jwt"
)

// Deps 路由依赖
type Deps struct {
	AuthHandler  *handler.AuthHandler
	UserHandler  *handler.UserHandler
	RoleHandler  *handler.RoleHandler
	OrgHandler   *handler.OrgHandler
	MenuHandler  *handler.MenuHandler
	AuditHandler *handler.AuditHandler

	JWTManager   *jwt.Manager
	Enforcer     *casbin.SyncedEnforcer
	RedisClient  *redis.Client
	DBPool       *pgxpool.Pool
	Logger       *slog.Logger
	RoleFetcher   middleware.RoleFetcher
	AuditService  middleware.AuditLogger
}

// New 创建 Gin 引擎并注册路由
func New(deps Deps) *gin.Engine {
	r := gin.New()

	// 全局中间件
	r.Use(middleware.Recovery(deps.Logger))
	r.Use(middleware.RequestID())
	r.Use(middleware.AccessLogger(deps.Logger))
	r.Use(middleware.CORS())
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.BodyLimit(1 << 20)) // 1MB

	// 健康检查
	r.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/health/ready", func(c *gin.Context) {
		if err := deps.DBPool.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "component": "db"})
			return
		}
		if err := deps.RedisClient.Ping(c.Request.Context()).Err(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "component": "redis"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// API v1
	v1 := r.Group("/api/v1")
	{
		// 认证模块（无需鉴权）
		auth := v1.Group("/auth")
		{
			auth.POST("/login", deps.AuthHandler.Login)
			auth.POST("/refresh", deps.AuthHandler.Refresh)
		}

		// 以下路由需要 JWT 认证 + Casbin 鉴权 + 审计日志
		authed := v1.Group("")
		authed.Use(
			middleware.JWT(deps.JWTManager, deps.RedisClient),
			middleware.CasbinAuth(deps.Enforcer, deps.RoleFetcher),
			middleware.AuditLog(deps.AuditService),
		)
		{
			// 认证模块（需鉴权）
			authed.POST("/auth/logout", deps.AuthHandler.Logout)
			authed.POST("/auth/password/update", deps.AuthHandler.UpdatePassword)

			// 用户模块
			users := authed.Group("/users")
			{
				users.GET("", deps.UserHandler.List)
				users.POST("", deps.UserHandler.Create)
				users.GET("/:id", deps.UserHandler.Get)
				users.POST("/update", deps.UserHandler.Update)
				users.POST("/delete", deps.UserHandler.Delete)
				users.POST("/status", deps.UserHandler.UpdateStatus)
				users.POST("/roles", deps.UserHandler.SetRoles)
				users.POST("/password/reset", deps.UserHandler.ResetPassword)
				users.POST("/orgs", deps.UserHandler.SetUserOrgs)
				users.GET("/:id/orgs", deps.UserHandler.GetUserOrgs)
			}

			// 当前用户信息
			userSelf := authed.Group("/user")
			{
				userSelf.GET("/profile", deps.UserHandler.GetProfile)
				userSelf.POST("/profile/update", deps.UserHandler.UpdateProfile)
				userSelf.GET("/menus", deps.UserHandler.GetMenus)
				userSelf.GET("/permissions", deps.UserHandler.GetPermissions)
			}

			// 角色模块
			roles := authed.Group("/roles")
			{
				roles.GET("", deps.RoleHandler.List)
				roles.POST("", deps.RoleHandler.Create)
				roles.GET("/:id", deps.RoleHandler.Get)
				roles.POST("/update", deps.RoleHandler.Update)
				roles.POST("/delete", deps.RoleHandler.Delete)
				roles.POST("/menus", deps.RoleHandler.AssignMenus)
				roles.GET("/:id/menus", deps.RoleHandler.GetMenus)
				roles.GET("/:id/permissions", deps.RoleHandler.GetPermissions)
			}

			// 组织模块
			orgs := authed.Group("/orgs")
			{
				orgs.GET("", deps.OrgHandler.GetTree)
				orgs.POST("", deps.OrgHandler.Create)
				orgs.GET("/:id", deps.OrgHandler.Get)
				orgs.POST("/update", deps.OrgHandler.Update)
				orgs.POST("/delete", deps.OrgHandler.Delete)
				orgs.POST("/move", deps.OrgHandler.Move)
				orgs.GET("/:id/members", deps.OrgHandler.GetMembers)
				orgs.POST("/members", deps.OrgHandler.AddMember)
				orgs.POST("/members/delete", deps.OrgHandler.RemoveMember)
			}

			// 菜单模块
			menus := authed.Group("/menus")
			{
				menus.GET("", deps.MenuHandler.GetTree)
				menus.POST("", deps.MenuHandler.Create)
				menus.GET("/:id", deps.MenuHandler.Get)
				menus.POST("/update", deps.MenuHandler.Update)
				menus.POST("/delete", deps.MenuHandler.Delete)
			}

			// 审计日志
			audit := authed.Group("/audit")
			{
				audit.GET("/logs", deps.AuditHandler.ListLogs)
			}
		}
	}

	return r
}
