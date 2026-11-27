//go:build integration

package risk_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func TestRiskDetailUsesCurrentSourceMirror(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	suffix := uniqueSuffix()
	source := seedBasisSource(t)
	old := baseObservation(suffix, "time-newer", "10")
	old.SourceRef = source
	old.SourceItemKey = "current-source-" + suffix

	send := func(observation model.FindingObservationV1) model.RiskReportItem {
		t.Helper()
		res, err := svc.ReportFromSource(ctx, model.RiskReportRequest{
			Observations: []model.FindingObservationV1{observation},
		}, source)
		require.NoError(t, err)
		return res.Items[0]
	}
	first := send(old)
	require.Equal(t, "accepted", first.Result)
	advanced := old
	advanced.EventID = "sequence-newer-" + suffix
	advanced.SourceSequence = "11"
	advanced.ObservedAt = old.ObservedAt.Add(-time.Minute)
	advanced.SourceSeverity = &model.RiskSourceSeverity{Scheme: "provider-five-level", Value: "2"}
	current := send(advanced)
	require.Equal(t, "accepted", current.Result)
	require.Equal(t, first.RiskRef, current.RiskRef)

	detail, err := svc.Detail(ctx, current.RiskRef.ID, userM)
	require.NoError(t, err)
	var summary struct {
		IngestID string `json:"ingest_id"`
		Sequence string `json:"source_sequence"`
	}
	require.NoError(t, json.Unmarshal(detail.SourceSummary, &summary))
	require.Equal(t, current.IngestID, summary.IngestID)
	require.Equal(t, "11", summary.Sequence)
	require.Equal(t, advanced.SourceSeverity, detail.View.SourceSeverity)
}

func TestRiskAssessmentNonMitigationCannotAuthorizeDispatch(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	suffix := uniqueSuffix()
	observation := baseObservation(suffix, "treatment", "1")
	observation.SourceRef = seedBasisSource(t)
	report, err := svc.ReportFromSource(ctx, model.RiskReportRequest{
		Observations: []model.FindingObservationV1{observation},
	}, observation.SourceRef)
	require.NoError(t, err)
	riskID := report.Items[0].RiskRef.ID

	deadline := time.Now().Add(24 * time.Hour).UTC()
	for _, step := range []struct {
		treatment string
		revision  string
	}{
		{"transfer", "1"}, {"avoid", "2"}, {"accept", "3"}, {"mitigate", "4"},
	} {
		result, err := svc.Assess(ctx, model.RiskAssessRequest{
			OperationID: "dispatch-strategy-" + suffix + "-" + step.treatment,
			ID:          riskID, ExpectedRevision: step.revision, Level: "medium",
			Treatment: step.treatment, AssessmentNote: "Synthetic treatment decision", Deadline: &deadline,
		}, userM)
		require.NoError(t, err)
		require.Equal(t, "assessed", result.Status)
		if step.treatment == "mitigate" {
			require.NotEmpty(t, currentBasis(t, svc, riskID))
			require.NotEmpty(t, lockBasis(t, svc, riskID))
		} else {
			require.Empty(t, currentBasis(t, svc, riskID), step.treatment)
			require.Empty(t, lockBasis(t, svc, riskID), step.treatment)
		}
	}
}
