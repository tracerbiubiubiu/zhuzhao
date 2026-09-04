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

// Submit POST /api/v1/tasks（task:submit）
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

// GetTask GET /api/v1/tasks/:id（task:read）——执行结果唯一出口（受理≠成功）
func (h *TaskrunnerHandler) GetTask(c *gin.Context) {
	data, err := h.svc.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		mapTaskrunnerErr(c, err)
		return
	}
	response.OK(c, data)
}

// ListRuns GET /api/v1/runs（task:read）——query 透传（request_id/action/status/from/to）
func (h *TaskrunnerHandler) ListRuns(c *gin.Context) {
	data, err := h.svc.ListRuns(c.Request.Context(), c.Request.URL.Query())
	if err != nil {
		mapTaskrunnerErr(c, err)
		return
	}
	response.OK(c, data)
}

// ListJobs GET /api/v1/jobs（task:read）——dept 过滤参数由 E-⑤ 策略层决定
func (h *TaskrunnerHandler) ListJobs(c *gin.Context) {
	data, err := h.svc.ListJobs(c.Request.Context(), c.Request.URL.Query())
	if err != nil {
		mapTaskrunnerErr(c, err)
		return
	}
	response.OK(c, data)
}

// CreateJob POST /api/v1/jobs（task:manage）
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

// UpdateJob POST /api/v1/jobs/update（task:manage；id 放 body，仓库惯例）
func (h *TaskrunnerHandler) UpdateJob(c *gin.Context) {
	var req jobIDReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "job_id 必填")
		return
	}
	body, err := c.GetRawData()
	if err != nil {
		response.BadRequest(c, "读取请求体失败")
		return
	}
	data, err := h.svc.UpdateJob(c.Request.Context(), req.JobID, json.RawMessage(body), actorOf(c))
	if err != nil {
		mapTaskrunnerErr(c, err)
		return
	}
	response.OK(c, data)
}

// Trigger POST /api/v1/jobs/trigger（task:manage；「立即执行」）
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

// Cancel POST /api/v1/tasks/cancel（task:manage；仅未开始）
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

// Retry POST /api/v1/tasks/retry（task:manage；失败/死信重试）
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

// DeadLetters GET /api/v1/dead-letters（task:manage；运维向）
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
