//go:build integration

package ticket

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	orgdeleg "github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// Removing the exclusive qualification guard lets a direct service/system
// writer commit while an admitted shared command still owns its decision.
func TestC04_QualificationWritersExcludeAdmittedCommands(t *testing.T) {
	for _, action := range []string{"assign", "system_assign", "initial_assign", "delete", "cc", "cc_remove", "system_cc"} {
		t.Run(action, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			fixture, actor, other, _, roles := setupTicket2a(t)
			roles[actor] = []string{"admin"}
			oid := childOrgID(t, "c04_qual")
			tk, err := fixture.Create(ctx, &model.CreateTicketRequest{TypeCode: "incident", Title: "qualification", OrgID: oid}, actor)
			require.NoError(t, err)
			if action == "cc_remove" {
				_, err = fixture.AddCC(ctx, &model.AddTicketCCRequest{TicketID: tk.ID, UserIDs: []int64{other}}, actor)
				require.NoError(t, err)
			}
			cfg, err := pgxpool.ParseConfig(testPool.Config().ConnString())
			require.NoError(t, err)
			cfg.MaxConns = 1
			tr := &qualificationTrace{attempt: make(chan struct{}, 1)}
			cfg.ConnConfig.Tracer = tr
			pool, err := pgxpool.NewWithConfig(ctx, cfg)
			require.NoError(t, err)
			defer pool.Close()
			svc := NewTicketService(pool, repository.NewTicketRepo(pool), repository.NewOrgRepo(pool), resource.NewRegistry(), roles, orgdeleg.NewOrgDelegationService(pool), nil, nil, nil)
			var mutate func() error
			req := &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &other}
			switch action {
			case "assign":
				mutate = func() error { return svc.Assign(ctx, req, actor) }
			case "system_assign":
				mutate = func() error { return svc.assignAsSystem(ctx, req, other) }
			case "initial_assign":
				mutate = func() error { return svc.assignAuthorized(ctx, req, other, true) }
			case "delete":
				mutate = func() error { return svc.Delete(ctx, tk.ID, actor) }
			case "cc":
				mutate = func() error {
					_, err := svc.AddCC(ctx, &model.AddTicketCCRequest{TicketID: tk.ID, UserIDs: []int64{other}}, actor)
					return err
				}
			case "cc_remove":
				mutate = func() error {
					return svc.RemoveCC(ctx, &model.RemoveTicketCCRequest{TicketID: tk.ID, UserID: other}, actor)
				}
			case "system_cc":
				mutate = func() error {
					_, err := svc.addCC(ctx, &model.AddTicketCCRequest{TicketID: tk.ID, UserIDs: []int64{other}}, actor, false)
					return err
				}
			}
			done := make(chan error, 1)
			err = repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
				go func() { done <- mutate() }()
				select {
				case <-tr.attempt:
				case <-time.After(time.Second):
					t.Error("writer never attempted IAM guard")
					return nil
				}
				select {
				case err := <-done:
					t.Errorf("qualification writer committed during admitted command: %v", err)
					done <- err
				case <-time.After(100 * time.Millisecond):
				}
				var status string
				require.NoError(t, tx.QueryRow(ctx, `SELECT status FROM tickets WHERE id=$1`, tk.ID).Scan(&status))
				require.Equal(t, "open", status)
				return nil
			})
			require.NoError(t, err)
			require.NoError(t, <-done)
			require.Zero(t, pool.Stat().CanceledAcquireCount())
			if action != "delete" {
				var auditActor int64
				if strings.Contains(action, "assign") {
					require.NoError(t, testPool.QueryRow(ctx, `SELECT user_id FROM ticket_events WHERE ticket_id=$1 AND action='assigned' ORDER BY id DESC LIMIT 1`, tk.ID).Scan(&auditActor))
					want := actor
					if action != "assign" {
						want = other
					}
					require.Equal(t, want, auditActor)
				}
			}
		})
	}
}

type qualificationTrace struct{ attempt chan struct{} }

func (tr *qualificationTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "pg_advisory_lock") {
		select {
		case tr.attempt <- struct{}{}:
		default:
		}
	}
	return ctx
}
func (*qualificationTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
