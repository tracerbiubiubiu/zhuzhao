package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

type RiskWorkflowCommands interface {
	Current(context.Context, int64, int64) (model.RiskWorkflowContext, error)
	Receipt(context.Context, int64, string) (riskremediation.Result, error)
	Finish(context.Context, riskremediation.FinishCommand, ticket.RiskWorkflowTicketInput) (riskremediation.Result, error)
	Review(context.Context, riskremediation.ReviewCommand, ticket.RiskWorkflowTicketInput) (riskremediation.Result, error)
	RequestAcceptance(context.Context, riskremediation.AcceptanceRequest, ticket.RiskWorkflowTicketInput) (riskremediation.Result, error)
	DecideAcceptance(context.Context, riskremediation.AcceptanceDecision, ticket.RiskWorkflowTicketInput) (riskremediation.Result, error)
	AssignHandler(context.Context, riskremediation.Command, int64, ticket.RiskWorkflowTicketInput) (riskremediation.Result, error)
	AcceptAssignment(context.Context, riskremediation.Command, ticket.RiskWorkflowTicketInput) (riskremediation.Result, error)
}

type RiskWorkflowHandler struct{ commands RiskWorkflowCommands }

func NewRiskWorkflowHandler(commands RiskWorkflowCommands) *RiskWorkflowHandler {
	return &RiskWorkflowHandler{commands: commands}
}

type riskWorkflowVersionedRequest struct {
	OperationID       string `json:"operation_id" binding:"required"`
	TicketID          int64  `json:"ticket_id,string" binding:"required"`
	ExpectedRevision  int64  `json:"expected_ticket_revision,string" binding:"required"`
	DefinitionVersion int64  `json:"definition_version,string" binding:"required"`
}

type riskWorkflowRequest struct {
	riskWorkflowVersionedRequest
	CustomDataPatch riskWorkflowPatch `json:"custom_data_patch,omitempty"`
	Comment         string            `json:"comment,omitempty"`
	patchJSON       json.RawMessage
}

type riskWorkflowPatch map[string]any

func (p *riskWorkflowPatch) UnmarshalJSON(raw []byte) error {
	if len(raw) > ticket.CustomDataMaxBytes {
		return errors.New("custom_data_patch exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return err
	}
	if object == nil {
		return errors.New("custom_data_patch must be a JSON object")
	}
	*p = object
	return nil
}

type riskWorkflowItemRequest struct {
	LinkID                  int64                         `json:"link_id,string" binding:"required"`
	ExpectedLinkRevision    int64                         `json:"expected_link_revision,string" binding:"required"`
	ExpectedRiskRevision    int64                         `json:"expected_risk_revision,string" binding:"required"`
	ExpectedProposalVersion int64                         `json:"expected_proposal_version,string,omitempty"`
	Resolution              string                        `json:"resolution,omitempty"`
	Measures                string                        `json:"measures,omitempty"`
	Decision                string                        `json:"decision,omitempty"`
	Note                    string                        `json:"note,omitempty"`
	Evidence                []riskremediation.EvidenceRef `json:"evidence"`
}

type riskWorkflowSubmitRequest struct {
	riskWorkflowRequest
	Items []riskWorkflowItemRequest `json:"items" binding:"required"`
}

func riskWorkflowInput(c *gin.Context) (riskWorkflowSubmitRequest, bool) {
	var value riskWorkflowSubmitRequest
	ok := decodeRiskWorkflowInput(c, &value, &value.riskWorkflowRequest)
	return value, ok
}

func decodeRiskWorkflowInput(c *gin.Context, value any, common *riskWorkflowRequest) bool {
	if !decodeRiskWorkflowBody(c, value, &common.riskWorkflowVersionedRequest) {
		return false
	}
	if common.CustomDataPatch != nil {
		encoded, err := json.Marshal(common.CustomDataPatch)
		if err != nil {
			response.BadRequest(c, "整改请求参数错误")
			return false
		}
		common.patchJSON = encoded
	}
	return true
}

func decodeRiskWorkflowBody(c *gin.Context, value any, common *riskWorkflowVersionedRequest) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		response.BadRequest(c, "整改请求格式错误")
		return false
	}
	var extra json.RawMessage
	id, idErr := uuid.Parse(common.OperationID)
	if decoder.Decode(&extra) != io.EOF || idErr != nil || id.String() != common.OperationID ||
		common.TicketID <= 0 || common.ExpectedRevision <= 0 || common.DefinitionVersion <= 0 {
		response.BadRequest(c, "整改请求参数错误")
		return false
	}
	return true
}

func (v riskWorkflowVersionedRequest) command(actor int64) riskremediation.Command {
	return riskremediation.Command{
		ActorID: actor, OperationID: v.OperationID, TicketID: v.TicketID, ExpectedTicketRevision: v.ExpectedRevision,
	}
}

func (v riskWorkflowRequest) inputs(actor int64) (riskremediation.Command, ticket.RiskWorkflowTicketInput) {
	return v.command(actor), ticket.RiskWorkflowTicketInput{DefinitionVersion: v.DefinitionVersion, CustomDataPatch: v.patchJSON, Comment: v.Comment}
}

type riskWorkflowOutcome struct {
	OperationID        string                   `json:"operation_id"`
	TicketID           int64                    `json:"ticket_id,string"`
	TicketRevision     int64                    `json:"ticket_revision,string"`
	DefinitionVersion  int64                    `json:"definition_version,string,omitempty"`
	AssignedToUserID   int64                    `json:"assigned_to_user_id,string,omitempty"`
	SuccessorUserID    int64                    `json:"successor_user_id,string,omitempty"`
	Generation         int64                    `json:"assignment_generation,string,omitempty"`
	Status             string                   `json:"status,omitempty"`
	Items              []riskWorkflowItemResult `json:"items"`
	AcceptanceID       int64                    `json:"acceptance_id,string,omitempty"`
	AcceptanceRevision int64                    `json:"acceptance_revision,string,omitempty"`
	AcceptanceStatus   string                   `json:"acceptance_status,omitempty"`
	TerminalAllowed    bool                     `json:"terminal_allowed"`
}

type riskWorkflowItemResult struct {
	LinkID            int64  `json:"link_id,string"`
	RiskID            int64  `json:"risk_id,string"`
	LinkRevision      int64  `json:"link_revision,string"`
	RiskRevision      int64  `json:"risk_revision,string"`
	ProposalVersion   int64  `json:"proposal_version,string"`
	ResolutionVersion int64  `json:"resolution_version,string"`
	Result            string `json:"result"`
}

func riskWorkflowReply(c *gin.Context, operationID string, result riskremediation.Result, err error) {
	if err != nil {
		writeServiceError(c, err)
		return
	}
	items := make([]riskWorkflowItemResult, 0, len(result.Items))
	for _, row := range result.Items {
		items = append(items, riskWorkflowItemResult{
			LinkID: row.LinkID, RiskID: row.RiskID, LinkRevision: row.LinkRevision, RiskRevision: row.RiskRevision,
			ProposalVersion: row.ProposalVersion, ResolutionVersion: row.ResolutionVersion, Result: row.Result,
		})
	}
	response.OK(c, riskWorkflowOutcome{
		OperationID: operationID,
		TicketID:    result.TicketID, TicketRevision: result.TicketRevision, Items: items, TerminalAllowed: result.TerminalAllowed,
		DefinitionVersion: result.DefinitionVersion, AssignedToUserID: result.AssignedToUserID, SuccessorUserID: result.SuccessorUserID, Generation: result.Generation, Status: result.Status,
		AcceptanceID: result.AcceptanceID, AcceptanceRevision: result.AcceptanceRevision, AcceptanceStatus: result.AcceptanceStatus,
	})
}

// Current
//
//	@Summary	读取逐资产办理需要的当前修订
//	@Tags		risk
//	@Produce	json
//	@Param		id	path	string	true	"工单 ID"
//	@Success	200	{object}	response.Response{data=model.RiskWorkflowContext}
//	@Router		/api/v1/risk-items/remediation-context/{id} [get]
func (h *RiskWorkflowHandler) Current(c *gin.Context) {
	id, err := parseDecimalID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "工单编号不合法")
		return
	}
	value, err := h.commands.Current(c.Request.Context(), riskActor(c), id)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, value)
}

// Receipt
//
//	@Summary	按原号只读查询逐资产整改回执
//	@Tags		risk
//	@Produce	json
//	@Param		id	path	string	true	"原操作号"
//	@Success	200	{object}	response.Response{data=riskWorkflowOutcome}
//	@Router		/api/v1/risk-items/remediation-operations/{id} [get]
func (h *RiskWorkflowHandler) Receipt(c *gin.Context) {
	value, err := h.commands.Receipt(c.Request.Context(), riskActor(c), c.Param("id"))
	riskWorkflowReply(c, c.Param("id"), value, err)
}

// Finish
//
//	@Summary	办理人提交逐资产整改证据
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		req	body	riskWorkflowSubmitRequest	true	"列明原操作号和逐资产修订"
//	@Success	200	{object}	response.Response{data=riskWorkflowOutcome}
//	@Router		/api/v1/risk-items/remediation-finish [post]
func (h *RiskWorkflowHandler) Finish(c *gin.Context) {
	req, ok := riskWorkflowInput(c)
	if !ok {
		return
	}
	command, ticketInput := req.inputs(riskActor(c))
	items := make([]riskremediation.Item, 0, len(req.Items))
	for _, row := range req.Items {
		if row.Decision != "" || row.Note != "" || row.ExpectedProposalVersion != 0 {
			response.BadRequest(c, "办理提交含复测字段")
			return
		}
		items = append(items, riskremediation.Item{
			LinkID: row.LinkID, ExpectedLinkRevision: row.ExpectedLinkRevision, ExpectedRiskRevision: row.ExpectedRiskRevision,
			Resolution: row.Resolution, Measures: row.Measures, Evidence: row.Evidence,
		})
	}
	result, err := h.commands.Finish(c.Request.Context(), riskremediation.FinishCommand{Command: command, Items: items}, ticketInput)
	riskWorkflowReply(c, req.OperationID, result, err)
}

// Review
//
//	@Summary	现势责任人逐项通过或打回
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		req	body	riskWorkflowSubmitRequest	true	"列明原操作号和逐资产提案版本"
//	@Success	200	{object}	response.Response{data=riskWorkflowOutcome}
//	@Router		/api/v1/risk-items/remediation-review [post]
func (h *RiskWorkflowHandler) Review(c *gin.Context) {
	req, ok := riskWorkflowInput(c)
	if !ok {
		return
	}
	command, ticketInput := req.inputs(riskActor(c))
	items := make([]riskremediation.ReviewItem, 0, len(req.Items))
	for _, row := range req.Items {
		if row.Resolution != "" || row.Measures != "" {
			response.BadRequest(c, "复测提交含办理字段")
			return
		}
		items = append(items, riskremediation.ReviewItem{
			LinkID: row.LinkID, ExpectedLinkRevision: row.ExpectedLinkRevision, ExpectedRiskRevision: row.ExpectedRiskRevision,
			ExpectedProposalVersion: row.ExpectedProposalVersion, Decision: row.Decision, Note: row.Note, Evidence: row.Evidence,
		})
	}
	result, err := h.commands.Review(c.Request.Context(), riskremediation.ReviewCommand{Command: command, Items: items}, ticketInput)
	riskWorkflowReply(c, req.OperationID, result, err)
}
