//go:build integration

package ticket

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
)

// A competing commit that wins the ticket lock must be visible to the later
// Update result and public-comment event, even after their admission reads.
func TestCommitSnapshot_PrecedingWriter(t *testing.T) {
	for _, kind := range []string{"update", "comment"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			svc, creator, _, assignee, roles := setupTicket2a(t)
			roles[creator] = []string{"admin"}
			tk := newTicketHelper(t, svc, creator, childOrgID(t, "snapshot"), "before competing commit")
			_, err := testPool.Exec(ctx, `UPDATE users SET real_name='snapshot creator' WHERE id=$1`, creator)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `UPDATE users SET real_name='current handler', employee_no=NULL WHERE id=$1`, assignee)
			require.NoError(t, err)
			reg, events, release := captureCommitSnapshot(t, svc)
			defer release()

			prior, err := testPool.Begin(ctx)
			require.NoError(t, err)
			defer prior.Rollback(context.Background())
			var priorPID int
			require.NoError(t, prior.QueryRow(ctx, `SELECT pg_backend_pid() FROM tickets WHERE id=$1 FOR UPDATE`, tk.ID).Scan(&priorPID))
			type result struct {
				ticket *model.Ticket
				err    error
			}
			done := make(chan result, 1)
			go func() {
				if kind == "update" {
					priority := 1
					got, err := svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Priority: &priority}, creator)
					done <- result{got, err}
				} else {
					_, err := svc.CreateComment(ctx, &model.CreateCommentRequest{TicketID: tk.ID, Content: "synthetic public reply"}, creator)
					done <- result{err: err}
				}
			}()
			waitSnapshotBlocker(t, ctx, priorPID)
			_, err = prior.Exec(ctx, `UPDATE tickets SET title='preceding commit', assigned_to=$2,
				status='assigned', priority=2, description=NULL, human_key=NULL,
				custom_data='{"historical":"kept"}'::jsonb WHERE id=$1`, tk.ID, assignee)
			require.NoError(t, err)
			require.NoError(t, prior.Commit(ctx))
			got := <-done
			require.NoError(t, got.err)
			priority := 2
			if kind == "update" {
				priority = 1
				require.Equal(t, "preceding commit", got.ticket.Title)
				require.Equal(t, "assigned", got.ticket.Status)
				require.Equal(t, &assignee, got.ticket.AssignedTo)
				require.JSONEq(t, `{"historical":"kept"}`, string(got.ticket.CustomData))
				require.Empty(t, got.ticket.Description, "nullable description keeps its API default")
				require.Equal(t, "snapshot creator", got.ticket.CreatedByName)
				require.NotEmpty(t, got.ticket.CreatedByNo)
				require.Equal(t, "current handler", got.ticket.AssigneeName)
				require.Empty(t, got.ticket.AssigneeNo)
				require.Equal(t, priority, got.ticket.Priority)
			}
			release()
			require.NoError(t, reg.Drain(ctx))
			ev := <-events
			require.Equal(t, "preceding commit", ev.Payload["title"])
			require.Equal(t, "assigned", ev.Payload["status"])
			require.Equal(t, priority, ev.Payload["priority"])
			require.Equal(t, "", ev.Payload["human_key"], "legacy empty key remains an explicit event value")
			require.Equal(t, strconv.FormatInt(assignee, 10), ev.Payload["to_assignee"])
			require.Equal(t, strconv.FormatInt(creator, 10), ev.Payload["creator"])
			require.NotContains(t, ev.Payload, "content")
		})
	}
}

// Gate the audit insert after the operation has captured its snapshot. A later
// writer must wait for its commit; queued events must survive that later write
// and mutations to the returned object/request without acquiring a new version.
func TestCommitSnapshot_FollowingWriter(t *testing.T) {
	for _, kind := range []string{"update", "comment"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			svc, creator, assignee, nextAssignee, roles := setupTicket2a(t)
			roles[creator] = []string{"admin"}
			tk := newTicketHelper(t, svc, creator, childOrgID(t, "snapshot_next"), "operation commit")
			_, err := testPool.Exec(ctx, `UPDATE tickets SET assigned_to=$2 WHERE id=$1`, tk.ID, assignee)
			require.NoError(t, err)
			reg, events, release := captureCommitSnapshot(t, svc)
			defer release()
			gate, err := testPool.Begin(ctx)
			require.NoError(t, err)
			defer gate.Rollback(context.Background())
			var gatePID int
			require.NoError(t, gate.QueryRow(ctx, `SELECT pg_backend_pid(), pg_advisory_xact_lock($1)`, tk.ID).Scan(&gatePID, new(any)))
			installSnapshotGate(t, ctx, tk.ID, kind)
			type result struct {
				ticket *model.Ticket
				err    error
			}
			done := make(chan result, 1)
			title := "operation commit"
			req := &model.UpdateTicketRequest{ID: tk.ID, Title: &title}
			go func() {
				if kind == "update" {
					got, err := svc.Update(ctx, req, creator)
					done <- result{got, err}
				} else {
					_, err := svc.CreateComment(ctx, &model.CreateCommentRequest{TicketID: tk.ID, Content: "synthetic public reply"}, creator)
					done <- result{err: err}
				}
			}()
			waitSnapshotBlocker(t, ctx, gatePID)
			var operationPID int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)) LIMIT 1`, gatePID).Scan(&operationPID))
			next := make(chan error, 1)
			go func() {
				_, err := testPool.Exec(ctx, `UPDATE tickets SET title='following commit', status='assigned', assigned_to=$2 WHERE id=$1`, tk.ID, nextAssignee)
				next <- err
			}()
			waitSnapshotBlocker(t, ctx, operationPID)
			require.NoError(t, gate.Commit(ctx))
			got := <-done
			require.NoError(t, got.err)
			require.NoError(t, <-next)
			if kind == "update" {
				require.Equal(t, "operation commit", got.ticket.Title)
				require.Equal(t, &assignee, got.ticket.AssignedTo)
				require.Equal(t, "open", got.ticket.Status)
				got.ticket.Title = "mutated response"
				*got.ticket.AssignedTo = nextAssignee
			}
			title = "mutated request"
			release()
			require.NoError(t, reg.Drain(ctx))
			ev := <-events
			require.Equal(t, "operation commit", ev.Payload["title"])
			require.Equal(t, "open", ev.Payload["status"])
			require.Equal(t, strconv.FormatInt(assignee, 10), ev.Payload["to_assignee"])
			if kind == "update" {
				require.Equal(t, []domainevent.Change{{Key: "title", NewVal: "operation commit"}}, ev.Payload["changes"])
			}
		})
	}
}

func captureCommitSnapshot(t *testing.T, svc *Service) (*domainevent.HookRegistry, <-chan domainevent.DomainEvent, func()) {
	t.Helper()
	reg := domainevent.NewHookRegistry(nil)
	events := make(chan domainevent.DomainEvent, 2)
	released := make(chan struct{})
	reg.Subscribe(domainevent.BizTicket, "commit.snapshot", domainevent.HookFunc(func(_ context.Context, ev domainevent.DomainEvent) {
		<-released
		events <- ev
	}))
	svc.SetEventRegistry(reg)
	// Calls are sequential, including cleanup after a failed assertion.
	closed := false
	release := func() {
		if !closed {
			close(released)
			closed = true
		}
	}
	t.Cleanup(func() {
		release()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		require.NoError(t, reg.Drain(ctx))
	})
	return reg, events, release
}

func waitSnapshotBlocker(t *testing.T, ctx context.Context, blockerPID int) {
	t.Helper()
	require.Eventually(t, func() bool {
		var blocked bool
		err := testPool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, blockerPID).Scan(&blocked)
		return err == nil && blocked
	}, 3*time.Second, 5*time.Millisecond, "operation must reach the controlled lock window")
}

func installSnapshotGate(t *testing.T, ctx context.Context, ticketID int64, action string) {
	t.Helper()
	if action == "update" {
		action = "updated"
	}
	name := "snapshot_gate_" + uniqueSuffix()
	_, err := testPool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN PERFORM pg_advisory_xact_lock(%d); RETURN NEW; END $$`, name, ticketID))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := testPool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON ticket_events; DROP FUNCTION %s()`, name, name))
		require.NoError(t, err)
	})
	_, err = testPool.Exec(ctx, fmt.Sprintf(`CREATE TRIGGER %s AFTER INSERT ON ticket_events FOR EACH ROW
		WHEN (NEW.ticket_id=%d AND NEW.action='%s') EXECUTE FUNCTION %s()`, name, ticketID, action, name))
	require.NoError(t, err)
}

func TestCommitSnapshot_UpdateCommitFailure(t *testing.T) {
	ctx := context.Background()
	svc, creator, _, _, roles := setupTicket2a(t)
	roles[creator] = []string{"admin"}
	tk := newTicketHelper(t, svc, creator, childOrgID(t, "snapshot_fail"), "before failed commit")
	reg, events, release := captureCommitSnapshot(t, svc)
	defer release()
	name := "snapshot_fail_" + uniqueSuffix()
	_, err := testPool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'synthetic deferred update failure'; END $$`, name))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := testPool.Exec(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON ticket_events; DROP FUNCTION %s()`, name, name))
		require.NoError(t, err)
	})
	_, err = testPool.Exec(ctx, fmt.Sprintf(`CREATE CONSTRAINT TRIGGER %s AFTER INSERT ON ticket_events
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (NEW.ticket_id=%d AND NEW.action='updated')
		EXECUTE FUNCTION %s()`, name, tk.ID, name))
	require.NoError(t, err)
	title := "must roll back"
	got, err := svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: &title}, creator)
	require.Error(t, err)
	require.Nil(t, got)
	release()
	require.NoError(t, reg.Drain(ctx))
	require.Empty(t, events, "a failed commit must not publish an update")
	var storedTitle string
	var audits int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT title FROM tickets WHERE id=$1`, tk.ID).Scan(&storedTitle))
	require.Equal(t, "before failed commit", storedTitle)
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_events WHERE ticket_id=$1 AND action='updated'`, tk.ID).Scan(&audits))
	require.Zero(t, audits)
}

func TestCommitSnapshot_LegacyDefaultsAndClosedComment(t *testing.T) {
	ctx := context.Background()
	svc, creator, _, _, roles := setupTicket2a(t)
	roles[creator] = []string{"admin"}
	tk := newTicketHelper(t, svc, creator, childOrgID(t, "snapshot_legacy"), "legacy defaults")
	_, err := testPool.Exec(ctx, `UPDATE tickets SET description=NULL, custom_data=NULL, human_key=NULL WHERE id=$1`, tk.ID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE users SET real_name='' WHERE id=$1`, creator)
	require.NoError(t, err)
	var username, employeeNo string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT username, employee_no FROM users WHERE id=$1`, creator).Scan(&username, &employeeNo))
	reg, events, release := captureCommitSnapshot(t, svc)
	defer release()
	got, err := svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID}, creator)
	require.NoError(t, err)
	require.Empty(t, got.Description)
	require.Nil(t, got.CustomData)
	require.Nil(t, got.AssignedTo)
	require.Empty(t, got.AssigneeName)
	require.Empty(t, got.AssigneeNo)
	require.Equal(t, username, got.CreatedByName, "empty real name falls back to username")
	require.Equal(t, employeeNo, got.CreatedByNo)
	_, err = testPool.Exec(ctx, `UPDATE tickets SET status='closed' WHERE id=$1`, tk.ID)
	require.NoError(t, err)
	comment, err := svc.CreateComment(ctx, &model.CreateCommentRequest{TicketID: tk.ID, Content: "reply to closed ticket"}, creator)
	require.NoError(t, err, "snapshot locking must preserve closed-ticket public replies")
	require.False(t, comment.IsInternal)
	release()
	require.NoError(t, reg.Drain(ctx))
	require.Len(t, events, 2)
	for i := 0; i < 2; i++ {
		ev := <-events
		require.NotContains(t, ev.Payload, "to_assignee")
		require.Equal(t, "", ev.Payload["human_key"])
		if ev.Action == domainevent.TicketCommented {
			require.Equal(t, "closed", ev.Payload["status"])
		}
	}
}
