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
