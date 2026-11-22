//go:build integration

package ticket

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func lifecycleRepairSetup(t *testing.T) (*Service, int64, int64, int64, string) {
	t.Helper()
	svc, actor, assignee, _, roles := setupTicket2a(t)
	roles[actor] = []string{"admin"}
	org := childOrgID(t, "lifecycle_"+uniqueSuffix())
	code := "lifecycle_" + uniqueSuffix()
	_, err := svc.CreateTicketType(context.Background(), &model.CreateTicketTypeRequest{Code: code, Name: "合成生命周期"})
	require.NoError(t, err)
	_, err = svc.UpdateTicketType(context.Background(), code, &model.UpdateTicketTypeRequest{TransitionMeta: []byte(`{"open":{"sla_due_hours":1},"assigned":{"sla_due_hours":2},"in_progress":{"sla_due_hours":3},"closed":{"sla_due_hours":5}}`)})
	require.NoError(t, err)
	publishD01(t, svc, code, actor)
	return svc, actor, assignee, org, code
}

func lifecycleRepairHours(t *testing.T, id int64, expected *float64) {
	t.Helper()
	var hours *float64
	require.NoError(t, testPool.QueryRow(context.Background(), `SELECT EXTRACT(EPOCH FROM (sla_due_at-status_changed_at))/3600 FROM tickets WHERE id=$1`, id).Scan(&hours))
	if expected == nil {
		require.Nil(t, hours, "closed/unconfigured states cannot retain a stale due time")
	} else {
		require.NotNil(t, hours, "every configured state entry must start its deadline")
		require.InDelta(t, *expected, *hours, 0.000001)
	}
}

func TestF5LifecycleRepair_SLAEveryEntryPoint(t *testing.T) {
	for _, entry := range []string{"create", "assign", "unassign", "reassign", "close", "transition_closed", "automatic_assign"} {
		t.Run(entry, func(t *testing.T) {
			svc, actor, assignee, org, code := lifecycleRepairSetup(t)
			ctx := context.Background()
			tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "合成截止时间", OrgID: org}, actor)
			require.NoError(t, err)
			hours := float64(1)
			switch entry {
			case "create":
				require.NotNil(t, tk.SLADueAt, "create response must reflect the persisted deadline")
			case "assign":
				require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &assignee}, actor))
				hours = 2
			case "unassign":
				require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &assignee}, actor))
				require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID}, actor))
			case "reassign":
				require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &assignee}, actor))
				var before *time.Time
				require.NoError(t, testPool.QueryRow(ctx, `UPDATE tickets SET sla_due_at=NOW()+INTERVAL '37 minutes' WHERE id=$1 RETURNING sla_due_at`, tk.ID).Scan(&before))
				require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &actor}, actor))
				var after *time.Time
				require.NoError(t, testPool.QueryRow(ctx, `SELECT sla_due_at FROM tickets WHERE id=$1`, tk.ID).Scan(&after))
				require.Equal(t, before, after, "same-state reassignment must not restart the clock")
				return
			case "close", "transition_closed":
				_, err := testPool.Exec(ctx, `UPDATE tickets SET sla_due_at=NOW()+INTERVAL '1 hour' WHERE id=$1`, tk.ID)
				require.NoError(t, err)
				if entry == "close" {
					require.NoError(t, svc.Close(ctx, &model.CloseTicketRequest{ID: tk.ID}, actor))
				} else {
					_, err := svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: StatusClosed}, actor)
					require.NoError(t, err)
				}
				lifecycleRepairHours(t, tk.ID, nil)
				return
			case "automatic_assign":
				_, err := testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, assignee, org)
				require.NoError(t, err)
				dispatch := NewDispatchService(repository.NewAssignmentRuleRepo(testPool), svc, nil)
				_, err = dispatch.CreateRule(ctx, &model.CreateAssignmentRuleRequest{TypeCode: code, Priority: 1, TargetOrg: org, TargetUser: &assignee}, actor)
				require.NoError(t, err)
				svc.SetDispatcher(dispatch)
				tk, err = svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "真实自动分派", OrgID: org}, actor)
				require.NoError(t, err)
				hours = 2
			}
			lifecycleRepairHours(t, tk.ID, &hours)
		})
	}
}

func TestF5LifecycleRepair_TransitionAndCloseCommentEvents(t *testing.T) {
	for _, operation := range []string{"transition", "close"} {
		t.Run(operation, func(t *testing.T) {
			svc, actor, assignee, org, code := lifecycleRepairSetup(t)
			ctx := context.Background()
			tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "合成事件连续性", OrgID: org}, actor)
			require.NoError(t, err)
			if operation == "transition" {
				require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &assignee}, actor))
				_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: StatusInProgress, Comment: "合成转换说明"}, assignee)
			} else {
				err = svc.Close(ctx, &model.CloseTicketRequest{ID: tk.ID, Comment: "合成关闭说明"}, actor)
			}
			require.NoError(t, err)
			var changed, commented, comments int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE action='status_changed'),count(*) FILTER(WHERE action='comment') FROM ticket_events WHERE ticket_id=$1`, tk.ID).Scan(&changed, &commented))
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_comments WHERE ticket_id=$1`, tk.ID).Scan(&comments))
			require.Equal(t, 1, changed, "the status transition must have its durable event")
			require.Equal(t, 1, commented, "the persisted public comment must have its own event")
			require.Equal(t, 1, comments)
		})
	}
}

func TestF5LifecycleRepair_EventFailureRollsBackWholeCommand(t *testing.T) {
	for _, operation := range []string{"transition", "close"} {
		t.Run(operation, func(t *testing.T) {
			svc, actor, assignee, org, code := lifecycleRepairSetup(t)
			ctx := context.Background()
			tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "合成事件失败回滚", OrgID: org}, actor)
			require.NoError(t, err)
			if operation == "transition" {
				require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &assignee}, actor))
			}
			snapshot := func() (string, int, int) {
				var row string
				var events, comments int
				require.NoError(t, testPool.QueryRow(ctx, `SELECT md5(to_jsonb(t)::text),(SELECT count(*) FROM ticket_events WHERE ticket_id=t.id),(SELECT count(*) FROM ticket_comments WHERE ticket_id=t.id) FROM tickets t WHERE id=$1`, tk.ID).Scan(&row, &events, &comments))
				return row, events, comments
			}
			before, events, comments := snapshot()
			name := "repair_lifecycle_" + uniqueSuffix()
			_, err = testPool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic lifecycle event failure'; END; $$;
CREATE TRIGGER %s BEFORE INSERT ON ticket_events FOR EACH ROW WHEN (NEW.ticket_id=%d AND NEW.action='comment') EXECUTE FUNCTION %s()`, name, name, tk.ID, name))
			require.NoError(t, err)
			defer func() {
				_, err := testPool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER %s ON ticket_events; DROP FUNCTION %s()`, name, name))
				require.NoError(t, err)
			}()
			if operation == "transition" {
				_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: StatusInProgress, Comment: "失败不落任何部分"}, assignee)
			} else {
				err = svc.Close(ctx, &model.CloseTicketRequest{ID: tk.ID, Comment: "失败不落任何部分"}, actor)
			}
			require.Error(t, err)
			after, afterEvents, afterComments := snapshot()
			require.Equal(t, before, after, "status, deadline and timestamps must roll back together")
			require.Equal(t, events, afterEvents)
			require.Equal(t, comments, afterComments)
		})
	}
}
