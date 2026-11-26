package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func combinedB1B3Config(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	outboundPath, callbackPath := filepath.Join(dir, "outbound"), filepath.Join(dir, "callback")
	require.NoError(t, os.WriteFile(outboundPath, []byte(strings.Repeat("outbound-", 5)+"\n"), 0600))
	require.NoError(t, os.WriteFile(callbackPath, []byte(strings.Repeat("callback-", 5)+"\n"), 0600))
	return &Config{
		Server:       ServerConfig{Mode: "debug", Port: 33333},
		Database:     DatabaseConfig{Host: "127.0.0.1", Port: 5432, DBName: "synthetic"},
		Redis:        RedisConfig{Host: "127.0.0.1"},
		JWT:          JWTConfig{Secret: strings.Repeat("synthetic-", 4), AccessTTL: time.Minute, RefreshTTL: time.Hour},
		Gateway:      GatewayConfig{AK: "legacy-gateway", SK: "gateway-key"},
		InternalJobs: InternalJobsConfig{Enabled: true, AK: "taskrunner-callback", SK: "taskrunner-callback-key"},
		Taskrunner:   TaskrunnerConfig{BaseURL: "http://127.0.0.1:5001", AK: "core-to-taskrunner", SK: "taskrunner-outbound-key"},
		B3: B3Config{
			Enabled: true, ALBaseURL: "http://127.0.0.1:5002", ALCommandAK: "b3-command", ALCommandSK: "b3-command-key",
			SourceAK: "b3-source", SourceSK: "b3-source-key", TypeName: "sample", Generation: "1",
			ConsumerID: "synthetic-consumer", RuleID: "synthetic-rule-id", RuleVersion: "1",
			ReadGrantRef: "synthetic-grant-id", ReadGrantRev: "1", ReadSubject: "synthetic-reader", ReadFields: []string{"name"},
		},
		ReferenceAssets: ReferenceAssetsConfig{
			Enabled: true, ExtensionID: "reference.assets", Profile: "synthetic.asset-directory.v1", ConfigRevision: "synthetic-v1",
			SourceID: "ref-assets-v1", AppOrigin: "http://127.0.0.1:5003", Timeout: time.Second, ReaderRoles: []string{"reader"},
			OutboundAK: "reference-outbound", OutboundKeyFile: outboundPath, CallbackAK: "reference-callback", CallbackKeyFile: callbackPath,
		},
	}
}

func TestCombinedB1B3RejectsReservedGatewayPrefixes(t *testing.T) {
	for _, tc := range []struct {
		prefix, rejected         string
		b1, b3, upstreamDisabled bool
	}{
		{"/internal/activelist", "b3: gateway prefix", true, true, false},
		{"/al/..", "b3: gateway prefix", false, true, false},
		{"/al/../internal/activelist", "b3: gateway prefix", true, true, false},
		{"/internal//activelist", "b3: gateway prefix", true, true, false},
		{"/internal/activelist/changes/push", "b3: gateway prefix", false, true, false},
		{"/internal/jobs/status", "b3: gateway prefix", true, true, true},
		{"/internal/jobs/status/child", "b3: gateway prefix", false, true, false},
		{"/internal/activelist", "", true, false, false},
		{"/extensions/reference-assets", "reference_assets: gateway prefix", true, true, false},
		{"/al/../extensions/reference-assets", "reference_assets: gateway prefix", true, true, false},
		{"/extensions/reference-assets", "", false, true, false},
		{"/al", "", true, true, false},
	} {
		t.Run(tc.prefix+"/"+tc.rejected, func(t *testing.T) {
			cfg := combinedB1B3Config(t)
			cfg.ReferenceAssets.Enabled, cfg.B3.Enabled = tc.b1, tc.b3
			cfg.Gateway.Upstreams = []GatewayUpstreamConfig{{Prefix: tc.prefix, Target: "http://127.0.0.1:5004", Disabled: tc.upstreamDisabled}}
			err := cfg.Validate()
			if tc.rejected == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.rejected)
		})
	}
}

func TestCombinedB1B3RejectsReusedCallerIDs(t *testing.T) {
	for _, ak := range []string{"b3-source", "b3-command", "taskrunner-callback", "core-to-taskrunner"} {
		t.Run(ak, func(t *testing.T) {
			cfg := combinedB1B3Config(t)
			require.NoError(t, cfg.Validate())
			for _, refAK := range []*string{&cfg.ReferenceAssets.OutboundAK, &cfg.ReferenceAssets.CallbackAK} {
				original := *refAK
				*refAK = ak
				require.ErrorContains(t, cfg.Validate(), "reference_assets and b3 require distinct service credentials")
				cfg.B3.Enabled = false
				require.NoError(t, cfg.Validate())
				cfg.B3.Enabled = true
				*refAK = original
			}
		})
	}
}

func TestCombinedB1B3RejectsReusedSigningKeys(t *testing.T) {
	for _, tc := range []struct {
		name   string
		setKey func(*Config, string)
	}{
		{"source", func(cfg *Config, key string) { cfg.B3.SourceSK = key }},
		{"command", func(cfg *Config, key string) { cfg.B3.ALCommandSK = key }},
		{"taskrunner callback", func(cfg *Config, key string) { cfg.InternalJobs.SK = key }},
		{"taskrunner outbound", func(cfg *Config, key string) { cfg.Taskrunner.SK = key }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := combinedB1B3Config(t)
			peer, _, err := cfg.OpenReferenceAssets()
			require.NoError(t, err)
			require.NotNil(t, peer)
			for _, key := range []string{strings.Repeat("outbound-", 5), strings.Repeat("callback-", 5)} {
				tc.setKey(cfg, key)
				peer, callbackKey, err := cfg.OpenReferenceAssets()
				require.ErrorContains(t, err, "reference_assets and b3 require distinct service credentials")
				require.Nil(t, peer)
				require.Empty(t, callbackKey)
			}
			cfg.B3.Enabled = false
			_, _, err = cfg.OpenReferenceAssets()
			require.NoError(t, err)
			cfg.ReferenceAssets.Enabled, cfg.B3.Enabled = false, true
			cfg.ReferenceAssets.OutboundKeyFile, cfg.ReferenceAssets.CallbackKeyFile = "", ""
			peer, callbackKey, err := cfg.OpenReferenceAssets()
			require.NoError(t, err)
			require.Nil(t, peer)
			require.Empty(t, callbackKey)
		})
	}
}
