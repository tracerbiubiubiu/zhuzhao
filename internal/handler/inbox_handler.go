// InboxHandler 站内消息中心端点（S6 二期 B6-7——06 号 A.8.5）。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// InboxHandler 收件箱处理器（读面零渲染——title/link 入站即渲染）。
type InboxHandler struct {
	inboxSvc *ticket.InboxService
}

// NewInboxHandler 创建收件箱处理器。
func NewInboxHandler(svc *ticket.InboxService) *InboxHandler {
	return &InboxHandler{inboxSvc: svc}
}

// Inbox GET /api/v1/notifications/inbox
//
//	@Summary	站内收件箱分页（倒序；unread=true 仅未读）
//	@Tags		notifications
//	@Produce	json
//	@Param		page			query	int		false	"页码"
//	@Param		page_size		query	int		false	"每页"
//	@Param		unread			query	bool	false	"仅未读"
//	@Success	200	{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/notifications/inbox [get]
func (h *InboxHandler) Inbox(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	unread := c.Query("unread") == "true"
	list, total, err := h.inboxSvc.Inbox(c.Request.Context(), c.GetInt64("userID"), unread, page, pageSize)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

type notificationIDReq struct {
	ID int64 `json:"id,string" binding:"required"`
}

// MarkRead POST /api/v1/notifications/read
//
//	@Summary	单条已读（幂等；非本人 404 同形）
//	@Tags		notifications
//	@Accept		json
//	@Produce	json
//	@Param		request	body		notificationIDReq	true	"通知 ID"
//	@Success	200	{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/notifications/read [post]
func (h *InboxHandler) MarkRead(c *gin.Context) {
	var req notificationIDReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.inboxSvc.MarkRead(c.Request.Context(), req.ID, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// MarkAllRead POST /api/v1/notifications/read-all
//
//	@Summary	全部已读（批量——返回置读条数）
//	@Tags		notifications
//	@Produce	json
//	@Success	200	{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/notifications/read-all [post]
func (h *InboxHandler) MarkAllRead(c *gin.Context) {
	n, err := h.inboxSvc.MarkAllRead(c.Request.Context(), c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"marked": n})
}

var _ = errcode.ErrNotFound
