# Phase 3 实现计划：生产加固 + 工单业务能力闭环

> ⚠️ **暂缓声明（2026-08-25）**：当前没有微服务需求，**Phase 3 整体暂缓、不排期**。本文件保留为参考，与 [../roadmap.md](../roadmap.md) 主视图一致——优先级是把单体服务（Phase 1 认证鉴权 + Phase 2 工单业务能力）做扎实跑稳。生产加固类能力按需取用，不拆 3a/3b 子阶段执行。
>
> **核心目标（参考方向，非执行项）**：① 单体服务上的生产加固（可观测性、可运维性、数据安全）；② **工单业务能力闭环**——工单模板/关联已前移 Phase 2a（纯 DB，见 [phase2/09-ticket §0](../phase2/09-ticket.md)），**SLA 计时/违约告警、站内/邮件通知、多级审批流、自动分派、报表仍属 Phase 3 范围（暂缓）**（依赖 2c Authorize + 事件机制），不阻塞 Phase 2 主线。
>
> 创建日期：2026-08-12  
> 修订：2026-08-13 — 拆为 **Phase 3a**（先上生产）与 **Phase 3b**（按需拆服务/平台），避免一次做太多。  
> 修订：2026-08-25（上）— ① 工单业务能力（**SLA/通知/审批流/分派/报表**）拉入 **3a**（进程内实现，事件用 L1 机制）；模板/关联前移至 2a；② **微服务拆分整体推迟到 3b 以后按需**。  
> 修订：2026-08-25（晚）— **Phase 3 整体暂缓、不排期**；以单体服务为先，详见 [../roadmap.md](../roadmap.md)。

---

## 0. 子阶段总览（暂缓参考，非排期）

> 以下 3a/3b 划分仅作能力归类参考。按 2026-08-25 决策，Phase 3 **整体暂缓、不排期**；工单**主链路**（CRUD / 状态机 / 模板 / 关联）已在 Phase 2a/2b 实现，**SLA/通知/审批流/分派/报表仍属 Phase 3（暂缓，设计就绪）**，生产加固类能力按需取用。

| 子阶段 | 目标 | 典型交付 | 状态 |
|--------|------|----------|------|
| **3a** | 单/多实例可运维、可观测、可恢复 + **工单业务能力（主链路已在 2a/2b；SLA/通知/审批流/分派/报表 3a 设计就绪）** | Metrics、Watcher、审计 L2、HA、安全增强、ops CI + **SLA/通知/审批流/模板/关联/分派/报表** | **暂缓**（工单主链路已在 Phase 2a/2b 实现） |
| **3b** | 事件基础设施升级 + 平台增强（**微服务化推迟**） | Outbox+Asynq、权限/菜单缓存平台、AK/SK | **暂缓** |

**建议顺序（参考，非排期）**：若未来启动 Phase 3，建议先完成 **3a** 再启动 **3b**。**微服务拆分不做**（当前无需求），整体推迟到未来有真实多团队/M2M 需求时再评估。

### Phase 3 启动条件（暂缓不是搁置，是有触发条件的等待）

Phase 3 在以下任一条件出现时评估启动（不要求全部满足）：

| 触发信号 | 启动的子能力 | 含义 |
|----------|-------------|------|
| 单体性能瓶颈（QPS/延迟超阈值） | observability + multi-instance | 需要可观测性定位 + 多实例分流 |
| 真实多团队开发需求 | microservice（推迟项重启评估） | 多团队并行开发，单体协作成本高 |
| 多消费者/异步邮件需求 | event-driven L2 升级 | L1 单消费者瓶颈，需 Outbox + Asynq worker 多消费者 |
| 会签/网关/分支审批需求 | ticket-business §4 BranchedStateEngine | 线性状态机表达不下（见 [ticket.md §8.6](../modules/ticket.md#86-工作流引擎升级触发信号量化表)）。**注：引擎本体为 Phase 3 硬交付（[10-ticket-business.md §4.4](./10-ticket-business.md)），触发信号只决定何时加更多流程定义，不决定引擎是否实现** |
| SLA 合规要求 | ticket-business §2 SLA 计时 | 业务方要求响应/解决时限 + 违约告警 |
| 高可用要求（99.9%+） | ha（PG Cluster + Redis Sentinel） | 单实例不满足 SLO |
| 外部系统 M2M 调用 | platform AK/SK | 有机器到机器调用方 |

> **原则**：暂缓期间不提前实现；触发条件出现时按需启动对应子能力，不要求一次性全做 Phase 3。**例外**：BranchedStateEngine 引擎本体为 Phase 3 硬交付（见 [10-ticket-business.md §4.4](./10-ticket-business.md)），触发信号仅决定「何时加更多流程定义」，不决定「引擎是否实现」。

> **工单业务能力为何属 Phase 3**：工单是 [ticket.md §6](../modules/ticket.md#6-事件驱动集成概要) 列举的 5 类事件源，下游消费者（通知、SLA、满意度、告警）全部卡在事件机制。若 Phase 3 只交付基础设施、工单业务能力延后，则工单作为"入口"是空心的。入口必须在 Phase 3 闭合，下游才能挂上去。详见 [10-ticket-business.md](./10-ticket-business.md)。

> **微服务化为何推迟**：[deployment-evolution.md §4-§5](../proposal/deployment-evolution.md) 的微服务拆分依赖 event-driven 做 IAM 数据 CQRS 复制，Phase 3+ 完成事件基础设施前拆服务会引入同步调用风险。当前代码已按 [§2.3](../proposal/deployment-evolution.md#23-代码分层隔离为未来拆分准备) 领域目录隔离，部署级分离（同二进制不同配置）可在 Phase 3 末按需验证边界，真微服务化留待 Phase 3+ 以后。详见 [11-deployment-split.md](./11-deployment-split.md)。

---

## 1. Phase 3 边界

> ⚠️ **暂缓说明（2026-08-25）**：Phase 3 整体暂不排期。以下 3a/3b 划分仅作能力归类参考，优先级为先把单体服务（Phase 1 + Phase 2 工单主链路）做扎实。工单**主链路**（CRUD / 状态机 / 模板 / 关联）已在 Phase 2a/2b 实现；**SLA/通知/审批流/分派/报表仍属 Phase 3（暂缓，设计就绪）**，生产加固类能力按需取用。

### 1.1 Phase 3a — 做什么（暂缓参考，不拆 3a/3b 执行）

> ⚠️ **暂缓期间不区分 3a/3b**：以下 3a/3b 分类仅作能力归类参考，不作为执行子阶段。Phase 3 启动时按需取用，不强制按 3a→3b 顺序。

| 类别 | 模块 | 核心能力 | 文档 |
|------|------|---------|------|
| 可观测性 | [observability](./01-observability.md) | 应用内 **可选开关**（Metrics / OTel / pprof）；Prometheus / Grafana / Collector **部署可选** | 已编写 |
| 多实例部署 | [multi-instance](./02-multi-instance.md) | Casbin Watcher、跨实例事件、分布式锁 | 待编写 |
| 审计日志升级 | [audit-l2](./03-audit-l2.md) | Redis List L2，进程崩溃不丢日志 | 待编写 |
| 高可用 | [ha](./06-ha.md) | PG Cluster、Redis Sentinel、Nginx | 待编写 |
| 安全增强 | [security-enhance](./07-security-enhance.md) | 异地登录、验证码、密码过期、API 限流 | 待编写 |
| 运维工具 | [ops](./08-ops.md) | Swagger CI、迁移 CI、集成测试自动化 | 待编写 |
| **工单业务能力** | [ticket-business](./10-ticket-business.md) | **SLA 计时/违约告警、站内通知、邮件通知、多级审批流（手写 BranchedStateEngine）、自动分派规则、工单报表**（进程内实现，事件用 L1 机制） | 本修订新增 |

> **工单业务能力实现方式**：Phase 3 不依赖 L2 Outbox，采用 [ticket.md §6](../modules/ticket.md#6-事件驱动集成概要) 定义的三档事件机制中的 **L1**（DB 持久化 + 轮询补偿 + 分布式锁，长期稳态见 ADR-001）+ Asynq（ADR-002），保证进程崩溃不丢、多实例不重复消费。L2 升级时业务逻辑不变，只换调度器。

### 1.2 Phase 3b — 做什么（暂缓参考，不拆 3a/3b 执行）

| 类别 | 模块 | 核心能力 | 文档 |
|------|------|---------|------|
| 事件驱动升级 | [ADR-001](./adr/ADR-001-event-mechanism-l1-steady-state.md) / [ADR-002](./adr/ADR-002-asynq-async-task-executor.md) | L1 → L2 升级：PostgreSQL Outbox + Asynq 可靠事件分发 + 异步任务队列（方案见 ADR-001/002，原 04-event-driven.md 未单独成篇） | 已决策（ADR） |
| 平台增强 | [platform](./09-platform.md) | 权限/菜单缓存跨实例失效、AK/SK（有调用方时） | 待编写 |

> **微服务拆分（microservice）当前不做**：[05-microservice.md](./05-microservice.md) 保留为参考文档。按 2026-08-25 决策，当前没有微服务需求，**整体推迟到未来有真实多团队/M2M 需求时再评估**，不排入 Phase 3。部署级分离（同二进制不同配置启动）作为后续验证手段，见 [11-deployment-split.md](./11-deployment-split.md)。

### 1.3 不做什么

| 不做 | 原因 | 阶段 |
|------|------|------|
| 多租户 | 预留字段即可，按需启用 | 按需 |
| 第三方登录（OAuth/SSO） | 预留字段，按需启用 | 按需 |
| Kafka/RabbitMQ | Redis List + Asynq 足够 | 按需 |
| K8s 部署 | Phase 3 先用 Docker Compose + Nginx，K8s 后续 | 按需 |
| **微服务拆分（gRPC/CQRS/独立仓）** | 依赖 event-driven 基础设施；代码已领域隔离，部署级分离可验证边界 | **不做（无需求，未来按需）** |

### 1.4 前置条件

**Phase 3** 启动前，**Phase 2b** 必须已完成（**2c 不阻塞 Phase 3 的基础设施部分**——组织委托可与生产加固并行；但**工单业务能力（[10-ticket-business.md](./10-ticket-business.md)）依赖 2c 的 Authorize 升级**，建议 2c 与 Phase 3 并行推进或前置完成）：

- [ ] 2a 验收：TicketResource + **assigned** 范围
- [ ] 2b 验收：虚拟组 / scope / HR Sync、工单附件、auth-enhance
- [ ] 2c 验收：**工单业务能力硬前置**（D7–D9 的 org admin/owner + ancestor owner Authorize 是审批流/分派规则依赖项）；建议在 Phase 3 工单业务能力启动前完成

### 1.5 可观测性：应用可选、部署可选

> 与 [design-decisions §18](../design/design-decisions.md#18-部署与代码解耦一套代码多种部署) 一致：**App 不因未安装 Prometheus/Grafana/OTel Collector 而无法启动**。

| 层级 | 是否必须 | 说明 |
|------|----------|------|
| Phase 1–2 | 不要求 | `/health/*` + slog + `request_id` 即可 |
| 应用内埋点 | 有能力、**配置开关** | `observability.metrics/tracing/pprof.enabled` |
| Prometheus | **部署可选** | 采集端；无它 App 正常运行 |
| Grafana | **部署可选** | 纯展示，永远不是 App 依赖 |
| OTel Collector | **部署可选** | dev 可用 `noop` / `stdout` |

Docker Compose 建议用 **profile**（如 `observability`）拉起 Prometheus/Grafana/Collector；默认 `docker compose up` 可不包含。

详见 [01-observability.md](./01-observability.md)。

---

## 2. 实施顺序（暂缓参考）

> 以下顺序仅作能力归类参考，Phase 3 整体未排期。工单**主链路**（CRUD / 状态机 / 模板 / 关联，Step 7 主体）已在 Phase 2a/2b 实现；**SLA/通知/审批流/分派/报表（Step 7 子任务 7a–7e）仍属 Phase 3（暂缓，设计就绪）**；其余步骤按需取用。

### 2.1 Phase 3（先上生产 + 工单业务能力闭环）— 暂缓参考

```
Phase 2b 验收通过（2c 建议并行或前置完成）
   │
   ├── Step 1: observability
   ├── Step 2: multi-instance → Step 3: audit-l2
   ├── Step 4: ha
   ├── Step 5: security-enhance（可与 Phase 3 并行）
   ├── Step 6: ops
   ├── Step 7: ticket-business（工单业务能力，依赖 Phase 2c Authorize）
   │     ├─ 7a: SLA 计时 + 违约告警
   │     ├─ 7b: 站内通知 + 邮件通知
   │     ├─ 7c: BranchedStateEngine + 多级审批流（变更类工单）
   │     ├─ 7d: 自动分派规则
   │     └─ 7e: 工单报表
   └── Step 8: Phase 3 生产验收（含工单业务能力验收）

> **工单模板/关联已前移到 Phase 2a**（2026-08-25）：`ticket_templates`（迁移 000015）和 `ticket_relations`（迁移 000016）因纯 DB 无事件依赖前移到 2a Step 2，见 [phase2/09-ticket.md §2](../phase2/09-ticket.md)。Step 7 子任务相应精简。
```

> **Step 7 与基础设施 Step 1–6 可并行**：工单业务能力的事件机制用 L1（DB 持久化 + 轮询）；**L1 事件消费**在多实例部署后需复用 Step 2 的分布式锁防重，但 **SLA 定时扫描由 Asynq Scheduler 单点调度，无需自写锁**（[ADR-002](../adr/ADR-002-asynq-async-task-executor.md)）。建议 Step 7 在 Step 2 之后启动，或接受单实例先跑通。

### 2.2 Phase 3（事件基础设施升级 + 平台增强）— 暂缓参考

```
Phase 3 主能力稳定运行
   │
   ├── Step 9: event-driven（L1 → L2 升级：Outbox + Asynq）
   ├── Step 10: platform（权限/菜单缓存、AK/SK）
   └── Step 11: 验收
```

> **微服务拆分不在本阶段（且不排入 Phase 3）**：[05-microservice.md](./05-microservice.md) 标注推迟。当前无微服务需求，整体推迟到未来有真实多团队/M2M 需求时再评估。

### 2.3 步骤对照表

| Step | 能力归类（暂缓参考） | 模块 | 依赖 | 文档 |
|------|--------|------|------|------|
| 1 | 原 3a | observability | Phase 2 | [01-observability.md](./01-observability.md) |
| 2 | 原 3a | multi-instance | Step 1 | [02-multi-instance.md](./02-multi-instance.md) |
| 3 | 原 3a | audit-l2 | Step 2 | [03-audit-l2.md](./03-audit-l2.md) |
| 4 | 原 3a | ha | Step 2 | [06-ha.md](./06-ha.md) |
| 5 | 原 3a | security-enhance | Phase 2 | [07-security-enhance.md](./07-security-enhance.md) |
| 6 | 原 3a | ops | Phase 2 | [08-ops.md](./08-ops.md) |
| 7 | 原 3a | **ticket-business** | Phase 2c + Step 2 | [10-ticket-business.md](./10-ticket-business.md) |
| 8 | 原 3a | 生产验收 | Step 1–7 | 本文档 §3.1 |
| 9 | 原 3b | event-driven | Phase 3 稳定 | [ADR-001](./adr/ADR-001-event-mechanism-l1-steady-state.md) / [ADR-002](./adr/ADR-002-asynq-async-task-executor.md) |
| 10 | 原 3b | platform | 按需 | [09-platform.md](./09-platform.md) |
| 11 | 原 3b | 验收 | Step 9–10 | 本文档 §3.2 |
| — | 推迟 | ~~microservice~~ | Phase 3 以后按需 | [05-microservice.md](./05-microservice.md)（参考文档，标注推迟） |
| — | 推迟 | ~~deployment-split~~（部署级分离） | Phase 3 末可选验证 | [11-deployment-split.md](./11-deployment-split.md) |

---

## 3. 生产验收标准

### 3.1 Phase 3 验收（两档）

按部署场景选档；**不得**把 Grafana/Prometheus 未部署视为 App 启动失败。

#### Phase 3-min（单实例、内网、低 SLA）

| 维度 | 指标 |
|------|------|
| 可用性 | 单实例可恢复；live/ready 正常 |
| 可观测性 | 结构化 slog + `request_id`；`observability.*.enabled=false` 时零额外开销 |
| 安全性 | Phase 1 限流/锁定 + HTTPS（有域名时） |
| 数据安全 | PG 定期备份 |
| 运维 | 一键部署、DB 迁移可脚本化 |
| **工单业务** | SLA 计时/告警、站内通知、邮件通知、多级审批流、模板/关联/分派/报表全部可用（[10-ticket-business.md](./10-ticket-business.md) 验收用例全过） |

#### Phase 3-full（多实例或需 SLO / 对外 SLA）

在 **Phase 3-min** 基础上：

| 维度 | 指标 |
|------|------|
| 可用性 | 多实例 99.9% |
| 可观测性 | 开启 Metrics（QPS/延迟/错误率）+ 分布式追踪；Grafana 大盘 **可选** |
| 多实例 | Casbin Watcher、缓存跨实例失效、L1 事件消费分布式锁防重（SLA 扫描由 Asynq 单点调度，无需自写锁） |
| 审计 | Redis List L2（进程崩溃不丢） |
| 安全性 | 密码策略、异地登录、API 限流等（见 security-enhance） |
| 运维 | Swagger CI、集成测试自动化 |

### 3.2 Phase 3+ 验收（按需）

| 维度 | 指标 |
|------|------|
| 事件驱动 | L1 → L2 升级完成；Outbox 表 + Asynq worker 运行；工单事件可靠分发，进程崩溃不丢 |
| 平台 | 权限缓存跨实例失效；有 M2M 时 AK/SK 可用 |
| 工单回归 | 事件机制升级后，SLA/通知/审批流用例全量回归通过（业务逻辑不变，只换调度器） |

---

## 4. 待决策点

| 事项 | 说明 | 状态 |
|------|------|------|
| ⚠️ K8s vs Docker Compose | Phase 3 是否上 K8s？ | 建议 Docker Compose + Nginx 足够，K8s 按需 |
| ~~⚠️ 微服务拆分粒度~~ | ~~先拆哪个服务？~~ | **推迟**：microservice 整体移出 Phase 3+，待 Phase 3 后按需评估 |
| ⚠️ gRPC vs HTTP | 内部通信协议 | 架构文档已决策：gRPC 内部 + REST 外部（推迟启用） |
| ⚠️ Redis 高可用方案 | Sentinel vs Cluster | 建议 Sentinel（简单），Cluster 按需 |
| ⚠️ PG 高可用方案 | 自建 vs 云托管 | 已决策：云托管 Cluster（2+VIP） |
| ⚠️ 部署级分离时机 | Phase 3 末是否验证部署级分离（同二进制不同配置）？ | 见 [11-deployment-split.md](./11-deployment-split.md)；有独立扩缩需求时验证 |
| ⚠️ 工单审批流引擎选型 | Phase 3 手写 BranchedStateEngine；未来是否引第三方引擎？ | 见 [ticket.md §8.4/§8.5](../modules/ticket.md#84-为什么-phase-2a-不引入工作流引擎)；Phase 3 首选手写 |
| ✅ 可观测性栈 | Prometheus/Grafana/OTel | **应用内可选开关 + 部署可选**；Phase 3-min 不要求全套栈 |
| ✅ 工单事件机制 | Phase 3 用 L1（DB 持久化+轮询+分布式锁），Phase 3+ 升级 L2（Outbox+Asynq） | 已决策：见 [ticket.md §6](../modules/ticket.md#6-事件驱动集成概要) |
| ⚠️ 工单组织可见性数据结构 | 现状：`tickets.org_path` 为 move 级联维护的镜像列，存在 Create×Move 并发写旧快照竞态（BK-11，fail-safe）与 P2-D1/MC2 级联维护负担 | **Phase 3 启动时拍板**：保留镜像列（+FOR SHARE）或去列改运行时 JOIN（organizations 单一真相源）；若拍板去列，「按创建时归属」的报表诉求改用 write-once `created_org_id` 承载。过渡期 FOR SHARE 修复已兜底（BK-11 ①） |

---

## 5. 文档索引

> 标注"待编写"的文档尚需创建，当前先占位。

| 文档 | 模块 | 状态 |
|------|------|------|
| [01-observability.md](./01-observability.md) | 可观测性 | 已编写 |
| [02-multi-instance.md](./02-multi-instance.md) | 多实例部署 | 待编写 |
| [03-audit-l2.md](./03-audit-l2.md) | 审计日志 L2 | 待编写 |
| [ADR-001](./adr/ADR-001-event-mechanism-l1-steady-state.md) / [ADR-002](./adr/ADR-002-asynq-async-task-executor.md) | 事件驱动（Phase 3+，L1→L2 升级） | 已决策（ADR，原 04-event-driven.md 未单独成篇） |
| [05-microservice.md](./05-microservice.md) | 微服务拆分 | **推迟**（Phase 3+ 以后按需；当前作为参考文档） |
| [06-ha.md](./06-ha.md) | 高可用 | 待编写 |
| [07-security-enhance.md](./07-security-enhance.md) | 安全增强 | 待编写 |
| [08-ops.md](./08-ops.md) | 运维工具 | 待编写 |
| [09-platform.md](./09-platform.md) | 平台增强（Phase 3+） | 待编写 |
| [10-ticket-business.md](./10-ticket-business.md) | **工单业务能力（Phase 3）** | **本修订新增，待编写** |
| [11-deployment-split.md](./11-deployment-split.md) | **部署级分离方案** | **本修订新增，待编写** |
