//go:build integration

package ticket

import (
	"context"
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

// A cached admin role, an admitted route, or ticket transition permission must
// never substitute for the current assignment route and resource permission.
func TestRiskWorkflowAssignmentPrelockCurrentAuthority(t *testing.T) {
	for _, change := range []string{"allow_without_transition", "assign_policy", "read_policy", "resource", "epoch", "actor"} {
		t.Run(change, func(t *testing.T) {
			svc, actor, org := setupTicketAdmin(t)
			ctx := context.Background()
			code := "workflow_assign_" + uniqueSuffix()
			_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
			require.NoError(t, err)
			def := publishD01(t, svc, code, actor)
			tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, OrgID: org()}, actor)
			require.NoError(t, err)
			require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &actor}, actor))
			tk, err = svc.Get(ctx, tk.ID, actor)
			require.NoError(t, err)
			var group, family, risk int64
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_dispatch_groups(finding_key,owner_user_id,management_org_id,target_org_id,type_code,definition_version,approved_scope,group_key,ticket_id,ticket_ref_snapshot,operator_user_id) VALUES($1,$2,$3,$3,$4,$5,'synthetic',$6,$7,$8,$2) RETURNING id`, code, actor, org(), code, def.DefinitionVersion, strings.Repeat("d", 64), tk.ID, tk.HumanKey).Scan(&group))
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_families(fingerprint_scope,canonical_key_hash) VALUES('synthetic',$1) RETURNING id`, code).Scan(&family))
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_items(human_key,family_id,status,org_id,owner_user_id,title,created_by) VALUES($1,$2,'remediating',$3,$4,$1,$4) RETURNING id`, code, family, org(), actor).Scan(&risk))
			_, err = testPool.Exec(ctx, `INSERT INTO risk_ticket_links(risk_id,group_id,owner_at_link) VALUES($1,$2,$3)`, risk, group, actor)
			require.NoError(t, err)
			// Draft edits must not replace the publication pinned to this ticket.
			_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{Transitions: rawJSON(`{"open":["assigned"],"assigned":["open"],"in_progress":["pending_verify"],"pending_verify":["closed"],"closed":["open"]}`)})
			require.NoError(t, err)
			var role, epoch, extraOrg int64
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO roles(code,name,priority) VALUES($1,$1,50) RETURNING id`, code).Scan(&role))
			_, err = testPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)`, actor, role)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id,is_primary,org_member_role,ticket_scope) VALUES($1,$2,false,'admin','assigned')`, actor, org())
			require.NoError(t, err)
			require.NoError(t, testPool.QueryRow(ctx, `SELECT session_epoch FROM users WHERE id=$1`, actor).Scan(&epoch))
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,path) VALUES($1::text,$1::text,$1::text::ltree) RETURNING id`, "assignextra"+uniqueSuffix()).Scan(&extraOrg))
			path := "/api/v1/risk-items/assignment-" + code
			readPath := "/api/v1/tickets/" + strconv.FormatInt(tk.ID, 10)
			for _, p := range []RiskWorkflowRoute{{Path: path, Method: "POST"}, {Path: readPath, Method: "GET"}, {Path: "/api/v1/tickets/assign", Method: "POST"}} {
				_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,$3) ON CONFLICT DO NOTHING`, "role::"+code, p.Path, p.Method)
				require.NoError(t, err)
			}
			m, err := casbinmodel.NewModelFromFile("../../../configs/casbin_model.conf")
			require.NoError(t, err)
			enforcer, err := casbin.NewSyncedEnforcer(m)
			require.NoError(t, err)
			_, err = enforcer.AddPolicy("role::"+code, path, "POST")
			require.NoError(t, err)
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("userID", actor); c.Set("session_epoch", epoch); c.Next() }, middleware.CasbinAuth(enforcer, stubRoleFetcher{actor: {code}}, nil))
			var result error
			called := false
			router.POST(path, func(c *gin.Context) {
				called = true
				// Middleware keeps its admitted snapshot while current DB facts change.
				result = repository.WithIAMGuard(c.Request.Context(), testPool, func(ctx context.Context) error {
					q := repository.IAMDB(ctx, testPool)
					switch change {
					case "assign_policy":
						_, err = q.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1='/api/v1/tickets/assign'`, "role::"+code)
					case "read_policy":
						_, err = q.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1=$2`, "role::"+code, readPath)
					case "resource":
						_, err = q.Exec(ctx, `UPDATE user_orgs SET org_member_role='member' WHERE user_id=$1 AND org_id=$2`, actor, tk.OrgID)
					case "epoch":
						_, err = q.Exec(ctx, `UPDATE users SET session_epoch=session_epoch+1 WHERE id=$1`, actor)
					}
					return err
				})
				if result != nil {
					return
				}
				result = repository.WithIAMCommandGuard(c.Request.Context(), testPool, func(ctx context.Context, tx pgx.Tx) error {
					return svc.WithRiskWorkflowAuthorizationTx(ctx, tx, actor, []RiskWorkflowRoute{{Path: path, Method: "POST"}}, func(current context.Context) error {
						child, e := tx.Begin(current)
						if e != nil {
							return e
						}
						_, e = svc.PrelockRiskWorkflowAssignmentTx(current, child, tk.ID, actor)
						require.ErrorIs(t, e, errcode.ErrNoPermission, "parent proof cannot authorize a child transaction")
						if e := child.Rollback(current); e != nil {
							return e
						}
						requestActor := actor
						if change == "actor" {
							requestActor++
						}
						scope, e := svc.PrelockRiskWorkflowAssignmentTx(current, tx, tk.ID, requestActor, extraOrg, tk.OrgID, extraOrg)
						if e != nil {
							return e
						}
						require.Equal(t, StatusAssigned, scope.Status())
						require.Equal(t, tk.Revision, scope.Revision())
						require.Equal(t, def.DefinitionVersion, scope.DefinitionVersion())
						require.True(t, scope.HasEdge(StatusAssigned, StatusInProgress), "the actual immutable publication must be retained")
						_, e = svc.ApplyRiskWorkflowTx(current, tx, scope, &model.TransitionTicketRequest{ID: tk.ID, To: StatusInProgress, ExpectedRevision: rawJSON(`"` + strconv.FormatInt(tk.Revision, 10) + `"`), DefinitionVersion: rawJSON(`"` + strconv.FormatInt(def.DefinitionVersion, 10) + `"`), IdempotencyKey: code})
						require.ErrorIs(t, e, errcode.ErrNoPermission, "assignment observation must not mint transition authority")
						probe, e := testPool.Begin(current)
						if e != nil {
							return e
						}
						defer probe.Rollback(current) //nolint:errcheck // probe is rolled back on every exit
						var id int64
						require.NoError(t, probe.QueryRow(current, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE NOWAIT`, tk.ID).Scan(&id), "prelock must leave ticket acquisition after risk locks")
						e = probe.QueryRow(current, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE NOWAIT`, extraOrg).Scan(&id)
						var locked *pgconn.PgError
						require.ErrorAs(t, e, &locked)
						require.Equal(t, "55P03", locked.Code)
						return nil
					})
				})
			})
			router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", path, nil))
			require.True(t, called)
			if change == "allow_without_transition" {
				require.NoError(t, result, "assignment prelock must not require ticket transition L1")
			} else {
				require.ErrorIs(t, result, errcode.ErrNoPermission)
			}
		})
	}
}

func TestRiskWorkflowAssignmentPrelockRequiresExactProof(t *testing.T) {
	svc, actor, _ := setupTicketAdmin(t)
	ctx := context.Background()
	bare, err := testPool.Begin(ctx)
	require.NoError(t, err)
	_, err = svc.PrelockRiskWorkflowAssignmentTx(ctx, bare, 1, actor)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	require.NoError(t, bare.Rollback(ctx))
	require.NoError(t, repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
		_, err := svc.PrelockRiskWorkflowAssignmentTx(ctx, tx, 1, actor)
		require.ErrorIs(t, err, errcode.ErrNoPermission)
		return nil
	}))
}
