// S3a B3-2 自动分派服务（03 号 §3.2/附录 A.2——五决议既定）：
// OnAfterCreate Hook 按 priority 升序遍历规则 → 首个命中 → 走 Assign 通道自动
// 分派（不绕鉴权——Assign 内部 authorizeCheck 以系统身份走 admin 通配）→
// 无命中保持 open + dispatch_missed 事件（决议④）；失败不回滚创建（决议⑤）。
package ticket

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// DispatchService 自动分派+规则管理面。
type DispatchService struct {
	ruleRepo *repository.AssignmentRuleRepo
	svc      *Service // 工单服务（assignAsSystem 通道——权限旁路口径见其注，G-32）
	logger   *slog.Logger
}

// NewDispatchService 创建。
func NewDispatchService(ruleRepo *repository.AssignmentRuleRepo, svc *Service, logger *slog.Logger) *DispatchService {
	if logger == nil {
		logger = slog.Default()
	}
	return &DispatchService{ruleRepo: ruleRepo, svc: svc, logger: logger}
}

// OnAfterCreate 工单创建后自动分派（决议②——OnAfterCreate Hook 语义）。
// 走 svc.AssignAsSystem（系统身份：鉴权在规则创建期行使；状态机→事务→事件全链不变，
// 事件 actor=规则创建者供归因）。失败不回滚创建（决议⑤）——仅记日志 + dispatch_missed 事件。
func (d *DispatchService) OnAfterCreate(ctx context.Context, ticket *model.Ticket) {
	rule, err := d.ruleRepo.MatchOnCreate(ctx, ticket.TypeCode, ticket.Title)
	if err != nil {
		d.logger.Warn("dispatch: rule match failed", "err", err, "ticket", ticket.ID)
		return
	}
	if rule == nil {
		d.emitDispatchMissed(ctx, ticket)
		return
	}
	// 命中 → 走 assignAsSystem（决议③ target_user 须属 target_org——写侧已校验）
	// 系统身份：不以 rule.CreatedBy 过 L2/L3（修前非该单可操作人建的规则一律 403 静默失效；
	// 权限旁路口径见 service.go assignAsSystem 注——G-32 委托放大族）；
	// 事件/ticket_events actor 仍记规则创建者供归因
	if rule.TargetUser != nil {
		if err := d.svc.assignAsSystem(ctx, &model.AssignTicketRequest{
			ID: ticket.ID, AssignedTo: rule.TargetUser,
		}, rule.CreatedBy); err != nil {
			d.logger.Warn("dispatch: auto-assign failed (not rolling back create)",
				"err", err, "ticket", ticket.ID, "rule", rule.ID)
			d.emitDispatchMissed(ctx, ticket)
		}
		return
	}
	// target_user=NULL=仅分派组织（暂不实现组织级分派——phase6 资产路由场景再扩）
	d.logger.Info("dispatch: org-level rule matched (target_user=nil, keeping open)",
		"ticket", ticket.ID, "rule", rule.ID)
	d.emitDispatchMissed(ctx, ticket)
}

func (d *DispatchService) emitDispatchMissed(ctx context.Context, ticket *model.Ticket) {
	// dispatch_missed = 走 DomainEvent{ticket, "ticket.dispatch_missed"}——
	// S6 规则面可配告警通知（管理端"新单无人接"监控场景）
	if d.svc != nil && d.svc.eventReg != nil {
		d.svc.emitEvent(domainevent.DomainEvent{
			BizType: domainevent.BizTicket,
			Action:  domainevent.TicketDispatchMissed,
			BizID:   strconv.FormatInt(ticket.ID, 10),
			Payload: domainevent.Payload{
				"title": ticket.Title, "type_code": ticket.TypeCode,
				"human_key": ticket.HumanKey,
			},
			Timestamp: time.Now(),
		})
	}
}

// ─── 管理面（规则 CRUD——类型管理页族扩展，零新码）───

// ListRules 全量规则。
func (d *DispatchService) ListRules(ctx context.Context) ([]*model.AssignmentRule, error) {
	return d.ruleRepo.List(ctx)
}

// CreateRule 建规则（决议③ target_user 属 target_org 写侧校验）。
func (d *DispatchService) CreateRule(ctx context.Context, req *model.CreateAssignmentRuleRequest, actorUserID int64) (*model.AssignmentRule, error) {
	if req.TargetUser != nil {
		ok, err := d.ruleRepo.UserBelongsToOrg(ctx, *req.TargetUser, req.TargetOrg)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errcode.New(errcode.ErrInvalidParams.Code,
				"target_user 不属于 target_org（决议③——写侧校验）")
		}
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	rule := &model.AssignmentRule{
		TypeCode: req.TypeCode, Priority: req.Priority, Keyword: req.Keyword,
		TargetOrg: req.TargetOrg, TargetUser: req.TargetUser,
		Enabled: enabled, CreatedBy: actorUserID,
	}
	if err := d.ruleRepo.Create(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

// DeleteRule 删规则。
func (d *DispatchService) DeleteRule(ctx context.Context, id int64) error {
	return d.ruleRepo.Delete(ctx, id)
}

// UpdateRuleEnabled 启停。
func (d *DispatchService) UpdateRuleEnabled(ctx context.Context, id int64, enabled bool) error {
	return d.ruleRepo.UpdateEnabled(ctx, id, enabled)
}
