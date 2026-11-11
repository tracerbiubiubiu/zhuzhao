// IntelHandler 情报查询端点（S9 B9-1——09 号附录 A.4）。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/threatintel"
)

// IntelHandler 情报查询处理器。
type IntelHandler struct {
	svc *threatintel.Service
}

// NewIntelHandler 创建情报查询处理器。
func NewIntelHandler(svc *threatintel.Service) *IntelHandler {
	return &IntelHandler{svc: svc}
}

// Query GET /api/v1/intel/query?type=ip&value=1.2.3.4
//
//	@Summary		威胁情报查询（先缓存后回源；stale=过期缓存降级标记）
//	@Description	外部限流 API 的缓存代理（ACL 防腐——result 上游原样透传）；配额耗尽降级返回过期缓存带 stale 标记
//	@Tags			intel
//	@Produce		json
//	@Param			type	query	string	true	"情报类型（ip/domain/hash/url）"
//	@Param			value	query	string	true	"查询值"
//	@Success		200		{object}	response.Response
//	@Failure		400		{object}	response.Response
//	@Security		BearerAuth
//	@Router			/api/v1/intel/query [get]
func (h *IntelHandler) Query(c *gin.Context) {
	intelType := c.Query("type")
	value := c.Query("value")
	if intelType == "" || value == "" {
		response.BadRequest(c, "type 与 value 必填")
		return
	}
	resp, err := h.svc.Query(c.Request.Context(), intelType, value)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, resp)
}

var _ = errcode.ErrIntelNotConfigured // 引用占位（映射表登记见 errors.go）
