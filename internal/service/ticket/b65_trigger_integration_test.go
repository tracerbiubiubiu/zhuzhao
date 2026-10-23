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

	// 偏好过滤回归锚（外审 F-S6-1-4——optRepo 在 hook 已接线但零用例；
	// 06 号 §10 验收标尺"偏好过滤"落锚）：收件人设 opt-out 后静默+零 log 行。
	t.Run("收件人 opt-out ticket.closed→静默不发送不落 log", func(t *testing.T) {
		sender.mu.Lock()
		sender.msgs = nil
		sender.mu.Unlock()

		// bid 对 ticket.closed 设静默（000056 UNIQUE 幂等）
		_, err := testPool.Exec(ctx, `
			INSERT INTO notification_optouts (user_id, biz_type, event_action)
			VALUES ($1, 'ticket', 'ticket.closed') ON CONFLICT DO NOTHING`, bid)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = testPool.Exec(ctx,
				`DELETE FROM notification_optouts WHERE user_id=$1 AND biz_type='ticket' AND event_action='ticket.closed'`, bid)
		})

		code3 := "b65o_" + uniqueSuffix()
		_, err = svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{
			Code: code3, Name: "偏好过滤批",
			Transitions: json.RawMessage(`{"open":["assigned"],"assigned":["closed"]}`),
		})
		require.NoError(t, err)
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code3, Title: "opt-out 静默单", Priority: 2, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid))
		time.Sleep(200 * time.Millisecond)

		require.NoError(t, svc.Close(ctx, &model.CloseTicketRequest{ID: tk.ID}, aid))
		time.Sleep(200 * time.Millisecond)

		// 断言只看 closed 模板——同单 Assign 触发的 assigned 通知（bid 合法收件）
		// 也会落捕获，opt-out 只对 ticket.closed 生效
		for _, m := range sender.messages() {
			if m.TemplateCode != "ticket_closed_notify" {
				continue
			}
			assert.NotContains(t, m.Recipients, strconv.FormatInt(bid, 10),
				"opt-out 收件人须被过滤不发送")
		}
		// 全过滤→hook 提前返回→零 log 行（"跳过即不落发送行"语义）
		var n int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT count(*) FROM message_log WHERE biz_id=$1 AND template_code='ticket_closed_notify'`,
			strconv.FormatInt(tk.ID, 10)).Scan(&n))
		assert.Equal(t, 0, n, "opt-out 过滤后该单不落发送 log 行")
	})
}
