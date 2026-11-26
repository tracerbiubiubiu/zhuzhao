package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

func TestRiskFeedRequiresTheConfiguredServiceIdentity(t *testing.T) {
	engine := gin.New()
	cfg := config.RiskIngestConfig{Enabled: true, SourceRef: "synthetic-finding-source", AK: "synthetic-feed", SK: "test-fixture"}
	calls := 0
	mountRiskFeed(engine, cfg, func(ctx *gin.Context, sourceRef string) {
		calls++
		require.Equal(t, cfg.SourceRef, sourceRef)
		ctx.Status(http.StatusAccepted)
	})

	const path = "/api/v1/risk-items/report"
	const body = `{"source_ref":"synthetic-finding-source"}`
	send := func(accessKey string) int {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		if accessKey != "" {
			aksk.Sign(request, []byte(body), aksk.SignOptions{
				AK: accessKey, SK: []byte(cfg.SK), RequestID: "synthetic-test",
			})
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response.Code
	}
	require.Equal(t, http.StatusUnauthorized, send(""))
	require.Equal(t, http.StatusUnauthorized, send("another-service"))
	require.Zero(t, calls)
	require.Equal(t, http.StatusAccepted, send(cfg.AK))
	require.Equal(t, 1, calls)
}
