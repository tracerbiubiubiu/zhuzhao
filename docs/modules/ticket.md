# 工单模块设计

> 内部 IT/运维工单系统，作为框架的第一个业务模块，验证三层鉴权体系的资源级权限能力。
>
> 创建日期：2026-08-12

---

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

> **执行顺序**（[11-authz-architecture-review §2](../phase2/11-authz-architecture-review.md#2-q1l2l3-执行顺序方向已确认待正式拍板落档) 修正后）：L1 → **L3 属主短路** → L2 → canOperate。属主命中即跳过 L2 组织关系，但**仍需通过 canOperate**（属主不等于能做所有动作）。

```
Allow ⟺ L1 通过 ∧ ( L3 属主命中 ∨ L2 组织关系命中 ) ∧ canOperate 通过
```

```
请求: POST /api/v1/tickets/123/update
  │
  ├─ L1 路由级 RBAC (Casbin)
  │   用户是否拥有 "ticket:update" 权限码？
  │   └─ 无 → 403
  │
  ├─ L3 属主短路（资源行上的列比较，不查组织关系）
  │   created_by == uid || assigned_to == uid？
  │   ├─ 命中 → 跳过 L2，仍需过 canOperate（见下）
  │   └─ 未命中 → 进入 L2
  │
  ├─ L2 资源级可见性（仅 L3 未命中时执行）
  │   检查 ticket scope:
  │   ├─ all:     全部工单可见
  │   ├─ group:   工单 org_path 在用户可见组织路径下
  │   └─ assigned: 退化为仅属主（L3 已判过，这里必 fail）
  │   └─ 不可见 → 404（不是 403，防信息泄露）
  │
  └─ canOperate（L3 命中或 L2 命中后都要过）
      用户能执行 "update" 操作吗？
      ├─ L3 命中：创建人可 update（2b）；处理人可 close
      ├─ L2 命中：scope 主管（2c org admin/owner）可管本组
      └─ 无权 → 403
```

> **关键澄清**：L3 属主命中 = 跳过 L2（不需要组织关系也能碰自己的工单），**不等于**能做所有动作。比如创建人是属主（L3 命中），但 `assign` 动作只有 admin/scope 主管能做——属主命中后仍被 `canOperate` 拦截返回 403。属主解决"转部门后还能看旧工单"，canOperate 解决"能碰不等于能改"。

### 2.3 权限矩阵

| 操作 | 路由级权限码 | 资源级条件 | 属主条件 |
|------|-------------|-----------|---------|
| 查看工单列表 | `ticket:list` | 按 scope 过滤 | - |
| 查看单条工单 | `ticket:read` | scope 可见性 | - |
| 创建工单 | `ticket:create` | - | - |
| 更新工单 | `ticket:update` | scope 可见性 | **2b：创建人**；2c + org admin/owner；透明读≠可改 |
| 关闭工单 | `ticket:close` | scope 可见性 | 处理人或创建人（2b）；+ 主管 / org admin（2c） |
| 分派工单 | `ticket:assign` | scope 可见性 | 主管以上 |
| 删除工单 | `ticket:delete` | - | admin |
| 添加回复 | `ticket:comment` | scope 可见性 | - |
| 添加内部备注 | `ticket:note` | scope 可见性 | 处理团队成员 |
| 导出工单 | `ticket:export` | - | admin |

### 2.4 Ticket Scope 设计

借鉴 Freshdesk 的三档可见性，用 `ltree` 组织树实现：

| Scope | 含义 | 典型角色 | SQL 过滤逻辑 |
|-------|------|---------|-------------|
| `all` | 全部工单可见 | 主管、admin | 无额外过滤 |
| `group` | 本组织（含子组织）工单 | 一线处理人 | `org_path <@ 用户可见组织路径` |
| `assigned` | 仅本人创建或被分派的工单 | 外包、实习生 | `created_by = $userID OR assigned_to = $userID` |

#### Scope 的存储

在 `user_orgs` 关联表上增加 scope 字段，支持同一用户在不同组织拥有不同可见范围：

```sql
-- 用户在组织中的工单可见范围
-- 一个用户在 A 部门可能是主管（scope=all），在 B 虚拟组只是普通成员（scope=assigned）
ALTER TABLE user_orgs ADD COLUMN ticket_scope VARCHAR(20) DEFAULT 'assigned';
-- 值: 'all' | 'group' | 'assigned'
```

#### 列表查询的行级过滤

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

实现 SSOT：[phase2/09-ticket.md §5.2](../phase2/09-ticket.md#52-phase-2b-scope-升级-部门内读写分离策略-b默认)、[03-org-enhance](../phase2/03-org-enhance.md#权限)。

---

## 3. 数据模型

### 3.1 工单类型配置表

借鉴 ECMDB 的模型驱动思路和 easy-workflow 的 JSON 模板配置，工单类型通过配置表管理，新增类型无需改代码：

```sql
-- 工单类型配置表（定义"有哪些工单类型"）
CREATE TABLE ticket_types (
    id              BIGSERIAL PRIMARY KEY,
    code            VARCHAR(50) UNIQUE NOT NULL,   -- incident, request, change, problem
    name            VARCHAR(100) NOT NULL,          -- "故障事件"、"服务请求"
    description     TEXT,
    states          JSONB NOT NULL DEFAULT '["open","assigned","in_progress","pending_verify","closed","rejected"]',
    transitions     JSONB NOT NULL DEFAULT '{"open":["assigned","closed"],...}',
    default_sla_hours INT DEFAULT 24,
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

### 3.2 工单主表

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
-- 双重职责：① 审计日志（工单详情页展示）；② 事件队列（Phase 3 启动后 L1 供通知/SLA 消费）
-- Phase 3 迁移 000021 会加 event_type（audit/signal）+ processed 列支持 L1 事件机制
CREATE TABLE ticket_events (
    id          BIGSERIAL PRIMARY KEY,
    ticket_id   BIGINT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    user_id     BIGINT NOT NULL,
    action      VARCHAR(50) NOT NULL,        -- created, assigned, status_changed, closed, sla_breached
    from_value  VARCHAR(50),                 -- 变更前值
    to_value    VARCHAR(50),                 -- 变更后值
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_events_ticket ON ticket_events (ticket_id, created_at);
```

### 3.3 状态机

每种工单类型可以有自己的状态机配置（存储在 `ticket_types.transitions` JSONB 中），但默认使用统一状态机：

```
                    ┌─────────┐
          submit    │  open   │
               ───▶ │         │
                    └────┬────┘
                         │ assign
                         ▼
                  ┌─────────────┐
                  │  assigned   │
                  └──────┬──────┘
                         │ start processing
                         ▼
                  ┌──────────────┐
            ┌────│ in_progress  │────┐
            │    └──────────────┘    │
            │ resolve               │ reject
            ▼                       ▼
  ┌──────────────────┐      ┌──────────┐
  │ pending_verify   │      │ rejected │
  └────────┬─────────┘      └──────────┘
           │ verify ok         ▲
           ▼                   │ reject
    ┌─────────┐               │
    │ closed  │───────────────┘
    └─────────┘  reopen (可选)
```

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

## 4. 接口定义

### 4.1 API 路由（遵循 GET + POST 规范）

| 方法 | 路径 | 权限码 | 说明 |
|------|------|--------|------|
| GET | `/api/v1/tickets` | `ticket:list` | 工单列表（按 scope 过滤） |
| POST | `/api/v1/tickets` | `ticket:create` | 创建工单 |
| GET | `/api/v1/tickets/:id` | `ticket:read` | 工单详情 |
| POST | `/api/v1/tickets/update` | `ticket:update` | 更新工单 |
| POST | `/api/v1/tickets/close` | `ticket:close` | 关闭工单 |
| POST | `/api/v1/tickets/assign` | `ticket:assign` | 分派工单 |
| POST | `/api/v1/tickets/delete` | `ticket:delete` | 删除工单 |
| GET | `/api/v1/tickets/:id/comments` | `ticket:read` | 回复列表 |
| POST | `/api/v1/tickets/comments` | `ticket:comment` | 添加回复 |
| POST | `/api/v1/tickets/notes` | `ticket:note` | 添加内部备注 |

### 4.2 Service 接口

```go
type TicketService interface {
    // Create 创建工单
    Create(ctx context.Context, req CreateTicketReq) (*Ticket, error)
    
    // Get 获取工单详情（含资源级鉴权）
    Get(ctx context.Context, userID int64, scope TicketScope, ticketID int64) (*Ticket, error)
    
    // List 工单列表（按 scope 行级过滤）
    List(ctx context.Context, filter TicketFilter) ([]Ticket, int64, error)
    
    // Update 更新工单（含属主判断）
    Update(ctx context.Context, userID int64, ticketID int64, req UpdateTicketReq) error
    
    // Close 关闭工单
    Close(ctx context.Context, userID int64, ticketID int64, comment string) error
    
    // Assign 分派工单
    Assign(ctx context.Context, ticketID int64, assigneeID int64) error
    
    // Transition 状态转换
    Transition(ctx context.Context, ticketID int64, targetStatus string) error
    
    // AddComment 添加回复
    AddComment(ctx context.Context, ticketID int64, req CommentReq) error
}
```

### 4.3 ResourceRegistry 注册

> **实现 SSOT**：[phase2/02-authz-resource.md §TicketResource](../phase2/02-authz-resource.md) + [phase2/09-ticket.md §5](../phase2/09-ticket.md)。`modules/ticket.md` 不再内联注册代码，以免与 SSOT 接口漂移。

工单模块在 Phase 2a 通过实现 `Resource` 接口（`Authorize` / `GetFilter`）自注册到 `ResourceRegistry`，见上述 SSOT。

---

## 5. 工单类型抽象与 Hook 机制

### 5.1 设计方案：统一接口 + 配置驱动 + Hook 扩展

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

### 5.2 Hook 接口定义

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

### 5.3 类型注册管理

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

### 5.4 Hook 使用示例

```go
// 变更类工单的特殊 Hook（Phase 3 需要审批流时注册）
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
    // Phase 3: 注册变更类工单的特殊 Hooks
    // m.Register("change", &ChangeTicketHooks{...})
    return m
}
```

### 5.5 前端动态表单

前端通过 API 获取工单类型的字段定义，动态渲染表单：

| API | 说明 |
|-----|------|
| `GET /api/v1/ticket-types` | 获取所有工单类型 |
| `GET /api/v1/ticket-types/:code` | 获取类型详情（含状态机配置） |
| `GET /api/v1/ticket-types/:code/fields` | 获取类型字段定义（前端动态渲染表单） |

### 5.6 TicketEngine Port（引擎可替换边界）

> **目的**：锁定工单流程引擎的可替换边界。Phase 2a 的线性状态机、Phase 3 的 `BranchedStateEngine`、未来的远程 HTTP 引擎共用同一个 Port，上层 `TicketService` 不感知实现。

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
| `LinearStateEngine` | Phase 2a | 进程内 | `map[string][]string` 加载自 `ticket_types.transitions`，instanceID = ticketID |
| `BranchedStateEngine` | Phase 3 | 进程内 | 手写 Node/Gateway 模型，支持分支/会签，借鉴 easy-workflow 设计不借代码 |
| `RemoteHTTPWorkflowEngine` | 未来按需 | 独立部署第三方引擎 | HTTP 对接，需满足 §8.5 集成契约 |

**关键边界——鉴权不进 Port**：

`TicketEngine` 只负责"状态怎么走、产生哪些待办"，**不做任何 L1/L2/L3 判断**。所有鉴权仍在 `TicketService`/`ResourceRegistry` 层：

- 引擎产出的 `Task` 只带 `ApprovalRequirement`（审批条件），只是**建议**，最终能不能操作由 L2/L3 属主规则裁决
- 引擎不持有用户身份，所有 `actor` 都是本地 user_id
- 引擎不知道 `org_path`/`ticket_visibility`/`ReadAnchorPaths` 等鉴权概念

这样无论将来换成什么引擎，鉴权体系都不用动——这是应对"引擎替换"的唯一正确姿势。远程引擎的完整集成契约见 §8.5。

---

## 6. 事件驱动集成（概要）

工单模块通过事件与其他模块协作。事件驱动作为独立的横切模块设计，L1→L2 升级（Outbox + Asynq）方案见 [ADR-001](../adr/ADR-001-event-mechanism-l1-steady-state.md) 与 [ADR-002](../adr/ADR-002-asynq-async-task-executor.md)，完整机制见 [phase3/10-ticket-business.md §7](../phase3/10-ticket-business.md#7-事件机制-l0--l1-升级)。

### 工单相关事件

| 事件类型 | 触发时机 | 消费者 |
|---------|---------|--------|
| `ticket.created` | 工单创建后 | 通知服务、SLA 计时器 |
| `ticket.assigned` | 工单被分派后 | 通知服务（通知处理人） |
| `ticket.status_changed` | 状态变更后 | 通知服务、SLA 计时器（启停） |
| `ticket.closed` | 工单关闭后 | 满意度调查、SLA 统计 |
| `ticket.sla_breached` | SLA 超时 | 告警服务、升级处理 |
| `ticket.approved` | 审批节点通过（submit 成功，同一事务写 `workflow_records`） | 通知发起人 / 通知下一节点候选 / SLA 节点重置 |
| `ticket.rejected` | 审批节点驳回（target=prev 退上级 / target=origin 打回发起人） | 通知发起人（被驳回）/ SLA 重置 |

### 三档事件机制（L0 / L1 / L2）

工单事件机制按阶段分三档演进，业务逻辑（通知内容、SLA 规则）跨档不变，只换调度器：

| 档 | 阶段 | 实现 | 可靠性 | 适用 |
|---|---|---|---|---|
| **L0** | Phase 2a | Go channel + goroutine | 进程崩溃丢 | MVP（2a 过渡） |
| **L1** | **Phase 2a 起（长期稳态，见 [ADR-001](../adr/ADR-001-event-mechanism-l1-steady-state.md)）** | **进程内事件 + DB 持久化（`ticket_events` 表 + 轮询补偿）+ 分布式锁** | **进程崩溃不丢，多实例靠分布式锁防重** | **生产单/多实例** |
| L2 | **暂缓（按需，见 ADR-001）** | Outbox + Asynq worker 多消费者 | 跨服务可靠 | 多消费者/微服务 |

> **Asynq 已引入**（[ADR-002](../adr/ADR-002-asynq-async-task-executor.md)）：作为"异步任务执行器"与 L1 并存，职责互补——L1 管事件事实持久化，Asynq 管异步任务执行（审批触发事件 + 预置定时任务）。Asynq 不替代 L1 事件源。

**L1 设计要点**（见 [phase3/10-ticket-business.md §7](../phase3/10-ticket-business.md#7-事件机制-l0--l1-升级)）：

- 事件先写 `ticket_events` 表（2a 已建，事务内保证不丢）
- 消费者在事务后立即拉取 `ticket_events` 处理；多实例下用 [multi-instance](../phase3/02-multi-instance.md) 的分布式锁保证同一事件不被重复消费
- SLA 定时扫描复用分布式锁，避免多实例重复扫描；**Asynq 已引入（ADR-002），Phase 3 起由 Asynq PeriodicTask 接管定时调度**
- L1 比 channel 可靠（崩溃不丢），比 Outbox 简单（无额外表/worker）——长期稳态
- L1 是**长期稳态机制，非临时过渡**（ADR-001）：`processed` 标记、分布式锁防重、双重职责（audit/signal）分离均按产线级实现，不因"以为 L1 是过渡"而偷工减料
- L2 升级时业务逻辑不变，只把"轮询拉取"换成"Asynq worker 消费 Outbox"——L2 是按需升级，不是必经路径

---

## 7. 分阶段实施

> **口径说明**（2026-08-25 修订）：Phase 3 整体暂缓（见 [roadmap.md](../roadmap.md)），不拆 3a/3b 子阶段执行。工单**主链路**（CRUD / 状态机 / 模板 / 关联）已在 Phase 2a/2b 实现；**SLA 计时/违约、通知、审批流、分派、报表仍属 Phase 3（暂缓，设计就绪）**，事件机制用 L1（长期稳态，ADR-001）+ Asynq（ADR-002）。以下按"Phase 2 已实现 / Phase 3 暂缓"两段式描述，能力细节见 [phase3/10-ticket-business.md](../phase3/10-ticket-business.md)（Phase 3 启动时取用）。
>
> **Phase 2 实现 SSOT**：[phase2/09-ticket.md](../phase2/09-ticket.md)（2a MVP + 2b scope + 2c Authorize）。  
> **工单业务能力细节 SSOT**：[phase3/10-ticket-business.md](../phase3/10-ticket-business.md)（SLA/通知/审批流/分派/报表/L1 升级，Phase 3 启动时取用）。

### Phase 2a：工单 MVP

| 能力 | 说明 |
|------|------|
| 工单 CRUD | 创建、查看、更新、关闭 |
| 工单类型配置 | `ticket_types` + `ticket_type_fields` 表，支持前端动态表单 |
| 状态机 | 从 `ticket_types.transitions` 加载，默认 open → assigned → in_progress → closed |
| 路由级 RBAC | Casbin 权限码控制操作 |
| 资源级 scope | **assigned** 可见性（2b 扩展 group/all） |
| 组内 admin/owner 资源操作 | **Phase 2c**：见 [04-org-delegation §4](../phase2/04-org-delegation.md#4-authorize-升级step-10) |
| 回复功能 | 公开回复 + 内部备注 |
| 事件日志 | 状态变更记录 |
| Hook 机制 | `TicketHooks` 接口 + `DefaultTicketHooks` |
| 进程内事件 | Go channel 分发（L0，2a 过渡；L1 随后接入，见 §6） |
| **工单模板**（前移） | `ticket_templates` 表，支持 `template_code` 预填字段快速创建；`default_sla_minutes` 仅存储，SLA 启用计时器后生效（迁移 000015） |
| **工单关联**（前移） | `ticket_relations` 表，支持 parent_child/duplicate/blocked_by/related 四种关系；建立关联时对 target 走 L2/L3 鉴权（迁移 000016） |

### Phase 2b：scope + 附件 + 体验

| 能力 | 说明 |
|------|------|
| group/all scope | **策略 B** 实体透明读 + scope 并集；`ticket_visibility` 配置，见 [03-org-enhance](../phase2/03-org-enhance.md) |
| 工单附件 | [10-storage](../phase2/10-storage.md) 预签名直传 |

### Phase 2c：组内委托

| 能力 | 说明 |
|------|------|
| org admin/owner Authorize | [04-org-delegation](../phase2/04-org-delegation.md) |

### Phase 3：工单完整业务能力闭环（暂缓，按需取用）

> 工单作为下游能力的入口，需闭合以挂载通知/SLA/满意度/告警。能力设计已完成（见 [phase3/10-ticket-business.md](../phase3/10-ticket-business.md)），Phase 3 启动时取用。
>
> ⚠️ **暂缓说明**（2026-08-25）：Phase 3 整体暂缓、不排期（见 [roadmap.md](../roadmap.md)）。以下能力**设计已就绪**，实际实现时机视真实需求决定。

| 能力 | 说明 | 实现方式 |
|------|------|---------|
| SLA 计时 | 按优先级设定响应/解决时限 | `ticket_sla` 表 + Asynq PeriodicTask 扫描（替代进程内定时器，见 ADR-002） |
| SLA 违约告警 | 超时触发告警/升级 | Asynq 定时扫描 + 分布式锁（多实例防重） |
| 站内通知 | 工单分派、状态变更通知 | L1 事件（`ticket_events` 轮询） + `notifications` 表 |
| 邮件通知 | 关键状态变更邮件提醒 | L1 事件 + SMTP（可走 Asynq 异步发送） |
| 多级审批流 | 变更类工单走审批流（会签/分支） | **`BranchedStateEngine`**（手写两层分离：状态机内核 + ApprovalTaskLayer，借鉴 aifei-go flow + easy-workflow，注册到 `TicketEngine` Port） |
| 自动分派规则 | 按类型/关键词自动分派 | `assignment_rules` 表 + 规则匹配引擎 |
| 工单报表 | 按组织/处理人/类型统计 | SQL 聚合查询 + 进程内缓存 |
| 事件机制 L1 | `ticket_events` 持久化 + 轮询补偿 + 分布式锁（长期稳态，ADR-001） | 见 §6 + [10-ticket-business §7](../phase3/10-ticket-business.md#7-事件机制-l0--l1-升级) |
| Asynq 异步任务 | 审批触发事件 + 预置定时任务（ADR-002） | L1 事件源 + Asynq 执行器，职责互补 |

> **已前移到 Phase 2a**（2026-08-25）：工单模板（`ticket_templates`，迁移 000015）、工单关联（`ticket_relations`，迁移 000016）。两者均为纯 DB 表，零事件依赖，前移后 Phase 3 聚焦 SLA/通知/审批流/分派/报表/L1+Asynq。

### L2 升级（暂缓，按需）

| 能力 | 说明 | 升级方式 |
|------|------|---------|
| L2 Outbox | 可靠事件分发，多消费者 | L1 → L2：轮询拉取换成 Asynq worker 消费 Outbox（业务逻辑不变） |
| 微服务拆分 | **不做**（无需求） | 见 [phase3/README §1.3](../phase3/README.md#13-不做什么) + [11-deployment-split.md](../phase3/11-deployment-split.md) |

---

## 8. 关键设计决策

### 8.1 为什么不用现成工单产品

| 方案 | 否决理由 |
|------|----------|
| Zammad | Ruby 栈，4GB+ RAM，独立用户体系，无法共享 IAM |
| FreeScout / GLPI | PHP 栈，需另起运行时 |
| go-help-desk | 6 star，无生产验证，AGPL 传染性 |
| escalated-go | 3 star，无审批流/工作流能力 |
| easy-workflow | 功能优秀（会签、自由驳回、混合网关），**MIT 许可**，可作设计参考蓝图。否决的是"移植代码进我们的 Go 进程"（`MySQL 8 CTE` 方言 + GORM + 全局单例引擎，与 PG + pgx + Wire 存在近重写成本，非绝对"不兼容"——GORM 本身支持 PG），不是"Web API Server 独立部署"模式——后者作为 Phase 3+ 备选见 §8.5。注：原官方仓库 `qunarcorp/easy-workflow` 已不可访问（404），活跃分支为社区版 `Bunny3th/easy-workflow`（158 commits / 260 stars）；其权限模型**外置**（角色解析在业务侧事件回调），不存在"鉴权耦合" |
| ECMDB/EFlow | 架构优秀（Wire DI + 模型驱动 + 插件化），但 MongoDB + Kafka 技术栈不匹配，且 EFlow 工单系统未开源 |

自研的核心优势：工单本身是一种"资源"，直接纳入框架已设计好的三层鉴权体系。

### 8.2 从 easy-workflow 借鉴什么（完整能力映射）

easy-workflow（MIT 许可）作为自写 `BranchedStateEngine` 的设计参考蓝图。**借的是设计，不是代码**——用 pgx 重写，避开它的三个实现局限。完整能力映射：

| easy-workflow 能力 | 设计要点 | 自写 BranchedStateEngine 的实现 |
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
| **计划任务** | `ScheduleTask` + 进程内 `ScheduledTaskPool` | ⚠️ Phase 3 用 Asynq PeriodicTask 替代（更可靠，复用 [multi-instance](../phase3/02-multi-instance.md) 分布式锁） |

**自写时必须避开的三个 easy-workflow 局限**：

1. **MySQL CTE 依赖** → 自写用 PG 的 `WITH RECURSIVE` 或邻接表 + BFS，不绑死数据库
2. **表达式求值下放 DB**（`ExpressionEvaluator` 拼 SQL 让 MySQL 算）→ Go 侧求值，避免 SQL 注入且可单元测试
3. **反射事件注册**（`RegisterEvents` 无编译期检查）→ 显式 `TicketHooks` 接口，编译期保证方法签名正确

> **Phase 2a 借鉴**（已落地）：流程定义 JSONB 存储、事件系统对应 `TicketHooks`、运行表+历史表双表设计。见 §3 `ticket_types.transitions`、§5 `TicketHooks`。
>
> **Phase 3 借鉴**（待落地）：混合网关三字段模型、会签+BatchCode、自由驳回（BFS 替代 CTE）、TaskAction 自描述。见 [phase3/10-ticket-business.md §4](../phase3/10-ticket-business.md#4-多级审批流branchedstateengine)。

### 8.3 从 ECMDB 借鉴什么

| 设计 | 借鉴方式 |
|------|---------|
| 模型驱动（Model → Attribute → Resource） | 工单类型驱动（TicketType → TicketTypeField → Ticket） |
| 自定义字段用动态存储 | `tickets.custom_data` JSONB（类似 ECMDB 的 `mongox.MapStr`） |
| 插件化资源动作 | `TicketHooks` 钩子机制，类型特定的业务逻辑通过 Hook 扩展 |
| Wire 模块化 Provider Set | 工单模块的 Wire 注入遵循相同的分 Set 组织方式 |

### 8.4 为什么 Phase 2a 不引入工作流引擎；Phase 3 如何升级

Phase 2a 的状态机是线性的（6 状态 + 几条转换规则），一个 map 就能表达。引入 Temporal 等工作流引擎属于过度设计。

**Phase 3 的升级路径**（[phase3/10-ticket-business.md](../phase3/10-ticket-business.md)）：当变更类工单需要多级审批、会签、条件分支时，**首选自研 `BranchedStateEngine`**——借鉴 easy-workflow 的 Node/Gateway 模型（**设计，不借代码**），用 pgx 重写一个支持分支/会签的轻量状态机，注册到 `TicketEngine` Port（见 §5.6）。

评估过的外部方案：
- Temporal（工业级，需独立 server）——过重，Phase 3 不引
- 移植 easy-workflow 源码（`MySQL 8 CTE` 方言 + GORM + 全局单例引擎）——**近重写**：GORM→pgx 重写持久层、MySQL→PG 迁移、全局状态→Wire DI 去全局化，粗估 2000-4000 行。注意：easy-workflow 的权限模型是**外置的**（通过 `NodeStartEvents` 让业务侧解析角色），不存在"鉴权耦合"问题；否决理由纯粹是技术栈近重写成本（官方原版 `qunarcorp/easy-workflow` 已 404，活跃分支 `Bunny3th/easy-workflow` MIT）。服务化集成场景的鉴权适配见 §8.5
- **easy-workflow Web API Server 独立部署**（MIT 许可，可行但有代价）——见下方对比，作为 Phase 3 备选
- 第三方闭源引擎服务化集成——见 §8.5 集成契约

**Phase 3 两个合法选项**（当 §8.6 触发信号出现时）：

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

**切换信号**：选项 A 首选开始；若出现"会签/网关需求复杂度超出预期（>3 种分支模式）"或"团队无带宽自写"或"需要可视化流程编辑器"，切到选项 B。

### 8.5 第三方/闭源工作流引擎集成契约

若未来出现需要引入第三方/闭源工作流引擎（独立部署、HTTP 对接）的场景，必须遵守以下契约。**这是硬过滤条件**——违反任一条的引擎直接 pass，不管功能多强：

| 引擎能力 | 用/禁 | 说明 |
|---|---|---|
| 状态机定义/流转 | ✅ 用 | 引引擎的价值 |
| 网关/会签/分支 | ✅ 用 | 线性机做不了的部分 |
| 产出审批条件（ApprovalRequirement） | ✅ 用 | 只作**建议**，传给我们，L2/L3 裁决 |
| **claim/complete 任务** | ❌ **禁用** | 必须改走我们的 `POST /api/v1/tickets/:id/...` 走三层鉴权 |
| **用户体系** | ❌ **禁用** | 引擎不能持有用户身份，所有 actor 都是我们映射过去的本地 user_id |
| **权限判断** | ❌ **禁用** | 引擎不知道 `org_path`/`ticket_visibility`/`ReadAnchorPaths`，这些概念见 [09-ticket §5.2](../phase2/09-ticket.md#52-phase-2b-scope-升级-部门内读写分离策略-b默认) |

**集成边界**：引擎只是"状态机 + 审批条件建议器"。所有"用户能不能 claim/complete"的判断，必须在 `TicketService` 走 L1/L2/L3（见 §2.2）。引擎产出的 `Task` 只带 `ApprovalRequirement`（审批任职条件，如 `approver_role`/`org_scope`），**不产出具体用户 ID**；具体"当前用户能否认领"由 `ApprovalTaskLayer` 调 `StateController`（内部走 L2/L3 属主规则）做最终裁决（见 §5.6）。

> **easy-workflow Web API Server 模式**（开源 MIT，非闭源）：当 §8.4 选项 B 被选中时，easy-workflow 作为独立 Web API Server 部署，本节集成契约**同样适用**——它的 `GetTaskToDoList`/`TaskPass`/`TaskReject`/`TaskTransfer` 等 API 均属"引擎管人"范畴，必须禁用；只用 `InstanceStart` + 事件产出候选人。easy-workflow 的 `RegisterEvents` 事件系统可用于内部流转，但**不能作为鉴权决策点**——所有事件回调必须重新走 `TicketService` 的三层鉴权。

**Port 与 Adapter 设计**（见 §5.6 `TicketEngine` Port）：

- **出站 Port**（我们调引擎）：`Submit/Trigger/Tasks/GetState/Cancel`，支持远程调用幂等键 + 错误分类（Retryable/NeedReconcile/Permanent）
- **入站 Port**（引擎回调我们）：`WorkflowCallbackHandler.OnWorkflowEvent`，HTTP handler 验签去重后进 Port
- **数据模型**：`tickets` 表加 `workflow_instance_id` + `workflow_engine` 字段（Phase 3 自写 BranchedStateEngine 时可不加，引远程引擎时迁移）
- **分布式一致性**：不引入 XA/2PC（第三方引擎不支持），用 Saga + 对账（本地先写"发起中"，调远程成功更新，失败标记"需对账"，定时拉远程 GetState 修正）
- **fail-closed**：远程引擎故障返回 503 deny，工单操作降级为只读

### 8.6 工作流引擎升级触发信号量化表

"Phase 3 再评估"太虚，给硬指标。触发任一即启动从 `LinearStateEngine` → `BranchedStateEngine` 的升级：

| 触发信号 | 阈值 | 含义 |
|---|---|---|
| 需要分支/会签的工单类型数 | ≥ 2 | 线性状态机表达不下 |
| 单流程平均节点数 | > 8 | map 可读性下降 |
| 并行审批路径数 | ≥ 1 | 线性机无并行能力 |
| 跨流程驳回场景 | 出现 | 需要自由驳回 |

> 注：Phase 3 的 `BranchedStateEngine` 是手写实现，不引第三方引擎。第三方/闭源引擎的引入时机更靠后，需同时满足 §8.5 契约 + 上述触发信号。

### 8.7 404 vs 403 的安全语义

资源不可见时返回 **404** 而非 403，防止信息泄露（攻击者无法通过 403 推断资源存在）。这是业界标准做法（Freshdesk 文档原文："The ticket is either not there or you don't have permission"）。

### 8.8 事件驱动为何独立设计

事件驱动不应耦合在工单模块内，因为：
- 工单创建/状态变更会触发事件
- 定时任务（SLA 检查、报表生成）会触发事件
- 其他服务（用户管理、组织变更）也会触发事件

因此事件驱动作为独立的横切模块设计，L1→L2 升级方案详见 [ADR-001](../adr/ADR-001-event-mechanism-l1-steady-state.md) 与 [ADR-002](../adr/ADR-002-asynq-async-task-executor.md)。三档演进（见 §6 + [ADR-001](../adr/ADR-001-event-mechanism-l1-steady-state.md)）：
- **Phase 2a（L0→L1）**：Go 原生 channel（L0，过渡）→ `ticket_events` 持久化 + 轮询补偿 + 分布式锁（L1，**长期稳态**）
- **Asynq 已引入**（[ADR-002](../adr/ADR-002-asynq-async-task-executor.md)）：与 L1 并存，作为异步任务执行器（审批触发事件 + 预置定时任务），不替代 L1 事件源
- **L2 暂缓（按需）**：PostgreSQL Outbox + Asynq worker 多消费者——多消费者需求出现时升级，业务逻辑不变只换调度器

> 跨服务事件传播（Redis Streams / Kafka）随微服务拆分推迟，不在近期范围。
