# 04 - 用户模块（user）

> Step 2（repo）+ Step 6（service/handler），依赖 Step 1（infra）和 Step 5（authz）。

---

## 预期功能

| 功能 | 场景 | API |
|------|------|-----|
| 用户列表 | 管理员查看用户列表，支持分页和筛选 | `GET /api/v1/users` |
| 创建用户 | **本地账号**开户（见 §用户来源与创建场景）；Phase 1 无 HR 时即唯一入口 | `POST /api/v1/users` |
| 用户详情 | 查看指定用户信息 | `GET /api/v1/users/:id` |
| 更新用户 | 修改用户基本信息 | `POST /api/v1/users/update` |
| 删除用户 | 软删除用户 | `POST /api/v1/users/delete` |
| 启用/禁用 | 禁用用户后无法登录 | `POST /api/v1/users/status` |
| 分配角色 | 给用户分配一个或多个角色 | `POST /api/v1/users/roles` |
| 分配组织 | 给用户绑定一个或多个组织（全量覆盖） | `POST /api/v1/users/orgs` |
| 管理员重置密码 | superadmin 重置用户密码，触发首次改密 | `POST /api/v1/users/password/reset` |
| 当前用户信息 | 获取自己的用户信息 | `GET /api/v1/user/profile` |
| 修改个人信息 | 用户修改自己的基本信息（不可改角色） | `POST /api/v1/user/profile/update` |

---

## 核心设计思路

### User 结构体

```go
type User struct {
    ID                 int64      `json:"id,string" db:"id"`
    Username           string     `json:"username" db:"username"`           // 资料/显示名（非账密登录键）
    EmployeeNo         string     `json:"employee_no" db:"employee_no"`     // 工号；**账密登录键**
    DomainAccount      string     `json:"domain_account" db:"domain_account"` // 域账号（AD sAMAccountName）
    UserDomain         string     `json:"user_domain" db:"user_domain"`       // 所在域（FQDN 或 NETBIOS，如 corp.example.com / CORP）
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
    username             VARCHAR(50) NOT NULL,            -- 资料/显示名；非账密登录键
    employee_no          VARCHAR(50),                     -- 工号（账密登录键；可空则不可登录）
    domain_account       VARCHAR(100),                    -- 域账号 AD sAMAccountName（可空）
    user_domain          VARCHAR(255),                    -- 所在域 FQDN 或 NETBIOS（可空）
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

-- 软删除后 username/工号可复用（工号有值时仍受唯一索引约束）
CREATE INDEX idx_users_username ON users(username) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_users_employee_no ON users(employee_no)
    WHERE deleted_at IS NULL AND employee_no IS NOT NULL AND employee_no <> '';
CREATE UNIQUE INDEX idx_users_domain_account ON users(user_domain, domain_account)
    WHERE deleted_at IS NULL
      AND domain_account IS NOT NULL AND domain_account <> ''
      AND user_domain IS NOT NULL AND user_domain <> '';
CREATE INDEX idx_users_tenant ON users(tenant_id) WHERE deleted_at IS NULL;
```

### 身份标识字段（工号 / 登录名 / 域账号）

办公场景里这几件事**不是同一个概念**，Phase 1 **分列存储、不合并到 `username`**：

| 字段 | 含义 | 示例 | Phase 1 登录 |
|------|------|------|--------------|
| **`id`** | 系统内部主键（BIGSERIAL） | `1001` | ❌ |
| **`employee_no`** | **工号**（HR 业务标识，稳定） | `E20240086` | ✅ **`POST /auth/login` 账密键**（须已填写） |
| **`username`** | **资料/显示名**（可重复） | `zhangsan` | ❌ 不作登录键 |
| **`domain_account`** | **域账号**（AD `sAMAccountName`） | `zhangsan` | ❌ Phase 3 SSO |
| **`user_domain`** | **所在域** | `corp.example.com` 或 `CORP` | ❌ 与域账号成对，预留联邦登录 |
| **`email`** | 邮箱（资料） | `zhang@corp.com` | ❌ |

**为什么保留 `username` 与 `employee_no` 两列？**

- 工号用于 **HR 对账与账密登录**；`username` 仅作界面显示或与工号不同的别名。
- 域账号与工号可能 **完全不同**；SSO 需要 `(user_domain, domain_account)` 独立列。
- 外包/测试号可暂不设工号，但 **无法账密登录**，须补工号或等 SSO。

**唯一性（Phase 1 小结）**：

| 字段 | 是否唯一 | 说明 |
|------|----------|------|
| `employee_no` | ✅ **全局唯一**（有值时） | 登录键；管理端精确查人、HR 对账 |
| `username` | ❌ **可重复** | 资料字段；列表 `?username=` 模糊查询 |
| `domain_account` 单独 | ❌ **不**全局唯一 | 不同 AD 域里可以有同名 `sAMAccountName`（如两个域都有 `zhangsan`） |
| `(user_domain, domain_account)` | ✅ **组合唯一**（两者均有值时） | 与 AD 一致：**同一域内**域账号不重复；跨域可重复 |

**域账号为什么不是单列唯一？**

- AD 里唯一的是 **域内** `sAMAccountName`，不是全公司/global 唯一。
- 多域林、并购子公司、测试域与生产域并存时，`CORP\zhangsan` 与 `LAB\zhangsan` 是两个人。
- Phase 3 SSO / OIDC 外部身份映射也用 **`(issuer/域, subject/域账号)`** 成对定位，不是单看域账号字符串。

**成对约束规则**：

- 只填 `domain_account` 或只填 `user_domain`：**允许**（Phase 1 暂不强制成对；HR/AD 同步时建议成对写入）。
- 两者**同时有值**：必须满足 `(user_domain, domain_account)` 组合唯一；冲突 → **409 + 30008**。
- 两者都空：不参与域唯一校验。

> **登录 vs 列表查询**：`POST /auth/login` 用 **`employee_no` + 密码**（见 [02-auth §登录与工号解析](./02-auth.md#登录与工号解析)）。`username` 仅用于列表模糊筛选 `GET /users?username=`。

**谁可以改（Phase 1）**：

| 字段 | 创建用户 | 管理员 update | profile/update |
|------|----------|---------------|----------------|
| `username` | ✅ | ⚠️ 一般不改（Phase 2 再定改名流程） | ❌ |
| `employee_no` | ✅ | ✅ | ❌ |
| `domain_account` / `user_domain` | ✅ | ✅ | ❌ |
| `real_name` / `email` / `phone` | ✅ | ✅ | ✅ |

**`last_login_ip`**：登录成功时由 `AuthService` 从 `gin.Context.ClientIP()` 写入（Nginx 反代需信任 `X-Forwarded-For`，见 [02-auth §登录成功副作用](./02-auth.md#登录成功副作用)）。只记录**最后一次成功登录** IP。

**Phase 2b HR Sync**：`employee_no` 以 HR 为准；`external_id` 存 HR 主键；`username` / 域字段本地或 HR 策略见 [hr-directory-sync](../proposal/hr-directory-sync.md)。**Phase 3** OAuth/AD 联邦登录用 `domain_account` + `user_domain` 映射外部身份。

### 用户来源与创建场景（`POST /users` 对应什么？）

运行时 **只读本地 PostgreSQL**，不实时调 HR。人员进库有两条写路径（Phase 2b 起用 `users.source` 区分）：

| 来源 | 谁写入 | 典型对象 | 工号/组织 |
|------|--------|----------|-----------|
| **`hr`** | `HRSyncService` 定时 Job | 公司在职员工 | HR 为准；主部门 `user_orgs(source=hr)` |
| **`local`** | 管理端 **`POST /users`** | 见下表 | 管理员填写；可绑 `org_ids` |
| **`system`** | 迁移种子 | 初始 `admin` | 种子数据 |

**`POST /users`（手工创建）适用场景**——**不在 HR 主数据里、或 IAM 侧单独托管的账号**：

| 场景 | 说明 |
|------|------|
| **Phase 1 全期** | 尚无 HR Job；开发/验收/试点全靠手工 + 种子 |
| **外包 / 顾问 / 临时工** | HR 系统无编制，但需登录本系统 |
| **集成 / 巡检 / 演示账号** | 非真实员工；生产可禁用或限权 |
| **HR 接入前的空窗** | 先开本地账号，HR 首次同步后再对账合并（按 `employee_no` / `external_id` 策略，避免重复） |
| **仅需本地账密** | HR 只同步姓名/工号/部门；**工号与初始密码**由管理员开户或 [重置密码](./02-auth.md#管理员重置密码--首次登录改密) |

**不适用 `POST /users`（应用 HR 同步 + 管理端维护）**：

| 场景 | 做法 |
|------|------|
| 正式员工入职 | HR Job **Upsert** `source=hr`；IAM **不**重复创建同工号用户 |
| 换部门 / 离职 | HR Job 更新主部门或 **禁用**；不手工删 HR 用户 |
| 赋角色 / 进虚拟组 | `POST /users/roles`、`POST /users/orgs`（HR **不覆盖** `user_roles`、虚拟组 `user_orgs`） |
| 开通登录（HR 已有人档） | **重置密码**；工号以 HR 同步为准 | 登录用 `employee_no` |

```
Phase 2b+ 写路径分工：

  HR API ──→ HRSyncService ──→ users (source=hr)     组织/姓名/工号/主部门
  管理端 ──→ UserService.Create ──→ users (source=local)   本地账号 + 初始密码
           UserService.Update / ResetPassword / SetRoles …   两类用户均可（HR 域字段有限制）
```

> HR 同步 **不覆盖** 已有 `username`（默认）、`user_roles`、虚拟组成员；详见 [hr-directory-sync §3.2](../proposal/hr-directory-sync.md#32-用户对账)。  
> 若手工创建时填了与 HR 相同的 `employee_no`，以唯一索引 **409**；应等 HR 同步或走对账合并，勿重复开户。

#### 创建时要不要校验「HR 里是否存在」？

**结论：Phase 1 / Phase 2b 均不在 `POST /users` 时实时调用 HR API。**

| 做法 | 是否采用 | 原因 |
|------|----------|------|
| 创建前 **调 HR 接口**查人是否存在 | ❌ | IAM 与 HR **解耦**；HR 宕机不能挡开户；外包/测试号 **本来就不在 HR** |
| 创建前要求「必须在 HR 存在」 | ❌ | 与 `source=local` 场景矛盾 |
| 创建前要求「必须不在 HR」 | ❌ | 无法在不调 HR 的情况下证明 |
| 创建前查 **本地库** + `employee_no` 唯一 | ✅ | HR 已同步的人会在本地有 `source=hr` 行；同工号再 Create → **409** |

**Phase 2b 推荐校验（仅本地 SQL，无 HR 远程调用）**：

```
POST /users Create
  │
  ├─ employee_no 为空
  │     → 允许创建，但 **不能账密登录**（须后续补工号）
  │
  └─ employee_no 有值
        → SELECT … WHERE employee_no = ? AND deleted_at IS NULL
        → 已存在且 source = 'hr'
              → 409 + 30007（或专用文案：「该工号已由 HR 同步，请重置密码/赋角色，勿重复开户」）
        → 已存在且 source = 'local'
              → 409 + 30007（工号冲突）
        → 不存在
              → 允许写入 source = 'local'（仍可能是尚未跑 HR Job 的正式工号 — 见下）
```

**运营约定（不靠 API 强校验 HR）**：

- **正式员工**：应等 HR Job 落库后再 **重置密码 / 赋角色**；管理员 **不应** 手工 Create 同工号。
- **local 账号**：优先 **不填 `employee_no`**；若填了且与将来 HR 同步撞车，以后 HR Upsert 或第二次 Create 会被唯一索引挡住。
- **空窗期**（HR 尚未同步但该员工已在 HR 系统）：允许先 Create `source=local`；**首次 HR Job** 应对账 **合并或拒绝 duplicate**（实现策略见 [hr-directory-sync §3.2](../proposal/hr-directory-sync.md#32-用户对账) — 建议 **同工号 merge 到 source=hr**，保留本地 `username`/密码/`user_roles`）。

Phase 1 无 `source` 字段时：仅 **`employee_no` 唯一索引** 一条规则即可。

```json
// POST /api/v1/users（节选）
{
  "username": "zhangsan",
  "password": "xxx",
  "employee_no": "E20240086",
  "domain_account": "zhangsan",
  "user_domain": "CORP",
  "real_name": "张三",
  "email": "zhang@corp.com"
}
```

### 密码处理

- 创建用户时：明文密码 → bcrypt(cost=12) → 存 DB
- 更新用户时：不传密码则不修改，传了才更新
- 查询用户时：永不返回 password 字段

### 头像（avatar）

`avatar` 表示**用户头像的访问地址**（URL 或对象 key 解析后的 URL），**不是**在库里存图片二进制。

| 项 | Phase 1 | Phase 2b+ |
|----|---------|-----------|
| `users.avatar` 列 | ✅ `VARCHAR(500)`，默认空串 | 同左 |
| API 返回 `avatar` | ✅ profile / 用户详情 / 列表 | 同左 |
| 写入 `avatar` | ✅ 创建、管理员更新、`profile/update` 可传 **外链 URL**（可选字段） | 同左 + 上传后写对象地址 |
| 头像上传 / MinIO | ❌ 不做 | [phase2/storage](../phase2/10-storage.md) 预签名直传 |
| 图片校验、裁剪、默认图服务 | ❌ | 按需 |

**存储原则**：

- **PostgreSQL 只存字符串**（≤500 字符），例如 `https://cdn.example/avatars/u1.webp` 或上传完成后写入的对象 URL。
- **文件本体**在 Phase 2b 起存 **S3 兼容对象存储**（MinIO 等），与工单附件共用 storage 模块。
- Phase 1 前端无图时可用占位（用户名首字等）；**不要求**后端生成默认头像。

**profile/update 可改字段（Phase 1）**：`real_name`、`email`、`phone`、`avatar`（URL 字符串）；**不可改** `username`、角色、组织、状态。

```json
// POST /api/v1/user/profile/update（节选）
{
  "real_name": "张三",
  "email": "zhang@example.com",
  "avatar": "https://example.com/static/avatar.png"
}
```

Phase 2b 上传流程（规划，见 storage 文档）：

```
预签名 URL → 前端直传对象存储 → profile/update 写入返回的对象 URL
```

### 软删除

```sql
-- users 表有 deleted_at 字段
-- 查询时自动过滤：WHERE deleted_at IS NULL
-- 唯一索引需包含 deleted_at：
CREATE INDEX idx_users_username ON users(username) WHERE deleted_at IS NULL;
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

### 多角色与有效 priority

- 一个用户可绑定 **0～N 个**角色（`role_ids` 数组）；**零角色**访问受保护 API 返回 **403 / 70003**（见 [03-authz §用户多角色](./03-authz.md#用户多角色phase-1)）。
- **Casbin / 菜单**：用户全部有效角色的权限 **并集（OR）**——任一角色有权限即放行（Phase 2b 含 BFS 三源，见 [05-role §权限继承模型](./05-role.md#角色-priority-与权限继承模型)）。
- **业务防提权**（重置密码、分配角色等）：使用 `roles.priority`，**数值越小权限越高**（业界常见，与 RuoYi `role_sort` 同类）。

  | priority | 角色 code |
  |----------|-----------|
  | 1 | `superadmin` |
  | 10 | `admin` |
  | 20 | `operator` |
  | 30 | `viewer` |

  - **EffectivePriority** = `min(priority)`（多角色取最强）
  - **能否管理目标用户**：`操作者 EffectivePriority <= 目标 EffectivePriority`
  - **能否分配某角色**：`操作者 EffectivePriority <= 该角色 priority`（只能分配同级或更弱角色）

  `superadmin` vs `admin` 对照见 [05-role §superadmin 与 admin](./05-role.md#superadmin-与-admin-的区别)。

### 用户-组织关联

> 详见 [modules/user.md](../modules/user.md) §2 + [modules/organization.md](../modules/organization.md) §4.3。  
> **双 HTTP 入口、单写逻辑**：用户侧与组织侧 API 均委托 `OrgService`（`AddMember` / `RemoveMember` / `SetUserOrgs`），不在 UserService 内重复写 `user_orgs`。

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

#### API 分工

| 场景 | API | 底层方法 |
|------|-----|----------|
| 组织页：添加/移除一名成员 | `POST /orgs/members`、`POST /orgs/members/delete` | `AddMember` / `RemoveMember` |
| 用户页：批量设置所属组织 | `POST /users/orgs` | `SetUserOrgs`（全量覆盖，同 `SetRoles`） |
| 创建用户时顺带绑组织 | `POST /users` body 含 `org_ids` | 创建成功后同事务调 `SetUserOrgs` |
| 查用户所属组织 | `GET /users/:id/orgs` | `GetUserOrgs` |
| 查组织成员 | `GET /orgs/:id/members` | `GetMembers` |

#### `POST /users/orgs` 请求体

```json
{
  "user_id": "5",
  "org_ids": ["2", "3"],
  "primary_org_id": "2"
}
```

- `org_ids` 全量覆盖：事务内先 `DELETE FROM user_orgs WHERE user_id = ?`，再插入新关联（与角色分配一致）。
- `primary_org_id` 可选；须为 `org_ids` 之一；同一用户最多一条 `is_primary = true`。
- 允许 `org_ids: []` 清空全部组织（用户可不属于任何组织）。

#### 创建用户携带组织

```json
// POST /api/v1/users（节选）
{
  "username": "zhangsan",
  "password": "xxx",
  "real_name": "张三",
  "org_ids": ["2"],
  "primary_org_id": "2"
}
```

`UserService.Create` 在**同一事务**内：`INSERT users` → `OrgService.SetUserOrgs`；任一步失败整体回滚。

```go
func (s *UserService) Create(ctx context.Context, req CreateUserRequest) (*model.User, error) {
    var user *model.User
    err := s.db.BeginTxFunc(ctx, func(tx pgx.Tx) error {
        var err error
        user, err = s.repo.CreateTx(ctx, tx, req)
        if err != nil {
            return err
        }
        if len(req.OrgIDs) > 0 {
            return s.orgSvc.SetUserOrgsTx(ctx, tx, user.ID, req.OrgIDs, req.PrimaryOrgID)
        }
        return nil
    })
    return user, err
}
```

> UserService **注入 `OrgService` 接口**（窄接口仅暴露 `SetUserOrgsTx` / `SetUserOrgs`），禁止 UserRepo 直接写 `user_orgs` 业务规则。

### 数据范围（Phase 1 明确不做）

`GET /users` 等列表接口只做路由级鉴权，**不过滤组织**。有 list 权限即可见全部未删除用户。部门隔离是 Phase 2 资源级过滤。

### 分页

`page` 从 1 起；`page_size` 默认 20，最大 100。超出截断为 100。

### 列表筛选

`GET /users` 支持 query 组合筛选（均为可选，多条件 **AND**）：

| 参数 | 匹配方式 | 结果条数 | 说明 |
|------|----------|----------|------|
| `username` | **模糊**（`username ILIKE '%' || $1 || '%'`） | **0~N，不要求唯一** | 关键字可命中多条 |
| `employee_no` | **精确**（`employee_no = $1`） | **0 或 1** | 工号为业务唯一键；有值则最多一条 |
| `role` | 精确（角色 `code`） | 0~N | 已有 |
| `status` | 精确 | 0~N | 可选 |

```go
type UserListQuery struct {
    Page        int
    PageSize    int
    Username    string // 模糊，非唯一
    EmployeeNo  string // 精确，唯一
    RoleCode    string
    Status      *int
}
```

**示例**：

```
GET /api/v1/users?username=zhang          → 200，list 可含 0~N 条（不要求唯一）
GET /api/v1/users?employee_no=E20240086 → 200，total 为 0 或 1
GET /api/v1/users?username=zhang&employee_no=E20240086 → AND，通常 0 或 1 条
```

Repository：`List(ctx, query)` 动态拼 WHERE；另提供 `FindByEmployeeNo(ctx, no)` 供精确查单条（HR、内部校验）。

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

    // 3. 会话吊销（Phase 1 必须）：AT 标记 + 删全部 RT（见 02-auth §会话吊销）
    s.rdb.Set(ctx, fmt.Sprintf("user:disabled:%d", userID), "1", jwtManager.AccessTTL())
    // DEL refresh:{userId}:*  → 禁止 POST /auth/refresh 换出新 AT
    return nil
}
```

---

## 测试用例

### Repository 层（testcontainers PG）

| 用例 | 验证点 |
|------|--------|
| 创建用户 | 返回的用户有 ID，password 是 bcrypt hash |
| List - username 模糊 | `username=zhang` | 0~N 条，**不要求唯一** |
| List - employee_no 精确 | `employee_no=E20240086` | **0 或 1** 条 |
| FindByEmployeeNo | 存在/不存在工号 | 单条或 ErrNotFound |
| 软删除用户 | deleted_at 不为空，列表查询不包含 |
| 软删除后用户名可复用 | 创建同名用户成功（唯一索引含 deleted_at 条件） |
| 分配角色 | user_roles 表记录正确 |
| 查询用户角色 | 返回角色列表 |

### Service 层（Mock Repo）

| 用例 | 输入 | 预期 |
|------|------|------|
| 创建用户 | 合法参数 | 调用 repo.Create，返回用户 |
| 创建用户 - 重复 username | 相同 username | **允许**（200；与登录无关，username 非登录键） |
| 创建用户 - 工号已存在 | 重复 employee_no | 返回 ErrEmployeeNoAlreadyExists |
| 创建用户 - 域账号冲突 | 同域重复 domain_account | 返回 ErrDomainAccountAlreadyExists |
| 创建用户 - 密码为空 | 无 password | 返回 ErrInvalidParams |
| 禁用用户 | userID | 调用 repo.UpdateStatus + 写 user:disabled |
| 禁用后旧 AT | 已禁用用户的 AT | 403 + 30003 |
| 删除最后一个 superadmin | 仅剩一名超管 | 返回 ErrCannotRemoveLastSuperadmin |
| 分配角色 | userID + roleIDs | 调用 repo.SetRoles |
| 分配角色 - 角色不存在 | 不存在的 roleID | 返回 ErrRoleNotFound |
| 分配组织 | userID + orgIDs | 调用 orgSvc.SetUserOrgs |
| 分配组织 - 组织不存在 | 不存在的 orgID | 返回 ErrOrgNotFound |
| 创建用户含 org_ids | Create + org_ids | 同事务创建用户并 SetUserOrgs |
| admin 分配 superadmin | admin 操作 | 返回越权错误（403 + 30009） |
| admin 重置同级 admin | 两用户均为 admin 角色 | 403 + 30005 |
| 多角色 EffectivePriority | 用户绑 viewer(30)+operator(20) | EffectivePriority=20；operator 不能重置该用户 |
| 多角色 OR 鉴权 | 用户绑 viewer+user_manager | viewer 无 POST 权限但 user_manager 有 → 200 |

### Handler 层（httptest）

| 用例 | 请求 | 预期 |
|------|------|------|
| 列表分页 | `GET /users?page=1&page_size=20` | 200 + 分页结构 |
| 列表 - username 模糊 | `GET /users?username=zhang` | 200；`total` 可 >1，**不要求唯一** |
| 列表 - employee_no 精确 | `GET /users?employee_no=E20240086` | 200；`total` 为 0 或 1 |
| 列表 - admin 不可见 superadmin 用户 | admin 调 `GET /users` | 列表中无绑定 superadmin 的用户 |
| 详情 - admin 查 superadmin 用户 id | admin 调 `GET /users/:id` | **404** |
| 列表筛选 | `GET /users?role=admin` | 200 + 筛选结果 |
| 创建 - 参数缺失 | `POST /users` 无 username | 400 |
| 创建 - 成功 | `POST /users` 合法 body | 201 |
| 创建含 org_ids | `POST /users` + org_ids | 201 + user_orgs 正确 |
| 分配组织 | `POST /users/orgs` | 200 + user_orgs 全量覆盖 |
| 详情 - 不存在 | `GET /users/99999` | 404 |
| 删除 - 成功 | `POST /users/delete` body `{ "id": "1" }` | 200 |

---

## 涉及文件

> 目标目录见 [architecture §3.5](../design/architecture.md#35-领域模块目录约定单仓可拆分)。

```
internal/repository/user/
internal/service/user/
internal/handler/user/
internal/model/user.go
```
