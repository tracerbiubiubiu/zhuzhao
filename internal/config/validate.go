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

// repoKnownSecrets 仓库内公开的默认值——release 模式拒绝（HS256 下密钥即一切，
// 仓库公开值不构成任何秘密性；debug 允许以便本地零配置启动）
var repoKnownSecrets = map[string]struct{}{
	"dev-only-0123456789abcdef0123456789abcdef0123456789abcdef": {},
}

// Validate 校验配置（B4-6 扩展：除 jwt.secret 外覆盖 TTL/端口/必填项——
// 原仅校验 secret，TTL 漏配为 0 时 token 签发即过期且无启动期报错，排障成本高）
func (c *Config) Validate() error {
	mode := c.Server.Mode
	if mode == "" {
		mode = "debug"
	}
	if err := c.JWT.validate(mode); err != nil {
		return err
	}

	// TTL：>0 且 AT ≤ RT（AT 比 RT 长属配置错误）
	if c.JWT.AccessTTL <= 0 {
		return fmt.Errorf("jwt.access_ttl must be positive")
	}
	if c.JWT.RefreshTTL <= 0 {
		return fmt.Errorf("jwt.refresh_ttl must be positive")
	}
	if c.JWT.AccessTTL > c.JWT.RefreshTTL {
		return fmt.Errorf("jwt.access_ttl must not exceed jwt.refresh_ttl")
	}

	// 端口范围
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be in (0, 65535]")
	}
	if c.Database.Port <= 0 || c.Database.Port > 65535 {
		return fmt.Errorf("database.port must be in (0, 65535]")
	}

	// 必填项（缺失时启动报明确错误，优于 Ping 失败的间接报错）
	if c.Database.Host == "" {
		return fmt.Errorf("database.host is required")
	}
	if c.Database.DBName == "" {
		return fmt.Errorf("database.dbname is required")
	}
	if c.Redis.Host == "" {
		return fmt.Errorf("redis.host is required")
	}
	return nil
}

func (c *JWTConfig) validate(mode string) error {
	secret := strings.TrimSpace(c.Secret)
	if secret == "" {
		return fmt.Errorf("jwt.secret is required")
	}
	// D2-09：已知弱密钥无条件拒绝（所有 mode）——原仅 release 拒绝，
	// debug（默认 mode）+ 公开弱密钥即可启动，裸机部署防线不对称
	if isWeakJWTSecret(secret) {
		return fmt.Errorf("jwt.secret must not use default or weak value (rejected in all modes)")
	}
	if mode == "release" {
		if _, known := repoKnownSecrets[secret]; known {
			return fmt.Errorf("jwt.secret must be overridden via JWT_SECRET in release mode (repo-known value rejected)")
		}
		if len(secret) < minJWTSecretLenRelease {
			return fmt.Errorf("jwt.secret must be at least %d characters in release mode", minJWTSecretLenRelease)
		}
	}
	return nil
}

func isWeakJWTSecret(secret string) bool {
	_, ok := weakJWTSecrets[strings.ToLower(strings.TrimSpace(secret))]
	return ok
}
