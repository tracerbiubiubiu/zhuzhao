package handler

import (
	"encoding/json"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/riskasset"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
)

// RiskHandler Phase 6 B2 第 6 项（D08/D09）风险登记与核实面。
// L1 = CasbinAuth（menu_apis：页面行仅 GET / 写端点挂按钮行——000089 B 案种子）；
// L2/L3 在风险 Service 判定；写命令在 IAM 守卫事务内重验当前授权（C04）。
type RiskHandler struct {
	svc            *risk.Service
	assetDirectory riskasset.Port
}

func NewRiskHandler(svc *risk.Service, assetDirectory riskasset.Port) *RiskHandler {
	return &RiskHandler{svc: svc, assetDirectory: assetDirectory}
}

func riskActor(c *gin.Context) int64 { return c.GetInt64("userID") }

// List GET /api/v1/risk-items
//
//	@Summary	授权风险列表（L2 过滤）
//	@Tags		risk
//	@Produce	json
//	@Param		page			query		int		false	"页码（>=1）"
//	@Param		page_size		query		int		false	"每页条数（1..100）"
//	@Param		status			query		string	false	"状态过滤"
//	@Param		level			query		string	false	"级别过滤"
//	@Param		owner_user_id	query		string	false	"责任人 ID（十进制字符串）"
//	@Param		affected_org_id	query		string	false	"受影响组织 ID（十进制字符串）"
//	@Param		source_ref		query		string	false	"来源过滤"
//	@Param		q				query		string	false	"标题模糊匹配"
//	@Param		sort			query		string	false	"created_at|deadline|level"
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/risk-items [get]
func (h *RiskHandler) List(c *gin.Context) {
	var q model.RiskListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.BadRequest(c, "查询参数不合法")
		return
	}
	res, err := h.svc.List(c.Request.Context(), q, riskActor(c))
	if err != nil {
		writeRiskError(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/risk-items/:id
//
//	@Summary	授权风险详情（不可见与不存在同 404）
//	@Tags		risk
//	@Produce	json
//	@Param		id	path	string	true	"风险 ID（十进制字符串）"
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/risk-items/{id} [get]
func (h *RiskHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 不合法")
		return
	}
	res, err := h.svc.Detail(c.Request.Context(), id, riskActor(c))
	if err != nil {
		writeRiskError(c, err)
		return
	}
	response.OK(c, res)
}

// History GET /api/v1/risk-items/:id/history
//
//	@Summary	风险核实/处置历史（有界分页）
//	@Tags		risk
//	@Produce	json
//	@Param		id			path	string	true	"风险 ID"
//	@Param		page		query	int		false	"页码"
//	@Param		page_size	query	int		false	"每页条数"
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/risk-items/{id}/history [get]
func (h *RiskHandler) History(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 不合法")
		return
	}
	page, pageSize := pageOf(c)
	res, err := h.svc.History(c.Request.Context(), id, page, pageSize, riskActor(c))
	if err != nil {
		writeRiskError(c, err)
		return
	}
	response.OK(c, res)
}

// IngestReceipt GET /api/v1/risk-items/ingest-receipts/:id
//
//	@Summary	按原 ingest_id 受权核查来源收件回执
//	@Tags		risk
//	@Produce	json
//	@Param		id	path	string	true	"收件身份（ingest_id）"
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/risk-items/ingest-receipts/{id} [get]
func (h *RiskHandler) IngestReceipt(c *gin.Context) {
	ingestID := c.Param("id")
	res, err := h.svc.IngestReceipt(c.Request.Context(), ingestID, riskActor(c))
	if err != nil {
		writeRiskError(c, err)
		return
	}
	response.OK(c, res)
}

// Create POST /api/v1/risk-items/create
//
//	@Summary	人工登记风险（主锚管理范围）
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		req	body	model.RiskCreateRequest	true	"登记请求"
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/risk-items/create [post]
func (h *RiskHandler) Create(c *gin.Context) {
	var req model.RiskCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "operation_id/title/org_id 必填")
		return
	}
	res, err := h.svc.Create(c.Request.Context(), req, riskActor(c))
	if err != nil {
		writeRiskError(c, err)
		return
	}
	response.OK(c, res)
}

// Assess POST /api/v1/risk-items/assess
//
//	@Summary	核实/评估（定级、处置策略与整改期限）
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		req	body	model.RiskAssessRequest	true	"评估请求"
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/risk-items/assess [post]
func (h *RiskHandler) Assess(c *gin.Context) {
	var req model.RiskAssessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "operation_id/id/expected_revision/level/treatment 必填")
		return
	}
	res, err := h.svc.Assess(c.Request.Context(), req, riskActor(c))
	if err != nil {
		writeRiskError(c, err)
		return
	}
	response.OK(c, res)
}

// Assign POST /api/v1/risk-items/assign
//
//	@Summary	责任确认（主锚管理者）
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		req	body	model.RiskAssignRequest	true	"指派请求"
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/risk-items/assign [post]
func (h *RiskHandler) Assign(c *gin.Context) {
	var req model.RiskAssignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "operation_id/id/expected_revision/owner_user_id/reason 必填")
		return
	}
	res, err := h.svc.Assign(c.Request.Context(), req, riskActor(c))
	if err != nil {
		writeRiskError(c, err)
		return
	}
	response.OK(c, res)
}

// Report POST /api/v1/risk-items/report
//
//	@Summary	合成来源上报（单条 FindingObservationV1 或 batch≤100）
//	@Tags		risk
//	@Accept		json
//	@Produce	json
//	@Param		req	body	model.RiskReportRequest	true	"批量信封，也接受单条 FindingObservationV1 对象"
//	@Success	200	{object}	response.Response{data=model.RiskReportResult}
//	@Router		/api/v1/risk-items/report [post]
func (h *RiskHandler) ReportFromSource(c *gin.Context, sourceRef string) {
	body, err := c.GetRawData()
	if err != nil {
		response.BadRequest(c, "请求体不可读")
		return
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil {
		response.BadRequest(c, "请求体不是 JSON 对象")
		return
	}
	var req model.RiskReportRequest
	if _, isBatch := probe["observations"]; isBatch {
		if err := json.Unmarshal(body, &req); err != nil {
			response.BadRequest(c, "observations 不合法")
			return
		}
	} else {
		var single model.FindingObservationV1
		if err := json.Unmarshal(body, &single); err != nil {
			response.BadRequest(c, "观察字段不合法")
			return
		}
		req.Observations = []model.FindingObservationV1{single}
	}
	res, err := h.svc.ReportFromSource(c.Request.Context(), req, sourceRef)
	if err != nil {
		writeRiskError(c, err)
		return
	}
	response.OK(c, res)
}

// OperationResult reads the result of an earlier risk mutation.
//
//	@Summary	按原操作号查询风险登记、核实或责任确认的结果
//	@Tags		risk
//	@Produce	json
//	@Param		id	path	string	true	"原操作号"
//	@Success	200	{object}	response.Response{data=model.RiskReceipt}
//	@Router		/api/v1/risk-items/operations/{id} [get]
func (h *RiskHandler) OperationResult(c *gin.Context) {
	result, err := h.svc.OperationResult(c.Request.Context(), c.Param("id"), riskActor(c))
	if err != nil {
		writeRiskError(c, err)
		return
	}
	response.OK(c, result)
}

// AssetEvidence returns current evidence from the directory for an authorized risk.
//
//	@Summary	按风险已绑定资产读取目录证据
//	@Tags		risk
//	@Produce	json
//	@Param		id	path	string	true	"风险 ID（十进制字符串）"
//	@Success	200	{object}	response.Response
//	@Failure	404	{object}	response.Response
//	@Router		/api/v1/risk-items/{id}/asset [get]
func (h *RiskHandler) AssetEvidence(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "id 不合法")
		return
	}
	evidence, err := h.svc.AssetEvidence(c.Request.Context(), id, riskActor(c), h.assetDirectory)
	if err != nil {
		writeRiskError(c, err)
		return
	}
	response.OK(c, evidence)
}
