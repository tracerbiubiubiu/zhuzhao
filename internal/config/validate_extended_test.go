package config

import (
	"testing"
	"time"
)

// TestConfig_Validate_Extended B4-6：TTL/端口/必填项扩展校验
func TestConfig_Validate_Extended(t *testing.T) {
	valid := func() *Config {
		return &Config{
			Server:   ServerConfig{Port: 33333},
			Database: DatabaseConfig{Host: "localhost", Port: 5432, DBName: "zhuzhao"},
			Redis:    RedisConfig{Host: "localhost"},
			JWT:      JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456", AccessTTL: 30 * time.Minute, RefreshTTL: 168 * time.Hour},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"ok", func(c *Config) {}, false},
		{"access_ttl_zero", func(c *Config) { c.JWT.AccessTTL = 0 }, true},
		{"refresh_ttl_zero", func(c *Config) { c.JWT.RefreshTTL = 0 }, true},
		{"access_gt_refresh", func(c *Config) { c.JWT.AccessTTL = 200 * time.Hour }, true},
		{"server_port_zero", func(c *Config) { c.Server.Port = 0 }, true},
		{"server_port_over", func(c *Config) { c.Server.Port = 70000 }, true},
		{"db_port_over", func(c *Config) { c.Database.Port = 70000 }, true},
		{"db_host_empty", func(c *Config) { c.Database.Host = "" }, true},
		{"db_name_empty", func(c *Config) { c.Database.DBName = "" }, true},
		{"redis_host_empty", func(c *Config) { c.Redis.Host = "" }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid()
			tt.mutate(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
