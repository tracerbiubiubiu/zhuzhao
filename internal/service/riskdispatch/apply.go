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
	ReasonRiskMissing               = "risk_missing"
	ReasonRevisionChanged           = "revision_changed"
	ReasonDispatchBasisUnavailable  = "dispatch_basis_unavailable"
	ReasonDispatchBasisChanged      = "dispatch_basis_changed"
	ReasonStatusNotDispatchable     = "status_not_dispatchable"
	ReasonOwnerUnconfirmed          = "owner_unconfirmed"
	ReasonOwnerChanged              = "owner_changed"
	ReasonOwnerUnavailable          = "owner_unavailable"
	ReasonIdentityChanged           = "risk_identity_changed"
	ReasonGroupScopeDrift           = "group_scope_drift"
	ReasonGroupCapacity             = "group_capacity_exceeded"
	ReasonMappingMissing            = "mapping_missing"
	ReasonMappingDisabled           = "mapping_disabled"
	ReasonDispatchForbidden         = "dispatch_forbidden"
	ReasonPlanExpired               = "plan_expired"
	ReasonTargetDefinitionChanged   = "target_definition_changed"
	ReasonTicketAssignmentForbidden = "ticket_assignment_forbidden"
	ReasonTicketAssignmentInvalid   = "ticket_assignment_invalid"
	ReasonGroupRejected             = "group_item_rejected"
	ReasonTicketStageNotAppendable  = "ticket_stage_not_appendable"
)

const (
	groupCompleted = "completed"
	groupFailed    = "failed"
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

// ApplyResult 整批结果与 parent 汇总（pending/partial/completed/failed）。
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
// 原号重投读原回执，只续尚无回执的组（详设 §5.1 批次合同）。
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

	result := ApplyResult{OperationID: input.OperationID, PlanID: input.PlanID.String()}
	for _, group := range groups {
		if receipt, found, err := readGroupReceipt(ctx, d.deps.DB, actorID, input.OperationID, group.Key); err != nil {
			return ApplyResult{}, err
		} else if found {
			result.Groups = append(result.Groups, d.projectReceipt(ctx, actorID, group.Key, receipt))
			continue
		}
		outcome, err := d.applyGroup(ctx, actorID, input, group, target, scope, plan.ExpiresAt)
		if err != nil {
			return ApplyResult{}, err
		}
		result.Groups = append(result.Groups, outcome)
	}
	result.Status = batchStatus(result.Groups, len(groups))
	return result, nil
}

// projectReceipt 投影原回执事实，按当下票读权遮蔽引用。
func (d *Dispatcher) projectReceipt(ctx context.Context, actorID int64, groupKey string, receipt GroupReceipt) GroupOutcome {
	outcome := GroupOutcome{
		GroupKey:           groupKey,
		GroupOperationID:   receipt.GroupOperationID,
		Status:             receipt.Status,
		Ticket:             d.viewTicket(ctx, actorID, receipt.TicketID, receipt.TicketRef),
		OwnerUserID:        receipt.OwnerUserID,
		OwnerOrgAtDispatch: receipt.OwnerOrgAtDispatch,
	}
	for _, item := range receipt.Items {
		projected := ItemOutcome{RiskID: item.RiskID, Result: item.Result, Reason: item.Reason}
		if item.Ticket != nil {
			projected.Ticket = d.viewTicket(ctx, actorID, item.Ticket.ID, item.Ticket.Ref)
		}
		outcome.Items = append(outcome.Items, projected)
	}
	return outcome
}

// applyGroup saves the first terminal receipt under the operation row lock.
// A failed validation writes only its receipt. Any execution error rolls the group back.
func (d *Dispatcher) applyGroup(ctx context.Context, actorID int64, input ApplyInput,
	group Group, target Target, scope DispatchScope, expiresAt time.Time) (GroupOutcome, error) {
	// The ticket bridge prepares outside the guard and locks type/org before the live group.
	// Pre-reading the live group avoids allocating a new ticket number on the append path.
	var live LiveGroup
	var preparation TicketPreparation
	var preparationErr error
	if time.Now().Before(expiresAt) {
		var err error
		live, err = readLiveGroup(ctx, d.deps.DB, group.Items[0].FindingKey, group.Items[0].OwnerID)
		if err != nil {
			return GroupOutcome{}, err
		}
		if live.ID == 0 {
			preparation, preparationErr = d.deps.Tickets.PrepareRiskDispatchCreate(ctx, d.buildCreateRequest(group, target), actorID)
		}
	}

	receipt, err := d.recordGroupReceipt(ctx, actorID, input.OperationID, group.Key, func(ctx context.Context, tx pgx.Tx) (GroupReceipt, error) {
		receipt := GroupReceipt{
			GroupOperationID: deriveGroupOperationID(input.OperationID, input.PlanHash, group.Key),
			Status:           groupFailed,
			CompletedAt:      time.Now().UTC(),
		}
		return d.applyGroupTx(ctx, tx, actorID, input.OperationID, group, target, scope, expiresAt,
			receipt, live, preparation, preparationErr)
	})
	var denied *ticketAssignmentRejection
	if errors.As(err, &denied) {
		// The bridge may already have staged ticket and link writes. The guard
		// has rolled those back. Recheck for a concurrently committed receipt
		// under the same lock before recording the rejection by itself.
		receipt, err = d.recordGroupReceipt(ctx, actorID, input.OperationID, group.Key, func(context.Context, pgx.Tx) (GroupReceipt, error) {
			return denied.receipt, nil
		})
	}
	if err != nil {
		return GroupOutcome{}, err
	}
	return d.projectReceipt(ctx, actorID, group.Key, receipt), nil
}

func (d *Dispatcher) recordGroupReceipt(ctx context.Context, actorID int64, operationID, groupKey string,
	execute func(context.Context, pgx.Tx) (GroupReceipt, error)) (GroupReceipt, error) {
	var receipt GroupReceipt
	err := d.deps.Tx.WithDispatchTransaction(ctx, actorID, func(ctx context.Context, tx pgx.Tx) error {
		var locked int
		err := tx.QueryRow(ctx, `
			SELECT 1 FROM risk_dispatch_operations WHERE actor_id = $1 AND operation_id = $2 FOR NO KEY UPDATE
		`, actorID, operationID).Scan(&locked)
		if err != nil {
			return fmt.Errorf("lock dispatch operation: %w", err)
		}
		stored, found, err := readGroupReceipt(ctx, tx, actorID, operationID, groupKey)
		if err != nil {
			return err
		}
		if found {
			receipt = stored
			return nil
		}
		receipt, err = execute(ctx, tx)
		if err != nil {
			return err
		}
		return saveGroupReceiptTx(ctx, tx, actorID, operationID, groupKey, receipt)
	})
	return receipt, err
}

func (d *Dispatcher) applyGroupTx(ctx context.Context, tx pgx.Tx, actorID int64, operationID string,
	group Group, target Target, scope DispatchScope, expiresAt time.Time, receipt GroupReceipt,
	live LiveGroup, preparation TicketPreparation, preparationErr error) (GroupReceipt, error) {
	first := group.Items[0]
	findingKey, ownerID, mgmtOrgID := first.FindingKey, first.OwnerID, first.ManagementOrgID
	riskIDs := make([]int64, 0, len(group.Items))
	expected := make(map[int64]int64, len(group.Items))
	for _, item := range group.Items {
		riskIDs = append(riskIDs, item.RiskID)
		expected[item.RiskID] = item.Revision
	}

	// 守卫事务内复验派单授权（预览后撤权 → 整组拒绝）
	if err := d.deps.Authorizer.AuthorizeDispatch(ctx, tx, actorID, scope, riskIDs, target); err != nil {
		if errors.Is(err, errcode.ErrForbidden) {
			return failGroup(receipt, group, ItemDenied, ReasonDispatchForbidden), nil
		}
		return GroupReceipt{}, err
	}
	if !receipt.CompletedAt.Before(expiresAt) {
		return failGroup(receipt, group, ItemConflict, ReasonPlanExpired), nil
	}
	if preparationErr != nil {
		if isTargetDefinitionDrift(preparationErr) {
			return failGroup(receipt, group, ItemConflict, ReasonTargetDefinitionChanged), nil
		}
		return GroupReceipt{}, fmt.Errorf("prepare risk dispatch ticket: %w", preparationErr)
	}

	// 建票路径先锁 type/template→org（冻结桥序：组织锁绝不在 risk 锁之后）
	var locked LockedTicketCreate
	if live.ID == 0 {
		var err error
		locked, err = d.deps.Tickets.LockRiskDispatchCreateTx(ctx, tx, preparation)
		if err != nil {
			if isTargetDefinitionDrift(err) {
				return failGroup(receipt, group, ItemConflict, ReasonTargetDefinitionChanged), nil
			}
			return GroupReceipt{}, fmt.Errorf("lock risk dispatch ticket: %w", err)
		}
	}

	// 组稳定键 advisory → 映射行 → 责任人 → 风险（升序）
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, advisoryGroupKey(findingKey, ownerID)); err != nil {
		return GroupReceipt{}, fmt.Errorf("lock dispatch group: %w", err)
	}
	mapping, err := lockTargetMapping(ctx, tx, mgmtOrgID, target)
	if err != nil {
		if errors.Is(err, errcode.ErrForbidden) {
			reason := ReasonMappingDisabled
			if strings.Contains(err.Error(), "missing") {
				reason = ReasonMappingMissing
			}
			return failGroup(receipt, group, ItemDenied, reason), nil
		}
		return GroupReceipt{}, err
	}
	// 映射漂移（范围/目标配置变更）= 基线不符，重分组须新预览
	if groupIdentityKey(findingKey, ownerID, mgmtOrgID, mapping.ApprovedScope, target) != group.Key {
		return failGroup(receipt, group, ItemConflict, ReasonGroupScopeDrift), nil
	}
	ownerFact, err := d.deps.Owners.LockOwnerFact(ctx, tx, ownerID)
	if err != nil {
		return GroupReceipt{}, err
	}
	live, err = lockLiveGroup(ctx, tx, findingKey, ownerID)
	if err != nil {
		return GroupReceipt{}, err
	}
	facts, err := d.deps.Facts.LockDispatchFacts(ctx, tx, riskIDs)
	if err != nil {
		return GroupReceipt{}, err
	}
	factByID := make(map[int64]RiskFact, len(facts))
	for _, fact := range facts {
		factByID[fact.ID] = fact
	}
	if live.ID == 0 && preparation == nil {
		// 预读到在办组、事务内已不在（本切片无组终结路径）——环境竞态，整组可重试
		return GroupReceipt{}, fmt.Errorf("dispatch group vanished under lock: %w", errcode.ErrServiceUnavailable)
	}

	// —— 逐项现势复验 ——
	var eligible []RiskFact
	if ownerFact.Employed {
		for _, item := range group.Items {
			result, err := validateItem(ctx, tx, item, factByID, ownerID, live, &eligible)
			if err != nil {
				return GroupReceipt{}, err
			}
			receipt.Items = append(receipt.Items, result)
		}
	} else {
		for _, item := range group.Items {
			receipt.Items = append(receipt.Items, GroupReceiptItem{
				RiskID: item.RiskID, Result: ItemDenied, Reason: ReasonOwnerUnavailable,
			})
		}
	}

	// 在办组兼容判定：存活组与重算键不一致 → 不并票、不另开并行在办票
	if live.ID > 0 && live.GroupKey != group.Key {
		for i := range receipt.Items {
			if receipt.Items[i].Result == ItemAlreadyLinked {
				continue
			}
			receipt.Items[i] = GroupReceiptItem{RiskID: receipt.Items[i].RiskID,
				Result: ItemConflict, Reason: ReasonGroupScopeDrift}
		}
		return receipt, nil
	}

	// 余量：整组原子，不足不部分追加、不拆组
	if live.ID > 0 && len(eligible) > MaxAssetsPerTicket-int(live.ActiveLinks) {
		for i := range receipt.Items {
			if receipt.Items[i].Result == ItemAlreadyLinked {
				continue
			}
			receipt.Items[i] = GroupReceiptItem{RiskID: receipt.Items[i].RiskID,
				Result: ItemDenied, Reason: ReasonGroupCapacity}
		}
		return receipt, nil
	}

	rejected := slices.ContainsFunc(receipt.Items, func(item GroupReceiptItem) bool {
		return item.Result != "" && item.Result != ItemAlreadyLinked
	})
	if rejected {
		for i := range receipt.Items {
			if receipt.Items[i].Result == "" {
				receipt.Items[i].Result = ItemConflict
				receipt.Items[i].Reason = ReasonGroupRejected
			}
		}
		return receipt, nil
	}
	if len(eligible) == 0 {
		ticket := TicketOutcome{ID: live.TicketID, Ref: live.TicketRefSnapshot}
		priority, due, err := activeDispatchAggregate(ctx, tx, live.ID)
		if err != nil {
			return GroupReceipt{}, err
		}
		return completeGroup(receipt, ticket, false, ownerID, live.OwnerOrgAtDispatch, priority, due), nil
	}

	// 落票/追加 + 风险域状态历史 + 回执（同事务）
	var ticket TicketOutcome
	if live.ID > 0 {
		var status string
		if err := tx.QueryRow(ctx, "SELECT status FROM tickets WHERE id = $1 FOR UPDATE", live.TicketID).Scan(&status); err != nil {
			return GroupReceipt{}, err
		}
		// A new open link cannot be submitted while its ticket is awaiting
		// review. Existing rejection/rework returns it to in_progress; already
		// linked operations above remain readable at every stage.
		if status == "pending_verify" {
			for i := range receipt.Items {
				if receipt.Items[i].Result == "" {
					receipt.Items[i].Result = ItemConflict
					receipt.Items[i].Reason = ReasonTicketStageNotAppendable
				}
			}
			return receipt, nil
		}
		ticket = TicketOutcome{ID: live.TicketID, Ref: live.TicketRefSnapshot}
		for _, fact := range eligible {
			if err := insertDispatchLinkTx(ctx, tx, fact.ID, live.ID, ownerID); err != nil {
				return GroupReceipt{}, err
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
			result, reason := ticketAssignmentFailure(err)
			if reason != "" {
				// Every item passed validation on the new-ticket path. Replace
				// provisional entries with the outcome of the rolled-back group.
				receipt.Items = nil
				return GroupReceipt{}, &ticketAssignmentRejection{receipt: failGroup(receipt, group, result, reason), cause: err}
			}
			return GroupReceipt{}, fmt.Errorf("create risk dispatch ticket: %w", err)
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
			OperationID:      operationID,
			ActorID:          actorID,
			OccurredAt:       receipt.CompletedAt,
		})
	}
	if err := d.deps.Commit.CommitDispatched(ctx, tx, commits); err != nil {
		return GroupReceipt{}, err
	}

	ownerOrg := ownerFact.PrimaryOrgID
	priority := group.Priority
	businessDue := group.BusinessDueAt
	if live.ID > 0 {
		ownerOrg = live.OwnerOrgAtDispatch
		priority, businessDue, err = activeDispatchAggregate(ctx, tx, live.ID)
		if err != nil {
			return GroupReceipt{}, err
		}
		if err := d.deps.Tickets.UpdateRiskDispatchPriorityTx(ctx, tx, ticket.ID, actorID, priority); err != nil {
			return GroupReceipt{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE risk_dispatch_groups SET business_due_at = $1 WHERE id = $2`, businessDue, live.ID); err != nil {
			return GroupReceipt{}, err
		}
	}
	return completeGroup(receipt, ticket, live.ID == 0, ownerID, ownerOrg, priority, businessDue), nil
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

func completeGroup(receipt GroupReceipt, ticket TicketOutcome, created bool, ownerID, ownerOrg int64,
	priority int, businessDue time.Time) GroupReceipt {
	receipt.Status = groupCompleted
	receipt.TicketID = ticket.ID
	receipt.TicketRef = ticket.Ref
	receipt.TicketCreated = created
	receipt.OwnerUserID = ownerID
	receipt.OwnerOrgAtDispatch = ownerOrg
	receipt.Priority = priority
	receipt.BusinessDueAt = businessDue.UTC()
	for i := range receipt.Items {
		if receipt.Items[i].Result == "" {
			receipt.Items[i].Result = ItemLinked
			if created {
				receipt.Items[i].Result = ItemCreated
			}
		}
	}
	return receipt
}

// validateItem 单风险现势复验：可行动项进 eligible，拒绝项给逐项结果。
func validateItem(ctx context.Context, tx pgx.Tx, item Candidate, factByID map[int64]RiskFact,
	ownerID int64, live LiveGroup, eligible *[]RiskFact) (GroupReceiptItem, error) {
	fact, found := factByID[item.RiskID]
	if !found {
		return GroupReceiptItem{RiskID: item.RiskID, Result: ItemInvalid, Reason: ReasonRiskMissing}, nil
	}
	if fact.Revision != item.Revision {
		return GroupReceiptItem{RiskID: item.RiskID, Result: ItemConflict, Reason: ReasonRevisionChanged}, nil
	}
	if fact.OwnerID <= 0 {
		return GroupReceiptItem{RiskID: item.RiskID, Result: ItemInvalid, Reason: ReasonOwnerUnconfirmed}, nil
	}
	if fact.OwnerID != ownerID {
		return GroupReceiptItem{RiskID: item.RiskID, Result: ItemConflict, Reason: ReasonOwnerChanged}, nil
	}
	if fact.FindingKey != item.FindingKey || fact.AssetKey != item.AssetKey || fact.ManagementOrgID != item.ManagementOrgID {
		return GroupReceiptItem{RiskID: item.RiskID, Result: ItemConflict, Reason: ReasonIdentityChanged}, nil
	}
	if !validDispatchBasis(fact.DispatchBasis) {
		return GroupReceiptItem{RiskID: item.RiskID, Result: ItemInvalid, Reason: ReasonDispatchBasisUnavailable}, nil
	}
	if fact.DispatchBasis != item.DispatchBasis {
		return GroupReceiptItem{RiskID: item.RiskID, Result: ItemConflict, Reason: ReasonDispatchBasisChanged}, nil
	}
	// 已有活跃关联先于状态判定短路：已入整改链的风险重选时按当前权限给原票核对。
	ticketID, linked, err := activeLinkTicket(ctx, tx, item.RiskID)
	if err != nil {
		return GroupReceiptItem{}, err
	}
	if linked {
		if live.TicketID == 0 || ticketID != live.TicketID {
			return GroupReceiptItem{RiskID: item.RiskID, Result: ItemConflict, Reason: ReasonGroupScopeDrift}, nil
		}
		return GroupReceiptItem{
			RiskID: item.RiskID, Result: ItemAlreadyLinked,
			Ticket: &TicketReference{ID: ticketID, Ref: live.TicketRefSnapshot},
		}, nil
	}
	if !dispatchableStatuses[fact.Status] {
		return GroupReceiptItem{RiskID: item.RiskID, Result: ItemInvalid, Reason: ReasonStatusNotDispatchable}, nil
	}
	*eligible = append(*eligible, fact)
	return GroupReceiptItem{RiskID: item.RiskID}, nil
}

func failGroup(receipt GroupReceipt, group Group, result ItemResult, reason string) GroupReceipt {
	for _, item := range group.Items {
		receipt.Items = append(receipt.Items, GroupReceiptItem{RiskID: item.RiskID, Result: result, Reason: reason})
	}
	return receipt
}

type ticketAssignmentRejection struct {
	receipt GroupReceipt
	cause   error
}

func (r *ticketAssignmentRejection) Error() string { return r.cause.Error() }
func (r *ticketAssignmentRejection) Unwrap() error { return r.cause }

func ticketAssignmentFailure(err error) (ItemResult, string) {
	switch {
	case errors.Is(err, errcode.ErrNoPermission), errors.Is(err, errcode.ErrForbidden):
		return ItemDenied, ReasonTicketAssignmentForbidden
	case errors.Is(err, errcode.ErrUserDisabled), errors.Is(err, errcode.ErrUserNotFound):
		return ItemDenied, ReasonOwnerUnavailable
	case errors.Is(err, errcode.ErrTicketInvalidTransition):
		return ItemConflict, ReasonTicketAssignmentInvalid
	default:
		return "", ""
	}
}

func isTargetDefinitionDrift(err error) bool {
	return errors.Is(err, errcode.ErrConcurrentModification) || errors.Is(err, errcode.ErrTicketTypeNotFound)
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
