# 错误码约定

> 业务错误码（`code`）分段、HTTP 映射与 Phase 1 验收对照。  
> **响应体外层结构**（`code` / `message` / `data` / `request_id`）以 [`response.md`](./response.md) 为 SSOT。  
> 实现：`internal/pkg/errcode/errcode.go` + `internal/pkg/response/response.go`

---

## 1. 与响应体的关系

所有 JSON API 使用统一 Envelope，见 **[response.md](./response.md)**。本文只定义 **非 0 的 `code` 含义** 及对应 HTTP 状态。

成功时：`code = 0`，`message = "success"`，`data` 为业务数据。  
失败时：`data = null`，`message` 取自下表或 errcode 常量。

客户端：**以 body.code 做业务分支**，不要只依赖 HTTP 状态码。

---

## 2. 分段规则

| 区间 | 模块 |
|------|------|
| 0 | 成功 |
| 10000–10999 | 通用 |
| 20000–20999 | 认证 |
| 30000–30999 | 用户 |
| 40000–40999 | 角色 |
| 50000–50999 | 组织 |
| 60000–60999 | 菜单 |
| 70000–70999 | 鉴权 / Casbin |
| 80000–80999 | 审计（Phase 2+） |
| 90000–90999 | 工单 |
| 91000–91999 | 文件存储 |

---

## 3. HTTP 状态码映射

| HTTP | 场景 |
|------|------|
| 200 | 成功 |
| 400 | 参数校验失败（`ErrInvalidParams`） |
| 401 | 未登录、Token 无效/过期/已吊销 |
| 403 | 已登录但无权限、账号禁用、须改密、无角色 |
| 404 | 资源不存在（对外可暴露「不存在」时） |
| 409 | 唯一约束冲突（工号/域账号/编码重复等）、乐观锁并发冲突 |
| 429 | 登录限流 |
| 503 | 鉴权链路 Redis 不可用（fail-close） |

**资源级「不可见」**：Phase 2 工单等场景对无 scope 的单条资源返回 **404**（非 403），防信息泄露。Phase 1 管理接口仍用 403/404 按是否存在资源决定。

---

## 4. 错误码清单

### 通用 10000–10999

| code | 常量 | message | HTTP |
|------|------|---------|------|
| 10000 | `ErrInternal` | 服务器内部错误 | 500 |
| 10001 | `ErrInvalidParams` | 参数错误 | 400 |
| 10002 | `ErrUnauthorized` | 未授权 | 401 |
| 10003 | `ErrForbidden` | 禁止访问 | 403 |
| 10004 | `ErrNotFound` | 资源不存在 | 404 |
| 10005 | `ErrConflict` | 资源冲突 | 409 |
| 10006 | `ErrConcurrentModification` | 数据已被修改，请刷新后重试 | 409 |
| 10007 | `ErrTooManyReqs` | 请求过于频繁 | 429 |
| 10008 | `ErrServiceUnavailable` | 服务暂时不可用 | 503 |

> `10006`：乐观锁冲突（`version` 不匹配），见 [10-concurrency §乐观锁](../phase1/10-concurrency.md#乐观锁建议实现)。已在 `errcode.go` 定义。
> `10008`：Phase 1 用于鉴权链路 Redis 不可用（fail-close）。JWT 中间件返回 HTTP 503 + 此 code。

### 认证 20000–20999

> **非法 / 混用凭证怎么处理**（HTTP、日志、Abort 规则）：见 [phase1/02-auth.md §非法认证请求的处理](../phase1/02-auth.md#非法认证请求的处理实现必读)。

| code | 常量 | message | HTTP |
|------|------|---------|------|
| 20001 | `ErrInvalidCredentials` | 工号或密码错误 | 401 |
| 20002 | `ErrTokenExpired` | token 已过期 | 401 |
| 20003 | `ErrTokenInvalid` | token 已失效 | 401 |
| 20004 | `ErrRefreshTokenInvalid` | 刷新令牌无效 | 401 |
| 20005 | `ErrTokenAlreadyRefreshed` | 令牌已被刷新 | 401 |
| 20006 | `ErrAccountLocked` | 账号已锁定 | 429 |
| 20007 | `ErrPasswordChangeRequired` | 需要修改密码 | 403 |
| 20008 | `ErrMultipleAuthMethods` | 不能同时使用多种认证方式 | 400 |

<!-- D2-44①：以下为未实现预留段（errcode.go 尚未定义，勿据此对接） -->

| 20009 | `ErrAKInvalid` | 访问密钥无效 | 401 |
| 20010 | `ErrAKTimestampExpired` | 请求已过期 | 401 |
| 20011 | `ErrAKReplay` | 重复请求 | 401 |
| 20012 | `ErrDeviceNotFound` | 设备不存在 | 404 |
| 20013 | `ErrPasswordTooWeak` | 密码不符合复杂度要求 | 400 |

> 文档中的 `PASSWORD_CHANGE_REQUIRED` 即 `20007`。**Phase 1 验收必需**：`20008`（Bearer 与 `X-AK-*` 混用，见验收 #22）、`20007`、`30006`、`70003` 等须写入 `errcode.go`；`20009`–`20011` 为 M2M（AK/SK）上线时使用。AK 验签失败对外统一 **20009**，不区分 AK 不存在与 SK 错误（防探测）。`20012`–`20013` 随 Phase **2b** auth-enhance 写入 `errcode.go`。

### 用户 30000–30999

> Phase 1 **`username` 可重复**，创建用户**不**因重复 username 返回 30001；30001 保留给其它冲突（如未来 external_id 唯一约束）。工号冲突用 **30007**。

| code | 常量 | message | HTTP |
|------|------|---------|------|
| 30001 | `ErrUserAlreadyExists` | 用户已存在 | 409 |
| 30002 | `ErrUserNotFound` | 用户不存在 | 404 |
| 30003 | `ErrUserDisabled` | 用户已禁用 | 403 |
| 30004 | `ErrUserIsSystem` | 系统内置用户不可删除 | 403 |
| 30005 | `ErrCannotResetHigher` | 不能重置同级或更高级用户的密码 | 403 |
| 30006 | `ErrCannotRemoveLastSuperadmin` | 不能移除最后一个超级管理员 | 403 |
| 30007 | `ErrEmployeeNoAlreadyExists` | 工号已存在（含软删占用，不可复用） | 409 |
| 30008 | `ErrDomainAccountAlreadyExists` | 同域下域账号已存在（含软删占用，不可复用） | 409 |
| 30009 | `ErrCannotAssignHigherRole` | 不能分配更高权限的角色 | 403 |
| 30010 | `ErrCannotManageHigher` | 不能操作同级或更高级权限对象 | 403 |

> `30006`、`70003`：Phase 1 **验收必需**；`30007`–`30010`、`50007` 随 user/org 模块实现写入 `errcode.go`（码号预留，勿改号）。
> `30010` 为通用防提权码（update/delete/status/roles/orgs 等写路径）；重置密码场景仍返回 `30005` 专用文案。

### 角色 40000–40999

| code | 常量 | message | HTTP |
|------|------|---------|------|
| 40001 | `ErrRoleAlreadyExists` | 角色已存在 | 409 |
| 40002 | `ErrRoleNotFound` | 角色不存在 | 404 |
| 40003 | `ErrRoleInUse` | 该角色仍有用户关联，无法删除 | 409 |
| 40004 | `ErrRoleIsSystem` | 系统内置角色不可删除 | 403 |

### 组织 50000–50999

| code | 常量 | message | HTTP |
|------|------|---------|------|
| 50001 | `ErrOrgAlreadyExists` | 组织已存在 | 409 |
| 50002 | `ErrOrgNotFound` | 组织不存在 | 404 |
| 50003 | `ErrOrgCannotMoveToChild` | 不能移动到子节点下 | 400 |
| 50004 | `ErrOrgHasChildren` | 该组织下有子组织，无法删除 | 409 |
| 50005 | `ErrOrgHasMembers` | 该组织下有成员，无法删除 | 409 |
| 50006 | `ErrOrgIsSystem` | 系统内置组织不可删除 | 403 |
| 50007 | `ErrNotOrgMember` | 用户不是该组织成员 | 404 |
| 50011 | `ErrDuplicatePrimaryOrg` | 该用户已有主组织，并发设置主组织冲突，请重试 | 409 |
| 50012 | `ErrOrgSystemProtected` | 系统内置组织受保护，禁止此操作 | 403 |

> `50011`（B3-3）：000008 部分唯一索引并发兜底；`50012`（B4-5）：Update 场景，与删除的 50006 区分文案。

<!-- D2-44①：以下为未实现预留段（errcode.go 尚未定义，勿据此对接） -->

| 50008 | `ErrCannotAssignHigherOrgMemberRole` | 不能分配更高的组内角色 | 403 |
| 50009 | `ErrCannotManageOrgMember` | 无权管理该组织成员 | 403 |
| 50010 | `ErrNotOrgOwner` | 需要组织负责人权限 | 403 |
| 50013 | `ErrOrgHasOpenTickets` | 该组织下有未结工单，无法删除 | 409 |

> `50008`–`50010`：已于 **2c Step 8** 写入 `errcode.go`（勿改号）。`50013`：**BK-20**（2026-09-03，禁删有未结工单的组织，design-decisions §21）写入 `errcode.go`。

### 菜单 60000–60999

| code | 常量 | message | HTTP |
|------|------|---------|------|
| 60001 | `ErrMenuAlreadyExists` | 菜单已存在 | 409 |
| 60002 | `ErrMenuNotFound` | 菜单不存在 | 404 |
| 60003 | `ErrMenuHasChildren` | 该菜单下有子菜单，无法删除 | 409 |
| 60004 | `ErrMenuIsSystem` | 系统内置菜单不可删除 | 403 |

### 鉴权 70000–70999

| code | 常量 | message | HTTP |
|------|------|---------|------|
| 70001 | `ErrNoPermission` | 无权限 | 403 |
| 70002 | `ErrPolicyExists` | 策略已存在 | 409 |
| 70003 | `ErrNoRoles` | 未分配角色 | 403 |
| 70004 | `ErrPolicyReloadFailed` | 策略已保存但内存刷新失败，权限可能延迟生效，请稍后重试或联系运维 | 500 |

> `70004`（B3-5）：AssignMenus/DeleteRole 提交 DB 后 LoadPolicy 重试失败时返回——DB 已生效、内存策略陈旧，客户端可感知「部分成功」语义。见上文通用说明：`30006` 与 `70003` 为 Phase 1 验收必需错误码。

### 工单 90000–90999

<!-- 90001–90004 已于 2a Step 2 写入 errcode.go 并接入 handler 映射；本段余量仍预留 -->

| code | 常量 | message | HTTP |
|------|------|---------|------|
| 90001 | `ErrTicketNotFound` | 工单不存在 | 404 |
| 90002 | `ErrTicketInvalidTransition` | 非法状态转换 | 400 |
| 90003 | `ErrTicketTypeNotFound` | 工单类型不存在 | 404 |
| 90004 | `ErrTicketAlreadyClosed` | 工单已关闭 | 409 |

> 90001–90004 已于 **2a Step 2** 写入 `errcode.go`（勿改号）；本段余量预留。不可见工单对外统一 **90001**（决策已收口 2026-08-19，不再与 10004 二选一；10004 保留给通用资源不存在场景，见 [phase2/09-ticket §7](../phase2/09-ticket.md)）。

### 文件存储 91000–91999

<!-- D2-44①：整段为未实现预留（errcode.go 尚未定义，勿据此对接） -->

| code | 常量 | message | HTTP |
|------|------|---------|------|
| 91001 | `ErrFileTooLarge` | 文件大小超过限制 | 400 |
| 91002 | `ErrFileTypeNotAllowed` | 文件类型不允许 | 400 |
| 91003 | `ErrFileNotFound` | 文件不存在 | 404 |
| 91004 | `ErrFileAlreadyBound` | 文件已关联其它资源 | 409 |

> Phase **2b** storage 模块实现时写入 `errcode.go`（码号预留，勿改号）。

---

## 5. Phase 1 验收路径对照

> 完整清单与里程碑（M1–M7）见 [phase1/README.md §1.3](../phase1/README.md#13-验收标准)。

| 验收场景 | 期望 HTTP | 期望 code（主要） |
|----------|-----------|-------------------|
| 错误密码 / 不存在用户 | 401 | 20001（同一文案） |
| 登录时账号已禁用 | 401 | 20001（与密码错误同一文案，防枚举） |
| 连续登录失败 | 429 | 20006（`ErrAccountLocked`） |
| 禁用用户带旧 AT（已登录后吊销） | 403 | 30003 |
| 禁用用户用旧 RT refresh | 401 | 20004（不得返回新 AT/RT） |
| 最后一个 superadmin 操作 | 403 | 30006 |
| admin 重置 superadmin 密码 | 403 | 30005 |
| admin 分配 superadmin 角色 | 403 | 30009 |
| 工号/域账号重复 | 409 | 30007 / 30008 |
| 首次改密期间访问其它 API | 403 | 20007 |
| mcp 期间 GET /user/menus | 403 | 20007 |
| 无角色访问鉴权路由 | 403 | 70003 |
| 无角色 GET /user/menus | 403 | 70003 |
| viewer 零 menu 访问 GET /users | 403 | 70001 |
| viewer 零 menu GET /user/menus | 200 | 0（menus 可为 []） |
| Redis 不可用（鉴权路由） | 503 | 10008 |
| 登出后访问 | 401 | 20003 |
| 添加组织成员 | 200 | 0 |
| 移除非成员 | 404 | 50007（`ErrNotOrgMember`） |
| Bearer + X-AK 混用（AuthN 预留） | 400 | 20008 |
| 请求带 X-AK 但未启用 M2M | 401 | 20009 |

---

## 6. 维护约定

1. 新增错误码：在 `errcode.go` 按模块区间追加，**禁止**复用已删除码号。
2. Handler 通过 `errors.As` 识别 `*errcode.Error`，映射 HTTP；未知错误统一 `10000` + 500。
3. **不要**在 `message` 中返回堆栈、SQL、Redis key 等内部细节。
4. 修改本文时同步更新 `architecture.md` §16（或注明以 `api/response.md` + 本文为准）。
