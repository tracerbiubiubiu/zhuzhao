//go:build integration

package riskworkflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	baseService "github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

var workflowPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup, err := testutil.SetupPostgresShared()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	workflowPool = pool
	code := m.Run()
	cleanup()
	os.Exit(code)
}

type databaseRoles struct{ pool *pgxpool.Pool }

func (r databaseRoles) GetRoleCodesByUserID(ctx context.Context, actor int64) ([]string, error) {
	return repository.NewRoleRepo(r.pool).GetEffectiveRoleCodes(ctx, actor)
}
func workflowTicketService() *ticket.Service {
	return ticket.NewTicketService(workflowPool, repository.NewTicketRepo(workflowPool), repository.NewOrgRepo(workflowPool), resource.NewRegistry(), databaseRoles{workflowPool}, baseService.NewOrgDelegationService(workflowPool), nil, nil, nil)
}

// A real owning ticket read runs on the IAM-bound child. Same connection alone
// never lets the outer transaction borrow the child's current authorization.
func TestWorkflowSavepointUsesExactChildC04(t *testing.T) {
	f := newReceiptFixture(t)
	_, err := f.admitted(t, f.operation, nil, func(ctx context.Context) (riskremediation.Result, error) {
		err := repository.WithIAMCommandGuard(ctx, workflowPool, func(parentCtx context.Context, parent pgx.Tx) error {
			return repository.WithIAMSavepoint(parentCtx, workflowPool, parent, func(childCtx context.Context, child pgx.Tx) error {
				require.NotSame(t, parent, child)
				require.Same(t, parent.Conn(), child.Conn())
				repo := repository.NewTicketRepo(workflowPool)
				require.NoError(t, repo.RequireGuardedCreateTx(childCtx, workflowPool, child))
				require.ErrorIs(t, repo.RequireGuardedCreateTx(childCtx, workflowPool, parent), errcode.ErrNoPermission)
				require.ErrorIs(t, repo.RequireGuardedCreateTx(parentCtx, workflowPool, child), errcode.ErrNoPermission)
				return f.tickets.WithRiskWorkflowAuthorizationTx(childCtx, child, f.actor, []ticket.RiskWorkflowRoute{{Path: receiptPathPrefix + f.operation, Method: "GET"}}, func(current context.Context) error {
					require.NoError(t, f.tickets.CheckRiskWorkflowReadTx(current, child, f.actor, f.ticket))
					require.ErrorIs(t, f.tickets.CheckRiskWorkflowReadTx(current, parent, f.actor, f.ticket), errcode.ErrNoPermission)
					return nil
				})
			})
		})
		return riskremediation.Result{}, err
	})
	require.NoError(t, err)
}

// Exercise the actual kernel atomic. A caller may handle the callback error and
// commit its outer transaction: neither a child fact nor projection may escape.
func TestFinishKernelCallbackUsesChildAndRollsBackFailure(t *testing.T) {
	f := newReceiptFixture(t)
	stop := errors.New("synthetic prelock failure after admitted child write")
	called, projected := false, false
	operation := "rollback-" + f.operation
	err := repository.WithIAMCommandGuard(f.ctx, workflowPool, func(ctx context.Context, outer pgx.Tx) error {
		unavailable := func(context.Context, pgx.Tx, riskremediation.Access) error { return stop }
		callbacks := riskremediation.Callbacks{
			Admit: func(ctx context.Context, child pgx.Tx, _ riskremediation.Access) error {
				called = true
				require.NotSame(t, outer, child)
				require.Same(t, outer.Conn(), child.Conn())
				repo := repository.NewTicketRepo(workflowPool)
				require.NoError(t, repo.RequireGuardedCreateTx(ctx, workflowPool, child))
				require.ErrorIs(t, repo.RequireGuardedCreateTx(ctx, workflowPool, outer), errcode.ErrNoPermission)
				_, err := child.Exec(ctx, `INSERT INTO risk_operations(actor_id,operation_id,request_hash,command,result_status) VALUES($1,$2,decode(repeat('ab',32),'hex'),'synthetic','completed')`, f.actor, operation)
				if err != nil {
					return err
				}
				return repository.AfterIAMCommit(ctx, func() error { projected = true; return nil })
			},
			Prelock: unavailable, Authorize: unavailable,
			TicketPolicy: func(context.Context, pgx.Tx, riskremediation.TicketCheck) (riskremediation.Policy, error) {
				return riskremediation.Policy{}, stop
			},
			Evidence:    func(context.Context, pgx.Tx, riskremediation.EvidenceCheck) error { return stop },
			ApplyTicket: func(context.Context, pgx.Tx, riskremediation.TicketEffect) (int64, error) { return 0, stop },
		}
		_, err := riskremediation.New(workflowPool, callbacks).FinishTx(ctx, outer, riskremediation.FinishCommand{
			Command: riskremediation.Command{ActorID: f.actor, TicketID: f.ticket, ExpectedTicketRevision: 1, OperationID: "command-" + operation},
			Items:   []riskremediation.Item{{LinkID: 1, ExpectedLinkRevision: 1, ExpectedRiskRevision: 1, Resolution: "fixed", Measures: "synthetic measure", Evidence: []riskremediation.EvidenceRef{{Kind: "file", Ref: "synthetic", Version: "1"}}}},
		})
		require.ErrorIs(t, err, stop)
		var count int
		require.NoError(t, outer.QueryRow(ctx, `SELECT count(*) FROM risk_operations WHERE actor_id=$1 AND operation_id=$2`, f.actor, operation).Scan(&count))
		require.Zero(t, count)
		return nil
	})
	require.NoError(t, err)
	require.True(t, called)
	require.False(t, projected)
	var count int
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT count(*) FROM risk_operations WHERE actor_id=$1 AND operation_id=$2`, f.actor, operation).Scan(&count))
	require.Zero(t, count)
}
