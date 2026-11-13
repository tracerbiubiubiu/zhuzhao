# 能力共同路径与三条互通链的冻结切片

> 日期：2026-11-13。承接[共同调用合同](capability-invocation-contract.md)、[当前模块边界](current-module-boundaries.md)与[逐操作库存](capability-operation-catalog.md)。本页冻结可实现、可估价的首批合同和反例；**没有实现共同runtime、AL效果收据/变更源或三链联调**。未来工作不是一次性总线改造：现有领域实现继续承担业务，HTTP、内部、任务、扩展及选定MCP只是调用入口。总排期与状态仍由[规划总账](../review/planning-backlog.md)拥有。

## 1. 本期明确什么，哪些能力仍按窗口进入

本次把“所有能力自由调用”收敛为：所有可复用产品操作有受权接口与明确写主；同一操作的所有入口必须进入同一个应用用例，执行相同的参数、权限、事务、幂等与额度规则。不会为了统一将本地调用绕HTTP，也不会把鉴权谓词、事务内部步骤、审计伪造、任意SQL或worker claim变成产品工具。现有158个去重产品能力映射与每个受控例外均列在[机器库存](capability-operation-catalog.json)，不是只验三个示例便宣称全覆盖。

首批代表场景为：A工单节点生成证据摘要任务并回显；B定时任务对一个获授权的AL文档做有版本条件的patch；C该AL变化经持久源事实与订阅触发下一任务。它们覆盖本地事务后继、跨库效果未知、源事实到反应三个不同断点。首批参数只支持有界标量/固定映射，不支持用户脚本、任意URL或动态SQL。风险/HR/设备/模型能力继续按各自模块演进，不假定它们已经存在。

CE02分S同步共同路径、D持久执行阶段、L现有库存逐模块迁移三部分。S在P6.1做代表入口；D在P6.3承接C03/R01/D12；L最终出口归P9，从P6.1起按P6.1/P6.3/P7/P9四段配额迁移，在对应模块第一次出现非HTTP消费者之前完成。其余模块可以先按原有HTTP用例工作，但不能先开放绕领域权限的内部/MCP捷径再补治理。四段2–3/3–4/2–3/2–3日只迁移S/D代表入口之外的其余存量，不重复计S/D；全部存量迁移单独计价，避免把“全能力”目标藏在三个演示的预算里。

## 2. 身份、版本、消息与可核对的DTO

精确定义见[JSON Schema与正反样本](capability-interconnect-detail.schema.json)。它使用draft-07，便于现有AJV6工具离线验证；描述目标v1，不替换当前wire。现有HTTP响应仍由各版本adapter转换，不强迫内部函数套HTTP响应外壳。

- `capability_id`是稳定业务动作名，`contract_version`是输入/结果合同版本，`executor_ref`绑定实现；旧路径、函数名和MCP工具名只作别名。当前库存中的名称是设计映射，未冒充runtime注册。FD03样例候选`risk.query`、`ticket.linked.query`、`knowledge.query`、`risk.dispatch.preview/apply`、`operation.receipt`均为版本`"1"`的计划项。
- `operation_id`标识一个业务效果，重试不变；`invocation_id`标识一次逻辑调用，任务/事件重投不新建业务效果；传输`delivery_id`和执行`attempt/fence`另由C03/R01记录。调度使用稳定的schedule revision＋occurrence身份，事件派生见第5节。不能用每次请求新UUID充当效果去重。
- command明确主体引用、授权revision、目标、精确参数、提交/过期时间、定义hash和causality。HTTP、内部、任务均通过可信ContextFactory恢复当前主体与目标资格；外部JSON的actor、root、角色或`skip_auth`没有授权效力。服务AK/SK证明调用者身份，不替代用户/持续grant的业务授权。
- 效果写主保存`operation_id`、规范输入hash、原主体/授权引用、目标、版本及不可变关联。相同key、相同输入且当前允许读此操作，返回原收据；相同key不同输入、主体或授权绑定冲突，不重做效果。重新授权可以生成新的明确操作，不能把旧unknown偷偷换key重投。
- 队列中不保存浏览器JWT或凭据。入队时校验一次；开始执行/真正提交前按C04/I02再验当前主体、授权、目标generation、定义hash和合同版本。失效进入具名held；不自动换新版本或扩大scope。撤权不会被缓存的“当时允许”覆盖。

规范输入hash取`capability_id/contract_version/target/params`与主体类型、ID、可选delegation引用、定义hash的规范JSON投影。entrypoint、transport、时间戳、fence和request ID不改变业务效果；原授权引用和因果root另由不可变操作记录绑定，后续请求不能借改root重置预算。服务端重新计算hash，不信任客户端摘要。遵循[RFC8785](https://www.rfc-editor.org/rfc/rfc8785)的确定编码思路，本切片只收安全整数、标量与有界对象：拒绝重复键、第二个顶层值、尾随垃圾、NaN、未配对代理项和超安全整数；不做Unicode归一化。金额、超范围整数、日期等要有明确字符串类型，不能默默浮点舍入。target与params中的ticket/document/type/generation必须语义一致；JSON Schema本身不能证明授权、跨字段相等或hash正确。

receipt状态限定accepted/queued/running/succeeded/failed/cancel_requested/canceled/unknown/held，带单调revision。succeeded必须有写主effect/资源版本或受控artifact引用；noop可成功但没有changed事实。failed必须有“未产生效果/已确认失败”的证据；网络超时、租约丢失不等同failed。unknown只允许对原operation查询/对账；cancel_requested只有确认未生效或已按合同撤销，才成为canceled。结果投影用operation＋receipt revision CAS，旧attempt/旧回执不能覆盖新结果。receipt查询也走当前资源可见性，撤权后不能靠猜operation读取旧内容。同一operation从执行/写端点重放时也要独立检查当前读资格；即使执行资格仍有效，旧succeeded/held/unknown记录不可读时统一not_found、不返回旧内容且不新增效果，不允许改新key来回避拒读。

候选对外面：领域GET/POST端点保留；新增`GET /api/v1/operations/receipt?operation_id=...`读受权收据。普通写走具名POST和body，内部通过相同typed application port。无任意`/invoke`万能后门，MCP也只发布审核后的descriptor，认证/凭据/维护机制不会自动成为工具。operation.receipt是该读能力的候选ID，不表示现在已有该API。

## 3. 链A：工单节点 → 任务 → 工单结果

代表输入为`ticket.evidence.report.generate@1`：ticket_id、已发布node_run_id、definition_hash和固定report_profile。它是本次选定的未来异步能力，不是现有任务名。首批生成有界证据摘要/报告引用并在票上显示状态与结果，**不自动关票或改业务字段**；未来写字段另走字段ACL、CAS和显式发布映射。

1. 发布节点绑定确切capability/version、参数来源、允许的结果视图与超时策略。运行时重验当前票/actor/节点版本。需要状态及责任同成同败的本地动作仍在工单同一tx；独立后继才写持久意图。
2. 工单事务同时写node_run、稳定operation关联和R01意图；若意图保存失败，不能显示“已安排任务”。不在持票锁期间调TR/外部平台。C03已有目标DB-first Submit/occurrence与调度重跑策略继续复用。
3. dispatcher以相同operation提交TR；受理回包丢失时对账原operation，不重新造任务。任务首批按当次已批准、精确输入的有时限授权执行；长期无人值守写等待I02持续grant，不把创建人JWT留在任务里。
4. executor读取受权数据并产生有界artifact＋effect receipt；任务系统的运行成功不自动成为票业务完成。core接收只信已登记的TR通道，核operation、输入hash、ticket/node/version和当前可见性，去重后保存结果投影。
5. 结果保存失败只重放投影；不重跑已经成功的报告任务。当前节点已取消/换版本的迟到结果保留审计和原运行视图，不套到新节点、不推进状态。界面可“重试投影/查询原操作”，unknown不给“重新执行”默认按钮。

现有`SpawnTaskAction.Execute`直接调用Submit且丢弃返回值、HTTP `TaskrunnerHandler.failFastAction`未自动覆盖该入口，是必须迁移的具体旧差异。TR本身已有HTTP/CLI/cron共用DB-first Submit基础，CE02-D负责统一它们的业务准入、身份/关联和效果语义，不重复造第二队列。D12拥有工单结果展示，CE02只计新增关联/合同与反例。

最小前端沿现有票详情：任务状态、最后更新时间、结果引用、具名held/unknown、受权查询/重试投影；没有看不见的“后台成功”。最小运维可由operation串起工单意图、TR task/occurrence/attempt、receipt和投影延迟，查询命令不改状态。

## 4. 链B：定时任务 → 同一受权用例 → AL有条件patch

候选`activelist.document.update@1`只修改一个明确类型/文档的1–32个白名单标量字段，必须带expected_revision和generation；权限由core按类型、行、字段与grant验，AL owns实际数据、revision和效果收据。现有Import是全量replace，不能冒充patch或作为风险/HR权威写后门。

TR仍是唯一业务定时器；missed-run策略、补跑时窗、上限和超限held由C03/D11决定。每个occurrence映射稳定operation，补跑不得共享一个效果key，也不能因为重试换key。

core的typed ALCommands.Update同时承接手工、任务与选定事件入口。旧`/al/api/v1/...`路径是已有网关wildcard，改造时只能由版本化gateway dispatch adapter转换到同一应用用例；禁止另注册冲突Gin路由，禁止保留绕row/field规则的裸proxy旁路。未迁移类型暂不能开放任务/MCP写。

跨仓调用使用现有签名服务信任并增加短时、指定purpose/audience/目标/字段/hash/operation/fence的准入凭证；`alExecutionRequest`与`admissionClaims`精确见schema，claims随既有AK/SK签名请求体绑定，非新建通用JWT/OAuth体系。AL重算输入hash并核command与claims的operation/hash/target/授权引用、patch字段子集，时限最长30秒且不超命令有效期；过期或不一致拒绝且不写。AL核边界与时效，再执行业务CAS/receipt，不复制一套IAM或OAuth授权服务器。core在发送前复验当前权限；不能承诺分布式零延迟撤权。若某高风险动作要求更强即时失效但提供方不支持，应禁止自动执行或单独做在线授权握手，不伪造保证。

AL候选本库事务：预留operation receipt唯一键→锁类型metadata/stream head→检查generation和文档expected_revision→patch/计算noop→保存文档、源事实和succeeded receipt→commit。receipt reservation锁序固定；涉及多类型按稳定ID排序，文档按ID排序。重复operation先比较原输入/主体绑定，返回已提交receipt，不用旧expected_revision再执行一次。任何步骤失败，同事务没有半个效果或无来源成功。事务里不访问core/TR或网络。

发出请求后core超时：unknown→按原operation查询AL receipt→确认有效果才投影成功；只有明确无效果且原合同允许，才按相同operation重试。lease/fence过期的旧worker不能覆盖新owner，也不能把较新状态标failed。若AL receipt丢失/保留期不满足最长重试窗，不允许盲重做，held并人工对账。

最小前端在任务/运行详情显示目标、字段、预计版本、当前receipt、冲突/撤权/未知原因和对账动作；创建时预览grant范围与补跑影响，不展示内部token。运维接线包括AL/core签名配置、能力开关、白名单与预算profile、receipt查验/保留、unknown积压和版本冲突指标；mock同合同可运行，真实AL写开关在故障验收后启用。

## 5. 链C：AL源事实 → 持久订阅 → 任务

### 5.1 事实产生处与完整游标

AL必须在每个受支持产品写事务里保存事实，覆盖Insert/Update/SoftDelete/Restore/Import以及任务经相同用例写；post-commit goroutine、网关成功后通知、数据库当前值回查不能替代源证据。外部直接改动态表不在产品保证内，必须禁作受支持写入口或另建已验证CDC；不能默认它也被捕获。类型Register/Evolve/Deprecate本身是已有公开能力，继续由metadata/history写主负责；首批不允许把schema生命周期当文档变化订阅，演进/废弃须持久标记stream状态、使不再兼容的订阅进入held，不能无声继续解释旧字段。

源事实采用[CloudEvents1.0](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/spec.md)的source＋id唯一身份，同事实重投保持id。event-time包含有限changed_fields和发布时声明允许的标量event_values；不是完整历史快照。generation、document_revision、operation、可信causality均明确。删除也不能靠事后查询当前表拼事实。

**首批选择每类型transactional stream head，而不是直接用nextval/global revision作为完整水位。** 写方在业务改动前锁该类型head，递增cursor，写事实并同tx提交；先取锁的未提交/回滚写不会让消费者先越过后提交的较小cursor。每流有序，不提供跨类型全局顺序；并发吞吐代价是每类型短事务串行，远期压力实测后再评估分区/CDC。PostgreSQL的[序列与回滚规则](https://www.postgresql.org/docs/18/functions-sequence.html)及[已提交快照](https://www.postgresql.org/docs/18/transaction-iso.html)决定了“序号变大”本身不能证明较小序号已提交；本期实际SQL反例已经复现ACK2后迟到1被漏读。

noop保存成功receipt，但不增加文档revision/变更cursor、不触发业务订阅。Import一次原子replace产生新generation、一条有界`al.import.committed.v1`摘要与受控manifest引用；不默认一万行变一万任务。旧generation任务进入held。按行回放须另批准有界拆分、权限/配额、manifest完整性和进度恢复合同，不包含在首批承诺里。

候选源协议`GET /api/v1/change-feed?stream_id=...&after=...&limit=...`只面向已登记签名消费者；`POST /api/v1/change-feed/ack`body带consumer/stream/cursor。初始水位为`"0"`。`changeFeedPage`包含after/retained_from/complete_through/next_after/has_more与最多100事实；complete_through是完整提交head，不是已消费位置。页内游标严格递增且同source/stream；有后页时next_after为本页末事件，禁止直接跳complete_through漏中间事实。空页只有已完整扫描到head才可前进。`changeFeedAck`只含consumer/stream/through和stage_commit_ref，不包含“任务成功”；只能ACK已服务且已stage的连续区间。读到的事实先进入core持久收件，再ACK；ACK表明接收端可恢复，不表明任务已成功。重复event以source/id和订阅版本/action唯一键去重。游标已过保留期返回410/resync_required并hold，不能跳latest当成功。

业务事实和业务状态同本地事务、跨库消费者幂等，是沿用[transactional outbox](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/transactional-outbox.html)的边界。本设计不新增Kafka前置：AL有源outbox/feed，core有持久收件/订阅，TR有任务队列，各自单writer。后期吞吐/隔离需求成立再替换传输，不能替换业务效果身份和ACK合同。

### 5.2 订阅版本、过滤时值与回放

订阅保存grant、固定capability/version、typed输入映射、source/stream与水位向量、规则版本。发布时逐目标Schema验证映射，拒绝任意表达式/路径/执行器。可用映射仅固定常量和schema列明的event pointer；未投影字段不能当历史筛选条件。filter只依据**event metadata＋明确event-time projection**，不会用现在数据去冒充当时值。结果读取仍重验当前资格；需要当前值的后续任务是另一个具名查询步骤，记录查询时间，与事实过滤分开。

从source取得当前完整watermark向量W后原子激活规则：旧版本处理其已登记区间至W，新版本从W之后开始；每流只按自己水位判定。规则替换默认hold旧版本已stage未执行动作，已运行动作按原operation收据收尾；不把旧队列静默改新参数。pause停止新执行准入并hold待执行，源收件继续；resume/replay先预览待补区间、数量、generation和grant，不自动清空积压或扩大范围。超限部分保留待处理，与用户已确认的补跑策略一致。

事件派生operation按`op_evt_`＋SHA256(规范化source、event_id、subscription_id、rule_version、action_index)生成；同事实重投不能触发第二效果。root预算/父operation继承可信源记录，不能在每一跳重新开root。默认同规则在同root中不得重入；A写AL→B触发→又写同AL时，即使新event_id也会因规则路径/预算被hold。source事实仍保存，抑制原因、动作与待处理都可观察，不因为防循环而吞掉真实数据变化。

最小前端是订阅列表/版本、目标与映射表单、发布校验与水位、暂停、hold原因、积压预览和显式回放；source健康/延迟可在运维页查看。源ACK管理不得以“重试任务”名义前移游标。候选schema限制每订阅最多16流、4动作；重复action_index、交叉字段不一致与目标Schema映射由语义验证器拦截，不仅靠JSON形状。

## 6. 并发、多实例、异步与预算的可实现下限

同一共同路径不代表所有操作都排队。短查询/本库短写同步处理；外部调用、长任务、可靠后继按既有C03/R01持久异步。不同入口重用相同应用规则和上下文，内部DBTX协作复用同一个事务，不另扣一次操作预算或创建另一operation。

首批admission profile候选：同步deadline10s，异步attempt30s，命令过期900s，输入64KiB、内联结果32KiB，root深度8/总步骤64/扇出16/时长900s，provider并发4/每秒start2，每订阅待执行1000，lease30s/10s续约。这些是有界设计基线，**不是本次实测容量或SLA**；真实供给可下调，向上调整须压测/故障和成本复核。图片/报告字节走受控对象引用，不把大文件塞消息。

配额维度区分主体/授权、capability/provider、订阅和causal root。accepted操作只记一次逻辑效果准入；attempt重试仍计provider starts/inflight/费用。root计数以core持久CAS为权威，跨实例provider节流使用共享Redis原子操作，本地semaphore仅补充。共享配额不可用时，新自动写fail closed并持久held；已确认效果可继续对账和投影，避免把设施失败放大成重复副作用。

租约只分配执行权，不证明旧worker已停止。每次效果提交核fence/有效期，效果写主以operation唯一收据和CAS兜底。发送后失联→unknown，不能仅因lease过期让另一个worker重做。单实例也执行该规则；多副本启用前在真实两个worker与共享设施下验抢占/旧fence/配额，不用单机goroutine通过替代多实例验收。

同步未受理超额返回429＋Retry-After；已受理积压超限进入可恢复held，不能删任务。core收件容量不够时停拉且不ACK；源outbox达到硬字节预算前报警，无法安全归档而必须拒新变更时给明确错误并回滚该写，不能提交数据后丢事实。源保留/receipt保留/最长补跑与恢复窗共同配置，不能无限保留承诺。以输入字节、扫描行、批大小、stage速率和artifact大小作预算，后期全量导出/回放不能绕过。

最小运维指标：outbox oldest age/bytes、source ACK lag、stage backlog、held按原因、unknown age、receipt reconcile失败、stale fence、provider starts/inflight/429、root抑制、投影延迟。runbook需提供只读追踪、原操作对账、暂停订阅、受权预览回放、容量保护、恢复后对账；危险手工改DB不能作为常规恢复接口。日志/trace以operation串联，不打印凭据、完整敏感事件值或任意回调URL。

## 7. 本期已做的合成验证与实现验收缺口

2026-11-13运行了以下设计证据；原始结果保存在本地ignored `.codex/audits/module-capability-20261113/`，可提交样本与反例位于本页schema。[仓内只读验证](examples/capability-reference/README.md)现在可直接复跑，不生成或重写Schema；最初ignored生成脚本不作为验证入口。设计工具使用现有UI AJV6.15.0，没有引入依赖；该工具进程Node22.14.0，不冒充项目目标Node26门禁。

| 证据 | 实际结果 | 证明的边界 |
|---|---:|---|
| Gin实际构造＋Go AST＋UI静态消费匹配 | 176路由/218生产文件/298声明/135 HTTP绑定＋2数据通道 | 受控快照枚举及逐项裁定；不是逐行安全或全页面运行测试 |
| 选定schema正反例 | 49/49（16正、33反） | 严格形状/边界；不证明授权与事务 |
| 规范输入投影 | 6/6 | 入口不改效果身份、主体/输入变化、键序、安全整数/Unicode边界 |
| 原始JSON解码 | 6/6 | 重复键、多顶层、尾随、NaN/安全整数拒绝与正常样本 |
| 合成语义决策oracle | 27/27 | 合成状态下确定期望；不是生产service实现测试 |
| 独立PG候选SQL | 8/8 | 最小事务/锁/去重与游标反例；不是AL真实代码验收 |

PG实验只在确认`audit.owner`标签后创建独立`ce_review_20261113_module_audit`数据库，未访问AL业务public数据。实验6表＋1序列，结束drop并查询pg_database计数0。8项分别为：head锁下未提交不可见且后写阻塞、数据/收据/事实/head回滚一致、同key同输入返回原收据、同key异输入拒绝、noop不生事实、16并发重投仅一效果一事实、订阅stage复合唯一去重、plain sequence先提交2后提交1导致ACK2遗漏1反例。此证据支持选型，不能代替五种真实写入口/多实例故障回归。

27个语义样本包含内部/HTTP未授权、伪造context、重复key、撤权、定义/版本变化、旧fence、过期attempt、unknown对账、发出后失租、noop、revision冲突、provider/root预算、同规则重入、receipt读撤权、重复event、stage ACK、generation变化、规则替换、正常效果及held不自动重投，新增执行入口重放succeeded/held/unknown但失去读权的3个交叉例。首次旧oracle红例、修复及移除读守卫的变异均有受控样例；3例要求不返回旧记录content。每个拒绝/held样本要求additional_effects=0，source事件样本保留原事实；唯正常执行样本允许1效果。

实现PR必须先写以下真实行为测试，再使实现变绿，遵守既有测试先行与完整门禁；本轮schema/oracle绿色不关闭这些未实现条件：

| 切片 | 必须运行的正例 | 必须运行的失败/反例 |
|---|---|---|
| S共同路径 | 同一票查询/写经HTTP与typed内部入口得到相同效果/错误/资源过滤 | 内部伪actor、漏body校验、无文件下载资格、旧定义/撤权、legacy proxy绕行均不多写 |
| A持久后继 | 票事务→意图→TR→原operation结果→票回显 | 每个commit前后kill、提交回包丢失、重复迟到结果、当前节点换版、投影失败只补投影 |
| B受权AL写 | 手工/cron同grant同patch，CAS与receipt核一致 | 撤权/字段越权、版本冲突、发送后kill、旧fence、同key异输入、unknown先查原receipt |
| C源与订阅 | 五种产品写入口＋任务写均同事务出事实，回放不多效果 | commit前后kill、ACK前后kill、重复/乱序/迟到、游标失效、规则替换、no-op、replace及非法event-time字段 |
| 配额/因果 | 两真实worker共享限流与root预算，积压可预览恢复 | Redis不可用、租约抢占、超容量不ACK、写回循环、回补超窗保留余项、持久held/unknown可查 |

## 8. 净增WBS、依赖与冻结范围

以下为相对既有总账的**新增实施净工程日**，含实现、必要评审、最小UI、运维接线与对应失败验收。当前审计/文档/合成实验不重复收费。它不含C03/C04/R01/X01/X02/I01/I02/I03/R06已价基础，不含旧范围补全的完整字段ACL、typed search、CmdK、真值diff、I04第二代表源或生产MCP接既有AS。

| 子交付 | 净日 | 可验收出口与复用 |
|---|---:|---|
| CE01描述目录与库存漂移守卫 | 3–5 | 将完整路由/非HTTP清单接入发布/缺项检查；复用X01目录身份，不再造插件host |
| CE02-S同步共同路径 | 6–9 | typed Context/参数版本/资格与代表HTTP/内部adapter、同效果反例；复用C04，不估整个IAM重写 |
| CE02-D持久阶段与链A关联 | 5–8 | operation/节点/任务/result关联、投影CAS及故障反例；复用C03/R01/D12的队列、状态、已有UI |
| CE02-L其余存量逐模块迁移 | 9–13 | 按158映射与例外逐个结案，按P6.1/P6.3/P7/P9配额随模块触碰推进；不是P6前置大重构 |
| CE03受权AL写三链B | 9–14 | core授权adapter3–4＋AL效果receipt3–4＋最小UI1–2＋接线/恢复验收2–4；复用I01/I02/R06 |
| CE04源事实/水位/订阅三链C | 13–20 | AL producer5–7＋core持久订阅/水位4–6＋UI1–2＋故障/批回放接线3–5；**已含P6-T063 AL producer，不另报价** |
| CE05共享准入、因果与回压 | 7–11 | 共享admission2–3＋root预算2–3＋最小held/预算反馈1–2＋多实例/运维2–3 |
| 合计 | **52–80** | 包含CE02总20–30；只计共同路径/互通增量，非整个未来路线总价 |

依赖不是一条阻塞所有模块的长链：CE01归P6.1，静态库存准备可先做，运行目录资格随X01/C04；S在P6.1随C04完成。D消费C03/R01，链A与D12薄切片同窗验，不依赖I01而形成环。CE03/CE04/CE05归P7。CE03实际自动写启用需I01/I02、R06及适用I03/CE05；AL receipt可先做源端合成验收。CE04 producer可先完善事务事实，订阅执行启用再等待R01/CE03/CE05；T063只在CE04计一次。CE05复用共享基础，不阻塞无副作用的既有普通查询。CE02-L最终出口计入P9，早期触碰按能力迁移窗口推进且不重复计价，最终集成者据总账编排，本文不另写一份冲突日历。

本页冻结身份/版本、三链所选参数与结果、五写入口事实覆盖、完整水位算法、unknown/CAS、过滤时值、替换/回放默认、因果/预算维度、最小UI/运维与验收分母。真实厂商限流、最大源规模、保留天数、共享设施实测容量、额外字段/动作和MCP具体工具投影在对应窗口通过测量细化；不影响按上述保守profile建立合成垂直链。新能力增加库存/typed schema和净价，不能以“以后再完善”跳过首次非HTTP消费者的同权限、同效果与恢复证明。
