// S3a B3-3 动作库框架（03 号 §3.3/附录 A.3——post-function 形态）：
// TransitionAction 接口（双触发形态：转换后 post-function[事务=转换事务] /
// 事件订阅[事务=触发方扫描批]——事务边界随触发方，A.3 裁定）+ 注册表（编译期
// 注册键，重复 panic）+ Delivery 一致性声明（2026-10-24 拍板：at_most_once 通知类
// / durable_submit 起任务类——两档不得共用投递语义）+ 链深限制（循环触发防护）。
//
// 首批四动作：notify（S6 兜底）/ cc（S10）/ reassign（转派）/ spawn_task
// （taskrunner 提交——durable_submit）。
package ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
)

// DeliverySemantics 一致性声明（2026-10-24 拍板——dd §31 同批设计纪律）。
type DeliverySemantics string

const (
	// DeliveryAtMostOnce 通知类：丢可补发不阻塞（事件异步投递同语义）
	DeliveryAtMostOnce DeliverySemantics = "at_most_once"
	// DeliveryDurableSubmit 起任务类：taskrunner 持久化凭证——事件重投=重复提交
	// 被回调幂等栅栏挡（at-least-once + 幂等消费）
	DeliveryDurableSubmit DeliverySemantics = "durable_submit"
)

// ActionContext 动作执行上下文（转换后/事件订阅两形态共用）。
type ActionContext struct {
	Ticket   *model.Ticket
	From, To string                   // 转换边（事件订阅形态=触发状态）
	ActorID  int64                    // 操作人（转换=当前用户；扫描=0）——审计归因
	Params   map[string]any           // post_actions 元素 params（对象形态）——动作自定义参数
	RawEvent *domainevent.DomainEvent // 事件订阅形态的原事件（可空）
	// SystemAsCreator 动作系统身份重放标记：post-function 是系统行为非用户行为——
	// cc/reassign 等写面动作以工单创建者身份执行（creator 恒有 update/assign 面），
	// ActorID 保留真实操作人供审计归因。
	SystemAsCreator bool
}

// TransitionAction 动作接口（A.3——编译期注册键 + 一致性声明）。
type TransitionAction interface {
	Name() string
	Delivery() DeliverySemantics
	Execute(ctx context.Context, ac ActionContext) error
}

// ActionRegistry 注册表（装配期写入运行期只读——jobs.Registry 同款）。
type ActionRegistry struct {
	mu       sync.RWMutex
	actions  map[string]TransitionAction
	maxDepth int                              // 循环触发链深限制（post_actions 级联上限——03 号防蔓延②）
	emitter  func(ev domainevent.DomainEvent) // G-5：失败事件出口（装配注入——nil=仅日志）
}

// SetEmitter G-5：注入失败事件出口（app 装配层调用——nil=仅日志不 emit）。
func (r *ActionRegistry) SetEmitter(fn func(ev domainevent.DomainEvent)) {
	r.emitter = fn
}

func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{actions: map[string]TransitionAction{}, maxDepth: 3}
}

// Register 注册动作（重复返回 error——架构守卫业务层禁裸 panic；
// app 装配层收到 err 再自行 fail-fast，与 jobs.Registry 的 panic 语义等价）。
func (r *ActionRegistry) Register(a TransitionAction) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.actions[a.Name()]; dup {
		return fmt.Errorf("actions: %q registered twice", a.Name())
	}
	r.actions[a.Name()] = a
	return nil
}

// Get 查动作。
func (r *ActionRegistry) Get(name string) (TransitionAction, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.actions[name]
	return a, ok
}

// Names 已注册动作名（排障/出站）。
func (r *ActionRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.actions))
	for k := range r.actions {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// actionRef post_actions 元素（字符串或 {name, params} 对象形态——A.1 随行件）。
type actionRef struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params"`
}

// parsePostActions 解析 post_actions 元素（两形态归一）。
func parsePostActions(tm *TransitionMeta) []actionRef {
	if tm == nil {
		return nil
	}
	out := make([]actionRef, 0, len(tm.PostActions))
	for _, raw := range tm.PostActions {
		var ref actionRef
		if err := json.Unmarshal(raw, &ref); err == nil && ref.Name != "" {
			out = append(out, ref)
			continue
		}
		var name string
		if err := json.Unmarshal(raw, &name); err == nil && name != "" {
			out = append(out, actionRef{Name: name})
		}
	}
	return out
}

// actionDepthKey ctx 携带的 post_actions 链深：Transition 读出当前深度，动作执行时 +1 透传——
// 动作内再触发 Transition（未来 transition 类动作）即形成 commit→post_actions→Transition 递归，
// 由 maxDepth 截断。修前调用方恒传 0 且无递归载体，maxDepth 守卫形同虚设（审计 #3）。
type actionDepthKey struct{}

// actionDepth 当前链深（无标记=0，顶层转换）。
func actionDepth(ctx context.Context) int {
	if d, ok := ctx.Value(actionDepthKey{}).(int); ok {
		return d
	}
	return 0
}

// withActionDepth 子动作 ctx（深度 +1）。
func withActionDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, actionDepthKey{}, depth)
}

// RunPostActions 执行转换后动作（Transition commit 后调用——决议：动作失败
// 不回滚转换[捕获+结构化日志]，链深 maxDepth 限循环级联——depth 来自 actionDepth(ctx)，
// 动作以 depth+1 的 ctx 执行，级联转换超深即截断并记日志）。
func (r *ActionRegistry) RunPostActions(ctx context.Context, logger *slog.Logger, tm *TransitionMeta, ac ActionContext, depth int) {
	if tm == nil {
		return
	}
	if depth > r.maxDepth {
		if logger != nil {
			logger.Warn("post_actions chain depth exceeded (cascade cut)",
				"ticket", ac.Ticket.ID, "depth", depth, "max", r.maxDepth)
		}
		return
	}
	actCtx := withActionDepth(ctx, depth+1)
	for _, ref := range parsePostActions(tm) {
		a, ok := r.Get(ref.Name)
		if !ok {
			continue // 未注册动作=配置漂移（保存期可校验）——运行期跳过
		}
		ac.Params = ref.Params
		if err := a.Execute(actCtx, ac); err != nil {
			// 失败语义（A.3）：不回滚转换——结构化日志（ops.action_failed 事件
			// 面登记触发驱动——当前无消费者先落日志）
			if logger != nil {
				logger.Warn("action failed (transition not rolled back)",
					"action", ref.Name, "ticket", ac.Ticket.ID, "err", err)
			}
			// G-5：emit ops.action_failed（A.3 可观测要求——S6 规则面可配告警路由）；
			// ActorID+payload.actor=收件人锚点（规则 recipient_type=creator 可路由到触发人；
			// 词表 2026-10-26 收口批补登记——inbox 面因 ops 域无 cc/assignee 锚点归 G-9 批）
			if r.emitter != nil {
				r.emitter(domainevent.DomainEvent{
					BizType: domainevent.BizOps, Action: domainevent.ActionOpsActionFailed,
					BizID: strconv.FormatInt(ac.Ticket.ID, 10), ActorID: ac.ActorID,
					Payload: domainevent.Payload{
						"action": ref.Name, "error": err.Error(),
						"ticket_id": strconv.FormatInt(ac.Ticket.ID, 10),
						"actor":     strconv.FormatInt(ac.ActorID, 10),
					},
					Timestamp: time.Now(),
				})
			}
		}
	}
}
