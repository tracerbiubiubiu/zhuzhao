//go:build integration

package service_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"testing"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestIAMAuditStaleCredentialAfterAdmission(t *testing.T) {
	for _, tc := range []struct {
		path   string
		replay bool
	}{
		{"/api/v1/users/roles", false}, {"/api/v1/users/roles", true}, {"/api/v1/roles/menus", false}, {"/api/v1/roles/menus", true},
	} {
		t.Run(fmt.Sprintf("%s/replay=%t", tc.path, tc.replay), func(t *testing.T) {
			ctx := context.Background()
			e := newEpochRegressionAuth(t)
			actor, err := e.repo.FindByEmployeeNo(ctx, "E800906")
			require.NoError(t, err)
			actorID := actor.ID
			repo := repository.NewRoleRepo(testPool)
			role, err := repo.FindByCode(ctx, e.role)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,'POST')`, "role::"+role.Code, tc.path)
			require.NoError(t, err)
			m, err := casbinmodel.NewModelFromFile("../../configs/casbin_model.conf")
			require.NoError(t, err)
			enforcer, err := casbin.NewSyncedEnforcer(m)
			require.NoError(t, err)
			_, err = enforcer.AddPolicy("role::"+role.Code, tc.path, "POST")
			require.NoError(t, err)
			rbac := service.NewRBACService(repo, e.repo, repository.NewMenuRepo(testPool), enforcer)
			users, _, _ := newUserServiceForD2(t)
			weak := rbacTestRole(t, "credential_"+uniqueSuffix(), 80, false)
			action := handler.NewUserHandler(users, nil).SetRoles
			body := fmt.Appendf(nil, `{"user_id":"%d","role_ids":["%d"]}`, e.user.ID, weak.ID)
			credential := e.actor.AccessToken
			var invalidate func()
			if tc.path == "/api/v1/users/roles" {
				invalidate = func() {
					e := repository.WithIAMGuard(ctx, testPool, func(ctx context.Context) error {
						_, e := repository.IAMDB(ctx, testPool).Exec(ctx, "UPDATE users SET session_epoch=session_epoch+1 WHERE id=$1", actorID)
						return e
					})
					require.NoError(t, e)
				}
			} else {
				pat, token, err := service.NewPatService(repository.NewPatRepo(testPool), slog.Default()).Create(ctx, actorID, "synthetic audit credential", 1)
				require.NoError(t, err)
				credential = token
				menu := d2TestMenu(t, "credential_"+uniqueSuffix(), "", "")
				body = fmt.Appendf(nil, `{"role_id":"%d","menu_ids":["%d"]}`, weak.ID, menu.ID)
				action = handler.NewRoleHandler(rbac).AssignMenus
				invalidate = func() { require.NoError(t, repository.NewPatRepo(testPool).Revoke(ctx, pat.ID, actorID)) }
			}
			r := gin.New()
			r.Use(middleware.JWT(e.mgr, e.rdb, repository.NewPatRepo(testPool), middleware.WithAccountSecurity(c05SnapshotSource{pool: testPool})))
			stale := false
			r.POST(tc.path, middleware.CasbinAuth(enforcer, rbac, nil), func(c *gin.Context) {
				if stale {
					invalidate()
				}
				c.Next()
			}, action)
			key := "credential-recheck-" + uniqueSuffix()
			if tc.replay {
				require.Equal(t, 200, epochRequest(ctx, r, http.MethodPost, tc.path, credential, key, body).Code)
			}
			stale = true
			res := epochRequest(ctx, r, http.MethodPost, tc.path, credential, key, body)
			require.Equal(t, 403, res.Code, res.Body.String())
			var count int
			require.NoError(t, testPool.QueryRow(ctx, "SELECT count(*) FROM iam_command_receipts WHERE actor_id=$1 AND operation_id=$2", actorID, key).Scan(&count))
			if tc.replay {
				require.Equal(t, 1, count)
			} else {
				require.Zero(t, count)
			}
		})
	}
}
