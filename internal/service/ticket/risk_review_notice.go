package ticket

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// CheckRiskReviewNoticeAuthorizationTx checks current ticket access for an
// internal review notification. It grants no command or HTTP authority.
func (s *Service) CheckRiskReviewNoticeAuthorizationTx(ctx context.Context, tx pgx.Tx, actor, ticketID int64, modelText string) error {
	current, err := s.reviewNoticeTicketRead(ctx, tx, actor, ticketID, modelText,
		ticketPolicyRoute{path: "/api/v1/tickets/transition", method: "POST"},
		ticketPolicyRoute{path: riskRetestReviewPath, method: "POST"})
	if err != nil {
		return err
	}
	before, err := s.ticketRepo.GetByID(ctx, ticketID)
	if err != nil {
		return err
	}
	if before.Status != StatusPendingVerify {
		return errcode.ErrTicketInvalidTransition
	}
	id := strconv.FormatInt(ticketID, 10)
	binding, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, ticketID)
	if err != nil {
		return err
	}
	if err := validateRiskTicketActionBinding(before, binding); err != nil {
		return err
	}
	retest := binding[0].RetestReviewerID == actor
	if err := s.authorizeCheck(current, actor, "transition", id); err != nil && !(retest && errors.Is(err, errcode.ErrNoPermission)) {
		return err
	}
	snapshot, _, err := s.ticketRepo.BoundTicketDefinitionTx(ctx, tx, before.TypeCode, before.DefinitionVersion)
	if err != nil {
		return err
	}
	sm, err := FromTicketType(&snapshot.Type)
	if err != nil {
		return errcode.ErrServiceUnavailable
	}
	scope := &RiskWorkflowScope{before: *before, sm: sm, definition: snapshot}
	for _, to := range []string{StatusPendingVerify, StatusClosed, StatusInProgress} {
		if err := scope.checkSupportedEdge(to); errors.Is(err, errcode.ErrTicketInvalidTransition) {
			continue
		} else if err != nil {
			return err
		}
		_, err := s.checkTransitionCondition(current, before, to, actor, &snapshot.Type)
		if err == nil {
			return nil
		}
		if !errors.Is(err, errcode.ErrNoPermission) {
			return err
		}
		if retest {
			meta, err := parseTransitionMeta(snapshot.Type.TransitionMeta, before.Status, to)
			if err != nil {
				return err
			}
			if checkCondition(meta, transitionActor{IsCreator: true}) {
				return nil
			}
		}
	}
	return errcode.ErrNoPermission
}

// CheckRiskReviewNoticeReceiptAuthorizationTx checks read access to the ticket
// behind a stored review delivery receipt, including after review completes.
func (s *Service) CheckRiskReviewNoticeReceiptAuthorizationTx(ctx context.Context, tx pgx.Tx, actor, ticketID int64, modelText string) error {
	_, err := s.reviewNoticeTicketRead(ctx, tx, actor, ticketID, modelText)
	return err
}

func (s *Service) reviewNoticeTicketRead(ctx context.Context, tx pgx.Tx, actor, ticketID int64, modelText string, routes ...ticketPolicyRoute) (context.Context, error) {
	if s == nil || s.ticketRepo == nil || strings.TrimSpace(modelText) == "" {
		return nil, errcode.ErrServiceUnavailable
	}
	if actor <= 0 || ticketID <= 0 {
		return nil, errcode.ErrInvalidParams
	}
	if err := s.ticketRepo.RequireGuardedCreateTx(ctx, s.db, tx); err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	id := strconv.FormatInt(ticketID, 10)
	routes = append(routes, ticketPolicyRoute{path: "/api/v1/tickets/" + id, method: "GET"})
	roles, err := s.freshCommandRoles(ctx, actor, modelText, routes...)
	if err != nil {
		return nil, err
	}
	// Order visibility with ticket transitions and reviewer handoffs.
	var lockedID int64
	err = tx.QueryRow(ctx, `SELECT id FROM tickets WHERE id=$1 FOR SHARE`, ticketID).Scan(&lockedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errcode.ErrTicketNotFound
	}
	if err != nil {
		return nil, err
	}
	current := context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: actor, roles: roles})
	if err := s.authorizeCheck(current, actor, "read", id); err != nil {
		return nil, err
	}
	return current, nil
}
