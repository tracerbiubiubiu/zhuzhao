//go:build integration

package service_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
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
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// Uses TestMain's fresh testcontainer PG and a fresh miniredis instance.
// E000001 is neither created nor updated by this selected test.
func TestAudit_AdminResetThenLogin_OldATMustStayRevoked(t *testing.T) {
	resetAuthTables(t)
	ctx := trustedIAMTestContext()
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
	// Direct repository fixtures need the real worker's gap discovery and durable
	// intent confirmation before production login may issue DB-epoch credentials.
	service.RunProjectionSweepOnce(ctx, service.NewSessionWorkerDeps(testPool, repo, rdb, mgr.AccessTTL()))
	for _, user := range []*model.User{actor, target} {
		snap, err := repo.SecuritySnapshot(ctx, user.ID)
		require.NoError(t, err)
		require.EqualValues(t, 1, snap.SessionEpoch)
		require.EqualValues(t, 1, snap.ProjectionEpoch)
		var confirmed int
		require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM session_projection_intents WHERE user_id=$1 AND target_epoch=1 AND status='confirmed'`, user.ID).Scan(&confirmed))
		require.Equal(t, 1, confirmed, "initial projection must be confirmed through a durable intent")
	}

	const resetPath = "/api/v1/users/password/reset"
	_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p','role::admin',$1,'POST') ON CONFLICT DO NOTHING`, resetPath)
	require.NoError(t, err)
	casbinModel, err := casbinmodel.NewModelFromFile("../../configs/casbin_model.conf")
	require.NoError(t, err)
	enforcer, err := casbin.NewSyncedEnforcer(casbinModel)
	require.NoError(t, err)
	_, err = enforcer.AddPolicy("role::admin", resetPath, "POST")
	require.NoError(t, err)
	rbac := service.NewRBACService(repository.NewRoleRepo(testPool), repo, nil, enforcer)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	jwtAuth := middleware.JWT(mgr, rdb, nil, middleware.WithAccountSecurity(c05SnapshotSource{pool: testPool}))
	userHandler := handler.NewUserHandler(users, nil)
	router.GET("/api/v1/user/profile", jwtAuth, middleware.SelfService(), middleware.CasbinAuth(enforcer, rbac, nil), userHandler.GetProfile)
	router.POST(resetPath, jwtAuth, middleware.CasbinAuth(enforcer, rbac, nil), userHandler.ResetPassword)
	actorPair, err := auth.Login(ctx, &model.LoginRequest{EmployeeNo: actor.EmployeeNo, Password: "AuditOld#2026", DeviceID: "manager-device"}, "127.0.0.1", "audit")
	require.NoError(t, err)
	oldPair, err := auth.Login(ctx, &model.LoginRequest{EmployeeNo: target.EmployeeNo, Password: "AuditOld#2026", DeviceID: "old-device"}, "127.0.0.1", "audit")
	require.NoError(t, err)
	oldClaims, err := mgr.ParseAccessToken(oldPair.AccessToken)
	require.NoError(t, err)
	require.Equal(t, jwtpkg.SessionSchemaDBEpochV1, oldClaims.SessionSchema)
	require.Equal(t, "1", oldClaims.SessionEpoch)
	request := func(at string) (int, string) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
		req.Header.Set("Authorization", "Bearer "+at)
		router.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	before, beforeBody := request(oldPair.AccessToken)
	require.Equal(t, http.StatusOK, before, beforeBody)
	// The management write is admitted by signed authentication and CasbinAuth;
	// no trusted-internal context or manually injected credential facts reach it.
	const operationID = "K-AUDIT-RESET-LOGIN"
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, resetPath, bytes.NewBufferString(fmt.Sprintf(`{"user_id":%q,"password":"AuditNew#2026"}`, fmt.Sprint(target.ID))))
	req.Header.Set("Authorization", "Bearer "+actorPair.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Operation-ID", operationID)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"code":0`)
	require.Equal(t, operationID, w.Header().Get("X-Operation-ID"))
	require.Equal(t, "completed", w.Header().Get("X-Operation-Status"))
	snapAfterReset, err := repo.SecuritySnapshot(ctx, target.ID)
	require.NoError(t, err)
	require.Equal(t, 1, snapAfterReset.Status, "password reset must leave the account active")
	require.True(t, snapAfterReset.MustChangePassword)
	require.True(t, crypto.CheckPassword("AuditNew#2026", snapAfterReset.PasswordHash))
	require.EqualValues(t, 2, snapAfterReset.SessionEpoch)
	require.EqualValues(t, 2, snapAfterReset.ProjectionEpoch)
	op, err := repository.NewSessionOperationRepo(testPool).FindOperationByID(ctx, operationID)
	require.NoError(t, err)
	require.Equal(t, service.SessionCmdUserResetPwd, op.Command)
	require.Equal(t, actor.ID, op.ActorUserID)
	require.Equal(t, target.ID, op.TargetUserID)
	require.Equal(t, "completed", op.Status)
	require.NotNil(t, op.ResultEpoch)
	require.EqualValues(t, 2, *op.ResultEpoch)
	revoked, revokedBody := request(oldPair.AccessToken)
	require.Equal(t, http.StatusUnauthorized, revoked, "reset must immediately invalidate the signed old DB epoch")
	require.Contains(t, revokedBody, fmt.Sprintf(`"code":%d`, errcode.ErrTokenInvalid.Code))
	fresh, err := auth.Login(ctx, &model.LoginRequest{EmployeeNo: target.EmployeeNo, Password: "AuditNew#2026", DeviceID: "new-device"}, "127.0.0.1", "audit")
	require.NoError(t, err)
	require.True(t, fresh.MustChangePassword)
	freshClaims, err := mgr.ParseAccessToken(fresh.AccessToken)
	require.NoError(t, err)
	require.Equal(t, jwtpkg.SessionSchemaDBEpochV1, freshClaims.SessionSchema)
	require.Equal(t, "2", freshClaims.SessionEpoch)
	freshStatus, freshBody := request(fresh.AccessToken)
	require.Equal(t, http.StatusForbidden, freshStatus)
	require.Contains(t, freshBody, fmt.Sprintf(`"code":%d`, errcode.ErrPasswordChangeRequired.Code))
	oldStatus, oldBody := request(oldPair.AccessToken)
	require.Equal(t, http.StatusUnauthorized, oldStatus, "a revoked pre-reset AT must not become valid after legitimate login")
	require.Contains(t, oldBody, fmt.Sprintf(`"code":%d`, errcode.ErrTokenInvalid.Code))
	refreshed, err := auth.Refresh(ctx, oldPair.RefreshToken, "127.0.0.1", "audit")
	require.Nil(t, refreshed, "the old RT must not cross the reset epoch either")
	require.ErrorIs(t, err, errcode.ErrPasswordChanged, "the active account rejects an RT signed for the pre-reset epoch")
	snapAfterLogin, err := repo.SecuritySnapshot(ctx, target.ID)
	require.NoError(t, err)
	require.Equal(t, snapAfterReset.SessionEpoch, snapAfterLogin.SessionEpoch, "login cannot undo or advance the committed reset epoch")
	require.Equal(t, snapAfterReset.ProjectionEpoch, snapAfterLogin.ProjectionEpoch)
	projectedEpoch, err := rdb.Get(ctx, fmt.Sprintf("user:session_epoch:db1:%d", target.ID)).Result()
	require.NoError(t, err)
	require.Equal(t, "2", projectedEpoch, "Redis must retain the confirmed DB reset epoch after login")
}
