# 实施计划

> 更新时间：2026-08-10  
> 目标：把框架搭起来，跑通核心链路，不纠结细节

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
