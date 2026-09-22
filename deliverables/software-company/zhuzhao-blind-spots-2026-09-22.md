# zhuzhao 全项目「未考虑问题」研究（2026-09-22）

> 委托：「对我整个项目当前的实现与设计进行研究，看看有什么没考虑到的问题」
> 方式：先建立**已登记全集**（phase4/00 的 85 处登记点 + review/11 §6/§8 + 16 号外部集成 + deliverables 三份报告），再按四维度做**代码级实证**，只输出**不在登记里的真空**。
> 基线：分支 `phase4`（HEAD `eb2ad63`，领先 origin/main 20 个提交），工作区干净。

## 0. 方法与执行说明（含失败披露）

- 派出 4 名并行审查员（性能与容量 / 可观测性与韧性 / 安全纵深 / 契约与数据治理）。
- **3 名审查员因网络中断失败**（502 ECONNRESET，非任务本身错误）：可观测性、安全纵深、契约与数据治理。
- 因此**本报告全部结论由主理人以代码级证据独立完成**，不使用任何失败审查员的产出；失败维度（可观测性/安全/契约）由主理人自行补做覆盖。
- 检索纪律：一律使用专用检索工具而非 shell `grep`——本机 BSD `grep` 存在**静默返回空**的已知坑（本轮再次触发 3 次误判，已规避）。
- **未修改任何代码或文档**（只读审查）。

## 1. 结论（TL;DR）

该项目**审查密度极高**：安全内核（JWT/黑名单/限流/审计）、并发（乐观锁四模块 + 两侧锁协议）、契约门禁（路由↔menu_apis fail-fast）等主面已被反复覆盖且**实证在位**。本轮从"整项目实现与设计"视角扫描，找到 **7 项此前未登记的真空**：

| 编号 | 问题 | 级别 | 维度 |
|------|------|------|------|
| **B-1** | 回调幂等栅栏无租约续期 / 无执行时长上限 → **长任务可被并发重认领并重复执行副作用** | **P1** | 韧性/契约 |
| B-2 | **在线迁移（DDL 锁）维度整体缺失**：49 个 `CREATE INDEX` 全部非 `CONCURRENTLY`，全库文档零处提及锁表/在线迁移 | P2 | 数据治理 |
| B-3 | `policy_evaluation_logs`（高写入量日志表）**零索引**：无 `created_at` / `trace_id` / `actor_id` 索引 | P2 | 性能 |
| B-4 | **CI 只跑单测**：集成测试与验收不进 CI；且活跃分支 `phase4` **不匹配 CI 触发列表** → 20 个提交零自动化检查 | P2 | 工程门禁 |
| B-5 | 覆盖率**度量口径失真**且**无阈值门禁**：`make test-cover` 不含 `-tags=integration` → 实测 18.7% | P2 | 工程门禁 |
| B-6 | **审计可用性缺口**：审计 INSERT 失败仅 `slog.Error`（无重试/补偿/指标/告警）；"告警"与"审计可用性"两维度未登记 | P2 | 可观测性 |
| B-7 | **登记链断裂新实例**：`golangci-lint`/`errcheck`/`gosec` 增强仅在 review/08 出现，未进活跃追踪 | P2 | 工程门禁 |

另有一项**过程性风险**（需所有者确认，见 §3）：已完成的虚标勘误审计被后续提交**静默回退**。

---

## 2. 新发现详述

### B-1【P1】回调幂等栅栏对「长任务」失效：可被并发重认领后重复执行

**机制现状**（我上一批次引入的修复，本轮复核发现其边界未定义）：

- 抢占谓词为固定 10 分钟陈旧窗口：`job_submission_repo.go:110-112`
  `... OR (status='running' AND claimed_at < NOW() - INTERVAL '10 minutes')`
- **无租约续期（无心跳）**：`claimed_at` 只在认领时写一次，执行期间不再刷新。
- `fence`（fencing token）**只保护状态写入，不保护副作用**：`jobs_callback.go:89/73` 以 `claimTs` 做 CAS（`MarkSucceeded/MarkFailed` 带 `claimTs`），所以"过期写者"不会覆盖新执行的状态——但**两个 `Handle` 都已真实执行**（`jobs_callback.go:72` handler 同步内联执行）。
- 现网唯一预置动作恰是**长任务形态**：`audit_archive` 会 `for{}` **循环至积压清空**（`audit_archive.go:142-171`，每批 5000 行且逐批 `f.Sync()`），首次运行处理历史积压时**轻易超 10 分钟**。
- 加剧因素：服务端 `WriteTimeout: 60 * time.Second`（`app.go:45`）使任何 >60s 的 `Handle` **无法送达 2xx**；而 taskrunner 侧契约是"5xx/**超时自动重试**"（`docs/phase3/16-external-integration.md` §1.1）→ 长任务的回调重试是**必然发生**，不是假设。
- 组合结果：`0–10min` 内重试被栅栏正当拦下；**一旦原执行跨过 10 分钟，下一次重试即抢占成功** → 同一 action 双执行并发跑同一张表的导出+删行。

**影响**：`audit_archive` 当前损失有限（"先导出后删行" + 按 id 删，重复导出可容忍）；但 registry 允许注册任意 action，**未来非幂等动作（HR 同步/通知/写外部系统）会双执行**。即"Handler 必须可重入"这条**隐式契约既未文档化、也无代码约束**（`jobs_callback.go:91-92` 只在注释里假设）。

**是否在册**：**否**。文档只把 10 分钟重认领写成"自愈特性"（`docs/phase2/README.md:255`、`docs/review/11-project-control.md:123`），**从未出现**"长任务被抢占""执行时长上限""租约续期""可重入契约"等表述（全 docs 检索 `claimed_at|重认领|租约|心跳` 无相关登记）。

**建议修法**（任一即可闭合，推荐 a+b）：

- **a. 心跳续租**：`Handle` 期间起一个协程周期（如 2min）`UPDATE claimed_at = NOW() WHERE task_id=$1 AND status='running'`，租约仅对"静默"任务过期。
- **b. 按动作声明租约/时长上限**：registry 注册时带元数据（如 `audit_archive` 声明 30min 租约），陈旧判定用该值而非全局常量。
- **c. 单批一回调（最简）**：把归档改为"每次回调只处理 N 批"（工作量有界），由 taskrunner 周期重排——同时天然消除 60s `WriteTimeout` 冲突。
- **d. 补文档+测试**：把"Handler 必须可重入"从注释提升为 registry 契约要求，并加一条"长任务 + 并发回调"的集成测试（现有 8 并发测试用的是短任务，**测不出该窗口**）。

### B-2【P2】在线迁移（DDL 锁）维度整体缺失

- 事实：`migrations/` 共 **49 处 `CREATE INDEX`，`CONCURRENTLY` 出现 0 次**；`ALTER TABLE ... ADD COLUMN` 亦无锁时间评估。
- 事实：全 `docs/` 检索 `CONCURRENTLY|锁表|在线迁移|online migration` → **零命中**（该风险从未进入任何文档/登记/清单）。
- 影响：项目已进入"多实例 + 持续部署"规划（phase4 有部署原子性/fail-fast 讨论），而任何 `CREATE INDEX`（含我上一批的 000028 唯一索引 + 全表 `UPDATE` 去重）在**业务运行中**执行都会阻塞该表写入。目前数据量小而无感，属"未来会踩"的定时问题。
- 另有数据治理细节：000028 的**去重软删不可逆**（down 不复活），且迁移本身不产出"将影响多少行"的预检报告。
- 建议：① 迁移规范补一条"大表索引改 `CONCURRENTLY`（并注意其不可在事务内执行的约束）"；② 加"迁移前预检行数/锁时间"的运行手册条目；③ 明确迁移执行窗口（停机 vs 在线）。

### B-3【P2】`policy_evaluation_logs` 零索引

- 事实：该表定义（`migrations/000020_policy_eval_request_id.up.sql`）**除主键外无任何索引**——没有 `created_at`、`trace_id`、`actor_id`、`resource_type` 索引（对照：`audit_logs` 有 `idx_audit_logs_created`（000009:26），`ticket_events` 有 `(ticket_id, created_at)`（000010:81）——**唯独这张表没有**）。
- 写入量级：该表是 L2 判定日志，**每次 L2 判定一行**（`audit_log_repo.go:147` `InsertPolicyEvals`，批量 200/次），是增长最快的一张表之一；且已配 180 天归档任务在扫它（`audit_archive.go:91`）。
- 影响：按 `request_id`（`trace_id`）反查——**这正是 request_id 全链贯通的设计意图**（`internal/pkg/reqid/reqid.go:3` 明确"同键"）——以及任何时间范围查询都退化为**全表扫描**；归档批查询 `WHERE created_at < ?` 同样无索引支撑。
- 建议：至少补 `(created_at)`（归档/时间查询）与 `(trace_id)`（跨表按 request_id 反查）；如需按人审计再加 `(actor_id, created_at)`。

### B-4【P2】CI 覆盖面：只跑单测，且活跃分支不在触发列表

- 事实：`.github/workflows/ci.yml` 的 `test` job 只跑 `go test -race -count=1 ./internal/...`——**不带 `-tags=integration`**；脚本自述"验收脚本需 Docker/Compose，仍按本地验收纪律人工执行"。
- 事实：触发分支为 `[dev, main, "feature/**"]`；但**当前活跃开发分支是 `phase4`**（领先 main 20 个提交，无 upstream）→ **这些提交从未经过任何自动化检查**。
- 影响：① 项目真正的正确性网（并发/迁移/集成测试，含我上一批新增的 4 个并发测试）**在 CI 中完全不执行**，只在本机跑；② 推送 `phase4` 静默跳过 CI，容易形成"绿了但没检查"的错觉。
- 建议：① 把 `phase4`（或 `release/**` 形态）加入触发分支，或改按"所有分支"触发；② CI 增加 `-tags=integration` 作业（GitHub Actions 原生支持 service containers 提供 PG/Redis，可跑起集成测试）。

### B-5【P2】覆盖率口径失真 + 无阈值

- 事实：`make test-cover` = `go test -race -coverprofile=coverage.out ./internal/...`（**无 integration tag**）；把该产物交给 `go tool cover -func` → **total 18.7%**。
- 事实：37 个集成测试文件全部在 `//go:build integration` 之后，其覆盖的正是并发/迁移/仓储等核心路径 → **18.7% 严重低估真实覆盖**，用它"看覆盖率趋势"（`docs/review/11-project-control.md` §5 的使用建议）会得出错误趋势。
- 事实：Makefile 无覆盖率阈值门禁（`grep coverprofile` 仅 test-cover 一处）。
- 建议：① 覆盖率分两档统计（单测 / 含 integration）；② 设一个非阻断的趋势阈值（如核心包不低于当前基线）以防回退。

### B-6【P2】审计可用性缺口 + 可观测性"告警"维度未登记

- 事实：审计写入失败路径**只 `slog.Error` 后返回**，无重试、无补偿队列、无指标、无告警：`middleware/audit.go:86-88`、`service/audit_service.go:54-56`。注释关注点是"不随断连丢弃"（F-5/WithoutCancel）与"停机 drain"，**未覆盖"INSERT 失败怎么办"**。
- 事实：全仓**无任何 metrics/告警基础设施**——`go.mod` 无 prometheus/opentelemetry，无 `/metrics` 路由，无告警通道代码。其中"指标/追踪"已登记为待实施（M1 可观测基座 + `/system/info` 提案），但**"告警规则/阈值/通知/值班"与"审计写入可用性"两项从未登记**。
- 影响：DB 抖动导致审计静默丢失时，**既无指标、也无告警**，只有一条 error 日志；对"审计是合规刚需"的系统，这是可用性（≠ 防篡改 F-23）层面的真空。
- 建议：把"审计写入失败"从"记日志"升级为可观测事件（计数指标 + 失败率告警 + 可选本地补偿文件），并在 M1 基座里补"告警"一节（当前只有采集）。

### B-7【P2】登记链断裂新实例（与 phase4/00 §8.1 同型）

- 事实：`golangci-lint`（errcheck/gosec）增强项仅出现在历史报告 `docs/review/08-final-recheck-and-new-findings.md:110`（第 15 条），在 `review/09`、`review/11 §6/§8`、`phase4/00` 等**活跃追踪入口全部缺席**。
- 意义：这正是 phase4/00 §8.1 定义的"评审登记后从追踪链脱落"模式的**第 4 个实例**（前三个：F-23 防篡改、依赖漏洞扫描、密码历史）。说明该模式不是偶发——**活跃登记入口缺一个"历史审查项回收"机制**。
- 建议：把"golangci-lint 最低集"并入 phase4/00 §4 一次性拍板项；并考虑给"登记入口"加一条核对纪律（新 phase 盘点时对上一 phase 报告逐条判"已落/已关/脱落"）。

---

## 3. 过程性风险（需所有者确认，非代码缺陷）

**已完成的一次审计批被后续提交静默回退。**

- `eab4535`（"虚标审计批"）在 15 处加了 ⚠ 勘误；其中 4 处随后被 `25bda2e`（"十九批全生态对照治理批"）**删除**：
  - `docs/design/design-decisions.md`：§9.4 JWTManager 算法切换构造器、§12 `ResourceAuthorizer` 接口、§13.6 `UserQueryService` 的三处勘误块被移除，**原文恢复为"接口预留/设计稿"的正面表述**；
  - `docs/phase2/09-ticket.md:515`：`~~hooks.go~~（该文件从未存在）` 被改回 `hooks.go`。
- **证据**：`git log -S "虚标审计勘误" -- docs/design/design-decisions.md` 显示 `eab4535` 添加、`25bda2e` 移除；`25bda2e` 仅改动这两个文档共 18 行，但其**提交信息只声明"design-decisions 新增 §26.6"，未披露删除了勘误**；`find . -name hooks.go` → 无此文件（**幻影文件确凿**）。
- **后果**：同一类声明现在**跨文件自相矛盾**——`standards.md:83`、`11-authz:142/190`、`modules/ticket.md:658` 仍带"未落码"勘误，而 `design-decisions`/`09-ticket` 已恢复为正面表述。对"以 docs 为 SSOT、靠勘误纪律维系可信度"的项目，这会让读者按文件得到相反结论。
- **待确认**：这是**有意的编辑（如认为勘误标注方式不妥，拟改为集中登记）还是误覆盖（基于旧副本编辑导致回退）**？若有意，建议改为"集中勘误索引 + 原文保留"的形态并一次性落档；若无意，建议按 `eab4535` 恢复并加提交信息纪律（提交不得静默回退审计结论）。

---

## 4. 复核后确认"已在位"（不必重做，避免重复排查）

以下项本轮**逐条实证在位**，与文档声明一致：

| 项 | 证据 |
|----|------|
| 连接池/超时配置完整（含 `statement_timeout`） | `configs/config.yaml` database/redis 段；`internal/app/providers.go:54-58` 正确接线 |
| 健康检查真探依赖 | `router.go:99-108`（`/health/ready` 实探 PG + Redis，分别返回 component） |
| 限流覆盖广（非仅登录） | `router.go:130`（auth 组）、`:140`（authed 全量）、`:309`（网关反代） |
| Redis 故障 **fail-close**（非 fail-open） | `ratelimit.go:89-92` → 503+10008；`jwt.go:67-84` 黑名单/disabled 查询失败 → 503 |
| HTTP 服务端硬化 | `app.go:43-46`（ReadHeader/Read/Write/Idle 四超时）+ `BodyLimit(1MB)` |
| 审计防"断连规避" | `middleware/audit.go:81-84`（`WithoutCancel` + 3s 独立超时）+ 请求体截断 2048 |
| 乐观锁覆盖（文档所称"四模块"属实） | `user_repo.go:224`、`role_repo.go:163`、`menu_repo.go:146`、`org_repo.go:331` 均 `AND version=$n`；工单三表见 000027 |
| P2-1 两侧锁协议在位 | Delete `FOR UPDATE` 认领 + 4 处 `FOR SHARE` 复核（`org_repo.go:107/154/238/564` 一带） |
| 回调 fence（过期写者保护）设计正确 | `jobs_callback.go:73/89` 带 `claimTs` 做 CAS，`!updated` 时不覆盖新执行 |
| schema 快照/迁移纪律 | `scripts/snapshot.sh` + Makefile `snapshot-diff`；集成测试重建全量迁移 |

## 5. 无法判定 / 建议人工确认

1. **L2 行级鉴权仅接线 ticket 资源**：全仓生产代码 `resource.Authorize` 仅 1 处调用（`service/ticket/service.go:61`），其余全在测试。这与已登记的 S-2/BK-21 一致（"策略库消费方清零、触发驱动"），**属设计内**；但若前端把非管理员角色开放到用户/组织/角色管理面，需重新评估横向越权面（建议在前端启动批回看一次）。
2. **软删恢复 / 备份恢复演练**：org 恢复在信号表里是"顺带清 org_roles 孤儿行"的触发项，但**"恢复流程本身"（谁、怎么恢复、如何验证）无成文 runbook**；taskrunner PG 备份口径仅 closure-report §五 一处冷门登记。建议并入 M1/ops 一次补齐。
3. **`policy_evaluation_logs` 是否已有读 API**：本轮仅确认"无索引"；是否存在面向运维的读端点未逐一核对（若无，则 B-3 的紧迫性取决于归档查询）。

---

## 6. 建议处置顺序（低成本高收益优先）

1. **B-1**（P1）：先补一条"长任务 + 并发重试"集成测试复现窗口，再按 a/c 任一闭合（1 天级）。
2. **B-6 + B-7**：两项都是"登记一笔 + 小幅实施"，可与 phase4/00 §4 一次性拍板项同批（半天级）。
3. **B-4 + B-5**：CI 触发分支与 integration 作业、覆盖率双档（半天级，收益面广）。
4. **B-3**：补 2 个索引迁移（下次迁移取号 000030 之后，注意 B-2 的 `CONCURRENTLY` 规范同批定）。
5. **B-2**：规范级（迁移编写规范 + 运行手册），文档改动为主。
6. **§3 过程性风险**：需所有者先定性"有意/误覆盖"，再决定恢复或改形态。

---

*本报告为只读研究产出：未修改任何代码或文档；所有结论均可按 citing 的 `file:line` 独立复核。*
