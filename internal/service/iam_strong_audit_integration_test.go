//go:build integration

package service_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestIAMStrongAuditAtomic(t *testing.T) {
	resetRBACTables(t)
	ctx := trustedIAMTestContext()
	users, _, _ := newUserServiceForD2(t)
	roles := newRBACServiceForTest(t)
	sa := rbacTestRole(t, "superadmin", 1, true)
	admin := rbacTestRole(t, "admin", 10, true)
	actor := rbacTestUser(t, "audit_super", "E633001", []int64{sa.ID})
	target := rbacTestUser(t, "audit_user", "E633002", nil)
	menu := d2TestMenu(t, "audit_menu", "", "")
	set := func(ids []int64) error {
		return users.SetRoles(ctx, &model.SetUserRolesRequest{UserID: target, RoleIDs: ids}, actor)
	}
	assign := func(ids []int64) error {
		return roles.AssignMenus(ctx, &model.AssignMenusRequest{RoleID: admin.ID, MenuIDs: ids}, actor)
	}
	var initial, initialDeliveries int
	require.NoError(t, testPool.QueryRow(ctx, "SELECT count(*) FROM iam_audit_deliveries").Scan(&initialDeliveries))
	require.NoError(t, testPool.QueryRow(ctx, "SELECT count(*) FROM iam_audit_facts").Scan(&initial))
	require.NoError(t, set([]int64{admin.ID}))
	require.NoError(t, assign([]int64{menu.ID}))
	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		require.NoError(t, testPool.QueryRow(ctx, q, args...).Scan(&n))
		return n
	}
	require.Equal(t, initial+2, count("SELECT count(*) FROM iam_audit_facts"))
	require.Equal(t, initialDeliveries, count("SELECT count(*) FROM iam_audit_deliveries"))
	service.ConfigureIAMAuditDelivery(users, roles, "http://127.0.0.1")
	for _, table := range []string{"iam_audit_facts", "iam_audit_deliveries", "iam_command_receipts"} {
		t.Run(table, func(t *testing.T) {
			_, err := testPool.Exec(ctx, `CREATE FUNCTION reject_iam_fact() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$; CREATE TRIGGER reject_insert BEFORE INSERT ON `+table+` FOR EACH ROW EXECUTE FUNCTION reject_iam_fact()`)
			require.NoError(t, err)
			t.Cleanup(func() {
				_, e := testPool.Exec(ctx, "DROP TRIGGER reject_insert ON "+table+"; DROP FUNCTION reject_iam_fact()")
				require.NoError(t, e)
			})
			require.ErrorContains(t, set(nil), "injected failure")
			require.ErrorContains(t, assign(nil), "injected failure")
			require.Equal(t, initialDeliveries, count("SELECT count(*) FROM iam_audit_deliveries"))
			require.Equal(t, initial+2, count("SELECT count(*) FROM iam_audit_facts"))
			require.Equal(t, 1, count("SELECT count(*) FROM user_roles WHERE user_id=$1 AND role_id=$2", target, admin.ID))
			require.Equal(t, 1, count("SELECT count(*) FROM role_menus WHERE role_id=$1 AND menu_id=$2", admin.ID, menu.ID))
		})
	}
}

func TestIAMStrongAuditCollector(t *testing.T) {
	resetRBACTables(t)
	ctx := trustedIAMTestContext()
	users, userRepo, _ := newUserServiceForD2(t)
	roles := newRBACServiceForTest(t)
	sa := rbacTestRole(t, "superadmin", 1, true)
	viewer := rbacTestRole(t, "audit_viewer", 90, false)
	actor := rbacTestUser(t, "collect_actor", "E633011", []int64{sa.ID})
	target := rbacTestUser(t, "collect_target", "E633012", nil)

	var calls atomic.Int32
	reached := make(chan struct{})
	release := make(chan struct{})
	type capture struct {
		id   string
		body []byte
	}
	requests := make(chan capture, 5)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read failed", 400)
			return
		}
		requests <- capture{r.Header.Get("Idempotency-Key"), body}
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2:
			close(reached)
			<-release
			w.WriteHeader(http.StatusNoContent)
		case 3:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	releaseOld := sync.OnceFunc(func() { close(release) })
	defer releaseOld()
	service.ConfigureIAMAuditDelivery(users, roles, server.URL)
	require.NoError(t, users.SetRoles(ctx, &model.SetUserRolesRequest{UserID: target, RoleIDs: []int64{viewer.ID, viewer.ID}}, actor))
	// A later administrator removes the binding. Replaying this original grant would reinstate it.
	require.NoError(t, userRepo.SetRoles(ctx, target, nil))
	worker := func() *service.IAMAuditDelivery {
		w, err := service.NewIAMAuditDelivery(testPool, server.URL)
		require.NoError(t, err)
		return w
	}
	require.Error(t, worker().Handle(ctx, nil))
	first := <-requests
	expire := func() {
		tag, err := testPool.Exec(ctx, "UPDATE iam_audit_deliveries SET available_at=now() WHERE record_id=$1 AND delivered_at IS NULL", first.id)
		require.NoError(t, err)
		require.EqualValues(t, 1, tag.RowsAffected())
	}
	expire()
	old := worker()
	done := make(chan error, 1)
	go func() { done <- old.Handle(ctx, nil) }()
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("stalled delivery did not reach collector")
	}
	expire()
	require.Error(t, worker().Handle(ctx, nil))
	releaseOld()
	require.ErrorContains(t, <-done, "lease expired")
	expire() // The expired worker's successful response cannot confirm the newer failed claim.
	restarted := worker()
	require.NoError(t, restarted.Handle(ctx, nil))
	require.NoError(t, restarted.Handle(ctx, nil))
	require.EqualValues(t, 4, calls.Load())
	for i := 0; i < 3; i++ {
		replay := <-requests
		require.Equal(t, first.id, replay.id)
		require.Equal(t, first.body, replay.body)
	}
	var payload map[string]any
	require.NoError(t, json.Unmarshal(first.body, &payload))
	require.Equal(t, map[string]any{"record_id": first.id, "event_name": "iam.user.roles.set", "occurred_at": payload["occurred_at"], "actor_id": fmt.Sprint(actor), "target_id": fmt.Sprint(target), "added_ids": []any{fmt.Sprint(viewer.ID)}, "removed_ids": []any{}, "outcome": "success", "policy_changed": false}, payload)
	var bound bool
	require.NoError(t, testPool.QueryRow(ctx, "SELECT EXISTS(SELECT FROM user_roles WHERE user_id=$1)", target).Scan(&bound))
	require.False(t, bound)
	_, err := testPool.Exec(ctx, "UPDATE iam_audit_facts SET target_id=target_id WHERE record_id=$1", first.id)
	require.ErrorContains(t, err, "immutable")
}
