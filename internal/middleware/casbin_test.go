package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
)

type stubRoleFetcher struct {
	roles map[int64][]string
}

func (s *stubRoleFetcher) GetRoleCodesByUserID(userID int64) ([]string, error) {
	return s.roles[userID], nil
}

func TestCasbinAuth_NoRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enforcer := newTestEnforcer(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	c.Set("userID", int64(1))

	middleware.CasbinAuth(enforcer, &stubRoleFetcher{roles: map[int64][]string{}})(c)
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

	middleware.CasbinAuth(enforcer, &stubRoleFetcher{roles: map[int64][]string{1: {"viewer"}}})(c)
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

	middleware.CasbinAuth(enforcer, &stubRoleFetcher{roles: map[int64][]string{1: {"admin"}}})(c)
	assert.False(t, c.IsAborted())
}

func TestCasbinAuth_ZeroRolesSelfServiceDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enforcer := newTestEnforcer(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user/menus", nil)
	c.Set("userID", int64(1))

	middleware.CasbinAuth(enforcer, &stubRoleFetcher{roles: map[int64][]string{}})(c)
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

	middleware.CasbinAuth(enforcer, &stubRoleFetcher{roles: map[int64][]string{1: {"viewer"}}})(c)
	assert.False(t, c.IsAborted())
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
