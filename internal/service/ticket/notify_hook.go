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

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/channelsender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/msgrender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// TicketNotifyHook 工单通知钩子（实现 DomainHook——06 号 拍板点 4 消费方侧
// 名字保持业务化）。
type TicketNotifyHook struct {
	tplRepo  *repository.MessageTemplateRepo
	ruleRepo *repository.MessageRuleRepo
	logRepo  *repository.MessageLogRepo
	sender   channelsender.ChannelSender
	logger   *slog.Logger
}

// NewTicketNotifyHook 创建工单通知钩子。
func NewTicketNotifyHook(
	tplRepo *repository.MessageTemplateRepo,
	ruleRepo *repository.MessageRuleRepo,
	logRepo *repository.MessageLogRepo,
	sender channelsender.ChannelSender,
	logger *slog.Logger,
) *TicketNotifyHook {
	if logger == nil {
		logger = slog.Default()
	}
	return &TicketNotifyHook{tplRepo: tplRepo, ruleRepo: ruleRepo, logRepo: logRepo, sender: sender, logger: logger}
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
