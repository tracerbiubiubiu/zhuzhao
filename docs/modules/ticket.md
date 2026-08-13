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

```
请求: POST /api/v1/tickets/123/update
  │
  ├─ 第1层: 路由级 RBAC (Casbin)
  │   用户是否拥有 "ticket:update" 权限码？
  │   └─ 无 → 403
  │
  ├─ 第2层: 资源级鉴权 (工单服务内联)
  │   用户能看到工单 #123 吗？
  │   └─ 检查 ticket scope:
  │      ├─ all:     全部工单可见
  │      ├─ group:   工单 org_path 在用户可见组织路径下
  │      └─ assigned: 工单 created_by 或 assigned_to = 当前用户
  │      └─ 不可见 → 404（不是 403，防信息泄露）
  │
  └─ 第3层: 属主/操作权限
      用户能执行 "update" 操作吗？
      └─ 创建人、处理人、或本组主管
         └─ 无权 → 403
```

### 2.3 权限矩阵

| 操作 | 路由级权限码 | 资源级条件 | 属主条件 |
|------|-------------|-----------|---------|
| 查看工单列表 | `ticket:list` | 按 scope 过滤 | - |
| 查看单条工单 | `ticket:read` | scope 可见性 | - |
| 创建工单 | `ticket:create` | - | - |
| 更新工单 | `ticket:update` | scope 可见性 | 创建人或处理人 |
| 关闭工单 | `ticket:close` | scope 可见性 | 处理人或主管 |
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
| POST | `/api/v1/tickets/:id/update` | `ticket:update` | 更新工单 |
| POST | `/api/v1/tickets/:id/close` | `ticket:close` | 关闭工单 |
| POST | `/api/v1/tickets/:id/assign` | `ticket:assign` | 分派工单 |
| POST | `/api/v1/tickets/:id/delete` | `ticket:delete` | 删除工单 |
| GET | `/api/v1/tickets/:id/comments` | `ticket:read` | 回复列表 |
| POST | `/api/v1/tickets/:id/comments` | `ticket:comment` | 添加回复 |
| POST | `/api/v1/tickets/:id/notes` | `ticket:note` | 添加内部备注 |

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

工单模块通过 `ResourceRegistry` 自注册资源级鉴权策略：

```go
func (s *ticketService) RegisterResource(registry ResourceRegistry) {
    registry.Register(ResourceConfig{
        Name: "ticket",
        // 资源级鉴权函数
        CheckAccess: func(ctx context.Context, userID int64, action string, resourceID int64) (bool, error) {
            ticket, err := s.repo.GetByID(ctx, resourceID)
            if err != nil {
                return false, err
            }
            
            // 获取用户对该工单所属组织的 scope
            scope, err := s.getScope(ctx, userID, ticket.OrgID)
            if err != nil {
                return false, err
            }
            
            // 按 scope 判断可见性
            switch scope {
            case ScopeAll:
                return true, nil
            case ScopeGroup:
                // 检查工单 org_path 是否在用户可见组织路径下
                return s.isOrgVisible(ctx, userID, ticket.OrgPath)
            case ScopeAssigned:
                return ticket.CreatedBy == userID || ticket.AssignedTo == userID, nil
            }
            return false, nil
        },
        // 属主判断函数（更细粒度的操作权限）
        CheckOwner: func(ctx context.Context, userID int64, action string, resourceID int64) (bool, error) {
            ticket, err := s.repo.GetByID(ctx, resourceID)
            if err != nil {
                return false, err
            }
            
            switch action {
            case "update":
                return ticket.CreatedBy == userID || ticket.AssignedTo == userID, nil
            case "close":
                return ticket.AssignedTo == userID, nil
            case "assign":
                // 主管以上才能分派
                return s.isManager(ctx, userID, ticket.OrgID)
            default:
                return false, nil
            }
        },
    })
}
```

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
    // Phase 1: 所有类型使用默认 Hooks
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

---

## 6. 事件驱动集成（概要）

工单模块通过事件与其他模块协作。事件驱动作为独立的横切模块设计，详细方案见 `docs/proposal/event-design.md`（待编写）。

### 工单相关事件

| 事件类型 | 触发时机 | 消费者 |
|---------|---------|--------|
| `ticket.created` | 工单创建后 | 通知服务、SLA 计时器 |
| `ticket.assigned` | 工单被分派后 | 通知服务（通知处理人） |
| `ticket.status_changed` | 状态变更后 | 通知服务、SLA 计时器（启停） |
| `ticket.closed` | 工单关闭后 | 满意度调查、SLA 统计 |
| `ticket.sla_breached` | SLA 超时 | 告警服务、升级处理 |

### Phase 1 的极简事件

Phase 1 不引入消息队列，使用 Go 原生 channel + goroutine 实现进程内事件分发。Phase 2 引入 Outbox + Asynq 实现可靠事件分发和异步任务。

---

## 7. 分阶段实施

### Phase 1：基础工单

| 能力 | 说明 |
|------|------|
| 工单 CRUD | 创建、查看、更新、关闭 |
| 工单类型配置 | `ticket_types` + `ticket_type_fields` 表，支持前端动态表单 |
| 状态机 | 从 `ticket_types.transitions` 加载，默认 open → assigned → in_progress → closed |
| 路由级 RBAC | Casbin 权限码控制操作 |
| 资源级 scope | 三档可见性（all/group/assigned） |
| 回复功能 | 公开回复 + 内部备注 |
| 事件日志 | 状态变更记录 |
| Hook 机制 | `TicketHooks` 接口 + `DefaultTicketHooks`，所有类型走默认逻辑 |
| 进程内事件 | Go channel 分发，触发通知等简单逻辑 |

### Phase 2：SLA + 通知 + 事件驱动

| 能力 | 说明 |
|------|------|
| SLA 计时 | 按优先级设定响应/解决时限 |
| SLA 违约告警 | asynq periodic task 定时检查 |
| 站内通知 | 工单分派、状态变更通知 |
| 邮件通知 | 关键状态变更邮件提醒 |
| Outbox + Asynq | 可靠事件分发，异步任务队列 |
| 工单报表 | 按组织/处理人/类型统计 |

### Phase 3：高级能力

| 能力 | 说明 |
|------|------|
| 多级审批流 | 注册 `ChangeTicketHooks`，变更类工单走审批流 |
| 工单模板 | 预设工单模板，快速创建 |
| 工单关联 | 工单之间的关联关系（父子、重复、阻塞） |
| 自动分派规则 | 按类型/关键词自动分派给对应团队 |

---

## 8. 关键设计决策

### 8.1 为什么不用现成工单产品

| 方案 | 否决理由 |
|------|----------|
| Zammad | Ruby 栈，4GB+ RAM，独立用户体系，无法共享 IAM |
| FreeScout / GLPI | PHP 栈，需另起运行时 |
| go-help-desk | 6 star，无生产验证，AGPL 传染性 |
| escalated-go | 3 star，无审批流/工作流能力 |
| easy-workflow | 功能优秀（会签、自由驳回、混合网关），但 MySQL-only + GORM + 全局状态，与 PG + pgx + Wire 架构不兼容 |
| ECMDB/EFlow | 架构优秀（Wire DI + 模型驱动 + 插件化），但 MongoDB + Kafka 技术栈不匹配，且 EFlow 工单系统未开源 |

自研的核心优势：工单本身是一种"资源"，直接纳入框架已设计好的三层鉴权体系。

### 8.2 从 easy-workflow 借鉴什么

| 设计 | 借鉴方式 |
|------|---------|
| 流程定义存储为 JSON 文本 | 工单类型的状态机配置用 JSONB 存储在 `ticket_types.transitions` |
| 事件系统（节点开始/结束/任务完成） | `TicketHooks` 接口（OnBeforeCreate/OnAfterCreate/OnBeforeTransition/...） |
| 运行表 + 历史表成对设计 | 工单事件日志表 `ticket_events` 记录所有状态变更 |
| 版本管理 + 历史归档 | `ticket_types` 支持版本升级（Phase 3） |

### 8.3 从 ECMDB 借鉴什么

| 设计 | 借鉴方式 |
|------|---------|
| 模型驱动（Model → Attribute → Resource） | 工单类型驱动（TicketType → TicketTypeField → Ticket） |
| 自定义字段用动态存储 | `tickets.custom_data` JSONB（类似 ECMDB 的 `mongox.MapStr`） |
| 插件化资源动作 | `TicketHooks` 钩子机制，类型特定的业务逻辑通过 Hook 扩展 |
| Wire 模块化 Provider Set | 工单模块的 Wire 注入遵循相同的分 Set 组织方式 |

### 8.4 为什么 Phase 1 不引入工作流引擎

Phase 1 的状态机是线性的（6 状态 + 几条转换规则），一个 map 就能表达。引入 Temporal 等工作流引擎属于过度设计。

Phase 3 如果需要多级审批、会签、条件分支，再评估：
- Temporal（工业级，需独立 server）
- 自研轻量状态机（借鉴 easy-workflow 的 Node/Gateway 模型，用 pgx 重写）

### 8.5 404 vs 403 的安全语义

资源不可见时返回 **404** 而非 403，防止信息泄露（攻击者无法通过 403 推断资源存在）。这是业界标准做法（Freshdesk 文档原文："The ticket is either not there or you don't have permission"）。

### 8.6 事件驱动为何独立设计

事件驱动不应耦合在工单模块内，因为：
- 工单创建/状态变更会触发事件
- 定时任务（SLA 检查、报表生成）会触发事件
- 其他服务（用户管理、组织变更）也会触发事件

因此事件驱动作为独立的横切模块设计，详见 `docs/proposal/event-design.md`（待编写）。初步规划：
- Phase 1：Go 原生 channel 进程内事件
- Phase 2：PostgreSQL Outbox + Asynq 可靠事件分发 + 异步任务队列
- Phase 3：评估是否引入 Redis Streams 或 Kafka 做跨服务事件传播
