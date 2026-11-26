package riskreminder

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// ScanReport exposes scan-time suppression as a durable result, not a delivery.
type ScanReport struct {
	Generated  []*Intent
	Suppressed []*Intent
	Failed     []ScanFailure
}

// ScanFailure is a durable pending attempt; it is not a qualified reminder.
type ScanFailure struct {
	AssignmentID int64
	Err          error
}

// GenerateEligibleDue is the runtime scan entry. GenerateDue/GenerateDueTx are
// low-level persistence helpers and do not establish recipient eligibility.
// A bounded unkeyed candidate list is revalidated under IAM→ticket ordering;
// no group/risk locks are taken after the ticket. Keyed rows never consume quota.
// Qualification read/configuration failures persist pending attempts under the
// original key, so failed candidates cannot permanently starve later tickets.
func (s *Service) GenerateEligibleDue(ctx context.Context, asOf time.Time, limit int) (*ScanReport, error) {
	if s.checker == nil {
		return nil, errors.New("riskreminder: eligibility checker not wired")
	}
	if asOf.IsZero() {
		return nil, errors.New("riskreminder: scan clock is required")
	}
	if limit <= 0 {
		limit = DefaultScanLimit
	}
	rows, err := s.db.Query(ctx, `SELECT a.id,a.ticket_id FROM riskreminder_assignments a
 WHERE a.accepted_at IS NULL AND a.accept_due_at <= $1
 AND NOT EXISTS(SELECT 1 FROM riskreminder_assignments n WHERE n.ticket_id=a.ticket_id AND n.generation>a.generation)
 AND NOT EXISTS(SELECT 1 FROM riskreminder_intents i WHERE i.assignment_id=a.id)
 ORDER BY a.accept_due_at,a.id LIMIT $2`, asOf, limit)
	if err != nil {
		return nil, err
	}
	type candidate struct{ id, ticket int64 }
	candidates, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (candidate, error) {
		var c candidate
		err := r.Scan(&c.id, &c.ticket)
		return c, err
	})
	if err != nil {
		return nil, err
	}
	report := &ScanReport{}
	var failures []error
	for _, c := range candidates {
		var intent *Intent
		var qualificationErr error
		err := repository.WithIAMCommandGuard(ctx, s.db, func(ctx context.Context, tx pgx.Tx) error {
			var id int64
			err := tx.QueryRow(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, c.ticket).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			var recipient int64
			err = tx.QueryRow(ctx, `SELECT a.assignee_id FROM riskreminder_assignments a
    WHERE a.id=$1 AND a.accepted_at IS NULL AND a.accept_due_at <= $2
    AND NOT EXISTS(SELECT 1 FROM riskreminder_assignments n WHERE n.ticket_id=a.ticket_id AND n.generation>a.generation)
    AND NOT EXISTS(SELECT 1 FROM riskreminder_intents i WHERE i.assignment_id=a.id)`, c.id, asOf).Scan(&recipient)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			// A SQL read failure can abort the transaction. Isolate the checker in
			// a savepoint while retaining the exact guarded tx identity it reads.
			sp, err := tx.Begin(ctx)
			if err != nil {
				return err
			}
			eligible, reason, checkErr := s.checker(ctx, tx, c.ticket, recipient)
			if checkErr != nil {
				if err := sp.Rollback(ctx); err != nil {
					return errors.Join(checkErr, err)
				}
				qualificationErr = checkErr
			} else if err := sp.Commit(ctx); err != nil {
				return err
			}
			intent, err = scanIntent(tx.QueryRow(ctx, `INSERT INTO riskreminder_intents
    (assignment_id,ticket_id,generation,recipient_id,due_at,source,source_event_id,rule_id,rule_version,channel,semantic_key)
    SELECT id,ticket_id,generation,assignee_id,accept_due_at,$2,id::text,rule_id,rule_version,$3,$4
    FROM riskreminder_assignments WHERE id=$1
    ON CONFLICT(source,source_event_id,rule_id,rule_version,recipient_id,channel,semantic_key) DO NOTHING RETURNING `+intentCols, c.id, Source, Channel, SemanticKey))
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			if qualificationErr != nil {
				_, err = tx.Exec(ctx, `UPDATE riskreminder_intents SET attempt_count=attempt_count+1,last_error=$2,last_attempted_at=clock_timestamp() WHERE id=$1`, intent.ID, truncateRunes(qualificationErr.Error(), 512))
				return err
			}
			if !eligible {
				if reason == "" {
					reason = "eligibility_withdrawn"
				}
				intent, err = suppressIntentTx(ctx, tx, intent.ID, truncateRunes(reason, 128))
			}
			return err
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("riskreminder assignment %d: %w", c.id, err))
			continue
		}
		if qualificationErr != nil {
			failure := fmt.Errorf("riskreminder assignment %d: %w", c.id, qualificationErr)
			failures = append(failures, failure)
			report.Failed = append(report.Failed, ScanFailure{AssignmentID: c.id, Err: qualificationErr})
			continue
		}
		if intent != nil {
			if intent.Status == StatusSuppressed {
				report.Suppressed = append(report.Suppressed, intent)
			} else {
				report.Generated = append(report.Generated, intent)
			}
		}
	}
	return report, errors.Join(failures...)
}
