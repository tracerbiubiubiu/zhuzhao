//go:build integration

package ticket

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// Middleware deliberately retains its admitted role/policy snapshot. The owning
// port must refresh it through the current C04 transaction, without another pool.
func TestRiskWorkflowAuthorizationCurrentFacts(t *testing.T) {
	for _, change := range []string{"allow", "role", "policy", "epoch", "ticket_read_policy", "ticket_hidden"} {
		t.Run(change, func(t *testing.T) {
			svc, actor, org := setupTicketAdmin(t)
			ticket := newTicketHelper(t, svc, actor, org(), "synthetic workflow read")
			code := "workflow_" + uniqueSuffix()
			path := "/api/v1/risk-items/operations/" + code
			ticketPath := "/api/v1/tickets/" + strconv.FormatInt(ticket.ID, 10)
			var role, epoch int64
			require.NoError(t, testPool.QueryRow(context.Background(), `INSERT INTO roles(code,name,priority) VALUES($1,$1,50) RETURNING id`, code).Scan(&role))
			_, err := testPool.Exec(context.Background(), `INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)`, actor, role)
			require.NoError(t, err)
			for _, p := range []string{path, ticketPath} {
				_, err = testPool.Exec(context.Background(), `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,'GET')`, "role::"+code, p)
				require.NoError(t, err)
			}
			require.NoError(t, testPool.QueryRow(context.Background(), `SELECT session_epoch FROM users WHERE id=$1`, actor).Scan(&epoch))
			m, err := casbinmodel.NewModelFromFile("../../../configs/casbin_model.conf")
			require.NoError(t, err)
			enforcer, err := casbin.NewSyncedEnforcer(m)
			require.NoError(t, err)
			_, err = enforcer.AddPolicy("role::"+code, path, "GET")
			require.NoError(t, err)
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("userID", actor); c.Set("session_epoch", epoch); c.Next() })
			router.Use(middleware.CasbinAuth(enforcer, stubRoleFetcher{actor: {code}}, nil))
			var result error
			callback := false
			router.GET(path, func(c *gin.Context) {
				result = repository.WithIAMGuard(c.Request.Context(), testPool, func(ctx context.Context) error {
					q := repository.IAMDB(ctx, testPool)
					switch change {
					case "role":
						_, err = q.Exec(ctx, `UPDATE roles SET status=0 WHERE id=$1`, role)
					case "policy":
						_, err = q.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1=$2`, "role::"+code, path)
					case "epoch":
						_, err = q.Exec(ctx, `UPDATE users SET session_epoch=session_epoch+1 WHERE id=$1`, actor)
					case "ticket_read_policy":
						_, err = q.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1=$2`, "role::"+code, ticketPath)
					case "ticket_hidden":
						_, err = q.Exec(ctx, `UPDATE tickets SET created_by=(SELECT min(id) FROM users WHERE id<>$2),assigned_to=NULL WHERE id=$1`, ticket.ID, actor)
					}
					return err
				})
				if result != nil {
					return
				}
				result = repository.WithIAMCommandGuard(c.Request.Context(), testPool, func(ctx context.Context, tx pgx.Tx) error {
					return svc.WithRiskWorkflowAuthorizationTx(ctx, tx, actor, []RiskWorkflowRoute{{Path: path, Method: "GET"}}, func(current context.Context) error {
						callback = true
						roles, e := svc.getRoles(current, actor)
						if e != nil {
							return e
						}
						// The service role fetcher still returns admin: this must be overridden.
						if len(roles) == 1 && roles[0] == "admin" {
							return errors.New("stale service role cache reached workflow")
						}
						return svc.CheckRiskWorkflowReadTx(current, tx, actor, ticket.ID)
					})
				})
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, http.StatusOK, response.Code, "middleware admission must succeed before revocation")
			switch change {
			case "allow":
				require.NoError(t, result)
				require.True(t, callback)
			case "ticket_read_policy":
				require.ErrorIs(t, result, errcode.ErrNoPermission)
				require.True(t, callback)
			case "ticket_hidden":
				require.ErrorIs(t, result, errcode.ErrTicketNotFound)
				require.True(t, callback)
			default:
				require.Error(t, result)
				require.False(t, callback, "current credentials/L1 must fail before callback")
			}
		})
	}
}

func TestRiskWorkflowAuthorizationRejectsUnownedContext(t *testing.T) {
	svc, actor, _ := setupTicketAdmin(t)
	ctx := context.Background()
	routes := []RiskWorkflowRoute{{Path: "/api/v1/risk-items/operations/synthetic", Method: "GET"}}
	called := false
	run := func(context.Context) error { called = true; return nil }
	bare, err := testPool.Begin(ctx)
	require.NoError(t, err)
	require.ErrorIs(t, svc.WithRiskWorkflowAuthorizationTx(ctx, bare, actor, routes, run), errcode.ErrNoPermission)
	require.NoError(t, bare.Rollback(ctx))
	require.NoError(t, repository.WithIAMCommandGuard(ctx, testPool, func(guarded context.Context, tx pgx.Tx) error {
		// An IAM guard alone cannot invent middleware identity or current ticket roles.
		require.ErrorIs(t, svc.WithRiskWorkflowAuthorizationTx(guarded, tx, actor, routes, run), errcode.ErrNoPermission)
		require.ErrorIs(t, svc.CheckRiskWorkflowReadTx(guarded, tx, actor, 1), errcode.ErrNoPermission)
		child, err := tx.Begin(guarded)
		require.NoError(t, err)
		// Same backend connection is insufficient: only the owning exact tx qualifies.
		require.Same(t, tx.Conn(), child.Conn())
		require.ErrorIs(t, svc.WithRiskWorkflowAuthorizationTx(guarded, child, actor, routes, run), errcode.ErrNoPermission)
		require.ErrorIs(t, svc.CheckRiskWorkflowReadTx(guarded, child, actor, 1), errcode.ErrNoPermission)
		return child.Rollback(guarded)
	}))
	require.False(t, called)
}
