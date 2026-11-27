// MessageTemplateHandler 消息模板管理 API（S6 B6-3——CRUD 四端点）。
package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/msgrender"
	"net/http"

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
	if err := h.templateService.Create(trustedTaskContext(c), &req); err != nil {
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
	if err := h.templateService.Update(trustedTaskContext(c), &req); err != nil {
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

// BuiltinTemplateVars 直接投影 msgrender 的单一字典，不另维护变量列表。
func BuiltinTemplateVars() []map[string]string {
	out := make([]map[string]string, 0, len(msgrender.BuiltinVariables()))
	for _, v := range msgrender.BuiltinVariables() {
		out = append(out, map[string]string{"path": v.Path, "desc": v.Description})
	}
	return out
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

type MessageTemplatePreviewRequest struct {
	Code    string `json:"code"`
	Channel string `json:"channel" binding:"required"`
	Subject string `json:"subject" binding:"required"`
	Body    string `json:"body" binding:"required"`
}

type MessageTemplatePreview struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// Preview evaluates the draft against synthetic values.
// @Summary  预览消息模板
// @Tags     message
// @Accept   json
// @Param    request body MessageTemplatePreviewRequest true "模板草稿"
// @Success  200 {object} response.Response{data=MessageTemplatePreview}
// @Router   /api/v1/message-templates/preview [post]
func (h *MessageTemplateHandler) Preview(c *gin.Context) {
	var req MessageTemplatePreviewRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "模板预览参数错误")
		return
	}
	subject, body, err := service.PreviewMessageTemplate(req.Code, req.Channel, req.Subject, req.Body)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, MessageTemplatePreview{Subject: subject, Body: body})
}
