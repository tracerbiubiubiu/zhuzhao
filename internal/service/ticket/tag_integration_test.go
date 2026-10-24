//go:build integration

// S10 B10-2 标签集成测试（10 号 §8/A.7 tags 三向 + 授权口径）：
// 跨用户共享可见 / 重复挂载幂等 / 按标签筛选与列表联动 / 摘他人 403 / 不可见 404。
package ticket

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestB102_Tags(t *testing.T) {
	ctx := context.Background()
	svc, aid, bid, vid, _ := setupTicket2a(t)
	oid := childOrgID(t, "b102")
	tk := newTicketHelper(t, svc, aid, oid, "B10-2 标签三向")
	tk2 := newTicketHelper(t, svc, aid, oid, "B10-2 无标签对照单")

	// bid 经抄送获得可见性（B10-1 特例支）——共享标签的观察者
	_, err := svc.AddCC(ctx, &model.AddTicketCCRequest{TicketID: tk.ID, UserIDs: []int64{bid}}, aid)
	require.NoError(t, err)

	t.Run("挂载/清洗/幂等", func(t *testing.T) {
		rows, err := svc.AddTags(ctx, &model.AddTicketTagsRequest{TicketID: tk.ID, Tags: []string{"urgent", " urgent ", "", "urgent", "network"}}, aid)
		require.NoError(t, err)
		var tags []string
		for _, r := range rows {
			tags = append(tags, r.Tag)
		}
		assert.ElementsMatch(t, []string{"urgent", "network"}, tags, "trim+去重+空滤后入库")

		// 全空白/全重复 → 400
		_, err = svc.AddTags(ctx, &model.AddTicketTagsRequest{TicketID: tk.ID, Tags: []string{"  ", "  "}}, aid)
		requireErrCode(t, err, errcode.ErrInvalidParams.Code)

		// 重复挂载幂等（单行）
		rows, err = svc.AddTags(ctx, &model.AddTicketTagsRequest{TicketID: tk.ID, Tags: []string{"urgent"}}, aid)
		require.NoError(t, err)
		assert.Len(t, rows, 2, "重复挂载不增行")
	})

	t.Run("跨用户共享可见（拍板点 6）", func(t *testing.T) {
		rows, err := svc.ListTags(ctx, tk.ID, bid)
		require.NoError(t, err)
		assert.Len(t, rows, 2, "被抄送人（非挂载人）可见共享标签")
		assert.NotEmpty(t, rows[0].CreatedByName, "挂载人留痕富化")
	})

	t.Run("按标签筛选与列表联动（可见性随工单 L2）", func(t *testing.T) {
		// aid 视角：两张单都可见，tag=urgent 只中 tk
		l, err := svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 50, Tag: "urgent"}, aid)
		require.NoError(t, err)
		ids := make([]int64, 0, len(l.List))
		for _, row := range l.List {
			ids = append(ids, row.ID)
		}
		assert.Contains(t, ids, tk.ID)
		assert.NotContains(t, ids, tk2.ID)

		// bid 视角：仅经 cc 可见 tk——tag 筛选不旁路 L2（tk2 不可见）
		l2, err := svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 50, Tag: "urgent"}, bid)
		require.NoError(t, err)
		ids2 := make([]int64, 0, len(l2.List))
		for _, row := range l2.List {
			ids2 = append(ids2, row.ID)
		}
		assert.EqualValues(t, []int64{tk.ID}, ids2, "标签筛选叠加 L2 可见性")

		// 无标签命中 → 空
		l3, err := svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 50, Tag: "不存在标签"}, aid)
		require.NoError(t, err)
		assert.EqualValues(t, 0, l3.Total)
	})

	t.Run("摘他人挂的 → 403；自摘/属主摘 → OK 幂等", func(t *testing.T) {
		err := svc.RemoveTag(ctx, &model.RemoveTicketTagRequest{TicketID: tk.ID, Tag: "urgent"}, bid)
		requireErrCode(t, err, errcode.ErrNoPermission.Code)

		// bid 自挂自摘（读可见面）
		_, err = svc.AddTags(ctx, &model.AddTicketTagsRequest{TicketID: tk.ID, Tags: []string{"from-b"}}, bid)
		require.NoError(t, err)
		require.NoError(t, svc.RemoveTag(ctx, &model.RemoveTicketTagRequest{TicketID: tk.ID, Tag: "from-b"}, bid))

		// 属主（update 权）摘他人挂的
		require.NoError(t, svc.RemoveTag(ctx, &model.RemoveTicketTagRequest{TicketID: tk.ID, Tag: "urgent"}, aid))
		require.NoError(t, svc.RemoveTag(ctx, &model.RemoveTicketTagRequest{TicketID: tk.ID, Tag: "urgent"}, aid), "再摘幂等")
	})

	t.Run("不可见用户 → 404 同形", func(t *testing.T) {
		_, err := svc.ListTags(ctx, tk.ID, vid)
		requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
		_, err = svc.AddTags(ctx, &model.AddTicketTagsRequest{TicketID: tk.ID, Tags: []string{"x"}}, vid)
		requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
	})
}
