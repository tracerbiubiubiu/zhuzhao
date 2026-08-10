package main

import (
	"fmt"
	"os"

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
