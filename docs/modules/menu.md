# 菜单模块设计

> 模块代码（目标路径）：`internal/service/menu/` + `internal/repository/menu/` + `internal/handler/menu/`
>
> 旧系统参考：`doc/module-assessment-2026-08/menu.md` + `dynamic-routing-research.md`
>
> 主键以 [phase1/07-menu.md](../phase1/07-menu.md) 为准（`BIGINT` + `code`）。

---

## 1. 模块定位

**核心底座配套模块**。菜单管理 + 前端权限数据源。菜单是动态路由的核心，连接角色和 API 策略。

> **菜单 ≠ 前端页面**：`menus` 表存路由/按钮**元数据**；Vue 组件在前端仓库。系统内置项由 **migration 种子** 写入；`POST /menus` 供管理员在「菜单管理」页**登记**增补项。见 [phase1/07-menu §菜单从哪里来](../phase1/07-menu.md#菜单从哪里来种子-sql-vs-登记-api)。

与其他模块的关系：
- 被 `role` 引用（角色-菜单分配）
- 菜单-API 绑定是 Casbin API 策略的来源
- 为前端提供菜单树和权限码

---

## 2. 数据模型

> 完整 DDL 见 [phase1/07-menu.md](../phase1/07-menu.md)。

```sql
CREATE TABLE menus (
    id          BIGSERIAL PRIMARY KEY,
    parent_id   BIGINT REFERENCES menus(id),
    code        VARCHAR(50) NOT NULL,
    name        VARCHAR(100) NOT NULL,
    menu_type   SMALLINT NOT NULL,
    path        VARCHAR(200),
    component   VARCHAR(200),
    icon        VARCHAR(100),
    permission  VARCHAR(100),
    sort_order  INT DEFAULT 0,
    visible     BOOLEAN DEFAULT TRUE,
    is_system   BOOLEAN DEFAULT FALSE,
    version     INT DEFAULT 1,
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_menus_parent ON menus(parent_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_menus_code ON menus(code) WHERE deleted_at IS NULL;
```

### 菜单-API 绑定

操作类 API 路径不含 `:id`（id 放 POST body），须完整写入 `menu_apis` 供 Casbin 策略生成：

```sql
CREATE TABLE menu_apis (
    menu_id     BIGINT NOT NULL REFERENCES menus(id) ON DELETE CASCADE,
    api_path    VARCHAR(200) NOT NULL,
    api_method  VARCHAR(10) NOT NULL,
    PRIMARY KEY (menu_id, api_path, api_method)
);
```

---

## 3. 接口定义

```go
type MenuService interface {
    // CRUD
    Create(ctx context.Context, req CreateMenuRequest) (*model.Menu, error)
    // Update 为 patch 语义（D2-17）：path/component/icon/permission/sort_order 指针化——
    // 未传（nil）保持现值（原全量覆盖零值穿透：未传 component/icon 即被清空）
    Update(ctx context.Context, code string, req UpdateMenuRequest) error
    Delete(ctx context.Context, code string) error
    GetByCode(ctx context.Context, code string) (*model.Menu, error)
    // GetTree 管理端完整菜单树：model.Menu 递归嵌套 Children（db:"-"），含按钮节点
    // （角色分配菜单需勾选按钮）。Phase 1 实现：internal/service/menu_service.go buildMenuTree。
    GetTree(ctx context.Context) ([]model.Menu, error)

    // API 绑定
    BindAPIs(ctx context.Context, menuCode string, apis []APIRef) error
    GetAPIs(ctx context.Context, menuCode string) ([]APIRef, error)

    // 前端权限数据
    // 用户侧菜单树（不含按钮）。Phase 1 实际方法名为 GetUserMenus（internal/service/menu_service.go）。
    GetUserMenuTree(ctx context.Context, userID int64) ([]model.Menu, error)
    GetUserPermissions(ctx context.Context, userID int64) ([]string, error)
}
```

---

## 4. 核心流程

### 4.1 菜单类型层级约束（借鉴旧系统）

```
目录(type=1) → 可包含 目录(1) + 菜单(2)
菜单(type=2) → 可包含 按钮(3)
按钮(type=3) → 不可有子节点
```

创建/更新菜单时校验类型层级。

**类型必要字段（B4-4）**：页面(type=2)必有 `path`（动态路由注册）、按钮(type=3)必有 `permission`（权限码下发）——缺失返回 400，防止「树里有节点、权限码里无路由」的矛盾数据。

### 4.2 前端菜单树构建

```
GET /api/v1/user/menus

1. 获取用户角色（Phase 1 仅 `user_roles` 直接角色；Phase 2b 起 BFS 三源展开）
   → roles = ["admin", "editor"]

2. 查角色绑定的菜单
   → SELECT menu_id FROM role_menus
     WHERE role_id IN (SELECT id FROM roles WHERE code = ANY($1))

3. 查菜单详情（仅 menu_type=1 和 menu_type=2，不含按钮）
   → SELECT * FROM menus
     WHERE id IN (...) AND menu_type IN (1, 2) AND visible = true AND deleted_at IS NULL
     ORDER BY sort_order

4. 构建树结构（递归或迭代）
   → 根据 parent_id 组装树

5. 返回菜单树
```

### 4.3 权限码查询

```
GET /api/v1/user/permissions

1. 获取用户角色
2. 查角色绑定的菜单
3. 查菜单中的按钮权限码
   → SELECT permission FROM menus
     WHERE id IN (...) AND menu_type = 3 AND permission <> '' AND deleted_at IS NULL
4. 合并 route 权限码
   → 菜单 type=1/2 的 path → "route:" + path
5. 返回权限码列表
   → ["button:user:create", "button:user:delete", "route:/system/user", ...]
```

### 4.4 删除菜单（级联，借鉴旧系统）

```
POST /api/v1/menus/delete

1. 系统菜单保护
   → menu.is_system == true？返回 403

2. 检查子菜单
   → 有子菜单？返回 409（需先删子菜单）
   -- 或递归删除子树（看需求）

3. 事务开始
   → BEGIN

4. 删除菜单-API 绑定
   → DELETE FROM menu_apis WHERE menu_id = ?

5. 删除角色-菜单绑定
   → DELETE FROM role_menus WHERE menu_id = ?

6. 软删除菜单
   → UPDATE menus SET deleted_at = NOW() WHERE id = ?

7. 事务提交

8. 事务外副作用
   → 触发绑定该菜单的所有角色 SyncPolicies（Casbin 策略更新）
   → 失效权限缓存
```

### 4.5 菜单-API 绑定 → Casbin 策略

菜单绑定 API 后，触发角色策略同步：

```
角色绑定菜单 → 菜单绑定 API → Casbin 策略
  role_menus         menu_apis          casbin_rule
  ──────────         ─────────          ───────────
  editor → user_menu  user_menu → /api/users, GET  →  [editor, /api/users, GET]
                       user_menu → /api/users, POST →  [editor, /api/users, POST]
```

---

## 5. 旧系统借鉴

| 设计 | 决策 | 理由 |
|------|------|------|
| 三类型（目录/菜单/按钮） | ✅ 直接采用 | 前端标准模式 |
| 类型层级约束 | ✅ 直接采用 | 防止错误嵌套 |
| 菜单-API 绑定 | ✅ 直接采用 | 亮点设计，自动生成策略 |
| 权限码三分类（route/button/api） | ✅ 直接采用 | 精细控制 |
| syncSystemMenus desired-state | ✅ 改为 migration 种子数据 | 更简单 |
| IsSystem 保护 | ✅ 直接采用 | 系统菜单不可删 |
| 前端字段（component/icon/sort/visible） | ✅ 直接采用 | 前端渲染需要 |
| swagger 驱动 API 同步 | ✅ Phase 2a Step 0 | 需要 swag 集成（[00 §3 Step 0](../phase2/00-implementation-plan.md) 承接：`make swag` + handler 注解） |

---

## 6. 分阶段实施

### Phase 1

- 菜单 CRUD（含类型层级约束）
- 菜单树查询
- 角色-菜单分配
- 前端菜单树接口（/user/menus）
- 权限码接口（/user/permissions）
- 系统菜单保护
- **Phase 1 菜单清单（目录 + 页面 + 按钮）** — 见 [07-menu §Phase 1 菜单清单](../phase1/07-menu.md#phase-1-菜单清单前端契约)；种子 SQL 须含按钮行

### Phase 2

- 菜单-API 绑定
- Casbin 策略自动同步
| swagger 驱动 API 同步 |
| 删除菜单级联（角色策略更新） |

### Phase 3

- 菜单缓存（Redis）
| 菜单主题/多租户菜单 |
