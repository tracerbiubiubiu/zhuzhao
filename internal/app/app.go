package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
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
	// 设置 Gin 模式
	if a.cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	a.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", a.cfg.Server.Port),
		Handler: a.router,
	}

	// 启动 HTTP 服务
	go func() {
		a.logger.Info("server starting",
			slog.Int("port", a.cfg.Server.Port),
			slog.String("mode", a.cfg.Server.Mode),
		)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("server failed", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	a.logger.Info("server shutting down...")

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
	// TODO: 3. 关闭 Casbin enforcer
	// TODO: 4. 关闭 Redis 连接
	// TODO: 5. 关闭 PostgreSQL 连接池

	a.logger.Info("server stopped")
	return nil
}
