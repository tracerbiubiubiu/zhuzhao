package service

import (
	"context"
	"testing"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// A float conversion collapses these adjacent epochs, letting a stale worker
// overwrite the newer state. Expectations are literal decimal int64 values.
func TestDBProjection_Int64OrderingRejectsStaleWorker(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()
	for _, pair := range [][2]string{
		{"9007199254740993", "9007199254740992"},
		{"9223372036854775807", "9223372036854775806"},
	} {
		require.NoError(t, rdb.Set(ctx, "user:session_epoch:db1:42", pair[0], 0).Err())
		require.NoError(t, rdb.Set(ctx, "user:session_disabled:db1:42", "1", 0).Err())
		n, err := rdb.Eval(ctx, installProjectionEpochLua,
			[]string{"user:session_epoch:db1:42", "user:session_disabled:db1:42"},
			pair[1], "1", "60000").Int64()
		require.NoError(t, err)
		require.Zero(t, n, "stale projection must not acknowledge installation")
		require.Equal(t, pair[0], mustProjectionValue(t, mr, "user:session_epoch:db1:42"))
		require.True(t, mr.Exists("user:session_disabled:db1:42"))
	}
}

func TestDBProjection_RejectsCorruptEpoch(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()
	for _, raw := range []string{"0", "-1", "+1", "01", "1.0", "9223372036854775808", "bad"} {
		t.Run(raw, func(t *testing.T) {
			require.NoError(t, rdb.Set(ctx, "user:session_epoch:db1:42", raw, 0).Err())
			_, err := rdb.Eval(ctx, installProjectionEpochLua,
				[]string{"user:session_epoch:db1:42", "user:session_disabled:db1:42"},
				"2", "1", "60000").Result()
			require.Error(t, err, "corrupt projection must require controlled recovery")
		})
	}
}

func mustProjectionValue(t *testing.T, mr *miniredis.Miniredis, key string) string {
	t.Helper()
	value, err := mr.Get(key)
	require.NoError(t, err)
	return value
}

// Installing or retrying an older projection must not erase the current login,
// touch Pwe, or remove any generation-scoped refresh slot.
func TestDBProjection_IsolatedKeysPreserveRefreshSlots(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()
	originals := map[string]string{
		"user:pw_epoch:42": "71", "user:disabled:42": "1",
		"refresh:42:browser":       "legacy-login",
		"refresh:db1:42:3:browser": "current-login",
		"refresh:db1:42:2:browser": "old-login",
	}
	for key, value := range originals {
		require.NoError(t, rdb.Set(ctx, key, value, 0).Err())
	}
	require.NoError(t, installRedisSessionProjection(ctx, rdb, 42, 3, true))
	require.Error(t, installRedisSessionProjection(ctx, rdb, 42, 2, false))
	require.NoError(t, installRedisSessionProjection(ctx, rdb, 42, 3, true))
	require.Equal(t, "3", mustProjectionValue(t, mr, "user:session_epoch:db1:42"))
	require.False(t, mr.Exists("user:session_disabled:db1:42"))
	for key, value := range originals {
		require.Equal(t, value, mustProjectionValue(t, mr, key))
	}
}

func TestDBProjection_ReadinessRequiresCurrentDBAndRedisEvidence(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()
	deps := SessionSecurityDeps{rdb: rdb}
	snap := &repository.UserSecuritySnapshot{SessionEpoch: 3, ProjectionEpoch: 3}
	require.Error(t, CheckProjectionReadiness(ctx, deps, 42, snap), "missing Redis is not epoch zero")
	for _, raw := range []string{"2", "4", "03", "+3", "bad"} {
		require.NoError(t, rdb.Set(ctx, "user:session_epoch:db1:42", raw, 0).Err())
		require.Error(t, CheckProjectionReadiness(ctx, deps, 42, snap))
	}
	require.NoError(t, rdb.Set(ctx, "user:session_epoch:db1:42", "3", 0).Err())
	require.NoError(t, CheckProjectionReadiness(ctx, deps, 42, snap))
	snap.ProjectionEpoch = 2
	require.Error(t, CheckProjectionReadiness(ctx, deps, 42, snap))
	snap.ProjectionEpoch = 4
	require.Error(t, CheckProjectionReadiness(ctx, deps, 42, snap))
	snap.ProjectionEpoch = 3
	require.NoError(t, rdb.Set(ctx, "user:session_disabled:db1:42", "1", time.Minute).Err())
	require.Error(t, CheckProjectionReadiness(ctx, deps, 42, snap))
	require.NoError(t, rdb.Del(ctx, "user:session_disabled:db1:42").Err())
	mr.SetError("unavailable")
	require.Error(t, CheckProjectionReadiness(ctx, deps, 42, snap))
}

func TestDBProjection_InvalidTargetLeavesProjectionMissing(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()
	for _, epoch := range []int64{0, -1} {
		require.Error(t, installRedisSessionProjection(ctx, rdb, 42, epoch, true))
	}
	require.False(t, mr.Exists("user:session_epoch:db1:42"))
	require.NoError(t, installRedisSessionProjection(ctx, rdb, 42, 9223372036854775807, false))
	require.Equal(t, "9223372036854775807", mustProjectionValue(t, mr, "user:session_epoch:db1:42"))
	require.True(t, mr.Exists("user:session_disabled:db1:42"))
}

// Legacy revocation retains its old namespace and cannot destroy a DB-epoch login.
func TestDBProjection_LegacyRevokeCannotTouchNewSlots(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()
	require.NoError(t, installRedisSessionProjection(ctx, rdb, 42, 3, true))
	require.NoError(t, rdb.Set(ctx, "refresh:db1:42:3:browser", "new-login", 0).Err())
	require.NoError(t, rdb.Set(ctx, "refresh:42:browser", "legacy-login", 0).Err())
	_, err := revokeUserSessionsWithRetryEpoch(ctx, rdb, 42, time.Minute)
	require.NoError(t, err)
	require.False(t, mr.Exists("refresh:42:browser"))
	require.Equal(t, "new-login", mustProjectionValue(t, mr, "refresh:db1:42:3:browser"))
	require.Equal(t, "3", mustProjectionValue(t, mr, "user:session_epoch:db1:42"))
	require.False(t, mr.Exists("user:session_disabled:db1:42"))
}
