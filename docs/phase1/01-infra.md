# 01 - 基础设施（infra）

> Step 1，无依赖，所有模块的基座。

---

## 预期功能

| 功能 | 场景 | 说明 |
|------|------|------|
| DB 迁移脚本 | `make migrate-up` 一键建表 | golang-migrate，`.up.sql` + `.down.sql` |
| 种子数据 | 首次启动初始化 admin 用户、角色、菜单 | `ON CONFLICT DO NOTHING` 幂等 |
| 配置加载 | 读取 `config.yaml` + 环境变量覆盖 | Viper，支持 `GOMAXPROCS` 等运行时配置；**DSN/Redis 地址不写死**，见 [design-decisions §18](../design/design-decisions.md#18-部署与代码解耦一套代码多种部署) |
| Wire 依赖注入 | `make wire` 自动生成 `wire_gen.go` | 已完成 |
| 优雅关闭 | 收到 SIGTERM/SIGINT 后停止接收新请求、等待处理中请求、释放资源 | 已完成 |
| 健康检查 | `/health/live`（进程存活）+ `/health/ready`（PG+Redis 连通性） | Liveness/Readiness 探针 |
| PG / Redis 连接池 | 启动建池、Ping 探活、优雅 `Close` | 见 [§连接与连接池](#连接与连接池) |
| 目录约定 | 新代码按 `internal/{layer}/{domain}/` 落盘 | 见 [architecture §3.5](../design/architecture.md#35-领域模块目录约定单仓可拆分) |

## 核心设计思路

### 领域目录（单仓 → 可拆分）

Phase 1 仍是一个 `cmd/server`，但 **自研模块按领域分子目录**，与 [modules/README 代码路径](../modules/README.md#代码路径约定) 一致：

```
internal/service/{auth,user,role,org,menu,authz,audit}/
internal/handler/{auth,user,role,org,menu,audit}/
internal/repository/{user,role,org,menu,audit}/
```

- 骨架已有扁平文件（如 `user_service.go`）可暂留；**新增第二文件时整域迁入子目录**。
- 跨域只调 **Service 接口**，禁止 handler → 他人 Repo。
- Phase 2 工单 → 新建 `service/ticket/`，不并入 IAM 域；Phase 2b HR → `integration/hr/`。

### DB 迁移

> 详见 [proposal/data-init.md](../proposal/data-init.md)。迁移文件按序号排列，schema 与 seed 分离，所有 seed 用 `ON CONFLICT DO NOTHING` 幂等。

```
migrations/
├── 000001_init.up.sql          # 建表：users, roles（含 priority/deleted_at）, organizations, menus, menu_apis,
│                               # user_roles, user_orgs（无 role_id）, role_menus, audit_logs, casbin_rule
├── 000001_init.down.sql
├── 000002_seed.up.sql          # 种子：4 角色 + 1 admin + 3 组织 + 25 系统菜单（6 页面 + 19 按钮）
│                               # + menu_apis + superadmin/admin 角色-菜单绑定 + Casbin 初始策略（p, role::superadmin/admin, *, *）
├── 000002_seed.down.sql
└── （无 000003；Casbin 表与种子策略分别在 000001 建表、000002 插入）
```

> **DDL SSOT**：`roles.priority` / `roles.deleted_at` 见 [05-role §roles 建表](./05-role.md#roles-建表-sql)；`user_orgs` 主键 `(user_id, org_id)`、**无** `role_id` 见 [04-user §用户-组织关联](./04-user.md#用户-组织关联)。菜单种子 25 条见 [07-menu §Phase 1 菜单清单](./07-menu.md#phase-1-菜单清单ssot) 与 [data-init §4.2](../proposal/data-init.md#42-种子数据内容)。

> **仓库现状**：`migrations/` 目录 Step 1 交付物，编码前可能为空；以本文 + `data-init.md` 为准编写 SQL，勿与骨架 `internal/model` 不一致处（如 `UserOrg.role_id`、`Role.priority`）自行假设。

### 种子数据内容

> 详见 [proposal/data-init.md](../proposal/data-init.md) §4。所有 seed 用 `ON CONFLICT DO NOTHING`，永远不覆盖 `created_at`/`created_by`。

| 数据 | 内容 | 说明 |
|------|------|------|
| 系统角色 | `superadmin`（超管）、`admin`（管理员）、`operator`（操作员）、`viewer`（只读） | `is_system=true`，不可删除 |
| 组织 | `root`（集团总部）、`tech`（技术中心）、`product`（产品中心） | ltree path：`root`、`root.tech`、`root.product` |
| admin 用户 | 工号 `E000001` / 密码 `admin123`（`username=admin`） | `is_system=true`，绑定 **superadmin** 角色 + root 组织（见 [05-role §种子用户](./05-role.md#种子用户)） |
| 系统菜单 | 6 页面/目录 + 19 按钮 = **25** 条（见 07-menu） | `is_system=true`，含 `menu_apis` 绑定 |
| 角色-菜单 | `superadmin` + `admin` 绑定全部 **25** 条 IAM 菜单（**必含**用户/角色/组织三模块页面+按钮；含菜单管理以便给 operator/viewer 分配权限） | `operator`/`viewer` **零** `role_menus`（无正式业务前由 admin 按需分配） |
| Casbin 策略 | `p, role::superadmin, *, *` + `p, role::admin, *, *` | 超管+管理员通配策略 |

> **关键原则**：种子数据用 `ON CONFLICT DO NOTHING`，不用 `ON CONFLICT DO UPDATE`（会覆盖 created_at 等审计字段）。系统重启不会更新已有数据。

### Redis Lua 脚本（LoginLocker）

登录限流由 Step 3 `AuthService` 使用，脚本在 Step 1 一并落盘（`//go:embed`，无运行时读文件路径问题）：

```
internal/pkg/redis/
├── redis.go                       # 客户端连接（已有）
├── scripts.go                     # go:embed 加载脚本 + Eval 封装（Step 1/3）
└── scripts/
    └── login_lock.lua             # LoginLocker：INCR + 首次 EXPIRE 原子，15min/5 次
```

| 文件 | 职责 |
|------|------|
| `scripts/login_lock.lua` | `KEYS[1]=lock:login:{employee_no}`；`ARGV[1]=900`，`ARGV[2]=5`；返回 0/1 |
| `scripts.go` | `LoginLockIncr(ctx, employeeNo)` / `LoginLockIsBlocked(ctx, employeeNo)`（或统一 `EvalLoginLock`） |

脚本语义与调用顺序见 [02-auth.md §登录限流](./02-auth.md)。Redis `EVAL` 失败 → 503 + 10008（fail-close）。

### 连接与连接池

> 实现位置：`internal/pkg/postgres/postgres.go`、`internal/pkg/redis/redis.go`；配置 `configs/config.yaml`。拓扑差异（Cluster/Sentinel/VIP）只改 **地址与装配**，见 [design-decisions §18](../design/design-decisions.md#18-部署与代码解耦一套代码多种部署)。

#### 通用原则（PG + Redis 共用）

| 原则 | 说明 |
|------|------|
| **启动时 Ping** | 建池/Client 后立即 `Ping`；失败 **进程不启动**（fail-fast） |
| **优雅关闭** | `wire` cleanup：`pool.Close()` / `client.Close()`，顺序见 [architecture §14.1](../design/architecture.md#141-优雅关闭) |
| **敏感项走 env** | `DB_PASSWORD`、`REDIS_PASSWORD`、`JWT_SECRET`；DSN 不写进仓库 |
| **请求带 context** | Repository/Service 用 `ctx` 传递超时/取消，禁止 `context.Background()` 贯穿 HTTP 链路 |
| **Ready 探针** | `/health/ready` 对 PG/Redis 各 `Ping` 一次；**不**把驱动错误原文返回客户端 |
| **可观测** | PG DSN 建议带 `application_name=zhuzhao`（便于 `pg_stat_activity`）；Phase 3 再暴露 pool metrics |

#### PostgreSQL（`pgxpool`）

**Phase 1 必须**：使用 **`pgxpool.Pool`**（非单连接）；全进程 **共用一个 Pool** 注入各 Repository。

| 配置项 | `config.yaml` / 代码 | 建议值（Phase 1 单 App） | 说明 |
|--------|----------------------|-------------------------|------|
| `MaxConns` | `database.max_open_conns` | **25**（默认） | 单实例办公后台足够；多 App 时 `总和 < PG max_connections` |
| `MinConns` | `database.max_idle_conns` | **5** | 保持少量热连接，降低突发延迟 |
| `MaxConnLifetime` | 代码（建议后续进配置） | **1h** | 定期轮换连接；须 **小于** PG/PGBouncer 的 idle/ lifetime 限制 |
| `MaxConnIdleTime` | 代码（建议后续进配置） | **30m** | 空闲过久回收 |
| `HealthCheckPeriod` | 代码 | **1m**（pgx 默认） | 池内连接健康检查 |
| `ConnectTimeout` | DSN 或 `ParseConfig` | **5s** | 启动与重连不宜无限等 |
| `statement_timeout` | DSN `options` 或 `RuntimeParams` | **30s**（可选） | 防止慢 SQL 占满连接；Admin 导出类接口可单独放宽 |
| `sslmode` | DSN | dev **`disable`** / prod **`require`** | 生产走 TLS；与部署解耦，仅改配置 |

```yaml
# configs/config.yaml（节选）
database:
  host: localhost
  port: 5432
  user: zhuzhao
  password: zhuzhao_dev   # 生产：DB_PASSWORD
  dbname: zhuzhao
  max_open_conns: 25
  max_idle_conns: 5
  # 实现时扩展（可选进 config）：
  # conn_max_lifetime: 1h
  # conn_max_idle_time: 30m
  # connect_timeout: 5s
  # sslmode: disable          # 生产 require
  # statement_timeout: 30s
```

```go
// internal/pkg/postgres/postgres.go — 要点（与现有骨架一致，补全配置项）
poolConfig.MaxConns = int32(cfg.MaxOpenConns)
poolConfig.MinConns = int32(cfg.MaxIdleConns)
poolConfig.MaxConnLifetime = cfg.ConnMaxLifetime   // 或 time.Hour
poolConfig.MaxConnIdleTime = cfg.ConnMaxIdleTime     // 或 30 * time.Minute
// poolConfig.HealthCheckPeriod = 1 * time.Minute    // 默认即可
if err := pool.Ping(ctx); err != nil { ... }         // 启动 fail-fast
```

**Sizing 粗算**：`MaxConns ≈ min(25, 预期并发 DB 请求)`；Phase 1 无 heavy 报表，25 足够。Phase 3 多 App：`实例数 × MaxConns < PG max_connections × 0.7`。

#### Redis（`go-redis/v9`）

`redis.Client` **内置连接池**；Phase 1 须显式配置池与超时（当前骨架仅 `Addr/Password/DB`，**Step 1 补齐**）。

| 配置项 | `config.yaml`（建议新增） | 建议值（Phase 1） | 说明 |
|--------|-------------------------|-------------------|------|
| `PoolSize` | `redis.pool_size` | **10 × GOMAXPROCS** 或 **20** | 并发 Redis 命令上限 |
| `MinIdleConns` | `redis.min_idle_conns` | **5** | 热连接 |
| `PoolTimeout` | `redis.pool_timeout` | **4s** | 池满时等待；鉴权链路宜短 |
| `DialTimeout` | `redis.dial_timeout` | **5s** | 建连 |
| `ReadTimeout` | `redis.read_timeout` | **3s** | 读命令（含 `Ping`） |
| `WriteTimeout` | `redis.write_timeout` | **3s** | 写命令 |
| `MaxRetries` | `redis.max_retries` | **2** | 网络抖动；**LoginLocker 等 Lua 仍须业务幂等** |
| `ConnMaxIdleTime` | `redis.conn_max_idle_time` | **5m** | 空闲连接回收 |

```yaml
redis:
  host: localhost
  port: 6379
  db: 0
  password: ""
  pool_size: 20
  min_idle_conns: 5
  pool_timeout: 4s
  dial_timeout: 5s
  read_timeout: 3s
  write_timeout: 3s
  max_retries: 2
```

```go
client := redis.NewClient(&redis.Options{
    Addr:            cfg.Addr(),
    Password:        cfg.Password,
    DB:              cfg.DB,
    PoolSize:        cfg.PoolSize,
    MinIdleConns:    cfg.MinIdleConns,
    PoolTimeout:     cfg.PoolTimeout,
    DialTimeout:     cfg.DialTimeout,
    ReadTimeout:     cfg.ReadTimeout,
    WriteTimeout:    cfg.WriteTimeout,
    MaxRetries:      cfg.MaxRetries,
    ConnMaxIdleTime: cfg.ConnMaxIdleTime,
})
if err := client.Ping(ctx).Err(); err != nil { ... }
```

**鉴权相关**：JWT 黑名单 / `user:disabled` / RT / LoginLocker 都走 Redis；`ReadTimeout` 过大会拖慢 **503 fail-close**，不宜 > 5s。

**Phase 3**：Sentinel 时在装配层换 `redis.NewFailoverClient`（或 Cluster），**Domain 仍注入 `*redis.Client` 接口**；配置增加 `master_name`、`sentinel_addrs`。

#### Step 1 实现清单（连接层）

| 项 | 状态 |
|----|------|
| PG `pgxpool` + 启动 Ping + cleanup | ✅ 骨架已有 |
| PG lifetime/idle/ssl/timeout 可配置 | 📋 建议 Step 1 补 config |
| Redis 池参数 + 超时 | 📋 Step 1 补（当前仅 Addr） |
| Wire cleanup 顺序：HTTP → Redis → PG | 📋 与优雅关闭一并验证 |
| `/health/ready` Ping PG + Redis | 📋 已有路由，需对齐 01 语义 |
| **`router.go` 注册 org 4 条 + `POST /users/orgs`** | 📋 M1；见 [06-organization §路由注册](./06-organization.md#路由注册ssot)、[04-user §路由注册](./04-user.md#路由注册) |

### 健康检查

`/health/ready` 需要实际检查 PG 和 Redis 连通性，不能只返回 `{"status":"ok"}`：

```go
func readyHandler(db *pgxpool.Pool, rdb *redis.Client) gin.HandlerFunc {
    return func(c *gin.Context) {
        if err := db.Ping(c.Request.Context()); err != nil {
            c.JSON(503, gin.H{"status": "unhealthy", "component": "db"})
            return
        }
        if err := rdb.Ping(c.Request.Context()).Err(); err != nil {
            c.JSON(503, gin.H{"status": "unhealthy", "component": "redis"})
            return
        }
        c.JSON(200, gin.H{"status": "ok"})
    }
}
```

## 测试用例

| 用例 | 验证点 |
|------|--------|
| `migrate-up` 后表结构正确 | 查询 `information_schema.tables` 确认所有表存在 |
| 种子角色 priority | `SELECT code, priority FROM roles WHERE is_system` | 1/10/20/30 与 [data-init](../proposal/data-init.md) 一致 |
| PG 启动 Ping | 应用启动 | Pool 建连成功；PG 不可达则 fail-fast |
| Redis 启动 Ping | 应用启动 | Client Ping 成功；Redis 不可达则 fail-fast |
| `migrate-down` 后表全部删除 | 确认无残留 |
| 种子数据幂等 | 连续执行两次 `migrate-up`，admin 用户不重复 |
| 健康检查 live | 返回 200 |
| 健康检查 ready（PG 正常） | 返回 200 |
| 健康检查 ready（PG 断开） | 返回 503，`component` 字段，不含 DB/Redis 错误原文 |
| 优雅关闭 | 发送 SIGTERM，确认正在处理的请求完成后退出 |

## 涉及文件

```
migrations/
├── 000001_init.up.sql
├── 000001_init.down.sql
├── 000002_seed.up.sql
├── 000002_seed.down.sql
internal/app/app.go              # 优雅关闭（已有，需验证）
internal/router/router.go        # 健康检查路由（需完善 ready）
internal/pkg/redis/
├── redis.go
├── scripts.go                   # embed + Eval 封装（LoginLocker）
└── scripts/login_lock.lua       # 登录限流 Lua（见 02-auth §登录限流）
```

## 待决策点

> 以下决策已在讨论中确认：

- ✅ **用户 ID 类型**：`BIGINT`/`int64`，JSON 序列化为 string（使用 `json:",string"` tag，前端精度安全）。
- ✅ **组织 ID / 编码**：ID 为 `BIGINT`；业务编码 `code` 为 `VARCHAR`（ltree 路径用 code）。
- ✅ **Casbin adapter**：直接上 PG adapter（`pckhoi/casbin-pgx-adapter/v3`）。
- ✅ **组织模块范围**：Phase 1 实现完整 CRUD。
- ✅ **ID 自增策略**：
  - 用户：BIGSERIAL 自增
  - 角色：BIGSERIAL 自增（`code` 做业务标识）
  - 菜单：BIGSERIAL 自增（`code` 做业务标识）
  - 审计日志：BIGSERIAL 自增
  - 组织：BIGSERIAL 自增，系统组织种子数据显式指定 ID（1/2/3），`setval` 重置序列避免冲突
- ✅ **密钥**：`config.yaml` 只放开发示例；生产用环境变量覆盖 `JWT_SECRET`、`DATABASE_PASSWORD`。仓库不提交真实密钥。
- ✅ **健康检查**：`/health/ready` 失败时只返回 `unhealthy` + 组件名，不返回 DB/Redis 错误原文。
- ✅ **org_type 枚举**：1=公司 2=部门 3=小组 4=虚拟组（Phase 1 只用 1-3，虚拟组 Phase 2b）
- ✅ **tenant_id**：`BIGINT NOT NULL DEFAULT 1`，Phase 1 不做过滤，Phase 2 多租户时自增
- ✅ **数据访问层**：每实体独立 Repository 接口（`UserRepo`、`OrgRepo` 等），Service 层依赖接口，底层可替换存储实现
