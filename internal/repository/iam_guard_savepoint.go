package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errIAMSavepointOwned = errors.New("IAM savepoint finalization is owned by its transaction guard")

type iamSavepointTx struct{ pgx.Tx }

func (*iamSavepointTx) Commit(context.Context) error   { return errIAMSavepointOwned }
func (*iamSavepointTx) Rollback(context.Context) error { return errIAMSavepointOwned }

// WithIAMSavepoint carries the exact guarded transaction identity into its own
// savepoint. External projections join the parent's after-commit work only
// after the savepoint is released successfully.
func WithIAMSavepoint(ctx context.Context, pool *pgxpool.Pool, parent pgx.Tx, fn func(context.Context, pgx.Tx) error) error {
	state, ok := ctx.Value(iamWriteKey{}).(*iamWriteState)
	if !ok || state.pool != pool || parent == nil || state.tx != parent || state.savepointOpen || fn == nil {
		return errors.New("IAM savepoint requires its exact guarded transaction")
	}
	child, err := parent.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin IAM savepoint: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = child.Rollback(cleanup)
	}()
	state.savepointOpen = true
	defer func() { state.savepointOpen = false }()
	bound := &iamSavepointTx{Tx: child}
	childState := &iamWriteState{pool: pool, tx: bound, shared: state.shared}
	childCtx := context.WithValue(ctx, iamWriteKey{}, childState)
	if err := fn(childCtx, bound); err != nil {
		return err
	}
	if err := child.Commit(ctx); err != nil {
		return fmt.Errorf("release IAM savepoint: %w", err)
	}
	state.after = append(state.after, childState.after...)
	return nil
}
