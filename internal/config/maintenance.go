package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// MaintenanceConfig is explicit deployment authority, never inferred from role
// names, task permissions, request actor strings or browser created_by fields.
type MaintenanceConfig struct {
	MaintainerUserIDs []int64                     `mapstructure:"maintainer_user_ids"`
	Overrides         []MaintenanceOverrideConfig `mapstructure:"overrides"`
	FixedJobs         []MaintenanceFixedJobConfig `mapstructure:"fixed_jobs"`

	// Load also serves connection-only emergency commands. Keep an invalid
	// startup override unusable, but report it only at the server Validate boundary.
	maintainerEnvError error
}

func (c *MaintenanceConfig) setMaintainerEnvironment(raw string) {
	c.MaintainerUserIDs = nil
	c.maintainerEnvError = nil
	if strings.TrimSpace(raw) == "" {
		return
	}
	var ids []int64
	seen := make(map[int64]bool)
	for _, part := range strings.Split(raw, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 || seen[id] {
			c.maintainerEnvError = fmt.Errorf("TASK_MAINTAINER_USER_IDS requires unique positive stable user IDs")
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	c.MaintainerUserIDs = ids
}

func (c MaintenanceConfig) validateStartup() error {
	if c.maintainerEnvError != nil {
		return c.maintainerEnvError
	}
	return nil
}

// ValidateFixedCron validates the complete fixed background definition before
// startup. Explicit cron_enabled is its deployment authority; it never grants a
// human caller maintenance rights or permits request parameters to override it.
func (c ArchiveConfig) ValidateFixedCron() error {
	if !c.CronEnabled {
		return nil
	}
	if tick, err := time.ParseDuration(c.CronTick); err != nil || tick <= 0 {
		return fmt.Errorf("audit.archive.cron_tick must be a positive duration when cron is enabled")
	}
	if c.RetentionDays <= 0 || c.RetentionDays > 36500 {
		return fmt.Errorf("audit.archive.retention_days must be in [1, 36500] when cron is enabled")
	}
	if c.BatchRows <= 0 {
		return fmt.Errorf("audit.archive.batch_rows must be positive when cron is enabled")
	}
	if strings.TrimSpace(c.OutDir) == "" {
		return fmt.Errorf("audit.archive.out_dir is required when cron is enabled")
	}
	if c.FileRetentionDays <= 0 {
		return fmt.Errorf("audit.archive.file_retention_days must be positive when cron is enabled")
	}
	return nil
}

type MaintenanceOverrideConfig struct {
	ID            string `mapstructure:"id"`
	UserID        int64  `mapstructure:"user_id"`
	RetentionDays int    `mapstructure:"retention_days"`
	ValidFrom     string `mapstructure:"valid_from"`
	ValidUntil    string `mapstructure:"valid_until"`
	Reason        string `mapstructure:"reason"`
}

type MaintenanceFixedJobConfig struct {
	JobID            string `mapstructure:"job_id"`
	DefinitionSHA256 string `mapstructure:"definition_sha256"`
	ValidFrom        string `mapstructure:"valid_from"`
	ValidUntil       string `mapstructure:"valid_until"`
	Reason           string `mapstructure:"reason"`
}
