//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type validationPoolRoleFetcher struct{ repo *repository.RoleRepo }

func (f validationPoolRoleFetcher) GetRoleCodesByUserID(ctx context.Context, id int64) ([]string, error) {
	return f.repo.GetEffectiveRoleCodes(ctx, id)
}

type validationPoolTraceKey struct{}
type validationPoolBarrier struct {
	target  int64
	arrived atomic.Int64
	done    chan struct{}
	once    sync.Once
}

func (b *validationPoolBarrier) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "SELECT custom_data FROM tickets WHERE id=$1") {
		return context.WithValue(ctx, validationPoolTraceKey{}, true)
	}
	return ctx
}
func (b *validationPoolBarrier) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if ctx.Value(validationPoolTraceKey{}) != true {
		return
	}
	if b.arrived.Add(1) == b.target {
		b.once.Do(func() { close(b.done) })
	}
	select {
	case <-b.done:
	case <-ctx.Done():
	}
}

func TestValidationReadsReuseTransaction(t *testing.T) {
	ctx := context.Background()
	admin := testPool
	dsn := admin.Config().ConnString()
	suffix := uniqueSuffix()
	orgCode, dictCode := "f15_org_"+suffix, "f15_dict_"+suffix
	var orgID int64
	err := admin.QueryRow(ctx, `INSERT INTO organizations(code,name,path)VALUES($1::text,$1::text,$1::text::ltree)RETURNING id`, orgCode).Scan(&orgID)
	require.NoError(t, err)
	var actor int64
	require.NoError(t, admin.QueryRow(ctx, `INSERT INTO users(username,employee_no,password)VALUES($1,$1,'non-login-audit-fixture')RETURNING id`, "E815_"+suffix).Scan(&actor))
	_, err = admin.Exec(ctx, `INSERT INTO user_roles(user_id,role_id)SELECT $1,id FROM roles WHERE code='admin'`, actor)
	require.NoError(t, err)
	roles, err := repository.NewRoleRepo(admin).GetEffectiveRoleCodes(ctx, actor)
	require.NoError(t, err)
	require.Contains(t, roles, "admin", "fixture must be legitimately authorized before testing pool behavior")
	_, err = admin.Exec(ctx, `INSERT INTO dict_types(code,name)VALUES($1,$1)`, dictCode)
	require.NoError(t, err)
	_, err = admin.Exec(ctx, `INSERT INTO dict_items(type_code,code,label)VALUES($1,'old','old'),($1,'new','new')`, dictCode)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, q := range []string{
			`DELETE FROM tickets WHERE created_by=$1`,
			`DELETE FROM ticket_events WHERE user_id=$1`,
			`DELETE FROM file_objects WHERE created_by=$1`,
			`DELETE FROM user_roles WHERE user_id=$1`,
			`DELETE FROM users WHERE id=$1`,
		} {
			_, err := admin.Exec(cleanCtx, q, actor)
			if err != nil {
				t.Errorf("cleanup owned fixture: %v", err)
			}
		}
		if _, err := admin.Exec(cleanCtx, `DELETE FROM ticket_type_fields WHERE type_code LIKE $1`, "f15_"+suffix+"_%"); err != nil {
			t.Errorf("cleanup fields: %v", err)
		}
		if _, err := admin.Exec(cleanCtx, `DELETE FROM ticket_types WHERE code LIKE $1`, "f15_"+suffix+"_%"); err != nil {
			t.Errorf("cleanup types: %v", err)
		}
		if _, err := admin.Exec(cleanCtx, `DELETE FROM dict_types WHERE code=$1`, dictCode); err != nil {
			t.Errorf("cleanup dict: %v", err)
		}
		if _, err := admin.Exec(cleanCtx, `DELETE FROM organizations WHERE id=$1`, orgID); err != nil {
			t.Errorf("cleanup org: %v", err)
		}
	})
	type runCase struct {
		name, kind string
		pool       int32
		workers    int
		dynamic    bool
	}
	cases := []runCase{{"dict_pool1", "dict", 1, 1, true}, {"file_pool1", "file", 1, 1, true}, {"dict_pool2_free", "dict", 2, 1, true}, {"file_pool2_free", "file", 2, 1, true}, {"dict_literal_pool1", "dict", 1, 1, false}, {"file_no_mime_pool1", "file", 1, 1, false}, {"dict_pool2_full", "dict", 2, 2, true}, {"file_pool2_full", "file", 2, 2, true}}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			caseSuffix := fmt.Sprintf("%s_%d", suffix, i)
			code := "f15_" + caseSuffix
			_, err := admin.Exec(ctx, `INSERT INTO ticket_types(code,name)VALUES($1,$1)`, code)
			require.NoError(t, err)
			key, typ, options := "choice", "select", `["old","new"]`
			if c.kind == "dict" && c.dynamic {
				options = fmt.Sprintf(`["dict:%s"]`, dictCode)
			}
			if c.kind == "file" {
				key, typ, options = "upload", "file", `[]`
				if c.dynamic {
					options = `["image/png"]`
				}
			}
			_, err = admin.Exec(ctx, `INSERT INTO ticket_type_fields(type_code,field_key,field_label,field_type,field_options)VALUES($1,$2,$2,$3,$4::jsonb)`, code, key, typ, options)
			require.NoError(t, err)
			var ids []int64
			var before []string
			var payload []json.RawMessage
			for n := 0; n < c.workers; n++ {
				var id int64
				initial := `{"choice":"old"}`
				next := `{"choice":"new"}`
				if c.kind == "file" {
					initial = `{"upload":[]}`
				}
				require.NoError(t, admin.QueryRow(ctx, `INSERT INTO tickets(type_code,title,priority,status,created_by,org_id,org_path,custom_data)VALUES($1,$1,2,'open',$2,$4,$5::ltree,$3::jsonb)RETURNING id`, code, actor, initial, orgID, orgCode).Scan(&id))
				if c.kind == "file" {
					var fid int64
					objectKey := fmt.Sprintf("ticket_attachment/2026/11/f15-%d.png", id)
					require.NoError(t, admin.QueryRow(ctx, `INSERT INTO file_objects(bucket,object_key,purpose,status,file_name,mime_type,size_bytes,created_by,owner_kind,owner_id)VALUES('f15',$1,'ticket_attachment','confirmed','fixture.png','image/png',1,$2,'ticket',$3::bigint::text)RETURNING id`, objectKey, actor, id).Scan(&fid))
					_, err = admin.Exec(ctx, `INSERT INTO ticket_attachments(ticket_id,file_object_id,uploaded_by)VALUES($1,$2,$3)`, id, fid, actor)
					require.NoError(t, err)
					b, e := json.Marshal(map[string]any{"upload": []string{objectKey}})
					require.NoError(t, e)
					next = string(b)
				}
				ids = append(ids, id)
				before = append(before, initial)
				payload = append(payload, json.RawMessage(next))
			}
			cfg, e := pgxpool.ParseConfig(dsn)
			require.NoError(t, e)
			cfg.MaxConns = c.pool
			var barrier *validationPoolBarrier
			if c.workers > 1 {
				barrier = &validationPoolBarrier{target: int64(c.workers), done: make(chan struct{})}
				cfg.ConnConfig.Tracer = barrier
			}
			pool, e := pgxpool.NewWithConfig(ctx, cfg)
			require.NoError(t, e)
			defer pool.Close()
			// Prewarm all permitted connections before measuring Update; startup latency is not pool-wait evidence.
			var held []*pgxpool.Conn
			for n := int32(0); n < c.pool; n++ {
				conn, err := pool.Acquire(ctx)
				require.NoError(t, err)
				held = append(held, conn)
			}
			for _, conn := range held {
				conn.Release()
			}
			svc := NewTicketService(pool, repository.NewTicketRepo(pool), repository.NewOrgRepo(pool), resource.NewRegistry(), validationPoolRoleFetcher{repository.NewRoleRepo(pool)}, nil, nil, nil, nil)
			callCtx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			start := time.Now()
			errs := make([]error, c.workers)
			var wg sync.WaitGroup
			for n := 0; n < c.workers; n++ {
				wg.Add(1)
				go func(n int) {
					defer wg.Done()
					_, errs[n] = svc.Update(callCtx, &model.UpdateTicketRequest{ID: ids[n], CustomData: payload[n]}, actor)
				}(n)
			}
			wg.Wait()
			elapsed := time.Since(start)
			canceled := pool.Stat().CanceledAcquireCount()
			arrived := int64(0)
			if barrier != nil {
				arrived = barrier.arrived.Load()
			}
			var messages []string
			for _, e := range errs {
				if e == nil {
					messages = append(messages, "")
				} else {
					messages = append(messages, e.Error())
				}
			}
			record := map[string]any{"case": c.name, "pool": c.pool, "workers": c.workers, "dynamic": c.dynamic, "elapsed_ms": elapsed.Milliseconds(), "canceled_pool_acquires": canceled, "barrier_arrived": arrived, "errors": messages}
			b, _ := json.Marshal(record)
			t.Log("validation_pool " + string(b))
			if c.workers > 1 {
				require.Equal(t, int64(c.workers), arrived, "both real Update transactions reached row-lock barrier")
			}
			for n, e := range errs {
				if e != nil {
					require.Greater(t, canceled, int64(0), "do not count unrelated fixture/query errors as nested-acquire proof")
					var stored string
					require.NoError(t, admin.QueryRow(ctx, `SELECT custom_data::text FROM tickets WHERE id=$1`, ids[n]).Scan(&stored))
					require.JSONEq(t, before[n], stored)
					var events int
					require.NoError(t, admin.QueryRow(ctx, `SELECT count(*) FROM ticket_events WHERE ticket_id=$1`, ids[n]).Scan(&events))
					require.Zero(t, events)
					t.Errorf("valid Update blocked while transaction owns pool connection: %v", e)
					continue
				}
				var stored string
				require.NoError(t, admin.QueryRow(ctx, `SELECT custom_data::text FROM tickets WHERE id=$1`, ids[n]).Scan(&stored))
				require.JSONEq(t, string(payload[n]), stored)
				var events int
				require.NoError(t, admin.QueryRow(ctx, `SELECT count(*) FROM ticket_events WHERE ticket_id=$1 AND action=$2`, ids[n], EventUpdated).Scan(&events))
				require.Equal(t, 1, events, "successful Update commits exactly one updated event")
			}
			require.Zero(t, canceled, "valid G2 must use its transaction for every DB read")
		})
	}
}
