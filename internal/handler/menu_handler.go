package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
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
	var req model.CreateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	menu, err := h.menuService.Create(c.Request.Context(), &req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, menu)
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

// Update POST /api/v1/menus/update
func (h *MenuHandler) Update(c *gin.Context) {
	var req model.UpdateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	menu, err := h.menuService.Update(c.Request.Context(), &req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, menu)
}

// Delete POST /api/v1/menus/delete
func (h *MenuHandler) Delete(c *gin.Context) {
	var req model.MenuIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	if err := h.menuService.Delete(c.Request.Context(), req.MenuID); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}
