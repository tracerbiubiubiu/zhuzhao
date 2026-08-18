# 03 - 鉴权模块（authz）

> **Step 5**，依赖 Step 4（JWT 已挂载）。Casbin 中间件 + PG adapter + 自服务白名单 + ResourceRegistry 空接口。  
> JWT 部分见 [09-middleware §Step 4 vs Step 5](./09-middleware.md#step-4-vs-step-5-分工)。

---

## 预期功能

| 功能 | 场景 | 说明 |
|------|------|------|
| 路由级 RBAC | 用户请求 API，Casbin 校验是否有权限 | 中间件层，基于角色 + 路径 + 方法 |
| Casbin 策略加载 | 从 PostgreSQL 加载策略到内存 | 启动时全量加载 |
| superadmin/admin 路由 bypass | 两角色在 Casbin matcher 直接放行 | `r.sub == "role::superadmin" \|\| r.sub == "role::admin"`；**业务层仍有分级**，见 [05-role §superadmin 与 admin](./05-role.md#superadmin-与-admin-的区别) |
| ResourceRegistry 空接口 | 资源自注册接口定义 | Phase 1 不注册业务 Resource |
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

### Casbin 模型（g 表消除；Phase 1 仅直接角色 enforce）

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

**无 `[role_definition] g` 段**。**Phase 1** 只查 `user_roles` 直接角色，逐 `role::{code}` enforce；**Phase 2b** 再扩展 BFS 三源合并（直接 + 组织 + 继承），见 [phase2/03-org-enhance](../phase2/03-org-enhance.md)。

- `sub` = `role::{roleCode}`（如 `role::admin`、`role::user_manager`）
- `obj` = API 路径（如 `/api/v1/users`）
- `act` = HTTP 方法（GET/POST）
- `r.sub == "role::superadmin" || r.sub == "role::admin"` — 两角色在 matcher 直接放行（路由级等价；业务层分级见 [05-role](./05-role.md#superadmin-与-admin-的区别)）
- `keyMatch2` — 支持路径通配符（`/api/v1/users/:id` 匹配 `/api/v1/users/123`）

### Enforcer 实例（Phase 1：仅一个）

| 项 | Phase 1 |
|----|---------|
| 实例数量 | **全局唯一**一个 `SyncedEnforcer`（Wire 单例，注入 Casbin 中间件） |
| 职责 | **路由级** RBAC（`role::{code}` × path × method） |
| 策略存储 | 单表 `casbin_rule` + PG adapter（`pckhoi/casbin-pgx-adapter/v3`） |
| 不做 | **每资源独立 Enforcer**、FilteredAdapter 多实例、资源级 Casbin PDP |

> **`SyncedEnforcer` ≠ 多个 Enforcer**：它是 Casbin 自带的**并发读安全**包装（内部读写锁），Phase 1 仍只有这一份实例。每资源独立 Enforcer 见上方「Phase 1 不做」及 [design-decisions §8](../design/design-decisions.md#8-casbin-策略爆炸每资源独立-enforcer)（Phase 2+ / 按需）。

### 用户多角色（Phase 1）

- **角色是实体**（`roles` 表），**用户可绑定多个**（`user_roles` 多对多，`POST /users/roles` 传 `role_ids` 数组，全量覆盖）。
- **路由鉴权（Casbin）**：查出用户全部直接角色，**逐个 enforce，任一放行即通过**（权限**并集 / OR**）。
- **菜单 / 权限码**：合并用户所有角色绑定的菜单与按钮码（并集）。
- **业务层分级**（重置密码、防提权等）：`roles.priority` **越小越强**；多角色取 **EffectivePriority = min(priority)**。详见 [05-role §priority 与继承](./05-role.md#角色-priority-与权限继承模型)、[04-user §多角色与有效 priority](./04-user.md#多角色与有效-priority)。
- JWT **不含角色**；每次请求由 `RoleFetcher` 查 `user_roles`，角色变更后下次请求即生效。
- **自服务路由**（profile/menus/logout 等）走 Casbin 中间件 **白名单**，不进 `menu_apis`；见 [modules/authz §2.2.1](../modules/authz.md#221-自服务路由业界做法-本项目决策)。

### 中间件流程（借鉴旧系统）

```go
func CasbinMiddleware(enforcer *casbin.SyncedEnforcer, roleFetcher RoleFetcher) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := c.GetInt64("userID")

        // 1. 获取用户角色（Phase 1 只查直接角色，Phase 2b 加 BFS 三源合并）
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

        path := c.Request.URL.Path
        method := c.Request.Method
        if isSelfServiceRoute(method, path) {
            c.Set("roles", roles)
            c.Next()
            return
        }

        // 2. 逐角色 enforce（superadmin/admin 在 matcher 中自动 bypass）
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

Phase 1 `getRoles` 只查 `user_roles` 表（直接角色）。

#### BFS 三源角色（Phase 2b）

Phase 2b 扩展为 BFS 三源合并：
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
    Action     string          // "create", "read", "update", "delete"
    ResourceID string          // 具体资源 ID（create 时为空；业务 ID 也可用 int64）
    Context    map[string]any  // 扩展上下文
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

**Wire DI 集成**：`NewRegistry` 作为 singleton provider 注入 `wire.go` / `wire_gen.go`；各 Service 构造函数接收 `Registry` 并在构造时自注册（Phase 2）。Phase 1 **仅** `wire` 注入空 Registry，**不**在 User/Role/Org Service 内 `Register`。

> **骨架现状**：`internal/pkg/resource/` 目录 Step 5 创建；未创建前以本文接口为准。

> **勿与生命周期混淆**：`Resource` 不含 `Start`/`Stop`；后台任务用 `app.Runner`（Phase 3+），见 [architecture §3.6](../design/architecture.md#36-组件注册与生命周期三者分离)。

**Phase 1 无资源注册、无数据范围过滤**：启动后 registry 为空。`GET /users` 等管理列表只做路由级鉴权，有权限即可见全部数据。Phase 2 各 Service 实现 `Resource` 接口并自注册，才做组织范围过滤。

### Casbin Adapter

Phase 1 直接使用 PostgreSQL adapter（`pckhoi/casbin-pgx-adapter/v3`），**不走内存 adapter**：

> **表结构**：adapter 读取列名 `p_type`（非 `ptype`）。`000001_init` 与 `000003_casbin_column` 已对齐；新环境勿混用旧 `ptype` 脚本。

> **骨架现状**：`internal/casbin/enforcer.go` 可能暂用内存 adapter + TODO；**Step 5 必须切换**为 PG adapter 并 `LoadPolicy()`，与 `000002_seed` 的 Casbin 初始策略及 `AssignMenus` 写 `casbin_rule` 联调。单测可继续用内存 adapter，集成测试 / 验收须 PG。

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

> 表中 `user_manager` 为**测试用自建角色**（非种子四角色 `superadmin/admin/operator/viewer`）。跑用例前须创建 `code=user_manager` 的角色并分配对应菜单/策略；只读场景可用种子角色 `viewer`。

### 路由级 RBAC

| 用例 | 角色 | 请求 | 预期 |
|------|------|------|------|
| admin 全通 | role::admin | `GET /api/v1/users` | 200 |
| admin 全通 | role::admin | `POST /api/v1/users` | 200 |
| 有权限 | role::user_manager | `GET /api/v1/users` | 200 |
| 无权限 | role::viewer | `POST /api/v1/users` | 403 |
| 无权限 | role::viewer | `GET /api/v1/roles` | 403 |
| 路径通配符 | role::user_manager | `GET /api/v1/users/123` | 200（keyMatch2 匹配） |
| 未认证 | - | 无 AT 的请求 | 401（JWT 中间件拦截） |
| 无角色 | 新建用户未分配角色 | `GET /api/v1/users` | 403 + 70003 |
| 自服务白名单 | role::viewer（零 menu） | `GET /api/v1/user/menus` | 200 |
| 零角色 + 自服务 | 未分配角色 | `GET /api/v1/user/menus` | 403 + 70003 |
| 多角色 OR | viewer + user_manager | user_manager 有 POST 策略时 `POST /api/v1/users` | 200 |

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
| 未注册资源查询 | Authorize("unknown", ...) | 返回 false |
| Phase 1 无注册 | 启动后 registry 为空 | 正常运行，无 panic |

---

## 涉及文件

> 目标目录见 [architecture §3.5](../design/architecture.md#35-领域模块目录约定单仓可拆分)。

```
internal/casbin/enforcer.go           # PG adapter（pckhoi/casbin-pgx-adapter/v3）
internal/middleware/casbin.go         # Casbin 中间件 + isSelfServiceRoute（Step 5 挂载）
internal/repository/user/role_fetcher.go   # 或 internal/service/authz/role_fetcher.go — RoleFetcher 实现
internal/pkg/resource/registry.go     # Resource 接口 + Registry（Phase 1 空）
```

### RoleFetcher 实现（Step 5）

`RoleFetcher` 避免中间件直接依赖 UserRepo，推荐：

```go
// internal/repository/user/role_fetcher.go（推荐：与 user_roles 查询同域）
type roleFetcher struct { repo UserRepo }

func (f *roleFetcher) FetchRoleCodes(ctx context.Context, userID int64) ([]string, error) {
    return f.repo.ListRoleCodesByUserID(ctx, userID)
}
```

Wire：`NewRoleFetcher(userRepo UserRepo) middleware.RoleFetcher`。单测可 mock `UserRepo`。

## 待决策点

> 以下决策已在讨论中确认：

- ✅ **Casbin 模型**：g 表消除；Phase 1 仅直接角色，Phase 2b BFS 三源。
- ✅ **Casbin adapter**：直接上 PG adapter（`pckhoi/casbin-pgx-adapter/v3`）。
- ✅ **策略同步时机**：角色菜单变更后，事务内写 casbin_rule + 事务后 ReloadPolicy（DB 为 source of truth）。
- ✅ **Phase 1 角色查询**：只查直接角色（user_roles 表），Phase 2b 扩展为 BFS 三源合并。
