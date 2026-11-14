# 首版四条核心联动链与预设端口复核

> **后继效力说明（F013）**：本报告保留当时观察和范围。公司随后允许本地账号先正式，且要求质量优先；完整UAA P/R核心稳定后交，1月为争取试用窗口，原固定日期/完整UAA先行读法已被[当前主计划](../../phase6/first-usable-release-plan.md)及[本次可行性](local-first-core-feasibility-2026-11-14.md)取代。源码现状证据不因此变成产品已实现。

日期：2026-11-14。主仓读取起点`48b2310a15defef1107feebf1f239523d9a0d13d`，范围为当前五仓源码、测试及既有设计。状态：定向源码复核与首版施工候选；未开发、未重跑产品门禁、未证明公司网络或真实提供方接入。

所有者目标：2027年1月试用、2月1日前可用；首版必须有风险、工单、taskrunner与activelist实际联动，公司消息走HTTP。以下先确定施工分母，不按日期倒压工时；AI、高级仪表盘后移，不重价长期76包。

## 1. 已有底座与最薄首版范围

已有工单CRUD/分派/转换/附件、TR任务/cron/重试与运行查询、AL类型/数据CRUD及导入导出、网关/签名、消息模板/规则/站内通知。它们是可复用实物，不等于四链已打通。首版固定一个结论源、一个整改类型、一项证据采集Action、一个AL类型/允许字段组、一条订阅和一个公司HTTP渠道，给实际业务角色可操作的最小UI。

固定Action消费专业平台的查询/结论能力，不自建扫描器或任意脚本执行平台。仍保留大量mock；来源mock可以驱动真实业务服务和数据库，但不能把模拟风险列表、伪发送或假AL权限当作首版实现。首期单实例，实例内并发、撤权、失败恢复仍须验证。

## 2. 风险源 → 风险 → 整改单

- **已有**：`internal/service/ticket/service.go`的`Service.Create`、`dispatch_service.go`的`DispatchService.OnAfterCreate`、`transition_service.go`的`Service.Transition`；工单资源权限及附件体系。创建/分派/转换已有测试，不能据此声称风险域完成。
- **新增主体**：当前`internal/`与迁移没有风险来源接收、risk实体/周期、risk_ticket_links及风险路由；`domainevent/event.go`实际只有ticket/ops域。设计中的RiskSourceConnector和risk.dispatch能力不是已注册API。
- **薄施工**：一份来源白名单DTO及私有映射、稳定source/entity/external_id和可信版本；风险列表/详情、负责人/组织、有限评估与整改状态；一张受控整改类型及风险—票关联。按已确认风险组职责派单，不额外索要通用任意建单权；未批准映射/无owner不得猜人。
- **不可省**：来源事实与本地处置写主分离；重复/旧版本不能误建/覆写；来源缺失不自动销风险；新整改类型查看范围、列表/详情/计数/导出一致，风险负责人复核资格不能借CC或普通处理权替代。固定类型也要防在途配置被覆盖，不要求先交完整设计器。
- **最薄验收**：受权普通业务人员从一条来源结论创建真实风险、派出整改票、提交证据、由合法复核人给结论；同来源重投不多建，越权不泄露，派单失败有持久待办；模拟源与真实源分别标记，不以源mock证明真实接入。

## 3. 工单任务 → TR → 工单结果／AL更新

- **已有接线**：`ticket/actions_builtin.go`的`SpawnTaskAction.Execute/TaskSubmitter`经`app/providers.go`的`taskSubmitterAdapter`调用`TaskrunnerService.Submit`；`pkg/taskrunner.Client.Submit`及utils/aksk签名；TR的`service.Service.Submit`先写`Store.InsertPending`，再入Asynq，cron/回收链已有实物。
- **未做的业务链**：SpawnTaskAction实际只序列化params并丢弃Submit返回值，注释中的`from:ticket.field`未成为真实映射；无稳定ticket/node/operation→task结果关联。TR是调度并HTTP回调核心；核心`provideJobsRegistry`目前仅注册`audit_archive`，不是已有证据采集或AL写Action，更不是脚本执行器。
- **完成语义缺口**：`JobsCallbackService.Execute`把未认领的running也作为幂等受理；TR的`callback.Client.Do`将任意2xx视为成功并丢响应体。MarkSucceeded记账失败仍返回成功；不能直接据这些状态向工单显示“业务完成”。`RunPostActions`在转换commit后运行，当前仅日志/Hook不足以证明任务意图不丢。
- **薄施工**：发布一项typed Action、固定输入映射和允许结果；工单事务保存稳定意图/关联，提交TR保留task ID；回调核原输入、当前授权、效果receipt和单调结果revision。工单详情显示queued/running/succeeded/failed/held/unknown与原证据引用；不默认自动转换或关闭工单。
- **AL写新增**：复用`DataService.Update`与`repository.GetDocForUpdate/UpdateDocData`的合并校验和版本CAS，补后端受权AL port、限定目标/字段的准入及同事务operation receipt。仅写本平台拥有的结果字段；超时先查原receipt，不能换key盲重做；TR不直连AL数据库。
- **最薄验收**：一张真实票触发任务，TR执行固定专业查询并将真实结果回贴，获准AL行发生一次版本更新；重投/响应丢失不重复效果，CAS冲突/撤权进入可解释失败或held，旧结果不能覆盖新版本；运行成功、结果投影成功分别可查。

## 4. AL变化 → 获权任务

- **已有**：AL的`DataService.Insert/Update/SoftDelete/Restore/Import`及`repository/data.go`提供本库事务/版本；`revision_integration_test.go`验证replace不复用版本。主仓`gateway.Registry.Mount/SetForwardHeaders`做JWT/L1、身份头覆盖与签名；这不等于AL已有行/字段级授权。
- **新增链**：AL当前没有源change-feed/outbox、持久订阅和事件到task的登记；`DataService.Import`是锁表全量replace，不是增量patch。不能用网关200、运行日志或updated_at轮询冒充完整变化回调。
- **薄施工**：一个受监控类型的受支持写事务产生稳定事件/提交水位；核心先持久收件再ACK，一条有版本的受审订阅只调用同一Action；普通CRUD与任务写复用领域入口。import用有界replace摘要/generation处理，不默认逐行生成海量任务；未支持的变化类型必须显式拒配/标明，不无声漏捕获。
- **不可省**：发布订阅是明确执行授权，不继承“写这一行的人”全部权力；执行时重查owner/服务主体、目标/字段范围和规则状态。保留因果来源、同事件+规则版本去重、有限并发/速率及防写回自激循环；无需先建通用规则画布或多实例平台。
- **最薄验收**：普通用户的合法AL变更触发一次真实TR任务，重启/重复投递仍只产生一次业务效果；未授权规则、撤权、noop、旧generation、批导入、结果字段写回循环均有确定结果；源提交后进程退出不能永久漏事。

## 5. 业务消息 → 公司HTTP → 发送回执

- **端口真实、发送未做**：`pkg/channelsender/port.go`有`ChannelSender.Send/Channel`，`app.NewApp`只装`UIPStubSender`。开发态返回成功；release即使配置endpoint仍返回“真实发送未实现”。`MessageDispatcher.Dispatch`已有规则/模板/收件人/偏好/节流及message_log，但成功标签可能只代表桩成功。
- **另一路已有HTTP**：`NotificationService.NotifyDeadLetter`经`pkg/webhook.Client.Post`真实发死信告警，属于尽力而为；不是普通风险/工单消息sender，也没有公司业务码/受理ID/送达回执合同。
- **薄施工**：在ChannelSender后接一个私有公司HTTP映射，重用模板/规则和用户身份映射；消息发送意图持久化、稳定幂等键、受理ID/状态/错误投影和有限重试。网络超时可能已受理时先查询或held，不假装失败可无限重发。
- **最薄验收**：风险派单/工单更新两种选定事件真正到公司测试接收方，记录渠道业务码和原回执；非2xx、HTTP200但业务失败、超时已受理、重复、秘密脱敏和未授权收件人有反例。若渠道只证明受理，UI只能显示“渠道已受理”，不能标送达/已读。

## 6. 首版消费的预设端口核对

- **HR**：当前未见生产HRDirectory/DirectoryProvider实现或装配，`docs/proposal/hr-directory-sync.md`是目标设计；现有本地User/Org服务可用。首版可用受审本地目录/合成HR供给，真实离职/撤部同步另验，不能称“已有HR换endpoint即可”。
- **风险数据源**：RiskSourceConnector在设计中，未形成运行注册；需新增归一化port/参考适配。已有`pkg/threatintel.Provider/NewProvider`确有stub及HTTP实现并在providers装配，但它查询情报，不是风险生命周期来源，不能混报。
- **DataFetcher**：`pkg/datafetcher.DataFetcher/Registry.Fetch/ApplyFetches`真实存在，Create会调用；`app/providers.go`的`provideApp`实际传空`NewRegistry()`，未注册internal_db/activelist/external_http实际实现。测试中的mockFetcher不是生产装配；若首版启预填，须补ctx/超时/白名单/授权及映射，现ApplyFetches用Background，不能直接塞任意HTTP调用。
- **TaskClient**：`pkg/taskrunner.Client`及TaskrunnerService已签名装配，可提交/查询/管理现有任务；业务Action、工单结果关联和效果完成语义仍须按第3节补齐。
- **AL client**：UI的`src/api/al/index.ts`经主仓网关调用真实AL API；未见可供worker复用的受权后端AL client。网关的X-Operator是归因、AK/SK是服务认证，不能替代当前行/字段资格；首版必须实现选定port，不直接复用浏览器请求函数或直库。
- **消息**：ChannelSender为已封装interface，UIP为两态桩；webhook为真实告警HTTP。三者状态不能合并称“消息HTTP已接通”。utils/aksk可复用签名机制，不能代替上述业务授权和厂商认证。

## 7. 有界粗估、扣重与真实条件

**2026-11-14后继收窄**：下列59–91日是前一轮较宽施工画像的保守候选，**不是基础必需范围不可缩减的最低工期**。§2–6同时说明既有缺口及较宽场景；首版按以下受控画像验收，未启用的formula、通用HR、全量import/流产品和状态框架不共同前置，也不能把原数机械减去几个旧包价来制造新低价。

- **固定整改类型与本地目录**：首版选定schema不用formula；服务端按可信发布profile/类型白名单拒绝开启公式和修改相关配置，不靠UI隐藏或文档约定。未获首版支持的旧类型不能通过HTTP、内部调用或任务入口旁路写入。采用现有本地人员/组织和受审固定流程，真实UAA仍按公司要求接入；不先建通用HR同步、任意状态设计器或全历史定义迁移。实际流程若确需公式/动态配置，先补其适用正确性再启用。
- **证据仍是必需**：普通附件/所选证据字段的归属、确认、读取/删除、当前引用保护与事务回滚须实证；C02实际会走到的路径不能因不开放formula而删除。更广的字段类型/富文本引用路径只有明确禁止且反例证明不可绕过，才可从该批施工移出。
- **受监控AL类型只开放insert/update**：该类型暂禁replace/import及破坏schema的操作，直接AL API/签名调用也须拒绝，拒绝时不改数据、不产生任务。不支持的删除/恢复或其它变化触发不可配置；其余未受监控AL类型的既有功能并未整体退役。所选写入和任务更新仍有行/字段授权、CAS、同事务receipt及源事实。
- **一条可靠直通链**：AL同事务durable outbox→签名推送→core持久inbox/ACK→版本化单订阅→真正TR任务；按记录确认/重试，不先建全分页、多consumer、水位保留/重同步产品。稳定事件/operation身份、当前授权、重复/失序、重启恢复、有限预算及防写回循环照常验，不能用updated_at轮询或进程内Hook代替持久交接。
- **固定动作/流程**：一项受权Action、一条受审配置和当前节点关联即可，不先建通用Action市场、状态机/映射编辑器或完整调度恢复台。若首版开放周期任务，其稳定发生次和实际恢复义务仍需补齐；未开放能力不伪称已实现。

共享operation/意图/receipt的机制只计一次；各链只计本域接入和效果事务。链内故障用例与最终联合回归须按真实case分工，不能重复计同一套测试。以下旧六行保留为复估输入，包含后端、服务间集成、定向测试及必要运维，**不含前端实现和浏览器E2E编写**；已完成研究/版本升级、已有队列/CRUD/模板/签名不重造，也不在长期76包总价上再加一遍。

- **共享护栏12–19日**：C01/C02适用正确性、所选写路径的C04事务/当前授权、R01持久意图和选定operation骨架；不建完整统一层或迁移158项。与各链共享一次，不能再全加C04/R01/CE02父价。
- **风险源→整改15–23日**：一来源归一化、风险域/权限/有限状态、固定整改定义与派单/复核关联及收件人事实；从D06–D10/D15/X02相关范围抽取，不承诺完整HR/设计器/风险运营台。
- **工单→TR→结果/AL12–18日**：选定C03/D12/I01/I02/R06/CE02-D/CE03的输入/主体/receipt/AL CAS与结果投影纵切；只补当前缺口，不重做TR。持续委托和资源授权的适用部分必须实交，不能因完整I02后移而缺失。
- **AL变化→任务12–18日**：从CE04/CE05抽取源事务事实、水位/持久收件、单订阅、因果/容量及故障验收；复用上一项AL receipt与共享意图，不再收一遍。本期不建任意规则编辑器或全部多实例调度。
- **公司HTTP消息4–7日**：一渠道adapter、受理/失败合同、持久意图/去重/有限重试与字段映射；复用消息规则/模板和共享机制，D11/E01相关部分只归一次。前提是有可实现的一个HTTP协议画像，真实差异另复核。
- **跨链联合验收4–6日**：五服务固定组合、PG/Redis/HTTP故障与恢复、权限负例、试用安装/排障；UI自身用例仍归UI工作，不把同一测试重复计入。

较宽画像的后端候选合计仍记**59–91工程日**；若其UI另16–26日，条件视图为75–117。它们**均不是上面受控首版画像的新报价或最低工期**。旧六行未将共用与特有净日完全拆开，本次只收窄施工分母，不无依据另报更低区间；实施前须按固定schema、两种AL写及直通投递逐项复估，再与UI逐项对齐。通用G治理未完成的适用部分、额外迁移/拓扑另列；维护收尾沿既有预留，不能藏入数字，也不据此降低旧包总价。

公司UAA/消息真实联调时间目前**未定**。本估算不含UAA-P/R新增施工、提供方排队/审批/改造、额外厂商、真实数据迁移及全套生产运维；若试用即要求中央纳管，须追加适用UAA纵切并与C04/X02/I02扣重，不能简单全额相加，也不能用本地登录绕过公司要求。消息真实可用必须以提供方条件和实测为准。

阶梯验收可先固定组合与消息合成合同，再交风险—整改人工闭环，继而工单—TR—结果/AL，最后AL变化订阅；2027年1月试用需写清当时已通过哪条链。2月1日前四链全部可用是目标，当前一有效席位不能凭这些粗估作承诺；需按真实就绪情况、可独立研发产能和试用分母重新排程，不把多个agent直接折成人。

## 8. 证据限界

本次只读既有测试：`TestJobsCallback_SnapshotMismatchRejected/RealSubmitThenCallbackSemanticParams`、`TestTaskrunnerServiceSubmitRecordsVoucher`、TR的`TestSubmitA1Idempotent/TestSubmitConcurrentSameTaskID`及callback/reclaim测试、AL的`TestA2_CRUDLifecycle/TestA4_OptimisticLock/TestA5_ImportValidationRollback`、UIP桩与死信webhook测试。它们证明已有测试覆盖面，不是本轮执行结果，更不证明四条新链已经通过。

开工仍按测试先行冻结该批正负/边界/权限用例、取得真实红例，再实现与独立验收；不得将Schema/离线参考、已有253项验收或UI旧绿改标成新风险域、真HTTP、AL授权及UAA已完成。
