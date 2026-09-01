package router

import (
	"fmt"
	"net/http"

	"log/slog"

	"github.com/casbin/casbin/v3"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
)

// Deps 路由依赖
type Deps struct {
	AuthHandler   *handler.AuthHandler
	UserHandler   *handler.UserHandler
	RoleHandler   *handler.RoleHandler
	OrgHandler    *handler.OrgHandler
	MenuHandler   *handler.MenuHandler
	AuditHandler  *handler.AuditHandler
	TicketHandler *handler.TicketHandler

	JWTManager   *jwt.Manager
	Enforcer     *casbin.SyncedEnforcer
	RedisClient  *redis.Client
	DBPool       *pgxpool.Pool
	Logger       *slog.Logger
	RoleFetcher  middleware.RoleFetcher
	AuditService middleware.AuditLogger
	Registry     resource.Registry

	// TrustedProxies 信任的反向代理网段（B1-4）；空切片 = 不信任任何代理
	TrustedProxies []string
}

// New 创建 Gin 引擎并注册路由
func New(deps Deps) *gin.Engine {
	r := gin.New()

	// B1-4：ClientIP 信任链——不配置则不信任任何代理（X-Forwarded-For 不参与
	// 解析），防伪造审计 IP / last_login_ip；Nginx 前置时按内网网段配置
	if err := r.SetTrustedProxies(deps.TrustedProxies); err != nil {
		// 网段格式非法属部署配置错误：fail-fast 比带病运行更安全
		panic(fmt.Sprintf("invalid trusted_proxies config: %v", err))
	}

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
			middleware.AuditLog(deps.AuditService),
		)
		{
			// 自服务路由（Casbin 白名单：任何已认证有角色用户可访问）
			// 注意：SelfService 必须先于 CasbinAuth 注册（router_test.go 行为测试守护此顺序）
			selfService := authed.Group("")
			selfService.Use(middleware.SelfService(), middleware.CasbinAuth(deps.Enforcer, deps.RoleFetcher, deps.Logger))
			{
				// 认证模块（需鉴权）
				selfService.POST("/auth/logout", deps.AuthHandler.Logout)
				selfService.POST("/auth/password/update", deps.AuthHandler.UpdatePassword)

				// 当前用户信息
				userSelf := selfService.Group("/user")
				{
					userSelf.GET("/profile", deps.UserHandler.GetProfile)
					userSelf.POST("/profile/update", deps.UserHandler.UpdateProfile)
					userSelf.GET("/menus", deps.UserHandler.GetMenus)
					userSelf.GET("/permissions", deps.UserHandler.GetPermissions)
				}
			}

			// 2c 委托路由（04 §3.1「org:update 或 effective owner/admin」语义）：
			// L1 仅要求认证（SelfService 标记跳过 Casbin enforce——owner/admin 不是
			// Casbin 角色，纯菜单 L1 会把无全局菜单的委托者挡在 L3 之前）；
			// 全局权（org 管理权限码）与组内委托矩阵统一在 OrgService L3 判定。
			// 注意：必须挂 authed 级且标记先于 CasbinAuth（与上 selfService 同模式），
			// 挂 biz 子组时组级 Casbin 先于子组标记执行
			orgDelegated := authed.Group("/orgs")
			orgDelegated.Use(middleware.SelfService(), middleware.CasbinAuth(deps.Enforcer, deps.RoleFetcher, deps.Logger))
			{
				orgDelegated.POST("/delete", deps.OrgHandler.Delete)
				orgDelegated.POST("/members", deps.OrgHandler.AddMember)
				orgDelegated.POST("/members/role", deps.OrgHandler.SetMemberRole)
				orgDelegated.POST("/members/scope", deps.OrgHandler.SetMemberScope)
				orgDelegated.POST("/owners", deps.OrgHandler.SetOwners)
				orgDelegated.POST("/members/delete", deps.OrgHandler.RemoveMember)
			}

			// 业务路由（需 Casbin 策略）
			biz := authed.Group("")
			biz.Use(middleware.CasbinAuth(deps.Enforcer, deps.RoleFetcher, deps.Logger))
			{
				// 用户模块
				users := biz.Group("/users")
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

				// 角色模块
				roles := biz.Group("/roles")
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
				orgs := biz.Group("/orgs")
				{
					orgs.GET("", deps.OrgHandler.GetTree)
					orgs.POST("", deps.OrgHandler.Create)
					orgs.GET("/:id", deps.OrgHandler.Get)
					orgs.POST("/update", deps.OrgHandler.Update)
					orgs.POST("/move", deps.OrgHandler.Move)
					orgs.GET("/:id/members", deps.OrgHandler.GetMembers)

				}

				// 菜单模块
				menus := biz.Group("/menus")
				{
					menus.GET("", deps.MenuHandler.GetTree)
					menus.POST("", deps.MenuHandler.Create)
					menus.GET("/:id", deps.MenuHandler.Get)
					menus.POST("/update", deps.MenuHandler.Update)
					menus.POST("/delete", deps.MenuHandler.Delete)
				}

				// 审计日志
				audit := biz.Group("/audit")
				{
					audit.GET("/logs", deps.AuditHandler.ListLogs)
				}

				// 工单模块（Phase 2a）
				tickets := biz.Group("/tickets")
				{
					tickets.GET("", deps.TicketHandler.List)
					tickets.POST("", deps.TicketHandler.Create)
					tickets.GET("/:id", deps.TicketHandler.Get)
					tickets.POST("/update", deps.TicketHandler.Update)
					tickets.POST("/close", deps.TicketHandler.Close)
					tickets.POST("/assign", deps.TicketHandler.Assign)
					tickets.POST("/delete", deps.TicketHandler.Delete)
					tickets.GET("/:id/comments", deps.TicketHandler.ListComments)
					tickets.POST("/comments", deps.TicketHandler.CreateComment)
					tickets.POST("/notes", deps.TicketHandler.CreateNote)
					tickets.GET("/:id/relations", deps.TicketHandler.ListRelations)
					tickets.POST("/relations", deps.TicketHandler.CreateRelation)
				}

				// 工单元数据（类型/模板）
				ticketMeta := biz.Group("")
				{
					ticketMeta.GET("/ticket-types", deps.TicketHandler.ListTicketTypes)
					ticketMeta.GET("/ticket-types/:code/fields", deps.TicketHandler.ListTicketTypeFields)
					ticketMeta.GET("/ticket-templates", deps.TicketHandler.ListTicketTemplates)
					ticketMeta.GET("/ticket-templates/:code", deps.TicketHandler.GetTicketTemplate)
				}
			}
		}
	}

	return r
}
