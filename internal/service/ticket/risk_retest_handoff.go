package ticket

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const (
	RiskRetestHandoffPath = "/api/v1/risk-items/remediation-handoff"
	riskRetestReviewPath  = "/api/v1/risk-items/remediation-review"
)

func riskRetestEdge(from, to string) bool {
	return from == StatusPendingVerify && (to == StatusPendingVerify || to == StatusClosed || to == StatusInProgress)
}

// PrelockRiskRetestHandoffTx observes a pinned definition without minting a
// ticket transition capability for the operator of the dedicated handoff route.
func (s *Service) PrelockRiskRetestHandoffTx(ctx context.Context, tx pgx.Tx, ticketID, actor int64, orgIDs ...int64) (*RiskWorkflowScope, error) {
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok || purpose.Path != RiskRetestHandoffPath || purpose.Method != "POST" || purpose.UserID != actor {
		return nil, errcode.ErrNoPermission
	}
	return s.prelockRiskWorkflowTx(ctx, tx, ticketID, actor, true, orgIDs...)
}

// CheckRiskRetestSuccessorTx checks the candidate as a current account against
// the real pin and current ticket read policy. It creates no credentials or roles.
// The risk adapter must also ensure independent risk visibility for every member.
func (s *Service) CheckRiskRetestSuccessorTx(ctx context.Context, tx pgx.Tx, p *RiskWorkflowScope, target int64) error {
	proof, ok := ctx.Value(riskWorkflowAuthorizationKey{}).(riskWorkflowAuthorization)
	purpose, purposeOK := middleware.IAMRequestFromContext(ctx)
	if p == nil || p.service != s || p.tx != tx || !ok || !purposeOK || purpose.Path != RiskRetestHandoffPath ||
		purpose.Method != "POST" || proof.service != s || proof.tx != tx || proof.actor != p.actor || target <= 0 {
		return errcode.ErrNoPermission
	}
	before, err := s.ticketRepo.GetByIDForUpdateTx(ctx, tx, p.before.ID)
	if err != nil {
		return err
	}
	if before.Revision != p.Revision() || before.Status != p.Status() || before.TypeCode != p.before.TypeCode || before.OrgID != p.before.OrgID ||
		before.DefinitionVersion == nil || *before.DefinitionVersion != p.DefinitionVersion() {
		return errcode.ErrConflict
	}
	if before.AssignedTo == nil || target == *before.AssignedTo {
		return errcode.ErrNoPermission
	}
	if err := validateRiskWorkflowDefinition(&p.definition.Type, p.definition.Fields, false); err != nil {
		return err
	}
	account, err := repository.NewUserRepo(s.db).FindByID(ctx, target)
	if err != nil || account.IsSystem {
		return errcode.ErrNoPermission
	}
	ticketID := strconv.FormatInt(p.before.ID, 10)
	roles, err := s.freshCommandRoles(ctx, target, purpose.Model,
		ticketPolicyRoute{path: riskRetestReviewPath, method: "POST"},
		ticketPolicyRoute{path: "/api/v1/tickets/transition", method: "POST"},
		ticketPolicyRoute{path: "/api/v1/tickets/" + ticketID, method: "GET"},
		ticketPolicyRoute{path: "/api/v1/risk-items/remediation-context/" + ticketID, method: "GET"})
	if err != nil {
		return err
	}
	current := context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: target, roles: roles})
	return s.authorizeCheck(current, target, "read", ticketID)
}
