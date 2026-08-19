package config

import (
	"fmt"
	"strings"
)

const minJWTSecretLenRelease = 32

var weakJWTSecrets = map[string]struct{}{
	"change-me-in-production": {},
	"changeme":                {},
	"secret":                  {},
	"test-secret":             {},
	"your-secret-key":         {},
}

// Validate 校验配置；release 模式下 JWT 密钥必须足够强。
func (c *Config) Validate() error {
	mode := c.Server.Mode
	if mode == "" {
		mode = "debug"
	}
	return c.JWT.validate(mode)
}

// UsesWeakJWTSecret 是否使用了已知弱密钥（debug 下允许但应告警）。
func (c *Config) UsesWeakJWTSecret() bool {
	return isWeakJWTSecret(c.JWT.Secret)
}

func (c *JWTConfig) validate(mode string) error {
	secret := strings.TrimSpace(c.Secret)
	if secret == "" {
		return fmt.Errorf("jwt.secret is required")
	}
	if isWeakJWTSecret(secret) {
		if mode == "release" {
			return fmt.Errorf("jwt.secret must not use default or weak value in release mode")
		}
		return nil
	}
	if mode == "release" && len(secret) < minJWTSecretLenRelease {
		return fmt.Errorf("jwt.secret must be at least %d characters in release mode", minJWTSecretLenRelease)
	}
	return nil
}

func isWeakJWTSecret(secret string) bool {
	_, ok := weakJWTSecrets[strings.ToLower(strings.TrimSpace(secret))]
	return ok
}
