package config

import (
	"fmt"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/riskasset"
)

// RiskAssetDirectoryConfig configures the separate read-only directory identity.
type RiskAssetDirectoryConfig struct {
	Enabled bool          `mapstructure:"enabled"`
	Origin  string        `mapstructure:"origin"`
	AK      string        `mapstructure:"ak"`
	SK      string        `mapstructure:"sk"`
	Timeout time.Duration `mapstructure:"timeout"`
	MaxAge  time.Duration `mapstructure:"max_age"`
}

func (c RiskAssetDirectoryConfig) Client() (riskasset.Port, error) {
	if !c.Enabled {
		return nil, nil
	}
	client, err := riskasset.NewClient(riskasset.Config{
		BaseURL: c.Origin, AK: c.AK, SK: []byte(c.SK), Timeout: c.Timeout, MaxAge: c.MaxAge,
	})
	if err != nil {
		return nil, fmt.Errorf("risk_asset_directory: %w", err)
	}
	return client, nil
}
