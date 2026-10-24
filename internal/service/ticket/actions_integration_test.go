//go:build integration

// S3a B3-3 动作库集成测试：ActionRegistry 注册/链深/post_actions 消费
// （cc 动作真实链路/notify 兜底事件/reassign 转派/未注册跳过/失败不回滚转换）。
package ticket

import (
	"context"
	"strconv"
	"testing"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
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
	reg.Register(&NotifyAction{Reg: svc.eventReg})
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

func itoa64(v int64) string { return strconv.FormatInt(v, 10) }
