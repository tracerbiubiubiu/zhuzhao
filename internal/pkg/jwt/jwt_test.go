package jwt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	return NewManager(config.JWTConfig{
		Secret:    "unit-test-secret-0123456789abcdef",
		AccessTTL: 30 * time.Minute,
	})
}

// F-1 特殊场景：RefreshToken 不得冒充 AccessToken（令牌类型混淆）。
// 修复前：AT/RT 同密钥同算法无 typ 声明，ParseAccessToken 可成功解析 RT，
// 使 RT 绕过 30 分钟时效、登出黑名单与强制改密拦截。
func TestParseAccessToken_RejectsRefreshToken(t *testing.T) {
	m := newTestManager(t)
	rt, _, err := m.GenerateRefreshToken(42, "device-1", 168*time.Hour)
	require.NoError(t, err)

	_, err = m.ParseAccessToken(rt)
	assert.Error(t, err, "refresh token must not be accepted as access token")
}

// F-1 反向场景：AccessToken 不得当作 RefreshToken 走刷新接口
func TestParseRefreshToken_RejectsAccessToken(t *testing.T) {
	m := newTestManager(t)
	at, _, err := m.GenerateAccessToken(42, "alice", false)
	require.NoError(t, err)

	_, err = m.ParseRefreshToken(at)
	assert.Error(t, err, "access token must not be accepted as refresh token")
}

func TestAccessTokenRoundTrip(t *testing.T) {
	m := newTestManager(t)
	at, jti, err := m.GenerateAccessToken(42, "alice", true)
	require.NoError(t, err)

	claims, err := m.ParseAccessToken(at)
	require.NoError(t, err)
	assert.Equal(t, int64(42), claims.UserID)
	assert.Equal(t, "alice", claims.Username)
	assert.Equal(t, jti, claims.JTI)
	assert.True(t, claims.MustChangePassword)
}

func TestRefreshTokenRoundTrip(t *testing.T) {
	m := newTestManager(t)
	rt, _, err := m.GenerateRefreshToken(42, "device-1", time.Hour)
	require.NoError(t, err)

	claims, err := m.ParseRefreshToken(rt)
	require.NoError(t, err)
	assert.Equal(t, int64(42), claims.UserID)
	assert.Equal(t, "device-1", claims.DeviceID)
}

// 密钥不符必须拒绝
func TestParseAccessToken_WrongSecret(t *testing.T) {
	m1 := NewManager(config.JWTConfig{Secret: "secret-aaaaaaaaaaaaaaaaaaaa", AccessTTL: time.Minute})
	m2 := NewManager(config.JWTConfig{Secret: "secret-bbbbbbbbbbbbbbbbbbbb", AccessTTL: time.Minute})
	at, _, err := m1.GenerateAccessToken(1, "u", false)
	require.NoError(t, err)
	_, err = m2.ParseAccessToken(at)
	assert.Error(t, err)
}
