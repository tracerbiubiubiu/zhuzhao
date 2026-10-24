//go:build integration

// S10 B10-3 角标聚合集成测试：todo section 随分派/关闭联动（A.4 sections 契约）。
package ticket

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func TestB103_Badges(t *testing.T) {
	ctx := context.Background()
	svc, aid, bid, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"} // assign 需主管面（b65 同款）
	oid := childOrgID(t, "b103")
	bs := NewBadgeService(testPool)
	tk := newTicketHelper(t, svc, aid, oid, "B10-3 角标联动单")

	// 未分派：bid 待办 0
	sec := bs.Sections(ctx, bid)
	require.Contains(t, sec, "todo")
	assert.EqualValues(t, 0, sec["todo"])

	// 分派给 bid：待办 1（open 计入——非终态）
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &bid}, aid))
	sec = bs.Sections(ctx, bid)
	assert.EqualValues(t, 1, sec["todo"])

	// 取消分派（open 回退——默认图 assigned 无 closed 直转边）：回到 0
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: nil}, aid))
	sec = bs.Sections(ctx, bid)
	assert.EqualValues(t, 0, sec["todo"], "取消分派后不计待办")
}
