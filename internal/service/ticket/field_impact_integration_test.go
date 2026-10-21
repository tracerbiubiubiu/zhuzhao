//go:build integration

// B1-5 字段变更在途影响分析——测试先行（01 号 §3.5 随行件①）。
// 语义：全量替换前，管理员可见每个字段有多少在途工单正在使用——被删字段的
// 在途使用数即"变更代价"提示（软告警不阻断——全量替换语义不变）。
package ticket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func TestFieldImpact_Analysis(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	roles[aid] = []string{"admin"}
	ctx := context.Background()
	oid := childOrgID(t, "fia")

	code := "fia" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "影响分析"})
	require.NoError(t, err)

	t.Run("FieldImpact——每字段在途使用计数", func(t *testing.T) {
		// 建两个字段 + 三张在途单（两张用了 host_ip，一张用了 note）
		err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code,
			Fields: []model.TicketTypeFieldInput{
				{FieldKey: "host_ip", FieldLabel: "IP", FieldType: "input", Required: false, SortOrder: 1},
				{FieldKey: "note", FieldLabel: "备注", FieldType: "textarea", SortOrder: 2},
			}})
		require.NoError(t, err)

		for _, cd := range []string{
			`{"host_ip":"1.1.1.1","note":"a"}`,
			`{"host_ip":"2.2.2.2"}`,
			`{"note":"c"}`,
		} {
			_, err := svc.Create(ctx, &model.CreateTicketRequest{
				TypeCode: code, Title: "影响", Priority: 2, OrgID: oid,
				CustomData: json.RawMessage(cd),
			}, aid)
			require.NoError(t, err)
		}

		impact, err := svc.FieldImpact(ctx, code)
		require.NoError(t, err)
		assert.Equal(t, 2, impact["host_ip"], "host_ip 在途使用=2（两单）")
		assert.Equal(t, 2, impact["note"], "note 在途使用=2（两张单有 note）")
	})

	t.Run("终态单不计入（closed 排除）", func(t *testing.T) {
		// 关一张单 → 该单字段不再计为在途
		var lastID int64
		err := testPool.QueryRow(ctx,
			`SELECT id FROM tickets WHERE type_code = $1 ORDER BY id DESC LIMIT 1`, code).Scan(&lastID)
		require.NoError(t, err)
		if lastID > 0 {
			err := svc.Close(ctx, &model.CloseTicketRequest{ID: lastID, Comment: "测试关闭"}, aid)
			require.NoError(t, err)
			impact, err := svc.FieldImpact(ctx, code)
			require.NoError(t, err)
			// 关闭后 host_ip/note 计数减 1（取决于关闭的是哪张——这里只验证总数下降）
			total := 0
			for _, v := range impact {
				total += v
			}
			assert.Less(t, total, 4, "关闭一张后在途总使用 < 4（原 4）")
		}
	})
}
