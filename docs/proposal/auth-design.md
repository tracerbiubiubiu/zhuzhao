# 认证鉴权方案

> 完整的认证（AuthN）和鉴权（AuthZ）方案设计，结合旧系统经验和业界 PEP/PDP 分层模型。
>
> 创建日期：2026-08-12

---

## 1. 认证方案（AuthN）

### 1.1 双 Token 机制

| 维度 | 方案 |
|------|------|
| AT（accessToken） | 短期（2h），无状态 JWT，每次请求携带 |
| RT（refreshToken） | 长期（7d），有状态存 Redis，仅用于 /refresh |
| RT 轮换 | 每次刷新废弃旧 RT、签发新 RT（防重放） |
| AT 吊销 | 登出/踢出时加入 Redis 黑名单（TTL = AT 剩余有效期） |
| 多设备 | 每个设备独立 RT，支持查看设备列表和踢出 |

### 1.2 JWT Payload（极简无状态）

```json
{
  "user_id": "xxx",
  "jti": "xxx",
  "exp": 1234567890
}
```

权限信息不入 JWT，走 Redis 缓存（`perm:user:{userId}`），管理员修改权限后主动失效缓存，下次请求即生效。

详见 [design-decisions.md §1](../design/design-decisions.md#1-jwt-无状态策略与权限缓存)。

### 1.3 认证流程

```
登录：
  POST /api/v1/auth/login {username, password}
  → 校验密码（bcrypt）
  → 签发 AT + RT
  → RT 存 Redis: refresh:{userId}:{deviceId}
  → 设备列表 SADD: devices:{userId}
  → 返回 {accessToken, refreshToken}

刷新：
  POST /api/v1/auth/refresh {refreshToken}
  → 校验 RT（Redis 查询 + 轮换）
  → 签发新 AT + 新 RT
  → 旧 RT 删除，新 RT 存 Redis
  → 返回 {accessToken, refreshToken}

登出：
  POST /api/v1/auth/logout
  → AT 加入黑名单: blacklist:at:{jti} (TTL = AT 剩余)
  → 删除当前设备 RT
  → 返回 200
```

### 1.4 登录安全（借鉴旧系统）

| 机制 | 实现 | 说明 |
|------|------|------|
| 登录限流 | Redis Lua 原子脚本：5 次失败锁 15 分钟 | INCR + EXPIRE + SET 锁定标记原子完成 |
| fail-close | Redis 故障返回 503 | 安全优先 |
| 防用户枚举 | 用户不存在和密码错误返回相同响应 | 不泄露用户是否存在 |
| 密码复杂度 | 4 种字符（大写/小写/数字/特殊）+ 最小 8 位 | 借鉴旧系统 PasswordValidator |
| bcrypt 上限 | 72 字节 | bcrypt 算法限制 |
| first_login 改密 | 首次登录强制改密 | 可选，Phase 2 |

### 1.5 权限缓存

```
Redis 缓存:
perm:user:{userId} → {
  "roles": ["admin", "editor"],
  "org_id": "xxx",
  "permissions": ["user:create", "article:read", ...]
}
TTL: 30min

失效时机：
  1. 管理员修改用户角色 → DEL perm:user:{userId}
  2. 管理员修改角色权限 → DEL perm:user:*（该角色下所有用户）
  3. 多实例时 → Pub/Sub 广播 cache:invalidate:perm:user:{userId}
```

---

## 2. 鉴权方案（AuthZ）

### 2.1 分层鉴权架构

遵循 OWASP / NIST PEP/PDP 分层模型：

```
请求 → JWT 中间件（认证，PEP-0）
     │  验证 Token 签名 + 过期 + 黑名单
     │  提取 user_id
     │
     → Casbin 中间件（路由级鉴权，PEP-1）
     │  查询 Casbin 策略表（独立数据源，不依赖业务 Service）
     │  判断：角色 × API 路径 × HTTP 方法
     │  超管 bypass：admin 角色 × * × *
     │
     → Handler → Service（资源级鉴权，PEP-2）
        调用 ResourceRegistry.Authorize(ctx, resourceCode, req)
        → 具体 Resource 实现判断：
           ├─ 超管 bypass
           ├─ 属主判断（代码内联，O(1)）
           ├─ 组织关系（SQL ltree 查询）
           └─ 可配置策略（独立 Casbin enforcer，按需）
```

### 2.2 路由级鉴权（Casbin RBAC）

**Casbin 模型**（借鉴旧系统 g 表消除）：

```ini
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && (p.obj == "*" || keyMatch2(r.obj, p.obj)) && (r.act == p.act || p.act == "*")
```

- 无 `[role_definition] g` 段，角色继承在中间件层 BFS 展开
- BFS 三源合并：直接角色 + 组织角色 + 继承角色
- expanded_roles 存 gin context 供 handler 复用
- BFS 结果缓存到 Redis（`perm:user:{userId}`）

**策略量**：角色数 × API 数 × 方法数 ≈ 1,000 条（可控）

### 2.3 资源级鉴权（资源抽象 + 代码内联 + 按需 enforcer）

详见 [resource-model.md](./resource-model.md)。

### 2.4 列表过滤（数据级权限）

对于列表查询接口（如"获取工单列表"），需要根据用户权限过滤数据：

```go
// Service 层调用
filter, err := registry.GetFilter(ctx, userID, "ticket", "read")
// filter = SQL WHERE 子句
// 例如：WHERE org_id IN (SELECT org_id FROM org_members WHERE user_id = ? AND org_id IN subtree(?))

query := "SELECT * FROM tickets WHERE " + filter.Where + " ORDER BY created_at DESC"
rows, err := db.Query(ctx, query, filter.Args...)
```

| 策略 | 生成的 WHERE 子句 | 说明 |
|------|------------------|------|
| 超管 | 无（不添加过滤） | 超管看全部 |
| 属主 | `creator_id = $1` | 只看自己创建的 |
| 本组织 | `org_id IN (SELECT org_id FROM org_members WHERE user_id = $1)` | 只看本组织的 |
| 本组织子树 | `org_id IN (SELECT id FROM organizations WHERE path <@ (SELECT path FROM organizations WHERE id = (SELECT org_id FROM user_orgs WHERE user_id = $1 AND is_primary)))` | 看本组织及子组织 |

### 2.5 "鉴权服务自身也需要鉴权"

用户/角色/组织/菜单的管理接口本身也是 HTTP 路由，经过同一套 Casbin 中间件做路由级鉴权。这不是循环依赖：

```
1. JWT 中间件：验证 token → 提取 user_id（不依赖任何业务数据）
2. Casbin 中间件：路由级鉴权（查 casbin_rule 表，不依赖业务 Service）
3. Handler → Service：资源级鉴权（查业务表）
```

Casbin 策略表是独立数据源（`casbin_rule` 表），中间件层只读策略表，Service 层读业务表。两层串行但解耦。

---

## 3. 组织架构方案

### 3.1 实体组织（树形）

```sql
CREATE TABLE organizations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(50) UNIQUE NOT NULL,
    name        VARCHAR(100) NOT NULL,
    parent_id   UUID REFERENCES organizations(id),
    path        LTREE NOT NULL,          -- 如 root.tech.be
    org_type    SMALLINT NOT NULL,       -- 1=公司 2=部门 3=小组
    status      SMALLINT DEFAULT 1,
    is_system   BOOLEAN DEFAULT FALSE,
    created_by  VARCHAR(50),
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_org_path ON organizations USING GIST(path);
CREATE INDEX idx_org_parent ON organizations(parent_id);
```

**ltree 查询示例**：

```sql
-- 查询子树（含自身）
SELECT * FROM organizations WHERE path <@ 'root.tech';

-- 查询祖先链
SELECT * FROM organizations WHERE path @> 'root.tech.be';

-- 查询直接子节点
SELECT * FROM organizations WHERE parent_id = (SELECT id FROM organizations WHERE code = 'tech');
```

### 3.2 虚拟组（独立表）

```sql
CREATE TABLE virtual_groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(50) UNIQUE NOT NULL,
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    status      SMALLINT DEFAULT 1,
    created_by  VARCHAR(50),
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- 虚拟组成员（多对多）
CREATE TABLE virtual_group_members (
    group_id    UUID REFERENCES virtual_groups(id),
    user_id     UUID REFERENCES users(id),
    role_id     UUID REFERENCES roles(id),  -- 组内角色
    PRIMARY KEY (group_id, user_id, role_id)
);
```

### 3.3 组织角色继承

```sql
-- 组织绑定角色（组织成员自动继承）
CREATE TABLE org_roles (
    org_id      UUID REFERENCES organizations(id),
    role_id     UUID REFERENCES roles(id),
    PRIMARY KEY (org_id, role_id)
);
```

成员加入组织 → 自动获得该组织绑定的角色。组织层级中，子组织成员不自动获得父组织角色（向上不继承），但父组织管理员对子组织有管理权限（向下继承管理权）。

---

## 4. 动态路由与前端权限

### 4.1 菜单模型（借鉴旧系统三分类）

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
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);
```

### 4.2 菜单-API 绑定

```sql
CREATE TABLE menu_apis (
    menu_id     UUID REFERENCES menus(id),
    api_path    VARCHAR(200) NOT NULL,
    api_method  VARCHAR(10) NOT NULL,
    PRIMARY KEY (menu_id, api_path, api_method)
);
```

角色绑定菜单 → 菜单关联 API → 自动生成 Casbin API 策略。

### 4.3 前端权限数据

```
GET /api/v1/user/menus → 菜单树（目录+菜单，不含按钮）
GET /api/v1/user/permissions → 权限码列表（按钮权限码 + 路由权限码）
```

---

## 5. 审计日志

### 5.1 日志分类

| 类型 | 存储 | 内容 | 保留期 |
|------|------|------|--------|
| 应用日志 | 文件（slog + Lumberjack） | 运行时调试信息 | 按文件轮转 |
| 审计日志 | PostgreSQL（异步写入） | 用户操作记录 | 180 天 |
| 访问日志 | PostgreSQL（异步写入） | HTTP 请求记录 | 90 天 |

### 5.2 审计日志写入流程

```
请求 → Audit 中间件
     │  记录：user_id, action, method, path, status_code, cost, request_body(截断4KB+脱敏)
     │  → channel（缓冲 1024）
     │
     → 异步 worker goroutine
        → 批量写入 PostgreSQL（每 100 条或每 5 秒）
        → channel 满时降级：同步写入 + 告警
```

### 5.3 敏感字段脱敏

```yaml
# configs/config.yaml
audit:
  sensitive_fields:
    - password
    - token
    - secret
    - private_key
  max_body_length: 4096
```

### 5.4 日志清理

PostgreSQL 不支持 TTL 索引，使用定时任务清理：

```sql
-- 每天 03:00 执行
DELETE FROM audit_logs WHERE created_at < NOW() - INTERVAL '180 days';
DELETE FROM access_logs WHERE created_at < NOW() - INTERVAL '90 days';
```

---

## 6. 安全加固

### 6.1 API 安全

| 机制 | 实现 |
|------|------|
| CORS | gin-contrib/cors，白名单域名 |
| SQL 注入 | 参数化查询（pgx 原生支持） |
| payload 限制 | Gin BodyLimit 中间件（默认 1MB） |
| 安全头 | X-Content-Type-Options, X-Frame-Options, X-XSS-Protection |
| HTTPS | 生产环境强制 HTTPS，HSTS |

### 6.2 配置安全

```yaml
# 敏感配置通过环境变量注入
database:
  password: ${DB_PASSWORD}    # 环境变量
jwt:
  secret: ${JWT_SECRET}       # 环境变量
redis:
  password: ${REDIS_PASSWORD}  # 环境变量
```

---

## 7. 分阶段实施

### Phase 1：最小可用（跑起来）

| 能力 | 范围 |
|------|------|
| 认证 | 登录 + 双 Token + 登出 + 黑名单 |
| 路由级鉴权 | Casbin RBAC 中间件 |
| 资源级鉴权 | ResourceRegistry 骨架 + 代码内联（属主判断） |
| 用户管理 | CRUD |
| 角色管理 | CRUD + 菜单分配 |
| 菜单管理 | CRUD |
| 动态路由 | /user/menus + /user/permissions |
| 种子数据 | admin 角色 + admin 用户 + 初始菜单（幂等） |

### Phase 2：核心完善（业务可用）

| 能力 | 范围 |
|------|------|
| 组织架构 | 组织 CRUD + ltree 树形 + 虚拟组 |
| 资源级鉴权 | 组织关系判断 + 列表过滤 |
| 审计日志 | 中间件记录 + 异步写入 |
| 多设备管理 | 设备列表 + 踢出 |
| 登录安全 | LoginLocker + 密码策略 |
| 可配置策略 | 按需引入独立 enforcer |

### Phase 3：生产加固

| 能力 | 范围 |
|------|------|
| 可观测性 | Prometheus + OpenTelemetry |
| 多租户 | tenant_id 预留启用 |
| 熔断限流 | 降级策略 |
| CI/CD | 自动化部署 |
| 微服务化 | IAM 独立部署 + 业务服务拆分 |
