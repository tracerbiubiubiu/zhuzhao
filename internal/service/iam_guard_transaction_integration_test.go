//go:build integration

package service_test

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	casbinpkg "github.com/tracerbiubiubiu/zhuzhao/internal/casbin"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestIAMGuard_LockSessionLossCannotCommitProtectedWrite(t *testing.T) {
	resetRBACTables(t)
	user := rbacTestUser(t, "iam_conn_loss", "IAM_CONN_LOSS", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ready, release := make(chan int32, 1), make(chan struct{})
	done := make(chan error, 1)
	projected := false
	go func() {
		done <- repository.WithIAMGuard(ctx, testPool, func(guarded context.Context) error {
			var pid int32
			if err := repository.IAMDB(guarded, testPool).QueryRow(guarded, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			if _, err := repository.IAMDB(guarded, testPool).Exec(guarded, `UPDATE users SET real_name='must rollback' WHERE id=$1`, user); err != nil {
				return err
			}
			if err := repository.AfterIAMCommit(guarded, func() error { projected = true; return nil }); err != nil {
				return err
			}
			ready <- pid
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	var pid int32
	select {
	case pid = <-ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var killed bool
	require.NoError(t, testPool.QueryRow(ctx, `SELECT pg_terminate_backend($1)`, pid).Scan(&killed))
	require.True(t, killed)
	close(release)
	require.Error(t, <-done, "a separate business connection would still commit after lock-session loss")
	current, err := repository.NewUserRepo(testPool).FindByID(ctx, user)
	require.NoError(t, err)
	require.Empty(t, current.RealName)
	require.False(t, projected)
	require.NoError(t, repository.WithIAMGuard(ctx, testPool, func(context.Context) error { return nil }), "dead session must not strand the local gate")
}

func TestIAMGuard_LocalWaitersDoNotOccupyPoolSlots(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	held, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- repository.WithIAMGuard(ctx, testPool, func(context.Context) error { close(held); <-release; return nil })
	}()
	select {
	case <-held:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	acquired := testPool.Stat().AcquiredConns()
	waiterCtx, waiterCancel := context.WithTimeout(ctx, 80*time.Millisecond)
	defer waiterCancel()
	called := false
	err := repository.WithIAMGuard(waiterCtx, testPool, func(context.Context) error { called = true; return nil })
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, called)
	require.Equal(t, acquired, testPool.Stat().AcquiredConns(), "local wait must happen before pool acquire")
	close(release)
	require.NoError(t, <-done)
	require.NoError(t, repository.WithIAMGuard(ctx, testPool, func(context.Context) error { return nil }))
}

func TestIAMGuard_PostCommitCanAcquireSingleConnectionPool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := testPool.Config().Copy()
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	called := false
	err = repository.WithIAMGuard(ctx, pool, func(guarded context.Context) error {
		return repository.AfterIAMCommit(guarded, func() error {
			called = true
			readCtx, readCancel := context.WithTimeout(ctx, 150*time.Millisecond)
			defer readCancel()
			var one int
			return pool.QueryRow(readCtx, `SELECT 1`).Scan(&one)
		})
	})
	require.NoError(t, err, "post-commit adapter reload needs the only pool slot; local serial can remain held")
	require.True(t, called)
}

func TestIAMGuard_RealAdapterReloadWithSingleConnectionPool(t *testing.T) {
	resetRBACTables(t)
	ctx, cancel := context.WithTimeout(trustedIAMTestContext(), 10*time.Second)
	defer cancel()
	super := rbacTestRole(t, "superadmin", 1, true)
	actor := rbacTestUser(t, "iam_pool1_operator", "IAM_POOL1_OPERATOR", []int64{super.ID})
	role := rbacTestRole(t, "iam_pool1_target", 80, false)
	var menuID int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO menus(code,name,menu_type,permission) VALUES('iam_pool1_menu','Synthetic',3,'synthetic:write') RETURNING id`).Scan(&menuID))
	_, err := testPool.Exec(ctx, `INSERT INTO menu_apis(menu_id,api_path,api_method) VALUES($1,'/api/v1/synthetic/write','POST')`, menuID)
	require.NoError(t, err)
	cfg := testPool.Config().Copy()
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	enforcer, _, cleanup, err := casbinpkg.New(config.CasbinConfig{Model: "../../configs/casbin_model.conf"}, pool)
	require.NoError(t, err)
	defer cleanup()
	svc := service.NewRBACService(repository.NewRoleRepo(pool), repository.NewUserRepo(pool), repository.NewMenuRepo(pool), enforcer)
	require.NoError(t, svc.AssignMenus(ctx, &model.AssignMenusRequest{RoleID: role.ID, MenuIDs: []int64{menuID}}, actor))
	allowed, err := enforcer.Enforce("role::iam_pool1_target", "/api/v1/synthetic/write", "POST")
	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, svc.DeleteRole(ctx, role.ID, actor))
	allowed, err = enforcer.Enforce("role::iam_pool1_target", "/api/v1/synthetic/write", "POST")
	require.NoError(t, err)
	require.False(t, allowed)
}
