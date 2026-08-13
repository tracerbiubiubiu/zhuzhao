# 菜单模块设计

> 模块代码：`internal/service/menu_service.go` + `internal/repository/menu_repo.go`
>
> 旧系统参考：`doc/module-assessment-2026-08/menu.md` + `dynamic-routing-research.md`

---

## 1. 模块定位

**核心底座配套模块**。菜单管理 + 前端权限数据源。菜单是动态路由的核心，连接角色和 API 策略。

与其他模块的关系：
- 被 `role` 引用（角色-菜单分配）
- 菜单-API 绑定是 Casbin API 策略的来源
- 为前端提供菜单树和权限码

---

## 2. 数据模型

```sql
CREATE TABLE menus (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id   UUID REFERENCES menus(id),
    code        VARCHAR(50) UNIQUE NOT NULL,  -- 唯一标识
    name        VARCHAR(100) NOT NULL,
    type        SMALLINT NOT NULL,             -- 1=目录 2=菜单 3=按钮
    path        VARCHAR(200),                  -- 前端路由路径
    component   VARCHAR(200),                  -- 前端组件路径
    icon        VARCHAR(50),
    permission  VARCHAR(100),                  -- 按钮权限码（type=3 时）
    sort        INT DEFAULT 0,
    visible     BOOLEAN DEFAULT TRUE,
    status      SMALLINT DEFAULT 1,
    is_system   BOOLEAN DEFAULT FALSE,
    created_by  VARCHAR(50),
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX idx_menus_parent ON menus(parent_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_menus_code ON menus(code) WHERE deleted_at IS NULL;
```

### 菜单-API 绑定

```sql
CREATE TABLE menu_apis (
    menu_id     UUID REFERENCES menus(id),
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
    Update(ctx context.Context, code string, req UpdateMenuRequest) error
    Delete(ctx context.Context, code string) error
    GetByCode(ctx context.Context, code string) (*model.Menu, error)
    GetTree(ctx context.Context) ([]*MenuNode, error)

    // API 绑定
    BindAPIs(ctx context.Context, menuCode string, apis []APIRef) error
    GetAPIs(ctx context.Context, menuCode string) ([]APIRef, error)

    // 前端权限数据
    GetUserMenuTree(ctx context.Context, userID string) ([]*MenuNode, error)
    GetUserPermissions(ctx context.Context, userID string) ([]string, error)
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

### 4.2 前端菜单树构建

```
GET /api/v1/user/menus

1. 获取用户角色（BFS 展开后）
   → roles = ["admin", "editor"]

2. 查角色绑定的菜单
   → SELECT menu_id FROM role_menus
     WHERE role_id IN (SELECT id FROM roles WHERE code = ANY($1))

3. 查菜单详情（仅 type=1 和 type=2，不含按钮）
   → SELECT * FROM menus
     WHERE id IN (...) AND type IN (1, 2) AND status = 1 AND visible = true
     ORDER BY sort

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
     WHERE id IN (...) AND type = 3 AND permission != ''
4. 合并 route 权限码
   → 菜单 type=1/2 的 path → "route:" + path
5. 返回权限码列表
   → ["button:user:create", "button:user:delete", "route:/system/user", ...]
```

### 4.4 删除菜单（级联，借鉴旧系统）

```
DELETE /api/v1/menus/:code

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
| swagger 驱动 API 同步 | ⏳ Phase 2 | 需要 swag 集成 |

---

## 6. 分阶段实施

### Phase 1

- 菜单 CRUD（含类型层级约束）
- 菜单树查询
- 角色-菜单分配
- 前端菜单树接口（/user/menus）
| 权限码接口（/user/permissions） |
| 系统菜单保护 |

### Phase 2

- 菜单-API 绑定
- Casbin 策略自动同步
| swagger 驱动 API 同步 |
| 删除菜单级联（角色策略更新） |

### Phase 3

- 菜单缓存（Redis）
| 菜单主题/多租户菜单 |
