// NotificationPrefHandler 用户通知偏好自服务端点（B6-3——handler→service 分层）。
package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

type NotificationPrefHandler struct {
	prefService *service.NotificationPrefService
}

func NewNotificationPrefHandler(ps *service.NotificationPrefService) *NotificationPrefHandler {
	return &NotificationPrefHandler{prefService: ps}
}

func (h *NotificationPrefHandler) ListOptouts(c *gin.Context) {
	uid := c.GetInt64("user_id")
	out, err := h.prefService.ListOptouts(c.Request.Context(), uid)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, out)
}

func (h *NotificationPrefHandler) SetOptout(c *gin.Context) {
	uid := c.GetInt64("user_id")
	var req struct {
		BizType     string `json:"biz_type" binding:"required"`
		EventAction string `json:"event_action" binding:"required"`
		OptOut      bool   `json:"opt_out"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.prefService.SetOptout(c.Request.Context(), uid, req.BizType, req.EventAction, req.OptOut); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}
