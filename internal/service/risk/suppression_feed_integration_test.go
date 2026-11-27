//go:build integration

package risk_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func TestRiskFeedStartsNewCycleOnlyAfterFalsePositiveSuppression(t *testing.T) {
	seedRiskFixture(t)
	ctx := t.Context()
	svc := newService()
	suffix := uniqueSuffix()
	source := seedBasisSource(t)
	now := time.Now().UTC()
	observation := baseObservation(suffix, "first", "1")
	observation.SourceRef = source
	observation.SourceItemKey = "suppression-" + suffix
	observation.ObservedAt = now.Add(-4 * time.Hour)

	send := func(eventID, sequence string, observedAt time.Time) model.RiskReportItem {
		t.Helper()
		input := observation
		input.EventID = eventID + "-" + suffix
		input.SourceSequence = sequence
		input.ObservedAt = observedAt
		result, err := svc.ReportFromSource(ctx, model.RiskReportRequest{Observations: []model.FindingObservationV1{input}}, source)
		require.NoError(t, err)
		return result.Items[0]
	}
	first := send("first", "1", now.Add(-4*time.Hour))
	closed, err := svc.FalsePositive(ctx, model.RiskFalsePositiveRequest{
		ID: first.RiskRef.ID, OperationID: "false-positive-" + suffix, ExpectedRevision: "1",
		Reason: "Synthetic evidence excludes this observation", SuppressionValidUntil: now.Add(time.Hour),
	}, userM)
	require.NoError(t, err)
	require.Equal(t, first.RiskRef.ID, send("suppressed", "2", now.Add(-3*time.Hour)).RiskRef.ID)

	// Advance the suppression clock by updating only this isolated fixture.
	_, err = testPool.Exec(ctx, `UPDATE risk_suppressions SET
		created_at=$2, valid_until=$3 WHERE id=$1`, closed.Suppression.ID, now.Add(-4*time.Hour), now.Add(-time.Hour))
	require.NoError(t, err)
	require.Equal(t, first.RiskRef.ID, send("late", "3", now.Add(-2*time.Hour)).RiskRef.ID)
	fresh := send("fresh", "4", now.Add(-5*time.Minute))
	require.Equal(t, "accepted", fresh.Result)
	require.NotEqual(t, first.RiskRef.ID, fresh.RiskRef.ID)
	freshMirror := mirrorHashOf(t, source, observation.SourceItemKey)
	lateAfterFresh := send("late-after-fresh", "5", now.Add(-90*time.Minute))
	require.Equal(t, "stale", lateAfterFresh.Result)
	require.Equal(t, "before_suppression_window_end", lateAfterFresh.SafeReason)
	require.Nil(t, lateAfterFresh.RiskRef)
	require.Equal(t, freshMirror, mirrorHashOf(t, source, observation.SourceItemKey))
	require.Equal(t, fresh.RiskRef.ID, send("after-fresh", "6", now.Add(-4*time.Minute)).RiskRef.ID)
	require.Equal(t, fresh.IngestID, send("fresh", "4", now.Add(-5*time.Minute)).IngestID)

	var generation, count int
	var currentCycle int64
	require.NoError(t, testPool.QueryRow(ctx, `
		SELECT ri.generation, f.current_cycle_id, (SELECT count(*) FROM risk_items WHERE family_id=f.id)
		FROM risk_items ri JOIN risk_families f ON f.id=ri.family_id WHERE ri.id=$1`, fresh.RiskRef.ID).
		Scan(&generation, &currentCycle, &count))
	require.Equal(t, 2, generation)
	require.Equal(t, 2, count)
	require.Equal(t, fresh.RiskRef.ID, currentCycle)
	old, err := svc.Detail(ctx, first.RiskRef.ID, userM)
	require.NoError(t, err)
	require.Equal(t, "closed", old.View.Status)
	require.Equal(t, "false_positive", *old.View.ClosureKind)
	receipt, err := svc.OperationResult(ctx, closed.Receipt.OperationID, userM)
	require.NoError(t, err)
	require.Equal(t, first.RiskRef.ID, receipt.RiskID)
	require.Equal(t, closed.Receipt.Revision, receipt.Revision)

	_, err = svc.FalsePositive(ctx, model.RiskFalsePositiveRequest{
		ID: fresh.RiskRef.ID, OperationID: "false-positive-repeat-" + suffix, ExpectedRevision: "1",
		Reason: "Independent synthetic review of the second cycle", SuppressionValidUntil: now.Add(2 * time.Hour),
	}, userM)
	require.NoError(t, err)
	secondMirror := mirrorHashOf(t, source, observation.SourceItemKey)
	lateAfterSecondClose := send("late-after-second-close", "7", now.Add(-80*time.Minute))
	require.Equal(t, "stale", lateAfterSecondClose.Result)
	require.Equal(t, "before_suppression_window_end", lateAfterSecondClose.SafeReason)
	require.Equal(t, secondMirror, mirrorHashOf(t, source, observation.SourceItemKey))
	require.Equal(t, fresh.RiskRef.ID, send("second-suppressed", "8", now.Add(-3*time.Minute)).RiskRef.ID)
}
