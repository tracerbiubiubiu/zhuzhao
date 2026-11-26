//go:build integration

package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

// epochScheduleHook fixes an interleaving at a real Redis command boundary.
// Its action performs a normally authorized command, never edits security keys.
type epochScheduleHook struct {
	once   sync.Once
	match  func(goredis.Cmder) bool
	after  bool
	action func()
}

func (h *epochScheduleHook) DialHook(next goredis.DialHook) goredis.DialHook { return next }
func (h *epochScheduleHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}
func (h *epochScheduleHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		if h.match(cmd) && !h.after {
			h.once.Do(h.action)
		}
		err := next(ctx, cmd)
		if err == nil && h.match(cmd) && h.after {
			h.once.Do(h.action)
		}
		return err
	}
}

type epochRegressionEnv struct {
	auth   *service.AuthService
	repo   *repository.UserRepo
	user   *model.User
	rdb    *goredis.Client
	mgr    *jwtpkg.Manager
	cfg    config.JWTConfig
	actor  *model.TokenPair
	engine *gin.Engine
	role   string
}

func newEpochRegressionAuth(t *testing.T) *epochRegressionEnv {
	t.Helper()
	resetAuthTables(t)
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	repo := repository.NewUserRepo(testPool)
	hash, err := crypto.HashPassword("EpochOld#2026")
	require.NoError(t, err)
	u := &model.User{Username: "epoch_race", EmployeeNo: "E800903", Password: hash, Status: 1}
	actor := &model.User{Username: "epoch_manager", EmployeeNo: "E800906", Password: hash, Status: 1}
	// Direct repository fixtures model accounts discovered by the production sweep.
	for _, user := range []*model.User{u, actor} {
		require.NoError(t, repo.Create(trustedIAMTestContext(), user))
	}
	roles := repository.NewRoleRepo(testPool)
	managerRole := &model.Role{Code: "epoch_manager_" + uniqueSuffix(), Name: "epoch manager", Status: 1, Priority: 10}
	viewerRole := &model.Role{Code: "epoch_viewer_" + uniqueSuffix(), Name: "epoch viewer", Status: 1, Priority: 30}
	for _, role := range []*model.Role{managerRole, viewerRole} {
		require.NoError(t, roles.Create(trustedIAMTestContext(), role))
	}
	require.NoError(t, repo.SetRoles(trustedIAMTestContext(), actor.ID, []int64{managerRole.ID}))
	require.NoError(t, repo.SetRoles(trustedIAMTestContext(), u.ID, []int64{viewerRole.ID}))
	for _, path := range []string{"/api/v1/users/password/reset", "/api/v1/users/status", "/api/v1/users/delete"} {
		_, err := testPool.Exec(context.Background(), `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,'POST')`, "role::"+managerRole.Code, path)
		require.NoError(t, err)
	}
	cfg := config.JWTConfig{Secret: "epoch-regression-synthetic-secret", AccessTTL: time.Minute, RefreshTTL: time.Hour}
	mgr := jwtpkg.NewManager(jwtpkg.Config{Secret: cfg.Secret, AccessTTL: cfg.AccessTTL})
	auth := service.NewAuthService(repo, mgr, rdb, redispkg.NewScripts(rdb), service.NewAuditService(repository.NewAuditLogRepo(testPool), repo), cfg, nil)
	service.RunProjectionSweepOnce(context.Background(), service.NewSessionWorkerDeps(testPool, repo, rdb, mgr.AccessTTL()))
	for _, user := range []*model.User{u, actor} {
		snap, err := repo.SecuritySnapshot(context.Background(), user.ID)
		require.NoError(t, err)
		require.EqualValues(t, 1, snap.SessionEpoch)
		require.EqualValues(t, 1, snap.ProjectionEpoch, "real discovery intent must be confirmed before login")
	}
	e := &epochRegressionEnv{auth: auth, repo: repo, user: u, rdb: rdb, mgr: mgr, cfg: cfg, role: managerRole.Code}
	e.engine = e.router(t, testPool, auth)
	e.actor, err = auth.Login(context.Background(), &model.LoginRequest{EmployeeNo: actor.EmployeeNo, Password: "EpochOld#2026", DeviceID: "manager"}, "127.0.0.1", "test")
	require.NoError(t, err)
	return e
}

func (e *epochRegressionEnv) router(t *testing.T, pool *pgxpool.Pool, auth *service.AuthService) *gin.Engine {
	t.Helper()
	m, err := casbinmodel.NewModelFromFile("../../configs/casbin_model.conf")
	require.NoError(t, err)
	enforcer, err := casbin.NewSyncedEnforcer(m)
	require.NoError(t, err)
	for _, path := range []string{"/api/v1/users/password/reset", "/api/v1/users/status", "/api/v1/users/delete"} {
		_, err := enforcer.AddPolicy("role::"+e.role, path, "POST")
		require.NoError(t, err)
	}
	repo, roles := repository.NewUserRepo(pool), repository.NewRoleRepo(pool)
	users := service.NewUserService(pool, repo, roles, nil, e.rdb, e.mgr)
	rbac := service.NewRBACService(roles, repo, nil, enforcer)
	h := handler.NewUserHandler(users, nil)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.JWT(e.mgr, e.rdb, repository.NewPatRepo(pool), middleware.WithAccountSecurity(c05SnapshotSource{pool: pool})))
	r.POST("/api/v1/users/password/reset", middleware.CasbinAuth(enforcer, rbac, nil), h.ResetPassword)
	r.POST("/api/v1/users/status", middleware.CasbinAuth(enforcer, rbac, nil), h.UpdateStatus)
	r.POST("/api/v1/users/delete", middleware.CasbinAuth(enforcer, rbac, nil), h.Delete)
	r.GET("/api/v1/users/status/result", middleware.SelfService(), middleware.CasbinAuth(enforcer, rbac, nil), h.OperationResult)
	r.POST("/api/v1/auth/password/update", middleware.SelfService(), middleware.CasbinAuth(enforcer, rbac, nil), handler.NewAuthHandler(auth, false).UpdatePassword)
	r.GET("/api/v1/user/profile", middleware.SelfService(), middleware.CasbinAuth(enforcer, rbac, nil), h.GetProfile)
	return r
}

// Authentication and Casbin mint the security facts for these requests.
func epochRequest(ctx context.Context, engine *gin.Engine, method, path, at, operationID string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+at)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Operation-ID", operationID)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func epochHTTPCode(t *testing.T, w *httptest.ResponseRecorder, status, code int) {
	t.Helper()
	var response struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, status, w.Code)
	require.Equal(t, code, response.Code)
}

func (e *epochRegressionEnv) login(t *testing.T, password, device string) *model.TokenPair {
	t.Helper()
	pair, err := e.auth.Login(context.Background(), &model.LoginRequest{EmployeeNo: e.user.EmployeeNo, Password: password, DeviceID: device}, "127.0.0.1", "test")
	require.NoError(t, err)
	return pair
}

func (e *epochRegressionEnv) command(ctx context.Context, engine *gin.Engine, path, operationID string) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"user_id":%q,"password":"EpochReset#2026","status":0}`, fmt.Sprint(e.user.ID))
	return epochRequest(ctx, engine, http.MethodPost, path, e.actor.AccessToken, operationID, []byte(body))
}

func (e *epochRegressionEnv) access(t *testing.T, at string, status, code int) {
	t.Helper()
	epochHTTPCode(t, epochRequest(context.Background(), e.engine, http.MethodGet, "/api/v1/user/profile", at, "", nil), status, code)
}

func TestAuthEpoch_UpdatePasswordInvalidatesOtherDevice(t *testing.T) {
	e := newEpochRegressionAuth(t)
	first, other := e.login(t, "EpochOld#2026", "first"), e.login(t, "EpochOld#2026", "other")
	e.access(t, other.AccessToken, 200, 0)
	w := epochRequest(context.Background(), e.engine, http.MethodPost, "/api/v1/auth/password/update", first.AccessToken, "epoch-change", []byte(`{"old_password":"EpochOld#2026","new_password":"EpochNew#2026","device_id":"first"}`))
	epochHTTPCode(t, w, 200, 0)
	require.Equal(t, "completed", w.Header().Get("X-Operation-Status"))
	var response struct {
		Data model.TokenPair `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	claims, err := e.mgr.ParseAccessToken(response.Data.AccessToken)
	require.NoError(t, err)
	require.Equal(t, jwtpkg.SessionSchemaDBEpochV1, claims.SessionSchema)
	require.Equal(t, "2", claims.SessionEpoch)
	e.access(t, other.AccessToken, 401, errcode.ErrTokenInvalid.Code)
	e.access(t, response.Data.AccessToken, 200, 0)
	_, err = e.auth.Refresh(context.Background(), other.RefreshToken, "127.0.0.1", "test")
	require.ErrorIs(t, err, errcode.ErrPasswordChanged)
}

// The gates only pause real queries. A reset/disable is always a signed HTTP command.
type epochQueryGate struct {
	once    sync.Once
	match   func(string) bool
	after   bool
	reached chan struct{}
	release chan struct{}
}
type epochQueryMatched struct{}

func (g *epochQueryGate) wait(ctx context.Context) {
	g.once.Do(func() {
		close(g.reached)
		select {
		case <-g.release:
		case <-ctx.Done():
		}
	})
}
func (g *epochQueryGate) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	matched := g.match(data.SQL)
	if matched && !g.after {
		g.wait(ctx)
	}
	return context.WithValue(ctx, epochQueryMatched{}, matched)
}
func (g *epochQueryGate) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if matched, _ := ctx.Value(epochQueryMatched{}).(bool); matched && g.after {
		g.wait(ctx)
	}
}

func epochWait(t *testing.T, ctx context.Context, reached <-chan struct{}) {
	t.Helper()
	select {
	case <-reached:
	case <-ctx.Done():
		t.Fatal("expected query boundary was not reached")
	}
}

func (e *epochRegressionEnv) tracedAuth(t *testing.T, ctx context.Context, tracer pgx.QueryTracer) (*pgxpool.Pool, *service.AuthService) {
	t.Helper()
	cfg := testPool.Config().Copy()
	cfg.ConnConfig.Tracer = tracer
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	repo := repository.NewUserRepo(pool)
	auth := service.NewAuthService(repo, e.mgr, e.rdb, redispkg.NewScripts(e.rdb), service.NewAuditService(repository.NewAuditLogRepo(pool), repo), e.cfg, nil)
	return pool, auth
}

func TestAuthEpoch_LoginCannotBorrowPostResetEpoch(t *testing.T) {
	e := newEpochRegressionAuth(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	gate := &epochQueryGate{match: func(sql string) bool { return strings.Contains(sql, "UPDATE users SET last_login_at") }, after: true, reached: make(chan struct{}), release: make(chan struct{})}
	var once sync.Once
	defer once.Do(func() { close(gate.release) })
	_, auth := e.tracedAuth(t, ctx, gate)
	type result struct {
		pair *model.TokenPair
		err  error
	}
	done := make(chan result, 1)
	go func() {
		pair, err := auth.Login(ctx, &model.LoginRequest{EmployeeNo: e.user.EmployeeNo, Password: "EpochOld#2026", DeviceID: "old"}, "127.0.0.1", "test")
		done <- result{pair, err}
	}()
	epochWait(t, ctx, gate.reached)
	w := e.command(ctx, e.engine, "/api/v1/users/password/reset", "epoch-login-reset")
	epochHTTPCode(t, w, 200, 0)
	require.Equal(t, "completed", w.Header().Get("X-Operation-Status"))
	once.Do(func() { close(gate.release) })
	select {
	case got := <-done:
		require.ErrorIs(t, got.err, errcode.ErrInvalidCredentials)
		require.Nil(t, got.pair, "credentials verified before reset must not borrow the new DB epoch")
	case <-ctx.Done():
		t.Fatal("login did not finish")
	}
	fresh := e.login(t, "EpochReset#2026", "after-reset")
	claims, err := e.mgr.ParseAccessToken(fresh.AccessToken)
	require.NoError(t, err)
	require.Equal(t, "2", claims.SessionEpoch)
	require.True(t, fresh.MustChangePassword)
	e.access(t, fresh.AccessToken, 403, errcode.ErrPasswordChangeRequired.Code)
}

func TestAuthEpoch_RefreshBindsConsumptionEpoch(t *testing.T) {
	e := newEpochRegressionAuth(t)
	ctx := context.Background()
	pair := e.login(t, "EpochOld#2026", "device")
	fired := false
	e.rdb.AddHook(&epochScheduleHook{
		after: true,
		match: func(cmd goredis.Cmder) bool {
			return cmd.Name() == "eval" && strings.Contains(fmt.Sprint(cmd.Args()[1]), "local old = redis.call('GET', KEYS[1])")
		},
		action: func() {
			fired = true
			epochHTTPCode(t, e.command(ctx, e.engine, "/api/v1/users/password/reset", "epoch-refresh-reset"), 200, 0)
		},
	})
	fresh, err := e.auth.Refresh(ctx, pair.RefreshToken, "127.0.0.1", "test")
	require.NoError(t, err)
	require.True(t, fired, "the reset must commit after the real RT rotation")
	claims, err := e.mgr.ParseAccessToken(fresh.AccessToken)
	require.NoError(t, err)
	require.Equal(t, jwtpkg.SessionSchemaDBEpochV1, claims.SessionSchema)
	require.Equal(t, "1", claims.SessionEpoch, "a consumed RT cannot borrow the reset's new epoch")
	e.access(t, fresh.AccessToken, 401, errcode.ErrTokenInvalid.Code)
	_, err = e.auth.Refresh(ctx, fresh.RefreshToken, "127.0.0.1", "test")
	require.ErrorIs(t, err, errcode.ErrPasswordChanged)
}

type failOneBlacklistWrite struct{ once sync.Once }

func (h *failOneBlacklistWrite) DialHook(next goredis.DialHook) goredis.DialHook { return next }
func (h *failOneBlacklistWrite) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}
func (h *failOneBlacklistWrite) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		if cmd.Name() == "set" && strings.HasPrefix(fmt.Sprint(cmd.Args()[1]), "blacklist:at:") {
			reject := false
			h.once.Do(func() { reject = true })
			if reject {
				return errors.New("synthetic one-command blacklist outage")
			}
		}
		return next(ctx, cmd)
	}
}

func TestAuthEpoch_BlacklistFailureCannotSkipGlobalRevocation(t *testing.T) {
	e := newEpochRegressionAuth(t)
	first, other := e.login(t, "EpochOld#2026", "first"), e.login(t, "EpochOld#2026", "other")
	e.rdb.AddHook(&failOneBlacklistWrite{})
	w := epochRequest(context.Background(), e.engine, http.MethodPost, "/api/v1/auth/password/update", first.AccessToken, "epoch-blacklist-failure", []byte(`{"old_password":"EpochOld#2026","new_password":"EpochNew#2026","device_id":"first"}`))
	epochHTTPCode(t, w, 503, errcode.ErrServiceUnavailable.Code)
	require.Equal(t, "epoch-blacklist-failure", w.Header().Get("X-Operation-ID"))
	require.Equal(t, "completed", w.Header().Get("X-Operation-Status"), "the password commit survives the single blacklist failure")
	snap, err := e.repo.SecuritySnapshot(context.Background(), e.user.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, snap.SessionEpoch)
	require.EqualValues(t, 2, snap.ProjectionEpoch)
	e.access(t, other.AccessToken, 401, errcode.ErrTokenInvalid.Code)
	fresh := e.login(t, "EpochNew#2026", "new")
	e.access(t, fresh.AccessToken, 200, 0)
	e.access(t, other.AccessToken, 401, errcode.ErrTokenInvalid.Code)
	_, err = e.auth.Refresh(context.Background(), other.RefreshToken, "127.0.0.1", "test")
	require.ErrorIs(t, err, errcode.ErrPasswordChanged)
}

func TestAuthEpoch_UserLifecycleCannotReviveOldAT(t *testing.T) {
	for _, action := range []string{"disable_enable", "delete"} {
		t.Run(action, func(t *testing.T) {
			e := newEpochRegressionAuth(t)
			ctx := context.Background()
			old := e.login(t, "EpochOld#2026", "old")
			e.access(t, old.AccessToken, 200, 0)
			if action == "disable_enable" {
				epochHTTPCode(t, e.command(ctx, e.engine, "/api/v1/users/status", "epoch-disable"), 200, 0)
				e.access(t, old.AccessToken, 403, errcode.ErrUserDisabled.Code)
				body := []byte(fmt.Sprintf(`{"user_id":%q,"status":1}`, fmt.Sprint(e.user.ID)))
				epochHTTPCode(t, epochRequest(ctx, e.engine, http.MethodPost, "/api/v1/users/status", e.actor.AccessToken, "epoch-enable", body), 200, 0)
				e.access(t, old.AccessToken, 401, errcode.ErrTokenInvalid.Code)
				fresh := e.login(t, "EpochOld#2026", "new")
				e.access(t, fresh.AccessToken, 200, 0)
				snap, err := e.repo.SecuritySnapshot(ctx, e.user.ID)
				require.NoError(t, err)
				require.EqualValues(t, 3, snap.SessionEpoch)
				require.EqualValues(t, 3, snap.ProjectionEpoch)
			} else {
				epochHTTPCode(t, e.command(ctx, e.engine, "/api/v1/users/delete", "epoch-delete"), 200, 0)
				// Expiry/loss of a Redis disabled projection must never revive a deleted DB account.
				removed, err := e.rdb.Del(ctx, fmt.Sprintf("user:session_disabled:db1:%d", e.user.ID)).Result()
				require.NoError(t, err)
				require.EqualValues(t, 1, removed, "exercise actual loss of the installed disabled projection")
				e.access(t, old.AccessToken, 401, errcode.ErrTokenInvalid.Code)
			}
		})
	}
}

// Compare the entire session state against only the authorized management
// effect, preserving the old "rejected writes change no sessions" criterion.
func epochRedisState(t *testing.T, ctx context.Context, rdb *goredis.Client) map[string]string {
	t.Helper()
	keys, err := rdb.Keys(ctx, "*").Result()
	require.NoError(t, err)
	state := make(map[string]string, len(keys))
	for _, key := range keys {
		value, err := rdb.Get(ctx, key).Result()
		require.NoError(t, err)
		state[key] = value
	}
	return state
}

func TestAuthEpoch_ChangePasswordUsesCurrentCredential(t *testing.T) {
	for _, change := range []string{"admin reset", "account disabled"} {
		t.Run(change, func(t *testing.T) {
			e := newEpochRegressionAuth(t)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			old := e.login(t, "EpochOld#2026", "test-device")
			expectedSessions := epochRedisState(t, ctx, e.rdb)
			// The real management command holds IAM and the target row before writing.
			gate := &epochQueryGate{match: func(sql string) bool {
				return strings.Contains(sql, "UPDATE users SET") && (strings.Contains(sql, "password = $3") || strings.Contains(sql, "status = $3"))
			}, reached: make(chan struct{}), release: make(chan struct{})}
			var once sync.Once
			defer once.Do(func() { close(gate.release) })
			adminPool, adminAuth := e.tracedAuth(t, ctx, gate)
			adminRouter := e.router(t, adminPool, adminAuth)
			path := "/api/v1/users/password/reset"
			if change == "account disabled" {
				path = "/api/v1/users/status"
			}
			adminDone := make(chan *httptest.ResponseRecorder, 1)
			go func() { adminDone <- e.command(ctx, adminRouter, path, "epoch-admin-first") }()
			epochWait(t, ctx, gate.reached)
			trace := &epochQueryGate{match: func(sql string) bool { return strings.Contains(sql, "pg_advisory_lock(") }, reached: make(chan struct{}), release: make(chan struct{})}
			close(trace.release)
			selfPool, selfAuth := e.tracedAuth(t, ctx, trace)
			selfRouter := e.router(t, selfPool, selfAuth)
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				done <- epochRequest(ctx, selfRouter, http.MethodPost, "/api/v1/auth/password/update", old.AccessToken, "epoch-rejected-self", []byte(`{"old_password":"EpochOld#2026","new_password":"EpochNew#2026","device_id":"test-device"}`))
			}()
			epochWait(t, ctx, trace.reached)
			once.Do(func() { close(gate.release) })
			select {
			case w := <-adminDone:
				epochHTTPCode(t, w, 200, 0)
			case <-ctx.Done():
				t.Fatal("management command did not finish")
			}
			select {
			case w := <-done:
				epochHTTPCode(t, w, 401, errcode.ErrInvalidCredentials.Code)
			case <-ctx.Done():
				t.Fatal("self change did not finish")
			}
			snap, err := e.repo.SecuritySnapshot(ctx, e.user.ID)
			require.NoError(t, err)
			require.EqualValues(t, 2, snap.SessionEpoch, "only the authorized management command advances the epoch")
			require.EqualValues(t, 2, snap.ProjectionEpoch)
			require.Equal(t, change == "admin reset", snap.MustChangePassword)
			if change == "admin reset" {
				require.True(t, crypto.CheckPassword("EpochReset#2026", snap.PasswordHash))
			} else {
				require.Zero(t, snap.Status)
				require.Equal(t, e.user.Password, snap.PasswordHash)
			}
			var rejectedOps int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM user_session_operations WHERE operation_id='epoch-rejected-self'`).Scan(&rejectedOps))
			require.Zero(t, rejectedOps, "the rejected write must not create a receipt or issue a new session")
			keys, err := e.rdb.Keys(ctx, fmt.Sprintf("refresh:db1:%d:2:*", e.user.ID)).Result()
			require.NoError(t, err)
			require.Empty(t, keys)
			expectedSessions[fmt.Sprintf("user:session_epoch:db1:%d", e.user.ID)] = "2"
			if change == "account disabled" {
				expectedSessions[fmt.Sprintf("user:session_disabled:db1:%d", e.user.ID)] = "1"
			}
			require.Equal(t, expectedSessions, epochRedisState(t, ctx, e.rdb), "the rejected self change must add, remove or overwrite no session state")
		})
	}
}
