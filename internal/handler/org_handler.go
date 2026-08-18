package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
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
	tree, err := h.orgService.GetTree(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, tree)
}

// Create POST /api/v1/orgs
func (h *OrgHandler) Create(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Get GET /api/v1/orgs/:id
func (h *OrgHandler) Get(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Update POST /api/v1/orgs/update
func (h *OrgHandler) Update(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Delete POST /api/v1/orgs/delete
func (h *OrgHandler) Delete(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Move POST /api/v1/orgs/move
func (h *OrgHandler) Move(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// GetMembers GET /api/v1/orgs/:id/members
func (h *OrgHandler) GetMembers(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// AddMember POST /api/v1/orgs/members
func (h *OrgHandler) AddMember(c *gin.Context) {
	var req model.OrgMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.orgService.AddMember(c.Request.Context(), &req); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// RemoveMember POST /api/v1/orgs/members/delete
func (h *OrgHandler) RemoveMember(c *gin.Context) {
	var req model.OrgMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.orgService.RemoveMember(c.Request.Context(), &req); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}
