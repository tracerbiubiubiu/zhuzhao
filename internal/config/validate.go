package config

import (
	"fmt"
	"math"
	"strings"
	"time"
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
	if err := c.ReferenceAssets.Validate(); err != nil {
		return err
	}
	for _, upstream := range c.Gateway.Upstreams {
		if c.ReferenceAssets.Enabled && upstream.Prefix != "" && referenceRouteConflict(upstream.Prefix) {
			return fmt.Errorf("reference_assets: gateway prefix conflicts with a reserved reference route")
		}
	}
	if err := c.validateB3(); err != nil {
		return err
	}
	if err := c.RiskIngest.validate(); err != nil {
		return err
	}
	if _, err := c.RiskAssetDirectory.Client(); err != nil {
		return err
	}
	if c.RiskAssetDirectory.Enabled && c.RiskIngest.Enabled &&
		(c.RiskAssetDirectory.AK == c.RiskIngest.AK || c.RiskAssetDirectory.SK == c.RiskIngest.SK) {
		return fmt.Errorf("risk asset directory and risk ingest require distinct credentials")
	}
	if c.RiskReminder.ScanInterval < 0 || (c.RiskReminder.ScanInterval > 0 && !c.RiskRemediationEnabled) {
		return fmt.Errorf("risk_reminder.scan_interval must be nonnegative and requires risk_remediation_enabled when positive")
	}
	if c.RiskRemediationEnabled && !c.Storage.Enabled {
		return fmt.Errorf("risk remediation requires configured attachment storage")
	}
	if err := c.Taskrunner.Maintenance.validateStartup(); err != nil {
		return err
	}
	if err := c.Audit.Archive.ValidateFixedCron(); err != nil {
		return err
	}
	mode := c.Server.Mode
	if mode == "" {
		mode = "debug"
	}
	if mode != "debug" && mode != "release" {
		return fmt.Errorf("server.mode %q invalid: expected debug or release (APP_SERVER_MODE)", mode)
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

	// 服务端启动 fail-fast（自 config.Load 迁入 2026-10-12：Load 保持读+默认值
	// 单一职责——admincli reset-password 只消费连接三节，不应被迫满足服务端
	// 启动前置，否则应急通道被无关检查卡死）
	if c.RateLimit.Enabled && (c.RateLimit.Default.RPS <= 0 || math.IsNaN(c.RateLimit.Default.RPS) || math.IsInf(c.RateLimit.Default.RPS, 0) || c.RateLimit.Default.Burst <= 0) {
		// 限流（07 §2）：enabled 时规则必须显式且为正——静默兜底会让漏配 burst
		// 变成「限流整体失效」的 fail-open（审计 M4），故 fail-fast 拒启
		return fmt.Errorf("rate_limit.enabled=true 但 default.rps/burst 须为正数（当前 %g/%d）——缺省将导致限流整体失效",
			c.RateLimit.Default.RPS, c.RateLimit.Default.Burst)
	}
	if c.RateLimit.Enabled {
		if c.Intel.QueryRatePerMin < 0 {
			return fmt.Errorf("intel.query_rate_per_min 须为非负整数（0 沿用通用限流）")
		}
		for route, rule := range c.RateLimit.Routes {
			if math.IsNaN(rule.RPS) || math.IsInf(rule.RPS, 0) {
				return fmt.Errorf("rate_limit.routes %q 的 rps 须为有限数", route)
			}
		}
	}
	if c.Log.MaxSize < 0 || c.Log.MaxBackups < 0 || c.Log.MaxAge < 0 {
		return fmt.Errorf("log.max_size、max_backups、max_age 须为非负整数")
	}

	if c.InternalJobs.Enabled && c.InternalJobs.SK == "" {
		return fmt.Errorf("internal_jobs.enabled=true 但 taskrunner_sk 未配置（env INTERNAL_JOBS_SK）——验签端点不允许裸奔")
	}
	if c.CompatAKSK.Enabled && len(c.CompatAKSK.Keys) == 0 {
		// 存量协议兼容层（aksk/compat）：启用而密钥环为空 = 每个请求都 401，无意义
		return fmt.Errorf("compat_aksk.enabled=true 但 keys 密钥环为空——存量调用方 AK/SK 未出带同步，验签端点不允许裸奔")
	}
	if c.CompatAKSK.Enabled {
		for ak, sk := range c.CompatAKSK.Keys {
			// 空 SK 条目 = HMAC 空密钥可伪造（验签器侧亦兜底按未知凭据拒），配置层直接拒启
			if sk == "" {
				return fmt.Errorf("compat_aksk.keys 中 AK %q 的 SK 为空——空密钥条目视同未知凭据（fail-closed）", ak)
			}
		}
	}
	if len(c.Gateway.Upstreams) > 0 && (c.Gateway.AK == "" || c.Gateway.SK == "") {
		// 网关反代（批次 B/E13）：配置了上游即必须配出站签名密钥（fail-fast 对齐 internal_jobs）
		return fmt.Errorf("gateway.upstreams 已配置但 ak/sk 缺失（env GATEWAY_AK / GATEWAY_SK）——出站签名必需")
	}

	// P2-2–4（2026-11-20 审计续批）：模块开启态显式非法 duration 拒启——原先
	// 装配端 ParseDuration 失败静默兜底（timeout_scan 兜 5m / captcha 兜 0 后服务层
	// 再兜 5m），配置笔误无启动期报错。空缺省（消费端既定 5m 兜底）与模块停用
	// （不装配=值无消费者）继续可用，不发明值域——仅拒「显式给了但非法」。
	if c.Tickets.TimeoutScanEnabled && c.Tickets.TimeoutScanTick != "" {
		if tick, err := time.ParseDuration(c.Tickets.TimeoutScanTick); err != nil || tick < 0 {
			return fmt.Errorf("tickets.timeout_scan_tick %q 非法：须为非负 duration（如 5m）；0 或留空走消费端 5m 缺省", c.Tickets.TimeoutScanTick)
		}
	}
	if c.CaptchaEnabled && c.CaptchaTTL != "" {
		if ttl, err := time.ParseDuration(c.CaptchaTTL); err != nil || ttl < 0 {
			return fmt.Errorf("captcha_ttl %q 非法：须为非负 duration（如 5m）；0 或留空走消费端 5m 缺省", c.CaptchaTTL)
		}
	}

	// W0a-P0-1：三个内网 SK 对齐 JWT 的 D2-09 纪律——仓库公开的 dev-* 默认值
	// release 拒绝（公开 SK 可签名调 /internal/jobs/callback 补录分支=未授权删
	// 审计，密钥即身份）；debug 放行以便本地零配置（compose 侧已去兜底 ${VAR:?}）。
	if mode == "release" {
		for field, value := range map[string]string{"database.password": c.Database.Password, "redis.password": c.Redis.Password} {
			if isRepoKnownDataSecret(value) {
				return fmt.Errorf("%s must be overridden with a non-empty non-default secret in release mode", field)
			}
		}
		if isRepoKnownSK(c.Gateway.SK) {
			return fmt.Errorf("gateway.sk must be overridden via GATEWAY_SK in release mode (repo-known value rejected)")
		}
		if isRepoKnownSK(c.Taskrunner.SK) {
			return fmt.Errorf("taskrunner.sk must be overridden via TASKRUNNER_SK in release mode (repo-known value rejected)")
		}
		if c.InternalJobs.Enabled && isRepoKnownSK(c.InternalJobs.SK) {
			return fmt.Errorf("internal_jobs.taskrunner_sk must be overridden via INTERNAL_JOBS_SK in release mode (repo-known value rejected)")
		}
		// phase5 S5 对象存储（05 号 A.2；debug 放行 compose 同源默认，release 拒仓库公开值）
		if c.Storage.Enabled && isRepoKnownDataSecret(c.Storage.SecretKey) {
			return fmt.Errorf("storage.secret_key must be overridden via STORAGE_SECRET_KEY in release mode (repo-known value rejected)")
		}
	}
	// phase5 S5 对象存储：disabled 直过，启用态逐项 fail-fast（storage 包内）
	if err := c.Storage.Validate(); err != nil {
		return err
	}
	return nil
}

// repoKnownSKs 仓库内公开的 SK 默认值（compose 历史兜底/configs 示例）——
// release 模式拒绝：公开值不构成秘密性，验签面（网关出站/回调入站）形同虚设。
var repoKnownSKs = map[string]struct{}{
	"dev-gateway-sk":    {},
	"dev-taskrunner-sk": {},
	"dev-callback-sk":   {},
}

func isRepoKnownSK(sk string) bool {
	_, ok := repoKnownSKs[strings.TrimSpace(sk)]
	return ok
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

func isRepoKnownDataSecret(value string) bool {
	switch strings.TrimSpace(value) {
	case "", "zhuzhao_dev", "zhuzhao_rustfs_dev", "zhuzhao_rustfs_bk_dev":
		return true
	default:
		return false
	}
}
