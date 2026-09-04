# 设计决策与细节讨论

> 本文档记录架构设计中各决策点的推理过程、方案对比和细节讨论。
> 架构层面的结论见 [architecture.md](./architecture.md)，本文档聚焦"为什么这么选"和"细节怎么处理"。
>
> **阶段编号以 [`roadmap.md`](../roadmap.md) 为准。** 下文部分段落仍写「Phase 2 微服务化 / RS256 / gRPC」，语义上对应 **Phase 3（拆服务）**；Phase 2 是单体进程内的工单，不换 RS256、不拆 IAM。

## 目录

- [1. JWT 无状态策略与权限缓存](#1-jwt-无状态策略与权限缓存)
- [2. Redis 重启与数据恢复](#2-redis-重启与数据恢复)
- [3. 三层鉴权拆分理由](#3-三层鉴权拆分理由)
- [4. 通用工具包是否独立项目](#4-通用工具包是否独立项目)
- [5. 资源级鉴权架构：Gateway 下放 vs 集中](#5-资源级鉴权架构gateway-下放-vs-集中)
- [6. 资源抽象与自注册机制](#6-资源抽象与自注册机制)
- [7. 系统重启与数据初始化幂等性](#7-系统重启与数据初始化幂等性)
- [8. Casbin 策略爆炸：每资源独立 Enforcer](#8-casbin-策略爆炸每资源独立-enforcer)
- [9. JWT 签名算法：HS256 → RS256 演进](#9-jwt-签名算法hs256-rs256-演进)
- [10. Casbin PostgreSQL Adapter 选型](#10-casbin-postgresql-adapter-选型)
- [11. 数据库高可用：PostgreSQL Cluster vs MongoDB 副本集](#11-数据库高可用postgresql-cluster-vs-mongodb-副本集)
- [12. ReBAC 引擎选型：自研 vs OpenFGA vs SpiceDB](#12-rebac-引擎选型自研-vs-openfga-vs-spicedb)
- [13. 微服务通信协议：gRPC 内部 + REST 外部](#13-微服务通信协议grpc-内部-rest-外部)
- [14. 待讨论事项](#14-待讨论事项)
- [18. 部署与代码解耦：一套代码多种部署](#18-部署与代码解耦一套代码多种部署)
- [19. RBAC 继承与级联（业界参考）](#19-rbac-继承与级联业界参考)

---

## 1. JWT 无状态策略与权限缓存

### 1.1 问题背景

JWT 签发后不可变，如果把角色、权限信息塞进 payload，管理员修改用户权限后无法实时生效——必须等 AT 过期或客户端主动 refresh。这对企业内部系统是不可接受的。

### 1.2 方案对比

| 方案 | JWT 内容 | 权限来源 | 实时性 | 性能 | 复杂度 |
|------|---------|---------|--------|------|--------|
| 权限入 JWT | user_id + role + org_id | JWT payload | 最差（等 AT 过期） | 最好（零查询） | 最低 |
| 每次查 DB | user_id | DB | 最好 | 最差（每请求一次 DB） | 低 |
| Redis 缓存（Phase 3 目标） | user_id | Redis（miss 时查 DB） | 好（主动失效） | 好（~0.1ms） | 中 |

### 1.3 目标架构（Phase 3 / 按需）

> **Phase 1 实际做法见 §1.6。** 以下为引入 Redis 权限缓存后的目标形态。

JWT 只存身份标识，权限信息走 Redis 缓存：

```
JWT payload（Phase 1 实际字段）:
{
  "uid": 1,
  "username": "admin",
  "jti": "xxx",
  "mcp": false,
  "exp": xxx
}

Redis 缓存（Phase 3）:
perm:user:{userId} → {
  "roles": ["admin", "operator"],
  "org_id": "1",
  "permissions": ["user:create", "article:read", ...]
}
TTL: 30min
```

### 1.4 权限变更实时生效（Phase 3 缓存启用后）

管理员修改用户角色时：
1. DB 事务提交（改 user_roles / role_menus）
2. `DEL perm:user:{userId}` 主动失效缓存
3. （多实例时）Pub/Sub 广播 `cache:invalidate:perm:user:{userId}`

该用户下次请求时 cache miss，从 DB 重新加载最新权限。延迟：下一次请求即生效。

**Phase 1**：无 Redis 权限缓存，Casbin 中间件每次查 `user_roles`，角色变更后**下一次请求即生效**。

### 1.5 Phase 3 缓存方案的额外优势

- **权限不入 JWT**：payload 不含 roles/permissions，变更不必等 AT 过期
- **权限缓存 miss 时查 DB**：仅适用于 **Phase 3 权限缓存**路径；**鉴权链路**（黑名单、`user:disabled`、登录限流）Redis 故障仍 **fail-close 503**
- **多角色支持自然**：Redis 存数组，不需要在 JWT 里设计序列化格式

### 1.6 实施阶段

**Phase 1（当前）**：JWT 只存身份（`uid`、`username`、`jti`、`mcp`）；路由级鉴权由 Casbin 中间件 + `RoleFetcher` 查 `user_roles`（直接角色，**无 Redis 权限缓存**）。角色/菜单变更后，用户下次请求即读到 DB 最新数据（或等 AT 过期后 refresh）。

**Phase 2b**：扩展 BFS 三源角色合并（组织角色、继承）。

**Phase 3 / 按需**：引入 `perm:user:{userId}` Redis 缓存 + Pub/Sub 失效（多实例、热点优化）。权限仍不入 JWT。

---

## 2. Redis 重启与数据恢复

### 2.1 问题背景

Redis 存储了 RT、AT 黑名单、设备列表、权限缓存、登录限流计数等数据。Redis 重启后这些数据是否丢失？用户是否需要重新登录？

### 2.2 持久化策略

| 策略 | 机制 | 重启后数据 | 数据丢失窗口 |
|------|------|-----------|-------------|
| RDB（默认） | 定时快照到磁盘 | 恢复到最近一次快照 | 最后一次快照后的数据丢失 |
| AOF | 每条写命令追加到日志 | 几乎全部恢复 | 最多丢 1 秒（`everysec`） |
| RDB + AOF（推荐） | 混合持久化 | 全部恢复 | 最多丢 1 秒 |

**推荐生产环境配置**：

```conf
save 900 1
save 300 10
save 60 10000
appendonly yes
appendfsync everysec
aof-use-rdb-preamble yes
```

### 2.3 各类数据重启后的影响

| 数据 | 丢失影响 | 严重程度 |
|------|---------|---------|
| RT 存储 `refresh:{userId}:{deviceId}` | 丢失窗口内的 RT 失效，对应用户刷新失败需重新登录 | 中（极少数用户） |
| AT 黑名单 `blacklist:at:{jti}` | 已登出的 AT 短暂可用（最多到 AT 过期 **30min**） | 低 |
| 设备列表 `devices:{userId}` | 多设备管理暂时不可用，重新登录后重建 | 低 |
| 权限缓存 `perm:user:{userId}` | **无影响**，cache miss 自动查 DB 回填 | 无 |
| 登录限流 `lock:login:{employee_no}` | 被锁定的工号短暂不可登录 | 低（最多到自然解锁） |

### 2.4 是否所有用户需要重新登录

**不需要。** AT 是无状态 JWT，校验只依赖签名和过期时间，不依赖 Redis。

- 持有有效 AT 的用户 → 正常访问（权限缓存 miss，多一次 DB 查询）
- AT 过期需要刷新的用户 → RT 可能丢失 → 刷新失败 → 需重新登录

**受影响比例估算**：假设 1000 在线用户，AT 有效期 **30min**，Redis 重启 30 秒：
- 这 30 秒内 AT 过期需刷新的用户 ≈ 1000 × (30s / 30min) ≈ **17 人**
- 仅这 17 人中 RT 在丢失窗口内的才需要重新登录

### 2.5 Redis 降级策略

**鉴权链路（JWT 黑名单、`user:disabled`、登录限流）采用 fail-close**：Redis 查询失败返回 **503**，禁止 fail-open 放行。

| 功能 | Redis 不可用时的行为 | 策略 |
|------|---------------------|------|
| AT 签名校验 | 正常（不依赖 Redis） | — |
| 黑名单 / `user:disabled` | **503** | fail-close（Phase 1 已定） |
| 登录限流 | **503** 或拒绝登录 | fail-close |
| 权限缓存（Phase 3） | 降级为查 DB | 功能正常，性能下降 |
| RT 刷新 | 失败，需重新登录 | 无法降级 |

---

## 3. 三层鉴权拆分理由

### 3.1 问题背景

架构中将鉴权分为三层：路由级 RBAC（Casbin）、资源级组织关系查询（ltree）、资源属主判断（ABAC）。为什么第二层和第三层要拆开，而不是统一处理？

### 3.2 第二层 vs 第三层的区别

| 维度 | 第二层：组织关系查询 | 第三层：资源属主判断 |
|------|---------------------|---------------------|
| 判断内容 | 用户与资源是否有**组织关系链**（如：所属组织的项目） | 资源的 **owner_id** 是否等于当前用户 |
| 数据来源 | ltree 路径遍历（org → project → resource） | 资源记录的 owner 字段 |
| 性能 | 需要 SQL 递归查询或路径遍历 | 单行比较，O(1) |
| 适用场景 | "我所在组织的项目我能不能看" | "这篇文章是不是我写的" |
| 实现方式 | 独立 SQL 查询 | 代码内联判断（加载资源后比较 owner_id） |

### 3.3 为什么不统一到 ReBAC

如果把属主判断也塞进 ReBAC：
- 每次都需要查关系表，即使判断逻辑只是简单的 `owner_id == userId`
- ReBAC 的关系遍历有性能开销，属主判断本应是 O(1)
- 语义上，"我是资源主人"和"我通过组织关系能访问资源"是两种不同的权限模型

### 3.4 执行顺序

```
请求 → 第一层 RBAC（路由级，Casbin 快速过滤）
     → 第二层 可见性（资源级，ltree 组织关系 / assigned scope）
     → 第三层 canOperate（动作权，属主判断作为输入而非短路层）
```

三层是**合取**关系（Allow ⟺ L1 ∧ L2 ∧ canOperate）：前一层拒绝则直接返回，不进入下一层。大多数请求在第一层就被拦截。

> **决策（路径 A，2026-08-26 拍板）**：L2 可见性在前，属主判断不短路 L2。属主是可见性集合的一个特例（assigned scope = 仅属主可见），不是独立短路层。详见 [phase2/11-authz-architecture-review §2](../phase2/11-authz-architecture-review.md#2-q1l2l3-执行顺序已拍板l2-可见性在前路径-a)。

---

## 4. 通用工具包是否独立项目

### 4.1 问题背景

logger、crypto、errcode、response 等工具包在后续多个微服务中都会用到。是现在就提取到独立项目，还是先放当前项目内？

### 4.2 建议

**先放当前项目内，后续按需提取。**

### 4.3 理由

| 阶段 | 策略 | 理由 |
|------|------|------|
| 当前（单服务） | 放 `internal/pkg/` 下 | 接口还在变，提前抽象会增加修改成本 |
| 2-3 个服务时 | 提取到独立 Go module | 接口趋于稳定，通过 `go.mod` replace 引用 |
| 多服务成熟后 | 独立仓库 + 语义化版本 | 正式发布，各服务按需升级 |

### 4.4 预留点

当前 `internal/pkg/` 下的包设计时注意：
- 不依赖业务 model 和 config 结构（或通过接口解耦）
- logger 已通过 `config.LogConfig` 耦合了配置结构，提取时需要改为传参
- crypto、errcode、response 无业务依赖，提取成本最低

---

## 5. 资源级鉴权架构：Gateway 下放 vs 集中

### 5.1 问题背景

最初设想：所有鉴权（路由级 + 资源级）全部在本项目（Gateway/底座）做。深入思考后提出：资源级鉴权是否应该下放到下游业务服务？API Gateway 只负责路由级校验？业界标准做法是什么？

### 5.2 业界标准：分层鉴权 + PEP/PDP 模式

OWASP、NIST SP 800-204B、Cerbos、AuthZed 一致推荐的模式：

```
请求 → API Gateway（PEP-1：粗粒度）
     │  认证：验证 JWT，提取用户身份
     │  路由级鉴权：这个用户能访问这个接口吗？（角色/scope 检查）
     │  限流、CORS、安全头
     │
     → Microservice（PEP-2：细粒度）
        资源级鉴权：这个用户能操作这条具体数据吗？
        │
        ├─ 方式 A：代码内联判断（简单场景）
        ├─ 方式 B：调用 PDP 服务（复杂场景）
        └─ 方式 C：嵌入式策略库（折中）
```

**NIST 三角色模型**：
- **PAP**（Policy Administration Point）：管理员配置策略
- **PDP**（Policy Decision Point）：评估策略，返回 allow/deny
- **PEP**（Policy Enforcement Point）：执行决策（Gateway 或 Service 代码）

### 5.3 为什么 Gateway 不做资源级鉴权

OWASP 明确指出：

> "Gateway-only authorization cannot safely handle object-level decisions, internal east-west traffic, or service-specific business logic."

原因：**Gateway 没有业务数据**。

```
POST /api/v1/users/delete  (body: {id: "U001"})

Gateway 层：
  - 能判断：用户是否有 "user:delete" 权限 → ✅
  - 不能判断：用户是否是 U001 的组织管理员 → ❌（需要查 DB）
  - 不能判断：U001 是否是系统用户 → ❌（需要查 DB）
  - 不能判断：U001 是否在删除者的子组织中 → ❌（需要查组织树）
```

如果硬要把资源级鉴权放到 Gateway，Gateway 就需要访问所有业务数据库，变成一个超级单体——违背微服务的初衷。

### 5.4 各方案对比

| 方案 | 新依赖 | Phase 1 适用 | 可配置 | 社区 | 决策 |
|------|--------|-------------|--------|------|------|
| Casbin ABAC | 无 | 部分适用（属主） | 部分 | 大 | 不单独用 |
| SpiceDB（Zanzibar） | 独立服务 | 过重 | 完全 | 大 | Phase 2 评估 |
| Warden | 嵌入库 | 可用 | 完全 | 小 | 不采用 |
| 自研 ConditionType | 无 | 可用 | 完全 | 无 | 不采用 |
| **代码内联 + SQL** | **无** | **适用** | **否** | **大(Casbin)** | **Phase 1 采用** |

### 5.5 采用方案：分层 + 分阶段

**Phase 1（当前，单体底座）**：

```
本项目（单体底座）:
  ├─ 中间件层：认证（JWT）+ 路由级鉴权（Casbin）
  └─ Service/Handler 层：资源级鉴权（代码内联判断）
      ├─ 属主判断：if userId == resource.CreatedBy
      ├─ 组织关系：SQL ltree 查询
      └─ 超管 bypass：if hasRole(roles, "ROLE_ADMIN")
```

- 路由级：Casbin RBAC（已用 Casbin）
- 资源级：代码内联 + PostgreSQL ltree SQL 查询
- 列表过滤：SQL WHERE 子句内联
- 零新依赖，代码分层上明确隔离 Gateway 逻辑和 Service 逻辑

**Phase 2（未来微服务化时）**：

```
API Gateway:
  - 认证 + Casbin 路由级鉴权
  - 传递 user_id + roles 到下游（通过 JWT 或 header）

各微服务:
  - 接收 user_id + roles
  - 自己做资源级鉴权
  - 可选：调用 PDP 服务（如果策略复杂）

可选 PDP:
  - SpiceDB（ReBAC 场景复杂时）
  - 或继续用代码内联（如果逻辑简单）
```

### 5.6 不采用自研 ConditionType 引擎的理由

现有系统 zhuzhao 的 9 种 ConditionType 设计虽然完整，但新框架不采用：

- **使用者少、无社区**：出了问题只能自己查
- **Phase 1 场景简单**：代码内联足够，不需要引擎
- **PostgreSQL ltree** 让组织关系查询变成一条 SQL，不需要引擎
- **未来需要策略可配置时**：可引入 JSONB 策略表 + evaluator 或 PDP 服务

### 5.7 关键原则

1. **Gateway 只做认证 + 路由级鉴权**——不做资源级判断
2. **资源级鉴权在 Service 层**——代码内联，逻辑清晰
3. **代码分层隔离**——为未来微服务化做准备
4. **不过度设计**——Phase 1 不引入 PDP，Phase 2 按需评估

---

## 6. 资源抽象与自注册机制

### 6.1 问题背景

用户、组织、角色、菜单、Casbin 策略，以及工单等业务服务，都需要资源级鉴权。如果每种资源的鉴权逻辑散落在各处，维护成本高且容易遗漏。借鉴现有系统 zhuzhao 的 `ResourceService` 自注册机制，将每种资源抽象为统一接口，各自实现鉴权策略并统一注册。

同时，这些服务自身也有对外管理接口（如管理员管理用户、角色、组织、菜单），这些接口也需要路由级 + 资源级鉴权——形成"鉴权服务自身也需要鉴权"的场景。这引出服务部署和依赖关系的设计问题。

### 6.2 核心场景

**目标系统**：通用办公管理后台。

- 每个部门/虚拟组织有对应权限
- 权限管理达到资源级检查
- 每种服务（用户、组织、角色、Casbin 策略、工单等）看作一种"资源"
- 每种资源的通用能力抽象为接口，各自实现并统一注册

### 6.3 资源接口抽象

```go
// Resource 接口：每种资源类型实现此接口
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

// ResourceRegistry 资源注册表
type ResourceRegistry interface {
    Register(res Resource)
    Get(code string) (Resource, bool)
    List() []Resource
    Authorize(ctx context.Context, resourceCode string, req AuthorizeRequest) (bool, error)
}
```

### 6.4 自注册机制

每个 Service 在构造函数中注册自己的资源定义，借鉴现有系统的模式：

```go
// internal/service/user/service.go
func NewUserService(repo repository.UserRepo, registry resource.Registry) *UserService {
    s := &UserService{repo: repo}

    // 自注册资源
    registry.Register(&UserResource{service: s})
    return s
}

// internal/service/user/resource.go
type UserResource struct {
    service *UserService
}

func (r *UserResource) Code() string { return "user" }
func (r *UserResource) Actions() []string { return []string{"create", "read", "update", "delete"} }

func (r *UserResource) Authorize(ctx context.Context, req resource.AuthorizeRequest) (bool, error) {
    // 超管 bypass
    if hasRole(req.Roles, "admin") {
        return true, nil
    }
    // 属主判断
    user, err := r.service.GetByID(ctx, req.ResourceID)
    if err != nil {
        return false, err
    }
    return user.CreatedBy == req.UserID, nil
}
```

### 6.5 资源注册表与鉴权链路

```
请求 → JWT 中间件（认证）
     → Casbin 中间件（路由级鉴权）
     → Handler
        → registry.Authorize(ctx, "user", req)
           → UserResource.Authorize(ctx, req)
              ├─ 超管 bypass
              ├─ 属主判断
              └─ 组织关系判断（SQL ltree）
```

### 6.6 "鉴权服务自身也需要鉴权"问题

用户/角色/组织/菜单等管理接口本身也是 HTTP 路由，经过同一套 Casbin 中间件做路由级鉴权。这不是循环依赖，而是**分层执行**：

```
1. JWT 中间件：验证 token → 提取 user_id（不依赖任何业务数据）
2. Casbin 中间件：路由级鉴权（查 Casbin 策略表，不依赖业务 Service）
3. Handler → Service：资源级鉴权（查业务数据）
```

Casbin 策略表是独立的数据源（`casbin_rule` 表），不依赖 User/Org Service。中间件层和 Service 层是**串行但解耦**的：中间件层只读 Casbin 策略表，Service 层读业务表。

### 6.7 服务部署策略

**Phase 1（单体底座，当前）**：所有服务在一个进程内，通过 Wire DI 注入。资源注册表是内存级别的单例，启动时各 Service 构造函数自注册。

```
单体进程：
  ├─ middleware（JWT + Casbin）
  ├─ ResourceRegistry（内存注册表）
  ├─ UserService → 注册 UserResource
  ├─ RoleService → 注册 RoleResource
  ├─ OrgService → 注册 OrgResource
  ├─ MenuService → 注册 MenuResource
  └─ TicketService → 注册 TicketResource
```

**Phase 2（微服务化时）**：有两种演进路径：

| 路径 | 描述 | 优点 | 缺点 |
|------|------|------|------|
| A：Auth 底座独立部署 | 用户/角色/组织/菜单/Casbin 作为一个 IAM 服务部署，业务服务（工单等）独立部署 | IAM 职责清晰，业务服务解耦 | 业务服务仍需调用 IAM 获取用户/组织信息 |
| B：全部拆分 | 每个服务独立部署 | 完全解耦 | 服务间调用复杂，需要 PDP 或数据复制 |

**推荐路径 A**：IAM 底座作为共享服务，提供用户/组织/角色/菜单的管理和查询能力。业务服务通过 API 或 gRPC 调用 IAM 获取身份信息，自己做资源级鉴权。

### 6.8 关键设计原则

1. **资源接口统一**：所有需要资源级鉴权的 Service 实现 `Resource` 接口
2. **自注册**：Service 构造函数注册资源定义，不改注册中心代码（开闭原则）
3. **注册表是内存态**：Phase 1 不持久化资源定义到 DB（代码即定义），Phase 2 按需评估
4. **管理接口走同一套中间件**：用户/角色/组织的管理接口和业务接口一样过 JWT + Casbin
5. **Casbin 策略表独立**：中间件层只读 `casbin_rule` 表，不依赖业务 Service，无循环依赖

---

## 7. 系统重启与数据初始化幂等性

### 7.1 问题背景

现有系统 zhuzhao 存在一个问题：每次重启时执行初始化逻辑（如创建 admin 用户），会导致 admin 用户的 `created_by`、`created_at` 等时间戳被更新，覆盖原始创建信息。这类"初始化不幂等"的问题需要在新框架中规避。

### 7.2 问题根因

现有系统的 `App.Run()` 在启动时执行 `normal.Sync()` + `httpSvcs.Sync()` 预加载元数据。如果初始化逻辑用"先删后建"或"无条件 upsert"，就会覆盖已有数据。

### 7.3 幂等性原则

**初始化操作必须是幂等的**：无论执行多少次，结果一致。已有数据不被修改。

| 操作类型 | 幂等做法 | 非幂等做法（避免） |
|---------|---------|-------------------|
| 创建 admin 用户 | `INSERT ... ON CONFLICT DO NOTHING` | 先 DELETE 再 INSERT |
| 创建 admin 角色 | 查询不存在才插入 | 无条件 upsert（覆盖时间戳） |
| 创建初始菜单 | 按 code 查询，不存在才插入 | 每次 upsert 全量覆盖 |
| Casbin 策略同步 | `INSERT ... ON CONFLICT DO NOTHING` | 先清空再批量插入 |

### 7.4 初始化分层

初始化分为三个层次，职责和执行时机不同：

```
层次 1：Schema 迁移（golang-migrate，显式执行）
  ├─ 建表、索引、外键
  └─ 时机：部署前手动执行或 CI/CD 自动执行
  └─ 幂等性：golang-migrate 自身保证（版本号追踪）

层次 2：种子数据（migration 文件，随 Schema 一起执行）
  ├─ 4 系统角色、admin 用户（绑定 superadmin）、初始菜单、初始组织
  └─ 时机：migration 执行时
  └─ 幂等性：INSERT ... ON CONFLICT DO NOTHING

层次 3：运行时 Sync（应用启动时，可选）
  ├─ Casbin 策略同步（从角色-菜单关系生成 API 策略）
  ├─ 系统资源保护标记（is_system）
  └─ 时机：App.Run() 启动时
  └─ 幂等性：desired-state sync（更新已存在、删除孤立，但保留 created_at/created_by）
```

### 7.5 种子数据 migration 设计

种子数据放在独立 migration 文件中，用 `ON CONFLICT DO NOTHING` 保证幂等。完整示例见 [data-init.md](../proposal/data-init.md) §4.2。

```sql
-- migrations/000002_seed.up.sql（节选）

-- 角色（已存在则跳过，不覆盖；主键 BIGSERIAL 自增）
INSERT INTO roles (code, name, description, is_system) VALUES
  ('superadmin', '超级管理员', '系统最高权限', true),
  ('admin', '管理员', '系统管理员', true)
ON CONFLICT (code) DO NOTHING;

-- admin 用户（已存在则跳过；登录用工号 E000001）
INSERT INTO users (username, employee_no, password, real_name, status, is_system, tenant_id)
SELECT 'admin', 'E000001', '$2a$12$xxxxx', '系统管理员', 1, true, 1
WHERE NOT EXISTS (
  SELECT 1 FROM users WHERE employee_no = 'E000001'
);

-- admin 用户关联 superadmin 角色（用 code/username 解析 id，避免硬编码 UUID）
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id FROM users u, roles r
WHERE u.username = 'admin' AND r.code = 'superadmin'
ON CONFLICT (user_id, role_id) DO NOTHING;
```

### 7.6 运行时 Sync 的安全规则

对于应用启动时需要同步的数据（如 Casbin 策略），遵循以下规则：

1. **保留审计字段**：`created_at`、`created_by` 永远不覆盖。upsert 只更新业务字段。
2. **desired-state 模式**：对比期望状态与实际状态，只更新差异部分。
3. **系统资源标记**：`is_system = true` 的记录在 Sync 中受保护，不被删除。
4. **失败不阻塞**：Sync 失败仅 Warn 日志，不阻塞启动（业务可用性优先）。

```go
// 伪代码：安全的 Sync 实现
func (s *RoleService) Sync(ctx context.Context) error {
    desired := s.getDesiredRoles() // 代码中定义的系统角色
    actual, err := s.repo.FindSystemRoles(ctx)
    if err != nil {
        return err
    }

    for _, d := range desired {
        a, exists := findByCode(actual, d.Code)
        if !exists {
            // 不存在 → 插入
            s.repo.Insert(ctx, d)
        } else {
            // 已存在 → 只更新 name/description，不碰 created_at/created_by
            s.repo.UpdateSystemFields(ctx, a.ID, d.Name, d.Description)
        }
    }
    // 孤立的系统角色（is_system=true 但 desired 中没有）→ 保留不删（安全策略）
    return nil
}
```

### 7.7 关键原则

1. **Schema 迁移和种子数据分离**：建表和初始数据是不同的 migration 文件
2. **种子数据用 `ON CONFLICT DO NOTHING`**：不覆盖已有记录
3. **运行时 Sync 保留审计字段**：`created_at`/`created_by` 永远不覆盖
4. **系统资源不删除**：`is_system = true` 的记录在 Sync 中不被清理
5. **Sync 失败不阻塞启动**：仅 Warn 日志

---

## 8. Casbin 策略爆炸：每资源独立 Enforcer

### 8.1 问题背景

现有系统使用 Restrict 的一个重要原因就是避免 Casbin 策略爆炸。如果所有资源级权限都塞进 Casbin，策略量 = 角色数 × 资源数 × 动作数，可能达到数万甚至数十万条。

用户提出：可以接受每个资源有自己的 enforcer，即每个资源类型一个独立的 Casbin enforcer 实例，各自管理自己的策略空间。

### 8.2 策略爆炸的根因

```
单个 Casbin enforcer 的策略量：
  = 角色数 × 资源类型数 × 每种资源的实例数 × 动作数

例如：10 角色 × 5 资源类型 × 1000 资源实例 × 4 动作 = 200,000 条策略
```

即使只做路由级 RBAC（角色 × API 路径 × 方法），策略量可控（~1000 条）。但一旦要做资源级，策略量爆炸。

### 8.3 方案：每资源独立 Enforcer

将 Casbin 的使用分为两层：

```
路由级 Enforcer（全局唯一）：
  策略：角色 × API 路径 × HTTP 方法
  量级：~1,000 条
  模型：RBAC，sub=角色，obj=路径，act=方法

资源级 Enforcer（每资源类型一个）：
  策略：角色 × 资源ID × 动作  或  角色 × 条件 × 动作
  量级：每类资源独立，互不影响
  模型：ABAC 或 RBAC + 条件
```

### 8.4 每资源 Enforcer 的适用场景

| 场景 | 是否需要独立 enforcer | 原因 |
|------|---------------------|------|
| 路由级 RBAC | 不需要，全局唯一 | 策略量可控 |
| 资源属主判断 | 不需要 | 代码内联 `owner_id == userId`，O(1) |
| 组织关系判断 | 不需要 | SQL ltree 查询，一条 SQL 解决 |
| 可配置的资源策略 | **需要** | 管理员要能配置"编辑只能读本组文章"，且策略可变 |
| 复杂 ReBAC（多级关系链） | **需要** | 策略复杂度高，隔离避免互相影响 |

### 8.5 与资源抽象的关系

每资源 Enforcer 与第 6 节的资源抽象机制结合：每个 `Resource` 实现可以选择是否使用 Casbin enforcer。

```go
type Resource interface {
    Code() string
    Actions() []string
    Authorize(ctx context.Context, req AuthorizeRequest) (bool, error)
}

// 简单资源：代码内联判断，不需要 Casbin
type UserResource struct { ... }
func (r *UserResource) Authorize(ctx, req) (bool, error) {
    if hasRole(req.Roles, "admin") { return true, nil }
    user, _ := r.service.GetByID(ctx, req.ResourceID)
    return user.CreatedBy == req.UserID, nil
}

// 复杂资源：使用独立 Casbin enforcer
type TicketResource struct {
    enforcer *casbin.SyncedEnforcer // Ticket 专属 enforcer
    ...
}
func (r *TicketResource) Authorize(ctx, req) (bool, error) {
    if hasRole(req.Roles, "admin") { return true, nil }
    // 查 Ticket 专属策略
    return r.enforcer.Enforce(req.UserID, req.ResourceID, req.Action)
}
```

### 8.6 策略存储

每资源 enforcer 的策略存储方案：

```
方案 A：共享 casbin_rule 表 + ptype 字段区分
  casbin_rule 表增加 ptype 字段（"route" / "ticket" / "doc"）
  每个 enforcer 查询时过滤 ptype
  优点：一张表管理
  缺点：大表索引压力

方案 B：每资源独立表
  casbin_rule_route    （路由级策略）
  casbin_rule_ticket   （工单策略）
  casbin_rule_doc      （文档策略）
  每个 enforcer 对应一张表
  优点：隔离性好，单表数据量小
  缺点：表数多，管理稍复杂
```

**推荐方案 B**：每资源独立表。路由级策略量小且稳定（~1000 条），资源级策略可能增长，隔离存储避免互相影响。

### 8.7 Phase 1 vs Phase 2

**Phase 1**：大部分资源用代码内联判断（属主、组织关系），不引入资源级 Casbin enforcer。只有当某类资源的策略需要管理员可配置时，才为其创建独立 enforcer。

**Phase 2**：按需为复杂资源引入独立 enforcer，或引入 PDP 服务（SpiceDB / Cerbos）统一管理。

### 8.8 关键原则

1. **路由级 Casbin 全局唯一**——所有 API 路由鉴权共用一个 enforcer
2. **资源级按需引入**——简单资源用代码内联，复杂资源用独立 enforcer
3. **每资源 enforcer 独立存储**——避免策略表互相影响
4. **资源接口统一**——无论用 Casbin 还是代码内联，对外都实现 `Resource` 接口
5. **避免策略爆炸**——不在单个 enforcer 中塞所有资源级策略

---

## 9. JWT 签名算法：HS256 → RS256 演进

### 9.1 问题背景

旧系统用 RSA 4096 非对称签名。新框架选 HS256 还是 RS256？

### 9.2 业界共识（2026）

| 算法 | 类型 | 适用场景 | 验签方能否伪造 Token |
|------|------|---------|-------------------|
| HS256 | 对称（共享 secret） | 单体/同一信任域 | **能**（持有 secret 即可签发） |
| RS256 | 非对称（RSA 私钥签发/公钥验签） | 微服务/跨信任域 | **不能**（公钥只能验签） |
| ES256 | 非对称（ECDSA） | 现代项目 | **不能**（更小更快） |

业界一致结论：**单体用 HS256，微服务用 RS256/ES256**。

### 9.3 决策

**Phase 1（单体底座）：HS256**

- 签发和验签在同一个进程内，不存在 secret 分发问题
- 实现最简单，配置一个 secret 即可
- 性能最好（HMAC 微秒级 vs RSA 毫秒级）

**Phase 3（拆服务时）：切换 RS256**

- Auth 服务持有私钥签发 Token
- 业务服务通过 JWKS endpoint 获取公钥验签
- 业务服务被攻破也不能伪造 Token（blast radius 小）
- 密钥轮换通过 JWKS overlap window 实现

### 9.4 代码预留

JWT Manager 接口设计支持算法切换：

```go
type JWTManager struct {
    method jwt.SigningMethod // Phase 1: HS256; Phase 3 拆服务: RS256
    key    interface{}       // Phase 1: []byte; Phase 2: *rsa.PrivateKey
}

// Phase 1: HS256
func NewHS256Manager(secret string) *JWTManager {
    return &JWTManager{
        method: jwt.SigningMethodHS256,
        key:    []byte(secret),
    }
}

// Phase 3: RS256（拆 IAM 时）
func NewRS256Manager(privateKey *rsa.PrivateKey) *JWTManager {
    return &JWTManager{
        method: jwt.SigningMethodRS256,
        key:    privateKey,
    }
}
```

**安全要求**：验签时必须显式 pin 算法，防止 key-confusion 攻击（攻击者用 HS256 算法 + RSA 公钥伪造 Token）。

### 9.5 关键原则

1. **Phase 1 用 HS256**——单体场景最简方案，secret 存环境变量
2. **Phase 3 切换 RS256**——拆服务时必须切换，公钥通过 JWKS 分发
3. **显式 pin 算法**——验签时强制校验 `alg` 字段，防 key-confusion
4. **接口预留**——JWTManager 设计时支持算法切换，切换时只改构造函数

---

## 10. Casbin PostgreSQL Adapter 选型

### 10.1 问题背景

Phase 1 骨架阶段用内存 Adapter（`memdb`），生产环境必须持久化到 PostgreSQL。需要选定一个社区维护、兼容 Casbin v3 + pgx v5 的 Adapter。

### 10.2 候选方案对比（2026-08 调研）

| Adapter | Casbin 版本 | pgx 版本 | pgxpool | Batch | Filtered | Updatable | Stars | 维护状态 |
|---------|------------|---------|---------|-------|----------|-----------|-------|---------|
| `pckhoi/casbin-pgx-adapter/v3` | v2 | v5 | ❌（单 Conn） | ❌ | ✅ | ❌ | ~150 | 停滞（v3.2.0 2024-08） |
| `noho-digital/casbin-pgx-adapter` | **v3** | **v5** | ✅ | ✅ | ✅ | ✅ | 新 | 活跃（v1.2.2 2026-07） |
| `onlyin32bit/casbin-pgx-pgxpool-adapter` | v3 | v5 | ✅ | ✅ | ❌ | ✅ | 3 | 新（2026-02） |
| 官方 `casbin/gorm-adapter` | v2 | GORM（非 pgx 原生） | ✅ | ✅ | ✅ | ✅ | ~600 | 活跃 |

### 10.3 决策

**选型：`noho-digital/casbin-pgx-adapter`（Casbin v3 + pgx v5）**
- SSOT：[phase1/03-authz.md](../phase1/03-authz.md)、[phase1/01-infra.md](../phase1/01-infra.md)
- 兼容 Casbin v3（API 与 v2 基本一致，迁移成本低）
- 兼容 pgx v5（项目当前版本）
- 支持 pgxpool（通过 `NewAdapterWithPool` 传入现有连接池）
- 支持 Batch + Filtered + Updatable，功能最全
- 活跃维护（v1.2.2 2026-07）

**为什么不选 GORM Adapter**：
- 项目 DB 层用 pgx 原生，引入 GORM 会多一层 ORM 抽象
- GORM 对 pgx 的 ltree、JSONB 等 PostgreSQL 特性支持不如原生
- 保持技术栈一致性

### 10.4 与"每资源独立 Enforcer"的兼容性

每个资源独立 Enforcer 意味着多个 Enforcer 实例共享同一个数据库。方案：

```sql
-- 所有 Enforcer 共用一张 casbin_rule 表，用 v0 字段区分资源类型
CREATE TABLE casbin_rule (
    id    BIGSERIAL PRIMARY KEY,
    ptype VARCHAR(100),  -- "p" 或 "g"
    v0    VARCHAR(100),  -- 资源类型标识（如 "user", "ticket"）
    v1    VARCHAR(100),  -- sub（角色）
    v2    VARCHAR(100),  -- obj（路径）
    v3    VARCHAR(100),  -- act（方法）
    v4    VARCHAR(100),
    v5    VARCHAR(100)
);
```

每个 Enforcer 初始化时用 `FilteredAdapter` 按 `v0` 过滤加载自己的策略，互不干扰。

### 10.5 SyncedEnforcer 兼容性

`noho-digital/casbin-pgx-adapter` 实现了标准 `Adapter` 接口（含 Context/Batch/Filtered/Updatable），可直接传入 `casbin.NewSyncedEnforcer(model, adapter)`。`SyncedEnforcer` 内部加锁保证并发安全，与 Adapter 无冲突。

---

## 11. 数据库高可用：PostgreSQL Cluster vs MongoDB 副本集

### 11.1 背景

数据库选型必须在项目初期确定——数据库迁移成本极高。需要综合考虑高可用、事务、查询能力等因素。

### 11.2 可选方案

| 维度 | PostgreSQL | MongoDB |
|------|-----------|---------|
| 云服务版本 | PG 15 | 4.4.13 |
| 云服务高可用架构 | Cluster（2 服务器 + 1 VIP） | 3 节点副本集 |
| failover 机制 | VIP 切换 + replica 提升 | 原生 Raft 选举 |
| failover 速度 | 秒级（VIP 漂移 + PG promote） | 10 秒内（Raft 选举） |
| 写一致性 | `synchronous_standby_names` | `w: majority` |
| root 权限 | 不需要（云托管） | 不需要（云托管） |

### 11.3 决策：PostgreSQL

**Phase 1**：自建单节点 PG 15（开发/小规模生产）。
**Phase 2**：迁移到云托管 PG Cluster（2 服务器 + VIP），高可用由云服务管理。

### 11.4 选 PostgreSQL 的理由

1. **ltree 不可替代**——组织树是本系统核心能力。PG ltree 一条 SQL 做层级查询/祖先判断；MongoDB 无等价物，需物化路径 + 应用层递归。
2. **强 ACID 事务**——user、user_roles、user_orgs 多表关联更新需原子性。MongoDB 4.0+ 支持多文档事务但有性能开销和 16MB 限制。
3. **资源级列表过滤**——"查出当前用户能看到的所有工单"用 PG 直接 `WHERE org_path @> user_org_path`；MongoDB 需先查路径再 `$in` 查询，多一轮交互。
4. **数据完整性**——外键、唯一约束、CHECK 约束由 DB 保证；MongoDB 依赖应用层。
5. **Casbin Adapter 兼容**——`noho-digital/casbin-pgx-adapter` 兼容 Casbin v3 + pgx v5（详见 §10）。
6. **当前投入已基于 PG**——所有设计文档、Schema、代码骨架均基于 PG，切换成本极高。

### 11.5 MongoDB 的优势及应对

| MongoDB 优势 | 严重程度 | PG 应对方案 |
|-------------|---------|-----------|
| TTL 索引自动过期日志 | 低 | PG 分区表（`pg_partman`）或定时清理 cron |
| 副本集高可用更简单 | 低 | 云托管 PG Cluster 的 VIP 方案对办公后台 QPS 足够 |
| Schema 灵活加字段快 | 低 | PG 加列（`ALTER TABLE ADD COLUMN`）也是零阻塞操作 |
| 旧系统已有 MongoDB 经验 | 中 | PG 生态成熟，学习曲线平缓 |

### 11.6 PG Cluster 高可用说明

云托管 PG Cluster（2 服务器 + 1 VIP）的工作方式：
- 主节点（Primary）+ 副本节点（Replica），通过流复制同步
- VIP 挂在 Primary 上，客户端连接 VIP
- Primary 故障 → 云服务将 VIP 漂移到 Replica → Replica 提升为 Primary
- 应用层通过 VIP 连接，无需感知切换（`pgxpool` 自动重连）

**应用层注意事项**：
- 连接池配置自动重连（`pgxpool` 已支持）
- failover 期间（秒级）会有短暂连接错误，需 retry 中间件
- 使用 `pgxpool` 而非单连接，确保连接恢复

---

## 12. ReBAC 引擎选型：自研 vs OpenFGA vs SpiceDB

### 12.1 问题背景

当前文档中"自研 ReBAC"的表述不准确。实际实现只是基于 PostgreSQL `ltree` 的组织关系 SQL 查询，不是 ReBAC 引擎。需要明确各阶段的资源级鉴权实现方案。

### 12.2 当前"自研"方案的真实定位

当前方案做的是**一条 SQL 查询**，不是 ReBAC 引擎。示例使用 **`resource_owners` 泛化表（Phase 2b 可选）**；Phase **2a 工单**直接用 `tickets.org_id/org_path`（见 [phase2/02-authz-resource.md](../phase2/02-authz-resource.md)）。

```sql
-- 用户 → 所属组织 → ltree 路径遍历 → 是否到达资源所属组织
SELECT EXISTS (
    SELECT 1 FROM user_orgs uo
    JOIN organizations uo_org ON uo.org_id = uo_org.id
    JOIN resource_owners ro ON ro.resource_type = $1 AND ro.resource_id = $2
    JOIN organizations res_org ON ro.org_id = res_org.id
    WHERE uo.user_id = $5
        AND uo_org.path @> res_org.path  -- ltree 祖先判断
) AS has_access;
```

它**没有**：关系图存储与遍历、任意深度关系链解析、一致性保证、通用 schema 定义语言。

### 12.3 业界开源 ReBAC 引擎（2026）

Google Zanzibar 论文的开源实现已成熟：

| 引擎 | 背景 | 特点 | 生产案例 |
|------|------|------|---------|
| **SpiceDB** (AuthZed) | 最忠实于 Zanzibar | gRPC 优先、ZedToken 一致性、Caveats 条件权限 | OpenAI ChatGPT Enterprise（百亿级关系） |
| **OpenFGA** (Auth0/Okta → CNCF) | CNCF 毕业 | REST 优先、DSL 可读、多语言 SDK | Okta 生态 |
| **Permify** | 开发者友好 | YAML schema、内置数据过滤、可视化 | 中小团队 |

核心模型是 **relationship tuple**：

```
user:alice —editor—→ document:doc1
document:doc1 —parent—→ folder:folder1
folder:folder1 —viewer—→ group:engineering
group:engineering —member—→ user:bob
```

一次 `Check` 请求解析整个关系链：bob 能否 view doc1？→ bob 是 engineering 成员 → engineering 是 folder1 viewer → folder1 是 doc1 parent → ✅ 允许。

### 12.4 方案对比

| 能力 | Phase 1 代码内联 + ltree | OpenFGA/SpiceDB |
|------|----------------------|-----------------|
| 组织树关系遍历 | ✅ ltree SQL | ✅ 原生支持 |
| 虚拟组成员关系 | ❌ 需额外 SQL | ✅ relationship tuple |
| 资源属主判断 | ✅ 代码内联 | ✅ `owner` relation |
| 任意深度关系链 | ❌ 只支持组织树 | ✅ 任意关系图 |
| 列表过滤 | ❌ 手写 SQL WHERE | ✅ `ListObjects` API |
| 多资源类型扩展 | ❌ 每种资源手写 | ✅ schema 定义 |
| 一致性保证 | ❌ 无 | ✅ ZedToken/Zookie |
| 运维复杂度 | 低（只有 PG） | 中（独立服务部署） |
| 新依赖 | 无 | 独立服务 + 数据存储 |

### 12.5 决策：分阶段演进

**Phase 1：代码内联 + ltree SQL（零新依赖）**

不引入 ReBAC 引擎。文档中不再叫"自研 ReBAC"，准确描述为"基于 ltree 的组织关系查询 + 代码内联鉴权"。

适用条件：
- 资源类型少（User、Org、Ticket）
- 关系链浅（用户 → 组织 → 资源，不超过 3 层）
- 策略不需要运行时可配置

**Phase 2：评估引入 OpenFGA 或 SpiceDB（按需）**

引入时机信号：
- 资源类型增多（>5 种），每种手写鉴权代码维护成本上升
- 关系链变深（跨资源嵌套，如 工单 → 项目 → 部门 → 公司）
- 需要策略运行时可配置（管理员在 UI 配置权限规则）
- 微服务化后需要统一 PDP

**选型倾向**：
- **OpenFGA**——CNCF 项目、REST 优先、Go SDK 完善、社区活跃，适合优先评估
- **SpiceDB**——如果需要 Zanzibar 级一致性保证（ZedToken）或百亿级关系

### 12.6 Phase 1 到 Phase 2 的迁移路径

Phase 1 的代码设计为未来迁移预留：

```go
// Phase 1：接口定义
type ResourceAuthorizer interface {
    Check(ctx context.Context, userID, resType, resID, action string) (bool, error)
    ListFilter(ctx context.Context, userID, resType, action string) (sql.Filter, error)
}

// Phase 1：本地实现（ltree SQL + 代码内联）
type localAuthorizer struct {
    db *pgxpool.Pool
}

// Phase 2：远程实现（替换接口，调用方无感知）
type remoteAuthorizer struct {
    client openfga.Client  // 或 spicedb.Client
}
```

### 12.7 文档修正

全文将"自研 ReBAC"修正为准确描述：
- `architecture.md`：§4.3 "第二层：资源级 ReBAC（自研关系遍历）"→ "第二层：资源级鉴权（ltree 组织关系查询）"
- `overview.md`：技术选型表"资源级鉴权"描述更新
- `system-comparison.md`：#3 相关描述更新

---

## 13. 微服务通信协议：gRPC 内部 + REST 外部

### 13.1 业界共识（2026）

2026 年业界高度收敛到**双协议栈**模式（Netflix、Google、Lyft、Dropbox 共同实践）：

- **North-South**（客户端 → Gateway）：REST/JSON，浏览器原生支持
- **East-West**（服务 ↔ 服务）：gRPC/Protobuf，二进制序列化小 3-10 倍，延迟低 2-5 倍

### 13.2 各阶段通信方案

| 阶段 | 对外 API | 内部通信 | 异步事件 |
|------|---------|---------|---------|
| Phase 1（单体） | REST（Gin） | 进程内接口调用 | 内存 channel |
| Phase 2（IAM 独立） | REST（Gin + gRPC-Gateway） | **gRPC** | Redis Pub/Sub |
| Phase 3（微服务化） | REST（Gateway） | **gRPC** | Kafka |

### 13.3 Phase 2 通信架构

```
客户端
  │ REST/JSON
  ▼
API Gateway（Gin + gRPC-Gateway）
  │
  │ gRPC/Protobuf（内部网络）
  ├──▶ IAM 服务（gRPC server）
  │      GetUser / GetUserRoles / GetUserOrgs / GetUserPermissions
  │
  └──▶ 业务服务（gRPC server）
         业务服务通过 gRPC client 调用 IAM 获取身份信息
```

### 13.4 选 gRPC 的理由

1. **高频 East-West 流量**——每个业务请求可能需要查用户角色/权限，gRPC 持久连接 + Protobuf 比 HTTP/JSON 快 3-5 倍
2. **强类型契约**——`.proto` 文件生成代码，比手写 HTTP client 安全
3. **Go 生态完善**——gRPC 原生支持 Go，interceptor 中间件（认证、日志、熔断）成熟
4. **Gateway 转换**——对外仍用 REST，gRPC-Gateway 从 `.proto` 自动生成 REST 代理

### 13.5 同步 vs 异步

| 通信类型 | 协议 | 场景 |
|---------|------|------|
| 同步查询 | gRPC | 业务服务实时查询 IAM 用户角色/权限 |
| 异步事件 | Redis Pub/Sub → Kafka | IAM 角色变更通知业务服务更新本地缓存 |
| 数据复制 | 事件驱动 CQRS | IAM 发布 `user.role.changed`，业务服务订阅维护本地副本 |

### 13.6 Phase 1 代码预留

```go
// Phase 1：接口定义（未来可替换为 gRPC client）
type UserQueryService interface {
    GetUser(ctx context.Context, userID string) (*User, error)
    GetUserRoles(ctx context.Context, userID string) ([]string, error)
}

// Phase 1：本地实现
type localUserQueryService struct {
    repo repository.UserRepo
}

// Phase 2：远程实现（替换接口，调用方无感知）
type remoteUserQueryService struct {
    client pb.UserServiceClient  // gRPC client
}
```

### 13.7 Phase 2 关键技术问题

| 问题 | 方案 |
|------|------|
| 服务发现 | K8s DNS（K8s 部署）或 Consul（自建部署） |
| 负载均衡 | gRPC client-side `round_robin`，需 L7 层（Envoy/Istio 或 K8s Service） |
| 认证 | mTLS（服务间互信）+ JWT 透传（Gateway → 服务，保留用户身份） |
| 熔断/重试 | gRPC interceptor + exponential backoff + jitter |
| 超时 | gRPC per-call deadline propagation |
| 链路追踪 | OpenTelemetry gRPC interceptor，trace_id 跨服务传播 |

---

## 14. 待讨论事项

> 以下问题已在对话中提及但尚未深入展开，后续可补充。

| 事项 | 状态 | 说明 |
|------|------|------|
| 数据库迁移文件管理 | ✅ 已明确 | `golang-migrate`，`.up.sql` + `.down.sql`，种子数据走迁移 |
| 应用日志 vs 审计日志 | ✅ 已明确 | 应用日志 → 文件（slog + Lumberjack）；审计日志 → DB（**Phase 1 同步写入**；Phase 2+ channel / Phase 3 Redis List） |
| Casbin 策略存储 | ✅ 已明确 | PostgreSQL adapter，当前阶段内存 adapter 过渡 |
| Wire 生成 vs 手动 | ✅ 已明确 | Wire CLI 已安装，`make wire` 自动生成 |
| 部署与代码解耦 | ✅ 已确认 | 一套代码、配置驱动多种部署；业务层不感知拓扑。见 [§18 部署与代码解耦](./design-decisions.md#18-部署与代码解耦一套代码多种部署) |
| 存量 qingtao/aksk 与自研 M2M 签名 | ✅ 已确认 | **仅 Canonical，强制迁移**；不双栈、不保留 `x-auth-*` 验签。见 [phase1/02-auth.md §已决策：存量 qingtao/aksk 迁移策略](../phase1/02-auth.md#已决策存量-qingtaoaksk-迁移策略) |

---

## 15. 工单系统选型：自研 vs 现成产品

### 决策：自研，Phase 2 开始

**否决现成产品的理由**：

| 方案 | 否决理由 |
|------|----------|
| Zammad | Ruby 栈，4GB+ RAM，独立用户体系 |
| FreeScout / GLPI | PHP 栈，另起运行时 |
| go-help-desk | 6 star，无生产验证 |
| escalated-go | 3 star，无审批流能力 |
| easy-workflow | MySQL-only + GORM + 全局状态，与 PG + pgx + Wire 不兼容 |
| ECMDB/EFlow | MongoDB + Kafka，技术栈不匹配，EFlow 未开源 |

**自研的核心优势**：工单本身是一种"资源"，直接纳入框架已设计好的三层鉴权体系。

**分阶段实施**：
- Phase 1：不做工单，核心目标是认证鉴权框架
- Phase 2：基础工单 CRUD + 类型配置 + 三层鉴权 + 进程内事件
- Phase 3：SLA + 通知 + 审批流 + 事件驱动

详见 `modules/ticket.md`。

---

## 16. 事件驱动方案：Outbox + Asynq

### 决策：分阶段引入，Phase 1 用进程内事件，Phase 2 引入 Outbox + Asynq

**方案选型对比**：

| 方案 | 结论 |
|------|------|
| Kafka | 个人项目过度设计，需独立集群 |
| Redis Streams | 可用但缺少任务管理（重试、死信、优先级） |
| Asynq | 异步任务最佳选择（13K star，开箱即用），但 Pub/Sub 能力弱 |
| **Outbox + Asynq** | **最终方案**：Outbox 保证 DB 写入和事件发布原子性，Asynq 处理异步任务 |

**分阶段**：
- Phase 1：Go 原生 channel + goroutine 进程内事件
- Phase 2：PostgreSQL Outbox 表 + Relay goroutine + Asynq 异步任务队列
- Phase 3：评估 Redis Streams 或 Kafka 做跨服务事件传播

**事件驱动作为独立横切模块设计**，不耦合在工单模块内，因为定时任务、其他服务调用也会触发事件。

---

## 17. 参考项目借鉴清单

审计了三个参考项目（ginfast、ecmdb、go-wind-admin），记录可借鉴的设计：

### ginfast（Gin + GORM + Casbin 多租户脚手架）

| 借鉴项 | 优先级 | 阶段 |
|--------|--------|------|
| 操作日志中间件（异步+脱敏+自动识别模块/类型） | P1 | Phase 1 |
| 审计字段自动注入（GORM Callback → repository 层 AOP） | P1 | Phase 1 |
| DataScope 5 级数据权限（补充"自定义部门"级别） | P2 | Phase 2 |
| 全栈代码生成（表结构→CRUD+前端+菜单注册） | P3 | Phase 3 |

### go-wind-admin（Kratos + Ent + Wire + Asynq）

| 借鉴项 | 优先级 | 阶段 |
|--------|--------|------|
| 事件总线 Manager（命名总线+异步安全 context） | P2 | Phase 2 |
| 授权引擎接口抽象（Casbin/OPA/Zanzibar 可切换） | P2 | Phase 2 |
| Wire 分层 ProviderSet（data/service/server 各自独立） | P1 | Phase 1 |
| 脚本引擎 Hook（Lua/JS 动态扩展业务逻辑） | P3 | Phase 3 |

### ecmdb（Gin + MongoDB + Wire + Kafka）

| 借鉴项 | 优先级 | 阶段 |
|--------|--------|------|
| 模型驱动设计（Model→Attribute→Resource） | 已采用 | 工单类型设计已借鉴 |
| 插件化资源动作绑定 | 已采用 | TicketHooks 机制已借鉴 |
| Wire 模块化 Provider Set | 已采用 | 已实践 |

---

## 18. 部署与代码解耦：一套代码多种部署

### 决策：同一套业务代码，部署拓扑由配置与编排决定，不因换部署方式而改 Handler/Service 逻辑

**目标**：开发环境（单 PG + 单 Redis + 1 App）、生产环境（PG Cluster/VIP、Redis Sentinel、Nginx 后多 App 副本）共用 **同一二进制、同一业务代码**；差异落在 **配置文件、环境变量、进程副本数、可选横切组件开关**。

### 分层原则

| 层 | 职责 | 是否随部署变 |
|----|------|-------------|
| **配置**（`config.yaml` / env） | DSN、Redis 地址、副本相关开关（Watcher、审计模式等） | ✅ 仅改配置 |
| **装配**（`internal/app`、Wire） | 连接池、Redis 客户端（含 Sentinel failover 客户端）、Casbin Enforcer、可选 Watcher | ✅ 按配置选实现，**不改** Domain API |
| **Domain / Service / Handler** | 业务规则、鉴权语义、CRUD | ❌ **不**写 `if 单实例` / `if 集群` |
| **编排**（Compose / Nginx / K8s） | 副本数、健康检查、LB | ✅ 运维层，与代码仓库分离 |

### 典型部署差异 → 改什么

| 部署变化 | 业务代码 | 通常只改 |
|----------|----------|----------|
| PG 单节点 → Cluster + VIP | 不变 | `database.url` 指向 VIP |
| Redis 单实例 → Sentinel | 不变 | `redis` 配置或启用 failover 客户端（装配层） |
| App 1 副本 → N 副本 | 不变 | 编排副本数 + LB；**启用** Casbin Watcher 等（Phase 3） |
| 审计 sync → Redis List L2 | 不变 | 配置选写入模式（Phase 3a） |

### 已按「共享外部状态」设计的能力（多 App 友好）

以下依赖 **PostgreSQL / Redis**，不依赖「请求必须打到同一进程」：

- RT 存储与 `GetDel` 轮换、AT 黑名单、`user:disabled`、LoginLocker Lua
- 鉴权读 `user_roles` + Casbin（策略持久化在 `casbin_rule`）
- Phase 1 审计同步写 DB

### Phase 分期的含义（不是「只能一种部署」）

| 阶段 | 代码心智 | 含义 |
|------|----------|------|
| Phase 1–2 | 默认 **单 App 副本** 验收 | **先不实现** Watcher、分布式锁、perm 缓存等；不是禁止 PG/Redis HA |
| Phase 3 | 打开多副本横切能力 | Watcher、跨实例失效、审计 L2、HA 编排；仍 **同一套代码** |

> **多 App 注意**：Phase 1 未启用 Casbin Watcher 时，各副本内存策略在 `ReloadPolicy` 后可能短暂不一致；生产多副本应在 Phase 3 启用 Watcher，或 Phase 1 仅单副本运行。

### 反模式（避免）

- 在 Service/Handler 里根据部署形态分支业务逻辑
- 「单实例用内存、多实例用 Redis」两套逻辑散落各处（Phase 1 故意 **不用** perm 缓存，即为此）
- 为 Cluster/Sentinel 单独维护另一套代码分支

### 实现约束（编码时）

1. PG/Redis 地址、超时、池大小 **只来自配置**，禁止硬编码主机名。
2. 可选组件（Watcher、审计 async、Sentinel client）在 **`internal/app` / Wire** 按配置注入，Domain 只依赖接口。
3. 横切能力 **可关闭** 的默认路径须能在单实例下完整跑通 Phase 1 验收。

---

## 19. RBAC 继承与级联（业界参考）

> **Phase 1 不实现**——本节与专文均为**设计备忘**，避免 Phase 1 误做 BFS / org_roles / 数据 scope。

复杂场景（父部门角色是否继承、删组织是否踢人、删角色是否级联删用户）在 AD、Keycloak、AWS IAM、若依等产品中**结论并不相同**，但成熟方案共同点是：

1. **功能权限**、**组织赋角**、**数据范围** 三维分离；
2. 组织树向下扩的是 **数据可见**，不是默认扩 **API 角色**；
3. 删角色/删部门多为 **拒绝式**（先解绑），少做「静默级联删用户」。

本项目 Phase 1–2b 的取舍、级联矩阵与 12 条验收用例已单独成文，避免在 phase 文档里重复展开：

→ **[rbac-inheritance-and-cascade.md](./rbac-inheritance-and-cascade.md)**（SSOT，**Phase 2b+ 再实现**）

**相关文档**：[architecture.md §11](./architecture.md#11-部署架构)、[phase1/10-concurrency.md](../phase1/10-concurrency.md)、[proposal/deployment-evolution.md](../proposal/deployment-evolution.md)、[roadmap.md](../roadmap.md)。

## 20. 职责分离（SoD）延后决策

> **日期**：2026-08-31 落档 ｜ **来源**：[phase2/11-authz-architecture-review §4](../phase2/11-authz-architecture-review.md)（结论在该评审中已给出，本次补正式落档）
>
> **编号说明**：该评审原建议编号 P2-D7，后被 [phase2/00 §10 变更记录](../phase2/00-implementation-plan.md) 的「P2-D7 = Phase 2b 三轨拆分」复用——本条改挂本节编号，避免歧义。

**决策**：现在不建模 SoD（职责分离），落「延后 + 届时优先动态 SoD」记录：

- NIST RBAC 分 flat / hierarchical / constrained（含 SoD 静态/动态互斥）。当前实现为 flat RBAC（角色→接口），无互斥约束。
- 审批流属 Phase 3（[10-ticket-business §4](../phase3/10-ticket-business.md)，BranchedStateEngine）。届时真实需求形态是**动态 SoD**（「不能审批自己发起的申请」——工作流规则），而非静态角色互斥（「不能同时持有审批人与申请人角色」——过粗且难配置）。
- 现在建互斥表属过度设计；schema 无需预留。
- 设计期注意：审批流的节点审批人解析已按 [`Assignee {rule, values}` 策略模型](../phase3/10-ticket-business.md) 设计（2026-08-31，eflow 对标），动态 SoD（如"审批人 ≠ 发起人"）在该模型上以运行时规则表达。

**关联**：[10-ticket-business §4](../phase3/10-ticket-business.md)（BranchedStateEngine）、[11-authz-architecture-review §4](../phase2/11-authz-architecture-review.md)

## 21. 软删组织与遗留工单的委托语义

**背景**（2026-09-02 外评实证）：工单委托可见性三处 SQL——`delegatedVisibilitySQL`（列表，ticket/resource.go）、`IsOrgAdminOrOwner` / `IsAncestorOwner`（单条 + L3 canOperate，org_delegation.go）——对 `organizations` 均无 `deleted_at IS NULL` 过滤，且组织删除守卫（org_repo.Delete / DeleteVgWithOwnerCleanup）只查 children/members，**不查工单、不查 `owner_user_ids`**。组合效果：软删组织的历史工单对该组织原 owner / admin·owner / 祖先 owner 保留**读 + 写**（委托是挂在活组织上的管理授权，组织删除后本应消亡）。

**触发面（实测收敛，比表象窄）**：
- 普通 `Delete` 路径**常规不可达**——`owner_user_ids` 双轨强同步（SetOwners 建 owner 成员行、RemoveMember/SetUserOrgs/删用户均 `array_remove`），带 owner 的组织必然带成员行被守卫拦住，逐个移除即同步清空数组；
- vg 删除路径（`DeleteVgWithOwnerCleanup`）**系统性产生**——它清 owner 成员行以过守卫，但保留 `owner_user_ids` 数组，删除后 ghost 委托即成形。
- 三处 SQL 行为一致（列表/单条/操作无错位），残留者仅为删除前的原责任人群，无越权扩大。

**决策**（两步划界，2026-09-02 拍板）：
1. **活工单 → 禁删守卫**（待实施，BK-20，见 00 §9）：`Delete` / `DeleteVgWithOwnerCleanup` 加 `status <> 'closed'` 工单计数，非零即 409（新错误码，语义「先结单或转派」）。与既有 children/members 门槛同构，与 BK-18「类型有单禁删」先例一致；子组织禁删已保证计数范围=自体。
2. **已结工单 → 委托残留 = 档案连续性，不修**：三处 SQL 不加 `deleted_at` 过滤。已结工单可操作余量极小（update 被 `ErrTicketAlreadyClosed` 拒），残留属档案连续而非危害；创建人/处理人腿与活祖先实体锚点腿不受影响，工单不会变黑洞。**显式断开杠杆**：删除前 `SetOwners` 清空（支持空列表，立即生效）。

**翻案条件**：出现「删组织必须收回遗留工单管辖」的真实运维诉求；或证明 `owner_user_ids` 双轨不变量有洞（普通 Delete 路径也能带 owner 软删）；或合规要求「删除即权限终止」。届时修法 = 三处 SQL 各加一行 `AND o.deleted_at IS NULL`（vg 路径的 ghost 随之消亡），无需动守卫。

**关联**：[09-ticket §5.2](../phase2/09-ticket.md)（策略 B 可见性）、[04-org-delegation §4.2](../phase2/04-org-delegation.md)（D7/D8）、00 §9 BK-20（禁删守卫待实施）、[phase2/11-authz-architecture-review](../phase2/11-authz-architecture-review.md)（Q5 不变量——本决策不涉缓存，与「每请求实时判定」兼容）

## 22. Phase 3 执行结构修订（2026-09-02 所有者拍板）

**背景**：Phase 3 排期规划稿（[phase3/13](../phase3/13-implementation-plan.md)）评审对话中，所有者对执行结构做出六项拍板，修订 2026-08-31 的部分决议。逐项：

1. **「W2 硬前置 W1」修订为「M2 硬依赖 = Asynq 底座」**：单实例起步时 multi-instance/Watcher 无验收手段（MI1–5 需 2 实例环境）；防重不依赖多实例里程碑——asynq Redis 集中调度天然防任务重跑，`sla:scan` 配 Unique、L1 消费配 advisory lock（[02-multi-instance](../phase3/02-multi-instance.md) 既有约定，W2 写码时遵守）。M1（observability/multi-instance/audit-l2）整体降 🚦「部署形态升级为多实例时启动」；**翻案条件**：启动时即确定多实例部署 → 恢复原前置。风险对冲：M2 交付后起多实例前，按 02 号文档复核消费代码（约定已在档，成本低）。
2. **HR 同步从「2b-ext 按需」升为主链正式任务**：组织架构来自公司接口定时更新为已确认的真实运行方式。本次实施「预留接口版」：`HRFetcher` 接口 + sync 引擎 + 本地 mock adapter，公司内网 adapter 后续实现接口即接入（零改引擎）。启动设计须拍板：离职在途工单处置、部门撤销 × tickets.org_path 级联（[hr-directory-sync §3.2/§4.3](../proposal/hr-directory-sync.md) 待设计块）+ **跨部门权限分配规则**（新加，见第 4 条）。
3. **activelist 集成拆两半**：zhuzhao 定位 = 工单系统单向上游——**本次 Phase 3 执行「审批通过 → L1 事件 → Asynq 任务 → ActivelistWriter 接口」管道**（数据契约进 7-0 拍板）；E13 反代 / G1 Mongo→PG / G3 多源 ingress 作为架构蓝图保留（ADR-003），🚦 由 activelist 独立项目成型触发。**activelist 使用独立数据库**（与工单库故障隔离）拍板认可。审批通过事件 = 写入触发点。
4. **跨部门签发模型拍板：建单不选部门、只指派给人**（assigned_to 可为外部门成员）。可见性/读写由属主豁免（`created_by OR assigned_to`）覆盖，与部门无关；跨部门处理人的 API 能力（update/推进/评论）由 viewer 等角色的 AssignMenus 运营配置补齐（L1 授权 + L2 属主边界，零开发）。**转派部门端点不做**（签发在创建时由指派完成）。衍生拍板项进 7-0：通知/SLA 的「跟人（处理人所属组织 owner）vs 跟单（工单归属组织）」语义、账号初始化依赖 HR 同步全量覆盖。
5. **7c 完整工单引擎照做（硬交付维持）**：Phase 3 原设想 = 完整工单能力，含完整引擎，参考 easy-workflow（[10 §4.8](../phase3/10-ticket-business.md) 复核结论）。
6. **工单引擎须可替换（新增约束）**：以 `WorkflowEngine` 接口抽象引擎（Start/Advance/CanAct/OnEvent 类契约），service 构造注入 + 配置选择实现（先例：ResourceAuthorizer 接缝）；**实例状态存储分通用层（状态/当前节点/参与者，通用表字段）与引擎私有层（JSONB）**——接口可换而存量实例可结转或按旧引擎跑完，杜绝「接口换了数据迁不动」。接口契约与分层细节 7-0 拍板后落 10 号 §4。

## 23. Phase 3 重定位：工单自研暂缓（内部引擎优先，自研兜底），通用能力 + 内网迁移主线（2026-09-02 所有者拍板）

**背景**：继 §22 排期讨论后，所有者明确战略方向——项目将**迁移到公司内部**，届时**对接公司内部的工单平台/工单引擎**；自研工单业务能力（SLA/通知/审批引擎/分派/报表）不再实施。§22.5「7c 完整引擎照做」**被本决策推翻**（§22 其余各项效力不受影响，除本文明确修订者）。

**决策**：
1. **工单模块 Phase 2 现状封版**：已交付能力（CRUD/状态机/三层鉴权/类型字段模板管理）冻结，仅保留数据安全类修复（如 BK-20 守卫）；7a–7e、BranchedStateEngine、B3 推进端点、12-frontend 施工全部**暂缓自研**。10 号（工单业务设计）/ 12-frontend 保留作**对接参考与历史设计**，不再驱动开发。
2. **Phase 3 主线 = 通用能力 + 迁移准备**：① Asynq 事件基建（首任务 = 审计归档 B11②，非工单闭环独立验收）；② activelist 独立实现（独立库 + 独立数据库已拍板；**外部事件接入契约由 activelist 侧定义**——取代 §22.3 的 ActivelistWriter 方向，工单不再作为首数据源）；③ HR 同步预留接口版（内网迁移前置，组织/人员来自公司接口）；④ 内网部署准备（security/ops 中与生产相关项随迁移形态重估）。
3. **内部工单平台对接 = 迁移后集成项（🚦）**：对接形态（zhuzhao 作为 IAM/鉴权提供方？数据同步？单点跳转？）待迁移时依公司平台契约拍板；届时 Phase 2 工单资产的处置（保留兜底 / 适配为对接层 / 逐步下线）一并拍板。
4. **zhuzhao 价值重定位**：IAM 内核 + 通用能力底座（事件/审计/组织/HR 同步）+ activelist；工单从「自研产品线」转为「公司平台的对接方」。

**翻案条件**：公司内部平台无法承接工单诉求（功能/流程/集成成本），或迁移计划取消 → 恢复 §22.5 自研路线（10 号设计 SSOT 完整保留，随时可重启）。

**§23 补充拍板（2026-09-02）：任务平台扩展 M-E**：M-E 从「Asynq 底座 + 审计归档」扩展为**任务平台**——① Asynq 基建；② 预置任务（审计归档 B11② 等，Go 编译内置 handler）；③ **自定义脚本任务**（用户经平台配置 Python/Shell：任务配置表 job_configs + `os/exec` 执行器 + 执行记录表 job_runs + 管理 CRUD/启停/手动触发/历史端点）。**安全边界（拍板）**：自定义脚本仅全局管理员可配（L1 收敛）、超时强杀进程组、执行留痕、脚本与凭据分离；容器/专用用户隔离为可选进阶。**参照**：执行器模式照 ginfast scheduler（已核验：执行记录/重试/BlockingPolicy/管理端点真实可用）、管理面范式对齐 xxl-job GLUE、运维视角先用 asynqmon。**运行时依赖**：部署镜像需带 python/shell 解释器——记入 M-Mig 内网迁移准备清单。

## 24. 公司 SSO 对接：OAuth2.0 授权码模式，预留接口版（2026-09-02 所有者拍板）

**决策**：登录对接公司单点登录，协议 **OAuth2.0（授权码模式）**。按 §22.2 HR 同步同款「预留接口」模式实施：

1. **架构红线：SSO 只替换认证，不动鉴权**——callback 换身份后**签发 zhuzhao 自有 JWT/RT**，L1 Casbin / L2 资源 / L3 状态机零改动；权限仍全量走 zhuzhao RBAC（不透传公司 token/权限）。
2. **`SSOProvider` 接口**：`LoginURL(state)` / `ExchangeCallback(code) (Identity, error)`——本次实现接口 + OAuth2.0 标准实现（authorize + code 换 token + userinfo，含 state 防 CSRF）；公司侧 endpoint/client_id/secret 走 config，拿到接入信息填配置即用。
3. **身份映射**：对账键沿用 HR 同步定义（`external_id` 优先、`employee_no`/`domain_account` 次之，schema 已预埋）；登录服务按 `domain_account`/`source` 路由——SSO 域用户走 SSO，本地账号（superadmin 等）保留密码登录兜底。
4. **JIT 默认关**：首次 SSO 登录仅限 HR 已同步账号；自动建号做成 config 开关（默认 false）——HR 同步先行的顺序天然对齐，放开只需改配置。
5. **登出**：zhuzhao 侧 revoke 照旧；与公司平台的单点登出（SLO）联动 = 可选项，迁移时按平台能力拍板。
6. **审计**：登录事件加 `method`（sso/local）。

## 25. 权限架构定版：PDP/PEP 分工、统一网关边界与平台策略库（2026-09-03 讨论定稿）

**背景**：§23 重定位后主链为 M-E（taskrunner，独立仓库+独立部署）+ M-A（activelist，零认证薄层）；zhuzhao 定位为**统一对外网关**（§13 M-E 拍板已含「zhuzhao 作网关：鉴权/编排/业务审计」）。本节定版权限侧结论，作为 M-E/M-A 权限前置的 SSOT。

### 25.1 PDP/PEP 分工（NIST 框架）

- **PDP（规则定义）**：身份/角色/组织关系**集中一份**（zhuzhao：users/roles/user_orgs/org_roles + Casbin policy）；行级策略**按领域分布**（资源属主管自己数据的行规则；activelist 零认证薄层可零权限代码——网关权限码挡住+数据组内全可见）。
- **PEP（判定执行）**：API 级在**网关**（CasbinAuth 中间件，对反代路由同样生效）；**行级必然在数据处**——行级判定等价于给属主查询加 WHERE，网关看不到属主库的行，**逻辑上不可能集中**。业界铁律：Google Zanzibar 的 Docs/Drive 内嵌 stub 自行发起 Check、K8s RBAC 在各 API server 执行、AWS IAM 策略集中但各服务是自己的 PEP、OPA bundle 集中分发 enforcement 分布执行——**没有任何系统把行级 PEP 放进网关**（Kong/Envoy+ext_authz 也只做认证+路由级）。
- **落地分工**：身份集中（外部服务不建 users/roles 表）＋ 行级归属主 + 网关用集中身份库管 API 级。「每个服务一套角色库」是把行级策略误当全套。

### 25.2 统一网关边界（zhuzhao）

网关吃下：**认证（JWT）+ 路由级授权（Casbin policy，path+method）+ 审计（请求级，proxy 路由跳 body——ADR-003 G4）+ API 级限流（待补）**。行级权限不进网关（25.1）；网关对行级的正确姿势 = **身份断言透传**。

**身份断言契约**（M-A 前置；M-E 经「回调 zhuzhao 内网端点」不经断言链）。**2026-09-03 基线修订（覆盖下述 A/B 之争）**：服务间通信统一 **AK/SK HMAC 签名**后，身份断言 = **明文 `X-Operator` 头 + 纳入签名覆盖**——明文断言不可伪造，方案 A（AT 验签）无消费方、降为触发条件驱动（服务可达面扩大 / 服务自行行级判定 / 合规要求）；基线 SSOT = phase3/16 号 §9。原始两方案保留作背景：
- **方案 A（推荐）：AT 原样透传 + 属主服务共享公钥验签**——无伪造面，属主不依赖网关实现细节；
- 方案 B：明文断言头（X-User-ID 等）+ 网络隔离防直连——有伪造面，仅限完全内网隔离场景。

**M-SSO 定位变化**：纯网关模型下 activelist 经网关访问，M-SSO（§24）降级为「非网关路径的独立 UI」场景可选项，按需再全量开工。

### 25.3 平台策略库（声明式接入，设计详见 authz.md §3.1）

- **形态**：三个内置 Resource 实现（`org-member`/`owner-only`/`role-gated`）+ `Builtin(code, policy)` 一行注册；schema 约定（org_id/created_by 列）。
- **策略逻辑与策略数据分离**：**事实进 DB**（谁绑什么角色/权限——已在做：casbin_rule/menu_apis/user_orgs），**语义进代码**（模块用哪个行级策略——与模块代码同生命周期，进 DB 只制造漂移面且无「不发版换策略」的变更场景）。K8s 内置 ClusterRole / AWS managed policy 同款取舍。
- 消费方：M-E 的 zhuzhao 侧任务提交/回调端点（org-member）。
- **归属 zhuzhao 本仓，不进共享 utils**（2026-09-03 补充拍板）：消费者分析锁死——taskrunner 不做业务判定（通用调度）、activelist 零权限（网关 L1 挡住+数据全可见），**唯一消费者是 zhuzhao 自身**；且策略谓词绑定 zhuzhao 库 schema（user_orgs）——抽进共享 utils 等于让独立服务拿到「查 zhuzhao 库」的构件，违反 25.1 属主原则。共享 utils 抽取边界就此明确：**只抽无数据依赖的纯工具**（jsonutil/logger/response 类）。独立服务将来需要组员判定 → 走身份断言（25.2），不共享策略库。
- **工单策略与策略库双路并存、永不合流**（2026-09-03 补充）：工单三轴+委托轴是工单特有语义，保持手写 Resource（`internal/service/ticket/`，冻结期原地封存，§23.1）；builtin 只服务普通模块。**策略的家=数据属主的家**：工单属主现为 zhuzhao 故在 zhuzhao；§23 主路径对接内部平台后行级判定随数据转移（zhuzhao 侧 645 行转对接参考资产）；翻案重启（§23 翻案条件命中）则解冻原位继续——Registry 双路设计即为此预留。内部平台行级表达力 vs zhuzhao 三轴的评估列入 M-Mig（13 号 §1）。

### 25.4 ReBAC/PBAC 不演进（触发器见 11-authz §5）

2026-09-03 评估：网关化/taskrunner/activelist 三场景**无一构成演进触发**——网关=路由级 RBAC（Casbin 已满足）；taskrunner=成员 EXISTS；activelist=行级自治（Zanzibar 系恰要求关系集中注册，与独立库决策正面冲突）。授权关系是树形（ltree），PG+SQL 判定有一致性/索引/可测试优势。命中 11-authz §5 触发器时演进 =「换 L2/L3 判定后端」（ResourceAuthorizer 接缝），L1 Casbin 不动；届时 OpenFGA（API 友好）与 SpiceDB（一致性选项多）二选一。

### 25.5 前置功能清单（M-E / M-A 动工前）

| 批次 | 项 | 量级 | 挂靠 |
|---|---|---|---|
| A（M-E 前置） | 平台策略库：三策略实现 + Builtin 注册 + schema 约定 fail-fast + 正负向测试（AST 护栏泛化可选） | 2–3 天 | authz.md §3.1 |
| B（M-A 前置） | 网关化：反代核心（前缀→上游注册表/ReverseProxy/错误映射）+ 身份断言（25.2 方案 A）+ API 级限流 + activelist API 入 menu_apis + proxy 审计跳 body（ADR-003 已设计） | ~1 周 | E13 泛化（ADR-003 蓝图保留） |
| C（随手） | 密码复杂度（网关化后=门户责任）+ 本节落档 | 半天 | IW2 auth-enhance |

**不做**：PDP 回调接口、策略配置进 DB/管理面（无消费方）、跨库判定、提前多实例（随 M1 触发条件）。

**影响面**：新增 2 端点（`/auth/sso/login`、`/auth/sso/callback`）+ SSOProvider 接口 + config SSO 段 + 登录审计字段；JWT 中间件/RT/三层鉴权/会话管理零改动。前端仅登录页加企业账号入口（前端后置策略下的最小例外）。

**与 HR 同步分工**：SSO 管「认证」（你是谁、凭证有效性——离职后公司侧自动拒登）；HR 同步管「账号数据」（建号/组织/禁用与会话吊销）。对账键一致，互为校验。排位 **M-SSO**（13 号），与 M-HR 相邻。

**§24 补充（2026-09-02 所有者确认）：字段适配层**——公司 IdP 的流程虽为标准授权码模式，但**消息字段不保证与标准 userinfo 一致**（命名/嵌套/自定义字段）。故 `Identity` 结构由 zhuzhao 侧定义为稳定最小契约（external_id / employee_no / domain_account / display_name / email），「公司报文 → Identity」的翻译收敛在 OAuth2.0 adapter 内部实现（差异大时支持字段映射配置），登录服务与身份映射只消费 `Identity`、不感知公司字段差异——adapter 是唯一的字段差异吸收点，公司接入信息到位后只改这一个文件。

**§24 补充（2026-09-02）：账密通道保留范围**——传统账密登录**保留**，但按账号来源分域：`superadmin` 与 `source=local` 用户保留账密（应急通道：SSO 故障/网络隔离时管理员仍可进入；服务/外部协作者类本地账号）；**`source=hr` 用户 SSO 上线后默认禁用账密**（HR 同步的密码为占位值，放行即绕过公司 MFA/密码策略形成安全旁路）——config 开关 `auth.local_password_for_hr`（默认 false）。SSO 未上线的过渡期不受影响（现状全账密）。
