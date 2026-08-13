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
| 认证 | [auth](./02-auth.md) | 登录、双 Token、RT 轮换、登出、黑名单 |
| 鉴权 | [authz](./03-authz.md) | 路由级 Casbin RBAC、ResourceRegistry 骨架 |
| 用户 | [user](./04-user.md) | 用户 CRUD、启用禁用、密码修改 |
| 角色 | [role](./05-role.md) | 角色 CRUD、菜单分配、Casbin 策略同步 |
| 组织 | [organization](./06-organization.md) | 组织树 CRUD、ltree 路径、用户-组织关联 |
| 菜单 | [menu](./07-menu.md) | 菜单 CRUD、菜单树、前端权限数据 |
| 审计日志 | [audit](./08-audit.md) | 操作日志中间件、同步写入、查询、应用日志规划 |
| 中间件 | [middleware](./09-middleware.md) | JWT、Casbin、CORS、Recovery、RequestID、安全头 |
| 并发与事务 | [concurrency](./10-concurrency.md) | DB 事务、SyncedEnforcer、Redis 原子操作、乐观锁 |

### 1.2 不做什么

| 不做 | 原因 | 阶段 |
|------|------|------|
| 工单模块 | Phase 1 聚焦框架，工单设计已完成待 Phase 2 | Phase 2 |
| 事件驱动 | Phase 1 用进程内 channel，不引入 Asynq/Outbox | Phase 2-3 |
| 多设备管理 | 需要 Redis 设备管理 | Phase 2 |
| 登录限流/锁定 | 需要 Redis Lua 脚本 | Phase 2 |
| 密码复杂度策略 | 基础 bcrypt 即可 | Phase 2 |
| 资源级鉴权完整实现 | Phase 1 只搭 ResourceRegistry 骨架，不实现 ltree 查询 | Phase 2 |
| AK/SK 完整管理 | Phase 1 只建表 + 中间件骨架 + 种子数据 | Phase 2 |
| 虚拟组 / 组织级权限 | Phase 1 只做实体组织树 CRUD | Phase 2 |
| 审计日志异步写入 | Phase 1 同步写入，保证不丢 | Phase 2 |
| 缓存体系 | Phase 1 无缓存，走 Casbin 内存 | Phase 2 |
| SLA / 通知 / 审批流 | 业务能力 | Phase 2-3 |
| 多实例 / 分布式锁 | Phase 1 单实例 | Phase 3 |
| Casbin Watcher | 多实例才需要 | Phase 3 |
| Metrics / 分布式追踪 | 可观测性 | Phase 3 |
| 微服务拆分 / gRPC | Phase 1 模块化单体 | Phase 3 |
| 多租户 | 预留 tenant_id 字段，不实现 | 按需 |

### 1.3 验收标准

Phase 1 完成后，以下流程能跑通：

```
1. make docker-up          # 启动 PG + Redis
2. make migrate-up         # 建表 + 种子数据
3. make dev                # 启动服务
4. curl POST /auth/login   # 登录，拿到双 Token
5. curl GET /user/menus    # 用 AT 获取菜单树
6. curl GET /user/permissions  # 获取权限码
7. curl GET /users         # 用 AT 获取用户列表（需 user:list 权限）
8. curl POST /auth/refresh # 刷新 Token
9. curl POST /auth/logout  # 登出
10. curl GET /users        # 登出后 AT 被黑名单，返回 401
```

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
   │                    └── Step 5: authz（Casbin 中间件 + ResourceRegistry 骨架）
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
| 组织编码 | `BIGINT`/`int64`，JSON 加 `,string` tag（不用 UUID，ltree 不兼容） | ✅ 已确认 |
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
| AT TTL | 30 分钟（短 AT + 长 RT） | ✅ 已确认 |

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
