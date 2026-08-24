# 04 - 用户模块（user）

> **Step 2**（repo）+ **Step 6**（service/handler）。  
> 硬依赖 Step 1（infra）、Step 5（authz）。**组织绑定（6b）另需 Step 9**，见 [README §2.2](./README.md#22-step-5-之后可并行)。

---

## Step 6 分期交付

| 分期 | 范围 | 依赖 | 验收 |
|------|------|------|------|
| **6a** | 用户 CRUD、profile、`SetRoles`、`ResetPassword`；**不含** `org_ids` / `POST /users/orgs` | Step 5 | M5 多数 user 用例（deny 路径 M3 即可测） |
| **6b** | `Create` 含 `org_ids`、`POST /users/orgs`、`GET /users/:id/orgs` | Step 9 `OrgService` | M5 #18–#20 |

> `UserService` 注入 `OrgService` 窄接口；6a 阶段 Wire 可注入 stub（`SetUserOrgs` 返回 `ErrNotImplemented`），6b 换真实实现。禁止 UserRepo 直接写 `user_orgs` 业务规则。

---

## 预期功能

> 权限码 SSOT：[07-menu §权限码命名规范](./07-menu.md#权限码命名规范与-api-对齐ssot)。`—` 表示仅需登录、不绑定菜单权限码。

| 功能 | 场景 | API | 权限码 |
|------|------|-----|--------|
| 用户列表 | 管理员查看用户列表，支持分页和筛选 | `GET /api/v1/users` | `user:list` |
| 创建用户 | **本地账号**开户（见 §用户来源与创建场景）；Phase 1 无 HR 时即唯一入口 | `POST /api/v1/users` | `user:create` |
| 用户详情 | 查看指定用户信息 | `GET /api/v1/users/:id` | `user:read` |
| 更新用户 | 修改用户基本信息 | `POST /api/v1/users/update` | `user:update` |
| 删除用户 | 软删除用户 | `POST /api/v1/users/delete` | `user:delete` |
| 启用/禁用 | 禁用用户后无法登录 | `POST /api/v1/users/status` | `user:status` |
| 分配角色 | 给用户分配一个或多个角色 | `POST /api/v1/users/roles` | `user:assign_role` |
| 分配组织 | 给用户绑定一个或多个组织（全量覆盖） | `POST /api/v1/users/orgs` | `user:assign_org` |
| 管理员重置密码 | superadmin 重置用户密码，触发首次改密 | `POST /api/v1/users/password/reset` | `user:reset_password` |
| 当前用户信息 | 获取自己的用户信息 | `GET /api/v1/user/profile` | `—` |
| 修改个人信息 | 用户修改自己的基本信息（不可改角色） | `POST /api/v1/user/profile/update` | `—` |

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

-- username 可重复；软删后仍可再建同名 username
-- 工号、(user_domain, domain_account) 仅在活跃记录间唯一（部分唯一索引过滤软删行，软删后可复用）
CREATE INDEX idx_users_username ON users(username) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_users_employee_no ON users(employee_no)
    WHERE employee_no IS NOT NULL AND employee_no <> '' AND deleted_at IS NULL;
CREATE UNIQUE INDEX idx_users_domain_account ON users(user_domain, domain_account)
    WHERE domain_account IS NOT NULL AND domain_account <> ''
      AND user_domain IS NOT NULL AND user_domain <> ''
      AND deleted_at IS NULL;
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
| `employee_no` | ✅ **全局唯一**（有值时；**仅活跃记录，软删后可复用**） | 登录键；管理端精确查人、HR 对账 |
| `username` | ❌ **可重复** | 资料字段；列表 `?username=` 模糊查询；软删后可再建同名 |
| `domain_account` 单独 | ❌ **不**全局唯一 | 不同 AD 域里可以有同名 `sAMAccountName`（如两个域都有 `zhangsan`） |
| `(user_domain, domain_account)` | ✅ **组合唯一**（两者均有值时；**仅活跃记录，软删后可复用**） | 与 AD 一致：**同一域内**域账号不重复；跨域可重复 |

**域账号为什么不是单列唯一？**

- AD 里唯一的是 **域内** `sAMAccountName`，不是全公司/global 唯一。
- 多域林、并购子公司、测试域与生产域并存时，`CORP\zhangsan` 与 `LAB\zhangsan` 是两个人。
- Phase 3 SSO / OIDC 外部身份映射也用 **`(issuer/域, subject/域账号)`** 成对定位，不是单看域账号字符串。

**成对约束规则**：

- 只填 `domain_account` 或只填 `user_domain`：**允许**（Phase 1 暂不强制成对；HR/AD 同步时建议成对写入）。
- 两者**同时有值**：必须满足 `(user_domain, domain_account)` 组合唯一（仅活跃记录，软删后可复用）；冲突 → **409 + 30008**。
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
| **仅需本地账密** | HR 只同步姓名/工号/部门；**工号与初始密码**由管理员开户或 [重置密码](./02-auth.md#管理员重置密码-首次登录改密) |

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
        → SELECT … WHERE employee_no = ?（仅活跃记录；软删工号可复用）
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

### API 响应字段（前端契约）

> 与 [api/response.md §3.3](../api/response.md#33-主键与-bigint) 一致：**`id` 与写操作 body 中的 `user_id` 均指 `users.id`（JSON string）**；**不是** `employee_no`。  
> 业界常见 B 端做法：**响应里同时返回 `id`（操作键）与 `employee_no`（展示/HR 键）**。

**标识分工**：

| 字段 | 响应 | 写 API body | 说明 |
|------|------|-------------|------|
| `id` | ✅ | `id` / `user_id` | 更新、删除、绑角色/组织 |
| `employee_no` | ✅（有则返回） | 创建/管理员 update 可写 | 列表展示、HR 对账；**登录键** |
| `password` | ❌ | 创建/改密时写入 | 永不出现在响应 |
| `deleted_at` | ❌ | — | 软删用户不出现在列表/详情 |
| `version` | ✅ | `POST /users/update` **须回传** | 乐观锁（见 [10-concurrency](./10-concurrency.md)） |

**各接口 `data` 形状（Phase 1）**：

| 接口 | 用户字段 |
|------|----------|
| `POST /auth/login` | 仅 **token**（`access_token` / `refresh_token` / `expires_in`）；资料走 `GET /user/profile` |
| `GET /user/profile` | 当前用户：`id`、`employee_no`、`username`、`real_name`、`email`、`phone`、`avatar`、`must_change_password` 等；**不含** password、角色、组织 |
| `GET /users`（分页） | `data.list[]`：与详情**同构**（完整 User 结构，含 `version`、域字段、`is_system` 等；`password` 永不返回；软删用户不出现）——B2-9 修订：实现即完整结构，列表/详情共用 |
| `GET /users/:id` | 同上（完整 User 结构） |
| `POST /users` / `POST /users/update` 成功 | 返回更新后的用户对象（含 `id`、`version`；无 password） |

**示例（列表项 / 详情）**：

```json
{
  "id": "1001",
  "employee_no": "E20240086",
  "username": "zhangsan",
  "real_name": "张三",
  "email": "zhang@corp.com",
  "phone": "",
  "avatar": "",
  "status": 1,
  "must_change_password": false,
  "is_system": false,
  "version": 1,
  "created_at": "2026-08-17T10:00:00Z",
  "updated_at": "2026-08-17T10:00:00Z"
}
```

**更新请求须带 `version`**（乐观锁）：

> **patch 语义（B2-3）**：仅更新**显式传入**的字段——未传字段保持不变，传空串显式清空（置 NULL）。`username` 不可更新（Phase 2 再定改名流程）。

```json
// POST /api/v1/users/update（只改 real_name 与 email；其余字段保持原值）
{
  "id": "1001",
  "version": 1,
  "real_name": "张三（新）",
  "email": "zhang@corp.com"
}
```

冲突 → **409 + 10006**（`ErrConcurrentModification`）。

**JWT（Phase 1）**：AT 仅含 `uid`（= `id`）、`username`、`mcp`；**不含** `employee_no`——前端从 profile 或列表取工号展示。

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
-- 业务查询自动过滤：WHERE deleted_at IS NULL
-- 工号 / (user_domain, domain_account) 唯一索引过滤软删行：软删即释放，可复用
-- username 无唯一约束，软删后可再建同名
CREATE INDEX idx_users_username ON users(username) WHERE deleted_at IS NULL;
```

**与唯一性的关系**：

| 字段 | 软删后 |
|------|--------|
| `employee_no` | **释放**；同工号再 Create 可成功 |
| `(user_domain, domain_account)` | **释放**；同组合再 Create 可成功 |
| `username` | **可复用**（非唯一键） |

> 离职/离岗应走 **禁用**（`status=0`），保留工号与 uid 及全部关联；软删会 **释放** 工号与域账号（同号新用户可再建，且软删同时清理 `user_roles` / `user_orgs` 关联）。迁移 000006 已将历史软删行的工号/域账号加 `#del#` 后缀清理占用。

### 用户-角色关联

> **`user_id` = `users.id`**（非 `employee_no`）；与 [§用户-组织关联](#用户-组织关联) 同一约定。

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
  - **能否管理目标用户**（改资料、禁用、删除、重置密码、分配角色等敏感写操作）：`操作者 EffectivePriority < 目标 EffectivePriority`（**严格更强**；admin **不能**动同级 admin）
  - **能否分配某角色**：待分配角色的 `priority >= 操作者 EffectivePriority`（只能分配同级或更弱角色；admin 不能分 superadmin）

  `superadmin` vs `admin` 对照见 [05-role §superadmin 与 admin](./05-role.md#superadmin-与-admin-的区别)。

### 用户-组织关联

> 详见 [modules/user.md](../modules/user.md) §2 + [modules/organization.md](../modules/organization.md) §4.3。  
> **双 HTTP 入口、单写逻辑**：用户侧与组织侧 API 均委托 `OrgService`（`AddMember` / `RemoveMember` / `SetUserOrgs`），不在 UserService 内重复写 `user_orgs`。  
> **标识约定**：下文及 API body 中的 **`user_id` = `users.id`（BIGINT，JSON `,string`）**；**不是** `employee_no`。工号仅用于登录、`GET /users?employee_no=`、HR 对账。

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

#### 路由注册

```go
// internal/router/router.go
users.POST("/orgs", deps.UserHandler.SetUserOrgs)   // 全量覆盖，body: user_id + org_ids
// 组织侧成员 API 见 06-organization §路由注册
```

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
| 不能禁用自己（B4-3） | 自禁用后需他人恢复，易产生工单；与「不能删除自己」对齐 |
| `is_system` 用户不可删除 | 种子 admin |
| `is_system` 用户不可禁用（B4-3） | 与删除保护对齐——种子 admin 被禁用将失去兜底管理入口 |
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
| 软删除后 username / employee_no 可复用 | 同名同工号 Create 成功（部分唯一索引过滤软删行） |
| 软删除后域账号可复用 | 同 `(user_domain, domain_account)` Create 成功 |
| 活跃用户间唯一性 | 同工号 Create → **409 + 30007**；同域域账号 → **409 + 30008** |
| 软删除级联 | `user_roles` / `user_orgs` 关联被清理；软删 0 行时报 ErrUserNotFound 且事务回滚 |
| 超管守护 | 最后一名 superadmin 不可软删/禁用（guard + count + 写同事务）；advisory lock 阻塞竞争事务 |
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
| admin 禁用/删除/改同级 | 非重置密码的写路径 | 403 + 30010（通用防提权码；重置密码保留 30005 专用文案） |
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
| 创建 - 成功 | `POST /users` 合法 body | 200 |
| 创建含 org_ids | `POST /users` + org_ids | 200 + user_orgs 正确 |
| 分配组织 | `POST /users/orgs` | 200 + user_orgs 全量覆盖 |
| 详情 - 不存在 | `GET /users/99999` | 404 |
| 删除 - 成功 | `POST /users/delete` body `{ "user_id": "1" }` | 200 |

---

## 涉及文件

> 目标目录见 [architecture §3.5](../design/architecture.md#35-领域模块目录约定单仓可拆分)。

```
internal/router/router.go               # POST /users/orgs（全量绑组织）；见 06-organization §路由注册
internal/repository/user/
internal/service/user/                  # 注入 OrgService（SetUserOrgsTx）
internal/handler/user/
internal/model/user.go
internal/model/organization.go          # UserOrg：Phase 1 仅 user_id/org_id/is_primary/joined_at
```
