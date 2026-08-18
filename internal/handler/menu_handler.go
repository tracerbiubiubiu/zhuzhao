package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// MenuHandler 菜单处理器
type MenuHandler struct {
	menuService *service.MenuService
}

func NewMenuHandler(menuService *service.MenuService) *MenuHandler {
	return &MenuHandler{menuService: menuService}
}

// GetTree GET /api/v1/menus
func (h *MenuHandler) GetTree(c *gin.Context) {
	tree, err := h.menuService.GetTree(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, tree)
}

// Create POST /api/v1/menus
func (h *MenuHandler) Create(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Get GET /api/v1/menus/:id
func (h *MenuHandler) Get(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Update POST /api/v1/menus/update（id 放 body）
func (h *MenuHandler) Update(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Delete POST /api/v1/menus/delete（id 放 body）
func (h *MenuHandler) Delete(c *gin.Context) {
	response.InternalError(c, "not implemented")
}
