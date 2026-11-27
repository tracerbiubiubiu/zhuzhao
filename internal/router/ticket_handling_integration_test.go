//go:build integration

package router_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/casbin/casbin/v3"
	"github.com/stretchr/testify/require"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	casbinpkg "github.com/tracerbiubiubiu/zhuzhao/internal/casbin"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/router"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	ticket "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

func TestTicketHandlingHTTP(t *testing.T) {
	ctx := context.Background()
	db, _, err := testutil.SetupPostgresShared()
	require.NoError(t, err)
	m, definition, err := casbinpkg.LoadModel("../../configs/casbin_model.conf")
	require.NoError(t, err)
	e, err := casbin.NewSyncedEnforcer(m)
	require.NoError(t, err)
	f := &definitionHTTPFixture{db: db, tokens: jwtpkg.NewManager(jwtpkg.Config{Secret: "handling-test-identity-012345678912345", AccessTTL: time.Minute})}
	f.exec(t, "INSERT INTO roles(code,name,priority) VALUES('handling','handling',60)")
	for path, method := range map[string]string{"/tickets/:id": "GET", "/tickets/transfer": "POST", "/tickets/withdraw": "POST", "/tickets/:id/actions": "GET", "/tickets/transition": "POST", "/tickets/assign": "POST"} {
		path = "/api/v1" + path
		_, err = e.AddPolicy("role::handling", path, method)
		require.NoError(t, err)
		f.exec(t, "INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p','role::handling',$1,$2)", path, method)
	}
	var org int64
	require.NoError(t, db.QueryRow(ctx, "INSERT INTO organizations(code,name,path,status) VALUES('handling','handling','handling',1) RETURNING id").Scan(&org))
	actor := func(name string, member bool) int64 {
		var id int64
		require.NoError(t, db.QueryRow(ctx, "INSERT INTO users(username,employee_no,password,status,session_epoch,session_projection_epoch) VALUES($1,$1,'synthetic',1,3,3) RETURNING id", name).Scan(&id))
		f.exec(t, "INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE code='handling'", id)
		if member {
			f.exec(t, "INSERT INTO user_orgs(user_id,org_id,ticket_scope) VALUES($1,$2,'assigned')", id, org)
		}
		return id
	}
	creator, assignee, target, outside := actor("ha_creator", true), actor("ha_assignee", true), actor("ha_target", true), actor("ha_outside", false)
	reg := resource.NewRegistry()
	roles := service.NewRBACService(repository.NewRoleRepo(db), repository.NewUserRepo(db), repository.NewMenuRepo(db), e)
	repo := repository.NewTicketRepo(db)
	svc := ticket.NewTicketService(db, repo, repository.NewOrgRepo(db), reg, roles, service.NewOrgDelegationService(db), nil, nil, nil)
	audit := peopleAudit{}
	f.engine = router.New(router.Deps{DBPool: db, JWTManager: f.tokens, AuditService: audit, RedisClient: startStubRedis(t), Enforcer: e, CasbinModel: definition, RoleFetcher: roles, Registry: reg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), TicketHandler: handler.NewTicketHandler(svc, audit, nil), TicketPeopleHandler: handler.NewTicketPeopleHandler(ticket.NewPeopleService(svc, e, repository.NewTicketPeopleRepo(db)))})
	f.exec(t, `INSERT INTO ticket_types(code,name,states,transitions,transition_meta,read_policy) VALUES('handling','handling','["open","assigned","closed","cancelled"]','{"open":["assigned","cancelled","closed"],"assigned":["open","closed"],"cancelled":[]}','{"open>cancelled":{"roles":["creator"]}}','inherit')`)
	typ, err := repo.GetTicketType(ctx, "handling")
	require.NoError(t, err)
	_, _, err = svc.PublishTicketType(ctx, typ, creator)
	require.NoError(t, err)
	create := func(t *testing.T) *model.Ticket {
		row, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: "handling", Title: "synthetic handling", OrgID: org}, creator)
		require.NoError(t, err)
		return row
	}
	caps := func(t *testing.T, id, user int64) map[string]bool {
		return definitionHTTPData[struct {
			Capabilities map[string]bool `json:"capabilities"`
		}](t, f.request(t, user, "GET", fmt.Sprintf("/api/v1/tickets/%d/actions", id), "", nil, 200)).Capabilities
	}
	submit := func(t *testing.T, user int64, action, key string, body map[string]string, status int) map[string]any {
		return definitionHTTPData[map[string]any](t, f.request(t, user, "POST", "/api/v1/tickets/"+action, key, body, status))
	}
	t.Run("only untouched creator withdraws and original result replays", func(t *testing.T) {
		row := create(t)
		require.True(t, caps(t, row.ID, creator)["withdraw"])
		body := map[string]string{"id": fmt.Sprint(row.ID), "expected_revision": fmt.Sprint(row.Revision), "comment": "withdrawn by creator"}
		f.request(t, creator, "POST", "/api/v1/tickets/transition", "", map[string]string{"id": body["id"], "to": "cancelled"}, 403)
		f.request(t, creator, "POST", "/api/v1/tickets/transition", "old-edge", map[string]string{"id": body["id"], "to": "cancelled", "expected_revision": body["expected_revision"], "definition_version": "1"}, 403)
		result := submit(t, creator, "withdraw", "withdraw-once", body, 200)
		require.Equal(t, map[string]any{"id": body["id"], "status": "cancelled", "assigned_to": nil, "revision": fmt.Sprint(row.Revision + 1)}, result)
		require.Equal(t, result, submit(t, creator, "withdraw", "withdraw-once", body, 200))
		require.False(t, caps(t, row.ID, creator)["withdraw"])
		var count int
		require.NoError(t, db.QueryRow(ctx, "SELECT count(*) FROM ticket_comments WHERE ticket_id=$1", row.ID).Scan(&count))
		require.Equal(t, 1, count)
	})
	t.Run("withdrawal exposes pinned mandatory comment only to eligible creator", func(t *testing.T) {
		f.exec(t, `INSERT INTO ticket_types(code,name,states,transitions,transition_meta,read_policy) SELECT 'handling_note','handling note',states,transitions,jsonb_set(transition_meta,'{open>cancelled,comment_required}','true'),'inherit' FROM ticket_types WHERE code='handling'`)
		note, err := repo.GetTicketType(ctx, "handling_note")
		require.NoError(t, err)
		_, _, err = svc.PublishTicketType(ctx, note, creator)
		require.NoError(t, err)
		row, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: "handling_note", Title: "pinned note", OrgID: org}, creator)
		require.NoError(t, err)
		require.True(t, caps(t, row.ID, creator)["withdraw_comment_required"])
		require.False(t, caps(t, row.ID, assignee)["withdraw_comment_required"])
		body := map[string]string{"id": fmt.Sprint(row.ID), "expected_revision": fmt.Sprint(row.Revision)}
		submit(t, creator, "withdraw", "note-withdraw", body, 400)
		body["comment"] = "required note"
		submit(t, creator, "withdraw", "note-withdraw", body, 200)
		require.False(t, caps(t, row.ID, creator)["withdraw_comment_required"])
	})
	t.Run("default read policy denies unrelated colleague action projection", func(t *testing.T) {
		f.exec(t, `INSERT INTO ticket_types(code,name,states,transitions,transition_meta)
			SELECT 'handling_restricted','handling restricted',states,transitions,transition_meta FROM ticket_types WHERE code='handling'`)
		restricted, err := repo.GetTicketType(ctx, "handling_restricted")
		require.NoError(t, err)
		_, _, err = svc.PublishTicketType(ctx, restricted, creator)
		require.NoError(t, err)
		row, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: "handling_restricted", Title: "restricted lifecycle", OrgID: org}, creator)
		require.NoError(t, err)
		require.True(t, caps(t, row.ID, creator)["withdraw"])
		f.request(t, assignee, "GET", fmt.Sprintf("/api/v1/tickets/%d/actions", row.ID), "", nil, 404)
	})
	t.Run("former assignment permanently disqualifies reopened ticket", func(t *testing.T) {
		row := create(t)
		f.exec(t, "UPDATE tickets SET assigned_to=$2,status='assigned' WHERE id=$1", row.ID, assignee)
		f.exec(t, "UPDATE tickets SET assigned_to=NULL,status='open' WHERE id=$1", row.ID)
		require.False(t, caps(t, row.ID, creator)["withdraw"])
		submit(t, creator, "withdraw", "once-assigned", map[string]string{"id": fmt.Sprint(row.ID), "expected_revision": fmt.Sprint(row.Revision + 2)}, 409)
	})
	t.Run("transfer target and same-state clock are fenced", func(t *testing.T) {
		row := create(t)
		f.exec(t, "UPDATE tickets SET assigned_to=$2,status='assigned',sla_due_at='2050-01-01',status_changed_at='2020-01-01' WHERE id=$1", row.ID, assignee)
		require.True(t, caps(t, row.ID, assignee)["transfer"])
		require.False(t, caps(t, row.ID, creator)["transfer"])
		picker := f.request(t, assignee, "GET", fmt.Sprintf("/api/v1/tickets/people?purpose=transfer&ticket_id=%d&q=ha_", row.ID), "", nil, 200).Body.String()
		require.Contains(t, picker, "ha_target")
		require.NotContains(t, picker, "ha_outside")
		require.NotContains(t, picker, "ha_assignee")
		body := map[string]string{"id": fmt.Sprint(row.ID), "expected_revision": fmt.Sprint(row.Revision + 1), "to_user_id": fmt.Sprint(outside), "comment": "handed over"}
		submit(t, assignee, "transfer", "ineligible", body, 400)
		body["to_user_id"] = fmt.Sprint(assignee)
		submit(t, assignee, "transfer", "same-target", body, 409)
		body["to_user_id"] = fmt.Sprint(target)
		body["expected_revision"] = fmt.Sprint(row.Revision)
		submit(t, assignee, "transfer", "stale", body, 409)
		body["expected_revision"] = fmt.Sprint(row.Revision + 1)
		f.exec(t, "UPDATE user_orgs SET expires_at=now()-interval '1 minute' WHERE user_id=$1", target)
		submit(t, assignee, "transfer", "expired-target", body, 400)
		f.exec(t, "UPDATE user_orgs SET expires_at=NULL WHERE user_id=$1", target)
		submit(t, creator, "transfer", "creator-only", body, 403)
		result := submit(t, assignee, "transfer", "once-transfer", body, 200)
		require.Equal(t, map[string]any{"id": body["id"], "status": "assigned", "assigned_to": body["to_user_id"], "revision": fmt.Sprint(row.Revision + 2)}, result)
		var preserved bool
		require.NoError(t, db.QueryRow(ctx, "SELECT sla_due_at='2050-01-01' AND status_changed_at='2020-01-01' AND definition_version=1 FROM tickets WHERE id=$1", row.ID).Scan(&preserved))
		require.True(t, preserved)
		var events int
		require.NoError(t, db.QueryRow(ctx, "SELECT count(*) FROM ticket_events WHERE ticket_id=$1 AND action='transferred' AND user_id=$2 AND from_value=$3 AND to_value=$4", row.ID, assignee, fmt.Sprint(assignee), fmt.Sprint(target)).Scan(&events))
		require.Equal(t, 1, events)
		require.Equal(t, result, submit(t, assignee, "transfer", "once-transfer", body, 200))
		body["comment"] = "different"
		submit(t, assignee, "transfer", "once-transfer", body, 409)
		body["comment"] = "handed over"
		f.exec(t, "DELETE FROM casbin_rule WHERE v0='role::handling' AND v1='/api/v1/tickets/transfer'")
		submit(t, assignee, "transfer", "once-transfer", body, 403)
		f.exec(t, "INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p','role::handling','/api/v1/tickets/transfer','POST')")
		f.exec(t, "UPDATE tickets SET title='later revision' WHERE id=$1", row.ID)
		require.Equal(t, result, submit(t, assignee, "transfer", "once-transfer", body, 200))
		f.exec(t, "DELETE FROM user_orgs WHERE user_id=$1", assignee)
		submit(t, assignee, "transfer", "once-transfer", body, 404)
		f.exec(t, "INSERT INTO user_orgs(user_id,org_id,ticket_scope) VALUES($1,$2,'assigned')", assignee, org)
	})
	t.Run("target needs active detail and transition grants", func(t *testing.T) {
		noRole := actor("eligible_none", true)
		readOnly := actor("eligible_reader", true)
		stepOnly := actor("eligible_handler", true)
		successor := actor("eligible_successor", true)
		f.exec(t, "DELETE FROM user_roles WHERE user_id=ANY($1)", []int64{noRole, readOnly, stepOnly, successor})
		var sourceOrg int64
		require.NoError(t, db.QueryRow(ctx, "INSERT INTO organizations(code,name,path,status) VALUES('handling_grants','handling grants','handlinggrants',1) RETURNING id").Scan(&sourceOrg))
		grants := []struct{ code, path, method string }{{"handling_read", "/api/v1/tickets/:id", "GET"}, {"handling_step", "/api/v1/tickets/transition", "POST"}}
		for _, grant := range grants {
			f.exec(t, "INSERT INTO roles(code,name,priority) VALUES($1,$1,60)", grant.code)
			_, err := e.AddPolicy("role::"+grant.code, grant.path, grant.method)
			require.NoError(t, err)
			f.exec(t, "INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,$3)", "role::"+grant.code, grant.path, grant.method)
		}
		f.exec(t, `INSERT INTO user_roles(user_id,role_id) SELECT u,id FROM roles CROSS JOIN unnest($1::bigint[]) AS u WHERE code='handling_read'`, []int64{readOnly, successor})
		f.exec(t, "INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE code='handling_step'", stepOnly)
		f.exec(t, "INSERT INTO user_orgs(user_id,org_id) VALUES($1,$2)", successor, sourceOrg)
		f.exec(t, "INSERT INTO org_roles(org_id,role_id) SELECT $1,id FROM roles WHERE code='handling_step'", sourceOrg)
		row := create(t)
		f.exec(t, "UPDATE tickets SET status='assigned',assigned_to=$2 WHERE id=$1", row.ID, assignee)
		query := fmt.Sprintf("/api/v1/tickets/people?purpose=transfer&ticket_id=%d&q=eligible_", row.ID)
		picked := f.request(t, assignee, "GET", query, "", nil, 200).Body.String()
		require.Contains(t, picked, "eligible_successor")
		for _, name := range []string{"eligible_none", "eligible_reader", "eligible_handler"} {
			require.NotContains(t, picked, name)
		}
		body := map[string]string{"id": fmt.Sprint(row.ID), "expected_revision": fmt.Sprint(row.Revision + 1)}
		for _, id := range []int64{noRole, readOnly, stepOnly} {
			body["to_user_id"] = fmt.Sprint(id)
			submit(t, assignee, "transfer", "target-check", body, 400)
		}
		body["to_user_id"] = fmt.Sprint(successor)
		for _, grant := range grants {
			// Leave the projection in memory unchanged. Both the picker and
			// transfer submission must use the current database policy.
			f.exec(t, "DELETE FROM casbin_rule WHERE ptype='p' AND v0=$1 AND v1=$2 AND v2=$3", "role::"+grant.code, grant.path, grant.method)
			require.NotContains(t, f.request(t, assignee, "GET", query, "", nil, 200).Body.String(), "eligible_successor")
			submit(t, assignee, "transfer", "target-check", body, 400)
			f.exec(t, "INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,$3)", "role::"+grant.code, grant.path, grant.method)
		}
		f.exec(t, "UPDATE organizations SET status=0 WHERE id=$1", sourceOrg)
		require.NotContains(t, f.request(t, assignee, "GET", query, "", nil, 200).Body.String(), "eligible_successor")
		submit(t, assignee, "transfer", "target-check", body, 400)
		f.exec(t, "UPDATE organizations SET status=1 WHERE id=$1", sourceOrg)
		submit(t, assignee, "transfer", "target-check", body, 200)
		f.request(t, successor, "GET", fmt.Sprintf("/api/v1/tickets/%d", row.ID), "", nil, 200)
		result := submit(t, successor, "transition", "", map[string]string{"id": fmt.Sprint(row.ID), "to": "closed"}, 200)
		require.Equal(t, "closed", result["status"])
	})

	t.Run("legacy unknown and previously accepted cannot withdraw", func(t *testing.T) {
		var legacy int64
		require.NoError(t, db.QueryRow(ctx, `INSERT INTO tickets(type_code,title,status,created_by,org_id,org_path,definition_version)
        VALUES('handling','legacy','open',$1,$2,'handling',1) RETURNING id`, creator, org).Scan(&legacy))
		submit(t, creator, "withdraw", "legacy", map[string]string{"id": fmt.Sprint(legacy), "expected_revision": "1"}, 409)
		row := create(t)
		f.exec(t, "UPDATE tickets SET status='assigned' WHERE id=$1", row.ID)
		f.exec(t, "UPDATE tickets SET status='open',withdrawal_eligible=true WHERE id=$1", row.ID)
		submit(t, creator, "withdraw", "reopened", map[string]string{"id": fmt.Sprint(row.ID), "expected_revision": "3"}, 409)
	})
	t.Run("risk-linked ticket suppresses every ordinary handling surface", func(t *testing.T) {
		row := create(t)
		f.exec(t, `WITH family AS(INSERT INTO risk_families(fingerprint_scope,canonical_key_hash) VALUES('handling','handling') RETURNING id),
        item AS(INSERT INTO risk_items(human_key,family_id,org_id,title,created_by) SELECT 'handling',id,$2,'test',$3 FROM family RETURNING id),
        grp AS(INSERT INTO risk_dispatch_groups(finding_key,owner_user_id,management_org_id,target_org_id,type_code,definition_version,approved_scope,group_key,ticket_id,ticket_ref_snapshot,operator_user_id)
        VALUES('handling',$3,$2,$2,'handling',1,'synthetic',repeat('a',64),$1,'handling',$3) RETURNING id)
        INSERT INTO risk_ticket_links(risk_id,group_id,owner_at_link) SELECT item.id,grp.id,$3 FROM item,grp`, row.ID, org, creator)
		require.False(t, caps(t, row.ID, creator)["withdraw"])
		submit(t, creator, "withdraw", "risk-withdraw", map[string]string{"id": fmt.Sprint(row.ID), "expected_revision": "1"}, 403)
		f.exec(t, "UPDATE tickets SET status='assigned',assigned_to=$2 WHERE id=$1", row.ID, creator)
		require.False(t, caps(t, row.ID, creator)["transfer"])
		f.request(t, creator, "GET", fmt.Sprintf("/api/v1/tickets/people?purpose=transfer&ticket_id=%d&q=ha_", row.ID), "", nil, 403)
		submit(t, creator, "transfer", "risk-transfer", map[string]string{"id": fmt.Sprint(row.ID), "to_user_id": fmt.Sprint(target), "expected_revision": "2"}, 403)
	})
	t.Run("failed receipt rolls back state comment history and reminder", func(t *testing.T) {
		row := create(t)
		f.exec(t, "INSERT INTO ticket_reminders(ticket_id,state,reminder_count,last_reminder_at) VALUES($1,'open',2,now())", row.ID)
		f.exec(t, `ALTER TABLE ticket_handling_receipts ADD CONSTRAINT handling_synthetic_failure CHECK(operation_key <> 'synthetic-failure')`)
		defer f.exec(t, "ALTER TABLE ticket_handling_receipts DROP CONSTRAINT handling_synthetic_failure")
		submit(t, creator, "withdraw", "synthetic-failure", map[string]string{"id": fmt.Sprint(row.ID), "expected_revision": "1", "comment": "should roll back"}, 500)
		var original bool
		require.NoError(t, db.QueryRow(ctx, `SELECT status='open' AND revision=1 AND withdrawal_eligible
          AND NOT EXISTS(SELECT 1 FROM ticket_comments WHERE ticket_id=$1)
          AND NOT EXISTS(SELECT 1 FROM ticket_events WHERE ticket_id=$1 AND action='withdrawn')
          AND EXISTS(SELECT 1 FROM ticket_reminders WHERE ticket_id=$1 AND reminder_count=2)
          FROM tickets WHERE id=$1`, row.ID).Scan(&original))
		require.True(t, original)
	})
	t.Run("assignment races creator withdrawal", func(t *testing.T) {
		manager := actor("ha_manager", true)
		f.exec(t, "UPDATE user_orgs SET ticket_scope='group' WHERE user_id=$1", manager)
		for i := 0; i < 5; i++ {
			row := create(t)
			requests := []*http.Request{
				f.newRequest(t, creator, "POST", "/api/v1/tickets/withdraw", fmt.Sprintf("race-%d", i), map[string]string{"id": fmt.Sprint(row.ID), "expected_revision": "1"}),
				f.newRequest(t, manager, "POST", "/api/v1/tickets/assign", "", map[string]string{"id": fmt.Sprint(row.ID), "assigned_to": fmt.Sprint(target)}),
			}
			start := make(chan struct{})
			outcomes := make(chan int, len(requests))
			for _, request := range requests {
				go func() { <-start; w := httptest.NewRecorder(); f.engine.ServeHTTP(w, request); outcomes <- w.Code }()
			}
			close(start)
			got := []int{<-outcomes, <-outcomes}
			slices.Sort(got)
			require.Equal(t, []int{200, 409}, got)
			var consistent bool
			require.NoError(t, db.QueryRow(ctx, "SELECT (status='cancelled' AND assigned_to IS NULL) OR (status='assigned' AND assigned_to=$2) FROM tickets WHERE id=$1", row.ID, target).Scan(&consistent))
			require.True(t, consistent)
		}
	})
}
