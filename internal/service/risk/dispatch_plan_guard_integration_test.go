//go:build integration

package risk_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskdispatch"
)

// A preview has its own current L1 policy, even if the middleware still admits
// the request using an earlier policy and credential snapshot.
func TestDispatchPlanCommandRevalidatesCurrentPolicyAndSession(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	const path = "/api/v1/risk-items/dispatch-plan"
	for _, variant := range []string{"happy", "policy_revoked_db", "session_revoked"} {
		t.Run(variant, func(t *testing.T) {
			roleCode, actor := seedCasbinRole(t, path)
			var epoch int64
			require.NoError(t, testPool.QueryRow(ctx, `SELECT session_epoch FROM users WHERE id=$1`, actor).Scan(&epoch))
			switch variant {
			case "policy_revoked_db":
				_, err := testPool.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1=$2 AND v2='POST'`, "role::"+roleCode, path)
				require.NoError(t, err)
			case "session_revoked":
				_, err := testPool.Exec(ctx, `UPDATE users SET session_epoch=session_epoch+1 WHERE id=$1`, actor)
				require.NoError(t, err)
			}

			m, err := casbinmodel.NewModelFromFile("../../../configs/casbin_model.conf")
			require.NoError(t, err)
			enforcer, err := casbin.NewSyncedEnforcer(m)
			require.NoError(t, err)
			_, err = enforcer.AddPolicy("role::"+roleCode, path, "POST")
			require.NoError(t, err)
			rbac := service.NewRBACService(repository.NewRoleRepo(testPool), repository.NewUserRepo(testPool), repository.NewMenuRepo(testPool), enforcer)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("userID", actor)
				c.Set("session_epoch", epoch)
				c.Next()
			})
			router.Use(middleware.CasbinAuth(enforcer, rbac, nil))
			called := false
			var commandErr error
			router.POST(path, func(c *gin.Context) {
				commandErr = newService().WithDispatchTransaction(c.Request.Context(), actor, func(ctx context.Context, tx pgx.Tx) error {
					called = true
					_, err := riskdispatch.SavePlanTx(ctx, tx, actor, riskdispatch.Target{}, riskdispatch.Plan{}, time.Now())
					return err
				})
				if commandErr != nil {
					c.Status(http.StatusForbidden)
					return
				}
				c.Status(http.StatusOK)
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
			var saved int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM risk_dispatch_plans WHERE actor_id=$1`, actor).Scan(&saved))
			if variant == "happy" {
				require.NoError(t, commandErr)
				require.Equal(t, http.StatusOK, recorder.Code)
				require.True(t, called)
				require.Equal(t, 1, saved)
			} else {
				require.ErrorIs(t, commandErr, errcode.ErrNoPermission)
				require.Equal(t, http.StatusForbidden, recorder.Code)
				require.False(t, called)
				require.Zero(t, saved)
			}
		})
	}
}

// Handling a failed nested dispatch must leave the outer transaction usable,
// without the failed preview or its after-commit work escaping the child.
func TestDispatchTransactionRollsBackFailedChildWhenOuterContinues(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	stop := errors.New("synthetic dispatch failure after preview save")
	for _, variant := range []string{"business_error", "sql_error"} {
		t.Run(variant, func(t *testing.T) {
			var planID uuid.UUID
			projected := false
			err := repository.WithIAMCommandGuard(ctx, testPool, func(parentCtx context.Context, parent pgx.Tx) error {
				err := newService().WithDispatchTransaction(parentCtx, userM, func(childCtx context.Context, child pgx.Tx) error {
					bound, err := riskdispatch.SavePlanTx(childCtx, child, userM, riskdispatch.Target{}, riskdispatch.Plan{}, time.Now())
					if err != nil {
						return err
					}
					planID = bound.ID
					require.Same(t, child, repository.IAMDB(childCtx, testPool))
					require.Same(t, parent.Conn(), child.Conn())
					require.NoError(t, repository.AfterIAMCommit(childCtx, func() error { projected = true; return nil }))
					if variant == "sql_error" {
						_, err := child.Exec(childCtx, `SELECT 1/0`)
						return err
					}
					return stop
				})
				if variant == "business_error" {
					require.ErrorIs(t, err, stop)
				} else {
					require.ErrorContains(t, err, "division by zero")
				}
				var rows int
				require.NoError(t, parent.QueryRow(parentCtx, `SELECT count(*) FROM risk_dispatch_plans WHERE id=$1`, planID).Scan(&rows))
				require.Zero(t, rows)
				return nil
			})
			require.NoError(t, err)
			require.NotEqual(t, uuid.Nil, planID)
			require.False(t, projected)
			var rows int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM risk_dispatch_plans WHERE id=$1`, planID).Scan(&rows))
			require.Zero(t, rows)
		})
	}
}
