package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// PatHandler P4-6 PAT 自服务面（SelfService——本人凭据本人管，无 Casbin 面；
// 认证消费=JWT 中间件 zpat_ 分支）。
type PatHandler struct {
	patService *service.PatService
}

func NewPatHandler(s *service.PatService) *PatHandler {
	return &PatHandler{patService: s}
}

// List GET /user/pats（明文永不出网——列表仅元数据）
func (h *PatHandler) List(c *gin.Context) {
	list, err := h.patService.List(c.Request.Context(), c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"pats": list})
}

// Create POST /user/pats——响应含明文 secret（仅此一次；前端弹窗展示+复制）
func (h *PatHandler) Create(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		ExpiresDays int    `json:"expires_days"` // 0=永不过期
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "name 必填")
		return
	}
	t, plain, err := h.patService.Create(c.Request.Context(), c.GetInt64("userID"), req.Name, req.ExpiresDays)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"pat": t, "secret": plain})
}

// Revoke POST /user/pats/delete（id 入 body；本人双条件防越权）
func (h *PatHandler) Revoke(c *gin.Context) {
	var req struct {
		ID int64 `json:"id,string" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "id 必填")
		return
	}
	if err := h.patService.Revoke(c.Request.Context(), c.GetInt64("userID"), req.ID); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OKWithMessage(c, "已吊销", nil)
}
