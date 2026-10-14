//go:build integration

package ticket

// W4 后端随批件集成测试（02 §2-W4 / 03 S9 / 02 §5.5 十六批 Med）：
// ① assignee=me 列表筛选（P1-c——工作台待办/已办卡数据源）
// ② 列表/详情姓名回填 created_by_name/assignee_name（处理人列裸 ID/N+1 消除；
//    real_name 优先、空回退 username、软删用户不回填）
// ③ Update 字段级 COALESCE（nil 字段不覆盖——两人改不同字段不再互相冲掉）

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func ticketIDsOf(tickets []*model.Ticket) []int64 {
	ids := make([]int64, 0, len(tickets))
	for _, t := range tickets {
		ids = append(ids, t.ID)
	}
	return ids
}

// W4-1（P1-c）：assignee=me 只命中处理人=当前用户的工单；取消分派后不再命中
func TestW4_AssigneeMeFilter(t *testing.T) {
	svc, aid, bid, _, roles := setupTicket2a(t)
	ctx := context.Background()

	// assign 动作=主管专属语义（资源级 authorize 拒 operator）——分派用 admin 越过；
	// a 断言全程 admin 视角（bypass L2，单变量验证 assignee 条件本身）；
	// b 保持 operator（无组织锚点，L2=仅见分派给自己的）验证 filter 与 L2 叠加。
	roles[aid] = []string{"admin"}

	root := rootOrgID(t)
	tkForA := newTicketHelper(t, svc, aid, root, "W4 待办-A")
	tkForB := newTicketHelper(t, svc, aid, root, "W4 待办-B")
	tkNone := newTicketHelper(t, svc, aid, root, "W4 未分派")
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tkForA.ID, AssignedTo: &aid}, aid))
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tkForB.ID, AssignedTo: &bid}, aid))

	// a 的「处理人=我」：只含分派给 a 的工单（分派给他人/未分派不命中）
	list, err := svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 100, AssigneeID: &aid}, aid)
	require.NoError(t, err)
	ids := ticketIDsOf(list.List)
	assert.Contains(t, ids, tkForA.ID, "分派给 a 的工单应命中")
	assert.NotContains(t, ids, tkForB.ID, "分派给 b 的工单不得命中")
	assert.NotContains(t, ids, tkNone.ID, "未分派工单不得命中")

	// b 对称（operator：L2「仅分派给我的」∩ assignee=bid）
	list, err = svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 100, AssigneeID: &bid}, bid)
	require.NoError(t, err)
	ids = ticketIDsOf(list.List)
	assert.Contains(t, ids, tkForB.ID)
	assert.NotContains(t, ids, tkForA.ID)

	// 取消分派（assigned→open）后退出「处理人=我」
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tkForA.ID}, aid))
	list, err = svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 100, AssigneeID: &aid}, aid)
	require.NoError(t, err)
	assert.NotContains(t, ticketIDsOf(list.List), tkForA.ID, "取消分派后不得再命中")

	// 不带 assignee 条件：行为不变（admin bypass L2 三张全可见）
	list, err = svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 100}, aid)
	require.NoError(t, err)
	ids = ticketIDsOf(list.List)
	assert.Contains(t, ids, tkForA.ID)
	assert.Contains(t, ids, tkForB.ID)
	assert.Contains(t, ids, tkNone.ID)
}

// W4-2（03 S9）：列表/详情姓名回填——real_name 优先、空回退 username、软删用户不回填
func TestW4_NameBackfill(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	ctx := context.Background()

	// 分派/全量列表断言用 admin 视角（同 W4-1：assign 为主管语义；bypass L2 使
	// 「分派给第三方用户」的工单对创建者可见）
	roles[aid] = []string{"admin"}

	// 专建带 real_name 的处理人（setupTicket2a 建的行 real_name 为空——回退 username 形）
	suffix := uniqueSuffix()
	var rid int64
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO users (username, password, employee_no, real_name, status) VALUES ($1, 'hash', $2, $3, 1) RETURNING id`,
		"w4_named_"+suffix, "E2W401_"+suffix, "张三"+suffix).Scan(&rid))
	// 创建人（aid）实际显示名：real_name 空 → username
	var aidName string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COALESCE(NULLIF(real_name, ''), username) FROM users WHERE id = $1`, aid).Scan(&aidName))
	// 创建人工号（2026-10-13 展示统一批：*_no 结构化消歧字段与姓名同批回填）
	var aidNo string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COALESCE(employee_no, '') FROM users WHERE id = $1`, aid).Scan(&aidNo))

	root := rootOrgID(t)
	tk := newTicketHelper(t, svc, aid, root, "W4 姓名回填")
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &rid}, aid))

	// 详情回填
	got, err := svc.Get(ctx, tk.ID, aid)
	require.NoError(t, err)
	assert.Equal(t, aidName, got.CreatedByName, "创建人姓名：real_name 空应回退 username")
	assert.Equal(t, aidNo, got.CreatedByNo, "创建人工号：结构化消歧字段（2026-10-13）")
	require.NotNil(t, got.AssigneeName)
	assert.Equal(t, "张三"+suffix, got.AssigneeName, "处理人姓名：real_name 优先")
	assert.Equal(t, "E2W401_"+suffix, got.AssigneeNo, "处理人工号：结构化消歧字段")

	// 列表回填
	list, err := svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 100}, aid)
	require.NoError(t, err)
	var row *model.Ticket
	for _, tk := range list.List {
		if tk.ID == got.ID {
			row = tk
			break
		}
	}
	require.NotNil(t, row, "列表应含目标工单")
	assert.Equal(t, aidName, row.CreatedByName)
	assert.Equal(t, aidNo, row.CreatedByNo)
	require.NotNil(t, row.AssigneeName)
	assert.Equal(t, "张三"+suffix, row.AssigneeName)
	assert.Equal(t, "E2W401_"+suffix, row.AssigneeNo)

	// 软删处理人 → 再读不回填（omitempty 隐去，前端降级显 ID）
	_, err = testPool.Exec(ctx, `UPDATE users SET deleted_at = NOW() WHERE id = $1`, rid)
	require.NoError(t, err)
	got, err = svc.Get(ctx, tk.ID, aid)
	require.NoError(t, err)
	assert.Empty(t, got.AssigneeName, "软删用户不回填姓名")
}

// W4-2b（2026-10-14 审查补钉）：无工号用户的回填分支——姓名照常回填、工号空串。
// 契约审计 B3 后四字段已统一值类型 + omitempty（空工号 → 字段在 JSON 隐去，
// 「有指派」语义由 AssignedTo 承载），本测试防该分支无断言回归。
func TestW4_NameBackfill_NoEmployeeNo(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	ctx := context.Background()
	roles[aid] = []string{"admin"}

	// 造「无工号创建人」：暂置种子账号 aid 工号为 NULL（defer 复原防串扰后续用例）
	var origNo *string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT employee_no FROM users WHERE id = $1`, aid).Scan(&origNo))
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(),
			`UPDATE users SET employee_no = $1 WHERE id = $2`, origNo, aid)
	})
	_, err := testPool.Exec(ctx, `UPDATE users SET employee_no = NULL WHERE id = $1`, aid)
	require.NoError(t, err)

	// 无工号处理人
	suffix := uniqueSuffix()
	var rid int64
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO users (username, password, employee_no, real_name, status) VALUES ($1, 'hash', NULL, $2, 1) RETURNING id`,
		"w4_nonum_"+suffix, "无工号处理人"+suffix).Scan(&rid))

	root := rootOrgID(t)
	tk := newTicketHelper(t, svc, aid, root, "W4 无工号回填")
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &rid}, aid))

	got, err := svc.Get(ctx, tk.ID, aid)
	require.NoError(t, err)
	assert.NotEmpty(t, got.CreatedByName, "无工号创建人姓名仍回填")
	assert.Empty(t, got.CreatedByNo, "无工号创建人：no 回填空串（JSON 端 omitempty 隐去）")
	require.NotNil(t, got.AssigneeName)
	assert.NotEmpty(t, got.AssigneeName, "无工号处理人姓名照常回填")
	assert.Empty(t, got.AssigneeNo, "无工号处理人：空串经 omitempty 在 JSON 隐去（与 created_by_no 同形态）")
}

// W4-3（十六批 Med）：Update 字段级 COALESCE——nil 字段不覆盖，显式空串仍生效
func TestW4_UpdateFieldLevelNoClobber(t *testing.T) {
	svc, aid, _, _, _ := setupTicket2a(t)
	ctx := context.Background()

	root := rootOrgID(t)
	tk := newTicketHelper(t, svc, aid, root, "W4 字段级更新")

	// 只改 title：description/priority 保持原值
	upd, err := svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: strPtr("W4 字段级更新-改标题")}, aid)
	require.NoError(t, err)
	assert.Equal(t, "W4 字段级更新-改标题", upd.Title)
	assert.Equal(t, tk.Description, upd.Description)
	assert.Equal(t, tk.Priority, upd.Priority)

	// 再只改 priority：上一步写入的 title 不被冲掉（等价两并发写各改一字段的终态语义）
	upd, err = svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Priority: intPtr(1)}, aid)
	require.NoError(t, err)
	assert.Equal(t, "W4 字段级更新-改标题", upd.Title, "未传字段不得被覆盖")
	assert.Equal(t, 1, upd.Priority)

	// 显式空串 description：B2-3 patch 语义（传空=清空）不回退
	empty := ""
	upd, err = svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Description: &empty}, aid)
	require.NoError(t, err)
	assert.Equal(t, "", upd.Description)
	assert.Equal(t, "W4 字段级更新-改标题", upd.Title)
}
