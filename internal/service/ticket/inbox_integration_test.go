//go:build integration

// S6 二期 B6-7 站内消息中心集成测试：事件→inbox 落库（收件人锚点/去重）/
// 已读单条+全部/未读数聚合/保留策略清理/未登记动作不落库。
package ticket

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
)

func TestB67_Inbox(t *testing.T) {
	ctx := context.Background()
	pool := testPool

	reg := domainevent.NewHookRegistry(nil)
	inbox := NewInboxService(pool, 90, nil)
	inbox.SetEventRegistry(reg)

	// 造收件人用户（FK——每 run 唯一工号防历史冲突）
	suffix := uniqueSuffix()
	_, err := pool.Exec(ctx, `INSERT INTO users (username, password, employee_no, status) VALUES ('b67a_'||$1,'hash','EB67A_'||$1,1),('b67b_'||$1,'hash','EB67B_'||$1,1)`, suffix)
	require.NoError(t, err)
	var uidA, uidB int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM users WHERE employee_no='EB67A_'||$1`, suffix).Scan(&uidA))
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM users WHERE employee_no='EB67B_'||$1`, suffix).Scan(&uidB))
	aid, bid := uidA, uidB

	ev := func(action domainevent.Action, to, cc string) domainevent.DomainEvent {
		return domainevent.DomainEvent{
			BizType: domainevent.BizTicket, Action: action, BizID: "777",
			ActorID: aid,
			Payload: domainevent.Payload{
				"title": "B67 单", "human_key": "TK-B67-0001",
				"to_assignee": to, "cc_users": cc,
			},
			Timestamp: time.Now(),
		}
	}

	t.Run("事件→落库：assigned 单收件人 + cc 多人去重", func(t *testing.T) {
		reg.Dispatch(ctx, ev(domainevent.TicketAssigned, itoaInbox(bid), itoaInbox(aid)+","+itoaInbox(bid)))
		// 落库为逐人 INSERT（inbox 循环）——两行可见时点不同步：只等 assignee
		// 行会在 cc 行落库前抢跑断言（2026-10-26 CI flake 实锤，nA actual=0）；
		// 两行都等到再取精确值（去重后各恰 1 条，本子测试内无后续写入）
		assert.Eventually(t, func() bool {
			var a, b int64
			require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE recipient_id = $1`, aid).Scan(&a))
			require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE recipient_id = $1`, bid).Scan(&b))
			return a >= 1 && b >= 1
		}, 2*time.Second, 50*time.Millisecond)
		var nA, nB int64
		require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE recipient_id = $1`, aid).Scan(&nA))
		require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE recipient_id = $1`, bid).Scan(&nB))
		assert.EqualValues(t, 1, nA, "cc 收件人落库")
		assert.EqualValues(t, 1, nB, "assignee+cc 去重——单条")
	})

	t.Run("未登记动作不落库", func(t *testing.T) {
		before := countAll(t)
		reg.Dispatch(ctx, ev("ticket.unknown_action", itoaInbox(bid), ""))
		time.Sleep(200 * time.Millisecond)
		assert.Equal(t, before, countAll(t))
	})

	t.Run("读面：inbox 分页/单条已读/全部已读/未读数", func(t *testing.T) {
		// bid 再来两条。Dispatch 每订阅者一个 goroutine——两条紧挨着发，落库顺序不定（曾 3 跑 1 挂），
		// 「最新在前」断言依赖 id 序，故第一条落库后再发第二条
		reg.Dispatch(ctx, ev(domainevent.TicketClosed, itoaInbox(bid), ""))
		assert.Eventually(t, func() bool {
			n, err := inbox.UnreadCount(ctx, bid)
			return err == nil && n >= 2
		}, 2*time.Second, 50*time.Millisecond)
		reg.Dispatch(ctx, ev(domainevent.TicketTimeout, itoaInbox(bid), ""))
		assert.Eventually(t, func() bool {
			n, err := inbox.UnreadCount(ctx, bid)
			return err == nil && n >= 3
		}, 2*time.Second, 50*time.Millisecond)

		list, total, err := inbox.Inbox(ctx, bid, false, 1, 20)
		require.NoError(t, err)
		assert.EqualValues(t, 3, total)
		require.NotEmpty(t, list)
		assert.Contains(t, list[0].Title, "超时催办", "倒序——最新在前")
		assert.NotEmpty(t, list[0].Link, "深链入站即渲染")

		// 单条已读
		require.NoError(t, inbox.MarkRead(ctx, list[0].ID, bid))
		n, err := inbox.UnreadCount(ctx, bid)
		require.NoError(t, err)
		assert.EqualValues(t, 2, n)
		// 非本人已读 → 404
		err = inbox.MarkRead(ctx, list[0].ID, aid)
		require.Error(t, err, "非本人 404 同形")

		// unread 过滤
		_, unreadTotal, err := inbox.Inbox(ctx, bid, true, 1, 20)
		require.NoError(t, err)
		assert.EqualValues(t, 2, unreadTotal)

		// 全部已读
		marked, err := inbox.MarkAllRead(ctx, bid)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, marked, int64(2))
		n, err = inbox.UnreadCount(ctx, bid)
		require.NoError(t, err)
		assert.Zero(t, n)
	})

	t.Run("保留策略：过期行物理清理", func(t *testing.T) {
		// 直插一条 100 天前的旧行
		_, err := pool.Exec(ctx, `
			INSERT INTO notifications (recipient_id, biz_type, title, created_at)
			VALUES ($1, 'ticket', '旧通知', NOW() - interval '100 days')`, bid)
		require.NoError(t, err)
		err = inbox.CleanupExpired(ctx)
		require.NoError(t, err)
		var remaining int64
		require.NoError(t, testPool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE created_at < NOW() - interval '90 days'`).Scan(&remaining))
		assert.Zero(t, remaining, "过期行已物理清理")
	})
}

func countAll(t *testing.T) int64 {
	t.Helper()
	var n int64
	require.NoError(t, testPool.QueryRow(context.Background(), `SELECT COUNT(*) FROM notifications`).Scan(&n))
	return n
}

func itoaInbox(v int64) string { return strconv.FormatInt(v, 10) }
