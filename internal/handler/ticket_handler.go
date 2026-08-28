package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
	ticketsvc "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// TicketHandler 工单处理器
type TicketHandler struct {
	ticketService *ticketsvc.Service
}

// NewTicketHandler 创建 TicketHandler
func NewTicketHandler(ticketService *ticketsvc.Service) *TicketHandler {
	return &TicketHandler{ticketService: ticketService}
}

// List GET /api/v1/tickets
//
//	@Summary	工单列表
//	@Tags		ticket
//	@Accept		json
//	@Produce	json
//	@Param		page		query	int		false	"页码"
//	@Param		page_size	query	int		false	"每页条数"
//	@Param		type_code	query	string	false	"工单类型"
//	@Param		status		query	string	false	"工单状态"
//	@Success	200			{object}	response.Response
//	@Router		/api/v1/tickets [get]
func (h *TicketHandler) List(c *gin.Context) {
	// BK-7：分页归一前置到 handler（响应回显与 SQL 实际执行一致，page=0/负数不再回显原值）
	q := model.TicketListQuery{
		Page:     queryInt(c, "page", 1),
		PageSize: queryInt(c, "page_size", 20),
		TypeCode: c.Query("type_code"),
		Status:   c.Query("status"),
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 100 {
		q.PageSize = 20
	}
	if p := c.Query("priority"); p != "" {
		v, err := strconv.Atoi(p)
		if err != nil {
			response.BadRequest(c, "priority 须为整数")
			return
		}
		q.Priority = &v
	}
	resp, err := h.ticketService.List(c.Request.Context(), q, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, resp)
}

// Create POST /api/v1/tickets
//
//	@Summary	创建工单
//	@Tags		ticket
//	@Accept		json
//	@Produce	json
//	@Param		request	body		model.CreateTicketRequest	true	"创建工单请求"
//	@Success	200		{object}	response.Response
//	@Router		/api/v1/tickets [post]
func (h *TicketHandler) Create(c *gin.Context) {
	var req model.CreateTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	ticket, err := h.ticketService.Create(c.Request.Context(), &req, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, ticket)
}

// Get GET /api/v1/tickets/:id
//
//	@Summary	工单详情
//	@Tags		ticket
//	@Produce	json
//	@Param		id	path		int	true	"工单 ID"
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/tickets/{id} [get]
func (h *TicketHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的工单 ID")
		return
	}
	ticket, err := h.ticketService.Get(c.Request.Context(), id, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, ticket)
}

// Update POST /api/v1/tickets/update
//
//	@Summary	更新工单
//	@Tags		ticket
//	@Accept		json
//	@Produce	json
//	@Param		request	body		model.UpdateTicketRequest	true	"更新工单请求"
//	@Success	200		{object}	response.Response
//	@Router		/api/v1/tickets/update [post]
func (h *TicketHandler) Update(c *gin.Context) {
	var req model.UpdateTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	ticket, err := h.ticketService.Update(c.Request.Context(), &req, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, ticket)
}

// Close POST /api/v1/tickets/close
//
//	@Summary	关闭工单
//	@Tags		ticket
//	@Accept		json
//	@Produce	json
//	@Param		request	body		model.CloseTicketRequest	true	"关闭工单请求"
//	@Success	200		{object}	response.Response
//	@Router		/api/v1/tickets/close [post]
func (h *TicketHandler) Close(c *gin.Context) {
	var req model.CloseTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.ticketService.Close(c.Request.Context(), &req, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// Assign POST /api/v1/tickets/assign
//
//	@Summary	分派工单
//	@Tags		ticket
//	@Accept		json
//	@Produce	json
//	@Param		request	body		model.AssignTicketRequest	true	"分派工单请求"
//	@Success	200		{object}	response.Response
//	@Router		/api/v1/tickets/assign [post]
func (h *TicketHandler) Assign(c *gin.Context) {
	var req model.AssignTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.ticketService.Assign(c.Request.Context(), &req, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// Delete POST /api/v1/tickets/delete
//
//	@Summary	删除工单
//	@Tags		ticket
//	@Accept		json
//	@Produce	json
//	@Param		request	body		model.DeleteTicketRequest	true	"删除工单请求"
//	@Success	200		{object}	response.Response
//	@Router		/api/v1/tickets/delete [post]
func (h *TicketHandler) Delete(c *gin.Context) {
	var req model.DeleteTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.ticketService.Delete(c.Request.Context(), req.ID, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// ListComments GET /api/v1/tickets/:id/comments
//
//	@Summary	工单回复列表
//	@Tags		ticket
//	@Produce	json
//	@Param		id	path		int	true	"工单 ID"
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/tickets/{id}/comments [get]
func (h *TicketHandler) ListComments(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的工单 ID")
		return
	}
	comments, err := h.ticketService.ListComments(c.Request.Context(), id, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"comments": comments})
}

// CreateComment POST /api/v1/tickets/comments
//
//	@Summary	创建工单回复
//	@Tags		ticket
//	@Accept		json
//	@Produce	json
//	@Param		request	body		model.CreateCommentRequest	true	"创建回复请求"
//	@Success	200		{object}	response.Response
//	@Router		/api/v1/tickets/comments [post]
func (h *TicketHandler) CreateComment(c *gin.Context) {
	var req model.CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	comment, err := h.ticketService.CreateComment(c.Request.Context(), &req, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, comment)
}

// CreateNote POST /api/v1/tickets/notes
//
//	@Summary	创建内部备注
//	@Tags		ticket
//	@Accept		json
//	@Produce	json
//	@Param		request	body		model.CreateNoteRequest	true	"创建备注请求"
//	@Success	200		{object}	response.Response
//	@Router		/api/v1/tickets/notes [post]
func (h *TicketHandler) CreateNote(c *gin.Context) {
	var req model.CreateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	comment, err := h.ticketService.CreateNote(c.Request.Context(), &req, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, comment)
}

// ListRelations GET /api/v1/tickets/:id/relations
//
//	@Summary	工单关联列表
//	@Tags		ticket
//	@Produce	json
//	@Param		id	path		int	true	"工单 ID"
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/tickets/{id}/relations [get]
func (h *TicketHandler) ListRelations(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的工单 ID")
		return
	}
	relations, err := h.ticketService.ListRelations(c.Request.Context(), id, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"relations": relations})
}

// CreateRelation POST /api/v1/tickets/relations
//
//	@Summary	建立工单关联
//	@Tags		ticket
//	@Accept		json
//	@Produce	json
//	@Param		request	body		model.CreateRelationRequest	true	"建立关联请求"
//	@Success	200		{object}	response.Response
//	@Router		/api/v1/tickets/relations [post]
func (h *TicketHandler) CreateRelation(c *gin.Context) {
	var req model.CreateRelationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	rel, err := h.ticketService.CreateRelation(c.Request.Context(), &req, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, rel)
}

// ListTicketTypes GET /api/v1/ticket-types
//
//	@Summary	工单类型列表
//	@Tags		ticket
//	@Produce	json
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/ticket-types [get]
func (h *TicketHandler) ListTicketTypes(c *gin.Context) {
	types, err := h.ticketService.ListTicketTypes(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"types": types})
}

// ListTicketTypeFields GET /api/v1/ticket-types/:code/fields
//
//	@Summary	工单类型字段定义
//	@Tags		ticket
//	@Produce	json
//	@Param		code	path		string	true	"工单类型编码"
//	@Success	200		{object}	response.Response
//	@Router		/api/v1/ticket-types/{code}/fields [get]
func (h *TicketHandler) ListTicketTypeFields(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		response.BadRequest(c, "无效的工单类型编码")
		return
	}
	fields, err := h.ticketService.ListTicketTypeFields(c.Request.Context(), code)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"fields": fields})
}

// ListTicketTemplates GET /api/v1/ticket-templates
//
//	@Summary	工单模板列表
//	@Tags		ticket
//	@Produce	json
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/ticket-templates [get]
func (h *TicketHandler) ListTicketTemplates(c *gin.Context) {
	templates, err := h.ticketService.ListTicketTemplates(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"templates": templates})
}

// GetTicketTemplate GET /api/v1/ticket-templates/:code
//
//	@Summary	工单模板详情
//	@Tags		ticket
//	@Produce	json
//	@Param		code	path		string	true	"模板编码"
//	@Success	200		{object}	response.Response
//	@Router		/api/v1/ticket-templates/{code} [get]
func (h *TicketHandler) GetTicketTemplate(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		response.BadRequest(c, "无效的模板编码")
		return
	}
	tmpl, err := h.ticketService.GetTicketTemplate(c.Request.Context(), code)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, tmpl)
}
