//go:build integration

package ticket

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type definitionCreateTrace struct {
	code, phase      string
	once             sync.Once
	arrived, release chan struct{}
}
type definitionTraceAfterKey struct{}

func (p *definitionCreateTrace) pause(ctx context.Context) {
	p.once.Do(func() { close(p.arrived) })
	select {
	case <-p.release:
	case <-ctx.Done():
	}
}
func (p *definitionCreateTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if len(d.Args) > 0 && d.Args[0] == p.code && strings.Contains(d.SQL, "SELECT id,is_active,current_published_version") {
		if p.phase == "before" {
			p.pause(ctx)
		} else {
			return context.WithValue(ctx, definitionTraceAfterKey{}, true)
		}
	}
	return ctx
}
func (p *definitionCreateTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if ctx.Value(definitionTraceAfterKey{}) == true {
		p.pause(ctx)
	}
}

func TestD01CreatePublicationInterleaving(t *testing.T) {
	for _, phase := range []string{"before", "after"} {
		t.Run(phase, func(t *testing.T) {
			svc, actor, org := setupTicketAdmin(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			code := "d01_race_" + uniqueSuffix()
			_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
			require.NoError(t, err)
			a := publishD01(t, svc, code, actor)
			_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: rawJSON(`{"open":{"sla_due_hours":3}}`)})
			require.NoError(t, err)
			draft, err := svc.ticketRepo.GetTicketType(ctx, code)
			require.NoError(t, err)
			trace := &definitionCreateTrace{code: code, phase: phase, arrived: make(chan struct{}), release: make(chan struct{})}
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(trace.release) }) }
			defer release()
			cfg := testPool.Config().Copy()
			cfg.ConnConfig.Tracer = trace
			pool, err := pgxpool.NewWithConfig(ctx, cfg)
			require.NoError(t, err)
			defer func() { release(); pool.Close() }()
			traced := *svc
			traced.db = pool
			traced.ticketRepo = repository.NewTicketRepo(pool)
			traced.orgRepo = repository.NewOrgRepo(pool)
			type result struct {
				ticket *model.Ticket
				err    error
			}
			done := make(chan result, 1)
			go func() {
				tk, err := traced.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "interleaving", OrgID: org()}, actor)
				done <- result{tk, err}
			}()
			select {
			case <-trace.arrived:
			case <-ctx.Done():
				t.Fatal("create did not reach publication fence")
			}
			if phase == "before" {
				_, changed, err := svc.PublishTicketType(ctx, draft, actor)
				require.NoError(t, err)
				require.True(t, changed)
				release()
				r := <-done
				requireErrCode(t, r.err, errcode.ErrConcurrentModification.Code)
				require.Nil(t, r.ticket)
				var count int
				require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE type_code=$1`, code).Scan(&count))
				require.Zero(t, count)
			} else {
				// A real PostgreSQL conflicting lock proves create keeps the pointer
				// protected until its ticket/event/attachment transaction commits.
				tx, err := testPool.Begin(ctx)
				require.NoError(t, err)
				_, lockErr := tx.Exec(ctx, `SELECT code FROM ticket_types WHERE code=$1 FOR UPDATE NOWAIT`, code)
				require.Error(t, lockErr)
				require.NoError(t, tx.Rollback(ctx))
				published := make(chan error, 1)
				go func() { _, _, err := svc.PublishTicketType(ctx, draft, actor); published <- err }()
				release()
				r := <-done
				require.NoError(t, r.err)
				require.Equal(t, a.DefinitionVersion, *r.ticket.DefinitionVersion)
				require.Nil(t, r.ticket.SLADueAt, "draft B's deadline must not affect A create")
				require.NoError(t, <-published)
			}
		})
	}
}
