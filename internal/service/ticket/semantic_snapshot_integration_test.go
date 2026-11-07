//go:build integration

package ticket

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"testing"
	"time"
)

func TestAuditValidatorFreshness(t *testing.T) {
	for _, path := range []string{"transition", "close", "assign"} {
		t.Run(path, func(t *testing.T) {
			svc, aid, bid, _, roles := setupTicket2a(t)
			roles[aid] = []string{"admin"}
			oid := childOrgID(t, "semantic")
			code := "semantic_" + uniqueSuffix()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
			require.NoError(t, err)
			meta := `{"open>closed":{"validator":"priority == 1"},"open>assigned":{"validator":"priority == 1"}}`
			_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: []byte(meta)})
			require.NoError(t, err)
			tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, Priority: 1, OrgID: oid}, aid)
			require.NoError(t, err)
			tx, err := testPool.Begin(ctx)
			require.NoError(t, err)
			defer tx.Rollback(context.Background())
			_, err = tx.Exec(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, tk.ID)
			require.NoError(t, err)
			done := make(chan error, 1)
			go func() {
				switch path {
				case "transition":
					_, err := svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "closed"}, aid)
					done <- err
				case "close":
					done <- svc.Close(ctx, &model.CloseTicketRequest{ID: tk.ID}, aid)
				case "assign":
					done <- svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid)
				}
			}()
			require.Eventually(t, func() bool {
				var n int
				e := testPool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND (query LIKE 'UPDATE tickets SET%' OR (query LIKE 'SELECT%FROM tickets WHERE id=$1 FOR UPDATE%'))`).Scan(&n)
				return e == nil && n > 0
			}, 3*time.Second, 5*time.Millisecond, "service passed validator and reached blocked write")
			_, err = tx.Exec(ctx, `UPDATE tickets SET priority=3 WHERE id=$1`, tk.ID)
			require.NoError(t, err)
			require.NoError(t, tx.Commit(ctx))
			result := <-done
			var state string
			var priority int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT status,priority FROM tickets WHERE id=$1`, tk.ID).Scan(&state, &priority))
			t.Logf("path=%s validator=priority1 committed_priority=%d final_state=%s service_error=%v", path, priority, state, result)
			require.ErrorIs(t, result, errcode.ErrConcurrentModification, "a stale validator cannot authorize a transition after concurrent data change")
			require.Equal(t, "open", state)
		})
	}
}

func TestAuditConditionAfterReassignment(t *testing.T) {
	svc, aid, bid, vid, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	oid := childOrgID(t, "condition")
	code := "condition_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	meta := `{"assigned>in_progress":{"roles":["assignee"]}}`
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: []byte(meta)})
	require.NoError(t, err)
	tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, Priority: 3, OrgID: oid}, aid)
	require.NoError(t, err)
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid))
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, tk.ID)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		_, e := svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "in_progress"}, bid)
		done <- e
	}()
	require.Eventually(t, func() bool {
		var n int
		e := testPool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND (query LIKE 'UPDATE tickets SET%' OR (query LIKE 'SELECT%FROM tickets WHERE id=$1 FOR UPDATE%'))`).Scan(&n)
		return e == nil && n > 0
	}, 3*time.Second, 5*time.Millisecond)
	_, err = tx.Exec(ctx, `UPDATE tickets SET assigned_to=$2 WHERE id=$1`, tk.ID, vid)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	transitionErr := <-done
	var state string
	var assignee int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT status,assigned_to FROM tickets WHERE id=$1`, tk.ID).Scan(&state, &assignee))
	t.Logf("old_assignee=%d new_assignee=%d actor=%d final_state=%s error=%v", bid, assignee, bid, state, transitionErr)
	require.ErrorIs(t, transitionErr, errcode.ErrConcurrentModification, "revoked assignee cannot pass assignee-only transition at commit")
	require.Equal(t, "assigned", state)
}

func TestAuditTransitionConfigFreshness(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	oid := childOrgID(t, "cfgfence")
	code := "cfgfence_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, Priority: 3, OrgID: oid}, aid)
	require.NoError(t, err)
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, `SELECT code FROM ticket_types WHERE code=$1 FOR UPDATE`, code)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		_, e := svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "closed"}, aid)
		done <- e
	}()
	require.Eventually(t, func() bool {
		var n int
		e := testPool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT version FROM ticket_types%'`).Scan(&n)
		return e == nil && n > 0
	}, 3*time.Second, 5*time.Millisecond)
	_, err = tx.Exec(ctx, `UPDATE ticket_types SET version=version+1,transition_meta='{"open>closed":{"validator":"priority == 1"}}' WHERE code=$1`, code)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	require.ErrorIs(t, <-done, errcode.ErrConcurrentModification)
	got, err := svc.Get(ctx, tk.ID, aid)
	require.NoError(t, err)
	require.Equal(t, "open", got.Status)
	// A fresh request now evaluates the new validator instead of bypassing it.
	_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "closed"}, aid)
	require.Error(t, err)
	require.Contains(t, err.Error(), "前置条件")
}
