package errcode

// 本包保留 zhuzhao 业务错误码（20000 起）。框架与通用码（10000-10999）
// 已抽到 zhuzhao-utils/errcode，此处转发引用，类型经别名保持同一。
import util "github.com/tracerbiubiubiu/zhuzhao-utils/errcode"

// Error 业务错误（= zhuzhao-utils/errcode.Error）
type Error = util.Error

// New 创建业务错误
var New = util.New

// 通用错误码 10000-10999（实现在 zhuzhao-utils/errcode）
var (
	ErrInternal               = util.ErrInternal
	ErrInvalidParams          = util.ErrInvalidParams
	ErrUnauthorized           = util.ErrUnauthorized
	ErrForbidden              = util.ErrForbidden
	ErrNotFound               = util.ErrNotFound
	ErrConflict               = util.ErrConflict
	ErrConcurrentModification = util.ErrConcurrentModification
	ErrTooManyReqs            = util.ErrTooManyReqs
	ErrServiceUnavailable     = util.ErrServiceUnavailable
)

// 认证模块 20000-20999
var (
	ErrInvalidCredentials     = util.New(20001, "工号或密码错误")
	ErrTokenExpired           = util.New(20002, "token 已过期")
	ErrTokenInvalid           = util.New(20003, "token 已失效")
	ErrRefreshTokenInvalid    = util.New(20004, "刷新令牌无效")
	ErrTokenAlreadyRefreshed  = util.New(20005, "令牌已被刷新")
	ErrAccountLocked          = util.New(20006, "账号已锁定")
	ErrPasswordChangeRequired = util.New(20007, "需要修改密码")
	ErrMultipleAuthMethods    = util.New(20008, "不能同时使用多种认证方式")
)

// 用户模块 30000-30999
var (
	ErrUserAlreadyExists          = util.New(30001, "用户已存在")
	ErrUserNotFound               = util.New(30002, "用户不存在")
	ErrUserDisabled               = util.New(30003, "用户已禁用")
	ErrUserIsSystem               = util.New(30004, "系统内置用户不可删除")
	ErrCannotResetHigher          = util.New(30005, "不能重置同级或更高级用户的密码")
	ErrCannotRemoveLastSuperadmin = util.New(30006, "不能移除最后一个超级管理员")
	// 文案与 errcode.md SSOT 逐条一致：「含软删占用，不可复用」是 000006
	// 部分唯一索引语义的用户可感知说明（撞到软删工号时明确为何不可复用）
	ErrEmployeeNoAlreadyExists    = util.New(30007, "工号已存在（含软删占用，不可复用）")
	ErrDomainAccountAlreadyExists = util.New(30008, "同域下域账号已存在（含软删占用，不可复用）")
	ErrCannotAssignHigherRole     = util.New(30009, "不能分配更高权限的角色")
	ErrCannotManageHigher         = util.New(30010, "不能操作同级或更高级权限对象")
)

// 角色模块 40000-40999
var (
	ErrRoleAlreadyExists = util.New(40001, "角色已存在")
	ErrRoleNotFound      = util.New(40002, "角色不存在")
	ErrRoleInUse         = util.New(40003, "该角色仍有用户关联，无法删除")
	ErrRoleIsSystem      = util.New(40004, "系统内置角色不可删除")
)

// 组织模块 50000-50999
var (
	ErrOrgAlreadyExists     = util.New(50001, "组织已存在")
	ErrOrgNotFound          = util.New(50002, "组织不存在")
	ErrOrgCannotMoveToChild = util.New(50003, "不能移动到子节点下")
	ErrOrgHasChildren       = util.New(50004, "该组织下有子组织，无法删除")
	ErrOrgHasMembers        = util.New(50005, "该组织下有成员，无法删除")
	ErrOrgHasOpenTickets    = util.New(50013, "该组织下有未结工单，无法删除")
	ErrOrgIsSystem          = util.New(50006, "系统内置组织不可删除")
	ErrNotOrgMember         = util.New(50007, "用户不是该组织成员")

	// 2c 组织委托（04-org-delegation §5）
	ErrCannotAssignHigherOrgMemberRole = util.New(50008, "不能分配更高的组内角色")
	ErrCannotManageOrgMember           = util.New(50009, "无权管理该组织成员")
	ErrNotOrgOwner                     = util.New(50010, "需要组织负责人权限")
	ErrDuplicatePrimaryOrg             = util.New(50011, "该用户已有主组织，并发设置主组织冲突，请重试")
	ErrOrgSystemProtected              = util.New(50012, "系统内置组织受保护，禁止此操作")
)

// 菜单模块 60000-60999
var (
	ErrMenuAlreadyExists = util.New(60001, "菜单已存在")
	ErrMenuNotFound      = util.New(60002, "菜单不存在")
	ErrMenuHasChildren   = util.New(60003, "该菜单下有子菜单，无法删除")
	ErrMenuIsSystem      = util.New(60004, "系统内置菜单不可删除")
)

// 权限模块 70000-70999
var (
	ErrNoPermission       = util.New(70001, "无权限")
	ErrPolicyExists       = util.New(70002, "策略已存在")
	ErrNoRoles            = util.New(70003, "未分配角色")
	ErrPolicyReloadFailed = util.New(70004, "策略已保存但内存刷新失败，权限可能延迟生效，请稍后重试或联系运维")
)

// 工单模块 90000-90999
var (
	ErrTicketNotFound          = util.New(90001, "工单不存在")
	ErrTicketInvalidTransition = util.New(90002, "非法状态转换")
	ErrTicketTypeNotFound      = util.New(90003, "工单类型不存在")
	ErrTicketAlreadyClosed     = util.New(90004, "工单已关闭")
)
