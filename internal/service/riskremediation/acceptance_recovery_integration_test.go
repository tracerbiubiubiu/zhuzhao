//go:build integration

package riskremediation

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func recoveryRequest(f *pgFixture, actor int64, operation string) AcceptanceRequest {
	i := f.item(0)
	return AcceptanceRequest{Command: f.command(actor, operation), RiskID: f.risks[0], ExpectedRiskRevision: i.ExpectedRiskRevision, LinkID: i.LinkID, ExpectedLinkRevision: i.ExpectedLinkRevision, Reason: "synthetic bounded acceptance", ValidUntil: time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)}
}

func expiredRecoveryRequest(t *testing.T, f *pgFixture) Result {
	t.Helper()
	pending, err := f.requestAcceptance(recoveryRequest(f, fixtureOwner, "original-pending"))
	require.NoError(t, err)
	_, err = f.pool.Exec(f.ctx, `UPDATE risk_acceptances SET valid_until=clock_timestamp()-interval '1 second' WHERE id=$1`, pending.AcceptanceID)
	require.NoError(t, err)
	return pending
}

func assertRecoveredAcceptance(t *testing.T, f *pgFixture, id int64, wantStatus, operation string) {
	t.Helper()
	var status, reason string
	var revision int64
	var decidedBy *int64
	var decidedAt *time.Time
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT status,revision,reason,decided_by,decided_at FROM risk_acceptances WHERE id=$1`, id).Scan(&status, &revision, &reason, &decidedBy, &decidedAt))
	require.Equal(t, wantStatus, status)
	require.EqualValues(t, 2, revision)
	require.Equal(t, "synthetic bounded acceptance", reason, "recovery preserves the original application")
	require.Nil(t, decidedBy, "invalidation is not an independent anchor decision")
	require.Nil(t, decidedAt)
	var events, intents int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM risk_events WHERE event_kind=$1 AND operation_id=$2 AND payload->>'acceptance_id'=$3`, "acceptance_"+wantStatus, operation, strconv.FormatInt(id, 10)).Scan(&events))
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM risk_remediation_intents WHERE event_kind=$1 AND operation_id=$2 AND payload->>'acceptance_id'=$3`, "acceptance_"+wantStatus, operation, strconv.FormatInt(id, 10)).Scan(&intents))
	require.Equal(t, 1, events, "recovery must retain one domain history fact")
	require.Equal(t, 1, intents, "recovery history and intent commit together")
}

func TestAcceptanceRecoveryExpiredPendingAllowsNewRequest(t *testing.T) {
	f := newPGFixture(t, 1)
	old := expiredRecoveryRequest(t, f)
	request := recoveryRequest(f, fixtureOwner, "request-after-expiry")
	current, err := f.requestAcceptance(request)
	require.NoError(t, err, "expired pending applications must release the pending slot during a legal new request")
	require.NotEqual(t, old.AcceptanceID, current.AcceptanceID)
	require.Equal(t, "pending", current.AcceptanceStatus)
	assertRecoveredAcceptance(t, f, old.AcceptanceID, "expired", request.OperationID)
	var pending int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM risk_acceptances WHERE status='pending'`).Scan(&pending))
	require.Equal(t, 1, pending)
	before := f.snapshot()
	replay, err := f.requestAcceptance(request)
	require.NoError(t, err)
	require.Equal(t, current, replay)
	require.Equal(t, before, f.snapshot(), "same-K recovery must not invalidate or create twice")
}

func TestAcceptanceRecoveryExpiredPendingAllowsNewFinish(t *testing.T) {
	f := newPGFixture(t, 1)
	old := expiredRecoveryRequest(t, f)
	command := FinishCommand{Command: f.command(fixtureHandler, "finish-after-expiry"), Items: []Item{f.item(0)}}
	result, err := f.finish(command)
	require.NoError(t, err)
	require.Equal(t, "pending_confirmation", result.Items[0].Result, "expired acceptance must permit a legal remediation submission")
	assertRecoveredAcceptance(t, f, old.AcceptanceID, "expired", command.OperationID)
	var status, state string
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT r.status,l.remediation_state FROM risk_items r JOIN risk_ticket_links l ON l.risk_id=r.id WHERE l.id=$1`, f.links[0]).Scan(&status, &state))
	require.Equal(t, "pending_verify", status)
	require.Equal(t, "pending_verify", state)
}

func TestAcceptanceRecoveryStaleOwnerOrRevisionRequiresNewOperation(t *testing.T) {
	for _, change := range []string{"owner", "risk_revision"} {
		t.Run(change, func(t *testing.T) {
			f := newPGFixture(t, 1)
			old, err := f.requestAcceptance(recoveryRequest(f, fixtureOwner, "old-application"))
			require.NoError(t, err)
			actor := fixtureOwner
			if change == "owner" {
				actor = fixtureOther
				_, err = f.pool.Exec(f.ctx, `UPDATE risk_items SET owner_user_id=$2,revision=revision+1 WHERE id=$1`, f.risks[0], actor)
			} else {
				_, err = f.pool.Exec(f.ctx, `UPDATE risk_items SET revision=revision+1 WHERE id=$1`, f.risks[0])
			}
			require.NoError(t, err)
			oldDecision := AcceptanceDecision{Command: f.command(fixtureManager, "stale-approval"), AcceptanceID: old.AcceptanceID, ExpectedAcceptanceRevision: old.AcceptanceRevision, ExpectedRiskRevision: f.item(0).ExpectedRiskRevision, Decision: "approve", Reason: "synthetic old-result decision"}
			before := f.snapshot()
			_, err = f.decideAcceptance(oldDecision)
			require.ErrorIs(t, err, errcode.ErrConflict)
			require.Equal(t, before, f.snapshot(), "a stale decision itself must have no recovery side effects")
			request := recoveryRequest(f, actor, "current-owner-new-application")
			current, err := f.requestAcceptance(request)
			require.NoError(t, err, "a current owner with current baselines must have an explicit recovery path")
			assertRecoveredAcceptance(t, f, old.AcceptanceID, "superseded", request.OperationID)
			decision := AcceptanceDecision{Command: f.command(fixtureManager, "current-approval"), AcceptanceID: current.AcceptanceID, ExpectedAcceptanceRevision: current.AcceptanceRevision, ExpectedRiskRevision: f.item(0).ExpectedRiskRevision, Decision: "approve", Reason: "synthetic current-result decision"}
			approved, err := f.decideAcceptance(decision)
			require.NoError(t, err)
			require.Equal(t, "approved", approved.AcceptanceStatus)
			var validDecision bool
			require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT decided_at<valid_until FROM risk_acceptances WHERE id=$1`, current.AcceptanceID).Scan(&validDecision))
			require.True(t, validDecision)
		})
	}
}

func TestAcceptanceRecoveryStaleCommandDoesNotInvalidateExpiredPending(t *testing.T) {
	f := newPGFixture(t, 1)
	expiredRecoveryRequest(t, f)
	request := recoveryRequest(f, fixtureOwner, "stale-recovery-request")
	request.ExpectedRiskRevision--
	before := f.snapshot()
	_, err := f.requestAcceptance(request)
	require.ErrorIs(t, err, errcode.ErrConflict)
	require.Equal(t, before, f.snapshot())
	command := FinishCommand{Command: f.command(fixtureHandler, "stale-recovery-finish"), Items: []Item{f.item(0)}}
	command.Items[0].ExpectedRiskRevision--
	result, err := f.finish(command)
	require.NoError(t, err)
	require.Equal(t, "conflict", result.Items[0].Result)
	var status string
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT status FROM risk_acceptances LIMIT 1`).Scan(&status))
	require.Equal(t, "pending", status, "a stale item cannot cause recovery")
}

func TestAcceptanceDecisionRechecksValidityAfterTicketPolicy(t *testing.T) {
	f := newPGFixture(t, 1)
	pending, err := f.requestAcceptance(recoveryRequest(f, fixtureOwner, "bounded-application"))
	require.NoError(t, err)
	_, err = f.pool.Exec(f.ctx, `UPDATE risk_acceptances SET valid_until=clock_timestamp()+interval '500 milliseconds' WHERE id=$1`, pending.AcceptanceID)
	require.NoError(t, err)
	callbacks := f.callbacks()
	policy := callbacks.TicketPolicy
	entered := false
	callbacks.TicketPolicy = func(ctx context.Context, tx pgx.Tx, c TicketCheck) (Policy, error) {
		entered = true
		_, err := tx.Exec(ctx, `SELECT pg_sleep(GREATEST(EXTRACT(EPOCH FROM valid_until-clock_timestamp()),0)+0.01) FROM risk_acceptances WHERE id=$1`, pending.AcceptanceID)
		if err != nil {
			return Policy{}, err
		}
		return policy(ctx, tx, c)
	}
	f.svc = New(f.pool, callbacks)
	before := f.snapshot()
	_, err = f.decideAcceptance(AcceptanceDecision{Command: f.command(fixtureManager, "decision-after-policy-delay"), AcceptanceID: pending.AcceptanceID, ExpectedAcceptanceRevision: pending.AcceptanceRevision, ExpectedRiskRevision: f.item(0).ExpectedRiskRevision, Decision: "approve", Reason: "synthetic delayed approval"})
	require.True(t, entered, "the request must still be valid when the ticket policy is entered")
	require.ErrorIs(t, err, errcode.ErrConflict, "approval must use the effective decision instant after policy work")
	require.Equal(t, before, f.snapshot(), "late approval must roll back all business effects")
}

func TestAcceptanceDecisionRechecksValidityAfterRiskWriteDelay(t *testing.T) {
	f := newPGFixture(t, 1)
	pending, err := f.requestAcceptance(recoveryRequest(f, fixtureOwner, "bounded-application"))
	require.NoError(t, err)
	_, err = f.pool.Exec(f.ctx, `
	CREATE FUNCTION delay_accepted_risk() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
	  PERFORM pg_sleep(GREATEST(EXTRACT(EPOCH FROM valid_until-clock_timestamp()),0)+0.01)
	  FROM risk_acceptances WHERE risk_id=NEW.id AND status='pending';
	  RETURN NEW;
	END $$;
	CREATE TRIGGER delay_accepted_risk BEFORE UPDATE ON risk_items
	FOR EACH ROW WHEN (NEW.status='accepted') EXECUTE FUNCTION delay_accepted_risk();`)
	require.NoError(t, err)
	_, err = f.pool.Exec(f.ctx, `UPDATE risk_acceptances SET valid_until=clock_timestamp()+interval '500 milliseconds' WHERE id=$1`, pending.AcceptanceID)
	require.NoError(t, err)
	before := f.snapshot()
	_, err = f.decideAcceptance(AcceptanceDecision{Command: f.command(fixtureManager, "decision-after-write-delay"), AcceptanceID: pending.AcceptanceID, ExpectedAcceptanceRevision: pending.AcceptanceRevision, ExpectedRiskRevision: f.item(0).ExpectedRiskRevision, Decision: "approve", Reason: "synthetic delayed approval"})
	require.ErrorIs(t, err, errcode.ErrConflict, "the final guarded decision write must reject a deadline crossed during business writes")
	require.Equal(t, before, f.snapshot())
}

func TestAcceptanceDecisionAdmitsBeforeAcceptanceLookup(t *testing.T) {
	f := newPGFixture(t, 1)
	_, err := f.pool.Exec(f.ctx, `UPDATE fixture_actors SET enabled=false WHERE id=$1`, fixtureManager)
	require.NoError(t, err)
	before := f.snapshot()
	_, err = f.decideAcceptance(AcceptanceDecision{Command: f.command(fixtureManager, "disabled-nonexistent-decision"), AcceptanceID: 987654321, ExpectedAcceptanceRevision: 1, ExpectedRiskRevision: 1, Decision: "approve", Reason: "synthetic denied lookup"})
	require.ErrorIs(t, err, errcode.ErrNoPermission, "credential admission must run before object existence lookup")
	require.Equal(t, before, f.snapshot())
}

func pendingRecoveryFailureFixture(t *testing.T, status string) (*pgFixture, Result) {
	t.Helper()
	f := newPGFixture(t, 1)
	if status == "expired" {
		return f, expiredRecoveryRequest(t, f)
	}
	pending, err := f.requestAcceptance(recoveryRequest(f, fixtureOwner, "original-pending"))
	require.NoError(t, err)
	_, err = f.pool.Exec(f.ctx, `UPDATE risk_items SET revision=revision+1 WHERE id=$1`, f.risks[0])
	require.NoError(t, err)
	return f, pending
}

func TestAcceptanceRecoveryRequestInsertFailureRollsBackAfterOuterCommit(t *testing.T) {
	for _, status := range []string{"expired", "superseded"} {
		t.Run(status, func(t *testing.T) {
			f, _ := pendingRecoveryFailureFixture(t, status)
			// Fail only after the old application and both recovery facts are
			// visible in this transaction. Missing recovery cannot pass by hitting
			// the same injected error before the behavior under test has happened.
			_, err := f.pool.Exec(f.ctx, `
			CREATE SEQUENCE fixture_recovery_observed;
			CREATE FUNCTION reject_replacement_acceptance() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN
			  IF EXISTS (
			    SELECT 1 FROM risk_acceptances a
			    WHERE a.risk_id=NEW.risk_id AND a.status IN ('expired','superseded') AND a.revision=2
			      AND EXISTS (SELECT 1 FROM risk_events e WHERE e.event_kind='acceptance_'||a.status
			        AND e.operation_id='replacement-request-fails' AND e.payload->>'acceptance_id'=a.id::text)
			      AND EXISTS (SELECT 1 FROM risk_remediation_intents i WHERE i.event_kind='acceptance_'||a.status
			        AND i.operation_id='replacement-request-fails' AND i.payload->>'acceptance_id'=a.id::text)
			  ) THEN
			    PERFORM nextval('fixture_recovery_observed');
			    RAISE EXCEPTION 'synthetic replacement insert rejected after recovery';
			  END IF;
			  RETURN NEW;
			END $$;
			CREATE TRIGGER reject_replacement_acceptance BEFORE INSERT ON risk_acceptances
			FOR EACH ROW EXECUTE FUNCTION reject_replacement_acceptance();`)
			require.NoError(t, err)
			request := recoveryRequest(f, fixtureOwner, "replacement-request-fails")
			before := f.snapshot()
			var commandErr error
			err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
				_, commandErr = f.svc.RequestAcceptanceTx(ctx, tx, request)
				// Deliberately swallow the command failure and commit an unrelated
				// outer write. The kernel's savepoint must protect every old fact.
				_, err := tx.Exec(ctx, `CREATE TABLE fixture_recovery_outer_commit AS SELECT 1 AS marker`)
				return err
			})
			require.NoError(t, err)
			require.ErrorIs(t, commandErr, errcode.ErrServiceUnavailable)
			var marker int
			require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT marker FROM fixture_recovery_outer_commit`).Scan(&marker))
			require.Equal(t, 1, marker, "the outer transaction really committed after swallowing the command error")
			// Sequence changes survive savepoint rollback and are excluded from
			// business snapshots. This proves the injected failure was reached
			// after complete recovery, rather than an earlier storage failure.
			var observedRecovery bool
			require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT is_called FROM fixture_recovery_observed`).Scan(&observedRecovery))
			require.True(t, observedRecovery)
			require.Equal(t, before, f.snapshot(), "old pending state, recovery history/intent and replacement effects must all roll back")
		})
	}
}

func TestAcceptanceRecoveryEvidenceBindFailureRollsBackAfterOuterCommit(t *testing.T) {
	for _, status := range []string{"expired", "superseded"} {
		t.Run(status, func(t *testing.T) {
			f, old := pendingRecoveryFailureFixture(t, status)
			callbacks := f.callbacks()
			evidence := callbacks.Evidence
			observedRecovery := false
			callbacks.Evidence = func(ctx context.Context, tx pgx.Tx, c EvidenceCheck) error {
				if c.Mode != EvidenceBind {
					return evidence(ctx, tx, c)
				}
				if err := tx.QueryRow(ctx, `SELECT EXISTS (
				  SELECT 1 FROM risk_acceptances a WHERE a.id=$1 AND a.status=$2 AND a.revision=2
				    AND EXISTS (SELECT 1 FROM risk_events e WHERE e.event_kind='acceptance_'||a.status
				      AND e.operation_id='replacement-bind-fails' AND e.payload->>'acceptance_id'=a.id::text)
				    AND EXISTS (SELECT 1 FROM risk_remediation_intents i WHERE i.event_kind='acceptance_'||a.status
				      AND i.operation_id='replacement-bind-fails' AND i.payload->>'acceptance_id'=a.id::text)
				)`, old.AcceptanceID, status).Scan(&observedRecovery); err != nil {
					return err
				}
				return errcode.ErrFileAlreadyBound
			}
			f.svc = New(f.pool, callbacks)
			command := FinishCommand{Command: f.command(fixtureHandler, "replacement-bind-fails"), Items: []Item{f.item(0)}}
			before := f.snapshot()
			var commandErr error
			err := f.guarded(func(ctx context.Context, tx pgx.Tx) error {
				_, commandErr = f.svc.FinishTx(ctx, tx, command)
				_, err := tx.Exec(ctx, `CREATE TABLE fixture_recovery_outer_commit AS SELECT 1 AS marker`)
				return err
			})
			require.NoError(t, err)
			require.ErrorIs(t, commandErr, errcode.ErrFileAlreadyBound)
			require.True(t, observedRecovery, "binding failure must occur after invalidation and both recovery facts")
			var marker int
			require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT marker FROM fixture_recovery_outer_commit`).Scan(&marker))
			require.Equal(t, 1, marker, "the outer transaction really committed after swallowing the command error")
			require.Equal(t, before, f.snapshot(), "old pending state and recovery history/intent must roll back with the failed submission")
		})
	}
}
