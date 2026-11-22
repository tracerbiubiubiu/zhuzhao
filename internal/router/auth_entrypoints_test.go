package router_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/gateway"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/router"
)

type entrypointAccountSnapshots struct {
	snapshot middleware.AccountSnapshot
	err      error
}

func (s entrypointAccountSnapshots) AccountSnapshot(context.Context, int64) (middleware.AccountSnapshot, error) {
	return s.snapshot, s.err
}

// Removing DB security from any independently mounted auth chain must fail this
// test, including cookie bridging and gateway routes outside /api/v1.
func TestAuthEntrypoints_MissingDatabaseFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)
	gw, err := gateway.New([]gateway.Upstream{{Prefix: "/synthetic", Target: upstream.URL}}, "synthetic-ak", "synthetic-sk")
	require.NoError(t, err)
	manager := jwtpkg.NewManager(jwtpkg.Config{Secret: "router-synthetic-test-secret", AccessTTL: time.Minute})
	engine := router.New(router.Deps{
		JWTManager: manager, RedisClient: startStubRedis(t), Enforcer: newRouterTestEnforcer(t),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		RoleFetcher: &stubRoleFetcher{roles: map[int64][]string{7: {"admin"}}}, Gateway: gw,
	})
	token, _, err := manager.GenerateAccessTokenWithSessionEpoch(7, "synthetic", false, 0, 3)
	require.NoError(t, err)
	for _, tc := range []struct {
		name, path string
		cookie     bool
	}{
		{"ordinary", "/api/v1/users", false},
		{"file_bearer", "/api/v1/files/synthetic.png", false},
		{"file_cookie", "/api/v1/files/synthetic.png", true},
		{"export", "/api/v1/tickets/export", false},
		{"gateway", "/synthetic/api/v1/items", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: middleware.FileSessionCookieName, Value: token})
			} else {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			require.Equal(t, http.StatusServiceUnavailable, w.Code)
		})
	}
}

func TestAuthEntrypoints_UseCurrentAccountAndSignedEpoch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)
	gw, err := gateway.New([]gateway.Upstream{{Prefix: "/synthetic", Target: upstream.URL}}, "synthetic-ak", "synthetic-sk")
	require.NoError(t, err)
	manager := jwtpkg.NewManager(jwtpkg.Config{Secret: "router-synthetic-test-secret", AccessTTL: time.Minute})
	for _, tc := range []struct {
		name     string
		snapshot middleware.AccountSnapshot
		err      error
		legacy   bool
		want     int
		code     string
	}{
		{"disabled", middleware.AccountSnapshot{Status: 0, SessionEpoch: 4}, nil, false, 403, `"code":30003`},
		{"deleted", middleware.AccountSnapshot{Status: 1, Deleted: true}, nil, false, 401, `"code":20003`},
		{"db_unavailable", middleware.AccountSnapshot{}, errors.New("synthetic unavailable"), false, 503, ""},
		{"legacy", middleware.AccountSnapshot{Status: 1, SessionEpoch: 3, ProjectionEpoch: 3}, nil, true, 401, `"code":20003`},
		{"mismatch", middleware.AccountSnapshot{Status: 1, SessionEpoch: 4, ProjectionEpoch: 4}, nil, false, 401, `"code":20003`},
		{"projection_pending", middleware.AccountSnapshot{Status: 1, SessionEpoch: 3, ProjectionEpoch: 2}, nil, false, 503, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := router.New(router.Deps{
				JWTManager: manager, RedisClient: startStubRedis(t), Enforcer: newRouterTestEnforcer(t),
				Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
				RoleFetcher: &stubRoleFetcher{roles: map[int64][]string{7: {"admin"}}}, Gateway: gw,
				AccountSnapshots: entrypointAccountSnapshots{snapshot: tc.snapshot, err: tc.err},
			})
			var token string
			var err error
			if tc.legacy {
				token, _, err = manager.GenerateAccessToken(7, "synthetic", false)
			} else {
				token, _, err = manager.GenerateAccessTokenWithSessionEpoch(7, "synthetic", false, 0, 3)
			}
			require.NoError(t, err)
			for _, path := range []string{"/api/v1/users", "/api/v1/files/synthetic.png", "/api/v1/tickets/export", "/synthetic/api/v1/items"} {
				t.Run(path, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodGet, path, nil)
					if path == "/api/v1/files/synthetic.png" {
						req.AddCookie(&http.Cookie{Name: middleware.FileSessionCookieName, Value: token})
					} else {
						req.Header.Set("Authorization", "Bearer "+token)
					}
					w := httptest.NewRecorder()
					engine.ServeHTTP(w, req)
					require.Equal(t, tc.want, w.Code)
					if tc.code != "" {
						require.Contains(t, w.Body.String(), tc.code)
					}
				})
			}
		})
	}
}
