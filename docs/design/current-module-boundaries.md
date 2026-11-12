# 当前模块依赖与事务边界核验

日期：2026-11-12。性质：FD01补研究，覆盖现有IAM、工单、对象、消息、调度及跨仓数据写主，并区分HR、风险、AI和私有扩展的目标接缝。本文给出源代码事实与渐进处置建议，**没有修改实现、批准新的规范豁免或证明未来合同已实现**。排期仍由[总账](../review/planning-backlog.md)拥有；根集成者同步控制台和FD01状态。

## 核验范围与结论

本次源码基线为主仓`061c56af`、taskrunner `44f6b644`、activelist `a34d6603`、utils `e2cf85a1`。全量静态库存扫描了四仓218个受控非测试Go文件，提取本地import与SQL原始字符串中的写表线索，再对下列入口、事务、装配和现有测试源码定向回读。库存不是类型解析后的完整调用图，动态SQL、数据库触发器/FK、所有运行交错及提供方接口没有由扫描证明。原始清单、源摘要和逐项裁定保留在本次ignored证据中。

主要结论：

- **IAM中的用户、角色、组织和菜单是当前同一一致性域。** `UserService`持`RoleRepo`不应直接定为跨服务腐化；用户及身份关系原子创建、删除时清理组织owner等应保留。
- **组织路径、工单附件和头像有必要的同库联合事务。** 简单改成异步事件或一次HTTP调用，会先破坏可见性、对象归属和GC互斥。本轮建议显式登记这些受控协作，而非为了目录隔离拆事务。
- 真正需要先收紧的是权限变化与业务提交的共同守卫、文件引用写侧、跨仓“受理/执行/记账”以及未来非HTTP入口的授权合同，分别承接C02/C03/C04/R01。窄接口限制能力，不替代这些正确性工作。
- HR、风险、LLM、MCP及私有扩展运行时尚未实现。现有DataFetcher空注册表、情报Provider、进程内Hook或不可变网关，都不能当成它们的实现证明。
- 当前没有要求拆出IAM、工单或对象服务。G01/G02/G03、工具链升级和U01不依赖全仓接口重写；只有真正改变本页边界的工作需要先完成对应合同和反例。

## 怎样理解规则与处置

`docs/design/architecture.md`的`§3.5.4`禁止跨域直接调对方仓储；当前`internal/architecture_test.go`的`TestArchitecture_ServiceDomainIsolation`实际阻止的是service根包与子域的直接import，不能识别平铺repository内的所有写表。它也明确允许`middleware.RoleFetcher`等横切接口引用，**service引用该接口并不是已经证实的HTTP反向依赖**。`TestGuard_TicketRepoListCallSites`只守特定列表调用，不能据此声称任意跨域SQL都已被守卫覆盖。

下文的“保留”表示有明确业务理由的现有协作；“窄接口”表示在消费方限制可调用能力；“建议登记例外”表示须在采纳时同步架构SSOT、源码注释和机械守卫，本文不自行放宽现行规则。接口可以接受显式DBTX以维持同一事务，不默认把每个接口变成远程API。后期拆分必须另交持久命令、收据、对账、数据迁移与回退，不能将本地tx直接穿过网络。

## IAM、组织与权限

### MB01 用户、角色和组织关系归IAM同一事务域

源码与合同锚点：[zhuzhao/internal/service/user_service.go · UserService.Create/Delete/SetRoles](../../internal/service/user_service.go)；[zhuzhao/internal/service/org_service.go · OrgService.SetUserOrgsTx](../../internal/service/org_service.go)；[zhuzhao/internal/repository/user_repo.go · UserRepo.SoftDeleteTx/SetRolesTx](../../internal/repository/user_repo.go)。

- **证据与现状**：`internal/service/user_service.go`的`UserService.Create`读取操作者/目标角色及组织，开启一个tx，依次调用`UserRepo.CreateTx/SetRolesTx`和`OrgService.SetUserOrgsTx`；后者交`OrgRepo.SetUserOrgsTx`。`internal/repository/user_repo.go`的`SoftDeleteTx`清`user_roles/user_orgs`、移除`organizations.owner_user_ids`并软删用户。`RBACService`、`MenuService`也直接消费IAM仓储。
- **事务与理由**：Create事务所有者是UserService；关系写入均接受同一tx。SoftDeleteTx由调用者事务持有，最后superadmin保护使用已有advisory guard。这些是IAM聚合命令，不需要为用户、角色、组织各造独立服务；拆成异步步骤会留下未分配身份或失效owner。角色/组织的部分授权预检在tx外，不能据“写同事务”宣称授权撤销竞争已经闭合。
- **处置与窗口**：保留同域仓储协作；G06/P6.1明确“IAM域”的例外边界，禁止借此让任意业务域获得整个IAM仓储。C04/P6.1把相关赋角、组织绑定、停用/删除纳入授权epoch入口覆盖；不为C04试点一次重排所有IAM文件。`s.orgService.orgRepo`这类内部字段穿透在触碰该用例时改为消费方窄查询，保留现有原子关系写入。
- **验收**：建用户后第二/第三步失败，用户与全部身份关系一并回滚；删除并发owner修改不复活已删身份；撤权与赋角按既定锁序只能产生合法串行结果。现有superadmin/身份集成测试是回归来源，本次未运行，不据源码断言并发安全已证明。

### MB02 组织移动与工单可见性投影是受控同库协作

源码与合同锚点：[zhuzhao/internal/repository/org_repo.go · OrgRepo.Move/Delete/DeleteVgWithOwnerCleanup/FindByIDForShareTx](../../internal/repository/org_repo.go)；[zhuzhao/internal/service/ticket/service.go · Service.Create](../../internal/service/ticket/service.go)。

- **证据与现状**：`internal/repository/org_repo.go`的`OrgRepo.Move`在一个tx中取`org:move` advisory lock、锁节点/父节点/旧子树，更新`organizations.path`，再更新`tickets.org_path`和`ticket_templates.org_path`。`ticket.Service.Create`在写票tx中调用`OrgRepo.FindByIDForShareTx`后保存路径。`OrgRepo.Delete/DeleteVgWithOwnerCleanup`锁组织并检查成员、子组织和未结工单；当前没有org_roles守卫。
- **事务与理由**：这里的`org_path`是当前可见性使用的冗余投影，不是不可变历史归属。移动与路径级联同事务、建单共享锁与移动写锁互斥，避免“票写旧路径”或旧票从新组织树静默消失。模板当前无同等查询消费者仍保留一致投影。组织删除检查在途票也是合法生命周期约束，不应仅为repo命名消除它。
- **处置与窗口**：建议登记此项受控跨域例外。C04/P6.1维持同库原子性和锁协议，将级联实现收为只更新组织路径投影的窄操作；是否搬SQL由同批可读性决定，不为抽接口拆成两个commit。org_roles删除/绑定两侧守卫归C04；D06的历史归属另存快照，不能把当前live路径当历史。E07仅在真拆组织或工单服务时替换为版本化投影/水位与授权停写政策。
- **验收**：保留`TestOrgRepo_MoveTxGuard/MoveConcurrentCrossMove`等现有源码样本；补移动与建单栅栏交错、票/模板级联任一步失败全回滚、删组织与绑角色只有一个合法结果。将来使用异步投影时，在投影不齐期间不得按陈旧路径放宽权限；该替代尚未获准或实现。

### MB03 PG授权事实、Casbin和Redis会话是不同提交边界

源码与合同锚点：[zhuzhao/internal/service/rbac_service.go · RBACService.AssignMenus/reloadPolicy](../../internal/service/rbac_service.go)；[zhuzhao/internal/service/session_revoke.go · revokeUserSessionsWithRetry](../../internal/service/session_revoke.go)；[zhuzhao/internal/repository/role_repo.go · RoleRepo.GetEffectiveRoleCodes](../../internal/repository/role_repo.go)。

- **证据与现状**：`RBACService.AssignMenus`调用`RoleRepo.AssignMenus`提交`role_menus/casbin_rule`，随后`reloadPolicy`刷新内存；失败重试并返回`ErrPolicyReloadFailed`。`UserService.UpdateStatus/Delete/ResetPassword`先写PG，再经`session_revoke.go`的`revokeUserSessionsWithRetry`写disabled、增加密码纪元并清refresh键。`RoleRepo.GetEffectiveRoleCodes`的组织角色种子联`org_roles/user_orgs`，未联组织status/deleted过滤。
- **理由与风险**：内存策略、Redis和PG不能由本地tx包成原子提交。现有错误/日志已明确“DB已提交、后继失败”，应保留诊断语义；不能回报整体无效果。组织停用并不自动等于当前所有派生角色撤销，新增HR也不能凭status字段假设这一点。这里只确认源码条件和跨介质窗口，没有动态复现任意越权。
- **处置与窗口**：C04/P6.1统一权限变化入口和业务写守卫，D06使用同一生命周期命令；C05收敛会话前后端，R01为不可丢撤销/重建任务留持久意图与对账。E09在选择多副本前验证跨实例策略刷新，不能提前声称Watcher已存在。保留PG写主与专门会话机制，不把所有缓存统一为同一个epoch。
- **验收**：PG已提交而Redis/LoadPolicy失败时明确可恢复、不能误示回滚；组织停用/软删失去该权源而独立用户角色仍保留；撤权先提交后旧业务不能提交。已有`TestRevokeUserSessionsWithRetry_*`只作为源码回归入口，本次没有重新验证运行结果。

## 工单、对象与事件

### MB04 工单用例拥有授权、字段和状态不变量

源码与合同锚点：[zhuzhao/internal/service/ticket/resource.go · Resource.Authorize/GetFilter、OrgDelegationChecker](../../internal/service/ticket/resource.go)；[zhuzhao/internal/service/ticket/type_admin.go · Service.validateCustomData](../../internal/service/ticket/type_admin.go)；[zhuzhao/internal/service/ticket/transition_service.go · Service.Transition](../../internal/service/ticket/transition_service.go)。

- **证据与现状**：`ticket.Service.authorize/resolveListFilter`经resource.Registry与RoleFetcher；`ticket.Resource`消费本包`ticketRepo/OrgDelegationChecker/CCChecker`窄接口。`Create/Update/Transition`拥有tx并调用`*Tx`仓储，部分票SQL仍在service。`type_admin.go`的`validateCustomData`接受查询器；`transition_service.go`用`LockTransitionSnapshotTx`保护状态/定义快照。
- **理由与风险**：角色接口和组织委托端口是已有的正确边界，不需先换SDK。定义、字典与字段校验是工单写不变量的一部分；tx内再从pool读可能占第二连接、看到不同版本，不能把它们随意远程化。service内同域SQL的存在不自动是越权，真正应禁止的是新入口绕过用例及空过滤器默认放行。
- **处置与窗口**：保留工单写主，C01/C04在P6.0/P6.1先修公式及显式DBTX试点；D01/D02/D03再落定义快照、字段/类型策略。查询/导出/AI入口都消费同一受权查询端口。只有实际触碰的SQL和过宽仓储依赖才收窄，不把“先全仓接口化”加入近期前置。
- **验收**：同一可见集用于列表/详情/导出/计数；未知action/空filter拒绝；池容量2仍可并发提交，失败不留半条事件。`TestValidationReadsReuseTransaction`与`TestTicketConflict_*`已有源码，后续改动须针对实际新交错补证。

### MB05 工单与附件落绑保留一个事务所有者

源码与合同锚点：[zhuzhao/internal/service/ticket/service.go · Service.Create/Update/bindAttachmentsTx](../../internal/service/ticket/service.go)；[zhuzhao/internal/repository/storage_repo.go · StorageRepo.BindCreateAttachments/ConfirmAndBind](../../internal/repository/storage_repo.go)。

- **证据与现状**：`ticket.Service.Create/bindAttachmentsTx`将显式附件、file及richtext引用交`StorageRepo.BindCreateAttachments`，共用工单tx，写`tickets/ticket_events/ticket_attachments/file_objects`。`Update`当前只把`req.Attachments`送落绑，未同规抽取更新后的全部file/richtext引用。`StorageRepo.ConfirmAndBind`另在已有票上传路径锁ticket→file并同事务落绑。
- **理由与风险**：文件字段内容与对象真实归属须一起成立；分成异步storage命令会留下引用已保存、归属未绑定的裂缝。对象存储HEAD/删除是网络效果，仍应在DB锁外处理。Create与Update不一致是真正已登记的C02问题，不能靠将`StorageRepo`改名为interface关账。
- **处置与窗口**：C02/P6.0建议用消费方窄接口承载“校验并落绑/引用对账”，显式传入ticket拥有的tx；保留同库受控例外。统一Create/Update/显式删除/GC锁序和引用元数据，上传B追加、移除字段不删A。若未来拆对象服务，必须先定义暂存认领/确认/补偿，当前不走此路线。
- **验收**：外单对象、别人暂存、错用途/未confirm拒绝；最后一个引用失败时整票回滚；并发双绑仅一票成功；GC先赢/绑定先赢均不误删；仍引用对象不能显式删除。已有`TestGC_TicketBindingWins`等并不证明C02新规则已经实现。

### MB06 头像原子引用与附件授权入口分别处理

源码与合同锚点：[zhuzhao/internal/repository/storage_avatar.go · StorageRepo.BindAvatar](../../internal/repository/storage_avatar.go)；[zhuzhao/internal/handler/storage_handler.go · TicketAuthorizer、StorageHandler.Confirm/ProxyDownload/DeleteAttachment](../../internal/handler/storage_handler.go)；[zhuzhao/internal/service/storage/service.go · Service.Download/BindAvatar](../../internal/service/storage/service.go)。

- **证据与现状**：`internal/repository/storage_avatar.go`的`StorageRepo.BindAvatar`锁users行再锁file_objects，同tx写`users.avatar`、对象owner及旧对象退役；storage.Service在提交后best-effort物理删除旧键。`handler.StorageHandler`持`TicketAuthorizer`，为confirm/list/delete及Download闭包调用工单授权；storage.Service本身不import ticket，Download在库内绑定关系定位ticket。
- **理由与风险**：头像联合事务防止GC删除仍被用户引用的对象，应作为对象引用协议的受控例外保留。当前授权由HTTP编排承担，适合已装配入口，但不能推导后台/AI直接调用storage.Service也自动有票权限；Download的可空回调尤其不是新调用方的授权保证。当前生产handler传入判定，本次未据该可空参数断言现有路由可越权。
- **处置与窗口**：头像事务保留，C02/C04在新增非HTTP调用前建立受权文件用例或必需的授权端口，拒绝用户态调用缺authorizer；对象基础读写不拥有角色政策。无需为了统一路径立即搬现有handler编排。C05只管会话，不能代替这项资源权限。
- **验收**：保留`TestBindAvatar_GCWins/BindingWinsGC/ConcurrentRebindRetiredKey`；新入口验证无权限、无授权端口、伪造ticket_id/对象key均无字节和写效果；物理删除失败能由原对象身份继续回收。

### MB07 提交后Hook、消息投递和必需后动作不可混为一层

源码与合同锚点：[zhuzhao/internal/service/ticket/actions_builtin.go · ReassignAction/SpawnTaskAction.Execute、TaskSubmitter](../../internal/service/ticket/actions_builtin.go)；[zhuzhao/internal/service/ticket/actions.go · ActionRegistry.RunPostActions](../../internal/service/ticket/actions.go)；[zhuzhao/internal/pkg/domainevent/registry.go · HookRegistry.Dispatch](../../internal/pkg/domainevent/registry.go)；[zhuzhao/internal/app/app.go · NewApp、taskSubmitterAdapter](../../internal/app/app.go)。

- **证据与现状**：`ticket.Service.emitTicketEvent`提交后调用`domainevent.HookRegistry.Dispatch`；后者启动goroutine，进程崩溃有丢失窗口。`app.NewApp`将ticket/ops订阅给`TicketNotifyHook`，并调用`InboxService.SetEventRegistry`登记inbox消费者。`Transition`提交后运行`ActionRegistry.RunPostActions`；`ReassignAction`调用另一个Assign事务，`SpawnTaskAction`经TaskSubmitter端口和`app.taskSubmitterAdapter`调用TR。
- **理由与风险**：事件避免service包互import，消息/站内信应消费业务事实而不掌管票状态。但Hook没有持久ACK；`ticket_events`也不是投递收据。当前转派失败不回滚已提交转换，与EXE-D01未来“必需本地状态+处理人同事务”仍有实现差距。TaskSubmitter已有窄接口但不因此成为可靠意图。
- **处置与窗口**：保留低一致性通知的进程内Hook与独立消息写主；C04/R01在D10/D12/A02等不可丢消费者前落同事务意图。必需本地动作进入ticket事务操作，独立通知/外部调用走持久后继，外部完成前提走待回执，不做统一“全部异步”。D11再扩消息通道和规则，不为命名`TicketNotifyHook`立即搬所有文件。
- **验收**：业务提交后kill、消费后ACK前kill、重复/迟到事件、订阅缺失及消息失败分别验证；结果投递失败不重做外部动作；转派失败按该用例不变量显示可解释状态。已有动作/通知测试不能替代R01崩溃恢复证据。

## 调度、跨仓执行与清单

### MB08 维护型扫描留主仓，业务调度归TR

源码与合同锚点：[zhuzhao/internal/service/cron_scheduler.go · CronScheduler.Run/tickOnce/runLocked](../../internal/service/cron_scheduler.go)；[zhuzhao/internal/app/app.go · NewApp/Run](../../internal/app/app.go)；[zhuzhao/internal/service/cron_registry.go · CronRegistry.Register](../../internal/service/cron_registry.go)。

- **证据与现状**：`internal/service/cron_scheduler.go`的`CronScheduler.Run/tickOnce/runLocked`用独立PG连接持session advisory lock，再调用`ReentrantJob.Handle`；业务job自己管理事务，锁连接不等于共享tx。`app.NewApp/Run`装配并启动audit_archive、storage_gc、timeout_scan、inbox/intel清理和用量检查；CronRegistry只聚合状态。`standards §3-15`已规定平台内务与业务任务分工。
- **理由与风险**：GC/超期扫描需要本地数据库和领域约束，迁去TR反而增加故障/重复执行边界；业务connector拉取和需要可见运行/重试的任务不能继续塞进维护闹钟。注册/状态可见不等于goroutine已启动；多个调度源同启也不受同一把本地锁保证。
- **处置与窗口**：保留现有分工。C03先修TR稳定发生次；D06/X02的真实周期采集由TR唯一调度，主仓只接feed或维护对账。I05/P7提供两源只读总览，不建第三调度器；G03验证实际装配与启停。不得以本页为由增加新业务cron到主仓。
- **验收**：保留`TestCronLockPool_*`源码反例；验证已启用任务实际进入Handle、取消/panic释放连接，GC与业务写不死锁；同数据集只能有一个周期writer。未来总览未知next显示未知，不能自己重算后改变执行事实。

### MB09 主仓提交凭证与callback业务完成不是TR执行库

源码与合同锚点：[zhuzhao/internal/service/taskrunner_service.go · TaskrunnerService.Submit/Trigger](../../internal/service/taskrunner_service.go)；[zhuzhao/internal/service/jobs_callback.go · JobsCallbackService.Execute](../../internal/service/jobs_callback.go)；[zhuzhao/internal/repository/job_submission_repo.go · ClaimCallbackRow/MarkSucceeded](../../internal/repository/job_submission_repo.go)。

- **证据与现状**：`TaskrunnerService.Submit/Trigger`先HTTP调用TR，再写本地`job_submissions`，记账失败留Error但返回受理结果。`JobsCallbackService.Execute`通过`ClaimCallbackRow`认领，比对action/params，再调用jobs.Handler，最后MarkSucceeded/Failed；未认领的running也返回幂等结果。`pkg/taskrunner.Client`承担签名/HTTP，不让业务service直接感知HTTP。
- **理由与风险**：主仓拥有本地动作效果、授权与凭证，TR拥有执行投递，两库不直连。callback栅栏只能拒过期写终态，不能撤销旧worker已发生的副作用；running的2xx又会被TR解释为完成，故“无import耦合”并不表示完成语义正确。当前受理后本地记账没有跨库原子性。
- **处置与窗口**：C03/P6.0收敛accepted/running/completed/unknown与receipt；C04/R01提供本地动作的事务/幂等和持久交接，D12再消费结果。保留TaskSubmitter适配与两库写主；不采用分布式事务，不让TR直接改ticket或风险表。I02前actor/operator只作为归因，不能据`action:<id>`字符串代替实时授权。
- **验收**：TR已受理而主仓记账失败、callback运行中重复到达、业务成功后MarkSucceeded失败、租约接管后的旧writer均可查询/恢复；unknown不盲重做不可幂等动作。现有`TestJobsCallback_SnapshotMismatchRejected/RealSubmitThenCallbackSemanticParams`仅覆盖其中部分事实。

### MB10 TR自己的DB、Redis和发生次也有边界

源码与合同锚点：[taskrunner/internal/service/submit.go · Service.Submit](../../../taskrunner/internal/service/submit.go)；[taskrunner/internal/service/cron.go · Loop.FireDue](../../../taskrunner/internal/service/cron.go)；[taskrunner/internal/worker/worker.go · Deps.handleCallback](../../../taskrunner/internal/worker/worker.go)；[taskrunner/internal/callback/client.go · Client.Do](../../../taskrunner/internal/callback/client.go)；[taskrunner/internal/repository/store.go · Store.MarkRunning/Finish](../../../taskrunner/internal/repository/store.go)。

- **证据与现状**：taskrunner的`service.Service.Submit`先`Store.InsertPending`再Asynq入队与MarkQueued，ReclaimLoop补queued=0；`Loop.FireDue`生成新task_id，提交后单独推进next_run。`worker.Deps.handleCallback`先MarkRunning，部分DB错误仍继续回调；`callback.Client.Do`将2xx视作完成、4xx不重试、其余重试，`Store.Finish`以execution_id栅栏写终态。
- **理由与风险**：DB-first是受理保障，不是DB/Redis原子性或业务exactly-once。推进next_run失败用新task_id重触发、响应丢失和缺行仍执行业务都需要独立处理。主仓业务不得通过读TR库修补这些状态，反之TR不能把业务结果猜成ticket状态。
- **处置与窗口**：保留独立服务/PG/Redis；C03稳定job+revision+UTC slot、严格认领和完成收据，I03补恢复/取消/预算/观测。只在声明可幂等的动作上重投。未有运行证据不把TR升级成SOAR或拆更多worker服务。
- **验收**：双副本同slot、next_run推进失败、MarkRunning失败、回调成功响应丢失、队列键过期和旧execution完成均有确定性反例；同一发生次不新增业务效果，DB/Redis重启后账实可对齐。现有delivery fence源码不是上述整链已成立的证明。

### MB11 AL拥有自己的Schema、动态表与replace事务

源码与合同锚点：[activelist/internal/service/type_service.go · TypeService.Register/Evolve](../../../activelist/internal/service/type_service.go)；[activelist/internal/service/data_service.go · DataService.Insert/Update/Import](../../../activelist/internal/service/data_service.go)；[zhuzhao/internal/router/router.go · New网关挂载链](../../internal/router/router.go)；[zhuzhao/internal/gateway/gateway.go · Registry.Mount](../../internal/gateway/gateway.go)。

- **证据与现状**：activelist的`TypeService.Register`同tx写meta、动态表与history；`Evolve`使用version CAS和history。`DataService.Insert/Update/Import`拥有本库tx，Import锁类型动态表、删除旧内容、分批解析插入并更新序列后commit。主仓`gateway.Registry.Mount`经过JWT、Casbin、身份断言和AK/SK；AL侧验签，operator用于归因，没有用户角色/资源权限模型。
- **理由与风险**：AL已有独立交付和数据机制边界，不需要再合回主仓。全量replace是AL清单语义，不是配置晋级、风险迁移或HR同步；借其导入能力直接装本地领域正本会绕开领域约束。网关L1也不能自动提供AL任意行/字段的L2政策。
- **处置与窗口**：保留薄清单/映射职责；D07/P6.2的受权查询与R06/P7的字段/行范围、导入EOF/世代/预算使用主仓领域门面或明确受信过滤合同，AL不复制IAM。风险、身份、证据正本不放动态表；跨库写永不承诺同tx。
- **验收**：未授权类型/列/行不由网关反代泄露；replace途中失败旧集合完整，成功后旧revision/ETag不复活；尾随/多顶层JSON、重复键、序列、schema竞争及预算一致分别验。已有AL真PG测试保留，本次未运行或推断所有目标已通过。

## 尚未实现的领域与接缝

### MB12 HR归一化、IAM生命周期和业务交接分工

源码与合同锚点：[zhuzhao/internal/service/user_service.go · UserService.UpdateStatus](../../internal/service/user_service.go)；[zhuzhao/internal/service/org_service.go · OrgService.Update/Move](../../internal/service/org_service.go)；[zhuzhao/docs/design/business-lifecycle-contracts.md · §7目标DirectorySync/交接合同](business-lifecycle-contracts.md)；[zhuzhao/docs/design/intranet-adapter-migration.md · §2–6目标batch/checkpoint/ACK](intranet-adapter-migration.md)。

- **证据与现状**：主仓受控源码没有HR DirectorySync服务/connector；现有的是人工IAM命令、组织写侧与撤会话。目标在`business-lifecycle-contracts.md §7`和`intranet-adapter-migration.md §2–6`：来源身份/完整快照/批次与ACK、字段权威、离职与撤部plan。
- **边界裁定**：私有connector拥有厂商协议、来源读取游标及可重投批次；公共接收用例归一化并调用IAM生命周期命令，拥有本地接收记录与IAM事务。源游标不能先于可恢复批次，核心ACK不能先于提交。停用账号安全动作不等待票/风险交接；批准映射只指定接管目标，不授权限或改历史创建人。
- **处置与窗口**：D06/P6.1先做合成端口/生命周期；X02同窗参考connector验证，P7/E01逐真实提供方。按MB01–03复用IAM，不让connector直写users/user_orgs/组织路径或自行删票。交接按逐领域命令和receipt执行，不能让HR模块为一次全员同步持所有工单锁。
- **验收**：部分快照不触发缺失离职；重放/乱序不覆盖新身份；停用后撤权失败可见可恢复；批准映射逐项重验active/权限/revision，无法直接交接保留待处理。真实字段和沙箱未知仍real_pending，不据本次静态设计宣称已接HR。

### MB13 风险与工单桥保留本地原子性，不共享任意仓储

源码与合同锚点：[zhuzhao/docs/design/business-lifecycle-contracts.md · §2.3、§8.4目标RiskTicketLink/Tx协调](business-lifecycle-contracts.md)；[zhuzhao/internal/service/ticket/service.go · Service.Create/Assign现有用例](../../internal/service/ticket/service.go)；[zhuzhao/docs/phase6/03-r3-ticket-linkage.md · R3目标N:1整改单桥](../phase6/03-r3-ticket-linkage.md)。

- **证据与现状**：没有已实现的风险service/repository/风险核心表。`business-lifecycle-contracts.md §8`计划风险family/cycle、observation、接受与RiskTicketLink；§8.4要求group内建票/links/风险状态/双域事实同事务；§2.3规定显式用例端口与协调，不直接互调对方repository。
- **边界裁定**：风险拥有本地生命周期、owner、接受/复测及link，工单拥有字段/状态/处理资格，来源拥有观测结论。主仓同库首期由一个桥接用例协调，锁序遵授权守卫→group→风险/工单/link；ticket提供受限建票/参与/完成操作，risk不获TicketRepo全量能力。新增票的created_by/actor/风险owner各司其职。
- **处置与窗口**：D08/D09/D10在P6.2及相关范围冻结前列所需Tx接口、锁序和唯一约束；实现时C04/R01先可用。风险终态消费票结果只认具名意图/结果版本，不以“外部SOC建议”自动关风险。将来E07拆分才改持久命令/最终一致，首期不为了服务化取消N:1分组原子性。
- **验收**：创建票后link失败全回滚；并发同风险仅一active link；分组剩余名额竞争、转owner、旧终态回执均不越界；已关闭风险不被迟到结果复活。真实provider不影响这些本地合成反例先做，风险功能当前仍未实现。

### MB14 DataFetcher与情报Provider是已有端口，能力范围不同

源码与合同锚点：[zhuzhao/internal/pkg/datafetcher/datafetcher.go · Registry.Fetch/ApplyFetches](../../internal/pkg/datafetcher/datafetcher.go)；[zhuzhao/internal/app/providers.go · provideApp空Registry/Intel Provider装配](../../internal/app/providers.go)；[zhuzhao/internal/pkg/threatintel/threatintel.go · Provider/NewProvider](../../internal/pkg/threatintel/threatintel.go)；[zhuzhao/internal/pkg/threatintel/service.go · Service.Query](../../internal/pkg/threatintel/service.go)。

- **证据与现状**：`datafetcher.Registry`支持注册/Fetch，providers给ticket注入空Registry，没有真实源注册。`ticket.Service.applyFetches`解析字段映射，`datafetcher.ApplyFetches`使用Background context，未映射/失败仅missed；这不是HR同步。`threatintel.Provider/NewProvider`已有stub/HTTP实现，`threatintel.Service.Query`直接在主仓缓存表读写与singleflight，远端调用不属于业务写tx。
- **理由与风险**：查询/预填与权威同步应分开；空Registry不是已对接资产/HR。当前情报Provider的通用HTTP协议也不证明真实厂商语义已验。`internal/pkg/threatintel`含持久缓存，属于主仓私有能力，不能因目录叫pkg自动下沉utils；同理missed预填不能充当必须权威字段的成功凭证。
- **处置与窗口**：D07/P6.2及X02先传递有界caller context、白名单映射和来源收据，远端取数在短业务tx外，入tx再验定义/权限/必要值；保留mock和可失败预填语义。I04/P7验证具体情报/CMDB供给。只在实际扩源时收窄查询端口和缓存归属，不先搬全目录。
- **验收**：缺源、超时、撤权、源结果含越权字段、映射陈旧和手填值冲突分别有结果；不能因取不到值回退成有权或伪造成功；取消请求不留下无界远端调用。真实情报来源/语义仍单列待验。

### MB15 AI与私有扩展消费业务用例，不接管领域事务

源码与合同锚点：[zhuzhao/internal/gateway/gateway.go · New/Mount当前不可变代理](../../internal/gateway/gateway.go)；[zhuzhao/docs/design/ai-invocation-contract.md · §3–5目标AI调用/授权/检查点](ai-invocation-contract.md)；[zhuzhao/docs/design/extension-compatibility.md · §3–6目标manifest/目录/身份/兼容](extension-compatibility.md)。

- **证据与现状**：源码没有LLMProvider、Agent host、MCP adapter、extension apply或通用feed实现。现有`gateway.New`只按配置构造不可变反代；菜单/API目录由迁移与启动守卫维护。目标分别见`ai-invocation-contract.md §3–5`、`extension-compatibility.md §3–6`和内网适配合同。
- **边界裁定**：模型产生建议，host拥有运行检查点/预算，ticket/risk拥有最终业务写与操作receipt。A02当前用户确认后调用同一ticket用例，模型/外部Agent不接收浏览器凭据；后台代人和持续任务等待I02 grant。私有扩展进程外独立数据，只能用受权HTTP/能力端口；同步身份/导航/目录一次性接缝由核心拥有，不import core internal或写core表。
- **处置与窗口**：X01在P6.0定最小身份、目录来源和注册原子边界，P6.1实现；A01/A02沿受权查询/建票切片推进，准确MCP授权/工具合同另按FD05补研究。Agent/skill具体场景留A05/A07，不让它们阻塞工程隔离、CI、工具链、U01或ticket正确性。公开core不依赖私有SDK，真实provider切换使用版本/receipt矩阵。
- **验收**：模型伪造actor/绕过确认/直接给对象key均无写权；确认后撤权或定义变化拒绝；unknown先查原receipt而非重建票。参考扩展升级/卸载只改自己目录绑定、失败保持旧活动配置；添加扩展无核心源码diff。当前没有这些机制的运行证据。

## 装配、配置与公共库

### MB16 生命周期与纯机制不得成为隐形业务中枢

源码与合同锚点：[zhuzhao/internal/app/providers.go · provideApp](../../internal/app/providers.go)；[zhuzhao/internal/app/app.go · NewApp/Run/Shutdown](../../internal/app/app.go)；[zhuzhao/internal/service/settings_service.go · SettingsService.Update/applyAllOverrides](../../internal/service/settings_service.go)；[zhuzhao/internal/architecture_test.go · TestArchitecture_LayerDependency/ServiceDomainIsolation/PkgNoBusinessDependency](../../internal/architecture_test.go)；[zhuzhao-utils/docs/api/platform-contracts.md · §1、§4–6公共机制/版本边界](../../../zhuzhao-utils/docs/api/platform-contracts.md)。

- **证据与现状**：`app.provideApp/NewApp`集中实例、动作、资源、网关与订阅装配；`App.Run/Shutdown`启动维护任务、关HTTP、排空policy writer/Hook后释放依赖。`SettingsService.Update`提交system_settings后重建共享`storage.ConfigStore`快照，含单实例假设。utils仅承载签名/JWT/错误/PG/Redis等机制，四仓不同pin并存。
- **理由与风险**：明确装配集中是合理的composition root，不等于运行时插件系统。对象配置快照保证同进程读一致，不自动传播到另一副本；Hook排空只缩小计划重启丢失窗口。PG构造器不该拥有ticket/risk的tx，签名operator也不等于授权。
- **处置与窗口**：G03/P6.0与G06/P6.1将声明的订阅/启动/规则清单和当前边界例外变成实际装配检查；保留现有横切接口例外和break-glass独立入口，不重复造框架。E09选多副本前验证配置/策略传播与消费者互斥；R07/P7只有两个真实消费者证明通用性才抽机制，不把Action政策、私有provider或运行状态塞utils。
- **验收**：移除一个必需订阅或未启动已启用任务应失败；关闭顺序下未落行可定位，其他扩展失败不拖垮非强依赖核心；配置CAS失败不改快照，多副本实际读取同版本。当前guards偏import/命名启发，G06应验证写表能力与允许例外而非只数包数。

## 对近期排期的影响与验证边界

1. **可直接继续**：G01隔离、G02/G03合入与组合CI、G07A工具链、U01入口修复。本文不增加“先全仓拆包/改接口”的前置。
2. **P6.0先冻结的最小边界**：C02附件联合事务与引用语义、C03双库受理/完成/发生次、X01同步身份/目录来源；避免实现者先按“禁止任何跨域仓储”拆掉必要事务。
3. **P6.1落实而非只换接口名**：C04授权/DBTX试点与组织生命周期，G06将MB01同IAM域及MB02/05/06受控例外同步到架构SSOT和守卫；D06/X02使用同一IAM命令及单writer/ACK合同。现有包内估算是否足够由实施切片复估，本文未暗中扩充原粗估。
4. **按业务窗口深化**：D08–D10冻结risk-ticket桥、D07/R06受权查询/AL边界、R01/D11/D12可靠后继；MCP/Agent/skill及真实provider继续各自窗口，不反向阻塞所有近期任务。
5. **E07保留后期拆分裁定**：只有实际独立发布、容量、故障隔离或维护责任证据成立才选服务；当前同库例外不等于将来可以跨库共享tx，也不是永久承诺共享表。

本页验证为源码/现行合同回读、静态库存、路径与符号核对。没有运行Go测试、acceptance、迁移、真实对象存储、TR/AL联调或私有provider，也没有改变远端、依赖、账号和运行栈；不能把文档补研究写成新功能验收。旧测试只提供可继承的回归入口，各实施PR仍遵现行完整门禁及“改动摘要/影响面/验证证据”要求。规划分支尚未推送/合main，分档门禁未启用；最终集成者另报告本批实际检查和未覆盖范围。
