//go:build integration

package riskreminder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestDeliveryUsesRealIAMCommandBarrier(t *testing.T) {
	f := createFixture(t)
	recordAssignment(t, f, 1, f.P, tDue)
	svc := newService(func(ctx context.Context, tx pgx.Tx, ticketID, recipient int64) (bool, string, error) {
		if !repository.IAMGuardActive(ctx) || repository.IAMDB(ctx, testPool) != tx {
			return false, "", errors.New("delivery has no real IAM barrier")
		}
		return true, "", nil
	})
	if _, err := svc.GenerateDue(context.Background(), tGen, 10); err != nil {
		t.Fatal(err)
	}
	report, err := svc.DeliverDue(context.Background(), tGen, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Delivered) != 1 || len(report.Failed) != 0 {
		t.Fatalf("guarded delivery required: %+v", report)
	}
}

func TestEligibleScanAndJobAreReentrant(t *testing.T) {
	f := createFixture(t)
	recordAssignment(t, f, 1, f.P, tDue)
	checks := 0
	svc := newService(func(ctx context.Context, tx pgx.Tx, ticket, recipient int64) (bool, string, error) {
		checks++
		if !repository.IAMGuardActive(ctx) {
			return false, "", errors.New("unguarded scan/delivery")
		}
		return true, "", nil
	})
	job := NewJob(svc, func() time.Time { return tGen }, 10, 10)
	if err := job.Handle(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := job.Handle(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if got := inboxCount(t, f.P); got != 1 {
		t.Fatalf("replayed job inbox=%d", got)
	}
	if checks != 2 {
		t.Fatalf("scan and send must each authorize, got %d", checks)
	}
}

func TestEligibleScanClosesOnMissingCheckerAndLeavesNoIntent(t *testing.T) {
	f := createFixture(t)
	recordAssignment(t, f, 1, f.P, tDue)
	svc := newService(nil)
	if _, err := svc.GenerateEligibleDue(context.Background(), tGen, 10); err == nil {
		t.Fatal("unwired checker accepted")
	}
	rows, err := svc.IntentsByTicket(context.Background(), f.TicketID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatal("unwired scan created intent")
	}
}

func TestJobReportsPerIntentDeliveryFailures(t *testing.T) {
	f := createFixture(t)
	recordAssignment(t, f, 1, f.P, tDue)
	calls := 0
	svc := newService(func(context.Context, pgx.Tx, int64, int64) (bool, string, error) {
		calls++
		if calls > 1 {
			return false, "", errors.New("synthetic qualification read failure")
		}
		return true, "", nil
	})
	err := NewJob(svc, func() time.Time { return tGen }, 10, 10).Handle(context.Background(), nil)
	if err == nil {
		t.Fatal("delivery failure disappeared as successful cron tick")
	}
	if got := inboxCount(t, f.P); got != 0 {
		t.Fatalf("failed delivery inbox=%d", got)
	}
}

// Two callers racing on an absent generation must arbitrate by the unique key
// without trying to read from PostgreSQL's aborted transaction (25P02).
func TestAssignmentUniqueRaceReturnsOriginalFact(t *testing.T) {
	f := createFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Rollback(context.Background())
	b, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Rollback(context.Background())
	in := &AssignmentInput{TicketID: f.TicketID, Generation: 1, AssigneeID: f.P, DispatchedBy: f.M, DispatchedAt: tDispatch, AcceptDueAt: tDue, RuleID: "synthetic", RuleVersion: 1}
	first, err := RecordAssignmentTx(ctx, a, in)
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	if err := b.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		row *Assignment
		err error
	}
	done := make(chan outcome, 1)
	go func() { row, err := RecordAssignmentTx(ctx, b, in); done <- outcome{row, err} }()
	for {
		var waiting bool
		err := testPool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND wait_event_type='Lock')`, pid).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(5 * time.Millisecond):
		}
	}
	if err := a.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	result := waitCh(t, done, 5*time.Second, "unique-key arbitration")
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.row.ID != first.ID {
		t.Fatalf("new fact %d, want original %d", result.row.ID, first.ID)
	}
	if err := b.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestEligibleScanFailedCandidateDoesNotStarveNextTicket(t *testing.T) {
	f := createFixture(t)
	recordAssignment(t, f, 1, f.P, tDue)
	second := addTicket(t, f)
	tx, err := testPool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = RecordAssignmentTx(context.Background(), tx, &AssignmentInput{TicketID: second, Generation: 1, AssigneeID: f.P2, DispatchedBy: f.M, DispatchedAt: tDispatch, AcceptDueAt: tDue, RuleID: "synthetic", RuleVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := newService(func(ctx context.Context, tx pgx.Tx, id, recipient int64) (bool, string, error) {
		if id == f.TicketID {
			return false, "", ErrPolicyUnavailable
		}
		return true, "", nil
	})
	if _, err := svc.GenerateEligibleDue(context.Background(), tGen, 1); !errors.Is(err, ErrPolicyUnavailable) {
		t.Fatalf("scan missing config = %v", err)
	}
	report, err := svc.GenerateEligibleDue(context.Background(), tGen, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Generated) != 1 || report.Generated[0].TicketID != second {
		t.Fatalf("healthy next ticket starved: %+v", report)
	}
	if got := inboxCount(t, f.P); got != 0 {
		t.Fatalf("configuration error delivered inbox=%d", got)
	}
}

func TestRuntimeRecreationRecoversFailedPolicyByOriginalKey(t *testing.T) {
	f := newEligibilityFixture(t)
	original := f.policy
	f.policy.RuleVersion++ // explicit configuration drift, never an approved default
	_, job, err := NewRuntime(testPool, repository.NewNotificationInboxRepo(testPool), eligibilityModel, f.resolver, func() time.Time { return tGen }, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := job.RunOnce(context.Background(), tGen)
	if !errors.Is(err, ErrPolicyUnavailable) {
		t.Fatalf("drift was not visible: %v", err)
	}
	if len(first.Scan.Failed) != 1 || len(first.Delivery.Delivered) != 0 {
		t.Fatalf("configuration drift reported success: %+v", first)
	}
	before := intentsOf(t, f.fixture)
	if len(before) != 1 || before[0].Status != StatusPending {
		t.Fatalf("missing recoverable original intent: %+v", before)
	}
	f.policy = original
	// A fresh runtime has no in-memory retry state. The persisted original key owns recovery.
	_, restarted, err := NewRuntime(testPool, repository.NewNotificationInboxRepo(testPool), eligibilityModel, f.resolver, func() time.Time { return tGen }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Handle(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Handle(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	after := intentsOf(t, f.fixture)
	if len(after) != 1 || after[0].ID != before[0].ID || after[0].Status != StatusDelivered {
		t.Fatalf("recovery changed original key: %+v", after)
	}
	if got := inboxCount(t, f.P); got != 1 {
		t.Fatalf("recreated runtime inbox=%d", got)
	}
}
