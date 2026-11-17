//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func callbackRepairRouter(t *testing.T, fn jobs.HandlerFunc) *gin.Engine {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `TRUNCATE job_submissions RESTART IDENTITY`)
	require.NoError(t, err)
	reg := jobs.NewRegistry()
	reg.Register("repair_action", fn)
	r := gin.New()
	r.POST("/internal/jobs/callback", handler.NewJobsHandler(service.NewJobsCallbackService(reg, repository.NewJobSubmissionRepo(testPool), nil)).Callback)
	return r
}

func callbackRepairHTTP(r *gin.Engine, taskID, params string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	body := `{"task_id":"` + taskID + `","action":"repair_action","params":` + params + `}`
	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/callback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestCallbackRepair_RunningIsNotComplete(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	r := callbackRepairRouter(t, func(ctx context.Context, _ json.RawMessage) error {
		if calls.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- callbackRepairHTTP(r, "repair-running", `{}`) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("first callback did not enter the action")
	}
	second := callbackRepairHTTP(r, "repair-running", `{}`)
	close(release)
	require.Equal(t, http.StatusOK, (<-first).Code)
	require.Equal(t, http.StatusServiceUnavailable, second.Code, second.Body.String())
	require.Equal(t, "1", second.Header().Get("Retry-After"))
	require.Contains(t, second.Body.String(), `"code":10008`)
	require.Equal(t, int32(1), calls.Load())
	third := callbackRepairHTTP(r, "repair-running", `{}`)
	require.Equal(t, http.StatusOK, third.Code, third.Body.String())
	require.Equal(t, int32(1), calls.Load(), "only a proven succeeded duplicate receives success without another effect")
}

func TestCallbackRepair_ExistingRunningIsNotComplete(t *testing.T) {
	var calls atomic.Int32
	r := callbackRepairRouter(t, func(context.Context, json.RawMessage) error { calls.Add(1); return nil })
	_, claimed, err := repository.NewJobSubmissionRepo(testPool).ClaimCallbackRow(context.Background(), "repair-existing-running", "repair_action", "synthetic", "", `{}`)
	require.NoError(t, err)
	require.True(t, claimed)
	w := callbackRepairHTTP(r, "repair-existing-running", `{}`)
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	require.Equal(t, "1", w.Header().Get("Retry-After"))
	require.Zero(t, calls.Load())
}

func TestCallbackRepair_MaintenanceWithoutAdmissionPolicyDenied(t *testing.T) {
	_, err := testPool.Exec(context.Background(), `TRUNCATE job_submissions RESTART IDENTITY`)
	require.NoError(t, err)
	var calls atomic.Int32
	reg := jobs.NewRegistry()
	reg.Register("audit_archive", jobs.HandlerFunc(func(context.Context, json.RawMessage) error { calls.Add(1); return nil }))
	r := gin.New()
	r.POST("/internal/jobs/callback", handler.NewJobsHandler(service.NewJobsCallbackService(reg, repository.NewJobSubmissionRepo(testPool), nil)).Callback)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/callback", strings.NewReader(`{"task_id":"repair-untrusted-maintenance","action":"audit_archive","params":{},"actor":"system"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Zero(t, calls.Load())
	var count int
	require.NoError(t, testPool.QueryRow(context.Background(), `SELECT count(*) FROM job_submissions`).Scan(&count))
	require.Zero(t, count, "missing maintenance admission must be rejected before claiming a row")
}

func TestCallbackRepair_LongRunningCannotReclaimActiveHandler(t *testing.T) {
	entered, overlap, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	r := callbackRepairRouter(t, func(context.Context, json.RawMessage) error {
		switch calls.Add(1) {
		case 1:
			close(entered)
		case 2:
			close(overlap)
		}
		<-release
		return nil
	})
	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- callbackRepairHTTP(r, "repair-long-running", `{}`) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("first callback did not enter the action")
	}
	// Move the database's stale-claim boundary while the first real handler is
	// held at a barrier. This exercises the same reclaim branch without waiting
	// ten minutes or relying on scheduler timing.
	_, err := testPool.Exec(context.Background(), `UPDATE job_submissions SET claimed_at=NOW()-INTERVAL '11 minutes' WHERE task_id='repair-long-running'`)
	require.NoError(t, err)
	second := make(chan *httptest.ResponseRecorder, 1)
	go func() { second <- callbackRepairHTTP(r, "repair-long-running", `{}`) }()
	var response *httptest.ResponseRecorder
	select {
	case response = <-second:
	case <-overlap:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("duplicate callback did not return or reach the controlled action barrier")
	}
	close(release)
	<-first
	if response == nil {
		response = <-second
	}
	require.Equal(t, int32(1), calls.Load(), "a long-running live handler cannot be reclaimed by a second callback")
	require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
}

func TestCallbackRepair_UnconfirmedFinishIsNotSuccess(t *testing.T) {
	for _, mode := range []string{"ledger_failure", "success_fence_miss", "failure_fence_miss"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			r := callbackRepairRouter(t, func(ctx context.Context, _ json.RawMessage) error {
				calls.Add(1)
				if mode != "ledger_failure" {
					_, err := testPool.Exec(ctx, `UPDATE job_submissions SET claimed_at=claimed_at + INTERVAL '1 second' WHERE task_id=$1`, mode)
					if err != nil {
						return err
					}
				}
				if mode == "failure_fence_miss" {
					return errors.New("synthetic action failure")
				}
				return nil
			})
			if mode == "ledger_failure" {
				_, err := testPool.Exec(context.Background(), `ALTER TABLE job_submissions ADD CONSTRAINT repair_reject_success CHECK (status <> 'succeeded')`)
				require.NoError(t, err)
				defer func() {
					_, err := testPool.Exec(context.Background(), `ALTER TABLE job_submissions DROP CONSTRAINT repair_reject_success`)
					require.NoError(t, err)
				}()
			}
			w := callbackRepairHTTP(r, mode, `{}`)
			require.GreaterOrEqual(t, w.Code, 500, "unconfirmed ledger/fence cannot acknowledge completion: %s", w.Body.String())
			var status string
			require.NoError(t, testPool.QueryRow(context.Background(), `SELECT status FROM job_submissions WHERE task_id=$1`, mode).Scan(&status))
			require.Equal(t, "running", status)
			require.Equal(t, int32(1), calls.Load())
			again := callbackRepairHTTP(r, mode, `{}`)
			require.Equal(t, http.StatusServiceUnavailable, again.Code, again.Body.String())
			require.Equal(t, int32(1), calls.Load(), "an unexpired claim may not repeat the action")
		})
	}
}

func TestCallbackRepair_ExactSnapshotAndInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name, snapshot, params, taskID string
		status                         int
	}{
		{"adjacent_big_integer", `{"n":9007199254740992}`, `{"n":9007199254740993}`, "repair-bigint", http.StatusConflict},
		{"duplicate_key", `{}`, `{"n":1,"n":2}`, "repair-duplicate", http.StatusBadRequest},
		{"task_id_width", `{}`, `{}`, strings.Repeat("x", 65), http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			r := callbackRepairRouter(t, func(context.Context, json.RawMessage) error { calls.Add(1); return nil })
			if tc.name == "adjacent_big_integer" {
				_, err := repository.NewJobSubmissionRepo(testPool).RecordSubmit(context.Background(), "repair_action", tc.taskID, "synthetic", "", tc.snapshot)
				require.NoError(t, err)
			}
			w := callbackRepairHTTP(r, tc.taskID, tc.params)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.Zero(t, calls.Load())
			var count int
			query := `SELECT count(*) FROM job_submissions`
			if tc.name == "adjacent_big_integer" {
				query += ` WHERE status <> 'submitted'`
			}
			require.NoError(t, testPool.QueryRow(context.Background(), query).Scan(&count))
			require.Zero(t, count, "rejection must not create or advance a claim")
		})
	}
}
