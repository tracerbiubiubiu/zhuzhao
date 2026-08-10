package service

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// AuthService 认证服务
type AuthService struct {
	userRepo   *repository.UserRepo
	jwtManager *jwt.Manager
	rdb        *redis.Client
}

// NewAuthService 创建 AuthService
func NewAuthService(userRepo *repository.UserRepo, jwtManager *jwt.Manager, rdb *redis.Client) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		jwtManager: jwtManager,
		rdb:        rdb,
	}
}

// Login 登录，签发双 Token
func (s *AuthService) Login(ctx context.Context, req *model.LoginRequest, ip string) (*model.TokenPair, error) {
	return nil, fmt.Errorf("not implemented")
}

// Refresh 刷新 Token，RT 轮换
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*model.TokenPair, error) {
	return nil, errcode.ErrRefreshTokenInvalid
}

// Logout 登出，吊销 AT + 删除 RT
func (s *AuthService) Logout(ctx context.Context, accessToken, deviceID string) error {
	return fmt.Errorf("not implemented")
}

// ListDevices 查询用户活跃设备列表
func (s *AuthService) ListDevices(ctx context.Context, userID string) ([]model.DeviceInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

// KickDevice 踢出指定设备
func (s *AuthService) KickDevice(ctx context.Context, userID, deviceID string) error {
	return fmt.Errorf("not implemented")
}
