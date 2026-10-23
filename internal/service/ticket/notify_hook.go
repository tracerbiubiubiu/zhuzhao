// TicketNotifyHook 工单事件→模板→渲染→发送的触发挂接（S6 B6-5——06 号 §5.2a）。
//
// 订阅 DomainEvent(ticket)，按 Action 选种子模板 → msgrender 占位符渲染 →
// ChannelSender 发送（当前=UIP 两态桩）。异步投递（06 号 §5.2——发送失败
// 不阻塞不回滚工单事务，at-most-once）。
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
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/msgrender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// NotifyDeadLetterSender 死信告警接口（06 号 §5.4——ChannelSender.Send 终败
// 时进程内直调；与 /internal/notify/dead-letter HTTP handler 共用内部函数）。
type NotifyDeadLetterSender interface {
	NotifyDeadLetter(ctx context.Context, p *model.DeadLetterNotifyPayload) error
}

// TicketNotifyHook 工单通知钩子（实现 DomainHook——06 号 拍板点 4 消费方侧
// 名字保持业务化）。
type TicketNotifyHook struct {
	tplRepo   *repository.MessageTemplateRepo
	ruleRepo  *repository.MessageRuleRepo
	logRepo   *repository.MessageLogRepo
	optRepo   *repository.NotificationOptoutRepo
	throttle  *notifyThrottle
	notifySvc NotifyDeadLetterSender
	sender    channelsender.ChannelSender
	logger    *slog.Logger
}

// NewTicketNotifyHook 创建工单通知钩子。
func NewTicketNotifyHook(
	tplRepo *repository.MessageTemplateRepo,
	ruleRepo *repository.MessageRuleRepo,
	logRepo *repository.MessageLogRepo,
	optRepo *repository.NotificationOptoutRepo,
	sender channelsender.ChannelSender,
	logger *slog.Logger,
	notifySvc NotifyDeadLetterSender,
) *TicketNotifyHook {
	if logger == nil {
		logger = slog.Default()
	}
	return &TicketNotifyHook{tplRepo: tplRepo, ruleRepo: ruleRepo, logRepo: logRepo, optRepo: optRepo, sender: sender, logger: logger, throttle: newNotifyThrottle(60 * time.Second), notifySvc: notifySvc}
}

// OnDomainEvent DomainHook 实现——工单事件触发通知（每订阅者独立 goroutine
// + recover 由 HookRegistry 保证；此方法内不再 panic 防）。
func (h *TicketNotifyHook) OnDomainEvent(ctx context.Context, ev domainevent.DomainEvent) {
	if ev.BizType != domainevent.BizTicket {
		return // 非 ticket 域不处理
	}
	// B6-6：message_rules 路由（替代硬编码映射——管理员可改）
	rule, err := h.ruleRepo.GetByAction(ctx, string(ev.BizType), string(ev.Action))
	if err != nil {
		h.logger.Warn("notify: rule lookup failed", "action", ev.Action, "err", err)
		return
	}
	if rule == nil {
		return // 无规则=不通知（updated/commented 等）
	}
	tplCode := rule.TemplateCode

	// 同单连发节流（06 号 §5.2 投递语义补遗——防状态翻转通知风暴）：
	// 同 (biz_type, biz_id, action) 在窗口内只发一次；窗口外自然过期。
	if h.throttle.shouldSkip(string(ev.BizType), ev.BizID, string(ev.Action)) {
		h.logger.Info("notify: throttled (same biz+action in window)",
			"action", ev.Action, "biz_id", ev.BizID)
		return
	}

	// ① 查模板
	tpl, err := h.tplRepo.GetByCode(ctx, tplCode)
	if err != nil {
		h.logger.Warn("notify: template not found", "action", ev.Action, "code", tplCode, "err", err)
		return
	}

	// ② 组装变量（event payload 即变量源——ticket.* 前缀）
	vars := map[string]any{
		"ticket": buildTicketVars(ev),
	}

	// ③ 渲染
	subject, body := msgrender.RenderBoth(tpl.Subject, tpl.Body, vars)

	// ④ 解析收件人（第一档=工单字段引用——06 号 §5.1）
	recipients := resolveRecipients(ev)
	if len(recipients) == 0 {
		h.logger.Info("notify: no recipients resolved", "action", ev.Action, "biz_id", ev.BizID)
		return
	}

	// B6-4 偏好过滤（06 号 §5.1——收件人解析后、Send 前查 opt-out/免打扰）
	filtered := make([]string, 0, len(recipients))
	for _, rcpt := range recipients {
		// rcpt=字符串化用户 ID——查 notification_optouts
		var uid int64
		if _, err := fmt.Sscan(rcpt, &uid); err == nil && uid > 0 && h.optRepo != nil {
			if opted, _ := h.optRepo.IsOptedOut(ctx, uid, string(ev.BizType), string(ev.Action)); opted {
				h.logger.Info("notify: recipient opted out", "user", rcpt, "action", ev.Action)
				continue
			}
		}
		filtered = append(filtered, rcpt)
	}
	if len(filtered) == 0 {
		h.logger.Info("notify: all recipients opted out", "action", ev.Action, "biz_id", ev.BizID)
		return
	}
	recipients = filtered

	// ⑤ 发送（ChannelSender——当前=UIP 两态桩）
	err = h.sender.Send(ctx, channelsender.Message{
		Recipients:   recipients,
		Subject:      subject,
		Body:         body,
		BizType:      string(ev.BizType),
		BizID:        ev.BizID,
		TemplateCode: tpl.Code,
	})
	status := "sent"
	sendErr := ""
	if err != nil {
		status = "failed"
		sendErr = err.Error()
		h.logger.Error("notify: send failed",
			"action", ev.Action, "biz_id", ev.BizID, "template", tpl.Code, "err", err)
		// 06 号 §5.4：Send 终败→进程内直调死信告警（与 /internal/notify/dead-letter 共用）
		if h.notifySvc != nil {
			_ = h.notifySvc.NotifyDeadLetter(ctx, &model.DeadLetterNotifyPayload{
				Action: "channel_send_failed",
				Error:  sendErr,
				Dept:   string(ev.BizType), // biz_type 复用 Dept 字段（载荷结构对齐 taskrunner 既有投递）
			})
		}
		// 不 return error——at-most-once（06 号 §5.2 异步投递，失败不阻塞）
	} else {
		h.logger.Info("notify: sent",
			"action", ev.Action, "biz_id", ev.BizID, "template", tpl.Code,
			"recipients", fmt.Sprint(recipients))
	}
	// B6-6：发送记录落行（成功/失败均记录——06 号 §5.4）
	_ = h.logRepo.Insert(ctx, &model.MessageLog{
		TemplateCode: tpl.Code, BizType: string(ev.BizType), BizID: ev.BizID,
		Channel: h.sender.Channel(), Recipients: strings.Join(recipients, ","),
		Subject: subject, Body: body, Status: status, Error: sendErr,
	})
}

// buildTicketVars 从事件载荷组装 ticket.* 变量树。
func buildTicketVars(ev domainevent.DomainEvent) map[string]any {
	vars := map[string]any{}
	for k, v := range ev.Payload {
		vars[k] = v
	}
	// 基础兜底（payload 缺失时占位符保留原样）
	if _, ok := vars["human_key"]; !ok {
		vars["human_key"] = ev.BizID // human_key 未实现前用 id
	}
	if _, ok := vars["link"]; !ok {
		vars["link"] = fmt.Sprintf("/ticket/detail/%s", ev.BizID)
	}
	return vars
}

// resolveRecipients 第一档收件人解析（06 号 §5.1——工单字段引用+固定人员）。
// 当前实现：assigned→to_assignee；created→actor（创建人=发起人）。
func resolveRecipients(ev domainevent.DomainEvent) []string {
	var ids []string
	switch ev.Action {
	case domainevent.TicketAssigned:
		// to_assignee 是用户 ID 的字符串化（strconv.FormatInt 产物——06 号 §5.1
		// 第一档工单字段引用；真实工号解析待 B6-4 完整实现）
		if to, ok := ev.Payload["to_assignee"].(string); ok && to != "" {
			ids = append(ids, to)
		}
	case domainevent.TicketCreated:
		// 创建通知→发起人本人（无意义？——生产态按 message_rules 配置收件人）
		// 首版暂不送 created 通知（避免自己通知自己）
		return nil
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
