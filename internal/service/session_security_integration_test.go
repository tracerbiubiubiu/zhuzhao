//go:build integration

package service_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
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

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

// c05EvalFault 定向故障注入：只拦 Redis EVAL（投影安装 Lua），读命令不受影响——
// 「Redis 旧值仍可读」的断言要求故障期间读路径存活（SetError 全局拦会误伤证据读取）。
type c05EvalFault struct {
	on  atomic.Bool
	err string
}

func (f *c05EvalFault) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		if f.on.Load() && (cmd.Name() == "eval" || cmd.Name() == "evalsha") {
			return errors.New(f.err)
		}
		return next(ctx, cmd)
	}
}
func (f *c05EvalFault) DialHook(next goredis.DialHook) goredis.DialHook { return next }
func (f *c05EvalFault) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}

func config_JWT() config.JWTConfig {
	return config.JWTConfig{Secret: "c05-session-security-synthetic-secret", AccessTTL: time.Minute, RefreshTTL: time.Hour}
}

// ---- C05 会话安全切片（底座 §4.2/§4.4、卡一）定向红绿 ----

const c05Password = "C05Session#2026"

// ensureSuperadminRole 幂等建 superadmin 角色（测试库不含种子）。
func ensureSuperadminRole(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO roles (code, name, description, priority, is_system)
		SELECT 'superadmin', '超级管理员', 'session-security 测试角色', 1, true
		WHERE NOT EXISTS (SELECT 1 FROM roles WHERE code = 'superadmin' AND deleted_at IS NULL)`)
	require.NoError(t, err)
}

// c05Env C05 切片测试环境（真实 PG + miniredis；DB 是安全事实，Redis 是投影）。
type c05Env struct {
	userSvc *service.UserService
	authSvc *service.AuthService
	repo    *repository.UserRepo
	deps    service.SessionSecurityDeps
	rdb     *goredis.Client
	mr      *miniredis.Miniredis
	jwt     *jwtpkg.Manager
}

func newC05Env(t *testing.T) *c05Env {
	t.Helper()
	ensureSuperadminRole(t)
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	repo := repository.NewUserRepo(testPool)
	cfg := config_JWT()
	mgr := jwtpkg.NewManager(jwtpkg.Config{Secret: cfg.Secret, AccessTTL: cfg.AccessTTL})
	auth := service.NewAuthService(repo, mgr, rdb, redispkg.NewScripts(rdb),
		service.NewAuditService(repository.NewAuditLogRepo(testPool), repo), cfg, nil)
	dep := service.NewSessionWorkerDeps(testPool, repo, rdb, mgr.AccessTTL())
	userSvc := service.NewUserService(testPool, repo, repository.NewRoleRepo(testPool), nil, rdb, mgr)
	return &c05Env{userSvc: userSvc, authSvc: auth, repo: repo, deps: dep, rdb: rdb, mr: mr, jwt: mgr}
}

// c05User 直建用户（绕过服务层——存量直建形态，未切换账号）。
func (e *c05Env) c05User(t *testing.T, employeeNo string, status int) *model.User {
	t.Helper()
	hash, err := crypto.HashPassword(c05Password)
	require.NoError(t, err)
	u := &model.User{Username: "c05-" + employeeNo, EmployeeNo: employeeNo, Password: hash, Status: status}
	require.NoError(t, e.repo.Create(trustedIAMTestContext(), u))
	return u
}

// c05Superadmin 建 superadmin 操作者。
func (e *c05Env) c05Superadmin(t *testing.T, employeeNo string) *model.User {
	t.Helper()
	u := e.c05User(t, employeeNo, 1)
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, r.id FROM roles r WHERE r.code = 'superadmin' AND r.deleted_at IS NULL
		AND NOT EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $1 AND ur.role_id = r.id)`, u.ID)
	require.NoError(t, err)
	return u
}

func (e *c05Env) login(t *testing.T, employeeNo, device string) *model.TokenPair {
	t.Helper()
	pair, err := e.authSvc.Login(trustedIAMTestContext(), &model.LoginRequest{
		EmployeeNo: employeeNo, Password: c05Password, DeviceID: device,
	}, "127.0.0.1", "c05-test")
	require.NoError(t, err)
	return pair
}

// bearerStatus 以账号快照装配（与 router 同构）跑 JWT 中间件。
func (e *c05Env) bearerStatus(t *testing.T, at string) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	c.Request.Header.Set("Authorization", "Bearer "+at)
	middleware.JWT(e.jwt, e.rdb, nil, middleware.WithAccountSecurity(c05SnapshotSource{pool: testPool}))(c)
	return w.Code, w.Body.String()
}

// c05SnapshotSource 与 router 装配桥同构的最小适配器。
type c05SnapshotSource struct{ pool *pgxpool.Pool }

func (s c05SnapshotSource) AccountSnapshot(ctx context.Context, userID int64) (middleware.AccountSnapshot, error) {
	snap, err := repository.NewAccountSecurityLookup(s.pool).AccountSecuritySnapshot(ctx, userID)
	if err != nil {
		return middleware.AccountSnapshot{}, err
	}
	return middleware.AccountSnapshot{Status: snap.Status, Deleted: snap.Deleted, SessionEpoch: snap.SessionEpoch, ProjectionEpoch: snap.ProjectionEpoch}, nil
}

// 1) 卡一首条红例：停用已提交而撤销投影持续失败——DB 事实/回执/意图留存，
// Redis 旧值可读；旧 AT/仅 cookie/RT/重登各按 DB 事实终态拒绝；对照账户不受通用故障截断。
func TestC05_DisableCommitted_ProjectionPending_TerminalRejects(t *testing.T) {
	e := newC05Env(t)
	ctx := trustedIAMTestContext()
	actor := e.c05Superadmin(t, "C059001")
	target := e.c05User(t, "C059002", 1)
	control := e.c05User(t, "C059003", 1)

	// 前置通过：签发可用凭据（创建后投影缺项由首轮 sweep 确认——切换就绪）
	service.RunProjectionSweepOnce(ctx, e.deps)
	targetPair := e.login(t, target.EmployeeNo, "dev1")
	targetAT := targetPair.AccessToken
	controlAT := e.login(t, control.EmployeeNo, "dev1").AccessToken
	code, _ := e.bearerStatus(t, targetAT)
	require.Equal(t, 200, code, "前置：停用前旧 AT 必须可用（对照不得被通用故障截断）")

	// 定向故障：只拦投影安装 EVAL（读命令存活，DB 正常）
	fault := &c05EvalFault{err: "c05-injected-projection-failure"}
	e.rdb.AddHook(fault)
	fault.on.Store(true)
	res, err := e.userSvc.UpdateStatus(ctx, &model.UpdateUserStatusRequest{UserID: target.ID, Status: 0}, actor.ID, "K-C05-1")
	require.Error(t, err, "投影失败必须上抛（可恢复 503）")
	require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
	require.NotNil(t, res, "回执已提交，结果必须随错误返回供头部回填")
	require.Equal(t, "K-C05-1", res.OperationID)

	// PG 事实：停用 + 新代际 + 原回执 + 唯一 pending 意图
	snap, err := e.repo.SecuritySnapshot(ctx, target.ID)
	require.NoError(t, err)
	require.Equal(t, 0, snap.Status)
	require.EqualValues(t, 2, snap.SessionEpoch)
	require.EqualValues(t, 1, snap.ProjectionEpoch, "投影水位仍停在前代")
	op, err := repository.NewSessionOperationRepo(testPool).FindOperationByID(ctx, "K-C05-1")
	require.NoError(t, err)
	require.Equal(t, "completed", op.Status) // DB 回执已提交
	pending, err := repository.NewSessionOperationRepo(testPool).PendingIntentCountForUser(ctx, target.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, pending, "唯一待同步意图")

	// Redis 旧值仍可读、disabled 未写入（写入被定向故障挡下）
	epochStr, err := e.rdb.Get(ctx, "user:session_epoch:db1:"+itoa(target.ID)).Result()
	require.NoError(t, err)
	require.Equal(t, "1", epochStr, "Redis 保留旧代投影")
	n, _ := e.rdb.Exists(ctx, "user:session_disabled:db1:"+itoa(target.ID)).Result()
	require.Zero(t, n, "撤销投影未写入")

	// 旧 AT 终态拒绝（DB 快照：inactive → 30003），不签新令牌
	code, body := e.bearerStatus(t, targetAT)
	require.Equal(t, 403, code)
	require.Contains(t, body, `"code":30003`)

	// 仅文件 cookie（无 header）同样终态拒绝
	{
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/files/list", nil)
		c.Request.AddCookie(&http.Cookie{Name: middleware.FileSessionCookieName, Value: targetAT})
		middleware.FileAuth(middleware.JWT(e.jwt, e.rdb, nil,
			middleware.WithAccountSecurity(c05SnapshotSource{pool: testPool})))(c)
		require.True(t, c.IsAborted())
		require.Equal(t, 403, w.Code)
	}

	// RT：无新令牌（账号停用分支拒绝）
	fault.on.Store(false) // 清故障：RT 判定走 DB 账号状态，投影不自动重试
	pair, err := e.authSvc.Refresh(ctx, targetPair.RefreshToken, "127.0.0.1", "c05-test")
	require.Error(t, err)
	require.Nil(t, pair)
	require.Equal(t, errcode.ErrRefreshTokenInvalid, err)

	// 重新登录被拒（停用分支，不比对密码语义与既有对齐）
	_, err = e.authSvc.Login(ctx, &model.LoginRequest{EmployeeNo: target.EmployeeNo, Password: c05Password}, "127.0.0.1", "c05-test")
	require.ErrorIs(t, err, errcode.ErrInvalidCredentials)

	// 对照账户：同线路正常通过（证明探针未被通用故障截断）
	code, _ = e.bearerStatus(t, controlAT)
	require.Equal(t, 200, code, "旁路对照必须仍可用")

	// 原操作号受权核查：已提交 + 投影待恢复
	view, err := e.userSvc.GetOperationResult(ctx, "K-C05-1", actor.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", view.Status)
	require.True(t, view.ProjectionPending)
	require.EqualValues(t, 2, view.ResultEpoch)
	replay, err := e.userSvc.UpdateStatus(ctx, &model.UpdateUserStatusRequest{UserID: target.ID, Status: 0}, actor.ID, "K-C05-1")
	require.NoError(t, err)
	require.Equal(t, "pending", replay.Status, "同 K 重交须呈现原代际的真实投影状态")

	// 故障保持期间启用被安全门拒绝且保持停用
	fault.on.Store(true)
	_, err = e.userSvc.UpdateStatus(ctx, &model.UpdateUserStatusRequest{UserID: target.ID, Status: 1}, actor.ID, "K-C05-2")
	require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
	snap2, _ := e.repo.SecuritySnapshot(ctx, target.ID)
	require.Equal(t, 0, snap2.Status, "待恢复时启用必须保持停用")
	require.EqualValues(t, 2, snap2.SessionEpoch, "拒绝启用不得推进代际")

	// 原同步意图恢复：解除故障 → 单轮 sweep 按原意图投影确认（代际/事实不增）
	fault.on.Store(false)
	service.RunProjectionSweepOnce(ctx, e.deps)
	snap3, err := e.repo.SecuritySnapshot(ctx, target.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, snap3.SessionEpoch)
	require.EqualValues(t, 2, snap3.ProjectionEpoch, "投影水位推进到原目标代")
	n, _ = e.rdb.Exists(ctx, "user:session_disabled:db1:"+itoa(target.ID)).Result()
	require.EqualValues(t, 1, n, "确认后 disabled 投影就位")
	pending2, _ := repository.NewSessionOperationRepo(testPool).PendingIntentCountForUser(ctx, target.ID)
	require.Zero(t, pending2, "原意图确认后不再 pending")

	// 确认后启用放行并推进新代，旧凭据仍拒绝
	res3, err := e.userSvc.UpdateStatus(ctx, &model.UpdateUserStatusRequest{UserID: target.ID, Status: 1}, actor.ID, "K-C05-3")
	require.NoError(t, err)
	require.Equal(t, "completed", res3.Status)
	require.EqualValues(t, 3, res3.Epoch)
	service.RunProjectionSweepOnce(ctx, e.deps)
	code, body = e.bearerStatus(t, targetAT)
	require.Equal(t, 401, code, "旧 AT 跨代际持续拒绝（已启用后归因代际失效而非停用）")
	require.Contains(t, body, `"code":20003`)
	_, err = e.authSvc.Login(ctx, &model.LoginRequest{EmployeeNo: target.EmployeeNo, Password: c05Password}, "127.0.0.1", "c05-test")
	require.NoError(t, err, "恢复启用后新登录可用")
}

// 2) F021 重复操作：同 K 携不同口令均不再次执行、不比较口令；异主体查询拒绝。
func TestC05_ResetPassword_SameOperationReplay(t *testing.T) {
	e := newC05Env(t)
	ctx := trustedIAMTestContext()
	actor := e.c05Superadmin(t, "C059011")
	target := e.c05User(t, "C059012", 1)
	other := e.c05User(t, "C059013", 1)

	res1, err := e.userSvc.ResetPassword(ctx, &model.ResetPasswordRequest{UserID: target.ID, Password: "NewPassA#2026"}, actor.ID, "K-C05-R1")
	require.NoError(t, err)
	require.Equal(t, "completed", res1.Status)
	require.EqualValues(t, 2, res1.Epoch)

	// 同 K 携口令 B：明确核查状态，不再次执行（口令保持 A、代际不推进）
	res2, err := e.userSvc.ResetPassword(ctx, &model.ResetPasswordRequest{UserID: target.ID, Password: "NewPassB#2026"}, actor.ID, "K-C05-R1")
	require.NoError(t, err)
	require.Equal(t, res1.Status, res2.Status)
	require.Equal(t, res1.Epoch, res2.Epoch)
	snap, _ := e.repo.SecuritySnapshot(ctx, target.ID)
	require.True(t, crypto.CheckPassword("NewPassA#2026", snap.PasswordHash), "口令不被 B 覆盖")
	require.True(t, snap.MustChangePassword)

	// 异主体查询：不能拿别人的 K 查结果
	_, err = e.userSvc.GetOperationResult(ctx, "K-C05-R1", other.ID)
	require.ErrorIs(t, err, errcode.ErrCannotManageHigher)
	// 原操作者可查
	view, err := e.userSvc.GetOperationResult(ctx, "K-C05-R1", actor.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", view.Status)
	// 查不到 K：not_found（调用方按待核查处理，不自动重做）
	view2, err := e.userSvc.GetOperationResult(ctx, "K-C05-NOPE", actor.ID)
	require.NoError(t, err)
	require.Equal(t, "not_found", view2.Status)
}

// 3) 同 K 并发首次请求：数据库唯一保留收敛，最多一个请求产生口令/代际效果。
func TestC05_ChangePassword_ConcurrentSameKey_OneEffect(t *testing.T) {
	e := newC05Env(t)
	ctx := trustedIAMTestContext()
	u := e.c05User(t, "C059021", 1)

	const opK = "K-C05-P1"
	var wg sync.WaitGroup
	results := make(chan string, 2)
	for _, pw := range []string{"ConcPassA#2026", "ConcPassB#2026"} {
		wg.Add(1)
		go func(pw string) {
			defer wg.Done()
			pair, res, err := e.authSvc.UpdatePassword(ctx, u.ID, c05Password, pw, "", "dev1", opK)
			switch {
			case err == nil && pair != nil:
				results <- "pair:" + res.OperationID
			case err == nil && pair == nil:
				results <- "replay:" + res.OperationID
			default:
				results <- "err:" + err.Error()
			}
		}(pw)
	}
	wg.Wait()
	close(results)

	snap, err := e.repo.SecuritySnapshot(ctx, u.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, snap.SessionEpoch, "代际恰好推进一次")
	opsRepo := repository.NewSessionOperationRepo(testPool)
	op, err := opsRepo.FindOperationByID(ctx, opK)
	require.NoError(t, err)
	require.Equal(t, "completed", op.Status)
	winners := 0
	for r := range results {
		if len(r) > 5 && r[:5] == "pair:" {
			winners++
		}
	}
	require.Equal(t, 1, winners, "恰好一笔成功签发，其余安全收敛")
}

// 4) 跨进程恢复：进程 A 投影失败留 pending；「另一进程」（新 deps 实例）按原
// 意图收敛——不依赖内存队列、不复发第二次代际推进。
func TestC05_ProjectionRecovery_CrossProcess(t *testing.T) {
	e := newC05Env(t)
	ctx := trustedIAMTestContext()
	actor := e.c05Superadmin(t, "C059031")
	target := e.c05User(t, "C059032", 1)
	service.RunProjectionSweepOnce(ctx, e.deps)

	e.mr.SetError("cross-process fault")
	_, err := e.userSvc.UpdateStatus(ctx, &model.UpdateUserStatusRequest{UserID: target.ID, Status: 0}, actor.ID, "K-C05-X1")
	require.Error(t, err)
	e.mr.SetError("")

	// 新进程：全新 deps 实例（同 DB/Redis，零内存态）单轮收敛
	procB := service.NewSessionWorkerDeps(testPool, e.repo, e.rdb, e.jwt.AccessTTL())
	service.RunProjectionSweepOnce(ctx, procB)
	snap, err := e.repo.SecuritySnapshot(ctx, target.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, snap.SessionEpoch)
	require.EqualValues(t, 2, snap.ProjectionEpoch, "跨进程按原意图确认")
	pending, _ := repository.NewSessionOperationRepo(testPool).PendingIntentCountForUser(ctx, target.ID)
	require.Zero(t, pending)
}

// 5) 单调投影 + 旧代意图被取代：迟到 worker 不能覆盖新代。
func TestC05_Projection_MonotonicAndSupersede(t *testing.T) {
	e := newC05Env(t)
	ctx := trustedIAMTestContext()

	// 单调：投影已到 5 时，安装更旧代际 2 被拒绝且不动 disabled 语义
	require.NoError(t, e.rdb.Set(ctx, "user:session_epoch:db1:424242", 5, 0).Err())
	require.NoError(t, e.rdb.Set(ctx, "user:session_disabled:db1:424242", "1", time.Minute).Err())
	dep := service.NewSessionWorkerDeps(testPool, e.repo, e.rdb, time.Minute)
	service.RunProjectionSweepOnce(ctx, dep) // 缺项发现会为 424242 合成意图？——该用户不存在，仅现库用户
	v, err := e.rdb.Get(ctx, "user:session_epoch:db1:424242").Result()
	require.NoError(t, err)
	require.Equal(t, "5", v, "投影不得回退")
	n, _ := e.rdb.Exists(ctx, "user:session_disabled:db1:424242").Result()
	require.EqualValues(t, 1, n, "旧代安装不得触碰 disabled 语义")

	// 迟到清理不覆盖新代：target 停用（epoch2 pending）→ 先启用确认（epoch3）→
	// 再释放 epoch2 时代的旧故障重试，不得把 disabled 写回
	actor := e.c05Superadmin(t, "C059041")
	target := e.c05User(t, "C059042", 1)
	service.RunProjectionSweepOnce(ctx, e.deps)
	e.mr.SetError("late-worker fault")
	_, err = e.userSvc.UpdateStatus(ctx, &model.UpdateUserStatusRequest{UserID: target.ID, Status: 0}, actor.ID, "K-C05-L1")
	require.Error(t, err)
	e.mr.SetError("")
	res, err := e.userSvc.UpdateStatus(ctx, &model.UpdateUserStatusRequest{UserID: target.ID, Status: 1}, actor.ID, "K-C05-L2")
	require.Error(t, err, "epoch2 撤销未投影时启用仍拒绝")
	_ = res
	service.RunProjectionSweepOnce(ctx, e.deps) // 先确认 epoch2
	_, err = e.userSvc.UpdateStatus(ctx, &model.UpdateUserStatusRequest{UserID: target.ID, Status: 1}, actor.ID, "K-C05-L3")
	require.NoError(t, err)
	service.RunProjectionSweepOnce(ctx, e.deps) // epoch3 确认，disabled 清除
	n, _ = e.rdb.Exists(ctx, "user:session_disabled:db1:"+itoa(target.ID)).Result()
	require.Zero(t, n, "启用投影清除 disabled")
	// 此时 epoch2 旧意图已确认/superseded；再跑 sweep 不复活旧 disabled
	service.RunProjectionSweepOnce(ctx, e.deps)
	n, _ = e.rdb.Exists(ctx, "user:session_disabled:db1:"+itoa(target.ID)).Result()
	require.Zero(t, n, "迟到 worker 不得覆盖新代 active 事实")
}

// 6) 存量初始化可重入：缺项发现补齐 + 重跑不重复（停用账号不被激活）。
func TestC05_Bootstrap_GapDiscovery_Reentrant(t *testing.T) {
	e := newC05Env(t)
	ctx := trustedIAMTestContext()
	disabled := e.c05User(t, "C059051", 1)                                           // 存量直建（repo.Create 恒激活）
	require.NoError(t, e.repo.UpdateStatus(trustedIAMTestContext(), disabled.ID, 0)) // 原始存量停用事实（不推代际）
	opsRepo := repository.NewSessionOperationRepo(testPool)

	before, err := opsRepo.PendingIntentCountForUser(ctx, disabled.ID)
	require.NoError(t, err)
	require.Zero(t, before, "直建账号初始无意图（未切换）")

	service.RunProjectionSweepOnce(ctx, e.deps)
	snap, err := e.repo.SecuritySnapshot(ctx, disabled.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, snap.SessionEpoch)
	require.EqualValues(t, 1, snap.ProjectionEpoch, "缺项发现补齐并确认")
	require.Equal(t, 0, snap.Status, "初始化不得激活停用账号")
	n, _ := e.rdb.Exists(ctx, "user:session_disabled:db1:"+itoa(disabled.ID)).Result()
	require.EqualValues(t, 1, n, "停用事实投影就位")

	// 可重入：再跑两轮不新增意图、不回退
	service.RunProjectionSweepOnce(ctx, e.deps)
	service.RunProjectionSweepOnce(ctx, e.deps)
	after, _ := opsRepo.PendingIntentCountForUser(ctx, disabled.ID)
	require.Zero(t, after)
	snap2, _ := e.repo.SecuritySnapshot(ctx, disabled.ID)
	require.EqualValues(t, 1, snap2.SessionEpoch, "重入不得推进代际")
}

// 7) 迁移 bootstrap 重入：000083 的存量初始化两条 INSERT 幂等——对迁移后
// 新建的账号重放迁移尾部 INSERT，首遍补齐、二遍零新增（停用不被激活）。
func TestC05_MigrationBootstrap_Idempotent(t *testing.T) {
	ctx := context.Background()
	// 迁移时点后的「存量」：直建含停用账号，模拟真实环境存量
	e := newC05Env(t)
	u := e.c05User(t, "C059061", 0)

	migration, err := os.ReadFile("../../migrations/000083_user_session_epoch.up.sql")
	require.NoError(t, err)
	_, bootstrap, found := strings.Cut(string(migration), "-- 存量初始化")
	require.True(t, found)
	_, statements, found := strings.Cut(bootstrap, "INSERT INTO user_session_operations")
	require.True(t, found)
	replay := func() error {
		_, err := testPool.Exec(ctx, "INSERT INTO user_session_operations"+statements)
		return err
	}
	require.NoError(t, replay())
	var ops, intents int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT COUNT(*) FROM user_session_operations WHERE operation_id = 'bootstrap:' || $1`, strconv.FormatInt(u.ID, 10)).Scan(&ops))
	require.NoError(t, testPool.QueryRow(ctx, `SELECT COUNT(*) FROM session_projection_intents WHERE operation_id = 'bootstrap:' || $1`, strconv.FormatInt(u.ID, 10)).Scan(&intents))
	require.Equal(t, 1, ops, "首遍补齐恰好一条回执")
	require.Equal(t, 1, intents, "首遍补齐恰好一条意图")
	require.NoError(t, replay())
	require.NoError(t, testPool.QueryRow(ctx, `SELECT COUNT(*) FROM user_session_operations WHERE operation_id = 'bootstrap:' || $1`, strconv.FormatInt(u.ID, 10)).Scan(&ops))
	require.NoError(t, testPool.QueryRow(ctx, `SELECT COUNT(*) FROM session_projection_intents WHERE operation_id = 'bootstrap:' || $1`, strconv.FormatInt(u.ID, 10)).Scan(&intents))
	require.Equal(t, 1, ops, "重入不得新增回执")
	require.Equal(t, 1, intents, "重入不得新增意图")
}

func TestC05_NewAccountNormallyReadyWhenCreateReturns(t *testing.T) {
	e := newC05Env(t)
	ctx := trustedIAMTestContext()
	actor := e.c05Superadmin(t, "C059076")
	user, err := e.userSvc.Create(ctx, &model.CreateUserRequest{
		Username: "c05-created-then-login", EmployeeNo: "C059077", Password: c05Password,
	}, actor.ID)
	require.NoError(t, err)
	snap, err := e.repo.SecuritySnapshot(ctx, user.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, snap.ProjectionEpoch, "normal projection follows the owning commit before response")
	require.NotNil(t, e.login(t, user.EmployeeNo, "first-device"))
}
