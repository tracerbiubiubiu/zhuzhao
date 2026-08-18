# 数据初始化与幂等性方案

> 结合旧系统 zhuzhao 的初始化经验和幂等性问题，设计新框架的数据初始化方案。
>
> 创建日期：2026-08-12

---

## 1. 问题背景

旧系统存在一个问题：每次重启时执行初始化逻辑（如创建 admin 用户），会导致 admin 用户的 `created_by`、`created_at` 等时间戳被更新，覆盖原始创建信息。这类"初始化不幂等"的问题在新框架中需要规避。

旧系统的初始化通过 4 个独立二进制完成：
- `cmd/init`：同步 Role→Menu→API 预设数据（desired-state，幂等）
- `cmd/rebuild`：Casbin 策略增量 diff 重建
- `cmd/dedup`：casbin_rule 去重
- `cmd/sync-apis`：从 swagger 同步 API

新框架用 PostgreSQL + golang-migrate 简化这流程，但核心的幂等性原则不变。

---

## 2. 初始化分层

初始化分为三个层次，职责和执行时机不同：

```
层次 1：Schema 迁移（golang-migrate，显式执行）
  ├─ 建表、索引、外键、ltree 扩展
  ├─ 时机：部署前手动执行或 CI/CD 自动执行
  └─ 幂等性：golang-migrate 自身保证（版本号追踪）

层次 2：种子数据（migration 文件，随 Schema 一起执行）
  ├─ 4 系统角色、admin 用户（绑定 superadmin）、初始菜单、初始组织
  ├─ 时机：migration 执行时
  └─ 幂等性：INSERT ... ON CONFLICT DO NOTHING

层次 3：运行时 Sync（应用启动时，可选）
  ├─ Casbin 策略同步（从角色-菜单关系生成 API 策略）
  ├─ 系统资源保护标记（is_system）
  └─ 时机：App.Run() 启动时
  └─ 幂等性：desired-state sync（更新已存在、删除孤立，但保留 created_at/created_by）
```

---

## 3. Schema 迁移

### 3.1 迁移文件结构

```
migrations/
├── 000001_init.up.sql           # 初始建表（所有表 + 索引 + 外键，含 casbin_rule）
├── 000001_init.down.sql         # 回滚（DROP TABLE）
├── 000002_seed.up.sql           # 种子数据（角色 + 用户 + 组织 + 菜单 + Casbin 初始策略）
├── 000002_seed.down.sql         # 回滚（DELETE 种子数据）
└── ...
```

### 3.2 迁移命令

```makefile
# Makefile
migrate-up:
    migrate -path migrations -database "postgres://zhuzhao:zhuzhao_dev@localhost:5432/zhuzhao?sslmode=disable" up

migrate-down:
    migrate -path migrations -database "postgres://zhuzhao:zhuzhao_dev@localhost:5432/zhuzhao?sslmode=disable" down

migrate-force:
    migrate -path migrations -database "..." force $(VERSION)
```

### 3.3 000001 建表要点（SSOT 索引）

`000001_init.up.sql` 须与 phase1 模块 DDL 一致，**关键列**：

| 表 | Phase 1 必需列 / 约束 | SSOT |
|----|----------------------|------|
| `roles` | `priority INT NOT NULL`、`deleted_at`、部分唯一 `code WHERE deleted_at IS NULL` | [05-role §roles 建表](../phase1/05-role.md#roles-建表-sql) |
| `user_orgs` | 主键 `(user_id, org_id)`；**无** `role_id` | [04-user §用户-组织](../phase1/04-user.md#用户-组织关联) |
| `menus` | `menu_type`（1 目录 / 2 页面 / 3 按钮）、`permission` | [07-menu](../phase1/07-menu.md) |
| `casbin_rule` | Casbin PG adapter 策略表 | [03-authz §Casbin Adapter](../phase1/03-authz.md#casbin-adapter) |

Go `model.Role` 须含 `Priority`、`DeletedAt`；`model.UserOrg` Phase 1 **不含** `RoleID`（见 [modules/user.md §关联表](../modules/user.md#关联表)）。

### 3.4 与旧系统的差异

| 维度 | 旧系统（MongoDB） | 新框架（PostgreSQL） |
|------|------------------|---------------------|
| Schema 管理 | 构造函数自动创建索引（幂等） | golang-migrate 版本化管理 |
| 种子数据 | `cmd/init` 二进制 + JSON 配置 | migration SQL 文件 |
| Casbin 重建 | `cmd/rebuild` 二进制 | `cmd/rebuild` 或运行时 Sync |
| API 同步 | `cmd/sync-apis` 从 swagger | 同（Phase 2） |
| 幂等保证 | desired-state sync | `ON CONFLICT DO NOTHING` + desired-state |

---

## 4. 种子数据设计

### 4.1 幂等性原则

**所有种子数据必须使用 `ON CONFLICT DO NOTHING`**，确保重复执行不覆盖已有数据。

```sql
-- ✅ 正确：幂等，不覆盖
INSERT INTO roles (code, name, is_system) VALUES
  ('admin', '管理员', true)
ON CONFLICT (code) DO NOTHING;

-- ❌ 错误：非幂等，覆盖审计字段
INSERT INTO roles (code, name, is_system) VALUES
  ('admin', '管理员', true)
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name;

-- ❌ 错误：先删后建，重置所有字段
DELETE FROM roles WHERE code = 'admin';
INSERT INTO roles ...;
```

### 4.2 种子数据内容

```sql
-- migrations/000002_seed.up.sql

-- ============================================
-- 角色（4 个系统角色）
-- ============================================
INSERT INTO roles (code, name, description, priority, is_system) VALUES
  ('superadmin', '超级管理员', '系统最高权限，可管理管理员', 1, true),
  ('admin', '管理员', '系统管理员，拥有全部权限', 10, true),
  ('operator', '操作员', '可管理组织成员/角色/子组织', 20, true),
  ('viewer', '访客', '只读访问', 30, true)
ON CONFLICT (code) DO NOTHING;

-- ============================================
-- 组织（树形，ltree path）
-- 系统组织显式指定 ID（1/2/3），运行时新建的组织走自增
-- ============================================
INSERT INTO organizations (id, code, name, parent_id, path, org_type, is_system, tenant_id) VALUES
  (1, 'root', '集团总部', NULL, 'root', 1, true, 1),
  (2, 'tech', '技术中心', 1, 'root.tech', 2, true, 1),
  (3, 'product', '产品中心', 1, 'root.product', 2, true, 1)
ON CONFLICT (code) WHERE deleted_at IS NULL DO NOTHING;
-- ↑ 依赖 phase1/06-organization.md 的部分唯一索引 idx_org_code；PG 15+ 语法
-- Phase 2b 迁移后：种子组织回填 source='system'（与 HR 域 source='hr' 分离，见 hr-directory-sync.md）

-- 重置序列到 max(id)+1，避免后续自增 ID 与种子数据冲突
SELECT setval('organizations_id_seq', (SELECT COALESCE(MAX(id), 0) + 1 FROM organizations));

-- ============================================
-- 超级管理员用户（密码: admin123；登录用工号 E000001，bcrypt hash 需实际生成）
-- ============================================
INSERT INTO users (username, employee_no, password, real_name, status, is_system, tenant_id)
SELECT 'admin', 'E000001', '$2a$12$xxxxx', '系统管理员', 1, true, 1
WHERE NOT EXISTS (
  SELECT 1 FROM users WHERE employee_no = 'E000001'
);

-- ============================================
-- 用户-角色绑定（admin 用户绑定 superadmin 角色）
-- ============================================
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id FROM users u, roles r
WHERE u.username = 'admin' AND r.code = 'superadmin'
ON CONFLICT (user_id, role_id) DO NOTHING;

-- ============================================
-- 用户-组织绑定
-- ============================================
INSERT INTO user_orgs (user_id, org_id, is_primary)
SELECT u.id, o.id, true FROM users u, organizations o
WHERE u.username = 'admin' AND o.code = 'root'
ON CONFLICT (user_id, org_id) DO NOTHING;

-- ============================================
-- 初始菜单（目录→菜单→按钮三层）
-- ============================================
INSERT INTO menus (code, name, menu_type, path, component, icon, sort_order, is_system) VALUES
  ('home', '首页', 1, '/home', 'home', 'home', 0, true),
  ('system', '系统管理', 1, '/system', '', 'settings', 1, true),
  ('system_user', '用户管理', 2, '/system/user', 'system/user/index', 'user', 1, true),
  ('system_role', '角色管理', 2, '/system/role', 'system/role/index', 'role', 2, true),
  ('system_menu', '菜单管理', 2, '/system/menu', 'system/menu/index', 'menu', 3, true),
  ('system_org', '组织管理', 2, '/system/org', 'system/org/index', 'org', 4, true)
ON CONFLICT (code) DO NOTHING;

-- 按钮（menu_type=3；parent 指向页面菜单；完整清单见 phase1/07-menu §Phase 1 菜单清单）
INSERT INTO menus (parent_id, code, name, menu_type, permission, sort_order, is_system)
SELECT p.id, v.code, v.name, 3, v.permission, v.sort_order, true
FROM (VALUES
  ('system_user', 'system_user_create', '新建用户', 'user:create', 1),
  ('system_user', 'system_user_update', '编辑用户', 'user:update', 2),
  ('system_user', 'system_user_delete', '删除用户', 'user:delete', 3),
  ('system_user', 'system_user_status', '启用/禁用', 'user:status', 4),
  ('system_user', 'system_user_reset_pwd', '重置密码', 'user:reset_password', 5),
  ('system_user', 'system_user_assign_role', '分配角色', 'user:assign_role', 6),
  ('system_user', 'system_user_assign_org', '分配组织', 'user:assign_org', 7),
  ('system_role', 'system_role_create', '新建角色', 'role:create', 1),
  ('system_role', 'system_role_update', '编辑角色', 'role:update', 2),
  ('system_role', 'system_role_delete', '删除角色', 'role:delete', 3),
  ('system_role', 'system_role_assign_menu', '分配菜单', 'role:assign_menu', 4),
  ('system_menu', 'system_menu_create', '登记菜单', 'menu:create', 1),
  ('system_menu', 'system_menu_update', '编辑菜单', 'menu:update', 2),
  ('system_menu', 'system_menu_delete', '删除菜单', 'menu:delete', 3),
  ('system_org', 'system_org_create', '新建组织', 'org:create', 1),
  ('system_org', 'system_org_update', '编辑组织', 'org:update', 2),
  ('system_org', 'system_org_delete', '删除组织', 'org:delete', 3),
  ('system_org', 'system_org_move', '移动组织', 'org:move', 4),
  ('system_org', 'system_org_member', '成员管理', 'org:member', 5)
) AS v(parent_code, code, name, permission, sort_order)
JOIN menus p ON p.code = v.parent_code
ON CONFLICT (code) DO NOTHING;

-- ============================================
-- 菜单-API 绑定（用于 Casbin 策略生成）
-- ============================================
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method FROM menus m, (VALUES
  -- system_user（须覆盖该菜单下全部 GET/POST 路由，见 phase1/07-menu.md）
  ('system_user', '/api/v1/users', 'GET'),
  ('system_user', '/api/v1/users', 'POST'),
  ('system_user', '/api/v1/users/:id', 'GET'),
  ('system_user', '/api/v1/users/update', 'POST'),
  ('system_user', '/api/v1/users/delete', 'POST'),
  ('system_user', '/api/v1/users/status', 'POST'),
  ('system_user', '/api/v1/users/roles', 'POST'),
  ('system_user', '/api/v1/users/orgs', 'POST'),
  ('system_user', '/api/v1/users/:id/orgs', 'GET'),
  ('system_user', '/api/v1/users/password/reset', 'POST'),
  -- system_role
  ('system_role', '/api/v1/roles', 'GET'),
  ('system_role', '/api/v1/roles', 'POST'),
  ('system_role', '/api/v1/roles/:id', 'GET'),
  ('system_role', '/api/v1/roles/update', 'POST'),
  ('system_role', '/api/v1/roles/delete', 'POST'),
  ('system_role', '/api/v1/roles/:id/menus', 'GET'),
  ('system_role', '/api/v1/roles/menus', 'POST'),
  ('system_role', '/api/v1/roles/:id/permissions', 'GET'),
  -- system_menu
  ('system_menu', '/api/v1/menus', 'GET'),
  ('system_menu', '/api/v1/menus', 'POST'),
  ('system_menu', '/api/v1/menus/:id', 'GET'),
  ('system_menu', '/api/v1/menus/update', 'POST'),
  ('system_menu', '/api/v1/menus/delete', 'POST'),
  -- system_org
  ('system_org', '/api/v1/orgs', 'GET'),
  ('system_org', '/api/v1/orgs', 'POST'),
  ('system_org', '/api/v1/orgs/:id', 'GET'),
  ('system_org', '/api/v1/orgs/update', 'POST'),
  ('system_org', '/api/v1/orgs/delete', 'POST'),
  ('system_org', '/api/v1/orgs/move', 'POST'),
  ('system_org', '/api/v1/orgs/:id/members', 'GET'),
  ('system_org', '/api/v1/orgs/members', 'POST'),
  ('system_org', '/api/v1/orgs/members/delete', 'POST')
) AS v(menu_code, api_path, api_method)
WHERE m.code = v.menu_code
ON CONFLICT DO NOTHING;

-- 审计查询：Phase 1 种子不单独建「审计管理」菜单。
-- GET /api/v1/audit/logs 由下方 admin/superadmin 通配 Casbin 策略放行。
-- 自定义角色若需查审计，后续再补菜单 + menu_apis（见 phase1/08-audit §路由鉴权）。

-- ============================================
-- 角色-菜单绑定
-- superadmin + admin：全部 IAM 菜单（用户/角色/组织/菜单管理 + 按钮，共 25 条）
-- operator + viewer： intentionally 不插入（Phase 1 无正式业务，由 admin 在角色管理里分配）
-- ============================================
INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id FROM roles r, menus m
WHERE r.code IN ('superadmin', 'admin')
ON CONFLICT (role_id, menu_id) DO NOTHING;

-- ============================================
-- Casbin 路由级策略：admin 通配
-- ============================================
INSERT INTO casbin_rule (ptype, v0, v1, v2) VALUES
  ('p', 'role::admin', '*', '*'),
  ('p', 'role::superadmin', '*', '*')
ON CONFLICT DO NOTHING;
```

### 4.3 回滚

```sql
-- migrations/000002_seed.down.sql
-- 按依赖反序删除（先删关联表，再删主表）

DELETE FROM role_menus WHERE role_id IN (
  SELECT id FROM roles WHERE is_system = true
);
DELETE FROM menu_apis WHERE menu_id IN (
  SELECT id FROM menus WHERE is_system = true
);
DELETE FROM user_roles WHERE role_id IN (
  SELECT id FROM roles WHERE is_system = true
);
DELETE FROM user_orgs WHERE user_id IN (
  SELECT id FROM users WHERE is_system = true
);
DELETE FROM casbin_rule WHERE v0 IN ('role::admin', 'role::superadmin', 'role::operator', 'role::viewer');
DELETE FROM menus WHERE is_system = true;
DELETE FROM users WHERE is_system = true;
DELETE FROM organizations WHERE is_system = true;
DELETE FROM roles WHERE is_system = true;
```

---

## 5. 运行时 Sync

### 5.1 启动流程

```
App.Run()
  → 迁移检查（可选，检查 DB schema 版本是否匹配）
  → Casbin 策略加载（从 casbin_rule 表 LoadPolicy）
  → 资源注册（各 Service 构造函数自注册到 ResourceRegistry）
  → 系统数据 Sync（可选，幂等）
      → Casbin 策略同步（从角色-菜单关系生成 API 策略）
      → 系统资源保护（is_system 标记校验）
  → HTTP 服务启动
```

### 5.2 Sync 安全规则

1. **保留审计字段**：`created_at`、`created_by` 永远不覆盖。upsert 只更新业务字段。
2. **desired-state 模式**：对比期望状态与实际状态，只更新差异部分。
3. **系统资源标记**：`is_system = true` 的记录在 Sync 中受保护，不被删除。
4. **失败不阻塞**：Sync 失败仅 Warn 日志，不阻塞启动（业务可用性优先）。

### 5.3 Casbin 策略同步

```go
// 启动时同步 Casbin 策略（从角色-菜单关系生成）
func (s *RoleService) SyncPolicies(ctx context.Context) error {
    roles, err := s.repo.FindAll(ctx)
    if err != nil {
        return err
    }

    for _, role := range roles {
        // 收集角色绑定的菜单 → 菜单关联的 API → 生成 API 策略
        expected, err := s.collectPolicies(ctx, role.Code)
        if err != nil {
            s.logger.Warn("sync_policies_collect_error", "role", role.Code, "err", err)
            continue
        }

        // 增量 diff（非全量清空重建）
        current, _ := s.casbin.GetFilteredPolicy(0, "role::"+role.Code)
        toAdd, toRemove := diffPolicies(current, expected)

        if len(toRemove) > 0 {
            s.casbin.RemovePolicies(toRemove)
        }
        if len(toAdd) > 0 {
            s.casbin.AddPolicies(toAdd)
        }
    }
    return nil
}
```

### 5.4 与旧系统的差异

旧系统有独立的 `cmd/rebuild` 二进制做 Casbin 策略重建。新框架在运行时 Sync 中完成，不需要独立二进制。如果策略不一致需要手动修复，可以：

```bash
# 触发全量 Sync（重启服务即可）
make dev

# 或提供 CLI 命令（Phase 2）
./server --sync-policies
```

---

## 6. 初始化操作手册

### 6.1 首次部署

```bash
# 1. 启动基础设施
make docker-up   # PostgreSQL + Redis

# 2. 执行迁移 + 种子数据
make migrate-up

# 3. 启动服务
make dev

# 4. 验证
curl -X POST http://localhost:33333/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"employee_no":"E000001","password":"admin123"}'
# 预期：200 OK，返回 accessToken + refreshToken
```

### 6.2 重启服务（幂等）

```bash
# 重启不会影响已有数据
make dev
# 种子数据已在 migration 中，ON CONFLICT DO NOTHING 保证幂等
# 运行时 Sync 只更新业务字段，不覆盖审计字段
```

### 6.3 重置数据库

```bash
# ⚠️ 仅测试环境使用
make docker-down
docker volume rm zhuzhao_pg_data
make docker-up
make migrate-up
```

### 6.4 验证数据完整性

```sql
-- 系统角色（应 4 条）
SELECT count(*) FROM roles WHERE is_system = true;  -- 预期：4

-- admin 用户（应 1 条）
SELECT count(*) FROM users WHERE username = 'admin';  -- 预期：1

-- admin 用户角色绑定（应 1 条，绑定 superadmin）
SELECT count(*) FROM user_roles ur
  JOIN users u ON ur.user_id = u.id
  JOIN roles r ON ur.role_id = r.id
  WHERE u.username = 'admin' AND r.code = 'superadmin';  -- 预期：1

-- 系统菜单（应 6 条）
SELECT count(*) FROM menus WHERE is_system = true;  -- 预期：6 目录/页面 + 19 按钮 = 25（见 07-menu §Phase 1 菜单清单）

-- Casbin 策略（至少 2 条通配）
SELECT count(*) FROM casbin_rule WHERE v1 = '*' AND v2 = '*';  -- 预期：2

-- 验证审计字段未被覆盖
SELECT username, created_at, created_by FROM users WHERE username = 'admin';
-- created_at 应为首次创建时间，不被重启覆盖
```

---

## 7. 关键原则

1. **Schema 迁移和种子数据分离**：建表（000001）和初始数据（000002）是不同的 migration 文件
2. **种子数据用 `ON CONFLICT DO NOTHING`**：不覆盖已有记录
3. **运行时 Sync 保留审计字段**：`created_at`/`created_by` 永远不覆盖
4. **系统资源不删除**：`is_system = true` 的记录在 Sync 中不被清理
5. **Sync 失败不阻塞启动**：仅 Warn 日志
6. **Casbin 策略增量 diff**：不全量清空重建，避免鉴权空窗
