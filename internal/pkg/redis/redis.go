package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

// New 创建 Redis 客户端
func New(cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr(),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// 测试连接
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return client, nil
}
