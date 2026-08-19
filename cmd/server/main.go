package main

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/app"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

func main() {
	// 加载配置
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		os.Exit(1)
	}
	if cfg.UsesWeakJWTSecret() && cfg.Server.Mode != "release" {
		fmt.Fprintf(os.Stderr, "WARN: jwt.secret is default or weak; set JWT_SECRET before production\n")
	}
	if err := cfg.Validate(); err != nil {
		fmt.Printf("invalid config: %v\n", err)
		os.Exit(1)
	}

	// 在 router 创建前设置 Gin 模式，避免 release 下输出 debug 路由日志
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 初始化应用（Wire 注入）
	application, cleanup, err := app.InitializeApp(cfg)
	if err != nil {
		fmt.Printf("failed to initialize app: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	// 启动
	if err := application.Run(); err != nil {
		fmt.Printf("server error: %v\n", err)
		os.Exit(1)
	}
}
