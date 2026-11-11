package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageConfig_Validate(t *testing.T) {
	base := func() *StorageConfig {
		return &StorageConfig{
			Enabled:          true,
			Endpoint:         "http://rustfs:9000",
			Bucket:           "zhuzhao",
			AccessKey:        "ak",
			SecretKey:        "sk",
			PresignUploadTTL: "15m",
			MaxUploadBytes:   10 << 20,
			AllowedMIMETypes: []string{"image/png"},
			BlockedMIMETypes: []string{"text/html"},
			OrphanGCDays:     7,
		}
	}

	t.Run("disabled 直接通过", func(t *testing.T) {
		cfg := &StorageConfig{}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("启用态全参合法", func(t *testing.T) {
		assert.NoError(t, base().Validate())
	})

	t.Run("必填缺失 fail-fast", func(t *testing.T) {
		for _, field := range []string{"endpoint", "bucket", "access_key", "secret_key"} {
			cfg := base()
			switch field {
			case "endpoint":
				cfg.Endpoint = ""
			case "bucket":
				cfg.Bucket = " "
			case "access_key":
				cfg.AccessKey = ""
			case "secret_key":
				cfg.SecretKey = ""
			}
			assert.Error(t, cfg.Validate(), field)
		}
	})

	t.Run("TTL 非法拒绝", func(t *testing.T) {
		cfg := base()
		cfg.PresignUploadTTL = "abc"
		assert.Error(t, cfg.Validate())
		cfg.PresignUploadTTL = "2h" // 超上限
		assert.Error(t, cfg.Validate())
	})

	t.Run("上传上限天花板钳制", func(t *testing.T) {
		cfg := base()
		cfg.MaxUploadBytes = MaxUploadBytesCeil + 1
		assert.Error(t, cfg.Validate())
	})

	t.Run("黑白名单交叠拒绝", func(t *testing.T) {
		cfg := base()
		cfg.AllowedMIMETypes = append(cfg.AllowedMIMETypes, "text/html")
		assert.Error(t, cfg.Validate())
	})
}

func TestStorageConfig_ApplyDefaults(t *testing.T) {
	cfg := &StorageConfig{}
	cfg.ApplyDefaults()
	assert.Equal(t, "us-east-1", cfg.Region)
	assert.Equal(t, "15m", cfg.PresignUploadTTL)
	assert.Equal(t, int64(10<<20), cfg.MaxUploadBytes)
	assert.Equal(t, 7, cfg.OrphanGCDays)
	assert.Contains(t, cfg.BlockedMIMETypes, "text/html", "svg/html 黑名单为硬底线默认")
}

// PresignPut 离线单测：httptest 伪 S3 应答桶检查两请求（HEAD 404→PUT 建桶 200），
// 验证预签名 URL 形态与 Content-Type 入签（判据④的客户端侧锚点）。
func TestMinioClient_PresignPut_CT入签(t *testing.T) {
	var sawBucketPut bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead: // BucketExists → 不存在
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPut: // MakeBucket
			sawBucketPut = true
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer srv.Close()

	cfg := &StorageConfig{
		Enabled: true, Endpoint: srv.URL, Bucket: "zhuzhao",
		AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m",
	}
	cfg.ApplyDefaults()
	c, err := NewMinioClient(cfg)
	require.NoError(t, err)

	pp, err := c.PresignPut(context.Background(), "tickets/2026/10/uuid.png", "image/png", 5*time.Minute)
	require.NoError(t, err)
	assert.True(t, sawBucketPut, "首签触发幂等建桶")

	u, err := url.Parse(pp.URL)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(u.Path, "/zhuzhao/tickets/2026/10/uuid.png"))
	signedHeaders := u.Query().Get("X-Amz-SignedHeaders")
	assert.Contains(t, signedHeaders, "content-type", "Content-Type 必须钉入签名")
	assert.Equal(t, "image/png", pp.Headers["Content-Type"])
	assert.False(t, pp.ExpiresAt.IsZero())
}

func TestNewMinioClient_未启用拒绝(t *testing.T) {
	_, err := NewMinioClient(&StorageConfig{})
	assert.Error(t, err)
	_, err = NewMinioClient(nil)
	assert.Error(t, err)
}
