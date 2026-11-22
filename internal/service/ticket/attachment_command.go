package ticket

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
)

// AttachmentCommand ties a ticket-state storage reservation or binding to
// current update permission. The callback uses the guarded connection; object
// storage I/O stays outside. Existing direct callers keep their own contract.
func (s *Service) AttachmentCommand(ctx context.Context, action string, actor, ticketID int64, write func(context.Context) error) error {
	_, http := middleware.IAMRequestFromContext(ctx)
	return s.withHTTPCommand(ctx, "attachment-"+action, actor, func(ctx context.Context, tx pgx.Tx) error {
		if http || actor > 0 {
			if err := s.authorizeCheck(ctx, actor, "update", strconv.FormatInt(ticketID, 10)); err != nil {
				return err
			}
		}
		ticket, err := s.ticketRepo.GetByIDForUpdateTx(ctx, tx, ticketID)
		if err != nil {
			return err
		}
		if _, _, _, err := s.resolveTicketDefinition(ctx, ticket); err != nil {
			return err
		}
		return write(ctx)
	})
}
