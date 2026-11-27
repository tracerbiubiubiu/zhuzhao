package router_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/router"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskdispatch"
)

const auditFixtureMarker = "synthetic-risk-audit-marker"

type riskAuditStatsFixture struct{}

func (riskAuditStatsFixture) ReadDepartmentStats(context.Context, int64) (riskdispatch.DepartmentStatsResult, error) {
	return riskdispatch.DepartmentStatsResult{
		Current: []riskdispatch.DeptTodoRow{{DeptID: 10, DeptName: auditFixtureMarker, OpenTickets: 1, OpenRisks: 2}},
	}, nil
}

func TestRiskRoutes_AuditOnlyMetadataOnSuccessAndDenial(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auditCapture := &captureAuditLogger{}
	var accessLogs bytes.Buffer
	m := jwtpkg.NewManager(jwtpkg.Config{
		Secret:    "synthetic-risk-audit-test-identity",
		AccessTTL: time.Minute,
	})
	r := router.New(router.Deps{
		JWTManager:          m,
		AccountSnapshots:    activeRouterAccountSnapshots{},
		Enforcer:            newRouterTestEnforcer(t),
		RedisClient:         startStubRedis(t),
		Logger:              slog.New(slog.NewJSONHandler(&accessLogs, nil)),
		RoleFetcher:         &stubRoleFetcher{roles: map[int64][]string{1: {"viewer"}, 2: {"admin"}}},
		AuditService:        auditCapture,
		RiskDispatchHandler: handler.NewRiskDispatchHandler(nil, riskAuditStatsFixture{}, nil),
	})

	// Exercise every registered risk route behind JWT and Casbin, including the group root.
	// The source ingest route is separately mounted behind AK/SK.
	for _, route := range r.Routes() {
		risk := route.Path == "/api/v1/risk-items" || strings.HasPrefix(route.Path, "/api/v1/risk-items/")
		suppression := route.Path == "/api/v1/risk-suppressions" || strings.HasPrefix(route.Path, "/api/v1/risk-suppressions/")
		if (!risk && !suppression) || route.Path == "/api/v1/risk-items/report" {
			continue
		}
		t.Run("denied/"+route.Method+route.Path, func(t *testing.T) {
			auditCapture.entries = nil
			accessLogs.Reset()
			path := strings.ReplaceAll(route.Path, ":id", auditFixtureMarker) + "?q=" + auditFixtureMarker
			w := sendRiskAuditFixtureRequest(t, r, m, 1, route.Method, path,
				`{"title":"`+auditFixtureMarker+`","assessment_note":"`+auditFixtureMarker+`","reason":"`+auditFixtureMarker+`"}`)
			require.Equal(t, http.StatusForbidden, w.Code)
			require.Len(t, auditCapture.entries, 1)
			assertRiskAuditMetadata(t, auditCapture.entries[0], w, 1, route.Method, route.Path)
			assert.Contains(t, accessLogs.String(), `"msg":"access"`)
			assert.NotContains(t, accessLogs.String(), auditFixtureMarker)
		})
	}

	t.Run("successful current dispatch statistics", func(t *testing.T) {
		auditCapture.entries = nil
		accessLogs.Reset()
		const path = "/api/v1/risk-items/dispatch-statistics"
		w := sendRiskAuditFixtureRequest(t, r, m, 2, http.MethodGet, path+"?q="+auditFixtureMarker, "")
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), auditFixtureMarker, "authorized client receives the actual response")
		require.Len(t, auditCapture.entries, 1)
		assertRiskAuditMetadata(t, auditCapture.entries[0], w, 2, http.MethodGet, path)
		assert.Contains(t, accessLogs.String(), `"msg":"access"`)
		assert.NotContains(t, accessLogs.String(), auditFixtureMarker)
	})

	t.Run("ordinary route still retains audit previews", func(t *testing.T) {
		auditCapture.entries = nil
		w := sendRiskAuditFixtureRequest(t, r, m, 1, http.MethodPost, "/api/v1/users",
			`{"name":"synthetic-ordinary-audit-marker"}`)
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Len(t, auditCapture.entries, 1)
		assert.Contains(t, auditCapture.entries[0].RequestBody, "synthetic-ordinary-audit-marker")
		assert.NotEmpty(t, auditCapture.entries[0].ResponseSummary)
	})
}

func sendRiskAuditFixtureRequest(t *testing.T, r *gin.Engine, m *jwtpkg.Manager, actor int64, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	token, _, err := m.GenerateAccessTokenWithSessionEpoch(actor, "synthetic-risk-audit-user", false, 0, 3)
	require.NoError(t, err)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func assertRiskAuditMetadata(t *testing.T, entry middleware.AuditLogEntry, w *httptest.ResponseRecorder, actor int64, method, route string) {
	t.Helper()
	assert.Equal(t, actor, entry.UserID)
	assert.Equal(t, method, entry.Method)
	assert.Equal(t, route, entry.Path)
	assert.Equal(t, w.Code, entry.StatusCode)
	assert.NotEmpty(t, entry.RequestID)
	assert.Equal(t, w.Header().Get("X-Request-ID"), entry.RequestID)
	assert.Empty(t, entry.RequestBody)
	assert.Empty(t, entry.ResponseSummary)
}
