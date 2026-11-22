//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/tracerbiubiubiu/zhuzhao-utils/crypto"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	redispkg "github.com/tracerbiubiubiu/zhuzhao-utils/redis"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

type profileWriteGateKey struct{}
type profileWriteGate struct{ reached, release chan struct{} }
type profileWriteTracer struct{}

// Pause after the real service's old read, before sending its actual UPDATE.
// The DB predicate and all auth/session logic remain production implementations.
func (profileWriteTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if gate, ok := ctx.Value(profileWriteGateKey{}).(*profileWriteGate); ok &&
		strings.Contains(data.SQL, "UPDATE users SET") && (strings.Contains(data.SQL, "username = $2") || strings.Contains(data.SQL, "real_name=NULLIF")) {
		close(gate.reached)
		select {
		case <-gate.release:
		case <-ctx.Done():
		}
	}
	return ctx
}

func (profileWriteTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// Reuse TestMain's isolated PostgreSQL; Redis is real and isolated too. No seed
// account (including E000001) is created, updated, or used by this test.
func TestUserSecurityState_ProfileCannotUndoAdmin(t *testing.T) {
	// Four sequential scenarios perform real cost-12 bcrypt under -race. Keep
	// the fixture deadline bounded and the actor's AT valid for that whole run.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "redis:8.2.11-alpine3.22@sha256:da23c2478319ceb4903857d77a2fd7aa5137d9e9d4c8165dafbacedac6403e95", ExposedPorts: []string{"6379/tcp"},
			Labels:     map[string]string{"audit.owner": "ECO-20261104-B14", "zhuzhao.test": "user-security-state"},
			WaitingFor: wait.ForListeningPort("6379/tcp"),
		}, Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(trustedIAMTestContext())) })
	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)
	rdb := goredis.NewClient(&goredis.Options{Addr: net.JoinHostPort(host, port.Port())})
	t.Cleanup(func() { _ = rdb.Close() })
	poolCfg := testPool.Config()
	poolCfg.ConnConfig.Tracer = profileWriteTracer{}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	repo, roles := repository.NewUserRepo(pool), repository.NewRoleRepo(pool)
	cfg := config.JWTConfig{Secret: "security-state-regression-synthetic-secret", AccessTTL: 5 * time.Minute, RefreshTTL: time.Hour}
	mgr := jwtpkg.NewManager(jwtpkg.Config{Secret: cfg.Secret, AccessTTL: cfg.AccessTTL})
	auth := service.NewAuthService(repo, mgr, rdb, redispkg.NewScripts(rdb), service.NewAuditService(repository.NewAuditLogRepo(pool), repo), cfg, nil)
	users := service.NewUserService(pool, repo, roles, nil, rdb, mgr)
	deps := service.NewSessionWorkerDeps(pool, repo, rdb, mgr.AccessTTL())
	userHandler := handler.NewUserHandler(users, nil)
	authHandler := handler.NewAuthHandler(auth, false)
	hash, err := crypto.HashPassword("StateOld#2026")
	require.NoError(t, err)
	suffix := uniqueSuffix()
	actor := &model.User{Username: "state_manager", EmployeeNo: "E8M" + suffix, Password: hash, Status: 1}
	require.NoError(t, repo.Create(ctx, actor))
	role := &model.Role{Code: "state_manager_" + suffix, Name: "state manager", Priority: 10, Status: 1}
	require.NoError(t, roles.Create(ctx, role))
	require.NoError(t, repo.SetRoles(ctx, actor.ID, []int64{role.ID}))
	memberRole := &model.Role{Code: "state_member_" + suffix, Name: "state member", Priority: 100, Status: 1}
	require.NoError(t, roles.Create(ctx, memberRole))
	casbinModel, err := casbinmodel.NewModelFromFile("../../configs/casbin_model.conf")
	require.NoError(t, err)
	enforcer, err := casbin.NewSyncedEnforcer(casbinModel)
	require.NoError(t, err)
	for _, path := range []string{"/api/v1/users/status", "/api/v1/users/password/reset", "/api/v1/users/update"} {
		_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,'POST')`, "role::"+role.Code, path)
		require.NoError(t, err)
		_, err = enforcer.AddPolicy("role::"+role.Code, path, http.MethodPost)
		require.NoError(t, err)
	}
	rbac := service.NewRBACService(roles, repo, repository.NewMenuRepo(pool), enforcer)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/business", middleware.JWT(mgr, rdb, nil, middleware.WithAccountSecurity(c05SnapshotSource{pool: pool})), func(c *gin.Context) { c.Status(http.StatusOK) })
	access := func(token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/business", nil).WithContext(ctx)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response
	}
	// Use the production signed-epoch authentication and purpose minting chain.
	// Neither userID nor session_epoch is inserted by test middleware.
	request := func(requestCtx context.Context, path, token string, self bool, action gin.HandlerFunc, body string) *httptest.ResponseRecorder {
		r := gin.New()
		r.Use(middleware.JWT(mgr, rdb, nil, middleware.WithAccountSecurity(c05SnapshotSource{pool: pool})))
		if self {
			r.Use(middleware.SelfService())
		}
		r.Use(middleware.CasbinAuth(enforcer, rbac, nil))
		r.POST(path, action)
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(requestCtx)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	service.RunProjectionSweepOnce(ctx, deps)
	actorPair, err := auth.Login(ctx, &model.LoginRequest{EmployeeNo: actor.EmployeeNo, Password: "StateOld#2026", DeviceID: "manager"}, "127.0.0.1", "security-state-test")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, access(actorPair.AccessToken).Code)
	for _, kind := range []string{"disable", "reset"} {
		for _, writer := range []string{"self_profile", "admin_profile"} {
			t.Run(kind+"/"+writer, func(t *testing.T) {
				target := &model.User{Username: "state_target", EmployeeNo: "E8T" + uniqueSuffix(), Password: hash, Status: 1}
				require.NoError(t, repo.Create(ctx, target))
				require.NoError(t, repo.SetRoles(ctx, target.ID, []int64{memberRole.ID}))
				// Gap discovery creates and confirms the real initial intent. The
				// DB watermark and Redis projection are never assigned by the test.
				service.RunProjectionSweepOnce(ctx, deps)
				snap, err := repo.SecuritySnapshot(ctx, target.ID)
				require.NoError(t, err)
				require.EqualValues(t, 1, snap.SessionEpoch)
				require.EqualValues(t, 1, snap.ProjectionEpoch)
				login := func(password, device string) (*model.TokenPair, error) {
					return auth.Login(ctx, &model.LoginRequest{EmployeeNo: target.EmployeeNo, Password: password, DeviceID: device}, "127.0.0.1", "security-state-test")
				}
				old, err := login("StateOld#2026", "old")
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, access(old.AccessToken).Code, "old session must work before the security command")
				denied := request(ctx, "/api/v1/users/status", old.AccessToken, false, userHandler.UpdateStatus,
					fmt.Sprintf(`{"user_id":%q,"status":0}`, fmt.Sprint(target.ID)))
				require.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
				require.Contains(t, denied.Body.String(), fmt.Sprintf(`"code":%d`, errcode.ErrNoPermission.Code))
				// A separate short deadline bounds the actual interleaving, so
				// a lock-order regression cannot consume the full bcrypt budget.
				interleavingCtx, cancelInterleaving := context.WithTimeout(ctx, 15*time.Second)
				defer cancelInterleaving()
				gate := &profileWriteGate{make(chan struct{}), make(chan struct{})}
				var releaseOnce sync.Once
				release := func() { releaseOnce.Do(func() { close(gate.release) }) }
				defer release()
				done := make(chan *httptest.ResponseRecorder, 1)
				name := "normal profile edit"
				go func() {
					if writer == "self_profile" {
						gctx := context.WithValue(interleavingCtx, profileWriteGateKey{}, gate)
						done <- request(gctx, "/api/v1/user/profile/update", old.AccessToken, true, userHandler.UpdateProfile, `{"real_name":"normal profile edit"}`)
					} else {
						// UserService.Update serializes its read and write under the IAM
						// guard, so pausing its SQL would block the security command.
						// Retain the independent narrow-column regression with an
						// authenticated, authorized admin request carrying a stale repo
						// model. The wait precedes repo.Update's IAM lock acquisition.
						done <- request(interleavingCtx, "/api/v1/users/update", actorPair.AccessToken, false, func(c *gin.Context) {
							user, err := repo.FindByID(c.Request.Context(), target.ID)
							if err == nil {
								user.RealName = name
								close(gate.reached)
								select {
								case <-gate.release:
								case <-c.Request.Context().Done():
								}
								err = repo.Update(c.Request.Context(), user)
							}
							if err != nil {
								c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
								return
							}
							response.OK(c, user)
						}, "")
					}
				}()
				select {
				case <-gate.reached:
				case <-interleavingCtx.Done():
					t.Fatal("profile did not reach SQL barrier")
				}
				var command *httptest.ResponseRecorder
				if kind == "disable" {
					command = request(interleavingCtx, "/api/v1/users/status", actorPair.AccessToken, false, userHandler.UpdateStatus,
						fmt.Sprintf(`{"user_id":%q,"status":0}`, fmt.Sprint(target.ID)))
				} else {
					command = request(interleavingCtx, "/api/v1/users/password/reset", actorPair.AccessToken, false, userHandler.ResetPassword,
						fmt.Sprintf(`{"user_id":%q,"password":"StateReset#2026"}`, fmt.Sprint(target.ID)))
				}
				require.Equal(t, http.StatusOK, command.Code, command.Body.String())
				require.NotEmpty(t, command.Header().Get("X-Operation-ID"))
				require.Equal(t, "completed", command.Header().Get("X-Operation-Status"))
				release()
				var written *httptest.ResponseRecorder
				select {
				case written = <-done:
				case <-interleavingCtx.Done():
					t.Fatal("profile did not finish after SQL barrier release")
				}
				cancelInterleaving()
				require.Equal(t, http.StatusOK, written.Code, written.Body.String())
				var profile struct {
					Data model.User `json:"data"`
				}
				require.NoError(t, json.Unmarshal(written.Body.Bytes(), &profile))
				current, err := repo.FindByID(ctx, target.ID)
				require.NoError(t, err)
				require.Equal(t, name, current.RealName, "normal profile edit persists")
				require.Equal(t, 2, current.Version)
				require.Equal(t, target.ID, profile.Data.ID)
				require.Equal(t, current.Status, profile.Data.Status, "write response uses persisted security state")
				require.Equal(t, current.MustChangePassword, profile.Data.MustChangePassword)
				snap, err = repo.SecuritySnapshot(ctx, target.ID)
				require.NoError(t, err)
				require.EqualValues(t, 2, snap.SessionEpoch, "profile cannot undo the security generation")
				require.EqualValues(t, 2, snap.ProjectionEpoch)
				if kind == "disable" {
					require.Zero(t, current.Status, "profile must not re-enable a disabled account")
					_, err = login("StateOld#2026", "disabled")
					require.ErrorIs(t, err, errcode.ErrInvalidCredentials)
					blocked := access(old.AccessToken)
					require.Equal(t, http.StatusForbidden, blocked.Code, blocked.Body.String())
					require.Contains(t, blocked.Body.String(), fmt.Sprintf(`"code":%d`, errcode.ErrUserDisabled.Code))
					// The dedicated management write must still work normally.
					enabled := request(ctx, "/api/v1/users/status", actorPair.AccessToken, false, userHandler.UpdateStatus,
						fmt.Sprintf(`{"user_id":%q,"status":1}`, fmt.Sprint(target.ID)))
					require.Equal(t, http.StatusOK, enabled.Code, enabled.Body.String())
					fresh, err := login("StateOld#2026", "reenabled")
					require.NoError(t, err)
					require.Equal(t, http.StatusOK, access(fresh.AccessToken).Code)
				} else {
					require.True(t, current.MustChangePassword, "profile must not clear a reset's forced change")
					_, err = login("StateOld#2026", "stale-password")
					require.ErrorIs(t, err, errcode.ErrInvalidCredentials, "profile cannot restore the previous password")
					fresh, err := login("StateReset#2026", "reset")
					require.NoError(t, err)
					require.True(t, fresh.MustChangePassword)
					blocked := access(fresh.AccessToken)
					require.Equal(t, http.StatusForbidden, blocked.Code, blocked.Body.String())
					require.Contains(t, blocked.Body.String(), fmt.Sprintf(`"code":%d`, errcode.ErrPasswordChangeRequired.Code))
					changed := request(ctx, "/api/v1/auth/password/update", fresh.AccessToken, true, authHandler.UpdatePassword,
						`{"old_password":"StateReset#2026","new_password":"StateFinal#2026","device_id":"reset"}`)
					require.Equal(t, http.StatusOK, changed.Code, changed.Body.String())
					var changedPair struct {
						Data model.TokenPair `json:"data"`
					}
					require.NoError(t, json.Unmarshal(changed.Body.Bytes(), &changedPair))
					require.NotEmpty(t, changedPair.Data.AccessToken)
					require.False(t, changedPair.Data.MustChangePassword)
					require.Equal(t, http.StatusOK, access(changedPair.Data.AccessToken).Code)
				}
				oldAccess := access(old.AccessToken)
				require.Equal(t, http.StatusUnauthorized, oldAccess.Code, "old AT stays revoked after legitimate new login")
				require.Contains(t, oldAccess.Body.String(), fmt.Sprintf(`"code":%d`, errcode.ErrTokenInvalid.Code))
				// CAS still protects concurrent profile edits (including admin edits).
				stale := request(ctx, "/api/v1/users/update", actorPair.AccessToken, false, userHandler.Update,
					fmt.Sprintf(`{"id":%q,"version":1,"real_name":"stale profile edit"}`, fmt.Sprint(target.ID)))
				require.Equal(t, http.StatusConflict, stale.Code, stale.Body.String())
				require.Contains(t, stale.Body.String(), fmt.Sprintf(`"code":%d`, errcode.ErrConcurrentModification.Code))
				afterCAS, err := repo.FindByID(ctx, target.ID)
				require.NoError(t, err)
				require.Equal(t, name, afterCAS.RealName, "rejected stale edit cannot overwrite profile")
				require.Equal(t, 2, afterCAS.Version)
				t.Logf("%s/%s: persisted status=%d forced_change=%t; auth/JWT and profile CAS controls passed", kind, writer, current.Status, current.MustChangePassword)
			})
		}
	}
}
