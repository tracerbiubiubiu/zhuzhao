package model

import (
	"encoding/json"
	"time"
)

// CreateTicketRequest 创建工单
type CreateTicketRequest struct {
	TypeCode     string          `json:"type_code" binding:"required,max=50"`
	Title        string          `json:"title" binding:"required,max=200"`
	Description  string          `json:"description"`
	Priority     int             `json:"priority"`
	AssignedTo   *int64          `json:"assigned_to,string,omitempty"`
	OrgID        int64           `json:"org_id,string" binding:"required"`
	TemplateCode string          `json:"template_code,omitempty"` // 可选：命中模板则预填
	CustomData   json.RawMessage `json:"custom_data,omitempty"`
	// B5-3 建单前绑定（A.5 建单前链）：attachment_create/richtext_create 悬挂
	// confirmed 对象键列表，Create 同事务校验+落绑 ticket_attachments。
	// 语义=增量落绑（只增不删）：解除绑定走 POST /tickets/attachments/delete
	//（内嵌图 ticket_richtext 不可经该端点删——随工单生命周期，A.7）
	Attachments []string `json:"attachments,omitempty"`
}

// UpdateTicketRequest 更新工单（POST /tickets/update，id 放 body）
type UpdateTicketRequest struct {
	ID          int64   `json:"id,string" binding:"required"`
	Title       *string `json:"title,omitempty" binding:"omitempty,max=200"`
	Description *string `json:"description,omitempty"`
	Priority    *int    `json:"priority,omitempty"`
	// B5-4 编辑增量绑定（05 号 A.3 a 案）：悬挂 confirmed 对象（attachment_create/
	// richtext_create）键列表，随保存同事务落绑——与 CreateTicketRequest.Attachments
	// 同语义（本人/confirmed/悬挂态/未绑定，超上限 400）；删附件走 attachments/delete 端点
	Attachments []string `json:"attachments,omitempty"`
	// B5-4 custom_data 增量 patch（05 号 §6/A.3 拍板项 2026-10-20）：仅传入键覆盖基线、
	// 值为 null 的键删除（清空字段）；合并后整体按类型字段 schema G2 重校验；仅非终态可改。
	// 字段级 diff（D20 changes[]）随 S6 B6-1 事件面统一落地——请求体已入通用审计行。
	CustomData json.RawMessage `json:"custom_data,omitempty"`
}

// RenderRichtextRequest richtext 服务端渲染（B5-4——05 号 §6 渲染/消毒 SSOT 出站：
// 前端不自行渲染用户输入；登录态自服务——无票据访问面，纯消毒渲染）。
type RenderRichtextRequest struct {
	Content string `json:"content" binding:"required" example:"# 标题\n\n- 列表"`
}

// RenderRichtextResponse 渲染产物（白名单 HTML）。
type RenderRichtextResponse struct {
	HTML string `json:"html"`
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
	// AssigneeID 处理人过滤（W4 P1-c）：handler 把 assignee=me 解析为当前用户 ID；
	// 空 = 不过滤。点亮工作台待办/已办卡（01 §8-W2）
	AssigneeID *int64
	// Keyword 标题关键字：ILIKE 子串（D2-21 转义）；纯数字时 OR 工单 ID 精确匹配
	//（运维「粘工单号」高频动作，纯标题子串会漏）。空 = 不过滤
	Keyword string
	// CreatedFromAt/CreatedToAt 创建时间区间：handler 把 YYYY-MM-DD 解析为 UTC 闭开
	// 区间边界（FromAt=当日 00:00 UTC，ToAt=次日 00:00 UTC，与 timestamptz 比较）；
	// nil = 不过滤
	CreatedFromAt *time.Time
	CreatedToAt   *time.Time
	// CreatedBy 创建人过滤：handler 把 created_by=me 解析为当前用户 ID（同 assignee=me
	// 契约；「我发起的」维度，与「我处理的」凑待办/已办两视角）
	CreatedBy *int64
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
	// W1（BK-18 整改）：code 从 path param 迁入 body（POST 风格，standards §3-2）
	Code        string          `json:"code" binding:"required"`
	Name        *string         `json:"name" binding:"omitempty,max=100"`
	Description *string         `json:"description"`
	States      json.RawMessage `json:"states"`
	Transitions json.RawMessage `json:"transitions"`
	IsActive    *bool           `json:"is_active"`
	// Version 可选乐观锁：nil = 不做 CAS（保持旧 patch 行为）；非 nil = 命中
	// ticket_types.version 才写，否则 409。须 > 0，否则 400。
	Version *int `json:"version,omitempty"`
}

// TicketTypeFieldInput 字段定义输入（ReplaceTypeFields 全量替换）
type TicketTypeFieldInput struct {
	FieldKey      string          `json:"field_key" binding:"required,max=50"`
	FieldLabel    string          `json:"field_label" binding:"required,max=100"`
	FieldType     string          `json:"field_type" binding:"required,oneof=input textarea number date select multi_select tips richtext"`
	FieldOptions  json.RawMessage `json:"field_options"`
	Required      bool            `json:"required"`
	ValidateRegex string          `json:"validate_regex" binding:"omitempty,max=200"`
	SortOrder     int             `json:"sort_order"`
}

// ReplaceTypeFieldsRequest 全量替换类型字段集
type ReplaceTypeFieldsRequest struct {
	// W1（BK-18 整改）：code 从 path param 迁入 body
	Code   string                 `json:"code" binding:"required"`
	Fields []TicketTypeFieldInput `json:"fields"`
	// Version 可选乐观锁：以父类型 ticket_types.version 做 CAS；nil = 不做 CAS。
	// 须 > 0，否则 400。
	Version *int `json:"version,omitempty"`
}

// DeleteByCodeRequest W1（BK-18 整改）：delete 端点 POST 化后的 body（code 原在 path）
type DeleteByCodeRequest struct {
	Code string `json:"code" binding:"required"`
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
	// W1（BK-18 整改）：code 从 path param 迁入 body
	Code              string          `json:"code" binding:"required"`
	Name              *string         `json:"name" binding:"omitempty,max=200"`
	DefaultPriority   *int            `json:"default_priority"`
	DefaultFields     json.RawMessage `json:"default_fields"`
	DefaultSLAMinutes *int            `json:"default_sla_minutes"`
	// Version 可选乐观锁：nil = 不做 CAS（保持旧 patch 行为）；非 nil = 命中
	// ticket_templates.version 才写，否则 409。须 > 0，否则 400。
	Version *int `json:"version,omitempty"`
}
