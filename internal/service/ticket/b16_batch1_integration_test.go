//go:build integration

// B1-6 批1（能力面扩展——后端 schema/G2 面，测试先行先红后绿；11 号 §1 B1-6 行）：
// radio（第 9 类型——与 select 同数据模型）/ rating（第 10——与 number 同模型）/
// file（第 11——object_key 数组 + required≥1 + MIME 白名单 + 建单落绑走既有
// attachment 链[01 号 §3.5 file×附件五点]）+ col_span/group/visible_when 元数据
// 校验 + 条件显隐 G2 skip（required 不校验、值剔除防绕过）。
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

// seedPendingObject 造一枚建单前悬挂对象（attachment_create + confirmed——
// bindAttachmentsTx 校验面：非本人/未确认/已绑定均拒；bucket=” 对齐
// setupTicket2a 零值 StorageConfig——绑定查询以 cfg.Bucket 为锚）。
func seedPendingObject(t *testing.T, uid int64, key, mime string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO file_objects (bucket, object_key, purpose, status, file_name, mime_type, created_by, confirmed_at)
		VALUES ('', $1, 'attachment_create', 'confirmed', 'f.png', $2, $3, NOW())`, key, mime, uid)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE object_key = $1`, key)
	})
}

func TestB16_NewTypes_G2(t *testing.T) {
	ctx := context.Background()
	svc, aid, _, _, _ := setupTicket2a(t)
	oid := childOrgID(t, "b16a")
	code := "b16t_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "三类型批"})
	require.NoError(t, err)

	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code,
		Fields: []model.TicketTypeFieldInput{
			{FieldKey: "env", FieldLabel: "环境", FieldType: "radio", SortOrder: 1, FieldOptions: json.RawMessage(`["prod","staging"]`)},
			{FieldKey: "score", FieldLabel: "评分", FieldType: "rating", SortOrder: 2},
			{FieldKey: "shot", FieldLabel: "截图", FieldType: "file", SortOrder: 3},
		}})
	require.NoError(t, err, "radio/rating/file 三类型保存")

	create := func(cd string) error {
		_, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "b16", Priority: 2, OrgID: oid,
			CustomData: json.RawMessage(cd)}, aid)
		return err
	}
	t.Run("radio：合法值过/越界拒（同 select 模型）", func(t *testing.T) {
		require.NoError(t, create(`{"env":"prod"}`))
		requireErrCode(t, create(`{"env":"dev"}`), errcode.ErrInvalidParams.Code)
	})
	t.Run("rating：数字过/字符串拒（同 number 模型）", func(t *testing.T) {
		require.NoError(t, create(`{"score":4.5}`))
		requireErrCode(t, create(`{"score":"high"}`), errcode.ErrInvalidParams.Code)
	})
	t.Run("file：字符串数组过/数字数组拒（无白名单时跳 MIME）", func(t *testing.T) {
		key := "tickets/2026/10/b16free_" + uniqueSuffix() + ".png"
		seedPendingObject(t, aid, key, "image/png")
		require.NoError(t, create(`{"shot":["`+key+`"]}`))
		requireErrCode(t, create(`{"shot":[1,2]}`), errcode.ErrInvalidParams.Code)
	})
	t.Run("radio 空选项拒（保存期）", func(t *testing.T) {
		_, err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code,
			Fields: []model.TicketTypeFieldInput{{FieldKey: "env", FieldLabel: "环境", FieldType: "radio", FieldOptions: json.RawMessage(`[]`)}}})
		requireErrCode(t, err, errcode.ErrInvalidParams.Code)
	})
}

func TestB16_File_BindAndMime(t *testing.T) {
	ctx := context.Background()
	svc, aid, _, _, _ := setupTicket2a(t)
	oid := childOrgID(t, "b16b")
	code := "b16f_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "file 落绑批"})
	require.NoError(t, err)
	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code,
		Fields: []model.TicketTypeFieldInput{
			{FieldKey: "shot", FieldLabel: "截图", FieldType: "file", Required: true, SortOrder: 1,
				FieldOptions: json.RawMessage(`["image/png"]`)},
		}})
	require.NoError(t, err)

	create := func(cd string) (int64, error) {
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "b16f", Priority: 2, OrgID: oid,
			CustomData: json.RawMessage(cd)}, aid)
		if err != nil {
			return 0, err
		}
		return tk.ID, nil
	}
	pngKey := "tickets/2026/10/b16m_" + uniqueSuffix() + ".png"
	seedPendingObject(t, aid, pngKey, "image/png")
	jpgKey := "tickets/2026/10/b16j_" + uniqueSuffix() + ".jpg"
	seedPendingObject(t, aid, jpgKey, "image/jpeg")

	t.Run("required：缺失/空数组拒", func(t *testing.T) {
		_, errC := create(`{}`)
		requireErrCode(t, errC, errcode.ErrInvalidParams.Code)
		_, errC = create(`{"shot":[]}`)
		requireErrCode(t, errC, errcode.ErrInvalidParams.Code)
	})
	t.Run("合法 png：提交过 + 建单落绑 ticket_attachments（file×附件五点①）", func(t *testing.T) {
		id, err := create(`{"shot":["` + pngKey + `"]}`)
		require.NoError(t, err)
		var n int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT count(*) FROM ticket_attachments ta JOIN file_objects fo ON fo.id = ta.file_object_id
			 WHERE ta.ticket_id = $1 AND fo.object_key = $2`, id, pngKey).Scan(&n))
		assert.Equal(t, 1, n, "file 字段 object_key 须走既有 attachment 落绑链")
	})
	t.Run("MIME 白名单：jpeg 越界拒（file_objects.mime_type 对照）", func(t *testing.T) {
		_, err := create(`{"shot":["` + jpgKey + `"]}`)
		requireErrCode(t, err, errcode.ErrInvalidParams.Code)
	})
	t.Run("伪造裸 key：悬挂校验拒（防伪造）", func(t *testing.T) {
		_, err := create(`{"shot":["tickets/2026/10/ghost_` + uniqueSuffix() + `.png"]}`)
		requireErrCode(t, err, errcode.ErrInvalidParams.Code)
	})
}

func TestB16_VisibleWhen(t *testing.T) {
	ctx := context.Background()
	svc, aid, _, _, _ := setupTicket2a(t)
	oid := childOrgID(t, "b16v")
	code := "b16v_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "显隐批"})
	require.NoError(t, err)
	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code,
		Fields: []model.TicketTypeFieldInput{
			{FieldKey: "a", FieldLabel: "开关", FieldType: "number", SortOrder: 1},
			{FieldKey: "b", FieldLabel: "条件字段", FieldType: "input", Required: true, SortOrder: 2,
				FieldMeta: json.RawMessage(`{"visible_when":"a == 1"}`)},
		}})
	require.NoError(t, err)

	get := func(cd string) map[string]any {
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "b16v", Priority: 2, OrgID: oid,
			CustomData: json.RawMessage(cd)}, aid)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(tk.CustomData, &m))
		return m
	}
	t.Run("隐藏：required 跳过 + 值不写入", func(t *testing.T) {
		m := get(`{"a":2,"b":"should-be-pruned"}`)
		assert.NotContains(t, m, "b", "隐藏字段值须剔除（防绕过）")
	})
	t.Run("可见：required 生效 + 值保留", func(t *testing.T) {
		m := get(`{"a":1,"b":"x"}`)
		assert.Equal(t, "x", m["b"])
	})
	t.Run("隐藏且未填：required 豁免过", func(t *testing.T) {
		m := get(`{"a":2}`)
		assert.NotContains(t, m, "b")
	})
}

func TestB16_MetaValidation(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := setupTicket2a(t)
	code := "b16m_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "meta 批"})
	require.NoError(t, err)
	save := func(meta string) error {
		_, err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code,
			Fields: []model.TicketTypeFieldInput{
				{FieldKey: "f1", FieldLabel: "一", FieldType: "input", SortOrder: 1, FieldMeta: json.RawMessage(meta)},
			}})
		return err
	}
	t.Run("col_span 合法/越界", func(t *testing.T) {
		require.NoError(t, save(`{"col_span":12,"group":"基本信息"}`))
		requireErrCode(t, save(`{"col_span":25}`), errcode.ErrInvalidParams.Code)
	})
	t.Run("visible_when 引用不存在字段拒", func(t *testing.T) {
		requireErrCode(t, save(`{"visible_when":"ghost == 1"}`), errcode.ErrInvalidParams.Code)
	})
}
