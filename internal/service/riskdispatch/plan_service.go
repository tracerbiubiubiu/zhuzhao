package riskdispatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// planCandidateLimit 首版候选总上限（服务端可收紧；超限要求分批，详设 §5.1）。
const planCandidateLimit = 200

// ReasonViewStale 预览请求携带的版本与现势不符（调用方刷新视图后重预览）。
const ReasonViewStale = "view_stale"

// PlanInput 预览输入：具名风险 + 乐观版本 + 目标组织/类型（详设 §5.1 候选输入）。
type PlanInput struct {
	Items          []RiskExpected
	TargetOrgID    int64
	TargetTypeCode string
}

// PlanItemView 组内风险项投影。
type PlanItemView struct {
	RiskID   int64               `json:"risk_id,string"`
	Revision int64               `json:"revision,string"`
	HumanKey string              `json:"human_key"`
	Title    string              `json:"title"`
	AssetRef *model.RiskAssetRef `json:"asset_ref,omitempty"`
	Deadline time.Time           `json:"deadline"`
}

// PlanGroupView 组投影；对已有存活在办组且当前仍获权的，给原票引用核对。
type PlanGroupView struct {
	GroupKey      string           `json:"group_key"`
	OwnerUserID   int64            `json:"owner_user_id,string"`
	Ticket        *TicketReference `json:"ticket,omitempty"`
	Items         []PlanItemView   `json:"items"`
	Priority      int              `json:"priority"`
	BusinessDueAt time.Time        `json:"business_due_at"`
}

// PlanBlockView 逐项阻断原因（仅已可见风险，详设 §5.1）。
type PlanBlockView struct {
	RiskID int64  `json:"risk_id,string"`
	Reason string `json:"reason"`
}

// PlanResult 预览结果：plan 是派单预览，不预留工单名额；提交仍按当时权限复检。
type PlanResult struct {
	PlanID    string          `json:"plan_id"`
	PlanHash  string          `json:"plan_hash"`
	ExpiresAt time.Time       `json:"expires_at"`
	Groups    []PlanGroupView `json:"groups"`
	Blocked   []PlanBlockView `json:"blocked,omitempty"`
}

// Plan 生成并持久化派单预览（actor 绑定 + 输入 hash + TTL）。
func (d *Dispatcher) Plan(ctx context.Context, actorID int64, input PlanInput) (PlanResult, error) {
	if len(input.Items) == 0 || len(input.Items) > planCandidateLimit ||
		input.TargetOrgID <= 0 || input.TargetTypeCode == "" {
		return PlanResult{}, errcode.ErrInvalidParams
	}
	var result PlanResult
	err := d.deps.Tx.WithDispatchTransaction(ctx, actorID, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		result, err = d.planTx(ctx, tx, actorID, input)
		return err
	})
	if err != nil {
		return PlanResult{}, err
	}
	return result, nil
}

// planTx keeps current authorization, fact reads, plan storage and projection in
// the owning IAM failure boundary. Preview grants no reservation or write right.
func (d *Dispatcher) planTx(ctx context.Context, tx pgx.Tx, actorID int64, input PlanInput) (PlanResult, error) {
	definition, err := d.deps.Definitions.CurrentPublished(ctx, tx, input.TargetTypeCode)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return PlanResult{}, fmt.Errorf("no published definition for %s: %w",
				input.TargetTypeCode, errcode.ErrConflict)
		}
		return PlanResult{}, err
	}
	target := Target{
		OrganizationID:    input.TargetOrgID,
		TypeCode:          input.TargetTypeCode,
		DefinitionVersion: definition.Version,
		DefinitionHash:    definition.Hash,
	}

	facts, err := d.deps.Facts.CurrentDispatchFacts(ctx, tx, actorID, input.Items)
	if err != nil {
		return PlanResult{}, err
	}
	// Invisible/nonexistent IDs stay undisclosed; visible stale items still
	// require current dispatch authority before any plan can be persisted.
	ids := make([]int64, 0, len(facts))
	for _, fact := range facts {
		ids = append(ids, fact.ID)
	}
	if len(ids) > 0 {
		scope := ScopeSingle
		if len(input.Items) > 1 {
			scope = ScopeBulk
		}
		if err := d.deps.Authorizer.AuthorizeDispatch(ctx, tx, actorID, scope, ids, target); err != nil {
			return PlanResult{}, err
		}
	}
	current := make(map[int64]RiskFact, len(facts))
	for _, fact := range facts {
		current[fact.ID] = fact
	}
	// 不可见/不存在：不返回也不报存在性；版本漂移：逐项给刷新原因
	var candidates []Candidate
	var stale []PlanBlockView
	for _, expected := range input.Items {
		fact, found := current[expected.ID]
		if !found {
			continue
		}
		if expected.Revision > 0 && expected.Revision != fact.Revision {
			stale = append(stale, PlanBlockView{RiskID: fact.ID, Reason: ReasonViewStale})
			continue
		}
		mapping, found, err := readTargetMapping(ctx, tx, fact.ManagementOrgID, target)
		if err != nil {
			return PlanResult{}, err
		}
		scope := ""
		if found && mapping.Enabled {
			scope = mapping.ApprovedScope
		}
		// 映射缺失/停用：空 scope 进分组器 → 逐项 incompatible_group_scope 阻断
		candidates = append(candidates, Candidate{
			RiskID:          fact.ID,
			Revision:        fact.Revision,
			DispatchBasis:   fact.DispatchBasis,
			FindingKey:      fact.FindingKey,
			AssetKey:        fact.AssetKey,
			AssetRef:        fact.AssetRef,
			OwnerID:         fact.OwnerID,
			ManagementOrgID: fact.ManagementOrgID,
			ApprovedScope:   scope,
			Priority:        fact.Priority,
			Deadline:        fact.Deadline,
			HumanKey:        fact.HumanKey,
			Title:           fact.Title,
		})
	}
	plan := BuildPlan(target, candidates)

	now := time.Now().UTC()
	bound, err := SavePlanTx(ctx, tx, actorID, target, plan, now)
	if err != nil {
		return PlanResult{}, err
	}

	result := PlanResult{
		PlanID:    bound.ID.String(),
		PlanHash:  bound.Hash,
		ExpiresAt: bound.ExpiresAt,
	}
	for _, group := range plan.Groups {
		view := PlanGroupView{
			GroupKey:      group.Key,
			OwnerUserID:   group.Items[0].OwnerID,
			Priority:      group.Priority,
			BusinessDueAt: group.BusinessDueAt,
		}
		for _, item := range group.Items {
			view.Items = append(view.Items, PlanItemView{
				RiskID: item.RiskID, Revision: item.Revision,
				HumanKey: item.HumanKey, Title: item.Title, AssetRef: item.AssetRef, Deadline: item.Deadline,
			})
		}
		// 已有存活在办组：预览即给原票引用（仍获权才显示，详设 §5.1）
		live, err := readLiveGroup(ctx, tx, group.Items[0].FindingKey, group.Items[0].OwnerID)
		if err != nil {
			return PlanResult{}, err
		}
		if live.ID > 0 {
			view.Ticket = d.viewTicket(ctx, actorID, live.TicketID, live.TicketRefSnapshot)
		}
		result.Groups = append(result.Groups, view)
	}
	for _, block := range plan.Blocked {
		result.Blocked = append(result.Blocked, PlanBlockView{RiskID: block.RiskID, Reason: string(block.Reason)})
	}
	result.Blocked = append(result.Blocked, stale...)
	return result, nil
}
