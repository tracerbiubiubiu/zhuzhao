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

func (h *RoleHandler) List(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

func (h *RoleHandler) Create(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

func (h *RoleHandler) Get(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

func (h *RoleHandler) Update(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

func (h *RoleHandler) Delete(c *gin.Context) {
	response.InternalError(c, "not implemented")
}
