// 网关反代单测：注册表校验（fail-fast）/ 前缀剥离 / 身份断言头透传 /
// 出站签名可在上游侧验签通过（闭环到 M-A6 验签语义）/ 上游不可达 502 信封。
package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
)

// fakeAuthz 模拟 CasbinAuth：只断言「转发前执行」并把标记写进请求头供上游断言顺序。
func fakeAuthz(t *testing.T, ran *bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		*ran = true
		c.Request.Header.Set("X-Authz-Ran", "1")
		c.Next()
	}
}

// newUpstream 起一个真实上游：回显收到的 path / X-Operator / X-Request-ID，
// 并用共享密钥验签（等价 activelist M-A6 验签语义）。
func newUpstream(t *testing.T, sk string) (*httptest.Server, *[]string) {
	seen := &[]string{}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*seen = append(*seen, r.URL.Path+"|"+r.Header.Get("X-Operator")+"|"+r.Header.Get("X-Request-ID"))
		v := &aksk.Verifier{Keys: map[string][]byte{"zhuzhao": []byte(sk)}, MaxBodyBytes: 1 << 20}
		full, _ := io.ReadAll(io.MultiReader(strings.NewReader(string(body)), r.Body))
		if err := v.Verify(r, full); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"message":"success"}`))
	}))
	t.Cleanup(up.Close)
	return up, seen
}

// cnRecorder httptest 记录器 + CloseNotify 壳：gin 1.12 responseWriter 对底层
// 硬断言 CloseNotifier，而 ReverseProxy 正常流会调用（生产 net/http 原生支持，
// 仅 httptest 记录器缺失）——测试补壳，不影响生产路径。
type cnRecorder struct{ *httptest.ResponseRecorder }

func (cnRecorder) CloseNotify() <-chan bool { return make(chan bool) }

// serve 走一次引擎并回传记录器。
func serve(e *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	w := cnRecorder{httptest.NewRecorder()}
	e.ServeHTTP(w, req)
	return w.ResponseRecorder
}

func TestNew_FailFast(t *testing.T) {
	for name, ups := range map[string][]Upstream{
		"空上游":       {},
		"前缀非根":      {{Prefix: "/", Target: "http://x"}},
		"前缀无斜杠":     {{Prefix: "al", Target: "http://x"}},
		"前缀重复":      {{Prefix: "/al", Target: "http://x"}, {Prefix: "/al", Target: "http://y"}},
		"target 非法": {{Prefix: "/al", Target: "ftp://x"}},
	} {
		_, err := New(ups, "zhuzhao", "sk")
		require.Error(t, err, name)
	}
	_, err := New([]Upstream{{Prefix: "/al", Target: "http://x"}}, "", "sk")
	require.Error(t, err, "空 AK")
}

func TestMount_StripPrefixAndForwardHeaders(t *testing.T) {
	up, seen := newUpstream(t, "sk-gw")
	upURL := strings.TrimPrefix(up.URL, "http://")
	r, err := New([]Upstream{{Prefix: "/al", Target: "http://" + upURL, StripPrefix: true}}, "zhuzhao", "sk-gw")
	require.NoError(t, err)

	authzRan := false
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	authed := e.Group("")
	authed.Use(func(c *gin.Context) {
		c.Set("username", "verify-op")   // JWT username → X-Operator
		c.Set("request_id", "req-fixed") // RequestID → X-Request-ID
		c.Next()
	})
	authz := fakeAuthz(t, &authzRan)
	r.Mount(authed, authz)

	req := httptest.NewRequest(http.MethodGet, "/al/api/v1/admin/types?x=1", nil)
	req.Header.Set("X-Request-ID", "req-fixed")
	w := serve(e, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, authzRan, "authz 中间件须先于代理执行")
	require.Equal(t, []string{"/api/v1/admin/types|verify-op|req-fixed"}, *seen,
		"上游收到剥离前缀后的路径 + 透传的操作者/关联键，且 HMAC 验签通过")
}

func TestMount_NoStripKeepsFullPath(t *testing.T) {
	up, seen := newUpstream(t, "sk-gw")
	upURL := strings.TrimPrefix(up.URL, "http://")
	r, err := New([]Upstream{{Prefix: "/al", Target: "http://" + upURL}}, "zhuzhao", "sk-gw")
	require.NoError(t, err)

	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	r.Mount(e.Group(""))

	req := httptest.NewRequest(http.MethodGet, "/al/api/v1/x", nil)
	req.Header.Set("X-Request-ID", "req-2")
	aksk.Sign(req, nil, aksk.SignOptions{AK: "zhuzhao", SK: []byte("sk-gw"),
		RequestID: "req-2", Operator: "op2"})
	// 直接以网关形态重放（含签名）到无剥离链路
	req.Header.Set("X-Request-ID", "req-2")
	w := serve(e, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{"/al/api/v1/x|op2|req-2"}, *seen)
}

func TestMount_DisabledUpstreamReturns503(t *testing.T) {
	up, seen := newUpstream(t, "sk-gw")
	upURL := strings.TrimPrefix(up.URL, "http://")
	r, err := New([]Upstream{{Prefix: "/al", Target: "http://" + upURL, StripPrefix: true, Disabled: true}}, "zhuzhao", "sk-gw")
	require.NoError(t, err)
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	r.Mount(e.Group(""))

	req := httptest.NewRequest(http.MethodGet, "/al/api/v1/x", nil)
	req.Header.Set("X-Request-ID", "req-503")
	w := serve(e, req)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), `"code":10008`)
	require.Empty(t, *seen, "停用上游不应收到任何转发")
	_ = up
}

func TestMount_UpstreamDownMapsTo502Envelope(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := strings.TrimPrefix(dead.URL, "http://")
	dead.Close() // 先关再建代理 → 连接拒绝

	r, err := New([]Upstream{{Prefix: "/al", Target: "http://" + deadURL, StripPrefix: true}}, "zhuzhao", "sk")
	require.NoError(t, err)
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	r.Mount(e.Group(""))

	req := httptest.NewRequest(http.MethodGet, "/al/api/v1/x", nil)
	req.Header.Set("X-Request-ID", "req-502")
	w := serve(e, req)
	require.Equal(t, http.StatusBadGateway, w.Code)

	var env map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.EqualValues(t, 10008, env["code"])
	require.Equal(t, "req-502", env["request_id"])
}
