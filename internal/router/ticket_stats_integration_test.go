//go:build integration

package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/stretchr/testify/require"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/router"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
	"github.com/xuri/excelize/v2"
)

func TestTicketStatisticsShareAuthorizedReadSet(t *testing.T) {
	ctx := context.Background()
	pool, _, err := testutil.SetupPostgresShared()
	require.NoError(t, err)
	suffix := fmt.Sprintf("stat%d", time.Now().UnixNano())
	mkUser := func(key string) int64 {
		var id int64
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users(username,employee_no,password,status,session_epoch,session_projection_epoch) VALUES($1::text,$1::text,'synthetic',1,3,3) RETURNING id`, suffix+key).Scan(&id))
		return id
	}
	actor := mkUser("a")
	creator := mkUser("b")
	var org int64
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO organizations(name,code,path) VALUES($1::text,$1::text,$1::text::ltree) RETURNING id`, suffix).Scan(&org))
	_, err = pool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id,ticket_scope) VALUES($1,$2,'all')`, actor, org)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO ticket_types(code,name) VALUES($1::text,$1::text)`, suffix)
	require.NoError(t, err)
	add := func(tp, status string, by int64, due *time.Time) int64 {
		var id int64
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO tickets(type_code,title,status,created_by,org_id,org_path,sla_due_at)
            SELECT $1,$2,$3,$4,id,path,$6 FROM organizations WHERE id=$5 RETURNING id`, tp, suffix, status, by, org, due).Scan(&id))
		return id
	}
	past := time.Now().Add(-24 * time.Hour)
	visible := []int64{add("incident", "open", creator, &past), add(suffix, "open", actor, nil), add(suffix, "closed", actor, &past), add(suffix, "cancelled", actor, &past)}
	hidden := add(suffix, "open", creator, &past)
	cm, err := casbinmodel.NewModelFromFile("../../configs/casbin_model.conf")
	require.NoError(t, err)
	enforcer, err := casbin.NewSyncedEnforcer(cm)
	require.NoError(t, err)
	roles := &stubRoleFetcher{roles: map[int64][]string{actor: {"synthetic_reader"}}}
	grant := func(path string) {
		_, e := enforcer.AddPolicy("role::synthetic_reader", path, "GET")
		require.NoError(t, e)
	}
	grant("/api/v1/tickets/:id")
	reg := resource.NewRegistry()
	svc := ticket.NewTicketService(pool, repository.NewTicketRepo(pool), repository.NewOrgRepo(pool), reg, roles, service.NewOrgDelegationService(pool), nil, nil, nil)
	jwt := jwtpkg.NewManager(jwtpkg.Config{Secret: "synthetic-read-scope-fixture-0123456789", AccessTTL: time.Minute})
	engine := router.New(router.Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DBPool: pool, JWTManager: jwt, RedisClient: startStubRedis(t), AuditService: peopleAudit{}, TicketHandler: handler.NewTicketHandler(svc, nil, nil), Enforcer: enforcer, RoleFetcher: roles, Registry: reg})
	get := func(path string, expected int) []byte {
		t.Helper()
		w := do(engine, jwt, actor, http.MethodGet, "/api/v1/tickets"+path)
		require.Equal(t, expected, w.Code)
		return w.Body.Bytes()
	}
	get("/stats", 403)
	get("/export", 403)
	get("", 403)
	grant("/api/v1/tickets")
	grant("/api/v1/tickets/export")
	grant("/api/v1/tickets/stats")
	get(fmt.Sprintf("/%d", hidden), 404)
	get(fmt.Sprintf("/%d", visible[0]), 200)
	query := "?keyword=" + suffix
	var list struct{ Data model.TicketListResponse }
	require.NoError(t, json.Unmarshal(get(query, 200), &list))
	var ids []int64
	for _, tk := range list.Data.List {
		ids = append(ids, tk.ID)
	}
	require.ElementsMatch(t, visible, ids)
	require.EqualValues(t, 4, list.Data.Total)
	book, err := excelize.OpenReader(bytes.NewReader(get("/export"+query, 200)))
	require.NoError(t, err)
	defer book.Close()
	rows, err := book.GetRows("Sheet1")
	require.NoError(t, err)
	var exported []string
	for _, r := range rows[1:] {
		exported = append(exported, r[0])
	}
	for _, id := range visible {
		require.Contains(t, exported, fmt.Sprint(id))
	}
	require.Len(t, exported, 4)
	var stats struct{ Data model.TicketStats }
	for _, group := range []string{"status", "priority", "type_code"} {
		require.NoError(t, json.Unmarshal(get("/stats"+query+"&group_by="+group+"&page_size=1", 200), &stats))
		require.EqualValues(t, 4, stats.Data.Total)
		require.False(t, stats.Data.AsOf.IsZero())
		if group == "status" {
			require.Len(t, stats.Data.Items, 3)
			for _, item := range stats.Data.Items {
				if item.Key == "open" {
					require.EqualValues(t, 2, item.Count)
					require.EqualValues(t, 2, item.InProgressCount)
					require.EqualValues(t, 1, item.OverdueCount)
					require.NotNil(t, item.OverdueRate)
					require.Equal(t, 0.5, *item.OverdueRate)
				} else {
					require.Zero(t, item.InProgressCount)
					require.Zero(t, item.OverdueCount)
					require.Nil(t, item.OverdueRate)
				}
			}
		}
	}
	// At the exact deadline a visible open ticket is still on time.
	asOf := time.Now().UTC().Truncate(time.Microsecond)
	_, err = pool.Exec(ctx, `UPDATE tickets SET sla_due_at=$1 WHERE id=$2`, asOf, visible[1])
	require.NoError(t, err)
	scope, err := reg.GetFilter(ctx, "ticket", actor, "read")
	require.NoError(t, err)
	atDeadline, err := repository.NewTicketRepo(pool).TicketStats(ctx, scope, model.TicketListQuery{Keyword: suffix, Status: "open"}, "status", asOf)
	require.NoError(t, err)
	require.Len(t, atDeadline.Items, 1)
	require.EqualValues(t, 1, atDeadline.Items[0].OverdueCount)
	get("/stats?group_by=unrecognized", 400)
	for _, path := range []string{"", "/stats", "/export"} {
		get(path+"?created_from=2026-01-02&created_to=2026-01-01", 400)
	}
}
