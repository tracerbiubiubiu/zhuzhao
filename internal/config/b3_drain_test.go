package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func setB3Drain(t *testing.T, b3 *B3Config) {
	t.Helper()
	v := viper.New()
	v.Set("b3.drain", true)
	require.NoError(t, v.UnmarshalKey("b3", b3))
}

func TestB3DrainIsExclusive(t *testing.T) {
	cfg := combinedB1B3Config(t)
	setB3Drain(t, &cfg.B3)
	require.ErrorContains(t, cfg.Validate(), "b3.enabled and b3.drain")
}

func TestB3DrainRequiresRuntimeIsolation(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		cfg := combinedB1B3Config(t)
		cfg.B3.Enabled = false
		setB3Drain(t, &cfg.B3)
		require.NoError(t, cfg.Validate())
	})
	t.Run("Taskrunner signing key", func(t *testing.T) {
		cfg := combinedB1B3Config(t)
		cfg.B3.Enabled = false
		setB3Drain(t, &cfg.B3)
		cfg.Taskrunner.SK = ""
		require.ErrorContains(t, cfg.Validate(), "b3 requires a Taskrunner outbound signing key")
	})
	t.Run("reserved routes", func(t *testing.T) {
		cfg := combinedB1B3Config(t)
		cfg.B3.Enabled = false
		setB3Drain(t, &cfg.B3)
		cfg.Gateway.Upstreams = []GatewayUpstreamConfig{{Prefix: "/internal/activelist", Target: "http://127.0.0.1:5004"}}
		require.ErrorContains(t, cfg.Validate(), "b3: gateway prefix")
	})
	t.Run("reference keys", func(t *testing.T) {
		cfg := combinedB1B3Config(t)
		cfg.B3.Enabled = false
		setB3Drain(t, &cfg.B3)
		cfg.B3.SourceSK = "outbound-outbound-outbound-outbound-outbound-"
		_, _, err := cfg.OpenReferenceAssets()
		require.ErrorContains(t, err, "reference_assets and b3 require distinct service credentials")
	})
}
