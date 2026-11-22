package middleware_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	standardjwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
)

// fakeAccountSnapshotter C05 DB 安全快照桩：账户事实判定不依赖真实 DB。
type fakeAccountSnapshotter struct {
	snap middleware.AccountSnapshot
	err  error
}

func (f *fakeAccountSnapshotter) AccountSnapshot(_ context.Context, _ int64) (middleware.AccountSnapshot, error) {
	return f.snap, f.err
}

// fakePATGetter PAT 查验桩。
type fakePATGetter struct {
	userID, patID int64
	userName      string
	err           error
}

func (f *fakePATGetter) FindActiveByHash(context.Context, string) (int64, string, int64, error) {
	return f.userID, f.userName, f.patID, f.err
}

func (f *fakePATGetter) TouchLastUsed(context.Context, int64) {}

// C05 卡一核心断言：停用已提交而 Redis 撤销投影失败时，DB 快照终态拒绝——
// Redis disabled 键缺省不能替代 DB 判断（修前该分支放行，首条红例）。
func TestJWT_AccountSnapshot_DisabledTerminal(t *testing.T) {
	c, w, manager, rdb := newJWTTestEnv(t, time.Minute)
	at, _, err := manager.GenerateAccessTokenWithEpoch(7, "bob", false, 1)
	require.NoError(t, err)
	require.NoError(t, rdb.Set(c, "user:pw_epoch:7", 1, 0).Err()) // Redis 旧值仍可读
	bearerRequest(c, at)

	middleware.JWT(manager, rdb, nil,
		middleware.WithAccountSecurity(&fakeAccountSnapshotter{snap: middleware.AccountSnapshot{Status: 0, SessionEpoch: 2}}),
	)(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), `"code":30003`)
}

// 软删账号终态 20003（重新登录语义，与禁用 30003 区分）。
func TestJWT_AccountSnapshot_DeletedTerminal(t *testing.T) {
	c, w, manager, rdb := newJWTTestEnv(t, time.Minute)
	at, _, err := manager.GenerateAccessToken(7, "bob", false)
	require.NoError(t, err)
	bearerRequest(c, at)

	middleware.JWT(manager, rdb, nil,
		middleware.WithAccountSecurity(&fakeAccountSnapshotter{snap: middleware.AccountSnapshot{Status: 1, Deleted: true, SessionEpoch: 2}}),
	)(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), `"code":20003`)
}

// DB 不可读 = 依赖故障 503（fail-close，不回退 Redis 判定）。
func TestJWT_AccountSnapshot_DBErrorFailClose(t *testing.T) {
	c, w, manager, rdb := newJWTTestEnv(t, time.Minute)
	at, _, err := manager.GenerateAccessToken(7, "bob", false)
	require.NoError(t, err)
	bearerRequest(c, at)

	middleware.JWT(manager, rdb, nil,
		middleware.WithAccountSecurity(&fakeAccountSnapshotter{err: errors.New("pg down")}),
	)(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// 正常路径：注入 session_epoch（int64，C04 消费），既有调用方兼容装配（无选项）
// 行为不变。
func TestJWT_AccountSnapshot_ActiveInjectsSessionEpoch(t *testing.T) {
	c, _, manager, rdb := newJWTTestEnv(t, time.Minute)
	at, _, err := manager.GenerateAccessTokenWithSessionEpoch(7, "bob", false, 0, 3)
	require.NoError(t, err)
	require.NoError(t, rdb.Set(c, "user:session_epoch:db1:7", "3", 0).Err())
	bearerRequest(c, at)

	middleware.JWT(manager, rdb, nil,
		middleware.WithAccountSecurity(&fakeAccountSnapshotter{snap: middleware.AccountSnapshot{Status: 1, SessionEpoch: 3, ProjectionEpoch: 3}}),
	)(c)

	assert.False(t, c.IsAborted())
	v, ok := c.Get("session_epoch")
	require.True(t, ok, "session_epoch 必须注入")
	epoch, ok := v.(int64)
	require.True(t, ok, "session_epoch 必须为 int64，实际 %T", v)
	assert.EqualValues(t, 3, epoch)
}

// PAT 分支：校验后注入 pat_id（int64，C04 消费）。
func TestJWT_PAT_InjectsPatID(t *testing.T) {
	c, _, manager, rdb := newJWTTestEnv(t, time.Minute)
	token := "zpat_" + "abcdefghij"
	bearerRequest(c, token)

	middleware.JWT(manager, rdb, &fakePATGetter{userID: 7, userName: "bob", patID: 42})(c)

	assert.False(t, c.IsAborted())
	v, ok := c.Get("pat_id")
	require.True(t, ok, "pat_id 必须注入")
	patID, ok := v.(int64)
	require.True(t, ok, "pat_id 必须为 int64，实际 %T", v)
	assert.EqualValues(t, 42, patID)
	assert.EqualValues(t, 7, c.GetInt64("userID"))
}

// 仅带文件 cookie（无 header）经 FileAuth 桥接后同样吃 DB 快照终态拒绝——
// 停用投影失败时 cookie 通道不残留旁路。
func TestFileSession_AccountSnapshot_DisabledTerminal(t *testing.T) {
	c, w, manager, rdb := newJWTTestEnv(t, time.Minute)
	at, _, err := manager.GenerateAccessTokenWithEpoch(7, "bob", false, 1)
	require.NoError(t, err)
	require.NoError(t, rdb.Set(c, fmt.Sprintf("user:pw_epoch:%d", 7), 1, 0).Err())
	c.Request.AddCookie(&http.Cookie{Name: middleware.FileSessionCookieName, Value: at})

	middleware.FileAuth(middleware.JWT(manager, rdb, nil,
		middleware.WithAccountSecurity(&fakeAccountSnapshotter{snap: middleware.AccountSnapshot{Status: 0, SessionEpoch: 2}}),
	))(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), `"code":30003`)
}

// A DB-backed chain must use only the signed session epoch, never copy the DB epoch
// into a legacy token or accept a signed epoch for another account generation.
func TestJWT_AccountSnapshot_RejectsUnboundOrMismatchedEpoch(t *testing.T) {
	for _, epoch := range []int64{0, 2, 4} {
		t.Run(fmt.Sprintf("signed_epoch_%d", epoch), func(t *testing.T) {
			c, w, manager, rdb := newJWTTestEnv(t, time.Minute)
			var token string
			var err error
			if epoch == 0 {
				token, _, err = manager.GenerateAccessToken(7, "bob", false)
			} else {
				token, _, err = manager.GenerateAccessTokenWithSessionEpoch(7, "bob", false, 0, epoch)
			}
			require.NoError(t, err)
			bearerRequest(c, token)
			middleware.JWT(manager, rdb, nil, middleware.WithAccountSecurity(&fakeAccountSnapshotter{
				snap: middleware.AccountSnapshot{Status: 1, SessionEpoch: 3},
			}))(c)
			require.True(t, c.IsAborted())
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.Contains(t, w.Body.String(), `"code":20003`)
			_, injected := c.Get("session_epoch")
			require.False(t, injected, "rejected tokens must not acquire a DB epoch")
		})
	}
}

func TestJWT_PAT_AccountSnapshotTerminalAndDependencyResults(t *testing.T) {
	for _, tc := range []struct {
		name     string
		snapshot middleware.AccountSnapshot
		err      error
		want     int
	}{
		{"disabled", middleware.AccountSnapshot{Status: 0}, nil, http.StatusForbidden},
		{"deleted", middleware.AccountSnapshot{Status: 1, Deleted: true}, nil, http.StatusUnauthorized},
		{"db_unavailable", middleware.AccountSnapshot{}, errors.New("unavailable"), http.StatusServiceUnavailable},
		{"active_without_session_projection", middleware.AccountSnapshot{Status: 1}, nil, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, w, manager, rdb := newJWTTestEnv(t, time.Minute)
			require.NoError(t, rdb.Close(), "PAT authentication must not depend on AT/RT Redis state")
			bearerRequest(c, "zpat_synthetic-only")
			middleware.JWT(manager, rdb, &fakePATGetter{userID: 7, userName: "bob", patID: 42},
				middleware.WithAccountSecurity(&fakeAccountSnapshotter{snap: tc.snapshot, err: tc.err}),
			)(c)
			require.Equal(t, tc.want, w.Code)
			require.Equal(t, tc.want != http.StatusOK, c.IsAborted())
		})
	}
}

func TestJWT_DBSession_ProjectionReadiness(t *testing.T) {
	for _, viaCookie := range []bool{false, true} {
		for _, tc := range []struct {
			name         string
			dbProjection int64
			redisEpoch   string
			wrongType    bool
			disabled     bool
			want         int
		}{
			{"ready", 3, "3", false, false, 200},
			{"db_missing", 0, "3", false, false, 503},
			{"db_lag", 2, "3", false, false, 503},
			{"db_ahead", 4, "3", false, false, 503},
			{"redis_missing", 3, "", false, false, 503},
			{"redis_malformed", 3, "broken", false, false, 503},
			{"redis_noncanonical", 3, "03", false, false, 503},
			{"redis_zero", 3, "0", false, false, 503},
			{"redis_negative", 3, "-1", false, false, 503},
			{"redis_overflow", 3, "9223372036854775808", false, false, 503},
			{"redis_lag", 3, "2", false, false, 503},
			{"redis_ahead", 3, "4", false, false, 503},
			{"redis_wrongtype", 3, "", true, false, 503},
			{"redis_disabled_stale", 3, "3", false, true, 503},
		} {
			t.Run(fmt.Sprintf("cookie_%t/%s", viaCookie, tc.name), func(t *testing.T) {
				c, w, manager, rdb := newJWTTestEnv(t, time.Minute)
				token, _, err := manager.GenerateAccessTokenWithSessionEpoch(7, "bob", false, 0, 3)
				require.NoError(t, err)
				if tc.redisEpoch != "" {
					require.NoError(t, rdb.Set(c, "user:session_epoch:db1:7", tc.redisEpoch, 0).Err())
				}
				if tc.wrongType {
					require.NoError(t, rdb.LPush(c, "user:session_epoch:db1:7", "synthetic").Err())
				}
				if tc.disabled {
					require.NoError(t, rdb.Set(c, "user:session_disabled:db1:7", "1", 0).Err())
				}
				chain := middleware.JWT(manager, rdb, nil, middleware.WithAccountSecurity(&fakeAccountSnapshotter{
					snap: middleware.AccountSnapshot{Status: 1, SessionEpoch: 3, ProjectionEpoch: tc.dbProjection},
				}))
				if viaCookie {
					c.Request.AddCookie(&http.Cookie{Name: middleware.FileSessionCookieName, Value: token})
					chain = middleware.FileAuth(chain)
				} else {
					bearerRequest(c, token)
				}
				chain(c)
				require.Equal(t, tc.want, w.Code)
				require.Equal(t, tc.want != http.StatusOK, c.IsAborted())
			})
		}
	}
}

func TestJWT_DBSession_IgnoresLegacyProjectionKeys(t *testing.T) {
	c, _, manager, rdb := newJWTTestEnv(t, time.Minute)
	token, _, err := manager.GenerateAccessTokenWithSessionEpoch(7, "bob", false, 0, 3)
	require.NoError(t, err)
	require.NoError(t, rdb.Set(c, "user:session_epoch:db1:7", "3", 0).Err())
	require.NoError(t, rdb.Set(c, "user:disabled:7", "1", 0).Err())
	require.NoError(t, rdb.LPush(c, "user:pw_epoch:7", "broken").Err())
	require.NoError(t, rdb.Set(c, "user:at_min_iat:7", "9223372036854775807", 0).Err())
	bearerRequest(c, token)
	middleware.JWT(manager, rdb, nil, middleware.WithAccountSecurity(&fakeAccountSnapshotter{
		snap: middleware.AccountSnapshot{Status: 1, SessionEpoch: 3, ProjectionEpoch: 3},
	}))(c)
	require.False(t, c.IsAborted())
	require.Equal(t, int64(3), c.GetInt64("session_epoch"))
}

func TestJWT_DBSession_RejectsMalformedSignedEpoch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		schema string
		epoch  any
	}{
		{"unknown_schema", "future", "3"},
		{"missing_schema", "", "3"},
		{"missing_epoch", "db_epoch_v1", nil},
		{"numeric_epoch", "db_epoch_v1", 3},
		{"leading_zero", "db_epoch_v1", "03"},
		{"plus_sign", "db_epoch_v1", "+3"},
		{"space", "db_epoch_v1", " 3"},
		{"zero", "db_epoch_v1", "0"},
		{"negative", "db_epoch_v1", "-3"},
		{"overflow", "db_epoch_v1", "9223372036854775808"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, w, manager, rdb := newJWTTestEnv(t, time.Minute)
			token, err := standardjwt.NewWithClaims(standardjwt.SigningMethodHS256, standardjwt.MapClaims{
				"uid": "7", "typ": "access", "jti": "synthetic", "exp": time.Now().Add(time.Minute).Unix(),
				"session_schema": tc.schema, "session_epoch": tc.epoch,
			}).SignedString([]byte("jwt-middleware-test-secret-0123456789"))
			require.NoError(t, err)
			bearerRequest(c, token)
			middleware.JWT(manager, rdb, nil, middleware.WithAccountSecurity(&fakeAccountSnapshotter{
				snap: middleware.AccountSnapshot{Status: 1, SessionEpoch: 3, ProjectionEpoch: 3},
			}))(c)
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.Contains(t, w.Body.String(), `"code":20003`)
		})
	}
}

func TestJWT_DBSession_PreservesExactLargeEpochs(t *testing.T) {
	for _, raw := range []string{"9007199254740993", "9223372036854775807"} {
		t.Run(raw, func(t *testing.T) {
			c, _, manager, rdb := newJWTTestEnv(t, time.Minute)
			epoch, err := strconv.ParseInt(raw, 10, 64)
			require.NoError(t, err)
			token, _, err := manager.GenerateAccessTokenWithSessionEpoch(7, "bob", false, 0, epoch)
			require.NoError(t, err)
			require.NoError(t, rdb.Set(c, "user:session_epoch:db1:7", raw, 0).Err())
			bearerRequest(c, token)
			middleware.JWT(manager, rdb, nil, middleware.WithAccountSecurity(&fakeAccountSnapshotter{
				snap: middleware.AccountSnapshot{Status: 1, SessionEpoch: epoch, ProjectionEpoch: epoch},
			}))(c)
			require.False(t, c.IsAborted())
			require.Equal(t, epoch, c.GetInt64("session_epoch"))
		})
	}
}

func TestJWT_DBSession_RedisUnavailableAndBlacklisted(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		t.Run(fmt.Sprintf("revoked_%t", revoked), func(t *testing.T) {
			c, w, manager, rdb := newJWTTestEnv(t, time.Minute)
			token, jti, err := manager.GenerateAccessTokenWithSessionEpoch(7, "bob", false, 0, 3)
			require.NoError(t, err)
			want := http.StatusServiceUnavailable
			if revoked {
				require.NoError(t, rdb.Set(c, "blacklist:at:"+jti, "1", 0).Err())
				want = http.StatusUnauthorized
			} else {
				require.NoError(t, rdb.Close())
			}
			bearerRequest(c, token)
			middleware.JWT(manager, rdb, nil, middleware.WithAccountSecurity(&fakeAccountSnapshotter{
				snap: middleware.AccountSnapshot{Status: 1, SessionEpoch: 3, ProjectionEpoch: 3},
			}))(c)
			require.Equal(t, want, w.Code)
			require.True(t, c.IsAborted())
		})
	}
}

func TestJWT_ExplicitNilAccountSnapshotSourceFailsClosed(t *testing.T) {
	for _, pat := range []bool{false, true} {
		t.Run(fmt.Sprintf("pat_%t", pat), func(t *testing.T) {
			c, w, manager, rdb := newJWTTestEnv(t, time.Minute)
			token, _, err := manager.GenerateAccessTokenWithSessionEpoch(7, "bob", false, 0, 3)
			require.NoError(t, err)
			if pat {
				token = "zpat_synthetic-only"
			}
			bearerRequest(c, token)
			middleware.JWT(manager, rdb, &fakePATGetter{userID: 7, userName: "bob", patID: 42}, middleware.WithAccountSecurity(nil))(c)
			require.Equal(t, http.StatusServiceUnavailable, w.Code)
			require.True(t, c.IsAborted())
		})
	}
}
