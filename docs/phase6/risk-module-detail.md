# Phase6 风险模块详细设计

日期：2026-11-14。状态：**可评审设计候选，未实现、未跑新增产品用例**。对应 D07–D10、D13–D15 及 E10 风险子范围；正常/异常语义、typed DTO 和逻辑表约束在此细化，业务已决项沿[生命周期合同](../design/business-lifecycle-contracts.md)§8–10。原 R1–R4 的历史量级、整包前置及旧草图不作为新承诺；排期/预算只由[总账](../planning/planning-backlog.json)和[本地首版净估](../planning/local-core-release-estimate.json)维护，不重复计价。

## 1. 实现基线、边界和阶梯

本次定向核查 `internal/model/`、`internal/handler/`、`internal/service/`、路由和迁移目录，风险主实体/服务/路由仍未实现。可复用的是 `internal/service/ticket/service.go` 的 `Create`/`ExportStream`、`internal/service/ticket/resource.go` 的 `Authorize`/`GetFilter`、通用标签/人读编号、通知和任务基础设施；它们不证明风险接口、风险三层权限或跨域事务已经存在。工单现有 `Create` 从 actor 派生创建人，风险桥不能简单换 actor 冒用 owner，也不能直接写工单表。

- **本地首版。** 有权限的工作人员可以手工登记风险，也可以从一种已确认格式的来源接收风险。在列表或详情中按四档定级，指定责任人和截止时间。满足同组条件时，最多把 20 条风险派到一张整改工单，办理人和复核人仍需按每条风险分别交证据、记录结论。接受风险要经申请和另一位有资格的人确认。首版还处理第一条风险通知及发送失败，用固定流程与基本字段即可办理。与工单、任务、清单和消息服务沿各自的业务接口联动。本地三层权限依次检查能否使用功能（L1）、能否看到这条记录（L2）、能否执行本次动作（L3），无需等待企业 UAA。
- **同阶段后继**：D13 固定指标/每日快照/有界风险时间线；D14 SOC引用与本地岗位协作；E10调查后批准的Excel迁移或跨指纹合并。上述不冒充本地首版硬前置。
- **后置能力**：第二类来源、通用连接器、评分模型、工作日日历、通用BI/DSL、专业研判/扫描/隔离封禁不在首版顺手实现。既定业务效果仍由原包承接，不以缩小首版取消。

扫描器或情报服务上报的一条风险发现叫“源观察”。系统按固定字段接收并匹配原来的记录，保留本地处置历史。来源重新上报、变更或不再列出该风险时，外部结果可以作为核实的依据，已关闭的本地风险不会因此自动重开，在办的风险也不会因此自动关闭。 [重导入的参考设计](https://docs.defectdojo.com/import_data/import_intro/reimport/)

## 2. 写主、模块端口和公共类型

各模块分别负责写入自己的业务事实。风险域维护同一指纹的风险族（family）、每次发生的风险周期（cycle）、来源观察，以及本地评估、接受申请、责任人和逐项结论。工单域维护工单及办理记录；风险工单桥协调关联表 links 和由桥维护的字段；通知域维护发送记录。来源适配器（adapter）只转换并传递规范化观察，不导入 `internal`，也不写本地状态、角色或工单库。asset_ref 取自外部 CMDB 或明确的临时映射，不另建一套资产权威数据。

HTTP、人工、受限风险派单、任务和未来 MCP 均适配到相同业务用例。可信 actor/service/grant 由[共同调用合同](../design/capability-invocation-contract.md)取得，不接受 body 中的 `actor_id/skip_auth/created_by`。HTTP 响应沿既有统一 envelope；下列为其中 data 的**目标类型**，不是生成代码或现有 API。

```text
ID = decimal string                     # JSON/SDK不用JS number承载int64
Revision = UInt64String                 # 十进制字符串，公共wire不使用JS number
Instant = RFC3339 with timezone          # 存储timestamptz，比较统一UTC
Level = urgent | high | medium | low     # priority映射1/2/3/4
RiskState = new | assessed | remediating | pending_verify | accepted | closed
Treatment = mitigate | accept | transfer | avoid
ClosureKind = fixed | false_positive | duplicate | transfer | avoid
RiskRef = {id: ID, human_key: string, revision: Revision}
EvidenceRef = {provider: string, external_id: string, version?: string}
Mutation = {operation_id: string, expected_revision: Revision}
RiskView = {risk: RiskRef, status: RiskState, closure_kind: ClosureKind|null,
  level: Level|null, owner_user_id: ID|null, org_id: ID, affected_org_id: ID|null,
  deadline: Instant|null, init_deadline: Instant|null, treatment: Treatment|null,
  source_severity: {scheme: string, value: string}|null,
  acceptance: AcceptanceSummary|null, links: AuthorizedLink[],
  capabilities: string[], pending_effects: string[]}
```

新公共revision使用十进制字符串；落PostgreSQL BIGINT时范围为1..9223372036854775807，服务端增量溢出拒绝且无效果，不绕回0。旧已有合同/快照若使用数字版本，仅在0..9007199254740991且原版本合同允许时无损读取并显式转换；超过安全整数、非整数或字符串/数字混用不可猜测，给兼容错误。新增风险无存量API，不为它创造另一套数字wire。

无 level 的新风险先显示“未评估”，不假定低风险。字段缺省/显式 null/空串逐类型校验：nullable列可清空，必填引用不收空串；PATCH式修改用 `changes` 中显式出现的字段，未出现不变。同一个 `operation_id` 对应同一次业务效果；批量派单的父编号标识整批，各分组另有派生编号。`risk.assess.v1` 等名称只标识能力定义。重试同一次操作、相同输入时继续使用原 operation_id；同号但参数不同返回 409。共同调用层负责把它与既有 `Idempotency-Key` 对应起来，风险域不另建幂等框架。

### 2.1 查询、录入和来源接入

候选 HTTP 路由均在 `/api/v1` 下，仅 GET/POST；写操作的业务标识在 body。新增按钮/页面绑定遵 B 案，不能把写端点挂风险列表页。

- `GET /risk-items`：`page>=1,page_size=1..100,status?,level?,owner_user_id?,affected_org_id?,source_ref?,system_ref?,q?,sort=created_at|deadline|level`。固定字段排序加 ID 稳定次序；先授权/字段裁剪再筛选/计数，返回 `{items:RiskView[],total:int,page,page_size}`。首版沿现有分页，不承诺跨写入稳定快照；批导出另绑定查询水位。
- `GET /risk-items/:id`：返回 RiskView 和有界当前来源摘要；findings、history、links 各分页，禁止默认把全部原始JSON/附件/关联票正文拼入详情。不可见与不存在均404；可见但动作不允许403。
- `POST /risk-items/create`：`{operation_id,title,description,org_id,affected_org_id?,owner_user_id?,system_ref?,finding_key?,asset_ref?,evidence_refs:EvidenceRef[],attributes:object}`；返回 `{risk:RiskRef,status:new,receipt_ref}`。手工登记的漏洞标识和资产引用须成对提交。存量无身份登记仍可评估和确认责任。手工源命名空间由服务端建立，禁止传外部source_ref伪装扫描结论；title/description/扩展字段沿统一大小上限与schema白名单。
- `POST /risk-items/report`：body为[FindingObservationV1](../design/business-lifecycle-contracts.md#81-规范化来源合同与唯一写主)或有界批次 `{batch_id,observations:[...]}`；批次建议上限100条/1MiB、来源峰值20QPS沿既定初值，配置可收紧。`source_sequence`保留字符串，由该源已验证比较器比较，不能按任意字符串字典序或无界JS整数比较。
- report返回 `{batch_receipt,items:[{event_id,ingest_id,result:accepted|duplicate|stale|quarantined,processing:pending|applied|held,risk_ref?:RiskRef,safe_reason?}]}`。`accepted`仅说明已持久收件；不可解释为已评估/已建风险/已通知。查询 `GET /risk-items/ingest-receipts/:id` 重验源范围或人工权限，不返回越权risk_ref。

来源可用 `valid_until` 声明观察期限（须晚于 `observed_at`）。派单时复验当前启用来源、映射、漏洞及资产身份、撤回状态和声明期限。未声明期限的现有观察沿用原镜像生命周期。

批次整体格式、签名/来源、大小不合法时整批400/401/403/413且零接入效果；合法批次中的字段/稳定键冲突逐项隔离并返回结果。外部未知字段默认不落强schema，仅允许经映射白名单的 attributes。HMAC只证明来源服务，不能授权它上传本地 owner/accepted/closed 等字段。

### 2.2 评估、责任、接受与结论

- `POST /risk-items/assess`：`Mutation+{id,level,assessment_note,treatment,deadline?,dispatch_basis_valid_until?}` → `{risk:RiskRef,status,level,deadline,receipt_ref}`；允许new/assessed，关键评估改变时使旧pending接受申请 superseded。deadline为空时按级别policy自然日带出；保存首次init_deadline和policy version，后续改期要求reason。人工派单需要最近一次评估的非空核实说明。可另填核实依据有效期，系统在锁内按该期限复核。以空说明重新评估可撤掉旧评估的派单资格。
- `POST /risk-items/update`：`Mutation+{id,changes:{description?,deadline?:Instant|null,attributes?},reason}` → `{risk:RiskRef,changed_fields,receipt_ref}`。仅当前获权字段，deadline只在非终态整改链修改，初次值保留init_deadline；普通update禁止改owner/level/接受/受管来源或closed历史，分别走专用命令。属性为逐字段白名单patch，不整包替换未知元数据。
- `POST /risk-items/assign`：`Mutation+{id,owner_user_id,reason,link_expected_revisions?}` → `{risk:RiskRef,handover_ref,links:[{link_id,result,revision}],pending_effects}`。主锚管理者可改责任；owner不能仅凭拥有本风险就把责任移给任意人。
- `POST /risk-items/accept-request`：`Mutation+{id,reason,valid_until:Instant}` → `{acceptance_id,acceptance_revision,status:pending,risk_revision}`。仅当前owner本人且具manage，期限必须未来；状态不立即变accepted。
- `POST /risk-items/accept-confirm`：`{operation_id,acceptance_id,expected_acceptance_revision,expected_risk_revision,decision:approve|reject,reason}` → `{acceptance_id,acceptance_revision,status,risk:RiskRef,accepted_until:Instant|null}`。当前主锚管理者、未自确认、申请基线仍有效才批准；拒绝必填理由，不能拒绝已批准申请伪造回滚。
- `POST /risk-items/close`：`Mutation+{id,resolution:fixed|false_positive|duplicate|canceled,evidence_refs,note,master_risk_id?}`。固定结论要求整改/复测人、复测时间和证据引用；false_positive/duplicate不借此绕过主锚资格或E10合并启用门。`canceled`回assessed，不是风险已修复或任务效果已取消；transfer/avoid首版只登记策略和依据，不能因改字段自动关闭。新增候选wire统一美式`canceled`，没有已发布的风险API需兼容旧拼写。

`POST /risk-items/false-positive`另收`Mutation+{id,reason,suppression_valid_until:Instant}`，主锚管理者才可写误报关闭和有界抑制；`close`中的false_positive只在具备同一完整参数/资格的适配器内委派该用例，缺抑制参数不得默用永久期。`GET /risk-suppressions`分页查主锚范围；`POST /risk-suppressions/remove`收`{operation_id,id,expected_revision,reason}`终止抑制但不自动重开历史误报cycle。接受/误报有效期独立于整改deadline，UI改期不改接受或抑制期限。

错误映射：400字段/枚举/无owner/不满足配置；401身份无效；403当前资格或可见对象动作不允许；404不可见/不存在；409 revision、同号异参、已存在active link或业务状态竞争；413上限；429来源限流；503持久层/授权依赖不可用且未提交。必要字段校验沿统一错误码和field path，无内部SQL/密钥。**已提交后网络失败**不能返回“全回滚”；调用者查原 operation/receipt，投影/通知失败进入pending，不另造身份重做。

## 3. 逻辑数据模型、约束和迁移

以下是待编码逻辑DDL，不是可执行迁移，不占全局编号。ID沿既有主仓生成策略，外部ID保留string。所有写表有 `created_at/updated_at`（追加历史仅created_at），时间由服务端产生；source发生时间另存。

| 表/写主 | 最小列与约束 | 主要索引/存量处理 |
|---|---|---|
| `risk_families` 风险域 | `id bigint PK,fingerprint_scope text,canonical_key_hash text,key_schema_version int,current_cycle_id bigint null,revision bigint`；唯一 `(fingerprint_scope,canonical_key_hash,key_schema_version)` | first insert用ON CONFLICT取得同family，再锁行；current_cycle只能指该family，不用hash碰撞直接覆盖原键 |
| `risk_items` 风险域cycle | `id,human_key,family_id,generation int,regression_of null,status,closure_kind null,revision,org_id,affected_org_id null,owner_user_id null,level null,deadline/init_deadline null,treatment,accepted_until null,closed_at null,archived_at null,attributes jsonb`；唯一human_key、`(family_id,generation)`；status/level/closure CHECK | `(org_id,status,id)`、`(affected_org_id,status,id)`、`(owner_user_id,status,id)`、`(level,deadline,id)`、family；`archived_at`是查询标签，不另造业务终态 |
| `risk_source_records` 来源投影 | `(source_ref,source_item_key) PK,latest_sequence,observed_at,content_hash,mapping_ref,mirror jsonb,revision` | 源删除只变可用性；不级联删cycle/history |
| `risk_observations` 风险接入 | `ingest_id,source_ref,event_id,source_item_key,source_sequence,observed_at,content_hash,mapping_ref,kind,processing_status,risk_id null,raw_ref,received_at,safe_reason`；唯一`(source_ref,event_id)`；有序源对非空sequence唯一`(source_ref,source_item_key,source_sequence)` | `(processing_status,received_at,ingest_id)`供恢复；同唯一键异hash进入冲突隔离记录，不能覆写已验原文 |
| `risk_ingest_conflicts` 风险接入 | `id bigint PK,original_ingest_id,source_ref,event_id,source_item_key,source_sequence null,content_hash,raw_ref,conflict_kind,state,received_at,resolved_by/at/reason null`；唯一`(source_ref,event_id,content_hash)` | 只存冲突输入及原观察引用，原观察不可变；`(source_ref,state,received_at,id)`用于有界核查。复用已有持久接入存储能保持同语义时可合表，不建通用隔离平台 |
| `risk_events`/`risk_treatments` 风险域 | `id,risk_id,aggregate_revision,event_kind,actor_ref,occurred_at,operation_id,payload jsonb`；前者领域事实，后者用户处置记录；均追加、同事务 | `(risk_id,occurred_at,id)`；事件含`ordinal int>=0`，唯一`(risk_id,aggregate_revision,event_kind,ordinal)`，避免同revision合法多事件互撞 |
| `risk_acceptances` 风险域 | `id,risk_id,applicant_id,owner_at_request,risk_revision,reason,valid_until,status,revision,decided_by/at/reason null`；每risk最多一个pending申请 | partial unique `(risk_id) WHERE status='pending'`；有效接受版本保留，不原地抹历史 |
| `risk_suppressions` 风险域 | `id,family_id,valid_until,reason,created_by,ended_at null,revision` | 同family最多一条当前记录；启用/过期都取family锁，禁止源side自加抑制 |
| `risk_ticket_links` 领域桥 | `id,risk_id,ticket_id null,ticket_ref_snapshot,status,revision,owner_at_link,resolution/proposal/evidence,ended_at null` | unique`(risk_id) WHERE status='active'`，`(ticket_id,status,id)`；ticket物理删用SET NULL+不可变引用快照，禁止级联删结论 |
| `risk_dispatch_plans` 领域桥 | `id,actor_ref,input_hash,definition_ref,item_revision_set,target_mapping_version,expires_at,status,operation_id` | 复用共享plan/receipt存储时不重复造表；不可由客户端改plan内容；过期只读，不代表建票 |

raw正文保留180天后可清理；稳定幂等身份/hash、源checkpoint、周期回链、处置/接受/receipt不随raw删除。否则180天后同event重推会再加计数。已关闭365天转归档标签且仍受授权可查；不是删除。原始证据若受法律保留/来源分类限制，部署前核类别参数；参数未绑不能把默认值当公司合规承诺。

迁移分批：①基础表/nullable兼容列与双写读fallback；②回填并验证唯一性、FK/授权锚与来源身份；③再收紧约束/启用功能。风险目前无存量，但共享ticket必须按原数据回填/pin定义，不清空旧票。up/down成对；down前停新写、封存新增正式数据、证明最低兼容代码和在途处理，不能把down视为删除业务历史的正常回退。B案旧代码/DB回退耦合继续有效。

## 4. 接入处理、事务和并发

1. 接入入口（ingress）校验服务身份、来源范围、schema、请求大小和频率限制。在短事务中保存收到的内容、hash 和可恢复的处理状态，然后才返回 accepted。基础版可以在同一请求中继续处理，但“已收件”和“业务处理完成”必须分别记录证据。
2. 消费者（consumer）读取本次固定的 mapping 版本，查询目录、CMDB 或外部结论；网络调用放在锁外，并限制总耗时。无法确认稳定身份时置 held，不能因为 IP 或姓名改变就新建风险。改变来源映射前，先预览对旧键的影响，不能直接重算 hash 覆盖历史。
3. 需要本地授权的命令沿共同锁顺序执行：先经过 C04 授权守卫，再锁适用组织及固定的定义版本，随后按 group/family 稳定键、risk ID、ticket ID、link ID 的顺序锁对象。同类对象按升序锁定，不涉及的对象跳过，不另建锁顺序。首次创建 family 靠唯一约束冲突后重读，不能对不存在的行使用 FOR UPDATE 来防重。来源 consumer 仅能在本源系统范围内处理，不冒用真人身份；归并计数不能顺带派单。
4. 锁内复验mapping启用状态、来源顺序、family当前cycle和revision。新event同内容计一次；同event重试不加计数；旧序号stale保留收据不覆盖镜像；等序异内容quarantined；timestamp相等且无可比序号、无法证明新观察时held，不能按接收时间裁定回归。
5. 一事务提交cycle/镜像/观察应用状态、历史和下游intent；未知提交结果先查原receipt。consumer崩溃靠同一ingest_id恢复；不将“任务已受理”当完成。

修复关闭时，保存各来源最后有效观察的位置（checkpoint）。之后只有确认再次观察到风险，才在 family 锁内创建 generation+1，用 regression_of 回链，并更新当前周期指针（head）；两条新观察同时到达也最多创建一个新周期。接受有效期/误报抑制期内只增观察；接受到期用申请版本+cycle锁CAS重开原cycle，续期先提交则expiry no-op。误报抑制过期的新观察可建新待评估cycle，标suppression_expired，不计修复回归。withdrawn/deleted/全量缺失不作已修复。

首版保留单体短事务+数据库约束，不引分布式锁服务/跨库事务。[PostgreSQL约束](https://www.postgresql.org/docs/current/ddl-constraints.html)兜底唯一性，[显式锁](https://www.postgresql.org/docs/current/explicit-locking.html)支持行锁/事务级advisory锁；本项目固定锁顺序、ON CONFLICT后再读取，死锁/serialization失败可在同operation且无外部效果时有限重试，超限返回可重试冲突，不无限循环。多实例不得依赖进程mutex防重；worker claim/lease/fence复用R01。

**冲突持久落点（R1/D08）**：同event异hash或同源条目等sequence异hash时，原`risk_observations`和已应用镜像不更新；把新输入的受控raw_ref、hash、原ingest引用及冲突种类同事务保存到`risk_ingest_conflicts`，返回引用该记录的quarantined结果。相同冲突重投返回原冲突引用，不重复派单/建cycle。不能保存冲突证据时给依赖/容量失败，不以quarantined假装已收妥。受信源仍须通过原source授权、body预算与流控；raw只在受控存储保留，普通receipt/日志不回原文。获权维护者按当前source/组织范围查询和记录处置；需要采纳新的业务内容时提交具名新观察/受审修正，不覆写旧事件或原风险历史。原始材料清理不能先删掉仍被open冲突唯一引用的证据，超量背压保留待送，不另造自动裁决平台。

## 5. 风险→工单及完成闭环

<a id="漏洞按责任人派单"></a>

**漏洞整改按漏洞和资产当前责任人派单。** 同一漏洞影响某人名下多台资产时，该人只对应一张在办整改工单。每台资产保留一条受影响记录，分别提交证据和复测。同一漏洞影响另一人名下的资产，给另一人另开工单。同一个人名下发生不同漏洞，也各自开单。团队归属不参与这一判断。责任人来自已核实的资产归属，派单、办理和复测分别按当前资格检查。

首批人工漏洞整改（B2）用合成来源的稳定漏洞标识和已核实的资产责任人演练。漏洞标识可沿用[规范化输入的 finding_key 示例](../design/business-lifecycle-contracts.md#81-规范化来源合同与唯一写主)，来源条目和资产标识分别保留。预览、提交和追加都按漏洞与责任人查在办工单，提交时复核该组合及权限。团队不同且责任人相同的样例，用同一个获准目标验证可纳入同一张单。目标、权限或容量校验失败时，明确报告原因并保留原单，同一组合不另开在办单。

### 5.1 plan/apply和原子边界

`POST /risk-items/dispatch-plan`候选输入 `{items:[{risk_id,expected_revision}],target_type_code,target_org_id,append_ticket_id?,expected_ticket_revision?}`，风险最多20条为一个group；批向导可提交多group，首版候选总上限200条（服务端可收紧），超限要求分批。输出 `{plan_id,expires_at,groups:[{group_key,owner_user_id,target_definition_ref,items,blocking_reasons}],input_hash}`；仅对已可见风险给逐项原因。plan 是派单预览，不预留工单名额。预览在同一 C04 事务复核当前权限、读取风险和钉版定义、保存计划与投影；确认时按当时权限及版本再检查。开启整改时，定义必须提供分派、接单、提交复测及复测留在原态、打回和关闭的六条边。派单前验证已发布边及允许的角色和字段；待复测工单拒绝追加，打回整改后可按现势条件重试。

`POST /risk-items/dispatch`及`batch-dispatch`应用 `{operation_id,plan_id,plan_hash}`；如沿生命周期旧请求附items/type/definition等冗余字段，必须与plan完全一致，否则400，不接受覆盖。通用风险分组键至少含 owner、风险管理组织、目标组织、类型、定义及批准业务范围。漏洞整改按本节的[漏洞与责任人规则](#漏洞按责任人派单)。single需risk:manage和主锚/owner；bulk需主锚管理，沿B05不另要通用ticket:create、也不增派单角色。

使用相同 operation 重投时，返回原分组的操作收据；风险已有 active link 时，按当前权限返回原工单或冲突，不再新建工单。apply在group/risk/ticket锁内验证当前权限、owner在职、类型发布/字段、目标映射、剩余名额和plan基线；**每group原子提交**票、links、风险状态、双域历史和必要协作intent。任一失败该group零新票/links，其他group可成功，返回逐risk `created/linked/already_linked/denied/invalid/conflict`。不支持第21项挤进旧票；重新分组需新预览。

批次 parent operation 先持久绑定 plan_hash 和完整 group 清单。`group_operation_id=derive(parent_operation_id,plan_hash,group_key)`。分组回执以 `(parent_operation_id,group_key)` 唯一，保存逐项结果和明确失败原因。成功组的票、关联和回执同事务提交。明确失败组只保留失败回执，业务写入回滚。原号查询投影仍有权读取的成功或失败事实，未返回的组尚无可读终态。

全部组成功为 `completed`。至少一组成功但仍有失败或未决组为 `partial`。全部结算而零成功为 `failed`。零成功且仍有未决组为 `pending`。进程在 A 组成功后中断，原号重投读回 A，并只接续尚无终态回执的组，提交前仍复验当前权限和基线。明确拒绝或冲突的 B 组使用新的预览和操作号，保留原批回执及原成功组。

链接专用命令沿生命周期合同：`POST /risk-items/link-add`/`link-remove`收`{operation_id,risk_id,ticket_id,expected_risk_revision,expected_ticket_revision,expected_link_revision?,reason}`，追加还核同组、当前责任人及 20 项上限，漏洞整改另核漏洞标识一致，移出回合法待处置并重算票聚合，返回双方revision/link receipt。不能通过普通票字段写risk_id绕唯一性，未获ticket读资格不泄露其状态或标题。

受限共享创建端口接收可信 `{operator_user_id,origin=risk_dispatch,creator_owner_ref,approved_target_mapping}`，不从外部body传。工单 `created_by=当前已核owner`、审计actor=真实操作者两者都留；普通创建用例继续按自身权限。`risk_owner`为桥管理字段，不能由普通表单/fetch/Agent改成自己。普通工单及通知只发布通用整改摘要与必要引用。风险标题、描述和逐项证据从风险详情按当前资格读取。公式和受管字段各自按钉版规则处理。

工单 priority 取有效关联风险中的最高级别；`business_due_at` 取最早的风险 deadline，与节点 `sla_due_at` 分开保存。追加、移出、改级或改期时重新计算工单值，不覆盖风险原 deadline，也不用已移出的风险继续触发提醒。整组风险交给同一新 owner 时，可以在一个事务中更新 risk_owner 和合法参与关系；只交接其中部分风险时，则解除这些 links，进入待交接或重新派单，不把整张工单的复核权一起转走。旧 creator 的历史不改，原有读取资格仍按类型策略判断。

### 5.2 逐风险结论与review gate

`POST /risk-items/remediation-finish`：`{operation_id,ticket_id,expected_ticket_revision,results:[{link_id,expected_link_revision,expected_risk_revision,resolution,evidence_refs,note}]}` → `{ticket_ref,items:[{link_id,result:applied|pending_confirmation|conflict|rejected,risk_ref?,receipt_ref}],pending_effects}`。命令走工单共享转换/字段采集与风险validator；不伪造普通transition事件。

- gate=off走已发布 `in_progress→closed` 的合法完成边；gate=on走 `in_progress→pending_verify`，当前risk_owner再按合法边复核。缺边/字段/当前资格则预检或提交拒绝，不能靠临时auto边绕过。
- gate=on 的处理人 finish 此时只保存逐项整改 proposal/evidence，并将待复核项置 pending_verify；**不先把风险 closed 或 link resolved**。候选 `POST /risk-items/remediation-review` 收 `{operation_id,ticket_id,expected_ticket_revision,items:[{link_id,expected_link_revision,expected_risk_revision,decision:approve|reject,note,evidence_refs}]}`，当前 risk_owner 按票合法复核边及风险资格判定；approve 才落实已核修复结论，reject 要理由并回合法整改状态、保留原证据。接受类 proposal 仍须接受申请/非自确认流程，复核不能代替接受批准。部分复核先保存逐项结果，所有 active 项均有生效结论/合法detach才允许票终态。普通 risk.close、票 transition、自动边和任务回贴也必须经同一桥 validator，不能换入口绕过 review gate；紧急管理纠正另有明确权限/理由及历史，不默认让管理员跳过。
- 已修复逐项写措施、证据、复测人/时间；接受由当前owner本人申请且主锚确认后才resolved；处理人非owner只能提出接受建议。误报/重复须主锚资格，E10 merge未启用时不能“重复”直接吞并风险，保留proposal待处理；取消回assessed并释放该link。
- 在办票的未决项需要交接复测时，当前获权管理者使用 `POST /risk-items/remediation-handoff` 提交一位后继、理由、钉版与全部未决项修订。后继须能读取相关风险并按该票钉版执行复测。保留原建单人、派单时责任和已生效项，交接事实追加留档。
- 首批 gate-on 在待复测阶段开放风险接受申请，并校验钉版定义可关闭或打回。有效待批时锁住该资产的整改结论，由另一名获权管理者确认或驳回。读取仅标识可恢复的过期或失效申请；当前责任人可重申，或沿原复测打回边恢复整改，提交事务保留旧申请和原提案。客户端收到明确拒绝后可刷新并显式重做；结果未知时只按原操作号核查或同参重试。申请前至少须有一名当前在职的独立主锚管理者，可按该票钉版定义走关闭和打回边；审批时再验资格。已批准事项若在工单仍在办时到期，当前获权管理者可用 `POST /risk-items/remediation-accept-rework` 提交原因和原批准、风险、关联、工单及定义的当前版本，将该项恢复为可提交整改。原批准及证据保留，已解决的兄弟项保持原结论。已关单的关联保持历史状态。
- 票terminal前所有active links必须有已授权生效结论或合法detach；不能先全票closed再把尚待接受确认的risk标fixed。部分结果可保存为proposal/待处理，响应逐项反映；同库原子协调可直接落实有效项，使用异步消费的入口须明确pending。
- 首批对活跃风险关联的物理删票关闭入口。历史提交或复测仍引用的工单附件、公开评论及工单保留，关联结束后仍参与删除与回收检查。无保护引用的普通工单沿既有删除规则。SET NULL只用于保留结束关联的墓碑。

票终态和risk-resolution intent同事务。消费键 `(ticket_id,ticket_revision,link_id,resolution_version)`；迟到结果发现risk/link/owner revision变更即conflict，不覆盖新owner/周期。补发投影只更新供查询展示的数据，不重新执行复测或建单。

首页的复核待办根据当前有效 risk_owner 和 pending_verify 记录生成，同一工单只列一次。旧处理人、发起人或 CC 即使仍可查看，也不因此成为当前复核责任人。当前owner无L1/参与可见资格时报配置阻断，不为统计改派或强授业务管理权。关联详情逐对象鉴权；“风险可看”不授任意票/附件全文。

## 6. D15通知与统一审计

每条风险业务变更、risk_events/处置历史及notify intent同事务；业务成功与消息发送分别记状态。通知复用既有消息端口，真实公司HTTP profile由消息包实现，不把UIP桩/站内inbox成功当企业消息送达。

intent固定 `{event_id,event_type,risk_ref,aggregate_revision,rule_ref,recipient_selector,org_anchor,template_ref,source_ref?,causation_id}`；收件人**发送时**解析当前owner/合法CC/批准负责人并重验资源及字段权限。旧actor或创建人不代当前owner。高级别上溯抄送一级部门负责人是已定能力；B08只绑定四级映射阈值、组织锚、多个负责人、时区/频次/静默例外。

解析失败分别 `recipient_unresolved/recipient_unauthorized`，进入可恢复配置待办，不广播全员/admin、不猜负责人；撤权抑止单独`suppressed`，超时/HTTP未知分别retry/unknown。站内、HTTP受理、送达、已读都按实际合同分开；无送达回执的服务最多显示“提供方已受理”。不因发送失败回滚已完成风险评估/派单。

去重键复用[消息属主](task-activelist-message-detail.md#3-候选逻辑数据约束与事务)的 `(source,source_event_id,rule_id,rule_version,recipient_user_id,channel,semantic_key)`。适配时用稳定风险域身份和event_id标识原事实，用rule_ref解析稳定规则ID和版本，解析当前收件人后绑定其ID。风险摄取的外部source_ref不替代投递的原事实域。重试时继续用原收件人和原投递键，撤权或改派抑止旧待发，必要的新发由新事实或获权补发关联原项。template_revision固定绑定原delivery的payload，不因模板更新让旧event再次发送。每日提醒/汇总按风险、事件kind及配置时区的时隙建稳定key，重启不会重发同一时隙。通知文本仅编号/必要动作/受权深链，读取深链再次鉴权，不把敏感字段塞入IM；规则频次不是所有消息通用“exactly once”。

风险路由审计保留方法、路由、结果与请求标识；风险正文从受权业务历史查询，旧审计记录在列表与归档输出中同样按风险路由遮蔽请求体和响应摘要。业务历史按风险/票当前可见性提供；跨模块审计检索/导出另需专门审计职责，沿[统一日志审计合同](../design/unified-observability-audit.md)。风险状态、接受、owner、派单/结论的领域历史与效果事务一致，必需历史写失败回滚本地命令；日志转发失败通过持久outbox补送。IAM两操作本地强审计失败回滚已由F017确认，仍按统一合同实施，不扩大为所有日志失败均回滚。

隔离候选的首次接单提醒读取工单钉版定义的 `transition_meta.assigned.first_accept_reminder`，条目包含 `rule_id`、`rule_version` 和 `accept_within_minutes`。记录时使用该代分派时刻与发布时限计算固定截止。真实改派产生新代次，当前办理人经已发布路径接单时记录接单时间。启动扫描需要显式开启 `risk_remediation_enabled`，并设置正的 `risk_reminder.scan_interval`。站内投递在同一 IAM 屏障内重查办理人、当前动作权限和代次。隔离验收使用合成规则，正式启用仍按批准的策略和真实渠道执行。

逐资产办理从 `GET /api/v1/risk-items/remediation-context/:id` 读取工单、绑定规则、风险和关联修订，以及每项最近五次提交和复测记录。分派办理人、接单、提交整改、逐项复测和接受审批各用独立 POST 按钮绑定。客户端保留原 `operation_id`，通过 `GET /api/v1/risk-items/remediation-operations/:id` 受权核查。文件引用使用同一工单已绑定附件的 `object_key` 和 `evidence_version`，同票公开记录可使用 `ticket-comment:<id>` 及其版本。已引用附件保留在票内。办结写票后复查现势结论和接受期限，满足条件时一并结束在办关联。整改写能力和附件存储均可用时允许新派单，既有受权回执可以继续查询。

## 7. 三层权限与异常可解释性

L1维持 `risk:read/risk:manage` 两码；“可见即可评论”是风险已决语义，不能套到SOC事件观察者。L2主锚风险管理org∨副锚affected_org范围∨owner，L3按命令/状态/字段再判；副锚仅读评论，主锚普通只读成员不因部门身份得manage。普通scope=all不是管理员语义；风险仍按已批准的风险范围，不用工单类型限制替代risk资源规则。

| 当前主体 | 可读范围 | 可执行写/额外约束 |
|---|---|---|
| 风险团队获授权管理者 | 主锚获权范围 | 评估、责任管理、批准接受、批量派单；确认不许自确认；向批准目标派单 |
| 当前owner且具manage | 本人风险 | 评估/改期、申请接受、单风险派单、合法复核；不自动获改owner/主锚merge权 |
| 只读成员/副锚业务成员 | 当前风险可见集 | 读评论；不能派单、接受、改期或改字段 |
| 来源connector | 本source收件/receipt | 规范化观察，不能写owner/终态/权限，不能借服务签名读全库 |
| 后台任务/未来Agent | 当前服务许可∩grant∩资源 | 排队与执行各复验，撤权hold；查询旧receipt也需当前读权 |

同一谓词用于list/detail/stats/export/history/来源摘要/通知/关联引用；不得通过total、聚合维度或错误泄露无权风险存在。引用不可读时不显示隐藏ID/标题/数量；固定“按当前权限展示”提示与是否真有隐藏对象无关。owner/组织变动、角色直接/组织/继承撤销及本地会话禁用后下一执行复验；本地提交用共同epoch/锁序，不假称锁能实现跨公司UAA瞬时原子撤权。

## 8. D13指标、快照和风险实体时间线

固定查询 `GET /risk-items/stats`：`group_by=status|level|source|system|owner|affected_org`及有界时间窗；维度不允许时400，不拼自由SQL。响应 `{as_of,timezone,metric_version,scope_kind,groups,definitions,data_quality}`；副锚只能自家affected_org、owner只本人，主锚按live scope。空集count=0；比率分母0返回null并标no_denominator，不画0%暗示达标。

首版可证明口径：在办敞口=`new/assessed/remediating/pending_verify`；接受单单列不计已修复；逾期在办=`deadline<as_of`且在办，分母为有deadline的在办风险；已修复率=窗口内fixed关闭cycle/窗口内有合法终结结论cycle，并列false_positive/duplicate/transfer/avoid，不能混成整改成功。MTTR仅fixed cycles的`closed_at-created_at`，这是本地创建至合法修复关闭，不是SOC检测时间；标样本数、覆盖率，缺时间不补零。回归数按新cycle.regression_of，回归率用同一关闭cohort并明示观察截止，禁止把本月回归除本月关闭制造伪可比率。

按来源或系统等多值维度分组时，每组按 distinct risk_id 计数，并在响应中标 non_additive。同一风险可能属于多个组，因此不能把各组数量相加当作风险总数。没有相应字段权限的维度禁止用于筛选或分组。首版duration取自然经过秒，不含停表/工作日扣减。deadline自动计算以服务端首次assessment时间与所钉自然日policy为基准，显式日期必须经配置时区转换到具体Instant；输入不存在/歧义的本地时刻须明确选定偏移，不能靠浏览器时区漂移。

工单响应/处置/节点时长沿[工单运营合同](../design/ticket-operations-supplement.md)，不将风险MTTR当接单响应时间；历史记录缺少准确的 closed 或 first_response 时间时，标 legacy_unknown/approximate，不用 updated_at 冒充准确时间。SOC MTTD/专业MTTR由来源提供时间/定义，缺专业起点不算。

每日快照候选 `risk_daily_snapshots(snapshot_date,timezone,metric_version,scope_ref,dimension_key,counts,cutoff_at,source_watermark,quality,revision)`，唯一日期+时区+口径+scope+维度。**不可给普通用户展示全局历史聚合再用当前主表过滤**：固定全局/组织快照仅对当前明确获该历史统计范围的管理者开放；普通/owner动态细粒度范围无安全历史维度时仅查当前受权行或显示历史统计未提供。不可用当前行补算历史敞口假装快照。更细历史ACL/重放另立需求，不默加全量事件湖。

快照计划切点用配置时区当地日界，存 planned_cutoff；实际读取用单条SQL或**只读REPEATABLE READ事务**保存 snapshot_as_of，不能拿默认READ COMMITTED多SELECT冒称一批同快照。重跑同key幂等；若迟到且没有可还原日界的事件水位，显示gap或明确实际时刻的late_snapshot，不能把当前状态标为零点历史。保留可还原事实时才补算并标backfilled/其水位，不能拿今天状态填昨天。日维护调度沿唯一维护CronScheduler，声明单实例/多实例advisory锁，失败入既有cron状态/告警；用户周期报告归TR，不再起另一调度器。

D13-T `GET /risk-items/:id/timeline`只聚合该风险与获权ticket/TR/notification引用，分页`cursor,limit<=100`，默认时间窗/总条数受预算。每项 `{origin,source_event_ref,event_kind,occurred_at,recorded_at,actor_ref,authorized_resource_ref,summary}`，同源稳定排序 `(occurred_at,origin,event_ref)`，晚到记录可回看；跨源无共同快照时标as_of各水位，不假称全局线性顺序。故障返回明确partial/provider_unavailable且保留已授权部分；禁止为填满页面无限逐引用查询。普通业务时间线不等于跨域审计授权。

## 9. D14 SOC引用与本地协作的准确首包

职责/动作矩阵沿[SOC协作授权合同](../design/soc-collaboration-authorization.md)，不复制第二套权限。候选表 `incident_cases(id,provider,external_id null,source_revision,source_observed_at,source_state,local_status,scope_revision,revision,campaign_ref null)`；唯一非空`(provider,external_id)`；`case_roles/case_links/case_events`分别管岗位、受权引用及本地历史，引用SOC事实不能由本地manage覆盖。临时无SOC ID仅显式local_pending_reference，后续追加映射保留旧ID。

候选 `GET /incident-cases`、`GET /incident-cases/:id`、`GET /incident-cases/:id/timeline`；写用 `POST /incident-cases/collaboration-plan` 与 `/collaboration-apply`。apply输入 `{operation_id,plan_id,expected_case_revision,expected_scope_revision,source_gap_ack?:{source_revision,observed_at,reason}}`，来源gap开关由服务端，不由ack启用。输出 `{case_ref,local_task_refs,receipt_ref,pending_effects}`，通用400/403/404/409和receipt规则同前。

case协调者仅向批准类型/组织创建本地岗位票；岗位办理者只能提交本人当前岗位证据，协调者不能冒签他人结果；风险组成员不自动得incident职责。创建/关联/case历史和可靠intent同事务；跨对象核当前权限，未知提交结果查原receipt。`local_status=completed`仅代表本地协作完成，SOC专业状态单独只读显示。

B09已确认：SOC源陈旧时自动按旧专业状态新增派单held；当前授权有效的协调者可确认所见source_revision/时间并填理由后手工创建**本地协作**。来源恢复则追加新revision/对账，不自动关本地票。权限/UAA失效不可被source_gap_ack豁免；不授隔离/封禁权。战役仅轻分组+有界时间线/actor筛选/按活动导出，不落专业日志湖；五秒/两点击为待实测UX目标，不是已达到指标。[TheHive cases](https://docs.strangebee.com/thehive/user-guides/analyst-corner/cases/about-cases/)将任务、附件和共享范围纳入case，本项目只借用受控协作结构，专业分析和事件结论保留SOC写主。

## 10. E10风险Excel和跨指纹合并

P6.1 出口前完成条件核验和方案，不必等 D08/D10 产品完成才开始调查。风险 Excel 导入是已确认的条件需求；默认限定流程为下载模板、离线生成导入预览（plan）并逐行校验、由授权用户确认，再通过标准风险用例或 feed 导入并返回逐行收据（receipt）。逻辑记录 `import_run(run_id,file_hash,template_version,source_namespace,actor_ref,status)`，行唯一`(run_id,row_key)`；断点重跑相同身份，不把整表重新造risk。报告含成功/失败/冲突及安全原因，失败行可修正后新revision重提，不能用直接灌DB代替权限/来源幂等。文件模板/保留/量级须绑定真实样本，私有转换器不导出公司原文件。

跨指纹merge是条件候选，未启用不能用close duplicate偷偷实现。获准后plan显示master/slaves revision、owner、接受、active links、来源归属和可见集变化；管理者CAS apply，保留merged_into/主从历史与源alias。不同owner、有效接受或active ticket冲突先hold并给人工处置，不自动转走责任/读权。重复apply幂等；源再报沿批准alias归主族，历史generation与原风险编号不重写。撤销为新补偿命令，不能删历史假装未合并。[DefectDojo去重](https://docs.defectdojo.com/triage_findings/finding_deduplication/about_deduplication/)区分匹配范围和人工归并，可借鉴其保留主记录/回链，不能据此默认把所有跨源同标题视作同风险。

两者实际建设按获准子包另估、固定P6.E窗口；E10的2–4日只计调查裁决。无真实存量/跨指纹诉求则明确未激活与重开证据；取消已确认Excel效果仍须owner确认。

## 11. 最小UI、配置和运维恢复

风险列表沿用统一的表格、查询栏、展开筛选区和操作区；详情分“评估与责任、整改票、接受/复测、来源与历史”。多选派单先预览，提交后按分组展示结果。已明确失败的组从保留原回执的界面另做预览，用新操作号提交。尚无终态的组仍可按原号接续。页面根据 capabilities 显示可用动作，后端仍须重新鉴权。源 severity 与本地 level 并列，accepted 和 fixed 分色分文案。归档保留原关闭状态。表单保留用户输入，409 提示刷新比较。无 owner 可保存 new，派单入口明确阻断。

派单提示“创建人：风险owner；操作人：当前用户”，不写“以他人登录”。票结果按risk逐项展开，pending_confirmation/协作待补/unknown各给受权动作。接受页列到期日和申请人/确认人；到期重审有待办。通知不可达与业务失败分开，详情可看到受权的待发/待解析原因；不给普通用户暴露消息接口原始响应/密钥。

配置最小集：源mode/schema/mapping版本/size/rate/retry上限；level→自然日deadline；目标type/org/template映射与review_gate；接受制衡默认开；D15规则/时区/上溯锚；180/365保留类别；查询/导出/时间线预算。仅必要默认，真实公司profile不进公共仓。风险服务禁用时既有IAM/工单/TR/AL仍运行，未启用能力不伪返回成功。

监控使用有限标签：ingress accepted/duplicate/stale/quarantined、oldest pending、source lag、active-link冲突、acceptance到期积压、通知pending/unknown、对账差异、snapshot gap。risk_id/工号/标题/operation_id不作Prometheus label；关联ID进脱敏结构日志。只读健康不触发源重采/派单。

恢复入口：①同ingest/operation查询receipt；②有权维护者重试held的既定输入（如需改参数先新plan）；③对账扫描 risk/link/ticket/intent差异；④逐项确认冲突。恢复不绕权限、不自动把unknown当failed重做。流控只延迟可重试工作，持久意图先落；积压超阈停止新自动派单并告警，人工读及受权已有事项办理继续，不能丢未处理记录。

## 12. TDD验收清单、依赖与待确认

R1/D07–D14各端点首次实施前，由对应风险用例属主交逐端点目录绑定表：方法/路由、L1权限码、页面GET或写按钮、`menu_apis`/`role_menus`迁移归属、服务间源凭据入口/受控selfService及依据、确需`catalogExempt`的精确理由；再列L2/L3与不可见/无权反例。风险查询、抑制资源、派单和SOC协作按各自资源职责登记，不因共享risk前缀授通配权限，也不为了字面统一强改资源名。目录表与OpenAPI、seed和B案守卫测试同批交付，不等RELEASE现场裁定。候选权限名/豁免不是已存在的授权。

以下是待写产品测试的具名验收规格，不是本轮已通过的计数。先按公约为本批全部正/负/边界/权限用例取得有效红，再实现；并发反例按现行至少8次稳定红/变异验牙取证。mock/contract/real三层分列。

| 用例组/候选测试名 | 正常与失败预期 |
|---|---|
| `RiskReport_DuplicateAndConflict` | 新event计一次、同号同hash重复零增量；同号/等sequence异hash冲突落库且原观察不变，冲突重投去重、落库失败不假ACK、无权不能看raw；整批超限零效果、合法批部分字段错逐行报告 |
| `RiskCycle_OrderedRegression` | 在办归并、fixed后真正新观察新一代回链、旧观察/等时未知不造回归、两个并发新观察只有一代 |
| `RiskReport_CrashAfterIngest` | 收件后崩溃可恢复；commit响应丢失查原receipt；raw清理后同event仍不重复效果 |
| `RiskAcceptance_CurrentOwnerAndEpoch` | owner申请他人确认、自己确认拒绝；owner/level/revision变旧superseded；撤权先提交后旧请求不能提交 |
| `RiskAcceptance_RenewalExpiryRace` | 新续期先提交则cron不重开；cron先到期则旧确认409；有效接受重推只计数，过期重开原cycle |
| `RiskDispatch_GroupAtomicAndLimit` | 合法N:1、20项上限、第21项/不同org或definition拒绝/新预览；任一组创建失败无孤票；group部分成功可重试 |
| 漏洞整改派单与追加（B2） | 同漏洞同责任人共单，团队标签不同也可共单；同漏洞不同责任人、不同漏洞同责任人分别派单。追加核漏洞、资产责任人及权限，失败保留原单；重试不会给同一组合另开在办单。 |
| `RiskDispatch_TrustedCreator` | 风险管理者无通用建单权仍可向批准目标派；只读/任意目标/伪created_by拒绝；真实actor与owner分别审计 |
| `RiskHandover_PartialGroup` | 全组同转更新受管owner；部分转detach且其它risk复核权不变；旧creator历史不改；无合法继任保持待交接 |
| `RiskFinish_PerLinkResult` | gate on/off均字段校验，固定流程缺相应完成/复核边时发布或派单预检拒绝；fixed/accept/proposal/误报/duplicate/canceled分别正确且canceled回assessed非fixed；pending接受不能全票terminal；迟到旧link结果409不覆盖 |
| `RiskNotify_CurrentRecipient` | 入队后owner/CC撤权不发旧人；缺负责人/无权负责人各独立待办；HTTP超时unknown不当送达，业务保留成功 |
| `RiskRead_ConsistentScope` | 主/副/owner三支与L1/L3交叉；list/detail/total/export/stats/时间线/任务结果同范围，隐藏与不存在输出不可区分 |
| `RiskStats_MetricDefinitions` | 分母0=null、accepted不算fixed、缺时间不补零、月底/闰日/时区日界；无权历史快照不泄露，缺日不伪补 |
| `IncidentCase_SourceGapAndRoles` | B09人工确认可协作、自动held；缺理由/撤权/越目标拒绝；本地完成不改SOC终态、迟到source不回退新revision |
| `RiskImportMerge_ReplayAndConflicts` | 条件获准后验逐行断点/权限/报告与merge主从回链、active link/owner冲突、重复确认和来源再报归属 |

产品实施后的最短旅程应这样验收：核实同一漏洞影响同一责任人名下的两台资产，向获准目标派到同一工单；处理人逐项填写措施和证据，按配置复核，逐条形成合法结论；最后核对风险、工单、历史和消息记录是否一致。还须分别验证撤权、响应丢失、consumer 重启、部分风险转 owner，以及消息失联的故障处理。TR/AL 的任务效果另按共同互通模块验收，风险旅程通过不证明整条跨仓链已经通过。

依赖按子出口：D07查目录需要X02-Q及新资源护栏；D08消费已批准source规则/最小目录与持久接入（完整feed范围以总账为准）；D09/首条通知前有D15+R01；D10等D02字段采集/可信受限创建和本地事务守卫；D13-M不等通用BI，D13-T仅消费已交D11/D12引用；D14等D10/D13但不先建专业SOC。完整UAA-P/R、AI、全站视觉推广不阻本地风险闭环。

当前不重新询问已确认的职责/N:1/接受制衡/设备/本地先行原则。**启用前需绑定**：

1. 首批真实风险类型/字段/目录、目标工单类型/组织/owner名单及review_gate：漏洞整改需核实资产责任人及每项能否纳入获准目标；固定流程须显式配置gate=on的`in_progress→pending_verify`及所需复核边，gate=off须显式配置`in_progress→closed`合法完成边。发布/派单预检缺边就拒绝该画像，不让运行时才发现无完成路径；管理员对预览样本确认而非逐字段技术决策。
2. B08真实通知实参：公司源五级如何映射四档、从哪一业务部门上溯、一个一级部门多负责人是否都须获权接收、汇总时区/时刻及静默例外；未齐只启合成规则，不假送达。
3. 来源新鲜度/稳定键/序号/撤回及观察时间真实语义：例如扫描重导报告只是更新时间变更，不能据此判已修复后回归。由提供方合同验证，不要求公司资料外传。
4. E10 P6.1出口：真实Excel模板/存量量级、是否已有迁移工具，以及一个跨指纹并单实例；按固定窗口裁决并给独立子估算。来源不具备不阻人工风险用例。
5. 正式部署前的数据类别保留和审计人员范围，沿统一发布门绑定；180/365默认不是对公司制度的推定。技术锁/typed命名由实现审查处理，不新增owner审批流程。

## 13. 公共reference v1：两风险的完整纵切

本节的序列/观察投影已有[可序列化向量及Schema](../design/examples/capability-reference/module-profile-vectors.json)，固定摘要见[参考manifest](../design/examples/capability-reference/module-reference-manifest.json)。它只验选定形状；本节角色/派单/关闭回归的时序和当前权限仍须产品独立预期，不冒称10个跨模块样例已执行完整风险旅程。

本节保留通用风险分组的参考输入。首批漏洞整改另用[代表场景](#14-漏洞整改代表场景)验证漏洞、资产与责任人的关系。

本段是完全合成的可复用测试输入/逐步预期，**不是实际公司映射或已运行产品**；风险模型/HTTP尚无实现。source_ref=ref-risk-v1、mapping_ref=ref-risk-mapping-v1，稳定source_item_key=finding-A/B；family scope=source_ref、canonical_key=source_item_key、key_schema_version=1，不随标题/IP/备注改键。source_sequence为规范无符号十进制串，核心拒符号/小数/指数/前导零；adapter仅证明源等价语义后归一化。比较按长度/同长字典序，不经JS number；observed_at为实际再次观察时刻。较大序号但观察仍在关闭checkpoint前，只更新适用传输事实，不建新cycle或改处置终态。

仅合成五档映射5→urgent/1、4→high/2、3→medium/3、2/1→low/4，未知held，不代替B08生产映射。EvidenceRef统一provider/external_id/version?对象；生命周期旧字符串例已校准，不能同一版本时而字符串时而对象。新wire用urgent，critical只为历史词。原始源digest、规范化payload_hash、mapper版本分别保留，不能把adapter重新编码当来源新结论。

合成组织G=91001、受影响D=91002、目标T=91003；M=92001具risk:read+risk:manage及虚拟风险组G管理职责，U=92002具相同功能许可且为两风险owner；P=92003为处理人并在本fixture显式获D风险read，V=92004仅D风险read，H=92005无风险L1，N=92006有read但范围为空。M/U/V按主锚/owner/副锚独立预期见A/B、total=2，V不得派。H任意ID均入口403且不查存在性；N对隐藏A/不存在均404。P票权不自动赋风险读，撤显式风险L1后详情/统计/关联投影按入口或可见性拒绝/省略，不能靠比较各入口“相同”代替独立范围真值。

批准映射向ref_risk_remediation的固定definition_version=1/合成hash派票，最多20同owner/org/type/definition共一组；不同任一维度先分组。参考字段为risk_owner（domain写主的只读普通字段）、风险摘要、remediation_note、remediation_evidence；file字段声明/DTO对齐依C02/TICKET接线，collect_fields/证据仍走D02。结构化risk links由桥写主，文本字段不是风险关系真值。

**必须新增的可信事务接缝**：`internal/service/ticket/service.go`的Create当前自开tx，creator、org成员资格、applyFetches和bindAttachmentsTx都用actorUserID；不能用Create(owner)冒用U，也不能顺序建票后再连risk声称原子。候选共享事务内创建内核接受已核准的风险桥上下文，creator=U、真实operator=M，取数/附件仍核M，目标org资格用受信批准映射，不接外部created_by/skip_auth。现Create拒assigned_to仍保留；如需初始处理P，在同一受权事务内通过正常分派核完成，不从raw创建字段偷设。

同库按§4的共同守卫→组织/所钉定义→group/family稳定键→risk ID→ticket ID→link ID锁序，复用G2/公式/附件、org FOR SHARE及编号预取，回滚烧号允许。transition_service.go的Transition也自开tx，提交后emitTicketEvent/RunPostActions不是可靠风险完成intent，须抽同事务转换/领域事实接缝。风险可借工单Authorize/GetFilter注册模式，实际谓词按风险主/副锚/owner独立实现，不复制creator/assignee/CC权。

逐步预期：ingest A/B→受权台账→评估/owner/期限→plan/apply同组同票→P按每link填措施/证据。gate on先pending_verify、U合格复核才落实；off走批准完成边，不跳字段/证据。P接受仅提议，U申请、非申请人M确认；pending阻整票terminal，已fixed不回滚。**finish逐link**可A已落实/B冲突并返回逐项结果；派单同组失败必须零孤票，不是A建票/B失败。另用目标不同而拆两组的变体，A组commit后kill只续B组；改参数须新plan/operation关联原parent。

REF-RISK-01–08独立场景：稳定键/大序号/等序异hash；各主体列表/详情/count/export/stats/时间线/receipt范围；无通用ticket:create的M合法派与V拒；可信creator/operator和字段/附件分轨；gate on/off及接受制衡；分组原子/部分恢复；checkpoint后新实际观察只开generation+1一次而accepted有效期重推只计观察；首通知失联/撤收件资格不伪送达。raw清理后重推仍查ingest/receipt，不因无原始JSON重复生成风险。

合成通知取UTC每日09:00、中低汇总、临期3/1天、同类每日一次用于固定重放；生产阈值/锚点/负责人/时区与渠道仍独立确认，不用该fixture启公司规则。实施先reference/权限预期→ingest/family/cycle/receipt→评估接受→票桥与逐项恢复→首通知；D13固定指标/日快照/有界时间线、D14 SOC引用/岗位协作、E10调查与获准P6.E子窗保持，不扩事件湖/BI/CMDB/SOC。来源排序/撤回、实际映射与批准名单沿原Q01/Q07/Q08/Q12集中绑定。

真实复用锚：`internal/service/ticket/create_move_race_test.go`的TestBK11_CreateBlocksBehindConcurrentMove、`validation_tx_pool_integration_test.go`的TestValidationReadsReuseTransaction、`notification_creator_integration_test.go`的TestNotificationCreator_ServiceEvents、`authz_resource_integration_test.go`的TestTicket_R4_InvisibleReturns404及`attachment_bind_integration_test.go`的TestCreate_TicketAttachmentBind（后四同目录）。它们未证明风险新实现；本页原RiskDispatch_TrustedCreator/RiskFinish_PerLinkResult/RiskRead_ConsistentScope承接这些reference断言，不另算一套范围或费用。

## 14. 漏洞整改代表场景

下面用虚构账号、资产和漏洞说明首条人工整改。将情报中的漏洞与资产目录里的设备匹配后，风险团队在自己负责的范围内核实、定级，有派单资格的人把选定风险交给整改人。整改人接单，核实漏洞是否适用，疑似误报时可以交证据，请有权人员判断；确认要整改则提交修复结果。当前风险负责人（owner）逐项复测。若确实接受某项风险，由当前负责人申请，另一位有资格的风险管理者批准，本人不能自批。整改人可以提出建议。等每项风险都有生效结论、或已经按规则移出，才能结束工单。正式接口、时限、两级审批和多来源接入仍按对应阶段确认及验收。

### 开发与隔离验收的外部依赖

开发、CI 和首条旅程的合成联调使用可替换 mock。核心同一用例消费领域端口，联调走同一装配机制与公共调用边界，只使用合成配置、请求和回读，按 provider 能力启用子接口。测试分别记录领域事实、合成 HTTP 契约证据和各提供方真实联调证据。开发与 CI 的测试路径只访问显式配置的隔离 fixture。单独批准的真实联调使用相应真实 profile，配置或权威依赖失败按对应合同报告或待处理，禁止切入 mock 数据继续生产办理。

- **情报与 CMDB**。情报 mock 提供稳定来源/事件键、漏洞标识、有效版本范围或明确匹配结论、快照时间、证据引用。CMDB mock 提供合成稳定资产、组件/版本、所属组织、系统责任人引用、数据新鲜度及查找结果。首条以单一模拟来源、两条已知匹配为正例，另验未知资产、无命中、空值、陈旧、超时和重复。尚无法解析的观察留在接入待核状态，进入台账且可归属的候选走 `new` 人工核实，具当前评估事实才进入派单预览。真实 CMDB 是否返回权威匹配结论归 Q08。若只给资产目录，冻结匹配规则、受影响范围、依据和反例后单列估算。多源变体保留各源证据，跨指纹合并仍走 E10 原策略。 [风险来源和接入](#4-接入处理事务和并发) · [目录查询端口](extension-ai-module-detail.md#3-x02-q先可替换查询再有状态-feed)
- **HR 与本地权限**。合成目录给稳定人员 ID、有效任职、直属主管关系及其版本/新鲜度。固定正常、缺主管、停用、变更和无法确认关系的 fixture。主仓仍从专用本地账号、组织与授权层判资格。主管关系输入只提供审批候选，审批与工单数据可见性走当前权限，失去资格不得办理。首条人工试用保留本地账号路径，后续真实 HR 逐项验证来源和管理关系。 [目录与组织生命周期](../design/business-lifecycle-contracts.md#71-公私边界与目标-dto)
- **站内消息与外部渠道**。开发以可控时钟验证站内应发、待发和失败，HTTP sender 使用隔离 mock 表达拒收、接受、可证送达或仅受理、响应丢失及原状态回查。收件人按事件、已确认等级、当前职责及可见字段求值，收件失效抑止旧待发。真实渠道的能力和时区等实参走 B08 与 MH01/MR1。 [消息端口](task-activelist-message-detail.md#6-消息公共http能力与一条私有映射)

### 首条人工整改的业务步骤

1. **核实与定级。** 同一漏洞在资产 A 和 B 各产生一条待核风险。风险人员核实漏洞标识、资产和责任人，再按每台资产的实际情况记录级别、依据、处置策略及版本。合成样例取 A 为高、业务截止当日 17:00，B 为中、业务截止次日 17:00，分别核即时通知与汇总规则。合票取高等级和当日 17:00 作为初始业务截止，接单时限另取该票类型在样例里指定的 30 分钟。复杂定级后继做可回放策略，分别保存输入摘要、规则版本、建议等级、命中理由与人工覆盖理由。等级及后续默认时限/知会规则分开保存，在途变化沿受控操作留历史。
2. **批量派单。** 两台合成资产 A、B 属于同一位责任人，向同一个获准目标派成一张工单，分别保留各自的受影响记录。换一个责任人或另一个漏洞就各自派单。复用在办工单时按漏洞及当前责任人核对，重复应用返回原结果。预览绑定每项匹配或获权人工核实的依据及版本。提交时重验已持久化的依据、有效期和权限，失效或无法核实时该组待核、零建票。有明确证据的获权人工核实可形成带留痕的本地依据。合成匹配成立时仍须验无派单权的操作者被拒。保留实际派单操作者、工单初始 creator、当前风险 owner、当前整改处理人和复测责任的各自证据。 [分组与派单合同](#51-planapply和原子边界)
3. **接单与核实。** 样例采用 UTC 可控时钟，09:00 按资产责任人 U 建单，09:05 由获权的 M 将原票办理任务分派给 P 并进入待接单。保留 U 的资产责任、派单时部门和当前复测资格，工单数不增。09:35 为接单截止。09:36 触发一次应发，去重包含节点发生次、分派代次（该票每次重新分派递增的计次）和投递身份。扫描和发送前均核当前代次、仍待接单、收件资格。09:40 合资格人工接单后抑止尚未发送的旧提醒，保留已发与超期历史。建单到首接单为 40 分钟，当前分派到接单为 35 分钟。站内入箱或外发成功不作为起点，也不等同于接单。接单只停止接单提醒。当前处理人确认适用性，发现疑似误报提证据交受权风险人员判断。转派重验目标资格，保留首次接单、原期限和超期历史。生产时区/日历、触发时点和再次分派的提醒策略走 Q10/Q11。 [工单里程碑与时限](ticket-module-detail.md#41-主责接单管理与时间)
4. **修复与复测。** 处理人先填修复方案和计划实施时间才进入修复阶段，完成后逐项提交证据进入待复测。样例启用复核门，发起人同时是具名风险 owner，由另一人整改。发生合法交接后按当前风险 owner 及复核范围重验，旧身份或旧提案版本无提交效果。锁内按 §5.2 写结论、证据、历史、票状态与必要意图。17:01 仍有已超期风险待复测时，显示逾期在办并保留原期限。接单或等待复测不自动改期或关票。A 通过、B 打回时保留 A 的确认和 B 的失败证据，整单闭环持续计时。B 再提交并通过后，所有有效关联具有生效结论或合法移出才可关闭。若最终确认发生于当日 17:20，整单闭环历时 8 小时 20 分钟，之前已发生的逾期事实仍可查。技术归档按留存记录，发布新版后，已存在的票仍按各自钉住的已发布配置版本继续办理。 [逐风险复核](#52-逐风险结论与review-gate)

**同一场景的独立变体**。在风险接受分支验证当前 owner 申请、本人确认失败、另一名获权主锚管理者批准及到期重评。通知变体验高风险即时意图、中风险合成汇总、受权部门负责人解析、撤权后抑止及无人时不广播。站内提示和外部 mock 状态各自留证，真实送达另按渠道合同验证。

### 扩展分支与各自验收

- **延期双签候选（Q10/E03）**。以按风险项申请为设计候选。申请绑定风险项、工单、当前业务版本、新截止、理由和缓解措施，双方签署同一申请版本。发起侧由实际发起人或具名风险责任人的主管确认，执行侧由当前整改责任人的直属主管确认。实际角色取法、同人命中两侧与缺员替代须由所有者确认。建议两侧独立、禁止本人确认，待批继续计时。对非终态整改，最终写入时复验当前职责、主管关系新鲜度和申请/业务版本。单签、拒绝、失效或缺主管都保留原期限并可查原因。双方有效才同事务更新获批风险项的截止、重算整单最早截止并写历史，原始值、旧超期和节点/首响时钟保留。启用强制双签时，评估、普通编辑、工单侧、批量和自动入口均使用同一改期限约束。派生的工单截止由已生效的风险期限及关联计算，票侧不能自行覆盖。正常初次定期限和合法关联变化沿原合同。风险接受继续走 §2.2，任何延期均不改变修复结论。 [轻审批与日历窗口](early-disposition-cards.md#2-e03分派日历轻审批与认证增强)
- **Taskrunner 与 ActiveList**。人工试用独立验收。组合阶段从已发布节点动作或整票闭环后动作中冻结一个触发事实。例如在确认修复方案节点触发只读取证，或整票最终确认后更新整改跟踪清单。所选提交生成持久意图，闭环后变体在单项通过时零触发。命令复验当前执行与目标行/字段权限，绑定原票/节点/操作。失权时暂停，效果未知先查原收据，迟到结果只回原运行。验跨仓效果使用实际本地 TR/AL 实现，外部依赖仍走 mock。实际无人值守按 Q09 单独准入。首版本地正式仍按原计划验票节点→任务→清单及受权后继，单一闭环后动作只证明本变体。 [跨仓结果与清单](task-activelist-message-detail.md#4-工单节点tr结果与al写)
- **验收归属**。人工试用工程先用模拟数据验证同漏洞多资产、按责任人分单、不同漏洞分单和重复派单、接单和闭环分别超期、部分复测、接受制衡及级别知会，叠加撤权、转派/消息迟到、失败重放。Q12 的首条真实试用建议采用真实的本地角色和经批准可核的人工风险与资产依据，由业务所有者确认。开发用的合成目录、匹配或投递结果只用于研发自验。实际流程唯一依赖的消息渠道须先实测，或采用业务已批准并验证的备用处置。真实多源和 CMDB/HR 接入、完整定级策略、双签及自动链按对应窗口独立验收。若接单提醒超出原有子项估算，或真实试用必须依赖双签/真实外部供给，按所需能力前移绑定并净估。 [当前交付顺序](../planning/intranet-uaa-release-schedule.md#1-接下来怎么做)

## 变更记录

- 2026-11-20：增加漏洞整改代表场景、外部模拟接口、两类时限和逐项复测验收，登记两级延期、真实来源及跨仓动作的待定分支。
- 2026-11-21：§1/§14 就地解释共票、三层鉴权、源观察、闭环并固定风险团队/风险 owner/整改人的职责读法，钉版读法与工单页一致；不改编号、时点、锚点与合同。

补充具名TDD：`TestDispatchBatch_GroupCommittedThenCrash`断言A提交/kill/重投只续B且parent不能提前completed；`TestRiskReviewGate_AllMutationEntrypoints`覆盖finish/close/transition/自动边均不能绕owner复核；`TestRiskTicketDelete_AtomicDetachAndRace`覆盖未接删除桥时拒删、接线后tombstone/风险/link同事务、与finish/owner交接竞争及已accepted/closed风险不反向重开；`TestRiskSnapshot_LateRunAndIsolation`覆盖多事务并写下同批快照一致、迟跑不假装日界和缺日gap。这些仍是待实现回归规格，未运行产品断言。

- 2026-11-14：按当前代码/本地首版边界补D07–D10/D13–D15/E10详细接口、逻辑DDL与唯一约束、跨域事务/权限、正常异常/并发恢复、薄UI和具名TDD规格；纠正R1指纹全表唯一待定、R2误报独立终态、R3泛终态直关及R4历史聚合越权风险。未改业务代码/迁移/估算，未宣称新增用例已执行。
