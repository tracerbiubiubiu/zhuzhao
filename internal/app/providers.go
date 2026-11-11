package app

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"fmt"
	"github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao-utils/logger"
	"github.com/tracerbiubiubiu/zhuzhao-utils/postgres"
	"github.com/tracerbiubiubiu/zhuzhao-utils/redis"
	"github.com/tracerbiubiubiu/zhuzhao/internal/casbin"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/gateway"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/audit"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/taskrunner"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/datafetcher"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/threatintel"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/webhook"
	"github.com/tracerbiubiubiu/zhuzhao/internal/router"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// 适配层：zhuzhao-utils 的基建包自带 Config 类型（与 internal/config 解耦），
// 这里把应用配置逐字段映射过去。

func provideLogger(cfg config.LogConfig) *slog.Logger {
	logger := logger.New(logger.Config{
		Level:      cfg.Level,
		Dir:        cfg.Dir,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
	})
	// §3-13 留痕中枢依赖默认 slog：全局 slog.Error（如 writeServiceError 未分类错误
	// 留痕）须与注入 logger 同 sink（app.log + stdout JSON）——不 SetDefault 则落
	// stderr 默认 TEXT，request_id 溯源失效（2026-10-12 复核批发现，496ba4a 后续）。
	slog.SetDefault(logger)
	return logger
}

func provideJWTManager(cfg config.JWTConfig) *jwt.Manager {
	return jwt.NewManager(jwt.Config{
		Secret:    cfg.Secret,
		AccessTTL: cfg.AccessTTL,
	})
}

func providePostgres(cfg config.DatabaseConfig) (*pgxpool.Pool, func(), error) {
	return postgres.New(postgres.Config{
		Host:         cfg.Host,
		Port:         cfg.Port,
		User:         cfg.User,
		Password:     cfg.Password,
		DBName:       cfg.DBName,
		MaxOpenConns: cfg.MaxOpenConns,
		// MaxIdleConns 接线已移除（2026-10-14 utils v0.4.2 口径修正）：pgxpool
		// 无空闲连接上限旋钮，空闲回收由 ConnMaxIdleTime 承接
		ConnMaxLifetime:  cfg.ConnMaxLifetime,
		ConnMaxIdleTime:  cfg.ConnMaxIdleTime,
		ConnectTimeout:   cfg.ConnectTimeout,
		SSLMode:          cfg.SSLMode,
		StatementTimeout: cfg.StatementTimeout,
		ApplicationName:  cfg.ApplicationName,
	})
}

func provideRedis(cfg config.RedisConfig) (*goredis.Client, func(), error) {
	return redis.New(redis.Config{
		Host:            cfg.Host,
		Port:            cfg.Port,
		DB:              cfg.DB,
		Password:        cfg.Password,
		PoolSize:        cfg.PoolSize,
		MinIdleConns:    cfg.MinIdleConns,
		PoolTimeout:     cfg.PoolTimeout,
		DialTimeout:     cfg.DialTimeout,
		ReadTimeout:     cfg.ReadTimeout,
		WriteTimeout:    cfg.WriteTimeout,
		MaxRetries:      cfg.MaxRetries,
		ConnMaxIdleTime: cfg.ConnMaxIdleTime,
	})
}

// 锁定参数取库默认值（与原 internal/pkg/redis 行为一致）。NewScripts 的
// variadic 形参 wire 无法注入，故固定为无参调用。
func provideRedisScripts(client *goredis.Client) *redis.Scripts {
	return redis.NewScripts(client)
}

// providePolicyEvalWriter 判定日志 L2 writer（B11①，03-audit-l2 §2；参数零值走默认）。
func providePolicyEvalWriter(cfg config.AuditConfig, rdb *goredis.Client,
	repo *repository.AuditLogRepo, logger *slog.Logger) *audit.PolicyEvalWriter {
	return audit.NewPolicyEvalWriter(audit.PolicyEvalConfig{
		BufferSize:    cfg.PolicyEval.BufferSize,
		BatchSize:     cfg.PolicyEval.BatchSize,
		FlushInterval: cfg.PolicyEval.FlushInterval,
		RedisKey:      cfg.PolicyEval.RedisKey,
	}, rdb, repo, logger)
}

// provideRegistry 建 Registry 并挂判定埋点（B11①：registry.Authorize 每次判定 →
// writer.Write 非阻塞入队；SetEvalHook 后注册的资源自动纳入）。
func provideRegistry(w *audit.PolicyEvalWriter) resource.Registry {
	reg := resource.NewRegistry()
	reg.SetEvalHook(func(ctx context.Context, e resource.EvalEntry) {
		w.Write(audit.PolicyEvalEntry{
			ActorID:      e.ActorID,
			ActorRoles:   e.ActorRoles,
			ResourceType: e.ResourceType,
			ResourceID:   e.ResourceID,
			Action:       e.Action,
			Result:       e.Result,
			Reason:       e.Reason,
			TraceID:      e.TraceID,
		})
	})
	return reg
}

// provideJobsRegistry 预置动作注册表 + 装配期注册内置动作（E-③：audit_archive / B11②）。
func provideJobsRegistry(repo *repository.AuditLogRepo, cfg config.AuditConfig, logger *slog.Logger) *jobs.Registry {
	reg := jobs.NewRegistry()
	reg.Register("audit_archive", service.NewAuditArchiveJob(repo,
		cfg.Archive.RetentionDays, cfg.Archive.BatchRows, cfg.Archive.OutDir,
		cfg.Archive.FileRetentionDays, logger),
		// S4 B4-1：动作元数据（GET /jobs/actions 出站——name/描述；kind/owner_service
		// 多执行端预留恒 handler/zhuzhao；params_schema 未声明=null 前端降级 JSON 模式）
		jobs.ActionMeta{
			Name:         "审计归档",
			Description:  "按保留期导出审计日志/判定日志到 JSONL 并删行（单批有界，可重入）",
			Kind:         "handler",
			OwnerService: "zhuzhao",
		})
	return reg
}

// provideTaskrunnerClient zhuzhao → taskrunner 出站 client（E-④；aksk 签名）。
// BaseURL 未配置时返回 nil client（服务层方法将返回不可达错误，应用其余功能不受影响）。
func provideTaskrunnerClient(cfg config.TaskrunnerConfig) *taskrunner.Client {
	if cfg.BaseURL == "" {
		return nil
	}
	return taskrunner.New(taskrunner.Config{
		BaseURL: cfg.BaseURL, AK: cfg.AK, SK: []byte(cfg.SK), Timeout: cfg.Timeout,
	})
}

// provideTaskrunnerHandler 任务管理端点（E-④；selfBaseURL 供缺省 callback 拼接）。
func provideTaskrunnerHandler(svc *service.TaskrunnerService, cfg config.TaskrunnerConfig, jobsReg *jobs.Registry) *handler.TaskrunnerHandler {
	return handler.NewTaskrunnerHandler(svc, cfg.SelfBaseURL, jobsReg)
}

// provideJobsCallbackService 回调编排服务（E-②；jobs_handler 薄化的分层修复）。
func provideJobsCallbackService(reg *jobs.Registry, repo *repository.JobSubmissionRepo, logger *slog.Logger) *service.JobsCallbackService {
	return service.NewJobsCallbackService(reg, repo, logger)
}

// provideGateway 网关反代注册表（W0b wire 转正）：upstreams 未配置返回
// (nil, nil)——router 侧 nil = 不挂载（网关化默认关闭）。
func provideGateway(cfg *config.Config) (*gateway.Registry, error) {
	ups := cfg.Gateway.Upstreams
	if len(ups) == 0 {
		return nil, nil
	}
	gwUps := make([]gateway.Upstream, len(ups))
	for i, u := range ups {
		gwUps[i] = gateway.Upstream{Prefix: u.Prefix, Target: u.Target, StripPrefix: u.StripPrefix, Disabled: u.Disabled}
	}
	return gateway.New(gwUps, cfg.Gateway.AK, cfg.Gateway.SK)
}

// provideRateLimitConfig W0b wire 转正：Deps.RateLimit 注入源。
func provideRateLimitConfig(cfg *config.Config) *config.RateLimitConfig {
	return &cfg.RateLimit
}

// provideApp Wire 唯一 provider（B 方案全量修 2026-10-24）——手码装配集中体。
// 历史注记：wire_gen.go 的注入体因手码段增多（ConfigStore/cron 装配/Set 注入/
// 闭包注册/条件初始化——wire 表达不了）已整体搬至此处；wire.go 的 wire.Build
// 只引用本 provider，再生永远可重现（"wire 转正"的真终态）。
func provideApp(cfg *config.Config) (*App, func(), error) {
	logConfig := cfg.Log
	logger := provideLogger(logConfig)
	databaseConfig := cfg.Database
	pool, cleanup, err := providePostgres(databaseConfig)
	if err != nil {
		return nil, nil, err
	}
	userRepo := repository.NewUserRepo(pool)
	jwtConfig := cfg.JWT
	manager := provideJWTManager(jwtConfig)
	redisConfig := cfg.Redis
	client, cleanup2, err := provideRedis(redisConfig)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	scripts := provideRedisScripts(client)
	auditLogRepo := repository.NewAuditLogRepo(pool)
	auditService := service.NewAuditService(auditLogRepo, userRepo)
	captchaSvc := service.NewCaptchaService(client, cfg.CaptchaEnabled, parseDuration(cfg.CaptchaTTL))
	authService := service.NewAuthService(userRepo, manager, client, scripts, auditService, jwtConfig, captchaSvc)
	authHandler := handler.NewAuthHandler(authService, cfg.Storage.FileCookieSecure)
	roleRepo := repository.NewRoleRepo(pool)
	orgRepo := repository.NewOrgRepo(pool)
	orgDelegationService := service.NewOrgDelegationService(pool)
	menuRepo := repository.NewMenuRepo(pool)
	casbinConfig := cfg.Casbin
	syncedEnforcer, cleanup3, err := casbin.New(casbinConfig, pool)
	if err != nil {
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	rbacService := service.NewRBACService(roleRepo, userRepo, menuRepo, syncedEnforcer)
	orgService := service.NewOrgService(orgRepo, userRepo, orgDelegationService, rbacService)
	userService := service.NewUserService(pool, userRepo, roleRepo, orgService, client, manager)
	menuService := service.NewMenuService(menuRepo, userRepo, roleRepo)
	userHandler := handler.NewUserHandler(userService, menuService)
	roleHandler := handler.NewRoleHandler(rbacService)
	orgHandler := handler.NewOrgHandler(orgService)
	menuHandler := handler.NewMenuHandler(menuService)
	auditHandler := handler.NewAuditHandler(auditService)
	notificationRepo := repository.NewNotificationRepo(pool)
	notificationWebhook := webhook.New()
	notificationService := service.NewNotificationService(notificationRepo, notificationWebhook, logger)
	notificationHandler := handler.NewNotificationHandler(notificationService)
	dictRepo := repository.NewDictRepo(pool)
	dictService := service.NewDictService(dictRepo, logger)
	dictHandler := handler.NewDictHandler(dictService)
	patRepo := repository.NewPatRepo(pool)
	patService := service.NewPatService(patRepo, logger)
	patHandler := handler.NewPatHandler(patService)

	ticketRepo := repository.NewTicketRepo(pool)
	auditConfig := cfg.Audit
	policyEvalWriter := providePolicyEvalWriter(auditConfig, client, auditLogRepo, logger)
	registry := provideRegistry(policyEvalWriter)
	// phase5 S5 B5-1（05 号 A.1 装配链）：禁用态 client=nil，服务层显式拒绝；
	// GC cron 装配在 NewApp（随 storage.enabled 门控——与 auditCron 同款）
	storageConfig := cfg.Storage
	// F-D3（2026-10-21）：三服务 + NewApp GC 共享同一原子快照存储（KV 运行时
	// 覆盖与请求侧读并发安全；修复前值拷贝指针分裂——orphan_gc_days 到不了 GC）。
	storageStore := pkgstorage.NewConfigStore(&storageConfig)
	var storageClient pkgstorage.Client
	if storageConfig.Enabled {
		mc, err := pkgstorage.NewMinioClient(&storageConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("storage client init: %w", err)
		}
		storageClient = mc
	}
	eventRegistry := provideEventRegistry(logger)
	storageRepo := repository.NewStorageRepo(pool)
	storageService := storageservice.NewService(storageRepo, storageClient, storageStore, logger)
	// G-1 审计修复：fetcherReg 第 9 参恒 nil（R1 同型第五例）——auto_fill
	// 生产静默不生效、formula 引用 fetch 字段提交期 400。空 Registry=无注册源
	// 时 ApplyFetches 跳过（语义不变），phase6 R3 首源接入时注册 fetcher 即激活。
	ticketService := ticket.NewTicketService(pool, ticketRepo, orgRepo, registry, rbacService, orgDelegationService, storageRepo, storageStore, datafetcher.NewRegistry())
	// S3a B3-2：分派规则服务（OnAfterCreate 在 ticketService.Create 内调——
	// 通过 DispatchService 持有 svc 反向引用；装配期注入）
	dispatchSvc := ticket.NewDispatchService(repository.NewAssignmentRuleRepo(pool), ticketService, logger)
	// G2-3 审计修复：SetDispatcher 从未被调用（装配遗漏——dispatcher 恒 nil
	// 致 OnAfterCreate 静默跳过，自动分派生产不可达；与 R1 同型）
	ticketService.SetDispatcher(dispatchSvc)
	// G2-4 审计修复：事件出口注入（修复前全仓无调用——emitEvent NOP 致生产
	// 全事件静默：inbox 零落库/通知链全哑/post_actions notify 不可达）
	ticketService.SetEventRegistry(eventRegistry)
	// S3a B3-3：动作库装配（修复前漏注入——actionReg 恒 nil 致 post_actions
	// 整链路生产不执行且静默跳过；审计 A1）
	actionReg := ticket.NewActionRegistry()
	actionReg.Register(&ticket.NotifyAction{Svc: ticketService}) // 经 Service 事件出口（eventRegistry 在 NewApp 晚于装配——Reg 注入不可达，改持 Svc）
	actionReg.Register(&ticket.CCAction{Svc: ticketService})
	actionReg.Register(&ticket.ReassignAction{Svc: ticketService})
	// G-5：动作失败事件出口接线（修复前全仓零调用——r.emitter 恒 nil，
	// ops.action_failed 永不派发：动作失败仅 slog.Warn，S6 规则面/inbox 均不可达。
	// fire-and-forget 同 emitEvent 口径：Background ctx，Dispatch 内部自管生命周期）
	actionReg.SetEmitter(func(ev domainevent.DomainEvent) {
		eventRegistry.Dispatch(context.Background(), ev)
	})
	ticketService.SetActionRegistry(actionReg)
	ticketHandler := handler.NewTicketHandler(ticketService, auditService, dispatchSvc)
	storageHandler := handler.NewStorageHandler(storageService, ticketService, auditService)
	settingsRepo := repository.NewSettingsRepo(pool)
	settingsService := service.NewSettingsService(settingsRepo, storageStore, logger)
	settingsHandler := handler.NewSettingsHandler(settingsService)
	jobsRegistry := provideJobsRegistry(auditLogRepo, auditConfig, logger)
	jobSubmissionRepo := repository.NewJobSubmissionRepo(pool)
	jobsCallbackService := provideJobsCallbackService(jobsRegistry, jobSubmissionRepo, logger)
	jobsHandler := handler.NewJobsHandler(jobsCallbackService)
	internalJobsConfig := cfg.InternalJobs
	compatAKSKConfig := cfg.CompatAKSK
	taskrunnerConfig := cfg.Taskrunner
	taskrunnerClient := provideTaskrunnerClient(taskrunnerConfig)
	taskrunnerService := service.NewTaskrunnerService(taskrunnerClient, jobSubmissionRepo)
	// spawn_task 动作（durable_submit——taskrunner 持久化凭证；适配器桥接。
	// Registry 指针引用——SetActionRegistry 已注入，此处 Register 同一实例生效）
	if err := actionReg.Register(ticket.NewSpawnTaskAction(&taskSubmitterAdapter{svc: taskrunnerService})); err != nil {
		panic(err) // 装配期 fail-fast
	}
	taskrunnerHandler := provideTaskrunnerHandler(taskrunnerService, taskrunnerConfig, jobsRegistry)
	gatewayRegistry, err := provideGateway(cfg)
	if err != nil {
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	rateLimitConfig := provideRateLimitConfig(cfg)
	v := provideTrustedProxies(cfg)
	// S9 B9-1 情报缓存代理（stub 默认 true——契约未到桩先行；非 stub 且
	// endpoint 空 → fail-fast 拒启防虚标）
	intelProvider, err := threatintel.NewProvider(cfg.Intel.Upstream, logger)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	intelTTL := map[string]time.Duration{}
	for k, v := range cfg.Intel.TTL {
		if d, perr := time.ParseDuration(v); perr == nil {
			intelTTL[k] = d
		}
	}
	intelSvc := threatintel.NewService(pool, intelProvider, intelTTL, logger)
	// S6 二期 B6-7 站内消息中心（inbox=事件消费者——订阅 ticket/ops 双域；
	// 未读数注入 badges sections；90 天保留清理骑 CronScheduler）
	inboxSvc := ticket.NewInboxService(pool, 90, logger)
	badgeSvc := ticket.NewBadgeService(pool)
	badgeSvc.RegisterUnread(func(ctx context.Context, uid int64) (int64, error) {
		return inboxSvc.UnreadCount(ctx, uid)
	})

	cronRegistry := service.NewCronRegistry()
	deps := router.Deps{
		AuthHandler:         authHandler,
		UserHandler:         userHandler,
		RoleHandler:         roleHandler,
		OrgHandler:          orgHandler,
		MenuHandler:         menuHandler,
		AuditHandler:        auditHandler,
		NotificationHandler: notificationHandler,
		DictHandler:         dictHandler,
		MessageTemplateHandler: handler.NewMessageTemplateHandler(
			service.NewMessageTemplateService(repository.NewMessageTemplateRepo(pool))),
		MessageAdminHandler:     handler.NewMessageAdminHandler(service.NewMessageAdminService(pool)),
		NotificationPrefHandler: handler.NewNotificationPrefHandler(service.NewNotificationPrefService(pool)),
		PatHandler:              patHandler,
		PatLookup:               patRepo,
		TicketHandler:           ticketHandler,
		BadgeHandler:            handler.NewBadgeHandler(badgeSvc),
		// G2-2 审计修复：InboxHandler 漏装配（deps 缺字段——nil 进路由，调用即
		// nil pointer panic，inbox 三端点全 500；notifications 零落库同根）
		InboxHandler:      handler.NewInboxHandler(inboxSvc),
		SystemHandler:     handler.NewSystemHandler(cronRegistry),
		IntelHandler:      handler.NewIntelHandler(intelSvc),
		StorageHandler:    storageHandler,
		SettingsHandler:   settingsHandler,
		JWTManager:        manager,
		Enforcer:          syncedEnforcer,
		RedisClient:       client,
		DBPool:            pool,
		Logger:            logger,
		RoleFetcher:       rbacService,
		AuditService:      auditService,
		Registry:          registry,
		JobsHandler:       jobsHandler,
		InternalJobs:      internalJobsConfig,
		CompatAKSK:        compatAKSKConfig,
		TaskrunnerHandler: taskrunnerHandler,
		Gateway:           gatewayRegistry,
		RateLimit:         rateLimitConfig,
		TrustedProxies:    v,
	}
	// P4-8：panic 聚合+运行时对账。engineRef 前置声明——闭包运行时取（engine 由
	// router.New(deps) 在下方构造后回填）
	var engineRef *gin.Engine
	panicRepo := repository.NewPanicRepo(pool)
	opsHandler := handler.NewOpsHandler(
		func(ctx context.Context, page, pageSize int) (any, int64, error) {
			return panicRepo.List(ctx, page, pageSize) // 适配桥（app 层可引 repo 具体类型）
		}, func(ctx context.Context) []string {
			bound, err := router.LoadBoundAPIs(ctx, pool)
			if err != nil {
				return []string{"[error] 绑定集查询失败: " + err.Error()}
			}
			var prefixes []string
			if gatewayRegistry != nil {
				prefixes = gatewayRegistry.Prefixes
			}
			gaps := router.AuditRouteCatalog(engineRef.Routes(), bound, prefixes)
			out := make([]string, 0, len(gaps))
			for _, g := range gaps {
				out = append(out, g.String())
			}
			return out
		})
	deps.OpsHandler = opsHandler
	deps.PanicSink = panicRepo

	engine := router.New(deps)
	engineRef = engine
	app, err := NewApp(cfg, logger, engine, policyEvalWriter, pool, gatewayRegistry, storageStore, cronRegistry, inboxSvc, eventRegistry)
	if err != nil {
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	return app, func() {
		cleanup3()
		cleanup2()
		cleanup()
	}, nil
}

// provideTrustedProxies 信任代理网段（空 = 不信任任何代理，安全默认）
func provideTrustedProxies(cfg *config.Config) []string {
	return cfg.Server.TrustedProxies
}

// parseDuration captcha TTL 配置解析（容错——空串返零值）
func parseDuration(s string) (d time.Duration) {
	if s == "" {
		return 0
	}
	d, _ = time.ParseDuration(s)
	return d
}
