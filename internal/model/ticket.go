package model

import (
	"encoding/json"
	"time"
)

// Ticket 工单主表模型
type Ticket struct {
	ID          int64           `json:"id,string" db:"id"`
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
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

// TicketType 工单类型配置
type TicketType struct {
	ID              int64           `json:"id,string" db:"id"`
	Code            string          `json:"code" db:"code"`
	Name            string          `json:"name" db:"name"`
	Description     string          `json:"description" db:"description"`
	States          json.RawMessage `json:"states" db:"states"`
	Transitions     json.RawMessage `json:"transitions" db:"transitions"`
	DefaultSLAHours int             `json:"default_sla_hours" db:"default_sla_hours"`
	HasCustomFields bool            `json:"has_custom_fields" db:"has_custom_fields"`
	IsActive        bool            `json:"is_active" db:"is_active"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
}

// TicketTypeField 工单类型字段定义（动态表单）
type TicketTypeField struct {
	ID           int64           `json:"id,string" db:"id"`
	TypeCode     string          `json:"type_code" db:"type_code"`
	FieldKey     string          `json:"field_key" db:"field_key"`
	FieldLabel   string          `json:"field_label" db:"field_label"`
	FieldType    string          `json:"field_type" db:"field_type"`
	FieldOptions json.RawMessage `json:"field_options,omitempty" db:"field_options"`
	Required     bool            `json:"required" db:"required"`
	SortOrder    int             `json:"sort_order" db:"sort_order"`
}

// TicketComment 工单回复/备注
type TicketComment struct {
	ID          int64     `json:"id,string" db:"id"`
	TicketID   int64     `json:"ticket_id,string" db:"ticket_id"`
	UserID     int64     `json:"user_id,string" db:"user_id"`
	Content    string    `json:"content" db:"content"`
	IsInternal bool      `json:"is_internal" db:"is_internal"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
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
	Code              string           `json:"code" db:"code"`
	Name              string           `json:"name" db:"name"`
	TypeCode          string           `json:"type_code" db:"type_code"`
	DefaultPriority   int             `json:"default_priority" db:"default_priority"`
	DefaultFields     json.RawMessage `json:"default_fields,omitempty" db:"default_fields"`
	DefaultSLAMinutes *int            `json:"default_sla_minutes,omitempty" db:"default_sla_minutes"`
	OrgID             int64           `json:"org_id,string" db:"org_id"`
	OrgPath           string          `json:"org_path" db:"org_path"`
	CreatedBy         int64           `json:"created_by,string" db:"created_by"`
	CreatedAt         time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at" db:"updated_at"`
	DeletedAt         *time.Time      `json:"deleted_at,omitempty" db:"deleted_at"`
}

// TicketRelation 工单关联（2a 前移）
type TicketRelation struct {
	ID             int64     `json:"id,string" db:"id"`
	SourceTicketID int64     `json:"source_ticket_id,string" db:"source_ticket_id"`
	TargetTicketID int64     `json:"target_ticket_id,string" db:"target_ticket_id"`
	RelationType   string    `json:"relation_type" db:"relation_type"`
	CreatedBy      int64     `json:"created_by,string" db:"created_by"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}
