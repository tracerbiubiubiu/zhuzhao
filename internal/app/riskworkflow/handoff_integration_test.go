//go:build integration

package riskworkflow

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

func seedHandoffSuccessor(t *testing.T, f *writeFixture) int64 {
	t.Helper()
	// The acceptance scenario runs several complete HTTP transactions plus a
	// migration lock check in one fixture. Keep a finite scenario-level budget.
	var cancel context.CancelFunc
	f.ctx, cancel = context.WithTimeout(t.Context(), 2*time.Minute)
	t.Cleanup(cancel)
	_, err := workflowPool.Exec(f.ctx, `UPDATE user_orgs SET org_member_role='admin' WHERE user_id=$1 AND org_id=ANY($2)`, f.actor, []int64{f.orgA, f.orgB})
	require.NoError(t, err)
	for _, route := range []struct{ path, method string }{{ticket.RiskRetestHandoffPath, "POST"}, {contextPrefix + ":id", "GET"}} {
		_, err = workflowPool.Exec(f.ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,$3)`, "role::"+f.role, route.path, route.method)
		require.NoError(t, err)
		_, err = f.enforcer.AddPolicy("role::"+f.role, route.path, route.method)
		require.NoError(t, err)
	}
	return addHandoffSuccessor(t, f, "-successor")
}

func addHandoffSuccessor(t *testing.T, f *writeFixture, suffix string) int64 {
	t.Helper()
	var successor int64
	require.NoError(t, workflowPool.QueryRow(f.ctx, `INSERT INTO users(username,password,status) VALUES($1,'synthetic-non-login',1) RETURNING id`, f.role+suffix).Scan(&successor))
	_, err := workflowPool.Exec(f.ctx, `INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE code=$2`, successor, f.role)
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES($1,$2),($1,$3)`, successor, f.orgA, f.orgB)
	require.NoError(t, err)
	return successor
}

func TestHandoffRetestOnOriginalCreatorPinnedTicket(t *testing.T) {
	f := newWriteFixture(t)
	successor := seedHandoffSuccessor(t, f)
	var originalDepartment = f.orgA
	_, err := workflowPool.Exec(f.ctx, `UPDATE risk_dispatch_groups SET owner_org_at_dispatch=$1 WHERE ticket_id=$2`, originalDepartment, f.ticket)
	require.NoError(t, err)
	finished, err := f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) {
		return f.service.Finish(ctx, f.finishCommand(t))
	})
	require.NoError(t, err)
	first := f.reviewCommand(t, finished, 0)
	partial, err := f.admittedWrite(t, f.actor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) {
		return f.service.Review(ctx, first)
	})
	require.NoError(t, err)
	second := finished.Items[1]
	readProposal := func() string {
		var body string
		require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT to_jsonb(s)::text FROM risk_remediation_submissions s WHERE link_id=$1 AND proposal_version=$2`, second.LinkID, second.ProposalVersion).Scan(&body))
		return body
	}
	proposal := readProposal()
	// Positive control for the actual dispatch-compatible immutable publication.
	repo := repository.NewTicketRepo(workflowPool)
	tk, err := repo.GetByID(f.ctx, f.ticket)
	require.NoError(t, err)
	tx, err := workflowPool.Begin(f.ctx)
	require.NoError(t, err)
	definition, _, cause := repo.BoundTicketDefinitionTx(f.ctx, tx, tk.TypeCode, tk.DefinitionVersion)
	require.NoError(t, tx.Rollback(f.ctx))
	require.NoError(t, cause)
	require.NoError(t, ticket.ValidateRiskDispatchDefinition(&definition.Type, definition.Fields))
	cmd := riskremediation.HandoffCommand{
		Command:           riskremediation.Command{ActorID: f.actor, OperationID: uuid.NewString(), TicketID: f.ticket, ExpectedTicketRevision: partial.TicketRevision},
		DefinitionVersion: f.version, SuccessorUserID: successor, Reason: "Synthetic change of retest responsibility",
		Items: []riskremediation.HandoffItem{{LinkID: second.LinkID, ExpectedLinkRevision: second.LinkRevision, ExpectedRiskRevision: second.RiskRevision, ExpectedProposalVersion: second.ProposalVersion}},
	}
	handoff := func(c riskremediation.HandoffCommand) (riskremediation.Result, error) {
		return f.admittedWrite(t, c.ActorID, ticket.RiskRetestHandoffPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Handoff(ctx, c) })
	}
	before := f.writeSnapshot(t)
	stale := cmd
	stale.ExpectedTicketRevision--
	_, err = handoff(stale)
	require.ErrorIs(t, err, errcode.ErrConflict)
	staleItem := cmd
	staleItem.Items = append([]riskremediation.HandoffItem(nil), cmd.Items...)
	staleItem.Items[0].ExpectedRiskRevision--
	_, err = handoff(staleItem)
	require.ErrorIs(t, err, errcode.ErrConflict)
	unmanaged := cmd
	unmanaged.ActorID = f.other
	_, err = handoff(unmanaged)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	self := cmd
	self.SuccessorUserID = f.other
	_, err = handoff(self)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	_, err = workflowPool.Exec(f.ctx, `UPDATE users SET status=0 WHERE id=$1`, successor)
	require.NoError(t, err)
	_, err = handoff(cmd)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	_, err = workflowPool.Exec(f.ctx, `UPDATE users SET status=1 WHERE id=$1`, successor)
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `DELETE FROM user_orgs WHERE user_id=$1 AND org_id=$2`, successor, f.orgB)
	require.NoError(t, err)
	_, err = handoff(cmd)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	_, err = workflowPool.Exec(f.ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES($1,$2)`, successor, f.orgB)
	require.NoError(t, err)
	// A different live ticket for this finding and successor is a legal state.
	// The two tickets keep their separate scopes after a rejected handoff.
	competing, err := f.tickets.Create(f.ctx, &model.CreateTicketRequest{TypeCode: tk.TypeCode, Title: "Synthetic successor's existing ticket", OrgID: f.orgA}, f.actor)
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `INSERT INTO risk_dispatch_groups(finding_key,owner_user_id,management_org_id,target_org_id,type_code,definition_version,approved_scope,group_key,ticket_id,ticket_ref_snapshot,operator_user_id)
		SELECT finding_key,$2,management_org_id,target_org_id,type_code,definition_version,approved_scope,repeat('a',64),$3,$4,operator_user_id
		FROM risk_dispatch_groups WHERE ticket_id=$1`, f.ticket, successor, competing.ID, competing.HumanKey)
	require.NoError(t, err)
	_, err = handoff(cmd)
	require.ErrorIs(t, err, errcode.ErrConflict)
	_, err = workflowPool.Exec(f.ctx, `UPDATE risk_dispatch_groups SET ended_at=clock_timestamp() WHERE ticket_id=$1`, competing.ID)
	require.NoError(t, err)
	require.Equal(t, before, f.writeSnapshot(t))
	f.postHandoff(t, cmd)
	done, err := f.read(t, cmd.OperationID, nil)
	require.NoError(t, err)
	require.Equal(t, partial.TicketRevision+1, done.TicketRevision)
	require.Len(t, done.Items, 2)
	require.Equal(t, "preserved", done.Items[0].Result)
	require.Equal(t, second.ProposalVersion, done.Items[1].ProposalVersion)
	require.Equal(t, successor, done.SuccessorUserID)
	require.Equal(t, proposal, readProposal())
	var creator, department, historicalOwner, currentOwner, effectiveOwner, effectiveRevision int64
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT t.created_by,g.owner_org_at_dispatch,l.owner_at_link,g.owner_user_id,l.effective_owner_id,l.resolved_risk_revision FROM tickets t JOIN risk_dispatch_groups g ON g.ticket_id=t.id JOIN risk_ticket_links l ON l.group_id=g.id WHERE t.id=$1 AND l.id=$2`, f.ticket, finished.Items[0].LinkID).Scan(&creator, &department, &historicalOwner, &currentOwner, &effectiveOwner, &effectiveRevision))
	require.Equal(t, f.actor, creator)
	require.Equal(t, originalDepartment, department)
	require.Equal(t, f.actor, historicalOwner)
	require.Equal(t, successor, currentOwner)
	require.Equal(t, f.actor, effectiveOwner)
	require.Equal(t, partial.Items[0].RiskRevision, effectiveRevision)
	var auditReason, dispatchKey, liveKey string
	var noticeContainsReason bool
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT e.payload->>'reason',g.group_key,g.retest_group_key,i.payload ? 'reason'
		FROM risk_events e JOIN risk_remediation_intents i ON i.risk_id=e.risk_id AND i.operation_id=e.operation_id AND i.event_kind=e.event_kind
		JOIN risk_dispatch_groups g ON g.ticket_id=i.ticket_id WHERE e.operation_id=$1 AND e.event_kind='retest_handoff'`, cmd.OperationID).
		Scan(&auditReason, &dispatchKey, &liveKey, &noticeContainsReason))
	require.Equal(t, cmd.Reason, auditReason)
	require.NotEqual(t, dispatchKey, liveKey)
	require.False(t, noticeContainsReason)
	postHandoff := f.writeSnapshot(t)
	replay, err := handoff(cmd)
	require.NoError(t, err)
	require.Equal(t, done, replay)
	recovered, err := f.read(t, cmd.OperationID, nil)
	require.NoError(t, err)
	require.Equal(t, done, recovered)
	changed := cmd
	changed.Reason = "Another synthetic reason"
	_, err = handoff(changed)
	require.ErrorIs(t, err, errcode.ErrConflict)
	require.Equal(t, postHandoff, f.writeSnapshot(t))
	err = f.tickets.Close(f.ctx, &model.CloseTicketRequest{ID: f.ticket, Comment: "Synthetic ordinary close"}, successor)
	require.ErrorIs(t, err, errcode.ErrNoPermission)

	current := f.readHandoffContext(t, successor)
	require.Equal(t, f.version, current.DefinitionVersion)
	require.Contains(t, current.AllowedActions, "review")
	require.NotContains(t, current.AllowedActions, "handoff")
	require.Len(t, current.Items, 2)
	require.False(t, current.Items[0].IsRiskOwner)
	require.True(t, current.Items[1].IsRiskOwner)
	result := finished
	result.TicketRevision = current.TicketRevision
	result.Items[1].RiskRevision = current.Items[1].RiskRevision
	result.Items[1].LinkRevision = current.Items[1].LinkRevision
	result.Items[1].ProposalVersion = current.Items[1].ProposalVersion
	next := f.reviewCommand(t, result, 1)
	_, err = f.admittedWrite(t, f.actor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Review(ctx, next) })
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	next.ActorID = successor
	closed, err := f.admittedWrite(t, successor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Review(ctx, next) })
	require.NoError(t, err)
	require.True(t, closed.TerminalAllowed)
	require.Equal(t, proposal, readProposal())
	// Historical handoff facts still block rollback even after the ticket closes.
	migration := handoffMigration(t, "down")
	rollback, err := workflowPool.Begin(f.ctx)
	require.NoError(t, err)
	_, err = rollback.Exec(f.ctx, migration)
	require.ErrorContains(t, err, "risk retest handoff facts exist")
	require.NoError(t, rollback.Rollback(f.ctx))
}

func handoffMigration(t *testing.T, direction string) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	require.True(t, ok)
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(here), "../../../migrations/000095_risk_retest_handoff."+direction+".sql"))
	require.NoError(t, err)
	return string(contents)
}

func (f *writeFixture) readHandoffContext(t *testing.T, actor int64) model.RiskWorkflowContext {
	t.Helper()
	var epoch int64
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT session_epoch FROM users WHERE id=$1`, actor).Scan(&epoch))
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", actor); c.Set("session_epoch", epoch); c.Next() }, middleware.CasbinAuth(f.enforcer, databaseRoles{workflowPool}, nil))
	reader := NewReceiptService(workflowPool, f.tickets)
	var got model.RiskWorkflowContext
	r.GET(contextPrefix+":id", func(c *gin.Context) {
		var err error
		got, err = reader.Current(c.Request.Context(), actor, f.ticket)
		require.NoError(t, err)
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", contextPrefix+strconv.FormatInt(f.ticket, 10), nil).WithContext(f.ctx))
	require.Equal(t, f.ticket, got.TicketID)
	return got
}

func TestHandoffRestoresFixedProposalAfterPendingAcceptance(t *testing.T) {
	f := newAcceptanceFixture(t)
	successor := seedHandoffSuccessor(t, f)
	otherSuccessor := addHandoffSuccessor(t, f, "-other-successor")
	acceptance, err := f.requestAcceptance(t, f.acceptanceRequest(t, f.riskA))
	require.NoError(t, err)
	current := f.readHandoffContext(t, f.actor)
	c := f.handoffCommand(current, successor, "Synthetic acceptance applicant changed")
	run := func(c riskremediation.HandoffCommand) (riskremediation.Result, error) {
		return f.admittedWrite(t, c.ActorID, ticket.RiskRetestHandoffPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Handoff(ctx, c) })
	}
	partial := c
	partial.Items = c.Items[:1]
	before := f.acceptanceSnapshot(t)
	_, err = run(partial)
	require.ErrorIs(t, err, errcode.ErrConflict)
	require.Equal(t, before, f.acceptanceSnapshot(t))
	type transfer struct {
		result riskremediation.Result
		err    error
	}
	start := make(chan struct{})
	results := make(chan transfer, 2)
	competing := c
	competing.OperationID += "-competing"
	competing.SuccessorUserID = otherSuccessor
	for _, command := range []riskremediation.HandoffCommand{c, competing} {
		go func() {
			<-start
			result, err := run(command)
			results <- transfer{result, err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	if first.err != nil {
		first, second = second, first
	}
	require.NoError(t, first.err)
	require.ErrorIs(t, second.err, errcode.ErrConflict)
	transferred := first.result
	successor = transferred.SuccessorUserID
	require.Contains(t, []int64{c.SuccessorUserID, competing.SuccessorUserID}, successor)
	require.Len(t, transferred.Items, 2)
	var state, reason string
	var previousApplicant, acceptanceRevision int64
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT status,reason,applicant_id,revision FROM risk_acceptances WHERE id=$1`, acceptance.AcceptanceID).Scan(&state, &reason, &previousApplicant, &acceptanceRevision))
	require.Equal(t, "superseded", state)
	require.NotEmpty(t, reason)
	require.Equal(t, f.other, previousApplicant)
	require.Equal(t, acceptance.AcceptanceRevision+1, acceptanceRevision)
	newContext := f.readHandoffContext(t, successor)
	for _, item := range newContext.Items {
		require.Equal(t, "pending_verify", item.RemediationState)
	}
	review := f.reviewCommand(t, transferred, 0, 1)
	review.ActorID = successor
	review.OperationID += "-after-acceptance"
	closed, err := f.admittedWrite(t, successor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Review(ctx, review) })
	require.NoError(t, err)
	require.True(t, closed.TerminalAllowed)
}

func TestHandoffBindsAlreadyConfirmedOwnerOfLegacyTicket(t *testing.T) {
	f := newWriteFixture(t)
	successor := seedHandoffSuccessor(t, f)
	// Existing data shape from responsibility confirmation before proposals.
	// The live group and its creator predate that change.
	_, err := workflowPool.Exec(f.ctx, `UPDATE risk_items SET owner_user_id=$1 WHERE id=ANY($2)`, successor, []int64{f.riskA, f.riskB})
	require.NoError(t, err)
	_, err = f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) {
		return f.service.Finish(ctx, f.finishCommand(t))
	})
	require.NoError(t, err)
	c := f.handoffCommand(f.readHandoffContext(t, f.actor), successor, "Synthetic confirmed owner needs legacy ticket binding")
	transfer, err := f.admittedWrite(t, f.actor, ticket.RiskRetestHandoffPath, nil, func(ctx context.Context) (riskremediation.Result, error) {
		return f.service.Handoff(ctx, c)
	})
	require.NoError(t, err)
	var previousOwner, previousGroupOwner int64
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT from_owner_id,from_group_owner_id FROM risk_retest_handoff_items WHERE operation_id=$1 AND link_id=$2`, c.OperationID, c.Items[0].LinkID).Scan(&previousOwner, &previousGroupOwner))
	require.Equal(t, successor, previousOwner)
	require.Equal(t, f.actor, previousGroupOwner)
	review := f.reviewCommand(t, transfer, 0, 1)
	review.ActorID = successor
	closed, err := f.admittedWrite(t, successor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Review(ctx, review) })
	require.NoError(t, err)
	require.True(t, closed.TerminalAllowed)
}

func (f *writeFixture) handoffCommand(current model.RiskWorkflowContext, successor int64, reason string) riskremediation.HandoffCommand {
	c := riskremediation.HandoffCommand{
		Command:           riskremediation.Command{ActorID: f.actor, TicketID: f.ticket, ExpectedTicketRevision: current.TicketRevision, OperationID: uuid.NewString()},
		DefinitionVersion: f.version, SuccessorUserID: successor, Reason: reason,
	}
	for _, item := range current.Items {
		if item.RemediationState != "resolved" {
			c.Items = append(c.Items, riskremediation.HandoffItem{LinkID: item.LinkID, ExpectedLinkRevision: item.LinkRevision,
				ExpectedRiskRevision: item.RiskRevision, ExpectedProposalVersion: item.ProposalVersion})
		}
	}
	return c
}

func (f workflowHTTPFixture) Handoff(ctx context.Context, c riskremediation.HandoffCommand) (riskremediation.Result, error) {
	return f.writer.Handoff(ctx, c)
}

func (f *writeFixture) postHandoff(t *testing.T, c riskremediation.HandoffCommand) {
	t.Helper()
	stringID := func(id int64) string { return strconv.FormatInt(id, 10) }
	items := make([]map[string]string, 0, len(c.Items))
	for _, item := range c.Items {
		items = append(items, map[string]string{"link_id": stringID(item.LinkID), "expected_link_revision": stringID(item.ExpectedLinkRevision),
			"expected_risk_revision": stringID(item.ExpectedRiskRevision), "expected_proposal_version": stringID(item.ExpectedProposalVersion)})
	}
	body, err := json.Marshal(map[string]any{
		"operation_id": c.OperationID, "ticket_id": stringID(c.TicketID), "definition_version": stringID(c.DefinitionVersion),
		"expected_ticket_revision": stringID(c.ExpectedTicketRevision), "successor_user_id": stringID(c.SuccessorUserID), "reason": c.Reason, "items": items,
	})
	require.NoError(t, err)
	var epoch int64
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT session_epoch FROM users WHERE id=$1`, c.ActorID).Scan(&epoch))
	r := gin.New()
	r.Use(func(ctx *gin.Context) { ctx.Set("userID", c.ActorID); ctx.Set("session_epoch", epoch); ctx.Next() }, middleware.CasbinAuth(f.enforcer, databaseRoles{workflowPool}, nil))
	h := handler.NewRiskWorkflowHandler(workflowHTTPFixture{writer: f.service})
	r.POST(ticket.RiskRetestHandoffPath, h.Handoff)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, ticket.RiskRetestHandoffPath, bytes.NewReader(body)).WithContext(f.ctx))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var outcome struct {
		Data struct {
			OperationID string `json:"operation_id"`
			SuccessorID string `json:"successor_user_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &outcome))
	require.Equal(t, c.OperationID, outcome.Data.OperationID)
	require.Equal(t, stringID(c.SuccessorUserID), outcome.Data.SuccessorID)
}
