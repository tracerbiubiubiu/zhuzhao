//go:build integration

package ticket

import (
	"context"
	"errors"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// The risk kernel owns the predecessor locks in assembly tests. This port test
// takes those real rows in the documented order, then tests only ticket writes.
func TestRiskWorkflowPublishedActionExactSavepointAndRollback(t *testing.T) {
	for _, tc := range []struct {
		name       string
		rollback   bool
		fieldType  string
		postAction bool
		comment    string
	}{
		{name: "commit"}, {name: "rollback", rollback: true}, {name: "file", fieldType: "file"}, {name: "richtext", fieldType: "richtext"}, {name: "post_actions", postAction: true},
		{name: "comment_file", comment: "![](/api/v1/files/tickets/synthetic-other-ticket.png)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rollback := tc.rollback
			unsupported := tc.fieldType != "" || tc.postAction || tc.comment != ""
			svc, actor, org := setupTicketAdmin(t)
			ctx := context.Background()
			code := "workflow_action_" + uniqueSuffix()
			_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
			require.NoError(t, err)
			if tc.fieldType != "" {
				_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{{FieldKey: "evidence", FieldLabel: "evidence", FieldType: tc.fieldType}}})
				require.NoError(t, err)
				_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: rawJSON(`{"assigned>in_progress":{"collect_fields":[{"key":"evidence","required":false}]}}`)})
				require.NoError(t, err)
			}
			if tc.postAction {
				reg := NewActionRegistry()
				require.NoError(t, reg.Register(&NotifyAction{Svc: svc}))
				svc.SetActionRegistry(reg)
				_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: rawJSON(`{"assigned>in_progress":{"post_actions":[{"name":"notify","params":{}}]}}`)})
				require.NoError(t, err)
			}
			def := publishD01(t, svc, code, actor)
			tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, OrgID: org()}, actor)
			require.NoError(t, err)
			require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &actor}, actor))
			tk, err = svc.Get(ctx, tk.ID, actor)
			require.NoError(t, err)
			// A newer unpublished draft removes the edge we will actually consume.
			// Existing tickets must retain the earlier actual publication.
			_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{Transitions: rawJSON(`{"open":["assigned"],"assigned":["open"],"in_progress":["pending_verify"],"pending_verify":["closed"],"closed":["open"]}`), TransitionMeta: rawJSON(`{}`)})
			require.NoError(t, err)
			var beforeEvents int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_events WHERE ticket_id=$1`, tk.ID).Scan(&beforeEvents))
			var extraOrg int64
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,path) VALUES($1::text,$1::text,$1::text::ltree) RETURNING id`, "extra"+uniqueSuffix()).Scan(&extraOrg))
			var group, family, risk, link int64
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_dispatch_groups(finding_key,owner_user_id,management_org_id,target_org_id,type_code,definition_version,approved_scope,group_key,ticket_id,ticket_ref_snapshot,operator_user_id) VALUES($1,$2,$3,$3,$4,$5,'synthetic',$6,$7,$8,$2) RETURNING id`, code, actor, org(), code, def.DefinitionVersion, strings.Repeat("d", 64), tk.ID, tk.HumanKey).Scan(&group))
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_families(fingerprint_scope,canonical_key_hash) VALUES('synthetic',$1) RETURNING id`, code).Scan(&family))
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_items(human_key,family_id,status,org_id,owner_user_id,title,created_by) VALUES($1,$2,'remediating',$3,$4,$1,$4) RETURNING id`, code, family, org(), actor).Scan(&risk))
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_ticket_links(risk_id,group_id,owner_at_link) VALUES($1,$2,$3) RETURNING id`, risk, group, actor).Scan(&link))
			_, err = testPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE code='admin' ON CONFLICT DO NOTHING`, actor)
			require.NoError(t, err)
			path := "/api/v1/risk-items/remediation/finish"
			for _, p := range []struct{ path, method string }{{path, "POST"}, {"/api/v1/tickets/transition", "POST"}, {"/api/v1/tickets/" + strconv.FormatInt(tk.ID, 10), "GET"}} {
				_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p','role::admin',$1,$2) ON CONFLICT DO NOTHING`, p.path, p.method)
				require.NoError(t, err)
			}
			m, err := casbinmodel.NewModelFromFile("../../../configs/casbin_model.conf")
			require.NoError(t, err)
			enforcer, err := casbin.NewSyncedEnforcer(m)
			require.NoError(t, err)
			_, err = enforcer.AddPolicy("role::admin", path, "POST")
			require.NoError(t, err)
			var epoch int64
			require.NoError(t, testPool.QueryRow(ctx, `SELECT session_epoch FROM users WHERE id=$1`, actor).Scan(&epoch))
			routes := []RiskWorkflowRoute{{Path: path, Method: "POST"}, {Path: "/api/v1/tickets/transition", Method: "POST"}}
			stop := errors.New("synthetic rollback after owning ticket write")
			var result error
			called := false
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set("userID", actor); c.Set("session_epoch", epoch); c.Next() }, middleware.CasbinAuth(enforcer, stubRoleFetcher{actor: {"admin"}}, nil))
			r.POST(path, func(c *gin.Context) {
				called = true
				result = repository.WithIAMCommandGuard(c.Request.Context(), testPool, func(ctx context.Context, outer pgx.Tx) error {
					return svc.WithRiskWorkflowAuthorizationTx(ctx, outer, actor, routes, func(parentCtx context.Context) error {
						err := repository.WithIAMSavepoint(parentCtx, testPool, outer, func(childCtx context.Context, child pgx.Tx) error {
							require.NotSame(t, outer, child)
							require.Same(t, outer.Conn(), child.Conn())
							_, e := svc.PrelockRiskWorkflowTx(childCtx, child, tk.ID, actor)
							require.ErrorIs(t, e, errcode.ErrNoPermission, "parent authorization proof cannot authorize its child")
							return svc.WithRiskWorkflowAuthorizationTx(childCtx, child, actor, routes, func(current context.Context) error {
								for _, invalidOrg := range []int64{0, -1} {
									_, e := svc.PrelockRiskWorkflowTx(current, child, tk.ID, actor, invalidOrg)
									require.ErrorIs(t, e, errcode.ErrInvalidParams)
								}
								scope, e := svc.PrelockRiskWorkflowTx(current, child, tk.ID, actor, extraOrg, tk.OrgID, extraOrg)
								if e != nil {
									return e
								}
								probe, e := testPool.Begin(current)
								if e != nil {
									return e
								}
								var id int64
								e = probe.QueryRow(current, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE NOWAIT`, extraOrg).Scan(&id)
								var locked *pgconn.PgError
								require.ErrorAs(t, e, &locked, "additional risk organization must be locked before the kernel scope")
								require.Equal(t, "55P03", locked.Code)
								require.NoError(t, probe.Rollback(current))
								require.Equal(t, StatusAssigned, scope.Status())
								require.Equal(t, def.DefinitionVersion, scope.DefinitionVersion())
								require.True(t, scope.Allows(StatusInProgress))
								require.True(t, scope.HasEdge(StatusAssigned, StatusInProgress))
								require.False(t, scope.Allows(StatusPendingVerify), "adapter must consume the actual published edge")
								for _, q := range []struct {
									sql string
									id  int64
								}{{`SELECT id FROM risk_dispatch_groups WHERE id=$1 FOR UPDATE`, group}, {`SELECT id FROM risk_items WHERE id=$1 FOR UPDATE`, risk}, {`SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, tk.ID}, {`SELECT id FROM risk_ticket_links WHERE id=$1 FOR UPDATE`, link}} {
									var id int64
									if e := child.QueryRow(current, q.sql, q.id).Scan(&id); e != nil {
										return e
									}
								}
								req := &model.TransitionTicketRequest{ID: tk.ID, To: StatusInProgress, Comment: tc.comment, ExpectedRevision: rawJSON(`"` + strconv.FormatInt(tk.Revision, 10) + `"`), DefinitionVersion: rawJSON(`"` + strconv.FormatInt(def.DefinitionVersion, 10) + `"`), IdempotencyKey: code}
								illegal := *req
								illegal.To = StatusPendingVerify
								_, e = svc.ApplyRiskWorkflowTx(current, child, scope, &illegal)
								require.ErrorIs(t, e, errcode.ErrTicketInvalidTransition)
								_, e = svc.ApplyRiskWorkflowTx(current, outer, scope, req)
								require.ErrorIs(t, e, errcode.ErrNoPermission, "child context cannot borrow the outer transaction")
								out, e := svc.ApplyRiskWorkflowTx(current, child, scope, req)
								if unsupported {
									require.ErrorIs(t, e, errcode.ErrServiceUnavailable, "unsupported published effects must stay closed before any owning ticket write")
									require.Nil(t, out)
									return e
								}
								if e != nil {
									return e
								}
								require.Equal(t, StatusInProgress, out.Status)
								require.Equal(t, tk.Revision+1, out.Revision)
								_, e = svc.ApplyRiskWorkflowTx(current, child, scope, req)
								require.ErrorIs(t, e, errcode.ErrNoPermission, "scope is one use")
								if rollback {
									return stop
								}
								return nil
							})
						})
						if unsupported {
							require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
							return nil
						}
						if rollback {
							require.ErrorIs(t, err, stop)
							return nil
						}
						return err
					})
				})
			})
			r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", path, nil))
			require.True(t, called)
			require.NoError(t, result)
			after, err := svc.Get(ctx, tk.ID, actor)
			require.NoError(t, err)
			var receipts, afterEvents int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_evidence_transition_receipts WHERE actor_id=$1 AND idempotency_key=$2`, actor, code).Scan(&receipts))
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_events WHERE ticket_id=$1`, tk.ID).Scan(&afterEvents))
			if rollback || unsupported {
				require.Equal(t, StatusAssigned, after.Status)
				require.Equal(t, tk.Revision, after.Revision)
				require.Zero(t, receipts)
				require.Equal(t, beforeEvents, afterEvents)
				var comments, attachments int
				require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_comments WHERE ticket_id=$1`, tk.ID).Scan(&comments))
				require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, tk.ID).Scan(&attachments))
				require.Zero(t, comments)
				require.Zero(t, attachments)
			} else {
				require.Equal(t, StatusInProgress, after.Status)
				require.Equal(t, tk.Revision+1, after.Revision)
				require.Equal(t, 1, receipts)
				require.Equal(t, beforeEvents+1, afterEvents)
			}
		})
	}
}
