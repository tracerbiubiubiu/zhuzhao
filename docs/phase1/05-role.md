# 05 - 角色模块（role）

> **Step 7**，依赖 Step 5（authz）。与 Step 8/9 **可并行**（AssignMenus 使用 Step 1 种子 `menu_apis`）。

---

## 预期功能

> 权限码 SSOT：[07-menu §权限码命名规范](./07-menu.md#权限码命名规范与-api-对齐ssot)。

| 功能 | 场景 | API | 权限码 |
|------|------|-----|--------|
| 角色列表 | 管理员查看所有角色 | `GET /api/v1/roles` | `role:list` |
| 创建角色 | 管理员创建新角色 | `POST /api/v1/roles` | `role:create` |
| 角色详情 | 查看角色信息 + 关联菜单 | `GET /api/v1/roles/:id` | `role:read` |
| 更新角色 | 修改角色名称、描述 | `POST /api/v1/roles/update` | `role:update` |
| 删除角色 | 删除角色（需检查是否有关联用户） | `POST /api/v1/roles/delete` | `role:delete` |
| 分配菜单 | 给角色分配可访问的菜单 | `POST /api/v1/roles/menus` | `role:assign_menu` |
| 查看角色菜单 | 获取角色关联的菜单列表 | `GET /api/v1/roles/:id/menus` | `role:assign_menu` |
| 查看角色权限 | 获取角色的 Casbin 策略 | `GET /api/v1/roles/:id/permissions` | `role:read` |

---

## 核心设计思路

### Role 结构体

```go
type Role struct {
    ID          int64      `json:"id,string" db:"id"`
    Code        string     `json:"code" db:"code"`           // 业务编码，Casbin subject 用
    Name        string     `json:"name" db:"name"`
    Description string     `json:"description" db:"description"`
    Status      int        `json:"status" db:"status"`       // 1=启用 0=禁用
    Priority    int        `json:"priority" db:"priority"`   // 越小权限越高；业务防提权用
    SortOrder   int        `json:"sort_order" db:"sort_order"`
    IsSystem    bool       `json:"is_system" db:"is_system"`  // 系统内置不可删除
    TenantID    int64      `json:"tenant_id,string" db:"tenant_id"`
    Version     int        `json:"version" db:"version"`      // 乐观锁
    DeletedAt   *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
    CreatedAt   time.Time  `json:"created_at" db:"created_at"`
    UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}
```

### roles 建表 SQL

```sql
CREATE TABLE roles (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(50) NOT NULL,            -- 如 "admin", "user_manager"
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    status      SMALLINT NOT NULL DEFAULT 1,
    priority    INT NOT NULL,                 -- 越小权限越高；间隔留空便于插自定义角色
    sort_order  INT DEFAULT 0,
    is_system   BOOLEAN DEFAULT FALSE,
    tenant_id   BIGINT NOT NULL DEFAULT 1,
    version     INT DEFAULT 1,
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_roles_code ON roles(code) WHERE deleted_at IS NULL;
CREATE INDEX idx_roles_tenant ON roles(tenant_id) WHERE deleted_at IS NULL;
```

> **Go model 对齐**：`internal/model/role.go` 须含 `Priority`、`DeletedAt`，与上表一致；缺字段则 Step 1 与 migration 一并补齐。

### 关联表

```sql
-- 用户-角色
CREATE TABLE user_roles (
    user_id BIGINT NOT NULL REFERENCES users(id),
    role_id BIGINT NOT NULL REFERENCES roles(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY(user_id, role_id)
);

-- 角色-菜单
CREATE TABLE role_menus (
    role_id BIGINT NOT NULL REFERENCES roles(id),
    menu_id BIGINT NOT NULL REFERENCES menus(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY(role_id, menu_id)
);
```

### 角色 → 菜单 → API → Casbin 策略链路

> 详见 [modules/role.md](../modules/role.md) §4。角色用 `code` 作为唯一标识，Casbin 策略中 subject 为 `role::{roleCode}`。

```
管理员给角色分配菜单
  │
  ├── 更新 role_menus 表（DB 事务内）
  ├── 根据菜单绑定的 API 路径，生成 Casbin 策略
  │   例如：菜单"用户管理"绑定了 GET /api/v1/users 和 POST /api/v1/users
  │   → 生成策略 p, role::user_manager, /api/v1/users, GET   （user_manager 为示例自定义角色）
  │   → 生成策略 p, role::user_manager, /api/v1/users, POST
  ├── 写入 casbin_rule 表（同一事务）
  └── 事务提交后 enforcer.ReloadPolicy()
```

### 菜单-API 绑定

> 详见 [modules/menu.md](../modules/menu.md)。菜单-API 绑定用独立表 `menu_apis`（见 [07-menu.md](./07-menu.md)），不用 JSONB 字段。

```sql
-- menu_apis 表（见 07-menu.md 完整定义）
CREATE TABLE menu_apis (
    menu_id     BIGINT NOT NULL REFERENCES menus(id) ON DELETE CASCADE,
    api_path    VARCHAR(200) NOT NULL,
    api_method  VARCHAR(10) NOT NULL,
    PRIMARY KEY (menu_id, api_path, api_method)
);
```

角色分配菜单时，通过 `menu_apis` 表 JOIN 查询菜单关联的 API 路径，自动生成 Casbin 策略。

### 策略三分类（借鉴旧系统）

> 详见 [modules/role.md](../modules/role.md) §4。Phase 1 实现路由级策略，Phase 2 补充按钮级和 API 级。

| 策略类型 | 来源 | Phase | 说明 |
|---------|------|-------|------|
| 路由级（route） | 菜单 api_paths 自动生成 | Phase 1 | Casbin enforce 用的核心策略 |
| 按钮级（button） | 菜单 permission 字段 | Phase 2 | 前端按钮显隐控制，不走 Casbin |
| API 级（api） | 手动配置 | Phase 2 | 细粒度 API 控制（如只读 vs 读写） |

### 删除角色保护

删除角色前检查是否有用户关联：

```go
func (s *roleService) Delete(ctx context.Context, roleCode string) error {
    count, _ := s.repo.CountUsersByRole(ctx, roleCode)
    if count > 0 {
        return ErrRoleInUse  // "该角色仍有用户关联，无法删除"
    }
    // 系统角色保护
    role, _ := s.repo.GetByCode(ctx, roleCode)
    if role.IsSystem {
        return ErrRoleIsSystem  // "系统内置角色不可删除"
    }
    return s.repo.Delete(ctx, roleCode)
}
```

### 内置角色

> **角色 vs 用户**：`superadmin` / `admin` / `operator` / `viewer` 是 **角色**（`roles` 表记录），不是用户类型。用户通过 `user_roles` **多对多**绑定角色，可同时拥有多个；详见 [03-authz §用户多角色](./03-authz.md#用户多角色phase-1)。

种子数据创建 4 个系统角色（**`priority` 越小权限越高**，与若依/RuoYi 等国内后台一致）：

| code | name | priority | Casbin 行为 |
|------|------|----------|------------|
| `superadmin` | 超级管理员 | **1** | matcher bypass |
| `admin` | 管理员 | **10** | matcher bypass |
| `operator` | 操作员 | **20** | 需 Casbin 策略 |
| `viewer` | 访客 | **30** | 需 Casbin 策略 |

自定义角色建议用 15、25… 插在系统角色之间。业务比较见 [§角色 priority 与权限继承模型](#角色-priority-与权限继承模型)。

Casbin 模型 matcher 中 `r.sub == "role::superadmin" || r.sub == "role::admin"` 直接放行，不需要为超管/管理员分配任何策略。

#### superadmin 与 admin 的区别

| 维度 | `superadmin` | `admin` |
|------|--------------|---------|
| Casbin 路由鉴权 | matcher bypass | matcher bypass（**路由级等价**） |
| **对其他管理员是否可见** | **对 admin 及以下「不可见」**（见下 §影子超管） | 对外呈现的**最高**管理员档位 |
| 重置密码 | 可重置任意用户（含 admin） | 只能重置 operator/viewer 等普通用户 |
| 分配角色 | 可分配任意角色（含 superadmin） | **不能**给他人分配 superadmin |
| 改系统资源 | 可管理 `is_system` 资源 | **不能**删改系统角色/菜单/组织，**不能**改 superadmin 用户 |
| 改角色菜单 | 可改任意角色菜单 | **不能**改 superadmin 角色的菜单 |
| 系统兜底 | 至少保留 **1 名** 用户绑定 superadmin | 无特殊地位 |

#### 影子超管（superadmin 对 admin 不可见）

> 业界常见说法：**break-glass / root 账号**不对日常管理员暴露；业务上 admin 已是「能感知到的最高管理员」，`superadmin` 仅运维/安全留底。

**原则**：非 superadmin 操作者（`EffectivePriority > superadmin.priority`，即非 superadmin 身份）在 **列表/下拉/详情** 中 **看不到** superadmin 这一档及其绑定用户；**不是**删除数据，而是 **读路径过滤 + 写路径 403/404**。

| 场景 | admin / operator / viewer 感知 | superadmin |
|------|--------------------------------|------------|
| `GET /roles` 角色列表 | **不含** `code=superadmin` | 含全部角色 |
| 分配角色下拉 | **无** superadmin 选项 | 有 |
| `GET /users` 用户列表 | **不含** 绑定 superadmin 的用户 | 含全部用户 |
| `GET /users/:id` 超管用户 | **404**（与非存在一致，防推断） | 200 |
| 改/删/禁用 superadmin 用户 | **404 或 403**（实现统一一种） | 允许（受最后一名 superadmin 保护） |
| 审计日志 | Phase 1 可全体可见；Phase 2 可按 actor 过滤敏感条目 | 全部 |

**与现有规则的关系**：

- Casbin 仍保留 `role::superadmin` matcher bypass（**后端真实权限**不变）。
- 「不可见」是 **展示层 / 列表 SQL / Handler 过滤**，避免 admin 知道还有更高账号而去社工或撞库。
- 种子用户 `admin` 绑定的是 **superadmin 角色**——该用户在 admin 视角的用户列表里 **也不出现**；仅 superadmin 会话或数据库运维可见。

**Phase 1 实现要点**（列表查询加条件，不必新表）：

```sql
-- 角色列表（actor 非 superadmin 时）
WHERE deleted_at IS NULL AND code <> 'superadmin'

-- 用户列表（actor 非 superadmin 时）
WHERE deleted_at IS NULL
  AND id NOT IN (SELECT user_id FROM user_roles ur JOIN roles r ON r.id = ur.role_id
                 WHERE r.code = 'superadmin' AND r.deleted_at IS NULL)
```

> 可选 Phase 2：角色表增加 `is_hidden BOOLEAN`，超管档通用化；Phase 1 **硬编码过滤 `superadmin`** 即可。

> 小结：**API 能不能进**看 Casbin（两者一样）；**能不能动更高或同级权限对象**看 `roles.priority`（越小越强，须 **严格更强** `actorP < targetP`）。多角色用户取 **EffectivePriority = min(priority)**，见 [04-user §多角色与有效 priority](./04-user.md#多角色与有效-priority)。

#### 角色 priority 与权限继承模型

> 业界通用：**路由权限（Casbin）**、**组织数据范围（ltree）**、**角色继承（parent / org_roles）** 分开建模，避免「父部门角色自动污染全子树」。  
> **业界对照与级联矩阵**见 [rbac-inheritance-and-cascade.md](../design/rbac-inheritance-and-cascade.md)。

**Phase 1**

| 能力 | 行为 |
|------|------|
| 路由鉴权 | `user_roles` 直接角色 → Casbin OR |
| 业务防提权 | `roles.priority`（越小越强） |
| 组织 | 树 + 成员；**无** `org_roles`、无按组织过滤 |
| 角色继承 | **无**（无 `roles.parent_id`） |

**Phase 2b**（见 [03-authz §BFS 三源角色](./03-authz.md#bfs-三源角色phase-2b)、[auth-design §3.3](../proposal/auth-design.md#33-组织角色继承phase-2b)）

| 能力 | 行为 |
|------|------|
| 有效角色（Casbin） | **BFS 三源并集**：① `user_roles` ② `user_orgs`→`org_roles` ③ `roles.parent_id` 链 |
| 组织赋角色 | 组织节点绑角色；**用户加入该组织**获得该组织角色 |
| 子组织不继承父 org_roles | 成员在子部门不自动获得父部门绑定的角色 |
| 组织树 **数据**范围 | `ticket_scope` + ltree `path <@`：上级可看 **下级数据**，≠ 角色继承 |
| 角色链继承 | `roles.parent_id`：子角色继承父角色 **菜单/API 策略**（源 3） |

```
Phase 2b 有效角色 = 直接 ∪ 所在组织的 org_roles ∪ parent 角色链
Phase 2b 数据可见  = ltree scope（group/all/assigned），与 org_roles 独立
```

#### 种子用户

初始账号 **`admin` / `admin123`**（`users.is_system=true`，**登录工号 `E000001`**）绑定 **`superadmin` 角色**；同时绑定 `root` 组织。DDL 见 [data-init.md §4.2](../proposal/data-init.md#42-种子数据内容)。

**业务层仍要分级**（Casbin bypass 不等于业务无约束）：

- 不能删除系统角色（`is_system`）
- `admin` 不能修改 `superadmin` 角色的菜单
- 不能删除仍有用户关联的角色
- 不能删除导致系统失去最后一个 superadmin 绑定的操作（与 user 模块一起校验）

#### 角色禁用语义（status=0）

> B1-1 修复（源自 [review/01 §R1-AUTHZ-01](../review/01-phase1-systematic-review-findings.md)）：禁用角色此前全链路不生效。

`UpdateRoleRequest.status` 允许 0（禁用）。禁用后**下次请求起**生效（与「角色变更下次请求生效」一致）：

| 生效点 | 行为 |
|--------|------|
| Casbin L1 鉴权 | `GetRoleCodes` 不返回禁用角色 → 逐角色 enforce 不含它 → 其策略全部失效 |
| 用户菜单/权限码 | `ListRoleIDsByUserID` 不含禁用角色 → 菜单与按钮权限码不下发 |
| priority 档位 | `GetRoles` 不含禁用角色 → 不计入 `effectivePriority`（防提权比较） |
| superadmin 保护 | `IsSuperadminUser*` / `CountActiveSuperadminUsers*` 均要求角色启用 |
| casbin_rule | **不清除**（策略保留在 DB）——重新启用即恢复，无需重配菜单 |

> 系统角色（superadmin/admin 等 `is_system=true`）不可禁用：`UpdateRole` 返回 `ErrRoleIsSystem`。
> 用户列表按角色筛选（`role_code` 查询参数）**不过滤**禁用角色——筛选是数据查询语义，管理员仍可按禁用角色查找历史绑定用户。

#### 角色写操作的目标校验

> B1-2 修复（源自 [review/01 §R2-RM-01](../review/01-phase1-systematic-review-findings.md)）：此前仅校验新 priority 值，未校验操作者与目标角色的强弱关系。

角色模块三个写操作统一接入 `canManageTarget`（与用户模块同语义：**操作者须严格更强** `actorP < targetP`，superadmin 直通）：

| 操作 | 校验链 |
|------|--------|
| `UpdateRole` | `is_system` → **目标档位**（ensureCanManageRole）→ **新 priority 值**（ensureRolePriorityAllowed） |
| `DeleteRole` | `is_system` → **目标档位** → 用户绑定 → 最后超管保护 |
| `AssignMenus` | **系统角色仅 superadmin 可改**（ErrRoleIsSystem）→ **目标档位** → 菜单校验 |

失败返回 `403 + 30010`（`ErrCannotManageHigher`，通用防提权码）。

---

## 测试用例

### Service 层

| 用例 | 输入 | 预期 |
|------|------|------|
| 创建角色 | name="用户管理员" | 返回角色 |
| 创建角色 - 指定 priority | priority=15 | 介于 admin(10) 与 operator(20) 之间 |
| 种子角色 priority | migrate-up 后查 roles | superadmin=1, admin=10, operator=20, viewer=30 |
| 创建角色 - 名称重复 | 已存在的 name | 返回 ErrRoleAlreadyExists |
| 删除角色 - 无关联用户 | roleID | 成功 |
| 删除角色 - 有关联用户 | 有用户的 roleID | 返回 ErrRoleInUse |
| 角色列表 - admin 不可见 superadmin | admin 调 `GET /roles` | 无 `code=superadmin` 项 |
| 分配菜单 | roleID + menuIDs | role_menus 更新 + casbin_rule 更新 |
| 分配菜单 - 菜单不存在 | 不存在的 menuID | 返回 ErrMenuNotFound |
| 分配菜单后策略生效 | 分配后用该角色请求 API | Casbin 放行 |
| 取消菜单后策略失效 | 取消后用该角色请求 API | Casbin 拒绝 |
| 禁用角色 - 权限收回 | 禁用某角色后其成员请求原 API | 403（下次请求生效） |
| 禁用角色 - 菜单不下发 | 禁用后成员调 `GET /user/menus` | 不含该角色菜单 |
| 禁用角色 - 重新启用 | 再次启用后成员请求原 API | 恢复放行（策略未清除） |
| 禁用系统角色 | status=0 且 is_system=true | 返回 ErrRoleIsSystem |
| 删除更强角色 | 低权自定义角色（priority=25）删更强角色（priority=15） | 403 + 30010 |
| 降权更强角色 | 低权角色 Update 更强角色的 priority | 403 + 30010 |
| admin 改系统角色菜单 | 非 superadmin 对 is_system 角色分配菜单 | 403 + 40004 |

### 集成测试

| 用例 | 验证点 |
|------|--------|
| 角色→菜单→策略 完整链路 | 分配菜单后 Casbin 策略表有正确记录 |
| 策略重载 | ReloadPolicy 后新策略立即生效 |
| admin 角色绕过 | admin 角色无需任何策略即可访问所有 API |

---

## 涉及文件

> 目标目录见 [architecture §3.5](../design/architecture.md#35-领域模块目录约定单仓可拆分)。

```
internal/repository/role/
internal/service/role/                # 含 Casbin 策略同步
internal/handler/role/
internal/model/role.go
```
