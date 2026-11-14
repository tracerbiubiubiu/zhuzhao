# Phase6 私有适配、设备与 AI 薄入口详细设计

日期：2026-11-14。状态：**详细设计候选，未实现**。对应 X01-C/B、X02-Q/F、X03、A01/A02；本地核心只取实际需要的稳定接缝，设备和 AI 不反向阻塞首版风险/工单/TR/AL/消息。完整 X01-H、UAA-P/R、持续委托、MCP 产品化及领域 Agent 保留原后续窗口。本页细化[扩展兼容](../design/extension-compatibility.md)、[内网适配](../design/intranet-adapter-migration.md)、[设备](../design/device-module-contract.md)、[AI 调用](../design/ai-invocation-contract.md)，不另立范围/报价总账。

## 1. 真实基础与边界

`internal/pkg/datafetcher/datafetcher.go` 的 Registry/ApplyFetches 是进程内注册/单触发取数，不能由私有仓导入核心 internal 来当稳定 SDK。`internal/pkg/threatintel/` 已有 Provider/HTTP 桩形态，但不是通用目录/feed/扩展宿主。当前没有 X03 或模型执行产品代码；既有 Go service 方法和 HTTP 目录是可复用效果入口，须逐条接统一应用用例及三层授权。

目标拓扑：核心公共 API/领域端口 ↔ 进程外私有 connector/应用 ↔ 公司已有 HR/CMDB/风险源/消息/模型等供给。厂商字段、SDK、网络地址和秘密留私有侧；核心拥有规范化领域语义。**不导入核心 internal、不跨库写、不共享表或跨库 FK**。升级前后公共 wire/语义与旧参考 adapter 通过同套契约；同主版本兼容，不承诺改任何业务语义仍永久零修改。参考 [Google AIP-180](https://google.aip.dev/180)核字段、默认值、错误、排序、完成和权限语义，而非只核 JSON 能解码。

## 2. X01-B：配置式接入、身份与请求路径

首版配置由受审文件/部署提供，修改后显式校验并重启/切版本，不做插件市场、动态任意代码或通用微前端。

```text
ExtensionConfigV1 = {extension_id: NamespacedKey, version: string,
  contract_major: "1", required_capabilities: [{id: string, version: "1"}],
  endpoint_ref: string, secret_ref: string, data_zone: string, enabled: bool,
  resources: [{code: NamespacedKey, kind: page|button, owned_by: string}],
  routes: [{method: GET|POST, path_template: string, capability_id: string,
    max_body_bytes: int, timeout_ms: int}],
  navigation: {title: string, icon: AllowedIcon, trusted_app_ref: string}}
ExtensionRequest = {audience: extension_id, request_id: string, request_ref: OpaqueRef,
  capability_id: string, resource_ref: ResourceRef, payload: TypedInput}
AuthorizeExtension = {audience: extension_id, capability_id: string,
  request_ref: OpaqueRef, resource_ref: ResourceRef, payload_hash: SHA256,
  requested_fields: string[]}
ExtensionAdmission = {principal_id: ID, audience: extension_id,
  capability_id: string, resource_ref: ResourceRef, payload_hash: SHA256,
  allowed_fields: string[], scope_constraints: {kind: none|org_set|all_approved,
    org_ids: ID[], policy_revision: UInt64String},
  operation_id: string?, valid_until: Timestamp}
```

配置不含真实用户/口令，资源 code 有扩展命名空间；依赖/重复路由/目录冲突或 B 案页面写绑定不合法时拒绝该候选启用，保留上一份完整有效配置，不部分暴露。既有核心迁移只改 core 来源行；停用扩展只撤它自己的目录/绑定，保留执行/审计与本地数据，不能按 URL 前缀粗删核心授权。

调用服务先验 HMAC/受信服务身份，再验证当前真实用户会话与 audience/路由；服务凭据不代表用户授权。首个同步用户调用采用**核心逐请求授权回调**：扩展收到经可信网关构造的受限请求引用，在执行前向核心验证该引用与当前主体/资源/字段；引用短期、一次请求绑定、不可在另一路由/对象复用。核心只签发已登记 capability/audience，不把用户 AT 或管理员密钥交给扩展/浏览器。准确签名字段复用既有 HMAC/未来 admission 合同，不能另编浏览器自报 user_id。

受限 request_ref 是核心生成的高熵 opaque 引用，服务端只存 hash，候选记录绑定当前主体/会话、audience、规范化方法与路由、capability/version、resource、payload_hash、允许字段、expires_at 和适用的固定 operation_id。**它只用于私有应用的代用户请求**；纯取数connector由核心先完成本地授权，再经受限服务端口查询，不创建用户交换ref。只读ref先复用现有Redis短TTL存储（候选30秒），不新增PG历史表/outbox/永久receipt；丢key/重启后旧ref拒绝，用户重发新读取即可。写效果的operation/receipt仍由领域持久化，不能因ref过期换号重做。

只有登记的目标扩展服务能经HMAC回调核验；缺失/过期/错服务/错目标/异参拒绝。回调每次重验当前主体、L1及登记在核心的组织/字段范围，返回上列Admission；核心拥有的资源仍完整核其L2/L3，**私有设备等资源的实际行属性只由私有属主核L2/L3**，不假称核心知道私有库内容。读取请求可在同一短期限重取当前许可；写请求重投同ref保留同operation，不把ref被读取等同业务已执行。现有HMAC只有时间窗且URL query未入签，不声称已有nonce防重；鉴权参数全部放签名body，短时ref及其绑定由服务端核验，不能改信X-Operator或普通query。首版不另造JWT/授权服务器，也不授予新的后台写。

公共核心做 L1，私有资源所有者仍做自己的行/字段和动作 L2/L3，不能只依菜单可见。首版不缓存正向用户授权以换可用性；回调失联拒绝受保护操作、显示依赖不可用。该逐请求接缝不授予后台持续写；无人值守必须等 I02。借鉴 [Backstage 服务身份与代用户身份分离](https://backstage.io/docs/auth/service-to-service-auth/)及[资源属主执行条件](https://backstage.io/docs/permissions/concepts/)，不照搬其插件运行框架。

首个私有设备 profile 选择有限组织集合：核心受审策略给出当前 org_set（最多256个本地 org ID，空集无权；超限显式拒绝/重新定 profile，不能放宽为全量），私有服务核设备权威 owner_org_ref 属于该集合及请求字段在交集内。all_approved 必须有明确独立授权，缺组织/未知映射默认不可见；当前动作再验入口配置和职责。私有源行属性失去可证明新鲜度时拒绝敏感入口，不由旧组织缓存扩大范围。scope_constraints 与 request_ref/当前 policy_revision 绑定，每请求获取；它不免除私有服务对象级校验。

HTTP、UI 网关、内部用例调用和后续 MCP 都使用静态 capability_id/contract_version 映射同一应用用例，operation_id 标一次动态效果。外部 REST 可保留业务端点，不要求一切变成 `/invoke`，内部也不必 localhost HTTP。代理只匹配登记方法/规范化模板，剔除客户端伪造敏感头，拒绝双重编码穿越、重复鉴权头、未知上游和超大 body。网络/重定向/目的地由配置约束；不将这个网关变成通用出站代理。

## 3. X02-Q：先可替换查询，再有状态 feed

```text
QueryRequestV1 = {query_id: RegisteredKey, contract_version: "1",
  source_id: string, parameters: QuerySpecificInput,
  cursor: string?, limit: int, requested_fields: string[]}
QueryPageV1<T> = {items: T[], next_cursor: string?, complete: bool,
  source_id: string, observed_at: Timestamp, fetched_at: Timestamp,
  source_revision: string?, mapping_version: string, warnings: Reason[]}
QueryFailure = {reason: invalid_input|not_authorized|source_unavailable|
  source_stale|unknown_enum|contract_mismatch|quota, retry_after_ms: int?}
```

query_id 的每个版本定义精确 parameters/items schema、允许排序/过滤/字段、时区和分页；不接任意 SQL/URL/厂商 JSON。limit 在服务端硬限且有请求预算，cursor 绑定 query/参数/源/当前 scope，不因知道 cursor 能越权；授权变化重算或拒绝旧 cursor。无匹配为 items=[]/complete=true，超时/权限不足不是空成功，partial 明示 complete=false；跨页不稳定源只声称本次页观察，不伪造完整快照。

mock 与私有 HTTP connector 实现同一 QueryPort，环境只换 endpoint/secret/profile；真实源断开不得静默改用 mock。connector 将厂商字典映射为公共枚举并保留映射版本/原值受权引用；未知状态显式 unknown 或 contract_mismatch，不能默认 healthy/active。多参数预填支持用户/部门/时间等注册输入；传递 request context/取消，外部取数在业务事务外，结果仍须服务端字段校验，不能用取数结果写授权锚点。

X02-F 仅在实际启用目录/发现同步时增加 [D06 envelope](foundation-iam-module-detail.md#5-d06-与-uaa-c来源目录和组织生命周期)或[风险接入 DTO](risk-module-detail.md)。一个数据集一个采集/重试主；默认直通核心事务提交接收记录+领域事实后返回 applied ACK，才推进 committed checkpoint。若选择异步收件，必须先持久保存完整可重放 payload，返回 staged/accepted receipt；它只证明可靠接收，接收侧拥有继续重试责任，独立 received checkpoint 可以前进，**领域 committed checkpoint 仍等 applied receipt**。AL push 的 durable inbox ACK 与此同为“可恢复收件”，不等于后继任务/领域已完成。真实上游仅允许一个 checkpoint 时，先在契约明确其代表已可靠接收还是领域提交，不混名。需提前确认上游时先持久存可重投批次，不能 checkpoint 领先 payload。全量缺失处理须完整性证明；查询 HTTP200、SCIM 分页计数和本地 received_at 都不能证明删除/最新。只取数与回传分开授权，默认关闭回写。

### 3.1 配置、数据和升级边界

公共最小 SourceRegistration 逻辑实体含 source_id（稳定主键）、contract_major、data_zone、capabilities、mapping_version、authoritative_fields、ordering_profile、enabled；endpoint/secret 只保存引用。替换同一源的 adapter 版本不改变 source_id/entity identity，不重建风险周期或用户。来源 scope 变更不能凭“不再读到”删除旧记录。

query 不造 inbox/checkpoint 表；feed 才按 `(source_id,entity_kind,external_id,generation,source_revision)` 收件去重和来源水位存储，完整记录与领域提交同事务；同版本异 hash conflict，无法排序 hold。私有 connector 库自行迁移；公共 core 的 down 不负责删除其库。升级合同基线冻结旧参考 adapter 的可执行制品/fixture/hash，新核心对旧客户端实际测试；首次正式消费者登记后，破坏性变化新主版本/显式转换，不再引用“全是测试环境”清正式数据。

## 4. X03：设备目录、状态与人工登录

设备私有应用拥有自己的本地补充与查询投影，CMDB/监控/SOC 保持权威。第一版一个合成 source、不自建扫描探针/NMS、不保存设备密码、不做无 SSO 设备自动登录。

```text
DeviceView = {device_id: ID, source_ref: ExternalRef, name: string,
  vendor: string?, model: string?, owner_org_ref: ID?, category: string,
  status: healthy|degraded|down|unknown, observed_at: Timestamp?, stale: bool,
  can_open: bool, entry_version: UInt64String?, login_username_hint: string?}
DeviceSummary = {healthy: int, degraded: int, down: int, unknown: int,
  stale: int, total: int, as_of: Timestamp, scope_revision: UInt64String}
OpenDeviceEntry = {device_id: ID, expected_entry_version: UInt64String}
LaunchResult = {launch_id: string, mode: manual_web,
  destination: ApprovedURL, target_network_hint: string,
  outcome: launch_requested, observed_at: Timestamp}
```

summary 的 healthy/degraded/down/unknown 互斥且合计 total；stale 为独立交叉计数，不能再与四状态相加。大屏只用专门受权只读主体/有限范围，隐藏入口/IP 等敏感字段，计数和明细用同一资源过滤器；无源/陈旧/部分失败不归零，显示时间与状态原因。状态字段源观测时间与获取时间分开，不由“设备管理页返回200”推定健康。

L1 分设备查看、入口打开、配置；L2 以设备组织/批准责任范围过滤；L3 再验当前 entry enabled/version/允许目标。`OpenDeviceEntry` 是候选 POST 用例，不把设备 ID 放 POST URL；当前不可见404、可见无打开权403、入口已换409。服务端只从私有受审配置解析 destination，禁止请求自带 URL、脚本协议、userinfo、凭据 query 和开放 redirect。普通直链浏览器打开，无核心抓取厂商页面；noopener/noreferrer，清楚提示到设备侧手动认证。launch 只证明请求入口，不记“登录成功”；设备真正授权仍由目标侧保证。

确有需要时仅在获专门字段授权的详情中展示 `login_username_hint`（人工登录的非秘密账号提示）；列表、大屏、通知及普通日志不返回该字段。它不携带密码/token，不提供代填/代理登录；同用户可见设备A不代表可见设备B账号。源未提供或未获权则字段缺失，不由默认管理员名猜测。

逻辑约束：device_refs UNIQUE(source_id,external_id,generation)，status 按 source_revision/observed_at 质量有界更新，access_entries UNIQUE(device_id,entry_key) 并 revision CAS；launch_records 带 actor/设备/entry revision/结果及审计引用。列表不返回无权入口私有地址；launch_result 响应禁止普通日志记录 destination 全值。入口配置变更不覆盖 CMDB 权威属性；停用只关新打开，既有目标侧会话无法由本平台假称已终止。

## 5. A01：固定场景受权自然语言查询

只在 A01 自身就绪时交付，不把 AI 模型可用当风险人工处理前置。模型仅已有内部/获准 API，无训练/部署。首选固定 `ticket.list/detail/aggregate` 与已就绪风险 Query；从目录把允许 schema/参数给模型，模型只产生提议，服务端确定性校验后走同一 Query/PDP。不可见字段不进入 prompt，返回给用户前再次投影；模型、源文档、工具返回均不授信权限。

```text
AIQueryRequest = {scenario: "authorized-work-items.v1", text: string,
  connection_alias: string, timezone: string, conversation_ref: string?}
QueryProposal = {query_id: RegisteredKey, parameters: TypedQueryInput,
  needs_clarification: string[]}
AIQueryResponse = {invocation_id: string,
  state: needs_input|succeeded|failed|canceled,
  answer: string?, sources: [{resource_ref: ResourceRef, revision: string?,
    observed_at: Timestamp}], complete: bool, reason: string?}
```

模型 connection 保存 alias/version、provider_kind、endpoint_ref/secret_ref、data_zone、能力集、超时/并发/输入输出预算。不能仅凭“OpenAI-compatible”假定 tools/JSON schema/usage 可用；adapter contract 证明，不支持结构化建议的连接只提供可验证摘要或明确拒绝该场景。内网不自动 fallback 外网；fallback 只限同授权数据区和所需能力并记录实际使用连接/模型标识，未知修订写 unknown。

每个 invocation 有 actor/config revision/input hash/state/reason/usage、创建与终止时间；prompt/敏感原文不进普通日志。查询页异步请求按 invocation 或前端递增 token 绑定，切账号/换问题清理旧结果；取消只停止后续读取/生成，已取得受限数据不向已撤权用户继续流式发送。来源超时显示 unavailable；有20行结果但 complete=false 不让模型回答全公司总量。缺少部门/窗口/类型时先 needs_input，不自行选择高权限目标。

## 6. A02：自然语言草稿、精确预览与确认

```text
TicketDraft = {draft_id: string, draft_revision: UInt64String,
  type_code: string, definition_version: UInt64String,
  definition_hash: SHA256, target_org_id: ID,
  fields: TypedTicketFields, custom_data: object, missing_fields: string[]}
PreviewRequest = {draft_id: string, expected_draft_revision: UInt64String}
ConfirmRequest = {plan_id: string, plan_hash: SHA256, idempotency_key: string}
Plan = {actor_id: ID, operation_id: string, canonical_command: TicketCreateCommand,
  definition_hash: SHA256, reference_revisions: object,
  effect_summary: string[], expires_at: Timestamp, plan_hash: SHA256}
```

模型只能写草稿。确定性 preview 核当前类型/发布定义、字段、可创建范围/目标、后继效果并保存 Plan；前端展示真实命令及责任/通知，不只一段 AI 解释。确认由当前真人提交；同 actor、计划、hash、时效和当前授权/引用重新复验，调用正常 Ticket.Create 应用用例。改任一目标/参数/定义/后继须重新预览；不能模型“自动纠正”继续执行。与风险受限派单是不同入口，不让自然语言创建绕过其职责限制。

逻辑 ai_plans 主键 plan_id，绑定 actor/draft revision/command hash/expiry，**preview 即分配固定 operation_id，UNIQUE(plan_id) 的确认关联保证一计划仅一次业务效果**。confirm 锁 plan 并 CAS pending→executing，确认关联与工单、领域事实、可靠后继同一本地事务提交；operation 与 `(caller_namespace,idempotency_key)` 都有唯一约束。同 plan 即使用两个不同 key 并发，也只能关联同一 operation/原 receipt；同 key 异命令409，不靠前端复用 key 保证安全。事务失败 plan 保持 pending，提交成功后新 key 不重新建单。确认响应丢失查原 receipt，不再次模型生成/换计划；plan 过期/定义变化409并要求重新预览；不能用过去成功 receipt 绕过当前读取权限。HTTP 超时但提交结果未明显示 outcome_unknown/待核实，不能当失败重建单。只读 A01、草稿 A02 与实际工单 effects 分开记录身份。

后台持续任务、无人值守工单写、跨服务代用户及 MCP 写等 I02/I03/A04；本页不发长期 grant。RAG 复用已有知识服务且逐主体 ACL；skill 保持受信版本指令/资产，allowed-tools 不授予权限；领域 Agent 的有限 host 按 P8 既定包展开。MCP 以后仅是同用例的薄协议适配，逐请求工具可见性、输出投影与当前授权不变。参考 [MCP tools](https://modelcontextprotocol.io/specification/2026-07-28/server/tools)的工具发现/调用及不可信 annotations 边界；**不在本次详设中升级既有已锁 MCP profile/SDK**，具体协议版本仍由 A03 契约和互操作结果决定。

## 7. 异常、容量与首批配置

查询/模型调用都在事务外，使用 context 取消、总 deadline、body/页数/输出上限；每源/连接有有界并发、超限429/Retry-After、短路/退避；用户不能耗尽共享连接池。首版配置式限额不建设自动弹性平台，数值在首个合同/负载测试后记录。429 遵提供方有界等待，鉴权/坏 schema 不盲重试，写效果 unknown 只对账。网络 endpoint 来自 allowlist 配置，不允许模型提供 URL 发起请求。

单实例先交；新增 replica 前验证共享配置 revision、PDP 检查、幂等 receipt/唯一约束和有主恢复，不靠单进程 map/按钮 disabled 保证唯一。connector、核心、模型各自仅重试自己拥有的那一跳；模型没有业务重试权。停用 connection/extension 立即阻止新调用、在途只允许受权查询原 receipt，不删除已产生业务效果。

## 8. 测试先行与可交付切片

| 用例 ID | 场景 | 必须证明 |
| --- | --- | --- |
| EXT-01 | 同 QueryPort 跑 mock/参考 HTTP，换 endpoint 后原客户端 | 输入输出/错误/时效和权限语义相同，core 零厂商差异 |
| EXT-02 | 服务 AK 正确但伪 user、错 audience、旧/重放请求引用、同用户跨两部门设备 | 拒绝或只返回获权组织/字段；私有属主核行属性，无管理员等价身份 |
| EXT-03 | 深链/批量/计数/导出、撤权后重用 cursor | 相同 L2/字段投影，撤权不从旧缓存继续读 |
| EXT-04 | 旧 adapter × 新 core，字段增量/未知枚举/语义破坏 | 可兼容增量通过；坏变更阻断候选并要求新版本/转换 |
| EXT-05 | 私有目录重复/冲突/停用中有在途调用 | 无部分启用；只撤本人来源，不删除业务历史 |
| DEV-01 | 无状态源/陈旧/部分源失败/用户只看一部门 | unknown/stale 明确，分母与可见明细一致、stale 不重复相加 |
| DEV-02 | 看状态无打开权、入口换版、恶意/未登记 URL | 服务端403/409或配置拒绝，不抓页面、不泄设备凭据 |
| DEV-03 | 多厂商且全无 SSO | 统一目录后人工登录；launch 不记 login success |
| AI-01 | 同义/缺条件/恶意提示/非法 tool/跨组织问总量 | 先澄清或拒绝，模型不能 SQL/越权、partial 不装总体 |
| AI-02 | 模型超时/不支持结构化/配额/内网外网 fallback | 明确 failed/needs_input，无空成功、无外泄降级 |
| AI-03 | preview 后改定义/目标/撤权/过期；同 plan 同 key/不同 key 并发 confirm | 旧 plan 拒绝；一 plan 固定一 operation 最多一票，同号异参409 |
| AI-04 | 建单响应丢失/恢复、切账号后旧 stream 到达 | 查原 receipt，不重建单、不向新账号展示旧答案 |

实施按 X01-C/B＋X02-Q 的一个参考应用先验，再实际 D06/发现 feed，X03/A01/A02各按自身窗口；每批保留薄 UI、可见失败和运维配置。DTO/逻辑 DDL 在这里给出准确候选，真正 OpenAPI/SDK/迁移与跨仓生产者契约需实现批同步；本页用例不是已执行测试。A01 语义黄金集/延迟阈值在实际模型选定后基线化，不承诺100%准确；安全权限反例必须全过。

## 9. 尚需确认与变化

无需再问是否保留 mock、本地 IAM、私有隔离、设备人工登录或模型只 API。具体公司 source/mapping、允许网络、真实模型能力/配额、设备可见组织/入口维护者在各自真实启用前确认；无源显示 unknown，不让缺厂商资料阻塞公共接口详设。涉及扩大实际业务范围/写权限/数据出区时给具体用户场景再确认，不把技术默认值变成业务批准。

- 2026-11-14：补 X01 基础同步身份接缝、X02 查询与 feed 区分、设备 DTO/计数/入口控制、A01/A02准确候选和异常/TDD。核心净估、远期 AI/设备实施波次不变，复用机制不重复计价。
- 2026-11-15：用户身份交换只限私有应用，源query不套此协议；短时只读ref不建永久表。保留在线当前授权，不用不完整HMAC头替代权限。
