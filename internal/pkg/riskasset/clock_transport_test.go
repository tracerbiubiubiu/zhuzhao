package riskasset

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
)

func TestLookupClockAtCompletionAndCredentialCopy(t *testing.T) {
	secret := []byte(fixtureSK)
	clockCalls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		v := aksk.Verifier{Keys: map[string][]byte{fixtureAK: []byte(fixtureSK)}, Now: fixedNow}
		if v.Verify(r, body) != nil {
			t.Error("credential alias changed signature")
			w.WriteHeader(401)
			return
		}
		respond(t, w, StatusOK, fixtureEvidence("asset-A"))
	}, func(cfg *Config) {
		cfg.SK = secret
		cfg.Now = func() time.Time {
			clockCalls++
			if clockCalls == 1 {
				return fixtureNow
			}
			return fixtureNow.Add(10 * time.Minute)
		}
	})
	secret[0] = '!'
	got, err := c.Lookup(context.Background(), LookupRequest{fixtureEvidence("asset-A").AssetRef, "synthetic-clock"})
	if err != nil || got.Status != StatusStale || got.Evidence == nil {
		t.Fatalf("got %+v, %v", got, err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("synthetic-upstream-sensitive-detail")
}

func TestLookupTransportAndReadFailureClassification(t *testing.T) {
	for _, kind := range []string{"network", "network_timeout", "body"} {
		t.Run(kind, func(t *testing.T) {
			body := &countingBody{Reader: failingReader{}}
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected network") }, func(cfg *Config) {
				cfg.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
					switch kind {
					case "network":
						return nil, errors.New("synthetic-upstream-sensitive-detail")
					case "network_timeout":
						return nil, &net.DNSError{Err: "synthetic-upstream-sensitive-detail", IsTimeout: true}
					default:
						return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
					}
				})
			})
			got, err := c.Lookup(context.Background(), LookupRequest{fixtureEvidence("asset-A").AssetRef, "synthetic-transport"})
			status, wantErr := StatusUnavailable, ErrUnavailable
			if kind == "network_timeout" {
				status = StatusTimeout
				wantErr = context.DeadlineExceeded
			}
			if got.Status != status || got.Evidence != nil || !errors.Is(err, wantErr) || strings.Contains(err.Error(), "sensitive-detail") {
				t.Fatalf("got %+v, %v", got, err)
			}
			if kind == "body" && !body.closed {
				t.Fatal("failed body left open")
			}
		})
	}
}

func TestLookupCanceledBeforePublication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { respond(t, w, StatusOK, fixtureEvidence("asset-A")) }, func(cfg *Config) {
		cfg.Now = func() time.Time {
			calls++
			if calls == 2 {
				cancel()
			}
			return fixtureNow
		}
	})
	got, err := c.Lookup(ctx, LookupRequest{fixtureEvidence("asset-A").AssetRef, "synthetic-late-cancel"})
	if got.Status != StatusCanceled || got.Evidence != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled result published: %+v, %v", got, err)
	}
}
