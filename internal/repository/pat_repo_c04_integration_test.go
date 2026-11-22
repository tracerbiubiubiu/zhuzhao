//go:build integration

package repository_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// A PAT revocation must wait for an admitted business command's commit.
func TestC04_PATRevokeWaitsForCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var user int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO users(username,password) VALUES($1,'synthetic') RETURNING id`, "c04_pat_"+time.Now().Format("150405.000000000")).Scan(&user))
	pat := &model.PersonalAccessToken{UserID: user, Name: "fixture", SecretHash: time.Now().Format("150405.000000000"), Scope: "full"}
	require.NoError(t, repository.NewPatRepo(testPool).Create(ctx, pat))
	cfg, err := pgxpool.ParseConfig(testPool.Config().ConnString())
	require.NoError(t, err)
	tr := &patC04Trace{attempt: make(chan struct{}, 1)}
	cfg.ConnConfig.Tracer = tr
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	done := make(chan error, 1)
	requestCtx := ctx
	err = repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
		go func() { done <- repository.NewPatRepo(pool).Revoke(requestCtx, pat.ID, user) }()
		select {
		case <-tr.attempt:
		case e := <-done:
			t.Errorf("PAT revoke completed before command commit: %v", e)
			done <- e
			return nil
		case <-time.After(time.Second):
			t.Error("PAT revoke never attempted IAM ordering")
			return nil
		}
		select {
		case e := <-done:
			t.Errorf("PAT revoke overtook command: %v", e)
			done <- e
		case <-time.After(100 * time.Millisecond):
		}
		var active bool
		require.NoError(t, tx.QueryRow(ctx, `SELECT revoked_at IS NULL FROM personal_access_tokens WHERE id=$1`, pat.ID).Scan(&active))
		require.True(t, active)
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, <-done)
	var revoked bool
	require.NoError(t, testPool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM personal_access_tokens WHERE id=$1`, pat.ID).Scan(&revoked))
	require.True(t, revoked)
	require.ErrorIs(t, repository.NewPatRepo(pool).Revoke(ctx, pat.ID, user), repository.ErrPatNotFound)
}

type patC04Trace struct{ attempt chan struct{} }

func (tr *patC04Trace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "pg_advisory_lock(") {
		select {
		case tr.attempt <- struct{}{}:
		default:
		}
	}
	return ctx
}
func (*patC04Trace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
