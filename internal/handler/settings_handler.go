package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// SettingsHandler B5-6 设置管理端点（05 号 A.4 #6——GET 列表 / POST 更新）。
type SettingsHandler struct{ svc *service.SettingsService }

func NewSettingsHandler(svc *service.SettingsService) *SettingsHandler {
	return &SettingsHandler{svc: svc}
}

// List GET /api/v1/system/settings
//
//	@Summary		设置列表（catalog 全量——含有效值/来源/编辑层级）
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	response.Response{data=[]model.SettingView}
//	@Router			/api/v1/system/settings [get]
func (h *SettingsHandler) List(c *gin.Context) {
	list, err := h.svc.List(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, list)
}

// Update POST /api/v1/system/settings/update
//
//	@Summary		更新设置（tier 分层+硬底线校验——越界 400）
//	@Tags			system
//	@Accept			json
//	@Produce		json
//	@Param			request	body	model.UpdateSettingRequest	true	"键值对"
//	@Success		200	{object}	response.Response
//	@Router			/api/v1/system/settings/update [post]
func (h *SettingsHandler) Update(c *gin.Context) {
	var req model.UpdateSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.Update(c.Request.Context(), &req, c.GetInt64("userID"), rolesFromGin(c)); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}
