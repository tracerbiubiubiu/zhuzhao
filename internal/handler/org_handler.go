package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// OrgHandler 组织处理器
type OrgHandler struct {
	orgService *service.OrgService
}

func NewOrgHandler(orgService *service.OrgService) *OrgHandler {
	return &OrgHandler{orgService: orgService}
}

func (h *OrgHandler) GetTree(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

func (h *OrgHandler) Create(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

func (h *OrgHandler) Get(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

func (h *OrgHandler) Update(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

func (h *OrgHandler) Delete(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

func (h *OrgHandler) Move(c *gin.Context) {
	response.InternalError(c, "not implemented")
}
