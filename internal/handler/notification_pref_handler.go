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

// ListOptouts GET /api/v1/me/notification-optouts
//
//	@Summary	当前用户通知退订列表（B6-4 偏好自服务）
//	@Tags		notification
//	@Produce	json
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/me/notification-optouts [get]
func (h *NotificationPrefHandler) ListOptouts(c *gin.Context) {
	uid := c.GetInt64("user_id")
	out, err := h.prefService.ListOptouts(c.Request.Context(), uid)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, out)
}

// SetOptout POST /api/v1/me/notification-optouts
//
//	@Summary	设置当前用户通知退订（按 biz_type+event_action；"*" 通配同域全退）
//	@Tags		notification
//	@Accept		json
//	@Produce	json
//	@Success	200	{object}	response.Response
//	@Failure	400	{object}	response.Response
//	@Router		/api/v1/me/notification-optouts [post]
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
