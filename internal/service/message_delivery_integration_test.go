//go:build integration

package service_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/channelsender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

type syntheticMessagePort struct {
	sends   atomic.Int64
	queries atomic.Int64
	send    func(channelsender.SendRequestV1) (channelsender.SendReceiptV1, error)
	query   func(channelsender.QueryRequestV1) (channelsender.SendReceiptV1, error)
}

func (p *syntheticMessagePort) Send(_ context.Context, r channelsender.SendRequestV1) (channelsender.SendReceiptV1, error) {
	p.sends.Add(1)
	return p.send(r)
}
func (p *syntheticMessagePort) Query(_ context.Context, r channelsender.QueryRequestV1) (channelsender.SendReceiptV1, error) {
	p.queries.Add(1)
	return p.query(r)
}

// These tests call one worker iteration directly. Make the row due on
// PostgreSQL's clock instead of relying on the host and fixture VM ticking together.
func makeSyntheticDeliveryDue(t *testing.T, ctx context.Context, id string) {
	t.Helper()
	_, err := testPool.Exec(ctx, "UPDATE notification_deliveries SET next_at=clock_timestamp() WHERE id=$1", id)
	require.NoError(t, err)
}

func TestMessage_TimeoutUnknown(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMessageDeliveryRepo(testPool)
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	candidate := repository.MessageDeliverySource{
		TicketID: 7815, TicketRevision: 7, RecipientID: 3815, OperationID: "synthetic-original-operation",
	}
	id, err := store.Record(ctx, tx, candidate, "mock-v1", "Review", "Open the workspace", "template-v1", "")
	require.NoError(t, err)
	duplicate, err := store.Record(ctx, tx, candidate, "mock-v1", "Review", "Open the workspace", "template-v1", "")
	require.NoError(t, err)
	require.Equal(t, id, duplicate)
	require.NoError(t, tx.Commit(ctx))
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, "DELETE FROM notification_deliveries WHERE id=$1", id)
	})

	var providerRequest channelsender.SendRequestV1
	port := &syntheticMessagePort{
		send: func(r channelsender.SendRequestV1) (channelsender.SendReceiptV1, error) {
			providerRequest = r
			return channelsender.SendReceiptV1{}, errors.New("synthetic lost response")
		},
		query: func(r channelsender.QueryRequestV1) (channelsender.SendReceiptV1, error) {
			require.Equal(t, providerRequest.DeliveryID, r.DeliveryID)
			require.Equal(t, providerRequest.ChannelProfileRef, r.ChannelProfileRef)
			return channelsender.SendReceiptV1{SchemaVersion: "1", DeliveryID: r.DeliveryID, ChannelProfileRef: r.ChannelProfileRef, PayloadSHA256: providerRequest.PayloadSHA256, State: "delivered", ProviderReceiptRef: "proof-original", ObservedAt: time.Now().UTC().Format(time.RFC3339), RetryClass: "none"}, nil
		},
	}
	check := func(context.Context, pgx.Tx, repository.MessageDeliverySource) (bool, string, error) {
		return true, "", nil
	}
	worker := service.NewMessageDeliveryWorker(testPool, "mock-v1", port, check)
	makeSyntheticDeliveryDue(t, ctx, id)
	require.NoError(t, worker.Handle(ctx, nil))
	receipt, err := store.Receipt(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "unknown", receipt.State)
	require.EqualValues(t, 1, port.sends.Load())
	require.EqualValues(t, 0, port.queries.Load())

	makeSyntheticDeliveryDue(t, ctx, id)
	restarted := service.NewMessageDeliveryWorker(testPool, "mock-v1", port, check)
	require.NoError(t, restarted.Handle(ctx, nil))
	receipt, err = store.Receipt(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "delivered", receipt.State)
	require.Equal(t, "proof-original", receipt.ProviderReceiptRef)
	require.EqualValues(t, 1, port.sends.Load())
	require.EqualValues(t, 1, port.queries.Load())
}

func TestMessage_ProviderCallbackOutOfOrder(t *testing.T) {
	ctx := context.Background()
	const profile = "synthetic-callback-v1"
	store := repository.NewMessageDeliveryRepo(testPool)
	source := repository.MessageDeliverySource{TicketID: 7816, TicketRevision: 8, RecipientID: 3816, OperationID: "synthetic-callback-operation"}
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	id, err := store.Record(ctx, tx, source, profile, "Review", "Review body", "template-v1", "")
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, "DELETE FROM notification_deliveries WHERE id=$1", id)
	})
	var request channelsender.SendRequestV1
	observed := time.Now().UTC().Truncate(time.Second)
	port := &syntheticMessagePort{send: func(r channelsender.SendRequestV1) (channelsender.SendReceiptV1, error) {
		request = r
		return channelsender.SendReceiptV1{SchemaVersion: "1", DeliveryID: id, ChannelProfileRef: profile,
			PayloadSHA256: r.PayloadSHA256, State: "accepted", ObservedAt: observed.Format(time.RFC3339), RetryClass: "none"}, nil
	}}
	check := func(context.Context, pgx.Tx, repository.MessageDeliverySource) (bool, string, error) {
		return true, "", nil
	}
	worker := service.NewMessageDeliveryWorker(testPool, profile, port, check)
	makeSyntheticDeliveryDue(t, ctx, id)
	require.NoError(t, worker.Handle(ctx, nil))
	svc := service.NewMessageDeliveryService(testPool, check, profile)
	snapshot, err := svc.Receipt(ctx, source.RecipientID, id)
	require.NoError(t, err)
	require.Equal(t, "accepted", snapshot.State)
	_, err = svc.Receipt(ctx, source.RecipientID+1, id)
	require.ErrorIs(t, err, errcode.ErrNotFound)

	proven := channelsender.AsyncReceiptV1{ProviderEventID: "proof-delivered", SendReceiptV1: channelsender.SendReceiptV1{
		SchemaVersion: "1", DeliveryID: id, ChannelProfileRef: profile, PayloadSHA256: request.PayloadSHA256,
		State: "delivered", ProviderReceiptRef: "proof-1", ObservedAt: observed.Add(time.Second).Format(time.RFC3339), RetryClass: "none",
	}}
	mismatched := proven
	mismatched.PayloadSHA256 = strings.Repeat("1", 64)
	require.ErrorIs(t, svc.AcceptProviderReceipt(ctx, mismatched), errcode.ErrConflict)
	require.NoError(t, svc.AcceptProviderReceipt(ctx, proven))
	require.NoError(t, svc.AcceptProviderReceipt(ctx, proven))
	conflict := proven
	conflict.ProviderReceiptRef = "other-proof"
	require.ErrorIs(t, svc.AcceptProviderReceipt(ctx, conflict), errcode.ErrConflict)

	late := proven
	late.ProviderEventID = "late-acceptance"
	late.State = "accepted"
	late.ObservedAt = observed.Format(time.RFC3339)
	require.NoError(t, svc.AcceptProviderReceipt(ctx, late))
	snapshot, err = svc.Receipt(ctx, source.RecipientID, id)
	require.NoError(t, err)
	require.Equal(t, "delivered", snapshot.State)
	require.Equal(t, "proof-1", snapshot.ProviderReceiptRef)
	var events int
	require.NoError(t, testPool.QueryRow(ctx, "SELECT count(*) FROM notification_provider_events WHERE delivery_id=$1", id).Scan(&events))
	require.Equal(t, 2, events)
}

func TestMessage_IntentPreferenceAndPreview(t *testing.T) {
	ctx := context.Background()
	recipient := rbacTestUser(t, "mock-notice-pref-"+uniqueSuffix(), "M"+uniqueSuffix(), nil)
	const profile = "synthetic-preference-v1"
	source := repository.MessageDeliverySource{TicketID: 7817, TicketRevision: 9, RecipientID: recipient, OperationID: "synthetic-preference-operation"}
	store := repository.NewMessageDeliveryRepo(testPool)
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	require.NoError(t, service.RecordReviewNotice(ctx, tx, testPool, source, profile))
	require.NoError(t, tx.Commit(ctx))
	var notice int64
	var id string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT notification_id,delivery_id FROM notification_review_notices
		WHERE ticket_id=$1 AND ticket_revision=$2 AND recipient_id=$3`, source.TicketID, source.TicketRevision, recipient).Scan(&notice, &id))
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, "DELETE FROM notification_review_notices WHERE notification_id=$1", notice)
		_, _ = testPool.Exec(ctx, "DELETE FROM notifications WHERE id=$1", notice)
		_, _ = testPool.Exec(ctx, "DELETE FROM notification_deliveries WHERE id=$1", id)
		_, _ = testPool.Exec(ctx, "DELETE FROM notification_optouts WHERE user_id=$1", recipient)
	})
	require.NoError(t, service.NewNotificationPrefService(testPool).SetOptout(ctx, recipient, "ticket", repository.ReviewNoticeAction, true))

	subject, body, err := service.PreviewMessageTemplate(repository.ReviewNoticeTemplate, "webhook", "Review", "{{ticket.status}}")
	require.NoError(t, err)
	require.Equal(t, "Review", subject)
	require.Equal(t, "pending_verify", body)
	_, _, err = service.PreviewMessageTemplate(repository.ReviewNoticeTemplate, "webhook", "{{ticket.title}}", "Body")
	require.ErrorIs(t, err, errcode.ErrInvalidParams)
	snapshot, err := store.Receipt(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "pending", snapshot.State)

	port := &syntheticMessagePort{}
	check := func(context.Context, pgx.Tx, repository.MessageDeliverySource) (bool, string, error) {
		return true, "", nil
	}
	makeSyntheticDeliveryDue(t, ctx, id)
	require.NoError(t, service.NewMessageDeliveryWorker(testPool, profile, port, check).Handle(ctx, nil))
	snapshot, err = store.Receipt(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "suppressed", snapshot.State)
	require.Equal(t, "recipient_opted_out", snapshot.Reason)
	require.Zero(t, port.sends.Load())
	require.Zero(t, port.queries.Load())
	var unread bool
	require.NoError(t, testPool.QueryRow(ctx, "SELECT read_at IS NULL FROM notifications WHERE id=$1", notice).Scan(&unread))
	require.True(t, unread)
}
