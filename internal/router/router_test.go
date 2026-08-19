package router_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/router"
)

// stubRoleFetcher 按 userID 返回固定角色
type stubRoleFetcher struct {
	roles map[int64][]string
}

func (s *stubRoleFetcher) GetRoleCodesByUserID(ctx context.Context, userID int64) ([]string, error) {
	return s.roles[userID], nil
}

// newRouterEngine 构造真实路由树：合法 JWT（stub Redis 放行 EXISTS）+ stub 角色查询 +
// 测试 enforcer。业务 handler 为零值——通过鉴权链的请求以 panic(→500) 落地，
// 因此用「非 403」判定放行、「403」判定拒绝，足以覆盖鉴权链行为。
func newRouterEngine(t *testing.T, roles map[int64][]string) (*gin.Engine, *jwtpkg.Manager) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	jwtManager := jwtpkg.NewManager(config.JWTConfig{
		Secret:    "router-test-secret-0123456789",
		AccessTTL: 30 * time.Minute,
	})

	deps := router.Deps{
		JWTManager:  jwtManager,
		Enforcer:    newRouterTestEnforcer(t),
		RedisClient: startStubRedis(t),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		RoleFetcher: &stubRoleFetcher{roles: roles},
	}
	return router.New(deps), jwtManager
}

func do(r *gin.Engine, m *jwtpkg.Manager, userID int64, method, path string) *httptest.ResponseRecorder {
	at, _, err := m.GenerateAccessToken(userID, fmt.Sprintf("user-%d", userID), false)
	if err != nil {
		panic(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+at)
	r.ServeHTTP(w, req)
	return w
}

// §2.3 核心场景：自服务路由上 SelfService 标签必须先于 CasbinAuth 生效。
// 若注册顺序被调换（CasbinAuth 在前），viewer 将看不到标签 → 403；
// 顺序正确时 viewer 放行 → 进入零值 handler panic → Recovery 兜底 500。
// 同时用真实 JWT 层（stub Redis 放行黑名单/disabled 检查）覆盖完整链路。
func TestSelfServiceChain_ViewerAllowed(t *testing.T) {
	r, m := newRouterEngine(t, map[int64][]string{1: {"viewer"}})

	w := do(r, m, 1, http.MethodGet, "/api/v1/user/menus")
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"viewer 访问自服务路由应通过鉴权链（500=零值 handler panic，属预期；403=SelfService 顺序被破坏）")
	assert.NotEqual(t, http.StatusUnauthorized, w.Code, "JWT 层不应拒绝（stub Redis 已放行）")
}

// 业务路由（无标签）：viewer 必须被 Casbin 拒绝
func TestBizChain_ViewerDenied(t *testing.T) {
	r, m := newRouterEngine(t, map[int64][]string{1: {"viewer"}})

	w := do(r, m, 1, http.MethodGet, "/api/v1/users")
	assert.Equal(t, http.StatusForbidden, w.Code, "viewer 访问业务路由应被 Casbin 拒绝")
}

// 零角色用户：自服务路由同样拒绝（70003 语义，自服务白名单不豁免零角色）
func TestSelfServiceChain_ZeroRolesDenied(t *testing.T) {
	r, m := newRouterEngine(t, map[int64][]string{1: {}})

	w := do(r, m, 1, http.MethodGet, "/api/v1/user/menus")
	assert.Equal(t, http.StatusForbidden, w.Code, "零角色用户即使在自服务路由也应被拒")
}

// admin 通配策略：业务路由放行（非 403）
func TestBizChain_AdminAllowed(t *testing.T) {
	r, m := newRouterEngine(t, map[int64][]string{1: {"admin"}})

	w := do(r, m, 1, http.MethodGet, "/api/v1/users")
	assert.NotEqual(t, http.StatusForbidden, w.Code, "admin 访问业务路由应通过 Casbin")
}

// 公开路由（无 Authorization）：login 不应被 JWT 层拒绝（401），
// 会直达零值 handler（500）；若返回 401 说明公开路由被误挂 JWT
func TestPublicRoute_NotBehindJWT(t *testing.T) {
	r, _ := newRouterEngine(t, map[int64][]string{1: {"viewer"}})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil))
	assert.NotEqual(t, http.StatusUnauthorized, w.Code, "login 是公开路由，不应经过 JWT 中间件")
}

func newRouterTestEnforcer(t *testing.T) *casbin.SyncedEnforcer {
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

// startStubRedis 启动最小 RESP2 协议 stub：EXISTS/DEL→0、GET→nil、SET→OK。
// 使 JWT 中间件的黑名单/disabled 检查全部放行，无需真实 Redis。
func startStubRedis(t *testing.T) *redis.Client {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleStubRedisConn(conn)
		}
	}()

	client := redis.NewClient(&redis.Options{
		Addr:     ln.Addr().String(),
		Protocol: 2,
		PoolSize: 2,
	})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func handleStubRedisConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" || line[0] != '*' {
			continue
		}
		n, _ := strconv.Atoi(line[1:])
		args := make([]string, 0, n)
		for i := 0; i < n; i++ {
			hdr, err := r.ReadString('\n')
			if err != nil {
				return
			}
			hdr = strings.TrimRight(hdr, "\r\n")
			if len(hdr) == 0 || hdr[0] != '$' {
				continue
			}
			length, _ := strconv.Atoi(hdr[1:])
			if length <= 0 {
				args = append(args, "")
				continue
			}
			buf := make([]byte, length+2)
			if _, err := io.ReadFull(r, buf); err != nil {
				return
			}
			args = append(args, string(buf[:length]))
		}
		if len(args) == 0 {
			continue
		}
		switch strings.ToUpper(args[0]) {
		case "EXISTS", "DEL":
			_, _ = fmt.Fprintf(conn, ":0\r\n")
		case "GET":
			_, _ = fmt.Fprintf(conn, "$-1\r\n")
		case "SET":
			_, _ = fmt.Fprintf(conn, "+OK\r\n")
		case "PING":
			_, _ = fmt.Fprintf(conn, "+PONG\r\n")
		default:
			_, _ = fmt.Fprintf(conn, "-ERR stub redis\r\n")
		}
	}
}
