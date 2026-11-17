//go:build integration

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/taskrunner"
)

func TestTaskDefinitionRepair_CreateCannotBorrowExistingCronApproval(t *testing.T) {
	pool := maintenanceTestPool(t)
	id := maintenanceTestUser(t, pool)
	ctx := WithTaskPrincipal(context.Background(), id, "reviewed-maintainer")
	definition := taskDefinition{JobID: "approved-existing-job", ActionID: maintenanceAction, TriggerType: "cron", CronSpec: "@daily", Params: json.RawMessage(`{}`), Enabled: true, CreatedBy: "reviewed-maintainer"}
	raw, err := json.Marshal(definition)
	require.NoError(t, err)
	digest, err := MaintenanceDefinitionDigest(raw)
	require.NoError(t, err)
	var posts atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		// TR Create allocates its own job ID; body.job_id is not JobInput.
		_, _ = w.Write([]byte(`{"code":0,"data":{"job_id":"new-unreviewed-job"}}`))
	}))
	defer peer.Close()
	client := taskrunner.New(taskrunner.Config{BaseURL: peer.URL, SK: []byte("synthetic")})
	now := time.Now()
	policy, err := NewTaskActionPolicy(pool, config.MaintenanceConfig{MaintainerUserIDs: []int64{id}, FixedJobs: []config.MaintenanceFixedJobConfig{{JobID: definition.JobID, DefinitionSHA256: digest, ValidFrom: now.Add(-time.Hour).Format(time.RFC3339), ValidUntil: now.Add(time.Hour).Format(time.RFC3339), Reason: "reviewed existing definition only"}}}, 180, client)
	require.NoError(t, err)
	_, err = NewTaskrunnerService(client, nil, policy).CreateJob(ctx, raw, "ignored-body-actor")
	require.Error(t, err, "a new server-allocated cron ID cannot inherit another definition's fixed-job approval")
	require.Zero(t, posts.Load(), "reject before creating an unreviewed enabled cron")
}
