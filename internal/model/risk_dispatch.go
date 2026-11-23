package model

import "time"

// Ordinary ticket and notification projections use these summaries. Risk titles
// and descriptions belong to the risk object's authorized read surface.
const (
	RiskRemediationTicketTitle       = "漏洞整改"
	RiskRemediationTicketDescription = "请在风险模块查看有权访问的整改事项。"
)

// RiskDispatchTarget is a configured target organization and pinned definition.
type RiskDispatchTarget struct {
	OrganizationID    int64
	TypeCode          string
	DefinitionVersion int64
	DefinitionHash    string
}

type RiskDispatchScope string

const (
	RiskDispatchSingle RiskDispatchScope = "single"
	RiskDispatchBulk   RiskDispatchScope = "bulk"
)

// RiskDispatchExpected captures the planner’s expected revision.
type RiskDispatchExpected struct {
	ID       int64
	Revision int64
}

// RiskDispatchFact 风险域提供的现势已核实事实（读侧投影；risk_items 为风险域唯一写主）。
type RiskDispatchFact struct {
	ID              int64
	Revision        int64
	HumanKey        string
	Status          string
	OwnerID         int64 // 0 = 责任待确认（不可派）
	ManagementOrgID int64
	AffectedOrgID   int64
	Priority        int // 1..4（urgent..low）
	Deadline        time.Time
	FindingKey      string
	AssetKey        string // provider + ":" + external_id，资产键身份
	AssetRef        *RiskAssetRef
	Title           string
	Description     string
	SystemRef       string
}

// RiskDispatchCommitItem is a dispatch write owned by the risk service.
type RiskDispatchCommitItem struct {
	RiskID           int64
	ExpectedRevision int64
	TicketID         int64
	TicketRef        string
	GroupKey         string
	OperationID      string
	ActorID          int64
	OccurredAt       time.Time
}
