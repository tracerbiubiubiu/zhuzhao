package router

import (
	"fmt"
	"net/http"

	"log/slog"

	"github.com/casbin/casbin/v3"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk/compat"
	"github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/gateway"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
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

	// phase5 S5 B5-1：登录态自服务面（avatar/*_create purpose——A.7）；
	// 禁用态（storage.enabled=false）路由仍挂载、服务层显式拒绝
	StorageHandler *handler.StorageHandler

	JWTManager   *jwt.Manager
	Enforcer     *casbin.SyncedEnforcer
	RedisClient  *redis.Client
	DBPool       *pgxpool.Pool
	Logger       *slog.Logger
	RoleFetcher  middleware.RoleFetcher
	AuditService middleware.AuditLogger
	Registry     resource.Registry

	// E-②：内网回调端点（/internal/jobs/callback，AK/SK 验签 + 专用网络拓扑；C10：action_id 在 body）。
	// Enabled=false（默认）不挂路由；SK 缺失已在 config.Load 拒绝启动（fail-closed）。
	JobsHandler  *handler.JobsHandler
	InternalJobs config.InternalJobsConfig

	// 存量协议兼容层（/compat 路由组，utils aksk/compat 验签；翻案决策见
	// phase1/02-auth §存量迁移翻案追记）。Enabled=false（默认）不挂路由；
	// 密钥环空/含空 SK 已在 config.Validate 拒启（fail-closed）。
	CompatAKSK config.CompatAKSKConfig

	// E-④：任务管理代理端点（biz 组，三层校验后出站 taskrunner）
	TaskrunnerHandler *handler.TaskrunnerHandler

	// P4-2 通知通道：管理 API（biz 组）+ 内网死信告警入口（internal 组）
	NotificationHandler *handler.NotificationHandler

	// P4-3 字典管理面（业务枚举运行时化——不碰权限策略面）
	DictHandler *handler.DictHandler

	// P4-6 PAT 自服务面（SelfService）+ JWT 中间件 zpat_ 分支消费的查验口
	PatHandler *handler.PatHandler
	PatLookup  middleware.PATGetter

	// P4-8 P2 小件：panic 聚合/路由对账/指标
	OpsHandler *handler.OpsHandler
	PanicSink  middleware.PanicSink

	// 批次 B/E13：网关反代注册表（前缀→上游 + AK/SK 出站签名）。
	// nil（未配置 gateway.upstreams）= 不挂载，网关化默认关闭。
	Gateway *gateway.Registry

	//（W0b wire 转正：前缀列表改经 Gateway.Prefixes 派生——见 GatewayPrefixList）

	// API 限流（07 §2）：nil = 不启用
	RateLimit *config.RateLimitConfig

	// TrustedProxies 信任的反代网段（B1-4）；空切片 = 不信任任何代理
	TrustedProxies []string
}

// GatewayPrefixList 反代上游前缀派生（原独立 GatewayPrefixes 字段与
// TrustedProxies 同为 []string 冲突 wire 注入，W0b 改由 Registry.Prefixes 派生）。
func (d Deps) GatewayPrefixList() []string {
	if d.Gateway == nil {
		return nil
	}
	return d.Gateway.Prefixes
}

// RateLimitOrDisabled 限流未配置时返回零值配置（中间件内直通）。
func (d Deps) RateLimitOrDisabled() config.RateLimitConfig {
	if d.RateLimit != nil {
		return *d.RateLimit
	}
	return config.RateLimitConfig{}
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
	r.Use(middleware.Recovery(deps.Logger, deps.PanicSink))
	r.Use(middleware.MetricsMiddleware()) // P4-8 指标埋点（全局链——AccessLogger 同位）
	r.Use(middleware.RequestID())
	r.Use(middleware.AccessLogger(deps.Logger))
	r.Use(middleware.CORS())
	r.Use(middleware.SecurityHeaders())
	// 1MB = zhuzhao 自有 API 上限；网关反代大文件导入不放宽（定性运维通道，
	// 2026-09-10 拍板）——浏览器端大文件导入需求出现时按 per-前缀放宽（16 号）
	r.Use(middleware.BodyLimit(1 << 20)) // 1MB

	// 健康检查
	// P4-8 指标端点（根级非 /api/v1——catalog 天然豁免；内网暴露面）
	r.GET("/metrics", middleware.RenderMetrics)
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
	// E-② 内网回调组：不走用户 JWT/Casbin——AK/SK 验签（utils aksk，验 taskrunner
	// 调用方签名；基线 §9 双防线之密码学层）+ 部署侧专用 network 拓扑。默认关闭。
	// onFail 用 response.AKSKFail()（2026-09-16 服务间验签统一批：信封+分档中文文案，
	// 替代 aksk 零依赖默认形态；验签通过后 caller/operator 归因键由中间件写入）。
	if deps.InternalJobs.Enabled {
		verifier := &aksk.Verifier{Keys: map[string][]byte{
			deps.InternalJobs.AK: []byte(deps.InternalJobs.SK),
		}}
		internalGroup := r.Group("/internal", aksk.GinMiddleware(verifier, response.AKSKFail()))
		internalGroup.POST("/jobs/callback", deps.JobsHandler.Callback)
		// P4-2 死信告警入口（E-⑥ 终败通知形态——taskrunner 投递，AK/SK 验签同组）
		internalGroup.POST("/notify/dead-letter", deps.NotificationHandler.DeadLetter)
	}

	// 存量协议兼容层（2026-10-14 预接线，翻案决策见 phase1/02-auth §存量迁移
	// 翻案追记）：Enabled=false（默认）不挂载；装配逻辑独立为 mountCompatGroup
	//（可测——开关/验签/归因的装配层断言见 compat_mount_test.go）。
	mountCompatGroup(r, deps.CompatAKSK)

	v1 := r.Group("/api/v1")
	{
		// 认证模块（无需鉴权）：匿名暴露面同样受限流保护（default 20rps/IP；
		// 可按路由模式配更严阈值——如 /api/v1/auth/login，与 LoginLocker 互补：
		// 前者限请求速率、后者限失败次数）
		auth := v1.Group("/auth")
		auth.Use(middleware.RateLimit(deps.RedisClient, deps.RateLimitOrDisabled()))
		{
			// P4-7 登录验证码（enabled=false 仅返回开关态——前端显隐插槽）
			auth.GET("/captcha", deps.AuthHandler.Captcha)
			auth.POST("/login", deps.AuthHandler.Login)
			auth.POST("/refresh", deps.AuthHandler.Refresh)
		}

		// 以下路由需要 JWT 认证 + Casbin 鉴权 + 审计日志
		authed := v1.Group("")
		authed.Use(
			middleware.JWT(deps.JWTManager, deps.RedisClient, deps.PatLookup),
			middleware.RateLimit(deps.RedisClient, deps.RateLimitOrDisabled()),
			middleware.AuditLog(deps.AuditService, deps.GatewayPrefixList()...),
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
					// P4-3 字典消费面（审计修复 2026-09-30：受众=全员业务表单选项——
					// SelfService 免 Casbin；管理面 CRUD 仍在 biz 组 L1）
					userSelf.GET("/dicts/:code/items", deps.DictHandler.EnabledItems)

					// P4-6 PAT 自服务（本人凭据——catalogExempt 登记）
					userSelf.GET("/pats", deps.PatHandler.List)
					userSelf.POST("/pats", deps.PatHandler.Create)
					userSelf.POST("/pats/delete", deps.PatHandler.Revoke)
					userSelf.GET("/menus", deps.UserHandler.GetMenus)
					userSelf.GET("/permissions", deps.UserHandler.GetPermissions)
					// 「我的组织」自服务数据源（P4-W3：非 admin 委托者可达——users/:id/orgs 挂 biz 组不可用）
					userSelf.GET("/orgs", deps.OrgHandler.GetMyOrgs)
				}
			}

			// phase5 S5 文件代理（A.4 #5）：JWT+限流独立链——不挂通用审计中间件
			//（A.4 #5 收窄：avatar 免审计、ticket 态 handler 级行随 B5-3、304 不落行）；
			// A.11 cookie 旁路凭证随 B5-2-2 演进本组认证（header 优先→cookie 回退）
			files := v1.Group("/files")
			files.Use(
				// A.11 双凭证：header 优先→file_session cookie 桥接（B5-2-2 形态 B）
				middleware.FileAuth(middleware.JWT(deps.JWTManager, deps.RedisClient, deps.PatLookup)),
				middleware.RateLimit(deps.RedisClient, deps.RateLimitOrDisabled()),
			)
			{
				files.GET("/*key", deps.StorageHandler.ProxyDownload)
			}

			// phase5 S5 对象存储自服务面（05 号 A.7——S5 评审 P0-1 修复 2026-10-20）：
			// 五端点=登录态+handler 层 L2 编排（ticket 态 purpose 走 TicketAuthorizer），
			// 挂 SelfService 标记组（PAT/orgDelegated 同款形态）——原挂 biz 组无标记，
			// catalogExempt 仅是启动对账豁免不产生运行时跳过，普通角色被 Casbin 全拦。
			// authed 级 JWT+限流+审计链保留（删附件审计经通用审计中间件落行）。
			storageSelf := authed.Group("")
			storageSelf.Use(middleware.SelfService(), middleware.CasbinAuth(deps.Enforcer, deps.RoleFetcher, deps.Logger))
			{
				storageSelf.POST("/storage/presign/upload", deps.StorageHandler.PresignUpload)
				// B5-2-3：头像回写（A.4 #7 契约行——B5-2 新增）
				storageSelf.POST("/storage/avatar/bind", deps.StorageHandler.BindAvatar)
				// A.4 #2 confirm（ticket 态落绑；L2 在 handler 编排层判定）
				storageSelf.POST("/tickets/attachments/confirm", deps.StorageHandler.Confirm)
				// B5-3 批3：附件列表（L2 read）/删除（L2 update）——与 confirm 同款
				storageSelf.GET("/tickets/:id/attachments", deps.StorageHandler.ListAttachments)
				storageSelf.POST("/tickets/attachments/delete", deps.StorageHandler.DeleteAttachment)
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
				// P4-W3「我的组织」名册（02 §2-W3 阻塞件）：L3 判定在 OrgService.ListMembersRoster
				orgDelegated.GET("/members/list", deps.OrgHandler.ListMembersRoster)
				orgDelegated.POST("/members/role", deps.OrgHandler.SetMemberRole)
				orgDelegated.POST("/members/scope", deps.OrgHandler.SetMemberScope)
				orgDelegated.GET("/roles/list", deps.OrgHandler.ListOrgRoles)
				orgDelegated.POST("/roles/bind", deps.OrgHandler.BindOrgRole)
				orgDelegated.POST("/roles/delete", deps.OrgHandler.UnbindOrgRole)
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
					users.GET("/:id/roles", deps.UserHandler.GetUserRoles)
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
					// W1（P4-W1 词表只读化）：三写接口删除——菜单行全走种子迁移
					//（GET 树/详情保留=角色分配数据源；应急隐藏正确姿势=角色解绑，runbook）
					menus.GET("", deps.MenuHandler.GetTree)

					menus.GET("/:id", deps.MenuHandler.Get)
				}

				// 审计日志
				audit := biz.Group("/audit")
				{
					// P4-8：panic 聚合查询+运行时路由对账（audit:read 面）
					audit.GET("/panics", deps.OpsHandler.ListPanics)
					audit.GET("/reconcile", deps.OpsHandler.Reconcile)
				}
				{
					audit.GET("/logs", deps.AuditHandler.ListLogs)
				}

				// 工单模块（Phase 2a）
				// 任务管理（E-④，task:submit/read/manage；代理 taskrunner API）
				tasks := biz.Group("/tasks")
				{
					tasks.POST("", deps.TaskrunnerHandler.Submit)
					tasks.GET("/:id", deps.TaskrunnerHandler.GetTask)
					tasks.POST("/cancel", deps.TaskrunnerHandler.Cancel)
					tasks.POST("/retry", deps.TaskrunnerHandler.Retry)
				}
				biz.GET("/runs", deps.TaskrunnerHandler.ListRuns)
				biz.GET("/dead-letters", deps.TaskrunnerHandler.DeadLetters)
				jobs := biz.Group("/jobs")
				{
					jobs.GET("", deps.TaskrunnerHandler.ListJobs)
					jobs.POST("", deps.TaskrunnerHandler.CreateJob)
					jobs.POST("/update", deps.TaskrunnerHandler.UpdateJob)
					jobs.POST("/trigger", deps.TaskrunnerHandler.Trigger)
				}

				// P4-3 字典（000034 词表：页面 dict:read+写按钮 dict:manage；消费端点挂页面行）
				dicts := biz.Group("/dicts")
				{
					dicts.GET("", deps.DictHandler.ListTypes)
					dicts.POST("", deps.DictHandler.CreateType)
					dicts.POST("/update", deps.DictHandler.UpdateType)
					dicts.POST("/delete", deps.DictHandler.DeleteType)
				}
				dictItems := biz.Group("/dict-items")
				{
					dictItems.GET("", deps.DictHandler.ListItems)
					dictItems.POST("", deps.DictHandler.CreateItem)
					dictItems.POST("/update", deps.DictHandler.UpdateItem)
					dictItems.POST("/delete", deps.DictHandler.DeleteItem)
				}

				// P4-2 通知配置管理面（菜单 system_notification——visible=false 过渡态，配置页后补）
				notifications := biz.Group("/notifications")
				{
					notifications.GET("", deps.NotificationHandler.List)
					notifications.POST("", deps.NotificationHandler.Create)
					notifications.POST("/update", deps.NotificationHandler.Update)
					notifications.POST("/delete", deps.NotificationHandler.Delete)
				}

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
					// phase5 S5 五端点已移 authed 级 SelfService 标记组（storageSelf，
					// 见 files 组后）——S5 评审 P0-1：biz 无标记时普通角色被 Casbin 全拦
				}

				// 工单元数据（类型/模板）
				ticketMeta := biz.Group("")
				{
					ticketMeta.GET("/ticket-types", deps.TicketHandler.ListTicketTypes)
					ticketMeta.GET("/ticket-types/:code/fields", deps.TicketHandler.ListTicketTypeFields)
					ticketMeta.GET("/ticket-templates", deps.TicketHandler.ListTicketTemplates)
					ticketMeta.GET("/ticket-templates/:code", deps.TicketHandler.GetTicketTemplate)
					// IW3/BK-18：类型/字段/模板管理（permission = ticket:type:manage，
					// L1 admin/superadmin matcher 通配；operator 经类型配置页 AssignMenus 放行）
					ticketMeta.POST("/ticket-types", deps.TicketHandler.CreateTicketType)
					// W1（BK-18 整改，000030）：PUT/DELETE 全仓清零——POST zhuzhao 风格
					//（code 入 body；standards §3-2 URL 不携带业务信息）
					ticketMeta.POST("/ticket-types/update", deps.TicketHandler.UpdateTicketType)
					ticketMeta.POST("/ticket-types/delete", deps.TicketHandler.DeleteTicketType)
					ticketMeta.POST("/ticket-types/fields/replace", deps.TicketHandler.ReplaceTicketTypeFields)
					ticketMeta.POST("/ticket-templates", deps.TicketHandler.CreateTicketTemplate)
					ticketMeta.POST("/ticket-templates/update", deps.TicketHandler.UpdateTicketTemplate)
					ticketMeta.POST("/ticket-templates/delete", deps.TicketHandler.DeleteTicketTemplate)
				}
			}

			// 网关反代（批次 B/E13）：上游路由对账属跨仓边界（BK-22 v1，见 catalog.go）
		}
	}

	// 网关反代（批次 B/E13）——根级挂载：前端路径 = /al/api/v1/...（与 menu_apis
	// 种子/activelist 上游路径口径一致；authed 内挂载产生 /api/v1/al/api/v1 双重前缀
	// 错位——BK-22 启动对账发现并修正）。全链 = JWT → 限流 → 审计(跳网关 body) →
	// CasbinAuth(menu_apis) → 身份断言 → AK/SK 出站签名透传（§25.1）。
	// 未配置上游不挂载（gateway 默认关闭）。
	if deps.Gateway != nil {
		deps.Gateway.Mount(r,
			middleware.JWT(deps.JWTManager, deps.RedisClient, deps.PatLookup),
			middleware.RateLimit(deps.RedisClient, deps.RateLimitOrDisabled()),
			middleware.AuditLog(deps.AuditService, deps.GatewayPrefixList()...),
			middleware.CasbinAuth(deps.Enforcer, deps.RoleFetcher, deps.Logger),
		)
	}

	return r
}

// mountCompatGroup 装配存量协议兼容层（/compat 路由组，2026-10-14 预接线，
// 翻案决策见 phase1/02-auth §存量迁移翻案追记）。Enabled=false（默认）不挂载
// 任何路由；启用则挂 compat.GinMiddleware 验签 + /compat/echo 判定靶（runbook
// §5.2 上线前向量比对）。装配独立成函数以便开关/验签/归因链路的装配层测试
// （compat_mount_test.go）；真实调用场景就位后业务端点同组并列（组内实质
// 单端点口径，standards §3.11 豁免）。
func mountCompatGroup(r *gin.Engine, cfg config.CompatAKSKConfig) {
	if !cfg.Enabled {
		return
	}
	keys := make(map[string][]byte, len(cfg.Keys))
	for ak, sk := range cfg.Keys {
		keys[ak] = []byte(sk)
	}
	compatGroup := r.Group("/compat", compat.GinMiddleware(&compat.Verifier{Keys: keys}, nil))
	compatGroup.POST("/echo", (&handler.CompatHandler{}).Echo)
}
