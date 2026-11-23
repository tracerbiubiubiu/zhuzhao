package handler

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

type riskWorkflowAcceptanceReworkCommands interface {
	ReworkAcceptance(context.Context, riskremediation.AcceptanceRework, ticket.RiskWorkflowTicketInput) (riskremediation.Result, error)
}

type riskWorkflowAcceptanceRework struct {
	riskWorkflowRequest
	RiskID                     int64  `json:"risk_id,string" binding:"required"`
	ExpectedRiskRevision       int64  `json:"expected_risk_revision,string" binding:"required"`
	LinkID                     int64  `json:"link_id,string" binding:"required"`
	ExpectedLinkRevision       int64  `json:"expected_link_revision,string" binding:"required"`
	AcceptanceID               int64  `json:"acceptance_id,string" binding:"required"`
	ExpectedAcceptanceRevision int64  `json:"expected_acceptance_revision,string" binding:"required"`
	Reason                     string `json:"reason" binding:"required"`
}

// ReworkAcceptance
//
//	@Summary	接续在办工单的到期风险接受
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		req	body	riskWorkflowAcceptanceRework	true	"原批准、关联和当前修订"
//	@Success	200		{object}	response.Response{data=riskWorkflowOutcome}
//	@Router		/api/v1/risk-items/remediation-accept-rework [post]
func (h *RiskWorkflowHandler) ReworkAcceptance(c *gin.Context) {
	var req riskWorkflowAcceptanceRework
	if !decodeRiskWorkflowInput(c, &req, &req.riskWorkflowRequest) {
		return
	}
	commands, ok := h.commands.(riskWorkflowAcceptanceReworkCommands)
	if !ok {
		writeServiceError(c, errcode.ErrServiceUnavailable)
		return
	}
	command, input := req.inputs(riskActor(c))
	result, err := commands.ReworkAcceptance(c.Request.Context(), riskremediation.AcceptanceRework{
		Command: command, RiskID: req.RiskID, ExpectedRiskRevision: req.ExpectedRiskRevision,
		LinkID: req.LinkID, ExpectedLinkRevision: req.ExpectedLinkRevision, AcceptanceID: req.AcceptanceID,
		ExpectedAcceptanceRevision: req.ExpectedAcceptanceRevision, Reason: req.Reason,
	}, input)
	riskWorkflowReply(c, req.OperationID, result, err)
}

type riskWorkflowAcceptanceRequest struct {
	riskWorkflowRequest
	RiskID               int64     `json:"risk_id,string" binding:"required"`
	ExpectedRiskRevision int64     `json:"expected_risk_revision,string" binding:"required"`
	LinkID               int64     `json:"link_id,string" binding:"required"`
	ExpectedLinkRevision int64     `json:"expected_link_revision,string" binding:"required"`
	Reason               string    `json:"reason" binding:"required"`
	ValidUntil           time.Time `json:"valid_until" binding:"required"`
}

type riskWorkflowAcceptanceDecision struct {
	riskWorkflowRequest
	AcceptanceID               int64  `json:"acceptance_id,string" binding:"required"`
	ExpectedAcceptanceRevision int64  `json:"expected_acceptance_revision,string" binding:"required"`
	ExpectedRiskRevision       int64  `json:"expected_risk_revision,string" binding:"required"`
	Decision                   string `json:"decision" binding:"required" enums:"approve,reject"`
	Reason                     string `json:"reason" binding:"required"`
}

// RequestAcceptance
//
//	@Summary	责任人发起工单关联风险接受
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		req	body	riskWorkflowAcceptanceRequest	true	"风险、关联、期限与原操作号"
//	@Success	200	{object}	response.Response{data=riskWorkflowOutcome}
//	@Router		/api/v1/risk-items/remediation-accept-request [post]
func (h *RiskWorkflowHandler) RequestAcceptance(c *gin.Context) {
	var req riskWorkflowAcceptanceRequest
	if !decodeRiskWorkflowInput(c, &req, &req.riskWorkflowRequest) {
		return
	}
	command, input := req.inputs(riskActor(c))
	result, err := h.commands.RequestAcceptance(c.Request.Context(), riskremediation.AcceptanceRequest{
		Command: command, RiskID: req.RiskID, ExpectedRiskRevision: req.ExpectedRiskRevision,
		LinkID: req.LinkID, ExpectedLinkRevision: req.ExpectedLinkRevision, Reason: req.Reason, ValidUntil: req.ValidUntil,
	}, input)
	riskWorkflowReply(c, req.OperationID, result, err)
}

// DecideAcceptance
//
//	@Summary	独立获权人员审批工单关联风险接受
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		req	body	riskWorkflowAcceptanceDecision	true	"接受申请、审批理由与原操作号"
//	@Success	200	{object}	response.Response{data=riskWorkflowOutcome}
//	@Router		/api/v1/risk-items/remediation-accept-decision [post]
func (h *RiskWorkflowHandler) DecideAcceptance(c *gin.Context) {
	var req riskWorkflowAcceptanceDecision
	if !decodeRiskWorkflowInput(c, &req, &req.riskWorkflowRequest) {
		return
	}
	command, input := req.inputs(riskActor(c))
	result, err := h.commands.DecideAcceptance(c.Request.Context(), riskremediation.AcceptanceDecision{
		Command: command, AcceptanceID: req.AcceptanceID, ExpectedAcceptanceRevision: req.ExpectedAcceptanceRevision,
		ExpectedRiskRevision: req.ExpectedRiskRevision, Decision: req.Decision, Reason: req.Reason,
	}, input)
	riskWorkflowReply(c, req.OperationID, result, err)
}
