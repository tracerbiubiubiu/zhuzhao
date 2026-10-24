package model

import (
	"encoding/json"
	"time"
)

// Ticket 工单主表模型
type Ticket struct {
	ID          int64           `json:"id,string" db:"id"`
	HumanKey    string          `json:"human_key,omitempty" db:"human_key"` // TK-yyyymmdd-NNNN
	TypeCode    string          `json:"type_code" db:"type_code"`
	Title       string          `json:"title" db:"title"`
	Description string          `json:"description" db:"description"`
	Priority    int             `json:"priority" db:"priority"`
	Status      string          `json:"status" db:"status"`
	CreatedBy   int64           `json:"created_by,string" db:"created_by"`
	AssignedTo  *int64          `json:"assigned_to,string,omitempty" db:"assigned_to"`
	OrgID       int64           `json:"org_id,string" db:"org_id"`
	OrgPath     string          `json:"org_path" db:"org_path"` // ltree 路径
	CustomData  json.RawMessage `json:"custom_data,omitempty" db:"custom_data"`
	SLADueAt    *time.Time      `json:"sla_due_at,omitempty" db:"sla_due_at"`
	// StatusChangedAt 进入当前状态时刻（S10 B10-4——E-2 拍定 a 案：UpdateStatusTx
	// 咽喉同写，节点停留时长数据源；存量回填=updated_at 近似）
	StatusChangedAt *time.Time `json:"status_changed_at,omitempty" db:"status_changed_at"`
	// EscalationSupervisor 催办上级·人工指定（S10 B10-4——E-1 拍定：人工指定为
	// 基线 > HR manager_id 为进阶[dd §31]；Create/Assign 可填可改）
	EscalationSupervisor *int64    `json:"escalation_supervisor_id,string,omitempty" db:"escalation_supervisor_id"`
	CreatedAt            time.Time `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time `json:"updated_at" db:"updated_at"`

	// 姓名回填（W4 随批件，03 S9：列表/详情处理人列 N+1 消除）——repo 层批量反查
	// users 非库列（db:"-"）；real_name 空回退 username，软删用户不回填（omitempty 隐去）。
	// 结构化消歧（2026-10-13 展示统一批）：*_no = 工号独立字段（非合并串），前端
	// UserNameText 统一渲染「人名(工号)」三态降级。
	// 四字段统一值类型 + omitempty（2026-10-14 契约审计 B3：原 Assignee 系 *string
	// 与 Created 系形态不对称——空工号时一侧字段消失一侧出 ""；「有指派」语义由
	// AssignedTo 承载，回填组无需 tri-state）
	CreatedByName string `json:"created_by_name,omitempty" db:"-"`
	CreatedByNo   string `json:"created_by_no,omitempty" db:"-"`
	AssigneeName  string `json:"assignee_name,omitempty" db:"-"`
	AssigneeNo    string `json:"assignee_no,omitempty" db:"-"`
}

// TicketCCRow 抄送行回显（S10 B10-1——repo 层 join users 富化；
// 「人名(工号)」结构化契约同 TicketRow 展示统一批口径：real_name 空回退 username、
// 软删用户不回填姓名但保留 ID 行——抄送关系本身可见）。
type TicketCCRow struct {
	UserID     int64     `json:"user_id,string"`
	Name       string    `json:"name,omitempty"`
	EmployeeNo string    `json:"employee_no,omitempty"`
	Role       string    `json:"role"`
	CreatedAt  time.Time `json:"created_at"`
}

// TagRow 标签行回显（S10 B10-2——默认共享+挂载人留痕；TagRepo join users 富化，
// 软删挂载人保留标签行不回填姓名——共享标签可溯源）。
type TagRow struct {
	Tag           string    `json:"tag"`
	CreatedBy     int64     `json:"created_by,string"`
	CreatedByName string    `json:"created_by_name,omitempty"`
	CreatedByNo   string    `json:"created_by_no,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// TicketType 工单类型配置
type TicketType struct {
	ID          int64           `json:"id,string" db:"id"`
	Code        string          `json:"code" db:"code"`
	Name        string          `json:"name" db:"name"`
	Description string          `json:"description" db:"description"`
	States      json.RawMessage `json:"states" db:"states"`
	Transitions json.RawMessage `json:"transitions" db:"transitions"`
	// TransitionMeta state 级节点元数据（S10 B10-4——timeout_minutes/escalation
	// 承载；S3a 后 "from>to" 转换级键复用同列）
	TransitionMeta  json.RawMessage `json:"transition_meta,omitempty" db:"transition_meta"`
	DefaultSLAHours int             `json:"default_sla_hours" db:"default_sla_hours"`
	HasCustomFields bool            `json:"has_custom_fields" db:"has_custom_fields"`
	IsActive        bool            `json:"is_active" db:"is_active"`
	// Version 乐观锁版本（P1-3）：每次更新 +1，客户端凭 GET 返回值做 CAS 提交
	Version   int64     `json:"version" db:"version"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// TicketTypeField 工单类型字段定义（动态表单）
type TicketTypeField struct {
	ID            int64           `json:"id,string" db:"id"`
	TypeCode      string          `json:"type_code" db:"type_code"`
	FieldKey      string          `json:"field_key" db:"field_key"`
	FieldLabel    string          `json:"field_label" db:"field_label"`
	FieldType     string          `json:"field_type" db:"field_type"`
	FieldOptions  json.RawMessage `json:"field_options,omitempty" db:"field_options"`
	FieldMeta     json.RawMessage `json:"field_meta,omitempty" db:"field_meta"` // B1-3 formula/extension
	Required      bool            `json:"required" db:"required"`
	ValidateRegex string          `json:"validate_regex,omitempty" db:"validate_regex"`
	SortOrder     int             `json:"sort_order" db:"sort_order"`
}

// TicketComment 工单回复/备注
type TicketComment struct {
	ID         int64     `json:"id,string" db:"id"`
	TicketID   int64     `json:"ticket_id,string" db:"ticket_id"`
	UserID     int64     `json:"user_id,string" db:"user_id"`
	Content    string    `json:"content" db:"content"`
	IsInternal bool      `json:"is_internal" db:"is_internal"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	// 作者回填（2026-10-13 展示统一批；此前 CreatedBy 裸 ID）——语义同 Ticket 姓名回填
	CreatedByName string `json:"created_by_name,omitempty" db:"-"`
	CreatedByNo   string `json:"created_by_no,omitempty" db:"-"`
}

// TicketEvent 工单事件日志（审计 + Phase 3 事件队列）
type TicketEvent struct {
	ID        int64     `json:"id,string" db:"id"`
	TicketID  int64     `json:"ticket_id,string" db:"ticket_id"`
	UserID    int64     `json:"user_id,string" db:"user_id"`
	Action    string    `json:"action" db:"action"`
	FromValue string    `json:"from_value,omitempty" db:"from_value"`
	ToValue   string    `json:"to_value,omitempty" db:"to_value"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// TicketTemplate 工单模板（2a 前移）
type TicketTemplate struct {
	ID                int64           `json:"id,string" db:"id"`
	Code              string          `json:"code" db:"code"`
	Name              string          `json:"name" db:"name"`
	TypeCode          string          `json:"type_code" db:"type_code"`
	DefaultPriority   int             `json:"default_priority" db:"default_priority"`
	DefaultFields     json.RawMessage `json:"default_fields,omitempty" db:"default_fields"`
	DefaultSLAMinutes *int            `json:"default_sla_minutes,omitempty" db:"default_sla_minutes"`
	// Version 乐观锁版本（P1-3）：每次更新 +1
	Version   int64      `json:"version" db:"version"`
	OrgID     int64      `json:"org_id,string" db:"org_id"`
	OrgPath   string     `json:"org_path" db:"org_path"`
	CreatedBy int64      `json:"created_by,string" db:"created_by"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

// TicketRelation 工单关联（2a 前移）
type TicketRelation struct {
	ID             int64      `json:"id,string" db:"id"`
	SourceTicketID int64      `json:"source_ticket_id,string" db:"source_ticket_id"`
	TargetTicketID int64      `json:"target_ticket_id,string" db:"target_ticket_id"`
	RelationType   string     `json:"relation_type" db:"relation_type"`
	CreatedBy      int64      `json:"created_by,string" db:"created_by"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

// AssignmentRule 自动分派规则（S3a B3-2——03 号附录 A.2）。
type AssignmentRule struct {
	ID         int64     `json:"id,string"`
	TypeCode   string    `json:"type_code"`
	Priority   int       `json:"priority"`
	Keyword    string    `json:"keyword,omitempty"` // 空=不限（类型兜底）；LIKE 包含
	TargetOrg  int64     `json:"target_org,string"`
	TargetUser *int64    `json:"target_user,string,omitempty"` // 决议③须属 target_org；空=仅分组织
	Enabled    bool      `json:"enabled"`
	CreatedBy  int64     `json:"created_by,string"`
	CreatedAt  time.Time `json:"created_at"`
}
