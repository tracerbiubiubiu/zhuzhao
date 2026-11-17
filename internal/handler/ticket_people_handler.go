package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	ticketservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// TicketPeopleHandler exposes context-bound business candidate search.
type TicketPeopleHandler struct{ service *ticketservice.PeopleService }

// NewTicketPeopleHandler creates the restricted search handler.
func NewTicketPeopleHandler(service *ticketservice.PeopleService) *TicketPeopleHandler {
	return &TicketPeopleHandler{service: service}
}

// Search returns at most 20 identities after checking the original write action.
// @Summary Context-bound ticket people search (2–50 characters, maximum 20 results)
// @Tags ticket
// @Produce json
// @Param purpose query string true "assign|cc|create-supervisor|rule-target"
// @Param q query string true "2–50 characters"
// @Param ticket_id query string false "Required for assign/cc"
// @Param org_id query string false "Required for create-supervisor/rule-target"
// @Param type_code query string false "Required for create-supervisor"
// @Security BearerAuth
// @Success 200 {object} response.Response
// @Router /api/v1/tickets/people [get]
func (h *TicketPeopleHandler) Search(c *gin.Context) {
	var q model.TicketPeopleQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.BadRequest(c, "选人上下文参数无效")
		return
	}
	list, err := h.service.Search(c.Request.Context(), q, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"list": list})
}
