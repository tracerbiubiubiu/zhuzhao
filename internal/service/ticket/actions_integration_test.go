//go:build integration

// S3a B3-3 动作库集成测试：ActionRegistry 注册/链深/post_actions 消费
// （cc 动作真实链路/notify 兜底事件/reassign 转派/未注册跳过/失败不回滚转换）。
package ticket

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/channelsender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestB33_Actions(t *testing.T) {
	ctx := context.Background()
	svc, aid, bid, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	svc.SetEventRegistry(domainevent.NewHookRegistry(nil))
	oid := childOrgID(t, "b33")
	code := "b33_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "动作批"})
	require.NoError(t, err)

	// bid 入 org（cc 动作无需 org——决议③ 是分派规则的事）
	reg := NewActionRegistry()
	reg.Register(NewCCAction(svc))
	reg.Register(NewReassignAction(svc))
	reg.Register(&NotifyAction{Svc: svc})
	svc.SetActionRegistry(reg)

	t.Run("注册表：重复 panic/Names 排序/未注册 Get false", func(t *testing.T) {
		assert.Error(t, reg.Register(NewCCAction(svc)), "重复注册返回 error")
		names := reg.Names()
		assert.Equal(t, []string{"cc", "notify", "reassign"}, names)
		_, ok := reg.Get("spawn_task")
		assert.False(t, ok, "spawn_task 未装配 taskrunner 通道——本测不注册")
	})

	t.Run("cc 动作：post_actions 消费真实链路（抄送落库+可见性）", func(t *testing.T) {
		// transition_meta: assigned>in_progress post_actions=[{name:cc,params:{user_ids:[aid]}}]
		meta := `{"assigned>in_progress":{"post_actions":[{"name":"cc","params":{"user_ids":[` + itoa64(aid) + `]}}]}}`
		_, err := svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: []byte(meta)})
		require.NoError(t, err)

		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "cc 动作单", Priority: 3, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid))

		// bid（assignee）执行转换 → cc 动作把 aid 抄送进来
		_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "in_progress"}, bid)
		require.NoError(t, err)
		rows, err := svc.ListCC(ctx, tk.ID, aid)
		require.NoError(t, err)
		require.Len(t, rows, 1, "cc 动作落库——aid 被抄送")
		assert.EqualValues(t, aid, rows[0].UserID)
	})

	t.Run("reassign 动作：params user_id 转派", func(t *testing.T) {
		meta := `{"in_progress>pending_verify":{"post_actions":[{"name":"reassign","params":{"user_id":` + itoa64(aid) + `}}]}}`
		_, err := svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: []byte(meta)})
		require.NoError(t, err)

		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "reassign 单", Priority: 3, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid))
		_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "in_progress"}, bid)
		require.NoError(t, err)
		// 转到 pending_verify → reassign 到 aid
		_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "pending_verify"}, bid)
		require.NoError(t, err)
		got, err := svc.Get(ctx, tk.ID, aid)
		require.NoError(t, err)
		require.NotNil(t, got.AssignedTo)
		assert.EqualValues(t, aid, *got.AssignedTo, "reassign 动作转派到 aid")
	})

	t.Run("未注册动作跳过 + 字符串形态 + 失败不回滚转换", func(t *testing.T) {
		// meta 含未注册动作 spawn_task（未装配通道）+ 字符串形态未知动作
		meta := `{"pending_verify>closed":{"post_actions":["spawn_task","no_such_action"]}}`
		_, err := svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: []byte(meta)})
		require.NoError(t, err)

		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "跳过单", Priority: 3, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid))
		_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "in_progress"}, bid)
		require.NoError(t, err)
		_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "pending_verify"}, bid)
		require.NoError(t, err)
		// 未注册动作失败不回滚转换——closed 生效
		_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "closed"}, bid)
		require.NoError(t, err, "spawn_task（未装配通道）与未知动作均不阻断转换本身")
		got, err := svc.Get(ctx, tk.ID, aid)
		require.NoError(t, err)
		assert.Equal(t, "closed", got.Status, "动作失败/跳过不回滚转换")
	})

	t.Run("notify 动作：兜底事件派发（重复注册 panic 前置已验——此处仅存活性）", func(t *testing.T) {
		// notify 已在顶部注册（重复注册 panic 已由首子测验证）——此处断言注册表仍持有
		_, ok := reg.Get("notify")
		assert.True(t, ok, "notify 动作注册存活")

	})
}

// TestB33_NotifyActionE2E 人工入口（入口三）端到端锚（审计缺口批——原仅注册
// 存活性断言，Execute 本体零覆盖：params 解析/事件派发/无规则静默撤了不会红）。
// 链路=转换携带 notify post_action → EmitActionNotify 派发 ticket.action_notify
// → cc 档规则（消费 payload cc_users←params.recipients）→ 捕获 sender+log 落行。
func TestB33_NotifyActionE2E(t *testing.T) {
	ctx := context.Background()
	reg := domainevent.NewHookRegistry(nil)
	tplRepo := repository.NewMessageTemplateRepo(testPool)
	sender := &captureSender{}
	hook := NewTicketNotifyHook(tplRepo, repository.NewMessageRuleRepo(testPool),
		repository.NewMessageLogRepo(testPool), repository.NewNotificationOptoutRepo(testPool),
		sender, nil, nil)
	reg.Subscribe(domainevent.BizTicket, "b33.notifye2e", hook)

	svc, aid, bid, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	svc.SetEventRegistry(reg)
	oid := childOrgID(t, "b33e2")
	code := "b33e2_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "动作通知端到端"})
	require.NoError(t, err)

	areg := NewActionRegistry()
	areg.Register(&NotifyAction{Svc: svc})
	svc.SetActionRegistry(areg)

	tpl := "b33e2_tpl_" + uniqueSuffix()
	require.NoError(t, tplRepo.Create(ctx, &model.MessageTemplate{
		Code: tpl, Name: "动作通知", Channel: "uip", BizType: "ticket",
		Subject: "{{ticket.title}}", Body: "{{ticket.human_key}}", Enabled: true,
	}))
	tpl2 := "b33e2_tpl2_" + uniqueSuffix()
	require.NoError(t, tplRepo.Create(ctx, &model.MessageTemplate{
		Code: tpl2, Name: "动作通知·覆盖版", Channel: "uip", BizType: "ticket",
		Subject: "[OVR] {{ticket.title}}", Body: "override", Enabled: true,
	}))
	// 000079 起有种子规则（ticket_action_notify）——upsert 到测试模板，cleanup 复原种子
	upsertRule := func(code string) {
		_, err := testPool.Exec(ctx, `INSERT INTO message_rules (biz_type, event_action, template_code, recipient_type)
			VALUES ('ticket', 'ticket.action_notify', $1, 'cc')
			ON CONFLICT (biz_type, event_action) DO UPDATE SET template_code = EXCLUDED.template_code, enabled = true`, code)
		require.NoError(t, err)
	}
	upsertRule(tpl)
	t.Cleanup(func() {
		upsertRule("ticket_action_notify") // 复原 000079 种子（负向子测会删规则）
		_, _ = testPool.Exec(ctx, `DELETE FROM message_templates WHERE code IN ($1, $2)`, tpl, tpl2)
	})

	setMeta := func(params string) {
		t.Helper()
		meta := `{"assigned>in_progress":{"post_actions":[{"name":"notify","params":` + params + `}]}}`
		_, err := svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: []byte(meta)})
		require.NoError(t, err)
	}
	setMeta(`{"recipients":[` + itoa64(bid) + `]}`)

	run := func(t *testing.T, title string) *model.Ticket {
		t.Helper()
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: title, Priority: 2, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid))
		time.Sleep(100 * time.Millisecond)
		_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "in_progress"}, bid)
		require.NoError(t, err)
		time.Sleep(200 * time.Millisecond) // 异步派发
		return tk
	}

	t.Run("正向：notify 动作→action_notify 事件→cc 规则→发送+log", func(t *testing.T) {
		tk := run(t, "动作通知正向单")

		var got *channelsender.Message
		for i := range sender.messages() {
			if m := sender.messages()[i]; m.TemplateCode == tpl {
				got = &m
				break
			}
		}
		require.NotNil(t, got, "action_notify 事件经规则路由到发送")
		assert.Contains(t, got.Recipients, itoa64(bid), "cc 档消费 params.recipients→cc_users")

		var n int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT count(*) FROM message_log WHERE biz_id=$1 AND template_code=$2`,
			itoa64(tk.ID), tpl).Scan(&n))
		assert.Equal(t, 1, n, "动作通知落 log")
	})

	t.Run("模板参数覆盖：params.template 指定 tpl2 → 覆盖规则默认（2026-10-26 收口批生效锚）", func(t *testing.T) {
		upsertRule(tpl) // 负向子测可能删过规则——复原到规则默认 tpl
		setMeta(`{"recipients":[` + itoa64(bid) + `], "template":"` + tpl2 + `"}`)
		before := len(sender.messages())

		tk := run(t, "动作通知覆盖单")

		var got *channelsender.Message
		for i := range sender.messages()[before:] {
			if m := sender.messages()[before:][i]; m.TemplateCode == tpl2 {
				got = &m
				break
			}
		}
		require.NotNil(t, got, "动作显式指定的模板须覆盖规则默认")
		assert.Contains(t, got.Subject, "[OVR]", "渲染自覆盖模板")
		for _, m := range sender.messages()[before:] {
			assert.NotEqual(t, tpl, m.TemplateCode, "规则默认模板不得同时再发一份")
		}
		var n int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT count(*) FROM message_log WHERE biz_id=$1 AND template_code=$2`,
			itoa64(tk.ID), tpl2).Scan(&n))
		assert.Equal(t, 1, n, "覆盖模板落 log")

		setMeta(`{"recipients":[` + itoa64(bid) + `]}`) // 还原无 template 参数形态
	})

	t.Run("负向：无规则静默（撤规则后同链路零发送零 log——规则仍是开关）", func(t *testing.T) {
		_, _ = testPool.Exec(ctx, `DELETE FROM message_rules WHERE event_action='ticket.action_notify'`)
		before := len(sender.messages())

		tk := run(t, "动作通知静默单")

		for _, m := range sender.messages()[before:] {
			assert.NotEqual(t, tpl, m.TemplateCode, "无规则=动作静默（设计内——cc 动作等旁链不染）")
		}
		var n int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT count(*) FROM message_log WHERE biz_id=$1 AND template_code=$2`,
			itoa64(tk.ID), tpl).Scan(&n))
		assert.Equal(t, 0, n, "静默不落发送行")
	})
}

func itoa64(v int64) string { return strconv.FormatInt(v, 10) }
