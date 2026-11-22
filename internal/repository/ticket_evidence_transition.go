package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// TicketEvidenceTransitionResult is the original business result, safe to
// retain without evidence values, comments, or the current ticket projection.
type TicketEvidenceTransitionResult struct {
	Status            string     `json:"status"`
	Revision          int64      `json:"revision,string"`
	DefinitionVersion int64      `json:"definition_version,string"`
	StatusChangedAt   *time.Time `json:"status_changed_at"`
}

// ReadTicketEvidenceRevisionTx reads the revision of a ticket locked by the caller.
func (r *TicketRepo) ReadTicketEvidenceRevisionTx(ctx context.Context, tx pgx.Tx, ticketID int64) (int64, error) {
	var revision int64
	err := tx.QueryRow(ctx, `SELECT revision FROM tickets WHERE id=$1`, ticketID).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errcode.ErrTicketNotFound
	}
	return revision, err
}

// ApplyTicketEvidenceTransitionTx writes the candidate and enters its state in
// one statement. The row trigger advances the revision once for the combined
// business change. A self-edge must change evidence and retains its entry time.
func (r *TicketRepo) ApplyTicketEvidenceTransitionTx(ctx context.Context, tx pgx.Tx, ticketID, revision, definitionVersion int64, from, to string, data json.RawMessage) (TicketEvidenceTransitionResult, error) {
	var out TicketEvidenceTransitionResult
	err := tx.QueryRow(ctx, `UPDATE tickets
		SET custom_data=$6::jsonb, status=$5, updated_at=statement_timestamp(),
		    status_changed_at=CASE WHEN status<>$5 THEN statement_timestamp() ELSE status_changed_at END
		WHERE id=$1 AND revision=$2 AND definition_version=$3 AND status=$4
		  AND (status<>$5 OR custom_data IS DISTINCT FROM $6::jsonb)
		RETURNING status,revision,definition_version,status_changed_at`,
		ticketID, revision, definitionVersion, from, to, data).
		Scan(&out.Status, &out.Revision, &out.DefinitionVersion, &out.StatusChangedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return TicketEvidenceTransitionResult{}, errcode.ErrConcurrentModification
	}
	return out, err
}

// FindTicketEvidenceTransitionReceiptTx must run after current authorization.
// A key is scoped to this capability and actor, including across target tickets.
func (r *TicketRepo) FindTicketEvidenceTransitionReceiptTx(ctx context.Context, tx pgx.Tx, actorID int64, key string) (*TicketEvidenceTransitionResult, []byte, error) {
	var out TicketEvidenceTransitionResult
	var hash []byte
	err := tx.QueryRow(ctx, `SELECT request_hash,result_status,result_revision,result_definition_version,result_status_changed_at
		FROM ticket_evidence_transition_receipts WHERE actor_id=$1 AND idempotency_key=$2`, actorID, key).
		Scan(&hash, &out.Status, &out.Revision, &out.DefinitionVersion, &out.StatusChangedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return &out, hash, nil
}

// RecordTicketEvidenceTransitionReceiptTx persists the original success in the
// same transaction as the write. A concurrent key collision must abort it.
func (r *TicketRepo) RecordTicketEvidenceTransitionReceiptTx(ctx context.Context, tx pgx.Tx, ticketID, actorID int64, key string, hash []byte, out TicketEvidenceTransitionResult) error {
	tag, err := tx.Exec(ctx, `INSERT INTO ticket_evidence_transition_receipts
		(ticket_id,actor_id,idempotency_key,request_hash,result_status,result_revision,result_definition_version,result_status_changed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (actor_id,idempotency_key) DO NOTHING`,
		ticketID, actorID, key, hash, out.Status, out.Revision, out.DefinitionVersion, out.StatusChangedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrConflict
	}
	return nil
}
