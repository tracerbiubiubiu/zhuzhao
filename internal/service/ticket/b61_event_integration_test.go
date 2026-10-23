//go:build integration

// B6-1 事件面基建集成测试：Create/Assign 事务提交后领域事件派发到订阅者。
package ticket

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
)

func TestB61_DomainEventDispatch(t *testing.T) {
	ctx := context.Background()

	// 建事件注册表 + 测试订阅者
	reg := domainevent.NewHookRegistry(nil)
	var mu sync.Mutex
	var created []domainevent.DomainEvent
	var assigned []domainevent.DomainEvent
	var wildcards []domainevent.DomainEvent

	reg.Subscribe(domainevent.BizTicket, "b61.capture", domainevent.HookFunc(func(_ context.Context, ev domainevent.DomainEvent) {
		mu.Lock()
		defer mu.Unlock()
		switch ev.Action {
		case domainevent.TicketCreated:
			created = append(created, ev)
		case domainevent.TicketAssigned:
			assigned = append(assigned, ev)
		}
	}))
	reg.SubscribeAll(domainevent.HookFunc(func(_ context.Context, ev domainevent.DomainEvent) {
		mu.Lock()
		defer mu.Unlock()
		wildcards = append(wildcards, ev)
	}))

	// 建带事件注册表的 service
	svc, aid, bid, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	svc.SetEventRegistry(reg)
	oid := childOrgID(t, "b61e")

	code := "b61evt_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "事件批"})
	require.NoError(t, err)

	t.Run("Create 后 ticket.created 派发", func(t *testing.T) {
		mu.Lock()
		created = nil
		wildcards = nil
		mu.Unlock()

		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "事件单", Priority: 2, OrgID: oid,
		}, aid)
		require.NoError(t, err)

		// 异步派发——等 goroutine
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		require.Len(t, created, 1, "ticket.created 事件到达订阅者")
		ev := created[0]
		assert.Equal(t, domainevent.BizTicket, ev.BizType)
		assert.Equal(t, domainevent.TicketCreated, ev.Action)
		assert.Equal(t, strconv.FormatInt(tk.ID, 10), ev.BizID) // BizID=字符串化 id
		assert.Equal(t, aid, ev.ActorID)
		assert.Equal(t, "事件单", ev.Payload["title"])
		// 外审实例批②生产端钉测：payload 须含 actor 键——resolveRecipientsFromRule
		// 的 creator 档读 payload["actor"]（修复前 Create 只带顶层 ActorID，键恒空）
		assert.Equal(t, strconv.FormatInt(aid, 10), ev.Payload["actor"], "creator 档收件人锚点")
		assert.GreaterOrEqual(t, len(wildcards), 1, "通配订阅者也收到")
	})

	t.Run("Assign 后 ticket.assigned 派发", func(t *testing.T) {
		mu.Lock()
		assigned = nil
		mu.Unlock()

		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "分派单", Priority: 2, OrgID: oid,
		}, aid)
		require.NoError(t, err)

		err = svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		require.Len(t, assigned, 1, "ticket.assigned 事件到达订阅者")
		ev := assigned[0]
		assert.Equal(t, domainevent.TicketAssigned, ev.Action)
		assert.Equal(t, strconv.FormatInt(tk.ID, 10), ev.BizID)
	})

	t.Run("eventReg=nil 时 NOP 不 panic", func(t *testing.T) {
		svc2, _, _, _, _ := setupTicket2a(t)
		// svc2 未 SetEventRegistry → eventReg=nil
		_, err := svc2.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "NOP 单", Priority: 2, OrgID: oid,
		}, aid)
		require.NoError(t, err, "nil 注册表不 panic")
	})
}
