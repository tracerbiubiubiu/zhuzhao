# 03 - 组织增强（org-enhance，Phase 2b-org）

> 虚拟组、组织 scope、组织角色、BFS 三源角色、临时成员。与 HR 目录同步同批落地 Schema，详见 [proposal/hr-directory-sync.md](../proposal/hr-directory-sync.md)。
> **2026-08-26 调整**：本文档归属 **2b-org**（与 2b-core 工单可见性并行）；HR 目录同步降为 **2b-ext 延后**（见 P2-D7）。

### 子阶段边界映射表（落实审查 P3-16）

| 03 小节 | 能力 | 子阶段归属 |
|---------|------|-----------|
| §2 虚拟组 CRUD | 虚拟组 + Reparent | 2b-org（Step 5） |
| §3 scope 枚举过滤 | `ticket_scope` 枚举 | 2b-core（Step 4，Ticketing 侧消费） |
| §4 组织角色边界 | `org_roles` | 2b-org（Step 5） |
| §5 BFS RoleFetcher | 三源角色展开 | 2b-org（Step 5） |
| §6 HR Sync 同步 | 目录拉取 | **2b-ext（延后，Step 7b）** |
| §7 Reparent 重挂 | 组织树重挂 | 2b-org（Step 5） |

---

## 预期功能

| 功能 | 场景 | 阶段 |
|------|------|------|
| 虚拟组 CRUD | 在实体部门下创建项目组（org_type=4） | 2b-org |
| 虚拟组成员 | 跨部门加人、`expires_at` 临时成员 | 2b-org |
| 组织 scope | `user_orgs.ticket_scope`：all / group / assigned | 2b-core（工单可见性本体，见 [09-ticket §2b](./09-ticket.md)） |
| 组织角色 | `org_roles` 表，BFS 源 2 | 2b-org |
| BFS 三源角色 | 直接角色 + 组织角色 + 继承（RoleFetcher 扩展） | 2b-org |
| Reparent 虚拟组 | HR 撤销部门时虚拟组上挂 | 2b-org |
| **HR 目录同步** | 每日拉取公司人员/部门 API | **2b-ext（延后）**：Phase 2 组织数据可用种子/手工维护，不阻塞主线 |

### 2b 不做（核心 + org）

| 功能 | 原因 | 阶段 |
|------|------|------|
| 实时 HR 同步 | 每日 batch 足够；**Phase 2 延后**，组织数据先种子/手工维护 | 2b-ext（延后） |
| HR 管理虚拟组 | 虚拟组纯本地 | — |
| 多父级组织 | ltree 单父 | 按需 |
| **组内 member/admin 完整验收** | 独立 **Phase 2c**（不并入 2b） | 2c | 见 §组织负责人、测试 D1–D12 |

---

## 核心设计思路

### 虚拟组与实体组织

- **统一** `organizations` 表，`is_virtual` 布尔区分：`false` 实体，`true` 虚拟组（000019 收敛原 `org_type` 1–3 实体 / 4 虚拟组——行为消费点仅区分实体/虚拟，1/2/3 层级细分零代码消费，层级由 path/nlevel 表达）。
- 虚拟组 **必须** 挂在实体（或 system 根）下；path 为 ltree 子节点。
- 完整挂载、HR 同步、撤销 reparent 规则见 **[hr-directory-sync.md](../proposal/hr-directory-sync.md)**（必读）。

### Schema 增量

`source` / `external_id` / `synced_at`（组织与用户）、`user_orgs.is_primary` / `source` / `expires_at` — DDL 见 hr-directory-sync §2。

**Phase 2b 增量**（工单可见性，策略 B）：

```sql
ALTER TABLE organizations ADD COLUMN ticket_visibility VARCHAR(30) NOT NULL DEFAULT 'entity_transparent_read';
-- CHECK (ticket_visibility IN ('entity_transparent_read'))   -- future 扩展时再加 project_isolated（与 09 §5.2.1 一致）
-- 仅实体（is_virtual=false，000019 起布尔化）有效；虚拟组继承最近实体祖先配置
```

### HR Sync Job

- 接口：`HRDirectoryClient` + `HRSyncService`（hr-directory-sync §5.1）。
- 只写 `source=hr`；虚拟组 `source=local` 永不进入 HR 删库对账。
- 实体 move → 子树 path 级联 **含虚拟组**。

### 权限

- 实体部门：`group` / `all` scope 用 ltree `<@`；**默认** `ticket_visibility=entity_transparent_read`（**策略 B**：兄弟虚拟组 **可读不可改**）。
- 虚拟组：成员与 scope 绑在虚拟组 `user_orgs`，**不**随 HR 父部门角色继承；L2 读扩大到 **挂载实体** anchor，L3 写仍绑 **工单 `org_id`**。
- 强隔离项目（**future**）：实体设 `ticket_visibility=project_isolated`，L2 回退为仅直接 org path + `ticket_scope`（兄弟虚拟组互不可见）。**2b-core 不交付，待真实需求出现再加**。详见 [09-ticket §5.2.1](./09-ticket.md#521-实体-ticket_visibility)。

### 组织负责人与组内分级（你描述的场景）

> **不是特例**，是 IAM + 业务资源里最常见的 **「容器（组织/项目）+ 负责人 + 组内角色」** 模型。Phase 1 **不做**；**Phase 2c** 交付（依赖 2b 虚拟组 + scope）；设计备忘见下表。

#### 你的场景（复述对齐）

```text
实体部门（HR）或虚拟项目组（本地）
  ├── 负责人：对该容器下、绑定到该容器（或子容器）的资源有管理权
  │            例：开发组 ↔ 项目 A/B 的工单、配置等
  └── 成员分级
        ├── 组管理员：删项目、任命管理员、增删成员……
        └── 普通成员：使用资源；管理员只能管普通成员，不能动其他管理员
```

#### 业界常见拆法（三轴，不要混成一轴）

| 轴 | 回答什么 | 典型产品 |
|----|----------|----------|
| **全局 RBAC** | 能不能调 `POST /tickets` 这类 API？ | Casbin / Jira「全局权限」 |
| **容器成员级别** | 在这个项目/组里是 owner、admin 还是 member？ | Jira **Project Role**；Google Group **Owner/Manager/Member**；Azure AD **Group Owner** |
| **数据范围 scope** | 列表里能看见哪些行？ | Freshdesk **Role + Scope**；若依 **数据权限** |

推荐映射到本项目：

| 概念 | 存储（规划） | 作用 |
|------|--------------|------|
| **组织/项目负责人** | `organizations.owner_user_ids`（或 `org_owners` 关联表） | 容器级最高委托；可任命组内 admin、删容器、管全部资源与成员 |
| **组内分级** | `user_orgs.org_member_role`：`owner` / `admin` / `member` | **仅在该 org 内**有效；admin 只能操作 `member`，不能升降其他 admin（类似全局 `priority` 防提权） |
| **功能权限（可选）** | `org_roles` → Casbin | 进组自动获得「工单处理员」等**全局菜单/API 能力** |
| **资源可见/可操作** | 资源带 `org_id` + `TicketResource.Authorize` | 负责人/组 admin 对 **绑定在该 org 下** 的资源 CRUD；列表用 `ticket_scope` + ltree |

#### 与 Jira / Freshdesk 对照

| 你的描述 | Jira | Freshdesk | 本项目规划 |
|----------|------|-----------|------------|
| 开发组 ↔ 多个项目 | Project 挂在 Space/Category；Project Admin | Group + Role | **虚拟组 org_type=4** ≈ 项目容器 |
| 项目负责人 | Project Lead / Admin | Supervisor | `owner_user_ids` + `org_member_role=owner` |
| 组内管理员 vs 普通用户 | Project Administrator vs Developer | Agent vs Supervisor 以下 | `user_orgs.org_member_role` admin/member |
| 管理员只能管普通用户 | Project Admin 不能删 Project Admin 权限需更高角色 | 主管不能改同级主管 | **组内 priority**：admin 不可改 admin/owner |
| 对组内资源 CRUD | Permission Scheme per project | Scope + Role | ResourceRegistry + org 绑定 + Authorize |

#### 当前计划覆盖度

| 能力 | 文档位置 | 阶段 | 状态 |
|------|----------|------|------|
| 实体组织 + HR 同步 | hr-directory-sync | 2b | ✅ 已设计 |
| 虚拟组（≈ 项目容器） | org-enhance、organization §2.2 | 2b | ✅ 已设计 |
| 跨部门加人、临时成员 | user_orgs.expires_at | 2b | ✅ 已设计 |
| 列表数据范围 all/group/assigned | ticket §2.4、user_orgs.ticket_scope | 2a/2b | ✅ 已设计（**可见性**，不是组内 admin） |
| **部门内策略 B（实体透明读）** | organizations.ticket_visibility、09-ticket §5.2 | 2b | ✅ 已定稿（默认） |
| 组织负责人 owner | organization §2.3 owner_ids | 2c | 📋 见 [04-org-delegation.md](./04-org-delegation.md) |
| org_roles（进组获 Casbin 角色） | 05-role、auth-design | 2b | ✅ 已设计 |
| **组内 member/admin/owner 分级** | — | 2c | 📋 见 [04-org-delegation.md](./04-org-delegation.md) |
| **admin 只能管 member** | — | 2c | 📋 见 [04-org-delegation.md](./04-org-delegation.md) |
| **资源绑定 org（项目工单）** | ticket.org_id / org_path | 2a | ✅ 工单 MVP；**泛化「项目」资源**未单独建模 |
| 负责人对子树资源 CRUD | system-comparison org_admin | 2c | 📋 见 [04-org-delegation §4](./04-org-delegation.md#4-authorize-升级step-9) |

#### 建议 Phase 2c 落地

> **实现 SSOT**：[04-org-delegation.md](./04-org-delegation.md)（API、Authorize 矩阵、错误码、Step 8–10）。本节保留设计背景与 D1–D12 摘要。

```sql
-- organizations：负责人（可多人）
ALTER TABLE organizations ADD COLUMN owner_user_ids BIGINT[] DEFAULT '{}';
-- 或 org_owners(org_id, user_id, role) 关联表

-- user_orgs：组内级别（与全局 user_roles 分离）
ALTER TABLE user_orgs ADD COLUMN org_member_role VARCHAR(20) NOT NULL DEFAULT 'member';
  -- member | admin | owner（owner 可与 organizations.owner 同步或 derived）
```

**Authorize 伪规则（工单/项目资源）**：

```
允许 update/delete 资源 R（R.org_id = O）当：
  全局 Casbin 有 ticket:update
  AND (
    用户是 R 的 created_by / assigned_to
    OR 用户在 O 的 org_member_role IN (admin, owner)
    OR 用户是 O 的 ancestor 链上某节点的 owner 且 R.org_path <@ 该节点.path  -- 部门负责人管子树
  )
组内任命 admin：仅 owner 或 org_member_role=owner
组内踢人/改 member：admin 只能动 org_member_role=member 的用户
```

> 实体 HR 部门的「负责人」通常来自 HR 字段或本地指定；虚拟项目组的负责人 **纯 IAM 指定**。HR Job **不覆盖** `owner_user_ids` / 虚拟组 `org_member_role`。

---

## 测试用例（节选）

| 用例 | 预期 |
|------|------|
| 在 tech 下创建虚拟组 | org_type=4, source=local, path 含 `root.tech.vg_*` |
| HR 移动 tech 部门 | tech 子树含虚拟组 path 一并更新 |
| HR 撤销 tech 且下有虚拟组 | 虚拟组 reparent，成员保留 |
| 用户 HR 主部门变更 | 仅 `is_primary+source=hr` 的 user_orgs 变 |
| 父部门 org_roles 不继承子部门 | 父绑 role X，子部门成员 | 子部门成员**无** role X（须子部门自身 org_roles 或 user_roles） |
| scope=group 工单列表 | 含本组织及子组织（含子树虚拟组 path 否：按工单 org_path 设计，见 ticket 模块） |
| **策略 B**：vg_a member 读 vg_b 工单 | **200**（实体 `entity_transparent_read`） |
| **策略 B**：vg_a member 改 vg_b 工单 | **403**（非创建人、非 vg_b admin） |
| 实体 `project_isolated`（**future，移出 2b-core 验收**） | vg_a member **不可读** vg_b（404） |

> **Step 5 落地状态（2026-08-28）**：上表前 6 行已由 `b2_org_integration_test.go` 覆盖（vg 创建约束/move 级联含 vg/BFS 三源与「父部门不继承子部门」/临时成员过期/策略 B 读写分离/scope 主管与全量）；HR 移动/撤销两行的自动化为 move 级联含 vg 用例，撤销 reparent 自动化属 2b-ext HR Sync。
#### 组织负责人与组内分级（Phase 2c — **非** 2b 验收范围）

> 与 §组织负责人与组内分级 设计对应；**验收 SSOT**：[04-org-delegation §7](./04-org-delegation.md#7-测试用例验收-ssot)。

| # | 用例 | 预期 |
|---|------|------|
| D1 | 设置虚拟组 owner | `organizations.owner_user_ids` 含负责人 |
| D2 | 组 owner 任命组内 admin | `user_orgs.org_member_role=admin` 成功 |
| D3 | 组 admin 任命 admin | **403**（组内防提权） |
| D4 | 组 admin 移除 member | 200；`user_orgs` 删除 |
| D5 | 组 admin 移除另一 admin | **403** |
| D6 | 组 owner 删虚拟组 | 200（无成员/资源约束时） |
| D7 | 组 admin 删绑定该组的工单 | 200（Casbin + Authorize） |
| D8 | 组 member 删同工单 | **403** |
| D9 | 实体部门 owner 对子树 org 下工单 | `org_path <@` + owner 链：**200** |
| D10 | HR 同步 | **不覆盖** `owner_user_ids`、`org_member_role`（local） |
| D11 | 策略 B：vg_a member vs vg_b 工单（**2b-core 验收，2c 回归**） | **可读**（L2）；**不可 update**（L3，403）；vg_b admin 可改本组 |
| D12 | `project_isolated` 实体（**future，移出 2b-core 验收**） | 兄弟虚拟组 **互不可读**（404） |

完整验收表见 [hr-directory-sync.md §7](../proposal/hr-directory-sync.md#7-验收用例phase-2b)。

---

## 涉及文件

```
internal/service/org/                 # 虚拟组 CRUD、Reparent、成员
internal/service/hr_sync/             # HR 对账（新建，或 integration/hr job）
internal/integration/hr/client.go     # HRDirectoryClient
internal/repository/org/
internal/repository/user/
cmd/sync-hr/main.go                      # 或 asynq periodic
migrations/0000xx_hr_source.up.sql
```

---

## 待决策点

| 事项 | 建议 | 状态 |
|------|------|------|
| Reparent 默认策略 | 自动上挂最近 HR 祖先（策略 A） | ✅ 建议 |
| 虚拟组 code 前缀 | `vg_` 区分 HR 部门 code | ✅ 建议 |
| HR Job 调度 | Cron 每日 + 分布式锁 + `hr_sync_runs`；手动触发 | ✅ 见 §3.4–3.5 |
