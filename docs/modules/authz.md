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

### 2.3 Phase 2+：BFS 三源合并 + 可选缓存

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
| g 表消除 + BFS 展开 | ✅ Phase 2 采用 | 简化 Casbin 模型 |
| expanded_roles 存 context | ✅ 采用 | Casbin 中间件写入 `roles` 供 handler 复用 |
| 三源合并 | ⏳ Phase 2 | Phase 1 仅直接角色 |
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
- Adapter：`pckhoi/casbin-pgx-adapter/v3`
- **不做**组织范围数据过滤

### Phase 2

- BFS 三源合并
- OrgResource + ltree 关系
- 列表过滤（GetFilter → SQL WHERE）
- 代码内联资源级鉴权（不上独立 Enforcer）

### Phase 3

- 可选 `perm:user:{userId}` Redis 缓存 + Pub/Sub 失效
- 每资源独立 Enforcer / PDP 评估（按需）
