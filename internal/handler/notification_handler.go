package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// NotificationHandler P4-2 通知通道：管理 API 面（配置页触发驱动后补——
// 2026-09-21 拍板）+ 内网死信告警入口（/internal/notify/dead-letter，
// taskrunner 终败投递，AK/SK 验签由 internal 组中间件承担）。
type NotificationHandler struct {
	notificationService *service.NotificationService
}

func NewNotificationHandler(s *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{notificationService: s}
}

// List
//
//	@Summary		通知配置列表
//	@Tags			notifications
//	@Produce		json
//	@Param			page query int false "页码"
//	@Param			page_size query int false "页大小"
//	@Param			code query string false "code 模糊过滤"
//	@Success		200 {object} response.Response
//	@Security		BearerAuth
//	@Router			/api/v1/notifications [get]
func (h *NotificationHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	resp, err := h.notificationService.List(c.Request.Context(), page, pageSize, c.Query("code"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, resp)
}

type notificationCreateRequest struct {
	Code       string `json:"code" binding:"required"`
	Name       string `json:"name" binding:"required"`
	Channel    string `json:"channel"`
	WebhookURL string `json:"webhook_url" binding:"required"`
	Enabled    *bool  `json:"enabled"`
}

// Create
//
//	@Summary		新建通知配置（webhook 渠道）
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Success		200 {object} response.Response
//	@Failure		400 {object} response.Response
//	@Security		BearerAuth
//	@Router			/api/v1/notifications [post]
func (h *NotificationHandler) Create(c *gin.Context) {
	var req notificationCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "code/name/webhook_url 必填")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	in := &model.NotificationConfig{
		Code: req.Code, Name: req.Name, Channel: req.Channel,
		WebhookURL: req.WebhookURL, Enabled: enabled,
	}
	if err := h.notificationService.Create(c.Request.Context(), in); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, in)
}

type notificationUpdateRequest struct {
	ID         int64  `json:"id,string" binding:"required"`
	Name       string `json:"name" binding:"required"`
	Channel    string `json:"channel"`
	WebhookURL string `json:"webhook_url" binding:"required"`
	Enabled    *bool  `json:"enabled"`
	Version    int64  `json:"version" binding:"required"`
}

// Update
//
//	@Summary		更新通知配置（version 乐观锁）
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Success		200 {object} response.Response
//	@Failure		400 {object} response.Response
//	@Security		BearerAuth
//	@Router			/api/v1/notifications/update [post]
func (h *NotificationHandler) Update(c *gin.Context) {
	var req notificationUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "id/name/webhook_url/version 必填")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	in := &model.NotificationConfig{
		ID: req.ID, Name: req.Name, Channel: req.Channel,
		WebhookURL: req.WebhookURL, Enabled: enabled, Version: req.Version,
	}
	if err := h.notificationService.Update(c.Request.Context(), in); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OKWithMessage(c, "已更新", nil)
}

// Delete
//
//	@Summary		删除通知配置（body 携带 code——zhuzhao 风格）
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Success		200 {object} response.Response
//	@Security		BearerAuth
//	@Router			/api/v1/notifications/delete [post]
func (h *NotificationHandler) Delete(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "code 必填")
		return
	}
	if err := h.notificationService.Delete(c.Request.Context(), req.Code); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OKWithMessage(c, "已删除", nil)
}

// DeadLetter 内网死信告警入口（/internal/notify/dead-letter——taskrunner 终败投递；
// 受理即 2xx：分发尽力而为，失败只记日志）
func (h *NotificationHandler) DeadLetter(c *gin.Context) {
	var p model.DeadLetterNotifyPayload
	if err := c.ShouldBindJSON(&p); err != nil || p.TaskID == "" || p.Action == "" {
		response.BadRequest(c, "task_id/action 必填")
		return
	}
	if err := h.notificationService.NotifyDeadLetter(c.Request.Context(), &p); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OKWithMessage(c, "已受理", nil)
}
