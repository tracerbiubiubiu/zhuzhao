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

// UpdatePassword 修改密码
// POST /api/v1/auth/password/update
func (h *AuthHandler) UpdatePassword(c *gin.Context) {
	response.InternalError(c, "not implemented")
}
