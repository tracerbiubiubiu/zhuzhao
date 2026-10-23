package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/gateway"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/audit"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/channelsender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
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
	cfg        *config.Config
	logger     *slog.Logger
	router     *gin.Engine
	server     *http.Server
	policyEval *audit.PolicyEvalWriter // B11① 判定日志 L2（随 Run 的信号 ctx 启停）
	auditCron  *service.CronScheduler  // P4 cron 收归：nil=未启用（配置开关）
	storageGC  *service.CronScheduler  // phase5 S5 孤儿 GC：nil=storage 未启用
}

// NewApp 创建应用实例
// NewApp 创建应用实例。构造期执行 BK-22 fail-fast 对账（路由↔menu_apis
// 双向；缺口 = 拒绝启动）——W0b wire 转正：原在 wire_gen 手码装配段的
// 对账逻辑迁入此处，使 wire_gen 可安全再生。
func NewApp(cfg *config.Config, logger *slog.Logger, router *gin.Engine, policyEval *audit.PolicyEvalWriter,
	pool *pgxpool.Pool, gw *gateway.Registry, storageCfg *pkgstorage.ConfigStore) (*App, error) {
	var prefixes []string
	if gw != nil {
		prefixes = gw.Prefixes
	}
	// B6-1：领域事件注册表（S6/phase6/S10 共享基础设施——一次变更多订阅者）
	eventRegistry := domainevent.NewHookRegistry(logger)
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
		channelsender.NewUIPStubSender(channelsender.UIPStubConfig{StubMode: cfg.Server.Mode != "release"}, logger), // 开发态桩
		logger,
		notifSvc, // 06 号 §5.4 死信集成（实现 NotifyDeadLetterSender）
	)
	eventRegistry.Subscribe(domainevent.BizTicket, notifyHook)

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
	// P4 cron 收归：配置开关启用则装配进程内调度器（audit_archive 单批可重入——
	// ReentrantJob 契约见 cron_scheduler.go）
	var auditCron *service.CronScheduler
	if cfg.Audit.Archive.CronEnabled {
		tick, _ := time.ParseDuration(cfg.Audit.Archive.CronTick)
		job := service.NewAuditArchiveJob(repository.NewAuditLogRepo(pool),
			cfg.Audit.Archive.RetentionDays, cfg.Audit.Archive.BatchRows, cfg.Audit.Archive.OutDir,
			cfg.Audit.Archive.FileRetentionDays, logger)
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
			gc := storageservice.NewGCJob(repository.NewStorageRepo(pool), gcClient, storageCfg, logger)
			storageGC = service.NewCronScheduler(pool, "storage_gc", gc, time.Hour, logger)
			logger.Info("storage_gc cron enabled (orphan pending objects, 1h tick)")
		} else {
			logger.Error("storage_gc disabled: client init failed", "err", cerr)
		}
	}
	return &App{
		cfg:        cfg,
		logger:     logger,
		router:     router,
		policyEval: policyEval,
		auditCron:  auditCron,
		storageGC:  storageGC,
	}, nil
}

// Run 启动应用
func (a *App) Run() error {
	a.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", a.cfg.Server.Port),
		Handler: a.router,
		// F-8：补齐超时，防 slowloris 慢连接耗尽资源
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 退出信号源先行创建：server / 判定日志管道共用同一取消
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 启动 HTTP 服务（goroutine 中运行，错误通过 channel 传递）
	serverErr := make(chan error, 1)
	go func() {
		a.logger.Info("server starting",
			slog.Int("port", a.cfg.Server.Port),
			slog.String("mode", a.cfg.Server.Mode),
		)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// B11① 判定日志 L2 管道随进程生命周期启停：收到信号 → pump 限时排空 channel
	// 入 Redis + flusher 收尾落库（AfterShutdown 内 drain 的行留 Redis，重启续消不丢）
	a.policyEval.Start(ctx)

	// P4 cron 收归：进程内调度（ticker+advisory lock；配置开关启用）
	if a.auditCron != nil {
		go a.auditCron.Run(ctx)
	}

	// phase5 S5：孤儿对象 GC cron（storage.enabled 门控——NewApp 装配）
	if a.storageGC != nil {
		go a.storageGC.Run(ctx)
	}

	select {
	case err := <-serverErr:
		// 服务器异常退出（非 graceful shutdown）
		a.logger.Error("server failed", slog.Any("error", err))
		return err
	case <-ctx.Done():
		// 收到退出信号
		a.logger.Info("server shutting down...")
	}

	// 优雅关闭
	return a.Shutdown()
}

// Shutdown 优雅关闭
func (a *App) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. 停止接受新请求（即使排空失败也继续后续收尾——提前 return 会跳过
	// policyEval.Stop，drain 打在已关闭 Redis 上静默丢行）
	serverErr := a.server.Shutdown(ctx)
	if serverErr != nil {
		a.logger.Error("server forced to shutdown", slog.Any("error", serverErr))
	}

	// 2. B11①：等判定日志管道收尾（pump drain + flusher 收尾 flush）。必须在
	// main.go defer cleanup() 关闭 Redis 之前完成；超时放弃等待（残留行留 Redis
	// 重启续消不丢，但会有 Warn 留痕）
	if a.policyEval != nil {
		if !a.policyEval.Stop(5 * time.Second) {
			a.logger.Warn("policy_eval writer shutdown timeout, residual rows stay in redis")
		}
	}

	// B4-6：审计为同步写入（08-audit.md Phase 1 决策：无队列，优雅关闭无需 drain）；
	// Casbin/Redis/PG 连接关闭由 main.go defer cleanup() 依 casbin→redis→pg 逆序执行

	a.logger.Info("server stopped")
	return serverErr
}
