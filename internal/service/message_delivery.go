package service

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/channelsender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/msgrender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type ReviewNoticeCheck func(context.Context, pgx.Tx, repository.MessageDeliverySource) (bool, string, error)

type MessageDeliveryWorker struct {
	db      *pgxpool.Pool
	repo    *repository.MessageDeliveryRepo
	port    channelsender.HTTPMessagePort
	profile string
	check   ReviewNoticeCheck
}

func NewMessageDeliveryWorker(db *pgxpool.Pool, profile string, port channelsender.HTTPMessagePort, check ReviewNoticeCheck) *MessageDeliveryWorker {
	return &MessageDeliveryWorker{db: db, repo: repository.NewMessageDeliveryRepo(db), profile: profile, port: port, check: check}
}

func RecordReviewNotice(ctx context.Context, tx pgx.Tx, db *pgxpool.Pool, source repository.MessageDeliverySource, profile string) error {
	store := repository.NewMessageDeliveryRepo(db)
	return store.WithReviewSource(ctx, tx, source, func() (int64, string, error) {
		id := source.TicketID
		notice := &model.NotificationRow{
			RecipientID: source.RecipientID, BizType: "ticket", BizID: &id,
			Title: "工单待复核", Link: "/tickets/" + strconv.FormatInt(id, 10),
		}
		if err := repository.NewNotificationInboxRepo(db).InsertTx(ctx, tx, notice); err != nil {
			return 0, "", err
		}
		if profile == "" {
			return notice.ID, "", nil
		}

		subject, body, version, err := store.ReviewTemplate(ctx, tx)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, "", err
		}
		reason := ""
		if version == 0 {
			reason = "template_unavailable"
		}
		// The shared mock profile only projects fixed safe fields. Custom ticket
		// fields, title, proposal contents and identities are never loaded here.
		vars := map[string]any{"ticket": map[string]any{"status": "pending_verify"}}
		s := msgrender.RenderQuadrantEscaped(subject, vars, "webhook")
		b := msgrender.RenderQuadrantEscaped(body, vars, "webhook")
		if s.Err != nil || b.Err != nil || len(s.Warnings) != 0 || len(b.Warnings) != 0 || len(b.Text) > 8192 || len(s.Text) == 0 {
			s.Text, b.Text = "", ""
			if reason == "" {
				reason = "template_invalid"
			}
		}
		deliveryID, err := store.Record(ctx, tx, source, profile, s.Text, b.Text,
			repository.ReviewNoticeTemplate+":"+strconv.FormatInt(version, 10), reason)
		return notice.ID, deliveryID, err
	})
}

func (w *MessageDeliveryWorker) Handle(ctx context.Context, _ json.RawMessage) error {
	var claim *repository.MessageDeliveryClaim
	err := repository.WithIAMCommandGuard(ctx, w.db, func(guarded context.Context, tx pgx.Tx) error {
		var err error
		claim, err = w.repo.Claim(guarded, tx, w.profile)
		if err != nil || claim == nil || claim.Mode == "query" {
			return err
		}
		var reason string
		err = repository.WithIAMSavepoint(guarded, w.db, tx, func(current context.Context, child pgx.Tx) error {
			var err error
			reason, err = w.admitSend(current, child, claim)
			return err
		})
		if err != nil {
			out := repository.MessageDeliveryOutcome{State: "failed", Reason: "recipient_check_unavailable"}
			if next := deliveryNextAttempt(claim, 0); next != nil {
				out.State, out.NextAt = "pending", next
			}
			err = w.repo.Finish(guarded, tx, claim, out)
			claim = nil
			return err
		}
		if reason != "" {
			err = w.repo.Finish(guarded, tx, claim, repository.MessageDeliveryOutcome{State: "suppressed", Reason: reason})
			claim = nil
			return err
		}
		return w.repo.MarkSend(guarded, tx, claim)
	})
	if err != nil {
		return errors.New("claim message delivery")
	}
	if claim == nil {
		return nil
	}

	out := w.attempt(ctx, claim)
	if err := w.repo.Finish(ctx, w.db, claim, out); err != nil {
		return errors.New("record message delivery result")
	}
	return nil
}

func (w *MessageDeliveryWorker) admitSend(ctx context.Context, tx pgx.Tx, claim *repository.MessageDeliveryClaim) (string, error) {
	if !time.Now().Before(claim.ExpiresAt) {
		return "expired", nil
	}
	if w.check == nil {
		return "", errors.New("message recipient check missing")
	}
	eligible, reason, err := w.check(ctx, tx, claim.MessageDeliverySource)
	if err != nil {
		return "", err
	}
	if !eligible {
		if reason == "" {
			reason = "recipient_unauthorized"
		}
		return reason, nil
	}
	optedOut, err := w.repo.RecipientOptedOut(ctx, tx, claim.RecipientID)
	if optedOut {
		return "recipient_opted_out", err
	}
	return "", err
}

func (w *MessageDeliveryWorker) attempt(ctx context.Context, claim *repository.MessageDeliveryClaim) repository.MessageDeliveryOutcome {
	request := claim.Request
	send := claim.Mode == "send"
	var receipt channelsender.SendReceiptV1
	var err error
	if send {
		receipt, err = w.port.Send(ctx, request)
	} else {
		receipt, err = w.port.Query(ctx, channelsender.QueryRequestV1{
			SchemaVersion: "1", DeliveryID: request.DeliveryID, ChannelProfileRef: request.ChannelProfileRef, ProviderReceiptRef: claim.ProviderReceiptRef,
		})
	}
	binding := receipt.ValidateBinding(request.DeliveryID, request.ChannelProfileRef, request.PayloadSHA256)
	if binding != nil || err != nil && receipt.State != "rejected" {
		reason := "acceptance_unconfirmed"
		var next *time.Time
		if errors.Is(err, channelsender.ErrQueryUnsupported) {
			reason = "query_unsupported"
		} else {
			next = deliveryNextAttempt(claim, 0)
		}
		return repository.MessageDeliveryOutcome{State: "unknown", Reason: reason, NextAt: next}
	}
	out := receiptOutcome(receipt)
	out.RetryIfStaleAt = deliveryNextAttempt(claim, 0)
	switch receipt.State {
	case "accepted", "unknown":
		out.NextAt = deliveryNextAttempt(claim, 0)
	case "rejected":
		var providerErr *channelsender.MessageHTTPError
		if send && errors.As(err, &providerErr) && providerErr.Retryable && claim.Sends+1 < 20 {
			if next := deliveryNextAttempt(claim, providerErr.RetryAfter); next != nil {
				out.State, out.NextAt = "pending", next
			}
		}
	}
	return out
}

func receiptOutcome(receipt channelsender.SendReceiptV1) repository.MessageDeliveryOutcome {
	observedAt, _ := time.Parse(time.RFC3339Nano, receipt.ObservedAt)
	state := receipt.State
	if state == "rejected" {
		state = "failed"
	}
	return repository.MessageDeliveryOutcome{
		State: state, Reason: receipt.SafeReasonCode, ProviderReceiptRef: receipt.ProviderReceiptRef, ObservedAt: &observedAt,
	}
}

func deliveryNextAttempt(claim *repository.MessageDeliveryClaim, retryAfter time.Duration) *time.Time {
	if claim.Attempt >= 20 {
		return nil
	}
	backoff := 5*time.Second<<min(claim.Attempt-1, 6) + time.Duration(rand.Int64N(int64(time.Second)))
	delay := max(retryAfter, min(backoff, 5*time.Minute))
	next := time.Now().Add(delay)
	if next.After(claim.ExpiresAt) {
		return nil
	}
	return &next
}

type MessageDeliveryService struct {
	db      *pgxpool.Pool
	check   ReviewNoticeCheck
	profile string
}

func NewMessageDeliveryService(db *pgxpool.Pool, check ReviewNoticeCheck, profile string) *MessageDeliveryService {
	return &MessageDeliveryService{db: db, check: check, profile: profile}
}

func (s *MessageDeliveryService) Receipt(ctx context.Context, recipient int64, id string) (repository.MessageDeliveryReceipt, error) {
	var result repository.MessageDeliveryReceipt
	if recipient <= 0 || len(id) != 68 || s.check == nil {
		return result, errcode.ErrNotFound
	}
	err := repository.WithIAMCommandGuard(ctx, s.db, func(guarded context.Context, tx pgx.Tx) error {
		receipt, source, err := repository.NewMessageDeliveryRepo(s.db).RecipientReceipt(guarded, tx, id, recipient)
		if errors.Is(err, pgx.ErrNoRows) {
			return errcode.ErrNotFound
		}
		if err != nil {
			return errcode.ErrServiceUnavailable
		}
		visible, _, err := s.check(guarded, tx, source)
		if err != nil {
			return errcode.ErrServiceUnavailable
		}
		if !visible {
			return errcode.ErrNotFound
		}
		result = receipt
		return nil
	})
	return result, err
}

func (s *MessageDeliveryService) AcceptProviderReceipt(ctx context.Context, receipt channelsender.AsyncReceiptV1) error {
	err := repository.NewMessageDeliveryRepo(s.db).ApplyProviderReceipt(ctx, s.profile, receipt, receiptOutcome(receipt.SendReceiptV1))
	if errors.Is(err, repository.ErrInvalidProviderMessage) {
		return errcode.ErrConflict
	}
	if err != nil {
		return errcode.ErrServiceUnavailable
	}
	return nil
}
