package router

import (
	"net/http"

	"log/slog"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jwt"
)

// Deps 路由依赖
type Deps struct {
	AuthHandler *handler.AuthHandler
	UserHandler *handler.UserHandler
	RoleHandler *handler.RoleHandler
	OrgHandler  *handler.OrgHandler
	MenuHandler *handler.MenuHandler

	JWTManager  *jwt.Manager
	Enforcer    *casbin.SyncedEnforcer
	RedisClient *redis.Client
	Logger      *slog.Logger
}

// New 创建 Gin 引擎并注册路由
func New(deps Deps) *gin.Engine {
	r := gin.New()

	// 全局中间件
	r.Use(middleware.RequestID())
	r.Use(middleware.Recovery(deps.Logger))
	r.Use(middleware.Logger(deps.Logger))

	// 健康检查
	r.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/health/ready", func(c *gin.Context) {
		// TODO: 检查 DB + Redis 连通性
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

		// 以下路由需要 JWT 认证
		authenticated := v1.Group("")
		authenticated.Use(middleware.JWT(deps.JWTManager, deps.RedisClient))
		{
			// 认证模块（需鉴权）
			authenticated.POST("/auth/logout", deps.AuthHandler.Logout)
			authenticated.GET("/auth/devices", deps.AuthHandler.ListDevices)
			authenticated.DELETE("/auth/devices/:deviceId", deps.AuthHandler.KickDevice)

			// 用户模块
			users := authenticated.Group("/users")
			{
				users.GET("", deps.UserHandler.List)
				users.POST("", deps.UserHandler.Create)
				users.GET("/:id", deps.UserHandler.Get)
				users.PUT("/:id", deps.UserHandler.Update)
				users.DELETE("/:id", deps.UserHandler.Delete)
			}

			// 当前用户信息
			userSelf := authenticated.Group("/user")
			{
				userSelf.GET("/menus", deps.UserHandler.GetMenus)
				userSelf.GET("/permissions", deps.UserHandler.GetPermissions)
			}

			// 角色模块
			roles := authenticated.Group("/roles")
			{
				roles.GET("", deps.RoleHandler.List)
				roles.POST("", deps.RoleHandler.Create)
				roles.GET("/:id", deps.RoleHandler.Get)
				roles.PUT("/:id", deps.RoleHandler.Update)
				roles.DELETE("/:id", deps.RoleHandler.Delete)
			}

			// 组织模块
			orgs := authenticated.Group("/orgs")
			{
				orgs.GET("", deps.OrgHandler.GetTree)
				orgs.POST("", deps.OrgHandler.Create)
				orgs.GET("/:id", deps.OrgHandler.Get)
				orgs.PUT("/:id", deps.OrgHandler.Update)
				orgs.DELETE("/:id", deps.OrgHandler.Delete)
				orgs.PATCH("/:id/move", deps.OrgHandler.Move)
			}

			// 菜单模块
			menus := authenticated.Group("/menus")
			{
				menus.GET("", deps.MenuHandler.GetTree)
				menus.POST("", deps.MenuHandler.Create)
				menus.GET("/:id", deps.MenuHandler.Get)
				menus.PUT("/:id", deps.MenuHandler.Update)
				menus.DELETE("/:id", deps.MenuHandler.Delete)
			}
		}
	}

	return r
}
