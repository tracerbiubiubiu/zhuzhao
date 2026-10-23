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

	t.Run("uip 渠道不转义（真实渠道名——DDL 默认）", func(t *testing.T) {
		out := RenderEscaped("{{ticket.severity}}", vars, "uip")
		assert.Contains(t, out, "High&Critical", "uip 桩态纯文本——& 不转义")
	})

	t.Run("值级转义——模板静态文本不被转义", func(t *testing.T) {
		out := RenderEscaped("A&B <b>标签</b> {{ticket.severity}}", vars, "email_html")
		assert.Contains(t, out, "A&B <b>标签</b>", "模板自身文本原样")
		assert.Contains(t, out, "High&amp;Critical", "只有变量值被转义")
	})

	t.Run("未知渠道保守 HTML 转义（fail-safe）", func(t *testing.T) {
		out := RenderEscaped("{{ticket.severity}}", vars, "mystery")
		assert.Contains(t, out, "High&amp;Critical", "未知渠道须转义")
	})
}

func TestRenderQuadrant(t *testing.T) {
	vars := map[string]any{
		"ticket": map[string]any{
			"title":  "标题",
			"fields": map[string]any{"f_123": "值"},
		},
	}

	t.Run("未命中占位符保留原样（当前实现——不分内置/动态）", func(t *testing.T) {
		out := Render("{{ticket.nonexistent}}", vars)
		assert.Contains(t, out, "{{ticket.nonexistent}}", "未命中保留")
	})

	t.Run("动态字段命中→替换值", func(t *testing.T) {
		out := Render("{{ticket.fields.f_123}}", vars)
		assert.Contains(t, out, "值")
	})

	t.Run("未命中不拒发（当前实现——设计差距注记见下）", func(t *testing.T) {
		// ⚠ 设计差距注记（外审实例批发现）：06 号 A.4 四象限设计=
		//   内置缺失=fail 拒发 / 动态缺失=skip+警告——当前实现统一"保留原样"。
		//   差距已知：Render 无 error 返回、无内置/动态前缀区分。实现随 B6-3
		//   增强批（Render 加 error + ticket.fields.* 前缀分档），此处测的是
		//   当前实际行为（非设计目标态），防虚标。
		// 此测试验证不 crash 即可
		out := Render("{{ticket.missing_builtin}} | {{ticket.fields.f_missing}}", vars)
		assert.NotEmpty(t, out)
	})
}
