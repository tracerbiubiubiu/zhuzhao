package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// StageSource returns the same durable stage reference on an identical
// redelivery. The first reception and its derived task commit together.
func (s *B3Store) StageSource(ctx context.Context, source, eventID, streamID string, cursor int64, hash, reference string, event json.RawMessage, derived *B3Execution) (string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		INSERT INTO activelist_source_inbox (source,event_id,stream_id,cursor,payload_hash,event,stage_commit_ref)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING
	`, source, eventID, streamID, cursor, hash, event, reference)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() == 0 {
		var originalHash string
		err := tx.QueryRow(ctx, `
			SELECT payload_hash,stage_commit_ref FROM activelist_source_inbox
			WHERE source=$1 AND event_id=$2
		`, source, eventID).Scan(&originalHash, &reference)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && hash != originalHash) {
			return "", errcode.ErrConflict
		}
		if err != nil {
			return "", err
		}
	} else if derived != nil {
		if err := s.Stage(ctx, tx, *derived); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return reference, nil
}
