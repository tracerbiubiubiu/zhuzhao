package riskdispatch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

const dispatchPlanTTL = 15 * time.Minute

type DispatchBinding struct {
	Target Target
	Plan   Plan
}

type BoundPlan struct {
	ID        uuid.UUID
	Hash      string
	Binding   DispatchBinding
	ExpiresAt time.Time
}

// SavePlanTx binds the preview to the authorized actor and its exact inputs.
func SavePlanTx(ctx context.Context, tx pgx.Tx, actorID int64, target Target, plan Plan, now time.Time) (BoundPlan, error) {
	binding := DispatchBinding{Target: target, Plan: plan}
	body, err := json.Marshal(binding)
	if err != nil {
		return BoundPlan{}, fmt.Errorf("encode dispatch preview: %w", err)
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return BoundPlan{}, fmt.Errorf("generate dispatch preview ID: %w", err)
	}
	now = now.UTC().Truncate(time.Microsecond)
	result := BoundPlan{
		ID: id, Hash: fmt.Sprintf("%x", sha256.Sum256(body)), Binding: binding,
		ExpiresAt: now.Add(dispatchPlanTTL),
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO risk_dispatch_plans (id, actor_id, plan_hash, binding, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, result.ID, actorID, result.Hash, body, now, result.ExpiresAt)
	if err != nil {
		return BoundPlan{}, fmt.Errorf("save dispatch preview: %w", err)
	}
	return result, nil
}

type planReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func ReadPlan(ctx context.Context, db planReader, actorID int64, planID uuid.UUID) (BoundPlan, error) {
	result := BoundPlan{ID: planID}
	var body []byte
	err := db.QueryRow(ctx, `
		SELECT plan_hash, binding, expires_at FROM risk_dispatch_plans WHERE id = $1 AND actor_id = $2
	`, planID, actorID).Scan(&result.Hash, &body, &result.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return BoundPlan{}, errcode.ErrNotFound
	}
	if err != nil {
		return BoundPlan{}, fmt.Errorf("read dispatch preview: %w", err)
	}
	if err := json.Unmarshal(body, &result.Binding); err != nil {
		return BoundPlan{}, fmt.Errorf("decode dispatch preview: %w", err)
	}
	result.ExpiresAt = result.ExpiresAt.UTC()
	return result, nil
}

// BindOperationTx claims a parent operation in the same namespace as other
// risk writes. A replay may resume only the original actor and preview.
func BindOperationTx(ctx context.Context, tx pgx.Tx, actorID int64, operationID string, planID uuid.UUID, hash string) error {
	if !validOperationID(operationID) {
		return errcode.ErrInvalidParams
	}
	plan, err := ReadPlan(ctx, tx, actorID, planID)
	if err != nil {
		return err
	}
	if plan.Hash != hash {
		return errcode.ErrConcurrentModification
	}
	requestHash := sha256.Sum256([]byte(planID.String() + ":" + hash))
	_, err = tx.Exec(ctx, `
		INSERT INTO risk_operations (actor_id, operation_id, request_hash, command, result_status)
		VALUES ($1, $2, $3, 'dispatch', 'pending')
		ON CONFLICT (actor_id, operation_id) DO NOTHING
	`, actorID, operationID, requestHash[:])
	if err != nil {
		return fmt.Errorf("claim dispatch operation: %w", err)
	}
	var command string
	var originalHash []byte
	err = tx.QueryRow(ctx, `
		SELECT command, request_hash FROM risk_operations
		WHERE actor_id = $1 AND operation_id = $2 FOR UPDATE
	`, actorID, operationID).Scan(&command, &originalHash)
	if err != nil {
		return fmt.Errorf("read claimed dispatch operation: %w", err)
	}
	if command != "dispatch" || !bytes.Equal(originalHash, requestHash[:]) {
		return errcode.ErrConcurrentModification
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO risk_dispatch_operations (actor_id, operation_id, plan_id, plan_hash)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (actor_id, operation_id) DO NOTHING
	`, actorID, operationID, planID, hash)
	if err != nil {
		return fmt.Errorf("bind dispatch operation: %w", err)
	}
	return nil
}

// New operation keys are stable opaque identifiers carried by a single URL segment.
func validOperationID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, ch := range id {
		if ch != '-' && ch != '_' && ch != '.' && ch != '~' &&
			(ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') {
			return false
		}
	}
	return id != "." && id != ".."
}
