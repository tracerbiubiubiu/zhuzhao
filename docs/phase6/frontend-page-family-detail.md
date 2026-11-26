# 页面族施工卡：权限、数据与失败恢复

本页维护前端页面的施工场景和浏览器验收，沿用 UI 仓 `docs/design/frontend-implementation-contract.md`、`frontend-pages.json` 的视觉体系、组件和 27 个基线路由。下文源码锚点源自 2026-11-15 设计记录，使用前核当前候选。B2 办理页先随人工整改验收，完整工作区和本期确认的全站推广随后在外部验收。 [剩余开发顺序与验收](../planning/intranet-uaa-release-schedule.md#剩余开发顺序与验收)

## 1. 共同施工边界

已有首页、列表、详情、配置、查询、恢复六类样板继续复用。每次读取至少记录当前身份版本（包括登录及授权变化）、资源和过滤条件、页码或游标，以及本次读取的版本号；写操作另保存提交瞬间的目标和值。首次加载失败、查询成功但没有记录、已有数据时刷新失败，要分别显示。刷新失败时，只有身份、资源、过滤条件和分页仍相同，才保留旧行和观察时间，不能把错误显示成零条。切换身份或资源后立即清理旧数据；旧请求即使最后才执行 finally，也不得改动新页面的状态。

按钮依据服务器返回的可用操作及原 L1 控制显示，服务器仍重验 L1/L2/L3；页面隐藏不是授权。保存已提交但刷新失败时只重新读取，不再自动写一次。普通非幂等写只有明确未产生效果才重试；已有明确幂等合同的能力，可用原目标、原输入和原 key 在约定范围内重放。结果为 unknown 时先查询原 operation，不能换 key 重做。权限收回后清除受限内容、计数和选中项，不继续呈现旧标题或正文。加载状态采用`idle/loading/ready/empty/error/stale`候选视图状态，不要求新全局状态框架。

所有页面沿既有token/主操作位置/查询筛选展开规则；用按钮或链接承载动作，具可读名称、可见焦点、Esc及对话框归还焦点。窄屏重点保持对象、状态和主动作可达，不把整页缩小。具体截图、键盘与浏览器验收仍由UI实现批交，不以本页代替。

### 1.1 现行界面基线与本页读法（2026-11-21）

先从各组实际锚点查现有页面和请求接线，再用待实现预期策划浏览器验收。举例来说，现有消息中心提供待办入口。B2 增加待复核提醒后，当前负责人打开同一张整改票应能逐项复测，刷新深链还能找回原工单。设计中的首页计数和详情布局按交付批次核状态。 [B2 场景](../planning/phase6-b2-kickoff-plan.md#入口与现场导览谁在哪里操作看到什么) · [已有界面复验](../review/reports/phase6-web-ux-audit-2026-11-20.md)

### 1.2 原定前端要求怎么验

先做安全默认页（U01）及公共页面样板（U02），从样板起随批执行浏览器验收（U07）。人工漏洞整改（B2）先验本批实际使用的风险和工单办理页。随后按接口依赖交完整工单旅程（U03）、表单与流程配置（U04）、情报工作区（U05）和全站推广（U06）。第 4–5 项已有 A/B 规则、证据提交和失败回滚的隔离证据。第 6–9 项覆盖风险登记、人工派单、逐台复测和完整旅程。 [第 4–5 项取证](../review/reports/phase6-b2-stage5-acceptance-2026-11-22.md) · [B2 开工计划](../planning/phase6-b2-kickoff-plan.md#第-69-项-风险登记个人派单与完整旅程)

原需求审计 Q20–Q25 在浏览器中逐条核对。 [原始要求](../review/reports/pre-phase6-requirements-audit-2026-11-11.md#2-原始要求及后续约束逐项核对)

1. **普通用户首页与 Logo（Q20，U01）。** 用普通、自定义和无业务菜单账号登录，从 Logo、根路径、刷新和关闭全部页签回到受权首页。直接打开深链，保留能访问的目标和查询条件。无权链接给出安全去向，无业务权限时首页不发送相应请求。
2. **新建按钮和卡头（Q21，U02/U06）。** 走查列表、详情和系统管理页，能在同一位置找到当前页的主要动作。卡头放短标题、必要数量和操作，规则解释放帮助或详情。控件在长中文与窄屏下仍可识别和操作。
3. **列表多选加标签（Q22，U03）。** 勾选若干工单，完整验收包括翻页仍保留显式所选 ID、切换查询或账号清空选择、逐票当前鉴权及部分失败。结果按票显示，取消只停未发送项。对可重试项保留原目标和标签。跨页范围以显式勾选为准，批量移除另评。 [列表动作合同](ticket-module-detail.md#42-文件草稿标签与通知)
4. **搜索、筛选与展开（Q23，U02/U03）。** 编辑筛选草稿后明确应用，折叠时仍看得到已生效条件。首页待办链接从站内、新标签及刷新进入，核 URL、控件、请求与结果相符。详情返回原条件、页码、滚动位置和行焦点。直接打开详情时也有应用内安全返回。
5. **情报查询与历史（Q24，U05）。** 左栏显示当前账号本机保存的查询条件，点历史回填，明确点击重查才发请求。右栏显示当前结果、来源和查询时间。初次失败可重试，刷新失败时同查询旧结果标陈旧。跨端历史和结果快照随各自接入评审。
6. **全站配色、字体、字号和比例（Q25，U02/U06/U07）。** 用实际页面检查亮暗、长文本、窄屏、原生缩放、可读对比度和布局。列表总数、分页及保存/关闭按钮可达。键盘能进入工单、消息和个人菜单，遮罩下控件不抢焦点，一次 Esc 只退出一层并归还焦点。 [具体已观察反例](../review/reports/phase6-web-ux-audit-2026-11-20.md)

首页工作台沿用已确认的单侧栏和身份顶栏，显示当前工号、组织身份、获准工单、本人任务和未读消息。验收用甲处理、乙复测的一张票核对：首页计数、待办列表、角标和消息使用同一责任口径。乙当前负有明确复测职责时只计一次。单纯发起、抄送或具有管理员操作权均不会增加个人待办。无任务读权时不发任务请求，类型运营与全局态势另走对应授权，读取失败显示不可用或陈旧。 [首页角色与数据](08-home-workbench.md#角色渐进同一张首页按权限码渲染卡片不做可配置仪表盘)

### 1.3 页面清单与首次入网验收

按 UI 仓 `frontend-pages.json` 逐条验收 27 个基线路由。范围包括首页，本地登录与改密，个人中心与我的组织，消息，工单列表、发起、详情、类型模板与分派规则，任务中心，活动列表数据与类型，情报，审计，用户、角色、菜单、组织、字典、通知与设置，以及 403、404、会话错误和页签跳转恢复。本期新增的风险页面和所选联动、查询、设备或确认操作页面随自己的业务包交付并纳入验收。对应页面至少能进入、按权操作、看到真实进度和结果，并能在失败或结果未知后查询原结果。 [工单原 26 件](06-ticket-completeness.md#26件当前对账2026-11-14) · [本期范围](../planning/intranet-uaa-release-schedule.md#3-全部-phase-6-目标与先后)

首次进入内网前，用同一隔离组合运行真实 UI、核心、任务服务、活动列表及数据服务。公司内网依赖用 mock 验成功、拒绝、超时、重复与恢复。逐页收录页面和制品版本、角色、操作、返回结果与落库证据。首读失败、成功无数据、刷新失败及权限撤回分别核对，旧响应不得污染新身份或新选择。页级用例同时检查亮暗、390/768/1440 CSS px、原生缩放、键盘焦点及 Vue 或控制台错误。B4 稳定后优先交公司登录、回跳和本地会话及菜单回归。受管来源展示、治理只读提示及完整治理 mock 归条件 B5。 [仅 SSO 浏览器任务](../planning/uaa-sso-local-authorization-plan.md#3-ui-登录导航和浏览器回归)获准进入内网后再走目标内网预演、真实依赖验证和获批试用。 [首版验收出口](first-usable-release-plan.md#4-开放试用和正式运行前要检查什么)

## 2. identity与recovery

最小视图模型：`session_generation/target_user/org_id/load_state/saved_state/error/last_success_at`；资料、PAT与我的组织独立加载。PAT失败不挡资料，组织503不显示“未加入组织”。`Profile.handleSubmit`写成功后`fetchProfileApi`失败，显示“已保存，资料刷新失败”并保留提交结果；清空字段仍依既有后端合同，不在UI假造可清空能力。

PAT秘密只在一次性展示内存中，关闭、退出或换身份清除，不进query cache/日志/持久草稿。我的组织按`org_id＋请求世代`收结果；写后刷新不污染后来所选组织；名册403沿现有只读语义，不当作全局会话失效。恢复页保存`reason/safe_redirect/retry_state`，503/429保留会话，确定终态走[C05共同退出](foundation-iam-module-detail.md#43-c05切换恢复与独立验收)。PAT弹窗窄屏内部滚动，取消/提交均可达。

实际锚点：UI `src/views/Profile/index.vue`的`handleSubmit/submitPat`、`src/views/MyOrg/index.vue`的`loadRoster/onSelectOrg`、`src/views/Error/SessionError.vue`的`retry`、`src/router/sessionError.ts`的`classifySessionLoadError/isSafeRedirect`。待实现预期：`Profile_SaveCommittedRefreshFailed`、`PAT_CloseAndSessionChangeClearSecret`、`MyOrg_LateRosterBoundToOrg`、`SessionRecovery_TransientRetainsTokenSafeTarget`。

## 3. tasks

每个 tab 保存`tab/filter/page/task_id/request_generation/load_state/as_of/error`。运行记录和定义提供 total；死信只有页码，翻到空页才结束，不能伪造 total。已保存的 status 与实时观察的 live_state 分列，“已受理”不显示“已完成”。先打开 A 再打开 B 时，返回结果必须同时匹配 task_id 和本次请求的世代，A 的迟到响应不能覆盖 B。首次读取失败显示错误和重试入口；刷新失败则保留同一任务或查询的旧内容。

`task:read/submit/operate/manage`分别约束请求和动作，敏感action再核维护资格；unknown只查原receipt。主动作按tab显示“提交任务”或“新建定义”，参数可折入详情，三tab键盘可达。定时回补完整恢复控件仍归I03，不借页面整理前移。

实际锚点：UI `src/views/task/list/index.vue`的`fetchRuns/openDetail/fetchDead/onRetryRun/onSubmitTask`，`src/api/task/index.ts`的`TaskDetail/listDeadLettersApi/RUN_STATUS`。当前openDetail无完整目标/世代守卫且失败置空，不能写成已具备。待实现预期：`TaskTabs_PaginationShapeAndIndependentError`、`TaskDetail_LateResponseBoundToTask`、`TaskOperation_UnknownQueriesOriginalReceipt`。

## 4. al

上下文为`type_name/schema_version/filter/after_cursor/selected_doc_id/doc_version/load_state/error`。切类型清游标、选择和旧编辑目标；同类型刷新失败保留旧行并标陈旧，首次失败不显示“无数据”。类型列表失败、没有类型、所选类型无数据分别展示。类型历史/演进编辑按type_name和generation绑定；写成功刷新失败只重读。

监控类型未支持的写入口由服务器能力明确拒绝；不能仅前端隐藏。窄屏类型侧栏可收纳为选择器，类型/字段/错误与按钮有键盘名称。复用已有导入文件替换保护，不重复派发。实际锚点：UI `src/views/al/data/index.vue`的`fetchPage/watch(selectedType)/submitForm/submitImport`已有fetchSeq，但catch仍清list；`src/views/al/types/index.vue`的`openHistory/submitEditor`。待实现预期：`ALData_FailedRefreshRetainsSameContextRows`、`ALHistory_LateResponseBoundToType`、`ALWrite_SaveSuccessRefreshFailureDoesNotRepeat`。

## 5. messages与audit

消息保存`notification_id/read_at/current_content_state/safe_link/list_state/mark_state`。标为已读失败时仍显示未读并给重试；跳转失败与标读失败分别反馈。用户失去目标资源的访问权后，不继续显示旧标题快照；待办 tab 与首页使用同一套责任判定，不能只由 UI 拼出 assigned/in_progress 就当作全部非终态。实际`src/views/message/center/index.vue`的`inboxQuery/todosQuery/openItem/readAll`中，openItem 现并行标读与跳转；候选设计要求分别记录两步结果，不要求两远端动作原子提交。

审计各tab保存`query_hash/page/last_success/error`；对账未执行、失败、完成且零缺口分别展示。列表默认元数据，不把完整body先送浏览器再藏按钮；报文/stack展示和复制均消费服务器详情资格/允许投影。实际`src/views/audit/log/index.vue`的`openPayload/prettyBody/copyRequestBody/fetchPanics/runReconcile`现直接使用行request_body，服务端投影归OBS，U06只计消费与交互。两组行操作可聚焦，长报文窄屏可读，关闭后回原行。

待实现预期：`Inbox_MarkReadFailureDoesNotPretendRead`、`Inbox_RevokeRemovesSnapshotAndCount`、`AuditTabs_UnrunFailedZeroAreDistinct`、`AuditPayload_MetadataReaderCannotExpandOrCopy`。历史收件事实与当前可见内容的边界见[统一审计](../design/unified-observability-audit.md#61-两改权事实与已有读取的准确投影)。

## 6. iam与config

IAM用户角色/组织弹窗沿[第一包](iam-first-batch-design.md#34-ui只需要成功快照与局部状态)。角色菜单树同样保存`target_id/source_revision/loaded_exact_ids/generation/dirty/state`；树与绑定集合均成功才能整体替换。失败/迟到不能拿空集或别的角色集合保存，目录搜索隐藏节点不等于取消授权；页面行仍只读。组织移动确认源/目标路径与后端影响摘要，失败保留选中。

字典请求绑定`type_code＋generation`，超过 100 项时实际分页，或明确加载完全部条目；通知模板、规则和 log 各自保存加载状态，更换模板不默改未知字段。settings 逐键保存`key/base_version/draft_value/outcome/error`，提交时固定这次修改的键集合和值。成功项更新基线，失败或冲突项保留草稿，不能统一调用 refresh/seedDrafts，把尚未保存的输入清掉。

当前主仓`internal/model/settings.go`的`UpdateSettingRequest.ExpectedVersion`与`internal/service/settings_service.go`的`SettingsService.Update`已有可选版本检查，UI先接expected_version；当前过期错误是ErrParamInvalid/400，不虚标409。`SettingsRepo.Upsert`在覆盖行缺失时直接插入，不检查expected，不能泛称完整CAS；`SettingsService.Reset`目前无版本参数且直接删除。

配置页后继候选把Update/Reset均接可选期望基线`{source:config|kv,version}`：存在性与版本在同一事务比较后条件写/删，过期零效果；缺省保留旧调用兼容。每key的单调revision须在reset后保留（复用逻辑head/tombstone，不强制新表），新覆盖不能从1重来，防删除再建的ABA；两类写方均推进该head，不能只给Reset加WHERE。列表回该基线，UI整体取成功快照。此小增量随U06配置组定准确DTO/迁移，不前移为首版本地核心前置；原expected_version兼容入口及新基线同时提供时必须一致。C04授权屏障不代替配置数据版本，不重建通用批量事务API。

实际锚点：UI `src/views/system/role/index.vue`的`openAssignMenus/submitAssignMenus`，`src/views/system/dict/index.vue`的`fetchTypes/fetchItems/selectType`，`src/views/system/setting/index.vue`的`saveDirty/refresh/seedDrafts/resetRow`。树键盘级联、窄屏字典选择器/保存条沿现有样板。待实现预期：`RoleMenus_LoadFailureNoReplacement`、`DictItems_LateResponseAnd101stItem`、`Settings_PartialSaveRetainsFailedDraft`、`Settings_UpdateSendsExpectedVersion`、`Settings_ResetStaleVersionHasNoEffect`。

## 7. 实施与证据

上述名称是待写验收场景，不是本轮已执行测试；按实际 UI 测试框架命名即可。每组先写能暴露目标错误的失败测试，再分别验证纯函数或组件、真实 HTTP 和浏览器操作；切换身份、响应乱序和部分保存成功不能仅靠快照测试。核心风险、工单、任务、AL、消息及 IAM 办理页面所需的卡先纳入原包，其它页面随 U06，不为了“统一”一次改完 27 路由。公共错误、页面 token 和权限判定只有一份实现，本页不要求另造八套通用组件。
