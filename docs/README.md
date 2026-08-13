# 文档目录

> 本项目所有文档的组织入口。按文档类型分目录存放。

## 目录结构

```
docs/
├── README.md                        # 本文件，文档索引
├── roadmap.md                       # 三阶段实施路线图总览
│
├── design/                          # 框架设计文档（技术架构层面）
│   ├── architecture.md              # 系统架构（What：系统是什么）
│   ├── design-decisions.md          # 设计决策（Why：为什么这么选）
│   ├── implementation-plan.md       # 实施计划（How：怎么搭）
│   └── system-comparison.md         # 现有系统对比分析
│
├── proposal/                        # 综合方案文档（业务+技术整合）
│   ├── overview.md                  # 方案总览：目标、场景、架构全景
│   ├── auth-design.md               # 认证鉴权方案：AuthN + AuthZ 完整设计
│   ├── data-init.md                 # 数据初始化与幂等性方案
│   ├── resource-model.md            # 资源抽象与自注册机制方案
│   └── deployment-evolution.md      # 部署演进：单体 → 微服务化路径
│
├── modules/                         # 模块级设计文档（各模块完整设计，跨阶段）
│   ├── README.md                    # 模块索引与依赖关系
│   ├── auth.md                      # 认证模块：登录、双Token、RT轮换
│   ├── user.md                      # 用户模块：CRUD、密码、角色绑定
│   ├── role.md                      # 角色模块：CRUD、菜单分配、策略同步
│   ├── organization.md              # 组织模块：ltree树形、成员、虚拟组
│   ├── menu.md                      # 菜单模块：三类型、API绑定、前端权限
│   ├── authz.md                     # 鉴权模块：路由级Casbin + 资源级Registry
│   ├── audit.md                     # 审计模块：访问日志、操作审计、异步写入
│   ├── middleware.md                # 中间件模块：JWT、CORS、限流、安全头
│   └── ticket.md                    # 工单模块：类型配置、状态机、权限模型
│
├── phase1/                          # Phase 1：最小可用（认证鉴权框架）
│   ├── README.md                    # 大纲 + 边界 + 实施顺序 + 测试策略
│   ├── 01-infra.md                  # 基础设施
│   ├── 02-auth.md                   # 认证（含 AK/SK 骨架）
│   ├── 03-authz.md                  # 鉴权（路由级 + Registry 骨架）
│   ├── 04-user.md                   # 用户
│   ├── 05-role.md                   # 角色
│   ├── 06-organization.md           # 组织
│   ├── 07-menu.md                   # 菜单
│   ├── 08-audit.md                  # 审计日志 + 应用日志规划
│   ├── 09-middleware.md             # 中间件
│   └── 10-concurrency.md            # 并发与事务
│
├── phase2/                          # Phase 2：业务可用（资源级鉴权 + 安全 + 工单）
│   ├── README.md                    # 大纲 + 边界 + 实施顺序
│   └── (01~09 待编写)
│
├── phase3/                          # Phase 3：生产加固（多实例 + 可观测性 + HA）
│   ├── README.md                    # 大纲 + 边界 + 实施顺序
│   └── (01~08 待编写)
│
├── api/                             # API 文档
│   └── (Swagger 生成)                # Phase 1 后期由 swag 自动生成
│
├── ops/                             # 运维文档
│   ├── deployment.md                # 部署指南
│   └── runbook.md                   # 故障处理手册
│
└── adr/                             # 架构决策记录（ADR）
    └── README.md                    # ADR 索引与模板说明
```

## 文档分类说明

### design/ — 框架设计文档

项目"设计阶段"产出的技术架构文档，描述系统结构、决策推理和实施步骤。

| 文档 | 定位 | 读者 |
|------|------|------|
| `architecture.md` | 系统全貌：模块边界、数据库 schema、API 总表、分阶段计划 | 所有开发者 |
| `design-decisions.md` | 决策推理：方案对比、Q&A 讨论、故障场景分析 | 架构 review |
| `implementation-plan.md` | 实施步骤：Phase 1 的逐步搭建计划与验收标准 | 执行开发者 |
| `system-comparison.md` | 现有系统审计对比：旧系统 vs 新框架的差异与决策 | 架构 review |

### proposal/ — 综合方案文档

结合业务场景和旧系统经验整理的完整方案设计。与 `design/` 的区别：
- `design/` 关注"框架怎么搭"（技术视角）
- `proposal/` 关注"业务怎么做"（方案视角，含场景、流程、演进路径）

| 文档 | 定位 | 内容 |
|------|------|------|
| `overview.md` | 方案总览 | 目标系统、典型场景、架构全景图、技术选型汇总 |
| `auth-design.md` | 认证鉴权方案 | AuthN（双 Token）+ AuthZ（分层鉴权 + 资源抽象）完整设计 |
| `data-init.md` | 数据初始化方案 | 迁移分层、种子数据幂等、运行时 Sync 安全规则 |
| `resource-model.md` | 资源模型方案 | 资源接口抽象、自注册机制、每资源独立 Enforcer |
| `deployment-evolution.md` | 部署演进方案 | 单体底座 → IAM 独立部署 → 微服务化路径 |

### modules/ — 模块级设计文档

每个核心模块的详细设计，包含数据模型、接口定义、核心流程、旧系统借鉴、分阶段实施。

| 文档 | 模块 | 核心职责 |
|------|------|---------|
| `auth.md` | 认证 | 登录、双 Token、RT 轮换、登出、多设备 |
| `user.md` | 用户 | CRUD、密码管理、角色绑定 |
| `role.md` | 角色 | CRUD、菜单分配、Casbin 策略同步 |
| `organization.md` | 组织 | ltree 树形、成员管理、组织角色 |
| `menu.md` | 菜单 | 三类型、API 绑定、前端权限 |
| `authz.md` | 鉴权 | 路由级 Casbin + 资源级 ResourceRegistry |
| `audit.md` | 审计 | 访问日志、操作审计、异步写入 |
| `middleware.md` | 中间件 | JWT、CORS、限流、安全头 |
| `ticket.md` | 工单 | 类型配置、状态机、权限模型 |

> `modules/` 描述每个模块的**完整形态**（跨阶段），`phase1/` `phase2/` `phase3/` 描述每阶段**做什么**。

### phase1/ phase2/ phase3/ — 分阶段实现计划

三阶段实施计划，每阶段一个目录，含 README 大纲和各模块详细计划。

| 阶段 | 目标 | 部署形态 |
|------|------|---------|
| [phase1/](./phase1/README.md) | 最小可用：认证鉴权框架 | 单实例 Docker Compose |
| [phase2/](./phase2/README.md) | 业务可用：资源级鉴权 + 安全 + 工单 | 单实例 Docker Compose |
| [phase3/](./phase3/README.md) | 生产加固：多实例 + 可观测性 + HA | 多实例 + Nginx + PG Cluster |

### roadmap.md — 三阶段总览

跨阶段的全景视图，一页看清三阶段的核心目标、模块清单和部署形态。详见 [roadmap.md](./roadmap.md)。

### api/ — API 文档

接口规格说明，供前后端联调使用。

- Phase 1 后期接入 `swag`，从代码注解自动生成 OpenAPI 文档
- 如需手写接口文档（如非 RESTful 接口），放在此目录

### ops/ — 运维文档

部署、配置、监控、故障处理等"上线后"所需的文档。

| 文档 | 内容 | 产出时机 |
|------|------|---------|
| `deployment.md` | 环境要求、配置项说明、部署步骤、Docker/K8s 配置 | Phase 1 |
| `runbook.md` | 常见故障现象、排查步骤、应急操作 | Phase 3 |

### adr/ — 架构决策记录

轻量级的决策日志，每个 ADR 记录一个独立的架构决策。

与 `design-decisions.md` 的区别：
- `design-decisions.md`：讨论过程的完整记录（含方案对比、推理细节）
- `adr/`：决策的最终结论摘要，编号管理，便于追溯

> ADR 格式适合在项目稳定后，从 `design-decisions.md` 中提取关键决策形成独立记录。Phase 1 暂不需要，目录预埋。

## 文档规范

### 命名

- 全小写，连字符分隔：`deployment-guide.md` 而非 `DeploymentGuide.md`
- 语义优先：`architecture.md` 而非 `doc1.md`

### 关联引用

文档间引用使用相对路径：

```markdown
详见 [设计决策](./design/design-decisions.md#1-jwt-无状态策略与权限缓存)。
```

### 更新时机

| 文档 | 更新时机 |
|------|---------|
| `design/architecture.md` | 架构变更时（新增模块、调整边界） |
| `design/design-decisions.md` | 每次有新的设计讨论 |
| `design/implementation-plan.md` | 每个 Phase 开始时 |
| `proposal/*` | 方案级设计变更时 |
| `api/` | 接口变更时 |
| `ops/` | 部署配置变更、新增故障案例时 |
