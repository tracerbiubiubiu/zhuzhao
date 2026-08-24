# 第二+三轮深度检查发现报告（60 项修复后的盲区扫描，含三轮合并）

> **报告日期**：2026-08-24（第三次复核更新：新增 §10 Phase 2/3 演进影响验证）
> **检查方式**：第二轮 4 个并行专项 agent（安全 / 逻辑与并发 / 性能与资源 / 测试与工程配置）+ 第三轮 5 个并行专项 agent（代码逻辑 / 安全性 / 性能 / 测试覆盖 / 依赖与配置）→ 主会话逐项交叉验证（全部 P1/P2 读码实证）
> **基线排除**：[01 号报告](./01-phase1-systematic-review-findings.md) 60 项已全部修复，本轮不重复；并抽查确认 60 项修复**无回归**（dummy bcrypt、status=1 过滤、DSN 编码、TrustedProxies、Move 事务化、双 primary 索引等均在位）
> **本轮定位**：修复完成后的盲区扫描——发现集中在「修复未横向覆盖」「部署/配置面（首轮未查）」「修复机制组合行为」三类
> **合并说明**：① 已并入另一份独立深度扫描（原 `03-deep-scan-findings.md`，35 项）——重叠 11 项互相印证、新增采纳 9 项，逐项记录见 §8，原文件已删除；② 已并入第三轮多维度检查（原 `04-third-round-deep-review-findings.md`，20 项）——新增采纳 2 项、展开并入 15 项、不采纳 3 项，逐项记录见 §9，原文件已删除；③ §10 为 2026-08-24 演进影响复核——以当前代码为准逐项验证，交叉比对 Phase 2/3 规划文档，新增 2 项规划级发现（D2-48/49），并将已有排期项显式标注

---

## 1. 执行摘要

| 严重度 | 数量 | 分布 |
|--------|------|------|
| **P1 严重** | **4** | Redis 驱逐策略攻击链（吊销机制可被废除）；docker-compose 环境变量命名错误（完整部署必失败）；角色更新 status 零值穿透（一次改名静默熔断全角色权限）；Phase 2 迁移编号 000008 冲突（规划级，§10.1） |
| **P2 一般** | **13** | 登录审计断连丢失、禁用 DB-Redis 部分写、两个错误码 handler 断链、审计表膨胀、弱密钥防线不对称、两个缺索引、审计写阻塞响应、组织写路径目标校验缺失、Casbin 多副本策略陈旧、pgx 预编译缓存未启用、2b 设备管理前置条件落空（规划级，§10.1） |
| **P3 建议** | **29** | 修复横向对齐缺口、防御性、规模预留项、合并补充项 |

> **演进标注（§10）**：46 项中 10 项已有 Phase 2/3 排期（Phase 1 不必修）；Phase 1 必修清单收敛见 §10.3。

**核心结论**：上轮 60 项修复质量高且无回归，但暴露三个系统性盲区：
1. **部署/配置面首次纳入检查即发现致命问题**（CFG-01：照文档执行 docker-compose 部署必然失败）
2. **修复不横向覆盖**：B2-3 patch 语义只改用户模块（角色/组织/菜单仍是全量覆盖零值穿透）、B3-3 只接了 SetUserOrgsTx、B4-6 分页上限只加给审计
3. **Redis 双重角色矛盾**：既是安全状态存储（吊销/禁用/锁定）又用 `allkeys-lru` 驱逐策略 + 登录输入无长度上限，构成完整攻击链

---

## 2. P1 严重（3 条代码级，全部主会话实证；另 1 条规划级 D2-48 见 §10.1）

### [D2-01] P1：Redis `allkeys-lru` + 登录输入无长度上限 → 应急吊销/禁用/锁定机制可被未认证攻击者主动废除

- **位置**：`deployments/redis/redis.conf:11-12`（`maxmemory 256mb` + `maxmemory-policy allkeys-lru`）+ `internal/model/token.go:22-27`（`LoginRequest.EmployeeNo` 仅 `required` 无 `max`）+ `internal/service/auth_service.go:71,90,103`（failLogin → `lock:login:{employee_no}` 键）
- **攻击链**（每环均实证）：未认证攻击者每次登录请求携带 ~1MB 随机 employee_no → INCR 创建 ~1MB 键（TTL 900s）→ 约 256 个请求灌满 256MB（受 dummy bcrypt 限速仍在分钟级）→ LRU 开始驱逐**最久未访问**的键 → `user:disabled:{uid}`（禁用标记）/ `blacklist:at:{jti}`（登出吊销）被驱逐 → **被禁用/已登出账户的存量 AT 复活（≤30min）**；`lock:login:*` 被驱逐则爆破锁定重置
- **严重性**：与上轮 P1「角色禁用不生效」同级——管理员应急禁用攻击者的操作可被攻击者主动废除；两个独立 agent（SEC-01/CFG-05）交叉确认
- **修复**：① `EmployeeNo` 加 `max=50`、`DeviceID` 加 `max=64`（顺带修 D2-12 资源滥用）；② redis.conf 改 `noeviction`（所有键均有 TTL，不会堆积）；③ 登录接口加 IP 级限流

### [D2-02] P1：docker-compose 数据库环境变量整体命名错误——完整部署必然启动失败

- **位置**：`deployments/docker-compose.yaml:54-58`（`APP_DB_HOST/PORT/USER/PASSWORD/NAME` 五个变量）vs `internal/config/config.go:154-163`
- **实证**：viper `AutomaticEnv` + `EnvKeyReplacer` 的自动映射名是 `APP_DATABASE_*`（键 `database.host` → `APP_DATABASE_HOST`）；`BindEnv` 显式绑定的只有 4 个键（其中 `database.password` → `DB_PASSWORD`）。**compose 注入的 `APP_DB_*` 五个变量全部不被读取**——app 容器按 config.yaml 连 `localhost:5432` → 容器内无 PG → Ping 失败 → `restart: unless-stopped` 无限重启。agent 用同版本 viper 做了行为实验复现（非推断）
- **连带**：`.env.example`/compose 文档引导用户走此路径；且 app 无 healthcheck，失败只能靠看日志发现
- **修复**：compose 改 `APP_DATABASE_HOST/PORT/USER/PASSWORD/DBNAME`（或 config.go 补 `database.*` 的 BindEnv）；修后跑一次 compose 冒烟

### [D2-03] P1：UpdateRole/UpdateOrg 的 Status 零值穿透——按文档典型用法一次「改名」请求即静默禁用角色

- **位置**：`internal/model/role_request.go:36`（`Status int binding:"oneof=0 1"`——零值 0 放行且无法区分「未传」）+ `internal/service/rbac_service.go:119`（`role.Status = req.Status` 无条件覆盖）；组织同型：`internal/model/org_request.go:27` + `internal/service/org_service.go:227`
- **触发**：[05-role.md:16](../phase1/05-role.md) 文档场景明确是「更新角色 | 修改角色名称、描述」——客户端只传 `{id, version, name, description}` 时 status 零值 0 穿透 → 角色被静默禁用 → **B1-1 语义下该角色全部成员的鉴权/菜单/优先级档位立即收回**，无任何告警。前端联调第一次改名字即触发
- **佐证**：用户模块 B2-3 已全面指针化（patch 语义），角色/组织未跟进；现有测试全部显式传 `Status: 1`，零值路径无守护
- **修复**：`UpdateRoleRequest.Status`/`UpdateOrgRequest.Status` 改 `*int`（与 B2-3 同型）；补零值守护测试

---

## 3. P2 一般（12 条；另 1 条规划级 D2-49 见 §10.1）

| ID | 位置 | 摘要 |
|----|------|------|
| D2-04 | `internal/service/audit_service.go:49` + `auth_service.go` 11 处 LogLogin | **登录审计未用 WithoutCancel**：F-5 修复只覆盖中间件路径；客户端断连时登录成功/失败审计全部丢失（审计「登录事件全量记录」承诺失效，撞库攻击无 DB 证据） |
| D2-05 | `internal/service/user_service.go:288-294,357-360` | **禁用/改密的 DB-Redis 部分写**：DB 提交后 revoke 失败（Redis 闪断）→ 存量 AT 继续完全访问；无重试/补偿。**对比说明**：禁用的会话失效依赖 revokeUserSessions（Redis 单点，非 user_roles 清理）；删除路径有 DB 层兜底（SoftDeleteTx 同事务清 user_roles → GetRoleCodes 空 → casbin 403），禁用路径无此兜底——这正是差异所在〔表述经质询澄清〕 |
| D2-06 | `internal/handler/errors.go:12-43`（无 50012 分支） | **ErrOrgSystemProtected handler 断链**：B4-5 修复在 HTTP 层失效——系统组织 Update 返回 500+10000，errcode.md 承诺 403+50012；零测试覆盖所以没被拦住 |
| D2-07 | `internal/handler/errors.go:41-43`（无 70004 分支） | **ErrPolicyReloadFailed 同型断链**：B3-5 的「已提交但策略刷新失败」语义到不了客户端（body.code=10000） |
| D2-08 | `internal/middleware/audit.go:41-43` + `audit_service.go:99-105` + `migrations/000001:140` | **未认证审计表膨胀 DoS**：登录失败同步写 audit_logs，request_body 含原文 employee_no 无长度上限（单行 ~1MB）、TEXT 列无截断——与 D2-01 独立的 DB 侧放大 |
| D2-09 | `internal/config/validate.go:71-79` + `configs/config.yaml:3` | **弱 JWT 密钥防线依赖自声明 mode**：debug（默认）+ 公开弱密钥即可启动，HS256 下密钥即一切；compose 路径已防护但裸机部署防线不对称 |
| D2-10 | `internal/repository/user_repo.go:60-85` + `migrations/000001:115-121` | **`user_orgs.org_id` 无索引**：GetMembers 的 COUNT+LIST 双查询、CountMembers、Delete 保护检查均顺序扫描（org_id 非复合 PK 前导列） |
| D2-11 | `internal/repository/audit_log_repo.go:61-121` + `migrations/000001:144-145` | **audit_logs 仅时间过滤/默认排序/COUNT 无索引**：created_at 是两个复合索引的第二列；只增表慢性恶化（与 Phase 2 清理策略一起处理） |
| D2-12 | `internal/middleware/audit.go:47-71` | **审计同步写阻塞响应实际送达**：Go net/http 整个中间件链返回后才 flush——所有响应 <4KB 时客户端等审计 INSERT 完成才收到响应，DB 抖动时每请求尾延迟 +3s；代码注释与 08-audit.md「用户无感知」表述不成立。修复：Insert 前 `c.Writer.Flush()` 一行 |
| D2-37 | `internal/service/org_service.go:95-107,236-255` + `router.go:143-155` | **组织写路径缺目标校验**〔合并自 S-P1-1，按 Phase 1 语境降级〕：AddMember/RemoveMember/Move/Update/Delete 五写路径仅做存在性检查，无 `canManageTarget` 类目标校验——用户侧 SetUserOrgs 有 ensureCanManage，组织侧不对称。**暴露面验证（§10 复核）**：种子默认态 org:* 仅 admin/superadmin 持有（role_menus/casbin 种子实证），但菜单管理页正常使用即可给自定义角色勾选 org 写按钮——第一个自定义角色形成即暴露面形成；另 operator 描述文案「可管理组织成员/角色/子组织」与实际零权限绑定不符（描述暗示的权限若被补绑同样暴露）。**⚠️ Phase 1 上线前决策点（外部质询采纳）**：若 Phase 1 独立上线对外，不应无声推到 2c——需二选一：① 显式记录「组织为共享资源」设计决策（文档化 + operator 描述文案修正）；② 补最小护栏（org 写操作仅存存在性校验升级为 + 持权者范围约束）。终局保护仍在 2c Step 9 防提权矩阵（见 §10.2） |
| D2-38 | `internal/casbin/enforcer.go:26-37` | **Casbin 未启用 StartAutoLoadPolicy**〔合并自 S-P0-2，降级〕：多副本部署下实例 A 改策略仅刷新自身，实例 B 陈旧直至重启——已撤销权限存在放行窗口。Phase 1 单实例无实际影响；cleanup 调 `StopAutoLoadPolicy()` 却从未 Start（无操作佐证）。Phase 2 多实例前置条件：`StartAutoLoadPolicy(30s)` 或部署文档约束单实例 |
| D2-39 | `internal/pkg/postgres/postgres.go:14-24` | **pgx 未启用预编译缓存**〔合并自 S-P1-5〕：全仓无 Prepare，每查询走 extended 协议重复 parse/plan；`poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe` 一行使全部 repository 受益（缓存按连接隔离、无在线 DDL，无失效风险） |

---

## 4. P3 建议（29 条，紧凑格式）

**修复横向对齐类**（B 系列修复只改了首发模块）：
- D2-13 分页 page 无上限（user/org，溢出回绕负 OFFSET→500）：`user_repo.go:577-588`——B4-6 只修了审计；建议 normalizePage 统一加上限
- D2-14 AssignMenus 重复 menu_id 误报 404：`rbac_service.go:212`（ListByIDs 去重行数 ≠ len）——B3-4 只给 SetUserOrgsTx 做了去重
- D2-15 AddMember 的 23505 未映射（并发双 primary → 500 而非 50011）：`org_repo.go:88-98`——B3-3 只接了 SetUserOrgsTx
- D2-16 SetUserOrgsTx 裸 INSERT 无软删防御：`org_repo.go:146-148`——B4-3 只给 SetRolesTx 做了 INSERT...SELECT
- D2-17 菜单/组织 Update 全量覆盖：`menu_service.go:205-210`（未传 component/icon → 清空）、org 的 description/sort_order 同型——B2-3 只改了用户模块
- D2-18 用户列表 pageSize 回显不一致（请求 200 实查 100 回显 200）：`user_service.go:59-65`——org GetMembers 已完整 clamp

**安全防御纵深类**：
- D2-19 maskSensitive 三重绕过面（Unmarshal 失败原文入库/仅顶层/大小写敏感）：`middleware/audit.go:78-93`——form-encoded 提交改密接口可致明文密码入审计表
- D2-20 禁用用户分支缺 dummy bcrypt（时序可枚举被禁用工号）：`auth_service.go:89-100`——B4-1 只修了「不存在」分支，一行补齐
- D2-21 ILIKE 通配符未转义（`%`/`_`/`\`）：`user_repo.go:547-550`
- D2-22 device_id 无长度/字符校验（RT JWT 与 Redis 键膨胀）：`model/token.go:25`〔第三轮 D3-04 建议并入：入 key 前字符白名单校验 `[a-zA-Z0-9_-]` 或 SHA-256 后缀，employeeNo 同理〕
- D2-23 CORS AllowAllOrigins 条件性风险：`middleware/cors.go:11`——当前 Bearer 认证下低危，**Phase 2 引入 cookie 前必须收紧**（建议在 phase2/01-auth-enhance.md 登记）〔按需求方指示暂挂观察，不纳入本轮修复〕
- D2-24 X-Request-ID 无条件信任客户端（日志膨胀/追踪污染）：`middleware/logger.go:15-21`

**逻辑/防御类**：
- D2-25 includeMenuAncestors 无环检测（DB 脏数据成环时 GetUserMenus goroutine 死循环）：`menu_service.go:313-334`——同文件 buildMenuTree 对环安全，行为不一致
- D2-26 用户 Create 的 SetRolesTx 静默丢角色（TOCTOU 窗口角色被软删→创建成功但角色未绑、无提示）：`user_service.go:148-151`
- D2-27 审计按工号筛选：软删用户 → 404，历史审计按工号不可查：`audit_service.go:56-61`
- D2-28 用户列表 role 筛选不过滤禁用角色（两视图矛盾）：`user_repo.go:559-565`——B1-1 计划内豁免，但需在 05-role.md 补行为说明
- D2-46 `ListRoleCodesByUserIDs` 命名与行为不符（入参实为 roleIDs，`WHERE id = ANY($1)`）：`role_repo.go:261`——B4-4 引入，调用方传参正确无功能影响，纯命名误导（按名理解传 userIDs 将查出错误结果）。修复：重命名 `ListRoleCodesByRoleIDs`〔第三轮 D3-03，已实证〕

**工程配置类**：
- D2-29 dev 与 prod compose 共享项目名/卷名/容器名（数据互污）：`deployments/docker-compose.dev.yaml` vs `docker-compose.yaml`
- D2-30 testutil.SetupPostgres 是共享容器自毁开关（t.Cleanup 注册 sharedTerm，当前零调用的埋雷 API）：`testutil/testdb_integration.go:79-87`
- D2-31 测试 schema 漂移（casbin_rule 列名 p_type vs 生产 ptype，缺 000004/000005）：`testutil/testdb_integration.go:61`
- D2-32 handler 层零测试（D2-06/07 正是从这个洞漏出；writeServiceError 全码表驱动测试是最低成本补法）
- D2-33 compose 文档化迁移步骤不可执行（镜像无 migrate 二进制）+ response.md 两处示意未同步（request_id 格式/流程图漏 Audit）
- D2-47 pgxpool Ping 与连接共用超时（DNS+TLS 慢时初始化误判）：`postgres.go`——建议 Ping 独立 10s 超时〔第三轮 D3-20〕

**性能规模预留类**（当前规模无感，随增长触发）：
- D2-34 写路径 N+1 残留（Create/SetUserOrgs 逐条 FindByID、事务内逐条 INSERT）——Phase 2 HR 批量同步前处理〔含合并项 S-P2-11 的 N+1 部分；其 TOCTOU 部分已由 B4-3 SetRolesTx INSERT...SELECT 修复；第三轮 D3-07 补充：AssignMenus 事务内 role_menus + casbin_rule 逐条 Exec，可改 `INSERT...SELECT...FROM unnest($1)` 批量〕
- D2-35 role_menus.menu_id / user_roles.role_id 缺索引（防御性，与 D2-10 合并一个迁移）
- D2-36 其余：GetUserPermissions 两次角色查询可合一〔含 S-P2-9、D3-08〕、**GetUserMenus 三次往返可省一（ListRoleIDs + ListByRoleIDs + ListAll——ListAll 结果内存过滤即可替代 ListByRoleIDs，`menu_service.go:31-49`）〔D3-02〕**、**Casbin 中间件逐角色串行 enforce（命中即 break，未命中遍历全部角色，`casbin.go:72-87`；角色数 1-3 时代价可忽略，策略增长后可 BatchEnforce/短路 admin）〔D3-01〕**、菜单/组织树无缓存〔含 S-P1-4〕、ILIKE 前导通配（pg_trgm 预留）、revokeUserSessions 全 keyspace SCAN、Casbin 全量重载写锁（万级策略触发）〔含 S-P2-5〕、登录 4 次 Redis 往返（不值得动）

**合并补充类**（源自深度扫描，未与本报告重叠）：
- D2-40 RT 轮换 GetDel→校验→Set 崩溃窗口〔S-P0-3，降级〕：进程崩溃于窗口内 → 用户被迫重登。**明确标注：非安全问题、低优先级**——并发刷新已由 GetDel 原子挡住（auth_service.go:162，`auth_service.go:118` clearUserDisabled 为独立路径），多设备各持独立键无竞态；S 报告的「多设备并发轮换竞态」不成立。可选 Lua 原子化〔表述经质询澄清〕
- D2-41 roles(status,deleted_at) 组合索引缺失〔S-P2-4〕：GetRoleCodes/GetRoles 每请求过滤该组合（B1-1 加的 status=1）；并入 D2-10/D2-35 的索引迁移一笔做
- D2-42 密码仅 min=8 无复杂度策略〔S-P2-2〕：文档已注明「完整复杂度策略 Phase 2」（user_request.go 注释），登记防遗漏
- D2-43 modules 文档路径与实际不符〔S-P1-8〕：`docs/modules/README.md:49`、`organization.md:3` 仍写 `internal/service/org/` 子目录，实际为扁平 `org_service.go`——Phase 1 验收曾指出未改
- D2-44 杂项（六小项，S-P3 合并）：① errcode.md 预留段（20009+/50008+/90001+）建议加更醒目的「未实现」分隔标注〔S-P0-1 的有效内核，见 §8.3〕② reloadPolicy 的 `time.Sleep(100ms)` 阻塞请求 goroutine 且不尊重 ctx（B3-5 引入）③ `model/user.go:10-28` tab/空格缩进混用需 gofmt ④ OrgService.Create 未前置校验 ltree path 深度上限（超深子树拼 path 报错为 500）⑤ GetUserOrgs 对任意用户可见（ensureVisible 仅挡超管目标）——管理端设计确认项 ⑥ `auth_service.go:183` ExpiresAt 理论 nil panic：令牌签发恒带 exp，仅持有密钥伪造无 exp 令牌可触达（纵深项）
- D2-45 测试残余缺口〔S-P1-6/P1-7/P2-6/P2-8 + 第三轮 D3-05/06/10~17 的未闭环部分〕：service 层 UserService.Create/Delete/SetRoles/ResetPassword 与 CreateRole priority 越权（30009）无直测；**登录锁定 Lua 脚本（阈值 5 次/窗口 15min/EXPIRE 时机）与 auth 集成锁定分支完全无测试**（`internal/pkg/redis/` 零测试文件——第三轮实证）；RT 篡改/跨设备/disabled 用户 RT 场景、并发刷新（-race）无 Go 测试；maskSensitive、validate.LtreeLabel/Identifier、includeMenuAncestors、revokeUserSessions、MapForeignKeyViolation(23503) 均无单测；handler 层 writeServiceError 全码表/extractBearer/审计 start>end 校验无测试（D2-06/07 正是从该洞漏出）；验收脚本缺种子数据完整性/密码 7 字符拒绝/改密脱敏断言——B1-B4 的 ~30 个守护测试已闭环大部分，此为收敛后的残余清单

---

## 5. 交叉验证记录

| 复核项 | 方法 | 结论 |
|--------|------|------|
| D2-01（Redis 攻击链） | 读 redis.conf L11-12 + token.go 无 max= + auth_service failLogin 三处 | **成立**（两个 agent 独立发现，合并） |
| D2-02（compose 变量） | 读 config.go BindEnv 清单 vs compose L54-58 逐变量比对 | **成立**（AutomaticEnv 映射名 APP_DATABASE_*，agent 另做 viper 同版本实验） |
| D2-03（status 零值） | 读 role_request.go:36 非指针 + oneof 放行零值 + rbac_service 无条件覆盖 | **成立** |
| D2-06/07（错误码断链） | grep errors.go 无两常量 | **成立** |
| D2-04（审计 ctx） | 读 audit_service.go:49 直接透传请求 ctx | **成立** |
| 误报剔除 | — | 无误报；1 组重复（SEC-01=CFG-05）、1 组部分重叠（F-07=CFG-03）已合并 |

**60 项修复回归抽查**：dummy bcrypt、status=1 七处过滤、DSN 编码、TrustedProxies、审计 WithoutCancel（中间件路径）、Move 事务化+并发交叉守护、双 primary 部分唯一索引、patch 语义、影子超管 404、XFF 防护——**全部在位**。

### 5.1 外部质询裁决记录（2026-08-24，两轮合并后）

收到对 D2-06/07（错误码断链）、D2-19（审计脱敏）的两条修正意见与 D2-05/40 的表述澄清请求。逐条读码裁决：

**质询 1「D2-06/07 断链论断不成立」——驳回，原论断维持**。质询方两个论据均与代码事实不符：
- 「ErrOrgSystemProtected 经 ErrOrgIsSystem 同分支（403）」：`errors.go:18` 的 `switch biz.Code` 为**数值匹配**，`ErrOrgIsSystem.Code=50006`（errcode.go:72）≠ `ErrOrgSystemProtected.Code=50012`（errcode.go:75），同族错误不互配，50012 落 default
- 「default → InternalError 仍透传正确 code」：`response.go:132` 签名 `InternalError(c *gin.Context, message string)` **只收 message 字符串**，default 分支调用 `InternalError(c, biz.Message)`（errors.go:42）时 biz.Code 无法传入，内部硬编码 `ErrInternal.Code=10000`——70004 到不了客户端的铁证在**函数签名**上
- 结论：50012 → 实际 500+10000（承诺 403+50012）、70004 → 实际 500+10000（承诺 500+70004），D2-06/07 维持 P2

**质询 2「form-encoded 改密不会进审计脱敏路径（会被 ShouldBind 400 拒绝）」——驳回，form-encoded 恰是最短触发路径**。执行顺序实证（audit.go 全文）：
- L40-44 审计中间件在 `c.Next()` **之前**读 body（不看 Content-Type，原文必被捕获）
- L47 handler 执行，form-encoded 时 ShouldBindJSON 400 ✓（质询方对此判断正确）
- L59 `maskSensitive(bodyBytes)` 对**原文**脱敏；L83-84 `json.Unmarshal` 失败 → `return string(body)` **原文入库**
- 关键：审计条目在 `c.Next()` 返回后**无条件写入**（L68），handler 400 不阻止——400 发生在 handler 层，脱敏发生在中间件层且中间件在前。D2-19 维持原文表述

**质询 3（D2-40）/质询 4（D2-05）表述澄清——采纳**：D2-40 补「非安全问题、低优先级」显式标注；D2-05 补「会话失效依赖 revokeUserSessions（Redis 单点），对比删除路径的 user_roles 清理 DB 兜底」对比说明。已更新正文条目。

---

## 6. 负面确认清单（四 agent 合并，重点项）

**安全**：密码哈希不出 API（json:"-"）；SQL 全参数化无动态 ORDER BY；keyMatch2 语义无过匹配（`/users` 策略不匹配 `/users/evil`）；路径标准化无分裂（Gin 与 Casbin 同一 Path）；日志注入不可行（JSON 转义）；JWT 强制 HMAC+typ 双向校验；登录锁定键不可绕过（精确等值+同源键）；错误信息零泄露；安全头五项齐备；admin matcher 通配有文档背书+三重业务兜底。

**逻辑**：全部事务方法 Begin/Rollback/Commit 模式完整；UserService.Create 跨 repo 正确共享事务；树构建对环安全（孤儿提升为根）；Casbin SyncedEnforcer 并发安全且 reload 在 Commit 后；最后超管三路径 advisory lock 闭环（禁用 superadmin 角色绕过不可达）；乐观锁四模块一致；启停顺序正确（Shutdown drain 覆盖同步审计）。

**性能**：JWT 中间件已 pipeline；连接池与低并发匹配；HTTP 超时体系无逃逸；鉴权热路径全索引化；Redis 键全 TTL 无泄漏。

**测试配置**：config.yaml↔config.go 字段完全对齐；errcode.go↔errcode.md 44 码逐条一致（B4 新增已同步）；TRUNCATE 顺序外键安全；零 t.Parallel 无竞态；miniredis 语义足够；依赖零冗余零 replace；.gitignore 完整；B1-B4 守护测试覆盖密度高（唯 B3-5 零测试）。

---

## 7. 修复优先级建议

> **演进视角修订（§10.3）**：本表为原始节奏；经 Phase 2/3 排期交叉比对后，**已有排期的 10 项从本清单移出**（Phase 1 不必修，见 §10.2 映射表），**D2-48（迁移编号）升入立即批次**。执行以 §10.3 收敛清单为准。

| 批次 | 内容 | 理由 |
|------|------|------|
| **立即（P1 + 顺带）** | D2-01（redis.conf 改 noeviction + 登录输入 max=50/64；IP 限流已排期 3a 不做）、D2-02（compose 变量名 + 冒烟）、D2-03（role/org Status 指针化 + 守护测试）、D2-48（Phase 2 迁移编号表更新，纯文档）；**D2-37 上线前决策**（若 Phase 1 独立对外：共享资源文档化 vs 最小护栏，二选一）；顺带 D2-20（一行 dummy bcrypt） | 吊销机制有效性 / 部署可用性 / 文档场景即触发的权限熔断 / 2a Step 2 撞号 / 组织暴露面不可无声推 2c |
| **第二批（P2）** | D2-04（LogLogin WithoutCancel）、D2-06/07（两个错误码映射 + writeServiceError 全码表测试）、D2-12（审计 Flush 一行）、D2-10/D2-35/D2-41（一个迁移补 5 个索引）、D2-13（page 上限统一）、D2-39（QueryExecMode 一行）、D2-46（函数重命名）、**D2-49①（修订 2b PRD 前置条件，纯文档；②= 2b Step 7 首任务，见 §10.2）** | 低成本高收益，多数 1-10 行 |
| **第三批（测试基建，Phase 2 地基）** | D2-30（删埋雷 API）、D2-31（testutil 迁移列表补齐）、D2-32（全码表测试） | Phase 2 §5.2 测试先行的直接依赖 |
| **第四批** | D2-05（revoke 补偿/对账）、D2-08（审计 body 截断）、D2-09（弱密钥无条件拒绝）、D2-14/15/16/17/18（横向对齐）、D2-19（脱敏递归）、D2-43/D2-44/D2-45（文档与测试残余） | 安全语义补强 + 修复一致性 |
| **Phase 2/3 backlog（§10.2 已排期，勿在 Phase 1 重复做）** | D2-37 终局修复（2c Step 9；**上线前决策点已在「立即」行，勿只看本行**）、D2-38（3a）、D2-42（2b Step 7，有里程碑绑定）、D2-11/D2-34（随 2b 清理/HR Sync）、D2-23（引 cookie 前）、D2-40（**暂无里程碑绑定**，可选优化）、其余 P3 规模预留项 | 逐项排期位置见 §10.2 映射表，勿以本概览行为准 |

---

## 8. 与 03-deep-scan-findings.md 合并记录（2026-08-24）

另一份独立深度扫描报告（原 `03-deep-scan-findings.md`，35 项，含 CORS 按需求方指示暂挂的备注）已并入本文档，原文件删除。两份报告由不同检查方独立完成，重叠项互为印证。

### 8.1 重叠项（11 项，两份独立检查互相印证）

| 深度扫描编号 | 本文档编号 | 处置 |
|---|---|---|
| S-P1-3 审计脱敏失效（嵌套/数组/原文入库） | D2-19 | 同一发现，保留 D2-19（本报告含大小写第三重绕过面） |
| S-P1-4 高频读路径零缓存 | D2-36 | 合并（S 报告建议 singleflight 方案附记） |
| S-P2-1 ILIKE 未转义 | D2-21 | 同一发现 |
| S-P2-3 审计无 created_at 索引 | D2-11 | 同一发现（D2-11 另含 COUNT/默认排序两个维度） |
| S-P2-5 LoadPolicy 全量重载 | D2-36 | 合并 |
| S-P2-7 handler 整包零测试 | D2-32 | 同一发现 |
| S-P2-9 同请求重复查询 | D2-36 | 合并 |
| S-P2-13 审计按工号 404 | D2-27 | 同一发现 |
| S-P2-11 Create 校验事务外 + N+1 | D2-34 | TOCTOU 部分已由 B4-3 修复（SetRolesTx INSERT...SELECT）；N+1 并入 D2-34 |
| S-P3 normalizePage 三处重复/上限不一致 | D2-13/D2-18 | 合并 |
| S-P3 wire 预留桩无消费者 | R2-INFRA-03（首轮） | 过期——B4-6 已加「Phase 2 预留」注释闭环 |

### 8.2 新增采纳（9 项，编入正文）

D2-37（组织目标校验）、D2-38（Casbin 自动重载）、D2-39（pgx 预编译缓存）、D2-40（RT 崩溃窗口）、D2-41（roles 组合索引）、D2-42（密码复杂度登记）、D2-43（modules 路径）、D2-44（杂项六条）、D2-45（测试残余）。

**重新定级说明**：S-P0-2 / S-P0-3 / S-P1-1 三项按「Phase 1 单实例部署 + 组织无权限语义」实际语境降级——多副本与 org_roles 均为 Phase 2 前提，当前不构成 P0/P1；但 D2-37 列为 **Phase 2b 硬性门槛**（org_roles 引入权限语义后组织即成提权通道）。

### 8.3 判定无效（4 项，不采纳、留档备查）

| 深度扫描编号 | 判定 | 依据 |
|---|---|---|
| S-P0-1 errcode「空壳契约」（18+ 未实现码） | **过度陈述** | errcode.md 的 20009-20013 / 50008-50010 / 90001+ 段落均带「Phase 2/2b/2c 预留，码号预留勿改号」显式标注（errcode.md:94、112-114、140）；44 个已实现码与文档逐条核对一致（本报告负面确认）。有效内核（预留段更醒目标注）采纳为 D2-44① |
| S-P2-10 disabled 键 TTL=30min 禁用后旧 RT 窗口 | **无效** | 禁用即 revokeUserSessions **删除全部 RT**；Refresh 直查 DB status 拒绝签发新 AT；disabled TTL=AccessTTL ≥ 任何存量 AT 剩余寿命——三入口闭环（本报告负面确认 #6） |
| S-P2-12 go-redis ConnMaxIdleTime 字段名不匹配 | **无效** | go-redis v9 `Options.ConnMaxIdleTime` 字段名正确存在 |
| S-P3 testutil sync.Once 并发干扰 | **表述不准** | sync.Once 按测试二进制隔离（repository/service 各自独立容器），跨包无共享；真实风险是 SetupPostgres 将共享容器 cleanup 注册进单测 t.Cleanup（已录为 D2-30） |

### 8.4 部分过期（B1-B4 守护测试已闭环大半）

S-P1-6（user_service 写路径零测试）/ S-P1-7（rbac_service 零测试）/ S-P2-6（auth 测试缺口）/ S-P2-8（org_service 无测试）——B1-B4 批次已新增 ~30 个守护测试（RBAC 目标校验矩阵、AssignMenus 校验、影子超管 404、patch 语义、UpdateStatus 保护、菜单必填字段、org Move 并发交叉、GetMembers 分页、改密相同 400、JWT 错误码区分等），上述断言大部分不再成立；**残余缺口收敛至 D2-45**（Create/Delete/SetRoles/ResetPassword 直测、CreateRole priority 直测、登录限流/RT 篡改/并发刷新 Go 测试）。

另：S 报告的 CORS 暂挂指示与 D2-23 的处置一致（保留观察，Phase 2 引入 cookie 前必须处理）。

---

## 9. 与 04-third-round-deep-review-findings.md 合并记录（2026-08-24）

第三轮多维度深度检查报告（原 `04-third-round-deep-review-findings.md`，5 个专项 agent、20 项发现）已并入本文档，原文件删除。合并前对 04 号报告的关键声明逐项读码验证（D3-01/02/03/05/18 及抽查项），验证结论如下。

### 9.1 新增采纳（2 项独立条目，编入正文）

| 新编号 | 源自 | 内容 |
|---|---|---|
| **D2-46** | D3-03（已实证） | **`ListRoleCodesByUserIDs` 函数名与行为不符**：`role_repo.go:261` 名为 ByUserIDs 实际入参 `roleIDs []int64`（SQL `WHERE id = ANY($1)`），按名理解传 userIDs 将得到完全错误的结果——B4-4 引入（通配展开），调用方 menu_service 传参正确故无功能影响，纯命名误导。修复：重命名 `ListRoleCodesByRoleIDs` |
| **D2-47** | D3-20 | **pgxpool Ping 与连接共用超时**：`postgres.go` Ping 走 `cfg.ConnectTimeout` ctx，DNS+TLS 慢时初始化误判。建议 Ping 独立 10s 超时。P3 防御项 |

### 9.2 展开并入（15 项，充实已有条目而非新增）

| 源自 | 并入 | 增量内容 |
|---|---|---|
| D3-01（实证 `casbin.go:72-87`） | D2-36 | Casbin 逐角色串行 enforce 维度（04 定 P2，按管理端低并发+角色数 1-3 降 P3） |
| D3-02（实证 `menu_service.go:31-49`） | D2-36 | GetUserMenus 三次往返可省一（04 只说双查询，实际 ListRoleIDs+ListByRoleIDs+ListAll 三次） |
| D3-04 | D2-22 | 字符白名单校验建议（见 §9.3 误报说明） |
| D3-05（实证 redis 包零测试文件） | D2-45 | 登录锁定 Lua 脚本测试缺失展开（阈值/窗口/EXPIRE 时机从未验证） |
| D3-06 | D2-32/D2-45 | handler 未测路径清单展开（writeServiceError 40+ 分支、extractBearer、start>end 校验） |
| D3-07 | D2-34 | AssignMenus 事务内逐条 INSERT 补充 |
| D3-10~D3-17 | D2-45 | 测试缺失明细展开（maskSensitive/validate/includeMenuAncestors/revokeUserSessions/23503/验收脚本断言缺口） |
| D3-19 | D2-24 | X-Request-ID 格式校验建议（`^req-[a-f0-9]{32}$`） |
| D3-08/D3-09 | — | 04 自标注为已有条目确认项，无需新增 |

### 9.3 不采纳（3 项，留档备查）

| 源自 | 判定 | 依据 |
|---|---|---|
| D3-04 的「SCAN 模式干扰」增量 | **误报** | 声称 `deviceID="a:*"` 使 `revokeUserSessions` 的 SCAN 误匹配——SCAN MATCH pattern 固定为 `refresh:{uid}:*` 前缀匹配，key 名中的 `*` 是普通字符（SET 不解释通配符），pattern 必然覆盖该用户全部键，无逃逸面（首轮安全审计负面确认 #7 已验证）。有效部分（长度/字符校验）已并入 D2-22 |
| D3-18（Recovery stack 泄漏） | **低价值不采纳** | `recovery.go:19` stack 仅进 **Error 日志**（排障刚需，业界标准做法），响应侧只返回通用「服务器内部错误」（:22）无泄露；release 模式去 stack 反而损害可排障性 |
| 04 §4.2 N1-P1（SetRoles fall-through） | 04 自行剔除的误报，认可 | `user_service.go:325` RunInTx 已正确 return，agent 误读代码结构 |

### 9.4 三轮累计统计（更新自 04 §6；§10 演进复核另增 2 项规划级）

| 轮次 | 报告 | P0 | P1 | P2 | P3 | 合计 |
|------|------|----|----|----|----|----|
| 第一轮（系统性检查） | [01 号](./01-phase1-systematic-review-findings.md) | 0 | 2 | 13 | 45 | 60（全部已修复） |
| 第二+三轮（盲区扫描 + 两轮合并） | 本文 §2-4 | 0 | 3 | 12 | 29 | 44 |
| 演进影响复核（§10） | 本文 §10.1 | 0 | 1 | 1 | 0 | 2（规划级） |
| **累计** | — | **0** | **6** | **26** | **74** | **106** |

**累计 106 项（0 P0 / 6 P1）**。核心安全机制（JWT 双 Token、Casbin fail-closed、bcrypt cost=12、登录锁定 Lua 原子、会话吊销、优先级防提权、乐观锁）经三轮独立检查互相印证，无阻断级缺陷。P1 现状：角色禁用语义、角色目标校验（首轮，已修）；Redis 驱逐攻击链、compose 变量命名、Status 零值穿透（待修，§10.3 立即批次）；Phase 2 迁移编号冲突（规划级，待修文档）。**46 项未修复项中 10 项已有 Phase 2/3 排期（§10.2），Phase 1 实际必修 36 项（§10.3 收敛清单）。**

### 9.5 第三轮手动补检负面确认（04 §4.3 采纳）

go.mod 依赖无已知高危 CVE；migrations up/down 全对称（000008 down 注释合理）；Dockerfile 多阶段构建正确（CGO 禁用、非 root UID 33333，仅缺 HEALTHCHECK 指令）；Makefile `test-unit` 含 `-race`；casbin_model.conf matcher 正确——均无新发现。Refresh 不查登录锁定为设计正确（有 isUserDisabled + DB status + GetDel 三重防护）；bcrypt 输出固定 60 字符 VARCHAR(100) 充裕；casbin 仅用 v0-v2 无需 v3-v5 索引；sslmode 默认 disable 有生产覆盖指引。

---

## 10. Phase 2/3 演进影响验证（2026-08-24 第三次复核）

> **方法**：以当前代码为唯一事实来源逐项重验 §2-4 全部条目，交叉比对 [phase2/00-implementation-plan](../phase2/00-implementation-plan.md)、[01-auth-enhance](../phase2/01-auth-enhance.md)、[04-org-delegation](../phase2/04-org-delegation.md)、[phase3/README](../phase3/README.md) 等规划文档。
> **目的**：区分「Phase 1 必修（不修会阻断/污染 Phase 2/3 演进）」与「已有排期（Phase 2/3 计划已覆盖，Phase 1 不必修）」；本报告定位是修复 Phase 1，不做超前优化。

### 10.1 新增发现（2 项，均为规划级——代码无缺陷，但计划文档与代码现状脱节，不修正将在 Phase 2 对应 Step 直接踩坑）

#### [D2-48] P1（规划）：Phase 2 迁移编号 000008 与已落地迁移冲突

- **证据**：`migrations/` 目录实证存在 `000008_user_orgs_single_primary`（B3-3，2026-08-24 落地）；而 [phase2/00-implementation-plan](../phase2/00-implementation-plan.md) §2 M2a-2 / §3 Step 2 / §8 SSOT 表三处仍写「迁移 **000008**：ticket_types / tickets / ticket_comments」。该计划创建于 2026-08-19，早于 B3-3 落地——**规划未随 Phase 1 修复批次更新**。
- **影响**：2a Step 2 按计划建 000008 迁移时会与现有文件直接冲突（migrate 工具按序号执行，同号文件命名不同也构成 dirty 状态）；且 **000009 随后也被 Phase 1 占用**（本报告修复批次的加固迁移 `000009_phase1_hardening`：D2-10/35/41/D2-11(部分) 五索引 + D2-37① 角色描述修正，与 P1 立即批次同日落地）——SSOT 映射表整体失真。
- **修复后状态（2026-08-24 已落地，覆盖原修复建议）**：phase2/README §2.4 与 00-implementation-plan（里程碑表 / Step 清单 / RK-6 / §5.3 检查单 / §8 映射表共 16 处）已整体重排——Phase 1 用至 **000009**；phase2 依次为：ticket=**000010**、org-enhance=**000011**（原建议「维持 000009」因 000009 被 Phase 1 加固迁移占用而失效，顺延一号）、auth-enhance=000012（视需要）、storage=000013、delegation=000014。testutil 迁移列表已同步纳入 000009（见 D2-31）。
  - 原「仅顺延 ticket、其余维持原号」方案作废：000009 占用使 org-enhance 及其后全部编号联动 +1；按 README §2.4 既有「顺序编号、不跳号」机制统一重排（即 §10.4 参考方案所述项目自身机制）。

#### [D2-49] P2（规划）：2b 设备管理 PRD 的两个前置条件在 Phase 1 均未成立

- **证据**（grep + 读码实证）：
  1. [01-auth-enhance](../phase2/01-auth-enhance.md) §1 前置条件声称「登录时已 `SADD devices:{userId}`、`SET refresh:{userId}:{deviceId}`（见 02-auth）」——**实际全仓 grep `SAdd|devices:` 零命中**，Phase 1 从未写过 devices 集合；
  2. 同文档 §2.1 承诺 RT value 为设备元数据 JSON（`{jti, device_name, ip, user_agent, created_at, last_refresh_at}`）——**实际 [auth_service.go:272](file:///../../internal/service/auth_service.go) 存储的是 `hashToken(rt)`（SHA-256 hex）**，无任何设备元数据。
- **影响**：2b Step 7（M2b-4）设备列表/踢出 API 的两个前置全缺——`GET /auth/devices` 无数据源（devices 集合不存在 + RT 无元数据可展示）。前置条件落空会在 2b 中期才暴露，届时需要回改 Phase 1 的 issueTokenPair/Logout/revoke 链路（改 RT value 结构还会牵动 Refresh 的 hash 比较逻辑与既有测试）。
- **修复选项**（推荐 a，零代码扰动）：
  - a. **修订 PRD 前置条件章节**：将「SADD devices 集合 + RT value 结构化（hash 与元数据并存，如 `{"hash": "...", "meta": {...}}`）」列为 2b Step 7 的首个任务项（属 Step 7 本职范围——设备管理本来就是它引入的能力）；
  - b. Phase 1 顺带补 `SADD`（一行，低成本）但不动 RT value 结构——不彻底，2b 仍需重构 value；
  - 不建议在 Phase 1 修改 RT value 结构：牵动 Refresh 比较逻辑（169 行 `storedHash != hashToken(rt)`）、B1-B4 既有守护测试，违背「Phase 1 收口不再扩面」原则。

### 10.2 已有排期标注（Phase 2/3 计划已覆盖——Phase 1 不必修，避免重复劳动与过度优化）

| 本报告条目 | 排期位置 | 说明 |
|---|---|---|
| **D2-49**（§10.1） | **排期动作（外部质询采纳，两步）**：① Phase 1 第二批即修订 [01-auth-enhance §1](../phase2/01-auth-enhance.md) 前置条件（改为「2b Step 7 首任务负责落地」而非声称已存在）；② 2b Step 7 任务清单显式加首任务「devices 集合初始化（SADD）+ RT value 结构升级（hash 与设备元数据并存）」——重构 Refresh 比较逻辑与测试同 Step 完成 | 原 §10.1 只给影响未给排期动作、§10.2/§10.3 漏列，属文档内部不一致，已补 |
| D2-42 密码复杂度 | **2b Step 7**（有明确里程碑绑定）：`ValidatePasswordPolicy` + 20013（[phase2/00 P2-D4 表](../phase2/00-implementation-plan.md) + 01-auth-enhance §3.4） | PRD 明确「策略归一：长度/复杂度统一走策略校验」 |
| D2-38 Casbin Watcher/自动重载 | 3a multi-instance（RK-9：「Watcher 后移 Phase 3」） | Phase 2 全程单实例，StartAutoLoadPolicy 无必要 |
| D2-36 菜单/组织树缓存、权限缓存 | 3b platform（RK-7：「缓存后移 Phase 3」）+ 权限缓存 L1 输入限定（authz 评审 §11） | Phase 2 数据规模不需要 |
| D2-36 revokeUserSessions 全 keyspace SCAN | 2b Step 7：devices 集合语义落地后自然消解 | **依赖 D2-49 两步动作落地** |
| D2-37 组织写路径目标校验 | **终局：2c Step 9** 防提权矩阵（AddMember/RemoveMember 扩展 + D1–D6 + 组内级别校验）。**但 Phase 1 上线前有显式决策点**（共享资源设计 vs 最小护栏，见 D2-37 条目）——不可无声推到 2c（外部质询采纳）；2b 的 ticket_visibility 只引入读语义，不新增 org 写暴露面 | 暴露面随第一个持 org 写权限的自定义角色形成 |
| D2-34 写路径 N+1/批量 INSERT | 2b Step 4 HR Sync（RK-7 单 SQL 三源）+ 批量同步需求落地时 | Phase 1 量级无感 |
| D2-23 CORS 收紧 | Phase 2 引入 cookie 会话前（01-auth-enhance 范畴） | 已标注暂挂观察 |
| D2-11 audit 索引/分区/清理 | Phase 2 日志清理（08-audit.md 已排期）随做 | 已标注 |
| D2-01 的「IP 级限流」建议 | 3a security-enhance（phase3/README §1.1 明确含「API 限流」） | **注意**：限流属治标，Phase 1 仍必修 noeviction + 输入长度上限（治本），两者不互斥 |
| D2-40 RT 轮换 Lua 原子化 | **暂无里程碑绑定**（Phase 2/3 计划无对应 Step，外部质询采纳标注）——若 D2-49 的 2b Step 7 RT value 重构落地，则该 Step 顺带评估原子化；在此之前维持可选优化 | 避免排期表给人「已覆盖」的错觉 |

### 10.3 收敛后的 Phase 1 必修清单（§7 优先级表的演进视角修订）

> 判定标准：不修会**阻断 Phase 2 演进**（部署/契约/规划失真）、**污染 Phase 2 开发**（测试基建/命名）、或**动摇 Phase 2/3 依赖的根基机制**（Redis 会话语义、审计可信）。

| 批次 | 条目 | 演进理由（为何不能留到 Phase 2） |
|------|------|------|
| **P1 立即** | D2-01（noeviction + 输入上限）、D2-02（compose 变量）、D2-03（Status 指针化）、D2-48（迁移编号，纯文档） | 2b 设备管理与 3a HA 全部建立在 Redis 会话语义上（D2-01）；Phase 2 每个里程碑验收都依赖 compose 环境可起（D2-02）；Phase 2 前端联调第一天就触发（D2-03）；2a Step 2 直接撞号（D2-48） |
| **上线前决策点（若 Phase 1 独立对外）** | **D2-37 决策**：共享资源设计文档化（含 operator 描述修正）vs 补最小护栏——二选一，不可无声推到 2c（外部质询采纳） | 暴露面随第一个持 org 写权限的自定义角色形成，终局保护在 2c Step 9，窗口可能数月 |
| **第二批** | D2-04/06/07/12/20 + D2-13 + D2-46 + D2-39 + **D2-49 第①步（修订 2b PRD 前置条件章节，纯文档）**；第②步（devices 集合 + RT value 结构升级）显式排入 2b Step 7 首任务（见 §10.2 D2-49 行） | D2-46 重命名：2b BFS 三源会新增同类角色查询函数，命名混乱将放大；D2-49 不在 Phase 1 改代码（RT value 牵动 Refresh 逻辑与守护测试），但 PRD 修正必须立即做——否则 2b 中期才发现前置落空 |
| **第三批（测试基建，Phase 2 §5.2「测试先行」的地基）** | D2-30（删 SetupPostgres 埋雷 API）、D2-31（testutil 迁移列表/schema 漂移）、D2-32（writeServiceError 全码表测试） | Phase 2 §5.3-5 明确要求「testutil 迁移列表同步」——D2-31 的 casbin schema 漂移（p_type vs ptype）会在 2a 引入真实 adapter 集成测试时爆雷；D2-06/07 已证明缺全码表测试的代价 |
| **其余 P2/P3** | D2-05/08/09/10 等 | Phase 1 质量项，按 §7 节奏修，无演进阻断性 |

### 10.4 成熟参考方案（关键条目）

| 条目 | 参考方案 | 出处/依据 |
|---|---|---|
| D2-01 | `maxmemory-policy noeviction`（本项目全部键带 TTL，无堆积风险）；安全键与缓存分实例是彻底方案，管理端单实例 noeviction 足够 | Redis 官方文档对「含淘汰敏感键」实例的建议；项目内 `session_revoke.go` 键清单可证全 TTL |
| D2-03 | 请求字段指针化 patch 语义——**项目内 B2-3（user 模块）已是成熟模式**，直接复制到 role/org | 本仓 02-phase1-remediation-plan D1 决策 |
| D2-06/07 | 错误码→HTTP 映射改 map 表驱动 + 全码表单测（新增错误码漏映射时测试编译期即失败） | Gin 生态常见 errors.go 模式；本报告 D2-32 建议的最低成本形态 |
| D2-19 | 递归脱敏（map/slice 深度遍历 + `strings.EqualFold`）或字段白名单（只记允许记录的字段——审计合规常用）；Unmarshal 失败落 `<binary len=N>` 占位而非原文 | 金融审计系统通行做法；本仓 audit.go 改动 ~30 行 |
| D2-12 | `c.Next()` 后、审计 Insert 前调 `c.Writer.Flush()`——响应先达客户端，审计仍在请求生命周期内（Shutdown drain 覆盖），不违反同步写契约 | Gin/net/http 标准行为；一行改动 |
| D2-39 | `pgx.QueryExecModeCacheDescribe`——pgx 官方提供的 describe 缓存模式，按 SQL 文本缓存、无需改任何 repository | pgx v5 Wiki「Query Execution Modes」 |
| D2-33 | 多阶段 Dockerfile 从 migrate 构建阶段拷贝二进制，或 compose 注释改宿主机 `make migrate-up` | golang-migrate 官方镜像用法 |
| D2-48 | 按 phase2/README §2.4 既有「编号顺延标注」机制更新计划表 | 项目自身机制，无需引入新约定 |

### 10.5 第三次复核的代码级重验结论

对 §2-4 全部条目按当前 HEAD 重验（重点抽样 + 全量 grep 确认），**44 项全部仍然成立，无过期项、无回归**：
- 三个 P1 证据链复核：redis.conf `allkeys-lru`（L11-12）+ token.go 无 max= + compose `APP_DB_*` vs viper AutomaticEnv 映射 + role_request.go:36 非指针 Status——均与报告一致
- 上轮修复抽查：dummy bcrypt、status=1 过滤、Move advisory lock、000008 部分唯一索引、audit WithoutCancel（中间件路径）在位
- 新增两项规划级发现（D2-48/49）为本轮独有——前两轮 agent 与独立深度扫描均未覆盖「规划文档 vs 代码现状」维度，印证了演进视角复核的必要性
