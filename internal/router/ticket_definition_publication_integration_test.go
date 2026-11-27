//go:build integration

package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/router"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	ticket "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

const (
	definitionPlanPath    = "/api/v1/ticket-types/definitions/plan"
	definitionPublishPath = "/api/v1/ticket-types/definitions/publish"
)

type definitionHTTPFixture struct {
	db        *pgxpool.Pool
	engine    *gin.Engine
	tokens    *jwtpkg.Manager
	publisher int64
	creator   int64
	viewer    int64
}

func newDefinitionHTTPFixture(t *testing.T) *definitionHTTPFixture {
	t.Helper()
	db, _, err := testutil.SetupPostgresShared()
	require.NoError(t, err)
	ctx := context.Background()
	enforcer := newRouterTestEnforcer(t)
	type grant struct{ path, method string }
	addActor := func(code string, grants ...grant) int64 {
		t.Helper()
		var actor int64
		require.NoError(t, db.QueryRow(ctx, `
			INSERT INTO users(username,password,employee_no,real_name,status,session_epoch,session_projection_epoch)
			VALUES($1,'synthetic-only',$1,$1,1,3,3) RETURNING id`, code).Scan(&actor))
		_, err := db.Exec(ctx, `
			WITH new_role AS (INSERT INTO roles(code,name,priority) VALUES($1,$1,60) RETURNING id)
			INSERT INTO user_roles(user_id,role_id) SELECT $2,id FROM new_role`, code, actor)
		require.NoError(t, err)
		for _, g := range grants {
			_, err = enforcer.AddPolicy("role::"+code, g.path, g.method)
			require.NoError(t, err)
			_, err = db.Exec(ctx, "INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,$3)", "role::"+code, g.path, g.method)
			require.NoError(t, err)
		}
		return actor
	}
	read := grant{"/api/v1/ticket-types/:code/published", http.MethodGet}
	f := &definitionHTTPFixture{
		db: db,
		publisher: addActor("d01_http_publisher",
			grant{definitionPlanPath, http.MethodPost}, grant{definitionPublishPath, http.MethodPost},
			grant{"/api/v1/ticket-types/fields/replace", http.MethodPost},
			grant{"/api/v1/ticket-types/update", http.MethodPost}, grant{"/api/v1/ticket-types/:code/fields", http.MethodGet}, read,
			grant{"/api/v1/ticket-types", http.MethodGet}),
		creator: addActor("d01_http_creator", read, grant{"/api/v1/tickets", http.MethodPost}, grant{"/api/v1/ticket-types", http.MethodGet}),
		viewer:  addActor("d01_http_viewer", read, grant{"/api/v1/ticket-types/:code/fields", http.MethodGet}),
		tokens:  jwtpkg.NewManager(jwtpkg.Config{Secret: "definition-test-identity-0123456789", AccessTTL: time.Minute}),
	}
	reg := resource.NewRegistry()
	ticket.RegisterExprDictLookup(db)
	orgs := repository.NewOrgRepo(db)
	roles := service.NewRBACService(repository.NewRoleRepo(db), repository.NewUserRepo(db), repository.NewMenuRepo(db), enforcer)
	svc := ticket.NewTicketService(db, repository.NewTicketRepo(db), orgs, reg, roles, service.NewOrgDelegationService(db), nil, nil, nil)
	actions := ticket.NewActionRegistry()
	require.NoError(t, actions.Register(&ticket.NotifyAction{Svc: svc}))
	require.NoError(t, actions.Register(ticket.NewSpawnTaskAction(nil)))
	svc.SetActionRegistry(actions)
	audit := peopleAudit{}
	f.engine = router.New(router.Deps{
		DBPool: db, JWTManager: f.tokens, AuditService: audit, RedisClient: startStubRedis(t),
		Enforcer: enforcer, RoleFetcher: roles, Registry: reg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		TicketHandler: handler.NewTicketHandler(svc, audit, nil),
	})
	return f
}

func (f *definitionHTTPFixture) request(t *testing.T, actor int64, method, path, operation string, body any, status int) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, f.newRequest(t, actor, method, path, operation, body))
	require.Equal(t, status, w.Code, "%s %s: %s", method, path, w.Body.String())
	return w
}

func (f *definitionHTTPFixture) newRequest(t *testing.T, actor int64, method, path, operation string, body any) *http.Request {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	at, _, err := f.tokens.GenerateAccessTokenWithSessionEpoch(actor, strconv.FormatInt(actor, 10), false, 0, 3)
	require.NoError(t, err)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+at)
	req.Header.Set("Content-Type", "application/json")
	if operation != "" {
		req.Header.Set("Idempotency-Key", operation)
	}
	return req
}

type definitionHTTPPlan struct {
	ID       string `json:"plan_id"`
	Hash     string `json:"canonical_hash"`
	Warnings []struct {
		ID string `json:"id"`
	} `json:"warnings"`
}

type definitionHTTPRef struct {
	Code    string `json:"type_code"`
	Version string `json:"definition_version"`
	Hash    string `json:"definition_hash"`
}

type definitionHTTPReceipt struct {
	Operation string            `json:"operation_id"`
	Changed   bool              `json:"changed"`
	Ref       definitionHTTPRef `json:"definition_ref"`
	Revision  string            `json:"draft_revision"`
}

func definitionHTTPData[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var body struct {
		Data T `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body), response.Body.String())
	return body.Data
}

func requireDefinitionReason(t *testing.T, response *httptest.ResponseRecorder, reason string) {
	t.Helper()
	require.Equal(t, reason, response.Header().Get("X-Ticket-Definition-Reason"))
}

func (f *definitionHTTPFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	_, err := f.db.Exec(context.Background(), sql, args...)
	require.NoError(t, err)
}

func (f *definitionHTTPFixture) baseline(t *testing.T, code string) map[string]any {
	t.Helper()
	var revision string
	var published *string
	err := f.db.QueryRow(context.Background(), `SELECT version::text,current_published_version::text FROM ticket_types WHERE code=$1`, code).Scan(&revision, &published)
	require.NoError(t, err)
	return map[string]any{"code": code, "expected_draft_revision": revision, "expected_published_version": published}
}

func (f *definitionHTTPFixture) plan(t *testing.T, base map[string]any) definitionHTTPPlan {
	t.Helper()
	resp := f.request(t, f.publisher, http.MethodPost, definitionPlanPath, "", base, http.StatusOK)
	plan := definitionHTTPData[definitionHTTPPlan](t, resp)
	require.Regexp(t, "^[1-9][0-9]*$", plan.ID)
	require.Regexp(t, "^[0-9a-f]{64}$", plan.Hash)
	return plan
}

func definitionPublishBody(base map[string]any, plan definitionHTTPPlan) map[string]any {
	acknowledged := make([]string, 0, len(plan.Warnings))
	for _, warning := range plan.Warnings {
		acknowledged = append(acknowledged, warning.ID)
	}
	return map[string]any{
		"code": base["code"], "expected_draft_revision": base["expected_draft_revision"],
		"expected_published_version": base["expected_published_version"], "plan_id": plan.ID,
		"canonical_hash": plan.Hash, "acknowledged_warning_ids": acknowledged,
	}
}

func (f *definitionHTTPFixture) assertPublication(t *testing.T, code string, pointer *int64, definitions, receipts int) {
	t.Helper()
	var actualPointer *int64
	var actualDefinitions, actualReceipts int
	require.NoError(t, f.db.QueryRow(context.Background(), `
		SELECT current_published_version,
			(SELECT count(*) FROM ticket_type_definitions d WHERE d.type_code=t.code),
			(SELECT count(*) FROM ticket_definition_publication_receipts r WHERE r.type_code=t.code)
		FROM ticket_types t WHERE code=$1`, code).Scan(&actualPointer, &actualDefinitions, &actualReceipts))
	require.Equal(t, pointer, actualPointer, "current pointer")
	require.Equal(t, definitions, actualDefinitions, "definitions")
	require.Equal(t, receipts, actualReceipts, "success receipts")
}

func TestD01DefinitionPublicationHTTP(t *testing.T) {
	f := newDefinitionHTTPFixture(t)
	f.exec(t, `INSERT INTO role_menus(role_id,menu_id)
		SELECT r.id,m.id FROM roles r CROSS JOIN menus m
		WHERE r.code='d01_http_publisher' AND m.code IN ('ticket_type_write_btn','ticket_type_manage')`)
	t.Run("current configuration duty", func(t *testing.T) {
		const code = "d01_http_admission"
		f.exec(t, `INSERT INTO ticket_types(code,name) VALUES($1,'Admission fixture')`, code)
		base := f.baseline(t, code)
		// Existing draft-management menus and both policy copies grant
		// access, but cannot substitute for the assigned publication button.
		f.request(t, f.publisher, http.MethodPost, definitionPlanPath, "", base, http.StatusForbidden)
		f.assertPublication(t, code, nil, 0, 0)
	})
	f.exec(t, `
		INSERT INTO role_menus(role_id,menu_id)
		SELECT r.id,m.id FROM roles r CROSS JOIN menus m
		WHERE r.code='d01_http_publisher' AND m.code='ticket_definition_publish_btn'`)
	one, two := int64(1), int64(2)

	t.Run("B3 ticket actions are checked at publication", func(t *testing.T) {
		const code = "d01_http_b3_actions"
		const update = `{"type_name":"review","document_id":"1","generation":"2","expected_revision":"3","grant_ref":"test-grant-review-v1","grant_revision":"4","patch":{"approved":true,"score":1.0}}`
		f.exec(t, `INSERT INTO ticket_types(code,name) VALUES($1,'B3 publication fixture')`, code)
		postAction := func(action, params string) map[string]any {
			return map[string]any{"name": "spawn_task", "params": map[string]any{"action": action, "params": json.RawMessage(params)}}
		}
		saveDraft := func(action map[string]any) {
			meta := map[string]any{"open>assigned": map[string]any{"post_actions": []any{action}}}
			f.request(t, f.publisher, http.MethodPost, "/api/v1/ticket-types/update", "", map[string]any{"code": code, "transition_meta": meta}, http.StatusOK)
		}
		for _, tc := range []struct{ name, action, params string }{
			{"missing report profile", "ticket.evidence.report.generate", "{}"},
			{"unknown report profile", "ticket.evidence.report.generate", `{"report_profile":"unknown"}`},
			{"report parameters are server-derived", "ticket.evidence.report.generate", `{"report_profile":"evidence_summary_v1","submitted_field_count":2}`},
			{"read is reserved for subscriptions", "activelist.document.get", "{}"},
			{"empty update patch", "activelist.document.update", strings.Replace(update, `{"approved":true,"score":1.0}`, "{}", 1)},
			{"non-scalar update field", "activelist.document.update", strings.Replace(update, "true", "[]", 1)},
			{"out-of-range update number", "activelist.document.update", strings.Replace(update, "1.0", "9007199254740992", 1)},
			{"invalid update target", "activelist.document.update", strings.Replace(update, `"document_id":"1"`, `"document_id":"01"`, 1)},
			{"missing update grant", "activelist.document.update", strings.Replace(update, `"grant_ref":"test-grant-review-v1",`, "", 1)},
			{"invalid revision", "activelist.document.update", strings.Replace(update, `"expected_revision":"3"`, `"expected_revision":"0"`, 1)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				saveDraft(postAction(tc.action, tc.params))
				f.request(t, f.publisher, http.MethodPost, definitionPlanPath, "", f.baseline(t, code), http.StatusBadRequest)
				f.assertPublication(t, code, nil, 0, 0)
			})
		}
		saveDraft(postAction("ordinary.fixture", "null"))
		base := f.baseline(t, code)
		f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-b3-ordinary", definitionPublishBody(base, f.plan(t, base)), http.StatusOK)
		f.assertPublication(t, code, &one, 1, 1)

		saveDraft(postAction("activelist.document.update", update))
		base = f.baseline(t, code)
		plan := f.plan(t, base)
		// Simulate an older plan for a draft without the required input.
		f.exec(t, `UPDATE ticket_types SET transition_meta=jsonb_set(transition_meta, '{open>assigned,post_actions,0,params,params,patch}', '{}') WHERE code=$1`, code)
		f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-b3-invalid-plan", definitionPublishBody(base, plan), http.StatusBadRequest)
		f.assertPublication(t, code, &one, 1, 1)

		saveDraft(postAction("activelist.document.update", update))
		base = f.baseline(t, code)
		f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-b3-update", definitionPublishBody(base, f.plan(t, base)), http.StatusOK)
		f.assertPublication(t, code, &two, 2, 2)
	})

	t.Run("published configuration lifecycle", func(t *testing.T) {
		const code = "d01_http_publication"
		f.exec(t, `INSERT INTO ticket_types(code,name) VALUES($1,'Published form A')`, code)
		readPath := "/api/v1/ticket-types/" + code + "/published"
		draftPath := "/api/v1/ticket-types/" + code + "/fields"
		got := f.request(t, f.creator, http.MethodGet, readPath, "", nil, http.StatusServiceUnavailable)
		requireDefinitionReason(t, got, "definition_unpublished")
		f.request(t, f.viewer, http.MethodGet, draftPath, "", nil, http.StatusForbidden)
		f.request(t, f.viewer, http.MethodGet, readPath, "", nil, http.StatusForbidden)
		f.request(t, f.publisher, http.MethodGet, draftPath, "", nil, http.StatusOK)
		f.request(t, f.publisher, http.MethodPost, definitionPlanPath, "", map[string]any{
			"code": code, "expected_draft_revision": "01", "expected_published_version": nil,
		}, http.StatusBadRequest)

		base := f.baseline(t, code)
		staleDraft := definitionPublishBody(base, f.plan(t, base))
		f.request(t, f.publisher, http.MethodPost, "/api/v1/ticket-types/fields/replace", "", map[string]any{
			"code": code, "fields": []map[string]any{{"field_key": "plan", "field_label": "Plan A", "field_type": "input"}},
		}, http.StatusOK)
		stale := f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-stale-draft", staleDraft, http.StatusConflict)
		requireDefinitionReason(t, stale, "draft_changed")
		f.assertPublication(t, code, nil, 0, 0)

		base = f.baseline(t, code)
		first := definitionPublishBody(base, f.plan(t, base))
		stalePointer := definitionPublishBody(base, f.plan(t, base))
		f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "", first, http.StatusBadRequest)
		// Simulate a stale compiled L1 policy after a revocation of the durable grant.
		f.exec(t, `DELETE FROM casbin_rule WHERE ptype='p' AND v0='role::d01_http_publisher' AND v1=$1 AND v2='POST'`, definitionPublishPath)
		f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-op-a", first, http.StatusForbidden)
		f.assertPublication(t, code, nil, 0, 0)
		f.exec(t, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p','role::d01_http_publisher',$1,'POST')`, definitionPublishPath)
		receiptA := definitionHTTPData[definitionHTTPReceipt](t, f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-op-a", first, http.StatusOK))
		require.Equal(t, "d01-op-a", receiptA.Operation)
		require.True(t, receiptA.Changed)
		require.Equal(t, "2", receiptA.Revision)
		require.Equal(t, code, receiptA.Ref.Code)
		require.Equal(t, "1", receiptA.Ref.Version)
		require.Regexp(t, "^[0-9a-f]{64}$", receiptA.Ref.Hash)

		stale = f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-stale-pointer", stalePointer, http.StatusConflict)
		requireDefinitionReason(t, stale, "published_pointer_changed")
		f.assertPublication(t, code, &one, 1, 1)
		f.request(t, f.publisher, http.MethodPost, "/api/v1/ticket-types/update", "", map[string]any{
			"code": code, "name": "Draft-only form B", "version": 2,
		}, http.StatusOK)
		f.request(t, f.publisher, http.MethodPost, "/api/v1/ticket-types/fields/replace", "", map[string]any{
			"code": code, "version": 3,
			"fields": []map[string]any{
				{"field_key": "plan", "field_label": "Plan A", "field_type": "input"},
				{"field_key": "env", "field_label": "Draft-only environment", "field_type": "input", "required": true},
			},
		}, http.StatusOK)
		f.assertPublication(t, code, &one, 1, 1)
		type publishedForm struct {
			Type   struct{ Name string } `json:"type"`
			Fields []struct {
				Key string `json:"field_key"`
			} `json:"fields"`
			Ref definitionHTTPRef `json:"definition_ref"`
		}
		got = f.request(t, f.creator, http.MethodGet, readPath, "", nil, http.StatusOK)
		form := definitionHTTPData[publishedForm](t, got)
		require.Equal(t, "Published form A", form.Type.Name)
		require.Equal(t, receiptA.Ref, form.Ref)
		require.Len(t, form.Fields, 1)
		require.Equal(t, "plan", form.Fields[0].Key)
		require.NotContains(t, got.Body.String(), "Draft-only")
		types := f.request(t, f.creator, http.MethodGet, "/api/v1/ticket-types", "", nil, http.StatusOK)
		require.Contains(t, types.Body.String(), "Published form A")
		require.NotContains(t, types.Body.String(), "Draft-only")
		require.Contains(t, f.request(t, f.publisher, http.MethodGet, draftPath, "", nil, http.StatusOK).Body.String(), "Draft-only environment")

		base = f.baseline(t, code)
		nextPlan := f.plan(t, base)
		require.Len(t, nextPlan.Warnings, 1)
		require.NotEmpty(t, nextPlan.Warnings[0].ID)
		unacknowledged := definitionPublishBody(base, nextPlan)
		unacknowledged["acknowledged_warning_ids"] = []string{}
		rejected := f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-op-b", unacknowledged, http.StatusConflict)
		requireDefinitionReason(t, rejected, "warning_acknowledgement_required")
		f.assertPublication(t, code, &one, 1, 1)
		second := definitionPublishBody(base, nextPlan)
		receiptB := definitionHTTPData[definitionHTTPReceipt](t, f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-op-b", second, http.StatusOK))
		require.True(t, receiptB.Changed)
		require.Equal(t, "2", receiptB.Ref.Version)
		require.NotEqual(t, receiptA.Ref.Hash, receiptB.Ref.Hash)
		require.Equal(t, "4", receiptB.Revision)
		form = definitionHTTPData[publishedForm](t, f.request(t, f.creator, http.MethodGet, readPath, "", nil, http.StatusOK))
		require.Equal(t, receiptB.Ref, form.Ref)
		require.Len(t, form.Fields, 2)
		replay := definitionHTTPData[definitionHTTPReceipt](t, f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-op-a", first, http.StatusOK))
		require.Equal(t, receiptA, replay)
		rejected = f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-op-a", second, http.StatusConflict)
		requireDefinitionReason(t, rejected, "operation_payload_changed")
		f.assertPublication(t, code, &two, 2, 2)
		f.exec(t, `DELETE FROM role_menus WHERE role_id=(SELECT id FROM roles WHERE code='d01_http_publisher')
		AND menu_id=(SELECT id FROM menus WHERE code='ticket_definition_publish_btn')`)
		f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-op-a", first, http.StatusForbidden)
		f.exec(t, `INSERT INTO role_menus(role_id,menu_id) SELECT r.id,m.id FROM roles r CROSS JOIN menus m
		WHERE r.code='d01_http_publisher' AND m.code='ticket_definition_publish_btn'`)

		base = f.baseline(t, code)
		noChange := definitionPublishBody(base, f.plan(t, base))
		receiptSame := definitionHTTPData[definitionHTTPReceipt](t, f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-op-same", noChange, http.StatusOK))
		require.False(t, receiptSame.Changed)
		require.Equal(t, receiptB.Ref, receiptSame.Ref)
		f.assertPublication(t, code, &two, 2, 3)
		f.request(t, f.publisher, http.MethodPost, "/api/v1/ticket-types/update", "", map[string]any{
			"code": code, "is_active": false, "version": 4,
		}, http.StatusOK)
		f.request(t, f.creator, http.MethodGet, readPath, "", nil, http.StatusNotFound)
	})

	t.Run("dictionary revision drift", func(t *testing.T) {
		const code = "d01_http_reference"
		f.exec(t, `INSERT INTO ticket_types(code,name) VALUES($1,'Reference fence')`, code)
		f.exec(t, `INSERT INTO dict_types(code,name) VALUES('d01_http_options','Options')`)
		f.exec(t, `INSERT INTO dict_items(type_code,code,label) VALUES('d01_http_options','one','One')`)
		f.request(t, f.publisher, http.MethodPost, "/api/v1/ticket-types/fields/replace", "", map[string]any{
			"code": code, "fields": []map[string]any{{
				"field_key": "options", "field_label": "Options", "field_type": "select", "field_options": []string{"dict:d01_http_options"},
			}},
		}, http.StatusOK)
		base := f.baseline(t, code)
		body := definitionPublishBody(base, f.plan(t, base))
		f.exec(t, `UPDATE dict_types SET version=version+1,name='Revised options' WHERE code='d01_http_options'`)
		stale := f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-reference-drift", body, http.StatusConflict)
		requireDefinitionReason(t, stale, "reference_changed")
		f.assertPublication(t, code, nil, 0, 0)
	})

	t.Run("transition reference revision drift", func(t *testing.T) {
		const code = "d01_http_transition_ref"
		f.exec(t, `INSERT INTO ticket_types(code,name) VALUES($1,'Transition reference fence')`, code)
		f.exec(t, `INSERT INTO dict_types(code,name) VALUES('d01_http_lookup','Lookup')`)
		f.exec(t, `INSERT INTO dict_items(type_code,code,label) VALUES('d01_http_lookup','one','1')`)
		f.exec(t, `INSERT INTO message_templates(code,name,subject,body) VALUES('d01_http_notice','Notice','Subject','Body')`)
		f.request(t, f.publisher, http.MethodPost, "/api/v1/ticket-types/update", "", map[string]any{
			"code": code,
			"transition_meta": json.RawMessage(`{"open>closed":{"validator":"lookup_dict(\"d01_http_lookup\",\"one\") == \"1\"",
				"post_actions":[{"name":"notify","params":{"template":"d01_http_notice"}}]}}`),
		}, http.StatusOK)
		base := f.baseline(t, code)
		body := definitionPublishBody(base, f.plan(t, base))
		f.exec(t, `UPDATE message_templates SET version=version+1,body='Revised body' WHERE code='d01_http_notice'`)
		stale := f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-template-drift", body, http.StatusConflict)
		requireDefinitionReason(t, stale, "reference_changed")
		f.assertPublication(t, code, nil, 0, 0)

		body = definitionPublishBody(base, f.plan(t, base))
		f.exec(t, `UPDATE dict_types SET version=version+1,name='Revised lookup' WHERE code='d01_http_lookup'`)
		stale = f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-lookup-drift", body, http.StatusConflict)
		requireDefinitionReason(t, stale, "reference_changed")
		f.assertPublication(t, code, nil, 0, 0)
	})

	t.Run("receipt write rollback", func(t *testing.T) {
		const code = "d01_http_rollback"
		f.exec(t, `INSERT INTO ticket_types(code,name) VALUES($1,'Receipt rollback')`, code)
		base := f.baseline(t, code)
		body := definitionPublishBody(base, f.plan(t, base))
		f.exec(t, `ALTER TABLE ticket_definition_publication_receipts ADD CONSTRAINT d01_http_test_fail_receipt CHECK (operation_id <> 'd01-op-fault')`)
		t.Cleanup(func() {
			f.exec(t, `ALTER TABLE ticket_definition_publication_receipts DROP CONSTRAINT IF EXISTS d01_http_test_fail_receipt`)
		})
		f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-op-fault", body, http.StatusInternalServerError)
		f.assertPublication(t, code, nil, 0, 0)
		f.exec(t, `ALTER TABLE ticket_definition_publication_receipts DROP CONSTRAINT d01_http_test_fail_receipt`)
		f.request(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-op-fault", body, http.StatusOK)
		f.assertPublication(t, code, &one, 1, 1)
	})

	t.Run("simultaneous retry resolves one success", func(t *testing.T) {
		const code = "d01_http_simultaneous"
		f.exec(t, `INSERT INTO ticket_types(code,name) VALUES($1,'Simultaneous receipt')`, code)
		base := f.baseline(t, code)
		body := definitionPublishBody(base, f.plan(t, base))
		start := make(chan struct{})
		done := make(chan *httptest.ResponseRecorder, 2)
		for range 2 {
			req := f.newRequest(t, f.publisher, http.MethodPost, definitionPublishPath, "d01-simultaneous", body)
			go func() {
				<-start
				w := httptest.NewRecorder()
				f.engine.ServeHTTP(w, req)
				done <- w
			}()
		}
		close(start)
		first, second := <-done, <-done
		require.Equal(t, http.StatusOK, first.Code, first.Body.String())
		require.Equal(t, http.StatusOK, second.Code, second.Body.String())
		require.Equal(t, definitionHTTPData[definitionHTTPReceipt](t, first), definitionHTTPData[definitionHTTPReceipt](t, second))
		f.assertPublication(t, code, &one, 1, 1)
	})
}
