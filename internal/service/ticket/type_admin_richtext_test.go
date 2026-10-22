package ticket

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// B5-4：richtext 第八种字段类型——枚举扩 + G2 值校验（字符串 + MD 源文本上限）。
func TestValidFieldTypes_RichtextEighth(t *testing.T) {
	assert.True(t, validFieldTypes["richtext"], "richtext 须在允许类型内（05 号 §6）")
	// B1-6 批1（2026-10-22）：第 9/10/11 类型加入——只增不改语义延续（11 种终态）
	assert.True(t, validFieldTypes["radio"] && validFieldTypes["rating"] && validFieldTypes["file"],
		"radio/rating/file 须在允许类型内（01 号 §2 B1-6）")
	assert.Len(t, validFieldTypes, 11, "只增不改：八种 + radio/rating/file")
}

func TestValidateFieldValue_Richtext(t *testing.T) {
	f := &model.TicketTypeField{FieldKey: "body", FieldType: "richtext"}

	t.Run("合法 MD 字符串放行（含稳定 URL 图片引用）", func(t *testing.T) {
		raw := json.RawMessage(`"# 标题\n\n![img](/api/v1/files/tickets/2026/10/x.png)\n\n- 列表"`)
		require.NoError(t, validateFieldValue(f, raw))
	})

	t.Run("非字符串拒 400", func(t *testing.T) {
		err := validateFieldValue(f, json.RawMessage(`{"md":"x"}`))
		var e *errcode.Error
		require.ErrorAs(t, err, &e)
		assert.Equal(t, errcode.ErrInvalidParams.Code, e.Code)
	})

	t.Run("恰到上限放行、超一字节拒", func(t *testing.T) {
		okRaw, _ := json.Marshal(strings.Repeat("a", RichtextMaxBytes))
		require.NoError(t, validateFieldValue(f, okRaw))

		bigRaw, _ := json.Marshal(strings.Repeat("a", RichtextMaxBytes+1))
		err := validateFieldValue(f, bigRaw)
		var e *errcode.Error
		require.ErrorAs(t, err, &e)
		assert.Equal(t, errcode.ErrInvalidParams.Code, e.Code)
		assert.Contains(t, e.Message, "上限")
	})

	t.Run("validate_regex 对 richtext 同样生效", func(t *testing.T) {
		fr := &model.TicketTypeField{FieldKey: "body", FieldType: "richtext", ValidateRegex: `^#\s`}
		require.NoError(t, validateFieldValue(fr, json.RawMessage(`"# ok"`)))
		require.Error(t, validateFieldValue(fr, json.RawMessage(`"no heading"`)))
	})
}
