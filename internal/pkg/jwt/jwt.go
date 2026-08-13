package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

// AccessClaims accessToken 的 payload
type AccessClaims struct {
	UserID             int64  `json:"uid,string"`
	Username           string `json:"username"`
	JTI                string `json:"jti"`
	MustChangePassword bool   `json:"mcp,omitempty"` // 首次登录改密标记
	jwt.RegisteredClaims
}

// RefreshClaims refreshToken 的 payload
type RefreshClaims struct {
	UserID   int64  `json:"uid,string"`
	DeviceID string `json:"device_id"`
	jwt.RegisteredClaims
}

// Manager JWT 签发与解析
type Manager struct {
	secret    []byte
	accessTTL time.Duration
}

// NewManager 创建 JWT Manager
func NewManager(cfg config.JWTConfig) *Manager {
	return &Manager{
		secret:    []byte(cfg.Secret),
		accessTTL: cfg.AccessTTL,
	}
}

// GenerateAccessToken 签发 accessToken
func (m *Manager) GenerateAccessToken(userID int64, username string, mustChangePassword bool) (string, string, error) {
	now := time.Now()
	jti := generateJTI()
	claims := AccessClaims{
		UserID:             userID,
		Username:           username,
		JTI:                jti,
		MustChangePassword: mustChangePassword,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        jti,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	return signed, jti, err
}

// GenerateRefreshToken 签发 refreshToken
func (m *Manager) GenerateRefreshToken(userID int64, deviceID string, ttl time.Duration) (string, string, error) {
	now := time.Now()
	jti := generateJTI()
	claims := RefreshClaims{
		UserID:   userID,
		DeviceID: deviceID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        jti,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	return signed, jti, err
}

// ParseAccessToken 解析 accessToken
func (m *Manager) ParseAccessToken(tokenString string) (*AccessClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AccessClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*AccessClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// ParseRefreshToken 解析 refreshToken
func (m *Manager) ParseRefreshToken(tokenString string) (*RefreshClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &RefreshClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*RefreshClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// AccessTTL 返回 AT 的有效期
func (m *Manager) AccessTTL() time.Duration {
	return m.accessTTL
}
