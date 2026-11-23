package riskdispatch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// ItemResult 逐风险派单结果词表（详设 §5.1）。
type ItemResult string

const (
	ItemCreated       ItemResult = "created"
	ItemLinked        ItemResult = "linked"
	ItemAlreadyLinked ItemResult = "already_linked"
	ItemDenied        ItemResult = "denied"
	ItemInvalid       ItemResult = "invalid"
	ItemConflict      ItemResult = "conflict"
)

// 拒绝/冲突原因（逐项与组级共用；组级失败时传播到全部可行动项）。
const (
	ReasonRiskMissing              = "risk_missing"
	ReasonRevisionChanged          = "revision_changed"
	ReasonStatusNotDispatchable    = "status_not_dispatchable"
	ReasonOwnerUnconfirmed         = "owner_unconfirmed"
	ReasonOwnerChanged             = "owner_changed"
	ReasonOwnerUnavailable         = "owner_unavailable"
	ReasonIdentityChanged          = "risk_identity_changed"
	ReasonGroupScopeDrift          = "group_scope_drift"
	ReasonGroupCapacity            = "group_capacity_exceeded"
	ReasonMappingMissing           = "mapping_missing"
	ReasonMappingDisabled          = "mapping_disabled"
	ReasonDispatchForbidden        = "dispatch_forbidden"
	ReasonPlanExpired              = "plan_expired"
	ReasonGroupRejected            = "group_item_rejected"
	ReasonTicketStageNotAppendable = "ticket_stage_not_appendable"
)

// dispatchableStatuses 允许发起派单的风险状态（已入整改链/终态不再重复派）。
var dispatchableStatuses = map[string]bool{"new": true, "assessed": true}

// ApplyInput 应用输入：仅收规范三元组；多带业务字段视为覆盖企图（handler 层 400）。
type ApplyInput struct {
	OperationID string
	PlanID      uuid.UUID
	PlanHash    string
}

// ItemOutcome 单风险结果。
type ItemOutcome struct {
	RiskID int64            `json:"risk_id,string"`
	Result ItemResult       `json:"result"`
	Ticket *TicketReference `json:"ticket,omitempty"` // already_linked 指向他票时给出（仍获权才投影）
	Reason string           `json:"reason,omitempty"` // denied/invalid/conflict 时非空
}

// GroupOutcome 单组结果。
type GroupOutcome struct {
	GroupKey           string           `json:"group_key"`
	GroupOperationID   string           `json:"group_operation_id,omitempty"`
	Status             string           `json:"status"` // completed | failed
	Ticket             *TicketReference `json:"ticket,omitempty"`
	OwnerUserID        int64            `json:"owner_user_id,string,omitempty"`
	OwnerOrgAtDispatch int64            `json:"owner_org_at_dispatch,string,omitempty"`
	Items              []ItemOutcome    `json:"items"`
}

// ApplyResult 整批结果与 parent 汇总（pending/partial/completed）。
type ApplyResult struct {
	OperationID string         `json:"operation_id"`
	PlanID      string         `json:"plan_id"`
	Status      string         `json:"status"`
	Groups      []GroupOutcome `json:"groups"`
}

// Dispatcher 风险派单编排服务（plan + apply）。
type Dispatcher struct {
	deps DispatcherDeps
}

// DispatcherDeps 组合根装配（Wire 属主会话；各域实现见 dispatch-handoff.md）。
type DispatcherDeps struct {
	DB          DB                   // 预览持久化与只读投影
	Tx          DispatchTransactions // 守卫事务源（工单服务守卫池）
	Authorizer  RiskAuthorizer       // 风险域三层授权
	Facts       RiskFacts            // 风险域现势事实
	Commit      RiskDispatchCommit   // 风险域状态/历史落账
	Tickets     TicketCreator        // 工单受限创建适配器（冻结桥 API）
	TicketViews TicketViewAuthorizer // 原票引用读权投影
	Owners      OwnerDirectory       // 责任人在职 + 主部门事实
	Definitions DefinitionResolver   // 已发布定义解析
}

func NewDispatcher(deps DispatcherDeps) *Dispatcher {
	return &Dispatcher{deps: deps}
}

// Apply 按 plan 逐组应用派单：每组独立守卫事务、组间互不牵连；
// 原号重投读原回执并只续未完成组（详设 §5.1 批次合同）。
func (d *Dispatcher) Apply(ctx context.Context, actorID int64, input ApplyInput) (ApplyResult, error) {
	if !validOperationID(input.OperationID) || input.PlanID == uuid.Nil || input.PlanHash == "" {
		return ApplyResult{}, errcode.ErrInvalidParams
	}
	plan, err := ReadPlan(ctx, d.deps.DB, actorID, input.PlanID)
	if err != nil {
		return ApplyResult{}, err
	}
	if plan.Hash != input.PlanHash {
		return ApplyResult{}, errcode.ErrConcurrentModification
	}
	groups := plan.Binding.Plan.Groups
	target := plan.Binding.Target
	if len(groups) == 0 {
		return ApplyResult{}, errcode.ErrInvalidParams
	}
	allRiskIDs := make([]int64, 0, len(groups))
	for _, group := range groups {
		for _, item := range group.Items {
			allRiskIDs = append(allRiskIDs, item.RiskID)
		}
	}
	scope := ScopeSingle
	if len(allRiskIDs) > 1 {
		scope = ScopeBulk
	}
	if err := d.deps.Tx.WithDispatchTransaction(ctx, actorID, func(ctx context.Context, tx pgx.Tx) error {
		if err := d.deps.Authorizer.AuthorizeDispatch(ctx, tx, actorID, scope, allRiskIDs, target); err != nil {
			return err
		}
		return BindOperationTx(ctx, tx, actorID, input.OperationID, input.PlanID, plan.Hash)
	}); err != nil {
		return ApplyResult{}, err
	}

	now := time.Now().UTC()
	result := ApplyResult{OperationID: input.OperationID, PlanID: input.PlanID.String()}
	completedGroups := 0
	for _, group := range groups {
		if receipt, found, err := readGroupReceipt(ctx, d.deps.DB, actorID, input.OperationID, group.Key); err != nil {
			return ApplyResult{}, err
		} else if found {
			result.Groups = append(result.Groups, d.projectReceipt(ctx, actorID, group.Key, receipt))
			completedGroups++
			continue
		}
		if now.After(plan.ExpiresAt) {
			// plan 过期只读：未完成组不再建票，原回执照常投影
			result.Groups = append(result.Groups, d.expiredGroupOutcome(group))
			continue
		}
		outcome, err := d.applyGroup(ctx, actorID, input, plan.Hash, group, target, scope, now)
		if err != nil {
			return ApplyResult{}, err
		}
		if outcome.Status == "completed" {
			completedGroups++
		}
		result.Groups = append(result.Groups, outcome)
	}
	result.Status = batchStatus(completedGroups, len(groups))
	return result, nil
}

// projectReceipt 原号重投的完成组投影：原回执事实 + 按当前权限遮蔽票引用。
func (d *Dispatcher) projectReceipt(ctx context.Context, actorID int64, groupKey string, receipt GroupReceipt) GroupOutcome {
	outcome := GroupOutcome{
		GroupKey:           groupKey,
		GroupOperationID:   receipt.GroupOperationID,
		Status:             "completed",
		Ticket:             d.viewTicket(ctx, actorID, receipt.TicketID, receipt.TicketRef),
		OwnerUserID:        receipt.OwnerUserID,
		OwnerOrgAtDispatch: receipt.OwnerOrgAtDispatch,
	}
	for _, item := range receipt.Items {
		projected := ItemOutcome{RiskID: item.RiskID, Result: item.Result}
		if item.Ticket != nil {
			projected.Ticket = d.viewTicket(ctx, actorID, item.Ticket.ID, item.Ticket.Ref)
		}
		outcome.Items = append(outcome.Items, projected)
	}
	return outcome
}

func (d *Dispatcher) expiredGroupOutcome(group Group) GroupOutcome {
	outcome := GroupOutcome{GroupKey: group.Key, Status: "failed"}
	for _, item := range group.Items {
		outcome.Items = append(outcome.Items, ItemOutcome{
			RiskID: item.RiskID, Result: ItemConflict, Reason: ReasonPlanExpired,
		})
	}
	return outcome
}

// applyGroup 单组原子应用：任一失败该组零新票/零关联/零状态翻转（事务回滚）。
// 返回 error 仅限中止整批的基础设施/合同错误（如原号异参）；业务拒绝进逐项结果。
func (d *Dispatcher) applyGroup(ctx context.Context, actorID int64, input ApplyInput, planHash string,
	group Group, target Target, scope DispatchScope, now time.Time) (GroupOutcome, error) {
	// Prepare 须在守卫事务外（冻结桥序）；追加路径不建票。预读仅为避免多余取号，
	// 权威判定在事务内重做——竞态下多备的 preparation 被弃用（取号空洞无害）。
	live, err := readLiveGroup(ctx, d.deps.DB, group.Items[0].FindingKey, group.Items[0].OwnerID)
	if err != nil {
		return GroupOutcome{}, err
	}
	var preparation TicketPreparation
	if live.ID == 0 {
		preparation, err = d.deps.Tickets.PrepareRiskDispatchCreate(ctx, d.buildCreateRequest(group, target), actorID)
		if err != nil {
			return GroupOutcome{}, fmt.Errorf("prepare risk dispatch ticket: %w", err)
		}
	}

	var outcome GroupOutcome
	err = d.deps.Tx.WithDispatchTransaction(ctx, actorID, func(ctx context.Context, tx pgx.Tx) error {
		var groupErr error
		outcome, groupErr = d.applyGroupTx(ctx, tx, actorID, input, planHash, group, target, scope, now, live, preparation)
		if groupErr == nil && outcome.Status == "failed" {
			return errDispatchGroupRejected
		}
		return groupErr
	})
	if errors.Is(err, errDispatchGroupRejected) {
		return outcome, nil
	}
	return outcome, err
}

var errDispatchGroupRejected = errors.New("dispatch group rejected")

func (d *Dispatcher) applyGroupTx(ctx context.Context, tx pgx.Tx, actorID int64, input ApplyInput, planHash string,
	group Group, target Target, scope DispatchScope, now time.Time, live LiveGroup, preparation TicketPreparation) (GroupOutcome, error) {
	outcome := GroupOutcome{GroupKey: group.Key}
	first := group.Items[0]
	findingKey, ownerID, mgmtOrgID := first.FindingKey, first.OwnerID, first.ManagementOrgID
	riskIDs := make([]int64, 0, len(group.Items))
	expected := make(map[int64]int64, len(group.Items))
	for _, item := range group.Items {
		riskIDs = append(riskIDs, item.RiskID)
		expected[item.RiskID] = item.Revision
	}

	var err error
	outcome.GroupOperationID = deriveGroupOperationID(input.OperationID, planHash, group.Key)

	// 守卫事务内复验派单授权（预览后撤权 → 整组拒绝）
	if err := d.deps.Authorizer.AuthorizeDispatch(ctx, tx, actorID, scope, riskIDs, target); err != nil {
		if errors.Is(err, errcode.ErrForbidden) {
			return *failGroup(&outcome, group, ItemDenied, ReasonDispatchForbidden), nil
		}
		return GroupOutcome{}, err
	}

	// 建票路径先锁 type/template→org（冻结桥序：组织锁绝不在 risk 锁之后）
	var locked LockedTicketCreate
	if live.ID == 0 && preparation != nil {
		if locked, err = d.deps.Tickets.LockRiskDispatchCreateTx(ctx, tx, preparation); err != nil {
			return GroupOutcome{}, fmt.Errorf("lock risk dispatch ticket: %w", err)
		}
	}

	// 组稳定键 advisory → 映射行 → 责任人 → 风险（升序）
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, advisoryGroupKey(findingKey, ownerID)); err != nil {
		return GroupOutcome{}, fmt.Errorf("lock dispatch group: %w", err)
	}
	if receipt, found, err := readGroupReceipt(ctx, tx, actorID, input.OperationID, group.Key); err != nil {
		return GroupOutcome{}, err
	} else if found {
		return d.projectReceipt(ctx, actorID, group.Key, receipt), nil
	}
	mapping, err := lockTargetMapping(ctx, tx, mgmtOrgID, target)
	if err != nil {
		if errors.Is(err, errcode.ErrForbidden) {
			reason := ReasonMappingDisabled
			if strings.Contains(err.Error(), "missing") {
				reason = ReasonMappingMissing
			}
			return *failGroup(&outcome, group, ItemDenied, reason), nil
		}
		return GroupOutcome{}, err
	}
	// 映射漂移（范围/目标配置变更）= 基线不符，重分组须新预览
	if groupIdentityKey(findingKey, ownerID, mgmtOrgID, mapping.ApprovedScope, target) != group.Key {
		return *failGroup(&outcome, group, ItemConflict, ReasonGroupScopeDrift), nil
	}
	ownerFact, err := d.deps.Owners.LockOwnerFact(ctx, tx, ownerID)
	if err != nil {
		return GroupOutcome{}, err
	}
	live, err = lockLiveGroup(ctx, tx, findingKey, ownerID)
	if err != nil {
		return GroupOutcome{}, err
	}
	facts, err := d.deps.Facts.LockDispatchFacts(ctx, tx, riskIDs)
	if err != nil {
		return GroupOutcome{}, err
	}
	factByID := make(map[int64]RiskFact, len(facts))
	for _, fact := range facts {
		factByID[fact.ID] = fact
	}
	if live.ID == 0 && preparation == nil {
		// 预读到在办组、事务内已不在（本切片无组终结路径）——环境竞态，整组可重试
		return GroupOutcome{}, fmt.Errorf("dispatch group vanished under lock: %w", errcode.ErrServiceUnavailable)
	}

	// —— 逐项现势复验 ——
	ticketRefFor := func(ticketID int64, snapshot string) *TicketReference {
		return d.viewTicket(ctx, actorID, ticketID, snapshot)
	}
	var eligible []RiskFact
	if ownerFact.Employed {
		for _, item := range group.Items {
			outcome.Items = append(outcome.Items, d.validateItem(ctx, tx, item, expected,
				factByID, ownerID, live.TicketID, ticketRefFor, &eligible))
		}
	} else {
		for _, item := range group.Items {
			outcome.Items = append(outcome.Items, ItemOutcome{
				RiskID: item.RiskID, Result: ItemDenied, Reason: ReasonOwnerUnavailable,
			})
		}
	}

	// 在办组兼容判定：存活组与重算键不一致 → 不并票、不另开并行在办票
	if live.ID > 0 && live.GroupKey != group.Key {
		for i := range outcome.Items {
			if outcome.Items[i].Result == ItemAlreadyLinked {
				continue
			}
			outcome.Items[i] = ItemOutcome{RiskID: outcome.Items[i].RiskID,
				Result: ItemConflict, Reason: ReasonGroupScopeDrift}
		}
		outcome.Status = "failed"
		return outcome, nil
	}

	// 余量：整组原子，不足不部分追加、不拆组
	if live.ID > 0 && len(eligible) > MaxAssetsPerTicket-int(live.ActiveLinks) {
		for i := range outcome.Items {
			if outcome.Items[i].Result == ItemAlreadyLinked {
				continue
			}
			outcome.Items[i] = ItemOutcome{RiskID: outcome.Items[i].RiskID,
				Result: ItemDenied, Reason: ReasonGroupCapacity}
		}
		outcome.Status = "failed"
		return outcome, nil
	}

	rejected := slices.ContainsFunc(outcome.Items, func(item ItemOutcome) bool {
		return item.Result != "" && item.Result != ItemAlreadyLinked
	})
	if rejected {
		for i := range outcome.Items {
			if outcome.Items[i].Result == "" {
				outcome.Items[i].Result = ItemConflict
				outcome.Items[i].Reason = ReasonGroupRejected
			}
		}
		outcome.Status = "failed"
		return outcome, nil
	}
	if len(eligible) == 0 {
		ticket := TicketOutcome{ID: live.TicketID, Ref: live.TicketRefSnapshot}
		priority, due, err := activeDispatchAggregate(ctx, tx, live.ID)
		if err != nil {
			return GroupOutcome{}, err
		}
		return d.completeGroupTx(ctx, tx, actorID, input.OperationID, outcome, ticket, false, ownerID,
			live.OwnerOrgAtDispatch, priority, due, now)
	}

	// 落票/追加 + 风险域状态历史 + 回执（同事务）
	var ticket TicketOutcome
	if live.ID > 0 {
		var status string
		if err := tx.QueryRow(ctx, "SELECT status FROM tickets WHERE id = $1 FOR UPDATE", live.TicketID).Scan(&status); err != nil {
			return GroupOutcome{}, err
		}
		// A new open link cannot be submitted while its ticket is awaiting
		// review. Existing rejection/rework returns it to in_progress; already
		// linked operations above remain readable at every stage.
		if status == "pending_verify" {
			for i := range outcome.Items {
				if outcome.Items[i].Result == "" {
					outcome.Items[i].Result = ItemConflict
					outcome.Items[i].Reason = ReasonTicketStageNotAppendable
				}
			}
			outcome.Status = "failed"
			return outcome, nil
		}
		ticket = TicketOutcome{ID: live.TicketID, Ref: live.TicketRefSnapshot}
		for _, fact := range eligible {
			if err := insertDispatchLinkTx(ctx, tx, fact.ID, live.ID, ownerID); err != nil {
				return GroupOutcome{}, err
			}
		}
	} else {
		// 组行在 link 回调内落（票已存在、file 绑定前）：组行携带完整票身份，
		// 满足组行 CHECK/FK 与冻结桥序（回调先于 file 绑定）
		link := func(ctx context.Context, tx pgx.Tx, ticketID int64) error {
			var ref *string
			if err := tx.QueryRow(ctx,
				`SELECT human_key FROM tickets WHERE id = $1`, ticketID).Scan(&ref); err != nil {
				return fmt.Errorf("read dispatched ticket ref: %w", err)
			}
			if ref == nil || *ref == "" {
				return fmt.Errorf("dispatched ticket %d has no human ref: %w", ticketID, errcode.ErrServiceUnavailable)
			}
			groupID, err := insertDispatchGroupTx(ctx, tx, findingKey, ownerID, mgmtOrgID, target,
				mapping.ApprovedScope, group.Key, TicketOutcome{ID: ticketID, Ref: *ref},
				actorID, ownerFact.PrimaryOrgID, group.BusinessDueAt)
			if err != nil {
				return err
			}
			for _, fact := range eligible {
				if err := insertDispatchLinkTx(ctx, tx, fact.ID, groupID, ownerID); err != nil {
					return err
				}
			}
			return nil
		}
		if ticket, err = d.deps.Tickets.CreateRiskDispatchTicketTx(ctx, tx, locked, ownerID, link); err != nil {
			return GroupOutcome{}, fmt.Errorf("create risk dispatch ticket: %w", err)
		}
	}

	commits := make([]DispatchCommitItem, 0, len(eligible))
	for _, fact := range eligible {
		commits = append(commits, DispatchCommitItem{
			RiskID:           fact.ID,
			ExpectedRevision: expected[fact.ID],
			TicketID:         ticket.ID,
			TicketRef:        ticket.Ref,
			GroupKey:         group.Key,
			OperationID:      input.OperationID,
			ActorID:          actorID,
			OccurredAt:       now,
		})
	}
	if err := d.deps.Commit.CommitDispatched(ctx, tx, commits); err != nil {
		return GroupOutcome{}, err
	}

	ownerOrg := ownerFact.PrimaryOrgID
	priority := group.Priority
	businessDue := group.BusinessDueAt
	if live.ID > 0 {
		ownerOrg = live.OwnerOrgAtDispatch
		priority, businessDue, err = activeDispatchAggregate(ctx, tx, live.ID)
		if err != nil {
			return GroupOutcome{}, err
		}
		if err := d.deps.Tickets.UpdateRiskDispatchPriorityTx(ctx, tx, ticket.ID, actorID, priority); err != nil {
			return GroupOutcome{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE risk_dispatch_groups SET business_due_at = $1 WHERE id = $2`, businessDue, live.ID); err != nil {
			return GroupOutcome{}, err
		}
	}
	return d.completeGroupTx(ctx, tx, actorID, input.OperationID, outcome, ticket, live.ID == 0,
		ownerID, ownerOrg, priority, businessDue, now)
}

func activeDispatchAggregate(ctx context.Context, tx pgx.Tx, groupID int64) (int, time.Time, error) {
	var priority *int
	var deadline *time.Time
	err := tx.QueryRow(ctx, `
		SELECT min(CASE r.level WHEN 'urgent' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 WHEN 'low' THEN 4 END),
		       min(r.deadline)
		FROM risk_ticket_links l JOIN risk_items r ON r.id = l.risk_id
		WHERE l.group_id = $1 AND l.ended_at IS NULL`, groupID).Scan(&priority, &deadline)
	if err != nil {
		return 0, time.Time{}, err
	}
	if priority == nil || deadline == nil {
		return 0, time.Time{}, errcode.ErrConflict
	}
	return *priority, *deadline, nil
}

func (d *Dispatcher) completeGroupTx(ctx context.Context, tx pgx.Tx, actorID int64, operationID string,
	outcome GroupOutcome, ticket TicketOutcome, created bool, ownerID, ownerOrg int64,
	priority int, businessDue, finishedAt time.Time) (GroupOutcome, error) {
	receipt := GroupReceipt{
		GroupOperationID: outcome.GroupOperationID, TicketID: ticket.ID, TicketRef: ticket.Ref,
		TicketCreated: created, OwnerUserID: ownerID, OwnerOrgAtDispatch: ownerOrg,
		Priority: priority, BusinessDueAt: businessDue.UTC(), CompletedAt: finishedAt,
	}
	for _, outcome := range outcome.Items {
		entry := GroupReceiptItem{RiskID: outcome.RiskID, Result: outcome.Result}
		switch entry.Result {
		case ItemAlreadyLinked:
			entry.Ticket = &TicketReference{ID: ticket.ID, Ref: ticket.Ref}
		case "":
			entry.Result = ItemLinked
			if created {
				entry.Result = ItemCreated
			}
		}
		receipt.Items = append(receipt.Items, entry)
	}
	if err := saveGroupReceiptTx(ctx, tx, actorID, operationID, outcome.GroupKey, receipt); err != nil {
		return GroupOutcome{}, err
	}
	return d.projectReceipt(ctx, actorID, outcome.GroupKey, receipt), nil
}

// validateItem 单风险现势复验：可行动项进 eligible，拒绝项给逐项结果。
func (d *Dispatcher) validateItem(ctx context.Context, tx pgx.Tx, item Candidate,
	expected map[int64]int64, factByID map[int64]RiskFact,
	ownerID, liveTicketID int64, ticketRefFor func(ticketID int64, snapshot string) *TicketReference,
	eligible *[]RiskFact) ItemOutcome {
	fact, found := factByID[item.RiskID]
	if !found {
		return ItemOutcome{RiskID: item.RiskID, Result: ItemInvalid, Reason: ReasonRiskMissing}
	}
	if fact.Revision != expected[item.RiskID] {
		return ItemOutcome{RiskID: item.RiskID, Result: ItemConflict, Reason: ReasonRevisionChanged}
	}
	if fact.OwnerID <= 0 {
		return ItemOutcome{RiskID: item.RiskID, Result: ItemInvalid, Reason: ReasonOwnerUnconfirmed}
	}
	if fact.OwnerID != ownerID {
		return ItemOutcome{RiskID: item.RiskID, Result: ItemConflict, Reason: ReasonOwnerChanged}
	}
	if fact.FindingKey != item.FindingKey || fact.AssetKey != item.AssetKey || fact.ManagementOrgID != item.ManagementOrgID {
		return ItemOutcome{RiskID: item.RiskID, Result: ItemConflict, Reason: ReasonIdentityChanged}
	}
	// 已有活跃关联先于状态判定短路：已入整改链的风险重选时按当前权限给原票核对，
	// 不重复建票（详设 §5.1）
	ticketID, linked, err := activeLinkTicket(ctx, tx, item.RiskID)
	if err != nil {
		return ItemOutcome{RiskID: item.RiskID, Result: ItemConflict, Reason: ReasonRiskMissing}
	}
	if linked {
		if liveTicketID == 0 || ticketID != liveTicketID {
			return ItemOutcome{RiskID: item.RiskID, Result: ItemConflict, Reason: ReasonGroupScopeDrift}
		}
		ref := ticketRefFor(ticketID, d.ticketRefSnapshot(ctx, tx, ticketID))
		return ItemOutcome{RiskID: item.RiskID, Result: ItemAlreadyLinked, Ticket: ref}
	}
	if !dispatchableStatuses[fact.Status] {
		return ItemOutcome{RiskID: item.RiskID, Result: ItemInvalid, Reason: ReasonStatusNotDispatchable}
	}
	*eligible = append(*eligible, fact)
	return ItemOutcome{RiskID: item.RiskID}
}

// ticketRefSnapshot 原票人读引用（活跃关联所在组的桥内快照）。
func (d *Dispatcher) ticketRefSnapshot(ctx context.Context, tx pgx.Tx, ticketID int64) string {
	var ref string
	_ = tx.QueryRow(ctx, `
		SELECT ticket_ref_snapshot FROM risk_dispatch_groups WHERE ticket_id = $1 LIMIT 1
	`, ticketID).Scan(&ref)
	return ref
}

func failGroup(outcome *GroupOutcome, group Group, result ItemResult, reason string) *GroupOutcome {
	for _, item := range group.Items {
		outcome.Items = append(outcome.Items, ItemOutcome{RiskID: item.RiskID, Result: result, Reason: reason})
	}
	outcome.Status = "failed"
	return outcome
}

// buildCreateRequest 受限建票请求：固定映射保留风险事实，assigned_to 一律不设
// （首片为 open 未指派票；桥合同禁止）。
func (d *Dispatcher) buildCreateRequest(group Group, target Target) *model.CreateTicketRequest {
	first := group.Items[0]
	title := "漏洞整改：" + first.Title
	if runes := []rune(title); len(runes) > 200 {
		title = string(runes[:200])
	}
	var lines strings.Builder
	for i, item := range group.Items {
		if i > 0 {
			lines.WriteString("\n")
		}
		lines.WriteString(item.HumanKey + " " + item.Title)
	}
	description := lines.String()
	return &model.CreateTicketRequest{
		TypeCode:          target.TypeCode,
		DefinitionVersion: strconv.FormatInt(target.DefinitionVersion, 10),
		DefinitionHash:    target.DefinitionHash,
		Title:             title,
		Description:       description,
		Priority:          group.Priority,
		OrgID:             target.OrganizationID,
	}
}
