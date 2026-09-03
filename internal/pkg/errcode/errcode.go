package errcode

// Error 业务错误，包含错误码和消息
type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

// New 创建业务错误
func New(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

// 通用错误码 10000-10999
var (
	ErrInternal               = New(10000, "服务器内部错误")
	ErrInvalidParams          = New(10001, "参数错误")
	ErrUnauthorized           = New(10002, "未授权")
	ErrForbidden              = New(10003, "禁止访问")
	ErrNotFound               = New(10004, "资源不存在")
	ErrConflict               = New(10005, "资源冲突")
	ErrConcurrentModification = New(10006, "数据已被修改，请刷新后重试")
	ErrTooManyReqs            = New(10007, "请求过于频繁")
	ErrServiceUnavailable     = New(10008, "服务暂时不可用")
)

// 认证模块 20000-20999
var (
	ErrInvalidCredentials     = New(20001, "工号或密码错误")
	ErrTokenExpired           = New(20002, "token 已过期")
	ErrTokenInvalid           = New(20003, "token 已失效")
	ErrRefreshTokenInvalid    = New(20004, "刷新令牌无效")
	ErrTokenAlreadyRefreshed  = New(20005, "令牌已被刷新")
	ErrAccountLocked          = New(20006, "账号已锁定")
	ErrPasswordChangeRequired = New(20007, "需要修改密码")
	ErrMultipleAuthMethods    = New(20008, "不能同时使用多种认证方式")
)

// 用户模块 30000-30999
var (
	ErrUserAlreadyExists          = New(30001, "用户已存在")
	ErrUserNotFound               = New(30002, "用户不存在")
	ErrUserDisabled               = New(30003, "用户已禁用")
	ErrUserIsSystem               = New(30004, "系统内置用户不可删除")
	ErrCannotResetHigher          = New(30005, "不能重置同级或更高级用户的密码")
	ErrCannotRemoveLastSuperadmin = New(30006, "不能移除最后一个超级管理员")
	// 文案与 errcode.md SSOT 逐条一致：「含软删占用，不可复用」是 000006
	// 部分唯一索引语义的用户可感知说明（撞到软删工号时明确为何不可复用）
	ErrEmployeeNoAlreadyExists    = New(30007, "工号已存在（含软删占用，不可复用）")
	ErrDomainAccountAlreadyExists = New(30008, "同域下域账号已存在（含软删占用，不可复用）")
	ErrCannotAssignHigherRole     = New(30009, "不能分配更高权限的角色")
	ErrCannotManageHigher         = New(30010, "不能操作同级或更高级权限对象")
)

// 角色模块 40000-40999
var (
	ErrRoleAlreadyExists = New(40001, "角色已存在")
	ErrRoleNotFound      = New(40002, "角色不存在")
	ErrRoleInUse         = New(40003, "该角色仍有用户关联，无法删除")
	ErrRoleIsSystem      = New(40004, "系统内置角色不可删除")
)

// 组织模块 50000-50999
var (
	ErrOrgAlreadyExists     = New(50001, "组织已存在")
	ErrOrgNotFound          = New(50002, "组织不存在")
	ErrOrgCannotMoveToChild = New(50003, "不能移动到子节点下")
	ErrOrgHasChildren       = New(50004, "该组织下有子组织，无法删除")
	ErrOrgHasMembers        = New(50005, "该组织下有成员，无法删除")
	ErrOrgHasOpenTickets    = New(50013, "该组织下有未结工单，无法删除")
	ErrOrgIsSystem          = New(50006, "系统内置组织不可删除")
	ErrNotOrgMember         = New(50007, "用户不是该组织成员")

	// 2c 组织委托（04-org-delegation §5）
	ErrCannotAssignHigherOrgMemberRole = New(50008, "不能分配更高的组内角色")
	ErrCannotManageOrgMember           = New(50009, "无权管理该组织成员")
	ErrNotOrgOwner                     = New(50010, "需要组织负责人权限")
	ErrDuplicatePrimaryOrg             = New(50011, "该用户已有主组织，并发设置主组织冲突，请重试")
	ErrOrgSystemProtected              = New(50012, "系统内置组织受保护，禁止此操作")
)

// 菜单模块 60000-60999
var (
	ErrMenuAlreadyExists = New(60001, "菜单已存在")
	ErrMenuNotFound      = New(60002, "菜单不存在")
	ErrMenuHasChildren   = New(60003, "该菜单下有子菜单，无法删除")
	ErrMenuIsSystem      = New(60004, "系统内置菜单不可删除")
)

// 权限模块 70000-70999
var (
	ErrNoPermission       = New(70001, "无权限")
	ErrPolicyExists       = New(70002, "策略已存在")
	ErrNoRoles            = New(70003, "未分配角色")
	ErrPolicyReloadFailed = New(70004, "策略已保存但内存刷新失败，权限可能延迟生效，请稍后重试或联系运维")
)

// 工单模块 90000-90999
var (
	ErrTicketNotFound          = New(90001, "工单不存在")
	ErrTicketInvalidTransition = New(90002, "非法状态转换")
	ErrTicketTypeNotFound      = New(90003, "工单类型不存在")
	ErrTicketAlreadyClosed     = New(90004, "工单已关闭")
)
