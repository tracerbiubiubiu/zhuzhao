// msgrender 值级转义测试（外审 F-1-2 恢复——c309ca7 曾误删，本批恢复+升级）。
// 守卫目标：值中 HTML 被转义、模板文本原样、二次展开消除——M1 变异防线。
package msgrender

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderQuadrantEscaped(t *testing.T) {
	vars := map[string]any{
		"ticket": map[string]any{
			"title": "<script>alert(1)</script>",
			"name":  "<b>bold</b>",
		},
	}

	t.Run("值中HTML被转义", func(t *testing.T) {
		r := RenderQuadrantEscaped("工单 {{ticket.title}}", vars, "email_html")
		require.NoError(t, r.Err)
		assert.Contains(t, r.Text, "&lt;script&gt;")
		assert.NotContains(t, r.Text, "<script>", "值中 HTML 不得原样入正文")
	})

	t.Run("模板文本原样——值级转义不碰模板", func(t *testing.T) {
		r := RenderQuadrantEscaped("<b>工单</b>: {{ticket.title}}", vars, "email_html")
		require.NoError(t, r.Err)
		assert.Contains(t, r.Text, "<b>工单</b>", "模板文本 <b> 不应被转义")
		assert.Contains(t, r.Text, "&lt;script&gt;", "值中 <script> 应被转义")
	})

	t.Run("非转义渠道(uip)原样", func(t *testing.T) {
		r := RenderQuadrantEscaped("{{ticket.title}}", vars, "uip")
		require.NoError(t, r.Err)
		assert.Contains(t, r.Text, "<script>", "uip 渠道纯文本不转义")
	})

	t.Run("二次展开消除——值内{{}}不再解析", func(t *testing.T) {
		vars2 := map[string]any{
			"ticket": map[string]any{
				"title": "{{ticket.name}}",
				"name":  "SECRET",
			},
		}
		r := RenderQuadrantEscaped("{{ticket.title}}", vars2, "uip")
		require.NoError(t, r.Err)
		assert.Equal(t, "{{ticket.name}}", r.Text, "值内 {{}} 应原样输出非再解析")
		assert.NotContains(t, r.Text, "SECRET", "不得通过值内占位符读到其它字段")
	})

	t.Run("转义+四象限内置键缺失=fail", func(t *testing.T) {
		noVars := map[string]any{"ticket": map[string]any{}}
		r := RenderQuadrantEscaped("{{ticket.title}}", noVars, "email_html")
		require.Error(t, r.Err, "内置键缺失在转义渠道同样拒发")
	})

	t.Run("未知渠道 fail-safe 转义", func(t *testing.T) {
		r := RenderQuadrantEscaped("{{ticket.title}}", vars, "unknown_channel")
		require.NoError(t, r.Err)
		assert.Contains(t, r.Text, "&lt;script&gt;", "未知渠道应 HTML fail-safe")
	})
}
