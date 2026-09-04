# 实施路线图（Roadmap）

> 三阶段实施总览，每阶段的核心目标、模块清单和能力边界。
>
> 创建日期：2026-08-12

---

## 总览

```
Phase 1：最小可用                    Phase 2：业务可用（工单）           单体服务优先（Phase 3 暂缓）
┌─────────────────────────┐     ┌─────────────────────────┐     ┌─────────────────────────┐
│ 认证鉴权框架             │     │ 资源级鉴权 + 工单        │     │ 生产加固类能力按需取用    │
│ · 登录 / 双 Token        │────▶│ · 资源级鉴权（ltree）    │────▶│ · 可观测性（Metrics 等） │
│ · 登录限流 / 会话吊销    │     │ · 虚拟组 / scope         │     │ · 安全增强（按需）       │
│ · 路由级 RBAC            │     │ · 对象存储 + 工单        │     │ · 运维工具（按需）       │
│ · 用户/角色/菜单/组织    │     │ · 多设备 UI / 密码策略   │     │ · 单体多副本（按需升档） │
│ · 同步审计               │     │ · 工单业务能力（设计就绪，Phase 3 实现） │     │ ⛔ 微服务拆分（无需求）  │
└─────────────────────────┘     └─────────────────────────┘     └─────────────────────────┘
   单实例 Docker Compose            单实例 Docker Compose            单实例 / 单体多副本（按需）
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

**Step 7–9 CRUD 补全**：角色/菜单/组织写 API 属于 Phase 1 交付（见 [phase1/README §2.4](./phase1/README.md#24-step-79-crud-补全计划)）。组织 CRUD 是 Phase 2b 的前置依赖；角色/菜单 CRUD 主路径可依赖种子数据，但管理端完整能力需在 Phase 1 收尾完成。

---

## Phase 2：业务可用

**核心目标**：资源级鉴权 + 工单。仍为模块化单体，不拆 IAM。

**子阶段**：**2a**（Registry + 工单 MVP）→ **2b**（组织 scope/虚拟组/HR + 附件）→ **2c**（组织内委托：owner + 组内 admin/member）。详见 [phase2/README.md](./phase2/README.md) §0。

| 子阶段 | 模块 | 核心能力 | 文档 |
|--------|------|---------|------|
| **2a** | 资源级鉴权 + 工单 MVP | Registry、属主、assigned 过滤、工单 CRUD+状态机 | [phase2/02-authz-resource.md](./phase2/02-authz-resource.md)、[09-ticket.md](./phase2/09-ticket.md) |
| **2b-core** | 工单可见性本体 | 策略 B 透明读 + `ticket_visibility`（✅ Step 4 已交付） | [09-ticket §2b](./phase2/09-ticket.md) |
| **2b-org** | 组织增强 | 虚拟组/scope、BFS 三源、临时成员（✅ Step 5 已交付）；HR 目录同步延后 | [phase2/03](./phase2/03-org-enhance.md)、[09-ticket §2b](./phase2/09-ticket.md) |
| **2b-ext** | 外延（延后/按需） | 附件、多设备 UI、密码策略、HR 目录同步 | [10-storage](./phase2/10-storage.md)、[01-auth-enhance](./phase2/01-auth-enhance.md) |
| **2c** | 组织内委托 | owner、org_member_role、组内防提权、工单 Authorize | [phase2/04-org-delegation.md](./phase2/04-org-delegation.md) |

**部署形态**：单实例 Docker Compose

**明确后移（暂缓，无近期计划）**：RS256、AK/SK、缓存平台、审计异步、每资源 Enforcer、微服务拆分（gRPC/IAM 独立/CQRS）→ 未来有真实需求时再评估，当前聚焦单体服务。

---

## Phase 3：生产加固（暂缓，先做好单体）

> **决策（2026-08-25）**：当前没有微服务需求，**Phase 3 整体暂缓**，暂不排期。
> 优先把单体服务（Phase 1 认证鉴权 + Phase 2 工单业务能力）做扎实、跑稳，再视真实需求决定是否启动 Phase 3。
> 微服务拆分（gRPC / IAM 独立 / CQRS 等）不在近期计划内，详见 [phase3/11-deployment-split.md](./phase3/11-deployment-split.md) 档位 1（单体多副本）为默认形态。

**核心目标（暂缓期间的方向，非执行项）**：单体服务上的可观测性、可运维性、数据安全。

| 模块 | 核心能力 | 状态 |
|------|---------|------|
| 可观测性 | Prometheus Metrics、Grafana、OpenTelemetry | 暂缓（[phase3/01-observability.md](./phase3/01-observability.md) 已编写，按需取用） |
| 多实例部署 | Casbin Watcher、跨实例事件广播、分布式锁 | 暂缓（单体多副本按需，非必须） |
| 审计日志 L2 | Redis List 队列，进程崩溃不丢 | 暂缓 |
| 事件驱动（L1 事件源） | PostgreSQL `ticket_events` 轮询（长期稳态） | 设计就绪（[ADR-001](./adr/ADR-001-event-mechanism-l1-steady-state.md)）；`ticket_events` 表 Phase 2a 已建（审计用），L1 机制（event_type/processed 列 + 轮询消费者 + 分布式锁）Phase 3 启动时实现 |
| 异步任务执行器 | Asynq（复用现有 Redis，覆盖审批触发事件 + 预置定时任务） | 设计就绪（[ADR-002](./adr/ADR-002-asynq-async-task-executor.md)）；Phase 3 启动时引入（Phase 2a 无异步业务） |
| 消息通知中心 | 站内通知 + 邮件 SMTP，由 Asynq worker 异步发送（订阅 `ticket_events` 各事件；也是审批/SLA 违约的下游副作用） | 设计就绪（[ADR-002 场景 D](../adr/ADR-002-asynq-async-task-executor.md) + [10-ticket-business §3](../phase3/10-ticket-business.md#3-通知服务站内--邮件)；用户明确后续基于 Asynq 集成，实现时机待定） |
| 事件驱动（L2 升级） | PostgreSQL Outbox + Asynq worker 多消费者 | 暂缓 |
| 微服务拆分 | gRPC、IAM 独立、API Gateway、RS256+JWKS | **不做**（无需求，推迟到未来按需） |
| 高可用 | PG Cluster、Redis Sentinel、Nginx 负载均衡 | 暂缓（单实例 Docker Compose 先用） |
| 安全增强 | 异地登录检测、验证码、密码过期、API 限流 | 暂缓（部分可在 Phase 2 顺带做） |
| 平台增强 | 缓存体系、AK/SK（有调用方时） | 暂缓（无 M2M 调用方） |
| 运维工具 | Swagger CI、DB 迁移 CI、集成测试自动化 | 部分可在 Phase 2 顺带做 |

**部署形态（当前默认）**：单实例 Docker Compose（PG + Redis + App）。有高可用需求时直接升档位 1（单体多副本 + Nginx），不引入服务拆分。

---

## 外部能力集成：activelist（独立服务，已定 ADR-003）+ taskrunner（M-E）

> 详见 [ADR-003-activelist-integration-form](./adr/ADR-003-activelist-integration-form.md)（镜像；**SSOT = activelist 仓库 `docs/ADR-003-integration-contract.md`**）；taskrunner 设计 SSOT = taskrunner 仓库 `docs/taskrunner.md`。**zhuzhao 侧配套细排见 [phase3/16-external-integration](./phase3/16-external-integration.md)**。

- **activelist 形态（2026-09-03 收敛 + M-A 重定位）**：activelist = **独立仓库独立实现的动态数据模型薄层**（`github.com/tracerbiubiubiu/activelist`；~~C2' 同仓库 `internal/activelist/` 包~~ 已被取代）——类型注册 / Schema 演进（方案 D：单一当前版本）/ 动态字段校验 / 数据 CRUD / 存储（PG **独立数据库**、每类型表 + `data` JSONB、id 自增、乐观锁、软删保留、导入幂等=**全量替换**）；**零认证**，仅经 zhuzhao 网关对外（反代 + 身份断言 = **明文 X-Operator 入 AK/SK 签名覆盖**，2026-09-03 拍板；design-decisions §25.2 修订注）；进程 3→1（仅 apiserver）；与其他里程碑**无链式依赖**。
- **taskrunner 形态（2026-09-03 定稿，M-E）**：事件/任务总线——独立仓库 + 独立部署 + 独立 Redis + 独立 DB；zhuzhao 作网关（发起/下发/按需查），预置动作 = 回调 zhuzhao 内网端点（业务 handler 在 zhuzhao）；taskrunner 自维护 `job_runs`，`request_id` 关联跨查。
- **事件（2026-09-03 收敛）**：~~activelist 变更事件落 L1 `ticket_events`~~ **事件归 zhuzhao**——zhuzhao 调 activelist 成功后在业务操作点**显式发布**（M-E taskrunner 形态承接）；activelist 不感知事件、不再实现 Change Stream。
- **审计（2026-09-03 收敛）**：~~两层审计（activelist 业务层自脱敏 accesslog）~~ **审计归 zhuzhao**——activelist 写接口返回变更后完整文档，zhuzhao 侧记录（落点机制 ⚠️ 待拍板）；activelist 只记技术/运行日志；`X-Request-ID` 贯穿两层。
- **共享 utils**：✅ **已完成（zhuzhao-utils v0.1.0，2026-09-03）**——9 包（logger/postgres/errcode 等）已抽取并 pin；resource 按 §25.3 拍板留 zhuzhao 不抽；**新增规划：`aksk` 包**（服务间 HMAC 签名，2026-09-03 拍板，16 号 §9 基线）。
- **排期（2026-09-02 §23.2）**：**M-A（activelist 独立实现）**为 Phase 3 现行主链一段（M0 → M-E → **M-A** → M-HR → M-SSO🚦 → M-Mig，见 [phase3/13](./phase3/13-implementation-plan.md)）；~~「Phase 3 启动后 L1+Asynq 就绪」依赖口径已废~~——M-A 无链式依赖，前置 = ~~共享 utils~~ ✅ + **批次 B 网关化**（§25.5：反代核心/身份断言/限流/menu_apis/审计跳 body，~1 周）。**HR 目录同步已升主链 M-HR**（~~属 Phase 2b~~，2026-09-02）。
- **与"微服务拆分"决策的关系**：activelist / taskrunner 均为**外部引入的独立能力模块**（独立仓库/库/数据库），不是把 zhuzhao 内部拆出去；与"无微服务拆分需求"不冲突，单列本条。
- **zhuzhao 侧待办**：批次 B 网关化（= 原 E13 泛化：`app/service/proxy/` + `SetForwardHeaders` + Restrict 资源 `activelist` + accesslog 跳过 body）+ D3 审计实现（✅ 落点已拍板，SSOT=ADR-003「审计落点机制」专节：client 封装层 + `activelist_audit_log` 表；D1–D5 对照见 [16 号](./phase3/16-external-integration.md)）。
- **启用条件**：对接数据（如封禁 IP 列表、动态活动列表）需要经 zhuzhao 统一鉴权对外暴露时（事件订阅已移交 zhuzhao 侧显式发布）。

**子阶段**：Phase 3 未排期；工单**主链路**（CRUD / 状态机）已在 Phase 2a 实现，**工单模板、工单关联已前移 Phase 2a（迁移 000015/000016）；SLA/通知/审批流/分派/报表仍属 Phase 3（暂缓，设计就绪）**（见 [phase2/README.md](./phase2/README.md)），生产加固类能力按需取用 phase3 文档，**不拆 3a/3b**。

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
│   └── 01-auth-enhance ~ 04-org-delegation + 09-ticket + 10-storage（Phase 2 全套已编写）
├── phase3/                     # 生产加固能力（暂缓，按需取用）
│   ├── README.md               # 大纲 + 边界 + 实施顺序（暂缓说明）
│   ├── 01-observability.md     # 已编写
│   ├── 10-ticket-business.md   # 已编写（工单业务能力闭环 SSOT）
│   ├── 11-deployment-split.md  # 已编写（部署级分离方案）
│   └── 02–09 待编写（按需启用，未排期）
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
