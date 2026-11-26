//go:build integration

package referenceassets_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/referenceassets"
	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
	"github.com/tracerbiubiubiu/zhuzhao/pkg/referencequery"
)

func TestReferenceAuthorization(t *testing.T) {
	pool, closePostgres, err := testutil.SetupPostgresShared()
	require.NoError(t, err)
	t.Cleanup(closePostgres)
	ctx := t.Context()
	users, roles, orgs := repository.NewUserRepo(pool), repository.NewRoleRepo(pool), repository.NewOrgRepo(pool)
	user := &model.User{Username: "ref_reader", EmployeeNo: "REF_READER", Password: "synthetic-unused", Status: 1}
	require.NoError(t, users.Create(ctx, user))
	role := &model.Role{Code: "reference_reader", Name: "Reference reader", Priority: 10, Status: 1}
	require.NoError(t, roles.Create(ctx, role))
	require.NoError(t, users.SetRoles(ctx, user.ID, []int64{role.ID}))
	snap, err := users.SecuritySnapshot(ctx, user.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE users SET session_projection_epoch = $2 WHERE id = $1`, user.ID, snap.SessionEpoch)
	require.NoError(t, err)
	credential := middleware.ReadCredential{
		UserID: user.ID, Epoch: snap.SessionEpoch, JTI: "synthetic-reference-session", ExpiresAt: time.Now().Add(time.Minute),
	}

	redisServer := miniredis.RunT(t)
	require.NoError(t, redisServer.Set(fmt.Sprintf("user:session_epoch:db1:%d", user.ID), strconv.FormatInt(snap.SessionEpoch, 10)))
	cache := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = cache.Close() })
	var sourceCalls atomic.Int64
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sourceCalls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(app.Close)
	peer, err := referencequery.NewPeer(app.URL, "core-to-reference", "synthetic-reference-key", 2*time.Second)
	require.NoError(t, err)
	cfg := config.ReferenceAssetsConfig{
		ConfigRevision: "scope-v1", SourceID: "ref-assets-v1", ReaderRoles: []string{role.Code}, Timeout: 5 * time.Second,
	}
	svc := referenceassets.New(cfg, peer, pool, cache)
	query := referencequery.QueryRequest{Limit: 1, RequestedFields: []string{"device_id"}}

	t.Run("no current direct membership", func(t *testing.T) {
		_, err := svc.Read(ctx, credential, "synthetic-request", query)
		requireReferenceDenied(t, err)
	})
	org := &model.Organization{Code: "reference_org", Name: "Reference organization", Path: "reference_org", Status: 1}
	other := &model.Organization{Code: "other_reference_org", Name: "Other reference organization", Path: "other_reference_org", Status: 1}
	require.NoError(t, orgs.Create(ctx, org))
	require.NoError(t, orgs.Create(ctx, other))
	require.NoError(t, orgs.AddMember(ctx, org.ID, user.ID, true))
	t.Run("authorized reader with disjoint filter", func(t *testing.T) {
		filtered := query
		filtered.Parameters.OwnerOrgRefs = []string{strconv.FormatInt(other.ID, 10)}
		page, err := svc.Read(ctx, credential, "synthetic-request", filtered)
		require.NoError(t, err)
		require.Equal(t, []map[string]any{}, page.Items)
		require.True(t, page.Complete)
		require.Equal(t, []string{"source_not_queried"}, page.Warnings)
	})
	t.Run("expired last membership", func(t *testing.T) {
		_, err := pool.Exec(ctx, `UPDATE user_orgs SET expires_at = NOW() - interval '1 second' WHERE user_id = $1 AND org_id = $2`, user.ID, org.ID)
		require.NoError(t, err)
		_, err = svc.Read(ctx, credential, "synthetic-request", query)
		requireReferenceDenied(t, err)
	})
	require.Zero(t, sourceCalls.Load())

	_, err = pool.Exec(ctx, `UPDATE user_orgs SET expires_at = NULL WHERE user_id = $1 AND org_id = $2`, user.ID, org.ID)
	require.NoError(t, err)
	t.Run("admission lifecycle", func(t *testing.T) {
		checkReferenceAdmissionLifecycle(t, cfg, pool, cache, redisServer, credential, func(ctx context.Context, granted bool) error {
			if granted {
				return users.SetRoles(ctx, user.ID, []int64{role.ID})
			}
			return users.SetRoles(ctx, user.ID, nil)
		})
	})
}
