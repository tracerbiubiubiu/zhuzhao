package audit

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// Pause an actual pump push before Redis sees it, exposing a final flush that
// overtakes its producer without relying on a randomly full or empty channel.
type pausedPolicyPush struct {
	entered    chan struct{}
	release    chan struct{}
	earlyFlush chan struct{}
	enterOnce  sync.Once
	flushOnce  sync.Once
	completed  atomic.Bool
}

func (h *pausedPolicyPush) DialHook(next goredis.DialHook) goredis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) { return next(ctx, network, addr) }
}

func (h *pausedPolicyPush) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}

func (h *pausedPolicyPush) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		if cmd.Name() == "rpush" {
			h.enterOnce.Do(func() { close(h.entered) })
			select {
			case <-h.release:
			case <-ctx.Done():
				return ctx.Err()
			}
			err := next(ctx, cmd)
			h.completed.Store(true)
			return err
		}
		if (cmd.Name() == "lmove" || cmd.Name() == "lrange") && !h.completed.Load() {
			h.flushOnce.Do(func() { close(h.earlyFlush) })
		}
		return next(ctx, cmd)
	}
}

func TestPolicyEvalShutdownFinalFlushWaitsForPump(t *testing.T) {
	store := &fakeStore{}
	w, redis, _ := newTestWriter(t, PolicyEvalConfig{FlushInterval: time.Hour}, store)
	hook := &pausedPolicyPush{entered: make(chan struct{}), release: make(chan struct{}), earlyFlush: make(chan struct{})}
	redis.AddHook(hook)
	ctx, cancel := context.WithCancel(context.Background())
	var releaseOnce sync.Once
	defer func() { cancel(); releaseOnce.Do(func() { close(hook.release) }); _ = w.Stop(5 * time.Second) }()
	w.Start(ctx)
	w.Write(PolicyEvalEntry{ActorID: 7, ResourceType: "shutdown", ResourceID: "late-row", Action: "read", Result: true})
	select {
	case <-hook.entered:
	case <-time.After(time.Second):
		t.Fatal("pump did not reach the controlled push barrier")
	}
	cancel()
	select {
	case <-hook.earlyFlush:
		t.Fatal("final flush overtook an unfinished pump push")
	case <-time.After(100 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(hook.release) })
	require.True(t, w.Stop(5*time.Second))
	require.Equal(t, 1, store.total(), "a healthy final flush must include the last admitted producer row")
	for _, key := range []string{w.cfg.RedisKey, w.cfg.ProcessingKey} {
		remaining, err := redis.LLen(context.Background(), key).Result()
		require.NoError(t, err)
		require.Zero(t, remaining)
	}
}

func TestPolicyEvalShutdownFailedFinalFlushRetainsDurableRows(t *testing.T) {
	store := &fakeStore{fail: 1}
	w, redis, _ := newTestWriter(t, PolicyEvalConfig{FlushInterval: time.Hour}, store)
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	w.Write(PolicyEvalEntry{ActorID: 7, ResourceType: "shutdown", ResourceID: "retry-row", Action: "read", Result: true})
	cancel()
	require.True(t, w.Stop(5*time.Second), "goroutines can stop even when final database flush fails")
	require.Zero(t, store.total())
	remaining, err := redis.LLen(context.Background(), w.cfg.ProcessingKey).Result()
	require.NoError(t, err)
	require.EqualValues(t, 1, remaining, "the failed row must remain in durable Redis processing")
	recoveredStore := &fakeStore{}
	recovered := NewPolicyEvalWriter(PolicyEvalConfig{FlushInterval: 10 * time.Millisecond}, redis, recoveredStore, nil)
	recoveryCtx, recoveryCancel := context.WithCancel(context.Background())
	recovered.Start(recoveryCtx)
	defer func() { recoveryCancel(); _ = recovered.Stop(5 * time.Second) }()
	require.Eventually(t, func() bool {
		remaining, err := redis.LLen(context.Background(), w.cfg.ProcessingKey).Result()
		return err == nil && remaining == 0 && recoveredStore.total() == 1
	}, time.Second, 10*time.Millisecond)
}
