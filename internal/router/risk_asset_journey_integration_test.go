//go:build integration

package router_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/riskasset"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/router"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

const (
	assetFresh int32 = iota
	assetStale
	assetTimeout
	assetUnavailable
)

func TestRiskFeedSyntheticIntelAndAssetHTTPJourney(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := t.Context()
	db, _, err := testutil.SetupPostgresShared()
	require.NoError(t, err)
	const source = "synthetic-step16-intel"
	const assetProvider = "synthetic-step16-assets"
	const route = "/api/v1/risk-items/:id/asset"
	const assessRoute = "/api/v1/risk-items/assess"

	var mode atomic.Int32
	var lookupCount atomic.Int32
	const assetAK, assetSK = "synthetic-asset-key", "synthetic-asset-secret"
	verifier := &aksk.Verifier{Keys: map[string][]byte{assetAK: []byte(assetSK)}}
	directory := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookupCount.Add(1)
		body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		if err != nil || r.Method != http.MethodPost || r.URL.Path != "/v1/assets/lookup" || verifier.Verify(r, body) != nil {
			t.Error("invalid synthetic CMDB request")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var input riskasset.LookupRequest
		if err := json.Unmarshal(body, &input); err != nil || input.RequestID == "" ||
			input.RequestID != r.Header.Get("X-Request-ID") || input.AssetRef.Provider != assetProvider {
			t.Error("invalid synthetic CMDB identity")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		evidence := riskasset.Evidence{AssetRef: input.AssetRef, ObservedAt: time.Now().UTC().Add(-time.Second)}
		status := riskasset.StatusOK
		switch input.AssetRef.ExternalID {
		case "asset-a":
			evidence.OwnerExternalRef = "synthetic-owner"
			evidence.OrgExternalRef = "synthetic-org"
			evidence.Snapshot = riskasset.Snapshot{DisplayName: "Synthetic shared name",
				Components: []riskasset.Component{{Name: "synthetic-component", Version: "1.0"}}}
		case "asset-b":
			evidence.Snapshot.DisplayName = "Synthetic shared name"
		default:
			status = riskasset.StatusNotFound
		}
		switch mode.Load() {
		case assetStale:
			evidence.ObservedAt = time.Now().UTC().Add(-time.Hour)
		case assetTimeout:
			<-r.Context().Done()
			return
		case assetUnavailable:
			http.Error(w, "synthetic upstream detail", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			Status riskasset.Status `json:"status"`
			riskasset.Evidence
		}{Status: status, Evidence: evidence}); err != nil {
			t.Error("write synthetic CMDB response")
		}
	}))
	t.Cleanup(directory.Close)
	assetClient, err := (config.RiskAssetDirectoryConfig{
		Enabled: true, Origin: directory.URL, AK: assetAK, SK: assetSK, Timeout: 350 * time.Millisecond, MaxAge: time.Minute,
	}).Client()
	require.NoError(t, err)

	enforcer := newRouterTestEnforcer(t)
	const role = "synthetic_step16_asset_reader"
	_, err = enforcer.AddPolicy("role::"+role, route, http.MethodGet)
	require.NoError(t, err)
	_, err = enforcer.AddPolicy("role::"+role, assessRoute, http.MethodPost)
	require.NoError(t, err)
	_, err = db.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,'POST')`, "role::"+role, assessRoute)
	require.NoError(t, err)
	var roleID, orgID int64
	require.NoError(t, db.QueryRow(ctx, `INSERT INTO roles(code,name,priority)
		VALUES($1,$1,60) RETURNING id`, role).Scan(&roleID))
	require.NoError(t, db.QueryRow(ctx, `INSERT INTO organizations(code,name,path,status,is_virtual)
		VALUES('synthetic_step16_asset_org','Synthetic Asset Organization','synthetic_step16_asset_org',1,true)
		RETURNING id`).Scan(&orgID))
	addUser := func(name string, member bool) int64 {
		t.Helper()
		var id int64
		require.NoError(t, db.QueryRow(ctx, `INSERT INTO users
			(username,password,employee_no,real_name,status,session_epoch,session_projection_epoch)
			VALUES($1,'synthetic-only',$1,$1,1,3,3) RETURNING id`, name).Scan(&id))
		_, err := db.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)`, id, roleID)
		require.NoError(t, err)
		if member {
			_, err := db.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id,org_member_role,ticket_scope)
				VALUES($1,$2,'admin','group')`, id, orgID)
			require.NoError(t, err)
		}
		return id
	}
	manager := addUser("synthetic_step16_asset_manager", true)
	outside := addUser("synthetic_step16_asset_outside", false)
	_, err = db.Exec(ctx, `INSERT INTO risk_sources
		(source_ref,display_name,fingerprint_scope,ordered,mapping_ref,allowed_org_ids,default_org_id,enabled)
		VALUES($1,$1,$1,true,'mapping:v1',ARRAY[$2::bigint],$2,true)`, source, orgID)
	require.NoError(t, err)

	feed := config.RiskIngestConfig{Enabled: true, SourceRef: source, AK: "synthetic-intel-key", SK: "synthetic-intel-secret"}
	jwt := jwtpkg.NewManager(jwtpkg.Config{Secret: "synthetic-step16-asset-jwt", AccessTTL: time.Minute})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := router.New(router.Deps{
		DBPool: db, JWTManager: jwt, Enforcer: enforcer, RedisClient: startStubRedis(t), Logger: logger,
		RoleFetcher:  service.NewRBACService(repository.NewRoleRepo(db), repository.NewUserRepo(db), repository.NewMenuRepo(db), enforcer),
		AuditService: service.NewAuditService(repository.NewAuditLogRepo(db), repository.NewUserRepo(db)),
		RiskIngest:   feed, RiskHandler: handler.NewRiskHandler(risk.NewService(db, logger), assetClient),
	})

	observedAt := time.Now().UTC().Add(-time.Minute)
	validUntil := observedAt.Add(time.Hour)
	observation := func(asset, sequence, conclusion string) model.FindingObservationV1 {
		return model.FindingObservationV1{
			SourceRef: source, SourceItemKey: "synthetic-finding:" + asset, SourceSequence: sequence,
			EventID: "synthetic-match-" + asset, ObservedAt: observedAt, ValidUntil: &validUntil,
			FindingKey: "synthetic-finding", AssetRef: &model.RiskAssetRef{Provider: assetProvider, ExternalID: asset},
			ConclusionRef: conclusion, MappingRef: "mapping:v1",
			EvidenceRefs: []model.RiskEvidenceRef{{Provider: source, ExternalID: "conclusion-" + asset}},
		}
	}
	// The synthetic provider supplies fixed positive conclusions. Unknown assets
	// enter as candidates without a match conclusion and stay unassigned.
	input, err := json.Marshal(model.RiskReportRequest{Observations: []model.FindingObservationV1{
		observation("asset-a", "1", "synthetic confirmed A"),
		observation("asset-b", "1", "synthetic confirmed B"),
		observation("asset-unknown", "1", ""),
	}})
	require.NoError(t, err)
	report := func(requestID string) []model.RiskReportItem {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/risk-items/report", bytes.NewReader(input))
		request.Header.Set("Content-Type", "application/json")
		aksk.Sign(request, input, aksk.SignOptions{AK: feed.AK, SK: []byte(feed.SK), RequestID: requestID})
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var result struct{ Data model.RiskReportResult }
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		require.Len(t, result.Data.Items, 3)
		return result.Data.Items
	}
	accepted := report("synthetic-intel-initial")
	replayed := report("synthetic-intel-replay")
	for i, item := range accepted {
		require.Equal(t, "accepted", item.Result)
		require.NotNil(t, item.RiskRef)
		require.Equal(t, "duplicate", replayed[i].Result)
		require.Equal(t, item.IngestID, replayed[i].IngestID)
		require.Equal(t, item.RiskRef, replayed[i].RiskRef)
	}
	require.NotEqual(t, accepted[0].RiskRef.ID, accepted[1].RiskRef.ID)

	lookup := func(id int64, want string) *riskasset.Evidence {
		t.Helper()
		path := fmt.Sprintf("/api/v1/risk-items/%d/asset", id)
		result := sendRiskAuditFixtureRequest(t, r, jwt, manager, http.MethodGet, path, "")
		require.Equal(t, http.StatusOK, result.Code, result.Body.String())
		require.NotContains(t, result.Body.String(), "synthetic upstream detail")
		var decoded struct{ Data risk.AssetEvidenceResult }
		require.NoError(t, json.Unmarshal(result.Body.Bytes(), &decoded))
		require.Equal(t, want, decoded.Data.Status)
		return decoded.Data.Evidence
	}
	idA, idB, idUnknown := accepted[0].RiskRef.ID, accepted[1].RiskRef.ID, accepted[2].RiskRef.ID
	hidden := sendRiskAuditFixtureRequest(t, r, jwt, outside, http.MethodGet,
		fmt.Sprintf("/api/v1/risk-items/%d/asset", idA), "")
	require.Equal(t, http.StatusNotFound, hidden.Code)
	require.Zero(t, lookupCount.Load())
	first := lookup(idA, "ok")
	require.NotNil(t, first)
	require.Equal(t, "asset-a", first.AssetRef.ExternalID)
	require.Equal(t, "synthetic-owner", first.OwnerExternalRef)
	require.Len(t, first.Snapshot.Components, 1)
	second := lookup(idB, "ok")
	require.NotNil(t, second)
	require.Equal(t, "asset-b", second.AssetRef.ExternalID)
	require.Equal(t, first.Snapshot.DisplayName, second.Snapshot.DisplayName)
	require.Empty(t, second.Snapshot.Components)
	require.Empty(t, second.OwnerExternalRef)
	require.Nil(t, lookup(idUnknown, "not_found"))
	mode.Store(assetStale)
	require.NotNil(t, lookup(idA, "stale"))
	mode.Store(assetUnavailable)
	require.Nil(t, lookup(idA, "unavailable"))
	mode.Store(assetTimeout)
	require.Nil(t, lookup(idA, "timeout"))
	mode.Store(assetFresh)
	require.NotNil(t, lookup(idA, "ok"))
	require.Equal(t, int32(7), lookupCount.Load())

	for _, item := range accepted {
		var owner *int64
		var revision int64
		var state string
		var events int
		require.NoError(t, db.QueryRow(ctx, `SELECT owner_user_id, revision, status,
			(SELECT count(*) FROM risk_events WHERE risk_id=risk_items.id)
			FROM risk_items WHERE id=$1`, item.RiskRef.ID).Scan(&owner, &revision, &state, &events))
		require.Nil(t, owner)
		require.Equal(t, int64(1), revision)
		require.Equal(t, "new", state)
		require.Equal(t, 1, events)
	}

	const assessmentNote = "Synthetic component and asset verified after directory recovery"
	assessJSON := fmt.Sprintf(`{"id":"%d","operation_id":"synthetic-step16-recovery-assessment",
		"expected_revision":"1","level":"medium","treatment":"mitigate","assessment_note":%q}`, idA, assessmentNote)
	assess := sendRiskAuditFixtureRequest(t, r, jwt, manager, http.MethodPost, assessRoute, assessJSON)
	require.Equal(t, http.StatusOK, assess.Code, assess.Body.String())
	var evaluated struct{ Data model.RiskMutationResult }
	require.NoError(t, json.Unmarshal(assess.Body.Bytes(), &evaluated))
	require.Equal(t, idA, evaluated.Data.Risk.ID)
	require.Equal(t, int64(2), evaluated.Data.Risk.Revision)
	var savedNote string
	require.NoError(t, db.QueryRow(ctx, `SELECT payload ->> 'note' FROM risk_events
		WHERE risk_id=$1 AND event_kind='assessed'`, idA).Scan(&savedNote))
	require.Equal(t, assessmentNote, savedNote)
}
