//go:build integration

package storage_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

// TestBindAvatar_GCWins covers the gap between service preflight and the durable binding.
// The user lock admits GC first; the binding must reject its newly retired object.
func TestBindAvatar_GCWins(t *testing.T) {
	f := newAvatarRaceFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	key := f.avatar(t)
	lock, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer lock.Rollback(context.Background())
	_, err = lock.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, f.uid)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { _, e := f.svc.BindAvatar(ctx, key, f.uid); done <- e }()
	waitAvatarLock(t, ctx, "%users%", 1)
	require.NoError(t, storageservice.NewGCJob(f.repo, f.client, f.cfg, testLogger(), false).Handle(ctx, nil))
	require.NoError(t, lock.Commit(ctx))
	err = <-done
	require.Error(t, err, "GC-retired object must never become a successful binding")
	require.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	var avatar string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT COALESCE(avatar,'') FROM users WHERE id=$1`, f.uid).Scan(&avatar))
	require.Empty(t, avatar)
	require.True(t, f.client.wasRemoved(key))
}

// TestBindAvatar_BindingWinsGC holds the binding just before commit. GC selects the
// still-unbound candidate and waits for its object lock, then must use a fresh reference snapshot.
func TestBindAvatar_BindingWinsGC(t *testing.T) {
	f := newAvatarRaceFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	key := f.avatar(t)
	gate, err := testPool.Acquire(ctx)
	require.NoError(t, err)
	defer gate.Release()
	_, err = gate.Exec(ctx, `SELECT pg_advisory_lock($1)`, f.uid)
	require.NoError(t, err)
	defer gate.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, f.uid)
	name := fmt.Sprintf("avatar_audit_gate_%d", f.uid)
	_, err = testPool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id=%d THEN PERFORM pg_advisory_xact_lock(%d); END IF; RETURN NEW; END $$`, name, f.uid, f.uid))
	require.NoError(t, err)
	defer testPool.Exec(context.Background(), fmt.Sprintf(`DROP FUNCTION %s()`, name))
	_, err = testPool.Exec(ctx, fmt.Sprintf(`CREATE TRIGGER %s AFTER UPDATE OF avatar ON users FOR EACH ROW EXECUTE FUNCTION %s()`, name, name))
	require.NoError(t, err)
	defer testPool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER %s ON users`, name))
	bound := make(chan error, 1)
	go func() { _, e := f.svc.BindAvatar(ctx, key, f.uid); bound <- e }()
	waitAvatarLock(t, ctx, "UPDATE users SET avatar%", 1)
	collected := make(chan error, 1)
	go func() {
		collected <- storageservice.NewGCJob(f.repo, f.client, f.cfg, testLogger(), false).Handle(ctx, nil)
	}()
	waitAvatarLock(t, ctx, "%file_objects%", 1)
	_, err = gate.Exec(ctx, `SELECT pg_advisory_unlock($1)`, f.uid)
	require.NoError(t, err)
	require.NoError(t, <-bound)
	require.NoError(t, <-collected)
	f.assertCurrent(t, key)
}

// TestBindAvatar_ConcurrentRebindRetiredKey exercises old-key ABA: while A→B is
// retiring A, a second request tries A again. It must observe A's terminal deleted state.
func TestBindAvatar_ConcurrentRebindRetiredKey(t *testing.T) {
	f := newAvatarRaceFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	old, newKey := f.avatar(t), f.avatar(t)
	_, err := f.svc.BindAvatar(ctx, old, f.uid)
	require.NoError(t, err)
	lock, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer lock.Rollback(context.Background())
	_, err = lock.Exec(ctx, `SELECT id FROM file_objects WHERE bucket=$1 AND object_key=$2 FOR UPDATE`, f.bucket, old)
	require.NoError(t, err)
	switched, rebound := make(chan error, 1), make(chan error, 1)
	go func() { _, e := f.svc.BindAvatar(ctx, newKey, f.uid); switched <- e }()
	waitAvatarLock(t, ctx, "UPDATE file_objects SET status%", 1)
	go func() { _, e := f.svc.BindAvatar(ctx, old, f.uid); rebound <- e }()
	waitAvatarLock(t, ctx, "%", 2)
	require.NoError(t, lock.Commit(ctx))
	require.NoError(t, <-switched)
	err = <-rebound
	require.Error(t, err)
	require.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	f.assertCurrent(t, newKey)
	require.True(t, f.client.wasRemoved(old))
}
