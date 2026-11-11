package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
)

// failingAction 恒败动作（G-5 机制测试用——触发 ops.action_failed emit 分支）。
type failingAction struct{}

func (failingAction) Name() string                { return "always_fail" }
func (failingAction) Delivery() DeliverySemantics { return DeliveryAtMostOnce }
func (failingAction) Execute(context.Context, ActionContext) error {
	return errors.New("boom")
}

// TestG5_ActionFailedEmitter G-5 机制锚：SetEmitter 接线后 post_action 失败须派发
// ops.action_failed 到事件注册表（修复前全仓零调用 SetEmitter——r.emitter 恒 nil，
// emit 分支死码、动作失败仅 slog.Warn）。装配层接线在 providers.go（与 Service
// 出口同款闭包），此处以同款闭包端到端验证机制：事件→订阅者+载荷锚点完整。
func TestG5_ActionFailedEmitter(t *testing.T) {
	eventReg := domainevent.NewHookRegistry(nil)
	var got *domainevent.DomainEvent
	done := make(chan struct{}, 1)
	eventReg.Subscribe(domainevent.BizOps, "g5.capture", domainevent.HookFunc(func(_ context.Context, ev domainevent.DomainEvent) {
		got = &ev
		done <- struct{}{}
	}))

	actionReg := NewActionRegistry()
	actionReg.Register(failingAction{})
	actionReg.SetEmitter(func(ev domainevent.DomainEvent) {
		eventReg.Dispatch(context.Background(), ev)
	})

	actionReg.RunPostActions(context.Background(), nil, &TransitionMeta{
		PostActions: []json.RawMessage{[]byte(`{"name":"always_fail"}`)},
	}, ActionContext{Ticket: &model.Ticket{ID: 42}, ActorID: 7}, 0)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ops.action_failed 未到达订阅者——emitter 未接线或未派发")
	}
	require.NotNil(t, got)
	assert.Equal(t, domainevent.BizOps, got.BizType)
	assert.Equal(t, domainevent.ActionOpsActionFailed, got.Action)
	assert.Equal(t, "42", got.BizID)
	assert.Equal(t, int64(7), got.ActorID)
	assert.Equal(t, "always_fail", got.Payload["action"])
	assert.Equal(t, "42", got.Payload["ticket_id"])
	assert.NotEmpty(t, got.Payload["error"])
	assert.Equal(t, "7", got.Payload["actor"], "creator 档收件人锚点（词表收口批）")
}
