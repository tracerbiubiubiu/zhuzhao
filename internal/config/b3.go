package config

import (
	"fmt"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3protocol"
)

const (
	InternalJobsPrefix = "/internal"
	B3TaskStatusRoute  = "/jobs/status"
	B3SourcePushPath   = "/internal/activelist/changes/push"
)

// B3Config binds one watched source, one subscription and independent service
// identities. The keys are provided by the deployment environment.
type B3Config struct {
	Enabled      bool     `mapstructure:"enabled"`
	ALBaseURL    string   `mapstructure:"al_base_url"`
	ALCommandAK  string   `mapstructure:"al_command_ak"`
	ALCommandSK  string   `mapstructure:"al_command_sk"`
	SourceAK     string   `mapstructure:"source_ak"`
	SourceSK     string   `mapstructure:"source_sk"`
	TypeName     string   `mapstructure:"type_name"`
	Generation   string   `mapstructure:"generation"`
	ConsumerID   string   `mapstructure:"consumer_id"`
	RuleID       string   `mapstructure:"rule_id"`
	RuleVersion  string   `mapstructure:"rule_version"`
	ReadGrantRef string   `mapstructure:"read_grant_ref"`
	ReadGrantRev string   `mapstructure:"read_grant_revision"`
	ReadSubject  string   `mapstructure:"read_subject"`
	ReadFields   []string `mapstructure:"read_fields"`
	EventFields  []string `mapstructure:"event_fields"`
}

func (c *Config) validateB3() error {
	b := c.B3
	if !b.Enabled {
		return nil
	}
	if !c.InternalJobs.Enabled || c.Taskrunner.BaseURL == "" || b.ALBaseURL == "" ||
		b.SourceAK == "" || b.SourceSK == "" || b.ALCommandAK == "" || b.ALCommandSK == "" {
		return fmt.Errorf("b3 requires Taskrunner with callbacks and dedicated ActiveList credentials")
	}
	for _, pair := range [][2]string{
		{c.InternalJobs.AK, c.InternalJobs.SK}, {c.Gateway.AK, c.Gateway.SK},
		{c.Taskrunner.AK, c.Taskrunner.SK}, {c.RiskIngest.AK, c.RiskIngest.SK},
		{b.ALCommandAK, b.ALCommandSK},
	} {
		if b.SourceAK == pair[0] || b.SourceSK == pair[1] {
			return fmt.Errorf("b3 source callback credentials must be distinct")
		}
	}
	if b.ALCommandAK == c.Gateway.AK || b.ALCommandSK == c.Gateway.SK {
		return fmt.Errorf("b3 ActiveList execution credentials must be distinct from the gateway")
	}
	if !b3protocol.ValidType(b.TypeName) || !b3protocol.PositiveNumber(b.Generation) ||
		!b3protocol.ValidID(b.ConsumerID) || !b3protocol.ValidID(b.RuleID) ||
		!b3protocol.PositiveNumber(b.RuleVersion) || !b3protocol.ValidID(b.ReadGrantRef) ||
		!b3protocol.PositiveNumber(b.ReadGrantRev) || b.ReadSubject == "" ||
		!b3protocol.ValidFields(b.ReadFields, 32) ||
		(len(b.EventFields) > 0 && !b3protocol.ValidFields(b.EventFields, 8)) {
		return fmt.Errorf("b3 requires an exact ActiveList generation, subscription and read grant")
	}
	return nil
}

func (c *Config) validateB3ReferenceCredentials(outboundKey, callbackKey string) error {
	ref, b3 := c.ReferenceAssets, c.B3
	if !ref.Enabled || !b3.Enabled {
		return nil
	}
	for _, credential := range [...][2]string{
		{b3.SourceAK, b3.SourceSK},
		{b3.ALCommandAK, b3.ALCommandSK},
		{c.InternalJobs.AK, c.InternalJobs.SK},
		{c.Taskrunner.AK, c.Taskrunner.SK},
	} {
		ak, sk := credential[0], credential[1]
		if ak == ref.OutboundAK || ak == ref.CallbackAK ||
			(sk != "" && (sk == outboundKey || sk == callbackKey)) {
			return fmt.Errorf("reference_assets and b3 require distinct service credentials")
		}
	}
	return nil
}
