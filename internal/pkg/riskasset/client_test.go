package riskasset

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
)

var fixtureNow = time.Date(2026, 11, 22, 4, 0, 0, 0, time.UTC)

const fixtureAK = "synthetic-asset-lookup-only"
const fixtureSK = "synthetic-directory-secret"

func fixedNow() time.Time { return fixtureNow }

func fixtureEvidence(id string) Evidence {
	return Evidence{
		AssetRef:         Ref{Provider: "synthetic-cmdb", ExternalID: id},
		ObservedAt:       fixtureNow.Add(-time.Minute),
		OwnerExternalRef: "synthetic-person-17", OrgExternalRef: "synthetic-org-2",
		DeepLink: "https://assets.example.invalid/view/" + id,
		Snapshot: Snapshot{DisplayName: "Synthetic device", Components: []Component{{Name: "sample-component", Version: "1.2.3"}}},
	}
}

func fixtureClient(t *testing.T, handler http.HandlerFunc, configure func(*Config)) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	// A fresh transport disables proxies and uses only the explicit loopback fixture URL.
	transport := &http.Transport{Proxy: nil}
	t.Cleanup(transport.CloseIdleConnections)
	cfg := Config{BaseURL: srv.URL, AK: fixtureAK, SK: []byte(fixtureSK), Timeout: time.Second,
		MaxAge: 5 * time.Minute, Now: fixedNow, Transport: transport}
	if configure != nil {
		configure(&cfg)
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func respond(t *testing.T, w http.ResponseWriter, status Status, evidence Evidence) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		Status Status `json:"status"`
		Evidence
	}{status, evidence}); err != nil {
		t.Error(err)
	}
}

func TestLookupSignedStableIdentityEvidenceAndRepeat(t *testing.T) {
	for _, id := range []string{"asset-A", "asset-B"} {
		t.Run(id, func(t *testing.T) {
			want := fixtureEvidence(id)
			request := LookupRequest{AssetRef: want.AssetRef, RequestID: "synthetic-request-1"}
			calls := 0
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
				if err != nil {
					t.Error(err)
					return
				}
				verifier := aksk.Verifier{Keys: map[string][]byte{fixtureAK: []byte(fixtureSK)}, Now: fixedNow}
				if err := verifier.Verify(r, body); err != nil {
					t.Error(err)
					w.WriteHeader(401)
					return
				}
				var got LookupRequest
				if err := json.Unmarshal(body, &got); err != nil {
					t.Error(err)
					return
				}
				if got != request || r.Method != "POST" || r.URL.RequestURI() != "/v1/assets/lookup" ||
					r.Header.Get("X-Request-ID") != request.RequestID || r.Header.Get("X-Operator") != "" ||
					r.Header.Get("Content-Type") != "application/json" {
					t.Error("unexpected signed request")
				}
				respond(t, w, StatusOK, want)
			}, nil)
			for range 2 {
				got, err := c.Lookup(context.Background(), request)
				if err != nil || got.Status != StatusOK || !reflect.DeepEqual(got.Evidence, &want) {
					t.Fatalf("Lookup = %+v, %v; want ok and exact source evidence", got, err)
				}
			}
			if calls != 2 {
				t.Fatalf("requests = %d, want 2 independent reads", calls)
			}
		})
	}
}
