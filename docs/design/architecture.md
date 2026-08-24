# 认证鉴权模块架构设计文档

> 文档版本：v0.6  
> 更新时间：2026-08-13  
> 状态：方案讨论阶段。**分阶段边界、主键类型、安全底线以 [`roadmap.md`](../roadmap.md) 与 [`phase1/`](../phase1/README.md) 为准。** 本文 §10 为表索引（DDL 见 phase1）；§18 已与 roadmap 对齐。

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
  - [16.4 API 设计规范](#164-api-设计规范)
- [17. API 路由总表](#17-api-路由总表)
- [18. 分阶段实施计划](#18-分阶段实施计划)
- [19. 决策状态总览](#19-决策状态总览)

---

## 1. 项目概述

### 1.1 模块目标

搭建一个通用的认证鉴权模块，提供以下核心能力：

| 功能 | 说明 |
|------|------|
| 用户登录 | 账号密码登录，双 Token 签发 |
| 认证 | Token 校验、无感刷新、登出、多设备管理 |
| 路由级鉴权 | 基于 RBAC，校验角色对 API 接口的访问权限 |
| 资源级鉴权 | 基于 ltree 组织关系查询 + 属主判断，校验对具体资源的操作权限 |
| 动态路由 | 接口返回权限树，驱动前端菜单和按钮渲染 |
| 组织架构 | 实体组织 + 虚拟组（项目组），权限沿组织树继承 |
| 审计日志 | 记录用户操作行为，支持按用户/组织/操作类型查询 |
| 多租户 | 预留多租户扩展能力 |

### 1.2 设计原则

- **按业务领域纵向切分，按职责横向分层**
- Casbin 只管路由级 RBAC，资源级鉴权用 ltree SQL + 代码内联（Phase 2 按需引入 PDP）
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
- 业务模块通过 `internal/pkg/resource` 的 Registry 进行资源级权限判断（Resource 自注册模式，见 [proposal/resource-model.md](../proposal/resource-model.md)：各业务 Service 构造时 `registry.Register(NewXxxResource(...))`，鉴权经 `Registry.Authorize` / `GetFilter`）
- 业务模块的资源表需包含 `creator_id` 字段以支持属主判断（Phase 2）
- 业务模块的资源可在 Phase 2b 通过 `resource_owners` 登记组织归属（可选；工单直接用 `tickets.org_path`，见 [phase2/02-authz-resource.md](../phase2/02-authz-resource.md)）

---

## 2. 技术选型

### 2.1 最终选型

| 技术 | 用途 | 推荐库/包 | 选型理由 |
|------|------|----------|----------|
| **Gin** | HTTP 框架 | `github.com/gin-gonic/gin` | 轻量、中间件生态成熟 |
| **Viper** | 配置管理 | `github.com/spf13/viper` | 支持多格式、热更新、环境变量覆盖 |
| **PostgreSQL** | 主数据库 | `github.com/jackc/pgx/v5` + `pgxpool` | 原生协议、连接池、性能优于 database/sql |
| **Casbin** | 路由级 RBAC | `github.com/casbin/casbin/v3` + `noho-digital/casbin-pgx-adapter` | pgx 原生 adapter，SyncedEnforcer |
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

#### 2.2.3 资源级鉴权方案

**决策**：路由级用 Casbin RBAC，资源级用基于 ltree 的组织关系查询 + 代码内联属主判断。Phase 2 按需评估引入 OpenFGA/SpiceDB 作为独立 PDP。

**理由**：

1. ltree 一条 SQL 做组织树层级判断，不需要 ReBAC 引擎。
2. 属主判断（`created_by == userID`）是简单 ABAC，代码内联最直接。
3. Phase 1 资源类型少、关系链浅，引入独立 ReBAC 服务过重。
4. 接口设计预留迁移路径，Phase 2 可替换为 OpenFGA/SpiceDB client。

> 详见 [design-decisions.md#12 ReBAC 引擎选型](./design-decisions.md#12-rebac-引擎选型自研-vs-openfga-vs-spicedb)

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
│   ├── repository/                # 数据访问层（目标：repository/{domain}/）
│   │   ├── user/
│   │   ├── role/
│   │   ├── org/
│   │   ├── menu/
│   │   └── audit/
│   │
│   ├── service/                   # 业务逻辑层（目标：service/{domain}/，见 §3.5）
│   │   ├── auth/                  # 或骨架期 auth_service.go
│   │   ├── user/
│   │   ├── role/
│   │   ├── authz/
│   │   ├── org/
│   │   ├── menu/
│   │   └── audit/
│   │
│   ├── middleware/                # Gin 中间件（仅路由级横切，不含资源级鉴权）
│   │   ├── jwt.go                 # JWT 解析与校验 + 黑名单 + user:disabled
│   │   ├── casbin.go              # 路由级 RBAC 中间件
│   │   ├── audit.go               # 审计日志记录
│   │   └── recovery.go            # Panic 恢复
│   │
│   ├── handler/                   # HTTP 处理器（目标：handler/{domain}/）
│   │   ├── auth/
│   │   ├── user/
│   │   ├── role/
│   │   ├── org/
│   │   ├── menu/
│   │   └── audit/
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
│   │   ├── resource/              # ResourceRegistry（资源级鉴权，Service 层调用）
│   │   ├── response/              # 统一响应封装
│   │   ├── errcode/               # 错误码定义
│   │   ├── logger/                # slog 日志封装
│   │   ├── redis/                 # Redis 客户端 + Lua 脚本（LoginLocker）
│   │   │   ├── redis.go
│   │   │   ├── scripts.go         # go:embed
│   │   │   └── scripts/
│   │   │       └── login_lock.lua
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
│   ├── README.md                  # 文档索引
│   ├── design/                    # 设计文档
│   │   ├── architecture.md        # 系统架构（本文档）
│   │   ├── design-decisions.md    # 设计决策与细节讨论
│   │   └── implementation-plan.md # 已废弃（见 phase1/README）
│   ├── api/                       # API 文档（Swagger 生成）
│   ├── ops/                       # 运维文档
│   └── adr/                       # 架构决策记录
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

> **与 §3.5 的关系**：上图为目标形态 `{layer}/{domain}/`；当前仓库骨架可能仍为扁平 `user_service.go` 等，新代码优先子目录，第二次加文件时整域迁入。

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

### 3.5 领域模块目录约定（单仓可拆分）

> 当前 **一个代码仓、一个进程**（Phase 1–2）；Phase 3 可能拆成 **IAM 底座 + 业务服务**（见 [deployment-evolution.md](../proposal/deployment-evolution.md)）。  
> **原则**：现在按 **领域（domain）** 划目录，将来按 **目录边界** 切仓库/服务，而不是按「当时谁写的文件」硬拆。

#### 3.5.1 两层结构：横向分层 + 纵向领域

```
internal/
├── app/、router/              # 进程编排（未来每个二进制一份）
├── middleware/、casbin/      # 横切 / 路由级鉴权（未来 → Gateway 或 IAM 边缘）
├── pkg/                        # 无业务语义的工具（可抽成公共 module）
├── model/                      # 领域模型（Phase 1 可扁平；领域增多再分子包）
│
├── handler/{domain}/             # HTTP 适配
├── service/{domain}/             # 业务逻辑 + 领域接口
├── repository/{domain}/          # 持久化
└── integration/{name}/           # 外部系统（如 hr/，Phase 2b）
```

| 轴 | 目录 | 拆分后归属 |
|----|------|------------|
| **横向** | `middleware`、`router`、`app` | Gateway 或各服务入口 |
| **横向** | `pkg/*`（response、errcode、logger、postgres、redis） | 公共库 `pkg/zhuzhao/...` 或各服务复制 |
| **纵向** | `auth`、`user`、`role`、`org`、`menu`、`authz`、`audit` | **IAM 服务** |
| **纵向** | `ticket`（Phase 2） | **业务服务** |
| **纵向** | `integration/hr`（Phase 2b） | IAM 内 Job，或独立 sync worker |

**骨架阶段**已有扁平文件（如 `internal/service/user_service.go`）可保留；**新增或改动较大的模块**优先落到 `{layer}/{domain}/` 子目录，避免继续在根上堆 `*_service.go`。

#### 3.5.2 领域与文档 / 未来服务映射

| 领域包 `{domain}` | 文档 | 未来进程 | 说明 |
|-------------------|------|----------|------|
| `auth` | [modules/auth.md](../modules/auth.md)、phase1/02-auth | IAM | 登录、Token、限流；依赖 `user` |
| `user` | modules/user、phase1/04-user | IAM | 用户 CRUD、角色绑定 |
| `role` | modules/role、phase1/05-role | IAM | 角色、菜单分配、策略同步 |
| `org` | modules/organization、phase1/06-organization | IAM | 组织树、成员 |
| `menu` | modules/menu、phase1/07-menu | IAM | 菜单树、menu_apis |
| `authz` | modules/authz、phase1/03-authz | IAM + Gateway | Casbin 策略、ResourceRegistry |
| `audit` | modules/audit、phase1/08-audit | IAM（或共享） | 操作审计 |
| `ticket` | modules/ticket | 业务服务 | Phase 2 起 |
| `integration/hr` | proposal/hr-directory-sync | IAM Job | HR 同步，勿与用户 CRUD 混包 |

#### 3.5.3 推荐目录示例（新代码目标形态）

```
internal/service/user/
    service.go          # UserService + 构造函数
    ports.go            # 可选：本领域对外需要的接口（如 UserReader）
    resource.go         # Phase 2：Resource 实现（勿放 handler）

internal/repository/user/
    repo.go             # UserRepo 实现

internal/handler/user/
    handler.go          # 只调 UserService，不直连接 Repo

internal/service/auth/
    service.go
internal/pkg/jwt/       # 认证专用库；拆 IAM 时随 auth 域迁移

internal/router/
    router.go           # 注册路由；按领域分组注释块

internal/app/
    wire.go             # Provider 按领域分段注释；拆分时整段复制到新 cmd
```

`internal/model/user.go` Phase 1 保持单文件即可；同一领域模型超过 3 个文件时再考虑 `model/user/`。

#### 3.5.4 跨领域调用规则（拆服务前就必须遵守）

| 允许 | 禁止 |
|------|------|
| `service/user` 注入 `service/role` 的 **接口** | `service/user` 直接 import `repository/role` |
| `service/org` 调用 `UserReader` 接口查用户 | `handler/org` 直接调 `UserRepo` |
| `auth` 调 `user` 验密码 | 领域间 **循环依赖**（应用层用 Wire 单向注入打破） |
| 事务由 **一个** Service 拥有并调 Repo | 在 handler 里开事务跨多个 Service |

拆分为 gRPC 后：原 `service/role` 接口 → `RoleServiceClient`；**调用方代码只改注入，不改业务方法签名**（见 design-decisions §13）。

#### 3.5.5 `internal/pkg` 边界

| 放入 `pkg/` | 不要放入 `pkg/` |
|-------------|-----------------|
| response、errcode、logger、postgres、redis、crypto | 带 `user_id` / 组织语义的 Resource 实现 |
| jwt（Phase 1 暂放 pkg，**语义属 auth 域**） | Casbin Enforcer 业务策略（应在 `casbin/` 或 `service/authz`） |
| resource **接口与 Registry 框架**（无具体资源） | 工单状态机、HR 对账逻辑 |

#### 3.5.6 拆分迁移检查清单

从单仓切出 `iam-server` / `ticket-server` 时，按目录打包：

1. `cmd/{service}/main.go` + `internal/app` 子集  
2. 对应 `handler/{domain}`、`service/{domain}`、`repository/{domain}`、`model/*`  
3. 该服务私有的 `migrations/` 表（或继续共享 PG schema，文档注明）  
4. `middleware` 中 JWT/Casbin 是否留在 Gateway — 见 deployment-evolution §4  

**Wire**：每个未来二进制一份 `wire.go`；Phase 1 单 `wire.go` 内用注释分块 `# region IAM` / `# region ticket`，便于复制。

#### 3.5.7 Phase 1 落地节奏

| 时机 | 动作 |
|------|------|
| Step 1  infra | 建 `migrations/`、`pkg/redis/scripts/`；**不**为了拆服务提前建多 cmd |
| Step 2+ 业务模块 | 新文件优先 `service/{domain}/`、`handler/{domain}/` |
| 仅 1 个文件的域 | 可暂用 `user_service.go`；第二次加文件（如 `resource.go`）时 **整域迁入子目录** |
| Phase 2 工单 | 新建 `service/ticket/`、`handler/ticket/`，**不**塞进 user/org |
| Phase 2b HR | 新建 `integration/hr/` + `service/org` 扩展，独立 client |

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
    │   ├── NewRegistry()      → resource.Registry（业务 Service 构造时自注册 Resource）
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

### 3.6 组件注册与生命周期（三者分离）

新增业务域或后台能力时，**不要**用一个「万能 Service 接口」同时承担依赖注入、鉴权、进程生命周期。本项目拆成三条独立机制：

| 机制 | 位置 | 职责 | 何时用 |
|------|------|------|--------|
| **Wire DI** | `internal/app/wire.go` | 编译期组装 Handler / Service / Repo / 基础设施 | 每个模块必走；新增 provider 即可 |
| **ResourceRegistry** | `internal/pkg/resource/` | 资源级鉴权：`Authorize` + `GetFilter` | Phase 2+ 需要数据范围时；`NewXxxService` 内 `registry.Register` |
| **Runner（可选）** | `internal/app/` | 后台 goroutine 的 `Start` / `Stop` | 仅审计 worker、Asynq、Casbin Watcher、定时同步等 |

```
                    ┌─────────────────────────────────────┐
                    │           cmd/server / App          │
                    └─────────────────────────────────────┘
           Wire 注入 │                    │ 生命周期编排
                    ▼                    ▼
    Handler ──▶ XxxService ──▶ Repo          Runners[]（Phase 3+）
                    │
                    └──▶ registry.Register(XxxResource)   ← 仅鉴权，无 Start/Stop
```

**`resource.Resource` 只管鉴权**，不包含 `Init` / `Run` / `Start` / `Stop`。各域 **`XxxService` 按用例定义业务方法**（`Create`、`List`…），不强制实现统一生命周期接口。

**基础设施**（PG、Redis、Casbin）沿用 Wire `New() (T, cleanup, error)`；`cleanup` 在进程退出最后阶段调用，见 [§14.1](#141-优雅关闭)。

**新增业务域 checklist**（与 [resource-model.md](../proposal/resource-model.md) 一致）：

1. `service/{domain}/` + `repository/{domain}/` + `handler/{domain}/` → 加入 Wire  
2. `router` 注册路由 + seed `menu_apis` + Casbin `p` 策略  
3. Phase 2+：另建 `{domain}/resource.go` 实现 `Resource`，构造函数自注册  
4. 仅有后台任务时：实现 `Runner`，由 `App` 编排；**不**改 ResourceRegistry 源码  

**多二进制部署**（[design-decisions §18](./design-decisions.md#18-部署与代码解耦一套代码多种部署)）：`cmd/server` 跑 HTTP；`cmd/worker`（Phase 3）单独 Wire Asynq consumer，共享 `internal/service`，不共用「万能 Service 接口」。

### 3.4 各模块对外接口边界

#### service 层接口（核心）

| Service | 对外方法 | 说明 |
|---------|----------|------|
| `AuthService` | `Login` | 登录，签发双 Token |
| | `Refresh` | 刷新 Token，RT 轮换 |
| | `Logout` | 登出，吊销 AT + 删除 RT |
| | `KickDevice` | 踢出指定设备 |
| | `ListDevices` | 查询用户活跃设备列表 |
| `resource.Registry` | `Authorize` / `GetFilter` / `Register` / `List` | 资源级权限校验（属主 → ltree 组织关系）与列表过滤；业务 Resource 自注册 |
| `MenuService` | `GetUserMenus` | 获取用户菜单树 |
| | `GetUserPermissions` | 获取用户按钮权限码列表 |
| `OrgService` | `GetOrgTree` | 获取组织树 |
| | `GetUserOrgs` | 获取用户所属组织列表 |
| `AuditService` | `Record` | 记录审计日志（Phase 1 **同步**写 DB） |
| | `Query` | 查询审计日志 |

#### middleware 层接口

| Middleware | 挂载位置 | 说明 |
|------------|----------|------|
| `Recovery` | 全局 | Panic 恢复 |
| `Logger` | 全局 | 请求日志 |
| `JWT` | 需认证路由 | Token 解析 + 黑名单 + `user:disabled`（Redis 故障 503） |
| `Casbin` | 需鉴权路由 | 路由级 RBAC（RoleFetcher 查角色） |
| `Audit` | 需审计路由 | 操作记录 |
| `RateLimit` | 登录等公开路由 | 登录限流在 **AuthService**（Lua）；API 级限流 Phase 3 |

> **资源级鉴权不在 middleware**。各 Service 通过 `internal/pkg/resource` 的 `ResourceRegistry.Authorize` / `GetFilter` 在 Handler→Service 内联执行（Phase 2 工单等）。详见 [design-decisions.md §5](./design-decisions.md#5-资源级鉴权架构gateway-下放-vs-集中)。

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
│  第二层：资源级鉴权（ltree 组织关系查询）                      │
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

**执行顺序**：AT 校验 → 第一层 Casbin → 第三层属主（短路） → 第二层 ltree 组织关系

> **架构定位**（2026-08-11 更新）：遵循 OWASP/NIST 分层鉴权标准，第一层（路由级）在中间件层执行（未来微服务化时由 API Gateway 承担），第二层和第三层（资源级）在 Service 层代码内联执行（未来由各业务服务自行承担）。Gateway 不做资源级鉴权，因为它缺少业务数据上下文。详见 [design-decisions.md#5](./design-decisions.md#5-资源级鉴权架构gateway-下放-vs-集中)。

### 4.2 第一层：路由级 RBAC（Casbin）

**职责**：校验"角色是否有权访问该 API 接口"。

**Casbin 模型**：RBAC，`sub`（角色）→ `obj`（API 路径）→ `act`（HTTP 方法），支持 `keyMatch2` 路径通配。

**策略量**：角色数 × API 数 × 方法数 ≈ 1,000 条，内存无压力。

**策略示例**（`role::editor` 为业务资源示例角色，非种子四角色）：

```
p, role::admin, /api/v1/*, *
p, role::editor, /api/v1/articles/*, GET
p, role::editor, /api/v1/articles/update, POST
```

> 无 Casbin `g` 段。用户→角色映射在 `user_roles` 表，中间件逐 `role::{code}` enforce。superadmin/admin 在 matcher bypass。

**热更新**：

- 单实例：AutoSave 模式，策略变更同时更新内存和 DB
- 多实例：Redis Watcher 模式，通过 Pub/Sub 广播策略变更通知

> 详细模型定义见 `configs/casbin_model.conf`，实现细节后续补充。

### 4.3 第二层：资源级鉴权（ltree 组织关系查询）

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

> **Phase 说明**：本 SQL 依赖 `resource_owners` 与 `org_permissions`，属 **Phase 2b+** 数据 scope 路径。Phase 2a 工单直接用 `tickets.org_id/org_path`，**不建** `resource_owners` 表（见 [phase2/02-authz-resource.md](../phase2/02-authz-resource.md)）。

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

### 4.5 资源抽象与自注册机制

第二层和第三层的鉴权逻辑通过统一的资源接口实现。每种资源类型（用户、角色、组织、工单等）实现 `Resource` 接口，在 Service 构造函数中自注册到 `ResourceRegistry`。Handler 通过 `registry.Authorize(ctx, resourceCode, req)` 统一调用。

详见 [design-decisions.md#6 资源抽象与自注册机制](./design-decisions.md#6-资源抽象与自注册机制)。

### 4.6 策略爆炸问题的解决

传统方案（全部塞进 Casbin）的策略量：

```
1000 用户 × 5 角色 × 50 资源 × 4 操作 × 3 组织层级 = 3,000,000 条
```

本方案的策略量：

| 层级 | 策略量 | 说明 |
|------|--------|------|
| 路由级 Casbin（全局唯一） | ~1,000 条 | 角色数 × API数 × 方法数 |
| 资源级代码内联 | 0 条 | 属主判断、组织关系（SQL ltree） |
| 资源级独立 enforcer（按需） | 每资源独立 | 仅策略可配置的复杂资源引入 |
| **总计** | **~1,000 条 + 按需** | 内存无压力 |

**每资源独立 Enforcer**：对于需要管理员可配置策略的复杂资源，为其创建独立的 Casbin enforcer 实例和独立策略表（`casbin_rule_{resource}`），避免策略互相影响。简单资源用代码内联判断，不引入 Casbin。详见 [design-decisions.md#8 Casbin 策略爆炸](./design-decisions.md#8-casbin-策略爆炸每资源独立-enforcer)。

---

## 5. 认证与 Token 机制

### 5.1 双 Token 方案概述

采用 **accessToken（AT）+ refreshToken（RT）** 双 Token 机制，RT 支持轮换。

| 维度 | 方案说明 |
|------|----------|
| AT 有效期 | **30min**，HS256 无状态 JWT，每次请求携带 |
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

### 5.2 Token Payload 设计原则

**核心原则：JWT 保持无状态，只存身份信息，不存权限信息。**

身份信息（user_id）不可变，适合放 JWT。权限信息（role、org_id）可变，放 JWT 会导致管理员修改权限后无法实时生效（必须等 AT 过期）。因此 AT 只携带最小化的身份标识，权限信息由 Redis 缓存提供，变更时主动失效。

**accessToken（Phase 1）**：

| 字段 | 类型 | 说明 |
|------|------|------|
| uid | int64 | 用户 ID（JSON `,string`） |
| username | string | 用户名 |
| jti | string | Token 唯一标识（黑名单） |
| mcp | bool | must_change_password |
| exp | timestamp | 过期时间 |

> 权限信息不入 JWT。Phase 1 路由级鉴权由 Casbin 中间件查 `user_roles`（无 Redis 权限缓存）。Phase 3 可按需加 `perm:user:{userId}` 缓存。

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

# 用户禁用/删除后即时吊销
# Key:   user:disabled:{userId}
user:disabled:1 → 1

# 用户权限缓存（Phase 3 / 按需，Phase 1 不使用）
# Key:   perm:user:{userId}
# Val:   JSON { roles, org_id, permissions }
# TTL:   30min
```

### 5.4 核心流程

#### 登录流程

```
1. 校验工号与密码
2. 签发 AT（**30min**，含 uid + username + jti + mcp）
3. 签发 RT（7d，精简 payload）
4. RT 存 Redis: refresh:{userId}:{deviceId}
5. 设备 ID 加入 Redis Set: devices:{userId}（Phase 1 可选，Phase 2 设备 UI）
6. 写登录审计
7. 返回 AT + RT
```

> Phase 1 **不写** `perm:user:{userId}` 缓存。路由级鉴权由后续 Casbin 中间件查 DB。

#### 刷新流程（RT 轮换）

```
1. 解析 RT，提取 userId, deviceId, jti
2. 查 Redis: refresh:{userId}:{deviceId}
   ├─ 不存在 → RT 已吊销 → 拒绝
   └─ 存在 → 比对 jti
       ├─ 不匹配 → 可能重放攻击 → 拒绝
       └─ 匹配 → 继续
3. 签发新 AT + 新 RT（`GETDEL` 轮换旧 RT）
4. 返回新 AT + 新 RT

※ Phase 1 无权限缓存，刷新 AT 不改变鉴权数据来源。
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

#### 非法与混用凭证（AuthN 拒绝原则）

> **SSOT（分类表、日志、伪代码）**：[phase1/02-auth.md §非法认证请求的处理](../phase1/02-auth.md#非法认证请求的处理实现必读)

| 原则 | 说明 |
|------|------|
| **互斥** | 受保护路由：JWT（Bearer）与 AK/SK **二选一**；同时出现 → **400/20008**，不以任一为准 |
| **早拒** | 混用检测在解析 JWT / 验 AK **之前**；AuthN 任一失败 **`Abort`**，不进 Casbin / Handler |
| **fail-close** | 鉴权链 Redis 故障 → **503/10008**；禁止 fail-open |
| **对外模糊** | 失败 `data=null`；AK 验签失败统一 **20009**，不区分「AK 不存在」与「SK 错误」 |
| **密码仅登录** | 业务 API 忽略 body 中的 password；无 Bearer 则 **401**，不尝试用密码鉴权 |

Phase 1 无 AK/SK；若请求带 `X-AK-*` → **401/20009**。M2M 上线后见 02-auth §M2M。

### 5.5 JWT 中间件校验流程（Phase 1）

```
请求进入（受保护路由）
  │
  ├─ 0. 【AuthN 前置】Bearer 与 X-AK-* 同时存在？→ 400 + 20008，Abort（见上节）
  ├─ 1. 提取 Authorization Header 中的 AT
  ├─ 2. 解析 JWT，校验 HS256 签名和 exp → uid, username, jti, mcp
  ├─ 3. 查 Redis 黑名单: EXISTS blacklist:at:{jti}
  │     ├─ Redis 错误 → 503（fail-close）
  │     └─ 在黑名单中 → 401
  ├─ 4. 查 user:disabled:{uid}
  │     ├─ Redis 错误 → 503
  │     └─ 存在 → 403 + ErrUserDisabled（30003）
  ├─ 5. mcp=true 且非改密路由 → 403 + 20007（ErrPasswordChangeRequired）
  ├─ 6. 注入 context: userID, username, mustChangePassword
  └─ 7. c.Next()

※ Casbin 中间件（下一层）通过 RoleFetcher 查 user_roles，不读 JWT 权限字段。
```

### 5.5.1 JWT 中间件 + 权限缓存（Phase 3 可选增强）

Phase 3 多实例/热点场景可引入 `perm:user:{userId}` 缓存，Casbin 中间件优先读缓存。详见 §5.7 与 [design-decisions.md §1](./design-decisions.md#1-jwt-无状态策略与权限缓存)。

### 5.6 对外接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/v1/auth/login` | POST | 登录，返回双 Token |
| `/api/v1/auth/refresh` | POST | 刷新 Token，RT 轮换 |
| `/api/v1/auth/logout` | POST | 登出，吊销 Token |
| `/api/v1/auth/password/update` | POST | 当前用户修改密码（须已登录） |

> Phase 2：`/api/v1/auth/devices`（设备列表）、`/api/v1/auth/devices/delete`（踢出设备）。Phase 1 允许多设备登录，不提供设备管理 API。

### 5.7 JWT 无状态策略与权限缓存（Phase 3 / 按需）

**长期方向**：JWT 仅作身份凭证，权限走 Redis 缓存 + 主动失效（多实例 Pub/Sub）。详见 [design-decisions.md §1](./design-decisions.md#1-jwt-无状态策略与权限缓存)。

**Phase 1 实际做法**：JWT 存 `uid/username/jti/mcp`；Casbin 中间件每次通过 `RoleFetcher` 查 `user_roles`（直接角色）。**不使用** `perm:user:{userId}` 缓存。

**Phase 3 引入缓存时的结构**：

```
Key:   perm:user:{userId}
Val:   { "roles": [...], "org_id": "...", "permissions": [...] }
TTL:   30min
```

权限变更：DB 事务提交 → `DEL perm:user:{userId}` → Pub/Sub 广播。

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

| 类型 | org_type | source | 说明 | 示例 |
|------|----------|--------|------|------|
| 种子根 / 实体组织 | 1–3 | system / hr / local | 公司真实组织架构；HR 同步见 [hr-directory-sync.md](../proposal/hr-directory-sync.md) | 集团、技术中心、产品中心 |
| 虚拟组 | 4 | local | 挂在实体组织下的项目组/虚拟团队 | `vg_alpha`、某项目组 |

> Phase 1 仅手工 CRUD 实体组织（`org_type` 1–3）；`source` / HR Sync / 虚拟组在 Phase 2b 落地。

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
- **写入方式（Phase 1）**：**同步**写入 PostgreSQL，保证审计不丢（见 [phase1/08-audit.md](../phase1/08-audit.md)）
- **写入方式（Phase 3a）**：可选 channel + Redis List 异步，降低请求延迟
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
| JWT Token | payload 仅含 `user_id`，`tenant_id` 放 Redis 权限缓存（`perm:user:{userId}`） |
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

> **SSOT：** 完整 DDL、索引与 seed 以 [`phase1/`](../phase1/README.md) 各模块文档及 `migrations/` 为准。本节仅作**表索引与约定摘要**，不在此维护可执行 SQL（原 UUID 大段 DDL 已移除）。

### 10.1 约定摘要

| 项 | Phase 1 |
|----|---------|
| 主键 | `BIGSERIAL` / `BIGINT`，JSON 序列化 `,string` |
| 业务键 | 角色 / 菜单 / 组织另有 `code`（Casbin subject 用 `role::{code}`） |
| 组织路径 | PostgreSQL `ltree`；`organizations.code` 须匹配 ltree 标签 `[A-Za-z0-9_]` |
| 乐观锁 | `version INT DEFAULT 1`（建表即含，见 [10-concurrency.md](../phase1/10-concurrency.md)） |
| 软删除 | `users`、`organizations` 等含 `deleted_at` |
| Casbin | 单表 `casbin_rule`；Phase 1 **直接角色**（`user_roles` → `p` 策略），无 `g` 表 BFS |

### 10.2 Phase 1 业务表

| 表 | 说明 | 权威文档 |
|----|------|----------|
| `users` | 用户账号、状态、`must_change_password` | [04-user.md](../phase1/04-user.md) |
| `user_roles` | 用户 ↔ 角色（Phase 1 权限主路径） | [04-user.md](../phase1/04-user.md)、[05-role.md](../phase1/05-role.md) |
| `user_orgs` | 用户 ↔ 组织（组织维度，Phase 1 不做数据过滤） | [04-user.md](../phase1/04-user.md)、[06-organization.md](../phase1/06-organization.md) |
| `organizations` | 实体组织 ltree 树 | [06-organization.md](../phase1/06-organization.md) |
| `roles` | 角色定义 | [05-role.md](../phase1/05-role.md) |
| `role_menus` | 角色 ↔ 菜单 | [05-role.md](../phase1/05-role.md) |
| `menus` | 菜单树（目录 / 菜单 / 按钮） | [07-menu.md](../phase1/07-menu.md) |
| `menu_apis` | 菜单 ↔ HTTP 路由（Casbin API 策略同步来源） | [07-menu.md](../phase1/07-menu.md) |
| `audit_logs` | 操作审计 | [08-audit.md](../phase1/08-audit.md) |
| `casbin_rule` | Casbin 策略存储 | [03-authz.md](../phase1/03-authz.md)、[01-infra.md](../phase1/01-infra.md) §迁移 |

迁移文件布局见 [01-infra.md](../phase1/01-infra.md)：`000001_init` 建表（含 `casbin_rule`）、`000002_seed` 种子（含 Casbin 初始策略）。

### 10.3 Phase 2+ 预留（本文档不展开 DDL）

| 表 / 能力 | 阶段 | 说明 |
|-----------|------|------|
| `resource_owners` | 2b（可选） | 泛化资源归属；**工单 2a 直接用 `tickets.org_id/org_path`** |
| `org_permissions` | 2b | 组织级权限模板 |
| 虚拟组 / HR 同步 / scope | 2b | 见 [organization 模块](../modules/organization.md)、[hr-directory-sync.md](../proposal/hr-directory-sync.md) |
| 组内 owner / `org_member_role` | **2c** | 见 [phase2/04-org-delegation.md](../phase2/04-org-delegation.md) |
| `api_credentials` | 3b / 按需 | AK/SK（Phase 1–2 不做；有 M2M 调用方时实现） |
| `casbin_rule_{resource}` | 2a+ | 按需独立策略表，见 [resource-model.md](../proposal/resource-model.md) |
| ResourceRegistry 实现 | 2a | Phase 1 为空接口 |

### 10.4 种子数据

> 详见 [01-infra.md](../phase1/01-infra.md) 与 [data-init.md](../proposal/data-init.md)。原则：`ON CONFLICT DO NOTHING`，不覆盖 `created_at` / `created_by`。

| 数据 | 内容 |
|------|------|
| 系统角色 | `superadmin`、`admin`、`operator`、`viewer`（`is_system=true`） |
| 组织 | `root`、`root.tech`、`root.product` |
| 初始用户 | 工号 `E000001` / 密码 `admin123`（`username=admin`），绑定 **superadmin** 与 root 组织 |
| 系统菜单 | 首页 + 系统管理（用户 / 角色 / 菜单 / 组织）及 `menu_apis` |
| Casbin | `p, role::superadmin, *, *` 与 `p, role::admin, *, *` |

模块级字段语义与 API 行为见 [`modules/`](../modules/) 目录。

---

## 11. 部署架构

> **原则（SSOT）**：[design-decisions.md §18 部署与代码解耦](./design-decisions.md#18-部署与代码解耦一套代码多种部署)——**一套代码、多种部署**，拓扑差异由配置与编排解决，业务 Handler/Service 不因 PG 集群 / Redis Sentinel / App 多副本而改写。

### 11.1 开发环境

```yaml
# deployments/docker-compose.yaml
version: '3.8'
services:
  postgres:
    image: postgres:15          # 对齐云服务版本
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
    image: redis:6.2-alpine     # 对齐云服务版本
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data

volumes:
  pg_data:
  redis_data:
```

### 11.2 生产环境部署

**反向代理：Nginx**

选型理由：暂时没有域名（Caddy/Traefik 的自动 HTTPS 优势用不上），Nginx 配置成熟、资源占用低、团队熟悉度高。有域名后可评估切换 Caddy。

```
客户端
  │ HTTP（Phase 1 无 TLS，Phase 2 加域名后配 HTTPS）
  ▼
Nginx（反向代理）
  │  - 静态文件服务（前端 SPA）
  │  - API 请求转发到 Go 进程
  │  - 超时控制、连接数限制
  │  - Phase 2: TLS 终止 + HTTP/2
  ▼
Go 进程（Gin :33333）
  ├── PostgreSQL 15（云托管 Cluster）
  └── Redis 6.2
```

**Nginx 核心配置要点**：

```
server {
    listen 80;
    # server_name zhuzhao.example.com;  # Phase 2 有域名后启用

    # 前端静态文件
    location / {
        root /var/www/zhuzhao;
        try_files $uri $uri/ /index.html;
    }

    # API 转发
    location /api/ {
        proxy_pass http://127.0.0.1:33333;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Request-Id $request_id;
        proxy_read_timeout 30s;
        proxy_connect_timeout 5s;
    }

    # 健康检查（不转发，Nginx 直接管）
    location /health/ {
        proxy_pass http://127.0.0.1:33333;
        access_log off;
    }

    # Swagger 文档
    location /swagger/ {
        proxy_pass http://127.0.0.1:33333;
    }
}
```

**各阶段 Nginx 演进**：

| 阶段 | Nginx 职责 | TLS | 说明 |
|------|-----------|-----|------|
| Phase 1 | 静态文件 + API 转发 | 无（HTTP） | 单实例，无域名 |
| Phase 2 | + TLS 终止 + HTTP/2 | 有域名后启用 | 可配合 certbot 自动续期 |
| Phase 3 | + 多服务路由 + 负载均衡 | 有 | 按路径分发到不同微服务 |

### 11.3 配置文件

```yaml
# configs/config.yaml
server:
  port: 33333
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
  secret: "${JWT_SECRET}"            # Phase 1: HS256 对称密钥（环境变量）
  # private_key: "${JWT_PRIVATE_KEY}"  # Phase 3 拆服务: RS256 RSA 私钥（环境变量/文件）
  # jwks_url: "/.well-known/jwks.json"  # Phase 3 拆服务: JWKS 公钥分发
  access_ttl: 30m
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
| **RT 并发刷新** | 多标签页同时用同一 RT 刷新，轮换后只有第一个成功 | 前端加请求防抖；后端 `GetDel` 轮换，第二个请求因 key 已删返回 **401 + 20004**（见 [phase1/02-auth §Token 刷新](../phase1/02-auth.md#token-刷新)） |
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
| RT 刷新 | ❌ | 不需要 | Redis `GETDEL` 原子读删天然防并发（见 phase1/02-auth） |

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
| 角色-菜单分配（路由策略，Phase 1） | `role_menus` + `casbin_rule` | 同一 PostgreSQL 事务，全成功或全回滚 |
| 用户-角色分配（Phase 1） | `user_roles` | 同一 PostgreSQL 事务（不写 Casbin g 表；Phase 1 无 g 表） |
| 组织结构变更（Phase 1） | `organizations` + `user_orgs` | 同一 PostgreSQL 事务，含 path 递归更新 |
| 组织级权限模板（Phase 2b） | `org_permissions` + `casbin_rule` | 同一 PostgreSQL 事务 |
| 资源归属（Phase 2b） | `resource_owners` + 业务表 | 同一 PostgreSQL 事务 |

#### DB 事务 + 缓存失效（最终一致性）

| 操作 | 流程 | 失败处理 |
|------|------|----------|
| 权限变更（Phase 3 权限缓存） | ① DB 事务提交 → ② Casbin 内存更新 → ③ 清除 Redis `perm:user:*` | ② 失败：DB 已提交，通过 Watcher 通知重载；③ 失败：缓存有 TTL，自然过期 |
| 角色-菜单变更（Phase 1） | ① DB 事务提交（`role_menus` + `casbin_rule`） → ② `enforcer.ReloadPolicy()` | Reload 失败：Watcher 或重启后重载 |
| 组织变更（Phase 3 组织/菜单缓存） | ① DB 事务提交（含 path 更新） → ② 清除 `orgs:user:*` / `menu:user:*` | ② 失败：缓存有 TTL，自然过期；可接受短暂不一致 |

**原则**：先写 DB（事务保证），再更新内存/缓存。缓存更新失败不影响数据正确性，只影响性能（TTL 过期后自动重建）。

### 12.4 审计日志可靠性

**Phase 1**：请求内**同步**写 DB（见 §8.2、[phase1/08-audit.md](../phase1/08-audit.md)）。以下为 Phase 3a 起的演进方案：

| 级别 | 方案 | 可靠性 | 复杂度 |
|------|------|--------|--------|
| L0（Phase 1） | 请求内同步 → DB | 最高（同事务路径），略增延迟 | 低 |
| L1 | channel + goroutine → DB | 进程崩溃丢 channel 内日志 | 低 |
| L2（Phase 3a 推荐） | channel → Redis List → goroutine → DB | 进程崩溃不丢，Redis 持久化 | 中 |
| L3（重型） | Kafka/RabbitMQ → 消费者 → DB | 最高可靠性，支持重放 | 高 |

### 12.5 跨实例事件广播

多实例部署时，需要通过 Redis Pub/Sub 广播以下事件：

| 事件 | Channel | 订阅者行为 |
|------|---------|-----------|
| Casbin 策略变更 | `casbin:policy:changed` | 触发 enforcer reload（加分布式锁） |
| 用户被禁用 | `user:disabled:{userId}` + `DEL refresh:{userId}:*` | AT：JWT 中间件 → 403（30003）；RT：`/auth/refresh` → 401（20004） |
| 权限缓存失效 | `cache:invalidate:{key}` | 删除本地/Redis 缓存（如 `perm:user:{userId}`，详见 §5.7） |

> 单实例阶段不需要 Pub/Sub，多实例部署时启用。

### 12.6 缓存策略（Phase 3 / 按需，Phase 1 不做）

Phase 1 路由鉴权每次查 `user_roles` + Casbin，**不使用**下列 Redis 缓存。

| 缓存对象 | Redis Key | TTL | 失效触发 |
|----------|-----------|-----|----------|
| 用户权限码列表 | `perm:user:{userId}` | 30min | 角色权限变更、用户角色变更（管理员操作后主动 `DEL`，详见 §5.7） |
| 用户菜单树 | `menu:user:{userId}` | 30min | 菜单变更、角色菜单关联变更 |
| 用户组织列表 | `orgs:user:{userId}` | 30min | 用户组织关系变更 |
| 组织树全量 | `org:tree` | 60min | 组织结构变更 |
| 资源级鉴权结果 | `authz:{userId}:{resType}:{resId}:{action}` | 5min | 权限变更时按 user 粒度清除 |

**缓存模式**：Cache-Aside（先查缓存，miss 查 DB 再回填）

**防击穿**：`singleflight`（同进程同 key 只放一个请求回源）

---

## 13. 安全加固

> 本章补充安全相关的遗漏设计。

### 13.1 密码安全

> Phase 1：bcrypt 存储 + 管理员重置 + 首次改密（`mcp`）。下列邮箱重置、密码历史等为 Phase 2+ 可选能力。

| 措施 | 说明 |
|------|------|
| 密码存储 | bcrypt，cost ≥ 12 |
| 密码复杂度 | 最少 8 位，含大小写+数字+特殊字符（可配置，Phase 2 完整策略） |
| 密码历史 | Phase 2+：记录最近 5 次 hash，防重用 |
| 密码重置 | Phase 1：管理员 `POST /users/password/reset`；Phase 2+：邮箱/手机一次性 token |
| 密码过期 | Phase 2+ 可选，如 90 天强制修改 |

### 13.2 登录安全（Phase 1）

| 措施 | 说明 |
|------|------|
| 登录限流 | 同一 **employee_no** 15 分钟内失败 **5** 次 → 429（Redis **Lua** LoginLocker，key `lock:login:{employee_no}`） |
| 防用户枚举 | 用户不存在 / 密码错误 / 登录时账号禁用 → 同一 401 文案 |
| 验证码 | Phase 2+：失败 3 次后图形验证码（可选） |
| 异地登录检测 | Phase 2+：IP 归属异常时审计或二次验证 |

### 13.3 API 安全

| 措施 | 说明 |
|------|------|
| CORS | Phase 1 `AllowAllOrigins`（全放开）；生产改为域名白名单 |
| SQL 注入 | 全部使用参数化查询（pgx 原生支持 `$1, $2` 占位符），**禁止字符串拼接 SQL** |
| 请求体大小限制 | 中间件限制 `max_body_size`（如 1MB） |
| 安全响应头 | `X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY` 等 |
| HTTPS | 生产环境强制 TLS（由反向代理/负载均衡层处理） |

**SQL 注入防护要点**：

1. **pgx 参数化查询**——所有 SQL 必须使用 `$1, $2` 占位符，pgx 会自动做类型转义
2. **禁止 `fmt.Sprintf` 拼接 SQL**——包括 WHERE 条件、ORDER BY、LIMIT 等
3. **列表过滤的 WHERE 子句**——资源级列表过滤生成 WHERE 时，字段名用白名单校验，值用参数化传入
4. **ltree 路径查询**——组织树路径查询也必须参数化，`org_path @> $1`
5. **动态 SQL 场景**——如必须动态拼接表名/列名，必须用白名单校验，不允许直接拼用户输入

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

> 启动与关闭的**编排**在 `internal/app`；**资源释放**由 Wire `cleanup` +（Phase 3+）`Runner.Stop` 完成。组件分层见 [§3.6](#36-组件注册与生命周期三者分离)。

#### 启动顺序（Phase 3+ 有 Runner 时）

```
1. Wire InitializeApp → 基础设施 Ping（PG / Redis fail-fast）
2. Runners.Start（逆依赖：Watcher → Worker → …）
3. HTTP ListenAndServe（goroutine）
```

Phase 1 仅步骤 1 + 3；无后台 Runner。

#### 关闭顺序

```
收到 SIGTERM/SIGINT
  │
  ├─ 1. 停止接受新请求（http.Server.Shutdown，超时如 30s）
  ├─ 2. 等待 in-flight 请求完成
  ├─ 3. Runners.Stop（与 Start 逆序；Phase 3+）
  ├─ 4. 刷空审计日志队列（channel / Redis List，带超时）
  ├─ 5. Wire cleanup：Casbin → Redis → PostgreSQL
  └─ 6. 退出进程
```

```go
// internal/app/lifecycle.go — Phase 3 引入；Phase 1 可不定义
type Runner interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

**原则**：被动 Domain Service（UserService、TicketService）**不**实现 `Runner`；只有显式起 goroutine 的组件才需要。

### 14.2 健康检查

| 探针 | 路径 | 检查内容 |
|------|------|----------|
| Liveness | `/health/live` | 进程存活（直接返回 200） |
| Readiness | `/health/ready` | DB 连通性 + Redis 连通性（ping 两者，全通过返回 200） |

### 14.3 可观测性（配置可选、部署可选）

> Phase 1–2：仅健康检查 + slog（见 [01-infra §健康检查](../phase1/01-infra.md#健康检查)）。  
> Phase 3a：应用内 **具备接入能力**，通过 `config.yaml` 开关；**不强制**部署 Prometheus / Grafana / OTel Collector。  
> 详见 [phase3/01-observability.md](../phase3/01-observability.md)。

| 维度 | 工具 | 应用内 | 外部栈 |
|------|------|--------|--------|
| Metrics | `prometheus/client_golang` + Gin 中间件 | `observability.metrics.enabled` → `/metrics` | Prometheus **可选** scrape |
| 分布式追踪 | OpenTelemetry | `observability.tracing.enabled`；exporter：`noop` / `stdout` / `otlp` | Collector → Jaeger/Tempo **可选** |
| 性能分析 | `net/http/pprof` | `observability.pprof.enabled` 或 `server.mode=debug` | 内网/admin 端口，不挂公网 33333 |
| 错误追踪 | Sentry（可选） | 配置 DSN 时启用 | — |
| 可视化 | Grafana | — | **永远可选**，非 App 依赖 |

**原则**：

1. `enabled: false` 时不注册路由、使用 noop tracer，**零额外运行时开销**。
2. App 启动 **不依赖** Prometheus/Grafana/Collector 进程存在。
3. Docker Compose 用 **profile** 拉起观测栈；小环境默认不带 profile。
4. 多实例或对外 SLA 选 [3a-full](../phase3/README.md#3a-full多实例或需-slo-对外-sla)；单实例内网可选 [3a-min](../phase3/README.md#3a-min单实例内网低-sla)。

### 14.4 数据库迁移

使用 `golang-migrate` 管理 Schema 版本：

```
migrations/
├── 000001_init.up.sql      # 初始建表
├── 000001_init.down.sql
├── 000002_seed.up.sql      # 种子数据（含 Casbin 初始策略）
├── 000002_seed.down.sql
└── ...
```

**种子数据幂等性原则**：

所有种子数据 migration 必须使用 `ON CONFLICT DO NOTHING`，确保重复执行不覆盖已有数据（特别是 `created_at`、`created_by` 等审计字段）。详见 [design-decisions.md#7 系统重启与数据初始化幂等性](./design-decisions.md#7-系统重启与数据初始化幂等性)。

```sql
-- 正确：幂等，不覆盖（主键 BIGSERIAL 自增，冲突键用 code）
INSERT INTO roles (code, name, is_system) VALUES
  ('admin', '管理员', true)
ON CONFLICT (code) DO NOTHING;

-- 错误：非幂等，覆盖审计字段
INSERT INTO roles (code, name, is_system) VALUES
  ('admin', '管理员', true)
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name;
```

**运行时 Sync 安全规则**：

应用启动时的数据同步（如 Casbin 策略同步）遵循：
1. `created_at`/`created_by` 永远不覆盖
2. 系统资源（`is_system = true`）不被删除
3. Sync 失败不阻塞启动（仅 Warn 日志）

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
| JWT 签发与解析 | `golang-jwt/jwt/v5` | — | 社区标准 JWT 库。Phase 1: HS256；Phase 3 拆服务: RS256 + JWKS |
| 密码哈希 | `golang.org/x/crypto/bcrypt` | `x/crypto/argon2`（更安全但更重） | bcrypt 足够，cost ≥ 12 |
| UUID 生成 | `google/uuid` | `gofrs/uuid` | uuid.New() 生成 UUIDv4 |
| CORS 中间件 | `gin-contrib/cors` | — | Gin 官方 contrib，配置简单 |

### 15.4 Casbin 生态

| 模块/用途 | 推荐 | 说明 |
|-----------|------|------|
| Casbin 核心 | `casbin/casbin/v3` | 使用 `SyncedEnforcer` 支持并发安全 |
| PostgreSQL Adapter | `noho-digital/casbin-pgx-adapter` | Casbin v3 + pgx v5，支持 Filtered/Batch/Updatable adapter。详见 `design-decisions.md` §10 |
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
| 反向代理 | Nginx | Caddy（有域名后可评估） | 无域名阶段最简方案，团队熟悉度高 |

### 15.7 API 文档与测试

| 模块/用途 | 推荐 | 备选 | 说明 |
|-----------|------|------|------|
| Swagger 生成 | `swaggo/swag` + `swaggo/gin-swagger` + `swaggo/files` | — | 注解生成 OpenAPI 文档，Gin 集成 |
| HTTP 测试 | `gin-gonic/gin` 内置 `httptest` | — | Go 标准库 `net/http/httptest` |
| 断言库 | `stretchr/testify` | `smartystreets/goconvey` | testify 是 Go 测试断言事实标准 |
| Mock | `uber-go/mock`（原 `gomock`） | `matryer/moq` | uber-go/mock 是 gomock 的活跃 fork |
| 测试容器 | `testcontainers/testcontainers-go` | — | 集成测试启动真实 PG + Redis 容器 |

**测试策略（测试先行）**：

核心原则：先写测试，再写实现。详见 [phase1/README.md §1.3 验收](../phase1/README.md#13-验收标准)。

| 层级 | 范围 | Mock 策略 |
|------|------|----------|
| Service 单元测试 | 业务逻辑 | Mock Repository 接口 |
| Repository 集成测试 | SQL 正确性 | 不 Mock，testcontainers 真实 PG |
| Middleware 单元测试 | 认证/鉴权逻辑 | Mock JWT Manager + Redis |
| Handler 集成测试 | 端到端 API | Mock Service + 真实 Gin 路由 |

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

> **SSOT**：[`api/response.md`](../api/response.md)（Envelope 字段、成功/失败/分页、实现约束、例外）。  
> **错误码**：[`api/errcode.md`](../api/errcode.md)。

所有 JSON API 返回：

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
| code | int | 业务码，**0=成功**，非 0 见 errcode.md |
| message | string | 描述信息（**不用 `msg`**） |
| data | any | 业务数据；**失败时为 null** |
| request_id | string | 请求 ID（中间件注入，串联日志） |

分页 `data` 形状见 response.md §3.2。

### 16.2 错误码设计

> **完整错误码清单以 [`api/errcode.md`](../api/errcode.md) 为准。** 以下仅做概览。

错误码按模块分段（0=成功，10000 通用，20000 认证，30000 用户，40000 角色，50000 组织，60000 菜单，70000 鉴权，80000 审计）。

HTTP 状态码映射：

| HTTP Status | 使用场景 |
|-------------|----------|
| 200 | 成功 |
| 400 | 参数校验失败 |
| 401 | 未认证（token 无效/过期/已吊销） |
| 403 | 已认证但无权限、账号禁用、须改密、无角色 |
| 404 | 资源不存在 |
| 409 | 冲突（如工号已存在） |
| 429 | 登录限流 |
| 500 | 服务器内部错误 |
| 503 | 鉴权链路 Redis 不可用（fail-close） |

### 16.3 错误处理链路

```
service 层 → 返回 errcode.Error（含业务码 + 消息）
    ↓
handler 层 → 识别 errcode，转换为统一响应格式 + HTTP 状态码
    ↓
middleware 层 → recovery 中间件兜底未处理的 panic
```

### 16.4 API 设计规范

**HTTP 方法**：

本项目仅使用 GET 和 POST 两种方法，不使用 PUT、DELETE、PATCH 等方法。理由：GET 和 POST 覆盖所有场景，简化前端对接和网关配置，避免非常规方法在部分代理/防火墙环境下被拦截的问题。

| 方法 | 用途 | 示例 |
|------|------|------|
| GET | 查询（列表、详情） | `GET /api/v1/users`、`GET /api/v1/users/:id` |
| POST | 创建、更新、删除、操作类接口 | `POST /api/v1/users`（创建）、`POST /api/v1/users/update`（更新）、`POST /api/v1/users/delete`（删除）、`POST /api/v1/auth/login`（登录） |

**URL 设计原则**：

1. **URL 简洁，不承载业务语义**——参数放 request body（POST）或 query string（GET），不要在 URL 路径中嵌套过多信息
2. **资源命名用名词复数**——`/api/v1/users`，不用 `/api/v1/getUser`
3. **操作类接口用动词子路径**——`POST /api/v1/auth/login`、`POST /api/v1/auth/logout`、`POST /api/v1/users/status`，资源 ID 放 body 不放 URL
4. **过滤/排序/分页用 query string**——`GET /api/v1/users?page=1&page_size=20&username=zhang&employee_no=E20240086&role=admin&sort=created_at:desc`（`username` 模糊可多结果；`employee_no` 精确唯一）
5. **避免深层嵌套**——URL 层级不超过 3 层，如 `/api/v1/orgs/:org_id/members`

**正确示例**：

```
GET  /api/v1/users/:id                    ✅ 查详情，GET 无 body，id 放路径
GET  /api/v1/users?page=1&role=admin      ✅ 过滤条件用 query string
POST /api/v1/users                        ✅ 创建数据放 body
POST /api/v1/users/roles                  ✅ 给用户分配角色，user_id + role_ids 放 body
POST /api/v1/users/orgs                   ✅ 给用户分配组织，user_id + org_ids 放 body（全量覆盖）
POST /api/v1/users/delete                 ✅ 删除操作用 POST + 动词子路径，id 放 body
```

**错误示例**：

```
GET  /api/v1/users/role/admin/page/1      ❌ 过滤条件塞进路径
GET  /api/v1/getUserById?id=123           ❌ URL 含动词
POST /api/v1/users/create                 ❌ 路径含 create 动词（POST 本身即创建）
PUT  /api/v1/users/:id                    ❌ 不使用 PUT，用 POST /api/v1/users/update
DELETE /api/v1/users/:id                  ❌ 不使用 DELETE，用 POST /api/v1/users/delete
GET  /api/v1/orgs/:org_id/depts/:dept_id/teams/:team_id/members  ❌ 层级过深
```

---

## 17. API 路由总表

> 完整的 API 端点清单，按模块分组。

### 17.1 认证模块（Phase 1）

| 路由 | 方法 | 鉴权 | 说明 |
|------|------|------|------|
| `/api/v1/auth/login` | POST | ❌ | 登录，返回双 Token |
| `/api/v1/auth/refresh` | POST | ❌ | 刷新 Token，RT 轮换 |
| `/api/v1/auth/logout` | POST | ✅ | 登出 |
| `/api/v1/auth/password/update` | POST | ✅ | 当前用户修改密码 |

> Phase 2：`/api/v1/auth/devices`、`/api/v1/auth/devices/delete`。管理员重置他人密码见 §17.2 `POST /api/v1/users/password/reset`（非公开邮箱重置）。

### 17.2 用户模块

| 路由 | 方法 | 鉴权 | 说明 |
|------|------|------|------|
| `/api/v1/users` | GET | ✅ | 用户列表（分页+筛选） |
| `/api/v1/users` | POST | ✅ | 创建用户 |
| `/api/v1/users/:id` | GET | ✅ | 用户详情 |
| `/api/v1/users/update` | POST | ✅ | 更新用户（id 放 body） |
| `/api/v1/users/delete` | POST | ✅ | 删除用户（软删除，id 放 body） |
| `/api/v1/users/status` | POST | ✅ | 启用/禁用用户（id 放 body） |
| `/api/v1/users/:id/orgs` | GET | ✅ | 用户所属组织列表 |
| `/api/v1/users/roles` | POST | ✅ | 分配用户角色（id 放 body） |
| `/api/v1/users/orgs` | POST | ✅ | 分配用户组织（全量覆盖，id 放 body） |
| `/api/v1/users/password/reset` | POST | ✅ | 管理员重置密码（id 放 body） |
| `/api/v1/user/menus` | GET | ✅ | 当前用户菜单树 |
| `/api/v1/user/permissions` | GET | ✅ | 当前用户权限码 |
| `/api/v1/user/profile` | GET | ✅ | 当前用户信息 |
| `/api/v1/user/profile/update` | POST | ✅ | 更新个人信息 |

### 17.3 角色模块

| 路由 | 方法 | 鉴权 | 说明 |
|------|------|------|------|
| `/api/v1/roles` | GET | ✅ | 角色列表 |
| `/api/v1/roles` | POST | ✅ | 创建角色 |
| `/api/v1/roles/:id` | GET | ✅ | 角色详情 |
| `/api/v1/roles/update` | POST | ✅ | 更新角色（id 放 body） |
| `/api/v1/roles/delete` | POST | ✅ | 删除角色（id 放 body） |
| `/api/v1/roles/:id/menus` | GET | ✅ | 角色关联菜单 |
| `/api/v1/roles/menus` | POST | ✅ | 分配角色菜单（id 放 body） |
| `/api/v1/roles/:id/permissions` | GET | ✅ | 角色权限策略 |

### 17.4 组织模块

| 路由 | 方法 | 鉴权 | 说明 |
|------|------|------|------|
| `/api/v1/orgs` | GET | ✅ | 组织树 |
| `/api/v1/orgs` | POST | ✅ | 创建组织 |
| `/api/v1/orgs/:id` | GET | ✅ | 组织详情 |
| `/api/v1/orgs/update` | POST | ✅ | 更新组织（id 放 body） |
| `/api/v1/orgs/delete` | POST | ✅ | 删除组织（id 放 body） |
| `/api/v1/orgs/move` | POST | ✅ | 移动组织（id 放 body） |
| `/api/v1/orgs/:id/members` | GET | ✅ | 组织成员列表 |
| `/api/v1/orgs/members` | POST | ✅ | 添加成员到组织（org_id、user_id 放 body） |
| `/api/v1/orgs/members/delete` | POST | ✅ | 从组织移除成员（org_id、user_id 放 body） |

> Phase 1 **用户-组织绑定**：双 HTTP 入口（`POST /users/orgs` + `POST /orgs/members*`），**单写逻辑**在 `OrgService`（`SetUserOrgs` / `AddMember` / `RemoveMember`）。创建用户 body 可含 `org_ids`。

**路由骨架对齐**（编码 Step 1/9 检查 `internal/router/router.go`）：

| 路由 | 文档 | 骨架 |
|------|------|------|
| `POST /users/orgs` | ✅ §17.2 | ✅ 已注册 |
| `GET /orgs/:id/members` | ✅ §17.4 | ✅ 已注册 |
| `POST /orgs/members` | ✅ §17.4 | ✅ 已注册 |
| `POST /orgs/members/delete` | ✅ §17.4 | ✅ 已注册 |

> 上表路由已在 `internal/router/router.go` 注册；Step 7–9 写 API 见 [phase1/README §2.4](../phase1/README.md#24-step-79-crud-补全计划)。

### 17.5 菜单模块

| 路由 | 方法 | 鉴权 | 说明 |
|------|------|------|------|
| `/api/v1/menus` | GET | ✅ | 菜单树（全量） |
| `/api/v1/menus` | POST | ✅ | 创建菜单 |
| `/api/v1/menus/:id` | GET | ✅ | 菜单详情 |
| `/api/v1/menus/update` | POST | ✅ | 更新菜单（id 放 body） |
| `/api/v1/menus/delete` | POST | ✅ | 删除菜单（id 放 body） |

### 17.6 审计模块

| 路由 | 方法 | 鉴权 | 说明 |
|------|------|------|------|
| `/api/v1/audit/logs` | GET | ✅ | 审计日志查询（分页+筛选） |

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

**目标**：认证鉴权框架可运行。含登录限流、会话吊销、fail-close、超管保护。**不做**部门数据隔离、不做 AK/SK。

| 能力 | 范围 | 说明 |
|------|------|------|
| 项目骨架 | 完整目录结构 + go.mod + Wire | 能 `go build` 通过 |
| 配置加载 | Viper 读取 config.yaml | DB/Redis/JWT 配置 |
| 数据库 | PG + Redis Docker Compose + 迁移脚本 | 一键启动开发环境 |
| 统一响应 | response 包 + errcode 包 | 统一 JSON 输出 |
| 健康检查 | `/health/live` + `/health/ready` | K8s/Docker 探针 |
| 种子数据 | 4 系统角色 + admin 用户（绑定 superadmin）+ 初始菜单 | 首次启动可用，幂等（`ON CONFLICT DO NOTHING`） |
| 用户登录 | **工号** + 密码 + 双 Token + **Lua LoginLocker** | AT 30min HS256 + RT 7d |
| Token 校验 | JWT 中间件 + 黑名单 + `user:disabled` | Redis 故障 503（fail-close） |
| Token 刷新 | RT `GETDEL` 轮换 | 无感刷新 |
| 登出 / 会话吊销 | AT：`user:disabled` + 黑名单；RT：`DEL refresh:*` + Refresh 兜底检查 | 禁用/删除后 AT→403/30003，RT refresh→401/20004 |
| 首次改密 | `must_change_password` + JWT `mcp` | 重置后强制改密 |
| 路由级鉴权 | Casbin RBAC 中间件 | 接口权限控制 |
| 资源注册表 | ResourceRegistry **空接口** | 不当独立大 Step |
| 用户管理 | CRUD + 启用禁用 + 超管保护 | 最后一个 superadmin 不可移除 |
| 角色 / 菜单 / 组织 | CRUD + 菜单分配 + ltree 组织树 | 组织关联无 `role_id` 主键 |
| 审计 | 同步写入 + 登录单独审计 | 登录是公开路由 |
| 优雅关闭 | signal 处理 + 资源释放 | 不丢请求 |

详见 [phase1/README.md](../phase1/README.md)。

### 18.2 Phase 2：业务可用（工单）

**目标**：资源级鉴权 + 工单。仍为模块化单体，不拆 IAM。子阶段 **2a → 2b → 2c**，详见 [phase2/README.md](../phase2/README.md) §0。

| 子阶段 | 能力 | 说明 |
|--------|------|------|
| **2a** | ResourceRegistry + 工单 MVP | **assigned** 范围；无附件 |
| **2b** | 组织增强 + 存储 + 体验 | 虚拟组/scope/HR、预签名附件、设备 UI、密码复杂度 |
| **2c** | 组织内委托 | owner、`org_member_role`、工单 Authorize（D1–D11） |

**明确后移**（Phase 3 或按需）：RS256、AK/SK、缓存平台、审计异步、每资源 Enforcer、IAM 拆分。

详见 [phase2/README.md](../phase2/README.md)、[roadmap.md](../roadmap.md)。

### 18.3 Phase 3：生产加固（可上线）

**目标**：可观测性、多实例、高可用；有需要时拆 IAM。

| 能力 | 范围 | 说明 |
|------|------|------|
| Metrics / 追踪 | 应用内可选开关；Prometheus + OTel **部署可选** | 3a-full 或多实例时建议开启；Grafana 永远可选 |
| 多实例 | Casbin Watcher、跨实例事件、分布式锁 | |
| 审计日志升级 | Redis List 队列（L2）/ 异步 | |
| 事件驱动 | Outbox + Asynq | |
| 微服务拆分 | IAM 独立、Gateway、gRPC、RS256+JWKS | |
| 高可用 | PG Cluster、Redis Sentinel、Nginx | |
| 平台增强 | 缓存体系、AK/SK（有调用方时） | |
| 安全增强 | 异地登录、验证码、密码过期、API 限流 | |

### 18.4 预留扩展（按需启用）

| 能力 | 预留点 | 启用条件 |
|------|--------|----------|
| 多租户 | tenant_id + Casbin 模型 | 有多客户需求时 |
| 第三方登录 | oauth_provider + oauth_id | 接 SSO/OAuth 时 |
| 消息队列 | 审计日志通道可替换 | 日志量大或需重放时 |
| 验证码 | 登录接口预留参数 | 安全要求提高时 |
| 密码过期 | users 表无额外字段 | 合规要求时 |

---

## 19. 决策状态总览

> 本表记录架构设计中所有关键决策的当前状态。`✅` 表示已决策，`⏳` 表示暂缓/后期阶段。

### 19.1 已决策项

| 事项 | 决策 | 参考章节 |
|------|------|----------|
| PostgreSQL 环境搭建 | Docker Compose 单机 | §5 |
| Casbin 策略存储 | PostgreSQL | §7 |
| 权限模型 | RBAC（路由级）+ 资源级（代码内联 + ltree SQL） | §7、`design-decisions.md` §5 |
| JWT 签名算法 | Phase 1: HS256 → Phase 3 拆服务时: RS256 + JWKS | `design-decisions.md` §9 |
| 日志库 | slog + Lumberjack | §15 |
| 双 Token 机制 | AT(30min HS256) + RT(7d) + `GETDEL` 轮换 | §8 |
| 依赖注入 | Google Wire，编译时生成 | §4 |
| 并发与事务 | 分布式锁 4 场景，跨存储操作失败策略 | §12 |
| 安全加固 | 密码安全、登录安全、API 安全、配置安全 | §13 |
| 运维可观测性 | 优雅关闭、健康检查、DB 迁移；Metrics 后期补充 | §14 |
| 审计日志可靠性 | Redis List 轻量队列，不引入重量级 MQ | §9 |
| 缓存策略 | Cache-Aside + singleflight 防击穿，按 user 粒度失效 | §8 |
| 统一响应与错误码 | 统一 JSON 结构 + 分段错误码 + HTTP 映射 | §16 |
| API 路由总表 | 7 个模块完整端点清单 | §17 |
| 分阶段实施计划 | Phase 1 跑起来 → Phase 2 业务可用 → Phase 3 生产加固 | §18 |
| Schema 完善 | 软删除、乐观锁、负责人、第三方登录预留、扩展字段 | §11 |
| 种子数据 | 4 系统角色 + admin 用户（绑定 superadmin）+ 组织关联（`ON CONFLICT DO NOTHING` 幂等） | `proposal/data-init.md` |
| API 版本管理 | `/api/v1` 前缀 | §17 |
| 资源级鉴权架构 | 分层 PEP/PDP：Gateway 路由级 + Service 资源级 | `design-decisions.md` §5 |
| 资源抽象机制 | Resource 接口 + 自注册 | `proposal/resource-model.md` |
| Casbin 策略爆炸 | 每资源独立 Enforcer | `design-decisions.md` §8 |
| 数据库高可用 | PG Cluster（2+VIP）云托管；Phase 1 单节点 | `design-decisions.md` §11 |
| ReBAC 引擎选型 | Phase 1 ltree+内联；Phase 2 按需评估 OpenFGA/SpiceDB | `design-decisions.md` §12 |
| 微服务通信协议 | gRPC 内部 + REST 外部（gRPC-Gateway） | `design-decisions.md` §13 |
| 测试策略 | 测试先行：先写测试再写实现 | [phase1/README.md](../phase1/README.md) §1.3 |

### 19.2 暂缓项（后期阶段）

| 事项 | 当前状态 | 计划阶段 |
|------|----------|----------|
| 消息队列 | 当前 Redis Pub/Sub + List 足够 | Phase 3 按需评估 |
| 多租户 | 表和模型预留 tenant_id | Phase 3 |
| Casbin Watcher | 多实例部署时必须 | Phase 3 |
| 验证码 / 异地登录检测 | 登录安全增强 | Phase 3 |
| Metrics + 分布式追踪 | 先实现健康检查 | Phase 3 |
| log sampling | 如需要可自定义 slog Handler | Phase 3 |
| 各模块实现细节 | ⏳ 待补充 | 边界已定，后续逐模块讨论实现 |
