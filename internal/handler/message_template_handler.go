// MessageTemplateHandler 消息模板管理 API（S6 B6-3——CRUD 四端点）。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"strconv"
)

type MessageTemplateHandler struct {
	templateService *service.MessageTemplateService
}

func NewMessageTemplateHandler(ts *service.MessageTemplateService) *MessageTemplateHandler {
	return &MessageTemplateHandler{templateService: ts}
}

// Func (h *messagetemplatehandler) list GET /api/v1/message-templates
//
//	@Summary		消息模板列表
//	@Tags		message
//	@Produce	json
//	@Router		/api/v1/message-templates [get]
//
// List GET /api/v1/message-templates
func (h *MessageTemplateHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	templates, total, err := h.templateService.List(c.Request.Context(), page, pageSize, c.Query("code_like"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OKPage(c, templates, total, page, pageSize)
}

// Func (h *messagetemplatehandler) create POST /api/v1/message-templates
//
//	@Summary		创建消息模板
//	@Tags		message
//	@Produce	json
//	@Router		/api/v1/message-templates [post]
//
// Create POST /api/v1/message-templates
func (h *MessageTemplateHandler) Create(c *gin.Context) {
	var req model.MessageTemplate
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if req.Code == "" || req.Name == "" || req.Subject == "" || req.Body == "" {
		response.BadRequest(c, "code/name/subject/body 均必填")
		return
	}
	if err := h.templateService.Create(c.Request.Context(), &req); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, req)
}

// Func (h *messagetemplatehandler) update POST /api/v1/message-templates/update
//
//	@Summary		更新消息模板
//	@Tags		message
//	@Produce	json
//	@Router		/api/v1/message-templates/update [post]
//
// Update POST /api/v1/message-templates/update
func (h *MessageTemplateHandler) Update(c *gin.Context) {
	var req model.MessageTemplate
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if req.Code == "" {
		response.BadRequest(c, "code 必填")
		return
	}
	if err := h.templateService.Update(c.Request.Context(), &req); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, req)
}

// Func (h *messagetemplatehandler) delete POST /api/v1/message-templates/delete
//
//	@Summary		删除消息模板
//	@Tags		message
//	@Produce	json
//	@Router		/api/v1/message-templates/delete [post]
//
// Delete POST /api/v1/message-templates/delete
func (h *MessageTemplateHandler) Delete(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.templateService.Delete(c.Request.Context(), req.Code); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// BuiltinTemplateVars 内置变量字典（G-2 同源修复——与 validatePlaceholders 的
// builtinVarSet 一一对应；新增事件载荷键须同步两处）。
func BuiltinTemplateVars() []map[string]string {
	return []map[string]string{
		{"path": "ticket.title", "desc": "工单标题"},
		{"path": "ticket.type_code", "desc": "工单类型编码"},
		{"path": "ticket.priority", "desc": "优先级（1-4）"},
		{"path": "ticket.status", "desc": "当前状态"},
		{"path": "ticket.human_key", "desc": "人读编号（TK-yyyymmdd-NNNN）"},
		{"path": "ticket.link", "desc": "工单链接"},
		{"path": "ticket.from_status", "desc": "转换前状态（transitioned）"},
		{"path": "ticket.to_status", "desc": "转换后状态（transitioned）"},
		{"path": "ticket.from_assignee", "desc": "分派前处理人 ID"},
		{"path": "ticket.to_assignee", "desc": "分派后处理人 ID"},
		{"path": "ticket.cc_users", "desc": "抄送人 ID 列表（逗号分隔）"},
		{"path": "ticket.reminder_count", "desc": "催办次数（timeout）"},
		{"path": "ticket.overdue_minutes", "desc": "已超时分钟数（timeout）"},
		{"path": "ticket.actor", "desc": "操作人 ID"},
		{"path": "event.job", "desc": "定时任务名（ops.cron_failed）"},
		{"path": "event.error", "desc": "错误信息（ops.cron_failed）"},
	}
}

// Variables GET /api/v1/ticket-types/:code/fields/variables
//
//	@Summary		模板可用变量集（变量字典——06 号 A.6）
//	@Tags			message
//	@Produce		json
//	@Router			/api/v1/ticket-types/{code}/fields/variables [get]
func (h *MessageTemplateHandler) Variables(c *gin.Context) {
	// 内置变量（通知链每次必供——四象限 fail 档锚点）
	vars := BuiltinTemplateVars()
	response.OK(c, vars)
}
