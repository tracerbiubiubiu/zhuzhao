# 跨仓执行合同 — 完成语义、稳定时隙与最小可靠交接

日期：2026-11-11。状态：**C03/R01 待实施详细设计**。与[工程施工设计](engineering-execution-design.md)共同承接[唯一总账](../review/planning-backlog.md)；不新增 Phase、不另算一份工期、不预占迁移编号。本文建议的类型/字段/路由全部明确属于目标，现有符号以“现状”段列明。

边界：zhuzhao 拥有本平台领域事实、授权、操作意图和业务效果；taskrunner 拥有有界胶水任务的调度、投递、取消/重试与执行账本；外部专业平台拥有自身工作流/资产/告警/情报事实。taskrunner 不是通用 SOAR，AL 不是业务操作权威或事件湖，utils 不承载领域表。外部 Agent、领域 Agent、内网 connector 均只通过[稳定扩展合同](extension-compatibility.md)和[适配迁移合同](intranet-adapter-migration.md)消费本页，不 import 主仓 internal/直写其库。

## C03 实际链路与不能混写的四种事实

现状源锚点：

- 主仓 `internal/service/taskrunner_service.go` 的 `TaskrunnerService.Submit/Trigger` 先调用 taskrunner 再 `JobSubmissionRepo.RecordSubmit`，凭证失败只记录日志。`internal/repository/job_submission_repo.go` 的 `job_submissions` 兼作提交日志与回调栅栏；`ClaimCallbackRow` 原子认领 submitted/failed 或超过10分钟的 running，`claimed_at` 是旧写者 fence，原始 action/params 快照保留。
- 主仓 `internal/service/jobs_callback.go` 的 `JobsCallbackService.Execute` 比对快照后，未 claimed 的 succeeded **和 running** 均返回 `CallbackIdempotent`；`MarkSucceeded` 失败或 fence 不匹配仍返回成功。`internal/handler/jobs_handler.go` 的 `JobsHandler.Callback` 把 Executed/Idempotent 都映射200。Handler已执行而记账失败不能被笼统写成“业务没做”，也不能因此声称两端完成事实相同。
- TR `internal/callback/client.go` 的 `Client.Do` 把任意2xx当完成，4xx当不可重试，其他/超时退避；不解析响应业务状态。`internal/worker/worker.go` 的 `Deps.handleCallback` 为每次 delivery 生成 execution_id 并调用 `Store.MarkRunning/Finish`；除 terminal/superseded 外，MarkRunning缺行/DB失败仍会调用下游，Finish故障主要留日志。DB-first前提下这些不能作为新动作安全默认。
- TR `internal/service/submit.go` 的 `Service.Submit` 已先持久化 pending payload，再入 Redis，`queued=0` 可由 reclaim 恢复。它解决 TR 自己的 DB→队列，不覆盖主仓 commit→远端提交。`internal/service/cron.go` 的 `Loop.FireDue` 读 due定义、生成随机ID、Submit，再独立 `UpdateJobNextRun`；后者失败或双副本扫描可能对同一 due 时隙起第二个业务任务。

目标分开记录：

1. **业务意图已提交**：主仓 DB 事务有持久凭证，尚不保证任务入队或业务效果。
2. **delivery已受理/仍运行**：TR到主仓的网络投递状态，与attempt/execution_id相关。
3. **效果状态**：未执行、执行中、已确认成功、已确认无效果的失败、效果未知。
4. **记账/对账状态**：结果凭证是否持久化、双方是否一致、是否需人工核实。

task_id 是同一业务执行的稳定身份；delivery_id/execution_id 是每次投递身份；request_id 只追踪请求，既不是幂等键也不是授权。重试保留 task_id，新的人工“再次执行”必须新操作号并显式关联旧号。任何网络超时都不能单凭超时证明对端没生效。

## C03 目标数据与接口合同

### 主仓执行凭证

保留 `job_submissions.task_id/action/params` 及既有查询兼容列，以 additive migration 增执行/结果字段或分离一对一 receipt 表；实施时查权威迁移表，不在本文分号。目标逻辑字段：

- `protocol_version`、`params_digest`、`action_version`、`operation_id`、`intent_id`（可空，R01接入后填）；原 params的null/空对象及键序兼容保留，目标规范化使用保真数字解析，不能把大整数经float64舍入后当相等。ID仍遵守项目string合同，未知重复键/非JSON输入拒绝，不用原始JSON字节顺序当合法回调的幂等依据。
- `execution_state` = `not_started/running/succeeded/failed_no_effect/outcome_unknown`；`reconcile_state` = `consistent/pending/manual_required`。旧 status 为兼容投影，不能把 unknown 投影成 succeeded；对旧只识四态的管理页附“结果待核实”详情，生产切换前升级读端。
- 单调 `claim_generation`、`lease_until`、`last_heartbeat_at`、`delivery_id`；目标新 fence 是任务内递增代次，不再仅靠时间戳。默认 lease120秒、heartbeat30秒，动作超时有上限；DB时钟判断过期，client时间不作授权或租约真值。
- `effect_receipt_id`、`result_schema_version`、有界 `result_summary/result_ref`、`error_category`、`completed_at`、`reconcile_due_at`。result不保存模型思维链、原始凭据或整个外部响应；仅白名单业务输出，下载另走授权。

`ActionMeta` 目标增加支持协议/结果schema/超时/`effect_mode`，仅三种可自动运行模式：`transactional`（领域写与receipt同DB事务）、`idempotent_external`（外部按operation_id持久去重）、`observable_external`（可查确切结果，未知时先查再决定）。无法幂等或查询的外部动作默认 `manual_only`，不自动重投。所有者不是要在平台重建下游业务引擎；这里仅管理调用证据。

现有 `jobs.Handler.Handle` 不能自动升级成事务安全。建议新增内部 `Execute(ctx, ExecutionContext) → ExecutionResult,error` 适配接口，Context提供稳定operation_id/claim_generation/受限主体，不让handler自行从请求拿数据库连接或伪造success。原handler显式标 legacy；逐个盘点可重入性后包适配器。transactional动作由领域service在同tx写业务、审计、receipt；不得先提交效果再补“同事务”的注释。涉及外部网络不可持DB行锁等待；先落操作意图，再按下游幂等/查询合同执行。

transactional效果凭证对`(owner_service, operation_id)`唯一，action_version/params_digest为同键一致性断言，不以换版本绕过同操作去重。副作用事务依次取得授权守卫、execution claim行锁、领域锁，锁内复验claim_generation/当前权限，再写效果与receipt并提交；续租/新代认领也必须竞争同claim行，不能在旧效果事务仍持锁时越过它。查不到未提交receipt只说明尚不可见，不证明旧事务无效果；新认领须等待旧事务终结并锁内复核唯一receipt/代次。外部observable查询的not_found或最终一致空结果同样不是无效果证明；只有提供方可核验的终态未执行/撤销凭证，且旧请求已不可能迟到生效，才允许重新发非幂等请求，否则保持unknown/人工处置。

### 认领和完成

认领先核服务授权、动作/params/版本/subject快照，再用条件更新取得 generation。DB错误或缺必要凭证不发副作用，TR相应MarkRunning失败也不得继续调用。凭证缺失的旧残留进入隔离对账队列，人工按来源决定补录或丢弃；不能以兼容历史为名默认无账执行。

API一次性提交目标改为先按既有可选 `TaskSubmitInput.TaskID` 生成/取得稳定任务号，持久化 `prepared` 提交快照再调用TR；本地持久化失败零远端调用，远端超时保留 `submission_unknown` 并按同task_id查询/重提，不能换号。TR同号不同action/params/owner_service必须409，不沿用当前“查到ID即幂等成功”的宽松行为。`Trigger` 目前由TR回带真实action/params，目标升级为携带稳定operation_id并原子冻结definition_revision；主仓凭受认证的TR回执补全快照，不再回退合成 `trigger:<job_id>` 接受无法核实定义的成功。cron首到允许创建凭证，但必须有受认证的occurrence/definition_revision、固定owner_service和已授权的动作策略，不能把未知服务随意提交的body当平台授权。C03只修单次提交/执行凭证；由领域事务自动产生并持续派发命令的能力归R01，避免两套poller重复发送同一任务。

- 已 succeeded：返回原receipt，重复不执行。
- 正在running且租约有效：返回 in_progress，不转 completed，不执行第二次。
- 租约过期：先查domain receipt/外部结果；transactional已提交返回完成，确认无效果且动作允许重试才取得新代；无法确认则置outcome_unknown并停止自动副作用。不能“10分钟到了”就推定旧进程不会继续生效。
- Handler成功但 `MarkSucceeded` 失败：transactional动作从同事务receipt重建；外部动作凭稳定operation_id查询确认，再补记。查询/DB不可用则返回结果未确认，保存/重建对账项；若连unknown写入也失败，由已存在running租约与定时扫描必然找回，不依赖临时内存列表。
- fence失配：旧写者不得改新代结果；能查到终态才回completed，否则回当前pending/unknown。旧写者“我已执行”不是新代完成凭据。非幂等外部下游不识fence时，lease不能阻止已发请求生效，因此不能只靠generation获得exactly-once。

`failed_no_effect` 必须有事务已回滚、请求尚未发出、或下游明确证明未执行的证据；普通error/ErrAbort、进程panic、timeout本身都不够。旧handler若可能部分成功后报错，适配结果默认unknown，不能为了继续沿用4xx语义把它标为可安全重试。事务commit断链也先query操作receipt，只有确认未提交才进入新执行。

取消与重试沿现有TR pending/failed/dead限制，补充业务效果分类：pending且未开始可取消；running只提交cancel_requested，只有可证明未生效/执行器确认终止才标canceled；外部效果unknown绝不能以“已取消”覆盖。重试failed_no_effect允许同task_id新delivery；unknown必须先reconcile。人工强制再执行需新operation_id、展示重复风险、现有L1/L2/L3权限与审计，不能复用普通retry按钮旁路。

### HTTP 兼容与 taskrunner 账本

保留 POST `/internal/jobs/callback`，AK/SK服务身份与固定目标地址不变；目标新增 POST `/internal/jobs/status`（task_id在body，避免依赖当前HMAC未覆盖的query值）和管理侧受控 reconcile操作。status只给有该owner_service权限的服务主体，按task_id+owner_service查询；禁止任意AK读取所有任务。actor/source_ip用于归因，不是用户授权凭证；R01意图带经平台签发/服务器查回的scope，生产动作不能信任回调body自报actor。

v2请求除现有body外增 `protocol_version=2`、`delivery_id`、`operation_id`、`action_version`；能力协商与版本固定来自组合manifest和服务配置，不接受未经认证client临时降级。响应仍沿用标准 `{code,message,data,request_id}`，data有 `task_id/state/receipt_id/retry_after_ms/result_schema_version/result_ref`，只有succeeded必须含持久receipt。HTTP语义：

- 200：已持久确认succeeded，含原receipt；幂等重放亦然。
- 202（仅协商v2）：in_progress或outcome_unknown，state明确区分；TR解析为“未终结”，不FinishSucceeded。in_progress按Retry-After有界poll，unknown进入reconcile不重新Handle。
- 409：快照不一致/业务明确拒绝且确认无效果等不可重试，stable reason区分；401/403服务凭据/授权错误隔离告警，不当普通业务retry；404未知动作版本进入配置失败。
- 429/503：临时不可用，可退避查询；无完成凭证不能回200。无效/截断/未知schema响应即protocol_error，不默认为成功；结果体有大小上限。

v1客户端不认识202，故主仓对v1 running/unknown用503并明确非完成，绝不202。真实旧core只有task_id/status响应，没有新receipt/status查询；v2 TR调用该格时只能标`legacy_observed`并保留旧队列投影，不能声称持久业务完成。已知running/unknown不认完成，不开放新自动动作。严格receipt模式必须配有receipt/status能力的core，否则manifest拒绝启用。确需混合运行时执行四格core N/N−1 × TR N/N−1回归，含真实旧client任意2xx逻辑与上述受限/拒绝结果，不要求不可能的旧端receipt；当前未正式部署可选择五仓协调替换，不建设长期v1桥。

TR保持 `job_runs` 现有status与execution fence，目标新增独立 `business_outcome/accounting_state/receipt_id` 和必要 `reconciling` delivery状态；UI不再把队列succeeded等同领域完成。callback Client改返回结构化outcome；严格模式worker只有持久receipt可写业务succeeded；legacy格的队列succeeded只可对应business_outcome=legacy_observed，不享新保证。Finish失败时不丢弃凭证，返回可重试的记账错误，下次先向status查询；query完成不再发新副作用。Redis retry budget耗尽只意味着delivery/dead_letter，不把领域outcome_unknown改failed_no_effect。对已证实receipt的重试可只补账，日志记录“业务已完成、记账待收敛”。

### 升级/回退

1. additive DB与只读状态投影先部署，旧列保留；清点现有running/failed，不能将历史succeeded无凭证伪造为新receipt。历史终态标legacy_observed，保留原证据来源。
2. 当前未正式部署，推荐清点开发在途后五仓协调替换，只需候选组合和必要开发数据恢复；若实际需要混合升级，先部署core的receipt/status兼容能力，再升级TR严格模式。新TR对真正旧core仅legacy_observed且禁新自动动作，不能凭allowlist凭空补出receipt。
3. 在途任务固定协议/action_version；确有混合消费者时适配层保留到其在途清零且约定迁移窗口结束，本轮不设6个月保留。切换后不能让旧TR执行v2任务；manifest拒绝这一组合，而非强迫所有私有adapter改代码。
4. 回退先停新提交，保留兼容reader/worker完成已接收任务；未理解unknown/receipt的新状态不能交给旧worker自动执行。数据库expand不做破坏性down；不支持安全回退的动作保持禁用并按operation_id对账。v1用503会耗尽旧TR预算，必须把此后delivery失败与业务待核实并列展示。

以上新增错误/字段/权限在实现PR同步Swagger/三仓合同测试，旧客户端未知可选字段可忽略，新增必需能力必须协商；禁止重定义原“成功”的字段含义却沿用原协议号。

## C03 稳定 cron occurrence

目标每次计划触发有 `occurrence_key=(job_id,schedule_revision,scheduled_for_utc)`；task_id采用该键的固定namespace UUID或等价无碰撞稳定编码，数据库UNIQUE仍为最终守卫。`schedule_revision`只在cron/spec/timezone或显式重排时间线时递增；普通停启/改补发策略仅改政策修订，不能创建新逻辑时隙身份。params/action用具有生效时间的definition_revision，按目标时隙选择并冻结快照，不能重写已起occurrence。manual trigger保持调用者operation_id，不与cron时隙碰撞。

TR repository新增 `ClaimDueOccurrence(now, limit)`：PG短事务按due行`FOR UPDATE SKIP LOCKED`，从当前持久next_run与政策/定义生效历史生成有界selected/skipped/deferred时隙计划，而非固定选最早一个。合并选真实最新遗漏时隙，逐次补选本批明确时隙；原子插入选中的pending job_runs/occurrence/参数快照、跳过区间回执与恢复批次预算，条件推进游标后commit，之后才向Redis enqueue。游标不越过未持久化的deferred范围；若另存恢复水位也须同事务可重建，不能清空旧积压。枚举/事务预算不足时暂停/延后，不猜最新时隙。Redis失败沿既有queued=0 reclaim恢复，不重新创造task_id。推进失败整笔回滚，下次仍使用同一时隙键。SQLite开发模式用单写事务/条件更新提供相同唯一合同，不使用其不支持的PG锁语法；多副本生产首选PG。PostgreSQL明确SKIP LOCKED适用于队列表而不提供通用一致读：[官方SELECT文档](https://www.postgresql.org/docs/current/sql-select.html)。

**按2026-11-11所有者反馈修订候选**：不同任务需要不同补发方式，创建/编辑计划时提供`misfire_policy`，不再建议全平台固定“最多补一次”。三种有界策略为：

- `skip`：超过正常迟到容忍时间的历史时隙不补，从下一次正常计划继续；例如过时的会议提醒。普通tick轮询导致的短暂迟到不等于错失，必须有显式grace/迟到阈值。
- `coalesce_latest`：将符合窗口的遗漏时隙合并为一次，选**真实最新遗漏时隙**作为scheduled_for，适合刷新当前设备/风险状态；其它被合并时隙记录原因，不能使用恢复时的now伪造新时隙。此候选替换旧稿“补最早一次”，不是已批准存量切换。
- `catch_up_bounded`：逐个回补符合窗口且尚未物化的时隙，适合按业务日生成历史报表。配置最大回补年龄、每次恢复积压的总数量、每批数量、并发/顺序和速率；限额不能每tick归零。超限余项明确暂停待处理或按已审策略跳过并留回执，不能无限补或静默丢。

计划保存策略/配置版本，并展示预计补发次数/范围；可选项受Action能力与权限限制，不允许把不支持历史数据/不具幂等效果的动作强行设逐次回补。无选项值的新计划需显式采用模板推荐并展示，不能以全局隐藏默认替用户决定。旧计划先清点并保留原行为，切换前预览范围和影响；系统维护类扫描按当前状态收敛，不套成逐日重发旧提醒。

回补传scheduled_for、业务时区和数据区间。比如补11月8日日报，必须读取11月8日可回溯数据；数据不存在就报告无法回补，不能查今天现状冒充历史。跨Action/params修改时按明确生效版本历史取对应快照；缺少旧版本则要求回补预览并批准指定版本，不能默用最新参数。补发只处理遗漏时隙；已有failed/unknown任务分别走原ID的执行重试/对账，不再造一次效果。当前授权/grant仍复验，过期授权不会因补发恢复。

改策略仅改变调度政策修订，不单凭递增schedule_revision让已执行逻辑时隙再次取得新身份。cron/timezone变更与启停需明确cutover barrier和新起点，旧积压保留原身份/快照；跨旧新revision的已物化时隙登记映射或明确不重叠范围，手动回补另有精确区间/幂等预览。timezone存显式值，UI可选Asia/Shanghai等IANA时区；已有未标时区按实际部署盘点冻结。DST按UTC instant唯一，时钟回退不撤销历史，向前跳按本计划策略处理。

业界依据：APScheduler按任务/计划配置coalescing、误点宽限和并发，Quartz CronTrigger也提供不同misfire策略；Kubernetes将迟到期限与并发策略分开。这些作为合同设计参考，不是引入这些引擎的决定。[APScheduler](https://apscheduler.readthedocs.io/en/stable/userguide.html#missed-job-executions-and-coalescing)、[Quartz](https://www.quartz-scheduler.org/documentation/quartz-2.3.0/tutorials/tutorial-lesson-06.html)、[Kubernetes CronJob](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/)

迁移时停止旧cronloop、记录cutover barrier与每job原next_run、导入schedule_revision=1，再启新loop；旧随机task_id仅有可证明slot映射时建立关联，不能凭创建时间粗猜去重。开始新loop前查询barrier附近旧job_runs并对账，无法确认的slot人工确认/按保守跳过记录，避免新旧两套调度同时起任务。重新启用job生成新调度revision并从明确起点计算，disable不删除历史occurrence。

C03实施切片：C03.1 状态/协议/错误与能力矩阵；C03.2 主仓receipt/lease与逐动作effect_mode；C03.3 TR结构化完成、记账与status对账；C03.4 cron原子occurrence、按任务补发策略/预算与切换演练。范围扩大后粗估6–10日，只覆盖首批既有动作与有界合同，不含为未知专业平台发明幂等API；缺下游能力的动作留manual_only，不因此无限延期C03本身。

C03反例出口必须同时通过：两个并发callback只有一个效果；running重复不使TR成功；效果已提交/响应丢失与MarkSucceeded失败均不重复效果且可收敛；lease过期旧writer不能覆新代；DB claim失败零调用；unknown不能普通retry；Finish失败先query补账；cron双副本/推进失败/Redis不可用/重启/时区边界同slot一任务；旧client真实现不会因202把未完成认成功；取消未知不假绿。分别保存mock时序、contract四格和真实PG+Redis+HTTP证据。

## R01 最小 durable intent 先于自动化消费者

现状：`internal/pkg/domainevent/event.go` 的 `DomainEvent` 无稳定EventID；`registry.go` 的 `HookRegistry.Dispatch` 进程内goroutine、recover隔离，commit后at-most-once。主仓 `ticket.Service.emitTicketEvent` 使用Background异步派发；`transition_service.go` 的 `Service.Transition` commit后才 `RunPostActions`。`ActionRegistry.RunPostActions` 捕获失败、写日志/ops.action_failed但不回滚转换；`ReassignAction.Delivery` 当前仍at_most_once。TR DB-first不是这些源事务的outbox。旧C-20“所有emitEvent透传tx、1–2日全转可靠”只是历史建议：真实入口是emitTicketEvent和多个独立发布者，不能照抄不存在的统一符号或承诺全事件已持久化。

R01推荐首批建立**业务操作意图+逐消费者投递凭证**，不改造所有通知、不建通用事件总线/任意脚本流程。仅覆盖本平台已明确不可丢的后置操作，以及D10/D12/A02等未来自动化消费者启用前的源事实。低一致性通知保留原Hook和现有成本；未经可靠目录登记的Hook不宣称可重放。后续需外部订阅时投递目的地可以替换为企业事件API，主仓源事务仍持久记录intent；已有专业SOAR编排尽量外包，仅保存协同operation_id和最小状态。

### 源事务、数据与命令/事件区别

目标 `business_intents` 存下列逻辑字段（实际表名可在实现PR调整，合同不得丢）：

- 全局稳定 `intent_id`、`schema_version`、`kind=command/event`、白名单 `intent_type`、`aggregate_type/id/version`、`source_event_id`、`operation_id`、`correlation_id/causation_id`、受限chain_depth。
- `producer/version`、发生/创建/截止时间、actor归因和受限服务主体/授权scope、目标connector/consumer逻辑ID及contract版本、脱敏最小payload与digest。event陈述已发生事实，不能当未经授权的command执行；command必须明确目标/权限/幂等键。单独nonce/用户RT/AKSK不入payload。
- 业务唯一约束：如 `(aggregate_id, aggregate_version, action_index, intent_type)`；同次提交重试找到同intent，不另发新命令。现有表缺aggregate version时用本事务唯一ticket_event/operation凭证关联，不能读max+1生成版本。

`intent_deliveries` 按 `(intent_id,consumer_id)` 唯一：固定consumer contract/config版本、state、lease generation/until、attempt_count/next_attempt_at/deadline、result receipt、最后error类别与有界摘要；state=`pending/in_flight/acknowledged/failed_terminal/outcome_unknown/canceled`。完成不删除intent；归档按保留政策并保留幂等墓碑，避免重放删除后的旧intent再造效果。首批默认可配置保留90日、到期只归档已终态元数据；unknown不得按普通TTL删除。保留期最终由根数据治理决策统一，此技术默认不宣称满足所有审计要求。

事务内写领域事实+ticket audit/event+intent及初始delivery；配置的目标/参数快照由发布时校验器生成并版本绑定，不运行用户任意代码。提交失败一并回滚，intent落库失败使不可丢操作的源命令失败，不做“先业务成功、后台补表也算可靠”。只有可证明从源版本/事件**完整无歧义重建**的消费者允许对账扫描替代intent，须在可靠目录逐项给出查询/去重/期限/删除兼容证明；日志、当前状态扫描、用户口头记忆不够。

首批可靠目录至少包含转换 `spawn_task`、选择异步语义的 `reassign`、受控结果写回/人工确认写入的操作凭证，以及首个D10/D12/A02消费者的生产者。仅在消费者实际交付时新增其entry，不预建所有风险/Agent领域表。目录每项有publisher符号、源事务、intent_type/schema、consumer/owner、期限、幂等方式、授权判定点、对账查询、退出办法；G03装配守卫对启用的可靠consumer核该目录，durable目录不能把旧Hook订阅冒充实现。

### 派发、幂等、授权与恢复

建议复用主仓现有CronScheduler/生命周期承载一个有界poller，初始每秒一批50行、并发2、单调用10秒、单租约120秒；参数配置化并由G04测量，首批不引Kafka/通用工作流引擎。短事务claim delivery（PG SKIP LOCKED+generation），释放DB锁后请求消费者；确认后凭同generation ack。崩溃在claim后/发送后/ack前均可恢复，不以进程内Hook成功返回代替持久ack。优雅关闭先停止claim，排空有限在途，剩余待租约恢复。

重试用指数退避+抖动（起5秒，上限5分钟），默认24小时投递期限且最多20次；授权撤销/参数不兼容立即terminal，超时/限流/网络进retry，可能生效的超时进unknown先query。达到期限告警并转人工/外部平台处理，不无穷重试；deadline不是允许在期限内重复业务效果的借口。告警metrics至少包括最老pending年龄、unknown数、expired数、每consumer失败与对账滞后；低基数标签不带人员/工单ID。

TR消费者使用 `task_id=稳定函数(intent_id,consumer_id)`，按现有DB-first提交；相同ID且不同payload必须冲突而不是沿用first payload假成功。下游收到重复intent须持久化 `(consumer_id,intent_id)` receipt并与本地效果同事务提交；如结果写回主仓则接收service同tx做版本检查、业务audit和inbox去重，ACK仅在commit后返回。不直接复用现有 `notifications`/`message_log`“查一下是否存在”的非原子读写冒充幂等；若某通知转durable，必须增加对应唯一键/事务。

授权不是无限期快照：源事务记录当时获准的命令与scope，执行前由平台重新校验目标对象版本/状态、服务主体是否撤销、原审批是否过期以及不可扩大scope的约束；与人工操作相同领域service执行，ActorID保留归因。长任务/外部Agent不可携带用户长期token；受控host与connector只拿本意图最小凭据引用。重放调用不是重新授权的后门；涉及真人审批的结果写回须绑定批准operation_id、参数digest与基线版本。

对账操作分 `inspect/query/confirm_succeeded/confirm_no_effect/retry/cancel`；管理入口只给专门权限+资源scope，POST body含intent/task/idempotency key、期望version、理由/外部receipt；审计同事务写。默认操作先query，人工确认保留证据引用而非改SQL。未知状态需双人审批与否由真正生产风险政策决定，技术先提供可接审批的操作凭证，不在本轮硬建通用审批系统。

effect已经发生而本平台暂记未知时，主平台不自动反向调用“撤销”来伪回滚；提供原操作查询/对账和下游正式补偿接口。补偿为新intent，有自己的授权、operation_id与结果，保持因果链；无补偿能力就人工处置/外部消费，不在本平台重造专业平台功能。超过预算的胶水链导向已有SOAR/企业Agent工作流，主仓只保留协作事实。

### reassign 的明确业务取舍

现有`Transition`提交后才经`RunPostActions → ReassignAction.Execute → assignAsSystem`另开事务转派，失败不回滚且可能不重试；但`Assign → assignAuthorized`本来就把处理人、open→assigned状态及事件放同一PG事务。两者写同一本地tickets表，没有必须异步的技术理由。前稿将全部reassign推荐异步过宽，现改为按业务不变量给owner举例确认：

- **必须同时成立的本地变更**：例如“提交复核”定义为状态进入待复核且复核人改为乙，乙停用/不合法或写失败时整笔失败，保留旧状态/处理人，不产生后继消息意图。候选采用同一短事务内状态+本地转派+审计，抽取接受现有tx的内部用例，不能直接嵌套调用自行开事务的Assign。
- **已成立事实的独立后继**：本地提交/指派成功后通知乙、同步专业平台等，在同一源事务写intent，提交后可靠异步；失败显示通知/同步待处理并重试/对账，不撤销本地已经成立的事实。只有业务明确允许“状态先成立、稍后再分工”的可选转派，才采用转换成功+转派独立待处理，并以assignment revision防重试覆盖新的人工作业。
- **外部结果是完成前提**：先进入执行中/待回执，异步调用专业平台；确认结果后再本地同事务推进状态/责任。不能持PG事务等待网络，也不能把请求已发当业务已完成。

不新增一个让普通用户随意切“同步/异步”的开关；由用例/转换定义声明哪些是必需本地变更、哪些是独立后继，发布前校验并给owner可理解的成功含义。EXE-D01仍待owner根据示例确认，现状不因本页修改而改变。上述范围与C04/R01复用事务/receipt，不重造流程引擎。[PostgreSQL原子事务](https://www.postgresql.org/docs/current/tutorial-transactions.html)、[Camunda事务边界示例](https://docs.camunda.org/manual/7.24/user-guide/process-engine/transactions-in-processes/)、[Transactional Outbox](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/transactional-outbox.html)

### 首批迁移与可验收切片

R01.1 可靠目录+表/唯一键/操作凭证，C04事务写入接缝；R01.2 poller+TR幂等提交+失败期限/inspect；R01.3 一个真实不可丢后置动作和一个结果写回consumer贯通，保留原best_effort路径；R01.4 kill/replay/撤权/回退演练与运行手册。总账3–5日针对这个最小闭环，不含迁移全部通知/全部专业系统或通用编排台。不能完成单consumer原子闭环时关闭该consumer，不把设计页作为D10/D12/A02开工通过凭据。

不回填猜测历史intent。发布cutover barrier，新源事务按已版本化配置单写新intent；迁移期避免新旧post_action同时执行同一效果。低风险影子期只产不投或仅核对不发效果；正式切换逐consumer，记录最后old/new操作号，旧执行器停止后再放新executor。在途旧操作按C03对账，不能批量重跑已完成历史工单。N−1读端不认识intent不影响读原事实，回退时兼容poller/reader保留，停止新意图类型；新数据库事实不做down删除。

R01反例出口：源事务回滚无intent；业务commit后进程被kill仍最终交接；消费者已commit但ACK丢失不重复效果；两个poller同intent不双执行；payload漂移同键拒绝；授权撤销/审批过期不自动执行；乱序结果写回版本冲突可见；unknown不能被expire/cleanup抹掉；队列重建不把旧已完成intent再跑；私有connector不可用只积压其delivery、不拖垮主仓；回退后仍可查询/对账未终结凭证。mock时序、合同N/N−1与真实小栈分别有证据；真实专业API尚未给出则该real格保持待验，不伪造成功。

## 决策清单、实施依赖与审查出口

- **EXE-D01（R01/C04）**：修正前稿一概异步建议；必需本地状态+转派推荐同事务，独立后继可靠异步，外部完成前提用待回执。owner要求先给例子，尚未批准最终业务语义。
- **EXE-D02（C03）**：owner提出按任务配置，已修订候选为skip/coalesce_latest/catch_up_bounded，独立设迟到/范围/数量/并发/速率；不再要求全平台选一种。默认/能力允许集与旧计划切换仍需示例验证，不能将提问当作全部细节已批准。

技术推荐不另抛所有者：状态/receipt、单调fence、稳定occurrence、DB先持久化、unknown先对账、additive协议、私有adapter独立验收按本文实施。真正下游API幂等/查询/取消能力未知须列connector合同差距，默认manual_only/关闭自动执行，不用“以后系统接好再说”无限挂账。

依赖为C03先定义完成/重放合同，C04提供同事务入口，R01最小闭环先于D10/D12/A02等不可丢消费者。G03持有组合manifest与core N/N−1×TR/adapter N/N−1×协议版本矩阵；内网provider仅API方式接入、凭据与出站目标受控，不涉及模型训练部署。结果仅白名单业务数据；事件仅支撑本平台安全生命周期，不扩展成SOC/CMDB/TIP/SOAR的第二套实现。

设计自审出口：逐条核认领/持久效果/HTTP/队列账本/业务结果五个时点；把每个“成功”追到receipt，unknown逐条有发现/期限/操作；新旧四格可升级可停可回退；运行参数有预算且能隔离故障；每个建议时态明确，不以旧验收证明新机制。实现时按两页case拆PR并更新实际源码/契约/总账，本文不宣称任何新路由、表、能力已启用。
