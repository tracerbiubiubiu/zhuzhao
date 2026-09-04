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

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/audit"
)

// App 应用实例
type App struct {
	cfg        *config.Config
	logger     *slog.Logger
	router     *gin.Engine
	server     *http.Server
	policyEval *audit.PolicyEvalWriter // B11① 判定日志 L2（随 Run 的信号 ctx 启停）
}

// NewApp 创建应用实例
func NewApp(cfg *config.Config, logger *slog.Logger, router *gin.Engine, policyEval *audit.PolicyEvalWriter) *App {
	return &App{
		cfg:        cfg,
		logger:     logger,
		router:     router,
		policyEval: policyEval,
	}
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

	// 1. 停止接受新请求
	if err := a.server.Shutdown(ctx); err != nil {
		a.logger.Error("server forced to shutdown", slog.Any("error", err))
		return err
	}

	// B4-6：审计为同步写入（08-audit.md Phase 1 决策：无队列，优雅关闭无需 drain）；
	// Casbin/Redis/PG 连接关闭由 main.go defer cleanup() 依 casbin→redis→pg 逆序执行

	a.logger.Info("server stopped")
	return nil
}
