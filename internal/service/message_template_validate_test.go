// 模板引用⊆字典校验回归测试（外审 F3-1-2——validatePlaceholders 零覆盖补课）。
// 三条红线：Create 拒动态键 / Update 拒动态键 / 合法内置键放行。
package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func TestValidatePlaceholders(t *testing.T) {

	t.Run("合法内置键放行", func(t *testing.T) {
		err := validatePlaceholders("[{{ticket.human_key}}] {{ticket.title}}", "链接：{{ticket.link}}")
		require.NoError(t, err)
	})

	t.Run("ticket.fields.* 前缀放行（F3-1-3——动态字段软告警路径生产可达）", func(t *testing.T) {
		err := validatePlaceholders("{{ticket.fields.f_1}}", "")
		require.NoError(t, err, "fields.* 前缀应放行——RenderQuadrant 动态键档兜底")
	})

	t.Run("非前缀动态键仍拒", func(t *testing.T) {
		err := validatePlaceholders("{{ticket.ghost}}", "")
		require.Error(t, err, "非白名单非前缀变量仍拒")
		assert.Contains(t, err.Error(), "ticket.ghost")
	})

	t.Run("Update 拒动态键（F-1-9b 旁路修复锚）", func(t *testing.T) {
		// 模拟 Update 链——validatePlaceholders 必须同在 Update 调用
		err := validatePlaceholders("", "{{ticket.ghost}}")
		require.Error(t, err)
	})

	t.Run("builtinVarSet 与 variables 端点一致", func(t *testing.T) {
		// 六键完整性——新增内置键须同步两处
		for _, k := range []string{"ticket.title", "ticket.type_code", "ticket.priority", "ticket.status", "ticket.human_key", "ticket.link"} {
			assert.True(t, builtinVarSet[k], "%s 应在白名单内", k)
		}
		assert.Len(t, builtinVarSet, 6)
	})
}

func TestValidatePlaceholders_IntegrationPaths(t *testing.T) {
	// 验证 Create/Update service 方法确实调用校验（防旁路——外审 F-1-9b）
	// 注：完整链路须 DB——此测用 nil repo 断言 panic/err 前置校验先触达
	s := &MessageTemplateService{}
	tpl := &model.MessageTemplate{
		Code: "t", Name: "t", Channel: "uip", BizType: "ticket",
		Subject: "{{ticket.ghost}}", Body: "", Enabled: true,
	}
	// Create 走到 repo 前应先被 validatePlaceholders 拒
	_ = json.RawMessage(`{}`)
	err := s.Create(context.Background(), tpl)
	require.Error(t, err, "Create 应在 repo 层前被校验拦截")
	assert.Contains(t, err.Error(), "ticket.ghost")

	// v3 审计 Y-5（2026-10-23）：Update 路径同款——修复前本文件仅直调
	// validatePlaceholders 函数（名为"Update 拒动态键"实为函数测），回退
	// Update 内的校验调用不会红任何测试；现经 svc.Update 真链路锁定。
	errU := s.Update(context.Background(), tpl)
	require.Error(t, errU, "Update 应在 repo 层前被校验拦截（F-1-9b 旁路修复真锁——撤 Update 内校验必红）")
	assert.Contains(t, errU.Error(), "ticket.ghost")
}
