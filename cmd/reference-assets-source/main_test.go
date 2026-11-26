package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
	"github.com/tracerbiubiubiu/zhuzhao/pkg/referencequery"
)

func TestSignedSourceScopesRowsAndBindsCursor(t *testing.T) {
	source, err := loadSource("", "test-source", "test-key")
	if err != nil {
		t.Fatal(err)
	}
	query := referencequery.QueryRequest{
		QueryID: referencequery.QueryID, ContractVersion: referencequery.Version, SourceID: "ref-assets-v1",
		Parameters: referencequery.Parameters{OwnerOrgRefs: []string{"1"}}, Limit: 1, RequestedFields: []string{"device_id"},
	}
	do := func(ak string) (*httptest.ResponseRecorder, referencequery.QueryPage) {
		t.Helper()
		body, err := json.Marshal(query)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		aksk.Sign(req, body, aksk.SignOptions{AK: ak, SK: []byte("test-key")})
		res := httptest.NewRecorder()
		source.ServeHTTP(res, req)
		var page referencequery.QueryPage
		if err := json.Unmarshal(res.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		return res, page
	}
	if res, _ := do("wrong-source"); res.Code != http.StatusUnauthorized {
		t.Fatalf("wrong credential was not rejected (status %d)", res.Code)
	}
	res, page := do("test-source")
	if res.Code != http.StatusOK || len(page.Items) != 1 || len(page.Items[0]) != 1 ||
		page.Items[0]["device_id"] != "93001" || page.NextCursor == "" {
		t.Fatalf("unexpected first page (status %d): %s", res.Code, res.Body)
	}
	query.Cursor = page.NextCursor
	res, page = do("test-source")
	if res.Code != http.StatusOK || len(page.Items) != 1 || len(page.Items[0]) != 1 ||
		page.Items[0]["device_id"] != "93002" || page.NextCursor != "" {
		t.Fatalf("unexpected second page (status %d): %s", res.Code, res.Body)
	}
	query.Parameters.OwnerOrgRefs = []string{"2"}
	if res, _ := do("test-source"); res.Code != http.StatusConflict {
		t.Fatalf("cursor survived changed scope (status %d)", res.Code)
	}
	query.Parameters.OwnerOrgRefs = nil
	query.Cursor = ""
	if res, _ := do("test-source"); res.Code != http.StatusBadRequest {
		t.Fatalf("scope-free request was not rejected (status %d)", res.Code)
	}
	query.Parameters.OwnerOrgRefs = []string{"2"}
	query.Parameters.Status = []string{"healthy"}
	res, page = do("test-source")
	if res.Code != http.StatusOK || page.Items == nil || len(page.Items) != 0 || !page.Complete {
		t.Fatalf("status filter failed (status %d): %s", res.Code, res.Body)
	}
}
