//go:build integration

package riskdispatch

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

type unavailablePlanGuard struct{}

func (unavailablePlanGuard) WithDispatchTransaction(context.Context, int64, func(context.Context, pgx.Tx) error) error {
	return errcode.ErrServiceUnavailable
}

type transactionalPlanFacts struct{ *stubFacts }

func (s transactionalPlanFacts) CurrentDispatchFacts(ctx context.Context, db Reader, actor int64, expected []RiskExpected) ([]RiskFact, error) {
	if _, ok := db.(pgx.Tx); !ok {
		return nil, errcode.ErrServiceUnavailable
	}
	return s.stubFacts.CurrentDispatchFacts(ctx, db, actor, expected)
}

func TestPlanRequiresCurrentDispatchPermissionBeforePersistence(t *testing.T) {
	f := setupDispatchFixture(t, dispatchTestDB, "plan-denied")
	risk := f.newRisk(t, dispatchTestDB, "plan-denied", "finding", "asset", f.ownerA, "high", testDeadline, "assessed")
	d := newAggregateTestDispatcher(f, map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}, risk)
	d.deps.Authorizer = &stubAuthorizer{allowed: map[string]bool{}}
	result, err := d.Plan(context.Background(), f.actorID, buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 1}))
	require.ErrorIs(t, err, errcode.ErrForbidden)
	require.Empty(t, result.PlanID)
	require.Zero(t, countRows(t, dispatchTestDB, `SELECT count(*) FROM risk_dispatch_plans WHERE actor_id=$1`, f.actorID))
}

func TestPlanFailsClosedWhenIAMGuardUnavailable(t *testing.T) {
	f := setupDispatchFixture(t, dispatchTestDB, "plan-fault")
	risk := f.newRisk(t, dispatchTestDB, "plan-fault", "finding", "asset", f.ownerA, "high", testDeadline, "assessed")
	d := newAggregateTestDispatcher(f, map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}, risk)
	d.deps.Tx = unavailablePlanGuard{}
	result, err := d.Plan(context.Background(), f.actorID, buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 1}))
	require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
	require.Empty(t, result.PlanID)
	require.Zero(t, countRows(t, dispatchTestDB, `SELECT count(*) FROM risk_dispatch_plans WHERE actor_id=$1`, f.actorID))
}

func TestPlanReadsFactsInOwningTransaction(t *testing.T) {
	f := setupDispatchFixture(t, dispatchTestDB, "plan-tx")
	risk := f.newRisk(t, dispatchTestDB, "plan-tx", "finding", "asset", f.ownerA, "high", testDeadline, "assessed")
	d := newAggregateTestDispatcher(f, map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}, risk)
	d.deps.Facts = transactionalPlanFacts{d.deps.Facts.(*stubFacts)}
	result, err := d.Plan(context.Background(), f.actorID, buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 1}))
	require.NoError(t, err)
	require.Len(t, result.Groups, 1)
	require.Equal(t, int64(1), countRows(t, dispatchTestDB, `SELECT count(*) FROM risk_dispatch_plans WHERE actor_id=$1`, f.actorID))
}

type planProjectionFaultTx struct{ pgx.Tx }

type planProjectionFaultRow struct{}

func (planProjectionFaultRow) Scan(...any) error { return errcode.ErrServiceUnavailable }

func (tx planProjectionFaultTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	if strings.Contains(query, "FROM risk_dispatch_groups") {
		return planProjectionFaultRow{}
	}
	return tx.Tx.QueryRow(ctx, query, args...)
}

type planProjectionFaultGuard struct{ db *pgxpool.Pool }

func (g planProjectionFaultGuard) WithDispatchTransaction(ctx context.Context, _ int64, run func(context.Context, pgx.Tx) error) error {
	tx, err := g.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := run(ctx, planProjectionFaultTx{tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func TestPlanProjectionFailureRollsBackSavedPlan(t *testing.T) {
	f := setupDispatchFixture(t, dispatchTestDB, "plan-project-fault")
	risk := f.newRisk(t, dispatchTestDB, "plan-project-fault", "finding", "asset", f.ownerA, "high", testDeadline, "assessed")
	d := newAggregateTestDispatcher(f, map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}}, risk)
	d.deps.Tx = planProjectionFaultGuard{dispatchTestDB}
	result, err := d.Plan(context.Background(), f.actorID, buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 1}))
	require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
	require.Empty(t, result.PlanID)
	require.Zero(t, countRows(t, dispatchTestDB, `SELECT count(*) FROM risk_dispatch_plans WHERE actor_id=$1`, f.actorID))
}
