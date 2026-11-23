package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

const (
	riskAcceptanceRequestPath  = "/api/v1/risk-items/remediation-accept-request"
	riskAcceptanceDecisionPath = "/api/v1/risk-items/remediation-accept-decision"
	riskAcceptanceReworkPath   = "/api/v1/risk-items/remediation-accept-rework"
)

// CheckRiskAcceptanceApproverTx checks a different subject's current ticket
// eligibility. The owner of the risk transaction checks scoped risk authority.
// The applicant's admission is retained and the candidate gains no request or
// write authority. ErrNoPermission means this candidate does not qualify.
func (s *Service) CheckRiskAcceptanceApproverTx(ctx context.Context, tx pgx.Tx, p *RiskWorkflowScope, candidate int64) error {
	purpose, err := s.riskAcceptancePurpose(ctx, tx, p, riskAcceptanceRequestPath)
	if err != nil {
		return err
	}
	if candidate <= 0 || candidate == p.actor {
		return errcode.ErrNoPermission
	}
	if p.Status() != StatusPendingVerify {
		return errcode.ErrConflict
	}
	current, err := s.authorizeRiskAcceptanceTicket(ctx, purpose.Model, riskAcceptanceDecisionPath, p.before.ID, candidate)
	if errors.Is(err, errcode.ErrNoRoles) || errors.Is(err, errcode.ErrTicketNotFound) {
		return errcode.ErrNoPermission
	}
	if err != nil {
		return err
	}
	return s.checkRiskAcceptanceDecisionExits(current, p, candidate)
}

func (s *Service) checkRiskAcceptanceDecisionExits(ctx context.Context, p *RiskWorkflowScope, candidate int64) error {
	for _, to := range []string{StatusClosed, StatusInProgress} {
		if err := p.checkSupportedEdge(to); err != nil {
			return err
		}
		if _, err := s.checkTransitionCondition(ctx, &p.before, to, candidate, &p.definition.Type); err != nil {
			return err
		}
	}
	return nil
}

// CheckRiskAcceptanceReworkTx validates rework under the command's exact
// guarded scope before the risk kernel writes any effects.
func (s *Service) CheckRiskAcceptanceReworkTx(ctx context.Context, tx pgx.Tx, p *RiskWorkflowScope, ticketID, actor, expectedRevision int64) error {
	_, err := s.checkRiskAcceptanceReworkTx(ctx, tx, p, ticketID, actor, expectedRevision)
	return err
}

func (s *Service) checkRiskAcceptanceReworkTx(ctx context.Context, tx pgx.Tx, p *RiskWorkflowScope, ticketID, actor, expectedRevision int64) (string, error) {
	purpose, err := s.riskAcceptancePurpose(ctx, tx, p, riskAcceptanceReworkPath)
	if err != nil {
		return "", err
	}
	if p.actor != actor {
		return "", errcode.ErrNoPermission
	}
	if p.before.ID != ticketID || p.Revision() != expectedRevision || p.DefinitionVersion() <= 0 {
		return "", errcode.ErrConflict
	}
	current, err := s.authorizeRiskAcceptanceTicket(ctx, purpose.Model, riskAcceptanceReworkPath, ticketID, actor)
	if err != nil {
		return "", err
	}
	to, err := riskAcceptanceReworkTarget(p.Status())
	if err != nil || to == "" {
		return to, err
	}
	if err := p.checkSupportedEdge(to); err != nil {
		return "", err
	}
	if _, err := s.checkTransitionCondition(current, &p.before, to, actor, &p.definition.Type); err != nil {
		return "", err
	}
	return to, nil
}

func riskAcceptanceReworkTarget(status string) (string, error) {
	switch status {
	case StatusInProgress:
		return "", nil
	case StatusPendingVerify:
		return StatusInProgress, nil
	default:
		return "", errcode.ErrConflict
	}
}

// ApplyRiskAcceptanceReworkTx applies only the pinned pending_verify to
// in_progress edge. An already in-progress ticket keeps its revision. The
// caller must pass errors back to the kernel's owning savepoint.
func (s *Service) ApplyRiskAcceptanceReworkTx(ctx context.Context, tx pgx.Tx, p *RiskWorkflowScope, ticketID, actor, expectedRevision int64, input RiskWorkflowTicketInput) (int64, error) {
	to, err := s.checkRiskAcceptanceReworkTx(ctx, tx, p, ticketID, actor, expectedRevision)
	if err != nil {
		return 0, err
	}
	if input.DefinitionVersion != p.DefinitionVersion() {
		return 0, errcode.ErrConflict
	}
	if to == "" {
		patch, err := normalizeTransitionPayload(input.CustomDataPatch, input.Comment)
		if err != nil {
			return 0, err
		}
		if string(patch) != "{}" || input.Comment != "" {
			return 0, errcode.ErrServiceUnavailable
		}
		if !p.used.CompareAndSwap(false, true) {
			return 0, errcode.ErrNoPermission
		}
		binding, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, ticketID)
		if err != nil {
			return 0, err
		}
		if !sameRiskWorkflowTopology(p.binding, binding) {
			return 0, errcode.ErrConcurrentModification
		}
		return p.Revision(), nil
	}
	req := &model.TransitionTicketRequest{
		ID:                ticketID,
		To:                to,
		ExpectedRevision:  json.RawMessage(strconv.Quote(strconv.FormatInt(expectedRevision, 10))),
		DefinitionVersion: json.RawMessage(strconv.Quote(strconv.FormatInt(input.DefinitionVersion, 10))),
		CustomDataPatch:   input.CustomDataPatch,
		Comment:           input.Comment,
		IdempotencyKey:    fmt.Sprintf("risk-accept-rework-%d-%d-%d", actor, ticketID, expectedRevision),
	}
	updated, err := s.ApplyRiskWorkflowTx(ctx, tx, p, req)
	if err != nil {
		return 0, err
	}
	return updated.Revision, nil
}

func (s *Service) riskAcceptancePurpose(ctx context.Context, tx pgx.Tx, p *RiskWorkflowScope, path string) (middleware.IAMRequestPurpose, error) {
	if s == nil || s.ticketRepo == nil {
		return middleware.IAMRequestPurpose{}, errcode.ErrInvalidParams
	}
	if err := s.ticketRepo.RequireGuardedCreateTx(ctx, s.db, tx); err != nil {
		return middleware.IAMRequestPurpose{}, err
	}
	proof, ok := ctx.Value(riskWorkflowAuthorizationKey{}).(riskWorkflowAuthorization)
	if p == nil || p.service != s || p.tx != tx || !ok || proof.service != s || proof.tx != tx || proof.actor != p.actor {
		return middleware.IAMRequestPurpose{}, errcode.ErrNoPermission
	}
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok || purpose.UserID != p.actor || purpose.Path != path || purpose.Method != "POST" || purpose.SelfService {
		return middleware.IAMRequestPurpose{}, errcode.ErrNoPermission
	}
	return purpose, nil
}

func (s *Service) authorizeRiskAcceptanceTicket(ctx context.Context, modelText, path string, ticketID, actor int64) (context.Context, error) {
	id := strconv.FormatInt(ticketID, 10)
	roles, err := s.freshCommandRoles(ctx, actor, modelText,
		ticketPolicyRoute{path: path, method: "POST"},
		ticketPolicyRoute{path: "/api/v1/tickets/" + id, method: "GET"},
		ticketPolicyRoute{path: "/api/v1/tickets/transition", method: "POST"},
	)
	if err != nil {
		return nil, err
	}
	current := context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: actor, roles: roles})
	for _, action := range []string{"read", "transition"} {
		if err := s.authorizeCheck(current, actor, action, id); err != nil {
			return nil, err
		}
	}
	return current, nil
}
