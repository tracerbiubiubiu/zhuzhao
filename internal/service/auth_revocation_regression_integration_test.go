//go:build integration

package service_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao-utils/crypto"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	redispkg "github.com/tracerbiubiubiu/zhuzhao-utils/redis"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// Uses TestMain's fresh testcontainer PG and a fresh miniredis instance.
// E000001 is neither created nor updated by this selected test.
func TestAudit_AdminResetThenLogin_OldATMustStayRevoked(t *testing.T) {
	resetAuthTables(t)
	ctx := context.Background()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	repo := repository.NewUserRepo(testPool)
	hash, err := crypto.HashPassword("AuditOld#2026")
	require.NoError(t, err)
	actor := &model.User{Username: "audit_manager", EmployeeNo: "E800901", Password: hash, Status: 1}
	target := &model.User{Username: "audit_target", EmployeeNo: "E800902", Password: hash, Status: 1}
	require.NoError(t, repo.Create(ctx, actor))
	require.NoError(t, repo.Create(ctx, target))
	_, err = testPool.Exec(ctx, `INSERT INTO roles(code,name,status,priority) SELECT 'admin','audit admin',1,10 WHERE NOT EXISTS(SELECT 1 FROM roles WHERE code='admin' AND deleted_at IS NULL)`)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE code='admin' AND deleted_at IS NULL`, actor.ID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO roles(code,name,status,priority) SELECT 'viewer','audit viewer',1,30 WHERE NOT EXISTS(SELECT 1 FROM roles WHERE code='viewer' AND deleted_at IS NULL)`)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE code='viewer' AND deleted_at IS NULL`, target.ID)
	require.NoError(t, err)
	cfg := config.JWTConfig{Secret: "audit-only-synthetic-secret-20261104", AccessTTL: 30 * time.Minute, RefreshTTL: 168 * time.Hour}
	mgr := jwtpkg.NewManager(jwtpkg.Config{Secret: cfg.Secret, AccessTTL: cfg.AccessTTL})
	auth := service.NewAuthService(repo, mgr, rdb, redispkg.NewScripts(rdb), service.NewAuditService(repository.NewAuditLogRepo(testPool), repo), cfg, nil)
	users := service.NewUserService(testPool, repo, repository.NewRoleRepo(testPool), nil, rdb, mgr)
	rbac := service.NewRBACService(repository.NewRoleRepo(testPool), repo, nil, nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/user/profile", middleware.JWT(mgr, rdb, nil), middleware.SelfService(), middleware.CasbinAuth(nil, rbac, nil), handler.NewUserHandler(users, nil).GetProfile)
	oldPair, err := auth.Login(ctx, &model.LoginRequest{EmployeeNo: target.EmployeeNo, Password: "AuditOld#2026", DeviceID: "old-device"}, "127.0.0.1", "audit")
	require.NoError(t, err)
	request := func(at string) (int, string) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
		req.Header.Set("Authorization", "Bearer "+at)
		router.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	before, _ := request(oldPair.AccessToken)
	require.Equal(t, http.StatusOK, before)
	require.NoError(t, users.ResetPassword(ctx, &model.ResetPasswordRequest{UserID: target.ID, Password: "AuditNew#2026"}, actor.ID))
	revoked, _ := request(oldPair.AccessToken)
	require.Equal(t, http.StatusForbidden, revoked, "reset initially revokes the old AT")
	fresh, err := auth.Login(ctx, &model.LoginRequest{EmployeeNo: target.EmployeeNo, Password: "AuditNew#2026", DeviceID: "new-device"}, "127.0.0.1", "audit")
	require.NoError(t, err)
	require.True(t, fresh.MustChangePassword)
	freshStatus, freshBody := request(fresh.AccessToken)
	require.Equal(t, http.StatusForbidden, freshStatus)
	require.Contains(t, freshBody, `"code":20007`)
	oldStatus, _ := request(oldPair.AccessToken)
	floors, err := rdb.Exists(ctx, fmt.Sprintf("user:at_min_iat:%d", target.ID)).Result()
	require.NoError(t, err)
	t.Logf("old_AT status: before=%d after_reset=%d after_new_login=%d; fresh_AT=%d/20007; floor_keys=%d", before, revoked, oldStatus, freshStatus, floors)
	require.Equal(t, http.StatusUnauthorized, oldStatus, "a revoked pre-reset AT must not become valid after legitimate login")
}
