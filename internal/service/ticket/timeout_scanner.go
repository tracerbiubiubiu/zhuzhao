// S10 B10-4 节点超时扫描器（10 号 §3.4/附录 A.5）：骑 CronScheduler（D18 维护型
// 口径），逐类型解析 transition_meta 的 state 级 timeout_minutes → 候选查询
// （活跃单部分索引）→ 按 escalation 段催办水位判定 → Dispatch ticket.timeout
// （通知复用 S6 规则面——扫描器只发事件零新通知代码）。
//
// 收件人解析（E-1 拍定 2026-10-24——dd §31）：escalation.notify 目标 =
// assignee（处理人）∪ supervisor（**人工指定 escalation_supervisor_id 优先**；
// users.manager_id 档挂 M-HR 后接入；未指定=缺席不推导）。载荷锚点：
// to_assignee + cc_users=全部目标（种子规则 recipient_type=cc 一条规则全员可达）。
//
// ⚠ 落点注：03 号 A.6 engine 子包化（S3a B3-1）后本文件收拢至
// internal/service/ticket/engine/——子包未建前宿主 ticket 包。
package ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
)

// escalationDefaults 无 escalation 段时的缺省：单发一次（A.5 原始语义）。
const (
	defaultReminderIntervalMinutes = 240 // escalation 段缺省间隔（10 号 §3.4）
	defaultMaxReminders            = 3   // escalation 段缺省次数上限
)

// stateMeta transition_meta 的 state 级段（结构对齐 validateTransitionMeta）。
type stateMeta struct {
	TimeoutMinutes *int `json:"timeout_minutes"`
	SLADueHours    *int `json:"sla_due_hours"` // SLA 层 0.5（deadline 模型 A——进入状态时刻+N 小时=due）
	Escalation     *struct {
		Notify                  []string `json:"notify"`
		ReminderIntervalMinutes *int     `json:"reminder_interval_minutes"`
		MaxReminders            *int     `json:"max_reminders"`
	} `json:"escalation"`
}

// TimeoutScanner 超时扫描任务（service.ReentrantJob 实现——任意时刻中断/并发/
// 重复执行安全：事件 at-most-once + 水位 upsert 幂等）。
type TimeoutScanner struct {
	db       *pgxpool.Pool
	eventReg *domainevent.HookRegistry
	logger   *slog.Logger
	observe  bool // 观察模式：只记日志不发事件不耗水位（存量回填近似值防误报——E-2 拍定缓解①）
}

// NewTimeoutScanner observe=true 为发布安全默认（存量 status_changed_at 为
// updated_at 近似回填，首轮可能误报——观察确认后关闭）。
func NewTimeoutScanner(db *pgxpool.Pool, eventReg *domainevent.HookRegistry, observe bool, logger *slog.Logger) *TimeoutScanner {
	if logger == nil {
		logger = slog.Default()
	}
	return &TimeoutScanner{db: db, eventReg: eventReg, logger: logger, observe: observe}
}

// Observe 观察模式标记（cron-status 端点透出——运维确认首跑无异常后关闭）。
func (s *TimeoutScanner) Observe() bool { return s.observe }

// Handle 扫描一跑（CronScheduler tick 调用；错误上抛由调度器记失败+告警链）。
func (s *TimeoutScanner) Handle(ctx context.Context, _ json.RawMessage) error {
	types, err := s.listTypesWithMeta(ctx)
	if err != nil {
		return err
	}
	scanned := 0
	for _, tc := range types {
		metas := map[string]stateMeta{}
		if err := json.Unmarshal(tc.meta, &metas); err != nil {
			// 单类型坏数据不拖垮全局（UpdateTicketType 有校验，此处防手工改库）
			s.logger.Warn("timeout_scan: bad transition_meta", "type", tc.code, "err", err)
			continue
		}
		for state, m := range metas {
			if m.TimeoutMinutes != nil && *m.TimeoutMinutes > 0 {
				n, err := s.scanState(ctx, tc.code, state, *m.TimeoutMinutes, m.Escalation)
				if err != nil {
					return fmt.Errorf("scan %s/%s: %w", tc.code, state, err)
				}
				scanned += n
			}
			// SLA 层 0.5：deadline 到期扫描（due 刻=进入状态+sla_due_hours）
			if m.SLADueHours != nil && *m.SLADueHours > 0 {
				n, err := s.scanDeadline(ctx, tc.code, state, *m.SLADueHours)
				if err != nil {
					return fmt.Errorf("deadline %s/%s: %w", tc.code, state, err)
				}
				scanned += n
			}
		}
	}
	if s.observe {
		s.logger.Warn("timeout_scan: OBSERVE MODE（不发送）", "candidates", scanned)
	} else if scanned > 0 {
		s.logger.Info("timeout_scan: dispatched", "events", scanned)
	}
	return nil
}

type typeMetaRow struct {
	code string
	meta json.RawMessage
}

func (s *TimeoutScanner) listTypesWithMeta(ctx context.Context) ([]typeMetaRow, error) {
	rows, err := s.db.Query(ctx,
		`SELECT code, transition_meta FROM ticket_types WHERE is_active AND transition_meta IS NOT NULL AND transition_meta <> '{}'::jsonb`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (typeMetaRow, error) {
		var t typeMetaRow
		return t, r.Scan(&t.code, &t.meta)
	})
}

type candidateRow struct {
	id                     int64
	humanKey               string
	title                  string
	assignedTo             *int64
	escalationSupervisorID *int64
	overdueMinutes         float64
}

func (s *TimeoutScanner) scanState(ctx context.Context, typeCode, state string, timeoutMinutes int, esc *stateMetaEscalation) (int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, COALESCE(human_key, ''), title, assigned_to, escalation_supervisor_id,
		       EXTRACT(EPOCH FROM (NOW() - status_changed_at)) / 60.0
		FROM tickets
		WHERE type_code = $1 AND status = $2
		  AND status_changed_at < NOW() - make_interval(mins => $3)
		ORDER BY id LIMIT 500`, typeCode, state, timeoutMinutes)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	cands, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (candidateRow, error) {
		var c candidateRow
		return c, r.Scan(&c.id, &c.humanKey, &c.title, &c.assignedTo, &c.escalationSupervisorID, &c.overdueMinutes)
	})
	if err != nil {
		return 0, err
	}

	notify, interval, maxReminders := escalationPlan(esc)
	sent := 0
	for _, c := range cands {
		next, ok, err := s.shouldRemind(ctx, c.id, state, interval, maxReminders)
		if err != nil {
			return sent, err
		}
		if !ok {
			continue
		}
		if s.observe {
			sent++
			s.logger.Warn("timeout_scan: overdue candidate (observe, not sent)",
				"ticket", c.id, "type", typeCode, "state", state,
				"overdue_minutes", int64(c.overdueMinutes))
			continue
		}
		s.emitTimeout(c, state, notify, next)
		if err := s.bumpReminder(ctx, c.id, state); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

// stateMetaEscalation escalation 段独立别名（scanState 传参清晰）。
type stateMetaEscalation = struct {
	Notify                  []string `json:"notify"`
	ReminderIntervalMinutes *int     `json:"reminder_interval_minutes"`
	MaxReminders            *int     `json:"max_reminders"`
}

// escalationPlan 展开催办计划：无段=单发 assignee；有段按 notify/interval/max
// （缺省 240 分钟/3 次——10 号 §3.4）。
func escalationPlan(esc *stateMetaEscalation) (notify []string, interval, maxReminders int) {
	notify = []string{"assignee"}
	interval, maxReminders = defaultReminderIntervalMinutes, defaultMaxReminders
	if esc == nil {
		return notify, interval, 1 // 无段=单发一次（A.5 原始语义）
	}
	if len(esc.Notify) > 0 {
		notify = esc.Notify
	}
	if esc.ReminderIntervalMinutes != nil && *esc.ReminderIntervalMinutes > 0 {
		interval = *esc.ReminderIntervalMinutes
	}
	if esc.MaxReminders != nil && *esc.MaxReminders >= 0 {
		maxReminders = *esc.MaxReminders
	}
	return notify, interval, maxReminders
}

// shouldRemind 水位判定：返回（本次是第几次催办, 是否应发, err）——次数供载荷
// {{ticket.reminder_count}} 渲染（种子模板 000064）。
func (s *TimeoutScanner) shouldRemind(ctx context.Context, ticketID int64, state string, interval, maxReminders int) (int, bool, error) {
	var count int
	var last *time.Time
	err := s.db.QueryRow(ctx,
		`SELECT reminder_count, last_reminder_at FROM ticket_reminders WHERE ticket_id = $1 AND state = $2`,
		ticketID, state).Scan(&count, &last)
	if err == pgx.ErrNoRows {
		return 1, true, nil
	}
	if err != nil {
		return 0, false, err
	}
	if count >= maxReminders {
		return 0, false, nil
	}
	if last == nil {
		return count + 1, true, nil
	}
	if time.Since(*last) > time.Duration(interval)*time.Minute {
		return count + 1, true, nil
	}
	return 0, false, nil
}

// bumpReminder 水位 +1（upsert 幂等——并发双跑经 advisory lock 已挡，残余竞态
// 至多多计一次催办，可接受）。
func (s *TimeoutScanner) bumpReminder(ctx context.Context, ticketID int64, state string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO ticket_reminders (ticket_id, state, reminder_count, last_reminder_at)
		VALUES ($1, $2, 1, NOW())
		ON CONFLICT (ticket_id, state)
		DO UPDATE SET reminder_count = ticket_reminders.reminder_count + 1, last_reminder_at = NOW()`,
		ticketID, state)
	return err
}

// emitTimeout 组装目标集并发事件。目标解析（dd §31）：
// assignee=处理人；supervisor=人工指定 escalation_supervisor_id（manager_id 档
// 挂 M-HR；未指定=缺席不推导）。cc_users=全部目标（种子规则 recipient_type=cc）。
func (s *TimeoutScanner) emitTimeout(c candidateRow, state string, notify []string, reminderCount int) {
	if s.eventReg == nil {
		return
	}
	var targets []string
	wantSupervisor, wantAssignee := false, false
	for _, n := range notify {
		if n == "assignee" {
			wantAssignee = true
		}
		if n == "supervisor" {
			wantSupervisor = true
		}
	}
	var toAssignee string
	if wantAssignee && c.assignedTo != nil {
		toAssignee = strconv.FormatInt(*c.assignedTo, 10)
		targets = append(targets, toAssignee)
	}
	var toSupervisor string
	if wantSupervisor && c.escalationSupervisorID != nil {
		toSupervisor = strconv.FormatInt(*c.escalationSupervisorID, 10)
		targets = append(targets, toSupervisor)
	}
	if len(targets) == 0 {
		return // 无可送达目标（未指派且无上级）——不发空事件
	}
	payload := domainevent.Payload{
		"title": c.title, "status": state,
	}
	if c.humanKey != "" {
		payload["human_key"] = c.humanKey
	} else {
		payload["human_key"] = strconv.FormatInt(c.id, 10)
	}
	if toAssignee != "" {
		payload["to_assignee"] = toAssignee
	}
	if toSupervisor != "" {
		payload["to_supervisor"] = toSupervisor
	}
	payload["cc_users"] = joinIDs(targets)
	payload["overdue_minutes"] = int64(c.overdueMinutes)
	payload["reminder_count"] = reminderCount
	s.eventReg.Dispatch(context.Background(), domainevent.DomainEvent{
		BizType: domainevent.BizTicket, Action: domainevent.TicketTimeout,
		BizID:     strconv.FormatInt(c.id, 10),
		Payload:   payload,
		Timestamp: time.Now(),
	})
}

func joinIDs(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ","
		}
		out += id
	}
	return out
}

// scanDeadline deadline 到期扫描（SLA 层 0.5——模型 A 截止时间戳语义）：
// 命中 emit ticket.deadline（收件人=assignee∪人工指定上级，与 timeout 同构——
// message_log 查重防重发）。due 写入点在 Transition commit 后按 to 态 sla_due_hours 计算。
func (s *TimeoutScanner) scanDeadline(ctx context.Context, typeCode, state string, dueHours int) (int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, COALESCE(human_key, ''), title, assigned_to, escalation_supervisor_id
		FROM tickets
		WHERE type_code = $1 AND status = $2
		  AND sla_due_at IS NOT NULL AND sla_due_at < NOW()`, typeCode, state)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	cands, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (candidateRow, error) {
		var c candidateRow
		err := r.Scan(&c.id, &c.humanKey, &c.title, &c.assignedTo, &c.escalationSupervisorID)
		return c, err
	})
	if err != nil {
		return 0, err
	}
	notify := []string{"assignee", "supervisor"}
	sent := 0
	for _, c := range cands {
		_, ok, err := s.shouldRemind(ctx, c.id, state, 24*60, 1) // deadline 单次提醒
		if err != nil {
			return sent, err
		}
		if !ok {
			continue
		}
		if s.observe {
			sent++
			s.logger.Warn("deadline_scan: overdue candidate (observe, not sent)", "ticket", c.id, "state", state)
			continue
		}
		s.emitDeadline(c, state, notify)
		if err := s.bumpReminder(ctx, c.id, "deadline:"+state); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

// emitDeadline 到期事件（cc_users=assignee∪上级——timeout 同构）。
func (s *TimeoutScanner) emitDeadline(c candidateRow, state string, notify []string) {
	if s.eventReg == nil {
		return
	}
	var targets []string
	var toAssignee string
	if c.assignedTo != nil {
		toAssignee = strconv.FormatInt(*c.assignedTo, 10)
		targets = append(targets, toAssignee)
	}
	if c.escalationSupervisorID != nil {
		targets = append(targets, strconv.FormatInt(*c.escalationSupervisorID, 10))
	}
	if len(targets) == 0 {
		return
	}
	payload := domainevent.Payload{"title": c.title, "status": state}
	if c.humanKey != "" {
		payload["human_key"] = c.humanKey
	} else {
		payload["human_key"] = strconv.FormatInt(c.id, 10)
	}
	payload["to_assignee"] = toAssignee
	payload["cc_users"] = joinIDs(targets)
	s.eventReg.Dispatch(context.Background(), domainevent.DomainEvent{
		BizType: domainevent.BizTicket, Action: domainevent.TicketDeadline,
		BizID:   strconv.FormatInt(c.id, 10),
		Payload: payload, Timestamp: time.Now(),
	})
}
