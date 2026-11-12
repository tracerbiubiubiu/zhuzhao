# 定时任务错失、积压与恢复策略详设

日期：2026-11-11。状态：C03/I03设计候选，**未实现**。所有者要求按任务选择并细化策略；本文给出字段、模板默认、状态、恢复与验收，非引入通用调度产品。工作量计在[总账](../review/planning-backlog.md)C03/I03，不另计一套。执行成功/未知/receipt仍以[跨仓合同](cross-repo-execution-contract.md)为权威。

## 1. 当前实现与本次增量

TR `internal/service/cron.go` 的`ParseSpec/NextRun/Loop.FireDue`当前使用五段cron、默认30秒tick，扫描到期定义后以随机task_id提交一次，再从now计算next_run；提交与推进分开。它没有稳定时隙键，也没有本文的三种恢复策略。`internal/repository/jobs.go`的`Job`没有独立timezone、版本、恢复窗口/预算或episode。`task_service.go`的`UpdateJob`当前连description/params/dept/timeout变更也会重新计算next_run，且无CAS。

`timeout_secs`的0/1–86400是当前动作/回调超时合同，0继承配置（客户端缺省30秒），不能充当迟到期限、回补窗口或整批恢复时长。当前parser接受TZ=/CRON_TZ=前缀，无独立时区字段；目标改为显式IANA时区，遇到表达式前缀与字段冲突应报错而非猜优先级。无未来时隙/Next返回零值也须拒绝保存，语法可解析不等于可调度。

## 2. 先区分四种事情

| 事实 | 例子 | 处理 |
|---|---|---|
| 正常调度迟到 | 10:00应触发，30秒tick在10:00:30发现 | 在正常容忍内照常执行，不因skip策略丢掉 |
| 漏建触发记录 | 停机，10:00/10:10均未建立occurrence | 本文misfire策略选择/补建稳定时隙 |
| 已建但未开始 | PG已存任务，Redis/worker停机导致pending/queued | 仍用原occurrence/task_id；执行前检查时效/合并，不另造替身 |
| 已执行失败或效果未知 | HTTP失败、超时可能已经生效 | 原任务执行重试/效果对账，不由“补发”复制任务 |

`scheduled_for`是原计划时刻，`recovery_cutoff`是本次恢复冻结截止，`started_at`是实际执行时间，三者分别保存。历史日报还带`business_timezone`与`data_window=[start,end)`；按当地日界计算，不以固定减24小时代替日界。所有时隙唯一性按UTC instant；同一秋季重复墙钟时刻可有两个不同UTC时隙。

时隙幂等不等于业务产物幂等。两个DST时隙可能映射同一个日报区间；报告类Action须另声明`report_type+target_scope+data_window`等业务幂等键，领域服务对同产物返回既有receipt，通知按结果版本去重。明确重生成才建新结果revision并审计/确认；调度器仍保留两个合法UTC时隙，不擅自对所有任务按日期去重。

## 3. 三种策略的准确含义

### skip：过期不补

对于未开始且超过正常触发迟到容忍的时隙，写`skipped_misfire`与原因/覆盖范围；正常短暂迟到仍可执行。适合已过时提醒。已经running/unknown的任务不被skip取消，业务状态不伪装成未执行。

### coalesce_latest：合并为最新一次

对本次截止前、满足自动回补年龄的候选时隙，选真实最新一个执行；其它记`coalesced_into=<occurrence_id>`或可验证的区间回执。若候选中包含正常迟到的最新时隙，只执行它一次，不能再额外补一个旧时隙。适合查询/同步当前状态，结果明确显示实际取数时间，不能称其重建了过去状态。

已物化的旧pending任务只有在锁内证明尚未开始、与worker认领互斥后才能标superseded/coalesced。已运行或效果未知者保留原任务并对账；不能只删Redis队列条目当取消成功。超出自动窗口的旧时隙明确记expired/范围，不能冒充合并后每个历史时隙都成功。

### catch_up_bounded：有界逐次补齐

为每个符合允许窗口、尚未有执行终态/未重复准入的时隙分别执行；默认时隙升序、并发1。可复用已有not_started任务；遗漏时隙才新建occurrence。同一时隙最多一个业务身份，episode_id不进入业务去重键。

适合历史数据可回溯、各业务日期有明确输入的任务。比如11月9/10/11日09:00的“前一日报表”分别读取11月8/9/10日当地时间`[00:00,次日00:00)`。缺少历史数据、旧定义或授权时暂停相应项并说明，不以今天状态生成假旧日报。对已发生外部效果不确定的任务先对账，回补不是绕过幂等/审批的入口。

## 4. 创建/编辑表单与配置合同

先选Action/业务参数，再配置频率/时间/明确时区，最后显示该Action允许的恢复策略。普通区呈现策略和可理解的恢复说明；高级区再展开限额。模型创建的计划同样经过服务端preview，不能只解析一句自然语言就保存。

| 字段（目标名） | 语义 | 首版推荐/约束 |
|---|---|---|
| `misfire_policy` | skip/coalesce_latest/catch_up_bounded | 必须保存选定值；模板默认也在界面显式展示 |
| `normal_lateness_seconds` | 未开始的常规触发可正常迟到多久 | 推荐120秒，结合30秒tick；不是动作超时 |
| `recovery_lookback_seconds` | 本次自动恢复最早可追溯多久 | skip无回补；latest推荐24小时；历史任务推荐31天，且不能超过可用历史/动作允许集 |
| `max_occurrences_per_episode` | 一个恢复批次累计最多准入多少时隙 | latest=1；逐次推荐100，不因tick/重启/重试刷新 |
| `admission_batch_size` | 单事务/扫描批次最多选多少 | 推荐10，且≤剩余总预算；不是执行并发 |
| `max_recovery_concurrency` | 同计划恢复执行同时在途数量 | 推荐1，受Action/组织/系统更严格上限限制 |
| `recovery_rate_per_minute` | 开始恢复执行的速率 | 推荐10/分钟，按共享准入计，不是单进程各10次 |
| `recovery_admission_duration_seconds` | 一个episode可自动准入多久 | 推荐24小时；到期保留未决范围、暂停而非删任务 |
| `recovery_start_ttl_seconds` | 已准入但未开始的恢复任务可等待多久 | 推荐15分钟；超期历史任务hold，当前状态/提醒按模板过期/合并，不套原始时隙+120秒误杀全部回补 |
| `overflow_policy` | 超龄、总量或恢复窗口耗尽的余项 | 逐次回补hold并通知，EXE-D03已确认；不得默默丢失或无限扩大 |
| `same_job_ordering` | 同计划实时任务与历史积压的顺序 | 历史模板默认按时隙顺序；只有Action声明区间独立才允许实时任务越过旧积压 |
| `enabled/start_at/end_at/timezone` | 新计划生效区间、停启和时区 | 创建时enabled必须明示；UI默认未启用，提供“保存未启用计划/创建并启用”明确操作。首次启用后下一时隙开始，不自动回补创建前历史 |

以上数值是保守模板候选，**不是owner已批准的业务SLA，也不是当前字段已存在**。G04实测后允许调技术上限，实际配置绑定版本。普通用户只能在Action允许范围内选，不能选择任意无限量/无幂等回补。拥有配置权限不自动取得资源或后台执行授权。

全局恢复同时在途推荐上限2，包含reserved/queued/running及仍需效果确认的占用。capacity reservation在入队前共享准入，不能让等待semaphore的任务占满现有worker；**开始速率**另在真正not_started→running认领前获取共享短期start-rate permit，permit绑定occurrence/attempt及有效期，旧入队reservation不代表恢复后可一起开始。令牌不足则持久延后并释放worker，不消耗业务失败重试；重复投递不能令同attempt同时开始，实际新attempt仍受速率限制。限流与认领跨存储时宁可保守少发，不能先执行再补令牌账。

常规任务与恢复使用可隔离的队列/配额，推荐4次常规:1次恢复的准入轮转，任一为空可借用；部署/实现必须证明常规任务仍有可用执行容量，不能仅靠队列权重宣称不会阻塞。无安全held/unknown阻断且资源可用时，验常规不饿死、历史最终获得准入；两个unknown占满上限是明确安全暂停，UI显示原因/人工对账入口，不能继续承诺无条件最终准入。这些是C03/I03有限调度能力，不扩为通用DAG。

## 5. 恢复episode、状态与预算

每个episode固定`episode_id/job_id/schedule_generation/policy_revision/cutoff/range/cursor/max_budget/admitted_count/state`、操作者/授权引用及计划hash。每计划同一积压只允许一个活动自动episode；held范围不通过换episode绕过原额度。之后的新时隙保留各自身份/水位，与旧范围分开记录。

状态：`planning → ready → admitting → draining → completed`，异常可进入`held_capacity/held_history/held_definition/held_authorization/held_expired`；终止自动恢复记`stopped`并保留未处理范围，**不写completed**。`completed`须有范围内每项执行终态或已明确批准的skip/coalesce回执，不能仅凭next_run已推进。

预算在occurrence被**持久准入**时消耗，对`(episode_id,occurrence_id)`最多计一次；发送失败、取消、任务失败或执行重试不返还额度。分页扫描次数、一次建几个任务、最终成功几个是不同计数，均不能拿来重置总预算。扩大预算/改变历史版本须preview→授权确认→CAS更新同episode的批准修订，原occurrence身份不变，审计记录前后差异。

一次TR有界事务：锁计划/episode，按策略生成selected/skipped/deferred，核定义/政策版本及可验证的服务授权凭证，持久写occurrence/快照/episode准入关联与预算、skip/合并回执、未决范围/水位，再commit。TR本地事务不能锁主仓IAM或声称跨服务原子撤权；最终高风险本地写由能力属主在C04守卫事务复验。C03早期仅开放有明确服务器策略的具名自主服务/参考Action，代人无人值守等待I02；owner_service/created_by字段本身不是授权证据。Redis只投已提交任务；失败重投原task_id。游标不得跨过尚未保存的deferred范围。单次枚举推荐最多1000时隙或200毫秒，到预算就保存cursor续算；不能猜“最新”或伪造精确跳过数量，未计算数值标unknown并给区间。

worker开始前在同一认领协议下核not_started、start_not_after、是否superseded、当前授权与版本。只有可证明未开始的任务能过期/被合并；认领与跳过CAS竞争只有一方成功。已running/unknown不因episode停用而假取消，遵C03receipt/对账。历史任务开始TTL耗尽后保留业务日期，不能默删；恢复后授权操作者可延长新的开始窗口，仍沿原ID和已冻结参数。

## 6. 实时、停启、编辑与版本

`schedule_generation`是时间线代际（对应跨仓页schedule_revision术语），`definition_revision`是Action/参数/动作超时的不可变生效版本，`policy_revision`是恢复规则版本；实例attempt和episode是运行事实。普通暂停/恢复、描述修改或改策略不增加时间线代际，也不重置next_run。改cron/timezone/显式重排才设明确cutover和新代际，原积压保留原代际；跨代际重叠范围需映射/排除，不能换版本重复同一个业务时隙。

参数修改默认只作用于明确effective_from之后的未物化时隙；旧occurrence与episode快照不改。缺旧参数历史时先held_definition，不默认拿最新参数补旧日。只改description/dept不得影响调度进度；编辑用expected_revision/CAS，冲突返回预览失效，不能静默覆盖另一操作者。

暂停不删除历史、队列或水位。恢复表单先展示暂停期间时隙数/日期、当前策略和将执行/跳过/待处理范围；默认按原保存策略评估，不在后台悄悄更换。用户若选择“从现在重新开始”，需明确确认跳过旧未开始范围并留回执。已成功任务不能由恢复入口重做，确需再执行业务是新操作/独立权限与重复风险确认。

新建start_at留空时以首次启用的服务器提交时刻为起点，eligible时隙须严格晚于该起点；显式start_at为允许的最早时隙（包含该时刻），同时仍不得早于首次启用起点。end_at留空表示无结束时间，有值则为不包含的截止；无后续合法时隙应返回预览错误，不能保存假next_run。新建未启用后首次启用不补此前历史，已启用过的暂停恢复则沿原时间线和策略。UI展示实际第一触发时间；timeout=0显示“继承X秒及来源”，不是无限。

编辑业务参数显示effective_from，编辑cron/timezone显示cutover以及旧→新范围/版本差异；旧任务沿原快照。只有修改说明/标签时明确“不改变触发时间”。“停止恢复”默认停止新准入并hold已准入但可证明未开始的任务，先preview受影响数量；已running/unknown仍执行/对账，不显示已取消。停止不等于跳过，累计admitted_count不返还；对CAS成功hold且可证明未开始的任务，可释放容量reservation/未使用的短期start permit，不能把并发占用与累计预算混为一谈。resume重新核期限/权限、重新取得容量/开始速率准入后沿原episode/occurrence继续，不再消耗一次累计时隙额度。“从现在重新开始”的now在preview固定成具体cutoff，确认时不能漂移。

现行`timeout_secs`保留单次动作含义。调度grace、恢复准入时长、队列开始TTL、单动作timeout与执行重试预算分别展示/记录。队列任务已建但尚未开始同样受时效与策略检查，不能只保护漏建时隙、却让Redis恢复后发送一周前会议提醒。

## 7. 服务端preview与最小接口

这是拟议操作，不是已注册API。遵本平台GET/POST与权限目录，TR仍由主仓受权门面暴露；浏览器不拿内部SK。preview无执行效果，返回`plan_id/hash/expires_at/expected_revision/cutoff`、时区/未来3–5次时间、策略/参数版本、selected/skipped/deferred范围、可计算计数、冲突/历史数据覆盖及预算。

create/update/recover-confirm绑定preview/hash、实际主体、授权引用、目标和expected_revision，改变任何计划内容要重做preview。操作`recovery.inspect/resume/stop/approve_skip/extend_budget`各有权限、理由、范围与receipt；扩大范围不绕过Action允许集、源历史能力或当前grant。所需旧数据/身份不可解析时给可定位原因，不能建议用户直接改PG。

恢复列表分三组显示：处理范围（总数/合并/跳过/待处理）、准入进度（累计准入/尚未准入）、执行结果（未开始/执行中/成功/失败/未知），各组注明分母；不能把重叠计数相加。给出日期、最近进度/原因和唯一下一步。`next_run`仅表示计划扫描进度，不代表历史任务全部完成。模型总结只能消费这些事实，不生成不存在的成功次数。

## 8. 例子、验收与阶段分工

固定例子（Asia/Shanghai）：每10分钟任务因停机漏10:00、10:10、10:20、10:30，10:37恢复，grace=120秒。skip记录四个跳过；latest只选10:30；bounded在配置预算内选四个原时隙，默认串行，10:40之后的新时隙另记。若是按日历史报表，按其对应业务日期读取历史；当前状态查询即便回补也返回现在取数时间。

验收至少覆盖：30秒正常迟到；上述三策略；同一最新时隙不被“正常+补发”双建；已有pending时效/认领竞争；失败/unknown不复制；固定cutoff与重启预算；总量超限余项保留；Redis/PG提交断链；历史日界/DST/无历史数据；参数/时区/CAS/停启切换；撤权和过期grant；积压公平/worker隔离；超枚举预算不猜数；批准扩大范围仍沿原身份。单元/合同/PG+Redis真实小栈分层验，不以文档例子代替实现证据。

C03/P6.0交稳定时隙/receipt/基础版本接缝、三策略合同与合成参考案例，实际任务保持已明确的受支持语义；不以合同通过声明三策略已上线。I03/P7交三策略完整执行、恢复episode/预算/worker准入、preview/恢复操作与UI接线，并配合I02持续grant、运维观测/专业平台场景。C03粗估6–10日不包办完整恢复控制台，I03扩项粗估9–15日（2026-11-12统一总账，替代早期8–12日）；共享基础只计一次。未知提供方不能自动开放回补，先mock/contract、真实另验；不把所有高级功能反过来挡住第一条可验证调度链。当前没有新增表、API、worker字段或任务策略控件，旧运行行为未改变。

设计借鉴（仅公开模式，未上传内部资料）：[APScheduler错失/合并](https://apscheduler.readthedocs.io/en/stable/userguide.html#missed-job-executions-and-coalescing)、[Quartz CronTrigger](https://www.quartz-scheduler.org/documentation/quartz-2.3.0/tutorials/tutorial-lesson-06.html)、[Kubernetes迟到与并发分离](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/)。本项目的恢复episode/权限/预算是自身业务合同，不声称这些产品默认值与本文相同，也不要求引入它们。

同日确认：默认追加上传/主动删除、必需本地转派原子与独立后继、按任务配置方向及超限hold已记决策册。EXE-D03不再待答，本文表中技术数值仍可在G04实测/模板配置时校准。


## I05单次与统一视图的后继范围

本页C03/I03细化周期时隙与恢复，不表示已覆盖一次性/延迟。I05/P7增加once/due_at、服务端确认时刻固化delay、IANA时区澄清及单个稳定发生次，复用I03的queued门控/取消/unknown和I02权限。单次只有一个待执行发生次，不套多时隙逐次回补；过期/grant失效须明确hold或skip，恢复不重新起算N分钟。维护/TR统一只读总览保持写主分开，不能产生第二调度器。最薄范围与依赖见[全项目演进](project-evolution-strategy.md)及唯一总账；字段/API仍未实施。
