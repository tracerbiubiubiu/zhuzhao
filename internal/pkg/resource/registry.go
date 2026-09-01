package resource

import (
	"context"
	"fmt"
	"sync"
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

// Registry 资源注册中心
type Registry interface {
	Register(res Resource)
	Get(code string) (Resource, bool)
	List() []Resource
	Authorize(ctx context.Context, resourceCode string, req AuthorizeRequest) (bool, error)
	GetFilter(ctx context.Context, resourceCode string, userID int64, action string) (Filter, error)
}

type registry struct {
	mu        sync.RWMutex
	resources map[string]Resource
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

func (r *registry) Authorize(ctx context.Context, code string, req AuthorizeRequest) (bool, error) {
	res, ok := r.Get(code)
	if !ok {
		return false, fmt.Errorf("resource %s not registered", code)
	}
	return res.Authorize(ctx, req)
}

func (r *registry) GetFilter(ctx context.Context, code string, userID int64, action string) (Filter, error) {
	res, ok := r.Get(code)
	if !ok {
		return Filter{}, fmt.Errorf("resource %s not registered", code)
	}
	return res.GetFilter(ctx, userID, action)
}
