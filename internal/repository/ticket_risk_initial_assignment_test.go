package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

type initialRiskRefreshTx struct {
	pgx.Tx
	t   *testing.T
	err error
}
type initialRiskRefreshRow func(...any) error

func (row initialRiskRefreshRow) Scan(d ...any) error { return row(d...) }
func (tx initialRiskRefreshTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	require.Contains(tx.t, q, "SELECT revision, updated_at, status_changed_at FROM tickets WHERE id=$1")
	require.Equal(tx.t, int64(101), args[0])
	return initialRiskRefreshRow(func(d ...any) error {
		if tx.err != nil {
			return tx.err
		}
		*d[0].(*int64) = 3
		*d[1].(*time.Time) = time.Unix(2, 0)
		at := time.Unix(2, 0)
		*d[2].(**time.Time) = &at
		return nil
	})
}
func TestRefreshRiskDispatchTicketTxUsesOwningTransactionAndPropagatesFailure(t *testing.T) {
	repo := NewTicketRepo(nil)
	ticket := &model.Ticket{ID: 101}
	require.NoError(t, repo.RefreshRiskDispatchTicketTx(context.Background(), initialRiskRefreshTx{t: t}, ticket))
	require.Equal(t, int64(3), ticket.Revision)
	require.Equal(t, time.Unix(2, 0), ticket.UpdatedAt)
	fail := errors.New("synthetic refresh")
	require.ErrorIs(t, repo.RefreshRiskDispatchTicketTx(context.Background(), initialRiskRefreshTx{t: t, err: fail}, ticket), fail)
}
