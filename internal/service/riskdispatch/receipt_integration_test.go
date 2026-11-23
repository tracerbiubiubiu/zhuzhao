//go:build integration

package riskdispatch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestReadOperationProjectsOnlyPersistedVisibleGroups(t *testing.T) {
	ctx := context.Background()
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "readonly-receipt")
	riskID := f.newRisk(t, db, "readonly-receipt", "receipt-finding", "asset-1", f.ownerA, "high", testDeadline, "assessed")
	key := fmtKey(f.actorID, riskID)
	facts := &stubFacts{visible: map[string]bool{key: true}}
	authorizer := &stubAuthorizer{allowed: map[string]bool{key: true}}
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}
	d := newTestDispatcher(db, authorizer, facts, &stubTicketCreator{}, stubTicketViews{all: true}, defs)
	op := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{riskID}, map[int64]int64{riskID: 1}), "op-readonly-dispatch")

	_, err := d.ReadOperation(ctx, f.actorID, op.OperationID)
	require.ErrorIs(t, err, errcode.ErrNotFound)

	tx, err := db.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	require.NoError(t, BindOperationTx(ctx, tx, f.actorID, op.OperationID, op.PlanID, op.PlanHash))
	require.NoError(t, tx.Commit(ctx))
	pending, err := d.ReadOperation(ctx, f.actorID, op.OperationID)
	require.NoError(t, err)
	require.Equal(t, "pending", pending.Status)
	require.Empty(t, pending.Groups)
	require.Zero(t, countFixtureTickets(t, db, f.typeCode))

	applied, err := d.Apply(ctx, f.actorID, op)
	require.NoError(t, err)
	require.Equal(t, "completed", applied.Status)

	authorizer.allowed[key] = false
	result, err := d.ReadOperation(ctx, f.actorID, op.OperationID)
	require.NoError(t, err)
	require.Equal(t, applied, result)
	require.Equal(t, int64(1), countFixtureTickets(t, db, f.typeCode))

	_, err = d.ReadOperation(ctx, f.ownerA, op.OperationID)
	require.ErrorIs(t, err, errcode.ErrNotFound)
	delete(facts.visible, key)
	_, err = d.ReadOperation(ctx, f.actorID, op.OperationID)
	require.ErrorIs(t, err, errcode.ErrNotFound)
}

func TestApplyReadsBackAlreadyLinkedAndRejectsMixedStaleGroup(t *testing.T) {
	ctx := context.Background()
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "single-group-stale")
	riskA := f.newRisk(t, db, "single-group-stale", "same-finding", "asset-a", f.ownerA, "high", testDeadline, "assessed")
	riskB := f.newRisk(t, db, "single-group-stale", "same-finding", "asset-b", f.ownerA, "high", testDeadline, "assessed")
	visible := map[string]bool{fmtKey(f.actorID, riskA): true, fmtKey(f.actorID, riskB): true}
	defs := map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}
	d := newTestDispatcher(db, &stubAuthorizer{allowed: visible}, &stubFacts{visible: visible},
		&stubTicketCreator{}, stubTicketViews{all: true}, defs)

	stale := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{riskA, riskB}, map[int64]int64{riskA: 1, riskB: 1}), "op-stale-in-group")
	execMustNoError(t, db, ctx, `UPDATE risk_items SET revision = 2 WHERE id = $1`, riskB)
	result, err := d.Apply(ctx, f.actorID, stale)
	require.NoError(t, err)
	require.Equal(t, "failed", result.Status)
	require.Zero(t, countFixtureTickets(t, db, f.typeCode))
	require.Equal(t, int64(1), requireRiskRevision(t, db, riskA))
	failed, err := d.ReadOperation(ctx, f.actorID, stale.OperationID)
	require.NoError(t, err)
	require.Equal(t, result, failed)
	require.Equal(t, []ItemOutcome{
		{RiskID: riskA, Result: ItemConflict, Reason: ReasonGroupRejected},
		{RiskID: riskB, Result: ItemConflict, Reason: ReasonRevisionChanged},
	}, failed.Groups[0].Items)

	input := buildPlanInput(f, []int64{riskA}, map[int64]int64{riskA: 1})
	created, err := d.Apply(ctx, f.actorID, applyPlan(t, d, f.actorID, input, "op-make-before-noop"))
	require.NoError(t, err)
	require.Equal(t, "completed", created.Status)

	input.Items[0].Revision = requireRiskRevision(t, db, riskA)
	op := applyPlan(t, d, f.actorID, input, "op-already-linked")
	already, err := d.Apply(ctx, f.actorID, op)
	require.NoError(t, err)
	require.Equal(t, "completed", already.Status)
	require.Equal(t, ItemAlreadyLinked, already.Groups[0].Items[0].Result)
	receipt, err := d.ReadOperation(ctx, f.actorID, op.OperationID)
	require.NoError(t, err)
	require.Equal(t, already, receipt)
	require.Equal(t, int64(1), countFixtureTickets(t, db, f.typeCode))
	require.Equal(t, int64(1), countTicketLinks(t, db, created.Groups[0].Ticket.ID))
}
