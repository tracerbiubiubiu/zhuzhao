//go:build integration

// B1-6 批2（函数注册表——端到端）：lookup_dict 经 AddFunction 注入 db 闭包后，
// formula 字段保存+提交求值全链（01 号 A.3 函数白名单实施形态）。
package ticket

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestB16_LookupDict_Formula(t *testing.T) {
	RegisterExprDictLookup(testPool) // 装配件同款注册（进程级——幂等覆盖）
	ctx := context.Background()
	svc, aid, _, _, _ := setupTicket2a(t)
	oid := childOrgID(t, "b16d")
	code := "b16d_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "字典函数批"})
	require.NoError(t, err)

	dcode := "b16dc_" + uniqueSuffix()
	_, err = testPool.Exec(ctx,
		`INSERT INTO dict_types (code, name) VALUES ($1, '函数字典')`, dcode)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = testPool.Exec(context.Background(), `DELETE FROM dict_types WHERE code=$1`, dcode) })
	for _, it := range []struct{ c, l string }{{"high", "高危"}, {"low", "低危"}} {
		_, err = testPool.Exec(ctx,
			`INSERT INTO dict_items (type_code, code, label, enabled) VALUES ($1, $2, $3, true)`, dcode, it.c, it.l)
		require.NoError(t, err)
	}

	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code,
		Fields: []model.TicketTypeFieldInput{
			{FieldKey: "lvl", FieldLabel: "级别码", FieldType: "input", Required: true, SortOrder: 1},
			{FieldKey: "lvl_name", FieldLabel: "级别名", FieldType: "input", SortOrder: 2,
				FieldMeta: json.RawMessage(`{"formula":"lookup_dict(\"` + dcode + `\", lvl)"}`)},
		}})
	require.NoError(t, err, "lookup_dict formula 保存（保存期防线识别已注册函数）")

	t.Run("命中：提交求值入 label", func(t *testing.T) {
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "b16d", Priority: 2, OrgID: oid,
			CustomData: json.RawMessage(`{"lvl":"high"}`)}, aid)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(tk.CustomData, &m))
		assert.Equal(t, "高危", m["lvl_name"])
	})
	t.Run("未命中：求值错误→提交 400", func(t *testing.T) {
		_, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "b16d", Priority: 2, OrgID: oid,
			CustomData: json.RawMessage(`{"lvl":"ghost"}`)}, aid)
		requireErrCode(t, err, errcode.ErrInvalidParams.Code)
	})
}

func TestB16_FormulaVisibilityAndOutputValidation(t *testing.T) {
	ctx := context.Background()
	svc, actor, _, _, _ := setupTicket2a(t)
	org := rootOrgID(t)
	code := "b16g2_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "公式提交"})
	require.NoError(t, err)
	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: []model.TicketTypeFieldInput{
		{FieldKey: "show", FieldLabel: "开关", FieldType: "number"},
		{FieldKey: "src", FieldLabel: "输入", FieldType: "input", Required: true, FieldMeta: json.RawMessage(`{"visible_when":"show == 1"}`)},
		{FieldKey: "computed", FieldLabel: "结果", FieldType: "input", Required: true, ValidateRegex: "^a+$",
			FieldMeta: json.RawMessage(`{"visible_when":"show == 1","formula":"src == 'empty' ? '' : src + src + src"}`)},
	}})
	require.NoError(t, err)
	create := func(data string) (*model.Ticket, error) {
		return svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "公式提交", OrgID: org, CustomData: json.RawMessage(data)}, actor)
	}
	t.Run("隐藏公式不重算也不写回", func(t *testing.T) {
		hidden, err := create(`{"show":0,"computed":"aaa"}`)
		require.NoError(t, err, "隐藏输入已被剔除，隐藏公式不应继续取值")
		require.JSONEq(t, `{"show":0}`, string(hidden.CustomData))
		visible, err := create(`{"show":1,"src":"a"}`)
		require.NoError(t, err)
		require.JSONEq(t, `{"show":1,"src":"a","computed":"aaa"}`, string(visible.CustomData))
		updated, err := svc.Update(ctx, &model.UpdateTicketRequest{ID: visible.ID, CustomData: json.RawMessage(`{"show":0}`)}, actor)
		require.NoError(t, err)
		require.JSONEq(t, `{"show":0}`, string(updated.CustomData))
	})
	t.Run("正则及总量检查应用于最终结果，失败不改工单", func(t *testing.T) {
		good, err := create(`{"show":1,"src":"a"}`)
		require.NoError(t, err)
		for _, tt := range []struct {
			src     string
			message string
		}{
			{"empty", "为必填"},
			{"b", "不匹配校验规则"},
			{strings.Repeat("a", 24<<10), "超过 64 KiB"},
		} {
			data, err := json.Marshal(map[string]any{"show": 1, "src": tt.src})
			require.NoError(t, err)
			_, err = create(string(data))
			requireErrCode(t, err, errcode.ErrInvalidParams.Code)
			require.ErrorContains(t, err, tt.message)
			_, err = svc.Update(ctx, &model.UpdateTicketRequest{ID: good.ID, CustomData: data}, actor)
			requireErrCode(t, err, errcode.ErrInvalidParams.Code)
			require.ErrorContains(t, err, tt.message)
		}
		stored, err := svc.Get(ctx, good.ID, actor)
		require.NoError(t, err)
		require.JSONEq(t, string(good.CustomData), string(stored.CustomData))
	})
}
