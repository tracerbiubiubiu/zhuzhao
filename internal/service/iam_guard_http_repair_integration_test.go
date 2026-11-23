//go:build integration

package service_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestIAMGuard_HTTPMenuRevocationAfterMiddlewareDeniesWithoutEffects(t *testing.T) {
	for _, command := range []string{"reset", "display_update"} {
		t.Run(command, func(t *testing.T) {
			f := targetProtectionSetup(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			// Keep target weak: the denial below must come from fresh API permission,
			// never from the independent target effective-role guard.
			_, err := testPool.Exec(ctx, `DELETE FROM org_roles WHERE org_id=$1`, f.org)
			require.NoError(t, err)
			path := "/api/v1/users/password/reset"
			body := fmt.Sprintf(`{"user_id":%q,"password":"SyntheticHTTPPassword#2026"}`, strconv.FormatInt(f.target, 10))
			h := handler.NewUserHandler(f.svc, nil)
			action := h.ResetPassword
			if command == "display_update" {
				path = "/api/v1/users/update"
				body = fmt.Sprintf(`{"id":%q,"version":1,"real_name":"Synthetic display"}`, strconv.FormatInt(f.target, 10))
				action = h.Update
			}
			var roleID int64
			require.NoError(t, testPool.QueryRow(ctx, `SELECT role_id FROM user_roles WHERE user_id=$1`, f.actor).Scan(&roleID))
			_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p','role::protect_actor',$1,'POST')`, path)
			require.NoError(t, err)
			m, err := casbinmodel.NewModelFromFile("../../configs/casbin_model.conf")
			require.NoError(t, err)
			enforcer, err := casbin.NewSyncedEnforcer(m)
			require.NoError(t, err)
			_, err = enforcer.AddPolicy("role::protect_actor", path, "POST")
			require.NoError(t, err)
			rbac := service.NewRBACService(repository.NewRoleRepo(testPool), repository.NewUserRepo(testPool), repository.NewMenuRepo(testPool), enforcer)
			actor, err := repository.NewUserRepo(testPool).SecuritySnapshot(ctx, f.actor)
			require.NoError(t, err)
			admitted, release := make(chan struct{}), make(chan struct{})
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set("userID", f.actor)
				c.Set("session_epoch", actor.SessionEpoch)
				c.Next()
			}, middleware.CasbinAuth(enforcer, rbac, nil))
			r.POST(path, func(c *gin.Context) {
				close(admitted)
				select {
				case <-release:
				case <-ctx.Done():
				}
				c.Next()
			}, action)
			before := targetProtectionSnapshot(t, f.target)
			w := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				defer close(done)
				req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body)).WithContext(ctx)
				req.Header.Set("Content-Type", "application/json")
				r.ServeHTTP(w, req)
			}()
			select {
			case <-admitted:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			require.NoError(t, repository.NewRoleRepo(testPool).AssignMenus(ctx, &model.Role{ID: roleID, Code: "protect_actor"}, nil, nil))
			close(release)
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), fmt.Sprintf(`"code":%d`, errcode.ErrNoPermission.Code))
			require.Equal(t, before, targetProtectionSnapshot(t, f.target))
		})
	}
}

func TestIAMGuard_ServiceMissingPurposeFailsClosed(t *testing.T) {
	f := targetProtectionSetup(t)
	before := targetProtectionSnapshot(t, f.target)
	// Deliberately no HTTP capability and no explicit trusted-internal fixture.
	_, err := f.svc.ResetPassword(context.Background(), &model.ResetPasswordRequest{UserID: f.target, Password: "SyntheticNoPurpose#2026"}, f.actor, "")
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	require.Equal(t, before, targetProtectionSnapshot(t, f.target))
}
