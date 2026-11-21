# 首批互通：现有入口到共同用例的准确接线

日期：2026-11-15。状态：设计候选，未实现共同内核、mock服务器或新端口。源码基线core `610c1d76`、TR `b0906b83`、AL `15e93682`。本页展开[任务/AL/消息详设](task-activelist-message-detail.md)，复用C03/R01及[共同Schema](../design/capability-interconnect-detail.schema.json)，不复制command/receipt、不引入broker、任意DAG或跨库事务。下方名称均为候选接口/待写测试，不是已注册产品能力。

## 1. 一次装配，共用内核

主仓`internal/app/providers.go`的`provideApp`构造一次共同操作内核、当前授权恢复器、operation查询与结果投影器；具名HTTP、已迁移工单动作、受控callback和AL订阅注入同一实例。`internal/app/wire.go`的`InitializeApp`保持wire.Build(provideApp)，生成文件由Wire产出，不手改。当前provideApp/provideJobsCallbackService尚未构造该内核。

TR自己的InitializeApp仍装配Submit/队列/cron/callback；AL自己的InitializeApp仍装配数据写主/源outbox。不下沉core repository到兄弟仓或utils。启动清单只登记已迁移capability/version/executor，设计库存不等于runtime目录。待实现预期：`TestAppComposition_SelectedEntriesShareKernel`、`TestAppComposition_UnregisteredCapabilityFailsClosed`。

## 2. 工单事务内计划，提交后不再直发同一任务

当前`internal/service/ticket/transition_service.go`的`Service.Transition`在 commit 后调用`actions.go`的`ActionRegistry.RunPostActions`，再由`actions_builtin.go`的`SpawnTaskAction.Execute`提交 TR；ActionContext 没有 tx 与稳定 node_run。因此，只把 TaskSubmitter 换成 IntentWriter，仍不能保证工单状态和待执行意图在同一事务中一起保存或一起失败。

候选 planner 只生成计划，不执行动作，输出`DurableActionPlan={action_index,capability_id,contract_version,definition_hash,typed_input,target,result_policy}`。action_index 沿用发布配置的原序号，不因筛除 best_effort 重新编号。Transition 在现有 tx.Commit 前，以同一 DBTX 写入 node_run、operation、intent 及初始 delivery；生成计划或保存任一记录失败，都使转换失败。planner 不得联网或调用旧 Execute。外部预填等数据在锁外读取，取得锁后核对已固定的输入和当前约束。

planner可预分配node_run_id；锁内确认原票、发布definition_hash、全票visit_no及原action_index后才形成最终command/hash。报告分支typed_input.ticket_id/node_run_id/definition_hash分别等于target及本事务node_run，不先hash再悄换绑定；不一致整笔回滚。

commit后的RunPostActions只处理本发布版本未迁移的best_effort动作，同一spawn不能双跑；仅本批可靠动作前移，不能把所有旧动作语义自动改强一致。待实现预期：`TestTransition_DurablePlanFailureRollsBackStatus`、`TestTransition_ActionIndexPreservesPublishedOrder`、`TestTransition_CommitThenKillNoDirectSpawn`、`TestTransition_PlanCommandNodeBindingMismatchRollsBack`。

## 3. TR旧action与结构化callback的桥

每个首批能力固定TR action/version，不接受callback临时指定另一能力。候选任务引用`CapabilityTaskRefV1={schema_version:"1",operation_id,canonical_input_hash,execution_binding_ref}`；core从持久绑定恢复原command、主体、definition、task/action/version关系，不信任body.actor。核固定关系、当前执行资格与所选唯一execution claim后才调用共同typed用例；不能先取legacy claim再叠能力claim。

现有`internal/pkg/jobs/registry.go`的`Handler/Registry`、`internal/service/jobs_callback.go`的`JobsCallbackService.Execute`、`internal/handler/jobs_handler.go`的`jobCallbackBody/Callback`继续服务未迁移维护动作；Handler.Handle的nil不是具效果凭证的receipt。TR `internal/callback/client.go`的`body/Client.Do`当前按2xx完成，不能沿用到严格能力。新adapter结果按[C03 v2](../design/cross-repo-execution-contract.md)映射200/202/503，不另造完成协议。待实现预期：`TestCapabilityCallback_ReferenceCannotSwapActionOrCommand`、`TestCapabilityCallback_LegacyNilIsNotEffectReceipt`；unknown/running/fence复用原合同用例。

## 4. 工单任务卡的唯一状态来源

候选`TicketTaskQueries.ListForTicket`返回有界卡：`{operation_id,node_run_id,definition_hash,task_id?,operation_status,receipt_revision,last_observed_at,projection_status,summary?,artifact_refs[],allowed_actions[]}`。operation_status只取core已核收据；TR queued/running作为另一执行观察并列时间，不以TR succeeded推断工单完成。summary/artifact只取原运行受控投影，不透传raw params/result/error。

当前`internal/handler/taskrunner_handler.go`及`internal/service/taskrunner_service.go`的`GetTask`透传 json.RawMessage，SpawnTaskAction.Execute 丢弃提交响应，均不是新卡实现。新读取入口先检查用户对当前工单和字段的访问权，再分页和计数；下载 artifact 时重新检查。TR 不可达时保留已证实的状态，并标明观察信息已过时，不能改判 failed。unknown 只提供原操作查询和获授权的对账；修复工单里的结果显示时不调用 executor。候选 GET `/api/v1/tickets/:id/task-results`是新只读入口，原任务 GET 保留并按维护资格迁移，不能让用户借它绕过限制读取原始结果。

待实现预期：`TestTicketTaskView_ExecutionSuccessIsNotBusinessCompletion`、`TestTicketTaskView_TRUnavailablePreservesConfirmedReceipt`、`TestTicketTaskView_RepairProjectionDoesNotExecute`。

## 5. AL换版边界是持久握手

core不能持AL数据库锁。固定签名POST `/internal/changes/boundary`由AL处理：`BoundaryRequestV1={boundary_operation_id,consumer_id,source,stream_id,generation}`；回`BoundaryReceiptV1={boundary_operation_id,source,stream_id,generation,through_cursor,boundary_ref}`。AL在与源写相同stream head锁内保存边界/W并提交，响应在锁外；同operation同参返原W、异参409。可按原operation重投或固定签名POST `/internal/changes/boundary/query`查询，目标均在body。

core 先暂停该规则发起动作，但 inbox 继续收件。收到边界后，在一次本库事务中保存旧 end_at、新 start_after 及激活状态，再恢复处理。如果 AL 已保存边界但响应丢失，就查询原 boundary；不能重新取更大的 W，悄悄改变新旧规则各自处理的范围。core 失败时保持准备态，按原 receipt 恢复。逻辑记录可并入候选 stream 结构，不建设通用配置发布平台。当前 AL DataService.Insert/Update 无该端口，不能把文档“取W”当已有跨仓实现。

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

站内通知独立消费业务事件，不等 HTTP 外发成功才创建。候选唯一键`(source,event_id,inbox_policy_id,policy_version,recipient_user_id,semantic_key)`，消费凭证与 notifications 显示记录在同一 core 事务中保存，重复 intent 返回原通知 ID。原站内收件规则和避免通知本人操作的规则作为具名 policy，不自动套用外发 message_rule 或外发偏好。read_at 只表示站内已读；外发失败、unknown 或补发，都不复制、撤销或标读站内通知。列表、总数、角标和深链都要按当前资源权限筛选及限制内容，不能只按 recipient 过滤。

切换时按 source 范围、consumer_id 和发布版本记录 legacy/shadow/durable；同一次源提交对同一消费者只能选择一条会产生真实效果的路线。源事务固定 consumer_id、route_revision 及本次路由决定，durable 路线保存可靠 intent。post-commit Hook 按这次保存的决定跳过已迁移消费者，不能看到新的全局开关后重新选路。切换只影响边界后的新提交，已经在途的操作继续按原决定完成或对账；关闭新 worker 不会自动退回 Hook 重发。shadow 路线只记录候选，不发送消息也不插入通知；真实效果仍走该次固定的 legacy 路线，不能把影子记录当作停止生产发送的指令。

`internal/app/app.go`的`NewApp`订阅ticket.notify/ops.notify；`internal/service/ticket/inbox_service.go`的`SetEventRegistry/OnDomainEvent/collectRecipients`订阅inbox.notify/inbox.ops，这是不同消费者。迁外发不关闭站内，迁站内则停同范围Hook Insert；回退停新效果但保留receipt/reader/在途对账。`internal/repository/notification_inbox_repo.go`的`Insert/Inbox/UnreadCount`当前无源事件唯一键，仅recipient过滤，候选尚未实现。

待实现预期：`TestInbox_DurableReplayCreatesOneProjection`、`TestConsumerCutover_HTTPAndInboxAreIndependent`、`TestInbox_ExternalUnknownDoesNotDuplicateOrMarkRead`、`TestInbox_CurrentProjectionFiltersListTotalAndBadge`、`TestConsumerCutover_ModeChangesBetweenCommitAndHook`。

## 9. 实施边界

以上均在原C03/R01/CE02/CE03、D11/D12、D15及薄UI范围内扣重，具名source/TDD不再重复计机制。真实公司HTTP鉴权/限流/回执/收件人仍按原窗口核；未知不阻公共mock。共同operation与事实/投递/查询各自一个身份及重试主，短同步与持久异步按原合同选择；这里没有新增消息队列采购或全平台一次切换要求。
