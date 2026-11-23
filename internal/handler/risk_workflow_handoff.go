package handler

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
)

type riskWorkflowHandoffCommands interface {
	Handoff(context.Context, riskremediation.HandoffCommand) (riskremediation.Result, error)
}

type riskWorkflowHandoffRequest struct {
	riskWorkflowVersionedRequest
	SuccessorUserID int64                         `json:"successor_user_id,string" binding:"required"`
	Reason          string                        `json:"reason" binding:"required"`
	Items           []riskWorkflowHandoffItemWire `json:"items" binding:"required"`
}

type riskWorkflowHandoffItemWire struct {
	LinkID                  int64  `json:"link_id,string" binding:"required"`
	ExpectedLinkRevision    int64  `json:"expected_link_revision,string" binding:"required"`
	ExpectedRiskRevision    int64  `json:"expected_risk_revision,string" binding:"required"`
	ExpectedProposalVersion *int64 `json:"expected_proposal_version,string" binding:"required"`
}

// Handoff
//
//	@Summary	交接整组未决风险复测
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		body	body	riskWorkflowHandoffRequest	true	"复测交接请求"
//	@Success	200		{object}	response.Response{data=riskWorkflowOutcome}
//	@Router		/api/v1/risk-items/remediation-handoff [post]
func (h *RiskWorkflowHandler) Handoff(c *gin.Context) {
	var value riskWorkflowHandoffRequest
	if !decodeRiskWorkflowBody(c, &value, &value.riskWorkflowVersionedRequest) {
		return
	}
	if value.SuccessorUserID <= 0 || len(value.Items) == 0 || len(value.Items) > 20 {
		response.BadRequest(c, "复测交接请求参数错误")
		return
	}
	items := make([]riskremediation.HandoffItem, 0, len(value.Items))
	for _, item := range value.Items {
		if item.ExpectedProposalVersion == nil {
			response.BadRequest(c, "复测交接需要每项当前提案版本")
			return
		}
		items = append(items, riskremediation.HandoffItem{LinkID: item.LinkID, ExpectedLinkRevision: item.ExpectedLinkRevision,
			ExpectedRiskRevision: item.ExpectedRiskRevision, ExpectedProposalVersion: *item.ExpectedProposalVersion})
	}
	commands, ok := h.commands.(riskWorkflowHandoffCommands)
	if !ok {
		writeServiceError(c, errcode.ErrServiceUnavailable)
		return
	}
	result, err := commands.Handoff(c.Request.Context(), riskremediation.HandoffCommand{
		Command: value.command(riskActor(c)), DefinitionVersion: value.DefinitionVersion, SuccessorUserID: value.SuccessorUserID, Reason: value.Reason, Items: items,
	})
	riskWorkflowReply(c, value.OperationID, result, err)
}
