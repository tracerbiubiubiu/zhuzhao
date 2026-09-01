# 04 - 组织内委托（org-delegation，Phase 2c）

> **Step 8–10**（正文 §6 编号：8=org-delegation 成员分级、9=Authorize、10=集成验收；09-ticket §0 引用的"Step 9"即此处 Step 9），依赖 Phase **2b-core + 2b-org** 验收（虚拟组、`ticket_scope`、TicketResource group/all 过滤；HR Sync 属 2b-ext 延后，非前置）。  
> 设计背景见 [03-org-enhance §组织负责人](./03-org-enhance.md#组织负责人与组内分级你描述的场景)；**本文档为 2c 实现 SSOT**。

---

## 0. 边界与分期决策

### 做什么

| 能力 | Step | 说明 |
|------|------|------|
| `owner_user_ids` | 8 | 实体部门 / 虚拟组均可指定负责人（可多人） |
| `user_orgs.org_member_role` | 8 | 组内 `owner` / `admin` / `member`，与全局 `user_roles` 分离 |
| 组内防提权 API | 8 | admin 只能管 member；任命 admin 仅 owner |
| 成员 API 委托扩展 | 8 | `AddMember` / `RemoveMember` 增加组内级别校验 |
| 工单 Authorize 升级 | 9 | 负责人 / 组 admin 对绑定 org 的资源 update/delete |
| 集成验收 | 10 | D1–D12 |

### 不做（留在 2b 或更晚）

| 不做 | 原因 | 阶段 |
|------|------|------|
| 虚拟组 CRUD / HR Sync | 已在 2b | 2b |
| `org_roles` → Casbin | 全局能力，非组内委托 | 2b |
| 泛化 ResourceRegistry 到非工单资源 | 2c 只升级 TicketResource | 按需 |
| 组内自定义 permission scheme | 过重；2c 用固定矩阵 | 按需 |

### 为何不继续拆 2d

Step 8（成员分级）与 Step 9（Authorize）**强耦合**：D7–D9 同时需要 `org_member_role` 与资源 Authorize；拆成两个 phase 会出现「能任命 admin 但不能管工单」的半交付状态。2c 仅 3 个 Step，体量可控，**保持 2c 单节点**。

---

## 1. 前置条件（2b-core + 2b-org 必须已验收）

- [ ] 虚拟组 `org_type=4` CRUD + 成员 + `expires_at`（2b-org）
- [ ] `user_orgs.ticket_scope`：`all` / `group` / `assigned`（2b-core）
- [ ] TicketResource `GetFilter` 支持 group/all（ltree `<@`）（2b-core）
- [ ] 工单带 `org_id` + `org_path` 字段
- [ ] HR Sync Job（**2b-ext 延后**，非 2c 前置；Phase 2 组织数据可种子/手工维护）

---

## 2. 数据模型

### 2.1 Schema 增量

```sql
-- organizations：负责人（可多人；Phase 2c 用数组，后续可迁 org_owners 关联表）
ALTER TABLE organizations
  ADD COLUMN owner_user_ids BIGINT[] NOT NULL DEFAULT '{}';

-- user_orgs：组内级别（与全局 user_roles 分离）
ALTER TABLE user_orgs
  ADD COLUMN org_member_role VARCHAR(20) NOT NULL DEFAULT 'member';
  -- CHECK (org_member_role IN ('member', 'admin', 'owner'))

CREATE INDEX idx_user_orgs_org_role ON user_orgs (org_id, org_member_role)
  WHERE org_member_role IN ('admin', 'owner');
```

**迁移文件**：`migrations/0000xx_org_delegation.up.sql`（版本号随仓库递增）。

### 2.2 组内 priority（与全局 roles.priority 同语义）

| org_member_role | priority | 说明 |
|-----------------|----------|------|
| `owner` | 1 | 容器最高委托；可任命 admin、删虚拟组、管全部绑定资源 |
| `admin` | 10 | 只能增删 **member**；不能升降 admin/owner |
| `member` | 20 | 默认；使用资源，无成员管理权 |

**EffectiveOrgPriority(user, org)** = 若 `user_id ∈ org.owner_user_ids` 则视为 `owner`（priority 1），否则取 `user_orgs.org_member_role` 对应 priority。

> `owner_user_ids` 与 `org_member_role=owner` **双轨对齐**：`SetOwners` 成功时，对每个 owner 确保存在 `user_orgs` 行且 `org_member_role='owner'`（无行则 INSERT，有行则 UPDATE）。

### 2.3 HR 同步隔离（D10）

HR Job **不得**写入或覆盖：

- `organizations.owner_user_ids`（实体负责人由 HR 字段映射或本地指定，写入策略见 [hr-directory-sync.md §5](../proposal/hr-directory-sync.md) 扩展）
- 虚拟组（`source=local`）的全部 `owner_user_ids`
- 任意 `user_orgs.org_member_role`（始终 IAM 本地字段）

---

## 3. API 设计

### 3.1 接口一览

| 功能 | API | Step | 路由级权限 |
|------|-----|------|-----------|
| 设置负责人 | `POST /api/v1/orgs/owners` | 8 | `org:update` **或** 该 org 的 effective owner |
| 任命/变更组内角色 | `POST /api/v1/orgs/members/role` | 8 | 该 org 的 effective owner |
| 添加成员（扩展） | `POST /api/v1/orgs/members` | 8 | `org:update` **或** org admin/owner |
| 移除成员（扩展） | `POST /api/v1/orgs/members/delete` | 8 | 同上 + 组内防提权 |
| 成员列表（扩展） | `GET /api/v1/orgs/:id/members` | 8 | `org:read` |
| 组织详情（扩展） | `GET /api/v1/orgs/:id` | 8 | 返回 `owner_user_ids` |
| 删除虚拟组（委托） | `POST /api/v1/orgs/delete` | 8 | 虚拟组 owner 可删（D6） |

### 3.1.1 路由级权限实现策略（P2-2/B1，2c 开工前置已决策）

上表「路由级权限」列的落地方式（**L1 与 L3 分层，不混一轴**）：

1. **L1（Casbin 路由级）**：本节全部新路由**复用既有权限码**，不新增权限码——
   - 写路由（owners / members/role / members / members/delete / delete）→ 挂在既有组织管理菜单（`org:update` 系 menu_apis）；
   - 读路由（members 列表 / 组织详情）→ `org:read` 系（`org:read` 菜单）。
   - 效果：持组织管理菜单的角色（admin/operator+ 等）L1 放行；owner/admin **不是** Casbin 角色，L1 对其天然放行与否取决于其全局角色——因此 effective owner/admin 的**细粒度校验全部下沉 L3（service 层）**。
2. **L3（service 层 effective 校验）**：`OrgDelegationService.IsOrgAdminOrOwner / IsAncestorOwner` 在 service 内校验；校验失败 → 403 + 50008–50010（防提权矩阵见 §4）。
3. **安全性**：L1 只负责「能不能调这个 API」，细粒度（谁能动哪个 org）由 L3 保证——与工单三层鉴权（09 §5）同构；L2 对组织资源不适用（组织本身即容器）。

> **落地注记（2026-08-28，Step 8 实施回标）**：
> 1. 五条委托路由（owners / members/role / members / members/delete / delete）挂 **authed 级 SelfService 标记组**（L1=认证，跳过 Casbin enforce）——纯菜单 L1 会把无全局菜单的 owner/admin 挡在 L3 之前；「org:update」全局侧由 `OrgDelegationService.HasOrgManagePermission`（org:* 权限码，BFS 有效角色）在 L3 等价判定。
> 2. **D6 语义细化**：虚拟组的 owner 成员行是 SetOwners 双轨派生数据，不构成 50005 成员占位——仍有非 owner 成员 → 409+50005；仅剩 owner 行 → 预清派生行后删除。
> 3. **D9 L2 前提**：ancestor owner 须经双轨成员行获得实体锚点（透明读）方可见子树工单——生产 SetOwners 天然满足；裸写 owner_user_ids 不建成员行将不可见（测试实证）。

### 3.2 `POST /api/v1/orgs/owners`

```json
{
  "org_id": "42",
  "owner_user_ids": ["5", "8"]
}
```

**逻辑**：

1. 校验 org 存在；各 user 存在且未软删
2. 调用方：全局 `org:update` **或** `EffectiveOrgPriority(caller, org) == owner`
3. 非 owner 调用方不可移除仍在列表中的现有 owner（防自我降权踢人）— 仅 global `org:update` 可清空 owner
4. 事务：`UPDATE organizations SET owner_user_ids = $1`
5. 对每个 owner：`EnsureMember(org, user, org_member_role=owner)`
6. 从 owner 列表移除的用户：若仅因 owner 身份为 owner 角色，降为 `member`（仍保留成员关系则保留 member 行）

**响应**：200 + 更新后 org 摘要（含 `owner_user_ids`）

### 3.3 `POST /api/v1/orgs/members/role`

```json
{
  "org_id": "42",
  "user_id": "10",
  "org_member_role": "admin"
}
```

**逻辑**：

1. 目标用户必须是 org 成员（否则先 `AddMember` 或 404 + 50007）
2. 调用方 `EffectiveOrgPriority` 必须为 **owner**（admin **不可**调用本接口 — D3）
3. 不可将目标设为 `owner`（owner 仅通过 `SetOwners`）；若请求 `owner` → 400
4. 不可修改 `owner_user_ids` 中用户的 `org_member_role`（derived owner 不可被降级）— 403 + 50009
5. 成功：`UPDATE user_orgs SET org_member_role = $1`

| 场景 | HTTP | code |
|------|------|------|
| admin 调用 | 403 | 50010 |
| admin 任命 admin | 403 | 50008 |
| owner 任命 admin | 200 | 0 |

### 3.4 扩展 `POST /api/v1/orgs/members`

请求体增加可选字段：

```json
{
  "org_id": "42",
  "user_id": "10",
  "is_primary": false,
  "org_member_role": "member"
}
```

- 默认 `org_member_role=member`
- 仅 **owner** 可指定 `admin`；admin 调用时传 `admin` → 403 + 50008
- admin/owner 可添加 member（D4 路径）

### 3.5 扩展 `POST /api/v1/orgs/members/delete`

在现有删除逻辑前增加组内校验：

```
callerPriority = EffectiveOrgPriority(caller, org)
targetPriority = EffectiveOrgPriority(target, org)  // 含 owner_user_ids

若 callerPriority == admin (10):
  若 targetPriority <= admin (10) → 403 + 50009   // D5
若 callerPriority == owner (1):
  允许（D4、D5 owner 删 admin）
若 caller 仅 member:
  403 + 70001（无 org 成员管理权）
```

全局 `org:update` 始终可绕过组内校验（平台管理员）。

### 3.6 虚拟组删除（D6）

`POST /api/v1/orgs/delete` 扩展：

- `org_type=4` 且调用方为 effective owner：**允许**删除（仍检查无子 org；成员可先清空或一并拒绝 — **建议**：有成员 → 409 + 50005，与 Phase 1 一致）
- 实体组织删除规则不变（Phase 1 / 2b）

---

## 4. Authorize 升级（Step 9）

### 4.1 三轴不混

| 轴 | 2c 是否变更 | 说明 |
|----|------------|------|
| 路由级 Casbin | 否 | 仍须 `ticket:update` 等 |
| 列表 scope（GetFilter） | **2b 已定** | **策略 B** 默认：`ReadAnchorPaths` 实体透明读 + scope 并集；2c 不改动 |
| 单资源操作（CheckOwner / Authorize） | **是** | 本 Step 核心；**2b 已限定 update=创建人**，2c 加 org admin/owner |

### 4.2 工单操作矩阵（2c 后）

在 [modules/ticket.md §2.2](../modules/ticket.md#22-三层鉴权在工单中的映射) 基础上，**第 3 层属主**扩展为：

| 操作 | 路由级 | scope 可见 | 属主 / 委托条件（2c） |
|------|--------|-----------|----------------------|
| update | `ticket:update` | 可见 | **2b：创建人**；2c + **org admin·owner** / **ancestor owner + 子树**（不含仅透明读的处理人） |
| close | `ticket:close` | 可见 | 处理人 / 创建人（2b）；+ **org admin·owner** / ancestor owner |
| delete | `ticket:delete` | — | 全局 admin / **org admin·owner** / ancestor owner |
| assign | `ticket:assign` | 可见 | 原「主管」+ **org admin·owner** / **ancestor owner + 子树** |

**ancestor owner（D9）**：

```sql
-- 用户 U 对工单 T 是否具备 ancestor owner 委托
EXISTS (
  SELECT 1
  FROM organizations o
  JOIN organizations ancestor ON ancestor.path @> o.path
  WHERE o.id = T.org_id
    AND U = ANY(ancestor.owner_user_ids)
)
AND T.org_path <@ o.path  -- 或 ancestor.path，与 ticket 存 path 一致
```

**org 内 admin/owner（D7–D8）**：

```sql
EXISTS (
  SELECT 1 FROM user_orgs uo
  WHERE uo.user_id = $user
    AND uo.org_id = T.org_id
    AND uo.org_member_role IN ('admin', 'owner')
)
-- 或 user_id = ANY((SELECT owner_user_ids FROM organizations WHERE id = T.org_id))
```

### 4.3 `CheckOwner` 伪代码

```go
// 实际实现（2026-08-28 对齐）：per-action 判定，勿用「属主短路」旧稿——
// 处理人可 close 但不可 update（RK-11），委托者按 §4.2 矩阵逐动作放行
func (s *ticketService) canOperate(ctx context.Context, userID int64, action string, t *Ticket) (bool, error) {
    isCreator := t.CreatedBy == userID
    isAssignee := t.AssignedTo != nil && *t.AssignedTo == userID

    switch action {
    case "read", "comment":
        return true, nil // L2 可见性已通过（策略 B）
    case "note":
        return isCreator || isAssignee, nil // 读写集合一致（BK-1）
    case "update":
        if isCreator { return true, nil }        // 2b：仅创建人（RK-11）
    case "close":
        if isAssignee || isCreator { return true, nil }
    case "assign":
        if scope.AllScope || pathInAnchors(t.OrgPath, scope.ScopePaths) { return true, nil } // 2b 主管
    case "delete":
        // 2b：仅 admin bypass（org admin/owner 走 2c 委托，见下）
    }
    // 2c 组织委托（admin·owner 精确匹配本 org；ancestor owner 管子树）
    if s.orgDelegation.IsOrgAdminOrOwner(ctx, userID, t.OrgID) { return true, nil }
    if s.orgDelegation.IsAncestorOwner(ctx, userID, t.OrgID, t.OrgPath) { return true, nil }
    return false, nil // 可见但无权 → 403+70001；不可见 → 404（上游 L2）
}
```

> **与旧稿的差异**：旧伪代码 `CreatedBy||AssignedTo → true` 对所有动作短路，暗示处理人可 update——与 §4.2 矩阵（update=创建人）矛盾。本版按动作分派，与实现（`ticket/resource.go canOperate`）逐行对齐。

**不可见工单**仍返回 **404**（非 403）；可见但无权操作返回 **403 + 70001**。

### 4.4 涉及文件

```
internal/service/org_delegation.go      # EffectiveOrgPriority、IsOrgAdminOrOwner、IsAncestorOwner
internal/service/org_service.go       # SetOwners、SetMemberRole；扩展 Add/RemoveMember
internal/service/ticket_service.go    # CheckOwner 调用 org_delegation
internal/handler/org_handler.go       # 新路由
internal/repository/org_repo.go       # owner_user_ids、org_member_role CRUD
migrations/0000xx_org_delegation.up.sql
```

---

## 5. 错误码（Phase 2c 新增）

| code | 常量 | message | HTTP | 场景 |
|------|------|---------|------|------|
| 50008 | `ErrCannotAssignHigherOrgMemberRole` | 不能分配更高的组内角色 | 403 | D3；admin 添加 admin |
| 50009 | `ErrCannotManageOrgMember` | 无权管理该组织成员 | 403 | D5；admin 删 admin |
| 50010 | `ErrNotOrgOwner` | 需要组织负责人权限 | 403 | 非 owner 调用 SetMemberRole / SetOwners（非全局） |

> 写入 `internal/pkg/errcode/errcode.go` 与 [api/errcode.md](../api/errcode.md)；**勿改号**。

---

## 6. 实施顺序（与 phase2/README Step 8–10 对齐）

```
2b-core + 2b-org 验收通过
   │
   ├── Step 8: org-delegation
   │      ├── migration + model
   │      ├── OrgDelegationService（priority 计算）
   │      ├── SetOwners / SetMemberRole API
   │      ├── AddMember / RemoveMember / Delete 扩展
   │      └── 单元测试：priority + 防提权
   │
   ├── Step 9: ticket Authorize
   │      ├── TicketResource CheckOwner 扩展
   │      ├── ancestor owner SQL
   │      └── 集成测试：D7–D9
   │
   └── Step 10: 集成验收 D1–D12 + HR 回归 D10（HR 回归待 2b-ext 落地后补）
```

| 子任务 | 验收 |
|--------|------|
| 8a migration | 幂等迁移可重复执行 |
| 8b SetOwners | D1 |
| 8c SetMemberRole + 防提权 | D2–D3 |
| 8d RemoveMember 扩展 | D4–D5 |
| 8e 虚拟组 owner 删除 | D6 |
| 9 Authorize | D7–D9 |
| 10 全量 | D1–D12 |

---

## 7. 测试用例（验收 SSOT）

| # | 用例 | 预期 |
|---|------|------|
| D1 | 设置虚拟组 owner | `owner_user_ids` 含负责人；对应 `user_orgs.org_member_role=owner` |
| D2 | 组 owner 任命组内 admin | 200 |
| D3 | 组 admin 任命 admin | **403 + 50008** |
| D4 | 组 admin 移除 member | 200 |
| D5 | 组 admin 移除另一 admin | **403 + 50009** |
| D6 | 组 owner 删空虚拟组 | 200 |
| D7 | 组 admin 删绑定该组的工单 | 200（Casbin + Authorize） |
| D8 | 组 member 删同工单 | **403** |
| D9 | 实体部门 owner 对子树 org 下工单 | ancestor owner：**200** |
| D10 | HR 同步 | **不覆盖** `owner_user_ids`、`org_member_role` |
| D11 | 策略 B：vg_a member 对 vg_b 工单（**2b-core 验收，2c 回归**） | **可读**（L2）；**不可 update**（L3，403）；vg_b **admin** 可改本组工单（D7） |
| D12 | 实体 `project_isolated`（**future，移出 2b-core 验收**） | 兄弟虚拟组工单 **404**（L2 强隔离） |

---

## 8. 待决策点

| 事项 | 建议 | 状态 |
|------|------|------|
| owner 存储 | Phase 2c 用 `owner_user_ids BIGINT[]` | ✅ 建议 |
| owner 降权 | 仅 global `org:update` 可清空 owner 列表 | ✅ 建议 |
| 删虚拟组有成员 | 拒绝删除（50005），与 Phase 1 一致 | ✅ 建议 |
| 实体部门 owner 来源 | HR 字段映射 **或** 本地 `SetOwners`；HR Job 不写 | 📋 2c 实现时二选一 documented in hr-directory-sync |
| 新路由权限码 | 复用 `org:update` / `org:read`，不新增 menu | ✅ 建议 |

---

## 9. 文档交叉引用

| 文档 | 关系 |
|------|------|
| [03-org-enhance.md](./03-org-enhance.md) | 2b 范围 + 设计背景 |
| [phase2/README.md](./README.md) §1.3、§2.3 | 子阶段总览与 Step 8–10 |
| [modules/organization.md](../modules/organization.md) | 组织模块完整形态 |
| [modules/ticket.md](../modules/ticket.md) | 三层鉴权与 scope |
| [hr-directory-sync.md](../proposal/hr-directory-sync.md) | D10 HR 隔离 |
