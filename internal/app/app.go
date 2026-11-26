package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/gateway"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/audit"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/channelsender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/singleton"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/webhook"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	routerPkg "github.com/tracerbiubiubiu/zhuzhao/internal/router"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// App 应用实例
type App struct {
	cfg              *config.Config
	logger           *slog.Logger
	router           *gin.Engine
	server           *http.Server
	backgroundCancel context.CancelFunc
	requestCancel    context.CancelFunc
	background       sync.WaitGroup
	policyEval       *audit.PolicyEvalWriter // B11① 判定日志 L2（HTTP排空后停止消费）
	auditCron        *service.CronScheduler  // P4 cron 收归：nil=未启用（配置开关）
	storageGC        *service.CronScheduler  // phase5 S5 孤儿 GC：nil=storage 未启用
	timeoutScan      *service.CronScheduler  // S10 B10-4 节点超时：nil=未启用（配置开关）
	iamAuditDelivery *service.CronScheduler
	riskReminder     *service.CronScheduler
	inboxCleanup     *service.CronScheduler // S6 B6-7 消息保留清理：恒启用（nil=装配缺省）
	intelCleanup     *service.CronScheduler // G-11 S9 情报缓存物理清理：恒启用（nil=intel 未启用）
	// 以下两件 2026-11-01 补启动（观测批注册了 scheduler 却漏 go Run——注册≠启动，
	// cron-status 可见但 11 个 tick 周期零执行；StartedAt 字段由此而设）
	storageUsageSample *service.CronScheduler    // C-6 存储用量采样：恒启用
	notifyVolumeCheck  *service.CronScheduler    // C-16 通知量级检查：恒启用
	events             *domainevent.HookRegistry // 领域事件总线：Shutdown 排空在途订阅者（nil=跳过）
}

// provideEventRegistry 领域事件注册表 provider（B6-1 共享基础设施——
// G2-4 审计修复：原 NewApp 内构造晚于 wire_gen 装配段，ticketService 事件
// 出口/inbox 订阅/NotifyAction 三处消费无法同实例注入——emitEvent NOP 致
// 生产全事件静默（集成测试自建 registry 故门禁无感）。provider 化后
// wire_gen 装配段可先取实例注入再传 NewApp）。
func provideEventRegistry(logger *slog.Logger) *domainevent.HookRegistry {
	return domainevent.NewHookRegistry(logger)
}

// NewApp 创建应用实例
// NewApp 创建应用实例。构造期执行 BK-22 fail-fast 对账（路由↔menu_apis
// 双向；缺口 = 拒绝启动）——W0b wire 转正：原在 wire_gen 手码装配段的
// 对账逻辑迁入此处，使 wire_gen 可安全再生。
func NewApp(cfg *config.Config, logger *slog.Logger, router *gin.Engine, policyEval *audit.PolicyEvalWriter,
	pool *pgxpool.Pool, gw *gateway.Registry, storageCfg *pkgstorage.ConfigStore,
	cronReg *service.CronRegistry, inboxSvc *ticket.InboxService,
	eventReg *domainevent.HookRegistry) (*App, error) {
	var prefixes []string
	if gw != nil {
		prefixes = gw.Prefixes
	}
	// B6-1：领域事件注册表（S6/phase6/S10 共享基础设施——一次变更多订阅者）
	eventRegistry := eventReg
	// 06 号 §5.4 死信集成：ChannelSender.Send 终败→进程内直调死信告警
	notifRepo := repository.NewNotificationRepo(pool)
	notifWebhook := webhook.New()
	notifSvc := service.NewNotificationService(notifRepo, notifWebhook, logger)

	// B6-5 触发挂接：工单事件→模板→渲染→ChannelSender 发送
	notifyHook := ticket.NewTicketNotifyHook(
		repository.NewMessageTemplateRepo(pool),
		repository.NewMessageRuleRepo(pool),
		repository.NewMessageLogRepo(pool),
		repository.NewNotificationOptoutRepo(pool),
		// UIP 两态桩（06 号 §3/A.3）：StubMode=非 release 推导；Endpoint 经
		// notification.uip.endpoint config 注入（微批 A——生产态空→92004 fail-fast）
		channelsender.NewUIPStubSender(channelsender.UIPStubConfig{
			StubMode: cfg.Server.Mode != "release",
			Endpoint: cfg.Notification.UIP.Endpoint,
		}, logger),
		logger,
		notifSvc, // 06 号 §5.4 死信集成（实现 NotifyDeadLetterSender）
	)
	eventRegistry.Subscribe(domainevent.BizTicket, "ticket.notify", notifyHook)
	// S6 二期 B6-7：inbox=事件消费者（站内通知落库）——SetEventRegistry 惯例注入
	// S10 B10-4：ops 域（cron 失败告警）双订阅——同一编排链（MessageDispatcher）
	eventRegistry.Subscribe(domainevent.BizOps, "ops.notify", notifyHook)

	// B1-6 批2：表达式函数注册表装配（lookup_dict 需 db 闭包；纯函数 now/upper
	// 已在 exprcalc 包内默认注册；risk_score/sla_deadline 待 phase6 源后加函数）
	ticket.RegisterExprDictLookup(pool)

	bound, err := routerPkg.LoadBoundAPIs(context.Background(), pool)
	if err != nil {
		return nil, err
	}
	if gaps := routerPkg.AuditRouteCatalog(router.Routes(), bound, prefixes); len(gaps) > 0 {
		return nil, fmt.Errorf("BK-22 路由↔menu_apis 对账失败（%d 项缺口，拒启 fail-fast）：%s",
			len(gaps), routerPkg.FormatGaps(gaps))
	}
	// B 案守卫（E2E 补齐批四程）：页面/目录行不得承载写端点——写端点挂页面行会随 viewer/operator
	// 预设（只绑页面）连带放行写权（000070/000073、000034/000059 三案实证）。例外登记 pageWriteAllow。
	pageWrites, err := routerPkg.AuditPageWriteBindings(context.Background(), pool)
	if err != nil {
		return nil, err
	}
	if len(pageWrites) > 0 {
		return nil, fmt.Errorf("B 案对账失败（页面行承载写端点 %d 项，拒启 fail-fast——迁按钮行或带设计出处登记 pageWriteAllow）：%s",
			len(pageWrites), routerPkg.FormatPageWriteBindings(pageWrites))
	}
	// P4 cron 收归：配置开关启用则装配进程内调度器（audit_archive 单批可重入——
	// ReentrantJob 契约见 cron_scheduler.go）
	var iamAuditDelivery *service.CronScheduler
	if cfg.Audit.IAMCollectorURL != "" {
		collector, err := audit.NewHTTPCollector(cfg.Audit.IAMCollectorURL)
		if err != nil {
			return nil, err
		}
		worker := service.NewIAMAuditDelivery(pool, collector)
		iamAuditDelivery = service.NewCronScheduler(pool, "iam_audit_delivery", worker, time.Second*5, logger)
		cronReg.Register("iam_audit_delivery", iamAuditDelivery)
	}
	var auditCron *service.CronScheduler
	if cfg.Audit.Archive.CronEnabled {
		tick, _ := time.ParseDuration(cfg.Audit.Archive.CronTick)
		job, err := service.NewFixedAuditArchiveJob(repository.NewAuditLogRepo(pool), cfg.Audit.Archive, logger)
		if err != nil {
			return nil, fmt.Errorf("fixed audit maintenance admission: %w", err)
		}
		auditCron = service.NewCronScheduler(pool, "audit_archive", job, tick, logger)
		logger.Info("audit_archive cron enabled (in-process ticker; disable taskrunner-side job to avoid double-run)")
	}
	// phase5 S5 B5-1：孤儿对象 GC（骑既有 CronScheduler——05 号 A.3 修正②）；
	// tick=1h 起步（低频扫描，config 化触发驱动）。GCJob 结构性满足 ReentrantJob，
	// 零 import service 根包（域隔离守护）——装配在此处完成；client 构造失败
	// 只禁 GC 不拒启（presign 主链路由 wire_gen 侧 fail-fast——失败时请求面已挡）。
	var storageGC *service.CronScheduler
	if cfg.Storage.Enabled {
		if gcClient, cerr := pkgstorage.NewMinioClient(&cfg.Storage); cerr == nil {
			// F-D3：GC 与 wire 三服务共享同一 ConfigStore——修复前 &cfg.Storage 与
			// wire 值拷贝 &storageConfig 分裂，KV 改 orphan_gc_days 到不了 GC。
			gc := storageservice.NewGCJob(repository.NewStorageRepo(pool), gcClient, storageCfg, logger, cfg.Storage.AvatarObserve())
			storageGC = service.NewCronScheduler(pool, "storage_gc", gc, time.Hour, logger)
			logger.Info("storage_gc cron enabled (orphan pending objects, 1h tick)")
		} else {
			logger.Error("storage_gc disabled: client init failed", "err", cerr)
		}
	}
	// S10 B10-4 节点超时扫描（10 号 A.5——骑 CronScheduler，D18 维护型口径；
	// observe 默认 true=发布安全默认：存量回填近似值确认无异常后经 config 关闭）
	var timeoutScan *service.CronScheduler
	var timeoutObserve bool
	if cfg.Tickets.TimeoutScanEnabled {
		tick, _ := time.ParseDuration(cfg.Tickets.TimeoutScanTick)
		if tick <= 0 {
			tick = 5 * time.Minute
		}
		timeoutObserve = cfg.Tickets.TimeoutObserve
		scanner := ticket.NewTimeoutScanner(pool, eventRegistry, timeoutObserve, logger)
		timeoutScan = service.NewCronScheduler(pool, "timeout_scan", scanner, tick, logger)
		if timeoutObserve {
			logger.Warn("timeout_scan enabled in OBSERVE mode (log-only, no dispatch — set tickets.timeout_observe=false after first clean cycle)", slog.Duration("tick", tick))
		} else {
			logger.Info("timeout_scan cron enabled", slog.Duration("tick", tick))
		}
	}
	// B10-4 观测面：cron 失败 → DomainEvent{ops, ops.cron_failed} → S6 规则面告警
	// （10 号 A.5 静默失败不可接受——种子规则收件人 empty=暂不发送，管理员配置后生效）
	cronFailEmit := func(job string, err error) {
		eventRegistry.Dispatch(context.Background(), domainevent.DomainEvent{
			BizType: domainevent.BizOps, Action: domainevent.ActionOpsCronFailed,
			BizID: job, Payload: domainevent.Payload{
				"job": job, "error": fmt.Sprintf("%v", err), "last_success": "see /system/cron-status",
			},
			Timestamp: time.Now(),
		})
	}
	if auditCron != nil {
		auditCron.SetOnFailure(cronFailEmit)
	}
	if storageGC != nil {
		storageGC.SetOnFailure(cronFailEmit)
	}
	if timeoutScan != nil {
		timeoutScan.SetOnFailure(cronFailEmit)
	}
	// cron-status 数据面：注册表共享（wire_gen 建 handler 时已注入同实例——
	// 装配环经注册表壳解开；nil 调度器跳过）
	// S6 B6-7：inbox=事件消费者（站内通知落库）——SetEventRegistry 惯例注入
	var inboxCleanup *service.CronScheduler
	if inboxSvc != nil {
		inboxSvc.SetEventRegistry(eventRegistry)
		// 消息保留清理 cron（恒启用——90 天封顶；cronFailEmit 同挂）
		inboxCleanup = service.NewCronScheduler(pool, "inbox_cleanup",
			cronJobFunc(func(ctx context.Context) error { return inboxSvc.CleanupExpired(ctx) }),
			24*time.Hour, logger)
		inboxCleanup.SetOnFailure(cronFailEmit)
		cronReg.Register("inbox_cleanup", inboxCleanup)
	}
	// G-11 审计修复：S9 情报缓存过期物理清理（生产前必挂——过期行只懒查
	// 不物理删=表只增不减；24h 低频+advisory lock 单实例+cron-status 可见）
	intelCleanup := service.NewCronScheduler(pool, "intel_cleanup",
		cronJobFunc(func(ctx context.Context) error {
			_, err := pool.Exec(ctx, `DELETE FROM intel_cache WHERE expires_at < NOW() - interval '7 days'`)
			return err
		}), 24*time.Hour, logger)
	intelCleanup.SetOnFailure(cronFailEmit)
	cronReg.Register("intel_cleanup", intelCleanup)
	// C-6（2026-10-27 观测批——A-2 拍板 a 案）：storage 用量 SQL 采样 gauge + 容量
	// 阈值检查（capacity_bytes=0 只暴露不判阈；80/90 双档 WARN+跃迁 emit；采样面与
	// 阈值面同轮同源）。file_objects 按 purpose 聚合——avatar 段泄漏当日显形。
	storageSampler := service.NewStorageUsageSampler(pool, eventRegistry, cfg.Storage.CapacityBytes, logger)
	storageUsageSample := service.NewCronScheduler(pool, "storage_usage_sample",
		cronJobFunc(storageSampler.Sample), time.Hour, logger)
	storageUsageSample.SetOnFailure(cronFailEmit)
	cronReg.Register("storage_usage_sample", storageUsageSample)
	// C-16（2026-10-27 观测批）：小时发送量差值检查——超阈值 WARN（volume_warn_per_hour
	// =0 关；日志面兜底不走 S6 通知闭环，通知系统自身故障时其告警通道同时失效）。
	// 计数器进程内单调，差值基线随进程重启重置（重启小时窗口静默——单实例低频可接受）。
	lastNotifyTotal := middleware.GetCounter("zhuzhao_notify_sent_total", "ok") +
		middleware.GetCounter("zhuzhao_notify_sent_total", "dead")
	notifyVolumeCheck := service.NewCronScheduler(pool, "notify_volume_check",
		cronJobFunc(func(ctx context.Context) error {
			total := middleware.GetCounter("zhuzhao_notify_sent_total", "ok") +
				middleware.GetCounter("zhuzhao_notify_sent_total", "dead")
			delta := total - lastNotifyTotal
			lastNotifyTotal = total
			if lim := cfg.Notification.VolumeWarnPerHour; lim > 0 && delta > float64(lim) {
				logger.Warn("notify: hourly send volume above threshold",
					"sent_last_hour", int64(delta), "threshold", lim,
					"hint", "检查 message_rules 收件面是否误配全员")
			}
			return nil
		}), time.Hour, logger)
	notifyVolumeCheck.SetOnFailure(cronFailEmit)
	cronReg.Register("notify_volume_check", notifyVolumeCheck)
	// G2-1 审计修复：typed-nil 陷阱——nil *CronScheduler 装进非 nil interface，
	// Register 的 p==nil 挡不住 → Status() nil 解引用 500（真机走查实证：
	// dev 配置 audit cron 关闭时 /system/cron-status 全路由 500）。
	// 与 timeout_scan 分支同款显式守卫。
	if auditCron != nil {
		cronReg.Register("audit_archive", auditCron)
	}
	if storageGC != nil {
		cronReg.Register("storage_gc", storageGC)
	}
	if timeoutScan != nil {
		cronReg.Register("timeout_scan", &timeoutScanProvider{sch: timeoutScan, observe: timeoutObserve})
	}
	return &App{
		cfg:                cfg,
		logger:             logger,
		router:             router,
		policyEval:         policyEval,
		auditCron:          auditCron,
		iamAuditDelivery:   iamAuditDelivery,
		storageGC:          storageGC,
		timeoutScan:        timeoutScan,
		inboxCleanup:       inboxCleanup,
		intelCleanup:       intelCleanup,
		storageUsageSample: storageUsageSample,
		notifyVolumeCheck:  notifyVolumeCheck,
		events:             eventRegistry,
	}, nil
}

// Run 启动应用
func (a *App) Run() error {
	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(signalCtx)
	defer cancel()
	a.backgroundCancel = cancel
	acquireCtx, acquireCancel := context.WithTimeout(ctx, 5*time.Second)
	guard, err := singleton.Acquire(acquireCtx, a.cfg.Database.DSN())
	acquireCancel()
	if err != nil {
		return err
	}
	defer guard.Close()
	ownerCtx := guard.OwnerContext()
	// Signals stop producers immediately. An admitted HTTP transaction may
	// drain normally, but detected ownership loss cancels its context directly.
	requestCtx, cancelRequests := context.WithCancel(ownerCtx)
	defer cancelRequests()
	a.requestCancel = cancelRequests
	requestCtx = singleton.WithOwner(requestCtx, ownerCtx)
	if a.events != nil {
		a.events.BindOwner(ownerCtx)
	}

	a.server = &http.Server{
		Addr: fmt.Sprintf(":%d", a.cfg.Server.Port), Handler: guard.Handler(a.router),
		BaseContext:       func(net.Listener) context.Context { return requestCtx },
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second,
	}
	// Keep fencing active during Shutdown as well: SIGTERM may start a normal
	// drain before the singleton session is subsequently lost.
	watchCtx, stopWatch := context.WithCancel(context.Background())
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		select {
		case <-guard.Lost():
			cancel()
			cancelRequests()
			_ = a.server.Close()
		case <-watchCtx.Done():
		}
	}()
	defer func() { stopWatch(); <-watchDone }()
	serverErr := make(chan error, 1)
	go func() {
		a.logger.Info("server starting", slog.Int("port", a.cfg.Server.Port), slog.String("mode", a.cfg.Server.Mode))
		err := a.server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverErr <- err
	}()
	if a.policyEval != nil {
		a.policyEval.Start(requestCtx, ownerCtx)
	}
	for _, scheduler := range []*service.CronScheduler{a.auditCron, a.iamAuditDelivery, a.storageGC, a.timeoutScan, a.riskReminder, a.inboxCleanup, a.intelCleanup, a.storageUsageSample, a.notifyVolumeCheck} {
		if scheduler == nil {
			continue
		}
		a.background.Add(1)
		go func(s *service.CronScheduler) { defer a.background.Done(); s.Run(ctx) }(scheduler)
	}
	var runErr error
	select {
	case runErr = <-serverErr:
		if runErr != nil {
			a.logger.Error("server failed", slog.Any("error", runErr))
		}
	case <-guard.Lost():
		runErr = singleton.ErrLost
		a.logger.Error("singleton ownership lost; stopping HTTP and background work")
		cancel()
		cancelRequests()
		// A lost deployment lease cannot continue draining accepted connections as
		// if it still owned the database. Cancellation also reaches request contexts.
		_ = a.server.Close()
		drainCtx, drainCancel := context.WithTimeout(context.Background(), 5*time.Second)
		runErr = errors.Join(runErr, guard.DrainRequests(drainCtx))
		drainCancel()
	case <-ctx.Done():
		a.logger.Info("server shutting down...")
	}
	cancel()
	shutdownErr := a.Shutdown()
	select {
	case <-guard.Lost():
		if !errors.Is(runErr, singleton.ErrLost) {
			runErr = errors.Join(runErr, singleton.ErrLost)
		}
	default:
	}
	// A timed-out drain or fencing may leave context-aware handlers completing
	// cancellation. Join them before releasing the singleton session or PG pool.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer drainCancel()
	return errors.Join(runErr, shutdownErr, guard.DrainRequests(drainCtx))
}

// Shutdown 优雅关闭
func (a *App) Shutdown() error {
	if a.backgroundCancel != nil {
		a.backgroundCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. 停止接受新请求（即使排空失败也继续后续收尾——提前 return 会跳过
	// policyEval.Stop，drain 打在已关闭 Redis 上静默丢行）
	serverErr := a.server.Shutdown(ctx)
	if a.requestCancel != nil {
		a.requestCancel()
	}
	if serverErr != nil {
		a.logger.Error("server forced to shutdown", slog.Any("error", serverErr))
		_ = a.server.Close()
	}

	// Join cron loops before releasing the singleton session or closing PG.
	backgroundDone := make(chan struct{})
	go func() { a.background.Wait(); close(backgroundDone) }()
	select {
	case <-backgroundDone:
	case <-ctx.Done():
		a.logger.Error("background shutdown timeout")
		serverErr = errors.Join(serverErr, ctx.Err())
	}

	// 2. 等判定日志管道退出。正常退出先排空并刷盘。失锁取消写入，
	// 已持久化的行留 Redis 供接管实例续消。
	if a.policyEval != nil {
		if !a.policyEval.Stop(5 * time.Second) {
			a.logger.Warn("policy_eval writer shutdown timeout; only completed Redis pushes are durable")
		}
	}

	// 3. 领域事件总线排空：等在途订阅者（通知发送/inbox 落行）跑完再放行 cleanup
	// 关 PG。正常排空允许订阅者完成，失锁时所有者上下文已取消，
	// 此处只等在途订阅者退出。总线采用 at-most-once，取消或超时可丢事件。
	if a.events != nil {
		drainStart := time.Now()
		drainCtx, drainCancel := context.WithTimeout(context.Background(), 8*time.Second)
		if err := a.events.Drain(drainCtx); err != nil {
			a.logger.Warn("domain event drain timeout, in-flight subscribers abandoned",
				slog.Any("error", err))
		} else {
			a.logger.Info("domain event bus drained", slog.Duration("took", time.Since(drainStart)))
		}
		drainCancel()
	}

	// B4-6：审计为同步写入（08-audit.md Phase 1 决策：无队列，优雅关闭无需 drain）；
	// Casbin/Redis/PG 连接关闭由 main.go defer cleanup() 依 casbin→redis→pg 逆序执行

	a.logger.Info("server stopped")
	return serverErr
}

// taskSubmitterAdapter TaskrunnerService → ticket.TaskSubmitter 端口适配
// （域隔离禁 service 互 import——app 装配层做类型转换，zip 同构字段）。
type taskSubmitterAdapter struct{ svc *service.TaskrunnerService }

func (a *taskSubmitterAdapter) Submit(ctx context.Context, in *ticket.TaskSubmitInput, actor, sourceIP, selfBaseURL string) (any, error) {
	return a.svc.Submit(ctx, &service.TaskSubmitInput{
		Action: in.Action, Params: in.Params, TimeoutSecs: in.TimeoutSecs,
	}, actor, sourceIP, selfBaseURL)
}

// cronJobFunc 函数适配 ReentrantJob（S6 B6-7 inbox_cleanup——InboxService.CleanupExpired）。
type cronJobFunc func(ctx context.Context) error

func (f cronJobFunc) Handle(ctx context.Context, _ json.RawMessage) error { return f(ctx) }

// timeoutScanProvider timeout_scan 注册表适配：Status 取自调度器 + Observe
// 标记取自装配 config（cron-status 透出——B10-4 观察模式运维确认口）。
type timeoutScanProvider struct {
	sch     *service.CronScheduler
	observe bool
}

func (p *timeoutScanProvider) Status() service.CronStatus { return p.sch.Status() }
func (p *timeoutScanProvider) Observe() bool              { return p.observe }
