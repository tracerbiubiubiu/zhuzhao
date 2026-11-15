# Phase6任务、ActiveList与消息模块详细设计

日期：2026-11-14。状态：**待实施的模块设计候选**，不表示下面的表、路由、协议或故障测试已经实现。范围为C03/R01/D12、CE01/CE02的P6切片、D11/D15消息侧；AL写与变化源承接CE03/CE04的首版本地子范围。范围与价格继续以[总账](../review/planning-backlog.json)、[本地核心分项](../review/local-core-release-estimate.json)为准，本页不新增一份总价。

本页把[六条接缝](module-design-handoff.md)展开到模块输入、事务、失败出口和具名测试。[跨仓执行合同](../design/cross-repo-execution-contract.md)拥有回调v2/occurrence/R01语义；[互通Schema](../design/capability-interconnect-detail.schema.json)拥有command、receipt、event、subscription、admissionClaims和alExecutionRequest的已有字段。本页细化首版本地画像，**不把较宽的多消费者change-feed、Import/删除/恢复源或完整I03控制台变成本地首发前置**。未来扩大支持入口时，仍须实现宽合同对应的源事实与恢复能力。

## 1. 当前代码、变化及分期

实际代码锚点均按文件与符号定位；兄弟仓路径相对于各自仓根：

- 主仓`internal/service/ticket/actions_builtin.go`的`SpawnTaskAction.Execute`调用Submit且丢弃任务响应；`internal/service/taskrunner_service.go`的`TaskrunnerService.Submit/Trigger`先调用TR再记本地凭证。目标是先持久意图/快照，保存原节点与operation，再提交TR，消除“票显示已安排但无可靠交接”的窗口。
- 主仓`internal/service/jobs_callback.go`的`JobsCallbackService.Execute`仍将running重复映射成幂等受理，成功后的记账错误仍回200；`JobSubmissionRepo.ClaimCallbackRow`用claimed_at作旧栅栏。目标用持久效果receipt、单调fence和明确in_progress/unknown替代“收到就完成”。
- TR`internal/service/submit.go`的`Service.Submit`已有DB-first及queued=0恢复，但遇已存在task_id不比较完整输入；`cron.go`的`Loop.FireDue`随机起任务后另行推进next_run；`internal/callback/client.go`的`Client.Do`按任意2xx判完成。目标复用队列/账本，补同号异参冲突、稳定occurrence及结构化完成，不能再造第二队列。
- AL`internal/service/data_service.go`的`DataService.Update`已有读合并、校验、锁行及version CAS；`internal/repository/data.go`的`GetDocForUpdate/UpdateDocData`是复用点。当前没有本页的operation效果receipt、授权准入和持久变化源。已有UpdateInput.Version映射目标expected_revision，Import仍是全量replace，不能拿它模拟patch。
- 主仓`internal/pkg/channelsender/port.go`的`ChannelSender`已有渠道端口；`UIPStubSender.Send`只具开发桩/未实现反馈。`ticket.MessageDispatcher.Dispatch`仍经Hook、进程内节流并在发送后写sent/failed日志，不能证明可靠应发实发。目标保留模板/偏好/站内通知，增加公共HTTP实现和私有字段映射，按实际提供方合同区分受理与送达。

交付阶梯：S先形成同一typed应用入口、当前授权和输入校验；D补稳定操作、事务意图和回调完成；本地核心合上一个固定工单动作、一个AL类型/字段组、一条只读后继和一个HTTP渠道。D11完整规则编辑/聚合、完整I03三策略恢复控制台、宽AL源入口与多消费者feed后继交付；它们不被本页取消，也不以本页存在冒称已完成。

## 2. 写主、类型与共同调用路径

核心独占工单/风险事实、授权、节点运行和结果投影；TR独占调度、投递、attempt和执行账本；AL独占文档、generation/revision、该写的效果receipt与源事实；消息用例独占应发/发送状态，私有HTTP adapter只做公司协议转换。各仓不跨库写，不把领域表放入utils。

HTTP、按钮、工单、计划与订阅先经可信ContextFactory，再进入同一已登记application port，最后进入原领域service/repository。内部调用直接走typed接口，不绕本机HTTP、不重复开启operation或另扣逻辑预算；外部协议adapter负责序列化和认证。短查询/本库短写同步，外部长调用及不可丢后继持久异步，无万能`/invoke`、任意SQL/URL或脚本入口。

候选类型只引用已有Schema，不复制第二套command/receipt：

```text
CapabilityCommand = schema#/definitions/command
CapabilityReceipt = schema#/definitions/receipt
ALExecutionRequest = schema#/definitions/alExecutionRequest
SourceFact = schema#/definitions/event
ALReadInput/ALReadResult = schema中activelist.document.get@1的本批补全候选

ApplicationPort.Execute(ctx: TrustedContext, command: CapabilityCommand)
  -> CapabilityReceipt | DomainError
OperationQueries.Get(ctx: TrustedContext, operation_id: OperationID)
  -> CapabilityReceipt | NotFound
ALCommands.Update(ctx: TrustedContext, command: CapabilityCommand)
  -> CapabilityReceipt | DomainError
ResultProjection.Apply(ctx: TrustedServiceContext,
  command_binding: {operation_id, ticket_id, node_run_id, definition_hash},
  receipt: CapabilityReceipt) -> {applied: boolean, current_receipt_revision: UIntString}
```

`TrustedContext`只能由已认证入口或持久受限授权构造；外部JSON中的principal只作绑定输入，不能自行授信。`OperationID`等格式、整数以十进制字符串传输、时间和大小上限沿原Schema。`operation_id`是稳定效果身份，`invocation_id`是逻辑调用，`event.source+id`是已发生事实，`task_id`是TR稳定任务，`occurrence_id`是计划时隙，`delivery_id/execution_id/attempt/fence`区分每次投递与认领；request_id只用于追踪。

同号重放先验当前读资格，再比较规范输入hash、主体/授权与原绑定；同号异参409，原记录不可读则not_found且不新增效果。客户端传hash不作真值。先按Schema严格解码，再校验跨字段相等、能力/版本、目标字段白名单、期限和当前权限；shape通过不证明语义或授权。仍用现有errcode映射，实施时登记符号码，不在设计里抢数字编号：invalid_input=400、未认证=401、无功能/动作资格=403、不可见对象=404、版本/同号异参=409、准入限额=429、依赖暂不可用=503；可能已经生效的网络错误必须显示unknown，不能仅靠HTTP500表示“可重做”。

对外保留具名GET/POST产品端点，POST业务标识入body。候选`GET /api/v1/operations/receipt?operation_id=...`经当前资源读权；服务间固定`POST /internal/jobs/status`把task/operation放签名body。旧AL网关wildcard必须转换到同一AL应用用例；监控类型不能留下绕开行/字段授权和源事实的裸proxy写入口。已有AL wire方法由adapter转换，不借本轮设计直接改变已发布API。

## 3. 候选逻辑数据、约束与事务

以下是逻辑DDL，实施时可复用/增列或合表；必须保留约束语义。迁移up/down成对，编号在实施当天核迁移地图，不预占。所有ID持久化类型须与当前模型一致；下列`UIntString`仅为wire表达。

- 核心`capability_operations`：operation_id主键，invocation_id、capability/version、input_hash、主体/authorization引用、target、definition_hash、causality、状态及单调receipt_revision、effect_ref/error、created/expires/reconcile_due。operation绑定不可变；状态更新CAS旧revision。索引`(status,reconcile_due,operation_id)`供有界对账，不扫描全部业务表猜效果。
- 核心`ticket_node_runs`：run_id主键，ticket_id、发布definition_hash、node_id/动作序号、node_generation、operation_id、结果策略；node_generation绑定工单详设**每票全局单调visit_no**，不是每节点各自从1计数，避免A首访1/action0与B首访1/action0碰撞。`UNIQUE(ticket_id,node_generation,action_index)`及operation_id唯一。回退重访产生新visit_no，迟到结果同时核原node_id/definition_hash/visit_no，不套新访问。`ticket_task_results`按operation_id唯一保存已核receipt_revision及有界摘要/artifact引用；禁止结果反写新节点。
- 核心`business_intents/intent_deliveries`复用R01：intent_id主键；源事件/业务版本/动作序号与intent_type唯一，投递`UNIQUE(intent_id,consumer_id)`。lease_generation递增，lease_until用DB时钟，attempt/next_attempt/deadline、result_receipt齐全；`(state,next_attempt_at,intent_id)`索引只认领可投递项。回放不改变intent/operation。
- TR扩现有job_runs/执行表：task_id唯一，operation_id与owner_service唯一绑定；固定input_hash、action/definition revision、business_outcome/accounting_state/receipt_id。`schedule_occurrences`按`(job_id,schedule_generation,scheduled_for_utc)`唯一，记录冻结定义与task_id；task插入与next_run推进同本库事务。PG用行锁/条件更新，SQLite用其实际单写事务，不照搬PG语法。
- AL`operation_receipts`：`UNIQUE(owner_service,operation_id)`，保存input_hash、主体/授权绑定、目标generation、effect/noop、document_revision和receipt。原operation同号重放先看已提交receipt，再决定是否检查旧expected_revision，不能重复成功后因旧CAS返回冲突。
- AL`change_streams`：stream_id唯一，type_name/generation、committed_cursor、状态和单一consumer_id。`change_outbox`：event_id主键，`UNIQUE(stream_id,cursor)`及稳定source/id唯一，规范payload/hash、递送状态/fence/next_attempt。每监控类型源写先锁stream head，再锁文档，源事实、数据和receipt同tx；不直接用nextval当“之前都已提交”的水位。
- 核心`source_inbox`：`UNIQUE(source,event_id)`，另存stream/cursor/payload_hash、received_at和stage_commit_ref。`subscription_versions`冻结规则/目标/映射/grant及start_after/end_at；`subscription_actions`以`UNIQUE(source,event_id,subscription_id,rule_version,action_index)`去重，关联派生operation与held/suppression原因。持久inbox接收不等于动作成功。
- 消息`notification_deliveries`：以`(source,source_event_id,rule_id,rule_version,recipient_user_id,channel,semantic_key)`唯一；source与event_id一起定位原事实，rule_id与version定位规则，不依赖semantic_key暗含这些身份。保存template/version、recipient_spec/version、当前目标映射引用、应发/实发状态及provider_receipt；发送attempt单独记录，不能靠发送后查message_log去重。已有message_log保留为兼容读投影，sent旧行标legacy语义，不补造真实送达凭证。重复同来源/事实/规则不重发，不同来源同id或不同规则同version不互吞；显式replacement保留原delivery及独立受控替代身份。

锁序只在各自本库成立：核心业务提交遵共同合同的授权守卫→虚拟组→风险（ID升序）→工单（ID升序）→关联/node顺序，只取得本次实际需要的锁，最后条件写operation/执行fence、结果/intent。worker/poller的claim属于**先行独立短事务**，提交后才进入业务事务，不带claim行锁去拿跨域锁或等网络；效果事务仍须锁内复验当前generation/权限并条件写执行行，旧fence不符整笔回滚，不能把一次claim当永久执行资格。认领方与效果事务最终竞争同执行行，发现持久终态receipt不得另做效果。AL按本库既有receipt reservation→metadata/stream head→document锁序。网络调用一律在业务锁外，返回后以generation CAS记账。并发撤权与本地效果提交竞争相同授权守卫；跨AL网络仅承诺发送前重验＋短时准入的有界延迟，不能宣称分布式瞬时撤权。

存量兼容：旧job_submissions/status/message_log列继续可读；无法证明的历史成功只标legacy_observed。切换按consumer停旧Hook/旧直接spawn路径后启新意图，影子阶段只产不投，避免双发。回退先停新副作用，兼容reader/对账worker保留到在途收敛；不down删除unknown/receipt或正式历史，不让不理解新状态的旧worker接手自动执行。

## 4. 工单节点→TR→结果与AL写

### 4.1 提交与执行

发布配置必须绑定capability_id/contract_version、definition_hash、允许输入来源、目标字段和结果展示策略。首版固定受审流程，参数仅有界标量与明确映射，不开放任意表达式。提交时L1验执行功能，L2验当前票/AL行字段，L3验节点、处理职责、资源状态和版本；配置者不因此获得执行资格。

工单事务写node_run、operation和R01意图；任一失败整笔不显示“任务已安排”。dispatcher同一operation提交稳定task_id；TR受理只置queued，响应丢失先查原ID，不能换号。worker必须成功取得持久claim后才能发效果，DB缺行/claim故障零调用。队列不存用户JWT/RT/密钥，只存授权引用和批准输入；执行前重新恢复当前主体/资格。

首版后台计划/订阅使用明确授予该固定能力及行字段范围的受限服务主体，不把创建者登录权当永久无人值守许可，不开放任意代人grant平台。权限不具备时保留held而不执行；真人一次确认的动作还必须在其明确有效期内。长期委托、通用授权台和MCP写仍归I02后继。

主仓callback v2沿[跨仓合同](../design/cross-repo-execution-contract.md)：200只给持久succeeded receipt；202仅在已协商v2表达in_progress/outcome_unknown，TR不得按完成；v1遇running/unknown返回503而非202。core/TR组合manifest拒绝不支持receipt/status的严格模式，当前测试环境可协调换新，无需引入永久v1桥。网络/记账失败后先query原receipt，failed_no_effect必须有确认无效果证据；租约超时不证明旧进程已停止。

### 4.2 AL有条件写与结果回贴

`activelist.document.update@1`沿既有alUpdateInput：type_name、document_id、generation、expected_revision及1–32个白名单标量patch；target/params中的目标必须一致。core提交前按当前角色/数据范围/字段策略验权，签发Schema的admissionClaims（purpose/audience、operation/hash、授权引用/版本、目标/字段、fence和时限），放入现有AK/SK签名body，不新建OAuth服务器。claims最长30秒且不超过命令expires_at；过期后重新验证当前资格才能重新签发，不自行延长旧授权。

AL验证可信服务、claims与command绑定及字段子集后进入同库事务；检查generation、CAS、schema、唯一值，保存数据/源事实/receipt。合法同值patch可回noop receipt，不增加revision/cursor、不触发后继。代际变化/行版本冲突409且无效果，不能自动读取新version覆盖用户修改。TR绝不直写AL库。

发送后超时进入unknown，按原operation查询AL效果收据；查到succeeded只补投影。查询空结果若不能证明旧请求已不可能迟到生效，继续unknown；不基于一次not_found盲重发非幂等动作。新worker持新fence只能更新自己认领阶段，旧writer无权覆盖新状态；AL效果唯一键及版本条件才是效果防重底线，租约不是exactly-once。

core结果投影验证已登记来源、operation/input_hash、原ticket/node_run/definition_hash，CAS receipt_revision后保存。投影失败只重放投影，不重做已成功效果；原节点已取消/流程换版，迟到结果只挂原运行视图及历史。首版仅展示有界摘要/artifact引用，**不自动改票状态/关风险**；后续字段写回另验字段ACL/CAS和发布映射。artifact下载每次验当前资源读权，临时URL不作长期权限凭证。

## 5. AL变化→持久收件→获权TR后继

### 5.1 首版源写与push协议

首版只选择一个监控类型的普通insert/update。该类型的import/replace、delete/restore、破坏性schema演进以及任何绕过用例的写入口由服务端明确拒绝并说明原因；其它未监控类型原有能力保留。监控类型的注册/停止监控须检查订阅/在途操作，不能悄然清掉stream或generation。以后放开入口，先补其事实、去重、批量预算和恢复合同。

使用原Schema的SourceFact，只接受`al.document.created.v1/al.document.updated.v1`；source/id稳定，event-time projection只含发布白名单字段，不能携全行秘密。单一源outbox向固定core地址push，**不先实现较宽的分页feed API**。新增局部传输DTO如下，不改变SourceFact字段，也不声称现有changeFeedAck已是push回执：

```text
PushDeliveryV1 = {schema_version:"1", delivery_id:DeliveryID,
  consumer_id:ConsumerID, event:SourceFact}
PushAckV1 = {schema_version:"1", delivery_id:DeliveryID,
  source:string, event_id:EventID, payload_sha256:SHA256,
  stage_commit_ref:CommitRef, state:"durably_staged"}
```

候选固定路由`POST /internal/activelist/changes/push`，双方按既有服务签名认证、固定consumer/source范围和body预算；不允许客户端给任意回调URL。200只在inbox事务持久提交后给上述ACK；相同source/id/hash返回原stage_commit_ref，不重复后继；同号异hash409隔离，签名/来源不符401/403。DB不可用503，收件容量超限429/503并不ACK；源保留待送。AL只接受与本次delivery/event/hash匹配的ACK，CAS自己的delivery fence，不能用任意2xx或“已收到”日志清outbox。

每流首版按cursor顺序发送，最多一个在途事件；等待ACK不持source业务锁。未确认事件阻塞该流后续递送并告警，不阻塞其它流；重投原事实，不靠当前表回查拼历史。push消费者按事件唯一键收件，不用“已收最大cursor”跳过迟到事实；源stream head仅用于可证明的规则切换边界和检测缺口，不拿序列号假设完整性。传输ACK可使临时副本归档，但不能删除业务历史/审计/永久执行证据。

### 5.2 订阅边界与只读后继

启用/换版以AL同一stream head锁串行取得边界W，配置在core持久保存start_after/end_at；准备期间先暂停该规则执行，inbox可继续收件。只有新的规则版本及W落库成功才激活；失败不恢复成半新半旧配置。旧版本覆盖≤W，新版覆盖>W，已有stage未执行动作按原规则held_pending，不静默改新参数。首版一个规则/一个动作，不以实现Schema允许的16流/4动作上限为首发门。

事件过滤只读metadata和明确event_values，业务读取当前AL值必须另外记录queried_at/document_revision，不能把当前值冒充事件时值。规则发布核字段存在、标量类型、映射目标Schema及grant可覆盖范围；event_values不含某字段时不能拿它作历史条件，亦不把“缺字段”解释成null命中。

首版真实后继选“读取发生变化的AL条目并生成受控复核结果”，**独立于工单存在**。复用现有库存的`activelist.document.get@1`（AL `DataService.Get`），经同一当前授权只读用例执行，运行详情显示当前数据与查询时间；若有合法原票运行关联，可另外投影引用，不能强制每条AL记录先建工单。该任务不会写AL或推进工单状态，**不是刷新列表/只发通知冒充AL→TR**。规则服务主体须具有该AL类型/行/字段读取和后继执行资格，撤销则held。

现互通Schema的command/receipt/subscription动作枚举原仅报告生成与AL.update，本批必须同步补该既有get能力的准确输入/结果与样例，不能绕过enum，更不另发明query别名。候选有类型合同：

```text
ALReadInput = {type_name, document_id, generation, fields: FieldName[1..32],
  source_event_id?: EventID}
ALReadResult = {observation_owner:"zhuzhao", data_owner:"activelist",
  type_name, document_id, queried_at: UTCInstant, generation: UIntString,
  document_revision: UIntString, values: AuthorizedScalarMap,
  source_event_id?: EventID, source_event_revision?: UIntString}
```

目标与params一致；fields去重、存在于schema且由当前授权允许。source_event_id若有必须用不可变subscription/operation绑定的source组成(source,id)核该条持久inbox及目标，不能跨来源全局搜event_id取第一条；事件revision只来自原事实。values只含获准标量、总量≤32KiB，不读全行再把秘密写日志；generation变化则held并提示核配置，不悄然把旧任务套入新类型世代。查询结果是queried_at时的当前document_revision，source_event_revision仅说明触发时版本，两者可不同，绝不冒称历史快照。

源记录存在且所选可选字段均未设置时，`values={}`合法；缺键表示未设置，显式null保留，不能为满足Schema把缺键伪造为null。记录不存在仍404，无权仍拒绝/held，依赖失败不能返回空成功；空投影不是权限过滤的替代。

成功由core保存同operation的只读观察结果，receipt必须带query_result且**禁止effect**；observation_owner=zhuzhao表示本地观察凭证写主，data_owner=activelist表示源数据写主，不要求AL增加只读写效果receipt，也不增加AL数据revision/outbox。其它原写/报告成功receipt仍必须effect、禁止query_result，不能借get分支放宽。重复同operation先重验当前行/字段读权后返回原结果及原queried_at，不偷偷更新成新查询；需要刷新时显式新operation关联原运行。撤权后旧结果不再可读。对get增加的schema分支、语义校验及无票用例是原AD实际只读后继的必要接线，归已价AD/CE迁移切片；不顺手扩成list/自由query/MCP全量导出。

get订阅动作采用专用target_from_event=true：只固定已登记的AL类型/generation与owner/resource_type，document_id从本条持久SourceFact取得，逐行执行当前授权，不能静态绑第一行后拿它套全部事件。静态resource_id与target_from_event混用拒绝；input_mapping仅支持type_name固定值、document_id/generation/source_event_id的指定event pointer及fields字段名常量数组。数组只在get的fields分支有效，不扩宽原update映射；错误类型/世代/事件目标、不获权行、伪字段数组均拒绝或held。

派生operation按既有`op_evt_`＋SHA256(source/event_id/subscription_id/rule_version/action_index)生成，action唯一键、operation与R01意图同核心事务。可信root与预算从源operation继承，不能每跳开新root；同rule同root拒重入，持久记录suppressed/held原因。首版只读后继已有结构性断环；未来确需回写时仍须因果/深度/扇出预算和明确触发字段策略。

暂停停止新的动作准入，继续可靠收件；恢复先预览积压范围、规则版本、当前grant和数量。超限保留余项held，获授权者可扩大受控范围或明确跳过并留记录，不能自动清空。generation改变、源事实丢失或保留窗缺口进入source_resync_required，禁止跳latest掩盖丢事实。损坏单条隔离并显示阻塞位置；人工跳过必须保留原记录、理由及对实际业务影响，不直接改游标。

## 6. 消息：公共HTTP能力与一条私有映射

### 6.1 应发、收件人及状态

业务事务写事实及消息意图，消息用例按固定规则/模板版本生成逐收件人的应发项；业务提交成功与发送成功分开。R01源失败则本批要求不可丢的业务命令失败；提交后提供方失联只影响消息状态，不回滚已完成工单/风险事实。原best_effort Hook只服务未迁移低一致性通知，不能同时执行本次durable消费者。

收件人spec固定为owner、owner_and_cc、业务明确的reviewer或受权fixed集合，按去重后的本地user_id处理。发送前重新解析当前职责、账号有效性、当前对象/字段可见性和批准的公司收件人映射；不能用CC获得处理权，不能因主管级别将无权对象正文群发。风险高级别上溯按D15/B08矩阵；无合法组织锚点/负责人或映射不唯一，记recipient_unresolved/unauthorized并待处置，不猜人、不广播。

每条delivery的recipient_user_id不可变。发送前重解析是在验证这条目标是否仍合适，**不是把A的旧delivery改寄B**：A→B改派后，尚未尝试发送给A的旧项记suppressed/recipient_changed；B由新业务事件产生新delivery，或由受权的显式replacement操作按稳定替代ID生成并关联原项/理由。已进入sending/unknown/accepted的A项保留原收件人及事实，按原项查询收敛，不能伪装成从未发送或改成B。

第一次实际HTTP发送前持久冻结规范recipient映射、subject/body及template/input版本、payload_hash、provider幂等键和固定渠道profile。发送前权限不足则抑止尚未发送项；发送后unknown只查询原项或在提供方幂等合同下**同号同payload**重投，不因模板、AL数据或owner变化重新渲染/换人。旧recipient失去资格时停止新发送与旧正文回显，但保留可能已有外部效果的待核状态；需要通知B或使用新内容是独立的新业务/替代delivery，不能借重试改变原操作。规范payload持久存储遵最小化与读权，普通日志只记hash/关联，不记正文秘密。

字段投影按每收件人当前资格渲染；缺少必要模板变量为failed_configuration，不发残缺误导消息。偏好/免打扰/节流对适用通知生效，抑制原因持久可查；哪些安全通知可强制送达须有已批准类别，不能因“风险”绕过全部偏好。A→B→A改派以assignment/event版本区别业务事实，同一版本重复不重发，不能用60秒进程节流吞第二次真实改派。配置预览不发送，测试发送只给当前受权操作者/批准测试目标并明确标测试。

状态分别为pending、suppressed、sending、accepted、delivered、failed、unknown：accepted仅证明提供方受理，delivered只在真实协议提供可核送达证明时使用。只提供HTTP受理而无送达查询/回调时，界面停在accepted并说明“渠道不提供送达回执”，不捏造delivered。站内通知落库是站内可读事实，与外部送达分开；通知失败不等于业务处置失败。

### 6.2 HTTP端口与失败分类

保留当前`ChannelSender.Send(ctx, Message) error`的旧适配，不直接破坏现消费者；新可靠路径增加独立有类型扩展端口，旧nil error不能被桥接成真实送达：

```text
SendRequestV1 = {delivery_id, idempotency_key, channel_profile_ref,
  recipient_ref, template_version, subject, body, business_ref,
  source_event_ref:{source,event_id}, rule_id, rule_version, operation_id, expires_at}
SendReceiptV1 = {delivery_id, state:"accepted"|"delivered"|"rejected"|"unknown",
  provider_receipt_ref?, observed_at, retry_class, safe_reason_code?}
HTTPMessagePort.Send(ctx, SendRequestV1) -> SendReceiptV1 | TransportError
HTTPMessagePort.Query(ctx, delivery_id, provider_receipt_ref?) -> SendReceiptV1 | Unsupported
```

HTTPMessagePort是公共业务端口；固定base URL、鉴权注入、公司收件人/字段/状态映射由私有adapter/profile实现，凭据只用secret引用，不入业务参数/日志。公共实现包含TLS校验、固定出站目标/允许重定向策略（默认拒绝跨目标）、超时、响应大小和schema验证、受控错误脱敏、mock profile；不让工单参数指定URL/headers。内网可以只更换adapter/config，核心不导入公司SDK/internal model。

已确认未受理的429/503按Retry-After与有界退避同delivery重投；鉴权错误停止并告警，字段/收件人/模板错误终止该项待修；超时/连接中断发生在可能受理之后则unknown，优先查询或用提供方真实幂等键同号重试。若渠道既无幂等也无查询，默认暂停unknown并提示人工核实，不盲重发。补发是受权的新delivery且显式关联原项/重复风险，不能偷偷换key；重复/乱序送达回执按provider事件身份和状态CAS去重，不允许迟到accepted覆盖delivered。

公共mock覆盖受理/明确拒绝/超时已生效/无查询/重复乱序回执；真实渠道激活必须另验公司鉴权、TLS/放通、收件人映射、限流、错误/幂等/回执。当前真实合同和联调日期未定，不填公司地址/名字，不把桩的sent当内网已送达。

## 7. 定时、多实例、流控与运行边界

业务定时只由TR负责；核心现有CronScheduler只承载本库有界维护/intent轮询，不接另一套业务cron。C03先交稳定occurrence与原子推进；三种错失策略的完整episode、开始准入、恢复操作/UI归[调度恢复详设](../design/schedule-recovery-policy.md)I03。首版选定计划必须显式保存实际支持的策略；未实现选项服务端拒绝而非表单能选却无行为。重复效果对账与漏建时隙补发永远分开；同slot重投不换task/operation，未知效果不因回补重新产生。

技术profile复用原合同候选，不另外发明互相冲突的全局值：R01 dispatcher批50、并发2、网络10秒、delivery lease120秒/心跳30秒；能力attempt默认30秒，登记能力execution lease30秒/10秒续约；AL准入最长30秒；消息发送沿现有5秒初值。C03 legacy callback执行默认120秒/30秒不是另一条必建租约：同一登记能力callback只用所选profile的一套execution claim/fence，run钉版，不能同时叠加legacy claim和能力worker claim。**投递lease、唯一执行lease和准入期限分层，不能拿intent lease当AL授权期限。** 默认重试5秒起、抖动、最高5分钟/最多20次且不超过24小时投递期限；业务命令expires_at更早时以其为准，过期held不新增效果。参数是待压测的初值，不是已实测容量/SLA。

同operation只记一次逻辑准入，每个真实attempt仍记provider start/并发和费用。**首版单实例/单订阅/单动作**先复用队列有界worker、每提供方限流、总deadline和积压硬门，不建设service/grant/root多维预算引擎；真实提供方限额须实际执行，限额依赖不可证明时受影响自动写held，不猜可用配额。一个provider断网不能耗尽主站请求池；claim后无令牌就持久延后并释放worker，不把排队等令牌记业务失败重试。多worker/持续grant或真实扇出启用时，再以共享Redis原子配额验其新增维度与抢占；不能把单进程semaphore宣称多实例安全。

共同合同的深度8/步骤64/扇出16/900秒是后续允许profile上界；**首版只允许一次订阅派生（rule_path最多1项）和一个只读后继**，保留同一root、真实parent/depth及事件去重，不能把协议depth清零或将整个root限成≤1。这样工单→TR→AL写→AL事件→TR只读后继仍合法；固定批准链静态有界，不为尚未开放的DAG建设通用预算执行器。inbox/outbox/每订阅积压和总字节仍有硬上限及提前报警。outbox空间不足且不能安全归档时，拒绝并回滚新的监控AL写；不能数据提交后丢事件。core收件压力不ACK，源继续保留；容量硬门不能以丢消息维持假健康。

支持多实例的claim、fence、幂等唯一键、quota和授权守卫必须跨实例共享，禁止内存互斥当唯一保证。首版本地可运行单副本；启两副本前必须以两个真实进程/实例验抢占、停机、旧fence和共享限流，不因单机goroutine测试绿宣称HA。优雅关闭停止新claim，有限drain，剩余由持久记录/租约找回；不能把SIGKILL等价于取消效果。

## 8. 权限、薄UI、日志与运维接线

最小权限矩阵按动作检查三层：普通办理人可看原票任务及允许结果，不获AL全库/任务全量读权；类型管理员只在批准类型/组织范围管理；CC仅获允许的查看/评论，不附执行/写回；后台服务按固定能力/行字段范围；运维对账另具操作权限和资源scope，不能用service AK读取所有receipt。列表、详情、total、artifact、日志片段、批量、导出和消息预览复用相同数据过滤，查到task_id不是授权。

薄UI随模块交付：工单任务卡显示queued/running/结果/unknown/held及最后观察时间；同页分开展示业务状态、执行状态和消息状态。AL显示版本冲突及监控写限制，订阅页最小映射/暂停/积压预览；消息列表显示应发/抑制/受理/送达证据/待核实。unknown默认“查询原操作”，不放普通重跑按钮；受权修复投影与重新执行是两种操作。无权对象不显示标题、摘要、数量或错误正文；界面术语、按钮位置、空态及颜色沿U02组件，失败不只藏日志。

对账命令/管理入口复用R01的inspect/query/confirm_succeeded/confirm_no_effect/retry/cancel；POST含目标、expected_revision、理由、证据引用和幂等键。确认无效果必须提供能排除旧请求迟到生效的证据；不能手改SQL、清队列、换operation或点击“成功”伪造业务。补偿是新受权operation，与原操作关联，非跨服务数据库回滚。

配置启动fail-fast核协议/动作版本、固定目标、服务授权、能力白名单、监控类型受支持入口、生产secret、receipt查询及保留窗。未知receipt/未终结operation不按普通TTL清除；任务执行历史/必要执行证据沿既有永久约定，源投递临时副本与普通运行日志可按各自批准策略归档，不能借ACK/日志轮转删永久证据。备份恢复要包含operation/receipt、outbox/inbox、规则版本及在途关联，而非仅业务表。

日志/审计复用[统一合同](../design/unified-observability-audit.md)：request→operation→intent→TR task/occurrence/attempt→AL effect/event→消息delivery可追溯；事实、拒绝、尝试及记账分别记录，业务历史与专门审计授权分开。秘密、公司原始报文、完整文档/任意error片段不入普通日志；trace采样不采掉receipt/事实。指标至少有pending最老年龄、unknown/held/过期量、投递/回调/投影延迟、CAS冲突、撤权拒绝、source gap、quota等待、应发/抑制/accepted/delivered/failed、各provider故障；标签不含用户/票号等高基数标识。

### 8.1 首个AL类型的用户读取合同

任务get/update接好权限不代表原列表安全。首版所选受保护类型的list、筛选、metadata和确需的count也必须走同一受权应用用例，**不能继续走旧裸wildcard代理**。其它不在本次启用范围的敏感入口（尤其export）在服务端拒绝；公开非敏感旧类型按既有批准范围保留，不因本条退役全部AL功能。

```text
ALListQueryV1 = {type_name: string, generation: UIntString,
  fields: FieldName[1..32], filters: [{field: FieldName, op: eq|in, value: Scalar|Scalar[]}],
  sort: {field: FieldName, direction: asc|desc}?, limit: int, cursor: string?}
ALAuthorizedReadV1 = {query_kind: list|count|metadata, query: ALListQueryV1,
  authorization: {principal_id: ID, scope_ref: string, policy_revision: UIntString,
    audience: "activelist", purpose: "authorized_read",
    type_name: string, generation: UIntString, allowed_fields: FieldName[],
    scope_kind: none|constrained|whole_type_approved,
    row_constraints: [{field: RegisteredScopeField, allowed_values: Scalar[]}],
    query_hash: SHA256, valid_until: Timestamp}}
ALListPageV1 = {items: AuthorizedDocument[], next_cursor: string?, has_more: bool,
  as_of: Timestamp, generation: UIntString}
ALCountV1 = {count: int, as_of: Timestamp, query_hash: SHA256}
```

这是同步QueryPort的候选接线，不向任务command枚举顺手增加list/自由query，也不保存每个GET的永久operation。核心从当前主体/L1/L2/L3计算row_constraints与allowed_fields；filters只是用户额外收窄，AL以AND求交。字段/作用域字段、可用运算符由所选类型profile登记，值绑定SQL参数，不接受用户SQL/JSONPath。`scope_kind=none`明确空页/计数0；constrained必须有非空可信条件、allowed_fields非空且请求字段为其子集，缺失/非法拒绝；whole_type_approved只来自显式批准profile，不从空条件推导。排序字段必须可读且加稳定document_id次序；首档limit≤100、条件≤10、in值≤50为可收紧候选。

RegisteredScopeField由受信属主/profile维护，是domain授权锚；普通insert/update不能自报或改写该组织/归属值，创建由可信服务根据获准目标设置，授权迁移走独立受审入口，缺失/非法锚默认不可见。否则即使row_constraints可信，也可能过滤一条用户自行篡改的归属。首组eq/in只对profile明确的非空标量及类型执行（0与false不混同）；缺键/null等未支持筛选明确拒绝，不偷做无保护cast。metadata中的类型清单同样只列获权类型/字段；metadata不收行filters/cursor/sort，count不收cursor/sort，避免多种解释。

核心→AL用固定候选 `POST /internal/data/query` 的**签名body**携完整query/authorization/query_hash；仅登记核心服务可发且AL重新核所有绑定。现有HMAC未覆盖URL query，不能把权限scope或过滤器放query后声称已签名，也不从X-Operator取权。浏览器旧GET参数由网关规范化，不能作为可信授权片段原样透传。授权准入期限沿现有短时约束，发送前复验；有权请求在跨网传输中的有界失效窗口明确保留，不承诺分布式瞬时撤权。

cursor是不透明有界分页引用，绑定类型/generation、规范查询hash、服务器scope/字段及排序基线；当前权限收紧则重验并拒绝旧cursor/要求重查，不放宽范围。源接口只给数组/无total时页面用has_more/next，不从已下载几页伪造总量；确需总量才在同一范围/谓词用count用例，不能先全量下载再过滤。metadata只返回当前可读字段和获准动作，不暴露隐藏列名/样本/全库总数。无数据返回空页；授权依赖失败503、无权拒绝，不能空成功。export启用前补同一谓词/字段、预算和撤权检查，未接时服务端拒绝，UI不显示已支持。

具名TDD `TestALList_AllReadEntrypointsScoped`、`TestALList_QueryTamperAndCursorScope`、`TestALMetadata_NoHiddenFields`：两组织同类型记录、无权字段条件/排序、签名后改query、旧cursor撤权/换账号、无total、超限/源失败与直接绕代理分别核；list/count明细范围相同，隐藏列/记录不从total或metadata泄露。这个selected profile归原TD/AD及F004必要读取范围，若真实业务确需新增复杂查询/导出，先核复用与净增再启用，不为补合同自动改价。

计价边界：原净估TD03只明列core typed AL client及selected读写投影，不能据此认定AL本库的所有list/count/metadata授权改造已免费含完。该接口批开工前对TD03/TD04、CE03及F004逐项扣重，未含必要接线补子估；未启用的大导出/复杂筛选不计首版。当前68–104日仍是候选，本文不宣称这次澄清已证明零净增或确定节省天数。

### 8.2 存量任务入口与维护动作的接线门

2026-11-15[权限专项](../review/reports/authorization-design-implementation-review-2026-11-15.md)确认：旧共享队列全可见是phase3/16 E-⑤的已定非敏感profile，不直接称当前IDOR；但接入受限风险/工单/AL数据后，以下旧入口须随D12及CE03本地子出口受保护，不能只建设新的operation/receipt接口。

| 现有入口/消费者 | 当前启用所需边界 |
| --- | --- |
| `TaskrunnerHandler.GetTask/ListRuns/ListJobs/DeadLetters`及后继artifact/日志 | 以真实业务锚或批准的非敏感共享profile决定范围，先过滤再分页/total，参数/结果/错误按字段投影；submitted_by筛选不是授权 |
| `CreateJob/UpdateJob/Trigger/Submit/Cancel/Retry` | 当前动作资格＋对象职责；提交、重跑、定时触发和执行均重验；只知道task/job ID不增加操作权 |
| `JobsCallbackService.Execute`与系统cron | 服务验签不等于原actor/固定后台主体有业务权；维护action另有限定主体，回调仍执行动作策略与受控参数验证 |
| 新建Job、`MessageAdminHandler.UpsertRule`及后继归因消费 | 核心从可信context写created_by/operator，不透传浏览器声明；TR明确服务合同，历史导入另外受控 |

AUTH-02已实测业务`task:submit`可执行`audit_archive(retention_days=1)`，将旧行移至JSONL并从在线表删除。最小修复不等CE03整包：维护action按系统/审计维护职责拒绝普通业务任务，保留期覆盖受控；查询action列表不能视为授执行权。具名反例`MaintenanceAction_BusinessSubmitDenied/RetryRechecksOriginalAction/SystemCronAllowed/RetentionOverrideControlled`，同时验服务配置正常、审计读403的用户仍不得借tasks入口维护审计。AUTH-06补`JobCreatedBy_ServerOwned/MessageRuleCreatedBy_ServerOwned`，对照HTTP审计真实actor未丢失；不凭伪造created_by构建“我的任务”权限。

首批只保护实际启用敏感能力与上述当前维护越界，不重写所有非敏感TR业务，也不引入万能invoke或第二个权限引擎；未具安全接线的敏感旧深链服务端拒绝。AUTH问题仍是开放缺陷，本页仅修设计，不能据此声称已拦截。

### 8.3 存量任务窄修复：准确接线与参数、归因

只读基线：主仓`610c1d76`、TR `b0906b83`。本段落实AUTH-02/06的4–7日有界子估，不另做CE03/完整I03；依赖的共同receipt/意图仍归C03/R01，不能重复计价。下列策略端口是候选，不是现成API或新增通用策略引擎。

现状`internal/pkg/jobs/registry.go`的`ActionMeta`只有描述/schema，`TaskrunnerHandler.failFastAction`只查注册；`TaskrunnerService.CreateJob/UpdateJob`转发raw body，`JobsCallbackService.Execute`的动作/params快照不证明actor有执行资格。注册、服务验签、任务幂等、动作授权是四件事，不能相互替代。

**最小动作策略**由核心动作属主登记：action_id、business/maintenance分类、允许的可信入口/主体及参数检查函数；先只用于实际注册动作。调用`AuthorizeAction(ctx, principal, action, immutable_input, origin)`的候选内部接缝返回允许/拒绝/依赖不可证，不把页面所见动作清单当可执行清单。未登记敏感动作默认拒绝。对普通既有非敏感profile保留原行为，不临时造一套任意条件DSL。

| 入口及真实锚点 | 修复时必须取的输入／顺序 | 失败出口 |
|---|---|---|
| 主仓`internal/handler/taskrunner_handler.go`的`Submit`→`TaskrunnerService.Submit` | 原JWT/L1后，用可信主体、action及严格解析params验资格；仍守64KB/timeout/callback_url约束；内部调用也走Service同一检查 | 无资格403、非法params400；拒绝时TR调用和新执行均为0，不能只藏按钮 |
| 同文件`CreateJob/UpdateJob` | 新建取将保存的真实action；更新读原定义后**只合并TR JobPatch白名单**再验对象管理权及最终action/params。现patch不能改action_id/trigger_type/created_by；params省略保持、显式null拒绝，不能用TR忽略的未知body字段替换授权对象；enabled激活也验 | 读失败/未知action不以空值放行，不先远程修改再补校验；不改变patch的省略/null既有约束 |
| 同文件`Trigger/Retry`及TR原job/task | 使用受权查询取得的原定义或原task快照，不能只检查body的job_id/task_id；重试仍按原action/params核当前资格，禁止换成新动作 | 获取不到原可信输入不执行；撤权后不得以“以前已创建”继续发起新执行 |
| 主仓`internal/service/jobs_callback.go`的`JobsCallbackService.Execute` | 验签/快照/执行权与当前动作资格均通过才进Handler；执行资格绑定核心持久准入事实或受审固定后台配置，不从body.actor/created_by猜身份 | 缺原可信准入记录的敏感旧任务held/拒绝，不能因ClaimCallbackRow可补行就现造授权；恢复只补必要事实，不重做已证明的效果 |
| 主仓`internal/service/cron_scheduler.go`的`CronScheduler`及app装配 | 进程内cron使用构造时固定的后台维护主体/动作/受控配置，不能由字符串actor="system"触发特权。TR cron须登记可信job版本/后台主体，并复用同动作策略 | 未批准的后台绑定不开通；配置非法启动/启用时明确失败。两种归档调度保留原二选一，不能同时恢复造成重复跑 |

**原定义读取是新增窄接线，不是现成功能**：当前主仓client只有ListJobs，TR HTTP也没有单job GET，不能靠分页列表恰好找到就声称可靠。拟补TR属主的`GET /v1/jobs/:job_id`与核心client内部GetJob port，复用TR repository的单条读取/JobView，经现AK/SK及受信caller保护；不新开核心前端绕权限读接口。核心只在原L1/职责检查允许的用例内部取配置用于判定，读取失败关闭，raw参数不下发无权用户。读到的定义/params需绑定后续准入快照，最终执行仍核该绑定与当前资格；跨网络GET本身不能锁住后来的TR修改，这个版本/快照接缝复用C03，不在窄修复中伪称已有跨库事务。

**保留期边界**：`internal/service/audit_archive.go`的`archiveParams/Handle`当前正值可覆盖配置；修复后普通业务输入不得降低或任意覆盖保留期。默认沿已受审服务配置；零/缺省、负数、类型错误及超界分别按原兼容与严格参数预期验证。若允许运维临时覆盖，须有独立受权入口、批准值/有效窗及审计，不复用普通task params。实际允许的维护主体及是否保留人工覆盖汇入待确认，不在本轮猜生产保留天数。在线归档与归档文件清理是两种保留配置，不能只限制其中一个或让外部任务传出目录/表名。

**归因合同**：主仓浏览器入口接受旧created_by字段也不采用它，可信actor在Service边界传入；新消息规则的created_by取当前主体，更新保留原creator，实际modifier记本次操作，不重写历史归属。`internal/service/message_admin_service.go`的`UpsertRule`现ON CONFLICT分支本就不改created_by，修复须保持CAS/version与此语义。内部调用亦显式提供可信上下文，不能只在handler补字段后仍留Service旁路。

TR `internal/handler/server.go`的`Deps.createJob`目前body.created_by优先于验签operator；目标改为从**被批准可声明操作人的caller**及验签context构造归因。收到operator仅证明签名内容，仍须核caller允许为该用户/服务调用；不能任意服务签一个admin名字就取得核心权限。服务调用没有真人时归因服务主体，不伪造真人。旧wire可兼容接收created_by但不授其权威，body伪造值覆盖/忽略的行为变化与消费者同步。当前64字符存储与签名operator旧形状保留兼容；稳定本地principal_id在核心可信准入中保存，username/工号展示值不能变成授权主键。历史记录不批量改造，存疑旧字段不作为“我的任务”的授权依据。

**独立验收（待新增）**：TASK-01业务task:submit但无审计读/维护权，Submit/Create/Update/Trigger/Retry及回调均不能移除任何审计行；TASK-02授权固定cron仍能按配置归档，旧actor="system"伪装失败；TASK-03覆盖参数非法/降保留期拒绝且默认正常，目录/文件保留配置不受body控制；TASK-04提交后撤权、旧敏感job无可信来源、原任务被换参均零新效果；TASK-05任务与消息created_by伪造不生效，更新原creator不变而modifier真实；TASK-06无真人服务归因可解释，未受信caller不能冒用用户，合法跨仓签名及普通业务任务仍通过。

可复用主仓`internal/handler/taskrunner_handler_test.go`的`TestTaskrunnerHandler_CreateJobFailFast/TestTaskrunnerHandler_BadRequest/TestTaskrunnerHandler_ProxyPassthrough`、`internal/service/taskrunner_service_integration_test.go`的`TestTaskrunnerServiceSubmitRecordsVoucher`、`internal/service/message_rule_version_integration_test.go`的`TestMessageRule_VersionCAS`；这些目前不证明动作授权。TR `internal/handler/server_test.go`的`TestCreateJobCreatedByFallback`明确断言body优先，实施须先说明合同纠正，再补有效红例，不能把当前绿色当可信归因。授权拒绝须真实HTTP＋PG审计行/下游调用数对照，最终Handler入口单独测；两种证据不能互代。

**待集中确认TASK-D01**：例如已具系统管理权的人临时把在线保留期从180天降为30天排障，是否允许通过TR手动发起？建议首批保留固定系统cron，人工维护仅独立受权、带理由和受控值；普通业务任务始终禁止。也可保留经批准的维护TR job，但需绑定明确后台主体和定义版本。该选择只阻塞维护人工/额外后台开通，不阻塞业务任务越界修复、可信归因、默认cron或其它模块设计；当前不执行任何归档/删行。

## 9. 测试先行用例与交付出口

下面是实施时必须先红后绿的具名预期，**本次没有执行这些未来产品测试**。每个用例记录当前候选五仓组合与真实效果；并发缺陷按standards移除修复后至少8次稳定红验证，不能因加timeout/skip掩盖错误。

1. `TestNodeRun_IntentCommitAtomic`：源事务/intent任一失败均无可见已安排；commit后kill仍能提交同task一次。
2. `TestSubmit_SameOperationSameInput`与`...DifferentInputConflict`：重复返回原受权receipt；异参/主体/授权绑定不同409且零新增效果。
3. `TestCallback_RunningIsNotCompleted`：两个并发投递一个效果；running不会让TR业务succeeded，v1/v2映射按协商分别验证。
4. `TestCallback_ClaimDBFailureNoEffect`：核心claim或TR MarkRunning失败均零下游调用，错误恢复不产生无账执行。
5. `TestReceipt_CommitResponseLost`、`...LedgerWriteFailed`：实际效果已提交而响应/记账失败，query收敛且效果计数仍一。
6. `TestFence_ExpiredWorkerCannotOverwrite`：双进程抢占后旧worker不能写新代；旧请求仍可能生效时保持unknown，不因过期重做。
7. `TestUnknown_RetryCancelDoNotPretendSuccess`：unknown禁止普通retry/直接canceled，只有证据确认后补账/处置。
8. `TestNodeProjection_OldNodeAndOldRevision`：旧节点/低receipt revision迟到只留原运行；投影重试不重跑效果。
9. `TestAL_CurrentAuthorizationAndAdmission`：提交后撤权、过期claims、换target/hash/purpose/字段/fence均拒绝；无权重放不泄露旧receipt。
10. `TestAL_CASGenerationNoop`：并发patch一成功一409，generation变化拒绝；noop不生源事实/二次后继。
11. `TestAL_DataFactReceiptAtomic`：源写、outbox、receipt任一步失败回滚；杀进程/恢复仍可读同operation效果。
12. `TestAL_MonitoredWriteSurfaceGuard`：监控类型全部不支持写入口和schema破坏被服务端拒绝，非监控类型已有功能保持。
13. `TestPush_CommitBeforeAck`、`...LostAckDuplicate`：DB失败不ACK，commit后ACK丢失重投同stage，不重复派生；同id异hash隔离。
14. `TestPush_HeadCommitOrdering`：并发事务含先分配后迟提交/回滚，不越过事实；单流首事件失败不清后续。
15. `TestSubscription_CutoverAndPause`：规则W前后各归唯一版本，发布崩溃无半激活；暂停继续收件不执行，恢复超限保留余项。
16. `TestSubscription_EventTimeNotCurrentValue`：后改当前AL值不改变原事实过滤；当前查询明确时间/版本，缺projection字段不误命中。
17. `TestSubscription_ActualReadOnlyFollowup`：真实AL insert/update→inbox→TR get任务→受控查询结果，**无关联工单也成功**；核当前值/事件版本不同、同号重放和字段权限，无AL回写/状态推进，撤权held。
18. `TestCausalRoot_ReentryAndBudget`：首版验事件去重、继承真实root/parent/depth、最多一次订阅派生及只读后继，工单→AL写→后继链不被误拦；启用更宽profile时再验重入/深度扇出预算，原事实和抑制原因仍可查。
19. `TestCron_DualScannerAtomicOccurrence`：两扫描器/推进失败/Redis故障/时区边界同slot一个task，回补不复制已有unknown。
20. `TestMessage_RecipientCurrentScope`：owner/CC/负责人/只读成员及撤权分别验字段投影；无映射不广播，A→B→A保留三次真实语义；旧未发A项不改寄B，replacement按新稳定身份关联原项。
21. `TestMessage_IntentPreferenceAndPreview`：业务commit后kill仍有应发；opt-out/免打扰/节流/强制类别各留原因；预览零发送，测试仅批准目标。
22. `TestMessage_AcceptedNotDelivered`、`...TimeoutUnknown`：无送达协议不造delivered；已受理超时先query/同幂等键同冻结payload，无幂等无查询则hold；模板/owner变更不改变unknown的原目标/正文。
23. `TestMessage_ProviderCallbackOutOfOrder`：重复/乱序回执不倒退状态，鉴权/映射/429/503分类及有界退避可解释。
24. `TestQuota_TwoWorkersNoBurst`与`TestRecovery_BackupIncludesReceipts`：首版先验提供方限额、故障隔离、容量拒写/不ACK和恢复后无重复效果；实际启用双实例前再验共享配额，两者不混成全套HA首发门。
25. `TestAccess_AllReadSurfacesAndAdapterCompatibility`：列表/total/详情/附件/结果/日志/导出/消息均不越范围，旧参考adapter×新core遵相同合同，未知版本显式拒绝。

出口按三个层级分别留证：mock用于可控时序与提供方错误；contract核严格JSON/签名/HTTP状态/新旧组合；真实PG/Redis/HTTP/浏览器核事务、双进程、效果计数与恢复。离线Schema样例只证明其约束覆盖的形状/语义，不证明实际服务链已运行。主仓完整适用make门禁及UI六项、TR真实投递/cron、AL真实PG回归按实际代码触及执行；不能以本次文档检查替代未来实现验收。

切片归属避免重价：C03拥有回调/receipt/稳定slot；R01拥有共有意图/poller/对账；CE01拥有能力登记，CE02-S/D拥有共同准入/操作关联，D12拥有原票结果与受控映射；AL事务效果及首版源/订阅消费分别归本地TD/AD子范围，D15拥有风险收件职责/矩阵，MH拥有公共HTTP与消息状态，MR仅一条真实私有公司映射。UI实现/浏览器用例归既有UI分项，RELEASE只计额外跨域回归和部署恢复，不再把同一测试/机制算一遍。完整I03/D11/多消费者源/更多channel或新能力是后续已有包或需明确增量，不由“详设”自动扩首版本地报价。

## 10. 待绑定场景与官方实践取舍

2026-11-15[核心交互专项](../review/reports/core-interaction-architecture-review-2026-11-15.md)补`Delivery_SourceNamespaceAndRuleIdentity`（两来源同id、两规则同version、同pair重复）及`WebhookTransportError_QuerySecretRedacted`反例。现存站内信只按recipient筛选/total，标题快照尚未接当前对象/字段授权；新受限业务启用前，列表/深链/预览都要接本页投影策略，不能只保护未来新通知页。保留历史收件/发送事实，撤权不继续回显旧敏感正文；未迁移历史通知的原语义单列，不冒称已重授权。死信Webhook单独url字段脱敏不代表error链已脱敏，Go url.Error亦可携带完整URL；只用合成秘密作回归，不记录真实配置。

当前无需所有者逐字段选技术方案；以下只阻断实际启用项：

- 首条真实旅程绑定：具体整改流程/节点、AL类型/字段/业务含义、独立AL复核的使用方式和可执行服务主体范围。建议先用一条可重复演示的真实日常任务；若必须通用list/query或监控类型import，先补对应能力合同和净估，不能硬用演示替代。
- B08风险消息：具体五级→四级映射、阈值、上溯组织锚点、频次/免打扰及允许投影字段，在首条真实风险通知前确认；“高级别上溯”既定方向不重问。
- 公司HTTP能力：真实映射/鉴权/幂等/查询/送达证据/限流及测试窗口由内网核验，资料无需导出；无查询无幂等时unknown暂停的技术默认如需改成冒重复风险补发，须展示具体消息场景再批准。
- 发布前绑定outbox/inbox容量、保留与最长中断/补发窗口、运维对账人员和实际provider预算；永久执行证据既定承诺保留，不用普通TTL覆盖。完整HA、通用持续授权和恢复台仍按后继窗口设计，单副本本地版不据此无限等待。

官方模式仅用于上述局部选择，不引入重型平台：

- [AWS Transactional Outbox](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/transactional-outbox.html)解释业务写与通知双写及重复消费风险；本项目选择本库事实/意图同tx、消费者持久幂等，保留现有TR队列，不新增Kafka前置。
- [PostgreSQL 18 SELECT](https://www.postgresql.org/docs/18/sql-select.html)说明SKIP LOCKED适合队列式取行而非通用一致视图；本项目只用它认领投递，文档CAS/源完整水位另由领域事务保证。
- [CloudEvents 1.0.2](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/spec.md)为事件身份/上下文提供规范；本项目复用既有source/id/type/data，不把消息外壳当授权、顺序或可靠投递保证。
- [RFC 9110 §15.3.3](https://www.rfc-editor.org/rfc/rfc9110.html#name-202-accepted)区分202受理与完成；本项目只在已协商v2采用具名未终态响应，效果完成仍需持久receipt，真实消息渠道受理也不冒称送达。

变更记录：2026-11-14首次补模块详设；收敛首版push/监控写面，补独立只读get后继与现有Schema枚举缺口，补逻辑约束、失败/恢复、消息真实接线和测试出口；没有新增产品实现或企业验收声明。
