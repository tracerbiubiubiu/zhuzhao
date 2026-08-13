# 03 - 鉴权模块（authz）

> Step 5，依赖 Step 4（middleware）。Phase 1 实现路由级 RBAC + ResourceRegistry 骨架。

---

## 预期功能

| 功能 | 场景 | 说明 |
|------|------|------|
| 路由级 RBAC | 用户请求 API，Casbin 校验是否有权限 | 中间件层，基于角色 + 路径 + 方法 |
| Casbin 策略加载 | 从 PostgreSQL 加载策略到内存 | 启动时全量加载 |
| admin 角色绕过 | admin 角色拥有所有权限 | Casbin matcher 中 `r.sub == "role::superadmin" \|\| r.sub == "role::admin"` 直接放行 |
| ResourceRegistry 骨架 | 资源自注册接口 | Phase 1 只定义接口，不实现资源级鉴权逻辑 |
| 策略管理 API | 管理员查看/添加/删除 Casbin 策略 | 角色管理模块调用 |

### Phase 1 不做

| 功能 | 原因 | 阶段 |
|------|------|------|
| 资源级鉴权（ltree 查询） | Phase 1 无业务资源 | Phase 2 |
| Casbin Watcher（多实例同步） | Phase 1 单实例 | Phase 3 |
| 每资源独立 Enforcer | Phase 1 无业务资源 | Phase 2 |
| OPA/Zanzibar 可切换 | 框架预留接口即可 | Phase 2 |

---

## 核心设计思路

### Casbin 模型（g 表消除 + 中间件 BFS 展开）

> 借鉴旧系统成熟设计，详见 [modules/authz.md](../modules/authz.md) §2.1。

```ini
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

**无 `[role_definition] g` 段**。角色继承不写 Casbin g 表，在中间件层 BFS 展开后逐角色 enforce。

- `sub` = `role::{roleCode}`（如 `role::admin`、`role::user_manager`）
- `obj` = API 路径（如 `/api/v1/users`）
- `act` = HTTP 方法（GET/POST）
- `r.sub == "role::admin"` — admin 角色直接放行
- `keyMatch2` — 支持路径通配符（`/api/v1/users/:id` 匹配 `/api/v1/users/123`）

### 中间件流程（借鉴旧系统）

```go
func CasbinMiddleware(enforcer *casbin.SyncedEnforcer, roleFetcher RoleFetcher) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := c.GetInt64("userID")

        // 1. 获取用户角色（Phase 1 只查直接角色，Phase 2 加 BFS 三源合并）
        roles, err := roleFetcher.FetchRoleCodes(c.Request.Context(), userID)
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

        // 2. 逐角色 enforce（superadmin/admin 在 matcher 中自动 bypass）
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

        // 3. 存入 context 供 handler 复用
        c.Set("roles", roles)
        c.Next()
    }
}

// RoleFetcher 接口，避免中间件直接依赖 UserRepo
type RoleFetcher interface {
    FetchRoleCodes(ctx context.Context, userID int64) ([]string, error)
}
```

Phase 1 `getRoles` 只查 `user_roles` 表（直接角色）。Phase 2 扩展为 BFS 三源合并：
1. 直接角色：`user_roles` 表
2. 组织角色：`user_orgs` → `org_roles`
3. 继承角色：`roles.parent_id` 链

### 策略来源：DB → Casbin

DB 是 source of truth，Casbin 是 derived。角色管理时更新 DB → 同步到 Casbin：

```
管理员分配角色菜单
  │
  ├── 更新 role_menus 表（DB 事务）
  ├── 根据 menu → api 映射，生成 Casbin 策略
  ├── 写入 casbin_rule 表（同一事务）
  └── 调用 enforcer.ReloadPolicy()（事务提交后）
```

### ResourceRegistry 接口骨架

> 详见 [proposal/resource-model.md](../proposal/resource-model.md) §2。Phase 1 只定义接口 + 实现 Registry，不注册任何资源。

```go
// Resource 资源接口，每个业务模块实现并自注册
type Resource interface {
    Code() string
    Name() string
    Actions() []string
    Authorize(ctx context.Context, req AuthorizeRequest) (bool, error)
    GetFilter(ctx context.Context, userID int64, action string) (Filter, error)
}

type AuthorizeRequest struct {
    UserID     int64
    Roles      []string
    Action     string
    ResourceID int64
    Context    context.Context
}

type Filter struct {
    Where string
    Args  []interface{}
}

// Registry 资源注册中心
type Registry interface {
    Register(r Resource)
    Get(code string) (Resource, bool)
    List() []Resource
    Authorize(ctx context.Context, resourceCode string, req AuthorizeRequest) (bool, error)
    GetFilter(ctx context.Context, resourceCode string, userID int64, action string) (Filter, error)
}

// registry 实现（sync.RWMutex + map）
type registry struct {
    mu        sync.RWMutex
    resources map[string]Resource
}

func NewRegistry() Registry {
    return &registry{resources: make(map[string]Resource)}
}

func (r *registry) Register(res Resource) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.resources[res.Code()] = res
}
```

**Wire DI 集成**：`NewRegistry` 作为 singleton provider，各 Service 构造函数接收 `Registry` 并在构造时自注册（`registry.Register(&UserResource{...})`）。

**Phase 1 无资源注册**：启动后 registry 为空，正常运行。Phase 2 各 Service 实现 `Resource` 接口并自注册。

### Casbin Adapter

Phase 1 直接使用 PostgreSQL adapter（`pckhoi/casbin-pgx-adapter/v3`），不走内存 adapter：

```go
func New(cfg config.CasbinConfig, pool *pgxpool.Pool) (*casbin.SyncedEnforcer, func(), error) {
    m, _ := model.NewModelFromFile(cfg.Model)
    adapter, _ := pgadapter.NewAdapter(pool)  // PG adapter
    enforcer, _ := casbin.NewSyncedEnforcer(m, adapter)
    enforcer.LoadPolicy()
    return enforcer, func() { enforcer.StopAutoLoadPolicy() }, nil
}
```

---

## 测试用例

### 路由级 RBAC

| 用例 | 角色 | 请求 | 预期 |
|------|------|------|------|
| admin 全通 | role::admin | `GET /api/v1/users` | 200 |
| admin 全通 | role::admin | `POST /api/v1/users` | 200 |
| 有权限 | role::user_manager | `GET /api/v1/users` | 200 |
| 无权限 | role::user_viewer | `POST /api/v1/users` | 403 |
| 无权限 | role::user_viewer | `GET /api/v1/roles` | 403 |
| 路径通配符 | role::user_manager | `GET /api/v1/users/123` | 200（keyMatch2 匹配） |
| 未认证 | - | 无 AT 的请求 | 401（JWT 中间件拦截） |
| 无角色 | 新建用户未分配角色 | `GET /api/v1/users` | 403 |

### 策略管理

| 用例 | 操作 | 预期 |
|------|------|------|
| 添加策略 | 角色分配菜单 | casbin_rule 表新增记录 + enforcer 生效 |
| 删除策略 | 角色取消菜单 | casbin_rule 表删除记录 + enforcer 生效 |
| 策略加载 | 服务启动 | 从 DB 加载所有策略到内存 |
| 策略重载 | 手动触发 | ReloadPolicy 后新策略立即生效 |

### ResourceRegistry

| 用例 | 操作 | 预期 |
|------|------|------|
| 注册资源 | Register("ticket", ...) | 注册成功，registry 中存在 |
| 未注册资源查询 | CheckAccess("unknown", ...) | 返回 false |
| Phase 1 无注册 | 启动后 registry 为空 | 正常运行，无 panic |

---

## 涉及文件

```
internal/casbin/enforcer.go           # Casbin enforcer 构建（已有，需改为 PG adapter）
internal/middleware/casbin.go         # Casbin 中间件（g 表消除，逐角色 enforce）
internal/pkg/resource/registry.go     # Resource 接口 + Registry 实现（需创建）
internal/service/authz_service.go     # 策略管理 Service
```

## 待决策点

> 以下决策已在讨论中确认：

- ✅ **Casbin 模型**：采用 g 表消除 + 中间件 BFS 展开（借鉴旧系统），不使用 `[role_definition] g` 段。
- ✅ **Casbin adapter**：直接上 PG adapter（`pckhoi/casbin-pgx-adapter/v3`）。
- ✅ **策略同步时机**：角色菜单变更后，事务内写 casbin_rule + 事务后 ReloadPolicy（DB 为 source of truth）。
- ✅ **Phase 1 角色查询**：只查直接角色（user_roles 表），Phase 2 扩展为 BFS 三源合并。
