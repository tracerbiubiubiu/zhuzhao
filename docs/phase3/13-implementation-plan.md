# 13 - Phase 3 执行计划（排期规划稿）

> **定位**：在 [phase3/README](./README.md) 已确认的 **Wave W0–W4** 执行结构之上，补充可执行的排期计划——里程碑、人日估算、依赖顺序、退出标准、不确定项与触发条件驱动项。
> **状态**：**规划稿（2026-09-02 建档）**。Phase 3 的正式启动仍以所有者确认触发条件为准（roadmap 维持「暂缓」直到启动）；本文档回答「启动后怎么做」。
> **⚠ 2026-09-02 重定位（SSOT = [design-decisions §23](../design/design-decisions.md)）**：工单自研暂缓（内部引擎优先，自研兜底）（Phase 2 现状封版），项目将迁移公司内部并对接内部工单平台/引擎；§4 工单业务闭环（含 7c 引擎）**全部暂缓自研**，Phase 3 主线改为「Asynq 事件基建 + activelist 独立实现 + HR 同步 + 迁移准备」。§4 及工单相关段落保留作历史设计与对接参考，**不再驱动排期**；现行主链见 §1 修订表。
> **配套**：启动检查单 [00-startup-checklist](./00-startup-checklist.md)；工单业务设计 [10-ticket-business](./10-ticket-business.md)（已转对接参考）；前端规格 [12-frontend](./12-frontend.md)（已转参考）。
> **标记约定**：`🚦` = 触发条件驱动，**是否纳入本次排期由所有者决定**；`⚠️` = 存在不确定性 / 待拍板，实现前需确认。

---

## 0. 排期口径（先读）

| 项 | 口径 |
|---|---|
| 人力假设 | **1 人全栈串行**（后端为主，前端单列可并行） |
| 估算性质 | 人日数为**工程估算**（有文档量级参考的标注出处），非承诺；实现前按实际校准 |
| 里程碑 | ~~M0→M1→M2→M3→M4→M5~~ **现行主链（2026-09-02 §23）**：M0 启动准备（收窄）→ **M-E 事件与任务平台** → **M-A activelist 独立实现** → **M-HR HR 同步** → **M-Mig 迁移准备**；~~M1 可运维基座~~ / ~~M2 工单业务~~ / ~~M3 加固~~ / ~~M4 activelist 集成~~ / ~~M5 3b~~ 均降 🚦 或暂缓（见 §1 修订表） |
| 硬前置 | ~~W2 以 W1 为硬前置~~ **修订（2026-09-02，design-decisions §22.1/§23）**：现行主链无硬前置链式依赖——**M-E 依赖 Asynq（ADR-002）引入 + 2c Authorize（已交付）**；M-A/M-HR/M-Mig 相互独立、按各自触发条件推进；防重约定（sla:scan Unique + L1 advisory lock）仅对事件基建适用，写码时按 [02-multi-instance](./02-multi-instance.md) 遵守 |
| 门禁 | 每 Wave 退出标准 + `make acceptance` 四档 + 13 包 `-race` + Phase 3-min/full 验收 |
| 迁移 | Phase 3 迁移编号启动时按 A2 规则重排（当前 000001–000019 已占用，Phase 3 起 **000020**） |

> ⚠️ **未给出日历排期**：本计划以「人日」计。若给定启动日期 + 人力数，可再展开为带日期的排期（Gantt）。

---

## 1. 里程碑总览

| 里程碑 | 内容（对应 Wave/Step） | 估算人日 | 退出标准 |
|---|---|---|---|
| **M0 启动准备**（收窄） | 迁移号核对 + 决策过表 + BK-20（工单封版前最后的数据安全修复） | 1–2 | 决策清单过完；BK-20 落地 |
| **M-E 事件与任务总线** | ① Asynq 底座（Scheduler/PeriodicTask/worker/重试/超时/阻塞策略按任务拍板）；② **预置动作 = 回调 zhuzhao 内网端点**（业务 handler 在 zhuzhao：审计归档 B11② 首个、通知、SLA 扫描、外部回调等按需）；③ 业务点**显式发布**（zhuzhao Enqueue/API 提交 → taskrunner 异步触发回调）；~~自定义脚本任务~~ **2026-09-03 降级 🚦**（上传 python/shell 暂不需要；Dagu/自研调研见 [15](./15-script-platform-dagu-vs-inhouse.md)，按需再启）；**独立仓库 + 独立部署 + 独立 Redis**（2026-09-03 拍板，形态见 §1 注记）；**权限前置（2026-09-03 落档）**：平台策略库（[authz.md §3.1](../modules/authz.md)——`org-member` 策略承载任务提交/回调端点判定；design-decisions §25.5 批次 A，2–3 天）；**zhuzhao 侧配套细排（内网回调端点体系 / 任务管理代理 / 部门可见性 / 提交日志，E1–E6）见 [16-external-integration](./16-external-integration.md)，约 5–7 人日与 taskrunner 侧并行**；**服务间 AK/SK HMAC 签名**（utils `aksk` 包 C8 先行，zhuzhao 回调端点验签 / client 签名，16 号 §9 基线，2026-09-03 拍板） | 3–4（⚠️ 待校准；原 6–8） | 归档任务按周期跑通；预置动作 触发→回调执行→失败重试 闭环；全门禁绿 |
| **M-A activelist 独立实现** | 独立库 + 独立数据库（§22.3/§23）；**外部事件接入契约由 activelist 侧定义**（工单非首数据源，§23.2）；**2026-09-03 职责收敛 + 需求澄清**（ADR-003 修订）：收窄为**动态数据模型薄层**——任意自定义类型（字段=`int`/`string`/列表）、零认证、查询=仅 id 分页+时间倒序、PG **每类型表 + `data` JSONB**（id 自增/乐观锁/软删保留）、导入导出 JSON（**幂等=全量替换**：单事务清表重插、保留源 id、不需要业务唯一键——方案 D 定稿同步，2026-09-03）；事件与审计移交 zhuzhao（Asynq 显式发布 / zhuzhao 侧记录），进程 3→1；独立部署保留；**前置：~~共享 utils 抽取~~ ✅ 已完成（zhuzhao-utils v0.1.0 已发布并 pin，9 包含 logger/postgres，2026-09-03）**；**权限前置（2026-09-03 落档）**：网关化批次（design-decisions §25.5 批次 B，~1 周）——反代核心/身份断言（明文 X-Operator 入 AK/SK 签名覆盖，B2 已关）/API 级限流/API 入 menu_apis/审计跳 body（ADR-003 G4 蓝图保留）；**zhuzhao 侧配套细排（D1–D5 对照 + D3 审计落点）见 [16-external-integration](./16-external-integration.md)** | 约 1.5–3（澄清后估算 ⚠️ 待校准；原单估） | 按其项目自身验收 |
| **M-HR HR 同步（预留接口版）** | `HRFetcher` 接口 + sync 引擎 + 本地 mock adapter（内网 adapter 即插）；拍板：离职处置/部门撤销级联/跨部门权限分配规则 | 3–5 | 组织同步三规则对 mock 源跑通；对账幂等；三拍板项落档 |
| **M-SSO 单点登录（OAuth2.0）**（🚦 进内网后实施——设计已定稿 §24，拿到公司接入信息即开工） | `SSOProvider` 接口 + OAuth2.0 授权码实现（authorize + code 换 token + userinfo，state 防 CSRF）+ `/auth/sso/login`·`/auth/sso/callback` 两端点 + 身份映射（对账键同 HR：external_id 优先）+ 登录审计 method 字段 + 登录页 SSO 入口；**鉴权三层零改动**（callback 签发自有 JWT/RT）；JIT 默认关（config 开关，仅限已同步账号）；本地密码登录兜底并存；公司接入信息填 config 即用（design-decisions §24） | 🚦 2–3 | mock/测试 IdP 走通完整回调链 → 自有 JWT 签发 → 三层鉴权行为不变 |
| **M-Mig 迁移准备** | 公司内网部署对接（网络/凭据/07 security 与 08 ops 中生产相关项按内网形态重估：CORS 收紧/限流/审计保留期）+ **正式命名 + module path 迁移**（2026-09-03 拍板：改名成本=全仓 import 重写，与进内网换 Git 平台的 module path 变更**合并执行一次**，README 现只注定位不改库）+ **内部工单平台权限表达力评估**（2026-09-03 登记：对照 zhuzhao 三轴模型——project_isolated 外包隔离/虚拟组兄弟隔离/委托轴 D7-D9——内部平台表达不了的项列为对接缺口，供「工单对接形态」拍板输入，§23.3） + ~~部署镜像带 python/shell 解释器~~（脚本任务运行时依赖，**2026-09-03 随脚本任务降级 🚦**） | 🚦 随迁移时点 | 内网环境部署演练通过 |
| ~~M2 工单业务闭环~~ | **暂缓自研（§23.1）**——§4 保留作对接参考；工单对接 = 迁移后集成项 🚦（design-decisions §23.3） | — | — |
| ~~M1 可运维基座~~（🚦） | 多实例/Watcher/audit-l2——随部署形态升级触发（§22.1）；audit-l2 的 B11① 判定日志**可与 M-E 合并实施**（价值独立于多实例） | 6–10（🚦） | MI1–5 通过（多实例时） |
| ~~M4 activelist 集成~~ | 被 **M-A** 取代（§23.2：独立实现而非反代集成；E13/G1/G3 蓝图保留于 ADR-003 🚦） | — | — |
| ~~M5 3b 按需~~ | 事件 L2 / platform / 分离档位 2——随迁移后形态评估 | 🚦 | — |

> **合计（现行主链，不含 🚦/单估）**：约 **7–11 人日** = M0 1–2 + M-E 3–4 + M-HR 3–5（M-A 澄清后约 1.5–3 ⚠️ 仍单列、M-Mig 🚦、M1 6–10 🚦 均未计入）。量级参考：审计归档参照 ginfast scheduler 执行器（design-decisions §23 补充拍板已核验可用）；脚本任务已降级 🚦。

> **M-E 部署形态（2026-09-03 拍板定稿，设计 SSOT = taskrunner 仓库 `docs/taskrunner.md`）**：zhuzhao 作**网关**（鉴权/编排/业务审计），taskrunner 事件/任务服务 = **独立仓库 + 独立部署 + 独立 Redis**。已定：① **独立仓库**（activelist 式，可独立部署，zhuzhao 经其 API 提交任务）；② **Redis 队列独立**；③ **预置动作 = 回调 zhuzhao 内网端点**（业务 handler 在 zhuzhao，taskrunner 只做通用调度/触发/重试，HTTP 回调执行）。**日志边界（2026-09-03 确认）**：zhuzhao Enqueue 时记「任务提交日志」（action + task_id + request_id；业务审计在 `audit_logs`）；taskrunner 执行时**自维护** `job_runs`（执行细节/重试/耗时），**不传回 zhuzhao**；两边以 `request_id` 关联，需要时跨查。新增预置动作 = zhuzhao 加内网端点 + taskrunner 加配置，**无需重新部署 taskrunner**。任务提交方式落地时细化 ⚠️：zhuzhao 直连 taskrunner Redis Enqueue vs 调 taskrunner API 提交（倾向 API，符合「网关调用能力」）。

---

## 2. M0 启动准备（启动前/启动时并行）

| 任务 | 人日 | 说明 / 标记 |
|---|---|---|
| 刷新启动检查单 | 0.2 | [00-startup-checklist](./00-startup-checklist.md) §1 顺序过表，状态与 11 §6/§8 对齐 |
| 迁移号核对重排 | 0.3 | A2 规则：Phase 3 原 000017–000021 整体重排 → 000020 起；与 2b-ext 附件竞争号（⚠️ 若附件先启动需再让位） |
| 决策清单过表 | 0.3 | 见 [README §4](./README.md#4-待决策点) + 本文 §5：K8s vs Compose、Redis/PG HA、部署级分离时机、03 写入管道（~~审批流引擎选型~~ 随 §23 暂缓） |
| **BK-19**（工单 handler Go 测试） | ~~0.5–1~~ **随封版后置**（§23） | 工单现状封版，handler 测试不再作为主链前置；翻案/对接时再评估 |
| **BK-20**（禁删有未结工单组织守卫） | 0.5 | 两删除函数计数守卫 + ErrOrgHasOpenTickets 409 + acceptance 删组织用例适配（工单封版前最后数据安全修复） |
| 顺手项（可选） | ~~0.5~~ **随封版后置** | BK-9 / F-31④ / F-32 / TC-2/3/4 / Q5 注记（工单相关随手项随 §23 后置） |
| 待编写文档 | 0 | **已补齐（2026-09-02）**：03 / 06 / 07 / 08 / 09 / ops-deployment + 13 本计划 |

> ⚠️ **不确定项**：① 工单审批/报表前端已随 §23 暂缓（仅 BK-18 管理页/动态表单仍为 IW3 独立窗口，见 [12-frontend](./12-frontend.md)）；② M-A activelist 澄清后核心功能人日**约 1.5–3（⚠️ 待校准）**，~~共享 utils 抽取为其前置 🚦~~ **已完成（zhuzhao-utils v0.1.0，2026-09-03）**；③ 迁移号是否需为附件让位。

---

## 3. M1 可运维基座（W1，Step 1/2/3）——🚦 降级为「部署形态升级为多实例时启动」（2026-09-02，design-decisions §22.1）

> ~~本里程碑是 M2 的硬前置~~ **已修订（§22.1/§23）**：现行主链无「M1→M2」链式依赖——事件基建（M-E）独立于多实例；防重约定（sla:scan Unique + L1 advisory lock）仅事件消费适用，写码时按 [02-multi-instance](./02-multi-instance.md) 遵守。本里程碑在单实例下无验收手段（MI1–5 需 2 实例 + Nginx），整体随部署形态升级触发；翻案条件 = 启动时即确定多实例部署。

| 任务 | 人日 | 依赖 | 退出标准 |
|---|---|---|---|
| **Step 1 observability**（[01](./01-observability.md)） | 2–3 | Phase 2 基线 | 全 `enabled:false` 零开销可启动；开启时 `/metrics` 200；OTLP 不可达默认 Warn 降级 |
| **Step 2 multi-instance**（[02](./02-multi-instance.md)） | 2–4 | Step 1 | Casbin Watcher（移植 eiam `ioc/casbin.go`，redis-watcher + StartAutoLoadPolicy 双保险）；L1 消费 `pg_try_advisory_xact_lock`；**MI1–5** 通过 |
| **Step 3 audit-l2**（[03](./03-audit-l2.md)） | 2–3 | Step 2 | Redis List L2 缓冲 + **B11① 判定日志表** + 埋点 `resource.Authorize` / `scope_resolver.resolve`；写入管道 ✅ **已拍板（2026-09-03）：异步**（channel → Redis List → 批量落库；request_id 全链路关联见 03 §3.4） |
| **B11② 审计归档**（03 §4） | 0.5–1 | 随 **M-E**（首个预置任务） | 判定日志 + audit_logs 超期导出 JSONL、成功后删行；保留 180 天可配置 |

> ⚠️ 03 文档已含 B11① 范围；写入管道决策点保留待实现前拍板（见 [03 §7](./03-audit-l2.md)）。

---

## 4. M2 工单业务闭环（W2，Step 7）—— ⏸ **暂缓自研（2026-09-02，design-decisions §23.1）**

> 本节全部任务（含 7c 引擎、7-0 设计期）不再实施；内容保留作**公司内部工单平台对接时的参考设计**。工单对接形态（IAM 提供方/数据同步/单点跳转）与 Phase 2 资产处置（保留兜底/适配层/下线）待迁移时拍板（§23.3）。

### 4.1 任务与顺序

```
7-0 设计期（1–2 天）→ [L1 事件升级 + Asynq 底座] → 7a SLA → 7b 通知 → 7c 审批流（可并行）→ 7d 分派 → 7e 报表
```

| 任务 | 人日 | 依赖 | 说明 / 标记 |
|---|---|---|---|
| **7-0 设计期** | 1–2 | 无 | 拍板 B1 全部设计项 + 权限码 seed（B2）；调研吸收（撤回/版本快照/Assignee/模板-流程绑定）已在 7-0 决议 §4.10 落大半，细节补全；**新增拍板项（2026-09-02）**：① ActivelistWriter 数据契约 + 审批通过事件触发点；② 通知/SLA「跟人 vs 跟单」语义；③ **引擎接口契约**——`WorkflowEngine` 接口（Start/Advance/CanAct/OnEvent）+ 实例状态通用层/引擎私有层（JSONB）分离（design-decisions §22.6） |
| **L1 事件升级**（[10 §7](./10-ticket-business.md)） | 2–3 | 2c + Asynq 底座（M2 内） | 迁移 000024（ticket_events ALTER event_type/processed）；单消费者轮询 + 分布式锁（advisory lock 防重，按 02 号约定）；audit/signal 双职责（决议 5 定死两条记录） |
| **Asynq 底座**（ADR-002） | 1–2 | L1 | Scheduler + PeriodicTask + worker；铁律：L1 管事件持久化、Asynq 管执行；sla:scan 配 Unique 去重 |
| **7a SLA**（[10 §2](./10-ticket-business.md)） | 3–5 | L1+Asynq | ticket_sla + sla_policies（迁移 000020）；扫描→sla:breach 链；**四必坑**（暂停态/提前取消/幂等/Enqueue 原子性）→ TB12–15 |
| **7b 通知**（[10 §3](./10-ticket-business.md)） | 3–4 | L1+Asynq | notifications 表（000021）+ 4 API + SMTP + 邮件矩阵；「主管」= 所属组织 owner_user_ids |
| **7c 审批流引擎**（[10 §4](./10-ticket-business.md)） | **8–12** | 2c + L1 | BranchedStateEngine（**硬交付**）+ workflow_* 表（000022）+ 发布快照表（决议 4）+ CanApproveNode + 驳回语义 + 发起人撤回（TB16）；引擎本体最重 |
| **7d 分派**（[10 §5](./10-ticket-business.md)） | 2–3 | 2c | assignment_rules（000023）+ 匹配引擎 + 五项决议；分派失败不回滚创建 |
| **7e 报表**（[10 §6](./10-ticket-business.md)） | 2–3 | 7a–7d | SQL 聚合 + 缓存 TTL 5min + 4 API + report:read；指标口径（SLA 达成率 / P50/P95） |
| 权限码 seed（B2 / 决议 6） | 0.5 | 7-0 | ticket:approve / notification:list / notification:read / workflow:manage / report:read + 菜单 |
| **B3** in_progress/pending_verify 端点 | 0.5–1 | 7-0 | BK-10 已拍板归 Phase 3 |
| 配置即代码（[10 §4.9](./10-ticket-business.md)） | 1–2 | 7a/7c | workflow/SLA YAML + 版本闸门 + dry-run + cmd/bootstrap |
| **前端**（[12-frontend](./12-frontend.md)） | 5–10 | 后端 API | 动态表单（BK-18 IW3 批次）+ 管理页 + 审批人配置页 + 审批操作页 + 报表页；**可与后端并行（需独立人力）** |

### 4.2 退出标准

- TB1–16 全绿（**含负向 TB12–16**，10 §8）；
- 前端 FE1–4 通过（12 §5）；
- ⚠️ 若启动不含前端，M2 后端完成即满足主链退出（前端另排期，不阻塞 Phase 3 验收的工单业务部分）。

---

## 5. M3 加固收尾（W3，Step 4/5/6/8）

| 任务 | 人日 | 依赖 | 说明 / 标记 |
|---|---|---|---|
| **Step 4 ha**（[06](./06-ha.md)） | 2–3 | M1 | PG Cluster（云托管 2+VIP 已决策）+ Redis Sentinel + Nginx；**🚦 是否需要 = 取决于部署形态**（Phase 3-min 不要求；full/SLO 才要） |
| **Step 5 security-enhance**（[07](./07-security-enhance.md)） | 3–5 | M-E（Redis/Asynq 就绪） | API 限流（Phase 1 登录限流基础上扩展）+ **B7 CORS AllowAll 收紧**（必做）；🚦 异地登录 / 密码过期 / 验证码 = 触发条件驱动（合规/安全要求） |
| **Step 6 ops**（[08](./08-ops.md) + [ops/deployment](../ops/deployment.md)） | 2–3 | M-E | Swagger CI / 迁移 CI / 集成测试自动化 / 一键部署 |
| **Step 8 生产验收**（README §3.1） | 1–2 | M-E/M-HR | Phase 3-min（单实例内网）/ 3-full（多实例/SLO）两档选一；含**主链（M-E/M-HR）**回归（工单业务已随 §23 暂缓，不纳入验收回归） |

> 🚦 若本次部署形态为单实例内网（3-min），ha + 多实例相关安全增强可**整体从本次排期剔除**，由你决定。

---

## 6. ~~M4 activelist 集成~~（2026-09-02 拆两半，design-decisions §22.3；**被 M-A 取代，§23.2**）

> ⚠ **本节为历史设计（§22.3），已被 §23.2 取代**：activelist 改为**独立实现**（M-A，独立库 + 独立数据库），外部事件接入契约由 **activelist 侧定义**（工单非首数据源）；旧「写侧管道 + ActivelistWriter 反代集成」方向不再实施。E13/G1/G3/G4 蓝图保留于 ADR-003（🚦）。下表仅作历史留档。

| 部分 | 内容 | 时机 | 标记 |
|---|---|---|---|
| ~~本次执行：写侧管道~~ | ~~审批通过（引擎节点事件）→ L1 ticket_events → Asynq 任务 → `ActivelistWriter` 接口~~（随 §23 一并暂缓；审批流已不实施） | ~~随 M2~~ | ~~本次做~~ → **被 M-A 取代** |
| **🚦 蓝图：E13 反代 / G1 独立库 / G3 多源 ingress / G4 两层审计** | zhuzhao 为 activelist 提供统一鉴权网关；activelist 独立数据库（故障隔离，已拍板认可）；工单多源适配 | activelist 独立项目成型后启动（ADR-003 SSOT：roadmap §activelist） | 🚦 |

> 历史定位：zhuzhao = 工单系统单向上游（写入 + 事件）；不与 activelist 项目进度互相阻塞。

---

## 7. M5 3b 按需（🚦 全部触发条件驱动）

| 项 | 触发条件 | 内容 | 标记 |
|---|---|---|---|
| 事件 L2（Outbox + Asynq worker 多消费者） | 多消费者 / 异步邮件需求（L1 单消费者瓶颈） | L1 换调度器，业务不变 | 🚦 |
| 权限/菜单缓存跨实例失效 | 多实例 + 热点（QPS 瓶颈） | `perm:user:{userId}` Redis 缓存 + Pub/Sub 失效（design-decisions §1） | 🚦 |
| AK/SK | 有 M2M 调用方 | 平台凭据体系 | 🚦 |
| 部署级分离档位 2 | 工单独立扩缩容需求 | 同二进制 + APP_MODULES（[11](./11-deployment-split.md)） | 🚦 |
| 微服务拆分 | 多团队/M2M | **不做**（无需求） | ❌ |

> **HR 同步**：**已升主链（2026-09-02 拍板，design-decisions §22.2/§23.2）**——见 **M-HR** 里程碑（预留接口版：HRFetcher + 引擎 + mock adapter）；组织架构来自公司接口定时更新为已确认真实诉求，且为**内网迁移前置**。**附件 / auth-enhance**（2b-ext）：仍独立窗口按需；附件启动会占用迁移号（⚠️ 与 Phase 3 竞争，A2 规则）。

---

## 8. 🚦 触发条件驱动项总表（由你决定）

| 项 | 触发条件 | 建议 | 纳入本次？ |
|---|---|---|---|
| **M1 可运维基座**（observability/multi-instance/audit-l2） | 部署形态升级为多实例 / 需要策略跨实例生效 | 单实例内网可不做；M-E 事件消费按 02 号约定写 | ☐（2026-09-02 默认后置） |
| ha（PG Cluster/Redis Sentinel） | 99.9%+ SLO / 对外 SLA | 单实例内网可不做 | ☐ |
| 异地登录 / 验证码 / 密码过期 | 合规或安全要求 | 密码过期合规时做，异地登录按需 | ☐ |
| activelist 蓝图（E13/G1/G3/G4）+ 多源接入 | activelist 独立项目成型（M-A） | **activelist = M-A 独立实现**（§23.2，契约由 activelist 侧定义）；E13/G1/G3/G4 蓝图随其项目 | ☐ |
| 事件 L2（Outbox） | 多消费者/异步邮件 | 出现时启动 | ☐ |
| AK/SK | M2M 调用方 | 有调用方时 | ☐ |
| 部署级分离档位 2 | 工单独立扩缩 | Phase 3 末验证边界 | ☐ |
| 附件 / auth-enhance（2b-ext） | 按需 | 独立窗口 | ☐ |
| BranchedStateEngine / 工单业务闭环 | ~~分支/会签类型 ≥2 等~~ **随 §23 暂缓**（内部引擎优先，自研兜底） | 引擎本体不再实施（~~§22.5~~ 被 §23 推翻）；翻案条件见 design-decisions §23 | ☐（2026-09-02 暂缓） |

> **例外（2026-09-02 §23）**：~~BranchedStateEngine 引擎本体为 Phase 3 硬交付~~ **已推翻**——工单自研整体暂缓，引擎不再作为硬交付；触发信号只决定「翻案后是否恢复自研」，不决定「当前是否写引擎」。

---

## 9. ⚠️ 不确定项清单（实现前逐项确认）

| # | 事项 | 当前状态 | 影响 |
|---|---|---|---|
| U1 | ~~03-audit-l2 写入管道：同步落库 vs Redis List 缓冲、fail-open vs fail-close~~ | ✅ **已拍板（2026-09-03）：异步写**（channel → Redis List → 批量落库；03 §7 D1 已同步）；fail-open 随 E-① 实现确认 | 已关闭 |
| U2 | SLA 暂停态清单、邮件矩阵最终细节 | ~~7-0 决议已定方向，细节待 7-0 修订~~ **随 §23 暂缓**（7a/7b 不再实施） | ~~7a/7b 实现~~ |
| U3 | 7e 报表指标口径（P50/P95 分桶、时间窗） | ~~四缺口已收口，口径待细化~~ **随 §23 暂缓**（7e 不再实施） | ~~7e 实现~~ |
| U4 | 7c 审批流引擎人日（8–12 为估算，1500–2500 行） | ~~按 4.5(2) 优先级分步~~ **随 §23 暂缓**（引擎不再实施） | ~~M2 工期~~ |
| U5 | 前端工程量与是否并行 | **随 §23 暂缓**：审批/报表前端不再实施；仅 BK-18 管理页/动态表单仍为 IW3 独立窗口 | ~~M2 交付范围~~ → IW3 |
| U6 | ~~多实例验收环境~~ 随 M1 后置，触发时再备 | 需 2 实例 + Nginx | M1（🚦）触发时 |
| U7 | 迁移号最终占用（与 2b-ext 附件竞争） | A2 规则已定，启动时核对 | 迁移规划 |
| U8 | 启动是否翻转 roadmap「暂缓」状态 | 由所有者启动时确认 | 文档状态 |
| U9 | 日历排期（人日→日期） | 待给启动日 + 人力 | 本计划展开 |
| U-A | M-A activelist 澄清后核心功能人日（动态数据模型薄层：任意类型 int/string/列表、id 分页、PG 每类型表+JSONB、导入导出 JSON） | 约 1.5–3（⚠️ 待校准，原单估） | M-A 工期 |
| U-B | ~~**共享 utils 抽取**：zhuzhao `internal/pkg` → 独立共享项目~~ | ✅ **已完成（2026-09-03）**：zhuzhao-utils v0.1.0 已发布并 pin（无 replace）；9 包齐（crypto/errcode/jsonutil/jwt/logger/postgres/redis/response/validate）；resource 按 §25.3 拍板留 zhuzhao 不抽 | 已关闭 |

---

## 10. 门禁与验收

- **每 Wave 退出标准**：见 §1 表（M-E/M-HR 退出标准；MI1–5 仅 M1 🚦 触发时；~~TB1–16/FE1–4~~ 随工单暂缓）。
- **全量门禁**：`make lint` + `make test-unit` + `make test-integration`（13 包 `-race`）+ `make acceptance` 四档链式。
- **Phase 3 验收**：[README §3.1](./README.md#31-phase-3-验收两档) —— Phase 3-min（单实例内网）/ 3-full（多实例/SLO）；不得把 Grafana/Prometheus 未部署视为 App 启动失败。
- **文档同步**：每次改动回填 [11-project-control](../review/11-project-control.md) 能力矩阵；迁移 up/down 成对、编号全局唯一。

---

## 11. 风险与缓解

| 风险 | 缓解 |
|---|---|
| ~~**M-E 自定义脚本任务安全（os/exec 执行任意脚本）**~~ **2026-09-03 随脚本任务降级 🚦**（启用时再评估：仅全局管理员可配、超时强杀进程组、执行留痕、脚本与凭据分离；按 07-security-enhance 纳入限流/审计） |
| 审计归档正确性（导出删行一致性） | B11② 按「导出 JSONL 成功后删行」原子语义；归档漏跑可容忍（非并发敏感），周期性任务加阻塞/去重策略 |
| 多实例一致性（Watcher 失效窗口） | redis-watcher 推送 + StartAutoLoadPolicy 1min 兜底（双保险，MI2 验证；仅 M1 🚦 触发时） |
| 迁移号竞争（附件/HR） | A2 规则；启动时一次性核对占用 |
| 事件机制实现偷工（ADR-001 红线） | L1 按产线级实现（processed/分布式锁/audit-signal 分离），不因「以为 L1 是过渡」降级 |
| ~~审批流引擎复杂度 / SLA 正确性 / 前端节奏脱节~~ | ~~随 §23 暂缓~~（自研不做；对接参考见 §4 保留内容） |

---

## 12. 变更记录

| 日期 | 说明 |
|---|---|
| 2026-09-02 | 建档：基于 phase3/README Wave 结构 + 10-ticket-business + 检查单，产出可执行排期计划（人日估算 / 里程碑 / 依赖 / 🚦 触发项 / ⚠️ 不确定项）；同步补齐 03/06/07/08/09/ops-deployment 文档 |
| 2026-09-02（所有者六项拍板，design-decisions §22） | ① M1 降 🚦（M2 硬依赖收窄为 Asynq 底座，防重按 02 号约定写码）；② HR 同步升主链（M2.5 预留接口版：HRFetcher+引擎+mock adapter）；③ M4 拆两半（写侧管道本次做 / E13·G1·G3 蓝图 🚦 随 activelist 独立项目；独立数据库拍板认可）；④ 签发模型 = 指派给人（转派部门端点不做；viewer 能力走 AssignMenus 运营配置；通知「跟人 vs 跟单」进 7-0）；⑤ 7c 完整引擎维持硬交付（参考 easy-workflow）；⑥ 引擎可替换约束（WorkflowEngine 接口 + 实例状态通用/私有分层，7-0 拍板契约）。7-0 新增拍板项：ActivelistWriter 契约+审批通过触发点、通知/SLA 语义、引擎接口契约 |
| 2026-09-02（**重定位：工单自研暂缓（内部引擎优先，自研兜底）**，design-decisions §23） | 所有者明确：项目迁移公司内部并**对接内部工单平台/引擎**——§22.5（引擎照做）被推翻，§4 工单业务闭环全部暂缓自研（10/12 号转对接参考），Phase 2 工单现状封版（仅保 BK-20 数据安全修复）。主链重排：M0 收窄 → **M-E 事件基建**（Asynq + 审计归档首任务，非工单闭环）→ **M-A activelist 独立实现**（独立库+独立库数据库；外部事件接入契约改由 activelist 侧定义）→ **M-HR HR 同步**（内网迁移前置）→ M-Mig 迁移准备；M1/M5 随部署形态 🚦；B11① 判定日志可与 M-E 合并；工单对接形态与 Phase 2 资产处置 = 迁移时拍板（🚦）。翻案条件见 §23 |
| 2026-09-02（M-E 扩展为任务平台） | 用户新增需求：平台支持自定义任务（预置能力 + 用户自定义 Python/Shell）——M-E 扩展：Asynq 底座 + 预置任务（审计归档首个）+ 自定义脚本任务（job_configs/job_runs 两表 + os/exec 执行器 + 管理端点；安全边界 = 仅全局管理员可配/超时强杀/执行留痕/凭据分离，design-decisions §23 补充拍板）。参照：执行器模式照 ginfast scheduler（已核验可用）、管理面范式对齐 xxl-job GLUE、运维视角先用 asynqmon。运行时依赖（镜像带 python/shell 解释器）记入 M-Mig |
| 2026-09-02（M-SSO 新增） | 登录对接公司 SSO，协议拍板 **OAuth2.0 授权码模式**（design-decisions §24）：SSOProvider 接口 + 两端点 + 身份映射（对账键同 HR）+ 登录审计 method；**鉴权三层零改动**（callback 签发自有 JWT/RT）；JIT 默认关（config 开关）；本地密码兜底并存；单点登出联动 = 可选项迁移时拍板。插位 = M-HR 之后，2–3 人日；前端仅登录页 SSO 入口（后置策略下最小例外） |
| 2026-09-03（activelist 职责收敛，ADR-003 修订） | **M-A 收窄为动态数据模型平台**：事件与审计移交 zhuzhao（事件 = zhuzhao Asynq 业务操作点显式发布；审计 = zhuzhao 侧记录），进程 3→1，独立部署保留；业界对标确认同类开源（NocoBase/Teable/Twenty 等）均连带事件/审计/UI，自研薄层合理；**新增共享 utils 决策**：zhuzhao `internal/pkg` 抽独立共享项目（activelist 复用，M-A 前置 🚦）；M-A 人日由单估下调为约 2–4（⚠️ 待校准） |
| 2026-09-03（M-E 收敛为事件/任务总线 + 独立部署拍板） | **M-E 从「任务平台」收敛为「事件/任务总线」**：脚本任务（上传 python/shell）**暂不需要、降级 🚦**（Dagu 调研落 [15](./15-script-platform-dagu-vs-inhouse.md)，结论 = 轻量场景不需要那么全面，按需再启）；核心 = Asynq 事件/任务总线**异步触发预置动作**（预置 handler 代码注册：审计归档/通知/SLA/外部回调按需）；**部署拍板 = 独立部署**——zhuzhao 仅作网关（鉴权/编排/审计），调用各能力、拉起各任务；事件/任务 worker 服务独立部署，**不与 zhuzhao 布一块**（形态细节 ⚠️ 待定，见 §1 注记） |
| 2026-09-03（M-E 形态定稿） | **独立仓库 + 独立部署 + 独立 Redis**；**预置动作 = 回调 zhuzhao 内网端点**（业务 handler 在 zhuzhao，taskrunner 只做通用调度/触发/重试）；**日志边界**：zhuzhao Enqueue 记「任务提交日志」（action + task_id + request_id，业务审计在 audit_logs），taskrunner 自维护 `job_runs` 执行细节、**不传回 zhuzhao**，request_id 关联跨查；新增预置动作 = zhuzhao 加内网端点 + taskrunner 加配置（无需重部署 taskrunner） |
| 2026-09-03（需求澄清 · 最终画像） | 逐条澄清收窄：任意自定义类型（字段=`int`/`string`/列表）、activelist 零认证、查询=仅 id 分页+时间倒序、百万行内、**敏感高危数据→可靠**（存储加密暂不需要 ⚠️）、低频 Schema 演进、软删保留、导入导出 JSON（幂等/并发）、id 自增；**存储引擎评审：PG 仍为最优**（技术栈统一/自增/事务 vs Mongo Change Stream 已失效），模型=**每类型表+`data` JSONB**（零 DDL）；M-A 人日再下调为约 1.5–3（⚠️ 待校准） |
| 2026-09-02（M-SSO 降 🚦） | 所有者拍板：SSO 现在**只预留设计、进公司内网后实施**——§24 蓝图定稿（接口/端点/映射/JIT 开关/兜底策略全在设计内），实施触发 = 拿到公司 OAuth2.0 接入信息；主链执行序中 M-SSO 移出，与 M-Mig 汇合 |
| 2026-09-03（外部集成配套 16 号建档 + 目录同步） | 新增 [16-external-integration](./16-external-integration.md)：taskrunner/activelist 两仓库契约的 zhuzhao 侧能力清单（E1–E6 / D1–D5）+ 实施细排 + 迁移规划 + 待拍板项 P1–P7，M-E/M-A 行挂引用；同步本计划：M-A 行导入幂等口径改「全量替换」（方案 D 定稿）、共享 utils 前置标 ✅ 已完成（v0.1.0）、U-B 关闭 |
| 2026-09-03（AK/SK 基线修订） | 所有者拍板：服务间通信统一 **AK/SK HMAC 签名**（utils `aksk` C8 先行；覆盖当日「零认证+拓扑」基线与 P5/C2/activelist 零认证三条；关闭 B2 身份断言——明文 X-Operator 入签名覆盖；M-E 行挂 C8/E-②/E-④，M-A 行批次 B 挂验签） |
