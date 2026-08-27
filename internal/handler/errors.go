package handler

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
)

// httpStatusByCode 业务错误码 → HTTP 状态码映射表（D2-06/07/D2-32 重构：
// 原 switch 分支列举，新增错误码漏登记时静默落 default → 500+10000 断链）。
// 表驱动后全码表测试（handler 全码表驱动测试）可逐码断言，漏映射在测试期即失败。
// 未登记的码维持 default：500 + 10000（auth 模块 20001+ 由 auth handler 直写，
// 10007/10008 由中间件直写，10000/70002 本义即 500）。
var httpStatusByCode = map[int]int{
	// 400 参数/业务规则
	errcode.ErrInvalidParams.Code:        400,
	errcode.ErrOrgCannotMoveToChild.Code: 400,

	// 401 未认证
	errcode.ErrUnauthorized.Code: 401,
	errcode.ErrTokenInvalid.Code: 401,

	// 403 禁止（目标校验/系统保护/权限）
	errcode.ErrUserDisabled.Code:               403,
	errcode.ErrUserIsSystem.Code:               403,
	errcode.ErrCannotResetHigher.Code:          403,
	errcode.ErrCannotRemoveLastSuperadmin.Code: 403,
	errcode.ErrCannotAssignHigherRole.Code:     403,
	errcode.ErrCannotManageHigher.Code:         403,
	errcode.ErrOrgHasChildren.Code:             403,
	errcode.ErrOrgHasMembers.Code:              403,
	errcode.ErrOrgIsSystem.Code:                403,
	// D2-06：50012 原漏映射 → 500+10000，errcode.md 承诺 403+50012
	errcode.ErrOrgSystemProtected.Code: 403,
	errcode.ErrRoleInUse.Code:          403,
	errcode.ErrRoleIsSystem.Code:       403,
	errcode.ErrMenuHasChildren.Code:    403,
	errcode.ErrMenuIsSystem.Code:       403,
	errcode.ErrForbidden.Code:          403,
	errcode.ErrNoPermission.Code:       403,
	errcode.ErrNoRoles.Code:            403,

	// 404 不存在
	errcode.ErrNotFound.Code:          404,
	errcode.ErrUserNotFound.Code:       404,
	errcode.ErrRoleNotFound.Code:       404,
	errcode.ErrOrgNotFound.Code:        404,
	errcode.ErrMenuNotFound.Code:       404,
	errcode.ErrNotOrgMember.Code:       404,
	errcode.ErrTicketNotFound.Code:     404,
	errcode.ErrTicketTypeNotFound.Code: 404,

	// 409 冲突
	errcode.ErrConflict.Code:                   409,
	errcode.ErrConcurrentModification.Code:     409,
	errcode.ErrUserAlreadyExists.Code:          409,
	errcode.ErrEmployeeNoAlreadyExists.Code:    409,
	errcode.ErrDomainAccountAlreadyExists.Code: 409,
	errcode.ErrRoleAlreadyExists.Code:          409,
	errcode.ErrOrgAlreadyExists.Code:           409,
	errcode.ErrMenuAlreadyExists.Code:          409,
	errcode.ErrDuplicatePrimaryOrg.Code:        409,
	errcode.ErrTicketAlreadyClosed.Code:        409,

	// 400 业务规则
	errcode.ErrTicketInvalidTransition.Code: 400,

	// 500 已提交但策略刷新失败（D2-07：原漏映射 → 500+10000，承诺 500+70004）
	errcode.ErrPolicyReloadFailed.Code: 500,
}

func writeServiceError(c *gin.Context, err error) {
	var biz *errcode.Error
	if !errors.As(err, &biz) {
		response.InternalError(c, errcode.ErrInternal.Message)
		return
	}
	if status, ok := httpStatusByCode[biz.Code]; ok {
		response.Error(c, status, biz)
		return
	}
	response.InternalError(c, biz.Message)
}
