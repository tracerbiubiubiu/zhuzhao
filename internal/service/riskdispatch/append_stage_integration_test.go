//go:build integration

package riskdispatch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyAppendWaitsForRetestReworkAndKeepsOriginalReceipt(t *testing.T) {
	db := dispatchTestDB
	ctx := context.Background()
	f := setupDispatchFixture(t, db, "append-stage")
	firstRisk := f.newRisk(t, db, "append-stage", "finding", "asset-1", f.ownerA, "medium", testDeadline, "assessed")
	nextRisk := f.newRisk(t, db, "append-stage", "finding", "asset-2", f.ownerA, "urgent", testDeadline, "assessed")
	d := newAggregateTestDispatcher(f, map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}, firstRisk, nextRisk)
	op1 := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{firstRisk}, map[int64]int64{firstRisk: 1}), "stage-first")
	first, err := d.Apply(ctx, f.actorID, op1)
	require.NoError(t, err)
	ticket := first.Groups[0].Ticket.ID
	// Preview before the ticket enters review; commit must recheck its stage.
	op2 := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{firstRisk, nextRisk}, map[int64]int64{firstRisk: 2, nextRisk: 1}), "stage-next")
	execMustNoError(t, db, ctx, `UPDATE tickets SET status='pending_verify' WHERE id=$1`, ticket)
	result, err := d.Apply(ctx, f.actorID, op2)
	require.NoError(t, err)
	require.Equal(t, "pending", result.Status)
	require.Equal(t, "failed", result.Groups[0].Status)
	require.Len(t, result.Groups[0].Items, 2)
	require.Equal(t, ItemAlreadyLinked, result.Groups[0].Items[0].Result)
	require.Equal(t, ItemConflict, result.Groups[0].Items[1].Result)
	require.Equal(t, "ticket_stage_not_appendable", result.Groups[0].Items[1].Reason)
	require.Equal(t, int64(1), countTicketLinks(t, db, ticket))
	require.Equal(t, "assessed", requireRiskStatus(t, db, nextRisk))
	require.Equal(t, 3, requireTicketPriority(t, db, ticket))
	replay, err := d.Apply(ctx, f.actorID, op1)
	require.NoError(t, err)
	require.Equal(t, "completed", replay.Status)
	require.Equal(t, ticket, replay.Groups[0].Ticket.ID)
	recovered, err := d.ReadOperation(ctx, f.actorID, op1.OperationID)
	require.NoError(t, err)
	require.Equal(t, "completed", recovered.Status)
	require.Equal(t, ticket, recovered.Groups[0].Ticket.ID)
	// Existing reject/rework path restores appendability; same operation can retry.
	execMustNoError(t, db, ctx, `UPDATE tickets SET status='in_progress' WHERE id=$1`, ticket)
	result, err = d.Apply(ctx, f.actorID, op2)
	require.NoError(t, err)
	require.Equal(t, "completed", result.Status)
	require.Equal(t, ticket, result.Groups[0].Ticket.ID)
	require.Equal(t, int64(2), countTicketLinks(t, db, ticket))
	require.Equal(t, int64(1), countFixtureTickets(t, db, f.typeCode))
}
