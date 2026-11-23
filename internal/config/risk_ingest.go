package config

import (
	"errors"
	"strings"
)

// RiskIngestConfig binds the first risk feed to one service identity and source.
type RiskIngestConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	SourceRef string `mapstructure:"source_ref"`
	AK        string `mapstructure:"ak"`
	SK        string `mapstructure:"sk"`
}

func (c RiskIngestConfig) validate() error {
	if !c.Enabled {
		return nil
	}
	if strings.TrimSpace(c.SourceRef) == "" {
		return errors.New("risk_ingest.source_ref is required when enabled")
	}
	if strings.TrimSpace(c.AK) == "" {
		return errors.New("risk_ingest.ak is required when enabled")
	}
	if strings.TrimSpace(c.SK) == "" {
		return errors.New("risk_ingest.sk is required when enabled")
	}
	return nil
}
