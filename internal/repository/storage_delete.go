package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/fileref"
)

var (
	ErrStorageInlineAttachment     = errors.New("repository: inline attachment follows ticket lifecycle")
	ErrStorageAttachmentReferenced = errors.New("repository: attachment still referenced")
)

// DeleteTicketAttachment serializes with all ticket field/attachment writes.
// Network cleanup belongs to the caller, only after this commit succeeds.
func (r *StorageRepo) DeleteTicketAttachment(ctx context.Context, bucket string, ticketID int64, key string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var typeCode string
	var data []byte
	err = tx.QueryRow(ctx, `SELECT type_code,custom_data FROM tickets WHERE id=$1 FOR UPDATE`, ticketID).Scan(&typeCode, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrStorageObjectNotFound
	}
	if err != nil {
		return err
	}
	var id int64
	var purpose, status string
	var ownerMatches bool
	err = tx.QueryRow(ctx, `SELECT id,purpose,status,
  ((COALESCE(owner_kind,'')='' AND COALESCE(owner_id,'')='') OR
   (owner_kind='ticket' AND owner_id=$3::bigint::text)) IS TRUE
  FROM file_objects WHERE bucket=$1 AND object_key=$2 FOR UPDATE`, bucket, key, ticketID).Scan(&id, &purpose, &status, &ownerMatches)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrStorageObjectNotFound
	}
	if err != nil {
		return err
	}
	// Separate statement after the locks: observe current association, never the
	// pre-wait snapshot. Do not expose another ticket's purpose in the response.
	var bound, foreignBound bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ticket_attachments WHERE ticket_id=$1 AND file_object_id=$2),
 EXISTS(SELECT 1 FROM ticket_attachments WHERE ticket_id<>$1 AND file_object_id=$2)`, ticketID, id).Scan(&bound, &foreignBound)
	if err != nil {
		return err
	}
	if !bound || status != "confirmed" {
		return ErrStorageObjectNotFound
	}
	if purpose == "ticket_richtext" {
		return ErrStorageInlineAttachment
	}
	if purpose != "ticket_attachment" {
		return ErrStorageObjectNotFound
	}
	if foreignBound || !ownerMatches {
		return ErrStorageAttachmentReferenced
	}
	if err := checkStoredAttachmentReferences(ctx, tx, typeCode, data, key); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM ticket_attachments WHERE ticket_id=$1 AND file_object_id=$2`, ticketID, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE file_objects SET status='deleted' WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// No visibility pruning here: hidden/current stored fields also protect bytes.
// Missing definitions and malformed values hold the destructive operation.
func checkStoredAttachmentReferences(ctx context.Context, tx pgx.Tx, typeCode string, raw []byte, key string) error {
	// SQL NULL and JSON null are legal empty custom_data on fieldless types.
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var data map[string]json.RawMessage
	if json.Unmarshal(raw, &data) != nil || data == nil {
		return ErrStorageAttachmentReferenced
	}
	fields, err := listTicketTypeFields(ctx, tx, typeCode)
	if err != nil {
		return err
	}
	for _, field := range fields {
		value, ok := data[field.FieldKey]
		delete(data, field.FieldKey)
		if !ok || string(value) == "null" {
			continue
		}
		switch field.FieldType {
		case "file":
			var keys []string
			if json.Unmarshal(value, &keys) != nil {
				return ErrStorageAttachmentReferenced
			}
			for _, ref := range keys {
				if ref == "" || ref == key {
					return ErrStorageAttachmentReferenced
				}
			}
		case "richtext":
			var markdown string
			if json.Unmarshal(value, &markdown) != nil {
				return ErrStorageAttachmentReferenced
			}
			for _, ref := range fileref.ExtractFileRefs(markdown) {
				if ref == key {
					return ErrStorageAttachmentReferenced
				}
			}
		}
	}
	for _, value := range data {
		if string(value) != "null" {
			return ErrStorageAttachmentReferenced
		}
	}
	return nil
}
