//go:build integration

package ticket

// IW3/BK-18 集成测试：类型/字段/模板管理闭环 + 创建时 schema 校验（G2）。
// SSOT: docs/phase2/00 §9 BK-18、docs/phase3/12-frontend §2。

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// setupTicketAdmin IW3 测试环境：svc + admin 用户 + org 取值闭包（root）
func setupTicketAdmin(t *testing.T) (*Service, int64, func() int64) {
	svc, admin, _, _, roles := setupTicket2a(t)
	roles[admin] = []string{"admin"} // R7 同款：显式注入 admin 角色
	orgID := func() int64 {
		var id int64
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT id FROM organizations WHERE code='root' LIMIT 1`).Scan(&id))
		return id
	}
	return svc, admin, orgID
}

func fmtInt(v int64) string            { return strconv.FormatInt(v, 10) }
func rawJSON(s string) json.RawMessage { return json.RawMessage(s) }
func boolPtr(b bool) *bool             { return &b }
func intPtr(i int) *int                { return &i }
func codesOf(list []*model.TicketType) []string {
	out := make([]string, 0, len(list))
	for _, t := range list {
		out = append(out, t.Code)
	}
	return out
}

func TestBK18_TypeCRUDAndValidation(t *testing.T) {
	svc, admin, envOrgID := setupTicketAdmin(t)
	ctx := context.Background()
	code := "bk18_t_" + uniqueSuffix()

	// ① 创建（缺省状态图）
	t1, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "BK18 类型"})
	require.NoError(t, err)
	assert.Contains(t, string(t1.States), "open")
	assert.True(t, t1.IsActive)

	// ② 重复 code → 409
	_, err = svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "重复"})
	requireErrCode(t, err, errcode.ErrConflict.Code)

	// ③ 字段集校验：重复 key / select 无选项 / 坏正则 → 400
	err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{
		{FieldKey: "env", FieldLabel: "环境", FieldType: "select", Required: true, FieldOptions: rawJSON(`[]`)},
		{FieldKey: "env", FieldLabel: "重复", FieldType: "input"},
	}})
	require.Error(t, err, "重复 field_key 应被拒")
	err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{
		{FieldKey: "env", FieldLabel: "环境", FieldType: "select", Required: true},
	}})
	require.Error(t, err, "select 无选项应被拒")
	err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{
		{FieldKey: "biz_no", FieldLabel: "单号", FieldType: "input", ValidateRegex: "["},
	}})
	require.Error(t, err, "坏正则应被拒")

	// ④ 合法字段集 → 替换成功 + has_custom_fields 同步
	err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{
		{FieldKey: "env", FieldLabel: "环境", FieldType: "select", Required: true,
			FieldOptions: rawJSON(`[{"label":"生产","value":"prod"},{"label":"测试","value":"test"}]`)},
		{FieldKey: "biz_no", FieldLabel: "业务单号", FieldType: "input", Required: true, ValidateRegex: "^BK[0-9]{4}$", SortOrder: 20},
	}})
	require.NoError(t, err)
	fields, err := svc.ListTicketTypeFieldsAdmin(ctx, code)
	require.NoError(t, err)
	assert.Len(t, fields, 2)
	assert.Equal(t, "^BK[0-9]{4}$", fields[1].ValidateRegex)

	// ⑤ 创建校验（G2）：缺必填 / 选项越界 / 正则不匹配 / 合法
	_, err = svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: code, Title: "缺必填", OrgID: envOrgID(), CustomData: rawJSON(`{}`),
	}, admin)
	require.Error(t, err, "缺必填字段应 400")
	_, err = svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: code, Title: "选项越界", OrgID: envOrgID(),
		CustomData: rawJSON(`{"env":"dev","biz_no":"BK1234"}`),
	}, admin)
	require.Error(t, err, "select 越界应 400")
	_, err = svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: code, Title: "正则不匹配", OrgID: envOrgID(),
		CustomData: rawJSON(`{"env":"prod","biz_no":"XX1"}`),
	}, admin)
	require.Error(t, err, "regex 不匹配应 400")
	tk, err := svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: code, Title: "合法创建", OrgID: envOrgID(),
		CustomData: rawJSON(`{"env":"prod","biz_no":"BK1234"}`),
	}, admin)
	require.NoError(t, err, "合法 custom_data 应通过")

	// ⑥ 有工单禁删 → 409
	err = svc.DeleteTicketType(ctx, code)
	requireErrCode(t, err, errcode.ErrConflict.Code)

	// ⑦ 停用 → 创建 90003；admin 全量列表含停用；普通列表不含
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{IsActive: boolPtr(false)})
	require.NoError(t, err)
	_, err = svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "停用后建单", OrgID: envOrgID()}, admin)
	requireErrCode(t, err, errcode.ErrTicketTypeNotFound.Code)
	adminList, err := svc.ListTicketTypesFor(ctx, admin, true)
	require.NoError(t, err)
	assert.Contains(t, codesOf(adminList), code)
	normalList, err := svc.ListTicketTypesFor(ctx, admin, false)
	require.NoError(t, err)
	assert.NotContains(t, codesOf(normalList), code)

	// ⑧ 无工单类型可删（换一个新建即删）
	t2, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code + "_del", Name: "待删"})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteTicketType(ctx, t2.Code))
	assert.NotZero(t, t2.ID)
	_ = tk
}

func TestBK18_TemplateCRUD(t *testing.T) {
	svc, admin, _ := setupTicketAdmin(t)
	ctx := context.Background()
	code := "bk18_tpl_" + uniqueSuffix()

	// 创建（org=root:1，org_path 服务端解析）
	tpl, err := svc.CreateTicketTemplate(ctx, &model.CreateTicketTemplateRequest{
		Code: code, Name: "BK18 模板", TypeCode: "incident", DefaultPriority: 2,
		OrgID: 1,
	}, admin)
	require.NoError(t, err)
	// 隔离债治理：模板残留会破坏 TestMeta_Templates_Empty 的空列表断言（-count=2 / 跨 run）
	t.Cleanup(func() {
		if _, err := testPool.Exec(ctx, `DELETE FROM ticket_templates WHERE code = $1`, code); err != nil {
			t.Logf("cleanup: delete template %s: %v", code, err)
		}
	})
	assert.Equal(t, "root", tpl.OrgPath, "root 模板 org_path 应为 root")

	// 重复 code → 409
	_, err = svc.CreateTicketTemplate(ctx, &model.CreateTicketTemplateRequest{
		Code: code, Name: "重复", TypeCode: "incident", OrgID: 1,
	}, admin)
	requireErrCode(t, err, errcode.ErrConflict.Code)

	// 模板默认优先级同样受 1–4 刻度约束
	_, err = svc.CreateTicketTemplate(ctx, &model.CreateTicketTemplateRequest{
		Code: code + "_bad", Name: "越界模板", TypeCode: "incident", DefaultPriority: 9, OrgID: 1,
	}, admin)
	require.Error(t, err, "模板 default_priority=9 应 400")

	// 更新（patch）
	updated, err := svc.UpdateTicketTemplate(ctx, code, &model.UpdateTicketTemplateRequest{
		Name: strPtr("BK18 模板 v2"), DefaultPriority: intPtr(1),
	})
	require.NoError(t, err)
	assert.Equal(t, "BK18 模板 v2", updated.Name)
	assert.Equal(t, 1, updated.DefaultPriority)

	// 删除 → 读回 404 语义
	require.NoError(t, svc.DeleteTicketTemplate(ctx, code))
	_, err = svc.GetTicketTemplate(ctx, code)
	require.Error(t, err)
}

// ---------- IW1 补：priority 刻度收敛（自由填 → 1–4 枚举） ----------

func TestPriorityScaleValidation(t *testing.T) {
	svc, admin, envOrgID := setupTicketAdmin(t)
	ctx := context.Background()

	// 越界：0 之外任意非 1–4 → 400
	_, err := svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: "incident", Title: "越界 9", OrgID: envOrgID(), Priority: 9}, admin)
	require.Error(t, err, "priority=9 应 400")
	_, err = svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: "incident", Title: "越界 -3", OrgID: envOrgID(), Priority: -3}, admin)
	require.Error(t, err, "priority=-3 应 400")

	// 0 = 缺省 → 归一为 3
	tk, err := svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: "incident", Title: "缺省优先级", OrgID: envOrgID()}, admin)
	require.NoError(t, err)
	assert.EqualValues(t, 3, tk.Priority)

	// 更新越界 → 400；合法 → 生效
	_, err = svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Priority: intPtr2(7)}, admin)
	require.Error(t, err, "更新 priority=7 应 400")
	up, err := svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Priority: intPtr2(1)}, admin)
	require.NoError(t, err)
	assert.EqualValues(t, 1, up.Priority)
}

func intPtr2(i int) *int { return &i }
