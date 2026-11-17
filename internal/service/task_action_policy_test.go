package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/taskrunner"
)

func TestMaintenanceActionUntrustedServiceCallerZeroMutation(t *testing.T) {
	for round := 0; round < 8; round++ {
		t.Run(fmt.Sprint(round), func(t *testing.T) {
			var mutations atomic.Int32
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_, _ = w.Write([]byte(`{"code":0,"data":{"job_id":"legacy-maintenance","task_id":"legacy-task","action_id":"audit_archive","action":"audit_archive","params":"{\"retention_days\":1}","enabled":true,"trigger_type":"manual"}}`))
					return
				}
				mutations.Add(1)
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"code":10001,"message":"synthetic downstream refuses fixture"}`))
			}))
			defer peer.Close()
			svc := NewTaskrunnerService(taskrunner.New(taskrunner.Config{BaseURL: peer.URL, AK: "test", SK: []byte("synthetic")}), nil)
			for _, entry := range []string{"submit", "create", "update", "trigger", "retry"} {
				t.Run(entry, func(t *testing.T) {
					mutations.Store(0)
					var err error
					switch entry {
					case "submit":
						_, err = svc.Submit(context.Background(), &TaskSubmitInput{Action: "audit_archive", Params: json.RawMessage(`{"retention_days":1}`)}, "system", "", "")
					case "create":
						_, err = svc.CreateJob(context.Background(), json.RawMessage(`{"action_id":"audit_archive","trigger_type":"manual","params":{"retention_days":1},"created_by":"superadmin"}`), "system")
					case "update":
						_, err = svc.UpdateJob(context.Background(), "legacy-maintenance", json.RawMessage(`{"job_id":"legacy-maintenance","enabled":true,"action_id":"ordinary","created_by":"superadmin"}`), "system")
					case "trigger":
						_, err = svc.Trigger(context.Background(), "legacy-maintenance", "system", "")
					case "retry":
						_, err = svc.RetryTask(context.Background(), "legacy-task", "system")
					}
					require.ErrorIs(t, err, errcode.ErrForbidden, "actor strings and body.created_by cannot manufacture maintenance authority")
					require.EqualValues(t, 0, mutations.Load(), "denial must precede every downstream mutation")
				})
			}
		})
	}
}

func TestMaintenanceConfigRejectsImplicitOrUnboundedAuthority(t *testing.T) {
	now := time.Now()
	for _, cfg := range []config.MaintenanceConfig{
		{MaintainerUserIDs: []int64{0}}, {MaintainerUserIDs: []int64{7, 7}},
		{MaintainerUserIDs: []int64{7}, Overrides: []config.MaintenanceOverrideConfig{{ID: "a", UserID: 8, RetentionDays: 5, ValidFrom: now.Add(-time.Hour).Format(time.RFC3339), ValidUntil: now.Add(time.Hour).Format(time.RFC3339), Reason: "reviewed"}}},
		{MaintainerUserIDs: []int64{7}, Overrides: []config.MaintenanceOverrideConfig{{ID: "a", UserID: 7, RetentionDays: 5}}},
		{FixedJobs: []config.MaintenanceFixedJobConfig{{JobID: "old", DefinitionSHA256: "not-a-digest"}}},
	} {
		_, err := NewTaskActionPolicy(nil, cfg, 180, nil)
		require.Error(t, err)
	}
}
