// S3a B3-1 转换执行服务层（03 号附录 A.1——统一转换端点+available 查询）。
package ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// Transition S3a B3-1 统一转换执行（POST /tickets/transition——P0-1 显式化）：
// L2/L3 资源级鉴权 → AssertTransition（状态机）→ condition 权限门（fail-closed）
// → validator 前置条件（expr）→ 单事务执行（UpdateStatusTx 咽喉含 status_changed_at）
// → commit 后 emit DomainEvent（B6-1 锚）。Close/Assign 定向端点保留兼容
// （收敛为其封装为 B3-1 后续批——本批首件先立执行落点+语义面，防破坏既有测试）。
func (s *Service) Transition(ctx context.Context, req *model.TransitionTicketRequest, actorUserID int64) (*model.Ticket, error) {
	// ① 资源级鉴权（L2 可见 + L3 transition 动作——新增 action）
	if err := s.authorizeCheck(ctx, actorUserID, "transition", strconv.FormatInt(req.ID, 10)); err != nil {
		return nil, err
	}
	ticket, err := s.ticketRepo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	// ② 状态机校验（AssertTransition=tx 前纯校验——transitions 邻接表）
	ttype, err := s.ticketRepo.GetTicketType(ctx, ticket.TypeCode)
	if err != nil {
		return nil, err
	}
	sm, err := FromTicketType(ttype)
	if err != nil {
		return nil, fmt.Errorf("build state machine: %w", err)
	}
	if err := sm.AssertTransition(ticket.Status, req.To); err != nil {
		return nil, err
	}
	// ③④ condition 权限门 + validator 前置条件（R3 审计修复——三路同源）
	tm, err := s.checkTransitionMeta(ctx, ticket, req.To, actorUserID, ttype)
	if err != nil {
		return nil, err
	}
	// ⑤ 单事务执行（UpdateStatusTx 咽喉=状态写+status_changed_at 同语句——E-2 锚）
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := s.ticketRepo.UpdateStatusTx(ctx, tx, req.ID, ticket.Status, req.To); err != nil {
		return nil, err
	}
	// S3a 随窗件 SLA 层 0.5：按 to 状态的 sla_due_hours 写 due 刻（模型 A——进入状态时刻+N 小时）。
	// 与状态写同事务：修前在 commit 后裸 Exec 且吞错（无事务无日志——写失败即状态已变而 due 刻陈旧）；
	// 无 meta/无配置一律显式清 NULL——修前 toMeta==nil 不清，靠「扫描只遍历配了 meta 的状态」隐式兜住。
	var slaHours *int
	if toMeta, perr := parseStateMeta(ttype.TransitionMeta, req.To); perr == nil && toMeta != nil &&
		toMeta.SLADueHours != nil && *toMeta.SLADueHours > 0 {
		slaHours = toMeta.SLADueHours
	}
	if slaHours != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE tickets SET sla_due_at = NOW() + make_interval(hours => $1) WHERE id = $2`,
			*slaHours, req.ID); err != nil {
			return nil, fmt.Errorf("set sla_due_at: %w", err)
		}
	} else if _, err := tx.Exec(ctx, `UPDATE tickets SET sla_due_at = NULL WHERE id = $1`, req.ID); err != nil {
		return nil, fmt.Errorf("clear sla_due_at: %w", err)
	}
	// 可选说明 → 写评论（与 Close 同语义）
	if req.Comment != "" {
		if err := s.ticketRepo.CreateCommentTx(ctx, tx, &model.TicketComment{
			TicketID: req.ID, UserID: actorUserID, Content: req.Comment, IsInternal: false,
		}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	// D1 修复（审计）：清旧状态催办水位——重入同状态时 reminder_count 从零起算
	// （修复前残留=达上限后永不催；全仓此前零 DELETE 调用点）
	s.clearReminderWatermark(ctx, req.ID, ticket.Status)
	// ⑥ commit 后 emit（B6-1 锚——统一转换事件；action 命名 ticket.transitioned
	// 与 created/assigned/closed 平级；Close/Assign 仍发各自事件=向后兼容）
	payload := domainevent.Payload{
		"title": ticket.Title, "type_code": ticket.TypeCode,
		"from_status": ticket.Status, "to_status": req.To,
	}
	if ticket.HumanKey != "" {
		payload["human_key"] = ticket.HumanKey
	}
	if ticket.AssignedTo != nil {
		payload["to_assignee"] = strconv.FormatInt(*ticket.AssignedTo, 10)
	}
	s.emitEvent(domainevent.DomainEvent{
		BizType: domainevent.BizTicket, Action: domainevent.TicketTransitioned,
		BizID: strconv.FormatInt(req.ID, 10), ActorID: actorUserID,
		Payload: payload, Timestamp: time.Now(),
	})
	// S3a B3-3：post_actions（commit 后——失败不回滚转换[RunPostActions 内捕获]）；
	// 链深取自 ctx（动作内再触发转换时递增——见 actionDepth），修前恒传 0 致 maxDepth 守卫形同虚设
	if s.actionReg != nil {
		s.actionReg.RunPostActions(ctx, slog.Default(), tm, ActionContext{
			Ticket: ticket, From: ticket.Status, To: req.To, ActorID: actorUserID,
			SystemAsCreator: true,
		}, actionDepth(ctx))
	}
	// 返回最新态
	return s.ticketRepo.GetByID(ctx, req.ID)
}

// checkTransitionMeta transition_meta 转换级语义检查（condition+validator——
// R3 审计修复：Close/Assign 与 Transition 三路同源调用，消"condition 只在
// /transition 路径生效"旁路）。tm=nil/空=放行（零配置兼容）。
func (s *Service) checkTransitionMeta(ctx context.Context, ticket *model.Ticket, toStatus string, actorUserID int64, ttype *model.TicketType) (*TransitionMeta, error) {
	tm, err := parseTransitionMeta(ttype.TransitionMeta, ticket.Status, toStatus)
	if err != nil {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, err.Error())
	}
	if tm == nil {
		return nil, nil // 无配置=既有行为
	}
	roles, err := s.getRoles(ctx, actorUserID)
	if err != nil {
		return nil, fmt.Errorf("fetch roles: %w", err)
	}
	isOrgAdmin, _ := s.delegation.IsOrgAdminOrOwner(ctx, actorUserID, ticket.OrgID)
	actor := resolveActor(ticket, actorUserID, roles, isOrgAdmin)
	if !checkCondition(tm, actor) {
		return nil, errcode.ErrNoPermission
	}
	if err := checkValidator(tm, ticket); err != nil {
		return nil, err
	}
	return tm, nil
}

// clearReminderWatermark 清旧状态催办水位（审计 D1——Transition/Close/Assign
// 三路同源：凡状态离开旧状态均清该状态水位行，重入时 reminder_count 归零）。
// 同清 deadline: 前缀键（G1-1——SLA deadline 水位与 timeout 水位同生命周期，
// 残留则重入该状态后 deadline 永不再提醒）。失败仅告警不影响主流程（下次状态
// 变更再清——与 D1 首修口径一致）。
func (s *Service) clearReminderWatermark(ctx context.Context, ticketID int64, state string) {
	if _, err := s.db.Exec(ctx,
		`DELETE FROM ticket_reminders WHERE ticket_id = $1 AND (state = $2 OR state = 'deadline:' || $2)`,
		ticketID, state); err != nil {
		slog.Warn("clear reminder watermark failed", "ticket", ticketID, "state", state, "err", err)
	}
}

// ListAvailableTransitions 当前用户可执行的转换列表（GET /:id/transitions/available
// ——Jira condition 语义=按当前用户过滤；服务端 AssertTransition+condition 兜底）。
func (s *Service) ListAvailableTransitions(ctx context.Context, ticketID, actorUserID int64) ([]string, error) {
	ticket, err := s.ticketRepo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	// 读可见性（同 Get 权限面）
	if err := s.authorizeCheck(ctx, actorUserID, "read", strconv.FormatInt(ticketID, 10)); err != nil {
		return nil, err
	}
	ttype, err := s.ticketRepo.GetTicketType(ctx, ticket.TypeCode)
	if err != nil {
		return nil, err
	}
	var transitions map[string][]string
	if err := json.Unmarshal(ttype.Transitions, &transitions); err != nil {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "类型 transitions 解析失败")
	}
	roles, err := s.getRoles(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	isOrgAdmin, _ := s.delegation.IsOrgAdminOrOwner(ctx, actorUserID, ticket.OrgID)
	return AvailableTransitionsFor(ctx, ticket, transitions, ttype.TransitionMeta, actorUserID, roles, isOrgAdmin), nil
}
