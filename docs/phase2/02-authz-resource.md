# 02 - 资源级鉴权（authz-resource，Phase 2a）

> **Step 1**，依赖 Phase 1 验收（Casbin 路由级 RBAC + ResourceRegistry 空接口）。  
> 完整资源抽象见 [proposal/resource-model.md](../proposal/resource-model.md)；**本文档为 2a 实现 SSOT**。

---

## 0. 边界

### 做什么（2a）

| 能力 | 说明 |
|------|------|
| `ScopeResolver` | 解析用户对某组织的 **effective ticket_scope**（2a 固定 `assigned`） |
| `Registry` 落地 | 实现 `Authorize` / `GetFilter` 委托到已注册 `Resource` |
| `TicketResource` | 第一个业务 Resource：可见性 + 属主判断 |
| **assigned** 列表过滤 | `created_by = $user OR assigned_to = $user` |
| Handler 辅助 | Service 层统一调用 `registry.Authorize` / `GetFilter` |

### 不做（2a）

| 不做 | 阶段 |
|------|------|
| `group` / `all` scope、ltree 过滤 | 2b（org-enhance + ticket 升级） |
| BFS 三源角色 | 2b |
| `resource_owners` 表 | **2a 不建**；工单直接用 `tickets.org_id` / `org_path` |
| 每资源独立 Casbin Enforcer | 按需 / Phase 3 |
| 组内委托 Authorize | 2c |

> **与 architecture 对齐**：`resource_owners` 在 [architecture §10.3](../design/architecture.md#103-phase-2-预留本文档不展开-ddl) 标注为 2b 预留；2a 工单不依赖该表。

---

## 1. 前置条件

- [ ] Phase 1 Casbin 中间件 + `RoleFetcher`（直接角色）可用
- [ ] `internal/pkg/resource/registry.go` 空 Registry 已 Wire 注入
- [ ] 组织 ltree 已存在（Phase 1），但 **2a 鉴权不使用 ltree 过滤**

---

## 2. 核心设计

### 2.1 三层鉴权（2a 范围）

```
请求 → JWT → Casbin（路由级）
         → TicketService
              ├─ GetFilter / 单条可见性（第 2 层，scope=assigned）
              └─ Authorize 属主（第 3 层，create/update/close/assign）
```

| 层 | 2a 实现 |
|----|---------|
| 路由级 | 不变；须 `ticket:*` 权限码 |
| 可见性 | **assigned**：本人创建或被分派 |
| 操作权 | 创建人 / 处理人（+ 路由级 admin bypass 可选） |

**不可见单条资源 → 404**（非 403），见 [modules/ticket.md §8.5](../modules/ticket.md#85-404-vs-403-的安全语义)。

### 2.2 ScopeResolver（2a 桩）

```go
// internal/service/authz/scope_resolver.go

type TicketScope string

const (
    ScopeAssigned TicketScope = "assigned"
    ScopeGroup    TicketScope = "group"  // 2b
    ScopeAll      TicketScope = "all"    // 2b
)

type ScopeResolver interface {
    // EffectiveTicketScope 2a：无 user_orgs.ticket_scope 时默认 assigned
    EffectiveTicketScope(ctx context.Context, userID, orgID int64) (TicketScope, error)
    // VisibleOrgPaths 2b 实现；2a 返回 nil
    VisibleOrgPaths(ctx context.Context, userID int64) ([]string, error)

    // —— 2b 策略 B 增量（见 09-ticket §5.2）——
    // ReadAnchorPaths：实体透明读锚点 + scope group/all 路径的并集（L2 读）
    ReadAnchorPaths(ctx context.Context, userID int64) ([]string, error)
    // nearestEntityOrg：org_id 沿 parent_id 上溯最近的 org_type IN (1,2,3) 实体
    NearestEntityOrg(ctx context.Context, orgID int64) (*model.Organization, error)
    // scopePathsForMembership：单条 user_orgs 按 ticket_scope 计算可见 ltree 路径
    ScopePathsForMembership(ctx context.Context, m model.UserOrg) ([]string, error)
}
```

2a 实现：`EffectiveTicketScope` 恒返回 `assigned`（或读 `user_orgs.ticket_scope` 列若 migration 已提前加，但 **GetFilter 仍只实现 assigned 分支**）。

### 2.3 TicketResource

```go
// internal/service/ticket/resource.go

type TicketResource struct {
    repo   TicketRepo
    scope  ScopeResolver
}

func (r *TicketResource) Code() string { return "ticket" }
func (r *TicketResource) Actions() []string {
    return []string{"create", "read", "update", "delete", "assign", "close", "comment", "note"}
}

func (r *TicketResource) Authorize(ctx context.Context, req resource.AuthorizeRequest) (bool, error) {
    if util.HasRole(req.Roles, "admin") || util.HasRole(req.Roles, "superadmin") {
        return true, nil
    }
    if req.Action == "create" {
        return true, nil // 路由级已校验 ticket:create
    }
    ticketID, _ := strconv.ParseInt(req.ResourceID, 10, 64)
    ticket, err := r.repo.GetByID(ctx, ticketID)
    if err != nil {
        return false, err
    }
    // 第 2 层：可见性
    if !r.canRead(ctx, req.UserID, req.Roles, ticket) {
        return false, nil // Service 层转 404
    }
    // 第 3 层：操作权
    return r.canOperate(ctx, req.UserID, req.Roles, req.Action, ticket)
}

func (r *TicketResource) GetFilter(ctx context.Context, userID int64, action string) (resource.Filter, error) {
    // 2a：仅 assigned
    return resource.Filter{
        Where: "(created_by = $1 OR assigned_to = $1)",
        Args:  []interface{}{userID},
    }, nil
}
```

**canRead（2a）**：`created_by == userID || assigned_to == userID`。

**canOperate（2a）**：

| action | 条件 |
|--------|------|
| read / comment | canRead |
| update | 创建人或处理人 |
| close | 处理人 |
| assign | 2a 暂不开放主管分派，仅 admin bypass 或后续 2b `ticket_scope=group/all` 扩展 |
| delete | 仅 admin bypass |

> 2b 升级见 [09-ticket.md §5.2](./09-ticket.md#52-phase-2b-scope-升级-部门内读写分离策略-b默认)（**策略 B**：实体透明读 + update 仅创建人）；2c 组内委托见 [04-org-delegation §4](./04-org-delegation.md#4-authorize-升级step-10)。

**canRead（2b，策略 B 默认）**：

- `created_by == userID || assigned_to == userID`，**或**
- `ticket.org_path <@ ANY(ReadAnchorPaths(userID))`（实体 `entity_transparent_read` anchor + scope group/all 路径并集）
- 不可见 → Service 转 **404**

**canOperate（2b）**：

| action | 条件 |
|--------|------|
| read / comment | canRead |
| **update** | **`created_by == userID`**（兄弟组透明读 **不含** 处理人改他人工单） |
| close | `assigned_to == userID` OR `created_by == userID`；或 scope∈{group,all} 主管（子树内） |
| assign | admin bypass；或 scope∈{group,all} 且工单在其 scope 子树内 |
| delete | 仅 admin bypass |

**canOperate（2c 增量）**：在 2b 基础上，对 **`ticket.org_id`** 增加 org admin/owner、ancestor owner（见 04-org-delegation §4）。**不能**凭 vg_a admin 改 vg_b 工单。

### 2.4 Service 层调用约定

```go
// 单条读
ok, err := s.registry.Authorize(ctx, "ticket", resource.AuthorizeRequest{
    UserID: userID, Roles: roles, Action: "read", ResourceID: idStr,
})
if !ok {
    return nil, errcode.ErrNotFound // 404，统一文案
}

// 列表
filter, _ := s.registry.GetFilter(ctx, "ticket", userID, "read")
rows, _ := s.repo.List(ctx, filter, page, size)
```

### 2.5 Wire 自注册

```go
func NewTicketService(..., registry resource.Registry, ...) *TicketService {
    s := &TicketService{...}
    registry.Register(NewTicketResource(s.repo, s.scope))
    return s
}
```

---

## 3. 数据模型（2a）

2a **无** authz 专用新表。工单表在 [09-ticket.md §2a](./09-ticket.md#phase-2a-迁移) 迁移中创建，含：

- `created_by`、`assigned_to`
- `org_id`、`org_path`（创建时从组织冗余，供 2b ltree 过滤）

---

## 4. 测试用例

| # | 用例 | 预期 |
|---|------|------|
| R1 | Registry 注册 ticket | `List()` 含 ticket |
| R2 | GetFilter assigned | WHERE 含 created_by / assigned_to |
| R3 | 用户 A 列表 | 仅 A 创建或被分派的工单 |
| R4 | A 读 B 的工单详情 | **404** |
| R5 | A 更新自己的工单 | 200 |
| R6 | A 更新 B 的工单（可见但非属主） | **403** + 70001 |
| R7 | admin 读任意工单 | 200 |
| R8 | 无 ticket:list 路由权限 | **403** + 70001 |
| R9 | 2b 策略 B：vg_a member 读 vg_b | **200** |
| R10 | 2b 策略 B：vg_a member 改 vg_b | **403**（非创建人） |
| R11 | 2b `project_isolated` | vg_a **不可读** vg_b → **404** |
| R12 | 2b 创建人改自己 vg_a 工单 | **200** |

---

## 5. 涉及文件

```
internal/pkg/resource/registry.go       # 已有；补全 Authorize/GetFilter 测试
internal/service/authz/scope_resolver.go
internal/service/ticket/resource.go
internal/service/ticket/service.go      # 调用 registry
```

---

## 6. 待决策点

| 事项 | 建议 | 状态 |
|------|------|------|
| admin 资源 bypass | TicketResource 内 `HasRole(admin/superadmin)` 放行 | ✅ 建议 |
| assign 在 2a | 仅 admin；一线分派等 2b scope | ✅ 建议 |
| resource_owners | 2a 不建；工单用业务表冗余 org | ✅ 与 architecture 对齐 |

---

## 7. 文档交叉引用

| 文档 | 关系 |
|------|------|
| [09-ticket.md](./09-ticket.md) | 工单 MVP + 2b scope 升级 |
| [03-org-enhance.md](./03-org-enhance.md) | 2b ticket_scope + ltree |
| [04-org-delegation.md](./04-org-delegation.md) | 2c Authorize 扩展 |
