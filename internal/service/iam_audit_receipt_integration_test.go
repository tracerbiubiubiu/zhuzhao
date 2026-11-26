//go:build integration

package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestIAMAuditLostResponseAndReplay(t *testing.T) {
	resetRBACTables(t)
	users, _, _ := newUserServiceForD2(t)
	roles := newRBACServiceForTest(t)
	sa := rbacTestRole(t, "superadmin", 1, true)
	admin := rbacTestRole(t, "admin", 10, true)
	actor := rbacTestUser(t, "receipt_actor", "E633021", []int64{sa.ID})
	target := rbacTestUser(t, "receipt_target", "E633022", nil)
	menu := d2TestMenu(t, "receipt_menu", "", "")
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", actor)
		c.Request = c.Request.WithContext(service.TrustedIAMInternal(c.Request.Context()))
		c.Next()
	})
	uh := handler.NewUserHandler(users, nil)
	router.GET("/api/v1/operations/receipt", uh.IAMReceipt)
	router.POST("/users/roles", uh.SetRoles)
	router.POST("/roles/menus", handler.NewRoleHandler(roles).AssignMenus)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Synthetic-Lose-Response") == "yes" {
			captured := httptest.NewRecorder()
			router.ServeHTTP(captured, r)
			if captured.Code != 200 {
				t.Errorf("unexpected first status %d", captured.Code)
			}
			panic(http.ErrAbortHandler)
		}
		router.ServeHTTP(w, r)
	}))
	defer server.Close()
	for _, tc := range []struct {
		path, command, target, ids, query string
		objectID, bindingID               int64
	}{
		{"/users/roles", "user.roles", "user_id", "role_ids", "SELECT count(*) FROM user_roles WHERE user_id=$1", target, admin.ID},
		{"/roles/menus", "role.menus", "role_id", "menu_ids", "SELECT count(*) FROM role_menus WHERE role_id=$1", admin.ID, menu.ID},
	} {
		t.Run(tc.command, func(t *testing.T) {
			key := "receipt-key-" + tc.command + "-" + uniqueSuffix()
			send := func(k, ids string, lose bool) (*http.Response, error) {
				body := fmt.Sprintf(`{"%s":"%d","%s":%s}`, tc.target, tc.objectID, tc.ids, ids)
				r, err := http.NewRequest(http.MethodPost, server.URL+tc.path, bytes.NewBufferString(body))
				require.NoError(t, err)
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Idempotency-Key", k)
				if lose {
					r.Header.Set("Synthetic-Lose-Response", "yes")
				}
				return server.Client().Do(r)
			}
			initial := fmt.Sprintf(`["%d"]`, tc.bindingID)
			_, err := send(key, initial, true)
			require.Error(t, err)
			bound := func() int {
				var n int
				require.NoError(t, testPool.QueryRow(trustedIAMTestContext(), tc.query, tc.objectID).Scan(&n))
				return n
			}
			require.Equal(t, 1, bound())
			resp, err := send(key+"-revoke", "[]", false)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, 200, resp.StatusCode)
			require.Zero(t, bound())
			resp, err = send(key, initial, false)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, 200, resp.StatusCode)
			require.Zero(t, bound(), "a replay after revocation must never restore an original grant")
			require.Equal(t, key, resp.Header.Get("X-Operation-ID"))
			resp, err = send(key, "[]", false)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, 409, resp.StatusCode)
			checkReceipt := func(selector, k string, wantChanged bool) {
				t.Helper()
				res, err := server.Client().Get(server.URL + "/api/v1/operations/receipt?" + selector + "=" + k)
				require.NoError(t, err)
				defer res.Body.Close()
				raw, err := io.ReadAll(res.Body)
				require.NoError(t, err)
				require.Equal(t, 200, res.StatusCode, string(raw))
				var envelope struct{ Data map[string]any }
				require.NoError(t, json.Unmarshal(raw, &envelope))
				name := repository.IAMUserRoles
				if tc.command == "role.menus" {
					name = repository.IAMRoleMenus
				}
				require.Equal(t, map[string]any{"operation_id": k, "command": name, "target_id": fmt.Sprint(tc.objectID), "status": "domain_committed", "changed": wantChanged}, envelope.Data)
			}
			checkReceipt("idempotency_key", key, true)
			checkReceipt("operation_id", key+"-revoke", true)
			noopKey := key + "-noop"
			resp, err = send(noopKey, "[]", false)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, 200, resp.StatusCode)
			checkReceipt("operation_id", noopKey, false)
			duplicate := fmt.Sprintf(`["%d","%d"]`, tc.bindingID, tc.bindingID)
			resp, err = send(key, duplicate, false)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, 200, resp.StatusCode)
			require.Zero(t, bound())

		})
	}
}

func TestIAMAuditReceiptRechecksCurrentPrincipalAndAction(t *testing.T) {
	e := newEpochRegressionAuth(t)
	users, _, roles := newUserServiceForD2(t)
	actor, err := e.repo.FindByEmployeeNo(context.Background(), "E800906")
	require.NoError(t, err)
	key := "receipt-real-auth-" + uniqueSuffix()
	ctx, _, err := service.IAMOperationContext(trustedIAMTestContext(), key)
	require.NoError(t, err)
	require.NoError(t, users.SetRoles(ctx, &model.SetUserRolesRequest{UserID: e.user.ID}, actor.ID))
	m, err := casbinmodel.NewModelFromFile("../../configs/casbin_model.conf")
	require.NoError(t, err)
	enforcer, err := casbin.NewSyncedEnforcer(m)
	require.NoError(t, err)
	rbac := service.NewRBACService(roles, e.repo, nil, enforcer)
	router := gin.New()
	router.Use(middleware.JWT(e.mgr, e.rdb, repository.NewPatRepo(testPool), middleware.WithAccountSecurity(c05SnapshotSource{pool: testPool})))
	router.GET("/api/v1/operations/receipt", middleware.SelfService(), middleware.CasbinAuth(enforcer, rbac, nil), handler.NewUserHandler(users, nil).IAMReceipt)
	route := "/api/v1/operations/receipt?operation_id=" + key
	read := func(token string) int {
		return epochRequest(context.Background(), router, "GET", route, token, "", nil).Code
	}
	_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,'/api/v1/users/roles','POST')`, "role::"+e.role)
	require.NoError(t, err)
	require.Equal(t, 200, read(e.actor.AccessToken))
	// The target has a current credential and role, but does not own the original receipt.
	newRole := rbacTestRole(t, "audit_receipt_observer", 80, false)
	require.NoError(t, e.repo.SetRoles(ctx, e.user.ID, []int64{newRole.ID}))
	targetLogin := e.login(t, "EpochOld#2026", "receipt-reader")
	require.Equal(t, 404, read(targetLogin.AccessToken))
	_, err = testPool.Exec(ctx, `DELETE FROM casbin_rule WHERE ptype='p' AND v0=$1 AND v1='/api/v1/users/roles'`, "role::"+e.role)
	require.NoError(t, err)
	require.Equal(t, 404, read(e.actor.AccessToken))
}
