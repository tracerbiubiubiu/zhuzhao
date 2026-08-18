//go:build integration

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

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

	dupName := &model.User{Username: "reuse_name", EmployeeNo: "E900002", Password: hash}
	require.NoError(t, repo.Create(ctx, dupName))

	dupNo := &model.User{Username: "other", EmployeeNo: "E900001", Password: hash}
	err = repo.Create(ctx, dupNo)
	require.Error(t, err)
	var bizErr *errcode.Error
	require.ErrorAs(t, err, &bizErr)
	assert.Equal(t, errcode.ErrEmployeeNoAlreadyExists.Code, bizErr.Code)
}

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

	u2 := &model.User{
		Username:      "u2",
		EmployeeNo:    "E800002",
		UserDomain:    "CORP",
		DomainAccount: "zhangsan",
		Password:      hash,
	}
	err = repo.Create(ctx, u2)
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
