package model

import (
	"encoding/json"
	"time"
)

// Phase 6 B2 第 6 项（D08/D09）风险登记与核实域。
// wire 约定（详设 risk-module-detail §2）：ID/Revision 均十进制字符串，
// Instant 为 RFC3339；本文件为 HTTP data 的目标类型。

// RiskEvidenceRef 证据引用（provider/external_id/version? 对象——生命周期合同 §8.1）。
type RiskEvidenceRef struct {
	Provider   string `json:"provider" binding:"required"`
	ExternalID string `json:"external_id" binding:"required"`
	Version    string `json:"version,omitempty"`
}

// RiskAssetRef 资产键身份（外部 CMDB 或明确临时映射；原样保留不另造权威）。
type RiskAssetRef struct {
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}

// RiskSourceSeverity 来源原始严重性（scheme+value 原样并列，不做本地映射）。
type RiskSourceSeverity struct {
	Scheme string `json:"scheme"`
	Value  string `json:"value"`
}

// RiskRef 风险引用（详设 RiskRef）。
type RiskRef struct {
	ID       int64  `json:"id,string"`
	HumanKey string `json:"human_key"`
	Revision int64  `json:"revision,string"`
}

// RiskReceipt durable 操作收据（risk_operations 行的投影）。
type RiskReceipt struct {
	OperationID string    `json:"operation_id"`
	Command     string    `json:"command"`
	Status      string    `json:"status"`
	RiskID      int64     `json:"risk_id,string,omitempty"`
	Revision    int64     `json:"revision,string,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	Replayed    bool      `json:"replayed"` // true=同号同参返回原收据，未产生新业务效果
}

// RiskMutationResult 写命令统一返回（create/assess/assign）。
type RiskMutationResult struct {
	Risk         RiskRef     `json:"risk"`
	Status       string      `json:"status"`
	Level        *string     `json:"level,omitempty"`
	Deadline     *time.Time  `json:"deadline,omitempty"`
	InitDeadline *time.Time  `json:"init_deadline,omitempty"`
	Receipt      RiskReceipt `json:"receipt_ref"`
}

// RiskCreateRequest 人工登记（POST /risk-items/create；详设 §2.1）。
// 人工源命名空间由服务端建立，不接受外部 source_ref。
type RiskCreateRequest struct {
	OperationID      string            `json:"operation_id" binding:"required,max=128"`
	Title            string            `json:"title" binding:"required,max=200"`
	Description      string            `json:"description"`
	OrgID            int64             `json:"org_id,string" binding:"required"`
	AffectedOrgID    *int64            `json:"affected_org_id,string,omitempty"`
	OwnerUserID      *int64            `json:"owner_user_id,string,omitempty"`
	SystemRef        string            `json:"system_ref,omitempty,max=200"`
	FindingKey       string            `json:"finding_key,omitempty" binding:"omitempty,max=200"` // 稳定漏洞键；与 AssetRef 成对
	AssetRef         *RiskAssetRef     `json:"asset_ref,omitempty"`                               // 资产键身份；与 FindingKey 成对
	EvidenceRefs     []RiskEvidenceRef `json:"evidence_refs,omitempty"`
	ExpectedRevision string            `json:"expected_revision,omitempty"` // 新建恒 "0"
	Attributes       json.RawMessage   `json:"attributes,omitempty" swaggertype:"object"`
}

// RiskAssessRequest 核实/评估（POST /risk-items/assess；详设 §2.2）。
type RiskAssessRequest struct {
	OperationID      string     `json:"operation_id" binding:"required,max=128"`
	ID               int64      `json:"id,string" binding:"required"`
	ExpectedRevision string     `json:"expected_revision" binding:"required"`
	Level            string     `json:"level" binding:"required"`
	AssessmentNote   string     `json:"assessment_note"`
	Treatment        string     `json:"treatment" binding:"required"`
	Deadline         *time.Time `json:"deadline,omitempty"`
}

// RiskAssignRequest 责任确认（POST /risk-items/assign；详设 §2.2 主锚管理者）。
type RiskAssignRequest struct {
	OperationID      string `json:"operation_id" binding:"required,max=128"`
	ID               int64  `json:"id,string" binding:"required"`
	ExpectedRevision string `json:"expected_revision" binding:"required"`
	OwnerUserID      int64  `json:"owner_user_id,string" binding:"required"`
	Reason           string `json:"reason" binding:"required"`
}

// FindingObservationV1 规范化来源观察（生命周期合同 §8.1 强 schema 字段；
// 不接受 local_status/owner/acceptance/created_by 等源写字段）。
type FindingObservationV1 struct {
	SourceRef       string              `json:"source_ref" binding:"required"`
	SourceItemKey   string              `json:"source_item_key" binding:"required,max=300"`
	EventID         string              `json:"event_id" binding:"required,max=200"`
	SourceSequence  string              `json:"source_sequence,omitempty"`
	ObservedAt      time.Time           `json:"observed_at" binding:"required"`
	ObservationKind string              `json:"observation_kind,omitempty"` // observed|withdrawn|deleted，缺省 observed
	AssetRef        *RiskAssetRef       `json:"asset_ref,omitempty"`
	FindingClass    string              `json:"finding_class,omitempty"`
	FindingKey      string              `json:"finding_key,omitempty,max=200"`
	Port            *int                `json:"port,omitempty"`
	SourceSeverity  *RiskSourceSeverity `json:"source_severity,omitempty"`
	ConclusionRef   string              `json:"conclusion_ref,omitempty"`
	EvidenceRefs    []RiskEvidenceRef   `json:"evidence_refs,omitempty"`
	MappingRef      string              `json:"mapping_ref" binding:"required"`
	Attributes      json.RawMessage     `json:"attributes,omitempty" swaggertype:"object"`
}

// RiskReportRequest 合成来源上报（POST /risk-items/report；单条或 batch≤100）。
// 单条观察由 handler 归一为 Observations 后进入服务层。
type RiskReportRequest struct {
	BatchID      string                 `json:"batch_id,omitempty,max=128"`
	Observations []FindingObservationV1 `json:"observations,omitempty"`
}

// RiskReportItem 上报逐项结果（详设 §2.1 report 返回）。
type RiskReportItem struct {
	EventID    string   `json:"event_id"`
	IngestID   string   `json:"ingest_id"`
	Result     string   `json:"result"`     // accepted|duplicate|stale|quarantined|held
	Processing string   `json:"processing"` // pending|applied|held
	RiskRef    *RiskRef `json:"risk_ref,omitempty"`
	SafeReason string   `json:"safe_reason,omitempty"`
}

// RiskReportResult 上报整体返回。
type RiskReportResult struct {
	BatchReceipt string           `json:"batch_receipt"`
	Items        []RiskReportItem `json:"items"`
}

// RiskView 风险视图（详设 RiskView 本批字段）。
// dispatch_blockers 显式给出派单准备阻断（第 7 项派单预览消费者据此拒绝）。
type RiskView struct {
	Risk             RiskRef             `json:"risk"`
	Title            string              `json:"title"`
	Status           string              `json:"status"`
	ClosureKind      *string             `json:"closure_kind"`
	Level            *string             `json:"level"`
	OwnerUserID      *int64              `json:"owner_user_id,string"`
	OrgID            int64               `json:"org_id,string"`
	AffectedOrgID    *int64              `json:"affected_org_id,string"`
	Deadline         *time.Time          `json:"deadline"`
	InitDeadline     *time.Time          `json:"init_deadline"`
	Treatment        *string             `json:"treatment"`
	SourceSeverity   *RiskSourceSeverity `json:"source_severity"`
	AssetRef         *RiskAssetRef       `json:"asset_ref"`
	FindingKey       string              `json:"finding_key"`
	DispatchBlockers []string            `json:"dispatch_blockers"`
}

// RiskDetailView 详情（列表字段 + 有界扩展）。
type RiskDetailView struct {
	View          RiskView        `json:"view"`
	Description   string          `json:"description"`
	SystemRef     string          `json:"system_ref"`
	EvidenceRefs  json.RawMessage `json:"evidence_refs" swaggertype:"array,object"`
	Attributes    json.RawMessage `json:"attributes" swaggertype:"object"`
	SourceSummary json.RawMessage `json:"source_summary" swaggertype:"object"`
}

// RiskListQuery 列表查询（详设 §2.1 参数面本批子集）。
type RiskListQuery struct {
	Page          int    `form:"page"`
	PageSize      int    `form:"page_size"`
	Status        string `form:"status"`
	Level         string `form:"level"`
	OwnerUserID   *int64 `form:"owner_user_id,string"`
	AffectedOrgID *int64 `form:"affected_org_id,string"`
	SourceRef     string `form:"source_ref"`
	Q             string `form:"q"`
	Sort          string `form:"sort"` // created_at|deadline|level
}

// RiskListResult 列表返回（先授权过滤再分页计数）。
type RiskListResult struct {
	Items    []RiskView `json:"items"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
}

// RiskEventView 风险域追加事件（核实/责任/来源应用历史）。
type RiskEventView struct {
	ID                int64           `json:"id,string"`
	RiskID            int64           `json:"risk_id,string"`
	AggregateRevision int64           `json:"aggregate_revision,string"`
	EventKind         string          `json:"event_kind"`
	Ordinal           int             `json:"ordinal"`
	ActorID           int64           `json:"actor_id,string"`
	OccurredAt        time.Time       `json:"occurred_at"`
	OperationID       string          `json:"operation_id,omitempty"`
	Payload           json.RawMessage `json:"payload" swaggertype:"object"`
}

// RiskHistoryResult 历史分页返回。
type RiskHistoryResult struct {
	Items    []RiskEventView `json:"items"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

// RiskIngestReceiptView 来源收件回执（丢响应后按原 ingest_id 受权核查）。
type RiskIngestReceiptView struct {
	IngestID   string    `json:"ingest_id"`
	SourceRef  string    `json:"source_ref"`
	EventID    string    `json:"event_id"`
	Result     string    `json:"result"`
	Processing string    `json:"processing"`
	SafeReason string    `json:"safe_reason,omitempty"`
	ReceivedAt time.Time `json:"received_at"`
	RiskRef    *RiskRef  `json:"risk_ref,omitempty"`
}
