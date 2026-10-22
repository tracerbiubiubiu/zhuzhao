// Package domainevent 进程内领域事件基建（S6 B6-1——06 号 A.1 契约；
// S6/phase6/S10 共享基础设施：一次状态变更、多订阅者，禁硬编码消费方）。
//
// 设计要点（06 号 §5.2a + dd §29）：
//   - DomainEvent 按 biz_type 路由（ticket/risk/…——域无关命名）
//   - DomainHook 每订阅者独立 goroutine + recover 隔离（一个炸不连坐）
//   - at-most-once（通知=低一致性消费者，丢失无害可补发——dd §29 分级）
//   - Dispatch 在事务提交后调用（异步投递——发送失败不阻塞不回滚工单事务）
//   - 消费方侧名字保持业务化（如 TicketNotifyHook——06 号拍板点 4）
package domainevent

import (
	"context"
	"fmt"
	"time"
)

// BizType 业务域标识（路由键——06 号拍板点 4 通用命名）。
type BizType string

const (
	BizTicket BizType = "ticket" // 工单域（S6 触发/S10 超时/phase6 R3 消费）
	// 后续按需新增（risk/task/…）——包不预注册消费方
)

// Action 事件动作（biz_type.verb 固定词表——06 号 A.1 事件分类学）。
type Action string

const (
	// ticket 域动作词表
	TicketCreated   Action = "ticket.created"
	TicketClosed    Action = "ticket.closed"
	TicketAssigned  Action = "ticket.assigned"
	TicketUpdated   Action = "ticket.updated"
	TicketCommented Action = "ticket.commented"
)

// Payload 事件载荷——按 Action 预留 payload schema 登记位（06 号 A.1）。
// D20：updated 类载荷带 changes[]（键级 diff——B6-1 定稿）。
type Payload map[string]any

// Change 键级变更（updated 类事件的载荷内容）。
type Change struct {
	Key    string `json:"key"`
	OldVal any    `json:"old_val,omitempty"`
	NewVal any    `json:"new_val,omitempty"`
}

// DomainEvent 领域事件（06 号 A.1 接口全文）。
type DomainEvent struct {
	BizType   BizType   `json:"biz_type"`
	Action    Action    `json:"action"`
	BizID     string    `json:"biz_id"`   // 业务对象 ID（工单 id/风险 id）
	ActorID   int64     `json:"actor_id"` // 操作人（审计归因）
	Payload   Payload   `json:"payload"`  // 按 Action 的载荷（changes/status/title…）
	Timestamp time.Time `json:"timestamp"`
}

// String 人类可读描述（日志/调试）。
func (e DomainEvent) String() string {
	return fmt.Sprintf("%s#%s(%s) actor=%d at=%s", e.BizType, e.Action, e.BizID, e.ActorID, e.Timestamp.Format(time.RFC3339))
}

// DomainHook 订阅者接口——消费方实现此接口并经 Subscribe 注册。
// 消费方侧名字保持业务化（如 TicketNotifyHook）。
type DomainHook interface {
	OnDomainEvent(ctx context.Context, ev DomainEvent)
}

// HookFunc 函数适配器（轻量订阅场景——匿名函数即订阅）。
type HookFunc func(ctx context.Context, ev DomainEvent)

func (f HookFunc) OnDomainEvent(ctx context.Context, ev DomainEvent) { f(ctx, ev) }
