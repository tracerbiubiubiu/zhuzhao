// BadgeHandler 角标聚合端点（S10 B10-3——10 号附录 A.4 GET /badges）。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	ticketsvc "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// BadgeHandler 角标聚合处理器（handler 只编排——sections 计算在 BadgeService 注册制）。
type BadgeHandler struct {
	badgeService *ticketsvc.BadgeService
}

// NewBadgeHandler 创建角标聚合处理器。
func NewBadgeHandler(badgeService *ticketsvc.BadgeService) *BadgeHandler {
	return &BadgeHandler{badgeService: badgeService}
}

// List GET /api/v1/badges
//
//	@Summary	待办角标聚合（sections 开放 map——前端 60s 轮询；未知 section 前端忽略）
//	@Tags		badge
//	@Produce	json
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/badges [get]
func (h *BadgeHandler) List(c *gin.Context) {
	sections := h.badgeService.Sections(c.Request.Context(), c.GetInt64("userID"))
	response.OK(c, gin.H{"sections": sections})
}
