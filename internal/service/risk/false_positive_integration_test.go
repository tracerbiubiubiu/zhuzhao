//go:build integration

package risk_test

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
)

func TestFalsePositiveClosesWithBoundedSuppressionAndOriginalReceipt(t *testing.T) {
	seedRiskFixture(t)
	ctx := t.Context()
	svc := newService()
	suffix := uniqueSuffix()
	created := mustCreate(t, svc, "create-fp-"+suffix, "Synthetic false positive", orgGID, &userU)
	cmd := model.RiskFalsePositiveRequest{
		ID: created.Risk.ID, OperationID: "close-fp-" + suffix, ExpectedRevision: "1",
		Reason: "Synthetic evidence excludes the finding", SuppressionValidUntil: time.Now().UTC().Truncate(time.Second).Add(time.Hour),
	}

	_, err := svc.FalsePositive(ctx, cmd, userU)
	require.ErrorIs(t, err, errcode.ErrNoPermission, "ownership is not primary-anchor management")
	_, err = svc.FalsePositive(ctx, cmd, userN)
	require.ErrorIs(t, err, risk.ErrRiskNotFound)
	past := cmd
	past.SuppressionValidUntil = time.Now().Add(-time.Hour)
	_, err = svc.FalsePositive(ctx, past, userM)
	require.ErrorIs(t, err, errcode.ErrInvalidParams)
	stale := cmd
	stale.ExpectedRevision = "2"
	_, err = svc.FalsePositive(ctx, stale, userM)
	require.ErrorIs(t, err, errcode.ErrConcurrentModification)

	closed, err := svc.FalsePositive(ctx, cmd, userM)
	require.NoError(t, err)
	require.Equal(t, "closed", closed.Status)
	require.EqualValues(t, 2, closed.Risk.Revision)
	require.NotZero(t, closed.Suppression.ID)
	require.EqualValues(t, 1, closed.Suppression.Revision)
	require.Equal(t, cmd.SuppressionValidUntil, closed.Suppression.ValidUntil.UTC())
	detail, err := svc.Detail(ctx, created.Risk.ID, userM)
	require.NoError(t, err)
	require.NotNil(t, detail.View.ClosureKind)
	require.Equal(t, "false_positive", *detail.View.ClosureKind)

	receipt, err := svc.OperationResult(ctx, cmd.OperationID, userM)
	require.NoError(t, err)
	require.Equal(t, closed.Receipt.OperationID, receipt.OperationID)
	require.Equal(t, closed.Receipt.Revision, receipt.Revision)
	replayed, err := svc.FalsePositive(ctx, cmd, userM)
	require.NoError(t, err)
	require.True(t, replayed.Receipt.Replayed)
	require.Equal(t, closed.Suppression, replayed.Suppression)
	changed := cmd
	changed.Reason += " changed"
	_, err = svc.FalsePositive(ctx, changed, userM)
	require.ErrorIs(t, err, risk.ErrRiskOperationConflict)

	list, err := svc.ListSuppressions(ctx, userM, 1, 100)
	require.NoError(t, err)
	active := requireListedSuppression(t, list, closed.Suppression.ID)
	require.Equal(t, closed.Suppression, active.RiskSuppressionRef)
	require.True(t, active.Active)
	require.Equal(t, cmd.Reason, active.Reason)
	outside, err := svc.ListSuppressions(ctx, userN, 1, 100)
	require.NoError(t, err)
	require.Empty(t, outside.Items)

	remove := model.RiskSuppressionRemoveRequest{ID: closed.Suppression.ID, OperationID: "remove-fp-" + suffix,
		ExpectedRevision: "1", Reason: "Synthetic source review restored observation"}
	_, err = svc.RemoveSuppression(ctx, remove, userU)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	removed, err := svc.RemoveSuppression(ctx, remove, userM)
	require.NoError(t, err)
	require.Equal(t, "removed", removed.Status)
	require.EqualValues(t, 2, removed.Suppression.Revision)
	require.Equal(t, closed.Risk, removed.Risk, "ending suppression leaves the closed risk unchanged")
	afterRemoval, err := svc.ListSuppressions(ctx, userM, 1, 100)
	require.NoError(t, err)
	inactive := requireListedSuppression(t, afterRemoval, closed.Suppression.ID)
	require.False(t, inactive.Active)
	require.NotNil(t, inactive.EndedBy)
	require.Equal(t, userM, *inactive.EndedBy)
	require.NotNil(t, inactive.EndedReason)
	require.Equal(t, remove.Reason, *inactive.EndedReason)
	replayedRemove, err := svc.RemoveSuppression(ctx, remove, userM)
	require.NoError(t, err)
	require.True(t, replayedRemove.Receipt.Replayed)
	require.Equal(t, removed.Suppression, replayedRemove.Suppression)
	replayedClose, err := svc.FalsePositive(ctx, cmd, userM)
	require.NoError(t, err)
	require.Equal(t, closed.Suppression, replayedClose.Suppression, "original close result does not depend on latest suppression revision")

	var suppressions, closeEvents, removeEvents int
	require.NoError(t, testPool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM risk_suppressions WHERE risk_id=$1),
		       (SELECT COUNT(*) FROM risk_events WHERE risk_id=$1 AND event_kind='false_positive_closed'),
		       (SELECT COUNT(*) FROM risk_events WHERE risk_id=$1 AND event_kind='suppression_removed')`, created.Risk.ID).
		Scan(&suppressions, &closeEvents, &removeEvents))
	require.Equal(t, []int{1, 1, 1}, []int{suppressions, closeEvents, removeEvents})
}

func TestFalsePositivePreservesActiveLinkedWorkflow(t *testing.T) {
	f := newOwnerWorkflowFixture(t)
	ctx := context.Background()
	var id, revision int64
	// An imported risk can carry an assessed envelope despite its existing link.
	require.NoError(t, testPool.QueryRow(ctx, `
		UPDATE risk_items r SET status='assessed' FROM risk_ticket_links l
		WHERE l.id=$1 AND l.risk_id=r.id RETURNING r.id,r.revision`, f.linkID).Scan(&id, &revision))
	before := f.snapshot()
	op := "linked-fp-" + uniqueSuffix()
	svc := newService()
	result, err := svc.FalsePositive(ctx, model.RiskFalsePositiveRequest{
		ID: id, OperationID: op, ExpectedRevision: strconv.FormatInt(revision, 10),
		Reason: "Synthetic linked risk", SuppressionValidUntil: time.Now().Add(time.Hour),
	}, userM)
	require.ErrorIs(t, err, risk.ErrRiskInvalidState)
	require.Nil(t, result)
	require.Equal(t, before, f.snapshot())
	_, err = svc.OperationResult(ctx, op, userM)
	require.ErrorIs(t, err, risk.ErrRiskNotFound)
}

func TestFalsePositiveConcurrentClosuresCreateOneSuppression(t *testing.T) {
	seedRiskFixture(t)
	ctx := t.Context()
	svc := newService()
	suffix := uniqueSuffix()
	created := mustCreate(t, svc, "create-fp-race-"+suffix, "Synthetic concurrent false positive", orgGID, nil)
	cmd := model.RiskFalsePositiveRequest{
		ID: created.Risk.ID, ExpectedRevision: "1", Reason: "Synthetic independently observed false positive",
		SuppressionValidUntil: time.Now().Add(time.Hour),
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, operation := range []string{"fp-race-a-", "fp-race-b-"} {
		go func() {
			req := cmd
			req.OperationID = operation + suffix
			<-start
			_, err := svc.FalsePositive(ctx, req, userM)
			results <- err
		}()
	}
	close(start)
	successes := 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
			continue
		}
		require.ErrorIs(t, err, errcode.ErrConcurrentModification)
	}
	require.Equal(t, 1, successes)
	var suppressions, receipts int
	require.NoError(t, testPool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM risk_suppressions WHERE risk_id=$1),
		       (SELECT count(*) FROM risk_operations WHERE result_risk_id=$1 AND command='false_positive')`, cmd.ID).
		Scan(&suppressions, &receipts))
	require.Equal(t, []int{1, 1}, []int{suppressions, receipts})
}

func TestFalsePositiveReceiptRechecksPrimaryManagement(t *testing.T) {
	seedRiskFixture(t)
	ctx := t.Context()
	svc := newService()
	suffix := uniqueSuffix()
	created := mustCreate(t, svc, "create-fp-revoked-"+suffix, "Synthetic false positive with revoked manager", orgGID, nil)
	_, actor := seedCasbinRole(t, "/api/v1/risk-items/false-positive")
	cmd := model.RiskFalsePositiveRequest{
		ID: created.Risk.ID, OperationID: "close-fp-revoked-" + suffix, ExpectedRevision: "1",
		Reason: "Synthetic false positive", SuppressionValidUntil: time.Now().Add(time.Hour),
	}
	_, err := svc.FalsePositive(ctx, cmd, actor)
	require.NoError(t, err)
	_, err = svc.OperationResult(ctx, cmd.OperationID, actor)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE user_orgs SET org_member_role='member' WHERE user_id=$1 AND org_id=$2`, actor, orgGID)
	require.NoError(t, err)
	_, err = svc.Detail(ctx, cmd.ID, actor)
	require.NoError(t, err, "primary membership still confers ordinary risk reading")
	_, err = svc.OperationResult(ctx, cmd.OperationID, actor)
	require.ErrorIs(t, err, risk.ErrRiskNotFound)
	_, err = svc.FalsePositive(ctx, cmd, actor)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
}

func requireListedSuppression(t *testing.T, list *model.RiskSuppressionList, id int64) model.RiskSuppressionView {
	t.Helper()
	index := slices.IndexFunc(list.Items, func(item model.RiskSuppressionView) bool { return item.ID == id })
	require.NotEqual(t, -1, index, "managed suppression %d missing", id)
	return list.Items[index]
}
