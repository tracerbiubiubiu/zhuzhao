# 现有系统 vs 新框架对比分析

> 基于 `doc/module-assessment-2026-08/` 42 份文档的审计结果，逐项对比现有 zhuzhao 系统与新框架设计，记录结论和决策。
>
> 审计日期：2026-08-11
> 现有系统版本：zhuzhao 0.1.0（MongoDB + Casbin + RSA JWT）
> 新框架：docs/design/architecture.md（PostgreSQL + Casbin + 无状态 JWT）

## 对比维度索引

> 以下按关联性分组，综合讨论时按组进行，而非逐项独立讨论。

### A 组：存储与鉴权（强关联，需综合讨论）

| # | 主题 | 关联点 | 状态 |
|---|------|--------|------|
| 1 | 数据库选型：MongoDB vs PostgreSQL | 影响 3/6/8/9 | ✅ 已决策（PostgreSQL，详见 design-decisions.md §11） |
| 3 | 资源级鉴权：Restrict 9 种 ConditionType vs ltree+内联 | 依赖 1（嵌套文档 vs 关系表） | ✅ 已决策（借鉴语义，分阶段实现） |
| 4 | Casbin 模型：g 表消除 + BFS 展开 vs 基础 RBAC | 依赖 1（Casbin adapter） | ✅ Phase 1 直接角色；Phase 2b BFS 三源 |
| 6 | 事务策略：MongoDB Transact vs PostgreSQL 事务 | 依赖 1（事务机制完全不同） | ✅ PG 原生事务；Casbin 同步事务外 |
| 8 | 级联删除与一致性 | 依赖 1/6（外键 vs 手动） | ✅ 映射表 CASCADE + 业务实体手动事务 |
| 9 | 组织架构设计 | 依赖 1（ltree vs BFS 遍历） | ✅ ltree + 同表虚拟组（org_type=4，Phase 2b） |

### B 组：认证与安全（可独立讨论）

| # | 主题 | 关联点 | 状态 |
|---|------|--------|------|
| 2 | JWT 策略：RSA 双 token vs 无状态 JWT | 与权限缓存方案相关 | ✅ HS256（Phase 1–2）；RS256（Phase 3b）；权限缓存 Phase 3 按需 |
| 7 | 登录安全：LoginLocker 设计 | 可直接借鉴 | ✅ Phase 1 **Lua** + fail-close |

### C 组：运维与工程化（可独立讨论）

| # | 主题 | 关联点 | 状态 |
|---|------|--------|------|
| 5 | 日志：zap + MongoWriteSyncer vs slog + Lumberjack | 依赖 1（日志存储） | ✅ slog + Lumberjack；审计 Phase 1 同步写 DB |
| 10 | 动态路由与菜单权限 | 可直接借鉴 | ✅ Phase 1 `menu_apis` + Casbin；swagger 同步 Phase 2 |
| 11 | 多二进制入口设计 | 工程化借鉴 | ⏳ 待讨论 |
| 12 | 配置管理：viper 环境变量化 | 工程化借鉴 | ⏳ 待讨论 |
| 13 | 测试体系 | 工程化借鉴 | ⏳ 待讨论 |

---

## 现有系统架构摘要

### 技术栈

| 维度 | 现有系统 | 新框架 |
|------|---------|--------|
| 语言 | Go 1.26.4 | Go（同） |
| HTTP | Gin v1.12 | Gin（同） |
| 数据库 | MongoDB 7.0+（单节点副本集 rs0） | PostgreSQL 15 |
| 缓存 | Redis 7.0+ | Redis 6.2 |
| 鉴权 | Casbin v2.135 + 自研 Restrict | Casbin + ltree 组织关系查询 + 代码内联 |
| JWT | golang-jwt/v5 + RSA 4096 | golang-jwt/v5 + HS256（Phase 1–2）→ RS256（Phase 3b） |
| DI | google/wire v0.7 | google/wire（同） |
| 日志 | zap + MongoWriteSyncer | slog + Lumberjack |
| 配置 | viper + ${VAR:-default} | viper（同） |
| 外部库 | zhuzhao-utils v0.2.0 | 无（暂内聚） |

### 两层权限模型

```
现有系统：
请求 → Authenticate（JWT 验证，存 uid）
     → Authorize（BFS 三源角色 → Casbin enforce 路由级）
     → Handler → restrictSvc.Authorize（9 种 ConditionType 资源级）

新框架：
请求 → JWT 中间件（验证 + 权限缓存查询）
     → Casbin 中间件（路由级 RBAC）
     → Handler → 资源级 ltree 组织关系查询 + ABAC（属主判断）
```

### 现有系统的成熟设计（值得借鉴）

1. **Casbin g 表消除**：角色继承不写 g 表，改 BFS 展开后用 `r.sub == p.sub` 匹配，简化模型
2. **expanded_roles context 复用**：Authorize 中间件 BFS 展开后存入 gin context，handler 直接取用，避免双倍查询
3. **文档优先策略后同步**：MongoDB 是 source of truth，Casbin 是 derived cache，同步失败不阻塞业务
4. **Transact + txnMarkerKey**：解决 sync.Mutex 不可重入问题，事务内写操作跳过 mutex
5. **LoginLocker Lua 原子脚本**：INCR + EXPIRE + SET 锁定标记原子完成，fail-close 策略
6. **Restrict 列表过滤生成**：将 condition 转为 MongoDB filter，实现列表查询的数据级权限
7. **多二进制入口**：server/init/rebuild/dedup/sync-apis 分离运维和运行时职责

### 现有系统的已知问题（新框架需规避）

1. **SetUserRoles 无事务**：delete-all → insert-all 无事务保护
2. **token 集合无 TTL 索引**：过期 token 文档累积
3. **restrict grants 缓存无 TTL**：多实例不一致
4. **DeleteUser 无法即时吊销 atoken**：Redis `RemoveByUid` 是 no-op，atoken 靠 TTL 自然失效
5. **Log() 接口泄漏**：7 个服务暴露 zap logger 到接口
6. **Casbin LoadPolicy 全量重载**：每次写操作后全量从 MongoDB 重载，策略量大时低效
7. **mutex 持有整个事务周期**：高并发写场景瓶颈
8. **无角色缓存**：每次请求 BFS 查 MongoDB 3 个集合

---

## 逐项讨论记录

> 以下按讨论顺序追加，每项包含：现有设计 → 新框架设计 → 差异分析 → 结论。

---

## 待讨论点详细清单

### #1 数据库选型：MongoDB vs PostgreSQL

**现有系统设计**：
- MongoDB 7.0+ 单节点副本集 rs0（必须副本集才能用事务）
- 10+ 个 database 分库（user/role/menu/organization/api/restrict/casbin/authenticator/sequencer/ctem）
- 日志单独一个 MongoDB 实例（17017 端口）
- 嵌套文档：restrict.grants = `map[resource][]Grant` 直接存一个文档
- 灵活 schema：ctem.data 存 `map[string]interface{}`
- TTL 索引自动清理日志
- FindOneAndUpdate 原子自增 ID（sequencer）
- 索引在 repository 构造函数中自动创建（幂等）

**新框架设计**：
- PostgreSQL 15
- 单库多表 + 外键约束
- golang-migrate 管理 schema 版本
- ltree 扩展处理组织树层级
- JSONB 处理灵活字段（如果需要）

**需要讨论的交叉点**：
- restrict.grants 嵌套文档结构在 PostgreSQL 中怎么存？JSONB 还是拆表？
- 日志 TTL 自动清理在 PostgreSQL 中怎么实现？分区表？定期 cron？
- Casbin adapter 从 MongoDB 换 PostgreSQL，行为是否一致？
- sequencer 自增 ID 是否直接用 PostgreSQL SERIAL/IDENTITY 替代？
- 索引管理从"构造函数自动创建"改为"migration 手动管理"，对开发流程的影响？

---

### #2 JWT 策略：RSA 双 token vs 无状态 JWT

**现有系统设计**：
- RSA 4096 非对称签名（公钥验签，私钥签发，密钥存文件系统 `build/key/jwt/*.pem`）
- atoken 5min + rtoken 7d
- rtoken 存 MongoDB `authenticator.token` 集合（session_id 唯一，支持多设备）
- rtoken 轮换：FindOneAndUpdate CAS 原子操作防重放
- atoken 黑名单存 Redis（key = `revoke:token:{sha256(atoken)}`，TTL = atoken 剩余有效期）
- Logout：删 MongoDB rtoken + 加 Redis 黑名单
- DeleteUser：仅删 MongoDB rtoken（Redis `RemoveByUid` 是 no-op），atoken 靠 TTL 自然失效
- JWT payload 含 uid（仅用户 ID，不含角色/权限——这点和新框架一致）

**新框架设计**（早期草案；**Phase 1 以 [`roadmap.md`](../roadmap.md) 为准**）：
- HS256 对称签名（secret 存环境变量）
- AT **30min** + RT 7d（早期草案曾写 2h；Phase 1 无权限缓存，30min 平衡安全与体验）
- RT 存 Redis（`refresh:{userId}:{deviceId}`）
- RT 轮换：Redis `GETDEL` 原子替换
- AT 黑名单存 Redis
- JWT 仅存 uid + username + jti + mcp + exp（无状态策略）
- 权限缓存 `perm:user:{userId}`：**Phase 3**，Phase 1 路由鉴权查 `user_roles` + Casbin

**需要讨论的点**：
- RSA vs HS256：现有系统用 RSA 4096，密钥管理更重但安全性更高（公钥可分发给其他服务验签）。新框架 Phase 1 用 HS256；Phase 3b 微服务化可改 RS256。
- AT 有效期：现有 5min（极短）vs Phase 1 **30min**。Phase 1 权限不入 JWT、无 Redis 权限缓存，30min 为已定方案。
- rtoken 存储：MongoDB（持久化）vs Redis（内存）。现有系统用 MongoDB 持久化 rtoken，Redis 重启不丢；新框架用 Redis，重启可能丢（需要 AOF 持久化）。
- DeleteUser 吊销问题：现有系统的已知 bug（`RemoveByUid` no-op），新框架怎么解决？方案：DeleteUser 时遍历该用户所有 AT 加入黑名单，或者用 `user:disabled:{userId}` 标记 + JWT 中间件检查。

---

### #3 资源级鉴权：Restrict 9 种 ConditionType vs ltree+代码内联

**现有系统设计**：
- 自研 Restrict 引擎，9 种 ConditionType：
  - `owner` — CurrentUserId == OwnerId
  - `self` — CurrentUserId == ResourceId
  - `super_admin` — ROLE_ADMIN in Roles
  - `org_admin` — 资源拥有者所属组织（含祖先）的管理员
  - `org_admin_of` — 资源本身（组织）的管理员
  - `org_member` — 同一组织
  - `org_subtree_member` — 资源拥有者在当前用户所属组织（含后代）中
  - `org_in_subtree` — 资源拥有者在当前用户所属组织子树中
  - `checked_orgs` — 资源拥有者在指定组织中
- 策略按角色存储（`restrict_policies._id = roleCode`），`grants = map[resource][]Grant`
- Grant = `{Action, Conditions[]}`，多角色 OR + 条件 AND
- 启动时全量加载 grants 到内存缓存
- 4 个 evaluator 持有 LRU 缓存（org 相关查询，256 容量 5min TTL）
- `GetFilterForResource`：将 conditions 转为 MongoDB filter，实现列表查询行级过滤
- evaluator 通过 DI 注入（开闭原则）

**新框架设计**：
- 三层鉴权：RBAC（路由级 Casbin）+ ltree 组织关系查询（资源级）+ ABAC（属主判断）
- ltree 组织关系：PostgreSQL 原生树形查询，一条 SQL 判断层级关系
- ABAC：代码内联 `owner_id == userId`

**需要讨论的点**（这是最核心的差异）：
- 现有系统的 9 种 ConditionType 实际上已经覆盖了 ltree 关系查询 + ABAC 的场景：
  - `owner`/`self` = ABAC（属主判断）
  - `org_admin`/`org_member`/`org_subtree_member` 等 = ltree 组织关系查询
  - `super_admin` = 路由级 bypass
- 现有系统的 ConditionType 更具体更可操作，新框架借鉴其语义分类
- 是否应该直接采用现有系统的 ConditionType 设计，而不是另起炉灶？
- `GetFilterForResource` 列表过滤是亮点设计，新框架需要同等能力，但 SQL filter 生成比 MongoDB filter 更复杂
- restrict grants 存 MongoDB 嵌套文档很自然，PostgreSQL 需要 JSONB 或拆表设计
- evaluator DI 注入模式值得借鉴

---

### #4 Casbin 模型：g 表消除 + BFS 展开 vs 基础 RBAC

**现有系统设计**：
- Casbin 模型 3-field `[sub, obj, act]`，无 `[role_definition] g` 段
- matcher: `r.sub == p.sub && (p.obj == "*" || keyMatch2(r.obj, p.obj)) && (r.act == p.act || p.act == "*")`
- 角色继承不写 g 表，改在 authorizor 中 BFS 展开后逐角色 enforce
- BFS 三源合并：user_role_mapping + org→role + role_parent（按层批量查询）
- expanded_roles 存 gin context 供 handler 复用
- 策略三类：route 策略（`route:/path`）、按钮权限（`button:perm`）、API 权限（`/api/path, METHOD`）
- 超管通配：`p = role::ROLE_ADMIN, *, *`
- SessionAdapter + mutex + txnMarkerKey 防重入死锁
- LoadPolicy 全量重载（已知性能瓶颈）
- cmd/rebuild 增量 diff 重建策略

**新框架设计**：
- 基础 RBAC 模型（casbin_model.conf 中有 g 表定义）
- 角色继承用 Casbin g 表
- keyMatch2 路径匹配
- admin bypass

**需要讨论的点**：
- g 表消除是现有系统经过实践验证的优化，简化了模型但增加了 BFS 逻辑复杂度
- 新框架用 g 表更标准，但策略量大时 g 表性能问题
- BFS 三源合并（直接角色 + 组织角色 + 继承角色）是现有系统的成熟设计，新框架是否需要？
- expanded_roles context 复用模式值得借鉴
- 策略三分类（route/button/api）比新框架更细，是否需要？
- SessionAdapter 的 mutex + txnMarkerKey 机制在 PostgreSQL adapter 下是否还需要？

---

### #5 日志：zap + MongoWriteSyncer vs slog + Lumberjack

**现有系统设计**：
- zap + 自研 MongoWriteSyncer（缓冲批量写 MongoDB）
- 日志分两类：审计日志（access.log，TTL 90 天）+ 业务日志（各服务 .log，TTL 180 天）
- MongoWriteSyncer：缓冲 + 批量写入 + sync.Once 防 panic
- 每个服务有自己的日志集合（user.log / role.log / menu.log 等）
- 审计日志：中间件记录，含 trace_id/actor/action/status_code/method/path/cost/request_body（4KB 截断 + 脱敏）
- 敏感字段脱敏：password/token/secret/private_key（配置驱动）
- trace_id 全链路传播（requestid 中间件 → context → zap field）

**新框架设计**：
- slog + Lumberjack（文件轮转）
- 日志输出到文件 + stdout
- 审计日志存 DB（Phase 1 **同步写入**；Phase 3a 异步演进）

**需要讨论的点**：
- zap vs slog：现有系统用 zap（性能更好但 API 复杂），新框架选 slog（标准库更简洁）。现有系统的 MongoWriteSyncer 是 zap 的自定义 Syncer，切 slog 后需要不同的集成方式。
- 日志存储：现有系统所有日志写 MongoDB（TTL 自动清理），新框架应用日志写文件、审计日志写 DB。审计日志写 PostgreSQL 需要考虑写入性能（批量写入 vs 逐条写入）。
- 日志清理：MongoDB TTL 索引自动清理很方便，PostgreSQL 需要分区表或 cron 定期 DELETE。
- 每服务独立日志集合 vs 统一日志表：现有系统每个服务一个日志集合，新框架是否需要一个统一 audit_log 表？
- trace_id 传播机制值得借鉴。

---

### #6 事务策略：MongoDB Transact vs PostgreSQL 事务

**现有系统设计**：
- `policySvc.Transact(ctx, fn)` 封装 MongoDB 多文档事务
- SessionAdapter：session + sync.Mutex（持有整个事务周期）+ txnMarkerKey（防重入死锁）
- 事务流程：重入检测 → 获取 mutex → 开 session + 事务 → 注入 txnMarkerKey → 执行 fn → commit → LoadPolicy
- 文档优先原则：Casbin 同步在事务外，失败仅 Warn
- 事务场景：DeleteRole（6 集合）、DeleteUser（3 集合）、DeleteOrganization（4 集合 × N 子树）
- SetUserRoles 无事务（已知 bug C2）

**新框架设计**：
- PostgreSQL 原生 ACID 事务
- 外键约束自动级联或手动事务
- Casbin 策略同步在事务内还是事务外？

**需要讨论的点**：
- PostgreSQL 原生事务比 MongoDB Transact 简单很多，不需要 SessionAdapter/mutex/txnMarkerKey
- 外键约束可以自动处理级联删除（ON DELETE CASCADE），减少手动事务代码
- 但 Casbin 策略不在 PostgreSQL 业务表中（独立表），需要手动同步，是否在事务内？
- 文档优先原则是否保留？PostgreSQL 事务可以同时写业务表 + Casbin 表（如果 Casbin 用 PostgreSQL adapter），但 LoadPolicy 仍需在事务外
- 现有系统的"事务内只写 MongoDB，Casbin 同步在事务外"策略是否适用于 PostgreSQL？

---

### #7 登录安全：LoginLocker 设计

**现有系统设计**：
- LoginLocker：Redis Lua 原子脚本，5 次失败锁 15 分钟
- Lua 脚本：INCR + 首次 EXPIRE + 达阈值 SET 锁定标记（原子操作）
- fail-close：Redis 故障返回 503 拒绝登录
- 防用户枚举：用户不存在和密码错误返回相同响应
- PasswordValidator：4 种字符复杂度（大写/小写/数字/特殊字符）+ 最小 8 位 + bcrypt 72 字节上限
- first_login 标记：首次登录强制改密
- token 集合无 TTL 索引（已知问题）

**新框架设计（Phase 1 已定）**：

- 登录限流：**Lua LoginLocker**（`INCR` + 首次 `EXPIRE` 原子），15min/5 次 → 429，见 [phase1/02-auth.md §登录限流](../phase1/02-auth.md)
- fail-close：Redis 故障 503
- 防用户枚举：用户不存在与密码错误同文案（401 + 20001）
- 密码：Phase 1 仅 bcrypt cost=12；复杂度 Phase 2
- 首次登录强制改密：`must_change_password` + JWT `mcp`

**决策**：✅ LoginLocker Lua、fail-close、防枚举、first_login 改密均已采纳。

---

### #8 级联删除与一致性

**现有系统设计**：
- DeleteUser：事务内删 3 集合（user + user_role_mapping + org_members），事务外撤销 JWT + 清登录锁
- DeleteRole：事务内删 6 集合，事务外清理 Casbin p 表 + 刷新 restrict 缓存
- DeleteOrganization：事务内删 4 集合 × N 子树，事务外失效组织缓存
- DeleteMenu：事务内删菜单 + 子菜单 + menu_api，事务外 SyncRolePermissions
- 系统资源保护：is_system=true 的角色/组织/菜单禁止删除
- Casbin 同步在事务外，失败仅 Warn（文档优先）
- restrict grants 缓存手动刷新

**新框架设计**：
- 外键 ON DELETE CASCADE 可能自动处理部分级联
- Casbin 策略同步需要手动处理

**需要讨论的点**：
- PostgreSQL 外键级联删除可以简化代码，但可能引发意外的大范围删除
- 哪些级联用外键、哪些用手动事务？
- Casbin 策略清理：现有系统在事务外做，新框架是否相同？
- restrict（资源级鉴权策略）的级联清理：现有系统手动刷新缓存，新框架如果有等价设计也需要考虑
- 系统资源保护（is_system）策略值得借鉴

---

### #9 组织架构设计

**现有系统设计**：
- 组织模型：physical（物理组织）和 virtual（虚拟组）两种类型
- 层级关系：organization_relation_mapping（多对多，支持多父级）
- 层级遍历：pkg/hierarchy 通用工具（BFSCollect/CanReach/GetDescendants 等）
- 成员管理：organization_members（多对多）
- 组织角色：organization_role_mapping（组织绑定角色，成员自动继承）
- 组织管理员：organizations.owner []string
- 环检测：AddRelation 前调用 HasCycle（BFS FindPath）
- 乐观锁：updated_at 字段
- restrict 的 org 系列 condition 依赖组织层级遍历

**新框架设计**：
- 实体组织 + 虚拟组统一建模
- PostgreSQL ltree 扩展处理层级
- path 字段存储层级路径

**需要讨论的点**：
- ltree vs 手动 BFS：ltree 查询性能更好但不支持多父级（DAG），现有系统支持多父级
- 如果业务需要多父级组织，ltree 无法满足，需要用闭包表或路径枚举 + 手动遍历
- 组织角色自动继承是现有系统的成熟设计，新框架是否需要？
- 组织管理员（owner []string）与 restrict 的 org_admin condition 配合使用
- 层级遍历工具（pkg/hierarchy）是否可以复用？

---

### #10 动态路由与菜单权限

**现有系统设计**：
- 菜单模型：3 种 type（1=目录, 2=菜单, 3=按钮），含 path/component/icon/sort/permission 等前端字段
- 菜单-API 绑定：menu_api 多对多，是 Casbin API 策略的来源
- Casbin 策略三分类：
  - route 策略：`route:/path`（前端路由权限）
  - 按钮权限：`button:perm`（前端按钮控制）
  - API 策略：`/api/path, METHOD`（后端接口权限）
- GetUserPermissions：从 Casbin p 表收集 route: 和 button: 前缀权限码
- 超管通配：`role::ROLE_ADMIN, *, *`
- swagger 驱动 API 同步：cmd/sync-apis 从 swagger.json upsert api 表

**新框架设计**：
- 菜单管理 CRUD
- `/user/menus` + `/user/permissions` 接口
- 前端权限码

**需要讨论的点**：
- 策略三分类（route/button/api）比新框架更精细，是否采用？
- 菜单-API 绑定设计是现有系统的亮点：菜单 → API 关联 → Casbin API 策略自动生成
- swagger 驱动 API 同步值得借鉴
- 菜单的完整前端字段（component/icon/svg_icon/keep_alive/affix/redirect/is_link 等）是否需要？

---

### #11 多二进制入口设计

**现有系统设计**：
- 5 个二进制：server（HTTP 服务）/ init（同步预设）/ rebuild（重建 Casbin）/ dedup（规则去重）/ sync-apis（同步 API）
- 前 4 个通过 `app.InitApp` 走 Wire DI 装配
- sync-apis 直接连 MongoDB 操作
- 共同模式：`ctx.NewGlobalContext()` → `app.InitApp(ctx)` → 调用 App 方法

**需要讨论的点**：
- init/rebuild/dedup 这类运维工具很有价值，新框架是否需要？
- 如果用 PostgreSQL，init 可以改为执行 migration + seed data
- rebuild 对应 Casbin 策略全量重建，在策略不一致时很有用

---

### #12 配置管理：viper 环境变量化

**现有系统设计**：
- viper + `${VAR:-default}` 环境变量展开（正则匹配）
- `SetEnvPrefix("ZHUZHAO")` + `AutomaticEnv`
- 凭据环境变量化：BIZ_DB_USERNAME / BIZ_DB_PASSWORD / REDIS_PASSWORD / CTEM_AK / CTEM_SK
- WatchConfig 热重载
- 两份配置：config/config.yaml（Docker 网络）+ build/config.yaml（host 网络）

**需要讨论的点**：
- `${VAR:-default}` 展开机制比 viper 原生 `AutomaticEnv` 更灵活，是否采用？
- 热重载是否需要？

---

### #13 测试体系

**现有系统设计**：
- 19 个集成测试（4127 行）+ 26 个单元测试文件
- 集成测试需 MongoDB + Redis 运行
- TestMain 入口初始化测试 app 实例复用
- miniredis 用于 Redis mock
- 测试覆盖：登录/锁定/密码/用户CRUD/角色CRUD/菜单/组织/Casbin E2E/Restrict/级联删除/CTEM

**需要讨论的点**：
- 测试体系设计成熟，值得借鉴
- PostgreSQL 测试用什么？testcontainers-go？
- miniredis 对 Redis mock 很方便

---

## A 组综合讨论结论（2026-08-11）

### 总体判断

现有系统的 MongoDB + Restrict + Casbin(无g表) + SessionAdapter 是一套互相咬合的齿轮。新框架不能逐个替换，必须整体评估。审计后认为：**PostgreSQL 仍然是对的选型，但现有系统的鉴权设计（Restrict ConditionType + g 表消除 + BFS 三源）比新框架的概念设计更成熟，应该借鉴重新实现。**

### 逐项结论

#### #1 数据库：坚持 PostgreSQL

现有系统用 MongoDB 的方式其实很"关系型"——映射表全是标准多对多关系，唯一利用灵活 schema 的只有 restrict.grants 嵌套文档（PostgreSQL JSONB 可替代）和 ctem.data（建议剥离）。PostgreSQL 的关系完整性、原生 ACID 事务、ltree 层级查询恰好解决了现有系统的痛点。

**决策：PostgreSQL。restrict.grants 用 JSONB 存储，ctem 不纳入底座。**

#### #3 资源级鉴权：借鉴 ConditionType 语义，分阶段实现

现有系统的 9 种 ConditionType 在语义上覆盖了新框架的组织关系查询 + ABAC，但更具体、更可操作。新框架**借鉴其语义分类**，但实现方式分阶段演进：

**Phase 1（代码内联，零新依赖）**：
- 不建独立引擎，不建策略表
- ConditionType 的 9 种语义通过代码内联 + PostgreSQL ltree SQL 实现：
  - `owner` → `WHERE created_by = $userID`
  - `org_member` / `org_manager` → `WHERE org_path @> (SELECT org_path FROM user_orgs WHERE user_id = $userID)`
  - `role_member` → 代码内 `hasRole()` 判断
  - `admin` bypass → `if hasRole(roles, "admin")`
- 列表过滤：直接生成 SQL WHERE 子句（参数化查询，防注入）
- 理由：Phase 1 场景简单，代码内联足够；PostgreSQL ltree 让组织关系查询变成一条 SQL

**Phase 2（策略可配置，微服务化时）**：
- 引入 JSONB 策略表 + evaluator 接口（开闭原则）
- 策略按 (role, resource, action) 配置 conditions，运行时可调
- `GetFilterForResource` 列表过滤生成 SQL WHERE 子句
- 内存缓存 + 手动失效
- 可选引入外部 PDP 服务（SpiceDB / OpenFGA / Cerbos）处理跨服务复杂关系

**Phase 2 存储方案预留**：
```sql
-- 方案 A：JSONB 嵌套（接近现有系统）
CREATE TABLE restrict_policies (
    role_code  VARCHAR PRIMARY KEY,
    grants     JSONB NOT NULL,  -- {"user":[{"action":"read","conditions":[{"type":"org_member"}]}]}
    checked_orgs JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 方案 B：拆表（更规范但查询稍复杂）
CREATE TABLE restrict_grants (
    role_code  VARCHAR,
    resource   VARCHAR,
    action     VARCHAR,
    conditions JSONB,
    PRIMARY KEY (role_code, resource, action)
);
```

**决策演进总结**：
- ✅ 借鉴 ConditionType 的 9 种语义分类（不另起炉灶）
- ✅ Phase 1 代码内联实现（不建引擎，零新依赖）
- ⏳ Phase 2 策略表 + evaluator（策略可配置时启用）
- ❌ 不直接移植自研 Restrict 引擎（使用者少、无社区、维护成本高）

> **更新（2026-08-11）**：经业界调研和讨论，资源级鉴权方案调整为分层架构。详见下方"资源级鉴权架构决策"。

#### 资源级鉴权架构决策（2026-08-11 更新）

**核心转变**：从"所有鉴权在一个项目做"改为"分层鉴权 + 资源级下放"。

**业界标准**（OWASP / NIST / Cerbos / AuthZed 一致推荐）：

```
请求 → API Gateway（PEP-1：粗粒度）
     │  认证：验证 JWT
     │  路由级鉴权：Casbin（角色 × 路径 × 方法）
     │
     → Service/Handler（PEP-2：细粒度）
        资源级鉴权：代码内联判断
        ├─ 属主判断：if userId == resource.CreatedBy
        ├─ 组织关系：SQL ltree 查询
        └─ 超管 bypass：if hasRole(roles, "ROLE_ADMIN")
```

**为什么 Gateway 不做资源级鉴权**：Gateway 没有业务数据，无法判断"这个用户是否是这条数据的属主"或"这个用户是否是资源所属组织的管理员"。

**Phase 1 方案（零新依赖）**：
- 路由级：Casbin RBAC（已用 Casbin）
- 资源级：代码内联 + PostgreSQL ltree SQL 查询
- 列表过滤：SQL WHERE 子句内联

**Phase 2 升级路径（微服务化时）**：
- Gateway 做认证 + 路由级鉴权
- 各微服务自己做资源级鉴权
- 可选引入 PDP 服务（SpiceDB / OpenFGA / Cerbos）处理跨服务复杂关系

**方案对比结论**：

| 方案 | 新依赖 | Phase 1 适用 | 可配置 | 社区 | 决策 |
|------|--------|-------------|--------|------|------|
| Casbin ABAC | 无 | 部分适用（属主） | 部分 | 大 | 不单独用 |
| SpiceDB | 独立服务 | 过重 | 完全 | 大 | Phase 2 评估 |
| Warden | 嵌入库 | 可用 | 完全 | 小 | 不采用 |
| 自研 ConditionType 引擎 | 无 | 可用 | 完全 | 无 | 不采用（但借鉴语义） |
| **代码内联 + SQL** | **无** | **适用** | **否** | **大(Casbin)** | **Phase 1 采用** |

#### #4 Casbin 模型：g 表消除 + BFS 展开 + 每资源独立 Enforcer

现有系统消除 g 表是经过实践验证的优化。新框架采用相同方案：
- Casbin 模型 3-field `[sub, obj, act]`，无 g 段
- matcher: `r.sub == p.sub && (p.obj == "*" || keyMatch2(r.obj, p.obj)) && (r.act == p.act || p.act == "*")`
- 角色继承在 authorizor BFS 展开，不写 g 表
- expanded_roles 存 gin context 供 handler 复用

改进点：BFS 结果缓存到 Redis（`perm:user:{userId}`），避免每次请求查 3 个表。

**策略爆炸解决方案**：现有系统用 Restrict 引擎避免 Casbin 策略爆炸。新框架采用"每资源独立 Enforcer"方案：
- 路由级：全局唯一 enforcer（~1,000 条策略，可控）
- 资源级：简单资源用代码内联判断（属主、组织关系），不引入 Casbin
- 资源级：复杂资源（策略可配置）用独立 enforcer，独立策略表（`casbin_rule_ticket` 等）
- 资源接口统一：无论用 Casbin 还是代码内联，对外都实现 `Resource` 接口

详见 [design-decisions.md#6 资源抽象与自注册机制](./design-decisions.md#6-资源抽象与自注册机制) 和 [design-decisions.md#8 Casbin 策略爆炸](./design-decisions.md#8-casbin-策略爆炸每资源独立-enforcer)。

**决策：g 表消除 + BFS 三源合并 + Redis 缓存。资源级按需引入独立 enforcer，避免策略爆炸。**

#### #6 事务策略：PostgreSQL 原生事务，去掉 SessionAdapter 全部机制

PostgreSQL 原生 ACID 事务不需要 SessionAdapter/mutex/txnMarkerKey。标准 `BeginTx → 操作 → Commit/Rollback` 即可。文档优先原则保留：Casbin 同步在事务外，失败仅 Warn。

**决策：PostgreSQL 原生事务。Casbin 同步事务外。文档优先原则保留。**

#### #8 级联删除：外键 + 手动事务混合

| 级联场景 | 策略 |
|---|---|
| 映射表（user_role / role_menu / menu_api 等） | 外键 ON DELETE CASCADE |
| 业务实体删除（删组织子树等） | 手动事务 |
| Casbin 策略清理 | 事务外 |
| JWT 吊销 / 缓存失效 | 事务外 |
| 系统资源保护 | is_system 禁止删除 |

**决策：映射表外键 CASCADE，业务实体手动事务，副作用事务外。is_system 保护。**

完整场景矩阵（用户/角色/组织/菜单 × 增删改移）见 [rbac-inheritance-and-cascade.md §3](./rbac-inheritance-and-cascade.md#3-本项目级联策略矩阵ssot)。

#### #9 组织架构：ltree（树形），虚拟组统一建表

大多数企业组织是树形（一个部门一个上级），ltree 足够。虚拟组与实体组织 **统一** 在 `organizations` 表，用 `org_type=4` + `source=local` 区分，仍作为 ltree 子节点挂载在实体下（见 [hr-directory-sync.md](../proposal/hr-directory-sync.md)）。

如果未来确需多父级，改用闭包表（closure table）。

**决策：ltree 树形组织 + 同表虚拟组（org_type=4）。暂不支持多父级。HR 同步见 [hr-directory-sync.md](../proposal/hr-directory-sync.md)。**

### A 组对架构文档的影响

以下章节需要更新：

| 架构文档章节 | 更新内容 |
|---|---|
| §4.2 第一层 Casbin | 改为 g 表消除模型 + BFS 展开 |
| §4.3 第二层资源级鉴权 | 改为 ltree 组织关系查询 + 代码内联 + 按需独立 enforcer |
| §4.4 第三层 ABAC | 合并到资源接口的 Authorize 实现 |
| §4.5 策略爆炸 | 更新为每资源独立 enforcer 方案 |
| §10 数据库 Schema | 增加 casbin_rule_{resource} 独立策略表（按需） |
| §12.3 事务分析 | 去掉 SessionAdapter，改为 PostgreSQL 原生事务 |
| §6 组织架构 | 确认 ltree 树形，虚拟组统一建表（org_type=4） |
| §14.4 数据库迁移 | 补充种子数据幂等性原则（ON CONFLICT DO NOTHING） |
| §18 Phase 1 | 补充资源自注册机制 + 运行时 Sync 安全规则 |

> 具体更新在 B 组讨论完成后统一进行。
