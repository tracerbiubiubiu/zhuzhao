package ticket

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// assignInitialRiskTicketTx is only for the newly inserted ticket owned by the
// risk create transaction. Risk authorization and enabled-owner validation must
// precede creation. It never admits an existing ticket or impersonates its owner.
func (s *Service) assignInitialRiskTicketTx(ctx context.Context, tx pgx.Tx, p *preparedTicketCreate, ticket *model.Ticket, owner int64) error {
	if ticket.Status != StatusOpen || ticket.AssignedTo != nil || owner <= 0 {
		return errcode.ErrConcurrentModification
	}
	sm, err := FromTicketType(p.ttype)
	if err != nil {
		return err
	}
	if err := sm.AssertTransition(StatusOpen, StatusAssigned); err != nil {
		return err
	}
	// Reuse the published edge's condition, validator and evidence restrictions
	// with the actual dispatcher, rather than the risk owner/created_by identity.
	if _, err := s.checkTransitionMeta(ctx, ticket, StatusAssigned, p.actor, p.ttype); err != nil {
		return err
	}
	if err := s.ticketRepo.UpdateAssignedToTx(ctx, tx, ticket.ID, &owner); err != nil {
		return err
	}
	if err := s.ticketRepo.UpdateStatusTx(ctx, tx, ticket.ID, StatusOpen, StatusAssigned); err != nil {
		return err
	}
	if err := s.applyStateSLARuleTx(ctx, tx, p.ttype, ticket, StatusAssigned); err != nil {
		return err
	}
	if err := s.ticketRepo.CreateEventTx(ctx, tx, &model.TicketEvent{TicketID: ticket.ID, UserID: p.actor, Action: EventAssigned, ToValue: strconv.FormatInt(owner, 10)}); err != nil {
		return err
	}
	ticket.Status = StatusAssigned
	ticket.AssignedTo = &owner
	return nil
}

// writeInitialRiskDispatchTx keeps created history, initial assignment, risk
// link and file binding atomic. Its callback cannot acquire earlier risk locks.
func (s *Service) writeInitialRiskDispatchTx(ctx context.Context, tx pgx.Tx, p *preparedTicketCreate, ticket *model.Ticket, owner int64, link RiskDispatchLinkWriter) error {
	if err := s.writePreparedCreateTx(ctx, tx, p, ticket, func(ctx context.Context, tx pgx.Tx, id int64) error {
		if err := s.assignInitialRiskTicketTx(ctx, tx, p, ticket, owner); err != nil {
			return err
		}
		return link(ctx, tx, id)
	}); err != nil {
		return err
	}
	return s.ticketRepo.RefreshRiskDispatchTicketTx(ctx, tx, ticket)
}
