//go:build integration

package riskremediation

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// This fails when the required original kernel migration is absent, rather
// than silently skipping database coverage or using a hand-copied kernel DDL.
func TestRemediationOriginalMigrationLoads(t *testing.T) {
	f := newPGFixture(t, 1)
	var name string
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT 'risk_remediation_intents'::regclass::text`).Scan(&name))
	require.Equal(t, "risk_remediation_intents", name)
}

func TestRemediationCardTwoPartialReviewResubmitsOnlyRejectedAsset(t *testing.T) {
	f := newPGFixture(t, 2)
	_, err := f.finish(FinishCommand{Command: f.command(fixtureHandler, "finish-AB"), Items: []Item{f.item(0), f.item(1)}})
	require.NoError(t, err)
	result, err := f.review(ReviewCommand{Command: f.command(fixtureOwner, "review-AB"), Items: []ReviewItem{f.reviewItem(0, "approve"), f.reviewItem(1, "reject")}})
	require.NoError(t, err)
	require.False(t, result.TerminalAllowed)
	var aState, bState, aResolution, riskBStatus string
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT remediation_state,resolution FROM risk_ticket_links WHERE id=$1`, f.links[0]).Scan(&aState, &aResolution))
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT l.remediation_state,r.status FROM risk_ticket_links l JOIN risk_items r ON r.id=l.risk_id WHERE l.id=$1`, f.links[1]).Scan(&bState, &riskBStatus))
	require.Equal(t, "resolved", aState)
	require.Equal(t, "fixed", aResolution)
	require.Equal(t, "rejected", bState)
	require.Equal(t, "remediating", riskBStatus)
	a := f.riskLinkSnapshot(0)
	_, err = f.finish(FinishCommand{Command: f.command(fixtureHandler, "resubmit-B"), Items: []Item{f.item(1)}})
	require.NoError(t, err)
	require.Equal(t, a, f.riskLinkSnapshot(0), "resubmitting B must retain A's effective conclusion")
	result, err = f.review(ReviewCommand{Command: f.command(fixtureOwner, "approve-B"), Items: []ReviewItem{f.reviewItem(1, "approve")}})
	require.NoError(t, err)
	require.True(t, result.TerminalAllowed)
	var unchangedConclusion, closedLink bool
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT to_jsonb(r) = $2::jsonb->'risk'
		AND (to_jsonb(l)-'ended_at'-'end_reason'-'revision') = (($2::jsonb->'link')-'ended_at'-'end_reason'-'revision'),
		l.ended_at IS NOT NULL AND l.end_reason='ticket_closed' AND l.revision=($2::jsonb->'link'->>'revision')::bigint+1
		FROM risk_items r JOIN risk_ticket_links l ON l.risk_id=r.id WHERE l.id=$1`, f.links[0], a).Scan(&unchangedConclusion, &closedLink))
	require.True(t, unchangedConclusion, "closing the group preserves A's effective conclusion")
	require.True(t, closedLink, "closing the group ends A's live link and advances its revision")
	var ticketStatus string
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT status FROM tickets WHERE id=$1`, f.ticket).Scan(&ticketStatus))
	require.Equal(t, "closed", ticketStatus)
	var aSubmissions, bSubmissions, reviews int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FILTER(WHERE link_id=$1),count(*) FILTER(WHERE link_id=$2) FROM risk_remediation_submissions`, f.links[0], f.links[1]).Scan(&aSubmissions, &bSubmissions))
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM risk_remediation_reviews`).Scan(&reviews))
	require.Equal(t, 1, aSubmissions)
	require.Equal(t, 2, bSubmissions)
	require.Equal(t, 3, reviews)
	err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		return f.svc.ValidateTerminalTx(ctx, tx, fixtureOwner, f.ticket, f.command(fixtureOwner, "unused").ExpectedTicketRevision)
	})
	require.NoError(t, err)
}

func TestRemediationReviewRejectsLostOwnerAndLateProposal(t *testing.T) {
	f := newPGFixture(t, 1)
	_, err := f.finish(FinishCommand{Command: f.command(fixtureHandler, "finish"), Items: []Item{f.item(0)}})
	require.NoError(t, err)
	late := f.reviewItem(0, "approve")
	_, err = f.pool.Exec(f.ctx, `UPDATE risk_items SET owner_user_id=$2,revision=revision+1 WHERE id=$1`, f.risks[0], fixtureOther)
	require.NoError(t, err)
	before := f.snapshot()
	_, err = f.review(ReviewCommand{Command: f.command(fixtureOwner, "lost-owner"), Items: []ReviewItem{f.reviewItem(0, "approve")}})
	require.True(t, errors.Is(err, errcode.ErrNoPermission) || errors.Is(err, errcode.ErrForbidden), "lost current owner must be denied: %v", err)
	require.Equal(t, before, f.snapshot())
	facts := f.factsSnapshot()
	result, err := f.review(ReviewCommand{Command: f.command(fixtureOther, "late-review"), Items: []ReviewItem{late}})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "conflict", result.Items[0].Result)
	require.Equal(t, facts, f.factsSnapshot(), "a per-item conflict may save its receipt but no business fact")
}

func TestRemediationReviewRetainsValidItemWhenAnotherProposalIsStale(t *testing.T) {
	f := newPGFixture(t, 2)
	_, err := f.finish(FinishCommand{Command: f.command(fixtureHandler, "finish-AB"), Items: []Item{f.item(0), f.item(1)}})
	require.NoError(t, err)
	_, err = f.review(ReviewCommand{Command: f.command(fixtureOwner, "reject-B"), Items: []ReviewItem{f.reviewItem(1, "reject")}})
	require.NoError(t, err)
	oldProposal := f.reviewItem(1, "approve").ExpectedProposalVersion
	_, err = f.finish(FinishCommand{Command: f.command(fixtureHandler, "resubmit-B"), Items: []Item{f.item(1)}})
	require.NoError(t, err)
	staleB := f.reviewItem(1, "approve")
	staleB.ExpectedProposalVersion = oldProposal
	b := f.riskLinkSnapshot(1)
	result, err := f.review(ReviewCommand{Command: f.command(fixtureOwner, "mixed-review"), Items: []ReviewItem{f.reviewItem(0, "approve"), staleB}})
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	require.Equal(t, "applied", result.Items[0].Result)
	require.Equal(t, "conflict", result.Items[1].Result)
	require.False(t, result.TerminalAllowed)
	require.Equal(t, b, f.riskLinkSnapshot(1), "the resubmitted proposal must not be approved by a late decision")
	var reviews int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM risk_remediation_reviews`).Scan(&reviews))
	require.Equal(t, 2, reviews, "only the earlier B rejection and current A approval exist")
}

func TestRemediationAcceptanceRequiresOwnerThenIndependentAnchor(t *testing.T) {
	f := newPGFixture(t, 1)
	i := f.item(0)
	request := AcceptanceRequest{Command: f.command(fixtureOwner, "accept-request"), RiskID: f.risks[0], ExpectedRiskRevision: i.ExpectedRiskRevision, LinkID: i.LinkID, ExpectedLinkRevision: i.ExpectedLinkRevision, Reason: "synthetic justified acceptance", ValidUntil: time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond)}
	denied := request
	denied.Command = f.command(fixtureHandler, "handler-cannot-apply")
	before := f.snapshot()
	_, err := f.requestAcceptance(denied)
	require.True(t, errors.Is(err, errcode.ErrNoPermission) || errors.Is(err, errcode.ErrForbidden), "only current owner applies: %v", err)
	require.Equal(t, before, f.snapshot())
	pending, err := f.requestAcceptance(request)
	require.NoError(t, err)
	require.Positive(t, pending.AcceptanceID)
	require.Equal(t, "pending", pending.AcceptanceStatus)
	err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		return f.svc.ValidateTerminalTx(ctx, tx, fixtureOwner, f.ticket, f.command(fixtureOwner, "unused").ExpectedTicketRevision)
	})
	require.ErrorIs(t, err, errcode.ErrConflict)
	// Even an otherwise authorized main-anchor manager cannot self-confirm.
	_, err = f.pool.Exec(f.ctx, `UPDATE fixture_actors SET main_anchor=true WHERE id=$1`, fixtureOwner)
	require.NoError(t, err)
	var riskRevision int64
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT revision FROM risk_items WHERE id=$1`, f.risks[0]).Scan(&riskRevision))
	decision := AcceptanceDecision{Command: f.command(fixtureOwner, "self-confirm"), AcceptanceID: pending.AcceptanceID, ExpectedAcceptanceRevision: pending.AcceptanceRevision, ExpectedRiskRevision: riskRevision, Decision: "approve", Reason: "synthetic approval"}
	before = f.snapshot()
	_, err = f.decideAcceptance(decision)
	require.True(t, errors.Is(err, errcode.ErrNoPermission) || errors.Is(err, errcode.ErrForbidden), "self-confirmation must be denied: %v", err)
	require.Equal(t, before, f.snapshot())
	decision.Command = f.command(fixtureOther, "non-anchor-confirm")
	_, err = f.decideAcceptance(decision)
	require.True(t, errors.Is(err, errcode.ErrNoPermission) || errors.Is(err, errcode.ErrForbidden), "non-anchor must be denied: %v", err)
	require.Equal(t, before, f.snapshot())
	decision.Command = f.command(fixtureManager, "anchor-confirm")
	approved, err := f.decideAcceptance(decision)
	require.NoError(t, err)
	require.Equal(t, "approved", approved.AcceptanceStatus)
	var status, resolution string
	var until time.Time
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT r.status,r.accepted_until,l.resolution FROM risk_items r JOIN risk_ticket_links l ON l.risk_id=r.id WHERE l.id=$1`, f.links[0]).Scan(&status, &until, &resolution))
	require.Equal(t, "accepted", status)
	require.Equal(t, "accepted", resolution)
	require.True(t, request.ValidUntil.Equal(until), "accepted_until must retain the requested instant")
}

func TestRemediationAcceptanceRejectsAlreadyExpiredRequest(t *testing.T) {
	f := newPGFixture(t, 1)
	i := f.item(0)
	before := f.snapshot()
	_, err := f.requestAcceptance(AcceptanceRequest{Command: f.command(fixtureOwner, "expired"), RiskID: f.risks[0], ExpectedRiskRevision: i.ExpectedRiskRevision, LinkID: i.LinkID, ExpectedLinkRevision: i.ExpectedLinkRevision, Reason: "synthetic", ValidUntil: time.Now().Add(-time.Minute)})
	require.ErrorIs(t, err, errcode.ErrInvalidParams)
	require.Equal(t, before, f.snapshot())
}

func TestRemediationPendingAcceptanceCannotBeApprovedAfterValidityEnds(t *testing.T) {
	f := newPGFixture(t, 1)
	i := f.item(0)
	pending, err := f.requestAcceptance(AcceptanceRequest{Command: f.command(fixtureOwner, "pending-to-expire"), RiskID: f.risks[0], ExpectedRiskRevision: i.ExpectedRiskRevision, LinkID: i.LinkID, ExpectedLinkRevision: i.ExpectedLinkRevision, Reason: "synthetic bounded acceptance", ValidUntil: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	// Age only this synthetic row instead of sleeping or changing a global clock.
	_, err = f.pool.Exec(f.ctx, `UPDATE risk_acceptances SET valid_until=clock_timestamp()-interval '1 second' WHERE id=$1`, pending.AcceptanceID)
	require.NoError(t, err)
	var revision int64
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT revision FROM risk_items WHERE id=$1`, f.risks[0]).Scan(&revision))
	before := f.snapshot()
	_, err = f.decideAcceptance(AcceptanceDecision{Command: f.command(fixtureManager, "late-acceptance"), AcceptanceID: pending.AcceptanceID, ExpectedAcceptanceRevision: pending.AcceptanceRevision, ExpectedRiskRevision: revision, Decision: "approve", Reason: "synthetic late decision"})
	require.ErrorIs(t, err, errcode.ErrConflict)
	require.Equal(t, before, f.snapshot())
}

func TestRemediationAcceptSuggestionCannotSubstituteOwnerApplication(t *testing.T) {
	f := newPGFixture(t, 1)
	i := f.item(0)
	i.Resolution = "accept"
	_, err := f.finish(FinishCommand{Command: f.command(fixtureHandler, "accept-suggestion"), Items: []Item{i}})
	require.NoError(t, err)
	var accepted, resolved, applications int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM risk_items WHERE status='accepted'`).Scan(&accepted))
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM risk_ticket_links WHERE remediation_state='resolved'`).Scan(&resolved))
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM risk_acceptances`).Scan(&applications))
	require.Zero(t, accepted)
	require.Zero(t, resolved)
	require.Zero(t, applications, "handler suggestion must not impersonate an owner application")
	facts := f.factsSnapshot()
	review, err := f.review(ReviewCommand{Command: f.command(fixtureOwner, "cannot-review-accept-suggestion"), Items: []ReviewItem{f.reviewItem(0, "approve")}})
	require.NoError(t, err)
	require.Len(t, review.Items, 1)
	require.Equal(t, "conflict", review.Items[0].Result)
	require.Equal(t, facts, f.factsSnapshot())
	err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		return f.svc.ValidateTerminalTx(ctx, tx, fixtureOwner, f.ticket, f.command(fixtureOwner, "unused").ExpectedTicketRevision)
	})
	require.ErrorIs(t, err, errcode.ErrConflict)
}

func TestRemediationTerminalRechecksEffectiveOwnerAndAcceptanceDeadline(t *testing.T) {
	t.Run("owner changed after fixed conclusion", func(t *testing.T) {
		f := newPGFixture(t, 1)
		_, err := f.finish(FinishCommand{Command: f.command(fixtureHandler, "finish"), Items: []Item{f.item(0)}})
		require.NoError(t, err)
		result, err := f.review(ReviewCommand{Command: f.command(fixtureOwner, "approve"), Items: []ReviewItem{f.reviewItem(0, "approve")}})
		require.NoError(t, err)
		require.True(t, result.TerminalAllowed)
		_, err = f.pool.Exec(f.ctx, `UPDATE risk_items SET owner_user_id=$2,revision=revision+1 WHERE id=$1`, f.risks[0], fixtureOther)
		require.NoError(t, err)
		before := f.snapshot()
		err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
			return f.svc.ValidateTerminalTx(ctx, tx, fixtureManager, f.ticket, f.command(fixtureManager, "unused").ExpectedTicketRevision)
		})
		require.ErrorIs(t, err, errcode.ErrConflict)
		require.Equal(t, before, f.snapshot())
	})
	t.Run("approved acceptance expired", func(t *testing.T) {
		f := newPGFixture(t, 1)
		i := f.item(0)
		pending, err := f.requestAcceptance(AcceptanceRequest{Command: f.command(fixtureOwner, "request"), RiskID: f.risks[0], ExpectedRiskRevision: i.ExpectedRiskRevision, LinkID: i.LinkID, ExpectedLinkRevision: i.ExpectedLinkRevision, Reason: "synthetic bounded acceptance", ValidUntil: time.Now().Add(time.Hour)})
		require.NoError(t, err)
		var revision int64
		require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT revision FROM risk_items WHERE id=$1`, f.risks[0]).Scan(&revision))
		_, err = f.decideAcceptance(AcceptanceDecision{Command: f.command(fixtureManager, "approve"), AcceptanceID: pending.AcceptanceID, ExpectedAcceptanceRevision: pending.AcceptanceRevision, ExpectedRiskRevision: revision, Decision: "approve", Reason: "synthetic decision"})
		require.NoError(t, err)
		// Age only synthetic facts; no global clock, scheduler or expiry worker.
		_, err = f.pool.Exec(f.ctx, `UPDATE risk_items SET accepted_until=clock_timestamp()-interval '1 second' WHERE id=$1`, f.risks[0])
		require.NoError(t, err)
		before := f.snapshot()
		err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
			return f.svc.ValidateTerminalTx(ctx, tx, fixtureManager, f.ticket, f.command(fixtureManager, "unused").ExpectedTicketRevision)
		})
		require.ErrorIs(t, err, errcode.ErrConflict)
		require.Equal(t, before, f.snapshot())
	})
}

func TestRemediationStandaloneAcceptanceHasNoFabricatedTicket(t *testing.T) {
	f := newPGFixture(t, 1)
	_, err := f.pool.Exec(f.ctx, `UPDATE risk_ticket_links SET ended_at=clock_timestamp(),end_reason='synthetic_detach' WHERE id=$1`, f.links[0])
	require.NoError(t, err)
	request := AcceptanceRequest{Command: Command{ActorID: fixtureOwner, OperationID: "standalone-request"}, RiskID: f.risks[0], ExpectedRiskRevision: 1, Reason: "synthetic standalone acceptance", ValidUntil: time.Now().Add(time.Hour)}
	pending, err := f.requestAcceptance(request)
	require.NoError(t, err)
	require.Zero(t, pending.TicketID)
	var revision int64
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT revision FROM risk_items WHERE id=$1`, f.risks[0]).Scan(&revision))
	approved, err := f.decideAcceptance(AcceptanceDecision{Command: Command{ActorID: fixtureManager, OperationID: "standalone-approve"}, AcceptanceID: pending.AcceptanceID, ExpectedAcceptanceRevision: pending.AcceptanceRevision, ExpectedRiskRevision: revision, Decision: "approve", Reason: "synthetic anchor decision"})
	require.NoError(t, err)
	require.Zero(t, approved.TicketID)
	require.Equal(t, "approved", approved.AcceptanceStatus)
	var ticketEvents, ticketIntents int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM fixture_ticket_events`).Scan(&ticketEvents))
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM fixture_ticket_intents`).Scan(&ticketIntents))
	require.Zero(t, ticketEvents)
	require.Zero(t, ticketIntents)
}

func TestRemediationAcceptanceReplayAfterLegalDetach(t *testing.T) {
	t.Run("pending owner request", func(t *testing.T) {
		f := newPGFixture(t, 1)
		i := f.item(0)
		request := AcceptanceRequest{Command: f.command(fixtureOwner, "request-before-detach"), RiskID: f.risks[0], ExpectedRiskRevision: i.ExpectedRiskRevision, LinkID: i.LinkID, ExpectedLinkRevision: i.ExpectedLinkRevision, Reason: "synthetic bounded acceptance", ValidUntil: time.Now().Add(time.Hour)}
		original, err := f.requestAcceptance(request)
		require.NoError(t, err)
		_, err = f.pool.Exec(f.ctx, `UPDATE risk_ticket_links SET revision=revision+1,ended_at=clock_timestamp(),end_reason='synthetic_legal_detach' WHERE id=$1`, f.links[0])
		require.NoError(t, err)
		before := f.snapshot()
		replayed, err := f.requestAcceptance(request)
		require.NoError(t, err, "an authorized replay must not require an already-detached live scope")
		require.Equal(t, original, replayed)
		require.Equal(t, before, f.snapshot())
	})
	t.Run("approved anchor decision", func(t *testing.T) {
		f := newPGFixture(t, 1)
		i := f.item(0)
		pending, err := f.requestAcceptance(AcceptanceRequest{Command: f.command(fixtureOwner, "request"), RiskID: f.risks[0], ExpectedRiskRevision: i.ExpectedRiskRevision, LinkID: i.LinkID, ExpectedLinkRevision: i.ExpectedLinkRevision, Reason: "synthetic bounded acceptance", ValidUntil: time.Now().Add(time.Hour)})
		require.NoError(t, err)
		var revision int64
		require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT revision FROM risk_items WHERE id=$1`, f.risks[0]).Scan(&revision))
		decision := AcceptanceDecision{Command: f.command(fixtureManager, "approve-before-detach"), AcceptanceID: pending.AcceptanceID, ExpectedAcceptanceRevision: pending.AcceptanceRevision, ExpectedRiskRevision: revision, Decision: "approve", Reason: "synthetic decision"}
		original, err := f.decideAcceptance(decision)
		require.NoError(t, err)
		_, err = f.pool.Exec(f.ctx, `UPDATE risk_ticket_links SET revision=revision+1,ended_at=clock_timestamp(),end_reason='synthetic_legal_detach' WHERE id=$1`, f.links[0])
		require.NoError(t, err)
		before := f.snapshot()
		replayed, err := f.decideAcceptance(decision)
		require.NoError(t, err, "the original authorized decision receipt survives legal detach")
		require.Equal(t, original, replayed)
		require.Equal(t, before, f.snapshot())
	})
}

func TestRemediationTerminalAllowsAllLinksLegallyDetached(t *testing.T) {
	t.Run("all historical links legally detached", func(t *testing.T) {
		f := newPGFixture(t, 2)
		_, err := f.pool.Exec(f.ctx, `UPDATE risk_ticket_links SET revision=revision+1,ended_at=clock_timestamp(),end_reason='synthetic_legal_detach' WHERE id=ANY($1)`, f.links)
		require.NoError(t, err)
		before := f.snapshot()
		command := f.command(fixtureManager, "unused")
		err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
			return f.svc.ValidateTerminalTx(ctx, tx, fixtureManager, f.ticket, command.ExpectedTicketRevision)
		})
		require.NoError(t, err, "every former association has an explicit legal detach fact")
		require.Equal(t, before, f.snapshot())
	})
	t.Run("empty group without association history", func(t *testing.T) {
		f := newPGFixture(t, 0)
		before := f.snapshot()
		command := f.command(fixtureManager, "unused")
		err := f.guarded(func(ctx context.Context, tx pgx.Tx) error {
			return f.svc.ValidateTerminalTx(ctx, tx, fixtureManager, f.ticket, command.ExpectedTicketRevision)
		})
		require.ErrorIs(t, err, errcode.ErrConflict)
		require.Equal(t, before, f.snapshot())
	})
}

func TestRemediationCallerRollbackRemovesAllFacts(t *testing.T) {
	f := newPGFixture(t, 1)
	c := FinishCommand{Command: f.command(fixtureHandler, "outer-rollback"), Items: []Item{f.item(0)}}
	before := f.snapshot()
	outerFailure := errors.New("synthetic outer command failure")
	err := f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		_, err := f.svc.FinishTx(ctx, tx, c)
		if err != nil {
			return err
		}
		var receipts int
		require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM risk_remediation_receipts`).Scan(&receipts))
		require.Equal(t, 1, receipts, "receipt is visible before outer rollback")
		return outerFailure
	})
	require.ErrorIs(t, err, outerFailure)
	require.Equal(t, before, f.snapshot(), "risk/link/ticket/history/receipts/intents must all roll back")
}

func TestRemediationOperationReplayAndCurrentReceiptAuthorization(t *testing.T) {
	f := newPGFixture(t, 1)
	c := FinishCommand{Command: f.command(fixtureHandler, "durable-K"), Items: []Item{f.item(0)}}
	first, err := f.finish(c)
	require.NoError(t, err)
	before := f.snapshot()
	replayed, err := f.finish(c)
	require.NoError(t, err)
	require.Equal(t, first, replayed)
	require.Equal(t, before, f.snapshot())
	c.Items[0].Measures = "different parameters"
	_, err = f.finish(c)
	require.ErrorIs(t, err, errcode.ErrConflict)
	require.Equal(t, before, f.snapshot())
	var receipt Result
	err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		var err error
		receipt, err = f.svc.ReceiptTx(ctx, tx, fixtureHandler, "durable-K")
		return err
	})
	require.NoError(t, err)
	require.Equal(t, first, receipt)
	_, err = f.pool.Exec(f.ctx, `UPDATE fixture_actors SET enabled=false WHERE id=$1`, fixtureHandler)
	require.NoError(t, err)
	err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		_, err := f.svc.ReceiptTx(ctx, tx, fixtureHandler, "durable-K")
		return err
	})
	require.True(t, errors.Is(err, errcode.ErrNoPermission) || errors.Is(err, errcode.ErrForbidden), "receipt reads need current authorization: %v", err)
}

func TestRemediationReplayChecksOriginalRiskAfterLinkDetach(t *testing.T) {
	f := newPGFixture(t, 2)
	c := FinishCommand{Command: f.command(fixtureHandler, "original-risk-receipt"), Items: []Item{f.item(0), f.item(1)}}
	_, err := f.finish(c)
	require.NoError(t, err)
	_, err = f.pool.Exec(f.ctx, `UPDATE risk_ticket_links SET ended_at=clock_timestamp(),end_reason='synthetic_detach' WHERE id=$1`, f.links[0])
	require.NoError(t, err)
	callbacks := f.callbacks()
	authorize := callbacks.Authorize
	callbacks.Authorize = func(ctx context.Context, tx pgx.Tx, a Access) error {
		if a.RiskID == f.risks[0] {
			return errcode.ErrNoPermission
		}
		return authorize(ctx, tx, a)
	}
	f.svc = New(f.pool, callbacks)
	before := f.snapshot()
	_, err = f.finish(c)
	require.True(t, errors.Is(err, errcode.ErrNoPermission) || errors.Is(err, errcode.ErrForbidden), "replay may not expose a detached original risk: %v", err)
	require.Equal(t, before, f.snapshot())
	err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		_, err := f.svc.ReceiptTx(ctx, tx, fixtureHandler, c.OperationID)
		return err
	})
	require.True(t, errors.Is(err, errcode.ErrNoPermission) || errors.Is(err, errcode.ErrForbidden), "receipt query must recheck original risk: %v", err)
	require.Equal(t, before, f.snapshot())
}

func TestRemediationHistoryOrIntentFailureRollsBackCommand(t *testing.T) {
	for _, table := range []string{"risk_events", "risk_remediation_intents", "fixture_ticket_events", "fixture_ticket_intents"} {
		t.Run(table, func(t *testing.T) {
			f := newPGFixture(t, 1)
			_, err := f.pool.Exec(f.ctx, `CREATE FUNCTION reject_required_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic required write failure'; END $$`)
			require.NoError(t, err)
			_, err = f.pool.Exec(f.ctx, fmt.Sprintf(`CREATE TRIGGER reject_required_write BEFORE INSERT ON %s FOR EACH ROW EXECUTE FUNCTION reject_required_write()`, pgx.Identifier{table}.Sanitize()))
			require.NoError(t, err)
			before := f.snapshot()
			_, err = f.finish(FinishCommand{Command: f.command(fixtureHandler, "must-rollback"), Items: []Item{f.item(0)}})
			require.Error(t, err)
			require.Equal(t, before, f.snapshot())
		})
	}
}

func TestRemediationFailedSavepointCannotLeakWhenCallerContinues(t *testing.T) {
	f := newPGFixture(t, 2)
	c := FinishCommand{Command: f.command(fixtureHandler, "failed-savepoint"), Items: []Item{f.item(0), f.item(1)}}
	callbacks := f.callbacks()
	apply := callbacks.ApplyTicket
	callbacks.ApplyTicket = func(ctx context.Context, tx pgx.Tx, effect TicketEffect) (int64, error) {
		if _, err := apply(ctx, tx, effect); err != nil {
			return 0, err
		}
		return 0, errors.New("synthetic failure after ticket history and intent")
	}
	svc := New(f.pool, callbacks)
	before := f.snapshot()
	err := f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		_, err := svc.FinishTx(ctx, tx, c)
		require.Error(t, err)
		// The caller deliberately handles the error and commits other work.
		_, err = tx.Exec(ctx, `CREATE TABLE caller_continued(value TEXT); INSERT INTO caller_continued VALUES('committed')`)
		return err
	})
	require.NoError(t, err)
	require.Equal(t, before, f.snapshot())
	var value string
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT value FROM caller_continued`).Scan(&value))
	require.Equal(t, "committed", value)
}

func TestRemediationConcurrentSameOperationHasOneDurableResult(t *testing.T) {
	f := newPGFixture(t, 1)
	c := FinishCommand{Command: f.command(fixtureHandler, "concurrent-same-K"), Items: []Item{f.item(0)}}
	type outcome struct {
		result Result
		err    error
	}
	start := make(chan struct{})
	done := make(chan outcome, 2)
	for range 2 {
		go func() {
			<-start
			r, err := f.finish(c)
			done <- outcome{r, err}
		}()
	}
	close(start)
	a, b := <-done, <-done
	require.NoError(t, a.err)
	require.NoError(t, b.err)
	require.Equal(t, a.result, b.result)
	var submissions, receipts int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM risk_remediation_submissions`).Scan(&submissions))
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM risk_remediation_receipts`).Scan(&receipts))
	require.Equal(t, 1, submissions)
	require.Equal(t, 1, receipts)
}

func TestRemediationConcurrentDifferentOperationsRejectStaleBaseline(t *testing.T) {
	f := newPGFixture(t, 1)
	c := FinishCommand{Command: f.command(fixtureHandler, "concurrent-K-0"), Items: []Item{f.item(0)}}
	start := make(chan struct{})
	done := make(chan error, 2)
	for n := range 2 {
		copy := c
		copy.OperationID = fmt.Sprintf("concurrent-K-%d", n)
		go func(command FinishCommand) {
			<-start
			_, err := f.finish(command)
			done <- err
		}(copy)
	}
	close(start)
	a, b := <-done, <-done
	if a == nil {
		require.ErrorIs(t, b, errcode.ErrConflict)
	} else {
		require.ErrorIs(t, a, errcode.ErrConflict)
		require.NoError(t, b)
	}
	var submissions int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM risk_remediation_submissions`).Scan(&submissions))
	require.Equal(t, 1, submissions)
}

// pg_blocking_pids is the proof of contention; elapsed time alone would also
// pass for a paused goroutine that never reached a database lock.
func TestRemediationLocksRiskUntilCallerTransactionCompletes(t *testing.T) {
	f := newPGFixture(t, 1)
	c := FinishCommand{Command: f.command(fixtureHandler, "row-lock-proof"), Items: []Item{f.item(0)}}
	holder, err := f.pool.Begin(f.ctx)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback(context.Background()) }()
	var holderPID int32
	require.NoError(t, holder.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&holderPID))
	_, err = holder.Exec(f.ctx, `SELECT id FROM risk_items WHERE id=$1 FOR UPDATE`, f.risks[0])
	require.NoError(t, err)
	entered := make(chan int32, 1)
	callbacks := f.callbacks()
	prelock := callbacks.Prelock
	callbacks.Prelock = func(ctx context.Context, tx pgx.Tx, a Access) error {
		if err := prelock(ctx, tx, a); err != nil {
			return err
		}
		var pid int32
		if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			return err
		}
		select {
		case entered <- pid:
		default:
		}
		return nil
	}
	f.svc = New(f.pool, callbacks)
	done := make(chan error, 1)
	go func() { _, err := f.finish(c); done <- err }()
	var waiterPID int32
	select {
	case waiterPID = <-entered:
	case <-f.ctx.Done():
		t.Fatal("command never entered authorization adapter")
	}
	require.Eventually(t, func() bool {
		var blocked bool
		err := f.pool.QueryRow(f.ctx, `SELECT $1::int=ANY(pg_blocking_pids($2))`, holderPID, waiterPID).Scan(&blocked)
		return err == nil && blocked
	}, 3*time.Second, 10*time.Millisecond, "kernel must wait on the current risk row")
	select {
	case err := <-done:
		t.Fatalf("command completed while risk was locked: %v", err)
	default:
	}
	require.NoError(t, holder.Commit(f.ctx))
	require.NoError(t, <-done)
}

type advisoryAttemptTrace struct{ pid chan int32 }

func (trace *advisoryAttemptTrace) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if data.SQL == `SELECT pg_advisory_lock($1)` {
		select {
		case trace.pid <- int32(conn.PgConn().PID()):
		default:
		}
	}
	return ctx
}
func (*advisoryAttemptTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestRemediationC04AdmittedCommandExcludesConcurrentRevocation(t *testing.T) {
	f := newPGFixture(t, 1)
	c := FinishCommand{Command: f.command(fixtureHandler, "admitted-command"), Items: []Item{f.item(0)}}
	entered := make(chan int32, 1)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	callbacks := f.callbacks()
	authorize := callbacks.Authorize
	callbacks.Authorize = func(ctx context.Context, tx pgx.Tx, a Access) error {
		if err := authorize(ctx, tx, a); err != nil {
			return err
		}
		var pid int32
		if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			return err
		}
		select {
		case entered <- pid:
		default:
		}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.svc = New(f.pool, callbacks)
	commandDone := make(chan error, 1)
	go func() { _, err := f.finish(c); commandDone <- err }()
	var admittedPID int32
	select {
	case admittedPID = <-entered:
	case <-f.ctx.Done():
		t.Fatal("command never reached locked authorization")
	}
	// A second pool bypasses the process-local gate; the observed exclusion
	// must therefore come from C04's PostgreSQL advisory lock.
	trace := &advisoryAttemptTrace{pid: make(chan int32, 1)}
	cfg := f.pool.Config()
	cfg.ConnConfig.Tracer = trace
	writerPool, err := pgxpool.NewWithConfig(f.ctx, cfg)
	require.NoError(t, err)
	defer func() {
		unblock()
		writerPool.Close()
	}()
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- repository.WithIAMGuard(f.ctx, writerPool, func(ctx context.Context) error {
			_, err := repository.IAMDB(ctx, writerPool).Exec(ctx, `UPDATE fixture_actors SET enabled=false WHERE id=$1`, fixtureHandler)
			return err
		})
	}()
	var writerPID int32
	select {
	case writerPID = <-trace.pid:
	case <-f.ctx.Done():
		t.Fatal("revocation never attempted PostgreSQL guard")
	}
	require.Eventually(t, func() bool {
		var blocked bool
		err := f.pool.QueryRow(f.ctx, `SELECT $1::int=ANY(pg_blocking_pids($2))`, admittedPID, writerPID).Scan(&blocked)
		return err == nil && blocked
	}, 3*time.Second, 10*time.Millisecond)
	unblock()
	require.NoError(t, <-commandDone)
	require.NoError(t, <-writerDone)
	callbacks = f.callbacks()
	f.svc = New(f.pool, callbacks)
	before := f.snapshot()
	_, err = f.finish(FinishCommand{Command: f.command(fixtureHandler, "after-revocation"), Items: []Item{f.item(0)}})
	require.True(t, errors.Is(err, errcode.ErrNoPermission) || errors.Is(err, errcode.ErrForbidden), "next command must observe revocation: %v", err)
	require.Equal(t, before, f.snapshot())
}

func TestRemediationRequiresExactCallerGuardAndAllAdapters(t *testing.T) {
	f := newPGFixture(t, 1)
	c := FinishCommand{Command: f.command(fixtureHandler, "unguarded"), Items: []Item{f.item(0)}}
	before := f.snapshot()
	tx, err := f.pool.Begin(f.ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = f.svc.FinishTx(f.ctx, tx, c)
	require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
	err = f.guarded(func(ctx context.Context, _ pgx.Tx) error {
		_, err := f.svc.FinishTx(ctx, tx, c)
		return err
	})
	require.ErrorIs(t, err, errcode.ErrServiceUnavailable, "a guard does not authorize a different transaction")
	for _, missing := range []string{"admit", "prelock", "authorize", "policy", "evidence", "apply"} {
		callbacks := f.callbacks()
		switch missing {
		case "admit":
			callbacks.Admit = nil
		case "prelock":
			callbacks.Prelock = nil
		case "authorize":
			callbacks.Authorize = nil
		case "policy":
			callbacks.TicketPolicy = nil
		case "evidence":
			callbacks.Evidence = nil
		case "apply":
			callbacks.ApplyTicket = nil
		}
		svc := New(f.pool, callbacks)
		err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
			_, err := svc.FinishTx(ctx, tx, c)
			return err
		})
		require.ErrorIs(t, err, errcode.ErrServiceUnavailable, "missing %s must fail closed", missing)
	}
	require.Equal(t, before, f.snapshot())
}

// Removing the kernel write must fail this assertion even with valid adapters.
func TestRemediationFinishStoresPendingProposal(t *testing.T) {
	f := newPGFixture(t, 1)
	cmd := FinishCommand{Command: f.command(fixtureHandler, "finish-first"), Items: []Item{f.item(0)}}
	var result Result
	err := f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		var err error
		result, err = f.svc.FinishTx(ctx, tx, cmd)
		return err
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	var riskStatus, linkState string
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT r.status,l.remediation_state FROM risk_items r JOIN risk_ticket_links l ON l.risk_id=r.id WHERE l.id=$1`, f.links[0]).Scan(&riskStatus, &linkState))
	require.Equal(t, "pending_verify", riskStatus)
	require.Equal(t, "pending_verify", linkState)
	var submissions, intents int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM risk_remediation_submissions`).Scan(&submissions))
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM risk_remediation_intents`).Scan(&intents))
	require.Equal(t, 1, submissions)
	require.Positive(t, intents)
	require.False(t, result.TerminalAllowed)
}
