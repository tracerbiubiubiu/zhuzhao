# 页面族施工卡：权限、数据与失败恢复

日期：2026-11-15。状态：设计候选，UI源码未改、浏览器未验。源码基线为UI main `875df0bf`、核心main `610c1d76`。本页接续UI仓`docs/design/frontend-implementation-contract.md`、`frontend-pages.json`和27路由审计；不另立配色、字号、组件或第二份页面清单。U06的首页样板之外还有identity/tasks/al/messages/audit/iam/config/recovery八组，本页补其准确施工接线；全站推广按U06，首版只取实际核心链用到的薄UI，费用不重复。

## 1. 共同施工边界

已有首页、列表、详情、配置、查询、恢复六类样板继续复用。数据上下文至少包含当前身份世代、资源/过滤条件、页/游标和读取世代；写操作另绑定提交时拍下的目标及值。首次加载失败、合法空集、有旧数据的刷新失败分别显示；后者保留同上下文旧行及观察时间，不把错误转成零。跨身份/资源的旧数据立即清理，旧请求包括finally均不得改新上下文。

按钮消费服务器能力与原L1控制，服务器重验L1/L2/L3；页面隐藏不是授权。保存已提交但刷新失败只重读，不再自动写一次。普通非幂等写仅明确无效果才重试；已有明确幂等合同的能力可按原目标/原输入/原key有界重放，unknown不换key重做，优先查询原operation。权限收回后清当前受限内容、计数及选中，不继续呈现旧标题/正文。加载状态采用`idle/loading/ready/empty/error/stale`候选视图状态，不要求新全局状态框架。

所有页面沿既有token/主操作位置/查询筛选展开规则；用按钮或链接承载动作，具可读名称、可见焦点、Esc及对话框归还焦点。窄屏重点保持对象、状态和主动作可达，不把整页缩小。具体截图、键盘与浏览器验收仍由UI实现批交，不以本页代替。

### 1.1 现行界面基线与本页读法（2026-11-21）

先从各组实际锚点查现有页面和请求接线，再用待实现预期策划浏览器验收。举例来说，现有消息中心提供待办入口。B2 增加待复核提醒后，当前负责人打开同一张整改票应能逐项复测，刷新深链还能找回原工单。设计中的首页计数和详情布局按交付批次核状态。 [B2 场景](../planning/phase6-b2-kickoff-plan.md#入口与现场导览谁在哪里操作看到什么) · [已有界面复验](../review/reports/phase6-web-ux-audit-2026-11-20.md)

## 2. identity与recovery

最小视图模型：`session_generation/target_user/org_id/load_state/saved_state/error/last_success_at`；资料、PAT与我的组织独立加载。PAT失败不挡资料，组织503不显示“未加入组织”。`Profile.handleSubmit`写成功后`fetchProfileApi`失败，显示“已保存，资料刷新失败”并保留提交结果；清空字段仍依既有后端合同，不在UI假造可清空能力。

PAT秘密只在一次性展示内存中，关闭、退出或换身份清除，不进query cache/日志/持久草稿。我的组织按`org_id＋请求世代`收结果；写后刷新不污染后来所选组织；名册403沿现有只读语义，不当作全局会话失效。恢复页保存`reason/safe_redirect/retry_state`，503/429保留会话，确定终态走[C05共同退出](foundation-iam-module-detail.md#43-c05切换恢复与独立验收)。PAT弹窗窄屏内部滚动，取消/提交均可达。

实际锚点：UI `src/views/Profile/index.vue`的`handleSubmit/submitPat`、`src/views/MyOrg/index.vue`的`loadRoster/onSelectOrg`、`src/views/Error/SessionError.vue`的`retry`、`src/router/sessionError.ts`的`classifySessionLoadError/isSafeRedirect`。待实现预期：`Profile_SaveCommittedRefreshFailed`、`PAT_CloseAndSessionChangeClearSecret`、`MyOrg_LateRosterBoundToOrg`、`SessionRecovery_TransientRetainsTokenSafeTarget`。

## 3. tasks

每tab保存`tab/filter/page/task_id/request_generation/load_state/as_of/error`。运行记录/定义有total；死信只有页码和空页终止，不能伪造total。持久status与live_state分列，“已受理”不显示“已完成”。打开A再打开B时，详情按task_id/世代收结果，初读失败有错误/重试，旧行刷新失败保留同上下文内容。

`task:read/submit/operate/manage`分别约束请求和动作，敏感action再核维护资格；unknown只查原receipt。主动作按tab显示“提交任务”或“新建定义”，参数可折入详情，三tab键盘可达。定时回补完整恢复控件仍归I03，不借页面整理前移。

实际锚点：UI `src/views/task/list/index.vue`的`fetchRuns/openDetail/fetchDead/onRetryRun/onSubmitTask`，`src/api/task/index.ts`的`TaskDetail/listDeadLettersApi/RUN_STATUS`。当前openDetail无完整目标/世代守卫且失败置空，不能写成已具备。待实现预期：`TaskTabs_PaginationShapeAndIndependentError`、`TaskDetail_LateResponseBoundToTask`、`TaskOperation_UnknownQueriesOriginalReceipt`。

## 4. al

上下文为`type_name/schema_version/filter/after_cursor/selected_doc_id/doc_version/load_state/error`。切类型清游标、选择和旧编辑目标；同类型刷新失败保留旧行并标陈旧，首次失败不显示“无数据”。类型列表失败、没有类型、所选类型无数据分别展示。类型历史/演进编辑按type_name和generation绑定；写成功刷新失败只重读。

监控类型未支持的写入口由服务器能力明确拒绝；不能仅前端隐藏。窄屏类型侧栏可收纳为选择器，类型/字段/错误与按钮有键盘名称。复用已有导入文件替换保护，不重复派发。实际锚点：UI `src/views/al/data/index.vue`的`fetchPage/watch(selectedType)/submitForm/submitImport`已有fetchSeq，但catch仍清list；`src/views/al/types/index.vue`的`openHistory/submitEditor`。待实现预期：`ALData_FailedRefreshRetainsSameContextRows`、`ALHistory_LateResponseBoundToType`、`ALWrite_SaveSuccessRefreshFailureDoesNotRepeat`。

## 5. messages与audit

消息保存`notification_id/read_at/current_content_state/safe_link/list_state/mark_state`。标读失败保留未读并给重试；导航失败与标读失败分别反馈。目标撤权后不继续显示标题快照；待办tab消费首页同一责任谓词，不能继续以UI拼assigned/in_progress代替所有非终态。实际`src/views/message/center/index.vue`的`inboxQuery/todosQuery/openItem/readAll`中，openItem现并行置读与跳转；候选必须分别记录两结果，不要求两远端动作原子提交。

审计各tab保存`query_hash/page/last_success/error`；对账未执行、失败、完成且零缺口分别展示。列表默认元数据，不把完整body先送浏览器再藏按钮；报文/stack展示和复制均消费服务器详情资格/允许投影。实际`src/views/audit/log/index.vue`的`openPayload/prettyBody/copyRequestBody/fetchPanics/runReconcile`现直接使用行request_body，服务端投影归OBS，U06只计消费与交互。两组行操作可聚焦，长报文窄屏可读，关闭后回原行。

待实现预期：`Inbox_MarkReadFailureDoesNotPretendRead`、`Inbox_RevokeRemovesSnapshotAndCount`、`AuditTabs_UnrunFailedZeroAreDistinct`、`AuditPayload_MetadataReaderCannotExpandOrCopy`。历史收件事实与当前可见内容的边界见[统一审计](../design/unified-observability-audit.md#61-两改权事实与已有读取的准确投影)。

## 6. iam与config

IAM用户角色/组织弹窗沿[第一包](iam-first-batch-design.md#34-ui只需要成功快照与局部状态)。角色菜单树同样保存`target_id/source_revision/loaded_exact_ids/generation/dirty/state`；树与绑定集合均成功才能整体替换。失败/迟到不能拿空集或别的角色集合保存，目录搜索隐藏节点不等于取消授权；页面行仍只读。组织移动确认源/目标路径与后端影响摘要，失败保留选中。

字典固定`type_code＋generation`，超过100项真分页或明确穷尽加载；通知模板/规则/log独立状态，更换模板不默改未知字段。settings逐键保存`key/base_version/draft_value/outcome/error`，提交时冻结dirty集合与值；成功项更新基线，失败/冲突项保留草稿，不能统一refresh/seedDrafts清掉失败输入。

当前主仓`internal/model/settings.go`的`UpdateSettingRequest.ExpectedVersion`与`internal/service/settings_service.go`的`SettingsService.Update`已有可选版本检查，UI先接expected_version；当前过期错误是ErrParamInvalid/400，不虚标409。`SettingsRepo.Upsert`在覆盖行缺失时直接插入，不检查expected，不能泛称完整CAS；`SettingsService.Reset`目前无版本参数且直接删除。

配置页后继候选把Update/Reset均接可选期望基线`{source:config|kv,version}`：存在性与版本在同一事务比较后条件写/删，过期零效果；缺省保留旧调用兼容。每key的单调revision须在reset后保留（复用逻辑head/tombstone，不强制新表），新覆盖不能从1重来，防删除再建的ABA；两类写方均推进该head，不能只给Reset加WHERE。列表回该基线，UI整体取成功快照。此小增量随U06配置组定准确DTO/迁移，不前移为首版本地核心前置；原expected_version兼容入口及新基线同时提供时必须一致。C04授权屏障不代替配置数据版本，不重建通用批量事务API。

实际锚点：UI `src/views/system/role/index.vue`的`openAssignMenus/submitAssignMenus`，`src/views/system/dict/index.vue`的`fetchTypes/fetchItems/selectType`，`src/views/system/setting/index.vue`的`saveDirty/refresh/seedDrafts/resetRow`。树键盘级联、窄屏字典选择器/保存条沿现有样板。待实现预期：`RoleMenus_LoadFailureNoReplacement`、`DictItems_LateResponseAnd101stItem`、`Settings_PartialSaveRetainsFailedDraft`、`Settings_UpdateSendsExpectedVersion`、`Settings_ResetStaleVersionHasNoEffect`。

## 7. 实施与证据

上述名称是待写验收场景，不是本轮已执行测试；按实际UI测试框架命名落地即可。每组先有效红，分别验纯函数/组件、真实HTTP和浏览器，跨身份/错序/部分成功不能仅靠快照测试。先把核心风险/工单/任务/AL/消息和IAM薄页用到的卡纳入原包；其它页面随U06，不为了“统一”一次改完27路由。公共错误、页面token和权限谓词只有一份实现，卡不另造八套通用组件。
