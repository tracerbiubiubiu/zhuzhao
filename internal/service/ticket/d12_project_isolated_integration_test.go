//go:build integration

package ticket

// IW1/BK-13 D12 + 委托轴回归：project_isolated 强隔离下的可见性与操作。
// SSOT: docs/phase2/09-ticket.md §5.2.1、检查单 B1（2026-08-31 拍板）。
// 委托轴核心断言：强隔离下锚点消失，org admin/owner / ancestor owner 仍须
// 可见可操作（否则 L3 委托被 L2 404 拦截，D7–D9 失效）。

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// setupD12 环境：root > E(实体) > vg_a / vg_b（兄弟虚拟组）。
// vg_a：ma（建单成员，scope=assigned）、aa（admin，scope=group）；
// vg_b：mb（member）；E.owner_user_ids = [pOwner]（非 vg 成员，ancestor owner）。
func setupD12(t *testing.T) (svc *Service, eID, vgA, vgB, ma, aa, mb, pOwner int64) {
	t.Helper()
	svc, _, _, _, roles := setupTicket2a(t)
	ctx := context.Background()
	suffix := uniqueSuffix()

	var err error
	mkorg := func(code, name string, parent int64, path string, orgType int) int64 {
		var id int64
		require.NoError(t, testPool.QueryRow(ctx, `
			INSERT INTO organizations (code, name, parent_id, path, org_type, status, sort_order, is_system)
			VALUES ($1, $2, $3, $4::ltree, $5, 1, 75, false) RETURNING id`,
			code, name, parent, path, orgType).Scan(&id))
		softDeleteOrg(t, id)
		return id
	}
	eID = mkorg("e_d12_"+suffix, "D12 实体", 1, "root.e_d12_"+suffix, 3)
	vgA = mkorg("vga_d12_"+suffix, "D12 vgA", eID, "root.e_d12_"+suffix+".vga_d12_"+suffix, 4)
	vgB = mkorg("vgb_d12_"+suffix, "D12 vgB", eID, "root.e_d12_"+suffix+".vgb_d12_"+suffix, 4)

	mkuser := func(name string) int64 {
		var id int64
		require.NoError(t, testPool.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO users (username, password, employee_no, status) VALUES ('%s', 'hash', 'E%s', 1)
			RETURNING id`, name, name)).Scan(&id))
		roles[id] = []string{"operator"}
		return id
	}
	ma = mkuser("d12ma_" + suffix)
	aa = mkuser("d12aa_" + suffix)
	mb = mkuser("d12mb_" + suffix)
	pOwner = mkuser("d12pown_" + suffix)

	member := func(uid, org int64, role, scope string) {
		_, err = testPool.Exec(ctx, `
			INSERT INTO user_orgs (user_id, org_id, is_primary, org_member_role, ticket_scope)
			VALUES ($1, $2, false, $3, $4)`, uid, org, role, scope)
		require.NoError(t, err)
	}
	member(ma, vgA, "member", "assigned")
	member(aa, vgA, "admin", "group")
	member(mb, vgB, "member", "assigned")
	// ancestor owner 双轨：E.owner_user_ids + E 成员行（生产 SetOwners 行为）
	member(pOwner, eID, "owner", "assigned")
	_, err = testPool.Exec(ctx,
		`UPDATE organizations SET owner_user_ids = ARRAY[$1]::bigint[] WHERE id = $2`, pOwner, eID)
	require.NoError(t, err)
	return
}

// IW1/BK-13 D12：强隔离兄弟互不可见；委托轴保住 admin/owner 的可见与操作
func TestD12_ProjectIsolated(t *testing.T) {
	svc, eID, vgA, _, ma, aa, mb, pOwner := setupD12(t)
	ctx := context.Background()

	tk := newTicketHelper(t, svc, ma, vgA, "D12 强隔离工单")

	// 基线（透明读）：vg_b 成员 mb 可读（策略 B）
	_, err := svc.Get(ctx, tk.ID, mb)
	require.NoError(t, err)

	// 开启强隔离
	_, err = testPool.Exec(ctx,
		`UPDATE organizations SET ticket_visibility = 'project_isolated' WHERE id = $1`, eID)
	require.NoError(t, err)

	// D12：兄弟虚拟组成员 → 404，列表不可见
	_, err = svc.Get(ctx, tk.ID, mb)
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
	bList, err := svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 50}, mb)
	require.NoError(t, err)
	assert.EqualValues(t, 0, bList.Total, "强隔离下兄弟成员列表应为空")

	// 创建人不受影响（属主豁免）
	_, err = svc.Get(ctx, tk.ID, ma)
	require.NoError(t, err)

	// vg_a admin：本组可见（scope=group 子树）+ D7 委托操作
	_, err = svc.Get(ctx, tk.ID, aa)
	require.NoError(t, err)
	aList, err := svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 50}, aa)
	require.NoError(t, err)
	assert.EqualValues(t, 1, aList.Total, "vg admin 应经 scope 子树 + 委托轴可见本组工单")
	_, err = svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: strPtr("强隔离下 admin 改名")}, aa)
	require.NoError(t, err, "强隔离下 D7 委托不得失效")

	// ancestor owner（实体 owner_user_ids）：Get/List/Update 经 L2 委托轴全通
	_, err = svc.Get(ctx, tk.ID, pOwner)
	require.NoError(t, err, "强隔离下 ancestor owner 应经委托轴可见（BK-13 核心断言）")
	pList, err := svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 50}, pOwner)
	require.NoError(t, err)
	assert.EqualValues(t, 1, pList.Total, "ancestor owner 列表经委托轴 EXISTS 应可见")
	_, err = svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: strPtr("强隔离下 owner 改名")}, pOwner)
	require.NoError(t, err, "强隔离下 D9 委托不得失效")

	// 关闭强隔离：兄弟成员恢复可见（开关双向）
	_, err = testPool.Exec(ctx,
		`UPDATE organizations SET ticket_visibility = 'entity_transparent_read' WHERE id = $1`, eID)
	require.NoError(t, err)
	_, err = svc.Get(ctx, tk.ID, mb)
	require.NoError(t, err)
}

// IW1/BK-13 委托轴不越界：org admin 对「他组」工单仍不可见（org 不匹配，
// 委托轴以工单所属 org 为锚，D11 语义不受影响）
func TestD12_DelegationAxisStaysBounded(t *testing.T) {
	svc, eID, _, vgB, _, aa, mb, pOwner := setupD12(t)
	ctx := context.Background()

	_, err := testPool.Exec(ctx,
		`UPDATE organizations SET ticket_visibility = 'project_isolated' WHERE id = $1`, eID)
	require.NoError(t, err)

	// vg_b 成员在 vg_b 建单；vg_a admin（aa）对其应 404（org 不匹配）
	tkB := newTicketHelper(t, svc, mb, vgB, "vgB 强隔离工单")
	_, err = svc.Get(ctx, tkB.ID, aa)
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)

	// 实体 owner（ancestor）跨 vg 仍可见（子树语义）
	_, err = svc.Get(ctx, tkB.ID, pOwner)
	require.NoError(t, err)
}
