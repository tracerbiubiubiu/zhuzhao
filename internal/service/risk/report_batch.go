package risk

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const maxReportPayloadBytes = 1 << 20

// ReportFromSource binds intake and recovery to the verified feed identity.
func (s *Service) ReportFromSource(ctx context.Context, req model.RiskReportRequest, sourceRef string) (*model.RiskReportResult, error) {
	if sourceRef == "" {
		return nil, errcode.ErrNoPermission
	}
	if _, err := validateRiskReport(req, sourceRef); err != nil {
		return nil, err
	}
	if repository.IAMGuardActive(ctx) {
		return nil, errors.New("risk ingest requires independent transactions")
	}
	batch, err := prepareReportBatch(req, sourceRef)
	if err != nil {
		return nil, err
	}
	err = repository.WithIAMCommandGuard(ctx, s.db, func(ctx context.Context, tx pgx.Tx) error {
		src, err := lockReportSource(ctx, tx, sourceRef)
		if err != nil {
			return err
		}
		batch.OrgID = *src.DefaultOrgID
		batch.MappingRef = src.MappingRef
		batch.FingerprintScope = src.FingerprintScope
		batch.KeySchemaVersion = src.KeySchemaVersion
		batch.Ordered = src.Ordered

		saved, err := repository.ClaimRiskIngestBatch(ctx, tx, batch)
		if err != nil {
			var dbErr *pgconn.PgError
			// PostgreSQL JSONB cannot store a NUL code point even if escaped.
			if errors.As(err, &dbErr) && dbErr.Code == "22P05" {
				return errcode.ErrInvalidParams
			}
			return err
		}
		if err := checkReportScope(src, saved); err != nil {
			return err
		}
		if !bytes.Equal(saved.RequestHash, batch.RequestHash) {
			return ErrRiskOperationConflict
		}
		batch = saved
		return nil
	})
	if err != nil {
		return nil, err
	}
	// The stage transaction has committed. Use the original context so this
	// application starts a new transaction and can recover from another process.
	return s.applyReportBatch(ctx, batch)
}

func prepareReportBatch(req model.RiskReportRequest, sourceRef string) (repository.RiskIngestBatch, error) {
	if len(req.BatchID) > 128 || strings.ContainsRune(req.BatchID, 0) {
		return repository.RiskIngestBatch{}, errcode.ErrInvalidParams
	}
	var rawInput bytes.Buffer
	encoder := json.NewEncoder(&rawInput)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(req.Observations)
	if err != nil {
		return repository.RiskIngestBatch{}, errcode.ErrInvalidParams
	}
	if rawInput.Len()-1 > maxReportPayloadBytes {
		return repository.RiskIngestBatch{}, ErrRiskBatchTooLarge
	}
	staged := newReportObservations(req.Observations)
	fingerprints := make([]observationFingerprint, len(req.Observations))
	for i, input := range req.Observations {
		if !validReportLedgerFields(input) {
			return repository.RiskIngestBatch{}, errcode.ErrInvalidParams
		}
		fingerprint := fingerprintObservation(input)
		staged[i].ContentHash = canonicalHash(fingerprint)
		// The batch contract keeps JSON numbers as given. Apply continues to
		// use the original per-event hash saved before JSONB normalization.
		if len(input.Attributes) != 0 {
			decoder := json.NewDecoder(bytes.NewReader(input.Attributes))
			decoder.UseNumber()
			if err := decoder.Decode(&fingerprint.Attributes); err != nil {
				return repository.RiskIngestBatch{}, errcode.ErrInvalidParams
			}
		}
		fingerprints[i] = fingerprint
	}
	hash := canonicalHash(fingerprints)
	key := "hash:" + hex.EncodeToString(hash)
	if req.BatchID != "" {
		key = "id:" + req.BatchID
	}
	observations, err := json.Marshal(staged)
	if err != nil {
		return repository.RiskIngestBatch{}, errcode.ErrInvalidParams
	}
	return repository.RiskIngestBatch{
		SourceRef: sourceRef, ReceiptKey: key, ReceiptID: uuid.NewString(),
		RequestHash: hash, Observations: observations,
	}, nil
}

func validReportLedgerFields(input model.FindingObservationV1) bool {
	return utf8.RuneCountInString(input.SourceItemKey) <= 300 &&
		utf8.RuneCountInString(input.EventID) <= 200 &&
		utf8.RuneCountInString(input.MappingRef) <= 200
}

func lockReportSource(ctx context.Context, tx pgx.Tx, sourceRef string) (*repository.RiskSourceRow, error) {
	src, err := repository.LockRiskIngestSource(ctx, tx, sourceRef)
	if err != nil {
		return nil, err
	}
	if !src.Enabled {
		return nil, ErrRiskSourceUnknown
	}
	if src.DefaultOrgID == nil || !containsInt64(src.AllowedOrgIDs, *src.DefaultOrgID) {
		return nil, ErrRiskSourceScope
	}
	active, err := repository.TxBool(ctx, tx, `SELECT EXISTS (SELECT 1 FROM organizations o
		WHERE o.id=$1 AND (`+repository.OrgSourceActiveSQL("o")+`))`, *src.DefaultOrgID)
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, ErrRiskSourceScope
	}
	return src, nil
}

func checkReportScope(src *repository.RiskSourceRow, batch repository.RiskIngestBatch) error {
	if *src.DefaultOrgID != batch.OrgID || src.MappingRef != batch.MappingRef ||
		src.FingerprintScope != batch.FingerprintScope || src.KeySchemaVersion != batch.KeySchemaVersion ||
		src.Ordered != batch.Ordered {
		return ErrRiskSourceScope
	}
	return nil
}

func (s *Service) applyReportBatch(ctx context.Context, batch repository.RiskIngestBatch) (*model.RiskReportResult, error) {
	var observations []reportObservation
	var keys []string
	if batch.Result == nil {
		if err := json.Unmarshal(batch.Observations, &observations); err != nil {
			return nil, fmt.Errorf("decode risk ingest observations: %w", err)
		}
		var err error
		keys, err = s.reserveRiskNumbers(ctx, observations)
		if err != nil {
			return nil, err
		}
	}

	var result model.RiskReportResult
	err := repository.WithIAMCommandGuard(ctx, s.db, func(ctx context.Context, tx pgx.Tx) error {
		src, err := lockReportSource(ctx, tx, batch.SourceRef)
		if err != nil {
			return err
		}
		current, err := repository.LockRiskIngestBatch(ctx, tx, batch.SourceRef, batch.ReceiptID)
		if err != nil {
			return err
		}
		if err := checkReportScope(src, current); err != nil {
			return err
		}
		if current.Result != nil {
			if err := json.Unmarshal(current.Result, &result); err != nil {
				return fmt.Errorf("decode risk ingest result: %w", err)
			}
			duplicate := !strings.HasPrefix(current.ReceiptKey, "id:")
			return projectReportResult(ctx, tx, src, &result, duplicate)
		}
		items, err := s.applyReport(ctx, tx, src, observations, 0, keys)
		if err != nil {
			return err
		}
		result = model.RiskReportResult{BatchReceipt: current.ReceiptID, Items: items}
		if err := projectReportResult(ctx, tx, src, &result, false); err != nil {
			return err
		}
		return repository.FinishRiskIngestBatch(ctx, tx, current.SourceRef, current.ReceiptID, jsonBytes(result))
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// projectReportResult rechecks risk references from older or overlapping
// deliveries under the current source authorization.
func projectReportResult(ctx context.Context, tx pgx.Tx, src *repository.RiskSourceRow, result *model.RiskReportResult, duplicate bool) error {
	for i := range result.Items {
		item := &result.Items[i]
		if duplicate && item.Result != "quarantined" {
			item.Result = "duplicate"
		}
		if item.RiskRef == nil {
			continue
		}
		var revision int64
		err := tx.QueryRow(ctx, `SELECT revision FROM risk_items WHERE id=$1 AND org_id=$2`,
			item.RiskRef.ID, *src.DefaultOrgID).Scan(&revision)
		if errors.Is(err, pgx.ErrNoRows) {
			item.RiskRef = nil
			continue
		}
		if err != nil {
			return fmt.Errorf("scope risk ingest result: %w", err)
		}
		if duplicate {
			item.RiskRef.Revision = revision
		}
	}
	return nil
}

// ResumeSourceReports continues bounded durable intake for the configured
// source. Application rechecks its pinned configuration and shares the same
// transaction and locks as live requests.
func (s *Service) ResumeSourceReports(ctx context.Context, sourceRef string, limit int) error {
	if sourceRef == "" || limit < 1 || limit > maxReportBatch || repository.IAMGuardActive(ctx) {
		return errcode.ErrInvalidParams
	}
	pending, err := repository.PendingRiskIngestBatches(ctx, s.db, sourceRef, limit)
	if err != nil {
		return err
	}
	var failures error
	for _, receiptID := range pending {
		if err := ctx.Err(); err != nil {
			return errors.Join(failures, err)
		}
		batch, err := repository.ReadRiskIngestBatch(ctx, s.db, sourceRef, receiptID)
		if err == nil {
			_, err = s.applyReportBatch(ctx, batch)
		}
		if err == nil || errors.Is(err, ErrRiskSourceScope) || errors.Is(err, ErrRiskSourceUnknown) {
			continue
		}
		// Keep a failed batch eligible but let untouched batches run first.
		failures = errors.Join(failures, err,
			repository.DeferRiskIngestBatchRetry(ctx, s.db, sourceRef, receiptID))
	}
	return failures
}
