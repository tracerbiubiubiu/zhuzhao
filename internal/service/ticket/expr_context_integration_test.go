//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/exprcalc"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type expressionCompletionRows struct {
	pgx.Rows
	completionErr error
	closed        bool
}

func (r *expressionCompletionRows) Close() {
	r.Rows.Close()
	r.closed = true
}

func (r *expressionCompletionRows) Err() error {
	if err := r.Rows.Err(); err != nil {
		return err
	}
	if r.closed {
		return r.completionErr
	}
	return nil
}

type expressionCompletionQuerier struct {
	base repository.RowQuerier
	rows *expressionCompletionRows
}

func (q *expressionCompletionQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	rows, err := q.base.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	q.rows = &expressionCompletionRows{Rows: rows, completionErr: errors.New("read completion failed")}
	return q.rows, nil
}

// Read a real dictionary row, then simulate an I/O error reported only when
// finishing the result. A scanned label alone must not certify a successful read.
func TestExprLookup_ReadCompletionError(t *testing.T) {
	ctx := context.Background()
	code := "expr_completion_" + uniqueSuffix()
	_, err := testPool.Exec(ctx, `INSERT INTO dict_types(code,name) VALUES($1,$1)`, code)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := testPool.Exec(ctx, `DELETE FROM dict_types WHERE code=$1`, code)
		assert.NoError(t, err)
	})
	_, err = testPool.Exec(ctx, `INSERT INTO dict_items(type_code,code,label,enabled) VALUES($1,'item','Known label',true)`, code)
	require.NoError(t, err)
	RegisterExprDictLookup(testPool)
	q := &expressionCompletionQuerier{base: testPool}
	value, err := exprcalc.EvalFormula(fmt.Sprintf(`lookup_dict(%q, "item")`, code), exprcalc.EvalEnv{Context: withDictQuerier(ctx, q)})
	require.Error(t, err)
	assert.Nil(t, value)
	assert.Contains(t, err.Error(), "未命中字典项")
	require.NotNil(t, q.rows)
	assert.True(t, q.rows.closed)
}

type exprPoolTrace struct {
	active             atomic.Bool
	nested             atomic.Int64
	mu                 sync.Mutex
	lookupPID, lockPID uint32
}

func (tr *exprPoolTrace) TraceQueryStart(ctx context.Context, c *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.EqualFold(d.SQL, "begin") {
		tr.active.Store(true)
	}
	tr.mu.Lock()
	if strings.Contains(d.SQL, "SELECT label FROM dict_items") {
		tr.lookupPID = c.PgConn().PID()
	}
	if strings.Contains(d.SQL, "SELECT custom_data FROM tickets") {
		tr.lockPID = c.PgConn().PID()
	}
	tr.mu.Unlock()
	return ctx
}
func (*exprPoolTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func (tr *exprPoolTrace) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	if tr.active.Load() {
		tr.nested.Add(1)
	}
	return ctx
}
func (*exprPoolTrace) TraceAcquireEnd(context.Context, *pgxpool.Pool, pgxpool.TraceAcquireEndData) {}

// The spare connection is held until a watchdog releases it. A regressed
// Background-based nested acquire is therefore bounded without hiding the
// violation: every acquire after BEGIN and the backend PIDs are asserted below.
func TestExprLookup_UpdateReusesTransaction(t *testing.T) {
	ctx := context.Background()
	_, actor, _, _, roles := setupTicket2a(t)
	roles[actor] = []string{"admin"}
	org := childOrgID(t, "exprtx")
	dict := "exprtx_" + uniqueSuffix()
	_, err := testPool.Exec(ctx, `INSERT INTO dict_types(code,name)VALUES($1,$1)`, dict)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO dict_items(type_code,code,label,enabled)VALUES($1,'high','High',true),($1,'low','Low',true)`, dict)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := testPool.Exec(ctx, `DELETE FROM dict_types WHERE code=$1`, dict)
		assert.NoError(t, err)
	})
	for _, kind := range []string{"formula", "visible_when"} {
		t.Run(kind, func(t *testing.T) {
			trace := &exprPoolTrace{}
			cfg := testPool.Config().Copy()
			cfg.MaxConns = 2
			cfg.MinConns = 0
			cfg.ConnConfig.Tracer = trace
			pool, err := pgxpool.NewWithConfig(ctx, cfg)
			require.NoError(t, err)
			defer pool.Close()
			storage := &pkgstorage.StorageConfig{}
			storage.ApplyDefaults()
			svc := NewTicketService(pool, repository.NewTicketRepo(pool), repository.NewOrgRepo(pool), resource.NewRegistry(), roles, nil, repository.NewStorageRepo(pool), pkgstorage.NewConfigStore(storage), nil)
			RegisterExprDictLookup(pool)
			code := "exprtx_" + uniqueSuffix()
			_, err = svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
			require.NoError(t, err)
			expression := fmt.Sprintf(`lookup_dict(%q, lvl)`, dict)
			if kind == "visible_when" {
				expression += ` == "High"`
			}
			meta, err := json.Marshal(map[string]string{kind: expression})
			require.NoError(t, err)
			_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: []model.TicketTypeFieldInput{
				{FieldKey: "lvl", FieldLabel: "Level", FieldType: "input", Required: true, SortOrder: 1},
				{FieldKey: "label", FieldLabel: "Label", FieldType: "input", FieldMeta: meta, SortOrder: 2},
			}})
			require.NoError(t, err)
			created, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, OrgID: org, CustomData: json.RawMessage(`{"lvl":"high","label":"initial"}`)}, actor)
			require.NoError(t, err)
			trace.active.Store(false)
			trace.nested.Store(0)
			trace.mu.Lock()
			trace.lookupPID = 0
			trace.lockPID = 0
			trace.mu.Unlock()
			spare, err := pool.Acquire(ctx)
			require.NoError(t, err)
			var once sync.Once
			release := func() { once.Do(spare.Release) }
			defer release()
			watchdog := time.AfterFunc(2*time.Second, release)
			defer watchdog.Stop()
			callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			updated, err := svc.Update(callCtx, &model.UpdateTicketRequest{ID: created.ID, CustomData: json.RawMessage(`{"lvl":"low","\u001fexpr_context":"attacker"}`)}, actor)
			watchdog.Stop()
			release()
			require.NoError(t, err)
			assert.Zero(t, trace.nested.Load(), "expression must not acquire a second pool connection while Update owns its transaction")
			trace.mu.Lock()
			lookupPID, lockPID := trace.lookupPID, trace.lockPID
			trace.mu.Unlock()
			assert.NotZero(t, lockPID)
			assert.Equal(t, lockPID, lookupPID, "lookup and FOR UPDATE must use the same backend")
			assert.Zero(t, pool.Stat().AcquiredConns())
			var got map[string]any
			require.NoError(t, json.Unmarshal(updated.CustomData, &got))
			assert.Equal(t, "low", got["lvl"])
			assert.Equal(t, "attacker", got["\x1fexpr_context"], "runtime context must never be written into custom_data")
			if kind == "formula" {
				assert.Equal(t, "Low", got["label"])
			} else {
				assert.NotContains(t, got, "label")
			}
		})
	}
}

// Identical source is evaluated by separate DBs and concurrently owned
// transactions. Uncommitted temporary rows identify the exact connection; neither
// the compile cache nor the process-wide fallback registration may retain it.
func TestExprLookup_CompiledProgramDBAndTransactionIsolation(t *testing.T) {
	ctx := context.Background()
	RegisterExprDictLookup(testPool)
	const source = `lookup_dict("same_type", "same_item")`
	var txs []pgx.Tx
	for i := 0; i < 2; i++ {
		name := "exprdb_" + uniqueSuffix()
		_, err := testPool.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
		require.NoError(t, err)
		t.Cleanup(func() {
			_, err := testPool.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
			assert.NoError(t, err)
		})
		cfg := testPool.Config().Copy()
		cfg.ConnConfig.Database = name
		cfg.MaxConns = 1
		cfg.MinConns = 0
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		require.NoError(t, err)
		t.Cleanup(pool.Close)
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
		_, err = tx.Exec(ctx, `CREATE TEMP TABLE dict_items(type_code text,code text,label text,enabled boolean) ON COMMIT DROP`)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `INSERT INTO dict_items VALUES('same_type','same_item',$1,true)`, fmt.Sprintf("db-%d", i))
		require.NoError(t, err)
		txs = append(txs, tx)
	}
	first, err := exprcalc.EvalFormula(source, exprcalc.EvalEnv{Context: withDictQuerier(ctx, txs[0])})
	require.NoError(t, err)
	require.Equal(t, "db-0", first)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i, tx := range txs {
		wg.Add(1)
		go func(i int, tx pgx.Tx) {
			defer wg.Done()
			for n := 0; n < 32; n++ {
				got, err := exprcalc.EvalFormula(source, exprcalc.EvalEnv{Fields: map[string]any{"\x1fexpr_context": "attacker"}, Context: withDictQuerier(ctx, tx)})
				if err != nil || got != fmt.Sprintf("db-%d", i) {
					errs <- fmt.Errorf("DB %d evaluation %d: got=%v err=%v", i, n, got, err)
					return
				}
			}
		}(i, tx)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}
