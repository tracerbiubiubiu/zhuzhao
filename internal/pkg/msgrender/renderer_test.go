// msgrender 占位符渲染单测（B6-3——06 号 A.4）。
package msgrender

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRender(t *testing.T) {
	vars := map[string]any{
		"ticket": map[string]any{
			"title":     "磁盘告警",
			"human_key": "TK-20261023-0001",
			"type_code": "incident",
			"priority":  2,
			"assignee": map[string]any{
				"name": "张三",
				"no":   "E000123",
			},
		},
	}

	t.Run("点路径取值", func(t *testing.T) {
		out := Render("工单 {{ticket.title}} 已创建", vars)
		assert.Equal(t, "工单 磁盘告警 已创建", out)
	})

	t.Run("嵌套路径", func(t *testing.T) {
		out := Render("处理人：{{ticket.assignee.name}}（{{ticket.assignee.no}}）", vars)
		assert.Equal(t, "处理人：张三（E000123）", out)
	})

	t.Run("数字值", func(t *testing.T) {
		out := Render("优先级 P{{ticket.priority}}", vars)
		assert.Equal(t, "优先级 P2", out)
	})

	t.Run("未命中保留原样", func(t *testing.T) {
		out := Render("{{ticket.ghost}} 和 {{ghost.var}}", vars)
		assert.Equal(t, "{{ticket.ghost}} 和 {{ghost.var}}", out)
	})

	t.Run("注入安全——字段值含 {{}} 不被执行", func(t *testing.T) {
		vars2 := map[string]any{
			"ticket": map[string]any{
				"title": "{{evil.template_call}}",
			},
		}
		out := Render("{{ticket.title}}", vars2)
		assert.Equal(t, "{{evil.template_call}}", out, "值中的 {{}} 应原样输出非再解析")
	})

	t.Run("多占位符同串", func(t *testing.T) {
		out := Render("[{{ticket.human_key}}] {{ticket.title}}（{{ticket.type_code}}）", vars)
		assert.Equal(t, "[TK-20261023-0001] 磁盘告警（incident）", out)
	})
}

func TestExtractPlaceholders(t *testing.T) {
	ph := ExtractPlaceholders("[{{ticket.human_key}}] {{ticket.title}} by {{ticket.assignee.name}}")
	assert.Equal(t, []string{"ticket.human_key", "ticket.title", "ticket.assignee.name"}, ph)
}

func TestRenderBoth(t *testing.T) {
	vars := map[string]any{
		"ticket": map[string]any{"title": "测试", "human_key": "TK-001"},
	}
	subj, body := RenderBoth("[{{ticket.human_key}}] 通知", "工单 {{ticket.title}}", vars)
	assert.Equal(t, "[TK-001] 通知", subj)
	assert.Equal(t, "工单 测试", body)
}
