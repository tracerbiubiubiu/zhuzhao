//go:build integration

package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func conflictTicket(t *testing.T, status string) int64 {
	t.Helper()
	ctx := context.Background()
	code := relUnique("conflict")
	_, err := testPool.Exec(ctx, `INSERT INTO ticket_types(code,name) VALUES($1,$1)`, code)
	require.NoError(t, err)
	org := insertOrg(t, code, code, nil)
	var id int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO tickets(type_code,title,status,created_by,org_id,org_path) VALUES($1,$1,$2,0,$3,$4::ltree) RETURNING id`, code, status, org, code).Scan(&id))
	t.Cleanup(func() {
		_, err := testPool.Exec(ctx, `DELETE FROM tickets WHERE id=$1`, id)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, org)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, `DELETE FROM ticket_types WHERE code=$1`, code)
		require.NoError(t, err)
	})
	return id
}

// All connections are admitted before either zero-row write begins. Transactions
// remain open until every result arrives, so an accidental second pool checkout
// deterministically waits for the deadline instead of obtaining a lucky spare.
func TestTicketConflict_FullPool(t *testing.T) {
	id := conflictTicket(t, "closed")
	for _, action := range []string{"status", "assignee"} {
		t.Run(action, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			cfg := testPool.Config()
			cfg.MaxConns, cfg.MinConns = 4, 0
			pool, err := pgxpool.NewWithConfig(ctx, cfg)
			require.NoError(t, err)
			defer pool.Close()
			repo := repository.NewTicketRepo(pool)
			txs := make([]pgx.Tx, 4)
			for i := range txs {
				txs[i], err = pool.Begin(ctx)
				require.NoError(t, err)
				defer txs[i].Rollback(context.Background()) //nolint:errcheck // cleanup after assertion failure
			}
			require.EqualValues(t, 4, pool.Stat().AcquiredConns())
			require.Zero(t, pool.Stat().IdleConns())
			results := make(chan error, len(txs))
			for _, tx := range txs {
				go func(tx pgx.Tx) {
					if action == "status" {
						results <- repo.UpdateStatusTx(ctx, tx, id, "open", "assigned")
					} else {
						results <- repo.UpdateAssignedToTx(ctx, tx, id, nil)
					}
				}(tx)
			}
			for range txs {
				err = <-results
				require.ErrorIs(t, err, errcode.ErrTicketAlreadyClosed, "conflict classification must reuse the admitted transaction")
			}
		})
	}
}

// Reusing tx also preserves its own writes, even when the pool has spare slots.
func TestTicketConflict_UncommittedState(t *testing.T) {
	ctx := context.Background()
	id := conflictTicket(t, "open")
	repo := repository.NewTicketRepo(testPool)
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx) //nolint:errcheck // transaction is deliberately uncommitted
	_, err = tx.Exec(ctx, `UPDATE tickets SET status='closed' WHERE id=$1`, id)
	require.NoError(t, err)
	require.ErrorIs(t, repo.UpdateStatusTx(ctx, tx, id, "open", "assigned"), errcode.ErrTicketAlreadyClosed)
	require.ErrorIs(t, repo.UpdateAssignedToTx(ctx, tx, id, nil), errcode.ErrTicketAlreadyClosed)
}

func TestTicketConflict_Semantics(t *testing.T) {
	ctx := context.Background()
	id := conflictTicket(t, "assigned")
	repo := repository.NewTicketRepo(testPool)
	// Nontransactional callers continue using the same pool as their executor.
	require.ErrorIs(t, repo.UpdateStatus(ctx, -id, "open", "assigned"), errcode.ErrTicketNotFound)
	require.ErrorIs(t, repo.UpdateStatus(ctx, id, "open", "assigned"), errcode.ErrConcurrentModification)
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx) //nolint:errcheck // semantic checks do not persist fixture changes
	require.ErrorIs(t, repo.UpdateStatusTx(ctx, tx, -id, "open", "assigned"), errcode.ErrTicketNotFound)
	require.ErrorIs(t, repo.UpdateAssignedToTx(ctx, tx, -id, nil), errcode.ErrTicketNotFound)
	require.NoError(t, repo.UpdateStatusTx(ctx, tx, id, "assigned", "in_progress"))
	require.NoError(t, repo.UpdateAssignedToTx(ctx, tx, id, nil))
	var status string
	require.NoError(t, tx.QueryRow(ctx, `SELECT status FROM tickets WHERE id=$1`, id).Scan(&status))
	require.Equal(t, "in_progress", status)
}
