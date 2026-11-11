package storage

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
)

func defaultAllowed() ([]string, []string) {
	cfg := &storage.StorageConfig{}
	cfg.ApplyDefaults()
	return cfg.AllowedMIMETypes, cfg.BlockedMIMETypes
}

func TestMIMEAllowed(t *testing.T) {
	allowed, blocked := defaultAllowed()

	assert.True(t, MIMEAllowed("image/png", allowed, blocked))
	assert.True(t, MIMEAllowed("IMAGE/PNG", allowed, blocked), "大小写归一")
	// 通配白名单：officedocument.* 覆盖具体子型
	assert.True(t, MIMEAllowed("application/vnd.openxmlformats-officedocument.wordprocessingml.document", allowed, blocked))
	// 硬底线黑名单优先于白名单通配
	assert.False(t, MIMEAllowed("text/html", allowed, blocked))
	assert.False(t, MIMEAllowed("image/svg+xml", allowed, blocked))
	// 显式枚举外拒绝（默认拒绝——非前缀放行）
	assert.False(t, MIMEAllowed("application/x-msdownload", allowed, blocked))
	assert.False(t, MIMEAllowed("", allowed, blocked))
	assert.False(t, MIMEAllowed("image/evildoc", allowed, blocked), "image/ 前缀不构成白名单（显式枚举纪律）")
}

func TestKeyFor(t *testing.T) {
	key, err := KeyFor("avatar", "image/png")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(key, "avatars/"), "avatar 域前缀")
	assert.Regexp(t, `^avatars/\d{4}/\d{2}/[0-9a-f-]{36}\.png$`, key, "uuid 服务端生成+ext 推导自 MIME")

	key2, err := KeyFor("attachment_create", "application/pdf")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(key2, "tickets/"), "建单前附件落工单域前缀")

	_, err = KeyFor("ticket_attachment", "image/png")
	assert.NoError(t, err, "ticket 态 B5-3 已入白名单")
	_, err = KeyFor("evil_purpose", "image/png")
	assert.Error(t, err, "白名单外 purpose 拒绝")

	_, err = KeyFor("avatar", "application/x-evil")
	assert.Error(t, err, "白名单外类型无法推导 ext")
}

func TestPurposePrefixes支持面(t *testing.T) {
	// B5-3 后五 purpose 支持面与 A.7 对表（扩面即守范围）
	assert.Equal(t, map[string]string{
		"avatar":            "avatars/",
		"attachment_create": "tickets/",
		"richtext_create":   "tickets/",
		"ticket_attachment": "tickets/",
		"ticket_richtext":   "tickets/",
	}, purposePrefixes)
}
