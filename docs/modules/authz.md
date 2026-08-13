# 鉴权模块设计

> 模块代码：`internal/middleware/casbin.go` + `internal/pkg/resource/` + 各 Service 的 Resource 实现
>
> 旧系统参考：`doc/module-assessment-2026-08/authorizor.md` + `restrict.md` + `resource.md` + `interaction-auth-chain.md`

---

## 1. 模块定位

**核心底座模块**。鉴权分为两层：路由级 RBAC（Casbin 中间件）+ 资源级（ResourceRegistry）。

与其他模块的关系：
- `middleware` 层做路由级鉴权（查 casbin_rule 表）
- 各 Service 自注册 Resource 到 ResourceRegistry
- Handler 调用 ResourceRegistry.Authorize 做资源级鉴权

---

## 2. 路由级鉴权（Casbin RBAC）

### 2.1 Casbin 模型（g 表消除，借鉴旧系统）

```ini
# configs/casbin_model.conf
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && (p.obj == "*" || keyMatch2(r.obj, p.obj)) && (r.act == p.act || p.act == "*")
```

无 `[role_definition] g` 段。角色继承在中间件层 BFS 展开。

### 2.2 中间件流程

```go
func CasbinMiddleware(enforcer *casbin.SyncedEnforcer, permCache *redis.Client) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := c.GetString("user_id")

        // 1. 获取用户角色（Redis 缓存，miss 查 DB）
        roles := getRoles(c, userID, permCache)
        if len(roles) == 0 {
            response.AbortWithStatus(c, 403, "no roles assigned")
            return
        }

        // 2. 超管 bypass
        for _, role := range roles {
            if role == "admin" {
                c.Set("roles", roles)
                c.Next()
                return
            }
        }

        // 3. 逐角色 enforce
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
            response.AbortWithStatus(c, 403, "permission denied")
            return
        }

        // 4. 存入 context 供 handler 复用
        c.Set("roles", roles)
        c.Next()
    }
}
```

### 2.3 BFS 三源合并（借鉴旧系统）

```go
func getRoles(c *gin.Context, userID string, cache *redis.Client) []string {
    // 1. 查 Redis 缓存
    if cached, err := cache.Get(c, "perm:user:"+userID).Result(); err == nil {
        return parseRoles(cached)
    }

    // 2. cache miss → 查 DB（BFS 三源合并）
    roles := []string{}

    // 源 1：直接角色
    directRoles := userRepo.GetRoles(userID)
    roles = append(roles, directRoles...)

    // 源 2：组织角色
    orgRoles := orgRepo.GetRoleByUser(userID)
    roles = append(roles, orgRoles...)

    // 源 3：继承角色（parent_id 链）
    for _, r := range roles {
        parentRoles := roleRepo.GetParents(r)
        roles = append(roles, parentRoles...)
    }

    // 3. 写缓存
    cache.Set(c, "perm:user:"+userID, serialize(roles), 30*time.Minute)

    return dedup(roles)
}
```

---

## 3. 资源级鉴权（ResourceRegistry）

### 3.1 Resource 接口

详见 [proposal/resource-model.md](../proposal/resource-model.md)。

```go
type Resource interface {
    Code() string
    Name() string
    Actions() []string
    Authorize(ctx context.Context, req AuthorizeRequest) (bool, error)
    GetFilter(ctx context.Context, userID string, action string) (Filter, error)
}
```

### 3.2 各资源实现

| 资源 | 实现方式 | 说明 |
|------|---------|------|
| UserResource | 代码内联 | 超管 bypass + 属主判断 |
| RoleResource | 代码内联 | 超管 bypass + 系统角色保护 |
| OrgResource | 代码内联 + ltree | 超管 bypass + 组织管理员判断 |
| MenuResource | 代码内联 | 超管 bypass + 系统菜单保护 |
| TicketResource | 代码内联 + 独立 enforcer | 属主 + 分配人 + 可配置策略 |

### 3.3 列表过滤

```go
// Handler/Service 调用
filter, err := registry.GetFilter(ctx, "ticket", userID, "read")
// filter.Where = "creator_id = $1 OR assignee_id = $1 OR org_id IN (...)"
// filter.Args = [userID]

query := "SELECT * FROM tickets WHERE deleted_at IS NULL AND (" + filter.Where + ") ORDER BY created_at DESC LIMIT $2 OFFSET $3"
rows, err := db.Query(ctx, query, append(filter.Args, size, offset)...)
```

### 3.4 鉴权链路完整时序

```
Client → POST /api/v1/tickets/T001/close
  │
  ├── Recovery → Logger → RequestID → CORS → SecurityHeaders
  │
  ├── JWT 中间件
  │   提取 Authorization: Bearer {token}
  │   验证签名 + 过期 + Redis 黑名单
  │   c.Set("user_id", "U001")
  │
  ├── Casbin 中间件（路由级，PEP-1）
  │   getRoles("U001") → ["operator"]（Redis 缓存）
  │   enforcer.Enforce("role::operator", "/api/v1/tickets/:id/close", "POST")
  │   → true（operator 有 ticket:close 路由权限）
  │   c.Set("roles", ["operator"])
  │
  ├── Audit 中间件（记录请求）
  │
  ├── TicketHandler.CloseTicket
  │   解析 ticketID = "T001"
  │   调用 ticketService.Close(ctx, "T001")
  │
  └── TicketService.Close
      registry.Authorize(ctx, "ticket", {
          UserID: "U001",
          Roles: ["operator"],
          Action: "close",
          ResourceID: "T001",
      })
      → TicketResource.Authorize
          ├─ 超管 bypass？否
          ├─ 属主判断：ticket.CreatedBy == "U001"？否
          ├─ 分配人判断：ticket.AssigneeID == "U001"？是 → 返回 true
          └─ （如都不通过）查 casbin_rule_ticket 表
      → true → 执行关闭操作
```

---

## 4. 旧系统借鉴

| 设计 | 决策 | 理由 |
|------|------|------|
| g 表消除 + BFS 展开 | ✅ 直接采用 | 简化模型 |
| expanded_roles 存 context | ✅ 直接采用 | 避免双倍查询 |
| 三源合并（直接+组织+继承） | ✅ 直接采用 | 成熟设计 |
| 超管通配 bypass | ✅ 直接采用 | 简洁高效 |
| 资源自注册机制 | ✅ 直接采用（改进） | 旧系统 ResourceService → 新框架 ResourceRegistry |
| Restrict 9 种 ConditionType | ❌ 改为代码内联 + 按需 enforcer | 过度抽象，PostgreSQL ltree 替代 |
| GetFilterForResource → MongoDB filter | ✅ 改为 GetFilter → SQL WHERE | 数据库不同 |
| evaluator DI 注入 | ⚠️ 简化 | 各 Resource 自己实现，不需要独立 evaluator |
| grants 内存缓存 | ❌ 改为 Redis 缓存 | 多实例一致 |
| LRU org 缓存 | ❌ 改为 SQL 查询 | ltree 性能足够 |

---

## 5. 分阶段实施

### Phase 1

- Casbin 中间件（路由级 RBAC）
- g 表消除模型
- 超管 bypass
- ResourceRegistry 骨架 + UserResource（属主判断）
- 基本角色查询（直接角色，不含 BFS 三源）
- Adapter：内存（骨架阶段）→ `pckhoi/casbin-pgx-adapter/v3`（接入 DB 后）

### Phase 2

- BFS 三源合并（直接角色 + 组织角色 + 继承角色）
- Redis 缓存展开结果
- OrgResource（ltree 组织关系判断）
- 列表过滤（GetFilter → SQL WHERE）
| 按需引入资源级独立 enforcer |

### Phase 3

- 资源级策略可配置界面
| PDP 服务评估（SpiceDB / Cerbos） |
