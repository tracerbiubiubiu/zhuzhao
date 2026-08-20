# 12 - Phase 1 实现核验与验收报告 + Phase 2 衔接分析

> 核验日期：2026-08-19  
> 核验分支：`feature/step-1-infra`（含 6 个修复提交 8be8205..d54bda6，工作区干净）  
> 方法：全量阅读 docs/（design / modules / api / proposal / phase1 / phase2 / phase3 / roadmap）后，以 [phase1/README §1.3](./README.md#13-验收标准) 的 27 个验收用例 + 10 份模块计划为基准，逐项比对 `internal/`、`migrations/`、`scripts/`、`deployments/` 实现代码；含 git 考古（对关键设计做提交历史溯源）。  
> 本文由两份独立核验报告（原 12 号草稿与 13 号独立核验）合并而成，所有代码引用均经二次验证。

---

## 一、项目文档全景

| 目录 | 职责 | 关键结论 |
|------|------|---------|
| `design/` | 为什么这样设计 | architecture（Gin/Viper/PG/Casbin/Redis + ltree 内联资源鉴权）、design-decisions（§18 部署与代码解耦等 18 项决策）、rbac-inheritance-and-cascade（Phase 2b+ 备忘）、system-comparison、implementation-plan（**已废弃**，phase1/ 取代） |
| `proposal/` | 具体方案 | resource-model（Resource/Registry 接口抽象）、hr-directory-sync（虚拟组/实体双源 + 三条硬规则）、data-init（迁移幂等原则）、auth-design、deployment-evolution、overview |
| `modules/` | 跨阶段模块完整形态 | 9 份（含 ticket：三层鉴权映射 + 6 张表 DDL + scope 设计） |
| `api/` | HTTP 契约 SSOT | errcode.md（1xxxx–7xxxx 已实现段 + 8xxxx/9xxxx 预留段）、response.md（code/message/data/request_id 信封） |
| `phase1/` | Phase 1 实施计划 | 00–11 共 12 份 + 本文；README §1.3 = 验收 SSOT（27 用例），§2.3 = M1–M7 里程碑 |
| `phase2/` | Phase 2 实施计划 | 6 份 PRD + README（2a Registry/工单 MVP → 2b 组织/存储/体验 → 2c 组织内委托），文档已较完整 |
| `phase3/` | 生产加固 | 仅 README + 01-observability；02–09 待编写（符合计划） |

---

## 二、Phase 1 实现核验

### 2.1 模块级核验

| 模块 | 计划要求 | 实现证据 | 结论 |
|------|---------|---------|------|
| infra | 迁移/配置/Wire/优雅关闭/健康检查 | `migrations/000001–000007`、`internal/config`、`internal/app/wire.go`+`wire_gen.go`、`/health/live`+`/health/ready`（PG+Redis 双探针） | ✅ |
| auth | 登录/双Token/RT轮换/登出/黑名单/限流/会话吊销 | 登录限流 Lua 原子（`internal/pkg/redis/scripts/login_lock.lua`，INCR+EXPIRE 15min/5次）；RT 轮换 `GetDel` 原子取旧 + SHA-256 hash 比较防重放；登出拉黑 AT jti；改密吊销全设备（F-4） | ✅ |
| authz | Casbin 路由级 RBAC + Registry 空接口 | PG adapter + SyncedEnforcer；自服务白名单中间件标签方案；Registry 见 §四 G-1 | ⚠️ 基本达成 |
| user | CRUD/启禁/改密/角色绑定/超管保护 | 路由 10 条全注册；超管保护 guard+count+写同事务 + advisory lock；优先级防提权（F-2） | ✅ |
| role | CRUD/菜单分配/Casbin 同步 | AssignMenus 事务写 role_menus/casbin_rule 后 `LoadPolicy()`；影子超管过滤 | ✅ |
| org | 树 CRUD/ltree/用户关联 | 9 条路由（含 members 增删/Move/GetMembers）；循环引用与移动到子节点防护（50003） | ✅ |
| menu | CRUD/菜单树/权限码 SSOT | 5 条路由 + `GET /user/menus`、`GET /user/permissions`（自服务） | ✅ |
| audit | 操作日志中间件/同步写/登录审计 | WithoutCancel+3s 独立超时（F-5）；登录审计各分支显式调用 + 密码脱敏 `***`；`GET /audit/logs` | ✅ |
| middleware | JWT fail-close/Casbin/CORS/RequestID/安全头 | JWT 中间件：混合凭证 400+20008 → 黑名单 → disabled → mcp，任一 Redis 错误 → 503（fail-close）；全局链 Recovery→RequestID→AccessLogger→CORS→SecurityHeaders→BodyLimit(1MB) | ✅ |
| concurrency | 事务/SyncedEnforcer/Redis原子/乐观锁 | RunInTx、pg_advisory_xact_lock、Lua、users.version 乐观锁 | ✅ |

### 2.2 验收用例 27/27 逐项核验

`scripts/acceptance-phase1.sh` 对 §1.3 全部 27 个用例**均有断言**（含 #1 ready/种子、#10 限流 429+20006、#16 并发 refresh 竞争、#17 Redis 停机 503+10008、#22 混合凭证 400+20008、#25–#27 自服务/RBAC 区分、M6 审计 4 项补充检查）。

| # | 用例 | 实现机制（已验证） | 结论 |
|---|------|------------------|------|
| 1 | migrate-up / dev | `Makefile`、migrations 000001–000007、ready 双探针 | ✅ |
| 2 | login 双 Token | `AuthService.Login` → `issueTokenPair` | ✅ |
| 3/4 | /user/menus、/user/permissions | `MenuService.GetUserMenus/GetUserPermissions` + 自服务路由 | ✅ |
| 5 | GET /users 需权限 | Casbin 中间件 enforce | ✅ |
| 6 | refresh 轮换 | `Refresh`：`GetDel` 原子取旧 RT + hash 比对 | ✅ |
| 7/8 | logout / 登出后 401 | `Logout` 拉黑 jti；JWT 中间件查 `blacklist:at:{jti}` | ✅ |
| 9 | 防枚举同文案 | Login 统一 `ErrInvalidCredentials` | ✅ |
| 10 | 限流 429 | `login_lock.lua` Lua 原子 | ✅ |
| 11/11b | 禁用后旧 AT/RT | JWT 中间件 `user:disabled`（403+30003）；Refresh `isUserDisabled`（401+20004） | ✅ |
| 12 | 最后 superadmin 拒绝 | `CountActiveSuperadminUsersTx` + `AcquireSuperadminGuard` 同事务 advisory lock | ✅ |
| 13 | admin 重置超管密码 403 | `ResetPassword` 中 `canManageTarget` → 30005 | ✅ |
| 14/14b | mcp 拦截 | JWT 中间件 `MustChangePassword` 仅放行 `/auth/password/update`（403+20007） | ✅ |
| 15/15b/26 | 无角色/无策略 | Casbin deny → 70003/70001；自服务白名单不豁免零角色 | ✅ |
| 16 | 并发 refresh 仅一次成功 | `GetDel` 原子性：竞争者拿不到 storedHash → 20004 | ✅ |
| 17 | Redis fail-close | 中间件每步 Redis 错误 → 503+10008 | ✅ |
| 18/19/20 | 组织成员 | AddMember 200；RemoveMember 影响行数→404+50007；SetUserOrgs 覆盖 | ✅ |
| 21 | priority 防提权 | `canSetRolePriority` / `canAssignRole` → 30009 | ✅ |
| 22 | 混合凭证 | JWT 中间件 `hasMixedAuth` → 400+20008 | ✅ |
| 23/24 | 模糊/精确查询 | `ListUsers` LIKE / 精确 | ✅ |
| 25/27 | viewer 零 menu / 勾菜单后 | 白名单 200+[]；AssignMenus 后 GET/POST /users 200 | ✅ |

**里程碑 M1–M7**：均有对应实现；M6 审计含登录审计（service 层显式调用，非中间件——有意设计）与操作审计中间件。

**运行状态**：`go build`/`go vet`/单测 6 包/集成测试 7 包（testcontainers PG，-race）全绿；验收脚本需 Docker Compose 环境执行。

### 2.3 与文档契约的一致性确认

1. **错误码**：errcode.go 10000–70003 与 api/errcode.md 已实现段逐条一致；8xxxx/9xxxx 预留段标注「Phase 2 实现时写入、勿改号」。
2. **Resource 接口签名**：`internal/pkg/resource/registry.go` 与 phase2/02 §2.3 用法完全匹配，Phase 2a 无需重设计。
3. **部署形态**：`deployments/` Compose 双套（dev 仅 PG+Redis / 完整部署）满足「单实例 Compose 默认验收拓扑」。
4. **响应规范**：统一信封与 response.md 一致。

---

## 三、实现缺口（Phase 1 收尾清单）

> 经 git 考古修正定性：G-1/G-2 均与「资源级鉴权预留」相关，但性质不同。

| # | 级别 | 发现 | 定性（考古修正） | 影响 |
|---|------|------|----------------|------|
| G-1 | **P1** | **ResourceRegistry 未接入依赖图**：wire.go L31 声明 `resource.NewRegistry` provider，但无任何消费者，`InitializeApp` 未实例化 | [phase1/03-authz.md L209](./03-authz.md) 计划「Phase 1 仅 wire 注入空 Registry」这一步**未完成**（provider 声明 ≠ 注入生效）。资源级鉴权实现本身属 Phase 2（正常），缺的是 Phase 1 承诺的注入链路 | phase2/02 §1 前置「空 Registry 已 Wire 注入」不满足；Phase 2a Step 1 需先接线（加 `Deps.Registry` 字段即可，方案已论证） |
| G-2 | **P1** | **双轨资源鉴权抽象**：`internal/service/authz_service.go` 的 `CheckResourcePermission` 为 "not implemented" stub（在 wire set 中但无消费者），与 SSOT 的 `pkg/resource/registry.go` 并存 | 骨架时代（08-10，`2e092c9`）按旧设计有意预埋；08-15 `0ab3462`「对齐分阶段 SSOT」确立 ResourceRegistry 模式后被**取代**，但代码与 architecture.md 三处旧描述未清理。签名（单 roleKey、无 GetFilter）已无法满足 Phase 2a 需求 | 误导后续实现者（读 architecture.md 会以为 authz_service 是正道）；处置 = 删代码 + 修 architecture.md 三处（见 §五 A-3）。**文档侧三处已于 2026-08-19 提前修正**（architecture.md 交互契约/DI 图/接口表均已改为 ResourceRegistry 模式），剩余仅删代码动作 |
| G-3 | P2 | Makefile 无 `lint` 目标 | — | CI 质量门禁缺失，可随 Phase 2 顺带补 |
| G-4 | P2 | phase1/README §5 文档索引滞后 | — | 本文与 11 号合并版入索引时一并处理——✅ 已处理（2026-08-19，索引含 11 合并版与本文） |

**与文档偏差（非缺陷）**：
- 登录审计在 `AuthService` 内联调用而非 `AuditService` 中间件——`audit_service.go` 注释明确为有意设计（公开路由不走鉴权链）；
- ResourceRegistry 空接口已定义（G-1 仅缺注入边）；
- 影子超管：`user_repo.ExcludeSuperadminUsers` + `ListRoles(hideSuperadmin)` 已实现。

---

## 四、Phase 1 实际完成情况对 Phase 2 的输入

1. **安全基线已高于原计划**：F-1~F-10 修复后（详见 [11-code-review.md](./11-code-review.md)），令牌类型混淆、优先级提权、TOCTOU、软删唯一键、审计断连、HTTP 超时、密码 min=8、种子强制改密均已在 Phase 1 落地。Phase 2 文档中所有「Phase 1 现状」描述需以此为准。
2. **会话吊销语义已定型**：双轨机制（`user:disabled` 拒绝标记 + 删全部 RT）+ 改密/重置/禁删三类触发场景，含 `clearUserDisabled` 时序约束（[02-auth.md §会话吊销](./02-auth.md)）。Phase 2b 多设备管理必须**复用**而非另建第三套语义。
3. **Casbin 同步方式**：AssignMenus 后全量 `LoadPolicy()`（无 Watcher，单实例）。策略规模增长后需评估（见 §五 B-2）。
4. **admin bypass 双机制**：路由级靠种子 g/p 策略；资源级（Phase 2）TicketResource.Authorize 硬编码 HasRole。两处机制不同但均以角色码为锚，Phase 2 文档应显式声明该差异。

---

## 五、Phase 2 文档需补充的内容（分级清单）

> Phase 2 主体文档（6 份 PRD + README）已齐备且与 Phase 1 契约大体吻合，缺口集中在**衔接门禁、接线任务、基线漂移、编号规划**四类。以下 A 类已同步补入 phase2 文档（2026-08-19）。

### A 类：阻断 Phase 2a 启动（已补）

| # | 缺口 | 落点 | 内容 |
|---|------|------|------|
| A-1 | 验收门禁无可执行证据链 | phase2/README §1.6 | 27 用例 × 自动化方式 × 运行入口矩阵；「修复提交合入 dev」为 2a 切分支前置 |
| A-2 | Registry 接线任务缺失（G-1） | phase2/02 Step 0 | `Deps.Registry` 字段接线 + 启动期资源清单日志；说明「provider 已声明、无消费者」现状 |
| A-3 | authz_service.go 处置未决策（G-2） | phase2/02 §5 | 删除 stub + wire set 调整 + architecture.md L76/L440/L502 三处旧描述修正 |
| A-4 | 迁移编号未规划 | phase2/README §2.4 | 自 000008 起按子阶段分配；幂等与 down 迁移要求（对齐 000006 down 加固经验） |

### B 类：降低返工（已补齐，2026-08-19）

| # | 缺口 | 落点 | 内容 |
|---|------|------|------|
| B-1 | 鉴权分层决策声明 | phase2/02 §2.1 | 何时走 Casbin、何时走 Registry.GetFilter/Authorize；为什么列表过滤不进 Casbin |
| B-2 | Casbin 策略规模边界 | phase2/02 §2.6 | 全量 LoadPolicy 在 Phase 2 量级可接受；Watcher 后移 Phase 3 的边界 |
| B-3 | 多设备与会话吊销衔接 | phase2/01 §0 | 复用双轨机制；KickDevice=删单设备 RT，不得触碰 `user:disabled` 键 |
| B-4 | 测试落点具体化 | 各 PRD 测试节 | 单测/集成/router 行为测试路径约定 |
| B-5 | 密码复杂度基线漂移 | phase2/01 §3.4 | Phase 1 已有 min=8（F-9）；2b 上线后校验归一策略（避免双错误码） |
| B-6 | 工单 404 错误码选型 | phase2/09 待决策点 | 收口为 90001（`ErrTicketNotFound`） |

### C 类：可选（暂不处理）

- 10-storage 的 MinIO compose 片段落 `deployments/`；
- phase2/README §3 两项长期「建议」转「已决策」；
- phase1/03 反向同步 assigned 不含 BFS 的结论指针。

---

## 六、结论

1. **Phase 1**：10 模块全部落地，27 个验收用例在代码层面全部有实现证据且验收脚本 27/27 覆盖；6 个安全修复提交已入库（详见 [11-code-review.md](./11-code-review.md)），单测/集成测试全绿。遗留 G-1（Registry 注入边，Phase 1 计划内未完成项）与 G-2（被取代的旧 stub + architecture.md 残留）两项收尾，均列为 Phase 2a Step 0 第一动作，不阻塞 Phase 1 验收结论。
2. **Phase 2 文档**：主体完备，接口签名与 Phase 1 代码零冲突；A 类 4 项衔接缺口已补入 phase2 文档（A-1~A-4），B 类 6 项已同步，C 类可选。
