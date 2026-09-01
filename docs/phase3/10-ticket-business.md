# 10 - 工单业务能力闭环（Phase 3，暂缓——设计就绪）

> **定位**：工单作为下游能力（通知、SLA、满意度、告警、审批流）的入口，在 Phase 3 闭合。  
> **模块完整设计**：[modules/ticket.md](../modules/ticket.md)（§5 Hook/Port、§6 事件、§7 分阶段、§8 决策）。  
> **本文档为工单业务能力设计 SSOT**（Phase 3 启动时取用），对应 [phase3/README §2.1 Step 7](./README.md#21-phase-3先上生产--工单业务能力闭环)。  
> 创建日期：2026-08-25。
>
> ⚠️ **暂缓说明**（2026-08-25）：Phase 3 整体暂缓、不排期（见 [roadmap.md](../roadmap.md)）。本文档为**设计就绪状态**——DDL、契约、验收用例已完整，实际实现时机视真实需求决定。原 3a/3b 子阶段划分保留为能力归类参考，不作为执行子阶段。

---

## 0. 前置条件

- [x] Phase 2a 验收：TicketResource + assigned + 状态机 + `ticket_events` 表（已交付 2026-08-28）
- [x] Phase 2b 验收：scope group/all + 实体透明读（已交付；附件属 2b-ext 未交付，不在本能力关键路径）
- [x] **Phase 2c 验收**：org admin/owner + ancestor owner Authorize（审批流/分派规则的属主依赖项）（已交付 2026-08-28）
- [ ] Phase 3 Step 2 multi-instance（L1 事件消费复用分布式锁防重；SLA 扫描由 Asynq Scheduler 单点调度，无需自写锁，见 §2.3 / [ADR-002](../adr/ADR-002-asynq-async-task-executor.md)）

> 2c 是工单业务能力的硬前置：自动分派规则判断"分派给哪个组的 admin"、审批流的审批人候选都依赖 2c 的 Authorize 升级。

---

## 1. 能力清单与实现方式

| 能力 | 实现方式 | 迁移 | 事件机制 |
|------|---------|------|---------|
| SLA 计时 | `ticket_sla` 表（创建时计算 deadline + Asynq Scheduler 定时扫描违约） | 000017 | L1 + Asynq |
| SLA 违约告警 | Asynq Scheduler 扫描 → `sla:breach` task → L1 事件 + 通知 + 升级 | — | L1 + Asynq |
| 站内通知 | `notifications` 表 + L1 事件消费 | 000018 | L1 |
| 邮件通知 | SMTP 同步发送 + L1 事件消费 | — | L1 |
| 多级审批流 | `BranchedStateEngine`（手写两层分离：状态机内核 + ApprovalTaskLayer）+ `workflow_*` 表 | 000019 | L1 |
| 自动分派规则 | `assignment_rules` 表 + 规则匹配引擎 | 000020 | L1 |
| 工单报表 | SQL 聚合查询 + 进程内缓存 | — | — |
| 审批通过触发事件（场景 A） | L1 消费者 → Asynq task（数据同步/外部回调/启动工作流） | — | L1 + Asynq |
| 预置定时任务（场景 B） | Asynq PeriodicTask / Scheduler | — | Asynq |
| 事件机制升级 | L0 → L1（`ticket_events` 持久化 + 轮询补偿） | — | L1 |

> 迁移编号从 000017 起（Phase 2 用至 000016，见 [phase2/README §2.4](../phase2/README.md#24-迁移编号规划自-000010-起)）。
>
> **已前移到 Phase 2a**（2026-08-25）：工单模板（原 000018，现 000015）和工单关联（原 000019，现 000016）因纯 DB 无事件依赖前移到 2a，Phase 3 迁移编号段整体前移至 000017-000021（含 000021 `ticket_events` ALTER）。

---

## 2. SLA 计时与违约告警

### 2.1 数据模型

```sql
-- 迁移 000017
CREATE TABLE ticket_sla (
    id              BIGSERIAL PRIMARY KEY,
    ticket_id       BIGINT NOT NULL REFERENCES tickets(id),
    sla_policy_id   BIGINT NOT NULL REFERENCES sla_policies(id),
    response_deadline  TIMESTAMPTZ NOT NULL,  -- 响应时限
    resolve_deadline   TIMESTAMPTZ NOT NULL,  -- 解决时限
    responded_at    TIMESTAMPTZ,              -- 实际响应时间
    resolved_at     TIMESTAMPTZ,              -- 实际解决时间（=工单 closed）
    status          VARCHAR(20) NOT NULL DEFAULT 'running',
    -- running | responded | resolved | breached_response | breached_resolve
    breached_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE sla_policies (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(100) NOT NULL,
    priority        INT NOT NULL,              -- 工单优先级匹配
    response_hours INT NOT NULL,               -- 响应时限（小时）
    resolve_hours  INT NOT NULL,               -- 解决时限（小时）
    enabled         BOOLEAN NOT NULL DEFAULT true,
    UNIQUE(priority)                            -- P2-2 修复：同 priority 只能有一条策略，避免歧义
);
```

### 2.2 SLA 字段 SSOT 说明（P2-2 修复）

工单的 SLA 相关字段分布在三处，SSOT 声明如下：

| 字段/表 | 所在阶段 | 语义 | 处置 |
|---------|---------|------|------|
| `tickets.sla_due_at` | 2a DDL（[ticket.md §3](../modules/ticket.md#3-数据模型)） | 2a 预留的单一截止时间字段 | **Phase 3 废弃**——改用 `ticket_sla.response_deadline` + `ticket_sla.resolve_deadline` 双字段。字段保留但不再写入（向后兼容），Phase 3 迁移可加注释标记 deprecated |
| `ticket_types.default_sla_hours` | 2a DDL（[ticket.md §3](../modules/ticket.md#3-数据模型)） | 工单类型的默认 SLA 小时数 | **保留作为回退**：当 `sla_policies` 无匹配 priority 的策略时，回退用 `default_sla_hours` 计算 deadline |
| `ticket_sla` + `sla_policies` | Phase 3 新增（本节） | SLA 计时实例 + 策略表 | **Phase 3 主数据源**：`sla_policies` 按 priority 匹配，`UNIQUE(priority)` 保证唯一；`ticket_sla` 记录实际计时 |

**匹配优先级**：`sla_policies`（按 priority 精确匹配）> `ticket_types.default_sla_hours`（回退）> 硬编码默认 24h（兜底）。

### 2.3 实现方式

- **创建工单**：`OnAfterCreate` Hook 查 `sla_policies`（按优先级匹配，无匹配回退 `ticket_types.default_sla_hours`），写 `ticket_sla` 计算两个 deadline
- **响应**：首次 `assigned` 或 `comment` 触发 `responded_at` 写入（**2026-08-31 拍板：不含内部备注 note**——note 是对内动作，响应计时只认对客口径）
- **解决**：工单 `closed` 触发 `resolved_at` 写入
- **违约扫描**：由 **Asynq Scheduler**（PeriodicTask，60s 间隔）触发，多实例下 Asynq 天然单点调度，无需自写分布式锁（见 §4.6 / ADR-002）。扫描逻辑读 `ticket_sla`、判违约、写 `ticket_events`(signal) 由 L1 消费者驱动通知。

```go
// SLA 扫描任务（Phase 3 实现，由 Asynq Scheduler 每 60s 入队触发，非进程内 ticker）
// 注册：scheduler.Register(periodic.NewPeriodicTask("* * * * * ?", "sla:scan", nil))
func (s *SLAChecker) Handle(ctx context.Context, t *asynq.Task) error {
    // 不再需要自写分布式锁：Asynq Scheduler 保证同一 PeriodicTask 单实例入队
    return s.scanAndAlert(ctx)
}
```

### 2.4 定时机制（Asynq Scheduler，Phase 3 启用）

- SLA 违约扫描的定时触发在 Phase 3 交由 **Asynq Scheduler**（替代原"进程内 60s ticker"），多实例下 Asynq 天然单点调度，无需自写分布式锁（见 §4.6 ADR-002）。
- 扫描逻辑（读 `ticket_sla`、判违约、写通知）业务逻辑不变，仅触发方式从进程内定时器改为 Asynq PeriodicTask。
- 告警通知的"发送"动作仍由 L1 消费者驱动（或丢 Asynq worker 异步发）。
- **每任务阻塞/去重策略须显式拍板（2026-09-01 ginfast 调研吸收）**：扫描慢于间隔时旧任务未清会并发重叠——`sla:scan` 应配 Asynq `Unique`（同参任务未完成不入队，语义=跳过本轮，宁可漏拍不可并发重扫）；后续每个新增 PeriodicTask（归档/备份/租户扫描）落地时逐个拍板「漏跑可容忍 vs 并发可容忍」，不设全局默认。ginfast（BlockingPolicy Discard/Parallel）的运维语义维度可作对照，管理界面/执行记录由 asynqmon 原生覆盖、无需自建。

### 2.5 违约处理链（SLA 扫描 → Asynq `sla_breach` → 通知/升级）

SLA 违约本质是一个"超时事件"，按 ADR-002「事件执行总线」模式走 Asynq 异步触发，与"审批通过 → 触发事件（场景 A）"共用同一管道。**探测（扫描）与执行（通知/升级）解耦**：`sla:scan` 只负责"发现违约"，不负责"通知 + 升级"的重活。

**链路**：

```
Asynq Scheduler 每 60s 入队 sla:scan
   │  scanAndAlert 读 ticket_sla，找 deadline 已到但未达状态且未暂停的工单
   ▼  判定违约
enqueue("sla:breach", {ticket_id, sla_id, breach_type})  ──► Asynq Redis
                                          │   （扫描只在事务内标记 breached，不直发通知）
              worker(s) 异步处理 sla:breach ──┼─► ① 写 L1 事件 ticket.sla_breached（事实源，可重放）
                                              ├─► ② 站内/邮件通知：按事件路由通知主管/处理人（§3.2）
                                              └─► ③ 工单升级：改派 / 升优先级 / 加签主管（assignment_rules + workflow）
```

**职责切分（与 ADR-002 一致）**：

- `sla:scan` 仅做"探测 + 标记"：命中即把 `ticket_sla.status` 置 `breached_response` / `breached_resolve`、写 `breached_at`，**并 `Enqueue("sla:breach")`**。"标记 breached" 与 "Enqueue" **须在同一数据库事务内提交**（**2026-08-31 拍板：定死同事务方案**，"只 Enqueue"备选弃用），**Enqueue 失败则整体回滚重试**——禁止"标记成功但 Enqueue 丢失"的半成功态：否则违约已记入库却无通知/升级，且下次扫描因状态已 `breached` 不再命中。不在此处同步发通知/升级，避免扫描任务过重且不可重试。
- `sla:breach` worker 负责"副作用"：先落 L1 事件 `ticket.sla_breached`（事实源），再由 L1 消费者（或 worker 内）驱动通知与升级。L1 事件保证崩溃不丢、可重放；Asynq 保证异步执行可重试。
- 通知失败：Asynq 自带 retry + backoff，不丢违约；批量违约时排队削峰，不炸邮件网关。
- 与 §4.6 场景 A 同构：`ticket.approved` → Enqueue 触发任务；此处 `ticket.sla_breached` → Enqueue 升级任务。两者都是"L1 落事实 → 消费者 Enqueue → worker 执行副作用"。

**四处必坑（实现 checklist，TB 负向用例须覆盖，见 §8）**：

1. **状态暂停排除**：工单处于 `suspended` / `waiting_customer` 等暂停态时 `ticket_sla` 计时须 pause（不计 `breached_at`），扫描逻辑须排除这些状态，否则误违约。暂停态清单（默认 `suspended`/`waiting_customer`）进类型元数据可配置（2026-08-31 拍板）。
2. **提前解决须 cancel**：工单在 deadline 前 `responded` / `resolved` 时，应在 `OnAfterUpdate` Hook 内 `scheduler.Enqueue` 取消该 `sla:breach` 待处理任务（或 worker 执行前二次校验 `status` 是否仍违约），否则已解决单仍会被判违约通知。
3. **幂等**：`sla:breach` handler 按 `ticket_id + sla_id + breach_type` 幂等（同一 SLA 阶段只违约一次，防 Asynq retry 重复升级/重复通知）。
4. **Enqueue 原子性（防静默丢失）**：见职责切分——"标记 + Enqueue" 同事务提交；若采用"先标记后 Enqueue"且 Enqueue 失败，必须事务回滚重跑（下次扫描重新命中）；"只 Enqueue"备选已于 2026-08-31 拍板弃用；核心是杜绝半成功态。

> 违约通知的 API 与权限码见 §3.3（`notification:list` / `notification:read`）；邮件发送异步化见 §3.4（Phase 3 L2 升级时迁 Asynq worker）。

---

## 3. 通知服务（站内 + 邮件）

### 3.1 数据模型

```sql
-- 迁移 000018
CREATE TABLE notifications (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users(id),
    type         VARCHAR(50) NOT NULL,  -- ticket_assigned / ticket_status_changed / sla_breached / ...
    title        VARCHAR(200) NOT NULL,
    content      TEXT NOT NULL,
    ref_type     VARCHAR(50),           -- ticket
    ref_id       BIGINT,                -- ticket_id
    read_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_user_unread ON notifications(user_id) WHERE read_at IS NULL;
```

### 3.2 实现方式

- **L1 事件消费**：通知服务订阅 `ticket_events` 表，轮询拉取未处理事件
- **站内通知**：写 `notifications` 表，前端轮询或 WebSocket（Phase 3 先轮询）
- **邮件通知**：SMTP 同步发送（关键状态变更：分派、SLA 违约、关闭）
- **事件路由**：`ticket.assigned` → 通知处理人；`ticket.status_changed` → 通知创建人；`ticket.sla_breached` → 通知主管
- **「主管」定义（2026-08-31 拍板）**：工单所属组织的 `owner_user_ids`（实体负责人）；无 owner 时降级为上级组织 owner，链上无则仅通知处理人
- **邮件矩阵初稿（2026-08-31 拍板）**：SLA 违约（主管+处理人）、工单分派（处理人）、审批待办（当前节点审批人）三类事件发邮件；站内通知覆盖全部事件；矩阵表随 7b 细化

### 3.3 通知 API（P3-2 修复）

用户能查看通知是闭环必需，Phase 3 必须提供 API：

| 方法 | 路径 | 权限码 | 说明 |
|------|------|--------|------|
| GET | `/api/v1/notifications` | `notification:list` | 当前用户通知列表（分页，支持 `?unread_only=true` 过滤） |
| GET | `/api/v1/notifications/unread-count` | `notification:list` | 未读数（前端轮询用，轻量） |
| POST | `/api/v1/notifications/:id/read` | `notification:read` | 标记单条已读 |
| POST | `/api/v1/notifications/read-all` | `notification:read` | 全部标记已读 |

> **权限码新增**：`notification:list` / `notification:read` 需加入 Casbin 策略，所有登录用户默认拥有（`role ~= *` 绑定）。2a 不做通知，权限码在 Phase 3 迁移时加入 seed。

### 3.4 L2 升级（暂缓，按需）

- 轮询拉取（L1 单消费者）→ Asynq worker 多消费者消费 Outbox（解 L1 单消费者瓶颈）
- SMTP 同步 → Asynq 异步队列发送（Asynq 已引入见 ADR-002，此处仅将"发送"动作迁到 Asynq worker）

---

## 4. 多级审批流（BranchedStateEngine）

### 4.1 设计原则

- **借鉴 aifei-go `flow` 的两层分离结构**（参考，不借代码）：`BranchedStateEngine`（状态机内核，只算"下一节点"）+ `ApprovalTaskLayer`（人工任务层，claim/submit + `StateController`）
- **网关统一用 `NodeType` 枚举**（Exclusive/Inclusive/Parallel/Loop，借鉴 aifei-go flow），比 easy-workflow 三字段更标准、PG JSONB 友好
- 注册到 `TicketEngine` Port（见 [ticket.md §5.6](../modules/ticket.md#56-ticketengine-port引擎可替换边界)）
- **鉴权不进 Port**：引擎只产出 `ApprovalRequirement`（审批条件），`StateController` 委托 L2/L3 属主规则做最终裁决（当前用户能否认领）
- 变更类工单通过 `ChangeTicketHooks.OnAfterCreate` 启动审批流实例
- **节点状态持久化**：审批实例当前停在哪个节点、驳回回到哪，用 `workflow_node_states` 表记录（借鉴 aifei-go `flow_state`），而非仅靠单一 `tickets.status`

### 4.2 easy-workflow + aifei-go 能力映射

`BranchedStateEngine` 能力设计以 easy-workflow（节点模型）+ aifei-go flow（两层分离 + 节点状态）为参考蓝图。核心借鉴点：

| 能力 | 借鉴来源 | 自写实现要点 |
|------|---------|------------|
| 节点类型 | easy-workflow 4 种 / aifei-go `NodeType` | `NodeType` 枚举：Root/Activity/Exclusive/Inclusive/Parallel/Loop/End |
| 网关 | aifei-go `NodeType`（Exclusive/Inclusive/Parallel） | **采用 aifei-go 式枚举**，统一表达；easy-workflow 三字段仅作语义参考 |
| 会签 | easy-workflow IsCosigned + BatchCode | 保留 `batch_code` 解决多轮驳回状态隔离 |
| 自由驳回 | easy-workflow CTE 上游链 / aifei-go `revert` | **邻接表 + BFS 替代 CTE**，结合 `workflow_node_states` 的 `revert`（回到上游节点写新 Record） |
| 人工任务层 | aifei-go `flow/workflow` claim/submit + `StateController` | `ApprovalTaskLayer`：`findTask`/`claim`/`submit`，`StateController` 比对 `ApprovalRequirement` 与当前用户 L2/L3 属性 |
| 节点状态 | aifei-go `flow_state(node_id,state,vars)` | 新增 `workflow_node_states` 表，待办查询 `WHERE state='waiting'` 而非走 L1 事件 |
| 轨迹/审计 | aifei-go `Record` 链 | 新增 `workflow_records` 表（每次节点进入/离开一条），兼作 TB7/TB8 验收记录 |
| 事件 | 4 类：NodeStart/NodeEnd/TaskFinish/Revoke | 显式 `TicketHooks` 接口（编译期检查） |
| 变量 | `$` 前缀 + 表达式求值下放 DB | JSONB 存变量 + Go 侧求值（避免 SQL 注入，可单测） |
| TaskAction | easy-workflow WhatCanIDo | 保留，前端按钮驱动 |
| 双表 | 5 运行表 + 5 历史表 | 运行表 + 历史归档，冗余 `ticket_id` |

**自写时必须避开的局限**：
1. MySQL CTE 依赖 → PG `WITH RECURSIVE` 或邻接表 + BFS
2. 表达式求值下放 DB → Go 侧求值（`govaluate` 或自写）
3. 反射事件注册 → 显式 `TicketHooks` 接口
4. （新增）引擎内嵌用户身份 → 用 `ApprovalRequirement` + `StateController` 委托 L2/L3，引擎不持有用户/组织概念

### 4.3 数据模型

> **2026-08-31 注（与 §4.10 决议的衔接）**：① 本节 000019 DDL **尚未含发布快照表**——按决议 4（版本/发布快照），快照表 DDL（`(workflow_id, version)` 唯一）随 7-0 修订补充，届时 Deploy 动作一并落地；② 节点 meta 示例中的 `min_level` 已被决议 1 **弃用**（改 `Assignee{rule,values}` 策略模型），示例待 7-0 更新，以决议为准。

```sql
-- 迁移 000019
CREATE TABLE workflow_definitions (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(50) UNIQUE NOT NULL,   -- change_approval
    name        VARCHAR(100) NOT NULL,
    definition  JSONB NOT NULL,                -- Node 列表（借鉴 aifei-go NodeType 枚举 + 节点 meta）
    version     INT NOT NULL DEFAULT 1,
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- definition 中每个 Node 结构（借鉴 aifei-go flow/workflow meta 机制；ApprovalRequirement 为 JSONB 条件对象，引擎只透传不解析）：
--   {"code":"approve_l1","type":"Activity","meta":{"role":"group_admin","org_scope":"ticket_org"}}
--   {"code":"approve_l2","type":"Activity","meta":{"role":"owner","org_scope":"ancestor","min_level":3}}
--   {"code":"approve_hr","type":"Activity","meta":{"any":[{"role":"dept_head","org_scope":"ticket_org","min_level":3},{"role":"hrbp","org_scope":"ancestor"}]}}
--   {"code":"gw","type":"Exclusive","meta":{}}
-- role 取值：group_admin | owner | assignee | creator | dept_head | hrbp | custom_role | user_id
-- org_scope 取值：ticket_org | ancestor | self
-- 支持职级(min_level)、OR 组合(any)、指定人(user_id) 等进阶语义，详见 ticket.md §5.6 ApprovalRequirement 说明

CREATE TABLE workflow_instances (
    id              BIGSERIAL PRIMARY KEY,
    ticket_id       BIGINT NOT NULL REFERENCES tickets(id),
    definition_id   BIGINT NOT NULL REFERENCES workflow_definitions(id),
    status          VARCHAR(20) NOT NULL DEFAULT 'running',  -- running | done | rejected | canceled
    current_node    VARCHAR(50),                  -- 当前所处节点 code（冗余，便于查询；真相见 workflow_node_states）
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at     TIMESTAMPTZ
);

-- 节点级状态表（借鉴 aifei-go flow_state）：审批实例当前停在哪、待办是谁、驳回回退
-- 待办查询直接 WHERE state='waiting' AND 满足 Requirement，不走 L1 事件轮询
CREATE TABLE workflow_node_states (
    id              BIGSERIAL PRIMARY KEY,
    instance_id     BIGINT NOT NULL REFERENCES workflow_instances(id),
    node_code       VARCHAR(50) NOT NULL,
    state           VARCHAR(20) NOT NULL,        -- waiting | auto_forward | done | rejected
    requirement     JSONB,                        -- 该节点的 ApprovalRequirement（来自 definition meta，冗余存储）
    vars            JSONB DEFAULT '{}',           -- 节点级变量（借鉴 aifei-go flow_state.vars）
    entered_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    left_at         TIMESTAMPTZ,
    UNIQUE(instance_id, node_code)
);

-- 轨迹/审计表（借鉴 aifei-go Record 链）：每次节点进入/离开一条，兼作 TB7/TB8 验收记录
CREATE TABLE workflow_records (
    id              BIGSERIAL PRIMARY KEY,
    instance_id     BIGINT NOT NULL REFERENCES workflow_instances(id),
    node_code       VARCHAR(50) NOT NULL,
    node_type       VARCHAR(20) NOT NULL,
    action          VARCHAR(20) NOT NULL,        -- enter | leave | approve | reject | revert
    actor_id        BIGINT,                       -- 操作人（NULL=系统自动）
    is_end          BOOLEAN DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE workflow_tasks (
    id              BIGSERIAL PRIMARY KEY,
    instance_id     BIGINT NOT NULL REFERENCES workflow_instances(id),
    node_code       VARCHAR(50) NOT NULL,
    batch_code      VARCHAR(50),                  -- 借鉴 easy-workflow 批次码，隔离多轮驳回产生的任务
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending | claimed | done | rejected
    claimed_by      BIGINT,
    claimed_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- workflow_instance_history（历史归档，冗余 ticket_id 便于关联查询，借鉴 easy-workflow 双表设计）
```

### 4.4 触发时机

按 [ticket.md §8.6](../modules/ticket.md#86-工作流引擎升级触发信号量化表) 量化表，触发任一即从 `LinearStateEngine` 升级为 `BranchedStateEngine`：
- 需要分支/会签的工单类型数 ≥ 2
- 单流程平均节点数 > 8
- 并行审批路径数 ≥ 1
- 跨流程驳回场景出现

> **Phase 3 启动时实现 BranchedStateEngine**（P2-3 修复）：§8 TB7/TB8 要求审批流实例有记录，是设计验证用例（Phase 3 启动时取用）。Phase 3 启动时，即使实际工单类型未达触发信号，`BranchedStateEngine` 仍需实现并通过验证——至少注册一个示例审批流定义（如变更类工单两节点审批）。§8.6 触发信号表的作用是**"后续扩展更多审批流类型"的决策依据**，不是"是否实现引擎"的依据。引擎实现是 Phase 3 硬交付，触发信号决定的是"何时加更多流程定义"，不是"何时写引擎"。

### 4.5 节点级鉴权与驳回语义（决策固化）

**（1）`CanApproveNode` 方法（节点级鉴权，对应前述确认 #2）**
- 新增 `CanApproveNode(ctx, ticketID, req json.RawMessage) (bool, error)`，位于 `TicketService` 层。
- 内部先调 `Authorize`（L2/L3 资源级，确认用户对工单有审批动作权）→ 再解析 `req`（ApprovalRequirement JSONB）比对当前用户的角色/组织属性（`org_path`、角色表、`min_level`、`any` 组合）。
- `StateController.IsOperatable` 的 TicketService 实现即调 `CanApproveNode`。**引擎内核零鉴权概念**。

**`CanApproveNode` 契约骨架（文档级，Phase 3 实现前先钉死接口）**

```go
// === 输入 ===
type CanApproveNodeInput struct {
    TicketID  int64           // 工单 ID（定位工单 + 当前节点）
    NodeCode  string          // 当前审批节点 code（来自 workflow_node_states）
    ActorUserID int64         // 当前操作人（来自 JWT/上下文）
    Req       json.RawMessage // ApprovalRequirement JSONB（节点审批要求：user_id / role+org_scope / min_level / any）
}

// === 输出 ===
type CanApproveNodeResult struct {
    Allow   bool   // 是否可审批
    Reason  string // 拒绝原因（debug / 前端提示用，如 "org_scope mismatch"）
}

// === 依赖（Phase 1 现状） ===
//   L1 资源级：casbin.SyncedEnforcer.Enforce(sub, obj, act)  —— 已实现（internal/casbin + rbac_service 封装）
//   L2 组织级：organizations.path (ltree "a.b.c")            —— 已实现（OrgRepo，祖先查询用 PG <@ / nlevel / ancestors）
//   L3 已办级：workflow_records 表（迁移 000019）            —— ⚠️ 未实现，Phase 3 需新建 WorkflowRecordRepo + TicketWorkflowRepo
func (s *TicketService) CanApproveNode(ctx context.Context, in CanApproveNodeInput) (CanApproveNodeResult, error) {
    // 步骤1 L1：资源级——当前用户对该工单有 approve 动作权（Casbin sub=roleCodes, obj="ticket:<id>", act="approve"）
    if allow, _ := s.enforcer.Enforce(actorRoleCodes, fmt.Sprintf("ticket:%d", in.TicketID), "approve"); !allow {
        return CanApproveNodeResult{Allow: false, Reason: "no approve permission"}, nil
    }
    // 步骤2 解析 Req（ApprovalRequirement JSONB）
    req := parseApprovalRequirement(in.Req) // user_id / role+org_scope / min_level / any
    // 步骤3 user_id 指定人（最简路径，必做）
    if req.UserID != nil {
        return CanApproveNodeResult{Allow: *req.UserID == in.ActorUserID}, nil
    }
    // 步骤4 role + org_scope（顺手做：查用户角色 + 组织祖先链比对）
    if req.Role != nil && req.OrgScope != nil {
        if s.userHasRole(in.ActorUserID, *req.Role) &&
           s.userOrgWithinScope(in.ActorUserID, *req.OrgScope) { // OrgRepo: organizations.path <@ scopePath
            return CanApproveNodeResult{Allow: true}, nil
        }
        return CanApproveNodeResult{Allow: false, Reason: "role/org_scope mismatch"}, nil
    }
    // 步骤5 min_level / any（结构预留，暂未实现解析，遇未知 key 明确报错）
    if req.MinLevel != nil || req.Any != nil {
        return CanApproveNodeResult{}, errcode.ErrNotImplemented // 待真实需求出现再补 if 分支
    }
    return CanApproveNodeResult{Allow: false, Reason: "empty requirement"}, nil
}
```

- **调用顺序铁律**：L1（资源级 Casbin）→ 解析 Req → L2（角色/组织祖先比对）→ L3（已办去重，Phase 3 接入 `workflow_records` 后追加：若 ActorUserID 已在该节点 `action=approve` 记录中则拒，防重复审批）。
- **L3 缺口说明**：`workflow_records` 表（迁移 000019）与 `WorkflowRecordRepo` 属于 Phase 3 范围，CanApproveNode 的 L3 分支需在 Phase 3 与审批流同批落地；当前只钉接口，不引未实现依赖。

**（2）驳回/回退语义（对应确认 #3）**
- `reject` 动作携带 `target` 参数：
  - `target=prev`：退回**上一级**审批节点（`workflow_node_states` 当前节点回退到上游节点，`state='waiting'`，写 `action=revert` + `action=enter` 两条 Record）。
  - `target=origin`：打回**发起人**（回到创建节点，整单重新提交；`workflow_records` 记 `revert` 到 origin）。
- 自由驳回（跨级回退到任意上游）：流程定义节点声明 `meta.allow_free_reject=true` 时开启，用邻接表 + BFS 计算上游链（借鉴 easy-workflow `TaskFreeRejectToUpstreamNode`）。
- 会签（多审批人）：同一节点 `meta.cosigned=true` 时，`workflow_tasks` 按 `batch_code` 生成多条任务，`workflow_node_states.state` 在全部 `done` 后才推进（借鉴 easy-workflow `BatchCode`）。

**（2.1）审批人解析实现优先级（按工作量从小到大，先实现前两项，避免过度设计）**
1. `user_id` 指定人 —— **必做，最简单**（直接比对 `当前用户 == req.user_id`，不查 org_path/角色表）。
2. `role` + `org_scope` —— **顺手做**（常见"退给组 admin 审批"，只需查一次用户角色/组织属性）。
3. `min_level` / `any` 组合 —— **结构已预留（JSONB），暂不实现解析分支**；`CanApproveNode` 遇到未支持的 key 返回明确错误，待真实需求出现再补（加一个 if 分支即可，工作量极小）。
> 结论：最常用场景"指定人审批"零额外成本即满足；职级/OR 等进阶语义不为当前阶段增加工作量，但接口不堵死。

**（3）审批完成与 L1 事件联动（对应确认 #4）**
- **L1 事件** = `ticket_events` 表中的业务动作记录（持久化 + 轮询单消费者异步驱动通知/SLA/审计，进程崩溃不丢）。
- 审批 `submit` 成功时，在**同一事务内**同时写：
  1. `workflow_records`（轨迹，action=approve/reject）+ 更新 `workflow_node_states`；
  2. `ticket_events(action='ticket.approved' | 'ticket.rejected', ticket_id, node_code)`。
- L1 消费者据此发"已通过通知给发起人 / 待你审批通知给下一节点候选 / SLA 节点重置"，**审批逻辑不硬塞发通知代码**，统一走 L1 管道。
- **审批通过 → 自动触发某个事件（场景 A）**：L1 消费者收到 `ticket.approved` 后，将"触发事件"作为 Asynq task 入队（见 §4.6），由 Asynq worker 异步执行（如数据同步、外部回调、启动另一工作流实例）。**不在审批事务内同步执行耗时动作**，也不让 Asynq 承担事件持久化（事件事实已在 L1）。

**（4）自写 + 双引擎借鉴（对应确认 #5）**
- **aifei-go flow 借鉴**：两层分离（状态机内核 + 人工任务层 `ApprovalTaskLayer`）、`StateController` 委托鉴权、`NodeType` 网关枚举（Exclusive/Inclusive/Parallel/Loop）、节点级状态表、Record 轨迹链、`revert` 回退、`claim/submit` 语义。
- **easy-workflow 借鉴**：4 节点类型、会签 `BatchCode`、自由驳回上游链（CTE→BFS）、`WhatCanIDo` 自描述操作、`TicketHooks` 事件接口（替代反射）。
- **任务可操作查询（落地 `WhatCanIDo`）**：前端不硬编码按钮，由后端按"当前节点 + 任务状态 + 当前用户角色"返回可执行动作集合。新增 API：`GET /api/v1/workflow-tasks/:task_id/actions` → 返回动作列表（如 `approve` / `reject`(`target=prev|origin`) / `transfer` / `free_reject`），后端实现即 `WhatCanIDo(taskID)`（状态机内核判定当前节点允许的转移 + `CanApproveNode` 判定当前用户是否具备操作资格）。权限码复用 `ticket:approve` 等现有 L1 动作权。
- **自写（不引代码）**：pgx 重写，DDL 用本节 `workflow_*` 表，鉴权走 `CanApproveNode` + L2/L3，不引入 aifei-go 任何运行时依赖。

**### 4.6 Asynq 异步任务执行器定位（对应 ADR-002）**

引入 Asynq 作为"异步任务执行器"（复用现有 Redis，不新增基础设施），与 L1 事件源职责互补。**铁律：L1 管事件持久化，Asynq 管异步执行，不在 Asynq 里重建事件源。**

| 业务场景 | 触发方式 | 机制 | 说明 |
|---------|---------|------|------|
| 对接数据自动建工单 | 同步 | 工单创建逻辑（现有） | — |
| 手动建工单 | 同步 | 工单创建 API | — |
| 审批通过 → 触发事件（场景 A） | 异步 | **L1 消费者 → `asynq.Enqueue`** | 消费者收到 `ticket.approved` 后入队；worker 执行数据同步/外部回调/启动工作流实例 |
| 预置定时任务（场景 B） | 定时 | **`asynq.PeriodicTask` / Scheduler** | 周期触发（如每日过期扫描、每周报表），替代原进程内定时器，多实例天然单点调度 |
| 通知/邮件发送 | 异步 | L1 消费者 + Asynq（可选） | 通知写入 L1 驱动，SMTP 发送可丢 Asynq 异步发 |
| SLA 违约 → 升级/通知 | 定时 | **`sla:scan` → `sla:breach` task** | 扫描只探测标记，worker 落 L1 事件 `ticket.sla_breached` 后驱动通知/升级（见 §2.5） |

**职责切分要点**：
- 审批 `submit` 成功 → 同一事务先落 L1 `ticket_events`（事实源，崩溃不丢）→ 消费者事务提交后 `asynq.Enqueue` 触发任务。
- 任务 handler 按 `ticket_id + event_id` 幂等（防 L1→Asynq 窗口重复入队）。
- 若"触发事件"= 启动另一工作流实例：`BranchedStateEngine.InstanceStart` 仍是引擎内核职责，Asynq 只入队"启动任务"，不让 Asynq 变成隐式流程引擎。
- Redis 挂了：L1 事件仍在，恢复后重放；Asynq 任务失败：Asynq 自带重试。

> 此定位与 ADR-001（L1 长期稳态、Asynq 不替代事件源）一致，详见 `adr/ADR-002-asynq-async-task-executor.md`。

### 4.7 前端配置形态（决策固化，2026-08-26）

**结论：流程骨架写死，节点审批人前端可编辑**（固定流程模板 + 审批人绑定可配）。

- **流程骨架固定**：节点序列、`NodeType`（Root/Activity/Exclusive/Inclusive/Parallel/Loop/End）、网关走向在代码或 seed 固定定义中写死（如变更类工单固定"L1 组长审批 → L2 主管审批"两段）。不提供拖拉拽画布编排流程结构——避免为"可能性"提前买单（与 design-decisions "不过度设计"一致）。
- **节点审批人可在前端编辑**：每个节点的 `ApprovalRequirement`（`role` / `org_scope` / `min_level` / `user_id` / `any` 组合，见 §4.5(1) 与 ticket.md §5.6）通过**前端配置界面**编辑，持久化回 `workflow_definitions.definition` 的对应 Node `meta`。即"流程长什么样"不可拖，"谁审这个节点"可配。
- **需提供的后端能力**：
  - 定义编辑 API：如 `PUT /api/v1/workflows/:code`（或 `PATCH` 单节点审批人），校验 `ApprovalRequirement` 合法性后更新 `definition` JSONB；权限码 `workflow:manage`（仅管理员/流程配置角色）。
  - 前端据此渲染"审批人配置"表单（按 `role+org_scope` / `user_id` / `min_level` / `any` 分类型的字段），保存即写回定义表。
- **不做**：拖拉拽可视化流程编辑器（画布/连线/版本 diff）。该能力作为**远期可选增强**——届时后端 `definition` 表与引擎内核可原样复用，仅新增前端画布层。
- **与"流程写死 A 方案"的区别**：纯写死 A 连审批人都写死在代码里，调审批人要发版；本决策让"审批人"成为数据（DB 配置），改审批人只需前端操作，契合业务多变的真实需求，且不引入流程结构编辑的复杂度。

### 4.8 借鉴 easy-workflow（ecmdb 同款流程引擎，2026-08-26 复核）

复核了 `Duke1616/ecmdb` 及其依赖的 `Bunny3th/easy-workflow`（Go+Gin 后端流程引擎库，前端"可视化自定义"由该库的 `POST /def/save` REST API 承载——入参是 `Process/Node` JSON 图，前端设计器把画布导出成该 JSON 提交）。结论：**zhuzhao 的 `workflow_definitions.definition` JSONB 在结构上已与该引擎的 `Node` 数组高度同构**，下列要点可直接吸收且不与 §4.7"骨架写死"决策冲突。

- **节点数组即定义（已对齐）**：easy-workflow 的 `Node{NodeID, NodeName, NodeType(0开始/1任务/2网关/3结束), PrevNodeIDs[], UserIDs[], Roles[], IsCosigned, GWConfig, Events[]}` 与 zhuzhao `definition` 中每个 Node 的 `{code, type, meta{role/org_scope/min_level/user_id/any}}` 一一对应。迁移到完整自定义时后端模型改动极小。
- **可吸收的细化点（即便骨架写死也值得加）**：
  1. **`WhatCanIDo` 任务可操作查询**：后端按当前节点/状态返回"当前用户能做什么"（通过/驳回/转交/自由驳回），前端不硬编码按钮。**已落地**：API `GET /api/v1/workflow-tasks/:task_id/actions`（§4.5(4)），后端 `WhatCanIDo(taskID)` = 状态机判定允许转移 + `CanApproveNode` 判定操作资格。
  2. **Root 自动通过 + 显式 End 防呆**：流程一开始 Root 节点即自动通过（无需人审）；保留显式 End 节点防"分支末尾漏标结束导致流程卡死"。zhuzhao 已有 Root/End `NodeType`，补一条"实例启动时 Root 自动完成"的规则即可（Phase 3 引擎实现时纳入）。
  3. **`BatchCode` 批次码（任务重提）**：节点被驳回后重新提交会产生"新一批 task"，用批次码区分，避免历史任务与重提任务混淆。**已落地**：`workflow_tasks.batch_code`（迁移 000019 DDL）已存在，§4.5(2) 会签 / §4.5(4) 已引用其语义。
  4. **节点级事件钩子**：easy-workflow 的 `NodeStartEvents/NodeEndEvents/TaskFinishEvents` 是"流程与业务解耦"的成熟手法——zhuzhao 已用 L1 `ticket_events` 做同理的事，可在 Node `meta` 预留 `events` 字段（如 `on_enter` 触发改派），与 ADR-001 对齐。
- **可选的结构简化（远期）**：easy-workflow 用**一个 `HybridGateway`**（Conditions + InevitableNodes + WaitForAllPrevNode）替代排他/并行/包含三种网关。zhuzhao 当前用 4 种显式 `NodeType`（Exclusive/Inclusive/Parallel/Loop），可读性更好、契合简单域；若未来网关变复杂，可平滑切换为混合网关，不必现在改。
- **实例变量（网关条件）**：easy-workflow 的 `ProcInstVariable`（key/value，如 `$days>=3`）供网关条件表达式求值。zhuzhao 网关当前是 `SimpleExpression`（priority/role），若未来要"按工单字段分流"，可加 `workflow_instances.vars`（已预留 JSONB）+ 轻量表达式求值，与 easy-workflow 同构。
- **2026-08-31 实地调研修正（eflow + easy-workflow 落地代码核验）**：①「同构」需补精度——**驳回回退是引擎最难的 10%**（eflow 靠系统事件 SystemPass/Reject/Skipped + 代理节点修正 prev_node_id 实现），§4.5 自由驳回按硬骨头排期；② `workflow_definitions` **必须补版本/发布快照**（eflow：编辑版/发布版分离，Deploy 锁快照表 `(workflow_id, process_id, version)` 唯一，运行时按实例版本取快照——原地改 definition 会污染在途实例）；③ **发起人撤回（Revoke）业务设计缺失**（eflow WITHDRAWING 栅栏状态机 + EventRevoke 校验 + 补偿任务），已并入 B1 设计期清单；④ 网关条件维持 SimpleExpression（eflow 裸 SQL 表达式靠 `1=1` 兜底，反面教材）。
- **多租户 `Source` 隔离**：引擎用 `Source` 字段区分"哪个系统创建的流程"，便于同一引擎服务多业务。zhuzhao 当前单租户，可暂不引入；若未来 IAM 多业务共用引擎，记为扩展点。
- **与 §4.7 的关系**：上述要点均作用于"定义内容/任务交互/审计"层，**不要求流程结构可拖拽**。§4.7 的"骨架写死、审批人可配"仍是当前决策；一旦未来要开放完整自定义，本 §4.8 证明后端 `definition` 模型已兼容，只需新增前端画布 + `PUT /workflows/:code` 全量保存（对齐 easy-workflow `POST /def/save`）。

### 4.9 配置即代码：workflow / SLA 种子版本化载入（借鉴 ecmdb `bootstrap`）

> 借鉴 ecmdb `bootstrap/loader.go`（YAML 幂等加载）+ `cmd/initial`（版本化增量 + dry-run）。**背景**：项目既有种子（`migrations/000002_seed.up.sql`，初始化角色/组织/admin/菜单/Casbin）用的是裸 SQL `INSERT ... ON CONFLICT DO NOTHING` 保证幂等，但**无版本闸门、无 dry-run**；且 `workflow_definitions` / `sla_policies`（迁移 000017/000019）此前**根本没有种子行**。因此本节的定位是：**为 workflow / SLA 新建声明式 YAML + 版本化幂等载入机制**（比旧 SQL 种子多了版本闸门 + dry-run + git 审计），未来 RBAC 种子也可按此范式升级。**目标**：杜绝多环境手改 SQL 漂移，并与 §4.7 的 `PUT /workflows/:code` 管理编辑天然共存。

#### 4.9.1 目录与文件布局
```
configs/
  workflows/
    change_approval.yaml     # 一条 workflow_definitions 行
    leave_approval.yaml
  sla/
    default_policies.yaml    # 一组 sla_policies 行（1 个文件多策略）
  bootstrap.yaml             # 可选：env / tenant_id / 加载顺序 / dry_run 开关
```
- 表 DDL 仍留在 migrations（000019 / 000017）；**仅行数据移入 YAML**。

#### 4.9.2 YAML 格式（与表 1:1 映射）
```yaml
# configs/workflows/change_approval.yaml
code: change_approval
name: 变更审批流
version: 3                 # 与 workflow_definitions.version 对齐；仅当 yaml.version > db.version 才应用
enabled: true
definition:                # 即 workflow_definitions.definition（JSONB）的 YAML 写法
  - { code: start,  type: Root,      meta: {} }
  - { code: approve_l1, type: Activity, meta: { role: group_admin, org_scope: ticket_org } }
  - { code: approve_hr, type: Activity, meta: { any: [ {role: dept_head, org_scope: ticket_org, min_level: 3}, {role: hrbp, org_scope: ancestor} ] } }
  - { code: gw,     type: Exclusive, meta: {} }
  - { code: end,    type: End,       meta: {} }
```
```yaml
# configs/sla/default_policies.yaml  （sla_policies 有 UNIQUE(priority)，按 priority upsert）
policies:
  - { name: "P0 紧急", priority: 1, response_hours: 1,  resolve_hours: 4,  enabled: true }
  - { name: "P1 高",   priority: 2, response_hours: 4,  resolve_hours: 24, enabled: true }
  - { name: "P2 中",   priority: 3, response_hours: 8,  resolve_hours: 72, enabled: true }
```

#### 4.9.3 载入器契约（核心规则）
1. **幂等 upsert**：workflow 按 `code` upsert；SLA 按 `priority` upsert（对齐 `UNIQUE(priority)`）。重复执行 = 无副作用。
2. **版本闸门（防回退 admin 编辑）**：仅当 `yaml.version > db.version` 才写；`yaml.version <= db.version` 跳过并 warning。这保证 §4.7 的 `PUT /workflows/:code` 编辑（会 bump `version`）不会被种子悄悄覆盖。
3. **与运行时 `PUT` 的冲突策略**：
   - 种子 = **出厂基线**（初始 version，如 v1/v2/v3）。
   - 管理员 `PUT` 编辑审批人 `meta` → bump version（如 v3→v4）。
   - 载入器只前进版本：只要 DB version 已 ≥ 种子 version，种子不再触碰该定义 → **admin 编辑被保留**。
   - 若确需"恢复出厂"，是显式危险操作（`reset-to-factory`，需确认），不属常规 bootstrap。
   - **可选更干净拆分（远期）**：把"骨架（节点序列/类型/网关）"与"审批人 `meta`（可编辑）"拆到两张表（`workflow_definitions.definition` 存骨架 + `workflow_definition_approvers` 存可编辑项），种子只写骨架、永远不碰审批人表 → 彻底无冲突。当前用版本闸门即可，无需现在改表。
4. **Dry-run**：`go run ./cmd/bootstrap --dry-run` 打印 diff（将插入 / 将 version N→M / 将跳过），不落库。CI 中跑 dry-run 检测 YAML 非法 / 与 DB 漂移。
5. **校验前置（fail-fast）**：应用前校验——node `code` 唯一、恰有 1 个 `Root`、每个 `End` 可达、`Exclusive` 分支 ≥1、`ApprovalRequirement` meta 合法（按 ticket.md §5.6）。任一不合法则整体中止。
6. **执行时机**：迁移（migrations）应用后、作为 `cmd/initial` / `cmd/bootstrap` 幂等步骤运行；因幂等+版本闸门，**每次启动重跑也安全**（无变化即 no-op）。tenant 默认 `tenant_id=1`，多租户时由 `bootstrap.yaml` 指定。

#### 4.9.4 与 ecmdb 的映射
| ecmdb | zhuzhao §4.9 |
|-------|-------------|
| `bootstrap/loader.go` 读 YAML 幂等建模型/字段/关系 | 读 YAML 幂等 upsert `workflow_definitions` / `sla_policies` |
| `cmd/initial` 版本化增量 + dry-run | `cmd/bootstrap [--dry-run]`，版本闸门 + dry-run |
| 模型 YAML 声明式、可重放 | workflow/SLA 定义声明式、可重放、可审计（谁改了基线看 git） |

#### 4.9.5 收益
- **Git 即审计**：流程/SLA 基线变更走 PR review，比 DBA 手改 SQL 安全、可回溯。
- **多环境一致**：dev/staging/prod 同一份 YAML，启动幂等载入，零漂移。
- **与 §4.7 兼容**：admin 仍可 `PUT` 微调审批人，载入器不回退其编辑。
- **为远期拖拽预留**：一旦开放完整自定义，种子变成"初始版本"，新版本由前端画布 `PUT` 产生，载入器契约不变。

---

### 4.10 7-0 设计期决议（2026-08-31 拍板，细节随 7-0 修订展开）

| # | 决议 | 内容 |
|---|------|------|
| 1 | 审批人策略模型 | 采纳 `Assignee {rule, values}`（指定人/发起人/模板字段/部门领导/分管领导/团队/部门 7 种，eflow 对标）；**弃 `min_level` 职级语义**（users 无 level 列，数据源悬空），组织计算运行期解析 |
| 2 | 模板-流程绑定 | **类型 1:1 默认**（`workflow_id` 挂 `ticket_types`）+ 模板 nullable 覆盖（`ticket_templates.workflow_id` 空则用类型默认）——Jira 模式为主、钉钉模式为辅 |
| 3 | 发起人撤回 | WITHDRAWING 栅栏模式（eflow 对标）：`WITHDRAWING` 态拦截其他流转 → 校验（仅发起人/审批中）→ `EventRevoke` 回收任务 → 补偿；引擎钩子接口已有（§4.2），业务态与校验按此补设计 |
| 4 | 版本/发布快照 | `workflow_definitions` 增 `version` + 发布快照表（`(workflow_id, version)` 唯一）：编辑版/发布版分离，Deploy 锁快照，运行时（Pass 校验/历史图）一律按实例版本取快照——原地改 definition 禁止 |
| 5 | signal 双写 | **定死两条记录分离**（audit/signal 各一条），弃单条双标（§7.2 回标） |
| 6 | 权限码 seed | `ticket:approve`、`notification:list`、`notification:read`、`workflow:manage`、`report:read`，随 seed 迁移挂对应菜单（管理角色） |

## 5. 自动分派规则

> **编号说明**：原 §5 工单模板、§6 工单关联已于 2026-08-25 前移到 Phase 2a（迁移 000015/000016，DDL 见 [phase2/09-ticket.md §2](../phase2/09-ticket.md)）。本节原 §7 自动分派顺延为 §5。

```sql
-- 迁移 000020
CREATE TABLE assignment_rules (
    id            BIGSERIAL PRIMARY KEY,
    name          VARCHAR(100) NOT NULL,
    type_code     VARCHAR(50),              -- 匹配工单类型（null=所有）
    priority_min  INT,                      -- 匹配优先级下限
    keyword       VARCHAR(100),             -- 标题/描述关键词匹配
    target_org_id BIGINT NOT NULL REFERENCES organizations(id),
    target_user_id BIGINT REFERENCES users(id),  -- 可选指定人
    priority      INT NOT NULL DEFAULT 100,  -- 规则优先级（小=先匹配）
    enabled       BOOLEAN NOT NULL DEFAULT true
);
```

- **规则匹配引擎**：工单 `OnAfterCreate` Hook 按优先级遍历 `assignment_rules`，首个匹配的规则执行分派
- 分派操作走 `TicketService.Assign`（走三层鉴权，不绕过）
- **规则引擎五项决议（2026-08-31 拍板）**：① keyword 匹配 = LIKE 包含（不引入分词）；② 同优先级按 `id` 升序 tie-break；③ `target_user_id` 必须属于 `target_org_id`（写入校验）；④ 无命中兜底 = 工单保持 open（写 `dispatch_missed` 事件供报表统计）；⑤ **分派失败不回滚创建**（Hook 内捕获失败 → 写事件 + 异步补偿），创建主链路永不被分派拖垮

---

## 6. 工单报表

- **Phase 3 实现**：SQL 聚合查询 + 进程内缓存（TTL 5min）
- API：`GET /api/v1/tickets/reports/by-org`、`by-assignee`、`by-type`、`sla-stats`
- **L2 升级（暂缓，按需）**：物化视图 + 定时刷新任务（Asynq PeriodicTask，Asynq 已引入见 ADR-002，定时触发机制复用）
- **四缺口收口（2026-08-31 拍板）**：① 权限码新增 `report:read`（挂报表菜单，仅管理角色）；② 缓存 = 进程内 TTL 5min + 管理端手动刷新端点；③ 指标口径：SLA 达成率 = 未违约工单/总数，响应/解决时长 = P50/P95 分布；④ 全部接口支持时间范围（默认近 30 天）+ 分页

---

## 7. 事件机制 L0 → L1 升级

### 7.1 L0（Phase 2a 现状）

- Go channel + goroutine
- 进程崩溃丢事件

### 7.2 L1（长期稳态，Phase 3 启动时实现，见 ADR-001）

**前置迁移**（迁移 000021）：2a 建的 `ticket_events` 表缺 L1 所需的两列，Phase 3 启动时需 ALTER 补列（L1 长期稳态，见 ADR-001）：

```sql
-- 迁移 000021
ALTER TABLE ticket_events ADD COLUMN event_type VARCHAR(16) NOT NULL DEFAULT 'audit';
-- audit（审计日志，工单详情页展示）/ signal（事件队列，供消费者拉取）
ALTER TABLE ticket_events ADD COLUMN processed BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX idx_ticket_events_signal_unprocessed ON ticket_events(event_type, processed) WHERE event_type = 'signal' AND processed = FALSE;
```

- 事件写 `ticket_events` 表（2a 已建，事务内保证不丢）
- **双重职责过滤**（P3-3 修复）：`ticket_events` 同时承担审计日志（`event_type='audit'`，工单详情页展示，无需 processed）和事件队列（`event_type='signal'`，供消费者拉取）。Service 层写入时按事件语义标记 `event_type`：状态变更/分派等操作记录写 `audit`；需要触发下游副作用（通知/SLA）的事件**同时写一条 `signal` 记录**（或一条记录同时标记两种类型，但分离更清晰）
- 消费者轮询拉取 `ticket_events`（`WHERE event_type = 'signal' AND processed = FALSE`）
- **单消费者模型**（P2-4 修复）：L1 为**单消费者**——通知服务串行处理全部副作用（通知写入 + SLA 更新 + 其他）。`processed` 标记只由通知服务写入。多订阅（通知/SLA/满意度各自独立消费）留到 L2 Outbox + Asynq，L1 不支持多消费者
- 多实例下用分布式锁保证同一事件不被重复消费
- 处理完成后标记 `processed = TRUE`
- **L1 为长期稳态机制，非临时过渡**（见 [ADR-001](../adr/ADR-001-event-mechanism-l1-steady-state.md)）：`processed` 标记、分布式锁防重、双重职责(audit/signal)分离均需按产线级别实现。Kafka 仅在真实微服务拆分时才评估。Asynq 作为异步任务执行器与 L1 职责互补（见 [ADR-002](../adr/ADR-002-asynq-async-task-executor.md)），不替代事件源。

### 7.3 L2（暂缓，按需升级）

- `ticket_events` 信号消费 → Outbox 表 + Asynq worker 多消费者（解 L1 单消费者瓶颈）
- 轮询拉取 → Asynq worker 消费（Asynq 已引入见 ADR-002，此处仅将"事件消费"也迁到 Asynq worker，业务不变只换调度器）
- 业务逻辑（通知内容、SLA 规则）不变

---

## 8. 验收用例

> 用例编号沿用 Phase 2 模式（T 前缀），Phase 3 工单业务能力新增 TB 前缀（Ticket Business）。以下为设计验证用例，Phase 3 启动时取用。
>
> **已前移到 Phase 2a**：TB9（模板创建）、TB10（工单关联）随 §5/§6 前移，2a 验收覆盖。

| 用例 | 场景 | 验证点 |
|------|------|--------|
| TB1 | 创建高优工单 → SLA 计时启动 | `ticket_sla` 有记录，deadline 正确 |
| TB2 | 工单分派 → 响应计时停止 | `responded_at` 写入 |
| TB3 | 工单关闭 → 解决计时停止 | `resolved_at` 写入 |
| TB4 | SLA 超时 → 违约告警 | `status=breached_*`，通知发出 |
| TB5 | 工单分派 → 站内通知 | `notifications` 表有记录 |
| TB6 | SLA 违约 → 邮件通知 | SMTP 发送成功（mock 验证） |
| TB7 | 变更类工单 → 审批流启动 | `workflow_instances` 有记录 |
| TB8 | 审批通过 → 工单状态流转 | 状态正确变更 |
| TB9 | 关键词匹配 → 自动分派 | `assignment_rules` 命中，工单分派到正确组 |
| TB10 | 报表查询 | 按组织/处理人/类型统计正确 |
| TB11 | 事件 L1 不丢 | 进程重启后未处理事件继续消费 |
| TB12 | 暂停态不误违约 | suspended/waiting_customer 工单到 deadline 不判违约（§2.5 必坑 1） |
| TB13 | 提前解决取消任务 | deadline 前 resolved → sla:breach 待处理任务被取消/二次校验拦截（必坑 2） |
| TB14 | 违约通知幂等 | Asynq 重试下同一 (ticket_id+sla_id+breach_type) 仅通知一次（必坑 3） |
| TB15 | Enqueue 原子性 | Enqueue 失败 → breached 标记回滚，下轮扫描重新命中（必坑 4，§4.10 决议） |
| TB16 | 发起人撤回 | 审批中撤回 → WITHDRAWING 栅栏 → 任务回收 → 工单终态正确（§4.10 决议 3） |

---

## 9. 迁移编号汇总

| 迁移 | 内容 | 所属能力 |
|------|------|---------|
| 000017 | `ticket_sla` + `sla_policies` | SLA |
| 000018 | `notifications` | 通知 |
| 000019 | `workflow_definitions` + `workflow_instances` + `workflow_node_states` + `workflow_records` + `workflow_tasks`（+ history） | 审批流（两层分离：BranchedStateEngine 内核 + ApprovalTaskLayer + 节点状态持久化） |
| 000020 | `assignment_rules` | 自动分派 |
| 000021 | ALTER `ticket_events` ADD `event_type` + `processed` + 部分索引 | 事件机制 L1 |

> 迁移规范遵循 [phase2/00-implementation-plan §5.3](../phase2/00-implementation-plan.md#53-迁移-pr-检查单每迁移必过) 检查单。
>
> **已前移到 Phase 2a**（2026-08-25）：`ticket_templates`（000015）和 `ticket_relations`（000016）因纯 DB 无事件依赖前移到 Phase 2a，见 [phase2/09-ticket.md §2](../phase2/09-ticket.md)。
