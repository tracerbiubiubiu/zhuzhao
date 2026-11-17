//go:build integration

package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao-utils/jsonutil"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func iamAUserService() *service.UserService {
	return service.NewUserService(testPool, repository.NewUserRepo(testPool), repository.NewRoleRepo(testPool), nil, nil, nil)
}

func iamADirectIDs(t *testing.T, userID int64) []int64 {
	t.Helper()
	rows, err := testPool.Query(trustedIAMTestContext(), `SELECT role_id FROM user_roles WHERE user_id=$1 ORDER BY role_id`, userID)
	require.NoError(t, err)
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}

// Wait on PostgreSQL's observed lock state, rather than assuming a scheduler
// interleaving after sleeping. Both transactions belong to this isolated DB.
func iamAWaitForLock(t *testing.T, ctx context.Context, pid uint32, done <-chan error) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			t.Fatalf("transaction returned before reaching the held target: %v", err)
		case <-timer.C:
			t.Fatal("second transaction did not reach the controlled lock barrier")
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
			var blocked bool
			err := testPool.QueryRow(ctx, `SELECT COALESCE(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&blocked)
			require.NoError(t, err)
			if blocked {
				return
			}
		}
	}
}

func TestIAMA_DirectReplacementSerializes(t *testing.T) {
	resetRBACTables(t)
	ctx, cancel := context.WithTimeout(trustedIAMTestContext(), 15*time.Second)
	defer cancel()
	a := rbacTestRole(t, "iama_replace_a", 60, false)
	b := rbacTestRole(t, "iama_replace_b", 70, false)
	target := rbacTestUser(t, "iama_replace", "IAMA_REPLACE", nil)
	repo := repository.NewUserRepo(testPool)
	txA, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer txA.Rollback(trustedIAMTestContext())
	require.NoError(t, repo.SetRolesTx(ctx, txA, target, []int64{a.ID}))
	txB, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer txB.Rollback(trustedIAMTestContext())
	done := make(chan error, 1)
	pidB := txB.Conn().PgConn().PID()
	go func() { done <- repo.SetRolesTx(ctx, txB, target, []int64{b.ID}) }()
	iamAWaitForLock(t, ctx, pidB, done)
	require.NoError(t, txA.Commit(ctx))
	require.NoError(t, <-done)
	require.NoError(t, txB.Commit(ctx))
	require.Equal(t, []int64{b.ID}, iamADirectIDs(t, target), "later replacement is the whole set; concurrent replacements must never form a union")
}

func TestIAMA_DeleteAndReplacementUseSameLockOrder(t *testing.T) {
	resetRBACTables(t)
	ctx, cancel := context.WithTimeout(trustedIAMTestContext(), 15*time.Second)
	defer cancel()
	a := rbacTestRole(t, "iama_delete_old", 60, false)
	b := rbacTestRole(t, "iama_delete_new", 70, false)
	target := rbacTestUser(t, "iama_delete", "IAMA_DELETE", []int64{a.ID})
	repo := repository.NewUserRepo(testPool)
	txA, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer txA.Rollback(trustedIAMTestContext())
	require.NoError(t, repository.AcquireIAMTransactionGuard(ctx, txA))
	_, err = txA.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, target)
	require.NoError(t, err)
	txB, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer txB.Rollback(trustedIAMTestContext())
	done := make(chan error, 1)
	pidB := txB.Conn().PgConn().PID()
	go func() { done <- repo.SoftDeleteTx(ctx, txB, target) }()
	iamAWaitForLock(t, ctx, pidB, done)
	// Trusted external transactions acquire the shared guard before row locks.
	// The delete waits before touching bindings; replacement cannot form a cycle.
	require.NoError(t, repo.SetRolesTx(ctx, txA, target, []int64{b.ID}))
	require.NoError(t, txA.Commit(ctx))
	require.NoError(t, <-done)
	require.NoError(t, txB.Commit(ctx))
	var deleted bool
	require.NoError(t, testPool.QueryRow(ctx, `SELECT deleted_at IS NOT NULL FROM users WHERE id=$1`, target).Scan(&deleted))
	require.True(t, deleted)
	require.Empty(t, iamADirectIDs(t, target))
}

// The HTTP handler and PostgreSQL are real. The actor is fixed here so this test
// proves edit/read/write semantics, not the separate router authentication gate.
func TestIAMA_DirectEditHTTPRoundTrip(t *testing.T) {
	resetRBACTables(t)
	ctx := trustedIAMTestContext()
	sa := rbacTestRole(t, "superadmin", 1, true)
	child := rbacTestRole(t, "iama_child", 50, false)
	parent := rbacTestRole(t, "iama_parent", 60, false)
	disabled := rbacTestRole(t, "iama_disabled", 70, false)
	orgRole := rbacTestRole(t, "iama_org", 80, false)
	actor := rbacTestUser(t, "iama_actor", "IAMA_ACTOR", []int64{sa.ID})
	target := rbacTestUser(t, "iama_target", "IAMA_TARGET", []int64{child.ID, disabled.ID})
	_, err := testPool.Exec(ctx, `UPDATE roles SET parent_id=$2 WHERE id=$1`, child.ID, parent.ID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE roles SET status=0 WHERE id=$1`, disabled.ID)
	require.NoError(t, err)
	var orgID int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,path,status) VALUES ('iama_org_' || $1,'合成组织',('root.iama_' || $1)::ltree,1) RETURNING id`, uniqueSuffix()).Scan(&orgID))
	_, err = testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES ($1,$2);`, target, orgID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO org_roles(org_id,role_id) VALUES ($1,$2)`, orgID, orgRole.ID)
	require.NoError(t, err)
	var created time.Time
	require.NoError(t, testPool.QueryRow(ctx, `UPDATE user_roles SET created_at='2020-01-02T03:04:05Z' WHERE user_id=$1 AND role_id=$2 RETURNING created_at`, target, disabled.ID).Scan(&created))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(service.TrustedIAMInternal(c.Request.Context()))
		c.Set("userID", actor)
		c.Next()
	})
	h := handler.NewUserHandler(iamAUserService(), nil)
	r.GET("/api/v1/users/:id/roles", h.GetUserRoles)
	r.POST("/api/v1/users/roles", h.SetRoles)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/users/%d/roles", target), nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response struct {
		Data struct {
			IDs []string `json:"role_ids"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.ElementsMatch(t, []string{strconv.FormatInt(child.ID, 10), strconv.FormatInt(disabled.ID, 10)}, response.Data.IDs,
		"edit source must retain disabled direct bindings and exclude organization/inherited projections")
	body, err := json.Marshal(map[string]any{"user_id": strconv.FormatInt(target, 10), "role_ids": response.Data.IDs})
	require.NoError(t, err)
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/roles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.ElementsMatch(t, []int64{child.ID, disabled.ID}, iamADirectIDs(t, target))
	var after time.Time
	require.NoError(t, testPool.QueryRow(ctx, `SELECT created_at FROM user_roles WHERE user_id=$1 AND role_id=$2`, target, disabled.ID).Scan(&after))
	require.True(t, created.Equal(after), "retaining a binding must not recreate its grant fact")
	_, err = testPool.Exec(ctx, `DELETE FROM user_orgs WHERE user_id=$1`, target)
	require.NoError(t, err)
	codes, err := repository.NewRoleRepo(testPool).GetEffectiveRoleCodes(ctx, target)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{child.Code, parent.Code}, codes, "saved edit must not preserve the removed organization source")
}

func TestIAMA_RoleBindingDeltaAuthorization(t *testing.T) {
	for _, scenario := range []string{"retain_disabled", "reject_new_disabled", "reject_remove_strong_disabled", "reject_soft_deleted_binding", "unknown_read_stays_empty"} {
		t.Run(scenario, func(t *testing.T) {
			resetRBACTables(t)
			ctx := trustedIAMTestContext()
			adminRole := rbacTestRole(t, "admin", 10, true)
			weak := rbacTestRole(t, "iama_weak", 40, false)
			old := rbacTestRole(t, "iama_old", 5, false)
			actor := rbacTestUser(t, "iama_admin", "IAMA_ADMIN", []int64{adminRole.ID})
			target := rbacTestUser(t, "iama_user", "IAMA_USER", []int64{weak.ID, old.ID})
			_, err := testPool.Exec(ctx, `UPDATE roles SET status=0 WHERE id=$1`, old.ID)
			require.NoError(t, err)
			svc := iamAUserService()
			before := iamADirectIDs(t, target)
			switch scenario {
			case "retain_disabled":
				require.NoError(t, svc.SetRoles(ctx, &model.SetUserRolesRequest{UserID: target, RoleIDs: jsonutil.Int64Slice{weak.ID, old.ID}}, actor))
				require.ElementsMatch(t, before, iamADirectIDs(t, target))
			case "reject_new_disabled":
				// Keep the old disabled grant within the actor's assignable range so
				// only the new disabled binding can reject this request.
				_, err = testPool.Exec(ctx, `UPDATE roles SET priority=60 WHERE id=$1`, old.ID)
				require.NoError(t, err)
				newRole := rbacTestRole(t, "iama_new_disabled", 70, false)
				_, err = testPool.Exec(ctx, `UPDATE roles SET status=0 WHERE id=$1`, newRole.ID)
				require.NoError(t, err)
				err = svc.SetRoles(ctx, &model.SetUserRolesRequest{UserID: target, RoleIDs: jsonutil.Int64Slice{weak.ID, old.ID, newRole.ID}}, actor)
				require.Error(t, err)
				require.ElementsMatch(t, before, iamADirectIDs(t, target), "invalid addition must have zero partial effects")
			case "reject_remove_strong_disabled":
				err = svc.SetRoles(ctx, &model.SetUserRolesRequest{UserID: target, RoleIDs: jsonutil.Int64Slice{weak.ID}}, actor)
				requireErrCode(t, err, errcode.ErrCannotAssignHigherRole)
				require.ElementsMatch(t, before, iamADirectIDs(t, target))
			case "reject_soft_deleted_binding":
				_, err = testPool.Exec(ctx, `UPDATE roles SET deleted_at=NOW() WHERE id=$1`, old.ID)
				require.NoError(t, err)
				_, err = svc.GetUserRoleIDs(ctx, target, actor)
				requireErrCode(t, err, errcode.ErrRoleNotFound)
				err = svc.SetRoles(ctx, &model.SetUserRolesRequest{UserID: target, RoleIDs: jsonutil.Int64Slice{weak.ID}}, actor)
				requireErrCode(t, err, errcode.ErrRoleNotFound)
				require.ElementsMatch(t, before, iamADirectIDs(t, target))
			case "unknown_read_stays_empty":
				ids, err := svc.GetUserRoleIDs(ctx, 99999999, actor)
				require.NoError(t, err)
				require.Empty(t, ids)
			}
		})
	}
}

func TestIAMA_OrgGrantRequiresIAMQualification(t *testing.T) {
	resetRBACTables(t)
	env := setupDelegation(t)
	ctx := trustedIAMTestContext()
	role := rbacTestRole(t, "iama_org_strong", 5, false)
	adminRole := rbacTestRole(t, "admin", 10, true)
	admin := rbacTestUser(t, "iama_global_admin", "IAMA_GADMIN", []int64{adminRole.ID})
	weak := rbacTestRole(t, "iama_org_creator", 60, false)
	creator := rbacTestUser(t, "iama_org_creator", "IAMA_CREATOR", []int64{weak.ID})
	var menuID int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO menus(code,name,menu_type,permission) VALUES ('iama_org_create','合成组织创建',3,'org:create') RETURNING id`).Scan(&menuID))
	_, err := testPool.Exec(ctx, `INSERT INTO role_menus(role_id,menu_id) VALUES ($1,$2)`, weak.ID, menuID)
	require.NoError(t, err)
	err = env.orgSvc.BindOrgRole(ctx, &model.BindOrgRoleRequest{OrgID: env.vgID, RoleID: role.ID}, creator)
	requireErrCode(t, err, errcode.ErrNoPermission)
	err = env.orgSvc.BindOrgRole(ctx, &model.BindOrgRoleRequest{OrgID: env.vgID, RoleID: role.ID}, admin)
	requireErrCode(t, err, errcode.ErrCannotAssignHigherRole)
	var count int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT COUNT(*) FROM org_roles WHERE org_id=$1`, env.vgID).Scan(&count))
	require.Zero(t, count)
	weakGrant := rbacTestRole(t, "iama_org_weak", 80, false)
	require.NoError(t, env.orgSvc.BindOrgRole(ctx, &model.BindOrgRoleRequest{OrgID: env.vgID, RoleID: weakGrant.ID}, admin))
	_, err = testPool.Exec(ctx, `UPDATE roles SET status=0 WHERE id=$1`, weakGrant.ID)
	require.NoError(t, err)
	require.NoError(t, env.orgSvc.UnbindOrgRole(ctx, &model.BindOrgRoleRequest{OrgID: env.vgID, RoleID: weakGrant.ID}, admin), "disabled existing grant can be legitimately removed")
	// The same org:create button must not authorize the reverse operation either.
	require.NoError(t, env.orgSvc.BindOrgRole(ctx, &model.BindOrgRoleRequest{OrgID: env.vgID, RoleID: role.ID}, env.super))
	err = env.orgSvc.UnbindOrgRole(ctx, &model.BindOrgRoleRequest{OrgID: env.vgID, RoleID: role.ID}, creator)
	requireErrCode(t, err, errcode.ErrNoPermission)
	err = env.orgSvc.UnbindOrgRole(ctx, &model.BindOrgRoleRequest{OrgID: env.vgID, RoleID: role.ID}, admin)
	requireErrCode(t, err, errcode.ErrCannotAssignHigherRole)
}
