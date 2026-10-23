//go:build integration

// S6 B6-5 触发挂接集成测试：工单事件→模板→渲染→UIP 桩发送全链。
package ticket

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/channelsender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// captureSender 测试用 ChannelSender 桩——捕获发送消息。
type captureSender struct {
	mu   sync.Mutex
	msgs []channelsender.Message
}

func (c *captureSender) Channel() string { return "test" }
func (c *captureSender) Send(_ context.Context, msg channelsender.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, msg)
	return nil
}
func (c *captureSender) messages() []channelsender.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]channelsender.Message{}, c.msgs...)
}

func TestB65_TriggerHook(t *testing.T) {
	ctx := context.Background()

	// 组装：事件注册表 + 模板 repo + 捕获 sender + 通知钩子
	reg := domainevent.NewHookRegistry(nil)
	tplRepo := repository.NewMessageTemplateRepo(testPool)
	sender := &captureSender{}
	hook := NewTicketNotifyHook(tplRepo, repository.NewMessageRuleRepo(testPool), repository.NewMessageLogRepo(testPool), repository.NewNotificationOptoutRepo(testPool), sender, nil, nil) // notifySvc=nil（测试不验死信面）
	reg.Subscribe(domainevent.BizTicket, hook)

	svc, aid, bid, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	svc.SetEventRegistry(reg)

	oid := childOrgID(t, "b65t")
	code := "b65t_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "触发批"})
	require.NoError(t, err)

	t.Run("Assign→ticket.assigned→模板渲染→发送", func(t *testing.T) {
		sender.mu.Lock()
		sender.msgs = nil
		sender.mu.Unlock()

		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "触发链测试单", Priority: 2, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		// created 事件→created 通知=不送（首版跳过自通知）
		time.Sleep(50 * time.Millisecond)

		err = svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid)
		require.NoError(t, err)
		time.Sleep(200 * time.Millisecond) // 异步派发

		msgs := sender.messages()
		require.GreaterOrEqual(t, len(msgs), 1, "分派事件须触发至少一条通知发送")

		last := msgs[len(msgs)-1]
		assert.Equal(t, "ticket", last.BizType)
		assert.Equal(t, "ticket_assigned_notify", last.TemplateCode)
		assert.Contains(t, last.Body, "触发链测试单", "正文含工单标题（占位符已渲染）")
		assert.NotContains(t, last.Subject, "{{", "占位符不应残留在渲染结果中")
		assert.NotEmpty(t, last.Body, "正文非空")
		assert.NotEmpty(t, last.Recipients, "收件人非空（分派对象）")
	})

	t.Run("Create→ticket.created→首版不送自通知", func(t *testing.T) {
		sender.mu.Lock()
		sender.msgs = nil
		sender.mu.Unlock()

		_, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "自通知单", Priority: 2, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		time.Sleep(200 * time.Millisecond)

		msgs := sender.messages()
		// 首版 created→不送（resolveRecipients 返回 nil）
		// 但 assigned 不在此子测中触发——msgs 应为空或仅含其他动作
		for _, m := range msgs {
			assert.NotEqual(t, "ticket_created_notify", m.TemplateCode,
				"首版创建通知不送自通知（06 号 §5.1 第一档）")
		}
	})
}
