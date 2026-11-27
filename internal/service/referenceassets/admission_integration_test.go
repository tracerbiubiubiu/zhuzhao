//go:build integration

package referenceassets_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
	"github.com/tracerbiubiubiu/zhuzhao-utils/jwt"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/referenceassets"
	"github.com/tracerbiubiubiu/zhuzhao/pkg/referencequery"
)

func checkReferenceAdmissionLifecycle(
	t *testing.T, cfg config.ReferenceAssetsConfig, pool *pgxpool.Pool, cache *redis.Client,
	clock *miniredis.Miniredis, credential middleware.ReadCredential, setRole func(context.Context, bool) error,
) {
	const (
		coreAK      = "core-to-synthetic-app"
		coreKey     = "synthetic-core-to-app-test-key"
		callbackAK  = "synthetic-app-to-core"
		callbackKey = "synthetic-app-to-core-test-key"
	)
	core := httptest.NewUnstartedServer(nil)
	t.Cleanup(core.Close)
	callback, err := referencequery.NewPeer("http://"+core.Listener.Addr().String(), callbackAK, callbackKey, cfg.Timeout)
	require.NoError(t, err)

	type appScenario func(context.Context, referencequery.Invocation) (referencequery.QueryPage, error)
	scenarios := make(chan appScenario, 1)
	var outbound atomic.Int64
	verifier := &aksk.Verifier{Keys: map[string][]byte{coreAK: []byte(coreKey)}}
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		outbound.Add(1)
		var invocation referencequery.Invocation
		if failure := referencequery.ReadSigned(r, verifier, referencequery.AppPath, &invocation); failure != nil {
			referencequery.WriteFailure(w, failure)
			return
		}
		select {
		case scenario := <-scenarios:
			page, err := scenario(r.Context(), invocation)
			var failure *referencequery.Failure
			switch {
			case err == nil:
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(page)
			case errors.As(err, &failure):
				referencequery.WriteFailure(w, failure)
			default:
				w.WriteHeader(http.StatusServiceUnavailable)
			}
		default:
			t.Error("unexpected call to the reference app")
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(app.Close)
	peer, err := referencequery.NewPeer(app.URL, coreAK, coreKey, cfg.Timeout)
	require.NoError(t, err)
	svc := referenceassets.New(cfg, peer, pool, cache)
	routes := gin.New()
	assets := handler.NewReferenceAssetsHandler(svc, callbackAK, []byte(callbackKey))
	routes.Use(middleware.RequestID())
	routes.POST(referencequery.AdmissionPath, assets.Admit)
	tokens := jwt.NewManager(jwt.Config{Secret: "synthetic-reference-query-session-test", AccessTTL: time.Minute})
	routes.GET(referencequery.QueryPath, middleware.JWT(tokens, cache, nil,
		middleware.WithAccountSecurity(referenceAccountLookup{repository.NewAccountSecurityLookup(pool)})), assets.Query)
	core.Config.Handler = routes
	core.Start()

	ctx := t.Context()
	query := referencequery.QueryRequest{Limit: 1, RequestedFields: []string{"device_id"}}
	postAdmission := func(ctx context.Context, inv referencequery.Invocation) (referencequery.Admission, error) {
		var admission referencequery.Admission
		err := callback.Post(ctx, referencequery.AdmissionPath, inv, &admission)
		return admission, err
	}
	pageForAdmission := func(admission referencequery.Admission) referencequery.QueryPage {
		stamp := time.Now().UTC().Format(time.RFC3339Nano)
		return referencequery.QueryPage{
			Items:    []map[string]any{{"device_id": "1", "owner_org_ref": admission.Query.Parameters.OwnerOrgRefs[0]}},
			SourceID: cfg.SourceID, ObservedAt: stamp, FetchedAt: stamp, MappingVersion: "admission-test",
			Warnings: []string{}, NextCursor: "synthetic-provider-cursor",
		}
	}
	for _, tc := range []struct {
		name   string
		before func(context.Context, *referencequery.Invocation) error
	}{
		{"wrong audience", func(_ context.Context, inv *referencequery.Invocation) error {
			inv.Audience = "unregistered.synthetic.app"
			return nil
		}},
		{"expired reference", func(context.Context, *referencequery.Invocation) error {
			clock.FastForward(time.Minute)
			return nil
		}},
		{"role revoked before admission", func(ctx context.Context, _ *referencequery.Invocation) error {
			return setRole(ctx, false)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			initialCalls := outbound.Load()
			scenarios <- func(ctx context.Context, inv referencequery.Invocation) (referencequery.QueryPage, error) {
				if err := tc.before(ctx, &inv); err != nil {
					return referencequery.QueryPage{}, err
				}
				_, err := postAdmission(ctx, inv)
				return referencequery.QueryPage{}, err
			}
			_, err := svc.Read(ctx, credential, "synthetic-request", query)
			requireReferenceDenied(t, err)
			require.Equal(t, initialCalls+1, outbound.Load())
			require.NoError(t, setRole(ctx, true))
		})
	}
	t.Run("query failures preserve the local login session", func(t *testing.T) {
		bearer, _, err := tokens.GenerateAccessTokenWithSessionEpoch(credential.UserID, "reference test reader", false, 0, credential.Epoch)
		require.NoError(t, err)
		read := func(token string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodGet, referencequery.QueryPath+"?limit=1&requested_fields=device_id", nil).WithContext(ctx)
			req.Header.Set("Authorization", "Bearer "+token)
			res := httptest.NewRecorder()
			routes.ServeHTTP(res, req)
			return res
		}
		initialCalls := outbound.Load()
		require.Equal(t, http.StatusUnauthorized, read("invalid-local-session").Code)
		require.Equal(t, initialCalls, outbound.Load())

		goodPage := func(ctx context.Context, inv referencequery.Invocation) (referencequery.QueryPage, error) {
			admission, err := postAdmission(ctx, inv)
			if err != nil {
				return referencequery.QueryPage{}, err
			}
			return pageForAdmission(admission), nil
		}
		for _, tc := range []struct {
			name   string
			app    appScenario
			status int
			reason string
		}{
			{"old app rejects Core service identity", func(context.Context, referencequery.Invocation) (referencequery.QueryPage, error) {
				return referencequery.QueryPage{}, &referencequery.Failure{Status: http.StatusUnauthorized, Code: "not_authorized"}
			}, http.StatusServiceUnavailable, "source_unavailable"},
			{"page has no admission acknowledgement", func(_ context.Context, inv referencequery.Invocation) (referencequery.QueryPage, error) {
				return pageForAdmission(referencequery.Admission{Query: inv.Query}), nil
			}, http.StatusServiceUnavailable, "source_unavailable"},
			{"admitted page violates the query contract", func(ctx context.Context, inv referencequery.Invocation) (referencequery.QueryPage, error) {
				page, err := goodPage(ctx, inv)
				page.SourceID = "unexpected-reference-source"
				return page, err
			}, http.StatusConflict, "contract_mismatch"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				scenarios <- tc.app
				res := read(bearer)
				require.Equal(t, tc.status, res.Code, res.Body.String())
				var failure referencequery.Failure
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &failure))
				require.Equal(t, tc.reason, failure.Code)

				scenarios <- goodPage
				success := read(bearer)
				require.Equal(t, http.StatusOK, success.Code, success.Body.String())
			})
		}
	})

	t.Run("revoked after admission cannot continue", func(t *testing.T) {
		scenarios <- func(ctx context.Context, inv referencequery.Invocation) (referencequery.QueryPage, error) {
			admission, err := postAdmission(ctx, inv)
			if err != nil {
				return referencequery.QueryPage{}, err
			}
			_, replayErr := postAdmission(ctx, inv)
			var failure *referencequery.Failure
			if assert.ErrorAs(t, replayErr, &failure) {
				assert.Equal(t, http.StatusForbidden, failure.Status)
			}
			if err := setRole(ctx, false); err != nil {
				return referencequery.QueryPage{}, err
			}
			return pageForAdmission(admission), nil
		}
		page, err := svc.Read(ctx, credential, "synthetic-request", query)
		require.NoError(t, err)
		require.Equal(t, []map[string]any{{"device_id": "1"}}, page.Items)
		require.NotEmpty(t, page.NextCursor)
		initialCalls := outbound.Load()
		query.Cursor = page.NextCursor
		_, err = svc.Read(ctx, credential, "synthetic-continuation", query)
		requireReferenceDenied(t, err)
		require.Equal(t, initialCalls, outbound.Load())
	})
}

func requireReferenceDenied(t *testing.T, err error) {
	t.Helper()
	var failure *referencequery.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, http.StatusForbidden, failure.Status)
	require.Equal(t, "not_authorized", failure.Code)
}

type referenceAccountLookup struct {
	*repository.AccountSecurityLookup
}

func (r referenceAccountLookup) AccountSnapshot(ctx context.Context, user int64) (middleware.AccountSnapshot, error) {
	snap, err := r.AccountSecuritySnapshot(ctx, user)
	return middleware.AccountSnapshot{Status: snap.Status, Deleted: snap.Deleted,
		SessionEpoch: snap.SessionEpoch, ProjectionEpoch: snap.ProjectionEpoch}, err
}
