package model

import "encoding/json"

// CreateTicketRequest 创建工单
type CreateTicketRequest struct {
	TypeCode     string          `json:"type_code" binding:"required"`
	Title        string          `json:"title" binding:"required"`
	Description  string          `json:"description"`
	Priority     int             `json:"priority"`
	AssignedTo   *int64          `json:"assigned_to,string,omitempty"`
	OrgID        int64           `json:"org_id,string" binding:"required"`
	TemplateCode string          `json:"template_code,omitempty"` // 可选：命中模板则预填
	CustomData   json.RawMessage `json:"custom_data,omitempty"`
}

// UpdateTicketRequest 更新工单（POST /tickets/update，id 放 body）
type UpdateTicketRequest struct {
	ID          int64   `json:"id,string" binding:"required"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Priority    *int    `json:"priority,omitempty"`
}

// AssignTicketRequest 分派工单（POST /tickets/assign）
type AssignTicketRequest struct {
	ID         int64  `json:"id,string" binding:"required"`
	AssignedTo *int64 `json:"assigned_to,string,omitempty"` // nil = 取消分派
}

// CloseTicketRequest 关闭工单（POST /tickets/close）
type CloseTicketRequest struct {
	ID      int64  `json:"id,string" binding:"required"`
	Comment string `json:"comment,omitempty"` // 可选关闭说明
}

// DeleteTicketRequest 删除工单（POST /tickets/delete）
type DeleteTicketRequest struct {
	ID int64 `json:"id,string" binding:"required"`
}

// CreateCommentRequest 创建公开回复（POST /tickets/comments）
type CreateCommentRequest struct {
	TicketID int64  `json:"ticket_id,string" binding:"required"`
	Content  string `json:"content" binding:"required"`
}

// CreateNoteRequest 创建内部备注（POST /tickets/notes）
type CreateNoteRequest struct {
	TicketID int64  `json:"ticket_id,string" binding:"required"`
	Content  string `json:"content" binding:"required"`
}

// CreateRelationRequest 建立工单关联（POST /tickets/relations）
type CreateRelationRequest struct {
	SourceTicketID int64  `json:"source_ticket_id,string" binding:"required"`
	TargetTicketID int64  `json:"target_ticket_id,string" binding:"required"`
	RelationType   string `json:"relation_type,omitempty"` // 默认 related
}

// TicketListResponse 工单列表响应
type TicketListResponse struct {
	List     []*Ticket `json:"list"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}

// TicketListQuery 工单列表筛选
type TicketListQuery struct {
	Page     int
	PageSize int
	TypeCode string
	Status   string
	Priority *int
}

// ===== IW3/BK-18：类型/字段/模板管理 =====

// CreateTicketTypeRequest 新建工单类型（states/transitions 缺省 6 态默认图）
type CreateTicketTypeRequest struct {
	Code        string          `json:"code" binding:"required,max=50"`
	Name        string          `json:"name" binding:"required,max=100"`
	Description string          `json:"description"`
	States      json.RawMessage `json:"states"`
	Transitions json.RawMessage `json:"transitions"`
	IsActive    *bool           `json:"is_active"`
}

// UpdateTicketTypeRequest 更新工单类型（patch：nil 保持；code 不可改）
type UpdateTicketTypeRequest struct {
	Name        *string         `json:"name" binding:"omitempty,max=100"`
	Description *string         `json:"description"`
	States      json.RawMessage `json:"states"`
	Transitions json.RawMessage `json:"transitions"`
	IsActive    *bool           `json:"is_active"`
}

// TicketTypeFieldInput 字段定义输入（ReplaceTypeFields 全量替换）
type TicketTypeFieldInput struct {
	FieldKey      string          `json:"field_key" binding:"required,max=50"`
	FieldLabel    string          `json:"field_label" binding:"required,max=100"`
	FieldType     string          `json:"field_type" binding:"required,oneof=input textarea number date select multi_select tips"`
	FieldOptions  json.RawMessage `json:"field_options"`
	Required      bool            `json:"required"`
	ValidateRegex string          `json:"validate_regex" binding:"omitempty,max=200"`
	SortOrder     int             `json:"sort_order"`
}

// ReplaceTypeFieldsRequest 全量替换类型字段集
type ReplaceTypeFieldsRequest struct {
	Fields []TicketTypeFieldInput `json:"fields"`
}

// CreateTicketTemplateRequest 新建模模板（org 决定可见范围，org_path 由服务端解析）
type CreateTicketTemplateRequest struct {
	Code              string          `json:"code" binding:"required,max=50"`
	Name              string          `json:"name" binding:"required,max=200"`
	TypeCode          string          `json:"type_code" binding:"required,max=50"`
	DefaultPriority   int             `json:"default_priority"`
	DefaultFields     json.RawMessage `json:"default_fields"`
	DefaultSLAMinutes *int            `json:"default_sla_minutes"`
	OrgID             int64           `json:"org_id,string" binding:"required"`
}

// UpdateTicketTemplateRequest 更新模板（patch：nil 保持；code/type_code/org 不可改）
type UpdateTicketTemplateRequest struct {
	Name              *string         `json:"name" binding:"omitempty,max=200"`
	DefaultPriority   *int            `json:"default_priority"`
	DefaultFields     json.RawMessage `json:"default_fields"`
	DefaultSLAMinutes *int            `json:"default_sla_minutes"`
}
