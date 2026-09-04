package resource

import (
	"context"
	"fmt"
	"sync"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
)

// Resource 资源接口，每个业务模块实现并自注册（Phase 2+）
type Resource interface {
	Code() string
	Name() string
	Actions() []string
	Authorize(ctx context.Context, req AuthorizeRequest) (bool, error)
	GetFilter(ctx context.Context, userID int64, action string) (Filter, error)
}

// AuthorizeRequest 统一鉴权请求
type AuthorizeRequest struct {
	UserID     int64
	Roles      []string
	Action     string
	ResourceID string
	Context    map[string]any
}

// Filter 列表过滤条件
type Filter struct {
	Where string
	Args  []any
	// Unscoped 显式声明本次查询不做行级 scope 过滤（admin bypass / ticket_scope=all /
	// 系统任务）。零值 Filter{} 不是合法的「全量」：无谓词且未豁免会被 repo 入口的
	// fail-closed 哨兵拒绝（IW4，2026-09-01——防漏接 L2 过滤导致静默全量）。
	Unscoped bool
}

// EvalEntry 判定日志行（B11①，03-audit-l2 §3.2）——registry.Authorize 统一埋点产出。
type EvalEntry struct {
	ActorID      int64
	ActorRoles   []string
	ResourceType string // 资源 code（ticket / builtin:task / ...）
	ResourceID   string
	Action       string
	Result       bool
	Reason       string // 拒绝原因（denied / error:...，已截断 200）
	TraceID      string // = request_id（reqid.From(ctx)）
}

// EvalHook 判定埋点钩子。实现必须非阻塞（fail-open：判定日志绝不阻断鉴权）。
type EvalHook func(ctx context.Context, e EvalEntry)

// Registry 资源注册中心
type Registry interface {
	Register(res Resource)
	Get(code string) (Resource, bool)
	List() []Resource
	Authorize(ctx context.Context, resourceCode string, req AuthorizeRequest) (bool, error)
	GetFilter(ctx context.Context, resourceCode string, userID int64, action string) (Filter, error)
	// SetEvalHook 挂判定埋点（app 装配时调用一次；之后 Authorize 每次判定回调）
	SetEvalHook(h EvalHook)
}

type registry struct {
	mu        sync.RWMutex
	resources map[string]Resource
	evalHook  EvalHook
}

// NewRegistry 创建空 Registry（Phase 1 不注册业务 Resource）
func NewRegistry() Registry {
	return &registry{resources: make(map[string]Resource)}
}

func (r *registry) Register(res Resource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resources[res.Code()] = res
}

func (r *registry) Get(code string) (Resource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res, ok := r.resources[code]
	return res, ok
}

func (r *registry) List() []Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Resource, 0, len(r.resources))
	for _, res := range r.resources {
		out = append(out, res)
	}
	return out
}

func (r *registry) SetEvalHook(h EvalHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evalHook = h
}

// evalReason 组装拒绝/错误原因（截断 200，对齐 DDL reason 列宽）。
func evalReason(allowed bool, err error) string {
	switch {
	case err != nil:
		return truncate("error: "+err.Error(), 200)
	case !allowed:
		return "denied"
	default:
		return ""
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func (r *registry) Authorize(ctx context.Context, code string, req AuthorizeRequest) (bool, error) {
	res, ok := r.Get(code)
	if !ok {
		return false, fmt.Errorf("resource %s not registered", code)
	}
	allowed, err := res.Authorize(ctx, req)
	// B11① 判定埋点：允许/拒绝/错误一律一行（允许行也是审计面——"谁被授权了什么"）。
	// hook 非阻塞义务由实现方保证（PolicyEvalWriter.Write 为非阻塞 channel 发送）。
	r.mu.RLock()
	hook := r.evalHook
	r.mu.RUnlock()
	if hook != nil {
		hook(ctx, EvalEntry{
			ActorID:      req.UserID,
			ActorRoles:   req.Roles,
			ResourceType: code,
			ResourceID:   req.ResourceID,
			Action:       req.Action,
			Result:       allowed && err == nil,
			Reason:       evalReason(allowed, err),
			TraceID:      reqid.From(ctx),
		})
	}
	return allowed, err
}

func (r *registry) GetFilter(ctx context.Context, code string, userID int64, action string) (Filter, error) {
	res, ok := r.Get(code)
	if !ok {
		return Filter{}, fmt.Errorf("resource %s not registered", code)
	}
	return res.GetFilter(ctx, userID, action)
}
