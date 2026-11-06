//go:build integration

package service_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// epochScheduleHook 在真实 Redis 命令边界固定交错，不用低概率并发抢时序。
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

func newEpochRegressionAuth(t *testing.T) (*service.AuthService, *repository.UserRepo, *model.User, *goredis.Client, *jwtpkg.Manager) {
	t.Helper()
	resetAuthTables(t)
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	repo := repository.NewUserRepo(testPool)
	hash, err := crypto.HashPassword("EpochOld#2026")
	require.NoError(t, err)
	u := &model.User{Username: "epoch_race", EmployeeNo: "E800903", Password: hash, Status: 1}
	require.NoError(t, repo.Create(context.Background(), u))
	cfg := config.JWTConfig{Secret: "epoch-regression-synthetic-secret", AccessTTL: time.Minute, RefreshTTL: time.Hour}
	mgr := jwtpkg.NewManager(jwtpkg.Config{Secret: cfg.Secret, AccessTTL: cfg.AccessTTL})
	auth := service.NewAuthService(repo, mgr, rdb, redispkg.NewScripts(rdb), service.NewAuditService(repository.NewAuditLogRepo(testPool), repo), cfg, nil)
	return auth, repo, u, rdb, mgr
}

func epochAccessStatus(t *testing.T, mgr *jwtpkg.Manager, rdb *goredis.Client, at string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	c.Request.Header.Set("Authorization", "Bearer "+at)
	middleware.JWT(mgr, rdb, nil)(c)
	return w.Code
}

func TestAuthEpoch_UpdatePasswordInvalidatesOtherDevice(t *testing.T) {
	auth, _, u, rdb, mgr := newEpochRegressionAuth(t)
	ctx := context.Background()
	login := func(device string) *model.TokenPair {
		pair, err := auth.Login(ctx, &model.LoginRequest{EmployeeNo: u.EmployeeNo, Password: "EpochOld#2026", DeviceID: device}, "127.0.0.1", "test")
		require.NoError(t, err)
		return pair
	}
	first, other := login("first"), login("other")
	fresh, err := auth.UpdatePassword(ctx, u.ID, "EpochOld#2026", "EpochNew#2026", first.AccessToken, "first")
	require.NoError(t, err)
	claims, err := mgr.ParseAccessToken(fresh.AccessToken)
	require.NoError(t, err)
	require.EqualValues(t, 1, claims.Pwe)
	require.Equal(t, 401, epochAccessStatus(t, mgr, rdb, other.AccessToken))
	require.Equal(t, 200, epochAccessStatus(t, mgr, rdb, fresh.AccessToken))
}

func TestAuthEpoch_LoginCannotBorrowPostResetEpoch(t *testing.T) {
	auth, repo, u, rdb, _ := newEpochRegressionAuth(t)
	ctx := context.Background()
	hash, err := crypto.HashPassword("EpochReset#2026")
	require.NoError(t, err)
	rdb.AddHook(&epochScheduleHook{
		match: func(cmd goredis.Cmder) bool {
			return cmd.Name() == "get" && fmt.Sprint(cmd.Args()[1]) == fmt.Sprintf("user:pw_epoch:%d", u.ID)
		},
		action: func() {
			// Login 已取旧凭据，模拟另一撤销在读纪元之前完成 DB+INCR。
			require.NoError(t, repo.UpdatePassword(ctx, u.ID, hash, true))
			require.NoError(t, rdb.Incr(ctx, fmt.Sprintf("user:pw_epoch:%d", u.ID)).Err())
			require.NoError(t, rdb.Set(ctx, fmt.Sprintf("user:disabled:%d", u.ID), 1, time.Minute).Err())
		},
	})
	pair, err := auth.Login(ctx, &model.LoginRequest{EmployeeNo: u.EmployeeNo, Password: "EpochOld#2026", DeviceID: "old"}, "127.0.0.1", "test")
	require.ErrorIs(t, err, errcode.ErrInvalidCredentials)
	require.Nil(t, pair)
	require.EqualValues(t, 1, rdb.Exists(ctx, fmt.Sprintf("user:disabled:%d", u.ID)).Val())
}

func TestAuthEpoch_RefreshBindsConsumptionEpoch(t *testing.T) {
	auth, _, u, rdb, mgr := newEpochRegressionAuth(t)
	ctx := context.Background()
	pair, err := auth.Login(ctx, &model.LoginRequest{EmployeeNo: u.EmployeeNo, Password: "EpochOld#2026", DeviceID: "device"}, "127.0.0.1", "test")
	require.NoError(t, err)
	rdb.AddHook(&epochScheduleHook{
		after: true,
		match: func(cmd goredis.Cmder) bool {
			return cmd.Name() == "eval" && strings.Contains(fmt.Sprint(cmd.Args()[1]), "GETDEL")
		},
		action: func() { require.NoError(t, rdb.Incr(ctx, fmt.Sprintf("user:pw_epoch:%d", u.ID)).Err()) },
	})
	fresh, err := auth.Refresh(ctx, pair.RefreshToken, "127.0.0.1", "test")
	require.NoError(t, err)
	claims, err := mgr.ParseAccessToken(fresh.AccessToken)
	require.NoError(t, err)
	require.Zero(t, claims.Pwe, "绑定消费时刻，不借用撤销后的新纪元")
	require.Equal(t, 401, epochAccessStatus(t, mgr, rdb, fresh.AccessToken))
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
	auth, _, u, rdb, mgr := newEpochRegressionAuth(t)
	ctx := context.Background()
	first, err := auth.Login(ctx, &model.LoginRequest{EmployeeNo: u.EmployeeNo, Password: "EpochOld#2026", DeviceID: "first"}, "127.0.0.1", "test")
	require.NoError(t, err)
	other, err := auth.Login(ctx, &model.LoginRequest{EmployeeNo: u.EmployeeNo, Password: "EpochOld#2026", DeviceID: "other"}, "127.0.0.1", "test")
	require.NoError(t, err)
	rdb.AddHook(&failOneBlacklistWrite{})
	_, err = auth.UpdatePassword(ctx, u.ID, "EpochOld#2026", "EpochNew#2026", first.AccessToken, "first")
	require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
	// DB 已改密；模拟 Redis 单命令失败后恢复，新密码登录不能使其他设备旧 AT 幸存。
	_, err = auth.Login(ctx, &model.LoginRequest{EmployeeNo: u.EmployeeNo, Password: "EpochNew#2026", DeviceID: "new"}, "127.0.0.1", "test")
	require.NoError(t, err)
	require.Equal(t, 401, epochAccessStatus(t, mgr, rdb, other.AccessToken))
}

func TestAuthEpoch_UserLifecycleCannotReviveOldAT(t *testing.T) {
	for _, action := range []string{"disable_enable", "delete"} {
		t.Run(action, func(t *testing.T) {
			auth, repo, u, rdb, mgr := newEpochRegressionAuth(t)
			ctx := context.Background()
			actor := &model.User{Username: "epoch_manager", EmployeeNo: "E800906", Password: u.Password, Status: 1}
			require.NoError(t, repo.Create(ctx, actor))
			_, err := testPool.Exec(ctx, `INSERT INTO roles(code,name,status,priority) SELECT 'admin','epoch admin',1,10 WHERE NOT EXISTS(SELECT 1 FROM roles WHERE code='admin' AND deleted_at IS NULL)`)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE code='admin' AND deleted_at IS NULL`, actor.ID)
			require.NoError(t, err)
			users := service.NewUserService(testPool, repo, repository.NewRoleRepo(testPool), nil, rdb, mgr)
			old, err := auth.Login(ctx, &model.LoginRequest{EmployeeNo: u.EmployeeNo, Password: "EpochOld#2026", DeviceID: "old"}, "127.0.0.1", "test")
			require.NoError(t, err)
			if action == "disable_enable" {
				require.NoError(t, users.UpdateStatus(ctx, &model.UpdateUserStatusRequest{UserID: u.ID, Status: 0}, actor.ID))
				require.Equal(t, 403, epochAccessStatus(t, mgr, rdb, old.AccessToken))
				require.NoError(t, users.UpdateStatus(ctx, &model.UpdateUserStatusRequest{UserID: u.ID, Status: 1}, actor.ID))
				require.Equal(t, 401, epochAccessStatus(t, mgr, rdb, old.AccessToken))
				fresh, err := auth.Login(ctx, &model.LoginRequest{EmployeeNo: u.EmployeeNo, Password: "EpochOld#2026", DeviceID: "new"}, "127.0.0.1", "test")
				require.NoError(t, err)
				require.Equal(t, 200, epochAccessStatus(t, mgr, rdb, fresh.AccessToken))
			} else {
				require.NoError(t, users.Delete(ctx, u.ID, actor.ID))
				// disabled 的 TTL 到期后，永久纪元仍须挡住尚未过期的旧 AT。
				require.NoError(t, rdb.Del(ctx, fmt.Sprintf("user:disabled:%d", u.ID)).Err())
				require.Equal(t, 401, epochAccessStatus(t, mgr, rdb, old.AccessToken))
			}
		})
	}
}
