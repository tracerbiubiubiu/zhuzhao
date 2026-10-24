// TicketNotifyHook 事件→通知的触发挂接（S6 B6-5——06 号 §5.2a）。
//
// B10-4（B-6 销账）：编排链抽出到 MessageDispatcher（06 号 §1.5 三入口公共
// 前置），本类型=薄壳（域过滤 + 转发），消费方侧名字保持业务化（06 号拍板点 4）。
// 订阅 ticket + ops 双域（10 号 A.5：cron 失败告警经同一渲染→发送→message_log 链）。
package ticket

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/channelsender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// NotifyDeadLetterSender 死信告警接口（06 号 §5.4——ChannelSender.Send 终败
// 时进程内直调；与 /internal/notify/dead-letter HTTP handler 共用内部函数）。
type NotifyDeadLetterSender interface {
	NotifyDeadLetter(ctx context.Context, p *model.DeadLetterNotifyPayload) error
}

// TicketNotifyHook 事件通知钩子（实现 DomainHook——薄壳）。
type TicketNotifyHook struct {
	d *MessageDispatcher
}

// NewTicketNotifyHook 创建事件通知钩子（内部组装 MessageDispatcher 编排链——
// 签名不变，既有装配/测试零改）。
func NewTicketNotifyHook(
	tplRepo *repository.MessageTemplateRepo,
	ruleRepo *repository.MessageRuleRepo,
	logRepo *repository.MessageLogRepo,
	optRepo *repository.NotificationOptoutRepo,
	sender channelsender.ChannelSender,
	logger *slog.Logger,
	notifySvc NotifyDeadLetterSender,
) *TicketNotifyHook {
	return &TicketNotifyHook{d: NewMessageDispatcher(tplRepo, ruleRepo, logRepo, optRepo, sender, logger, notifySvc)}
}

// OnDomainEvent DomainHook 实现——ticket/ops 双域事件触发通知（每订阅者独立
// goroutine + recover 由 HookRegistry 保证；此方法内不再 panic 防）。
func (h *TicketNotifyHook) OnDomainEvent(ctx context.Context, ev domainevent.DomainEvent) {
	if ev.BizType != domainevent.BizTicket && ev.BizType != domainevent.BizOps {
		return // 非 ticket/ops 域不处理
	}
	h.d.Dispatch(ctx, ev)
}

// buildEventVars 从事件载荷组装变量树（根键按域：ticket→"ticket"、其余→"event"
// ——ops 告警模板 {{event.job}} 形态）。ticket 域带基础兜底（缺失占位符保留原样）。
func buildEventVars(ev domainevent.DomainEvent) map[string]any {
	root := "event"
	if ev.BizType == domainevent.BizTicket {
		root = "ticket"
	}
	inner := map[string]any{}
	for k, v := range ev.Payload {
		inner[k] = v
	}
	if ev.BizType == domainevent.BizTicket {
		if _, ok := inner["human_key"]; !ok {
			inner["human_key"] = ev.BizID
		}
		if _, ok := inner["link"]; !ok {
			inner["link"] = fmt.Sprintf("/ticket/detail/%s", ev.BizID)
		}
	}
	return map[string]any{root: inner}
}

// resolveRecipientsFromRule 按 message_rules 的收件人配置解析。
// recipient_type: assignee→事件 payload to_assignee；creator→actor；fixed→DDL 列表；
// cc→payload cc_users（S10 B10-1——ticket.cc_added 的本次新增抄送人，逗号分隔）。
func resolveRecipientsFromRule(ev domainevent.DomainEvent, recipientType, fixedRecipients string) []string {
	var ids []string
	switch recipientType {
	case "assignee":
		if to, ok := ev.Payload["to_assignee"].(string); ok && to != "" {
			ids = append(ids, to)
		}
	case "creator":
		if actor, ok := ev.Payload["actor"].(string); ok && actor != "" {
			ids = append(ids, actor)
		}
	case "cc":
		if cc, ok := ev.Payload["cc_users"].(string); ok {
			for _, id := range strings.Split(cc, ",") {
				if id = strings.TrimSpace(id); id != "" {
					ids = append(ids, id)
				}
			}
		}
	case "fixed":
		for _, id := range strings.Split(fixedRecipients, ",") {
			if id = strings.TrimSpace(id); id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// notifyThrottle 同单连发节流器（06 号 §5.2——防状态翻转通知风暴）。
// 进程内 sync.Map + 时间窗：同 (biz, id, action) 在 window 内只放行首次。
// 单实例部署形态足够（多副本升 Redis——06 号 §5.2 注记）。
type notifyThrottle struct {
	mu     sync.Mutex
	seen   map[string]time.Time
	window time.Duration
}

func newNotifyThrottle(window time.Duration) *notifyThrottle {
	return &notifyThrottle{seen: map[string]time.Time{}, window: window}
}

// shouldSkip 返回 true=窗口内重复（跳过）；false=首次或已过期（放行并记时间戳）。
func (t *notifyThrottle) shouldSkip(bizType, bizID, action string) bool {
	key := bizType + ":" + bizID + ":" + action
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	// 顺手清理过期项（防 map 无限增长——量级=工单×动作，天然有界但保守清理）
	for k, ts := range t.seen {
		if now.Sub(ts) > t.window {
			delete(t.seen, k)
		}
	}
	if ts, ok := t.seen[key]; ok && now.Sub(ts) < t.window {
		return true
	}
	t.seen[key] = now
	return false
}
