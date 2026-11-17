package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/taskrunner"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestTaskJSONContractRepair_CaseAliasesCannotChangeAuthorizedDefinition(t *testing.T) {
	var posts atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"job_id":"synthetic-job"}}`))
	}))
	defer peer.Close()
	client := taskrunner.New(taskrunner.Config{BaseURL: peer.URL, SK: []byte("synthetic")})
	policy, err := service.NewTaskActionPolicy(nil, config.MaintenanceConfig{}, 180, client)
	require.NoError(t, err)
	registry := jobs.NewRegistry()
	registry.Register("audit_archive", jobs.HandlerFunc(func(context.Context, json.RawMessage) error { return nil }))
	registry.Register("ordinary_action", jobs.HandlerFunc(func(context.Context, json.RawMessage) error { return nil }))
	h := handler.NewTaskrunnerHandler(service.NewTaskrunnerService(client, nil, policy), "", registry)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", int64(123)); c.Set("username", "ordinary-actor"); c.Next() })
	r.POST("/jobs", h.CreateJob)
	for _, body := range []string{
		`{"action_id":"audit_archive","ACTION_ID":"","trigger_type":"manual","params":{}}`,
		`{"action_id":"ordinary_action","ACTION_ID":"audit_archive","trigger_type":"manual","params":{}}`,
		`{"action_id":"ordinary_action","trigger_type":"manual","params":{},"PARAMS":{"retention_days":1}}`,
	} {
		t.Run(body, func(t *testing.T) {
			before := posts.Load()
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			require.GreaterOrEqual(t, w.Code, 400, w.Body.String())
			require.Less(t, w.Code, 500, w.Body.String())
			require.Equal(t, before, posts.Load(), "invalid or unauthorized definitions must not reach the downstream mutation")
		})
	}
}
