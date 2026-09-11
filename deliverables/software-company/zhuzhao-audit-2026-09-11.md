# zhuzhao 生态 · 实现↔文档一致性 + 设计合理性 + 业界对标 审查报告

> **审查性质**：只读静态审查（未修改任何项目代码/文档）
> **审查对象**：`zhuzhao`（主）+ 父目录生态 `zhuzhao-utils` / `taskrunner` / `activelist` / `zhuzhao-ui`
> **日期**：2026-09-11
> **组织**：交付总监齐活林（主理人）｜ 架构师高见远（实现↔文档一致性）｜ 产品经理许清楚（业界对标）
> **方法**：静态阅读 + `grep/find/wc` 对账 + 抽样核验；**未运行** `make acceptance`（无 PG/Redis，运行态结论未复现）
> **置信度标注**：🟢 已确认 ｜ 🟡 疑似/表述级 ｜ ⬜ 未验证

---

## 0. TL;DR

**zhuzhao 是一个实现度很高、文档基本可信、整体符合业界主流的 Go 模块化单体 IAM + 工单系统。** 它的核心设计（双 Token + RT 轮换、三层鉴权、ltree 资源级过滤、组织树 + 虚拟组 + 委托、工单状态机 + 类型级授权）**没有方向性错误**；真正的问题集中在**"文档随代码漂移"的局部失真**（3 条确认不一致 + 约 8 条表述滞后）、**文档体量过重（文档行数 > 代码行数）**、**生态跨仓契约无自动校验**三点上。修复成本极低（多为文档改字），收益明确。

| 维度 | 结论 | 评级 |
|---|---|---|
| 实现能力 | IAM 五域 + 审计 + 三层鉴权 + 工单（封版）+ 任务代理/回调 + 网关反代 + 限流，**全部落地且带测试** | 优秀 |
| 实现↔文档一致性 | 抽查「已修复/已实施」14 项**全部有代码+测试双证**；确认不一致 **3** 条、疑似/滞后 **~8** 条 | 良好（局部失真） |
| 文档设计合理性 | SSOT 分层清晰、ADR 规范、review 体系良好；但**双文档树并存**、**体量偏重**、**可量化腐烂**（失效路径/死链） | 良好（有债务） |
| 业界实践符合度 | 六维对标**四维符合、两维部分符合（更轻/可收敛）**，无偏离主流 | 良好 |
| 生态耦合 | 边界清晰（不直连他库、AK/SK 双向对称正确）；**跨仓契约靠人工同步**（3 处风险） | 良好（有风险点） |

---

## 1. 实现能力盘点（🟢 证据充分）

**规模实测**：Go 文件 **143**（非测试 **76** / 测试 **67**）｜ Go 行 **25,137** ｜ 测试函数 **254** ｜ 迁移 **25 对（50 文件，000001–000025 连续无缺）** ｜ `docs/` **87 篇 / 25,874 行** + `doc/` **44 篇**

| 能力域 | 状态 | 关键代码位置（已核验存在） |
|---|---|---|
| 认证（AT/RT、RT 轮换、登出、黑名单、登录限流、会话吊销、改密） | ✅ | `internal/service/auth_service.go`、`session_revoke.go`、`internal/middleware/jwt.go` |
| 用户 CRUD/启停/改密/角色组织绑定/超管保护 | ✅ | `internal/service/user_service.go`、`internal/handler/user_handler.go` |
| 角色 CRUD/菜单分配/Casbin 同步/优先级防提权 | ✅ | `internal/service/rbac_service.go`、`internal/repository/role_repo.go` |
| 组织（ltree 树/move/成员/owner/虚拟组/scope） | ✅ | `internal/service/org_service.go`、`org_delegation.go`、`internal/repository/org_repo.go` |
| 菜单 CRUD/树/权限码 | ✅ | `internal/service/menu_service.go` |
| 审计（中间件同步写/登录审计/事件 FK 去 CASCADE） | ✅ | `internal/middleware/audit.go`、`internal/service/audit_service.go`、`migrations/000014` |
| **鉴权 L1** 路由级 Casbin（BFS + 超管通配） | ✅ | `internal/middleware/casbin.go`、`internal/casbin/enforcer.go`、seed `000002` |
| **鉴权 L2/L3** 资源级（属主/assigned/scope/BFS 三源/委托） | ✅ | `internal/service/ticket/resource.go`、`scope_resolver.go` |
| 工单（CRUD/状态机/分派/评论/备注/关联/类型模板） | ✅（已封版） | `internal/service/ticket/service.go`、`state_machine.go`、`type_admin.go` |
| 平台策略库 builtin（org-member/owner-only/role-gated） | ⚠️ **已实现+测试，但生产零接线** | `internal/pkg/resource/builtin.go`（仅测试引用） |
| 判定日志 L2（EvalHook → Redis List → 批量落库） | ✅ | `internal/pkg/audit/policyeval.go`、`registry.go`、`internal/pkg/reqid/`、`000020` |
| 内网回调 `/internal/jobs/callback`（AK/SK 验签） | ✅ | `internal/router/router.go:42,113-119`、`handler/jobs_handler.go`、`pkg/jobs/`、`000021` |
| 审计归档 `audit_archive`（JSONL 导出后删行） | ✅ | `internal/service/audit_archive.go`、`internal/app/providers.go` |
| 任务管理代理（E-④/E-⑦，出站 AK/SK） | ✅ | `internal/pkg/taskrunner/client.go`、`service/taskrunner_service.go`、`000022/000023` |
| 网关反代（前缀→上游、出站签名、身份断言、502） | ✅ | `internal/gateway/gateway.go`、`router.go:300` |
| API 限流（Redis Lua 令牌桶，429 + Retry-After，fail-close） | ✅ | `internal/middleware/ratelimit.go` |
| 路由↔menu_apis 对账（BK-22，启动 fail-fast） | ✅ | `internal/router/catalog.go`（`AuditRouteCatalog`） |
| **架构守护测试**（ArchUnit 式 AST 断言） | ✅ **加分项** | `internal/architecture_test.go`（`make guard`） |

> **实现质量亮点**：① 有 `architecture_test.go` 把架构约定钉成可失败断言；② RBAC 角色展开用 request context 缓存（BK-17）；③ `org_path` 快照竞态用 `FOR SHARE` 行锁 + 事务重构根治（OP1）；④ 行级过滤 fail-closed 哨兵 + AST 调用点守护（IW4）。

---

## 2. 实现 ↔ 文档一致性核对

### 2.1 总账

| 级别 | 数量 | 内容 |
|---|---|---|
| 🟢 **一致（实核）** | 绝大多数 | §1 能力矩阵 19 行**关键代码路径 19/19 真实存在**；§4 迁移地图 25 对**精确吻合**；§2 三层鉴权**逐条对应**；§6/§8 抽查 14 项"已修复"**全部有代码+测试双证** |
| 🟠 **确认不一致** | **3** | 见 2.2 |
| 🟡 **疑似/表述/滞后** | ~8 | 见 2.3 |
| ⬜ **未验证** | — | 门禁运行结论（无 DB 未复现）、swagger 逐项、compose 细节 |

> **核心判断：11 号项目控制台不是"纸面文档"——抽查的每条"已修复"都能在代码+测试定位到。** 问题集中在少数**未随代码同步的表述**。

### 2.2 🟠 确认不一致（3 条，均已主理人二次核验）

| # | 问题 | 证据 | 影响 |
|---|---|---|---|
| **I-1** | `docs/review/11` **§1 与 §3 自相矛盾**：§1 写回调端点 `/internal/jobs/<action_id>`，§3 写 `/internal/jobs/callback`；代码实为 **`/internal/jobs/callback`（action 在 body）** | 文档 `11:37` vs `11:87`；代码 `router.go:42`（注释同 source） | 实现者可能按 path 参数去接，契约认知偏差 |
| **I-2** | `11` **§3 称 `ticket-types`/`ticket-templates` 为「只读」**，但实际有 **7 个写端点**（2 POST + 5 PUT/DELETE）；且 §1 BK-18 行自述"7 管理端点"——**§3 与 §1 自相矛盾** | 代码 `router.go:275-287`；文档 `11:85` vs `11:166` | 权限/接口认知偏差；前端可能漏接管理面 |
| **I-3** | `docs/standards.md`（**工程公约 SSOT**）§4 写健康检查 `/healthz`+`/readyz`，但 **zhuzhao 代码与 11 号 §3、architecture.md、ops/deployment.md 等 8+ 处文档均为 `/health/live`+`/health/ready`** | `standards.md:52` vs `router.go:95,98`；`11:90` | SSOT 与代码冲突，违反"文档断言可与代码对账"纪律 |

> 注：`phase3/16:173` 的 `/healthz`+`/readyz` **不是错误**——那是 taskrunner/activelist 两个子服务的约定（其 README 一致）。真正冲突的是 `standards.md §4` 用未区分主/子服务路径的表述覆盖了全生态。

### 2.3 🟡 疑似 / 表述 / 滞后（~8 条）

| # | 问题 | 证据 |
|---|---|---|
| S-1 | §0 规模数字滞后：「70 非测试 / 51 测试 / 201 测试函数」vs 实测 **76 / 67 / 254**（文档自注"随代码漂移"） | `find`+`grep -c "^func Test"` |
| S-2 | §1「平台策略库 ✅」易误读为已投用——实际**生产装配零消费者**（`Builtin/OrgMember/OwnerOnly/RoleGated/RequireSchema` 仅出现在 `builtin.go` 与 2 个测试文件）；建议改标「🟡 库就绪待接线」 | Grep 全 `internal/` 无生产引用 |
| S-3 | §3 漏列：`POST /user/profile/update`、委托组 `members/scope`、`roles/list`、`roles/bind`、`roles/delete` | `router.go:151,169-174` |
| S-4 | §3 任务代理写作 `/v1/tasks`…，实为 zhuzhao 侧 `/api/v1/tasks`…；`/v1/*` 是**出站**到 taskrunner 的上游路径（概念混用） | `taskrunner/client.go` |
| S-5 | 文档中 **15 处 `internal/*` 代码路径不存在**：多为 utils 抽取后旧路径（`internal/pkg/{jwt,postgres,redis,response}` → 已迁 `zhuzhao-utils`）、规划未实现项（`internal/middleware/metrics.go` 等）、`internal/service/authz_service.go`（不存在） | `grep -rhoE 'internal/[a-z_/]+\.go' docs/` 对账磁盘 |
| S-6 | `docs/adr/ADR-003` 版本号滞后（写 `zhuzhao-utils v0.1.0`，现行 **v0.2.0**）；待办 E13 记的落地路径 `app/service/proxy/` 与实际 `internal/gateway/` 不符 | `ADR-003:120,139`；`go.mod` pin v0.2.0 |
| S-7 | 死链 **11 条**（含 `phase2/14` 自链错误、`review/03` 的 `file:///../../` 伪链、`doc/project-structure.md` 指向不存在的 `../legacy` 等）——A6② 声称"断链已修正"后仍有残留。**（`docs/` 内部分已治理；`doc/` 部分三轮关闭）** | 链接扫描脚本 |
| S-8 | `design/system-comparison.md` 称 `doc/` 有「42 份文档」，实际 **44**——**三轮判定为「有意不改」**：42 是审计时点 2026-08-11 的快照数，属历史语境 | `find doc -name "*.md" | wc -l` |

### 2.4 附：公约与存量的显式冲突（非缺陷，但需知情）

`standards.md §3` 约定「**方法仅 GET/POST**，PUT/DELETE/PATCH 不引入」，而 §3.5 显式**豁免**了 zhuzhao 工单管理面的 **5 处 PUT/DELETE**（封版不改，精确可核）。**这说明 standards 自身是准确的**——冲突方是 11 号 §3 的"只读"标注。新端点须遵守 GET/POST 约定。

---

## 3. 文档设计合理性评估

| # | 级别 | 评估 | 说明 |
|---|---|---|---|
| D-1 | 🟠 | **双文档树并存** | `docs/`（87 篇，新体系）+ `doc/module-assessment-2026-08/`（44 篇，旧系统评估）并列，但顶层导航（`docs/README.md`、`11 §7`）**只列 `docs/`**，`doc/` 成"隐形文档树" |
| D-2 | 🟠 | **SSOT 与代码冲突** | 见 I-3（健康检查命名） |
| D-3 | 🟡 | **体量偏重** | `docs/` **25,874 行 > 代码 23,233 行**；`design/architecture.md` 1,899 行、`design-decisions.md` 1,414 行单文件过长 |
| D-4 | 🟡 | **可量化的文档腐烂** | 15 处失效代码路径 + 11 条死链 + 版本号/规模滞后（见 §2.3）——正是项目自己在 B13 提的「F-1 路径腐烂教训」未彻底落 |
| D-5 | 🟢 | **SSOT 分层清晰** | `standards.md`（工程公约）+ `design-decisions §25`（权限架构）+ `phase3/16 §9`（服务实例基线）职责明确，且文首声明冲突优先级 |
| D-6 | 🟢 | **ADR 用法规范** | 3 篇均为"重大不可逆"决策，带日期/状态/背景/决策/后果，无滥用——但**覆盖不足**（见 §4 建议 D） |
| D-7 | 🟢 | **review 体系（少见，加分）** | 00–11 顺序编号 + 角色分工（findings/remediation/verification/control）+ 11 号"每次改动回填"活性治理层，多数开源项目不具备 |
| D-8 | 🟡 | 阶段目录含大量"设计就绪未实施"文档（phase3），且 §23 重定位后部分转"对接参考"——**读者需仔细分辨"设计稿/参考/已实现"** | `phase3/*` 状态横幅 |

> **D-1 复核修正（三轮）**：Owner 澄清 `doc/` 是**最初版旧文档归档、`.gitignore` 有意忽略**，现行一律以 `docs/` 为准。故「双文档树」**不是缺陷**——两棵树定位清晰（`docs/` 现行 / `doc/` 历史归档）；S-7/S-8 中涉及 `doc/` 的部分随之**关闭**（历史归档无维护义务），仅保留 `docs/` 内部的链接治理。

**总评**：文档体系**设计意识强、SSOT 明确、ADR 规范、review 治理层是亮点**；主要债务是**结构性的双树并行**与**可量化的腐烂**，以及**体量过重带来的维护成本**（多处漂移即其证）。

---

## 4. 业界实践对标（产品经理调研）

> 每维：「业界主流 → 代表实现 → 对照 zhuzhao 判断」。检索求证日期 2026-09-11。

| 维度 | 业界主流（代表） | 对照 zhuzhao | 判定 |
|---|---|---|---|
| **AuthN 会话** | 双 Token + RT 轮换（RFC 9700 / 2025 **MUST** 选项）；Keycloak 26.4 / Auth0 / Zitadel | 双 Token + RT 轮换 + 黑名单 jti + 登录限流 | 🟢 **符合**（黑名单属可选增强；高安全可演进 **DPoP**） |
| **AuthZ 谱系** | RBAC→ABAC→ReBAC（OpenFGA/SpiceDB，CNCF）；OPA/Cedar 策略引擎 | L1 Casbin RBAC → L2 ltree scope → L3 属主/委托；自研 ResourceRegistry + 策略评估日志 | 🟡 **部分符合（更轻）**：分层结构主流；未用 ReBAC 是**合理取舍**；自研评估器建议评估收敛到成熟引擎 |
| **组织树与继承** | ltree/闭包表/邻接表；Keycloak Org Groups / ServiceNow 动态组 | PG ltree + 虚拟组 + owner/委托 + **环检测/单调约束** | 🟢 **符合（偏严谨）**：防环比多数实现严格；动态组成员能力偏弱（随 HR 同步补） |
| **工单 / ITSM** | 状态机 + 表单 schema + 可见性 + 事件 + SLA（ServiceNow/JSM/Zendesk） | 状态机 + 类型/字段/模板 schema 校验 + `ticket_visibility` + `ticket_events` | 🟢 **符合**：与 ServiceNow user criteria / JSM project scope 同构；**SLA 缺失**（自研暂缓，转对接） |
| **Go 模块化单体** | handler/service/repository + Wire（编译期 DI）+ migrate/goose/atlas；同类 go-admin/RuoYi-Go/go-wind-admin | Wire DI + up/down 编号唯一迁移 + testcontainers **四档链式门禁** + Swagger | 🟢 **符合（偏严谨）**：门禁高于多数脚手架；迁移工具可评估 atlas 自动化 |
| **文档驱动 / 架构文档** | ADR（Nygard/MADR）+ C4 + arc42 + Diátaxis + docs-as-code | docs/ 权威源 + 改动同步 + ADR 3 篇 + review 回填 | 🟡 **部分符合**：SSOT/docs-as-code 踩中核心；ADR 覆盖不足、缺 C4 图、体量偏重 |

### 4.1 明显贴合业界（应保持）

1. **双 Token + RT 轮换 + 登出/吊销** — 直接命中 RFC 9700(2025) 对公开客户端的 MUST 建议。
2. **三层鉴权结构（路由级 + 资源级 + 属主/委托级）** — 业界无争议的主流结构；ltree 属 list 过滤三种主流实现之一。
3. **ltree 组织树 + 虚拟组 + 组织内委托 + 环检测/单调约束** — 对标 Keycloak Org Groups / ServiceNow 组树继承，防环比多数实现更严谨。
4. **工单骨架与类型级授权** — 与 ServiceNow user criteria / JSM project scope / Zendesk brand scoping 同构。
5. **工程化纪律：Wire DI + 编号治理 + testcontainers 四档门禁 + 文档 SSOT + review 回填** — 同类中上，门禁/文档治理甚至高于多数开源脚手架。

### 4.2 偏离 / 有更优替代（建议评估，非缺陷）

| # | 点 | 现状 | 业界更优/更简 | 建议 |
|---|---|---|---|---|
| A | 自研 ResourceRegistry + 策略评估器 | 自研 builtin + `policy_evaluation_logs` | Casbin 原生 model / OPA / Cedar | 保留日志价值；评估把**策略表达**交给成熟引擎，降自研维护成本 |
| B | 黑名单 jti（有状态吊销） | 服务端黑名单表 | RFC 9700 另一 MUST 选项：**DPoP / sender-constrained** | 高安全场景可引入 DPoP；黑名单须保证**校验侧强查 + TTL 清理** |
| C | 迁移编号人工占用表 | 自管理编号 | **atlas**（diff/lint/CI）/ goose | 评估迁移到 atlas 自动化编号/漂移检查 |
| D | **ADR 仅 3 篇** | 事件机制/Asynq/activelist | ADR 是业界标准，关键决策均应有 | 补 5–8 篇：三层鉴权、ltree 选型、双 Token/黑名单、编号治理、ticket_visibility、虚拟组 |
| E | 动态组成员弱 / SLA 缺失 | 虚拟组偏静态；SLA 未落 | Keycloak IdP mapper 动态入组；ITSM 标配 SLA | 随 `hr-directory-sync` 补动态成员；SLA 随工单对接形态定 |

---

## 5. 生态（父目录其他项目）

| 项目 | 定位 | 规模 | 状态 | 与 zhuzhao 的关系 |
|---|---|---|---|---|
| **zhuzhao** | IAM + 工单 主项目 | 143 Go / 87 md | P1、P2 交付；P3 设计就绪、部分主线已动 | — |
| **zhuzhao-utils** | 共享工具库（10 包） | 19 Go / 2 md | `aksk`（HMAC-SHA256）2026-09-04 已实现 | 被 zhuzhao/taskrunner/activelist 共用 |
| **taskrunner** | 事件/任务总线（Asynq） | 22 Go / 4 md | M1/M2 完成，**M3 部署联调待做** | zhuzhao 经 AK/SK 调其 API；回调 zhuzhao `/internal/jobs/callback` |
| **activelist** | 动态数据模型薄层 | 34 Go / 6 md | 已收敛为薄层，事件/审计移交 zhuzhao | zhuzhao 网关经 `/al` 前缀反代 |
| **zhuzhao-ui** | 前端 | 0 Go / 1 md | **未启动**（README："没想好"） | — |

### 5.1 生态耦合风险（🟡 中）

| # | 风险 | 证据 | 影响 |
|---|---|---|---|
| E-1 | **跨仓路由清单人工同步**：`migrations/000024` 的 14 条 `menu_apis` 须与 activelist 实际 handler 路径人工保持同步；BK-22 对账对 `/al` 前缀**整体豁免** | `000024:3` 注释；`catalog.go` 网关前缀豁免 | 上游改路径 → 死策略/混权，当前**无自动校验** |
| E-2 | **zhuzhao-utils 版本 pin 无 go.work**：zhuzhao 依赖已发布 v0.2.0（无 `replace`） | `go.mod` pin；无 `go.work` | utils 语义变更后各服务版本可能不一致（缓解=语义化版本+发版纪律） |
| E-3 | **taskrunner 集成 = HTTP 隐式契约**：出站 `/v1/*` + 回调 `/internal/jobs/callback`，contract = action_id + 信封，无共享类型/生成式契约 | `taskrunner/client.go`；`jobs/registry.go` | 契约漂移无编译期护栏 |

**正面**：zhuzhao **不直连他库**（`go.mod` 无 taskrunner/activelist 依赖，仅 HTTP/网关）；**AK/SK 双向对称正确**（出站 `gateway`/`Transport` 与入站 `router.go:114` 同用 utils `aksk`，canonical 覆盖 `X-Operator`/`X-Request-ID`，身份断言不可伪造）。**共同缓解方向** = 把 BK-22 的"上报校验器"推广到跨仓边界（design-decisions §26.2）。

---

## 6. 问题清单与修复建议（按优先级）

### P0 — 文档改字（成本极低、收益明确，建议立即做）

1. **I-1**：`11 §1` 回调路径 `/internal/jobs/<action_id>` → `/internal/jobs/callback`（action 在 body）。
2. **I-2**：`11 §3` 去掉 `ticket-types/templates` 的「只读」，补写端点（2 POST + 5 PUT/DELETE，标注公约豁免）。
3. **I-3**：`standards.md §4` 健康检查按服务区分——zhuzhao 用 `/health/live`+`/health/ready`，子服务用 `/healthz`+`/readyz`。

### P1 — 文档腐烂治理（半天内，doc-only）

4. **S-5**：回改 utils 抽取遗留的 4 处旧路径（`internal/pkg/{jwt,postgres,redis,response}`）+ 其余失效路径引用（含 `internal/service/authz_service.go`、迁移号 000007→000008）。
5. **S-7**：修死链（`phase2/14` 自链、`review/03` 伪链等）——正是 B13/F-1 的教训推广。**（`doc/*` 部分三轮关闭：历史归档无维护义务）**
6. **S-1/S-8**：规模数字重算，或统一改为"统计日期 + 允许漂移"口径。**（S-8 的 `doc/` 文档数部分三轮关闭）**

### P2 — 状态语义与结构

7. **S-2**：`11 §1` builtin 策略库行状态改「🟡 库就绪待接线」（避免误读为已投用）。
8. ~~**D-1**：把 `doc/module-assessment-2026-08/` 登记进顶层导航~~ **（三轮关闭：`doc/` 有意不入版本控制，无需登记）**
9. **D-3**：拆分/精简 `architecture.md`(1,899 行)、`design-decisions.md`(1,414 行)。

### P3 — 架构演进（触发驱动，非缺陷）

10. **AD**：补 5–8 篇核心 ADR（ADR 是业界标准最低成本最高回报实践）。
11. **A/B**：评估策略表达收敛到成熟引擎（OPA/Casbin model）；高安全场景引入 DPoP。
12. **E-1/E-3**：跨仓契约自动校验（activelist 路由清单自动比对 + BK-22 上报校验器）；`zhuzhao-utils` 引入 `go.work` 或明确发版纪律。

---

## 7. 方法与未覆盖项

- **已覆盖**：`internal/` 全部 76 个非测试 Go 文件、`router.go`+`catalog.go`、50 个迁移文件、`Makefile`、`docs/review/11`（逐节全核）、`standards.md`、`VISION.md`、`docs/README.md`、`adr/*`、`phase3/16`、`api/*`、`modules/*`、`phase1/2/3/*`、`doc/*`（抽样）、4 个生态仓 README。
- **未覆盖/未验证**：⚠️ **未运行** `make acceptance`/`make test-*`（无 PG/Redis）——文档中"四档全绿"的**运行结论未复现**；未逐项核对 swagger.json 与 handler 注解；未核验 `deployments/` compose 细节；未逐行核对全部 155 篇 Markdown 的每条断言（重点全量 + 其余抽样）。
- **原始报告**：
  - 架构审查：`.workbuddy/review-audit/architect-report.md`
  - 业界对标：`.workbuddy/review-audit/pm-benchmark-report.md`

> **结论一句话**：**这份代码库和它的文档，可信度远高于同类项目；它的问题不是"做错了什么"，而是"改得快、写得更多，文档在局部追不上代码"。** 修 3 条确认不一致 + 清一轮路径/死链腐烂，即可把一致性拉回"优秀"档。

---

## 8. 修复实施记录（2026-09-11，doc-only）

> 应"对上述问题进行修改"要求，已落实 P0 + P1 + P2 的文档级修复。**全部为文档改动，未触碰任何代码 / 迁移 / 配置。**

### 8.1 已修（13 个 docs 文件 + 1 处代码注释）

> 第 11/14/15 项（涉及旧 `doc/` 树）已在**第三轮回退**，见 §8.4。

| # | 级别 | 问题 | 修复 | 文件 |
|---|---|---|---|---|
| 1 | P0 | 回调端点 §1/§3 矛盾 | §1 改 `/internal/jobs/callback`（action 在 body，C10），并注明早期路径参数方案未采用 | `docs/review/11-project-control.md` |
| 2 | P0 | ticket 元数据误标「只读」 | 改为「读 + 管理」，补 7 写端点（2 POST + 5 PUT/DELETE）与权限码 `ticket:type:manage` | 同上 |
| 3 | P0 | 健康检查命名与代码冲突 | `standards.md §4` 按服务区分：zhuzhao `/health/live`+`/health/ready`；子服务 `/healthz`+`/readyz` | `docs/standards.md` |
| 4 | P1 | utils 抽取遗留旧路径 | `internal/pkg/response/response.go` → `zhuzhao-utils/response`（2 处） | `docs/api/errcode.md`、`docs/api/response.md` |
| 5 | P1 | 同上 | `internal/pkg/{postgres,redis}` → `zhuzhao-utils/{postgres,redis}`（2 处） | `docs/phase1/01-infra.md` |
| 6 | P1 | 同上 | `internal/pkg/logger/logger.go` → `zhuzhao-utils/logger` | `docs/phase1/08-audit.md` |
| 7 | P1 | 重构后路径未同步 | `internal/service/ticket_service.go` → `internal/service/ticket/service.go` | `docs/phase2/04-org-delegation.md` |
| 8 | P1 | 已实现功能路径写错 | `internal/task/audit_archive.go` → `internal/service/audit_archive.go` | `docs/phase3/03-audit-l2.md` |
| 9 | P1 | 跨仓路径未标仓 | `internal/handler/response.go` → 加 `activelist 仓库` 前缀（消除与 zhuzhao 同名的歧义） | `docs/phase3/16-external-integration.md` |
| 10 | P1 | 死链/伪链 | `file:///` 伪链改普通代码引用；`docs/phase2/14` 模板自链改无链接占位 | `docs/review/03-*`、`docs/phase2/14-*` |
| 11 | P2 | 规模数字漂移 | §0 改 **75 / 67 / 254**（实测）+ 标注统计时点 2026-09-11 | `docs/review/11-project-control.md` |
| 12 | P2 | builtin 状态易误读 | ✅ → **🟡 库就绪待接线**（生产零消费者；对齐基线的 🟡 记号） | 同上 |
| 13 | P1 | 目录形态旧路径漏网（二轮 grep 只扫 `.go`） | `internal/pkg/{redis,response,jwt}/` → `zhuzhao-utils/...` | `docs/api/README.md`、`docs/phase1/01-infra.md`、`docs/phase1/02-auth.md` |
| 14 | P1 | 旧端点漏网 | `/internal/jobs/:action_id` 标注为旧路径形态（现行 `/internal/jobs/callback`） | `docs/phase3/16-external-integration.md` |
| — | — | 代码注释同步（**非逻辑改动**） | `POST /internal/jobs/<action_id>` → `/internal/jobs/callback`（action_id 在 body） | `internal/pkg/jobs/registry.go:4` |

### 8.2 有意**不改**（已判定为历史快照 / 规划项 / 已注解）

- **历史快照**：`docs/review/00–10`、`docs/phase1/12-*`、`docs/phase2/14-*` 中出现的 `internal/service/authz_service.go`（描述其**删除**）、`internal/service/auth_service_test.go` 等——属历史语境，改之反而失真。
- **规划项**：`internal/{middleware/metrics.go, pkg/storage/s3_client.go, integration/hr/client.go, task/outbox_worker.go, service/user/*, handler/storage_handler.go}` 等——设计期规划路径，尚未实现，**保留**。
- **模板占位**：`docs/modules/README.md` 的 `internal/service/xxx_service.go`（字面模板）。
- **已自带迁移注解**：`docs/phase1/11-code-review.md:33` 已注明"已迁 zhuzhao-utils/jwt"，无需改。

### 8.3 ⚠️ 修复过程中**新发现**的问题（需你决策，未擅自处理）

| # | 问题 | 证据 | 影响 | 建议 |
|---|---|---|---|---|
| **N-1** | ~~`doc/` 整个目录被 `.gitignore` 忽略~~ **（三轮已确认：有意为之）** —— 44 篇旧系统评估归档不在版本控制内（`git ls-files doc/` = 0） | `.gitignore:40` = `doc/` | ① 新人 clone 后 `doc/` 不存在 → `docs/design/system-comparison.md`、`docs/modules/*` 对其的引用为**历史溯源性质**，非现行依赖；② 无需修复 | Owner 澄清：`doc/` 是最初版旧文档归档，**有意忽略、不入版本控制**；现行一律以 `docs/` 为准。**判定为预期状态，非缺陷；已回退所有 `doc/` 改动** |

### 8.4 第三轮：`doc/` 回退 + 以 `docs/` 为基线的一致性复核

**触发**：Owner 澄清 **`doc/` 为最初版旧文档归档（`.gitignore` 有意忽略，不入版本控制），现行一律以 `docs/` 为准**。据此把第二轮对 `doc/` 的改动**全部回退**，并以 `docs/` 既有惯例为基线复核本轮所有改动的一致性。

**8.4.1 回退（2 轮内对 `doc/` 的改动，共 4 处）**

| 项 | 二轮动作 | 三轮处置 |
|---|---|---|
| `doc/README.md` | 新建归档说明 | **删除** |
| `doc/module-assessment-2026-08/{project-structure,interaction-auth-chain}.md` | 修 5 处旧仓库死链 | **`git checkout` 回退** |
| `docs/README.md` | 顶层导航新增 `doc/` 归档登记块 | **移除该块** |
| `docs/design/system-comparison.md` | 「42 份」加数字+归档注记 | **`git checkout` 回退**（「42 份」是审计时点 2026-08-11 的快照数，属历史语境、非现行断言 → 归入 §8.2「有意不改」） |

**8.4.2 一致性修正（对齐 `docs/` 基线惯例）**

| # | 项 | 处置 |
|---|---|---|
| 1 | 状态标记 `🔷`（二轮自创，`docs/` 基线无此符号，既有无 🔷） | 改回基线既有 **🟡** |
| 2 | 跨仓称谓「activelist 仓」 | 统一为基线写法 **「activelist 仓库」**（`16-external-integration.md`、`api/errcode.md`） |
| 3 | utils 抽取注记措辞 | 统一为基线格式「**2026-09 公共包抽取：`internal/pkg/X` → `zhuzhao-utils/X`，原路径删除**」（errcode/response/01-infra/08-audit） |
| 4 | 目录形态旧路径 `internal/pkg/{redis,response,jwt}/`（二轮 `grep` 只匹配 `.go` 漏掉） | `api/README.md`、`phase1/01-infra.md`（×2）、`phase1/02-auth.md` 补抽取注记 |
| 5 | `16-external-integration.md` E-② 旧端点 `/internal/jobs/:action_id`（二轮漏改） | 标注为**旧路径形态**，现行 `/internal/jobs/callback` |

**8.4.3 复核结果（`docs/` 全库扫描）**

- 裸「仓」引用（非「仓库」）= **0** ｜ `🔷` 残留 = **0** ｜ 指向旧 `doc/`（非 `docs/`）= **0**
- 活跃文档中的旧 `internal/pkg/*` 路径 = **0**（历史快照类文档 `review/03`、`review/04`、`phase1/12` 有意保留原貌）
- 最终改动清单 = **13 个 `docs/` 文件 + 1 处代码注释**（`internal/pkg/jobs/registry.go`，仅注释）；`.gitignore` 有 1 行改动**非本次所为**（见 §9 提示）

> 未验证项同 §7：未跑 `make acceptance`；本次为文档改动，不影响代码与门禁。
