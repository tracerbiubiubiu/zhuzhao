# 首批互通：现有入口到共同用例的准确接线

日期：2026-11-15。状态：设计候选，未实现共同内核、mock服务器或新端口。源码基线core `610c1d76`、TR `b0906b83`、AL `15e93682`。本页展开[任务/AL/消息详设](task-activelist-message-detail.md)，复用C03/R01及[共同Schema](../design/capability-interconnect-detail.schema.json)，不复制command/receipt、不引入broker、任意DAG或跨库事务。下方名称均为候选接口/待写测试，不是已注册产品能力。

## 1. 一次装配，共用内核

主仓`internal/app/providers.go`的`provideApp`构造一次共同操作内核、当前授权恢复器、operation查询与结果投影器；具名HTTP、已迁移工单动作、受控callback和AL订阅注入同一实例。`internal/app/wire.go`的`InitializeApp`保持wire.Build(provideApp)，生成文件由Wire产出，不手改。当前provideApp/provideJobsCallbackService尚未构造该内核。

TR自己的InitializeApp仍装配Submit/队列/cron/callback；AL自己的InitializeApp仍装配数据写主/源outbox。不下沉core repository到兄弟仓或utils。启动清单只登记已迁移capability/version/executor，设计库存不等于runtime目录。待实现预期：`TestAppComposition_SelectedEntriesShareKernel`、`TestAppComposition_UnregisteredCapabilityFailsClosed`。

## 2. 工单事务内计划，提交后不再直发同一任务

当前`internal/service/ticket/transition_service.go`的`Service.Transition`在commit后调用`actions.go`的`ActionRegistry.RunPostActions`，`actions_builtin.go`的`SpawnTaskAction.Execute`才提交TR；ActionContext没有tx与稳定node_run。只把TaskSubmitter换成IntentWriter仍不能做到票/意图原子。

候选无副作用planner输出`DurableActionPlan={action_index,capability_id,contract_version,definition_hash,typed_input,target,result_policy}`，action_index沿发布配置的原序号，不因筛除best_effort重新编号。Transition在现有tx.Commit前，以相同DBTX写node_run、operation、intent及初始delivery；任何计划/持久化失败使转换失败。planner不得联网/调用旧Execute；外部预填等在锁外取数，锁内核被冻结输入及当前约束。

planner可预分配node_run_id；锁内确认原票、发布definition_hash、全票visit_no及原action_index后才形成最终command/hash。报告分支typed_input.ticket_id/node_run_id/definition_hash分别等于target及本事务node_run，不先hash再悄换绑定；不一致整笔回滚。

commit后的RunPostActions只处理本发布版本未迁移的best_effort动作，同一spawn不能双跑；仅本批可靠动作前移，不能把所有旧动作语义自动改强一致。待实现预期：`TestTransition_DurablePlanFailureRollsBackStatus`、`TestTransition_ActionIndexPreservesPublishedOrder`、`TestTransition_CommitThenKillNoDirectSpawn`、`TestTransition_PlanCommandNodeBindingMismatchRollsBack`。

## 3. TR旧action与结构化callback的桥

每个首批能力固定TR action/version，不接受callback临时指定另一能力。候选任务引用`CapabilityTaskRefV1={schema_version:"1",operation_id,canonical_input_hash,execution_binding_ref}`；core从持久绑定恢复原command、主体、definition、task/action/version关系，不信任body.actor。核固定关系、当前执行资格与所选唯一execution claim后才调用共同typed用例；不能先取legacy claim再叠能力claim。

现有`internal/pkg/jobs/registry.go`的`Handler/Registry`、`internal/service/jobs_callback.go`的`JobsCallbackService.Execute`、`internal/handler/jobs_handler.go`的`jobCallbackBody/Callback`继续服务未迁移维护动作；Handler.Handle的nil不是具效果凭证的receipt。TR `internal/callback/client.go`的`body/Client.Do`当前按2xx完成，不能沿用到严格能力。新adapter结果按[C03 v2](../design/cross-repo-execution-contract.md)映射200/202/503，不另造完成协议。待实现预期：`TestCapabilityCallback_ReferenceCannotSwapActionOrCommand`、`TestCapabilityCallback_LegacyNilIsNotEffectReceipt`；unknown/running/fence复用原合同用例。

## 4. 工单任务卡的唯一状态来源

候选`TicketTaskQueries.ListForTicket`返回有界卡：`{operation_id,node_run_id,definition_hash,task_id?,operation_status,receipt_revision,last_observed_at,projection_status,summary?,artifact_refs[],allowed_actions[]}`。operation_status只取core已核收据；TR queued/running作为另一执行观察并列时间，不以TR succeeded推断工单完成。summary/artifact只取原运行受控投影，不透传raw params/result/error。

当前`internal/handler/taskrunner_handler.go`及`internal/service/taskrunner_service.go`的`GetTask`透传json.RawMessage，SpawnTaskAction.Execute丢弃提交响应，均不是新卡实现。新读端先验当前票/字段权再分页/计数；artifact下载重验。TR不可达保留已证状态并标观察陈旧，不改判failed。unknown只给原操作查询/受权对账；投影修复不调executor。候选GET `/api/v1/tickets/:id/task-results`是新只读入口，原任务GET保留并按维护资格迁移，不能让它成为raw结果旁路。

待实现预期：`TestTicketTaskView_ExecutionSuccessIsNotBusinessCompletion`、`TestTicketTaskView_TRUnavailablePreservesConfirmedReceipt`、`TestTicketTaskView_RepairProjectionDoesNotExecute`。

## 5. AL换版边界是持久握手

core不能持AL数据库锁。固定签名POST `/internal/changes/boundary`由AL处理：`BoundaryRequestV1={boundary_operation_id,consumer_id,source,stream_id,generation}`；回`BoundaryReceiptV1={boundary_operation_id,source,stream_id,generation,through_cursor,boundary_ref}`。AL在与源写相同stream head锁内保存边界/W并提交，响应在锁外；同operation同参返原W、异参409。可按原operation重投或固定签名POST `/internal/changes/boundary/query`查询，目标均在body。

core先暂停该规则动作准入，收边界后一次本库事务保存旧end_at/新start_after及激活状态，再恢复处理，inbox继续收。AL已提交而响应丢失，查询原boundary，不能重新取更大的W偷偷改变范围；core失败保持准备态按原receipt恢复。逻辑记录可并入候选stream结构，不建设通用配置发布平台。当前AL DataService.Insert/Update无该端口，不能把文档“取W”当已有跨仓实现。

待实现预期：`TestSubscription_BoundaryReplyLostReturnsSameW`、`TestSubscription_CoreCrashAfterSourceBoundary`、`TestSubscription_NoNetworkCallUnderStreamLock`。

## 6. AL普通写的可信来源

监控类型的登记能力写继承command.operation/causality；普通产品insert/update经可信入口构造`NativeWriteOriginV1={operation_id,invocation_id,request_key,causality,producer_ref,authorization_binding_ref}`，与数据、receipt、事实同AL事务持久。无父操作的真人写创建一次root，事件/后继继承；同request_key重投保留operation/root。origin只在受控adapter的签名body元信息传递，不混入动态data列；该签名仍只证明允许服务的声明，必须继续核登记producer和当前行/字段准入。

authorization_binding_ref不是授权本身，解析到受信当前主体/授权引用、native动作、type/generation、update目标/版本或insert创建范围、字段集合、规范输入hash及短期限，AL核其与本次输入匹配后执行，不能只验producer登记。复用已有短时准入机制，不建永久用户交换平台。request_key按producer＋主体命名空间去重；同命名空间同key异目标/输入拒绝，已绑定operation不接受主体改写。不同主体可各自使用相同key但不能串读receipt，不将key误作全局唯一或访问凭证。

当前AL `internal/service/data_service.go`的`InsertInput/UpdateInput/DataService.Insert/Update`只含data/version/operator，`internal/handler/server.go`的`currentOperator`不能证明用户当前权；X-Operator、浏览器actor/root不授信。core网关或已批准服务主体生成可信origin；监控类型拒绝不能恢复来源/当前写权的旧裸入口，非监控原能力保留。此处不把insert冒充已加进现共同command枚举的新能力，也不先实现通用全AL事件平台。

待实现预期：`TestAL_NativeMonitoredWriteHasTrustedOrigin`、`TestAL_NativeRetryPreservesOperationAndRoot`、`TestAL_OperatorCannotForgeCausality`、`TestAL_NativeOriginAuthorizationOrInputMismatchNoEffect`。首个合成监控profile选已有参考`device_status`，事件白名单选status，get也只请求status；它仅表示观测状态，不是CMDB或设备入口。以后需要其它字段先发布白名单新版本，不把任意整行/隐藏字段纳入事件。

## 7. 消息HTTP公共mock的绑定

可靠路径的SendRequestV1增加schema_version="1"与payload_sha256；SendReceiptV1增加schema_version、channel_profile_ref、payload_sha256。payload hash由首次发送前冻结的公共语义载荷计算，不是厂商转换后HTTP字节hash：键精确为delivery_id/channel_profile_ref/recipient_ref/template_version/subject/body/business_ref/source_event_ref/rule_id/rule_version/operation_id/expires_at；不含schema_version封装、hash本身、幂等键、签名/attempt时间。rule_version为正integer，其余按参考Schema。沿共同规范编码，禁止Trim/Unicode归一化。profile_ref钉不可变版本，接收人或内容变化只能独立replacement，不借重试改payload。

公共mock固定签名POST `/v1/messages/send`、`/v1/messages/query`，core固定签名POST `/internal/messages/receipts`收异步回执；QueryRequestV1精确为`{schema_version:"1",delivery_id,channel_profile_ref,provider_receipt_ref?}`放body，不靠未签query鉴权。真实公司adapter自行转换其协议，不要求公司也遵这三个URL。回执核原delivery/profile/payload；异步另含provider_event_id，按可信来源/事件去重并以本地revision CAS更新，缺真实证据不补造delivered，迟到accepted不回退delivered。未知provider回执隔离，不直接回填成功。

fixture按(profile,idempotency_key)记录受理：接收方先按12字段重算payload_sha256，声明与实际不符则受理前拒绝，不能用自报hash证明同参。同hash返原receipt，异hash409且效果不增；core按本地冻结hash核回执。合同测试须实际模拟“已受理再断连接”，core重启query仍找原receipt；无查询/无幂等profile进入unknown待核，不能只靠异常mock声称效果零次。**本轮只有协议候选与合成向量，不声称已跑该HTTP服务器/恢复旅程**。旧`internal/pkg/channelsender/port.go`的`ChannelSender/UIPStubSender.Send`仍是error旧端口，nil不转delivered；`MessageDispatcher.Dispatch`的sent旧语义沿legacy投影。

待实现预期：`TestHTTPMessage_SameKeyDifferentPayload409`、`TestHTTPMessage_ReceiptWrongProfileOrPayloadRejected`、`TestHTTPMessage_AcceptThenDisconnectQueryAfterCoreRestart`、`TestHTTPMessage_LateAcceptedCannotRegressDelivered`、`TestHTTPMessage_DeclaredHashMismatchRejectedBeforeAcceptance`。

## 8. 站内投影与逐消费者切换

站内是独立消费者，不是HTTP成功回调。候选唯一键`(source,event_id,inbox_policy_id,policy_version,recipient_user_id,semantic_key)`，消费凭证与notifications投影同core事务，重复intent返原通知ID。原站内收件/自噪音规则作为具名policy，不自动套外发message_rule/外发偏好。read_at只表示站内已读；外发失败/unknown/补发不复制、撤销或置读站内通知。当前list/total/badge/深链均要经当前资源投影，不能只recipient过滤。

切换按source范围＋consumer_id＋发布版本记录legacy/shadow/durable，同一源提交对同消费者真实效果仅选一路。已选源事务冻结consumer_id、route_revision及该次路由决定，durable保存可靠intent；post-commit Hook根据该次快照跳过已迁移消费者，不按新全局开关重新选路。切换只影响边界后新提交，旧在途沿原决定收敛，关新worker不自动回退Hook重发。shadow链只记录候选不发不插，真实效果仍沿该次冻结的legacy路线，不把影子误当生产停发指令。

`internal/app/app.go`的`NewApp`订阅ticket.notify/ops.notify；`internal/service/ticket/inbox_service.go`的`SetEventRegistry/OnDomainEvent/collectRecipients`订阅inbox.notify/inbox.ops，这是不同消费者。迁外发不关闭站内，迁站内则停同范围Hook Insert；回退停新效果但保留receipt/reader/在途对账。`internal/repository/notification_inbox_repo.go`的`Insert/Inbox/UnreadCount`当前无源事件唯一键，仅recipient过滤，候选尚未实现。

待实现预期：`TestInbox_DurableReplayCreatesOneProjection`、`TestConsumerCutover_HTTPAndInboxAreIndependent`、`TestInbox_ExternalUnknownDoesNotDuplicateOrMarkRead`、`TestInbox_CurrentProjectionFiltersListTotalAndBadge`、`TestConsumerCutover_ModeChangesBetweenCommitAndHook`。

## 9. 实施边界

以上均在原C03/R01/CE02/CE03、D11/D12、D15及薄UI范围内扣重，具名source/TDD不再重复计机制。真实公司HTTP鉴权/限流/回执/收件人仍按原窗口核；未知不阻公共mock。共同operation与事实/投递/查询各自一个身份及重试主，短同步与持久异步按原合同选择；这里没有新增消息队列采购或全平台一次切换要求。
