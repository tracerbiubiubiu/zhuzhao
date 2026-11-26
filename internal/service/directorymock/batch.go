package directorymock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	StatusSucceeded = "succeeded"
	statusPartial   = "partial"
	statusFailed    = "failed"
)

type Receipt struct {
	RunID          string         `json:"run_id"`
	ScopeRef       string         `json:"scope_ref"`
	BatchID        string         `json:"batch_id"`
	Status         string         `json:"status"`
	AppliedPages   int            `json:"applied_pages"`
	PageWatermark  string         `json:"page_watermark"`
	BaselineBefore string         `json:"baseline_before"`
	BaselineAfter  string         `json:"baseline_after"`
	CursorAfter    string         `json:"cursor_after"`
	Records        []RecordResult `json:"records"`
	Problem        string         `json:"problem,omitempty"`
}

type RecordResult struct {
	ExternalID     string `json:"external_id"`
	SourceRevision int64  `json:"source_revision"`
	Outcome        string `json:"outcome"`
}

// Apply serializes one scope and commits its accepted pages and receipt together.
// An unknown commit result can be reconciled by submitting the same batch again.
func Apply(ctx context.Context, conn *pgx.Conn, batch Batch) (receipt Receipt, err error) {
	if err := batch.validate(); err != nil {
		return Receipt{}, err
	}
	encoded, err := json.Marshal(batch)
	if err != nil {
		return Receipt{}, fmt.Errorf("encode mock batch: %w", err)
	}
	digest := sha256.Sum256(encoded)
	hash := hex.EncodeToString(digest[:])

	err = pgx.BeginTxFunc(ctx, conn, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		var applyErr error
		receipt, applyErr = applyBatch(ctx, tx, batch, hash)
		return applyErr
	})
	if err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func applyBatch(ctx context.Context, tx pgx.Tx, batch Batch, hash string) (Receipt, error) {
	baseline, cursor, err := lockScope(ctx, tx, batch.ScopeRef)
	if err != nil {
		return Receipt{}, err
	}
	stored, found, err := storedReceipt(ctx, tx, batch, hash)
	if err != nil || found {
		return stored, err
	}
	if baseline != batch.ExpectedBaseline {
		return Receipt{}, ErrBaselineConflict
	}

	runID, err := uuid.NewRandom()
	if err != nil {
		return Receipt{}, fmt.Errorf("allocate mock directory run: %w", err)
	}
	receipt := Receipt{
		RunID: runID.String(), ScopeRef: batch.ScopeRef, BatchID: batch.BatchID,
		BaselineBefore: baseline, BaselineAfter: baseline, CursorAfter: cursor,
		Records: []RecordResult{},
	}
	if err := applyPages(ctx, tx, batch, &receipt); err != nil {
		return Receipt{}, err
	}
	if receipt.Status == StatusSucceeded {
		receipt.BaselineAfter = batch.BatchID
		receipt.CursorAfter = batch.SuccessCursor
		if err := advanceScope(ctx, tx, receipt); err != nil {
			return Receipt{}, err
		}
	}
	return receipt, saveReceipt(ctx, tx, receipt, hash)
}

func applyPages(ctx context.Context, tx pgx.Tx, batch Batch, receipt *Receipt) error {
	seenCursors := map[string]bool{"": true}
	seenRecords := make(map[string]bool)
	for index, page := range batch.Pages {
		last := index == len(batch.Pages)-1
		if problem := pageProblem(page, receipt.PageWatermark, last, seenCursors, seenRecords); problem != "" {
			receipt.interrupt(problem)
			return nil
		}

		results, problem, err := writePage(ctx, tx, batch.ScopeRef, page.Records)
		if err != nil {
			return err
		}
		if problem != "" {
			receipt.interrupt(problem)
			return nil
		}

		receipt.Records = append(receipt.Records, results...)
		receipt.AppliedPages++
		receipt.PageWatermark = page.NextCursor
		if page.Done {
			receipt.Status = StatusSucceeded
			return nil
		}
		seenCursors[page.NextCursor] = true
	}
	receipt.interrupt("page_missing")
	return nil
}

func pageProblem(page Page, expected string, last bool, cursors, externalIDs map[string]bool) string {
	if page.Cursor != expected {
		return "page_cursor_mismatch"
	}
	if page.Error != "" {
		if page.Error == "fetch_failed" && last && !page.Done && page.NextCursor == "" && len(page.Records) == 0 {
			return "fetch_failed"
		}
		return "invalid_page"
	}
	if page.Done {
		if !last || page.NextCursor != "" {
			return "invalid_page"
		}
	} else if !validRef(page.NextCursor) || cursors[page.NextCursor] {
		return "invalid_page"
	}
	if len(page.Records) > maxRecords {
		return "invalid_page"
	}
	for _, record := range page.Records {
		if !record.valid() {
			return "invalid_department"
		}
		if externalIDs[record.ExternalID] {
			return "duplicate_department"
		}
		externalIDs[record.ExternalID] = true
	}
	return ""
}

func (receipt *Receipt) interrupt(problem string) {
	receipt.Problem = problem
	receipt.Status = statusFailed
	if receipt.AppliedPages > 0 {
		receipt.Status = statusPartial
	}
}
