package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// FalsePositive closes an unlinked risk and records a bounded suppression.
//
//	@Summary	关闭误报并记录限期抑制
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.RiskFalsePositiveRequest	true	"误报、抑制期限及回执身份"
//	@Success	200		{object}	response.Response{data=model.RiskSuppressionActionResult}
//	@Router		/api/v1/risk-items/false-positive [post]
func (h *RiskHandler) FalsePositive(c *gin.Context) {
	var req model.RiskFalsePositiveRequest
	if !decodeStrict(c, &req) {
		return
	}
	out, err := h.svc.FalsePositive(c.Request.Context(), req, riskActor(c))
	if err != nil {
		writeRiskError(c, err)
		return
	}
	response.OK(c, out)
}

// Suppressions lists suppression history for areas managed by the current actor.
//
//	@Summary	按主锚管理权限查询误报抑制
//	@Tags		risk
//	@Produce	json
//	@Param		page		query	int	false	"页码"
//	@Param		page_size	query	int	false	"每页条数"
//	@Success	200		{object}	response.Response{data=model.RiskSuppressionList}
//	@Router		/api/v1/risk-suppressions [get]
func (h *RiskHandler) Suppressions(c *gin.Context) {
	page, pageSize := pageOf(c)
	out, err := h.svc.ListSuppressions(c.Request.Context(), riskActor(c), page, pageSize)
	if err != nil {
		writeRiskError(c, err)
		return
	}
	response.OK(c, out)
}

// RemoveSuppression ends an active suppression, retaining the historical closure.
//
//	@Summary	终止误报抑制
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.RiskSuppressionRemoveRequest	true	"抑制及当前修订"
//	@Success	200		{object}	response.Response{data=model.RiskSuppressionActionResult}
//	@Router		/api/v1/risk-suppressions/remove [post]
func (h *RiskHandler) RemoveSuppression(c *gin.Context) {
	var req model.RiskSuppressionRemoveRequest
	if !decodeStrict(c, &req) {
		return
	}
	out, err := h.svc.RemoveSuppression(c.Request.Context(), req, riskActor(c))
	if err != nil {
		writeRiskError(c, err)
		return
	}
	response.OK(c, out)
}
