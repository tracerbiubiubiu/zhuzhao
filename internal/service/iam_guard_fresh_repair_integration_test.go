//go:build integration

package service_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

type iamFreshTraceKey struct{}
type iamFreshTrace struct {
	event   chan string
	release chan struct{}
	once    sync.Once
}

func (tr *iamFreshTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "pg_advisory_lock(") {
		tr.once.Do(func() { tr.event <- "waiting_guard" })
	}
	return context.WithValue(ctx, iamFreshTraceKey{}, strings.Contains(data.SQL, "FROM roles WHERE id IN (SELECT id FROM expanded)"))
}
func (tr *iamFreshTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if final, _ := ctx.Value(iamFreshTraceKey{}).(bool); final {
		tr.once.Do(func() { tr.event <- "checked_old_snapshot"; <-tr.release })
	}
}

// The same schedule is used for the old and fixed implementation: another
// cooperating writer owns the guard, the protected command either reaches its
// final target check (old code) or waits for the common guard (fixed code), then
// the revocation/stronger grant commits before the protected write may continue.
func TestIAMGuard_RevocationAndTargetGrantCommitBeforeSensitiveWrite(t *testing.T) {
	for _, change := range []string{"actor_direct_revoked", "target_granted_stronger_org_role"} {
		t.Run(change, func(t *testing.T) {
			f := targetProtectionSetup(t)
			ctx, cancel := context.WithTimeout(trustedIAMTestContext(), 15*time.Second)
			defer cancel()
			_, err := testPool.Exec(ctx, `DELETE FROM org_roles WHERE org_id=$1`, f.org)
			require.NoError(t, err)
			before := targetProtectionSnapshot(t, f.target)
			trace := &iamFreshTrace{event: make(chan string, 1), release: make(chan struct{})}
			cfg := testPool.Config().Copy()
			cfg.ConnConfig.Tracer = trace
			cfg.MaxConns = 2
			pool, err := pgxpool.NewWithConfig(ctx, cfg)
			require.NoError(t, err)
			defer pool.Close()
			mr := miniredis.RunT(t)
			rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
			defer rdb.Close()
			users := repository.NewUserRepo(pool)
			svc := service.NewUserService(pool, users, repository.NewRoleRepo(pool), nil, rdb, jwtpkg.NewManager(jwtpkg.Config{Secret: "iam-fresh-test-0123456789abcdef", AccessTTL: time.Hour}))
			held := make(chan struct{})
			finish := make(chan struct{})
			writerDone := make(chan error, 1)
			go func() {
				writerDone <- repository.WithIAMGuard(ctx, testPool, func(guarded context.Context) error {
					if change == "actor_direct_revoked" {
						_, err = repository.IAMDB(guarded, testPool).Exec(guarded, `DELETE FROM user_roles WHERE user_id=$1`, f.actor)
					} else {
						_, err = repository.IAMDB(guarded, testPool).Exec(guarded, `INSERT INTO org_roles(org_id,role_id) VALUES($1,$2)`, f.org, f.strong.ID)
					}
					if err != nil {
						return err
					}
					close(held)
					select {
					case <-finish:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
			}()
			select {
			case <-held:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			done := make(chan error, 1)
			go func() {
				done <- svc.ResetPassword(ctx, &model.ResetPasswordRequest{UserID: f.target, Password: "SyntheticChangedPassword#2026"}, f.actor)
			}()
			select {
			case event := <-trace.event:
				t.Log("observed", event)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			close(finish)
			require.NoError(t, <-writerDone)
			close(trace.release)
			require.ErrorIs(t, <-done, errcode.ErrCannotResetHigher)
			require.Equal(t, before, targetProtectionSnapshot(t, f.target), "password and account facts must remain unchanged")
			require.Empty(t, mr.Keys(), "denied write must not emit a session revocation")
		})
	}
}
