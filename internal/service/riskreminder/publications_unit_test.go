// 发布解析纯函数单元测试（无 PG；进 make lint/test-unit 面）。
package riskreminder

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func pubSnapshot(states, meta string) *model.TicketDefinitionSnapshot {
	return &model.TicketDefinitionSnapshot{Type: model.TicketType{
		States:         json.RawMessage(states),
		TransitionMeta: json.RawMessage(meta),
	}}
}

const pubUnitStates = `["open","assigned","in_progress","closed"]`

func TestParseAcceptancePublicationValid(t *testing.T) {
	meta := `{"assigned":{"first_accept_reminder":{"rule_id":"first_accept","rule_version":3,"accept_within_minutes":30}}}`
	pub, err := parseAcceptancePublication(pubSnapshot(pubUnitStates, meta))
	if err != nil {
		t.Fatalf("valid publication rejected: %v", err)
	}
	if pub.AwaitingState != "assigned" || pub.RuleID != "first_accept" || pub.RuleVersion != 3 || pub.AcceptWithinMinutes != 30 {
		t.Fatalf("publication fields wrong: %+v", pub)
	}
	// 同状态段的其他元数据（节点 SLA 域）不得影响发布解析
	meta = `{"assigned":{"timeout_minutes":8,"escalation":{"notify":["assignee"]},"first_accept_reminder":{"rule_id":"r","rule_version":1,"accept_within_minutes":45}}}`
	if pub, err = parseAcceptancePublication(pubSnapshot(pubUnitStates, meta)); err != nil ||
		pub.AcceptWithinMinutes != 45 || pub.RuleID != "r" {
		t.Fatalf("node SLA metadata must not leak into publication: pub=%+v err=%v", pub, err)
	}
}

// 缺省/空段/显式 null/transition 级键都不是发布——唯一合法的「无提醒事实」路径。
func TestParseAcceptancePublicationAbsent(t *testing.T) {
	for _, meta := range []string{
		`{}`,
		`{"assigned":{}}`,
		`{"assigned":{"first_accept_reminder":null}}`,
		`{"assigned":{"timeout_minutes":5}}`,
		`{"assigned>in_progress":{"first_accept_reminder":{"rule_id":"x","rule_version":1,"accept_within_minutes":5}}}`,
	} {
		if _, err := parseAcceptancePublication(pubSnapshot(pubUnitStates, meta)); !errors.Is(err, ErrReminderPolicyAbsent) {
			t.Fatalf("meta %s should be absent, got %v", meta, err)
		}
	}
}

// 显式条目存在但损坏（空对象/字段非法/超长/多状态重复/域不可读）→ 可见错误，
// 绝不静默降级为「无发布」。
func TestParseAcceptancePublicationBrokenVisible(t *testing.T) {
	long := strings.Repeat("x", 129)
	for _, meta := range []string{
		`{"assigned":{"first_accept_reminder":{}}}`,
		`{"assigned":{"first_accept_reminder":{"rule_id":"","rule_version":1,"accept_within_minutes":30}}}`,
		`{"assigned":{"first_accept_reminder":{"rule_id":"r","rule_version":0,"accept_within_minutes":30}}}`,
		`{"assigned":{"first_accept_reminder":{"rule_id":"r","rule_version":1,"accept_within_minutes":-5}}}`,
		`{"assigned":{"first_accept_reminder":{"rule_id":"r","rule_version":1}}}`,
		`{"assigned":{"first_accept_reminder":{"rule_id":"` + long + `","rule_version":1,"accept_within_minutes":30}}}`,
		`{"assigned":{"first_accept_reminder":{"rule_id":"r","rule_version":1,"accept_within_minutes":30}},"open":{"first_accept_reminder":{"rule_id":"r","rule_version":1,"accept_within_minutes":30}}}`,
	} {
		_, err := parseAcceptancePublication(pubSnapshot(pubUnitStates, meta))
		if err == nil || errors.Is(err, ErrReminderPolicyAbsent) {
			t.Fatalf("broken explicit entry must be a visible error, meta %s got %v", meta, err)
		}
	}
	// 域本身不可读（states/transition_meta 非法 JSON）→ 错误而非 absent
	if _, err := parseAcceptancePublication(pubSnapshot(`notjson`, `{}`)); err == nil {
		t.Fatal("unreadable states must error")
	}
	if _, err := parseAcceptancePublication(pubSnapshot(pubUnitStates, `notjson`)); err == nil {
		t.Fatal("unreadable transition_meta must error")
	}
	if _, err := parseAcceptancePublication(nil); !errors.Is(err, ErrPolicyUnavailable) {
		t.Fatalf("nil snapshot must be ErrPolicyUnavailable, got %v", err)
	}
}
