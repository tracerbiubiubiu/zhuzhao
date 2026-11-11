//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// TestBK11_UpdateHammer —— Update 事务窗口嵌套取池死锁锚（BK11 同类残留，2026-10-27 外审核出）。
// 修前：validateCustomData/applyFormulas 在 Begin 后走池读 ListTicketTypeFields——
// N≥MaxConns 个并发 Update（带 custom_data）各持 1 连接、各请求第 2 个，池满无人可释放：
// 自闭环死锁（比 Create 原病灶更直接——不需要第三方持连）。修复=两 helper 接 RowQuerier
// （Update 传 tx、Create 传池）；本锚在 testutil MaxConns=4 下以 6 并发×10 轮压 Update，
// 每轮 5s 兜底 ctx——修前 round 0 即池耗尽（ctx deadline），修后毫秒级完成。
func TestBK11_UpdateHammer(t *testing.T) {
	ctx := context.Background()
	svc, aid, _, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}

	// 无字段临时类型：validate/apply 对空 schema 直接透传——Update 全链只剩嵌套取数这一变量
	code := "hmr_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "锤击无字段"})
	require.NoError(t, err)
	oid := childOrgID(t, "hmr")

	const N, rounds = 6, 10
	ids := make([]int64, N)
	for i := range ids {
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: fmt.Sprintf("hammer%d_%s", i, code), OrgID: oid,
		}, aid)
		require.NoError(t, err)
		ids[i] = tk.ID
	}

	var wg sync.WaitGroup
	for r := 0; r < rounds; r++ {
		rctx, rcancel := context.WithTimeout(ctx, 5*time.Second)
		start := make(chan struct{})
		errs := make([]error, N)
		for i := range ids {
			wg.Add(1)
			go func(i int, id int64) {
				defer wg.Done()
				<-start
				_, errs[i] = svc.Update(rctx, &model.UpdateTicketRequest{
					ID:         id,
					CustomData: json.RawMessage(fmt.Sprintf(`{"hmr_round":%d}`, r)),
				}, aid)
			}(i, ids[i])
		}
		close(start)
		wg.Wait()
		rcancel()
		for i, e := range errs {
			require.NoErrorf(t, e, "round %d goroutine %d——修前形态=池耗尽死锁至 5s 兜底", r, i)
		}
	}
}
