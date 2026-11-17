package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaintenanceStartupRepair_InvalidGrantDoesNotBlockConnectionConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`server:
  port: 33333
database:
  host: localhost
  port: 5432
  dbname: synthetic
redis:
  host: localhost
jwt:
  secret: synthetic-maintenance-config-secret-123456789
  access_ttl: 15m
  refresh_ttl: 24h
taskrunner:
  maintenance:
    maintainer_user_ids: [7]
`), 0600))
	for _, raw := range []string{"admin", "42,", "42,0", "42,43,not-a-user", "42,42"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("TASK_MAINTAINER_USER_IDS", raw)
			cfg, err := Load(path)
			require.NoError(t, err, "break-glass consumes connection config without server maintenance validation")
			require.Equal(t, "localhost", cfg.Database.Host)
			require.Equal(t, "synthetic", cfg.Database.DBName)
			require.Equal(t, "localhost", cfg.Redis.Host)
			require.Positive(t, cfg.JWT.AccessTTL)
			require.Empty(t, cfg.Taskrunner.Maintenance.MaintainerUserIDs, "an invalid replacement must never retain a partial grant")
			require.ErrorContains(t, cfg.Validate(), "TASK_MAINTAINER_USER_IDS", "the normal server must still fail closed")
		})
	}
}

func TestMaintenanceStartupRepair_FixedCronRejectsInvalidReviewedDefinition(t *testing.T) {
	base := func() *Config {
		return &Config{
			Server:   ServerConfig{Port: 33333},
			Database: DatabaseConfig{Host: "localhost", Port: 5432, DBName: "synthetic"},
			Redis:    RedisConfig{Host: "localhost"},
			JWT:      JWTConfig{Secret: "synthetic-maintenance-config-secret-123456789", AccessTTL: 1, RefreshTTL: 2},
			Audit:    AuditConfig{Archive: ArchiveConfig{CronEnabled: true, CronTick: "30s", RetentionDays: 180, BatchRows: 5000, OutDir: "data/archive", FileRetentionDays: 395}},
		}
	}
	for name, change := range map[string]func(*ArchiveConfig){
		"invalid tick":            func(c *ArchiveConfig) { c.CronTick = "not-a-duration" },
		"zero tick":               func(c *ArchiveConfig) { c.CronTick = "0s" },
		"negative retention":      func(c *ArchiveConfig) { c.RetentionDays = -1 },
		"retention out of bounds": func(c *ArchiveConfig) { c.RetentionDays = 36501 },
		"empty output":            func(c *ArchiveConfig) { c.OutDir = " " },
		"invalid batch":           func(c *ArchiveConfig) { c.BatchRows = -1 },
		"invalid file retention":  func(c *ArchiveConfig) { c.FileRetentionDays = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := base()
			change(&cfg.Audit.Archive)
			require.Error(t, cfg.Validate(), "an enabled fixed maintenance definition must be checked before startup")
		})
	}
	require.NoError(t, base().Validate())
	var disabled ArchiveConfig
	cfg := base()
	cfg.Audit.Archive = disabled
	require.NoError(t, cfg.Validate(), "an unconfigured disabled cron grants no authority and must remain compatible")
}
