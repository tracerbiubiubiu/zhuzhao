package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// pwEpochKey 密码纪元键（C3）：改密/重置/禁用/删除等全量吊销事件 INCR——
// AT/RT 均携带签发纪元，JWT/Refresh 与当前值不符即拒。键无 TTL，需随
// 会话 Redis 持久化；若纪元丢失，仍未过期的纪元 0 AT 可能重新有效，
// 灾难恢复须同时轮换 JWT secret 或等待旧 AT 全过期，不能称 Redis 丢失无回退面。
func pwEpochKey(userID int64) string {
	return fmt.Sprintf("user:pw_epoch:%d", userID)
}

// currentUserPwEpoch 只有缺键才视为旧纪元 0；错误/损坏不可回退为 0。
func currentUserPwEpoch(ctx context.Context, rdb *goredis.Client, userID int64) (int64, error) {
	n, err := rdb.Get(ctx, pwEpochKey(userID)).Int64()
	if errors.Is(err, goredis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, fmt.Errorf("invalid password epoch")
	}
	return n, nil
}

func revokeUserSessions(ctx context.Context, rdb *goredis.Client, userID int64, accessTTL time.Duration) error {
	_, err := revokeUserSessionsWithEpoch(ctx, rdb, userID, accessTTL)
	return err
}

// revokeUserSessionsWithEpoch 返回本次 INCR 的值，不能在返回后重读并借用另一笔撤销的纪元。
func revokeUserSessionsWithEpoch(ctx context.Context, rdb *goredis.Client, userID int64, accessTTL time.Duration) (int64, error) {
	disabledKey := fmt.Sprintf("user:disabled:%d", userID)
	if err := rdb.Set(ctx, disabledKey, "1", accessTTL).Err(); err != nil {
		return 0, fmt.Errorf("set user disabled: %w", err)
	}
	epoch, err := rdb.Incr(ctx, pwEpochKey(userID)).Result()
	if err != nil {
		return 0, fmt.Errorf("bump pw epoch: %w", err)
	}
	if epoch <= 0 {
		return 0, fmt.Errorf("invalid password epoch after increment")
	}
	pattern := fmt.Sprintf("refresh:%d:*", userID)
	iter := rdb.Scan(ctx, 0, pattern, 100).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return 0, fmt.Errorf("scan refresh keys: %w", err)
	}
	if len(keys) > 0 {
		if err := rdb.Del(ctx, keys...).Err(); err != nil {
			return 0, fmt.Errorf("del refresh keys: %w", err)
		}
	}
	return epoch, nil
}

// clearUserDisabledAtEpoch 只允许本次认证的纪元清标记，避免在途登录/改密清掉后来撤销。
func clearUserDisabledAtEpoch(ctx context.Context, rdb *goredis.Client, userID, epoch int64) (bool, error) {
	n, err := rdb.Eval(ctx, `
local current = redis.call('GET', KEYS[1]) or '0'
if current ~= ARGV[1] then return 0 end
redis.call('DEL', KEYS[2])
return 1`, []string{pwEpochKey(userID), fmt.Sprintf("user:disabled:%d", userID)}, fmt.Sprint(epoch)).Int64()
	return n == 1, err
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
	_, err := revokeUserSessionsWithRetryEpoch(ctx, rdb, userID, accessTTL)
	return err
}

func revokeUserSessionsWithRetryEpoch(ctx context.Context, rdb *goredis.Client, userID int64, accessTTL time.Duration) (int64, error) {
	const attempts = 3
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			// 100ms / 200ms 退避，吸收瞬时闪断
			select {
			case <-time.After(time.Duration(i) * 100 * time.Millisecond):
			case <-ctx.Done():
				slog.Error("session revoke aborted by ctx, reconcile required", "user_id", userID, "tag", "reconcile", "err", ctx.Err())
				return 0, ctx.Err()
			}
		}
		epoch, err := revokeUserSessionsWithEpoch(ctx, rdb, userID, accessTTL)
		lastErr = err
		if err == nil {
			return epoch, nil
		}
	}
	slog.Error("session revoke failed after retries; DB committed but sessions alive, reconcile required",
		"user_id", userID, "tag", "reconcile", "err", lastErr)
	return 0, lastErr
}

// RevokeUserSessionsWithRetry 导出包装（break-glass admincli 复用）。语义与内部版
// 完全一致：DB 提交之后调用（disabled 标记 + 密码纪元 +1 + refresh 键清除），
// 短退避重试 3 次，最终失败上抛并带 reconcile 标记日志。
func RevokeUserSessionsWithRetry(ctx context.Context, rdb *goredis.Client, userID int64, accessTTL time.Duration) error {
	return revokeUserSessionsWithRetry(ctx, rdb, userID, accessTTL)
}
