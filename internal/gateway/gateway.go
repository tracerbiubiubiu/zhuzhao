// Package gateway 网关反代（批次 B / E13 蓝图，ADR-003 D2）：前缀→上游注册表、
// ReverseProxy 透传、AK/SK 出站签名、身份断言头注入与错误映射。
//
// 路由级鉴权不在此包：挂载点（router authed 组）已带 JWT + 审计，CasbinAuth 由
// 调用方传入（§25.1——反代路由同样过 CasbinAuth，menu_apis keyMatch2 匹配 :param 模式）；
// 上游侧鉴权 = activelist M-A6 验签（本包出站请求经 utils aksk.Transport 签名，
// canonical 覆盖 X-Request-ID / X-Operator）。
package gateway

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
)

// Upstream 反代上游定义（config gateway.upstreams 项）。
type Upstream struct {
	Prefix      string `mapstructure:"prefix"`       // 网关暴露前缀（如 /al）：全注册表唯一、以 / 开头、非根
	Target      string `mapstructure:"target"`       // 上游基址（如 http://activelist:8080）
	StripPrefix bool   `mapstructure:"strip_prefix"` // true：转发时剥离 Prefix（/al/api/v1/x → 上游 /api/v1/x）
	Disabled    bool   `mapstructure:"disabled"`     // Restrict 资源开关（首版语义：config 总开关；用户级授权由 Casbin 承担——升级路径 per-user 授权表）
}

// proxyMount 构建完成的挂载项（prefix + 就绪的代理 handler）。
type proxyMount struct {
	prefix   string
	disabled bool
	handler  gin.HandlerFunc
}

// Registry 上游注册表（New 后不可变；Mount 可重复调用）。
type Registry struct {
	mounts []proxyMount
}

// New 构建注册表。fail-fast 校验：前缀格式/唯一性、上游 http(s)、AK/SK 非空。
func New(ups []Upstream, ak, sk string) (*Registry, error) {
	if len(ups) == 0 {
		return nil, fmt.Errorf("gateway: upstreams 为空")
	}
	if ak == "" || sk == "" {
		return nil, fmt.Errorf("gateway: ak/sk 未配置——出站签名必需（env GATEWAY_AK / GATEWAY_SK）")
	}
	seen := make(map[string]bool, len(ups))
	r := &Registry{}
	for i, u := range ups {
		if !strings.HasPrefix(u.Prefix, "/") || u.Prefix == "/" || strings.HasSuffix(u.Prefix, "/") {
			return nil, fmt.Errorf("gateway: upstreams[%d].prefix 须以单个 / 开头且非根：%q", i, u.Prefix)
		}
		if seen[u.Prefix] {
			return nil, fmt.Errorf("gateway: upstreams[%d].prefix 重复：%q", i, u.Prefix)
		}
		seen[u.Prefix] = true
		target, err := url.Parse(u.Target)
		if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
			return nil, fmt.Errorf("gateway: upstreams[%d].target 须为合法 http(s) URL：%q", i, u.Target)
		}
		m, err := buildMount(u, target, ak, []byte(sk))
		if err != nil {
			return nil, fmt.Errorf("gateway: upstream %q 构建失败: %w", u.Prefix, err)
		}
		r.mounts = append(r.mounts, m)
	}
	return r, nil
}

// buildMount 组装单上游代理：路径改写（SetURL + StripPrefix）、出站签名
// （aksk.Transport——Operator/RequestID 取转发头）、上游不可达 → 502+10008 信封。
func buildMount(u Upstream, target *url.URL, ak string, sk []byte) (proxyMount, error) {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()
			if u.StripPrefix {
				pr.Out.URL.Path = strings.TrimPrefix(pr.Out.URL.Path, u.Prefix)
				pr.Out.URL.RawPath = strings.TrimPrefix(pr.Out.URL.RawPath, u.Prefix)
			}
		},
		Transport: &aksk.Transport{
			Base:      http.DefaultTransport,
			AK:        ak,
			SK:        sk,
			Operator:  func(req *http.Request) string { return req.Header.Get("X-Operator") },
			RequestID: func(req *http.Request) string { return req.Header.Get("X-Request-ID") },
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, perr error) {
			slog.Warn("gateway: upstream unreachable",
				"prefix", u.Prefix, "target", u.Target, "err", perr)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(response.Response{
				Code:      10008, // 通用段 ErrServiceUnavailable：5xx 可重试语义
				Message:   "上游服务暂时不可用",
				RequestID: req.Header.Get("X-Request-ID"),
			})
		},
	}
	handler := func(c *gin.Context) {
		proxy.ServeHTTP(c.Writer, c.Request)
	}
	return proxyMount{prefix: u.Prefix, disabled: u.Disabled, handler: handler}, nil
}

// Mount 在给定路由组/引擎下挂载全部上游的通配反代路由（根级挂载传 *gin.Engine）。
// authzMiddlewares（如 CasbinAuth）先于 SetForwardHeaders/代理执行——
// 未授权请求在出站前即被拦截。
func (r *Registry) Mount(router gin.IRouter, authzMiddlewares ...gin.HandlerFunc) {
	for _, m := range r.mounts {
		g := router.Group(m.prefix)
		g.Use(authzMiddlewares...)
		if m.disabled {
			// Restrict 资源开关（首版）：停用 = 503（位于 Casbin 之后——未授权者
			// 仍 403，不向未授权者泄露停用状态；升级路径 = per-user 授权表）
			g.Use(func(c *gin.Context) {
				response.Fail(c, http.StatusServiceUnavailable, 10008, "上游资源维护中，暂停访问")
				c.Abort()
			})
		}
		g.Use(SetForwardHeaders())
		g.Any("/*rest", m.handler)
	}
}

// SetForwardHeaders 身份断言注入（转发前调用）：X-Operator = 网关认证身份
// （JWT username），X-Request-ID 沿用入站关联键——二者在出站侧经 AK/SK 签名覆盖，
// activelist M-A6 验签后用作操作者归因与跨查关联键。
func SetForwardHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		if u := c.GetString("username"); u != "" {
			c.Request.Header.Set("X-Operator", u)
		}
		if rid := c.GetString("request_id"); rid != "" {
			c.Request.Header.Set("X-Request-ID", rid)
		}
		c.Next()
	}
}
