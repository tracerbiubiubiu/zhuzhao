// MessageAdminHandler 消息管理 API（S6 B6-6 收口——handler→service→repo 分层）。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

type MessageAdminHandler struct {
	adminService *service.MessageAdminService
}

func NewMessageAdminHandler(as *service.MessageAdminService) *MessageAdminHandler {
	return &MessageAdminHandler{adminService: as}
}

// func (h *MessageAdminHandler) ListLogs GET /api/v1/message-logs
//
//	@Summary		发送记录列表
//	@Tags		message
//	@Produce	json
//	@Router		/api/v1/message-logs [get]
//
// ListLogs GET /api/v1/message-logs
func (h *MessageAdminHandler) ListLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	logs, total, err := h.adminService.ListLogs(c.Request.Context(), page, pageSize,
		c.Query("biz_type"), c.Query("template_code"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	// 拍板 B 过渡脱敏（外审 2.3）：列表面 body 可能含工单字段值（越 L2 面），
	// 一期先截断出站——完整方案=通知中心独立权限码+菜单重绑（S6 二期或 S10 前）
	for _, l := range logs {
		if l != nil && len(l.Body) > 200 {
			l.Body = l.Body[:200] + "…（截断——详情请查 message_log 表）"
		}
	}
	response.OKPage(c, logs, total, page, pageSize)
}

// func (h *MessageAdminHandler) ListRules GET /api/v1/message-rules
//
//	@Summary		订阅规则列表
//	@Tags		message
//	@Produce	json
//	@Router		/api/v1/message-rules [get]
//
// ListRules GET /api/v1/message-rules
func (h *MessageAdminHandler) ListRules(c *gin.Context) {
	rules, err := h.adminService.ListRules(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, rules)
}

// func (h *MessageAdminHandler) UpsertRule POST /api/v1/message-rules
//
//	@Summary		创建/更新订阅规则
//	@Tags		message
//	@Produce	json
//	@Router		/api/v1/message-rules [post]
//
// UpsertRule POST /api/v1/message-rules
func (h *MessageAdminHandler) UpsertRule(c *gin.Context) {
	var req model.MessageRule
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if req.EventAction == "" || req.TemplateCode == "" {
		response.BadRequest(c, "event_action 与 template_code 必填")
		return
	}
	if err := h.adminService.UpsertRule(c.Request.Context(), &req); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// func (h *MessageAdminHandler) DeleteRule POST /api/v1/message-rules/delete
//
//	@Summary		删除订阅规则
//	@Tags		message
//	@Produce	json
//	@Router		/api/v1/message-rules/delete [post]
//
// DeleteRule POST /api/v1/message-rules/delete
func (h *MessageAdminHandler) DeleteRule(c *gin.Context) {
	var req struct {
		ID int64 `json:"id,string" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.adminService.DeleteRule(c.Request.Context(), req.ID); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}
