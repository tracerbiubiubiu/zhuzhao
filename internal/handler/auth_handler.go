package handler

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
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
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcode.ErrInvalidParams.Message)
		return
	}

	pair, err := h.authService.Login(c.Request.Context(), &req, c.ClientIP())
	if err != nil {
		writeAuthError(c, err)
		return
	}
	response.OK(c, pair)
}

// Refresh 刷新 Token
// POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req model.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcode.ErrInvalidParams.Message)
		return
	}

	pair, err := h.authService.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		writeAuthError(c, err)
		return
	}
	response.OK(c, pair)
}

// Logout 登出
// POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	auth := c.GetHeader("Authorization")
	if auth == "" {
		response.Unauthorized(c, errcode.ErrUnauthorized.Message)
		return
	}

	var req model.LogoutRequest
	_ = c.ShouldBindJSON(&req)

	token := extractBearer(auth)
	if token == "" {
		response.Unauthorized(c, errcode.ErrUnauthorized.Message)
		return
	}

	if err := h.authService.Logout(c.Request.Context(), token, req.DeviceID); err != nil {
		writeAuthError(c, err)
		return
	}
	response.OK(c, nil)
}

// UpdatePassword 修改密码
// POST /api/v1/auth/password/update
func (h *AuthHandler) UpdatePassword(c *gin.Context) {
	response.InternalError(c, "not implemented")
}

func writeAuthError(c *gin.Context, err error) {
	var biz *errcode.Error
	if !errors.As(err, &biz) {
		response.InternalError(c, errcode.ErrInternal.Message)
		return
	}
	switch biz.Code {
	case errcode.ErrInvalidParams.Code:
		response.BadRequest(c, biz.Message)
	case errcode.ErrInvalidCredentials.Code, errcode.ErrRefreshTokenInvalid.Code, errcode.ErrTokenInvalid.Code:
		response.UnauthorizedError(c, biz)
	case errcode.ErrAccountLocked.Code:
		response.TooManyRequests(c, biz)
	case errcode.ErrServiceUnavailable.Code:
		response.ServiceUnavailable(c)
	default:
		response.InternalError(c, biz.Message)
	}
}

func extractBearer(auth string) string {
	const prefix = "Bearer "
	if len(auth) > len(prefix) && auth[:len(prefix)] == prefix {
		return auth[len(prefix):]
	}
	return ""
}
