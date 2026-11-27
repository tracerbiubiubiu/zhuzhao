package repository

import (
	"bytes"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// HandlingReceiptTx resolves only the actor's original request. Callers first
// check the current credential, route grant, ticket visibility and domain scope.
func (r *TicketRepo) HandlingReceiptTx(ctx context.Context, tx pgx.Tx, actor int64, key string, digest []byte) (*model.TicketHandlingResult, error) {
	var saved []byte
	var out model.TicketHandlingResult
	err := tx.QueryRow(ctx, `SELECT request_hash,ticket_id,result_assignee,result_revision,result_status
  FROM ticket_handling_receipts WHERE actor_id=$1 AND operation_key=$2`, actor, key).Scan(&saved, &out.ID, &out.AssignedTo, &out.Revision, &out.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(digest, saved) {
		return nil, errcode.ErrConflict
	}
	return &out, nil
}

func (r *TicketRepo) SaveHandlingReceiptTx(ctx context.Context, tx pgx.Tx, actor int64, key string, digest []byte, out *model.TicketHandlingResult) error {
	tag, err := tx.Exec(ctx, `INSERT INTO ticket_handling_receipts(actor_id,operation_key,request_hash,ticket_id,result_assignee,result_revision,result_status)
  VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(actor_id,operation_key) DO NOTHING`, actor, key, digest, out.ID, out.AssignedTo, out.Revision, out.Status)
	if err == nil && tag.RowsAffected() != 1 {
		return errcode.ErrConflict
	}
	return err
}

// WithdrawUntouchedTx consumes the monotone eligibility fact and records terminal
// state entry on the same row. Obsolete reminders cannot survive the transaction.
func (r *TicketRepo) WithdrawUntouchedTx(ctx context.Context, tx pgx.Tx, id, actor, rev int64) error {
	tag, err := tx.Exec(ctx, `UPDATE tickets SET status='cancelled',updated_at=statement_timestamp(),status_changed_at=statement_timestamp(),sla_due_at=NULL
  WHERE id=$1 AND created_by=$2 AND revision=$3 AND withdrawal_eligible AND status='open' AND assigned_to IS NULL`, id, actor, rev)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errcode.ErrConcurrentModification
	}
	_, err = tx.Exec(ctx, "DELETE FROM ticket_reminders WHERE ticket_id=$1", id)
	return err
}
