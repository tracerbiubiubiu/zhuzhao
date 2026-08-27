package ticket

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// seedTransitions 对齐 migrations/000010_ticket.up.sql 中 ticket_types.transitions
// 的 DEFAULT 值，保证状态机契约与种子数据一致。
const seedTransitions = `{
	"open":          ["assigned", "closed"],
	"assigned":      ["in_progress", "open"],
	"in_progress":   ["pending_verify", "rejected", "closed"],
	"pending_verify": ["closed", "in_progress"],
	"closed":        ["open"],
	"rejected":      ["open"]
}`

// TestNewStateMachine_ParsesValidTransitions 验证合法 transitions JSON 能正确解析。
func TestNewStateMachine_ParsesValidTransitions(t *testing.T) {
	sm, err := NewStateMachine(json.RawMessage(seedTransitions))
	require.NoError(t, err)
	require.NotNil(t, sm)
}

// TestNewStateMachine_InvalidJSONReturnsError 非法 JSON 应返回解析错误。
func TestNewStateMachine_InvalidJSONReturnsError(t *testing.T) {
	sm, err := NewStateMachine(json.RawMessage(`{not-json`))
	require.Error(t, err)
	assert.Nil(t, sm)
}

// TestNewStateMachine_EmptyTransitions 空对象构造出「无任何转换」的状态机。
func TestNewStateMachine_EmptyTransitions(t *testing.T) {
	sm, err := NewStateMachine(json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.False(t, sm.CanTransition("open", "closed"))
}

// TestCanTransition_SeedValidPaths 种子配置中的合法转换路径均应返回 true。
func TestCanTransition_SeedValidPaths(t *testing.T) {
	sm := mustSM(t, seedTransitions)
	cases := []struct{ from, to string }{
		{"open", "assigned"},
		{"open", "closed"},
		{"assigned", "in_progress"},
		{"assigned", "open"},
		{"in_progress", "pending_verify"},
		{"in_progress", "rejected"},
		{"in_progress", "closed"},
		{"pending_verify", "closed"},
		{"pending_verify", "in_progress"},
		{"closed", "open"},
		{"rejected", "open"},
	}
	for _, c := range cases {
		assert.True(t, sm.CanTransition(c.from, c.to),
			"expected %s -> %s to be allowed", c.from, c.to)
	}
}

// TestCanTransition_SeedInvalidPaths 种子配置中未定义的转换路径应返回 false。
func TestCanTransition_SeedInvalidPaths(t *testing.T) {
	sm := mustSM(t, seedTransitions)
	cases := []struct{ from, to string }{
		// 反向路径未定义
		{"open", "in_progress"},
		{"open", "rejected"},
		{"assigned", "closed"},
		{"closed", "in_progress"},
		{"rejected", "closed"},
		// 终态自环未定义（closed -> closed / rejected -> rejected）
		{"closed", "closed"},
		{"rejected", "rejected"},
	}
	for _, c := range cases {
		assert.False(t, sm.CanTransition(c.from, c.to),
			"expected %s -> %s to be rejected", c.from, c.to)
	}
}

// TestCanTransition_UnknownSource 未登记的源状态应返回 false。
func TestCanTransition_UnknownSource(t *testing.T) {
	sm := mustSM(t, seedTransitions)
	assert.False(t, sm.CanTransition("nonexistent", "open"))
}

// TestAssertTransition_ValidReturnsNil 合法转换 AssertTransition 返回 nil。
func TestAssertTransition_ValidReturnsNil(t *testing.T) {
	sm := mustSM(t, seedTransitions)
	assert.NoError(t, sm.AssertTransition("open", "assigned"))
	assert.NoError(t, sm.AssertTransition("in_progress", "closed"))
}

// TestAssertTransition_InvalidReturnsErrTicketInvalidTransition 非法转换
// 应返回 errcode.ErrTicketInvalidTransition（错误码 90002）。
func TestAssertTransition_InvalidReturnsErrTicketInvalidTransition(t *testing.T) {
	sm := mustSM(t, seedTransitions)
	err := sm.AssertTransition("open", "in_progress")
	require.Error(t, err)
	var biz *errcode.Error
	require.True(t, errors.As(err, &biz))
	assert.Equal(t, errcode.ErrTicketInvalidTransition.Code, biz.Code)
}

// TestFromTicketType_BuildsFromSeed 从 model.TicketType（种子 transitions）构建状态机。
// 覆盖 Close 流程实际使用的构造路径：service.Close → FromTicketType → AssertTransition。
func TestFromTicketType_BuildsFromSeed(t *testing.T) {
	ttype := &model.TicketType{
		Code:        "incident",
		Transitions: json.RawMessage(seedTransitions),
	}
	sm, err := FromTicketType(ttype)
	require.NoError(t, err)
	// Close 流程核心断言：open -> closed 合法（直接关单）
	assert.True(t, sm.CanTransition("open", "closed"))
	// assigned -> closed 不在种子配置（须先 in_progress）→ 非法
	assert.False(t, sm.CanTransition("assigned", "closed"))
}

// mustSM 测试辅助：从 transitions JSON 构建状态机，失败即 Fatal。
func mustSM(t *testing.T, raw string) *StateMachine {
	t.Helper()
	sm, err := NewStateMachine(json.RawMessage(raw))
	require.NoError(t, err)
	return sm
}
