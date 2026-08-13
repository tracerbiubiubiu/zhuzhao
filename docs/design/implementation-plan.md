# 实施计划

> **已废弃（DEPRECATED）**：本文档为 2026-08-12 初版实施记录，**不再作为执行依据**。
> 请以 [`phase1/README.md`](../phase1/README.md)、[`roadmap.md`](../roadmap.md) 为准。
> 下文「当前状态」仅作历史快照，可能已过时。
>
> 更新时间：2026-08-12  
> Phase 1 核心目标：**为后续所有服务搭好认证鉴权框架**。工单等业务模块 Phase 2 开始。

---

## Phase 1 定位

Phase 1 不做业务模块（工单、审批等），核心目标是：

1. **认证框架**：双 JWT 登录、RT 轮换、多设备管理、登出黑名单
2. **鉴权框架**：路由级 Casbin RBAC + ResourceRegistry **空接口**（Phase 2 再实现资源级）
3. **基础管理**：用户/角色/组织/菜单的 CRUD + 动态路由（前端菜单树+权限码）
4. **基础设施**：PG 迁移、Redis 连接、Wire DI、优雅关闭、审计日志

工单模块设计已完成（见 `modules/ticket.md`），Phase 2 开始实现。

---

## 当前状态

### 已完成

| 模块 | 状态 | 说明 |
|------|------|------|
| 项目骨架 | ✅ | 目录结构、go.mod、Makefile |
| 配置加载 | ✅ | Viper 读取 config.yaml + 环境变量覆盖 |
| Wire 依赖注入 | ✅ | wire.go + wire_gen.go，编译通过 |
| 应用生命周期 | ✅ | 启动、优雅关闭、signal 处理 |
| 基础设施连接 | ✅ | PostgreSQL 连接池、Redis 客户端、Casbin enforcer |
| 路由注册 | ✅ | 所有 API 路由已定义，分组清晰 |
| 中间件骨架 | ✅ | Recovery、Logger、JWT、Casbin（stubs） |
| Handler 骨架 | ✅ | Auth/User/Role/Org/Menu Handler（stubs） |
| Service 骨架 | ✅ | 7 个 Service（stubs） |
| Repository 骨架 | ✅ | 5 个 Repo（stubs） |
| Model 定义 | ✅ | User/Role/Org/Menu/AuditLog/Token |
| 通用工具包 | ✅ | logger、jwt、crypto、redis、postgres、response、errcode |
| Docker Compose | ✅ | PostgreSQL 15 + Redis 6.2 |
| 健康检查 | ✅ | /health/live + /health/ready 路由 |

### 未完成

| 模块 | 状态 | 说明 |
|------|------|------|
| 数据库迁移 | ❌ | migrations/ 目录为空 |
| Repository 实现 | ❌ | 全部返回 "not implemented" |
| Service 实现 | ❌ | 全部返回 "not implemented" |
| JWT 中间件黑名单 | ❌ | TODO 标记 |
| Casbin 策略加载 | ❌ | 内存 enforcer，无策略 |
| Docker 镜像 | ❌ | 未拉取 |

---

## 测试策略

### 核心原则：测试先行

每个模块的实现必须遵循"先写测试，再写实现"的节奏：

1. **接口定义先行**——先定义 Service/Repository 接口方法签名
2. **测试用例先行**——根据接口契约编写测试用例（含正常、边界、异常场景）
3. **实现代码**——编写实现使测试通过
4. **重构**——测试通过后重构代码，确保测试仍然通过

### 测试分层

| 层级 | 范围 | 工具 | 运行时机 |
|------|------|------|---------|
| 单元测试 | Service 层业务逻辑（Mock Repository） | `testing` + `testify` + `uber-go/mock` | 每次 `make test` |
| 单元测试 | Repository 层 SQL（testcontainers PG） | `testcontainers-go` + `pgx` | 每次 `make test-integration` |
| 单元测试 | Middleware（Mock JWT Manager + Redis） | `httptest` + `testify` | 每次 `make test` |
| 集成测试 | 端到端 API（真实 PG + Redis 容器） | `httptest` + `testcontainers-go` | CI 流水线 |
| 基准测试 | 关键路径性能（JWT 解析、Casbin Enforce） | `testing.B` | 按需 |

### 测试目录结构

```
internal/
├── service/
│   ├── auth_service.go
│   ├── auth_service_test.go          # 单元测试（Mock Repo）
│   └── ...
├── repository/
│   ├── user_repo.go
│   ├── user_repo_test.go             # 集成测试（testcontainers PG）
│   └── ...
├── middleware/
│   ├── jwt.go
│   ├── jwt_test.go                   # 单元测试（Mock JWT + Redis）
│   └── ...
├── handler/
│   ├── auth_handler.go
│   ├── auth_handler_test.go          # 集成测试（httptest + 真实路由）
│   └── ...
└── testutil/
    ├── testdb.go                     # testcontainers PG helper
    ├── testredis.go                  # testcontainers Redis helper
    └── fixture.go                    # 测试数据工厂
```

### Mock 策略

- **Service 层测试**：Mock Repository 接口（`uber-go/mock` 生成 mock）
- **Handler 层测试**：Mock Service 接口 + 真实 Gin 路由（`httptest`）
- **Repository 层测试**：不 Mock，用 testcontainers 启动真实 PG（避免 SQL 行为与 mock 不一致）
- **Middleware 测试**：Mock JWT Manager + Redis Client

### 每个 Step 的测试要求

| Step | 测试先行要求 |
|------|------------|
| Step 1（DB 迁移） | 迁移脚本 up/down 幂等性验证 |
| Step 2（Repository） | 先写 `*_repo_test.go`：CRUD 正常、软删除、唯一约束冲突 |
| Step 3（认证核心） | 先写 `auth_service_test.go`：登录成功/密码错误/用户禁用/RT 轮换/RT 过期 |
| Step 4（JWT 中间件） | 先写 `jwt_test.go`：无 Token/过期 Token/黑名单 Token/有效 Token |
| Step 5（基础 CRUD） | 每个 Service 先写测试再写实现 |
| Step 6（动态路由） | 先写菜单树构建测试：空树/多层嵌套/权限码过滤 |
| Step 7（Casbin） | 先写鉴权测试：admin bypass/有权限/无权限/通配符匹配 |

---

## 实施步骤

### Step 1：数据库迁移

**目标**：`make docker-up && make migrate-up` 后数据库就绪

**产出文件**：
- `migrations/000001_init.up.sql` — 建表（来自架构文档第 10 章）
- `migrations/000001_init.down.sql` — 回滚
- `migrations/000002_seed.up.sql` — 种子数据（角色、组织、admin 用户、初始菜单）
- `migrations/000002_seed.down.sql` — 回滚

**完成标准**：迁移执行无报错，表结构正确

---

### Step 2：Repository 层

**目标**：用真实 SQL 替换 "not implemented"

**涉及文件**：
- `internal/repository/user_repo.go` — 查询、创建、更新、软删除
- `internal/repository/role_repo.go` — CRUD
- `internal/repository/menu_repo.go` — CRUD + 按角色查询菜单
- `internal/repository/org_repo.go` — CRUD（Phase 2 细化）
- `internal/repository/audit_log_repo.go` — 插入 + 查询（Phase 2 细化）

**完成标准**：每个方法能正确执行 SQL，返回 model 结构体

---

### Step 3：认证核心（登录 + Token）

**目标**：`POST /api/v1/auth/login` 能返回双 Token

**涉及文件**：
- `internal/service/auth_service.go` — Login / Refresh / Logout
- `internal/handler/auth_handler.go` — Login / Refresh / Logout 接口

**实现内容**：
- 密码校验（bcrypt）
- 签发 AT + RT
- RT 存 Redis（支持多设备）
- 登出时 AT 黑名单 + RT 删除

**完成标准**：用 curl 能登录、刷新、登出

---

### Step 4：JWT 中间件完善

**目标**：认证路由能正确校验 Token

**涉及文件**：
- `internal/middleware/jwt.go` — 补充 Redis 黑名单检查

**完成标准**：无 Token 或黑名单 Token 被拒绝，有效 Token 注入上下文

---

### Step 5：基础 CRUD

**目标**：用户/角色/菜单管理接口可用

**涉及文件**：
- `internal/service/user_service.go` + `internal/handler/user_handler.go`
- `internal/service/rbac_service.go` + `internal/handler/role_handler.go`
- `internal/service/menu_service.go` + `internal/handler/menu_handler.go`

**完成标准**：CRUD 接口能正常操作数据库

---

### Step 6：动态路由

**目标**：`GET /user/menus` 返回菜单树，`GET /user/permissions` 返回权限码

**涉及文件**：
- `internal/service/menu_service.go` — 菜单树构建 + 权限码查询

**完成标准**：登录后能获取自己的菜单和权限

---

### Step 7：Casbin 接入

**目标**：路由级 RBAC 生效

**涉及文件**：
- `internal/casbin/enforcer.go` — 从 DB 加载策略
- `internal/middleware/casbin.go` — 中间件挂载到需鉴权路由
- `internal/router/router.go` — 调整中间件挂载

**完成标准**：admin 角色全通，其他角色按策略放行/拒绝

---

## 验收标准

Phase 1 完成后，以下流程能跑通：

```
1. make docker-up          # 启动 PG + Redis
2. make migrate-up         # 建表 + 种子数据
3. make dev                # 启动服务
4. curl POST /auth/login   # 登录，拿到双 Token
5. curl GET /user/menus    # 用 AT 获取菜单树
6. curl GET /users         # 用 AT 获取用户列表
7. curl POST /auth/refresh # 刷新 Token
8. curl POST /auth/logout  # 登出
```
