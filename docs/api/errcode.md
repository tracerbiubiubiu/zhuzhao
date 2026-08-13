# 错误码约定

> 统一 JSON 响应中的业务错误码（`code`）与 HTTP 状态码映射。  
> 实现：`internal/pkg/errcode/errcode.go` + `internal/pkg/response/response.go`  
> Phase 1 实现时以本文与 phase 验收路径为准；新增错误码须同步更新本文。

---

## 1. 响应格式

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

| 字段 | 说明 |
|------|------|
| `code` | 业务错误码，`0` 表示成功 |
| `message` | 面向用户的简短中文说明 |
| `data` | 成功时为业务数据；失败时通常省略或为 `null` |

前端联调：**以 `code` 做分支**，不要只依赖 HTTP 状态码（例如登录失败 401 与 token 无效 401 可能共用 HTTP 401，但 `code` 不同）。

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

---

## 3. HTTP 状态码映射

| HTTP | 场景 |
|------|------|
| 200 | 成功 |
| 400 | 参数校验失败（`ErrInvalidParams`） |
| 401 | 未登录、Token 无效/过期/已吊销 |
| 403 | 已登录但无权限、账号禁用、须改密、无角色 |
| 404 | 资源不存在（对外可暴露「不存在」时） |
| 409 | 唯一约束冲突（用户名/编码重复等） |
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
| 10007 | `ErrTooManyReqs` | 请求过于频繁 | 429 |
| 10008 | （预留） | 服务暂时不可用 | 503 |

### 认证 20000–20999

| code | 常量 | message | HTTP |
|------|------|---------|------|
| 20001 | `ErrInvalidCredentials` | 用户名或密码错误 | 401 |
| 20002 | `ErrTokenExpired` | token 已过期 | 401 |
| 20003 | `ErrTokenInvalid` | token 已失效 | 401 |
| 20004 | `ErrRefreshTokenInvalid` | 刷新令牌无效 | 401 |
| 20005 | `ErrTokenAlreadyRefreshed` | 令牌已被刷新 | 401 |
| 20006 | `ErrAccountLocked` | 账号已锁定 | 429 |
| 20007 | `ErrPasswordChangeRequired` | 需要修改密码 | 403 |

> 文档中的 `PASSWORD_CHANGE_REQUIRED` 即 `20007`。

### 用户 30000–30999

| code | 常量 | message | HTTP |
|------|------|---------|------|
| 30001 | `ErrUserAlreadyExists` | 用户已存在 | 409 |
| 30002 | `ErrUserNotFound` | 用户不存在 | 404 |
| 30003 | `ErrUserDisabled` | 用户已禁用 | 403 |
| 30004 | `ErrUserIsSystem` | 系统内置用户不可删除 | 403 |
| 30005 | `ErrCannotResetHigher` | 不能重置同级或更高级用户的密码 | 403 |
| 30006 | `ErrCannotRemoveLastSuperadmin` | 不能移除最后一个超级管理员 | 403 |

> `30006`：**Phase 1 待实现**（文档与验收已要求，实现 user 模块时加入 `errcode.go`）。

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

> `70003`：**Phase 1 待实现**（Casbin 中间件无角色时返回）。

---

## 5. Phase 1 验收路径对照

| 验收场景 | 期望 HTTP | 期望 code（主要） |
|----------|-----------|-------------------|
| 错误密码 / 不存在用户 | 401 | 20001（同一文案） |
| 连续登录失败 | 429 | 20006 或 10007 |
| 禁用用户带旧 AT | 401/403 | 30003 |
| 最后一个 superadmin 操作 | 403 | 30006 |
| admin 重置 superadmin 密码 | 403 | 30005 |
| 首次改密期间访问其它 API | 403 | 20007 |
| 无角色访问鉴权路由 | 403 | 70003 |
| Redis 不可用（鉴权路由） | 503 | 10000 |
| 登出后访问 | 401 | 20003 |

---

## 6. 维护约定

1. 新增错误码：在 `errcode.go` 按模块区间追加，**禁止**复用已删除码号。
2. Handler 通过 `errors.As` 识别 `*errcode.Error`，映射 HTTP；未知错误统一 `10000` + 500。
3. **不要**在 `message` 中返回堆栈、SQL、Redis key 等内部细节。
4. 修改本文时同步更新 `architecture.md` §16.2（或注明以本文为准）。
