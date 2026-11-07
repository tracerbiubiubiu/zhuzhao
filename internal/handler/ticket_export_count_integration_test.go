//go:build integration

package handler_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	ticketsvc "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

type exportCountSQL struct{}
type exportCountTrace struct {
	armed      atomic.Bool
	afterCount func()
}

func (t *exportCountTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, exportCountSQL{}, d.SQL)
}
func (t *exportCountTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	sql, _ := ctx.Value(exportCountSQL{}).(string)
	if d.Err == nil && strings.HasPrefix(sql, "SELECT COUNT(*) FROM tickets") && t.armed.CompareAndSwap(true, false) {
		t.afterCount()
	}
}

type exportCountRoles struct{}

func (exportCountRoles) GetRoleCodesByUserID(context.Context, int64) ([]string, error) {
	return []string{"admin"}, nil
}

type exportCountAudit struct{ entries []middleware.AuditLogEntry }

func (a *exportCountAudit) Insert(_ context.Context, e middleware.AuditLogEntry) error {
	a.entries = append(a.entries, e)
	return nil
}

func TestExportFileCountMatchesAudit(t *testing.T) {
	for _, tc := range []struct {
		name           string
		initial, delta int
		wantStatus     int
		cancel         bool
	}{
		{name: "delete_after_count", initial: 3, delta: -1, wantStatus: 200},
		{name: "insert_under_limit", initial: 3, delta: 2, wantStatus: 200},
		{name: "insert_over_limit", initial: 49999, delta: 2, wantStatus: 400},
		{name: "canceled_before_stream", initial: 3, wantStatus: 500, cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			prefix := fmt.Sprintf("f16_%d", time.Now().UnixNano())
			var uid, oid int64
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO users(username,password,employee_no,status)VALUES($1,'hash',$1,1)RETURNING id`, prefix).Scan(&uid))
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,path,status)VALUES($1::text,$1::text,$1::text::ltree,1)RETURNING id`, prefix).Scan(&oid))
			t.Cleanup(func() {
				_, _ = testPool.Exec(context.Background(), `DELETE FROM tickets WHERE created_by=$1`, uid)
				_, _ = testPool.Exec(context.Background(), `DELETE FROM organizations WHERE id=$1`, oid)
				_, _ = testPool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, uid)
			})
			seed := func(n int) {
				_, err := testPool.Exec(ctx, `INSERT INTO tickets(type_code,title,status,created_by,org_id,org_path)SELECT 'incident',$1::text,'closed',$2,$3,$1::text::ltree FROM generate_series(1,$4)`, prefix, uid, oid, n)
				require.NoError(t, err)
			}
			seed(tc.initial)
			callCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			trace := &exportCountTrace{afterCount: func() {
				if tc.delta > 0 {
					seed(tc.delta)
				} else if tc.delta < 0 {
					_, err := testPool.Exec(ctx, `DELETE FROM tickets WHERE id IN(SELECT id FROM tickets WHERE title=$1 LIMIT 1)`, prefix)
					require.NoError(t, err)
				}
				if tc.cancel {
					cancel()
				}
			}}
			cfg := testPool.Config()
			cfg.ConnConfig.Tracer = trace
			pool, err := pgxpool.NewWithConfig(ctx, cfg)
			require.NoError(t, err)
			defer pool.Close()
			svc := ticketsvc.NewTicketService(pool, repository.NewTicketRepo(pool), repository.NewOrgRepo(pool), resource.NewRegistry(), exportCountRoles{}, nil, nil, nil, nil)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/tickets/export?keyword="+prefix, nil).WithContext(callCtx)
			c.Set("userID", uid)
			audit := &exportCountAudit{}
			temp := t.TempDir()
			t.Setenv("TMPDIR", temp)
			trace.armed.Store(true)
			handler.NewTicketHandler(svc, audit, nil).Export(c)
			assert.Equal(t, tc.wantStatus, rec.Code)
			entries, err := os.ReadDir(temp)
			require.NoError(t, err)
			assert.Empty(t, entries, "failed oversized export must also release partial worksheet spill")
			if tc.wantStatus != http.StatusOK {
				assert.Empty(t, audit.entries, "failed generation must not record a successful export")
				assert.Empty(t, rec.Header().Get("Content-Disposition"), "error must happen before attachment headers or XLSX bytes")
				assert.False(t, bytes.HasPrefix(rec.Body.Bytes(), []byte("PK")))
				return
			}
			require.Len(t, audit.entries, 1)
			book, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
			require.NoError(t, err)
			defer book.Close()
			rows, err := book.GetRows("Sheet1")
			require.NoError(t, err)
			actual := len(rows) - 1
			assert.Equal(t, tc.initial+tc.delta, actual)
			assert.Equal(t, fmt.Sprintf("rows=%d", actual), audit.entries[0].ResponseSummary)
		})
	}
}
