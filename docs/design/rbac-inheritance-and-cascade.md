# RBAC继承、数据范围与级联 — 本项目规则与演进边界

> **当前定位（2026-11-08）**：本篇解释本项目角色来源、L2数据范围与实体级联。Phase1“勿实现复杂继承”的限制属于当时边界：组织赋角、角色parent写侧/BFS和scope已随Phase2落地，HR真实同步/Reparent自动执行仍未交付。本文不再把本项目组织角色规则泛称为行业唯一做法。
> 先看§1/§2区分功能角色与数据范围，再看§3当前级联、§4权限展开及§7实现状态。具体API/错误码以[错误码](../api/errcode.md)及源码为准；主仓阶段/未完边界见[项目控制台](../review/11-project-control.md)。

---

## 1. 为什么容易混：三条独立维度

本项目把下面三件事**分开建模**，否则会出现「父部门经理自动管全子公司 API」或「有菜单权限却看不到数据」：

| 维度 | 回答的问题 | 典型载体 | 是否沿组织树向下继承 |
|------|------------|----------|----------------------|
| **功能权限**（能调哪些 API / 看到哪些菜单） | 能不能点这个按钮？ | RBAC 角色、`role_menus`、Casbin `p` | 本项目不通过组织父子自动增加功能角色 |
| **角色来源**（用户从哪获得角色） | 有效角色集合怎么算？ | `user_roles`、组织 `org_roles`、`roles.parent_id` | 组织赋角：**仅本节点**；角色链：**沿 parent 向上展开** |
| **数据范围**（能看哪些行/工单） | 列表里出现谁的数据？ | `data_scope` / ltree `path <@` / `assigned` | ✅ **可**沿组织树向下（「本部门及以下」） |

```
错误做法（混在一起）          本项目选择（非所有产品共同规则）
─────────────────────        ─────────────────────────
父部门绑「经理」角色    →      父部门 org_roles：仅父部门成员获得「经理」
子部门全员自动变经理          子部门成员：不继承；若也要经理 → 子部门单独 org_roles
上级能看子部门工单    →      数据 scope=group：ltree 过滤，与 org_roles 无关
「审计」继承「只读」  →      roles.parent_id：功能权限并集（BFS 源 3）
```

---

## 2. 产品对照与本项目场景

### 2.1 仅保留已核实的对照

Keycloak的Group可层级嵌套，**子组成员会继承父组与子组的attributes/role mappings**；Composite Role展开组合角色。此前“Keycloak只给直接组成员、子组不继承”的描述不准确。本项目选择只取直接所属org的org_roles，与Keycloak该行为不同；此差异本身不构成缺陷。依据：[Keycloak 26.5.7官方Server Administration Guide](https://www.keycloak.org/docs/26.5.7/server_admin/index.html)的Groups和Composite Roles节（2026-11-08核对）。

不同产品的组、OU、role、数据scope并非同一种实体。本篇不再用未经逐项核实的AD/AWS/SAP/Google删除或吊销行为，推导“所有系统都只直绑”“禁用必然即时吊销既存token”等普遍结论。

### 2.2 本项目场景规则

| 场景 | 当前规则与边界 |
|---|---|
| 父组织绑经理角色、子组织成员访问 | org_roles仅直接成员节点参与，不沿ltree继承；子成员若需该角色须显式绑定。数据可见scope另算 |
| audit角色继承viewer | roles.parent_id向上BFS能力并集；写侧校验child.priority≤parent.priority、父启用/非系统/非自身/无环；能力包默认不无故启用 |
| 用户同属实体部门与本地虚拟组 | 多user_orgs并集；虚拟标识当前is_virtual，source=local与HR源绑定分工仍须按HR方案实施 |
| Move组织子树 | 当前ltree路径级联更新，成员/角色绑定按org id保留；不是复制功能角色到子节点 |
| HR撤销节点下仍有本地虚拟组 | Reparent到最近HR祖先与保留local成员是HR方案，真实同步未交付，不当作当前自动级联 |
| 禁用/删除用户 | DB提交后走会话吊销：disabled＋pw_epoch递增＋清refresh；失败短重试后报错并留reconcile，不能一律称best-effort成功 |
| 删除仍有直接用户绑定的角色 | RBACService.DeleteRole返回ErrRoleInUse=40003，现行HTTP403；系统角色禁止删，不删除用户 |
| 改角色菜单/parent | role_menus和casbin_rule持久化同事务，提交后ReloadPolicy；parent防环/单调约束不依赖未来缓存 |

源码锚：`internal/repository/role_repo.go`的`RoleRepo.GetEffectiveRoleCodes/Delete`；`internal/repository/user_repo.go`的`UserRepo.GetRoles`；`internal/service/rbac_service.go`的`RBACService.DeleteRole`；`internal/service/session_revoke.go`的`revokeUserSessionsWithEpoch/revokeUserSessionsWithRetryEpoch`。

---

## 3. 本项目级联策略矩阵（SSOT）

> **原则**：数据库持久化按各仓储真实事务处理；role_menus与casbin_rule可同事务，不能一律说Casbin在事务外。内存ReloadPolicy、Redis会话吊销在提交后执行，失败可能上抛并要求对账，不是统一best-effort。菜单写API现已收为迁移/种子管理，下方菜单删除行仅保留Phase1历史设计。

### 3.1 用户（User）

| 操作 | 数据库（事务内） | 事务外副作用 | 拒绝条件 |
|------|------------------|--------------|----------|
| **创建** | INSERT users；可选同事务 `user_roles` / `SetUserOrgsTx` | — | 工号/域账号组合冲突 |
| **更新** | UPDATE 资料字段 | — | `is_system` 保护（admin 不可改 superadmin） |
| **禁用** | `status=0` | `user:disabled`＋INCR `user:pw_epoch`＋DEL全部RT（提交后短重试，最终失败上抛） | 最后一个 superadmin |
| **自服务改密** | UPDATE `password_hash` | 先disabled＋pw_epoch递增＋清RT，再写兼容at_min_iat地板；按本次纪元清disabled并发新AT/RT，旧AT同秒也因纪元不符拒绝 | 旧密码校验失败 |
| **删除（软删）** | DELETE `user_roles`、`user_orgs`；`deleted_at=NOW()` | 同上会话吊销 | `is_system`、最后 superadmin、不能删自己 |
| **SetRoles** | DELETE+INSERT `user_roles` | 当前无Redis角色缓存；perm:user失效仅为后续缓存方案 | priority 防提权 |
| **SetUserOrgs** | DELETE+INSERT `user_orgs` | — | primary 须在 org_ids 内 |
| **移出组织** | DELETE 单行 `user_orgs` | — | 非成员 → 404 |

### 3.2 角色（Role）

| 操作 | 数据库（事务内） | 事务外副作用 | 拒绝条件 |
|------|------------------|--------------|----------|
| **创建** | INSERT roles | — | code 冲突 |
| **改菜单** | REPLACE `role_menus` + INSERT/DELETE `casbin_rule` | `ReloadPolicy` | `is_system` 且非 superadmin 操作 |
| **改 parent_id**（2b） | UPDATE roles.parent_id | 当前现查角色，无已实施perm缓存失效链 | 成环、指向软删角色 |
| **删除（软删）** | DELETE `role_menus`；软删 roles | 提交后reloadPolicy；casbin_rule与role_menus/软删在repo同事务 | `is_system`、**仍有user_roles**（403/40003） |
| **删除** | — | — | **不**自动删用户；须先解绑 |


### 3.3 组织（Organization）

| 操作 | 数据库（事务内） | 事务外副作用 | 拒绝条件 |
|------|------------------|--------------|----------|
| **创建** | INSERT + 计算 ltree path | — | code 非法、父不存在 |
| **Move** | 子树 **path 级联** UPDATE | 当前按数据库路径读取；独立组织缓存不是此处已交付保证 | 不能移到自身子树下 |
| **AddMember** | INSERT `user_orgs` | 有效角色现查；未来启用缓存须另设计失效 | org/user 不存在 |
| **RemoveMember** | DELETE `user_orgs` | 同上 | 非成员 404 |
| **删除（软删）** | 仅软删 org 行 | — | `is_system`、**有子节点**、**有成员**、有未结工单（当前守卫status<>closed） |
| **删除 org_roles 绑定**（2b） | DELETE `org_roles` | 角色展开现查；不伪称已接perm缓存 | — |
| **HR撤销部门（规划，真实同步未交付）** | Reparent 虚拟组；软删 hr 节点 | 审计 | 见 hr-directory-sync |


### 3.4 菜单（Menu，Phase1历史写设计）

| 操作 | 数据库（事务内） | 事务外副作用 | 拒绝条件 |
|------|------------------|--------------|----------|
| **删除** | DELETE `menu_apis`、`role_menus`；软删 menu | 所有关联角色 **SyncPolicies** | `is_system`、有子菜单（409，先删子） |
| **删父菜单** | **不**自动递归删子（Phase 1） | — | 有 children 则拒绝 |


### 3.5 当前外键摘要（软删不触发ON DELETE）

| 关系 | 迁移中的行为 | 应用侧边界 |
|---|---|---|
| user_roles → users/roles | 000001未指定ON DELETE，PostgreSQL默认NO ACTION | 用户软删在仓储事务显式清user_roles；角色仍有用户绑定先拒绝 |
| role_menus → roles/menus | 000001默认NO ACTION | 角色删除在同事务清绑定；不等于DB自动CASCADE |
| user_orgs → users/orgs | 000001默认NO ACTION | 用户软删显式清关系，组织删除检查成员 |
| org_roles → orgs/roles | 000012默认NO ACTION | 绑定写侧已实现；软删实体不触发外键自动清理，不能据旧CASCADE建议假设关系已删除 |
| organizations.parent_id → organizations | 000001默认NO ACTION | 父节点删除有业务子节点守卫；不要把默认NO ACTION写成显式RESTRICT语法 |
| menu_apis → menus | 000001显式ON DELETE CASCADE | 只在物理删除触发；当前菜单由迁移维护 |

证据为`migrations/000001_init.up.sql`的映射表定义和`migrations/000012_org_enhance.up.sql`的org_roles定义；`internal/repository/user_repo.go`的`UserRepo.SoftDeleteTx`显式清用户关系，`internal/repository/role_repo.go`的`RoleRepo.Delete`同事务清role_menus/casbin_rule。旧“所有映射表CASCADE”只是建议，已由此实际摘要取代。

---

## 4. Phase 2b 有效权限展开（复习）

有效功能角色集合由`RoleRepo.GetEffectiveRoleCodes`取直接user_roles、**用户直接所属节点**的org_roles，再展开roles.parent_id祖先；不沿组织树自动继承org_roles。L1/菜单用展开集，user模块管理防提权的守卫按`UserRepo.GetRoles`的直接角色集，保持两轨。

**L2不是纯scope switch**：当前工单可见集包含创建人/处理人OR支，再并入all、实体透明读/group路径、组织委托与抄送。assigned不会排除创建人；all仍需L1，L2失败404、可见但动作拒绝403。实际以`internal/service/ticket/resource.go`的`Resource.Authorize/GetFilter/canOperate`为准，详[工单§2](../modules/ticket.md)。

**复杂用例速查**：

| 用例 | 功能角色 | 数据范围 |
|------|----------|----------|
| 父部门 HR 经理，子部门员工 | 子员工**无**父部门 org_roles | 若 scope=group 且经理主部门在父节点，**可**看下级工单 |
| 子部门单独绑 `operator` | 仅子部门成员有 operator | 与父部门 org_roles 无关 |
| 用户 direct `viewer` + 部门 org_roles `operator` | 并集 OR，都能过的 API 放行 | 列表仍看 scope |
| 角色 `audit` parent=`viewer` | 展开含 viewer + audit 的 Casbin 策略 | 不改变 scope |

---

## 5. 设计验收清单（历史阶段列，非本轮测试执行结果）

| # | 场景 | 预期（摘要） | 阶段 |
|---|------|--------------|------|
| 1 | 父 org 绑 role，子 org 成员访问 | 无父 org 角色，403（无策略） | 2b |
| 2 | 子 org 绑同一 role | 200 | 2b |
| 3 | composite role parent 链 | 子角色用户可访问父角色菜单 API | 2b |
| 4 | parent_id 成环 | 400 拒绝 | 2b |
| 5 | 删角色仍有 user_roles | 403 ErrRoleInUse | 1 |
| 6 | 删组织有成员 | 409 ErrOrgHasMembers | 1 |
| 7 | Move 含虚拟组子树 | 虚拟组 path 同步更新 | 2b |
| 8 | HR撤销部门+本地虚拟组 | Reparent/保留成员为规划验收，真实同步未交付 | HR触发批 |
| 9 | 删用户 | user_roles/user_orgs 清、会话吊销 | 1 |
| 10 | 删菜单有子节点 | 历史服务设计409；当前无菜单写API，迁移维护另核 | 1历史 |
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

## 7. 当前实现与未交付边界

| 能力 | 当前状态 |
|---|---|
| user_roles、priority防提权、三源BFS | 已交付；能力展开与管理直接角色守卫分轨 |
| org_roles绑定/解绑/列表 | 已交付（BK-12），仅全局管理员写；直接成员节点生效 |
| roles.parent_id写侧 | 已交付父启用/非系统/非自身/防环/深度32及child≤parent；使用继承按真实能力打包诉求启用 |
| scope/透明读/委托与属主OR | 已随Phase2落地；当前谓词详工单模块，不复用旧纯组织隔离描述 |
| is_virtual与组织负责人 | 已落库，org_type=4是000019前旧表示；负责人/委托规则按Phase2c |
| HR真实同步/Reparent自动执行 | 仍规划/触发，不能凭本文场景推断运行中 |
| Redis perm缓存/Watcher | 非当前已交付能力；启用需独立设计跨实例失效与验证 |

2026-08-31曾确认的单调继承和直接/展开双轨决定保持有效。本轮只校准文档，不改权限实现、不声称重新运行以上验收。
