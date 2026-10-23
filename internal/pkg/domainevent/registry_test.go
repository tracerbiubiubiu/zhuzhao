// DomainEvent 事件面基建单测（B6-1——06 号 A.1 契约；测试先行先红后绿）。
// 覆盖：按 biz_type 路由/多订阅者各自收到/recover 隔离不连坐/通配订阅者/
// at-most-once（Dispatch 即发即忘）/ nil hook 安全。
package domainevent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHookRegistry_BizTypeRouting(t *testing.T) {
	r := NewHookRegistry(nil)
	var ticketGot, riskGot atomic.Int32

	r.Subscribe(BizTicket, "routing.ticket", HookFunc(func(_ context.Context, ev DomainEvent) {
		if ev.BizType == BizTicket {
			ticketGot.Add(1)
		}
	}))
	r.Subscribe("risk", "routing.risk", HookFunc(func(_ context.Context, ev DomainEvent) {
		if ev.BizType == "risk" {
			riskGot.Add(1)
		}
	}))

	// 派发 ticket 事件——只有 ticket 订阅者收到
	r.Dispatch(context.Background(), DomainEvent{
		BizType: BizTicket, Action: TicketCreated, BizID: "1", Timestamp: time.Now(),
	})
	time.Sleep(50 * time.Millisecond) // goroutine 异步——测试用短等待

	assert.Equal(t, int32(1), ticketGot.Load(), "ticket 订阅者收到")
	assert.Equal(t, int32(0), riskGot.Load(), "risk 订阅者不收（biz_type 路由）")
}

func TestHookRegistry_MultiSubscriber(t *testing.T) {
	r := NewHookRegistry(nil)
	var count atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)
		r.Subscribe(BizTicket, fmt.Sprintf("multi.%d", i), HookFunc(func(_ context.Context, _ DomainEvent) {
			defer wg.Done()
			count.Add(1)
		}))
	}

	r.Dispatch(context.Background(), DomainEvent{
		BizType: BizTicket, Action: TicketCreated, BizID: "1", Timestamp: time.Now(),
	})
	wg.Wait()
	assert.Equal(t, int32(5), count.Load(), "5 个订阅者各自收到（多订阅者面）")
}

func TestHookRegistry_RecoverIsolation(t *testing.T) {
	r := NewHookRegistry(nil)
	var healthy atomic.Int32
	done := make(chan struct{})

	// 订阅者 1 panic——不连坐
	r.Subscribe(BizTicket, "recover.panic", HookFunc(func(_ context.Context, _ DomainEvent) {
		panic("subscriber blew up")
	}))
	// 订阅者 2 正常
	r.Subscribe(BizTicket, "recover.healthy", HookFunc(func(_ context.Context, _ DomainEvent) {
		healthy.Add(1)
		close(done)
	}))

	require.NotPanics(t, func() {
		r.Dispatch(context.Background(), DomainEvent{
			BizType: BizTicket, Action: TicketClosed, BizID: "2", Timestamp: time.Now(),
		})
	})

	select {
	case <-done:
		assert.Equal(t, int32(1), healthy.Load(), "panic 订阅者不影响后续订阅者（recover 隔离）")
	case <-time.After(2 * time.Second):
		t.Fatal("正常订阅者未被调用——panic 连坐了")
	}
}

func TestHookRegistry_WildcardSubscriber(t *testing.T) {
	r := NewHookRegistry(nil)
	var ticketCnt, riskCnt, wildCnt atomic.Int32

	r.Subscribe(BizTicket, "wild.ticket", HookFunc(func(_ context.Context, ev DomainEvent) { ticketCnt.Add(1) }))
	r.Subscribe("risk", "wild.risk", HookFunc(func(_ context.Context, ev DomainEvent) { riskCnt.Add(1) }))
	r.SubscribeAll(HookFunc(func(_ context.Context, ev DomainEvent) { wildCnt.Add(1) }))

	r.Dispatch(context.Background(), DomainEvent{BizType: BizTicket, Action: TicketCreated, Timestamp: time.Now()})
	r.Dispatch(context.Background(), DomainEvent{BizType: "risk", Action: "risk.created", Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, int32(1), ticketCnt.Load())
	assert.Equal(t, int32(1), riskCnt.Load())
	assert.Equal(t, int32(2), wildCnt.Load(), "通配订阅者收到所有域事件")
}

func TestHookRegistry_NilHookSafe(t *testing.T) {
	r := NewHookRegistry(nil)
	r.Subscribe(BizTicket, "nil.hook", nil) // 不 panic 不注册（名字也不占用）
	assert.Equal(t, 0, r.SubscriberCount(BizTicket))
}

func TestHookRegistry_SubscriberCount(t *testing.T) {
	r := NewHookRegistry(nil)
	r.Subscribe(BizTicket, "count.t1", HookFunc(func(_ context.Context, _ DomainEvent) {}))
	r.Subscribe(BizTicket, "count.t2", HookFunc(func(_ context.Context, _ DomainEvent) {}))
	r.Subscribe("risk", "count.r1", HookFunc(func(_ context.Context, _ DomainEvent) {}))
	assert.Equal(t, 2, r.SubscriberCount(BizTicket))
	assert.Equal(t, 1, r.SubscriberCount("risk"))
	assert.Equal(t, []BizType{"risk", BizTicket}, r.SortedBizTypes())
}

// 重名防护（06 号 A.1「重复名 panic（装配期暴露）」——微批 A）：双注册属装配
// 错误，启动即 panic 好过运行期重复派发静默翻倍。
func TestHookRegistry_DuplicateNamePanics(t *testing.T) {
	r := NewHookRegistry(nil)
	r.Subscribe(BizTicket, "dup.name", HookFunc(func(_ context.Context, _ DomainEvent) {}))
	assert.PanicsWithValue(t,
		`domainevent: hook 注册名 "dup.name" 重复（biz=risk）——装配期双注册`,
		func() {
			r.Subscribe("risk", "dup.name", HookFunc(func(_ context.Context, _ DomainEvent) {}))
		}, "跨域重名同样 panic（名字全局唯一）")
	assert.Equal(t, 0, r.SubscriberCount("risk"), "panic 前未落注册")
}

func TestDomainEvent_String(t *testing.T) {
	ev := DomainEvent{
		BizType: BizTicket, Action: TicketCreated, BizID: "123",
		ActorID: 42, Timestamp: time.Date(2026, 10, 23, 1, 0, 0, 0, time.UTC),
	}
	assert.Contains(t, ev.String(), "ticket")
	assert.Contains(t, ev.String(), "ticket.created")
	assert.Contains(t, ev.String(), "123")
	assert.Contains(t, ev.String(), "42")
}
