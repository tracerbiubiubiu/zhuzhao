# 认证鉴权方案

> 完整的认证（AuthN）和鉴权（AuthZ）方案设计，结合旧系统经验和业界 PEP/PDP 分层模型。
>
> 创建日期：2026-08-12  
> 修订：2026-08-13  
> **分阶段边界与编码细节以 [`roadmap.md`](../roadmap.md)、[`phase1/`](../phase1/README.md) 为准。** 下文若与 phase 计划冲突，以 phase 计划为准。

---

## 1. 认证方案（AuthN）

### 1.1 双 Token 机制

| 维度 | 方案 |
|------|------|
| AT（accessToken） | 短期（**30min**），HS256 无状态 JWT，每次请求携带 |
| RT（refreshToken） | 长期（7d），有状态存 Redis，仅用于 /refresh |
| RT 轮换 | 每次刷新 `GETDEL` 删旧 RT、签发新 RT（防重放） |
| AT 吊销 | 登出/禁用用户时黑名单 + `user:disabled:{id}` |
| 多设备 | 每设备独立 RT；设备列表/踢出 UI 在 Phase 2 |

### 1.2 JWT Payload（Phase 1）

```json
{
  "uid": 1,
  "username": "admin",
  "jti": "a1b2c3d4e5f6",
  "mcp": false,
  "exp": 1234567890
}
```

- `uid`：`int64`，JSON 加 `,string`
- `mcp`：`must_change_password`，首次改密约束（Phase 1 必做）
- **权限信息不入 JWT**。Phase 1 路由级鉴权由 Casbin 中间件查 `user_roles`（直接角色，无 Redis 缓存）。Phase 3 平台增强再引入 `perm:user:{userId}` 缓存（见 §1.5）。

### 1.3 认证流程

```
登录：
  POST /api/v1/auth/login {employee_no, password}
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
| 登录限流 | Redis **Lua**（15min/5 次，`lock:login:{employee_no}`） | Phase 1 |
| fail-close | 鉴权链路 Redis 故障返回 **503** | 黑名单、`user:disabled` 查询失败禁止放行 |
| 防用户枚举 | 用户不存在和密码错误返回相同响应 | 不泄露用户是否存在 |
| 密码复杂度 | 4 种字符 + 最小 8 位 | Phase 2 完整策略；Phase 1 仅 bcrypt |
| bcrypt 上限 | 72 字节 | bcrypt 算法限制 |
| first_login 改密 | `must_change_password` + JWT `mcp` | **Phase 1 必做** |
| 登录审计 | 成功/失败写 audit | 公开路由，不走 AuditLog 中间件 |

### 1.5 权限缓存（Phase 3 / 按需，Phase 1 不做）

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
        调用 ResourceRegistry.Authorize（**Service 层内联，无 resource_authz 中间件**）
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
m = r.sub == "role::superadmin" || \
    r.sub == "role::admin" || \
    (r.sub == p.sub && (p.obj == "*" || keyMatch2(r.obj, p.obj)) && (r.act == p.act || p.act == "*"))
```

- 无 `[role_definition] g` 段
- **Phase 1**：`RoleFetcher` 只查 `user_roles`（直接角色）；superadmin/admin 在 matcher bypass
- **Phase 2b**：BFS 三源合并（直接 + 组织 + 继承）
- **Phase 3**：可选 Redis `perm:user:{userId}` 缓存

**策略量**：角色数 × API 数 × 方法数 ≈ 1,000 条（可控）

### 2.3 资源级鉴权（资源抽象 + 代码内联 + 按需 enforcer）

详见 [resource-model.md](./resource-model.md)。

### 2.4 列表过滤（数据级权限）

对于列表查询接口（如"获取工单列表"），需要根据用户权限过滤数据：

```go
// Service 层调用
filter, err := registry.GetFilter(ctx, "ticket", userID, "read")
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

> **DDL 以 [phase1/06-organization.md](../phase1/06-organization.md) 与 [modules/organization.md](../modules/organization.md) 为准**（`BIGINT` 主键 + `code` 业务键，ltree path 用 code 拼接）。

### 3.1 实体组织（Phase 1）

Phase 1 仅实体组织树（`org_type` 1=公司 / 2=部门 / 3=小组）。成员关系走 `user_orgs`（用户 ↔ 组织，无组织级角色继承）。

完整建表 SQL 见 phase1 文档；此处只保留 ltree 查询示例：

```sql
-- 查询子树（含自身）
SELECT * FROM organizations WHERE path <@ 'root.tech';

-- 查询祖先链
SELECT * FROM organizations WHERE path @> 'root.tech.be';

-- 查询直接子节点
SELECT * FROM organizations WHERE parent_id = (SELECT id FROM organizations WHERE code = 'tech');
```

### 3.2 虚拟组（Phase 2b）

Phase 1 **不建**独立 `virtual_groups` 表。虚拟组在 Phase 2b 以 `organizations.org_type = 4`（`source=local`）实现，详见 [organization 模块 §2.2](../modules/organization.md#22-虚拟组与组织角色phase-2b) 与 [hr-directory-sync.md](./hr-directory-sync.md)。

### 3.3 组织角色继承（Phase 2b）

Phase 1 权限路径仅为 `user_roles`（直接角色）。Phase 2b 引入 `org_roles`（组织绑定角色，成员加入组织后自动获得）：

```sql
-- Phase 2b 草案（ID 均为 BIGINT）
CREATE TABLE org_roles (
    org_id      BIGINT NOT NULL REFERENCES organizations(id),
    role_id     BIGINT NOT NULL REFERENCES roles(id),
    PRIMARY KEY (org_id, role_id)
);
```

成员加入组织 → 自动获得该组织绑定的角色。子组织成员**不**自动继承父组织角色（向上不继承）；父组织管理员对子组织有管理权限（向下继承管理权）。Casbin BFS 三源合并见 [phase1/03-authz.md](../phase1/03-authz.md)。  
> AD / Keycloak / 若依等对照场景与删改级联策略见 [rbac-inheritance-and-cascade.md](../design/rbac-inheritance-and-cascade.md)。

---

## 4. 动态路由与前端权限

> 菜单 DDL 以 [phase1/07-menu.md](../phase1/07-menu.md) 与 [modules/menu.md](../modules/menu.md) 为准（`BIGINT` 主键 + `code`，`menu_type` 1=目录 / 2=菜单 / 3=按钮）。

### 4.1 菜单模型（借鉴旧系统三分类）

| 类型 | `menu_type` | 用途 |
|------|-------------|------|
| 目录 | 1 | 前端路由分组，可嵌套 |
| 菜单 | 2 | 可访问页面，含 `path` / `component` |
| 按钮 | 3 | 页面内操作，`permission` 权限码 |

### 4.2 菜单-API 绑定

独立表 `menu_apis`（`menu_id` + `api_path` + `api_method`），完整 DDL 见 phase1。角色绑定菜单 → 遍历 `menu_apis` → 写入 `casbin_rule`（路径 + HTTP 方法一一对应；POST 操作类路由 id 放 body，见 [07-menu.md](../phase1/07-menu.md)）。

### 4.3 前端权限数据

```
GET /api/v1/user/menus → 菜单树（目录+菜单，不含按钮）
GET /api/v1/user/permissions → 权限码列表（按钮权限码 + 路由权限码）
```

---

## 5. 审计日志

> **Phase 1**：同步写入 PostgreSQL（见 [phase1/08-audit.md](../phase1/08-audit.md)）。  
> **Phase 3**：Redis List L2 异步方案（Phase 2 不做审计异步，见 [phase2/README §1.5](../phase2/README.md)）。

### 5.1 日志分类

| 类型 | 存储 | 内容 | 保留期 |
|------|------|------|--------|
| 应用日志 | 文件（slog + Lumberjack） | 运行时调试信息 | 按文件轮转 |
| 审计日志 | PostgreSQL（Phase 1 同步；Phase 3 异步） | 用户操作记录 | 180 天（Phase 3 cron 清理） |
| 访问日志 | PostgreSQL（Phase 3+，`access_logs` 表） | HTTP 请求记录 | 90 天 |

### 5.2 审计日志写入流程

**Phase 1（同步，权威实现见 phase1/08-audit.md）**：

```
请求 → Audit 中间件
     │  c.Next() 完成后记录：user_id, method, path, status_code, duration, request_body(脱敏)
     │  → 同步 INSERT audit_logs（失败只记应用日志）
     └── 返回
```

查询：`GET /api/v1/audit/logs`。

**Phase 2 / Phase 3（异步演进，非 Phase 1）**：

```
请求 → Audit 中间件
     │  记录：user_id, action, method, path, status_code, cost, request_body(截断4KB+脱敏)
     │  → channel（缓冲 1024）或 Redis List（Phase 3 L2）
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

### 5.4 日志清理（Phase 2+）

> Phase 1 **不做**自动清理（见 [phase1/08-audit.md](../phase1/08-audit.md)）。PostgreSQL 不支持 TTL 索引，Phase 2+ 用 cron 或分区表：

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
| CORS | gin-contrib/cors，Phase 1 AllowAllOrigins（全放开）；生产白名单 |
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

### Phase 1：最小可用

| 能力 | 范围 |
|------|------|
| 认证 | 登录 + 双 Token + 限流 + 会话吊销 + 首次改密 |
| 路由级鉴权 | Casbin + 直接角色 |
| 资源级鉴权 | ResourceRegistry **空接口** |
| 用户/角色/菜单/组织 | CRUD（组织含移动节点） |
| 审计 | 同步写入 + 登录审计 |

详见 [phase1/README.md](../phase1/README.md)。

### Phase 2：业务可用（2a → 2b → 2c）

| 子阶段 | 能力 |
|--------|------|
| **2a** | ResourceRegistry + 工单 MVP（**assigned** 范围，无附件） |
| **2b** | 虚拟组/scope、ltree group 过滤、HR 同步、对象存储、多设备 UI、密码策略 |
| **2c** | 组织内委托：`owner_user_ids`、`org_member_role`、工单 Authorize（D1–D11） |

详见 [phase2/README.md](../phase2/README.md)、[04-org-delegation.md](../phase2/04-org-delegation.md)。

### Phase 3：生产加固（暂缓，按需取用）

| 能力域 | 范围 |
|--------|------|
| **生产加固类** | 可观测性、多实例、审计 L2、HA、安全增强、运维 CI |
| **事件/平台类** | Outbox+Asynq、IAM 拆分、gRPC、RS256、缓存平台、AK/SK（按需） |

> Phase 3 整体暂缓、不拆子阶段执行。详细边界见 [phase3/README.md](../phase3/README.md)。
