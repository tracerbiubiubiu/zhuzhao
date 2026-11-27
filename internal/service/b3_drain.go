package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// EnableDrain stops admission and limits the flow to reconciliation.
// Assembly selects this mode before starting any handlers or workers.
func (f *B3Flow) EnableDrain() { f.draining = true }

func (f *B3Flow) drainDelivery(ctx context.Context, delivery repository.B3Delivery) error {
	run, err := f.store.Execution(ctx, delivery.TaskID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && run.Status != "pending") {
		return nil
	}
	if err != nil {
		return err
	}
	if err := f.store.ClaimExecution(ctx, &run); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	defer f.store.ReleaseExecution(ctx, run)
	err = f.drainExecution(ctx, run)
	if errors.Is(err, errcode.ErrServiceUnavailable) {
		// Pending work remains due after the ordinary outbox backoff.
		return nil
	}
	return err
}

func (f *B3Flow) drainExecution(ctx context.Context, run repository.B3Execution) error {
	if !run.RemoteStarted {
		return f.finish(ctx, run, "held", "integration_draining", nil)
	}
	return f.reconcileUpdate(ctx, run)
}
