package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/errcode"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// JobsHandler /internal/jobs 回调端点 HTTP 层（E-②）：不做用户 JWT——路由组挂
// AK/SK 验签（utils aksk，验 taskrunner 签名）+ 专用网络拓扑（基线 §9）。
// 编排逻辑在 service.JobsCallbackService（分层：handler 不直接触达 repository）。
type JobsHandler struct {
	svc *service.JobsCallbackService
}

func NewJobsHandler(svc *service.JobsCallbackService) *JobsHandler {
	return &JobsHandler{svc: svc}
}

// jobCallbackBody 回调请求体（taskrunner callback client 契约字段）。
type jobCallbackBody struct {
	TaskID    string          `json:"task_id" binding:"required"`
	Action    string          `json:"action" binding:"required"` // C10：标识在 body，URL 统一 /internal/jobs/callback
	RequestID string          `json:"request_id"`                // taskrunner 侧关联键（cron 触发为空）
	Params    json.RawMessage `json:"params"`
	Actor     string          `json:"actor"` // 原始提交人工号（审计归因回传）
	SourceIP  string          `json:"source_ip"`
}

// Callback
//
//	@Summary		预置动作回调（taskrunner → zhuzhao，AK/SK 验签内网端点）
//	@Description	结果映射（P6/P7）：2xx=执行完全成功/幂等受理；404=未知动作；409=不可重试；500=可重试
//	@Tags			internal-jobs
//	@Accept			json
//	@Produce		json
//	@Success		200 {object} response.Response
//	@Router			/internal/jobs/callback [post]
//
// Executed/Idempotent→2xx；UnknownAction→404；NonRetryable→409；Retryable→500。
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
	default:
		response.InternalError(c, msg)
	}
}
