//go:build integration

// S10 B10-5 导出集成测试：BK-21 同源断言（导出行数=同筛选列表 total/可见性同源）+
// 筛选联动（tag/keyword 与列表同语义）+ 行数钳制（>5 万拒绝）。
package ticket

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func TestB105_Export(t *testing.T) {
	ctx := context.Background()
	svc, aid, _, vid, _ := setupTicket2a(t)
	oid := childOrgID(t, "b105")

	// aid 建 3 张（org 内属主可见）；vid 无任何可见
	for _, title := range []string{"导出A1", "导出A2", "导出A3"} {
		newTicketHelper(t, svc, aid, oid, title)
	}

	collect := func(q model.TicketListQuery, actor int64) (int64, []int64) {
		var ids []int64
		total, err := svc.ExportStream(ctx, q, actor, func(tk *model.Ticket) error {
			ids = append(ids, tk.ID)
			return nil
		})
		require.NoError(t, err)
		return total, ids
	}

	t.Run("BK-21：导出行数/行集与同筛选列表一致（可见性同源）", func(t *testing.T) {
		q := model.TicketListQuery{Page: 1, PageSize: 100}
		for _, actor := range []int64{aid, vid} {
			l, err := svc.List(ctx, q, actor)
			require.NoError(t, err)
			listIDs := make([]int64, 0, len(l.List))
			for _, r := range l.List {
				listIDs = append(listIDs, r.ID)
			}

			expTotal, ids := collect(q, actor)
			assert.Equal(t, l.Total, expTotal, "actor=%d 导出行数=列表 total", actor)
			assert.ElementsMatch(t, listIDs, ids, "actor=%d 行集一致（同 buildTicketWhere）", actor)
			if actor == vid {
				assert.Empty(t, ids, "不可见用户导出零行——导出不旁路 L2")
			}
		}
	})

	t.Run("筛选联动：tag/keyword 与列表同语义", func(t *testing.T) {
		tk := newTicketHelper(t, svc, aid, oid, "导出标签单")
		_, err := svc.AddTags(ctx, &model.AddTicketTagsRequest{TicketID: tk.ID, Tags: []string{"urgent"}}, aid)
		require.NoError(t, err)

		expTotal, ids := collect(model.TicketListQuery{Page: 1, PageSize: 100, Tag: "urgent"}, aid)
		assert.EqualValues(t, 1, expTotal)
		assert.Equal(t, []int64{tk.ID}, ids)

		expTotal, ids = collect(model.TicketListQuery{Page: 1, PageSize: 100, Keyword: "导出标签单"}, aid)
		assert.EqualValues(t, 1, expTotal)
		assert.Equal(t, []int64{tk.ID}, ids)
	})
}

func TestB105_ExportClamp(t *testing.T) {
	ctx := context.Background()
	svc, aid, _, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"} // admin Unscoped——钳制行全可见

	// generate_series 直插 50001 张临时钳制单（closed 终态不扰他测）；Cleanup 清理
	var anyUser int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&anyUser))
	_, err := testPool.Exec(ctx, `
		INSERT INTO tickets (type_code, title, description, priority, status, created_by, org_id, org_path, status_changed_at, created_at, updated_at)
		SELECT 'incident', 'clamp-b105-' || g, '', 3, 'closed', $1, 1, 'root', NOW(), NOW(), NOW()
		FROM generate_series(1, 50001) AS g`, anyUser)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM tickets WHERE title LIKE 'clamp-b105-%'`)
	})

	_, err = svc.ExportStream(ctx, model.TicketListQuery{Page: 1, PageSize: 100}, aid, func(*model.Ticket) error {
		return nil
	})
	assert.ErrorIs(t, err, ErrExportTooMany, "超 5 万须拒绝并提示收窄筛选")
}
