//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

type cronAuditJob func(context.Context) error

func (f cronAuditJob) Handle(ctx context.Context, _ json.RawMessage) error { return f(ctx) }

func cronAuditPool(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	cfg := testPool.Config()
	cfg.MaxConns, cfg.MinConns = 1, 0
	name := "cron_audit_" + uniqueSuffix()
	cfg.ConnConfig.RuntimeParams["application_name"] = name
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool, name
}

func cronAuditSessions(t *testing.T, name string) int {
	t.Helper()
	var n int
	require.NoError(t, testPool.QueryRow(context.Background(), `SELECT count(*) FROM pg_stat_activity WHERE application_name=$1`, name).Scan(&n))
	return n
}

func TestCronLockPool_IndependentInstancesStillExclusive(t *testing.T) {
	pool, name := cronAuditPool(t)
	require.NoError(t, pool.Ping(context.Background()))
	var running, peak, calls atomic.Int32
	job := cronAuditJob(func(ctx context.Context) error {
		n := running.Add(1)
		defer running.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		calls.Add(1)
		// Holding the lock session must leave the sole business connection usable.
		if _, err := pool.Exec(ctx, `SELECT 1`); err != nil {
			return err
		}
		select {
		case <-time.After(30 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{}, 2)
	a := service.NewCronScheduler(pool, name, job, 5*time.Millisecond, nil)
	b := service.NewCronScheduler(pool, name, job, 5*time.Millisecond, nil)
	go func() { a.Run(ctx); done <- struct{}{} }()
	go func() { b.Run(ctx); done <- struct{}{} }()
	maxSessions := 0
	deadline := time.Now().Add(180 * time.Millisecond)
	for time.Now().Before(deadline) {
		n := cronAuditSessions(t, name)
		if n > maxSessions {
			maxSessions = n
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	<-done
	require.Greater(t, calls.Load(), int32(0))
	require.Equal(t, int32(1), peak.Load(), "same-name schedulers must remain mutually exclusive")
	require.LessOrEqual(t, maxSessions, 3, "one business connection plus at most two active schedulers")
	require.Eventually(t, func() bool { return cronAuditSessions(t, name) == 1 }, time.Second, 10*time.Millisecond, "dedicated sessions must close after Run stops")
	pool.Close()
	require.Eventually(t, func() bool { return cronAuditSessions(t, name) == 0 }, time.Second, 10*time.Millisecond)
	t.Logf("two scheduler connection peak=%d; business budget=1; after stop/close sessions=0; handler peak=%d", maxSessions, peak.Load())
}

func TestCronLockPool_FailurePanicCancellationReleaseSessions(t *testing.T) {
	for _, kind := range []string{"failure", "panic", "cancel", "unlock_failure"} {
		t.Run(kind, func(t *testing.T) {
			pool, name := cronAuditPool(t)
			entered := make(chan struct{}, 1)
			job := cronAuditJob(func(ctx context.Context) error {
				select {
				case entered <- struct{}{}:
				default:
				}
				switch kind {
				case "panic":
					panic("audit panic")
				case "cancel":
					<-ctx.Done()
					return ctx.Err()
				case "unlock_failure":
					// Terminate only this test's dedicated session, not the pool or
					// any shared PostgreSQL session. The pool query has its own PID.
					_, err := pool.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name=$1 AND pid<>pg_backend_pid()`, name)
					return err
				default:
					return errors.New("audit failure")
				}
			})
			ctx, cancel := context.WithCancel(context.Background())
			scheduler := service.NewCronScheduler(pool, name, job, 10*time.Millisecond, nil)
			failures := make(chan error, 32)
			scheduler.SetOnFailure(func(_ string, err error) {
				select {
				case failures <- err:
				default:
				}
			})
			done := make(chan struct{})
			go func() { defer close(done); scheduler.Run(ctx) }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("job did not acquire dedicated lock")
			}
			if kind == "cancel" {
				cancel()
			} else {
				require.Eventually(t, func() bool { return !scheduler.Status().LastFailureAt.IsZero() }, 2*time.Second, 5*time.Millisecond)
				cancel()
			}
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("lock cleanup did not respect its independent bound")
			}
			require.False(t, scheduler.Status().LastFailureAt.IsZero())
			if kind == "unlock_failure" {
				// A later cancellation may overwrite LastError; assert the exact
				// failure delivered to the existing operational callback instead.
				found := false
				for len(failures) > 0 {
					found = strings.Contains((<-failures).Error(), "cron unlock") || found
				}
				require.True(t, found, "unlock failure must reach the operational failure callback")
			}
			pool.Close()
			require.Eventually(t, func() bool { return cronAuditSessions(t, name) == 0 }, time.Second, 10*time.Millisecond, "failed attempt leaked a session or lock")
		})
	}
}

func TestCronLockPool_ConnectFailuresAreObservable(t *testing.T) {
	cfg := testPool.Config()
	cfg.ConnConfig.Port = 1 // no server: deterministic local refusal, never shared PG
	cfg.ConnConfig.Fallbacks = nil
	cfg.MaxConns, cfg.MinConns = 1, 0
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	defer pool.Close()
	var failures atomic.Int32
	scheduler := service.NewCronScheduler(pool, "refusal", cronAuditJob(func(context.Context) error { t.Error("job called after lock connection failure"); return nil }), 10*time.Millisecond, nil)
	scheduler.SetOnFailure(func(_ string, err error) {
		if err != nil {
			failures.Add(1)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); scheduler.Run(ctx) }()
	require.Eventually(t, func() bool { return !scheduler.Status().LastFailureAt.IsZero() }, time.Second, 5*time.Millisecond)
	cancel()
	<-done
	require.True(t, scheduler.Status().LastSuccessAt.IsZero())
	require.Contains(t, scheduler.Status().LastError, "cron lock connect")
	require.Greater(t, failures.Load(), int32(0), fmt.Sprint(scheduler.Status()))
}
