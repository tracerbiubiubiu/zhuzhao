package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// RiskSuppressionWindow identifies the last administratively suppressed cycle.
type RiskSuppressionWindow struct {
	RiskID                 int64
	Active                 bool
	EffectiveUntil         time.Time
	ValidUntil             time.Time
	PreviousEffectiveUntil *time.Time
}

// GetRiskSuppressionWindowTx reads the latest window while the caller holds its family lock.
func GetRiskSuppressionWindowTx(ctx context.Context, tx pgx.Tx, familyID int64) (*RiskSuppressionWindow, error) {
	var out RiskSuppressionWindow
	err := tx.QueryRow(ctx, `
		SELECT s.risk_id, s.ended_at IS NULL AND s.valid_until > statement_timestamp(),
		       LEAST(s.valid_until, COALESCE(s.ended_at, s.valid_until)), s.valid_until,
		       (SELECT LEAST(prev.valid_until, COALESCE(prev.ended_at, prev.valid_until))
		        FROM risk_suppressions prev WHERE prev.family_id=s.family_id AND prev.id<s.id
		        ORDER BY prev.id DESC LIMIT 1)
		FROM risk_suppressions s
		WHERE s.family_id=$1
		ORDER BY s.id DESC LIMIT 1`, familyID).
		Scan(&out.RiskID, &out.Active, &out.EffectiveUntil, &out.ValidUntil, &out.PreviousEffectiveUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read risk suppression window: %w", err)
	}
	return &out, nil
}

// LockRiskFamilyByRiskTx establishes the source-report lock order before a risk row is locked.
func LockRiskFamilyByRiskTx(ctx context.Context, tx pgx.Tx, riskID int64) error {
	var familyID int64
	err := tx.QueryRow(ctx, `
		SELECT f.id FROM risk_items ri JOIN risk_families f ON f.id=ri.family_id
		WHERE ri.id=$1 FOR UPDATE OF f`, riskID).Scan(&familyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRiskNotFound
	}
	return err
}

func RiskHasActiveLinkTx(ctx context.Context, tx pgx.Tx, riskID int64) (bool, error) {
	return TxBool(ctx, tx, `SELECT EXISTS(SELECT 1 FROM risk_ticket_links WHERE risk_id=$1 AND ended_at IS NULL)`, riskID)
}

func CloseRiskFalsePositiveTx(ctx context.Context, tx pgx.Tx, riskID, expectedRevision int64) (*RiskItemRow, error) {
	result, err := scanRiskItemRow(tx.QueryRow(ctx, `
		UPDATE risk_items SET status='closed', closure_kind='false_positive', revision=revision+1,
		       closed_at=clock_timestamp(), updated_at=clock_timestamp()
		WHERE id=$1 AND revision=$2 AND status IN ('new','assessed') RETURNING `+riskItemColumns,
		riskID, expectedRevision))
	if errors.Is(err, ErrRiskNotFound) {
		return nil, errcode.ErrConcurrentModification
	}
	return result, err
}

func CreateRiskSuppressionTx(ctx context.Context, tx pgx.Tx, familyID, riskID, actor int64, validUntil time.Time, reason string) (model.RiskSuppressionRef, error) {
	result, err := scanSuppressionRef(tx.QueryRow(ctx, `
		WITH now AS MATERIALIZED (SELECT clock_timestamp() AS at)
		INSERT INTO risk_suppressions(family_id,risk_id,valid_until,reason,created_by,created_at)
		SELECT $1,$2,$3,$4,$5,now.at FROM now WHERE $3::timestamptz > now.at
		RETURNING id,revision,valid_until`, familyID, riskID, validUntil, reason, actor))
	if errors.Is(err, pgx.ErrNoRows) {
		return result, errcode.ErrInvalidParams
	}
	return result, err
}

func GetRiskIDForSuppressionTx(ctx context.Context, tx pgx.Tx, id int64) (int64, error) {
	var riskID int64
	err := tx.QueryRow(ctx, `SELECT risk_id FROM risk_suppressions WHERE id=$1`, id).Scan(&riskID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrRiskNotFound
	}
	return riskID, err
}

func EndRiskSuppressionTx(ctx context.Context, tx pgx.Tx, id, expectedRevision, actor int64, reason string) (model.RiskSuppressionRef, error) {
	result, err := scanSuppressionRef(tx.QueryRow(ctx, `
		WITH now AS MATERIALIZED (SELECT clock_timestamp() AS at)
		UPDATE risk_suppressions s SET ended_at=now.at, ended_by=$3, ended_reason=$4, revision=revision+1
		FROM now WHERE s.id=$1 AND s.revision=$2 AND s.ended_at IS NULL AND s.valid_until > now.at
		RETURNING s.id,s.revision,s.valid_until`, id, expectedRevision, actor, reason))
	if errors.Is(err, pgx.ErrNoRows) {
		return result, errcode.ErrConcurrentModification
	}
	return result, err
}

func GetSuppressionByRiskTx(ctx context.Context, tx pgx.Tx, riskID int64) (model.RiskSuppressionRef, error) {
	return scanSuppressionRef(tx.QueryRow(ctx, `SELECT id,revision,valid_until FROM risk_suppressions WHERE risk_id=$1`, riskID))
}

func scanSuppressionRef(row pgx.Row) (model.RiskSuppressionRef, error) {
	var result model.RiskSuppressionRef
	err := row.Scan(&result.ID, &result.Revision, &result.ValidUntil)
	return result, err
}

func ListRiskSuppressions(ctx context.Context, q IAMDBTX, actor int64, limit, offset int) ([]model.RiskSuppressionView, int64, error) {
	const scope = `FROM risk_suppressions s JOIN risk_items ri ON ri.id=s.risk_id
		JOIN organizations go ON go.id=ri.org_id WHERE `
	authorized := scope + riskManagePredicate("$1")
	var total int64
	if err := q.QueryRow(ctx, `SELECT count(*) `+authorized, actor).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT s.id,s.revision,s.valid_until,s.risk_id,s.reason,s.created_by,s.created_at,
			s.ended_at,s.ended_by,s.ended_reason,s.ended_at IS NULL AND s.valid_until>statement_timestamp() `+authorized+
		` ORDER BY s.id DESC LIMIT $2 OFFSET $3`, actor, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.RiskSuppressionView{}
	for rows.Next() {
		var item model.RiskSuppressionView
		if err := rows.Scan(&item.ID, &item.Revision, &item.ValidUntil, &item.RiskID, &item.Reason, &item.CreatedBy,
			&item.CreatedAt, &item.EndedAt, &item.EndedBy, &item.EndedReason, &item.Active); err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}
