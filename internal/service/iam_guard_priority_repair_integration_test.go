//go:build integration

package service_test

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestIAMGuard_OrgOnlyActorCannotUseHighPrioritySentinel(t *testing.T) {
	for _, command := range []string{"role_create", "user_create", "display_update"} {
		t.Run(command, func(t *testing.T) {
			f := targetProtectionSetup(t)
			ctx := context.Background()
			var actorRole int64
			require.NoError(t, testPool.QueryRow(ctx, `SELECT role_id FROM user_roles WHERE user_id=$1`, f.actor).Scan(&actorRole))
			_, err := testPool.Exec(ctx, `INSERT INTO org_roles(org_id,role_id) VALUES($1,$2)`, f.org, actorRole)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES($1,$2)`, f.actor, f.org)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `DELETE FROM user_roles WHERE user_id=$1`, f.actor)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `UPDATE roles SET priority=10000 WHERE id=$1`, f.weak.ID)
			require.NoError(t, err)
			users := handler.NewUserHandler(f.svc, nil)
			path := ""
			body := ""
			var action gin.HandlerFunc
			expected := errcode.ErrCannotAssignHigherRole.Code
			switch command {
			case "role_create":
				rbac := service.NewRBACService(repository.NewRoleRepo(testPool), repository.NewUserRepo(testPool), repository.NewMenuRepo(testPool), rbacTestEnforcer(t))
				action = handler.NewRoleHandler(rbac).Create
				path = "/api/v1/roles"
				body = `{"code":"sentinel_created","name":"Synthetic role","priority":10000}`
			case "user_create":
				action = users.Create
				path = "/api/v1/users"
				body = fmt.Sprintf(`{"username":"sentinel_created","employee_no":"SENTINEL_CREATED","password":"SyntheticKnown#2026","role_ids":[%q]}`, strconv.FormatInt(f.weak.ID, 10))
			case "display_update":
				action = users.Update
				path = "/api/v1/users/update"
				body = fmt.Sprintf(`{"id":%q,"version":1,"real_name":"must not update"}`, strconv.FormatInt(f.target, 10))
				expected = errcode.ErrCannotManageHigher.Code
			}
			before := targetProtectionSnapshot(t, f.target)
			w := sourceHTTP(t, f.actor, path, false, action, body)
			require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), fmt.Sprintf(`"code":%d`, expected))
			require.Equal(t, before, targetProtectionSnapshot(t, f.target))
			var created int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT (SELECT count(*) FROM users WHERE employee_no='SENTINEL_CREATED')+(SELECT count(*) FROM roles WHERE code='sentinel_created')`).Scan(&created))
			require.Zero(t, created)
		})
	}
}
