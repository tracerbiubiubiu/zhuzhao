//go:build integration

package service_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
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
		strings.Contains(data.SQL, "UPDATE users SET") && strings.Contains(data.SQL, "username = $2") {
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
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "redis:7-alpine", ExposedPorts: []string{"6379/tcp"},
			Labels:     map[string]string{"audit.owner": "ECO-20261104-B14", "zhuzhao.test": "user-security-state"},
			WaitingFor: wait.ForListeningPort("6379/tcp"),
		}, Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })
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
	cfg := config.JWTConfig{Secret: "security-state-regression-synthetic-secret", AccessTTL: time.Minute, RefreshTTL: time.Hour}
	mgr := jwtpkg.NewManager(jwtpkg.Config{Secret: cfg.Secret, AccessTTL: cfg.AccessTTL})
	auth := service.NewAuthService(repo, mgr, rdb, redispkg.NewScripts(rdb), service.NewAuditService(repository.NewAuditLogRepo(pool), repo), cfg, nil)
	users := service.NewUserService(pool, repo, roles, nil, rdb, mgr)
	hash, err := crypto.HashPassword("StateOld#2026")
	require.NoError(t, err)
	suffix := uniqueSuffix()
	actor := &model.User{Username: "state_manager", EmployeeNo: "E8M" + suffix, Password: hash, Status: 1}
	require.NoError(t, repo.Create(ctx, actor))
	role := &model.Role{Code: "state_manager_" + suffix, Name: "state manager", Priority: 10, Status: 1}
	require.NoError(t, roles.Create(ctx, role))
	require.NoError(t, repo.SetRoles(ctx, actor.ID, []int64{role.ID}))
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/business", middleware.JWT(mgr, rdb, nil), func(c *gin.Context) { c.Status(http.StatusOK) })
	access := func(token string) int {
		request := httptest.NewRequest(http.MethodGet, "/business", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response.Code
	}
	for _, kind := range []string{"disable", "reset"} {
		for _, writer := range []string{"self_profile", "admin_profile"} {
			t.Run(kind+"/"+writer, func(t *testing.T) {
				target := &model.User{Username: "state_target", EmployeeNo: "E8T" + uniqueSuffix(), Password: hash, Status: 1}
				require.NoError(t, repo.Create(ctx, target))
				login := func(password, device string) (*model.TokenPair, error) {
					return auth.Login(ctx, &model.LoginRequest{EmployeeNo: target.EmployeeNo, Password: password, DeviceID: device}, "127.0.0.1", "security-state-test")
				}
				old, err := login("StateOld#2026", "old")
				require.NoError(t, err)
				gate := &profileWriteGate{make(chan struct{}), make(chan struct{})}
				var releaseOnce sync.Once
				release := func() { releaseOnce.Do(func() { close(gate.release) }) }
				defer release()
				done := make(chan struct {
					user *model.User
					err  error
				}, 1)
				name := "normal profile edit"
				go func() {
					gctx := context.WithValue(ctx, profileWriteGateKey{}, gate)
					var user *model.User
					var err error
					if writer == "self_profile" {
						user, err = users.UpdateProfile(gctx, target.ID, &model.UpdateProfileRequest{RealName: &name})
					} else {
						user, err = users.Update(gctx, &model.UpdateUserRequest{ID: target.ID, Version: target.Version, RealName: &name}, actor.ID)
					}
					done <- struct {
						user *model.User
						err  error
					}{user, err}
				}()
				select {
				case <-gate.reached:
				case <-ctx.Done():
					t.Fatal("profile did not reach SQL barrier")
				}
				if kind == "disable" {
					require.NoError(t, users.UpdateStatus(ctx, &model.UpdateUserStatusRequest{UserID: target.ID, Status: 0}, actor.ID))
				} else {
					require.NoError(t, users.ResetPassword(ctx, &model.ResetPasswordRequest{UserID: target.ID, Password: "StateReset#2026"}, actor.ID))
				}
				release()
				var written struct {
					user *model.User
					err  error
				}
				select {
				case written = <-done:
				case <-ctx.Done():
					t.Fatal("profile did not finish after SQL barrier release")
				}
				require.NoError(t, written.err)
				current, err := repo.FindByID(ctx, target.ID)
				require.NoError(t, err)
				require.Equal(t, name, current.RealName, "normal profile edit persists")
				require.Equal(t, 2, current.Version)
				require.Equal(t, current.Status, written.user.Status, "write response uses persisted security state")
				require.Equal(t, current.MustChangePassword, written.user.MustChangePassword)
				if kind == "disable" {
					require.Zero(t, current.Status, "profile must not re-enable a disabled account")
					_, err = login("StateOld#2026", "disabled")
					require.ErrorIs(t, err, errcode.ErrInvalidCredentials)
					require.Equal(t, http.StatusForbidden, access(old.AccessToken))
					// The dedicated management write must still work normally.
					require.NoError(t, users.UpdateStatus(ctx, &model.UpdateUserStatusRequest{UserID: target.ID, Status: 1}, actor.ID))
					fresh, err := login("StateOld#2026", "reenabled")
					require.NoError(t, err)
					require.Equal(t, http.StatusOK, access(fresh.AccessToken))
				} else {
					require.True(t, current.MustChangePassword, "profile must not clear a reset's forced change")
					fresh, err := login("StateReset#2026", "reset")
					require.NoError(t, err)
					require.True(t, fresh.MustChangePassword)
					require.Equal(t, http.StatusForbidden, access(fresh.AccessToken))
					changed, err := auth.UpdatePassword(ctx, target.ID, "StateReset#2026", "StateFinal#2026", fresh.AccessToken, "reset")
					require.NoError(t, err)
					require.False(t, changed.MustChangePassword)
					require.Equal(t, http.StatusOK, access(changed.AccessToken))
				}
				require.Equal(t, http.StatusUnauthorized, access(old.AccessToken), "old AT stays revoked after legitimate new login")
				// CAS still protects concurrent profile edits (including admin edits).
				_, err = users.Update(ctx, &model.UpdateUserRequest{ID: target.ID, Version: 1, RealName: &name}, actor.ID)
				require.ErrorIs(t, err, errcode.ErrConcurrentModification)
				t.Logf("%s/%s: persisted status=%d forced_change=%t; auth/JWT and profile CAS controls passed", kind, writer, current.Status, current.MustChangePassword)
			})
		}
	}
}
