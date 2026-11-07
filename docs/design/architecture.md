# zhuzhao模块化单体架构——Phase5现况与演进

> 文档版本：v0.7（2026-11-07 L-5主体校准）
> 现行实现证据基线：Phase5 `996c167f4567e39362226937b25368b55b7f44bd`。代码能力、运行部署与未来规划分别表述；当前仍在Phase5审计修复，未合main、未启动Phase6。
> 初稿创建于2026-08，保留其分层、选型与阶段演进动机；标明历史的目录/接口/备选库不是现行实现。能力/迁移/健康状态以[11号控制台](../review/11-project-control.md)为第一锚点，设计边界以[standards](../standards.md)及对应阶段稿为准。

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

系统已演进为Go模块化单体：IAM底座+隔离工单领域+横切基础设施；前端及任务/清单服务在兄弟仓。以下IAM能力是主仓的一个组成部分，不再将整仓等同认证模块：

| 功能 | 说明 |
|------|------|
| 用户登录 | 账号密码登录，双 Token 签发 |
| 认证 | Token 校验、无感刷新、登出、多设备管理 |
| 路由级鉴权 | 基于 RBAC，校验角色对 API 接口的访问权限 |
| 资源级鉴权 | 基于 ltree 组织关系查询 + 属主判断，校验对具体资源的操作权限 |
| 动态路由 | 接口返回权限树，驱动前端菜单和按钮渲染 |
| 组织架构 | 实体组织 + 虚拟组（项目组），权限沿组织树继承 |
| 审计日志 | 记录操作/安全行为；实际查询条件以AuditListQuery与handler为准 |
| 多租户 | 预留多租户扩展能力 |

### 1.2 设计原则

- **按业务领域纵向切分，按职责横向分层**
- Casbin 只管路由级 RBAC，资源级鉴权用 ltree SQL + 代码内联；当前没有OpenFGA/SpiceDB独立PDP
- 内部包（`internal/`）禁止外部 import
- 依赖注入，组件可替换
- 边界优先：先定边界和接口，再填充实现

### 1.3 项目与生态边界

| 位置 | 当前职责 | 明确边界 |
|---|---|---|
| zhuzhao | IAM、工单、storage元数据/鉴权、消息/inbox、内务cron、任务门面、AL网关、S9情报缓存 | 模块化单体，不是已拆IAM/工单微服务；对用户执行L1/L2/L3 |
| zhuzhao-ui | Vue控制台、动态路由、表单/状态图设计器，消费主仓契约 | 不直连内部系统；浏览器显隐不替代服务端鉴权；图坐标是本地视图偏好 |
| taskrunner | 任务定义/提交、调度与Asynq worker、执行记录/重试/死信/回调 | 不拥有工单状态机或用户IAM；任务可重试不等于主仓领域事件可靠重放 |
| activelist | 类型/schema管理、清单数据CRUD/校验/导入 | 主仓经`/al`反代并出站签名；不是当前已实现的风险R1台账 |
| zhuzhao-utils | jwt、aksk/compat、response/errcode、jsonutil、crypto、logger、PG/Redis通用件 | 无工单/组织业务策略；消费者版本以各仓go.mod为准，不把utils HEAD当已消费版本 |

主仓证据：[internal/app/providers.go 的 `provideApp / provideGateway`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/app/providers.go)；[internal/service/taskrunner_service.go 的 `TaskrunnerService.Submit`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/taskrunner_service.go)；[internal/gateway/gateway.go 的 `Registry.Mount`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/gateway/gateway.go)；[internal/pkg/threatintel/service.go 的 `Service.Query`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/pkg/threatintel/service.go)。
兄弟证据：[taskrunner `worker.Deps.handleCallback`（`1dcc831`）](https://github.com/tracerbiubiubiu/taskrunner/blob/1dcc83185f24b8dc2bed7bba6786272a5fdfb19f/internal/worker/worker.go)、[activelist `DataService`（`af0b350`）](https://github.com/tracerbiubiubiu/activelist/blob/af0b350da650517df2c9d15cca4892cdcd51be88/internal/service/data_service.go)、[utils `jwt.Manager`（`9296ae1`）](https://github.com/tracerbiubiubiu/zhuzhao-utils/blob/9296ae170b71b06c623c54b6010a107406903346/jwt/jwt.go)、[UI `FlowGraphDesigner`（`01c5dea`）](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/01c5deae23e677853316f1ee2dd692c708246651/src/components/FlowGraphDesigner/index.vue)。

未来资源接入走`Resource`接口与schema守卫，不要求凭空创建`resource_owners/org_permissions`两张旧提案表。工单实际直接使用`created_by/org_id/org_path`；HR真实同步、risk域与更远连接器/编排能力仍按阶段规划启动。

---

## 2. 技术选型

### 2.1 最终选型

| 技术 | 用途 | 推荐库/包 | 选型理由 |
|------|------|----------|----------|
| **Gin** | HTTP 框架 | `github.com/gin-gonic/gin` | 轻量、中间件生态成熟 |
| **Viper** | 配置加载 | `github.com/spf13/viper` | 当前Load读取文件/绑定env；不等于全配置自动热更新 |
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

**决策**：路由级用 Casbin RBAC，资源级用基于 ltree 的组织关系查询 + 代码内联属主判断。OpenFGA/SpiceDB仅为历史可替换方向，当前未接入。

**理由**：

1. ltree 一条 SQL 做组织树层级判断，不需要 ReBAC 引擎。
2. 属主判断（`created_by == userID`）是简单 ABAC，代码内联最直接。
3. Phase 1 资源类型少、关系链浅，引入独立 ReBAC 服务过重。
4. 保留资源接口边界，未来确有需求再评估替换，不把某阶段编号当实施承诺。

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

### 3.1 当前目录结构

```text
cmd/server/                     # 主HTTP进程入口
internal/app/                   # Wire、跨域装配与生命周期
internal/router/                # 路由链与目录守卫
internal/handler/               # 当前主要为扁平handler文件
internal/service/               # IAM/消息配置/cron/任务门面等
internal/service/ticket/        # 工单领域隔离：状态图、转换、分派、动作、inbox等
internal/service/storage/       # 对象存储业务状态机/GC
internal/repository/            # 当前主要为扁平SQL仓储文件
internal/model/                 # 模型及HTTP契约
internal/middleware/            # 认证、路由RBAC、日志、限流等
internal/casbin/                 # PG adapter / SyncedEnforcer
internal/gateway/                # AL反代与签名传输
internal/pkg/                   # 主仓私有基础件：resource/domainevent/exprcalc/...
migrations/                     # 成对SQL；编号权威地图见11号
```

通用JWT/AKSK/响应/Redis等已抽至独立Go模块zhuzhao-utils；主仓业务错误码、资源策略、存储适配等仍按实际职责保留。`api/v1/dto/`、统一`handler/{domain}/repository/{domain}`及`cmd/worker`不是当前完整目录实物。
证据：[internal/app/wire.go 的 `InitializeApp`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/app/wire.go)；[internal/service/ticket/service.go 的 `Service`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/service.go)；[internal/service/storage/service.go 的 `Service`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/storage/service.go)；[internal/app/providers.go 的 `provideJWTManager / provideRedis`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/app/providers.go)。

2026-08曾拟逐域迁移handler/service/repository，下面§3.5保留其拆分原则；它是新代码组织方向，不是所有旧文件已经迁完的状态。

### 3.2 各模块职责边界

| 模块 | 职责 | 禁止事项 |
|------|------|----------|
| `cmd/server` | 启动编排 | 不包含任何业务逻辑 |
| `internal/config` | 加载配置，提供类型安全结构体 | 不依赖业务模块 |
| `internal/model` | 领域数据与HTTP序列化契约 | 不拥有跨域业务流程；实际会引用jsonutil等契约类型 |
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

> **历史目录演进方案（2026-08）**：主仓当前仍一个模块化单体进程；taskrunner/activelist/utils/UI为生态独立仓，不代表主仓IAM/工单已拆服务。Phase3拆IAM的旧设想未实施；后续拆分仍按需求裁定（见[deployment-evolution.md](../proposal/deployment-evolution.md)）。
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
| **横向** | 通用response/errcode/logger/postgres/redis等 | 已抽为zhuzhao-utils；业务私有件仍留internal/pkg |
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
| `integration/hr`（历史目标目录，未落地） | proposal/hr-directory-sync | 后续IAM Job | HR真实同步未实现，Phase6启动前置按mock/adapter规划 |

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
                        # （internal/pkg/jwt/ 已抽至 zhuzhao-utils/jwt，2026-09 公共包抽取；
                        #   认证平面拆 IAM 时仍随 auth 域演进）

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

#### 3.5.7 Phase1历史落地节奏（实际目录以§3.1为准）

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
| **生命周期编排** | `internal/app/` | 显式启动/停止现有后台组件 | 当前为PolicyEvalWriter、CronScheduler、事件Drain；未实现旧统一Runner接口/主仓Asynq worker |

```
                    ┌─────────────────────────────────────┐
                    │           cmd/server / App          │
                    └─────────────────────────────────────┘
           Wire 注入 │                    │ 生命周期编排
                    ▼                    ▼
    Handler ──▶ XxxService ──▶ Repo          当前后台组件（见§14.1）
                    │
                    └──▶ registry.Register(XxxResource)   ← 仅鉴权，无 Start/Stop
```

**`resource.Resource` 只管鉴权**，不包含 `Init` / `Run` / `Start` / `Stop`。各域 **`XxxService` 按用例定义业务方法**（`Create`、`List`…），不强制实现统一生命周期接口。

**基础设施**（PG、Redis、Casbin）沿用 Wire `New() (T, cleanup, error)`；`cleanup` 在进程退出最后阶段调用，见 [§14.1](#141-优雅关闭)。

**新增业务域 checklist**（与 [resource-model.md](../proposal/resource-model.md) 一致）：

1. `service/{domain}/` + `repository/{domain}/` + `handler/{domain}/` → 加入 Wire  
2. `router` 注册路由 + seed `menu_apis` + Casbin `p` 策略  
3. Phase 2+：另建 `{domain}/resource.go` 实现 `Resource`，构造函数自注册  
4. 有后台任务时：按现有组件的启动/停止约定由 `App` 显式编排；当前没有通用 `Runner` 接口，勿据历史草图新增依赖；ResourceRegistry 继续只负责鉴权

**历史多二进制方案**（[design-decisions §18](./design-decisions.md#18-部署与代码解耦一套代码多种部署)）：曾拟主仓`cmd/worker`单独Wire Asynq consumer；当前没有该二进制，任务执行器实际在taskrunner仓，不能据此启动不存在的worker。

### 3.4 各模块对外接口边界

#### service层当前接口边界

| 组件 | 当前入口/职责 |
|---|---|
| AuthService | Login/Refresh/Logout/UpdatePassword/Captcha；设备槽位RT不等于已提供设备管理UI/API |
| RBACService | 角色/菜单分配与`GetRoleCodesByUserID`；角色直接绑定、有效组织角色、父链BFS展开 |
| MenuService | GetUserMenus/GetUserPermissions；生产菜单写入口已退役 |
| OrgService | GetTree/GetUserOrgs及成员/委托写面 |
| ticket.Service | 工单用例及Resource自注册；详细能力见[工单模块](../modules/ticket.md) |
| resource.Registry | Authorize/GetFilter/Register/List；未知资源默认拒绝，动作判定委派具体 Resource；不拥有后台生命周期 |
| AuditService | Insert同步审计/List查询；判定日志另走PolicyEvalWriter，不混为同一可靠性模型 |

证据：[internal/service/rbac_service.go 的 `RBACService.GetRoleCodesByUserID`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/rbac_service.go)；[internal/service/menu_service.go 的 `MenuService.GetUserMenus / GetUserPermissions`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/menu_service.go)；[internal/service/org_service.go 的 `OrgService.GetTree`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/org_service.go)；[internal/service/audit_service.go 的 `AuditService.Insert`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/audit_service.go)。

#### middleware 层接口

| Middleware | 挂载位置 | 说明 |
|------------|----------|------|
| `Recovery` | 全局 | Panic 恢复 |
| `Logger` | 全局 | 请求日志 |
| `JWT` | 需认证路由 | Token 解析 + 黑名单 + `user:disabled` + 改密地板 `user:at_min_iat`（AT.iat < 地板即 401，2026-11-01 P1-D；三键同趟 pipeline，Redis 故障 503——GET 缺键 redis.Nil 不算故障，bb4760c） |
| `Casbin` | 需鉴权路由 | 路由级 RBAC（RoleFetcher 查角色） |
| `Audit` | 需审计路由 | 操作记录 |
| `RateLimit` | 登录等公开路由 | 登录失败锁在AuthService（Lua）；受保护API与公开认证路由已有Redis RateLimit中间件 |

> **资源级鉴权不在 middleware**。各 Service 通过 `internal/pkg/resource` 的 `ResourceRegistry.Authorize` / `GetFilter` 在 Handler→Service 内联执行（Phase 2 工单等）。详见 [design-decisions.md §5](./design-decisions.md#5-资源级鉴权架构gateway-下放-vs-集中)。

---

### 3.7 Phase5业务协作数据流

| 数据流 | 当前执行链与设计边界 |
|---|---|
| 工单配置→操作 | S1字段/field_meta由后端校验并消费，S2图保存states/transitions（坐标仅浏览器）；S3a转换在L2/L3后叠加condition/validator，提交后执行动作。状态图分支不是多token会签；具体schema/状态/规则设计见工单模块§3–5。 |
| 存储附件 | presign先落pending元数据并签PUT→客户端直传对象存储→confirm以HEAD实测大小/类型及etag后confirmed→ticket态落关联或头像回写object_key→files代理校验可见性/etag。GC处理悬挂/弃置对象，DB事务不包住S3物理删除；富文本服务端消毒，不能把前端预览当安全边界。 |
| 消息 | 提交后DomainEvent分别交TicketNotifyHook/MessageDispatcher及InboxService；前者rules→模板→变量/偏好/节流→sender→message_log，后者独立落notifications。两条消费链不互相证明可靠投递，inbox也不是已配置的外发渠道；真实UIP/IM与变量/类型化路由尾款仍按H组。 |
| 任务/清单/情报 | 工单spawn_task或控制台经TaskrunnerService提交签名请求；执行/重试/死信在taskrunner，回调由主仓jobs注册表消费。AL提供schema化清单数据，经/al反代；S9是主仓PG TTL缓存+外部Provider端口。当前未具备任意SSH/脚本包/DAG、完整任务结果映射或风险R1台账。 |

这些链路由app/providers与NewApp装配；具体能力与缺口见[工单模块§3.4/§6](../modules/ticket.md)、[Phase5各段](../phase5/README.md)与[债台账H组/C-21+](../phase5/S6-S5-debt-ledger.md)。源码证据分别为storage_handler.go的PresignUpload/Confirm/ProxyDownload、ticket/dispatcher.go的MessageDispatcher.Dispatch、ticket/inbox_service.go的InboxService.OnDomainEvent、taskrunner_service.go的TaskrunnerService.Submit及gateway.Registry.Mount（均为本文件996c167基线，前文有固定链接）。

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
│  粒度：created_by / assigned_to 等动作输入                      │
│  策略量：0（代码逻辑判断）                                  │
│  存储：资源表中的属主字段（具体策略按资源）                          │
│  问题："用户A 是文章123的创建者吗？"                       │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

**执行顺序**（2026-08-26 路径 A 拍板，[11-authz §2](../phase2/11-authz-architecture-review.md)）：AT 校验 → 第一层 Casbin（路由级） → 第二层可见性（ltree 组织关系 / assigned scope） → 第三层 canOperate（属主判断，**不短路 L2**）

> **架构定位**（2026-08-11 更新）：遵循 OWASP/NIST 分层鉴权标准，第一层（路由级）在中间件层执行（未来微服务化时由 API Gateway 承担），第二层和第三层（资源级）在 Service 层代码内联执行（未来由各业务服务自行承担）。Gateway 不做资源级鉴权，因为它缺少业务数据上下文。详见 [design-decisions.md#5](./design-decisions.md#5-资源级鉴权架构gateway-下放-vs-集中)。

### 4.2 第一层：路由级 RBAC（Casbin）

**职责**：校验"角色是否有权访问该 API 接口"。

**Casbin 模型**：RBAC，`sub`（角色）→ `obj`（API 路径）→ `act`（HTTP 方法），支持 `keyMatch2` 路径通配。

**策略量设计估算**：角色数×API数×方法数约千级；这是初稿量级示例，不是本次实测条数。实际矩阵/条数见固定审计报告。

**策略示例**（`role::editor` 为业务资源示例角色，非种子四角色）：

```
p, role::admin, /api/v1/*, *
p, role::editor, /api/v1/articles/*, GET
p, role::editor, /api/v1/articles/update, POST
```

> 无 Casbin `g` 段。用户→角色映射在 `user_roles` 表，中间件逐 `role::{code}` enforce。superadmin/admin 在 matcher bypass。

**热更新**：

- 单实例：RBAC写路径先提交DB策略，再LoadPolicy同步本实例内存；不能把AutoSave当作整个事务链的替代
- 多实例：Redis Watcher/PubSub为历史增强方案，当前未注册Watcher；部署多副本前需另行验证同步边界

> 当前角色由RBACService进行三源BFS展开，不只直接user_roles；请求内复用角色快照。Casbin.New启动LoadPolicy；未接Redis Watcher，不将多实例自动同步写成已完成。证据：[configs/casbin_model.conf 的 `matchers`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/configs/casbin_model.conf)；[internal/service/rbac_service.go 的 `RBACService.GetRoleCodesByUserID`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/rbac_service.go)；[internal/casbin/enforcer.go 的 `New`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/casbin/enforcer.go)。

### 4.3 第二层：资源级可见性

Resource接口由业务Service调用，组织树通过ltree提供关系查询，具体资源定义可见集，不能用一条泛化org SQL覆盖所有资源。

工单当前L2读集=属主(created_by/assigned_to) ∪ all scope ∪ 透明读锚点/group子树 ∪ 组织委托 ∪ cc。`GetFilter`与`Authorize`同源表达，过期成员不贡献组织角色/scope/委托；cc只扩大读集。L2在前不意味着属主必须另过组织路径条件：属主本身已是L2的OR分支。旧“转部门自动不可见”叙述与当前已验收规则不同，现按实现明确记录，不改变权限。

**历史方案差异**：初稿scope=1/2/3与`resource_owners/org_permissions` JOIN是通用资源提案，没有随Phase2b建成这两表。当前工单用`org_id/org_path`及ScopeResolver；新资源可用`BuiltinResource`策略/schema守卫，仍须明确自己的过滤条件。

证据：[internal/service/ticket/resource.go 的 `Resource.Authorize / GetFilter`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/resource.go)；[internal/service/ticket/scope_resolver.go 的 `pgxScopeResolver.resolve`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/scope_resolver.go)；[internal/pkg/resource/builtin.go 的 `BuiltinResource.GetFilter / RequireSchema`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/pkg/resource/builtin.go)。

### 4.4 第三层：资源属主判断（ABAC，代码内联）

**职责**：可见后判具体动作；属主只是动作权的输入之一，不能把所有写操作简化为创建人判断。

- 策略量：0（纯代码逻辑）
- 工单实际字段为`created_by/assigned_to`；`canOperate`区分read/comment/note/update/close/transition/assign/delete，并按动作纳入组织委托
- 属主判断（L3）不短路：先过 L2 可见性，属主命中不豁免 L2（对齐 02/11 决策路径 A：L2 在前、属主不短路）

### 4.5 资源抽象与自注册机制

第二层和第三层的鉴权逻辑通过统一的资源接口实现。每种资源类型按各自需要实现 `Resource` 接口，在 Service 构造函数中自注册到 `ResourceRegistry`。Handler 通过 `registry.Authorize(ctx, resourceCode, req)` 统一调用。

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

**历史每资源独立Enforcer提案（未实施）**：对于需要管理员可配置策略的复杂资源，为其创建独立的 Casbin enforcer 实例和独立策略表（`casbin_rule_{resource}`），避免策略互相影响。简单资源用代码内联判断，不引入 Casbin。详见 [design-decisions.md#8 Casbin 策略爆炸](./design-decisions.md#8-casbin-策略爆炸每资源独立-enforcer)。

---

## 5. 认证与 Token 机制

### 5.1 双 Token 方案概述

AT为HS256 JWT，基线TTL30min；RT为7d签名令牌，每用户+device_id一个Redis槽位轮换。JWT只存身份/安全纪元，不存角色/组织权限；L1角色由RBACService实时读取并请求内复用，没有`perm:user:*`权限缓存。

### 5.2 Token Payload 设计原则

| 载荷 | 实际关键字段 |
|---|---|
| AT | uid（string）、username、jti、mcp、pwe、typ=access及标准时间声明 |
| RT | uid（string）、device_id、pwe、typ=refresh及标准时间声明 |

算法/载荷实现归utils，账号状态/密码纪元/Redis会话裁决归主仓，不能在utils里加入工单或IAM策略。
证据：[internal/service/auth_service.go 的 `AuthService.issueTokenPair / Refresh`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/auth_service.go)；[internal/middleware/jwt.go 的 `JWT`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/middleware/jwt.go)；[utils `AccessClaims/RefreshClaims`（`9296ae1`）](https://github.com/tracerbiubiubiu/zhuzhao-utils/blob/9296ae170b71b06c623c54b6010a107406903346/jwt/jwt.go)。

### 5.3 Redis 存储结构

| 当前键 | 语义 |
|---|---|
| refresh:uid:device | RT哈希，不是早稿的设备JSON/jti对象；SET带refresh TTL |
| blacklist:at:jti | 单AT吊销，TTL覆盖其剩余有效期 |
| user:disabled:uid | 禁用/删除即时拒绝 |
| user:pw_epoch:uid | 单调密码纪元；AT/RT签发绑定，改密/重置后旧凭证不得复活 |
| user:at_min_iat:uid | 历史兼容时间地板，不能单独解决同秒新旧凭证 |

Refresh先验签/类型并检查账号，Lua原子消费RT槽并读纪元；纪元不符、空槽、哈希不符分码拒绝。重放会消费该设备槽位并使当前RT也失效，属于已记录的安全取舍；不是宽限窗口重放容忍。旧稿`devices:*` Set和完整设备列表/踢设备API没有当前实现依据，不能用这些草图指导运维。
证据：[internal/service/auth_service.go 的 `consumeRefreshLua / AuthService.Refresh / issueTokenPair`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/auth_service.go)；[internal/middleware/jwt.go 的 `JWT`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/middleware/jwt.go)。

### 5.4 核心流程

Login校验账号/密码及启用的验证码策略后签发绑定纪元的AT/RT；Refresh按上节原子消费槽与纪元裁决；Logout吊销当前AT并移除该设备RT。UpdatePassword推进单调纪元，旧AT/RT不得因新登录复活。具体流程以AuthService四个同名方法和JWT中间件为准，不沿用早期非原子jti草图。

<a id="非法与混用凭证authn-拒绝原则"></a>
### 5.5 JWT 中间件校验流程

用户API由JWT中间件解析AT/PAT并检查吊销/账号/纪元等；真实Redis故障fail-closed，缺键redis.Nil不当作服务故障。强制改密门与后续Casbin分层；禁用、旧纪元与混用凭证不得进入业务handler。PAT是独立生命周期凭证，不能把AT/RT密码纪元语义无条件套到PAT，处置失陷需按runbook同步吊销PAT。

服务间静态AK/SK复用utils HMAC：taskrunner内网回调验签、AL及任务门面出站签名；与普通浏览器JWT链是不同挂载面。旧M2M凭据DB管理/API及RS256/JWKS仍是历史/按需方案；不能因内部AKSK已用而宣称该管理面已实现。存量x-auth兼容仅挂专用受配置控制的组。
证据：[internal/router/router.go 的 `New / mountCompatGroup`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/router/router.go)；[internal/pkg/taskrunner/client.go 的 `Client.do`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/pkg/taskrunner/client.go)；[internal/gateway/gateway.go 的 `buildMount`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/gateway/gateway.go)。

### 5.5.1 JWT中间件与权限缓存（历史增强方案，未实施）

早稿曾拟perm:user缓存与Pub/Sub主动失效；当前仍为服务端有效角色BFS加请求内复用，不是该Redis权限缓存方案。

### 5.6 对外接口

POST `/api/v1/auth/login`、`refresh`、`logout`、`password/update`；GET `captcha`。设备管理、OAuth/SSO与权限Redis缓存按真实需求另批，不沿用“Phase2/3自动具备”的旧排期。
证据：[internal/router/router.go 的 `New`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/router/router.go)。

### 5.7 JWT无状态策略与权限缓存（历史方向）

保留角色/组织权限不进JWT的设计原则。旧perm:user缓存形态及失效广播未落地；跨实例边界见§4.2/§12，单设备RT槽位不是权限缓存或设备目录。

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
| 实体组织 + 虚拟组统一建表 | 用 `is_virtual` 布尔区分（000019，原 `org_type` 四值收敛——行为仅区分实体/虚拟），避免双表 JOIN |
| ltree 路径枚举 | 存储 `root.tech.fe`，支持 `@>`（祖先）和 `<@`（后代）高效查询 |
| 用户-组织多对多 | 一个用户可属于多个组织/组，在组织中有角色 |
| 权限继承 | 通过 ltree 路径关系，scope 控制继承方向 |
| 跨组织支持 | 用户可同时属于不同组织，权限取并集 |

### 6.3 组织类型

| 类型 | is_virtual | source | 说明 | 示例 |
|------|----------|--------|------|------|
| 种子根 / 实体组织 | false | system / hr / local | 公司真实组织架构；HR 同步见 [hr-directory-sync.md](../proposal/hr-directory-sync.md) | 集团、技术中心、产品中心 |
| 虚拟组 | true | local | 挂在实体组织下的项目组/虚拟团队 | `vg_alpha`、某项目组 |

> 000019 起 `org_type` 四值枚举（1=公司 2=部门 3=小组 4=虚拟组）收敛为 `is_virtual` 布尔——行为消费点仅区分实体/虚拟，1/2/3 的层级细分零代码消费（层级由 path/nlevel 表达，展示关注点留展示层）。Phase1起步为手工实体CRUD；source/虚拟组/scope/委托已在后续落地，**HRFetcher/真实HR同步未落地**，不能将预留source=hr视为接入成功。

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

### 8.1 记录内容（最初信息模型，实际字段以AuditLog为准）

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

### 8.2 当前可靠性与边界

普通操作审计在middleware请求结束后以独立超时同步Insert，写失败留日志，不能称其与业务事务原子或绝不丢失。登录/刷新重放公开入口由AuditService显式写；请求断连不直接取消审计写入。敏感字段递归脱敏，具体字段/截断规则以AuditLog中间件为准。

判定日志`PolicyEvalWriter`另走channel→Redis→DB管道，Shutdown有Stop；不能把它的队列语义套到普通audit_logs或工单DomainEvent。工单事件日志也独立于HTTP审计。

证据：[internal/middleware/audit.go 的 `AuditLog / maskSensitiveValue`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/middleware/audit.go)；[internal/service/audit_service.go 的 `AuditService.Insert / LogLogin / LogRefreshReplay`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/audit_service.go)；[internal/pkg/audit/policyeval.go 的 `PolicyEvalWriter`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/pkg/audit/policyeval.go)。

---

## 9. 多租户预留

### 9.1 预留策略

**历史扩展提案，未实施**：当前无tenant隔离模型，下面是将来引入时要改的点，不表示已建tenant_id列或可开关启用：

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

<a id="103-phase-2-预留本文档不展开-ddl"></a>
### 10.3 Phase2–5当前数据面（不重复完整DDL）

| 表 / 能力 | 阶段 | 说明 |
|-----------|------|------|
| 工单/类型/字段/模板/关联 | Phase2底座+Phase5深化 | tickets/ticket_types/ticket_type_fields及模板/关联；配置CAS、field_meta、transition_meta、human_key、超时/催办 |
| 组织增强/委托 | Phase2b/2c | user_orgs scope/expires_at/org_member_role、org_roles、roles.parent_id、owner_user_ids |
| 对象/附件 | Phase5 S5 | file_objects、ticket_attachments及owner/GC索引；对象流在RustFS/S3兼容存储 |
| 消息与收件箱 | Phase5 S6 | message_templates/rules/log、notification_optouts、notifications |
| 协作与自动化 | Phase5 S10/S3a | ticket_cc、tags、ticket_reminders、assignment_rules |
| 横切与生态 | Phase3/4/5分批 | job_submissions、settings/dict/PAT/panic/判定日志、intel_cache等，以迁移地图为准 |
| 尚未建成的旧提案 | 不是当前schema | resource_owners/org_permissions、api_credentials管理面、workflow运行/历史表、domain_event_outbox、risk表族 |

迁移现存编号截至000081，逐号以[11号§4](../review/11-project-control.md)对账，不在本文件分配下一编号。证据：[internal/model/ticket.go 的 `Ticket / TicketType / TicketTypeField`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/model/ticket.go)；[internal/model/storage.go 的 `FileObject / TicketAttachmentView`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/model/storage.go)；[internal/router/router.go 的 `New`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/router/router.go)及对应migrations。

### 10.4 种子数据

> 详见 [01-infra.md](../phase1/01-infra.md) 与 [data-init.md](../proposal/data-init.md)。原则：`ON CONFLICT DO NOTHING`，不覆盖 `created_at` / `created_by`。

| 数据 | 内容 |
|------|------|
| 系统角色 | `superadmin`、`admin`、`operator`、`viewer`（`is_system=true`） |
| 组织 | `root`、`root.tech`、`root.product` |
| fresh迁移初始用户 | 000002种子为工号E000001/密码admin123（bcrypt），must_change_password=true，绑定superadmin与root；首次必须改密 |
| dev/验收账号终态 | acceptance幂等流、UI E2E globalSetup及break-glass CLI收敛为admin12345；不是fresh迁移初始口令 |
| 系统菜单 | 首页 + 系统管理（用户 / 角色 / 菜单 / 组织）及 `menu_apis` |
| Casbin | `p, role::superadmin, *, *` 与 `p, role::admin, *, *` |

E000001密码终态恒为admin12345，测试账号/复原操作唯一权威为[runbook §4](../runbook.md)，不得用本节历史种子表改号。模块级字段语义与API行为见[`modules/`](../modules/)目录。

---

证据：[000002种子INSERT（996c167）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/migrations/000002_seed.up.sql)、[acceptance-phase1登录/改密收敛（996c167）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/scripts/acceptance-phase1.sh)、[UI globalSetup的main凭据闭环（01c5dea）](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/01c5deae23e677853316f1ee2dd692c708246651/e2e/global-setup.ts)。本批只读核对，未登录、改密或改变任何环境账号。

## 11. 部署架构

### 11.1 当前拓扑与环境边界

主仓Go HTTP应用依赖PG/Redis，按功能接S3兼容对象存储、taskrunner和activelist；UI独立构建。单体与兄弟服务的独立部署是两种边界，不能把taskrunner存在写成IAM/工单已微服务化。

开发/测试/生产的配置与卷分开；基线`configs/config.yaml`不为本地态改写，拓扑差异走env/编排。端口唯一对账源为[port-allocation.md](../ops/port-allocation.md)，操作步骤见[runbook](../runbook.md)。`make dev`/dev-stack、主仓compose与兄弟compose是实际入口，本页不维护第二份可执行YAML，避免旧5432/Redis无密码/初始化SQL等样例被误用。

证据：[internal/config/config.go 的 `Load`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/config/config.go)；[internal/app/providers.go 的 `provideGateway / providePostgres / provideRedis`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/app/providers.go)；[Makefile（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/Makefile)、[dev-stack.sh（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/scripts/dev-stack.sh)。

### 11.2 历史部署决策与当前责任

2026-08初稿选择Nginx前置、单实例PG/Redis起步，保留“一套代码、多种部署”的动机。旧“Phase1 HTTP→有域名后Phase2 TLS→Phase3多微服务”是当时路线，并非当前已部署事实，也不是生产免TLS理由。TLS/反代/HA须按实际环境验收；未由本次文档校准证明生产拓扑、Watcher、Sentinel或PG集群已启用。

### 11.3 配置与版本

配置加载以`config.Load`绑定的真实字段/env为准；数据库URL处理、pgxpool参数等不沿用早稿`max_idle_conns`样例。storage/system_settings的运行期配置有各自store，不能将Viper支持热更误写成整份应用配置均能热更新。

当前Go最低版本见go.mod（1.26.10）；用户已明确Go1.27与Node24/26 LTS整体升级放在Phase5合main后的独立main批。本批不升级工具链、不开始Phase6。

---

## 12. 并发与可靠性

<a id="121-并发问题与应对"></a>
### 12.1 当前机制

| 场景 | 当前保证 | 限制 |
|---|---|---|
| RT刷新/改密吊销 | 原子消费RT槽+密码纪元；AT也核纪元 | 多标签迟到旧RT仍可能触发当前设备槽失效，未启用family宽限 |
| 类型/字段/图更新 | 配置version CAS、字段替换事务，状态转换锁定同代单据/类型快照 | 不推广为所有资源都有version或任意跨存储事务 |
| 组织与委托 | SQL事务/版本及到期过滤；具体组织移动锁规则以Repo实现为准 | 不是旧文档预设的Redis org-tree缓存快照 |
| 周期内务 | CronScheduler ticker + **专用PG会话advisory lock**，同任务多副本互斥 | 业务仍须可重入；与taskrunner同名job双启不由本仓锁自动合并 |
| 对象与DB | 元数据状态机、同事务绑定/退役、best-effort物理删除与GC | 不存在DB与S3原子提交；失败/崩溃需按对应状态补偿 |
| 情报缓存 | threatintel.Service的TTL缓存/单飞与过期策略 | 不等于全仓权限/菜单/组织Redis缓存 |

证据：[internal/service/auth_service.go 的 `AuthService.Refresh`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/auth_service.go)；[internal/repository/ticket_transition_snapshot.go 的 `TicketRepo.LockTransitionSnapshotTx`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/repository/ticket_transition_snapshot.go)；[internal/service/cron_scheduler.go 的 `CronScheduler.runLocked`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/cron_scheduler.go)；[internal/service/storage/service.go 的 `Service.BindAvatar`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/storage/service.go)；[internal/pkg/threatintel/service.go 的 `Service.Query`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/pkg/threatintel/service.go)。

### 12.2 分布式锁

当前运行锁见上表CronScheduler的PG会话锁；旧Redis锁/缓存方案记录在§12.6，不是已实现保证。

### 12.3 事务性分析

一个业务事务由Service拥有；事务窗口内需要数据时用同一tx，不能再向同一pgxpool取第二条连接（BK11：低并发可能假绿，池饱和会死锁）。当前已有Tx方法和守卫，不把它扩大成“全仓DBTX抽象已完成”；dbtx仍按挂账试点推进。

工单日志、附件落绑等按具体用例事务处理；HTTP审计、对象流、Redis状态、外部任务提交及通知各有独立失败模型。跨存储失败不能统一声称“最多性能损失”或“部分失败不影响安全”。角色策略提交后的内存同步也要核实际错误处理，未装Watcher不能用其兜底。

### 12.4 审计日志可靠性

普通HTTP审计=独立超时同步写DB；权限判定日志=PolicyEvalWriter队列管道；工单业务日志=具体Service事务写入。这三者各自的失败/退出行为见§8、§14，不再沿用“主仓所有审计均Redis List可靠队列”的旧分档概括。

### 12.5 跨实例与领域事件（当前at-most-once）

`DomainEvent`在事务提交后经进程内`HookRegistry`派发，goroutine/recover隔离订阅者；没有持久化Outbox、processed轮询或自动重放。`ticket_events`是业务日志，不能推导通知事实已持久化。`Drain`收窄计划内退出窗口，不覆盖commit至派发前崩溃、进程崩溃或排空超时。

inbox消费后落notifications，外发记message_log，二者均不把上游总线升级成at-least-once。未来R3风险联动规划对账，C-20 Outbox按高一致性/跨进程消费触发；taskrunner的Asynq重试属于任务域。

证据：[internal/pkg/domainevent/registry.go 的 `HookRegistry.Dispatch / Drain`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/pkg/domainevent/registry.go)；[internal/service/ticket/service.go 的 `Service.emitTicketEvent`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/service.go)；[internal/service/ticket/inbox_service.go 的 `InboxService.OnDomainEvent`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/inbox_service.go)；[Phase5 C-20](../phase5/S6-S5-debt-ledger.md)、[Phase6 R3](../phase6/03-r3-ticket-linkage.md)。

### 12.6 缓存策略（历史方案，未实施）

早稿列过Redis `lock:org:tree`、`lock:casbin:reload`、Watcher跨实例广播，以及`perm:user/menu:user/orgs:user/authz:*`的TTL/主动失效方案。它们是历史候选，当前源中没有相应全套实现；也不能声称Casbin reload在一个实例执行即可让其他实例内存自动更新。未来多实例或权限缓存须按新的验收窗口设计，不借用上述方案当现状。

---

## 13. 安全加固

> 当前安全能力按下列源码边界表述；密码历史/地理风控等旧候选不因阶段号自动完成。

### 13.1 密码安全

> Phase 1：bcrypt 存储 + 管理员重置 + 首次改密（`mcp`）。下列邮箱重置、密码历史等为 Phase 2+ 可选能力。

| 措施 | 说明 |
|------|------|
| 密码存储 | 复用utils crypto的bcrypt实现；参数以该包为准 |
| 密码校验 | 以当前validate与账号写入口为准；旧“必须四类字符”的完整策略不是本表可宣称的现实现 |
| 密码历史 | Phase 2+：记录最近 5 次 hash，防重用 |
| 密码重置 | Phase 1：管理员 `POST /users/password/reset`；Phase 2+：邮箱/手机一次性 token |
| 密码过期 | Phase 2+ 可选，如 90 天强制修改 |

### 13.2 登录安全（现行与历史增强边界）

| 措施 | 说明 |
|------|------|
| 登录限流 | 同一 **employee_no** 15 分钟内失败 **5** 次 → 429（Redis **Lua** LoginLocker，key `lock:login:{employee_no}`） |
| 防用户枚举 | 用户不存在 / 密码错误 / 登录时账号禁用 → 同一 401 文案 |
| 验证码 | 已有Captcha端点与可选配置；没有据此证明“失败三次才触发”的自适应策略 |
| 异地登录检测 | Phase 2+：IP 归属异常时审计或二次验证 |

### 13.3 API 安全

| 措施 | 说明 |
|------|------|
| CORS | 当前策略以middleware.CORS为准；生产来源策略按部署要求验收，不把早稿建议当已配置 |
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

Wire负责构造，App显式编排，ResourceRegistry只鉴权。`App.Run`启动HTTP、PolicyEvalWriter及配置启用的内务CronScheduler：audit_archive/storage_gc/timeout_scan/inbox_cleanup/intel_cleanup/storage_usage_sample/notify_volume_check。注册状态与实际StartedAt分开观察，不能将注册条目当goroutine已运行。

`App.Shutdown`先HTTP Shutdown（30s），再PolicyEvalWriter Stop（5s，残留留Redis并告警），再领域事件Drain（8s，超时告警），最后由main的Wire cleanup释放连接。信号ctx取消cron来源；App并没有旧草图的通用`Runner[]`、主仓`cmd/worker`或主仓Asynq消费者。

历史设计曾拟Start/Stop统一Runner与审计channel排空，保留“被动Domain Service不拥有进程生命周期”的原则；具体实现已按上述组件分开，不能复制旧Runner接口作为现行契约。
证据：[internal/app/app.go 的 `App.Run / Shutdown`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/app/app.go)；[cmd/server/main.go 的 `main`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/cmd/server/main.go)；[internal/service/cron_registry.go 的 `CronRegistry`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/cron_registry.go)。


### 14.2 健康检查

| 探针 | 路径 | 检查内容 |
|------|------|----------|
| Liveness | `/health/live` | 进程存活（直接返回 200） |
| Readiness | `/health/ready` | DB 连通性 + Redis 连通性（ping 两者，全通过返回 200） |

### 14.3 当前观测与历史完整栈方案

当前具备request_id/slog、健康探针、`/metrics` Prometheus文本、cron状态和采样水位。metrics由主仓手写计数/gauge渲染，含HTTP/panic/5xx与存储/通知/GC观测；外部Prometheus/Grafana不是启动依赖。

早稿的`observability.tracing.enabled`、OTLP/Collector、Sentry、debug即开pprof等完整栈是候选，不是当前已接线配置；不能因go.mod间接出现OTel（如测试依赖）就认为生产链路已启用追踪。
证据：[internal/middleware/metrics.go 的 `MetricsMiddleware / RenderMetrics`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/middleware/metrics.go)；[internal/router/router.go 的 `New`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/router/router.go)；[internal/app/app.go 的 `NewApp`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/app/app.go)。


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

首次初始化/新增种子默认使用`ON CONFLICT DO NOTHING`，不覆盖created_at/created_by。后续迁移若明确调整既有授权或业务语义，应单独说明兼容与回滚，不能把该原则理解成所有UPDATE都禁止。详见 [design-decisions.md#7 系统重启与数据初始化幂等性](./design-decisions.md#7-系统重启与数据初始化幂等性)。

```sql
-- 正确：幂等，不覆盖（主键 BIGSERIAL 自增，冲突键用 code）
INSERT INTO roles (code, name, is_system) VALUES
  ('admin', '管理员', true)
ON CONFLICT (code) DO NOTHING;

-- 有意更新存量语义须单独评审；这不是普通首次初始化，不可顺便覆盖审计字段
INSERT INTO roles (code, name, is_system) VALUES
  ('admin', '管理员', true)
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name;
```

**运行时 Sync 安全规则**：

应用启动时的数据同步（如 Casbin 策略同步）遵循：
1. `created_at`/`created_by` 永远不覆盖
2. 系统资源（`is_system = true`）不被删除
3. 必须区分可降级同步与安全目录守卫；当前`AuditRouteCatalog`/`AuditPageWriteBindings`等启动核对失败会拒启，不能统一写成“Sync失败仅Warn不阻塞”

---

## 15. 推荐库与开源参考

> **2026-08历史选型/候选清单，非当前依赖清单**。表中的sqlc/limiter/Watcher/OTel等推荐不表示已引入；实际依赖看go.mod与生产import，当前生效组件见前文。保留这些表是为了说明当时为何选Gin/pgx/Wire等，不据此重复推荐或默认启动新栈。

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
| Handler集成测试 | HTTP契约与实际服务/仓储 | 当前多处使用真实PG+Service/Gin，必要外部适配可stub；不能统一写成Mock Service |

### 15.8 可观测性（历史候选；当前手写metrics见§14.3）

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
| 429 | 登录/受保护API限流 |
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

完整方法/路径/链路注册以`router.New`、生成Swagger、`CatalogExemptRoutes`目录为准。此处按职责索引，移除早稿已退役POST菜单与不存在的设备/单类型GET等断言，避免维护另一份“完整清单”。

| 入口族 | 当前代表入口/边界 |
|---|---|
| 认证/自服务 | auth login/refresh/captcha；logout/password/update；user profile/menus/permissions；PAT/inbox等身份作用域面 |
| IAM管理 | users/roles/orgs等GET/POST；成员/委托路由单独检查组织权限 |
| 菜单 | 生产只注册GET树与详情，**无POST /menus、update、delete**；菜单目录/权限种子走迁移与守卫 |
| 工单用例 | tickets CRUD、close/assign、transition与available、comments/notes/relations、export |
| 工单配置 | ticket-types列表/字段读取、类型/字段/模板写面、assignment-rules；没有GET /ticket-types/:code单类型路由 |
| 工单协作/存储 | cc/tags、badges、storage presign/avatar bind、attachments confirm/list/delete、files代理、richtext render；已登记selfService不代表免L2/L3 |
| 消息 | message-templates/rules/logs、inbox与偏好；外发真实渠道边界见工单模块§6 |
| 任务与AL | jobs门面经服务端签名访问taskrunner；`/al`反代先用户认证/路由授权再出站AKSK；internal/jobs/callback为独立签名链 |
| 系统/观测 | system settings、cron-status、健康、metrics、Swagger等；访问与暴露范围按实际注册/部署分别验收 |

证据：[internal/router/router.go 的 `New / mountCompatGroup`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/router/router.go)；[internal/router/catalog.go 的 `CatalogExemptRoutes / AuditRouteCatalog / AuditPageWriteBindings`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/router/catalog.go)；[internal/service/menu_service.go 的 `MenuService.GetUserMenus`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/menu_service.go)。

页面行仅挂GET、写端点挂按钮；唯一已登记页面写例外=000068工单transition。L1通过只代表路由可达，不能代替后续具体资源检查。旧“ticket:export/admin专属”等摘要已经被实际菜单绑定取代，工单详情见[modules/ticket §2](../modules/ticket.md)。

---

### 17.4 组织模块

GET组织树/详情、POST创建/更新/移动注册在biz组；删除、成员/scope/owner与组织角色写面使用selfService并另做OrgService检查。GET /orgs/:id/members仍为业务GET面，GET /orgs/members/list是委托名册，两者不能互相冒充完整清单。用户侧POST /users/orgs与组织侧成员入口共享OrgService用例，执行前须核全局/委托权。

### 17.6 审计模块

GET /api/v1/audit/logs按Casbin路由授权查询；同步审计写与判定日志管道分开，失败/脱敏范围见§8/§12。导出、文件投递等有自身审计编排，不能从本接口推出全部操作和业务事务同原子提交。

以上两小节保留旧入站章节锚点，路径与链路仍以本节router.New固定源码为准。

## 18. 分阶段实施计划

| 阶段/决策时点 | 已发生演进 | 不能由阶段名称推导的能力 |
|---|---|---|
| 2026-08 Phase1 | 从认证鉴权草案落到Go/PG/Redis/Wire、角色/组织/菜单/同步审计 | 当时不含资源工单、内部AKSK、审批引擎 |
| Phase2a→2b→2c | 工单资源与模板/关联，组织scope/透明读/委托、BFS有效角色 | 附件与HR早期曾排2b但并未据此全部交付；附件后归S5，HR真实同步仍缺 |
| 2026-08-25 / 09-02历史暂停 | 工单深化曾暂停/封版，SLA/审批/L1/Asynq设计保留 | 不是当前Phase3整体暂停，也不是这些旧设计已落码 |
| Phase3/4 | 按切片推进平台/生态能力与前端控制台，任务执行器和AL独立仓接入 | 未实现的Watcher、IAM微服务拆分、RS256/JWKS、完整OTel不能用“阶段收口”虚标 |
| 2026-10-09翻案→Phase5 | 自研工单恢复，字段/状态图、转换/分派/动作、存储富媒体、消息/inbox、协作/超时/情报缓存落地 | S3b审批、完整SLA、结果输入输出映射、真实UIP/IM渠道及风险模块仍按债/规划边界 |
| 2026-11当前Phase5 | 全生态审计、分批修复、门禁、文档/分支收口 | 本文件不是main已合或生产已部署声明 |
| Phase6与后续 | 风险R1–R4及后续业务/连接器/编排等按各阶段稿规划 | **Phase5先收口并经授权合main，再从main切Phase6**；批准规划不是已建表/API，也不自动启动远期阶段 |

当前能力取本文件前述源码与11号；阶段历史原稿保留于[phase1](../phase1/README.md)/[phase2](../phase2/README.md)/[phase3](../phase3/README.md)/[phase4](../phase4/README.md)/[phase5](../phase5/README.md)，未实现项与接续边界取[Phase5债台账](../phase5/S6-S5-debt-ledger.md)、[Phase6 README §5与07索引](../phase6/README.md)、[roadmap](../roadmap.md)。

---

## 19. 决策状态总览

### 19.1 当前采用

| 事项 | 当前裁定 | 证据入口 |
|---|---|---|
| 单体/领域 | 主仓模块化单体；工单service领域隔离；兄弟仓各负其责 | §1/§3及工单模块 |
| 鉴权 | 路由RBAC+资源L2/L3；L2含属主/组织/委托/抄送并集，不把执行顺序当组织强隔离规则 | §4 |
| 凭证 | HS256身份JWT、RT哈希槽、AT/RT密码纪元、PAT独立生命周期、内部静态AKSK | §5 |
| 状态/自动化 | StateMachine邻接表+转换元数据；创建后规则分派与post_actions | 工单模块§3–5 |
| 消息/任务 | DomainEvent进程内at-most-once；inbox真实落库；外发UIP桩；任务执行在taskrunner | §1/§12 |
| 内务/观测 | 本仓CronScheduler+PG会话锁，手写metrics，显式生命周期 | §12/§14 |
| 审计 | HTTP同步审计、判定日志队列、业务事件日志分开 | §8/§12 |
| 配置/工具 | env覆盖冻结基线，Wire/Swag固定CLI入口；版本升级另批 | §11、go.mod、Makefile |
| 演进 | 保留早期设计动机，以代码/SSOT标清当前和未来 | §18 |



### 19.2 后续与触发项

| 事项 | 当前状态/接续依据 |
|---|---|
| 风险R1–R4/工单完善 | Phase6规划已批准的部分仍须满足启动前置；不在本批实现 |
| 审批S3b/BranchedStateEngine/远程引擎 | 历史/后续设计，尚无当前实现；可视化状态图不等于审批运行时 |
| 完整响应/解决SLA、工作日历、运营统计DSL | 超出当前SLA0.5与有限列表/导出能力 |
| Outbox/跨进程领域事件 | C-20触发驱动；R3对账按Phase6设计，不能预称at-least-once |
| 真实外发渠道/任务结果映射 | 受Phase5 H组、C-21+与Phase6/06等挂账约束 |
| Watcher/权限缓存/拆IAM/RS256/JWKS/多租户/完整OTel | 历史增强方向，当前没有上述整套运行实现；按实际需求另批 |
| Go1.27、Node24/26 LTS升级 | 所有者明确放在Phase5合main后基于main的独立升级批 |

文档债收口不等于上述能力销账；本轮现行断言完成源码核对后，剩余措辞/记账类P2/P3按审计截断规则仅记录，不继续抛光扩面。
