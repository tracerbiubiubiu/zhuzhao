//go:build integration

package riskdispatch

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

type changedDispatchDefinition struct{ *stubTicketCreator }

func (changedDispatchDefinition) PrepareRiskDispatchCreate(context.Context, *model.CreateTicketRequest, int64) (TicketPreparation, error) {
	return nil, errcode.ErrConcurrentModification
}

type rejectedDispatchAssignment struct{ *stubTicketCreator }

func (s rejectedDispatchAssignment) CreateRiskDispatchTicketTx(ctx context.Context, tx pgx.Tx,
	locked LockedTicketCreate, owner int64, link RiskDispatchLinkWriter) (TicketOutcome, error) {
	if _, err := s.stubTicketCreator.CreateRiskDispatchTicketTx(ctx, tx, locked, owner, link); err != nil {
		return TicketOutcome{}, err
	}
	return TicketOutcome{}, errcode.ErrNoPermission
}

func TestApplyPersistsExplicitTicketBridgeFailures(t *testing.T) {
	ctx := context.Background()
	db := dispatchTestDB
	for _, tc := range []struct {
		name    string
		tickets TicketCreator
		result  ItemResult
		reason  string
	}{
		{"definition", changedDispatchDefinition{&stubTicketCreator{}}, ItemConflict, "target_definition_changed"},
		{"assignment", rejectedDispatchAssignment{&stubTicketCreator{}}, ItemDenied, "ticket_assignment_forbidden"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setupDispatchFixture(t, db, "bridge_"+tc.name)
			risk := f.newRisk(t, db, tc.name, "finding", "asset", f.ownerA, "high", testDeadline, "assessed")
			d := newAggregateTestDispatcher(f, map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}, risk)
			d.deps.Tickets = tc.tickets
			input := buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 1})
			op := applyPlan(t, d, f.actorID, input, "op-bridge")
			failed, err := d.Apply(ctx, f.actorID, op)
			require.NoError(t, err)
			require.Equal(t, "failed", failed.Status)
			require.Len(t, failed.Groups, 1)
			require.Equal(t, []ItemOutcome{{RiskID: risk, Result: tc.result, Reason: tc.reason}}, failed.Groups[0].Items)
			require.Zero(t, countFixtureTickets(t, db, f.typeCode))
			require.Zero(t, countRows(t, db, `SELECT COUNT(*) FROM risk_ticket_links WHERE risk_id = $1`, risk))
			require.Equal(t, int64(1), requireRiskRevision(t, db, risk))

			d.deps.Tickets = &stubTicketCreator{}
			replayed, err := d.Apply(ctx, f.actorID, op)
			require.NoError(t, err)
			require.Equal(t, failed, replayed)
			read, err := d.ReadOperation(ctx, f.actorID, op.OperationID)
			require.NoError(t, err)
			require.Equal(t, failed, read)
		})
	}
}
