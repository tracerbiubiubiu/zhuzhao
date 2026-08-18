# 角色模块设计

> 模块代码（目标路径）：`internal/service/role/` + `internal/repository/role/` + `internal/handler/role/`
>
> 旧系统参考：`doc/module-assessment-2026-08/role.md` + `policy.md` + `interaction-casbin-sync.md`
>
> 主键以 [phase1/05-role.md](../phase1/05-role.md) 为准（`BIGINT` + `code`）。

---

## 1. 模块定位

**核心底座模块**。角色管理 + Casbin 策略同步。角色是 RBAC 的核心，连接用户、菜单、API 策略。

与其他模块的关系：
- 被 `user` 引用（用户-角色绑定）
- 依赖 `menu`（角色-菜单分配）
- 依赖 `casbin`（策略同步）
- Phase 2 起自注册 `RoleResource` 到 `authz` 的 ResourceRegistry

---

## 2. 数据模型

> 完整 DDL 以 [phase1/05-role.md](../phase1/05-role.md) 为准。

```sql
CREATE TABLE roles (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(50) NOT NULL,
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    priority    INT NOT NULL,                 -- 越小权限越高；业务防提权
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

CREATE TABLE role_menus (
    role_id     BIGINT NOT NULL REFERENCES roles(id),
    menu_id     BIGINT NOT NULL REFERENCES menus(id),
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (role_id, menu_id)
);
```

### Casbin 策略表

由 `noho-digital/casbin-pgx-adapter`（Casbin v3）管理，策略 subject 为 `role::{code}`：

```sql
CREATE TABLE casbin_rule (
    id    BIGSERIAL PRIMARY KEY,
    ptype VARCHAR(10) NOT NULL,
    v0    VARCHAR(255) NOT NULL,
    v1    VARCHAR(255) NOT NULL,
    v2    VARCHAR(255) DEFAULT '',
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
    AssignMenus(ctx context.Context, roleCode string, menuIDs []int64) error
    GetMenus(ctx context.Context, roleCode string) ([]*model.Menu, error)

    // Casbin 策略同步
    SyncPolicies(ctx context.Context, roleCode string) error
    RebuildAllPolicies(ctx context.Context) error
}
```

---

## 4. 核心流程

### 4.1 Casbin 策略生成（借鉴旧系统三分类）

> 下列 `role::editor` 为**业务示例角色**（非系统种子角色），用来说明三分类策略形态。

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
  3. （Phase 3 按需）失效权限缓存：DEL perm:user:*（该角色下所有用户）
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
POST /api/v1/roles/delete

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
-- superadmin + admin 通配策略（种子数据；subject 为 role::{code}）
INSERT INTO casbin_rule (ptype, v0, v1, v2) VALUES
  ('p', 'role::superadmin', '*', '*'),
  ('p', 'role::admin', '*', '*')
ON CONFLICT DO NOTHING;
```

Casbin matcher 中 `r.sub == "role::superadmin" || r.sub == "role::admin"` 直接 bypass；其余角色靠 `keyMatch2` 匹配 `p` 策略。内置角色与 superadmin/admin 业务差异见 [phase1/05-role §superadmin 与 admin](../phase1/05-role.md#superadmin-与-admin-的区别)。

---

## 5. 角色继承

### 5.1 BFS 三源合并（借鉴旧系统）

旧系统的角色展开有三来源：

```
用户的有效角色 = BFS 展开三源合并：
  1. 直接角色：user_roles 表
  2. 组织角色：user_orgs → org_roles（**仅该组织**绑定的角色；子组织成员不继承父组织 org_roles）
  3. 继承角色：roles.parent_id（角色继承链）
```

### 5.2 g 表消除（借鉴旧系统）

Casbin 模型无 `[role_definition] g` 段，角色继承在中间件层 BFS 展开，不写 g 表。

**matcher**: `r.sub == p.sub && (p.obj == "*" || keyMatch2(r.obj, p.obj)) && (r.act == p.act || p.act == "*")`

中间件展开后的角色列表逐个 enforce，任一匹配即放行。

### 5.3 Redis 权限缓存（Phase 3 / 按需）

Phase 1–2 **不使用** `perm:user:{userId}`。Phase 3 多实例/热点场景可按需引入：

```
perm:user:{userId} → {
  "roles": ["admin", "editor"],
  "permissions": ["user:create", ...]
}
TTL: 30min
```

---

## 6. 旧系统借鉴

| 设计 | 决策 | 理由 |
|------|------|------|
| 策略三分类（route/button/api） | ✅ 直接采用 | 比纯 API 策略更精细 |
| 菜单-API 绑定自动生成策略 | ✅ 直接采用 | 亮点设计 |
| g 表消除 + BFS 展开 | ✅ Phase 2b 采用 | 简化 Casbin 模型；Phase 1 仅直接角色 |
| 增量 diff 重建（非全量清空） | ✅ Phase 1 采用 | 角色菜单变更同事务写 casbin_rule |
| 超管通配策略 | ✅ 直接采用 | 简洁 |
| expanded_roles 存 context | ✅ 直接采用 | 避免双倍查询 |
| BFS 三源合并 | ⏳ Phase 2b | Phase 1 仅 user_roles 直接角色 |
| Redis 缓存展开结果 | ⏳ Phase 3 按需 | Phase 1–2 查 DB |
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
- 策略三分类完整实现（route/button/api）

### Phase 3

- Redis 缓存展开结果（`perm:user:{userId}`，按需）
- 增量 diff 重建优化（大数据量场景）
- 角色继承可视化
- 角色预设（RolePresetProvider）
