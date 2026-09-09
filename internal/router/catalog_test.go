// BK-22 对账纯函数单测：双向缺口判定 / 豁免集 / 网关前缀边界。
// 真实路由 × 真实 menu_apis 的发现跑见 catalog_discovery_integration_test.go。
package router

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func testEngine() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/healthz", func(c *gin.Context) {}) // 探针：非 /api/v1，天然豁免
	v1 := r.Group("/api/v1")
	v1.POST("/auth/login", func(c *gin.Context) {}) // 公开端点豁免
	authed := v1.Group("")
	authed.GET("/user/menus", func(c *gin.Context) {}) // 自服务豁免
	authed.GET("/orgs/roles/list", func(c *gin.Context) {})
	authed.GET("/orgs/other", func(c *gin.Context) {}) // 委托前缀下、非豁免清单内
	biz := authed.Group("")
	biz.GET("/users", func(c *gin.Context) {})
	biz.POST("/users", func(c *gin.Context) {})
	biz.GET("/ghost-route", func(c *gin.Context) {}) // 有路由无绑定（裸奔）
	return r
}

func bound(pairs ...string) map[string]bool {
	m := map[string]bool{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]+" "+pairs[i+1]] = true
	}
	return m
}

func kinds(gaps []CatalogGap) map[string]bool {
	m := map[string]bool{}
	for _, g := range gaps {
		m[g.Kind+" "+g.Method+" "+g.Path] = true
	}
	return m
}

func TestAuditRouteCatalog_Bidirectional(t *testing.T) {
	boundSet := bound(
		"GET", "/api/v1/users",
		"POST", "/api/v1/users",
		"GET", "/api/v1/dead-route", // 有绑定无路由（死策略）
	)
	gaps := AuditRouteCatalog(testEngine().Routes(), boundSet, nil)
	k := kinds(gaps)

	require.False(t, k["missing_binding GET /api/v1/users"], "已绑定路由不报缺")
	require.True(t, k["missing_binding GET /api/v1/ghost-route"], "有路由无绑定=裸奔缺口")
	require.True(t, k["dead_binding GET /api/v1/dead-route"], "有绑定无路由=死策略缺口")
	require.False(t, k["missing_binding POST /api/v1/auth/login"], "公开端点豁免")
	require.False(t, k["missing_binding GET /api/v1/user/menus"], "自服务豁免")
	require.False(t, k["dead_binding GET /api/v1/user/menus"], "豁免路由的绑定不算死策略")
	require.False(t, k["missing_binding GET /api/v1/orgs/roles/list"], "orgDelegated 豁免")
	require.True(t, k["missing_binding GET /api/v1/orgs/other"], "orgDelegated 前缀下未入豁免清单的路由仍须对账")
	require.Equal(t, 3, len(gaps), "精确缺口数：ghost-route 裸奔 + dead-route 死策略 + orgs/other 裸奔")
}

func TestAuditRouteCatalog_GatewayPrefixExempt(t *testing.T) {
	// /al 前缀下的路由与绑定互相不可枚举（跨仓 activelist）——双向豁免
	boundSet := bound("GET", "/al/api/v1/data/:typeName")
	gaps := AuditRouteCatalog(testEngine().Routes(), boundSet, []string{"/al"})
	for _, g := range gaps {
		require.False(t, underGatewayPrefix(g.Path, []string{"/al"}), "网关前缀下不应出缺口：%v", g)
	}
}
