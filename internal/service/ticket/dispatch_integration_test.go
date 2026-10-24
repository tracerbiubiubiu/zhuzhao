//go:build integration

// S3a B3-2 自动分派集成测试（03 号 §8——A.7 验收承重）：
// 规则配置 → 命中 → 自动分派全链 / keyword LIKE / priority 首个命中 /
// 无命中保持 open + dispatch_missed / 决议③ target_user 属 org 写侧校验 /
// 决议⑤ 失败不回滚创建。
package ticket

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestB32_Dispatch(t *testing.T) {
	ctx := context.Background()
	svc, aid, bid, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	oid := childOrgID(t, "b32")
	code := "b32_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "分派批"})
	require.NoError(t, err)

	// bid/aid 加入 oid 组织（决议③ 校验需要——priority 子测分派 aid）
	for _, uid := range []int64{bid, aid} {
		_, err = testPool.Exec(ctx, `INSERT INTO user_orgs (user_id, org_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, uid, oid)
		require.NoError(t, err)
	}

	// DispatchService 挂到 svc（真实装配同形）
	disp := NewDispatchService(repository.NewAssignmentRuleRepo(testPool), svc, nil)
	svc.SetDispatcher(disp)

	t.Run("决议③：target_user 不属 target_org → 400", func(t *testing.T) {
		var rootOrg int64
		require.NoError(t, testPool.QueryRow(ctx, `SELECT id FROM organizations WHERE code='root' LIMIT 1`).Scan(&rootOrg))
		_, err := disp.CreateRule(ctx, &model.CreateAssignmentRuleRequest{
			TypeCode: code, TargetOrg: rootOrg, TargetUser: &bid, // bid 不在 root
		}, aid)
		requireErrCode(t, err, errcode.ErrInvalidParams.Code)
		assert.Contains(t, err.Error(), "不属于")
	})

	t.Run("规则配置 → 命中 → 自动分派全链（keyword LIKE）", func(t *testing.T) {
		_, err := disp.CreateRule(ctx, &model.CreateAssignmentRuleRequest{
			TypeCode: code, Priority: 100, Keyword: "数据库故障",
			TargetOrg: oid, TargetUser: &bid,
		}, aid)
		require.NoError(t, err)

		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "【数据库故障】主库连接池耗尽", Priority: 1, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		got, err := svc.Get(ctx, tk.ID, aid)
		require.NoError(t, err)
		require.NotNil(t, got.AssignedTo, "命中 keyword → 自动分派到 bid")
		assert.EqualValues(t, bid, *got.AssignedTo)
		assert.Equal(t, "assigned", got.Status, "走 Assign 通道 → assigned 态")
	})

	t.Run("priority 首个命中（低值优先）", func(t *testing.T) {
		// 更高优先级（值更小）规则：keyword 网络 → 分派 aid
		_, err := disp.CreateRule(ctx, &model.CreateAssignmentRuleRequest{
			TypeCode: code, Priority: 50, Keyword: "网络",
			TargetOrg: oid, TargetUser: &aid,
		}, aid)
		require.NoError(t, err)
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "网络抖动 + 数据库故障双命中", Priority: 2, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		got, err := svc.Get(ctx, tk.ID, aid)
		require.NoError(t, err)
		require.NotNil(t, got.AssignedTo)
		assert.EqualValues(t, aid, *got.AssignedTo, "priority 50 先于 100 命中")
	})

	t.Run("无命中 → 保持 open（决议④）", func(t *testing.T) {
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "无人认领的普通单", Priority: 3, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		got, err := svc.Get(ctx, tk.ID, aid)
		require.NoError(t, err)
		assert.Nil(t, got.AssignedTo, "无命中保持 open 不分派")
		assert.Equal(t, "open", got.Status)
	})

	t.Run("启停：disabled 规则不参与匹配", func(t *testing.T) {
		rule, err := disp.CreateRule(ctx, &model.CreateAssignmentRuleRequest{
			TypeCode: code, Priority: 10, Keyword: "停用测试",
			TargetOrg: oid, TargetUser: &aid, Enabled: boolPtr(false),
		}, aid)
		require.NoError(t, err)
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "停用测试单", Priority: 3, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		got, err := svc.Get(ctx, tk.ID, aid)
		require.NoError(t, err)
		assert.Nil(t, got.AssignedTo, "disabled 规则跳过")
		require.NoError(t, disp.UpdateRuleEnabled(ctx, rule.ID, true))
		tk2, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "停用测试单（启用后）", Priority: 3, OrgID: oid,
		}, aid)
		require.NoError(t, err)
		got2, err := svc.Get(ctx, tk2.ID, aid)
		require.NoError(t, err)
		require.NotNil(t, got2.AssignedTo, "启用后命中")
	})

	t.Run("管理面：列表/删除", func(t *testing.T) {
		rules, err := disp.ListRules(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, rules)
		last := rules[len(rules)-1]
		require.NoError(t, disp.DeleteRule(ctx, last.ID))
		err = disp.DeleteRule(ctx, last.ID)
		require.Error(t, err, "再删=404")
	})
}
