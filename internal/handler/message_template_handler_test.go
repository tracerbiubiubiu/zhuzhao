package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 2026-11-02 审计补：Variables 端点字典与 service 白名单「两处同源」的字典侧镜像守卫——
// 与 internal/service/message_template_validate_test.go TestValidatePlaceholders_OpsSeedTemplates 成对。
func TestBuiltinTemplateVars_CoversOpsSeedKeys(t *testing.T) {
	got := map[string]bool{}
	for _, v := range BuiltinTemplateVars() {
		got[v["path"]] = true
	}
	for _, k := range []string{"ticket.creator", "event.job", "event.error", "event.last_success",
		"event.action", "event.ticket_id", "event.actor",
		"event.level", "event.ratio", "event.usage_bytes", "event.capacity_bytes"} {
		assert.True(t, got[k], "variables 端点字典缺 %s——管理员看不到可用键", k)
	}
}
