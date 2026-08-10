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
	ErrInternal      = New(10000, "服务器内部错误")
	ErrInvalidParams = New(10001, "参数错误")
	ErrUnauthorized  = New(10002, "未授权")
	ErrForbidden     = New(10003, "禁止访问")
	ErrNotFound      = New(10004, "资源不存在")
	ErrConflict      = New(10005, "资源冲突")
	ErrTooManyReqs   = New(10007, "请求过于频繁")
)

// 认证模块 20000-20999
var (
	ErrInvalidCredentials = New(20001, "用户名或密码错误")
	ErrTokenExpired       = New(20002, "token 已过期")
	ErrTokenInvalid       = New(20003, "token 已失效")
	ErrRefreshTokenInvalid = New(20004, "刷新令牌无效")
	ErrTokenAlreadyRefreshed = New(20005, "令牌已被刷新")
	ErrAccountLocked      = New(20006, "账号已锁定")
)

// 用户模块 30000-30999
var (
	ErrUserAlreadyExists = New(30001, "用户已存在")
	ErrUserNotFound      = New(30002, "用户不存在")
	ErrUserDisabled      = New(30003, "用户已禁用")
)

// 角色模块 40000-40999
var (
	ErrRoleAlreadyExists = New(40001, "角色已存在")
	ErrRoleNotFound      = New(40002, "角色不存在")
)

// 组织模块 50000-50999
var (
	ErrOrgAlreadyExists   = New(50001, "组织已存在")
	ErrOrgNotFound        = New(50002, "组织不存在")
	ErrOrgCannotMoveToChild = New(50003, "不能移动到子节点下")
)

// 菜单模块 60000-60999
var (
	ErrMenuAlreadyExists = New(60001, "菜单已存在")
	ErrMenuNotFound      = New(60002, "菜单不存在")
)

// 权限模块 70000-70999
var (
	ErrNoPermission   = New(70001, "无权限")
	ErrPolicyExists   = New(70002, "策略已存在")
)
