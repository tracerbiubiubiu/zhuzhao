# docs/api/errcode.md — 业务错误码对外契约登记

> 定义权威 = `internal/pkg/errcode/errcode.go`（通用段转发正式 utils 依赖）；HTTP 权威 = `internal/handler/errors.go`、`auth_handler.go` 及认证/限流中间件。本表覆盖当前77个已定义码，默认文案逐字对应 Error.Message；业务参数校验可在现有码下给出具体提示。
> 测试使用 Go AST 对账真实定义与运行时清单，再双向核对登记、默认文案和实际响应HTTP。本文件为唯一登记；根api/旧文件及原指针已归并，AST对账测试读取本路径。

## 1. 主仓当前定义

HTTP列为所属出口现行行为。20005、70002仅保留定义，无生产发出点，其默认降级行为在备注明确；登记不新增线上出口。

### 10000–10999 通用

| code | 常量 | 默认文案 | HTTP | 备注 |
|---|---|---|---|---|
| 10000 | ErrInternal | 服务器内部错误 | 500 |  |
| 10001 | ErrInvalidParams | 参数错误 | 400 |  |
| 10002 | ErrUnauthorized | 未授权 | 401 |  |
| 10003 | ErrForbidden | 禁止访问 | 403 |  |
| 10004 | ErrNotFound | 资源不存在 | 404 |  |
| 10005 | ErrConflict | 资源冲突 | 409 |  |
| 10006 | ErrConcurrentModification | 数据已被修改，请刷新后重试 | 409 |  |
| 10007 | ErrTooManyReqs | 请求过于频繁 | 429 | 限流/锁定直写429。 |
| 10008 | ErrServiceUnavailable | 服务暂时不可用 | 503 | 依赖故障直写503。 |

### 20000–20999 认证

| code | 常量 | 默认文案 | HTTP | 备注 |
|---|---|---|---|---|
| 20001 | ErrInvalidCredentials | 工号或密码错误 | 401 |  |
| 20002 | ErrTokenExpired | token 已过期 | 401 | JWT过期直写401。 |
| 20003 | ErrTokenInvalid | 登录状态已失效，请重新登录 | 401 |  |
| 20004 | ErrRefreshTokenInvalid | 登录已过期，请重新登录 | 401 |  |
| 20005 | ErrTokenAlreadyRefreshed | 令牌已被刷新 | 500 | 仅保留定义，无生产出口；默认渲染降为500+10000，不新增20005出口。 |
| 20006 | ErrAccountLocked | 账号已锁定：连续登录失败次数过多，请稍后再试或联系管理员重置 | 429 | 认证锁定直写429。 |
| 20007 | ErrPasswordChangeRequired | 需要修改密码 | 403 | JWT强制改密直写403。 |
| 20008 | ErrMultipleAuthMethods | 不能同时使用多种认证方式 | 400 | JWT混用凭证直写400。 |
| 20014 | ErrPasswordChanged | 密码已修改，请重新登录 | 401 |  |
| 20015 | ErrRefreshTokenReplayed | 登录态异常：刷新令牌已被使用，请重新登录 | 401 |  |

### 21000–21999 任务动作

| code | 常量 | 默认文案 | HTTP | 备注 |
|---|---|---|---|---|
| 21010 | ErrActionNotRegistered | 动作未注册 | 400 | 提交 fail-fast（不再回调期 404）；合法动作见 GET /jobs/actions |

### 30000–30999 用户

| code | 常量 | 默认文案 | HTTP | 备注 |
|---|---|---|---|---|
| 30001 | ErrUserAlreadyExists | 用户已存在 | 409 |  |
| 30002 | ErrUserNotFound | 用户不存在 | 404 |  |
| 30003 | ErrUserDisabled | 用户已禁用 | 403 |  |
| 30004 | ErrUserIsSystem | 系统内置用户不可删除 | 403 |  |
| 30005 | ErrCannotResetHigher | 不能重置同级或更高级用户的密码 | 403 |  |
| 30006 | ErrCannotRemoveLastSuperadmin | 不能移除最后一个超级管理员 | 403 |  |
| 30007 | ErrEmployeeNoAlreadyExists | 工号已存在（含软删占用，不可复用） | 409 |  |
| 30008 | ErrDomainAccountAlreadyExists | 同域下域账号已存在（含软删占用，不可复用） | 409 |  |
| 30009 | ErrCannotAssignHigherRole | 不能分配更高权限的角色 | 403 |  |
| 30010 | ErrCannotManageHigher | 不能操作同级或更高级权限对象 | 403 |  |

### 40000–40999 角色

| code | 常量 | 默认文案 | HTTP | 备注 |
|---|---|---|---|---|
| 40001 | ErrRoleAlreadyExists | 角色已存在 | 409 |  |
| 40002 | ErrRoleNotFound | 角色不存在 | 404 |  |
| 40003 | ErrRoleInUse | 该角色仍有用户关联，无法删除 | 403 | 现行HTTP为403；旧设计稿409不作为改动生产行为的依据。 |
| 40004 | ErrRoleIsSystem | 系统内置角色不可删除 | 403 |  |

### 50000–50999 组织

| code | 常量 | 默认文案 | HTTP | 备注 |
|---|---|---|---|---|
| 50001 | ErrOrgAlreadyExists | 组织已存在 | 409 |  |
| 50002 | ErrOrgNotFound | 组织不存在 | 404 |  |
| 50003 | ErrOrgCannotMoveToChild | 不能移动到子节点下 | 400 |  |
| 50004 | ErrOrgHasChildren | 该组织下有子组织，无法删除 | 409 |  |
| 50005 | ErrOrgHasMembers | 该组织下有成员，无法删除 | 409 |  |
| 50006 | ErrOrgIsSystem | 系统内置组织不可删除 | 403 |  |
| 50007 | ErrNotOrgMember | 用户不是该组织成员 | 404 |  |
| 50008 | ErrCannotAssignHigherOrgMemberRole | 不能分配更高的组内角色 | 403 |  |
| 50009 | ErrCannotManageOrgMember | 无权管理该组织成员 | 403 |  |
| 50010 | ErrNotOrgOwner | 需要组织负责人权限 | 403 |  |
| 50011 | ErrDuplicatePrimaryOrg | 该用户已有主组织，并发设置主组织冲突，请重试 | 409 |  |
| 50012 | ErrOrgSystemProtected | 系统内置组织受保护，禁止此操作 | 403 |  |
| 50013 | ErrOrgHasOpenTickets | 该组织下有未结工单，无法删除 | 409 |  |

### 60000–60999 菜单

| code | 常量 | 默认文案 | HTTP | 备注 |
|---|---|---|---|---|
| 60002 | ErrMenuNotFound | 菜单不存在 | 404 |  |

### 70000–70999 权限

| code | 常量 | 默认文案 | HTTP | 备注 |
|---|---|---|---|---|
| 70001 | ErrNoPermission | 无权限 | 403 |  |
| 70002 | ErrPolicyExists | 策略已存在 | 500 | 仅保留定义，无生产出口；默认渲染降为500+10000。 |
| 70003 | ErrNoRoles | 未分配角色 | 403 |  |
| 70004 | ErrPolicyReloadFailed | 策略已保存但内存刷新失败，权限可能延迟生效，请稍后重试或联系运维 | 500 |  |

### 90000–90999 工单

| code | 常量 | 默认文案 | HTTP | 备注 |
|---|---|---|---|---|
| 90001 | ErrTicketNotFound | 工单不存在 | 404 |  |
| 90002 | ErrTicketInvalidTransition | 非法状态转换 | 400 |  |
| 90003 | ErrTicketTypeNotFound | 工单类型不存在 | 404 |  |
| 90004 | ErrTicketAlreadyClosed | 工单已关闭 | 409 |  |

### 91000–91999 对象存储

| code | 常量 | 默认文案 | HTTP | 备注 |
|---|---|---|---|---|
| 91001 | ErrFileTooLarge | 文件大小超过限制 | 400 | presign 声明值预检 + confirm HEAD 实测双卡点 |
| 91002 | ErrFileTypeNotAllowed | 文件类型不允许 | 400 | 显式枚举白名单（支持尾部 `.*` 通配）+ html/svg 黑名单硬底线 |
| 91003 | ErrFileNotFound | 文件不存在 | 404 | 含防枚举归属遮蔽（非本人对象同形 404） |
| 91004 | ErrFileAlreadyBound | 文件已关联其它资源 | 409 | 重复 confirm / 已绑定（B5-3 关联表语义） |

### 92000–92299 通知

| code | 常量 | 默认文案 | HTTP | 备注 |
|---|---|---|---|---|
| 92001 | ErrNotificationConfigNotFound | 通知配置不存在 | 404 | P4-2 notification_configs |
| 92002 | ErrNotificationCodeExists | 通知配置 code 已存在 | 409 |  |
| 92003 | ErrNotificationChannelNotSupport | 不支持的通知渠道 | 400 |  |
| 92004 | ErrNotificationNotConfigured | 通知渠道未配置（生产态 fail-fast） | 500 | 06 号 §3 两态桩防线——S6 B6-2；607a2a9 起 port.go 真发射 |

### 92300–92399 字典

| code | 常量 | 默认文案 | HTTP | 备注 |
|---|---|---|---|---|
| 92301 | ErrDictTypeNotFound | 字典类型不存在或版本冲突 | 404 | 含版本冲突（乐观锁）同形 |
| 92302 | ErrDictItemNotFound | 字典项不存在或版本冲突 | 404 | 同上 |
| 92304 | ErrDictTypeInUse | 字典类型被字段引用中，不可删除 | 400 | B1-2 删除守卫：ticket_type_fields 引用 `dict:<code>` 时拒删；清引用后可删 |

### 92400–92499 PAT

| code | 常量 | 默认文案 | HTTP | 备注 |
|---|---|---|---|---|
| 92401 | ErrPatNotFound | PAT 不存在或已吊销 | 404 |  |

### 92500–92599 消息模板与规则

| code | 常量 | 默认文案 | HTTP | 备注 |
|---|---|---|---|---|
| 92501 | ErrMsgTplNotFound | 消息模板不存在 | 404 |  |
| 92502 | ErrMsgTplDupCode | 模板 code 已存在 | 409 |  |
| 92503 | ErrMsgTplSystem | 系统模板不可修改或删除 | 403 | 种子模板只读（is_system=true） |
| 92504 | ErrMsgTplReferenced | 模板被订阅规则引用中，不可删除 | 409 | 须先解引 |
| 92505 | ErrMsgTplVersionConflict | 模板版本冲突——请刷新后重试 | 409 | CAS 乐观锁 |
| 92506 | ErrMsgRuleVersionConflict | 订阅规则版本冲突——请刷新后重试 | 409 | CAS 乐观锁（000060——微批 A） |

### 92600–92699 威胁情报

| code | 常量 | 默认文案 | HTTP | 备注 |
|---|---|---|---|---|
| 92600 | ErrIntelNotConfigured | 情报上游未配置（intel.upstream.endpoint） | 500 | 生产态空 endpoint fail-fast |
| 92601 | ErrIntelUpstreamQuota | 情报上游配额耗尽（无可用缓存） | 503 | 降级矩阵无缓存臂 |
| 92602 | ErrIntelUpstream | 情报上游不可用 | 502 | 401/5xx 归并 |

92303保留未分配，不是已定义码，不进入有效码表；已退役菜单写码等未定义号位亦不恢复。

## 2. 兄弟服务契约边界

- **activelist** 的九个有效业务码为100000–100008，依次为 `VALIDATION_ERROR`、`RESERVED_FIELD`、`TYPE_NOT_FOUND`、`TYPE_ALREADY_EXISTS`、`TYPE_DEPRECATED`、`DATA_NOT_FOUND`、`FIELD_DEPRECATED`、`NEW_REQUIRED_FIELD`、`CONFLICT`。HTTP由各入口的apperr.Error.HTTP及handler.Fail决定，不能假设一码唯一HTTP：VALIDATION_ERROR现有400/413/422三种情形。跨仓定义权威 = [apperr映射与错误类型](https://github.com/tracerbiubiubiu/activelist/blob/19a4c1d5cd84cc892b82ad52554db320bfad8f7f/internal/apperr/apperr.go)，渲染权威 = [handler.Fail](https://github.com/tracerbiubiubiu/activelist/blob/19a4c1d5cd84cc892b82ad52554db320bfad8f7f/internal/handler/response.go)（本次核验固定点）。内部/依赖故障复用10000/10008，5xx不公开数据库诊断或Detail。升级时须对照实际受审版本更新指针，本仓单测不冒充跨仓自动契约测试。
- **taskrunner** 的101000–101999段仍未启用；当前NotFound/Invalid/Conflict/其他错误分别为404+10004、400+10001、409+10005、500+10000。跨仓HTTP权威 = [handler.Deps.fail](https://github.com/tracerbiubiubiu/taskrunner/blob/5e3f5f3720a54fad9f5cec4a08481364693b8f7d/internal/handler/server.go)（本次核验固定点）；AK/SK失败复用通用认证码，不虚构101xxx线上码。
- 通用10000段权威 = [utils errcode](https://github.com/tracerbiubiubiu/zhuzhao-utils/blob/9296ae170b71b06c623c54b6010a107406903346/errcode/errcode.go)。消费者按各自go.mod的不可变版本复核，上表采用主仓正式依赖的实际值。

## 3. 变更记录

| 日期 | 内容 |
|---|---|
| 2026-11-06 | B12补齐归一时遗漏的当前定义，明确兼容码与跨仓权威边界；补定义、登记、文案、HTTP完整性守卫，生产码与HTTP行为不变。 |
