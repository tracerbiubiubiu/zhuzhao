package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

// New 创建 Redis 客户端
func New(cfg config.RedisConfig) (*redis.Client, func(), error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr(),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// 测试连接
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	cleanup := func() {
		client.Close()
	}
	return client, cleanup, nil
}
