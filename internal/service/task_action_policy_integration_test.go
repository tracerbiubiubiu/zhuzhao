//go:build integration

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/taskrunner"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func maintenanceTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _, err := testutil.SetupPostgresShared()
	require.NoError(t, err)
	return pool
}
func maintenanceTestUser(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	name := fmt.Sprint("maint-", time.Now().UnixNano())
	var id int64
	require.NoError(t, pool.QueryRow(context.Background(), `INSERT INTO users(username,employee_no,password,status)VALUES($1,$1,'synthetic-unused',1)RETURNING id`, name).Scan(&id))
	return id
}
func TestMaintenancePolicyCurrentEligibilityAndControlledRetention(t *testing.T) {
	pool := maintenanceTestPool(t)
	id := maintenanceTestUser(t, pool)
	ctx := WithTaskPrincipal(context.Background(), id, "dedicated maintainer")
	now := time.Now()
	cfg := config.MaintenanceConfig{MaintainerUserIDs: []int64{id}, Overrides: []config.MaintenanceOverrideConfig{{ID: "approved-short", UserID: id, RetentionDays: 5, ValidFrom: now.Add(-time.Hour).Format(time.RFC3339), ValidUntil: now.Add(time.Hour).Format(time.RFC3339), Reason: "synthetic restore drill approval"}}}
	p, err := NewTaskActionPolicy(pool, cfg, 180, nil)
	require.NoError(t, err)
	for _, raw := range []string{`{}`, `{"retention_days":0}`, `{"retention_days":180}`} {
		require.NoError(t, p.authorizeAction(ctx, maintenanceAction, json.RawMessage(raw)))
	}
	for _, raw := range []string{`{"retention_days":-1}`, `{"retention_days":null}`, `{"retention_days":"5"}`, `{"retention_days":5,"retention_days":180}`, `{"retention_days":5,"reason":"self approved"}`, `[]`} {
		require.ErrorIs(t, p.authorizeAction(ctx, maintenanceAction, json.RawMessage(raw)), errcode.ErrInvalidParams)
	}
	for _, raw := range []string{`{"retention_days":5}`, `{"retention_days":6,"approval_id":"approved-short"}`, `{"retention_days":5,"approval_id":"forged"}`} {
		require.ErrorIs(t, p.authorizeAction(ctx, maintenanceAction, json.RawMessage(raw)), errcode.ErrForbidden)
	}
	body := json.RawMessage(`{"retention_days":5,"approval_id":"approved-short"}`)
	taskID := fmt.Sprint("approved-", time.Now().UnixNano())
	require.NoError(t, p.prepareTask(ctx, taskID, maintenanceAction, body))
	executeCtx, err := p.AuthorizeCallback(context.Background(), taskID, maintenanceAction, body)
	require.NoError(t, err)
	marker := taskID
	_, err = pool.Exec(ctx, `INSERT INTO audit_logs(username,method,path,status_code,duration,created_at)VALUES('maint','GET',$1,200,1,NOW()-INTERVAL '10 days')`, marker)
	require.NoError(t, err)
	job := NewAuditArchiveJob(repository.NewAuditLogRepo(pool), 180, 5000, t.TempDir(), 395, nil)
	require.ErrorIs(t, job.Handle(context.Background(), body), jobs.ErrAbort)
	require.NoError(t, job.Handle(executeCtx, body))
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*)FROM audit_logs WHERE path=$1`, marker).Scan(&count))
	require.Zero(t, count)
	// Restart with withdrawn independent grant denies queued old admissions. L1
	// button/role state is intentionally unrelated to this explicit grant.
	restarted, err := NewTaskActionPolicy(pool, config.MaintenanceConfig{}, 180, nil)
	require.NoError(t, err)
	_, err = restarted.AuthorizeCallback(ctx, taskID, maintenanceAction, body)
	require.ErrorIs(t, err, errcode.ErrForbidden)
	_, err = p.AuthorizeCallback(ctx, "legacy-without-fact", maintenanceAction, json.RawMessage(`{}`))
	require.ErrorIs(t, err, errcode.ErrForbidden)
	_, err = pool.Exec(ctx, `UPDATE users SET status=0 WHERE id=$1`, id)
	require.NoError(t, err)
	_, err = p.AuthorizeCallback(ctx, taskID, maintenanceAction, body)
	require.ErrorIs(t, err, errcode.ErrForbidden)
	require.ErrorIs(t, job.Handle(executeCtx, body), errcode.ErrForbidden, "previous authorization context cannot bypass final fresh user state")
}

func TestMaintenanceFinalUserRevokeSerializesWithEffectEightRounds(t *testing.T) {
	pool := maintenanceTestPool(t)
	ctx := context.Background()
	for round := 0; round < 8; round++ {
		t.Run(fmt.Sprint(round), func(t *testing.T) {
			id := maintenanceTestUser(t, pool)
			p, err := NewTaskActionPolicy(pool, config.MaintenanceConfig{MaintainerUserIDs: []int64{id}}, 180, nil)
			require.NoError(t, err)
			taskID := fmt.Sprint("revoke-", time.Now().UnixNano())
			require.NoError(t, p.prepareTask(WithTaskPrincipal(ctx, id, "dedicated"), taskID, maintenanceAction, json.RawMessage(`{}`)))
			authorized, err := p.AuthorizeCallback(ctx, taskID, maintenanceAction, json.RawMessage(`{}`))
			require.NoError(t, err)
			grant := authorized.Value(maintenanceExecutionKey{}).(maintenanceExecution)
			// A revocation already holding the user row wins before any effect.
			tx, err := pool.Begin(ctx)
			require.NoError(t, err)
			_, err = tx.Exec(ctx, `UPDATE users SET status=0 WHERE id=$1`, id)
			require.NoError(t, err)
			entered := make(chan struct{})
			done := make(chan error, 1)
			go func() { done <- grant.execute(ctx, func(context.Context, pgx.Tx) error { close(entered); return nil }) }()
			select {
			case <-entered:
				_ = tx.Rollback(ctx)
				<-done
				t.Fatal("effect entered while committed current eligibility was still unproved")
			case <-time.After(35 * time.Millisecond):
			}
			require.NoError(t, tx.Commit(ctx))
			require.ErrorIs(t, <-done, errcode.ErrForbidden)
			// Conversely, an authorized effect's SHARE lock prevents status revocation
			// from committing until that local transaction has finished.
			_, err = pool.Exec(ctx, `UPDATE users SET status=1 WHERE id=$1`, id)
			require.NoError(t, err)
			entered2 := make(chan struct{})
			release := make(chan struct{})
			done2 := make(chan error, 1)
			go func() {
				done2 <- grant.execute(ctx, func(context.Context, pgx.Tx) error { close(entered2); <-release; return nil })
			}()
			<-entered2
			revokeDone := make(chan error, 1)
			go func() { _, e := pool.Exec(ctx, `UPDATE users SET status=0 WHERE id=$1`, id); revokeDone <- e }()
			select {
			case e := <-revokeDone:
				close(release)
				<-done2
				t.Fatalf("revocation committed during protected effect: %v", e)
			case <-time.After(35 * time.Millisecond):
			}
			close(release)
			require.NoError(t, <-done2)
			require.NoError(t, <-revokeDone)
		})
	}
}

func TestMaintenancePrewrittenFactPrecedesImmediateCallbackAndConditionalTrigger(t *testing.T) {
	pool := maintenanceTestPool(t)
	id := maintenanceTestUser(t, pool)
	ctx := WithTaskPrincipal(context.Background(), id, "trusted maintainer")
	var p *TaskActionPolicy
	var posts atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"code":0,"data":{"job_id":"reviewed-manual","action_id":"audit_archive","trigger_type":"manual","params":{},"enabled":true}}`))
			return
		}
		posts.Add(1)
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		var taskID string
		require.NoError(t, json.Unmarshal(body["task_id"], &taskID))
		require.NotEmpty(t, taskID)
		_, e := p.AuthorizeCallback(context.Background(), taskID, maintenanceAction, json.RawMessage(`{}`))
		require.NoError(t, e, "fact must exist even before TR returns acceptance")
		if strings.Contains(r.URL.Path, "trigger") {
			require.JSONEq(t, `{}`, string(body["expected_params"]))
			require.JSONEq(t, `"audit_archive"`, string(body["expected_action"]))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"task_id": taskID, "accepted": true, "action": "audit_archive", "params": map[string]any{}}})
	}))
	defer peer.Close()
	client := taskrunner.New(taskrunner.Config{BaseURL: peer.URL, SK: []byte("synthetic")})
	var err error
	p, err = NewTaskActionPolicy(pool, config.MaintenanceConfig{MaintainerUserIDs: []int64{id}}, 180, client)
	require.NoError(t, err)
	svc := NewTaskrunnerService(client, nil, p)
	_, err = svc.Submit(ctx, &TaskSubmitInput{Action: maintenanceAction, Params: json.RawMessage(`{}`)}, "forged-body-actor", "", "")
	require.NoError(t, err)
	_, err = svc.Trigger(ctx, "reviewed-manual", "system", "")
	require.NoError(t, err)
	require.EqualValues(t, 2, posts.Load())
}

func TestMaintenanceFixedCronRequiresReviewedDefinitionAndFreshEnqueue(t *testing.T) {
	pool := maintenanceTestPool(t)
	now := time.Now().UTC()
	jobID := "fixed-reviewed"
	taskID := fmt.Sprint("cron-", now.UnixNano())
	params := json.RawMessage(`{}`)
	definition := taskDefinition{JobID: jobID, ActionID: maintenanceAction, TriggerType: "cron", CronSpec: "@daily", Params: params, Enabled: true, CreatedBy: "service:zhuzhao"}
	jobRaw, _ := json.Marshal(definition)
	digest, err := MaintenanceDefinitionDigest(jobRaw)
	require.NoError(t, err)
	var requestID string
	enqueued := now
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/jobs/") {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": json.RawMessage(jobRaw)})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"task_id": taskID, "job_id": jobID, "action": "audit_archive", "params": "{}", "request_id": requestID, "submitted_by": "", "enqueued_at": enqueued}})
	}))
	defer peer.Close()
	client := taskrunner.New(taskrunner.Config{BaseURL: peer.URL, SK: []byte("synthetic")})
	cfg := config.MaintenanceConfig{FixedJobs: []config.MaintenanceFixedJobConfig{{JobID: jobID, DefinitionSHA256: digest, ValidFrom: now.Add(-time.Minute).Format(time.RFC3339Nano), ValidUntil: now.Add(time.Hour).Format(time.RFC3339Nano), Reason: "reviewed synthetic scheduled maintenance"}}}
	p, err := NewTaskActionPolicy(pool, cfg, 180, client)
	require.NoError(t, err)
	_, err = p.AuthorizeCallback(context.Background(), taskID, maintenanceAction, params)
	require.NoError(t, err)
	requestID = "manual-http-request"
	_, err = p.AuthorizeCallback(context.Background(), taskID, maintenanceAction, params)
	require.ErrorIs(t, err, errcode.ErrForbidden, "a signed manual TR request is not a fixed scheduler grant")
	requestID = ""
	enqueued = now.Add(-time.Hour)
	_, err = p.AuthorizeCallback(context.Background(), taskID, maintenanceAction, params)
	require.ErrorIs(t, err, errcode.ErrForbidden, "old tasks cannot acquire a new approval retroactively")
	enqueued = now
	definition.Params = json.RawMessage(`{"retention_days":1}`)
	jobRaw, _ = json.Marshal(definition)
	_, err = p.AuthorizeCallback(context.Background(), taskID, maintenanceAction, params)
	require.ErrorIs(t, err, errcode.ErrForbidden, "changed current definition invalidates fixed job binding")
}

func TestMaintenanceDefinitionUsesOriginalActionAndServerOwnedCreator(t *testing.T) {
	pool := maintenanceTestPool(t)
	id := maintenanceTestUser(t, pool)
	ctx := WithTaskPrincipal(context.Background(), id, "trusted maintainer")
	var sent map[string]json.RawMessage
	var posts int
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"code":0,"data":{"job_id":"old-manual","action_id":"audit_archive","trigger_type":"manual","params":{},"enabled":true,"created_by":"original"}}`))
			return
		}
		posts++
		sent = nil
		require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
		_, _ = w.Write([]byte(`{"code":0,"data":{"job_id":"result"}}`))
	}))
	defer peer.Close()
	client := taskrunner.New(taskrunner.Config{BaseURL: peer.URL, SK: []byte("synthetic")})
	p, err := NewTaskActionPolicy(pool, config.MaintenanceConfig{MaintainerUserIDs: []int64{id}}, 180, client)
	require.NoError(t, err)
	svc := NewTaskrunnerService(client, nil, p)
	_, err = svc.CreateJob(ctx, json.RawMessage(`{"action_id":"audit_archive","trigger_type":"manual","enabled":true,"params":{},"created_by":"forged"}`), "system")
	require.NoError(t, err)
	require.JSONEq(t, `"trusted maintainer"`, string(sent["created_by"]))
	_, err = svc.UpdateJob(ctx, "old-manual", json.RawMessage(`{"job_id":"old-manual","action_id":"ordinary","trigger_type":"cron","created_by":"forged","enabled":false,"description":null}`), "system")
	require.NoError(t, err)
	require.NotContains(t, sent, "action_id")
	require.NotContains(t, sent, "trigger_type")
	require.NotContains(t, sent, "created_by")
	require.NotContains(t, sent, "description")
	require.JSONEq(t, `false`, string(sent["enabled"]))
	_, err = svc.UpdateJob(ctx, "old-manual", json.RawMessage(`{"job_id":"old-manual","action_id":"ordinary","params":{"retention_days":1}}`), "system")
	require.ErrorIs(t, err, errcode.ErrForbidden)
	require.Equal(t, 2, posts, "forged new action cannot hide original maintenance params")
	_, err = svc.UpdateJob(ctx, "old-manual", json.RawMessage(`{"job_id":"old-manual","params":null}`), "system")
	require.ErrorIs(t, err, errcode.ErrInvalidParams)
	require.Equal(t, 2, posts)
	_, err = svc.CreateJob(ctx, json.RawMessage(`{"action_id":"audit_archive","trigger_type":"cron","enabled":null,"params":{}}`), "system")
	require.ErrorIs(t, err, errcode.ErrForbidden, "null uses TR's default enabled=true and requires a fixed-job binding")
	require.Equal(t, 2, posts)
}
