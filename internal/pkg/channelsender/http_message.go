package channelsender

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3protocol"
)

const (
	sendMessagePath     = "/v1/messages/send"
	queryMessagePath    = "/v1/messages/query"
	maxMessageBodyBytes = 64 << 10
	maxReceiptBodyBytes = 8 << 10
	maxRetryAfter       = 5 * time.Minute
)

var (
	ErrQueryUnsupported      = errors.New("message channel does not support receipt query")
	ErrMessageOutcomeUnknown = errors.New("message channel outcome unknown")
	ErrInvalidMessageRequest = errors.New("invalid message request")
	ErrInvalidMessageReceipt = errors.New("invalid message receipt")

	messageHashPattern = regexp.MustCompile("^[a-f0-9]{64}$")
	reasonCodePattern  = regexp.MustCompile("^[a-z][a-z0-9_]{0,63}$")
)

type MessageSourceRef struct {
	Source  string `json:"source"`
	EventID string `json:"event_id"`
}

type SendRequestV1 struct {
	SchemaVersion     string           `json:"schema_version"`
	DeliveryID        string           `json:"delivery_id"`
	IdempotencyKey    string           `json:"idempotency_key"`
	ChannelProfileRef string           `json:"channel_profile_ref"`
	PayloadSHA256     string           `json:"payload_sha256"`
	RecipientRef      string           `json:"recipient_ref"`
	TemplateVersion   string           `json:"template_version"`
	Subject           string           `json:"subject"`
	Body              string           `json:"body"`
	BusinessRef       string           `json:"business_ref"`
	SourceEventRef    MessageSourceRef `json:"source_event_ref"`
	RuleID            string           `json:"rule_id"`
	RuleVersion       int64            `json:"rule_version"`
	OperationID       string           `json:"operation_id"`
	ExpiresAt         string           `json:"expires_at"`
}

// Hash covers the exact frozen semantic projection, excluding the request
// envelope, idempotency key, and transport metadata.
func (r SendRequestV1) Hash() (string, error) {
	if !b3protocol.ValidID(r.DeliveryID) || !b3protocol.ValidID(r.OperationID) ||
		!b3protocol.ValidID(r.SourceEventRef.EventID) || !validMessageRef(r.SourceEventRef.Source, 256) ||
		!validMessageRef(r.ChannelProfileRef, 128) || !validMessageRef(r.RecipientRef, 128) ||
		!validMessageRef(r.TemplateVersion, 128) || !validMessageRef(r.BusinessRef, 128) ||
		!validMessageRef(r.RuleID, 128) || r.RuleVersion < 1 || r.RuleVersion > 1<<53-1 ||
		!utf8.ValidString(r.Subject) || !utf8.ValidString(r.Body) ||
		len(r.Subject)+len(r.Body) > maxMessageBodyBytes || !validMessageTimestamp(r.ExpiresAt) {
		return "", ErrInvalidMessageRequest
	}
	projection, err := json.Marshal(map[string]any{
		"delivery_id": r.DeliveryID, "channel_profile_ref": r.ChannelProfileRef, "recipient_ref": r.RecipientRef,
		"template_version": r.TemplateVersion, "subject": r.Subject, "body": r.Body,
		"business_ref": r.BusinessRef, "source_event_ref": r.SourceEventRef, "rule_id": r.RuleID,
		"rule_version": r.RuleVersion, "operation_id": r.OperationID, "expires_at": r.ExpiresAt,
	})
	if err != nil {
		return "", ErrInvalidMessageRequest
	}
	// This shared encoder hashes sorted ASCII-key JSON and preserves exact
	// UTF-8 string values, including the Unicode line separators.
	return b3protocol.SourceHash(projection)
}

type QueryRequestV1 struct {
	SchemaVersion      string `json:"schema_version"`
	DeliveryID         string `json:"delivery_id"`
	ChannelProfileRef  string `json:"channel_profile_ref"`
	ProviderReceiptRef string `json:"provider_receipt_ref,omitempty"`
}

type SendReceiptV1 struct {
	SchemaVersion      string `json:"schema_version"`
	DeliveryID         string `json:"delivery_id"`
	ChannelProfileRef  string `json:"channel_profile_ref"`
	PayloadSHA256      string `json:"payload_sha256"`
	State              string `json:"state"`
	ProviderReceiptRef string `json:"provider_receipt_ref,omitempty"`
	ObservedAt         string `json:"observed_at"`
	RetryClass         string `json:"retry_class"`
	SafeReasonCode     string `json:"safe_reason_code,omitempty"`
}

func (r SendReceiptV1) validate() error {
	if r.SchemaVersion != "1" || !b3protocol.ValidID(r.DeliveryID) ||
		!validMessageRef(r.ChannelProfileRef, 128) || !messageHashPattern.MatchString(r.PayloadSHA256) ||
		!validMessageTimestamp(r.ObservedAt) ||
		(r.ProviderReceiptRef != "" && !validMessageRef(r.ProviderReceiptRef, 128)) ||
		(r.SafeReasonCode != "" && !reasonCodePattern.MatchString(r.SafeReasonCode)) {
		return ErrInvalidMessageReceipt
	}
	switch r.State {
	case "accepted", "delivered":
		if r.RetryClass == "none" && (r.State != "delivered" || r.ProviderReceiptRef != "") {
			return nil
		}
	case "rejected":
		if r.RetryClass == "none" || r.RetryClass == "same_operation_allowed" {
			return nil
		}
	case "unknown":
		if r.RetryClass == "reconcile_only" || r.RetryClass == "same_operation_allowed" {
			return nil
		}
	}
	return ErrInvalidMessageReceipt
}

func (r SendReceiptV1) ValidateBinding(deliveryID, profileRef, hash string) error {
	if r.DeliveryID != deliveryID || r.ChannelProfileRef != profileRef || r.PayloadSHA256 != hash {
		return ErrInvalidMessageReceipt
	}
	return r.validate()
}

type AsyncReceiptV1 struct {
	SendReceiptV1
	ProviderEventID string `json:"provider_event_id"`
}

// DecodeAsyncMessageReceipt validates the bounded public wire shape. The
// receiver must authenticate the provider and check the frozen local binding.
func DecodeAsyncMessageReceipt(reader io.Reader) (AsyncReceiptV1, error) {
	var receipt AsyncReceiptV1
	if err := decodeMessageReceipt(reader, &receipt, true); err != nil {
		return AsyncReceiptV1{}, err
	}
	if !validMessageRef(receipt.ProviderEventID, 128) || receipt.validate() != nil {
		return AsyncReceiptV1{}, ErrInvalidMessageReceipt
	}
	return receipt, nil
}

func decodeMessageReceipt(reader io.Reader, out any, async bool) error {
	raw, err := io.ReadAll(io.LimitReader(reader, maxReceiptBodyBytes+1))
	if err != nil || len(raw) > maxReceiptBodyBytes {
		return ErrInvalidMessageReceipt
	}
	var fields map[string]json.RawMessage
	if err := b3protocol.DecodeBytes(raw, &fields); err != nil {
		return ErrInvalidMessageReceipt
	}
	for name, value := range fields {
		switch name {
		case "schema_version", "delivery_id", "channel_profile_ref", "payload_sha256", "state",
			"observed_at", "retry_class", "provider_receipt_ref", "safe_reason_code":
		case "provider_event_id":
			if !async {
				return ErrInvalidMessageReceipt
			}
		default:
			return ErrInvalidMessageReceipt
		}
		// Optional wire fields are omitted when absent. Explicit null and
		// empty strings would otherwise decode to the same Go zero value.
		var text string
		if err := json.Unmarshal(value, &text); err != nil || text == "" {
			return ErrInvalidMessageReceipt
		}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return ErrInvalidMessageReceipt
	}
	return nil
}

func validMessageRef(ref string, max int) bool {
	return utf8.ValidString(ref) && strings.TrimSpace(ref) != "" && utf8.RuneCountInString(ref) <= max
}

func validMessageTimestamp(ts string) bool {
	_, err := time.Parse(time.RFC3339Nano, ts)
	return err == nil
}

type HTTPMessagePort interface {
	Send(context.Context, SendRequestV1) (SendReceiptV1, error)
	Query(context.Context, QueryRequestV1) (SendReceiptV1, error)
}

type HTTPMessageClient struct {
	origin string
	client *http.Client
}

func NewHTTPMessageClient(origin, ak, sk string, timeout time.Duration) (*HTTPMessageClient, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || u.User != nil || u.ForceQuery || u.RawQuery != "" ||
		u.Fragment != "" || u.RawFragment != "" || u.RawPath != "" ||
		(u.Path != "" && u.Path != "/") || strings.Contains(origin, "#") ||
		(u.Scheme != "https" && !(u.Scheme == "http" && net.ParseIP(u.Hostname()).IsLoopback())) ||
		strings.TrimSpace(ak) == "" || strings.ContainsAny(ak, "\r\n") || sk == "" || timeout <= 0 {
		return nil, errors.New("invalid HTTP message channel configuration")
	}
	u.Path = ""
	transport := &http.Transport{
		ForceAttemptHTTP2:      true,
		IdleConnTimeout:        90 * time.Second,
		MaxResponseHeaderBytes: 16 << 10,
	}
	return &HTTPMessageClient{origin: u.String(), client: &http.Client{
		Timeout:       timeout,
		Transport:     &aksk.Transport{Base: transport, AK: ak, SK: []byte(sk)},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (c *HTTPMessageClient) Send(ctx context.Context, request SendRequestV1) (SendReceiptV1, error) {
	hash, err := request.Hash()
	if err != nil || request.SchemaVersion != "1" || hash != request.PayloadSHA256 || !validMessageRef(request.IdempotencyKey, 128) {
		return SendReceiptV1{}, ErrInvalidMessageRequest
	}
	return c.post(ctx, sendMessagePath, request, request.DeliveryID, request.ChannelProfileRef, hash)
}

// Query checks the receipt shape and signed query identifiers. The caller
// verifies the returned hash against its frozen local delivery before storing it.
func (c *HTTPMessageClient) Query(ctx context.Context, request QueryRequestV1) (SendReceiptV1, error) {
	if request.SchemaVersion != "1" || !b3protocol.ValidID(request.DeliveryID) ||
		!validMessageRef(request.ChannelProfileRef, 128) ||
		(request.ProviderReceiptRef != "" && !validMessageRef(request.ProviderReceiptRef, 128)) {
		return SendReceiptV1{}, ErrInvalidMessageRequest
	}
	return c.post(ctx, queryMessagePath, request, request.DeliveryID, request.ChannelProfileRef, "")
}

// MessageHTTPError accompanies a matching provider receipt confirming the
// request was rejected. Retryable reports an allowed, same-operation retry.
type MessageHTTPError struct {
	Status     int
	RetryAfter time.Duration
	Retryable  bool
}

func (*MessageHTTPError) Error() string { return "message channel rejected the request" }

func (c *HTTPMessageClient) post(ctx context.Context, path string, payload any, deliveryID, profileRef, hash string) (SendReceiptV1, error) {
	data, err := json.Marshal(payload)
	if err != nil || len(data) > maxMessageBodyBytes {
		return SendReceiptV1{}, ErrInvalidMessageRequest
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+path, bytes.NewReader(data))
	if err != nil {
		return SendReceiptV1{}, ErrInvalidMessageRequest
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return SendReceiptV1{}, ErrMessageOutcomeUnknown
	}
	defer response.Body.Close()
	if path == queryMessagePath && (response.StatusCode == http.StatusMethodNotAllowed || response.StatusCode == http.StatusNotImplemented) {
		return SendReceiptV1{}, ErrQueryUnsupported
	}

	var receipt SendReceiptV1
	if err := decodeMessageReceipt(response.Body, &receipt, false); err != nil ||
		receipt.validate() != nil || receipt.DeliveryID != deliveryID || receipt.ChannelProfileRef != profileRef ||
		(hash != "" && receipt.PayloadSHA256 != hash) {
		return SendReceiptV1{}, ErrMessageOutcomeUnknown
	}
	switch response.StatusCode {
	case http.StatusOK:
		return receipt, nil
	case http.StatusAccepted:
		if receipt.State == "accepted" {
			return receipt, nil
		}
	default:
		if path == sendMessagePath && receipt.State == "rejected" && (response.StatusCode >= 400 && response.StatusCode < 500 ||
			response.StatusCode == http.StatusServiceUnavailable) {
			retry := receipt.RetryClass == "same_operation_allowed" &&
				(response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusServiceUnavailable)
			result := &MessageHTTPError{Status: response.StatusCode, Retryable: retry}
			if retry {
				result.RetryAfter = retryAfter(response.Header.Get("Retry-After"), time.Now())
			}
			return receipt, result
		}
	}
	return SendReceiptV1{}, ErrMessageOutcomeUnknown
}

func retryAfter(value string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
		return min(time.Duration(seconds)*time.Second, maxRetryAfter)
	}
	if resume, err := http.ParseTime(value); err == nil {
		return max(0, min(resume.Sub(now), maxRetryAfter))
	}
	return 0
}
