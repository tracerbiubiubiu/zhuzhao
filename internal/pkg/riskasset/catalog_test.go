package riskasset

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
)

// This synthetic service only matches stable references. A shared display name
// cannot cause unknown identities to inherit another asset's evidence or owner.
func TestSyntheticCatalogStableRefMatchingControls(t *testing.T) {
	assets := map[Ref]Evidence{}
	for _, id := range []string{"asset-A", "asset-B"} {
		e := fixtureEvidence(id)
		assets[e.AssetRef] = e
	}
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		v := aksk.Verifier{Keys: map[string][]byte{fixtureAK: []byte(fixtureSK)}, Now: fixedNow}
		if v.Verify(r, body) != nil {
			t.Error("catalog signature failed")
			w.WriteHeader(401)
			return
		}
		var req LookupRequest
		if json.Unmarshal(body, &req) != nil {
			w.WriteHeader(400)
			return
		}
		if evidence, ok := assets[req.AssetRef]; ok {
			respond(t, w, StatusOK, evidence)
		} else {
			respond(t, w, StatusNotFound, Evidence{AssetRef: req.AssetRef, ObservedAt: fixtureNow})
		}
	}, nil)
	for _, tc := range []struct {
		name   string
		ref    Ref
		status Status
	}{
		{"known_A", Ref{"synthetic-cmdb", "asset-A"}, StatusOK},
		{"known_B_same_display_name", Ref{"synthetic-cmdb", "asset-B"}, StatusOK},
		{"unknown_asset", Ref{"synthetic-cmdb", "asset-unknown"}, StatusNotFound},
		{"wrong_source_same_id", Ref{"other-cmdb", "asset-A"}, StatusNotFound},
		{"no_fuzzy_match", Ref{"synthetic-cmdb", "Synthetic device"}, StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.Lookup(context.Background(), LookupRequest{tc.ref, "synthetic-catalog"})
			if err != nil || got.Status != tc.status {
				t.Fatalf("got %+v, %v", got, err)
			}
			if tc.status == StatusOK {
				if got.Evidence == nil || got.Evidence.AssetRef != tc.ref {
					t.Fatal("mismatched source evidence")
				}
			} else if got.Evidence != nil {
				t.Fatal("absence inherited evidence")
			}
		})
	}
}
