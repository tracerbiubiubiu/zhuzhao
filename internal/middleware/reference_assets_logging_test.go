package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/pkg/referencequery"
)

func TestReferenceAssetsLogsContainOnlyRequestMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, referencequery.QueryPath},
		{http.MethodPost, referencequery.AdmissionPath},
		{http.MethodGet, config.ReferenceAssetsPage},
		{http.MethodGet, config.ReferenceAssetsPage + "/app.js"},
		{http.MethodGet, config.ReferenceAssetsPage + "/app.css"},
	} {
		t.Run(route.path, func(t *testing.T) {
			var logs bytes.Buffer
			sink := &nestedRedactionAuditCapture{}
			engine := gin.New()
			engine.Use(RequestID(), AccessLogger(slog.New(slog.NewJSONHandler(&logs, nil))), AuditLog(sink))
			engine.Handle(route.method, route.path, func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"items": []gin.H{{"name": "synthetic-asset", "owner_org_ref": "17"}}})
			})
			res := httptest.NewRecorder()
			req := httptest.NewRequest(route.method, route.path+"?cursor=synthetic-cursor&owner_org_refs=17",
				strings.NewReader(`{"request_ref":"synthetic-admission"}`))
			engine.ServeHTTP(res, req)
			require.Equal(t, http.StatusOK, res.Code)
			require.Contains(t, res.Body.String(), "synthetic-asset")
			require.Equal(t, route.path, sink.entry.Path)
			require.Equal(t, http.StatusOK, sink.entry.StatusCode)
			require.NotEmpty(t, sink.entry.RequestID)
			assert.Empty(t, sink.entry.RequestBody)
			assert.Empty(t, sink.entry.ResponseSummary)
			var access map[string]any
			require.NoError(t, json.Unmarshal(logs.Bytes(), &access))
			require.Equal(t, route.path, access["path"])
			require.Equal(t, sink.entry.RequestID, access["request_id"])
			assert.Empty(t, access["query"])
		})
	}
}
