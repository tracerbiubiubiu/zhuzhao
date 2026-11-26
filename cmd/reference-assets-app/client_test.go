package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFrozenCallerKeepsV1ShapeAndToleratesOptionalFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/extensions/reference-assets" ||
			r.Header.Get("Authorization") != "Bearer compatibility-credential" ||
			r.URL.Query().Get("limit") != "1" || r.URL.Query().Get("requested_fields") != "device_id" {
			t.Errorf("unexpected compatibility request: %s %s", r.Method, r.URL)
		}
		if r.URL.Query().Get("cursor") == "invalid" {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"reason":"contract_mismatch"}`)
			return
		}
		if r.URL.Query().Get("cursor") == "drifted" {
			_, _ = io.WriteString(w, `{"data":[{"device_id":"1"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"items":[{"device_id":"1"}],"next_cursor":"next","complete":false,"source_id":"fixture","observed_at":"2026-11-26T00:00:00Z","fetched_at":"2026-11-26T00:00:01Z","mapping_version":"v1","warnings":[],"future_optional":"new"}`)
	}))
	defer server.Close()

	for _, tc := range []struct {
		cursor, wantOutput string
		wantErr            bool
	}{
		{"", "future_optional", false},
		{"invalid", "contract_mismatch", true},
		{"drifted", "", true},
	} {
		var out bytes.Buffer
		query := "limit=1&requested_fields=device_id&cursor=" + tc.cursor
		err := checkCore(server.URL, "compatibility-credential", query, time.Second, &out)
		if (err != nil) != tc.wantErr || !strings.Contains(out.String(), tc.wantOutput) {
			t.Fatalf("cursor %q output %q, error %v", tc.cursor, out.String(), err)
		}
	}
}
