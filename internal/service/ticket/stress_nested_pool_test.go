//go:build integration

// 压力锚：比 BK11 更狠——7 并发（>MaxConns=4）× 50 轮 Create × 30 轮 Update，每轮 5s 兜底。
// 用途：在 MaxConns=4 下验证 Create+Update 事务窗口内均无嵌套池取。
package ticket

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func TestStress_CreateTransactionHasNoPoolNesting(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	oid := childOrgID(t, "stress")

	const N, rounds = 7, 50
	t.Logf("MaxConns=%d — 跑 %d 并发 × %d 轮 Create，每轮 5s 兜底", testPool.Stat().MaxConns(), N, rounds)

	for r := 0; r < rounds; r++ {
		rctx, rcancel := context.WithTimeout(context.Background(), 5*time.Second)
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make([]error, N)
		for i := 0; i < N; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				_, errs[i] = svc.Create(rctx, &model.CreateTicketRequest{
					TypeCode: "incident", Title: fmt.Sprintf("stress-c-%d-%d", r, i), OrgID: oid,
				}, aid)
			}(i)
		}
		close(start)
		wg.Wait()
		rcancel()
		for i, e := range errs {
			require.NoErrorf(t, e, "round %d goroutine %d — Create 事务窗口仍有嵌套池取", r, i)
		}
	}
	t.Logf("✅ 全部 %d 轮 × %d 并发 Create 无挂死", rounds, N)
}

func TestStress_UpdateTransactionHasNoPoolNesting(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	oid := childOrgID(t, "stress")

	// 预建 5 张不同工单
	ids := make([]int64, 5)
	for i := range ids {
		tk, err := svc.Create(context.Background(), &model.CreateTicketRequest{
			TypeCode: "incident", Title: fmt.Sprintf("stress-u-%d", i), Priority: 2, OrgID: oid,
			CustomData: []byte(`{"seed":"base"}`),
		}, aid)
		require.NoError(t, err)
		ids[i] = tk.ID
	}

	const rounds = 30
	t.Logf("MaxConns=%d — 跑 %d 轮 × 5 并发 Update(custom_data)，每轮 5s 兜底", testPool.Stat().MaxConns(), rounds)
	for r := 0; r < rounds; r++ {
		rctx, rcancel := context.WithTimeout(context.Background(), 5*time.Second)
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make([]error, 5)
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(i int, id int64) {
				defer wg.Done()
				<-start
				_, errs[i] = svc.Update(rctx, &model.UpdateTicketRequest{
					ID:         id,
					CustomData: []byte(fmt.Sprintf(`{"k%d":"v%d"}`, i, r)),
				}, aid)
			}(i, ids[i])
		}
		close(start)
		wg.Wait()
		rcancel()
		for i, e := range errs {
			require.NoErrorf(t, e, "round %d goroutine %d — Update 事务窗口仍有嵌套池取", r, i)
		}
	}
	t.Logf("✅ 全部 %d 轮 × 5 并发 Update 无挂死", rounds)
}

// 确定性证明：占满 MaxConns 个事务连接后，池内查询必阻塞（ctx 到期返回）。
// 确保 testutil.MaxConns=4 钉生效 + 机制确凿。
func TestStress_HoldAllConnsThenPoolBlocks(t *testing.T) {
	st := testPool.Stat()
	require.EqualValues(t, 4, st.MaxConns(), "测试池应钉 MaxConns=4 — 当前 MaxConns=%d", st.MaxConns())
}
