package ticket

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// applyStateSLARuleTx applies the immutable rule for the state being entered.
// Same-state reassignments preserve the existing state-entry time and deadline.
func (s *Service) applyStateSLARuleTx(ctx context.Context, tx pgx.Tx, typ *model.TicketType, ticket *model.Ticket, state string) error {
	var hours *int
	if state != StatusClosed {
		meta, err := parseStateMeta(typ.TransitionMeta, state)
		if err != nil {
			return fmt.Errorf("parse state deadline: %w", err)
		}
		if meta != nil && meta.SLADueHours != nil && *meta.SLADueHours > 0 {
			hours = meta.SLADueHours
		}
	}
	if err := tx.QueryRow(ctx, `UPDATE tickets
		SET sla_due_at=CASE WHEN $2::integer IS NULL THEN NULL
		 ELSE status_changed_at+make_interval(hours=>$2::integer) END
		WHERE id=$1 RETURNING sla_due_at`, ticket.ID, hours).Scan(&ticket.SLADueAt); err != nil {
		return fmt.Errorf("apply state deadline: %w", err)
	}
	return nil
}

// writeLifecycleCommentTx gives command comments the same durable event as the
// public comment endpoint, atomically with status and deadline changes.
func (s *Service) writeLifecycleCommentTx(ctx context.Context, tx pgx.Tx, ticketID, actorID int64, content string) error {
	if content == "" {
		return nil
	}
	if err := s.ticketRepo.CreateCommentTx(ctx, tx, &model.TicketComment{
		TicketID: ticketID, UserID: actorID, Content: content, IsInternal: false,
	}); err != nil {
		return err
	}
	return s.ticketRepo.CreateEventTx(ctx, tx, &model.TicketEvent{TicketID: ticketID, UserID: actorID, Action: EventComment})
}
