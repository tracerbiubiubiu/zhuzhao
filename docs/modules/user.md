# 用户模块设计

> 模块代码：`internal/service/user_service.go` + `internal/repository/user_repo.go`
>
> 旧系统参考：`doc/module-assessment-2026-08/user.md`

---

## 1. 模块定位

**核心底座模块**。用户身份管理，是认证和鉴权的主体数据源。管理用户 CRUD、密码、角色绑定、状态。

与其他模块的关系：
- 为 `auth` 提供密码验证
- 依赖 `role` 管理用户角色
- 依赖 `organization` 管理用户组织归属
- 自注册 `UserResource` 到 `authz` 的 ResourceRegistry

---

## 2. 数据模型

```sql
CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username    VARCHAR(50) UNIQUE NOT NULL,
    password    VARCHAR(100) NOT NULL,       -- bcrypt hash
    real_name   VARCHAR(100),
    email       VARCHAR(100),
    phone       VARCHAR(20),
    status      SMALLINT DEFAULT 1,          -- 1=启用 0=禁用
    is_system   BOOLEAN DEFAULT FALSE,
    created_by  VARCHAR(50),                 -- 审计字段，不覆盖
    created_at  TIMESTAMPTZ DEFAULT NOW(),   -- 审计字段，不覆盖
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ                  -- 软删除
);

CREATE INDEX idx_users_username ON users(username) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_status ON users(status) WHERE deleted_at IS NULL;
```

### 关联表

```sql
-- 用户-角色（多对多）
CREATE TABLE user_roles (
    user_id     UUID REFERENCES users(id),
    role_id     UUID REFERENCES roles(id),
    PRIMARY KEY (user_id, role_id)
);

-- 用户-组织（多对多，含角色）
CREATE TABLE user_orgs (
    user_id     UUID REFERENCES users(id),
    org_id      UUID REFERENCES organizations(id),
    role_id     UUID REFERENCES roles(id),
    is_primary  BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (user_id, org_id, role_id)
);
```

---

## 3. 接口定义

```go
type UserService interface {
    // CRUD
    Create(ctx context.Context, req CreateUserRequest) (*model.User, error)
    GetByID(ctx context.Context, id string) (*model.User, error)
    GetByUsername(ctx context.Context, username string) (*model.User, error)
    Update(ctx context.Context, id string, req UpdateUserRequest) error
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, query UserListQuery) ([]*model.User, int64, error)

    // 密码
    UpdatePassword(ctx context.Context, userID, newPassword string) error
    VerifyPassword(ctx context.Context, userID, password string) (bool, error)

    // 状态
    Enable(ctx context.Context, id string) error
    Disable(ctx context.Context, id string) error

    // 角色绑定
    SetRoles(ctx context.Context, userID string, roleIDs []string) error
    GetRoles(ctx context.Context, userID string) ([]*model.Role, error)

    // 组织绑定
    SetOrgs(ctx context.Context, userID string, orgRoles []OrgRole) error
    GetOrgs(ctx context.Context, userID string) ([]*model.Organization, error)
}
```

---

## 4. 核心流程

### 4.1 创建用户

```
POST /api/v1/users {username, password, real_name, ...}

1. 密码强度校验（PasswordValidator）
   → 长度 ≥ 8，4 种字符类，bcrypt 72 字节上限

2. 用户名唯一性检查
   → SELECT * FROM users WHERE username = ? AND deleted_at IS NULL

3. bcrypt 哈希
   → bcrypt.GenerateFromPassword(password, 12)

4. 插入
   → INSERT INTO users (username, password, ...) VALUES (...)

5. 返回用户信息（不含 password）
```

### 4.2 删除用户（级联）

```
DELETE /api/v1/users/:id

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

7. 事务外副作用
   → 吊销 JWT（auth.KickAllDevices）
   → 清除登录锁（Redis DEL lock:login:{username}）
   → 失效权限缓存（Redis DEL perm:user:{userId}）
   → Casbin 策略清理（如有用户级策略）
```

### 4.3 设置用户角色

旧系统已知问题：`SetUserRoles` 用 delete-all → insert-all 无事务保护。

**新框架方案**：用 PostgreSQL 事务。

```go
func (s *UserService) SetRoles(ctx context.Context, userID string, roleIDs []string) error {
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
    // 事务外：失效权限缓存
    // s.redis.DEL(ctx, fmt.Sprintf("perm:user:%s", userID))
}
```

### 4.4 列表查询（含资源级过滤）

```
GET /api/v1/users?page=1&size=20

1. 路由级 Casbin 通过

2. 资源级列表过滤
   → registry.GetFilter(ctx, "user", userID, "read")
   → filter.Where = "creator_id = $1 OR org_id IN (...)"

3. 分页查询
   → SELECT * FROM users WHERE deleted_at IS NULL AND (filter.Where)
     ORDER BY created_at DESC LIMIT $2 OFFSET $3

4. 返回列表（不含 password）
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
| first_login 标记 | ⏳ Phase 2 | 非首期必须 |
| 软删除 | ✅ 采用 | 保留审计数据 |

---

## 7. 分阶段实施

### Phase 1

- 用户 CRUD（含软删除）
- 密码管理（校验 + bcrypt + 修改）
- 用户-角色绑定（事务）
- 用户名唯一性检查
- 系统用户保护（is_system）

### Phase 2

- 用户-组织绑定
- 列表查询资源级过滤
- DeleteUser 级联吊销 JWT
- first_login 强制改密
- 用户导入导出

### Phase 3

- 密码历史（不能重复使用最近 N 次密码）
- 密码过期策略
- 第三方登录预留（OAuth2/OIDC 字段）
