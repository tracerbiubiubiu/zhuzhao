# Phase6 风险模块详细设计

日期：2026-11-14。状态：**可评审设计候选，未实现、未跑新增产品用例**。对应 D07–D10、D13–D15 及 E10 风险子范围；正常/异常语义、typed DTO 和逻辑表约束在此细化，业务已决项沿[生命周期合同](../design/business-lifecycle-contracts.md)§8–10。原 R1–R4 的历史量级、整包前置及旧草图不作为新承诺；排期/预算只由[总账](../review/planning-backlog.json)和[本地首版净估](../review/local-core-release-estimate.json)维护，不重复计价。

## 1. 实现基线、边界和阶梯

本次定向核查 `internal/model/`、`internal/handler/`、`internal/service/`、路由和迁移目录，风险主实体/服务/路由仍未实现。可复用的是 `internal/service/ticket/service.go` 的 `Create`/`ExportStream`、`internal/service/ticket/resource.go` 的 `Authorize`/`GetFilter`、通用标签/人读编号、通知和任务基础设施；它们不证明风险接口、风险三层权限或跨域事务已经存在。工单现有 `Create` 从 actor 派生创建人，风险桥不能简单换 actor 冒用 owner，也不能直接写工单表。

- **本地首版**：人工录入/一种已确认规范化来源、台账与详情、四档定级、责任和期限、接受申请/制衡、受限整改派单、至多20风险共票、逐风险结论/复测/证据、首条风险通知及失败恢复。固定受审流程/基本字段满足业务，不以全流程编辑器为门。与工单/TR/AL/消息走共同用例通道；无需真实 UAA，保留本地完整 L1/L2/L3。
- **同阶段后继**：D13 固定指标/每日快照/有界风险时间线；D14 SOC引用与本地岗位协作；E10调查后批准的Excel迁移或跨指纹合并。上述不冒充本地首版硬前置。
- **后置能力**：第二类来源、通用连接器、评分模型、工作日日历、通用BI/DSL、专业研判/扫描/隔离封禁不在首版顺手实现。既定业务效果仍由原包承接，不以缩小首版取消。

采用 DefectDojo 的“源记录规范化、稳定匹配、处置历史保留”模式；其[重导入](https://docs.defectdojo.com/import_data/import_intro/reimport/)允许按配置自动关闭或重开，本项目选择**源观察不直接变更本地处置终态**。这是本项目生命周期取舍，不宣称复制了其全部漏洞管理能力。

## 2. 写主、模块端口和公共类型

风险域独占 risk family/cycle、来源观察、本地评估/接受/责任/逐项结论；工单独占票及办理事实；风险工单桥协调 links/受管字段，通知域独占发送事实。来源 adapter 只转换/传递规范化观察，不导入 `internal`、不写本地状态/角色/工单库；asset_ref 来自外部 CMDB 或明确临时映射，不建设第二资产权威。

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

无 level 的新风险先显示“未评估”，不假定低风险。字段缺省/显式 null/空串逐类型校验：nullable列可清空，必填引用不收空串；PATCH式修改用 `changes` 中显式出现的字段，未出现不变。`operation_id`是一次效果身份，`risk.assess.v1`等只是能力定义别名；重试相同输入沿原身份，同号异参409。与既有 `Idempotency-Key` 的映射归共同门面，不在风险域再造第二套幂等框架。

### 2.1 查询、录入和来源接入

候选 HTTP 路由均在 `/api/v1` 下，仅 GET/POST；写操作的业务标识在 body。新增按钮/页面绑定遵 B 案，不能把写端点挂风险列表页。

- `GET /risk-items`：`page>=1,page_size=1..100,status?,level?,owner_user_id?,affected_org_id?,source_ref?,system_ref?,q?,sort=created_at|deadline|level`。固定字段排序加 ID 稳定次序；先授权/字段裁剪再筛选/计数，返回 `{items:RiskView[],total:int,page,page_size}`。首版沿现有分页，不承诺跨写入稳定快照；批导出另绑定查询水位。
- `GET /risk-items/:id`：返回 RiskView 和有界当前来源摘要；findings、history、links 各分页，禁止默认把全部原始JSON/附件/关联票正文拼入详情。不可见与不存在均404；可见但动作不允许403。
- `POST /risk-items/create`：`{operation_id,title,description,org_id,affected_org_id?,owner_user_id?,system_ref?,evidence_refs:EvidenceRef[],attributes:object}`；返回 `{risk:RiskRef,status:new,receipt_ref}`。手工源命名空间由服务端建立，禁止传外部source_ref伪装扫描结论；title/description/扩展字段沿统一大小上限与schema白名单。
- `POST /risk-items/report`：body为[FindingObservationV1](../design/business-lifecycle-contracts.md#81-规范化来源合同与唯一写主)或有界批次 `{batch_id,observations:[...]}`；批次建议上限100条/1MiB、来源峰值20QPS沿既定初值，配置可收紧。`source_sequence`保留字符串，由该源已验证比较器比较，不能按任意字符串字典序或无界JS整数比较。
- report返回 `{batch_receipt,items:[{event_id,ingest_id,result:accepted|duplicate|stale|quarantined,processing:pending|applied|held,risk_ref?:RiskRef,safe_reason?}]}`。`accepted`仅说明已持久收件；不可解释为已评估/已建风险/已通知。查询 `GET /risk-items/ingest-receipts/:id` 重验源范围或人工权限，不返回越权risk_ref。

批次整体格式、签名/来源、大小不合法时整批400/401/403/413且零接入效果；合法批次中的字段/稳定键冲突逐项隔离并返回结果。外部未知字段默认不落强schema，仅允许经映射白名单的 attributes。HMAC只证明来源服务，不能授权它上传本地 owner/accepted/closed 等字段。

### 2.2 评估、责任、接受与结论

- `POST /risk-items/assess`：`Mutation+{id,level,assessment_note,treatment,deadline?}` → `{risk:RiskRef,status,level,deadline,receipt_ref}`；允许new/assessed，关键评估改变时使旧pending接受申请 superseded。deadline为空时按级别policy自然日带出；保存首次init_deadline和policy version，后续改期要求reason。
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

1. ingress验服务/源范围、schema/体积/限流；短事务持久收件/hash与可恢复处理状态，再答accepted。基础版可同请求完成后半处理，但持久收件和领域完成必须分别记证据。
2. consumer取固定mapping版本、解析目录/CMDB/外部结论，网络在锁外且有总预算。不能解析稳定身份时held，不因IP/姓名变了就造另一风险。来源映射变化需预览旧键影响，禁原地重hash抹历史。
3. 需要本地授权的命令先取共同C04守卫→适用组织/所钉定义→group/family稳定键→risk ID→ticket ID→link ID，各类内部升序、无对象跳过；不建第二套锁序。family首次创建由唯一冲突重读，不依赖不存在行FOR UPDATE；来源consumer仅本源系统作用域，不伪造真人；归并计数不能顺带派单。
4. 锁内复验mapping启用状态、来源顺序、family当前cycle和revision。新event同内容计一次；同event重试不加计数；旧序号stale保留收据不覆盖镜像；等序异内容quarantined；timestamp相等且无可比序号、无法证明新观察时held，不能按接收时间裁定回归。
5. 一事务提交cycle/镜像/观察应用状态、历史和下游intent；未知提交结果先查原receipt。consumer崩溃靠同一ingest_id恢复；不将“任务已受理”当完成。

修复关闭保存各源最后有效观察checkpoint。之后确有新观察才在family锁内建generation+1、regression_of回链并移动head；同时两条新观察最多生成一代。接受有效期/误报抑制期内只增观察；接受到期用申请版本+cycle锁CAS重开原cycle，续期先提交则expiry no-op。误报抑制过期的新观察可建新待评估cycle，标suppression_expired，不计修复回归。withdrawn/deleted/全量缺失不作已修复。

首版保留单体短事务+数据库约束，不引分布式锁服务/跨库事务。[PostgreSQL约束](https://www.postgresql.org/docs/current/ddl-constraints.html)兜底唯一性，[显式锁](https://www.postgresql.org/docs/current/explicit-locking.html)支持行锁/事务级advisory锁；本项目固定锁顺序、ON CONFLICT后再读取，死锁/serialization失败可在同operation且无外部效果时有限重试，超限返回可重试冲突，不无限循环。多实例不得依赖进程mutex防重；worker claim/lease/fence复用R01。

**冲突持久落点（R1/D08）**：同event异hash或同源条目等sequence异hash时，原`risk_observations`和已应用镜像不更新；把新输入的受控raw_ref、hash、原ingest引用及冲突种类同事务保存到`risk_ingest_conflicts`，返回引用该记录的quarantined结果。相同冲突重投返回原冲突引用，不重复派单/建cycle。不能保存冲突证据时给依赖/容量失败，不以quarantined假装已收妥。受信源仍须通过原source授权、body预算与流控；raw只在受控存储保留，普通receipt/日志不回原文。获权维护者按当前source/组织范围查询和记录处置；需要采纳新的业务内容时提交具名新观察/受审修正，不覆写旧事件或原风险历史。原始材料清理不能先删掉仍被open冲突唯一引用的证据，超量背压保留待送，不另造自动裁决平台。

## 5. 风险→工单及完成闭环

### 5.1 plan/apply和原子边界

`POST /risk-items/dispatch-plan`候选输入 `{items:[{risk_id,expected_revision}],target_type_code,target_org_id,append_ticket_id?,expected_ticket_revision?}`，风险最多20条为一个group；批向导可提交多group，首版候选总上限200条（服务端可收紧），超限要求分批。输出 `{plan_id,expires_at,groups:[{group_key,owner_user_id,target_definition_ref,items,blocking_reasons}],input_hash}`；仅对已可见风险给逐项原因。plan不是预约名额，也不锁住live授权。

`POST /risk-items/dispatch`及`batch-dispatch`应用 `{operation_id,plan_id,plan_hash}`；如沿生命周期旧请求附items/type/definition等冗余字段，必须与plan完全一致，否则400，不接受覆盖。固定分组键至少owner+风险管理org+目标org/type/definition+批准业务范围，不仅同owner。single需risk:manage和主锚/owner；bulk需主锚管理，沿B05不另要通用ticket:create、也不增派单角色。

同operation重投返回原group收据；同risk已有active link返回受权原票或冲突，不新建。apply在group/risk/ticket锁内验证当前权限、owner在职、类型发布/字段、目标映射、剩余名额和plan基线；**每group原子提交**票、links、风险状态、双域历史和必要协作intent。任一失败该group零新票/links，其他group可成功，返回逐risk `created/linked/already_linked/denied/invalid/conflict`。不支持第21项挤进旧票；重新分组需新预览。

批次 parent operation 先持久绑定 plan_hash/完整 group 清单；`group_operation_id=derive(parent_operation_id,plan_hash,group_key)`，候选 group receipt 唯一 `(parent_operation_id,group_key)`，保存固定输入hash、状态与原票/逐项结果。每group的票/links和其成功 receipt 同事务；parent汇总为 pending/partial/completed，不因A成功就宣称整批完成。A提交后进程中断，重投查A原receipt并只继续未完成B，仍复验当前权限/基线；已被拒绝或冲突的B不盲重试。修B参数/基线须新plan+新operation仅含待处理B，关联原parent，不可同号改参；parent读取仍按当前逐对象权限投影。

链接专用命令沿生命周期合同：`POST /risk-items/link-add`/`link-remove`收`{operation_id,risk_id,ticket_id,expected_risk_revision,expected_ticket_revision,expected_link_revision?,reason}`，追加还核同owner/分组/20项上限，移出回合法待处置并重算票聚合，返回双方revision/link receipt。不能通过普通票字段写risk_id绕唯一性，未获ticket读资格不泄露其状态或标题。

受限共享创建端口接收可信 `{operator_user_id,origin=risk_dispatch,creator_owner_ref,approved_target_mapping}`，不从外部body传。工单 `created_by=当前已核owner`、审计actor=真实操作者两者都留；普通创建用例继续按自身权限。`risk_owner`为桥管理字段，不能由普通表单/fetch/Agent改成自己。固定映射保留源风险ref/级别/系统/描述，取数建议可人工改，公式/受管字段照其独立规则。

票priority取active risks最高级别；`business_due_at`取最早风险deadline，与节点`sla_due_at`分列。追加/移出/改级/改期重算聚合，不覆盖风险原deadline，不用已移出risk继续触发提醒。owner全组同转可原子更新risk_owner及合法参与；只转部分risk则detach这些links、进入待交接/重新派单，不转走全票复核权。旧creator历史不改，旧读权按类型策略解释。

### 5.2 逐风险结论与review gate

`POST /risk-items/remediation-finish`：`{operation_id,ticket_id,expected_ticket_revision,results:[{link_id,expected_link_revision,expected_risk_revision,resolution,evidence_refs,note}]}` → `{ticket_ref,items:[{link_id,result:applied|pending_confirmation|conflict|rejected,risk_ref?,receipt_ref}],pending_effects}`。命令走工单共享转换/字段采集与风险validator；不伪造普通transition事件。

- gate=off走已发布 `in_progress→closed` 的合法完成边；gate=on走 `in_progress→pending_verify`，当前risk_owner再按合法边复核。缺边/字段/当前资格则预检或提交拒绝，不能靠临时auto边绕过。
- gate=on 的处理人 finish 此时只保存逐项整改 proposal/evidence，并将待复核项置 pending_verify；**不先把风险 closed 或 link resolved**。候选 `POST /risk-items/remediation-review` 收 `{operation_id,ticket_id,expected_ticket_revision,items:[{link_id,expected_link_revision,expected_risk_revision,decision:approve|reject,note,evidence_refs}]}`，当前 risk_owner 按票合法复核边及风险资格判定；approve 才落实已核修复结论，reject 要理由并回合法整改状态、保留原证据。接受类 proposal 仍须接受申请/非自确认流程，复核不能代替接受批准。部分复核先保存逐项结果，所有 active 项均有生效结论/合法detach才允许票终态。普通 risk.close、票 transition、自动边和任务回贴也必须经同一桥 validator，不能换入口绕过 review gate；紧急管理纠正另有明确权限/理由及历史，不默认让管理员跳过。
- 已修复逐项写措施、证据、复测人/时间；接受由当前owner本人申请且主锚确认后才resolved；处理人非owner只能提出接受建议。误报/重复须主锚资格，E10 merge未启用时不能“重复”直接吞并风险，保留proposal待处理；取消回assessed并释放该link。
- 票terminal前所有active links必须有已授权生效结论或合法detach；不能先全票closed再把尚待接受确认的risk标fixed。部分结果可保存为proposal/待处理，响应逐项反映；同库原子协调可直接落实有效项，使用异步消费的入口须明确pending。
- 管理员仍能走现有物理删票入口，桥必须同事务将其active links置detached（reason=ticket_deleted）、风险回合法待处置状态并保存原票引用/历史；接受/关闭风险不被删票反向重开，未具备该删除接缝前对受保护active关联拒删。SET NULL只用于保留关联墓碑，不允许留下active且无票的静默孤儿。

票终态和risk-resolution intent同事务。消费键 `(ticket_id,ticket_revision,link_id,resolution_version)`；迟到结果发现risk/link/owner revision变更即conflict，不覆盖新owner/周期。投影补发只修投影；不会重新执行复测或再建票。

复核首页待办来自当前有效risk_owner和pending_verify事实，同票去重；旧处理人/发起人/CC不因可看就算当前责任。当前owner无L1/参与可见资格时报配置阻断，不为统计改派或强授业务管理权。关联详情逐对象鉴权；“风险可看”不授任意票/附件全文。

## 6. D15通知与统一审计

每条风险业务变更、risk_events/处置历史及notify intent同事务；业务成功与消息发送分别记状态。通知复用既有消息端口，真实公司HTTP profile由消息包实现，不把UIP桩/站内inbox成功当企业消息送达。

intent固定 `{event_id,event_type,risk_ref,aggregate_revision,rule_ref,recipient_selector,org_anchor,template_ref,source_ref?,causation_id}`；收件人**发送时**解析当前owner/合法CC/批准负责人并重验资源及字段权限。旧actor或创建人不代当前owner。高级别上溯抄送一级部门负责人是已定能力；B08只绑定四级映射阈值、组织锚、多个负责人、时区/频次/静默例外。

解析失败分别 `recipient_unresolved/recipient_unauthorized`，进入可恢复配置待办，不广播全员/admin、不猜负责人；撤权抑止单独`suppressed`，超时/HTTP未知分别retry/unknown。站内、HTTP受理、送达、已读都按实际合同分开；无送达回执的服务最多显示“提供方已受理”。不因发送失败回滚已完成风险评估/派单。

去重键复用消息属主 `(source_event_id,rule_version,recipient_user_id,channel,semantic_key)`；D15的event_id/rule_revision/recipient_ref在适配边界显式映射为上述字段。template_revision固定绑定原delivery的payload，不因模板更新让旧event再次发送；需要业务补发时走受权新delivery并关联原项。每日提醒/汇总按风险、事件kind及配置时区的时隙建稳定key，重启不会重发同一时隙。通知文本仅编号/必要动作/受权深链，读取深链再次鉴权，不把敏感字段塞入IM；规则频次不是所有消息通用“exactly once”。

业务历史按风险/票当前可见性提供；跨模块审计检索/导出另需专门审计职责，沿[统一日志审计合同](../design/unified-observability-audit.md)。风险状态、接受、owner、派单/结论的领域历史与效果事务一致，必需历史写失败回滚本地命令；日志转发失败通过持久outbox补送。IAM两操作本地强审计失败回滚已由F017确认，仍按统一合同实施，不扩大为所有日志失败均回滚。

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

按来源/系统多值维度分组时每bucket按distinct risk_id计，响应注明non_additive，不能将bucket相加当总风险；低于授权字段资格的维度禁止筛选/分组。首版duration取自然经过秒，不含停表/工作日扣减。deadline自动计算以服务端首次assessment时间与所钉自然日policy为基准，显式日期必须经配置时区转换到具体Instant；输入不存在/歧义的本地时刻须明确选定偏移，不能靠浏览器时区漂移。

工单响应/处置/节点时长沿[工单运营合同](../design/ticket-operations-supplement.md)，不将风险MTTR当接单响应时间；历史缺准确closed/first_response时间标legacy_unknown/approximate，不用updated_at伪造精确值。SOC MTTD/专业MTTR由来源提供时间/定义，缺专业起点不算。

每日快照候选 `risk_daily_snapshots(snapshot_date,timezone,metric_version,scope_ref,dimension_key,counts,cutoff_at,source_watermark,quality,revision)`，唯一日期+时区+口径+scope+维度。**不可给普通用户展示全局历史聚合再用当前主表过滤**：固定全局/组织快照仅对当前明确获该历史统计范围的管理者开放；普通/owner动态细粒度范围无安全历史维度时仅查当前受权行或显示历史统计未提供。不可用当前行补算历史敞口假装快照。更细历史ACL/重放另立需求，不默加全量事件湖。

快照计划切点用配置时区当地日界，存 planned_cutoff；实际读取用单条SQL或**只读REPEATABLE READ事务**保存 snapshot_as_of，不能拿默认READ COMMITTED多SELECT冒称一批同快照。重跑同key幂等；若迟到且没有可还原日界的事件水位，显示gap或明确实际时刻的late_snapshot，不能把当前状态标为零点历史。保留可还原事实时才补算并标backfilled/其水位，不能拿今天状态填昨天。日维护调度沿唯一维护CronScheduler，声明单实例/多实例advisory锁，失败入既有cron状态/告警；用户周期报告归TR，不再起另一调度器。

D13-T `GET /risk-items/:id/timeline`只聚合该风险与获权ticket/TR/notification引用，分页`cursor,limit<=100`，默认时间窗/总条数受预算。每项 `{origin,source_event_ref,event_kind,occurred_at,recorded_at,actor_ref,authorized_resource_ref,summary}`，同源稳定排序 `(occurred_at,origin,event_ref)`，晚到记录可回看；跨源无共同快照时标as_of各水位，不假称全局线性顺序。故障返回明确partial/provider_unavailable且保留已授权部分；禁止为填满页面无限逐引用查询。普通业务时间线不等于跨域审计授权。

## 9. D14 SOC引用与本地协作的准确首包

职责/动作矩阵沿[SOC协作授权合同](../design/soc-collaboration-authorization.md)，不复制第二套权限。候选表 `incident_cases(id,provider,external_id null,source_revision,source_observed_at,source_state,local_status,scope_revision,revision,campaign_ref null)`；唯一非空`(provider,external_id)`；`case_roles/case_links/case_events`分别管岗位、受权引用及本地历史，引用SOC事实不能由本地manage覆盖。临时无SOC ID仅显式local_pending_reference，后续追加映射保留旧ID。

候选 `GET /incident-cases`、`GET /incident-cases/:id`、`GET /incident-cases/:id/timeline`；写用 `POST /incident-cases/collaboration-plan` 与 `/collaboration-apply`。apply输入 `{operation_id,plan_id,expected_case_revision,expected_scope_revision,source_gap_ack?:{source_revision,observed_at,reason}}`，来源gap开关由服务端，不由ack启用。输出 `{case_ref,local_task_refs,receipt_ref,pending_effects}`，通用400/403/404/409和receipt规则同前。

case协调者仅向批准类型/组织创建本地岗位票；岗位办理者只能提交本人当前岗位证据，协调者不能冒签他人结果；风险组成员不自动得incident职责。创建/关联/case历史和可靠intent同事务；跨对象核当前权限，未知提交结果查原receipt。`local_status=completed`仅代表本地协作完成，SOC专业状态单独只读显示。

B09已确认：SOC源陈旧时自动按旧专业状态新增派单held；当前授权有效的协调者可确认所见source_revision/时间并填理由后手工创建**本地协作**。来源恢复则追加新revision/对账，不自动关本地票。权限/UAA失效不可被source_gap_ack豁免；不授隔离/封禁权。战役仅轻分组+有界时间线/actor筛选/按活动导出，不落专业日志湖；五秒/两点击为待实测UX目标，不是已达到指标。[TheHive cases](https://docs.strangebee.com/thehive/user-guides/analyst-corner/cases/about-cases/)将任务、附件和共享范围纳入case，本项目只借用受控协作结构，专业分析和事件结论保留SOC写主。

## 10. E10风险Excel和跨指纹合并

P6.1出口完成条件核验/方案，不等待D08/D10产品完成才调查。风险Excel是已确认的条件需求；默认有限路线为模板下载→离线plan/逐行校验→授权用户确认→标准风险用例/feed→逐行receipt。逻辑记录 `import_run(run_id,file_hash,template_version,source_namespace,actor_ref,status)`，行唯一`(run_id,row_key)`；断点重跑相同身份，不把整表重新造risk。报告含成功/失败/冲突及安全原因，失败行可修正后新revision重提，不能用直接灌DB代替权限/来源幂等。文件模板/保留/量级须绑定真实样本，私有转换器不导出公司原文件。

跨指纹merge是条件候选，未启用不能用close duplicate偷偷实现。获准后plan显示master/slaves revision、owner、接受、active links、来源归属和可见集变化；管理者CAS apply，保留merged_into/主从历史与源alias。不同owner、有效接受或active ticket冲突先hold并给人工处置，不自动转走责任/读权。重复apply幂等；源再报沿批准alias归主族，历史generation与原风险编号不重写。撤销为新补偿命令，不能删历史假装未合并。[DefectDojo去重](https://docs.defectdojo.com/triage_findings/finding_deduplication/about_deduplication/)区分匹配范围和人工归并，可借鉴其保留主记录/回链，不能据此默认把所有跨源同标题视作同风险。

两者实际建设按获准子包另估、固定P6.E窗口；E10的2–4日只计调查裁决。无真实存量/跨指纹诉求则明确未激活与重开证据；取消已确认Excel效果仍须owner确认。

## 11. 最小UI、配置和运维恢复

风险列表沿统一表格/查询栏/筛选展开/操作位；详情分“评估与责任、整改票、接受/复测、来源与历史”。列表多选派单先预览、逐group结果只重试失败；按钮用capabilities反馈，后端仍授权。源severity与本地level并列；accepted和fixed分色分文案；归档不伪装已关闭新状态。表单保留用户输入，409提示刷新比较，不盲目覆盖；无owner可保存new但派单入口明确阻断。

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
| `RiskDispatch_TrustedCreator` | 风险管理者无通用建单权仍可向批准目标派；只读/任意目标/伪created_by拒绝；真实actor与owner分别审计 |
| `RiskHandover_PartialGroup` | 全组同转更新受管owner；部分转detach且其它risk复核权不变；旧creator历史不改；无合法继任保持待交接 |
| `RiskFinish_PerLinkResult` | gate on/off均字段校验，固定流程缺相应完成/复核边时发布或派单预检拒绝；fixed/accept/proposal/误报/duplicate/canceled分别正确且canceled回assessed非fixed；pending接受不能全票terminal；迟到旧link结果409不覆盖 |
| `RiskNotify_CurrentRecipient` | 入队后owner/CC撤权不发旧人；缺负责人/无权负责人各独立待办；HTTP超时unknown不当送达，业务保留成功 |
| `RiskRead_ConsistentScope` | 主/副/owner三支与L1/L3交叉；list/detail/total/export/stats/时间线/任务结果同范围，隐藏与不存在输出不可区分 |
| `RiskStats_MetricDefinitions` | 分母0=null、accepted不算fixed、缺时间不补零、月底/闰日/时区日界；无权历史快照不泄露，缺日不伪补 |
| `IncidentCase_SourceGapAndRoles` | B09人工确认可协作、自动held；缺理由/撤权/越目标拒绝；本地完成不改SOC终态、迟到source不回退新revision |
| `RiskImportMerge_ReplayAndConflicts` | 条件获准后验逐行断点/权限/报告与merge主从回链、active link/owner冲突、重复确认和来源再报归属 |

最短真实旅程：两个风险同owner→评估→批准目标同票→处理人填措施/证据→按配置复核→逐风险合法结论→风险/票/历史/消息对账一致；再加入撤权、响应丢失、consumer重启、部分转owner、消息失联四条故障链。连接TR/AL的任务效果另按共同互通模块验，不因本风险旅程绿推定全链绿。

依赖按子出口：D07查目录需要X02-Q及新资源护栏；D08消费已批准source规则/最小目录与持久接入（完整feed范围以总账为准）；D09/首条通知前有D15+R01；D10等D02字段采集/可信受限创建和本地事务守卫；D13-M不等通用BI，D13-T仅消费已交D11/D12引用；D14等D10/D13但不先建专业SOC。完整UAA-P/R、AI、全站视觉推广不阻本地风险闭环。

当前不重新询问已确认的职责/N:1/接受制衡/设备/本地先行原则。**启用前需绑定**：

1. 首批真实风险类型/字段/目录、目标工单类型/组织/owner名单及review_gate：例如两个risk同owner但目标组织不同，必须分组；固定流程须显式配置gate=on的`in_progress→pending_verify`及所需复核边，gate=off须显式配置`in_progress→closed`合法完成边。发布/派单预检缺边就拒绝该画像，不让运行时才发现无完成路径；管理员对预览样本确认而非逐字段技术决策。
2. B08真实通知实参：公司源五级如何映射四档、从哪一业务部门上溯、一个一级部门多负责人是否都须获权接收、汇总时区/时刻及静默例外；未齐只启合成规则，不假送达。
3. 来源新鲜度/稳定键/序号/撤回及观察时间真实语义：例如扫描重导报告只是更新时间变更，不能据此判已修复后回归。由提供方合同验证，不要求公司资料外传。
4. E10 P6.1出口：真实Excel模板/存量量级、是否已有迁移工具，以及一个跨指纹并单实例；按固定窗口裁决并给独立子估算。来源不具备不阻人工风险用例。
5. 正式部署前的数据类别保留和审计人员范围，沿统一发布门绑定；180/365默认不是对公司制度的推定。技术锁/typed命名由实现审查处理，不新增owner审批流程。

## 13. 公共reference v1：两风险的完整纵切

本节的序列/观察投影已有[可序列化向量及Schema](../design/examples/capability-reference/module-profile-vectors.json)，固定摘要见[参考manifest](../design/examples/capability-reference/module-reference-manifest.json)。它只验选定形状；本节角色/派单/关闭回归的时序和当前权限仍须产品独立预期，不冒称10个跨模块样例已执行完整风险旅程。

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

## 变更记录

补充具名TDD：`TestDispatchBatch_GroupCommittedThenCrash`断言A提交/kill/重投只续B且parent不能提前completed；`TestRiskReviewGate_AllMutationEntrypoints`覆盖finish/close/transition/自动边均不能绕owner复核；`TestRiskTicketDelete_AtomicDetachAndRace`覆盖未接删除桥时拒删、接线后tombstone/风险/link同事务、与finish/owner交接竞争及已accepted/closed风险不反向重开；`TestRiskSnapshot_LateRunAndIsolation`覆盖多事务并写下同批快照一致、迟跑不假装日界和缺日gap。这些仍是待实现回归规格，未运行产品断言。

- 2026-11-14：按当前代码/本地首版边界补D07–D10/D13–D15/E10详细接口、逻辑DDL与唯一约束、跨域事务/权限、正常异常/并发恢复、薄UI和具名TDD规格；纠正R1指纹全表唯一待定、R2误报独立终态、R3泛终态直关及R4历史聚合越权风险。未改业务代码/迁移/估算，未宣称新增用例已执行。
