//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestIAMSavepointBindsExactChildAndDefersOnlyCommittedProjections(t *testing.T) {
	ctx := context.Background()
	stop := errors.New("rollback synthetic savepoint")
	projected := 0
	require.NoError(t, repository.WithIAMCommandGuard(ctx, testPool, func(guarded context.Context, parent pgx.Tx) error {
		_, err := parent.Exec(guarded, `CREATE TEMPORARY TABLE iam_savepoint_probe (id int) ON COMMIT DROP`)
		require.NoError(t, err)
		work := func(childCtx context.Context, child pgx.Tx, fail bool) error {
			require.Error(t, repository.AfterIAMCommit(guarded, func() error { projected += 100; return nil }))
			require.Error(t, child.Commit(childCtx))
			require.Error(t, child.Rollback(childCtx))
			require.True(t, repository.IAMGuardActive(childCtx))
			require.False(t, repository.IAMWriteActive(childCtx))
			require.Same(t, child, repository.IAMDB(childCtx, testPool))
			require.NotSame(t, parent, child)
			require.Same(t, parent.Conn(), child.Conn())
			require.NoError(t, repository.AfterIAMCommit(childCtx, func() error { projected++; return nil }))
			_, err := child.Exec(childCtx, `INSERT INTO iam_savepoint_probe VALUES (1)`)
			require.NoError(t, err)
			if fail {
				return stop
			}
			return nil
		}
		require.ErrorIs(t, repository.WithIAMSavepoint(guarded, testPool, parent, func(childCtx context.Context, child pgx.Tx) error {
			return work(childCtx, child, true)
		}), stop)
		require.NoError(t, repository.WithIAMSavepoint(guarded, testPool, parent, func(childCtx context.Context, child pgx.Tx) error {
			return work(childCtx, child, false)
		}))
		require.Zero(t, projected)
		require.Same(t, parent, repository.IAMDB(guarded, testPool))
		var rows int
		require.NoError(t, parent.QueryRow(guarded, `SELECT count(*) FROM iam_savepoint_probe`).Scan(&rows))
		require.Equal(t, 1, rows)
		return nil
	}))
	require.Equal(t, 1, projected)
}

func TestIAMSavepointProjectionWaitsForOuterCommit(t *testing.T) {
	ctx := context.Background()
	stop := errors.New("rollback synthetic outer transaction")
	observed := false
	err := repository.WithIAMCommandGuard(ctx, testPool, func(outerCtx context.Context, parent pgx.Tx) error {
		require.NoError(t, repository.WithIAMSavepoint(outerCtx, testPool, parent, func(childCtx context.Context, child pgx.Tx) error {
			return repository.WithIAMSavepoint(childCtx, testPool, child, func(innerCtx context.Context, inner pgx.Tx) error {
				require.Same(t, inner, repository.IAMDB(innerCtx, testPool))
				return repository.AfterIAMCommit(innerCtx, func() error { observed = true; return nil })
			})
		}))
		require.False(t, observed)
		return stop
	})
	require.ErrorIs(t, err, stop)
	require.False(t, observed)
}
