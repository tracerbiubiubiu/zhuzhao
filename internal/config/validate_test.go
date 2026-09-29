package config

import "testing"

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
			Database: DatabaseConfig{Host: "h", Port: 5432, DBName: "d"},
			Redis:    RedisConfig{Host: "r"},
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
