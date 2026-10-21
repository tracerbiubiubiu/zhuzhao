//go:build integration

// F-B1 并发回归测试：custom_data 增量 patch 的读-改-写竞态。
// 修复前：两个并发 Update 各读同一基线 → 各自 merge → 后写覆盖前写 → 丢键。
// 修复后：行锁内 merge（FOR UPDATE 先于读）→ 串行化 → 键全在。
package ticket

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// TestUpdate_CustomData_ConcurrentKeys 20 个 goroutine 并发 Update 同一工单，
// 各写一个不同键——最终 20 个键必须全在（丢失 = 竞态窗口命中）。
func TestUpdate_CustomData_ConcurrentKeys(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	ctx := context.Background()
	oid := childOrgID(t, "cdp")

	tk, err := svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: "incident", Title: "race", Priority: 2, OrgID: oid,
		CustomData: []byte(`{"seed":"base"}`),
	}, aid)
	require.NoError(t, err)

	const N = 3 // 最小并发数（3 足以暴露丢键；更高在集成池排队超时）
	var wg sync.WaitGroup
	start := make(chan struct{}) // 栅栏：等全部就绪后同时发射——最大化竞态窗口重叠
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = svc.Update(ctx, &model.UpdateTicketRequest{
				ID:         tk.ID,
				CustomData: []byte(fmt.Sprintf(`{"k%02d":"v%02d"}`, i, i)),
			}, aid)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "goroutine %d 不应失败（失败=锁/校验问题而非丢键）", i)
	}

	// 读回：20 个键 + seed 必须全在
	fresh, err := svc.Get(ctx, tk.ID, aid)
	require.NoError(t, err)
	for i := 0; i < N; i++ {
		key := fmt.Sprintf("k%02d", i)
		assert.Contains(t, string(fresh.CustomData), key,
			"并发写键 %s 丢失——读-改-写竞态（F-B1）", key)
	}
	assert.Contains(t, string(fresh.CustomData), "seed", "原始键不应被并发覆盖丢失")
}
