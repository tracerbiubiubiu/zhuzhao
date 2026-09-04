package app

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao-utils/logger"
	"github.com/tracerbiubiubiu/zhuzhao-utils/postgres"
	"github.com/tracerbiubiubiu/zhuzhao-utils/redis"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/audit"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// 适配层：zhuzhao-utils 的基建包自带 Config 类型（与 internal/config 解耦），
// 这里把应用配置逐字段映射过去。

func provideLogger(cfg config.LogConfig) *slog.Logger {
	return logger.New(logger.Config{
		Level:      cfg.Level,
		Dir:        cfg.Dir,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
	})
}

func provideJWTManager(cfg config.JWTConfig) *jwt.Manager {
	return jwt.NewManager(jwt.Config{
		Secret:    cfg.Secret,
		AccessTTL: cfg.AccessTTL,
	})
}

func providePostgres(cfg config.DatabaseConfig) (*pgxpool.Pool, func(), error) {
	return postgres.New(postgres.Config{
		Host:             cfg.Host,
		Port:             cfg.Port,
		User:             cfg.User,
		Password:         cfg.Password,
		DBName:           cfg.DBName,
		MaxOpenConns:     cfg.MaxOpenConns,
		MaxIdleConns:     cfg.MaxIdleConns,
		ConnMaxLifetime:  cfg.ConnMaxLifetime,
		ConnMaxIdleTime:  cfg.ConnMaxIdleTime,
		ConnectTimeout:   cfg.ConnectTimeout,
		SSLMode:          cfg.SSLMode,
		StatementTimeout: cfg.StatementTimeout,
		ApplicationName:  cfg.ApplicationName,
	})
}

func provideRedis(cfg config.RedisConfig) (*goredis.Client, func(), error) {
	return redis.New(redis.Config{
		Host:            cfg.Host,
		Port:            cfg.Port,
		DB:              cfg.DB,
		Password:        cfg.Password,
		PoolSize:        cfg.PoolSize,
		MinIdleConns:    cfg.MinIdleConns,
		PoolTimeout:     cfg.PoolTimeout,
		DialTimeout:     cfg.DialTimeout,
		ReadTimeout:     cfg.ReadTimeout,
		WriteTimeout:    cfg.WriteTimeout,
		MaxRetries:      cfg.MaxRetries,
		ConnMaxIdleTime: cfg.ConnMaxIdleTime,
	})
}

// 锁定参数取库默认值（与原 internal/pkg/redis 行为一致）。NewScripts 的
// variadic 形参 wire 无法注入，故固定为无参调用。
func provideRedisScripts(client *goredis.Client) *redis.Scripts {
	return redis.NewScripts(client)
}

// providePolicyEvalWriter 判定日志 L2 writer（B11①，03-audit-l2 §2；参数零值走默认）。
func providePolicyEvalWriter(cfg config.AuditConfig, rdb *goredis.Client,
	repo *repository.AuditLogRepo, logger *slog.Logger) *audit.PolicyEvalWriter {
	return audit.NewPolicyEvalWriter(audit.PolicyEvalConfig{
		BufferSize:    cfg.PolicyEval.BufferSize,
		BatchSize:     cfg.PolicyEval.BatchSize,
		FlushInterval: cfg.PolicyEval.FlushInterval,
		RedisKey:      cfg.PolicyEval.RedisKey,
	}, rdb, repo, logger)
}

// provideRegistry 建 Registry 并挂判定埋点（B11①：registry.Authorize 每次判定 →
// writer.Write 非阻塞入队；SetEvalHook 后注册的资源自动纳入）。
func provideRegistry(w *audit.PolicyEvalWriter) resource.Registry {
	reg := resource.NewRegistry()
	reg.SetEvalHook(func(ctx context.Context, e resource.EvalEntry) {
		w.Write(audit.PolicyEvalEntry{
			ActorID:      e.ActorID,
			ActorRoles:   e.ActorRoles,
			ResourceType: e.ResourceType,
			ResourceID:   e.ResourceID,
			Action:       e.Action,
			Result:       e.Result,
			Reason:       e.Reason,
			TraceID:      e.TraceID,
		})
	})
	return reg
}
