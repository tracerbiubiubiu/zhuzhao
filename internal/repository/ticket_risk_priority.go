package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func (r *TicketRepo) SetRiskDispatchPriorityTx(ctx context.Context, tx pgx.Tx, before *model.Ticket, priority int) error {
	tag, err := tx.Exec(ctx, `UPDATE tickets SET priority=$1,revision=revision+1,updated_at=clock_timestamp()
		WHERE id=$2 AND revision=$3 AND status<> 'closed'`, priority, before.ID, before.Revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errcode.ErrConcurrentModification
	}
	return nil
}
