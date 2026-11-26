//go:build integration

package riskworkflow

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

const workflowBucket = "synthetic-workflow-evidence"

type writeFixture struct {
	*receiptFixture
	version  int64
	evidence []riskremediation.EvidenceRef
	service  *Service
}

func newWriteFixture(t *testing.T) *writeFixture { return newWriteFixtureWithCondition(t, "assignee") }
func newWriteFixtureWithCondition(t *testing.T, condition string) *writeFixture {
	t.Helper()
	return newWriteFixtureWithRoles(t, condition, "creator", "creator")
}

func newWriteFixtureWithRoles(t *testing.T, finishRole, approveRole, rejectRole string) *writeFixture {
	t.Helper()
	f := newPreparedReceiptFixture(t, func(f *receiptFixture, code string) {
		_, err := f.tickets.ReplaceTicketTypeFields(f.ctx, code, &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{{FieldKey: "review_mark", FieldLabel: "Review mark", FieldType: "input"}}})
		require.NoError(t, err)
		_, err = f.tickets.UpdateTicketType(f.ctx, code, &model.UpdateTicketTypeRequest{Code: code,
			Transitions:    json.RawMessage(`{"open":["assigned"],"assigned":["in_progress"],"in_progress":["pending_verify"],"pending_verify":["pending_verify","closed","in_progress"],"closed":[]}`),
			TransitionMeta: json.RawMessage(`{"in_progress>pending_verify":{"roles":["` + finishRole + `"],"comment_required":true},"pending_verify>pending_verify":{"roles":["creator"],"collect_fields":[{"key":"review_mark","required":true}]},"pending_verify>closed":{"roles":["` + approveRole + `"],"collect_fields":[{"key":"review_mark","required":true}]},"pending_verify>in_progress":{"roles":["` + rejectRole + `"],"collect_fields":[{"key":"review_mark","required":true}]}}`),
		})
		require.NoError(t, err)
	}, func(f *receiptFixture, tk *model.Ticket) {
		_, err := workflowPool.Exec(f.ctx, `UPDATE user_orgs SET ticket_scope='group' WHERE user_id=$1`, f.actor)
		require.NoError(t, err)
		_, err = f.tickets.Update(f.ctx, &model.UpdateTicketRequest{ID: tk.ID, CustomData: json.RawMessage(`{"review_mark":"initial"}`)}, f.actor)
		require.NoError(t, err)
		// U assigns P before the risk bridge takes ownership of the ticket lifecycle.
		require.NoError(t, f.tickets.Assign(f.ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &f.other}, f.actor))
		moved, err := f.tickets.Transition(f.ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "in_progress"}, f.other)
		require.NoError(t, err)
		*tk = *moved
	})
	// Historical fixture topology is now U-owned with P as the current handler.
	_, err := workflowPool.Exec(f.ctx, `UPDATE risk_items SET owner_user_id=$1 WHERE id=ANY($2)`, f.actor, []int64{f.riskA, f.riskB})
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `UPDATE risk_ticket_links SET owner_at_link=$1 WHERE risk_id=ANY($2)`, f.actor, []int64{f.riskA, f.riskB})
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `UPDATE risk_dispatch_groups SET owner_user_id=$1 WHERE ticket_id=$2`, f.actor, f.ticket)
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE code=$2`, f.other, f.role)
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES($1,$2),($1,$3)`, f.other, f.orgA, f.orgB)
	require.NoError(t, err)
	for _, route := range []struct{ path, method string }{{finishPath, "POST"}, {reviewPath, "POST"}, {"/api/v1/tickets/transition", "POST"}} {
		_, err = workflowPool.Exec(f.ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,$3)`, "role::"+f.role, route.path, route.method)
		require.NoError(t, err)
		_, err = f.enforcer.AddPolicy("role::"+f.role, route.path, route.method)
		require.NoError(t, err)
	}
	tk, err := repository.NewTicketRepo(workflowPool).GetByID(f.ctx, f.ticket)
	require.NoError(t, err)
	w := &writeFixture{receiptFixture: f, version: *tk.DefinitionVersion}
	repo := repository.NewStorageRepo(workflowPool)
	obj := &model.FileObject{Bucket: workflowBucket, ObjectKey: "tickets/" + f.role, Purpose: "ticket_attachment", Status: "pending", FileName: "synthetic.txt", MIMEType: "text/plain", CreatedBy: f.other}
	require.NoError(t, repo.InsertPending(f.ctx, obj))
	require.NoError(t, repo.ConfirmAndBind(f.ctx, obj.ID, f.ticket, f.other, 10, "stable-version", 20))
	w.evidence = []riskremediation.EvidenceRef{{Kind: "file", Ref: obj.ObjectKey, Version: "stable-version"}}
	w.service = NewService(workflowPool, f.tickets, storageservice.NewBoundTicketEvidenceValidator(workflowPool, workflowBucket))
	return w
}
func (f *writeFixture) admittedWrite(t *testing.T, actor int64, path string, before func(), run func(context.Context) (riskremediation.Result, error)) (riskremediation.Result, error) {
	t.Helper()
	var epoch int64
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT session_epoch FROM users WHERE id=$1`, actor).Scan(&epoch))
	var got riskremediation.Result
	var result error
	called := false
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", actor); c.Set("session_epoch", epoch); c.Next() }, middleware.CasbinAuth(f.enforcer, databaseRoles{workflowPool}, nil))
	r.POST(path, func(c *gin.Context) {
		called = true
		if before != nil {
			before()
		}
		got, result = run(c.Request.Context())
	})
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("POST", path, nil).WithContext(f.ctx))
	require.True(t, called, "middleware positive control must admit, status=%d", response.Code)
	return got, result
}
func (f *writeFixture) finishCommand(t *testing.T) FinishCommand {
	tk, err := repository.NewTicketRepo(workflowPool).GetByID(f.ctx, f.ticket)
	require.NoError(t, err)
	c := FinishCommand{TicketInput: ticket.RiskWorkflowTicketInput{DefinitionVersion: f.version, Comment: "Synthetic finish"}, Command: riskremediation.Command{ActorID: f.other, TicketID: f.ticket, ExpectedTicketRevision: tk.Revision, OperationID: "finish-" + f.role}}
	for _, item := range f.want.Items {
		c.Items = append(c.Items, riskremediation.Item{LinkID: item.LinkID, ExpectedLinkRevision: 1, ExpectedRiskRevision: 1, Resolution: "fixed", Measures: "Synthetic repair", Evidence: f.evidence})
	}
	return c
}
func (f *writeFixture) reviewCommand(t *testing.T, after riskremediation.Result, indexes ...int) ReviewCommand {
	c := ReviewCommand{TicketInput: ticket.RiskWorkflowTicketInput{DefinitionVersion: f.version, CustomDataPatch: json.RawMessage(`{"review_mark":"asset-` + strconv.Itoa(indexes[0]) + `"}`), Comment: "Synthetic review"}, Command: riskremediation.Command{ActorID: f.actor, TicketID: f.ticket, ExpectedTicketRevision: after.TicketRevision, OperationID: "review-" + f.role + strconv.Itoa(indexes[0])}}
	for _, i := range indexes {
		item := after.Items[i]
		c.Items = append(c.Items, riskremediation.ReviewItem{LinkID: item.LinkID, ExpectedLinkRevision: item.LinkRevision, ExpectedRiskRevision: item.RiskRevision, ExpectedProposalVersion: item.ProposalVersion, Decision: "approve", Note: "Synthetic confirmed", Evidence: f.evidence})
	}
	return c
}
func TestWorkflowFinishAndPerItemReviewRealWrites(t *testing.T) {
	f := newWriteFixture(t)
	cmd := f.finishCommand(t)
	finished, err := f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Finish(ctx, cmd) })
	require.NoError(t, err)
	require.False(t, finished.TerminalAllowed)
	require.Len(t, finished.Items, 2)
	require.Equal(t, "pending_confirmation", finished.Items[0].Result)
	var state string
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT status FROM tickets WHERE id=$1`, f.ticket).Scan(&state))
	require.Equal(t, "pending_verify", state)
	first := f.reviewCommand(t, finished, 0)
	reviewed, err := f.admittedWrite(t, f.actor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Review(ctx, first) })
	require.NoError(t, err)
	require.False(t, reviewed.TerminalAllowed)
	require.Greater(t, reviewed.TicketRevision, finished.TicketRevision)
	// Only the requested asset is closed; pending sibling keeps the ticket live.
	var statuses []string
	rows, err := workflowPool.Query(f.ctx, `SELECT status FROM risk_items WHERE id=ANY($1) ORDER BY id`, []int64{f.riskA, f.riskB})
	require.NoError(t, err)
	statuses, err = pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	require.Equal(t, []string{"closed", "pending_verify"}, statuses)
	next := f.reviewCommand(t, finished, 1)
	next.ExpectedTicketRevision = reviewed.TicketRevision
	closed, err := f.admittedWrite(t, f.actor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Review(ctx, next) })
	require.NoError(t, err)
	require.True(t, closed.TerminalAllowed)
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT status FROM tickets WHERE id=$1`, f.ticket).Scan(&state))
	require.Equal(t, "closed", state)
	var ended bool
	var openLinks int
	require.NoError(t, workflowPool.QueryRow(f.ctx, `
		SELECT g.ended_at IS NOT NULL,
		       (SELECT count(*) FROM risk_ticket_links l WHERE l.group_id=g.id AND l.ended_at IS NULL)
		FROM risk_dispatch_groups g WHERE g.ticket_id=$1
	`, f.ticket).Scan(&ended, &openLinks))
	require.True(t, ended, "a closed ticket must vacate its live dispatch group")
	require.Zero(t, openLinks, "a closed ticket must preserve only historical links")
	before := f.writeSnapshot(t)
	replay, err := f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Finish(ctx, cmd) })
	require.NoError(t, err)
	require.Equal(t, finished, replay)
	require.Equal(t, before, f.writeSnapshot(t))
	for _, check := range []struct {
		table string
		want  int
	}{{"risk_remediation_submissions", 2}, {"risk_remediation_reviews", 2}, {"risk_remediation_receipts", 3}, {"risk_remediation_intents", 4}} {
		var n int
		query := `SELECT count(*) FROM ` + check.table + ` WHERE `
		switch check.table {
		case "risk_remediation_submissions":
			query += `risk_id=ANY($1)`
		case "risk_remediation_reviews":
			query += `submission_id IN (SELECT id FROM risk_remediation_submissions WHERE risk_id=ANY($1))`
		default:
			query += `ticket_id=$1`
		}
		var param any = f.ticket
		if check.table == "risk_remediation_reviews" || check.table == "risk_remediation_submissions" {
			param = []int64{f.riskA, f.riskB}
		}
		require.NoError(t, workflowPool.QueryRow(f.ctx, query, param).Scan(&n))
		if check.table == "risk_remediation_receipts" {
			check.want++ /* historic fixture receipt */
		}
		require.Equal(t, check.want, n, check.table)
	}
}

func TestWorkflowBindingReplayAndReject(t *testing.T) {
	f := newWriteFixture(t)
	cmd := f.finishCommand(t)
	finished, err := f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Finish(ctx, cmd) })
	require.NoError(t, err)
	first := f.reviewCommand(t, finished, 0)
	reviewed, err := f.admittedWrite(t, f.actor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Review(ctx, first) })
	require.NoError(t, err)
	before := f.writeSnapshot(t)
	first.TicketInput.CustomDataPatch = json.RawMessage(` { "review_mark" : "asset-0" } `)
	replay, err := f.admittedWrite(t, f.actor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Review(ctx, first) })
	require.NoError(t, err)
	require.Equal(t, reviewed, replay)
	for _, change := range []string{"version", "patch", "comment", "caller binding"} {
		changed := first
		switch change {
		case "version":
			changed.TicketInput.DefinitionVersion++
		case "patch":
			changed.TicketInput.CustomDataPatch = json.RawMessage(`{"review_mark":"changed"}`)
		case "comment":
			changed.TicketInput.Comment += "!"
		case "caller binding":
			changed.TicketBinding = "caller-controlled"
		}
		got, err := f.admittedWrite(t, f.actor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Review(ctx, changed) })
		require.Error(t, err, change)
		require.Empty(t, got)
		require.Equal(t, before, f.writeSnapshot(t))
	}
	rejected := f.reviewCommand(t, finished, 1)
	rejected.ExpectedTicketRevision = reviewed.TicketRevision
	rejected.Items[0].Decision = "reject"
	result, err := f.admittedWrite(t, f.actor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Review(ctx, rejected) })
	require.NoError(t, err)
	require.Equal(t, "rejected", result.Items[0].Result)
	require.False(t, result.TerminalAllowed)
	var state string
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT status FROM tickets WHERE id=$1`, f.ticket).Scan(&state))
	require.Equal(t, "in_progress", state)
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT status FROM risk_items WHERE id=$1`, f.riskB).Scan(&state))
	require.Equal(t, "remediating", state)
	// P may repair the rejected asset again; original submission/review remain.
	again := f.finishCommand(t)
	again.OperationID += "-repair-again"
	again.Items = []riskremediation.Item{{LinkID: result.Items[0].LinkID, ExpectedLinkRevision: result.Items[0].LinkRevision, ExpectedRiskRevision: result.Items[0].RiskRevision, Resolution: "fixed", Measures: "Synthetic second repair", Evidence: f.evidence}}
	resubmitted, err := f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Finish(ctx, again) })
	require.NoError(t, err)
	require.Equal(t, "pending_confirmation", resubmitted.Items[0].Result)
	require.Greater(t, resubmitted.Items[0].ProposalVersion, result.Items[0].ProposalVersion)
	var submissions, reviews int
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM risk_remediation_submissions WHERE risk_id=$1), (SELECT count(*) FROM risk_remediation_reviews WHERE submission_id IN (SELECT id FROM risk_remediation_submissions WHERE risk_id=$1))`, f.riskB).Scan(&submissions, &reviews))
	require.Equal(t, 2, submissions)
	require.Equal(t, 1, reviews)
}

func TestWorkflowBoundInputAndCurrentReceiptScope(t *testing.T) {
	f := newWriteFixture(t)
	cmd := f.finishCommand(t)
	finished, err := f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Finish(ctx, cmd) })
	require.NoError(t, err)
	before := f.writeSnapshot(t)
	for _, field := range []string{"version", "patch", "comment"} {
		changed := cmd
		switch field {
		case "version":
			changed.TicketInput.DefinitionVersion++
		case "patch":
			changed.TicketInput.CustomDataPatch = json.RawMessage(`{"review_mark":"changed"}`)
		case "comment":
			changed.TicketInput.Comment += "!"
		}
		got, err := f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Finish(ctx, changed) })
		require.ErrorIs(t, err, errcode.ErrConflict, field)
		require.Empty(t, got)
		require.Equal(t, before, f.writeSnapshot(t))
	}
	// Recover the newly committed real operation; read/replay require original
	// ticket visibility even when every original risk is still readable.
	f.actor = f.other
	got, err := f.read(t, cmd.OperationID, nil)
	require.NoError(t, err)
	require.Equal(t, finished, got)
	// Revoke only the original ticket's L1 policy; risk route remains admitted.
	_, err = workflowPool.Exec(f.ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1='/api/v1/tickets/:id'`, "role::"+f.role)
	require.NoError(t, err)
	got, err = f.read(t, cmd.OperationID, nil)
	require.Error(t, err)
	require.Empty(t, got)
	got, err = f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Finish(ctx, cmd) })
	require.Error(t, err)
	require.Empty(t, got)
}

func TestWorkflowRealTicketWriteRollsBackChildOnSQLFailure(t *testing.T) {
	f := newWriteFixture(t)
	cmd := f.finishCommand(t)
	input, binding, err := ticket.PrepareRiskWorkflowTicketInput(cmd.TicketInput)
	require.NoError(t, err)
	cmd.TicketBinding = binding
	before := f.writeSnapshot(t)
	childCommitted, outerCommitted, applied := false, false, false
	_, err = f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) {
		err := repository.WithIAMCommandGuard(ctx, workflowPool, func(ctx context.Context, outer pgx.Tx) error {
			callbacks := f.service.writeCallbacks(f.other, finishPath, input)
			apply := callbacks.ApplyTicket
			callbacks.ApplyTicket = func(ctx context.Context, child pgx.Tx, effect riskremediation.TicketEffect) (int64, error) {
				revision, err := apply(ctx, child, effect)
				if err != nil {
					return 0, err
				}
				applied = true
				var submissions, events, receipts int
				require.NoError(t, child.QueryRow(ctx, `SELECT
				 (SELECT count(*) FROM risk_remediation_submissions WHERE risk_id=ANY($1)),
				 (SELECT count(*) FROM ticket_events WHERE ticket_id=$2),
				 (SELECT count(*) FROM ticket_evidence_transition_receipts WHERE ticket_id=$2)`, []int64{f.riskA, f.riskB}, f.ticket).Scan(&submissions, &events, &receipts))
				require.Equal(t, 2, submissions)
				require.Positive(t, events)
				require.Equal(t, 1, receipts)
				require.NoError(t, repository.AfterIAMCommit(ctx, func() error { childCommitted = true; return nil }))
				_, err = child.Exec(ctx, `SELECT 1/0`)
				return revision, err
			}
			_, err := riskremediation.New(workflowPool, callbacks).FinishTx(ctx, outer, riskremediation.FinishCommand{Command: cmd.Command, Items: cmd.Items})
			require.Error(t, err)
			var one int
			require.NoError(t, outer.QueryRow(ctx, `SELECT 1`).Scan(&one), "aborted SQL child must restore usable outer transaction")
			return repository.AfterIAMCommit(ctx, func() error { outerCommitted = true; return nil })
		})
		return riskremediation.Result{}, err
	})
	require.NoError(t, err)
	require.True(t, applied)
	require.True(t, outerCommitted)
	require.False(t, childCommitted)
	require.Equal(t, before, f.writeSnapshot(t))
}

func TestWorkflowRejectConflictDoesNotChooseRejectEdge(t *testing.T) {
	f := newWriteFixture(t)
	cmd := f.finishCommand(t)
	finished, err := f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Finish(ctx, cmd) })
	require.NoError(t, err)
	c := f.reviewCommand(t, finished, 0, 1)
	c.Items[1].Decision = "reject"
	c.Items[1].ExpectedRiskRevision++
	got, err := f.admittedWrite(t, f.actor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Review(ctx, c) })
	require.NoError(t, err)
	require.Equal(t, "applied", got.Items[0].Result)
	require.Equal(t, "conflict", got.Items[1].Result)
	var state string
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT status FROM tickets WHERE id=$1`, f.ticket).Scan(&state))
	require.Equal(t, "pending_verify", state)
}

func TestWorkflowTicketInputsFailClosedWithoutBusinessMutation(t *testing.T) {
	for _, kind := range []string{"foreign ticket file", "wrong pin", "missing pin", "uncollected patch", "self edge unchanged"} {
		t.Run(kind, func(t *testing.T) {
			f := newWriteFixture(t)
			cmd := f.finishCommand(t)
			switch kind {
			case "foreign ticket file":
				foreign := newWriteFixture(t)
				cmd.Items[0].Evidence = foreign.evidence
			case "wrong pin":
				cmd.TicketInput.DefinitionVersion++
			case "missing pin":
				cmd.TicketInput.DefinitionVersion = 0
			case "uncollected patch":
				cmd.TicketInput.CustomDataPatch = json.RawMessage(`{"review_mark":"not permitted on finish"}`)
			}
			if kind == "self edge unchanged" {
				finished, err := f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Finish(ctx, cmd) })
				require.NoError(t, err)
				first := f.reviewCommand(t, finished, 0)
				first.TicketInput.CustomDataPatch = json.RawMessage(`{"review_mark":"initial"}`)
				before := f.writeSnapshot(t)
				got, err := f.admittedWrite(t, f.actor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Review(ctx, first) })
				require.ErrorIs(t, err, errcode.ErrConcurrentModification)
				require.Empty(t, got)
				require.Equal(t, before, f.writeSnapshot(t))
				return
			}
			before := f.writeSnapshot(t)
			got, err := f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Finish(ctx, cmd) })
			require.Error(t, err)
			require.Empty(t, got)
			require.Equal(t, before, f.writeSnapshot(t))
		})
	}
}

func TestWorkflowIncompleteFinishCannotStrandUnsubmittedSibling(t *testing.T) {
	for _, mode := range []string{"omitted sibling", "conflicted sibling"} {
		t.Run(mode, func(t *testing.T) {
			f := newWriteFixture(t)
			cmd := f.finishCommand(t)
			if mode == "omitted sibling" {
				cmd.Items = cmd.Items[:1]
			} else {
				cmd.Items[1].ExpectedRiskRevision++
			}
			before := f.writeSnapshot(t)
			got, err := f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Finish(ctx, cmd) })
			require.ErrorIs(t, err, errcode.ErrConflict)
			require.Empty(t, got)
			require.Equal(t, before, f.writeSnapshot(t))
		})
	}
}
func TestWorkflowFinishRollbackOnEvidenceAndPublishedCondition(t *testing.T) {
	for _, failure := range []string{"wrong version", "published condition", "U cannot finish", "M cannot finish", "M cannot review", "P cannot review", "revoked policy", "disabled actor"} {
		t.Run(failure, func(t *testing.T) {
			f := newWriteFixture(t)
			if failure == "published condition" {
				f = newWriteFixtureWithCondition(t, "creator")
			}
			cmd := f.finishCommand(t)
			actor := f.other
			if failure == "M cannot finish" || failure == "M cannot review" {
				require.NoError(t, workflowPool.QueryRow(f.ctx, `INSERT INTO users(username,password,status) VALUES($1,'synthetic',1) RETURNING id`, "manager-"+f.role).Scan(&actor))
				_, e := workflowPool.Exec(f.ctx, `INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE code=$2`, actor, f.role)
				require.NoError(t, e)
				_, e = workflowPool.Exec(f.ctx, `INSERT INTO user_orgs(user_id,org_id,org_member_role) VALUES($1,$2,'admin'),($1,$3,'admin')`, actor, f.orgA, f.orgB)
				require.NoError(t, e)
				cmd.ActorID = actor
			}
			var before func()
			switch failure {
			case "wrong version":
				cmd.Items[1].Evidence = []riskremediation.EvidenceRef{{Kind: "file", Ref: f.evidence[0].Ref, Version: "wrong"}}
			case "U cannot finish":
				actor = f.actor
				cmd.ActorID = actor
			case "revoked policy":
				before = func() {
					_, err := workflowPool.Exec(f.ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1=$2`, "role::"+f.role, finishPath)
					require.NoError(t, err)
				}
			case "disabled actor":
				before = func() {
					_, err := workflowPool.Exec(f.ctx, `UPDATE users SET status=0 WHERE id=$1`, actor)
					require.NoError(t, err)
				}
			}
			if failure == "P cannot review" || failure == "M cannot review" {
				reviewer := actor
				actor = f.other
				cmd.ActorID = actor
				fin, err := f.admittedWrite(t, actor, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Finish(ctx, cmd) })
				require.NoError(t, err)
				c := f.reviewCommand(t, fin, 0)
				c.ActorID = reviewer
				snap := f.writeSnapshot(t)
				got, err := f.admittedWrite(t, reviewer, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Review(ctx, c) })
				require.ErrorIs(t, err, errcode.ErrNoPermission)
				require.Empty(t, got)
				require.Equal(t, snap, f.writeSnapshot(t))
				return
			}
			snap := f.writeSnapshot(t)
			got, err := f.admittedWrite(t, actor, finishPath, before, func(ctx context.Context) (riskremediation.Result, error) { return f.service.Finish(ctx, cmd) })
			require.Error(t, err)
			require.Empty(t, got)
			require.Equal(t, snap, f.writeSnapshot(t))
		})
	}
}

// Cover every command fact, owning ticket history/receipt and attachment binding.
// IAM fixture revocations are deliberately excluded from business snapshots.
func (f *writeFixture) writeSnapshot(t *testing.T) string {
	t.Helper()
	var result string
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT jsonb_build_object(
 'ticket',(SELECT to_jsonb(t) FROM tickets t WHERE id=$1),
 'ticket_comments',(SELECT jsonb_agg(to_jsonb(c) ORDER BY id) FROM ticket_comments c WHERE ticket_id=$1),
 'ticket_events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM ticket_events e WHERE ticket_id=$1),
 'ticket_comments',(SELECT jsonb_agg(to_jsonb(c) ORDER BY id) FROM ticket_comments c WHERE ticket_id=$1),
 'ticket_receipts',(SELECT jsonb_agg(to_jsonb(e) ORDER BY actor_id,idempotency_key) FROM ticket_evidence_transition_receipts e WHERE ticket_id=$1),
 'risk',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM risk_items r WHERE id=ANY($2)),
 'links',(SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM risk_ticket_links l WHERE risk_id=ANY($2)),
 'submissions',(SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM risk_remediation_submissions s WHERE risk_id=ANY($2)),
 'reviews',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM risk_remediation_reviews r WHERE submission_id IN (SELECT id FROM risk_remediation_submissions WHERE risk_id=ANY($2))),
 'risk_events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM risk_events e WHERE risk_id=ANY($2)),
 'operations',(SELECT jsonb_agg(to_jsonb(o) ORDER BY actor_id,operation_id) FROM risk_operations o WHERE actor_id=ANY($3)),
 'receipts',(SELECT jsonb_agg(to_jsonb(r) ORDER BY actor_id,operation_id) FROM risk_remediation_receipts r WHERE ticket_id=$1),
 'intents',(SELECT jsonb_agg(to_jsonb(i) ORDER BY id) FROM risk_remediation_intents i WHERE ticket_id=$1),
 'bindings',(SELECT jsonb_agg(to_jsonb(a) ORDER BY file_object_id) FROM ticket_attachments a WHERE ticket_id=$1),
 'files',(SELECT jsonb_agg(to_jsonb(f) ORDER BY id) FROM file_objects f WHERE id IN (SELECT file_object_id FROM ticket_attachments WHERE ticket_id=$1))
 )::text`, f.ticket, []int64{f.riskA, f.riskB}, []int64{f.actor, f.other}).Scan(&result))
	return result
}
