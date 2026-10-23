// NotificationPrefHandler 用户通知偏好自服务端点（B6-3 增强批——06 号 §5.1 偏好面）。
package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
)

type NotificationPrefHandler struct {
	pool *pgxpool.Pool
}

func NewNotificationPrefHandler(pool *pgxpool.Pool) *NotificationPrefHandler {
	return &NotificationPrefHandler{pool: pool}
}

// ListOptouts GET /api/v1/me/notification-optouts
func (h *NotificationPrefHandler) ListOptouts(c *gin.Context) {
	uid := c.GetInt64("user_id")
	rows, err := h.pool.Query(c.Request.Context(),
		`SELECT biz_type, event_action FROM notification_optouts WHERE user_id = $1 ORDER BY biz_type, event_action`, uid)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	defer rows.Close()
	type Optout struct {
		BizType     string `json:"biz_type"`
		EventAction string `json:"event_action"`
	}
	out := []Optout{}
	for rows.Next() {
		var o Optout
		if err := rows.Scan(&o.BizType, &o.EventAction); err != nil {
			writeServiceError(c, err)
			return
		}
		out = append(out, o)
	}
	response.OK(c, out)
}

// SetOptout POST /api/v1/me/notification-optouts（body: {biz_type, event_action, opt_out}）
func (h *NotificationPrefHandler) SetOptout(c *gin.Context) {
	uid := c.GetInt64("user_id")
	var req struct {
		BizType     string `json:"biz_type" binding:"required"`
		EventAction string `json:"event_action" binding:"required"`
		OptOut      bool   `json:"opt_out"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if req.OptOut {
		_, err := h.pool.Exec(c.Request.Context(),
			`INSERT INTO notification_optouts (user_id, biz_type, event_action) VALUES ($1, $2, $3)
			 ON CONFLICT (user_id, biz_type, event_action) DO NOTHING`, uid, req.BizType, req.EventAction)
		if err != nil {
			writeServiceError(c, err)
			return
		}
	} else {
		_, err := h.pool.Exec(c.Request.Context(),
			`DELETE FROM notification_optouts WHERE user_id = $1 AND biz_type = $2 AND event_action = $3`,
			uid, req.BizType, req.EventAction)
		if err != nil {
			writeServiceError(c, err)
			return
		}
	}
	response.OK(c, nil)
}
