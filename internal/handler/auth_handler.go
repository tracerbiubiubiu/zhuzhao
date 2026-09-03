package handler

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
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
//
//	@Summary	用户登录
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		request	body		model.LoginRequest	true	"登录请求"
//	@Success	200		{object}	response.Response	"登录成功，返回 Token 对"
//	@Router		/api/v1/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcode.ErrInvalidParams.Message)
		return
	}

	pair, err := h.authService.Login(c.Request.Context(), &req, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		writeAuthError(c, err)
		return
	}
	response.OK(c, pair)
}

// Refresh 刷新 Token
//
//	@Summary	刷新 Token
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		request	body		model.RefreshRequest	true	"刷新请求"
//	@Success	200		{object}	response.Response	"刷新成功"
//	@Router		/api/v1/auth/refresh [post]
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
//
//	@Summary	用户登出
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		request	body		model.LogoutRequest	true	"登出请求"
//	@Success	200		{object}	response.Response	"登出成功"
//	@Router		/api/v1/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	auth := c.GetHeader("Authorization")
	if auth == "" {
		response.Unauthorized(c, errcode.ErrUnauthorized.Message)
		return
	}

	var req model.LogoutRequest
	// B4-1：body 非法 JSON 显式 400（原忽略错误 → DeviceID 缺省删 default 设备的 RT）
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcode.ErrInvalidParams.Message)
		return
	}

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
//
//	@Summary	修改密码
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		request	body		model.UpdatePasswordRequest	true	"修改密码请求"
//	@Success	200		{object}	response.Response			"修改成功"
//	@Router		/api/v1/auth/password/update [post]
func (h *AuthHandler) UpdatePassword(c *gin.Context) {
	var req model.UpdatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcode.ErrInvalidParams.Message)
		return
	}
	accessToken := extractBearer(c.GetHeader("Authorization"))
	pair, err := h.authService.UpdatePassword(c.Request.Context(), c.GetInt64("userID"), req.OldPassword, req.NewPassword, accessToken, req.DeviceID)
	if err != nil {
		writeAuthError(c, err)
		return
	}
	response.OK(c, pair)
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
	case errcode.ErrUserNotFound.Code:
		// B4-1：用户在 AT 签发后被删（可达性低）——404 而非 500
		response.NotFound(c, biz.Message)
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
