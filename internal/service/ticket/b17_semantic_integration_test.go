//go:build integration

// B1-7 批2 语义收口（测试先行——先红后绿，矩阵=01 号 A.6）：锁死守卫
// （ReplaceTypeFields 按 key diff：硬锁=改 type/改 key/删除、软提示=options
// 收窄/required 加严、余自由）/ 禁链式+引用存在性（formula 仅引同列表数据
// 字段）/ 字典码存在性（三路F-B-8 收口）/ fetch 校验（trigger_by 存在且非
// formula、mapping 目标存在、触发图无环）/ formula 类型漂移提交期校验
// （三路F-B-3——重算后按 field_type 硬 400）。
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

func TestB17_LockGuard(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := setupTicket2a(t)
	code := "b17lock_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "锁死批"})
	require.NoError(t, err)

	base := func() []model.TicketTypeFieldInput {
		return []model.TicketTypeFieldInput{
			{FieldKey: "num", FieldLabel: "数量", FieldType: "number", SortOrder: 1},
			{FieldKey: "sel", FieldLabel: "选项", FieldType: "select", SortOrder: 2, FieldOptions: json.RawMessage(`["a","b"]`)},
			{FieldKey: "lab", FieldLabel: "标签", FieldType: "input", SortOrder: 3},
		}
	}
	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: base()})
	require.NoError(t, err, "首次定义不受锁（旧集为空）")

	mutErr := func(f func(fs []model.TicketTypeFieldInput) []model.TicketTypeFieldInput) error {
		fs := f(base())
		_, err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: fs})
		return err
	}

	t.Run("硬锁：改 field_type", func(t *testing.T) {
		requireErrCode(t, mutErr(func(fs []model.TicketTypeFieldInput) []model.TicketTypeFieldInput {
			fs[0].FieldType = "input"
			return fs
		}), errcode.ErrInvalidParams.Code)
	})
	t.Run("硬锁：改 field_key（=删+增）", func(t *testing.T) {
		requireErrCode(t, mutErr(func(fs []model.TicketTypeFieldInput) []model.TicketTypeFieldInput {
			fs[0].FieldKey = "num2"
			return fs
		}), errcode.ErrInvalidParams.Code)
	})
	t.Run("硬锁：删除存量字段", func(t *testing.T) {
		requireErrCode(t, mutErr(func(fs []model.TicketTypeFieldInput) []model.TicketTypeFieldInput {
			return fs[1:]
		}), errcode.ErrInvalidParams.Code)
	})
	t.Run("软提示：required false→true（可继续+警告）", func(t *testing.T) {
		fs := base()
		fs[0].Required = true
		ws, err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: fs})
		require.NoError(t, err, "软提示不阻断保存")
		require.Len(t, ws, 1)
		assert.Equal(t, "num", ws[0].Field)
		assert.Contains(t, ws[0].Reason, "required")
	})
	t.Run("软提示：options 收窄（可继续+警告）", func(t *testing.T) {
		fs := base()
		fs[1].FieldOptions = json.RawMessage(`["a"]`)
		ws, err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: fs})
		require.NoError(t, err, "软提示不阻断保存")
		require.Len(t, ws, 1)
		assert.Equal(t, "sel", ws[0].Field)
		assert.Contains(t, ws[0].Reason, "options")
	})
	t.Run("自由面：改 label+排序+新增+挂 formula/fetch（末位——后续子测以 base 为基）", func(t *testing.T) {
		err := mutErr(func(fs []model.TicketTypeFieldInput) []model.TicketTypeFieldInput {
			fs[0].FieldLabel = "数量（改）"
			fs[0].SortOrder = 9
			return append(fs,
				model.TicketTypeFieldInput{FieldKey: "tot", FieldLabel: "合计", FieldType: "number", SortOrder: 4,
					FieldMeta: json.RawMessage(`{"formula":"num * 2"}`)},
				model.TicketTypeFieldInput{FieldKey: "own", FieldLabel: "责任人", FieldType: "input", SortOrder: 5,
					FieldMeta: json.RawMessage(`{"fetch":{"source":{"type":"mock"},"trigger_by":"lab","mapping":{"owner":"own"}}}`)})
		})
		require.NoError(t, err, "label/排序/新增/formula/fetch 属自由面（兼容性判据版）")
	})
}

func TestB17_FormulaRefs(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := setupTicket2a(t)
	code := "b17fr_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "禁链式批"})
	require.NoError(t, err)

	save := func(fs []model.TicketTypeFieldInput) error {
		_, err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: fs})
		return err
	}
	price := model.TicketTypeFieldInput{FieldKey: "price", FieldLabel: "单价", FieldType: "number", SortOrder: 1}

	t.Run("负：formula 引用 formula 字段（禁链式——N1 终局）", func(t *testing.T) {
		requireErrCode(t, save([]model.TicketTypeFieldInput{
			price,
			{FieldKey: "total", FieldLabel: "合计", FieldType: "number", SortOrder: 2,
				FieldMeta: json.RawMessage(`{"formula":"price * 2"}`)},
			{FieldKey: "grand", FieldLabel: "总计", FieldType: "number", SortOrder: 3,
				FieldMeta: json.RawMessage(`{"formula":"total + 1"}`)},
		}), errcode.ErrInvalidParams.Code)
	})
	t.Run("负：formula 引用不存在字段", func(t *testing.T) {
		requireErrCode(t, save([]model.TicketTypeFieldInput{
			price,
			{FieldKey: "tot", FieldLabel: "合计", FieldType: "number", SortOrder: 2,
				FieldMeta: json.RawMessage(`{"formula":"ghost_field * 2"}`)},
		}), errcode.ErrInvalidParams.Code)
	})
	t.Run("正：formula 引用数据字段", func(t *testing.T) {
		require.NoError(t, save([]model.TicketTypeFieldInput{
			price,
			{FieldKey: "total", FieldLabel: "合计", FieldType: "number", SortOrder: 2,
				FieldMeta: json.RawMessage(`{"formula":"price * 2"}`)},
		}))
	})
}

func TestB17_DictCodeExistence(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := setupTicket2a(t)
	code := "b17dc_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "字典码批"})
	require.NoError(t, err)

	t.Run("负：引用不存在的字典码（三路F-B-8 收口）", func(t *testing.T) {
		_, err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code,
			Fields: []model.TicketTypeFieldInput{
				{FieldKey: "env", FieldLabel: "环境", FieldType: "select", SortOrder: 1,
					FieldOptions: json.RawMessage(`["dict:b17_nope_` + uniqueSuffix() + `"]`)},
			}})
		requireErrCode(t, err, errcode.ErrInvalidParams.Code)
	})
	t.Run("正：引用真实字典", func(t *testing.T) {
		dc := "b17dict_" + uniqueSuffix()
		_, err := testPool.Exec(ctx,
			`INSERT INTO dict_types (code, name) VALUES ($1, 'B1-7 字典码批')`, dc)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM dict_types WHERE code=$1`, dc)
		})
		_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code,
			Fields: []model.TicketTypeFieldInput{
				{FieldKey: "env", FieldLabel: "环境", FieldType: "select", SortOrder: 1,
					FieldOptions: json.RawMessage(`["dict:` + dc + `"]`)},
			}})
		require.NoError(t, err)
	})
}

func TestB17_FetchChecks(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := setupTicket2a(t)
	code := "b17fc_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "取数校验批"})
	require.NoError(t, err)

	save := func(fs []model.TicketTypeFieldInput) error {
		_, err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: fs})
		return err
	}
	host := model.TicketTypeFieldInput{FieldKey: "host_ip", FieldLabel: "主机", FieldType: "input", SortOrder: 1}
	fetchOn := func(key, trigger string) model.TicketTypeFieldInput {
		return model.TicketTypeFieldInput{FieldKey: key, FieldLabel: key, FieldType: "input", SortOrder: 2,
			FieldMeta: json.RawMessage(`{"fetch":{"source":{"type":"mock"},"trigger_by":"` + trigger + `","mapping":{"owner":"` + key + `"}}}`)}
	}

	t.Run("负：trigger_by 引用不存在字段", func(t *testing.T) {
		requireErrCode(t, save([]model.TicketTypeFieldInput{host, fetchOn("own", "ghost")}), errcode.ErrInvalidParams.Code)
	})
	t.Run("负：trigger_by 绑 formula 字段（时序不可能）", func(t *testing.T) {
		calc := model.TicketTypeFieldInput{FieldKey: "calc", FieldLabel: "算", FieldType: "number", SortOrder: 1,
			FieldMeta: json.RawMessage(`{"formula":"1 + 1"}`)} // 常量式 formula：无标识符引用，单独触发②
		requireErrCode(t, save([]model.TicketTypeFieldInput{calc, fetchOn("own", "calc")}), errcode.ErrInvalidParams.Code)
	})
	t.Run("负：mapping 目标不存在", func(t *testing.T) {
		fs := []model.TicketTypeFieldInput{host,
			{FieldKey: "own", FieldLabel: "own", FieldType: "input", SortOrder: 2,
				FieldMeta: json.RawMessage(`{"fetch":{"source":{"type":"mock"},"trigger_by":"host_ip","mapping":{"owner":"ghost_dst"}}}`)}}
		requireErrCode(t, save(fs), errcode.ErrInvalidParams.Code)
	})
	t.Run("负：触发引用图成环", func(t *testing.T) {
		requireErrCode(t, save([]model.TicketTypeFieldInput{
			fetchOn("x", "y"), fetchOn("y", "x"),
		}), errcode.ErrInvalidParams.Code)
	})
	t.Run("正：单跳 fetch（host→own）", func(t *testing.T) {
		require.NoError(t, save([]model.TicketTypeFieldInput{host, fetchOn("own", "host_ip")}))
	})
}

func TestB17_FormulaDrift(t *testing.T) {
	ctx := context.Background()
	svc, aid, _, _, _ := setupTicket2a(t)
	code := "b17dr_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "漂移批"})
	require.NoError(t, err)
	oid := childOrgID(t, "b17dr")

	t.Run("负：input 型 formula 产出数字 → 提交 400（三路F-B-3）", func(t *testing.T) {
		_, err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code,
			Fields: []model.TicketTypeFieldInput{
				{FieldKey: "qty", FieldLabel: "数量", FieldType: "number", Required: true, SortOrder: 1},
				{FieldKey: "tag", FieldLabel: "标签", FieldType: "input", SortOrder: 2,
					FieldMeta: json.RawMessage(`{"formula":"qty * 2"}`)},
			}})
		require.NoError(t, err, "保存期不拦（预判不可实施）——漂移在提交期拦")
		_, err = svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, Title: "漂移单", Priority: 2, OrgID: oid,
			CustomData: json.RawMessage(`{"qty":3}`),
		}, aid)
		requireErrCode(t, err, errcode.ErrInvalidParams.Code)
		assert.True(t, strings.Contains(err.Error(), "tag"), "错误应带字段名")
	})
	t.Run("正：number 型 formula 匹配（既有语义回归）", func(t *testing.T) {
		code2 := "b17drok_" + uniqueSuffix()
		_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code2, Name: "漂移正例"})
		require.NoError(t, err)
		_, err = svc.ReplaceTicketTypeFields(ctx, code2, &model.ReplaceTypeFieldsRequest{Code: code2,
			Fields: []model.TicketTypeFieldInput{
				{FieldKey: "qty", FieldLabel: "数量", FieldType: "number", Required: true, SortOrder: 1},
				{FieldKey: "tot", FieldLabel: "合计", FieldType: "number", SortOrder: 2,
					FieldMeta: json.RawMessage(`{"formula":"qty * 2"}`)},
			}})
		require.NoError(t, err)
	})
}
