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

func TestJWTConfig_validate_debug_allows_weak(t *testing.T) {
	cfg := &JWTConfig{Secret: "change-me-in-production"}
	if err := cfg.validate("debug"); err != nil {
		t.Fatalf("debug should allow weak secret: %v", err)
	}
}

func TestUsesWeakJWTSecret(t *testing.T) {
	cfg := &Config{JWT: JWTConfig{Secret: "change-me-in-production"}}
	if !cfg.UsesWeakJWTSecret() {
		t.Fatal("expected weak secret")
	}
	cfg.JWT.Secret = "abcdefghijklmnopqrstuvwxyz123456"
	if cfg.UsesWeakJWTSecret() {
		t.Fatal("expected strong secret")
	}
}
