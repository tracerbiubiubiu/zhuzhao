package riskdispatch

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// ReadOperation reads the original operation without applying unfinished groups.
func (d *Dispatcher) ReadOperation(ctx context.Context, actorID int64, operationID string) (ApplyResult, error) {
	if actorID <= 0 || len(operationID) == 0 || len(operationID) > 128 {
		return ApplyResult{}, errcode.ErrNotFound
	}
	tx, err := d.deps.DB.Begin(ctx)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("begin dispatch receipt read: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY"); err != nil {
		return ApplyResult{}, fmt.Errorf("set dispatch receipt snapshot: %w", err)
	}
	var planID uuid.UUID
	var planHash string
	err = tx.QueryRow(ctx, `
		SELECT plan_id, plan_hash FROM risk_dispatch_operations WHERE actor_id = $1 AND operation_id = $2
	`, actorID, operationID).Scan(&planID, &planHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return ApplyResult{}, errcode.ErrNotFound
	}
	if err != nil {
		return ApplyResult{}, fmt.Errorf("read dispatch operation: %w", err)
	}
	plan, err := ReadPlan(ctx, tx, actorID, planID)
	if err != nil {
		return ApplyResult{}, err
	}
	groups := plan.Binding.Plan.Groups
	if plan.Hash != planHash || len(groups) == 0 {
		return ApplyResult{}, errcode.ErrServiceUnavailable
	}

	var expected []RiskExpected
	for _, group := range groups {
		for _, item := range group.Items {
			expected = append(expected, RiskExpected{ID: item.RiskID})
		}
	}
	facts, err := d.deps.Facts.CurrentDispatchFacts(ctx, tx, actorID, expected)
	if err != nil {
		return ApplyResult{}, err
	}
	if len(facts) != len(expected) {
		return ApplyResult{}, errcode.ErrNotFound
	}

	type storedGroup struct {
		key     string
		receipt GroupReceipt
	}
	var persisted []storedGroup
	for _, group := range groups {
		receipt, found, err := readGroupReceipt(ctx, tx, actorID, operationID, group.Key)
		if err != nil {
			return ApplyResult{}, err
		}
		if found {
			persisted = append(persisted, storedGroup{key: group.Key, receipt: receipt})
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ApplyResult{}, fmt.Errorf("end dispatch receipt snapshot: %w", err)
	}
	result := ApplyResult{
		OperationID: operationID, PlanID: planID.String(),
		Status: batchStatus(len(persisted), len(groups)),
		Groups: make([]GroupOutcome, 0, len(persisted)),
	}
	for _, group := range persisted {
		result.Groups = append(result.Groups, d.projectReceipt(ctx, actorID, group.key, group.receipt))
	}
	return result, nil
}

func batchStatus(completedGroups, groupCount int) string {
	switch completedGroups {
	case groupCount:
		return "completed"
	case 0:
		return "pending"
	default:
		return "partial"
	}
}
