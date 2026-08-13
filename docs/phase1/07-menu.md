# 07 - 菜单模块（menu）

> Step 8，依赖 Step 7（role）。Phase 1 实现菜单 CRUD + 前端权限数据。

---

## 预期功能

| 功能 | 场景 | API |
|------|------|-----|
| 菜单树 | 管理员查看完整菜单树 | `GET /api/v1/menus` |
| 创建菜单 | 管理员创建新菜单（目录/菜单/按钮） | `POST /api/v1/menus` |
| 菜单详情 | 查看单个菜单信息 | `GET /api/v1/menus/:id` |
| 更新菜单 | 修改菜单名称、路径、图标等 | `POST /api/v1/menus/:id/update` |
| 删除菜单 | 删除菜单（需检查子菜单） | `POST /api/v1/menus/:id/delete` |
| 当前用户菜单树 | 登录后获取自己的菜单树（按角色过滤） | `GET /api/v1/user/menus` |
| 当前用户权限码 | 获取自己的按钮权限码列表 | `GET /api/v1/user/permissions` |

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
    code        VARCHAR(50) UNIQUE NOT NULL,    -- 业务编码
    name        VARCHAR(100) NOT NULL,          -- 显示名称
    menu_type   SMALLINT NOT NULL,             -- 1=目录 2=菜单 3=按钮
    path        VARCHAR(200),                   -- 前端路由路径（如 /system/users）
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
  例如：菜单 "用户管理" 关联了 menu_apis:
    (/api/v1/users, GET), (/api/v1/users, POST), (/api/v1/users/:id, GET)
  → 生成策略：
    p, role::user_manager, /api/v1/users, GET
    p, role::user_manager, /api/v1/users, POST
    p, role::user_manager, /api/v1/users/:id, GET
```

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
  ├── 按钮权限码：menu_type=3 且 permission 非空 → "user:create", "user:delete"
  ├── 路由权限码：menu_type=1/2 且 path 非空 → "route:/system/users"
  └── 合并返回：["route:/system/users", "button:user:create", "button:user:delete", ...]
```

### 系统内置菜单

种子数据创建初始菜单树（如"系统管理 > 用户管理 / 角色管理 / 菜单管理 / 组织管理"），标记 `is_system=true`，不可删除。

---

## 测试用例

### Service 层

| 用例 | 输入 | 预期 |
|------|------|------|
| 创建菜单 - 目录 | type=1, name="系统管理" | 返回菜单 |
| 创建菜单 - 菜单 | type=2, name="用户管理", path="/system/users" | 返回菜单 |
| 创建菜单 - 按钮 | type=3, name="删除用户", permission="user:delete" | 返回菜单 |
| 删除菜单 - 有子菜单 | 有子节点的菜单 | 返回 ErrMenuHasChildren |
| 删除菜单 - 系统内置 | is_system=true | 返回 ErrMenuIsSystem |
| 用户菜单树 - admin | admin 角色 | 返回所有菜单 |
| 用户菜单树 - 普通用户 | 只有部分菜单的角色 | 只返回已分配的菜单（含父级目录） |
| 权限码 - admin | admin 角色 | 返回所有按钮权限码 |
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

```
internal/repository/menu_repo.go      # 菜单数据访问
internal/service/menu_service.go      # 菜单业务逻辑 + 树构建
internal/handler/menu_handler.go      # HTTP Handler
internal/handler/user_handler.go      # GetMenus/GetPermissions 在这里
internal/model/menu.go                # 菜单模型
```
