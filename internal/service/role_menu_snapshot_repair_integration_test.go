//go:build integration

package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao-utils/jsonutil"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestRoleMenuSnapshotPreservesNewerSubmission(t *testing.T) {
	resetRBACTables(t)
	ctx := trustedIAMTestContext()
	svc := newRBACServiceForTest(t)
	super := rbacTestRole(t, "superadmin", 1, true)
	actor := rbacTestUser(t, "menu_snapshot_owner", "E630001", []int64{super.ID})
	target := rbacTestRole(t, "admin", 10, true)
	other := rbacTestRole(t, "menu_snapshot_other", 80, false)
	ids := []int64{}
	for _, code := range []string{"menu_snapshot_a", "menu_snapshot_b", "menu_snapshot_c"} {
		var id int64
		require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO menus(code,name,menu_type) VALUES($1,$1,3) RETURNING id`, code).Scan(&id))
		ids = append(ids, id)
	}
	original := jsonutil.Int64Slice{ids[0]}
	require.NoError(t, svc.AssignMenus(ctx, &model.AssignMenusRequest{RoleID: target.ID, MenuIDs: original}, actor))
	_, err := testPool.Exec(ctx, `INSERT INTO role_menus(role_id,menu_id) VALUES($1,$2)`, other.ID, ids[2])
	require.NoError(t, err)
	// Two administrators read the same collection before one submits a new set.
	a, err := svc.GetRoleMenuIDs(ctx, target.ID, actor)
	require.NoError(t, err)
	b := jsonutil.Int64Slice(append([]int64{}, a...))
	require.NoError(t, svc.AssignMenus(ctx, &model.AssignMenusRequest{RoleID: target.ID, MenuIDs: jsonutil.Int64Slice{ids[1]}, OriginalMenuIDs: &original}, actor))
	requireErrCode(t, svc.AssignMenus(ctx, &model.AssignMenusRequest{RoleID: target.ID, MenuIDs: jsonutil.Int64Slice{}, OriginalMenuIDs: &b}, actor), errcode.ErrConcurrentModification)
	got, err := svc.GetRoleMenuIDs(ctx, target.ID, actor)
	require.NoError(t, err)
	require.Equal(t, []int64{ids[1]}, got)
	otherIDs, err := svc.GetRoleMenuIDs(ctx, other.ID, actor)
	require.NoError(t, err)
	require.Equal(t, []int64{ids[2]}, otherIDs)
	// Authorization still precedes the condition, including hidden superadmin reads.
	low := rbacTestRole(t, "menu_snapshot_low", 90, false)
	lowActor := rbacTestUser(t, "menu_snapshot_low_actor", "E630002", []int64{low.ID})
	requireErrCode(t, svc.AssignMenus(ctx, &model.AssignMenusRequest{RoleID: target.ID, MenuIDs: jsonutil.Int64Slice{}, OriginalMenuIDs: &b}, lowActor), errcode.ErrRoleIsSystem)
	_, err = svc.GetRoleMenuIDs(ctx, super.ID, lowActor)
	requireErrCode(t, err, errcode.ErrRoleNotFound)
	// Explicit empty snapshot protects an empty collection; omitted snapshot retains legacy replacement.
	latest := jsonutil.Int64Slice(got)
	require.NoError(t, svc.AssignMenus(ctx, &model.AssignMenusRequest{RoleID: target.ID, MenuIDs: jsonutil.Int64Slice{}, OriginalMenuIDs: &latest}, actor))
	empty := jsonutil.Int64Slice{}
	require.NoError(t, svc.AssignMenus(ctx, &model.AssignMenusRequest{RoleID: target.ID, MenuIDs: original, OriginalMenuIDs: &empty}, actor))
	require.NoError(t, svc.AssignMenus(ctx, &model.AssignMenusRequest{RoleID: target.ID, MenuIDs: latest}, actor))
}
