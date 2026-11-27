//go:build integration

package service_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/channelsender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

const (
	messageHTTPFixtureAK    = "synthetic-review-client"
	messageHTTPFixtureSK    = "synthetic-review-fixture-key"
	messageHTTPFixtureProof = "synthetic-provider-receipt-original"
	messageHTTPWorkerChild  = "ZZ_TEST_MESSAGE_HTTP_WORKER_CHILD"
)

type messageHTTPFixture struct {
	mu           sync.Mutex
	reject       bool
	queryResults []string
	effects      map[[2]string]channelsender.SendRequestV1
	sends        []channelsender.SendRequestV1
	queries      []channelsender.QueryRequestV1
	accepted     chan<- channelsender.SendRequestV1
}

func (f *messageHTTPFixture) serve(t *testing.T, w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	verifier := aksk.Verifier{Keys: map[string][]byte{messageHTTPFixtureAK: []byte(messageHTTPFixtureSK)}}
	if err != nil || r.Method != http.MethodPost || r.URL.RawQuery != "" || verifier.Verify(r, raw) != nil {
		t.Error("provider received a request without the expected signed POST body")
		http.Error(w, "invalid test request", http.StatusForbidden)
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/v1/messages/send":
		var request channelsender.SendRequestV1
		if err := json.Unmarshal(raw, &request); err != nil || request.SchemaVersion != "1" || request.IdempotencyKey == "" ||
			request.PayloadSHA256 != messageHTTPFixtureHash(request) {
			t.Error("provider received an invalid send envelope or frozen payload hash")
			http.Error(w, "invalid test payload", http.StatusBadRequest)
			return
		}
		f.sends = append(f.sends, request)
		if f.reject {
			receipt := messageHTTPFixtureReceipt(request, "rejected")
			receipt.ProviderReceiptRef = ""
			receipt.SafeReasonCode = "recipient_rejected"
			writeMessageHTTPFixtureReceipt(t, w, http.StatusUnprocessableEntity, receipt)
			return
		}
		key := [2]string{request.ChannelProfileRef, request.IdempotencyKey}
		if prior, exists := f.effects[key]; exists {
			if prior.PayloadSHA256 != request.PayloadSHA256 {
				t.Error("provider received different content under the same idempotency key")
				http.Error(w, "key conflict", http.StatusConflict)
				return
			}
			writeMessageHTTPFixtureReceipt(t, w, http.StatusAccepted, messageHTTPFixtureReceipt(prior, "accepted"))
			return
		}
		f.effects[key] = request
		if f.accepted != nil {
			// Hold the response so the parent can kill the worker after the
			// provider effect and before any local result is recorded.
			f.accepted <- request
			<-r.Context().Done()
			return
		}
		// Acceptance is recorded before the transport drops the response.
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("fixture cannot drop the accepted HTTP connection")
			http.Error(w, "hijacking unavailable", http.StatusInternalServerError)
			return
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("fixture could not drop connection: %v", err)
			return
		}
		_ = conn.Close()
	case "/v1/messages/query":
		var query channelsender.QueryRequestV1
		if err := json.Unmarshal(raw, &query); err != nil || query.SchemaVersion != "1" {
			t.Error("provider received an invalid query envelope")
			http.Error(w, "invalid test query", http.StatusBadRequest)
			return
		}
		f.queries = append(f.queries, query)
		for _, request := range f.effects {
			if query.DeliveryID != request.DeliveryID || query.ChannelProfileRef != request.ChannelProfileRef ||
				(query.ProviderReceiptRef != "" && query.ProviderReceiptRef != messageHTTPFixtureProof) {
				continue
			}
			result := "delivered"
			if index := len(f.queries) - 1; index < len(f.queryResults) {
				result = f.queryResults[index]
			}
			switch result {
			case "unsupported":
				w.WriteHeader(http.StatusNotImplemented)
			case "wrong_hash":
				receipt := messageHTTPFixtureReceipt(request, "delivered")
				receipt.PayloadSHA256 = strings.Repeat("0", 64)
				writeMessageHTTPFixtureReceipt(t, w, http.StatusOK, receipt)
			case "stale_rejection":
				receipt := messageHTTPFixtureReceipt(request, "rejected")
				receipt.ObservedAt = time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
				receipt.ProviderReceiptRef = ""
				receipt.SafeReasonCode = "recipient_rejected"
				writeMessageHTTPFixtureReceipt(t, w, http.StatusOK, receipt)
			default:
				writeMessageHTTPFixtureReceipt(t, w, http.StatusOK, messageHTTPFixtureReceipt(request, result))
			}
			return
		}
		t.Error("query did not identify the provider's original accepted delivery")
		http.NotFound(w, r)
	default:
		t.Errorf("unexpected provider route %s", r.URL.Path)
		http.NotFound(w, r)
	}
}

type messageHTTPWorkerInputs struct {
	DSN, Profile, Origin string
}

func init() {
	// The child shares the parent's real PG fixture. Exit before the ordinary
	// TestMain can create another container, apply migrations, or run tests.
	if os.Getenv(messageHTTPWorkerChild) != "one-attempt" {
		return
	}
	os.Exit(runMessageHTTPWorkerChild())
}

func runMessageHTTPWorkerChild() int {
	var input messageHTTPWorkerInputs
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Fprintln(os.Stderr, "decode isolated worker fixture input")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, input.DSN)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open isolated worker PostgreSQL")
		return 1
	}
	defer pool.Close()
	client, err := channelsender.NewHTTPMessageClient(input.Origin, messageHTTPFixtureAK, messageHTTPFixtureSK, 12*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open isolated worker HTTP channel")
		return 1
	}
	check := func(context.Context, pgx.Tx, repository.MessageDeliverySource) (bool, string, error) {
		return true, "", nil
	}
	if err := service.NewMessageDeliveryWorker(pool, input.Profile, client, check).Handle(ctx, nil); err != nil {
		fmt.Fprintln(os.Stderr, "run isolated message delivery attempt:", err)
		return 1
	}
	return 0
}

func isolatedMessageHTTPWorker(t *testing.T, ctx context.Context, profile, origin string) *exec.Cmd {
	t.Helper()
	input, err := json.Marshal(messageHTTPWorkerInputs{DSN: testPool.Config().ConnString(), Profile: profile, Origin: origin})
	require.NoError(t, err)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), messageHTTPWorkerChild+"=one-attempt")
	cmd.Stdin = bytes.NewReader(input)
	return cmd
}

func TestMessageHTTPDeliveryRecoversAfterWorkerProcessCrash(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	accepted := make(chan channelsender.SendRequestV1, 1)
	fixture := messageHTTPFixture{
		effects: make(map[[2]string]channelsender.SendRequestV1), accepted: accepted,
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.serve(t, w, r)
	}))
	t.Cleanup(provider.Close)

	ticketID := rand.Int64N(1<<46) + 1
	source := repository.MessageDeliverySource{
		TicketID: ticketID, TicketRevision: 3, RecipientID: 152,
		OperationID: fmt.Sprintf("op-review-crash-%d", ticketID),
	}
	profile := fmt.Sprintf("review-http-crash-v1-%d", ticketID)
	store := repository.NewMessageDeliveryRepo(testPool)
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())
	id, err := store.Record(ctx, tx, source, profile, "Review requested", "Open the workbench", "review-template:2", "")
	require.NoError(t, err)
	_, err = tx.Exec(ctx, "UPDATE notification_deliveries SET next_at=clock_timestamp() WHERE id=$1", id)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	t.Cleanup(func() {
		_, err := testPool.Exec(context.Background(), "DELETE FROM notification_deliveries WHERE id=$1", id)
		require.NoError(t, err)
	})

	first := isolatedMessageHTTPWorker(t, ctx, profile, provider.URL)
	var firstOutput bytes.Buffer
	first.Stdout, first.Stderr = &firstOutput, &firstOutput
	require.NoError(t, first.Start())
	t.Cleanup(func() {
		if first.ProcessState == nil {
			_ = first.Process.Kill()
			_ = first.Wait()
			t.Log(firstOutput.String())
		}
	})
	var sent channelsender.SendRequestV1
	select {
	case sent = <-accepted:
	case <-time.After(8 * time.Second):
		t.Fatal("first worker did not reach provider acceptance")
	}
	require.Equal(t, id, sent.DeliveryID)
	var frozenHash string
	var sending bool
	require.NoError(t, testPool.QueryRow(ctx, `SELECT payload_sha256, state='sending' AND lease_until > clock_timestamp()
		FROM notification_deliveries WHERE id=$1`, id).Scan(&frozenHash, &sending))
	require.True(t, sending, "the provider has accepted while the original lease is active")
	require.Equal(t, frozenHash, sent.PayloadSHA256)
	require.NoError(t, first.Process.Kill())
	require.Error(t, first.Wait(), "first worker must terminate without committing any result")

	// Advance only the failed process's real lease to exercise restart
	// recovery without a two-minute sleep or rewriting the sending state.
	tag, err := testPool.Exec(ctx, `UPDATE notification_deliveries SET lease_until=clock_timestamp()-interval '1 sec'
		WHERE id=$1 AND state='sending' AND lease_until IS NOT NULL`, id)
	require.NoError(t, err)
	require.EqualValues(t, 1, tag.RowsAffected())
	replacement := isolatedMessageHTTPWorker(t, ctx, profile, provider.URL)
	output, err := replacement.CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.NotEqual(t, first.Process.Pid, replacement.Process.Pid)

	receipt, err := store.Receipt(ctx, id)
	require.NoError(t, err)
	require.Equal(t, id, receipt.ID)
	require.Equal(t, "delivered", receipt.State)
	require.Equal(t, messageHTTPFixtureProof, receipt.ProviderReceiptRef)
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	require.Len(t, fixture.sends, 1)
	require.Len(t, fixture.queries, 1)
	require.Equal(t, id, fixture.queries[0].DeliveryID)
	require.Equal(t, profile, fixture.queries[0].ChannelProfileRef)
	require.Empty(t, fixture.queries[0].ProviderReceiptRef)
	require.Len(t, fixture.effects, 1)
	require.Equal(t, frozenHash, fixture.effects[[2]string{profile, sent.IdempotencyKey}].PayloadSHA256)
}

func messageHTTPFixtureHash(request channelsender.SendRequestV1) string {
	// The fixture uses ASCII text and Go's sorted map encoding. Compute the
	// provider hash independently from the client's request validation.
	canonical, _ := json.Marshal(map[string]any{
		"business_ref": request.BusinessRef, "body": request.Body,
		"channel_profile_ref": request.ChannelProfileRef, "delivery_id": request.DeliveryID,
		"expires_at": request.ExpiresAt, "operation_id": request.OperationID, "recipient_ref": request.RecipientRef,
		"rule_id": request.RuleID, "rule_version": request.RuleVersion,
		"source_event_ref": map[string]string{"event_id": request.SourceEventRef.EventID, "source": request.SourceEventRef.Source},
		"subject":          request.Subject, "template_version": request.TemplateVersion,
	})
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:])
}

func messageHTTPFixtureReceipt(request channelsender.SendRequestV1, state string) channelsender.SendReceiptV1 {
	return channelsender.SendReceiptV1{
		SchemaVersion: "1", DeliveryID: request.DeliveryID, ChannelProfileRef: request.ChannelProfileRef,
		PayloadSHA256: request.PayloadSHA256, State: state, ProviderReceiptRef: messageHTTPFixtureProof,
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), RetryClass: "none",
	}
}

func writeMessageHTTPFixtureReceipt(t *testing.T, w http.ResponseWriter, status int, receipt channelsender.SendReceiptV1) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(receipt); err != nil {
		t.Errorf("fixture could not write receipt: %v", err)
	}
}

func TestMessageHTTPDeliveryRecoversOriginalProviderEffect(t *testing.T) {
	for _, tc := range []struct {
		name         string
		reject       bool
		queryResults []string
		states       []string
		reasons      []string
		proof        []string
	}{
		{
			name: "disconnect_then_query", queryResults: []string{"wrong_hash", "accepted", "stale_rejection", "delivered"},
			states:  []string{"unknown", "unknown", "accepted", "accepted", "delivered"},
			reasons: []string{"acceptance_unconfirmed", "acceptance_unconfirmed", "", "", ""},
			proof:   []string{"", "", messageHTTPFixtureProof, messageHTTPFixtureProof, messageHTTPFixtureProof},
		},
		{
			name: "query_unsupported", queryResults: []string{"unsupported"},
			states: []string{"unknown", "unknown"}, reasons: []string{"acceptance_unconfirmed", "query_unsupported"}, proof: []string{"", ""},
		},
		{
			name: "explicit_rejection", reject: true,
			states: []string{"failed"}, reasons: []string{"recipient_rejected"}, proof: []string{""},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := messageHTTPFixture{
				reject: tc.reject, queryResults: tc.queryResults, effects: make(map[[2]string]channelsender.SendRequestV1),
			}
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fixture.serve(t, w, r)
			}))
			t.Cleanup(provider.Close)

			ticketID := rand.Int64N(1<<46) + 1
			source := repository.MessageDeliverySource{
				TicketID: ticketID, TicketRevision: 7, RecipientID: 152,
				OperationID: fmt.Sprintf("op-review-http-%d", ticketID),
			}
			profile := fmt.Sprintf("review-http-v1-%d", ticketID)
			const subject, body, template = "A ticket needs review", "Visit the workbench.\nKeep this trailing space: ", "review-template:1"
			ctx := t.Context()
			store := repository.NewMessageDeliveryRepo(testPool)
			tx, err := testPool.Begin(ctx)
			require.NoError(t, err)
			defer tx.Rollback(context.Background())
			id, err := store.Record(ctx, tx, source, profile, subject, body, template, "")
			require.NoError(t, err)
			require.NoError(t, tx.Commit(ctx))
			t.Cleanup(func() {
				_, err := testPool.Exec(context.Background(), "DELETE FROM notification_deliveries WHERE id=$1", id)
				require.NoError(t, err)
			})

			allowed := func(_ context.Context, _ pgx.Tx, got repository.MessageDeliverySource) (bool, string, error) {
				require.Equal(t, source, got)
				return true, "", nil
			}
			runFreshWorker := func() {
				t.Helper()
				client, err := channelsender.NewHTTPMessageClient(provider.URL, messageHTTPFixtureAK, messageHTTPFixtureSK, 3*time.Second)
				require.NoError(t, err)
				require.NoError(t, service.NewMessageDeliveryWorker(testPool, profile, client, allowed).Handle(ctx, nil))
			}
			for i, wantState := range tc.states {
				// Make the scheduled attempt due on the database clock, without
				// changing delivery identity, content, or any recorded outcome.
				tag, err := testPool.Exec(ctx, "UPDATE notification_deliveries SET next_at=clock_timestamp() WHERE id=$1 AND next_at IS NOT NULL", id)
				require.NoError(t, err)
				require.EqualValues(t, 1, tag.RowsAffected())
				runFreshWorker()
				receipt, err := store.Receipt(ctx, id)
				require.NoError(t, err)
				require.Equal(t, id, receipt.ID)
				require.Equal(t, wantState, receipt.State, "after attempt %d", i+1)
				require.Equal(t, tc.reasons[i], receipt.Reason)
				require.Equal(t, tc.proof[i], receipt.ProviderReceiptRef)
			}
			var idle bool
			require.NoError(t, testPool.QueryRow(ctx, "SELECT next_at IS NULL AND lease_until IS NULL FROM notification_deliveries WHERE id=$1", id).Scan(&idle))
			require.True(t, idle)
			runFreshWorker()

			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			require.Len(t, fixture.sends, 1, "recovery must never replay the send")
			sent := fixture.sends[0]
			require.Equal(t, id, sent.DeliveryID)
			require.Equal(t, id, sent.IdempotencyKey)
			require.Equal(t, profile, sent.ChannelProfileRef)
			operationRef := sha256.Sum256([]byte(source.OperationID))
			require.Equal(t, "op_"+hex.EncodeToString(operationRef[:]), sent.OperationID)
			require.Equal(t, fmt.Sprintf("risk-review-ticket-%d:7", ticketID), sent.SourceEventRef.EventID)
			require.Equal(t, subject, sent.Subject)
			require.Equal(t, body, sent.Body)
			require.Equal(t, template, sent.TemplateVersion)
			var frozenHash string
			require.NoError(t, testPool.QueryRow(ctx, "SELECT payload_sha256 FROM notification_deliveries WHERE id=$1", id).Scan(&frozenHash))
			require.Equal(t, frozenHash, sent.PayloadSHA256)
			require.Len(t, fixture.queries, len(tc.queryResults))
			for i, query := range fixture.queries {
				require.Equal(t, id, query.DeliveryID)
				require.Equal(t, profile, query.ChannelProfileRef)
				require.Equal(t, tc.proof[i], query.ProviderReceiptRef)
			}
			if tc.reject {
				require.Empty(t, fixture.effects)
			} else {
				require.Len(t, fixture.effects, 1)
				require.Equal(t, sent, fixture.effects[[2]string{profile, id}])
			}
		})
	}
}
