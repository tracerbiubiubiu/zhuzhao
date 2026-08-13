# 05 - 角色模块（role）

> Step 7，依赖 Step 5（authz）。

---

## 预期功能

| 功能 | 场景 | API |
|------|------|-----|
| 角色列表 | 管理员查看所有角色 | `GET /api/v1/roles` |
| 创建角色 | 管理员创建新角色 | `POST /api/v1/roles` |
| 角色详情 | 查看角色信息 + 关联菜单 | `GET /api/v1/roles/:id` |
| 更新角色 | 修改角色名称、描述 | `POST /api/v1/roles/:id/update` |
| 删除角色 | 删除角色（需检查是否有关联用户） | `POST /api/v1/roles/:id/delete` |
| 分配菜单 | 给角色分配可访问的菜单 | `POST /api/v1/roles/:id/menus` |
| 查看角色菜单 | 获取角色关联的菜单列表 | `GET /api/v1/roles/:id/menus` |
| 查看角色权限 | 获取角色的 Casbin 策略 | `GET /api/v1/roles/:id/permissions` |

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
  │   → 生成策略 p, role::user_manager, /api/v1/users, GET
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

种子数据创建 4 个系统角色：

| code | name | 说明 | Casbin 行为 |
|------|------|------|------------|
| `superadmin` | 超级管理员 | 系统最高权限 | matcher 直接 bypass |
| `admin` | 管理员 | 系统管理权限 | matcher 直接 bypass |
| `operator` | 操作员 | 可管理组织成员 | 需配置 Casbin 策略 |
| `viewer` | 访客 | 只读访问 | 需配置 Casbin 策略 |

Casbin 模型 matcher 中 `r.sub == "role::superadmin" || r.sub == "role::admin"` 直接放行，不需要为超管/管理员分配任何策略。

**业务层仍要分级**（Casbin bypass 不等于业务无约束）：

- 不能删除系统角色（`is_system`）
- `admin` 不能修改 `superadmin` 角色的菜单
- 不能删除仍有用户关联的角色
- 不能删除导致系统失去最后一个 superadmin 绑定的操作（与 user 模块一起校验）

---

## 测试用例

### Service 层

| 用例 | 输入 | 预期 |
|------|------|------|
| 创建角色 | name="用户管理员" | 返回角色 |
| 创建角色 - 名称重复 | 已存在的 name | 返回 ErrRoleAlreadyExists |
| 删除角色 - 无关联用户 | roleID | 成功 |
| 删除角色 - 有关联用户 | 有用户的 roleID | 返回 ErrRoleInUse |
| 分配菜单 | roleID + menuIDs | role_menus 更新 + casbin_rule 更新 |
| 分配菜单 - 菜单不存在 | 不存在的 menuID | 返回 ErrMenuNotFound |
| 分配菜单后策略生效 | 分配后用该角色请求 API | Casbin 放行 |
| 取消菜单后策略失效 | 取消后用该角色请求 API | Casbin 拒绝 |

### 集成测试

| 用例 | 验证点 |
|------|--------|
| 角色→菜单→策略 完整链路 | 分配菜单后 Casbin 策略表有正确记录 |
| 策略重载 | ReloadPolicy 后新策略立即生效 |
| admin 角色绕过 | admin 角色无需任何策略即可访问所有 API |

---

## 涉及文件

```
internal/repository/role_repo.go      # 角色数据访问
internal/service/role_service.go      # 角色业务逻辑（含策略同步）
internal/handler/role_handler.go      # HTTP Handler
internal/model/role.go                # 角色模型
```
