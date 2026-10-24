// Package jobs 预置动作注册表（E-②，16 号 §3；taskrunner 三层模型的「动作」层）。
//
// action_id 即跨仓库契约（taskrunner 仓库 docs/taskrunner.md §4）：taskrunner 按
// job 定义回调 zhuzhao POST /internal/jobs/callback（action_id 在 body，C10），本包查表分发执行。
// 新增动作 = 实现 Handler + Register（发版一次），任务定义在 taskrunner 侧运行时配置。
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// Handler 预置动作业务处理器。params 为任务定义/提交携带的业务参数（JSON 原样）。
//
// 错误语义（P7 定案：无响应体状态字段，业务失败映射 HTTP 状态码）：
//   - 返回 nil → 2xx（执行完全成功）；
//   - 返回 ErrAbort 或包装它的错误 → 4xx（不可重试，taskrunner 判 failed 终态）；
//   - 其他错误 → 5xx（可重试，taskrunner 按退避策略重试）。
//
// 幂等义务：at-least-once 语义下 taskrunner 可能重复回调；job_submissions 表在
// succeeded 后拦下重复回调，但「执行中途失败后重试」会再次进入 Handle——
// 对「导出 + 删除」类副作用动作，Handle 自身仍须可安全重入（B11② 语义）。
type Handler interface {
	Handle(ctx context.Context, params json.RawMessage) error
}

// HandlerFunc 函数适配。
type HandlerFunc func(ctx context.Context, params json.RawMessage) error

func (f HandlerFunc) Handle(ctx context.Context, params json.RawMessage) error { return f(ctx, params) }

// ErrAbort 不可重试的业务失败（→ 4xx）。可 wrap：fmt.Errorf("...: %w", jobs.ErrAbort)。
var ErrAbort = fmt.Errorf("jobs: non-retryable failure")

// ActionMeta 动作元数据（S4 B4-1——04 号附录 A.1：动作清单读端点供给）。
// 缺省允许（存量注册零破坏——前端对空 meta 降级显示 action_id）。
// ParamsSchema 钉 JSON Schema draft 2020-12 子集（与 S1 渲染器/AI 工具定义同语言）。
type ActionMeta struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	Kind         string          `json:"kind,omitempty"`          // 多执行端预留（handler/script/connector——2026-10-20 回灌）
	OwnerService string          `json:"owner_service,omitempty"` // 多执行端预留（当前恒 zhuzhao）
	ParamsSchema json.RawMessage `json:"params_schema,omitempty"` // nil=未声明（前端降级 JSON 模式）
}

// ActionInfo 动作清单出站行（A.1 契约——最小读面：不含参数快照/运行数据/凭据）。
type ActionInfo struct {
	ActionID string `json:"action_id"`
	ActionMeta
}

// Registry action_id → Handler 注册表（并发安全；app 装配期写入，运行期只读）。
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
	metas    map[string]ActionMeta
}

func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]Handler), metas: make(map[string]ActionMeta)}
}

// Register 注册动作；action_id 重复注册直接 panic（装配期错误，fail-fast）。
// meta 可变参缺省允许（存量调用零破坏）。
func (r *Registry) Register(actionID string, h Handler, meta ...ActionMeta) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.handlers[actionID]; dup {
		panic(fmt.Sprintf("jobs: action %q registered twice", actionID))
	}
	r.handlers[actionID] = h
	if len(meta) > 0 {
		r.metas[actionID] = meta[0]
	}
}

// Actions 动作清单（按 action_id 排序——出站稳定序）。
func (r *Registry) Actions() []ActionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ActionInfo, 0, len(r.handlers))
	for id := range r.handlers {
		out = append(out, ActionInfo{ActionID: id, ActionMeta: r.metas[id]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ActionID < out[j].ActionID })
	return out
}

// Get 查动作；未注册返回 false（P6 定案：不做前置校验，未知 action 经回调 404 快速失败）。
func (r *Registry) Get(actionID string) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[actionID]
	return h, ok
}

// ActionIDs 已注册动作 ID 清单（排障用；出站结构体清单见 Actions）。
func (r *Registry) ActionIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.handlers))
	for k := range r.handlers {
		out = append(out, k)
	}
	return out
}
