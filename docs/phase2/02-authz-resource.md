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
// internal/service/ticket/scope_resolver.go（2a：HasRole 辅助函数；assigned 语义已直接实现在 resource.go 的 canRead/GetFilter 中）

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

**现状（2026-08-28，Step 4/5 落地后）**：接口已重构为 `ReadAnchorPaths` + `ResolveScope`（透明锚点 ∪ group 作用域 ∪ all 开关，`ticket/scope_resolver.go`）；`user_orgs.ticket_scope` 已随 000012 落地。下方伪代码为 2b-org 设定稿的演进起点，实际形状以代码为准。

### 2.3 TicketResource

```go
// internal/service/ticket/resource.go

type TicketResource struct {
    repo   TicketRepo
    scope  ScopeResolver
}

func (r *TicketResource) Code() string { return "ticket" }
func (r *TicketResource) Actions() []string {
    return []string{"list", "create", "read", "update", "delete", "assign", "close", "comment", "note"}
    // 关联操作（建立工单关联）复用 update 权限码，不单独注册 relation（见 09-ticket §API）
}

func (r *TicketResource) Authorize(ctx context.Context, req resource.AuthorizeRequest) (bool, error) {
    if util.HasRole(req.Roles, "admin") || util.HasRole(req.Roles, "superadmin") {
        return true, nil
    }
    if req.Action == "create" || req.Action == "list" {
        return true, nil // 路由级已校验 ticket:create
    }
    ticketID, _ := strconv.ParseInt(req.ResourceID, 10, 64)
    ticket, err := r.repo.GetByID(ctx, ticketID)
    if err != nil {
        return false, err
    }
    // L2 可见性（数据访问边界门，先于属主；scope=assigned 即仅属主可见）
    if !r.canRead(ctx, req.UserID, req.Roles, ticket) {
        return false, nil // Service 层转 404
    }
    // L3 属主 + canOperate（动作权；内部用属主判断决定 read/comment/update/close 放行，assign/delete 属主也不放行）
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
| note | 创建人或处理人 |
| update | 创建人或处理人 |
| close | 处理人或创建人 |
| assign | 2a 暂不开放主管分派，仅 admin bypass 或后续 2b `ticket_scope=group/all` 扩展 |
| delete | 仅 admin bypass |

> 2b 升级见 [09-ticket.md §5.2](./09-ticket.md#52-phase-2b-scope-升级-部门内读写分离策略-b默认)（**策略 B**：实体透明读 + update 仅创建人）；2c 组内委托见 [04-org-delegation §4](./04-org-delegation.md#4-authorize-升级step-9)。

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
// 现行签名（2c 起新增第三参 OrgDelegationChecker——工单委托判定，wire 绑定 OrgDelegationService）
func NewTicketService(..., registry resource.Registry, ...,
    delegation ticket.OrgDelegationChecker) *Service {
    s := &Service{db: db, ticketRepo: ticketRepo, ...}
    registry.Register(NewResource(s.ticketRepo, NewPgxScopeResolver(db), delegation))
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

| # | 用例 | 预期 | 2a 状态 & 落点 |
|---|------|------|----------------|
| R1 | Registry 注册 ticket | `List()` 含 ticket | ✅ PASS; NewTicketService 内 `registry.Register(NewResource(...))`，wire 注入顺序校验于 `internal/app/wire_gen.go:71-73` |
| R2 | GetFilter assigned | WHERE 含 created_by / assigned_to | ✅ PASS; `resource.go` GetFilter——2b 升级为 `(created_by=$1 OR assigned_to=$1 OR org_path <@ ANY($2::ltree[]))`（无锚点时退化为 assigned 语义） |
| R3 | 用户 A 列表 | 仅 A 创建或被分派的工单 | ✅ PASS; 服务真表=`TestTicket_R3_AssignedScopeList`; HTTP=`acceptance-phase2a.sh` §T2 |
| R4 | A 读 B 的工单详情 | **404** | ✅ PASS; 服务真表=`TestTicket_R4_InvisibleReturns404` (code=90001); HTTP=`acceptance-phase2a.sh` §T3 |
| R5 | A 更新自己的工单 | 200 | ✅ PASS; 服务真表=`TestTicket_R5_UpdateOwn`; HTTP=`acceptance-phase2a.sh` §T4 |
| R6 | A 更新 B 的工单（可见但非属主） | **403** + 70001 | ✅ PASS; 服务真表=`TestTicket_R6_VisibleNotOwner_403` (assign/delete → ErrNoPermission 70001); HTTP=`acceptance-phase2a.sh` §R6 |
| R7 | admin 读任意工单 | 200 | ✅ PASS; 服务真表=`TestTicket_R7_AdminBypass`; HTTP=`acceptance-phase2a.sh` §R7 admin 列表 & 详情 |
| R8 | 无 ticket:list 路由权限 | **403** + 70001 | ✅ PASS; service 层 assigned scope 行为见 `TestTicket_R8_ViewerServiceLayerAssigned`；**真正 L1 Casbin 拦截** 由 HTTP `acceptance-phase2a.sh` §T7（viewer GET/POST → 403 Casbin 中间件）覆盖 |
| R9 | 2b 策略 B：vg_a member 读 vg_b | **200** | ✅ PASS（Step 5）；`TestB2Org_R9R10_VgSiblingReadWrite` |
| R10 | 2b 策略 B：vg_a member 改 vg_b | **403**（非创建人） | ✅ PASS（Step 5）；`TestB2Org_R9R10_VgSiblingReadWrite`（update/note 均 403，comment 放行） |
| R11 | `project_isolated`（**future，2b-core 不交付**） | vg_a **不可读** vg_b → **404** | ⏳ 延期 Phase 3 |
| R12 | 2b 创建人改自己 vg_a 工单 | **200** | ✅ 创建人半边 Step 4 已验（`TestB2_WriteSeparation` R12 段）；vg 形态待 Step 5 |

**测试落点约定**（B4）：R1/R2 单测 → `internal/pkg/resource/registry_test.go`；R3–R8 集成测试 → `internal/service/ticket/authz_resource_integration_test.go`（真表 testcontainers PG，复用 phase1 `testutil` 模式；`internal/testutil/testdb_integration.go` 已追加 `000010_ticket.up.sql` 迁移列表）；中间件顺序回归 + Casbin L1 → `scripts/acceptance-phase2a.sh` Section A（Phase1 27 例回归 P2-D5）+ §T7（R8 viewer 403）。

---

## 5. 涉及文件

```
internal/pkg/resource/registry.go       # 已有；补全 Authorize/GetFilter 测试
internal/service/ticket/scope_resolver.go（2a：HasRole 辅助函数；assigned 语义已直接实现在 resource.go 的 canRead/GetFilter 中）
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
