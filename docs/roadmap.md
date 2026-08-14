# 实施路线图（Roadmap）

> 三阶段实施总览，每阶段的核心目标、模块清单和能力边界。
>
> 创建日期：2026-08-12

---

## 总览

```
Phase 1：最小可用                    Phase 2：业务可用（工单）           Phase 3：生产加固
┌─────────────────────────┐     ┌─────────────────────────┐     ┌─────────────────────────┐
│ 认证鉴权框架             │     │ 资源级鉴权 + 工单        │     │ 多实例 + 可观测 + HA     │
│ · 登录 / 双 Token        │────▶│ · 资源级鉴权（ltree）    │────▶│ · Watcher / 分布式锁     │
│ · 登录限流 / 会话吊销    │     │ · 虚拟组 / scope         │     │ · Metrics / 追踪         │
│ · 路由级 RBAC            │     │ · 对象存储 + 工单        │     │ · Outbox + Asynq         │
│ · 用户/角色/菜单/组织    │     │ · 多设备 UI / 密码策略   │     │ · 拆服务 / gRPC / RS256  │
│ · 同步审计               │     │                         │     │ · PG Cluster / Redis HA  │
└─────────────────────────┘     └─────────────────────────┘     └─────────────────────────┘
   单实例 Docker Compose            单实例 Docker Compose            多实例 + Nginx
```

---

## Phase 1：最小可用

**核心目标**：为后续所有服务搭好认证鉴权框架。

| 模块 | 核心能力 | 文档 |
|------|---------|------|
| 基础设施 | DB 迁移、配置、Wire DI、优雅关闭、健康检查 | [phase1/01-infra.md](./phase1/01-infra.md) |
| 认证 | 登录、双 Token、RT 轮换、登出、黑名单、登录限流、会话吊销 | [phase1/02-auth.md](./phase1/02-auth.md) |
| 鉴权 | 路由级 Casbin RBAC、ResourceRegistry 空接口 | [phase1/03-authz.md](./phase1/03-authz.md) |
| 用户 | CRUD、启用禁用、密码修改、角色绑定、超管保护 | [phase1/04-user.md](./phase1/04-user.md) |
| 角色 | CRUD、菜单分配、Casbin 策略同步 | [phase1/05-role.md](./phase1/05-role.md) |
| 组织 | 树形 CRUD、ltree 路径、用户关联 | [phase1/06-organization.md](./phase1/06-organization.md) |
| 菜单 | CRUD、菜单树、权限码、前端数据 | [phase1/07-menu.md](./phase1/07-menu.md) |
| 审计日志 | 操作日志中间件、同步写入、登录审计 | [phase1/08-audit.md](./phase1/08-audit.md) |
| 中间件 | JWT（含 fail-close）、Casbin、gin-contrib CORS/RequestID/slog、安全头 | [phase1/09-middleware.md](./phase1/09-middleware.md) |
| 并发与事务 | DB 事务、SyncedEnforcer、Redis 原子操作、乐观锁 | [phase1/10-concurrency.md](./phase1/10-concurrency.md) |

**部署形态**：单实例 Docker Compose（PG + Redis + App）——**默认验收拓扑**；同一套代码可通过配置/编排扩展为多副本、PG Cluster、Redis Sentinel（[design-decisions §18](./design/design-decisions.md#18-部署与代码解耦一套代码多种部署)）。

**验收标准**：主路径 + 对抗路径（限流、会话吊销、最后一个 superadmin、Redis 503），见 [phase1/README.md](./phase1/README.md) §1.3。Phase 1 **不做**部门数据隔离。

---

## Phase 2：业务可用

**核心目标**：资源级鉴权 + 工单。仍为模块化单体，不拆 IAM。

**子阶段**：**2a**（Registry + 工单 MVP）→ **2b**（组织 scope/虚拟组/HR + 附件）→ **2c**（组织内委托：owner + 组内 admin/member）。详见 [phase2/README.md](./phase2/README.md) §0。

| 子阶段 | 模块 | 核心能力 | 文档 |
|--------|------|---------|------|
| **2a** | 资源级鉴权 + 工单 MVP | Registry、属主、assigned 过滤、工单 CRUD+状态机 | [phase2/02-authz-resource.md](./phase2/02-authz-resource.md)、[09-ticket.md](./phase2/09-ticket.md) |
| **2b** | 组织增强 + 存储 + 体验 | 虚拟组/scope、HR 目录同步、附件、多设备 UI、密码策略 | [phase2/03](./phase2/03-org-enhance.md)、[10-storage](./phase2/10-storage.md)、[01-auth-enhance](./phase2/01-auth-enhance.md)、[09-ticket §2b](./phase2/09-ticket.md) |
| **2c** | 组织内委托 | owner、org_member_role、组内防提权、工单 Authorize | [phase2/04-org-delegation.md](./phase2/04-org-delegation.md) |

**部署形态**：单实例 Docker Compose

**明确后移**：RS256、AK/SK、缓存平台、审计异步、每资源 Enforcer、IAM 拆分 → Phase 3 或按需。

---

## Phase 3：生产加固

**核心目标**：可观测性、多实例部署、高可用。

| 模块 | 核心能力 | 文档 |
|------|---------|------|
| 可观测性 | Prometheus Metrics、Grafana、OpenTelemetry | [phase3/01-observability.md](./phase3/01-observability.md)（**已编写**） |
| 多实例部署 | Casbin Watcher、跨实例事件广播、分布式锁 | phase3/02-multi-instance.md（待编写） |
| 审计日志 L2 | Redis List 队列，进程崩溃不丢 | phase3/03-audit-l2.md（待编写） |
| 事件驱动 | PostgreSQL Outbox + Asynq | phase3/04-event-driven.md（待编写） |
| 微服务拆分 | gRPC、IAM 独立、API Gateway、RS256+JWKS | phase3/05-microservice.md（待编写） |
| 高可用 | PG Cluster、Redis Sentinel、Nginx 负载均衡 | phase3/06-ha.md（待编写） |
| 安全增强 | 异地登录检测、验证码、密码过期、API 限流 | phase3/07-security-enhance.md（待编写） |
| 平台增强 | 缓存体系、AK/SK（有调用方时） | phase3/09-platform.md（待编写） |
| 运维工具 | Swagger CI、DB 迁移 CI、集成测试自动化 | phase3/08-ops.md（待编写） |

**部署形态**：多实例 + Nginx 负载均衡 + PG Cluster + Redis Sentinel

**子阶段**：建议 **3a**（可观测 + 多实例 + HA）先上线，**3b**（拆服务 + RS256 + 平台）按需。详见 [phase3/README.md](./phase3/README.md) §0。

---

## 预留扩展（按需启用）

| 能力 | 预留点 | 启用条件 |
|------|--------|----------|
| 多租户 | tenant_id + Casbin 模型 | 有多客户需求时 |
| 第三方登录 | oauth_provider + oauth_id | 接 SSO/OAuth 时 |
| 消息队列（Kafka） | 审计日志通道可替换 | 日志量大或需重放时 |
| K8s 部署 | Docker Compose 可平滑迁移 | 团队有 K8s 运维能力时 |
| 密码过期策略 | users 表无额外字段 | 合规要求时 |

---

## 文档体系总览

```
docs/
├── design/                     # 架构设计（高层）
│   ├── architecture.md         # 总体架构文档
│   ├── design-decisions.md     # 设计决策记录
│   ├── implementation-plan.md  # 已废弃，见 phase1/
│   └── system-comparison.md    # 新旧系统对比
├── proposal/                   # 方案提案（详细）
│   ├── overview.md             # 总览
│   ├── auth-design.md          # 认证设计
│   ├── resource-model.md       # 资源模型
│   ├── data-init.md            # 数据初始化
│   └── deployment-evolution.md # 部署演进
├── modules/                    # 模块设计（跨阶段）
│   ├── auth.md / authz.md / user.md / role.md
│   ├── organization.md / menu.md / audit.md
│   ├── middleware.md / ticket.md
├── phase1/                     # Phase 1 详细实现计划
│   ├── README.md               # 大纲 + 边界 + 实施顺序
│   ├── 01-infra.md ~ 10-concurrency.md
├── phase2/                     # Phase 2 详细实现计划
│   ├── README.md               # 大纲 + 边界 + 实施顺序
│   └── 01-auth-enhance ~ 04-org-delegation（Phase 2 全套已编写）
├── phase3/                     # Phase 3 详细实现计划
│   ├── README.md               # 大纲 + 边界 + 实施顺序
│   └── 01-observability 已编写；02–09 待编写
├── roadmap.md                  # 本文：跨阶段总览
├── api/                        # API 文档
├── ops/                        # 运维文档
└── adr/                        # 架构决策记录
```

**文档层级关系**：
- `design/` — 为什么这样设计（决策与权衡）
- `proposal/` — 具体方案是什么（详细提案）
- `modules/` — 模块完整设计（跨阶段的完整形态）
- `phase1/` `phase2/` `phase3/` — 每阶段做什么（分阶段实施计划）
- `roadmap.md` — 三阶段总览（本文）
