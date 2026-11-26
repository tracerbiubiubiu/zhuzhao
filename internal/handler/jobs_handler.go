package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/errcode"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3protocol"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// JobsHandler /internal/jobs 回调端点 HTTP 层（E-②）：不做用户 JWT——路由组挂
// AK/SK 验签（utils aksk，验 taskrunner 签名）+ 专用网络拓扑（基线 §9）。
// 编排逻辑在 service.JobsCallbackService（分层：handler 不直接触达 repository）。
type JobsHandler struct {
	svc *service.JobsCallbackService
	b3  *service.B3Flow
}

func NewJobsHandler(svc *service.JobsCallbackService) *JobsHandler {
	return &JobsHandler{svc: svc}
}

// jobCallbackBody 回调请求体（taskrunner callback client 契约字段）。
type jobCallbackBody struct {
	ProtocolVersion int             `json:"protocol_version"`
	ActionVersion   int             `json:"action_version"`
	OperationID     string          `json:"operation_id"`
	DeliveryID      string          `json:"delivery_id"`
	JobID           string          `json:"job_id"`
	TaskID          string          `json:"task_id" binding:"required"`
	Action          string          `json:"action" binding:"required"` // C10：标识在 body，URL 统一 /internal/jobs/callback
	RequestID       string          `json:"request_id"`                // taskrunner 侧关联键（cron 触发为空）
	Params          json.RawMessage `json:"params"`
	Actor           string          `json:"actor"` // 原始提交人工号（审计归因回传）
	SourceIP        string          `json:"source_ip"`
}

// Callback
//
//	@Summary		预置动作回调（taskrunner → zhuzhao，AK/SK 验签内网端点）
//	@Description	结果映射：200=已证明成功；400=非法输入；403=无当前动作资格；404=未知动作；409=不可重试；503=仍在途/终态未确认（Retry-After）；500=可重试
//	@Tags			internal-jobs
//	@Accept			json
//	@Produce		json
//	@Success		200 {object} response.Response
//	@Router			/internal/jobs/callback [post]
//
// 只有持久化成功终态可回200，在途/记账未确认保留重试。
func (h *JobsHandler) Callback(c *gin.Context) {
	var body jobCallbackBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "task_id 必填")
		return
	}

	// 关联键：优先 taskrunner 回传的 request_id（X-Request-ID 头对齐前 body 为准），
	// 覆盖 ctx 值供 job_submissions 落档——保证与 taskrunner job_runs 同键跨查。
	if body.RequestID != "" {
		c.Request = c.Request.WithContext(reqid.With(c.Request.Context(), body.RequestID))
	}

	if b3protocol.IsAction(body.Action) {
		if h.b3 == nil || body.JobID != "" {
			writeServiceError(c, errcode.ErrForbidden)
			return
		}
		receipt, err := h.b3.Callback(c.Request.Context(), service.B3Callback{
			ProtocolVersion: body.ProtocolVersion, ActionVersion: body.ActionVersion, TaskID: body.TaskID,
			Action: body.Action, OperationID: body.OperationID, DeliveryID: body.DeliveryID, Params: body.Params,
		})
		if err != nil {
			writeServiceError(c, err)
			return
		}
		response.OK(c, receipt)
		return
	}

	outcome, msg := h.svc.Execute(c.Request.Context(), service.CallbackInput{
		TaskID: body.TaskID, RequestID: body.RequestID, Action: body.Action,
		Params: body.Params, Actor: body.Actor, SourceIP: body.SourceIP,
	})

	switch outcome {
	case service.CallbackExecuted:
		response.OKWithMessage(c, "已执行", gin.H{"task_id": body.TaskID, "status": "succeeded"})
	case service.CallbackIdempotent:
		response.OKWithMessage(c, "已执行（幂等受理）", gin.H{"task_id": body.TaskID, "status": msg})
	case service.CallbackUnknownAction:
		response.NotFound(c, msg)
	case service.CallbackNonRetryable:
		response.Fail(c, http.StatusConflict, errcode.ErrConflict.Code, msg)
	case service.CallbackInProgress:
		c.Header("Retry-After", "1")
		response.Fail(c, http.StatusServiceUnavailable, errcode.ErrServiceUnavailable.Code, msg)
	case service.CallbackInvalidInput:
		response.BadRequest(c, msg)
	case service.CallbackForbidden:
		response.Fail(c, http.StatusForbidden, errcode.ErrForbidden.Code, msg)
	default:
		response.InternalError(c, msg)
	}
}

func (h *JobsHandler) SetB3Flow(flow *service.B3Flow) { h.b3 = flow }
