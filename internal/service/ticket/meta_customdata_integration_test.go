//go:build integration

package ticket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// ---------- 元数据接口（ListTypes / ListFields / Templates） ----------

// TestMeta_ListTicketTypes_Seeds 验证最小种子包含 incident + request 两种类型
// （流程差异化的前置——每新增类型只要 INSERT 就会出现在这里）
func TestMeta_ListTicketTypes_Seeds(t *testing.T) {
	repo := repository.NewTicketRepo(testPool)
	ctx := context.Background()

	types, err := repo.ListTicketTypes(ctx)
	require.NoError(t, err)
	found := map[string]*model.TicketType{}
	for _, tt := range types {
		found[tt.Code] = tt
	}

	assert.NotNilf(t, found["incident"], "incident 工单类型必须在 ListTicketTypes 中")
	assert.NotNilf(t, found["request"], "request 工单类型必须在 ListTicketTypes 中")
	assert.True(t, found["incident"].IsActive, "incident 应为启用")
	assert.True(t, found["request"].IsActive, "request 应为启用")
}

// TestMeta_ListTicketTypeFields_EmptySeed 现 2a 未补 incident/request 字段种子，
// 应返回空数组而不是报错。将来 ticket_type_fields 种子 INSERT 后该测试会失败，
// 届时更新断言为具体字段数即可（用来检测字段定义是否被加载）。
func TestMeta_ListTicketTypeFields_EmptySeed(t *testing.T) {
	repo := repository.NewTicketRepo(testPool)
	ctx := context.Background()

	incidentFields, err := repo.ListTicketTypeFields(ctx, "incident")
	require.NoError(t, err)
	assert.Emptyf(t, incidentFields, "2a 阶段未写 ticket_type_fields 种子，incident 字段应为空")

	requestFields, err := repo.ListTicketTypeFields(ctx, "request")
	require.NoError(t, err)
	assert.Emptyf(t, requestFields, "2a 阶段未写 ticket_type_fields 种子，request 字段应为空")

	// 不存在的类型也应返回空数组（FK 不存在 → 空，不报错）
	ghost, err := repo.ListTicketTypeFields(ctx, "type_not_exist_xyz")
	require.NoError(t, err)
	assert.Empty(t, ghost)
}

// TestMeta_Templates_Empty 查询空模板列表 + 不存在模板返回 ErrNotFound
func TestMeta_Templates_Empty(t *testing.T) {
	repo := repository.NewTicketRepo(testPool)
	ctx := context.Background()

	list, err := repo.ListTicketTemplates(ctx)
	require.NoError(t, err)
	assert.Emptyf(t, list, "2a 无模板种子，ListTicketTemplates 为空数组")

	_, err = repo.GetTicketTemplate(ctx, "template_never_exist")
	requireErrCode(t, err, errcode.ErrNotFound.Code)
}

// ---------- custom_data JSONB 往返持久化 ----------

// TestCustomData_PersistRoundTrip 验证 Create 时传入的复杂 custom_data
// 能原样从 Get/List 取回（JSONB 不能丢字段、不能改类型）
func TestCustomData_PersistRoundTrip(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	oid := childOrgID(t, "cd1")
	roles[aid] = []string{"admin"} // 用 admin 绕开 assigned scope 读回来的断言，直接测持久化
	ctx := context.Background()

	payload := []byte(`{
		"hardware_model": "MacBookPro18,3",
		"os_version": "macOS 15.0",
		"reproduce_steps": ["step1", "step2", "step3"],
		"attachments": [{"name":"log.txt","size":2048}],
		"crash": {
			"thread": 7,
			"signal": "SIGSEGV",
			"callstack": ["a","b","c"]
		},
		"impact_score": 95,
		"is_critical": true,
		"workaround": null
	}`)
	// 紧凑化，用于与 Get 回来的 compact bytes 比较
	var compactWant bytes.Buffer
	require.NoError(t, json.Compact(&compactWant, payload))

	tk, err := svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode:   "incident",
		Title:      "custom_data 往返",
		Priority:   2,
		OrgID:      oid,
		CustomData: json.RawMessage(payload),
	}, aid)
	require.NoError(t, err)
	require.NotZero(t, tk.ID)

	// 1) Get 回来 bytes 语义等价
	got, err := svc.Get(ctx, tk.ID, aid)
	require.NoError(t, err)
	var compactGot bytes.Buffer
	require.NotNil(t, got.CustomData, "CustomData 不应为 nil")
	require.NoError(t, json.Compact(&compactGot, got.CustomData))
	assert.JSONEqf(t, compactWant.String(), compactGot.String(),
		"Create 入 custom_data 与 Get 出 JSON 语义应等价 (bytes: want=%s got=%s)",
		compactWant.String(), compactGot.String())

	// 2) List 里的同一条也有相同 custom_data
	list, err := svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 100, TypeCode: "incident"}, aid)
	require.NoError(t, err)
	found := false
	for _, x := range list.List {
		if x.ID == tk.ID {
			found = true
			assert.JSONEqf(t, compactWant.String(), string(x.CustomData),
				"List 项的 custom_data 也应与 Create 输入等价")
			break
		}
	}
	assert.True(t, found, "List 应能找到刚创建的工单（admin bypass 不过滤）")
}

// TestCustomData_NilAndEmpty 验证 nil/empty 的 custom_data 不会炸、入库后返回有效 JSON
func TestCustomData_NilAndEmpty(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	oid := childOrgID(t, "cd2")
	roles[aid] = []string{"admin"}
	ctx := context.Background()

	// nil CustomData
	t1, err := svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: "incident", Title: "nil-cd", Priority: 3, OrgID: oid,
		CustomData: nil,
	}, aid)
	require.NoError(t, err)
	got1, err := svc.Get(ctx, t1.ID, aid)
	require.NoError(t, err)
	// 允许是 null 或 {}；DB 列 DEFAULT 为 '{}' 但 CreateTx 里显式传了 $10=nil 所以 PG 写入 null 不触发 DEFAULT
	// 总之要能通过 json.Valid：
	if len(got1.CustomData) > 0 {
		assert.Truef(t, json.Valid(got1.CustomData), "nil custom_data 读回来必须是合法 JSON，got=%s", string(got1.CustomData))
	}

	// 空对象 CustomData
	t2, err := svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: "incident", Title: "empty-cd", Priority: 3, OrgID: oid,
		CustomData: json.RawMessage(`{}`),
	}, aid)
	require.NoError(t, err)
	got2, err := svc.Get(ctx, t2.ID, aid)
	require.NoError(t, err)
	assert.JSONEqf(t, `{}`, string(got2.CustomData), "空 custom_data 读回来应为空对象")
}

// ---------- 流程差异化（真表，不同 type_code 对应不同状态机） ----------

// TestTypeDiversity_DBTransitionsDiffer 直接从 DB 取 incident 和 request
// 的 transitions，验证「同一 FromTicketType 对不同 type_code → 行为不同」。
// 这是「新增类型只要 INSERT 不用改 Go」在集成级的核心证明。
func TestTypeDiversity_DBTransitionsDiffer(t *testing.T) {
	repo := repository.NewTicketRepo(testPool)
	ctx := context.Background()

	incident, err := repo.GetTicketType(ctx, "incident")
	require.NoError(t, err)
	request, err := repo.GetTicketType(ctx, "request")
	require.NoError(t, err)

	smI, err := FromTicketType(incident)
	require.NoError(t, err)
	smR, err := FromTicketType(request)
	require.NoError(t, err)

	// incident transitions (DDL DEFAULT):
	//   open→[assigned,closed] assigned→[in_progress,open] in_progress→[pending_verify,rejected,closed]
	//   pending_verify→[closed,in_progress] closed→[open] rejected→[open]
	// request transitions (seedMinimal 显式):
	//   open→[assigned,closed] assigned→[in_progress,open,reassigned] in_progress→[pending_verify,closed]
	//   pending_verify→[closed,reassigned] closed→[open] reassigned→[assigned]
	// 差异：incident 有 rejected 无 reassigned；request 有 reassigned 无 rejected。

	// —— A. rejected→open：incident 合法 / request 非法（request 无 rejected 态）——
	assert.NoError(t, smI.AssertTransition("rejected", "open"),
		"incident.rejected → open 合法（rejected→[open]）")
	requireErrCodeInline(t, smR.AssertTransition("rejected", "open"), errcode.ErrTicketInvalidTransition.Code,
		"request 无 rejected，rejected→open 应非法")

	// —— B. reassigned→assigned：request 合法 / incident 非法（incident 无 reassigned 态）——
	requireErrCodeInline(t, smI.AssertTransition("reassigned", "assigned"), errcode.ErrTicketInvalidTransition.Code,
		"incident 无 reassigned，reassigned→assigned 应非法")
	assert.NoError(t, smR.AssertTransition("reassigned", "assigned"),
		"request.reassigned → assigned 合法（reassigned→[assigned]）")

	// —— C. in_progress→rejected：incident 合法 / request 非法（request 的 in_progress 无 rejected 目标）——
	assert.NoError(t, smI.AssertTransition("in_progress", "rejected"),
		"incident.in_progress → rejected 合法")
	requireErrCodeInline(t, smR.AssertTransition("in_progress", "rejected"), errcode.ErrTicketInvalidTransition.Code,
		"request.in_progress→rejected 非法（in_progress→[pending_verify,closed]）")

	// —— D. 两边都合法的 common 路径 ——
	for _, pair := range [][2]string{
		{"open", "assigned"}, {"open", "closed"},
		{"assigned", "in_progress"}, {"assigned", "open"},
		{"in_progress", "pending_verify"}, {"pending_verify", "closed"},
		{"closed", "open"},
	} {
		from, to := pair[0], pair[1]
		assert.NoErrorf(t, smI.AssertTransition(from, to), "incident 通用 %s→%s 应合法", from, to)
		assert.NoErrorf(t, smR.AssertTransition(from, to), "request 通用 %s→%s 应合法", from, to)
	}
}

// requireErrCodeInline 要求 err 为 errcode.Error 且 code == expect
func requireErrCodeInline(t *testing.T, err error, expect int, msgAndArgs ...any) {
	t.Helper()
	require.Error(t, err, msgAndArgs...)
	var ee *errcode.Error
	require.Truef(t, errors.As(err, &ee), "err 应为 errcode.Error：%T %v · %v", err, err, msgAndArgs)
	assert.Equalf(t, expect, ee.Code, "错误码不匹配：err=%v · %v", ee, msgAndArgs)
}
