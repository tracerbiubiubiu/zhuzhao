//go:build integration

package riskworkflow

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

type workflowHTTPFixture struct {
	*ReceiptService
	writer     *Service
	assignment *AssignmentService
}

func (f workflowHTTPFixture) Finish(ctx context.Context, cmd riskremediation.FinishCommand, input ticket.RiskWorkflowTicketInput) (riskremediation.Result, error) {
	return f.writer.Finish(ctx, FinishCommand{Command: cmd.Command, Items: cmd.Items, TicketInput: input})
}

func (f workflowHTTPFixture) Review(ctx context.Context, cmd riskremediation.ReviewCommand, input ticket.RiskWorkflowTicketInput) (riskremediation.Result, error) {
	return f.writer.Review(ctx, ReviewCommand{Command: cmd.Command, Items: cmd.Items, TicketInput: input})
}

func (f workflowHTTPFixture) RequestAcceptance(ctx context.Context, cmd riskremediation.AcceptanceRequest, input ticket.RiskWorkflowTicketInput) (riskremediation.Result, error) {
	return f.writer.RequestAcceptance(ctx, AcceptanceRequestCommand{AcceptanceRequest: cmd, TicketInput: input})
}

func (f workflowHTTPFixture) DecideAcceptance(ctx context.Context, cmd riskremediation.AcceptanceDecision, input ticket.RiskWorkflowTicketInput) (riskremediation.Result, error) {
	return f.writer.DecideAcceptance(ctx, AcceptanceDecisionCommand{AcceptanceDecision: cmd, TicketInput: input})
}

func (f workflowHTTPFixture) AssignHandler(ctx context.Context, cmd riskremediation.Command, assignee int64, input ticket.RiskWorkflowTicketInput) (riskremediation.Result, error) {
	return f.assignment.AssignHandler(ctx, AssignHandlerCommand{AssignmentCommand: fromHTTPAssignment(cmd, input), AssignedToUserID: assignee})
}

func (f workflowHTTPFixture) AcceptAssignment(ctx context.Context, cmd riskremediation.Command, input ticket.RiskWorkflowTicketInput) (riskremediation.Result, error) {
	return f.assignment.AcceptAssignment(ctx, fromHTTPAssignment(cmd, input))
}

func fromHTTPAssignment(cmd riskremediation.Command, input ticket.RiskWorkflowTicketInput) AssignmentCommand {
	return AssignmentCommand{ActorID: cmd.ActorID, OperationID: cmd.OperationID, TicketID: cmd.TicketID,
		ExpectedTicketRevision: cmd.ExpectedTicketRevision, TicketInput: input}
}

func TestWorkflowHTTPExpiredPendingAcceptanceCanBeRequestedAgain(t *testing.T) {
	f := newAcceptanceFixture(t)
	pending, err := f.requestAcceptance(t, f.acceptanceRequest(t, f.riskA))
	require.NoError(t, err)
	actor := f.other
	var epoch int64
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT session_epoch FROM users WHERE id=$1`, actor).Scan(&epoch))
	_, err = workflowPool.Exec(f.ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,'GET')`,
		"role::"+f.role, contextPrefix+":id")
	require.NoError(t, err)
	_, err = f.enforcer.AddPolicy("role::"+f.role, contextPrefix+":id", "GET")
	require.NoError(t, err)
	h := handler.NewRiskWorkflowHandler(workflowHTTPFixture{ReceiptService: NewReceiptService(workflowPool, f.tickets)})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", actor)
		c.Set("session_epoch", epoch)
		c.Next()
	}, middleware.CasbinAuth(f.enforcer, databaseRoles{workflowPool}, nil))
	r.GET(contextPrefix+":id", h.Current)
	read := func() map[string]any {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, contextPrefix+strconv.FormatInt(f.ticket, 10), nil).WithContext(f.ctx))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var body struct {
			Data struct{ Items []map[string]any }
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		for _, item := range body.Data.Items {
			if item["risk_id"] == strconv.FormatInt(f.riskA, 10) {
				return item
			}
		}
		t.Fatal("missing risk item in context")
		return nil
	}
	active := read()
	require.Equal(t, strconv.FormatInt(pending.AcceptanceID, 10), active["pending_acceptance_id"])
	require.NotContains(t, active, "pending_acceptance_recoverable")

	_, err = workflowPool.Exec(f.ctx, `UPDATE risk_acceptances SET valid_until=clock_timestamp()-interval '1 second' WHERE id=$1`, pending.AcceptanceID)
	require.NoError(t, err)
	before := f.acceptanceSnapshot(t)
	expired := read()
	require.Equal(t, "pending_acceptance", expired["remediation_state"])
	require.Equal(t, true, expired["pending_acceptance_recoverable"])
	require.NotContains(t, expired, "pending_acceptance_id")
	require.Equal(t, before, f.acceptanceSnapshot(t), "context only projects expiry")

	next := f.acceptanceRequest(t, f.riskA)
	next.OperationID += "-renewal"
	out, err := f.requestAcceptance(t, next)
	require.NoError(t, err)
	require.NotEqual(t, pending.AcceptanceID, out.AcceptanceID)
	require.Equal(t, "pending", out.AcceptanceStatus)
	var oldStatus string
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT status FROM risk_acceptances WHERE id=$1`, pending.AcceptanceID).Scan(&oldStatus))
	require.Equal(t, "expired", oldStatus)
}

func TestWorkflowHTTPRevisionAndOriginalReceipt(t *testing.T) {
	f := newWriteFixture(t)
	actor := f.other
	var epoch int64
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT session_epoch FROM users WHERE id=$1`, actor).Scan(&epoch))
	_, err := workflowPool.Exec(f.ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,'GET')`,
		"role::"+f.role, contextPrefix+":id")
	require.NoError(t, err)
	_, err = f.enforcer.AddPolicy("role::"+f.role, contextPrefix+":id", "GET")
	require.NoError(t, err)
	ports := workflowHTTPFixture{ReceiptService: NewReceiptService(workflowPool, f.tickets), writer: f.service}
	h := handler.NewRiskWorkflowHandler(ports)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", actor)
		c.Set("session_epoch", epoch)
		c.Next()
	}, middleware.CasbinAuth(f.enforcer, databaseRoles{workflowPool}, nil))
	r.GET(contextPrefix+":id", h.Current)
	r.GET(receiptPathPrefix+":id", h.Receipt)
	r.POST(finishPath, h.Finish)
	request := func(method, path string, body any) *httptest.ResponseRecorder {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		var reader *bytes.Reader
		if body != nil {
			reader = bytes.NewReader(raw)
		} else {
			reader = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, path, reader).WithContext(f.ctx)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		return res
	}
	contextPath := contextPrefix + strconv.FormatInt(f.ticket, 10)
	res := request(http.MethodGet, contextPath, nil)
	require.Equal(t, 200, res.Code, res.Body.String())
	var entry struct {
		Data model.RiskWorkflowContext
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &entry))
	require.Equal(t, "in_progress", entry.Data.Status)
	require.Equal(t, f.version, entry.Data.DefinitionVersion)
	require.Len(t, entry.Data.Items, 2)
	body := map[string]any{
		"operation_id": uuid.NewString(), "ticket_id": strconv.FormatInt(f.ticket, 10),
		"expected_ticket_revision": strconv.FormatInt(entry.Data.TicketRevision, 10),
		"definition_version":       strconv.FormatInt(entry.Data.DefinitionVersion, 10), "comment": "Synthetic HTTP submit",
	}
	items := []map[string]any{}
	for _, item := range entry.Data.Items {
		items = append(items, map[string]any{
			"link_id": strconv.FormatInt(item.LinkID, 10), "expected_link_revision": strconv.FormatInt(item.LinkRevision, 10),
			"expected_risk_revision": strconv.FormatInt(item.RiskRevision, 10), "resolution": "fixed",
			"measures": "Synthetic HTTP measure", "evidence": f.evidence,
		})
	}
	body["items"] = items
	body["ticket_binding"] = "externally supplied"
	before := f.writeSnapshot(t)
	res = request(http.MethodPost, finishPath, body)
	require.Equal(t, 400, res.Code)
	require.Equal(t, before, f.writeSnapshot(t))
	delete(body, "ticket_binding")

	res = request(http.MethodPost, finishPath, body)
	require.Equal(t, 200, res.Code, res.Body.String())
	getReceipt := request(http.MethodGet, receiptPathPrefix+body["operation_id"].(string), nil)
	require.Equal(t, 200, getReceipt.Code, getReceipt.Body.String())
	var applied, receipt struct{ Data json.RawMessage }
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &applied))
	require.NoError(t, json.Unmarshal(getReceipt.Body.Bytes(), &receipt))
	require.JSONEq(t, string(applied.Data), string(receipt.Data))
	committed := f.writeSnapshot(t)
	require.Equal(t, 200, request(http.MethodGet, receiptPathPrefix+body["operation_id"].(string), nil).Code)
	require.Equal(t, committed, f.writeSnapshot(t))
}
