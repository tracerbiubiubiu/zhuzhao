package ticket

import (
	"context"
	"errors"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// PrelockRiskWorkflowAssignmentTx refreshes the current actor's ticket read and
// assignment authority, then share-locks type and live organizations in order.
// It takes no group/risk/ticket/link locks and grants no target eligibility.
// The trusted adapter must revalidate the supplied organization observations
// under its subsequent risk locks and use LockRiskTicketReassignmentScopeTx for
// the fixed target. Invoke on the exact WithRiskWorkflowAuthorizationTx context.
// The returned scope exposes the actual pinned publication for observation; it
// cannot authorize ApplyRiskWorkflowTx, which requires transition admission.
func (s *Service) PrelockRiskWorkflowAssignmentTx(ctx context.Context, tx pgx.Tx, ticketID, actor int64, organizationIDs ...int64) (*RiskWorkflowScope, error) {
	if s == nil || s.ticketRepo == nil || s.orgRepo == nil || ticketID <= 0 || actor <= 0 {
		return nil, errcode.ErrInvalidParams
	}
	for _, id := range organizationIDs {
		if id <= 0 {
			return nil, errcode.ErrInvalidParams
		}
	}
	if err := s.CheckRiskWorkflowReadTx(ctx, tx, actor, ticketID); err != nil {
		return nil, err
	}
	purpose, _ := middleware.IAMRequestFromContext(ctx)
	roles, err := s.freshCommandRoles(ctx, actor, purpose.Model, ticketPolicyRoute{path: "/api/v1/tickets/assign", method: "POST"})
	if err != nil {
		return nil, err
	}
	current := context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: actor, roles: roles})
	if err := s.authorizeCheck(current, actor, "assign", strconv.FormatInt(ticketID, 10)); err != nil {
		return nil, err
	}
	before, err := s.ticketRepo.GetByID(current, ticketID)
	if err != nil {
		return nil, err
	}
	var code string
	err = tx.QueryRow(ctx, `SELECT code FROM ticket_types WHERE code=$1 FOR SHARE`, before.TypeCode).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errcode.ErrTicketTypeNotFound
	}
	if err != nil {
		return nil, err
	}
	snapshot, _, err := s.ticketRepo.BoundTicketDefinitionTx(ctx, tx, before.TypeCode, before.DefinitionVersion)
	if err != nil {
		return nil, err
	}
	if before.OrgID <= 0 {
		return nil, errcode.ErrInvalidParams
	}
	orgs := append([]int64{before.OrgID}, organizationIDs...)
	sort.Slice(orgs, func(i, j int) bool { return orgs[i] < orgs[j] })
	for i, id := range orgs {
		if i > 0 && orgs[i-1] == id {
			continue
		}
		if _, err := s.orgRepo.FindByIDForShareTx(ctx, tx, id); err != nil {
			return nil, err
		}
	}
	binding, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, ticketID)
	if err != nil {
		return nil, err
	}
	if err := validateRiskTicketActionBinding(before, binding); err != nil {
		return nil, err
	}
	sm, err := FromTicketType(&snapshot.Type)
	if err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	scope := &RiskWorkflowScope{service: s, tx: tx, actor: actor, before: cloneTicketSnapshot(before), binding: binding, sm: sm, definition: snapshot}
	// An assignment prelock never minted the transition capability consumed by
	// ApplyRiskWorkflowTx. Keep its useful immutable observations, but close that
	// one-use capability before handing this scope to the assignment adapter.
	scope.used.Store(true)
	return scope, nil
}
