package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/errcode"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// JobsHandler /internal/jobs 回调端点（E-②，16 号 §3；taskrunner 仓库
// zhuzhao-integration.md §2.1 契约）：不走用户 JWT——路由组挂 AK/SK 验签
// （utils aksk，验 taskrunner 签名）+ 专用网络拓扑（基线 §9）。
type JobsHandler struct {
	registry *jobs.Registry
	repo     *repository.JobSubmissionRepo
}

func NewJobsHandler(registry *jobs.Registry, repo *repository.JobSubmissionRepo) *JobsHandler {
	return &JobsHandler{registry: registry, repo: repo}
}

// jobCallbackBody 回调请求体（taskrunner callback client 契约字段）。
type jobCallbackBody struct {
	TaskID    string          `json:"task_id" binding:"required"`
	RequestID string          `json:"request_id"` // taskrunner 侧关联键（cron 触发为空）
	Params    json.RawMessage `json:"params"`
	Actor     string          `json:"actor"` // 原始提交人工号（审计归因回传）
	SourceIP  string          `json:"source_ip"`
}

// Callback POST /internal/jobs/:action_id。
//
// 契约（P6/P7 定案）：
//   - 未知 action_id → 404（不做前置校验，快速失败不重试）；
//   - 已 succeeded 的 task_id 再次回调 → 2xx 幂等受理（at-least-once 防副作用重复）；
//   - Handle 返回 nil → 2xx（执行完全成功）；
//   - ErrAbort → 409（不可重试业务失败）；其他错误 → 500（可重试，taskrunner 退避重试）。
func (h *JobsHandler) Callback(c *gin.Context) {
	action := c.Param("action_id")

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
	ctx := c.Request.Context()

	handler, ok := h.registry.Get(action)
	if !ok {
		// P6：无前置校验的兜底——未知动作 4xx 快速失败（taskrunner 判 failed 终态）
		response.NotFound(c, "未注册的动作: "+action)
		return
	}

	row, alreadyDone, err := h.repo.EnsureCallbackRow(ctx, body.TaskID, action, body.Actor, body.SourceIP)
	if err != nil {
		response.InternalError(c, "回调受理失败")
		return
	}
	if alreadyDone {
		// 幂等拦截：该 task_id 已执行完全成功，重复回调直接 2xx（不重复执行副作用）
		response.OKWithMessage(c, "已执行（幂等受理）", gin.H{"task_id": body.TaskID, "status": row.Status})
		return
	}

	if err := handler.Handle(ctx, body.Params); err != nil {
		_ = h.repo.MarkFailed(ctx, body.TaskID, err.Error())
		if errors.Is(err, jobs.ErrAbort) {
			response.Fail(c, http.StatusConflict, errcode.ErrConflict.Code, "动作执行失败（不可重试）: "+err.Error())
			return
		}
		response.InternalError(c, "动作执行失败（可重试）")
		return
	}
	if err := h.repo.MarkSucceeded(ctx, body.TaskID); err != nil {
		// 执行已成功、记账 UPDATE 失败（极低概率）：业务事实已发生，必须返回 2xx；
		// 若 taskrunner 因超时等重试，会再次进入 Handle——Handler 可重入契约兜底
		// （registry.go Handler 注释，B11② 语义），此处仅留痕
		slog.Warn("jobs: mark succeeded failed after execution",
			slog.String("task_id", body.TaskID), slog.String("action", action), slog.Any("err", err))
	}
	response.OKWithMessage(c, "已执行", gin.H{"task_id": body.TaskID, "status": "succeeded"})
}
