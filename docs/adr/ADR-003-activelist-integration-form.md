# ADR-003: activelist 集成形态 —— 独立服务 + 统一 PG + 事件总线对接（非事务耦合）

## 日期
2026-08-25

## 状态
已采纳

## 背景
`doc/soar/activelist.md` 设计了一个基于 MongoDB + Go 的动态多类型数据全生命周期管理平台（高可靠/高可用要求）。需要决策：它与 zhuzhao 的关系（独立 vs 内化）、数据库选型（Mongo vs PG）、以及是否需要跨事务一致。

经两轮验证澄清了真实业务场景：
- **场景1（事件广播）**：activelist 数据变更（如封禁 IP 列表新增一个 IP）→ 立马触发一个**预定义事件**（可被工单或其他模块订阅触发）。这是数据变更→发事件→订阅方响应的**事件驱动**模型，activelist 设计自身即"最终一致 / At-Least-Once + 幂等"，不要求与订阅方同事务。
- **场景2（多源摄取）**：工单数据源多样，除手动创建外，还有一部分数据由其他模块获取/产生。工单持有对各源的引用/快照，各源独立写、工单独立建，属**松一致**的 ingress 适配问题，不是与某数据源锁同一事务。

这两场景都不需要 activelist 与 zhuzhao 之间的跨事务一致。

## 决策
- **集成形态：activelist 作为独立服务**（不内化进 zhuzhao 单体，不做 C3 进程内合并）。保留部署边界 = 保留故障隔离；其高 SLA 负担（watcher HA、oplog/Resume Token 续传、节假日预案）不抬高 zhuzhao 单体关键性。
- **数据库：Mongo 迁移到 PostgreSQL（统一技术栈）**。理由见下"PG 优于 Mongo 的额外收益"。
- **耦合方式：事件总线对接，非事务耦合**。activelist 的变更事件**经 zhuzhao 网关 HTTP 事件摄入端点（带 `X-Operator` 鉴权）写入 zhuzhao 的 L1 `ticket_events`（事件事实源，ADR-001 既定）**，由 zhuzhao 侧消费者/Asynq worker 处理分发、**工单模块及其他模块订阅**；不要求跨服务/跨库事务，**事件表的数据库所有权始终在 zhuzhao 侧（activelist 不直接写 zhuzhao 库，符合 C2' 隔离）**。此处"事件总线"指 zhuzhao 的 **L1 事件源 + Asynq 执行器**组合，与 ADR-001/ADR-002 完全一致：**L1 是事件源（持久化/可重放），Asynq 是异步任务执行器，Asynq 不当事件总线**。
- **鉴权边界不变**：zhuzhao 网关统一 JWT/Casbin/Restrict 鉴权，内网信任透传 `X-Operator`；activelist 自身不做权限检查（见 `doc/soar/activelist.md` §19.2）。
- **网络隔离不变**：apiserver 跨 `activelist_internal` + `zhuzhao_to_activelist` 双 network，仅 zhuzhao 容器可达（§18.2）。

### PG 优于 Mongo 的额外收益（转 PG 的加分项）
除"少运维一套数据库、与 zhuzhao 技术栈同源（JSONB/ltree/Outbox/Casbin-pgx）"外，转 PG 还有一项被忽视的硬优势：
- **事务能力 PG 远强于 Mongo**。PG 提供成熟的 MVCC + 多语句事务 + 保存点 + 外键级联 + 一致性快照读；Mongo 的"多文档事务"是 4.0 后才加、需副本集、有 16MB/60s 限制、且默认仍是单文档原子。尽管当前两个场景不要求跨服务事务，但 activelist 落到 PG 后，**其内部**的"写主数据 + 写历史快照 + 落事件"可用 PG 单机事务保证原子（Mongo 下只能靠异步队列最终一致）。即：统一 PG 不仅统一栈，还让 activelist 自身的可靠性底座更稳。

### Mongo vs PG 能力对比（设计阶段验证，不计成本）
| 能力（activelist 需求） | Mongo | PG | 结论 |
|------|------|------|------|
| 动态类型/字段（无重启扩展） | 文档模型天然 | JSONB / 运行时 CREATE TABLE | 持平 |
| 物理隔离（每类型独立集合） | 天然 | 分区表 / 每类型表 | Mongo 更自然 |
| 动态 JSON Schema 校验 | 应用层 | 应用层 / CHECK | 持平 |
| 事件捕获（断点续传） | Change Stream 开箱 | 逻辑复制 / 复用 zhuzhao Outbox | Mongo 开箱；PG 复用现有范式可行 |
| 乐观锁 | version 字段 | `UPDATE..WHERE version=` | 持平 |
| 历史快照审计 | `_history` 集合 | `_history` 表+分区 | 持平 |
| 高可用 | 副本集 | 流复制/PG Cluster | 持平 |
| **事务能力（内部原子）** | 多文档事务受限 | **完整 ACID 事务** | **PG 显著更优** |

## 理由
- 两真实场景均为事件驱动 / 松一致，跨事务一致无需求，内化（C3）不成立。
- 独立服务保留故障隔离，符合"单体保持简单、无微服务拆分需求"的总体决策（activelist 是外部引入能力模块，非拆分 zhuzhao 内部）。
- 转 PG 统一技术栈、复用 zhuzhao 已有 JSONB/ltree/Outbox/Asynq 积木，且 PG 事务能力更强，提升 activelist 自身可靠性底座。
- 事件总线对接契合 zhuzhao 已设计的 L1/L2 事件机制（ADR-001/ADR-002）：activelist 变更事件经 zhuzhao 网关事件摄入端点写入 L1 `ticket_events`（事件源），L1 消费者/Asynq worker 负责执行分发（Asynq 仅执行器，不当总线）。**activelist 与 zhuzhao 各自运行独立的 Asynq + Redis 实例，不共享任务执行器后端**（避免跨服务基础设施耦合）；事件事实始终以 zhuzhao 的 L1 表为源、不依赖 Redis 持久化（与 ADR-001 红线一致）。

## 后果
- 正面：技术栈统一（去 Mongo）、故障隔离保留、事件机制复用、PG 事务底座更稳、activelist 事件可被工单/其他模块统一订阅。
- 负面/风险：activelist 事件捕获从 Change Stream 改为 Outbox/逻辑复制，需改造（但复用 zhuzhao Outbox 范式增量可控）；独立服务仍需 E13 反向代理模块 + `SetForwardHeaders` 中间件。
- 待办（见下"待办"）：① 事件桥接缺口——activelist 变更事件如何汇入 zhuzhao 统一事件目录；② 工单多源 ingress——zhuzhao 工单模块需设计多种数据源适配器（activelist 仅其一）。

## 审计日志分工（已确认，两层）
- **不能只在 zhuzhao 记录 activelist 调用日志**，采用 `doc/soar/activelist.md` §19.7 的两层审计：
  - **网关层（zhuzhao accesslog）**：记录谁/何时/访问了什么 API（method/path/actor/status/cost），proxy 路由**跳过 body**（防动态字段明文泄露到 zhuzhao 日志库）。
  - **业务层（activelist accesslog）**：记录谁对什么数据做了什么（含请求体 + 变更前后快照），由 activelist 按 Schema 标记**自行精确脱敏**（zhuzhao 不认识动态字段语义，黑名单命中不了手机号/身份证/薪酬）。
- 日志**基础设施复用 zhuzhao 的 `pkg/log/zap/logger`、`pkg/trace`、accesslog 核心**（C2' 同仓库直接 import/拷贝），但两进程写**各自日志目的地**（zhuzhao 审计库 / activelist 自身 log 集合）。

## 待办
- **E13（zhuzhao 侧）**：反向代理模块 `app/service/proxy/` + `SetForwardHeaders` 中间件 + Restrict 资源 `activelist` + accesslog 对 `/api/v1/data/*` 跳过 body（仅记 HTTP 元信息）。
- **G1（activelist 侧，转 PG）**：Mongo → PG 迁移设计（动态集合→分区表/每类型表；Change Stream→Outbox/逻辑复制；历史快照落 PG 表）。**含日志 writer 迁移**：§19.7.1 的 `mongo_writer.go` / `NewMongoWriteSyncer` 需改为 PG writer，否则转 PG 后日志仍依赖 Mongo。
- **G2（集成缺口）**：activelist 变更事件 → zhuzhao 统一事件目录（L1 事件源）的桥接设计：明确为 **activelist → zhuzhao 网关 HTTP 事件摄入端点（`X-Operator` 鉴权）→ zhuzhao 自写 `ticket_events`**，数据库所有权留在 zhuzhao；activelist 与 zhuzhao 各自独立 Asynq/Redis，不共享执行器后端。
- **G3（zhuzhao 工单侧）**：多源 ingress 适配器设计（手动 + activelist + 其他模块）。
- **G4（审计层，已确认）**：两层审计落地——zhuzhao 网关层（E13 跳过 body）+ activelist 业务层（自脱敏 accesslog）。

## 建议阶段
- **Phase 3 启动后**（L1 事件机制 + Asynq 就绪后）。原拟 Phase 2b，因前置依赖 L1/Asynq 实际于 Phase 3 启动时落地，已决策顺延至 Phase 3 启动后——避免为 activelist 单独写一套临时事件分发再迁移回 L1（违反 ADR-001「不偷工减料」原则）。**HR 目录同步不依赖 L1，仍属 Phase 2b**（desired-state reconciliation 直接写表，非事件消费者）。前置依赖：L1 事件机制 + Asynq 执行器就绪（Phase 3 启动时实现，见 [ADR-001](./ADR-001-event-mechanism-l1-steady-state.md)/[ADR-002](./ADR-002-asynq-async-task-executor.md)）、网关反代模块（E13）。
- Mongo→PG 迁移（G1）与方案 F 可并行设计，不阻塞 zhuzhao 主链路；若 2b 排期紧张，标为 Phase 2 按需增强。

## 关联文档
- `doc/soar/activelist.md` §3（设计原则）、§7.2（Change Stream）、§18（部署）、§19（与 zhuzhao 集成）
- `adr/ADR-001-event-mechanism-l1-steady-state.md`（L1 事件源长期稳态）
- `adr/ADR-002-asynq-async-task-executor.md`（Asynq 作为异步任务执行器，可共用 Redis 实例；注意 Asynq 不当事件总线，事件源仍是 L1）
- `docs/roadmap.md`（外部能力集成：activelist 小节）
