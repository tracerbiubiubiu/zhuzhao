package audit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

type fakeStore struct {
	mu      sync.Mutex
	batches [][]PolicyEvalEntry
	fail    int // 前 N 次调用失败（重试语义验证）
	calls   int
}

func (f *fakeStore) InsertPolicyEvals(_ context.Context, rows []PolicyEvalEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls <= f.fail {
		return errors.New("simulated insert failure")
	}
	cp := make([]PolicyEvalEntry, len(rows))
	copy(cp, rows)
	f.batches = append(f.batches, cp)
	return nil
}

func (f *fakeStore) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, b := range f.batches {
		n += len(b)
	}
	return n
}

func newTestWriter(t *testing.T, cfg PolicyEvalConfig, store PolicyEvalStore) (*PolicyEvalWriter, *goredis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return NewPolicyEvalWriter(cfg, rdb, store, nil), rdb, mr
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

func TestPolicyEvalWriteFlushToStore(t *testing.T) {
	store := &fakeStore{}
	w, _, _ := newTestWriter(t, PolicyEvalConfig{BatchSize: 10, FlushInterval: 30 * time.Millisecond}, store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	w.Write(PolicyEvalEntry{ActorID: 7, ActorRoles: []string{"user", "admin"}, ResourceType: "ticket",
		ResourceID: "123", Action: "update", Result: true, TraceID: "req-abc"})
	w.Write(PolicyEvalEntry{ActorID: 8, ResourceType: "ticket", ResourceID: "456", Action: "read",
		Result: false, Reason: "denied"})

	waitUntil(t, 2*time.Second, func() bool { return store.total() == 2 })

	store.mu.Lock()
	defer store.mu.Unlock()
	got := store.batches[0]
	if len(got) != 2 {
		t.Fatalf("batch len = %d", len(got))
	}
	if got[0].ActorID != 7 || got[0].Result != true || got[0].TraceID != "req-abc" {
		t.Fatalf("row0 mutated: %+v", got[0])
	}
	if got[0].ActorRoles[0] != "user" || got[0].ActorRoles[1] != "admin" {
		t.Fatalf("roles lost: %v", got[0].ActorRoles)
	}
	if got[1].Result != false || got[1].Reason != "denied" {
		t.Fatalf("row1 mutated: %+v", got[1])
	}
	if got[0].CreatedAt == "" {
		t.Fatal("CreatedAt should be stamped")
	}
}

func TestPolicyEvalInsertFailureRetries(t *testing.T) {
	store := &fakeStore{fail: 1} // 首次落库失败 → 行留 processing，下轮重试
	w, _, _ := newTestWriter(t, PolicyEvalConfig{BatchSize: 10, FlushInterval: 30 * time.Millisecond}, store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	w.Write(PolicyEvalEntry{ActorID: 1, ResourceType: "ticket", Action: "read", Result: true})
	waitUntil(t, 2*time.Second, func() bool { return store.total() == 1 })
}

func TestPolicyEvalBufferFullDropsFailOpen(t *testing.T) {
	store := &fakeStore{}
	// 不 Start（pump 不消费）→ channel=1 立即塞满
	w, _, _ := newTestWriter(t, PolicyEvalConfig{BufferSize: 1}, store)
	w.Write(PolicyEvalEntry{ActorID: 1, ResourceType: "t", Action: "read", Result: true})
	w.Write(PolicyEvalEntry{ActorID: 2, ResourceType: "t", Action: "read", Result: true})
	if w.Dropped() != 1 {
		t.Fatalf("want 1 dropped, got %d", w.Dropped())
	}
}

func TestPolicyEvalPoisonEntrySkipped(t *testing.T) {
	store := &fakeStore{}
	w, rdb, _ := newTestWriter(t, PolicyEvalConfig{BatchSize: 10, FlushInterval: 30 * time.Millisecond}, store)
	cfg := PolicyEvalConfig{BatchSize: 10, RedisKey: "audit:policy_eval", ProcessingKey: "audit:policy_eval:processing"}.withDefaults()
	// 毒丸行直接注入 List（跳过正常序列化路径）
	if err := rdb.RPush(context.Background(), cfg.RedisKey, "{not-json").Err(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	w.Write(PolicyEvalEntry{ActorID: 9, ResourceType: "t", Action: "read", Result: true})
	waitUntil(t, 2*time.Second, func() bool { return store.total() == 1 })
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.batches[0][0].ActorID != 9 {
		t.Fatalf("poison row should be skipped, healthy row kept: %+v", store.batches[0])
	}
}

func TestPolicyEvalShutdownDrain(t *testing.T) {
	store := &fakeStore{}
	w, rdb, _ := newTestWriter(t, PolicyEvalConfig{BatchSize: 10, FlushInterval: time.Hour}, store)
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	// flush 周期设 1h：条目只会进 List 不会被消费，验证 cancel 后 pump 的 drain 语义
	w.Write(PolicyEvalEntry{ActorID: 1, ResourceType: "t", Action: "read", Result: true})
	w.Write(PolicyEvalEntry{ActorID: 2, ResourceType: "t", Action: "read", Result: true})
	cancel()
	// 零丢失不变量：条目分布在 store（flusher 收尾已落库）/ List（drain 后未及消费）/
	// processing（LMOVE 中途）三者之和恒为 2——重启后 List/processing 续消（AL4）。
	waitUntil(t, 4*time.Second, func() bool {
		ctxBg := context.Background()
		inList, _ := rdb.LLen(ctxBg, "audit:policy_eval").Result()
		inProc, _ := rdb.LLen(ctxBg, "audit:policy_eval:processing").Result()
		return int(inList)+int(inProc)+store.total() == 2
	})
}
