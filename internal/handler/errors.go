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
	case errcode.ErrConcurrentModification.Code:
		response.Error(c, 409, biz)
	case errcode.ErrEmployeeNoAlreadyExists.Code, errcode.ErrDomainAccountAlreadyExists.Code, errcode.ErrUserAlreadyExists.Code,
		errcode.ErrOrgAlreadyExists.Code, errcode.ErrRoleAlreadyExists.Code, errcode.ErrMenuAlreadyExists.Code, errcode.ErrDuplicatePrimaryOrg.Code, errcode.ErrConflict.Code:
		response.Error(c, 409, biz)
	case errcode.ErrCannotResetHigher.Code, errcode.ErrCannotAssignHigherRole.Code, errcode.ErrCannotManageHigher.Code, errcode.ErrUserIsSystem.Code, errcode.ErrCannotRemoveLastSuperadmin.Code, errcode.ErrUserDisabled.Code,
		errcode.ErrOrgHasChildren.Code, errcode.ErrOrgHasMembers.Code, errcode.ErrOrgIsSystem.Code,
		errcode.ErrRoleInUse.Code, errcode.ErrRoleIsSystem.Code,
		errcode.ErrMenuHasChildren.Code, errcode.ErrMenuIsSystem.Code:
		response.ForbiddenError(c, biz)
	case errcode.ErrOrgCannotMoveToChild.Code:
		response.Error(c, 400, biz)
	case errcode.ErrUnauthorized.Code, errcode.ErrTokenInvalid.Code:
		response.UnauthorizedError(c, biz)
	case errcode.ErrNoPermission.Code, errcode.ErrForbidden.Code, errcode.ErrNoRoles.Code:
		response.ForbiddenError(c, biz)
	case errcode.ErrRoleNotFound.Code, errcode.ErrMenuNotFound.Code, errcode.ErrUserNotFound.Code, errcode.ErrOrgNotFound.Code:
		response.Error(c, 404, biz)
	case errcode.ErrNotOrgMember.Code:
		response.Error(c, 404, biz)
	default:
		response.InternalError(c, biz.Message)
	}
}
