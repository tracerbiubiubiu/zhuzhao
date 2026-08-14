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

// GetTree GET /api/v1/orgs
func (h *OrgHandler) GetTree(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Create POST /api/v1/orgs
func (h *OrgHandler) Create(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Get GET /api/v1/orgs/:id
func (h *OrgHandler) Get(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Update POST /api/v1/orgs/update（id 放 body）
func (h *OrgHandler) Update(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Delete POST /api/v1/orgs/delete（id 放 body）
func (h *OrgHandler) Delete(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Move POST /api/v1/orgs/move（id 放 body）
func (h *OrgHandler) Move(c *gin.Context) {
	response.InternalError(c, "not implemented")
}
