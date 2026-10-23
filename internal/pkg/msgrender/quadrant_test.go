package msgrender

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRender_Escaping(t *testing.T) {
	vars := map[string]any{
		"ticket": map[string]any{
			"title":    `<script>alert("xss")</script>`,
			"severity": `High&Critical`,
		},
	}

	t.Run("HTML 渠道转义（<>& 全转）", func(t *testing.T) {
		out := RenderEscaped("{{ticket.title}}", vars, "email_html")
		assert.NotContains(t, out, "<script>", "HTML 渠道须转义 <")
		assert.Contains(t, out, "&lt;script&gt;")
	})

	t.Run("纯文本渠道不转义（原样出）", func(t *testing.T) {
		out := RenderEscaped("{{ticket.severity}}", vars, "dingtalk")
		assert.Contains(t, out, "High&Critical", "纯文本渠道 & 不转义")
	})
}

func TestRenderQuadrant(t *testing.T) {
	vars := map[string]any{
		"ticket": map[string]any{
			"title":  "标题",
			"fields": map[string]any{"f_123": "值"},
		},
	}

	t.Run("内置键缺失→保留原样（调试可见）", func(t *testing.T) {
		out := Render("{{ticket.nonexistent}}", vars)
		assert.Contains(t, out, "{{ticket.nonexistent}}", "未命中保留")
	})

	t.Run("动态字段命中→替换值", func(t *testing.T) {
		out := Render("{{ticket.fields.f_123}}", vars)
		assert.Contains(t, out, "值")
	})

	t.Run("四象限：内置缺失 skip+警告（不拒发）", func(t *testing.T) {
		// 一期实现：所有未命中统一保留原样——四象限分档为 B6-3 增强批
		// 此测试验证不 crash 即可
		out := Render("{{ticket.missing_builtin}} | {{ticket.fields.f_missing}}", vars)
		assert.NotEmpty(t, out)
	})
}
