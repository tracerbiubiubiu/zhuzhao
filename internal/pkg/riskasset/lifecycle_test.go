package riskasset

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
)

func TestLookupBadDedicatedCredentialRejected(t *testing.T) {
	var calls atomic.Int32
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		v := aksk.Verifier{Keys: map[string][]byte{fixtureAK: []byte(fixtureSK)}, Now: fixedNow}
		if v.Verify(r, body) == nil {
			t.Error("wrong credential accepted")
		}
		w.WriteHeader(401)
	}, func(cfg *Config) { cfg.SK = []byte("synthetic-wrong-secret") })
	got, err := c.Lookup(context.Background(), LookupRequest{fixtureEvidence("asset-A").AssetRef, "synthetic-bad-signature"})
	if got.Status != StatusUnavailable || !errors.Is(err, ErrUnavailable) || calls.Load() != 1 {
		t.Fatalf("got %+v, %v, calls=%d", got, err, calls.Load())
	}
}

func TestLookupTimeoutAndCancelReachServer(t *testing.T) {
	for _, kind := range []string{"timeout", "caller_deadline", "cancel", "body_timeout", "body_cancel"} {
		t.Run(kind, func(t *testing.T) {
			started := make(chan struct{})
			stopped := make(chan struct{})
			release := make(chan struct{})
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				// Drain the finite POST body so net/http can detect connection closure.
				_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 4096))
				if kind == "body_timeout" || kind == "body_cancel" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"status":`))
					w.(http.Flusher).Flush()
				}
				close(started)
				select {
				case <-r.Context().Done():
					close(stopped)
				case <-release:
				}
			}, func(cfg *Config) {
				if kind == "timeout" || kind == "body_timeout" {
					cfg.Timeout = 100 * time.Millisecond
				}
			})
			t.Cleanup(func() { close(release) })
			ctx, cancel := context.WithCancel(context.Background())
			if kind == "caller_deadline" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			}
			defer cancel()
			type outcome struct {
				result LookupResult
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				r, e := c.Lookup(ctx, LookupRequest{fixtureEvidence("asset-A").AssetRef, "synthetic-context"})
				done <- outcome{r, e}
			}()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("request never reached fixture")
			}
			if kind == "cancel" || kind == "body_cancel" {
				cancel()
			}
			var out outcome
			select {
			case out = <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("client ignored context")
			}
			status, wantErr := StatusTimeout, context.DeadlineExceeded
			if kind == "cancel" || kind == "body_cancel" {
				status = StatusCanceled
				wantErr = context.Canceled
			}
			if out.result.Status != status || out.result.Evidence != nil || !errors.Is(out.err, wantErr) {
				t.Fatalf("got %+v,%v", out.result, out.err)
			}
			select {
			case <-stopped:
			case <-time.After(2 * time.Second):
				t.Fatal("fixture did not observe cancellation")
			}
		})
	}
}

func TestLookupPreCanceledAndInvalidInputNeverSend(t *testing.T) {
	var calls atomic.Int32
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := c.Lookup(ctx, LookupRequest{fixtureEvidence("asset-A").AssetRef, "synthetic-precancel"})
	if got.Status != StatusCanceled || !errors.Is(err, context.Canceled) {
		t.Fatalf("got %+v, %v", got, err)
	}
	for _, req := range []LookupRequest{{}, {Ref{"synthetic-cmdb", ""}, "r"}, {Ref{"", "asset-A"}, "r"}, {Ref{"synthetic-cmdb", "asset-A"}, ""}, {Ref{"synthetic-cmdb", " asset-A"}, "r"}, {Ref{"synthetic-cmdb", "asset\nA"}, "r"}} {
		got, err = c.Lookup(context.Background(), req)
		if got.Status != StatusUnavailable || !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("invalid request = %+v, %v", got, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid/canceled request sent")
	}
}

func TestLookupNeverFollowsRedirect(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}, nil)
	got, err := c.Lookup(context.Background(), LookupRequest{fixtureEvidence("asset-A").AssetRef, "synthetic-redirect"})
	if got.Status != StatusUnavailable || !errors.Is(err, ErrUnavailable) || targetCalls.Load() != 0 {
		t.Fatalf("redirect outcome %+v, %v, target calls=%d", got, err, targetCalls.Load())
	}
}
