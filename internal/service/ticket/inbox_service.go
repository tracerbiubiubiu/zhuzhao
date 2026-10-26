// S6 二期 B6-7 站内消息中心服务+端点（06 号 §6/附录 A.8.5）：
// inbox 通道动作（DomainEvent→notifications 落库——入站即渲染）+ 收件箱读面
// （分页/单条已读/全部已读）+ 未读数并入 badges sections + 保留策略清理。
package ticket

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// InboxService 站内消息中心。
type InboxService struct {
	repo          *repository.NotificationInboxRepo
	eventReg      *domainevent.HookRegistry
	retentionDays int
	logger        *slog.Logger
}

// NewInboxService 创建并订阅事件（inbox=事件系统消费者——一次订阅全事件面；
// 落库即消费者自身的持久化，非 ChannelSender 外发）。
func NewInboxService(db *pgxpool.Pool, retentionDays int, logger *slog.Logger) *InboxService {
	if logger == nil {
		logger = slog.Default()
	}
	if retentionDays <= 0 {
		retentionDays = 90 // 缺省 90 天封顶（06 号 §6 保留策略）
	}
	return &InboxService{repo: repository.NewNotificationInboxRepo(db), retentionDays: retentionDays, logger: logger}
}

// SetEventRegistry 注入事件注册表并完成双域订阅（app 装配层调用——eventRegistry
// 实例在 NewApp 局部作用域，wire_gen 不可达——Set 惯例同 SetEventRegistry[Service]）。
func (s *InboxService) SetEventRegistry(reg *domainevent.HookRegistry) *InboxService {
	s.eventReg = reg
	if reg != nil {
		reg.Subscribe(domainevent.BizTicket, "inbox.notify", s)
		reg.Subscribe(domainevent.BizOps, "inbox.ops", s)
	}
	return s
}

// seedTemplates B6-7 入站渲染摘要（biz_type+action → title 模板；06 号 A.8.5
// 消息行=摘要+时间+深链——读面零渲染的代价是入站渲染面在此集中）。
// seedTemplates 入站渲染词表（审计 P3-2：注释约束勿运行时写——Go 无 const map
// 惯例，防御=不变量约定+代码评审；新事件动作须同步登记此表否则站内静默不落库）
var seedTemplates = map[string]string{ //nolint:gochecknoglobals // 装配期初始化运行期只读
	"ticket.created":         "新工单：{{title}}",
	"ticket.assigned":        "工单已分派给您：{{title}}",
	"ticket.closed":          "工单已关闭：{{title}}",
	"ticket.updated":         "工单已更新：{{title}}",
	"ticket.commented":       "工单有新回复：{{title}}",
	"ticket.cc_added":        "工单抄送给您：{{title}}",
	"ticket.timeout":         "工单超时催办：{{title}}",
	"ticket.transitioned":    "工单状态变更：{{title}}",
	"ticket.dispatch_missed": "工单待分派：{{title}}",
	"ticket.deadline":        "工单已到处理期限：{{title}}",
	"ops.cron_failed":        "定时任务失败：{{job}}",
}

// OnDomainEvent DomainHook 实现——事件→站内通知落库。
// 收件人解析：payload 的 to_assignee/cc_users（S6 一期同锚点）；无收件人不落库。
func (s *InboxService) OnDomainEvent(ctx context.Context, ev domainevent.DomainEvent) {
	tpl, ok := seedTemplates[string(ev.Action)]
	if !ok {
		return // 未登记动作不落站内（词表面收敛——防全量事件灌inbox）
	}
	recipients := collectRecipients(ev)
	if len(recipients) == 0 {
		return
	}
	title := renderTitle(tpl, ev)
	link := inboxLink(ev)
	for _, rid := range recipients {
		var bid *int64
		if id, err := strconv.ParseInt(ev.BizID, 10, 64); err == nil {
			bid = &id
		}
		row := &model.NotificationRow{
			RecipientID: rid, BizType: string(ev.BizType), BizID: bid,
			Title: title, Link: link,
		}
		if err := s.repo.Insert(ctx, row); err != nil {
			s.logger.Warn("inbox: insert failed", "action", ev.Action, "recipient", rid, "err", err)
		}
	}
}

// selfNoiseActions 操作人即内容生产者的动作——不给 actor 自己落站内信（处理人编辑/评论自己的单
// 又收到「工单已更新」=纯噪音，审计 #8）。assigned/cc_added 等保留 actor 可为收件人（自派/「抄送我」
// 产生确认——TestB67 既定语义）；全局 actor 排除与接 message_rules 路由属产品拍板（台账 G-7 并账）。
var selfNoiseActions = map[domainevent.Action]bool{
	domainevent.TicketUpdated:   true,
	domainevent.TicketCommented: true,
}

// collectRecipients 收件人解析（to_assignee ∪ cc_users——S6 一期同锚点；去重）。
func collectRecipients(ev domainevent.DomainEvent) []int64 {
	seen := map[int64]bool{}
	skipActor := selfNoiseActions[ev.Action]
	var out []int64
	add := func(s string) {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 || seen[id] || (skipActor && id == ev.ActorID) {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	if to, ok := ev.Payload["to_assignee"].(string); ok {
		add(to)
	}
	if cc, ok := ev.Payload["cc_users"].(string); ok {
		for _, id := range splitCSV(cc) {
			add(id)
		}
	}
	// creator 档（updated/commented 类——actor 是操作人不是收件人；created_by 需查库——
	// 一期收件人面无 created_by 键，站内一期只覆盖 to_assignee/cc_users 两锚点+ops 域跳过）
	return out
}

// titleVar 标题模板占位 {{key}}（单词字符键——payload 浅层键名）。
var titleVar = regexp.MustCompile(`\{\{(\w+)\}\}`)

// renderTitle 入站渲染（{{key}} 单层占位替换——payload 浅层；缺键保留原样）。
// 单次扫描：替换入的值不再被重新扫描——修前按 payload map 逐键 ReplaceAll（遍历随机序），
// 工单标题含 "{{human_key}}"/"{{cc_users}}" 字面即可能被二次展开（msgrender 有同款防护，此处平行实现曾缺）。
func renderTitle(tpl string, ev domainevent.DomainEvent) string {
	return titleVar.ReplaceAllStringFunc(tpl, func(m string) string {
		k := m[2 : len(m)-2]
		if v, ok := ev.Payload[k]; ok {
			return fmt.Sprint(v)
		}
		return m
	})
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// inboxLink 深链路由（payload 路由既定——ticket 域 → /tickets/:id；ops → /tasks）。
func inboxLink(ev domainevent.DomainEvent) string {
	switch ev.BizType {
	case domainevent.BizTicket:
		return "/tickets/" + ev.BizID
	case domainevent.BizOps:
		return "/tasks"
	}
	return ""
}

// ─── 读面（handler 消费）───

// Inbox 收件箱分页。
func (s *InboxService) Inbox(ctx context.Context, recipientID int64, unreadOnly bool, page, pageSize int) ([]*model.NotificationRow, int64, error) {
	return s.repo.Inbox(ctx, recipientID, unreadOnly, page, pageSize)
}

// MarkRead 单条已读。
func (s *InboxService) MarkRead(ctx context.Context, id, recipientID int64) error {
	return s.repo.MarkRead(ctx, id, recipientID)
}

// MarkAllRead 全部已读。
func (s *InboxService) MarkAllRead(ctx context.Context, recipientID int64) (int64, error) {
	return s.repo.MarkAllRead(ctx, recipientID)
}

// UnreadCount 未读数（badges sections.unread）。
func (s *InboxService) UnreadCount(ctx context.Context, recipientID int64) (int64, error) {
	return s.repo.UnreadCount(ctx, recipientID)
}

// CleanupExpired 保留策略（骑 CronScheduler——Reentrant 幂等）。
func (s *InboxService) CleanupExpired(ctx context.Context) error {
	n, err := s.repo.CleanupExpired(ctx, s.retentionDays)
	if err != nil {
		return err
	}
	if n > 0 {
		s.logger.Info("inbox: expired notifications purged", "rows", n)
	}
	return nil
}
