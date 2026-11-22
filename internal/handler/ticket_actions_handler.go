package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// AvailableActions returns collection fields for the ticket's pinned rules.
//
//	@Summary	取得工单可执行动作及采集字段
//	@Tags		ticket
//	@Produce	json
//	@Param		id	path	string	true	"工单 ID"
//	@Success	200	{object}	response.Response{data=model.TicketActionList}
//	@Router		/api/v1/tickets/{id}/actions [get]
func (h *TicketHandler) AvailableActions(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的工单 ID")
		return
	}
	var result *model.TicketActionList
	result, err = h.ticketService.ListAvailableActions(c.Request.Context(), id, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, result)
}
