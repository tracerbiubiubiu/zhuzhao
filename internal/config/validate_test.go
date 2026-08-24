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
