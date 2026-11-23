package handler

import "github.com/gin-gonic/gin"

type riskWorkflowAssigneeRequest struct {
	riskWorkflowRequest
	AssignedToUserID int64 `json:"assigned_to_user_id,string" binding:"required"`
}

// AssignHandler
//
//	@Summary	管理者向具名办理人分派风险整改工单
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		req	body	riskWorkflowAssigneeRequest	true	"目标办理人、工单修订与原操作号"
//	@Success	200	{object}	response.Response{data=riskWorkflowOutcome}
//	@Router		/api/v1/risk-items/remediation-assign-handler [post]
func (h *RiskWorkflowHandler) AssignHandler(c *gin.Context) {
	var req riskWorkflowAssigneeRequest
	if !decodeRiskWorkflowInput(c, &req, &req.riskWorkflowRequest) {
		return
	}
	command, input := req.inputs(riskActor(c))
	result, err := h.commands.AssignHandler(c.Request.Context(), command, req.AssignedToUserID, input)
	riskWorkflowReply(c, req.OperationID, result, err)
}

// AcceptAssignment
//
//	@Summary	工单当前办理人接单
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		req	body	riskWorkflowRequest	true	"工单修订、定义版本与原操作号"
//	@Success	200	{object}	response.Response{data=riskWorkflowOutcome}
//	@Router		/api/v1/risk-items/remediation-accept-assignment [post]
func (h *RiskWorkflowHandler) AcceptAssignment(c *gin.Context) {
	var req riskWorkflowRequest
	if !decodeRiskWorkflowInput(c, &req, &req) {
		return
	}
	command, input := req.inputs(riskActor(c))
	result, err := h.commands.AcceptAssignment(c.Request.Context(), command, input)
	riskWorkflowReply(c, req.OperationID, result, err)
}
