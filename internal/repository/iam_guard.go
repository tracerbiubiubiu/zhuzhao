package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/semaphore"
)

// IAMDBTX is the connection used by authorization reads and their protected
// writes. Begin on a guarded transaction creates a savepoint, never another
// pool connection.
type IAMDBTX interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type iamWriteKey struct{}
type iamWriteState struct {
	pool          *pgxpool.Pool
	tx            pgx.Tx
	after         []func() error
	shared        bool
	savepointOpen bool
}

var iamLocalGates sync.Map

// This session lock and the business transaction always use the SAME connection.
// It protects cooperating IAM writers in the same PostgreSQL fact database.
const iamGuardKey int64 = 0x5a5a49414d000001

func IAMWriteActive(ctx context.Context) bool {
	state, ok := ctx.Value(iamWriteKey{}).(*iamWriteState)
	return ok && !state.shared
}

func IAMGuardActive(ctx context.Context) bool {
	_, ok := ctx.Value(iamWriteKey{}).(*iamWriteState)
	return ok
}

func IAMDB(ctx context.Context, pool *pgxpool.Pool) IAMDBTX {
	if state, ok := ctx.Value(iamWriteKey{}).(*iamWriteState); ok && state.pool == pool {
		return state.tx
	}
	return pool
}

// AfterIAMCommit defers existing external projections until the database commit
// is acknowledged. Failure still means "DB committed, projection failed".
func AfterIAMCommit(ctx context.Context, fn func() error) error {
	if state, ok := ctx.Value(iamWriteKey{}).(*iamWriteState); ok {
		if state.savepointOpen {
			return errors.New("after-commit projection requires its active IAM savepoint context")
		}
		state.after = append(state.after, fn)
		return nil
	}
	return fn()
}

func WithIAMGuard(ctx context.Context, pool *pgxpool.Pool, fn func(context.Context) error) error {
	_, err := WithIAMGuardValue(ctx, pool, func(ctx context.Context) (struct{}, error) { return struct{}{}, fn(ctx) })
	return err
}

func WithIAMGuardValue[T any](ctx context.Context, pool *pgxpool.Pool, fn func(context.Context) (T, error)) (result T, err error) {
	return withIAMGuardValue(ctx, pool, false, fn)
}

// WithIAMCommandGuard protects a business write that reads IAM facts. It can
// overlap other business commands, while IAM writers serialize with all of them.
func WithIAMCommandGuard(ctx context.Context, pool *pgxpool.Pool, fn func(context.Context, pgx.Tx) error) error {
	_, err := withIAMGuardValue(ctx, pool, true, func(ctx context.Context) (struct{}, error) {
		state := ctx.Value(iamWriteKey{}).(*iamWriteState)
		return struct{}{}, fn(ctx, state.tx)
	})
	return err
}

func withIAMGuardValue[T any](ctx context.Context, pool *pgxpool.Pool, shared bool, fn func(context.Context) (T, error)) (result T, err error) {
	if state, ok := ctx.Value(iamWriteKey{}).(*iamWriteState); ok {
		if state.pool != pool {
			return result, errors.New("IAM guard cannot span independent pools")
		}
		if state.shared && !shared {
			return result, errors.New("IAM command guard cannot acquire IAM write privileges")
		}
		return fn(ctx)
	}
	// Queue local waiters before they occupy a connection. A queued writer
	// receives the gate once existing commands finish, ahead of later commands.
	const exclusiveWeight int64 = 1 << 32
	weight := exclusiveWeight
	lockSQL, unlockSQL := `SELECT pg_advisory_lock($1)`, `SELECT pg_advisory_unlock($1)`
	if shared {
		weight = 1
		lockSQL, unlockSQL = `SELECT pg_advisory_lock_shared($1)`, `SELECT pg_advisory_unlock_shared($1)`
	}
	gateAny, _ := iamLocalGates.LoadOrStore(pool, semaphore.NewWeighted(exclusiveWeight))
	gate := gateAny.(*semaphore.Weighted)
	if err := gate.Acquire(ctx, weight); err != nil {
		return result, err
	}
	gateHeld := true
	defer func() {
		if gateHeld {
			gate.Release(weight)
		}
	}()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return result, fmt.Errorf("acquire IAM connection: %w", err)
	}
	locked, released := false, false
	release := func() {
		if released {
			return
		}
		released = true
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if locked {
			var unlocked bool
			if unlockErr := conn.QueryRow(cleanup, unlockSQL, iamGuardKey).Scan(&unlocked); unlockErr != nil || !unlocked {
				_ = conn.Conn().Close(cleanup)
			}
		}
		conn.Release()
	}
	defer release()
	if _, err = conn.Exec(ctx, lockSQL, iamGuardKey); err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = conn.Conn().Close(cleanup)
		return result, fmt.Errorf("acquire IAM guard: %w", err)
	}
	locked = true
	// Begin only after the lock: NOW() includes the wait, so a membership that
	// expires while waiting cannot be revived by a stale transaction timestamp.
	tx, err := conn.Begin(ctx)
	if err != nil {
		return result, fmt.Errorf("begin IAM transaction: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	state := &iamWriteState{pool: pool, tx: tx, shared: shared}
	guarded := context.WithValue(ctx, iamWriteKey{}, state)
	result, err = fn(guarded)
	if err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return result, fmt.Errorf("commit IAM transaction: %w", err)
	}
	// Adapter reload may need this same pool even when MaxConns=1. The DB
	// transaction is committed; release its session before external projections,
	// while the process gate still serializes this instance through projection.
	release()
	if shared {
		gate.Release(weight)
		gateHeld = false
	}
	for _, after := range state.after {
		if err = after(); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (r *UserRepo) WithIAMGuard(ctx context.Context, fn func(context.Context) error) error {
	return WithIAMGuard(ctx, r.db, fn)
}
func (r *OrgRepo) WithIAMGuard(ctx context.Context, fn func(context.Context) error) error {
	return WithIAMGuard(ctx, r.db, fn)
}
func (r *RoleRepo) WithIAMGuard(ctx context.Context, fn func(context.Context) error) error {
	return WithIAMGuard(ctx, r.db, fn)
}

// CurrentIAMAuthorization reads the effective L1 source and p rows through the
// guarded transaction. Direct IAM management tiers remain a separate query.
func (r *UserRepo) CurrentIAMAuthorization(ctx context.Context, userID int64) ([]string, [][]string, error) {
	rows, err := IAMDB(ctx, r.db).Query(ctx, EffectiveRoleCTE()+`SELECT code FROM roles WHERE id IN(SELECT id FROM expanded) ORDER BY code`, userID)
	if err != nil {
		return nil, nil, err
	}
	roles, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, nil, err
	}
	policies, err := r.CurrentIAMPolicies(ctx)
	return roles, policies, err
}

// CurrentIAMPolicies returns the live Casbin policy projection on the guarded connection.
func (r *UserRepo) CurrentIAMPolicies(ctx context.Context) ([][]string, error) {
	rows, err := IAMDB(ctx, r.db).Query(ctx, `SELECT COALESCE(v0,''),COALESCE(v1,''),COALESCE(v2,'') FROM casbin_rule WHERE ptype='p' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) ([]string, error) {
		p := make([]string, 3)
		err := row.Scan(&p[0], &p[1], &p[2])
		return p, err
	})
}

func (r *MenuRepo) WithIAMGuard(ctx context.Context, fn func(context.Context) error) error {
	return WithIAMGuard(ctx, r.db, fn)
}

// AcquireIAMTransactionGuard closes reviewed low-level Tx write helpers as
// well. External transaction owners must call it before ANY IAM row lock; they
// do not gain service authorization by owning a transaction. Runtime services
// acquire the full guard before Begin, including all decision reads.
func AcquireIAMTransactionGuard(ctx context.Context, tx pgx.Tx) error {
	if state, ok := ctx.Value(iamWriteKey{}).(*iamWriteState); ok {
		if state.shared {
			return errors.New("IAM command guard cannot acquire IAM write privileges")
		}
		return nil
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, iamGuardKey); err != nil {
		return fmt.Errorf("acquire IAM transaction guard: %w", err)
	}
	return nil
}
