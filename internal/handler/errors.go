package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// httpStatusByCode 业务错误码 → HTTP 状态码映射表（D2-06/07/D2-32 重构：
// 原 switch 分支列举，新增错误码漏登记时静默落 default → 500+10000 断链）。
// 表驱动后全码表测试（handler 全码表驱动测试）可逐码断言，漏映射在测试期即失败。
// 未登记的码维持 default：500 + 10000（auth 模块 20001+ 由 auth handler 直写，
// 10007/10008 由中间件直写，10000/70002 本义即 500）。
var httpStatusByCode = map[int]int{
	// P4 新段（审计修复 2026-09-30：原漏登记——全部断链 500+10000；2026-10-15 起键值
	// 改引 errcode 常量——§3-11 码值引用纪律，与表内其余行同风格）
	errcode.ErrNotificationConfigNotFound.Code:    http.StatusNotFound,            // 通知配置不存在
	errcode.ErrNotificationCodeExists.Code:        http.StatusConflict,            // 通知配置 code 已存在
	errcode.ErrNotificationChannelNotSupport.Code: http.StatusBadRequest,          // 不支持的通知渠道
	errcode.ErrNotificationNotConfigured.Code:     http.StatusInternalServerError, // 92004 渠道未配置 fail-fast（台账口径 500）
	errcode.ErrDictTypeNotFound.Code:              http.StatusNotFound,            // 字典类型不存在/版本冲突
	errcode.ErrDictTypeInUse.Code:                 http.StatusBadRequest,          // 92304
	// S6 B6-3 消息模板段（92500-92599——外审 F-1-4 修复）
	errcode.ErrMsgTplNotFound.Code:         http.StatusNotFound,   // 92501
	errcode.ErrMsgTplDupCode.Code:          http.StatusConflict,   // 92502
	errcode.ErrMsgTplSystem.Code:           http.StatusForbidden,  // 92503
	errcode.ErrMsgTplReferenced.Code:       http.StatusConflict,   // 92504
	errcode.ErrMsgTplVersionConflict.Code:  http.StatusConflict,   // 92505 模板 version CAS 冲突
	errcode.ErrMsgRuleVersionConflict.Code: http.StatusConflict,   // 92506 订阅规则 version CAS 冲突（000060）
	errcode.ErrDictItemNotFound.Code:       http.StatusNotFound,   // 字典项不存在/版本冲突
	errcode.ErrPatNotFound.Code:            http.StatusNotFound,   // PAT 不存在或已吊销
	errcode.ErrFileTooLarge.Code:           http.StatusBadRequest, // 文件大小超限（phase5 S5）
	errcode.ErrFileTypeNotAllowed.Code:     http.StatusBadRequest, // 文件类型不允许
	errcode.ErrFileNotFound.Code:           http.StatusNotFound,   // 文件不存在（含防枚举归属遮蔽）
	errcode.ErrFileAlreadyBound.Code:       http.StatusConflict,   // 已确认/已关联
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
	// 50004/50005 契约为 409（errcode.md/D2-06·07）——原误映射 403，PRD D6 验收暴露
	errcode.ErrOrgHasChildren.Code: 409,
	errcode.ErrOrgHasMembers.Code:  409,
	// BK-20：50013 与 50004/50005 同族（删组织守卫）→ 409
	errcode.ErrOrgHasOpenTickets.Code: 409,
	errcode.ErrOrgIsSystem.Code:       403,
	// D2-06：50012 原漏映射 → 500+10000，errcode.md 承诺 403+50012
	errcode.ErrOrgSystemProtected.Code: 403,
	errcode.ErrRoleInUse.Code:          403,
	errcode.ErrRoleIsSystem.Code:       403,
	errcode.ErrForbidden.Code:          403,
	errcode.ErrNoPermission.Code:       403,
	errcode.ErrNoRoles.Code:            403,

	// 404 不存在
	errcode.ErrNotFound.Code:     404,
	errcode.ErrUserNotFound.Code: 404,
	errcode.ErrRoleNotFound.Code: 404,
	errcode.ErrOrgNotFound.Code:  404,
	errcode.ErrMenuNotFound.Code: 404,
	errcode.ErrNotOrgMember.Code: 404,

	// 2c 组织委托（04-org-delegation §5）
	errcode.ErrCannotAssignHigherOrgMemberRole.Code: 403,
	errcode.ErrCannotManageOrgMember.Code:           403,
	errcode.ErrNotOrgOwner.Code:                     403,
	errcode.ErrTicketNotFound.Code:                  404,
	errcode.ErrTicketTypeNotFound.Code:              404,

	// 409 冲突
	errcode.ErrConflict.Code:                   409,
	errcode.ErrConcurrentModification.Code:     409,
	errcode.ErrUserAlreadyExists.Code:          409,
	errcode.ErrEmployeeNoAlreadyExists.Code:    409,
	errcode.ErrDomainAccountAlreadyExists.Code: 409,
	errcode.ErrRoleAlreadyExists.Code:          409,
	errcode.ErrOrgAlreadyExists.Code:           409,
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
		// 27 批 D-5：PG 22001（超长）兜底 400——model max= 为主防线（D-3），
		// 此处防新字段漏绑时客户端输入错误计 5xx 污染告警
		if ec := repository.MapStringValueOverflow(err); ec != nil {
			response.BadRequest(c, ec.Message)
			return
		}
		// 可观测性（2026-10-12，首例=首页统计卡 GET /tickets 瞬时 500 无迹可查）：
		// 未知错误此前只回通用文案不落日志，500 事后无法按 request_id 溯源。
		// panic 不走此函数（gin recovery → stderr），另行排查。
		slog.Error("unclassified service error",
			"request_id", c.GetString("request_id"),
			"path", c.FullPath(), "err", err)
		response.InternalError(c, errcode.ErrInternal.Message)
		return
	}
	if status, ok := httpStatusByCode[biz.Code]; ok {
		response.Error(c, status, biz)
		return
	}
	// 业务码存在但未登记 HTTP 映射——同样落 ERROR（表驱动漏登记在测试期应被全码表
	// 测试拦截，运行期出现即映射表与码表脱节，需暴露）
	slog.Error("service error without http mapping",
		"request_id", c.GetString("request_id"),
		"path", c.FullPath(), "code", biz.Code, "err", err)
	response.InternalError(c, biz.Message)
}
