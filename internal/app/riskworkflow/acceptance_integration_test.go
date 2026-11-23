//go:build integration

package riskworkflow

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// Reuse the existing real publication and owning Finish. The creator is the
// independent primary-scope approver; the assignee is the current risk owner.
// No acceptance-specific publication, graph or permissive callback is installed.
func newAcceptanceFixture(t *testing.T) *writeFixture {
	return seedAcceptanceFixture(t, false)
}

func seedAcceptanceFixture(t *testing.T, reviewSecondAsCreator bool) *writeFixture {
	t.Helper()
	return prepareAcceptanceFixture(t, newWriteFixture(t), reviewSecondAsCreator)
}

func prepareAcceptanceFixture(t *testing.T, f *writeFixture, reviewSecondAsCreator bool) *writeFixture {
	t.Helper()
	_, err := workflowPool.Exec(f.ctx, `UPDATE risk_items SET owner_user_id=$1 WHERE id=ANY($2)`, f.other, []int64{f.riskA, f.riskB})
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `UPDATE risk_ticket_links SET owner_at_link=$1 WHERE risk_id=ANY($2)`, f.other, []int64{f.riskA, f.riskB})
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `UPDATE risk_dispatch_groups SET owner_user_id=$1 WHERE ticket_id=$2`, f.other, f.ticket)
	require.NoError(t, err)
	// Both have primary management qualification: self-approval must fail for
	// identity separation, not merely because the applicant lacks authority.
	_, err = workflowPool.Exec(f.ctx, `UPDATE user_orgs SET org_member_role='admin',ticket_scope='group' WHERE user_id=ANY($1)`, []int64{f.actor, f.other})
	require.NoError(t, err)
	for _, path := range []string{acceptRequestPath, acceptDecisionPath} {
		_, err = workflowPool.Exec(f.ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,'POST')`, "role::"+f.role, path)
		require.NoError(t, err)
		_, err = f.enforcer.AddPolicy("role::"+f.role, path, "POST")
		require.NoError(t, err)
	}
	if reviewSecondAsCreator {
		// Set B's current reviewer before submission freezes its owner.
		_, err = workflowPool.Exec(f.ctx, `UPDATE risk_items SET owner_user_id=$2 WHERE id=$1`, f.riskB, f.actor)
		require.NoError(t, err)
	}
	c := f.finishCommand(t)
	_, err = f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Finish(ctx, c) })
	require.NoError(t, err)
	return f
}

func (f *writeFixture) acceptanceRequest(t *testing.T, risk int64) AcceptanceRequestCommand {
	t.Helper()
	c := AcceptanceRequestCommand{AcceptanceRequest: riskremediation.AcceptanceRequest{
		Command: riskremediation.Command{ActorID: f.other, TicketID: f.ticket, OperationID: "accept-request-" + f.role},
		RiskID:  risk, Reason: "Synthetic bounded risk acceptance", ValidUntil: time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond),
	}, TicketInput: ticket.RiskWorkflowTicketInput{DefinitionVersion: f.version}}
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT t.revision,r.revision,l.id,l.revision FROM tickets t JOIN risk_dispatch_groups g ON g.ticket_id=t.id JOIN risk_ticket_links l ON l.group_id=g.id JOIN risk_items r ON r.id=l.risk_id WHERE t.id=$1 AND r.id=$2 AND l.ended_at IS NULL`, f.ticket, risk).Scan(&c.ExpectedTicketRevision, &c.ExpectedRiskRevision, &c.LinkID, &c.ExpectedLinkRevision))
	return c
}

func (f *writeFixture) acceptanceDecision(t *testing.T, pending riskremediation.Result) AcceptanceDecisionCommand {
	t.Helper()
	c := AcceptanceDecisionCommand{AcceptanceDecision: riskremediation.AcceptanceDecision{
		Command:      riskremediation.Command{ActorID: f.actor, TicketID: f.ticket, ExpectedTicketRevision: pending.TicketRevision, OperationID: "accept-decision-" + f.role},
		AcceptanceID: pending.AcceptanceID, ExpectedAcceptanceRevision: pending.AcceptanceRevision,
		ExpectedRiskRevision: pending.Items[0].RiskRevision, Decision: "approve", Reason: "Synthetic independent confirmation",
	}, TicketInput: ticket.RiskWorkflowTicketInput{DefinitionVersion: f.version}}
	return c
}

func (f *writeFixture) acceptanceSnapshot(t *testing.T) string {
	t.Helper()
	var acceptances string
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(a) ORDER BY id),'[]')::text FROM risk_acceptances a WHERE risk_id=ANY($1)`, []int64{f.riskA, f.riskB}).Scan(&acceptances))
	return f.writeSnapshot(t) + acceptances
}

func (f *writeFixture) requestAcceptance(t *testing.T, c AcceptanceRequestCommand) (riskremediation.Result, error) {
	t.Helper()
	return f.admittedWrite(t, c.ActorID, acceptRequestPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.RequestAcceptance(ctx, c) })
}
func (f *writeFixture) decideAcceptance(t *testing.T, c AcceptanceDecisionCommand) (riskremediation.Result, error) {
	t.Helper()
	return f.admittedWrite(t, c.ActorID, acceptDecisionPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.DecideAcceptance(ctx, c) })
}

func TestWorkflowAcceptanceOwnerAndIndependentPrimaryApprover(t *testing.T) {
	f := newAcceptanceFixture(t)
	c := f.acceptanceRequest(t, f.riskA)
	pending, err := f.requestAcceptance(t, c)
	require.NoError(t, err)
	require.Equal(t, "pending", pending.AcceptanceStatus)
	require.False(t, pending.TerminalAllowed)
	require.Equal(t, c.ExpectedTicketRevision, pending.TicketRevision)
	var state string
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT status FROM tickets WHERE id=$1`, f.ticket).Scan(&state))
	require.Equal(t, "pending_verify", state)
	// An ordinary owning ticket command also cannot close while acceptance is pending.
	before := f.acceptanceSnapshot(t)
	_, err = f.tickets.Transition(f.ctx, &model.TransitionTicketRequest{ID: f.ticket, To: "closed"}, f.actor)
	require.Error(t, err)
	require.Equal(t, before, f.acceptanceSnapshot(t))
	d := f.acceptanceDecision(t, pending)
	self := d
	self.ActorID = f.other
	manage, err := repository.CanManageRiskItem(f.ctx, workflowPool, f.riskA, f.other)
	require.NoError(t, err)
	require.True(t, manage, "self-approval negative control has genuine primary management authority")
	_, err = f.decideAcceptance(t, self)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	require.Equal(t, before, f.acceptanceSnapshot(t))
	approved, err := f.decideAcceptance(t, d)
	require.NoError(t, err)
	require.Equal(t, "approved", approved.AcceptanceStatus)
	require.False(t, approved.TerminalAllowed, "unresolved sibling prevents termination")
	require.Equal(t, pending.TicketRevision, approved.TicketRevision)
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT status FROM risk_items WHERE id=$1`, f.riskA).Scan(&state))
	require.Equal(t, "accepted", state)
	// The second independent decision consumes the already-published closed edge.
	c2 := f.acceptanceRequest(t, f.riskB)
	c2.OperationID += "-second"
	pending2, err := f.requestAcceptance(t, c2)
	require.NoError(t, err)
	d2 := f.acceptanceDecision(t, pending2)
	d2.OperationID += "-second"
	d2.TicketInput.CustomDataPatch = json.RawMessage(`{"review_mark":"accepted"}`)
	closed, err := f.decideAcceptance(t, d2)
	require.NoError(t, err)
	require.True(t, closed.TerminalAllowed)
	require.Greater(t, closed.TicketRevision, pending2.TicketRevision)
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT status FROM tickets WHERE id=$1`, f.ticket).Scan(&state))
	require.Equal(t, "closed", state)
	before = f.acceptanceSnapshot(t)
	for _, replay := range []func() (riskremediation.Result, error){
		func() (riskremediation.Result, error) { return f.requestAcceptance(t, c) },
		func() (riskremediation.Result, error) { return f.decideAcceptance(t, d2) },
	} {
		_, err = replay()
		require.NoError(t, err)
		require.Equal(t, before, f.acceptanceSnapshot(t))
	}
}

func TestWorkflowAcceptanceOriginalKAndParameterBinding(t *testing.T) {
	f := newAcceptanceFixture(t)
	c := f.acceptanceRequest(t, f.riskA)
	pending, err := f.requestAcceptance(t, c)
	require.NoError(t, err)
	before := f.acceptanceSnapshot(t)
	c.TicketInput.CustomDataPatch = json.RawMessage(` { } `)
	got, err := f.requestAcceptance(t, c)
	require.NoError(t, err)
	require.Equal(t, pending, got)
	for _, change := range []string{"reason", "deadline", "risk revision", "link revision", "ticket revision", "pin", "patch", "comment", "caller binding"} {
		d := c
		switch change {
		case "reason":
			d.Reason += "!"
		case "deadline":
			d.ValidUntil = d.ValidUntil.Add(time.Second)
		case "risk revision":
			d.ExpectedRiskRevision++
		case "link revision":
			d.ExpectedLinkRevision++
		case "ticket revision":
			d.ExpectedTicketRevision++
		case "pin":
			d.TicketInput.DefinitionVersion++
		case "patch":
			d.TicketInput.CustomDataPatch = json.RawMessage(`{"review_mark":"changed"}`)
		case "comment":
			d.TicketInput.Comment = "changed"
		case "caller binding":
			d.TicketBinding = "caller-controlled"
		}
		got, err := f.requestAcceptance(t, d)
		require.Error(t, err, change)
		require.Empty(t, got)
		require.Equal(t, before, f.acceptanceSnapshot(t))
	}
	d := f.acceptanceDecision(t, pending)
	approved, err := f.decideAcceptance(t, d)
	require.NoError(t, err)
	before = f.acceptanceSnapshot(t)
	got, err = f.decideAcceptance(t, d)
	require.NoError(t, err)
	require.Equal(t, approved, got)
	d.Reason += "!"
	got, err = f.decideAcceptance(t, d)
	require.ErrorIs(t, err, errcode.ErrConflict)
	require.Empty(t, got)
	require.Equal(t, before, f.acceptanceSnapshot(t))
}

func TestWorkflowAcceptanceCurrentIAMAndPrimaryScope(t *testing.T) {
	for _, change := range []string{"not owner", "secondary only", "revoked route", "disabled", "stale credential", "lost ticket read", "owner changed", "expired", "stale acceptance"} {
		t.Run(change, func(t *testing.T) {
			f := newAcceptanceFixture(t)
			c := f.acceptanceRequest(t, f.riskA)
			if change == "not owner" {
				c.ActorID = f.actor
				before := f.acceptanceSnapshot(t)
				_, err := f.requestAcceptance(t, c)
				require.ErrorIs(t, err, errcode.ErrNoPermission)
				require.Equal(t, before, f.acceptanceSnapshot(t))
				return
			}
			pending, err := f.requestAcceptance(t, c)
			require.NoError(t, err)
			d := f.acceptanceDecision(t, pending)
			mutate := func() {
				var err error
				switch change {
				case "secondary only":
					_, err = workflowPool.Exec(f.ctx, `UPDATE risk_items SET affected_org_id=$1 WHERE id=$2`, f.orgB, f.riskA)
					require.NoError(t, err)
					_, err = workflowPool.Exec(f.ctx, `DELETE FROM user_orgs WHERE user_id=$1 AND org_id=$2`, f.actor, f.orgA)
					require.NoError(t, err)
					_, _, _, err = repository.GetRiskItemVisible(f.ctx, workflowPool, f.riskA, f.actor)
					require.NoError(t, err, "affected anchor retains visibility but cannot approve")
				case "revoked route":
					_, err = workflowPool.Exec(f.ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1=$2`, "role::"+f.role, acceptDecisionPath)
				case "disabled":
					_, err = workflowPool.Exec(f.ctx, `UPDATE users SET status=0 WHERE id=$1`, f.actor)
				case "stale credential":
					_, err = workflowPool.Exec(f.ctx, `UPDATE users SET session_epoch=session_epoch+1,session_projection_epoch=session_projection_epoch+1 WHERE id=$1`, f.actor)
				case "lost ticket read":
					_, err = workflowPool.Exec(f.ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1='/api/v1/tickets/:id'`, "role::"+f.role)
				case "owner changed":
					_, err = workflowPool.Exec(f.ctx, `UPDATE risk_items SET owner_user_id=$1 WHERE id=$2`, f.actor, f.riskA)
				case "expired":
					_, err = workflowPool.Exec(f.ctx, `UPDATE risk_acceptances SET valid_until=clock_timestamp()-interval '1 second' WHERE id=$1`, pending.AcceptanceID)
				case "stale acceptance":
					d.ExpectedAcceptanceRevision++
				}
				require.NoError(t, err)
			}
			// Mutation is after genuine middleware admission. Snapshot after the
			// independent mutation so it cannot hide any command-side partial write.
			var before string
			got, err := f.admittedWrite(t, d.ActorID, acceptDecisionPath, func() { mutate(); before = f.acceptanceSnapshot(t) }, func(ctx context.Context) (riskremediation.Result, error) { return f.service.DecideAcceptance(ctx, d) })
			require.Error(t, err)
			require.Empty(t, got)
			require.Equal(t, before, f.acceptanceSnapshot(t))
		})
	}
}

func TestWorkflowAcceptanceRejectUsesExistingRule(t *testing.T) {
	f := newAcceptanceFixture(t)
	c := f.acceptanceRequest(t, f.riskA)
	pending, err := f.requestAcceptance(t, c)
	require.NoError(t, err)
	d := f.acceptanceDecision(t, pending)
	d.Decision = "reject"
	d.TicketInput.CustomDataPatch = json.RawMessage(`{"review_mark":"rejected"}`)
	got, err := f.decideAcceptance(t, d)
	require.NoError(t, err)
	require.Equal(t, "rejected", got.AcceptanceStatus)
	require.False(t, got.TerminalAllowed)
	var status string
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT status FROM tickets WHERE id=$1`, f.ticket).Scan(&status))
	require.Equal(t, "in_progress", status)
}

func TestWorkflowRiskOwnerCanReturnExpiredAcceptanceToRemediation(t *testing.T) {
	f := newAcceptanceFixture(t)
	pending, err := f.requestAcceptance(t, f.acceptanceRequest(t, f.riskA))
	require.NoError(t, err)
	// Exercise recovery after the pending request loses its only independent approver.
	_, err = workflowPool.Exec(f.ctx, `UPDATE tickets SET created_by=$1 WHERE id=$2`, f.other, f.ticket)
	require.NoError(t, err)
	run := func(c ReviewCommand) (riskremediation.Result, error) {
		return f.admittedWrite(t, c.ActorID, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) {
			return f.service.Review(ctx, c)
		})
	}
	review := f.reviewCommand(t, pending, 0)
	review.ActorID = f.other
	review.Items[0].Decision = "reject"
	review.Items[0].Evidence = nil
	active, err := run(review)
	require.NoError(t, err)
	require.Equal(t, "conflict", active.Items[0].Result)

	_, err = workflowPool.Exec(f.ctx, `UPDATE risk_acceptances SET valid_until=clock_timestamp()-interval '1 second' WHERE id=$1`, pending.AcceptanceID)
	require.NoError(t, err)
	review.OperationID += "-approval"
	review.Items[0].Decision = "approve"
	blocked, err := run(review)
	require.NoError(t, err)
	require.Equal(t, "conflict", blocked.Items[0].Result)

	review.OperationID += "-return"
	review.Items[0].Decision = "reject"
	got, err := run(review)
	require.NoError(t, err)
	require.Equal(t, "rejected", got.Items[0].Result)
	var ticketStatus, itemState, acceptanceStatus string
	require.NoError(t, workflowPool.QueryRow(f.ctx, `
		SELECT t.status, l.remediation_state, a.status
		FROM risk_acceptances a JOIN risk_ticket_links l ON l.id = a.link_id
		JOIN risk_dispatch_groups g ON g.id = l.group_id JOIN tickets t ON t.id = g.ticket_id
		WHERE a.id=$1`, pending.AcceptanceID).Scan(&ticketStatus, &itemState, &acceptanceStatus))
	require.Equal(t, "in_progress", ticketStatus)
	require.Equal(t, "rejected", itemState)
	require.Equal(t, "expired", acceptanceStatus)
}

func TestWorkflowAcceptanceNarrowBoundaryDoesNotSwallowInputs(t *testing.T) {
	f := newAcceptanceFixture(t)
	c := f.acceptanceRequest(t, f.riskA)
	before := f.acceptanceSnapshot(t)
	for _, change := range []string{"request patch", "request comment", "standalone", "caller binding", "missing purpose", "wrong purpose", "wrong actor"} {
		d := c
		var got riskremediation.Result
		var err error
		switch change {
		case "request patch":
			d.TicketInput.CustomDataPatch = json.RawMessage(`{"review_mark":"would-be-lost"}`)
		case "request comment":
			d.TicketInput.Comment = "would-be-lost"
		case "standalone":
			d.TicketID = 0
			d.ExpectedTicketRevision = 0
		case "caller binding":
			d.TicketBinding = "caller-controlled"
		}
		switch change {
		case "missing purpose":
			got, err = f.service.RequestAcceptance(f.ctx, d)
		case "wrong purpose":
			got, err = f.admittedWrite(t, d.ActorID, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.RequestAcceptance(ctx, d) })
		case "wrong actor":
			got, err = f.admittedWrite(t, f.actor, acceptRequestPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.RequestAcceptance(ctx, d) })
		default:
			got, err = f.requestAcceptance(t, d)
		}
		require.Error(t, err, change)
		require.Empty(t, got)
		require.Equal(t, before, f.acceptanceSnapshot(t))
	}
	pending, err := f.requestAcceptance(t, c)
	require.NoError(t, err, "failed attempts must release the same K for the valid request")
	d := f.acceptanceDecision(t, pending)
	d.TicketInput.Comment = "partial approval must not swallow this"
	before = f.acceptanceSnapshot(t)
	got, err := f.decideAcceptance(t, d)
	require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
	require.Empty(t, got)
	require.Equal(t, before, f.acceptanceSnapshot(t))
}

func TestWorkflowAcceptanceHiddenDecisionMatchesAbsent(t *testing.T) {
	f := newAcceptanceFixture(t)
	hidden := newAcceptanceFixture(t)
	pending, err := hidden.requestAcceptance(t, hidden.acceptanceRequest(t, hidden.riskA))
	require.NoError(t, err)
	d := f.acceptanceDecision(t, pending)
	tk, err := repository.NewTicketRepo(workflowPool).GetByID(f.ctx, f.ticket)
	require.NoError(t, err)
	d.ExpectedTicketRevision = tk.Revision
	before, hiddenBefore := f.acceptanceSnapshot(t), hidden.acceptanceSnapshot(t)
	for _, id := range []int64{pending.AcceptanceID, 1 << 62} {
		d.AcceptanceID = id
		got, err := f.decideAcceptance(t, d)
		require.ErrorIs(t, err, errcode.ErrNotFound, "hidden and missing acceptance must be indistinguishable")
		require.Empty(t, got)
		require.Equal(t, before, f.acceptanceSnapshot(t))
		require.Equal(t, hiddenBefore, hidden.acceptanceSnapshot(t))
	}
}

func TestWorkflowAcceptanceDecisionCannotProbeIDsWithoutTicketRead(t *testing.T) {
	f := newAcceptanceFixture(t)
	pending, err := f.requestAcceptance(t, f.acceptanceRequest(t, f.riskA))
	require.NoError(t, err)
	d := f.acceptanceDecision(t, pending)
	before := f.acceptanceSnapshot(t)
	for _, id := range []int64{pending.AcceptanceID, 1 << 62} {
		d.AcceptanceID = id
		got, err := f.admittedWrite(t, d.ActorID, acceptDecisionPath, func() {
			_, err := workflowPool.Exec(f.ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1='/api/v1/tickets/:id'`, "role::"+f.role)
			require.NoError(t, err)
		}, func(ctx context.Context) (riskremediation.Result, error) { return f.service.DecideAcceptance(ctx, d) })
		require.ErrorIs(t, err, errcode.ErrNoPermission, "ticket GET must be checked before looking up any acceptance ID")
		require.Empty(t, got)
		require.Equal(t, before, f.acceptanceSnapshot(t))
	}
}

func TestWorkflowAcceptanceOriginalKAfterDetach(t *testing.T) {
	f := newAcceptanceFixture(t)
	c := f.acceptanceRequest(t, f.riskA)
	pending, err := f.requestAcceptance(t, c)
	require.NoError(t, err)
	d := f.acceptanceDecision(t, pending)
	approved, err := f.decideAcceptance(t, d)
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `UPDATE risk_ticket_links SET ended_at=clock_timestamp(),end_reason='synthetic authorized detach' WHERE id=$1`, c.LinkID)
	require.NoError(t, err)
	before := f.acceptanceSnapshot(t)
	got, err := f.requestAcceptance(t, c)
	require.NoError(t, err)
	require.Equal(t, pending, got)
	got, err = f.decideAcceptance(t, d)
	require.NoError(t, err)
	require.Equal(t, approved, got)
	require.Equal(t, before, f.acceptanceSnapshot(t))
}

func TestWorkflowAcceptanceExpiryDuringRealTicketWriteRollsBack(t *testing.T) {
	f := newAcceptanceFixture(t)
	pending, err := f.requestAcceptance(t, f.acceptanceRequest(t, f.riskA))
	require.NoError(t, err)
	_, err = f.decideAcceptance(t, f.acceptanceDecision(t, pending))
	require.NoError(t, err)
	c := f.acceptanceRequest(t, f.riskB)
	c.OperationID += "-second"
	pending, err = f.requestAcceptance(t, c)
	require.NoError(t, err)
	d := f.acceptanceDecision(t, pending)
	d.OperationID += "-second"
	d.TicketInput.CustomDataPatch = json.RawMessage(`{"review_mark":"expiry"}`)
	// Fault is on the real ticket write after the kernel's guarded decision.
	// Wait until the stored acceptance deadline instead of guessing a race delay.
	name := "wf_accept_expiry_" + strconv.FormatInt(f.ticket, 10)
	_, err = workflowPool.Exec(f.ctx, `CREATE FUNCTION `+name+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.id=`+strconv.FormatInt(f.ticket, 10)+` AND NEW.status='closed' THEN
 PERFORM pg_sleep(0.02+GREATEST(0,EXTRACT(EPOCH FROM ((SELECT valid_until FROM risk_acceptances WHERE id=`+strconv.FormatInt(pending.AcceptanceID, 10)+`)-clock_timestamp()))));
 END IF; RETURN NEW; END $$`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := workflowPool.Exec(context.Background(), `DROP FUNCTION `+name+`() CASCADE`)
		require.NoError(t, err)
	})
	_, err = workflowPool.Exec(f.ctx, `CREATE TRIGGER `+name+` BEFORE UPDATE OF status ON tickets FOR EACH ROW EXECUTE FUNCTION `+name+`() `)
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `UPDATE risk_acceptances SET valid_until=clock_timestamp()+interval '1 second' WHERE id=$1`, pending.AcceptanceID)
	require.NoError(t, err)
	before := f.acceptanceSnapshot(t)
	got, err := f.decideAcceptance(t, d)
	require.ErrorIs(t, err, errcode.ErrConflict)
	require.Empty(t, got)
	require.Equal(t, before, f.acceptanceSnapshot(t), "late expiry must roll back acceptance, risk, link, ticket, events, receipts and intents")
}

func TestWorkflowFinalReviewRechecksAcceptedRiskAfterTicketWrite(t *testing.T) {
	f := seedAcceptanceFixture(t, true)
	pending, err := f.requestAcceptance(t, f.acceptanceRequest(t, f.riskA))
	require.NoError(t, err)
	approved, err := f.decideAcceptance(t, f.acceptanceDecision(t, pending))
	require.NoError(t, err)
	require.False(t, approved.TerminalAllowed)
	var item riskremediation.ItemResult
	require.NoError(t, workflowPool.QueryRow(f.ctx, `
		SELECT l.id,l.revision,r.revision,l.proposal_version
		FROM risk_ticket_links l JOIN risk_items r ON r.id=l.risk_id
		WHERE r.id=$1 AND l.ended_at IS NULL`, f.riskB).
		Scan(&item.LinkID, &item.LinkRevision, &item.RiskRevision, &item.ProposalVersion))
	review := f.reviewCommand(t, riskremediation.Result{TicketRevision: approved.TicketRevision, Items: []riskremediation.ItemResult{item}}, 0)

	name := "wf_review_expiry_" + strconv.FormatInt(f.ticket, 10)
	_, err = workflowPool.Exec(f.ctx, `CREATE FUNCTION `+name+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
		IF NEW.id=`+strconv.FormatInt(f.ticket, 10)+` AND NEW.status='closed' THEN
			PERFORM pg_sleep(0.02+GREATEST(0,EXTRACT(EPOCH FROM ((SELECT accepted_until FROM risk_items WHERE id=`+strconv.FormatInt(f.riskA, 10)+`)-clock_timestamp()))));
		END IF; RETURN NEW; END $$`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := workflowPool.Exec(context.Background(), `DROP FUNCTION `+name+`() CASCADE`)
		require.NoError(t, err)
	})
	_, err = workflowPool.Exec(f.ctx, `CREATE TRIGGER `+name+` BEFORE UPDATE OF status ON tickets FOR EACH ROW EXECUTE FUNCTION `+name+`() `)
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `UPDATE risk_items SET accepted_until=clock_timestamp()+interval '1 second' WHERE id=$1`, f.riskA)
	require.NoError(t, err)
	before := f.acceptanceSnapshot(t)
	got, err := f.admittedWrite(t, f.actor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Review(ctx, review) })
	require.ErrorIs(t, err, errcode.ErrConflict)
	require.Empty(t, got)
	require.Equal(t, before, f.acceptanceSnapshot(t))
}

func TestWorkflowAcceptanceRejectsNewRequestWhileTicketInProgress(t *testing.T) {
	f := newAcceptanceFixture(t)
	pending, err := f.requestAcceptance(t, f.acceptanceRequest(t, f.riskA))
	require.NoError(t, err)
	d := f.acceptanceDecision(t, pending)
	d.Decision = "reject"
	d.TicketInput.CustomDataPatch = json.RawMessage(`{"review_mark":"rejected"}`)
	_, err = f.decideAcceptance(t, d)
	require.NoError(t, err)
	c := f.acceptanceRequest(t, f.riskB)
	c.OperationID += "-B"
	before := f.acceptanceSnapshot(t)
	got, err := f.requestAcceptance(t, c)
	require.ErrorIs(t, err, errcode.ErrConflict)
	require.Empty(t, got)
	require.Equal(t, before, f.acceptanceSnapshot(t), "rejected request may not freeze the ticket or consume the operation")
}

func TestWorkflowLegacyInProgressAcceptanceCanBeRejectedWithoutSelfEdge(t *testing.T) {
	f := newAcceptanceFixture(t)
	pending, err := f.requestAcceptance(t, f.acceptanceRequest(t, f.riskA))
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `UPDATE tickets SET status='in_progress' WHERE id=$1`, f.ticket)
	require.NoError(t, err)
	d := f.acceptanceDecision(t, pending)
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT revision FROM tickets WHERE id=$1`, f.ticket).Scan(&d.ExpectedTicketRevision))
	d.Decision = "reject"
	got, err := f.decideAcceptance(t, d)
	require.NoError(t, err)
	require.Equal(t, "rejected", got.AcceptanceStatus)
	require.Equal(t, d.ExpectedTicketRevision, got.TicketRevision, "the decision must not invent a ticket transition")
	var status string
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT status FROM tickets WHERE id=$1`, f.ticket).Scan(&status))
	require.Equal(t, "in_progress", status)
}

func TestWorkflowAcceptanceExactChildRollsBackAfterRealTicketSQL(t *testing.T) {
	f := newAcceptanceFixture(t)
	// Reject consumes a real edge and writes a ticket receipt before the fault.
	c := f.acceptanceRequest(t, f.riskA)
	pending, err := f.requestAcceptance(t, c)
	require.NoError(t, err)
	d := f.acceptanceDecision(t, pending)
	d.Decision = "reject"
	d.TicketInput.CustomDataPatch = json.RawMessage(`{"review_mark":"fault"}`)
	input, binding, err := ticket.PrepareRiskWorkflowTicketInput(d.TicketInput)
	require.NoError(t, err)
	d.TicketBinding = binding
	before := f.acceptanceSnapshot(t)
	applied, childCommitted, outerCommitted := false, false, false
	_, err = f.admittedWrite(t, d.ActorID, acceptDecisionPath, nil, func(ctx context.Context) (riskremediation.Result, error) {
		err := repository.WithIAMCommandGuard(ctx, workflowPool, func(ctx context.Context, outer pgx.Tx) error {
			callbacks := f.service.acceptanceCallbacks(d.ActorID, acceptDecisionPath, input)
			apply := callbacks.ApplyTicket
			callbacks.ApplyTicket = func(ctx context.Context, child pgx.Tx, effect riskremediation.TicketEffect) (int64, error) {
				require.NotSame(t, outer, child)
				revision, err := apply(ctx, child, effect)
				if err != nil {
					return 0, err
				}
				applied = true
				require.NoError(t, repository.AfterIAMCommit(ctx, func() error { childCommitted = true; return nil }))
				_, err = child.Exec(ctx, `SELECT 1/0`)
				return revision, err
			}
			_, err := riskremediation.New(workflowPool, callbacks).DecideAcceptanceTx(ctx, outer, d.AcceptanceDecision)
			require.Error(t, err)
			var one int
			require.NoError(t, outer.QueryRow(ctx, `SELECT 1`).Scan(&one))
			return repository.AfterIAMCommit(ctx, func() error { outerCommitted = true; return nil })
		})
		return riskremediation.Result{}, err
	})
	require.NoError(t, err)
	require.True(t, applied)
	require.True(t, outerCommitted)
	require.False(t, childCommitted)
	require.Equal(t, before, f.acceptanceSnapshot(t))
}
