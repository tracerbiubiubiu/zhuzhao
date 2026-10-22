// HookRegistry 订阅注册表 + 派发器（06 号 A.1——按 biz_type 路由、多订阅者、
// 每订阅者独立 goroutine + recover 隔离、at-most-once）。
package domainevent

import (
	"context"
	"log/slog"
	"sort"
	"sync"
)

// HookRegistry 进程内事件注册表（零依赖——slog 可选注入，nil=NOP logger）。
type HookRegistry struct {
	mu     sync.RWMutex
	subs   map[BizType][]DomainHook
	all    []DomainHook // 全域订阅者（"*" 通配）
	logger *slog.Logger
}

// NewHookRegistry 创建注册表。
func NewHookRegistry(logger *slog.Logger) *HookRegistry {
	if logger == nil {
		logger = slog.Default()
	}
	return &HookRegistry{
		subs:   make(map[BizType][]DomainHook),
		logger: logger,
	}
}

// Subscribe 按业务域订阅（同域多订阅者按注册序派发——order 字段不引入，
// 注册序即声明序，消费方间无依赖）。
func (r *HookRegistry) Subscribe(biz BizType, hook DomainHook) {
	if hook == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subs[biz] = append(r.subs[biz], hook)
}

// SubscribeAll 全域订阅（通配 "*"——跨域消费者如审计/调试面）。
func (r *HookRegistry) SubscribeAll(hook DomainHook) {
	if hook == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.all = append(r.all, hook)
}

// Dispatch 派发事件——每订阅者独立 goroutine + recover 隔离（at-most-once：
// 派发后进程崩溃即丢一条——通知低一致性接受，phase6 高一致性走对账扫描）。
// 调用方须在事务提交后调用（06 号 §5.2 异步投递——发送失败不阻塞不回滚）。
func (r *HookRegistry) Dispatch(ctx context.Context, ev DomainEvent) {
	r.mu.RLock()
	targeted := make([]DomainHook, len(r.subs[ev.BizType]))
	copy(targeted, r.subs[ev.BizType])
	wild := make([]DomainHook, len(r.all))
	copy(wild, r.all)
	r.mu.RUnlock()

	for _, h := range append(targeted, wild...) {
		go func(hook DomainHook) {
			defer func() {
				if rec := recover(); rec != nil {
					r.logger.Error("DomainHook subscriber panicked",
						"biz_type", ev.BizType, "action", ev.Action, "biz_id", ev.BizID,
						"panic", rec)
				}
			}()
			hook.OnDomainEvent(ctx, ev)
		}(h)
	}
}

// SubscriberCount 返回指定域的订阅者数（测试/诊断面）。
func (r *HookRegistry) SubscriberCount(biz BizType) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.subs[biz])
}

// SortedBizTypes 返回已注册的域列表（诊断面——排序保输出稳定）。
func (r *HookRegistry) SortedBizTypes() []BizType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]BizType, 0, len(r.subs))
	for t := range r.subs {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
	return types
}
