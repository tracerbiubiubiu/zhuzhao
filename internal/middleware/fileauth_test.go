package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// B5-2-2（A.11 Cookie 双凭证——形态 B 确认制）：纯单测——
// cookie 属性契约（Path/HttpOnly/SameSite/MaxAge）、桥接次序（header 优先、
// 并存时 cookie 静默忽略）、清除语义。端到端（真 JWT 链+登出吊销联动）随验收面。

// newCookieRouter：FileAuth 先挂、探针后挂（探针记录桥接后的 Authorization——
// 模拟 JWT 链入口所见）。
func newCookieRouter(capture *string, fa gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(fa)
	r.Use(func(c *gin.Context) {
		*capture = c.GetHeader("Authorization")
		c.Set("userID", int64(1))
		c.Next()
	})
	r.GET("/probe", func(c *gin.Context) { c.Status(200) })
	return r
}

func TestFileSessionCookie_Attributes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	SetFileSessionCookie(c, "tok-abc", 900, false)
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	ck := cookies[0]
	assert.Equal(t, FileSessionCookieName, ck.Name)
	assert.Equal(t, "tok-abc", ck.Value)
	assert.Equal(t, "/api/v1/files", ck.Path, "Path 限定（A.11 负向③锚点）")
	assert.True(t, ck.HttpOnly, "HttpOnly 防 JS 读取")
	assert.Equal(t, http.SameSiteLaxMode, ck.SameSite)
	assert.Equal(t, 900, ck.MaxAge, "TTL 对齐 access token 有效期")
	assert.False(t, ck.Secure, "默认 false——内网 HTTP 兼容（A.11「生产 Secure」经配置开启）")
}

// S5 评审 P2-4（2026-10-20）：A.11「生产 Secure」配置化——secure=true 属性在。
func TestSetFileSessionCookie_SecureFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	SetFileSessionCookie(c, "tok-abc", 900, true)
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.True(t, cookies[0].Secure, "secure=true 须带 Secure 属性（HTTPS 网关生产形态）")
}

func TestFileSessionCookie_Clear(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	ClearFileSessionCookie(c)
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, FileSessionCookieName, cookies[0].Name)
	assert.Equal(t, -1, cookies[0].MaxAge, "过期清除（MaxAge<0）")
}

func TestFileAuth_BridgeOrder(t *testing.T) {
	t.Run("无 header——cookie 桥接为 Bearer", func(t *testing.T) {
		var seen string
		r := newCookieRouter(&seen, FileAuth(func(c *gin.Context) { c.Next() }))

		req := httptest.NewRequest(http.MethodGet, "/probe", nil)
		req.AddCookie(&http.Cookie{Name: FileSessionCookieName, Value: "tok-xyz"})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, 200, w.Code)
		assert.Equal(t, "Bearer tok-xyz", seen)
	})

	t.Run("header 优先——并存时 cookie 静默忽略（hasMixedAuth 语义不变）", func(t *testing.T) {
		var seen string
		r := newCookieRouter(&seen, FileAuth(func(c *gin.Context) { c.Next() }))

		req := httptest.NewRequest(http.MethodGet, "/probe", nil)
		req.Header.Set("Authorization", "Bearer real-header-tok")
		req.AddCookie(&http.Cookie{Name: FileSessionCookieName, Value: "tok-stale"})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, "Bearer real-header-tok", seen)
	})

	t.Run("无 header 无 cookie——透传（JWT 链自会 401）", func(t *testing.T) {
		var seen string
		r := newCookieRouter(&seen, FileAuth(func(c *gin.Context) { c.Next() }))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))
		assert.Equal(t, "", seen)
	})
}
