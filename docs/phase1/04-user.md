# 04 - 用户模块（user）

> Step 2（repo）+ Step 6（service/handler），依赖 Step 1（infra）和 Step 5（authz）。

---

## 预期功能

| 功能 | 场景 | API |
|------|------|-----|
| 用户列表 | 管理员查看用户列表，支持分页和筛选 | `GET /api/v1/users` |
| 创建用户 | 管理员创建新用户 | `POST /api/v1/users` |
| 用户详情 | 查看指定用户信息 | `GET /api/v1/users/:id` |
| 更新用户 | 修改用户基本信息 | `POST /api/v1/users/:id/update` |
| 删除用户 | 软删除用户 | `POST /api/v1/users/:id/delete` |
| 启用/禁用 | 禁用用户后无法登录 | `POST /api/v1/users/:id/status` |
| 分配角色 | 给用户分配一个或多个角色 | `POST /api/v1/users/:id/roles` |
| 当前用户信息 | 获取自己的用户信息 | `GET /api/v1/user/profile` |
| 修改个人信息 | 用户修改自己的基本信息（不可改角色） | `POST /api/v1/user/profile/update` |

---

## 核心设计思路

### User 结构体

```go
type User struct {
    ID                 int64      `json:"id,string" db:"id"`
    Username           string     `json:"username" db:"username"`
    Password           string     `json:"-" db:"password"`              // 永不返回
    RealName           string     `json:"real_name" db:"real_name"`
    Email              string     `json:"email" db:"email"`
    Phone              string     `json:"phone" db:"phone"`
    Avatar             string     `json:"avatar" db:"avatar"`
    Status             int        `json:"status" db:"status"`           // 1=启用 0=禁用
    MustChangePassword bool       `json:"must_change_password" db:"must_change_password"`
    LastLoginAt        *time.Time `json:"last_login_at" db:"last_login_at"`
    LastLoginIP        string     `json:"last_login_ip" db:"last_login_ip"`
    IsSystem           bool       `json:"is_system" db:"is_system"`
    TenantID           int64      `json:"tenant_id,string" db:"tenant_id"` // 默认 1
    Version            int        `json:"version" db:"version"`            // 乐观锁
    DeletedAt          *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
    CreatedAt          time.Time  `json:"created_at" db:"created_at"`
    UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`
}
```

### users 建表 SQL

```sql
CREATE TABLE users (
    id                   BIGSERIAL PRIMARY KEY,
    username             VARCHAR(50) NOT NULL,
    password             VARCHAR(100) NOT NULL,           -- bcrypt hash
    real_name            VARCHAR(100),
    email                VARCHAR(100),
    phone                VARCHAR(20),
    avatar               VARCHAR(500),
    status               SMALLINT NOT NULL DEFAULT 1,     -- 1=启用 0=禁用
    must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
    last_login_at        TIMESTAMPTZ,
    last_login_ip        VARCHAR(50),
    is_system            BOOLEAN DEFAULT FALSE,
    tenant_id            BIGINT NOT NULL DEFAULT 1,
    version              INT DEFAULT 1,
    deleted_at           TIMESTAMPTZ,
    created_at           TIMESTAMPTZ DEFAULT NOW(),
    updated_at           TIMESTAMPTZ DEFAULT NOW()
);

-- 软删除唯一索引：删除后用户名可复用
CREATE UNIQUE INDEX idx_users_username ON users(username) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_tenant ON users(tenant_id) WHERE deleted_at IS NULL;
```

### 密码处理

- 创建用户时：明文密码 → bcrypt(cost=12) → 存 DB
- 更新用户时：不传密码则不修改，传了才更新
- 查询用户时：永不返回 password 字段

### 软删除

```sql
-- users 表有 deleted_at 字段
-- 查询时自动过滤：WHERE deleted_at IS NULL
-- 唯一索引需包含 deleted_at：
CREATE UNIQUE INDEX idx_users_username ON users(username) WHERE deleted_at IS NULL;
```

### 用户-角色关联

> 详见 [modules/user.md](../modules/user.md) §2。用户和角色是多对多关系，分配角色时在 DB 事务内先删后插。

```sql
CREATE TABLE user_roles (
    user_id BIGINT NOT NULL REFERENCES users(id),
    role_id BIGINT NOT NULL REFERENCES roles(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY(user_id, role_id)
);
```

分配角色时先删后插（全量覆盖，事务内）：

```go
func (r *UserRepo) SetRoles(ctx context.Context, userID int64, roleIDs []int64) error {
    tx, _ := r.pool.Begin(ctx)
    defer tx.Rollback(ctx)
    // 删除旧关联
    tx.Exec(ctx, "DELETE FROM user_roles WHERE user_id = $1", userID)
    // 插入新关联
    for _, roleID := range roleIDs {
        tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", userID, roleID)
    }
    return tx.Commit(ctx)
}
```

> **旧系统修复**：旧系统 SetRoles 非事务（先 DELETE 再 INSERT，中间崩溃产生无角色用户），新框架用 PostgreSQL 事务保证原子性。

### 用户-组织关联

> 详见 [modules/user.md](../modules/user.md) §2 + [modules/organization.md](../modules/organization.md) §2.4。

```sql
CREATE TABLE user_orgs (
    user_id     BIGINT NOT NULL REFERENCES users(id),
    org_id      BIGINT NOT NULL REFERENCES organizations(id),
    is_primary  BOOLEAN DEFAULT FALSE,
    joined_at   TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY(user_id, org_id)
);
```

Phase 1 **不含**组织内角色。`role_id` 在 Phase 2 用新迁移添加，不能放进 Phase 1 主键（PostgreSQL 中 `NULL` 互不相等，可空 `role_id` 会导致同一用户重复加入同一组织）。

### 数据范围（Phase 1 明确不做）

`GET /users` 等列表接口只做路由级鉴权，**不过滤组织**。有 list 权限即可见全部未删除用户。部门隔离是 Phase 2 资源级过滤。

### 分页

`page` 从 1 起；`page_size` 默认 20，最大 100。超出截断为 100。

### 角色分级与系统保护（业务校验，非 Casbin）

| 规则 | 说明 |
|------|------|
| 不能删除/禁用最后一个 `superadmin` | 避免锁死系统 |
| 不能删除自己 | — |
| `is_system` 用户不可删除 | 种子 admin |
| `admin` 不能改 `is_system` 资源 | 不能删系统角色/菜单/组织，不能改 superadmin 用户 |
| `admin` 不能给他人分配 `superadmin` | 防提权 |
| `admin` 不能重置 `admin`/`superadmin` 密码 | 已在 02-auth 定义 |

禁用/删除用户成功后必须写 `user:disabled:{userId}` 并删除该用户全部 RT（见 02-auth）。Phase 1 即实现，不是 Phase 2。

### 删除用户级联

> 详见 [modules/user.md](../modules/user.md) §4。系统用户保护 + 事务内清理 + 事务外吊销。

```go
func (s *userService) Delete(ctx context.Context, userID int64) error {
    // 1. 系统用户保护
    user, _ := s.repo.GetByID(ctx, userID)
    if user.IsSystem {
        return ErrUserIsSystem  // "系统内置用户不可删除"
    }

    // 2. 事务内清理关联
    tx, _ := s.pool.Begin(ctx)
    defer tx.Rollback(ctx)
    tx.Exec(ctx, "DELETE FROM user_roles WHERE user_id = $1", userID)
    tx.Exec(ctx, "DELETE FROM user_orgs WHERE user_id = $1", userID)
    tx.Exec(ctx, "UPDATE users SET deleted_at = NOW() WHERE id = $1", userID)
    if err := tx.Commit(ctx); err != nil {
        return err
    }

    // 3. 会话吊销（Phase 1 必须）
    s.rdb.Set(ctx, fmt.Sprintf("user:disabled:%d", userID), "1", jwtManager.AccessTTL())
    // DEL refresh:{userId}:*
    return nil
}
```

---

## 测试用例

### Repository 层（testcontainers PG）

| 用例 | 验证点 |
|------|--------|
| 创建用户 | 返回的用户有 ID，password 是 bcrypt hash |
| 查询用户 by username | 正确返回，password 字段不返回 |
| 查询不存在用户 | 返回 ErrNotFound |
| 软删除用户 | deleted_at 不为空，列表查询不包含 |
| 软删除后用户名可复用 | 创建同名用户成功（唯一索引含 deleted_at 条件） |
| 分配角色 | user_roles 表记录正确 |
| 查询用户角色 | 返回角色列表 |

### Service 层（Mock Repo）

| 用例 | 输入 | 预期 |
|------|------|------|
| 创建用户 | 合法参数 | 调用 repo.Create，返回用户 |
| 创建用户 - 用户名已存在 | 重复 username | 返回 ErrUserAlreadyExists |
| 创建用户 - 密码为空 | 无 password | 返回 ErrInvalidParam |
| 禁用用户 | userID | 调用 repo.UpdateStatus + 写 user:disabled |
| 禁用后旧 AT | 已禁用用户的 AT | 401 |
| 删除最后一个 superadmin | 仅剩一名超管 | 返回 ErrCannotRemoveLastSuperadmin |
| 分配角色 | userID + roleIDs | 调用 repo.SetRoles |
| 分配角色 - 角色不存在 | 不存在的 roleID | 返回 ErrRoleNotFound |
| admin 分配 superadmin | admin 操作 | 返回越权错误 |

### Handler 层（httptest）

| 用例 | 请求 | 预期 |
|------|------|------|
| 列表分页 | `GET /users?page=1&page_size=20` | 200 + 分页结构 |
| 列表筛选 | `GET /users?role=admin` | 200 + 筛选结果 |
| 创建 - 参数缺失 | `POST /users` 无 username | 400 |
| 创建 - 成功 | `POST /users` 合法 body | 201 |
| 详情 - 不存在 | `GET /users/99999` | 404 |
| 删除 - 成功 | `POST /users/1/delete` | 200 |

---

## 涉及文件

```
internal/repository/user_repo.go      # 用户数据访问
internal/service/user_service.go      # 用户业务逻辑
internal/handler/user_handler.go      # HTTP Handler
internal/model/user.go                # 用户模型
```
