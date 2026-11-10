//go:build integration

// RR-HY-02（2026-11-10，口径=所有者拍「与业界一致」）：停用或移除的选项只禁新选、
// 存量值放行。修前本文件以下子测红：dict 项停用 / 静态选项移除后存量单改任何字段
// 400「不在选项内」；lookup_dict 公式引用停用项的存量单改单 400；multi_select
// 保留旧元素时 400。新选停用值仍须拒（只禁新选，不放宽新值）。
package ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// hyType 建类型并配置字段（choice 选项字段 + 可选 formula 字段）。
func hyType(t *testing.T, svc *Service, kind string, options json.RawMessage, formula string) string {
	t.Helper()
	ctx := context.Background()
	code := "hytype_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "存量选项值"})
	require.NoError(t, err)
	fields := []model.TicketTypeFieldInput{
		{FieldKey: "choice", FieldLabel: "选择", FieldType: kind, FieldOptions: options, SortOrder: 1},
		{FieldKey: "note", FieldLabel: "备注", FieldType: "input", SortOrder: 2},
	}
	if formula != "" {
		meta, err := json.Marshal(map[string]string{"formula": formula})
		require.NoError(t, err)
		fields = append(fields, model.TicketTypeFieldInput{FieldKey: "label", FieldLabel: "名称", FieldType: "input", FieldMeta: meta, SortOrder: 3})
	}
	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: fields})
	require.NoError(t, err)
	return code
}

func hyDisable(t *testing.T, dictRef json.RawMessage, item string) {
	t.Helper()
	var arr []string
	require.NoError(t, json.Unmarshal(dictRef, &arr))
	_, err := testPool.Exec(context.Background(),
		`UPDATE dict_items SET enabled=false WHERE type_code=$1 AND code=$2`, arr[0][len("dict:"):], item)
	require.NoError(t, err)
}

func hyPatch(ctx context.Context, svc *Service, id, actor int64, body string) error {
	_, err := svc.Update(ctx, &model.UpdateTicketRequest{ID: id, CustomData: json.RawMessage(body)}, actor)
	return err
}

func TestLegacyOptionValues_OnlyNewSelectionsBlocked(t *testing.T) {
	svc, actor, _, _, roles := setupTicket2a(t)
	roles[actor] = []string{"admin"}
	RegisterExprDictLookup(testPool)
	ctx := context.Background()
	org := rootOrgID(t)

	newTicket := func(t *testing.T, code, data string) int64 {
		t.Helper()
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "存量选项值", OrgID: org,
			CustomData: json.RawMessage(data)}, actor)
		require.NoError(t, err)
		return tk.ID
	}

	t.Run("dict 单选：项停用后存量单改其他字段放行，新选停用项仍拒", func(t *testing.T) {
		ref := g14Dict(t, "a", "b") // g14Dict 另带一个停用项 code=disabled
		code := hyType(t, svc, "select", ref, "")
		id := newTicket(t, code, `{"choice":"a"}`)
		hyDisable(t, ref, "a")
		require.NoError(t, hyPatch(ctx, svc, id, actor, `{"note":"只改备注"}`), "存量值 a 应放行（修前 400）")
		require.NoError(t, hyPatch(ctx, svc, id, actor, `{"choice":"a","note":"原样回传"}`), "原样回传存量值应放行")
		require.NoError(t, hyPatch(ctx, svc, id, actor, `{"choice":"b"}`), "改为启用项放行")
		require.Error(t, hyPatch(ctx, svc, id, actor, `{"choice":"a"}`), "改走后再选回停用项=新选，须拒")
		require.Error(t, hyPatch(ctx, svc, id, actor, `{"choice":"disabled"}`), "新选停用项须拒")
	})

	t.Run("静态单选：选项移除后存量单改其他字段放行，新选已移除值仍拒", func(t *testing.T) {
		code := hyType(t, svc, "radio", json.RawMessage(`["a","b"]`), "")
		id := newTicket(t, code, `{"choice":"a"}`)
		_, err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: []model.TicketTypeFieldInput{
			{FieldKey: "choice", FieldLabel: "选择", FieldType: "radio", FieldOptions: json.RawMessage(`["b"]`), SortOrder: 1},
			{FieldKey: "note", FieldLabel: "备注", FieldType: "input", SortOrder: 2},
		}})
		require.NoError(t, err)
		require.NoError(t, hyPatch(ctx, svc, id, actor, `{"note":"只改备注"}`), "静态存量值 a 应放行（修前 400）")
		id2 := newTicket(t, code, `{"choice":"b"}`)
		require.Error(t, hyPatch(ctx, svc, id2, actor, `{"choice":"a"}`), "新选已移除的静态值须拒")
	})

	t.Run("multi_select：保留旧元素放行，新增停用元素仍拒", func(t *testing.T) {
		ref := g14Dict(t, "a", "b")
		code := hyType(t, svc, "multi_select", ref, "")
		id := newTicket(t, code, `{"choice":["a","b"]}`)
		hyDisable(t, ref, "a")
		require.NoError(t, hyPatch(ctx, svc, id, actor, `{"note":"只改备注"}`), "旧元素 a 应放行（修前 400）")
		require.NoError(t, hyPatch(ctx, svc, id, actor, `{"choice":["b","a"]}`), "仅重排旧元素放行")
		require.NoError(t, hyPatch(ctx, svc, id, actor, `{"choice":["b"]}`), "移除旧元素放行")
		require.Error(t, hyPatch(ctx, svc, id, actor, `{"choice":["b","a"]}`), "已移除后再加回停用元素=新选，须拒")
		require.Error(t, hyPatch(ctx, svc, id, actor, `{"choice":["b","disabled"]}`), "新增停用元素须拒")
	})

	t.Run("lookup_dict 公式引用停用项：存量单改单放行且名称仍可解析", func(t *testing.T) {
		ref := g14Dict(t, "a", "b")
		var arr []string
		require.NoError(t, json.Unmarshal(ref, &arr))
		code := hyType(t, svc, "select", ref, fmt.Sprintf(`lookup_dict(%q, choice)`, arr[0][len("dict:"):]))
		id := newTicket(t, code, `{"choice":"a"}`)
		hyDisable(t, ref, "a")
		require.NoError(t, hyPatch(ctx, svc, id, actor, `{"note":"只改备注"}`), "公式引用停用项的存量单应放行（修前 400）")
		got, err := svc.Get(ctx, id, actor)
		require.NoError(t, err)
		var cd map[string]any
		require.NoError(t, json.Unmarshal(got.CustomData, &cd))
		require.Equal(t, "a", cd["label"], "停用项名称仍可解析（g14Dict 的 label=code）")
	})

	t.Run("建单仍按启用项全量校验（新单无存量值）", func(t *testing.T) {
		ref := g14Dict(t, "a", "b")
		code := hyType(t, svc, "select", ref, "")
		hyDisable(t, ref, "a")
		_, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "新单", OrgID: org,
			CustomData: json.RawMessage(`{"choice":"a"}`)}, actor)
		require.Error(t, err, "新建单选停用项须拒")
	})
}
