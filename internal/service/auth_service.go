package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao-utils/crypto"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	redispkg "github.com/tracerbiubiubiu/zhuzhao-utils/redis"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const defaultDeviceID = "default"

// AuthService 认证服务
type AuthService struct {
	userRepo     *repository.UserRepo
	jwtManager   *jwtpkg.Manager
	rdb          *goredis.Client
	scripts      *redispkg.Scripts
	auditService *AuditService
	refreshTTL   time.Duration
}

// NewAuthService 创建 AuthService
func NewAuthService(
	userRepo *repository.UserRepo,
	jwtManager *jwtpkg.Manager,
	rdb *goredis.Client,
	scripts *redispkg.Scripts,
	auditService *AuditService,
	jwtCfg config.JWTConfig,
) *AuthService {
	return &AuthService{
		userRepo:     userRepo,
		jwtManager:   jwtManager,
		rdb:          rdb,
		scripts:      scripts,
		auditService: auditService,
		refreshTTL:   jwtCfg.RefreshTTL,
	}
}

// Login 登录，签发双 Token
func (s *AuthService) Login(ctx context.Context, req *model.LoginRequest, ip, userAgent string) (*model.TokenPair, error) {
	if req.EmployeeNo == "" || req.Password == "" {
		s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, nil, "", 400)
		return nil, errcode.ErrInvalidParams
	}
	// D2-22：device_id 字符白名单——直接进 Redis 键与 JWT claims，
	// 任意字符可致键不可读/膨胀（长度上限已在 binding max=64）
	if req.DeviceID != "" && !validDeviceID(req.DeviceID) {
		return nil, errcode.ErrInvalidParams
	}

	blocked, err := s.scripts.LoginLockIsBlocked(ctx, req.EmployeeNo)
	if err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	if blocked {
		s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, nil, "", 429)
		return nil, errcode.ErrAccountLocked
	}

	user, err := s.userRepo.FindByEmployeeNo(ctx, req.EmployeeNo)
	if err != nil {
		blocked, lockErr := failLogin(ctx, s.scripts, req.EmployeeNo)
		if lockErr != nil {
			return nil, errcode.ErrServiceUnavailable
		}
		if blocked {
			s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, nil, "", 429)
			return nil, errcode.ErrAccountLocked
		}
		if errors.Is(err, errcode.ErrUserNotFound) {
			// B4-1：dummy bcrypt 拉平时延——工号存在分支会执行 cost=12 比对（数百 ms），
			// 此分支不比对则可通过响应时间差枚举有效工号
			crypto.CheckDummyPassword(req.Password)
			s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, nil, "", 401)
			return nil, errcode.ErrInvalidCredentials
		}
		return nil, err
	}

	if user.Status != 1 {
		blocked, lockErr := failLogin(ctx, s.scripts, req.EmployeeNo)
		if lockErr != nil {
			return nil, errcode.ErrServiceUnavailable
		}
		if blocked {
			s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, nil, "", 429)
			return nil, errcode.ErrAccountLocked
		}
		// D2-20：与「工号不存在」分支（B4-1）对齐——禁用分支不比对密码，
		// 响应时延差可枚举被禁用工号
		crypto.CheckDummyPassword(req.Password)
		s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, nil, "", 401)
		return nil, errcode.ErrInvalidCredentials
	}

	if !crypto.CheckPassword(req.Password, user.Password) {
		blocked, lockErr := failLogin(ctx, s.scripts, req.EmployeeNo)
		if lockErr != nil {
			return nil, errcode.ErrServiceUnavailable
		}
		if blocked {
			s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, nil, "", 429)
			return nil, errcode.ErrAccountLocked
		}
		s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, nil, "", 401)
		return nil, errcode.ErrInvalidCredentials
	}

	if err := s.scripts.LoginLockClear(ctx, req.EmployeeNo); err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	if err := clearUserDisabled(ctx, s.rdb, user.ID); err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	if err := s.userRepo.UpdateLastLogin(ctx, user.ID, ip); err != nil {
		// B4-1：认证已通过但后续失败同样落审计（此前该分支无登录记录）
		s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, &user.ID, user.Username, 500)
		return nil, fmt.Errorf("update last login: %w", err)
	}

	pair, err := s.issueTokenPair(ctx, user, normalizeDeviceID(req.DeviceID))
	if err != nil {
		s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, &user.ID, user.Username, 500)
		return nil, err
	}
	s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, &user.ID, user.Username, 200)
	return pair, nil
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
	if !validDeviceID(deviceID) {
		// D2-22：防御——claims 来自服务端签名令牌，正常不可达；
		// 伪造/历史脏 claims 拒绝刷新
		return nil, errcode.ErrRefreshTokenInvalid
	}
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
	// D2-22：device_id 白名单（与 Login 对齐）
	if deviceID != "" && !validDeviceID(deviceID) {
		return errcode.ErrInvalidParams
	}

	// D2-44⑥：ExpiresAt 理论 nil 防御——本服务签发的 AT 恒带 exp，
	// 仅持有密钥伪造无 exp 令牌可触达（纵深项，nil 时跳过拉黑仅删 RT）
	var ttl time.Duration
	if claims.ExpiresAt != nil {
		ttl = time.Until(claims.ExpiresAt.Time)
	}
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

// UpdatePassword 用户修改密码（需旧密码验证），成功后吊销旧会话并重新签发 Token
func (s *AuthService) UpdatePassword(ctx context.Context, userID int64, oldPassword, newPassword, accessToken, deviceID string) (*model.TokenPair, error) {
	if oldPassword == "" || newPassword == "" {
		return nil, errcode.ErrInvalidParams
	}
	// D2-22：device_id 白名单（与 Login/Logout 对齐——重签 Token 写 Redis 键）
	if deviceID != "" && !validDeviceID(deviceID) {
		return nil, errcode.ErrInvalidParams
	}
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !crypto.CheckPassword(oldPassword, user.Password) {
		return nil, errcode.ErrInvalidCredentials
	}
	// B2-2：新密码与旧密码相同 → 400（02-auth.md 测试用例承诺）；
	// 防止「以为改了密码」实际凭证未变却吊销了全部会话
	if oldPassword == newPassword {
		return nil, errcode.ErrInvalidParams
	}
	hash, err := crypto.HashPassword(newPassword)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	if err := s.userRepo.UpdatePassword(ctx, userID, hash, false); err != nil {
		return nil, err
	}

	// F-4 修复：改密后吊销**全部设备**会话（此前只删当前 deviceID 的 RT，
	// 其他设备可在 168h 内继续刷新）；当前 AT 单独拉黑 jti。
	if accessToken != "" {
		if err := s.revokeAccessToken(ctx, accessToken); err != nil {
			return nil, err
		}
	}
	// D2-05：改密吊销同样走重试补偿（DB 已提交、Redis 闪断时防旧会话存活）
	if err := revokeUserSessionsWithRetry(ctx, s.rdb, userID, s.jwtManager.AccessTTL()); err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	// disabled 键会拦截刚签发的新 AT，须在重新签发前清除
	// （密码已验证通过，旧 RT 已全部删除，其他设备旧 AT 最长 30 分钟自然过期）
	if err := clearUserDisabled(ctx, s.rdb, userID); err != nil {
		return nil, errcode.ErrServiceUnavailable
	}

	// 重新签发不含 mcp 标记的 Token pair
	user.MustChangePassword = false
	return s.issueTokenPair(ctx, user, normalizeDeviceID(deviceID))
}

// revokeAccessToken 将 AT 加入黑名单（剩余 TTL 内失效）
func (s *AuthService) revokeAccessToken(ctx context.Context, accessToken string) error {
	claims, err := s.jwtManager.ParseAccessToken(accessToken)
	if err != nil {
		// 旧 AT 无效则无需拉黑
		return nil
	}
	var ttl time.Duration
	if claims.ExpiresAt != nil {
		ttl = time.Until(claims.ExpiresAt.Time)
	}
	if ttl > 0 {
		blacklistKey := fmt.Sprintf("blacklist:at:%s", claims.JTI)
		if err := s.rdb.Set(ctx, blacklistKey, "1", ttl).Err(); err != nil {
			return errcode.ErrServiceUnavailable
		}
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

func failLogin(ctx context.Context, scripts *redispkg.Scripts, employeeNo string) (blocked bool, err error) {
	return scripts.LoginLockIncr(ctx, employeeNo)
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

// validDeviceID D2-22：device_id 字符白名单 [a-zA-Z0-9_-]{1,64}——
// 前端 UUID/自定义标识均落在该集合内；拒绝任意字符进 Redis 键/JWT claims
func validDeviceID(deviceID string) bool {
	if len(deviceID) == 0 || len(deviceID) > 64 {
		return false
	}
	for _, ch := range deviceID {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}
