//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func g14Dict(t *testing.T, values ...string) json.RawMessage {
	t.Helper()
	code := "g14dict_" + uniqueSuffix()
	_, err := testPool.Exec(context.Background(), `INSERT INTO dict_types(code,name) VALUES($1,'合成选项集')`, code)
	require.NoError(t, err)
	for i, value := range values {
		_, err = testPool.Exec(context.Background(), `INSERT INTO dict_items(type_code,code,label,sort_order,enabled) VALUES($1,$2,$2,$3,true)`, code, value, i)
		require.NoError(t, err)
	}
	// 停用项不属于 G2 的实际允许集。
	_, err = testPool.Exec(context.Background(), `INSERT INTO dict_items(type_code,code,label,sort_order,enabled) VALUES($1,'disabled','停用项',99,false)`, code)
	require.NoError(t, err)
	return json.RawMessage(`["dict:` + code + `"]`)
}

func g14Type(t *testing.T, svc *Service, kind string, options json.RawMessage) string {
	t.Helper()
	ctx := context.Background()
	code := "g14type_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "合成选项影响"})
	require.NoError(t, err)
	ws, err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: []model.TicketTypeFieldInput{{FieldKey: "choice", FieldLabel: "选项", FieldType: kind, FieldOptions: options}}})
	require.NoError(t, err)
	require.Empty(t, ws, "首次配置没有旧字段，不发影响提示")
	return code
}

func g14InFlight(t *testing.T, svc *Service, actor int64, code, kind string) {
	t.Helper()
	ctx := context.Background()
	value := any("a")
	if kind == "multi_select" {
		value = []string{"a"}
	}
	raw, err := json.Marshal(map[string]any{"choice": value})
	require.NoError(t, err)
	for i := 0; i < 4; i++ {
		data := raw
		if i == 3 {
			data = json.RawMessage(`{}`)
		}
		ticket, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "合成影响单", OrgID: rootOrgID(t), CustomData: data}, actor)
		require.NoError(t, err)
		if i == 2 {
			require.NoError(t, svc.Close(ctx, &model.CloseTicketRequest{ID: ticket.ID, Comment: "终态不计入"}, actor))
		}
	}
	impact, err := svc.FieldImpact(ctx, code)
	require.NoError(t, err)
	require.Equal(t, 2, impact["choice"], "仅两张有该字段的在途单；关闭单和缺键单不计入")
}

func TestG14_ChoiceSetWarnings(t *testing.T) {
	svc, actor, _, _, roles := setupTicket2a(t)
	roles[actor] = []string{"admin"}
	a := g14Dict(t, "a", "b")
	b := g14Dict(t, "a", "c")
	one := g14Dict(t, "a")
	all := g14Dict(t, "a", "b", "c")
	empty := g14Dict(t)
	empty2 := g14Dict(t)
	tests := []struct {
		name, kind string
		old, neu   json.RawMessage
		warning    string
	}{
		{"radio removal", "radio", json.RawMessage(`["a","b"]`), json.RawMessage(`["a"]`), "收窄"},
		{"same cardinality replacement", "select", json.RawMessage(`["a","b"]`), json.RawMessage(`["a","c"]`), "收窄"},
		{"larger set still removes value", "select", json.RawMessage(`["a","b"]`), json.RawMessage(`["a","c","d"]`), "收窄"},
		{"multi same cardinality", "multi_select", json.RawMessage(`["a","b"]`), json.RawMessage(`["a","c"]`), "收窄"},
		{"reorder and label only", "radio", json.RawMessage(`["a","b"]`), json.RawMessage(`[{"label":"乙","value":"b"},{"label":"甲","value":"a"}]`), ""},
		{"add only", "select", json.RawMessage(`["a"]`), json.RawMessage(`["a","b"]`), ""},
		{"dictionary A to B", "radio", a, b, "收窄"},
		{"static to dictionary", "select", json.RawMessage(`["a","b"]`), one, "收窄"},
		{"dictionary to static", "multi_select", a, json.RawMessage(`["a"]`), "收窄"},
		{"dictionary add only", "select", a, all, ""},
		{"dictionary to empty", "radio", a, empty, "收窄"},
		{"static to empty", "select", json.RawMessage(`["a"]`), empty, "收窄"},
		{"empty to static", "select", empty, json.RawMessage(`["a"]`), ""},
		{"empty to dictionary", "multi_select", empty, one, ""},
		{"both empty", "radio", empty, empty2, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initial := tt.old
			if string(initial) == string(empty) {
				initial = json.RawMessage(`["a"]`)
			}
			code := g14Type(t, svc, tt.kind, initial)
			g14InFlight(t, svc, actor, code, tt.kind)
			if string(initial) != string(tt.old) {
				// 存量值在选项集收窄前创建。空集已不允许创建该选项。
				_, err := svc.ReplaceTicketTypeFields(context.Background(), code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: []model.TicketTypeFieldInput{{FieldKey: "choice", FieldLabel: "旧选项", FieldType: tt.kind, FieldOptions: tt.old}}})
				require.NoError(t, err)
			}
			ws, err := svc.ReplaceTicketTypeFields(context.Background(), code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: []model.TicketTypeFieldInput{{FieldKey: "choice", FieldLabel: "更新选项", FieldType: tt.kind, FieldOptions: tt.neu}}})
			require.NoError(t, err, "提示不能阻断合法配置保存")
			if tt.warning == "" {
				require.Empty(t, ws)
			} else {
				require.Len(t, ws, 1)
				assert.Contains(t, ws[0].Reason, tt.warning)
				assert.Equal(t, "choice", ws[0].Field)
				assert.Equal(t, 2, ws[0].InFlight)
			}
			var persisted string
			require.NoError(t, testPool.QueryRow(context.Background(), `SELECT field_options::text FROM ticket_type_fields WHERE type_code=$1 AND field_key='choice'`, code).Scan(&persisted))
			assert.JSONEq(t, string(tt.neu), persisted)
		})
	}
}

func TestG14_StaticEmptyOptionsRejected(t *testing.T) {
	svc, _, _, _, _ := setupTicket2a(t)
	code := g14Type(t, svc, "select", json.RawMessage(`["a"]`))
	_, err := svc.ReplaceTicketTypeFields(context.Background(), code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: []model.TicketTypeFieldInput{{FieldKey: "choice", FieldLabel: "选项", FieldType: "select", FieldOptions: json.RawMessage(`[]`)}}})
	requireErrCode(t, err, errcode.ErrInvalidParams.Code)
	var persisted string
	require.NoError(t, testPool.QueryRow(context.Background(), `SELECT field_options::text FROM ticket_type_fields WHERE type_code=$1`, code).Scan(&persisted))
	assert.JSONEq(t, `["a"]`, persisted)
}

func TestG14_UnassessableOptionsRemainAdvisory(t *testing.T) {
	svc, actor, _, _, roles := setupTicket2a(t)
	roles[actor] = []string{"admin"}
	ref := g14Dict(t, "a", "b")
	role := "g14_read_" + uniqueSuffix()
	_, err := testPool.Exec(context.Background(), "CREATE ROLE "+pgx.Identifier{role}.Sanitize())
	require.NoError(t, err)
	for _, sql := range []string{
		"GRANT USAGE ON SCHEMA public TO " + pgx.Identifier{role}.Sanitize(),
		"GRANT SELECT ON dict_types,tickets TO " + pgx.Identifier{role}.Sanitize(),
		"GRANT SELECT,INSERT,UPDATE,DELETE ON ticket_types,ticket_type_fields TO " + pgx.Identifier{role}.Sanitize(),
		"GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO " + pgx.Identifier{role}.Sanitize(),
	} {
		_, err = testPool.Exec(context.Background(), sql)
		require.NoError(t, err)
	}
	cfg := testPool.Config().Copy()
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET ROLE "+pgx.Identifier{role}.Sanitize())
		return err
	}
	restricted, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	defer restricted.Close()
	limited := *svc
	limited.db = restricted
	limited.ticketRepo = repository.NewTicketRepo(restricted)
	for _, tt := range []struct {
		name     string
		old, neu json.RawMessage
	}{
		{"old dictionary read denied", ref, json.RawMessage(`["a"]`)},
		{"new dictionary read denied", json.RawMessage(`["a"]`), ref},
	} {
		t.Run(tt.name, func(t *testing.T) {
			code := g14Type(t, svc, "select", tt.old)
			g14InFlight(t, svc, actor, code, "select")
			ws, err := limited.ReplaceTicketTypeFields(context.Background(), code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: []model.TicketTypeFieldInput{{FieldKey: "choice", FieldLabel: "可保存", FieldType: "select", FieldOptions: tt.neu}}})
			require.NoError(t, err)
			require.Len(t, ws, 1)
			assert.Contains(t, ws[0].Reason, "无法评估")
			assert.NotContains(t, ws[0].Reason, "收窄")
			assert.Equal(t, 2, ws[0].InFlight)
			var persisted string
			require.NoError(t, testPool.QueryRow(context.Background(), `SELECT field_options::text FROM ticket_type_fields WHERE type_code=$1`, code).Scan(&persisted))
			assert.JSONEq(t, string(tt.neu), persisted)
		})
	}
	t.Run("same reference label change does not invent a historical diff", func(t *testing.T) {
		code := g14Type(t, svc, "select", ref)
		var prettyRef []string
		require.NoError(t, json.Unmarshal(ref, &prettyRef))
		formatted, err := json.MarshalIndent(prettyRef, "", "  ")
		require.NoError(t, err)
		ws, err := limited.ReplaceTicketTypeFields(context.Background(), code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: []model.TicketTypeFieldInput{{FieldKey: "choice", FieldLabel: "仅改名字", FieldType: "select", FieldOptions: formatted}}})
		require.NoError(t, err)
		require.Empty(t, ws)
	})
	t.Run("legacy unparseable options produce advisory", func(t *testing.T) {
		code := g14Type(t, svc, "select", json.RawMessage(`["a"]`))
		g14InFlight(t, svc, actor, code, "select")
		_, err := testPool.Exec(context.Background(), `UPDATE ticket_type_fields SET field_options='[{"bad":"legacy"}]'::jsonb WHERE type_code=$1`, code)
		require.NoError(t, err)
		ws, err := svc.ReplaceTicketTypeFields(context.Background(), code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: []model.TicketTypeFieldInput{{FieldKey: "choice", FieldLabel: "修复旧配置", FieldType: "select", FieldOptions: json.RawMessage(`["a"]`)}}})
		require.NoError(t, err)
		require.Len(t, ws, 1)
		assert.Contains(t, ws[0].Reason, "无法评估")
		assert.Equal(t, 2, ws[0].InFlight)
	})
}

// 注入真实 PG 查询结果/错误，既不复制允许集算法，也不伪造 pgx.Rows。
type g14FaultQuery struct{ query string }

func (q g14FaultQuery) Query(ctx context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return testPool.Query(ctx, q.query)
}

func TestG14_DictStreamErrorsRemainFailOpen(t *testing.T) {
	svc := &Service{}
	ref := json.RawMessage(`["dict:synthetic"]`)
	streamSQL := `SELECT CASE WHEN n=1 THEN 'a' ELSE (1/(n-2))::text END FROM generate_series(1,2)n`
	rows, err := testPool.Query(context.Background(), streamSQL)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var first string
	require.NoError(t, rows.Scan(&first))
	require.Equal(t, "a", first)
	require.False(t, rows.Next())
	require.Error(t, rows.Err(), "必须是已交付首行后的真实迭代错误")
	rows.Close()
	for _, tt := range []struct{ name, query string }{
		{"row iteration error", streamSQL},
		{"scan error", `SELECT NULL::text`},
		{"query error", `SELECT * FROM g14_missing_table`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := svc.expandDictOptions(context.Background(), g14FaultQuery{tt.query}, ref)
			assert.Nil(t, out, "G2 仍按既定故障策略跳过成员校验，不得留下部分允许集")
		})
	}
}

func TestG14_OneConnection(t *testing.T) {
	svc, _, _, _, _ := setupTicket2a(t)
	ref := g14Dict(t, "a", "b")
	cfg := testPool.Config().Copy()
	cfg.MaxConns = 1
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	defer pool.Close()
	low := *svc
	low.db = pool
	low.ticketRepo = repository.NewTicketRepo(pool)
	const n = 3
	codes := make([]string, n)
	for i := range codes {
		codes[i] = g14Type(t, &low, "select", ref)
	}
	start := make(chan struct{})
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for _, code := range codes {
		wg.Add(1)
		go func(code string) {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ws, err := low.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: []model.TicketTypeFieldInput{{FieldKey: "choice", FieldLabel: "单连接", FieldType: "select", FieldOptions: json.RawMessage(`["a"]`)}}})
			if err == nil && (len(ws) != 1 || ws[0].Field != "choice") {
				err = fmt.Errorf("unexpected warnings: %v", ws)
			}
			errs <- err
		}(code)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err, "全部选项查询须在 Repo 写事务前释放连接")
	}
	require.Zero(t, pool.Stat().AcquiredConns())
	code := g14Type(t, &low, "select", ref)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	emptyBefore := pool.Stat().EmptyAcquireCount()
	expanded := low.expandDictOptions(ctx, tx, ref)
	assert.JSONEq(t, `["a","b"]`, string(expanded))
	_, err = low.validateCustomData(ctx, tx, code, json.RawMessage(`{"choice":"outside"}`), nil)
	requireErrCode(t, err, errcode.ErrInvalidParams.Code)
	require.NoError(t, ctx.Err())
	assert.Equal(t, emptyBefore, pool.Stat().EmptyAcquireCount(), "事务内字典/schema读只能复用传入tx")
}
