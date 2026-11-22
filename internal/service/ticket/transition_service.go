// S3a B3-1 转换执行服务层（03 号附录 A.1——统一转换端点+available 查询）。
package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// Transition S3a B3-1 统一转换执行（POST /tickets/transition——P0-1 显式化）：
// L2/L3 资源级鉴权 → AssertTransition（状态机）→ condition 权限门（fail-closed）
// → validator 前置条件（expr）→ 单事务执行（UpdateStatusTx 咽喉含 status_changed_at）
// → commit 后 emit DomainEvent（B6-1 锚）。Close/Assign 定向端点保留兼容
// （收敛为其封装为 B3-1 后续批——本批首件先立执行落点+语义面，防破坏既有测试）。
func (s *Service) Transition(ctx context.Context, req *model.TransitionTicketRequest, actorUserID int64) (*model.Ticket, error) {
	cmd, err := parseEvidenceTransitionRequest(req)
	if err != nil {
		return nil, err
	}
	var ticket, committed *model.Ticket
	var tm *TransitionMeta
	var replayed bool
	var lockedTickets []int64
	if cmd.versioned() {
		lockedTickets = []int64{req.ID}
	}
	err = s.withHTTPCommand(ctx, "transition", actorUserID, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		// ① 资源级鉴权（L2 可见 + L3 transition 动作——新增 action）
		if err := s.authorizeCheck(ctx, actorUserID, "transition", strconv.FormatInt(req.ID, 10)); err != nil {
			return err
		}
		ticket, err = s.ticketRepo.GetByID(ctx, req.ID)
		if err != nil {
			return err
		}
		// ② 写入前校验状态机；HTTP 路径此时已持有 IAM 事务。
		ttype, fields, ref, err := s.resolveTicketDefinition(ctx, ticket)
		if err != nil {
			return err
		}
		if cmd.versioned() {
			replay, err := s.findEvidenceTransitionReplayTx(ctx, tx, cmd, actorUserID)
			if err != nil {
				return err
			}
			if replay != nil {
				_, _, originalRef, err := s.resolveTicketDefinition(ctx, &model.Ticket{
					TypeCode: ticket.TypeCode, DefinitionVersion: &replay.DefinitionVersion,
				})
				if err != nil {
					return err
				}
				replayed = true
				committed = ticketFromEvidenceReceipt(ticket.ID, originalRef, *replay)
				return nil
			}
			tm, err = s.checkTransitionCondition(ctx, ticket, req.To, actorUserID, ttype)
			if err != nil {
				return err
			}
			snapshot := model.TicketDefinitionSnapshot{Type: *ttype, Fields: fields}
			receipt, err := s.applyEvidenceTransitionTx(ctx, tx, cmd, ticket, &snapshot, actorUserID)
			if err != nil {
				return err
			}
			committed = ticketFromEvidenceReceipt(ticket.ID, ref, receipt)
			return nil
		}
		sm, err := FromTicketType(ttype)
		if err != nil {
			return fmt.Errorf("build state machine: %w", err)
		}
		if err := sm.AssertTransition(ticket.Status, req.To); err != nil {
			return err
		}
		// ③④ condition 权限门 + validator 前置条件（R3 审计修复——三路同源）
		tm, err = s.checkTransitionMeta(ctx, ticket, req.To, actorUserID, ttype)
		if err != nil {
			return err
		}
		// ⑤ 单事务执行（UpdateStatusTx 咽喉=状态写+status_changed_at 同语句——E-2 锚）
		ownTx := tx == nil
		if ownTx {
			tx, err = s.db.Begin(ctx)
			if err != nil {
				return fmt.Errorf("begin tx: %w", err)
			}
			defer tx.Rollback(ctx)
		}
		if err := s.ticketRepo.LockTransitionSnapshotTx(ctx, tx, ticket, nil); err != nil {
			return err
		}
		if err := s.ticketRepo.UpdateStatusTx(ctx, tx, req.ID, ticket.Status, req.To); err != nil {
			return err
		}
		if err := s.applyStateSLARuleTx(ctx, tx, ttype, ticket, req.To); err != nil {
			return err
		}
		if err := s.ticketRepo.CreateEventTx(ctx, tx, &model.TicketEvent{
			TicketID: req.ID, UserID: actorUserID, Action: EventStatusChanged,
			FromValue: ticket.Status, ToValue: req.To,
		}); err != nil {
			return err
		}
		if err := s.writeLifecycleCommentTx(ctx, tx, req.ID, actorUserID, req.Comment); err != nil {
			return err
		}
		if ownTx {
			return tx.Commit(ctx)
		}
		return nil
	}, lockedTickets...)
	if err != nil {
		return nil, err
	}
	if replayed || (committed != nil && ticket.Status == req.To) {
		return committed, nil
	}
	// D1 修复（审计）：清旧状态催办水位——重入同状态时 reminder_count 从零起算
	// （修复前残留=达上限后永不催；全仓此前零 DELETE 调用点）
	s.clearReminderWatermark(ctx, req.ID, ticket.Status)
	// ⑥ commit 后 emit（B6-1 锚——统一转换事件；action 命名 ticket.transitioned
	// 与 created/assigned/closed 平级；Close/Assign 仍发各自事件=向后兼容）
	payload := domainevent.Payload{
		"title": ticket.Title, "type_code": ticket.TypeCode,
		"from_status": ticket.Status, "to_status": req.To, "status": req.To,
	}
	if ticket.HumanKey != "" {
		payload["human_key"] = ticket.HumanKey
	}
	if ticket.AssignedTo != nil {
		payload["to_assignee"] = strconv.FormatInt(*ticket.AssignedTo, 10)
	}
	s.emitTicketEvent(ticket, domainevent.DomainEvent{
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
	if committed != nil {
		return committed, nil
	}
	// 旧转换请求继续返回最新态。
	updated, err := s.ticketRepo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	return s.describeTicketDefinition(ctx, updated)
}

func ticketFromEvidenceReceipt(id int64, ref *model.TicketDefinitionRef, receipt repository.TicketEvidenceTransitionResult) *model.Ticket {
	return &model.Ticket{
		ID: id, TypeCode: ref.TypeCode, Status: receipt.Status,
		Revision: receipt.Revision, DefinitionVersion: &receipt.DefinitionVersion,
		DefinitionRef: ref, DefinitionResolution: "exact", StatusChangedAt: receipt.StatusChangedAt,
	}
}

// checkTransitionMeta transition_meta 转换级语义检查（condition+validator——
// R3 审计修复：Close/Assign 与 Transition 三路同源调用，消"condition 只在
// /transition 路径生效"旁路）。tm=nil/空=放行（零配置兼容）。
func (s *Service) checkTransitionMeta(ctx context.Context, ticket *model.Ticket, toStatus string, actorUserID int64, ttype *model.TicketType) (*TransitionMeta, error) {
	tm, err := s.checkTransitionCondition(ctx, ticket, toStatus, actorUserID, ttype)
	if err != nil {
		return nil, err
	}
	if err := rejectLegacyEvidenceTransition(ttype.TransitionMeta, ticket.Status, toStatus, tm); err != nil {
		return nil, err
	}
	if err := checkValidator(withDictQuerier(ctx, repository.IAMDB(ctx, s.db)), tm, ticket); err != nil {
		return nil, err
	}
	return tm, nil
}

// checkTransitionCondition uses ownership and roles from the current transaction.
// Evidence validators run separately on the submitted candidate.
func (s *Service) checkTransitionCondition(ctx context.Context, ticket *model.Ticket, toStatus string, actorUserID int64, ttype *model.TicketType) (*TransitionMeta, error) {
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
	// 读可见性（同 Get 权限面）
	id := strconv.FormatInt(ticketID, 10)
	if err := s.authorizeCheck(ctx, actorUserID, "read", id); err != nil {
		return nil, err
	}
	if err := s.authorizeCheck(ctx, actorUserID, "transition", id); err != nil {
		if errors.Is(err, errcode.ErrNoPermission) {
			return []string{}, nil
		}
		return nil, err
	}
	ticket, err := s.ticketRepo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	ttype, _, _, err := s.resolveTicketDefinition(ctx, ticket)
	if errors.Is(err, errcode.ErrServiceUnavailable) {
		return []string{}, nil
	}
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
