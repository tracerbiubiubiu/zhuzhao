package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao-utils/crypto"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	redispkg "github.com/tracerbiubiubiu/zhuzhao-utils/redis"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const defaultDeviceID = "default"

// AuthService 认证服务
type AuthService struct {
	userRepo     *repository.UserRepo
	opsRepo      *repository.SessionOperationRepo
	pool         *pgxpool.Pool
	jwtManager   *jwtpkg.Manager
	rdb          *goredis.Client
	scripts      *redispkg.Scripts
	auditService *AuditService
	refreshTTL   time.Duration
	captchaSvc   *CaptchaService
}

// NewAuthService 创建 AuthService
func NewAuthService(
	userRepo *repository.UserRepo,
	jwtManager *jwtpkg.Manager,
	rdb *goredis.Client,
	scripts *redispkg.Scripts,
	auditService *AuditService,
	jwtCfg config.JWTConfig,
	captchaSvc *CaptchaService,
) *AuthService {
	return &AuthService{
		userRepo:     userRepo,
		opsRepo:      repository.NewSessionOperationRepo(userRepo.Pool()),
		pool:         userRepo.Pool(),
		jwtManager:   jwtManager,
		rdb:          rdb,
		scripts:      scripts,
		auditService: auditService,
		refreshTTL:   jwtCfg.RefreshTTL,
		captchaSvc:   captchaSvc,
	}
}

// sessionDeps C05 统一安全写依赖集（自助改密与账号管理命令共用同一事实库）。
func (s *AuthService) sessionDeps() SessionSecurityDeps {
	return SessionSecurityDeps{
		pool:      s.pool,
		userRepo:  s.userRepo,
		opsRepo:   s.opsRepo,
		rdb:       s.rdb,
		accessTTL: s.jwtManager.AccessTTL(),
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

	// P4-7：验证码一次性校验（enabled=false 恒通过——dev/E2E 登录契约不变）
	if err := s.captchaSvc.Verify(ctx, req.CaptchaID, req.Captcha); err != nil {
		s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, nil, "", 400)
		return nil, err
	}

	blocked, err := s.scripts.LoginLockIsBlocked(ctx, req.EmployeeNo)
	if err != nil {
		// §3-13 留痕：业务码 503 不经错误中枢日志，源头落一行否则 Redis 故障无迹可查
		slog.Error("login lock check failed", "err", err)
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
			slog.Error("fail-login lock write failed", "err", lockErr)
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

	// Validate the credential and epoch from one DB snapshot. A later reread must
	// still match it; credentials verified before a reset cannot borrow its epoch.
	snapBefore, err := s.userRepo.SecuritySnapshot(ctx, user.ID)
	if err != nil {
		return nil, errcode.ErrServiceUnavailable
	}

	if snapBefore.Deleted || snapBefore.Status != 1 {
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

	if !crypto.CheckPassword(req.Password, snapBefore.PasswordHash) {
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
	if err := s.userRepo.UpdateLastLogin(ctx, user.ID, ip); err != nil {
		// B4-1：认证已通过但后续失败同样落审计（此前该分支无登录记录）
		s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, &user.ID, user.Username, 500)
		return nil, fmt.Errorf("update last login: %w", err)
	}

	// C05：重读 DB 安全快照——认证期间发生改密/停用提交（代际推进/软删/停用）
	// 即拒绝，不借用新代签发旧凭据会话
	snapAfter, err := s.userRepo.SecuritySnapshot(ctx, user.ID)
	if err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	if snapAfter.Deleted || snapAfter.Status != 1 || snapAfter.SessionEpoch != snapBefore.SessionEpoch || snapAfter.PasswordHash != snapBefore.PasswordHash {
		s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, &user.ID, user.Username, 401)
		return nil, errcode.ErrInvalidCredentials
	}
	// C05 切换就绪门：已切换账号在当前代投影未确认（不可证明 Redis 已同步）时
	// 可恢复 503，恢复后可正常登录；缺键不回退旧代
	if err := CheckProjectionReadiness(ctx, s.sessionDeps(), user.ID, snapAfter); err != nil {
		s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, &user.ID, user.Username, 503)
		return nil, err
	}

	user.MustChangePassword = snapAfter.MustChangePassword
	pair, err := s.issueTokenPair(ctx, user, normalizeDeviceID(req.DeviceID), snapBefore.SessionEpoch)
	if err != nil {
		s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, &user.ID, user.Username, 500)
		return nil, err
	}
	s.auditService.LogLogin(ctx, req.EmployeeNo, ip, userAgent, &user.ID, user.Username, 200)
	return pair, nil
}

// rotateSessionPairLua compares the original RT hash and installs its successor
// atomically. A late refresh never removes or overwrites a newer Login/Refresh.
// Compare epoch strings directly rather than converting int64 to a Lua number.
const rotateSessionPairLua = `
if redis.call('GET', KEYS[2]) ~= ARGV[1] or redis.call('EXISTS', KEYS[3]) ~= 0 then
  return -2
end
local old = redis.call('GET', KEYS[1])
if not old then return 0 end
if old ~= ARGV[2] then return -1 end
redis.call('PSETEX', KEYS[1], ARGV[4], ARGV[3])
return 1
`

// Refresh 刷新 Token，RT 轮换
func (s *AuthService) Refresh(ctx context.Context, refreshToken, ip, userAgent string) (*model.TokenPair, error) {
	claims, err := s.jwtManager.ParseRefreshToken(refreshToken)
	if err != nil {
		return nil, errcode.ErrRefreshTokenInvalid
	}
	epoch, err := dbSessionClaimsEpoch(claims.SessionSchema, claims.SessionEpoch)
	if err != nil || claims.UserID < 1 {
		return nil, errcode.ErrRefreshTokenInvalid
	}

	snap, err := s.userRepo.SecuritySnapshot(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, errcode.ErrUserNotFound) {
			return nil, errcode.ErrRefreshTokenInvalid
		}
		return nil, errcode.ErrServiceUnavailable
	}
	if snap.Deleted || snap.Status != 1 {
		return nil, errcode.ErrRefreshTokenInvalid
	}
	if snap.SessionEpoch != epoch {
		return nil, errcode.ErrPasswordChanged
	}
	if err := CheckProjectionReadiness(ctx, s.sessionDeps(), claims.UserID, snap); err != nil {
		return nil, err
	}
	user, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, errcode.ErrUserNotFound) {
			return nil, errcode.ErrRefreshTokenInvalid
		}
		return nil, errcode.ErrServiceUnavailable
	}
	user.MustChangePassword = snap.MustChangePassword
	deviceID := normalizeDeviceID(claims.DeviceID)
	if !validDeviceID(deviceID) {
		// D2-22：防御——claims 来自服务端签名令牌，正常不可达；
		// 伪造/历史脏 claims 拒绝刷新
		return nil, errcode.ErrRefreshTokenInvalid
	}
	pair, err := s.generateSessionTokenPair(user, deviceID, epoch)
	if err != nil {
		return nil, err
	}
	result, err := s.rdb.Eval(ctx, rotateSessionPairLua, []string{
		dbRefreshKey(claims.UserID, epoch, deviceID), dbSessionEpochKey(claims.UserID), dbSessionDisabledKey(claims.UserID),
	}, claims.SessionEpoch, hashToken(refreshToken), hashToken(pair.RefreshToken), s.refreshTTL.Milliseconds()).Int()
	if err != nil || result == -2 {
		return nil, errcode.ErrServiceUnavailable
	}
	if result == 0 {
		return nil, errcode.ErrRefreshTokenInvalid
	}
	if result == -1 {
		// C-5/RT-1 观测先行（2026-10-27 拍板）：重放命中即落审计行+WARN——记录面
		// 不设降噪前提（合法重放也留痕，三周后追查「一查便知」依赖此行）；告警面
		// （宽限窗口/family_id）仍按台账 C-5 触发条件挂起，记录≠告警两事不互斥
		s.auditService.LogRefreshReplay(ctx, claims.UserID, user.Username, ip, userAgent)
		slog.Warn("auth: refresh token replay detected (RT-1)",
			"user_id", claims.UserID, "username", user.Username, "ip", ip,
			"request_id", reqid.From(ctx))
		return nil, errcode.ErrRefreshTokenReplayed
	}
	if result != 1 {
		return nil, errcode.ErrServiceUnavailable
	}
	return pair, nil
}

// Logout 登出，吊销 AT + 删除 RT
func (s *AuthService) Logout(ctx context.Context, accessToken, deviceID string) error {
	claims, err := s.jwtManager.ParseAccessToken(accessToken)
	if err != nil {
		return errcode.ErrTokenInvalid
	}
	epoch, err := dbSessionClaimsEpoch(claims.SessionSchema, claims.SessionEpoch)
	if err != nil || claims.UserID < 1 {
		return errcode.ErrTokenInvalid
	}
	// W0b（二十四批登出槽）：device_id 必填——空值曾静默归一 "default" 槽，
	// 误删 default 会话而真实设备 RT 残留（最长 168h）。AT claims 无 DeviceID
	// 可用（AccessClaims 仅 uid/username/jti/mcp/typ），故从请求体收紧；
	// 前端契约（01 §6）已承诺登录/登出/改密三处同源，零前端改动。
	if deviceID == "" {
		return errcode.ErrInvalidParams
	}
	// D2-22：device_id 白名单（与 Login 对齐）
	if !validDeviceID(deviceID) {
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

	rtKey := dbRefreshKey(claims.UserID, epoch, deviceID)
	if err := s.rdb.Del(ctx, rtKey).Err(); err != nil {
		return errcode.ErrServiceUnavailable
	}
	return nil
}

// UpdatePassword 用户修改密码（需旧密码验证）。C05：同事务写口令/代际/回执/意图
// （F021——同 K 重投只核查原结果，不再执行秘密写、不重放 TokenPair；口令不进
// 回执/指纹/日志）。投影失败保持 DB 已提交事实，原意图可恢复（可恢复 503）。
func (s *AuthService) UpdatePassword(ctx context.Context, userID int64, oldPassword, newPassword, accessToken, deviceID, operationID string) (*model.TokenPair, *SessionCommandResult, error) {
	if oldPassword == "" || newPassword == "" {
		return nil, nil, errcode.ErrInvalidParams
	}
	// D2-22：device_id 白名单（与 Login/Logout 对齐——重签 Token 写 Redis 键）
	if deviceID != "" && !validDeviceID(deviceID) {
		return nil, nil, errcode.ErrInvalidParams
	}
	deps := s.sessionDeps()
	opID := ResolveOperationID(operationID)
	// F021：已有 operation 无论在途/完成/待核查，只给明确核查状态
	if res, hit, err := lookUpOperation(ctx, deps, SessionCmdChangePassword, opID, userID, userID); err != nil {
		return nil, nil, err
	} else if hit {
		return nil, res, nil
	}
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if !crypto.CheckPassword(oldPassword, user.Password) {
		return nil, nil, errcode.ErrInvalidCredentials
	}
	// B2-2：新密码与旧密码相同 → 400（02-auth.md 测试用例承诺）；
	// 防止「以为改了密码」实际凭证未变却吊销了全部会话
	if oldPassword == newPassword {
		return nil, nil, errcode.ErrInvalidParams
	}
	hash, err := crypto.HashPassword(newPassword) // bcrypt 在锁外计算
	if err != nil {
		return nil, nil, fmt.Errorf("hash password: %w", err)
	}
	// C05 统一安全事务：IAM 守卫内锁行重核（原 hash/active/epoch），同事务写
	// 口令、代际+1、回执与投影意图；投影在守卫提交后按 AfterIAMCommit 尝试。
	var newEpoch int64
	guardErr := repository.WithIAMGuard(ctx, s.pool, func(gctx context.Context) error {
		var werr error
		newEpoch, _, _, werr = securityWrite(gctx, deps, SessionCmdChangePassword, opID, "http", userID, userID,
			func(snap *repository.UserSecuritySnapshot) error {
				if snap.Deleted {
					return errcode.ErrUserNotFound
				}
				if snap.Status != 1 {
					return errcode.ErrInvalidCredentials
				}
				return nil
			},
			func(tx pgx.Tx, snap *repository.UserSecuritySnapshot) (int64, bool, error) {
				// 锁内重核原 hash/epoch/active：管理员新重置后不得覆盖新密码
				ne, err := s.userRepo.ChangePasswordSecureTx(ctx, tx, userID, user.Password, hash, snap.SessionEpoch)
				return ne, true, err
			},
			func(snap *repository.UserSecuritySnapshot) bool { return true },
			"self-change-password")
		if werr != nil {
			return werr
		}
		return projectAfterCommit(gctx, deps, userID, newEpoch)
	})
	if errors.Is(guardErr, errcode.ErrConcurrentModification) {
		// 同 K 并发：唯一保留失败收敛为读原回执（最多一个请求产生效果）
		if res, hit, lerr := lookUpOperation(ctx, deps, SessionCmdChangePassword, opID, userID, userID); lerr == nil && hit {
			return nil, res, nil
		}
		return nil, nil, errcode.ErrInvalidCredentials
	}
	if guardErr != nil {
		if errors.Is(guardErr, errcode.ErrInvalidCredentials) || errors.Is(guardErr, errcode.ErrUserNotFound) {
			return nil, nil, guardErr
		}
		// DB 已提交而投影待恢复：可恢复 503（安全事实已落库，worker 按原意图恢复）
		if errors.Is(guardErr, errcode.ErrServiceUnavailable) {
			return nil, &SessionCommandResult{OperationID: opID, Status: "pending", Epoch: newEpoch}, guardErr
		}
		return nil, nil, guardErr
	}
	// 先完成全设备吊销（投影已推进代际），再单独拉黑当前 AT；单条黑名单写失败
	// 不能跳过代际推进。
	if accessToken != "" {
		if err := s.revokeAccessToken(ctx, accessToken); err != nil {
			return nil, &SessionCommandResult{OperationID: opID, Status: "completed", Epoch: newEpoch}, err
		}
	}
	// P1-D（2026-11-01 审计）：写改密地板——其他设备旧 AT 即时失效。保留兼容地板，
	// TTL=AccessTTL+60s 自然过期；写失败与吊销同档 fail-close。
	if err := s.rdb.Set(ctx, fmt.Sprintf("user:at_min_iat:%d", userID), time.Now().Unix(),
		s.jwtManager.AccessTTL()+time.Minute).Err(); err != nil {
		return nil, &SessionCommandResult{OperationID: opID, Status: "completed", Epoch: newEpoch}, errcode.ErrServiceUnavailable
	}
	// 代际绑定本次结果；重读保证之后的重置/删除不能借用本次改密结果签发会话。
	snapNow, err := s.userRepo.SecuritySnapshot(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if snapNow.Deleted || snapNow.PasswordHash != hash || snapNow.Status != 1 || snapNow.MustChangePassword || snapNow.SessionEpoch != newEpoch {
		return nil, nil, errcode.ErrInvalidCredentials
	}
	if err := CheckProjectionReadiness(ctx, deps, userID, snapNow); err != nil {
		return nil, &SessionCommandResult{OperationID: opID, Status: "pending", Epoch: newEpoch}, err
	}
	current := &model.User{ID: snapNow.ID, Username: user.Username, MustChangePassword: false}
	pair, err := s.issueTokenPair(ctx, current, normalizeDeviceID(deviceID), newEpoch)
	if err != nil {
		return nil, &SessionCommandResult{OperationID: opID, Status: "completed", Epoch: newEpoch}, err
	}
	return pair, &SessionCommandResult{OperationID: opID, Status: "completed", Epoch: newEpoch}, nil
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

// installSessionPairLua binds the slot write to the exact active projection. The
// decimal epoch is compared as a string so epochs beyond Lua's integer precision
// remain distinct. Login may replace its device slot; refresh must use CAS below.
const installSessionPairLua = `
if redis.call('GET', KEYS[2]) ~= ARGV[1] or redis.call('EXISTS', KEYS[3]) ~= 0 then
  return 0
end
redis.call('PSETEX', KEYS[1], ARGV[3], ARGV[2])
return 1
`

func (s *AuthService) issueTokenPair(ctx context.Context, user *model.User, deviceID string, epoch int64) (*model.TokenPair, error) {
	pair, err := s.generateSessionTokenPair(user, deviceID, epoch)
	if err != nil {
		return nil, err
	}
	ok, err := s.rdb.Eval(ctx, installSessionPairLua, []string{
		dbRefreshKey(user.ID, epoch, deviceID), dbSessionEpochKey(user.ID), dbSessionDisabledKey(user.ID),
	}, strconv.FormatInt(epoch, 10), hashToken(pair.RefreshToken), s.refreshTTL.Milliseconds()).Int()
	if err != nil || ok != 1 {
		return nil, errcode.ErrServiceUnavailable
	}
	return pair, nil
}

func (s *AuthService) generateSessionTokenPair(user *model.User, deviceID string, epoch int64) (*model.TokenPair, error) {
	at, _, err := s.jwtManager.GenerateAccessTokenWithSessionEpoch(user.ID, user.Username, user.MustChangePassword, 0, epoch)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}
	rt, _, err := s.jwtManager.GenerateRefreshTokenWithSessionEpoch(user.ID, deviceID, s.refreshTTL, 0, epoch)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	return &model.TokenPair{
		AccessToken:        at,
		RefreshToken:       rt,
		ExpiresIn:          int(s.jwtManager.AccessTTL().Seconds()),
		MustChangePassword: user.MustChangePassword,
	}, nil
}

func dbSessionClaimsEpoch(schema, raw string) (int64, error) {
	if schema != jwtpkg.SessionSchemaDBEpochV1 {
		return 0, errcode.ErrTokenInvalid
	}
	epoch, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || epoch < 1 || strconv.FormatInt(epoch, 10) != raw {
		return 0, errcode.ErrTokenInvalid
	}
	return epoch, nil
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

// Captcha 生成验证码（透传 captchaSvc——nil 安全由构造保证）
func (s *AuthService) Captcha(ctx context.Context) (map[string]any, error) {
	return s.captchaSvc.Generate(ctx)
}
