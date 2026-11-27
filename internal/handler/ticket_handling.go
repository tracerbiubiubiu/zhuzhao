package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// Transfer changes the current assignee without transitioning the ticket.
// @Summary 转办工单
// @Tags ticket
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "幂等键"
// @Param request body model.TicketTransferRequest true "转办请求"
// @Success 200 {object} response.Response{data=model.TicketHandlingResult}
// @Router /api/v1/tickets/transfer [post]
func (h *TicketHandler) Transfer(c *gin.Context) {
	var req model.TicketTransferRequest
	if !bindStrictTicketJSON(c, &req) {
		return
	}
	out, err := h.ticketService.Transfer(c.Request.Context(), req, c.GetHeader("Idempotency-Key"), c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, out)
}

// Withdraw cancels an untouched open ticket on its published creator edge.
// @Summary 撤回工单
// @Tags ticket
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "幂等键"
// @Param request body model.TicketWithdrawRequest true "撤回请求"
// @Success 200 {object} response.Response{data=model.TicketHandlingResult}
// @Router /api/v1/tickets/withdraw [post]
func (h *TicketHandler) Withdraw(c *gin.Context) {
	var req model.TicketWithdrawRequest
	if !bindStrictTicketJSON(c, &req) {
		return
	}
	out, err := h.ticketService.Withdraw(c.Request.Context(), req, c.GetHeader("Idempotency-Key"), c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, out)
}
