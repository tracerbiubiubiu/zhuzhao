package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// FillRiskIdentityTx fills missing identity fields on the row already locked and
// advanced by the assessment. The assessment and its identity change roll back
// together if a supplied value differs from a field already present.
// Missing stored fields follow the dispatch basis's btrim rule. Non-blank
// values are compared as stored, including any padding.
func FillRiskIdentityTx(ctx context.Context, tx pgx.Tx, id, revision int64,
	findingKey, assetProvider, assetExternalID string) (*RiskItemRow, error) {
	row := tx.QueryRow(ctx, `UPDATE risk_items SET
		finding_key=$3,
		asset_provider=$4,
		asset_external_id=$5
		WHERE id=$1 AND revision=$2
			AND (NULLIF(btrim(finding_key), '') IS NULL OR finding_key=$3)
			AND (NULLIF(btrim(asset_provider), '') IS NULL OR asset_provider=$4)
			AND (NULLIF(btrim(asset_external_id), '') IS NULL OR asset_external_id=$5)
		RETURNING `+riskItemColumns, id, revision, findingKey, assetProvider, assetExternalID)
	risk, err := scanRiskItemRow(row)
	if errors.Is(err, ErrRiskNotFound) {
		return nil, ErrRiskInvalidState
	}
	return risk, err
}

// CurrentRiskSourceEvidenceTx reads the current applied mirror's evidence while
// the assessment holds the risk row lock. Report applies a mirror change only
// after acquiring that same risk lock, so no source-side lock is necessary.
func CurrentRiskSourceEvidenceTx(ctx context.Context, tx pgx.Tx, id int64) ([]byte, error) {
	var refs []byte
	err := tx.QueryRow(ctx, `SELECT current_source.mirror -> 'evidence_refs' FROM risk_items ri
		`+riskCurrentSourceJoin+` WHERE ri.id=$1`, id).Scan(&refs)
	if err != nil {
		return nil, fmt.Errorf("read current risk source evidence: %w", err)
	}
	return refs, nil
}
