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

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/crypto"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jwt"
	redispkg "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/redis"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// D2-03 守护：UpdateRole Status 指针化 patch 语义——
// 未传 status（nil）的「只改名」请求不得静默禁用角色（文档典型用法，
// 原零值穿透一次改名即熔断全员权限）；显式传 0 才禁用。
func TestRBACService_UpdateRole_StatusPatchSemantics(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	svc := newRBACServiceForTest(t)

	target := rbacTestRole(t, "patch_target", 20, false)
	saRole := rbacTestRole(t, "superadmin", 1, true)
	saActorID := rbacTestUser(t, "sa_user", "E620001", []int64{saRole.ID})

	// ① 未传 status（nil）→ 保持启用（修复前：零值穿透 → status=0 静默禁用）
	updated, err := svc.UpdateRole(ctx, &model.UpdateRoleRequest{
		ID: target.ID, Version: 1, Name: "patched", Priority: 20,
	}, saActorID)
	require.NoError(t, err)
	assert.Equal(t, 1, updated.Status, "未传 status 必须保持现值（1），不得零值穿透禁用")

	// ② 显式传 0 → 禁用（patch 语义不阻碍显式操作）
	zero := 0
	updated, err = svc.UpdateRole(ctx, &model.UpdateRoleRequest{
		ID: target.ID, Version: 2, Name: "patched", Priority: 20, Status: &zero,
	}, saActorID)
	require.NoError(t, err)
	assert.Equal(t, 0, updated.Status, "显式传 0 应禁用")

	// ③ 显式传 1 → 恢复启用
	one := 1
	updated, err = svc.UpdateRole(ctx, &model.UpdateRoleRequest{
		ID: target.ID, Version: 3, Name: "patched", Priority: 20, Status: &one,
	}, saActorID)
	require.NoError(t, err)
	assert.Equal(t, 1, updated.Status, "显式传 1 应恢复启用")
}

// D2-03 守护：组织 Update Status patch 语义（与角色同型）。
func TestOrgService_Update_StatusPatchSemantics(t *testing.T) {
	resetOrgTables(t)
	ctx := context.Background()
	svc := service.NewOrgService(repository.NewOrgRepo(testPool), repository.NewUserRepo(testPool), service.NewOrgDelegationService(testPool), newRBACServiceForTest(t))

	var orgID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, path, is_virtual, status, is_system, sort_order)
		VALUES ('d2_patch', '补丁语义', 'd2_patch', false, 1, false, 1) RETURNING id`).Scan(&orgID))

	// ① 未传 status → 保持启用
	updated, err := svc.Update(ctx, &model.UpdateOrgRequest{
		ID: orgID, Version: 1, Name: "改名",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, updated.Status, "未传 status 必须保持现值（1）")

	// ② 显式传 0 → 禁用
	zero := 0
	updated, err = svc.Update(ctx, &model.UpdateOrgRequest{
		ID: orgID, Version: 2, Name: "改名", Status: &zero,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, updated.Status, "显式传 0 应禁用")
}

// ---- D2-45：auth 残余场景（RT 篡改/跨设备/disabled/并发刷新/锁定分支）----

// d2LoginUser 建用户 + 返回暴露 miniredis 实例的 AuthService（可注入 disabled 等键）
func d2LoginUser(t *testing.T, employeeNo, password string) (*service.AuthService, *model.User, *miniredis.Miniredis) {
	t.Helper()
	resetAuthTables(t)
	ctx := context.Background()
	repo := repository.NewUserRepo(testPool)
	hash, err := crypto.HashPassword(password)
	require.NoError(t, err)
	user := &model.User{Username: "d2_" + employeeNo, EmployeeNo: employeeNo, Password: hash, Status: 1}
	require.NoError(t, repo.Create(ctx, user))

	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	jwtCfg := config.JWTConfig{Secret: "d2-test-secret-0123456789abcdef", AccessTTL: 30 * time.Minute, RefreshTTL: 168 * time.Hour}
	auditSvc := service.NewAuditService(repository.NewAuditLogRepo(testPool), repo)
	authSvc := service.NewAuthService(repo, jwt.NewManager(jwtCfg), rdb, redispkg.NewScripts(rdb), auditSvc, jwtCfg)
	return authSvc, user, mr
}

// D2-45：篡改 RT（改签名段字符）→ 解析失败拒绝刷新
func TestAuthService_RTTampered(t *testing.T) {
	authSvc, user, _ := d2LoginUser(t, "E630001", "passw0rd123")
	ctx := context.Background()

	pair, err := authSvc.Login(ctx, &model.LoginRequest{EmployeeNo: user.EmployeeNo, Password: "passw0rd123", DeviceID: "dev-1"}, "127.0.0.1", "t")
	require.NoError(t, err)

	rt := []byte(pair.RefreshToken)
	rt[len(rt)-2] ^= 0x01 // 破坏签名段末字符
	_, err = authSvc.Refresh(ctx, string(rt))
	requireErrCode(t, err, errcode.ErrRefreshTokenInvalid)
}

// D2-45：多设备 RT 隔离——登出 dev-1 后 dev-2 的 RT 仍可刷新
func TestAuthService_RTCrossDeviceIsolation(t *testing.T) {
	authSvc, user, _ := d2LoginUser(t, "E630002", "passw0rd123")
	ctx := context.Background()

	p1, err := authSvc.Login(ctx, &model.LoginRequest{EmployeeNo: user.EmployeeNo, Password: "passw0rd123", DeviceID: "dev-1"}, "127.0.0.1", "t")
	require.NoError(t, err)
	p2, err := authSvc.Login(ctx, &model.LoginRequest{EmployeeNo: user.EmployeeNo, Password: "passw0rd123", DeviceID: "dev-2"}, "127.0.0.1", "t")
	require.NoError(t, err)

	// 登出 dev-1
	require.NoError(t, authSvc.Logout(ctx, p1.AccessToken, "dev-1"))

	// dev-1 RT 已死
	_, err = authSvc.Refresh(ctx, p1.RefreshToken)
	requireErrCode(t, err, errcode.ErrRefreshTokenInvalid)
	// dev-2 RT 不受影响
	_, err = authSvc.Refresh(ctx, p2.RefreshToken)
	require.NoError(t, err, "登出 dev-1 不应影响 dev-2 的会话")
}

// D2-45：disabled 用户 RT——禁用标记存在时拒绝刷新（不查 DB 即拦截）
func TestAuthService_DisabledUserRT(t *testing.T) {
	authSvc, user, mr := d2LoginUser(t, "E630003", "passw0rd123")
	ctx := context.Background()

	pair, err := authSvc.Login(ctx, &model.LoginRequest{EmployeeNo: user.EmployeeNo, Password: "passw0rd123"}, "127.0.0.1", "t")
	require.NoError(t, err)

	// 模拟禁用路径写入的 disabled 标记（revokeUserSessions 语义见 session_revoke 单测）
	mr.Set(fmt.Sprintf("user:disabled:%d", user.ID), "1")

	_, err = authSvc.Refresh(ctx, pair.RefreshToken)
	requireErrCode(t, err, errcode.ErrRefreshTokenInvalid)
}

// D2-45：并发刷新同一 RT——GetDel 原子性保证恰好一个成功
func TestAuthService_ConcurrentRefresh(t *testing.T) {
	authSvc, user, _ := d2LoginUser(t, "E630004", "passw0rd123")
	ctx := context.Background()

	pair, err := authSvc.Login(ctx, &model.LoginRequest{EmployeeNo: user.EmployeeNo, Password: "passw0rd123", DeviceID: "dev-1"}, "127.0.0.1", "t")
	require.NoError(t, err)

	const n = 8
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() { _, err := authSvc.Refresh(ctx, pair.RefreshToken); results <- err }()
	}
	success := 0
	for i := 0; i < n; i++ {
		if err := <-results; err == nil {
			success++
		} else {
			requireErrCode(t, err, errcode.ErrRefreshTokenInvalid)
		}
	}
	assert.Equal(t, 1, success, "同一 RT 并发刷新应恰好一次成功（GetDel 原子）")
}

// D2-45：登录锁定集成分支——连续失败达阈值后，正确密码也返回 429
func TestAuthService_LoginLockoutBranch(t *testing.T) {
	authSvc, user, _ := d2LoginUser(t, "E630005", "passw0rd123")
	ctx := context.Background()

	for i := 0; i < 6; i++ {
		_, err := authSvc.Login(ctx, &model.LoginRequest{EmployeeNo: user.EmployeeNo, Password: "wrong-pass"}, "127.0.0.1", "t")
		require.Error(t, err)
	}

	// 第 7 次即使密码正确 → 锁定（阈值在密码比对前拦截）
	_, err := authSvc.Login(ctx, &model.LoginRequest{EmployeeNo: user.EmployeeNo, Password: "passw0rd123"}, "127.0.0.1", "t")
	requireErrCode(t, err, errcode.ErrAccountLocked)
}

// ---- D2-45：service 层直测残余（Create/SetRoles/ResetPassword/CreateRole 越权）----

func newUserServiceForD2(t *testing.T) (*service.UserService, *repository.UserRepo, *repository.RoleRepo) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	userRepo := repository.NewUserRepo(testPool)
	roleRepo := repository.NewRoleRepo(testPool)
	orgSvc := service.NewOrgService(repository.NewOrgRepo(testPool), userRepo, service.NewOrgDelegationService(testPool), newRBACServiceForTest(t))
	jwtCfg := config.JWTConfig{Secret: "d2-test-secret-0123456789abcdef", AccessTTL: 30 * time.Minute, RefreshTTL: 168 * time.Hour}
	return service.NewUserService(testPool, userRepo, roleRepo, orgSvc, rdb, jwt.NewManager(jwtCfg)), userRepo, roleRepo
}

// D2-45：Create 重复工号 → 30007（含软删占用语义由 000006 部分索引保证）
func TestUserService_CreateDuplicateEmployeeNo(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	svc, _, _ := newUserServiceForD2(t)

	actor := rbacTestUser(t, "d2_actor", "E631001", nil)
	_, err := svc.Create(ctx, &model.CreateUserRequest{
		Username: "dup1", Password: "passw0rd123", EmployeeNo: "E631002",
	}, actor)
	require.NoError(t, err)

	_, err = svc.Create(ctx, &model.CreateUserRequest{
		Username: "dup2", Password: "passw0rd123", EmployeeNo: "E631002",
	}, actor)
	requireErrCode(t, err, errcode.ErrEmployeeNoAlreadyExists)
}

// D2-45：SetRoles 分配更高角色 → 30009（低权 actor 不可绑定 superadmin）
func TestUserService_SetRolesHigherRejected(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	svc, _, _ := newUserServiceForD2(t)

	weakRole := rbacTestRole(t, "d2_weak", 25, false)
	saRole := rbacTestRole(t, "superadmin", 1, true)
	actor := rbacTestUser(t, "d2_actor", "E631003", []int64{weakRole.ID})
	target := rbacTestUser(t, "d2_target", "E631004", nil)

	err := svc.SetRoles(ctx, &model.SetUserRolesRequest{UserID: target, RoleIDs: []int64{saRole.ID}}, actor)
	requireErrCode(t, err, errcode.ErrCannotAssignHigherRole)
}

// D2-45：ResetPassword 对同级/更高目标 → 30005（专用文案码）
func TestUserService_ResetPasswordHigherRejected(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	svc, _, _ := newUserServiceForD2(t)

	weakRole := rbacTestRole(t, "d2_weak2", 25, false)
	saRole := rbacTestRole(t, "superadmin", 1, true)
	actor := rbacTestUser(t, "d2_actor", "E631005", []int64{weakRole.ID})
	target := rbacTestUser(t, "d2_target", "E631006", []int64{saRole.ID})

	err := svc.ResetPassword(ctx, &model.ResetPasswordRequest{UserID: target, Password: "newpassw0rd"}, actor)
	requireErrCode(t, err, errcode.ErrCannotResetHigher)
}

// D2-45：CreateRole priority 越权 → 30009（actor 档位之上不可设）
func TestRBACService_CreateRolePriorityEscalation(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	svc := newRBACServiceForTest(t)

	adminRole := rbacTestRole(t, "admin", 10, true)
	actor := rbacTestUser(t, "d2_actor", "E631007", []int64{adminRole.ID})

	// admin(10) 试图创建 priority=1（superadmin 档位）角色 → 拒绝
	_, err := svc.CreateRole(ctx, &model.CreateRoleRequest{
		Code: "d2_escalate", Name: "越权档位", Priority: 1,
	}, actor)
	requireErrCode(t, err, errcode.ErrCannotAssignHigherRole)
}
