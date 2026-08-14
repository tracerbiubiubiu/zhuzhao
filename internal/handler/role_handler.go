package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// RoleHandler 角色处理器
type RoleHandler struct {
	rbacService *service.RBACService
}

func NewRoleHandler(rbacService *service.RBACService) *RoleHandler {
	return &RoleHandler{rbacService: rbacService}
}

// List GET /api/v1/roles
func (h *RoleHandler) List(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Create POST /api/v1/roles
func (h *RoleHandler) Create(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Get GET /api/v1/roles/:id
func (h *RoleHandler) Get(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Update POST /api/v1/roles/update（id 放 body）
func (h *RoleHandler) Update(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Delete POST /api/v1/roles/delete（id 放 body）
func (h *RoleHandler) Delete(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// AssignMenus POST /api/v1/roles/menus（id 放 body）
func (h *RoleHandler) AssignMenus(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// GetMenus GET /api/v1/roles/:id/menus
func (h *RoleHandler) GetMenus(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// GetPermissions GET /api/v1/roles/:id/permissions
func (h *RoleHandler) GetPermissions(c *gin.Context) {
	response.InternalError(c, "not implemented")
}
