# 全文档系统性审查报告（资深后端 / 架构师视角）

> 审查日期：2026-08-27
> 审查范围：`docs/` 全目录（VISION / roadmap / design / proposal / modules / phase1 / phase2 / phase3 / adr / api / ops），对照代码实现（commit `66e2c39` + 后续修正至集成测试修复提交）
> 审查方法：多代理并行审查（目标边界 / 安全可靠性 / 流程与一致性 / 测试工程化代码对照）+ 人工交叉验证；测试与 lint 实测执行（`go vet`、`go test ./...` exit 0）

---

## 总评

| 维度 | 评级 | 一句话结论 |
|------|------|-----------|
| 1. 目标与能力边界 | **A-** | SSOT 纪律是亮点，个别锚点数据过时 |
| 2. 模块划分与使用场景 | **A-** | 9 模块划分符合 IAM+工单系统业界实践，排除项清晰 |
| 3. 安全性设计 | **B+** | 认证/授权设计达到行业水准，备份恢复与日志防篡改缺位 |
| 4. 可靠性设计 | **B** | 事务边界清晰，但无备份/PITR 文档，TOCTOU 有遗留案例 |
| 5. 模块交互与业务流程 | **B+** | 主链路完整，异常分支部分依赖 Phase 3 设计稿 |
| 6. 阶段目标与边界 | **A** | 准入准出明确，延期项均有登记（业内少见的高纪律） |
| 7. 文档一致性 | **B** | 抽查主体一致，存在系统性「已完成但忘改旧文」回标滞后 |
| 8. 测试完整性 | **B+** | 文档宣称的测试落点全部可验证，有 3 个未登记盲区 |
| 9. 工程化配置 | **B-** | Makefile 覆盖面好，但有 2 处「开箱即坏」（详见 §9） |

---

## 1. 整体目标与能力边界

**结论：合格，定义完整清晰。**

- [VISION.md](../VISION.md) 明确定义「企业内部 IAM + 工单统一平台」，并以 §2「MVP 边界铁律」给出可执行的裁决规则（凡依赖 2c Authorize / 事件机制 / 审批流的一律 defer 到 Phase 3）。这种「裁决规则 + 应用示例」的写法优于多数项目纯愿望式愿景。
- 能力排除项记载一致且充分：多租户（预留不实现）、微服务拆分（明确不做）、RS256/AK/SK（按需后移）在 [roadmap.md](../roadmap.md) 各阶段重复声明且互不矛盾。

### 问题

| # | 级别 | 位置 | 问题 | 建议 |
|---|------|------|------|------|
| G-1 | P2 | [VISION.md §4](../VISION.md) | 「磁盘 `migrations/` 现状至 000009」已过时——000010/000015/000016 已落地并经 CLI up/down 验证 | 更新为「现状至 000016」，或将此动态信息移出 VISION（真相源指向磁盘本身更稳） |
| G-2 | P2 | [roadmap.md:98](../roadmap.md) | 引用 `doc/soar/activelist.md` 路径不存在于 docs 树（疑为外部仓库文档或笔误） | 确认路径；若为外部文档改为链接说明，避免死链 |
| G-3 | P2 | [VISION.md](../VISION.md) | 未提及「数据备份/恢复」属于哪个阶段的职责边界 | 在 VISION 或 ops 文档补一句归属（关联 S-2，见 §3） |

---

## 2. 模块划分与使用场景

**结论：合格，符合业界通用实践。**

- 9 个模块（auth/user/role/org/menu/authz/audit/middleware/ticket）划分与业界 IAM 产品（Keycloak/Casdoor 的领域拆分）及工单系统（Jira Service Management 的最小域）对齐；[modules/README.md](../modules/README.md) 提供了职责总表。
- 「不负责」边界写得明确（[architecture.md L59-L72](../design/architecture.md)：不做消息推送、文件存储、前端路由），这是很多项目缺失的部分。

### 问题

| # | 级别 | 位置 | 问题 | 建议 |
|---|------|------|------|------|
| M-1 | P2 | roadmap.md 预留扩展表 | 第三方登录仅预留字段级说明，无 OAuth2/OIDC 接入的最小设计（协议流、账号合并策略）；企业内部场景 LDAP/AD 同步常见度高 | 不阻塞当前阶段；建议在 proposal/ 下补一页《企业身份源接入》设计占位（LDAP/JIT 同步可作为 HR 目录同步的近亲复用 ltree 方案） |
| M-2 | P2 | modules/ticket.md | 使用场景缺少「异常运营场景」：撤单（非 closed 终态取消）、升级/转派跨组织、重开工单的处理人归属。transitions JSON 里 closed→open 存在但重开后 assigned_to 归属语义未见文档 | 在 09-ticket 或 10-ticket-business 补一小节「重开语义」（重开后 created_by 不变，assigned_to 清空回 open——这与现有 Assign 取消分派逻辑一致，写明即可） |

---

## 3. 安全性设计（对照 OWASP ASVS / NIST）

**结论：达标为主，有两处行业基线缺口。**

已达标项（有设计且有代码落点）：
- **认证**：双 Token + RT 轮换 + 吊销黑名单 + 多设备会话管理（[auth-design.md L13-L20](../proposal/auth-design.md)）；登录限流 Redis Lua + fail-close + 防用户枚举 + bcrypt 成本上限（[auth-design.md L64-L74](../proposal/auth-design.md)、[auth.md L119-L158](../modules/auth.md)）——覆盖 ASVS V2 认证核心要求。
- **授权**：三层模型（Casbin 路由级 → 可见性 L2 → 操作权 L3）默认拒绝、合取关系（[02-authz-resource.md L59-L85](../phase2/02-authz-resource.md)）；防提权以角色 priority 为轴 + 「只能管理比自己弱的角色」校验并有专项单测——覆盖 IDOR/BOLA 与水平越权主场景（不可见资源统一 404 反枚举已在 2a 重构中落地并经集成测试验证）。
- **审计**：同步审计中间件 + 登录审计 + 字段脱敏 + action 注册制。

### 缺口

| # | 级别 | 位置 | 问题 | 建议 |
|---|------|------|------|------|
| S-1 | P1 | 全 docs 无对应设计 | **审计日志不可篡改性零设计**：audit 与 ticket_events 均为同库普通表，DBA/应用漏洞可直接 UPDATE/DELETE。ASVS V7 建议至少「追加写 + 定期归档」 | 低成本方案：给 audit/ticket_events 加 `BEFORE UPDATE OR DELETE` 触发器 RAISE EXCEPTION（只允许物理级维护窗口操作），Phase 3 L2 异步再考虑外置存储 |
| S-2 | P1 | [ops/README.md](../ops/README.md) 仅空壳 | **备份恢复策略完全缺失**：单实例 Docker Compose 下 PG 数据卷无备份文档（pg_dump 周期/保留期/PITR/RPO-RTO 均无）。这是生产可用性的行业基线（即使暂缓 Phase 3，备份不该等观测性一起做） | Phase 2 内补最简版：每日 `pg_dump` cron + 异地卷挂载 + 一页恢复演练记录（Makefile 加 `backup`/`restore` 目标即可起步） |
| S-3 | P2 | auth-design.md | JWT 黑名单基于 Redis 单实例，Redis 故障时 fail-close 已设计（fail-safe 正确），但黑名单内存增长策略（TTL 对齐 AT 过期）未见量化描述 | 补一句 TTL 策略即可 |

---

## 4. 可靠性设计

**结论：合格边缘。**

- 写路径事务边界文档化良好：[10-concurrency.md](../phase1/10-concurrency.md) 列明哪些写路径必须同事务（角色菜单同步、用户角色、组织移动），且 2a 修复轮把工单状态变更 + 事件写入收进同一事务，与文档要求一致。
- 乐观锁（version 列）、SyncedEnforcer、Redis 原子操作均有专章。
- Phase 1 复盘沉淀的 TOCTOU 教训（F 系列）被真实转化为后续约束，这个闭环质量高。

### 问题

| # | 级别 | 位置 | 问题 | 建议 |
|---|------|------|------|------|
| R-1 | P2 | phase2/BK-3（已登记） | Update patch 不写 ticket_events + closed 读后写 TOCTOU 属已知项，缺回归守护测试 | 修复时同批补「closed 后 patch 必须失败」的条件更新测试（`UPDATE ... WHERE status != 'closed'`） |
| R-2 | P2 | [docker-compose.dev.yaml healthcheck] + Makefile | 依赖 healthcheck 的启动等待（dev-reset 用固定 sleep 5s）在慢环境会竞态 | 用 `docker compose up -d --wait` 替代 sleep（v2 插件语法），见 §9 E-3 |

---

## 5. 模块交互与业务流程

**结论：合格，主链路完整，部分异常分支合理依赖 Phase 3 设计稿。**

- 授权流（角色→菜单→menu_apis→Casbin 同步）文档链路完整且有 16 条绑定三方对账（router/handler/menu_apis 种子）佐证。
- 组织流（建组织→加成员→单主组织约束→move 级联）设计中 P2-D1 的 ltree 级联 SQL 已实现并通过 CLI 迁移与集成测试；但「子树+孙代 org_path 重映射」分支目前只有脚本层叶子节点用例（见 T-1）。
- 工单流 create→assign→in_progress→pending_verify→close 覆盖正常路径；reject/open→reopen 分支存在于 transitions 配置且有状态机单测。

### 问题

| # | 级别 | 位置 | 问题 | 建议 |
|---|------|------|------|------|
| F-1 | P2 | [09-ticket.md] 关联章节 | 反向关联判重（A→B + B→A 共存）PRD 未覆盖，实现自定行为依赖单向唯一索引 | 二选一：service 层加 OR 双向查重，或文档标注为已知限制 |
| F-2 | P2 | [09-ticket.md] §5 | ListComments 不过滤 is_internal——2a 读者⊆备注可见集合故无泄露，但 2b group/all scope 落地后即成真泄露，属「埋雷型」缺口 | 在 09-ticket §5 加显式 TODO：「2b 起 ListComments 对非 creator/assignee/admin 过滤 is_internal=true」（否则 2b 实现者按现文档必然遗漏） |
| F-3 | P2 | 模板相关 | PRD 要求模板 DefaultFields 预填，实现只覆盖 DefaultPriority（登记为 H-4 遗留）。文档-实现不一致应短暂标注「2b 实现」而非静默悬置 | 在 09-ticket §4 模板小节标注当前 2a 只落 priority，DefaultFields 于 2b 接线 |

---

## 6. 阶段目标与边界 / 演进平滑度

**结论：优秀（本次审查最高分维度）。**

- 三阶段划分准入/准出标准明确：phase1/README §1.3 验收 27 例含对抗路径；phase2/README 给出 2a→2b→2c 推进逻辑与每阶段验收焦点；phase3 文档明示「设计就绪稿非交付承诺」（VISION §5），防止详尽设计绑架当前节奏——这两条边界控制手段值得保持。
- 延期项全部留痕：模板/关联前移 2a 有决策日期与依据；SLA/通知/审批 defer 有 ADR 支撑；activelist 因 L1/Asynq 前置顺延有明确因果。演进平滑，无「跳阶段交付」。

### 问题

| # | 级别 | 位置 | 问题 | 建议 |
|---|------|------|------|------|
| P-1 | P1 | [00-implementation-plan.md] §门禁表/任务标题/SSOT 映射表/P2-D4 行 | Step 编号重排（2c=8-10）后自身 4 处仍用旧编号 9-11，与自述和 README 互相矛盾 | 统一为 8/9/10（工作量 10 分钟，之前审查已定位具体行号） |

---

## 7. 文档间一致性

**结论：主体一致，存在系统性回标滞后。**

抽查通过项：
- 错误码分段：errcode.go ↔ api/errcode.md ↔ 各 PRD 逐字一致，90001-90004 已正确入册；
- 路由数：router.go 16 条 ↔ 09-ticket §3 表 ↔ menu_apis 16 行 ↔ 验收断言四方吻合；
- 多租户预留：model ↔ DDL ↔ 模块文档三处口径一致（菜单故意不加 tenant_id 有专文说明）。

### 问题

| # | 级别 | 位置 | 问题 | 建议 |
|---|------|------|------|------|
| C-1 | P2 | [09-ticket.md] L274/L381/L461 三处 | 仍写「90001 尚未在 errcode.go 定义」，而代码与 errcode.md 均已实现——典型「PRD 是 SSOT 但滞后于事实」 | PR 回标三处为「已于 2a Step 2 写入」 |
| C-2 | P2 | [01-infra.md] L276 | 环境变量名写作 `DATABASE_PASSWORD`，实际有效名为 `DB_PASSWORD`（BindEnv 显式别名）——照文档设置会静默失效落回 yaml 默认值 | 修为 DB_PASSWORD，并注明 compose 场景用 APP_* 前缀、裸机用短别名两条命名通道的区别 |
| C-3 | P2 | scope 相关 3 处 | scope_resolver 路径写 `internal/service/authz/`，实际在 `internal/service/ticket/`（已于本轮回标 2 处，需确认 03-org-enhance 等剩余文档） | grep `service/authz` 收尾清零 |
| C-4 | P2 | [00-implementation-plan.md] §5.3 迁移列表 | 若仍列 000010_menu 独立行则与「已并入 000010」的实现不符 | 回标为「菜单种子并入 000010_ticket」 |

---

## 8. 测试完整性

**结论：B+。文档宣称的每个测试落点均可在代码中验证到（这在同类项目中罕见），实测 `go vet` + `go test ./...` 全绿。**

覆盖面：状态机 9 用例（另有 3 个超额差异化用例未被 PRD 登记，加分）；R3-R8 集成测试真表落点逐条对应；验收脚本 Section A 先跑 phase1 27 例回归再进 2a 用例（P2-D5 设计落实）；404/403/400 三路异常语义均有断言；Testcontainers 集成测试覆盖 4 个 repository 包 + service 包含 ticket×4。

### 问题

| # | 级别 | 位置 | 问题 | 建议 |
|---|------|------|------|------|
| T-1 | **P1** | BK-6 自认 | 组织 move 级联「后代分支」（subpath 重映射 ELSE 分支）零 Go 自动化测试，脚本仅测 fe 叶子 move。该分支恰是 scope=group 正确性根基且最易错 | 按 BK-6 承诺在 Step 5 前补：「建孙代组织+挂工单→move 祖先→断言所有后代工单 org_path 重映射」。**建议提前至 Step 4 前** |
| T-2 | P2 | 90004 ErrTicketAlreadyClosed | 错误码已定义并映射 HTTP 409，但全仓无用例触发（close 后再 assign/close）——死码风险 | 补一条集成用例断言 409+90004 |
| T-3 | P2 | 09 PRD「153 用例」口径 | 153 实为 check() 断言数，27 为编号用例数——对外易被误读 | 统一口径表述 |
| T-4 | P2 | 关联鉴权负向用例 | CreateRelation 双端 L2/L3 鉴权是实现取严的安全点，仅有脚本正向覆盖 | 补 service 层越权关联负向集成用例 |

---

## 9. 工程化配置（Makefile / 部署 / CI）

**结论：B-。覆盖面完整（build/run/dev/migrate/swag/lint/test/test-integration 全齐，gofmt 门禁已补），但有 2 处「按文档走就报错」的开箱即坏项。**

### 问题

| # | 级别 | 位置 | 问题 | 建议 |
|---|------|------|------|------|
| E-1 | **P1** | [00-implementation-plan.md §5.5] vs Makefile | 文档承诺 `make acceptance` 目标不存在（No rule to make target）——本地回归纪律的唯一入口 | 补一行：`acceptance:` + `bash scripts/acceptance-phase1.sh`（可加 acceptance2a 变体） |
| E-2 | **P1** | 两份验收脚本头部 vs dev compose | 脚本默认容器名 `zhuzhao-postgres`/`zhuzhao-redis`，实际 dev compose 是 `zhuzhao-dev-*`——按 README §1.6 主工作流（docker-dev-up→migrate-up→dev→跑脚本）执行会在第一组 psql 断言崩掉 | 脚本自动探测容器名（按 compose project name 过滤 `docker ps --format`），或在 README 要求导出 PG_CONTAINER |
| E-3 | P2 | [Makefile] docker-dev-reset | 固定 `sleep 5` 等 PG 就绪 + 使用 EOL 的 `docker-compose` v1 命令 | `up -d --wait` 或 pg_isready 循环；命令切 v2 插件 |
| E-4 | P2 | [ops/README.md] | deployment.md 待创建；生产迁移命令需手敲一次性 migrate 容器且未 make 化 | 补 deployment.md + `migrate-up-docker` 目标 |
| E-5 | P2 | 仓库根 | 无任何 CI 平台配置。文档已声明「个人项目不套 CI」属知情决策，但 GitHub Actions 免费且成本低 | 最小 workflow：lint(go vet+gofmt) + test-unit 两个 job 即可获得防回归门禁 |

---

## 10. 代码 vs 文档一致性（含判定）

| 抽查点 | 判定 | 说明 |
|--------|------|------|
| errcode 分段与文案 | ✅ 一致 | errcode.go ↔ errcode.md 逐字核对通过 |
| 工单路由 16 条 | ✅ 一致 | router.go 12+4 = PRD 表格 = menu_apis 种子 = 脚本断言 |
| config BindEnv | ⚠️ 文档侧过时 | 有效变量为 DB_PASSWORD/JWT_SECRET/REDIS_PASSWORD/APP_SERVER_MODE；01-infra 一处笔误（C-2）。**代码为准** |
| role.TenantID 多租户预留 | ✅ 一致 | DDL DEFAULT 1 + repo 0→1 兜底 + 查询无租户过滤，与「预留不实现」描述吻合 |
| Swagger 产物 | ✅（已修复） | 曾整体缺失 ticket 注解，本轮回标后 docs.go/swagger.json 含 39 处 ticket 命中 |
| 迁移编号 | ✅（已修复） | 原 000010 双文件版本冲突 blocker 已合并消除，CLI up/down 实测通过 |

---

## 修复优先级排序（合并所有维度）

```
立即（半小时内，两处开箱即坏）:
  E-1 make acceptance 目标补齐        E-2 验收脚本容器名适配
  P-1 00-plan Step 编号统一            T-1 move 级联后代分支 Go 测试（BK-6 承诺提前）

短期（Phase 2 内）:
  S-2 备份恢复最简版（pg_dump cron + Makefile backup/restore）
  S-1 审计表防篡改触发器               C-1/C-2/C-3/C-4 四处文档回标
  G-1 VISION 迁移现状过时              T-2 90004 死码用例
  F-2 is_internal 2b TODO 显式化       F-3 模板 DefaultFields 现状标注

中期（随 2b/Phase 3）:
  M-1 企业身份源接入设计占位           M-2 工单重开语义补充
  F-1 反向关联判重决策                 E-3/E-4/E-5 工程化增强
```

## 结语

本项目文档体系在 **SSOT 纪律、边界控制、延期留痕、测试落点可验证** 四个方面明显高于个人项目平均水平，接近成熟团队工程规范。主要短板集中在两类：一是「完成代码忘回标旧文」的低风险失步（全部集中在回标方向，未发现「文档新于代码导致误导实现」的反向危险）；二是运维纵深（备份、防篡改、CI）尚未跟上功能开发速度——这恰是从「demo 级」走向「生产可用级」的分水岭，建议在 Phase 2b 开发前优先偿还 E-1/E-2/S-2 三债。
