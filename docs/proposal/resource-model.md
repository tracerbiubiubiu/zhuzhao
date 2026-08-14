# 资源抽象与自注册机制方案

> 将每种服务看作一种"资源"，通过统一接口抽象，各自实现鉴权策略并统一注册。
>
> 结合旧系统 zhuzhao 的 ResourceService 自注册机制和业界 Resource Registry 模式。
>
> **用户 ID 为 `int64`。** Phase 1 只留空接口；Phase 2 工单用代码内联 + ltree，**不上独立 Enforcer**（独立 Enforcer 后移，按需）。
>
> 创建日期：2026-08-12

---

## 1. 设计目标

### 1.1 核心场景

通用办公管理后台，每个部门/虚拟组织有对应权限，权限管理达到资源级检查。每种服务（用户、组织、角色、Casbin 策略、工单等）都看作一种"资源"，每种资源的通用能力抽象为接口，各自实现并统一注册。

### 1.2 设计要求

- 每种资源实现统一接口，各自定义鉴权策略
- 资源自注册：Service 构造函数注册资源定义，不改注册中心代码（开闭原则）
- 简单资源用代码内联判断，复杂资源用独立 Casbin enforcer
- 统一的列表过滤能力（数据级权限）
- 为未来微服务化做准备：资源接口可迁移到独立服务

### 1.3 与 Wire / 生命周期的边界

**ResourceRegistry 只负责鉴权**，与以下机制分开，勿合并为单一 `Service` 接口：

| 机制 | 职责 |
|------|------|
| Wire | 依赖注入：组装 Handler / Service / Repo |
| `resource.Resource` | 数据级 `Authorize` / `GetFilter` |
| `app.Runner`（Phase 3+） | 后台 goroutine 的 `Start` / `Stop` |
| Wire `cleanup` | PG / Redis / Casbin 连接释放 |

详见 [architecture §3.6](../design/architecture.md#36-组件注册与生命周期三者分离)。

---

## 2. 资源接口设计

### 2.1 核心接口

```go
// internal/pkg/resource/resource.go

package resource

// Resource 资源接口：每种资源类型实现此接口
type Resource interface {
    // 元数据
    Code() string              // "user", "role", "ticket"
    Name() string              // "用户", "角色", "工单"
    Actions() []string         // ["create", "read", "update", "delete"]

    // 资源级鉴权（PEP-2）
    Authorize(ctx context.Context, req AuthorizeRequest) (bool, error)

    // 列表过滤（数据级权限）
    GetFilter(ctx context.Context, userID int64, action string) (Filter, error)
}

// AuthorizeRequest 统一鉴权请求
type AuthorizeRequest struct {
    UserID     int64
    Roles      []string
    Action     string          // "create", "read", "update", "delete"
    ResourceID string          // 具体资源 ID（create 时为空；业务 ID 也可用 int64）
    Context    map[string]any  // 扩展上下文
}

// Filter 列表过滤条件
type Filter struct {
    Where string        // SQL WHERE 子句（不含 "WHERE"）
    Args  []interface{} // 参数化查询值
}

// Registry 资源注册表接口
type Registry interface {
    Register(res Resource)
    Get(code string) (Resource, bool)
    List() []Resource
    Authorize(ctx context.Context, resourceCode string, req AuthorizeRequest) (bool, error)
    GetFilter(ctx context.Context, resourceCode string, userID int64, action string) (Filter, error)
}
```

### 2.2 注册表实现

```go
// internal/pkg/resource/registry.go

type registry struct {
    mu        sync.RWMutex
    resources map[string]Resource
}

func NewRegistry() Registry {
    return &registry{
        resources: make(map[string]Resource),
    }
}

func (r *registry) Register(res Resource) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.resources[res.Code()] = res
}

func (r *registry) Get(code string) (Resource, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    res, ok := r.resources[code]
    return res, ok
}

func (r *registry) List() []Resource {
    r.mu.RLock()
    defer r.mu.RUnlock()
    result := make([]Resource, 0, len(r.resources))
    for _, res := range r.resources {
        result = append(result, res)
    }
    return result
}

func (r *registry) Authorize(ctx context.Context, code string, req AuthorizeRequest) (bool, error) {
    res, ok := r.Get(code)
    if !ok {
        return false, fmt.Errorf("resource %s not registered", code)
    }
    return res.Authorize(ctx, req)
}

func (r *registry) GetFilter(ctx context.Context, code string, userID int64, action string) (Filter, error) {
    res, ok := r.Get(code)
    if !ok {
        return Filter{}, fmt.Errorf("resource %s not registered", code)
    }
    return res.GetFilter(ctx, userID, action)
}
```

---

## 3. 资源实现示例

### 3.1 简单资源：用户资源（代码内联判断）

```go
// internal/service/user/resource.go

type UserResource struct {
    service *UserService
}

func NewUserResource(svc *UserService) *UserResource {
    return &UserResource{service: svc}
}

func (r *UserResource) Code() string    { return "user" }
func (r *UserResource) Name() string    { return "用户" }
func (r *UserResource) Actions() []string {
    return []string{"create", "read", "update", "delete"}
}

func (r *UserResource) Authorize(ctx context.Context, req resource.AuthorizeRequest) (bool, error) {
    // 超管 bypass
    if util.HasRole(req.Roles, "admin") {
        return true, nil
    }

    // create：有 user:create 权限即可
    if req.Action == "create" {
        return true, nil // 路由级 Casbin 已校验
    }

    // read/update/delete：属主判断
    user, err := r.service.GetByID(ctx, req.ResourceID)
    if err != nil {
        return false, err
    }

    // 属主判断
    if user.CreatedBy == req.UserID {
        return true, nil
    }

    // 组织管理员判断（可选）
    // if isAdminOfUserOrg(req.UserID, user.OrgID) { return true, nil }

    return false, nil
}

func (r *UserResource) GetFilter(ctx context.Context, userID int64, action string) (resource.Filter, error) {
    // 超管不过滤（看全部）
    // 属主：creator_id = $1
    // 组织成员：org_id IN (SELECT org_id FROM user_orgs WHERE user_id = $1)
    return resource.Filter{
        Where: "creator_id = $1 OR org_id IN (SELECT org_id FROM user_orgs WHERE user_id = $1)",
        Args:  []interface{}{userID},
    }, nil
}
```

### 3.2 工单资源（Phase 2：代码内联；独立 enforcer 按需）

**Phase 2 默认**：属主 + 分配人 + ltree 组织范围，**代码内联**，不上 Ticket 专属 Casbin enforcer（与 [phase2/README.md](../phase2/README.md) 一致）。

**Phase 3 / 按需**：若工单策略需可配置化，再引入独立 enforcer（以下为可选参考实现）：

```go
// internal/service/ticket/resource.go — Phase 2 默认无 enforcer 字段

type TicketResource struct {
    service *TicketService
    // enforcer *casbin.SyncedEnforcer  // Phase 3 按需
}

func NewTicketResource(svc *TicketService) *TicketResource {
    return &TicketResource{service: svc}
}

func (r *TicketResource) Code() string    { return "ticket" }
func (r *TicketResource) Name() string    { return "工单" }
func (r *TicketResource) Actions() []string {
    return []string{"create", "read", "update", "delete", "assign", "close"}
}

func (r *TicketResource) Authorize(ctx context.Context, req resource.AuthorizeRequest) (bool, error) {
    // 超管 bypass
    if util.HasRole(req.Roles, "admin") {
        return true, nil
    }

    // 属主判断（创建者可操作）
    ticket, err := r.service.GetByID(ctx, req.ResourceID)
    if err != nil {
        return false, err
    }
    if ticket.CreatedBy == req.UserID || ticket.AssigneeID == req.UserID {
        return true, nil
    }

    // Phase 2 assigned 范围：无权限返回 false（对外 404）
    return false, nil

    // Phase 3 按需：可配置策略
    // return r.enforcer.Enforce(...)
}

func (r *TicketResource) GetFilter(ctx context.Context, userID int64, action string) (resource.Filter, error) {
    // 复杂过滤：属主 + 分配给我的 + 本部门的
    return resource.Filter{
        Where: `(creator_id = $1 OR assignee_id = $1 OR org_id IN (SELECT org_id FROM user_orgs WHERE user_id = $1))`,
        Args:  []interface{}{userID},
    }, nil
}
```

---

## 4. 自注册机制

### 4.1 Service 构造函数注册

每个 Service 在构造函数中注册自己的资源定义：

```go
// internal/service/user/service.go

func NewUserService(repo repository.UserRepo, registry resource.Registry) *UserService {
    s := &UserService{repo: repo}

    // 自注册资源
    registry.Register(NewUserResource(s))
    return s
}
```

### 4.2 Wire 注入

```go
// internal/app/wire.go

func InitializeApp() (*App, func(), error) {
    wire.Build(
        // 资源注册表（单例）
        resource.NewRegistry,

        // 各 Service（构造函数中自注册资源）
        user.NewUserService,
        role.NewRoleService,
        org.NewOrgService,
        menu.NewMenuService,
        ticket.NewTicketService,

        // Handler
        handler.NewAuthHandler,
        handler.NewUserHandler,
        // ...

        // 其他基础设施
        postgres.New,
        redis.New,
        casbin.New,
        logger.New,

        wire.Bind(new(resource.Registry), new(*resource.Registry)),
        NewApp,
    )
    return &App{}, nil, nil
}
```

### 4.3 注册顺序

Wire 按依赖图自动确定初始化顺序。资源注册表（`NewRegistry`）必须在各 Service 之前创建。由于 Service 依赖 Registry（注入），Wire 会自动保证顺序。

---

## 5. 每资源独立 Enforcer

### 5.1 策略爆炸问题

```
单个 Casbin enforcer 的策略量：
  = 角色数 × 资源类型数 × 资源实例数 × 动作数

例如：10 角色 × 5 资源类型 × 1000 资源实例 × 4 动作 = 200,000 条策略
```

### 5.2 分层 Enforcer

```
路由级 Enforcer（全局唯一）：
  策略：角色 × API 路径 × HTTP 方法
  量级：~1,000 条
  模型：RBAC，sub=角色，obj=路径，act=方法
  表：casbin_rule

资源级 Enforcer（每资源类型一个，按需创建）：
  策略：用户 × 资源ID × 动作  或  角色 × 条件 × 动作
  量级：每类资源独立，互不影响
  模型：ABAC 或 RBAC + 条件
  表：casbin_rule_{resource}（如 casbin_rule_ticket）
```

### 5.3 适用场景

| 场景 | 是否需要独立 enforcer | 原因 |
|------|---------------------|------|
| 路由级 RBAC | 不需要，全局唯一 | 策略量可控（~1000 条） |
| 资源属主判断 | 不需要 | 代码内联 `owner_id == userId`，O(1) |
| 组织关系判断 | 不需要 | SQL ltree 查询，一条 SQL 解决 |
| 可配置的资源策略 | **需要** | 管理员要能配置"编辑只能读本组文章"，且策略可变 |
| 复杂 ReBAC（多级关系链） | **需要** | 策略复杂度高，隔离避免互相影响 |

### 5.4 策略存储

每资源 enforcer 使用独立策略表：

```sql
-- 路由级策略（全局唯一）
CREATE TABLE casbin_rule (
    id    BIGSERIAL PRIMARY KEY,
    ptype VARCHAR(10) NOT NULL,
    v0    VARCHAR(255) NOT NULL,
    v1    VARCHAR(255) NOT NULL,
    v2    VARCHAR(255) DEFAULT '',
    v3    VARCHAR(255) DEFAULT '',
    v4    VARCHAR(255) DEFAULT '',
    v5    VARCHAR(255) DEFAULT ''
);

-- 工单资源级策略（按需创建）
CREATE TABLE casbin_rule_ticket (
    id    BIGSERIAL PRIMARY KEY,
    ptype VARCHAR(10) NOT NULL,
    v0    VARCHAR(255) NOT NULL,  -- user_id
    v1    VARCHAR(255) NOT NULL,  -- ticket_id
    v2    VARCHAR(255) DEFAULT '', -- action
    v3    VARCHAR(255) DEFAULT '',
    v4    VARCHAR(255) DEFAULT '',
    v5    VARCHAR(255) DEFAULT ''
);
```

---

## 6. 鉴权链路

### 6.1 完整请求流程

```
请求 POST /api/v1/tickets/delete
  │
  ├── JWT 中间件
  │   验证 token → 提取 user_id = 1001（int64）
  │
  ├── Casbin 中间件（路由级，PEP-1）
  │   判断：角色 × /api/v1/tickets/delete × POST → 通过
  │
  ├── TicketHandler
  │   body: { "id": "2001" }  // 工单业务 ID
  │   调用 Service
  │
  └── TicketService
      registry.Authorize(ctx, "ticket", {
          UserID: 1001,
          Roles: ["operator"],
          Action: "delete",
          ResourceID: "2001",
      })
      → TicketResource.Authorize
          ├─ 属主/处理人判断 → 通过
      → 执行删除
```

### 6.2 列表查询流程

```
请求 GET /api/v1/tickets?page=1&size=20
  │
  ├── JWT + Casbin 中间件（同上）
  │
  └── TicketHandler → TicketService
      filter, _ := registry.GetFilter(ctx, "ticket", 1001, "read")
      // filter.Where = "creator_id = $1 OR assignee_id = $1 OR org_id IN (...)"
      // filter.Args = []interface{}{1001}

      query := `
        SELECT * FROM tickets
        WHERE ` + filter.Where + `
        ORDER BY created_at DESC
        LIMIT $2 OFFSET $3
      `
      rows, _ := db.Query(ctx, query, filter.Args[0], 20, 0)
```

---

## 7. "鉴权服务自身也需要鉴权"

### 7.1 问题描述

用户/角色/组织/菜单等管理接口本身也是 HTTP 路由，需要鉴权。而这些 Service 又是鉴权体系的一部分（提供用户/角色数据）。这是否是循环依赖？

### 7.2 不是循环依赖

```
1. JWT 中间件：验证 token → 提取 user_id
   依赖：JWT 签名密钥 + Redis（黑名单）
   不依赖：任何业务 Service

2. Casbin 中间件：路由级鉴权
   依赖：casbin_rule 表（独立数据源）
   不依赖：User/Role/Org Service

3. Handler → Service：资源级鉴权
   依赖：业务数据表（users, tickets 等）
   这一步才需要查业务数据
```

中间件层（步骤 1-2）和 Service 层（步骤 3）是**串行但解耦**的。中间件层只读 `casbin_rule` 表，Service 层读业务表。

### 7.3 Casbin 策略表的独立性

Casbin 策略表（`casbin_rule`）是一个独立的表，由管理员通过管理接口配置。策略同步流程：

```
管理员配置角色-菜单 → 写 role_menus 表 → 触发 Casbin 策略同步
                                        → 写 casbin_rule 表（API 策略）

运行时：
  请求 → Casbin 中间件读 casbin_rule 表（只读，不查 Service）
```

策略同步在**写操作后触发**，不在请求路径上。所以请求时的 Casbin 鉴权只依赖 `casbin_rule` 表，不依赖任何 Service。

### 7.4 管理场景的鉴权边界：路由级 vs 资源级

用户提出的关键问题：新建用户、修改用户绑定的角色，这些操作怎么管控？

#### 核心原则：管理操作的路由级鉴权已足够

Phase 1 的管理操作（用户/角色/组织/菜单 CRUD）**只用路由级 Casbin 鉴权**，不需要资源级鉴权。原因：

| 操作类型 | 鉴权方式 | 理由 |
|---------|---------|------|
| 列表查询 `GET /users` | 路由级 | Casbin 策略 `p, role::admin, /api/v1/users, GET` |
| 新建用户 `POST /users` | 路由级 | 有权限的角色都能创建，不区分"创建谁" |
| 修改用户 `POST /users/update` | 路由级 + 属主/角色范围 | 见下方分析 |
| 修改用户角色 `POST /users/roles` | 路由级 + 角色范围校验 | 防止越权提权 |
| 删除用户 `POST /users/delete` | 路由级 + 系统用户保护 | 代码硬编码保护 |

#### 典型场景分析

**场景 1：新建用户**

```
POST /api/v1/users
  │
  ├── Casbin 中间件：检查 role::xxx 是否有 /api/v1/users 的 POST 权限
  │   └── 有 → 放行；无 → 403
  └── Handler → UserService.Create()
      └── 业务逻辑：校验工号/域账号唯一性、密码策略、bcrypt 加密（**username 可重复**，不作唯一校验）
```

不需要资源级鉴权——"创建用户"这个行为本身不涉及"操作某个已有资源"的权限判断。

**场景 2：修改用户绑定的角色（关键场景）**

```
POST /api/v1/users/roles
  │
  ├── Casbin 中间件：检查 role::xxx 是否有该路由的 POST 权限
  │   └── 有 → 放行
  ├── Handler → UserService.SetRoles()
  │   ├── 业务校验 1：目标用户是否存在
  │   ├── 业务校验 2：要分配的角色是否合法
  │   └── 业务校验 3（防越权）：`EffectivePriority`（越小越强）≤ 目标用户；待分配角色 `priority` ≥ 操作者
  │       ├── admin 不能给用户分配 superadmin 角色
  │       └── admin 不能修改 superadmin 用户的角色
  └── 写 user_roles 表 + 触发权限缓存失效
```

这里"防越权提权"是**业务校验**，不是 Casbin 资源级鉴权。见 [04-user §多角色与有效 priority](../phase1/04-user.md#多角色与有效-priority)。代码内联示例：

```go
func (s *UserService) SetRoles(ctx context.Context, targetUserID int64, roleCodes []string) error {
    actorP := EffectivePriority(ctx) // min(roles.priority)
    targetP := s.roleRepo.EffectivePriority(ctx, targetUserID)

    if actorP > targetP { // 数字更大 = 更弱，不能管更强的人
        return ErrPermissionDenied
    }
    for _, code := range roleCodes {
        if actorP > rolePriority(code) { // 不能分配更强角色（priority 更小）
            return ErrCannotAssignHigherRole
        }
    }
    // 执行分配...
}
```

**场景 3：删除用户**

```
POST /api/v1/users/delete
  │
  ├── Casbin 中间件：路由级权限检查
  ├── Handler → UserService.Delete()
  │   ├── 系统用户保护：is_system = true 的用户不可删除
  │   ├── 不能删除自己
  │   └── 事务：删 user_roles + user_orgs + users
  └── 审计日志
```

### 7.5 基础服务的部署策略

> 用户问：用户、角色、组织这几个 Service 是否建议一起部署？

**Phase 1：模块化单体，统一部署**

```
┌─────────────────────────────────────────┐
│              单个 Go 进程                │
│                                         │
│  HTTP Router (Gin)                      │
│    ├── /auth/*        → AuthService     │
│    ├── /users/*       → UserService     │
│    ├── /roles/*       → RoleService     │
│    ├── /orgs/*        → OrgService      │
│    └── /menus/*       → MenuService     │
│                                         │
│  共享：PostgreSQL + Redis + Casbin      │
└─────────────────────────────────────────┘
```

理由：
- Phase 1 目标是搭框架，不是微服务化
- 模块间调用是进程内函数调用，零网络开销
- Wire 管理依赖注入，模块边界清晰（`internal/service/user/`、`internal/service/role/`）
- 共享同一 DB 连接池，事务可以跨模块（如创建用户 + 分配角色）

**Phase 3 演进：按需拆分**

当某个 Service 有独立的伸缩需求时才拆分。拆分顺序：
1. 先拆业务模块（工单等），IAM 模块最后拆
2. IAM 内部拆分顺序：Org → User → Role → Auth → Menu
3. 拆分后用 gRPC 通信，Casbin 策略通过各自的 Enforcer 独立加载

### 7.6 各 Service 的鉴权策略一览

| Service | 路由级鉴权（Casbin） | 资源级鉴权 | Phase |
|---------|---------------------|-----------|-------|
| Auth（登录/刷新/登出） | 不需要（公开路由） | 不需要 | Phase 1 |
| User（用户 CRUD） | 需要 | 属主判断 + 角色范围校验（代码内联） | Phase 1 路由级 / Phase 2 资源级 |
| Role（角色 CRUD） | 需要 | is_system 保护 + priority 防提权校验（代码内联） | Phase 1 路由级 / Phase 2 资源级 |
| Organization（组织 CRUD） | 需要 | 属主判断 + ltree 路径范围（代码内联） | Phase 1 路由级 / Phase 2 资源级 |
| Menu（菜单 CRUD） | 需要 | 不需要（管理操作，路由级足够） | Phase 1 |
| Audit（审计日志查询） | 需要 | 不需要（只读，路由级足够） | Phase 1 |
| Ticket（工单） | 需要 | 属主判断 + 工单状态机 + 组织范围 | Phase 2 |

> **总结**：Phase 1 所有管理操作的路由级 Casbin 鉴权已足够。资源级鉴权（属主判断、组织范围过滤）是 Phase 2 的内容。但"防越权提权"等业务校验从 Phase 1 就需要用代码内联实现，不等 Phase 2。

---

## 8. 与旧系统的对比

| 维度 | 旧系统 zhuzhao | 新框架 |
|------|---------------|--------|
| 资源注册 | `ResourceService.Register()` 内存 map | `ResourceRegistry.Register()` 内存 map（同） |
| 资源定义 | `Resource{Code, Name, Actions, IsSystem}` | `Resource` 接口（含 Authorize + GetFilter） |
| 鉴权执行 | `RestrictService.Authorize()` 集中引擎 | 各 Resource 自己实现 Authorize（分布式） |
| 策略存储 | `restrict_policies` MongoDB 嵌套文档 | 代码内联 + 按需 casbin_rule_{resource} 表 |
| 列表过滤 | `GetFilterForResource` → MongoDB filter | `GetFilter` → SQL WHERE 子句 |
| 策略可配置 | 9 种 ConditionType + evaluator DI | 代码内联（简单）+ 独立 enforcer（复杂） |
| 缓存 | grants 内存缓存 + LRU org 缓存 | Redis 权限缓存 + Casbin 内部缓存 |

### 不采用旧系统 Restrict 引擎的理由

- **使用者少、无社区**：出了问题只能自己查
- **9 种 ConditionType 过度抽象**：大部分场景只需属主判断和组织关系
- **PostgreSQL ltree** 让组织关系查询变成一条 SQL，不需要引擎
- **代码内联更直观**：每个资源的鉴权逻辑在自己文件里，可读性好
- **按需引入 enforcer**：只有需要可配置策略时才引入 Casbin enforcer

---

## 9. 分阶段实施

### Phase 1

- 定义 `Resource` 接口和 `Registry`
- 实现 `UserResource`（属主判断）
- 各 Service 构造函数自注册
- 不引入资源级 enforcer

### Phase 2

- 实现 `RoleResource`、`OrgResource`、`MenuResource`
- 实现列表过滤（`GetFilter` → SQL WHERE）
- 为需要可配置策略的资源引入独立 enforcer
- 完善组织关系判断（ltree 查询）

### Phase 3

- 微服务化时，各服务自带 Resource 实现
- 可选引入 PDP 服务（SpiceDB / Cerbos）
- 资源接口迁移为 gRPC/HTTP API
