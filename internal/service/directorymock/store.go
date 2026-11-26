package directorymock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func lockScope(ctx context.Context, tx pgx.Tx, scope string) (baseline, cursor string, err error) {
	_, err = tx.Exec(ctx, `INSERT INTO directory_mock_scopes (scope_ref) VALUES ($1) ON CONFLICT DO NOTHING`, scope)
	if err != nil {
		return "", "", fmt.Errorf("ensure mock directory scope: %w", err)
	}
	err = tx.QueryRow(ctx, `SELECT success_baseline, success_cursor FROM directory_mock_scopes
		WHERE scope_ref = $1 FOR UPDATE`, scope).Scan(&baseline, &cursor)
	if err != nil {
		return "", "", fmt.Errorf("lock mock directory scope: %w", err)
	}
	return baseline, cursor, nil
}

func storedReceipt(ctx context.Context, tx pgx.Tx, batch Batch, hash string) (Receipt, bool, error) {
	var originalHash string
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT input_hash, receipt FROM directory_mock_runs
		WHERE scope_ref = $1 AND batch_id = $2`, batch.ScopeRef, batch.BatchID).Scan(&originalHash, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Receipt{}, false, nil
	}
	if err != nil {
		return Receipt{}, false, fmt.Errorf("read mock directory receipt: %w", err)
	}
	if originalHash != hash {
		return Receipt{}, false, ErrBatchConflict
	}
	var receipt Receipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return Receipt{}, false, fmt.Errorf("decode mock directory receipt: %w", err)
	}
	return receipt, true, nil
}

func advanceScope(ctx context.Context, tx pgx.Tx, receipt Receipt) error {
	_, err := tx.Exec(ctx, `UPDATE directory_mock_scopes SET success_baseline = $2, success_cursor = $3
		WHERE scope_ref = $1`, receipt.ScopeRef, receipt.BaselineAfter, receipt.CursorAfter)
	if err != nil {
		return fmt.Errorf("advance mock directory watermark: %w", err)
	}
	return nil
}

func saveReceipt(ctx context.Context, tx pgx.Tx, receipt Receipt, hash string) error {
	_, err := tx.Exec(ctx, `INSERT INTO directory_mock_runs (scope_ref, batch_id, run_id, input_hash, receipt)
		VALUES ($1, $2, $3, $4, $5)`, receipt.ScopeRef, receipt.BatchID, receipt.RunID, hash, receipt)
	if err != nil {
		return fmt.Errorf("write mock directory receipt: %w", err)
	}
	return nil
}

// writePage holds a savepoint so a revision error cannot leave a half-page update.
func writePage(ctx context.Context, tx pgx.Tx, scope string, departments []Department) ([]RecordResult, string, error) {
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("begin mock directory page: %w", err)
	}
	defer func() { _ = savepoint.Rollback(ctx) }()

	results := make([]RecordResult, 0, len(departments))
	for _, dept := range departments {
		outcome, problem, err := writeDepartment(ctx, savepoint, scope, dept)
		if err != nil {
			return nil, "", err
		}
		if problem != "" {
			if err := savepoint.Rollback(ctx); err != nil {
				return nil, "", fmt.Errorf("roll back mock directory page: %w", err)
			}
			return nil, problem, nil
		}
		results = append(results, RecordResult{ExternalID: dept.ExternalID, SourceRevision: dept.SourceRevision, Outcome: outcome})
	}
	if err := savepoint.Commit(ctx); err != nil {
		return nil, "", fmt.Errorf("finish mock directory page: %w", err)
	}
	return results, "", nil
}

func writeDepartment(ctx context.Context, tx pgx.Tx, scope string, dept Department) (outcome, problem string, err error) {
	var name string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT display_name, source_revision FROM directory_mock_departments
		WHERE scope_ref = $1 AND external_id = $2`, scope, dept.ExternalID).Scan(&name, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err := tx.Exec(ctx, `INSERT INTO directory_mock_departments (scope_ref, external_id, display_name, source_revision)
			VALUES ($1, $2, $3, $4)`, scope, dept.ExternalID, dept.DisplayName, dept.SourceRevision)
		if err != nil {
			return "", "", fmt.Errorf("create mock department: %w", err)
		}
		return "created", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("read mock department: %w", err)
	}
	if dept.SourceRevision < revision {
		return "", "source_revision_stale", nil
	}
	if dept.SourceRevision == revision {
		if dept.DisplayName != name {
			return "", "source_revision_conflict", nil
		}
		return "unchanged", "", nil
	}
	_, err = tx.Exec(ctx, `UPDATE directory_mock_departments SET display_name = $3, source_revision = $4, updated_at = clock_timestamp()
		WHERE scope_ref = $1 AND external_id = $2`, scope, dept.ExternalID, dept.DisplayName, dept.SourceRevision)
	if err != nil {
		return "", "", fmt.Errorf("update mock department: %w", err)
	}
	return "updated", "", nil
}
