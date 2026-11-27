package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// RiskIngestBatch stores a signed delivery and its original result separately
// from the observation ledger. A nil Result identifies a committed intake
// whose application has not committed.
type RiskIngestBatch struct {
	SourceRef        string
	ReceiptKey       string
	ReceiptID        string
	RequestHash      []byte
	OrgID            int64
	MappingRef       string
	FingerprintScope string
	KeySchemaVersion int
	Ordered          bool
	Observations     []byte
	Result           []byte
}

// LockRiskIngestSource serializes intake and application with source changes.
// Both transactions take this lock before looking up the batch row.
func LockRiskIngestSource(ctx context.Context, tx pgx.Tx, sourceRef string) (*RiskSourceRow, error) {
	var locked string
	err := tx.QueryRow(ctx, `SELECT source_ref FROM risk_sources WHERE source_ref=$1 FOR SHARE`, sourceRef).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRiskSourceUnknown
	}
	if err != nil {
		return nil, fmt.Errorf("lock risk ingest source: %w", err)
	}
	return GetRiskSource(ctx, tx, locked)
}

const riskIngestBatchColumns = `source_ref, receipt_key, batch_receipt::text, request_hash,
	org_id, mapping_ref, fingerprint_scope, key_schema_version, ordered, observations, result`

func scanRiskIngestBatch(row pgx.Row) (RiskIngestBatch, error) {
	var b RiskIngestBatch
	err := row.Scan(&b.SourceRef, &b.ReceiptKey, &b.ReceiptID, &b.RequestHash,
		&b.OrgID, &b.MappingRef, &b.FingerprintScope, &b.KeySchemaVersion, &b.Ordered,
		&b.Observations, &b.Result)
	return b, err
}

func ClaimRiskIngestBatch(ctx context.Context, tx pgx.Tx, b RiskIngestBatch) (RiskIngestBatch, error) {
	_, err := tx.Exec(ctx, `INSERT INTO risk_ingest_batches
		(source_ref, receipt_key, batch_receipt, request_hash, org_id, mapping_ref, fingerprint_scope,
		 key_schema_version, ordered, observations)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (source_ref, receipt_key) DO NOTHING`,
		b.SourceRef, b.ReceiptKey, b.ReceiptID, b.RequestHash, b.OrgID, b.MappingRef, b.FingerprintScope,
		b.KeySchemaVersion, b.Ordered, b.Observations)
	if err != nil {
		return RiskIngestBatch{}, fmt.Errorf("claim risk ingest batch: %w", err)
	}
	return scanRiskIngestBatch(tx.QueryRow(ctx, `SELECT `+riskIngestBatchColumns+`
		FROM risk_ingest_batches WHERE source_ref=$1 AND receipt_key=$2`, b.SourceRef, b.ReceiptKey))
}

func ReadRiskIngestBatch(ctx context.Context, db IAMDBTX, sourceRef, receiptID string) (RiskIngestBatch, error) {
	return scanRiskIngestBatch(db.QueryRow(ctx, `SELECT `+riskIngestBatchColumns+`
		FROM risk_ingest_batches WHERE source_ref=$1 AND batch_receipt=$2`, sourceRef, receiptID))
}

func LockRiskIngestBatch(ctx context.Context, tx pgx.Tx, sourceRef, receiptID string) (RiskIngestBatch, error) {
	return scanRiskIngestBatch(tx.QueryRow(ctx, `SELECT `+riskIngestBatchColumns+`
		FROM risk_ingest_batches WHERE source_ref=$1 AND batch_receipt=$2 FOR UPDATE`, sourceRef, receiptID))
}

func FinishRiskIngestBatch(ctx context.Context, tx pgx.Tx, sourceRef, receiptID string, result []byte) error {
	tag, err := tx.Exec(ctx, `UPDATE risk_ingest_batches SET result=$3
		WHERE source_ref=$1 AND batch_receipt=$2 AND result IS NULL`, sourceRef, receiptID, result)
	if err != nil {
		return fmt.Errorf("finish risk ingest batch: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("risk ingest batch is not pending")
	}
	return nil
}

// PendingRiskIngestBatches skips receipts whose pinned source configuration is
// no longer current. Application repeats the comparison under the source lock.
func PendingRiskIngestBatches(ctx context.Context, db IAMDBTX, sourceRef string, limit int) ([]string, error) {
	// Avoid loading the observation payloads until their batches are handled.
	rows, err := db.Query(ctx, `SELECT b.batch_receipt::text FROM risk_ingest_batches b
		JOIN risk_sources s ON s.source_ref=b.source_ref
		JOIN organizations o ON o.id=b.org_id
		WHERE b.source_ref=$1 AND b.result IS NULL AND s.enabled
		  AND b.org_id=s.default_org_id AND b.org_id=ANY(s.allowed_org_ids)
		  AND b.mapping_ref=s.mapping_ref AND b.fingerprint_scope=s.fingerprint_scope
		  AND b.key_schema_version=s.key_schema_version AND b.ordered=s.ordered
		  AND (`+OrgSourceActiveSQL("o")+`)
		ORDER BY b.last_attempt_at NULLS FIRST, b.received_at, b.batch_receipt LIMIT $2`, sourceRef, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending risk ingest batches: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func DeferRiskIngestBatchRetry(ctx context.Context, db IAMDBTX, sourceRef, receiptID string) error {
	_, err := db.Exec(ctx, `UPDATE risk_ingest_batches SET last_attempt_at=clock_timestamp()
		WHERE source_ref=$1 AND batch_receipt=$2 AND result IS NULL`, sourceRef, receiptID)
	return err
}
