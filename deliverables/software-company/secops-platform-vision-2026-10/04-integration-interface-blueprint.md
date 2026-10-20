# 04 · 对接接口设计与预留蓝图

> 性质：发散研究稿（见目录 README）。回答"内部对接的接口怎么设计、现在怎么预留"。全部遵循 [docs/standards.md](../../../docs/standards.md) 既有公约（仅 GET/POST、POST URL 不携带业务信息、服务间 AK/SK HMAC、OKPage 分页、errcode 族），冲突处显式标注。

## 1. 总原则

沿用（不重申细节，只列与对接相关的）：
- **方法**：仅 GET/POST；POST 的资源标识与动作参数全部在请求体。
- **鉴权**：内部服务间 AK/SK HMAC（utils `aksk.GinMiddleware`）；存量 `x-auth-*` 调用方走 `aksk/compat`；用户面 JWT + 三层鉴权。
- **错误**：errcode 三形态；对外 2xx=成功、4xx=不可重试、5xx=可重试（taskrunner 回调契约同款）。
- **分页**：OKPage，page_size 钳制回显。

本蓝图新增五条约定（建议随首个采纳提案收编 standards）：
1. **Schema 即契约**：所有可被编排/脚本/AI 调用的能力必须有 JSON Schema 输入输出（A1）。
2. **幂等键**：所有"创建型 POST"接受 `idempotency_key`（请求体字段，非 header——与 POST 体承载业务信息的公约一致）；服务端按 (caller, key) 去重 24h。
3. **委托三元组**：`actor / principal / on_behalf_of` 第一档放请求体 `_ctx` 字段；协议升版后进签名 canonical 串（A6）。on_behalf_of 断言须先通过 delegation_policy（03-A6 ⑤），无策略即拒（09 号补强）。
4. **事件化输出**：任何会改变业务状态的对接动作，完成后发 domain_event（A3），而非由调用方轮询推断。
5. **原始先落盘**：外部进来的任何 payload 先存原始再归一化（可重放、可排障）。

## 2. 四象限视图

```
            ┌──────────────── 北向：用户面 ────────────────┐
            │  UI（zhuzhao-ui）/ REST（JWT+三层鉴权）/ MCP（AI）│
            └──────────────────────┬───────────────────────┘
 ┌─────────── 西/东向：服务间 ──────┼──────────────────────────┐
 │ zhuzhao ⇄ taskrunner ⇄ sandbox-runner ⇄ activelist           │
 │ AK/SK HMAC + X-Operator + 专用 network                        │
 └──────────────────────────────────┼──────────────────────────┘
            ┌──────────────── 南向：执行面 ────────────────┐
            │  Action：handler / connector / script / builtin │
            └──────────────────────┬───────────────────────┘
            ┌──────────────── 外部：生态面 ────────────────┐
            │ 入：webhook 入口站（每源验签）/ 上报 API（AK/SK）│
            │ 出：连接器（每连接器凭据）/ 出站 webhook（我方签名）│
            │ 拉：定时连接器拉取（漏扫结果平台/情报/资产按需查询）│
            └──────────────────────────────────────────────┘
```

## 3. 预留端点清单（草图，字段仅示意）

### 3.1 事件/告警入口

**内部上报（已鉴权系统）** `POST /internal/v1/events`
```json
{ "idempotency_key": "scanner-a:finding:8812",
  "type": "security.alert", "source": "scanner-a", "external_id": "8812",
  "occurred_at": "2026-10-20T08:00:00+08:00", "severity": "high",
  "category": "vuln", "title": "CVE-xxxx on host-01",
  "entities": [{"type":"host","value":"host-01"},{"type":"ip","value":"10.0.0.8"}],
  "payload": { "...": "原始字段" } }
→ { "code":0, "data": { "alert_id":"…", "status":"new|aggregated|suppressed", "risk_item_id":"…" } }
```
phase6 R1 的"上报 API"与此合并为同一端点（type 区分 alert/finding/manual）。**拉取侧**见 §4.10（推/拉皆备——所有者 2026-10-20 口径）：拉取结果经同一归一化流水线落同一张原始表 `raw_ingest_events`。

**外部 webhook** `POST /webhooks/in/{token}`（D6 豁免 URL 规范）——每源配置验签方式；响应恒 `202` + delivery id；处理异步（A7）。

### 3.2 Action 目录与调用

- `GET /v1/actions?kind=&status=&q=` → 列表（含 input/output schema，前端据此渲染表单；AI 据此生成工具列表）。
- `POST /v1/action-invocations`
```json
{ "action_id": "ip.intel.enrich", "version": "1.x", "input": {"ip":"10.0.0.8"},
  "idempotency_key": "ticket-1234:enrich:1", "_ctx": {"on_behalf_of": "E000002"} }
→ { "code":0, "data": { "invocation_id":"…", "task_id":"…", "status":"pending" } }
```
- `GET /v1/action-invocations?invocation_id=` → 状态/输出/日志引用/工件引用。
- `GET /v1/actions/openapi` → 由注册表生成的 OpenAPI 文档（每个 active action 一条 operation，input/output schema 直接复用），供 REST 习惯的外部集成方（SOAR 等）消费（09 号补强）。
内部映射：invocation → taskrunner Submit（kind=handler/script）或连接器直调（kind=connector，同步短操作可直接返回结果）。

### 3.3 资产解析（外部资产平台的查询门面——平台不自建台账，所有者 2026-10-20 补充）

`POST /internal/v1/assets/resolve`
```json
{ "identifiers": [{"type":"ip","value":"10.0.0.8"},{"type":"hostname","value":"host-01"}] }
→ { "code":0, "data": { "matches": [ { "asset_ref":"ap-7f3c…","kind":"host","name":"host-01",
      "owner":{"user_id":"…","name_no":"张三(E000002)"},"org_id":"…","criticality":3,
      "resolved_at":"2026-10-20T09:00:00+08:00","confidence":0.98 } ], "unresolved": [] } }
```
门面职责：调用资产平台连接器（解析/列举两操作）、字段归一、**键映射**（对方责任人键 → 我们的 users，对方部门码 → 我们的 org_id——L2 锚点自动填充的前提）、可选读穿缓存（S9 模式，TTL 分钟到小时级）、解析失败返回 `unresolved`。消费者只认门面，不直接耦合对方 API：告警流水线、脚本节点（SDK `zh.assets.resolve()`）、S1 字段联动（DataFetcher 首个企业外部源）、工单资产字段。自有实体存 `asset_ref` + `asset_snapshot`（解析时的名称/责任人/部门/等级，审计稳定——与审计快照列既有模式同款）。

### 3.4 工作流

- `POST /v1/workflow-runs` `{definition_id, version?, inputs, trigger:{kind:"manual"}, idempotency_key}`
- `POST /v1/workflow-runs/signal` `{run_id, node_id, signal:"approve|reject|resume", payload}`（审批节点由工单终态经事件自动 signal；此端点供人工/外部补位）
- `GET /v1/workflow-runs?run_id=` / `GET /v1/workflow-runs/steps?run_id=`
- `POST /v1/workflow-runs/cancel` `{run_id, reason}`

### 3.5 脚本包生命周期

```
POST /v1/script-packages/upload-url   {filename, sha256}        → 预签名上传 URL（S5 通道）
POST /v1/script-packages              {object_key, sha256}      → 解析 manifest、静态校验、落 draft
POST /v1/script-packages/submit       {package_id}              → 发起审批（创建工单，复用自家流程）
POST /v1/script-packages/approve      {package_id, decision}    → 审批终态→注册 actions(kind=script)、预构建 venv
GET  /v1/script-packages?status=&q=   / GET /v1/script-packages/versions?name=
POST /v1/script-packages/retire       {package_id}              → actions 置 deprecated，在途运行不受影响
```

### 3.6 运行日志与工件

- `GET /v1/runs/logs?run_id=&cursor=&limit=` → 行级分页（stderr 捕获的日志，带行号/时间/级别）；大日志超限标记 `truncated=true` 并给 S5 全量文件引用。
- `GET /v1/runs/artifacts?run_id=` → `[{name, size, sha256, url(预签名短时效)}]`。
- 运行记录 `GET /v1/runs?run_id=` 统一视图：taskrunner job_run + step_run + 脚本执行元信息（exit_code/duration/resource peak）。

### 3.7 出站 webhook（我方作为事件发送方）

- `POST /v1/webhook-subscriptions` `{name, event_types[], url, secret, enabled}`
- 投递头：`X-Zh-Delivery-Id`（唯一，接收方幂等用）、`X-Zh-Timestamp`、`X-Zh-Signature: sha256=HMAC(secret, timestamp.body)`；重试退避 + 死信可见（复用 taskrunner 死信面）。
- 投递体 = **CloudEvents 信封**（id / type / source / subject / time / data，映射见 03-A9；09 号补强），接收方可用通用 CloudEvents SDK 解析，无需学我们的私有格式。
- 对 SOAR/SOC/其它内部系统订阅我们的"工单状态变更/风险单创建/处置完成"事件——这是 D3 协同模式下"结果回流"的反向通道。

### 3.8 内部服务间新增契约

- taskrunner ⇄ sandbox-runner：`POST /v1/script-runs`（dispatch：package ref、input、limits、secrets ref、callback）/ `POST /v1/script-runs/report`（回报：exit_code、stdout JSON、stderr 引用、artifacts、metrics）——AK/SK HMAC 同款。
- 凭据下发：sandbox-runner 不直接读凭据库，由 taskrunner 在 dispatch 时按 manifest `secrets` 列表解析并随请求体加密传递（短生命周期），runner 注入 env 后即弃。

## 4. 逐平台对接设计

| 平台 | 方向 | 协议与鉴权 | 数据形状 | 我们预留 | 拍板点 |
|---|---|---|---|---|---|
| **4.0 漏扫结果平台**（phase6 首源；非扫描器直连，自动复测暂无计划） | 推 + 拉皆备（所有者 2026-10-20：正常对方推，曾需我们主动去其库拉） | 推：上报 API（AK/SK）或 webhook 入口站；拉：http-pull（对方 API）/ db-pull（只读账号 + 参数化 SQL 模板）/ file-pull（导出文件） | 发现：scanner / external_id / 资产标识 / vuln_id(CVE) / severity / port·service / first·last_seen / scanner_status / evidence | 统一接入层（原始表 + 源适配器 + 水位表）+ 指纹聚合进风险单 + R3 整改工单 + 人工复测关单联动（07-§4.1 / D13） | **四项均已定（2026-10-20 三审同步 E-D3）**：D12 db-pull 边界 · D13 联动策略已拍；D14 已修订为每源单模式不对账（§4.10）；稳定键由 D19 定为每源配置优先级（external_id 跨扫稳定则用之，否则 asset_ref+vuln_id+port——07-§4.1(d)） |
| **4.1 资产平台**（外部提供能力，只查询——所有者 2026-10-20 补充） | 出（按需查询：解析 + 列举）；不同步、不对账、不建本地台账 | 对方 API（Bearer/API Key 常见）→ 连接器凭据 | 解析：标识符 → 资产/责任人/部门/等级；列举：按 kind/部门取清单（巡检 Job 用） | 解析门面（3.3）+ 声明式 HTTP 连接器两操作 + 可选读穿缓存（S9 模式）+ 实体 `asset_ref/asset_snapshot` 字段 | 责任人键映射（工号？）、部门码 → org_id 映射、配额/时延（决定缓存）、是否支持批量解析、变更通知有无 |
| **4.2 日志平台** | 出：我们调其查询 API 做富化/深链；入：其告警 webhook 推我们 | 查询 API 鉴权（对方定）；告警入口站每源 HMAC | 查询=时间窗+实体+返回命中摘要；告警=其规则命中事件 | 场景 10 深链构造器（URL 模板按实体/时间窗）；入口站源适配器 | 是否要"日志摘要嵌入案例页"（需查询 API 配额）vs 纯深链 |
| **4.3 SOC** | 入：告警推送（主）；出：处置状态回传（我们处置完告诉 SOC） | 入口站（HMAC/IP allowlist）；回传走连接器或出站 webhook（3.7） | 告警（severity/category/entities/原始）；回传（alert_id ↔ risk_item_id 映射 + 处置结论） | A8 域模型 + normalizer；回传 Action `soc.alert.update_status` | 告警量级与聚合窗口；哪些类别默认进处置流（路由规则第一档代码内置） |
| **4.4 SOAR**（公司已有——所有者 2026-10-20 晚确认；过渡期编排留 SOAR，**终态编排迁回平台、SOAR 退役**） | 过渡期双向：入 = SOAR 调我们的 Action（编排在 SOAR，我们是能力提供方）；出 = 我们触发 SOAR playbook；SOAR 产出经统一接入层回流为事件。终态（phase10）：playbook 在平台重建，SOAR 仅余长尾或下线 | 入：SOAR 持 service account（A6）调 `/v1/action-invocations`（3.2；AK/SK 或 compat 层按其能力）；出：连接器调 SOAR API；结果：出站 webhook（3.7）或轮询 | 动作请求（action_id + input + idempotency_key + on_behalf_of）；结果（status / output / artifacts）；playbook 触发（playbook_id + 实体 + 审批凭证） | Action 注册表（A1）+ `/v1/action-invocations` + 出站 webhook + `soar.playbook.run` 连接器动作 + **"SOAR 动作 → 平台 Action"映射表**（phase8 能力迁移时建、phase10 playbook 重建时用） | SOAR 侧是否接受 on_behalf_of 归因；SOAR 调我们的鉴权形态（AK/SK vs 其既有 token 机制）；迁移优先级与双跑期（同一能力两边都有时以平台为准）；SOAR 现有 playbook 清单（phase10 精估依据） |
| **4.5 威胁情报** | 出（查询） | S9 缓存代理已排期；ThreatIntelProvider 端口 | IOC → 评分/家族/标签 | S9 即预留；脚本 SDK 封装 `zh.intel.query()` 走 S9 | 配额降级策略（S9 启动拍板点已在） |
| **4.6 UAA（认证/权限数据面）** | 17 号已全设计（SSO + MQ 三消费者 + 回传） | OAuth2 授权码 + amqp091 | — | 本蓝图不重复；仅补一条：**service account（A6）不走 UAA 同步**，本地管理 | source 列 **✅ 已拍方案 a**（U1，17 号 §6#7，随 phase6 建表批合批——2026-10-20 三审同步 E-D3，原"已悬空"已过期）；与 4.1 的 source 列共用方案 |
| **4.7 统一消息平台** | 出 | S6 ChannelSender 端口 | 模板+收件人+渠道 | S6 即预留；本蓝图新增消费者 = 告警通知/工作流通知/值班升级 | 通知聚合防轰炸（06-F9） |
| **4.8 HR** | 入（组织/人员同步） | M-HR HRFetcher（⚠ 接口未落码） | 组织树/人员/岗位 | 与 UAA 件 4 定主备后统一；资产责任人解析依赖人员主数据完整 | M-HR 去留（17 号 §6#8） |

### 4.9 声明式 HTTP 连接器（通用件，覆盖多数对接零代码）

```yaml
connector: soc-a
kind: http
base_url: https://soc-a.internal.example      # config 化，禁硬编码
auth: { type: api_key, header: X-API-Key, secret_ref: SOC_A_API_KEY }   # 或 bearer / basic / oauth2_cc / aksk / hmac_custom
limits: { rate_per_sec: 5, timeout_sec: 10, retries: 2, circuit_breaker: { failures: 5, cooldown_sec: 60 } }
health: { method: GET, path: /api/health, expect_status: 200 }
operations:
  update_alert_status:
    method: POST
    path: /api/v1/alerts/status
    body: { alert_id: "{{ input.external_id }}", status: "{{ input.status }}", note: "{{ input.note }}" }
    input_schema:  { type: object, required: [external_id, status], properties: { external_id: {type: string}, status: {enum: [open, resolved, false_positive]}, note: {type: string} } }
    output_map:    { ok: "$.success", message: "$.message" }
    output_schema: { type: object, properties: { ok: {type: boolean}, message: {type: string} } }
```
每个 operation 自动注册为 `actions(kind=connector)`；模板引擎用受限模板（非 text/template 全功能，防注入——与 S6 占位符替换的选型纪律一致）；复杂对接（分页拉取/多步握手）写 Go 代码连接器实现同一 `Connector` 接口。

**[业界出处]** XSOAR 的 Generic HTTP integration、n8n HTTP 节点、Tines HTTP action——"声明式 HTTP 连接器覆盖八成对接"是行业共识。

### 4.10 拉取型连接器（推/拉皆备——所有者 2026-10-20 口径）

| 型 | 形态 | 水位 | 安全边界 |
|---|---|---|---|
| `http-pull` | 周期调对方 API（分页 / 增量参数） | 时间戳 / 游标 / 页码 | 连接器凭据 + 限流 + 超时（同 §4.9） |
| `db-pull` | 周期对对方数据库执行**参数化只读 SQL 模板** | 水位列必填（updated_at / 自增 id） | 只读账号；SQL 模板审批入库、禁自由 SQL；行数 / 时长上限；网络白名单；每次拉取审计（D12） |
| `file-pull` | SFTP / 共享目录导出文件（CSV / JSON） | 文件名 / 修改时间 | 文件大小上限 + 格式校验 + 处理后归档 |

统一约定：拉取由 taskrunner cron job 驱动（03-A4 / D18 分工口径：业务 / 拉取 / 脚本类周期工作的唯一来源）；`ingest_sources(source, mode, adapter, cursor, schedule, last_run_at, last_status, last_error, enabled)` 记录水位；拉取结果与推送进**同一原始表 `raw_ingest_events`** 与同一归一化流水线；**每类数据只走一种模式**（D14 修订），指纹去重防重投 / 重拉而非推拉差异；源与平台数据一致性按 D19（字段归属 + `source_observed_at` 时间判优 + 每源稳定键配置，07-§4.1(d)）；失败重试与死信复用 taskrunner；每源一个 normalizer（代码注册）。**每个 db-pull 源须登记退出条件**（对方提供查询 API / 导出即切 http-pull / file-pull）——db-pull 是历史特例的兜底，不是常态（09 号补强）。

```yaml
connector: scanner-a
kind: db-pull
dsn_ref: SCANNER_A_RO_DSN              # 只读账号，凭据库引用；禁明文
schedule: "0 */6 * * *"               # 6 小时一次（可按源调）
query_template: |                      # 审批入库的参数化模板，运行时仅绑定水位与上限参数
  SELECT id, host_ip, cve_id, severity, port, title, status, last_seen, scan_id
  FROM findings
  WHERE last_seen > :watermark
  ORDER BY last_seen
  LIMIT :max_rows
watermark: { column: last_seen, type: timestamp }
identity: { primary: external_id, fallback: [asset_ref, cve_id, port] }   # D19 稳定键优先级（external_id 漂移的源把 fallback 放前）
observed_at: last_seen                 # 源观测时间列，用于 D19 时间判优；缺失时退化到 scan_id 变化判据
limits: { max_rows: 5000, timeout_sec: 60 }
adapter: scanner-a-findings            # 归一化适配器名
exit_condition: "对方提供 findings 查询 API 后切换为 http-pull"   # 09 号补强：db-pull 为兜底，须登记退出条件
```

### 4.11 AI 智能体平台对接（终态，双向——所有者 2026-10-20 补充，D17）

| 方向 | 形态 | 预留 |
|---|---|---|
| **出：平台调用智能体能力作为输入 / 任务** | 智能体 = 一种能力来源：连接器 `kind=agent`（或 §4.9 声明式连接器的 agent 型操作），注册为 `actions(kind=connector)`；长任务按 taskrunner 语义异步（提交 → 轮询 / 回调 → 结果事件），凭据 / 限流 / 熔断同 §4.9 | A1 注册表天然容纳；SDK `zh.agents.run(agent, input)` 可作脚本节点的一步 |
| **入：智能体产出回流** | 分析结论 / 事件 / 处置建议作为**事件源**走统一接入层（§3.1 上报 API 或 webhook 入口站），同一归一化流水线、同指纹去重 | A7 入口站 + 源适配器，零新通道 |
| **反向：智能体调用平台能力** | Action 暴露（MCP server，D10）；身份 = `principal=agent:<name>` + `on_behalf_of` 发起人（A6），权限取交集；Responder 类动作默认需人工确认（D8） | A1 + A6 已覆盖；审计三元组必带 |

门控：智能体平台的供给与形态均未明，与 LLM 供给同门控（phase11 智能面或独立触发驱动段）；**接口面无需新建**——A1（能力来源 / Action 暴露）与 A7（事件来源）两面已把它当作普通的"连接器 + 源"容纳，届时只加适配器。

## 5. 版本与演进规则

- Action / 连接器 / 脚本包：semver；`actions(action_id, version)` 复合主键；调用方可钉 `1.x`；同 major 内 schema 只允许**增量**（新增可选字段）；破坏性变更升 major 并发 deprecated 事件，90 天日落。
- 事件类型：`type@v1` 后缀；订阅者声明可消费版本。
- REST：维持 `/v1`；新增端点不升版；字段增量兼容。
- 兼容层：`aksk/compat` 存量调用方迁完整包删除（既定），webhook 入口站不复用该层。

## 6. 契约守护（随实施收编）

- **actions ↔ 权限码对账**：每个 action 的 `permission_code` 必须存在于权限词表（延伸 BK-22 路由↔menu_apis 对账的 fail-fast 模式）。
- **schema 快照测试**：每个 action 的 input/output schema 入测试夹具，变更即测试红（防静默破坏编排与 AI 工具）。
- **连接器健康对账**：启动期 health 探测失败只告警不拒启（外部依赖不应阻断平台），运行期熔断。
- **入口站重放测试**：原始落盘表 + normalizer 的回归用例以真实 payload 脱敏样本为夹具。
