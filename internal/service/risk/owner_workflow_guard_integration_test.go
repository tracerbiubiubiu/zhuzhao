//go:build integration

package risk_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
)

// This fixture keeps the real risk mutation and remediation kernel on the same
// disposable database. Its synthetic callbacks isolate owner/revision semantics;
// published ticket authorization and attachment validation have their own suites.
type ownerWorkflowFixture struct {
	t                *testing.T
	riskID, ticketID int64
	linkID           int64
	suffix           string
	svc              *riskremediation.Service
}

func newOwnerWorkflowFixture(t *testing.T) *ownerWorkflowFixture {
	t.Helper()
	seedRiskFixture(t)
	f := &ownerWorkflowFixture{t: t, suffix: uniqueSuffix()}
	ctx := context.Background()
	created := mustCreate(t, newService(), "owner-create-"+f.suffix, "Synthetic owner workflow", orgGID, &userU)
	f.riskID = created.Risk.ID
	_, err := testPool.Exec(ctx, `UPDATE risk_items SET status='remediating' WHERE id=$1`, f.riskID)
	require.NoError(t, err)
	code := "owner_guard_" + f.suffix
	_, err = testPool.Exec(ctx, `INSERT INTO ticket_types(code,name) VALUES($1,'Synthetic owner guard')`, code)
	require.NoError(t, err)
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO tickets(type_code,title,status,created_by,assigned_to,org_id,org_path)
		VALUES($1,'Synthetic owner guard','in_progress',$2,$3,$4,'org91001') RETURNING id`, code, userU, userM, orgGID).Scan(&f.ticketID))
	var groupID int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_dispatch_groups(finding_key,owner_user_id,management_org_id,target_org_id,type_code,
		definition_version,approved_scope,group_key,ticket_id,ticket_ref_snapshot,operator_user_id)
		VALUES($1,$2,$3,$3,$4,1,'synthetic',repeat('a',64),$5,'Synthetic owner ticket',$6) RETURNING id`,
		"SYNTHETIC-"+f.suffix, userU, orgGID, code, f.ticketID, userM).Scan(&groupID))
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_ticket_links(risk_id,group_id,owner_at_link) VALUES($1,$2,$3) RETURNING id`, f.riskID, groupID, userU).Scan(&f.linkID))
	f.svc = riskremediation.New(testPool, riskremediation.Callbacks{
		Admit:   func(context.Context, pgx.Tx, riskremediation.Access) error { return nil },
		Prelock: func(context.Context, pgx.Tx, riskremediation.Access) error { return nil },
		Authorize: func(_ context.Context, _ pgx.Tx, a riskremediation.Access) error {
			if a.Action == "review" && a.ActorID != a.OwnerID {
				return errcode.ErrNoPermission
			}
			return nil
		},
		TicketPolicy: func(context.Context, pgx.Tx, riskremediation.TicketCheck) (riskremediation.Policy, error) {
			return riskremediation.Policy{ReviewGate: true}, nil
		},
		Evidence: func(context.Context, pgx.Tx, riskremediation.EvidenceCheck) error { return nil },
		ApplyTicket: func(ctx context.Context, tx pgx.Tx, e riskremediation.TicketEffect) (int64, error) {
			status := "pending_verify"
			if e.TerminalAllowed {
				status = "closed"
			}
			for _, item := range e.Items {
				if item.Result == "rejected" {
					status = "in_progress"
				}
			}
			var revision int64
			err := tx.QueryRow(ctx, `UPDATE tickets SET status=$2,revision=revision+1 WHERE id=$1 AND revision=$3 RETURNING revision`,
				e.TicketID, status, e.ExpectedRevision).Scan(&revision)
			return revision, err
		},
	})
	return f
}

func (f *ownerWorkflowFixture) command(actor int64, operation string) riskremediation.Command {
	f.t.Helper()
	var revision int64
	require.NoError(f.t, testPool.QueryRow(context.Background(), `SELECT revision FROM tickets WHERE id=$1`, f.ticketID).Scan(&revision))
	return riskremediation.Command{ActorID: actor, OperationID: operation + f.suffix, TicketID: f.ticketID, ExpectedTicketRevision: revision}
}

func (f *ownerWorkflowFixture) finish() riskremediation.Result {
	f.t.Helper()
	var linkRevision, riskRevision int64
	require.NoError(f.t, testPool.QueryRow(context.Background(), `SELECT l.revision,r.revision FROM risk_ticket_links l JOIN risk_items r ON r.id=l.risk_id WHERE l.id=$1`, f.linkID).Scan(&linkRevision, &riskRevision))
	c := riskremediation.FinishCommand{Command: f.command(userM, fmt.Sprintf("owner-finish-%d-", riskRevision)), Items: []riskremediation.Item{{
		LinkID: f.linkID, ExpectedLinkRevision: linkRevision, ExpectedRiskRevision: riskRevision,
		Resolution: "fixed", Measures: "Synthetic patch", Evidence: []riskremediation.EvidenceRef{{Kind: "reference", Ref: "synthetic-evidence", Version: "1"}},
	}}}
	var out riskremediation.Result
	require.NoError(f.t, repository.WithIAMCommandGuard(context.Background(), testPool, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = f.svc.FinishTx(ctx, tx, c)
		return err
	}))
	require.Equal(f.t, "pending_confirmation", out.Items[0].Result)
	return out
}

func (f *ownerWorkflowFixture) review(actor int64, decision string) riskremediation.Result {
	f.t.Helper()
	var linkRevision, riskRevision, proposal int64
	require.NoError(f.t, testPool.QueryRow(context.Background(), `SELECT l.revision,r.revision,l.proposal_version FROM risk_ticket_links l JOIN risk_items r ON r.id=l.risk_id WHERE l.id=$1`, f.linkID).Scan(&linkRevision, &riskRevision, &proposal))
	c := riskremediation.ReviewCommand{Command: f.command(actor, fmt.Sprintf("owner-review-%d-", proposal)), Items: []riskremediation.ReviewItem{{
		LinkID: f.linkID, ExpectedLinkRevision: linkRevision, ExpectedRiskRevision: riskRevision,
		ExpectedProposalVersion: proposal, Decision: decision, Note: "Synthetic retest decision",
	}}}
	var out riskremediation.Result
	require.NoError(f.t, repository.WithIAMCommandGuard(context.Background(), testPool, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = f.svc.ReviewTx(ctx, tx, c)
		return err
	}))
	return out
}

func (f *ownerWorkflowFixture) assign(owner int64) (*model.RiskMutationResult, error) {
	f.t.Helper()
	var revision int64
	require.NoError(f.t, testPool.QueryRow(context.Background(), `SELECT revision FROM risk_items WHERE id=$1`, f.riskID).Scan(&revision))
	return newService().Assign(context.Background(), model.RiskAssignRequest{
		OperationID: "owner-assign-" + f.suffix, ID: f.riskID, ExpectedRevision: strconv.FormatInt(revision, 10),
		OwnerUserID: owner, Reason: "Synthetic owner handover",
	}, userM)
}

func (f *ownerWorkflowFixture) snapshot() string {
	f.t.Helper()
	var snapshot string
	require.NoError(f.t, testPool.QueryRow(context.Background(), `SELECT jsonb_build_object(
		'risk',(SELECT to_jsonb(r) FROM risk_items r WHERE id=$1),
		'link',(SELECT to_jsonb(l) FROM risk_ticket_links l WHERE id=$2),
		'ticket',(SELECT to_jsonb(t) FROM tickets t WHERE id=$3),
		'submissions',(SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM risk_remediation_submissions s WHERE link_id=$2),
		'reviews',(SELECT jsonb_agg(to_jsonb(v) ORDER BY v.id) FROM risk_remediation_reviews v JOIN risk_remediation_submissions s ON s.id=v.submission_id WHERE s.link_id=$2),
		'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM risk_events e WHERE risk_id=$1),
		'operation',(SELECT to_jsonb(o) FROM risk_operations o WHERE actor_id=$4 AND operation_id=$5)
		)::text`, f.riskID, f.linkID, f.ticketID, userM, "owner-assign-"+f.suffix).Scan(&snapshot))
	return snapshot
}

func TestRiskOwnerAssignmentPreservesPendingGateOnReview(t *testing.T) {
	for _, owner := range []int64{userV, userU} {
		t.Run(fmt.Sprintf("target_%d", owner), func(t *testing.T) {
			f := newOwnerWorkflowFixture(t)
			f.finish()
			before := f.snapshot()
			out, err := f.assign(owner)
			require.ErrorIs(t, err, risk.ErrRiskInvalidState)
			require.Nil(t, out)
			require.Equal(t, before, f.snapshot(), "rejected assignment must not change a proposal baseline or leave a receipt")
			result := f.review(userU, "approve")
			require.Equal(t, "applied", result.Items[0].Result, "the original owner can still finish the pending retest")
			require.True(t, result.TerminalAllowed)
		})
	}
}

func TestRiskOwnerAssignmentPreservesExistingGateOnProposalAfterRejection(t *testing.T) {
	f := newOwnerWorkflowFixture(t)
	f.finish()
	rejected := f.review(userU, "reject")
	require.Equal(t, "rejected", rejected.Items[0].Result)
	before := f.snapshot()
	assigned, err := f.assign(userV)
	require.ErrorIs(t, err, risk.ErrRiskInvalidState)
	require.Nil(t, assigned)
	require.Equal(t, before, f.snapshot())
	f.finish()
	approved := f.review(userU, "approve")
	require.Equal(t, "applied", approved.Items[0].Result, "rejection permits rework and retest under the original responsibility")
	require.True(t, approved.TerminalAllowed)
}

func TestRiskOwnerAssignmentPreservesPendingPhaseWithoutProposal(t *testing.T) {
	f := newOwnerWorkflowFixture(t)
	_, err := testPool.Exec(context.Background(), `UPDATE risk_ticket_links SET remediation_state='pending_verify' WHERE id=$1`, f.linkID)
	require.NoError(t, err)
	before := f.snapshot()
	out, err := f.assign(userV)
	require.ErrorIs(t, err, risk.ErrRiskInvalidState)
	require.Nil(t, out)
	require.Equal(t, before, f.snapshot())
}

func TestRiskOwnerAssignmentLeavesNonReviewWorkflowsUnchanged(t *testing.T) {
	for _, state := range []string{"open", "detached", "unlinked"} {
		t.Run(state, func(t *testing.T) {
			f := newOwnerWorkflowFixture(t)
			if state == "detached" {
				f.finish()
				_, err := testPool.Exec(context.Background(), `UPDATE risk_ticket_links SET ended_at=now(),end_reason='synthetic_handover' WHERE id=$1`, f.linkID)
				require.NoError(t, err)
			} else if state == "unlinked" {
				_, err := testPool.Exec(context.Background(), `DELETE FROM risk_ticket_links WHERE id=$1`, f.linkID)
				require.NoError(t, err)
			}
			out, err := f.assign(userV)
			require.NoError(t, err)
			require.NotNil(t, out)
		})
	}
}
