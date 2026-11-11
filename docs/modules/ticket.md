# 工单模块设计

> 2026-11-06 ECO-B08角标补齐：列表/导出另支持 `exclude_statuses=closed,rejected`，与status/statuses三者互斥，沿同样CSV校验与scope管线。用于S10非终态待办角标导航，包含自定义非终态；首页既有assigned/in_progress两状态卡仍用statuses，两种统计口径不互换。

> 2026-11-06 ECO-B08 待办筛选：列表与导出新增可选 `statuses=assigned,in_progress`（逗号分隔，至多32项，每项1–20字符，去重；与单值status互斥）。`assignee=me`同时限制处理人为当前用户；省略集合保留原查询。handler解析及repo谓词三路共用，不能前端分页后再滤。首页“我的待办”保持既有assigned+in_progress统计口径；S10角标的非终态统计是另一已存口径，此批不合并两种定义。

> 内部 IT/运维工单系统，作为框架的第一个业务模块，验证三层鉴权体系的资源级权限能力。
>
> **现况校准（2026-11-07，L-5）**：本文现行实现证据基线为 `996c167`（Phase5分支），不是main/生产部署声明。工单自研已在2026-10-09翻案，S1/S2/S3a/S5/S6/S10相应能力已落码，当前继续Phase5审计与修复；Phase6须待Phase5经授权合main后再从main切出。§5/§8明确标出的引擎Port与多级审批是历史设计，未据此宣称实现。阶段/债务SSOT分别为[11号](../review/11-project-control.md)、[Phase5台账](../phase5/s6-s5-debt-ledger.md)、[Phase6边界](../phase6/README.md)。
>
> 创建日期：2026-08-12

---


## 阅读入口与适用范围

先读§2的L1/L2/L3与scope边界，再看§3.4当前能力、§4实际入口和§6事件可靠性；§3早期DDL、§5/§8引擎原型均已标为历史，不是现行迁移或可调用接口。近期L-5校准的固定源码链接保留原证据基线；新HEAD/运行环境须再看[项目控制台](../review/11-project-control.md)及[最新followup](../review/reports/zhuzhao-report-followup-2026-11-08.md)。

当前新增校准仅涉及状态图示与outbox计划：单status邻接表支持多出边和回路，不支持多token并行/人工审批任务行；outbox当前未实现，但已是Phase9引擎的核心前置，不再只记无归属触发项。

## 1. 模块定位

### 职责边界

工单模块负责内部 IT 工单的全生命周期管理：提交、分派、处理、验证、关闭。它是框架的第一个业务资源模块，直接注册到 `ResourceRegistry`，作为资源级鉴权的典型实现。

### 与其他模块的关系

```
                    ┌──────────┐
                    │  ticket  │ ← 工单模块（本模块）
                    └────┬─────┘
         ┌───────────────┼───────────────┐
         ▼               ▼               ▼
   ┌──────────┐   ┌──────────┐   ┌──────────┐
   │   user   │   │   org    │   │  authz   │
   │ (创建人/  │   │ (组织路径 │   │ (Resource│
   │  处理人)  │   │  过滤)   │   │ Registry)│
   └──────────┘   └──────────┘   └──────────┘
         │               │
         ▼               ▼
   ┌──────────┐   ┌──────────┐
   │   role   │   │  audit   │
   │ (权限码) │   │ (操作记录)│
   └──────────┘   └──────────┘
```

- 依赖 `user`：获取创建人、处理人信息
- 依赖 `organization`：利用 `ltree` 组织路径做行级可见性过滤
- 依赖 `authz`：通过 `ResourceRegistry` 自注册资源级鉴权策略
- 依赖 `audit`：工单状态变更写入审计日志

---

## 2. 权限模型设计

### 2.1 业界参考

借鉴两个成熟系统的设计：

| 参考 | 借鉴点 |
|------|--------|
| [Freshdesk](https://support.freshdesk.com/support/solutions/articles/97079-understanding-ticket-scope-and-agent-role) | Role + Scope 双轴模型：角色控制"能做什么"，Scope 控制"能看到哪些" |
| [Jira Service Management](https://confluence.atlassian.com/adminjiraserver/customizing-jira-service-management-permissions-938847162.html) | Permission Scheme + Issue Security 两层：项目级权限 + 单条工单安全级别 |

### 2.2 三层鉴权在工单中的映射

当前执行顺序是认证 → **L1路由RBAC → L2可见性 → L3 `canOperate`**。L1由`menu_apis`关联的路径/方法策略裁决，permission用于菜单/按钮声明，不能把前端权限码检查替代后端Casbin。已登记selfService的cc/tags/存储等入口跳过业务路由enforce，仍须认证、有有效角色，并由handler/service执行相应资源检查。

当前非全局管理员的L2可见集为：**创建人/处理人 ∪ all scope ∪ 实体透明读锚点/group子树 ∪ 组织委托 ∪ 抄送**。列表`GetFilter`与单条`Authorize`共同表达这些分支；临时成员过期不再提供组织scope/委托来源。L2失败404，L2通过但动作拒绝403；未知动作默认拒绝。admin/superadmin为实现中显式全局豁免。

**演进差异**：2026-08旧稿曾写“转部门后路径不符即404，属主不豁免组织校验”。该强隔离叙述不符合当前实现：L2规则本身包含属主OR支，不能由“L2先执行”推断转部门会自动失去旧单可见性。这里记录既有实现与旧设计的差异，不改变鉴权，也不把文档差异直接判为越权。

证据：[internal/middleware/casbin.go 的 `CasbinAuth / SelfService`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/middleware/casbin.go)；[internal/service/ticket/resource.go 的 `Resource.Authorize / GetFilter / canOperate`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/resource.go)；[internal/service/ticket/scope_resolver.go 的 `pgxScopeResolver.resolve`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/scope_resolver.go)。

### 2.3 权限矩阵

| 操作 | L1入口 | L2/L3要点（非全局管理员） |
|---|---|---|
| 列表/详情/评论读取/导出 | 菜单GET绑定；完整策略按迁移及路由目录核对 | 列表/导出走同一scope过滤；详情走read |
| 创建 | 创建按钮POST绑定 | 创建归属组织检查，随后建单事务 |
| 更新 | 更新按钮POST绑定 | 创建人或相应组织委托；仅处理人不因此获得update |
| close/transition | 既有close按钮或已登记transition页面例外 | 创建人/处理人或组织委托；再叠加转换condition/validator |
| assign | 分派按钮POST绑定 | group/all主管或组织委托；不是普通属主自动放行 |
| delete | 删除按钮POST绑定 | 对应组织委托或全局管理员，不能简化成“仅admin” |
| comment/note | 对应按钮POST绑定 | comment=读可见；note=创建人/处理人或组织委托，并与内部备注读面一致 |
| cc/tags | 登录态selfService | 仅扩展自身/读取的操作按read；管理他人/修改按update检查，详各service |

导出不是独立`ticket:export`权限码，也不能写成“admin专属”；实际`/tickets/export`挂工单列表页面GET面。页面行仅GET的唯一登记例外为000068的transition POST。证据：[internal/router/router.go 的 `New`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/router/router.go)；[internal/service/ticket/resource.go 的 `Resource.canOperate`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/resource.go)；[internal/service/ticket/cc_service.go 的 `Service.AddCC / RemoveCC`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/cc_service.go)；[internal/service/ticket/tag_service.go 的 `Service.AddTags / RemoveTag`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/tag_service.go)；[迁移000065](../../migrations/000065_ticket_export_api.up.sql)、[000068](../../migrations/000068_transition_apis.up.sql)。

### 2.4 Ticket Scope 设计

三档scope是L2可见集的组织扩展轴，不替代属主/委托/抄送分支；实体透明读与group路径按并集合并：

| Scope | 含义 | 典型角色 | SQL 过滤逻辑 |
|-------|------|---------|-------------|
| `all` | 全部工单可见 | 仅有全局管理权限者可配置；仍需L1入口 | `Filter.Unscoped=true` |
| `group` | 本组织（含子组织）工单 | 一线处理人 | `org_path <@ 用户可见组织路径` |
| `assigned` | 仅本人创建或被分派的工单 | 外包、实习生 | `created_by = $userID OR assigned_to = $userID` |

#### Scope 的存储

`user_orgs.ticket_scope`已由000012落列，组织成员的scope可分别配置；`expires_at`过期关系不参与。下列DDL为Phase2设计摘要，不是应再次执行的迁移：

```sql
-- 用户在组织中的工单可见范围
-- 用户可在一个成员组织为group，在另一个为assigned；all写侧受全局管理权限限制
ALTER TABLE user_orgs ADD COLUMN ticket_scope VARCHAR(20) DEFAULT 'assigned';
-- 值: 'all' | 'group' | 'assigned'
```

#### 列表查询的行级过滤

**历史简化伪代码**：下例只保留scope轴的设计动机，不是当前签名/SQL。生产调用`Resource.GetFilter`，包含属主/委托/抄送，`TicketRepo`拒绝既无Where又未显式Unscoped的空过滤器；不得照抄此switch跳过守卫。

```go
// repository/ticket_repo.go
func (r *TicketRepo) List(ctx context.Context, filter TicketFilter) ([]Ticket, int64, error) {
    baseQuery := `SELECT ... FROM tickets WHERE 1=1`
    
    // 根据用户 scope 动态拼接 WHERE
    switch filter.Scope {
    case ScopeAll:
        // 全部工单，不加额外过滤
    case ScopeGroup:
        // 本组工单：用 ltree 的祖先匹配
        // org_path <@ 用户可见组织路径之一
        baseQuery += ` AND org_path ? $1::ltree[]`
    case ScopeAssigned:
        // 仅本人
        baseQuery += ` AND (created_by = $1 OR assigned_to = $1)`
    }
    
    // ...执行查询
}
```

#### 2.4.1 部门内读/写分离（策略 B，Phase 2b 默认）

同一**实体部门**下多个虚拟组（项目组）时，**不要**把「能看见」和「能改」绑在同一 ltree 路径上：

| 轴 | 规则（默认 `ticket_visibility=entity_transparent_read`） |
|----|--------------------------------------------------------|
| **L2 读** | 用户在实体子树内任一有 `user_orgs` → 可读该实体子树下全部工单（**含兄弟虚拟组**） |
| **L3 写** | **update 默认仅创建人**；close 为处理人/创建人；**工单所属虚拟组** admin/owner（2c）可管本组 |
| **强隔离** | 实体设 `project_isolated` → L2 回退为仅直接 org + `ticket_scope`（旧行为） |

实现 SSOT：[phase2/09-ticket.md §5.2](../phase2/09-ticket.md#52-phase-2b--scope-升级--部门内读写分离策略-b默认)、[03-org-enhance](../phase2/03-org-enhance.md#权限)。

---

## 3. 数据模型

### 3.1 工单类型配置表（初始设计投影）

借鉴 ECMDB 的模型驱动思路和 easy-workflow 的 JSON 模板配置，工单类型通过配置表管理，新增类型无需改Go代码。以下SQL保留2026-08设计草案投影，**不是迁移原文或完整现行DDL**；Phase5新增列/表见§3.4，迁移编号以11号地图为准：

```sql
-- 工单类型配置表（定义"有哪些工单类型"）
CREATE TABLE ticket_types (
    id              BIGSERIAL PRIMARY KEY,
    code            VARCHAR(50) UNIQUE NOT NULL,   -- incident, request, change, problem
    name            VARCHAR(100) NOT NULL,          -- "故障事件"、"服务请求"
    description     TEXT,
    states          JSONB NOT NULL DEFAULT '["open","assigned","in_progress","pending_verify","closed","rejected"]',
    transitions     JSONB NOT NULL DEFAULT '{"open":["assigned","closed"],...}',
    default_sla_hours INT DEFAULT 24,               -- 小时（类型级默认）；Phase 2a 仅配置存储，完整响应/解决SLA未实施；当前deadline见§3.4。单位口径见 §3.2.1
    has_custom_fields BOOLEAN DEFAULT FALSE,
    is_active       BOOLEAN DEFAULT TRUE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- 工单类型字段定义（类似 ECMDB 的 attribute 表，前端动态渲染表单）
CREATE TABLE ticket_type_fields (
    id              BIGSERIAL PRIMARY KEY,
    type_code       VARCHAR(50) NOT NULL REFERENCES ticket_types(code),
    field_key       VARCHAR(50) NOT NULL,       -- "affected_system", "impact_range"
    field_label     VARCHAR(100) NOT NULL,      -- "受影响系统", "影响范围"
    field_type      VARCHAR(20) NOT NULL,       -- text, number, select, date, textarea
    field_options   JSONB DEFAULT '[]',         -- select 类型的选项
    required        BOOLEAN DEFAULT FALSE,
    sort_order      INT DEFAULT 0,
    UNIQUE(type_code, field_key)
);
```

### 3.2 工单主表（初始设计投影）

所有类型共用一张表，`type_code` 区分类型，`custom_data` JSONB 存储自定义字段值：

```sql
CREATE TABLE tickets (
    id           BIGSERIAL PRIMARY KEY,
    type_code    VARCHAR(50) NOT NULL REFERENCES ticket_types(code),
    title        VARCHAR(200) NOT NULL,
    description  TEXT,
    priority     SMALLINT DEFAULT 3,          -- 1紧急 2高 3中 4低
    status       VARCHAR(20) DEFAULT 'open',  -- open, assigned, in_progress, pending_verify, closed, rejected
    created_by   BIGINT NOT NULL,             -- 提交人
    assigned_to  BIGINT,                      -- 处理人
    org_id       BIGINT NOT NULL,             -- 所属组织
    org_path     ltree NOT NULL,              -- 冗余组织路径（用于 ltree 查询）
    custom_data  JSONB DEFAULT '{}',          -- 自定义字段值（类似 ECMDB 的 mongox.MapStr）
    sla_due_at   TIMESTAMPTZ,                -- SLA 截止时间
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_tickets_type_status ON tickets (type_code, status);
CREATE INDEX idx_tickets_org_path    ON tickets USING GIST (org_path);
CREATE INDEX idx_tickets_status      ON tickets (status);
CREATE INDEX idx_tickets_assigned    ON tickets (assigned_to);
CREATE INDEX idx_tickets_created     ON tickets (created_by);
CREATE INDEX idx_tickets_org_status  ON tickets (org_id, status);

-- 工单回复/备注表
CREATE TABLE ticket_comments (
    id          BIGSERIAL PRIMARY KEY,
    ticket_id   BIGINT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    user_id     BIGINT NOT NULL,
    content     TEXT NOT NULL,
    is_internal BOOLEAN DEFAULT FALSE,       -- 内部备注 vs 公开回复
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_comments_ticket ON ticket_comments (ticket_id, created_at);

-- 工单事件日志（状态变更、分派等操作记录）
-- 当前仅业务审计记录，不是事件队列；旧event_type/processed轮询提案未实施
-- 初建表投影：实际000014已撤销ticket_id的ON DELETE CASCADE；存量字段以迁移为准
CREATE TABLE ticket_events (
    id          BIGSERIAL PRIMARY KEY,
    ticket_id   BIGINT REFERENCES tickets(id) ON DELETE SET NULL,
    user_id     BIGINT NOT NULL,
    action      VARCHAR(50) NOT NULL,        -- created, assigned, status_changed, closed, sla_breached
    from_value  VARCHAR(50),                 -- 变更前值
    to_value    VARCHAR(50),                 -- 变更后值
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_events_ticket ON ticket_events (ticket_id, created_at);
```

### 3.2.1 工单模板与关联表（Phase 2a 前移）

> **阶段归属（2026-08-25 锁定）**：工单模板、工单关联**无事件依赖、纯 DB**，已从 Phase 3 前移至 **Phase 2a**（迁移 `000015` / `000016`）。SSOT DDL 见 [phase2/09-ticket §2](../phase2/09-ticket.md#工单模板2a-前移迁移-000015)；此处为模块完整形态补录，两者必须一致。
>
> **SLA 单位口径**：`ticket_types.default_sla_hours` 以**小时**计（类型级默认）；`ticket_templates.default_sla_minutes` 以**分钟**计（模板级预填）。两者在Phase2a仅存储；当前SLA0.5使用状态元数据`transition_meta[state].sla_due_hours`，不能把旧类型/模板默认值视为已接入完整SLA策略，完整响应/解决/工作日历仍是后续能力。

```sql
-- 迁移 000015：工单模板（2a 前移，纯 DB）
CREATE TABLE IF NOT EXISTS ticket_templates (
    id                  BIGSERIAL PRIMARY KEY,
    code                VARCHAR(50) NOT NULL,
    name                VARCHAR(200) NOT NULL,
    type_code           VARCHAR(50) NOT NULL REFERENCES ticket_types(code),
    default_priority    SMALLINT DEFAULT 3,
    default_fields      JSONB DEFAULT '{}',
    default_sla_minutes INT,                      -- 分钟；历史预填配置，不代表当前SLA0.5取值源
    org_id              BIGINT NOT NULL REFERENCES organizations(id),
    org_path            ltree NOT NULL,
    created_by          BIGINT NOT NULL,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_ticket_templates_code ON ticket_templates(code) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ticket_templates_org_path ON ticket_templates USING GIST (org_path);

-- 迁移 000016：工单关联（2a 前移，纯 DB）
CREATE TABLE IF NOT EXISTS ticket_relations (
    id                BIGSERIAL PRIMARY KEY,
    source_ticket_id  BIGINT NOT NULL REFERENCES tickets(id),
    target_ticket_id  BIGINT NOT NULL REFERENCES tickets(id),
    relation_type     VARCHAR(30) NOT NULL DEFAULT 'related',  -- related/blocks/duplicates/split
    created_by        BIGINT NOT NULL,
    created_at        TIMESTAMPTZ DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ,
    CHECK (source_ticket_id <> target_ticket_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_ticket_relations_pair ON ticket_relations(source_ticket_id, target_ticket_id, relation_type) WHERE deleted_at IS NULL;
-- 2026-09-11 并发批次：上索引为方向性判重（并发 A→B / B→A 双双入库），已被迁移 000028 的
-- 规范化对唯一索引 uq_ticket_relations_normalized(LEAST,GREATEST,relation_type) WHERE deleted_at IS NULL 取代（建索引前先软删历史双向重复行）。
CREATE INDEX IF NOT EXISTS idx_ticket_relations_target ON ticket_relations(target_ticket_id) WHERE deleted_at IS NULL;
```

### 3.3 状态机

当前`StateMachine`从`ticket_types.transitions`邻接表校验边；每张工单只有一个当前status，支持配置的不同后继与回路，不等于多token并行/会签工作流。下面保留Phase2默认示例，实际边以该类型配置为准；Phase5可视化状态图编辑器保存的仍是同一组states/transitions及meta。

原ASCII简图曾把closed回退箭头画到rejected，与下方邻接表不符，现以邻接表为唯一示例：**closed/rejected均可回open，pending_verify可回in_progress**。这只说明默认配置边，不代表任意用户均可触发；实际还须经过L1/L2/L3与condition/validator。

合法状态转换定义：

```go
var ticketTransitions = map[string][]string{
    "open":           {"assigned", "closed"},
    "assigned":       {"in_progress", "open"},
    "in_progress":    {"pending_verify", "rejected", "closed"},
    "pending_verify": {"closed", "in_progress"},
    "closed":         {"open"},         // reopen
    "rejected":       {"open"},         // reopen
}
```

---

### 3.4 Phase5数据面与能力边界

| 当前数据/能力 | 已落实现 | 边界 |
|---|---|---|
| 类型与字段 | `TicketType.TransitionMeta/Version`，字段`FieldMeta/ValidateRegex`；000018/000027/000051/000064 | `field_meta`承载公式/联动/可见性等配置；后端`validateCustomData`做整体校验，UI不独自决定合法性 |
| 单据 | `human_key`、`status_changed_at`、`escalation_supervisor_id`、`sla_due_at` | 无workflow instance/task审批表；工单主表不因此具有通用version CAS |
| 转换语义 | `transition_meta["from>to"]`的roles/validator/post_actions | 与`transition_meta[state]`超时配置共列，保存期校验键域；不等同审批引擎；图坐标仅UI localStorage视图偏好，不存在FlowLayout数据库列 |
| 自动分派 | 000069 `assignment_rules`，类型+大小写敏感标题字面包含，priority/id升序首命中 | 创建提交后执行；配置授权的系统分派通道，归因保留规则创建者；无命中/仅org规则保持open并发dispatch_missed，失败不回滚创建 |
| SLA0.5/催办 | 状态级`timeout_minutes/escalation/sla_due_hours`，000064 `ticket_reminders` | `Transition`同事务设置/清理due；扫描器发timeout/deadline事件。没有响应/解决双时钟、暂停态、工作日历完整SLA |
| 存储附件 | 000045–000049对象元数据/附件关联/owner与GC，独立storage service | presign→直传→confirm；头像/附件/富文本按object_key寻址，ticket态额外L2；对象存储不是DB事务的一部分 |
| 协作 | 000062抄送、000063标签、badges、导出 | cc扩读不扩写；标签不是DomainEvent生产者；待办集合/非终态集合口径不互换 |
| 消息 | message_templates/rules/log、notifications与optouts，S6两期 | inbox真实落库；外发UIP仍为两态桩，不能称邮件/IM已真实接线 |

证据：[internal/model/ticket.go 的 `Ticket / TicketType / TicketTypeField`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/model/ticket.go)；[internal/service/ticket/type_admin.go 的 `Service.validateCustomData / validateTypeGraph`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/type_admin.go)；[internal/service/ticket/transition_service.go 的 `Service.Transition`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/transition_service.go)；[internal/repository/assignment_rule_repo.go 的 `AssignmentRuleRepo.MatchOnCreate`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/repository/assignment_rule_repo.go)；[internal/service/ticket/dispatch_service.go 的 `DispatchService.OnAfterCreate`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/dispatch_service.go)；[internal/service/ticket/timeout_scanner.go 的 `TimeoutScanner.Handle`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/timeout_scanner.go)；[internal/handler/storage_handler.go 的 `StorageHandler.Confirm / BindAvatar / ListAttachments`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/handler/storage_handler.go)；[internal/service/ticket/inbox_service.go 的 `InboxService.OnDomainEvent`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/inbox_service.go)。

---

## 4. 接口定义

### 4.1 API 路由（遵循 GET + POST 规范）

下表为主链路摘要，不是完整路由目录；L1一律按§2.3与menu_apis实际绑定核对，表内旧权限名仅表示动作意图。完整注册见`internal/router/router.go`的`New`。

| 方法 | 路径 | 权限码 | 说明 |
|------|------|--------|------|
| GET | `/api/v1/tickets` | `ticket:list` | 工单列表（按 scope 过滤） |
| POST | `/api/v1/tickets` | `ticket:create` | 创建工单 |
| GET | `/api/v1/tickets/:id` | 工单页面GET面 | 工单详情 |
| POST | `/api/v1/tickets/update` | `ticket:update` | 更新工单（patch：title/description/priority/custom_data 增量[键覆盖·null 删键·G2 整体重校验]/attachments 增量落绑——B5-4） |
| POST | `/api/v1/tickets/richtext/render` | 登录态（自服务） | richtext 服务端渲染/消毒（B5-4——白名单 HTML 出站，无票据访问面） |
| POST | `/api/v1/tickets/close` | `ticket:close` | 关闭工单 |
| POST | `/api/v1/tickets/assign` | `ticket:assign` | 分派工单 |
| POST | `/api/v1/tickets/delete` | `ticket:delete` | 删除工单 |
| GET | `/api/v1/tickets/:id/comments` | 工单页面GET面 | 回复列表 |
| POST | `/api/v1/tickets/comments` | `ticket:comment` | 添加回复 |
| POST | `/api/v1/tickets/notes` | `ticket:note` | 添加内部备注 |
| POST | `/api/v1/tickets/cc` | 登录态（L2/L3 动态） | 添加抄送（S10 B10-1：仅本人=读可见；含他人=update 权；幂等；emit ticket.cc_added→S6 规则路由通知） |
| POST | `/api/v1/tickets/cc/remove` | 登录态（L2/L3 动态） | 移除抄送（自摘=读可见；摘他人=update 权；幂等） |
| GET | `/api/v1/tickets/:id/cc` | 登录态（L2 read 面） | 抄送列表（「人名(工号)」富化；不可见单 404 同形） |

补充现行面：`POST /tickets/transition`、`GET /tickets/:id/transitions/available`；`GET /tickets/export`；relations；tags；badges/inbox；storage附件及files代理；类型/字段/模板管理与`assignment-rules`。`GET /ticket-types/:code`没有注册，不能按旧动态表单示例调用；类型详情配置由实际列表/管理响应承载。

### 4.2 Service接口边界

实际为`internal/service/ticket.Service`具体类型，而非旧稿虚构的`TicketService`接口。主链路包括`Create/Get/List/Update/Close/Assign/Delete/CreateComment/CreateNote/CreateRelation/ListRelations`，转换走`Transition/ListAvailableTransitions`，配置/协作分别放同包的type_admin/cc/tag文件。请求和actor参数以具体方法签名为准，不复制另一套接口。

证据：[internal/service/ticket/service.go 的 `Service / NewTicketService`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/service.go)；[internal/service/ticket/transition_service.go 的 `Service.Transition / ListAvailableTransitions`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/transition_service.go)。

### 4.3 ResourceRegistry 注册

> **实现 SSOT**：[phase2/02-authz-resource.md §TicketResource](../phase2/02-authz-resource.md) + [phase2/09-ticket.md §5](../phase2/09-ticket.md)。`modules/ticket.md` 不再内联注册代码，以免与 SSOT 接口漂移。

工单模块在 Phase 2a 通过实现 `Resource` 接口（`Authorize` / `GetFilter`）自注册到 `ResourceRegistry`，见上述 SSOT。

---

## 5. 工单类型抽象与 Hook 机制

**当前落点**：统一`ticket.Service`+类型/字段配置；领域通知使用`DomainHook`，自动分派由`DispatchService.OnAfterCreate`显式调用，动作由`ActionRegistry`执行。下述`TicketHooks/TicketManager/TicketEngine`代码均为2026-08历史原型，仓内没有这些实现，不能作为可调用接口；现行语义见§3.4/§6。

### 5.1 设计方案：统一接口 + 配置驱动 + Hook 扩展（历史设计，未按此落码）

参考 ECMDB 的模型驱动资产管理（模型 → 属性 → 实例）和 easy-workflow 的 JSON 流程模板配置，采用混合方案：

| 层 | 职责 | 实现方式 |
|---|------|---------|
| 统一接口 | 所有工单类型共用一套 API 和 Service | `TicketService` 接口 |
| 配置驱动 | 新增工单类型只需配置，不改代码 | `ticket_types` + `ticket_type_fields` 表 |
| Hook 扩展 | 需要类型特定逻辑时通过钩子扩展 | `TicketHooks` 接口 |

三种方案对比：

| 方案 | 优点 | 缺点 |
|------|------|------|
| 纯接口抽象（每种类型一个 Service 实现） | 类型隔离清晰 | 类型膨胀时重复代码多；新增类型必须改代码 |
| 纯前端配置（一张表 + JSON） | 零代码新增类型 | 无法实现类型特定业务逻辑 |
| **混合方案（统一表 + 配置 + Hook）** | 新增类型只需配置；需要定制逻辑时用 Hook | 需要 Hook 机制设计 |

### 5.2 Hook 接口定义（历史设计，未按此落码）

借鉴 easy-workflow 的事件系统（NodeStartEvents/NodeEndEvents/TaskFinishEvents），设计工单生命周期钩子：

```go
// TicketHooks 工单生命周期钩子（可选实现）
type TicketHooks interface {
    OnBeforeCreate(ctx context.Context, ticket *Ticket) error
    OnAfterCreate(ctx context.Context, ticket *Ticket) error
    OnBeforeTransition(ctx context.Context, ticket *Ticket, targetStatus string) error
    OnAfterTransition(ctx context.Context, ticket *Ticket, oldStatus string) error
    OnAfterClose(ctx context.Context, ticket *Ticket) error
}

// DefaultTicketHooks 默认 Hook（空实现，所有类型共用）
type DefaultTicketHooks struct{}

func (h *DefaultTicketHooks) OnBeforeCreate(ctx context.Context, t *Ticket) error    { return nil }
func (h *DefaultTicketHooks) OnAfterCreate(ctx context.Context, t *Ticket) error     { return nil }
func (h *DefaultTicketHooks) OnBeforeTransition(ctx context.Context, t *Ticket, s string) error { return nil }
func (h *DefaultTicketHooks) OnAfterTransition(ctx context.Context, t *Ticket, old string) error { return nil }
func (h *DefaultTicketHooks) OnAfterClose(ctx context.Context, t *Ticket) error      { return nil }
```

### 5.3 类型注册管理（历史设计，未按此落码）

```go
// TicketManager 管理工单类型和对应的 Hooks
type TicketManager struct {
    types map[string]TicketType
    hooks map[string]TicketHooks
}

// Register 注册工单类型及其 Hooks
// 未注册 Hooks 的类型使用 DefaultTicketHooks
func (m *TicketManager) Register(typeCode string, hooks TicketHooks) {
    if hooks == nil {
        hooks = &DefaultTicketHooks{}
    }
    m.hooks[typeCode] = hooks
}

// GetHooks 获取类型对应的 Hooks
func (m *TicketManager) GetHooks(typeCode string) TicketHooks {
    if h, ok := m.hooks[typeCode]; ok {
        return h
    }
    return &DefaultTicketHooks{}
}
```

### 5.4 Hook 使用示例（历史设计，未按此落码）

```go
// 历史原型：当时拟在Phase3审批场景注册，当前未实现该接口/调用
type ChangeTicketHooks struct {
    DefaultTicketHooks
    workflowEngine WorkflowEngine
}

func (h *ChangeTicketHooks) OnAfterCreate(ctx context.Context, ticket *Ticket) error {
    // 变更类工单创建后自动启动审批流
    return h.workflowEngine.Start(ctx, "change_approval", ticket.ID)
}

// Wire 注入时注册
func NewTicketManager() *TicketManager {
    m := &TicketManager{
        types: make(map[string]TicketType),
        hooks: make(map[string]TicketHooks),
    }
    // Phase 2a: 所有类型使用默认 Hooks
    // 历史计划: 注册变更类工单的特殊Hooks，当前未落码
    // m.Register("change", &ChangeTicketHooks{...})
    return m
}
```

### 5.5 前端动态表单

前端通过 API 获取工单类型的字段定义，动态渲染表单：

| API | 说明 |
|-----|------|
| `GET /api/v1/ticket-types` | 类型列表响应携带该类型配置；未注册单类型GET |
| `GET /api/v1/ticket-types/:code/fields` | 获取类型字段定义（前端动态渲染表单） |

<a id="56-ticketengine-port引擎可替换边界"></a>
### 5.6 TicketEngine Port（引擎可替换边界）（历史设计，未按此落码）

> **目的**：锁定工单流程引擎的可替换边界。当时拟让本地状态机/分支引擎/远程引擎共用Port；当前没有此Port，实际仍是`StateMachine`与同包`Service`。审批语义属S3b等后续设计，不能因S2状态图已交付而宣称存在此接口。

```go
// TicketEngine 工单流程引擎 Port（实现可替换，鉴权不在此层）
type TicketEngine interface {
    // Submit 提交工单到流程，返回流程实例 ID
    // idempotencyKey 用于远程引擎幂等重试；本地实现可忽略
    Submit(ctx context.Context, ticketID int64, typeCode string, idempotencyKey string) (instanceID string, err error)

    // Trigger 触发动作（approve/reject/reassign/...），返回新状态与待办
    Trigger(ctx context.Context, instanceID, action string, actor int64, idempotencyKey string) (newState string, tasks []Task, err error)

    // Tasks 查询实例的当前待办（供 L3 属主校验消费）
    Tasks(ctx context.Context, instanceID string) ([]Task, error)

    // GetState 远程引擎网络丢包后主动反查状态（本地实现直接读 DB）
    GetState(ctx context.Context, instanceID string) (state string, err error)

    // Cancel 工单本地关闭/删除时终止流程实例
    Cancel(ctx context.Context, instanceID, reason string) error
}

// Task 引擎产出的待办，带审批条件建议，供 L2/L3 校验
//
// 关键边界（对齐 aifei-go flow/workflow.StateController 设计，2026-08-25 修订）：
// 引擎不持有用户身份、不理解 org_path / ticket_visibility / ReadAnchorPaths。
// 因此引擎只产出"任职条件"（ApprovalRequirement，JSONB），不产出具体用户 ID 列表。
// 具体"当前用户能否认领"由 ApprovalTaskLayer 调 StateController（内部走 L2/L3）裁决。
type Task struct {
    ID         string
    NodeCode   string
    Requirement json.RawMessage // 审批任职条件（JSONB，见 ApprovalRequirement 说明），L2/L3 据此裁决
}

// ApprovalRequirement —— "该节点由谁审批"的条件对象（**不是具体用户**），
// 存于 workflow_definitions.definition 节点的 meta，引擎只透传、不解析。
//
// 基础语义（角色 + 部门/组织范围，天然支持"按角色、按部门审批"）：
//   {"role":"group_admin","org_scope":"ticket_org"}  → 工单所属组织的任一 admin
//   {"role":"owner","org_scope":"ancestor"}          → 工单祖先组织的任一 owner
//   {"role":"assignee"}                              → 当前处理人
//   {"role":"creator"}                               → 工单创建人
//
// 进阶语义（JSONB 可扩展，引擎零改动）：
//   - 职级：{"role":"dept_head","org_scope":"ticket_org","min_level":3}  → 部门主管且职级≥3
//   - OR 组合：{"any":[{"role":"dept_head","org_scope":"ticket_org","min_level":3},
//                      {"role":"hrbp","org_scope":"ancestor"}]}            → 主管(≥3级) 或 HRBP
//   - 指定人（兜底）：{"user_id":12345}
//
// role 取值：group_admin | owner | assignee | creator | dept_head | hrbp | custom_role | user_id
// org_scope 取值：ticket_org（工单所属组织）| ancestor（祖先组织）| self（仅本人）
//
// StateController（走 CanApproveNode）负责解析比对，引擎内核不感知任何鉴权概念。
// 借鉴 aifei-go flow/workflow.StateController.IsOperatable(ctx, node) 的"节点 meta 匹配"思想。
type ApprovalRequirement = map[string]interface{}

// StateController 决定"当前用户能否操作某节点"（人工任务层）。
// 实现委托给现有 L2/L3 属主规则（CanApproveNode），引擎内核不感知鉴权概念。
// 灵感来自 aifei-go flow/workflow.StateController.IsOperatable(ctx, node)。
type StateController interface {
    // IsOperatable 报告当前用户（ctx 中的 actor）能否操作该节点的待办。
    IsOperatable(ctx context.Context, ticketID int64, req json.RawMessage) (bool, error)
    // IsAutoForward 报告该节点是否自动推进（无需人，如网关节点）。
    IsAutoForward(req json.RawMessage) bool
}
```

**三种实现**：

| 实现 | 阶段 | 部署形态 | 说明 |
|---|---|---|---|
| `LinearStateEngine`（未实现此类型名；实物为StateMachine） | 历史原型 | 进程内 | `map[string][]string` 加载自 `ticket_types.transitions`，instanceID = ticketID |
| `BranchedStateEngine`（未实现） | 后续审批设计 | 进程内 | 手写 Node/Gateway 模型，支持分支/会签，借鉴 easy-workflow 设计不借代码 |
| `RemoteHTTPWorkflowEngine` | 未来按需 | 独立部署第三方引擎 | HTTP 对接，需满足 §8.5 集成契约 |

**关键边界——鉴权不进 Port**：

`TicketEngine` 只负责"状态怎么走、产生哪些待办"，**不做任何 L1/L2/L3 判断**。所有鉴权仍在 `TicketService`/`ResourceRegistry` 层：

- 引擎产出的 `Task` 只带 `ApprovalRequirement`（审批条件），只是**建议**，最终能不能操作由 L2/L3 属主规则裁决
- 引擎不持有用户身份，所有 `actor` 都是本地 user_id
- 引擎不知道 `org_path`/`ticket_visibility`/`ReadAnchorPaths` 等鉴权概念

这样无论将来换成什么引擎，鉴权体系都不用动——这是本项目拟保持的引擎替换边界，不能外推为所有产品唯一实现方式。远程引擎的完整集成契约见 §8.5。

---

## 6. 事件驱动集成（概要）

工单业务日志`ticket_events`与进程内`DomainEvent`是两条不同机制。业务代码在相应用例事务内写审计事件；事务提交后的`emitTicketEvent`才调用`HookRegistry.Dispatch`。日志表没有event_type/processed轮询消费者，**不能用审计记录存在推导通知可重放**；统一转换等路径的具体落日志范围以各方法实现及挂账为准。

当前事件包括created/assigned/updated/commented/closed/cc_added/timeout/transitioned/dispatch_missed/action_notify/deadline；这是`domainevent`词表，不等于每个动作都必定发送、所有订阅者都有规则。旧status_changed/sla_breached/approved/rejected表不是现行DomainEvent词表。

`HookRegistry`按biz_type分发，每订阅者独立goroutine并隔离panic；**at-most-once**，没有Outbox和自动重放。计划内退出先停止来源，再`Drain`等待在途订阅者（App预算8s）；进程崩溃、commit至dispatch间隙或排空超时仍可能丢通知，不能称可靠投递。

当前两个消费面：`TicketNotifyHook → MessageDispatcher`按rules/模板/偏好/节流/通道发送并记message_log；`InboxService`独立订阅并落notifications。inbox落库是消费者自身持久化，不是事件源持久化；外发UIP桩成功也不表示真实邮件/IM送达。

| 历史方案 | 当前裁定 |
|---|---|
| L0 channel | 早期设想，Phase2a未实现当时描述的分发；当前使用HookRegistry goroutine |
| L1 ticket_events轮询+processed+分布式锁 | ADR历史备选，未落码，不是Phase5稳态 |
| L2 Outbox/Asynq事件消费者 | 未实施。C-20已按既有决策归Phase9引擎前置（可随引擎提前而前移，不能后滑）；taskrunner已用Asynq执行任务，不等于主仓领域事件已持久化 |

未来风险联动不能继承“通知可以丢”的容忍度；Phase6 R3规划终态对账，Outbox已在[Phase9核心⑦](../phase9/README.md)规划为引擎前置，若引擎提前须同批或更早落地；当前仍未实施。本批不实现或承诺这些能力。

证据：[internal/pkg/domainevent/event.go 的 `DomainEvent / Action`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/pkg/domainevent/event.go)；[internal/pkg/domainevent/registry.go 的 `HookRegistry.Dispatch / Drain`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/pkg/domainevent/registry.go)；[internal/service/ticket/service.go 的 `Service.emitTicketEvent`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/service.go)；[internal/service/ticket/dispatcher.go 的 `MessageDispatcher.Dispatch`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/dispatcher.go)；[internal/service/ticket/inbox_service.go 的 `InboxService.SetEventRegistry / OnDomainEvent`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/inbox_service.go)；[internal/app/app.go 的 `App.Shutdown`（`996c167`）](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/app/app.go)；历史依据保留[ADR-001](../adr/adr-001-event-mechanism-l1-steady-state.md)/[ADR-002](../adr/adr-002-asynq-async-task-executor.md)。

## 7. 分阶段实施

| 时点/阶段 | 演进事实 | 当前边界 |
|---|---|---|
| Phase1 | IAM、登录/角色/组织/菜单与审计底座 | 不代表当时已有资源工单/审批引擎 |
| Phase2a | 工单CRUD、状态邻接表、公开回复/内部备注、事件日志；模板/关联以000015/16前移 | 当时没有TicketHooks或L0 channel；保留历史虚标勘误 |
| Phase2b/2c | scope、透明读/强隔离配置、组织角色继承与委托 | 附件/HR早期排期不等于交付：附件后来由Phase5 S5承接，HR真实接入仍未落地 |
| 2026-08-25暂停/2026-09-02封版 | 当时工单深化暂缓，旧SLA/审批/Asynq方案留设计 | 历史决策，不能继续描述为当前Phase3整体暂停 |
| Phase3/4底座与生态 | 生产横切能力按实际切片推进；taskrunner/AL网关与前端控制台形成 | 不把历史Phase3愿望清单全部视为实现；当前组件边界见architecture.md |
| 2026-10-09翻案→Phase5 | 自研工单主线恢复；S1字段/表单、S2状态图、S3a转换/分派/动作、S5富媒体、S6消息、S10协作/超时等落码 | 审批S3b、完整SLA、通用报表/结果映射等不因此完成；S7/S8等按阶段稿的概要/触发边界判断 |
| 当前Phase5审计修复 | 仍在phase5分支收口设计/代码/门禁与债务，main未合 | 以11号最新时点与各批固定验证报告为准，不将代码落地等同生产部署 |
| Phase6+ | 风险R1–R4、工单完善06号、挂账索引07号；更远阶段为路线规划 | **先Phase5收口→经授权合main→从main切Phase6**；不把已批准规划当作当前API/表/任务 |

历史设计入口：[Phase2工单](../phase2/09-ticket.md)、[Phase3业务深化旧设计](../phase3/10-ticket-business.md)；现行实现/规划分别对齐[Phase5](../phase5/README.md)、[挂账台账](../phase5/s6-s5-debt-ledger.md)、[Phase6 README §5边界与07号索引](../phase6/README.md)。

## 8. 关键设计决策

§8.1–8.6保留2026-08的引擎调研/替换契约，勾号表示当时采纳的设计方向，**不是实现验收**。当前Phase5仍为单status状态图+转换语义，无BranchedStateEngine/ApprovalTaskLayer/远程引擎。S2可视化已经交付，不再单独构成“须引外部引擎”的信号。

### 8.1 为什么不用现成工单产品

| 方案 | 否决理由 |
|------|----------|
| Zammad | Ruby 栈，4GB+ RAM，独立用户体系，无法共享 IAM |
| FreeScout / GLPI | PHP 栈，需另起运行时 |
| go-help-desk | 历史调研时生产验证不足、许可证不适配 |
| escalated-go | 历史调研未满足审批流/工作流需求 |
| easy-workflow | 功能优秀（会签、自由驳回、混合网关），**MIT 许可**，可作设计参考蓝图。否决的是"移植代码进我们的 Go 进程"（`MySQL 8 CTE` 方言 + GORM + 全局单例引擎，与 PG + pgx + Wire 存在近重写成本，非绝对"不兼容"——GORM 本身支持 PG），不是"Web API Server 独立部署"模式——后者作为 Phase 3+ 备选见 §8.5。注：原官方仓库 `qunarcorp/easy-workflow` 已不可访问（404），活跃分支为社区版 `Bunny3th/easy-workflow`（历史调研分支，当前活跃度未在本批复验）；其权限模型**外置**（角色解析在业务侧事件回调），不存在"鉴权耦合" |
| ECMDB/EFlow | 架构优秀（Wire DI + 模型驱动 + 插件化），但 MongoDB + Kafka 技术栈不匹配，且 EFlow 工单系统未开源 |

自研的核心优势：工单本身是一种"资源"，直接纳入框架已设计好的三层鉴权体系。

### 8.2 从 easy-workflow 借鉴什么（完整能力映射）

easy-workflow（MIT 许可）作为自写 `BranchedStateEngine` 的设计参考蓝图。**借的是设计，不是代码**——用 pgx 重写，避开它的三个实现局限。完整能力映射：

| easy-workflow 能力 | 历史调研设计要点 | 历史自研拟案（未落地） |
|------|---------|---------|
| **4 种节点类型** | Root(0)/Task(1)/Gateway(2)/End(3)，极简分类 | ✅ 直接复用，`workflow_definitions.definition` JSONB 存 Node 列表 |
| **混合网关** | 三字段组合：`Conditions`（排他分支）+ `InevitableNodes`（并行分发）+ `WaitForAllPrevNode`（0=包含/1=并行汇合） | ⚠️ **收窄为借鉴参考**：网关统一采用 aifei-go flow 的 `NodeType` 枚举（Exclusive/Inclusive/Parallel/Loop）表达，比 easy-workflow 三字段更标准、PG JSONB 友好、易持久化。easy-workflow 三字段仅作语义参考（会签 BatchCode + 自由驳回 BFS 仍借鉴），不照搬字段结构 |
| **会签** | `IsCosigned` 字段（0=任一通过/1=全部通过）+ `BatchCode` 批次码区分多轮驳回 | ✅ 保留 BatchCode 设计，解决"同节点多次驳回产生多批任务"的状态隔离 |
| **自由驳回** | `TaskFreeRejectToUpstreamNode` + CTE 递归查上游链 | ⚠️ 用邻接表 + BFS 替代 CTE（**摆脱 MySQL 依赖**，PG 原生支持） |
| **直接提交到上次驳回我的节点** | `TaskPass` 第4参数 `DirectlyToWhoRejectedMe` | ✅ 记录驳回历史即可 |
| **4 类事件** | NodeStart/NodeEnd/TaskFinish/Revoke，反射注册 | ⚠️ 用显式 `TicketHooks` 接口替代反射（**编译期检查**），保留 4 类事件时机划分 |
| **变量系统** | `$` 前缀变量 + `InstanceVariablesSave` + 表达式求值下放 DB | ⚠️ JSONB 存变量，Go 侧求值（`govaluate` 库或自写，**不交给 DB**——避免 SQL 注入且可单测） |
| **TaskAction 自描述** | `WhatCanIDo` 返回可执行操作集合 | ✅ 保留，前端按钮直接驱动 |
| **双表设计** | 5 运行表 + 5 历史表，冗余字段便于查询 | ✅ 运行表（`workflow_instances`/`workflow_tasks`）+ 历史归档，冗余 `ticket_id` 便于关联查询 |
| **计划任务** | `ScheduleTask` + 进程内 `ScheduledTaskPool` | 历史曾拟Asynq PeriodicTask；当前主仓内务用CronScheduler，任务执行在taskrunner，不实施旧L1轮询 |

**自写时必须避开的三个 easy-workflow 局限**：

1. **MySQL CTE 依赖** → 自写用 PG 的 `WITH RECURSIVE` 或邻接表 + BFS，不绑死数据库
2. **表达式求值下放 DB**（`ExpressionEvaluator` 拼 SQL 让 MySQL 算）→ Go 侧求值，避免 SQL 注入且可单元测试
3. **反射事件注册**（`RegisterEvents` 无编译期检查）→ 显式 `TicketHooks` 接口，编译期保证方法签名正确

> **Phase2a已落地的借鉴**：`ticket_types.transitions` JSONB。`TicketHooks`、workflow运行/历史双表没有落地；ticket_events审计表不能冒充workflow历史表。当前领域事件另见§6。
>
> **历史审批蓝图（未实现，后续S3b另评估）**：混合网关三字段模型、会签+BatchCode、自由驳回（BFS替代CTE）、TaskAction自描述。见 [phase3/10-ticket-business.md §4](../phase3/10-ticket-business.md#4-多级审批流branchedstateengine)。

### 8.3 从 ECMDB 借鉴什么

| 设计 | 借鉴方式 |
|------|---------|
| 模型驱动（Model → Attribute → Resource） | 工单类型驱动（TicketType → TicketTypeField → Ticket） |
| 自定义字段用动态存储 | `tickets.custom_data` JSONB（类似 ECMDB 的 `mongox.MapStr`） |
| 插件化资源动作 | 原拟TicketHooks；当前为显式领域事件/ActionRegistry，见§5/§6 |
| Wire依赖装配 | 当前通过app/wire.go显式注入；不据此宣称分域Provider Set已拆完 |

<a id="84-为什么-phase-2a-不引入工作流引擎"></a>
### 8.4 为什么Phase2a不引入工作流引擎（历史升级方案）

历史稿称“线性”不精确：Phase2a也是单status状态图，默认配置含分支和回路，一个邻接map能表达；它缺的是并行token/会签任务行，不是任何分支都不支持。引入 Temporal 等工作流引擎属于过度设计。

**2026-08历史升级路径（当前未实施）**（[phase3/10-ticket-business.md](../phase3/10-ticket-business.md)）：当时拟在变更类工单出现多级审批/会签场景时首选自研`BranchedStateEngine`，注册到`TicketEngine` Port。两个类型都尚未实现；后续S3b须重新核场景与实施边界，不能把该设想当Phase5现况。

评估过的外部方案：
- Temporal（工业级，需独立 server）——过重，Phase 3 不引
- 移植 easy-workflow 源码（`MySQL 8 CTE` 方言 + GORM + 全局单例引擎）——**近重写**：GORM→pgx 重写持久层、MySQL→PG 迁移、全局状态→Wire DI 去全局化，粗估 2000-4000 行。注意：easy-workflow 的权限模型是**外置的**（通过 `NodeStartEvents` 让业务侧解析角色），不存在"鉴权耦合"问题；否决理由纯粹是技术栈近重写成本（官方原版 `qunarcorp/easy-workflow` 已 404，活跃分支 `Bunny3th/easy-workflow` MIT）。服务化集成场景的鉴权适配见 §8.5
- **easy-workflow Web API Server 独立部署**（MIT 许可，可行但有代价）——见下方对比，作为 Phase 3 备选
- 第三方闭源引擎服务化集成——见 §8.5 集成契约

**历史比较的两个备选方案**（后续须重评估，未构成当前批准排期）：

| 维度 | 选项 A：自写 BranchedStateEngine（首选） | 选项 B：easy-workflow Web API Server（备选） |
|---|---|---|
| 开发量 | 1500-2500 行 Go（两层分离：状态机内核 + 人工任务层，借鉴 §8.2 + aifei-go flow 结构） | 200-300 行 Adapter + HTTP client |
| 数据库 | 共用 PG | 新增 MySQL（双库） |
| 鉴权 | 天然在 TicketService 层（StateController 委托 L2/L3，引擎不持有用户身份） | 需禁用引擎的 claim/分派 API（见 §8.5） |
| 一致性 | 单库事务 | Saga + 对账（双库） |
| 运维 | 零新增 | MySQL + easy-workflow 服务 |
| 功能 | 需自实现会签/网关/自由驳回（采用 aifei-go flow 两层分离 + NodeType 网关，蓝图清晰） | 现成（会签/网关/自由驳回/计划任务） |
| 许可证 | 自研 | MIT，无障碍 |
| 风险 | 自写复杂度可控（有 §8.2 easy-workflow 蓝图 + aifei-go flow 参考两层分离结构） | 双库一致性 + 鉴权适配层 |

**切换信号**：选项 A 首选开始；若出现"会签/网关需求复杂度超出预期（>3 种分支模式）"或"团队无带宽自写"或当时设想的"需要可视化流程编辑器"（该信号已由现行S2自研图编辑器解决，不再单独触发替换），切到选项 B。

### 8.5 第三方/闭源工作流引擎集成契约

若未来出现需要引入第三方/闭源工作流引擎（独立部署、HTTP 对接）的场景，必须遵守以下契约。**这是硬过滤条件**——违反任一条的引擎直接 pass，不管功能多强：

| 引擎能力 | 用/禁 | 说明 |
|---|---|---|
| 状态机定义/流转 | ✅ 用 | 引引擎的价值 |
| 自动网关/会签任务行 | ✅ 未来按契约用 | 当前单status图可有人工分支，但无多token会签任务行 |
| 产出审批条件（ApprovalRequirement） | ✅ 用 | 只作**建议**，传给我们，L2/L3 裁决 |
| **claim/complete 任务** | ❌ **禁用** | 必须改走我们的 `POST /api/v1/tickets/<动作>（ID放body）` 走三层鉴权 |
| **用户体系** | ❌ **禁用** | 引擎不能持有用户身份，所有 actor 都是我们映射过去的本地 user_id |
| **权限判断** | ❌ **禁用** | 引擎不知道 `org_path`/`ticket_visibility`/`ReadAnchorPaths`，这些概念见 [09-ticket §5.2](../phase2/09-ticket.md#52-phase-2b--scope-升级--部门内读写分离策略-b默认) |

**集成边界**：引擎只是"状态机 + 审批条件建议器"。所有"用户能不能 claim/complete"的判断，必须在 `TicketService` 走 L1/L2/L3（见 §2.2）。引擎产出的 `Task` 只带 `ApprovalRequirement`（审批任职条件，如 `approver_role`/`org_scope`），**不产出具体用户 ID**；具体"当前用户能否认领"由 `ApprovalTaskLayer` 调 `StateController`（内部走 L2/L3 属主规则）做最终裁决（见 §5.6）。

> **easy-workflow Web API Server 模式**（开源 MIT，非闭源）：当 §8.4 选项 B 被选中时，easy-workflow 作为独立 Web API Server 部署，本节集成契约**同样适用**——它的 `GetTaskToDoList`/`TaskPass`/`TaskReject`/`TaskTransfer` 等 API 均属"引擎管人"范畴，必须禁用；只用 `InstanceStart` + 事件产出候选人。easy-workflow 的 `RegisterEvents` 事件系统可用于内部流转，但**不能作为鉴权决策点**——所有事件回调必须重新走 `TicketService` 的三层鉴权。

**Port 与 Adapter 设计**（见 §5.6 `TicketEngine` Port）：

- **出站 Port**（我们调引擎）：`Submit/Trigger/Tasks/GetState/Cancel`，支持远程调用幂等键 + 错误分类（Retryable/NeedReconcile/Permanent）
- **入站 Port**（引擎回调我们）：`WorkflowCallbackHandler.OnWorkflowEvent`，HTTP handler 验签去重后进 Port
- **数据模型**：`tickets` 表加 `workflow_instance_id` + `workflow_engine` 字段（Phase 3 自写 BranchedStateEngine 时可不加，引远程引擎时迁移）
- **分布式一致性**：不引入 XA/2PC（第三方引擎不支持），用 Saga + 对账（本地先写"发起中"，调远程成功更新，失败标记"需对账"，定时拉远程 GetState 修正）
- **fail-closed**：远程引擎故障返回 503 deny，工单操作降级为只读

<a id="86-工作流引擎升级触发信号量化表"></a>
### 8.6 工作流引擎升级触发信号量化表（历史评估条件，非已批准排期）

以下保留当时的量化评估条件；命中只代表需要重新评估，不自动启动或承诺引擎替换。当前实际类型为StateMachine，未实现LinearStateEngine：

| 触发信号 | 阈值 | 含义 |
|---|---|---|
| 需要任务行审批/会签的工单类型数 | ≥ 2 | 历史评估阈值；人工多出边本身不触发替换 |
| 单流程平均节点数 | > 8 | map 可读性下降 |
| 并行审批路径数 | ≥ 1 | 当前单status机无多token并行能力 |
| 跨流程驳回场景 | 出现 | 需要自由驳回 |

> 注：旧方案首选手写BranchedStateEngine，但当前没有实现；S3b与第三方引入须重核真实场景，第三方方案还需满足§8.5契约。

<a id="85-404-vs-403-的安全语义"></a>
### 8.7 404 vs 403 的安全语义

资源不可见时返回 **404** 而非 403，防止信息泄露（攻击者无法通过 403 推断资源存在）。这是本项目资源读取的既定隐藏策略；动作已可见但不可操作时仍返回403，完整边界见§2.2，不用单一第三方文案证明所有系统行为。

### 8.8 事件驱动为何独立设计

工单、ops及未来业务域共享`domainevent`，一次变更多订阅者，业务域不硬编码未来risk消费者。历史ADR的L0/L1/L2命名保留作方案沿革；当前交付语义、可靠性限制与Outbox已拍排期统一见§6，不能将旧轮询方案当现行架构。


> 跨服务事件传播（Redis Streams / Kafka）随微服务拆分推迟，不在近期范围。
