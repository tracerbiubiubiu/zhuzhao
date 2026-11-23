//go:build integration

package riskremediation

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestFinishWithoutReviewStillRequiresEvidenceAndLegalTicketEdge(t *testing.T) {
	f := newPGFixture(t, 1)
	callbacks := f.callbacks()
	callbacks.TicketPolicy = func(context.Context, pgx.Tx, TicketCheck) (Policy, error) { return Policy{ReviewGate: false}, nil }
	s := New(f.pool, callbacks)
	c := FinishCommand{Command: f.command(fixtureHandler, "off-missing-proof"), Items: []Item{f.item(0)}}
	c.Items[0].Evidence = nil
	require.ErrorIs(t, f.guarded(func(ctx context.Context, tx pgx.Tx) error { _, err := s.FinishTx(ctx, tx, c); return err }), errcode.ErrInvalidParams)
	c.Items[0] = f.item(0)
	callbacks.TicketPolicy = func(context.Context, pgx.Tx, TicketCheck) (Policy, error) {
		return Policy{}, errcode.ErrTicketInvalidTransition
	}
	s = New(f.pool, callbacks)
	require.ErrorIs(t, f.guarded(func(ctx context.Context, tx pgx.Tx) error { _, err := s.FinishTx(ctx, tx, c); return err }), errcode.ErrTicketInvalidTransition)
	callbacks.TicketPolicy = func(context.Context, pgx.Tx, TicketCheck) (Policy, error) { return Policy{ReviewGate: false}, nil }
	s = New(f.pool, callbacks)
	var res Result
	require.NoError(t, f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		var err error
		res, err = s.FinishTx(ctx, tx, c)
		return err
	}))
	require.True(t, res.TerminalAllowed)
	require.Equal(t, "applied", res.Items[0].Result)
	var state string
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT status FROM risk_items WHERE id=$1`, f.risks[0]).Scan(&state))
	require.Equal(t, "closed", state)
}

func TestEvidenceAdapterFailureHasNoSubmissionOrReceipt(t *testing.T) {
	f := newPGFixture(t, 2)
	callbacks := f.callbacks()
	callbacks.Evidence = func(_ context.Context, _ pgx.Tx, c EvidenceCheck) error {
		if c.LinkID == f.links[1] {
			return errcode.ErrFileAlreadyBound
		}
		return nil
	}
	s := New(f.pool, callbacks)
	c := FinishCommand{Command: f.command(fixtureHandler, "file-ineligible"), Items: []Item{f.item(0), f.item(1)}}
	c.Items[1].Evidence = []EvidenceRef{{Kind: "file", Ref: "synthetic-object", Version: "v1"}}
	require.ErrorIs(t, f.guarded(func(ctx context.Context, tx pgx.Tx) error { _, err := s.FinishTx(ctx, tx, c); return err }), errcode.ErrFileAlreadyBound)
	for _, table := range []string{"risk_remediation_submissions", "risk_remediation_receipts", "risk_events", "risk_remediation_intents"} {
		var n int
		require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM `+table).Scan(&n))
		require.Zero(t, n)
	}
}
