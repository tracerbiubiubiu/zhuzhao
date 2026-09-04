# 16 - 外部服务集成配套：taskrunner / activelist（zhuzhao 侧能力清单与实施细排）

> **定位**：M-E（taskrunner）与 M-A（activelist）的契约已于 2026-09-03 在**外部独立仓库**定稿。本文把「**zhuzhao 需要提供什么能力**」归拢为：契约要点摘要 + 代码现状对照 + 能力清单（M-E 线 / M-A 线）+ 里程碑对齐 + 迁移/权限码规划 + 待拍板项——供 zhuzhao 侧排期与实现，防跨仓库工作项遗漏。
> **契约 SSOT**：taskrunner = taskrunner 仓库 `docs/taskrunner.md`（配套清单 `docs/zhuzhao-integration.md`）；activelist = activelist 仓库 `docs/ADR-003-integration-contract.md`。**排期 SSOT 仍为 [13-implementation-plan](./13-implementation-plan.md) §1**；权限前置 SSOT = [design-decisions §25.5](../design/design-decisions.md)。
> **状态**：已编写（2026-09-03，基于两仓库契约文档 + zhuzhao 全仓代码盘点）。**编号约定**：本文 E1–E6（taskrunner 配套，对齐 zhuzhao-integration §2 编号）、D1–D5（activelist 能力需求，与 ADR-003 SSOT 同名同义）、P1–P7（待拍板）均**仅在本文范围内引用**。

---

## 0. 结论一句话

zhuzhao 地基已有大半（三层鉴权链 / RequestID / `audit_logs` / L1 `ticket_events` / resource 注册表含 IW4 护栏 / **zhuzhao-utils v0.1.0 已发布并 pin**），**两大块零起步**：① M-E 线的**内网回调端点体系**（`/internal/*` 路由组 + `action_id` 注册表 + 幂等）与**任务管理代理**；② M-A 线的**网关化**（反代 + 身份断言 + 限流 + Restrict，即 §25.5 批次 B）。首个预置动作 = 审计归档（B11②，[03](./03-audit-l2.md) §4）。

---

## 1. 契约要点摘要（防跳转）

### 1.1 taskrunner（M-E，事件/任务总线）

- **形态**：独立仓库 / 独立部署 / 独立 Redis / 独立 DB（SQLite 起步）；zhuzhao 作网关（鉴权/编排/业务审计），**只经 HTTP API 对接**（不 import 代码、不直连 Redis）。
- **一句话分工**：zhuzhao 负责「决定要做什么」（发起、下发、按需查），taskrunner 负责「可靠地去做」（调度、重试、死信、记录、看板）。
- **三层模型**：动作（`action_id` + handler，**在 zhuzhao 代码**）→ 任务定义（job，存 taskrunner DB，API 管理）→ 执行实例（run，`job_runs`，taskrunner 自有）。
- **回调契约**：taskrunner 按 job 定义回调 `POST /internal/jobs/<action_id>`（task_id + request_id + params，HTTP 头带 `X-Request-ID`）；**at-least-once——幂等是 zhuzhao 侧义务**（按 task_id+request_id 先查重）；5xx/超时自动重试、4xx 不重试、**2xx = 执行完全成功（P7 定案：无状态字段，业务失败映射 4xx/5xx）**；执行结果**只经查询接口获取，不推送**；**回调带 AK/SK 签名（2026-09-03 基线修订，覆盖 P5 原拍板——见 §9）**。
- **日志边界**：zhuzhao 记「任务提交日志」（`{action, task_id, request_id}`，薄）+ 业务审计（`audit_logs`）；taskrunner 自维护 `job_runs`（细节不回传）；两边以 `request_id` 关联跨查。
- **不做**：用户上传脚本（🚦 按需再启，选型见 [15](./15-script-platform-dagu-vs-inhouse.md)）；业务 handler；事件源（事件事实源仍是 zhuzhao L1）。

### 1.2 activelist（M-A，动态数据模型平台）

- **形态**：独立服务 + 独立库 + **独立数据库**（与工单库故障隔离）；**零认证薄层**（无网关不能对外暴露）；进程 3→1（仅 apiserver）。
- **职责收敛后唯一职责**：类型注册 / Schema 演进（方案 D：单一当前版本、数据行不带 `schema_version`、破坏性变更懒执行）/ 动态字段校验 / 数据 CRUD / 存储（PG 每类型表 + `data` JSONB、id 自增、乐观锁、软删保留）；导入幂等 = **全量替换**（单事务清表重插、保留源 id、不需要业务唯一键）；查询 = 仅 id 分页 + 时间倒序。
- **移交 zhuzhao**：事件（zhuzhao 业务操作点显式发布，activelist 不感知）与审计（activelist 写接口返回变更后完整文档，zhuzhao 侧记录，落点机制 ⚠️ 待拍板）。

---

## 2. zhuzhao 代码现状对照（2026-09-03 盘点）

### 2.1 已有积木（可直接复用）

| 积木 | 位置 | 复用点 |
|---|---|---|
| 三层鉴权链（JWT → CasbinAuth → Resource.Authorize） | `internal/router/router.go` / `internal/middleware/` / `internal/pkg/resource/` | 任务管理端点、反代路由的 L1 判定（§25.1：反代路由同样过 CasbinAuth） |
| RequestID 中间件（`req-`+32hex，接受入站透传） | `internal/middleware/logger.go:15` | 任务提交关联键：zhuzhao 生成 → 透传 taskrunner → 回调带回 → `job_runs` 落档 |
| 审计中间件（同步写 `audit_logs`，action 注册） | `internal/middleware/audit.go` | 任务提交/建改定义的业务审计；网关层请求级审计（proxy 跳 body 为新增开关） |
| L1 事件（`ticket_events`，与业务同事务写） | `internal/repository/ticket_repo.go` | 回调 handler 产生的业务事件照常落 L1（契约 §4） |
| resource 注册表 + IW4 行级护栏（Filter 哨兵 + Unscoped + AST 守护） | `internal/pkg/resource/` / `internal/service/ticket/resource.go` | 平台策略库（批次 A）的承载底座；`Builtin()` 注册即接入 |
| zhuzhao-utils v0.1.0（9 包：crypto/errcode/jsonutil/jwt/logger/postgres/redis/response/validate，已 pin 无 replace） | `go.mod` + 独立仓库 | **D1 已完成**；taskrunner/activelist 引用同源（taskrunner 用 logger/errcode/response；activelist 用 logger/postgres） |
| 权限码 / 菜单 / `menu_apis` 体系 | `internal/service/menu*` / 迁移 seed | 任务管理端点、activelist API 的权限码与菜单登记 |

### 2.2 taskrunner 六项配套 vs 现状

| # | 契约要求（zhuzhao-integration §2） | 现状 | 缺口性质 |
|---|---|---|---|
| E1 | 动作 handler 注册表 + `POST /internal/jobs/:action_id` 回调端点（幂等：task_id+request_id 查重）【M3 前必做】 | 路由仅 `/health/*` + `/api/v1/*`；中间件链全为用户 JWT 体系 | **全新**：内网路由组（**AK/SK 验签**——utils `aksk` 验 taskrunner 签名，2026-09-03 基线修订）+ 注册表 + 幂等表 |
| E2 | ~~动作清单端点 `GET /internal/jobs`~~ | 无 | ✅ **已定案不做前置校验**（taskrunner M2 定案：不存在/未注册的 action 经回调 4xx 快速失败；清单端点降为可选增强，无消费方不建） |
| E3 | 任务管理功能：提交/建改定义/手动触发/查记录的 API + 页面（代理 taskrunner API；request_id 在此生成透传；写接口带 actor 工号 + source_ip） | 无 taskrunner client、无任务管理端点 | **全新**（鉴权/审计/RequestID 积木可复用） |
| E4 | 部门可见性策略：策略模型 + zhuzhao 自有表 + 管理功能；三层校验消费；决定传给 taskrunner 的 `dept` 过滤参数与写权限 | **无 dept 概念**（可见性是工单专属三轴，§25.3 双路不合流） | **全新，最大语义 gap**（P1） |
| E5 | 任务提交日志 `{action, task_id, request_id}` 落 zhuzhao（薄） | 无 | 新表（可与 E1 幂等查重合并一张） |
| E6 | 终败通知端点 `/internal/notifications/task-dead` | 无 | **后置不做**（契约明确：仅当启用终败通知时需要） |

### 2.3 activelist 能力需求（D1–D5，ADR-003 SSOT）vs 现状

| # | 能力需求 | 现状 | 阻塞关系 |
|---|---|---|---|
| D1 | 共享 utils（logger/postgres 硬依赖；errcode/response/jsonutil/validate/crypto 按需） | **✅ 已完成**（v0.1.0 已 pin；`internal/pkg` 仅剩 errcode 别名 + resource——resource 按 §25.3 拍板留 zhuzhao 不抽） | 无 |
| D2 | 反向代理 + header 透传（E13：`app/service/proxy/` + `SetForwardHeaders` + Restrict 资源 `activelist` + accesslog 跳 body） | **零起步**（全仓无 ReverseProxy/SetForwardHeaders；Restrict 中间件不存在） | 不阻塞开发；**阻塞联调与上线**（activelist 零认证） |
| D3 | 业务审计记录（activelist 写接口返回变更后文档；zhuzhao 侧落审计；导入按批次） | 无 | 不阻塞开发；阻塞审计闭环验收（落点机制 ⚠️ P2） |
| D4 | 事件发布（zhuzhao 业务操作点显式发布） | 依赖 M-E 就绪 | 无硬依赖（activelist 不感知事件） |
| D5 | 网络隔离（双 network，仅 zhuzhao 容器可达 apiserver） | 部署层 | 部署期事项（activelist M-A6） |

---

## 3. M-E 线：zhuzhao 侧能力清单（按依赖序）

| 项 | 内容 | 依赖 / 挂靠 | 量级 |
|---|---|---|---|
| **前置 · 批次 A** | 平台策略库（`org-member`/`owner-only`/`role-gated` + `Builtin()` 注册 + schema 约定 fail-fast + 正负向测试）——承载任务提交/回调端点判定 | §25.5 / [authz.md §3.1](../modules/authz.md)；与 taskrunner 侧开发并行 | 2–3 天 |
| **E-①** | ✅ **已实施（2026-09-04）**：迁移 000020（判定日志表 + 两表加列）+ reqid ctx 注入 + Casbin 打点补 rid + registry EvalHook 埋点 + L2 writer（P3 管道）——[03 §3](./03-audit-l2.md)；B11② 归档前提已就绪 | 已完成 |
| **E-②** | ✅ **已实施（2026-09-04）**：`/internal/jobs/:action_id`（AK/SK 验签 utils `aksk` + config `internal_jobs`（默认关，SK 缺失拒启））+ `internal/pkg/jobs` 注册表 + `job_submissions` 一表两用（000021：提交凭证 + 回调幂等栅栏——succeeded 拦重复、failed 容重试）+ P6/P7 契约落地（未知动作 404 / ErrAbort→409 / 其他→500）；utils 暂以 go.mod 本地 replace 引用（发 v0.2.0 后删除） | 已完成（audit_archive 注册随 E-③） |
| **E-③** | 首个预置动作 `audit_archive`（B11②）：JSONL 导出 → 成功后按批删行；保留期默认 180 天可配；每任务阻塞/去重 | E-②；存储位置 P4；[03 §4](./03-audit-l2.md) | 0.5–1 天 |
| **E-④** | taskrunner client + 任务管理端点（E3）：提交/建改定义/触发/查询全过三层校验后代理；**request_id 从 ctx 取入站 rid 透传（含 body 与 `X-Request-ID` 头，03 §3.4 全链路关联）**；写接口透传 actor+source_ip；任务管理权限码/菜单 seed | taskrunner M2 API 就绪；E4 | 1–2 天 |
| **E-⑤** | 部门可见性策略（E4）：策略表（000022，按 P1 拍板定形态）+ 管理端点 + 消费逻辑 | P1 | 1–2 天 |
| E-⑥ | 终败通知端点（E6） | 🚦 后置 | — |

> zhuzhao 侧 M-E 配套合计约 **5–7 人日**（不含批次 A 与 taskrunner 仓库自身 M1–M4）；13 号 M-E 行的 3–4 人日指 taskrunner 侧核心运行时，两侧并行。

## 4. M-A 线：zhuzhao 侧能力清单

| 项 | 内容 | 依赖 / 挂靠 | 量级 |
|---|---|---|---|
| **前置 · 批次 B** | 网关化：反代核心（前缀→上游注册表 / ReverseProxy / 错误映射）+ 身份断言（§25.2 **方案 A：AT 原样透传 + activelist 持公钥验签**——activelist 侧需配合加验签层，需同步登记到其契约文档）+ `SetForwardHeaders` + **Restrict 中间件（新建）** + 资源 `activelist` + API 级限流（复用 [07 §2](./07-security-enhance.md) 设计）+ activelist API 入 `menu_apis` + proxy 审计跳 body | §25.5 / ADR-003 D2；与 activelist 侧开发并行 | ~1 周 |
| **D-②** | D3 业务审计：**P2 已拍板**（SSOT = activelist ADR-003「审计落点机制」专节）——client 封装层同请求路径同步写 `activelist_audit_log` 表 + 失败落本地重投队列；`X-Request-ID` 优先透传入站 rid（03 §3.4）；脱敏/水位对账风险接受（钩子已预留） | 批次 B | 1–2 天 |
| D-④ | D4 事件发布：zhuzhao 网关上对 activelist 的写操作成功后显式发布（M-E 就绪后接） | M-E | 随用 |
| D-⑤ | D5 网络隔离：docker-compose 双 network | 部署期（activelist M-A6） | 部署项 |

> D1 已完成收尾：ADR-003（activelist SSOT）D1 状态行与 zhuzhao 侧镜像已于 2026-09-03 同步为「已完成」。

---

## 5. 里程碑对齐（taskrunner 侧 M1–M4 × zhuzhao 配套）

| taskrunner 里程碑 | zhuzhao 侧需要就绪 |
|---|---|
| M1 核心运行时（入队→回调→记录） | ✅ **已完成（2026-09-03）**；A1 提交幂等扩为终身 / A2 cancel 竞态防护已修（commit 03dab52） |
| M2 HTTP API（v1 全端点 + credential） | ✅ **已完成（2026-09-03）**；~~E2 选型定案~~（P6 已关：不做前置校验）；zhuzhao 侧待 E-④/E-⑤（联调依赖），B2 归因打点小改随 M3 |
| M3 首个预置动作（audit_archive 闭环） | E-②（`audit_archive` handler + `/internal/jobs/:action_id`）+ E5 提交日志；对齐 [03 §4](./03-audit-l2.md) 验收 AL5/AL6 |
| M4 运维完善（指标/死信告警/asynqmon `/monitor`） | 无 zhuzhao 侧依赖（可并行） |

**M-E 验收入口** = 归档任务按周期跑通 + 「触发 → 回调执行 → 失败重试」闭环 + 全门禁绿（13 号 §1 M-E 行）。

## 6. 新增迁移与权限码规划（编号启动时按 A2 核对，Phase 3 起 000020）

| 迁移（建议） | 内容 | 归属 |
|---|---|---|
| 000020 | `policy_evaluation_logs`（B11① 判定日志，03 §3.2 DDL 草案）+ **`audit_logs` / `ticket_events` 各加 `request_id` 列**（03 §3.4 全链路关联，一次迁移合并） | M-E / M1 |
| 000021 | 任务提交日志 + 幂等表（`{action, task_id, request_id, ...}`，E1/E5 一表两用） | M-E |
| 000022 | 部门可见性策略表（按 P1 拍板定形态） | M-E |
| seed | 任务管理权限码（如 `task:submit` / `task:manage` / `task:read`，命名随实现定）+ 菜单 | M-E |
| — | activelist 侧表全部在 activelist 自有数据库（zhuzhao 零迁移） | M-A |

## 7. 待拍板项（P1–P7，实现前确认）

| # | 决策项 | 建议 |
|---|---|---|
| P1 | ~~dept 归属标签语义~~ | ✅ **已拍板（2026-09-03）：复用 org 树**——org code 即标签值，新建策略表存「org/角色 → 可见标签集」映射，用户多组织按并集；不新造部门维度；org code 变更即标签变更（低频、可追溯、随 M-HR 对齐） |
| P2 | ~~D3 activelist 审计落点~~ | ✅ **已拍板（SSOT = activelist ADR-003「审计落点机制」专节）**：client 封装层同请求路径同步写 `activelist_audit_log` 表 + 失败落本地重投队列；X-Request-ID 由 client 层生成透传；导入/导出按批次行；脱敏暂不做（风险接受，E13 accesslog 跳 body 使敏感值只落审计表一处）；~~水位对账~~ 暂缓风险接受（version 单调递增，将来补对账成本低） |
| P3 | ~~03 号 U1/D2 审计管道~~ | ✅ **已拍板（2026-09-03）：异步写**——channel → Redis List（AOF）→ 批量落库 goroutine（03 号 §2/§7 D1 已同步；选 Redis 而非纯协程管道：鉴权链对 Redis 本就 fail-close，writer 依赖零新增风险、持久化免费）；fail-open 随 E-① 实现确认。**附带登记关联键缺口（03 号 §3.4）**：request_id 现未注入 request context、audit_logs 无该列——E-① 需补（trace_id = request_id），打通 taskrunner job_runs 跨查链 |
| P4 | ~~03 号 D3 归档存储位置~~ | ✅ **已拍板（2026-09-03）：本地 JSONL（Docker 卷）+ 纳入宿主卷备份**，对象存储后置；注意点已登记：删库重建场景归档不随 PG dump 回来，180 天等保口径靠卷备份覆盖——入 M-Mig 部署清单 |
| P5 | ~~回调鉴权机制（taskrunner → zhuzhao `/internal`）~~ | ~~✅ 已拍板（2026-09-03）：不做独立机制~~ → **同日被 AK/SK 基线修订覆盖**：回调带 HMAC 签名（§9）；信任 = 签名 + 专用 network 双防线；capability URL 增强随之作废 |
| P6 | ~~E2 动作清单校验方式~~ | ✅ **已定案（taskrunner M2）：不做前置校验**——不存在的 action 经回调 4xx 快速失败（non-retryable，failed 可见）；清单端点保留为可选增强 |
| P7 | ~~2xx 受理 / 业务失败的响应体状态字段~~ | ✅ **已拍板（2026-09-03）：不要状态字段**——zhuzhao handler 直接用 HTTP 状态码表达：2xx 仅在执行完全成功时返回；业务失败按可重试性映射 4xx（不可重试）/ 5xx（可重试），走常规 errcode → response 映射。taskrunner M2 代码已符合（2xx/4xx/5xx 判定），仅注释随下次提交更新；zhuzhao handler 侧零新增约定。**B11② 原子语义由「导出失败返回 5xx 重试 + task_id 幂等」保障** |

> P1–P7 与 13 号 §9 不确定项并存；03 号 §7 的 D1–D5 归 03 号文档本地编号，注意区分。**P1–P7 已全部关闭（2026-09-03 拍板/定案同步）**；M-E/M-A 动工前的决策面清零，剩余均为实施项。

## 8. 与既有文档的边界（防重复）

| 主题 | SSOT | 本文角色 |
|---|---|---|
| taskrunner 设计 / API / job_runs | taskrunner 仓库 `docs/taskrunner.md` | 摘要 + zhuzhao 侧展开 |
| activelist 契约 / 存储模型 | activelist 仓库 `docs/ADR-003-integration-contract.md` | 摘要 + zhuzhao 侧展开 |
| 排期（里程碑 / 人日 / 主链顺序） | [13 号](./13-implementation-plan.md) §1 | 细排参考（M-E/M-A 行引用本文） |
| 权限前置（批次 A/B/C） | design-decisions §25.5 | 引用 |
| 平台策略库设计 | [authz.md §3.1](../modules/authz.md) | 引用 |
| B11①② 审计设计 | [03 号](./03-audit-l2.md) | 引用（E-①/E-③ 为其在 M-E 线的落位） |
| 脚本任务选型（🚦） | [15 号](./15-script-platform-dagu-vs-inhouse.md) | 引用 |

## 9. 公共能力统一基线（内部能力服务共用，2026-09-03 所有者拍板）

> taskrunner / activelist（及将来的同级内部服务）公共能力**策略必须一致**，差异只允许来自职责/量级。**服务间鉴权统一为 AK/SK HMAC 签名 + 专用 network 双防线**（2026-09-03 AK/SK 基线修订，覆盖当日早前「零认证 + 拓扑」口径与 P5「回调不做鉴权」原拍板；见下表服务鉴权行）。基线本文为 SSOT，两服务仓库文档引用。

| 能力 | 统一策略 |
|---|---|
| 服务鉴权 | ✅ **AK/SK HMAC 签名 + 专用 network 双防线**（2026-09-03 拍板，覆盖当日早前「零认证+拓扑」口径）：服务间通信一律签名/验签（utils `aksk`，按调用方发 SK：zhuzhao/taskrunner/activelist 各一把，env 注入）；专用 network 仍保留（攻击面收敛 + 第二道防线）；用户鉴权全部在 zhuzhao 网关 |
| 身份断言 | ✅ **明文 `X-Operator` 头 + 纳入签名覆盖**（B2 关闭）：明文断言经 HMAC 覆盖后不可伪造；§25.2 方案 A（AT 验签）不需要、降为触发条件驱动（服务可达面扩大 / 服务自行做行级判定 / 合规要求） |
| **通信协议** | ✅ **已拍板（2026-09-03）：HTTP + JSON**（zhuzhao→taskrunner API、taskrunner→zhuzhao 回调、zhuzhao→activelist 反代/编排层）；**gRPC 不引入**（覆盖旧「gRPC 内部 + REST 外部」预留表述） |
| 用户权限 | 零（不建角色/权限模型，不引用策略库——§25.3 消费者分析） |
| 日志框架 | zhuzhao-utils `logger`（slog + lumberjack，JSON Lines，字段稳定命名供 ES 演进） |
| 请求级访问日志 | **统一中间件出口**，每请求一行（method / path / operator / trace_id / 请求参数 4KB 截断 / 结果状态）+ 响应回显 `X-Request-ID`；`X-Operator` 缺失兜底 `"system"`；脱敏钩子预留（启用只改一处） |
| request_id 协议 | `X-Request-ID` 头进出全程透传（与业务 body 的 request_id 并存：taskrunner job_runs / activelist trace_id 同键） |
| 响应结构 | utils `errcode` + `response` |
| 业务审计 | 零业务语义日志；审计正本全在 zhuzhao（任务提交日志 / `activelist_audit_log`） |
| 事件 | 服务自身不发不订；事件事实源 = zhuzhao L1，异步执行 = taskrunner 总线 |
| 健康检查 | `/healthz` + `/readyz`（检各自硬依赖：Redis / PG） |
| 时区 | 容器固定 `TZ=Asia/Shanghai`（cron/时间语义一致） |
| 迁移 | 各自独立编号，与 zhuzhao 迁移号无关 |
| 配置 | yaml + `${VAR}` 环境变量展开（对齐 zhuzhao 模式；敏感值注入 env） |

**允许差异**（职责/量级决定，非策略分歧）：~~存储选型~~ ✅ **已拍板统一 PG（2026-09-03）**——taskrunner job_runs 迁 PG（各自独立数据库实例/库不变，复用 utils `postgres`，schema 不变；SQLite 保留为 M1/M2 已交付实现，C7 切换）；Redis·Asynq（taskrunner 需要 / activelist 无）；副本数（PG 后均可多副本，按运维需要）。

**taskrunner 对齐改动清单**（activelist 已符合基线，taskrunner 侧待实施）：

| # | 改动 | 时序 |
|---|---|---|
| C1 | 统一访问日志中间件（含 `X-Request-ID` 读头/回显 + `X-Operator` 兜底 `"system"`）——顺带关闭 B2（logOp 归因缺口） | M3 随手 |
| C2 | API 鉴权换 **AK/SK 验签**（静态 Bearer → HMAC 签名验签；utils `aksk`，覆盖 M2 credential 定案；**不再依赖 C3 时序**——签名即密码学防线，拓扑降为第二道防线） | 随 C8，M3 |
| C3 | 部署网络隔离：compose 双 network（服务端口仅挂 zhuzhao 专用网络，对齐 activelist D5 模式） | M3 联调 / M4 部署 |
| C4 | `/readyz`（检 Redis + PG 可写） | M4 |
| C5 | Dockerfile `TZ=Asia/Shanghai` | 下次提交 |
| C6 | 配置迁 yaml + `${VAR}` 展开 | ✅ 已拍板统一（2026-09-03），随 M3/M4 顺手落地 |
| C7 | **job_runs 迁 PG**（✅ 已拍板统一存储 2026-09-03：独立 PG 数据库 + utils `postgres`，schema 不变；解除 SQLite 单写者单副本约束） | M3/M4（约半天） |
| C8 | **utils `aksk` 包实现**（signer/verifier/gin 中间件工厂 + 常量时间比较 + 时间窗防重放 ±5min + 测试，~0.5 天；canonical = METHOD\nPATH\nsha256(body)\nTS\nX-Request-ID\nX-Operator（C9 的「覆盖 X-Request-ID」由此落在 canonical 里））——全部服务间签名的公共底座，**先行** | M3 前置 |
| C9 | callback client 请求签名（taskrunner → zhuzhao 回调带自身 SK 签名，覆盖 X-Request-ID/X-Operator 头） | 随 C8，M3 |

> 时序要点：**先拓扑、后拆 token**——网络隔离没落地前 Bearer 是实际防线（taskrunner 现部署在普通内网可达面），不裸奔切换。

## 10. 变更记录

| 日期 | 说明 |
|---|---|
| 2026-09-03 | 建档：基于 taskrunner/activelist 两仓库契约文档（2026-09-03 定稿）+ zhuzhao 全仓代码盘点，落「zhuzhao 侧能力清单 + 实施细排 + 里程碑对齐 + 迁移规划 + 待拍板项 P1–P7」；D1 同步为已完成（zhuzhao-utils v0.1.0） |
| 2026-09-03 | 同步 taskrunner 侧定案（所有者拍板 + M2 实现）：**P5 关闭**（回调鉴权不做独立机制——内网隔离 + callback_url 由 zhuzhao 指定，E-② 去掉 credential 中间件）；**P6 关闭**（action 不做前置校验、4xx 快速失败，E2 降为可选增强）；**P7 升级为 M3 硬前置**（audit_archive 副作用动作依赖业务级失败可表达）；M3 临界路径拍板项仅剩 P7 |
| 2026-09-03（所有者拍板批次） | **P1 关闭**（dept 标签复用 org 树，org code 即标签值）；**P2 关闭**（SSOT=activelist ADR-003「审计落点机制」专节：client 封装层同步写 `activelist_audit_log` + 本地重投队列、脱敏/水位对账均风险接受，修正本文原「水位对账兜底」表述）；**P4 关闭**（归档=本地卷+卷备份，03 号 D3 同步）；**P7 修订建议**：不要响应体状态字段——2xx 仅代表执行成功、业务失败直接映射 4xx/5xx（taskrunner M2 代码零改动），待所有者确认 |
| 2026-09-03（P3 拍板 + 关联键核查） | **P3 关闭**：异步写（所有者拍板）——channel → Redis List → 批量落库（03 号 §7 D1 同步）；核查登记**关联键缺口**：request_id 未注入 request context、audit_logs 无该列（03 号新增 §3.4，E-① 补：trace_id=request_id + audit_logs 加列建议）；仅剩 P7 待确认 |
| 2026-09-03（P7 拍板，表清零） | **P7 关闭**：不要响应体状态字段——2xx 仅代表执行完全成功、业务失败直接映射 4xx/5xx（taskrunner callback client 现有判定即最终行为，注释随下次提交更新；zhuzhao handler 走常规 errcode）；同步 taskrunner.md §4 / zhuzhao-integration.md §2.1 措辞。**P1–P7 全部关闭，M-E/M-A 动工前决策面清零** |
| 2026-09-03（公共能力统一基线） | 所有者拍板：**同级内部服务公共能力策略必须一致**，新增 §9 基线（SSOT）——鉴权统一为**零认证 + 专用 network 拓扑**（P5 推广为通用原则，覆盖 taskrunner M2 静态 Bearer 定案，时序=先拓扑后拆）；访问日志统一中间件（X-Request-ID 读头/回显 + X-Operator 兜底，关 B2）；request_id 头协议对齐 activelist；/readyz、TZ、配置形态统一；允许差异仅限存储/副本/Redis。taskrunner 对齐清单 C1–C6（§9）；activelist 已符合 |
| 2026-09-03（B1/B4 拍板） | **通信协议定稿：HTTP + JSON**（内部服务间全部；gRPC 不引入，覆盖旧「gRPC 内部+REST 外部」预留——phase3 README §4 同步关闭）；**配置形态统一 yaml + `${VAR}`**（C6 升为统一项，随 M3/M4）；基线表补通信协议行。待拍：B2 身份断言（建议简化为明文 X-Operator+拓扑，撤销方案 A 验签）、B3 存储（建议终态统一 PG、taskrunner SQLite 起步） |
| 2026-09-03（B3 拍板） | **存储统一 PG**（所有者拍板）：taskrunner job_runs 迁 PG（C7，独立数据库 + utils `postgres`，schema 不变，约半天；SQLite 保留为 M1/M2 已交付实现）；C4 readyz 改检 PG；允许差异收窄为 Redis·Asynq 与副本数。待拍仅剩 B2 身份断言（建议：明文 X-Operator + 拓扑，方案 A 降为触发条件驱动——场景展开已呈所有者） |
| 2026-09-03（AK/SK 基线修订） | 所有者拍板：**服务间通信统一 AK/SK HMAC 签名**（utils `aksk` 通用包 C8 先行 + 各服务接线 C2/C9/批次 B/M-A6）——**覆盖当日早前三条拍板**（C2 拆 Bearer→换验签、P5 回调无鉴权→带签名、activelist 零认证→验签），**关闭 B2**（明文 X-Operator 入签名覆盖，方案 A 降为触发条件）；专用 network 保留为第二道防线；C2 不再依赖 C3 时序；09 号外部 M2M AK/SK 加分层注记（算法复用、管理面仍 🚦） |
