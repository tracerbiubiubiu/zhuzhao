package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jwt"
)

// newJWTTestEnv 构造 JWT 中间件测试环境（miniredis：黑名单/disabled 检查全放行）
func newJWTTestEnv(t *testing.T, accessTTL time.Duration) (*gin.Context, *httptest.ResponseRecorder, *jwtpkg.Manager, *goredis.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	manager := jwtpkg.NewManager(config.JWTConfig{
		Secret:    "jwt-middleware-test-secret-0123456789",
		AccessTTL: accessTTL,
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	return c, w, manager, rdb
}

func bearerRequest(c *gin.Context, token string) {
	c.Request.Header.Set("Authorization", "Bearer "+token)
}

// B2-1 守护：过期 AT → 401 + code 20002（客户端可静默 refresh）。
// 修复前过期/无效统一 20003，客户端无法区分「该刷新」还是「需跳登录页」。
func TestJWT_ExpiredToken_Returns20002(t *testing.T) {
	// 负 TTL：签发即过期
	c, w, manager, rdb := newJWTTestEnv(t, -time.Minute)
	at, _, err := manager.GenerateAccessToken(1, "user-1", false)
	require.NoError(t, err)
	bearerRequest(c, at)

	middleware.JWT(manager, rdb)(c)

	assert.True(t, c.IsAborted(), "过期 token 必须中断请求链")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), `"code":20002`, "过期应返回 20002 而非 20003")
}

// B2-1 守护：签名篡改（错误密钥）→ 401 + code 20003
func TestJWT_InvalidSignature_Returns20003(t *testing.T) {
	c, w, manager, rdb := newJWTTestEnv(t, 30*time.Minute)
	at, _, err := manager.GenerateAccessToken(1, "user-1", false)
	require.NoError(t, err)
	bearerRequest(c, at)

	// 用另一密钥的 manager 解析（等效签名篡改）
	forgedManager := jwtpkg.NewManager(config.JWTConfig{
		Secret:    "another-secret-key-0123456789abcdef",
		AccessTTL: 30 * time.Minute,
	})
	middleware.JWT(forgedManager, rdb)(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), `"code":20003`, "签名无效应返回 20003")
}

// B2-1 守护：RT 冒充 AT → 401 + 20003（typ 校验失败属「无效」而非「过期」）
func TestJWT_RefreshTokenAsAccess_Returns20003(t *testing.T) {
	c, w, manager, rdb := newJWTTestEnv(t, 30*time.Minute)
	_, rt, err := manager.GenerateRefreshToken(1, "dev-1", 168*time.Hour)
	require.NoError(t, err)
	bearerRequest(c, rt)

	middleware.JWT(manager, rdb)(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), `"code":20003`)
}

// 有效 AT + Redis 放行：不中断，claims 注入 context
func TestJWT_ValidToken_Passes(t *testing.T) {
	c, w, manager, rdb := newJWTTestEnv(t, 30*time.Minute)
	at, _, err := manager.GenerateAccessToken(42, "user-42", false)
	require.NoError(t, err)
	bearerRequest(c, at)

	middleware.JWT(manager, rdb)(c)

	assert.False(t, c.IsAborted(), "有效 token 不应中断")
	assert.NotEqual(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, int64(42), c.GetInt64("userID"))
}

// 黑名单命中（登出后的 AT）→ 401 + 20003
func TestJWT_BlacklistedToken_Returns20003(t *testing.T) {
	c, w, manager, rdb := newJWTTestEnv(t, 30*time.Minute)
	at, _, err := manager.GenerateAccessToken(1, "user-1", false)
	require.NoError(t, err)
	bearerRequest(c, at)

	// 模拟登出拉黑：写 jti 黑名单键
	claims, err := manager.ParseAccessToken(at)
	require.NoError(t, err)
	require.NoError(t, rdb.Set(c, "blacklist:at:"+claims.JTI, "1", time.Minute).Err())

	middleware.JWT(manager, rdb)(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), `"code":20003`)
}

// 无 Authorization 头 → 401（10002 通用未授权）
func TestJWT_MissingHeader_Rejected(t *testing.T) {
	c, w, manager, rdb := newJWTTestEnv(t, 30*time.Minute)
	middleware.JWT(manager, rdb)(c)
	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), `"code":10002`)
}
