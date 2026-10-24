// MessageDispatcher 事件→通知编排链（S10 B10-4 抽出——06 号 §1.5 三入口公共
// 前置；B-6 挂账销账件）：rules 路由 → 模板 → 渲染（四象限转义）→ 收件人解析
// → 偏好过滤 → 节流 → ChannelSender 发送 → message_log 落行 + 终败死信告警。
// 消费方：TicketNotifyHook（DomainHook 入口——ticket/ops 双域）；行为与抽出前
// 零变化（原 notify_hook 内联链整体迁入）。
package ticket

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/channelsender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/msgrender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// MessageDispatcher 消息编排器。
type MessageDispatcher struct {
	tplRepo   *repository.MessageTemplateRepo
	ruleRepo  *repository.MessageRuleRepo
	logRepo   *repository.MessageLogRepo
	optRepo   *repository.NotificationOptoutRepo
	throttle  *notifyThrottle
	notifySvc NotifyDeadLetterSender
	sender    channelsender.ChannelSender
	logger    *slog.Logger
}

// NewMessageDispatcher 创建消息编排器。
func NewMessageDispatcher(
	tplRepo *repository.MessageTemplateRepo,
	ruleRepo *repository.MessageRuleRepo,
	logRepo *repository.MessageLogRepo,
	optRepo *repository.NotificationOptoutRepo,
	sender channelsender.ChannelSender,
	logger *slog.Logger,
	notifySvc NotifyDeadLetterSender,
) *MessageDispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &MessageDispatcher{tplRepo: tplRepo, ruleRepo: ruleRepo, logRepo: logRepo, optRepo: optRepo, sender: sender, logger: logger, throttle: newNotifyThrottle(60 * time.Second), notifySvc: notifySvc}
}

// Dispatch 单事件编排（规则路由→渲染→发送→落行）。
func (h *MessageDispatcher) Dispatch(ctx context.Context, ev domainevent.DomainEvent) {
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

	// ① 查模板
	tpl, err := h.tplRepo.GetByCode(ctx, tplCode)
	if err != nil {
		h.logger.Warn("notify: template not found", "action", ev.Action, "code", tplCode, "err", err)
		return
	}

	// channel 错配运行时守卫（审计 P1-2 + 06 §3 F-3-2 两级渠道词表）：channel=
	// adapter 层，白名单保留 uip/webhook，但一期仅 uip 单 sender——错配模板
	// （如 webhook）显式失败并落 failed 行，防静默错发（修复前：照发 UIP 桩+
	// log 记 sent+渠道失真）。路由层（map[channel]sender）落地后此守卫自然
	// 变 no-op——触发驱动（首个非 uip 模板真实需求，~0.5 天）。
	if tpl.Channel != h.sender.Channel() {
		h.logger.Error("notify: channel mismatch — no adapter route",
			"action", ev.Action, "biz_id", ev.BizID, "template", tpl.Code,
			"tpl_channel", tpl.Channel, "sender_channel", h.sender.Channel())
		_ = h.logRepo.Insert(ctx, &model.MessageLog{
			TemplateCode: tpl.Code, BizType: string(ev.BizType), BizID: ev.BizID,
			Channel: tpl.Channel, Status: "failed",
			Error: fmt.Sprintf("channel mismatch: template=%s sender=%s (adapter route not enabled)",
				tpl.Channel, h.sender.Channel()),
		})
		return
	}

	// ② 组装变量（event payload 即变量源——ticket.* 前缀）
	vars := buildEventVars(ev)

	// ③ 渲染（四象限三分档——B6-3 增强批：内置键缺失=fail 拒发/动态键=skip+warn）
	subjR := msgrender.RenderQuadrantEscaped(tpl.Subject, vars, tpl.Channel)
	bodyR := msgrender.RenderQuadrantEscaped(tpl.Body, vars, tpl.Channel)
	if subjR.Err != nil || bodyR.Err != nil {
		err := subjR.Err
		if err == nil {
			err = bodyR.Err
		}
		h.logger.Warn("notify: render rejected (builtin key missing)",
			"action", ev.Action, "biz_id", ev.BizID, "template", tpl.Code, "err", err)
		return
	}
	subject := subjR.Text
	body := bodyR.Text
	for _, w := range append(subjR.Warnings, bodyR.Warnings...) {
		h.logger.Info("notify: render warning", "detail", w, "action", ev.Action, "biz_id", ev.BizID)
	}

	// ④ 解析收件人（消费 message_rules 的 recipient_type/fixed_recipients——修复前旁路 DDL）
	recipients := resolveRecipientsFromRule(ev, rule.RecipientType, rule.FixedRecipients)
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

	// A-8 审计修复：节流移到偏好过滤后——全 opt-out 不占节流窗口
	if h.throttle.shouldSkip(string(ev.BizType), ev.BizID, string(ev.Action)) {
		h.logger.Info("notify: throttled (same biz+action in window)",
			"action", ev.Action, "biz_id", ev.BizID)
		return
	}

	// ⑤ 发送（ChannelSender——A-7 审计修复：ctx 加 5s 超时防 goroutine 永久阻塞）
	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = h.sender.Send(sendCtx, channelsender.Message{
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
