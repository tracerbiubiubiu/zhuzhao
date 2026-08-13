# 角色模块设计

> 模块代码：`internal/service/role_service.go` + `internal/repository/role_repo.go`
>
> 旧系统参考：`doc/module-assessment-2026-08/role.md` + `policy.md` + `interaction-casbin-sync.md`

---

## 1. 模块定位

**核心底座模块**。角色管理 + Casbin 策略同步。角色是 RBAC 的核心，连接用户、菜单、API 策略。

与其他模块的关系：
- 被 `user` 引用（用户-角色绑定）
- 依赖 `menu`（角色-菜单分配）
- 依赖 `casbin`（策略同步）
- 自注册 `RoleResource` 到 `authz` 的 ResourceRegistry

---

## 2. 数据模型

```sql
CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(50) UNIQUE NOT NULL,  -- "admin", "editor"
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    parent_id   UUID REFERENCES roles(id),    -- 角色继承（可选）
    status      SMALLINT DEFAULT 1,
    is_system   BOOLEAN DEFAULT FALSE,
    created_by  VARCHAR(50),
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- 角色-菜单（多对多）
CREATE TABLE role_menus (
    role_id     UUID REFERENCES roles(id),
    menu_id     UUID REFERENCES menus(id),
    PRIMARY KEY (role_id, menu_id)
);
```

### Casbin 策略表

```sql
-- 路由级策略（全局唯一）
CREATE TABLE casbin_rule (
    id    SERIAL PRIMARY KEY,
    ptype VARCHAR(10) NOT NULL,  -- "p"
    v0    VARCHAR(255) NOT NULL, -- 角色 code
    v1    VARCHAR(255) NOT NULL, -- API 路径
    v2    VARCHAR(255) DEFAULT '', -- HTTP 方法
    v3    VARCHAR(255) DEFAULT '',
    v4    VARCHAR(255) DEFAULT '',
    v5    VARCHAR(255) DEFAULT ''
);
CREATE UNIQUE INDEX idx_casbin_rule ON casbin_rule (ptype, v0, v1, v2);
```

---

## 3. 接口定义

```go
type RoleService interface {
    // CRUD
    Create(ctx context.Context, req CreateRoleRequest) (*model.Role, error)
    GetByCode(ctx context.Context, code string) (*model.Role, error)
    Update(ctx context.Context, code string, req UpdateRoleRequest) error
    Delete(ctx context.Context, code string) error
    List(ctx context.Context) ([]*model.Role, error)
    GetTree(ctx context.Context) ([]*RoleNode, error)  -- 角色继承树

    // 菜单分配
    AssignMenus(ctx context.Context, roleCode string, menuIDs []string) error
    GetMenus(ctx context.Context, roleCode string) ([]*model.Menu, error)

    // Casbin 策略同步
    SyncPolicies(ctx context.Context, roleCode string) error
    RebuildAllPolicies(ctx context.Context) error
}
```

---

## 4. 核心流程

### 4.1 Casbin 策略生成（借鉴旧系统三分类）

角色绑定菜单 → 菜单关联 API → 生成 Casbin 策略：

```
角色 ROLE_EDITOR 绑定菜单 [system_user, system_role]

菜单 system_user (type=2, path=/system/user):
  → route 策略: [role::editor, route:/system/user, access]
  → 绑定的 API:
    → /api/users, GET → API 策略: [role::editor, /api/users, GET]
    → /api/users/:id, GET → API 策略: [role::editor, /api/users/:id, GET]

菜单 system_user 下有按钮 (type=3, permission=user:create):
  → button 策略: [role::editor, button:user:create, access]
```

**策略三分类**（借鉴旧系统）：

| 策略类型 | 格式 | 用途 |
|---------|------|------|
| route | `[role, route:/path, access]` | 前端路由权限 |
| button | `[role, button:perm_code, access]` | 前端按钮权限 |
| API | `[role, /api/path, METHOD]` | 后端接口权限 |

### 4.2 策略同步流程

```
管理员修改角色-菜单绑定 → AssignMenus()
  1. 事务内：写 role_menus 表
  2. 事务外：SyncPolicies(roleCode)
     → 收集角色绑定的菜单
     → 遍历菜单 → 生成 route/button 策略
     → 遍历菜单-API 绑定 → 生成 API 策略
     → 增量 diff（非全量清空）
     → Casbin RemovePolicies(toRemove) + AddPolicies(toAdd)
  3. 失效权限缓存：DEL perm:user:*（该角色下所有用户）
```

### 4.3 增量 diff 重建（借鉴旧系统 cmd/rebuild）

```go
func (s *RoleService) RebuildAllPolicies(ctx context.Context) error {
    // 1. 收集期望策略（从 DB 计算）
    expected, err := s.collectAllExpectedPolicies(ctx)

    // 2. 读取当前 Casbin 策略
    current := s.casbin.GetFilteredPolicy(0)

    // 3. diff
    toAdd, toRemove := diffPolicies(current, expected)

    // 4. 增量更新（非全量清空，避免鉴权空窗）
    if len(toRemove) > 0 {
        s.casbin.RemovePolicies(toRemove)
    }
    if len(toAdd) > 0 {
        s.casbin.AddPolicies(toAdd)
    }
    return nil
}
```

### 4.4 删除角色（级联）

```
DELETE /api/v1/roles/:code

1. 系统角色保护
   → role.is_system == true？返回 403

2. 事务开始
   → BEGIN

3. 检查是否有用户绑定
   → SELECT count(*) FROM user_roles WHERE role_id = ?
   → > 0？返回 409（有用户绑定，不能删除）

4. 删除角色-菜单绑定
   → DELETE FROM role_menus WHERE role_id = ?

5. 删除角色
   → UPDATE roles SET deleted_at = NOW() WHERE id = ?
   -- 或 DELETE FROM roles WHERE id = ?（如无软删除需求）

6. 事务提交
   → COMMIT

7. 事务外副作用
   → Casbin RemoveFilteredPolicy(0, "role::"+code)
   → 失效权限缓存
```

### 4.5 超管通配策略

```sql
-- admin 角色通配策略（种子数据）
INSERT INTO casbin_rule (ptype, v0, v1, v2) VALUES
  ('p', 'admin', '*', '*')
ON CONFLICT DO NOTHING;
```

Casbin matcher 中 `p.obj == "*" || keyMatch2(r.obj, p.obj)` 匹配通配。

---

## 5. 角色继承

### 5.1 BFS 三源合并（借鉴旧系统）

旧系统的角色展开有三来源：

```
用户的有效角色 = BFS 展开三源合并：
  1. 直接角色：user_roles 表
  2. 组织角色：user_orgs → org_roles（组织绑定的角色）
  3. 继承角色：roles.parent_id（角色继承链）
```

### 5.2 g 表消除（借鉴旧系统）

Casbin 模型无 `[role_definition] g` 段，角色继承在中间件层 BFS 展开，不写 g 表。

**matcher**: `r.sub == p.sub && (p.obj == "*" || keyMatch2(r.obj, p.obj)) && (r.act == p.act || p.act == "*")`

中间件展开后的角色列表逐个 enforce，任一匹配即放行。

### 5.3 Redis 缓存

```
perm:user:{userId} → {
  "roles": ["admin", "editor"],     // BFS 展开后的完整角色列表
  "permissions": ["user:create", ...] // 权限码列表
}
TTL: 30min
```

---

## 6. 旧系统借鉴

| 设计 | 决策 | 理由 |
|------|------|------|
| 策略三分类（route/button/api） | ✅ 直接采用 | 比纯 API 策略更精细 |
| 菜单-API 绑定自动生成策略 | ✅ 直接采用 | 亮点设计 |
| g 表消除 + BFS 展开 | ✅ 直接采用 | 简化模型 |
| 增量 diff 重建（非全量清空） | ✅ 直接采用 | 避免鉴权空窗 |
| 超管通配策略 | ✅ 直接采用 | 简洁 |
| expanded_roles 存 context | ✅ 直接采用 | 避免双倍查询 |
| BFS 三源合并 | ✅ 直接采用 | 成熟设计 |
| Redis 缓存展开结果 | ✅ 新增 | 旧系统无缓存，每次查 DB |
| SessionAdapter + mutex | ❌ 不采用 | PostgreSQL 不需要 |
| LoadPolicy 全量重载 | ❌ 改为增量 | 旧系统性能瓶颈 |
| cmd/rebuild 独立二进制 | ⚠️ 改为运行时 Sync | 简化部署 |
| RolePresetProvider 接口 | ⏳ Phase 2 | 预设角色扩展点 |

---

## 7. 分阶段实施

### Phase 1

- 角色 CRUD
- 角色-菜单分配
- Casbin 策略同步（从菜单-API 绑定生成）
- 超管通配策略
- 系统角色保护

### Phase 2

- BFS 三源合并（直接角色 + 组织角色 + 继承角色）
- Redis 缓存展开结果
- 增量 diff 重建
- 策略三分类完整实现

### Phase 3

- 角色继承可视化
| 角色预设（RolePresetProvider） | 预设角色自动同步 |
