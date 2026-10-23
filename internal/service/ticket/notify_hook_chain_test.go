//go:build integration

// S6 核心链路修复——测试先行（外审报告批 1）。
// 核心断言：resolveRecipients 消费 message_rules 的 recipient_type/fixed_recipients
// （当前旁路 DDL 硬编码 switch）；created 走规则驱动（删除 return nil 特判）。
package ticket

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
)

func TestResolveRecipients_RulesDriven(t *testing.T) {
	ev := domainevent.DomainEvent{
		BizType: "ticket", Action: domainevent.TicketCreated,
		Payload: map[string]any{"actor": "42", "to_assignee": "100"},
	}

	t.Run("recipient_type=assignee→取 payload assignee", func(t *testing.T) {
		ids := resolveRecipientsFromRule(ev, "assignee", "")
		assert.Equal(t, []string{"100"}, ids)
	})

	t.Run("recipient_type=creator→取 payload actor", func(t *testing.T) {
		ids := resolveRecipientsFromRule(ev, "creator", "")
		assert.Equal(t, []string{"42"}, ids)
	})

	t.Run("recipient_type=fixed→取 DDL fixed_recipients", func(t *testing.T) {
		ids := resolveRecipientsFromRule(ev, "fixed", "20001,20002")
		assert.Equal(t, []string{"20001", "20002"}, ids)
	})

	t.Run("created 不再硬编码 nil——走规则驱动", func(t *testing.T) {
		// 修复前：resolveRecipients 对 created 直接 return nil
		// 修复后：created 也按 rule 的 recipient_type 解析
		ids := resolveRecipientsFromRule(ev, "creator", "")
		assert.NotEmpty(t, ids, "created 事件配了规则须走解析而非硬编码 nil")
	})

	t.Run("未识别 recipient_type→空（保守）", func(t *testing.T) {
		ids := resolveRecipientsFromRule(ev, "unknown_type", "")
		assert.Empty(t, ids)
	})
}
