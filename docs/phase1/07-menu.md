# 07 - 菜单模块（menu）

> **Step 8**，依赖 Step 5（authz）。与 Step 7（role）**可并行**；`AssignMenus` 在 role 模块，菜单 CRUD 在本模块。  
> **`GET /user/menus` / `GET /user/permissions` 在本 Step 交付**（里程碑 M4，见 [README §2.3](./README.md#23-里程碑验收推荐按此推进)）。

---

## 预期功能

> 权限码 SSOT：[§权限码命名规范与 API 对齐](#权限码命名规范与-api-对齐ssot)。`—` 表示仅需登录。

| 功能 | 场景 | API | 权限码 |
|------|------|-----|--------|
| 菜单树 | 管理员查看完整菜单树（含按钮节点） | `GET /api/v1/menus` | `menu:list` |
| **登记菜单** | 管理员在「菜单管理」页登记一条路由/按钮元数据 | `POST /api/v1/menus` | `menu:create` |
| 菜单详情 | 查看单个菜单信息 | `GET /api/v1/menus/:id` | `menu:read` |
| 更新菜单 | 修改名称、path、component、图标等 | `POST /api/v1/menus/update` | `menu:update` |
| 删除菜单 | 删除菜单（需检查子菜单） | `POST /api/v1/menus/delete` | `menu:delete` |
| 当前用户菜单树 | 登录后获取自己的菜单树（按角色过滤） | `GET /api/v1/user/menus` | `—` |
| 当前用户权限码 | 获取自己的按钮/路由权限码 | `GET /api/v1/user/permissions` | `—` |

> **术语**：「菜单」= 前端路由/按钮的**元数据记录**（存 `menus` 表），不是 Vue 页面本身。页面组件在前端仓库；DB 里登记 `path` + `component` + `permission`，供动态路由与按钮显隐。

---

## 菜单从哪里来：种子 SQL vs 登记 API

| 来源 | 何时用 | 谁写 | 例子 |
|------|--------|------|------|
| **迁移种子 SQL**（主路径） | 系统内置页、随版本发布的新功能页 | 开发在 `000002_seed.up.sql`（或新 migration） | Phase 1 的「用户/角色/菜单/组织」 |
| **登记 API** `POST /menus` | 运维/管理员在 UI 上增补、热修元数据；未来租户自定义菜单 | 超级管理员 | 「菜单管理」页里手工加一行 |
| **角色分配** `POST /roles/menus` | 已有菜单记录，决定某角色能看哪些 | 管理员 | 给 `operator` 只勾「用户管理」+ 部分按钮 |

**新增前端路由时的推荐流程**（Phase 1 起）：

```
1. 前端：新增 views/system/ticket/index.vue（组件先写好或占位）
2. 后端：同一 PR / migration 写入 menus + menu_apis 种子（is_system=true）
3. 后端：router 注册对应 API
4. 角色：默认仅 superadmin/admin 全绑；自定义角色由管理员在「角色管理」勾菜单
5. （可选）管理员日后用 POST /menus 微调 name/icon/sort，不改 component 契约
```

> **不要**指望「只改前端、不调 API、不写 migration」——`GET /user/menus` 读的是 DB；没有记录就没有动态路由。登记 API 是**运维入口**，不是替代种子的主路径。

系统内置菜单以 [data-init §4.2](../proposal/data-init.md#42-种子数据内容) 为 SSOT；本文 [§Phase 1 菜单清单](#phase-1-菜单清单前端契约) 为前后端对齐清单。

---

## 核心设计思路

### Menu 结构体

```go
type Menu struct {
    ID         int64      `json:"id,string" db:"id"`
    ParentID   *int64     `json:"parent_id,string" db:"parent_id"`
    Code       string     `json:"code" db:"code"`           // 业务编码
    Name       string     `json:"name" db:"name"`
    MenuType   int        `json:"menu_type" db:"menu_type"` // 1=目录 2=菜单 3=按钮
    Path       string     `json:"path" db:"path"`           // 前端路由
    Component  string     `json:"component" db:"component"`  // 前端组件
    Icon       string     `json:"icon" db:"icon"`
    Permission string     `json:"permission" db:"permission"` // 按钮权限码
    SortOrder  int        `json:"sort_order" db:"sort_order"`
    Visible    bool       `json:"visible" db:"visible"`
    IsSystem   bool       `json:"is_system" db:"is_system"`
    Version    int        `json:"version" db:"version"`
    DeletedAt  *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
    CreatedAt  time.Time  `json:"created_at" db:"created_at"`
    UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
}

// MenuAPI 菜单-API 绑定
type MenuAPI struct {
    MenuID    int64  `json:"menu_id,string" db:"menu_id"`
    APIPath   string `json:"api_path" db:"api_path"`
    APIMethod string `json:"api_method" db:"api_method"`
}
```

### 菜单类型

```
menu_type:
  1 = directory   目录（如"系统管理"，只做分组，不对应页面）
  2 = menu        菜单（如"用户管理"，对应一个页面路由）
  3 = button      按钮（如"删除用户"，不对应页面，只有权限码）
```

### 菜单表结构

> 详见 [modules/menu.md](../modules/menu.md) §2。菜单-API 绑定用独立表 `menu_apis`，不用 JSONB 字段。

```sql
CREATE TABLE menus (
    id          BIGSERIAL PRIMARY KEY,
    parent_id   BIGINT REFERENCES menus(id),
    code        VARCHAR(50) NOT NULL,             -- 业务编码（唯一性由下方部分唯一索引保证）
    name        VARCHAR(100) NOT NULL,          -- 显示名称
    menu_type   SMALLINT NOT NULL,             -- 1=目录 2=菜单 3=按钮
    path        VARCHAR(200),                   -- 前端路由路径（如 /system/user）
    component   VARCHAR(200),                   -- 前端组件路径（如 system/users/index）
    icon        VARCHAR(100),                   -- 图标
    permission  VARCHAR(100),                   -- 权限码（按钮类型，如 user:list, user:create）
    sort_order  INT DEFAULT 0,
    visible     BOOLEAN DEFAULT TRUE,           -- 是否在菜单中显示
    is_system   BOOLEAN DEFAULT FALSE,          -- 系统内置菜单不可删除
    version     INT DEFAULT 1,                  -- 乐观锁
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_menus_parent ON menus(parent_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_menus_deleted ON menus(deleted_at) WHERE deleted_at IS NOT NULL;
-- code 唯一性仅约束活跃记录（部分唯一索引）：软删菜单后同 code 可重建
CREATE UNIQUE INDEX idx_menus_code ON menus(code) WHERE deleted_at IS NULL;

-- 菜单是全局的，Phase 1 不加 tenant_id（用户/角色/组织上的 tenant_id 仅为多租户预留）

-- 菜单-API 绑定表（用于 Casbin 策略生成）
CREATE TABLE menu_apis (
    menu_id     BIGINT NOT NULL REFERENCES menus(id) ON DELETE CASCADE,
    api_path    VARCHAR(200) NOT NULL,          -- 如 /api/v1/users
    api_method  VARCHAR(10) NOT NULL,           -- GET / POST
    PRIMARY KEY (menu_id, api_path, api_method)
);
```

### 菜单-API 绑定与 Casbin 策略生成

> 详见 [modules/role.md](../modules/role.md) §4。角色分配菜单时，遍历菜单关联的 `menu_apis` 生成 Casbin 路由级策略。

```
角色分配菜单 → 遍历 menu_apis → 生成 Casbin 策略
  例如：菜单 "用户管理" 关联了 menu_apis（操作类 POST 的 id 放 body，路径不含 :id）：
    (/api/v1/users, GET)
    (/api/v1/users, POST)                    -- 创建
    (/api/v1/users/:id, GET)                 -- 详情（GET 保留 :id）
    (/api/v1/users/update, POST)
    (/api/v1/users/delete, POST)
    (/api/v1/users/status, POST)
    (/api/v1/users/roles, POST)
    (/api/v1/users/orgs, POST)
    (/api/v1/users/:id/orgs, GET)
    (/api/v1/users/password/reset, POST)
  → 生成策略（路径 + 方法一一对应；`user_manager` 为**示例自定义角色**，非种子四角色）：
    p, role::user_manager, /api/v1/users, GET
    p, role::user_manager, /api/v1/users, POST
    p, role::user_manager, /api/v1/users/:id, GET
    p, role::user_manager, /api/v1/users/update, POST
    p, role::user_manager, /api/v1/users/delete, POST
    ...
```

> 角色/组织/菜单模块同理：种子 `menu_apis` 须覆盖该菜单下所有 GET/POST 路由（含 `/update`、`/delete` 等动词子路径），否则 Casbin 会漏鉴权。

### 当前用户菜单树构建

```
GET /user/menus
  │
  ├── 获取当前用户的所有角色
  ├── 通过 role_menus 表获取角色关联的菜单 ID 集合
  ├── 查询这些菜单（含父级目录，即使父级未分配也要返回，否则树断链）
  ├── 按 sort_order 排序，构建树形结构
  └── 过滤掉 menu_type=button（按钮不进菜单树，走权限码接口）
```

### 当前用户权限码

> 详见 [modules/menu.md](../modules/menu.md) §4。权限码包含按钮码和路由码两部分。

```
GET /user/permissions
  │
  ├── 获取用户角色的所有菜单
  ├── 按钮权限码：menu_type=3 且 permission 非空 → `button:user:create` 等
  ├── 路由权限码：menu_type=1/2 且 path 非空 → `route:/system/user`
  └── 合并返回：`["route:/system/user", "button:user:create", "button:user:delete", ...]`
```

### 系统内置菜单

种子数据创建初始菜单树（目录 + 页面菜单 + **按钮**），标记 `is_system=true`，不可删除。详见 [data-init §4.2](../proposal/data-init.md#42-种子数据内容) 与下节清单。

---

## Phase 1 菜单清单（前端契约）

> **Phase 1 交付物之一**：在写 Vue 之前定好本清单（path / component / permission），前后端与种子 SQL 对齐。  
> 前端仓库约定：`src/views/{component}.vue`（如 `system/user/index.vue` ↔ `component=system/user/index`）。

### 为什么要定按钮？

| 只有页面菜单（type=2） | 加上按钮（type=3） |
|------------------------|-------------------|
| 能控制「进不进这个页面」 | 能控制页内「新建 / 删除 / 导出」等按钮显隐 |
| `GET /user/menus` | `GET /user/permissions` → `button:user:create` 等 |
| Casbin 按 `menu_apis` 拦 API | 按钮码**不走 Casbin**（Phase 1）；前端 `v-permission` / 后端仍靠 API 鉴权 |

**结论：Phase 1 应把 IAM 页的按钮一并写进种子**；否则前端只能整页隐藏，无法做「能看不能删」。

按钮也是 `menus` 表记录（`menu_type=3`，`parent_id` 指向页面菜单），通过 `role_menus` 与页面一起分配给角色。

### 目录 + 页面（与 data-init 一致）

| code | menu_type | name | path | component |
|------|-----------|------|------|-----------|
| `home` | 1 目录 | 首页 | `/home` | `home` |
| `system` | 1 目录 | 系统管理 | `/system` | — |
| `system_user` | 2 菜单 | 用户管理 | `/system/user` | `system/user/index` |
| `system_role` | 2 菜单 | 角色管理 | `/system/role` | `system/role/index` |
| `system_menu` | 2 菜单 | 菜单管理 | `/system/menu` | `system/menu/index` |
| `system_org` | 2 菜单 | 组织管理 | `/system/org` | `system/org/index` |

### 按钮（Phase 1 种子，见 data-init §4.2）

| parent | code | name | permission | 对应 API（已在 menu_apis） |
|--------|------|------|------------|---------------------------|
| `system_user` | `system_user_create` | 新建用户 | `user:create` | POST `/users` |
| `system_user` | `system_user_update` | 编辑用户 | `user:update` | POST `/users/update` |
| `system_user` | `system_user_delete` | 删除用户 | `user:delete` | POST `/users/delete` |
| `system_user` | `system_user_status` | 启用/禁用 | `user:status` | POST `/users/status` |
| `system_user` | `system_user_reset_pwd` | 重置密码 | `user:reset_password` | POST `/users/password/reset` |
| `system_user` | `system_user_assign_role` | 分配角色 | `user:assign_role` | POST `/users/roles` |
| `system_user` | `system_user_assign_org` | 分配组织 | `user:assign_org` | POST `/users/orgs` |
| `system_role` | `system_role_create` | 新建角色 | `role:create` | POST `/roles` |
| `system_role` | `system_role_update` | 编辑角色 | `role:update` | POST `/roles/update` |
| `system_role` | `system_role_delete` | 删除角色 | `role:delete` | POST `/roles/delete` |
| `system_role` | `system_role_assign_menu` | 分配菜单 | `role:assign_menu` | POST `/roles/menus`；含 GET `/roles/:id/menus` |
| `system_menu` | `system_menu_create` | 登记菜单 | `menu:create` | POST `/menus` |
| `system_menu` | `system_menu_update` | 编辑菜单 | `menu:update` | POST `/menus/update` |
| `system_menu` | `system_menu_delete` | 删除菜单 | `menu:delete` | POST `/menus/delete` |
| `system_org` | `system_org_create` | 新建组织 | `org:create` | POST `/orgs` |
| `system_org` | `system_org_update` | 编辑组织 | `org:update` | POST `/orgs/update` |
| `system_org` | `system_org_delete` | 删除组织 | `org:delete` | POST `/orgs/delete` |
| `system_org` | `system_org_move` | 移动组织 | `org:move` | POST `/orgs/move` |
| `system_org` | `system_org_member` | 成员管理 | `org:member` | GET `/orgs/:id/members`、POST `/orgs/members`、POST `/orgs/members/delete` |

### 权限码命名规范与 API 对齐（SSOT）

与工单模块一致：`{resource}:{action}`（见 [modules/ticket.md §2.3](../modules/ticket.md#23-权限矩阵)）。**同一字符串**用于：文档/API 标注、按钮 `menus.permission`、`GET /user/permissions` 的 `button:` 前缀。

**两层 enforcement（名字对齐，机制不同）**：

| 机制 | Phase 1 实际拦截 | 与权限码关系 |
|------|------------------|--------------|
| **Casbin L1** | `menu_apis` → `(path, method)` | 角色绑定**页面菜单**即获得该页全部 `menu_apis`；**不**直接 match `user:list` 字符串 |
| **前端显隐** | `GET /user/permissions` | 写操作 → `button:{permission}`；进页 → `route:{path}` |
| **Phase 2+ 文档/注解** | Handler 注释 `// perm: user:create` | 与按钮码同表，便于 swagger/审计对齐 |

**动词表（IAM Phase 1）**：

| 动词 | 含义 | 典型 HTTP |
|------|------|-----------|
| `list` | 列表 | GET 集合 |
| `read` | 详情/子资源读 | GET `/:id`、GET 子路径 |
| `create` | 新建 | POST 集合 |
| `update` | 更新 | POST `/…/update` |
| `delete` | 删除 | POST `/…/delete` |
| `status` | 启用/禁用 | POST `/…/status` |
| `reset_password` | 管理员重置密码 | POST `/…/password/reset` |
| `assign_role` | 分配角色 | POST `/users/roles` |
| `assign_org` | 分配组织 | POST `/users/orgs` |
| `assign_menu` | 分配菜单 | POST `/roles/menus`（含查看已绑菜单 GET） |
| `move` | 移动组织树 | POST `/orgs/move` |
| `member` | 组织成员增删查 | GET/POST `/orgs/…/members*` |
| `audit:list` | 审计日志（**Phase 1 无菜单**） | GET `/audit/logs` |

**页面菜单（type=2）隐式权限** — 无单独按钮，绑定页面即通过 Casbin 获得：

| 页面 code | 权限码 | menu_apis |
|-----------|--------|-----------|
| `system_user` | `user:list` | GET `/api/v1/users` |
| | `user:read` | GET `/api/v1/users/:id`、GET `/api/v1/users/:id/orgs` |
| `system_role` | `role:list` | GET `/api/v1/roles` |
| | `role:read` | GET `/api/v1/roles/:id`、GET `/api/v1/roles/:id/permissions` |
| `system_menu` | `menu:list` | GET `/api/v1/menus` |
| | `menu:read` | GET `/api/v1/menus/:id` |
| `system_org` | `org:list` | GET `/api/v1/orgs` |
| | `org:read` | GET `/api/v1/orgs/:id`、GET `/api/v1/orgs/:id/members` |

**写操作按钮（type=3）** — 与上表 `permission` 列一致；`GET /user/permissions` 返回 `button:{code}`。

**对齐检查结果（2026-08-17）**：

| 项 | 结论 |
|----|------|
| CRUD 动词 `create/update/delete` | ✅ 与 ticket 模块一致 |
| 缺 `user:status` 按钮 | ✅ 已补（对应 POST `/users/status`） |
| `user:reset_password` / `assign_role` / `assign_org` | ✅ 扩展动词，命名已统一 snake_case |
| `role:assign_menu` 覆盖 GET menus | ✅ 一个码管「分配菜单」能力 |
| `org:member` 覆盖成员三 API | ✅ 一个码管「成员管理」面板 |
| 路径示例 `/system/users` | ✅ 已改为 `/system/user`（与 seed 一致） |
| Phase 1 无菜单种子 | `audit:list` 仅 admin/superadmin 通配 Casbin | ✅ 08-audit 已标注；Phase 2+ 补菜单 |

> 新增 API 时：**先**在本表加 `{resource}:{action}`，**再**写 `menu_apis` + 按需加按钮种子；禁止自造同义码（如 `user:reset` 与 `user:reset_password` 并存）。

### `GET /user/menus` 响应（Vue 动态路由契约）

只返回 `menu_type IN (1, 2)`，`visible=true`；**不含按钮**。前端用 `addRoute` 注册：

```json
{
  "code": 0,
  "data": {
    "menus": [
      {
        "id": "1",
        "code": "home",
        "name": "首页",
        "menu_type": 1,
        "path": "/home",
        "component": "home",
        "icon": "home",
        "sort_order": 0,
        "visible": true,
        "children": []
      },
      {
        "id": "2",
        "code": "system",
        "name": "系统管理",
        "menu_type": 1,
        "path": "/system",
        "component": "",
        "icon": "settings",
        "sort_order": 1,
        "visible": true,
        "children": [
          {
            "id": "3",
            "code": "system_user",
            "name": "用户管理",
            "menu_type": 2,
            "path": "/system/user",
            "component": "system/user/index",
            "icon": "user",
            "sort_order": 1,
            "visible": true,
            "children": []
          }
        ]
      }
    ]
  }
}
```

**Vue 侧约定（Phase 1 文档定稿，实现可 Phase 1 末或前端仓库启动时）**：

- `component` 映射：`import.meta.glob('../views/**/*.vue')`，键为 `../views/{component}.vue`。
- 目录（type=1）且 `component` 为空：用布局组件（如 `Layout`）或仅作侧栏分组。
- 登录成功后：`GET /user/menus` → 递归 `router.addRoute` → 再 `GET /user/permissions` 写入 permission store。

### `GET /user/permissions` 响应

```json
{
  "code": 0,
  "data": {
    "permissions": [
      "route:/system/user",
      "route:/system/role",
      "button:user:create",
      "button:user:status",
      "button:user:delete",
      "button:role:assign_menu"
    ]
  }
}
```

前端指令示例：`v-permission="'button:user:delete'"`；store 内统一用 **`button:` 前缀** 与 bare code 二选一，全项目勿混用。

---

## 测试用例

### Service 层

| 用例 | 输入 | 预期 |
|------|------|------|
| 创建菜单 - 目录 | type=1, name="系统管理" | 返回菜单 |
| 登记菜单 - 页面 | type=2, name="用户管理", path="/system/user" | 返回菜单 |
| 登记菜单 - 按钮 | type=3, name="删除用户", permission="user:delete" | 返回菜单 |
| 登记菜单 - 页面缺 path（B4-4） | type=2, 无 path | 400（动态路由注册必需） |
| 登记菜单 - 按钮缺 permission（B4-4） | type=3, 无 permission | 400（权限码下发必需） |
| 删除菜单 - 有子菜单 | 有子节点的菜单 | 返回 ErrMenuHasChildren |
| 删除菜单 - 系统内置 | is_system=true | 返回 ErrMenuIsSystem |
| 删除菜单 - 清理角色绑定（B4-4） | 已分配给角色的菜单删除 | role_menus 同事务清理，GetRoleMenuIDs 不回显幽灵勾选 |
| 管理端菜单树 | 含按钮的种子数据 | 返回树形，**含按钮节点**（menu_type=3 出现在页面 children 中，供角色分配勾选） |
| 用户菜单树 - admin | admin 角色 | 返回所有菜单（不含按钮） |
| 用户菜单树 - 普通用户 | 只有部分菜单的角色 | 只返回已分配的菜单（含父级目录，不含按钮） |
| 权限码 - admin | admin 角色 | 返回**全部**按钮权限码（B4-4：通配展开——即使 admin 角色的菜单勾选被清空，权限码仍按全量菜单展开，与 Casbin matcher bypass 对齐） |
| 权限码 - 普通用户 | 部分菜单的角色 | 只返回已分配按钮的权限码 |

### 树构建边界

| 用例 | 验证点 |
|------|--------|
| 空菜单树 | 返回 `[]` |
| 三层嵌套 | 树结构正确 |
| 子菜单分配但父目录未分配 | 父目录自动包含（防断链） |
| sort_order 排序 | 按 sort_order 升序 |

---

## 涉及文件

> 目标目录见 [architecture §3.5](../design/architecture.md#35-领域模块目录约定单仓可拆分)。

```
internal/repository/menu/
internal/service/menu/                # 菜单树 + 权限码
internal/handler/menu/
internal/handler/user/                # GetMenus/GetPermissions（或拆 user/profile）
internal/model/menu.go
```
