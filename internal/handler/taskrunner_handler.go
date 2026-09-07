package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/errcode"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"

	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// TaskrunnerHandler 任务管理端点（E-④，16 号 §3）：三层校验（JWT/Casbin/审计）
// 后代理 taskrunner API。权限码：task:submit / task:read / task:manage（000022 seed）。
type TaskrunnerHandler struct {
	svc         *service.TaskrunnerService
	selfBaseURL string // 缺省 callback 拼接用（部署同网可达的内网地址，config 可覆盖请求值）
}

func NewTaskrunnerHandler(svc *service.TaskrunnerService, selfBaseURL string) *TaskrunnerHandler {
	return &TaskrunnerHandler{svc: svc, selfBaseURL: selfBaseURL}
}

// actor 从 gin ctx 取工号（JWT 中间件注入；system 场景为空串）。
func actorOf(c *gin.Context) string { return c.GetString("username") }

// Submit
//
//	@Summary		提交一次性任务（受理 ≠ 执行成功，结果经查询接口获取）
//	@Tags			tasks
//	@Accept			json
//	@Produce		json
//	@Param			req body service.TaskSubmitInput true "提交参数（action 必填；callback_url 可缺省自动拼）"
//	@Success		200 {object} response.Response "task_id + accepted"
//	@Failure		400 {object} response.Response
//	@Failure		502 {object} response.Response "taskrunner 未配置/不可达"
//	@Security		BearerAuth
//	@Router			/api/v1/tasks [post]
func (h *TaskrunnerHandler) Submit(c *gin.Context) {
	var req service.TaskSubmitInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "action 必填")
		return
	}
	resp, err := h.svc.Submit(c.Request.Context(), &req, actorOf(c), c.ClientIP(), h.selfBaseURL)
	if err != nil {
		mapTaskrunnerErr(c, err)
		return
	}
	response.OK(c, resp)
}

// GetTask
//
//	@Summary		查任务状态（job_runs 全景 + live_state 实时态）
//	@Description	执行结果的唯一出口；taskrunner 不主动推送
//	@Tags			tasks
//	@Produce		json
//	@Param			id path string true "task_id"
//	@Success		200 {object} response.Response
//	@Failure		404 {object} response.Response
//	@Security		BearerAuth
//	@Router			/api/v1/tasks/{id} [get]
func (h *TaskrunnerHandler) GetTask(c *gin.Context) {
	data, err := h.svc.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		mapTaskrunnerErr(c, err)
		return
	}
	response.OK(c, data)
}

// ListRuns
//
//	@Summary		查执行记录（request_id 跨查即此）
//	@Description	query 透传：request_id / action / status / job_id / from / to（RFC3339）
//	@Tags			tasks
//	@Produce		json
//	@Success		200 {object} response.Response
//	@Security		BearerAuth
//	@Router			/api/v1/runs [get]
func (h *TaskrunnerHandler) ListRuns(c *gin.Context) {
	data, err := h.svc.ListRuns(c.Request.Context(), c.Request.URL.Query())
	if err != nil {
		mapTaskrunnerErr(c, err)
		return
	}
	response.OK(c, data)
}

// ListJobs
//
//	@Summary		任务定义列表（dept/action_id/enabled 过滤透传）
//	@Tags			jobs
//	@Produce		json
//	@Success		200 {object} response.Response
//	@Security		BearerAuth
//	@Router			/api/v1/jobs [get]
func (h *TaskrunnerHandler) ListJobs(c *gin.Context) {
	data, err := h.svc.ListJobs(c.Request.Context(), c.Request.URL.Query())
	if err != nil {
		mapTaskrunnerErr(c, err)
		return
	}
	response.OK(c, data)
}

// CreateJob
//
//	@Summary		新建任务定义（action_id + trigger_type + cron_spec + params）
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Success		200 {object} response.Response
//	@Failure		400 {object} response.Response "cron_spec 非法等"
//	@Security		BearerAuth
//	@Router			/api/v1/jobs [post]
func (h *TaskrunnerHandler) CreateJob(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		response.BadRequest(c, "读取请求体失败")
		return
	}
	data, err := h.svc.CreateJob(c.Request.Context(), json.RawMessage(body), actorOf(c))
	if err != nil {
		mapTaskrunnerErr(c, err)
		return
	}
	response.OK(c, data)
}

type jobIDReq struct {
	JobID string `json:"job_id" binding:"required"`
}

// UpdateJob
//
//	@Summary		修改任务定义（cron/params/启停；生效 ≤ 下个 cron tick）
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Success		200 {object} response.Response
//	@Security		BearerAuth
//	@Router			/api/v1/jobs/update [post]
func (h *TaskrunnerHandler) UpdateJob(c *gin.Context) {
	// 单次读 raw 再解字段：ShouldBindJSON 会耗尽 body，二次 GetRawData 拿到空
	raw, err := c.GetRawData()
	if err != nil {
		response.BadRequest(c, "读取请求体失败")
		return
	}
	var req jobIDReq
	if err := json.Unmarshal(raw, &req); err != nil || req.JobID == "" {
		response.BadRequest(c, "job_id 必填")
		return
	}
	data, err := h.svc.UpdateJob(c.Request.Context(), req.JobID, json.RawMessage(raw), actorOf(c))
	if err != nil {
		mapTaskrunnerErr(c, err)
		return
	}
	response.OK(c, data)
}

// Trigger
//
//	@Summary		手动执行一次任务定义（「立即执行」按钮）
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Success		200 {object} response.Response "task_id + accepted"
//	@Failure		409 {object} response.Response "定义已停用"
//	@Security		BearerAuth
//	@Router			/api/v1/jobs/trigger [post]
func (h *TaskrunnerHandler) Trigger(c *gin.Context) {
	var req jobIDReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "job_id 必填")
		return
	}
	resp, err := h.svc.Trigger(c.Request.Context(), req.JobID, actorOf(c), c.ClientIP())
	if err != nil {
		mapTaskrunnerErr(c, err)
		return
	}
	response.OK(c, resp)
}

type taskIDReq struct {
	TaskID string `json:"task_id" binding:"required"`
}

// Cancel
//
//	@Summary		取消未开始的任务（执行中 409）
//	@Tags			tasks
//	@Accept			json
//	@Produce		json
//	@Success		200 {object} response.Response
//	@Failure		409 {object} response.Response
//	@Security		BearerAuth
//	@Router			/api/v1/tasks/cancel [post]
func (h *TaskrunnerHandler) Cancel(c *gin.Context) {
	var req taskIDReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "task_id 必填")
		return
	}
	data, err := h.svc.CancelTask(c.Request.Context(), req.TaskID, actorOf(c))
	if err != nil {
		mapTaskrunnerErr(c, err)
		return
	}
	response.OK(c, data)
}

// Retry
//
//	@Summary		重试失败/死信任务
//	@Tags			tasks
//	@Accept			json
//	@Produce		json
//	@Success		200 {object} response.Response
//	@Failure		409 {object} response.Response
//	@Security		BearerAuth
//	@Router			/api/v1/tasks/retry [post]
func (h *TaskrunnerHandler) Retry(c *gin.Context) {
	var req taskIDReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "task_id 必填")
		return
	}
	data, err := h.svc.RetryTask(c.Request.Context(), req.TaskID, actorOf(c))
	if err != nil {
		mapTaskrunnerErr(c, err)
		return
	}
	response.OK(c, data)
}

// DeadLetters
//
//	@Summary		死信列表（运维向，配合 retry 端点闭环）
//	@Tags			tasks
//	@Produce		json
//	@Success		200 {object} response.Response
//	@Security		BearerAuth
//	@Router			/api/v1/dead-letters [get]
func (h *TaskrunnerHandler) DeadLetters(c *gin.Context) {
	data, err := h.svc.ListDeadLetters(c.Request.Context(), c.Request.URL.Query())
	if err != nil {
		mapTaskrunnerErr(c, err)
		return
	}
	response.OK(c, data)
}

// mapTaskrunnerErr taskrunner 信封错误（errcode.Error，码段与 zhuzhao 同源——
// utils errcode）→ 复用 writeServiceError 的码→状态映射；非 errcode 错误
// （网络/超时等）按 502 上游不可用透出，避免伪装成本服务内部 500。
func mapTaskrunnerErr(c *gin.Context, err error) {
	var biz *errcode.Error
	if errors.As(err, &biz) {
		writeServiceError(c, err)
		return
	}
	response.Fail(c, http.StatusBadGateway, errcode.ErrServiceUnavailable.Code,
		"任务服务不可达: "+err.Error())
}
