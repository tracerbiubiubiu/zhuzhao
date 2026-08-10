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

// List 用户列表
// GET /api/v1/users
func (h *UserHandler) List(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Create 创建用户
// POST /api/v1/users
func (h *UserHandler) Create(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Get 用户详情
// GET /api/v1/users/:id
func (h *UserHandler) Get(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Update 更新用户
// PUT /api/v1/users/:id
func (h *UserHandler) Update(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Delete 删除用户
// DELETE /api/v1/users/:id
func (h *UserHandler) Delete(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// GetMenus 当前用户菜单树
// GET /api/v1/user/menus
func (h *UserHandler) GetMenus(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// GetPermissions 当前用户权限码
// GET /api/v1/user/permissions
func (h *UserHandler) GetPermissions(c *gin.Context) {
	response.InternalError(c, "not implemented")
}
