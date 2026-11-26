//go:build integration

package riskremediation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// This adapter persists synthetic file bindings on the supplied transaction.
// Unlike the original syntax-only fixture, a misplaced callback is observable
// as a durable attachment write or a duplicate-binding error.
func evidenceBindingFixture(t *testing.T, count int) *pgFixture {
	t.Helper()
	f := newPGFixture(t, count)
	_, err := f.pool.Exec(f.ctx, `CREATE TABLE fixture_evidence_bindings(ref TEXT,version TEXT,link_id BIGINT NOT NULL,action TEXT NOT NULL,PRIMARY KEY(ref,version))`)
	require.NoError(t, err)
	callbacks := f.callbacks()
	callbacks.Evidence = func(ctx context.Context, tx pgx.Tx, c EvidenceCheck) error {
		if (c.Mode != EvidenceValidate && c.Mode != EvidenceBind) || (c.Historical && c.Mode != EvidenceValidate) {
			return errcode.ErrInvalidParams
		}
		for _, ref := range c.Evidence {
			if ref.Kind != "file" || ref.Ref == "" || ref.Version != "1" {
				return errcode.ErrInvalidParams
			}
			if c.Mode == EvidenceValidate {
				var here, elsewhere bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fixture_evidence_bindings WHERE ref=$1 AND version=$2 AND link_id=$3),EXISTS(SELECT 1 FROM fixture_evidence_bindings WHERE ref=$1 AND version=$2 AND link_id<>$3)`, ref.Ref, ref.Version, c.LinkID).Scan(&here, &elsewhere); err != nil {
					return err
				}
				if elsewhere || (c.Historical && !here) {
					return errcode.ErrFileAlreadyBound
				}
				continue
			}
			command, err := tx.Exec(ctx, `INSERT INTO fixture_evidence_bindings(ref,version,link_id,action) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, ref.Ref, ref.Version, c.LinkID, c.Action)
			if err != nil {
				return err
			}
			if command.RowsAffected() != 1 {
				return errcode.ErrFileAlreadyBound
			}
		}
		return nil
	}
	f.svc = New(f.pool, callbacks)
	return f
}

func fileEvidence(ref string) []EvidenceRef {
	return []EvidenceRef{{Kind: "file", Ref: ref, Version: "1"}}
}

func evidenceBindings(t *testing.T, f *pgFixture) []string {
	t.Helper()
	rows, err := f.pool.Query(f.ctx, `SELECT ref FROM fixture_evidence_bindings ORDER BY ref`)
	require.NoError(t, err)
	refs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	return refs
}

func TestFinishEvidenceBindingsExcludeEveryConflictedItem(t *testing.T) {
	for _, conflict := range []string{"link_revision", "risk_revision", "pending_proposal"} {
		t.Run(conflict, func(t *testing.T) {
			f := evidenceBindingFixture(t, 2)
			if conflict == "pending_proposal" {
				_, err := f.pool.Exec(f.ctx, `UPDATE risk_ticket_links SET remediation_state='pending_verify' WHERE id=$1`, f.links[1])
				require.NoError(t, err)
			}
			c := FinishCommand{Command: f.command(fixtureHandler, "finish-mixed-file-"+conflict), Items: []Item{f.item(0), f.item(1)}}
			c.Items[0].Evidence = fileEvidence("applied-file")
			c.Items[1].Evidence = fileEvidence("conflicted-file")
			if conflict == "link_revision" {
				c.Items[1].ExpectedLinkRevision++
			}
			if conflict == "risk_revision" {
				c.Items[1].ExpectedRiskRevision++
			}
			unchanged := f.riskLinkSnapshot(1)
			res, err := f.finish(c)
			require.NoError(t, err)
			require.Equal(t, "pending_confirmation", res.Items[0].Result)
			require.Equal(t, "conflict", res.Items[1].Result)
			require.Equal(t, unchanged, f.riskLinkSnapshot(1))
			require.Equal(t, []string{"applied-file"}, evidenceBindings(t, f), "conflict items must never acquire attachments")
			before := f.snapshot()
			replayed, err := f.finish(c)
			require.NoError(t, err)
			require.Equal(t, res, replayed)
			require.Equal(t, before, f.snapshot())
			require.Equal(t, []string{"applied-file"}, evidenceBindings(t, f), "receipt replay must not bind again")
		})
	}
}

func TestFinishAllConflictsHaveNoEvidenceOrDomainEffects(t *testing.T) {
	f := evidenceBindingFixture(t, 2)
	c := FinishCommand{Command: f.command(fixtureHandler, "finish-all-files-conflict"), Items: []Item{f.item(0), f.item(1)}}
	for i := range c.Items {
		c.Items[i].ExpectedLinkRevision++
		c.Items[i].Evidence = fileEvidence(fmt.Sprintf("stale-file-%d", i))
	}
	before := f.factsSnapshot()
	res, err := f.finish(c)
	require.NoError(t, err)
	for _, item := range res.Items {
		require.Equal(t, "conflict", item.Result)
	}
	require.Equal(t, before, f.factsSnapshot(), "all conflicts may persist only their operation receipt")
	require.Empty(t, evidenceBindings(t, f))
}

func TestReviewEvidenceHistoryIsReadOnlyAndStaleRejectBindsNothing(t *testing.T) {
	for _, conflict := range []string{"link_revision", "risk_revision", "proposal_version", "submission_baseline"} {
		t.Run(conflict, func(t *testing.T) {
			f := evidenceBindingFixture(t, 2)
			c := FinishCommand{Command: f.command(fixtureHandler, "file-proposals"), Items: []Item{f.item(0), f.item(1)}}
			c.Items[0].Evidence = fileEvidence("original-A")
			c.Items[1].Evidence = fileEvidence("original-B")
			_, err := f.finish(c)
			require.NoError(t, err)
			review := ReviewCommand{Command: f.command(fixtureOwner, "review-mixed-files-"+conflict), Items: []ReviewItem{f.reviewItem(0, "approve"), f.reviewItem(1, "reject")}}
			review.Items[0].Evidence = fileEvidence("review-A")
			review.Items[1].Evidence = fileEvidence("stale-reject-B")
			switch conflict {
			case "link_revision":
				review.Items[1].ExpectedLinkRevision++
			case "risk_revision":
				review.Items[1].ExpectedRiskRevision++
			case "proposal_version":
				review.Items[1].ExpectedProposalVersion++
			case "submission_baseline":
				_, err = f.pool.Exec(f.ctx, `UPDATE risk_remediation_submissions SET owner_at_submission=$2 WHERE link_id=$1`, f.links[1], fixtureOther)
				require.NoError(t, err)
			}
			unchanged := f.riskLinkSnapshot(1)
			res, err := f.review(review)
			require.NoError(t, err, "historical files are already bound and must only be revalidated")
			require.Equal(t, "applied", res.Items[0].Result)
			require.Equal(t, "conflict", res.Items[1].Result)
			require.Equal(t, unchanged, f.riskLinkSnapshot(1))
			require.Equal(t, []string{"original-A", "original-B", "review-A"}, evidenceBindings(t, f))
		})
	}
}

func TestReviewAllConflictsHaveNoNewEvidenceOrDomainEffects(t *testing.T) {
	f := evidenceBindingFixture(t, 1)
	c := FinishCommand{Command: f.command(fixtureHandler, "file-proposal"), Items: []Item{f.item(0)}}
	c.Items[0].Evidence = fileEvidence("original-file")
	_, err := f.finish(c)
	require.NoError(t, err)
	review := ReviewCommand{Command: f.command(fixtureOwner, "review-file-conflict"), Items: []ReviewItem{f.reviewItem(0, "reject")}}
	review.Items[0].ExpectedProposalVersion++
	review.Items[0].Evidence = fileEvidence("stale-reject-file")
	before := f.factsSnapshot()
	res, err := f.review(review)
	require.NoError(t, err)
	require.Equal(t, "conflict", res.Items[0].Result)
	require.Equal(t, before, f.factsSnapshot())
	require.Equal(t, []string{"original-file"}, evidenceBindings(t, f))
}

func TestHistoricalEvidenceDoesNotRequireCurrentReviewerToBeUploader(t *testing.T) {
	f := evidenceBindingFixture(t, 1)
	_, err := f.pool.Exec(f.ctx, `CREATE TABLE fixture_evidence_uploads(ref TEXT PRIMARY KEY,uploaded_by BIGINT NOT NULL)`)
	require.NoError(t, err)
	_, err = f.pool.Exec(f.ctx, `INSERT INTO fixture_evidence_uploads VALUES('handler-upload',$1)`, fixtureHandler)
	require.NoError(t, err)
	checkEvidence := f.svc.callbacks.Evidence
	f.svc.callbacks.Evidence = func(ctx context.Context, tx pgx.Tx, check EvidenceCheck) error {
		if !check.Historical {
			for _, ref := range check.Evidence {
				var uploader int64
				if err := tx.QueryRow(ctx, `SELECT uploaded_by FROM fixture_evidence_uploads WHERE ref=$1`, ref.Ref).Scan(&uploader); err != nil {
					return err
				}
				if uploader != check.ActorID {
					return errcode.ErrNoPermission
				}
			}
		}
		return checkEvidence(ctx, tx, check)
	}
	finish := FinishCommand{Command: f.command(fixtureHandler, "handler-file-proposal"), Items: []Item{f.item(0)}}
	finish.Items[0].Evidence = fileEvidence("handler-upload")
	_, err = f.finish(finish)
	require.NoError(t, err)
	review := ReviewCommand{Command: f.command(fixtureOwner, "owner-review-handler-file"), Items: []ReviewItem{f.reviewItem(0, "approve")}}
	review.Items[0].Evidence = nil
	res, err := f.review(review)
	require.NoError(t, err, "an authorized current owner may read the original handler's already bound evidence")
	require.Equal(t, "applied", res.Items[0].Result)
	require.Equal(t, []string{"handler-upload"}, evidenceBindings(t, f))
}

func TestRejectedBResubmissionBindsOnlyFreshEffectiveEvidence(t *testing.T) {
	f := evidenceBindingFixture(t, 2)
	finish := FinishCommand{Command: f.command(fixtureHandler, "files-original-AB"), Items: []Item{f.item(0), f.item(1)}}
	finish.Items[0].Evidence = fileEvidence("original-A")
	finish.Items[1].Evidence = fileEvidence("original-B")
	_, err := f.finish(finish)
	require.NoError(t, err)
	review := ReviewCommand{Command: f.command(fixtureOwner, "files-approve-A-reject-B"), Items: []ReviewItem{f.reviewItem(0, "approve"), f.reviewItem(1, "reject")}}
	review.Items[0].Evidence = fileEvidence("review-A")
	review.Items[1].Evidence = fileEvidence("rejection-B")
	res, err := f.review(review)
	require.NoError(t, err)
	require.Equal(t, "applied", res.Items[0].Result)
	require.Equal(t, "rejected", res.Items[1].Result)
	unchangedA := f.riskLinkSnapshot(0)
	stale := finish.Items[1]
	stale.Evidence = fileEvidence("stale-retry-B")
	res, err = f.finish(FinishCommand{Command: f.command(fixtureHandler, "files-stale-B-retry"), Items: []Item{stale}})
	require.NoError(t, err)
	require.Equal(t, "conflict", res.Items[0].Result)
	fresh := f.item(1)
	fresh.Evidence = fileEvidence("fresh-retry-B")
	res, err = f.finish(FinishCommand{Command: f.command(fixtureHandler, "files-fresh-B-retry"), Items: []Item{fresh}})
	require.NoError(t, err)
	require.Equal(t, "pending_confirmation", res.Items[0].Result)
	require.Equal(t, int64(2), res.Items[0].ProposalVersion)
	require.Equal(t, unchangedA, f.riskLinkSnapshot(0), "retrying B must preserve approved A")
	require.Equal(t, []string{"fresh-retry-B", "original-A", "original-B", "rejection-B", "review-A"}, evidenceBindings(t, f))
}

func TestReviewEvidenceFailureRollsBackPriorItemAndBindings(t *testing.T) {
	for _, failure := range []string{"historical", "binding", "risk_remediation_reviews", "fixture_ticket_intents", "risk_remediation_receipts"} {
		t.Run(failure, func(t *testing.T) {
			f := evidenceBindingFixture(t, 2)
			finish := FinishCommand{Command: f.command(fixtureHandler, "files-before-review-failure"), Items: []Item{f.item(0), f.item(1)}}
			finish.Items[0].Evidence = fileEvidence("original-A")
			finish.Items[1].Evidence = fileEvidence("original-B")
			_, err := f.finish(finish)
			require.NoError(t, err)
			review := ReviewCommand{Command: f.command(fixtureOwner, "files-failed-review-"+failure), Items: []ReviewItem{f.reviewItem(0, "approve"), f.reviewItem(1, "reject")}}
			review.Items[0].Evidence = fileEvidence("review-A")
			review.Items[1].Evidence = fileEvidence("review-B")
			if failure == "historical" || failure == "binding" {
				checkEvidence := f.svc.callbacks.Evidence
				f.svc.callbacks.Evidence = func(ctx context.Context, tx pgx.Tx, check EvidenceCheck) error {
					if err := checkEvidence(ctx, tx, check); err != nil {
						return err
					}
					if check.LinkID == f.links[1] && failure == "historical" && check.Historical {
						return errcode.ErrNoPermission
					}
					if check.LinkID == f.links[1] && failure == "binding" && check.Mode == EvidenceBind {
						return errcode.ErrFileAlreadyBound
					}
					return nil
				}
			} else {
				_, err = f.pool.Exec(f.ctx, `CREATE FUNCTION reject_review_evidence_fact() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic review transaction failure'; END $$`)
				require.NoError(t, err)
				_, err = f.pool.Exec(f.ctx, fmt.Sprintf(`CREATE TRIGGER reject_review_evidence_fact BEFORE INSERT ON %s FOR EACH ROW EXECUTE FUNCTION reject_review_evidence_fact()`, pgx.Identifier{failure}.Sanitize()))
				require.NoError(t, err)
			}
			before := f.snapshot()
			err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
				_, commandErr := f.svc.ReviewTx(ctx, tx, review)
				require.Error(t, commandErr)
				_, err := tx.Exec(ctx, `CREATE TABLE fixture_review_caller_committed(value TEXT); INSERT INTO fixture_review_caller_committed VALUES('kept')`)
				return err
			})
			require.NoError(t, err)
			require.Equal(t, before, f.snapshot())
			require.Equal(t, []string{"original-A", "original-B"}, evidenceBindings(t, f))
			var value string
			require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT value FROM fixture_review_caller_committed`).Scan(&value))
			require.Equal(t, "kept", value)
		})
	}
}

func TestEvidenceBindingAndRequiredWritesRollbackInsideSavepoint(t *testing.T) {
	for _, failure := range []string{"binding", "risk_events", "risk_remediation_intents", "fixture_ticket_events", "fixture_ticket_intents", "risk_remediation_receipts"} {
		t.Run(failure, func(t *testing.T) {
			f := evidenceBindingFixture(t, 2)
			c := FinishCommand{Command: f.command(fixtureHandler, "files-rollback-"+failure), Items: []Item{f.item(0), f.item(1)}}
			c.Items[0].Evidence = fileEvidence("rollback-A")
			c.Items[1].Evidence = fileEvidence("rollback-B")
			if failure == "binding" {
				bind := f.svc.callbacks.Evidence
				f.svc.callbacks.Evidence = func(ctx context.Context, tx pgx.Tx, check EvidenceCheck) error {
					if err := bind(ctx, tx, check); err != nil {
						return err
					}
					var bound bool
					if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fixture_evidence_bindings WHERE ref='rollback-B')`).Scan(&bound); err != nil {
						return err
					}
					if bound {
						return errcode.ErrFileAlreadyBound
					}
					return nil
				}
			} else {
				_, err := f.pool.Exec(f.ctx, `CREATE FUNCTION reject_evidence_fact() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic evidence transaction failure'; END $$`)
				require.NoError(t, err)
				_, err = f.pool.Exec(f.ctx, fmt.Sprintf(`CREATE TRIGGER reject_evidence_fact BEFORE INSERT ON %s FOR EACH ROW EXECUTE FUNCTION reject_evidence_fact()`, pgx.Identifier{failure}.Sanitize()))
				require.NoError(t, err)
			}
			before := f.snapshot()
			err := f.guarded(func(ctx context.Context, tx pgx.Tx) error {
				_, commandErr := f.svc.FinishTx(ctx, tx, c)
				require.Error(t, commandErr)
				_, err := tx.Exec(ctx, `CREATE TABLE fixture_evidence_caller_committed(value TEXT); INSERT INTO fixture_evidence_caller_committed VALUES('kept')`)
				return err
			})
			require.NoError(t, err)
			require.Equal(t, before, f.snapshot())
			require.Empty(t, evidenceBindings(t, f), "swallowing the error must not commit any attachment binding")
			var value string
			require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT value FROM fixture_evidence_caller_committed`).Scan(&value))
			require.Equal(t, "kept", value)
		})
	}
}

func TestEvidenceBindingsJoinCallerRollback(t *testing.T) {
	f := evidenceBindingFixture(t, 1)
	c := FinishCommand{Command: f.command(fixtureHandler, "files-outer-rollback"), Items: []Item{f.item(0)}}
	c.Items[0].Evidence = fileEvidence("outer-rollback-file")
	before := f.snapshot()
	rollback := errors.New("synthetic caller rollback")
	err := f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		_, err := f.svc.FinishTx(ctx, tx, c)
		if err != nil {
			return err
		}
		var count int
		require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM fixture_evidence_bindings`).Scan(&count))
		require.Equal(t, 1, count)
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	require.Equal(t, before, f.snapshot())
	require.Empty(t, evidenceBindings(t, f))
}
