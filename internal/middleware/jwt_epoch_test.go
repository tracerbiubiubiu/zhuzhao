package middleware_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
)

func TestJWT_PasswordEpoch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		claim  int64
		stored string
		status int
	}{
		{"legacy_no_key", 0, "", 200},
		{"old_epoch_same_second", 0, "1", 401},
		{"current_epoch_same_second", 1, "1", 200},
		{"future_claim", 2, "1", 401},
		{"missing_nonzero_epoch", 1, "", 401},
		{"malformed", 0, "broken", 503},
		{"negative", 0, "-1", 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, w, manager, rdb := newJWTTestEnv(t, time.Minute)
			at, _, err := manager.GenerateAccessTokenWithEpoch(42, "alice", false, tc.claim)
			require.NoError(t, err)
			claims, err := manager.ParseAccessToken(at)
			require.NoError(t, err)
			// 固定等于 iat 的地板，稳定复现同秒边界，不依赖睡眠/低概率抢秒。
			require.NoError(t, rdb.Set(c, "user:at_min_iat:42", claims.IssuedAt.Unix(), time.Minute).Err())
			if tc.stored != "" {
				require.NoError(t, rdb.Set(c, "user:pw_epoch:42", tc.stored, 0).Err())
			}
			bearerRequest(c, at)
			middleware.JWT(manager, rdb, nil)(c)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, tc.status != 200, c.IsAborted())
		})
	}
}

func TestFileSession_PasswordEpoch(t *testing.T) {
	c, w, manager, rdb := newJWTTestEnv(t, time.Minute)
	at, _, err := manager.GenerateAccessToken(42, "alice", false)
	require.NoError(t, err)
	require.NoError(t, rdb.Set(c, fmt.Sprintf("user:pw_epoch:%d", 42), 1, 0).Err())
	c.Request.AddCookie(&http.Cookie{Name: middleware.FileSessionCookieName, Value: at})
	middleware.FileAuth(middleware.JWT(manager, rdb, nil))(c)
	require.True(t, c.IsAborted())
	require.Equal(t, 401, w.Code)
}

func TestJWT_EpochWrongTypeAfterMissingFloor_FailsClosed(t *testing.T) {
	c, w, manager, rdb := newJWTTestEnv(t, time.Minute)
	at, _, err := manager.GenerateAccessToken(42, "alice", false)
	require.NoError(t, err)
	// GET floor 的首个 redis.Nil 不得掩盖后续 GET epoch 的 WRONGTYPE。
	require.NoError(t, rdb.LPush(c, "user:pw_epoch:42", "not-an-integer").Err())
	bearerRequest(c, at)
	middleware.JWT(manager, rdb, nil)(c)
	require.True(t, c.IsAborted())
	require.Equal(t, 503, w.Code)
}
