//go:build integration

package riskremediation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// requestAcceptanceIndependence drives a legal owner request for risk index zero.
func requestAcceptanceIndependence(f *pgFixture, actor int64, operation string) AcceptanceRequest {
	i := f.item(0)
	return AcceptanceRequest{Command: f.command(actor, operation), RiskID: f.risks[0], ExpectedRiskRevision: i.ExpectedRiskRevision, LinkID: i.LinkID, ExpectedLinkRevision: i.ExpectedLinkRevision, Reason: "synthetic bounded acceptance", ValidUntil: time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)}
}

// approvedAcceptance drives finish → request → independent approval for risk
// index zero, then optionally expires the approval in place on the database
// clock. The kernel never receives a wall-clock override; tests shift only the
// persisted validity columns relative to the locked database clock.
func approvedAcceptance(t *testing.T, f *pgFixture, operation string, expire bool) Result {
	t.Helper()
	finished, err := f.finish(FinishCommand{Command: f.command(fixtureHandler, operation+"-finish"), Items: []Item{f.item(0)}})
	require.NoError(t, err)
	require.Equal(t, "pending_confirmation", finished.Items[0].Result)
	i := f.item(0)
	pending, err := f.requestAcceptance(AcceptanceRequest{Command: f.command(fixtureOwner, operation+"-request"), RiskID: f.risks[0], ExpectedRiskRevision: i.ExpectedRiskRevision, LinkID: i.LinkID, ExpectedLinkRevision: i.ExpectedLinkRevision, Reason: "synthetic bounded acceptance", ValidUntil: time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)})
	require.NoError(t, err)
	decision := AcceptanceDecision{Command: f.command(fixtureManager, operation+"-decision"), AcceptanceID: pending.AcceptanceID, ExpectedAcceptanceRevision: pending.AcceptanceRevision, ExpectedRiskRevision: pending.Items[0].RiskRevision, Decision: "approve", Reason: "synthetic independent approval"}
	approved, err := f.decideAcceptance(decision)
	require.NoError(t, err)
	require.Equal(t, "approved", approved.AcceptanceStatus)
	if expire {
		_, err = f.pool.Exec(f.ctx, `WITH previous AS (
			UPDATE risk_acceptances SET valid_until=statement_timestamp()-interval '1 second' WHERE id=$1
			RETURNING risk_id, valid_until
		) UPDATE risk_items SET accepted_until=previous.valid_until FROM previous WHERE id=previous.risk_id`, approved.AcceptanceID)
		require.NoError(t, err)
	}
	return approved
}

// reworkCommand builds a manager rework command from the current persisted
// revisions of the approved acceptance on risk index zero.
func reworkCommand(t *testing.T, f *pgFixture, actor int64, operation string, approved Result) AcceptanceRework {
	t.Helper()
	i := f.item(0)
	var acceptanceRevision int64
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT revision FROM risk_acceptances WHERE id=$1`, approved.AcceptanceID).Scan(&acceptanceRevision))
	return AcceptanceRework{
		Command:                    f.command(actor, operation),
		RiskID:                     f.risks[0],
		ExpectedRiskRevision:       i.ExpectedRiskRevision,
		LinkID:                     i.LinkID,
		ExpectedLinkRevision:       i.ExpectedLinkRevision,
		AcceptanceID:               approved.AcceptanceID,
		ExpectedAcceptanceRevision: acceptanceRevision,
		Reason:                     "synthetic expired acceptance rework",
	}
}

func TestAcceptanceRequestRejectsWithoutIndependentApprover(t *testing.T) {
	t.Run("creator only qualification", func(t *testing.T) {
		f := newPGFixture(t, 1)
		// The applicant becomes the only possible independent decider.
		_, err := f.pool.Exec(f.ctx, `UPDATE fixture_actors SET main_anchor=false WHERE id=$1`, fixtureManager)
		require.NoError(t, err)
		_, err = f.pool.Exec(f.ctx, `UPDATE fixture_actors SET main_anchor=true WHERE id=$1`, fixtureOwner)
		require.NoError(t, err)
		before := f.snapshot()
		_, err = f.requestAcceptance(requestAcceptanceIndependence(f, fixtureOwner, "creator-only-request"))
		require.ErrorIs(t, err, errcode.ErrConflict, "a creator-only acceptance request must not open a pending application")
		require.Equal(t, before, f.snapshot(), "the rejection must leave zero writes behind")
	})
	t.Run("independent anchor permits the request", func(t *testing.T) {
		f := newPGFixture(t, 1)
		pending, err := f.requestAcceptance(requestAcceptanceIndependence(f, fixtureOwner, "independent-request"))
		require.NoError(t, err)
		require.Equal(t, "pending", pending.AcceptanceStatus)
	})
}

func TestAcceptanceReworkReturnsExpiredApprovalToRework(t *testing.T) {
	f := newPGFixture(t, 3)
	_, err := f.finish(FinishCommand{Command: f.command(fixtureHandler, "sibling-finish"), Items: []Item{f.item(1)}})
	require.NoError(t, err)
	_, err = f.review(ReviewCommand{Command: f.command(fixtureOwner, "sibling-approve"), Items: []ReviewItem{f.reviewItem(1, "approve")}})
	require.NoError(t, err)
	approved := approvedAcceptance(t, f, "expire-rework", true)
	var acceptedUntil *time.Time
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT accepted_until FROM risk_items WHERE id=$1`, f.risks[0]).Scan(&acceptedUntil))
	require.NotNil(t, acceptedUntil)
	submissionsBefore := f.snapshot()
	siblingBefore := f.riskLinkSnapshot(1)

	rework := reworkCommand(t, f, fixtureManager, "expired-rework", approved)
	result, err := f.reworkAcceptance(rework)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "reworked", result.Items[0].Result)
	require.Equal(t, approved.AcceptanceID, result.AcceptanceID)
	require.Equal(t, "reworked", result.AcceptanceStatus)
	require.False(t, result.TerminalAllowed)

	var status, state, resolution string
	var acceptedAfter *time.Time
	var resolutionVersion int64
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT r.status,l.remediation_state,COALESCE(l.resolution,''),l.resolution_version,r.accepted_until FROM risk_items r JOIN risk_ticket_links l ON l.risk_id=r.id WHERE l.id=$1`, f.links[0]).Scan(&status, &state, &resolution, &resolutionVersion, &acceptedAfter))
	require.Equal(t, "remediating", status, "the risk must return to a submittable state")
	require.Equal(t, "open", state, "the link must return to open remediation")
	require.Empty(t, resolution)
	require.Nil(t, acceptedAfter, "the lapsed acceptance must stop being effective")
	require.EqualValues(t, 1, resolutionVersion, "the resolution history version must be preserved, not advanced")

	// The original approval, its evidence and the sibling conclusion survive.
	var approvalStatus, decidedBy string
	var approvalRevision int64
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT status,revision,decided_by FROM risk_acceptances WHERE id=$1`, approved.AcceptanceID).Scan(&approvalStatus, &approvalRevision, &decidedBy))
	require.Equal(t, "approved", approvalStatus, "the original approval record must be preserved")
	require.Equal(t, approved.AcceptanceRevision, approvalRevision)
	require.NotEmpty(t, decidedBy)
	sibling := f.riskLinkSnapshot(1)
	require.Contains(t, submissionsBefore, `"resolution_version":1`)
	var submissionCount, reviewCount, siblingLinks int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM risk_remediation_submissions WHERE link_id=$1),(SELECT count(*) FROM risk_remediation_reviews), (SELECT count(*) FROM risk_ticket_links WHERE id=$2 AND remediation_state='resolved')`, f.links[0], f.links[1]).Scan(&submissionCount, &reviewCount, &siblingLinks))
	require.Equal(t, 1, submissionCount, "original evidence submissions must be preserved")
	require.Equal(t, 1, reviewCount, "original review conclusions must be preserved")
	require.Equal(t, 1, siblingLinks, "sibling conclusions must be untouched")
	require.Equal(t, siblingBefore, sibling)
	var events int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM risk_events WHERE event_kind='acceptance_reworked' AND operation_id=$1`, rework.OperationID).Scan(&events))
	require.Equal(t, 1, events, "the rework must record one durable domain fact")

	// Replay by original operation id returns the same receipt without new writes.
	before := f.snapshot()
	replay, err := f.reworkAcceptance(rework)
	require.NoError(t, err)
	require.Equal(t, result, replay)
	require.Equal(t, before, f.snapshot(), "rework replay must be receipt-idempotent")

	// The returned item is submittable again by the current handler.
	i := f.item(0)
	resubmitted, err := f.finish(FinishCommand{Command: f.command(fixtureHandler, "rework-resubmit"), Items: []Item{{LinkID: i.LinkID, ExpectedLinkRevision: i.ExpectedLinkRevision, ExpectedRiskRevision: i.ExpectedRiskRevision, Resolution: "fixed", Measures: "synthetic rework resubmission", Evidence: i.Evidence}}})
	require.NoError(t, err, "a reworked item must be submittable again")
	require.Equal(t, "pending_confirmation", resubmitted.Items[0].Result)
}

func TestAcceptanceReworkRejectsUnexpiredApprovalWithoutWrites(t *testing.T) {
	f := newPGFixture(t, 2)
	approved := approvedAcceptance(t, f, "unexpired-rework", false)
	rework := reworkCommand(t, f, fixtureManager, "unexpired-rework-command", approved)
	before := f.snapshot()
	_, err := f.reworkAcceptance(rework)
	require.ErrorIs(t, err, errcode.ErrConflict, "a still-valid approval must not be reworkable")
	require.Equal(t, before, f.snapshot(), "the rejection must leave zero writes behind")
}

func TestAcceptanceReworkRequiresCurrentManagerQualification(t *testing.T) {
	f := newPGFixture(t, 2)
	approved := approvedAcceptance(t, f, "unqualified-rework", true)
	rework := reworkCommand(t, f, fixtureOther, "unqualified-rework-command", approved)
	before := f.snapshot()
	_, err := f.reworkAcceptance(rework)
	require.ErrorIs(t, err, errcode.ErrNoPermission, "only an independently qualified manager may rework")
	require.Equal(t, before, f.snapshot())
}

func TestAcceptanceReworkRejectsStaleBaselinesWithoutWrites(t *testing.T) {
	f := newPGFixture(t, 2)
	approved := approvedAcceptance(t, f, "stale-rework", true)
	for name, mutate := range map[string]func(c *AcceptanceRework){
		"risk_revision":       func(c *AcceptanceRework) { c.ExpectedRiskRevision-- },
		"link_revision":       func(c *AcceptanceRework) { c.ExpectedLinkRevision-- },
		"link_identity":       func(c *AcceptanceRework) { c.LinkID++ },
		"ticket_revision":     func(c *AcceptanceRework) { c.ExpectedTicketRevision-- },
		"acceptance_revision": func(c *AcceptanceRework) { c.ExpectedAcceptanceRevision-- },
	} {
		t.Run(name, func(t *testing.T) {
			c := reworkCommand(t, f, fixtureManager, "stale-rework-"+name, approved)
			mutate(&c)
			before := f.snapshot()
			_, err := f.reworkAcceptance(c)
			require.ErrorIs(t, err, errcode.ErrConflict)
			require.Equal(t, before, f.snapshot())
		})
	}
}

func TestAcceptanceReworkNeverReopensClosedTickets(t *testing.T) {
	f := newPGFixture(t, 1)
	approved := approvedAcceptance(t, f, "closed-rework", true)
	// Mirror the owning workflow's closed dispatch scope: the group and its
	// links are historical facts after the ticket really closed.
	_, err := f.pool.Exec(f.ctx, `
		UPDATE risk_dispatch_groups SET ended_at=clock_timestamp() WHERE ticket_id=$1;
		UPDATE risk_ticket_links l SET ended_at=g.ended_at, end_reason='ticket_closed', revision=l.revision+1
		FROM risk_dispatch_groups g WHERE l.group_id=g.id AND g.ticket_id=$1 AND l.ended_at IS NULL;
	`, f.ticket)
	require.NoError(t, err)
	rework := reworkCommand(t, f, fixtureManager, "closed-rework-command", approved)
	before := f.snapshot()
	_, err = f.reworkAcceptance(rework)
	require.ErrorIs(t, err, errcode.ErrConflict, "a closed dispatch scope must not be reopened by rework")
	require.Equal(t, before, f.snapshot())
	var ended bool
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT ended_at IS NOT NULL FROM risk_dispatch_groups WHERE ticket_id=$1`, f.ticket).Scan(&ended))
	require.True(t, ended)
}
