//go:build integration

package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestTicketSnapshot_FullPoolUsesOnlyTransaction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	config := testPool.Config()
	config.MaxConns, config.MinConns = 4, 0
	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)
	defer pool.Close()
	repo := repository.NewTicketRepo(pool)
	tickets := make([]*model.Ticket, 4)
	types := make([]*model.TicketType, 4)
	for i := range tickets {
		id := conflictTicket(t, "open")
		tickets[i], err = repo.GetByID(ctx, id)
		require.NoError(t, err)
		types[i], err = repo.GetTicketType(ctx, tickets[i].TypeCode)
		require.NoError(t, err)
		// Display enrichment is not part of the persisted snapshot.
		tickets[i].CreatedByName, tickets[i].AssigneeName = "display", "display"
	}
	txs := make([]pgx.Tx, 4)
	for i := range txs {
		txs[i], err = pool.Begin(ctx)
		require.NoError(t, err)
		defer txs[i].Rollback(context.Background()) //nolint:errcheck // cleanup
	}
	require.EqualValues(t, 4, pool.Stat().AcquiredConns())
	require.Zero(t, pool.Stat().IdleConns())
	results := make(chan error, 4)
	for i, tx := range txs {
		go func(i int, tx pgx.Tx) {
			results <- repo.LockTransitionSnapshotTx(ctx, tx, tickets[i], types[i])
		}(i, tx)
	}
	for range txs {
		require.NoError(t, <-results, "snapshot fencing cannot acquire a second pool connection")
	}
}
