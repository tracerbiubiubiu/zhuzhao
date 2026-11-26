package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/taskrunner"
)

func TestB3TaskVisibilityUsesStoredAction(t *testing.T) {
	for _, tt := range []struct {
		name, taskID, storedID, action string
		wantErr                        *errcode.Error
	}{
		{"ordinary", "standard-task", "standard-task", "ordinary.action", nil},
		{"ordinary_prefixed", "b3_old", "b3_old", "ordinary.action", nil},
		{"report_prefixed", "b3_report", "b3_report", "ticket.evidence.report.generate", errcode.ErrNotFound},
		{"update_unprefixed", "update", "update", "activelist.document.update", errcode.ErrNotFound},
		{"read_unprefixed", "read", "read", "activelist.document.get", errcode.ErrNotFound},
		{"missing_action", "missing", "missing", "", errcode.ErrServiceUnavailable},
		{"different_identity", "requested", "different", "ordinary.action", errcode.ErrServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var mutations atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/v1/tasks/"+tt.taskID:
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]string{
						"task_id": tt.storedID, "action": tt.action, "params": "{}",
					}})
				case r.Method == http.MethodPost && (r.URL.Path == "/v1/tasks/cancel" || r.URL.Path == "/v1/tasks/retry"):
					mutations.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]bool{"ok": true}})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			svc := NewTaskrunnerService(taskrunner.New(taskrunner.Config{BaseURL: server.URL}), nil)
			svc.HideB3Runs()
			ctx := context.Background()
			data, getErr := svc.GetTask(ctx, tt.taskID)
			_, cancelErr := svc.CancelTask(ctx, tt.taskID, "")
			_, retryErr := svc.RetryTask(ctx, tt.taskID, "")
			if tt.wantErr != nil {
				require.ErrorIs(t, getErr, tt.wantErr)
				require.Empty(t, data)
				require.ErrorIs(t, cancelErr, tt.wantErr)
				require.ErrorIs(t, retryErr, tt.wantErr)
				require.Zero(t, mutations.Load())
				return
			}
			require.NoError(t, getErr)
			require.Contains(t, string(data), tt.taskID)
			require.NoError(t, cancelErr)
			require.NoError(t, retryErr)
			require.EqualValues(t, 2, mutations.Load())
		})
	}
}
