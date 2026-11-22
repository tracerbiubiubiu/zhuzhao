//go:build integration

package service_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func c04Grant(t *testing.T, f *createC04Fixture, actor int64, path string) {
	t.Helper()
	ctx := context.Background()
	_, err := testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2)VALUES('p',$1,$2,'POST') ON CONFLICT DO NOTHING`, "role::"+f.code, path)
	require.NoError(t, err)
	_, err = f.enforcer.AddPolicy("role::"+f.code, path, "POST")
	require.NoError(t, err)
	if actor != f.actor {
		_, err = testPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id)VALUES($1,$2) ON CONFLICT DO NOTHING`, actor, f.role)
		require.NoError(t, err)
	}
}

func c04PAT(t *testing.T, actor int64) int64 {
	t.Helper()
	var id int64
	require.NoError(t, testPool.QueryRow(context.Background(), `INSERT INTO personal_access_tokens(user_id,name,secret_hash)VALUES($1,'fixture',$2)RETURNING id`, actor, "c04pat_"+uniqueSuffix()).Scan(&id))
	return id
}

func insertC04OrderingTicket(t *testing.T, ctx context.Context, f *createC04Fixture, status string, assignee *int64) int64 {
	t.Helper()
	// The shared create fixture inserts its fields directly. Allocate their
	// stable identities before publishing the current graph for this ticket.
	_, err := testPool.Exec(ctx, `INSERT INTO ticket_field_identities(type_code,field_key)
		SELECT type_code,field_key FROM ticket_type_fields WHERE type_code=$1
		ON CONFLICT(type_code,field_key) WHERE retired_at IS NULL DO NOTHING`, f.code)
	require.NoError(t, err)
	definition := publishC04FixtureDefinition(t, ctx, f.code, f.actor, nil)
	return insertC04BoundTicket(t, ctx, f.code, "work", status, f.actor, f.org, assignee, definition)
}

func c04HTTP(ctx context.Context, f *createC04Fixture, actor int64, kind string, credential any, path, body string, before gin.HandlerFunc, action gin.HandlerFunc) *httptest.ResponseRecorder {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", actor)
		c.Set(kind, credential)
		c.Next()
	}, middleware.CasbinAuth(f.enforcer, f.rbac, nil))
	if before != nil {
		r.POST(path, before, action)
	} else {
		r.POST(path, action)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Snapshot includes the entire ticket and its durable business children, not
// just the status: denial must preserve assignee/SLA/timestamps and audit data.
func c04BusinessSnapshot(t *testing.T, id int64) string {
	t.Helper()
	var snapshot string
	require.NoError(t, testPool.QueryRow(context.Background(), `SELECT jsonb_build_object(
 'ticket',to_jsonb(t),
 'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM ticket_events e WHERE ticket_id=t.id),
 'comments',(SELECT jsonb_agg(to_jsonb(c) ORDER BY c.id) FROM ticket_comments c WHERE ticket_id=t.id),
 'reminders',(SELECT jsonb_agg(to_jsonb(r)) FROM ticket_reminders r WHERE ticket_id=t.id),
 'cc',(SELECT jsonb_agg(to_jsonb(c)) FROM ticket_cc c WHERE ticket_id=t.id))::text
 FROM tickets t WHERE id=$1`, id).Scan(&snapshot))
	return snapshot
}

// Changing assignment after HTTP admission must invalidate the old assignee's
// start command even though the same credential, L1 and read visibility survive.
func TestC04_HTTPCurrentAssignee(t *testing.T) {
	for _, order := range []string{"assign_first", "start_first"} {
		t.Run(order, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			f := newCreateC04(t, nil)
			var old, current int64
			for i, dst := range []*int64{&old, &current} {
				require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO users(username,password)VALUES($1,'fixture')RETURNING id`, fmt.Sprintf("%s_%d", f.code, i)).Scan(dst))
				_, err := testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id)VALUES($1,$2)`, *dst, f.org)
				require.NoError(t, err)
				c04Grant(t, f, *dst, "/api/v1/tickets/transition")
			}
			c04Grant(t, f, f.actor, "/api/v1/tickets/assign")
			_, err := testPool.Exec(ctx, `UPDATE user_orgs SET org_member_role='admin' WHERE user_id=$1`, f.actor)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `UPDATE ticket_types SET transition_meta='{"assigned>in_progress":{"roles":["assignee"]}}' WHERE code=$1`, f.code)
			require.NoError(t, err)
			id := insertC04OrderingTicket(t, ctx, f, "assigned", &old)
			h := handler.NewTicketHandler(f.svc, nil, nil)
			oldPAT, newPAT, superPAT := c04PAT(t, old), c04PAT(t, current), c04PAT(t, f.actor)
			body := fmt.Sprintf(`{"id":"%d","to":"in_progress","comment":"start"}`, id)
			before := ""
			assign := func() {
				w := c04HTTP(ctx, f, f.actor, "pat_id", superPAT, "/api/v1/tickets/assign", fmt.Sprintf(`{"id":"%d","assigned_to":"%d"}`, id, current), nil, h.Assign)
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			}
			var admitted bool
			if order == "assign_first" {
				_, err = f.svc.Get(ctx, id, old)
				require.NoError(t, err)
				w := c04HTTP(ctx, f, old, "pat_id", oldPAT, "/api/v1/tickets/transition", body, func(c *gin.Context) {
					admitted = true
					assign()
					_, err = f.svc.Get(ctx, id, old)
					require.NoError(t, err, "old assignee remains readable")
					before = c04BusinessSnapshot(t, id)
					c.Next()
				}, h.Transition)
				require.True(t, admitted, "old assignee must pass L1 admission")
				require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
				require.Equal(t, before, c04BusinessSnapshot(t, id), "denial has zero business writes")
				w = c04HTTP(ctx, f, current, "pat_id", newPAT, "/api/v1/tickets/transition", body, nil, h.Transition)
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			} else {
				w := c04HTTP(ctx, f, old, "pat_id", oldPAT, "/api/v1/tickets/transition", body, nil, h.Transition)
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				assign()
			}
			var status string
			var assignee int64
			var starts, comments int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT status,assigned_to,(SELECT count(*) FROM ticket_events WHERE ticket_id=$1 AND action='status_changed'),(SELECT count(*) FROM ticket_comments WHERE ticket_id=$1) FROM tickets WHERE id=$1`, id).Scan(&status, &assignee, &starts, &comments))
			require.Equal(t, "in_progress", status)
			require.Equal(t, current, assignee)
			require.Equal(t, 1, starts)
			require.Equal(t, 1, comments)
		})
	}
}

// Both orderings run real HTTP handlers and the shared database IAM barrier.
// A second pool makes the PostgreSQL lock observable, without timing the Go gate.
func TestC04_HTTPCredentialCommitOrdering(t *testing.T) {
	for _, command := range []string{"transition", "assign"} {
		for _, kind := range []string{"session_epoch", "pat_id"} {
			for _, order := range []string{"revoke_first", "commit_first"} {
				t.Run(command+"/"+kind+"/"+order, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					f := newCreateC04(t, nil)
					_, err := testPool.Exec(ctx, `UPDATE user_orgs SET org_member_role='admin' WHERE user_id=$1`, f.actor)
					require.NoError(t, err)
					path := "/api/v1/tickets/" + command
					c04Grant(t, f, f.actor, path)
					id := insertC04OrderingTicket(t, ctx, f, "open", nil)
					h := handler.NewTicketHandler(f.svc, nil, nil)
					action := h.Transition
					body := fmt.Sprintf(`{"id":"%d","to":"closed","comment":"done"}`, id)
					if command == "assign" {
						action = h.Assign
						body = fmt.Sprintf(`{"id":"%d","assigned_to":"%d"}`, id, f.actor)
					}
					var credential int64
					if kind == "pat_id" {
						credential = c04PAT(t, f.actor)
					} else {
						require.NoError(t, testPool.QueryRow(ctx, `SELECT session_epoch FROM users WHERE id=$1`, f.actor).Scan(&credential))
					}
					cfg, err := pgxpool.ParseConfig(testPool.Config().ConnString())
					require.NoError(t, err)
					tr := &c04Trace{pids: make(map[uint32]bool), attempt: make(chan struct{}, 1)}
					cfg.ConnConfig.Tracer = tr
					pool, err := pgxpool.NewWithConfig(ctx, cfg)
					require.NoError(t, err)
					defer pool.Close()
					revoke := func() error {
						if kind == "pat_id" {
							return repository.NewPatRepo(pool).Revoke(ctx, credential, f.actor)
						}
						return repository.WithIAMGuard(ctx, pool, func(ctx context.Context) error {
							_, e := repository.IAMDB(ctx, pool).Exec(ctx, `UPDATE users SET session_epoch=session_epoch+1 WHERE id=$1`, f.actor)
							return e
						})
					}
					before := c04BusinessSnapshot(t, id)
					done := make(chan *httptest.ResponseRecorder, 1)
					revoked := make(chan error, 1)
					ready, release := make(chan struct{}), make(chan struct{})
					var releaseOnce sync.Once
					unblock := func() { releaseOnce.Do(func() { close(release) }) }
					defer unblock()
					if order == "revoke_first" {
						go func() {
							revoked <- repository.WithIAMGuard(ctx, pool, func(guarded context.Context) error {
								close(ready)
								select {
								case <-release:
								case <-ctx.Done():
									return ctx.Err()
								}
								if kind == "pat_id" {
									return repository.NewPatRepo(pool).Revoke(guarded, credential, f.actor)
								}
								_, e := repository.IAMDB(guarded, pool).Exec(guarded, `UPDATE users SET session_epoch=session_epoch+1 WHERE id=$1`, f.actor)
								return e
							})
						}()
						select {
						case <-ready:
						case <-ctx.Done():
							t.Fatal(ctx.Err())
						}
						f.trace.attempt = make(chan struct{}, 1)
						go func() { done <- c04HTTP(ctx, f, f.actor, kind, credential, path, body, nil, action) }()
						select {
						case <-f.trace.attempt:
						case <-ctx.Done():
							t.Fatal(ctx.Err())
						}
						unblock()
						require.NoError(t, <-revoked)
						w := <-done
						require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
						require.Equal(t, before, c04BusinessSnapshot(t, id))
					} else {
						var once sync.Once
						f.trace.beforeWrite = func(context.Context) {
							once.Do(func() {
								close(ready)
								select {
								case <-release:
								case <-ctx.Done():
								}
							})
						}
						go func() { done <- c04HTTP(ctx, f, f.actor, kind, credential, path, body, nil, action) }()
						select {
						case <-ready:
						case <-ctx.Done():
							t.Fatal(ctx.Err())
						}
						go func() { revoked <- revoke() }()
						select {
						case <-tr.attempt:
						case <-ctx.Done():
							t.Fatal(ctx.Err())
						}
						select {
						case e := <-revoked:
							t.Fatalf("revocation overtook admitted command: %v", e)
						case <-time.After(100 * time.Millisecond):
						}
						unblock()
						w := <-done
						require.Equal(t, http.StatusOK, w.Code, w.Body.String())
						require.NoError(t, <-revoked)
						require.NotEqual(t, before, c04BusinessSnapshot(t, id))
						// The same admitted credential cannot issue the next command after revocation.
						before = c04BusinessSnapshot(t, id)
						w = c04HTTP(ctx, f, f.actor, kind, credential, path, body, nil, action)
						require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
						require.Equal(t, before, c04BusinessSnapshot(t, id))
					}
				})
			}
		}
	}
}

// Credentials are captured at admission; later Gin values cannot replace the
// original JWT generation, including zero. Malformed facts are never coerced.
func TestC04_HTTPJWTGenerationFacts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fact    any
		current int64
		want    int
	}{
		{"legacy_zero", int64(0), 1, 403}, {"current", int64(3), 3, 200}, {"old", int64(1), 2, 403}, {"future", int64(3), 2, 403}, {"negative", int64(-1), 1, 403}, {"wrong_type", int(1), 1, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCreateC04(t, nil)
			ctx := context.Background()
			_, err := testPool.Exec(ctx, `UPDATE users SET session_epoch=$2 WHERE id=$1`, f.actor, tc.current)
			require.NoError(t, err)
			c04Grant(t, f, f.actor, "/api/v1/tickets/transition")
			id := insertC04OrderingTicket(t, ctx, f, "open", nil)
			before := c04BusinessSnapshot(t, id)
			w := c04HTTP(ctx, f, f.actor, "session_epoch", tc.fact, "/api/v1/tickets/transition", fmt.Sprintf(`{"id":"%d","to":"closed"}`, id), func(c *gin.Context) { c.Set("session_epoch", tc.current); c.Next() }, handler.NewTicketHandler(f.svc, nil, nil).Transition)
			require.Equal(t, tc.want, w.Code, w.Body.String())
			if tc.want == 403 {
				require.Equal(t, before, c04BusinessSnapshot(t, id))
			}
		})
	}
}

// Real successful JWT/PAT middleware must provide the credential fact consumed
// by Casbin's purpose, and that exact admitted credential is rechecked on write.
func TestC04_HTTPProductionCredentialPurpose(t *testing.T) {
	for _, command := range []string{"transition", "assign"} {
		for _, kind := range []string{"jwt", "pat"} {
			for _, change := range []string{"allow", "missing_fact", "revoked"} {
				t.Run(command+"/"+kind+"/"+change, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					f := newCreateC04(t, nil)
					path := "/api/v1/tickets/" + command
					c04Grant(t, f, f.actor, path)
					_, err := testPool.Exec(ctx, `UPDATE user_orgs SET org_member_role='admin' WHERE user_id=$1`, f.actor)
					require.NoError(t, err)
					var epoch int64
					require.NoError(t, testPool.QueryRow(ctx, `SELECT session_epoch FROM users WHERE id=$1`, f.actor).Scan(&epoch))
					_, err = testPool.Exec(ctx, `UPDATE users SET session_projection_epoch=session_epoch WHERE id=$1`, f.actor)
					require.NoError(t, err)
					id := insertC04OrderingTicket(t, ctx, f, "open", nil)
					mr := miniredis.RunT(t)
					rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
					defer rdb.Close()
					require.NoError(t, rdb.Set(ctx, fmt.Sprintf("user:session_epoch:db1:%d", f.actor), epoch, 0).Err())
					manager := jwtpkg.NewManager(jwtpkg.Config{Secret: "c04-synthetic-signing-key-0123456789", AccessTTL: time.Minute})
					token, _, err := manager.GenerateAccessTokenWithSessionEpoch(f.actor, f.code, false, 0, epoch)
					require.NoError(t, err)
					var patID int64
					if kind == "pat" {
						token = "zpat_c04_synthetic_" + uniqueSuffix()
						require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO personal_access_tokens(user_id,name,secret_hash)VALUES($1,'fixture',$2)RETURNING id`, f.actor, middleware.HashToken(token)).Scan(&patID))
					}
					r := gin.New()
					admitted := false
					security := middleware.WithAccountSecurity(c04AccountSnapshotSource{lookup: repository.NewAccountSecurityLookup(testPool)})
					r.Use(middleware.JWT(manager, rdb, repository.NewPatRepo(testPool), security), func(c *gin.Context) {
						if change == "missing_fact" {
							c.Set("session_epoch", nil)
							c.Set("pat_id", nil)
						}
						c.Next()
					}, middleware.CasbinAuth(f.enforcer, f.rbac, nil))
					h := handler.NewTicketHandler(f.svc, nil, nil)
					action := h.Transition
					body := fmt.Sprintf(`{"id":"%d","to":"closed","comment":"done"}`, id)
					if command == "assign" {
						action = h.Assign
						body = fmt.Sprintf(`{"id":"%d","assigned_to":"%d"}`, id, f.actor)
					}
					before := ""
					r.POST(path, func(c *gin.Context) {
						admitted = true
						if change == "revoked" {
							if kind == "pat" {
								require.NoError(t, repository.NewPatRepo(testPool).Revoke(ctx, patID, f.actor))
							} else {
								require.NoError(t, repository.WithIAMGuard(ctx, testPool, func(guarded context.Context) error {
									_, e := repository.IAMDB(guarded, testPool).Exec(guarded, `UPDATE users SET session_epoch=session_epoch+1 WHERE id=$1`, f.actor)
									return e
								}))
							}
						}
						before = c04BusinessSnapshot(t, id)
						c.Next()
					}, action)
					req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body)).WithContext(ctx)
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer "+token)
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
					require.True(t, admitted, "fixture must pass real authentication and L1")
					want := http.StatusForbidden
					if change == "allow" {
						want = http.StatusOK
					}
					require.Equal(t, want, w.Code, w.Body.String())
					if change != "allow" {
						require.Equal(t, before, c04BusinessSnapshot(t, id))
					}
				})
			}
		}
	}
}

type c04AccountSnapshotSource struct {
	lookup *repository.AccountSecurityLookup
}

func (source c04AccountSnapshotSource) AccountSnapshot(ctx context.Context, userID int64) (middleware.AccountSnapshot, error) {
	snap, err := source.lookup.AccountSecuritySnapshot(ctx, userID)
	return middleware.AccountSnapshot{Status: snap.Status, Deleted: snap.Deleted, SessionEpoch: snap.SessionEpoch,
		ProjectionEpoch: snap.ProjectionEpoch}, err
}
