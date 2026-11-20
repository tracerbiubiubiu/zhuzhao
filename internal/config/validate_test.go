package config

import (
	"math"
	"testing"
)

func TestJWTConfig_validate_release(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{"empty", "", true},
		{"default", "change-me-in-production", true},
		{"short", "abcdefghijklmnopqrstuvwxyz12", true}, // 30 chars
		{"repo-known-dev-default", "dev-only-0123456789abcdef0123456789abcdef0123456789abcdef", true},
		{"ok", "abcdefghijklmnopqrstuvwxyz123456", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &JWTConfig{Secret: tt.secret}
			err := cfg.validate("release")
			if (err != nil) != tt.wantErr {
				t.Fatalf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// D2-09 守护：已知弱密钥在 debug 模式同样拒绝（原 debug 放行——防线不对称，
// 裸机部署 debug+公开弱密钥即可启动，HS256 下密钥即一切）
func TestJWTConfig_validate_debug_rejects_weak(t *testing.T) {
	for _, weak := range []string{"change-me-in-production", "changeme", "secret", "test-secret", "your-secret-key"} {
		cfg := &JWTConfig{Secret: weak}
		if err := cfg.validate("debug"); err == nil {
			t.Fatalf("debug should reject weak secret %q", weak)
		}
	}
}

// D2-09 守护：仓库公开的 dev 默认值 debug 放行（本地零配置）、release 拒绝
func TestJWTConfig_validate_repo_known_dev_default(t *testing.T) {
	cfg := &JWTConfig{Secret: "dev-only-0123456789abcdef0123456789abcdef0123456789abcdef"}
	if err := cfg.validate("debug"); err != nil {
		t.Fatalf("debug should allow repo dev default: %v", err)
	}
	if err := cfg.validate("release"); err == nil {
		t.Fatal("release should reject repo-known dev default")
	}
}

// W0a-P0-1 守护：三个内网 SK（gateway/taskrunner/internal_jobs）对齐 JWT 的
// D2-09 纪律——仓库公开的 dev-* 默认值 release 拒绝（公开 SK 可签名调
// /internal/jobs/callback 补录分支=未授权删审计，密钥即身份）；debug 放行
// 以便本地零配置。compose 侧同步去兜底（${VAR:?}）。
func TestConfig_validate_repo_known_dev_sk_release_rejected(t *testing.T) {
	base := func() *Config {
		return &Config{
			JWT:      JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456", AccessTTL: 1, RefreshTTL: 2},
			Server:   ServerConfig{Mode: "release", Port: 33333},
			Database: DatabaseConfig{Host: "h", Port: 5432, DBName: "d", Password: "synthetic-database-secret"},
			Redis:    RedisConfig{Host: "r", Password: "synthetic-redis-secret"},
		}
	}
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"gateway-upstream-sk", func(c *Config) {
			c.Gateway = GatewayConfig{AK: "zhuzhao", SK: "dev-gateway-sk", Upstreams: []GatewayUpstreamConfig{{Prefix: "/al", Target: "http://x"}}}
		}},
		{"taskrunner-sk", func(c *Config) {
			c.Taskrunner.SK = "dev-taskrunner-sk"
		}},
		{"internal-jobs-sk", func(c *Config) {
			c.InternalJobs = InternalJobsConfig{Enabled: true, AK: "taskrunner", SK: "dev-callback-sk"}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base()
			tt.mutate(cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("release should reject repo-known dev SK")
			}
		})
	}
}

// debug 模式放行同一批 dev SK（本地零配置启动不破坏）
func TestConfig_validate_dev_sk_debug_allowed(t *testing.T) {
	cfg := &Config{
		JWT:          JWTConfig{Secret: "dev-only-0123456789abcdef0123456789abcdef0123456789abcdef", AccessTTL: 1, RefreshTTL: 2},
		Server:       ServerConfig{Mode: "debug", Port: 33333},
		Database:     DatabaseConfig{Host: "h", Port: 5432, DBName: "d"},
		Redis:        RedisConfig{Host: "r"},
		Gateway:      GatewayConfig{AK: "zhuzhao", SK: "dev-gateway-sk", Upstreams: []GatewayUpstreamConfig{{Prefix: "/al", Target: "http://x"}}},
		Taskrunner:   TaskrunnerConfig{SK: "dev-taskrunner-sk"},
		InternalJobs: InternalJobsConfig{Enabled: true, AK: "taskrunner", SK: "dev-callback-sk"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("debug should allow repo dev SKs: %v", err)
	}
}

// P2-2–4（2026-11-20 审计续批）：开启态显式非法 duration 在 Validate 拒启——
// 原先装配端 ParseDuration 失败静默兜 5m（timeout_scan，app.go）/静默返 0 由
// 服务层兜 5m（captcha，CaptchaService），配置笔误无启动期报错、排障成本高。
// 空缺省（消费端既定 5m 兜底）与模块停用（不装配=值无消费者）继续可用，
// 不发明值域——仅拒「显式给了但解析失败或 ≤0」。
func TestConfig_validate_enabled_gated_durations(t *testing.T) {
	base := func() *Config {
		return &Config{
			JWT:      JWTConfig{Secret: "dev-only-0123456789abcdef0123456789abcdef0123456789abcdef", AccessTTL: 1, RefreshTTL: 2},
			Server:   ServerConfig{Mode: "debug", Port: 33333},
			Database: DatabaseConfig{Host: "h", Port: 5432, DBName: "d"},
			Redis:    RedisConfig{Host: "r"},
		}
	}
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"timeout-scan-enabled-parse-error", func(c *Config) {
			c.Tickets = TicketsConfig{TimeoutScanEnabled: true, TimeoutScanTick: "abc"}
		}, true},
		{"timeout-scan-enabled-zero", func(c *Config) {
			c.Tickets = TicketsConfig{TimeoutScanEnabled: true, TimeoutScanTick: "0s"}
		}, false},
		{"timeout-scan-enabled-negative", func(c *Config) {
			c.Tickets = TicketsConfig{TimeoutScanEnabled: true, TimeoutScanTick: "-5m"}
		}, true},
		{"timeout-scan-enabled-empty-default-ok", func(c *Config) {
			c.Tickets = TicketsConfig{TimeoutScanEnabled: true, TimeoutScanTick: ""}
		}, false},
		{"timeout-scan-enabled-valid-ok", func(c *Config) {
			c.Tickets = TicketsConfig{TimeoutScanEnabled: true, TimeoutScanTick: "5m"}
		}, false},
		{"timeout-scan-disabled-illegal-value-passes", func(c *Config) {
			c.Tickets = TicketsConfig{TimeoutScanEnabled: false, TimeoutScanTick: "abc"}
		}, false},
		{"captcha-enabled-parse-error", func(c *Config) {
			c.CaptchaEnabled, c.CaptchaTTL = true, "abc"
		}, true},
		{"captcha-enabled-zero", func(c *Config) {
			c.CaptchaEnabled, c.CaptchaTTL = true, "0"
		}, false},
		{"captcha-enabled-negative", func(c *Config) {
			c.CaptchaEnabled, c.CaptchaTTL = true, "-1m"
		}, true},
		{"captcha-enabled-empty-default-ok", func(c *Config) {
			c.CaptchaEnabled, c.CaptchaTTL = true, ""
		}, false},
		{"captcha-enabled-valid-ok", func(c *Config) {
			c.CaptchaEnabled, c.CaptchaTTL = true, "5m"
		}, false},
		{"captcha-disabled-illegal-value-passes", func(c *Config) {
			c.CaptchaEnabled, c.CaptchaTTL = false, "abc"
		}, false},
		{"negative-log-size", func(c *Config) { c.Log.MaxSize = -1 }, true},
		{"negative-log-backups", func(c *Config) { c.Log.MaxBackups = -1 }, true},
		{"negative-log-age", func(c *Config) { c.Log.MaxAge = -1 }, true},
		{"negative-intel-limit", func(c *Config) {
			c.RateLimit.Enabled = true
			c.RateLimit.Default = RateLimitRule{RPS: 10, Burst: 20}
			c.Intel.QueryRatePerMin = -1
		}, true},
		{"nonfinite-rate", func(c *Config) {
			c.RateLimit.Enabled = true
			c.RateLimit.Default = RateLimitRule{RPS: math.Inf(1), Burst: 20}
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base()
			tt.mutate(cfg)
			err := cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("开启态显式非法 duration 应拒启，实际通过")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("该用例不应拒启，实际: %v", err)
			}
		})
	}
}
