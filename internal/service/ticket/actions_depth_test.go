package ticket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
)

// recursiveAction 模拟「动作内再触发转换」：Transition 从 ctx 取深度再跑一轮 post_actions。
type recursiveAction struct {
	reg    *ActionRegistry
	tm     *TransitionMeta
	calls  *int
	depths *[]int
}

func (a *recursiveAction) Name() string                { return "recurse" }
func (a *recursiveAction) Delivery() DeliverySemantics { return DeliveryAtMostOnce }
func (a *recursiveAction) Execute(ctx context.Context, ac ActionContext) error {
	*a.calls++
	*a.depths = append(*a.depths, actionDepth(ctx))
	a.reg.RunPostActions(ctx, nil, a.tm, ac, actionDepth(ctx))
	return nil
}

// 审计 #3：maxDepth 守卫此前形同虚设（调用方恒传 0 且无递归载体）。现深度经 ctx 透传：
// 顶层 depth=0 → 动作以 depth+1 执行 → 动作内再触发即逐级 +1，depth>maxDepth 截断。
func TestRunPostActions_DepthCutsCascade(t *testing.T) {
	reg := NewActionRegistry() // maxDepth=3
	tm := &TransitionMeta{PostActions: []json.RawMessage{json.RawMessage(`"recurse"`)}}
	var calls int
	var depths []int
	require.NoError(t, reg.Register(&recursiveAction{reg: reg, tm: tm, calls: &calls, depths: &depths}))

	ctx := context.Background()
	reg.RunPostActions(ctx, nil, tm, ActionContext{Ticket: &model.Ticket{ID: 1}}, actionDepth(ctx))

	// 深度 0/1/2/3 各执行一轮（depth 4 > max 3 截断）——修前此用例栈溢出
	assert.Equal(t, reg.maxDepth+1, calls, "级联在 maxDepth 处截断")
	assert.Equal(t, []int{1, 2, 3, 4}, depths, "动作内可见深度=本轮 depth+1")
}

// 审计 #6：JSON 数字解出为 float64，fmt.Sprint(1e6)="1e+06"——须经 int64 格式化。
func TestIdsFromParam_FloatAndNumericString(t *testing.T) {
	got := idsFromParam([]any{float64(1000000), "1234567", float64(42), "x", true, nil})
	assert.Equal(t, []int64{1000000, 1234567, 42}, got)
}

// P2 回归（2026-11-01 审计）：动作 panic 被 recover 转为统一失败分支——不沿同步栈
// 上抛（转换已 commit），且 ops.action_failed 事件照发（error 带 panic 前缀）。
type boomAction struct{}

func (boomAction) Name() string                                 { return "boom" }
func (boomAction) Delivery() DeliverySemantics                  { return DeliveryAtMostOnce }
func (boomAction) Execute(context.Context, ActionContext) error { panic("kaboom") }

func TestRunPostActions_PanicRecovered(t *testing.T) {
	reg := NewActionRegistry()
	require.NoError(t, reg.Register(boomAction{}))
	var got *domainevent.DomainEvent
	reg.SetEmitter(func(ev domainevent.DomainEvent) { cp := ev; got = &cp })
	tm := &TransitionMeta{PostActions: []json.RawMessage{json.RawMessage(`{"name":"boom"}`)}}

	assert.NotPanics(t, func() {
		reg.RunPostActions(context.Background(), nil, tm, ActionContext{Ticket: &model.Ticket{ID: 7}, ActorID: 9}, 0)
	}, "动作 panic 不得沿 RunPostActions 同步栈上抛")
	require.NotNil(t, got, "panic 走统一失败分支——ops.action_failed 必须照发")
	assert.Equal(t, domainevent.ActionOpsActionFailed, got.Action)
	assert.Equal(t, "boom", got.Payload["action"])
	assert.Contains(t, got.Payload["error"], "action panic: kaboom")
}
