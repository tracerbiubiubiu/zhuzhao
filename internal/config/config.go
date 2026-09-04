package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 全局配置
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	Casbin   CasbinConfig   `mapstructure:"casbin"`
	Log      LogConfig      `mapstructure:"log"`
	Audit    AuditConfig    `mapstructure:"audit"`

	InternalJobs InternalJobsConfig `mapstructure:"internal_jobs"`
}

// InternalJobsConfig 内网回调端点（E-②，16 号 §3）：/internal/jobs/<action_id>
// 走 AK/SK 验签（utils aksk，验 taskrunner 签名，基线 §9）+ 专用网络拓扑。
// Enabled 默认 false——未配置不挂路由（不破坏存量启动；验签密钥未配置而启用则拒绝启动）。
type InternalJobsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	AK      string `mapstructure:"taskrunner_ak"` // 默认 "taskrunner"
	SK      string `mapstructure:"taskrunner_sk"` // env：INTERNAL_JOBS_SK
}

// AuditConfig 审计管道（B11① 判定日志 L2，03-audit-l2 §2.3；P3 拍板 2026-09-03：
// 判定日志异步写。audit_logs 主审计仍为中间件同步写，不在此段管辖）。
type AuditConfig struct {
	PolicyEval PolicyEvalConfig `mapstructure:"policy_eval"`
	Archive    ArchiveConfig    `mapstructure:"archive"`
}

// ArchiveConfig 审计归档（B11②，03-audit-l2 §4；P4 拍板 2026-09-03：本地 JSONL + 卷备份）。
type ArchiveConfig struct {
	RetentionDays int    `mapstructure:"retention_days"` // 默认 180（等保 ≥6 个月口径）
	BatchRows     int    `mapstructure:"batch_rows"`     // 默认 5000（单批导出后删行）
	OutDir        string `mapstructure:"out_dir"`        // JSONL 落盘目录，默认 data/archive
}

// PolicyEvalConfig 判定日志管道参数（零值取默认，见 audit.PolicyEvalConfig.withDefaults）。
type PolicyEvalConfig struct {
	BufferSize    int           `mapstructure:"buffer_size"`
	BatchSize     int           `mapstructure:"batch_size"`
	FlushInterval time.Duration `mapstructure:"flush_interval"`
	RedisKey      string        `mapstructure:"redis_key"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"` // debug / release
	// TrustedProxies 信任的反向代理网段（CIDR/IP，如 Nginx 内网段）。
	// 留空 = 不信任任何代理：ClientIP 取直连地址，X-Forwarded-For 不参与
	// 解析（安全默认，防伪造审计 IP / last_login_ip）。
	TrustedProxies []string `mapstructure:"trusted_proxies"`
}

type DatabaseConfig struct {
	Host             string        `mapstructure:"host"`
	Port             int           `mapstructure:"port"`
	User             string        `mapstructure:"user"`
	Password         string        `mapstructure:"password"`
	DBName           string        `mapstructure:"dbname"`
	MaxOpenConns     int           `mapstructure:"max_open_conns"`
	MaxIdleConns     int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime  time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime  time.Duration `mapstructure:"conn_max_idle_time"`
	ConnectTimeout   time.Duration `mapstructure:"connect_timeout"`
	SSLMode          string        `mapstructure:"sslmode"`
	StatementTimeout time.Duration `mapstructure:"statement_timeout"` // 0 表示不设置
	ApplicationName  string        `mapstructure:"application_name"`
}

func (c *DatabaseConfig) applyDefaults() {
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = 25
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 5
	}
	if c.ConnMaxLifetime == 0 {
		c.ConnMaxLifetime = time.Hour
	}
	if c.ConnMaxIdleTime == 0 {
		c.ConnMaxIdleTime = 30 * time.Minute
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = 5 * time.Second
	}
	if c.SSLMode == "" {
		c.SSLMode = "disable"
	}
	if c.ApplicationName == "" {
		c.ApplicationName = "zhuzhao"
	}
}

// DSN 返回 PostgreSQL 连接字符串。
// B1-3：用 net/url 构造，UserPassword 对密码中的保留字符（@ : / ? # % 等）
// 自动转义——密码经 DB_PASSWORD 环境变量注入，字符不受控。
func (c DatabaseConfig) DSN() string {
	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.User, c.Password),
		Host:     fmt.Sprintf("%s:%d", c.Host, c.Port),
		Path:     c.DBName,
		RawQuery: "sslmode=" + url.QueryEscape(c.SSLMode),
	}
	return u.String()
}

type RedisConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	DB              int           `mapstructure:"db"`
	Password        string        `mapstructure:"password"`
	PoolSize        int           `mapstructure:"pool_size"`
	MinIdleConns    int           `mapstructure:"min_idle_conns"`
	PoolTimeout     time.Duration `mapstructure:"pool_timeout"`
	DialTimeout     time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	MaxRetries      int           `mapstructure:"max_retries"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
}

func (c *RedisConfig) applyDefaults() {
	if c.PoolSize == 0 {
		c.PoolSize = 20
	}
	if c.MinIdleConns == 0 {
		c.MinIdleConns = 5
	}
	if c.PoolTimeout == 0 {
		c.PoolTimeout = 4 * time.Second
	}
	if c.DialTimeout == 0 {
		c.DialTimeout = 5 * time.Second
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = 3 * time.Second
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = 3 * time.Second
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = 2
	}
	if c.ConnMaxIdleTime == 0 {
		c.ConnMaxIdleTime = 5 * time.Minute
	}
}

// Addr 返回 Redis 地址
func (c RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type JWTConfig struct {
	Secret     string        `mapstructure:"secret"`
	AccessTTL  time.Duration `mapstructure:"access_ttl"`
	RefreshTTL time.Duration `mapstructure:"refresh_ttl"`
}

type CasbinConfig struct {
	Model string `mapstructure:"model"`
}

type LogConfig struct {
	Level      string `mapstructure:"level"`
	Dir        string `mapstructure:"dir"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
}

// Load 加载配置文件
func Load(path string) (*Config, error) {
	viper.SetConfigFile(path)

	// 环境变量覆盖（敏感配置通过环境变量注入）
	viper.SetEnvPrefix("APP")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// 环境变量绑定（AutomaticEnv 对 nested key 不自动映射，需显式 BindEnv）
	viper.BindEnv("server.mode", "APP_SERVER_MODE")
	viper.BindEnv("jwt.secret", "JWT_SECRET")
	viper.BindEnv("database.password", "DB_PASSWORD")
	viper.BindEnv("redis.password", "REDIS_PASSWORD")
	viper.BindEnv("internal_jobs.taskrunner_sk", "INTERNAL_JOBS_SK")

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg.Database.applyDefaults()
	cfg.Redis.applyDefaults()

	if cfg.Audit.Archive.RetentionDays <= 0 {
		cfg.Audit.Archive.RetentionDays = 180
	}
	if cfg.Audit.Archive.BatchRows <= 0 {
		cfg.Audit.Archive.BatchRows = 5000
	}
	if cfg.Audit.Archive.OutDir == "" {
		cfg.Audit.Archive.OutDir = "data/archive"
	}

	if cfg.InternalJobs.Enabled {
		if cfg.InternalJobs.SK == "" {
			return nil, fmt.Errorf("internal_jobs.enabled=true 但 taskrunner_sk 未配置（env INTERNAL_JOBS_SK）——验签端点不允许裸奔")
		}
		if cfg.InternalJobs.AK == "" {
			cfg.InternalJobs.AK = "taskrunner"
		}
	}

	return &cfg, nil
}
