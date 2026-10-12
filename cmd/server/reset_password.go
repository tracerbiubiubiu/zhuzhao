package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao-utils/postgres"
	"github.com/tracerbiubiubiu/zhuzhao-utils/redis"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/admincli"
)

// runResetPassword break-glass 子命令（docs/runbook.md §1）：交互/参数层——
// 录密与确认在此完成，核心语义在 internal/pkg/admincli（进门禁测试覆盖）。
func runResetPassword(args []string) error {
	fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	username := fs.String("username", "", "目标用户名（必填，须持启用中的 superadmin 角色）")
	passwordStdin := fs.Bool("password-stdin", false, "从 stdin 读一行密码（管道形态；交互形态为明文双录）")
	yes := fs.Bool("yes", false, "跳过确认提示（脚本化场景用；默认需交互输入 yes）")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *username == "" {
		fs.Usage()
		return errors.New("--username 必填")
	}

	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	// 只消费 DB/Redis/JWT TTL 三节，不做服务端全量 Validate——后者含
	// internal_jobs 的 SK 强制等与服务端启动耦合的检查，不应构成应急
	// 路径前置条件（dev 栈演练实测：enabled=true 无 SK 即被卡）。
	// 最低限度自检兜住「配置缺字段」类事故。
	if cfg.Database.Host == "" || cfg.Database.DBName == "" || cfg.Redis.Host == "" {
		return errors.New("配置缺少 database/redis 必要字段")
	}
	accessTTL := cfg.JWT.AccessTTL
	if accessTTL <= 0 {
		accessTTL = 15 * time.Minute // disabled 标记兜底 TTL（零值会使标记即刻过期）
	}

	br := bufio.NewReader(os.Stdin)
	password, err := readPassword(br, *passwordStdin)
	if err != nil {
		return err
	}
	if len(password) < 8 {
		return errors.New("新密码长度须 ≥8（与管理端重置契约一致）")
	}

	if !*yes {
		fmt.Printf("即将重置用户 %q 的密码（下次登录强制改密）并吊销其全部会话。\n确认请输入 yes：", *username)
		line, _ := br.ReadString('\n')
		if strings.TrimSpace(line) != "yes" {
			return errors.New("已取消")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, cleanupPool, err := postgres.New(postgres.Config{
		Host:            cfg.Database.Host,
		Port:            cfg.Database.Port,
		User:            cfg.Database.User,
		Password:        cfg.Database.Password,
		DBName:          cfg.Database.DBName,
		MaxOpenConns:    2, // 一次性工具，不占连接池水位
		ConnectTimeout:  cfg.Database.ConnectTimeout,
		SSLMode:         cfg.Database.SSLMode,
		ApplicationName: "zhuzhao-admincli",
	})
	if err != nil {
		return fmt.Errorf("连接 DB 失败: %w", err)
	}
	defer cleanupPool()

	rdb, cleanupRDB, err := redis.New(redis.Config{
		Host:         cfg.Redis.Host,
		Port:         cfg.Redis.Port,
		DB:           cfg.Redis.DB,
		Password:     cfg.Redis.Password,
		DialTimeout:  cfg.Redis.DialTimeout,
		ReadTimeout:  cfg.Redis.ReadTimeout,
		WriteTimeout: cfg.Redis.WriteTimeout,
		MaxRetries:   cfg.Redis.MaxRetries,
	})
	if err != nil {
		return fmt.Errorf("连接 Redis 失败: %w", err)
	}
	defer cleanupRDB()

	res, err := admincli.ResetSuperadminPassword(ctx, admincli.ResetOptions{
		Username:  *username,
		Password:  password,
		Note:      breakGlassNote(),
		Pool:      pool,
		RDB:       rdb,
		AccessTTL: accessTTL,
	})
	if err != nil {
		return err
	}

	fmt.Printf("✅ 密码已重置：user_id=%d username=%s（下次登录强制改密）\n", res.UserID, res.Username)
	if res.Reenabled {
		fmt.Println("ℹ️ 目标原处于禁用态，已连带启用。")
	}
	if !res.SessionsRevoked {
		fmt.Fprintln(os.Stderr, "⚠️ 会话吊销失败（Redis 异常）——存量 token 可能残留，按 runbook §1.4 对账步骤清理。")
	}
	if !res.AuditWritten {
		fmt.Fprintln(os.Stderr, "⚠️ 审计落库失败——请按 runbook §1.5 人工补记台账。")
	}
	fmt.Println("提醒：本次为 break-glass 操作，请补记人工台账（runbook §1.5）。")
	return nil
}

// readPassword 密码输入：管道形态读一行；交互形态明文双录（不引 x/term 新依赖，
// 共享终端建议用 --password-stdin，runbook §1.3 已注记）。br 由调用方创建并
// 与确认提示共用，避免多 Reader 缓冲吃行。
func readPassword(br *bufio.Reader, fromStdin bool) (string, error) {
	if fromStdin {
		line, err := br.ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("读取密码失败: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Print("新密码: ")
	first, err := br.ReadString('\n')
	if err != nil && first == "" {
		return "", fmt.Errorf("读取密码失败: %w", err)
	}
	pw := strings.TrimRight(first, "\r\n")
	fmt.Print("再输一次: ")
	second, err := br.ReadString('\n')
	if err != nil && second == "" {
		return "", fmt.Errorf("读取密码失败: %w", err)
	}
	if strings.TrimRight(second, "\r\n") != pw {
		return "", errors.New("两次输入不一致")
	}
	return pw, nil
}

// breakGlassNote 审计注记：记录执行宿主（操作者身份由人工台账补全）。
func breakGlassNote() string {
	host, _ := os.Hostname()
	return "executed via CLI on " + host
}
