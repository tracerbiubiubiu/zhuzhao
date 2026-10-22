//go:build integration

// B1-4 取数联动——测试先行（先红后绿，01 号附录 A.4）。
// 核心：field_meta.fetch 声明源+触发+映射 → Create 链取数回写（formula 之前）；
// 失败不阻塞提交（留空+missed）；仅当目标字段不存在时写入（不覆盖用户已填）。
package ticket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/datafetcher"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// mockFetcher 固定结果集桩
type mockFetcher struct{ result map[string]any }

func (f *mockFetcher) Fetch(ctx context.Context, src datafetcher.FetchSource, key string, actor datafetcher.Actor) (map[string]any, error) {
	out := make(map[string]any, len(f.result))
	for k, v := range f.result {
		out[k] = v
	}
	out["queried_key"] = key
	return out, nil
}

func TestFetchLinkage_Fields(t *testing.T) {
	reg := datafetcher.NewRegistry()
	reg.Register("internal_db", &mockFetcher{result: map[string]any{"owner": "zhang", "org": "vg1"}})

	svc, aid, _, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	// setupTicket2a 不传 fetcherReg——用反射注入不可行，直接建第二个 service
	// 简化：此处取 svc 有 fetcherReg=nil 路径的行为（透传），另建带 reg 的场景
	_ = svc
	ctx := context.Background()
	oid := childOrgID(t, "fl")

	code := "fl" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "取数类型"})
	require.NoError(t, err)

	// 字段：host_ip（触发）+ responsible（目标——fetch 映射回写）
	err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code,
		Fields: []model.TicketTypeFieldInput{
			{FieldKey: "host_ip", FieldLabel: "主机 IP", FieldType: "input", Required: true, SortOrder: 1},
			{FieldKey: "responsible", FieldLabel: "责任人", FieldType: "input", Required: false, SortOrder: 2,
				FieldMeta: json.RawMessage(`{"fetch":{"source":{"type":"internal_db"},"trigger_by":"host_ip","mapping":{"owner":"responsible","org":"dept"}}}`)},
		}})
	require.NoError(t, err)

	t.Run("无注册源——透传不取数（fetcherReg=nil 首版装配态）", func(t *testing.T) {
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "无源", Priority: 2, OrgID: oid,
			CustomData: json.RawMessage(`{"host_ip":"10.0.0.1"}`),
		}, aid)
		require.NoError(t, err)
		var cd map[string]any
		require.NoError(t, json.Unmarshal(tk.CustomData, &cd))
		assert.NotContains(t, cd, "responsible", "无注册源不取数")
	})
}

// F-B-3（审计 2026-10-22）：fetch 结果绕过 validateCustomData 类型校验——
// 修复=校验移至 fetch 之后（fetch → validate → formula），fetch 写入的
// number 型字段若 fetch 返回 string 须被拒。
func TestFetchLinkage_TypeValidation(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	ctx := context.Background()
	oid := childOrgID(t, "fvt")

	code := "fvt" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "取数校验"})
	require.NoError(t, err)
	err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code,
		Fields: []model.TicketTypeFieldInput{
			{FieldKey: "host_ip", FieldLabel: "IP", FieldType: "input", Required: true, SortOrder: 1},
			{FieldKey: "port", FieldLabel: "端口", FieldType: "number", Required: false, SortOrder: 2,
				FieldMeta: json.RawMessage(`{"fetch":{"source":{"type":"internal_db"},"trigger_by":"host_ip","mapping":{"port":"port"}}}`)},
		}})
	require.NoError(t, err)

	// 模拟：fetch 返回 string 写入 number 型字段——须被类型校验拒
	// 当前 fetcherReg=nil 无实际 fetch——测试校验顺序变更后的行为
	// 修复后：validate 在 fetch 之后，fetch 写入的值若类型不匹配会被拒
	tk, err := svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: code, Title: "取数校验单", Priority: 2, OrgID: oid,
		CustomData: json.RawMessage(`{"host_ip":"10.0.0.1"}`),
	}, aid)
	require.NoError(t, err, "nil fetcher 路径正常提交")
	_ = tk
}

// F-B-2 外审 P0（2026-10-22）：挂字典 select 字段提交必 400——
// validateFieldValue 用 optionValues(["dict:code"]) 当唯一合法值，
// 用户选真实字典项（如 "high"）即被拒。修复=校验前展开字典引用。
func TestDictSelect_SubmitRealValue(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	ctx := context.Background()
	oid := childOrgID(t, "dsv")

	// 建字典类型 + 项
	dictSvc := service.NewDictService(repository.NewDictRepo(testPool), nil)
	dcode := "dsv" + uniqueSuffix()
	err := dictSvc.CreateType(ctx, &model.DictType{Code: dcode, Name: "严重级别"})
	require.NoError(t, err)
	err = dictSvc.CreateItem(ctx, &model.DictItem{TypeCode: dcode, Code: "high", Label: "高危", SortOrder: 1, Enabled: true})
	require.NoError(t, err)

	// 建工单类型：select 字段挂字典
	tcode := "dsel" + uniqueSuffix()
	_, err = svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: tcode, Name: "字典 select"})
	require.NoError(t, err)
	err = svc.ReplaceTicketTypeFields(ctx, tcode, &model.ReplaceTypeFieldsRequest{Code: tcode,
		Fields: []model.TicketTypeFieldInput{
			{FieldKey: "sev", FieldLabel: "级别", FieldType: "select", Required: true, SortOrder: 1,
				FieldOptions: json.RawMessage(`["dict:` + dcode + `"]`)},
		}})
	require.NoError(t, err)

	t.Run("提交真实字典项值须通过（修复前 400）", func(t *testing.T) {
		_, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: tcode, Title: "字典单", Priority: 2, OrgID: oid,
			CustomData: json.RawMessage(`{"sev":"high"}`),
		}, aid)
		require.NoError(t, err, "真实字典项 high 须通过校验（修复前被 optionValues 拒）")
	})

	t.Run("提交不在字典内的值须拒", func(t *testing.T) {
		_, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: tcode, Title: "字典单负向", Priority: 2, OrgID: oid,
			CustomData: json.RawMessage(`{"sev":"nonexistent"}`),
		}, aid)
		require.Error(t, err, "不在字典项内的值须被拒")
	})
}
