package handler

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	ticketsvc "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// PlanTicketDefinition validates the current draft and returns a publication plan.
//
//	@Summary	预览工单规则发布
//	@Tags		ticket
//	@Accept		json
//	@Produce	json
//	@Param		req	body	model.TicketDefinitionPlanRequest	true	"草稿和当前发布版本"
//	@Success	200	{object}	response.Response{data=model.TicketDefinitionPlan}
//	@Failure	409	{object}	response.Response
//	@Header		409	{string}	X-Ticket-Definition-Reason	"计划冲突归因"
//	@Router		/api/v1/ticket-types/definitions/plan [post]
func (h *TicketHandler) PlanTicketDefinition(c *gin.Context) {
	var req model.TicketDefinitionPlanRequest
	if !bindDefinitionPublication(c, &req) {
		return
	}
	plan, err := h.ticketService.PlanTicketDefinition(c.Request.Context(), req, c.GetInt64("userID"))
	if err != nil {
		writeDefinitionPublicationError(c, err)
		return
	}
	response.OK(c, plan)
}

// PublishTicketDefinition applies a plan or replays a committed operation receipt.
//
//	@Summary	显式发布工单规则
//	@Tags		ticket
//	@Accept		json
//	@Produce	json
//	@Param		Idempotency-Key	header	string	true	"首次发布及重试使用的同一操作号"
//	@Param		req	body	model.TicketDefinitionPublishRequest	true	"发布计划与告警确认"
//	@Success	200	{object}	response.Response{data=model.TicketDefinitionPublicationReceipt}
//	@Failure	409	{object}	response.Response
//	@Header		409	{string}	X-Ticket-Definition-Reason	"发布冲突归因"
//	@Router		/api/v1/ticket-types/definitions/publish [post]
func (h *TicketHandler) PublishTicketDefinition(c *gin.Context) {
	var req model.TicketDefinitionPublishRequest
	if !bindDefinitionPublication(c, &req) {
		return
	}
	keys := c.Request.Header.Values("Idempotency-Key")
	if len(keys) != 1 {
		response.BadRequest(c, "须提供一个 Idempotency-Key")
		return
	}
	receipt, err := h.ticketService.ExecuteTicketDefinitionPublication(c.Request.Context(), req, c.GetInt64("userID"), keys[0])
	if err != nil {
		writeDefinitionPublicationError(c, err)
		return
	}
	response.OK(c, receipt)
}

// GetPublishedTicketForm returns the immutable current form schema for ticket creation.
//
//	@Summary	读取建单用已发布表单
//	@Tags		ticket
//	@Produce	json
//	@Param		code	path	string	true	"工单类型编码"
//	@Success	200	{object}	response.Response{data=model.PublishedTicketForm}
//	@Failure	404	{object}	response.Response
//	@Failure	503	{object}	response.Response
//	@Header		503	{string}	X-Ticket-Definition-Reason	"definition_unpublished 表示类型尚无发布版本"
//	@Router		/api/v1/ticket-types/{code}/published [get]
func (h *TicketHandler) GetPublishedTicketForm(c *gin.Context) {
	form, err := h.ticketService.GetPublishedTicketForm(c.Request.Context(), c.Param("code"), c.GetInt64("userID"))
	if err != nil {
		writeDefinitionPublicationError(c, err)
		return
	}
	response.OK(c, form)
}

func (h *TicketHandler) ListTicketConfigurationTypes(c *gin.Context) {
	types, err := h.ticketService.ListTicketConfigurationTypes(c.Request.Context(), c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"types": types})
}

func (h *TicketHandler) GetTicketDefinitionDraftFields(c *gin.Context) {
	fields, err := h.ticketService.GetTicketDefinitionDraftFields(c.Request.Context(), c.Param("code"), c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"fields": fields})
}

func bindDefinitionPublication(c *gin.Context, req any) bool {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(req); err != nil {
		response.BadRequest(c, "参数错误")
		return false
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		response.BadRequest(c, "参数错误")
		return false
	}
	return true
}

func writeDefinitionPublicationError(c *gin.Context, err error) {
	var failure *ticketsvc.DefinitionPublicationError
	if !errors.As(err, &failure) {
		writeServiceError(c, err)
		return
	}
	c.Header("X-Ticket-Definition-Reason", failure.Reason)
	writeServiceError(c, failure.Cause)
}
