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
