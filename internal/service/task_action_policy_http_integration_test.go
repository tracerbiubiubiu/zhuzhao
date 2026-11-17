//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/taskrunner"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMaintenanceActionHandlerTrustedPrincipalAndRealArchiveEffect(t *testing.T) {
	ctx := context.Background()
	maintID := rbacTestUser(t, "http-maint-"+uniqueSuffix(), "HM"+uniqueSuffix(), nil)
	businessID := rbacTestUser(t, "http-biz-"+uniqueSuffix(), "HB"+uniqueSuffix(), nil)
	marker := "http-maint-" + uniqueSuffix()
	seedAuditRow(t, marker, time.Now().Add(-200*24*time.Hour))
	var posts atomic.Int32
	var policy *service.TaskActionPolicy
	archive := service.NewAuditArchiveJob(repository.NewAuditLogRepo(testPool), 180, 5000, t.TempDir(), 395, nil)
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		var body struct {
			TaskID      string          `json:"task_id"`
			Action      string          `json:"action"`
			Params      json.RawMessage `json:"params"`
			SubmittedBy string          `json:"submitted_by"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "trusted-http", body.SubmittedBy)
		execute, e := policy.AuthorizeCallback(ctx, body.TaskID, body.Action, body.Params)
		require.NoError(t, e)
		require.NoError(t, archive.Handle(execute, body.Params))
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"task_id": body.TaskID, "accepted": true}})
	}))
	defer peer.Close()
	client := taskrunner.New(taskrunner.Config{BaseURL: peer.URL, SK: []byte("synthetic")})
	var err error
	policy, err = service.NewTaskActionPolicy(testPool, config.MaintenanceConfig{MaintainerUserIDs: []int64{maintID}}, 180, client)
	require.NoError(t, err)
	h := handler.NewTaskrunnerHandler(service.NewTaskrunnerService(client, nil, policy), "", nil)
	call := func(id int64) *httptest.ResponseRecorder {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("userID", id); c.Set("username", "trusted-http"); c.Next() })
		r.POST("/tasks", h.Submit)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{"action":"audit_archive","params":{},"actor":"system","created_by":"superadmin"}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	require.Equal(t, http.StatusForbidden, call(businessID).Code)
	require.Zero(t, posts.Load())
	require.EqualValues(t, 1, countRows(t, `SELECT count(*) FROM audit_logs WHERE path=$1`, marker))
	require.Equal(t, http.StatusOK, call(maintID).Code)
	require.EqualValues(t, 1, posts.Load())
	require.Zero(t, countRows(t, `SELECT count(*) FROM audit_logs WHERE path=$1`, marker))
}
func TestMessageRuleCreatedByServerOwnedAndUpdatePreservesCreator(t *testing.T) {
	id := rbacTestUser(t, "rule-creator-"+uniqueSuffix(), "RC"+uniqueSuffix(), nil)
	modifier := rbacTestUser(t, "rule-modifier-"+uniqueSuffix(), "RM"+uniqueSuffix(), nil)
	svc := service.NewMessageAdminService(testPool)
	repo := repository.NewMessageRuleRepo(testPool)
	biz := fmt.Sprint("test-", time.Now().UnixNano())
	defer func() { _, _ = testPool.Exec(context.Background(), `DELETE FROM message_rules WHERE biz_type=$1`, biz) }()
	forged := modifier
	rule := &model.MessageRule{BizType: biz, EventAction: "ticket.created", TemplateCode: "ticket_created_notify", RecipientType: "creator", Enabled: true, CreatedBy: &forged}
	require.Error(t, svc.UpsertRule(context.Background(), rule), "body creator is not a service identity")
	require.NoError(t, svc.UpsertRule(service.WithTaskPrincipal(context.Background(), id, "original"), rule))
	saved, err := repo.GetByAction(context.Background(), biz, rule.EventAction)
	require.NoError(t, err)
	require.NotNil(t, saved.CreatedBy)
	require.Equal(t, id, *saved.CreatedBy)
	rule.Version = saved.Version
	rule.CreatedBy = &modifier
	rule.TemplateCode = "ticket_closed_notify"
	require.NoError(t, svc.UpsertRule(service.WithTaskPrincipal(context.Background(), modifier, "actual modifier"), rule))
	saved, err = repo.GetByAction(context.Background(), biz, rule.EventAction)
	require.NoError(t, err)
	require.Equal(t, id, *saved.CreatedBy)
	require.EqualValues(t, 2, saved.Version)
}
