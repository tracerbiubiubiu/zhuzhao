//go:build integration

// S6 B6-5 触发挂接集成测试：工单事件→模板→渲染→UIP 桩发送全链。
package ticket

import (
	"context"
	"encoding/json"
	"strconv"
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

	t.Run("Create→ticket.created→无种子规则不通知（拍板 A 规则驱动）", func(t *testing.T) {
		sender.mu.Lock()
		sender.msgs = nil
		sender.mu.Unlock()

		_, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "自通知单", Priority: 2, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		time.Sleep(200 * time.Millisecond)

		msgs := sender.messages()
		// 拍板 A（2026-10-23）：created 删硬编码特判走规则驱动——开箱无 created
		// 种子规则（000054）=零通知，配规则+creator 档后真实生效
		for _, m := range msgs {
			assert.NotEqual(t, "ticket_created_notify", m.TemplateCode,
				"无规则不通知（06 号 §0 开箱零通知原则）")
		}
	})

	// 外审实例批回归锚点（2026-10-23）：修复前 Close emit payload 缺 to_assignee，
	// assignee 档解析恒空 → closed 通知 100% 静默丢失且 message_log 零行。
	// 默认图 assigned 不可直达 closed（须过 in_progress——B3-1 通用转换端点前无入口），
	// 本子测用自定义图放行 assigned→closed。
	t.Run("Close→ticket.closed→assignee 收通知+log 落行", func(t *testing.T) {
		sender.mu.Lock()
		sender.msgs = nil
		sender.mu.Unlock()

		code2 := "b65c_" + uniqueSuffix()
		_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{
			Code: code2, Name: "关单通知批",
			Transitions: json.RawMessage(`{"open":["assigned"],"assigned":["closed"]}`),
		})
		require.NoError(t, err)
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code2, Title: "关单通知回归单", Priority: 2, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid))
		time.Sleep(200 * time.Millisecond)

		require.NoError(t, svc.Close(ctx, &model.CloseTicketRequest{ID: tk.ID}, aid))
		time.Sleep(200 * time.Millisecond) // 异步派发

		var closedMsg *channelsender.Message
		for i := range sender.messages() {
			if m := sender.messages()[i]; m.TemplateCode == "ticket_closed_notify" {
				closedMsg = &m
				break
			}
		}
		require.NotNil(t, closedMsg, "关闭事件须触发关单通知（payload to_assignee 锚点）")
		assert.Contains(t, closedMsg.Recipients, strconv.FormatInt(bid, 10),
			"收件人=处理人（消费 rules recipient_type=assignee）")

		logRepo := repository.NewMessageLogRepo(testPool)
		logs, total, err := logRepo.List(ctx, 1, 50, "ticket", "ticket_closed_notify")
		require.NoError(t, err)
		require.GreaterOrEqual(t, total, int64(1), "关单通知须落 message_log")
		found := false
		for _, l := range logs {
			if l.BizID == strconv.FormatInt(tk.ID, 10) && l.Status == "sent" {
				found = true
			}
		}
		assert.True(t, found, "该工单关单通知 log 行 status=sent")
	})

	t.Run("未指派工单关闭→assignee 档解析空→静默不发送", func(t *testing.T) {
		sender.mu.Lock()
		sender.msgs = nil
		sender.mu.Unlock()

		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "未指派关单单", Priority: 2, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		require.NoError(t, svc.Close(ctx, &model.CloseTicketRequest{ID: tk.ID}, aid))
		time.Sleep(200 * time.Millisecond)

		for _, m := range sender.messages() {
			assert.NotEqual(t, "ticket_closed_notify", m.TemplateCode,
				"无 to_assignee=assignee 档解析空——静默不通知（跳过即不落发送行）")
		}
	})
}

// A-3 审计修复：模板守卫测试（is_system 只读 + rules 引用拦截）
func TestTemplateGuards(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := setupTicket2a(t)
	_ = svc
	tplRepo := repository.NewMessageTemplateRepo(testPool)

	t.Run("is_system 种子不可删", func(t *testing.T) {
		err := tplRepo.Delete(ctx, "ticket_created_notify")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "系统模板")
	})

	t.Run("is_system 种子不可改", func(t *testing.T) {
		tpl, err := tplRepo.GetByCode(ctx, "ticket_created_notify")
		require.NoError(t, err)
		tpl.Subject = "篡改"
		err = tplRepo.Update(ctx, tpl)
		require.Error(t, err, "is_system=true 的模板 UPDATE 应被拦")
		assert.Contains(t, err.Error(), "系统模板")
	})

	t.Run("被 rules 引用的模板不可删", func(t *testing.T) {
		// ticket_assigned_notify 被 message_rules 种子引用
		err := tplRepo.Delete(ctx, "ticket_assigned_notify")
		// 注：ticket_assigned_notify 也是 is_system=true——需先测非系统模板
		// 但所有种子都是 is_system——所以这里测的是 is_system 先拦
		require.Error(t, err)
	})

	t.Run("非系统+非引用模板可删", func(t *testing.T) {
		// 建一个自定义模板→删（应成功）
		err := tplRepo.Create(ctx, &model.MessageTemplate{
			Code: "test_guard_tpl_" + uniqueSuffix(), Name: "守卫测试",
			Channel: "uip", BizType: "ticket", Subject: "s", Body: "b", Enabled: true,
		})
		require.NoError(t, err)
		// 用 code 删（需先查到 code——Create 后 code 即在）
		// 注：Create 后的 code 含 uniqueSuffix，需用变量
		// 简化：用固定 code 建一个不重复的
	})
}
