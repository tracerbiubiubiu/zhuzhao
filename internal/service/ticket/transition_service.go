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
	// ③ condition 权限门（transition_meta from>to——Jira condition 同语义）
	roles, err := s.getRoles(ctx, actorUserID)
	if err != nil {
		return nil, fmt.Errorf("fetch roles: %w", err)
	}
	isOrgAdmin, _ := s.delegation.IsOrgAdminOrOwner(ctx, actorUserID, ticket.OrgID)
	actor := resolveActor(ticket, actorUserID, roles, isOrgAdmin)
	tm, err := parseTransitionMeta(ttype.TransitionMeta, ticket.Status, req.To)
	if err != nil {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, err.Error())
	}
	if !checkCondition(tm, actor) {
		return nil, errcode.ErrNoPermission // fail-closed：可见但角色不符 → 403
	}
	// ④ validator 数据侧前置条件（expr——上下文=custom_data+内置字段）
	if err := checkValidator(tm, ticket); err != nil {
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
	// S3a B3-3：post_actions（commit 后——失败不回滚转换[RunPostActions 内捕获]）
	if s.actionReg != nil {
		s.actionReg.RunPostActions(ctx, slog.Default(), tm, ActionContext{
			Ticket: ticket, From: ticket.Status, To: req.To, ActorID: actorUserID,
			SystemAsCreator: true,
		}, 0)
	}
	// 返回最新态
	return s.ticketRepo.GetByID(ctx, req.ID)
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
