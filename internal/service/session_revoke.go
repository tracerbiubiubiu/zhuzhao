package service

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

func revokeUserSessions(ctx context.Context, rdb *goredis.Client, userID int64, accessTTL time.Duration) error {
	disabledKey := fmt.Sprintf("user:disabled:%d", userID)
	if err := rdb.Set(ctx, disabledKey, "1", accessTTL).Err(); err != nil {
		return fmt.Errorf("set user disabled: %w", err)
	}
	pattern := fmt.Sprintf("refresh:%d:*", userID)
	iter := rdb.Scan(ctx, 0, pattern, 100).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("scan refresh keys: %w", err)
	}
	if len(keys) > 0 {
		if err := rdb.Del(ctx, keys...).Err(); err != nil {
			return fmt.Errorf("del refresh keys: %w", err)
		}
	}
	return nil
}

func clearUserDisabled(ctx context.Context, rdb *goredis.Client, userID int64) error {
	return rdb.Del(ctx, fmt.Sprintf("user:disabled:%d", userID)).Err()
}
