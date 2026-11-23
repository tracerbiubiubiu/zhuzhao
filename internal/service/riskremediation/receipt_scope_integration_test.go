//go:build integration

package riskremediation

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func receiptForTest(f *pgFixture, operation string) (res Result, err error) {
	err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		res, err = f.svc.ReceiptTx(ctx, tx, fixtureHandler, operation)
		return err
	})
	return res, err
}

func TestReceiptAndReplayIgnoreUnrelatedActiveItem(t *testing.T) {
	f := newPGFixture(t, 2)
	_, err := f.pool.Exec(f.ctx, `UPDATE risk_ticket_links SET ended_at=clock_timestamp(),end_reason='synthetic excluded' WHERE id=$1`, f.links[1])
	require.NoError(t, err)
	c := FinishCommand{Command: f.command(fixtureHandler, "receipt-original-set"), Items: []Item{f.item(0)}}
	original, err := f.finish(c)
	require.NoError(t, err)
	_, err = f.pool.Exec(f.ctx, `UPDATE risk_ticket_links SET ended_at=NULL,end_reason=NULL WHERE id=$1`, f.links[1])
	require.NoError(t, err)
	callbacks := f.callbacks()
	authorize := callbacks.Authorize
	callbacks.Authorize = func(ctx context.Context, tx pgx.Tx, a Access) error {
		if a.RiskID == f.risks[1] {
			return errcode.ErrNoPermission
		}
		return authorize(ctx, tx, a)
	}
	f.svc = New(f.pool, callbacks)
	before := f.snapshot()
	replayed, err := f.finish(c)
	require.NoError(t, err)
	require.Equal(t, original, replayed)
	read, err := receiptForTest(f, c.OperationID)
	require.NoError(t, err, "adding an unrelated unreadable active risk cannot revoke the original receipt")
	require.Equal(t, original, read)
	require.Equal(t, before, f.snapshot())
}

func TestReceiptAndReplayRequireOriginalTicketRead(t *testing.T) {
	f := newPGFixture(t, 1)
	c := FinishCommand{Command: f.command(fixtureHandler, "receipt-ticket-revoked"), Items: []Item{f.item(0)}}
	_, err := f.finish(c)
	require.NoError(t, err)
	callbacks := f.callbacks()
	authorize := callbacks.Authorize
	callbacks.Authorize = func(ctx context.Context, tx pgx.Tx, a Access) error {
		if a.Action == "receipt" && a.TicketID == f.ticket && a.RiskID == 0 {
			return errcode.ErrNoPermission
		}
		return authorize(ctx, tx, a)
	}
	f.svc = New(f.pool, callbacks)
	before := f.snapshot()
	_, err = f.finish(c)
	require.ErrorIs(t, err, errcode.ErrNoPermission, "risk read permission alone must not authorize original ticket replay")
	_, err = receiptForTest(f, c.OperationID)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	require.Equal(t, before, f.snapshot())
}

func TestReceiptAdmissionPrecedesOperationLookup(t *testing.T) {
	f := newPGFixture(t, 1)
	_, err := f.pool.Exec(f.ctx, `UPDATE fixture_actors SET enabled=false WHERE id=$1`, fixtureHandler)
	require.NoError(t, err)
	_, err = receiptForTest(f, "nonexistent-operation")
	require.ErrorIs(t, err, errcode.ErrNoPermission, "unadmitted callers must not learn whether an operation exists")
}

func TestReceiptAndReplayDenyDeletedOriginalTicket(t *testing.T) {
	f := newPGFixture(t, 1)
	c := FinishCommand{Command: f.command(fixtureHandler, "receipt-deleted-ticket"), Items: []Item{f.item(0)}}
	_, err := f.finish(c)
	require.NoError(t, err)
	_, err = f.pool.Exec(f.ctx, `UPDATE risk_dispatch_groups SET ended_at=clock_timestamp() WHERE ticket_id=$1`, f.ticket)
	require.NoError(t, err)
	_, err = f.pool.Exec(f.ctx, `DELETE FROM tickets WHERE id=$1`, f.ticket)
	require.NoError(t, err)
	before := f.snapshot()
	_, err = f.finish(c)
	require.ErrorIs(t, err, errcode.ErrNotFound)
	_, err = receiptForTest(f, c.OperationID)
	require.ErrorIs(t, err, errcode.ErrNotFound)
	require.Equal(t, before, f.snapshot())
}

func TestTicketCommandBindingIsPartOfOperationIdentity(t *testing.T) {
	f := newPGFixture(t, 1)
	command := FinishCommand{Command: f.command(fixtureHandler, "bound-ticket-request"), Items: []Item{f.item(0)}}
	command.TicketBinding = strings.Repeat("a", 64)
	first, err := f.finish(command)
	require.NoError(t, err)
	same, err := f.finish(command)
	require.NoError(t, err)
	require.Equal(t, first, same)
	before := f.snapshot()
	command.TicketBinding = strings.Repeat("b", 64)
	_, err = f.finish(command)
	require.ErrorIs(t, err, errcode.ErrConflict)
	require.Equal(t, before, f.snapshot())
}
