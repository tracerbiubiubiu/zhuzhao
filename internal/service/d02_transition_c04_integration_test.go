//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestD02_HTTPVersionedTransitionRechecksCurrentAssignee(t *testing.T) {
	const (
		transitionPath = "/api/v1/tickets/transition"
		assignPath     = "/api/v1/tickets/assign"
		updatePath     = "/api/v1/tickets/update"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	f := newCreateC04(t, nil)
	var old, current int64
	for i, id := range []*int64{&old, &current} {
		name := fmt.Sprintf("%s_d02_%d", f.code, i)
		require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO users(username,password)VALUES($1,'fixture') RETURNING id`, name).Scan(id))
		_, err := testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES($1,$2)`, *id, f.org)
		require.NoError(t, err)
		c04Grant(t, f, *id, transitionPath)
	}
	c04Grant(t, f, f.actor, assignPath)
	c04Grant(t, f, f.actor, updatePath)
	_, err := testPool.Exec(ctx, `UPDATE user_orgs SET org_member_role='admin' WHERE user_id=$1`, f.actor)
	require.NoError(t, err)
	_, err = f.svc.UpdateTicketType(ctx, f.code, &model.UpdateTicketTypeRequest{
		TransitionMeta: json.RawMessage(`{"assigned>in_progress":{"roles":["assignee"],"collect_fields":[{"key":"source","required":true}]}}`),
	})
	require.NoError(t, err)
	publishCreateD01(t, f)
	draft, err := repository.NewTicketRepo(testPool).GetTicketType(ctx, f.code)
	require.NoError(t, err)
	require.NotNil(t, draft.CurrentPublishedVersion)
	version := *draft.CurrentPublishedVersion
	// Keep the old assignee as creator so reassignment leaves general transition
	// permission intact. It is the assignee-only action condition that must deny.
	id := insertC04BoundTicket(t, ctx, f.code, "D02 C04 fixture", "assigned", old, f.org, &old, version)

	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })
	manager := jwtpkg.NewManager(jwtpkg.Config{Secret: "d02-c04-synthetic-key-0123456789", AccessTTL: time.Minute})
	credential := func(actor int64) string {
		t.Helper()
		var epoch int64
		require.NoError(t, testPool.QueryRow(ctx, `UPDATE users SET session_projection_epoch=session_epoch WHERE id=$1 RETURNING session_epoch`, actor).Scan(&epoch))
		require.NoError(t, rdb.Set(ctx, fmt.Sprintf("user:session_epoch:db1:%d", actor), epoch, 0).Err())
		token, _, err := manager.GenerateAccessTokenWithSessionEpoch(actor, f.code, false, 0, epoch)
		require.NoError(t, err)
		return token
	}
	oldToken, currentToken, adminToken := credential(old), credential(current), credential(f.actor)
	r := gin.New()
	security := middleware.WithAccountSecurity(c04AccountSnapshotSource{lookup: repository.NewAccountSecurityLookup(testPool)})
	r.Use(middleware.JWT(manager, rdb, repository.NewPatRepo(testPool), security), middleware.CasbinAuth(f.enforcer, f.rbac, nil))
	h := handler.NewTicketHandler(f.svc, nil, nil)
	r.POST(assignPath, h.Assign)
	r.POST(updatePath, h.Update)
	var afterAdmission func()
	r.POST(transitionPath, func(c *gin.Context) {
		if afterAdmission != nil {
			afterAdmission()
		}
		c.Next()
	}, h.Transition)
	post := func(path, token, key, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	revision := func() int64 {
		t.Helper()
		var rev int64
		require.NoError(t, testPool.QueryRow(ctx, `SELECT revision FROM tickets WHERE id=$1`, id).Scan(&rev))
		return rev
	}
	body := func(rev int64, evidence string) string {
		return fmt.Sprintf(`{"id":"%d","to":"in_progress","expected_revision":"%d","definition_version":"%d","comment":"ready","custom_data_patch":{"source":%q}}`, id, rev, version, evidence)
	}
	staleRevision := revision()
	var assigned string
	afterAdmission = func() {
		reassigned := post(assignPath, adminToken, "", fmt.Sprintf(`{"id":"%d","assigned_to":"%d"}`, id, current))
		require.Equal(t, http.StatusOK, reassigned.Code, reassigned.Body.String())
		assigned = c04BusinessSnapshot(t, id)
	}
	w := post(transitionPath, oldToken, "stale-d02-"+f.code, body(staleRevision, "stale evidence"))
	afterAdmission = nil
	require.NotEmpty(t, assigned, "original request must pass real authentication and L1 admission")
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Equal(t, assigned, c04BusinessSnapshot(t, id), "denial must preserve the reassigned ticket, evidence and durable children")
	var receipts int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_evidence_transition_receipts WHERE ticket_id=$1`, id).Scan(&receipts))
	require.Zero(t, receipts)

	freshRevision := revision()
	require.Greater(t, freshRevision, staleRevision)
	key, payload := "commit-d02-"+f.code, body(freshRevision, "current evidence")
	committed := post(transitionPath, currentToken, key, payload)
	require.Equal(t, http.StatusOK, committed.Code, committed.Body.String())
	var status, evidence, savedKey string
	var savedActor, savedRevision, savedVersion int64
	var events, comments int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT t.status,t.custom_data->>'source',r.idempotency_key,r.actor_id,r.result_revision,r.result_definition_version,
		(SELECT count(*) FROM ticket_events WHERE ticket_id=t.id AND action='status_changed' AND user_id=$2),
		(SELECT count(*) FROM ticket_comments WHERE ticket_id=t.id AND content='ready' AND user_id=$2)
		FROM tickets t JOIN ticket_evidence_transition_receipts r ON r.ticket_id=t.id WHERE t.id=$1`, id, current).
		Scan(&status, &evidence, &savedKey, &savedActor, &savedRevision, &savedVersion, &events, &comments))
	require.Equal(t, "in_progress", status)
	require.Equal(t, "current evidence", evidence)
	require.Equal(t, key, savedKey)
	require.Equal(t, current, savedActor)
	require.Equal(t, freshRevision+1, savedRevision)
	require.Equal(t, version, savedVersion)
	require.Equal(t, 1, events)
	require.Equal(t, 1, comments)

	edited := post(updatePath, adminToken, "", fmt.Sprintf(`{"id":"%d","title":"Later edit"}`, id))
	require.Equal(t, http.StatusOK, edited.Code, edited.Body.String())
	require.Greater(t, revision(), savedRevision)
	beforeReplay := c04BusinessSnapshot(t, id)
	replay := post(transitionPath, currentToken, key, payload)
	require.Equal(t, http.StatusOK, replay.Code, replay.Body.String())
	require.JSONEq(t, committed.Body.String(), replay.Body.String(), "retry must return the original result rather than the intervening edit")
	require.Equal(t, beforeReplay, c04BusinessSnapshot(t, id))
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_evidence_transition_receipts WHERE ticket_id=$1`, id).Scan(&receipts))
	require.Equal(t, 1, receipts)
}
