# 实施路线图（Roadmap）

> 三阶段实施总览，每阶段的核心目标、模块清单和能力边界。
>
> 创建日期：2026-08-12

---

## 总览

```
Phase 1：最小可用（跑起来）          Phase 2：业务可用（完善）           Phase 3：生产加固（上线）
┌─────────────────────────┐     ┌─────────────────────────┐     ┌─────────────────────────┐
│ 认证鉴权框架             │     │ 资源级鉴权 + 安全加固     │     │ 多实例 + 可观测性 + HA   │
│                         │     │                         │     │                         │
│ · 登录 / 双 Token        │────▶│ · 多设备管理              │────▶│ · Casbin Watcher         │
│ · 路由级 RBAC            │     │ · 登录限流 / 锁定          │     │ · 跨实例事件广播          │
│ · 用户 / 角色 / 菜单 CRUD │     │ · 资源级鉴权（ltree）      │     │ · 分布式锁               │
│ · 组织树 CRUD            │     │ · 虚拟组 / 组织级权限      │     │ · Metrics + 追踪          │
│ · 审计日志（同步）        │     │ · 缓存体系                 │     │ · 审计日志 L2（Redis List）│
│ · AK/SK 骨架             │     │ · AK/SK 完整实现           │     │ · 事件驱动（Outbox+Asynq） │
│                         │     │ · JWT 升级 RS256           │     │ · 微服务拆分 / gRPC       │
│                         │     │ · 工单模块                 │     │ · PG Cluster + Redis HA  │
│                         │     │ · 文件存储（S3 兼容）       │     │ · 异地登录检测            │
│                         │     │ · 审计日志异步              │     │                          │
└─────────────────────────┘     └─────────────────────────┘     └─────────────────────────┘
   单实例 · Docker Compose          单实例 · Docker Compose          多实例 · Nginx 负载均衡
```

---

## Phase 1：最小可用

**核心目标**：为后续所有服务搭好认证鉴权框架。

| 模块 | 核心能力 | 文档 |
|------|---------|------|
| 基础设施 | DB 迁移、配置、Wire DI、优雅关闭、健康检查 | [phase1/01-infra.md](./phase1/01-infra.md) |
| 认证 | 登录、双 Token、RT 轮换、登出、黑名单、AK/SK 骨架 | [phase1/02-auth.md](./phase1/02-auth.md) |
| 鉴权 | 路由级 Casbin RBAC、ResourceRegistry 骨架 | [phase1/03-authz.md](./phase1/03-authz.md) |
| 用户 | CRUD、启用禁用、密码修改、角色绑定 | [phase1/04-user.md](./phase1/04-user.md) |
| 角色 | CRUD、菜单分配、Casbin 策略同步 | [phase1/05-role.md](./phase1/05-role.md) |
| 组织 | 树形 CRUD、ltree 路径、用户关联 | [phase1/06-organization.md](./phase1/06-organization.md) |
| 菜单 | CRUD、菜单树、权限码、前端数据 | [phase1/07-menu.md](./phase1/07-menu.md) |
| 审计日志 | 操作日志中间件、同步写入、应用日志规划 | [phase1/08-audit.md](./phase1/08-audit.md) |
| 中间件 | JWT、Casbin、CORS(gin-contrib)、Recovery、RequestID(gin-contrib)、AccessLogger(gin-contrib/slog)、安全头 | [phase1/09-middleware.md](./phase1/09-middleware.md) |
| 并发与事务 | DB 事务、SyncedEnforcer、Redis 原子操作、乐观锁 | [phase1/10-concurrency.md](./phase1/10-concurrency.md) |

**部署形态**：单实例 Docker Compose（PG + Redis + App）

**验收标准**：登录 → 获取菜单/权限 → CRUD 操作 → 刷新 Token → 登出 → 黑名单生效

---

## Phase 2：业务可用

**核心目标**：资源级鉴权、安全加固、缓存体系、第一个业务模块（工单）。

| 模块 | 核心能力 | 文档 |
|------|---------|------|
| 安全加固 | 多设备管理、登录限流/锁定、密码复杂度、密码重置 | phase2/01-auth-enhance.md（待编写） |
| 资源级鉴权 | ltree 组织关系查询、属主判断、每资源独立 Enforcer | phase2/02-authz-resource.md（待编写） |
| 组织增强 | 虚拟组、组织角色、组织级权限（scope） | phase2/03-org-enhance.md（待编写） |
| 缓存体系 | 权限缓存、菜单缓存、组织缓存、Cache-Aside + singleflight | phase2/04-cache.md（待编写） |
| 审计日志增强 | channel + batch 异步写入、日志过期清理 | phase2/05-audit-enhance.md（待编写） |
| 限流中间件 | Redis + 令牌桶/滑动窗口 | phase2/06-ratelimit.md（待编写） |
| AK/SK 管理 | 服务间认证完整实现、管理 API | phase2/07-m2m-aksk.md（待编写） |
| JWT 升级 | HS256 → RS256 + JWKS | phase2/08-jwt-rs256.md（待编写） |
| 工单模块 | 工单类型配置、状态机、权限模型 | phase2/09-ticket.md（待编写） |
| 文件存储 | S3 兼容对象存储、预签名 URL 直传 | phase2/10-storage.md（待编写） |

**部署形态**：单实例 Docker Compose

---

## Phase 3：生产加固

**核心目标**：可观测性、多实例部署、高可用。

| 模块 | 核心能力 | 文档 |
|------|---------|------|
| 可观测性 | Prometheus Metrics、Grafana、OpenTelemetry | phase3/01-observability.md（待编写） |
| 多实例部署 | Casbin Watcher、跨实例事件广播、分布式锁 | phase3/02-multi-instance.md（待编写） |
| 审计日志 L2 | Redis List 队列，进程崩溃不丢 | phase3/03-audit-l2.md（待编写） |
| 事件驱动 | PostgreSQL Outbox + Asynq | phase3/04-event-driven.md（待编写） |
| 微服务拆分 | gRPC 内部通信、服务拆分、API Gateway | phase3/05-microservice.md（待编写） |
| 高可用 | PG Cluster、Redis Sentinel、Nginx 负载均衡 | phase3/06-ha.md（待编写） |
| 安全增强 | 异地登录检测、验证码、密码过期 | phase3/07-security-enhance.md（待编写） |
| 运维工具 | Swagger CI、DB 迁移 CI、集成测试自动化 | phase3/08-ops.md（待编写） |

**部署形态**：多实例 + Nginx 负载均衡 + PG Cluster + Redis Sentinel

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
│   ├── implementation-plan.md  # 原实现计划（已被 phase1/ 取代）
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
│   └── (01~09 待编写)
├── phase3/                     # Phase 3 详细实现计划
│   ├── README.md               # 大纲 + 边界 + 实施顺序
│   └── (01~08 待编写)
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
