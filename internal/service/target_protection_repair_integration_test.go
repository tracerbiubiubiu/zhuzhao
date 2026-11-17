//go:build integration

package service_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

type targetProtectionFixture struct {
	svc                        *service.UserService
	actor, target, org, parent int64
	strong, weak               *model.Role
}

func targetProtectionSetup(t *testing.T) targetProtectionFixture {
	t.Helper()
	resetRBACTables(t)
	ctx := trustedIAMTestContext()
	f := targetProtectionFixture{}
	actorRole := rbacTestRole(t, "protect_actor", 60, false)
	f.strong = rbacTestRole(t, "protect_strong", 5, false)
	f.weak = rbacTestRole(t, "protect_weak", 80, false)
	f.actor = rbacTestUser(t, "protect_actor", "PROTECT_ACTOR", []int64{actorRole.ID})
	f.target = rbacTestUser(t, "protect_target", "PROTECT_TARGET", []int64{f.weak.ID})
	code := "protect_" + uniqueSuffix()
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,parent_id,path,status) VALUES ($1,'合成保护父组',1,$2::ltree,1) RETURNING id`, code, "root."+code).Scan(&f.parent))
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,parent_id,path,status) VALUES ($1,'合成保护子组',$2,$3::ltree,1) RETURNING id`, code+"_child", f.parent, "root."+code+".child").Scan(&f.org))
	_, err := testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES ($1,$2)`, f.target, f.org)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO org_roles(org_id,role_id) VALUES ($1,$2)`, f.org, f.strong.ID)
	require.NoError(t, err)
	f.svc, _, _ = newUserServiceForD2(t)
	return f
}

func targetProtectionSnapshot(t *testing.T, userID int64) string {
	t.Helper()
	var snapshot string
	require.NoError(t, testPool.QueryRow(trustedIAMTestContext(), `SELECT md5(jsonb_build_object(
		'user',to_jsonb(u),
		'roles',(SELECT jsonb_agg(to_jsonb(r) ORDER BY role_id) FROM user_roles r WHERE user_id=u.id),
		'orgs',(SELECT jsonb_agg(to_jsonb(m) ORDER BY org_id) FROM user_orgs m WHERE user_id=u.id),
		'owners',(SELECT jsonb_agg(jsonb_build_array(id,owner_user_ids) ORDER BY id) FROM organizations WHERE u.id=ANY(owner_user_ids))
	)::text) FROM users u WHERE id=$1`, userID).Scan(&snapshot))
	return snapshot
}

// These are real handler/service/PostgreSQL effects. JWT/L1 is deliberately
// fixed to a permitted actor here; this tests the additional target guard.
func TestTargetProtectionRepair_SensitiveHTTPCommandsDenyWithoutEffects(t *testing.T) {
	for _, command := range []string{"reset", "delete", "disable", "enable", "roles", "orgs", "employee_no", "domain_account", "user_domain"} {
		t.Run(command, func(t *testing.T) {
			f := targetProtectionSetup(t)
			if command == "enable" {
				_, err := testPool.Exec(trustedIAMTestContext(), `UPDATE users SET status=0 WHERE id=$1`, f.target)
				require.NoError(t, err)
			}
			user, err := repository.NewUserRepo(testPool).FindByID(trustedIAMTestContext(), f.target)
			require.NoError(t, err)
			before := targetProtectionSnapshot(t, f.target)
			h := handler.NewUserHandler(f.svc, nil)
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Request = c.Request.WithContext(service.TrustedIAMInternal(c.Request.Context()))
				c.Set("userID", f.actor)
				c.Next()
			})
			body := map[string]any{"user_id": strconv.FormatInt(f.target, 10)}
			var action gin.HandlerFunc
			switch command {
			case "reset":
				action, body["password"] = h.ResetPassword, "SyntheticNewPassword#2026"
			case "delete":
				action = h.Delete
			case "disable", "enable":
				action = h.UpdateStatus
				body["status"] = 0
				if command == "enable" {
					body["status"] = 1
				}
			case "roles":
				action, body["role_ids"] = h.SetRoles, []string{}
			case "orgs":
				action, body["org_ids"] = h.SetUserOrgs, []string{}
			default:
				action = h.Update
				body = map[string]any{"id": strconv.FormatInt(f.target, 10), "version": user.Version, command: "synthetic-changed"}
			}
			r.POST("/command", action)
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/command", bytes.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			code := errcode.ErrCannotManageHigher.Code
			if command == "reset" {
				code = errcode.ErrCannotResetHigher.Code
			}
			require.Contains(t, w.Body.String(), fmt.Sprintf(`"code":%d`, code))
			require.Equal(t, before, targetProtectionSnapshot(t, f.target), "denial must leave identity/password/status/direct/org/owner facts unchanged")
		})
	}
}

func TestTargetProtectionRepair_FieldAndSelfCompatibility(t *testing.T) {
	f := targetProtectionSetup(t)
	ctx := trustedIAMTestContext()
	user, err := repository.NewUserRepo(testPool).FindByID(ctx, f.target)
	require.NoError(t, err)
	updated, err := f.svc.Update(ctx, &model.UpdateUserRequest{ID: f.target, Version: user.Version, RealName: strPtr("合成展示资料")}, f.actor)
	require.NoError(t, err, "ordinary display fields retain the original direct-role target policy")
	_, err = f.svc.Update(ctx, &model.UpdateUserRequest{ID: f.target, Version: updated.Version, EmployeeNo: strPtr(updated.EmployeeNo)}, f.actor)
	require.NoError(t, err, "an explicit unchanged identity value is not an identity change")
	user, err = repository.NewUserRepo(testPool).FindByID(ctx, f.target)
	require.NoError(t, err)
	_, err = f.svc.Update(ctx, &model.UpdateUserRequest{ID: f.target, Version: user.Version, DomainAccount: strPtr("self-change")}, f.target)
	require.NoError(t, err, "the original self branch stays available")
	err = f.svc.SetRoles(ctx, &model.SetUserRolesRequest{UserID: f.target, RoleIDs: []int64{f.strong.ID}}, f.target)
	requireErrCode(t, err, errcode.ErrCannotAssignHigherRole)
}

func TestTargetProtectionRepair_CurrentSourcesAndSuperadmin(t *testing.T) {
	for _, source := range []string{"active", "disabled_role", "disabled_org", "disabled_parent", "expired", "revoked", "superadmin", "stronger_actor"} {
		t.Run(source, func(t *testing.T) {
			f := targetProtectionSetup(t)
			ctx := trustedIAMTestContext()
			var err error
			switch source {
			case "disabled_role":
				_, err = testPool.Exec(ctx, `UPDATE roles SET status=0 WHERE id=$1`, f.strong.ID)
			case "disabled_org":
				_, err = testPool.Exec(ctx, `UPDATE organizations SET status=0 WHERE id=$1`, f.org)
			case "disabled_parent":
				_, err = testPool.Exec(ctx, `UPDATE organizations SET status=0 WHERE id=$1`, f.parent)
			case "expired":
				_, err = testPool.Exec(ctx, `UPDATE user_orgs SET expires_at=NOW()-INTERVAL '1 second' WHERE user_id=$1`, f.target)
			case "revoked":
				_, err = testPool.Exec(ctx, `DELETE FROM org_roles WHERE org_id=$1`, f.org)
			case "superadmin":
				sa := rbacTestRole(t, "superadmin", 1, true)
				require.NoError(t, repository.NewUserRepo(testPool).SetRoles(ctx, f.actor, []int64{sa.ID}))
				_, err = testPool.Exec(ctx, `UPDATE roles SET priority=1 WHERE id=$1`, f.strong.ID)
			case "stronger_actor":
				_, err = testPool.Exec(ctx, `UPDATE roles SET priority=2 WHERE code='protect_actor'`)
			}
			require.NoError(t, err)
			user, err := repository.NewUserRepo(testPool).FindByID(ctx, f.target)
			require.NoError(t, err)
			_, err = f.svc.Update(ctx, &model.UpdateUserRequest{ID: f.target, Version: user.Version, DomainAccount: strPtr("current-source-check")}, f.actor)
			if source == "active" {
				requireErrCode(t, err, errcode.ErrCannotManageHigher)
			} else {
				require.NoError(t, err, "inactive sources do not create target authority; legitimate stronger/superadmin actors keep their original authority")
			}
		})
	}
}
