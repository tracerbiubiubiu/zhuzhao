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

func (h *MenuHandler) GetTree(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

func (h *MenuHandler) Create(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

func (h *MenuHandler) Get(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

func (h *MenuHandler) Update(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

func (h *MenuHandler) Delete(c *gin.Context) {
	response.InternalError(c, "not implemented")
}
