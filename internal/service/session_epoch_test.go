package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestSessionEpoch_StrictReadAndConditionalClear(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()
	n, err := currentUserPwEpoch(ctx, rdb, 42)
	require.NoError(t, err)
	require.Zero(t, n)
	for _, raw := range []string{"broken", "-1"} {
		require.NoError(t, rdb.Set(ctx, pwEpochKey(42), raw, 0).Err())
		_, err = currentUserPwEpoch(ctx, rdb, 42)
		require.Error(t, err)
	}
	require.NoError(t, rdb.Del(ctx, pwEpochKey(42)).Err())
	epoch, err := revokeUserSessionsWithRetryEpoch(ctx, rdb, 42, time.Minute)
	require.NoError(t, err)
	require.EqualValues(t, 1, epoch)
	// 撤销先发生，在途旧登录不能再清除新拒绝标记。
	ok, err := clearUserDisabledAtEpoch(ctx, rdb, 42, 0)
	require.NoError(t, err)
	require.False(t, ok)
	require.True(t, mr.Exists("user:disabled:42"))
	ok, err = clearUserDisabledAtEpoch(ctx, rdb, 42, epoch)
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, mr.Exists("user:disabled:42"))
	mr.SetError("unavailable")
	_, err = currentUserPwEpoch(ctx, rdb, 42)
	require.Error(t, err)
}
