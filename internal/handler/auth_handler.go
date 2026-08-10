package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Login 登录
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Refresh 刷新 Token
// POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// Logout 登出
// POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// ListDevices 查询活跃设备列表
// GET /api/v1/auth/devices
func (h *AuthHandler) ListDevices(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

// KickDevice 踢出指定设备
// DELETE /api/v1/auth/devices/:deviceId
func (h *AuthHandler) KickDevice(c *gin.Context) {
	response.InternalError(c, "not implemented")
}
