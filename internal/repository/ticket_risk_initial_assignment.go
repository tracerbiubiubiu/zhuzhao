package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// RefreshRiskDispatchTicketTx captures the database revision and clocks after
// the same transaction has created and initially assigned this provisional row.
func (r *TicketRepo) RefreshRiskDispatchTicketTx(ctx context.Context, tx pgx.Tx, ticket *model.Ticket) error {
	return tx.QueryRow(ctx, `SELECT revision, updated_at, status_changed_at FROM tickets WHERE id=$1`, ticket.ID).Scan(&ticket.Revision, &ticket.UpdatedAt, &ticket.StatusChangedAt)
}
