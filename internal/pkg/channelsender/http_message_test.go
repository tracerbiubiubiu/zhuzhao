package channelsender

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
)

const publishedMessageHash = "eb9b695109c500676a35bdc06ed1b15dac24f19ae0094e1d272e0be229368342"

func publicMessageVector() SendRequestV1 {
	return SendRequestV1{
		SchemaVersion: "1", DeliveryID: "delivery-msg-demo-0001", IdempotencyKey: "idem-msg-demo-0001",
		ChannelProfileRef: "ref-http-message-v1", PayloadSHA256: publishedMessageHash,
		RecipientRef: "ref-recipient-A-v1", TemplateVersion: "ref-template-v1",
		Subject: "Synthetic message é", Body: "Synthetic é payload.\nKeep trailing space: ",
		BusinessRef:    "ref-ticket-0001",
		SourceEventRef: MessageSourceRef{Source: "ref-events-v1", EventID: "event-msg-demo-0001"},
		RuleID:         "rule-msg-demo-0001", RuleVersion: 1,
		OperationID: "op-msg-demo-000001", ExpiresAt: "2026-11-15T02:00:00Z",
	}
}

func publicMessageReceipt() SendReceiptV1 {
	return SendReceiptV1{
		SchemaVersion: "1", DeliveryID: "delivery-msg-demo-0001", ChannelProfileRef: "ref-http-message-v1",
		PayloadSHA256: publishedMessageHash, State: "accepted", ObservedAt: "2026-11-15T01:00:01Z",
		ProviderReceiptRef: "ref-provider-receipt-A", RetryClass: "none",
	}
}

func TestHTTPMessagePublishedFrozenPayload(t *testing.T) {
	request := publicMessageVector()
	got, err := request.Hash()
	if err != nil || got != publishedMessageHash {
		t.Fatalf("frozen vector got %q, err %v", got, err)
	}
	request.IdempotencyKey = "different-attempt-key"
	request.PayloadSHA256 = ""
	same, err := request.Hash()
	if err != nil || same != publishedMessageHash {
		t.Fatalf("non-projection fields changed hash to %q: %v", same, err)
	}
	request.Body = strings.TrimSpace(request.Body)
	different, err := request.Hash()
	if err != nil || different == publishedMessageHash {
		t.Fatalf("trailing space was not covered by hash: %q %v", different, err)
	}
}

func TestHTTPMessageAcceptThenDisconnectQueriesOriginal(t *testing.T) {
	request := publicMessageVector()
	var sends atomic.Int32
	var accepted atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		verifier := aksk.Verifier{Keys: map[string][]byte{"synthetic": []byte("synthetic-key")}}
		if err != nil || verifier.Verify(r, raw) != nil || r.Method != http.MethodPost || r.URL.RawQuery != "" {
			t.Error("message request method, body signature or URL invalid")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/v1/messages/send":
			var got SendRequestV1
			if err := json.Unmarshal(raw, &got); err != nil || got != request {
				t.Error("send request changed", got, err)
			}
			sends.Add(1)
			accepted.Store(true)
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
		case "/v1/messages/query":
			var query QueryRequestV1
			if err := json.Unmarshal(raw, &query); err != nil || query.DeliveryID != request.DeliveryID ||
				query.ChannelProfileRef != request.ChannelProfileRef || query.SchemaVersion != "1" {
				t.Error("query did not bind the original delivery", query, err)
			}
			if !accepted.Load() {
				t.Error("query preceded acceptance")
			}
			receipt := publicMessageReceipt()
			receipt.State = "delivered"
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(receipt)
		default:
			t.Error("unexpected provider path", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	newClient := func() *HTTPMessageClient {
		client, err := NewHTTPMessageClient(srv.URL, "synthetic", "synthetic-key", time.Second)
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	if _, err := newClient().Send(t.Context(), request); !errors.Is(err, ErrMessageOutcomeUnknown) {
		t.Fatalf("connection lost after acceptance must be ambiguous, got %v", err)
	}
	receipt, err := newClient().Query(t.Context(), QueryRequestV1{
		SchemaVersion: "1", DeliveryID: request.DeliveryID, ChannelProfileRef: request.ChannelProfileRef,
	})
	if err != nil || receipt.State != "delivered" || receipt.ValidateBinding(request.DeliveryID, request.ChannelProfileRef, request.PayloadSHA256) != nil || sends.Load() != 1 {
		t.Fatalf("reconciliation got %#v, sends %d, err %v", receipt, sends.Load(), err)
	}
}

func TestHTTPMessageReceiptRefusesInvalidProviderEvidence(t *testing.T) {
	request := publicMessageVector()
	for _, scenario := range []struct {
		name string
		edit func(string) string
	}{
		{"wrong payload", func(s string) string { return strings.Replace(s, publishedMessageHash, strings.Repeat("a", 64), 1) }},
		{"duplicate state", func(s string) string {
			return strings.Replace(s, `"state":"accepted"`, `"state":"accepted","state":"delivered"`, 1)
		}},
		{"noncanonical field name", func(s string) string { return strings.Replace(s, `"state":`, `"State":`, 1) }},
		{"delivery without evidence", func(s string) string {
			return strings.Replace(strings.Replace(s, `"state":"accepted"`, `"state":"delivered"`, 1), `"ref-provider-receipt-A"`, "null", 1)
		}},
		{"legacy state", func(s string) string { return strings.Replace(s, `"state":"accepted"`, `"state":"sent"`, 1) }},
		{"bad timestamp", func(s string) string { return strings.Replace(s, `"2026-11-15T01:00:01Z"`, `"yesterday"`, 1) }},
		{"unpaired Unicode escape", func(s string) string { return strings.Replace(s, `"none"`, `"\uD800"`, 1) }},
		{"oversized response", func(s string) string { return s + strings.Repeat(" ", 128<<10) }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			raw, err := json.Marshal(publicMessageReceipt())
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, scenario.edit(string(raw)))
			}))
			t.Cleanup(srv.Close)
			client, err := NewHTTPMessageClient(srv.URL, "synthetic", "synthetic-key", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Send(t.Context(), request); !errors.Is(err, ErrMessageOutcomeUnknown) {
				t.Fatalf("invalid evidence produced %v", err)
			}
		})
	}
}

func TestHTTPMessageOnlyRetriesAConfirmedRefusal(t *testing.T) {
	for _, explicit := range []bool{true, false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusServiceUnavailable)
			if explicit {
				rejected := publicMessageReceipt()
				rejected.State, rejected.RetryClass, rejected.ProviderReceiptRef = "rejected", "same_operation_allowed", ""
				_ = json.NewEncoder(w).Encode(rejected)
			}
		}))
		t.Cleanup(srv.Close)
		client, err := NewHTTPMessageClient(srv.URL, "synthetic", "synthetic-key", time.Second)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := client.Send(t.Context(), publicMessageVector())
		if explicit {
			var providerError *MessageHTTPError
			if !errors.As(err, &providerError) || providerError.Status != http.StatusServiceUnavailable ||
				providerError.RetryAfter != 3*time.Second || receipt.State != "rejected" {
				t.Fatalf("confirmed refusal got receipt %#v, error %v", receipt, err)
			}
		} else if !errors.Is(err, ErrMessageOutcomeUnknown) {
			t.Fatalf("generic 503 must remain ambiguous, got %v", err)
		}
	}
}

func TestHTTPMessageDoesNotFollowRedirect(t *testing.T) {
	var redirected atomic.Int32
	next := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected.Add(1)
	}))
	t.Cleanup(next.Close)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, next.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(provider.Close)
	client, err := NewHTTPMessageClient(provider.URL, "synthetic", "synthetic-key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Send(context.Background(), publicMessageVector()); !errors.Is(err, ErrMessageOutcomeUnknown) || redirected.Load() != 0 {
		t.Fatalf("redirect produced %v with %d follow-up requests", err, redirected.Load())
	}
}

func TestHTTPMessageUnsupportedQuery(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
	}))
	t.Cleanup(provider.Close)
	client, err := NewHTTPMessageClient(provider.URL, "synthetic", "synthetic-key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	request := publicMessageVector()
	_, err = client.Query(t.Context(), QueryRequestV1{
		SchemaVersion: "1", DeliveryID: request.DeliveryID, ChannelProfileRef: request.ChannelProfileRef,
	})
	if !errors.Is(err, ErrQueryUnsupported) {
		t.Fatalf("unsupported query got %v", err)
	}
}

func TestDecodeAsyncMessageReceiptRequiresProviderEvent(t *testing.T) {
	receipt := publicMessageReceipt()
	receipt.State = "delivered"
	raw, err := json.Marshal(AsyncReceiptV1{SendReceiptV1: receipt, ProviderEventID: "ref-provider-event-delivered-A"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeAsyncMessageReceipt(strings.NewReader(string(raw)))
	if err != nil || got.ProviderEventID != "ref-provider-event-delivered-A" || got.SendReceiptV1 != receipt {
		t.Fatalf("callback got %#v, error %v", got, err)
	}
	raw, err = json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeAsyncMessageReceipt(strings.NewReader(string(raw))); err == nil {
		t.Fatal("receipt with no provider event ID accepted")
	}
}

func TestHTTPMessageRequestWithChangedHashSendsNothing(t *testing.T) {
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(provider.Close)
	client, err := NewHTTPMessageClient(provider.URL, "synthetic", "synthetic-key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	request := publicMessageVector()
	request.Subject = "Changed after freeze"
	if _, err := client.Send(t.Context(), request); !errors.Is(err, ErrInvalidMessageRequest) || calls.Load() != 0 {
		t.Fatalf("changed payload produced %v and %d calls", err, calls.Load())
	}
}

func TestHTTPMessageProviderAcceptanceDoesNotImplyDelivery(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		state  string
	}{
		{name: "accepted", status: http.StatusAccepted, state: "accepted"},
		{name: "delivered", status: http.StatusOK, state: "delivered"},
		{name: "rejected", status: http.StatusOK, state: "rejected"},
		{name: "unknown", status: http.StatusOK, state: "unknown"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				receipt := publicMessageReceipt()
				receipt.State = scenario.state
				if scenario.state == "unknown" {
					receipt.RetryClass, receipt.ProviderReceiptRef = "reconcile_only", ""
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(scenario.status)
				_ = json.NewEncoder(w).Encode(receipt)
			}))
			t.Cleanup(provider.Close)
			client, err := NewHTTPMessageClient(provider.URL, "synthetic", "synthetic-key", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := client.Send(t.Context(), publicMessageVector())
			if err != nil || receipt.State != scenario.state {
				t.Fatalf("typed %s produced %#v, error %v", scenario.state, receipt, err)
			}
		})
	}
}

func TestHTTPMessageClientRequiresFixedOrigin(t *testing.T) {
	for _, origin := range []string{
		"http://example.invalid",
		"https://example.invalid/path",
		"https://example.invalid?target=self",
		"https://user@example.invalid",
	} {
		if _, err := NewHTTPMessageClient(origin, "synthetic", "synthetic-key", time.Second); err == nil {
			t.Fatalf("accepted invalid origin %q", origin)
		}
	}
}

func TestHTTPMessageQueryFailureDoesNotBecomeDeliveryRefusal(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		receipt := publicMessageReceipt()
		receipt.State, receipt.RetryClass, receipt.ProviderReceiptRef = "rejected", "same_operation_allowed", ""
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(receipt)
	}))
	t.Cleanup(provider.Close)
	client, err := NewHTTPMessageClient(provider.URL, "synthetic", "synthetic-key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	request := publicMessageVector()
	receipt, err := client.Query(t.Context(), QueryRequestV1{
		SchemaVersion: "1", DeliveryID: request.DeliveryID, ChannelProfileRef: request.ChannelProfileRef,
	})
	if !errors.Is(err, ErrMessageOutcomeUnknown) || receipt != (SendReceiptV1{}) {
		t.Fatalf("failed query incorrectly reported a delivery result: %#v %v", receipt, err)
	}
}
