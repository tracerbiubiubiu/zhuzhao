package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
)

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
