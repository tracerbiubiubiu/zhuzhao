# 鉴权模块设计

> 模块代码：`internal/middleware/casbin.go` + `internal/pkg/resource/` + 各 Service 的 Resource 实现
>
> 旧系统参考：`doc/module-assessment-2026-08/authorizor.md` + `restrict.md` + `resource.md` + `interaction-auth-chain.md`
>
> **Phase 1 行为以 [phase1/03-authz.md](../phase1/03-authz.md) 为准**（直接角色、无 Redis 权限缓存、Registry 空接口）。

---

## 1. 模块定位

**核心底座模块**。鉴权分为两层：

| 层级 | 位置 | Phase 1 | Phase 2+ |
|------|------|---------|----------|
| 路由级 RBAC | Casbin **中间件** | ✅ 直接角色 enforce | BFS 三源 + 可选缓存 |
| 资源级 | **Service** + ResourceRegistry | 空接口 | Authorize + GetFilter |

与其他模块的关系：
- `middleware/casbin.go`：路由级鉴权（查 `casbin_rule` + `user_roles`）
- 各 Service 在 Phase 2 自注册 Resource 到 ResourceRegistry
- Handler 调用 `registry.Authorize`（**不在 middleware**，无 `resource_authz.go`）

---

## 2. 路由级鉴权（Casbin RBAC）

### 2.1 Casbin 模型（g 表消除）

```ini
# configs/casbin_model.conf
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == "role::superadmin" || \
    r.sub == "role::admin" || \
    (r.sub == p.sub && (p.obj == "*" || keyMatch2(r.obj, p.obj)) && (r.act == p.act || p.act == "*"))
```

无 `[role_definition] g` 段。用户→角色在 `user_roles` 表，中间件 subject 为 `role::{code}`。

### 2.2 Phase 1 中间件流程（直接角色）

```go
func CasbinMiddleware(enforcer *casbin.SyncedEnforcer, fetcher RoleFetcher) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := c.GetInt64("userID")

        roles, err := fetcher.FetchRoleCodes(c.Request.Context(), userID)
        if err != nil {
            response.InternalError(c, errcode.ErrInternal)
            c.Abort()
            return
        }
        if len(roles) == 0 {
            response.Forbidden(c, errcode.ErrNoRoles)
            c.Abort()
            return
        }

        path := c.Request.URL.Path
        method := c.Request.Method
        allowed := false
        for _, role := range roles {
            if enforcer.Enforce("role::"+role, path, method) {
                allowed = true
                break
            }
        }
        if !allowed {
            response.Forbidden(c, errcode.ErrNoPermission)
            c.Abort()
            return
        }

        c.Set("roles", roles)
        c.Next()
    }
}
```

- superadmin/admin bypass 在 **matcher** 中完成，不在 Go 代码里硬编码循环
- Phase 1 **无** Redis `perm:user:{userId}` 缓存

### 2.2.1 自服务路由（业界做法 + 本项目决策）

**业界常见做法**（IAM / 后台管理系统）可归纳为三类：

| 做法 | 代表 | 特点 |
|------|------|------|
| **A. 鉴权层白名单 / `authenticated-only`** | Spring Security `permitAll`/`authenticated` 分区；API Gateway 对 `/me`、`/logout` 单独路由规则；OAuth2 **UserInfo** 与业务 Resource 分离 | 自服务接口**不进**业务权限码表；「已登录 + 有身份」即可，与菜单/RBAC 解耦 |
| **B. 隐式权限码** | Keycloak Account API、部分 SaaS 的 `profile:read` 默认绑 every user | 仍走统一 PDP，但给所有角色预置「基础包」 |
| **C. 绑进菜单/RBAC** | 部分 RuoYi 衍生把 `getInfo`/`getRouters` 写死放行或挂虚拟菜单 | 模型统一，但菜单表掺非 UI 路由，新角色易漏绑 |

**主流倾向 A**：自服务（当前用户 profile、会话 logout、动态 menus/permissions）属于 **AuthN 之后的基础能力**，不应与「用户管理」「工单列表」等业务权限同一套菜单策略；否则 `operator`/`viewer` 零菜单时连登录后拉菜单都会 403。

**本项目 Phase 1 已采纳 A**：Casbin 中间件在 `len(roles)>0` 之后、逐角色 `Enforce` 之前，对**自服务路由**直接放行（仍要求 JWT 已通过、且非 `mcp` 拦截场景下的非法路径）。
**实现方案（B4-2 回写）**：`SelfService()` 中间件在**路由注册期**打 context 标记（`SelfServiceContextKey`），`CasbinAuth` 检测标记放行——**非** method+path 路径匹配（原设计已废弃：路径匹配可被路径构造绕过且与路由注册解耦；标签方案注册期生效、请求侧不可伪造，router_test.go 行为测试守护注册顺序）。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/user/profile` | 当前用户资料 |
| POST | `/api/v1/user/profile/update` | 更新资料 |
| GET | `/api/v1/user/menus` | 前端动态路由 |
| GET | `/api/v1/user/permissions` | 按钮权限码 |
| POST | `/api/v1/auth/logout` | 登出（`mcp=true` 强制改密期间 **禁止**，仅允许改密） |
| POST | `/api/v1/auth/password/update` | 改密（含 `mcp` 场景唯一允许的业务写操作） |

- **零角色**用户：白名单**不**生效，仍 **403 + 70003**（未分配角色不能进系统）。
- **不写入** `menu_apis` / 菜单树；权限码列标 `—`（见 [07-menu](../phase1/07-menu.md)）。
- 实现 SSOT：[phase1/09-middleware §Casbin](../phase1/09-middleware.md#casbin-中间件g-表消除) 伪代码。

### 2.3 admin 路由 bypass ≠ 业务无约束

> **常见误解**：matcher 里 `role::admin` 与 `role::superadmin` 一样「全通」，是否意味着 admin 能改 superadmin / 其他 admin？  
> **否**。Casbin 只回答 **「这个 HTTP 请求能不能进 Handler」**；**「能不能动这个具体对象」** 在 **Service 业务层** 用 `roles.priority` + 影子超管规则拦截。

**两层分工**：

| 层 | 位置 | 回答什么 | superadmin | admin |
|----|------|----------|------------|-------|
| **L1 路由级** | Casbin 中间件 | 能否调用 `POST /users/update` 这类 API？ | matcher bypass | matcher bypass（**等价**） |
| **L2 业务防提权** | 各 Service 方法内 | 能否改**这个**用户/角色/菜单？ | 几乎无限制（兜底规则除外） | **priority + is_system + 影子超管** |

**为何 admin 仍做 Casbin bypass？**

- Phase 1 管理端是「全局管理员」模型：admin 应能调全部 IAM API，不必为每个路由维护 `p` 策略。
- 细粒度「对谁动手」不适合塞进 Casbin 策略表（策略爆炸 + 无法表达「目标用户是谁」）。
- 与若依/RuoYi 同类：**路由权限** 与 **数据/对象级防提权** 分离。

**L2 拦截点（Phase 1 实现位置）**：

| 场景 | 拦截位置 | 规则 | 失败 |
|------|----------|------|------|
| 重置密码 | `UserService.ResetPassword` | `actorP < targetP`（严格更强）；影子超管用户对 admin **404** | 403 + 30005 |
| 分配角色 | `UserService.SetRoles` | `actorP < targetP`；待分配角色 `priority >= actorP`（不能分 superadmin） | 403 + 30009 |
| 改/删/禁用用户 | `UserService.Update/Delete/SetStatus` | `actorP < targetP`；`is_system` 保护；不能删最后 superadmin | 403 / 404 |
| 改角色菜单 | `RoleService.AssignMenus` | admin **不能**改 `superadmin` 角色的菜单 | 403 |
| 删改系统资源 | `Role/Menu/Org Service` | `is_system=true` 时 admin 拒绝 | 403 |
| 列表/详情「看不见」 | `UserRepo.List` / `RoleRepo.List` / `GetByID` | 非 superadmin：**过滤** superadmin 角色及绑定用户 | 列表不含 / 详情 **404** |

**priority 计算**（与 [04-user §多角色](../phase1/04-user.md#多角色与有效-priority) 一致）：

```go
// EffectivePriority = min(用户全部角色的 priority)；越小越强
func CanManageUser(actorP, targetP int) bool {
    return actorP < targetP // 严格更强才能管对方（含 admin 不能动 admin）
}
```

**示例**：

| 操作者 | 目标 | L1 Casbin | L2 业务 |
|--------|------|-----------|---------|
| admin | operator 用户 | ✅ 放行 | ✅ `10 < 20` |
| admin | 另一 admin 用户 | ✅ 放行 | ❌ `10 < 10` 不成立 → 403 |
| admin | superadmin 用户 | ✅ 放行 | ❌ 列表不可见；直调 id → **404**；若强行写则 `10 < 1` → 403 |
| superadmin | admin 用户 | ✅ | ✅ `1 < 10` |

> SSOT 对照：[05-role §superadmin 与 admin](../phase1/05-role.md#superadmin-与-admin-的区别)、[04-user §角色分级与系统保护](../phase1/04-user.md#角色分级与系统保护业务校验非-casbin)、[resource-model §场景 2](../proposal/resource-model.md)。

### 2.4 Phase 2b+：BFS 三源合并 + 可选缓存

Phase 2 扩展 `RoleFetcher`：

```go
func (f *roleFetcher) FetchRoleCodes(ctx context.Context, userID int64) ([]string, error) {
    // Phase 2: 源1 直接角色 + 源2 组织角色 + 源3 继承角色，去重
    // Phase 3: 可选 Redis perm:user:{userId} 缓存
}
```

---

## 3. 资源级鉴权（ResourceRegistry）

Phase 1 只定义 `Resource` / `Registry` 接口，不注册业务 Resource。

Phase 2 工单等模块在 Service 构造函数中 `registry.Register(...)`，在 Service 方法内调用：

```go
ok, err := s.registry.Authorize(ctx, "ticket", resource.AuthorizeRequest{
    UserID: uid,
    Roles:  roles,
    Action: "update",
    ResourceID: ticketID,
})
```

列表过滤：`registry.GetFilter(ctx, "ticket", uid, "read")` → SQL WHERE。

详见 [proposal/resource-model.md](../proposal/resource-model.md)。

---

## 4. 旧系统借鉴

| 设计 | 决策 | 理由 |
|------|------|------|
| g 表消除 + BFS 展开 | ✅ Phase 2b 采用 | 简化 Casbin 模型 |
| expanded_roles 存 context | ✅ 采用 | Casbin 中间件写入 `roles` 供 handler 复用 |
| 三源合并 | ⏳ Phase 2b | Phase 1 仅直接角色 |
| 资源自注册 | ✅ Phase 2 工单验证 | Phase 1 空接口 |
| Restrict 9 种 ConditionType | ❌ 改为 ltree + 内联 | 降低抽象成本 |
| grants 内存缓存 | ❌ Phase 3 Redis 按需 | 多实例一致 |

---

## 5. 分阶段实施

### Phase 1

- Casbin 中间件（路由级 RBAC）
- g 表消除模型 + matcher superadmin/admin bypass
- `RoleFetcher` 查 `user_roles`（直接角色）；**多角色 OR 鉴权、业务分级用 priority（min）**，见 [phase1/03-authz §用户多角色](../phase1/03-authz.md#用户多角色phase-1)
- ResourceRegistry **空接口**
- Adapter：`noho-digital/casbin-pgx-adapter`（Casbin v3）
- **不做**组织范围数据过滤

### Phase 2

- BFS 三源合并
- OrgResource + ltree 关系
- 列表过滤（GetFilter → SQL WHERE）
- 代码内联资源级鉴权（不上独立 Enforcer）

### Phase 3

- 可选 `perm:user:{userId}` Redis 缓存 + Pub/Sub 失效
- 每资源独立 Enforcer / PDP 评估（按需）
