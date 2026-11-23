package ticket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestRiskAcceptanceApproverNeedsBothPinnedDecisionExits(t *testing.T) {
	for _, tc := range []struct {
		name, transitions, meta string
		want                    error
	}{
		{"both conditions", `{"pending_verify":["closed","in_progress"]}`, `{"pending_verify>closed":{"roles":["creator"]},"pending_verify>in_progress":{"roles":["creator"]}}`, nil},
		{"decision may supply collected values later", `{"pending_verify":["closed","in_progress"]}`, `{"pending_verify>closed":{"roles":["creator"],"collect_fields":[{"key":"review_mark","required":true}],"validator":"custom_data.review_mark == \"ok\""}}`, nil},
		{"denied approval", `{"pending_verify":["closed","in_progress"]}`, `{"pending_verify>closed":{"roles":["assignee"]}}`, errcode.ErrNoPermission},
		{"denied rejection", `{"pending_verify":["closed","in_progress"]}`, `{"pending_verify>in_progress":{"roles":["assignee"]}}`, errcode.ErrNoPermission},
		{"missing rejection edge", `{"pending_verify":["closed"]}`, `{}`, errcode.ErrTicketInvalidTransition},
		{"unsupported approval effects", `{"pending_verify":["closed","in_progress"]}`, `{"pending_verify>closed":{"post_actions":[{}]}}`, errcode.ErrServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const candidate int64 = 22
			s := &Service{delegation: initialRiskDelegation{}}
			ctx := riskRoleContext(candidate)
			sm, err := NewStateMachine(json.RawMessage(tc.transitions))
			require.NoError(t, err)
			applicant := int64(11)
			scope := &RiskWorkflowScope{
				actor:  applicant,
				before: model.Ticket{Status: StatusPendingVerify, CreatedBy: candidate, AssignedTo: &applicant},
				sm:     sm,
				definition: &model.TicketDefinitionSnapshot{
					Type:   model.TicketType{TransitionMeta: json.RawMessage(tc.meta)},
					Fields: []*model.TicketTypeField{{FieldKey: "review_mark", FieldType: "text"}},
				},
			}
			require.ErrorIs(t, s.checkRiskAcceptanceDecisionExits(ctx, scope, candidate), tc.want)
		})
	}
}

func TestRiskAcceptanceReworkTarget(t *testing.T) {
	for _, tc := range []struct {
		status, want string
		wantErr      error
	}{
		{StatusInProgress, "", nil},
		{StatusPendingVerify, StatusInProgress, nil},
		{StatusClosed, "", errcode.ErrConflict},
		{StatusAssigned, "", errcode.ErrConflict},
		{StatusOpen, "", errcode.ErrConflict},
		{StatusRejected, "", errcode.ErrConflict},
	} {
		t.Run(tc.status, func(t *testing.T) {
			target, err := riskAcceptanceReworkTarget(tc.status)
			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.want, target)
		})
	}
}

func TestRiskAcceptanceTicketPortsRequireGuard(t *testing.T) {
	s := &Service{ticketRepo: repository.NewTicketRepo(nil)}
	ctx := context.Background()
	scope := &RiskWorkflowScope{service: s, actor: 11}
	require.ErrorIs(t, s.CheckRiskAcceptanceApproverTx(ctx, nil, scope, 22), errcode.ErrNoPermission)
	require.ErrorIs(t, s.CheckRiskAcceptanceReworkTx(ctx, nil, scope, 41, 11, 2), errcode.ErrNoPermission)
	_, err := s.ApplyRiskAcceptanceReworkTx(ctx, nil, scope, 41, 11, 2, RiskWorkflowTicketInput{})
	require.ErrorIs(t, err, errcode.ErrNoPermission)
}
