# Phase6 工单与前端模块详细设计

日期：2026-11-14。状态：**实现前设计候选，未开发、未完成产品验收**。本页补 D01–D05/D11/D16、U01–U07 及 LS01–LS04 的实施相关合同；范围、工日和状态仍归[总账](../planning/planning-backlog.json)、[净估](../planning/local-core-release-estimate.json)和[模块交接](module-design-handoff.md)，不另加一期或自动开始施工。

共同身份、operation/receipt、版本和错误沿[生命周期合同](../design/business-lifecycle-contracts.md)与[共同调用合同](../design/capability-invocation-contract.md)。本页把其工单部分展开成数据、事务和用例；与 00/06/08/09 的旧候选冲突时采用本页及现行共同合同，旧原文保留追溯。实际 HTTP/数值错误码、逻辑表物理命名在对应代码批核目录后登记；以下未新增可调用路由或迁移，也不预占编号。

## 1. 当前代码与阶梯范围

本次定向核对的现状：

- `internal/service/ticket/transition_service.go` 的 `Transition` 仍按 live 类型取图，已有 `LockTransitionSnapshotTx` 防预检后行/配置变化，尚非不可变发布快照；通用转换会写 SLA，但 Create/Assign/Close 入态尚未完整收敛 K2。
- `internal/model/ticket_request.go` 的 `TransitionTicketRequest` 仍为 `id/to/comment`，没有转换字段采集；类型 `version` 已有 CAS，不能冒充实例 revision 或 definition version。
- `internal/service/ticket/resource.go` 的 `Authorize/GetFilter/canOperate` 已有三层基础及 CC 读特例，尚无类型策略；`cc_service.go` 的 `AddCC/RemoveCC` 仍按 read/update 区分本人/他人，D03 要改为已批准的独立管理资格。`tag_service.go` 的 `AddTags` 已按单受权、规范化及幂等，不重建标签服务。
- `internal/service/ticket/type_admin.go` 的 `validateCustomData/applyFormulas` 已保留 schema 校验/重算；C01 同名引用和输入隔离、F11 非选项字段空 options 往返仍待修。Phase5 已修的 MIME、双模式保真、fresh 字段版本和未设置列宽不再重复报新功能。
- UI 的 `src/views/ticket/create/index.vue` 已将 file key 追加；单 file 控件尚非一次多选。`src/views/ticket/list/index.vue` 尚无批量标签；`detail/index.vue` 的能力投影与活动流仍待补。`src/router/routePermission.ts` 将 `/home` 列为 exempt，不代表默认页动态注册/Logo 落点问题已经解决。

**本地核心先交**：经过评审的固定流程和基本字段、不可变定义、完整的当前三层权限、受限类型、转换时采集字段与证据、唯一主责和 CC、K2、必要业务历史、风险复核责任投影、批量标签及最小可用界面（薄 UI）。首版只需实现实际启用字段和动作所需的合同，不要求全部设计器、LS01/02 或完整 D11/D16 先完成。

**完整 P6 逐步交**：D05 配置晋级、LS01 常驻字段 ACL、LS02 typed 检索、LS03 受权快速入口、LS04 完整字段diff、D16 自动路由，U03–U06 全站/配置器推广及 D11 通知增强。D01 的同名状态最小迁版随定义交付；删状态完整映射归 D18/P7。共享轮询/认领归 E03 的已排处置窗口，实际近期启用先取所需子范围并净估，不把整套队列藏进 D02。服务端草稿、会签/加签、移动端和通用工作流不在此批。

### 1.1 常用流程能力与 B2 边界

“已实现”以现行主仓代码为准。“B2 必需”指人工漏洞整改首条旅程的设计与验收范围。后置能力按现有工作包和触发窗口取舍。各项产品验收随实际候选记录。[B2 交付排期](../planning/intranet-uaa-release-schedule.md#1-接下来怎么做) · [两条风险验收卡](../planning/phase6-b2-acceptance-cards.md)

- **改变工单状态时检查什么。** 现有 `internal/service/ticket/transition_service.go` 的 `Transition` 在保存状态前检查工单和动作权限，也可查询可办动作并校验字段。`internal/service/ticket/transition.go` 的 `checkCondition` 核某个操作者是否满足动作要求的 `roles`，空数组表示没有额外角色限制。B2 计划让工单按创建时关联的规则办理。已有工单升级补记版本或获准改用另一版规则后，以关联的版本为准。每次办理仍要检查当前身份、类型范围、参与资格和复核职责。例如预览后风险负责人换人，原负责人再提交复核应被拒，即使动作没有额外角色限制。自动按条件选下一步（D16）后续只按优先级选一条。多个分支并行后汇合或按条件选择多条分支仍在引擎范围之外。 [规则版本示例](#33-发布迁版与配置晋级) · [引擎边界](00-flow-engine-v2.md#6-边界不做)
- **会签、或签与依次审批。** 同一环节全部同意是会签，任一人同意即可通过是或签，逐位发待办是依次审批。现行转换的准入判据对应单个操作者。B2 使用当前风险负责人的逐风险复核。延期双主管列为 Q10/E03 的后置轻审批候选。S3b 有多人待办、会签批次与回退蓝本，或签、临时加减签、审批代理及灵活通过门槛按真实场景裁决。[飞书审批手册](https://www.feishu.cn/hc/zh-CN/articles/360033971554-飞书审批管理员手册) · [S3b 蓝本](../phase5/03-s3-flow-semantics.md#4-s3b审批流引擎等首个真实审批场景)
- **驳回、补交及重提。** 已有配置状态边和转换评论。B2 必需逐项通过或打回，打回记录理由且保留原证据，只重提待补项。同票其余已生效结论继续保留。迟到的处理结果若携带的票、风险、关联或结论版本号已落后于当前值（即基于旧状态做出的判断），按对应入口拒绝或冲突。关票前，每条仍有效关联须有已授权的生效结论，合法移出须留因和历史。若启用风险接受变体，仍由当前 owner 申请、非申请人的获权主锚管理者确认，待确认项不能充作生效结论。普通关闭、风险关闭、转换及已启用的自动入口共用复核门，暂未接入的入口按实际范围受控关闭。通用审批的退回、回收旧待办与重新生成审批轮次归 S3b 的实际场景裁决。[风险复核合同](risk-module-detail.md#52-逐风险结论与review-gate)
- **分派、接单、转办、抄送和撤回。** 已实现普通分派、抄送及建单自动分派规则。B2 需要具名唯一主责、人工接单、最小类型可见性，按实际启用路径验参与来源和复核资格。完整转办、发起人撤回未受理且未分派的工单及更完整的协作入口后置，随 D02/D03 实际交付范围。合格池轮询、按类型认领和工作日历由 E03 逐项定。抄送和转办权限按各自职责判定。[转办与撤回对账](06-ticket-completeness.md#26件当前对账2026-11-14) · [E03](early-disposition-cards.md#2-e03分派日历轻审批与认证增强)
- **超时、催办与执行后动作。** 现行代码已有超时扫描、事件和通知/抄送/转派/任务注册动作。B2 需要人工接单、接单与业务截止分开计、首次接单超时的最小站内提醒，以及所选风险事件的必要持久意图和失败可查。手动催办、完整通知配置、跨仓自动链、实际外部送达按对应窗口。已提交事实与通知或后续动作回执分别核对。[现有动作](../phase5/03-s3-flow-semantics.md) · [B2 验收卡](../planning/phase6-b2-acceptance-cards.md#卡二-b2-两条合成风险共票并逐项复测)
- **发布、证据与历史。** 现在有工单类型配置版本、字段校验和状态历史，但办理时读取的是当前类型配置。B2 要保存每版发布规则，并让工单关联创建时的版本（D01）。在独立测试环境验证升级前的工单，能核实原规则才补记版本。办理下一步时按项提交有效证据（D02），同时保存业务结果、历史和需要继续处理的任务。两张用不同规则的工单都按当下权限办。跨环境部署配置（D05）、完整画布和活动流，以及删除状态后的批量迁版（D18）各按原计划。 [发布与迁版](#33-发布迁版与配置晋级)

### 1.2 双主管延期的 E03 轻审批候选

双主管延期在 B2 首批留后。按 [Q10](../planning/owner-decision-agenda.md#q10-工作日分派轻审批与报告配置) 评估一条专门的改期用例，由风险域保存申请、分别审批和改期事实，工单读取风险期限计算业务截止。以下接线是[延期业务合同](risk-module-detail.md#14-漏洞整改代表场景)的设计映射。按风险项申请是候选，实际粒度、两侧身份来源和并行或串行仍待绑定。实际获选后，再核适用范围、净估与排期并取得实施授权。若现场制度要求在真实试用前强制双签，按范围变更评估前移。

1. **申请与派给两侧。** 选在办风险与对应工单，绑定当前业务版本、申请版本、新截止、理由和缓解措施。风险侧主管的候选来源是实际发起人或具名风险责任人的主管，整改侧是当前整改责任人的直属主管。建议由两个不同主体分别代表两侧，禁止本人审批。同人命中两侧、缺员、停用或无法核实人员关系时留待处理，按 Q10 将来批准的替代规则分派。主管关系提供候选，动作资格与数据可见性须独立核当前权限。
2. **以同一版签署。** 每次确认记录主体、对应侧、申请版本、结论和时间。改期限、理由或措施生成新版并重审。负责人、主管或资格变化时按当前来源复验，失效确认须重新取得。建议待批继续计时。单签、驳回、失效或缺主管时保留原期限和原因，逾期事实继续记录。
3. **签齐后限权生效。** 风险域改期命令在业务提交边界重验执行主体资格、申请和业务基线、风险与关联状态，以及两侧当下资格和同版有效确认。换版、撤权、主管变更或关票先提交时，改期须拒绝或按实际原因待重审，原期限保留。成功时在同一事务更新获批风险的截止、重算整单最早截止、写历史与原操作回执。有效签署和改期生效分别回读。执行冲突或失败沿原操作核查，期限以实际已提交事实为准。不同操作号并发时，同一申请版本也只允许生效一次。保留原始截止、历史逾期、节点与首响时钟及复测结论。
4. **验所有改期入口。** 若启用强制双签，评估、普通编辑、票侧、批量及自动路径共享同一改期规则。票的业务截止由已生效风险期限与有效关联派生，票侧覆盖请求须拒绝。初次定期、合法关联变化和风险接受各沿原合同。验收至少覆盖单签、跨版本拼两签、审批后换人或撤权、审批与关票并发、重复生效，以及签齐后业务写失败并回读原期限。正例验证只改获批风险，其余风险期限照原事实参与重算。

### 1.3 何时启动通用审批设计

沿用 S3b 的真实审批场景触发。先拿到真实消费者、流程与具名职责，再看它是否需要可独立持久化并通用配置的审批任务。**触发评估的典型场景**包括以下任一类：

- **多个真实流程需要共用审批规则。** 假设具名的业务变更和安全例外流程都需要可配置审批人、审批顺序、共同待办和统一撤回及历史，可以比较共用任务层的收益。需要拿到流程实例和实际职责后才据此启动评估。风险接受第一档继续采用既定的 owner 申请与获权主锚管理者确认。
- **单个真实流程也可能需要通用任务层。** 当场景要求按输入动态生成审批人或轮次、配置通过门槛、临时加减签、代理或跨节点退回等规则，且每人的待办和决议需要独立留存、重算或取消，就值得评估 S3b。固定两侧对同一申请确认可沿 E03 业务域先验。

立项时用一条具名旅程冻结参与资格、待办/签署身份、审批汇总、拒绝/退回/撤回、异常重试、版本迁移和审计。比较现有 FSM 加领域审批、接专业流程和建设 S3b 的成本与验收证据，由所有者批准选型、范围和排期后再启动实现。若选 S3b，领取、签署、撤回和退回复用 L1、请求解析、L2、L3 的服务端守卫顺序。多人审批同一个节点可与单状态工单组合。并行办理分支与汇合若成实际需求，单独核执行模型、分支取消和恢复，再裁定是否使用专业编排。[S3b 触发与蓝本](../phase5/03-s3-flow-semantics.md#4-s3b审批流引擎等首个真实审批场景) · [Camunda 多实例任务](https://docs.camunda.io/docs/components/modeler/bpmn/multi-instance/)

验证与验收者（DeepSeek）和防御性安全与权限审查者（Codex）对同一初稿作了独立只读复核（摘录 SHA-256 `b5a6b3a77b4ce33a94c10cb1e23ca748c80cbb6ec90e41928272cadd1d8081ae`）。定稿前修订纳入既有守卫复用、逐项关票条件、双签失效与改期生效边界，以及真实场景评估和开工授权的时点。评审只覆盖设计与源码证据。

### 1.4 产品升级空间与演进方向

**当前建议是保留工单 FSM。** 单状态足以表达 B2 的工单生命周期，逐项复测的结论由风险域分别记录。以后按实际场景加审批任务或编排。easy-workflow 仅作设计参考，借鉴每人独立任务、同一轮次汇总和退回后识别新轮次的思路。烛照沿用自己的不可变定义、当前授权与事务合同。[上游任务批次源码](https://github.com/Bunny3th/easy-workflow/blob/c9889e9c4eca28996a549169238b4d379ed8a87b/workflow/engine/ProcessTask.go#L64-L101)

交付顺序及 B3–B5 发布验收点以现行排期为准。本节列产品能力方向，各项按真实依赖独立评估，轻审批可直接接在工单和风险域上。实际范围、工量、日期与实施授权按对应工作包确认。[现行发布排期](../planning/intranet-uaa-release-schedule.md#1-接下来怎么做)

#### 产品能力方向

- **B2 工单与领域复核。** 提供固定流程，工单记录所用的规则版本，支持提交证据和逐项复测。当前风险负责人的首页按工单计待复核数，同一张工单只计一次。首次接单超时发站内提醒。工单保存当前业务阶段，风险域保存各条关联、证据引用和结论。一票多风险时保留已生效项的结论及历史，待补项继续办理。[B2 范围](#11-常用流程能力与-b2-边界)
- **工单配置与单路自动化。** 沿 U04/D05/D16 提供发布预检、设计器、配置晋级和条件自动选边。用户可核发布版本、选中的边及执行回执。自动动作通过实际身份和提交时资格复验后生效，重复提交回读既有事实，撤权或求值异常按规则拒绝或暂停，并展示原因。功能按原工作包启用。[D16 自动路由](#5-字段安全检索快速入口与自动路由) · [发布与晋级](#33-发布迁版与配置晋级)
- **限定业务轻审批。** 风险延期是 Q10/E03 候选。风险域保存版本化申请、两侧决定及改期事实。用户可分别看到签署进度、重审原因和实际生效的截止。沿 §1.2 验双签资格、版本、重复、并发及写入失败。签署顺序和启用范围随 Q10/E03 确认。若现场制度要求真实试用前强制双签，按范围变更评估前移。[延期候选](#12-双主管延期的-e03-轻审批候选)
- **可复用的人工审批任务。** 具名场景需要按条件选人、生成轮次、设置通过门槛或支持转审及退回，并须独立跟踪每人的待办时，按 S3b 评估审批实例与逐人任务层。产品可共用待审批、已处理、失效及审计视图。验收可用退回后旧待办关闭、迟到签署拒绝、新轮次保留原签署历史来验证。把逐人任务和审批批次作为独立事实，工单仍可使用单状态。[S3b 评估条件](#13-何时启动通用审批设计) · [Jira 审批资源](https://developer.atlassian.com/cloud/jira/service-desk/rest/api-group-request/) · [Camunda 多实例](https://docs.camunda.io/docs/components/modeler/bpmn/multi-instance/)
- **并行活动与专业编排。** 若真实旅程出现多个不同活动各自等待和推进，并要求汇合、局部取消及故障恢复，再评估多活跃节点执行模型。假设技术复测和合规核验分别启动，需能各自回读进度，解释何时可汇合、取消后哪些迟到结果失效，以及恢复后如何确认只汇合一次。选用独立编排器时，编排侧拥有执行与等待事实，工单通过受控用例拥有业务状态。用具名场景比较现有 FSM 对场景的覆盖度，以及专业编排或新增执行层的运维、数据与验收成本。[Camunda 并行网关](https://docs.camunda.io/docs/components/modeler/bpmn/parallel-gateways/)

#### B2 要留稳的升级接缝

- **版本和操作者。** 使用 D01 已发布定义引用、D02 业务 `revision` 及共同 operation/receipt 合同表达当前请求。以后审批轮次可绑定实际业务版本和目标，提交及执行仍核当前身份、作用域与职责。定义版本、业务修订、当前授权各有自己的语义。
- **业务写主和统一复核门。** 后续审批决定和编排执行结果作为业务命令的受控输入。风险结论仍由风险域确认，工单状态仍由工单用例推进，应用事务内核主体、作用域、目标、当前职责和适用版本。关票要求每条仍有效关联都有获权的已生效结论，移出须留因和历史。普通关闭、风险关闭、转换及已启用的自动入口共用此门，暂缓入口按首批范围关闭。[逐风险结论与复核门](risk-module-detail.md#52-逐风险结论与review-gate)
- **结果重试及迟到处置。** 沿原操作号回读，后置动作按原回执恢复。单一决定对同一目标动作只生效一次，换操作号也须识别。携旧票、风险、关联或结论版本的迟到写入按当前入口拒绝或报冲突，后续任务按适用审批轮次或分支代次判断。B2 留必要持久意图和可查失败。完整待办投影随获选能力交付。
- **已有实例继续可办。** B2 在隔离环境验可追溯回填。发布新版后原票继续钉原版，两版操作均核当前权限。缺对应定义时写入返回 503、动作停用，只读明确标为 `degraded`（降级显示）。来源无可靠依据的历史标为未知，关票只计已生效结论。[发布与迁版](#33-发布迁版与配置晋级) · [Flowable 实例版本规则](https://documentation.flowable.com/latest/user/control/processes#process-instances)

后续显式迁版沿 §3.3。D01 最小迁版限同名且无删状态，逐票核字段、动作、资格及修订号。删状态映射留 D18。迁版保留原历史、已生效结论、证据、节点时间及当前期限，后置动作零重放。仍绑定旧版的未生效结果按目标定义及当前资格重新核验，冲突则明示拒绝或重审。

B2 首批验 D01–D04 的实际启用部分，用两条合成风险验证办理，并在隔离环境验证升级前已有的工单。审批任务、显式迁版、配置器、自动路由和并行编排按各自工作包裁决。[B2 两版隔离回归](../planning/phase6-b2-acceptance-cards.md#卡二-b2-两条合成风险共票并逐项复测)

验证与验收者（DeepSeek）和防御性安全与权限审查者（Codex）独立核读了本节同一初稿（摘录 SHA-256 `4dad42640f843a9406654e109e885583a597150eb3c4a6bdd562f40c2d157895`）。修订区分了能力方向与交付排期，补齐提交时复验、统一复核门、旧票降级与在途结果处置。复核范围为所提供的设计摘录。

## 2. 准确输入输出与权限

第 17 项第一批的存量类型保持 `inherit`。新建类型可在 `POST /api/v1/ticket-types` 声明 `read_policy` 和 `read_role_codes`，缺省为 `participants_only`。限制范围的类型由全局管理员管理持久抄送。行级策略由列表、详情、导出及 `GET /api/v1/tickets/stats` 共用。统计另验 `ticket:stats`，按 `status`、`priority` 或 `type_code` 返回计数、在办数、超期数和比率。零在办样本返回空比率。 [冻结口径](../planning/phase6-step17-ticket-implementation-plan.md#开工冻结)

### 2.1 类型、实例与版本

所有 ID 在 JSON 中为十进制字符串，时间采用带时区的 RFC3339。新请求与响应（DTO）中的 `revision`、`draft_revision`、`definition_version` 和 `authorization_epoch` 均为规范十进制字符串，服务端按有界非负整数解析，前端不用 JS number 运算。现有 `version` 的 number 类型只在明确的旧接口兼容层保留，新响应不能让同一字段有时为 number、有时为 string。`draft_revision` 用于检查管理草稿是否被并发修改（CAS）；`definition_version/hash` 标识发布内容；`revision` 是工单业务写入的序号；`authorization_epoch` 只用于授权失效与诊断，四者不能混用。请求不能自行指定可信操作者或 `created_by`。

```typescript
type Revision = string // 规范十进制，无前导零；零是否允许由各字段语义约束
type DefinitionRef = { type_code: string; definition_version: Revision; definition_hash: string }
type MutationBase = { expected_revision: Revision }
type CollectField = { key: string; required: boolean }
type ViewerCapabilities = {
  update: boolean; attach: boolean; comment: boolean; note: boolean;
  assign: boolean; transfer: boolean; manage_participants: boolean;
  manage_instance: boolean; urge: boolean; delete: boolean;
}
type AvailableAction = {
  action_id: string; to: string; label: string; collect_fields: CollectField[];
  comment_required: boolean; expected_revision: Revision; definition_ref: DefinitionRef;
}
type TransitionInput = MutationBase & {
  id: string; to: string; definition_version: Revision;
  custom_data_patch?: Record<string, unknown>; comment?: string;
}
type TicketView = {
  id: string; revision: Revision; definition_ref: DefinitionRef;
  definition_resolution: 'exact' | 'degraded'; status: string;
  assigned_to: string | null; viewer_capabilities: ViewerCapabilities;
  available_actions: AvailableAction[]; blocked_responsibility_reasons: string[];
}
```

这些类型描述公共语义。旧 available 接口的真实 data 为 `{transitions:string[]}`，不能直接改成 AvailableAction 数组。候选 `GET /api/v1/tickets/:id/actions` 返回完整动作对象，详情与可选 capabilities 汇总复用同一套结果。旧 Ticket 未分派时因 omitempty 省略 assigned_to，新 TicketView 才明确返回 null，因此响应结构有变化。现有只传 id/to/comment 的客户端短期仍依赖锁和判定谓词；传 expected_revision 才能防止基于旧内容提交。新 UI、SDK 和私有用例必须传 revision。

action_id首批为只读标识，写以definition_version＋to定位。新发布小FSM同from/to只一条边；preflight发现重复目标边须报告并受控修订，不任意选一条迁移或放宽为通用多边工作流。真实旧配置若需保留不同动作同目标，列具体场景再决，不假称只添action_id已解决写歧义。

| 用例/路由（候选或现有增量） | 输入与成功输出 | 准入及失败 |
| --- | --- | --- |
| definition plan / deploy | 沿生命周期 §3 的 `code/expected_draft_revision/expected_published_version`、plan/hash/引用revision；输出 `changed/definition_ref` | 类型管理功能＋配置职责；草稿、引用、发布指针漂移 409，不自动忽略 warning |
| 现有 create/update/transition | create 增量带预览 definition；update 增量带 revision；transition 采用上方 DTO，返回新 revision 和实际定义 | L1＋当前 L2＋动作/字段 L3；错误时零状态、字段、事件、后动作效果 |
| `POST /api/v1/tickets/transfer` | `id,to_user_id,expected_revision,comment?`；新 assigned_to/revision | 当前主责或独立获准管理者；目标 active、在批准组织/类型候选集；同状态不重置节点/SLA |
| `POST /api/v1/tickets/accept` | `id,expected_revision`；`accepted_at,revision` | 当前处理人；只允许本类型可接单阶段。简单流程可在一次业务用例内接单并进入配置的处理态，不靠前端连发两次 |
| `POST /api/v1/tickets/manage` | `id,expected_revision,reason,description?,custom_data_patch?` | 类型/组织实例管理职责＋字段白名单；不能借类型编辑权改实例，不能改历史/来源/公式/域资格字段 |
| participants list/change | 沿生命周期 §5 的角色、origin、add/remove、revision；新增 active，受管项不能由通用入口删 | `ticket:participant:manage` 按钮＋当前对象管理范围；创建人不自动可邀人；CC 不附送办理权 |
| fields evaluate/prefill | 沿生命周期 §6，带创建/实例上下文及 `input_generation`；输出 visibility/computed/errors/dependency 状态 | 普通端只处理获权字段；设计器未保存 schema 只在管理模式；非法表达式/依赖失败不伪成功 |
| stats / home summary | `view=personal/type_operations`、白名单筛选；输出 `as_of,definition_of_metric,total,items/filter_token` | 先服务器行/字段过滤再聚合；类型运营单独授权，不用个人待办替代全局统计 |

错误沿 `errcode` 及现有 envelope。400 表示字段/动作不成立；401 会话无效；403 为票可见但动作禁止；不可见/不存在同形 404；409 为 revision/定义/幂等/状态竞争；429 为可重试预算耗尽（如催办节流）；503 为权威依赖不可用。响应可携稳定 `reason`、安全的字段错误和 `request_id`；数据库细节、不可见字段值或成员不放入错误。

### 2.2 字段与策略不是同一种授权

默认 `participants_only`，存量 `inherit`；`role_restricted` 加指定有效角色，业务管理职责另带组织锚点。没有 `is_sensitive` 新体系，`visibility_override` 不启用。旧连字符写法仅历史草案，产品首发采用生命周期的下划线规范值，配置导入只做显式归一化，不允许两个值在核心分叉。

| 主体 | 读/评论 | 转换、字段与管理 |
| --- | --- | --- |
| 主责/发起人 | 仍需 L1，且票的当前策略允许；分别保留各自职责 | 当前边资格＋状态＋获权字段；发起人不因此可邀请他人 |
| 普通 participant / CC | 合法授予后查看、评论；内部备注仍独立受权 | 不因此获得转派、普通编辑、状态转换或管理参与人 |
| 同部门、普通 scope=all、组织 owner | 仅 inherit 或显式配置的受限范围 | 不能穿透新类型限制；管理者也不自动成为责任待办 |
| 受权类型业务管理者 | 仅该类型与所管组织的交集，策略须显式包含其职责 | 每个干预动作/字段另验，并填原因；不能绕 validator/证据 |
| 系统级 admin/superadmin | 保留既有受控例外 | 仍受业务不变量/目标状态/证据及审计约束；外部同名角色不等价 |
| 风险复核人/受限服务 | 具名领域桥维护当前职责及必要参与来源 | `field:<key>` 取写前权威值；不能本次 patch 把自己设成复核人 |

`viewer_capabilities` 和 actions 只帮助前端展示，实际提交重新授权；缺能力字段默认不展示写入口。comment 仍经当前功能许可，不把 CC 的域资格解释为跳过 L1。类型配置、实例管理、参与人管理、审计检索四种职责分开，复用现有组织/虚拟组织授予，不增加一套全局管理员。

## 3. 逻辑数据、事务与恢复

### 3.1 候选 DDL 与存量处理

下表是实现批要生成的逻辑 schema/约束，不是已执行迁移。各域只改自己数据，外部 adapter 不能引用内部表。

| 逻辑实体 | 必要列/约束/索引 | 写主与迁移 |
| --- | --- | --- |
| `ticket_type_definitions` | `type_code,definition_version BIGINT,definition_hash,schema_format_version,snapshot JSONB,source_draft_revision,published_by,published_at`；PK `(type_code,definition_version)`；snapshot 必须 object，version>0；hash 普通索引 | 工单定义用例追加；不可 update。内容回退可产生新版本，不用全历史 hash 唯一约束禁止 A→B→A |
| `ticket_field_identities` | `type_code,field_id,field_key,created_at,retired_at`；PK `(type_code,field_id)`，有效key在类型内唯一；发布snapshot保存field_id | 字段身份独立于Replace全删重插产生的行ID；改显示名/布局保留身份，真正删除后复用key创建新身份；既有当前字段一次性映射，不伪造已丢历史身份 |
| `ticket_types` 增量 | `current_published_version`；草稿仍用现有 version；当前指针引用同类型定义 | 先允许空并回填，核零悬挂后约束；类型删除须核所有票/计划/当前引用 |
| `tickets` 增量 | `definition_version,revision BIGINT,lifecycle_cycle INT`；引用同类型定义；revision>0；`(type_code,definition_version,status)` 索引支持已有工单及迁版 | 每种类型保存初版。升级前已有的工单有可信依据才补记规则版本。规则来源未知的工单记录到迁移结果，按缺定义只读处理。原历史中无法核实的内容保留并标明来源未知。revision 初始化后每次相关业务写递增 |
| `ticket_events` 增量 | `event_id,cycle_no,visit_no,payload,operation_id,definition_version,aggregate_revision`；`UNIQUE(ticket_id,aggregate_revision,event_kind,event_ordinal)`；`(ticket_id,created_at,event_id)` 分页 | 与状态/字段/参与写同事务；新事件可重放，旧无payload事件不补造；file/richtext 记键/受权引用，正文不进事件镜像 |
| `ticket_state_visits` | `ticket_id,cycle_no,visit_no,state,definition_version,entered_at,exited_at`；PK `(ticket_id,visit_no)`；每票未离态最多一行；`exited_at>=entered_at` | 统一入态核创建/结束区间；同状态转派不新增 visit；终态驻留不计办理时长；重开开始新 cycle 但 visit_no 继续递增 |
| `ticket_milestones` | `ticket_id,cycle_no,kind,occurrence_key,occurred_at,actor_id,event_id`；唯一 `(ticket_id,cycle_no,kind,occurrence_key)`；kind 固定 `accepted/processing_started/result_submitted/business_closed` | 首次接单 occurrence_key 固定 `first`；节点办理点绑定 visit；提交/重开保留原事件，指标能区分首次与后续周期；不保存可被业务管理员任改的人工时间 |
| `ticket_member_sources` | `(ticket_id,user_id,role,origin,source_key)` 唯一，role 仅 participant/cc；持久origin为manual及获准domain（如risk_review）；`(ticket_id,user_id,role)` 索引 | 只保存不能从权威字段推导的独立授予；现有cc回填manual。creator/assignee从当前CreatedBy/AssignedTo计算，不复制成需同步的来源行 |
| 发布/迁版/晋级 plan | plan id、类型/目标/包hash、各基线revision、选中票及revision、警告确认、审批者/到期时间、结果receipt引用 | 复用 D05 的受控计划模型和共同 operation 记录，不再建第二套通用任务/receipt/outbox表；plan过期或基线变化409 |

成员来源表只在D03最小实现，不引通用关系引擎。manual的source_key固定manual，各user/role只有一个手动来源；domain用风险link等可信稳定key，public add/remove不自报origin，remove仅manual。creator/assignee仅只读虚拟origin，权威仍票字段。同一人手工参与＋risk_review，交接只撤对应risk源，其它源保留但不保已转出的复核权；来源投影不暴露隐藏risk ID/数量。TICKET-D01已由F019确认：今后所有新增CC含自加均需独立管理授权，旧inherit无自加例外；已有合法绑定不自动撤。本人仅退自己manual CC，不删domain/强制来源，退出后按剩余有效来源重新求权；所有读取仍核当前L1和本票查看规则，保留CC不等于永久可读。

所有迁移 up/down 成对。扩列、初版定义、批量回填、校验、读写切换、非空/外键分步交；迁移可幂等恢复。down 前停新写、导出新增快照/历史、验证目标旧代码能读，不能删掉已有新定义后让旧单静默读最新版。新增历史事实启用后回退代码只保留其数据，不凭空恢复原来的 live 语义。实际物理拆分/SQL及编号在代码批按迁移地图复核。

### 3.2 一次转换的提交算法

1. 在持锁前解析请求、验证大小/语法；必要外部取数在请求预算内完成，并保留经平台核实的来源引用。客户端传回的建议不成为受管权威值。
2. 事务先取 C04 当前授权守卫（排序 stable key），再取类型 live 策略/发布引用的共享锁；涉及风险桥时遵共同 `group→risk IDs→ticket IDs→link IDs` 顺序，普通工单直接取按 ID 排序的票锁。定义内容不可变，不反向锁草稿。
3. 读锁内当前值，重验身份、L1/L2/L3、revision、定义、合法边及写前执行资格。转换只接受该边 collect_fields 的普通字段 patch，domain/formula 字段拒写；同源显隐、公式/G2、业务 validator/证据均成功才写。
4. 在同一事务中按预期版本更新（CAS）字段、状态和 revision。真正进入新状态时，结束旧状态驻留记录（visit），开启新记录，并以同一数据库时刻更新 status_changed_at、SLA 和本轮节点提醒的代次，同时生成事件与可靠处理意图（intent）。任何一项失败都回滚，不能只写状态或提前发通知。
5. commit 后返回业务提交结果。派单/消息/TR/自动下一跳由 R01 持久 intent 交接；只在其 receipt 证明成功后显示对应成功状态。通知失败或后续转派失败不回滚已提交状态，显示待处理/失败；只重试该后继，不重复整次转换。

Create/Assign/Close/Transfer/领域桥/自动路由调用同一入态与用例核，禁止各写一次 SLA 或绕过状态/字段校验。Create 进入初态、Assign 确实改变状态、Close 进入终态分别按 snapshot 处理 SLA；Assign 仅换人、接单、备注、迁版、改字段都不重置原节点 due。旧 `clearReminderWatermark` 的 commit后删除不能作为新语义唯一依据，改按 visit/代次隔离水位，旧删除仅清理；崩溃后不因旧水位抑制新一轮。

PG 行锁/死锁处理沿[PostgreSQL 18 官方锁说明](https://www.postgresql.org/docs/18/explicit-locking.html)的短事务与统一锁序模式。本项目采用数据库唯一/CAS 作为最终防线，进程 mutex 不作跨实例正确性保证；仅明确可重试的死锁/序列化错误作有限退避，409 不自动覆盖人类编辑。

### 3.3 发布、迁版与配置晋级

**第 4–5 项隔离候选**使用 `POST /ticket-types/definitions/plan` 审看草稿与引用，使用 `POST /ticket-types/definitions/publish` 发布。发布所需菜单权限为 `ticket:type:publish`。提交时核对计划、草稿修订、已发布指针和引用，成功后写入快照、指针和原操作回执。普通建单页面从 `GET /ticket-types/:code/published` 读取已发布表单，并提交展示的定义版本及摘要。后端同事务重核当前已发布指针、模板和 IAM，草稿变化不会影响按已发布版本建单。

迁移 `000084_ticket_definitions` 为可发布的既有类型保存切换基线，使升级后仍可以建新单；早于切换的工单保留空版本引用。可信旧单映射由 `bind-ticket-definitions` 逐工单核对所选已发布规则、工单原始事实和依据摘要。补绑只添加规则引用与回执，保留业务修订、状态、时间、期限和历史；缺依据或缺可用规则的旧票可按原有查看权限阅读。

当前切片的 `GET /tickets/:id/actions` 提供按实例绑定规则可用的动作和字段。版本化转换用 `Idempotency-Key`、预期修订、定义版本和本次证据补丁提交。提交事务写证据、附件绑定、状态与时间、SLA、既有状态事件和原结果回执。按单通用按钮能力、字段级事件差异、驻留区间及跨域可靠后继由相应后续工作包交付。 [隔离验证记录](../review/reports/phase6-b2-stage5-acceptance-2026-11-22.md)

deploy 在类型行锁内校验 plan、父草稿revision、current指针与引用语义revision；复制整份图/字段快照，写新定义/指针/成功历史同事务。相同当前内容无变化返回 changed=false；重复操作号返回原receipt。保存草稿不发布，管理 UI 必须区分“已保存草稿”和“已发布”；旧配置写客户端须升级或受控拒写，不偷偷自动 deploy。

工单默认继续使用创建时关联的已发布规则。若找不到对应规则，允许有权限的用户查看可确认的信息，界面应提示“只能查看，原流程规则缺失”。此时关闭办理动作，修改请求返回 503（`definition_resolution='degraded'`）。已绑定工单改用另一版规则仍在受控迁版设计范围。首批拟支持两版有同名状态且没有删除状态的情况。每张先预览，再核当前记录是否仍与预览一致后执行。保留进入当前状态的时间和原期限，已经发生的动作不再执行。单张遇到冲突可单独处理。即使状态同名，也要核字段和资格是否兼容。删除状态后的完整映射由 Phase 7 的 D18 承接。 [设计参考](https://docs.camunda.io/docs/components/concepts/process-instance-migration/)

例如发布 B 版时新增必填项“复测环境”，已经用 A 版创建的 T-201 继续按 A 办，发布 B 后按 B 创建的 T-305 才要填这一项。工单在 `definition_ref.definition_version` 保存规则版本，每次保存都在同一事务内重新检查当前操作权限。升级前已有的 T-100 要根据可信旧配置补上版本。无法核实的历史结论标明来源未知。 [试用与两项隔离测试](../planning/phase6-b2-kickoff-plan.md#旧票范围与定向验收)

D05 继续[配置晋级提案](../proposal/config-promotion.md)：包仅含已发布类型/完整字段和选定模板、稳定外部引用；不带用户ID、真实URL、密码或任意脚本。plan 绑定包hash/目标/审批/目标基线，apply 整包短事务校验引用与审批并发布，等价重放不增版；模板独立配置语义不塞进实例定义。审计写失败的事务政策按该用例既有合同执行；SetRoles/AssignMenus两IAM操作的本地成功审计失败回滚已由F017另行确认、待C接线，不扩大其适用范围。补偿是发布前后受控计划，不等于灾备恢复。

## 4. 工单常见业务与异常出口

### 4.1 主责、接单、管理与时间

**第 17 项第一批命令。** `POST /api/v1/tickets/transfer` 接收 `id`、`to_user_id`、`expected_revision` 和可选 `comment`。`POST /api/v1/tickets/withdraw` 接收 `id`、`expected_revision` 和可选 `comment`。两条命令使用 `Idempotency-Key`，返回原 `id`、`assigned_to`、`revision` 和 `status`。`GET /api/v1/tickets/:id/actions` 提供当前修订、转办/撤回资格及按绑定规则要求的必填说明。`GET /api/v1/tickets/people` 的 `purpose=transfer` 与提交共用目标当前直属组织资格、详情及转换入口授权。新写入口各自绑定按钮。转办改变主责，保留节点、定义、时间、SLA 和原提醒事实。重放复核当前入口和可见范围，再读回原提交结果。转办后失去最后一条读取来源时，原处理人的重放随当前范围返回 404。

撤回须由发起人本人在已绑定规则允许的 `open` 状态下执行。新建记录的撤回资格在首次分派或离开 `open` 后永久失效。存量工单缺少可信资格时只按现有动作办理。发布图中的 `cancelled` 是发起人专用撤回终态，使用唯一的 `open` 入边并且没有出边。旧图仍可读取，`cancelled` 转换须满足受控撤回合同。风险在办单按风险领域职责办理。 [首批冻结与验收](../planning/phase6-step17-ticket-implementation-plan.md#开工冻结)

唯一 `assigned_to` 继续单值；多人协作/CC/复核分别建职责。未分派必须显示当前管理组织及“待分派”，不随机给无资格的人。共享任务的未来 E03 默认合格池轮询，特定类型认领用单赢家 CAS；资格变化重新筛候选，空池 hold。受限票不因进入候选池向全池开放，须显式获准类型队列摘要，否则仅当前可见者可认领。参考[JSM 队列分派](https://support.atlassian.com/jira-service-management-cloud/docs/assign-requests-from-your-queue/)的分派视图，首版复用现固定规则；不为“随机分人”引入优化调度器。

接单为持久人工事实，自动分派/查看页面/机器人评论不计响应。固定指标：首次接单响应=`首个合法人工accepted−created_at`；节点驻留按 visit；处理历时按 processing_started→对应 result_submitted；整单闭环=created→最终业务确认。归档若只是技术动作不冒充业务完成；重开新周期、保留首次闭环。未接单/未结束样本不当0，未记录的旧历史为 unavailable。默认自然历时；工作日历/暂停有明确配置才另算，不能通过管理员补字/改派/接单重置K2。借鉴[JSM SLA 起停条件](https://support.atlassian.com/jira-service-management-cloud/docs/set-up-sla-conditions/)可显式配置的思路，首版保留四类可解释事实，不复制完整 SLA 产品。

类型业务管理员只能在所管类型/组织内干预获准字段与动作，原因必填，事件记录真实人、前后键级差异及revision。关闭、跳节点、覆盖复核结论没有通用“强制”旁路。补充描述不能改源平台正本或历史时间。系统管理员可见例外不等于免业务 validator。

### 4.2 文件、草稿、标签与通知

file 控件默认支持多选，上传成功的文件逐个追加；上传 B 不替换已有的 A，按 object key 去重，每个失败项保留可重试状态。建单前移除文件，只是不把它纳入提交；已建单移除字段引用，不会删除工单附件。显式删除须在同一事务核对当前字段和正文的全部引用，仍被引用时返回 409。不能用文件名猜归属，也不能绑定别张工单的 key。C02 负责与 GC 互斥，并让 Update 使用同样的绑定规则；现稿以 `custom_data_patch` 和 `collect_fields` 为规范字段，取代草案中的 `fields` 和 `fields_on_transition`，不改原附件下载鉴权。

本机草稿以 `user_id+type_code+definition_version+form_kind` 隔离，内容带schema/hash和更新时间；恢复前先取当前定义/权限，仅恢复允许普通字段并列差异。退出/换账号清内存与对应草稿，凭据、秘密和预签URL不持久化；file key恢复先向服务端核确认/归属/过期，失效显示需重传，不自动提交。存储不可用/超额提示“未保存本机草稿”，保留当前表单；没有服务端 draft 工单状态。

批量标签首版按净估 UI/U8 **复用现 `AddTags` API**：仅显式所选ID，跨页保留稳定集合；改变查询/账号清空选择；展示已选数、处理进度、成功/失败/未发送，失败可按相同标签集合和ID重试。有限并发默认3、最多50票是技术候选，实际预算由压测确认；取消只停止未发送，不回滚已成功标签。逐票当前授权，不把筛选总数当“全选全部”；同tag重复请求幂等。无需新batch后端接口或通用批量编辑器；UI仓旧batch路由推荐在实现前回填为此最终选型，不能两者同时计价。

D11 的业务配置采用有限“类型/流程版本/节点事件×收件职责×渠道”矩阵：待处理主责、待复核具名复核人、关键结果CC；强制风险义务不被普通偏好取消。保存期校验事件/职责/模板及发布引用，业务发生后交可靠意图，发送时重验收件人/字段；站内和HTTP渠道选择同源。预览不发送、测试发送只到授权测试收件人并带标记，不能冒充真实业务通知。催办采用稳定票/操作者60s节流，消息失败不退业务、不广播猜人。predue/digest以受控时钟/时区及持久水位恢复，具体B08真实映射由启用前绑定；消息输送准确合同由消息模块详设承接。借鉴[Jira 分开的关注/评论/附件权限](https://support.atlassian.com/jira-cloud-administration/docs/voters-watchers-comment-and-attachment-permissions/)，本项目CC资格仍以B02/B04为准，不能把关注理解为自动扩权。

## 5. 字段安全、检索、快速入口与自动路由

这些是 P6.3 后续功能，使用有限基本字段的首版不必等待整包交付。若新增真正需要保密的字段，必须先实现它所需的最小保护，不能只用 visible_when 或前端隐藏代替权限控制。

**LS01** 按角色展开后的固定决策表执行键 `(type_code,state,field_id,role_code)`，field_id为稳定公开身份，由可信schema解析到field_key，改名/删除后复用key不能继承旧字段授权，执行值为 `readable/editable/required`；schema安全策略与业务定义修订分别有CAS，管理入口须独立权限和影响预览。无规则维持旧基础字段语义；有规则为固定白名单grant求并集，再与当前行、domain/write_authority、动作、状态约束求交，`required`不能授写权；准确规则对象沿原LS01的`read_role_codes/write_role_codes/required_on_write`，写角色必须为读角色子集。冲突/不可读必填/资格字段可自写在发布期拒绝。受权角色三源有效展开，但带组织的业务管理职责必须求组织交集，不能仅拿role字符串全局放开。

候选表 `ticket_field_policies(policy_id,type_code,policy_revision,rules JSONB,updated_by)`（rules内稳定field_id，不绑定易变field_key）＋按类型唯一当前指针，发布历史只追加。读出口一律调用统一字段投影：详情、列表动态列、导出、事件旧值、快照、公式产物、错误、任务结果、通知。不能筛选/排序/分组无读权字段；公式/显隐依赖不可读输入时，按原LS01返回受限/不可用，不能经计算产物旁路泄露秘密输入；不回显环境。写前复验策略revision，撤权后下载/异步结果也重新授权。不能承诺“所有公式都可公开”，需字段所有者明确产物可见范围。

**LS02** 候选 `POST /api/v1/tickets/search`：`type_code,definition_versions?,filters:[{field_id,op,value}],sort?,page,page_size`。field_id为已发布字段的稳定公开标识，由可信schema映射到内部field_key/存储路径，不开放任意路径。首组单层AND；同字段in/contains_any表达有限OR。运算符首组：text eq/contains（有长度与预算）、number/date eq/range、bool eq、select/user/dept eq/in、multi_select contains_any，以及is_missing/is_null/is_empty；数组/NULL/缺键各有精确定义，不让null字符串等同null。一次最多10条件、in最多50值、page_size≤100为待性能实测初值；与[LS02合同](../design/legacy-scope-completion.md#2-类型化custom_data检索补p6-t019)同源。保存schema白名单生成绑定参数SQL，禁止任意JSONPath/SQL/列名。先行/字段授权再谓词，计数和导出同一query model；预算超限400/429而非退成全量。

旧数据与字段换型在定义preflight/离线校验识别，查询只转换经验证的同类型值；坏值列为不匹配并记录质量计数，不执行可能因一条脏字符串使全查询500的无保护cast。首组热点采用受控表达式/部分索引并对实际 `EXPLAIN` 验证；不为所有JSON键建索引，不在首版要求通用检索平台。

**LS03** Cmd/Ctrl+K 最薄入口只搜索当前可见导航和完整工单号，不做跨域全文/命令执行。候选 `QuickEntryQueryV1={kind:'navigation'|'ticket_number',query:string,limit:1..10}`；navigation在当前后端下发可见菜单集合做本机匹配，ticket_number只接受完整登记的`human_key`格式并调用受权精确查找端口（候选`GET /api/v1/tickets/lookup?human_key=...`）。返回`{items:[{kind,label,target}],as_of}`；target由可信路由映射生成，不接受用户自填URL/脚本。无票读资格不发票查询，当前L2不可见/不存在均空列表，不返回标题、数量或不同原因；依赖错误显示不可用而非“没有结果”。查询长度/频率有界，用户换账号/授权变更清空缓存与结果，旧响应generation不匹配丢弃；不可把旧可见菜单当永久授权。

快捷面板打开聚焦输入、上下/Enter选、Esc关并归还原焦点；输入法composition期间不拦快捷键；窄屏提供可命名按钮。选中后目标页仍做权限核验，撤权时给安全反馈，不为了方便预取全部工单。验收至少`QuickEntry_NoPermissionNoTicketRequest`、`QuickEntry_HiddenAndMissingSameResult`、`QuickEntry_RevokeAndLateResponse`、`QuickEntry_KeyboardAndIME`，按LS03/P6.3单独计净增，依U01/U02/D03，不藏入通用U06样式价。

**LS04** 完整字段真值diff在P6.3补D02最小状态/采集历史之外的全写入口。稳定事件形态为：

```typescript
type FieldChangeValue = { present: boolean; value?: unknown; value_hash?: string; evidence_ref?: string }
type FieldChange = {
  field_id: string; change: 'set' | 'remove';
  before: FieldChangeValue; after: FieldChangeValue;
  value_recording: 'inline' | 'reference' | 'keys_only';
}
type TicketFieldsChangedV1 = {
  event_id: string; ticket_id: string; aggregate_revision: Revision;
  definition_ref: DefinitionRef; operation_id: string; occurred_at: string;
  actor: { principal_id: string; kind: 'human' | 'service'; operator_user_id: string | null };
  changes: FieldChange[];
}
```

diff函数接收锁内旧真值与规范化/G2/服务端公式/领域更新后的新真值，比较实际差异，不用请求patch当after，不把未变化的重传字段记为变化。present区别缺键与显式null，数组保留顺序语义，数字/日期按schema规范化；工单票据、定义、actor和operation由服务端写。普通安全小标量可inline；file/richtext只记稳定对象/字段键及hash/受控证据引用，秘密不复制原文到事件、普通审计或通知。payload大小有界，需完整证据时写受控引用，不静默截断后冒称完整值。

差异记录与该次业务写入、revision、节点事实和可靠 intent 在同一事务持久化；事务回滚时不能留下成功 diff。创建初始化、更新、转换采集、管理干预和受权结果映射均复用该函数。结果 receipt 只说明任务事实，不能替代实际字段更改事件。业务效果已经提交而活动流尚未更新时，要明确显示“效果已发生，投影待补”；补齐活动流不能再次执行原业务动作。

读取历史先当前票L2，再按**当前**字段ACL投影；无读权字段整条change省略，不露field_id、名字、旧值、变化数量或“某秘密被修改”提示。删除/重命名字段按事件记录的稳定field_id解析，不把新同名key的权限移给旧值；缺可信定义/权限时保守隐藏值，可只显示获权的普通操作类型。event_id和revision为稳定游标，不以时间戳排序唯一；`after_event_id/page_size`有界分页，重复页不重复活动项。`FieldDiff_NormalizedTruth`、`FieldDiff_AtomicRollbackAndReplay`、`FieldDiff_CurrentACLNoFieldSideChannel`、`FieldDiff_RenameAndMissingDefinition`覆盖真值、失败、重复、撤权与旧定义；本包依D02/LS01，通用活动流展示只在U03计一次。

**D07预填接缝**沿[生命周期 §6](../design/business-lifecycle-contracts.md#6-d04--d07字段求值约束与可替换数据查询)准确`{source_ref,query_ref,params,mapping}`合同；params显式区分`{field:...}`与`{const:...}`，一个查询可映射多个获准普通字段，多参数依赖有环保存期拒绝、最多三趟有界收敛。旧`source:{type,config}/trigger_by`由核心兼容层规范化，不泄露sourceConfig/URL/secret给普通调用者。建议值可人工编辑，但发布配置规定的“总是补空”必须明示；用户改了输入generation后旧结果不覆盖人工值，数字0/false不是空。domain/write_authority字段不能由prefill获得写权；权威字段只消费平台核实的来源引用。正常一查多填、未命中、依赖失联、同参去重、多跳超限、人工更改和撤权分别在D07合同/风险详设核验，工单只消费被允许的建议，不另造取数协议。

**D16** 自动路由只处理发布定义中明确标为 automatic 的出边。条件使用同一个求值器，按显式 priority 选择首条命中边，并要求恰有一条默认边；人工 actions 列表不显示自动边。命中后开启新的短事务，用 CAS 检查状态与版本是否仍符合预期，重新核验当前受限服务的职责、源状态、revision、字段、G2、validator 和证据；自动执行只跳过人工角色资格检查。

业务提交事务保存可靠唤醒 intent，提交后处理，扫描器补查提交后未能交接的情况。多个实例领取待执行项时，沿 R01 的租约与防旧执行者提交标记（lease/fence）控制，不能只靠本地 goroutine 保证正确性。自动链深的候选上限为 16，并在静态检查时拒绝明显自动环；达到上限时暂停（hold）并展示原因，不能静默办结。默认边只处理“所有条件均合法求值但没有命中”的情况；无匹配结果不能用来掩盖非法表达式或权威依赖错误。分派 condition 沿同一词表，旧 keyword 仍兼容，规则优先级明确稳定，不依赖 JSON 对象顺序。

## 6. U01–U07 页面与状态详细合同

视觉数值和27页清单继续以[UI施工合同](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/main/docs/design/frontend-implementation-contract.md)、[页面清单](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/main/docs/design/frontend-pages.json)为权威；本页只明确跨模块数据和行为。现有UI库、品牌与Vue/Element Plus继续使用，不增一套组件系统。

| 包 | 本批交付与明确结果 | 失败/权限/交互验收 |
| --- | --- | --- |
| U01 | `/home`为公共安全落点；登录、Logo、根路径、404返回用同一safeLanding；常量注册安全首页/自服务 | 空业务菜单仍进首页，零工单/任务请求；旧深链无权落403/安全页，无导航循环；账号切换清动态路由 |
| U02 | 单侧栏＋身份顶栏；PageHeader唯一主操作；列表FilterBar/ProTable、卡头/状态/SLA基础tokens冻结；首页责任数据样板 | 主副文字不塞实现术语；卡头只放短标题/必要数，规则算法移帮助详情；长中文、缩放、900高和窄宽不溢出；加载/错误不闪0 |
| U03 | 四视角、human_key展示与精确查找、普通组合筛选、详情双栏活动流、草稿、多选追加/批量标签 | 四视角均叠当前L2；“已办”取真实历史动作且当前仍可见，不把当前已关票当全量已办；详情主处理/管理动作分区；多动作没有默认破坏性主按钮 |
| U04 | 类型/字段/流程独立页，步骤导航及右属性抽屉；显隐/公式构建器产同一表达式；专家JSON次级；预览共用实际渲染器 | 保存草稿/发布分开；409保留本地草稿与服务器新值供重载/对比，不自动覆写；预览file默认不真正上传；未保存离开确认；旧col_span=1不批量改值 |
| U05 | 左侧本机最近条件、右侧输入/当前结果；历史仅回填，明确“重新查询”才请求；显示来源和本次获取时间 | 请求generation/Abort取消与迟到抑制；失败保留上次结果并标陈旧，首次失败不显示空结果；按用户隔离，不把本机历史当审计/跨端快照 |
| U06 | 按27页清单逐页迁现有首页/任务/AL/消息/系统管理；新增业务薄UI归其业务批 | 所有适用页面涵盖初读/刷新、成功空/筛选空、无权/撤权、失败重试、长文本、键盘、当前版本；改共用组件不等于全部页面已关闭 |
| U07 | 代表真实组件/页面和角色旅程基线；视觉/键盘/焦点/状态/权限请求及console异常验收 | 不只断pageerror；Vue errorHandler/console error也失败；对比度、聚焦、状态播报与真实栈关键链路同时核 |

**工单界面已排重设计。** P6.1 的 U02/U07 先交视觉与组件样板、浏览器验收底座。P6.3 的 U03 承接日常工单列表、详情及发起/编辑的草稿和附件体验。U04 承接类型、字段和流程配置工作区。首条人工整改旅程先验实际使用的薄 UI，完整工作区在 U03/U04 验收。

两轮[Web 易用性复验](../review/reports/phase6-web-ux-audit-2026-11-20.md)把 U01/U03 的入口与返回具体化为保留待办深链查询、恢复手工筛选与安全返回，给 U02/U07 补上键盘入口、背景焦点、Esc 逐层退出、窄屏分页和主题色对照。U06 的消息及头像裁剪沿原事项补实测场景。结论对应审计基线的现有界面，实施与验收仍按现有包跟踪。

U06系统管理用户页的当前安全修复须前移随C04交，不等待全站外观：角色/组织弹窗加载失败或pending不许整体保存；甲/乙迟到响应绑定目标和请求世代，组织失败不能沿用甲的选择。当前真实SFC函数已复现该问题，[第二轮复验](../review/reports/permission-boundary-recheck-2026-11-15.md)区分script替身证据与待补真实浏览器回归。加入`UserRoleEditor_LoadFailureNoReplacement/UserOrgEditor_LateResponseBoundToTarget`，不是只给按钮加loading。

个人首页候选 `GET /api/v1/workbench/summary` 仅在权限内返回domain小块 `{state:ok/unavailable,as_of,stale,counts,filter}`；无资格domain省略，服务端不查询，前端也不补发越权列表。已有消息/PAT自服务独立可用。工单主数=当前处理或具名领域复核的非终态票去重，原因分别展示；甲提交给乙复核但assigned_to仍甲时，乙计复核，甲不计当前处理，不改assigned_to凑数。类型运营视图须独立职责＋组织范围，卡片点击携同口径filter；`can_transition`或admin可操作范围不能代替“由我负责”。

所有资源页请求键包含 `user/authorization_epoch/resource/filters/definition` 的适用部分，写回和撤权使相关缓存失效；选择、自动保存、订阅和迟到结果核当前上下文。前一张票的迟到响应不回贴到当前查看的新对象上，换账号不显示前一人的历史/选择。统计刷新失败标上次时间，不能把unknown/null当0；计数/列表暂不一致须显示as_of并可刷新，而非承诺跨HTTP读取事务完全同刻。

可访问性采用[WCAG2.2](https://www.w3.org/TR/WCAG22/)的适用基础项：正常文字对比度4.5:1、大字3:1；键盘可达、焦点可见不被sticky遮挡、错误关联字段、异步状态可播报，颜色之外有文字/图形。首期按代表页及逐页交付验证，不以一次自动扫描宣称全平台合规认证。

## 7. TDD 用例目录与验收出口

2026-11-15[权限专项](../review/reports/authorization-design-implementation-review-2026-11-15.md)将已有D03“全读出口一致”落到存量`ticket_relations`：A仍可见而B撤CC后不可见时，`ListRelations(A)`须按对端当前read资格过滤后再生成结果及任何后继分页/计数，不能回B的ID、关系创建人/时间或可识别占位；双向关系同样处理。当前main只验A，已隔离复现元数据泄露，尚未修。C04/D03正式启用前补`Relations_PeerRevokedNoMetadata`，同时验仍合法对端不误过滤；CreateRelation已有双端update检查继续保留。组织status=0后scope/本组与祖先委托撤权归底座FND-14同一修复，不重复造权限规则；BK-20历史档案例外单列。

下列名字是应新增或改造的行为测试候选，不表示已有这些测试。先按当前获准的实现批次写全正向、负向、边界和权限失败用例，再实现到通过；并发缺陷用闩或通道固定竞争点，至少稳定复现失败 8 次，不靠随机 sleep。只对当前实际使用的范围开工，不一次写全远期失败测试。

| 用例组（候选测试名） | 必须证明的正/负/异常预期 |
| --- | --- |
| `Definition_DeployDraftIsolation` / `Definition_StalePlanNoPublish` | A建单→改草稿不影响A→发B后新单B；引用/父CAS改变409且无半发布；同幂等号异参拒绝 |
| `Definition_BackfillResume` / `Migration_PreservesClockAndActions` | 回填中断可续且零悬挂；最小迁版不重置due/重跑后动作；缺快照写拒、只读明确degraded |
| `Transition_CollectAtomic` / `Transition_QualificationUsesBeforeImage` | 字段＋状态＋历史＋intent全有或全无；非法/隐藏必填/证据缺失拒绝；本次patch不能自授资格；列表actions与实际执行同源 |
| `Ticket_ConcurrentTransitionOneWinner` / `Ticket_RevokeBeforeCommitDenied` | 两实例同revision唯一赢家，撤权先提交旧授权不能提交；网络等待时不持票锁；Unknown后查原operation不另造效果 |
| `Ticket_AllEntrancesStartSLA` / `Ticket_AssigneeChangeDoesNotRestart` | Create/Assign/Transition/Close所有入态写/清SLA；同态换人/补字/接单不重置；提醒旧代次不抑制重入 |
| `Ticket_AcceptIsHumanIdempotent` / `Ticket_MilestoneCycleHistory` | 自动分派不算响应；一键接单开始原子；重开新周期保留原时长；未知旧历史不可计算而非0 |
| `Visibility_AllReadSurfacesAgree` / `Participants_SourceRevocation` | 三策略×主体矩阵在列表/详情/计数/导出/关联/任务结果一致；普通all/owner不越权；CC读评论不办；移除一种来源不误删另一来源 |
| `Ticket_BusinessManagerScopeAndFields` | 配置管理员无实例权；获权业务管理仅所管组织/字段，原因必填；仍受发布快照/validator/历史不可改约束 |
| `Field_EvaluateEqualsSubmission` / `Field_RoundtripElevenTypes` | 复合显隐/公式同源、伪公式输入隔离；十一旧类型与user/dept增量分别验；非选项absent/null/[]语义往返；非法元素绑定拒绝 |
| `Attachment_AppendAndReferenceGuard` / `Attachment_ConcurrentGCBind` | B追加不删A；逐项失败；跨票/未确认key拒；删除被字段或正文引用409；GC与绑定单赢家、失败不遗半绑 |
| `FieldACL_ReadWriteAllSurfaces` / `Search_ForbiddenFieldNoInference` | 无读字段不在历史/导出/结果/错误泄露，不用于filter/sort；脏值不使cast炸全查询；预算拒绝且无全量fallback |
| `Routing_DefaultIsNotErrorFallback` / `Routing_MultiInstanceCAS` | 正常多命中优先级/默认；表达式/依赖错hold而非默认；链深、重复唤醒、崩溃重领均无重复效果 |
| `UI_SafeLandingNoBusinessRequests` / `UI_HomeResponsibilityMatchesList` | 无菜单/空权限可用；无权domain零请求；处理/复核同票去重；失败非0，点击过滤同义 |
| `UI_BulkTagsPartialAndRetry` / `UI_ContextChangeDropsLateReply` | 跨页明确选择、部分失败/取消/同参重试；换查询或用户清选择，迟到请求不盖新对象 |
| `UI_IntelHistoryDoesNotQuery` / `UI_DesignerConflictKeepsDraft` | 历史只回填、重查新时间；设计器409保留输入并对比；本机存储禁用不丢当前表单；主动作焦点/错误播报/console检查 |

验收路径按最小依赖推进：C01/C02/C04 与 D01/D03/D04 → D02/薄UI → 风险 N:1/复核投影 → D12/TR/AL/消息原运行闭环；D05、LS01–LS04、D16 和U04/U06按各自窗单独验。U07从样板起随批执行，不能等全站末尾再查权限/交互。D11最小业务通知由D15/消息核心先取需要的子合同，完整增强后继。

产品出口必须跑对应真实PG/Redis、HTTP服务和浏览器链，主仓完整适用命令/acceptance、迁移up/down/往返与UI六件；mock/contract/real分别列，设计参考和用例目录不是运行证据。当前文档不改变现行全门禁，不声称新增覆盖率。

### 7.1 TICKET准确wire、候选人及实施切片

只读基线core `610c1d76`、UI `875df0bf`。本表路径均省略/api/v1公共前缀，其余为完整业务路径；只补接线/兼容，不取代本页状态/字段/权限/事务与既有TDD。

| 当前wire与源符号 | 增量用例及兼容合同 |
|---|---|
| `POST /ticket-types/update`：code/states/transitions/transition_meta/version→TicketType；`/ticket-types/fields/replace`：code/fields/version→裸字段数组，软提示在envelope.message。`internal/handler/ticket_handler.go`的UpdateTicketType/ReplaceTicketTypeFields→type_admin.go同名Service | D01沿候选deploy/plan、deploy、definitions读，发布输出统一changed/definition_ref/draft_revision；旧live保存切草稿是可见语义变化，管理UI显示published=false/当前发布引用，不暗deploy。UI `src/api/ticket/index.ts`现两个包装Promise<void>须接结果/软提示，不虚标data.warnings |
| `/tickets/transition`：id/to/comment→Ticket；`/tickets/assign`：id/assigned_to?/escalation_supervisor_id?→null；`/tickets/close`：id/comment?→null。`internal/model/ticket_request.go`及transition_service.go的Transition、service.go的assignAuthorized/Close | 新transition增量revision/定义/采集patch，旧available/assign/close包装与响应保留；新增accept记录人工接单、transfer换唯一主责，目标范围重验，同态不重置SLA。validateAssignTargetUser现仅存在/启用，不冒称已有类型/组织候选校验 |
| `/tickets/update`：id/title/description/priority/custom_data/attachments→Ticket；service.go的Update/mergeCustomData | custom_data仍patch/null删键、attachments增量；新custom_data_patch只在新DTO适配，不无版本改名。manage独立实例职责＋reason/字段白名单，不能借类型管理强跳或改历史/受管字段 |
| POST `/tickets/cc`：ticket_id/user_ids→{cc:rows}，`/tickets/cc/remove`→null；GET `/tickets/:id/cc`→{cc:rows}。`cc_service.go`的AddCC/RemoveCC/ListCC | 新participants写role/action/revision，manual origin由服务端确定，不能自报domain来源；移除指向已获权manual grant，域受管项只由可信桥维护。创建者不自动获邀人权。ticket_cc_repo.ListByTicket现不筛role，扩角色前先精确查询cc；creator/assignee仍字段派生 |
| 当前无fields/evaluate或prefill HTTP；service.go的applyFetches取live旧fetch且失败保原输入 | D04用类型新建/已有票互斥上下文和input_generation，提示失败不等全提交失败；仅真正依赖权威值的动作阻断，不把旧fetch等同新确定性预览 |
| UI `src/views/Home/index.vue`固定状态多次查total且失败当0；`internal/service/ticket/badge_service.go`的todoCount只查assigned_to并排两终态 | summary按服务器责任谓词产指标ID/as_of/桶/筛选令牌，角标/卡片/钻取共用，不以assignee=me代风险复核职责；unavailable不显示0，不做BI |

`TicketTypeFieldInput.FieldType`在ticket_request.go的binding声明8种，type_admin.validateFieldInputs/validFieldTypes及UI已11种含radio/rating/file；统一声明/生成物/HTTP回归是当前接线修正，本轮未跑HTTP，不把静态冲突夸成已复现运行拒绝。CreateTicketRequest虽声明assigned_to，Service.Create明确拒收；新创建包装及风险桥不能误当可选原始分派。

**选人采用独立受权Query**，不能借UI UserPicker两次管理GET /users且catch为空替代。候选GET /api/v1/tickets/assignment-candidates输入ticket_id或创建type_code/definition_version/org_id互斥上下文、purpose、q、page/page_size≤50，不接自报scope。已有票允许assign/transfer/participant/cc，须先验调用者对应动作功能及本票职责，光可见不允许搜索；创建上下文首期只为独立获准管理者预配置participant/cc，不支持assign/transfer，普通Create仍拒assigned_to。两上下文混用/非法purpose组合400，无权不返回候选。

随后按同一目标用户谓词过滤当前active/类型/组织候选，返回最小user_id/display_name＋eligibility_revision/as_of，组织筛选只收窄；缺字段不暴露工号/邮箱/角色目录。无候选正常空，无权404/403、依赖失败503，不能吞成空；提交再验，Query非grant。主责单选、CC/participant多选，缓存绑定上下文/权限/世代。

实施仍分为四个可独立验收的部分：T1 先交定义端口，再扩列、幂等回填初版并核对没有悬空引用，之后切换读写，最后加约束及草稿 UI；T2 交转换核心、字段采集、accept、transfer 及最小详情界面；T3 交类型策略、参与来源、独立管理资格和候选 Query；T4 交 evaluate、prefill 与责任 summary。

先验证升级前已有工单能补上可信的规则版本、重复发布请求只生效一次、缺规则时只能查看。再验证字段、状态、事件和 intent 同事务提交，并发提交只有一个成功；随后验证 all-scope 不穿透类型限制、来源撤销，以及列表、计数、导出使用同一可见范围。最后验证迟到预填不覆盖新值、无权时不发请求、未知结果不显示为 0，以及同票复核责任去重。全部沿本页既有具名 TDD 与原包扣除重复工作，不另建父包或第二套规则核心。

**已确认TICKET-D01（F019）**：统一新增规则，保留已有合法关系。旧inherit类型也不保留“仅因可见便自行加CC”的例外；今后新增含自己均验独立参与人管理授权。既有合法manual CC保留，原组织读源撤销后该独立来源仍按当前类型/L1规则求值；本人可退出自己manual CC，不删domain/强制来源，失去L1时仍拒读。不重问新类型邀人管理、CC只读评论或创建者无自动邀请权；确切答复见[后继F019](../planning/planning-followups.json#F019-module-detail-owner-decisions)。

**本人退出的准确复用（D03）**：现有`POST /api/v1/tickets/cc/remove`与`RemoveTicketCCRequest{ticket_id,user_id}`保留；`internal/service/ticket/cc_service.go`的`RemoveCC`已经对本人用read、他人用update，`cc_integration_test.go`已钉本人自摘，不新造self-exit端点。来源化后由Service在共同改权守卫及工单/来源锁序内重验当前read资格；本人分支只移除`user_id=actor`、`role=cc`、`origin=manual`的精确来源，不能调用未限定来源的全量删除来摘domain/强制关系。存量未标注来源先按D03迁移映射，不默认为可自删；无manual则按受权可见范围返回无变化，退出后无剩余读源的下一请求仍404同形。摘他人及新增含本人继续验独立管理资格，不能由这个分支获邀人权。

与管理方新增/移除、风险domain关系交接并发时，同一守卫/对象锁内串行求值；退出只撤本人manual，不能删后来合法domain来源，管理方再次新增是独立获权动作与历史。原wire在目录表保留身份作用域selfService例外依据，例外只允许此受限分支，不增加页面写绑定或给普通人`ticket:participant:manage`。本项归D03来源/参与规则，复用已有路由与自摘测试，只拆新增来源限定、迁移和并发回归的剩余量，不重新计完整self-exit功能。

`Participants_SourceRevocation`补旧合法CC原组织源撤销后仍按当前策略可读、L1撤销拒读、新无管理权自加拒绝、本人退出不误删domain四组预期；另用原remove wire验伪user_id不能冒充本人、manual/domain并存仅退manual、并发管理/领域交接保合法来源、退出后不可见仍不泄成员。这些是待实现验收，不以本次确认关闭旧路由问题。

## 8. 开工冻结、运维和待绑定

代码批只需冻结本包实际选用的类型/组织/字段/状态、DTO及相邻接口，不等所有未来场景；DB/路由/目录/Swagger/UI生成物同步，所有写路由绑定按钮，页面只挂GET，不增 pageWriteAllow 例外。每次交付三节短说明回填项目控制。

最低运维可观测项：定义解析失败/回填悬挂、动作409与拒绝原因、最老待交接intent及hold、SLA逾期/扫描落后、通知待发/未知、字段求值依赖失败、查询预算和UI接口错误率；记录request/operation/correlation/定义/visit引用，不记录附件URL、全文、秘密和无权字段。持久事实/intent与日志留存分开；受权恢复入口只重放未完成后继或补投影，不改历史消警。

仍需绑定但**不阻无关设计**：

1. 首个真实类型/组织/业务管理员/字段白名单、risk owner映射与消息收件对象：在该用例启用前用实际例子核验，不重问B01–B05既定默认。首版仍可用合成mock。
2. E03哪些类型采用轮询或认领、合格池与超时/升级目标：**P6.3出口完成范围裁决**，后继实施仍按E03/P8及实际获准子窗；不能将“后期实现”误当“到P8才问”。未选则继续固定合法主责，不假设队列已实现，也不为赶日期丢功能。
3. 四类时长首组展示、自然历时外的工作日历/暂停：事实写入先按本页保全，D13投影前确认真正指标；未配置工作日历不假算工作时长。
4. B08生产通知级别映射/组织锚点/汇总时区时刻与强制对象、真实HTTP消息联调：首条对应真实渠道前确认。两IAM操作失败政策已有F017确认，后续只验C实际事务接线，不再挂待答。

实际限流、批量上限、自动链深和缓存/保留时长是可观测技术候选，压测后配置并列证据，不把样例数字写成用户已批准SLA。新增净范围由原包净估扣重复用后回总账，本页不改现有日期承诺。

## 变更记录

- 2026-11-14：按现行main定向补齐工单定义/实例/权限/字段/成员来源/里程碑的数据与事务合同、异常恢复、UI逐包行为和TDD出口；纠正旧pending决策、字段命名及batch API推荐。保持有限核心先交与完整P6后续范围，未实施产品。
- 2026-11-15：参与来源仅持久独立授予，creator/assignee直接用现有权威事实；明确E03的P6.3早期裁决，不提前实施后包。
- 2026-11-21：§1.1/§3.3 就地解释钉版、旧票、回填、降级等抽象词，补三类工单记录区分的示意例；不改编号、时点、锚点与合同。
