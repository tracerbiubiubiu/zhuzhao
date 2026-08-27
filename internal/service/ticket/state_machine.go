package ticket

import (
	"encoding/json"
	"fmt"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// StateMachine 工单状态机，基于 ticket_types.transitions JSONB 校验状态转换合法性。
// transitions 格式：{"open":["assigned","closed"], "assigned":["in_progress","open"], ...}
type StateMachine struct {
	transitions map[string]map[string]bool // from -> set(to)
}

// NewStateMachine 从 ticket_types.transitions JSONB 构建状态机
func NewStateMachine(transitionsRaw json.RawMessage) (*StateMachine, error) {
	var transitions map[string][]string
	if err := json.Unmarshal(transitionsRaw, &transitions); err != nil {
		return nil, fmt.Errorf("parse transitions: %w", err)
	}
	sm := &StateMachine{
		transitions: make(map[string]map[string]bool, len(transitions)),
	}
	for from, tos := range transitions {
		sm.transitions[from] = make(map[string]bool, len(tos))
		for _, to := range tos {
			sm.transitions[from][to] = true
		}
	}
	return sm, nil
}

// CanTransition 校验 from -> to 是否为合法转换
func (sm *StateMachine) CanTransition(from, to string) bool {
	tos, ok := sm.transitions[from]
	if !ok {
		return false
	}
	return tos[to]
}

// AssertTransition 校验转换合法性，非法则返回 ErrTicketInvalidTransition
func (sm *StateMachine) AssertTransition(from, to string) error {
	if !sm.CanTransition(from, to) {
		return errcode.ErrTicketInvalidTransition
	}
	return nil
}

// FromTicketType 从工单类型构建状态机
func FromTicketType(t *model.TicketType) (*StateMachine, error) {
	return NewStateMachine(t.Transitions)
}
