# 用户模块设计

> 模块代码（目标路径）：`internal/service/user/` + `internal/repository/user/` + `internal/handler/user/`（[§3.5](../design/architecture.md#35-领域模块目录约定单仓可拆分)）
>
> 旧系统参考：`doc/module-assessment-2026-08/user.md`
>
> **主键与分阶段以 [phase1/04-user.md](../phase1/04-user.md) 为准**（`BIGINT`/`int64`，JSON `,string`）。

---

## 1. 模块定位

**核心底座模块**。用户身份管理，是认证和鉴权的主体数据源。管理用户 CRUD、密码、角色绑定、状态。

与其他模块的关系：
- 为 `auth` 提供密码验证
- 依赖 `role` 管理用户角色
- 依赖 `organization` 管理用户组织归属
- Phase 2 起自注册 `UserResource` 到 `authz` 的 ResourceRegistry（Phase 1 为空接口）

---

## 2. 数据模型

```sql
CREATE TABLE users (
    id          BIGSERIAL PRIMARY KEY,
    username    VARCHAR(50) NOT NULL,            -- 资料/显示名；可重复；非账密登录键
    employee_no VARCHAR(50),                   -- 工号（可空，有值全局唯一；软删即释放、可复用）
    domain_account VARCHAR(100),               -- 域账号（与 user_domain 成对唯一；软删即释放、可复用）
    user_domain VARCHAR(255),                  -- 所在域
    password    VARCHAR(100) NOT NULL,       -- bcrypt hash
    real_name   VARCHAR(100),
    email       VARCHAR(100),
    phone       VARCHAR(20),
    avatar      VARCHAR(500),              -- 头像 URL，非二进制；上传见 phase2/storage
    status      SMALLINT DEFAULT 1,          -- 1=启用 0=禁用
    must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
    is_system   BOOLEAN DEFAULT FALSE,
    created_by  VARCHAR(50),                 -- 审计字段，不覆盖
    created_at  TIMESTAMPTZ DEFAULT NOW(),   -- 审计字段，不覆盖
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ                  -- 软删除
);

CREATE INDEX idx_users_username ON users(username) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_status ON users(status) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_users_employee_no ON users(employee_no)
    WHERE deleted_at IS NULL AND employee_no IS NOT NULL AND employee_no <> '';
CREATE UNIQUE INDEX idx_users_domain_account ON users(user_domain, domain_account)
    WHERE deleted_at IS NULL
      AND domain_account IS NOT NULL AND domain_account <> ''
      AND user_domain IS NOT NULL AND user_domain <> '';
```

### 关联表

```sql
-- 用户-角色（多对多；一用户可绑多个角色，权限并集，业务分级取 EffectivePriority = min(priority)）
CREATE TABLE user_roles (
    user_id     BIGINT REFERENCES users(id),
    role_id     BIGINT REFERENCES roles(id),
    PRIMARY KEY (user_id, role_id)
);
```

> Phase 1 通过 `POST /api/v1/users/roles` 分配，`role_ids` 为数组，事务内先删后插（全量覆盖）。路由鉴权 OR、业务分级用 `roles.priority`（越小越强），见 [phase1/03-authz §用户多角色](../phase1/03-authz.md#用户多角色phase-1)、[phase1/04-user §多角色与有效 priority](../phase1/04-user.md#多角色与有效-priority)。

```sql
-- 用户-组织（多对多）。Phase 1 主键不含 role_id（可空列进主键会导致重复入组）
CREATE TABLE user_orgs (
    user_id     BIGINT REFERENCES users(id),
    org_id      BIGINT REFERENCES organizations(id),
    is_primary  BOOLEAN DEFAULT FALSE,
    joined_at   TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, org_id)
);
```

> **Go model 对齐**：Phase 1 `UserOrg` **不得**含 `RoleID` 字段；组织内角色 / `org_member_role` 在 Phase 2c 迁移后另加。骨架若已有 `role_id` tag，Step 1 迁移前删除，避免与 DDL 漂移。

---

## 3. 接口定义

```go
type UserService interface {
    // CRUD
    Create(ctx context.Context, req CreateUserRequest) (*model.User, error)
    GetByID(ctx context.Context, id int64) (*model.User, error)
    GetByUsername(ctx context.Context, username string) (*model.User, error) // 列表/管理：模糊或精确
    FindByEmployeeNo(ctx context.Context, employeeNo string) (*model.User, error) // 登录：工号精确（repo 层，未删除用户）
    Update(ctx context.Context, id int64, req UpdateUserRequest) error
    Delete(ctx context.Context, id int64) error
    List(ctx context.Context, query UserListQuery) ([]*model.User, int64, error)

    // 密码
    UpdatePassword(ctx context.Context, userID int64, newPassword string) error
    VerifyPassword(ctx context.Context, userID int64, password string) (bool, error)

    // 状态
    Enable(ctx context.Context, id int64) error
    Disable(ctx context.Context, id int64) error

    // 角色绑定
    SetRoles(ctx context.Context, userID int64, roleIDs []int64) error
    GetRoles(ctx context.Context, userID int64) ([]*model.Role, error)

    // 组织绑定
    SetOrgs(ctx context.Context, userID int64, orgRoles []OrgRole) error
    GetOrgs(ctx context.Context, userID int64) ([]*model.Organization, error)
}
```

---

## 4. 核心流程

### 4.1 创建用户

```
POST /api/v1/users {username, password, real_name, ...}

1. 密码强度校验（PasswordValidator）
   → 长度 ≥ 8，4 种字符类，bcrypt 72 字节上限

2. 工号唯一性检查（若提供 employee_no；**仅活跃记录**，软删工号即释放、可复用——见 phase1/04-user §唯一性与软删除）
   → SELECT * FROM users WHERE employee_no = ? AND deleted_at IS NULL AND employee_no IS NOT NULL AND employee_no <> ''
   → Phase 2b：若已存在 source=hr → 409，提示走重置密码/赋角色，勿重复开户
   → **不调 HR API**（见 04-user §创建时要不要校验 HR）

3. bcrypt 哈希
   → bcrypt.GenerateFromPassword(password, 12)

4. 插入
   → INSERT INTO users (username, password, ...) VALUES (...)

5. 返回用户信息（不含 password；含 `id` string 与 `employee_no`，见 [phase1/04-user §API 响应字段](../phase1/04-user.md#api-响应字段前端契约)）
```

### 4.2 删除用户（级联）

```
POST /api/v1/users/delete

1. 系统用户保护
   → user.is_system == true？返回 403

2. 事务开始
   → BEGIN

3. 软删除用户
   → UPDATE users SET deleted_at = NOW() WHERE id = ?

4. 删除角色绑定
   → DELETE FROM user_roles WHERE user_id = ?

5. 删除组织绑定
   → DELETE FROM user_orgs WHERE user_id = ?

6. 事务提交
   → COMMIT

7. 事务外副作用（Phase 1）
   → SET user:disabled:{userId}，DEL 全部 refresh:{userId}:*
   → DEL lock:login:{employee_no}
```

### 4.3 设置用户角色

旧系统已知问题：`SetUserRoles` 用 delete-all → insert-all 无事务保护。

**新框架方案**：用 PostgreSQL 事务。

```go
func (s *UserService) SetRoles(ctx context.Context, userID int64, roleIDs []int64) error {
    return s.db.BeginTxFunc(ctx, func(tx pgx.Tx) error {
        // 事务内删除旧角色
        if _, err := tx.Exec(ctx, "DELETE FROM user_roles WHERE user_id = $1", userID); err != nil {
            return err
        }
        // 事务内插入新角色
        for _, roleID := range roleIDs {
            if _, err := tx.Exec(ctx,
                "INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
                userID, roleID); err != nil {
                return err
            }
        }
        return nil
    })
}
```

> Phase 3 引入 `perm:user:{userId}` 缓存后，SetRoles 成功再 `DEL` 该 key。

### 4.4 列表查询

**Phase 1**：仅路由级 Casbin，有 `GET /users` 权限即可见全部未删除用户。

**Query 筛选**（见 [phase1/04-user §列表筛选](../phase1/04-user.md#列表筛选)）：

| 参数 | 匹配 | 结果 |
|------|------|------|
| `username` | 模糊 | 0~N，不要求唯一 |
| `employee_no` | 精确 | 0 或 1 |

```go
type UserListQuery struct {
    Page       int
    PageSize   int
    Username   string // ILIKE 模糊
    EmployeeNo string // 精确
    RoleCode   string
    Status     *int
}
```

**Phase 2+**（资源级过滤示例）：

```
GET /api/v1/users?page=1&size=20

1. 路由级 Casbin 通过
2. registry.GetFilter(ctx, "user", userID, "read")
3. SELECT ... WHERE deleted_at IS NULL AND (filter.Where) ...
```

---

## 5. 密码安全（借鉴旧系统 PasswordValidator）

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| MinLength | 8 | 最小长度 |
| RequireUpper | true | 需大写字母 |
| RequireLower | true | 需小写字母 |
| RequireDigit | true | 需数字 |
| RequireSpecial | true | 需特殊字符 |
| SpecialChars | `!@#$%^&*` | 特殊字符集 |
| BcryptMaxLen | 72 | bcrypt 算法上限 |

```go
type PasswordValidator struct {
    MinLength      int
    RequireUpper   bool
    RequireLower   bool
    RequireDigit   bool
    RequireSpecial bool
    SpecialChars   string
}

func (v *PasswordValidator) Validate(password string) error {
    if len(password) > 72 {
        return ErrPasswordTooLong
    }
    if len(password) < v.MinLength {
        return ErrPasswordTooShort
    }
    // 检查字符复杂度...
    return nil
}
```

---

## 6. 旧系统借鉴

| 设计 | 决策 | 理由 |
|------|------|------|
| PasswordValidator 4 种字符复杂度 | ✅ 直接采用 | 成熟设计 |
| bcrypt 72 字节上限 | ✅ 直接采用 | 算法限制 |
| CreateUser 密码校验 + 唯一性检查 | ✅ 直接采用 | 标准流程 |
| SetUserRoles delete-all→insert-all 无事务 | ❌ 修复 | 用 PostgreSQL 事务 |
| ListUsers 合并 restrict 数据级过滤 | ✅ 改为 ResourceRegistry.GetFilter | 统一接口 |
| 13 个依赖注入 | ⚠️ 精简 | 用窄接口（ISP）减少依赖 |
| 窄接口（UserMenuService 等） | ✅ 采用 | 接口隔离原则 |
| first_login 标记 | ✅ Phase 1 | `must_change_password` + JWT `mcp` |
| 软删除 | ✅ 采用 | 保留审计数据 |
| 最后一个 superadmin 保护 | ✅ Phase 1 | 不可禁用/删除/降级最后一个超管 |

---

## 7. 分阶段实施

### Phase 1

- 用户 CRUD（含软删除）
- 密码管理（bcrypt + 修改；复杂度策略可简化）
- 用户-角色绑定（事务）
- 用户-组织绑定：`OrgService.SetUserOrgs` / `AddMember`；API 为 `POST /users/orgs`（全量）、`POST /orgs/members`（单条增删）、创建用户 `org_ids`；读 `GET /users/:id/orgs`
- 工号唯一性检查（username 可重复）
- 系统用户保护（`is_system`）+ 最后一个 superadmin 保护
- 禁用/删除后吊销会话（`user:disabled` + 删全部 RT）
- `must_change_password` 强制改密
- **头像**：`avatar` 字段预留 + API 透传 URL；**不上传**（Phase 2b storage）
- Phase 1 **不做**列表的组织范围数据过滤

### Phase 2

- 组织内角色 / 临时成员有效期（`user_orgs.expires_at`）
- **HR 用户同步**：仅更新 `source=hr` 用户与 **主部门**（`user_orgs.is_primary=true, source=hr`）；不覆盖虚拟组绑定与 `user_roles`（见 [hr-directory-sync.md](../proposal/hr-directory-sync.md)）
- 列表查询资源级过滤（GetFilter）
- 用户导入导出
- **头像上传**（预签名 + 对象存储，更新 `users.avatar`）
- 密码复杂度完整策略

### Phase 3

- 密码历史（不能重复使用最近 N 次密码）
- 密码过期策略
- 第三方登录预留（OAuth2/OIDC 字段）
