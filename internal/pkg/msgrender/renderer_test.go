// msgrender 占位符渲染单测（B6-3——06 号 A.4）。
package msgrender

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestRenderQuadrant(t *testing.T) {
	vars := map[string]any{
		"ticket": map[string]any{
			"title": "测试工单", "type_code": "incident",
			"priority": 2, "status": "open",
			"human_key": "TK-20261023-0001", "link": "/tickets/1",
			"custom_field": nil, // 键在但无值
		},
	}

	t.Run("内置键有值=正常替换", func(t *testing.T) {
		r := RenderQuadrant("[{{ticket.human_key}}] {{ticket.title}}", vars)
		require.NoError(t, r.Err)
		assert.Equal(t, "[TK-20261023-0001] 测试工单", r.Text)
		assert.Empty(t, r.Warnings)
	})

	t.Run("内置键缺失=fail拒发", func(t *testing.T) {
		noVars := map[string]any{"ticket": map[string]any{"type_code": "x"}}
		r := RenderQuadrant("{{ticket.title}}", noVars)
		require.Error(t, r.Err, "内置键缺失必须返回错误（拒发信号）")
		assert.Contains(t, r.Err.Error(), "ticket.title")
		assert.Contains(t, r.Err.Error(), "缺失")
	})

	t.Run("动态键缺失=skip+warn", func(t *testing.T) {
		r := RenderQuadrant("工单 {{ticket.ghost_field}} 处理中", vars)
		require.NoError(t, r.Err, "动态键缺失不拒发")
		assert.Equal(t, "工单  处理中", r.Text, "动态键缺失替换为空串")
		require.Len(t, r.Warnings, 1)
		assert.Contains(t, r.Warnings[0], "ghost_field")
	})

	t.Run("键在但值为nil=空串+warn", func(t *testing.T) {
		r := RenderQuadrant("值：{{ticket.custom_field}}！", vars)
		require.NoError(t, r.Err)
		assert.Equal(t, "值：！", r.Text)
		require.Len(t, r.Warnings, 1)
		assert.Contains(t, r.Warnings[0], "custom_field")
	})

	t.Run("混合场景：内置OK+动态缺失+内置缺失=拒发", func(t *testing.T) {
		partial := map[string]any{"ticket": map[string]any{"type_code": "incident"}}
		r := RenderQuadrant("{{ticket.title}} and {{ticket.ghost}}", partial)
		require.Error(t, r.Err, "有内置键缺失→整体拒发")
	})

	t.Run("RenderBothQuadrant 任一Err则整体拒发", func(t *testing.T) {
		noVars := map[string]any{"ticket": map[string]any{}}
		subjR, bodyR := RenderBothQuadrant("{{ticket.human_key}}", "{{ticket.ghost}}", noVars)
		require.Error(t, subjR.Err, "主题内置键缺失→拒发")
		require.NoError(t, bodyR.Err, "正文动态键→不拒发")
	})
}
