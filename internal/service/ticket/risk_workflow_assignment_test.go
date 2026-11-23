package ticket

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestRiskWorkflowAssignmentPrelockRejectsInvalidInputBeforeDatabase(t *testing.T) {
	svc := &Service{ticketRepo: repository.NewTicketRepo(nil), orgRepo: repository.NewOrgRepo(nil)}
	for _, tc := range []struct {
		name   string
		svc    *Service
		ticket int64
		actor  int64
		orgs   []int64
	}{
		{name: "nil service", ticket: 1, actor: 1},
		{name: "missing repositories", svc: &Service{}, ticket: 1, actor: 1},
		{name: "ticket", svc: svc, actor: 1},
		{name: "actor", svc: svc, ticket: 1},
		{name: "zero organization", svc: svc, ticket: 1, actor: 1, orgs: []int64{0}},
		{name: "negative organization", svc: svc, ticket: 1, actor: 1, orgs: []int64{-1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope, err := tc.svc.PrelockRiskWorkflowAssignmentTx(context.Background(), nil, tc.ticket, tc.actor, tc.orgs...)
			require.ErrorIs(t, err, errcode.ErrInvalidParams)
			require.Nil(t, scope)
		})
	}
}
