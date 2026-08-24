package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
)

type stubRoleFetcher struct {
	roles map[int64][]string
}

// 按 F-7 修复后的 RoleFetcher 接口签名（ctx 传播）
func (s *stubRoleFetcher) GetRoleCodesByUserID(ctx context.Context, userID int64) ([]string, error) {
	return s.roles[userID], nil
}

func TestCasbinAuth_NoRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enforcer := newTestEnforcer(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	c.Set("userID", int64(1))

	middleware.CasbinAuth(enforcer, &stubRoleFetcher{roles: map[int64][]string{}}, nil)(c)
	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCasbinAuth_ViewerDeniedOnUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enforcer := newTestEnforcer(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	c.Set("userID", int64(1))

	middleware.CasbinAuth(enforcer, &stubRoleFetcher{roles: map[int64][]string{1: {"viewer"}}}, nil)(c)
	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCasbinAuth_AdminAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enforcer := newTestEnforcer(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	c.Set("userID", int64(1))

	middleware.CasbinAuth(enforcer, &stubRoleFetcher{roles: map[int64][]string{1: {"admin"}}}, nil)(c)
	assert.False(t, c.IsAborted())
}

// F-1 联动场景：零角色用户即使在自服务路由上也被拒（70003）
func TestCasbinAuth_ZeroRolesSelfServiceDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enforcer := newTestEnforcer(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user/menus", nil)
	c.Set("userID", int64(1))
	// 引用导出常量，防止字符串字面量与实现脱钩
	c.Set(middleware.SelfServiceContextKey, true)

	middleware.CasbinAuth(enforcer, &stubRoleFetcher{roles: map[int64][]string{}}, nil)(c)
	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCasbinAuth_ViewerSelfServiceAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enforcer := newTestEnforcer(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user/menus", nil)
	c.Set("userID", int64(1))
	c.Set(middleware.SelfServiceContextKey, true)

	middleware.CasbinAuth(enforcer, &stubRoleFetcher{roles: map[int64][]string{1: {"viewer"}}}, nil)(c)
	assert.False(t, c.IsAborted())
}

// §2.3 特殊场景：真实执行链验证 SelfService → CasbinAuth 的中间件顺序语义。
// 与 router.go 中 selfService.Use(middleware.SelfService(), middleware.CasbinAuth(...))
// 的注册模式一致：若顺序被调换，viewer 将在自服务路由上被拒（403）。
func TestSelfServiceOrder_ViewerAllowedOnRealChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enforcer := newTestEnforcer(t)
	fetcher := &stubRoleFetcher{roles: map[int64][]string{1: {"viewer"}}}

	// 正向：SelfService 先于 CasbinAuth（router.go 的注册模式），viewer 放行
	r2 := gin.New()
	r2.Use(func(c *gin.Context) { c.Set("userID", int64(1)); c.Next() })
	g2 := r2.Group("")
	g2.Use(
		middleware.SelfService(),
		middleware.CasbinAuth(enforcer, fetcher, nil),
	)
	g2.GET("/api/v1/user/menus", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"menus": []any{}})
	})

	w2 := httptest.NewRecorder()
	r2.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/api/v1/user/menus", nil))
	assert.Equal(t, http.StatusOK, w2.Code, "viewer 经真实中间件链访问自服务路由应放行")

	// 反向对照：无 SelfService 标签的同一 fetcher，viewer 访问业务路由被拒
	r3 := gin.New()
	r3.Use(func(c *gin.Context) { c.Set("userID", int64(1)); c.Next() })
	g3 := r3.Group("")
	g3.Use(middleware.CasbinAuth(enforcer, fetcher, nil))
	g3.GET("/api/v1/users", func(c *gin.Context) { c.Status(http.StatusOK) })

	w3 := httptest.NewRecorder()
	r3.ServeHTTP(w3, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))
	assert.Equal(t, http.StatusForbidden, w3.Code, "viewer 访问业务路由（无标签）应被拒")
}

func newTestEnforcer(t *testing.T) *casbin.SyncedEnforcer {
	t.Helper()
	m, err := model.NewModelFromString(`
[request_definition]
r = sub, obj, act
[policy_definition]
p = sub, obj, act
[policy_effect]
e = some(where (p.eft == allow))
[matchers]
m = r.sub == "role::superadmin" || r.sub == "role::admin" || (r.sub == p.sub && (p.obj == "*" || keyMatch2(r.obj, p.obj)) && (r.act == p.act || p.act == "*"))
`)
	require.NoError(t, err)
	e, err := casbin.NewSyncedEnforcer(m)
	require.NoError(t, err)
	_, err = e.AddPolicy("role::admin", "*", "*")
	require.NoError(t, err)
	return e
}
