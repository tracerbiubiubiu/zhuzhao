//go:build integration

// F-B1 并发回归测试：custom_data 增量 patch 的读-改-写竞态。
// 修复前：两个并发 Update 各读同一基线 → 各自 merge → 后写覆盖前写 → 丢键。
// 修复后：行锁内 merge（FOR UPDATE 先于读）→ 串行化 → 键全在。
//
// 守卫强度说明（2026-10-21 变异验证实测）：单轮 3 并发的竞态命中是概率性的——
// 撤掉 FOR UPDATE 后单轮约 25% 假绿（8 次漏 2 次）。故改为串行 rounds 轮、
// 每轮新建工单 + 3 并发栅栏同发，漏守率随轮数指数下降（≈0.25^rounds）。
// 3 并发是集成连接池不超时的上限，靠轮数而非并发数堆命中概率。
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

// TestUpdate_CustomData_ConcurrentKeys 串行 rounds 轮，每轮 3 个 goroutine 并发
// Update 同一工单各写一个不同键——每轮结束 3 个键 + seed 必须全在
// （任一轮丢键 = 读-改-写竞态命中，F-B1 回归）。
func TestUpdate_CustomData_ConcurrentKeys(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	ctx := context.Background()
	oid := childOrgID(t, "cdp")

	const (
		N      = 3 // 单轮并发数——集成连接池不超时的上限
		rounds = 6 // 串行轮数——堆竞态命中概率（漏守率≈单轮漏守^rounds）
	)

	for round := 0; round < rounds; round++ {
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: "incident", Title: fmt.Sprintf("race-r%d", round), Priority: 2, OrgID: oid,
			CustomData: []byte(`{"seed":"base"}`),
		}, aid)
		require.NoError(t, err, "round %d 建单失败", round)

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
			require.NoError(t, err, "round %d goroutine %d 不应失败（失败=锁/校验问题而非丢键）", round, i)
		}

		// 读回：本轮 3 个键 + seed 必须全在
		fresh, err := svc.Get(ctx, tk.ID, aid)
		require.NoError(t, err)
		for i := 0; i < N; i++ {
			key := fmt.Sprintf("k%02d", i)
			assert.Contains(t, string(fresh.CustomData), key,
				"round %d 并发写键 %s 丢失——读-改-写竞态（F-B1）", round, key)
		}
		assert.Contains(t, string(fresh.CustomData), "seed", "round %d 原始键不应被并发覆盖丢失", round)
	}
}
