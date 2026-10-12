package main

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/app"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

//	@title		zhuzhao API
//	@version	1.0
//	@description	zhuzhao——Go 模块化单体 IAM + 工单系统（三层鉴权 + 资源级工单）

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
	// 子命令分发：默认行为仍是启动 server（保持 ENTRYPOINT 兼容，零镜像改动）。
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "reset-password":
			if err := runResetPassword(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "reset-password: %v\n", err)
				os.Exit(1)
			}
			return
		case "help", "-h", "--help":
			fmt.Fprint(os.Stderr, mainUsage)
			return
		}
	}
	if err := run(); err != nil {
		fmt.Printf("server error: %v\n", err)
		os.Exit(1)
	}
}

const mainUsage = `zhuzhao

用法:
  zhuzhao                       启动 API server（默认）
  zhuzhao reset-password [flags]  break-glass 重置 superadmin 密码（见 docs/runbook.md §1）
  zhuzhao help                  显示本帮助
`
