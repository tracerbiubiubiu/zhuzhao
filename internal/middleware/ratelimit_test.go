// API 限流单测（miniredis + 令牌桶 Lua）：放行/超限 429+Retry-After/
// 路由覆盖/关闭直通/fail-close 503。
package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

func rlSetup(t *testing.T, cfg config.RateLimitConfig) (*gin.Engine, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(RequestID(), RateLimit(rdb, cfg))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	return r, mr
}

func TestRateLimit_WithinBurstPasses(t *testing.T) {
	r, _ := rlSetup(t, config.RateLimitConfig{
		Enabled: true,
		Default: config.RateLimitRule{RPS: 5, Burst: 5},
	})
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))
		require.Equal(t, http.StatusOK, w.Code, "第 %d 次应放行", i+1)
	}
}

func TestRateLimit_OverLimit429WithRetryAfter(t *testing.T) {
	r, _ := rlSetup(t, config.RateLimitConfig{
		Enabled: true,
		Default: config.RateLimitRule{RPS: 2, Burst: 2},
	})
	var retryAfter string
	var env map[string]any
	w := httptest.NewRecorder()
	for i := 0; i < 3; i++ {
		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))
		retryAfter = w.Header().Get("Retry-After")
		_ = json.Unmarshal(w.Body.Bytes(), &env)
	}
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.EqualValues(t, 10007, env["code"], "429 走通用段 ErrTooManyReqs")
	require.NotEmpty(t, retryAfter, "429 须带 Retry-After")
}

func TestRateLimit_RouteOverride(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled: true,
		Default: config.RateLimitRule{RPS: 100, Burst: 100},
		Routes: map[string]config.RateLimitRule{
			"/ping": {RPS: 1, Burst: 1}, // 精确覆盖：/ping 更严
		},
	}
	r, _ := rlSetup(t, cfg)
	ok, limited := 0, 0
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))
		if w.Code == http.StatusOK {
			ok++
		} else {
			limited++
		}
	}
	require.Equal(t, 1, ok, "覆盖规则 rps=1/burst=1：仅首次放行")
	require.Equal(t, 2, limited)
}

func TestRateLimit_DisabledPassesThrough(t *testing.T) {
	r, _ := rlSetup(t, config.RateLimitConfig{
		Enabled: false,
		Default: config.RateLimitRule{RPS: 1, Burst: 1},
	})
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))
		require.Equal(t, http.StatusOK, w.Code)
	}
}

func TestRateLimit_RedisDownFailsClosed(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	mr.Close() // 模拟 Redis 不可用
	t.Cleanup(func() { _ = rdb.Close() })

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(RequestID(), RateLimit(rdb, config.RateLimitConfig{
		Enabled: true,
		Default: config.RateLimitRule{RPS: 10, Burst: 10},
	}))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))
	require.Equal(t, http.StatusServiceUnavailable, w.Code, "fail-close 503（对齐 Phase 1 登录限流口径）")
}

// C1 回归：路由覆盖桶与默认桶必须隔离——默认流量耗尽不得影响路由覆盖规则的预算
// （122b9c6 的「桶隔离」修复实为 no-op：bucket 两分支恒 default，路由桶被默认流量稀释）。
func TestRateLimit_RouteBucketIsolation(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(RequestID(), RateLimit(rdb, config.RateLimitConfig{
		Enabled: true,
		Default: config.RateLimitRule{RPS: 100, Burst: 2},
		Routes: map[string]config.RateLimitRule{
			"/ping": {RPS: 1, Burst: 2},
		},
	}))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/other", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// 先打满默认桶（burst=2，经 /other）
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/other", nil))
		require.Equal(t, http.StatusOK, w.Code)
	}
	// 路由桶独立计费：/ping 的预算不受默认桶影响
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))
		require.Equal(t, http.StatusOK, w.Code, "第 %d 次应放行（路由桶独立于默认桶）", i+1)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))
	require.Equal(t, http.StatusTooManyRequests, w.Code, "第 3 次按路由覆盖规则 429")
}
