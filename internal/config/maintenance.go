package config

// MaintenanceConfig is explicit deployment authority, never inferred from role
// names, task permissions, request actor strings or browser created_by fields.
type MaintenanceConfig struct {
	MaintainerUserIDs []int64                     `mapstructure:"maintainer_user_ids"`
	Overrides         []MaintenanceOverrideConfig `mapstructure:"overrides"`
	FixedJobs         []MaintenanceFixedJobConfig `mapstructure:"fixed_jobs"`
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
