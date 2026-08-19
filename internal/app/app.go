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
)

// App 应用实例
type App struct {
	cfg    *config.Config
	logger *slog.Logger
	router *gin.Engine
	server *http.Server
}

// NewApp 创建应用实例
func NewApp(cfg *config.Config, logger *slog.Logger, router *gin.Engine) *App {
	return &App{
		cfg:    cfg,
		logger: logger,
		router: router,
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

	// 等待退出信号或服务器错误
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	// TODO: 2. 刷空审计日志队列
	// TODO: 3. 关闭 Casbin enforcer（由 cleanup 函数处理）
	// TODO: 4. 关闭 Redis 连接（由 cleanup 函数处理）
	// TODO: 5. 关闭 PostgreSQL 连接池（由 cleanup 函数处理）

	a.logger.Info("server stopped")
	return nil
}
