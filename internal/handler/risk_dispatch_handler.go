// RiskDispatchHandler 风险派单（Phase 6 B2 第 7 项 / D10）入口：
// 预览（dispatch-plan）与按原号应用（dispatch）。业务标识一律在 body，
// 仅收规范字段——apply 多带业务字段视为覆盖企图（详设 §5.1「不接受覆盖」）。
// 路由注册/Wire/菜单种子属共享接缝，由统筹会话接线（见 dispatch-handoff.md）。
package handler

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskdispatch"
)

type RiskDispatchStatsReader interface {
	ReadDepartmentStats(context.Context, int64) (riskdispatch.DepartmentStatsResult, error)
}

type RiskDispatchHandler struct {
	dispatch *riskdispatch.Dispatcher
	stats    RiskDispatchStatsReader
	canApply func() bool
}

type riskDispatchPlanItemRequest struct {
	RiskID           string `json:"risk_id" binding:"required"`
	ExpectedRevision string `json:"expected_revision,omitempty"`
}

type riskDispatchPlanRequest struct {
	Items          []riskDispatchPlanItemRequest `json:"items" binding:"required"`
	TargetOrgID    string                        `json:"target_org_id" binding:"required"`
	TargetTypeCode string                        `json:"target_type_code" binding:"required"`
}

type riskDispatchApplyRequest struct {
	OperationID string `json:"operation_id" binding:"required"`
	PlanID      string `json:"plan_id" binding:"required"`
	PlanHash    string `json:"plan_hash" binding:"required"`
}

func NewRiskDispatchHandler(d *riskdispatch.Dispatcher, stats RiskDispatchStatsReader, canApply func() bool) *RiskDispatchHandler {
	return &RiskDispatchHandler{dispatch: d, stats: stats, canApply: canApply}
}

// decodeStrict 严格解码：未知字段即 400（固定契约，不接受旧客户端冗余字段覆盖）。
func decodeStrict(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		response.BadRequest(c, "参数错误")
		return false
	}
	return true
}

// parseDecimalID 十进制字符串 ID → int64（公共 wire 不用 JS number 承载 int64）。
func parseDecimalID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errcode.ErrInvalidParams
	}
	return id, nil
}

// DispatchPlan POST /api/v1/risk-items/dispatch-plan
//
//	@Summary	风险派单预览：按漏洞与已核实责任人分组（D10 plan）
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		req	body	riskDispatchPlanRequest	true	"具名风险版本和目标"
//	@Success	200	{object}	response.Response{data=riskdispatch.PlanResult}
//	@Failure	400	{object}	response.Response
//	@Failure	409	{object}	response.Response
//	@Router		/api/v1/risk-items/dispatch-plan [post]
func (h *RiskDispatchHandler) DispatchPlan(c *gin.Context) {
	if h.canApply == nil || !h.canApply() {
		writeServiceError(c, errcode.ErrServiceUnavailable)
		return
	}
	actorID := c.GetInt64("userID")
	var req riskDispatchPlanRequest
	if !decodeStrict(c, &req) {
		return
	}
	input := riskdispatch.PlanInput{TargetTypeCode: req.TargetTypeCode}
	targetOrgID, err := parseDecimalID(req.TargetOrgID)
	if err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	input.TargetOrgID = targetOrgID
	for _, item := range req.Items {
		riskID, err := parseDecimalID(item.RiskID)
		if err != nil {
			response.BadRequest(c, "参数错误")
			return
		}
		expected := riskdispatch.RiskExpected{ID: riskID}
		if item.ExpectedRevision != "" {
			revision, err := strconv.ParseInt(item.ExpectedRevision, 10, 64)
			if err != nil || revision < 0 {
				response.BadRequest(c, "参数错误")
				return
			}
			expected.Revision = revision
		}
		input.Items = append(input.Items, expected)
	}
	out, err := h.dispatch.Plan(c.Request.Context(), actorID, input)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, out)
}

// Dispatch POST /api/v1/risk-items/dispatch
//
//	@Summary	应用派单预览：逐组原子建票/追加并按原号恢复（D10 apply）
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		req	body	riskDispatchApplyRequest	true	"原操作号、预览号及摘要"
//	@Success	200	{object}	response.Response{data=riskdispatch.ApplyResult}
//	@Failure	400	{object}	response.Response
//	@Failure	403	{object}	response.Response
//	@Failure	404	{object}	response.Response
//	@Failure	409	{object}	response.Response
//	@Router		/api/v1/risk-items/dispatch [post]
//	@Router		/api/v1/risk-items/batch-dispatch [post]
func (h *RiskDispatchHandler) Dispatch(c *gin.Context) {
	if h.canApply == nil || !h.canApply() {
		writeServiceError(c, errcode.ErrServiceUnavailable)
		return
	}
	actorID := c.GetInt64("userID")
	var req riskDispatchApplyRequest
	if !decodeStrict(c, &req) {
		return
	}
	planID, err := uuid.Parse(req.PlanID)
	if err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if req.OperationID == "" || len(req.OperationID) > 128 || req.PlanHash == "" {
		response.BadRequest(c, "参数错误")
		return
	}
	out, err := h.dispatch.Apply(c.Request.Context(), actorID, riskdispatch.ApplyInput{
		OperationID: req.OperationID,
		PlanID:      planID,
		PlanHash:    req.PlanHash,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, out)
}

// OperationResult looks up the receipts already stored under the original operation.
//
//	@Summary	按原操作号读取已提交的派单回执
//	@Tags		risk
//	@Produce	json
//	@Param		id	path	string	true	"原操作号"
//	@Success	200	{object}	response.Response{data=riskdispatch.ApplyResult}
//	@Failure	404	{object}	response.Response
//	@Router		/api/v1/risk-items/dispatch-operations/{id} [get]
func (h *RiskDispatchHandler) OperationResult(c *gin.Context) {
	result, err := h.dispatch.ReadOperation(c.Request.Context(), c.GetInt64("userID"), c.Param("id"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, result)
}

// DepartmentStats returns current and historical counts within both read scopes.
//
//	@Summary	按当前及派单时主部门读取已授权的派单统计
//	@Tags		risk
//	@Produce	json
//	@Success	200	{object}	response.Response{data=riskdispatch.DepartmentStatsResult}
//	@Router		/api/v1/risk-items/dispatch-statistics [get]
func (h *RiskDispatchHandler) DepartmentStats(c *gin.Context) {
	result, err := h.stats.ReadDepartmentStats(c.Request.Context(), c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, result)
}
