package repository

import (
	"context"
	"errors"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// StorageFieldReference carries the schema's purpose, rather than guessing a
// field's authority from an object_key or from untrusted JSON alone.
type StorageFieldReference struct{ ObjectKey, Purpose string }

// BindFieldReferences permits already-bound references only on the same ticket.
// New references still require the current editor's confirmed dangling object
// of the schema's exact purpose. Caller holds the ticket row before object rows.
func (r *StorageRepo) BindFieldReferences(ctx context.Context, exec rowExec, bucket string, userID, ticketID int64, refs []StorageFieldReference, maxCount int) error {
	purposes := make(map[string]string, len(refs))
	for _, ref := range refs {
		if ref.ObjectKey == "" || (ref.Purpose != "ticket_attachment" && ref.Purpose != "ticket_richtext") {
			return ErrStorageObjectNotFound
		}
		if prior, exists := purposes[ref.ObjectKey]; exists && prior != ref.Purpose {
			return ErrStorageObjectNotFound
		}
		purposes[ref.ObjectKey] = ref.Purpose
	}
	keys := make([]string, 0, len(purposes))
	for key := range purposes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	newKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		var id, creator int64
		var purpose, status, ownerKind, ownerID string
		var sameTicket, alreadyBound bool
		err := exec.QueryRow(ctx, `SELECT id,purpose,status,created_by,COALESCE(owner_kind,''),COALESCE(owner_id,''),
   EXISTS(SELECT 1 FROM ticket_attachments WHERE file_object_id=fo.id AND ticket_id=$3),
   EXISTS(SELECT 1 FROM ticket_attachments WHERE file_object_id=fo.id)
   FROM file_objects fo WHERE bucket=$1 AND object_key=$2 FOR UPDATE`, bucket, key, ticketID).
			Scan(&id, &purpose, &status, &creator, &ownerKind, &ownerID, &sameTicket, &alreadyBound)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrStorageObjectNotFound
		}
		if err != nil {
			return err
		}
		if status != "confirmed" {
			return ErrStorageObjectNotFound
		}
		expected := purposes[key]
		// 000047 deliberately left historical owner metadata NULL. The existing
		// same-ticket attachment relation remains authoritative in that case;
		// nonempty contradictory metadata must not be treated as legacy absence.
		ownerMatches := (ownerKind == "" && ownerID == "") ||
			(ownerKind == model.FileOwnerTicket && ownerID == strconv.FormatInt(ticketID, 10))
		if purpose == expected && ownerMatches && sameTicket {
			continue
		}
		dangling := "attachment_create"
		if expected == "ticket_richtext" {
			dangling = "richtext_create"
		}
		if purpose != dangling || creator != userID || ownerKind != "" || ownerID != "" || alreadyBound {
			return ErrStorageObjectNotFound
		}
		newKeys = append(newKeys, key)
	}
	return r.BindCreateAttachments(ctx, exec, bucket, userID, ticketID, newKeys, maxCount)
}

// BindUpdateAttachments takes every object lock in stable order before either
// input source is consumed. Explicit duplicate/previously-bound keys retain the
// original rejection behavior; field references may retain same-ticket objects.
func (r *StorageRepo) BindUpdateAttachments(ctx context.Context, exec rowExec, bucket string, userID, ticketID int64, explicit []string, refs []StorageFieldReference, maxCount int) error {
	unique := map[string]bool{}
	for _, key := range explicit {
		unique[key] = true
	}
	for _, ref := range refs {
		unique[ref.ObjectKey] = true
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		var id int64
		err := exec.QueryRow(ctx, `SELECT id FROM file_objects WHERE bucket=$1 AND object_key=$2 FOR UPDATE`, bucket, key).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrStorageObjectNotFound
		}
		if err != nil {
			return err
		}
	}
	if err := r.BindCreateAttachments(ctx, exec, bucket, userID, ticketID, explicit, maxCount); err != nil {
		return err
	}
	return r.BindFieldReferences(ctx, exec, bucket, userID, ticketID, refs, maxCount)
}
