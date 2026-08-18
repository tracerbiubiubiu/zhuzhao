package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// UserHandler 用户处理器
type UserHandler struct {
	userService *service.UserService
	menuService *service.MenuService
}

func NewUserHandler(userService *service.UserService, menuService *service.MenuService) *UserHandler {
	return &UserHandler{userService: userService, menuService: menuService}
}

// List GET /api/v1/users
func (h *UserHandler) List(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Create POST /api/v1/users
func (h *UserHandler) Create(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Get GET /api/v1/users/:id
func (h *UserHandler) Get(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Update POST /api/v1/users/update（id 放 body）
func (h *UserHandler) Update(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Delete POST /api/v1/users/delete（id 放 body）
func (h *UserHandler) Delete(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// UpdateStatus POST /api/v1/users/status（id 放 body）
func (h *UserHandler) UpdateStatus(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// SetRoles POST /api/v1/users/roles（id 放 body）
func (h *UserHandler) SetRoles(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// ResetPassword POST /api/v1/users/password/reset（id 放 body）
func (h *UserHandler) ResetPassword(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// GetUserOrgs GET /api/v1/users/:id/orgs
func (h *UserHandler) GetUserOrgs(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// SetUserOrgs POST /api/v1/users/orgs（全量覆盖用户组织绑定）
func (h *UserHandler) SetUserOrgs(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// GetProfile GET /api/v1/user/profile
func (h *UserHandler) GetProfile(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// UpdateProfile POST /api/v1/user/profile/update
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// GetMenus GET /api/v1/user/menus
func (h *UserHandler) GetMenus(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// GetPermissions GET /api/v1/user/permissions
func (h *UserHandler) GetPermissions(c *gin.Context) {
	response.InternalError(c, "not implemented")
}
