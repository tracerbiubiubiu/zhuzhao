# 01 - 基础设施（infra）

> Step 1，无依赖，所有模块的基座。

---

## 预期功能

| 功能 | 场景 | 说明 |
|------|------|------|
| DB 迁移脚本 | `make migrate-up` 一键建表 | golang-migrate，`.up.sql` + `.down.sql` |
| 种子数据 | 首次启动初始化 admin 用户、角色、菜单 | `ON CONFLICT DO NOTHING` 幂等 |
| 配置加载 | 读取 `config.yaml` + 环境变量覆盖 | Viper，支持 `GOMAXPROCS` 等运行时配置 |
| Wire 依赖注入 | `make wire` 自动生成 `wire_gen.go` | 已完成 |
| 优雅关闭 | 收到 SIGTERM/SIGINT 后停止接收新请求、等待处理中请求、释放资源 | 已完成 |
| 健康检查 | `/health/live`（进程存活）+ `/health/ready`（PG+Redis 连通性） | Liveness/Readiness 探针 |

## 核心设计思路

### DB 迁移

> 详见 [proposal/data-init.md](../proposal/data-init.md)。迁移文件按序号排列，schema 与 seed 分离，所有 seed 用 `ON CONFLICT DO NOTHING` 幂等。

```
migrations/
├── 000001_init.up.sql          # 建表：users, roles, organizations, menus, menu_apis, user_roles, user_orgs, role_menus, audit_logs, api_credentials
├── 000001_init.down.sql
├── 000002_seed.up.sql          # 种子数据：3 角色 + 1 admin 用户 + 3 组织 + 6 菜单 + 菜单-API 绑定 + 角色-菜单绑定
├── 000002_seed.down.sql
└── 000003_casbin.up.sql        # Casbin 策略表（casbin_rule）+ admin 通配策略
```

### 种子数据内容

> 详见 [proposal/data-init.md](../proposal/data-init.md) §4。所有 seed 用 `ON CONFLICT DO NOTHING`，永远不覆盖 `created_at`/`created_by`。

| 数据 | 内容 | 说明 |
|------|------|------|
| 系统角色 | `superadmin`（超管）、`admin`（管理员）、`operator`（操作员）、`viewer`（只读） | `is_system=true`，不可删除 |
| 组织 | `root`（集团总部）、`tech`（技术中心）、`product`（产品中心） | ltree path：`root`、`root.tech`、`root.product` |
| admin 用户 | `admin` / `admin123`（bcrypt hash） | `is_system=true`，绑定 admin 角色 + root 组织 |
| 系统菜单 | 首页目录 + 系统管理目录（含用户/角色/菜单/组织管理 4 个子菜单） | `is_system=true`，含 `menu_apis` 绑定 |
| 角色-菜单 | admin 绑定所有菜单 | 全量绑定 |
| Casbin 策略 | `p, role::superadmin, *, *` + `p, role::admin, *, *` | 超管+管理员通配策略 |

> **关键原则**：种子数据用 `ON CONFLICT DO NOTHING`，不用 `ON CONFLICT DO UPDATE`（会覆盖 created_at 等审计字段）。系统重启不会更新已有数据。

### 健康检查

`/health/ready` 需要实际检查 PG 和 Redis 连通性，不能只返回 `{"status":"ok"}`：

```go
func readyHandler(db *pgxpool.Pool, rdb *redis.Client) gin.HandlerFunc {
    return func(c *gin.Context) {
        if err := db.Ping(c.Request.Context()); err != nil {
            c.JSON(503, gin.H{"status": "unhealthy", "db": err.Error()})
            return
        }
        if err := rdb.Ping(c.Request.Context()).Err(); err != nil {
            c.JSON(503, gin.H{"status": "unhealthy", "redis": err.Error()})
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
| `migrate-down` 后表全部删除 | 确认无残留 |
| 种子数据幂等 | 连续执行两次 `migrate-up`，admin 用户不重复 |
| 健康检查 live | 返回 200 |
| 健康检查 ready（PG 正常） | 返回 200 |
| 健康检查 ready（PG 断开） | 返回 503 |
| 优雅关闭 | 发送 SIGTERM，确认正在处理的请求完成后退出 |

## 涉及文件

```
migrations/
├── 000001_init.up.sql
├── 000001_init.down.sql
├── 000002_seed.up.sql
├── 000002_seed.down.sql
└── 000003_casbin.up.sql
internal/app/app.go              # 优雅关闭（已有，需验证）
internal/router/router.go        # 健康检查路由（需完善 ready）
```

## 待决策点

> 以下决策已在讨论中确认：

- ✅ **用户 ID 类型**：`BIGINT`/`int64`，JSON 序列化为 string（使用 `json:",string"` tag，前端精度安全）。
- ✅ **组织编码**：`BIGINT`/`int64`，JSON 序列化为 string。不用 UUID（ltree 路径不兼容）。
- ✅ **Casbin adapter**：直接上 PG adapter（`pckhoi/casbin-pgx-adapter/v3`）。
- ✅ **组织模块范围**：Phase 1 实现完整 CRUD。
- ✅ **ID 自增策略**：
  - 用户：BIGSERIAL 自增
  - 角色：BIGSERIAL 自增（`code` 做业务标识）
  - 菜单：BIGSERIAL 自增（`code` 做业务标识）
  - 审计日志：BIGSERIAL 自增
  - 组织：BIGSERIAL 自增，系统组织种子数据显式指定 ID（1/2/3），`setval` 重置序列避免冲突
  - api_credentials：BIGSERIAL 自增（AK 做业务标识）
- ✅ **org_type 枚举**：1=公司 2=部门 3=小组 4=虚拟组（Phase 1 只用 1-3，虚拟组 Phase 2）
- ✅ **tenant_id**：`BIGINT NOT NULL DEFAULT 1`，Phase 1 不做过滤，Phase 2 多租户时自增
- ✅ **数据访问层**：每实体独立 Repository 接口（`UserRepo`、`OrgRepo` 等），Service 层依赖接口，底层可替换存储实现
