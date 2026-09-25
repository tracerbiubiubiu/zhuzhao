package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
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

// Get GET /api/v1/menus/:id
func (h *MenuHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的菜单 ID")
		return
	}
	menu, err := h.menuService.GetByID(c.Request.Context(), id)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, menu)
}
