//go:build integration

package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestC04_CommandGuardSerializesWithIAMWrites(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	other, err := pgxpool.New(ctx, testPool.Config().ConnString())
	require.NoError(t, err)
	defer other.Close()
	probe, err := other.Acquire(ctx)
	require.NoError(t, err)
	defer probe.Release()
	unexpected := func(context.Context) error {
		t.Error("incompatible IAM lock admitted a transaction")
		return nil
	}
	var commandPID int32
	var key int64
	var granted bool

	require.NoError(t, repository.WithIAMCommandGuard(ctx, testPool, func(guarded context.Context, tx pgx.Tx) error {
		require.Equal(t, tx, repository.IAMDB(guarded, testPool))
		require.True(t, repository.IAMGuardActive(guarded))
		require.False(t, repository.IAMWriteActive(guarded))
		require.Error(t, repository.WithIAMGuard(guarded, testPool, unexpected))
		require.Error(t, repository.AcquireIAMTransactionGuard(guarded, nil))
		require.NoError(t, tx.QueryRow(guarded, `
			SELECT pid, (classid::bigint << 32) | objid::bigint
			FROM pg_locks WHERE pid=pg_backend_pid() AND locktype='advisory' AND mode='ShareLock' AND granted`,
		).Scan(&commandPID, &key))
		require.NoError(t, probe.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&granted))
		require.False(t, granted)

		// Commands against different tickets may overlap in the same service
		// and across pools. Drop the inherited guard to acquire a fresh one.
		for _, pool := range []*pgxpool.Pool{testPool, other} {
			require.NoError(t, repository.WithIAMCommandGuard(ctx, pool, func(context.Context, pgx.Tx) error { return nil }))
		}
		wait, stop := context.WithTimeout(ctx, 120*time.Millisecond)
		defer stop()
		require.ErrorIs(t, repository.WithIAMGuard(wait, other, unexpected), context.DeadlineExceeded)
		return nil
	}))

	// The session returned to the pool alive and without its advisory lock.
	var sessionAlive, lockHeld bool
	require.NoError(t, probe.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid=$1),
		       EXISTS (SELECT 1 FROM pg_locks WHERE pid=$1 AND locktype='advisory')`, commandPID,
	).Scan(&sessionAlive, &lockHeld))
	require.True(t, sessionAlive)
	require.False(t, lockHeld)

	require.NoError(t, repository.WithIAMGuard(ctx, other, func(guarded context.Context) error {
		require.True(t, repository.IAMWriteActive(guarded))
		require.NoError(t, probe.QueryRow(ctx, `SELECT pg_try_advisory_lock_shared($1)`, key).Scan(&granted))
		require.False(t, granted)
		wait, stop := context.WithTimeout(ctx, 120*time.Millisecond)
		defer stop()
		require.ErrorIs(t, repository.WithIAMCommandGuard(wait, testPool, func(ctx context.Context, _ pgx.Tx) error { return unexpected(ctx) }), context.DeadlineExceeded)
		waitLocal, stopLocal := context.WithTimeout(ctx, 120*time.Millisecond)
		defer stopLocal()
		require.ErrorIs(t, repository.WithIAMCommandGuard(waitLocal, other, func(ctx context.Context, _ pgx.Tx) error { return unexpected(ctx) }), context.DeadlineExceeded)
		return nil
	}))
	require.NoError(t, repository.WithIAMCommandGuard(ctx, testPool, func(context.Context, pgx.Tx) error { return nil }))
}
