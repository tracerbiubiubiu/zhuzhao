package repository

import (
	"context"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// LockTransitionSnapshotTx fences a preflight authorization/validator snapshot.
// Configuration is locked before the ticket; all queries reuse the admitted tx.
// A changed snapshot requires a fresh request instead of using stale permission
// or validator input. Locks remain held through the caller's write and commit.
func (r *TicketRepo) LockTransitionSnapshotTx(ctx context.Context, tx pgx.Tx, expected *model.Ticket, config *model.TicketType) error {
	if config != nil {
		var version int64
		err := tx.QueryRow(ctx, `SELECT version FROM ticket_types WHERE code=$1 FOR SHARE`, config.Code).Scan(&version)
		if errors.Is(err, pgx.ErrNoRows) {
			return errcode.ErrTicketTypeNotFound
		}
		if err != nil {
			return err
		}
		if version != config.Version {
			return errcode.ErrConcurrentModification
		}
	}
	current, err := scanTicketRow(tx.QueryRow(ctx, `SELECT`+ticketSelectColumns+` FROM tickets WHERE id=$1 FOR UPDATE`, expected.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return errcode.ErrTicketNotFound
	}
	if err != nil {
		return err
	}
	if current.Status != expected.Status && current.Status == "closed" {
		return errcode.ErrTicketAlreadyClosed
	}
	// GetByID enriches these four display-only values after reading the ticket.
	// They are not authorization/validator input or stored ticket state.
	baseline := *expected
	baseline.CreatedByName, baseline.CreatedByNo = "", ""
	baseline.AssigneeName, baseline.AssigneeNo = "", ""
	if !reflect.DeepEqual(*current, baseline) {
		return errcode.ErrConcurrentModification
	}
	return nil
}
