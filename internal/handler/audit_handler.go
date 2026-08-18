package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// AuditHandler 审计日志处理器
type AuditHandler struct {
	auditService *service.AuditService
}

func NewAuditHandler(auditService *service.AuditService) *AuditHandler {
	return &AuditHandler{auditService: auditService}
}

// ListLogs GET /api/v1/audit/logs
func (h *AuditHandler) ListLogs(c *gin.Context) {
	q := repository.AuditListQuery{
		Page:     queryInt(c, "page", 1),
		PageSize: queryInt(c, "page_size", 20),
		Path:     c.Query("path"),
	}
	if uid := c.Query("user_id"); uid != "" {
		id, err := strconv.ParseInt(uid, 10, 64)
		if err != nil {
			response.BadRequest(c, "无效的用户 ID")
			return
		}
		q.UserID = &id
	}
	if start := c.Query("start"); start != "" {
		t, err := time.Parse("2006-01-02", start)
		if err != nil {
			response.BadRequest(c, "无效的开始日期")
			return
		}
		q.Start = &t
	}
	if end := c.Query("end"); end != "" {
		t, err := time.Parse("2006-01-02", end)
		if err != nil {
			response.BadRequest(c, "无效的结束日期")
			return
		}
		q.End = &t
	}

	resp, err := h.auditService.List(c.Request.Context(), q)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, resp)
}
