//go:build integration

// B1-3 formula 计算字段——测试先行（先红后绿，01 号 §3.3①/§8 验收）。
// 核心断言：服务端求值 SSOT——formula 字段提交时一律重算覆盖（篡改必被覆盖）；
// formula 字段引用缺失 → 400 fail-fast。
package ticket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestFormula_Fields(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	ctx := context.Background()
	oid := childOrgID(t, "fm")

	code := "fm" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "公式类型"})
	require.NoError(t, err)

	t.Run("formula 字段保存+提交重算覆盖（篡改被覆盖）", func(t *testing.T) {
		// price*qty=total（篡改 total=99999 → 服务端覆盖为 12）
		err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code,
			Fields: []model.TicketTypeFieldInput{
				{FieldKey: "price", FieldLabel: "单价", FieldType: "number", Required: true, SortOrder: 1},
				{FieldKey: "qty", FieldLabel: "数量", FieldType: "number", Required: true, SortOrder: 2},
				{FieldKey: "total", FieldLabel: "合计", FieldType: "number", Required: false, SortOrder: 3,
					FieldMeta: json.RawMessage(`{"formula":"price * qty"}`)},
			}})
		require.NoError(t, err)

		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "公式单", Priority: 2, OrgID: oid,
			CustomData: json.RawMessage(`{"price":3,"qty":4,"total":99999}`), // 篡改
		}, aid)
		require.NoError(t, err)
		assert.JSONEq(t, `{"price":3,"qty":4,"total":12}`, string(tk.CustomData), "formula 重算覆盖（篡改被清除）")
	})

	t.Run("Update patch 联动重算", func(t *testing.T) {
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "公式编辑", Priority: 2, OrgID: oid,
			CustomData: json.RawMessage(`{"price":10,"qty":5,"total":50}`),
		}, aid)
		require.NoError(t, err)

		got, err := svc.Update(ctx, &model.UpdateTicketRequest{
			ID: tk.ID, CustomData: json.RawMessage(`{"price":20}`),
		}, aid)
		require.NoError(t, err)
		var cd map[string]any
		require.NoError(t, json.Unmarshal(got.CustomData, &cd))
		assert.Equal(t, float64(100), cd["total"], "patch price → total 联动重算")
	})

	t.Run("formula 非法类型拒保存", func(t *testing.T) {
		err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code,
			Fields: []model.TicketTypeFieldInput{
				{FieldKey: "d", FieldLabel: "日期", FieldType: "date", SortOrder: 1,
					FieldMeta: json.RawMessage(`{"formula":"now()"}`)},
			}})
		e, ok := err.(*errcode.Error)
		require.True(t, ok)
		assert.Equal(t, errcode.ErrInvalidParams.Code, e.Code, "formula 仅 number/input 型")
	})

	t.Run("formula 语法错误拒保存", func(t *testing.T) {
		err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code,
			Fields: []model.TicketTypeFieldInput{
				{FieldKey: "n", FieldLabel: "数", FieldType: "number", SortOrder: 1},
				{FieldKey: "x", FieldLabel: "计", FieldType: "number", SortOrder: 2,
					FieldMeta: json.RawMessage(`{"formula":"n * * 2"}`)}, // 语法错误
			}})
		e, ok := err.(*errcode.Error)
		require.True(t, ok)
		assert.Equal(t, errcode.ErrInvalidParams.Code, e.Code, "保存时即拒语法错误")
	})
}
