package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AccessClaims accessToken 的 payload
type AccessClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	OrgID    string `json:"org_id"`
	DeviceID string `json:"device_id"`
	jwt.RegisteredClaims
}

// RefreshClaims refreshToken 的 payload
type RefreshClaims struct {
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`
	jwt.RegisteredClaims
}

// Manager JWT 签发与解析
type Manager struct {
	secret    []byte
	accessTTL time.Duration
}

// NewManager 创建 JWT Manager
func NewManager(secret string, accessTTL time.Duration) *Manager {
	return &Manager{
		secret:    []byte(secret),
		accessTTL: accessTTL,
	}
}

// GenerateAccessToken 签发 accessToken
func (m *Manager) GenerateAccessToken(userID, username, role, orgID, deviceID string) (string, error) {
	now := time.Now()
	claims := AccessClaims{
		UserID:   userID,
		Username: username,
		Role:     role,
		OrgID:    orgID,
		DeviceID: deviceID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        generateJTI(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// GenerateRefreshToken 签发 refreshToken
func (m *Manager) GenerateRefreshToken(userID, deviceID string, ttl time.Duration) (string, string, error) {
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
