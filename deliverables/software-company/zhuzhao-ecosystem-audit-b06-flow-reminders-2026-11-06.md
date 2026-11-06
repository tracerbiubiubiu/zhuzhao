# 生态审计 B06 续批：流程快照、提醒与调度（2026-11-06）

> **后续实现增量**：新增F13/P1导出XLSX收尾/临时文件、F14/P1多键设置快照、F16/P1导出计数并发窗口，详末节；F13/F14修复各自专项通过，完整联合门禁待新组合，不能用738既有全绿关闭新增问题。F15事务字段校验嵌套取池正在独立PG核验，先不混入已闭范围。当前test app实际7389、UI61，dev已按原工作树保卷恢复；旧“test f69/dev不动”是之前时点，不代表现在。全面审计继续，未合main。

> **当前结论（2026-11-06）**：F05–F10已在独立联合快照 `12401aa52d5fa8842dd944b9366917b19ae7775e` 固定重建，通过 lint、完整单元、完整集成529.99s、cover、原样验收253/0/0和81对迁移往返，以及真实撤销13步/成员生命周期15步，代码层复验关闭。F11漏通知与F12连续数组审计漏脱敏已在最新联合7389ed7固定全量门禁通过，代码层关闭；前笔1a99临时RustFS初始化超时失败仍保留，详末节。未合main；常驻test app=f69，只覆盖其中部分修复，dev不动。以下旧“待复验”段落保留验证过程时点，不代表当前状态。

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

## B06-F12 / P1：审计递归脱敏跳过连续数组

原业务`5c03581d5999139a3e8e5ead5972c026868aa181`及当前`92adc205c85c07e900d79801743a3a2e174c16d1`的[`audit.go`的`maskSensitiveMap`](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/internal/middleware/audit.go:127)只递归数组直接包含的map，遇到slice→slice→map停止。文档[`phase1/08`敏感字段脱敏](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/docs/phase1/08-audit.md:100)明确承诺map/slice深度遍历；已认证请求会经过AuditLog，即便handler因参数错误拒绝，审计仍记录。真实Gin+AuditLog+responseCapture探针（仅合成fixture值）让请求`params.rows=[[{"PaSsWoRd":"synthetic-request-password"}]]`和响应`data.rows=[[{"refresh_token":"synthetic-response-token"}]]`进入两类审计字段，均泄露原文，直接password正控仍正确为***。原基线及当前候选各-count8八红，非DB故障/编译失败。证据`audit-redaction/baseline-eight.json`与`original-eight.log/current-eight.log`。

独立修复将此改为统一`maskSensitiveValue`递归map与slice，不扩敏感键、不改变大小写、非JSON占位、2KB截断或bodylimit。**真实提交hash=`f1ff68a79c3bf409d9b9f052836a3c9e6ab794e6`**，分支codex/fix-audit-nested-redaction，base92adc20，4文件82+/9-；代码锚[`maskSensitiveValue`](/Users/bujibuji/.codex/tmp/zhuzhao-audit/ECO-20261104-fix/audit-nested-redaction/internal/middleware/audit.go:150)，真实中间件回归[`TestAuditLog_NestedArrayRedaction`](/Users/bujibuji/.codex/tmp/zhuzhao-audit/ECO-20261104-fix/audit-nested-redaction/internal/middleware/audit_nested_redaction_test.go:43)。本次保护之后写入的审计，不自动改写既有记录；不涉及已接受的params业务读面原文窗口。

请求/响应、直接map/array、连续两/三层数组与非敏感shape正控专项-race八轮绿，完整middleware-race、lint/unit/cover绿（middleware覆盖率72.0%，未量化基线差值）；把audit.go恢复当前92adc20原文件后，HTTP链与数组回归各8红。证据`audit-redaction/gates.json,mutation-eight.json,manifest.json`；没有共享DB或账号变更。已汇入独立联合 `7389ed79bdf409e466542992d9f1befa0c1a8529`，源码逐文件等同来源；新固定完整integration/acceptance正在进行，未标完成/部署/合main。

原快照永久锚点：[`internal/middleware/audit.go:127` 的 `maskSensitiveMap`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/middleware/audit.go#L127)。所有复现仅使用合成口令，未扫描或收录真实用户凭证。

## 最新联合门禁失败记录与接续

`1a99a2534372da2106828b75962d162d98fdadac` 完整integration624.23秒exit2，唯一失败TestStorageE2E_RustFS在临时容器startup/health等待123.22秒后Docker get-state context deadline，尚未进入存储业务断言；其它包含F11真实ticket链全部过。**该轮未通过，未记绿**。相同冻结源码只跑该目标后来通过，源码未改；这仅缩小为初始化时序问题的线索，具体延迟原因尚未查明，不以同HEAD重跑绿冒称已经修复。原日志、空闲资源快照和定向诊断均保留于ECO-20261104-joint/1a99a25/gates。

新7389ed7从独立git archive重新运行原样全量门禁；没有放宽RustFS等待预算或业务断言。F11/F12在该轮完成前保持待复验。既有12401aa的完整通过结论及F05–F10代码关闭仍是原范围，不外推覆盖后两项。

## 最新固定联合复验关闭（2026-11-06）

`7389ed79bdf409e466542992d9f1befa0c1a8529` 的697份跟踪文件独立归档等价，门禁后源码未漂移：lint3.87秒、完整单元10.83秒、完整integration614.98秒、cover14.54秒、原样验收包装95.39秒均exit0。四档原脚本124+68+26+35=253 PASS/0 FAIL/0 SKIP，81对迁移往返零漂移；真实撤销13步与成员生命周期15步通过。F11/F12在此范围代码层复验关闭，早先待验段保留过程时点。

当前默认build标签总覆盖率20.4%；未与原5c作同口径归档对比，不报增幅，也不把integration测试算进该数字。验收专属栈清理成功；运行期间另一审计B11新增其自有PG/Redis，快照仅多这两容器、无移除/重启，不冒称容器集合完全不变。

常驻test app已升级7389，容器内二进制SHA与image ID逐一匹配，AL/TR/UI/RustFS和dev的ID/启动时间保持；UI f104同一产物在新组合完整41例152.039秒全过。仅测试部署，**未合main、未把分支修复声明为生产发布**。旧1a99启动超时仍按未知初始化时序风险保留，后续绿不回写原失败。


## B06-F13 / P1：导出工作表XML未收尾，spill临时文件未释放

原5c与最新738的 `TicketHandler.Export` 字节一致。原锚点：[ticket_export.go 的 Export / NewFile / ExportStream / Write](https://github.com/tracerbiubiubiu/zhuzhao/blob/7389ed79bdf409e466542992d9f1befa0c1a8529/internal/handler/ticket_export.go#L54)。设计phase5/10 §A.6–A.7要求逐行编码及文件可读；原TestB105只收Service行集，没有检查最终XLSX。

真实Gin handler→Service→隔离PG→XLSX ZIP中sheet1.xml，空表/一行/600行均严格XML解析unexpected EOF，原有效八轮XML共24次失败。excelize v2.11.0的Write不自动调用StreamWriter.Flush；其OpenReader/GetRows宽容解析仍可能返回行，因此不声称全部客户端打不开或数据完全丢失。600×32760B实物超过16MiB触发spill，成功、writer错误、取消后writer错误三路原样共24次临时文件残留。

独立修复 **50cad5e32a7c3763edeec0b8ed010abe7ce5587e**，parent738、分支codex/fix-ticket-export-finalize-joint。锚点：[TicketHandler.Export 的 defer Close / Flush](/Users/bujibuji/.codex/tmp/zhuzhao-audit/ECO-20261104-fix/ticket-export-finalize-joint/internal/handler/ticket_export.go:54)、[TestTicketExportWorkbook](/Users/bujibuji/.codex/tmp/zhuzhao-audit/ECO-20261104-fix/ticket-export-finalize-joint/internal/handler/ticket_export_integration_test.go:60)。NewFile后立即defer Close覆盖所有早退/写失败，流成功后Flush，失败在成功审计及附件响应前返回。无新增依赖/路由/权限/迁移。

修复race八轮绿246.154s；分别移除Flush仅XML八红、移除Close仅spill八红，预取消DB反控制仍绿。lint、完整单元、完整handler integration-race26.878s、cover通过，原/修复文件实物和日志在本地 `evidence/ECO-20261104-b06/export-xlsx/`。重复TestMain、internal包可见性、fixture参数与keyword过长的无效尝试保留，正式原证据从original-eight-v4.log算起。COUNT窗口不是本修复范围，见F16；完整联合门禁待后笔冻结，暂不标完整关单。

## B06-F14 / P1：修改另一设置使已配置限制在运行时回退

原5c/最新738的 `SettingsService.Update` 从factoryDefaults复制后只Apply当前键，其他KV只在boot/Reset全量读取。锚点：[settings_service.go 的 Update / applyAllOverrides](https://github.com/tracerbiubiubiu/zhuzhao/blob/7389ed79bdf409e466542992d9f1befa0c1a8529/internal/service/settings_service.go#L240)。S5约定出厂默认→全部KV覆盖→ConfigStore，已有CAS债C19不保证多键内存快照正确。

真实738 Service/PG八轮：先设置max_upload_bytes=1048576，再设置max_presign_batch=7，运行上传限制回退10485760；数据库和svc.List仍显示1MiB，重新构造服务又恢复1MiB。不是UI缓存，也不是未来多实例传播能力。管理面与实际约束不一致，按P1。

独立修复 **6d788d081d79fb626abe9097bdd3a0e3c0fc28c9**，parent50cad5e，分支codex/fix-settings-snapshot。[SettingsService 的 applyMu / Update / Reset / applyAllOverrides](/Users/bujibuji/.codex/tmp/zhuzhao-audit/ECO-20261104-fix/settings-snapshot/internal/service/settings_service.go:129)对单实例持久写与全量重建串行，以出厂快照叠加所有有效KV再发布；读取失败保留旧完整快照并返回错误。硬底线校验与expected_version CAS保留，无API/权限/迁移/依赖变化。

真实QueryTracer在Reset取得KV后暂停，另一Update在锁下等待；无锁变异允许旧Reset结果覆盖新Update。持久写后取消ctx的重读失败控制证明返回error且保留旧指针。**DB写可能已提交，错误后需重试/重启收敛；不是DB与内存跨介质原子事务，不承诺多实例传播。**

三父项各race八轮绿8.909s；partial-apply/no-mutex/ignore-reload-error三类各八红。lint/unit/完整service integration-race255.315s/cover通过；unit-only service覆盖11.6→11.5%，新增分支由integration覆盖，不冒称覆盖率提升。证据 `evidence/ECO-20261104-b06/settings-state/`；首轮构造函数形参自动改稿导致编译失败不计通过。完整联合门禁待新冻结，未部署/合main、不提前关单。

## B06-F16 / P1：导出COUNT预检与实际行流存在并发窗口

基线 **7389ed79bdf409e466542992d9f1befa0c1a8529**，F13/F14不改变该Service路径。原锚点：[Service.ExportStream 的 CountByFilter / StreamRowsByFilter](https://github.com/tracerbiubiubiu/zhuzhao/blob/7389ed79bdf409e466542992d9f1befa0c1a8529/internal/service/ticket/service.go#L438)、[TicketRepo 的两次独立查询](https://github.com/tracerbiubiubiu/zhuzhao/blob/7389ed79bdf409e466542992d9f1befa0c1a8529/internal/repository/ticket_repo.go#L134)。phase5/10 §A.6明确5万上限和导出自审计。

pgx QueryTracer在实际COUNT完成后，用另一真实PG会话提交插入/删除，再放行原Service：预检49999加2，rowFn实际50001、nil error；预检3删1，实际2行但返回总数3。两问题各八红；稳定50000正常、预检50001拒绝且零callback各八绿。不是权限过滤旁路证据，不升级安全P0。证据 `evidence/ECO-20261104-b06/export-count-window/`，原探针53.160s，数据库及Ryuk均精确清理。

2026-11-06本轮已分号，独立修复基于6d788d0继续，须保证实际输出不越5万、成功审计数量等于文件，不允许静默截断。修复/变异与联合门禁尚未完成，保持开放；F13的XML/Close修复不代销此并发问题。F15字段校验嵌套池另由独立PG核验，未证实分支不混入本节结论。
