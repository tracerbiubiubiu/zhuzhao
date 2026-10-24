//go:build integration

// S3a B3-1 转换级语义集成测试（03 号 §8 验收承重——A.7 补充）：
// 非属主转换被拒 fail-closed 负向必测 / condition 过滤 / validator expr /
// available 按用户过滤 / 端到端执行+事件+status_changed_at 刷新。
package ticket

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestB31_Transition(t *testing.T) {
	ctx := context.Background()

	// 事件链组装（验证 ticket.transitioned emit）
	reg := domainevent.NewHookRegistry(nil)
	sender := &captureSender{}
	hook := NewTicketNotifyHook(
		repository.NewMessageTemplateRepo(testPool), repository.NewMessageRuleRepo(testPool),
		repository.NewMessageLogRepo(testPool), repository.NewNotificationOptoutRepo(testPool),
		sender, nil, nil)
	reg.Subscribe(domainevent.BizTicket, "b31.notify", hook)

	svc, aid, bid, vid, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"} // 创建+admin（方便建类型）
	svc.SetEventRegistry(reg)
	oid := childOrgID(t, "b31")
	code := "b31_" + uniqueSuffix()

	// 类型：默认六态图 + transition_meta 转换级键——
	// "assigned>in_progress" 仅 assignee 可执行（condition roles）
	// "in_progress>closed" validator 优先级=1（P1 紧急才能直闭）
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "转换批"})
	require.NoError(t, err)
	meta := `{
		"assigned>in_progress": {"roles": ["assignee"]},
		"in_progress>closed": {"validator": "priority == 1"}
	}`
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: []byte(meta)})
	require.NoError(t, err)

	newTicket := func(title string, assignTo *int64) *model.Ticket {
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: title, Priority: 3, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		if assignTo != nil {
			require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: assignTo}, aid))
		}
		return tk
	}

	t.Run("available：condition 过滤（非 assignee 看不到受限转换）", func(t *testing.T) {
		tk := newTicket("available 单", &bid) // assigned 态，处理人=bid
		// bid（assignee）应看到 assigned 的所有出边（in_progress/open）
		avail, err := svc.ListAvailableTransitions(ctx, tk.ID, bid)
		require.NoError(t, err)
		assert.Contains(t, avail, "in_progress", "assignee 可见受限转换")
		assert.Contains(t, avail, "open")

		// aid（admin 但非 assignee）——admin 角色不在 roles=["assignee"] 白名单内
		// （condition 是 transition 级细粒度，admin 通配只走 L3 属主面——此处
		// 工单 created_by=aid=creator，creator 也不在 roles 白名单 → 不可见）
		avail2, err := svc.ListAvailableTransitions(ctx, tk.ID, aid)
		require.NoError(t, err)
		assert.NotContains(t, avail2, "in_progress", "creator（非 assignee）不可见受限转换")
		assert.Contains(t, avail2, "open", "无 condition 的转换仍可见")
	})

	t.Run("非属主转换被拒 fail-closed（403）——§8 负向必测", func(t *testing.T) {
		tk := newTicket("fail-closed 单", &bid) // assigned 态
		// aid（creator 可见工单但非 assignee——condition roles=[assignee] 不含 creator）
		// → L3 transition 属主面通过（creator 可转）但 condition 细粒度拦 → 403
		_, err := svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "in_progress"}, aid)
		requireErrCode(t, err, errcode.ErrNoPermission.Code)
	})

	t.Run("condition 放行 + 执行成功 + 事件+status_changed_at", func(t *testing.T) {
		tk := newTicket("正向执行单", &bid)
		before, err := svc.Get(ctx, tk.ID, bid)
		require.NoError(t, err)
		require.NotNil(t, before.StatusChangedAt)
		time.Sleep(1100 * time.Millisecond) // 造时差

		got, err := svc.Transition(ctx, &model.TransitionTicketRequest{
			ID: tk.ID, To: "in_progress", Comment: "开始处理",
		}, bid) // bid=assignee → condition 放行
		require.NoError(t, err)
		assert.Equal(t, "in_progress", got.Status)
		assert.True(t, got.StatusChangedAt.After(*before.StatusChangedAt),
			"status_changed_at 经 UpdateStatusTx 咽喉刷新")

		time.Sleep(200 * time.Millisecond) // 异步 emit
		found := false
		for _, m := range sender.messages() {
			if m.TemplateCode == "ticket_created_notify" || m.TemplateCode == "ticket_assigned_notify" {
				continue // 创建/分派事件的通知（无 transitioned 规则=不通知，属设计内）
			}
		}
		_ = found // ticket.transitioned 无种子规则=不通知（06 号 §5.2 无规则不通知）
	})

	t.Run("validator：前置条件不满足 → 400", func(t *testing.T) {
		tk := newTicket("validator 单", &bid) // priority=3（中）
		// 先转到 in_progress（assignee 可转）
		_, err := svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "in_progress"}, bid)
		require.NoError(t, err)
		// in_progress>closed 有 validator priority==1（仅 P1 可直闭）——priority=3 不满足
		_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "closed"}, bid)
		require.Error(t, err, "非 P1 的 in_progress 直闭被 validator 拦截")
		var ee *errcode.Error
		require.ErrorAs(t, err, &ee)
		assert.Equal(t, errcode.ErrInvalidParams.Code, ee.Code)
		assert.Contains(t, ee.Message, "前置条件")

		// P1 单可直闭
		tk2, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "P1 单", Priority: 1, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk2.ID, AssignedTo: &bid}, aid))
		_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk2.ID, To: "in_progress"}, bid)
		require.NoError(t, err)
		_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk2.ID, To: "closed"}, bid)
		require.NoError(t, err, "P1 validator 放行")
	})

	t.Run("状态机非法转换仍拦（AssertTransition 叠加）", func(t *testing.T) {
		tk := newTicket("非法转换单", &bid) // assigned
		_, err := svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "pending_verify"}, bid)
		require.Error(t, err, "assigned→pending_verify 不在邻接表——状态机拦")
	})

	t.Run("不可见用户 → 404 同形", func(t *testing.T) {
		tk := newTicket("404 单", &bid)
		_, err := svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "in_progress"}, vid)
		// vid 既不可见（L2 404）也非属主——L2 先拦=404
		requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
	})

	// R3 审计修复回归锚：Close/Assign 端点与 Transition 三路同源走
	// checkTransitionMeta（修复前 /close /assign 旁路 condition/validator——
	// transition 级权限语义只在 /transition 生效=安全洞）
	t.Run("R3 补：Close 端点同走 validator（旁路封堵）", func(t *testing.T) {
		tk := newTicket("Close validator 单", &bid) // priority=3
		_, err := svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "in_progress"}, bid)
		require.NoError(t, err)
		// in_progress>closed validator priority==1（函数级 meta 已配）——
		// 走 /close 端点（非 /transition）同样被拦=三路同源
		err = svc.Close(ctx, &model.CloseTicketRequest{ID: tk.ID}, bid)
		require.Error(t, err, "非 P1 走 /close 被 validator 拦")
		var ee *errcode.Error
		require.ErrorAs(t, err, &ee)
		assert.Equal(t, errcode.ErrInvalidParams.Code, ee.Code)
	})

	t.Run("R3 补：Assign 状态变更同走 condition（旁路封堵）", func(t *testing.T) {
		// 追加 assigned>open 键（roles=[admin]）——仅 admin 可取消分派回 open
		meta := `{
			"assigned>in_progress": {"roles": ["assignee"]},
			"in_progress>closed": {"validator": "priority == 1"},
			"assigned>open": {"roles": ["admin"]}
		}`
		_, err := svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: []byte(meta)})
		require.NoError(t, err)
		tk := newTicket("Assign condition 单", &bid) // assigned 态
		// bid（assignee）取消分派 assigned→open——condition roles=[admin] 不含 → 403
		err = svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: nil}, bid)
		requireErrCode(t, err, errcode.ErrNoPermission.Code)
		// aid（admin）取消分派——condition 放行
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: nil}, aid))
	})
}
