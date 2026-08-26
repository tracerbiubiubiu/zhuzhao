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
- [x] `internal/pkg/resource/registry.go` 接口已定义 — ⚠️ **但 Wire 注入未接通**（Phase 1 遗留 G-1：`wire.go` 已声明 `resource.NewRegistry` provider，但依赖图上无消费者，`wire_gen.go` 未生成实例化代码），由下方 Step 0 第一动作完成
- [ ] 组织 ltree 已存在（Phase 1），但 **2a 鉴权不使用 ltree 过滤**

### Step 0：Registry 接线（Phase 1 遗留收尾）

> 来源：[phase1/12 号报告](../phase1/12-phase1-acceptance-report.md) G-1。phase1/03-authz.md 计划「Phase 1 仅 wire 注入空 Registry」未落地——provider 声明 ≠ 注入生效（Wire 是编译期按需生成，无消费者的 provider 不会出现在 `wire_gen.go`）。

**任务**（进入 Step 1 前完成，改动量约 5 行）：

1. `internal/router/router.go` 的 `Deps` 结构体增加字段 `Registry resource.Registry`（依赖 `wire.Struct(new(router.Deps), "*")` 自动接线）；
2. `router.New()` 中遍历 `deps.Registry.List()` 输出启动期资源清单日志（`resource registered`：code/name/actions）——Phase 1 空表零输出，Phase 2a TicketResource 自注册后自动可见，作为装配自检手段；
3. `router/router_test.go` 手工构造 `Deps` 处补 `Registry: resource.NewRegistry()` 字段（fail-fast：漏传启动即 panic，暴露装配错误）；
4. `make wire` 重新生成 `wire_gen.go`（预期出现 `registry := resource.NewRegistry()` 实例化 + `Deps` 赋值两处）。

**验证**：`grep NewRegistry internal/app/wire_gen.go` 出现 2 处；`go build ./... && go test -race ./internal/router/...` 通过。

**同批处理 G-2**（见 §5 涉及文件）：删除 `internal/service/authz_service.go` 旧 stub 并修正 `architecture.md` 三处旧描述。

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

**鉴权分层决策声明**（何时走 Casbin、何时走 Registry）：

| 判断维度 | 走 Casbin（路由级） | 走 Registry（资源级） |
|----------|--------------------|--------------------|
| 回答的问题 | 「这个用户能不能调这个接口」 | 「这个用户能不能碰这条数据」「列表该给他看哪些行」 |
| 依据 | 角色绑定的菜单 → `menu_apis` → 路由策略（p 表） | 工单属主 / ticket_scope / org_path |
| 形态 | 中间件统一拦截（声明式，策略可热加载） | Service 层显式调用 `Authorize` / `GetFilter`（代码内联） |

1. **列表过滤不进 Casbin**：Casbin 表达不了「`created_by = $user OR assigned_to = $user`」这类行级谓词（KeyMatch2 只做路径匹配）；每资源独立 Enforcer 已明确后移 Phase 3（见 [README §1.5](./README.md#15-不做什么整个-phase-2)）。因此路由级只做「有没有 ticket:list」，行级过滤必须由 `GetFilter` 在 Repository 查询中拼接 WHERE 完成。
2. **单条操作两层都要过**：路由级（`ticket:update`）挡住无权限码者；资源级 `Authorize` 再做属主判断。中间件不可见数据细节，Service 不可见路由策略——职责正交，不重复不缺口。
3. **admin bypass 双机制锚点一致**：路由级靠种子 g/p 策略（`role::admin` 通配）；资源级在 `TicketResource.Authorize` 硬编码 `HasRole(admin/superadmin)`。两处机制实现不同，但均以角色码为锚——新增全局管理角色时需两处同步，此为已知 tradeoff。

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
    // L3 属主短路：属主命中则跳过 L2（转部门后仍能看旧工单）
    // 注意：属主命中 ≠ 能做所有动作，仍需过 canOperate
    ownerHit := r.isOwner(ctx, req.UserID, ticket)
    if !ownerHit {
        // L2 可见性：非属主走组织关系判定
        if !r.canRead(ctx, req.UserID, req.Roles, ticket) {
            return false, nil // Service 层转 404
        }
    }
    // canOperate：无论 L3 命中还是 L2 命中都要过（属主不等于能改）
    return r.canOperate(ctx, req.UserID, req.Roles, req.Action, ticket, ownerHit)
}

// isOwner L3 属主判断：资源行上的列比较，不查组织关系
func (r *TicketResource) isOwner(ctx context.Context, userID int64, ticket *Ticket) bool {
    return ticket.CreatedBy == userID || ticket.AssignedTo == userID
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

> **L3 属主短路 + canOperate 关系**（对齐 [11-authz §2.2](./11-authz-architecture-review.md#22-已确认的语义负责人表态)）：`Authorize` 先判 `isOwner`（L3），命中则跳过 `canRead`（L2）；但无论 L3/L2 谁命中，都要过 `canOperate`。`canOperate` 的 `ownerHit` 参数让实现知道属主是否命中——属主对 `read/comment` 必放行，对 `update` 需区分创建人 vs 处理人，对 `assign/close` 属主不一定有权（需 scope 主管）。

**canOperate（2a）**（`ownerHit` = L3 属主是否命中）：

> **逐动作 ownerHit 规则**（P2-5 修复）：属主命中（`ownerHit=true`）不等于能做所有动作。下表逐动作定义属主命中时的放行规则：

| action | ownerHit=true（属主） | ownerHit=false（非属主，走 L2） | 说明 |
|--------|------|------|------|
| read / comment | ✅ 放行 | 需 L2 canRead 通过 | 属主必能读/评论自己的工单 |
| update | ✅ 放行（创建人改自己的） | 需 L2 + scope 主管 | 属主可改标题/字段；处理人也可改（视为协作者） |
| close | ✅ 放行（创建人或处理人） | 需 L2 + scope 主管 | 属主可关闭自己的工单 |
| assign | ❌ **不放行** | 仅 admin bypass（2b 扩展 scope 主管） | **属主不能分派**——分派是管理动作，需 admin/scope 主管权限 |
| delete | ❌ **不放行** | 仅 admin bypass | 属主不能删自己的工单（防误删） |

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

> Wire 语义保证：同一 provider（`NewRegistry`）单例；生成代码按依赖拓扑序执行，`TicketService` 构造（自注册）必然先于 `router.New`（消费 `Deps.Registry`），启动期日志可见完整资源清单。前提是 Step 0 接线完成。

### 2.6 Casbin 策略规模边界（Phase 2 现状声明）

Phase 1 的策略同步方式为**写后全量 `LoadPolicy()`**（`AssignMenus` / `DeleteRole` 事务提交后调用），无 Watcher。Phase 2 该方式的边界：

| 量级 | 说明 | 结论 |
|------|------|------|
| 角色数 × 绑定菜单 API 数 | 全量加载行数 ≈ Σ(每角色绑定的 menu_apis 行)。Phase 2 内部系统典型量级（<100 角色 × <500 API）为万行以内 | 全量 `LoadPolicy()` 单次 <100ms，**可接受** |
| 多实例部署 | 实例 A 改策略，实例 B 感知不到（无 Watcher） | Phase 2 单实例 Compose 拓扑无此问题；**多实例是引入 Watcher 的触发条件** |
| 触发时机 | 直接改 `casbin_rule` 表（DBA 手工）需重启进程或触发一次 AssignMenus | 运维须知（Phase 1 审查 P2-3，记录在 [phase1/11 §3.4](../phase1/11-code-review.md)） |

**决策**：Watcher 后移 Phase 3（对齐 [phase3/README](../phase3/README.md)）；Phase 2 内不引入。若 2b/2c 期间角色菜单绑定操作频次显著上升（如 HR 同步批量改组），再评估增量加载。

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

**测试落点约定**（B4）：R1/R2 单测 → `internal/pkg/resource/registry_test.go`；R3–R8 集成测试 → `internal/service/ticket/`（testcontainers PG，复用 phase1 `testutil` 模式）；中间件顺序回归 → `internal/router/router_test.go` 扩展 ticket 路由。

---

## 5. 涉及文件

```
internal/pkg/resource/registry.go       # 已有；补全 Authorize/GetFilter 测试
internal/service/authz/scope_resolver.go
internal/service/ticket/resource.go
internal/service/ticket/service.go      # 调用 registry
```

**Step 0 一并处置（Phase 1 遗留 G-2）**：

1. **删除 `internal/service/authz_service.go`**：其中 `CheckResourcePermission` 为骨架时代按旧设计预埋的 stub（"not implemented"），已被 [proposal/resource-model.md](../proposal/resource-model.md) 确立的 ResourceRegistry 模式取代——签名（单 roleKey、无 GetFilter）无法满足 2a 需求，保留会形成双轨误导。同批从 `wire.go` 的 provider set 移除 `NewAuthzService`，`make wire` 再生成；
2. **修正 `docs/design/architecture.md` 三处旧描述**（L77 交互契约、L440 Wire DI 图、L502 接口表）为 ResourceRegistry 模式——✅ 已于 2026-08-19 提前完成（全文档一致性核查），Step 0 时仅需核对；

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
