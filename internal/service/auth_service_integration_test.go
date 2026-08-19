//go:build integration

package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/crypto"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jwt"
	redispkg "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/redis"
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
	authSvc := service.NewAuthService(repo, jwt.NewManager(jwtCfg), rdb, redispkg.NewScripts(rdb), auditSvc, jwtCfg)

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
	authSvc := service.NewAuthService(repo, jwt.NewManager(jwtCfg), rdb, redispkg.NewScripts(rdb), auditSvc, jwtCfg)

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
