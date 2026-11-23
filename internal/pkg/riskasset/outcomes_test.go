package riskasset

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLookupIdentityTimeAndOutcomeControls(t *testing.T) {
	valid := fixtureEvidence("asset-A")
	cases := []struct {
		name         string
		change       func(map[string]any)
		status       Status
		wantErr      error
		wantEvidence bool
	}{
		{"fresh", func(m map[string]any) {}, StatusOK, nil, true},
		{"fresh_boundary", func(m map[string]any) { m["observed_at"] = fixtureNow.Add(-5 * time.Minute) }, StatusOK, nil, true},
		{"stale_boundary", func(m map[string]any) { m["observed_at"] = fixtureNow.Add(-5*time.Minute - time.Nanosecond) }, StatusStale, nil, true},
		{"source_stale", func(m map[string]any) { m["status"] = "stale" }, StatusStale, nil, true},
		{"no_hit", func(m map[string]any) {
			m["status"] = "not_found"
			delete(m, "snapshot")
			delete(m, "owner_external_ref")
			delete(m, "org_external_ref")
			delete(m, "deep_link")
		}, StatusNotFound, nil, false},
		{"stale_absence", func(m map[string]any) { m["status"] = "not_found"; m["observed_at"] = fixtureNow.Add(-time.Hour) }, StatusUnavailable, ErrUnavailable, false},
		{"wrong_provider", func(m map[string]any) { m["asset_ref"] = Ref{"another-source", "asset-A"} }, StatusUnavailable, ErrUnavailable, false},
		{"wrong_id", func(m map[string]any) { m["asset_ref"] = Ref{"synthetic-cmdb", "asset-B"} }, StatusUnavailable, ErrUnavailable, false},
		{"id_case_mismatch", func(m map[string]any) { m["asset_ref"] = Ref{"synthetic-cmdb", "ASSET-A"} }, StatusUnavailable, ErrUnavailable, false},
		{"missing_identity", func(m map[string]any) { delete(m, "asset_ref") }, StatusUnavailable, ErrUnavailable, false},
		{"not_found_wrong_identity", func(m map[string]any) { m["status"] = "not_found"; m["asset_ref"] = Ref{"other", "asset-A"} }, StatusUnavailable, ErrUnavailable, false},
		{"missing_observed_at", func(m map[string]any) { delete(m, "observed_at") }, StatusUnavailable, ErrUnavailable, false},
		{"null_observed_at", func(m map[string]any) { m["observed_at"] = nil }, StatusUnavailable, ErrUnavailable, false},
		{"invalid_observed_at", func(m map[string]any) { m["observed_at"] = "yesterday" }, StatusUnavailable, ErrUnavailable, false},
		{"zero_observed_at", func(m map[string]any) { m["observed_at"] = time.Time{} }, StatusUnavailable, ErrUnavailable, false},
		{"future_observed_at", func(m map[string]any) { m["observed_at"] = fixtureNow.Add(time.Nanosecond) }, StatusUnavailable, ErrUnavailable, false},
		{"unknown_status", func(m map[string]any) { m["status"] = "healthy" }, StatusUnavailable, ErrUnavailable, false},
		{"missing_status", func(m map[string]any) { delete(m, "status") }, StatusUnavailable, ErrUnavailable, false},
		{"unresolved_owner", func(m map[string]any) { delete(m, "owner_external_ref"); delete(m, "org_external_ref") }, StatusOK, nil, true},
		{"display_empty", func(m map[string]any) { m["snapshot"] = Snapshot{} }, StatusOK, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, _ := json.Marshal(valid)
			var m map[string]any
			_ = json.Unmarshal(payload, &m)
			m["status"] = "ok"
			tc.change(m)
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(m)
			}, nil)
			got, err := c.Lookup(context.Background(), LookupRequest{valid.AssetRef, "synthetic-outcome"})
			if got.Status != tc.status || !errors.Is(err, tc.wantErr) || (got.Evidence != nil) != tc.wantEvidence {
				t.Fatalf("got %+v, %v; want %s, %v, evidence=%v", got, err, tc.status, tc.wantErr, tc.wantEvidence)
			}
			if got.Evidence != nil && (got.Evidence.AssetRef != valid.AssetRef || got.Evidence.OwnerExternalRef != m["owner_external_ref"] && m["owner_external_ref"] != nil) {
				t.Fatal("source evidence changed")
			}
			if tc.name == "unresolved_owner" && got.Evidence.OwnerExternalRef != "" {
				t.Fatal("invented local owner")
			}
		})
	}
}

func TestLookupHTTPAndBodyFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		body string
	}{
		{"bare_404_is_not_authoritative_absence", 404, `{}`},
		{"unauthorized", 401, `{"secret":"synthetic-upstream-detail"}`},
		{"forbidden", 403, `{}`}, {"rate_limit", 429, `{}`}, {"failed_dependency", 503, `{}`},
		{"empty", 200, ""}, {"null", 200, "null"}, {"array", 200, "[]"}, {"malformed", 200, "{"},
		{"too_large", 200, strings.Repeat("x", 1025)},
		{"trailing_json", 200, `{} {}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}, func(cfg *Config) { cfg.MaxResponseBytes = 1024 })
			got, err := c.Lookup(context.Background(), LookupRequest{fixtureEvidence("asset-A").AssetRef, "synthetic-failure"})
			if got.Status != StatusUnavailable || got.Evidence != nil || !errors.Is(err, ErrUnavailable) {
				t.Fatalf("got %+v, %v", got, err)
			}
			if err.Error() != ErrUnavailable.Error() {
				t.Fatal("upstream detail leaked")
			}
		})
	}
}
