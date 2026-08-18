package handler

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
)

func writeServiceError(c *gin.Context, err error) {
	var biz *errcode.Error
	if !errors.As(err, &biz) {
		response.InternalError(c, errcode.ErrInternal.Message)
		return
	}
	switch biz.Code {
	case errcode.ErrInvalidParams.Code:
		response.BadRequest(c, biz.Message)
	case errcode.ErrUnauthorized.Code, errcode.ErrTokenInvalid.Code:
		response.UnauthorizedError(c, biz)
	case errcode.ErrNoPermission.Code, errcode.ErrForbidden.Code, errcode.ErrNoRoles.Code:
		response.ForbiddenError(c, biz)
	case errcode.ErrRoleNotFound.Code, errcode.ErrMenuNotFound.Code, errcode.ErrUserNotFound.Code:
		response.NotFound(c, biz.Message)
	default:
		response.InternalError(c, biz.Message)
	}
}
