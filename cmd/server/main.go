package main

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/app"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

// run 业务逻辑（B4-6：defer cleanup 在函数内注册——原 main 直调 os.Exit(1)
// 会跳过 defer，错误路径 DB/Redis/Casbin 不优雅关闭）
func run() error {
	// 加载配置
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	// D2-09：弱/仓库默认密钥已在 Validate 中无条件拒绝（原 debug 放行 + 启动期告警）
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// 在 router 创建前设置 Gin 模式，避免 release 下输出 debug 路由日志
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 初始化应用（Wire 注入）
	application, cleanup, err := app.InitializeApp(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize app: %w", err)
	}
	defer cleanup()

	return application.Run()
}

func main() {
	if err := run(); err != nil {
		fmt.Printf("server error: %v\n", err)
		os.Exit(1)
	}
}
