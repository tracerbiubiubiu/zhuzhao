# 03 · 发散架构提案

> 性质：发散研究稿（见目录 README）。每条提案固定四节：`[现边界]`（现行决策/实现）、`[本稿建议]`、`[业界出处]`、`[代价与风险 / 对已实现架构的影响]`。标 ⚠ 翻案 的条目触碰既有拍板，采纳即须按文档同步纪律落档。

---

## A1 · 统一 Action 抽象（全局钥匙）

**[现边界]** taskrunner 任务 = `action_id`（不透明字符串）+ `params`（无 schema 的 JSON ≤64KB）；能力目录 `capability_registry` 方案已定稿、实现后置（触发条件：第二执行端 / zhuzhao 薄化 / 手填负担三者其一）；S3a 动作库"提交任务"为理想态登记。

**[本稿建议]** 把"能力"升格为平台一等公民 **Action**，统一五类来源：

| 来源 | 现状 | 统一后 |
|---|---|---|
| 属主服务 handler（如 audit_archive） | jobs registry 注册 | `kind=handler`，exec=zhuzhao 回调 |
| 外部系统 API（SOC/资产/SOAR） | 无 | `kind=connector`，exec=连接器名+操作 |
| 上传脚本 | 无 | `kind=script`，exec=脚本包@版本（05 号契约） |
| 平台内建（建单/通知/打标） | 散落在 service | `kind=builtin` |
| AI 工具 | 无 | 同一张表的只读视图（D10：直接以 MCP server 形态暴露） |

注册表草图：

```sql
actions (
  action_id        text,            -- 稳定标识，如 ip.intel.enrich
  version          text,            -- semver；同 id 多版本并存，默认取 active 最高
  kind             text,            -- handler|connector|script|builtin
  display_name     text,
  input_schema     jsonb,           -- JSON Schema（draft 2020-12 子集）
  output_schema    jsonb,
  exec             jsonb,           -- {target:"zhuzhao-callback"} | {connector:"soc-a",op:"block_ip"} | {script:"pkg@1.2.0"}
  permission_code  text,            -- 调用所需权限码（接三层鉴权，默认 task:submit）
  idempotent       boolean,         -- 可安全重试
  timeout_default  int,
  retry_policy     jsonb,           -- {max:3, backoff:"exponential"}
  owner, status(active|deprecated|disabled), created_by, created_at, updated_at,
  PRIMARY KEY (action_id, version)
)
```

四个消费者一次打通：① **前端参数表单自动生成**（JSON Schema → 表单；S1 自研渲染器的七类型本就是 schema 驱动，可共用渲染族）；② **DAG 节点类型**（A5 的 action 节点只认 action_id）；③ **工单动作库**（S3a "提交任务"直接列 actions）；④ **AI 工具**（function calling / MCP 的 tool 定义就是 name + input JSON Schema）；⑤ **OpenAPI 导出**（`GET /v1/actions/openapi` 由注册表生成——SOAR 等 REST 习惯的外部集成方直接消费；09 号补强）。

校验落点：taskrunner `Submit` 对 input 做 schema 校验（失败 4xx，复用 errcode 族）；回调/完成时对 output 校验（第一档 warn 不 fail）。

**[业界出处]** XSOAR integration YAML（command 的 arguments/outputs 声明 → 自动表单）、StackStorm pack 元数据、MCP tool schema——三者都证明"schema 即契约"是收敛点。

**[代价与风险]** 一张表 + 校验中间件 + 注册 CLI，1–2 周。对已实现架构：taskrunner `action_id` 字段语义不变，仅加 FK 到 actions（存量 audit_archive 以 `input_schema={type:object}` 宽松注册迁入）；zhuzhao jobs registry 变为 `kind=handler` 的注册适配器。**无破坏性变更**。

---

## A2 · 脚本执行面：sandbox-runner 作为第二执行端

**[现边界]** 任务唯一形态=属主服务回调；"上传 python/shell 脚本"在 16 号 taskrunner 不做清单、15 号降级 🚦；phase5 S4 段尾备忘"仅备忘不构成触发"。

**[本稿建议]** 新增独立服务 **sandbox-runner**（第二执行端——这正好命中 capability_registry 的第一个触发条件），职责单一："拿到一次脚本运行指令，在隔离环境里按 05 号契约跑完，回报结果"。taskrunner 仍是唯一调度/受理面，不膨胀。

```
zhuzhao ──submit(action kind=script)──▶ taskrunner ──dispatch──▶ sandbox-runner
   ▲                                        │                        │ 一次性执行环境
   └──────── result callback ◀──────────────┘◀──── report ──────────┘ (stdin/stdout/stderr/artifacts)
脚本包：S5 对象存储（zip: manifest.yaml + entry.py + requirements.lock） ──注册──▶ actions(kind=script)
```

隔离三档（随 D2 威胁模型选）：

| 档 | 形态 | 隔离强度 | 前提 |
|---|---|---|---|
| ① 容器池 + 子进程 | runner 容器内 `subprocess` 跑脚本：非 root、rlimit、seccomp 默认档、只读 rootfs、工件目录可写、默认禁网 | 中（防事故不防恶意） | 无 docker socket，compose 即可 |
| ② 每运行一容器 | runner 经容器运行时 API 为每次运行起一次性容器 | 高 | runner 持有运行时 socket（唯一特权组件，独立主机） |
| ③ microVM | gVisor/Firecracker | 很高 | 基础设施能力 |

**建议第一档起步**：D2 若定为"仅管理员上传 + 审批工单"，威胁模型是"失误"而非"恶意"，①足够；D2 若开放更多上传者，再升②。两档对脚本契约**完全透明**（契约只认 stdin/stdout/env/exit code）。

依赖策略：内部 PyPI 镜像 + `requirements.lock` hash 锁定；**包审批通过时预构建 venv 缓存**（按包版本），运行时零安装（否则每次运行拉依赖=慢且不可控）。

**[业界出处]** Cortex analyzers/responders 的每分析器容器化、XSOAR 每 integration 镜像、Lambda 的三通道分离。

**[代价与风险]** 新服务 2–3 周 + 契约/SDK/脚本库管理面 1–2 周。风险：出网白名单在 compose 下的实现（第一档建议"默认禁网 + 经内部出站代理放行白名单域"）；日志体积（契约有上限）；依赖供应链（镜像源 + 锁定 + 审批解决）。

⚠ 未计成本补记（2026-10-20 双审计；D28 已拍采纳——同日二轮）：① taskrunner 出站现为服务端定址单一回调目标（02-G2 现状）——sandbox-runner 第二执行端需扩定址白名单（配置级但须随批）；② 分钟级长脚本的回调超时窗口适配（现回调模型按秒级 handler 假设）；③ "内部出站代理"为新组件（04 号未设计）——设计节随 phase8 前置批补；④ taskrunner→sandbox-runner 的 dispatch / report 协议本身是执行模型变更（现 SSOT 为服务端定址单目标回调），须出协议设计而非仅配置；⑤ 分钟级脚本的租约续期与幂等栅栏继承（taskrunner 现栅栏按秒级 handler 假设）——④⑤为三审 E-E1 扩入 D28（2026-10-20，所有者 ok）。

---

## A3 · 域事件持久化：outbox 中间态 ⚠ 翻案提案

> **一句话解释（所有者 2026-10-20 晚问"域事件持久化指的是什么"）**：现在 S6 的事件是"业务事务提交后，在内存里把 DomainEvent 发给各订阅者"——进程此刻崩了、或某个订阅者处理失败，这条事件就没了（at-most-once），只能靠对账扫描事后补。"持久化"= 把事件**先写进数据库的一张表**（与业务改动同一个事务，所以业务成功事件必在），再由一个派发器从表里读出来投给订阅者、按订阅者记游标——事件不丢（at-least-once）、可重放、可审计"当时到底发了什么"，外部系统（SOAR / webhook）也能可靠消费。它**不是 Kafka**，就是 PG 一张表加一个轮询 goroutine；对订阅者代码零改动（11 号 §5.10 已登记此升级路径）。什么时候需要：当某个消费者"漏一条就出事"时——风险单联动关闭、工作流推进、给 SOAR 的结果回传。——**✅ D7 已拍采纳（2026-10-20 二轮，所有者"按建议"）**：翻案生效；落地时点 = phase9 工作流推进（触发式、不提前建），S6 一期仍按 at-most-once 现状 + 事件结构体可序列化留门实施。

**[现边界]** phase5 拍板"触发 = 显式订阅接口，**非 event bus**"；S6#4 落 DomainEvent/DomainHook（进程内钩子）；phase6 要求"事件面可订阅、禁硬编码"。

**[本稿建议]** 保留"显式订阅、代码注册"的语义，**加一层持久化**：

```sql
domain_events (
  event_id uuid PK, type text, aggregate_kind text, aggregate_id text,
  occurred_at timestamptz, actor jsonb,   -- {user_id, on_behalf_of, principal}
  payload jsonb, dedup_key text, created_at timestamptz
)  -- 追加写，按月分区，保留期可配
event_subscriptions (name PK, type_filter text[], cursor bigint, status, last_error, updated_at)
```

写侧：业务事务内同事务追加 `domain_events`（outbox 模式，天然与业务一致）；读侧：zhuzhao 内一个 dispatcher goroutine 按订阅游标拉取、投递给进程内订阅者（S6 通知、工作流触发、风险联动、webhook 出站）；至少一次 + 消费者幂等（订阅者用 event_id 去重）；**可重放**（重置游标）。同事务强一致的消费者（如状态变更的同步校验）仍走现有 DomainHook，两者并存。

不引 Kafka/NATS/RabbitMQ：PG 一张表就是 broker，与"不建 event bus"的初衷（不引基础设施、不做泛化总线）一致，翻案的只是"事件要不要落盘"。

**[业界出处]** Transactional outbox（微服务界标准模式）；SOAR 的 incident 事件流都是持久化的，否则 playbook 触发不可靠也不可审计。

**[代价与风险]** 一表一 goroutine 一游标表，≈ 1 周；风险：事件表膨胀（分区+保留期）、投递延迟（tick 秒级，可接受）。**对 S6 的影响**：S6#4 若已按进程内钩子实现，追加 outbox 是增量不是重写——建议 S6 一期实现时把事件结构体设计成"可序列化"，为本提案留门。

---

## A4 · 调度分工收敛（2026-10-20 晚修订：由"全收敛"改为"分工口径"）

**[现边界]** zhuzhao 进程内 CronScheduler（ticker + PG advisory lock，P4 收归；S10 将为其补可观测面，phase6 三个扫描任务按 README 口径骑它）与 taskrunner cronloop（分钟级、无分布式锁）并存；audit_archive 双形态部署二选一。

**[本稿建议]**（D18）两层分工、各守边界：
- **进程内 CronScheduler = 平台自身维护型扫描**：超期 / 到期 / 终态对账 / GC 这类需要同进程 DB 事务、单实例 advisory lock 已就位、S10 可观测面覆盖的任务——phase6 扫描任务维持原口径。
- **taskrunner = 业务任务、脚本、连接器拉取、以及一切需要重试·死信·审计·UI 可见的周期工作**——统一接入层的 http-pull / db-pull / file-pull 全部在此。
- **纪律：CronScheduler 不再承接业务任务**；taskrunner cronloop 补分布式锁（PG advisory 或 Redis SETNX 带租约）、`overlap_policy`（skip/queue/allow）、`run_at` 一次性延迟任务 API、jitter。
- 升级路径：若多副本或"统一可观测面"成为硬需求，再把维护型扫描迁为 taskrunner jobs（audit_archive 双形态即先例），不在 phase5/6 做。

早先"taskrunner 成为唯一调度面、CronScheduler 退役"的全收敛方案撤回：它与 phase6 README 和 S10 既定设计正面冲突，而维护型扫描对"同事务 + 本地锁"的需求是真实的，全收敛收益不抵改动。

**[业界出处]** xxl-job"一调度中心 + N 执行器"（taskrunner 文档已注记）用于业务侧；进程内维护 cron 是单体服务的常规形态。

**[代价与风险]** 口径本身零成本；cronloop 增强 0.5–1 周（随 phase7 地基批）。风险：双轨边界要在 standards 或 11 号登记成文，否则会再漂。补强（09 号）：phase7 M4 加"计划任务总览"只读视图——并列两调度器的任务 / 上次运行 / 下次运行 / 失败态，防双轨割裂。

---

## A5 · 轻量 DAG 编排引擎（自研，留逃生门）

> **状态（2026-10-20 晚，两次口径）**：**Stage 2 既定排期**——公司已有 SOAR，Stage 1（phase7–8）任务编排由 SOAR 承担、平台先做能力提供方（A1 + A2 + 连接器）；**但所有者明确 SOAR 只是过渡、编排最终全部迁回平台**：phase9 本提案的引擎 MVP 先跑平台自有链路，phase10 playbook 设计器 + SOAR playbook 逐个重建（"SOAR 动作 → 平台 Action"映射表为前置，phase8 能力迁移时同步维护）+ 双跑期（以平台结果为准）+ SOAR 退役（07-§4.4–4.5、D1）。

**[现边界]** 16 号红线"taskrunner 不做通用流程引擎；首选外部 DAG 引擎逐节点调 API；最小替代 on_success 钩子 + result 摘要列"；S2 分层"画布层可复用、引擎不合并"。

**[本稿建议]** 在 zhuzhao 内（非 taskrunner）新建 **workflow 引擎**，节点执行一律委托 taskrunner（复用其幂等/重试/死信/审计），引擎只管"图的推进"：

```sql
workflow_definitions (id, name, version, graph jsonb /*nodes+edges，S2 画布 schema 适配层产出*/,
                      status draft|active|retired, org_id, created_by, ...)
workflow_runs (run_id, definition_id, version, trigger jsonb /*{kind:manual|schedule|event|webhook, ref}*/,
               status, inputs jsonb, context jsonb, started_by, on_behalf_of, started_at, finished_at, error)
step_runs (step_run_id, run_id, node_id, attempt, status, action_ref, input jsonb, output jsonb,
           idempotency_key text /*= run_id+node_id*/, task_id /*→ taskrunner*/, started_at, finished_at, error, log_ref)
```

节点类型（第一档六种）：`action`（A1）· `script`（A2，本质也是 action）· `approval`（创建审批/工单并挂起，工单终态经 A3 事件唤醒——**审批节点就是人机分治的接缝**）· `wait`（定时或事件）· `branch`（条件表达式——复用 S1 已选型的 expr-lang 求值器，白名单函数）· `parallel/join`。

推进模型：事件驱动（step 完成 → 评估后继），DB 为真相源，每个 run 一把 advisory lock 防并发推进；崩溃恢复 = 扫描非终态 run 重评估（与 taskrunner ReclaimLoop 同款思路）。

逃生门：定义/运行/步骤三抽象后面放 `WorkflowEngine` Port（先例 = TicketEngine Port），未来若引 Temporal，适配器替换、表与 API 不变。

**[业界出处]** XSOAR/Shuffle/Tines 的 playbook 内核均为自研 DAG + action；Temporal 作为逃生门候选。

**[代价与风险]** 3–5 周（含 S2 画布适配；量级校准注：合并卡审计建议 4–6 周——状态机 + 并发推进 + 崩溃恢复 + S2 适配，参照 S3a ~2 周复杂度量级——已并入 D25 量级重估项一并定，原值暂留）。⚠ 对 16 号红线的关系：红线禁止的是"taskrunner 变成通用流程引擎"与"工单流程与任务流程捏成通用模块"——本提案引擎在 zhuzhao、执行委托 taskrunner、与工单引擎分离，**是在红线内扩张而非翻案**；但"自研 DAG 优先于外部引擎"与红线"首选外部 DAG"相反，须所有者拍 D1。

---

## A6 · 非人身份与委托链

**[现边界]** 用户主体 + PAT；服务间 AK/SK HMAC + X-Operator 明文断言覆盖签名（B2）；无 service account 类型、无 on-behalf-of 语义。

**[本稿建议]**
1. **主体类型**：新增 `principals`（或 users.kind=service）——service account 持 AK/SK、绑定角色/权限码、不可登录、可吊销、按资源范围限定（如仅 `task:submit` + 指定 action 前缀）。D9 建议专用类型而非扩展 PAT：PAT 语义是"人的令牌"，审计里出现的应是"哪个服务"。
2. **委托链**：请求携带 `actor`（发起人）/ `principal`（执行主体）/ `on_behalf_of`（被代理人）三元组；权限判定**以被代理人为准且不得超出执行主体的能力上限**（取交集）；审计三字段全记。
3. **载体**：内部调用在签名 canonical 串追加 `X-On-Behalf-Of`（canonical 变更 = utils aksk 协议版本升 v2，三仓同步，与 nonce 校验一并做）；或第一档放请求体避免协议升版——建议第一档走请求体，协议升版随 nonce 批。
4. **AI 主体**：`principal=ai-assistant`，恒 on-behalf-of 当前用户，审计打 `via=ai` 标记（D8）。
5. **委托策略表（09 号补强，补漏）**：`delegation_policies(principal_id, subject_scope{users[] | orgs[] | any}, allowed_action_prefixes[], allowed_biz_types[], expires_at)`——on_behalf_of 断言先过策略、再做权限交集；**无策略 = 拒绝**，否则任何可信调用方都能代任何人。SOAR、脚本沙箱、AI 主体各一条策略，范围最小化。

**[业界出处]** OAuth token exchange / on-behalf-of 流、XSOAR playbook 运行身份、零信任服务身份。

**[代价与风险]** 1–2 周；风险：三层鉴权判定逻辑需引入"交集"语义（L1 菜单/L2 行级/L3 操作三层各自对委托的解释需逐层定义——建议 L2 以被代理人为准、L3 取交集、L1 不适用于非人主体）。

---

## A7 · Webhook 入口站

**[现边界]** 入站仅 AK/SK HMAC（GinMiddleware）+ 存量 `x-auth-*` 兼容层；nonce 校验后置；无 webhook 概念。

**[本稿建议]** 统一入口 `POST /webhooks/in/{token}`（D6：token 为 opaque 秘密，映射到 `webhook_sources` 配置），能力：
- 每源验签方式可配：共享密钥 HMAC（头名/算法可配，覆盖多数 SOC/日志平台）、IP allowlist、后续 mTLS；
- **防重放**：时间戳 ±5min + nonce/摘要在 Redis 短存（webhook 必须一等公民，不能像内部 aksk 那样后置）；
- 限流：复用 Redis Lua 令牌桶，按源分桶；body 上限；
- 原始落盘：`raw_webhook_events(id, source, received_at, headers_subset, body, status)` 先存后处理（可重放、可排障）；
- 源适配器：每源一个 normalizer（代码注册，第一档）→ A8 的 `security_alerts`。

**拉取侧**（所有者 2026-10-20 补充：推/拉接口都要提供——曾出现需要我们主动去对方数据库拉的情况）：入口站只覆盖"推"；"拉"由三型连接器承担（`http-pull` / `db-pull` / `file-pull`，04-§4.10），taskrunner cron job 驱动，`ingest_sources` 水位表记录游标；拉取结果与推送进**同一原始表与同一归一化流水线**，同指纹去重即对账。A7 因此宜理解为"统一接入层"的推侧半边，首源 = phase6 漏扫平台（07-§4.1）。

**[业界出处]** GitHub/Stripe webhook 规范（签名头 + 时间戳 + 幂等 ID）、SOAR 的 webhook 触发器。

**[代价与风险]** 1 周；风险：URL 携带 token 与 standards"POST URL 不带业务信息"字面冲突（D6 建议豁免并在 standards 登记：token 是认证材料非业务参数）。

---

## A8 · 告警/事件域模型（原始独立、聚合复用风险单）

**[现边界]** 无告警实体；phase6 R1 风险单 = 汇聚台账（指纹去重）；phase6 明确不做"SIEM/SOAR 实时响应编排"。

**[本稿建议]**（D5）
```sql
security_alerts (
  alert_id uuid PK, source text, external_id text, received_at, occurred_at,
  severity_raw text, severity int /*1-5 归一*/, category text /*OCSF 骨架裁剪：auth|network|malware|vuln|policy|...*/,
  title text, entities jsonb /*[{type:ip|host|user|url|hash|account, value}]*/,
  asset_ref text /*外部资产平台 ID，解析成功时；平台不自建资产表*/, asset_snapshot jsonb /*解析时的名称/责任人/部门/等级*/,
  attack_tactics text[], payload jsonb,
  fingerprint text, status new|aggregated|suppressed|dropped, risk_item_id /*→ risk_items*/,
  UNIQUE(source, external_id)   -- 建为部分唯一索引 WHERE external_id IS NOT NULL（PG 多 NULL 不冲突）；external_id 为空的源以 fingerprint 为唯一去重键（三审 E-E2）
)
alert_routing_rules (id, priority, match jsonb /*expr-lang 表达式*/, action create_risk|attach|notify|suppress|start_workflow, params jsonb, enabled)
maintenance_windows (id, match jsonb, starts_at, ends_at, created_by, reason)
```
流水线：入口站 → normalizer → 资产解析（G1）→ 规则引擎（抑制/维护窗优先）→ 指纹 → 聚合到风险单（R1 语义：存在则计数+last_seen，否则建单）→ 事件（A3）→ 通知/工作流。

**不做**：日志存储与检索（日志平台的活，场景 10 用深链）、跨源关联分析（SIEM 的活）、实时自动响应（若公司有 SOAR 归它，D3）。

**[业界出处]** OCSF 字段骨架；XSOAR incident dedup；TheHive alert→case 提升模型。

**[代价与风险]** 表+规则+流水线 2–3 周；对 phase6 的影响：risk_items 需加 `source_kind`、`alert_count`、`first_seen/last_seen`、`attack_tactics`、`asset_ref/asset_snapshot`（外部资产引用+解析时快照，不建资产表）几列——建议 phase6 建表时**预留**而非日后加列。

---

## A9 · 事件分类学与统一钩子（不是新机制）

**[现边界]** S6#4 已定 `DomainEvent{BizType, Action, BizID, RefCode, From, To, ActorID, OccurredAt, Payload json.RawMessage}` + `HookRegistry / DomainHook`（编译期注册、事务提交后异步派发、at-most-once，`docs/phase5/06:118-134`）；S3a 有 transition validator / condition（行为型拦截）；taskrunner 运行结果无事件（查询是唯一出口），16 号登记 `on_success` 钩子为最小替代。

**[本稿建议]**（D20）所有者问"工单、风险事件是否都要 onSuccess / onFailed / onChange，能否统一抽象"——答：**统一，但统一在既有 DomainEvent 之上，不另起钩子体系**：

1. **分类学**：`Action = <biz_type>.<verb>`，verb 取固定词表——实体类 `created / updated / status_changed / assigned / commented / closed / reopened / deleted`；执行类（run = taskrunner job run、脚本运行、工作流步骤）`started / succeeded / failed / canceled / dead / timed_out`；系统类 `ops.action_failed / ops.cron_failed / ops.ingest_failed`。所谓 onSuccess / onFailed 就是订阅 `run.succeeded / run.failed`；onChange 就是 `*.updated`。
2. **载荷约定**：`updated` 必带 `changes: [{field, from, to}]`，订阅者按字段过滤（只在 severity 变化时通知）；`status_changed` 用既有 From / To；每个 Action 的 payload 登记 JSON Schema（与 Action 注册表同纪律，便于工作流 `wait` 节点与 AI 工具发现）。出站序列化信封对齐 **CloudEvents**（09 号补强）：`id`=event_id、`type`=Action、`source`=服务名、`subject`=`biz_type:biz_id`、`time`=OccurredAt、`data`=Payload——进程内结构体不变，webhook / outbox 投递用此映射，接收方可用通用 CloudEvents SDK 解析。
3. **两类钩子不混**：观测型（发生了什么）= DomainEvent；行为型（能否发生 / 发生前必须补什么）= S3a validator / condition——"关单前必须填复测结论"是 validator，不是事件。
4. **运行类事件补齐**：taskrunner 回调完成时由 zhuzhao 侧 Dispatch `run.succeeded / failed / dead / canceled`（16 号 `on_success` 的真正落点）；工作流引擎（A5）订阅它推进 DAG，S6 订阅它通知失败，A8 的 Job 型脚本 `emits` 也经此通道。
5. **一致性档位**：默认 at-most-once（现状）；需要 at-least-once 的消费者（风险联动、工作流推进、出站 webhook）按 11 号 §5.10 路径升级 outbox（A3），DomainHook 契约不变。

**[业界出处]** 领域事件命名惯例（aggregate.verb）、CloudEvents 的 type / source / subject 字段、SOAR 平台的 incident lifecycle hooks。

**[代价与风险]** 分类学与载荷约定零代码（S6 一期顺手采纳，见 08 号 A 类）；运行类事件 Dispatch 1–2 天随 phase7；风险 = verb 词表膨胀，须在 standards 登记词表并守住。

---

## 提案间依赖图

```
A1 Action ──┬──▶ A2 sandbox-runner（script 是一种 action）
            ├──▶ A5 DAG（节点只认 action）
            └──▶ AI 工具暴露（D10）
A3 outbox ──┬──▶ A5 的 approval/wait 节点唤醒
            ├──▶ A8 的告警→通知/工作流
            └──▶ S6 通知可重放
A6 身份 ────┬──▶ A5 run 的 on_behalf_of
            └──▶ A2 脚本以谁的身份出站
A7 入口站 ──▶ A8 告警域模型 ──▶ phase6 风险单（聚合实体）
A4 调度分工 ──▶ A5 的 schedule 触发（业务类）只来自 taskrunner；维护型扫描留进程内
A9 事件分类学 ──▶ A5 推进 / S6 通知 / A3 持久化 / A8 emits 共用同一 Action 命名与载荷约定
```

建议落地顺序：A1 → A6 → A4 → A2 → A5 →（并行）A7/A8 → A3 穿插（S6 一期时预留结构，A8 时正式落）。
