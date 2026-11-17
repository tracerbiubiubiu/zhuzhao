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
	pool  *pgxpool.Pool
	tx    pgx.Tx
	after []func() error
}

var iamLocalGates sync.Map

// This session lock and the business transaction always use the SAME connection.
// It protects cooperating IAM writers in the same PostgreSQL fact database.
const iamGuardKey int64 = 0x5a5a49414d000001

func IAMWriteActive(ctx context.Context) bool {
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
	if state, ok := ctx.Value(iamWriteKey{}).(*iamWriteState); ok {
		if state.pool != pool {
			return result, errors.New("IAM guard cannot span independent pools")
		}
		return fn(ctx)
	}
	// Wait before occupying a pool slot: concurrent waiters cannot consume the
	// small pool while the current writer needs its one guarded connection.
	candidate := make(chan struct{}, 1)
	gateAny, _ := iamLocalGates.LoadOrStore(pool, candidate)
	gate := gateAny.(chan struct{})
	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return result, ctx.Err()
	}
	defer func() { <-gate }()
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
			if unlockErr := conn.QueryRow(cleanup, `SELECT pg_advisory_unlock($1)`, iamGuardKey).Scan(&unlocked); unlockErr != nil || !unlocked {
				_ = conn.Conn().Close(cleanup)
			}
		}
		conn.Release()
	}
	defer release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, iamGuardKey); err != nil {
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
	state := &iamWriteState{pool: pool, tx: tx}
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
	rows, err = IAMDB(ctx, r.db).Query(ctx, `SELECT COALESCE(v0,''),COALESCE(v1,''),COALESCE(v2,'') FROM casbin_rule WHERE ptype='p' ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	policies, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) ([]string, error) {
		p := make([]string, 3)
		err := row.Scan(&p[0], &p[1], &p[2])
		return p, err
	})
	return roles, policies, err
}

func (r *MenuRepo) WithIAMGuard(ctx context.Context, fn func(context.Context) error) error {
	return WithIAMGuard(ctx, r.db, fn)
}

// AcquireIAMTransactionGuard closes reviewed low-level Tx write helpers as
// well. External transaction owners must call it before ANY IAM row lock; they
// do not gain service authorization by owning a transaction. Runtime services
// acquire the full guard before Begin, including all decision reads.
func AcquireIAMTransactionGuard(ctx context.Context, tx pgx.Tx) error {
	if IAMWriteActive(ctx) {
		return nil
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, iamGuardKey); err != nil {
		return fmt.Errorf("acquire IAM transaction guard: %w", err)
	}
	return nil
}
