// Package riskdispatch plans human dispatch of verified vulnerabilities to individual owners.
package riskdispatch

import (
	"cmp"
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

const MaxAssetsPerTicket = 20

type Target = model.RiskDispatchTarget

// Candidate contains current risk facts already visible to the planner's actor.
type Candidate struct {
	RiskID          int64
	Revision        int64
	DispatchBasis   string // 风险域核实依据的稳定版本，仅保存于 plan 绑定
	FindingKey      string
	AssetKey        string
	AssetRef        *model.RiskAssetRef `json:",omitempty"`
	OwnerID         int64
	ManagementOrgID int64
	ApprovedScope   string
	Priority        int
	Deadline        time.Time
	HumanKey        string // 风险人读编号（建票映射与回执投影用；不参与分组键）
	Title           string // 风险标题（建票映射用；不参与分组键）
}

type Group struct {
	Key           string
	Items         []Candidate
	Priority      int
	BusinessDueAt time.Time
}

type Reason string

const (
	MissingPersonalOwner   Reason = "missing_personal_owner"
	IncompatibleGroupScope Reason = "incompatible_group_scope"
	GroupCapacityExceeded  Reason = "group_capacity_exceeded"
	UnverifiedRisk         Reason = "unverified_risk"
	DuplicateAsset         Reason = "duplicate_asset"
	DuplicateRiskID        Reason = "duplicate_risk_id"
)

type Block struct {
	RiskID int64
	Reason Reason
}

type Plan struct {
	Groups  []Group
	Blocked []Block
}

func BuildPlan(target Target, candidates []Candidate) Plan {
	var plan Plan
	seen := make(map[int64]bool, len(candidates))
	for _, candidate := range candidates {
		duplicateReported, exists := seen[candidate.RiskID]
		if exists && !duplicateReported {
			plan.Blocked = append(plan.Blocked, Block{RiskID: candidate.RiskID, Reason: DuplicateRiskID})
		}
		seen[candidate.RiskID] = exists
	}
	if len(plan.Blocked) > 0 {
		sortBlocks(plan.Blocked)
		return plan
	}
	type identity struct {
		findingKey string
		ownerID    int64
	}
	var identities []identity
	buckets := make(map[identity][]Candidate)
	for _, candidate := range candidates {
		if candidate.OwnerID <= 0 {
			plan.Blocked = append(plan.Blocked, Block{RiskID: candidate.RiskID, Reason: MissingPersonalOwner})
			continue
		}
		if !validDispatchBasis(candidate.DispatchBasis) {
			plan.Blocked = append(plan.Blocked, Block{RiskID: candidate.RiskID, Reason: UnverifiedRisk})
			continue
		}
		key := identity{findingKey: candidate.FindingKey, ownerID: candidate.OwnerID}
		if len(buckets[key]) == 0 {
			identities = append(identities, key)
		}
		buckets[key] = append(buckets[key], candidate)
	}
	slices.SortFunc(identities, func(left, right identity) int {
		if order := cmp.Compare(left.findingKey, right.findingKey); order != 0 {
			return order
		}
		return cmp.Compare(left.ownerID, right.ownerID)
	})
	for _, key := range identities {
		items := buckets[key]
		slices.SortFunc(items, func(left, right Candidate) int {
			return cmp.Compare(left.RiskID, right.RiskID)
		})
		group, reason := prepareGroup(target, items)
		if reason == "" {
			plan.Groups = append(plan.Groups, group)
			continue
		}
		for _, item := range items {
			plan.Blocked = append(plan.Blocked, Block{RiskID: item.RiskID, Reason: reason})
		}
	}
	sortBlocks(plan.Blocked)
	return plan
}

func validDispatchBasis(basis string) bool {
	return strings.TrimSpace(basis) != ""
}

func sortBlocks(blocks []Block) {
	slices.SortFunc(blocks, func(left, right Block) int {
		return cmp.Compare(left.RiskID, right.RiskID)
	})
}

func prepareGroup(target Target, items []Candidate) (Group, Reason) {
	if len(items) > MaxAssetsPerTicket {
		return Group{}, GroupCapacityExceeded
	}
	if target.OrganizationID <= 0 || target.TypeCode == "" || target.DefinitionVersion <= 0 || target.DefinitionHash == "" {
		return Group{}, IncompatibleGroupScope
	}
	first := items[0]
	seenAssets := make(map[string]struct{}, len(items))
	group := Group{Items: items, Priority: first.Priority, BusinessDueAt: first.Deadline}
	for _, item := range items {
		if item.RiskID <= 0 || item.Revision <= 0 || item.FindingKey == "" || item.AssetKey == "" ||
			item.Priority < 1 || item.Priority > 4 || item.Deadline.IsZero() {
			return Group{}, UnverifiedRisk
		}
		if item.ManagementOrgID != first.ManagementOrgID || item.ManagementOrgID <= 0 ||
			item.ApprovedScope != first.ApprovedScope || item.ApprovedScope == "" {
			return Group{}, IncompatibleGroupScope
		}
		if _, found := seenAssets[item.AssetKey]; found {
			return Group{}, DuplicateAsset
		}
		seenAssets[item.AssetKey] = struct{}{}
		group.Priority = min(group.Priority, item.Priority)
		if item.Deadline.Before(group.BusinessDueAt) {
			group.BusinessDueAt = item.Deadline
		}
	}
	group.Key = groupIdentityKey(first.FindingKey, first.OwnerID, first.ManagementOrgID, first.ApprovedScope, target)
	return group, ""
}

// groupIdentityKey 分组键输入：漏洞、责任人、管理组织、批准范围 + 目标组织、
// 类型、已发布定义（详设 §5.1）；apply 复算比对在办组是否同一基线。
func groupIdentityKey(findingKey string, ownerUserID, managementOrgID int64, approvedScope string, target Target) string {
	key := fmt.Sprintf("%q:%d:%d:%q:%d:%q:%d:%q",
		findingKey, ownerUserID, managementOrgID, approvedScope,
		target.OrganizationID, target.TypeCode, target.DefinitionVersion, target.DefinitionHash)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
}

// HandoffGroupKey computes the current dispatch compatibility key without
// overwriting the original dispatch identity and receipts.
func HandoffGroupKey(findingKey string, ownerUserID, managementOrgID int64, approvedScope string, target Target) string {
	return groupIdentityKey(findingKey, ownerUserID, managementOrgID, approvedScope, target)
}
