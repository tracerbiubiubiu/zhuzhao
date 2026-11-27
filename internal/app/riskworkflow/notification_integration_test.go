//go:build integration

package riskworkflow

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	messageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

func TestReviewNoticeFinishUsesStableReviewerOnce(t *testing.T) {
	f := newWriteFixture(t)
	cmd := f.finishCommand(t)
	var notices []ReviewNotice
	f.service.SetReviewNoticeRecorder(func(ctx context.Context, tx pgx.Tx, notice ReviewNotice) error {
		require.Same(t, tx, repository.IAMDB(ctx, workflowPool))
		notices = append(notices, notice)
		_, err := tx.Exec(ctx, `INSERT INTO notifications (recipient_id,biz_type,biz_id,title)
			VALUES($1,'ticket',$2,'synthetic review notice')`, notice.RecipientID, notice.TicketID)
		return err
	})
	caller := func(ctx context.Context) (riskremediation.Result, error) { return f.service.Finish(ctx, cmd) }
	got, err := f.admittedWrite(t, f.other, finishPath, nil, caller)
	require.NoError(t, err)
	require.Len(t, got.Items, 2)
	want := ReviewNotice{TicketID: f.ticket, TicketRevision: got.TicketRevision, RecipientID: f.actor, OperationID: cmd.OperationID}
	require.Equal(t, []ReviewNotice{want}, notices, "both links require the same reviewer")
	replay, err := f.admittedWrite(t, f.other, finishPath, nil, caller)
	require.NoError(t, err)
	require.Equal(t, got, replay)
	require.Equal(t, []ReviewNotice{want}, notices)
	var count int
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT count(*) FROM notifications
		WHERE biz_id=$1 AND title='synthetic review notice'`, f.ticket).Scan(&count))
	require.Equal(t, 1, count)
}

func TestReviewNoticeWriteFailureRollsBackFinish(t *testing.T) {
	f := newWriteFixture(t)
	cmd := f.finishCommand(t)
	before := f.writeSnapshot(t)
	called := false
	f.service.SetReviewNoticeRecorder(func(ctx context.Context, tx pgx.Tx, notice ReviewNotice) error {
		called = true
		require.Same(t, tx, repository.IAMDB(ctx, workflowPool))
		_, err := tx.Exec(ctx, `INSERT INTO notifications (recipient_id,biz_type,biz_id,title)
			VALUES(NULL,'ticket',$1,'synthetic failed review notice')`, notice.TicketID)
		return err
	})
	got, err := f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) {
		return f.service.Finish(ctx, cmd)
	})
	require.True(t, called)
	require.Error(t, err)
	require.Empty(t, got)
	require.Equal(t, before, f.writeSnapshot(t))
}

func TestReviewNoticeRechecksOriginalPendingReview(t *testing.T) {
	f := newWriteFixture(t)
	cmd := f.finishCommand(t)
	var notice ReviewNotice
	f.service.SetReviewNoticeRecorder(func(_ context.Context, _ pgx.Tx, n ReviewNotice) error {
		notice = n
		return nil
	})
	result, err := f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) {
		return f.service.Finish(ctx, cmd)
	})
	require.NoError(t, err)
	check := func(n ReviewNotice) (eligible bool, reason string) {
		t.Helper()
		require.NoError(t, repository.WithIAMCommandGuard(f.ctx, workflowPool, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			eligible, reason, err = CheckReviewNotice(ctx, tx, f.tickets, receiptModel, n)
			return err
		}))
		return eligible, reason
	}
	eligible, reason := check(notice)
	require.True(t, eligible, reason)

	wrongRecipient := notice
	wrongRecipient.RecipientID = f.other
	eligible, reason = check(wrongRecipient)
	require.False(t, eligible)
	require.NotEmpty(t, reason)

	partial := f.reviewCommand(t, result, 0)
	partiallyReviewed, err := f.admittedWrite(t, f.actor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) {
		return f.service.Review(ctx, partial)
	})
	require.NoError(t, err)
	require.Greater(t, partiallyReviewed.TicketRevision, notice.TicketRevision)
	eligible, reason = check(notice)
	require.True(t, eligible, reason, "the other item from the original operation still awaits review")

	_, err = workflowPool.Exec(f.ctx, `DELETE FROM casbin_rule WHERE ptype='p' AND v0=$1 AND v1=$2 AND v2='POST'`, "role::"+f.role, reviewPath)
	require.NoError(t, err)
	eligible, reason = check(notice)
	require.False(t, eligible)
	require.Equal(t, "recipient_unauthorized", reason)
}

func TestReviewNoticeInboxUsesCurrentScopeBeforeCountsAndWrites(t *testing.T) {
	f := newWriteFixture(t)
	f.service.SetReviewNoticeRecorder(func(ctx context.Context, tx pgx.Tx, n ReviewNotice) error {
		return messageservice.RecordReviewNotice(ctx, tx, workflowPool, repository.MessageDeliverySource(n), "")
	})
	inbox := ticket.NewInboxService(workflowPool, 90, nil)
	inbox.SetReviewNoticeCheck(func(ctx context.Context, tx pgx.Tx, source repository.MessageDeliverySource) (bool, string, error) {
		return CheckReviewNotice(ctx, tx, f.tickets, receiptModel, ReviewNotice(source))
	})
	cmd := f.finishCommand(t)
	_, err := f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) {
		return f.service.Finish(ctx, cmd)
	})
	require.NoError(t, err)

	list, total, err := inbox.Inbox(f.ctx, f.actor, true, 1, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, list, 1)
	require.Equal(t, "工单待复核", list[0].Title)
	require.NotEmpty(t, list[0].Link)
	noticeID := list[0].ID

	assertHidden := func(actor int64) {
		t.Helper()
		list, total, err := inbox.Inbox(f.ctx, actor, false, 1, 1)
		require.NoError(t, err)
		require.Zero(t, total)
		require.Empty(t, list)
		unread, err := inbox.UnreadCount(f.ctx, actor)
		require.NoError(t, err)
		require.Zero(t, unread)
		var notFound *errcode.Error
		require.ErrorAs(t, inbox.MarkRead(f.ctx, noticeID, actor), &notFound)
		require.Equal(t, errcode.ErrNotFound.Code, notFound.Code)
		marked, err := inbox.MarkAllRead(f.ctx, actor)
		require.NoError(t, err)
		require.Zero(t, marked)
	}
	assertHidden(f.other)

	_, err = workflowPool.Exec(f.ctx, `DELETE FROM casbin_rule WHERE ptype='p' AND v0=$1 AND v1=$2 AND v2='POST'`, "role::"+f.role, reviewPath)
	require.NoError(t, err)
	assertHidden(f.actor)

	var preservedUnread bool
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT read_at IS NULL FROM notifications WHERE id=$1`, noticeID).Scan(&preservedUnread))
	require.True(t, preservedUnread)
}

func TestReviewNoticeReceiptAllowsCompletedReviewWithCurrentView(t *testing.T) {
	f := newWriteFixture(t)
	cmd := f.finishCommand(t)
	var notice ReviewNotice
	f.service.SetReviewNoticeRecorder(func(_ context.Context, _ pgx.Tx, n ReviewNotice) error {
		notice = n
		return nil
	})
	finished, err := f.admittedWrite(t, f.other, finishPath, nil, func(ctx context.Context) (riskremediation.Result, error) {
		return f.service.Finish(ctx, cmd)
	})
	require.NoError(t, err)
	review := f.reviewCommand(t, finished, 0, 1)
	closed, err := f.admittedWrite(t, f.actor, reviewPath, nil, func(ctx context.Context) (riskremediation.Result, error) {
		return f.service.Review(ctx, review)
	})
	require.NoError(t, err)
	require.True(t, closed.TerminalAllowed)

	check := func() (eligible bool, reason string) {
		t.Helper()
		require.NoError(t, repository.WithIAMCommandGuard(f.ctx, workflowPool, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			eligible, reason, err = CheckReviewNoticeReceipt(ctx, tx, f.tickets, receiptModel, notice)
			return err
		}))
		return eligible, reason
	}
	_, err = workflowPool.Exec(f.ctx, `DELETE FROM casbin_rule WHERE ptype='p' AND v0=$1 AND v1=$2 AND v2='POST'`, "role::"+f.role, reviewPath)
	require.NoError(t, err)
	eligible, reason := check()
	require.True(t, eligible, reason, "review completion and losing the review action leave the historical status view")

	_, err = workflowPool.Exec(f.ctx, `DELETE FROM casbin_rule WHERE ptype='p' AND v0=$1 AND v1='/api/v1/tickets/:id' AND v2='GET'`, "role::"+f.role)
	require.NoError(t, err)
	eligible, reason = check()
	require.False(t, eligible)
	require.Equal(t, "recipient_unauthorized", reason)
}
