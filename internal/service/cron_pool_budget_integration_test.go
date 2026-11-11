//go:build integration

package service_test

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"testing"
	"time"
)

func TestCronLockPool_RealArchiveWithOneConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cfg := testPool.Config()
	cfg.MaxConns = 1
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	job := service.NewAuditArchiveJob(repository.NewAuditLogRepo(pool), 180, 10, t.TempDir(), 395, nil)
	scheduler := service.NewCronScheduler(pool, "audit-pool-budget", job, 5*time.Millisecond, nil)
	stopped := make(chan struct{})
	go func() { defer close(stopped); scheduler.Run(ctx) }()
	// Real ArchiveFetchBatch must run with a single configured connection.
	// Wait for either a completed tick or cancellation; this is not a sleep race.
	deadline := time.NewTimer(1200 * time.Millisecond)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if !scheduler.Status().LastSuccessAt.IsZero() {
				cancel()
				<-stopped
				return
			}
		case <-deadline.C:
			cancel()
			<-stopped
			st := scheduler.Status()
			t.Logf("pool_max=1 acquired=%d idle=%d last_success=%v last_failure=%v error=%q", pool.Stat().AcquiredConns(), pool.Stat().IdleConns(), st.LastSuccessAt, st.LastFailureAt, st.LastError)
			require.False(t, st.LastSuccessAt.IsZero(), "cron locking must not consume the only connection needed by its real maintenance job")
			return
		}
	}
}
