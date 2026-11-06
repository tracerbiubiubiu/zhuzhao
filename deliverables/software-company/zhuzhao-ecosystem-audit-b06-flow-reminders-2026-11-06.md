# 生态审计 B06 续批：流程快照、提醒与调度（2026-11-06）

> **当前结论（2026-11-06）**：F05–F10已在独立联合快照 `12401aa52d5fa8842dd944b9366917b19ae7775e` 固定重建，通过 lint、完整单元、完整集成529.99s、cover、原样验收253/0/0和81对迁移往返，以及真实撤销13步/成员生命周期15步，代码层复验关闭。F11新增漏通知修复已提交，最新联合1a99a25另行复验中，暂不关闭。未合main；常驻test app=f69，只覆盖其中部分修复，dev不动。以下旧“待复验”段落保留验证过程时点，不代表当前状态。

## 范围与基线

受审主仓完整快照 `5c03581d5999139a3e8e5ead5972c026868aa181`；全部新增探针仅加入独立 git archive，动态使用自有 testcontainers PostgreSQL 15、真实迁移/服务/仓储。初始原样业务代码不改。不是完整 HTTP 压测，不触共享 dev/test，不改 E000001。证据本地 `evidence/ECO-20261104-b06/semantic-review/`。

本批延续票据状态机/协作扫描和运维实践深读；不代表原快照135份docs文件（130份Markdown，另5份生成/原型产物）或全部历史债已审完。未来phase6定义快照/引擎v2仍待实施，但当前已交付的condition/validator必须在当前写入中成立，不能以未来规划豁免当前陈旧预检。

## B06-F05 / P0：同状态变化可使旧处理人或旧validator通过提交

原实现锚点：`internal/service/ticket/transition_service.go` 的 `Transition`、`checkTransitionMeta`；`internal/service/ticket/service.go` 的 `Close`、`assignAuthorized`。设计为S3a condition谁能执行、validator数据前置；工程公约§6检查+写入同事务。预检在Begin前基于GetByID旧行，后续仅status条件更新不能察觉同状态数据变化。

原快照真实交错：

1. 配置 `open>closed` / `open>assigned` 的 `validator=priority==1`，创建priority1工单。
2. 另一个事务锁住同一工单；真实Transition、Close、Assign分别完成预检并阻塞在UPDATE；通过pg_stat_activity确认真实锁等待，非睡眠猜中。
3. 锁持有者在状态仍open时把priority提交为3；三路继续并成功写成closed/assigned，service_error=nil。三路各8/8，`baseline-result.json`及`baseline-probe.log`。
4. 权限变体：`assigned>in_progress roles=[assignee]`，旧处理人发起真实Transition。到UPDATE等待点后并发将assigned_to改给另一个人、状态仍assigned；旧处理人仍成功进入in_progress。8/8，`condition-reassignment-result.json`与日志。因condition权限门禁失效，本根因P0；validator变体并入，不重复三条计数。

SQL仅用于安排并发写和夹具，目标执行链为真实service/repo/PG；不是声称已通过HTTP完成重放。独立role/user_orgs关系撤销不是本证明对象。

修复 `fb0fff29da5774f557e51d714e1ee7c0b1b8d879`：`TicketRepo.LockTransitionSnapshotTx`，先对类型定义FOR SHARE并核对version，再对工单FOR UPDATE，核对所有持久字段，忽略展示姓名回填。预检到提交之间数据变了返回409要求重读；并发关闭保留90004、缺失保留90001。三个写入口同源调用，读写都复用已准入tx；不会在新事务窗口重调池版角色/委托查询制造BK11。仅保证**本张工单自身字段和类型version预检快照**；不声称独立user_roles/user_orgs/角色状态也具跨表原子撤权。

专项八轮绿，三路移除/忽略快照拒绝，各8/8红（Transition另含旧处理人8红）；单独忽略type version核对8/8红；四连接全占用、不同工单并行调用新helper八轮绿。首次Assign实验副本移除call致未使用变量编译失败，属于无效变异，已保留并以“调用仍执行但忽略错误”的针对性变异重做，不计编译红为缺陷证据。

## B06-F06 / P1：无目标未派发却耗尽提醒额度，候选窗口可饥饿

锚点：`timeout_scanner.go` 的 `scanState`、`emitTimeout`、`scanDeadline`、`emitDeadline`。原emit函数无目标或reg=nil直接return，调用者却无条件bump水位并记“dispatched”。配置notify仅supervisor而未指定上级，真实Handle后无事件却count1，max1时后续同状态补上supervisor也不会再提醒。原快照8/8。deadline无目标同根。

与G33去重：G33接受“已经派发后记账失败、下次可重发”，不接受“根本没有事件却先耗额度”，故是新功能缺陷。S6进程内事件投递的at-most-once丢失窗口仍保留，不把本修复误报成可靠送达。

修复 `e329db07452ac3ccf7a93cf981dccd561b5f33b5`：emit返回是否已交事件注册表，只有true才推进水位；两路同时改。timeout候选SQL先按配置目标检查是否存在，防前500无目标行占窗口使第501条可达提醒饥饿。回归八轮覆盖缺目标、缺registry、同状态补上目标可首次提醒、再次扫描不再发、500无目标行不饿死有效候选。分别移除候选过滤、timeout/deadline派发结果守卫各8/8红。

## B06-F07 / P1：max_reminders=0 仍首次派发一次

max_reminders校验允许非负，UI可选0。`shouldRemind` 在无水位行时不看max就返回first=true；scanState的NOT EXISTS也允许不存在水位行，因此零次配置仍count1。原快照8/8，日志中 `max_reminders=0 actual_count=1`。

同笔 `e329db0` 在scanState和shouldRemind首部按零上限返回不发；原配置形态不变。八轮绿，单独移除零额度守卫8/8红。

## B06-F08 / P1：Cron会话锁占用业务池预算

原基线 `CronScheduler.tickOnce` Acquire业务pool的连接持会话级advisory锁，真实维护job又走同pool。独立实验设合法MaxConns=1，真实AuditArchiveJob的ArchiveFetchBatch无法取连；八轮均无成功tick、ctx超时，`cron-pool-budget-baseline.json`/日志。较大池被多个cron及其他事务占满时同构，已有2分钟execCtx让它超时失败而非永久挂死。初核未发现对应已接受债；不能只规定池≥2规避整个资源模型。

独立后笔修复 `8b3dc2d54cecf30ecd48097c2cde61b8516f7512`（`codex/fix-cron-lock-budget`，base e329db0）：每次tick从pool连接配置复制构建专用pgx会话，取锁者不再占业务池；锁未得即关闭，持锁调用真实job后在独立2秒预算解锁及关闭。运行仍有2分钟execCtx；失败（含connect/trylock/unlock/close）进入recordFailure及既有操作通知，竞争未得锁不记失败。已取消Run不再启动新尝试。部署说明明确每个活跃cron最多额外一条PG连接；当前七项维护任务并发预算须另加，不把提高MaxConns当修复。

实际AuditArchiveJob+业务池MaxConns1、两个独立scheduler互斥、失败/panic/取消/专用会话中止及连接失败可观察八轮 `-race` 全绿；实际pg_stat_activity观测双实例峰值2–3（业务池1+至多两个活跃专用会话）、job同时执行峰值1，停机关闭业务池后0残留。恢复共用池、去除失败记录、去除会话关闭、忽略解锁错误四处变异各8/8红。候选lint/unit全绿；首次探针因取消后LastError被新错误覆盖而断言过严，保留失败日志并改为捕获确切OnFailure回调，不把该探针断言失败报产品缺陷。证据 `cron-lock-budget/{targeted-eight-v2.json,mutation-results.json,candidate-gates.json}`。未自行起第三轮全量integration；联合固定源完整integration/acceptance待root复验，不提前标完成。

## 修复状态与验证

独立 `codex/fix-transition-snapshot`，base `475b7ac896c94f9c7ef29d27236feff1eb1269e9`，两笔明确窗口：fb0fff29… → e329db07…。无新迁移/端点/权限码/依赖，原分支未合main、未push；root已在联合验证分支d38f10f组合修复并仅部署隔离test，完整集成尚未收口。

候选lint/完整单元通过、专项v3八轮通过、满池helper八轮通过、八类有效变异各8/8红（详mutation-results.json）；完整integration首轮exit2/982.94秒：其他包均通过，最后ticket包因测试执行中我将临时回归拆成两个文件，Go前期枚举与后期编译不一致，生成_testmain引用三个已移动符号而构建失败；属于审计验证操作错误，不记为产品回归，也不冒充通过，保留原日志。固定提交e329db0已独立git archive重建，lint/unit/专项八轮/满池八轮/覆盖率五项全绿，结果fixed-gates.json。后续只在冻结归档运行；root联合固定树正在执行完整integration和原样makeacceptance，闭环前不提前关单。

## 文档/覆盖剩余项

新增实读：VISION、phase7/8/9 README、ADR002、ADR目录README、modules目录README、ops三文档及backup脚本/恢复说明。phase3/README、phase6/README、ledger、roadmap、ADR003多次读取输出有截断，保持partial，不能据目录/搜索声称已全文。module长期稿及phase1/2/3/4/5其余unread/partial仍需完成。

P2候选文档漂移：docs/ops/deployment.md当前可执行指引仍≥Go1.22、SERVER_PORT/SERVER_MODE/DATABASE_DSN/REDIS_ADDR、裸docker compose和:8080；实际go.mod1.26.5、配置BindEnv APP_PORT/APP_SERVER_MODE、部署文件在deployments且当前入口33333。ops/README仍把已经存在runbook标未编写。建议后续纯文档批标历史参考并指向现行deployments/README及docs/runbook，避免复制多套旧环境键；尚未修改共享文档。

## 主审接续与锚点（2026-11-06）

原快照可点击锚点：[`transition_service.go` 的 `Transition`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/ticket/transition_service.go#L22)、[`service.go` 的 `Close`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/ticket/service.go#L778)、[`service.go` 的 `assignAuthorized`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/ticket/service.go#L908)、[`timeout_scanner.go` 的 `scanState`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/ticket/timeout_scanner.go#L134) / [`shouldRemind`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/ticket/timeout_scanner.go#L219)、[`cron_scheduler.go` 的 `tickOnce`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/cron_scheduler.go#L127)。源码符号为主，行号仅辅助。

原候选完整integration最终exit2：测试运行期间在途文件被拆分，Go前期枚举与后期编译不一致，生成_testmain引用已移走的三测试；属验证操作错误，不计产品回归或通过，原日志保留。后续只在冻结归档运行。e329db0固定git archive已独立通过lint/完整单元/专项八轮/满池八轮/cover（fixed-gates.json），尚缺该原分支独立完整集成与acceptance，不以窄门禁关单。

联合 `d38f10f9c60a8457417c2630afba0926b103797d` 逐文件比对来源相等，固定归档已经lint/完整单元/专项八轮/cover及原样 **253 PASS / 0 FAIL / 0 SKIP、81对迁移往返零漂移**，真实撤销13步/成员生命周期15步通过；完整集成进行中，新F05–F07暂不提前关单。隔离test app的容器内二进制SHA已核，部署只用于联合浏览器验收，不代表合入main或dev已升级。F08仍为独立修复批，未随本次部署。

本报告与[B06存储/事务前批](./zhuzhao-ecosystem-audit-b06-storage-transactions-2026-11-05.md)分列，发现编号连续且不重复计数。原始证据仅本地`.codex/audits/ECO-20261104/evidence/ECO-20261104-b06/semantic-review/`及`ECO-20261104-joint/d38f10f/`；过程材料不纳管。未读/未验项保留于执行卡，整体审计继续。

## B06-F09 / P1：creator收件人把创建者与操作者混淆

原快照 `5c03581`，`notify_hook.go` 的 `resolveRecipientsFromRule` 把 ticket 的creator规则固定解释成payload.actor；`Close` 等发布点却不带该键，因此漏发；CC发布点带actor=本次加抄送者，因而错发。真实Create(A)→Close(B)→HookRegistry→MessageDispatcher八轮红（零消息），真实Create(A)→AddCC(B)同链八轮红（收件人B而非A）。`semantic-review/creator-recipient-baseline.{json,log}`与`creator-recipient-cc-baseline.{json,log}`分开保留。先前会间消息把Close的源码推断误写成实际发给B，已及时更正，以这两组真实日志为准。

修复`824305f5abf8b96679d98a684352cf21f16fe8c3`，分支`codex/fix-notification-creator`基于`823455c`（Cron与提醒测试隔离均为前置），16文件237+/39-。现有工单发布入口统一从工单created_by快照注入creator，actor仍本次操作者；扫描查询同源携带created_by。ticket规则缺creator时不猜测回退actor；ops无工单创建者，保留既有creator=actor责任人约定。异步消费者不查DB，真实Close排队后Delete再放行仍可通知创建者。新增ticket.creator变量字典、白名单同步，H-3其他动态字段/时间变量仍开放。Create/Update/Assign/Close/Transition/CC/notify动作/分派未命中/timeout/deadline十类已补真实发布回归，固定/cc/assignee与ops正对照并行验证。Delete/评论/备注原本只落ticket_events审计流，不因本修复新增DomainEvent行为。专项八轮、六类有效变异各八红、lint/unit/完整ticket包-race/cover全绿，证据`notification-creator/gates-v2.json`及`mutation-results.json`；v1 lint因额外导入的原始探针未gofmt失败（验证产物问题），保留原日志；另建v2冻结树仅格式化该探针后全部重跑。完整联合integration/acceptance仍由root复验，不提前关单。


## 联合门禁续记：提醒回归夹具污染（验证缺陷，不计新产品问题）

root联合d38完整integration发现新增TestAuditNoRegistryNoWatermark遗留活跃类型和无水位超期单，后续全局TestB104扫描多收到一次通知。我在原d38冻结归档完整ticket包28.89s独立复现（exit1），未放宽旧断言。单独提交`823455cb867c721d45730f826bc8d18fed9c7575`：只改新增reminder_delivery_integration_test.go，五类类型t.Cleanup停用、四registry限时Drain。原顺序每轮新PG八红、修复同进程新旧用例顺序每轮新PG八绿、修复完整ticket包-race20.67s及lint绿。证据`reminder-isolation/`。联合全门禁将纳入此后笔重新冻结；原失败保留，不把审计测试污染归成产品回归或假装先前完整integration绿。


## B06-F10 / P1：SWR与清理扫描交错导致正常前台查询panic

原快照`5c03581`，`internal/pkg/threatintel/service.go` 的`Service.refresh`和`Service.Query`共用singleflight group/key，前台Do成功要求`*Response`，后台成功却返回nil。复现为：真PG预置expires_at八天前缓存→真实Query立即返回stale并阻塞在受控Provider→真实CleanupExpired删旧行→前台Query真实miss并加入同flight→runtime.Stack确认真实WaitGroup等待后释放上游→后台成功存新缓存但返回nil→前台类型断言panic。原独立进程/PG八轮均同一panic、等待栅栏失败0，证据`semantic-review/intel-singleflight-cleanup-eight.json`及round-1..8.log。首次错误地-count8复用SetupPostgresShared，首轮有效panic而后七轮closed pool，属无效重复已保留，不能计八红。

最小修复：refresh的Do回调返回fetchAndStore的真实Response和原err，后台错误仍Warn；其他缓存语义不改。永久回归合入既有B91的PG生命周期，覆盖真cleanup、实际singleflight等待、成功结果及Provider失败错误传递，避免额外顶层测试提早关闭全包共享池。冻结源原探针八绿，恢复nil返回的有效变异八红；完整情报包-race、lint/unit/cover绿，证据`intel-refresh-result/{fixed-original-eight.json,mutation-eight.json,gates.json,manifest.json}`。独立提交`c23e73c1eb6e94a0308784c0d1091fe8ab7810c2`（4文件99+/3-），完整联合integration/acceptance待root复验。


## B06-F11 / P1：真实编辑与公开回复未接通已承诺消息中心

原快照 `5c03581d5999139a3e8e5ead5972c026868aa181` 及最新含身份修复的c23均复现：真实Create→Assign给B→订阅Inbox→A调用Update/CreateComment→Drain→查B notifications，更新/公开回复各8/8无消息。Update发ticket.updated但缺to_assignee；CreateComment只有ticket_events事务留痕，无ticket.commented。既有Inbox词表/规则白名单和S6当前能力已承诺该事件；G7/G29偏好/规则增强不涵盖真实生产者完全未接线。没有手工构造事件来替代生产链。

修复 `92adc205c85c07e900d79801743a3a2e174c16d1`：Update补现有处理人锚，公开评论事务成功后发已有ticket.commented，携工单身份/目标快照和元数据，不带正文、不扩CC广播；内部备注维持私有且不发布。多一次快照读取发生在事务前，未在tx窗口内再取池。原真实探针修复八绿、操作者去噪/内部备注/事务失败反向均通过；三类移除修复变异各八红，其中deferred constraint trigger真实强制commit失败，提前emit必失败。lint/unit/完整ticket包race18.48s/cover过；联合 `1a99a2534372da2106828b75962d162d98fdadac` 冻结完整integration/acceptance进行中，暂不关单。证据 `inbox-business-events/{current-c23-baseline.json,gates.json,mutation-results.json,manifest.json}`。

## 本批新增源码锚点与固定联合证据

- [`internal/service/ticket/notify_hook.go:80` 的 `resolveRecipientsFromRule`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/ticket/notify_hook.go#L80)。
- [`internal/service/ticket/service.go:778` 的 `func (s *Service) Close`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/ticket/service.go#L778)。
- [`internal/service/ticket/cc_service.go:24` 的 `func (s *Service) AddCC`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/ticket/cc_service.go#L24)。
- [`internal/pkg/threatintel/service.go:167` 的 `func (s *Service) refresh`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/pkg/threatintel/service.go#L167)。
- [`internal/service/ticket/service.go:1032` 的 `func (s *Service) CreateComment`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/ticket/service.go#L1032)。
- [`internal/service/ticket/service.go:651` 的 `func (s *Service) Update`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/ticket/service.go#L651)。

完整固定联合结果：`.codex/audits/ECO-20261104/evidence/ECO-20261104-joint/12401aa/gates/results.json`与`full-acceptance-result.json`。全部受审跟踪源码在git archive中逐字节相等，门禁后源码未漂移；临时隔离栈清理成功、已有容器ID/启动时间不变。原d38完整集成706.89秒失败确因为新增提醒夹具污染后续TestB104（非产品缺陷），823455c只清理新增夹具而未放宽原断言；原失败、无效变异与操作失误日志全部保留。单元口径cover是默认build标签范围，不将它解释为全integration覆盖率增长。

P2当前文档状态失真汇入后续单一校准批，原始135文件阅读索引持续补齐，历史台账仍逐行对账，**整体审计尚未收口**。原始证据/过程材料不纳管；必要报告与docs活状态纳管。
