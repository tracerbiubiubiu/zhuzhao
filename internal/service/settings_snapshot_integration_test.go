//go:build integration

package service_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	storcfg "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

type settingTraceSQL struct{}
type settingsSnapshotTrace struct {
	pause       atomic.Bool
	cancelWrite atomic.Bool
	entered     chan struct{}
	release     chan struct{}
	cancel      context.CancelFunc
}

func (t *settingsSnapshotTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, settingTraceSQL{}, d.SQL)
}
func (t *settingsSnapshotTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	sql, _ := ctx.Value(settingTraceSQL{}).(string)
	if d.Err != nil {
		return
	}
	if strings.Contains(sql, "FROM system_settings") && !strings.Contains(sql, "WHERE") && t.pause.CompareAndSwap(true, false) {
		close(t.entered)
		<-t.release
	}
	if (strings.Contains(sql, "UPDATE system_settings SET") || strings.Contains(sql, "DELETE FROM system_settings")) && t.cancelWrite.CompareAndSwap(true, false) {
		t.cancel()
	}
}
func settingsTracedService(t *testing.T, trace *settingsSnapshotTrace) (*service.SettingsService, *storcfg.ConfigStore, int64) {
	t.Helper()
	_, store, uid := newSettingsSvc(t)
	cfg := testPool.Config()
	cfg.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return service.NewSettingsService(repository.NewSettingsRepo(pool), store, nil), store, uid
}

func TestSettingsSnapshotSerializesResetAndUpdate(t *testing.T) {
	trace := &settingsSnapshotTrace{entered: make(chan struct{}), release: make(chan struct{})}
	// Close once even if an assertion aborts, so neither a DB query nor cleanup hangs.
	released := false
	defer func() {
		if !released {
			close(trace.release)
		}
	}()
	svc, store, uid := settingsTracedService(t, trace)
	ctx := context.Background()
	roles := []string{"superadmin"}
	for key, val := range map[string]string{"storage.max_upload_bytes": "1048576", "storage.max_presign_batch": "7"} {
		require.NoError(t, svc.Update(ctx, &model.UpdateSettingRequest{Key: key, Value: []byte(val)}, uid, roles))
	}
	trace.pause.Store(true)
	resetDone := make(chan error, 1)
	go func() { resetDone <- svc.Reset(ctx, "storage.max_upload_bytes", uid, roles) }()
	select {
	case <-trace.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("Reset did not reach captured full KV snapshot")
	}
	updateDone := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		updateDone <- svc.Update(ctx, &model.UpdateSettingRequest{Key: "storage.max_presign_batch", Value: []byte(`9`)}, uid, roles)
	}()
	<-started
	completed := false
	select {
	case err := <-updateDone:
		completed = true
		require.NoError(t, err)
		t.Error("Update published while Reset's captured snapshot was still in flight")
	case <-time.After(200 * time.Millisecond):
	}
	close(trace.release)
	released = true
	require.NoError(t, <-resetDone)
	if !completed {
		require.NoError(t, <-updateDone)
	}
	// Read persisted values separately from the active snapshot after the
	// controlled stale-snapshot window.
	live, err := svc.List(ctx)
	require.NoError(t, err)
	for _, row := range live {
		if row.Key == "storage.max_presign_batch" {
			assert.JSONEq(t, `9`, string(row.Value))
		}
	}
	assert.Equal(t, 9, store.Load().MaxPresignBatch, "active snapshot must match DB after controlled Update/Reset interleave")
	assert.EqualValues(t, 10<<20, store.Load().MaxUploadBytes, "Reset must remove only its own override")
}

func TestSettingsSnapshotReloadFailure(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(map[bool]string{false: "update", true: "reset"}[reset], func(t *testing.T) {
			trace := &settingsSnapshotTrace{}
			_, store, uid := newSettingsSvc(t)
			cfg := testPool.Config()
			cfg.ConnConfig.Tracer = trace
			pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
			require.NoError(t, err)
			defer pool.Close()
			svc := service.NewSettingsService(repository.NewSettingsRepo(pool), store, nil)
			roles := []string{"superadmin"}
			ctx := context.Background()
			require.NoError(t, svc.Update(ctx, &model.UpdateSettingRequest{Key: "storage.max_presign_batch", Value: []byte(`7`)}, uid, roles))
			before := store.Load()
			failCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			trace.cancel = cancel
			trace.cancelWrite.Store(true)
			if reset {
				err = svc.Reset(failCtx, "storage.max_presign_batch", uid, roles)
			} else {
				err = svc.Update(failCtx, &model.UpdateSettingRequest{Key: "storage.max_presign_batch", Value: []byte(`9`)}, uid, roles)
			}
			require.Error(t, err, "persisted write followed by failed rebuild must not report success")
			assert.Same(t, before, store.Load(), "failed rebuild must retain the last complete active snapshot")
			if reset {
				require.NoError(t, svc.Reset(ctx, "storage.max_presign_batch", uid, roles))
				assert.Equal(t, 20, store.Load().MaxPresignBatch)
			} else {
				require.NoError(t, svc.Update(ctx, &model.UpdateSettingRequest{Key: "storage.max_presign_batch", Value: []byte(`9`)}, uid, roles))
				assert.Equal(t, 9, store.Load().MaxPresignBatch)
			}
		})
	}
}
