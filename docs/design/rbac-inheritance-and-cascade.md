# RBAC 继承、数据范围与级联：业界参考与本项目决策

> **📋 Phase 1 不实现本文中的复杂继承与扩展级联**——当前仅作**设计备忘**，供 Phase 2b+ 与 HR/工单联调前对齐；编码 Phase 1 时**不要**提前建 `org_roles`、`roles.parent_id`、BFS 三源、数据 scope 等。  
> 复杂权限场景的行业做法对照，以及 **用户 / 角色 / 组织** 增删改时的级联策略 SSOT。  
> 与 [phase1/05-role §权限继承模型](../phase1/05-role.md#角色-priority-与权限继承模型)、[auth-design §3.3](../proposal/auth-design.md#33-组织角色继承phase-2b)、[system-comparison §#8](./system-comparison.md#8-级联删除与一致性) 互补；**Phase 1 实现边界以 [phase1/README §1.4](../phase1/README.md#14-已知限制验收时不要误判为已实现) 为准**。

---

## 1. 为什么容易混：三条独立维度

业界成熟 IAM / 后台系统几乎都把下面三件事**分开建模**，否则会出现「父部门经理自动管全子公司 API」或「有菜单权限却看不到数据」：

| 维度 | 回答的问题 | 典型载体 | 是否沿组织树向下继承 |
|------|------------|----------|----------------------|
| **功能权限**（能调哪些 API / 看到哪些菜单） | 能不能点这个按钮？ | RBAC 角色、`role_menus`、Casbin `p` | ❌ 默认**不**随组织父子继承 |
| **角色来源**（用户从哪获得角色） | 有效角色集合怎么算？ | `user_roles`、组织 `org_roles`、`roles.parent_id` | 组织赋角：**仅本节点**；角色链：**沿 parent 向上展开** |
| **数据范围**（能看哪些行/工单） | 列表里出现谁的数据？ | `data_scope` / ltree `path <@` / `assigned` | ✅ **可**沿组织树向下（「本部门及以下」） |

```
错误做法（混在一起）          业界常见做法（本项目对齐）
─────────────────────        ─────────────────────────
父部门绑「经理」角色    →      父部门 org_roles：仅父部门成员获得「经理」
子部门全员自动变经理          子部门成员：不继承；若也要经理 → 子部门单独 org_roles
上级能看子部门工单    →      数据 scope=group：ltree 过滤，与 org_roles 无关
「审计」继承「只读」  →      roles.parent_id：功能权限并集（BFS 源 3）
```

---

## 2. 业界产品与典型场景对照

### 2.1 总览表

| 产品 / 框架 | 功能权限 | 组织/组赋角 | 组织树与数据范围 | 角色继承 | 删实体时常见策略 |
|-------------|----------|-------------|------------------|----------|------------------|
| **Microsoft AD / Entra ID** | 组 → ACL / App Role | 安全组 / M365 组 membership | OU 委派管理范围可限定子树；**组成员不因子 OU 自动扩权** | 嵌套组（组入组）≈ composite role | 删组前需无成员或先移出；禁用户 = 吊销会话 |
| **Keycloak** | Realm / Client Role | **Group ↔ Role mapping**（仅该组成员） | 组可嵌套，但 role mapping 默认作用于**直接成员**（子组需单独映射或显式配置） | **Composite Role**（子角色并集） | 删 Role 前解绑用户/组；删 Realm 级联极重 → 生产禁止 |
| **AWS IAM** | Policy attach | 无「组织成员自动角色」；SCP 是账户级**上限** | OU + SCP：**向下约束**能做什么，不是给用户加角色 | 无角色 parent；权限显式 attach | 删 Role 前 detach 所有 principal |
| **若依 RuoYi** | 菜单 + 按钮权限 | 角色绑定部门（数据权限维度） | **数据范围**：全部 / 自定义 / 本部门 / **本部门及以下** / 仅本人 | 一般无角色 parent；靠 data_scope | 删角色前检查 `sys_user_role`；删部门检查用户 |
| **SAP GRC / Oracle IAM** | 业务角色 catalog | 组织岗位 → 角色（岗位继承有限） | 组织层级 + 职责分离 | 角色组合 / 继承树 | 软失效 + 审计保留为主 |
| **Google Workspace** | Admin 角色 | Org Unit 分配 | 部分策略**继承到子 OU**，部分**不继承**（产品分类型） | Admin 角色无树形 inherit | 删 OU 前移用户 |

### 2.2 场景 1：父部门经理能否管子部门的人？

| 产品 | 行为 |
|------|------|
| **AD** | 「经理」若是**组** membership：只有组成员享有；子 OU 用户**不会**因父 OU 委派自动进组。 |
| **Keycloak** | 父 **Group** 映的 Role 只给**该 Group 成员**；子 Group 成员不自动获得父 Group 的 role mapping（除非把 role 也映到子 Group）。 |
| **若依** | 「本部门及以下」只扩大**数据列表**，不自动增加**菜单/API 权限**。 |
| **本项目 Phase 2b** | `org_roles` 绑在**父部门节点** → 仅 `user_orgs` 含该 org_id 的用户获得角色；**子部门成员不继承**。子部门要经理 → 在子部门再绑 `org_roles` 或 `user_roles` 直接赋角。 |

**结论**：行业主流是 **「组织赋角不向下继承」**；向下的是 **数据可见范围**，不是功能角色。

### 2.3 场景 2：「审计员」要包含「只读用户」全部权限

| 产品 | 行为 |
|------|------|
| **Keycloak** | `audit` = **Composite Role**，包含 `viewer`；有效权限 = 展开并集。 |
| **AD** | 组 A 嵌套组 B，成员得 A∪B 权限。 |
| **AWS IAM** | 无内置 inherit；多条 Policy attach 到同一 User → 并集。 |
| **本项目 Phase 2b** | `roles.parent_id`：`audit.parent_id → viewer`；BFS **源 3** 展开父链；Casbin 对展开后的每个 `role::{code}` OR enforce。 |

### 2.4 场景 3：用户同时在「技术部」和「项目虚拟组」

| 产品 | 行为 |
|------|------|
| **AD** | 用户可同时属于多个组；有效权限 = 各组 ACL 并集。 |
| **Keycloak** | 多 Group membership → 多组 role mapping 并集。 |
| **本项目** | 多 `user_orgs`；有效角色 = 各 org 的 `org_roles` ∪ `user_roles`（Phase 2b BFS）；**虚拟组**（`org_type=4`）成员与 HR 主部门**独立**，HR 同步不覆盖 `source=local` 绑定（见 [hr-directory-sync](../proposal/hr-directory-sync.md)）。 |

### 2.5 场景 4：HR 把「技术中心」整棵子树迁到「产品中心」下

| 产品 | 行为 |
|------|------|
| **AD** | Move OU → 对象 GUID 不变，委派/组成员关系随对象走；**嵌套组逻辑不变**。 |
| **本项目** | `organizations.path` **子树级联**更新（含其下虚拟组 path）；`org_roles` 仍挂在原 org **id** 上，不随 move 丢失；成员 `user_orgs` 不变。 |

### 2.6 场景 5：撤销 HR 部门，但下面还有本地「项目虚拟组」

| 产品 | 行为 |
|------|------|
| **AD** | 删 OU 前通常需为空或先移对象；否则拒绝。 |
| **本项目 Phase 2b** | HR 撤销：**Reparent** 虚拟组到最近 HR 祖先（策略 A），**保留**虚拟组成员；不硬删 `source=local` 数据（见 [hr-directory-sync §4.3](../proposal/hr-directory-sync.md)）。 |

### 2.7 场景 6：禁用 / 删除用户

| 产品 | 行为 |
|------|------|
| **AD / Entra** | Disable account → 现有 token/会话失效；组成员关系可保留或清除（视流程）。 |
| **Keycloak** | Disable user → 会话终止；Group/Role mapping 可保留便于恢复。 |
| **本项目 Phase 1** | 禁用/删除：**事务内**清 `user_roles` / `user_orgs`（删除时）；**事务外** `user:disabled` + 删全部 RT；软删用户保留审计。 |

### 2.8 场景 7：删除仍有人用的角色

| 产品 | 行为 |
|------|------|
| **Keycloak / 若依** | **拒绝删除**，提示先解绑用户/组。 |
| **AWS** | 必须先 Detach 所有关联。 |
| **本项目** | `ErrRoleInUse`（409）；`is_system` 角色禁止删；删前清 `role_menus`，事务外清 Casbin `p` 策略。 |

### 2.9 场景 8：改角色菜单 / 改 role 的 parent（防环）

| 产品 | 行为 |
|------|------|
| **Keycloak** | Composite 不能形成环；保存时校验。 |
| **本项目** | 改 `role_menus`：**同事务**写 `role_menus` + `casbin_rule`，提交后 `ReloadPolicy`；改 `parent_id`：写入前 **DFS 环检测**；有子角色时禁止把 parent 设为自己后代。 |

---

## 3. 本项目级联策略矩阵（SSOT）

> **原则**：映射表可用 FK `ON DELETE CASCADE`；**业务实体**（用户/组织/角色/菜单）用手动事务 + 业务规则；**Casbin / Redis / 审计**在事务外 best-effort。

### 3.1 用户（User）

| 操作 | 数据库（事务内） | 事务外副作用 | 拒绝条件 |
|------|------------------|--------------|----------|
| **创建** | INSERT users；可选同事务 `user_roles` / `SetUserOrgsTx` | — | 工号/域账号组合冲突 |
| **更新** | UPDATE 资料字段 | — | `is_system` 保护（admin 不可改 superadmin） |
| **禁用** | `status=0` | `user:disabled` + DEL 全部 RT | 最后一个 superadmin |
| **删除（软删）** | DELETE `user_roles`、`user_orgs`；`deleted_at=NOW()` | 同上会话吊销 | `is_system`、最后 superadmin、不能删自己 |
| **SetRoles** | DELETE+INSERT `user_roles` | Phase 3：`DEL perm:user:{id}` | priority 防提权 |
| **SetUserOrgs** | DELETE+INSERT `user_orgs` | — | primary 须在 org_ids 内 |
| **移出组织** | DELETE 单行 `user_orgs` | — | 非成员 → 404 |

### 3.2 角色（Role）

| 操作 | 数据库（事务内） | 事务外副作用 | 拒绝条件 |
|------|------------------|--------------|----------|
| **创建** | INSERT roles | — | code 冲突 |
| **改菜单** | REPLACE `role_menus` + INSERT/DELETE `casbin_rule` | `ReloadPolicy` | `is_system` 且非 superadmin 操作 |
| **改 parent_id**（2b） | UPDATE roles.parent_id | 失效 perm 缓存 | 成环、指向软删角色 |
| **删除（软删）** | DELETE `role_menus`；软删 roles | RemoveFilteredPolicy `role::{code}` | `is_system`、**仍有 user_roles**（409） |
| **删除** | — | — | **不**自动删用户；须先解绑 |

> **行业对齐**：Keycloak / 若依均为「有绑定则禁止删角色」，不级联删用户。

### 3.3 组织（Organization）

| 操作 | 数据库（事务内） | 事务外副作用 | 拒绝条件 |
|------|------------------|--------------|----------|
| **创建** | INSERT + 计算 ltree path | — | code 非法、父不存在 |
| **Move** | 子树 **path 级联** UPDATE | 组织缓存失效 | 不能移到自身子树下 |
| **AddMember** | INSERT `user_orgs` | Phase 2b：用户有效角色变更 → 可选失效 perm 缓存 | org/user 不存在 |
| **RemoveMember** | DELETE `user_orgs` | 同上 | 非成员 404 |
| **删除（软删）Phase 1** | 仅软删 org 行 | — | `is_system`、**有子节点**、**有成员** |
| **删除 org_roles 绑定**（2b） | DELETE `org_roles` | ReloadPolicy / perm 缓存 | — |
| **HR 撤销部门**（2b） | Reparent 虚拟组；软删 hr 节点 | 审计 | 见 hr-directory-sync |

> **行业对齐**：AD/若依删部门前要求**无成员/无子节点**（或先迁移）；本项目 Phase 1 **拒绝式**级联，不自动把成员「抛」到父部门。

### 3.4 菜单（Menu）

| 操作 | 数据库（事务内） | 事务外副作用 | 拒绝条件 |
|------|------------------|--------------|----------|
| **删除** | DELETE `menu_apis`、`role_menus`；软删 menu | 所有关联角色 **SyncPolicies** | `is_system`、有子菜单（409，先删子） |
| **删父菜单** | **不**自动递归删子（Phase 1） | — | 有 children 则拒绝 |

> **行业对齐**：多数后台要求**自底向上**删菜单树，避免误删大面积权限。

### 3.5 映射表 FK 建议（Phase 1 迁移）

| 表 | FK 行为 | 说明 |
|----|---------|------|
| `user_roles` | `ON DELETE CASCADE` from users / roles | 硬删时自动清；软删走 Service 显式 DELETE |
| `role_menus` | `ON DELETE CASCADE` from roles | 删角色时清绑定 |
| `user_orgs` | `ON DELETE CASCADE` from users | 删用户时清组织关系 |
| `org_roles`（2b） | `ON DELETE CASCADE` from organizations / roles | 删 org/role 时清组织赋角 |
| `organizations.parent_id` | `ON DELETE RESTRICT` | 防止误删父节点留下孤儿；删父前先移/删子 |

---

## 4. Phase 2b 有效权限展开（复习）

```
用户 U 的有效功能角色（Casbin subject 展开）：

  R = user_roles(U)
    ∪ ⋃{ org_roles(O) | O ∈ user_orgs(U) }      -- 仅用户所在组织节点，不含父/子部门
    ∪ ⋃{ ancestor roles via roles.parent_id }   -- 沿角色链向上（源 3）

数据可见（列表 WHERE，与 R 独立）：

  ticket_scope = all     → 无 org 过滤（仍受路由级 Casbin 约束）
  ticket_scope = group   → org_path <@ 用户主部门 path（含子树）
  ticket_scope = assigned → assignee_id = U
```

**复杂用例速查**：

| 用例 | 功能角色 | 数据范围 |
|------|----------|----------|
| 父部门 HR 经理，子部门员工 | 子员工**无**父部门 org_roles | 若 scope=group 且经理主部门在父节点，**可**看下级工单 |
| 子部门单独绑 `operator` | 仅子部门成员有 operator | 与父部门 org_roles 无关 |
| 用户 direct `viewer` + 部门 org_roles `operator` | 并集 OR，都能过的 API 放行 | 列表仍看 scope |
| 角色 `audit` parent=`viewer` | 展开含 viewer + audit 的 Casbin 策略 | 不改变 scope |

---

## 5. 测试 / 验收应覆盖的「级联 + 继承」用例

| # | 场景 | 预期（摘要） | 阶段 |
|---|------|--------------|------|
| 1 | 父 org 绑 role，子 org 成员访问 | 无父 org 角色，403（无策略） | 2b |
| 2 | 子 org 绑同一 role | 200 | 2b |
| 3 | composite role parent 链 | 子角色用户可访问父角色菜单 API | 2b |
| 4 | parent_id 成环 | 400 拒绝 | 2b |
| 5 | 删角色仍有 user_roles | 409 ErrRoleInUse | 1 |
| 6 | 删组织有成员 | 409 ErrOrgHasMembers | 1 |
| 7 | Move 含虚拟组子树 | 虚拟组 path 同步更新 | 2b |
| 8 | HR 撤销部门 + 下挂虚拟组 | Reparent，成员保留 | 2b |
| 9 | 删用户 | user_roles/user_orgs 清、会话吊销 | 1 |
| 10 | 删菜单有子节点 | 409，不级联删子 | 1 |
| 11 | 改 role_menus | casbin_rule 同事务更新 + ReloadPolicy | 1 |
| 12 | 禁用用户 | AT 403、RT 401 | 1 |

完整 Phase 1 路径见 [phase1/README §1.3](../phase1/README.md)；Phase 2b 见 [hr-directory-sync §7](../proposal/hr-directory-sync.md) 与 [phase2/03-org-enhance](../phase2/03-org-enhance.md)。

---

## 6. 相关文档

| 文档 | 内容 |
|------|------|
| [phase1/05-role.md](../phase1/05-role.md) | priority、BFS 三源、删角色保护 |
| [phase1/06-organization.md](../phase1/06-organization.md) | 组织 Move、删组织、成员 API |
| [phase1/04-user.md](../phase1/04-user.md) | 删用户级联、SetRoles/SetUserOrgs |
| [proposal/auth-design.md](../proposal/auth-design.md) | AuthN/Z 总览 |
| [proposal/hr-directory-sync.md](../proposal/hr-directory-sync.md) | HR Move/Reparent/虚拟组 |
| [design/system-comparison.md](./system-comparison.md) | 旧系统对比、级联 #8 |

---

## 7. 实现分期备忘（Phase 1 勿做）

| 能力 | 阶段 | Phase 1 状态 |
|------|------|----------------|
| `user_roles` 直接角色 + Casbin OR | Phase 1 | ✅ 要做 |
| `roles.priority` 防提权 | Phase 1 | ✅ 要做 |
| 用户/组织/角色 **基础 CRUD 级联**（§3.1–3.4 中标注 Phase 1 的行） | Phase 1 | ✅ 要做 |
| `org_roles`、组织赋角 | Phase 2b | 📋 本文记录，**暂不实现** |
| `roles.parent_id`、BFS 源 3 | Phase 2b | 📋 暂不实现 |
| 数据 scope（group/all/assigned）+ 工单过滤 | Phase 2a/2b | 📋 暂不实现 |
| HR Move / Reparent / 虚拟组 | Phase 2b | 📋 暂不实现 |
| §5 中用例 #1–4、#7–8 | Phase 2b | 📋 验收时再补 |
| 组织负责人 + 组内分级 D1–D11 | **Phase 2c** | 📋 [04-org-delegation §7](../phase2/04-org-delegation.md#7-测试用例验收-ssot) |

> 开 Phase 2 前：重读本文 §2–§4，并对照 [roadmap.md](../roadmap.md) / [phase2/README.md](../phase2/README.md) 更新迁移与测试。
