package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/crypto"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jwt"
	redispkg "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/redis"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const defaultDeviceID = "default"

// AuthService 认证服务
type AuthService struct {
	userRepo   *repository.UserRepo
	jwtManager *jwtpkg.Manager
	rdb        *goredis.Client
	scripts    *redispkg.Scripts
	refreshTTL time.Duration
}

// NewAuthService 创建 AuthService
func NewAuthService(
	userRepo *repository.UserRepo,
	jwtManager *jwtpkg.Manager,
	rdb *goredis.Client,
	scripts *redispkg.Scripts,
	jwtCfg config.JWTConfig,
) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		jwtManager: jwtManager,
		rdb:        rdb,
		scripts:    scripts,
		refreshTTL: jwtCfg.RefreshTTL,
	}
}

// Login 登录，签发双 Token
func (s *AuthService) Login(ctx context.Context, req *model.LoginRequest, ip string) (*model.TokenPair, error) {
	if req.EmployeeNo == "" || req.Password == "" {
		return nil, errcode.ErrInvalidParams
	}

	blocked, err := s.scripts.LoginLockIsBlocked(ctx, req.EmployeeNo)
	if err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	if blocked {
		return nil, errcode.ErrAccountLocked
	}

	user, err := s.userRepo.FindByEmployeeNo(ctx, req.EmployeeNo)
	if err != nil {
		if failLogin(ctx, s.scripts, req.EmployeeNo) {
			return nil, errcode.ErrAccountLocked
		}
		if errors.Is(err, errcode.ErrUserNotFound) {
			return nil, errcode.ErrInvalidCredentials
		}
		return nil, err
	}

	if user.Status != 1 {
		if failLogin(ctx, s.scripts, req.EmployeeNo) {
			return nil, errcode.ErrAccountLocked
		}
		return nil, errcode.ErrInvalidCredentials
	}

	if !crypto.CheckPassword(req.Password, user.Password) {
		if failLogin(ctx, s.scripts, req.EmployeeNo) {
			return nil, errcode.ErrAccountLocked
		}
		return nil, errcode.ErrInvalidCredentials
	}

	if err := s.scripts.LoginLockClear(ctx, req.EmployeeNo); err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	if err := clearUserDisabled(ctx, s.rdb, user.ID); err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	if err := s.userRepo.UpdateLastLogin(ctx, user.ID, ip); err != nil {
		return nil, fmt.Errorf("update last login: %w", err)
	}

	return s.issueTokenPair(ctx, user, normalizeDeviceID(req.DeviceID))
}

// Refresh 刷新 Token，RT 轮换
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*model.TokenPair, error) {
	claims, err := s.jwtManager.ParseRefreshToken(refreshToken)
	if err != nil {
		return nil, errcode.ErrRefreshTokenInvalid
	}

	if disabled, err := s.isUserDisabled(ctx, claims.UserID); err != nil {
		return nil, errcode.ErrServiceUnavailable
	} else if disabled {
		return nil, errcode.ErrRefreshTokenInvalid
	}

	user, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, errcode.ErrUserNotFound) {
			return nil, errcode.ErrRefreshTokenInvalid
		}
		return nil, err
	}
	if user.Status != 1 {
		return nil, errcode.ErrRefreshTokenInvalid
	}

	deviceID := normalizeDeviceID(claims.DeviceID)
	key := refreshKey(claims.UserID, deviceID)
	storedHash, err := s.rdb.GetDel(ctx, key).Result()
	if err == goredis.Nil {
		return nil, errcode.ErrRefreshTokenInvalid
	}
	if err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	if storedHash != hashToken(refreshToken) {
		return nil, errcode.ErrRefreshTokenInvalid
	}

	return s.issueTokenPair(ctx, user, deviceID)
}

// Logout 登出，吊销 AT + 删除 RT
func (s *AuthService) Logout(ctx context.Context, accessToken, deviceID string) error {
	claims, err := s.jwtManager.ParseAccessToken(accessToken)
	if err != nil {
		return errcode.ErrTokenInvalid
	}

	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl > 0 {
		blacklistKey := fmt.Sprintf("blacklist:at:%s", claims.JTI)
		if err := s.rdb.Set(ctx, blacklistKey, "1", ttl).Err(); err != nil {
			return errcode.ErrServiceUnavailable
		}
	}

	rtKey := refreshKey(claims.UserID, normalizeDeviceID(deviceID))
	if err := s.rdb.Del(ctx, rtKey).Err(); err != nil {
		return errcode.ErrServiceUnavailable
	}
	return nil
}

// UpdatePassword 用户修改密码（需旧密码验证）
func (s *AuthService) UpdatePassword(ctx context.Context, userID int64, oldPassword, newPassword string) error {
	if oldPassword == "" || newPassword == "" {
		return errcode.ErrInvalidParams
	}
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if !crypto.CheckPassword(oldPassword, user.Password) {
		return errcode.ErrInvalidCredentials
	}
	hash, err := crypto.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if err := s.userRepo.UpdatePassword(ctx, userID, hash, false); err != nil {
		return err
	}
	return nil
}

func (s *AuthService) issueTokenPair(ctx context.Context, user *model.User, deviceID string) (*model.TokenPair, error) {
	at, _, err := s.jwtManager.GenerateAccessToken(user.ID, user.Username, user.MustChangePassword)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}
	rt, _, err := s.jwtManager.GenerateRefreshToken(user.ID, deviceID, s.refreshTTL)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	key := refreshKey(user.ID, deviceID)
	if err := s.rdb.Set(ctx, key, hashToken(rt), s.refreshTTL).Err(); err != nil {
		return nil, errcode.ErrServiceUnavailable
	}

	return &model.TokenPair{
		AccessToken:        at,
		RefreshToken:       rt,
		ExpiresIn:          int(s.jwtManager.AccessTTL().Seconds()),
		MustChangePassword: user.MustChangePassword,
	}, nil
}

func (s *AuthService) isUserDisabled(ctx context.Context, userID int64) (bool, error) {
	n, err := s.rdb.Exists(ctx, fmt.Sprintf("user:disabled:%d", userID)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func failLogin(ctx context.Context, scripts *redispkg.Scripts, employeeNo string) bool {
	blocked, err := scripts.LoginLockIncr(ctx, employeeNo)
	return err == nil && blocked
}

func refreshKey(userID int64, deviceID string) string {
	return fmt.Sprintf("refresh:%d:%s", userID, deviceID)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func normalizeDeviceID(deviceID string) string {
	if deviceID == "" {
		return defaultDeviceID
	}
	return deviceID
}

// ParseTokenExpired 判断 JWT 是否过期（供 handler 映射错误码）
func ParseTokenExpired(err error) bool {
	return errors.Is(err, jwt.ErrTokenExpired)
}
