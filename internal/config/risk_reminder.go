package config

import "time"

type RiskReminderConfig struct {
	// A positive interval starts the scanner. Its rule comes from the ticket's
	// published definition, independently of this scheduler setting.
	ScanInterval time.Duration `mapstructure:"scan_interval"`
}
