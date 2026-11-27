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
		reference, err = b3SourceStage(ctx, tx, source, eventID, hash)
		if errors.Is(err, pgx.ErrNoRows) {
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

// ExistingSourceStage checks a previously committed event without staging work.
func (s *B3Store) ExistingSourceStage(ctx context.Context, source, eventID, hash string) (string, error) {
	return b3SourceStage(ctx, s.db, source, eventID, hash)
}

func b3SourceStage(ctx context.Context, db IAMDBTX, source, eventID, hash string) (string, error) {
	var originalHash, reference string
	err := db.QueryRow(ctx, `
		SELECT payload_hash,stage_commit_ref FROM activelist_source_inbox
		WHERE source=$1 AND event_id=$2
	`, source, eventID).Scan(&originalHash, &reference)
	if err != nil {
		return "", err
	}
	if hash != originalHash {
		return "", errcode.ErrConflict
	}
	return reference, nil
}
