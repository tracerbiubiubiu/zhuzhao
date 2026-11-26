//go:build integration

package riskremediation

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestFinishPersistsProposalWithoutResolving(t *testing.T) {
	f := newPGFixture(t, 2)
	c := FinishCommand{Command: f.command(fixtureHandler, "proposal-K"), Items: []Item{f.item(0), f.item(1)}}
	for i := range c.Items {
		c.Items[i].Evidence[0].Kind = "reference"
	}
	var result Result
	err := f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		var err error
		result, err = f.svc.FinishTx(ctx, tx, c)
		return err
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	require.False(t, result.TerminalAllowed)
	var n int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM risk_ticket_links WHERE remediation_state='pending_verify' AND resolution IS NULL`).Scan(&n))
	require.Equal(t, 2, n)
}
