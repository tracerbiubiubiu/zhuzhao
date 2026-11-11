//go:build integration

package ticket

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"testing"
	"time"
)

type auditCaptureEvent chan domainevent.DomainEvent

// Every scanner reads all active types. Retire this fixture before the next test
// so an intentionally unspent reminder cannot become another test's notification.
func auditRetireReminderType(t *testing.T, code string) {
	t.Helper()
	t.Cleanup(func() {
		_, err := testPool.Exec(context.Background(), `UPDATE ticket_types SET is_active=false WHERE code=$1`, code)
		require.NoError(t, err)
	})
}

func auditReminderRegistry(t *testing.T) *domainevent.HookRegistry {
	t.Helper()
	reg := domainevent.NewHookRegistry(nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		require.NoError(t, reg.Drain(ctx))
	})
	return reg
}

func TestAuditTimeoutWatermarkWithoutEvent(t *testing.T) {
	svc, aid, bid, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	ctx := context.Background()
	oid := childOrgID(t, "remind")
	code := "remind_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	auditRetireReminderType(t, code)
	meta := `{"assigned":{"timeout_minutes":1,"escalation":{"notify":["supervisor"],"max_reminders":1}}}`
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: []byte(meta)})
	require.NoError(t, err)
	tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, Priority: 3, OrgID: oid}, aid)
	require.NoError(t, err)
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid))
	backdateStatusChanged(t, tk.ID, 2*time.Hour)
	reg := auditReminderRegistry(t)
	scanner := NewTimeoutScanner(testPool, reg, false, nil)
	require.NoError(t, scanner.Handle(ctx, nil))
	var count int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT coalesce((SELECT reminder_count FROM ticket_reminders WHERE ticket_id=$1 AND state='assigned'),0)`, tk.ID).Scan(&count))
	t.Logf("no supervisor, no event, reminder_count=%d", count)
	require.Zero(t, count, "no target is not a reminder and must not consume its budget")
}

func TestAuditTimeoutZeroLimit(t *testing.T) {
	svc, aid, bid, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	ctx := context.Background()
	oid := childOrgID(t, "zero")
	code := "zero_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	auditRetireReminderType(t, code)
	meta := `{"assigned":{"timeout_minutes":1,"escalation":{"notify":["assignee"],"max_reminders":0}}}`
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: []byte(meta)})
	require.NoError(t, err)
	tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, Priority: 3, OrgID: oid}, aid)
	require.NoError(t, err)
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid))
	backdateStatusChanged(t, tk.ID, 2*time.Hour)
	reg := auditReminderRegistry(t)
	scanner := NewTimeoutScanner(testPool, reg, false, nil)
	require.NoError(t, scanner.Handle(ctx, nil))
	var count int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT coalesce((SELECT reminder_count FROM ticket_reminders WHERE ticket_id=$1 AND state='assigned'),0)`, tk.ID).Scan(&count))
	t.Logf("max_reminders=0 actual_count=%d", count)
	require.Zero(t, count, fmt.Sprintf("configured zero cap must hold; got %d", count))
}

func (h auditCaptureEvent) OnDomainEvent(_ context.Context, ev domainevent.DomainEvent) { h <- ev }

func TestAuditReminderTargetsArriveLater(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprintf("deadline=%v", deadline), func(t *testing.T) {
			svc, aid, bid, vid, roles := setupTicket2a(t)
			roles[aid] = []string{"admin"}
			ctx := context.Background()
			oid := childOrgID(t, "later")
			code := "later_" + uniqueSuffix()
			_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
			require.NoError(t, err)
			auditRetireReminderType(t, code)
			meta := `{"assigned":{"timeout_minutes":1,"escalation":{"notify":["supervisor"],"max_reminders":1}}}`
			if deadline {
				meta = `{"assigned":{"sla_due_hours":1}}`
			}
			_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: []byte(meta)})
			require.NoError(t, err)
			tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, Priority: 3, OrgID: oid}, aid)
			require.NoError(t, err)
			require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid))
			backdateStatusChanged(t, tk.ID, 2*time.Hour)
			if deadline {
				_, err = testPool.Exec(ctx, `UPDATE tickets SET sla_due_at=NOW()-interval '1 hour',assigned_to=NULL WHERE id=$1`, tk.ID)
				require.NoError(t, err)
			}
			reg := auditReminderRegistry(t)
			events := make(auditCaptureEvent, 4)
			reg.Subscribe(domainevent.BizTicket, "audit.later", events)
			scanner := NewTimeoutScanner(testPool, reg, false, nil)
			require.NoError(t, scanner.Handle(ctx, nil))
			var count int
			state := "assigned"
			if deadline {
				state = "deadline:assigned"
			}
			require.NoError(t, testPool.QueryRow(ctx, `SELECT coalesce((SELECT reminder_count FROM ticket_reminders WHERE ticket_id=$1 AND state=$2),0)`, tk.ID, state).Scan(&count))
			require.Zero(t, count)
			// Same-state assignment may set the missing supervisor; it does not clear
			// reminder watermarks, so the first scan must not have spent the budget.
			require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid, EscalationSupervisor: &vid}, aid))
			require.NoError(t, scanner.Handle(ctx, nil))
			select {
			case event := <-events:
				require.Contains(t, fmt.Sprint(event.Payload["cc_users"]), fmt.Sprint(vid))
			case <-time.After(time.Second):
				t.Fatal("late target never received event")
			}
			require.NoError(t, testPool.QueryRow(ctx, `SELECT reminder_count FROM ticket_reminders WHERE ticket_id=$1 AND state=$2`, tk.ID, state).Scan(&count))
			require.Equal(t, 1, count)
			require.NoError(t, scanner.Handle(ctx, nil))
			require.NoError(t, testPool.QueryRow(ctx, `SELECT reminder_count FROM ticket_reminders WHERE ticket_id=$1 AND state=$2`, tk.ID, state).Scan(&count))
			require.Equal(t, 1, count)
		})
	}
}

func TestAuditReminderCandidateWindowSkipsMissingTargets(t *testing.T) {
	svc, aid, bid, vid, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	ctx := context.Background()
	oid := childOrgID(t, "window")
	code := "window_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	auditRetireReminderType(t, code)
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: []byte(`{"assigned":{"timeout_minutes":1,"escalation":{"notify":["supervisor"],"max_reminders":1}}}`)})
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO tickets(type_code,title,status,created_by,assigned_to,org_id,org_path,status_changed_at) SELECT $1,'no target','assigned',$2,$3,$4,(SELECT path FROM organizations WHERE id=$4),NOW()-interval '2 hours' FROM generate_series(1,500)`, code, aid, bid, oid)
	require.NoError(t, err)
	var id int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO tickets(type_code,title,status,created_by,assigned_to,org_id,org_path,status_changed_at,escalation_supervisor_id) VALUES($1,'has target','assigned',$2,$3,$4,(SELECT path FROM organizations WHERE id=$4),NOW()-interval '2 hours',$5) RETURNING id`, code, aid, bid, oid, vid).Scan(&id))
	reg := auditReminderRegistry(t)
	events := make(auditCaptureEvent, 2)
	reg.Subscribe(domainevent.BizTicket, "audit.window", events)
	require.NoError(t, NewTimeoutScanner(testPool, reg, false, nil).Handle(ctx, nil))
	var n int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT coalesce((SELECT reminder_count FROM ticket_reminders WHERE ticket_id=$1 AND state='assigned'),0)`, id).Scan(&n))
	require.Equal(t, 1, n, "first 500 rows with no chosen target must not starve a deliverable reminder")
	select {
	case event := <-events:
		require.Equal(t, fmt.Sprint(id), event.BizID)
	case <-time.After(time.Second):
		t.Fatal("deliverable candidate starved")
	}
}

func TestAuditNoRegistryNoWatermark(t *testing.T) {
	svc, aid, bid, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	ctx := context.Background()
	oid := childOrgID(t, "noreg")
	code := "noreg_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	auditRetireReminderType(t, code)
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: []byte(`{"assigned":{"timeout_minutes":1,"sla_due_hours":1}}`)})
	require.NoError(t, err)
	tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, Priority: 3, OrgID: oid}, aid)
	require.NoError(t, err)
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid))
	backdateStatusChanged(t, tk.ID, 2*time.Hour)
	_, err = testPool.Exec(ctx, `UPDATE tickets SET sla_due_at=NOW()-interval '1 hour' WHERE id=$1`, tk.ID)
	require.NoError(t, err)
	require.NoError(t, NewTimeoutScanner(testPool, nil, false, nil).Handle(ctx, nil))
	var n int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_reminders WHERE ticket_id=$1`, tk.ID).Scan(&n))
	require.Zero(t, n, "without a dispatch registry neither timeout nor deadline was emitted")
}
