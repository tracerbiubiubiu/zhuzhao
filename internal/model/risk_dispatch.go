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
	// DispatchBasis 当前有效派单依据的稳定核实事实版本，空 = 不可派
	// （未评估 / 漏洞-资产身份缺失 / 来源镜像撤回或缺失）。
	// 人工源 = man1:<risk_id>:<最近 assessed 事件 revision>（owner 确认不使依据失效）；
	// 合成来源 = src1:<现势镜像 content_hash>（新断言换版本、撤回即空）。
	// 由 risk/dispatch_port.go 读/锁两路径从现势事实填入；
	// Candidate 绑定与预览/提交比较归派单助手（riskdispatch）。
	DispatchBasis string
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
