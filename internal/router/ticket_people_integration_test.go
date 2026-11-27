//go:build integration

package router_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/router"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	ticket "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

type peopleAudit struct{}

func (peopleAudit) Insert(context.Context, middleware.AuditLogEntry) error { return nil }

type peopleSearchSpy struct {
	repo  *repository.TicketPeopleRepo
	calls int
}

func (r *peopleSearchSpy) SearchEnabled(ctx context.Context, q string, org int64) ([]model.TicketPerson, error) {
	r.calls++
	return r.repo.SearchEnabled(ctx, q, org)
}

func TestTicketPeopleHTTP_ContextGateAndProjection(t *testing.T) {
	pool, _, err := testutil.SetupPostgresShared()
	require.NoError(t, err)
	ctx := context.Background()
	uid := func(name string, status int) int64 {
		var id int64
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users(username,password,employee_no,real_name,status,session_epoch,session_projection_epoch) VALUES($1,'not-a-credential',$1,$1,$2,3,3) RETURNING id`, name, status).Scan(&id))
		return id
	}
	actor := uid("f3_operator", 1)
	viewer := uid("f3_viewer", 1)
	outsider := uid("f3_outsider", 1)
	wanted := uid("needle_active", 1)
	disabled := uid("needle_disabled", 0)
	deleted := uid("needle_deleted", 1)
	_, err = pool.Exec(ctx, `UPDATE users SET deleted_at=NOW() WHERE id=$1`, deleted)
	require.NoError(t, err)
	org := func(code, path string, parent *int64) int64 {
		var id int64
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO organizations(code,name,path,parent_id,status,ticket_visibility) VALUES($1,$1,$2::ltree,$3,1,'project_isolated') RETURNING id`, code, path, parent).Scan(&id))
		return id
	}
	parent := org("f3_parent", "f3parent", nil)
	group := org("f3_group", "f3parent.child", &parent)
	foreign := org("f3_foreign", "f3foreign", nil)
	_, err = pool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id,ticket_scope) VALUES($1,$3,'group'),($2,$3,'assigned')`, actor, wanted, group)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO ticket_types(code,name,read_policy) VALUES('f3_type','synthetic','inherit')`)
	require.NoError(t, err)
	tk := func(orgID, creator int64) int64 {
		var id int64
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO tickets(type_code,title,created_by,org_id,org_path) SELECT 'f3_type','synthetic',$2,id,path FROM organizations WHERE id=$1 RETURNING id`, orgID, creator).Scan(&id))
		return id
	}
	visible := tk(group, actor)
	hidden := tk(foreign, outsider)
	_, err = pool.Exec(ctx, `INSERT INTO ticket_cc(ticket_id,user_id,role,created_by) VALUES($1,$2,'cc',$3)`, visible, viewer, actor)
	require.NoError(t, err)
	roles := &stubRoleFetcher{roles: map[int64][]string{actor: {"f3_writer"}, viewer: {"f3_viewer"}, outsider: {"f3_writer"}}}
	enforcer := newRouterTestEnforcer(t)
	for _, path := range []string{"/api/v1/tickets/assign", "/api/v1/tickets", "/api/v1/assignment-rules"} {
		_, err = enforcer.AddPolicy("role::f3_writer", path, "POST")
		require.NoError(t, err)
	}
	reg := resource.NewRegistry()
	orgs := repository.NewOrgRepo(pool)
	svc := ticket.NewTicketService(pool, repository.NewTicketRepo(pool), orgs, reg, roles, service.NewOrgDelegationService(pool), nil, nil, nil)
	spy := &peopleSearchSpy{repo: repository.NewTicketPeopleRepo(pool)}
	m := jwtpkg.NewManager(jwtpkg.Config{Secret: "people-test-identity-only-0123456789", AccessTTL: time.Minute})
	r := router.New(router.Deps{DBPool: pool, JWTManager: m, AuditService: peopleAudit{}, RedisClient: startStubRedis(t), Enforcer: enforcer, RoleFetcher: roles, Registry: reg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), TicketPeopleHandler: handler.NewTicketPeopleHandler(ticket.NewPeopleService(svc, enforcer, spy))})
	query := func(purpose string, id int64) string {
		return fmt.Sprintf("/api/v1/tickets/people?purpose=%s&ticket_id=%d&q=needle", purpose, id)
	}
	assertDenied := func(t *testing.T, user int64, path string, status int) {
		t.Helper()
		before := spy.calls
		w := do(r, m, user, http.MethodGet, path)
		require.Equal(t, status, w.Code, w.Body.String())
		assert.Equal(t, before, spy.calls, "unauthorized/invalid request must never search candidates")
		assert.NotContains(t, w.Body.String(), "needle_")
	}
	t.Run("original-users-management-stays-denied", func(t *testing.T) {
		w := do(r, m, actor, http.MethodGet, "/api/v1/users")
		require.Equal(t, http.StatusForbidden, w.Code)
	})
	t.Run("write-context-without-user-read", func(t *testing.T) {
		w := do(r, m, actor, http.MethodGet, query("assign", visible))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var body struct {
			Data struct {
				List []map[string]any `json:"list"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body.Data.List, 1)
		assert.Equal(t, fmt.Sprint(wanted), body.Data.List[0]["id"])
		assert.Len(t, body.Data.List[0], 2)
		assert.NotEmpty(t, body.Data.List[0]["label"])
		assert.NotContains(t, w.Body.String(), fmt.Sprintf(`"id":"%d"`, disabled))
		assert.NotContains(t, w.Body.String(), fmt.Sprintf(`"id":"%d"`, deleted))
	})
	t.Run("no-action-no-visibility-zero-disclosure", func(t *testing.T) {
		assertDenied(t, viewer, query("assign", visible), 403)
		assertDenied(t, actor, query("assign", hidden), 404)
		assertDenied(t, viewer, query("cc", visible), 403) // Read visibility alone cannot enumerate other CC candidates.
	})
	t.Run("cc-uses-original-dynamic-update-not-new-L1", func(t *testing.T) {
		roles.roles[actor] = []string{"f3_no_static_policy"}
		defer func() { roles.roles[actor] = []string{"f3_writer"} }()
		w := do(r, m, actor, http.MethodGet, query("cc", visible))
		require.Equal(t, 200, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "needle_active")
		assertDenied(t, actor, query("assign", visible), 403)
	})
	t.Run("cc-search-requires-restricted-type-cc-management", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO ticket_types(code,name,read_policy) VALUES('f3_cc_restricted','synthetic','participants_only')`)
		require.NoError(t, err)
		var id int64
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO tickets(type_code,title,created_by,org_id,org_path)
			SELECT 'f3_cc_restricted','synthetic',$2,id,path FROM organizations WHERE id=$1 RETURNING id`, group, actor).Scan(&id))
		assertDenied(t, actor, query("cc", id), http.StatusForbidden)
		roles.roles[actor] = []string{"admin"}
		defer func() { roles.roles[actor] = []string{"f3_writer"} }()
		w := do(r, m, actor, http.MethodGet, query("cc", id))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "needle_active")
	})
	t.Run("creation-requires-current-org-and-type", func(t *testing.T) {
		path := fmt.Sprintf("/api/v1/tickets/people?purpose=create-supervisor&org_id=%d&type_code=f3_type&q=needle", group)
		w := do(r, m, actor, "GET", path)
		require.Equal(t, 200, w.Code, w.Body.String())
		assertDenied(t, actor, strings.Replace(path, fmt.Sprint(group), fmt.Sprint(foreign), 1), 403)
		assertDenied(t, viewer, path, 403)
		assertDenied(t, actor, strings.Replace(path, "f3_type", "missing_type", 1), 404)
	})
	t.Run("creation-does-not-enumerate-for-invalid-legacy-graph", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO ticket_types(code,name,states,transitions) VALUES('f3_bad_graph','synthetic','["new","done"]','{"new":["done"]}')`)
		require.NoError(t, err)
		path := fmt.Sprintf("/api/v1/tickets/people?purpose=create-supervisor&org_id=%d&type_code=f3_bad_graph&q=needle", group)
		assertDenied(t, actor, path, 400)
	})
	t.Run("global-creation-still-needs-an-existing-organization", func(t *testing.T) {
		var roleID, menuID int64
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO roles(code,name,priority) VALUES('f3_global_members','synthetic',60) RETURNING id`).Scan(&roleID))
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO menus(code,name,menu_type,permission) VALUES('f3_global_members','synthetic',3,'org:member') RETURNING id`).Scan(&menuID))
		_, err := pool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)`, actor, roleID)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO role_menus(role_id,menu_id) VALUES($1,$2)`, roleID, menuID)
		require.NoError(t, err)
		defer func() {
			_, err := pool.Exec(ctx, `DELETE FROM user_roles WHERE user_id=$1 AND role_id=$2`, actor, roleID)
			require.NoError(t, err)
		}()
		assertDenied(t, actor, "/api/v1/tickets/people?purpose=create-supervisor&org_id=999999999&type_code=f3_type&q=needle", 404)
	})
	t.Run("rule-member-source-suspends-and-expires", func(t *testing.T) {
		path := fmt.Sprintf("/api/v1/tickets/people?purpose=rule-target&org_id=%d&q=needle", group)
		w := do(r, m, actor, "GET", path)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "needle_active")
		assertDenied(t, viewer, path, 403)
		for step, sql := range []string{`UPDATE organizations SET status=0 WHERE id=$1`, `UPDATE organizations SET status=1 WHERE id=$1`} {
			_, err = pool.Exec(ctx, sql, parent)
			require.NoError(t, err)
			if step == 1 {
				_, err = pool.Exec(ctx, `UPDATE user_orgs SET expires_at=NOW()-INTERVAL '1 minute' WHERE user_id=$1`, wanted)
				require.NoError(t, err)
			}
			w = do(r, m, actor, "GET", path)
			require.Equal(t, 200, w.Code, w.Body.String())
			assert.JSONEq(t, `[]`, jsonList(t, w.Body.Bytes()))
		}
	})
	t.Run("invalid-context-short-and-wildcard-input", func(t *testing.T) {
		for _, path := range []string{"?q=needle", "?purpose=assign&q=needle", "?purpose=assign&ticket_id=-1&q=needle", "?purpose=assign&ticket_id=abc&q=needle", fmt.Sprintf("?purpose=assign&ticket_id=%d&q=x", visible), fmt.Sprintf("?purpose=assign&ticket_id=%d&q=needle&org_id=%d", visible, group)} {
			assertDenied(t, actor, "/api/v1/tickets/people"+path, 400)
		}
		w := do(r, m, actor, "GET", fmt.Sprintf("/api/v1/tickets/people?purpose=assign&ticket_id=%d&q=%s", visible, url.QueryEscape("%%")))
		require.Equal(t, 200, w.Code, w.Body.String())
		assert.JSONEq(t, `[]`, jsonList(t, w.Body.Bytes()))
	})
	t.Run("limit-20-no-total-or-extra-fields", func(t *testing.T) {
		for i := 0; i < 24; i++ {
			uid(fmt.Sprintf("manyneedle_%02d", i), 1)
		}
		w := do(r, m, actor, "GET", strings.Replace(query("assign", visible), "q=needle", "q=manyneedle", 1))
		require.Equal(t, 200, w.Code, w.Body.String())
		var rows []map[string]any
		require.NoError(t, json.Unmarshal([]byte(jsonList(t, w.Body.Bytes())), &rows))
		require.Len(t, rows, 20)
		for _, row := range rows {
			assert.Len(t, row, 2)
		}
		assert.NotContains(t, w.Body.String(), `"total"`)
	})
}

func jsonList(t *testing.T, data []byte) string {
	t.Helper()
	var body struct {
		Data struct {
			List json.RawMessage `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(data, &body))
	return string(body.Data.List)
}
