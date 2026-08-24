package service

import (
	"context"
	"fmt"
	"log/slog"
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

// revokeUserSessionsWithRetry revokeUserSessions 的补偿重试版（D2-05）。
// 场景：禁用/删除/重置密码/改密均「先提交 DB、后吊销会话」——Redis 闪断时
// 部分写成立（DB 已禁用、存量 AT/RT 仍有效），原一次失败直接 500 无重试。
// 短退避重试 3 次；最终失败以 reconcile 标记落 Error 日志供运维对账
// （对账 SQL：SELECT id,username,status FROM users WHERE status=0 /
// audit_logs 中 reconcile=revive 的补吊销操作），仍向上返回错误（503）。
func revokeUserSessionsWithRetry(ctx context.Context, rdb *goredis.Client, userID int64, accessTTL time.Duration) error {
	const attempts = 3
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			// 100ms / 200ms 退避，吸收瞬时闪断
			select {
			case <-time.After(time.Duration(i) * 100 * time.Millisecond):
			case <-ctx.Done():
				slog.Error("session revoke aborted by ctx, reconcile required", "user_id", userID, "tag", "reconcile", "err", ctx.Err())
				return ctx.Err()
			}
		}
		if lastErr = revokeUserSessions(ctx, rdb, userID, accessTTL); lastErr == nil {
			return nil
		}
	}
	slog.Error("session revoke failed after retries; DB committed but sessions alive, reconcile required",
		"user_id", userID, "tag", "reconcile", "err", lastErr)
	return lastErr
}
