//go:build integration

// S10 B10-4 节点超时集成测试：写入点（status_changed_at/supervisor 指定）+
// 扫描链路（候选/观察模式/催办水位/间隔与上限/目标集含人工指定上级）。
package ticket

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// backdateStatusChanged 把单的进入状态时间拨回（造超时候选——测试专用直改库）。
func backdateStatusChanged(t *testing.T, ticketID int64, d time.Duration) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`UPDATE tickets SET status_changed_at = NOW() - make_interval(secs => $2::double precision / 1.0) WHERE id = $1`,
		ticketID, d.Seconds())
	require.NoError(t, err)
}

func TestB104_Timeout(t *testing.T) {
	ctx := context.Background()

	// 通知链组装（000064 种子规则：ticket.timeout → ticket_timeout_notify, cc）
	reg := domainevent.NewHookRegistry(nil)
	sender := &captureSender{}
	hook := NewTicketNotifyHook(
		repository.NewMessageTemplateRepo(testPool), repository.NewMessageRuleRepo(testPool),
		repository.NewMessageLogRepo(testPool), repository.NewNotificationOptoutRepo(testPool),
		sender, nil, nil)
	reg.Subscribe(domainevent.BizTicket, "b104.notify", hook)

	svc, aid, bid, vid, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"} // assign 需主管面
	oid := childOrgID(t, "b104")
	code := "b104_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "超时批"})
	require.NoError(t, err)
	// transition_meta：assigned 节点 60 分钟超时 + escalation（间隔 30 分/上限 2 次/
	// notify 含 supervisor——人工指定档）+ in_progress 节点 sla_due_hours 1h（G1-1 锚）
	meta := `{"assigned":{"timeout_minutes":60,"escalation":{"notify":["assignee","supervisor"],"reminder_interval_minutes":30,"max_reminders":2}},"in_progress":{"sla_due_hours":1}}`
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: jsonRaw(t, meta)})
	require.NoError(t, err)

	tk, err := svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: code, Title: "超时链路单", Priority: 3, OrgID: oid,
		EscalationSupervisor: &vid, // 发起时人工指定催办上级（dd §31 基线档）
	}, aid)
	require.NoError(t, err)
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid))

	scanner := NewTimeoutScanner(testPool, reg, false, nil)
	observeScanner := NewTimeoutScanner(testPool, reg, true, nil)

	countReminder := func() (int, time.Time) {
		var c int
		var last *time.Time
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT reminder_count, last_reminder_at FROM ticket_reminders WHERE ticket_id = $1 AND state = 'assigned'`, tk.ID).
			Scan(&c, &last))
		var lt time.Time
		if last != nil {
			lt = *last
		}
		return c, lt
	}
	timeoutMsgs := func() int {
		n := 0
		for _, m := range sender.messages() {
			if m.TemplateCode == "ticket_timeout_notify" {
				n++
			}
		}
		return n
	}

	t.Run("未超时：扫描空过（水位零行）", func(t *testing.T) {
		require.NoError(t, scanner.Handle(ctx, nil))
		var rows int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM ticket_reminders WHERE ticket_id = $1`, tk.ID).Scan(&rows))
		assert.Zero(t, rows)
	})

	t.Run("超时候选：事件到人（assignee∪人工指定上级）+ 水位 1", func(t *testing.T) {
		backdateStatusChanged(t, tk.ID, 2*time.Hour)
		require.NoError(t, scanner.Handle(ctx, nil))
		time.Sleep(200 * time.Millisecond) // 事件异步派发

		msgs := sender.messages()
		require.NotEmpty(t, msgs, "超时候选须触发通知")
		var hit *struct{ recipients []string }
		for _, m := range msgs {
			if m.TemplateCode == "ticket_timeout_notify" {
				hit = &struct{ recipients []string }{m.Recipients}
			}
		}
		require.NotNil(t, hit, "须有 ticket_timeout_notify 发送")
		assert.ElementsMatch(t, []string{itoa(bid), itoa(vid)}, hit.recipients,
			"目标集=处理人 ∪ 人工指定催办上级（cc 档载荷锚点）")
		c, _ := countReminder()
		assert.Equal(t, 1, c)
	})

	t.Run("间隔未到：不重发", func(t *testing.T) {
		require.NoError(t, scanner.Handle(ctx, nil))
		time.Sleep(150 * time.Millisecond)
		assert.Equal(t, 1, timeoutMsgs())
		c, _ := countReminder()
		assert.Equal(t, 1, c)
	})

	t.Run("间隔已到：第 2 次催办；达上限后停", func(t *testing.T) {
		// 催办重复断言钉水位（权威源）：通知面 60s 节流窗与测试加速的时间回拨
		// 互扰（生产间隔 ≥5min 恒 > 节流窗——校验层已下限挡配置），消息送达链
		// 已由首催子测验证
		_, err := testPool.Exec(ctx, `UPDATE ticket_reminders SET last_reminder_at = NOW() - interval '31 minutes' WHERE ticket_id = $1`, tk.ID)
		require.NoError(t, err)
		require.NoError(t, scanner.Handle(ctx, nil))
		c, _ := countReminder()
		assert.Equal(t, 2, c, "间隔已到水位 +1")

		// 上限 2 已达——再扫不进位
		_, err = testPool.Exec(ctx, `UPDATE ticket_reminders SET last_reminder_at = NOW() - interval '31 minutes' WHERE ticket_id = $1`, tk.ID)
		require.NoError(t, err)
		require.NoError(t, scanner.Handle(ctx, nil))
		c, _ = countReminder()
		assert.Equal(t, 2, c, "max_reminders=2 达上限停止")
	})

	t.Run("观察模式：只记日志不发送不耗水位", func(t *testing.T) {
		// 新候选（第二张单，无 escalation 段=单发档）
		tk2, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "观察模式单", Priority: 3, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk2.ID, AssignedTo: &bid}, aid))
		backdateStatusChanged(t, tk2.ID, 2*time.Hour)

		before := timeoutMsgs()
		require.NoError(t, observeScanner.Handle(ctx, nil))
		time.Sleep(150 * time.Millisecond)
		assert.Equal(t, before, timeoutMsgs(), "观察模式不发送")
		var rows int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM ticket_reminders WHERE ticket_id = $1`, tk2.ID).Scan(&rows))
		assert.Zero(t, rows, "观察模式不耗水位")
	})

	t.Run("D1 修复：重入同状态后水位归零（审计 D1）", func(t *testing.T) {
		tk2, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "D1 重入单", Priority: 3, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk2.ID, AssignedTo: &bid}, aid))
		backdateStatusChanged(t, tk2.ID, 2*time.Hour)
		for i := 0; i < 2; i++ {
			require.NoError(t, scanner.Handle(ctx, nil))
			_, err := testPool.Exec(ctx, `UPDATE ticket_reminders SET last_reminder_at = NOW() - interval '31 minutes' WHERE ticket_id = $1`, tk2.ID)
			require.NoError(t, err)
		}
		// 转出→转回（走 Assign 通道——Assign 也经 UpdateStatusTx 写状态但不走 Transition
		// 的清水位逻辑……Assign 不清水位！只有 Transition 清。所以此处用 Transition 转出再转回）
		_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk2.ID, To: "open"}, bid)
		require.NoError(t, err)
		_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk2.ID, To: "assigned"}, bid)
		require.NoError(t, err)
		backdateStatusChanged(t, tk2.ID, 2*time.Hour)

		var count int
		err = testPool.QueryRow(ctx,
			`SELECT reminder_count FROM ticket_reminders WHERE ticket_id = $1 AND state = 'assigned'`, tk2.ID).Scan(&count)
		if err != nil { // 行可能不存在（DELETE 后未再催）——nil 即归零
			count = 0
		}
		assert.Zero(t, count, "重入后旧状态水位被 Transition 清零")

		require.NoError(t, scanner.Handle(ctx, nil))
		var after int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT reminder_count FROM ticket_reminders WHERE ticket_id = $1 AND state = 'assigned'`, tk2.ID).Scan(&after))
		assert.Equal(t, 1, after, "重入后再催=从 1 起算")
	})

	t.Run("D1 补全：Assign 通道状态变更同清水位（三路同源）", func(t *testing.T) {
		tk3, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "D1 Assign 通道单", Priority: 3, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk3.ID, AssignedTo: &bid}, aid))
		backdateStatusChanged(t, tk3.ID, 2*time.Hour)
		require.NoError(t, scanner.Handle(ctx, nil))
		// 取消分派（assigned→open——Assign 通道状态变更）→ assigned 水位应清
		// （修复前仅 Transition 清——Assign 残留致重分派后水位累加永不催）
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk3.ID, AssignedTo: nil}, aid))
		var count int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM ticket_reminders WHERE ticket_id = $1 AND state = 'assigned'`, tk3.ID).Scan(&count))
		assert.Zero(t, count, "Assign 通道状态变更清旧状态水位")
		// 重分派回 assigned → 再催=从 1 起算（非残留累加）
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk3.ID, AssignedTo: &bid}, aid))
		backdateStatusChanged(t, tk3.ID, 2*time.Hour)
		require.NoError(t, scanner.Handle(ctx, nil))
		var after int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT reminder_count FROM ticket_reminders WHERE ticket_id = $1 AND state = 'assigned'`, tk3.ID).Scan(&after))
		assert.Equal(t, 1, after, "重分派后水位从 1 起算")
	})

	t.Run("D1 补全：Close 通道清旧状态水位", func(t *testing.T) {
		tk4, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "D1 Close 通道单", Priority: 3, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk4.ID, AssignedTo: &bid}, aid))
		backdateStatusChanged(t, tk4.ID, 2*time.Hour)
		require.NoError(t, scanner.Handle(ctx, nil))
		// 默认六态图 assigned→closed 不邻接——先转 in_progress 再 Close
		_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk4.ID, To: "in_progress"}, bid)
		require.NoError(t, err)
		// Close 前直插 in_progress 水位行（类型未配 in_progress timeout 扫描器
		// 造不出——SQL 直插等价模拟），使断言特异指向 Close 的清理
		// （Transition 清 assigned / Close 清 in_progress——两断言互不覆盖）
		if _, err := testPool.Exec(ctx,
			`INSERT INTO ticket_reminders (ticket_id, state, reminder_count, last_reminder_at) VALUES ($1, 'in_progress', 1, NOW())`, tk4.ID); err != nil {
			require.NoError(t, err)
		}
		require.NoError(t, svc.Close(ctx, &model.CloseTicketRequest{ID: tk4.ID}, aid))
		var count int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM ticket_reminders WHERE ticket_id = $1 AND state = 'in_progress'`, tk4.ID).Scan(&count))
		assert.Zero(t, count, "Close 清旧状态水位（修复前残留——重开场景永不催）")
	})

	t.Run("状态离开：partial 索引天然排除", func(t *testing.T) {
		before := timeoutMsgs() // D1 子测可能追加了 tk2 的催办——取基线差值
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: nil}, aid))
		_, err := testPool.Exec(ctx, `UPDATE ticket_reminders SET last_reminder_at = NOW() - interval '31 minutes' WHERE ticket_id = $1`, tk.ID)
		require.NoError(t, err)
		require.NoError(t, scanner.Handle(ctx, nil))
		time.Sleep(150 * time.Millisecond)
		assert.Equal(t, before, timeoutMsgs(), "离开超时状态后不再催办（tk 离开 assigned）")
	})

	// G1-1 审计锚：deadline 水位键读写一致（修复前 shouldRemind 读裸 state 而
	// bumpReminder 写 deadline:state——查重恒不命中每 tick 重发，且被 timeout
	// 水位反向压制）
	deadlineMsgs := func() int {
		n := 0
		for _, m := range sender.messages() {
			if m.TemplateCode == "ticket_deadline_notify" {
				n++
			}
		}
		return n
	}
	t.Run("SLA deadline：到期单次提醒+查重（G1-1 修复锚）", func(t *testing.T) {
		before := deadlineMsgs()
		tk5, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "SLA deadline 单", Priority: 3, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk5.ID, AssignedTo: &bid}, aid))
		// Transition 到 in_progress（to 态 sla_due_hours=1 → due=NOW()+1h 写入）
		_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk5.ID, To: "in_progress"}, bid)
		require.NoError(t, err)
		var due int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM tickets WHERE id = $1 AND sla_due_at IS NOT NULL`, tk5.ID).Scan(&due))
		require.Equal(t, 1, due, "Transition 后 due_at 写入（in_progress sla_due_hours 配置）")
		// 拨到期 → 第一次扫描：应发一次
		_, err = testPool.Exec(ctx, `UPDATE tickets SET sla_due_at = NOW() - interval '1 minute' WHERE id = $1`, tk5.ID)
		require.NoError(t, err)
		require.NoError(t, scanner.Handle(ctx, nil))
		time.Sleep(150 * time.Millisecond)
		assert.Equal(t, before+1, deadlineMsgs(), "到期首扫发一次 deadline 通知")
		var count int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT reminder_count FROM ticket_reminders WHERE ticket_id = $1 AND state = 'deadline:in_progress'`, tk5.ID).Scan(&count))
		assert.Equal(t, 1, count, "deadline 水位写在 deadline: 前缀键")
		// 第二次扫描：水位查重应拦截（修复前每 tick 重发）。断言走水位而非消息
		// 计数——重发会被 S6 节流窗吞（消息面不红），但 scanner 侧 bumpReminder
		// 同步执行必递增——水位是穿透节流的特异性证据。
		require.NoError(t, scanner.Handle(ctx, nil))
		var count2 int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT reminder_count FROM ticket_reminders WHERE ticket_id = $1 AND state = 'deadline:in_progress'`, tk5.ID).Scan(&count2))
		assert.Equal(t, 1, count2, "第二扫查重拦截——水位不递增（变异态重发+bump→2 必红）")
		time.Sleep(150 * time.Millisecond)
		assert.Equal(t, before+1, deadlineMsgs(), "第二扫查重拦截——不重发")
	})
}

func TestB104_WritePoints(t *testing.T) {
	ctx := context.Background()
	svc, aid, bid, vid, roles := setupTicket2a(t)
	oid := childOrgID(t, "b104w")

	t.Run("Create：status_changed_at 初始化 + supervisor 落库", func(t *testing.T) {
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: "incident", Title: "写入点单", Priority: 3, OrgID: oid,
			EscalationSupervisor: &vid,
		}, aid)
		require.NoError(t, err)
		got, err := svc.Get(ctx, tk.ID, aid)
		require.NoError(t, err)
		require.NotNil(t, got.StatusChangedAt, "创建即初始化进入状态时间")
		assert.False(t, got.StatusChangedAt.After(time.Now().Add(time.Minute)))
		require.NotNil(t, got.EscalationSupervisor)
		assert.EqualValues(t, vid, *got.EscalationSupervisor)
	})

	t.Run("Assign：状态转换刷新 status_changed_at + supervisor 随分派改", func(t *testing.T) {
		roles[aid] = []string{"admin"} // assign 需主管面
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: "incident", Title: "分派写入点单", Priority: 3, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		before, err := svc.Get(ctx, tk.ID, aid)
		require.NoError(t, err)
		require.NotNil(t, before.StatusChangedAt)

		time.Sleep(1100 * time.Millisecond) // 造时差（同秒难以断言先后）
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{
			ID: tk.ID, AssignedTo: &bid, EscalationSupervisor: &vid,
		}, aid))
		after, err := svc.Get(ctx, tk.ID, aid)
		require.NoError(t, err)
		require.NotNil(t, after.StatusChangedAt)
		assert.True(t, after.StatusChangedAt.After(*before.StatusChangedAt),
			"状态转换（open→assigned）刷新进入状态时间——UpdateStatusTx 咽喉同写")
		require.NotNil(t, after.EscalationSupervisor)
		assert.EqualValues(t, vid, *after.EscalationSupervisor, "分派时人工指定催办上级落库")
	})

	t.Run("幻影 supervisor：FK 400", func(t *testing.T) {
		roles[aid] = []string{"admin"}
		ghost := int64(999999999)
		err := svc.Assign(ctx, &model.AssignTicketRequest{ID: 1, AssignedTo: &bid, EscalationSupervisor: &ghost}, aid)
		// ID=1 若不存在先 404——换实存单：直接建一张
		tk, cerr := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: "incident", Title: "幻影上级单", Priority: 3, OrgID: oid,
		}, aid)
		require.NoError(t, cerr)
		err = svc.Assign(ctx, &model.AssignTicketRequest{
			ID: tk.ID, AssignedTo: &bid, EscalationSupervisor: &ghost,
		}, aid)
		requireErrCode(t, err, errcode.ErrUserNotFound.Code) // 幽灵 supervisor 前移为 30002（分派目标校验批）
	})
}

// jsonRaw 测试便捷：[]byte 直转 json.RawMessage。
func jsonRaw(t *testing.T, s string) []byte {
	t.Helper()
	return []byte(s)
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
