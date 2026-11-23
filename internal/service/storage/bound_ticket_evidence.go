package storage

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// BoundTicketEvidenceRef identifies an already confirmed ticket attachment.
// Version is the exact ETag recorded by storage confirmation, without rewriting.
type BoundTicketEvidenceRef struct {
	ObjectKey string
	Version   string
}

// BoundTicketEvidenceValidator owns validation against one fact database and a
// fixed bucket. Rebuild it when changing the configured bucket. It never uploads,
// confirms, binds, or reads the physical object store.
type BoundTicketEvidenceValidator struct {
	pool   *pgxpool.Pool
	bucket string
}

func NewBoundTicketEvidenceValidator(pool *pgxpool.Pool, bucket string) *BoundTicketEvidenceValidator {
	return &BoundTicketEvidenceValidator{pool: pool, bucket: bucket}
}

// ValidateBoundTicketEvidenceTx consumes the exact caller-owned IAM transaction.
// The caller must first authorize current ticket readability/action and hold the
// ticket row lock, after all group/risk/link locks. All file rows are then locked
// in ID order before their current binding and metadata are inspected. Keep the
// transaction open through the evidence write; return every error to its owner.
// A bare tx, a child savepoint with its parent's context, or another pool's guard
// is rejected. No authorization capability is implied by a successful result.
func (v *BoundTicketEvidenceValidator) ValidateBoundTicketEvidenceTx(ctx context.Context, tx pgx.Tx, ticketID int64, refs []BoundTicketEvidenceRef) error {
	if v == nil || v.pool == nil || tx == nil || !repository.IAMGuardActive(ctx) || repository.IAMDB(ctx, v.pool) != tx {
		return errcode.ErrNoPermission
	}
	if v.bucket == "" || ticketID <= 0 || len(refs) > 50 {
		return errcode.ErrInvalidParams
	}
	versions := make(map[string]string, len(refs))
	keys := make([]string, 0, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref.ObjectKey) == "" || len(ref.ObjectKey) > 500 || strings.TrimSpace(ref.Version) == "" || len(ref.Version) > 128 {
			return errcode.ErrInvalidParams
		}
		if !strings.HasPrefix(ref.ObjectKey, "tickets/") {
			return errcode.ErrFileNotFound
		}
		if old, exists := versions[ref.ObjectKey]; exists {
			if old != ref.Version {
				return errcode.ErrFileNotFound
			}
			continue
		}
		versions[ref.ObjectKey] = ref.Version
		keys = append(keys, ref.ObjectKey)
	}
	if len(keys) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT id,object_key FROM file_objects
		WHERE bucket=$1 AND object_key=ANY($2)
		AND purpose IN ('ticket_attachment','ticket_richtext') ORDER BY id FOR UPDATE`, v.bucket, keys)
	if err != nil {
		return err
	}
	type object struct {
		id  int64
		key string
	}
	objects, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (object, error) {
		var obj object
		err := row.Scan(&obj.id, &obj.key)
		return obj, err
	})
	if err != nil {
		return err
	}
	if len(objects) != len(keys) {
		return errcode.ErrFileNotFound
	}
	for _, obj := range objects {
		var valid bool
		// A new statement after all file locks observes changes committed while
		// waiting. Legacy empty owner metadata is accepted only with a real link;
		// nonempty contradictory ownership is never treated as legacy absence.
		err := tx.QueryRow(ctx, `SELECT COALESCE(status='confirmed' AND confirmed_at IS NOT NULL
			AND etag=$2 AND purpose IN ('ticket_attachment','ticket_richtext')
			AND ((COALESCE(owner_kind,'')='' AND COALESCE(owner_id,'')='') OR (owner_kind='ticket' AND owner_id=$4))
			AND EXISTS(SELECT 1 FROM ticket_attachments WHERE file_object_id=f.id AND ticket_id=$3)
			AND NOT EXISTS(SELECT 1 FROM ticket_attachments WHERE file_object_id=f.id AND ticket_id<>$3),false)
			FROM file_objects f WHERE id=$1 AND bucket=$5`, obj.id, versions[obj.key], ticketID, strconv.FormatInt(ticketID, 10), v.bucket).Scan(&valid)
		if errors.Is(err, pgx.ErrNoRows) {
			return errcode.ErrFileNotFound
		}
		if err != nil {
			return err
		}
		if !valid {
			return errcode.ErrFileNotFound
		}
	}
	return nil
}
