// MessageAdminHandler 消息管理 API（S6 B6-6 收口——message_log 查看 + message_rules CRUD）。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type MessageAdminHandler struct {
	logRepo  *repository.MessageLogRepo
	ruleRepo *repository.MessageRuleRepo
	pool     *pgxpool.Pool
}

func NewMessageAdminHandler(pool *pgxpool.Pool) *MessageAdminHandler {
	return &MessageAdminHandler{
		logRepo:  repository.NewMessageLogRepo(pool),
		ruleRepo: repository.NewMessageRuleRepo(pool),
		pool:     pool,
	}
}

// ListLogs GET /api/v1/message-logs（发送记录——06 号 §5.4）
func (h *MessageAdminHandler) ListLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	logs, total, err := h.logRepo.List(c.Request.Context(), page, pageSize,
		c.Query("biz_type"), c.Query("template_code"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OKPage(c, logs, total, page, pageSize)
}

// ListRules GET /api/v1/message-rules（订阅规则——06 号 A.5）
func (h *MessageAdminHandler) ListRules(c *gin.Context) {
	rules, err := h.ruleRepo.ListRules(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, rules)
}

// UpsertRule POST /api/v1/message-rules（创建/更新——event_action 唯一键 upsert）
func (h *MessageAdminHandler) UpsertRule(c *gin.Context) {
	var req model.MessageRule
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if req.EventAction == "" || req.TemplateCode == "" {
		response.BadRequest(c, "event_action 与 template_code 必填")
		return
	}
	if req.BizType == "" {
		req.BizType = "ticket"
	}
	if req.RecipientType == "" {
		req.RecipientType = "assignee"
	}
	_, err := h.pool.Exec(c.Request.Context(), `
		INSERT INTO message_rules (biz_type, event_action, template_code, recipient_type, fixed_recipients, enabled, created_by)
		VALUES ($1, $2, $3, $4, NULLIF($5,''), $6, $7)
		ON CONFLICT (biz_type, event_action) DO UPDATE SET
			template_code = EXCLUDED.template_code,
			recipient_type = EXCLUDED.recipient_type,
			fixed_recipients = EXCLUDED.fixed_recipients,
			enabled = EXCLUDED.enabled,
			updated_at = NOW()`,
		req.BizType, req.EventAction, req.TemplateCode, req.RecipientType,
		req.FixedRecipients, req.Enabled, req.CreatedBy)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// DeleteRule POST /api/v1/message-rules/delete
func (h *MessageAdminHandler) DeleteRule(c *gin.Context) {
	var req struct {
		ID int64 `json:"id,string" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	_, err := h.pool.Exec(c.Request.Context(), `DELETE FROM message_rules WHERE id = $1`, req.ID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}
