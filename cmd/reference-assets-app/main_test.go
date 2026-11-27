package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
	"github.com/tracerbiubiubiu/zhuzhao/pkg/referencequery"
)

func TestAppRechecksAdmissionAndSourceRows(t *testing.T) {
	var sourceMode atomic.Int64
	var sourceCalls atomic.Int64
	var admissionCalls atomic.Int64
	inv := referencequery.Invocation{
		RequestRef: "opaque-test-reference", Audience: referencequery.ExtensionID,
		CapabilityID: referencequery.QueryID, ContractVersion: referencequery.Version,
		ConfigRevision: "revision-a", RequestID: "req-1",
		Method: http.MethodGet, Path: "/api/v1/extensions/reference-assets",
		Query: referencequery.QueryRequest{
			QueryID: referencequery.QueryID, ContractVersion: referencequery.Version, SourceID: "ref-assets-v1",
			Parameters: referencequery.Parameters{OwnerOrgRefs: []string{"1"}}, Limit: 1,
			RequestedFields: []string{"name"},
		},
	}
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admissionCalls.Add(1)
		var got referencequery.Invocation
		verifyTestRequest(t, w, r, referencequery.AdmissionPath, "app-admission", "admission-key", &got)
		if !reflect.DeepEqual(got, inv) {
			t.Errorf("admission invocation changed: %+v", got)
		}
		if sourceMode.Load() == 6 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"reason":"not_authorized"}`)
			return
		}
		if sourceMode.Load() == 3 {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"reason":"not_authorized"}`)
			return
		}
		query := got.Query
		if sourceMode.Load() == 4 {
			query.Parameters.OwnerOrgRefs = []string{}
		}
		if sourceMode.Load() == 5 {
			query.Parameters.OwnerOrgRefs = []string{"2"}
		}
		_ = json.NewEncoder(w).Encode(referencequery.Admission{Query: query})
	}))
	defer callback.Close()

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sourceCalls.Add(1)
		var got referencequery.QueryRequest
		verifyTestRequest(t, w, r, "/v1/query", "app-source", "source-key", &got)
		if len(got.RequestedFields) != 3 || len(got.Parameters.OwnerOrgRefs) != 1 || got.Parameters.OwnerOrgRefs[0] != "1" {
			t.Errorf("provider request lacks verification fields or exact scope: %+v", got)
		}
		org := "1"
		switch sourceMode.Load() {
		case 1:
			org = "2"
		case 7:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"reason":"not_authorized"}`)
			return
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"reason":"source_unavailable"}`)
			return
		}
		stamp := "2026-11-26T00:00:00Z"
		_ = json.NewEncoder(w).Encode(referencequery.QueryPage{
			Items:    []map[string]any{{"name": "Device A", "owner_org_ref": org, "device_id": "1"}},
			Complete: true, SourceID: "ref-assets-v1", ObservedAt: stamp, FetchedAt: stamp,
			MappingVersion: "ref-assets-mapping-v1", Warnings: []string{},
		})
	}))
	defer provider.Close()

	admission, err := referencequery.NewPeer(callback.URL, "app-admission", "admission-key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	source, err := referencequery.NewClient(provider.URL, "app-source", "source-key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	app := newAppHandler("trusted-core", "core-key", admission, source, time.Second)
	do := func(operator string) (*httptest.ResponseRecorder, referencequery.QueryPage) {
		t.Helper()
		body, err := json.Marshal(inv)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/reference-assets/query", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		aksk.Sign(req, body, aksk.SignOptions{AK: "trusted-core", SK: []byte("core-key"), Operator: operator, RequestID: inv.RequestID})
		res := httptest.NewRecorder()
		app.ServeHTTP(res, req)
		var page referencequery.QueryPage
		if err := json.Unmarshal(res.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		return res, page
	}

	if res, _ := do("forged-actor"); res.Code != http.StatusUnauthorized || admissionCalls.Load() != 0 {
		t.Fatalf("operator was not rejected before admission, status=%d calls=%d", res.Code, admissionCalls.Load())
	}
	res, page := do("")
	if res.Code != http.StatusOK || !reflect.DeepEqual(page.Items, []map[string]any{{"name": "Device A"}}) {
		t.Fatalf("unexpected projection (status %d): %s", res.Code, res.Body)
	}
	for _, tc := range []struct {
		name       string
		mode       int64
		wantStatus int
		wantReason string
		sourceHit  bool
	}{
		{"out-of-scope row", 1, http.StatusConflict, "contract_mismatch", true},
		{"unavailable provider", 2, http.StatusServiceUnavailable, "source_unavailable", true},
		{"revoked admission", 3, http.StatusForbidden, "not_authorized", false},
		{"empty scope", 4, http.StatusOK, "", false},
		{"expanded admission scope", 5, http.StatusServiceUnavailable, "source_unavailable", false},
		{"admission service rejects app identity", 6, http.StatusServiceUnavailable, "source_unavailable", false},
		{"source service rejects app identity", 7, http.StatusServiceUnavailable, "source_unavailable", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sourceMode.Store(tc.mode)
			before := sourceCalls.Load()
			res, page := do("")
			if tc.wantStatus != res.Code || tc.sourceHit != (sourceCalls.Load() > before) {
				t.Fatalf("unexpected result (status %d, source calls %d): %s", res.Code, sourceCalls.Load()-before, res.Body)
			}
			var failure referencequery.Failure
			if err := json.Unmarshal(res.Body.Bytes(), &failure); err != nil || failure.Code != tc.wantReason {
				t.Fatalf("unexpected failure reason: %s", res.Body)
			}
			if tc.mode == 4 && (page.Items == nil || len(page.Items) != 0 || !page.Complete) {
				t.Fatalf("empty scope must return a complete empty page: %s", res.Body)
			}
		})
	}
}

func verifyTestRequest(t *testing.T, w http.ResponseWriter, r *http.Request, path, ak, key string, dest any) {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	verifier := aksk.Verifier{Keys: map[string][]byte{ak: []byte(key)}}
	if err != nil || r.URL.Path != path || r.Method != http.MethodPost ||
		verifier.Verify(r, body) != nil || json.Unmarshal(body, dest) != nil {
		t.Errorf("invalid signed request to %s", path)
		w.WriteHeader(http.StatusBadRequest)
	}
}
