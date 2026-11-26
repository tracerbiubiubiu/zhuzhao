//go:build integration

package riskworkflow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

const acceptanceReworkTestRoute = "/api/v1/risk-items/remediation-accept-rework"

func newAcceptanceReworkFixture(t *testing.T) (*writeFixture, riskremediation.AcceptanceRework) {
	t.Helper()
	f := newAcceptanceFixture(t)
	_, err := workflowPool.Exec(f.ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,'POST')`, "role::"+f.role, acceptanceReworkTestRoute)
	require.NoError(t, err)
	_, err = f.enforcer.AddPolicy("role::"+f.role, acceptanceReworkTestRoute, "POST")
	require.NoError(t, err)
	pending, err := f.requestAcceptance(t, f.acceptanceRequest(t, f.riskA))
	require.NoError(t, err)
	approved, err := f.decideAcceptance(t, f.acceptanceDecision(t, pending))
	require.NoError(t, err)
	require.False(t, approved.TerminalAllowed)
	require.Len(t, approved.Items, 1)
	item := approved.Items[0]
	return f, riskremediation.AcceptanceRework{
		Command: riskremediation.Command{
			ActorID: f.actor, TicketID: f.ticket, ExpectedTicketRevision: approved.TicketRevision,
			OperationID: "accept-rework-" + f.role,
		},
		RiskID: item.RiskID, ExpectedRiskRevision: item.RiskRevision,
		LinkID: item.LinkID, ExpectedLinkRevision: item.LinkRevision,
		AcceptanceID: approved.AcceptanceID, ExpectedAcceptanceRevision: approved.AcceptanceRevision,
		Reason: "Synthetic approved risk validity lapsed",
	}
}

func (f *writeFixture) reworkAcceptance(t *testing.T, c riskremediation.AcceptanceRework, input ticket.RiskWorkflowTicketInput, afterAdmission func()) (riskremediation.Result, error) {
	t.Helper()
	return f.admittedWrite(t, c.ActorID, acceptanceReworkTestRoute, afterAdmission, func(ctx context.Context) (riskremediation.Result, error) {
		return f.service.ReworkAcceptance(ctx, c, input)
	})
}

func (f *writeFixture) expireApprovedRisk(t *testing.T, acceptanceID int64) {
	t.Helper()
	updated, err := workflowPool.Exec(f.ctx, `WITH lapsed AS (
		UPDATE risk_acceptances SET valid_until=clock_timestamp()-interval '1 second'
		WHERE id=$1 AND risk_id=$2 AND status='approved' RETURNING risk_id, valid_until
	) UPDATE risk_items r SET accepted_until=lapsed.valid_until FROM lapsed WHERE r.id=lapsed.risk_id`, acceptanceID, f.riskA)
	require.NoError(t, err)
	require.EqualValues(t, 1, updated.RowsAffected())
}

func (f *writeFixture) acceptanceReworkPreservedFacts(t *testing.T, acceptanceID int64) string {
	t.Helper()
	var result string
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT jsonb_build_object(
		'approval',(SELECT to_jsonb(a) FROM risk_acceptances a WHERE id=$1),
		'sibling_risk',(SELECT to_jsonb(r) FROM risk_items r WHERE id=$2),
		'sibling_link',(SELECT to_jsonb(l) FROM risk_ticket_links l WHERE id=$3),
		'submissions',(SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM risk_remediation_submissions s WHERE risk_id=$4)
	)::text`, acceptanceID, f.riskB, f.linkB, f.riskA).Scan(&result))
	return result
}

func TestWorkflowAcceptanceReworkUsesPinnedPendingVerifyExit(t *testing.T) {
	f, c := newAcceptanceReworkFixture(t)
	input := ticket.RiskWorkflowTicketInput{
		DefinitionVersion: f.version, CustomDataPatch: json.RawMessage(`{"review_mark":"rework"}`),
		Comment: "Synthetic control owner restarts remediation",
	}
	before := f.acceptanceSnapshot(t)
	got, err := f.reworkAcceptance(t, c, input, nil)
	require.ErrorIs(t, err, errcode.ErrConflict)
	require.Empty(t, got)
	require.Equal(t, before, f.acceptanceSnapshot(t))

	f.expireApprovedRisk(t, c.AcceptanceID)
	preserved := f.acceptanceReworkPreservedFacts(t, c.AcceptanceID)
	before = f.acceptanceSnapshot(t)
	for _, tc := range []string{"wrong link", "unknown approval", "wrong pin", "uncollected field"} {
		t.Run(tc, func(t *testing.T) {
			invalidCommand, invalidInput := c, input
			switch tc {
			case "wrong link":
				invalidCommand.LinkID = f.linkB
			case "unknown approval":
				invalidCommand.AcceptanceID = 1 << 62
			case "wrong pin":
				invalidInput.DefinitionVersion++
			case "uncollected field":
				invalidInput.CustomDataPatch = json.RawMessage(`{"not_a_collected_field":"rework"}`)
			}
			got, err := f.reworkAcceptance(t, invalidCommand, invalidInput, nil)
			require.Error(t, err)
			require.Empty(t, got)
			require.Equal(t, before, f.acceptanceSnapshot(t), "rejection rolls back changes and releases K")
		})
	}

	got, err = f.reworkAcceptance(t, c, input, nil)
	require.NoError(t, err)
	require.Equal(t, "reworked", got.AcceptanceStatus)
	require.Equal(t, c.AcceptanceID, got.AcceptanceID)
	require.Equal(t, c.ExpectedAcceptanceRevision, got.AcceptanceRevision)
	require.Len(t, got.Items, 1)
	require.Equal(t, "reworked", got.Items[0].Result)
	require.Greater(t, got.TicketRevision, c.ExpectedTicketRevision)
	require.False(t, got.TerminalAllowed)

	var ticketStatus, mark, riskStatus, linkState string
	var acceptedUntil *time.Time
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT t.status,t.custom_data->>'review_mark',r.status,l.remediation_state,r.accepted_until
		FROM risk_ticket_links l JOIN risk_items r ON r.id=l.risk_id
		JOIN risk_dispatch_groups g ON g.id=l.group_id JOIN tickets t ON t.id=g.ticket_id
		WHERE l.id=$1`, c.LinkID).Scan(&ticketStatus, &mark, &riskStatus, &linkState, &acceptedUntil))
	require.Equal(t, "in_progress", ticketStatus)
	require.Equal(t, "rework", mark)
	require.Equal(t, "remediating", riskStatus)
	require.Equal(t, "open", linkState)
	require.Nil(t, acceptedUntil)
	require.Equal(t, preserved, f.acceptanceReworkPreservedFacts(t, c.AcceptanceID))

	before = f.acceptanceSnapshot(t)
	replay, err := f.reworkAcceptance(t, c, input, nil)
	require.NoError(t, err)
	require.Equal(t, got, replay)
	input.Comment += "!"
	changed, err := f.reworkAcceptance(t, c, input, nil)
	require.ErrorIs(t, err, errcode.ErrConflict)
	require.Empty(t, changed)
	require.Equal(t, before, f.acceptanceSnapshot(t))
}

func TestWorkflowAcceptanceReworkInProgressKeepsTicketRevision(t *testing.T) {
	f, c := newAcceptanceReworkFixture(t)
	requestB := f.acceptanceRequest(t, f.riskB)
	requestB.OperationID += "-sibling"
	pending, err := f.requestAcceptance(t, requestB)
	require.NoError(t, err)
	decisionB := f.acceptanceDecision(t, pending)
	decisionB.OperationID += "-sibling"
	decisionB.Decision = "reject"
	decisionB.TicketInput.CustomDataPatch = json.RawMessage(`{"review_mark":"sibling-rejected"}`)
	rejected, err := f.decideAcceptance(t, decisionB)
	require.NoError(t, err)
	c.ExpectedTicketRevision = rejected.TicketRevision
	f.expireApprovedRisk(t, c.AcceptanceID)

	input := ticket.RiskWorkflowTicketInput{DefinitionVersion: f.version, Comment: "No edge to consume this comment"}
	before := f.acceptanceSnapshot(t)
	got, err := f.reworkAcceptance(t, c, input, nil)
	require.Error(t, err)
	require.Empty(t, got)
	require.Equal(t, before, f.acceptanceSnapshot(t))
	input.Comment = ""
	got, err = f.reworkAcceptance(t, c, input, nil)
	require.NoError(t, err)
	require.Equal(t, "reworked", got.AcceptanceStatus)
	require.Equal(t, c.ExpectedTicketRevision, got.TicketRevision)
	var status string
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT status FROM tickets WHERE id=$1`, f.ticket).Scan(&status))
	require.Equal(t, "in_progress", status)
}

func TestWorkflowAcceptanceReworkRechecksManagementAfterAdmission(t *testing.T) {
	f, c := newAcceptanceReworkFixture(t)
	f.expireApprovedRisk(t, c.AcceptanceID)
	input := ticket.RiskWorkflowTicketInput{DefinitionVersion: f.version}
	var before string
	got, err := f.reworkAcceptance(t, c, input, func() {
		_, err := workflowPool.Exec(f.ctx, `UPDATE user_orgs SET org_member_role='member' WHERE user_id=$1 AND org_id=$2`, c.ActorID, f.orgA)
		require.NoError(t, err)
		before = f.acceptanceSnapshot(t)
	})
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	require.Empty(t, got)
	require.Equal(t, before, f.acceptanceSnapshot(t))
}
