//go:build integration

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/crypto"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestUserRepo_CreateAndFind(t *testing.T) {
	resetUsers(t)
	repo := repository.NewUserRepo(testPool)
	ctx := context.Background()

	hash, err := crypto.HashPassword("secret123")
	require.NoError(t, err)

	user := &model.User{
		Username:   "zhangsan",
		EmployeeNo: "E20240086",
		Password:   hash,
		RealName:   "张三",
	}
	require.NoError(t, repo.Create(ctx, user))
	assert.NotZero(t, user.ID)
	assert.Equal(t, 1, user.Version)

	got, err := repo.FindByEmployeeNo(ctx, "E20240086")
	require.NoError(t, err)
	assert.Equal(t, user.ID, got.ID)
	assert.Equal(t, "zhangsan", got.Username)

	_, err = repo.FindByEmployeeNo(ctx, "NO_SUCH")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errcode.ErrUserNotFound))
}

func TestUserRepo_ListFilters(t *testing.T) {
	resetUsers(t)
	repo := repository.NewUserRepo(testPool)
	ctx := context.Background()

	hash, err := crypto.HashPassword("p")
	require.NoError(t, err)

	for i, name := range []string{"zhang_a", "zhang_b", "lisi"} {
		require.NoError(t, repo.Create(ctx, &model.User{
			Username:   name,
			EmployeeNo: fmt.Sprintf("E10000%d", i),
			Password:   hash,
		}))
	}

	users, total, err := repo.List(ctx, repository.UserListQuery{Username: "zhang", Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, users, 2)

	users, total, err = repo.List(ctx, repository.UserListQuery{EmployeeNo: "E100001", Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, users, 1)
}

// F-6 修复后语义：部分唯一索引过滤软删行，软删用户的 username / employee_no 均可复用；
// 活跃用户之间唯一性仍由索引保证
func TestUserRepo_SoftDeleteRules(t *testing.T) {
	resetUsers(t)
	repo := repository.NewUserRepo(testPool)
	ctx := context.Background()

	hash, err := crypto.HashPassword("p")
	require.NoError(t, err)

	user := &model.User{
		Username:   "reuse_name",
		EmployeeNo: "E900001",
		Password:   hash,
	}
	require.NoError(t, repo.Create(ctx, user))
	require.NoError(t, repo.SoftDelete(ctx, user.ID))

	_, err = repo.FindByID(ctx, user.ID)
	require.ErrorIs(t, err, errcode.ErrUserNotFound)

	// 软删后 username 可复用
	dupName := &model.User{Username: "reuse_name", EmployeeNo: "E900002", Password: hash}
	require.NoError(t, repo.Create(ctx, dupName))

	// 软删后 employee_no 可复用（F-6：索引含 deleted_at IS NULL）
	dupNo := &model.User{Username: "other", EmployeeNo: "E900001", Password: hash}
	require.NoError(t, repo.Create(ctx, dupNo))

	// 活跃用户之间工号冲突仍被拦截
	dupActive := &model.User{Username: "another", EmployeeNo: "E900001", Password: hash}
	err = repo.Create(ctx, dupActive)
	require.Error(t, err)
	var bizErr *errcode.Error
	require.ErrorAs(t, err, &bizErr)
	assert.Equal(t, errcode.ErrEmployeeNoAlreadyExists.Code, bizErr.Code)
}

// F-6 修复后语义：软删用户的域账号可复用；活跃用户之间同域域账号仍唯一
func TestUserRepo_DomainAccountUnique(t *testing.T) {
	resetUsers(t)
	repo := repository.NewUserRepo(testPool)
	ctx := context.Background()

	hash, err := crypto.HashPassword("p")
	require.NoError(t, err)

	u1 := &model.User{
		Username:      "u1",
		EmployeeNo:    "E800001",
		UserDomain:    "CORP",
		DomainAccount: "zhangsan",
		Password:      hash,
	}
	require.NoError(t, repo.Create(ctx, u1))
	require.NoError(t, repo.SoftDelete(ctx, u1.ID))

	// 软删后同域域账号可复用（F-6）
	u2 := &model.User{
		Username:      "u2",
		EmployeeNo:    "E800002",
		UserDomain:    "CORP",
		DomainAccount: "zhangsan",
		Password:      hash,
	}
	require.NoError(t, repo.Create(ctx, u2))

	// 活跃用户之间同域域账号冲突仍被拦截
	u3 := &model.User{
		Username:      "u3",
		EmployeeNo:    "E800003",
		UserDomain:    "CORP",
		DomainAccount: "zhangsan",
		Password:      hash,
	}
	err = repo.Create(ctx, u3)
	require.Error(t, err)
	var bizErr *errcode.Error
	require.ErrorAs(t, err, &bizErr)
	assert.Equal(t, errcode.ErrDomainAccountAlreadyExists.Code, bizErr.Code)
}

func TestUserRepo_SetRolesAndGetRoles(t *testing.T) {
	resetUsers(t)
	repo := repository.NewUserRepo(testPool)
	ctx := context.Background()

	var roleID int64
	err := testPool.QueryRow(ctx, `
		INSERT INTO roles (code, name, priority, is_system)
		VALUES ('viewer', '访客', 30, false) RETURNING id`).Scan(&roleID)
	require.NoError(t, err)

	hash, err := crypto.HashPassword("p")
	require.NoError(t, err)
	user := &model.User{Username: "role_user", EmployeeNo: "E700001", Password: hash}
	require.NoError(t, repo.Create(ctx, user))
	require.NoError(t, repo.SetRoles(ctx, user.ID, []int64{roleID}))

	codes, err := repo.GetRoleCodes(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"viewer"}, codes)

	roles, err := repo.GetRoles(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, roles, 1)
	assert.Equal(t, "viewer", roles[0].Code)
}

// SoftDelete 级联：事务内清理 user_roles / user_orgs；
// 软删 0 行（目标已软删）时报 ErrUserNotFound 且整体回滚，先行的关联 DELETE 不生效
func TestUserRepo_SoftDeleteCascades(t *testing.T) {
	resetUsers(t)
	repo := repository.NewUserRepo(testPool)
	ctx := context.Background()

	var roleID int64
	err := testPool.QueryRow(ctx, `
		INSERT INTO roles (code, name, priority, is_system)
		VALUES ('cascade_role', '级联测试角色', 30, false) RETURNING id`).Scan(&roleID)
	require.NoError(t, err)

	var orgID int64
	err = testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, path, org_type)
		VALUES ('cascade_org', '级联测试组织', 'cascade_org', 2) RETURNING id`).Scan(&orgID)
	require.NoError(t, err)

	hash, err := crypto.HashPassword("p")
	require.NoError(t, err)
	user := &model.User{Username: "cascade_user", EmployeeNo: "E600001", Password: hash}
	require.NoError(t, repo.Create(ctx, user))
	require.NoError(t, repo.SetRoles(ctx, user.ID, []int64{roleID}))
	_, err = testPool.Exec(ctx, `
		INSERT INTO user_orgs (user_id, org_id, is_primary) VALUES ($1, $2, TRUE)`,
		user.ID, orgID)
	require.NoError(t, err)

	// 正常路径：软删除后角色/组织关联均被清理
	require.NoError(t, repo.SoftDelete(ctx, user.ID))
	var n int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_roles WHERE user_id = $1`, user.ID).Scan(&n))
	assert.Zero(t, n, "软删除应清理 user_roles 关联")
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_orgs WHERE user_id = $1`, user.ID).Scan(&n))
	assert.Zero(t, n, "软删除应清理 user_orgs 关联")

	// 回滚路径：目标已软删但关联行残留（不一致状态）→ 软删 0 行报错，
	// 事务回滚使先行的关联 DELETE 不生效
	require.NoError(t, repo.SetRoles(ctx, user.ID, []int64{roleID}))
	err = repo.SoftDelete(ctx, user.ID)
	require.ErrorIs(t, err, errcode.ErrUserNotFound)
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_roles WHERE user_id = $1`, user.ID).Scan(&n))
	assert.Equal(t, 1, n, "软删 0 行时事务应整体回滚，关联行保留")
}

// 超管守护：RunInTx + AcquireSuperadminGuard + CountActiveSuperadminUsersTx 的事务模式
// （UserService.Delete / UpdateStatus / SetRoles 复用同一组原语）。
// 验证最后一名 superadmin 不可被软删/禁用，计数排除已禁用用户
func TestUserRepo_SuperadminGuard(t *testing.T) {
	resetUsers(t)
	repo := repository.NewUserRepo(testPool)
	ctx := context.Background()

	// is_system=false 便于 resetUsers 清理
	var superRoleID int64
	err := testPool.QueryRow(ctx, `
		INSERT INTO roles (code, name, priority, is_system)
		VALUES ('superadmin', '超级管理员', 1, false) RETURNING id`).Scan(&superRoleID)
	require.NoError(t, err)

	hash, err := crypto.HashPassword("p")
	require.NoError(t, err)
	u1 := &model.User{Username: "super1", EmployeeNo: "E500001", Password: hash}
	u2 := &model.User{Username: "super2", EmployeeNo: "E500002", Password: hash}
	require.NoError(t, repo.Create(ctx, u1))
	require.NoError(t, repo.Create(ctx, u2))
	require.NoError(t, repo.SetRoles(ctx, u1.ID, []int64{superRoleID}))
	require.NoError(t, repo.SetRoles(ctx, u2.ID, []int64{superRoleID}))

	n, err := repo.CountActiveSuperadminUsers(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	// 模拟 UserService.Delete 的事务模式：guard → count → 写
	deleteSuperadmin := func(userID int64) error {
		return repo.RunInTx(ctx, func(tx pgx.Tx) error {
			if err := repository.AcquireSuperadminGuard(ctx, tx); err != nil {
				return err
			}
			n, err := repo.CountActiveSuperadminUsersTx(ctx, tx)
			if err != nil {
				return err
			}
			if n <= 1 {
				return errcode.ErrCannotRemoveLastSuperadmin
			}
			return repo.SoftDeleteTx(ctx, tx, userID)
		})
	}

	// 两名超管：删第一名成功
	require.NoError(t, deleteSuperadmin(u1.ID))
	n, err = repo.CountActiveSuperadminUsers(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	// 仅剩最后一名：拒绝软删，用户仍在且启用
	err = deleteSuperadmin(u2.ID)
	require.ErrorIs(t, err, errcode.ErrCannotRemoveLastSuperadmin)
	got, err := repo.FindByID(ctx, u2.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Status)

	// 仅剩最后一名：禁用同样被守护（UpdateStatus 路径）
	err = repo.RunInTx(ctx, func(tx pgx.Tx) error {
		if err := repository.AcquireSuperadminGuard(ctx, tx); err != nil {
			return err
		}
		n, err := repo.CountActiveSuperadminUsersTx(ctx, tx)
		if err != nil {
			return err
		}
		if n <= 1 {
			return errcode.ErrCannotRemoveLastSuperadmin
		}
		return repo.UpdateStatusTx(ctx, tx, u2.ID, 0)
	})
	require.ErrorIs(t, err, errcode.ErrCannotRemoveLastSuperadmin)

	// 计数排除已禁用的超管
	require.NoError(t, repo.UpdateStatus(ctx, u2.ID, 0))
	n, err = repo.CountActiveSuperadminUsers(ctx)
	require.NoError(t, err)
	assert.Zero(t, n)
}

// TOCTOU 修复机制验证：advisory lock 使持锁事务与竞争事务串行化。
// tx1 持锁未提交时，tx2 获取同把锁被阻塞直至超时；tx1 提交后 tx2 立即获得
func TestUserRepo_SuperadminGuardLockSerializes(t *testing.T) {
	resetUsers(t)
	ctx := context.Background()

	tx1, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx1.Rollback(ctx)
	require.NoError(t, repository.AcquireSuperadminGuard(ctx, tx1))

	tx2, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx2.Rollback(ctx)

	// tx1 未提交持锁：tx2 短超时获取被阻塞
	//（超时取消查询后 tx2 连接不可复用，后续用新事务验证）
	lockCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	err = repository.AcquireSuperadminGuard(lockCtx, tx2)
	require.Error(t, err, "持锁事务未释放时，竞争事务应阻塞至超时")

	// tx1 提交释放锁后：新事务可立即获得
	require.NoError(t, tx1.Commit(ctx))
	tx3, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx3.Rollback(ctx)
	require.NoError(t, repository.AcquireSuperadminGuard(ctx, tx3))
}
