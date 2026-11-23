//go:build integration

package riskdispatch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlanIncludesOnlyRisksWithVerifiedDispatchBasis(t *testing.T) {
	ctx := context.Background()
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "basis_plan")
	verified := f.newRisk(t, db, "basis_plan", "finding", "verified", f.ownerA, "high", testDeadline, "assessed")
	unverified := f.newRisk(t, db, "basis_plan", "finding", "unverified", f.ownerA, "high", testDeadline, "assessed")
	visible := map[string]bool{fmtKey(f.actorID, verified): true, fmtKey(f.actorID, unverified): true}
	facts := &stubFacts{visible: visible, basisByRisk: map[int64]string{unverified: ""}}
	d := newTestDispatcher(db, &stubAuthorizer{allowed: visible}, facts, &stubTicketCreator{},
		stubTicketViews{}, map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}})

	plan, err := d.Plan(ctx, f.actorID, buildPlanInput(f, []int64{verified, unverified}, map[int64]int64{verified: 1, unverified: 1}))
	require.NoError(t, err)
	require.Len(t, plan.Groups, 1)
	require.Len(t, plan.Groups[0].Items, 1)
	require.Equal(t, verified, plan.Groups[0].Items[0].RiskID)
	require.Equal(t, []PlanBlockView{{RiskID: unverified, Reason: string(UnverifiedRisk)}}, plan.Blocked)
}

func TestApplyRevalidatesDispatchBasisAndReplaysSuccessfulReceipt(t *testing.T) {
	ctx := context.Background()
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "basis_apply")
	unchanged := f.newRisk(t, db, "basis_apply", "finding-A", "same", f.ownerA, "high", testDeadline, "assessed")
	removed := f.newRisk(t, db, "basis_apply", "finding-B", "removed", f.ownerA, "high", testDeadline, "assessed")
	changed := f.newRisk(t, db, "basis_apply", "finding-C", "changed", f.ownerA, "high", testDeadline, "assessed")
	visible := map[string]bool{fmtKey(f.actorID, unchanged): true, fmtKey(f.actorID, removed): true, fmtKey(f.actorID, changed): true}
	facts := &stubFacts{visible: visible, basisByRisk: map[int64]string{}}
	tickets := &stubTicketCreator{}
	d := newTestDispatcher(db, &stubAuthorizer{allowed: visible}, facts, tickets,
		stubTicketViews{all: true}, map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}})

	op := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{unchanged, removed, changed},
		map[int64]int64{unchanged: 1, removed: 1, changed: 1}), "op-basis")
	facts.basisByRisk[removed] = ""
	facts.basisByRisk[changed] = "synthetic-basis-v2"
	result, err := d.Apply(ctx, f.actorID, op)
	require.NoError(t, err)
	require.Equal(t, "partial", result.Status)
	require.Len(t, result.Groups, 3)
	require.Equal(t, "completed", result.Groups[0].Status)
	require.Equal(t, "failed", result.Groups[1].Status)
	require.Equal(t, "failed", result.Groups[2].Status)
	require.Equal(t, []ItemOutcome{{RiskID: removed, Result: ItemInvalid, Reason: "dispatch_basis_unavailable"}}, result.Groups[1].Items)
	require.Equal(t, []ItemOutcome{{RiskID: changed, Result: ItemConflict, Reason: "dispatch_basis_changed"}}, result.Groups[2].Items)
	require.Equal(t, int64(1), countFixtureTickets(t, db, f.typeCode))
	require.Equal(t, 1, tickets.creates)

	facts.basisByRisk[unchanged] = ""
	replay, err := d.Apply(ctx, f.actorID, op)
	require.NoError(t, err)
	require.Equal(t, result, replay)
	require.Equal(t, 1, tickets.creates)
	fromRead, err := d.ReadOperation(ctx, f.actorID, op.OperationID)
	require.NoError(t, err)
	require.Equal(t, result, fromRead)
}
