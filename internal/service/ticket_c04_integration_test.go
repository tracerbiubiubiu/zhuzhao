//go:build integration

package service_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	ticketsvc "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// These fixtures are owned exclusively by the package's testcontainers PG.
// Revoking after middleware admission must invalidate the entire command,
// including its durable event/comment, even with a stale in-memory enforcer.
func TestC04_HTTPFreshAuthorization(t *testing.T) {
	for _, command := range []string{"update", "transition", "close", "assign", "comments", "notes", "relations", "delete", "cc", "cc/remove", "tags", "tags/remove"} {
		for _, change := range []string{"allow", "policy", "role", "account", "membership", "l3", "hidden", "cc", "deleted", "org_allow", "org_revoke", "expired", "inherit_allow", "inherit_revoke", "wrong_route", "wrong_actor", "rollback", "post", "concurrent_account", "allow_reverse", "target_hidden", "target_l3", "duplicate_reverse", "closed_reply", "post_ignored", "post_builtin", "credential_missing", "pat_revoked", "pat_expired", "pat_owner", "pat_removed", "pat_wrong_type", "credential_ambiguous"} {
			if command != "transition" && change == "post" {
				continue
			}
			if (change == "allow_reverse" || change == "target_hidden" || change == "target_l3" || change == "duplicate_reverse") && command != "relations" {
				continue
			}
			if change == "closed_reply" && command != "comments" && command != "notes" {
				continue
			}
			if change == "post_ignored" && command != "close" && command != "assign" {
				continue
			}
			if change == "post_builtin" && command != "transition" {
				continue
			}
			t.Run(command+"/"+change, func(t *testing.T) {
				selfService := command == "cc" || command == "cc/remove" || command == "tags" || command == "tags/remove"
				if selfService && change == "policy" {
					return
				} // self-service has no route policy
				if (command == "comments" || command == "cc" || command == "cc/remove" || command == "tags" || command == "tags/remove") && (change == "l3" || change == "cc") {
					return
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				code := "c04_" + uniqueSuffix()
				var actor, other, role, org int64
				require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO users(username,password)VALUES($1,'fixture')RETURNING id`, code).Scan(&actor))
				require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO users(username,password)VALUES($1,'fixture')RETURNING id`, code+"_other").Scan(&other))
				require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO roles(code,name,priority)VALUES($1::text,$1::text,50)RETURNING id`, code).Scan(&role))
				require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,path,ticket_visibility)VALUES($1::text,$1::text,$1::text::ltree,'project_isolated')RETURNING id`, code).Scan(&org))
				_, err := testPool.Exec(ctx, `INSERT INTO ticket_types(code,name)VALUES($1::text,$1::text)`, code)
				require.NoError(t, err)
				_, err = testPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id)VALUES($1,$2)`, actor, role)
				require.NoError(t, err)
				_, err = testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id,org_member_role)VALUES($1,$2,'admin')`, actor, org)
				require.NoError(t, err)
				if change == "org_allow" || change == "org_revoke" || change == "expired" {
					_, err = testPool.Exec(ctx, `DELETE FROM user_roles WHERE user_id=$1`, actor)
					require.NoError(t, err)
					_, err = testPool.Exec(ctx, `INSERT INTO org_roles(org_id,role_id)VALUES($1,$2)`, org, role)
					require.NoError(t, err)
				}
				if change == "inherit_allow" || change == "inherit_revoke" {
					var child int64
					require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO roles(code,name,priority,parent_id)VALUES($1::text,$1::text,60,$2)RETURNING id`, code+"_child", role).Scan(&child))
					_, err = testPool.Exec(ctx, `UPDATE user_roles SET role_id=$2 WHERE user_id=$1`, actor, child)
					require.NoError(t, err)
				}
				path := "/api/v1/tickets/" + command
				if change == "wrong_route" {
					path += "/other"
				}
				_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2)VALUES('p',$1,$2,'POST')`, "role::"+code, path)
				require.NoError(t, err)
				cfg, err := pgxpool.ParseConfig(testPool.Config().ConnString())
				require.NoError(t, err)
				cfg.MaxConns = 1
				trace := &c04Trace{pids: make(map[uint32]bool), attempt: make(chan struct{}, 1)}
				cfg.ConnConfig.Tracer = trace
				pool, err := pgxpool.NewWithConfig(ctx, cfg)
				require.NoError(t, err)
				defer pool.Close()
				m, err := casbinmodel.NewModelFromFile("../../configs/casbin_model.conf")
				require.NoError(t, err)
				enforcer, err := casbin.NewSyncedEnforcer(m)
				require.NoError(t, err)
				_, err = enforcer.AddPolicy("role::"+code, path, "POST")
				require.NoError(t, err)
				rbac := service.NewRBACService(repository.NewRoleRepo(pool), repository.NewUserRepo(pool), repository.NewMenuRepo(pool), enforcer)
				svc := ticketsvc.NewTicketService(pool, repository.NewTicketRepo(pool), repository.NewOrgRepo(pool), resource.NewRegistry(), rbac, service.NewOrgDelegationService(pool), nil, nil, nil)
				var post *c04PostProbe
				var actionReg *ticketsvc.ActionRegistry
				if change == "post" || change == "post_ignored" {
					_, err = testPool.Exec(ctx, `UPDATE ticket_types SET transition_meta='{"open>closed":{"post_actions":["c04_probe"]},"open>assigned":{"post_actions":["c04_probe"]}}'::jsonb WHERE code=$1`, code)
					require.NoError(t, err)
					post = &c04PostProbe{svc: svc}
					actionReg = ticketsvc.NewActionRegistry()
					require.NoError(t, actionReg.Register(post))
				}
				if change == "post_builtin" {
					_, err = testPool.Exec(ctx, `UPDATE ticket_types SET transition_meta=$2::jsonb WHERE code=$1`, code, fmt.Sprintf(`{"open>assigned":{"post_actions":[{"name":"cc","params":{"user_ids":[%d]}},{"name":"reassign","params":{"user_id":%d}}]}}`, other, other))
					require.NoError(t, err)
					actionReg = ticketsvc.NewActionRegistry()
					require.NoError(t, actionReg.Register(ticketsvc.NewCCAction(svc)))
					require.NoError(t, actionReg.Register(ticketsvc.NewReassignAction(svc)))
				}
				if change == "rollback" && (command == "relations" || selfService) {
					return
				}
				svc.SetActionRegistry(actionReg)
				definition := publishC04FixtureDefinition(t, ctx, code, actor, actionReg)
				creator := actor
				if change == "membership" || change == "l3" || change == "hidden" || change == "cc" {
					creator = other
				}
				id := insertC04BoundTicket(t, ctx, code, "before", "open", creator, org, nil, definition)
				target := insertC04BoundTicket(t, ctx, code, "target", "open", creator, org, nil, definition)
				if command == "cc/remove" {
					_, err = testPool.Exec(ctx, `INSERT INTO ticket_cc(ticket_id,user_id,created_by)VALUES($1,$2,$2)`, id, actor)
					require.NoError(t, err)
				}
				if command == "tags/remove" {
					_, err = testPool.Exec(ctx, `INSERT INTO tags(entity_type,entity_id,tag,created_by)VALUES('ticket',$1,'fixture',$2)`, id, actor)
					require.NoError(t, err)
				}
				if change == "closed_reply" {
					_, err = testPool.Exec(ctx, `UPDATE tickets SET status='closed' WHERE id=$1`, id)
					require.NoError(t, err)
				}
				if change == "target_hidden" {
					var hiddenOrg int64
					require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,path,ticket_visibility)VALUES($1::text,$1::text,$1::text::ltree,'project_isolated')RETURNING id`, code+"hidden").Scan(&hiddenOrg))
					_, err = testPool.Exec(ctx, `UPDATE tickets SET created_by=$2,org_id=$3,org_path=$4::ltree WHERE id=$1`, target, other, hiddenOrg, code+"hidden")
					require.NoError(t, err)
				}
				if change == "target_l3" {
					_, err = testPool.Exec(ctx, `UPDATE tickets SET created_by=$2 WHERE id=$1`, target, other)
					require.NoError(t, err)
					_, err = testPool.Exec(ctx, `UPDATE user_orgs SET org_member_role='member',ticket_scope='all' WHERE user_id=$1`, actor)
					require.NoError(t, err)
				}
				if change == "duplicate_reverse" {
					_, err = testPool.Exec(ctx, `INSERT INTO ticket_relations(source_ticket_id,target_ticket_id,relation_type,created_by)VALUES($1,$2,'related',$3)`, target, id, actor)
					require.NoError(t, err)
				}
				if change == "rollback" {
					constraint := code + "_event"
					_, err = testPool.Exec(ctx, fmt.Sprintf(`ALTER TABLE ticket_events ADD CONSTRAINT %s CHECK(ticket_id<>%d) NOT VALID`, constraint, id))
					require.NoError(t, err)
					t.Cleanup(func() {
						_, err := testPool.Exec(context.Background(), `ALTER TABLE ticket_events DROP CONSTRAINT `+constraint)
						require.NoError(t, err)
					})
				}
				h := handler.NewTicketHandler(svc, nil, nil)
				action := h.Update
				body := fmt.Sprintf(`{"id":"%d","title":"after"}`, id)
				if command == "transition" {
					action = h.Transition
					body = fmt.Sprintf(`{"id":"%d","to":"closed","comment":"fixture comment"}`, id)
				}
				switch command {
				case "close":
					action = h.Close
					body = fmt.Sprintf(`{"id":"%d","comment":"fixture comment"}`, id)
				case "assign":
					action = h.Assign
					body = fmt.Sprintf(`{"id":"%d","assigned_to":"%d"}`, id, other)
				case "delete":
					action = h.Delete
					body = fmt.Sprintf(`{"id":"%d"}`, id)
				case "comments":
					action = h.CreateComment
					body = fmt.Sprintf(`{"ticket_id":"%d","content":"fixture comment"}`, id)
				case "notes":
					action = h.CreateNote
					body = fmt.Sprintf(`{"ticket_id":"%d","content":"fixture note"}`, id)
				case "relations":
					action = h.CreateRelation
					body = fmt.Sprintf(`{"source_ticket_id":"%d","target_ticket_id":"%d"}`, id, target)
				case "cc":
					action = h.AddCC
					body = fmt.Sprintf(`{"ticket_id":"%d","user_ids":["%d"]}`, id, actor)
				case "cc/remove":
					action = h.RemoveCC
					body = fmt.Sprintf(`{"ticket_id":"%d","user_id":"%d"}`, id, actor)
				case "tags":
					action = h.AddTags
					body = fmt.Sprintf(`{"ticket_id":"%d","tags":["fixture"]}`, id)
				case "tags/remove":
					action = h.RemoveTag
					body = fmt.Sprintf(`{"ticket_id":"%d","tag":"fixture"}`, id)
				}
				if change == "allow_reverse" {
					body = fmt.Sprintf(`{"source_ticket_id":"%d","target_ticket_id":"%d"}`, target, id)
				}
				if change == "post_builtin" {
					body = fmt.Sprintf(`{"id":"%d","to":"assigned","comment":"fixture comment"}`, id)
				}
				want := http.StatusForbidden
				if change == "allow" || change == "org_allow" || change == "inherit_allow" || change == "post" || change == "allow_reverse" || change == "closed_reply" || change == "post_ignored" || change == "post_builtin" {
					want = http.StatusOK
				}
				if change == "rollback" {
					want = http.StatusInternalServerError
				}
				if change == "duplicate_reverse" {
					want = http.StatusConflict
				}
				if change == "hidden" || change == "membership" || change == "target_hidden" {
					want = http.StatusNotFound
				}
				if selfService && change == "inherit_revoke" || command == "cc/remove" && (change == "membership" || change == "hidden") {
					want = http.StatusOK
				}

				var patID int64
				require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO personal_access_tokens(user_id,name,secret_hash)VALUES($1,'fixture',$2)RETURNING id`, actor, code).Scan(&patID))
				r := gin.New()
				r.Use(func(c *gin.Context) {
					c.Set("userID", actor)
					if change != "credential_missing" {
						c.Set("pat_id", patID)
					}
					if change == "pat_wrong_type" {
						c.Set("pat_id", "fixture")
					}
					if change == "credential_ambiguous" {
						c.Set("session_epoch", int64(0))
					}
					c.Next()
				})
				if selfService {
					r.Use(middleware.SelfService())
				}
				r.Use(middleware.CasbinAuth(enforcer, rbac, nil))
				var revokeDone chan error
				r.POST(path, func(c *gin.Context) {
					if change == "concurrent_account" {
						ready, release := make(chan struct{}), make(chan struct{})
						revokeDone = make(chan error, 1)
						go func() {
							revokeDone <- repository.WithIAMGuard(ctx, testPool, func(guarded context.Context) error {
								close(ready)
								<-release
								_, err := repository.IAMDB(guarded, testPool).Exec(guarded, `UPDATE users SET status=0 WHERE id=$1`, actor)
								return err
							})
						}()
						<-ready
						go func() {
							select {
							case <-trace.attempt:
							case <-time.After(time.Second):
							}
							time.Sleep(300 * time.Millisecond)
							close(release)
						}()
						c.Next()
						return
					}

					err := repository.WithIAMGuard(ctx, testPool, func(ctx context.Context) error {
						q := repository.IAMDB(ctx, testPool)
						var err error
						switch change {
						case "pat_revoked":
							return repository.NewPatRepo(testPool).Revoke(ctx, patID, actor)
						case "pat_expired":
							_, err = q.Exec(ctx, `UPDATE personal_access_tokens SET expires_at=NOW()-interval '1 second' WHERE id=$1`, patID)
						case "pat_owner":
							_, err = q.Exec(ctx, `UPDATE personal_access_tokens SET user_id=$2 WHERE id=$1`, patID, other)
						case "pat_removed":
							_, err = q.Exec(ctx, `DELETE FROM personal_access_tokens WHERE id=$1`, patID)
						case "policy":
							_, err = q.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1`, "role::"+code)
						case "role":
							_, err = q.Exec(ctx, `UPDATE roles SET status=0 WHERE id=$1`, role)
						case "org_revoke":
							_, err = q.Exec(ctx, `DELETE FROM org_roles WHERE org_id=$1`, org)
						case "expired":
							_, err = q.Exec(ctx, `UPDATE user_orgs SET expires_at=NOW()-interval '1 second' WHERE user_id=$1`, actor)
						case "inherit_revoke":
							_, err = q.Exec(ctx, `UPDATE roles SET status=0 WHERE id=$1`, role)
						case "deleted":
							_, err = q.Exec(ctx, `UPDATE users SET deleted_at=NOW() WHERE id=$1`, actor)
						case "account":
							_, err = q.Exec(ctx, `UPDATE users SET status=0 WHERE id=$1`, actor)
						case "membership", "hidden", "cc":
							_, err = q.Exec(ctx, `DELETE FROM user_orgs WHERE user_id=$1`, actor)
						case "l3":
							scope := "all"
							if command == "assign" {
								scope = "self"
							}
							_, err = q.Exec(ctx, `UPDATE user_orgs SET org_member_role='member',ticket_scope=$2 WHERE user_id=$1`, actor, scope)
							if err == nil && command == "assign" {
								_, err = q.Exec(ctx, `INSERT INTO ticket_cc(ticket_id,user_id,created_by)VALUES($1,$2,$2)`, id, actor)
							}
						}
						if err == nil && change == "cc" {
							_, err = q.Exec(ctx, `INSERT INTO ticket_cc(ticket_id,user_id,created_by)VALUES($1,$2,$2)`, id, actor)
						}
						return err
					})
					require.NoError(t, err)
					if change == "wrong_actor" {
						c.Set("userID", other)
					}
					c.Next()
				}, action)
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body)).WithContext(ctx)
				req.Header.Set("Content-Type", "application/json")
				r.ServeHTTP(w, req)
				if revokeDone != nil {
					require.NoError(t, <-revokeDone)
				}
				require.Equal(t, want, w.Code, w.Body.String())
				if want == http.StatusOK && command == "delete" {
					var n int
					require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE id=$1`, id).Scan(&n))
					require.Zero(t, n)
					return
				}
				var title, status string
				var events, comments int
				require.NoError(t, testPool.QueryRow(ctx, `SELECT title,status,(SELECT count(*) FROM ticket_events WHERE ticket_id=$1),(SELECT count(*) FROM ticket_comments WHERE ticket_id=$1)FROM tickets WHERE id=$1`, id).Scan(&title, &status, &events, &comments))
				var relations, cc, tags int
				require.NoError(t, testPool.QueryRow(ctx, `SELECT (SELECT count(*) FROM ticket_relations WHERE source_ticket_id=$1 OR target_ticket_id=$1),(SELECT count(*) FROM ticket_cc WHERE ticket_id=$1),(SELECT count(*) FROM tags WHERE entity_type='ticket' AND entity_id=$1)`, id).Scan(&relations, &cc, &tags))
				if command == "relations" {
					if want == http.StatusOK || change == "duplicate_reverse" {
						require.Equal(t, 1, relations)
					} else {
						require.Zero(t, relations)
					}
				}
				if selfService {
					expected := 0
					if (command == "cc" || command == "tags") && want == http.StatusOK || (command == "cc/remove" || command == "tags/remove") && want != http.StatusOK {
						expected = 1
					}
					if command == "cc" || command == "cc/remove" {
						require.Equal(t, expected, cc)
					} else {
						require.Equal(t, expected, tags)
					}
				}
				if want != http.StatusOK {
					require.Equal(t, "before", title)
					require.Equal(t, "open", status)
					require.Zero(t, events)
					require.Zero(t, comments)
				} else if change == "post_builtin" {
					require.Equal(t, "assigned", status)
					require.Equal(t, 3, events)
					require.Equal(t, 1, comments)
					require.Equal(t, 1, cc)
					var assigned int64
					require.NoError(t, testPool.QueryRow(ctx, `SELECT assigned_to FROM tickets WHERE id=$1`, id).Scan(&assigned))
					require.Equal(t, other, assigned)
				} else if command == "assign" {
					require.Equal(t, "assigned", status)
					require.Equal(t, 1, events)
				} else if command == "comments" || command == "notes" {
					require.Equal(t, 1, comments)
					require.Equal(t, 1, events)
				} else if command == "relations" || selfService {
					require.Equal(t, "open", status)
					require.Zero(t, events)
				} else if command == "update" {
					require.Equal(t, "after", title)
					require.Equal(t, 1, events)
				} else if change == "post" {
					require.NoError(t, post.err)
					require.Equal(t, "open", status, "nested command runs after outer commit and local gate release")
					require.Equal(t, 3, events)
					require.Equal(t, 1, comments)
				} else {
					require.Equal(t, "closed", status)
					require.Equal(t, 2, events)
					require.Equal(t, 1, comments)
				}
				if change == "credential_missing" || change == "pat_wrong_type" || change == "credential_ambiguous" {
					require.Empty(t, trace.pids, "invalid credential facts fail before decision reads")
				} else if change != "wrong_route" && change != "wrong_actor" {
					require.Len(t, trace.pids, 1, "fresh IAM reads, L2/L3 and writes use one connection")
				}
				if change == "post_ignored" {
					require.Zero(t, post.calls, "Close/Assign do not execute template post_actions")
				}
				if command == "relations" && want == http.StatusOK {
					require.Equal(t, []int64{id, target}, trace.rowLocks)
				}
				require.Zero(t, pool.Stat().CanceledAcquireCount(), "authorization and write cannot acquire a second connection")
			})
		}
	}
}

func publishC04FixtureDefinition(t *testing.T, ctx context.Context, code string, actor int64, actionReg *ticketsvc.ActionRegistry) int64 {
	t.Helper()
	// Setup uses the untraced pool so only the HTTP command contributes to the
	// authorization, locking and connection assertions.
	repo := repository.NewTicketRepo(testPool)
	svc := ticketsvc.NewTicketService(testPool, repo, repository.NewOrgRepo(testPool), resource.NewRegistry(), nil, nil, nil, nil, nil)
	svc.SetActionRegistry(actionReg)
	draft, err := repo.GetTicketType(ctx, code)
	require.NoError(t, err)
	definition, changed, err := svc.PublishTicketType(ctx, draft, actor)
	require.NoError(t, err)
	require.True(t, changed, "fresh fixture must publish its initial definition")
	return definition.DefinitionVersion
}

func insertC04BoundTicket(t *testing.T, ctx context.Context, code, title, status string, creator, org int64, assignee *int64, definition int64) int64 {
	t.Helper()
	var id int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO tickets
		(type_code,title,status,created_by,org_id,org_path,assigned_to,definition_version)
		VALUES($1::text,$2,$3,$4,$5,$1::text::ltree,$6,$7) RETURNING id`,
		code, title, status, creator, org, assignee, definition).Scan(&id))
	return id
}

// Observe production query execution, not an authorization stub.
type c04Trace struct {
	mu          sync.Mutex
	pids        map[uint32]bool
	attempt     chan struct{}
	rowLocks    []int64
	beforeWrite func(context.Context)
}

func (tr *c04Trace) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if tr.beforeWrite != nil && strings.Contains(data.SQL, "UPDATE tickets") {
		tr.beforeWrite(ctx)
	}
	if strings.Contains(data.SQL, "pg_advisory_lock") {
		select {
		case tr.attempt <- struct{}{}:
		default:
		}
	}
	if strings.Contains(data.SQL, "FOR UPDATE") && strings.Contains(data.SQL, "FROM tickets") && len(data.Args) > 0 {
		if id, ok := data.Args[0].(int64); ok {
			tr.mu.Lock()
			tr.rowLocks = append(tr.rowLocks, id)
			tr.mu.Unlock()
		}
	}
	if repository.IAMGuardActive(ctx) {
		tr.mu.Lock()
		tr.pids[conn.PgConn().PID()] = true
		tr.mu.Unlock()
	}
	return ctx
}
func (*c04Trace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

type c04PostProbe struct {
	svc   *ticketsvc.Service
	err   error
	calls int
}

func (*c04PostProbe) Name() string                          { return "c04_probe" }
func (*c04PostProbe) Delivery() ticketsvc.DeliverySemantics { return ticketsvc.DeliveryAtMostOnce }
func (p *c04PostProbe) Execute(ctx context.Context, ac ticketsvc.ActionContext) error {
	p.calls++
	if repository.IAMGuardActive(ctx) {
		p.err = fmt.Errorf("post action retained transaction context")
		return p.err
	}
	if !ac.SystemAsCreator {
		p.err = fmt.Errorf("automation semantics changed")
		return p.err
	}
	_, p.err = p.svc.Transition(ctx, &model.TransitionTicketRequest{ID: ac.Ticket.ID, To: "open"}, ac.ActorID)
	return p.err
}
