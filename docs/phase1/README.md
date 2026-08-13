# Phase 1 实现计划：认证鉴权框架

> **核心目标**：为后续所有服务搭好认证鉴权框架。Phase 1 不做业务模块（工单等），聚焦于认证、鉴权、基础管理。
>
> 创建日期：2026-08-12

---

## 1. Phase 1 边界

### 1.1 做什么

| 类别 | 模块 | 核心能力 |
|------|------|---------|
| 基础设施 | [infra](./01-infra.md) | DB 迁移、配置加载、Wire DI、优雅关闭、健康检查 |
| 认证 | [auth](./02-auth.md) | 登录、双 Token、RT 轮换、登出、黑名单、登录限流、会话吊销 |
| 鉴权 | [authz](./03-authz.md) | 路由级 Casbin RBAC、ResourceRegistry 空接口 |
| 用户 | [user](./04-user.md) | 用户 CRUD、启用禁用、密码修改、超管保护 |
| 角色 | [role](./05-role.md) | 角色 CRUD、菜单分配、Casbin 策略同步 |
| 组织 | [organization](./06-organization.md) | 组织树 CRUD、ltree 路径、用户-组织关联 |
| 菜单 | [menu](./07-menu.md) | 菜单 CRUD、菜单树、前端权限数据 |
| 审计日志 | [audit](./08-audit.md) | 操作日志中间件、同步写入、登录审计 |
| 中间件 | [middleware](./09-middleware.md) | JWT（fail-close）、Casbin、CORS、Recovery、RequestID、安全头 |
| 并发与事务 | [concurrency](./10-concurrency.md) | DB 事务、SyncedEnforcer、Redis 原子操作、乐观锁 |

### 1.2 不做什么

| 不做 | 原因 | 阶段 |
|------|------|------|
| 工单模块 | Phase 1 聚焦框架 | Phase 2 |
| 数据范围过滤 | 管理接口按「全局管理员」模型，不做组织范围过滤 | Phase 2 |
| 虚拟组 / 组织级权限 | Phase 1 只做实体组织树 CRUD | Phase 2 |
| 资源级鉴权完整实现 | Phase 1 只搭 ResourceRegistry 空接口，不实现 ltree 查询 | Phase 2 |
| 多设备管理 UI / 踢出 | 允许多设备登录，不提供设备列表 | Phase 2 |
| 登录锁定（Lua） | Phase 1 用 INCR+EXPIRE 即可 | 不必 Lua |
| 密码复杂度策略 | 基础 bcrypt 即可 | Phase 2 |
| AK/SK | 无服务间调用方，不建表、不写中间件 | 有 M2M 需求时 |
| 文件存储 | 无附件场景 | Phase 2 |
| 事件驱动 / Asynq / Outbox | 无异步业务 | Phase 3 |
| 审计日志异步写入 | Phase 1 同步写入，保证不丢 | Phase 3 |
| 缓存体系 | Phase 1 无缓存，走 Casbin 内存 | 工单跑通后按需 |
| JWT RS256 / JWKS | 单体无收益，拆服务时再换 | Phase 3 |
| 每资源独立 Enforcer | 简单资源用代码内联 | 策略需可配置时 |
| IAM 独立部署 / gRPC | Phase 1–2 模块化单体 | Phase 3 |
| 多实例 / 分布式锁 / Watcher | Phase 1 单实例 | Phase 3 |
| Metrics / 分布式追踪 | 可观测性 | Phase 3 |
| 多租户 | 预留 tenant_id 字段，不实现 | 按需 |

### 1.3 验收标准

Phase 1 完成后，以下流程能跑通：

```
主路径
1. make docker-up / migrate-up / make dev
2. POST /auth/login              # 拿到双 Token
3. GET  /user/menus              # 菜单树
4. GET  /user/permissions        # 权限码
5. GET  /users                   # 需路由级权限
6. POST /auth/refresh            # 轮换 Token
7. POST /auth/logout             # 登出
8. GET  /users                   # 登出后 401

对抗路径（必须覆盖，不是可选）
9.  错误密码 / 不存在用户          # 均返回同一文案，防枚举
10. 连续登录失败                   # 触发限流 429
11. 禁用用户后带旧 AT 访问         # 401/403（会话吊销）
12. 删除/禁用最后一个 superadmin   # 拒绝
13. admin 重置 superadmin 密码     # 403
14. 首次登录改密期间访问其它 API   # 403 PASSWORD_CHANGE_REQUIRED
15. 无角色用户访问鉴权路由         # 403
16. 并发两次 refresh               # 只有一次成功
17. Redis 不可用时访问鉴权路由     # 503（fail-close）
```

### 1.4 已知限制（验收时不要误判为已实现）

| 限制 | 说明 |
|------|------|
| 无数据范围过滤 | 拥有 `GET /users` 权限的角色能看到**全部**用户，不做部门隔离 |
| 无虚拟组 | 只有实体组织树 |
| AT 存哪 | 前端自行存 AT/RT（建议内存 + 刷新），后端不设 Cookie |
| AK/SK 不可用 | 没有服务间认证 |
| 管理接口是全局模型 | `admin`/`superadmin` 路由级 bypass；`operator`/`viewer` 靠菜单策略，仍无行级过滤 |

---

## 2. 实施顺序

按依赖关系排列，每个 Step 是一个可独立验证的交付物：

```
Step 1: infra（DB 迁移 + 种子数据 + 配置 + Wire）
   │
   ├── Step 2: user repo（用户数据访问）
   │      │
   │      └── Step 3: auth（登录 + 双 Token + 登出）
   │             │
   │             └── Step 4: middleware（JWT 中间件 + 黑名单）
   │                    │
   │                    └── Step 5: authz（Casbin 中间件 + ResourceRegistry 空接口）
   │                           │
   │                           ├── Step 6: user service/handler（用户 CRUD）
   │                           ├── Step 7: role service/handler（角色 + 菜单分配）
   │                           └── Step 8: menu service/handler（菜单树 + 权限码）
   │                                  │
   │                                  └── Step 9: organization（组织树 CRUD）
   │                                         │
   │                                         └── Step 10: audit（操作日志中间件）
   │                                                │
   │                                                └── Step 11: 集成验收
```

| Step | 模块 | 依赖 | 文档 |
|------|------|------|------|
| 1 | infra | 无 | [01-infra.md](./01-infra.md) |
| 2 | user repo | Step 1 | [04-user.md](./04-user.md) |
| 3 | auth | Step 2 | [02-auth.md](./02-auth.md) |
| 4 | middleware | Step 3 | [09-middleware.md](./09-middleware.md) |
| 5 | authz | Step 4 | [03-authz.md](./03-authz.md) |
| 6 | user | Step 5 | [04-user.md](./04-user.md) |
| 7 | role | Step 5 | [05-role.md](./05-role.md) |
| 8 | menu | Step 7 | [07-menu.md](./07-menu.md) |
| 9 | organization | Step 5 | [06-organization.md](./06-organization.md) |
| 10 | audit | Step 4 | [08-audit.md](./08-audit.md) |
| 11 | 集成验收 | All | 本文档 §1.3 |

---

## 3. 测试策略

### 3.1 核心原则：测试先行

每个模块的实现必须遵循"先写测试，再写实现"的节奏：

1. **接口定义先行** — 先定义 Service/Repository 接口方法签名
2. **测试用例先行** — 根据接口契约编写测试用例（含正常、边界、异常场景）
3. **实现代码** — 编写实现使测试通过
4. **重构** — 测试通过后重构代码，确保测试仍然通过

### 3.2 测试分层

| 层级 | 范围 | 工具 | 运行 |
|------|------|------|------|
| 单元测试 | Service 层（Mock Repository） | testing + testify + uber-go/mock | `make test-unit` |
| 集成测试 | Repository 层（testcontainers PG） | testcontainers-go + pgx | `make test-integration` |
| 单元测试 | Middleware（Mock JWT + Redis） | httptest + testify | `make test-unit` |
| 集成测试 | 端到端 API | httptest + testcontainers | CI |
| 基准测试 | 关键路径（JWT 解析、Casbin Enforce） | testing.B | 按需 |

### 3.3 Mock 策略

- **Service 层**：Mock Repository 接口（uber-go/mock 生成）
- **Handler 层**：Mock Service 接口 + 真实 Gin 路由（httptest）
- **Repository 层**：不 Mock，用 testcontainers 启动真实 PG
- **Middleware**：Mock JWT Manager + Redis Client

---

## 4. 待决策点

> 以下决策已在讨论中确认：

| 事项 | 决策 | 状态 |
|------|------|------|
| 用户 ID 类型 | `BIGINT`/`int64`，JSON 加 `,string` tag | ✅ 已确认 |
| 组织 ID / 编码 | ID 为 `BIGINT`/`int64`（JSON `,string`）；业务编码 `code` 为 `VARCHAR`（ltree 路径用 code，只能字母数字下划线） | ✅ 已确认 |
| Casbin adapter | 直接上 PG adapter（`pckhoi/casbin-pgx-adapter/v3`） | ✅ 已确认 |
| 密码策略 | 仅 bcrypt cost=12，不增加复杂度校验 | ✅ 已确认 |
| 组织模块范围 | Phase 1 实现完整 CRUD | ✅ 已确认 |
| 审计日志写入方式 | Phase 1 同步写入（见下方分析） | ✅ 已确认 |
| 应用日志 | slog 同步写入文件，不异步（见下方分析） | ✅ 已确认 |
| ID 自增策略 | 用户/角色/菜单/日志/凭证 BIGSERIAL；组织 BIGSERIAL + 种子显式 ID | ✅ 已确认 |
| org_type 枚举 | 1=公司 2=部门 3=小组 4=虚拟组 | ✅ 已确认 |
| tenant_id | BIGINT DEFAULT 1，Phase 1 不过滤 | ✅ 已确认 |
| 数据访问层 | 每实体独立 Repository 接口 | ✅ 已确认 |
| superadmin 角色 | 新增，4 个系统角色 | ✅ 已确认 |
| 登录限流 | Phase 1 用 Redis INCR+EXPIRE，不引入 Lua | ✅ 已确认 |
| 会话吊销 | 禁用/删除用户写 `user:disabled:{id}`，JWT 中间件检查 | ✅ 已确认 |
| Redis 故障 | 鉴权链路 fail-close，返回 503 | ✅ 已确认 |
| AK/SK | Phase 1 不做 | ✅ 已确认 |
| 数据范围 | Phase 1 不做组织范围过滤 | ✅ 已确认 |

---

## 5. 文档索引

| 文档 | 模块 | 核心内容 |
|------|------|---------|
| [01-infra.md](./01-infra.md) | 基础设施 | DB 迁移、配置、Wire、优雅关闭 |
| [02-auth.md](./02-auth.md) | 认证 | 登录、双 Token、RT 轮换、登出 |
| [03-authz.md](./03-authz.md) | 鉴权 | Casbin RBAC、ResourceRegistry |
| [04-user.md](./04-user.md) | 用户 | CRUD、密码、角色绑定 |
| [05-role.md](./05-role.md) | 角色 | CRUD、菜单分配、策略同步 |
| [06-organization.md](./06-organization.md) | 组织 | 树形 CRUD、ltree、用户关联 |
| [07-menu.md](./07-menu.md) | 菜单 | CRUD、菜单树、权限码 |
| [08-audit.md](./08-audit.md) | 审计日志 | 操作日志中间件、同步写入、应用日志规划 |
| [09-middleware.md](./09-middleware.md) | 中间件 | JWT、Casbin、CORS、安全头 |
| [10-concurrency.md](./10-concurrency.md) | 并发与事务 | DB 事务、SyncedEnforcer、Redis 原子操作 |
