//go:build integration

package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao-utils/crypto"
	"github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	redispkg "github.com/tracerbiubiubiu/zhuzhao-utils/redis"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestAuthService_LoginRefreshLogout(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	repo := repository.NewUserRepo(testPool)
	ctx := context.Background()
	resetAuthTables(t)

	hash, err := crypto.HashPassword("admin123")
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, &model.User{
		Username: "admin", EmployeeNo: "E000001", Password: hash, Status: 1,
	}))

	jwtCfg := config.JWTConfig{
		Secret:     "test-secret-key-for-auth-service",
		AccessTTL:  30 * time.Minute,
		RefreshTTL: 168 * time.Hour,
	}
	auditSvc := service.NewAuditService(repository.NewAuditLogRepo(testPool), repo)
	authSvc := service.NewAuthService(repo, jwt.NewManager(jwt.Config{Secret: jwtCfg.Secret, AccessTTL: jwtCfg.AccessTTL}), rdb, redispkg.NewScripts(rdb), auditSvc, jwtCfg)

	pair, err := authSvc.Login(ctx, &model.LoginRequest{
		EmployeeNo: "E000001",
		Password:   "admin123",
		DeviceID:   "dev-1",
	}, "127.0.0.1", "test-agent")
	require.NoError(t, err)
	assert.NotEmpty(t, pair.AccessToken)

	refreshed, err := authSvc.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err)
	assert.NotEmpty(t, refreshed.AccessToken)

	_, err = authSvc.Refresh(ctx, pair.RefreshToken)
	require.Error(t, err)
	var biz *errcode.Error
	require.ErrorAs(t, err, &biz)
	assert.Equal(t, errcode.ErrRefreshTokenInvalid.Code, biz.Code)

	require.NoError(t, authSvc.Logout(ctx, refreshed.AccessToken, "dev-1"))
}

// C3 回归：改密后旧 RT（旧纪元）刷新必拒；新纪元 RT 正常刷新。
// 注意单槽语义：同一设备上先试旧 RT 会消费掉新 RT 的槽位（02-auth §RT 轮换
// 「旧 RT 提交本身即盗用信号」）——故先验新 RT，后验旧 RT 拒绝。
func TestAuthService_PasswordChangeInvalidatesOldRefreshToken(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	repo := repository.NewUserRepo(testPool)
	ctx := context.Background()
	resetAuthTables(t)

	hash, err := crypto.HashPassword("admin123")
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, &model.User{
		Username: "admin", EmployeeNo: "E000001", Password: hash, Status: 1,
	}))

	jwtCfg := config.JWTConfig{Secret: "test-secret-key-for-auth-service", AccessTTL: 30 * time.Minute, RefreshTTL: 168 * time.Hour}
	authSvc := service.NewAuthService(repo, jwt.NewManager(jwt.Config{Secret: jwtCfg.Secret, AccessTTL: jwtCfg.AccessTTL}), rdb, redispkg.NewScripts(rdb), service.NewAuditService(repository.NewAuditLogRepo(testPool), repo), jwtCfg)

	pair, err := authSvc.Login(ctx, &model.LoginRequest{
		EmployeeNo: "E000001", Password: "admin123", DeviceID: "dev-1",
	}, "127.0.0.1", "test-agent")
	require.NoError(t, err)

	// 旧 RT 纪元钉住：改密前签发的 RT 无 pwe 字段（解析为 0）
	mgr := jwt.NewManager(jwt.Config{Secret: jwtCfg.Secret, AccessTTL: jwtCfg.AccessTTL})
	oldClaims, perr := mgr.ParseRefreshToken(pair.RefreshToken)
	require.NoError(t, perr)
	require.EqualValues(t, 0, oldClaims.Pwe, "改密前 RT 应为纪元 0（平滑兼容基线）")

	// 改密（触发全量吊销 + 纪元 INCR + 重签当前设备 pair，pwe=新纪元）
	newPair, err := authSvc.UpdatePassword(ctx, oldClaims.UserID, "admin123", "NewPass456", pair.AccessToken, "dev-1")
	require.NoError(t, err)
	newClaims, perr := mgr.ParseRefreshToken(newPair.RefreshToken)
	require.NoError(t, perr)
	require.EqualValues(t, 1, newClaims.Pwe, "改密后重签 RT 应携带新纪元")

	// 纪元键已 INCR
	n, gerr := rdb.Get(ctx, fmt.Sprintf("user:pw_epoch:%d", oldClaims.UserID)).Result()
	require.NoError(t, gerr)
	require.Equal(t, "1", n)

	// 新 RT（新纪元）正常刷新
	refreshed, err := authSvc.Refresh(ctx, newPair.RefreshToken)
	require.NoError(t, err)
	assert.NotEmpty(t, refreshed.AccessToken)

	// 旧 RT（pwe=0）刷新 → 纪元比对不一致 → 401 + 20004
	// （单槽语义副作用：该次消费同时清掉 dev-1 槽位，属 02-auth 既定盗用信号设计）
	_, err = authSvc.Refresh(ctx, pair.RefreshToken)
	require.Error(t, err)
	var biz *errcode.Error
	require.ErrorAs(t, err, &biz)
	assert.Equal(t, errcode.ErrRefreshTokenInvalid.Code, biz.Code)

	// 新口令在另一设备登录 → 新纪元 RT 不受 dev-1 槽位影响
	relogin, err := authSvc.Login(ctx, &model.LoginRequest{
		EmployeeNo: "E000001", Password: "NewPass456", DeviceID: "dev-2",
	}, "127.0.0.1", "test-agent")
	require.NoError(t, err)
	_, err = authSvc.Refresh(ctx, relogin.RefreshToken)
	require.NoError(t, err)
}

func TestAuthService_LoginWrongPassword(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	repo := repository.NewUserRepo(testPool)
	ctx := context.Background()
	resetAuthTables(t)

	hash, _ := crypto.HashPassword("admin123")
	require.NoError(t, repo.Create(ctx, &model.User{Username: "admin", EmployeeNo: "E000001", Password: hash}))

	jwtCfg := config.JWTConfig{Secret: "test-secret", AccessTTL: 30 * time.Minute, RefreshTTL: 168 * time.Hour}
	auditSvc := service.NewAuditService(repository.NewAuditLogRepo(testPool), repo)
	authSvc := service.NewAuthService(repo, jwt.NewManager(jwt.Config{Secret: jwtCfg.Secret, AccessTTL: jwtCfg.AccessTTL}), rdb, redispkg.NewScripts(rdb), auditSvc, jwtCfg)

	_, err = authSvc.Login(ctx, &model.LoginRequest{EmployeeNo: "E000001", Password: "wrong"}, "127.0.0.1", "test-agent")
	require.Error(t, err)
	var biz *errcode.Error
	require.ErrorAs(t, err, &biz)
	assert.Equal(t, errcode.ErrInvalidCredentials.Code, biz.Code)
}

func resetAuthTables(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `TRUNCATE user_roles, users RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
}

// newAuthServiceForTest 构造带 miniredis 的 AuthService
func newAuthServiceForTest(t *testing.T) *service.AuthService {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	jwtCfg := config.JWTConfig{Secret: "test-secret", AccessTTL: 30 * time.Minute, RefreshTTL: 168 * time.Hour}
	repo := repository.NewUserRepo(testPool)
	auditSvc := service.NewAuditService(repository.NewAuditLogRepo(testPool), repo)
	return service.NewAuthService(repo, jwt.NewManager(jwt.Config{Secret: jwtCfg.Secret, AccessTTL: jwtCfg.AccessTTL}), rdb, redispkg.NewScripts(rdb), auditSvc, jwtCfg)
}

// B2-2 守护：新密码与旧密码相同 → 400（ErrInvalidParams），
// 不吊销会话、不重签 Token（修复前会「成功」改密并吊销全部设备）。
func TestAuthService_UpdatePasswordSameRejected(t *testing.T) {
	resetAuthTables(t)
	ctx := context.Background()
	repo := repository.NewUserRepo(testPool)
	authSvc := newAuthServiceForTest(t)

	hash, err := crypto.HashPassword("oldpass123")
	require.NoError(t, err)
	user := &model.User{Username: "pwd_user", EmployeeNo: "E620001", Password: hash, Status: 1}
	require.NoError(t, repo.Create(ctx, user))

	_, err = authSvc.UpdatePassword(ctx, user.ID, "oldpass123", "oldpass123", "", "dev-1")
	require.Error(t, err)
	var biz *errcode.Error
	require.ErrorAs(t, err, &biz)
	assert.Equal(t, errcode.ErrInvalidParams.Code, biz.Code, "新旧密码相同应返回 400 参数错误")

	// 密码未变更：旧密码仍可验证通过
	got, err := repo.FindByID(ctx, user.ID)
	require.NoError(t, err)
	assert.True(t, crypto.CheckPassword("oldpass123", got.Password), "相同密码不应触发实际改密")
}
