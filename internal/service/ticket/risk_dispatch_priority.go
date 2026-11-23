package ticket

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// UpdateRiskDispatchPriorityTx applies the scoped risk group's derived value.
// An authorized dispatcher holds the group, affected risks and ticket locks.
func (s *Service) UpdateRiskDispatchPriorityTx(ctx context.Context, tx pgx.Tx, ticketID, actorUserID int64, priority int) error {
	if err := s.ticketRepo.RequireGuardedCreateTx(ctx, s.db, tx); err != nil {
		return err
	}
	if ticketID <= 0 || actorUserID <= 0 || priority < 1 || priority > 4 {
		return errcode.ErrInvalidParams
	}
	before, err := s.ticketRepo.GetByIDForUpdateTx(ctx, tx, ticketID)
	if err != nil {
		return err
	}
	if before.Status == StatusClosed {
		return errcode.ErrTicketAlreadyClosed
	}
	binding, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, ticketID)
	if err != nil {
		return err
	}
	if err := validateRiskTicketActionBinding(before, binding); err != nil {
		return err
	}
	if priority == before.Priority {
		return nil
	}
	if err := s.ticketRepo.SetRiskDispatchPriorityTx(ctx, tx, before, priority); err != nil {
		return err
	}
	return s.ticketRepo.CreateEventTx(ctx, tx, &model.TicketEvent{
		TicketID: ticketID, UserID: actorUserID, Action: EventUpdated,
		FromValue: strconv.Itoa(before.Priority), ToValue: strconv.Itoa(priority),
	})
}
