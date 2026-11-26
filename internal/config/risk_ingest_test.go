package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRiskIngestNeedsExplicitSourceAndCredentials(t *testing.T) {
	require.NoError(t, (RiskIngestConfig{}).validate())
	require.ErrorContains(t, (RiskIngestConfig{Enabled: true}).validate(), "source_ref")
	require.ErrorContains(t, (RiskIngestConfig{Enabled: true, SourceRef: "synthetic-findings"}).validate(), "ak")
	require.ErrorContains(t, (RiskIngestConfig{Enabled: true, SourceRef: "synthetic-findings", AK: "synthetic-feed"}).validate(), "sk")
	require.NoError(t, (RiskIngestConfig{
		Enabled: true, SourceRef: "synthetic-findings", AK: "synthetic-feed", SK: "test-fixture",
	}).validate())
}
