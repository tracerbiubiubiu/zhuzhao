//go:build integration

// S9 B9-1 情报缓存代理集成测试（09 号附录 A.5 验收承重）：
// 首查回源入库 / 二查命中零上游 / 归一化同行 / 404 入缓存 / 429 降级矩阵 /
// singleflight 并发 / 懒过期 stale / ACL 透传。
package threatintel_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/threatintel"
	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

// fakeProvider 计数可控桩。
type fakeProvider struct {
	mu     sync.Mutex
	calls  int64
	result json.RawMessage
	err    error // 非 nil 时返回该错误
}

func (f *fakeProvider) Query(_ context.Context, q threatintel.Query) (json.RawMessage, error) {
	atomic.AddInt64(&f.calls, 1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func (f *fakeProvider) count() int64 { return atomic.LoadInt64(&f.calls) }

func TestB91_IntelCache(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testutil.SetupPostgresShared()
	require.NoError(t, err)
	t.Cleanup(cleanup)

	newSvcP := func(p threatintel.Provider) *threatintel.Service {
		return threatintel.NewService(pool, p, map[string]time.Duration{
			"ip": time.Hour, "domain": time.Hour, "hash": 2 * time.Hour, "url": time.Hour,
		}, nil)
	}

	unique := fmt.Sprintf("b91_%d", time.Now().UnixNano())

	t.Run("首查回源入库 → 二查命中零上游调用（hit_count 递增）", func(t *testing.T) {
		p := &fakeProvider{result: json.RawMessage(`{"risk":"high","source":"fake"}`)}
		svc := newSvcP(p)
		v := unique + ".a.com"

		r1, err := svc.Query(ctx, "domain", v)
		require.NoError(t, err)
		assert.EqualValues(t, 1, p.count())
		assert.False(t, r1.Stale)

		r2, err := svc.Query(ctx, "domain", v)
		require.NoError(t, err)
		assert.EqualValues(t, 1, p.count(), "二查命中——零上游调用")
		assert.False(t, r2.Stale)
		assert.GreaterOrEqual(t, r2.HitCount, int(2))
		// G1-2 审计锚：命中路径经 toResponse——type/value 透传（修复前恒空串）
		assert.Equal(t, "domain", r2.Type, "命中路径 type 透传")
		assert.Equal(t, v, r2.Value, "命中路径 value 透传")
		assert.JSONEq(t, `{"risk":"high","source":"fake"}`, string(r2.Result), "命中路径 result 保真")
	})

	t.Run("归一化：EXAMPLE.COM 与 example.com 同缓存行", func(t *testing.T) {
		p := &fakeProvider{result: json.RawMessage(`{}`)}
		svc := newSvcP(p)
		_, err := svc.Query(ctx, "domain", unique+".EXAMPLE.com")
		require.NoError(t, err)
		_, err = svc.Query(ctx, "domain", unique+".example.com")
		require.NoError(t, err)
		assert.EqualValues(t, 1, p.count(), "归一化后同键——仅一次回源")
	})

	t.Run("404 查无 = 正常入缓存（深审项 D）→ 二查零上游", func(t *testing.T) {
		p := &fakeProvider{err: threatintel.ErrIntelNotFound}
		svc := newSvcP(p)
		v := unique + "-nf"
		r1, err := svc.Query(ctx, "ip", v)
		require.NoError(t, err, "404=正常结果非错误")
		assert.False(t, r1.Stale)
		_, err = svc.Query(ctx, "ip", v)
		require.NoError(t, err)
		assert.EqualValues(t, 1, p.count(), "clean 答案缓存 TTL 内免打上游")
	})

	t.Run("429 降级矩阵：有缓存 stale / 无缓存 92601", func(t *testing.T) {
		// 有缓存（先成功入缓存再切换 429）
		p := &fakeProvider{result: json.RawMessage(`{"risk":"low"}`)}
		svc := newSvcP(p)
		v := unique + "-quota"
		_, err := svc.Query(ctx, "ip", v)
		require.NoError(t, err)
		// 手动把缓存行拨过期（降级臂验证）
		_, err = pool.Exec(ctx, `UPDATE intel_cache SET expires_at = NOW() - interval '1 minute' WHERE query_value = $1`, v)
		require.NoError(t, err)
		p.mu.Lock()
		p.err = threatintel.ErrIntelUpstreamQuota
		p.mu.Unlock()
		r, err := svc.Query(ctx, "ip", v)
		require.NoError(t, err)
		assert.True(t, r.Stale, "配额耗尽 + 有过期缓存 → stale 返回")

		// 无缓存 + 429 → 92601
		_, err = svc.Query(ctx, "ip", unique+"-noquota")
		assert.ErrorIs(t, err, threatintel.ErrIntelUpstreamQuota)
	})

	t.Run("懒过期：stale 返回 + 异步回源刷新（后台自愈）", func(t *testing.T) {
		p := &fakeProvider{result: json.RawMessage(`{"v":1}`)}
		svc := newSvcP(p)
		v := unique + "-lazy"
		_, err := svc.Query(ctx, "domain", v)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `UPDATE intel_cache SET expires_at = NOW() - interval '1 minute' WHERE query_value = $1`, v)
		require.NoError(t, err)

		r, err := svc.Query(ctx, "domain", v)
		require.NoError(t, err)
		assert.True(t, r.Stale, "已过期缓存 → stale 立即返回（不阻塞）")
		// 后台刷新 goroutine——等待自愈
		assert.Eventually(t, func() bool { return p.count() >= 2 }, 3*time.Second, 100*time.Millisecond,
			"懒过期异步回源刷新执行")
	})

	t.Run("singleflight：并发 10 同键 → 上游仅 1 次", func(t *testing.T) {
		var calls int64
		slow := &slowProvider{calls: &calls}
		svc := newSvcP(slow)
		v := unique + "-sf"
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = svc.Query(ctx, "domain", v)
			}()
		}
		wg.Wait()
		assert.LessOrEqual(t, atomic.LoadInt64(&calls), int64(2), "同键并发回源收敛（≤1 次+后台刷新竞态余量）")
	})

	t.Run("ACL：上游响应未知字段透传不解析", func(t *testing.T) {
		p := &fakeProvider{result: json.RawMessage(`{"unknown_field":{"nested":true},"risk":"mid"}`)}
		svc := newSvcP(p)
		r, err := svc.Query(ctx, "url", unique+".acl")
		require.NoError(t, err)
		assert.JSONEq(t, `{"unknown_field":{"nested":true},"risk":"mid"}`, string(r.Result),
			"result 原样透传（防腐层）")
	})

	t.Run("参数校验：未知类型/空值 400", func(t *testing.T) {
		svc := newSvcP(&fakeProvider{result: json.RawMessage(`{}`)})
		_, err := svc.Query(ctx, "cert", "x")
		assert.Error(t, err)
		_, err = svc.Query(ctx, "ip", "  ")
		assert.Error(t, err)
	})
}

// slowProvider 慢回源桩（放大并发窗口让 singleflight 可观测）。
type slowProvider struct{ calls *int64 }

func (s *slowProvider) Query(_ context.Context, q threatintel.Query) (json.RawMessage, error) {
	atomic.AddInt64(s.calls, 1)
	time.Sleep(150 * time.Millisecond)
	return json.RawMessage(`{"slow":true}`), nil
}
