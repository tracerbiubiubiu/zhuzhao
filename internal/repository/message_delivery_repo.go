package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/channelsender"
)

const ReviewNoticeAction = "ticket.review_requested"
const ReviewNoticeTemplate = "ticket_review_http_mock"

type MessageDeliverySource struct {
	TicketID, TicketRevision, RecipientID int64
	OperationID                           string
}

type MessageDeliveryReceipt struct {
	ID                 string    `json:"delivery_id"`
	State              string    `json:"state"`
	Reason             string    `json:"reason,omitempty"`
	ProviderReceiptRef string    `json:"-"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type MessageDeliveryClaim struct {
	MessageDeliverySource
	Request            channelsender.SendRequestV1
	Mode               string
	Revision           int64
	Attempt, Sends     int
	ExpiresAt          time.Time
	ProviderReceiptRef string
}

type MessageDeliveryOutcome struct {
	State, Reason, ProviderReceiptRef string
	ObservedAt                        *time.Time
	NextAt, RetryIfStaleAt            *time.Time
}

type MessageDeliveryRepo struct{ db *pgxpool.Pool }

func NewMessageDeliveryRepo(db *pgxpool.Pool) *MessageDeliveryRepo {
	return &MessageDeliveryRepo{db: db}
}

func (r *MessageDeliveryRepo) RegisterProfile(ctx context.Context, profile, binding string) error {
	tag, err := r.db.Exec(ctx, `INSERT INTO notification_http_profiles (profile_ref,binding_sha256)
		VALUES ($1,$2) ON CONFLICT (profile_ref) DO UPDATE SET binding_sha256=EXCLUDED.binding_sha256
		WHERE notification_http_profiles.binding_sha256=EXCLUDED.binding_sha256`, profile, binding)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("HTTP message profile binding changed")
	}
	return nil
}

func (r *MessageDeliveryRepo) ReviewTemplate(ctx context.Context, tx pgx.Tx) (subject, body string, version int64, err error) {
	err = tx.QueryRow(ctx, `SELECT subject,body,version FROM message_templates
		WHERE code=$1 AND review_http_mock_seed AND channel='webhook' AND biz_type='ticket' AND enabled`, ReviewNoticeTemplate).
		Scan(&subject, &body, &version)
	return
}

func (r *MessageDeliveryRepo) WithReviewSource(ctx context.Context, tx pgx.Tx, source MessageDeliverySource, create func() (int64, string, error)) error {
	tag, err := tx.Exec(ctx, `INSERT INTO notification_review_notices (ticket_id,ticket_revision,recipient_id,operation_id)
		VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, source.TicketID, source.TicketRevision, source.RecipientID, source.OperationID)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	notificationID, deliveryID, err := create()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE notification_review_notices SET notification_id=$4,delivery_id=NULLIF($5,'')
		WHERE ticket_id=$1 AND ticket_revision=$2 AND recipient_id=$3`, source.TicketID, source.TicketRevision, source.RecipientID, notificationID, deliveryID)
	return err
}

// Record freezes a single recipient's approved synthetic payload before a worker can claim it.
func (r *MessageDeliveryRepo) Record(ctx context.Context, tx pgx.Tx, source MessageDeliverySource, profile, subject, body, templateVersion, reason string) (string, error) {
	const sourceName = "core.risk.remediation"
	const ruleID = "risk-review-https"
	const ruleVersion = int64(1)
	const channel = "http_mock"
	const semanticKey = "review_requested"
	if reason == "" {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('message-backlog:' || $1, 0))`, profile); err != nil {
			return "", err
		}
		// Accepted or unknown receipts remain readable after automatic reconciliation stops.
		var queued int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM notification_deliveries
			WHERE profile_ref=$1 AND state IN ('pending','sending','accepted','unknown')
			  AND (next_at IS NOT NULL OR lease_until IS NOT NULL)`, profile).Scan(&queued); err != nil {
			return "", err
		}
		if queued >= 5000 {
			return "", errors.New("message delivery backlog at capacity")
		}
	}
	eventID := "risk-review-ticket-" + strconv.FormatInt(source.TicketID, 10) + ":" + strconv.FormatInt(source.TicketRevision, 10)
	recipient := strconv.FormatInt(source.RecipientID, 10)
	keyMaterial, err := json.Marshal([]any{sourceName, eventID, ruleID, ruleVersion, recipient, channel, semanticKey})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(keyMaterial)
	id := "msg_" + hex.EncodeToString(digest[:])
	expiresAt := time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)
	operationHash := sha256.Sum256([]byte(source.OperationID))
	request := channelsender.SendRequestV1{
		SchemaVersion: "1", DeliveryID: id, IdempotencyKey: id, ChannelProfileRef: profile,
		RecipientRef: "mock-user-" + recipient, TemplateVersion: templateVersion,
		Subject: subject, Body: body, BusinessRef: "ticket-" + strconv.FormatInt(source.TicketID, 10),
		SourceEventRef: channelsender.MessageSourceRef{Source: sourceName, EventID: eventID},
		RuleID:         ruleID, RuleVersion: ruleVersion, OperationID: "op_" + hex.EncodeToString(operationHash[:]), ExpiresAt: expiresAt.Format(time.RFC3339),
	}
	state := "pending"
	var nextAt *time.Time
	if reason == "" {
		request.PayloadSHA256, err = request.Hash()
		if err != nil {
			return "", err
		}
		now := time.Now()
		nextAt = &now
	} else {
		state = "failed"
	}
	_, err = tx.Exec(ctx, `INSERT INTO notification_deliveries
		(id,source,source_event_id,rule_id,rule_version,recipient_user_id,channel,semantic_key,
		 ticket_id,ticket_revision,operation_id,profile_ref,payload,payload_sha256,state,reason,next_at,expires_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		 ON CONFLICT (source,source_event_id,rule_id,rule_version,recipient_user_id,channel,semantic_key) DO NOTHING`,
		id, sourceName, eventID, ruleID, ruleVersion, source.RecipientID, channel, semanticKey,
		source.TicketID, source.TicketRevision, source.OperationID, profile, request, request.PayloadSHA256, state, reason, nextAt, expiresAt)
	return id, err
}

func (r *MessageDeliveryRepo) Claim(ctx context.Context, tx pgx.Tx, profile string) (*MessageDeliveryClaim, error) {
	// Close overdue reconciliation without recording an unperformed provider
	// attempt. An established acceptance remains available for manual inquiry.
	if _, err := tx.Exec(ctx, `WITH expired AS (
		SELECT id FROM notification_deliveries WHERE profile_ref=$1
		AND state IN ('sending','accepted','unknown')
		AND (lease_until<=clock_timestamp() OR next_at<=clock_timestamp())
		AND (expires_at<=clock_timestamp() OR attempts>=20)
		ORDER BY id LIMIT 50 FOR UPDATE SKIP LOCKED
	), stopped AS (
		UPDATE notification_deliveries d SET state=CASE state WHEN 'sending' THEN 'unknown' ELSE state END,
			revision=revision+1,reason='reconciliation_limit',next_at=NULL,lease_until=NULL,updated_at=clock_timestamp()
		FROM expired WHERE d.id=expired.id RETURNING d.id,d.attempts
	)
	UPDATE notification_delivery_attempts a SET finished_at=clock_timestamp(),result='interrupted'
	FROM stopped s WHERE a.delivery_id=s.id AND a.attempt=s.attempts AND a.finished_at IS NULL`, profile); err != nil {
		return nil, err
	}
	var claim MessageDeliveryClaim
	err := tx.QueryRow(ctx, `WITH candidate AS (
		SELECT id, state FROM notification_deliveries
		WHERE profile_ref=$1 AND (next_at IS NOT NULL OR lease_until IS NOT NULL) AND (
			(state='pending' AND next_at<=clock_timestamp())
			OR (state IN ('sending','accepted','unknown') AND expires_at>clock_timestamp() AND attempts<20 AND
				(lease_until<=clock_timestamp() OR (lease_until IS NULL AND next_at<=clock_timestamp()))))
		ORDER BY COALESCE(lease_until,next_at),id LIMIT 1 FOR UPDATE SKIP LOCKED
	)
	UPDATE notification_deliveries d SET
		state=CASE c.state WHEN 'pending' THEN 'sending' WHEN 'sending' THEN 'unknown' ELSE c.state END,
		revision=d.revision+1, attempts=d.attempts+1,
		next_at=NULL, lease_until=clock_timestamp()+interval '120 seconds', updated_at=clock_timestamp()
	FROM candidate c WHERE d.id=c.id
	RETURNING d.ticket_id,d.ticket_revision,d.recipient_user_id,d.operation_id,d.payload,d.revision,
		d.attempts,d.sends,d.expires_at,d.provider_receipt_ref,CASE c.state WHEN 'pending' THEN 'send' ELSE 'query' END`,
		profile).Scan(&claim.TicketID, &claim.TicketRevision, &claim.RecipientID, &claim.OperationID, &claim.Request, &claim.Revision,
		&claim.Attempt, &claim.Sends, &claim.ExpiresAt, &claim.ProviderReceiptRef, &claim.Mode)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `WITH interrupted AS (
		UPDATE notification_delivery_attempts SET finished_at=clock_timestamp(),result='interrupted'
		WHERE delivery_id=$1 AND attempt=$2-1 AND finished_at IS NULL
	)
	INSERT INTO notification_delivery_attempts (delivery_id,attempt,mode) VALUES($1,$2,$3)`,
		claim.Request.DeliveryID, claim.Attempt, claim.Mode)
	return &claim, err
}

func (r *MessageDeliveryRepo) MarkSend(ctx context.Context, tx pgx.Tx, claim *MessageDeliveryClaim) error {
	_, err := tx.Exec(ctx, `UPDATE notification_deliveries SET sends=sends+1 WHERE id=$1 AND revision=$2`, claim.Request.DeliveryID, claim.Revision)
	return err
}

func (r *MessageDeliveryRepo) Finish(ctx context.Context, db IAMDBTX, claim *MessageDeliveryClaim, out MessageDeliveryOutcome) error {
	_, err := db.Exec(ctx, `WITH completed AS (
		UPDATE notification_deliveries SET
			state=CASE WHEN $7::timestamptz < provider_observed_at OR state='accepted' AND $4='unknown' THEN state ELSE $4 END,
			reason=CASE WHEN $7::timestamptz < provider_observed_at THEN reason ELSE $5 END,
			provider_receipt_ref=CASE WHEN $7::timestamptz < provider_observed_at THEN provider_receipt_ref
				ELSE COALESCE(NULLIF($6,''),provider_receipt_ref) END,
			provider_observed_at=GREATEST($7,provider_observed_at),
			next_at=CASE WHEN $7::timestamptz < provider_observed_at THEN $9::timestamptz ELSE $8::timestamptz END,
			lease_until=NULL, updated_at=clock_timestamp()
		WHERE id=$1 AND revision=$2
		RETURNING id
	)
	UPDATE notification_delivery_attempts SET finished_at=clock_timestamp(),
		result=CASE WHEN EXISTS(SELECT 1 FROM completed) THEN $4 ELSE 'superseded' END
	WHERE delivery_id=$1 AND attempt=$3`,
		claim.Request.DeliveryID, claim.Revision, claim.Attempt, out.State, out.Reason, out.ProviderReceiptRef, out.ObservedAt, out.NextAt, out.RetryIfStaleAt)
	return err
}

func (r *MessageDeliveryRepo) RecipientOptedOut(ctx context.Context, tx pgx.Tx, recipientID int64) (bool, error) {
	var optedOut bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notification_optouts
		WHERE user_id=$1 AND biz_type='ticket' AND event_action IN ('*',$2))`, recipientID, ReviewNoticeAction).Scan(&optedOut)
	return optedOut, err
}

func (r *MessageDeliveryRepo) Receipt(ctx context.Context, id string) (MessageDeliveryReceipt, error) {
	return readDeliveryReceipt(ctx, r.db, id, 0)
}

func (r *MessageDeliveryRepo) RecipientReceipt(ctx context.Context, tx pgx.Tx, id string, recipient int64) (MessageDeliveryReceipt, MessageDeliverySource, error) {
	receipt, err := readDeliveryReceipt(ctx, tx, id, recipient)
	if err != nil {
		return MessageDeliveryReceipt{}, MessageDeliverySource{}, err
	}
	var source MessageDeliverySource
	source.RecipientID = recipient
	err = tx.QueryRow(ctx, `SELECT ticket_id,ticket_revision,operation_id FROM notification_deliveries
		WHERE id=$1 AND recipient_user_id=$2`, id, recipient).Scan(&source.TicketID, &source.TicketRevision, &source.OperationID)
	return receipt, source, err
}

func readDeliveryReceipt(ctx context.Context, db IAMDBTX, id string, recipient int64) (MessageDeliveryReceipt, error) {
	var result MessageDeliveryReceipt
	err := db.QueryRow(ctx, `SELECT id,state,reason,provider_receipt_ref,updated_at FROM notification_deliveries
		WHERE id=$1 AND ($2::bigint=0 OR recipient_user_id=$2)`, id, recipient).
		Scan(&result.ID, &result.State, &result.Reason, &result.ProviderReceiptRef, &result.UpdatedAt)
	return result, err
}

var ErrInvalidProviderMessage = errors.New("invalid provider message receipt")

// ApplyProviderReceipt deduplicates the provider event and applies only
// observations bound to an actual attempt against the frozen delivery.
func (r *MessageDeliveryRepo) ApplyProviderReceipt(ctx context.Context, configuredProfile string, receipt channelsender.AsyncReceiptV1, out MessageDeliveryOutcome) error {
	if receipt.ChannelProfileRef != configuredProfile {
		return ErrInvalidProviderMessage
	}
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var hash, state string
		var sends, attempts int
		var observedAt *time.Time
		var expiresAt time.Time
		var active bool
		err := tx.QueryRow(ctx, `SELECT payload_sha256,state,sends,attempts,provider_observed_at,expires_at,
			       next_at IS NOT NULL OR lease_until IS NOT NULL
			FROM notification_deliveries WHERE id=$1 AND profile_ref=$2 FOR UPDATE`, receipt.DeliveryID, configuredProfile).
			Scan(&hash, &state, &sends, &attempts, &observedAt, &expiresAt, &active)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidProviderMessage
		}
		if err != nil {
			return err
		}
		if sends == 0 || receipt.ValidateBinding(receipt.DeliveryID, configuredProfile, hash) != nil {
			return ErrInvalidProviderMessage
		}
		tag, err := tx.Exec(ctx, `INSERT INTO notification_provider_events (profile_ref,event_id,delivery_id,receipt)
			VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, configuredProfile, receipt.ProviderEventID, receipt.DeliveryID, receipt)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			var same bool
			err := tx.QueryRow(ctx, `SELECT receipt=$3::jsonb FROM notification_provider_events WHERE profile_ref=$1 AND event_id=$2`,
				configuredProfile, receipt.ProviderEventID, receipt).Scan(&same)
			if err == nil && !same {
				return ErrInvalidProviderMessage
			}
			return err
		}
		if state == "delivered" || state == "failed" || state == "suppressed" ||
			observedAt != nil && out.ObservedAt.Before(*observedAt) || state == "accepted" && out.State == "unknown" {
			return nil
		}
		// Late evidence can update a paused receipt without restarting reconciliation.
		if active && (out.State == "accepted" || out.State == "unknown") && attempts < 20 && time.Now().Add(5*time.Second).Before(expiresAt) {
			next := time.Now().Add(5 * time.Second)
			out.NextAt = &next
		}
		_, err = tx.Exec(ctx, `UPDATE notification_deliveries SET revision=revision+1,state=$2,reason=$3,
			provider_receipt_ref=COALESCE(NULLIF($4,''),provider_receipt_ref),provider_observed_at=$5,
			next_at=$6,lease_until=NULL,updated_at=clock_timestamp() WHERE id=$1`,
			receipt.DeliveryID, out.State, out.Reason, out.ProviderReceiptRef, out.ObservedAt, out.NextAt)
		return err
	})
}
