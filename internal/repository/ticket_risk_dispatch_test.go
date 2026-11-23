package repository

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

type riskGuardTestTx struct{ pgx.Tx }

func TestRequireGuardedCreateTx(t *testing.T) {
	pool, otherPool := new(pgxpool.Pool), new(pgxpool.Pool)
	tx, otherTx := &riskGuardTestTx{}, &riskGuardTestTx{}
	repo := NewTicketRepo(pool)
	for _, tc := range []struct {
		name     string
		state    *iamWriteState
		supplied pgx.Tx
		allowed  bool
	}{
		{name: "no guard", supplied: tx},
		{name: "wrong pool", state: &iamWriteState{pool: otherPool, tx: tx}, supplied: tx},
		{name: "wrong tx", state: &iamWriteState{pool: pool, tx: tx}, supplied: otherTx},
		{name: "nil tx", state: &iamWriteState{pool: pool, tx: tx}},
		{name: "empty guard", state: &iamWriteState{pool: pool}, supplied: tx},
		{name: "command guard", state: &iamWriteState{pool: pool, tx: tx, shared: true}, supplied: tx, allowed: true},
		{name: "write guard", state: &iamWriteState{pool: pool, tx: tx}, supplied: tx, allowed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.state != nil {
				ctx = context.WithValue(ctx, iamWriteKey{}, tc.state)
			}
			err := repo.RequireGuardedCreateTx(ctx, pool, tc.supplied)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errcode.ErrNoPermission)
			}
		})
	}
	ctx := context.WithValue(context.Background(), iamWriteKey{}, &iamWriteState{pool: pool, tx: tx})
	require.ErrorIs(t, repo.RequireGuardedCreateTx(ctx, otherPool, tx), errcode.ErrNoPermission)
}
