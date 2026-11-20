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
	for _, command := range []string{"update", "transition"} {
		for _, change := range []string{"allow", "policy", "role", "account", "membership", "l3", "hidden", "cc", "deleted", "org_allow", "org_revoke", "expired", "inherit_allow", "inherit_revoke", "wrong_route", "wrong_actor", "rollback", "post"} {
			if command == "update" && change == "post" {
				continue
			}
			t.Run(command+"/"+change, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				code := "c04_" + uniqueSuffix()
				var actor, other, role, org, id int64
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
				creator := actor
				if change == "membership" || change == "l3" || change == "hidden" || change == "cc" {
					creator = other
				}
				require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO tickets(type_code,title,status,created_by,org_id,org_path)VALUES($1::text,'before','open',$2,$3,$1::text::ltree)RETURNING id`, code, creator, org).Scan(&id))
				path := "/api/v1/tickets/" + command
				if change == "wrong_route" {
					path += "/other"
				}
				_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2)VALUES('p',$1,$2,'POST')`, "role::"+code, path)
				require.NoError(t, err)
				cfg, err := pgxpool.ParseConfig(testPool.Config().ConnString())
				require.NoError(t, err)
				cfg.MaxConns = 1
				trace := &c04Trace{pids: make(map[uint32]bool)}
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
				if change == "post" {
					_, err = testPool.Exec(ctx, `UPDATE ticket_types SET transition_meta='{"open>closed":{"post_actions":["c04_probe"]}}'::jsonb WHERE code=$1`, code)
					require.NoError(t, err)
					post = &c04PostProbe{svc: svc}
					reg := ticketsvc.NewActionRegistry()
					require.NoError(t, reg.Register(post))
					svc.SetActionRegistry(reg)
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
				want := http.StatusForbidden
				if change == "allow" || change == "org_allow" || change == "inherit_allow" || change == "post" {
					want = http.StatusOK
				}
				if change == "rollback" {
					want = http.StatusInternalServerError
				}
				if change == "hidden" || change == "membership" {
					want = http.StatusNotFound
				}
				r := gin.New()
				r.Use(func(c *gin.Context) { c.Set("userID", actor); c.Next() }, middleware.CasbinAuth(enforcer, rbac, nil))
				r.POST(path, func(c *gin.Context) {
					err := repository.WithIAMGuard(ctx, testPool, func(ctx context.Context) error {
						q := repository.IAMDB(ctx, testPool)
						var err error
						switch change {
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
							_, err = q.Exec(ctx, `UPDATE user_orgs SET org_member_role='member',ticket_scope='all' WHERE user_id=$1`, actor)
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
				require.Equal(t, want, w.Code, w.Body.String())
				var title, status string
				var events, comments int
				require.NoError(t, testPool.QueryRow(ctx, `SELECT title,status,(SELECT count(*) FROM ticket_events WHERE ticket_id=$1),(SELECT count(*) FROM ticket_comments WHERE ticket_id=$1)FROM tickets WHERE id=$1`, id).Scan(&title, &status, &events, &comments))
				if want != http.StatusOK {
					require.Equal(t, "before", title)
					require.Equal(t, "open", status)
					require.Zero(t, events)
					require.Zero(t, comments)
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
				if change != "wrong_route" && change != "wrong_actor" {
					require.Len(t, trace.pids, 1, "fresh IAM reads, L2/L3 and writes use one connection")
				}
				require.Zero(t, pool.Stat().CanceledAcquireCount(), "authorization and write cannot acquire a second connection")
			})
		}
	}
}

// Observe production query execution, not an authorization stub.
type c04Trace struct {
	mu   sync.Mutex
	pids map[uint32]bool
}

func (tr *c04Trace) TraceQueryStart(ctx context.Context, conn *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	if repository.IAMGuardActive(ctx) {
		tr.mu.Lock()
		tr.pids[conn.PgConn().PID()] = true
		tr.mu.Unlock()
	}
	return ctx
}
func (*c04Trace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

type c04PostProbe struct {
	svc *ticketsvc.Service
	err error
}

func (*c04PostProbe) Name() string                          { return "c04_probe" }
func (*c04PostProbe) Delivery() ticketsvc.DeliverySemantics { return ticketsvc.DeliveryAtMostOnce }
func (p *c04PostProbe) Execute(ctx context.Context, ac ticketsvc.ActionContext) error {
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
