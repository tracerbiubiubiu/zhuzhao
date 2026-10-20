package storage

import (
	"fmt"
	"strings"
	"time"
)

// StorageConfig storage: 配置段（05 号 A.2 出厂默认）。
// Enabled 双态对齐 compat_aksk 先例：默认 false 不装配客户端（零现栈影响），
// 启用而字段非法在 Validate fail-fast 拒启（虚标纪律——不静默兜底）。
type StorageConfig struct {
	Enabled                 bool     `mapstructure:"enabled"`
	Endpoint                string   `mapstructure:"endpoint"`
	Region                  string   `mapstructure:"region"`
	Bucket                  string   `mapstructure:"bucket"`
	AccessKey               string   `mapstructure:"access_key"`
	SecretKey               string   `mapstructure:"secret_key"`
	UseSSL                  bool     `mapstructure:"use_ssl"`
	PresignUploadTTL        string   `mapstructure:"presign_upload_ttl"`
	MaxUploadBytes          int64    `mapstructure:"max_upload_bytes"`
	MaxAttachmentsPerTicket int      `mapstructure:"max_attachments_per_ticket"`
	MaxPresignBatch         int      `mapstructure:"max_presign_batch"`
	AllowedMIMETypes        []string `mapstructure:"allowed_mime_types"`
	BlockedMIMETypes        []string `mapstructure:"blocked_mime_types"`
	InlineMIMEPrefixes      []string `mapstructure:"inline_mime_prefixes"`
	OrphanGCDays            int      `mapstructure:"orphan_gc_days"`
	// FileCookieSecure 文件通道 cookie Secure 属性（A.11「生产 Secure」——
	// HTTPS 网关部署置 true；默认 false 内网 HTTP 兼容。S5 评审 P2-4）
	FileCookieSecure bool `mapstructure:"file_cookie_secure"`
}

// MaxUploadBytesCeil 上传上限代码天花板（导出供 B5-6 catalog 硬底线校验）（A.2 注：KV 可覆盖至天花板，
// 天花板本身不可配——硬底线纪律）。
const MaxUploadBytesCeil = 100 << 20

// ApplyDefaults 出厂默认（缺省项补齐；显式配置优先——Load 后处理调用）。
func (c *StorageConfig) ApplyDefaults() {
	if c.Region == "" {
		c.Region = "us-east-1"
	}
	if c.PresignUploadTTL == "" {
		c.PresignUploadTTL = "15m"
	}
	if c.MaxUploadBytes <= 0 {
		c.MaxUploadBytes = 10 << 20
	}
	if c.MaxAttachmentsPerTicket <= 0 {
		c.MaxAttachmentsPerTicket = 20
	}
	if c.MaxPresignBatch <= 0 {
		c.MaxPresignBatch = 20
	}
	if len(c.AllowedMIMETypes) == 0 {
		c.AllowedMIMETypes = []string{
			"image/jpeg", "image/png", "image/webp", "image/gif",
			"application/pdf", "text/plain", "application/zip",
			"application/msword", "application/vnd.openxmlformats-officedocument.*",
		}
	}
	if len(c.BlockedMIMETypes) == 0 {
		c.BlockedMIMETypes = []string{"text/html", "image/svg+xml"}
	}
	if len(c.InlineMIMEPrefixes) == 0 {
		c.InlineMIMEPrefixes = []string{"image/"}
	}
	if c.OrphanGCDays <= 0 {
		c.OrphanGCDays = 7
	}
}

// Validate 启用态校验（fail-fast；disabled 直接通过）。
func (c *StorageConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	for k, v := range map[string]string{
		"endpoint":   c.Endpoint,
		"bucket":     c.Bucket,
		"access_key": c.AccessKey,
		"secret_key": c.SecretKey,
	} {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("storage.%s 启用态必填", k)
		}
	}
	ttl, err := c.TTL()
	if err != nil {
		return fmt.Errorf("storage.presign_upload_ttl 非法: %w", err)
	}
	if ttl <= 0 || ttl > time.Hour {
		return fmt.Errorf("storage.presign_upload_ttl 须在 (0, 1h]")
	}
	if c.MaxUploadBytes <= 0 || c.MaxUploadBytes > MaxUploadBytesCeil {
		return fmt.Errorf("storage.max_upload_bytes 须在 (0, %d]", MaxUploadBytesCeil)
	}
	if c.MaxAttachmentsPerTicket > 100 || c.MaxPresignBatch > 100 {
		return fmt.Errorf("storage.max_attachments_per_ticket/max_presign_batch 上限 100")
	}
	blocked := map[string]bool{}
	for _, m := range c.BlockedMIMETypes {
		blocked[m] = true
	}
	for _, m := range c.AllowedMIMETypes {
		if blocked[m] {
			return fmt.Errorf("storage mime %q 同时在白名单与黑名单", m)
		}
	}
	if c.OrphanGCDays < 1 {
		return fmt.Errorf("storage.orphan_gc_days 须 >= 1")
	}
	return nil
}

// TTL 解析 presign TTL。
func (c *StorageConfig) TTL() (time.Duration, error) {
	return time.ParseDuration(c.PresignUploadTTL)
}
