//go:build integration

package ticket

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type exportWindowSQL struct{}
type exportWindowTrace struct {
	armed      atomic.Bool
	afterCount func()
}

func (t *exportWindowTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, exportWindowSQL{}, d.SQL)
}
func (t *exportWindowTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	sql, _ := ctx.Value(exportWindowSQL{}).(string)
	if d.Err == nil && strings.HasPrefix(sql, "SELECT COUNT(*) FROM tickets") && t.armed.CompareAndSwap(true, false) {
		t.afterCount()
	}
}

// A committed change in another connection after the actual preflight COUNT
// controls the window; no sleep or probabilistic transaction ordering is used.
func TestExportStreamActualLimitAndCount(t *testing.T) {
	_, aid, _, _, _ := setupTicket2a(t)
	oid := childOrgID(t, "exportwindow")
	ctx := context.Background()
	callbackFailure := errors.New("export generator failed")
	cases := []struct {
		name                  string
		initial, delta        int
		cancel, callbackError bool
	}{
		{name: "empty"}, {name: "exact_boundary", initial: 50000},
		{name: "insert_after_count", initial: 49999, delta: 2},
		{name: "insert_under_limit", initial: 3, delta: 2},
		{name: "delete_after_count", initial: 3, delta: -1},
		{name: "reject_precheck", initial: 50001},
		{name: "canceled_before_stream", initial: 3, cancel: true},
		{name: "generator_error", initial: 3, callbackError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prefix := "f16_" + uniqueSuffix()
			seed := func(n int) {
				_, err := testPool.Exec(ctx, `INSERT INTO tickets(type_code,title,status,created_by,org_id,org_path) SELECT 'incident',$1,'closed',$2,$3,'root' FROM generate_series(1,$4)`, prefix, aid, oid, n)
				require.NoError(t, err)
			}
			seed(tc.initial)
			t.Cleanup(func() {
				_, err := testPool.Exec(context.Background(), `DELETE FROM tickets WHERE title=$1`, prefix)
				require.NoError(t, err)
			})
			callCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			trace := &exportWindowTrace{afterCount: func() {
				if tc.delta > 0 {
					seed(tc.delta)
				}
				if tc.delta < 0 {
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
			svc := NewTicketService(pool, repository.NewTicketRepo(pool), repository.NewOrgRepo(pool), resource.NewRegistry(), stubRoleFetcher{aid: {"admin"}}, nil, nil, nil, nil)
			trace.armed.Store(true)
			attempts := int64(0)
			generated := int64(0)
			total, err := svc.ExportStream(callCtx, model.TicketListQuery{Keyword: prefix}, aid, func(*model.Ticket) error {
				attempts++
				if tc.callbackError && attempts == 2 {
					return callbackFailure
				}
				generated++
				return nil
			})
			t.Logf("preflight=%d committed_delta=%d generated=%d returned=%d error=%v", tc.initial, tc.delta, generated, total, err)
			switch {
			case tc.cancel:
				require.ErrorIs(t, err, context.Canceled)
				assert.Zero(t, generated)
			case tc.callbackError:
				require.ErrorIs(t, err, callbackFailure)
				assert.EqualValues(t, 1, generated)
			case tc.initial > MaxExportRows:
				require.ErrorIs(t, err, ErrExportTooMany)
				assert.Zero(t, generated)
			case tc.initial+tc.delta > MaxExportRows:
				require.ErrorIs(t, err, ErrExportTooMany, "do not return a silently truncated successful export")
				assert.EqualValues(t, MaxExportRows, generated, "the oversized row must never reach the generator")
			default:
				require.NoError(t, err)
				assert.EqualValues(t, tc.initial+tc.delta, generated)
				assert.Equal(t, generated, total, "successful audit count must describe generated rows")
			}
		})
	}
}
