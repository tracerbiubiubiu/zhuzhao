//go:build integration

package risk_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
)

func TestRiskFeedWithoutComparableSequenceUsesObservationTime(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	source := "mock-scan-time-" + uniqueSuffix()
	_, err := testPool.Exec(ctx, `
INSERT INTO risk_sources(source_ref, display_name, fingerprint_scope, ordered, mapping_ref, allowed_org_ids, default_org_id, enabled)
VALUES ($1,'Synthetic scan time feed',$1,false,$2,ARRAY[$3::bigint],$3,true)`, source, sourceMapping, orgGID)
	require.NoError(t, err)

	first := baseObservation(uniqueSuffix(), "current", "10")
	first.SourceRef = source
	first.ObservedAt = first.ObservedAt.Add(500 * time.Nanosecond)
	report := func(t *testing.T, obs model.FindingObservationV1) model.RiskReportItem {
		t.Helper()
		res, err := svc.ReportFromSource(ctx, model.RiskReportRequest{Observations: []model.FindingObservationV1{obs}}, source)
		require.NoError(t, err)
		return res.Items[0]
	}

	applied := report(t, first)
	require.Equal(t, "accepted", applied.Result)
	require.NotNil(t, applied.RiskRef)
	currentHash := mirrorHashOf(t, source, first.SourceItemKey)

	stale := first
	stale.EventID = "older"
	stale.SourceSequence = ""
	stale.ObservedAt = first.ObservedAt.Add(-time.Minute)
	stale.ObservationKind = "withdrawn"
	t.Run("older", func(t *testing.T) {
		item := report(t, stale)
		require.Equal(t, "stale", item.Result)
		require.Equal(t, "held", item.Processing)
		require.Equal(t, "stale_observed_at", item.SafeReason)
		require.Nil(t, item.RiskRef)
		require.Equal(t, currentHash, mirrorHashOf(t, source, first.SourceItemKey))
		replay := report(t, stale)
		require.Equal(t, "duplicate", replay.Result)
		require.Equal(t, item.IngestID, replay.IngestID)
		require.Equal(t, item.SafeReason, replay.SafeReason)
	})

	equal := first
	equal.EventID = "equal"
	equal.SourceSequence = ""
	equal.ObservedAt = first.ObservedAt.Add(200 * time.Nanosecond)
	equal.ObservationKind = "deleted"
	t.Run("equal at database precision", func(t *testing.T) {
		item := report(t, equal)
		require.Equal(t, "held", item.Result)
		require.Equal(t, "held", item.Processing)
		require.Equal(t, "ambiguous_observed_at", item.SafeReason)
		require.Nil(t, item.RiskRef)
		require.Equal(t, currentHash, mirrorHashOf(t, source, first.SourceItemKey))
		replay := report(t, equal)
		require.Equal(t, "duplicate", replay.Result)
		require.Equal(t, item.IngestID, replay.IngestID)
		require.Equal(t, item.SafeReason, replay.SafeReason)
		receipt, err := svc.IngestReceipt(ctx, item.IngestID, userM)
		require.NoError(t, err)
		require.Equal(t, item.Result, receipt.Result)
		require.Equal(t, item.SafeReason, receipt.SafeReason)
	})
	history, err := svc.History(ctx, applied.RiskRef.ID, 1, 50, userM)
	require.NoError(t, err)
	require.Len(t, history.Items, 1, "late and ambiguous events do not add risk history")

	advance := first
	advance.EventID = "newer"
	advance.SourceSequence = ""
	advance.ObservedAt = first.ObservedAt.Add(time.Minute)
	next := report(t, advance)
	require.Equal(t, "accepted", next.Result)
	require.Equal(t, applied.RiskRef, next.RiskRef)
	nextHash := mirrorHashOf(t, source, first.SourceItemKey)
	require.NotEqual(t, currentHash, nextHash)

	// Existing mirror rows permit a null observation time.
	_, err = testPool.Exec(ctx, `UPDATE risk_source_records SET observed_at=NULL WHERE source_ref=$1`, source)
	require.NoError(t, err)
	unknown := advance
	unknown.EventID = "missing-baseline"
	unknown.ObservedAt = advance.ObservedAt.Add(time.Minute)
	item := report(t, unknown)
	require.Equal(t, "held", item.Result)
	require.Equal(t, "missing_observed_at_baseline", item.SafeReason)
	require.Nil(t, item.RiskRef)
	require.Equal(t, nextHash, mirrorHashOf(t, source, first.SourceItemKey))
}

func TestRiskFeedHeldReplayReturnsPersistedReason(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	observation := baseObservation(uniqueSuffix(), "missing-sequence", "")
	req := model.RiskReportRequest{Observations: []model.FindingObservationV1{observation}}
	first, err := svc.ReportFromSource(ctx, req, sourceRef)
	require.NoError(t, err)
	item := first.Items[0]
	require.Equal(t, "held", item.Result)
	require.Equal(t, "missing_sequence", item.SafeReason)
	require.Nil(t, item.RiskRef)

	replay, err := svc.ReportFromSource(ctx, req, sourceRef)
	require.NoError(t, err)
	require.Equal(t, item.IngestID, replay.Items[0].IngestID)
	require.Equal(t, "duplicate", replay.Items[0].Result)
	require.Equal(t, "held", replay.Items[0].Processing)
	require.Equal(t, item.SafeReason, replay.Items[0].SafeReason)
	require.Nil(t, replay.Items[0].RiskRef)
}

func TestRiskScanWithoutOwnerIsHiddenFromOutOfScopeAssessment(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	suffix := uniqueSuffix()
	observation := baseObservation(suffix, "scan", "1")
	observation.SourceItemKey = "scan-assess-" + suffix
	res, err := svc.ReportFromSource(ctx, model.RiskReportRequest{Observations: []model.FindingObservationV1{observation}}, sourceRef)
	require.NoError(t, err)
	item := res.Items[0]
	require.Equal(t, "accepted", item.Result)
	require.NotNil(t, item.RiskRef)

	req := model.RiskAssessRequest{
		OperationID: "scan-assess-" + suffix, ID: item.RiskRef.ID, ExpectedRevision: "1",
		Level: "medium", Treatment: "mitigate", AssessmentNote: "Verified from synthetic evidence",
	}
	_, err = svc.Assess(ctx, req, userN)
	require.ErrorIs(t, err, risk.ErrRiskNotFound)
	_, err = svc.OperationResult(ctx, req.OperationID, userN)
	require.ErrorIs(t, err, risk.ErrRiskNotFound)

	assessed, err := svc.Assess(ctx, req, userM)
	require.NoError(t, err)
	require.Equal(t, "assessed", assessed.Status)
	require.Equal(t, int64(2), assessed.Risk.Revision)
}
