//go:build integration

package service_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/datafetcher"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	ticketsvc "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// All records live in the package-owned synthetic database. The fetch adapter
// only uses that database; no external source or network service is contacted.
type createC04Fetch struct{ run func() }

func (f createC04Fetch) Fetch(context.Context, datafetcher.FetchSource, string, datafetcher.Actor) (map[string]any, error) {
	f.run()
	return map[string]any{"value": "prepared"}, nil
}

type createC04Fixture struct {
	code             string
	actor, role, org int64
	pool             *pgxpool.Pool
	svc              *ticketsvc.Service
	enforcer         *casbin.SyncedEnforcer
	rbac             *service.RBACService
	trace            *createC04Trace
}

type createC04Trace struct {
	c04Trace
	beforeInsert func(context.Context)
}

func (tr *createC04Trace) TraceQueryStart(ctx context.Context, conn *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	tr.c04Trace.TraceQueryStart(ctx, conn, d)
	if tr.beforeInsert != nil && strings.Contains(d.SQL, "INSERT INTO tickets") {
		tr.beforeInsert(ctx)
	}
	return ctx
}

func newCreateC04(t *testing.T, prepare func(*createC04Fixture)) *createC04Fixture {
	t.Helper()
	ctx := context.Background()
	f := &createC04Fixture{code: "c04create_" + uniqueSuffix()}
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO users(username,password)VALUES($1,'synthetic-non-login')RETURNING id`, f.code).Scan(&f.actor))
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO roles(code,name,priority)VALUES($1::text,$1::text,50)RETURNING id`, f.code).Scan(&f.role))
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,path)VALUES($1::text,$1::text,$1::text::ltree)RETURNING id`, f.code).Scan(&f.org))
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO user_roles(user_id,role_id)VALUES($1,$2)`, []any{f.actor, f.role}},
		{`INSERT INTO user_orgs(user_id,org_id)VALUES($1,$2)`, []any{f.actor, f.org}},
		{`INSERT INTO ticket_types(code,name)VALUES($1::text,$1::text)`, []any{f.code}},
		{`INSERT INTO ticket_templates(code,name,type_code,default_priority,default_fields,org_id,org_path,created_by)VALUES($1::text,$1::text,$1::text,2,'{"description":"template description"}',$2,$1::text::ltree,$3)`, []any{f.code, f.org, f.actor}},
		{`INSERT INTO ticket_type_fields(type_code,field_key,field_label,field_type,field_meta)VALUES($1,'source','source','input','{}'),($1,'result','result','input','{"fetch":{"source":{"type":"internal_db"},"trigger_by":"source","mapping":{"value":"result"}}}')`, []any{f.code}},
		{`INSERT INTO casbin_rule(ptype,v0,v1,v2)VALUES('p',$1,'/api/v1/tickets','POST')`, []any{"role::" + f.code}},
	} {
		_, err := testPool.Exec(ctx, statement.sql, statement.args...)
		require.NoError(t, err)
	}
	cfg, err := pgxpool.ParseConfig(testPool.Config().ConnString())
	require.NoError(t, err)
	cfg.MaxConns = 1
	f.trace = &createC04Trace{c04Trace: c04Trace{pids: make(map[uint32]bool)}}
	cfg.ConnConfig.Tracer = f.trace
	f.pool, err = pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(f.pool.Close)
	m, err := casbinmodel.NewModelFromFile("../../configs/casbin_model.conf")
	require.NoError(t, err)
	f.enforcer, err = casbin.NewSyncedEnforcer(m)
	require.NoError(t, err)
	_, err = f.enforcer.AddPolicy("role::"+f.code, "/api/v1/tickets", "POST")
	require.NoError(t, err)
	f.rbac = service.NewRBACService(repository.NewRoleRepo(f.pool), repository.NewUserRepo(f.pool), repository.NewMenuRepo(f.pool), f.enforcer)
	reg := datafetcher.NewRegistry()
	reg.Register("internal_db", createC04Fetch{run: func() {
		require.Zero(t, f.pool.Stat().AcquiredConns(), "fetch must finish before the command owns a connection")
		if prepare != nil {
			prepare(f)
		}
	}})
	f.svc = ticketsvc.NewTicketService(f.pool, repository.NewTicketRepo(f.pool), repository.NewOrgRepo(f.pool), resource.NewRegistry(), f.rbac, service.NewOrgDelegationService(f.pool), repository.NewStorageRepo(f.pool), pkgstorage.NewConfigStore(&pkgstorage.StorageConfig{Bucket: "c04", MaxAttachmentsPerTicket: 20}), reg)
	return f
}

func (f *createC04Fixture) request(ctx context.Context, purposeChange string) *httptest.ResponseRecorder {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", f.actor)
		c.Set("session_epoch", int64(1)) // 初始数据库安全代际的合成准入事实
		c.Next()
	}, middleware.CasbinAuth(f.enforcer, f.rbac, nil))
	path := "/api/v1/tickets"
	if purposeChange == "wrong_route" {
		path += "/update"
		_, _ = f.enforcer.AddPolicy("role::"+f.code, path, "POST")
	}
	r.POST(path, func(c *gin.Context) {
		if purposeChange == "wrong_actor" {
			c.Set("userID", f.actor+1)
		}
		c.Next()
	}, handler.NewTicketHandler(f.svc, nil, nil).Create)
	body := fmt.Sprintf(`{"type_code":%q,"title":%q,"org_id":"%d","template_code":%q,"custom_data":{"source":"synthetic"}}`, f.code, f.code, f.org, f.code)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func (f *createC04Fixture) counts(t *testing.T, want int) {
	t.Helper()
	var rows, events int
	require.NoError(t, testPool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM tickets WHERE type_code=$1),(SELECT count(*) FROM ticket_events WHERE user_id=$2)`, f.code, f.actor).Scan(&rows, &events))
	require.Equal(t, want, rows)
	require.Equal(t, want, events)
	require.Zero(t, f.pool.Stat().CanceledAcquireCount())
}

func TestC04_CreateRejectsBeforeFetchingOutsideOrganization(t *testing.T) {
	f := newCreateC04(t, func(*createC04Fixture) {
		t.Fatal("a request outside the admitted organization must not call a fetch provider")
	})
	_, err := testPool.Exec(context.Background(), `DELETE FROM user_orgs WHERE user_id=$1`, f.actor)
	require.NoError(t, err)
	w := f.request(context.Background(), "")
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	f.counts(t, 0)
}

func TestC04_CreateHTTPPreparationFence(t *testing.T) {
	for _, change := range []string{"allow", "policy", "role", "account", "membership", "org_deleted", "type_inactive", "type_config", "template", "template_deleted", "template_recreated", "type_version", "type_recreated", "fields", "fetch_mapping", "wrong_route", "wrong_actor", "rollback"} {
		t.Run(change, func(t *testing.T) {
			f := newCreateC04(t, func(f *createC04Fixture) {
				err := repository.WithIAMGuard(context.Background(), testPool, func(ctx context.Context) error {
					q := repository.IAMDB(ctx, testPool)
					var sql string
					var args []any
					switch change {
					case "policy":
						sql, args = `DELETE FROM casbin_rule WHERE v0=$1`, []any{"role::" + f.code}
					case "role":
						sql, args = `UPDATE roles SET status=0 WHERE id=$1`, []any{f.role}
					case "account":
						sql, args = `UPDATE users SET status=0 WHERE id=$1`, []any{f.actor}
					case "membership":
						sql, args = `DELETE FROM user_orgs WHERE user_id=$1`, []any{f.actor}
					case "org_deleted":
						sql, args = `UPDATE organizations SET deleted_at=NOW() WHERE id=$1`, []any{f.org}
					case "type_inactive":
						sql, args = `UPDATE ticket_types SET is_active=false WHERE code=$1`, []any{f.code}
					case "type_config":
						sql, args = `UPDATE ticket_types SET default_sla_hours=7 WHERE code=$1`, []any{f.code}
					case "template":
						sql, args = `UPDATE ticket_templates SET default_fields='{"description":"new template"}' WHERE code=$1`, []any{f.code}
					case "template_deleted":
						sql, args = `DELETE FROM ticket_templates WHERE code=$1`, []any{f.code}
					case "template_recreated":
						if _, err := q.Exec(ctx, `DELETE FROM ticket_templates WHERE code=$1`, f.code); err != nil {
							return err
						}
						sql, args = `INSERT INTO ticket_templates(code,name,type_code,default_priority,default_fields,org_id,org_path,created_by)VALUES($1::text,$1::text,$1::text,2,'{"description":"template description"}',$2,$1::text::ltree,$3)`, []any{f.code, f.org, f.actor}
					case "type_version":
						sql, args = `UPDATE ticket_types SET version=version+1 WHERE code=$1`, []any{f.code}
					case "type_recreated":
						for _, query := range []string{`DELETE FROM ticket_templates WHERE code=$1`, `DELETE FROM ticket_type_fields WHERE type_code=$1`, `DELETE FROM ticket_types WHERE code=$1`} {
							if _, err := q.Exec(ctx, query, f.code); err != nil {
								return err
							}
						}
						sql, args = `INSERT INTO ticket_types(code,name)VALUES($1::text,$1::text)`, []any{f.code}
					case "fields":
						sql, args = `UPDATE ticket_type_fields SET required=true WHERE type_code=$1 AND field_key='result'`, []any{f.code}
					case "fetch_mapping":
						sql, args = `UPDATE ticket_type_fields SET field_meta='{}' WHERE type_code=$1 AND field_key='result'`, []any{f.code}
					}
					if sql == "" {
						return nil
					}
					_, err := q.Exec(ctx, sql, args...)
					return err
				})
				require.NoError(t, err)
			})
			if change == "rollback" {
				constraint := f.code + "_event"
				_, err := testPool.Exec(context.Background(), fmt.Sprintf(`ALTER TABLE ticket_events ADD CONSTRAINT %s CHECK(user_id<>%d) NOT VALID`, constraint, f.actor))
				require.NoError(t, err)
				t.Cleanup(func() {
					_, err := testPool.Exec(context.Background(), `ALTER TABLE ticket_events DROP CONSTRAINT `+constraint)
					require.NoError(t, err)
				})
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			w := f.request(ctx, change)
			if change == "allow" {
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				f.counts(t, 1)
				var priority int
				var description, data, key string
				require.NoError(t, testPool.QueryRow(ctx, `SELECT priority,description,custom_data::text,human_key FROM tickets WHERE type_code=$1`, f.code).Scan(&priority, &description, &data, &key))
				require.Equal(t, 2, priority)
				require.Equal(t, "template description", description)
				require.JSONEq(t, `{"source":"synthetic","result":"prepared"}`, data)
				require.NotEmpty(t, key)
				require.Len(t, f.trace.pids, 1, "fresh authorization and persistence share one connection")
			} else {
				wantHTTP, wantCode := http.StatusConflict, errcode.ErrConcurrentModification.Code
				switch change {
				case "policy", "account", "membership", "wrong_route", "wrong_actor":
					wantHTTP, wantCode = http.StatusForbidden, errcode.ErrNoPermission.Code
				case "role":
					wantHTTP, wantCode = http.StatusForbidden, errcode.ErrNoRoles.Code
				case "org_deleted":
					wantHTTP, wantCode = http.StatusNotFound, errcode.ErrOrgNotFound.Code
				case "type_inactive":
					wantHTTP, wantCode = http.StatusNotFound, errcode.ErrTicketTypeNotFound.Code
				case "rollback":
					wantHTTP, wantCode = http.StatusInternalServerError, errcode.ErrInternal.Code
				}
				require.Equal(t, wantHTTP, w.Code, w.Body.String())
				var response struct {
					Code int `json:"code"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				require.Equal(t, wantCode, response.Code, w.Body.String())
				f.counts(t, 0)
			}
		})
	}
}

func TestC04_CreateCommitsBeforeQueuedRevocation(t *testing.T) {
	f := newCreateC04(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	f.trace.beforeInsert = func(ctx context.Context) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
		}
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- f.request(ctx, "allow") }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	revoked := make(chan error, 1)
	go func() {
		revoked <- repository.WithIAMGuard(ctx, testPool, func(ctx context.Context) error {
			_, err := repository.IAMDB(ctx, testPool).Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1`, "role::"+f.code)
			return err
		})
	}()
	require.Eventually(t, func() bool {
		var waiting bool
		err := testPool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND NOT granted AND database=(SELECT oid FROM pg_database WHERE datname=current_database()))`).Scan(&waiting)
		return err == nil && waiting
	}, 2*time.Second, 10*time.Millisecond, "revocation must queue behind the admitted create")
	once.Do(func() { close(release) })
	w := <-done
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, <-revoked)
	f.counts(t, 1)
	w = f.request(ctx, "allow")
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	f.counts(t, 1)
}

func TestC04_CreateDirectServiceKeepsDefaults(t *testing.T) {
	f := newCreateC04(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, err := testPool.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1`, "role::"+f.code)
	require.NoError(t, err)
	ticket, err := f.svc.Create(ctx, &model.CreateTicketRequest{TypeCode: f.code, Title: f.code, OrgID: f.org, TemplateCode: "missing_optional_template"}, f.actor)
	require.NoError(t, err, "direct service callers do not acquire an HTTP L1 requirement")
	require.Equal(t, 3, ticket.Priority)
	require.Empty(t, ticket.Description)
	f.counts(t, 1)
}

func TestC04_CreateRejectsNewlyAppearedOptionalTemplate(t *testing.T) {
	f := newCreateC04(t, func(f *createC04Fixture) {
		_, err := testPool.Exec(context.Background(), `INSERT INTO ticket_templates(code,name,type_code,default_priority,default_fields,org_id,org_path,created_by)VALUES($1::text,$1::text,$1::text,2,'{}',$2,$1::text::ltree,$3)`, f.code, f.org, f.actor)
		require.NoError(t, err)
	})
	_, err := testPool.Exec(context.Background(), `DELETE FROM ticket_templates WHERE code=$1`, f.code)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	w := f.request(ctx, "allow")
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	f.counts(t, 0)
}

func TestC04_CreateSingleConnectionValidationAndDispatch(t *testing.T) {
	f := newCreateC04(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO dict_types(code,name)VALUES($1::text,$1::text)`, []any{f.code}},
		{`INSERT INTO dict_items(type_code,code,label)VALUES($1,'valid','valid')`, []any{f.code}},
		{`INSERT INTO ticket_type_fields(type_code,field_key,field_label,field_type,field_options)VALUES($1,'choice','choice','select',$2::jsonb),($1,'upload','upload','file','["image/png"]')`, []any{f.code, fmt.Sprintf(`["dict:%s"]`, f.code)}},
		{`INSERT INTO file_objects(bucket,object_key,purpose,status,file_name,mime_type,size_bytes,created_by)VALUES('c04',$1,'attachment_create','confirmed','fixture.png','image/png',1,$2)`, []any{f.code + ".png", f.actor}},
		{`INSERT INTO assignment_rules(type_code,priority,target_org,target_user,enabled,created_by)VALUES($1,1,$2,$3,true,$3)`, []any{f.code, f.org, f.actor}},
	} {
		_, err := testPool.Exec(ctx, statement.sql, statement.args...)
		require.NoError(t, err)
	}
	f.svc.SetDispatcher(ticketsvc.NewDispatchService(repository.NewAssignmentRuleRepo(f.pool), f.svc, nil))
	// This is the same production handler/middleware/route, with MIME and
	// dictionary reads active inside G2 and a real post-commit assignment.
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", f.actor)
		c.Set("session_epoch", int64(1))
		c.Next()
	}, middleware.CasbinAuth(f.enforcer, f.rbac, nil))
	r.POST("/api/v1/tickets", handler.NewTicketHandler(f.svc, nil, nil).Create)
	body := fmt.Sprintf(`{"type_code":%q,"title":%q,"org_id":"%d","template_code":%q,"custom_data":{"source":"synthetic","choice":"valid","upload":[%q]}}`, f.code, f.code, f.org, f.code, f.code+".png")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tickets", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var status string
	var assignee int64
	var events, attachments int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT status,assigned_to,(SELECT count(*) FROM ticket_events WHERE ticket_id=t.id),(SELECT count(*) FROM ticket_attachments WHERE ticket_id=t.id) FROM tickets t WHERE type_code=$1`, f.code).Scan(&status, &assignee, &events, &attachments))
	require.Equal(t, "assigned", status, "automatic assignment must see the committed ticket")
	require.Equal(t, f.actor, assignee)
	require.Equal(t, 2, events, "created and assigned each commit once")
	require.Equal(t, 1, attachments)
	require.Zero(t, f.pool.Stat().CanceledAcquireCount())
	require.Len(t, f.trace.pids, 1)
}

func TestC04_CreateHoldsConfigurationUntilCommit(t *testing.T) {
	for _, config := range []string{"type", "template", "fields"} {
		t.Run(config, func(t *testing.T) {
			f := newCreateC04(t, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			f.trace.beforeInsert = func(ctx context.Context) {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() { done <- f.request(ctx, "allow") }()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			writer, err := testPool.Acquire(ctx)
			require.NoError(t, err)
			defer writer.Release()
			pid := writer.Conn().PgConn().PID()
			query := `UPDATE ticket_types SET transition_meta='{"open":{"sla_due_hours":4}}',version=version+1 WHERE code=$1`
			if config == "template" {
				query = `UPDATE ticket_templates SET default_priority=1,version=version+1 WHERE code=$1`
			} else if config == "fields" {
				query = `UPDATE ticket_type_fields SET required=true WHERE type_code=$1 AND field_key='source'`
			}
			changed := make(chan error, 1)
			go func() { _, err := writer.Exec(ctx, query, f.code); changed <- err }()
			require.Eventually(t, func() bool {
				var waiting bool
				err := testPool.QueryRow(ctx, `SELECT cardinality(pg_blocking_pids($1))>0`, pid).Scan(&waiting)
				return err == nil && waiting
			}, 2*time.Second, 10*time.Millisecond, "configuration writer must wait for create's decision locks")
			once.Do(func() { close(release) })
			w := <-done
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.NoError(t, <-changed)
			f.counts(t, 1)
		})
	}
}
