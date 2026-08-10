# 认证鉴权模块架构设计文档

> 文档版本：v0.5  
> 更新时间：2026-08-10  
> 状态：方案讨论阶段（边界已定，分阶段实施计划已明确，实现细节待补充）

---

## 目录

- [1. 项目概述](#1-项目概述)
- [2. 技术选型](#2-技术选型)
- [3. 模块边界划分](#3-模块边界划分)
- [4. 鉴权体系设计](#4-鉴权体系设计)
- [5. 认证与 Token 机制](#5-认证与-token-机制)
- [6. 组织架构设计](#6-组织架构设计)
- [7. 动态路由与前端权限](#7-动态路由与前端权限)
- [8. 审计日志](#8-审计日志)
- [9. 多租户预留](#9-多租户预留)
- [10. 数据库 Schema](#10-数据库-schema)
- [11. 部署架构](#11-部署架构)
- [12. 并发与可靠性](#12-并发与可靠性)
- [13. 安全加固](#13-安全加固)
- [14. 运维与可观测性](#14-运维与可观测性)
- [15. 推荐库与开源参考](#15-推荐库与开源参考)
- [16. 统一响应与错误处理](#16-统一响应与错误处理)
- [17. API 路由总表](#17-api-路由总表)
- [18. 分阶段实施计划](#18-分阶段实施计划)
- [19. 待决策事项](#19-待决策事项)

---

## 1. 项目概述

### 1.1 模块目标

搭建一个通用的认证鉴权模块，提供以下核心能力：

| 功能 | 说明 |
|------|------|
| 用户登录 | 账号密码登录，双 Token 签发 |
| 认证 | Token 校验、无感刷新、登出、多设备管理 |
| 路由级鉴权 | 基于 RBAC，校验角色对 API 接口的访问权限 |
| 资源级鉴权 | 基于 ReBAC 关系遍历 + 属主判断，校验对具体资源的操作权限 |
| 动态路由 | 接口返回权限树，驱动前端菜单和按钮渲染 |
| 组织架构 | 实体组织 + 虚拟组（项目组），权限沿组织树继承 |
| 审计日志 | 记录用户操作行为，支持按用户/组织/操作类型查询 |
| 多租户 | 预留多租户扩展能力 |

### 1.2 设计原则

- **按业务领域纵向切分，按职责横向分层**
- Casbin 只管路由级 RBAC，资源级权限自研，避免策略爆炸
- 内部包（`internal/`）禁止外部 import
- 依赖注入，组件可替换
- 边界优先：先定边界和接口，再填充实现

### 1.3 项目边界

**本模块负责**：

- 用户身份认证（登录、登出、Token 管理）
- 权限校验（路由级 + 资源级）
- 用户/角色/组织/菜单/权限的数据管理
- 审计日志记录

**本模块不负责**：

- 具体业务逻辑（文章管理、项目管理等由业务模块实现）
- 消息推送、文件存储等基础设施能力
- 前端路由和权限渲染的具体实现（本模块只提供数据接口）

**与业务模块的交互契约**：

- 业务模块通过 `internal/middleware` 提供的中间件获得鉴权能力
- 业务模块通过 `internal/service/authz_service` 的接口进行资源级权限判断
- 业务模块的资源表需包含 `creator_id` 字段以支持属主判断
- 业务模块的资源需在 `resource_owners` 表中登记组织归属

---

## 2. 技术选型

### 2.1 最终选型

| 技术 | 用途 | 推荐库/包 | 选型理由 |
|------|------|----------|----------|
| **Gin** | HTTP 框架 | `github.com/gin-gonic/gin` | 轻量、中间件生态成熟 |
| **Viper** | 配置管理 | `github.com/spf13/viper` | 支持多格式、热更新、环境变量覆盖 |
| **PostgreSQL** | 主数据库 | `github.com/jackc/pgx/v5` + `pgxpool` | 原生协议、连接池、性能优于 database/sql |
| **Casbin** | 路由级 RBAC | `github.com/casbin/casbin/v2` + `github.com/casbin/casbin-pg-adapter` | 角色继承、路径匹配、PG adapter |
| **Redis** | 缓存/Session | `github.com/redis/go-redis/v9` | 官方维护、功能完整、与 go context 集成 |
| **Swagger** | API 文档 | `github.com/swaggo/swag` + `github.com/swaggo/gin-swagger` | 注解生成，与 Gin 集成 |
| **slog** | 日志 | Go 1.21+ 标准库 `log/slog` | 标准库，结构化日志，性能足够 |
| **Lumberjack** | 日志切割 | `gopkg.in/natefinch/lumberjack.v2` | 与 slog 配合，日志文件轮转 |
| **Wire** | 依赖注入 | `github.com/google/wire` | 编译时生成代码，无运行时开销 |

### 2.2 关键选型决策

#### 2.2.1 PostgreSQL 替代 MongoDB

**决策**：主库使用 PostgreSQL，放弃 MongoDB。

**理由**：

1. **事务一致性**：权限操作和业务操作在同一数据库内事务保证。MongoDB 多文档事务需要副本集环境，增加运维复杂度。
2. **关系型数据**：用户-角色-组织-菜单之间是典型的多对多关系，关系型数据库天然适合。
3. **Casbin Adapter**：PostgreSQL adapter 是 Casbin 官方重点维护的，成熟稳定。
4. **组织树查询**：PostgreSQL 的 `ltree` 扩展可以高效处理组织架构的祖先/后代查询。
5. **审计日志**：结构化审计记录在关系型数据库中查询更方便。
6. **业务数据特征**：当前业务数据以关系型为主，结构固定。

#### 2.2.2 slog 替代 Zap

**决策**：使用 Go 标准库 `slog`，不使用 Zap。

**理由**：

1. **标准库**：Go 1.21+ 内置，无第三方依赖，Go 团队永久维护。
2. **性能足够**：认证鉴权模块的 QPS 不会到日志库成为瓶颈的程度。
3. **结构化日志**：slog 原生支持结构化字段。
4. **生态成熟**：Gin 已有 slog 中间件，Lumberjack 兼容 slog。
5. **减少依赖**：少一个第三方依赖就少一个维护风险。

> **备注**：如果后期需要 log sampling（高频日志采样降级），可自定义 slog Handler 实现。

#### 2.2.3 资源级权限用 ReBAC 而非纯 ABAC

**决策**：路由级用 Casbin RBAC，资源级用 ReBAC（自研关系遍历），属主判断用简单 ABAC（代码内联）。

**理由**：

- 用户场景包含实体组织 + 虚拟组，权限沿组织层级继承，本质是关系图遍历问题。
- ABAC 基于属性匹配，难以表达组织层级的传递性继承。
- ReBAC 基于关系遍历，天然支持"用户 → 虚拟组 → 实体组织 → 资源"的权限传递链。
- Casbin 做 ReBAC 有局限（`g` 角色继承只能表达有限层级），且会导致策略爆炸。

#### 2.2.4 Wire 依赖注入

**决策**：使用 Google Wire 进行依赖注入。

**理由**：

1. **编译时生成**：Wire 在编译阶段生成依赖注入代码，无运行时反射开销，对性能零影响。
2. **类型安全**：依赖关系在编译期检查，缺失依赖或循环依赖在编译时即报错，而非运行时 panic。
3. **显式依赖链**：通过 Provider 和 Injector 函数，依赖关系一目了然，便于理解组件如何组装。
4. **与 Go 风格契合**：生成的代码就是普通的 Go 函数调用，可读性好，无 magic。

**Wire 在本项目中的职责边界**：

- **负责**：组件的构造与组装（DB 连接池、Redis 客户端、Casbin enforcer、repository → service → handler 的依赖链）
- **不负责**：业务逻辑、请求处理、数据访问

**目录结构影响**：

- `internal/app/` 下新增 `wire.go`（Injector 定义，`//go:build wireinject` 约束）和 `wire_gen.go`（Wire 生成的代码）
- 各包提供 `NewXXX()` 构造函数（Provider），接收依赖参数，返回实例
- `cmd/server/main.go` 只需调用 `app.InitializeApp()` 即可获得组装好的应用实例

---

## 3. 模块边界划分

### 3.1 目录结构

```
zhuzhao/
├── cmd/
│   └── server/
│       └── main.go                # 唯一入口，只做启动编排
│
├── internal/                      # 内部包，禁止外部 import
│   ├── config/                    # 配置加载与热更新 (Viper)
│   │   └── config.go
│   │
│   ├── model/                     # 数据模型定义（纯结构体，无逻辑）
│   │   ├── user.go
│   │   ├── role.go
│   │   ├── organization.go
│   │   ├── menu.go
│   │   ├── audit_log.go
│   │   └── token.go               # Token 相关结构体
│   │
│   ├── repository/                # 数据访问层（PostgreSQL CRUD）
│   │   ├── user_repo.go
│   │   ├── role_repo.go
│   │   ├── org_repo.go
│   │   ├── menu_repo.go
│   │   └── audit_log_repo.go
│   │
│   ├── service/                   # 业务逻辑层
│   │   ├── auth_service.go        # 登录、Token 签发/刷新/登出
│   │   ├── user_service.go        # 用户管理
│   │   ├── rbac_service.go        # 角色-权限业务逻辑
│   │   ├── authz_service.go       # 资源级鉴权（ReBAC + 属主判断）
│   │   ├── org_service.go         # 组织架构管理
│   │   ├── menu_service.go        # 菜单树构建
│   │   └── audit_service.go       # 审计日志
│   │
│   ├── middleware/                # Gin 中间件
│   │   ├── jwt.go                 # JWT 解析与校验 + 黑名单检查
│   │   ├── casbin.go              # 路由级鉴权中间件
│   │   ├── resource_authz.go      # 资源级鉴权中间件
│   │   ├── ratelimit.go           # 限流
│   │   ├── audit.go               # 审计日志记录
│   │   └── recovery.go            # Panic 恢复
│   │
│   ├── handler/                   # HTTP 处理器（Controller 层）
│   │   ├── auth_handler.go        # 登录/刷新/登出/设备管理
│   │   ├── user_handler.go
│   │   ├── role_handler.go
│   │   ├── org_handler.go
│   │   ├── menu_handler.go
│   │   └── permission_handler.go
│   │
│   ├── router/                    # 路由注册
│   │   ├── router.go              # 路由总入口
│   │   └── routes.go              # 路由表定义 + 中间件挂载
│   │
│   ├── casbin/                    # Casbin 专属封装
│   │   ├── enforcer.go            # enforcer 初始化与单例
│   │   ├── model.conf             # Casbin RBAC 模型定义
│   │   └── policy.go              # 策略管理（增删改查）
│   │
│   ├── pkg/                       # 项目内通用工具包
│   │   ├── jwt/                   # JWT 签发与解析
│   │   ├── response/              # 统一响应封装
│   │   ├── errcode/               # 错误码定义
│   │   ├── logger/                # slog 日志封装
│   │   ├── redis/                 # Redis 客户端封装
│   │   └── crypto/                # 密码加密（bcrypt）
│   │
│   └── app/                       # 应用依赖注入与生命周期
│       ├── app.go                 # 应用结构体与启动/关闭逻辑
│       ├── wire.go                # Wire Injector 定义 (//go:build wireinject)
│       └── wire_gen.go            # Wire 生成的注入代码 (自动生成)
│
├── api/                           # API 定义
│   └── v1/
│       └── dto/                   # 请求/响应结构体
│
├── configs/
│   ├── config.yaml                # 主配置
│   └── casbin_model.conf          # Casbin 模型
│
├── docs/                          # 文档
│   └── architecture.md            # 本文档
│
├── scripts/
│   └── swagger.sh                 # swag init 脚本
│
├── deployments/
│   └── docker-compose.yaml        # 开发环境
│
├── migrations/                    # 数据库迁移 (golang-migrate)
│   ├── 000001_init.up.sql
│   ├── 000001_init.down.sql
│   └── ...
│
├── go.mod
├── go.sum
└── Makefile
```

### 3.2 各模块职责边界

| 模块 | 职责 | 禁止事项 |
|------|------|----------|
| `cmd/server` | 启动编排 | 不包含任何业务逻辑 |
| `internal/config` | 加载配置，提供类型安全结构体 | 不依赖业务模块 |
| `internal/model` | 纯数据结构定义 | 不依赖任何外部包，不含逻辑 |
| `internal/repository` | 数据读写，封装 SQL | 不做业务判断，不暴露连接池 |
| `internal/service` | 核心业务逻辑 | 不直接处理 HTTP |
| `internal/handler` | 请求解析、参数校验、调用 service | 不直接操作 repository |
| `internal/middleware` | 横切关注点（鉴权、日志、限流） | 不包含业务逻辑 |
| `internal/casbin` | Casbin 初始化、模型、策略管理 | 避免散落各处 |
| `internal/pkg` | 通用工具 | 不依赖业务模块 |
| `internal/app` | Wire 依赖注入、应用生命周期管理 | 不包含业务逻辑；`wire.go` 手写，`wire_gen.go` 自动生成 |

### 3.3 依赖方向

```
cmd/server
    ↓
internal/app (Wire 注入入口)
    ↓
internal/router → internal/handler → internal/service → internal/repository → internal/model
                   ↓                    ↓                   ↓
              internal/middleware   internal/casbin      internal/pkg
```

**规则**：依赖只能向下游流动，不允许反向引用。`model` 是最底层，所有人都可以引用它。

**Wire 依赖注入链**：

```
Wire Injector (wire.go)
    │
    ├── 基础设施 Provider
    │   ├── NewConfig()        → *config.Config
    │   ├── NewPostgres()      → *pgxpool.Pool
    │   ├── NewRedis()         → *redis.Client
    │   ├── NewCasbin()        → *casbin.SyncedEnforcer
    │   └── NewLogger()        → *slog.Logger
    │
    ├── Repository Provider（依赖 DB）
    │   ├── NewUserRepo()      → *repository.UserRepo
    │   ├── NewRoleRepo()      → *repository.RoleRepo
    │   ├── NewOrgRepo()       → *repository.OrgRepo
    │   └── ...
    │
    ├── Service Provider（依赖 Repository + 基础设施）
    │   ├── NewAuthService()   → *service.AuthService
    │   ├── NewAuthzService()  → *service.AuthzService
    │   └── ...
    │
    ├── Handler Provider（依赖 Service）
    │   ├── NewAuthHandler()   → *handler.AuthHandler
    │   └── ...
    │
    ├── Middleware Provider（依赖 Casbin + Redis）
    │   ├── NewJWTMiddleware() → gin.HandlerFunc
    │   └── ...
    │
    └── Router Provider（依赖 Handler + Middleware）
        └── NewRouter()        → *gin.Engine

最终生成: InitializeApp(cfg) → *App
```

### 3.4 各模块对外接口边界

#### service 层接口（核心）

| Service | 对外方法 | 说明 |
|---------|----------|------|
| `AuthService` | `Login` | 登录，签发双 Token |
| | `Refresh` | 刷新 Token，RT 轮换 |
| | `Logout` | 登出，吊销 AT + 删除 RT |
| | `KickDevice` | 踢出指定设备 |
| | `ListDevices` | 查询用户活跃设备列表 |
| `AuthzService` | `CheckResourcePermission` | 资源级权限校验（属主 → ReBAC） |
| `MenuService` | `GetUserMenus` | 获取用户菜单树 |
| | `GetUserPermissions` | 获取用户按钮权限码列表 |
| `OrgService` | `GetOrgTree` | 获取组织树 |
| | `GetUserOrgs` | 获取用户所属组织列表 |
| `AuditService` | `Record` | 记录审计日志（异步） |
| | `Query` | 查询审计日志 |

#### middleware 层接口

| Middleware | 挂载位置 | 说明 |
|------------|----------|------|
| `Recovery` | 全局 | Panic 恢复 |
| `Logger` | 全局 | 请求日志 |
| `JWT` | 需认证路由 | Token 解析 + 黑名单检查 |
| `Casbin` | 需鉴权路由 | 路由级 RBAC 校验 |
| `ResourceAuthz` | 需资源鉴权路由 | 资源级权限校验 |
| `RateLimit` | 全局或按路由 | 限流 |
| `Audit` | 需审计路由 | 操作记录 |

---

## 4. 鉴权体系设计

### 4.1 三层权限解析架构

```
┌──────────────────────────────────────────────────────────┐
│                    权限解析三层架构                         │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  第一层：路由级 RBAC（Casbin 处理）                        │
│  ──────────────────────────────                          │
│  粒度：API 路径 × HTTP 方法                                │
│  策略量：角色数 × API数 × 方法数 ≈ 1,000 条（可控）         │
│  存储：Casbin + PostgreSQL                                │
│  问题："editor 角色能访问文章编辑接口吗？"                  │
│                                                          │
│  第二层：资源级 ReBAC（关系遍历，自研）                     │
│  ──────────────────────────────                          │
│  粒度：资源 × 组织关系 × 操作                              │
│  策略量：不预生成策略，运行时遍历关系图（零策略存储）         │
│  存储：PostgreSQL 关系表 + ltree 递归查询                  │
│  问题："用户A 通过组织关系能否编辑文章123？"                 │
│                                                          │
│  第三层：资源属主判断（简单 ABAC，代码内联）                │
│  ──────────────────────────────                          │
│  粒度：creator_id == current_user_id                      │
│  策略量：0（代码逻辑判断）                                  │
│  存储：资源表中的 creator_id 字段                          │
│  问题："用户A 是文章123的创建者吗？"                       │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

**执行顺序**：AT 校验 → 第一层 Casbin → 第三层属主（短路） → 第二层 ReBAC

### 4.2 第一层：路由级 RBAC（Casbin）

**职责**：校验"角色是否有权访问该 API 接口"。

**Casbin 模型**：RBAC，`sub`（角色）→ `obj`（API 路径）→ `act`（HTTP 方法），支持 `keyMatch2` 路径通配。

**策略量**：角色数 × API 数 × 方法数 ≈ 1,000 条，内存无压力。

**策略示例**：

```
p, admin, /api/v1/*, *
p, editor, /api/v1/articles/*, GET
p, editor, /api/v1/articles/:id, PUT
g, user_001, editor
```

**热更新**：

- 单实例：AutoSave 模式，策略变更同时更新内存和 DB
- 多实例：Redis Watcher 模式，通过 Pub/Sub 广播策略变更通知

> 详细模型定义见 `configs/casbin_model.conf`，实现细节后续补充。

### 4.3 第二层：资源级 ReBAC（自研关系遍历）

**职责**：校验"用户通过组织关系能否操作该具体资源"。

**核心思路**：不在 Casbin 中预生成资源级策略（避免策略爆炸），运行时通过 PostgreSQL `ltree` 递归查询判断权限。

**权限判断链**：

```
用户 → 所属组织（含虚拟组）→ 沿组织树向上/向下遍历 → 是否到达资源所属组织
```

**权限范围（scope）**：

| scope 值 | 含义 | 场景 |
|-----------|------|------|
| 1 | 仅本组 | 只能操作自己所在组织的资源 |
| 2 | 本组及子组 | 主管可以操作下属组织的资源 |
| 3 | 本组及父级链 | 成员可以向上访问上级组织的共享资源 |

**关键 SQL**（利用 ltree `@>` 祖先判断）：

```sql
SELECT EXISTS (
    SELECT 1
    FROM user_orgs uo
    JOIN organizations uo_org ON uo.org_id = uo_org.id
    JOIN resource_owners ro ON ro.resource_type = $1 AND ro.resource_id = $2
    JOIN organizations res_org ON ro.org_id = res_org.id
    JOIN org_permissions op
        ON op.role_id = $3 AND op.resource_type = $1 AND op.action = $4
    WHERE uo.user_id = $5
        AND (
            (op.scope = 2 AND uo_org.path @> res_org.path)
            OR (op.scope = 1 AND uo_org.id = res_org.id)
            OR (op.scope = 3 AND res_org.path @> uo_org.path)
        )
) AS has_access;
```

> 实现细节后续补充。

### 4.4 第三层：资源属主判断（ABAC，代码内联）

**职责**：校验"用户是否是资源的创建者"。

- 策略量：0（纯代码逻辑）
- 判断方式：资源表中的 `creator_id == userID`
- 短路优先：在资源级鉴权中最先执行，命中即放行

### 4.5 策略爆炸问题的解决

传统方案（全部塞进 Casbin）的策略量：

```
1000 用户 × 5 角色 × 50 资源 × 4 操作 × 3 组织层级 = 3,000,000 条
```

本方案的策略量：

| 层级 | 策略量 | 说明 |
|------|--------|------|
| 第一层 Casbin RBAC | ~1,000 条 | 角色数 × API数 × 方法数 |
| 第二层 ReBAC | 0 条 | 运行时查询，不预生成 |
| 第三层 ABAC | 0 条 | 代码逻辑 |
| **总计** | **~1,000 条** | 内存无压力 |

---

## 5. 认证与 Token 机制

### 5.1 双 Token 方案概述

采用 **accessToken（AT）+ refreshToken（RT）** 双 Token 机制，RT 支持轮换。

| 维度 | 方案说明 |
|------|----------|
| AT 有效期 | 短（2h），无状态 JWT，每次请求携带 |
| RT 有效期 | 长（7d），有状态存 Redis，仅用于 /refresh |
| RT 轮换 | 每次刷新时废弃旧 RT、签发新 RT |
| AT 吊销 | 登出/踢出时加入 Redis 黑名单（TTL = AT 剩余有效期） |
| 多设备 | 每个设备独立 RT，支持查看设备列表和踢出指定设备 |

**为什么需要双 Token**：

- JWT 的无状态性导致 AT 签发后无法主动撤销 → AT 有效期必须短
- AT 有效期短 → 用户需频繁重新登录 → RT 实现无感刷新
- RT 存 Redis → 可主动吊销 → 弥补 AT 无状态的安全缺陷
- 本场景为企业内部系统 + 有审计要求 + 需多设备管理 → 双 Token 明确必要

**为什么 RT 需要轮换**：

- 每次刷新废弃旧 RT → 即使 RT 被截获，攻击者只有一次使用机会
- 轮换可检测 RT 被盗：攻击者和真正用户不能同时使用同一个 RT，先刷新的一方会使另一方的 RT 失效

### 5.2 Token Payload

**accessToken**：

| 字段 | 类型 | 说明 |
|------|------|------|
| user_id | string | 用户 ID |
| username | string | 用户名 |
| role | string | 主角色 key |
| org_id | string | 主组织 ID |
| device_id | string | 设备标识 |
| jti | string | Token 唯一标识（用于黑名单） |
| exp | timestamp | 过期时间 |

**refreshToken**（精简）：

| 字段 | 类型 | 说明 |
|------|------|------|
| user_id | string | 用户 ID |
| device_id | string | 设备标识 |
| jti | string | 唯一标识（与 Redis 中比对，防重放） |
| exp | timestamp | 过期时间 |

### 5.3 Redis 存储结构

```
# RT 存储（支持多设备 + 轮换）
# Key:   refresh:{userId}:{deviceId}
# Val:   JSON { jti, device_info, ip, created_at, last_refresh_at }
# TTL:   7d
refresh:user_001:device_mac     → {"jti":"rt_abc123","device":"MacBook","ip":"10.0.0.1",...}
refresh:user_001:device_iphone  → {"jti":"rt_def456","device":"iPhone","ip":"10.0.0.2",...}

# 设备列表（用于查询用户所有活跃设备）
# Key:   devices:{userId}
# Val:   SET of device_ids
devices:user_001 → {device_mac, device_iphone}

# AT 黑名单（登出/踢出时使用）
# Key:   blacklist:at:{atJti}
# TTL:   AT 剩余有效期
blacklist:at:at_xyz789 → 1
```

### 5.4 核心流程

#### 登录流程

```
1. 校验用户名密码
2. 签发 AT（2h，含用户信息）
3. 签发 RT（7d，精简 payload）
4. RT 存 Redis: refresh:{userId}:{deviceId}
5. 设备 ID 加入 Redis Set: devices:{userId}
6. 返回 AT + RT
```

#### 刷新流程（RT 轮换）

```
1. 解析 RT，提取 userId, deviceId, jti
2. 查 Redis: refresh:{userId}:{deviceId}
   ├─ 不存在 → RT 已吊销 → 拒绝
   └─ 存在 → 比对 jti
       ├─ 不匹配 → 可能重放攻击 → 拒绝
       └─ 匹配 → 继续
3. 查用户最新信息（角色可能变化）
4. 签发新 AT + 新 RT
5. 用新 RT 覆盖 Redis（旧 RT 自动失效）
6. 返回新 AT + 新 RT

※ 如果攻击者截获 RT 并抢先刷新：
   - 攻击者获得新 AT + 新 RT，旧 RT 失效
   - 真正用户再用旧 RT 时 jti 不匹配 → 被拒 → 察觉异常
```

#### 登出流程

```
1. 解析 AT 获取 jti 和过期时间
2. AT 加入 Redis 黑名单: blacklist:at:{jti}，TTL = AT 剩余有效期
3. 删除该设备的 RT: DEL refresh:{userId}:{deviceId}
4. 从设备列表移除: SREM devices:{userId} {deviceId}
```

#### 踢出设备

```
1. 删除该设备的 RT: DEL refresh:{userId}:{deviceId}
2. 从设备列表移除: SREM devices:{userId} {deviceId}
3. AT 在过期后自然失效（无法刷新获取新 AT）
```

### 5.5 JWT 中间件校验流程

```
请求进入
  │
  ├─ 1. 提取 Authorization Header 中的 AT
  ├─ 2. 解析 JWT，校验签名和过期时间
  ├─ 3. 查 Redis 黑名单: EXISTS blacklist:at:{jti}
  │     └─ 在黑名单中 → 拒绝（token 已失效）
  ├─ 4. 注入上下文: userID, username, role, orgID, deviceID
  └─ 5. c.Next()
```

### 5.6 对外接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/v1/auth/login` | POST | 登录，返回双 Token |
| `/api/v1/auth/refresh` | POST | 刷新 Token，RT 轮换 |
| `/api/v1/auth/logout` | POST | 登出，吊销 Token |
| `/api/v1/auth/devices` | GET | 查询当前用户活跃设备列表 |
| `/api/v1/auth/devices/:deviceId` | DELETE | 踢出指定设备 |

---

## 6. 组织架构设计

### 6.1 组织模型

```
┌─────────────────────────────────────────────────────────┐
│                      集团 (root)                         │
│                      path: root                          │
├──────────────────────┬──────────────────────────────────┤
│  技术中心 (entity)    │       产品中心 (entity)           │
│  path: root.tech     │       path: root.product          │
├──────┬───────────────┤                                  │
│ 前端组│  后端组        │                                  │
│(virtual)│(virtual)    │                                  │
│root.   │root.         │                                  │
│tech.fe │tech.be       │                                  │
└──────┴───────────────┴──────────────────────────────────┘

用户A: 属于 [前端组, 产品中心]（跨组织，多角色）
资源R1: 属于 [前端组]
→ 用户A 通过前端组的关系链可以访问 R1
→ 用户A 也可以通过产品中心的关系链访问产品中心的资源
```

### 6.2 设计要点

| 要点 | 说明 |
|------|------|
| 实体组织 + 虚拟组统一建表 | 用 `org_type` 区分，避免双表 JOIN |
| ltree 路径枚举 | 存储 `root.tech.fe`，支持 `@>`（祖先）和 `<@`（后代）高效查询 |
| 用户-组织多对多 | 一个用户可属于多个组织/组，在组织中有角色 |
| 权限继承 | 通过 ltree 路径关系，scope 控制继承方向 |
| 跨组织支持 | 用户可同时属于不同组织，权限取并集 |

### 6.3 组织类型

| 类型 | org_type | 说明 | 示例 |
|------|----------|------|------|
| 实体组织 | 1 | 公司真实的组织架构 | 集团、技术中心、产品中心 |
| 虚拟组 | 2 | 挂在实体组织下的项目组/虚拟团队 | 前端组、后端组、某项目组 |

---

## 7. 动态路由与前端权限

### 7.1 数据流

```
1. 数据库中存储 menu 表（树形结构，包含路由路径、组件、权限标识）
2. menu 和 role 关联（role_menus 表）
3. 用户登录后，调用 /api/v1/user/menus 接口
4. 后端根据用户角色查询关联的 menu 列表，构建树形结构返回
5. 前端根据返回的数据动态生成路由和菜单
6. 按钮级权限：每个按钮绑定一个 permission code（如 "user:create"）
   后端返回用户拥有的所有 permission codes，前端用 v-if 判断
```

### 7.2 菜单类型

| type | 含义 | 前端处理 |
|------|------|----------|
| 1 | 目录 | 渲染为菜单分组，不对应路由 |
| 2 | 菜单 | 渲染为菜单项，对应路由和组件 |
| 3 | 按钮 | 不渲染为菜单，前端按 permission code 控制显隐 |

### 7.3 接口设计

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/v1/user/menus` | GET | 返回当前用户的菜单树（目录+菜单） |
| `/api/v1/user/permissions` | GET | 返回当前用户拥有的所有 permission codes（按钮权限） |

---

## 8. 审计日志

### 8.1 记录内容

| 字段 | 说明 |
|------|------|
| user_id, username | 操作人（username 冗余存储，用户删除后仍可查） |
| org_id, org_path | 操作时的组织上下文 |
| method, path | HTTP 方法和请求路径 |
| action | 业务操作标识，如 `article.create` |
| resource_type, resource_id | 操作的资源 |
| request_body | 请求参数（脱敏后，JSONB） |
| response_code | 响应状态码 |
| ip, user_agent | 客户端信息 |
| latency_ms | 耗时 |
| status | 成功/失败 |
| error_msg | 错误信息 |

### 8.2 实现要点

- **记录位置**：middleware 层，在请求完成后记录
- **写入方式**：异步写入，channel + goroutine 消费，避免阻塞请求
- **脱敏处理**：对 password、token 等敏感字段在记录前移除
- **查询索引**：按用户+时间、组织路径+时间、操作类型+时间、租户+时间

> 实现细节后续补充。

---

## 9. 多租户预留

### 9.1 预留策略

当前阶段不实现多租户，但在设计中预留扩展点：

| 预留点 | 方式 |
|--------|------|
| 数据库表 | 所有业务表增加 `tenant_id` 字段（当前默认值） |
| Casbin 模型 | 预留 tenant 维度：`r = tenant, sub, obj, act` |
| JWT Token | payload 中预留 `tenant_id` |
| 中间件 | 预留租户解析中间件，从 header 或 token 中提取 |
| repository | 查询自动追加 `WHERE tenant_id = $1` |

### 9.2 Casbin 多租户模型预留

```ini
[request_definition]
r = tenant, sub, obj, act

[policy_definition]
p = tenant, sub, obj, act

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.tenant, r.sub, p.sub) && \
    r.tenant == p.tenant && \
    keyMatch2(r.obj, p.obj) && \
    r.act == p.act
```

> 当前阶段先用不含 tenant 的简化模型，后期迁移时再切换。

---

## 10. 数据库 Schema

### 10.1 完整建表 SQL

```sql
-- 启用扩展
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 用户表
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username        VARCHAR(50) UNIQUE NOT NULL,
    password        VARCHAR(255) NOT NULL,          -- bcrypt
    real_name       VARCHAR(100),                   -- 真实姓名
    email           VARCHAR(100) UNIQUE,
    phone           VARCHAR(20),
    avatar          VARCHAR(500),                   -- 头像 URL
    status          SMALLINT DEFAULT 1,             -- 1=启用 0=禁用
    last_login_at   TIMESTAMPTZ,                    -- 最后登录时间
    last_login_ip   VARCHAR(50),                    -- 最后登录 IP
    oauth_provider  VARCHAR(50),                    -- 第三方登录预留: google/github/...
    oauth_id        VARCHAR(200),                   -- 第三方账号 ID
    tenant_id       UUID DEFAULT gen_random_uuid(), -- 多租户预留
    version         INT DEFAULT 1,                  -- 乐观锁
    deleted_at      TIMESTAMPTZ,                    -- 软删除
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_users_deleted ON users(deleted_at) WHERE deleted_at IS NOT NULL;

-- 组织表（实体组织 + 虚拟组统一建表）
CREATE TABLE organizations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id   UUID REFERENCES organizations(id) ON DELETE SET NULL,
    name        VARCHAR(100) NOT NULL,
    org_type    SMALLINT NOT NULL,              -- 1=实体组织 2=虚拟组
    code        VARCHAR(50) UNIQUE NOT NULL,
    path        LTREE NOT NULL,                 -- 如 root.tech.fe
    leader_id   UUID REFERENCES users(id) ON DELETE SET NULL,  -- 组织负责人
    status      SMALLINT DEFAULT 1,
    sort_order  INT DEFAULT 0,
    tenant_id   UUID DEFAULT gen_random_uuid(), -- 多租户预留
    version     INT DEFAULT 1,                  -- 乐观锁
    deleted_at  TIMESTAMPTZ,                    -- 软删除
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_org_path ON organizations USING GIST(path);
CREATE INDEX idx_org_parent ON organizations(parent_id);
CREATE INDEX idx_org_tenant ON organizations(tenant_id);
CREATE INDEX idx_org_deleted ON organizations(deleted_at) WHERE deleted_at IS NOT NULL;

-- 角色表
CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(50) UNIQUE NOT NULL,    -- 角色key: "admin", "editor"
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    status      SMALLINT DEFAULT 1,
    sort_order  INT DEFAULT 0,
    tenant_id   UUID DEFAULT gen_random_uuid(), -- 多租户预留
    version     INT DEFAULT 1,                  -- 乐观锁
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- 用户-组织关系表（多对多，含角色）
CREATE TABLE user_orgs (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id      UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role_id     UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    is_primary  BOOLEAN DEFAULT FALSE,
    joined_at   TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, org_id)
);
CREATE INDEX idx_user_orgs_user ON user_orgs(user_id);
CREATE INDEX idx_user_orgs_org ON user_orgs(org_id);

-- 菜单表
CREATE TABLE menus (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id   UUID REFERENCES menus(id) ON DELETE CASCADE,
    name        VARCHAR(100) NOT NULL,
    path        VARCHAR(200),                   -- 前端路由路径
    route_name  VARCHAR(100),                   -- 前端命名路由名（如 "user-list"）
    component   VARCHAR(200),                   -- 前端组件路径
    redirect    VARCHAR(200),                   -- 重定向路径
    icon        VARCHAR(50),
    sort_order  INT DEFAULT 0,
    type        SMALLINT NOT NULL,              -- 1=目录 2=菜单 3=按钮
    permission  VARCHAR(100),                   -- 权限标识: "article:create"
    hidden      BOOLEAN DEFAULT FALSE,
    version     INT DEFAULT 1,                  -- 乐观锁
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- 角色-菜单关联表
CREATE TABLE role_menus (
    role_id     UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    menu_id     UUID NOT NULL REFERENCES menus(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, menu_id)
);

-- 资源-组织归属表
CREATE TABLE resource_owners (
    resource_type   VARCHAR(50) NOT NULL,
    resource_id     UUID NOT NULL,
    org_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    extra           JSONB,                      -- 预留扩展维度（如项目、部门）
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (resource_type, resource_id, org_id)
);
CREATE INDEX idx_resource_owners ON resource_owners(resource_type, resource_id);

-- 组织级权限模板表
CREATE TABLE org_permissions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id         UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    resource_type   VARCHAR(50) NOT NULL,
    action          VARCHAR(20) NOT NULL,       -- read/write/delete
    scope           SMALLINT NOT NULL,          -- 1=本组 2=本组及子组 3=本组及父级
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(role_id, resource_type, action)
);

-- 审计日志表
CREATE TABLE audit_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID,
    username        VARCHAR(50),                -- 冗余存储，用户删除后仍可查
    org_id          UUID,
    org_path        LTREE,                      -- 记录操作时的组织路径
    method          VARCHAR(10),
    path            VARCHAR(500),
    action          VARCHAR(50),                -- 业务操作: "article.create"
    resource_type   VARCHAR(50),
    resource_id     UUID,
    request_body    JSONB,                      -- 请求参数（脱敏后）
    response_code   INT,
    ip              VARCHAR(50),
    user_agent      VARCHAR(500),
    latency_ms      INT,
    status          SMALLINT,                   -- 1=成功 0=失败
    error_msg       TEXT,
    tenant_id       UUID,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_audit_user_time ON audit_logs(user_id, created_at DESC);
CREATE INDEX idx_audit_org_time ON audit_logs USING GIST(org_path);
CREATE INDEX idx_audit_action_time ON audit_logs(action, created_at DESC);
CREATE INDEX idx_audit_tenant_time ON audit_logs(tenant_id, created_at DESC);

-- Casbin 策略表（由 postgres-adapter 自动创建，结构如下）
-- casbin_rule: p_type, v0, v1, v2, v3, v4, v5
```

### 10.2 示例数据

```sql
-- 组织架构
INSERT INTO organizations (id, name, org_type, code, path) VALUES
('00000000-0000-0000-0000-000000000001', '集团', 1, 'root', 'root'),
('00000000-0000-0000-0000-000000000002', '技术中心', 1, 'tech', 'root.tech'),
('00000000-0000-0000-0000-000000000003', '产品中心', 1, 'product', 'root.product'),
('00000000-0000-0000-0000-000000000004', '前端组', 2, 'fe', 'root.tech.fe'),
('00000000-0000-0000-0000-000000000005', '后端组', 2, 'be', 'root.tech.be');

-- 更新 parent_id
UPDATE organizations SET parent_id = NULL WHERE code = 'root';
UPDATE organizations SET parent_id = '00000000-0000-0000-0000-000000000001'
  WHERE code IN ('tech', 'product');
UPDATE organizations SET parent_id = '00000000-0000-0000-0000-000000000002'
  WHERE code IN ('fe', 'be');

-- 角色
INSERT INTO roles (id, code, name, description) VALUES
('00000000-0000-0000-0000-000000000010', 'admin', '管理员', '系统管理员，拥有全部权限'),
('00000000-0000-0000-0000-000000000011', 'editor', '编辑', '内容编辑，可管理文章'),
('00000000-0000-0000-0000-000000000012', 'viewer', '访客', '只读访问');

-- 超级管理员用户（密码: admin123，bcrypt hash 需实际生成）
INSERT INTO users (id, username, password, real_name, status) VALUES
('00000000-0000-0000-0000-000000000020', 'admin', '$2a$12$xxxxx', '系统管理员', 1);

-- admin 用户关联到集团组织
INSERT INTO user_orgs (user_id, org_id, role_id, is_primary) VALUES
('00000000-0000-0000-0000-000000000020',
 '00000000-0000-0000-0000-000000000001',
 '00000000-0000-0000-0000-000000000010',
 true);

-- 组织级权限模板
INSERT INTO org_permissions (role_id, resource_type, action, scope) VALUES
('editor', 'article', 'read', 2),   -- 编辑可读本组及子组的文章
('editor', 'article', 'write', 1),  -- 编辑只能写本组文章
('viewer', 'article', 'read', 1);   -- 访客只能读本组文章
```

---

## 11. 部署架构

### 11.1 开发环境

```yaml
# deployments/docker-compose.yaml
version: '3.8'
services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_DB: zhuzhao
      POSTGRES_USER: zhuzhao
      POSTGRES_PASSWORD: zhuzhao_dev
    ports:
      - "5432:5432"
    volumes:
      - pg_data:/var/lib/postgresql/data
      - ./init.sql:/docker-entrypoint-initdb.d/init.sql  # 自动建表

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data

volumes:
  pg_data:
  redis_data:
```

### 11.2 配置文件

```yaml
# configs/config.yaml
server:
  port: 8080
  mode: debug  # debug / release

database:
  host: localhost
  port: 5432
  user: zhuzhao
  password: zhuzhao_dev
  dbname: zhuzhao
  max_open_conns: 25
  max_idle_conns: 5

redis:
  host: localhost
  port: 6379
  db: 0
  password: ""

jwt:
  secret: "your-secret-key"
  access_ttl: 2h
  refresh_ttl: 168h  # 7天

casbin:
  model: "configs/casbin_model.conf"

log:
  level: debug       # debug / info / warn / error
  dir: "logs"
  max_size: 100      # MB
  max_backups: 7
  max_age: 30        # days
```

---

## 12. 并发与可靠性

> 本章梳理架构中的并发场景、事务性要求、分布式锁需求等横切关注点。

### 12.1 并发问题与应对

| 场景 | 问题描述 | 应对方案 |
|------|----------|----------|
| **RT 并发刷新** | 多标签页同时用同一 RT 刷新，轮换后只有第一个成功 | 前端加请求防抖（同一 RT 只发一次刷新）；后端返回明确错误码 `token_already_refreshed`，提示客户端用最新 RT |
| **登录 Redis 非原子** | 存 RT（SET）和加入设备列表（SADD）是两个操作，中间崩溃产生孤儿 | 用 Redis Lua 脚本或 MULTI/EXEC 包裹两步操作，保证原子性 |
| **Casbin 策略并发修改** | 两个管理员同时修改同一角色策略，互相覆盖 | Casbin `SyncedEnforcer` 内部有写锁；DB 层用乐观锁（version 字段）或行锁 |
| **审计日志 channel 满** | 高负载下 channel 满直接丢日志 | 增大 channel 缓冲；满时降级为同步写入并告警；详见 12.4 审计日志可靠性 |
| **组织树 path 更新** | 移动节点需更新所有子节点 path，与并发读冲突 | 加分布式锁（12.2）；读操作走缓存快照，写操作在事务内完成 |
| **缓存击穿** | 多请求同时 cache miss，全部打到 DB | `singleflight`（单实例）或 Redis NX 锁（多实例），只放一个请求回源 |

### 12.2 分布式锁

当前阶段多实例部署非必须，但设计中预留分布式锁的使用场景：

| 场景 | 是否需要 | 锁实现 | 说明 |
|------|----------|--------|------|
| 组织树结构变更 | ✅ | Redis 锁 `lock:org:tree` | 防止并发修改 org path，持有期间阻塞其他写操作 |
| Casbin 策略批量重载 | ✅ | Redis 锁 `lock:casbin:reload` | 多实例收到 Watcher 通知后，只有一个实例执行 reload |
| 缓存重建 | ✅ | Redis NX 或 `singleflight` | 防止缓存击穿，只放一个请求回源重建缓存 |
| RT 刷新 | ❌ | 不需要 | Redis `GETSET` 原子操作天然防并发 |

**锁实现方式**：Redis `SET key value NX PX ttl`，释放时用 Lua 脚本验证 owner 后删除，防止误删。

### 12.3 事务性分析

#### 跨存储操作（不可事务化）

| 操作 | 涉及存储 | 失败处理策略 |
|------|----------|-------------|
| 登录 | DB 读（校验用户） + Redis 写（存 RT + 设备列表） | Redis 写失败 → 返回 500，用户重试登录；不会产生脏数据 |
| 登出 | Redis 删（RT + 设备列表 + AT 黑名单） | 三步用 Lua 脚本原子化；部分失败不影响安全（最多 RT 残留到 TTL 过期） |
| 踢出设备 | Redis 删（RT + 设备列表） | 同登出，Lua 脚本原子化 |

#### 同库事务（可事务化）

| 操作 | 涉及表 | 事务保证 |
|------|--------|----------|
| 角色权限变更 | Casbin 策略表 + org_permissions | 同一 PostgreSQL 事务，全成功或全回滚 |
| 用户-角色分配 | user_orgs + Casbin g 表 | 同一 PostgreSQL 事务 |
| 组织结构变更 | organizations + user_orgs + resource_owners | 同一 PostgreSQL 事务，含 path 递归更新 |
| 菜单变更 | menus + role_menus | 同一 PostgreSQL 事务 |

#### DB 事务 + 缓存失效（最终一致性）

| 操作 | 流程 | 失败处理 |
|------|------|----------|
| 权限变更 | ① DB 事务提交 → ② Casbin 内存更新 → ③ 清除 Redis 权限缓存 | ② 失败：DB 已提交，通过 Watcher 通知重载；③ 失败：缓存有 TTL，自然过期 |
| 组织变更 | ① DB 事务提交（含 path 更新） → ② 清除相关缓存 | ② 失败：缓存有 TTL，自然过期；可接受短暂不一致 |

**原则**：先写 DB（事务保证），再更新内存/缓存。缓存更新失败不影响数据正确性，只影响性能（TTL 过期后自动重建）。

### 12.4 审计日志可靠性

当前设计用 channel + goroutine 异步写入，存在丢日志风险。改进方案：

| 级别 | 方案 | 可靠性 | 复杂度 |
|------|------|--------|--------|
| L1（当前） | channel + goroutine → DB | 进程崩溃丢 channel 内日志 | 低 |
| L2（推荐） | channel → Redis List（持久化）→ goroutine 消费 → DB | 进程崩溃不丢，Redis 持久化 | 中 |
| L3（重型） | Kafka/RabbitMQ → 消费者 → DB | 最高可靠性，支持重放 | 高 |

**推荐 L2**：Redis List (`LPUSH` + `BRPOP`) 做轻量队列，兼顾可靠性和简单性。当前阶段不引入重量级 MQ。

### 12.5 跨实例事件广播

多实例部署时，需要通过 Redis Pub/Sub 广播以下事件：

| 事件 | Channel | 订阅者行为 |
|------|---------|-----------|
| Casbin 策略变更 | `casbin:policy:changed` | 触发 enforcer reload（加分布式锁） |
| 用户被禁用 | `user:disabled:{userId}` | 清除该用户的权限缓存；其 AT 继续有效到过期或下次刷新被拒 |
| 权限缓存失效 | `cache:invalidate:{key}` | 删除本地/Redis 缓存 |

> 单实例阶段不需要 Pub/Sub，多实例部署时启用。

### 12.6 缓存策略

| 缓存对象 | Redis Key | TTL | 失效触发 |
|----------|-----------|-----|----------|
| 用户权限码列表 | `perm:user:{userId}` | 30min | 角色权限变更、用户角色变更 |
| 用户菜单树 | `menu:user:{userId}` | 30min | 菜单变更、角色菜单关联变更 |
| 用户组织列表 | `orgs:user:{userId}` | 30min | 用户组织关系变更 |
| 组织树全量 | `org:tree` | 60min | 组织结构变更 |
| ReBAC 权限判断结果 | `authz:{userId}:{resType}:{resId}:{action}` | 5min | 权限变更时按 user 粒度清除 |

**缓存模式**：Cache-Aside（先查缓存，miss 查 DB 再回填）

**防击穿**：`singleflight`（同进程同 key 只放一个请求回源）

---

## 13. 安全加固

> 本章补充安全相关的遗漏设计。

### 13.1 密码安全

| 措施 | 说明 |
|------|------|
| 密码存储 | bcrypt，cost ≥ 12 |
| 密码复杂度 | 最少 8 位，含大小写+数字+特殊字符（可配置） |
| 密码历史 | 记录最近 5 次密码 hash，防止重用（新增 `password_history` 表） |
| 密码重置 | 通过邮箱/手机发送一次性 token（存 Redis，TTL 15min），验证后重置 |
| 密码过期 | 可选策略，如 90 天强制修改（企业合规需要时启用） |

### 13.2 登录安全

| 措施 | 说明 |
|------|------|
| 登录限流 | 同 IP 5 次/分钟；同账号 5 次/5 分钟（Redis 计数器） |
| 账号锁定 | 连续失败 5 次锁定 15 分钟（Redis key `lock:login:{username}`） |
| 验证码 | 登录失败 3 次后要求图形验证码（可选，后期补充） |
| 异地登录检测 | 对比本次登录 IP 与上次 IP 的地理归属，异常时记录审计日志或要求二次验证 |

### 13.3 API 安全

| 措施 | 说明 |
|------|------|
| CORS | 白名单域名配置，通过中间件处理 |
| SQL 注入 | 全部使用参数化查询（pgx 原生支持） |
| 请求体大小限制 | 中间件限制 `max_body_size`（如 1MB） |
| 安全响应头 | `X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY` 等 |
| HTTPS | 生产环境强制 TLS（由反向代理/负载均衡层处理） |

### 13.4 配置安全

| 敏感配置 | 存储方式 |
|----------|----------|
| JWT secret | 环境变量 `JWT_SECRET`，不写入配置文件 |
| 数据库密码 | 环境变量 `DB_PASSWORD` |
| Redis 密码 | 环境变量 `REDIS_PASSWORD` |

Viper 支持环境变量覆盖配置文件值，生产环境通过环境变量注入敏感配置。

---

## 14. 运维与可观测性

### 14.1 优雅关闭

```
收到 SIGTERM/SIGINT
  │
  ├─ 1. 停止接受新请求（关闭 Gin listener）
  ├─ 2. 等待 in-flight 请求完成（带超时，如 30s）
  ├─ 3. 刷空审计日志队列（等待 channel 消费完，带超时）
  ├─ 4. 关闭 Casbin enforcer
  ├─ 5. 关闭 Redis 连接
  ├─ 6. 关闭 PostgreSQL 连接池
  └─ 7. 退出进程
```

### 14.2 健康检查

| 探针 | 路径 | 检查内容 |
|------|------|----------|
| Liveness | `/health/live` | 进程存活（直接返回 200） |
| Readiness | `/health/ready` | DB 连通性 + Redis 连通性（ping 两者，全通过返回 200） |

### 14.3 可观测性（后期补充）

| 维度 | 工具 | 说明 |
|------|------|------|
| Metrics | Prometheus + promhttp | 请求 QPS、延迟分布、错误率、DB 连接池状态 |
| 分布式追踪 | OpenTelemetry | 跨中间件/Service/Repository 的调用链追踪 |
| 错误追踪 | Sentry（可选） | panic 和 5xx 错误自动上报 |

> 当前阶段先实现健康检查，Metrics 和追踪后期按需补充。

### 14.4 数据库迁移

使用 `golang-migrate` 管理 Schema 版本：

```
migrations/
├── 000001_init.up.sql      # 初始建表
├── 000001_init.down.sql
├── 000002_seed_roles.up.sql # 种子数据
├── 000002_seed_roles.down.sql
└── ...
```

---

## 15. 推荐库与开源参考

> 按模块汇总推荐的 Go 库及可选参考项目。**推荐**列为建议采用，**备选**列为同类替代，**参考**列为可学习的开源项目。

### 15.1 核心基础设施

| 模块/用途 | 推荐 | 备选 | 说明 |
|-----------|------|------|------|
| HTTP 框架 | `gin-gonic/gin` | `labstack/echo`、`go-chi/chi` | Gin 中间件生态最成熟 |
| 配置管理 | `spf13/viper` | `kelseyhightower/envconfig`（纯环境变量） | Viper 支持文件+环境变量+热更新 |
| 依赖注入 | `google/wire` | `uber-go/fx`（运行时 DI） | Wire 编译时生成，类型安全，无运行时开销 |
| 日志（结构化） | Go 标准库 `log/slog` | `uber-go/zap`、`zerolog` | slog 足够，标准库零依赖 |
| 日志切割 | `natefinch/lumberjack` | — | 按大小/时间轮转，与 slog 配合 |

### 15.2 数据库与 ORM

| 模块/用途 | 推荐 | 备选 | 说明 |
|-----------|------|------|------|
| PostgreSQL 驱动 | `jackc/pgx/v5` | `lib/pq`（已停止维护） | pgx 原生协议，性能最优，支持 `pgxpool` 连接池 |
| 代码生成/查询构建 | `sqlc`（编译时生成类型安全代码） | `Masterminds/squirrel`（查询构建器）、`uptrace/bun`（轻量 ORM） | **推荐 sqlc**：写 SQL → 生成 Go 代码，类型安全且无 ORM 魔法 |
| 数据库迁移 | `golang-migrate/migrate` | `pressly/goose`、`ariga/atlas` | golang-migrate 支持 SQL 文件和 CLI，生态成熟 |
| Redis 客户端 | `redis/go-redis/v9` | `gomodule/redigo` | go-redis 官方维护，支持 context、pipeline、Pub/Sub |

### 15.3 认证与 Token

| 模块/用途 | 推荐 | 备选 | 说明 |
|-----------|------|------|------|
| JWT 签发与解析 | `golang-jwt/jwt/v5` | — | 社区标准 JWT 库，维护活跃 |
| 密码哈希 | `golang.org/x/crypto/bcrypt` | `x/crypto/argon2`（更安全但更重） | bcrypt 足够，cost ≥ 12 |
| UUID 生成 | `google/uuid` | `gofrs/uuid` | uuid.New() 生成 UUIDv4 |
| CORS 中间件 | `gin-contrib/cors` | — | Gin 官方 contrib，配置简单 |

### 15.4 Casbin 生态

| 模块/用途 | 推荐 | 说明 |
|-----------|------|------|
| Casbin 核心 | `casbin/casbin/v2` | 使用 `SyncedEnforcer` 支持并发安全 |
| PostgreSQL Adapter | `casbin/casbin-pg-adapter` | 注意确认与 Casbin v2 版本兼容 |
| Redis Watcher | `casbin/redis-watcher` | 多实例部署时用于策略同步广播 |

### 15.5 中间件与安全

| 模块/用途 | 推荐 | 备选 | 说明 |
|-----------|------|------|------|
| 限流 | `ulule/limiter/v3`（Redis 后端） | `didip/tollbooth`、自研（Redis + Lua） | ulule/limiter 支持多种限流策略和存储后端 |
| 请求 ID | `gin-contrib/requestid` | — | 自动生成 X-Request-Id，串联日志 |
| gzip 压缩 | `gin-contrib/gzip` | — | 响应压缩 |
| 输入校验 | `go-playground/validator/v10` | — | Gin 内置 binding 使用，标签式校验 |

### 15.6 并发与缓存

| 模块/用途 | 推荐 | 备选 | 说明 |
|-----------|------|------|------|
| 缓存防击穿 | `golang.org/x/sync/singleflight` | — | 标准库扩展，同 key 只放一个请求回源 |
| 分布式锁 | 自研（Redis `SET NX PX` + Lua 释放） | `bsm/redislock`、`redsync/redsync` | 简单场景自研足够；复杂场景用 redsync（Redlock 算法） |
| 优雅关闭 | Go 1.16+ 标准库 `os/signal` + `http.Server.Shutdown` | — | Gin 本身不管理关闭，需在 app 层编排 |

### 15.7 API 文档与测试

| 模块/用途 | 推荐 | 备选 | 说明 |
|-----------|------|------|------|
| Swagger 生成 | `swaggo/swag` + `swaggo/gin-swagger` + `swaggo/files` | — | 注解生成 OpenAPI 文档，Gin 集成 |
| HTTP 测试 | `gin-gonic/gin` 内置 `httptest` | — | Go 标准库 `net/http/httptest` |
| 断言库 | `stretchr/testify` | `smartystreets/goconvey` | testify 是 Go 测试断言事实标准 |
| Mock | `uber-go/mock`（原 `gomock`） | `matryer/moq` | uber-go/mock 是 gomock 的活跃 fork |
| 测试容器 | `testcontainers/testcontainers-go` | — | 集成测试启动真实 PG + Redis 容器 |

### 15.8 可观测性（后期补充）

| 模块/用途 | 推荐 | 说明 |
|-----------|------|------|
| Metrics | `prometheus/client_golang` | Prometheus 官方 Go 客户端 |
| Gin Metrics 中间件 | `gin-contrib/prometheus` | 自动采集 HTTP 指标 |
| 分布式追踪 | `go.opentelemetry.io/otel` | OpenTelemetry 标准，支持多种后端 |

### 15.9 开源参考项目

以下开源项目可作为架构和实现参考（非直接使用）：

| 项目 | 仓库 | 参考价值 |
|------|------|----------|
| **gin-admin** | `github.com/LyricTian/gin-admin` | Gin + Casbin + RBAC 的完整后台架构，目录分层、中间件组织方式值得参考 |
| **go-admin** | `github.com/go-admin-team/go-admin` | Gin + Casbin + RBAC + 动态路由，功能与本模块高度重合，可对比设计差异 |
| **Casdoor** | `github.com/casdoor/casdoor` | Casbin 团队出品的 IAM 系统，认证、授权、用户管理一站式，权限模型设计可参考 |
| **GoZero** | `github.com/zeromicro/go-zero` | 微服务框架，其限流、熔断、缓存封装可参考 |
| **Kratos** | `github.com/go-kratos/kratos` | Bilibili 微服务框架，配置管理、日志、依赖注入的组织方式可参考 |
| **Ory Kratos/Keto** | `github.com/ory/kratos` + `github.com/ory/keto` | Ory 身份认证体系，Kratos 做身份管理，Keto 做权限服务器（ReBAC），ReBAC 设计可参考 |

> **注意**：参考项目的目的是学习架构模式和设计思路，不建议直接复制代码。每个项目的技术栈、业务场景、约束条件不同，需结合自身需求取舍。

### 15.10 库版本管理建议

- `go.mod` 中对关键依赖（Casbin、pgx、go-redis）使用语义化版本锁定
- 定期检查 `go list -m -u all` 更新依赖
- Casbin 和 pgx 的大版本升级需充分测试（API 不保证向后兼容）

---

## 16. 统一响应与错误处理

### 16.1 统一响应格式

所有 API 返回统一 JSON 结构：

```json
{
  "code": 0,
  "message": "success",
  "data": {},
  "request_id": "req_xxx"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| code | int | 业务码，0=成功，非 0=失败 |
| message | string | 描述信息 |
| data | any | 业务数据，失败时为 null |
| request_id | string | 请求 ID（来自中间件，串联日志） |

分页响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "list": [],
    "total": 100,
    "page": 1,
    "page_size": 20
  },
  "request_id": "req_xxx"
}
```

### 16.2 错误码设计

错误码按模块分段：

| 范围 | 模块 | 示例 |
|------|------|------|
| 0 | 通用成功 | `0 = success` |
| 10000-10999 | 通用错误 | `10001 = 参数错误`、`10002 = 未授权`、`10003 = 禁止访问` |
| 20000-20999 | 认证模块 | `20001 = 用户名或密码错误`、`20002 = token 已过期`、`20003 = token 已失效`、`20004 = 刷新令牌无效` |
| 30000-30999 | 用户模块 | `30001 = 用户已存在`、`30002 = 用户不存在`、`30003 = 用户已禁用` |
| 40000-40999 | 角色模块 | `40001 = 角色已存在`、`40002 = 角色不存在` |
| 50000-50999 | 组织模块 | `50001 = 组织已存在`、`50002 = 组织不存在`、`50003 = 不能移动到子节点下` |
| 60000-60999 | 菜单模块 | `60001 = 菜单已存在`、`60002 = 菜单不存在` |
| 70000-70999 | 权限模块 | `70001 = 无权限`、`70002 = 策略已存在` |
| 80000-80999 | 审计模块 | `80001 = 日志查询参数错误` |

HTTP 状态码映射：

| HTTP Status | 使用场景 |
|-------------|----------|
| 200 | 成功 |
| 400 | 参数校验失败 |
| 401 | 未认证（token 无效/过期） |
| 403 | 已认证但无权限 |
| 404 | 资源不存在 |
| 409 | 冲突（如用户名已存在） |
| 429 | 限流 |
| 500 | 服务器内部错误 |

### 16.3 错误处理链路

```
service 层 → 返回 errcode.Error（含业务码 + 消息）
    ↓
handler 层 → 识别 errcode，转换为统一响应格式 + HTTP 状态码
    ↓
middleware 层 → recovery 中间件兜底未处理的 panic
```

---

## 17. API 路由总表

> 完整的 API 端点清单，按模块分组。

### 17.1 认证模块

| 路由 | 方法 | 鉴权 | 说明 |
|------|------|------|------|
| `/api/v1/auth/login` | POST | ❌ | 登录，返回双 Token |
| `/api/v1/auth/refresh` | POST | ❌ | 刷新 Token，RT 轮换 |
| `/api/v1/auth/logout` | POST | ✅ | 登出 |
| `/api/v1/auth/devices` | GET | ✅ | 查询活跃设备列表 |
| `/api/v1/auth/devices/:deviceId` | DELETE | ✅ | 踢出指定设备 |
| `/api/v1/auth/password` | PUT | ✅ | 修改密码 |
| `/api/v1/auth/password/reset` | POST | ❌ | 密码重置（通过邮箱/手机） |

### 17.2 用户模块

| 路由 | 方法 | 鉴权 | 说明 |
|------|------|------|------|
| `/api/v1/users` | GET | ✅ | 用户列表（分页+筛选） |
| `/api/v1/users` | POST | ✅ | 创建用户 |
| `/api/v1/users/:id` | GET | ✅ | 用户详情 |
| `/api/v1/users/:id` | PUT | ✅ | 更新用户 |
| `/api/v1/users/:id` | DELETE | ✅ | 删除用户（软删除） |
| `/api/v1/users/:id/status` | PATCH | ✅ | 启用/禁用用户 |
| `/api/v1/users/:id/orgs` | GET | ✅ | 用户所属组织列表 |
| `/api/v1/users/:id/roles` | PUT | ✅ | 分配用户角色 |
| `/api/v1/user/menus` | GET | ✅ | 当前用户菜单树 |
| `/api/v1/user/permissions` | GET | ✅ | 当前用户权限码 |
| `/api/v1/user/profile` | GET | ✅ | 当前用户信息 |
| `/api/v1/user/profile` | PUT | ✅ | 更新个人信息 |

### 17.3 角色模块

| 路由 | 方法 | 鉴权 | 说明 |
|------|------|------|------|
| `/api/v1/roles` | GET | ✅ | 角色列表 |
| `/api/v1/roles` | POST | ✅ | 创建角色 |
| `/api/v1/roles/:id` | GET | ✅ | 角色详情 |
| `/api/v1/roles/:id` | PUT | ✅ | 更新角色 |
| `/api/v1/roles/:id` | DELETE | ✅ | 删除角色 |
| `/api/v1/roles/:id/menus` | GET | ✅ | 角色关联菜单 |
| `/api/v1/roles/:id/menus` | PUT | ✅ | 分配角色菜单 |
| `/api/v1/roles/:id/permissions` | GET | ✅ | 角色权限策略 |

### 17.4 组织模块

| 路由 | 方法 | 鉴权 | 说明 |
|------|------|------|------|
| `/api/v1/orgs` | GET | ✅ | 组织树 |
| `/api/v1/orgs` | POST | ✅ | 创建组织 |
| `/api/v1/orgs/:id` | GET | ✅ | 组织详情 |
| `/api/v1/orgs/:id` | PUT | ✅ | 更新组织 |
| `/api/v1/orgs/:id` | DELETE | ✅ | 删除组织 |
| `/api/v1/orgs/:id/move` | PATCH | ✅ | 移动组织（变更父节点） |
| `/api/v1/orgs/:id/members` | GET | ✅ | 组织成员列表 |
| `/api/v1/orgs/:id/members` | POST | ✅ | 添加成员到组织 |

### 17.5 菜单模块

| 路由 | 方法 | 鉴权 | 说明 |
|------|------|------|------|
| `/api/v1/menus` | GET | ✅ | 菜单树（全量） |
| `/api/v1/menus` | POST | ✅ | 创建菜单 |
| `/api/v1/menus/:id` | GET | ✅ | 菜单详情 |
| `/api/v1/menus/:id` | PUT | ✅ | 更新菜单 |
| `/api/v1/menus/:id` | DELETE | ✅ | 删除菜单 |

### 17.6 审计模块

| 路由 | 方法 | 鉴权 | 说明 |
|------|------|------|------|
| `/api/v1/audit-logs` | GET | ✅ | 审计日志查询（分页+筛选） |

### 17.7 系统模块

| 路由 | 方法 | 鉴权 | 说明 |
|------|------|------|------|
| `/health/live` | GET | ❌ | 存活探针 |
| `/health/ready` | GET | ❌ | 就绪探针 |
| `/swagger/*` | GET | ❌ | Swagger 文档 |

---

## 18. 分阶段实施计划

> 按"先跑起来再增强"的原则，分三个阶段实施。

### 18.1 Phase 1：最小可用（跑起来）

**目标**：系统能启动、能登录、能鉴权、能管理基础数据。

| 能力 | 范围 | 说明 |
|------|------|------|
| 项目骨架 | 完整目录结构 + go.mod + Wire | 能 `go build` 通过 |
| 配置加载 | Viper 读取 config.yaml | DB/Redis/JWT 配置 |
| 数据库 | PG + Redis Docker Compose + 迁移脚本 | 一键启动开发环境 |
| 统一响应 | response 包 + errcode 包 | 统一 JSON 输出 |
| 健康检查 | `/health/live` + `/health/ready` | K8s/Docker 探针 |
| 种子数据 | 初始化 admin 角色 + 超管用户 + 初始菜单 | 首次启动可用 |
| 用户登录 | 账号密码 + 双 Token 签发 | AT + RT |
| Token 校验 | JWT 中间件 + 黑名单 | 路由级认证 |
| Token 刷新 | RT 轮换 | 无感刷新 |
| 登出 | AT 黑名单 + RT 删除 | 会话结束 |
| 路由级鉴权 | Casbin RBAC 中间件 | 接口权限控制 |
| 用户管理 | CRUD + 启用禁用 | 基础用户管理 |
| 角色管理 | CRUD + 菜单分配 | 基础角色管理 |
| 菜单管理 | CRUD | 基础菜单管理 |
| 动态路由 | `/user/menus` + `/user/permissions` | 前端权限数据 |
| 优雅关闭 | signal 处理 + 资源释放 | 不丢请求 |

### 18.2 Phase 2：核心完善（业务可用）

**目标**：组织架构、资源级鉴权、审计日志、安全加固。

| 能力 | 范围 | 说明 |
|------|------|------|
| 组织架构 | 组织 CRUD + 树形展示 + 移动节点 | 实体组织 + 虚拟组 |
| 用户-组织 | 用户分配到组织 + 组织内角色 | 多对多关系 |
| 资源级鉴权 | ReBAC 关系遍历 + 属主判断 | 第二层 + 第三层权限 |
| 组织级权限 | org_permissions 管理 | scope 配置 |
| 审计日志 | 中间件记录 + 异步写入 + 查询 | L1 channel 方案 |
| 多设备管理 | 设备列表 + 踢出设备 | Redis 设备管理 |
| 登录安全 | 限流 + 账号锁定 | Redis 计数器 |
| 密码安全 | 复杂度校验 + 修改密码 | bcrypt |
| 缓存 | 权限缓存 + 菜单缓存 + 组织缓存 | Cache-Aside |
| 限流中间件 | Redis + 令牌桶/滑动窗口 | API 限流 |

### 18.3 Phase 3：生产加固（可上线）

**目标**：可观测性、多实例、高可用。

| 能力 | 范围 | 说明 |
|------|------|------|
| Metrics | Prometheus + Grafana | QPS/延迟/错误率 |
| 分布式追踪 | OpenTelemetry | 调用链 |
| Casbin Watcher | Redis Pub/Sub 策略同步 | 多实例 |
| 跨实例事件 | Redis Pub/Sub 广播 | 缓存失效/用户禁用 |
| 审计日志升级 | Redis List 队列（L2） | 不丢日志 |
| 分布式锁 | 组织树变更 + 缓存重建 | Redis NX + Lua |
| 数据库迁移 | golang-migrate CLI + CI 集成 | 版本管理 |
| Swagger | 注解完善 + CI 生成 | API 文档 |
| 集成测试 | testcontainers-go | 真实容器测试 |
| 密码重置 | 邮箱/手机 token | 重置流程 |
| 异地登录检测 | IP 地理位置比对 | 安全审计 |

### 18.4 预留扩展（按需启用）

| 能力 | 预留点 | 启用条件 |
|------|--------|----------|
| 多租户 | tenant_id + Casbin 模型 | 有多客户需求时 |
| 第三方登录 | oauth_provider + oauth_id | 接 SSO/OAuth 时 |
| 消息队列 | 审计日志通道可替换 | 日志量大或需重放时 |
| 验证码 | 登录接口预留参数 | 安全要求提高时 |
| 密码过期 | users 表无额外字段 | 合规要求时 |

---

## 19. 待决策事项

| 事项 | 当前状态 | 备注 |
|------|----------|------|
| PostgreSQL 环境搭建 | ✅ 已确定 | Docker Compose 单机 |
| Casbin 策略存储 | ✅ 已确定 | PostgreSQL |
| 权限模型 | ✅ 已确定 | RBAC（路由级）+ ReBAC（资源级）+ ABAC（属主） |
| 日志库 | ✅ 已确定 | slog + Lumberjack |
| 双 Token 机制 | ✅ 已确定 | AT(2h) + RT(7d) + RT 轮换 + 多设备管理 |
| 依赖注入 | ✅ 已确定 | Google Wire，编译时生成，无运行时开销 |
| 并发与事务 | ✅ 已梳理 | 见第 12 章：分布式锁 4 场景，跨存储操作失败策略已定义 |
| 安全加固 | ✅ 已补充 | 见第 13 章：密码安全、登录安全、API 安全、配置安全 |
| 运维可观测性 | ✅ 已补充 | 见第 14 章：优雅关闭、健康检查、DB 迁移；Metrics 后期补充 |
| 审计日志可靠性 | ✅ 改进 | Redis List 轻量队列（L2 方案），不引入重量级 MQ |
| 缓存策略 | ✅ 已设计 | Cache-Aside + singleflight 防击穿，按 user 粒度失效 |
| 统一响应与错误码 | ✅ 已补充 | 见第 16 章：统一 JSON 结构 + 分段错误码 + HTTP 映射 |
| API 路由总表 | ✅ 已补充 | 见第 17 章：7 个模块完整端点清单 |
| 分阶段实施计划 | ✅ 已制定 | 见第 18 章：Phase 1 跑起来 → Phase 2 业务可用 → Phase 3 生产加固 |
| Schema 完善 | ✅ 已补充 | 软删除、乐观锁、负责人、第三方登录预留、扩展字段 |
| 种子数据 | ✅ 已补充 | admin 角色 + 超管用户 + 组织关联 |
| 消息队列 | ⏳ 暂不需要 | 当前阶段 Redis Pub/Sub + List 足够，后期按需评估 |
| 多租户 | ⏳ 预留 | 表和模型预留 tenant_id，暂不实现 |
| API 版本管理 | ✅ 已确定 | `/api/v1` 前缀 |
| 是否开始搭建代码 | ⏳ 待确认 | 方案基本完整，可开始 Phase 1 骨架搭建 |
| log sampling | ⏳ 后期评估 | 如需要可自定义 slog Handler |
| Casbin Watcher | ⏳ Phase 3 | 多实例部署时必须 |
| 验证码 / 异地登录检测 | ⏳ Phase 3 | 登录安全增强，当前先做限流+锁定 |
| Metrics + 分布式追踪 | ⏳ Phase 3 | 先实现健康检查，可观测性按需补充 |
| 各模块实现细节 | ⏳ 待补充 | 边界已定，后续逐模块讨论实现 |
