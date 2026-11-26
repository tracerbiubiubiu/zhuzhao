//go:build integration

package risk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestRiskFeedBoundSourceCannotWriteAnotherSource(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	observation := baseObservation(uniqueSuffix(), "event-source-boundary", "1")
	observation.SourceItemKey = "source-boundary-" + uniqueSuffix()
	request := model.RiskReportRequest{Observations: []model.FindingObservationV1{observation}}

	_, err := svc.ReportFromSource(ctx, request, "another-source")
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	var count int
	require.NoError(t, testPool.QueryRow(ctx, `
		SELECT count(*) FROM risk_observations WHERE source_ref = $1 AND event_id = $2
	`, observation.SourceRef, observation.EventID).Scan(&count))
	require.Zero(t, count)

	report, err := svc.ReportFromSource(ctx, request, observation.SourceRef)
	require.NoError(t, err)
	require.Len(t, report.Items, 1)
	require.Equal(t, "applied", report.Items[0].Processing)
	require.NotNil(t, report.Items[0].RiskRef)
}
