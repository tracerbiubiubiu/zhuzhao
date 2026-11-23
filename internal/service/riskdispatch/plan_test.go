package riskdispatch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildPlanGroupsByFindingAndPersonalOwner(t *testing.T) {
	deadline := time.Date(2026, 11, 22, 17, 0, 0, 0, time.UTC)
	target := Target{OrganizationID: 30, TypeCode: "synthetic_remediation", DefinitionVersion: 1, DefinitionHash: "published"}
	candidates := []Candidate{
		{RiskID: 4, Revision: 7, DispatchBasis: "synthetic-basis-v1", FindingKey: "finding-002", AssetKey: "asset-a", OwnerID: 11, ManagementOrgID: 10, ApprovedScope: "scope-1", Priority: 3, Deadline: deadline},
		{RiskID: 2, Revision: 5, DispatchBasis: "synthetic-basis-v1", FindingKey: "finding-001", AssetKey: "asset-b", OwnerID: 11, ManagementOrgID: 10, ApprovedScope: "scope-1", Priority: 3, Deadline: deadline},
		{RiskID: 3, Revision: 6, DispatchBasis: "synthetic-basis-v1", FindingKey: "finding-001", AssetKey: "asset-c", OwnerID: 12, ManagementOrgID: 10, ApprovedScope: "scope-1", Priority: 3, Deadline: deadline},
		{RiskID: 1, Revision: 4, DispatchBasis: "synthetic-basis-v1", FindingKey: "finding-001", AssetKey: "asset-a", OwnerID: 11, ManagementOrgID: 10, ApprovedScope: "scope-1", Priority: 2, Deadline: deadline.Add(-time.Hour)},
	}
	plan := BuildPlan(target, candidates)

	require.Empty(t, plan.Blocked)
	require.Len(t, plan.Groups, 3)
	require.Equal(t, []Candidate{candidates[3], candidates[1]}, plan.Groups[0].Items)
	require.Equal(t, 2, plan.Groups[0].Priority)
	require.Equal(t, deadline.Add(-time.Hour), plan.Groups[0].BusinessDueAt)
	require.Equal(t, []Candidate{candidates[2]}, plan.Groups[1].Items)
	require.Equal(t, []Candidate{candidates[0]}, plan.Groups[2].Items)
	require.NotEqual(t, plan.Groups[0].Key, plan.Groups[1].Key)
	require.NotEqual(t, plan.Groups[0].Key, plan.Groups[2].Key)

	reversed := []Candidate{candidates[3], candidates[2], candidates[1], candidates[0]}
	require.Equal(t, plan, BuildPlan(target, reversed))
}

func TestBuildPlanBlocksUnknownOwnerAndIncompatibleGroupScope(t *testing.T) {
	deadline := time.Date(2026, 11, 22, 17, 0, 0, 0, time.UTC)
	target := Target{OrganizationID: 30, TypeCode: "synthetic_remediation", DefinitionVersion: 1, DefinitionHash: "published"}
	candidates := []Candidate{
		{RiskID: 1, Revision: 1, DispatchBasis: "synthetic-basis-v1", FindingKey: "finding-001", AssetKey: "asset-a", OwnerID: 11, ManagementOrgID: 10, ApprovedScope: "scope-1", Priority: 2, Deadline: deadline},
		{RiskID: 2, Revision: 1, DispatchBasis: "synthetic-basis-v1", FindingKey: "finding-001", AssetKey: "asset-b", OwnerID: 11, ManagementOrgID: 20, ApprovedScope: "scope-2", Priority: 2, Deadline: deadline},
		{RiskID: 3, Revision: 1, DispatchBasis: "synthetic-basis-v1", FindingKey: "finding-001", AssetKey: "asset-c", ManagementOrgID: 10, ApprovedScope: "scope-1", Priority: 2, Deadline: deadline},
	}

	plan := BuildPlan(target, candidates)

	require.Empty(t, plan.Groups)
	require.Equal(t, []Block{
		{RiskID: 1, Reason: IncompatibleGroupScope},
		{RiskID: 2, Reason: IncompatibleGroupScope},
		{RiskID: 3, Reason: MissingPersonalOwner},
	}, plan.Blocked)
}

func TestBuildPlanDoesNotSplitFullGroup(t *testing.T) {
	deadline := time.Date(2026, 11, 22, 17, 0, 0, 0, time.UTC)
	target := Target{OrganizationID: 30, TypeCode: "synthetic_remediation", DefinitionVersion: 1, DefinitionHash: "published"}
	var candidates []Candidate
	for index := range 21 {
		candidates = append(candidates, Candidate{
			RiskID: int64(index + 1), Revision: 1, DispatchBasis: "synthetic-basis-v1", FindingKey: "finding-001", AssetKey: string(rune('a' + index)),
			OwnerID: 11, ManagementOrgID: 10, ApprovedScope: "scope-1", Priority: 2, Deadline: deadline,
		})
	}

	full := BuildPlan(target, candidates[:MaxAssetsPerTicket])
	require.Empty(t, full.Blocked)
	require.Len(t, full.Groups, 1)
	require.Len(t, full.Groups[0].Items, MaxAssetsPerTicket)

	plan := BuildPlan(target, candidates)

	require.Empty(t, plan.Groups)
	require.Len(t, plan.Blocked, 21)
	for _, block := range plan.Blocked {
		require.Equal(t, GroupCapacityExceeded, block.Reason)
	}
}

func TestBuildPlanRejectsDuplicateRiskIDBeforeGrouping(t *testing.T) {
	deadline := time.Date(2026, 11, 22, 17, 0, 0, 0, time.UTC)
	target := Target{OrganizationID: 30, TypeCode: "synthetic_remediation", DefinitionVersion: 1, DefinitionHash: "published"}
	original := Candidate{
		RiskID: 1, Revision: 4, DispatchBasis: "synthetic-basis-v1", FindingKey: "finding-001", AssetKey: "asset-a",
		OwnerID: 11, ManagementOrgID: 10, ApprovedScope: "scope-1", Priority: 2, Deadline: deadline,
	}
	conflicting := original
	conflicting.AssetKey = "asset-b"
	conflicting.OwnerID = 12

	plan := BuildPlan(target, []Candidate{original, conflicting})

	require.Empty(t, plan.Groups)
	require.Len(t, plan.Blocked, 1)
	require.Equal(t, original.RiskID, plan.Blocked[0].RiskID)
	require.Equal(t, Reason("duplicate_risk_id"), plan.Blocked[0].Reason)
	require.Equal(t, plan, BuildPlan(target, []Candidate{conflicting, original}))
}
