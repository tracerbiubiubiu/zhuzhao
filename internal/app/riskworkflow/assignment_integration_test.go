//go:build integration

package riskworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskreminder"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

type assignmentFixture struct {
	*writeFixture
	manager int64
	service *AssignmentService
	now     time.Time
}

func newAssignmentFixture(t *testing.T, condition string) *assignmentFixture {
	t.Helper()
	f := newPreparedReceiptFixture(t, func(f *receiptFixture, code string) {
		_, err := f.tickets.UpdateTicketType(f.ctx, code, &model.UpdateTicketTypeRequest{Code: code,
			Transitions:    json.RawMessage(`{"open":["assigned"],"assigned":["in_progress"],"in_progress":["closed"],"closed":[]}`),
			TransitionMeta: json.RawMessage(`{"assigned":{"first_accept_reminder":{"rule_id":"synthetic-explicit","rule_version":1,"accept_within_minutes":30}},"assigned>in_progress":{"roles":["` + condition + `"],"comment_required":true}}`),
		})
		require.NoError(t, err)
	}, func(f *receiptFixture, tk *model.Ticket) {
		_, err := workflowPool.Exec(f.ctx, `UPDATE user_orgs SET ticket_scope='group' WHERE user_id=$1`, f.actor)
		require.NoError(t, err)
		require.NoError(t, f.tickets.Assign(f.ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &f.actor}, f.actor))
	})
	_, err := workflowPool.Exec(f.ctx, `UPDATE risk_items SET owner_user_id=$1 WHERE id=ANY($2)`, f.actor, []int64{f.riskA, f.riskB})
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `UPDATE risk_ticket_links SET owner_at_link=$1 WHERE risk_id=ANY($2)`, f.actor, []int64{f.riskA, f.riskB})
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `UPDATE risk_dispatch_groups SET owner_user_id=$1 WHERE ticket_id=$2`, f.actor, f.ticket)
	require.NoError(t, err)
	var manager int64
	require.NoError(t, workflowPool.QueryRow(f.ctx, `INSERT INTO users(username,password,status) VALUES($1,'synthetic',1) RETURNING id`, "m-"+f.role).Scan(&manager))
	for _, id := range []int64{manager, f.other} {
		_, err = workflowPool.Exec(f.ctx, `INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE code=$2`, id, f.role)
		require.NoError(t, err)
		role := "member"
		if id == manager {
			role = "admin"
		}
		_, err = workflowPool.Exec(f.ctx, `INSERT INTO user_orgs(user_id,org_id,org_member_role,ticket_scope) VALUES($1,$2,$4,'group'),($1,$3,$4,'group')`, id, f.orgA, f.orgB, role)
		require.NoError(t, err)
	}
	for _, path := range []string{assignHandlerPath, acceptAssignmentPath, "/api/v1/tickets/assign", "/api/v1/tickets/transition"} {
		_, err = workflowPool.Exec(f.ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,'POST')`, "role::"+f.role, path)
		require.NoError(t, err)
		_, err = f.enforcer.AddPolicy("role::"+f.role, path, "POST")
		require.NoError(t, err)
	}
	tk, err := repository.NewTicketRepo(workflowPool).GetByID(f.ctx, f.ticket)
	require.NoError(t, err)
	a := &assignmentFixture{writeFixture: &writeFixture{receiptFixture: f, version: *tk.DefinitionVersion}, manager: manager, now: time.Date(2026, 11, 22, 9, 5, 0, 0, time.UTC)}
	recorder := riskreminder.NewService(workflowPool, nil, nil, nil)
	a.service = NewAssignmentService(workflowPool, f.tickets, AssignmentDependencies{
		Clock:            func() time.Time { return a.now },
		RecordAssignment: recorder.RecordConfiguredAssignment,
		RecordAcceptance: recorder.RecordConfiguredAcceptance,
	})
	return a
}

func (f *assignmentFixture) command(t *testing.T, actor int64, op string) AssignmentCommand {
	tk, err := repository.NewTicketRepo(workflowPool).GetByID(f.ctx, f.ticket)
	require.NoError(t, err)
	return AssignmentCommand{ActorID: actor, OperationID: op + f.role, TicketID: f.ticket, ExpectedTicketRevision: tk.Revision, TicketInput: ticket.RiskWorkflowTicketInput{DefinitionVersion: f.version, Comment: "Synthetic personal acceptance"}}
}
func (f *assignmentFixture) invoke(t *testing.T, c AssignmentCommand, target int64, before func()) (AssignmentResult, error) {
	path := acceptAssignmentPath
	if target > 0 {
		path = assignHandlerPath
	}
	var out AssignmentResult
	_, err := f.admittedWrite(t, c.ActorID, path, before, func(ctx context.Context) (riskremediation.Result, error) {
		var e error
		if target > 0 {
			out, e = f.service.AssignHandler(ctx, AssignHandlerCommand{AssignmentCommand: c, AssignedToUserID: target})
		} else {
			out, e = f.service.AcceptAssignment(ctx, c)
		}
		return riskremediation.Result{}, e
	})
	return out, err
}
func (f *assignmentFixture) snapshot(t *testing.T) string {
	var facts string
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT jsonb_build_object('assignments',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM riskreminder_assignments a WHERE ticket_id=$1),'intents',(SELECT jsonb_agg(to_jsonb(i) ORDER BY id) FROM riskreminder_intents i WHERE ticket_id=$1),'operations',(SELECT jsonb_agg(to_jsonb(o) ORDER BY actor_id,operation_id) FROM risk_operations o WHERE actor_id=ANY($2)))::text`, f.ticket, []int64{f.actor, f.other, f.manager}).Scan(&facts))
	return f.writeSnapshot(t) + facts
}

func TestAssignmentHandlerAndPersonalAcceptance(t *testing.T) {
	f := newAssignmentFixture(t, "assignee")
	assign := f.command(t, f.manager, "assign-")
	got, err := f.invoke(t, assign, f.other, nil)
	require.NoError(t, err)
	require.Equal(t, f.other, got.AssignedToUserID)
	require.EqualValues(t, 1, got.Generation)
	require.Equal(t, "assigned", got.Status)
	// Existing operation recovery must retain original ticket authorization.
	readActor := f.actor
	f.actor = f.manager
	recovered, e := f.read(t, assign.OperationID, nil)
	require.NoError(t, e)
	f.actor = readActor
	require.Equal(t, f.ticket, recovered.TicketID)
	require.Equal(t, got.Items, recovered.Items)
	before := f.snapshot(t)
	f.now = f.now.Add(time.Minute)
	replay, err := f.invoke(t, assign, f.other, nil)
	require.NoError(t, err)
	require.Equal(t, got, replay)
	require.Equal(t, before, f.snapshot(t))
	var owner, handler int64
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT r.owner_user_id,t.assigned_to FROM risk_items r CROSS JOIN tickets t WHERE r.id=$1 AND t.id=$2`, f.riskA, f.ticket).Scan(&owner, &handler))
	require.Equal(t, f.actor, owner)
	require.Equal(t, f.other, handler)
	for _, actor := range []int64{f.actor, f.manager} {
		c := f.command(t, actor, "impersonate-")
		out, e := f.invoke(t, c, 0, nil)
		require.Error(t, e)
		require.Empty(t, out)
		require.Equal(t, before, f.snapshot(t))
	}
	accept := f.command(t, f.other, "accept-")
	f.now = f.now.Add(34 * time.Minute)
	accepted, err := f.invoke(t, accept, 0, nil)
	require.NoError(t, err)
	require.Equal(t, "in_progress", accepted.Status)
	before = f.snapshot(t)
	f.now = f.now.Add(time.Hour)
	replay, err = f.invoke(t, accept, 0, nil)
	require.NoError(t, err)
	require.Equal(t, accepted, replay)
	require.Equal(t, before, f.snapshot(t))
	var at time.Time
	var by int64
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT accepted_at,accepted_by FROM riskreminder_assignments WHERE ticket_id=$1 AND generation=1`, f.ticket).Scan(&at, &by))
	require.Equal(t, f.other, by)
	require.Equal(t, time.Date(2026, 11, 22, 9, 40, 0, 0, time.UTC), at.UTC())
	changed := assign
	changed.ExpectedTicketRevision++
	_, err = f.invoke(t, changed, f.other, nil)
	require.Error(t, err)
	require.Equal(t, before, f.snapshot(t))
}

func TestAssignmentFailuresRollBack(t *testing.T) {
	for _, kind := range []string{"owner cannot assign", "target disabled", "target policy revoked", "target scope expired", "manager revoked", "stale revision", "policy failure", "clock unavailable", "published condition", "missing acceptance fact", "accept SQL failure", "actor epoch revoked", "accept policy revoked"} {
		t.Run(kind, func(t *testing.T) {
			condition := "assignee"
			if kind == "published condition" {
				condition = "creator"
			}
			f := newAssignmentFixture(t, condition)
			c := f.command(t, f.manager, "assign-")
			var before func()
			switch kind {
			case "owner cannot assign":
				c.ActorID = f.actor
			case "target disabled":
				_, e := workflowPool.Exec(f.ctx, `UPDATE users SET status=0 WHERE id=$1`, f.other)
				require.NoError(t, e)
			case "target policy revoked":
				_, e := workflowPool.Exec(f.ctx, `DELETE FROM user_roles WHERE user_id=$1`, f.other)
				require.NoError(t, e)
			case "target scope expired":
				_, e := workflowPool.Exec(f.ctx, `UPDATE user_orgs SET expires_at=now()-interval '1 minute' WHERE user_id=$1`, f.other)
				require.NoError(t, e)
			case "manager revoked":
				before = func() {
					_, e := workflowPool.Exec(f.ctx, `UPDATE user_orgs SET org_member_role='member',ticket_scope='assigned' WHERE user_id=$1`, f.manager)
					require.NoError(t, e)
				}
			case "stale revision":
				c.ExpectedTicketRevision++
			case "policy failure":
				f.service.deps.RecordAssignment = func(context.Context, pgx.Tx, int64, int64, int64, time.Time) (*riskreminder.Assignment, bool, error) {
					return nil, false, errors.New("synthetic publication failure")
				}
			case "clock unavailable":
				f.service.deps.Clock = func() time.Time { return time.Time{} }
			}
			accepting := kind == "published condition" || kind == "missing acceptance fact" || kind == "accept SQL failure" || kind == "actor epoch revoked" || kind == "accept policy revoked"
			if accepting {
				_, e := f.invoke(t, c, f.other, nil)
				require.NoError(t, e)
				c = f.command(t, f.other, "accept-")
				switch kind {
				case "missing acceptance fact":
					_, e = workflowPool.Exec(f.ctx, `DELETE FROM riskreminder_assignments WHERE ticket_id=$1`, f.ticket)
					require.NoError(t, e)
				case "actor epoch revoked":
					before = func() {
						_, e := workflowPool.Exec(f.ctx, `UPDATE users SET session_epoch=session_epoch+1 WHERE id=$1`, f.other)
						require.NoError(t, e)
					}
				case "accept policy revoked":
					before = func() {
						_, e := workflowPool.Exec(f.ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1='/api/v1/tickets/transition'`, "role::"+f.role)
						require.NoError(t, e)
					}
				case "accept SQL failure":
					fn := "assignment_fail_" + fmt.Sprint(f.ticket)
					_, e = workflowPool.Exec(f.ctx, `CREATE FUNCTION `+fn+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic failure'; END $$; CREATE TRIGGER `+fn+` BEFORE UPDATE ON riskreminder_assignments FOR EACH ROW WHEN (OLD.ticket_id=`+fmt.Sprint(f.ticket)+`) EXECUTE FUNCTION `+fn+`()`)
					require.NoError(t, e)
					t.Cleanup(func() { _, _ = workflowPool.Exec(context.Background(), `DROP FUNCTION `+fn+`() CASCADE`) })
				}
			}
			snap := f.snapshot(t)
			target := f.other
			if accepting {
				target = 0
			}
			out, e := f.invoke(t, c, target, before)
			require.Error(t, e)
			require.Empty(t, out)
			require.Equal(t, snap, f.snapshot(t))
		})
	}
}

func TestAssignmentNestedCallbackFailureCannotEscape(t *testing.T) {
	f := newAssignmentFixture(t, "assignee")
	c := f.command(t, f.manager, "nested-")
	stop := errors.New("synthetic callback failure")
	f.service.deps.RecordAssignment = func(ctx context.Context, tx pgx.Tx, _, _, _ int64, _ time.Time) (*riskreminder.Assignment, bool, error) {
		_, err := tx.Exec(ctx, `INSERT INTO risk_operations(actor_id,operation_id,request_hash,command,result_status) VALUES($1,$2,decode(repeat('ab',32),'hex'),'synthetic','completed')`, f.manager, "callback-"+f.role)
		if err != nil {
			return nil, false, err
		}
		return nil, false, stop
	}
	before := f.snapshot(t)
	_, err := f.admittedWrite(t, c.ActorID, assignHandlerPath, nil, func(ctx context.Context) (riskremediation.Result, error) {
		err := repository.WithIAMCommandGuard(ctx, workflowPool, func(ctx context.Context, outer pgx.Tx) error {
			_, e := f.service.AssignHandler(ctx, AssignHandlerCommand{AssignmentCommand: c, AssignedToUserID: f.other})
			require.ErrorIs(t, e, stop)
			var one int
			require.NoError(t, outer.QueryRow(ctx, `SELECT 1`).Scan(&one))
			return nil // Parent handles the failed operation and commits unrelated work.
		})
		return riskremediation.Result{}, err
	})
	require.NoError(t, err)
	require.Equal(t, before, f.snapshot(t))
}

func TestAssignmentAcceptMaximumOperationID(t *testing.T) {
	f := newAssignmentFixture(t, "assignee")
	_, err := f.invoke(t, f.command(t, f.manager, "assign-"), f.other, nil)
	require.NoError(t, err)
	c := f.command(t, f.other, "accept-")
	c.OperationID = strings.Repeat("k", 128)
	_, err = f.invoke(t, c, 0, nil)
	require.NoError(t, err)
}

func TestAssignmentConcurrentOriginalOperations(t *testing.T) {
	f := newAssignmentFixture(t, "assignee")
	for _, accepting := range []bool{false, true} {
		actor, path := f.manager, assignHandlerPath
		if accepting {
			actor, path = f.other, acceptAssignmentPath
		}
		c := f.command(t, actor, fmt.Sprint("concurrent-", accepting))
		_, err := f.admittedWrite(t, actor, path, nil, func(ctx context.Context) (riskremediation.Result, error) {
			type outcome struct {
				result AssignmentResult
				err    error
			}
			done := make(chan outcome, 2)
			for range 2 {
				go func() {
					var out AssignmentResult
					var e error
					if accepting {
						out, e = f.service.AcceptAssignment(ctx, c)
					} else {
						out, e = f.service.AssignHandler(ctx, AssignHandlerCommand{AssignmentCommand: c, AssignedToUserID: f.other})
					}
					done <- outcome{out, e}
				}()
			}
			first, second := <-done, <-done
			require.NoError(t, first.err)
			require.NoError(t, second.err)
			require.Equal(t, first.result, second.result)
			return riskremediation.Result{}, nil
		})
		require.NoError(t, err)
	}
	var facts, receipts, assignEvents int
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT
  (SELECT count(*) FROM riskreminder_assignments WHERE ticket_id=$1),
  (SELECT count(*) FROM risk_remediation_receipts WHERE ticket_id=$1 AND actor_id=ANY($2)),
  (SELECT count(*) FROM ticket_events WHERE ticket_id=$1 AND action='assigned' AND user_id=$3)`, f.ticket, []int64{f.manager, f.other}, f.manager).Scan(&facts, &receipts, &assignEvents))
	require.Equal(t, 1, facts)
	require.Equal(t, 2, receipts)
	require.Equal(t, 1, assignEvents)
}

func TestAssignmentReminderSuppressionAndHistory(t *testing.T) {
	f := newAssignmentFixture(t, "assignee")
	first, err := f.invoke(t, f.command(t, f.manager, "first-"), f.other, nil)
	require.NoError(t, err)
	seed := func(generation int64, delivered bool) int64 {
		var id int64
		require.NoError(t, workflowPool.QueryRow(f.ctx, `INSERT INTO riskreminder_intents(assignment_id,ticket_id,generation,recipient_id,due_at,source,source_event_id,rule_id,rule_version,channel,semantic_key,status,notification_id,delivered_at)
   SELECT id,ticket_id,generation,assignee_id,accept_due_at,'synthetic',$3,rule_id,rule_version,'inbox','accept',CASE WHEN $4 THEN 'delivered' ELSE 'pending' END,CASE WHEN $4 THEN 1 ELSE NULL END,CASE WHEN $4 THEN now() ELSE NULL END
   FROM riskreminder_assignments WHERE ticket_id=$1 AND generation=$2 RETURNING id`, f.ticket, generation, fmt.Sprint(f.role, generation, delivered), delivered).Scan(&id))
		return id
	}
	oldPending, delivered := seed(first.Generation, false), seed(first.Generation, true)
	var p2 int64
	require.NoError(t, workflowPool.QueryRow(f.ctx, `INSERT INTO users(username,password,status) VALUES($1,'synthetic',1) RETURNING id`, "p2-"+f.role).Scan(&p2))
	_, err = workflowPool.Exec(f.ctx, `INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE code=$2`, p2, f.role)
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `INSERT INTO user_orgs(user_id,org_id,ticket_scope) VALUES($1,$2,'group'),($1,$3,'group')`, p2, f.orgA, f.orgB)
	require.NoError(t, err)
	f.now = f.now.Add(time.Minute)
	second, err := f.invoke(t, f.command(t, f.manager, "second-"), p2, nil)
	require.NoError(t, err)
	require.Equal(t, first.Generation+1, second.Generation)
	newPending := seed(second.Generation, false)
	f.now = f.now.Add(35 * time.Minute)
	_, err = f.invoke(t, f.command(t, p2, "accept-p2-"), 0, nil)
	require.NoError(t, err)
	for _, check := range []struct {
		id             int64
		status, reason string
	}{{oldPending, "suppressed", "reassigned"}, {delivered, "delivered", ""}, {newPending, "suppressed", "accepted"}} {
		var status, reason string
		require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT status,COALESCE(suppress_reason,'') FROM riskreminder_intents WHERE id=$1`, check.id).Scan(&status, &reason))
		require.Equal(t, check.status, status)
		require.Equal(t, check.reason, reason)
	}
	var oldAccepted *time.Time
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT accepted_at FROM riskreminder_assignments WHERE ticket_id=$1 AND generation=$2`, f.ticket, first.Generation).Scan(&oldAccepted))
	require.Nil(t, oldAccepted)
}
