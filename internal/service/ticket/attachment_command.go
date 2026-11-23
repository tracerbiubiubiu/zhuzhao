package ticket

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// AttachmentCommand ties a ticket-state storage reservation or binding to
// current update permission, or the private opt-in risk evidence scope. The
// callback uses the guarded connection; object storage I/O stays outside.
// Existing direct callers keep their own contract.
func (s *Service) AttachmentCommand(ctx context.Context, action string, actor, ticketID int64, write func(context.Context) error) error {
	_, http := middleware.IAMRequestFromContext(ctx)
	return s.withHTTPCommand(ctx, "attachment-"+action, actor, func(ctx context.Context, tx pgx.Tx) error {
		upload, scoped := ctx.Value(riskAttachmentKey{}).(riskAttachmentRequest)
		scoped = scoped && http && upload.service == s && upload.actor == actor && upload.ticket == ticketID && upload.action == action && (action == "presign" || action == "confirm")
		var risk *riskAttachmentScope
		if http || actor > 0 {
			if err := s.authorizeCheck(ctx, actor, "update", strconv.FormatInt(ticketID, 10)); err != nil {
				if !scoped || !upload.enabled || !errors.Is(err, errcode.ErrNoPermission) {
					return err
				}
				var e error
				risk, e = s.prelockRiskAttachment(ctx, tx, actor, ticketID)
				if e != nil {
					return e
				}
			}
		}
		ticket, err := s.ticketRepo.GetByIDForUpdateTx(ctx, tx, ticketID)
		if err != nil {
			return err
		}
		if _, _, _, err := s.resolveTicketDefinition(ctx, ticket); err != nil {
			return err
		}
		if risk != nil {
			if err := s.checkRiskAttachment(ctx, tx, actor, ticket, risk); err != nil {
				return err
			}
		}
		if scoped && action == "confirm" {
			if err := checkAttachmentReservation(ctx, tx, upload, risk != nil); err != nil {
				return err
			}
		}
		if err := write(ctx); err != nil {
			return err
		}
		if scoped && action == "presign" {
			tag, err := tx.Exec(ctx, `UPDATE file_objects SET owner_kind=$4,owner_id=$5 WHERE bucket=$1 AND object_key=$2 AND created_by=$3 AND purpose='ticket_attachment' AND status='pending' AND owner_kind IS NULL AND owner_id IS NULL`, upload.file.Bucket, upload.file.ObjectKey, actor, model.FileOwnerTicket, strconv.FormatInt(ticketID, 10))
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return errcode.ErrConflict
			}
		}
		return nil
	})
}
