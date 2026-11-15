# Phase6 工单与前端模块详细设计

日期：2026-11-14。状态：**实现前设计候选，未开发、未完成产品验收**。本页补 D01–D05/D11/D16、U01–U07 及 LS01–LS04 的实施相关合同；范围、工日和状态仍归[总账](../review/planning-backlog.json)、[净估](../review/local-core-release-estimate.json)和[模块交接](module-design-handoff.md)，不另加一期或自动开始施工。

共同身份、operation/receipt、版本和错误沿[生命周期合同](../design/business-lifecycle-contracts.md)与[共同调用合同](../design/capability-invocation-contract.md)。本页把其工单部分展开成数据、事务和用例；与 00/06/08/09 的旧候选冲突时采用本页及现行共同合同，旧原文保留追溯。实际 HTTP/数值错误码、逻辑表物理命名在对应代码批核目录后登记；以下未新增可调用路由或迁移，也不预占编号。

## 1. 当前代码与阶梯范围

本次定向核对的现状：

- `internal/service/ticket/transition_service.go` 的 `Transition` 仍按 live 类型取图，已有 `LockTransitionSnapshotTx` 防预检后行/配置变化，尚非不可变发布快照；通用转换会写 SLA，但 Create/Assign/Close 入态尚未完整收敛 K2。
- `internal/model/ticket_request.go` 的 `TransitionTicketRequest` 仍为 `id/to/comment`，没有转换字段采集；类型 `version` 已有 CAS，不能冒充实例 revision 或 definition version。
- `internal/service/ticket/resource.go` 的 `Authorize/GetFilter/canOperate` 已有三层基础及 CC 读特例，尚无类型策略；`cc_service.go` 的 `AddCC/RemoveCC` 仍按 read/update 区分本人/他人，D03 要改为已批准的独立管理资格。`tag_service.go` 的 `AddTags` 已按单受权、规范化及幂等，不重建标签服务。
- `internal/service/ticket/type_admin.go` 的 `validateCustomData/applyFormulas` 已保留 schema 校验/重算；C01 同名引用和输入隔离、F11 非选项字段空 options 往返仍待修。Phase5 已修的 MIME、双模式保真、fresh 字段版本和未设置列宽不再重复报新功能。
- UI 的 `src/views/ticket/create/index.vue` 已将 file key 追加；单 file 控件尚非一次多选。`src/views/ticket/list/index.vue` 尚无批量标签；`detail/index.vue` 的能力投影与活动流仍待补。`src/router/routePermission.ts` 将 `/home` 列为 exempt，不代表默认页动态注册/Logo 落点问题已经解决。

**本地核心先交**：固定受审流程/基本字段、不可变定义、完整当前三层权限、受限类型、转换采集/证据、唯一主责与 CC、K2、必要业务历史、风险复核责任投影、批量标签及薄 UI。它只消费实际启用字段/动作所需合同，不要求所有设计器、LS01/02 或完整 D11/D16 先完成。

**完整 P6 逐步交**：D05 配置晋级、LS01 常驻字段 ACL、LS02 typed 检索、LS03 受权快速入口、LS04 完整字段diff、D16 自动路由，U03–U06 全站/配置器推广及 D11 通知增强。D01 的同名状态最小迁版随定义交付；删状态完整映射归 D18/P7。共享轮询/认领归 E03 的已排处置窗口，实际近期启用先取所需子范围并净估，不把整套队列藏进 D02。服务端草稿、会签/加签、移动端和通用工作流不在此批。

## 2. 准确输入输出与权限

### 2.1 类型、实例与版本

所有 ID 在 JSON 中为十进制字符串，时间为带时区 RFC3339；新 DTO 的 `revision/draft_revision/definition_version/authorization_epoch` 均为规范十进制字符串，服务端按有界非负整数解析，不用JS number运算。旧现行 `version` number 只在明确旧接口兼容层保留，不能让新响应同字段时而number时而string。`draft_revision` 是管理草稿 CAS，`definition_version/hash` 是发布内容，`revision` 是票的写序号，`authorization_epoch` 只供失效/诊断，不能混用。请求不能自填可信操作者或 `created_by`。

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

这些是公共语义类型，不要求把 `AvailableAction[]` 强塞入已有返回字符串数组的端点造成 wire 破坏。保留旧 available 响应，新增明确对象形态的候选 `GET /api/v1/tickets/:id/actions`，详情可增量嵌入对象。既有 `id/to/comment` 客户端短期仍用服务端行锁和谓词 CAS；只有已传 `expected_revision` 的调用才具备陈旧编辑防护。新的 UI/SDK/私有用例必须传 revision；禁止称老客户端已因此自动防覆盖。

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
| `tickets` 增量 | `definition_version,revision BIGINT,lifecycle_cycle INT`；引用同类型定义；revision>0；`(type_code,definition_version,status)` 索引支持旧单/迁版 | 每类型冻结初版、存量填版本，不猜过去定义；revision 初始化后每次相关业务写递增；历史真相未知标迁移基线 |
| `ticket_events` 增量 | `event_id,cycle_no,visit_no,payload,operation_id,definition_version,aggregate_revision`；`UNIQUE(ticket_id,aggregate_revision,event_kind,event_ordinal)`；`(ticket_id,created_at,event_id)` 分页 | 与状态/字段/参与写同事务；新事件可重放，旧无payload事件不补造；file/richtext 记键/受权引用，正文不进事件镜像 |
| `ticket_state_visits` | `ticket_id,cycle_no,visit_no,state,definition_version,entered_at,exited_at`；PK `(ticket_id,visit_no)`；每票未离态最多一行；`exited_at>=entered_at` | 统一入态核创建/结束区间；同状态转派不新增 visit；终态驻留不计办理时长；重开开始新 cycle 但 visit_no 继续递增 |
| `ticket_milestones` | `ticket_id,cycle_no,kind,occurrence_key,occurred_at,actor_id,event_id`；唯一 `(ticket_id,cycle_no,kind,occurrence_key)`；kind 固定 `accepted/processing_started/result_submitted/business_closed` | 首次接单 occurrence_key 固定 `first`；节点办理点绑定 visit；提交/重开保留原事件，指标能区分首次与后续周期；不保存可被业务管理员任改的人工时间 |
| `ticket_member_sources` | `(ticket_id,user_id,role,origin,source_key)` 唯一，role 仅 participant/cc；持久origin为manual及获准domain（如risk_review）；`(ticket_id,user_id,role)` 索引 | 只保存不能从权威字段推导的独立授予；现有cc回填manual。creator/assignee从当前CreatedBy/AssignedTo计算，不复制成需同步的来源行 |
| 发布/迁版/晋级 plan | plan id、类型/目标/包hash、各基线revision、选中票及revision、警告确认、审批者/到期时间、结果receipt引用 | 复用 D05 的受控计划模型和共同 operation 记录，不再建第二套通用任务/receipt/outbox表；plan过期或基线变化409 |

成员来源表只在D03所需最小来源实现；不提前引入通用关系授权引擎。creator/assignee可以作为API里的只读虚拟origin展示，权威仍是票字段，不再增加Assign/Transfer/回填时的第二份同步。同一人既是手工参与人又是风险复核人，风险交接只移除旧risk_review源，仍可因手工来源可见；保留读来源不保留已经转出的复核权。本人退出自己manual CC可保留既有读可见退出入口，但domain/强制来源不能借此移除，也不把普通退订应用到强制风险通知；新增自己CC仍遵B04，不借selfService绕过。

所有迁移 up/down 成对。扩列、初版定义、批量回填、校验、读写切换、非空/外键分步交；迁移可幂等恢复。down 前停新写、导出新增快照/历史、验证目标旧代码能读，不能删掉已有新定义后让旧单静默读最新版。新增历史事实启用后回退代码只保留其数据，不凭空恢复原来的 live 语义。实际物理拆分/SQL及编号在代码批按迁移地图复核。

### 3.2 一次转换的提交算法

1. 在持锁前解析请求、验证大小/语法；必要外部取数在请求预算内完成，并保留经平台核实的来源引用。客户端传回的建议不成为受管权威值。
2. 事务先取 C04 当前授权守卫（排序 stable key），再取类型 live 策略/发布引用的共享锁；涉及风险桥时遵共同 `group→risk IDs→ticket IDs→link IDs` 顺序，普通工单直接取按 ID 排序的票锁。定义内容不可变，不反向锁草稿。
3. 读锁内当前值，重验身份、L1/L2/L3、revision、定义、合法边及写前执行资格。转换只接受该边 collect_fields 的普通字段 patch，domain/formula 字段拒写；同源显隐、公式/G2、业务 validator/证据均成功才写。
4. 一个事务 CAS 写字段/状态/revision；真正入新态时关旧 visit、开新 visit、同一数据库时刻更新 status_changed_at、SLA 和节点提醒代次；生成事件与可靠 intent。任何一项失败回滚全部，不能只写状态或先发通知。
5. commit 后返回业务提交结果。派单/消息/TR/自动下一跳由 R01 持久 intent 交接；只在其 receipt 证明成功后显示对应成功状态。通知失败或后续转派失败不回滚已提交状态，显示待处理/失败；只重试该后继，不重复整次转换。

Create/Assign/Close/Transfer/领域桥/自动路由调用同一入态与用例核，禁止各写一次 SLA 或绕过状态/字段校验。Create 进入初态、Assign 确实改变状态、Close 进入终态分别按 snapshot 处理 SLA；Assign 仅换人、接单、备注、迁版、改字段都不重置原节点 due。旧 `clearReminderWatermark` 的 commit后删除不能作为新语义唯一依据，改按 visit/代次隔离水位，旧删除仅清理；崩溃后不因旧水位抑制新一轮。

PG 行锁/死锁处理沿[PostgreSQL 18 官方锁说明](https://www.postgresql.org/docs/18/explicit-locking.html)的短事务与统一锁序模式。本项目采用数据库唯一/CAS 作为最终防线，进程 mutex 不作跨实例正确性保证；仅明确可重试的死锁/序列化错误作有限退避，409 不自动覆盖人类编辑。

### 3.3 发布、迁版与配置晋级

deploy 在类型行锁内校验 plan、父草稿revision、current指针与引用语义revision；复制整份图/字段快照，写新定义/指针/成功历史同事务。相同当前内容无变化返回 changed=false；重复操作号返回原receipt。保存草稿不发布，管理 UI 必须区分“已保存草稿”和“已发布”；旧配置写客户端须升级或受控拒写，不偷偷自动 deploy。

旧单继续钉旧版；缺定义写503并停用动作，只读 fallback 明确 degraded。最小迁版仅同名状态且无删状态，逐单 plan/apply/CAS，保留原节点时间与当前 due，不重放 post-actions。部分冲突逐单可恢复。完整删状态映射 D18/P7，不因有相同状态名跳过字段/资格兼容校验。借鉴[Camunda 实例迁移](https://docs.camunda.io/docs/components/concepts/process-instance-migration/)的明确源/目标与映射思想；本项目保留小 FSM，未引入 Camunda 或 BPMN。

D05 继续[配置晋级提案](../proposal/config-promotion.md)：包仅含已发布类型/完整字段和选定模板、稳定外部引用；不带用户ID、真实URL、密码或任意脚本。plan 绑定包hash/目标/审批/目标基线，apply 整包短事务校验引用与审批并发布，等价重放不增版；模板独立配置语义不塞进实例定义。审计写失败的事务政策按该用例既有合同执行；SetRoles/AssignMenus两IAM操作的本地成功审计失败回滚已由F017另行确认、待C接线，不扩大其适用范围。补偿是发布前后受控计划，不等于灾备恢复。

## 4. 工单常见业务与异常出口

### 4.1 主责、接单、管理与时间

唯一 `assigned_to` 继续单值；多人协作/CC/复核分别建职责。未分派必须显示当前管理组织及“待分派”，不随机给无资格的人。共享任务的未来 E03 默认合格池轮询，特定类型认领用单赢家 CAS；资格变化重新筛候选，空池 hold。受限票不因进入候选池向全池开放，须显式获准类型队列摘要，否则仅当前可见者可认领。参考[JSM 队列分派](https://support.atlassian.com/jira-service-management-cloud/docs/assign-requests-from-your-queue/)的分派视图，首版复用现固定规则；不为“随机分人”引入优化调度器。

接单为持久人工事实，自动分派/查看页面/机器人评论不计响应。固定指标：首次接单响应=`首个合法人工accepted−created_at`；节点驻留按 visit；处理历时按 processing_started→对应 result_submitted；整单闭环=created→最终业务确认。归档若只是技术动作不冒充业务完成；重开新周期、保留首次闭环。未接单/未结束样本不当0，未记录的旧历史为 unavailable。默认自然历时；工作日历/暂停有明确配置才另算，不能通过管理员补字/改派/接单重置K2。借鉴[JSM SLA 起停条件](https://support.atlassian.com/jira-service-management-cloud/docs/set-up-sla-conditions/)可显式配置的思路，首版保留四类可解释事实，不复制完整 SLA 产品。

类型业务管理员只能在所管类型/组织内干预获准字段与动作，原因必填，事件记录真实人、前后键级差异及revision。关闭、跳节点、覆盖复核结论没有通用“强制”旁路。补充描述不能改源平台正本或历史时间。系统管理员可见例外不等于免业务 validator。

### 4.2 文件、草稿、标签与通知

file 控件默认多选追加成功项；B上传不替换A，按object key去重，逐项失败保留可重试状态。建单前移除只是不纳入提交；已建单移除字段引用不删除工单附件。显式删除须同事务核当前字段/正文全部引用，仍引用409；不能用文件名猜归属，不允许把别票key绑定过来。C02 负责与GC互斥及Update同规绑定，现稿 `fields`/`fields_on_transition` 改以 `custom_data_patch`/`collect_fields` 为规范；不改原附件下载鉴权。

本机草稿以 `user_id+type_code+definition_version+form_kind` 隔离，内容带schema/hash和更新时间；恢复前先取当前定义/权限，仅恢复允许普通字段并列差异。退出/换账号清内存与对应草稿，凭据、秘密和预签URL不持久化；file key恢复先向服务端核确认/归属/过期，失效显示需重传，不自动提交。存储不可用/超额提示“未保存本机草稿”，保留当前表单；没有服务端 draft 工单状态。

批量标签首版按净估 UI/U8 **复用现 `AddTags` API**：仅显式所选ID，跨页保留稳定集合；改变查询/账号清空选择；展示已选数、处理进度、成功/失败/未发送，失败可按相同标签集合和ID重试。有限并发默认3、最多50票是技术候选，实际预算由压测确认；取消只停止未发送，不回滚已成功标签。逐票当前授权，不把筛选总数当“全选全部”；同tag重复请求幂等。无需新batch后端接口或通用批量编辑器；UI仓旧batch路由推荐在实现前回填为此最终选型，不能两者同时计价。

D11 的业务配置采用有限“类型/流程版本/节点事件×收件职责×渠道”矩阵：待处理主责、待复核具名复核人、关键结果CC；强制风险义务不被普通偏好取消。保存期校验事件/职责/模板及发布引用，业务发生后交可靠意图，发送时重验收件人/字段；站内和HTTP渠道选择同源。预览不发送、测试发送只到授权测试收件人并带标记，不能冒充真实业务通知。催办采用稳定票/操作者60s节流，消息失败不退业务、不广播猜人。predue/digest以受控时钟/时区及持久水位恢复，具体B08真实映射由启用前绑定；消息输送准确合同由消息模块详设承接。借鉴[Jira 分开的关注/评论/附件权限](https://support.atlassian.com/jira-cloud-administration/docs/voters-watchers-comment-and-attachment-permissions/)，本项目CC资格仍以B02/B04为准，不能把关注理解为自动扩权。

## 5. 字段安全、检索、快速入口与自动路由

这些是 P6.3 后续功能，不作有限基本字段首版的整包前置。新增真正保密字段即必须先交所需最小保护，不能用 visible_when/前端隐藏代替。

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

与该次业务写/revision/节点事实/可靠intent同事务持久化；回滚无成功diff。创建初始化、更新、转换采集、管理干预、受权结果映射均复用该函数；结果receipt只说明任务事实，不能替代实际字段更改事件。先有“效果已发生，投影待补”的明确状态，补活动投影不能再执行结果副作用。

读取历史先当前票L2，再按**当前**字段ACL投影；无读权字段整条change省略，不露field_id、名字、旧值、变化数量或“某秘密被修改”提示。删除/重命名字段按事件记录的稳定field_id解析，不把新同名key的权限移给旧值；缺可信定义/权限时保守隐藏值，可只显示获权的普通操作类型。event_id和revision为稳定游标，不以时间戳排序唯一；`after_event_id/page_size`有界分页，重复页不重复活动项。`FieldDiff_NormalizedTruth`、`FieldDiff_AtomicRollbackAndReplay`、`FieldDiff_CurrentACLNoFieldSideChannel`、`FieldDiff_RenameAndMissingDefinition`覆盖真值、失败、重复、撤权与旧定义；本包依D02/LS01，通用活动流展示只在U03计一次。

**D07预填接缝**沿[生命周期 §6](../design/business-lifecycle-contracts.md#6-d04--d07字段求值约束与可替换数据查询)准确`{source_ref,query_ref,params,mapping}`合同；params显式区分`{field:...}`与`{const:...}`，一个查询可映射多个获准普通字段，多参数依赖有环保存期拒绝、最多三趟有界收敛。旧`source:{type,config}/trigger_by`由核心兼容层规范化，不泄露sourceConfig/URL/secret给普通调用者。建议值可人工编辑，但发布配置规定的“总是补空”必须明示；用户改了输入generation后旧结果不覆盖人工值，数字0/false不是空。domain/write_authority字段不能由prefill获得写权；权威字段只消费平台核实的来源引用。正常一查多填、未命中、依赖失联、同参去重、多跳超限、人工更改和撤权分别在D07合同/风险详设核验，工单只消费被允许的建议，不另造取数协议。

**D16** 自动路由只为发布定义中显式automatic出边启用。条件同一求值核，按显式priority取首命中、恰一默认边；人工actions不出现自动边。命中后新的短事务CAS，重新验当前受限服务职责、源状态/revision、字段/G2/validator/证据；只跳人工角色资格。业务提交事务写可靠唤醒intent，commit后处理，扫描器补失联窗口；多实例认领沿R01 lease/fence，不靠本地goroutine保证。链深候选上限16且静态拒明显自动环；触顶hold并展示原因，不静默办结。无匹配/非法表达式/权威依赖错误不能走默认边掩盖求值失败；默认仅处理“合法求值均未命中”。分派condition沿同词表，旧keyword仍兼容，规则优先级明确稳定，不依赖JSON对象顺序。

## 6. U01–U07 页面与状态详细合同

视觉数值和27页清单继续以[UI施工合同](../../../zhuzhao-ui/docs/design/frontend-implementation-contract.md)、[页面清单](../../../zhuzhao-ui/docs/design/frontend-pages.json)为权威；本页只明确跨模块数据和行为。现有UI库、品牌与Vue/Element Plus继续使用，不增一套组件系统。

| 包 | 本批交付与明确结果 | 失败/权限/交互验收 |
| --- | --- | --- |
| U01 | `/home`为公共安全落点；登录、Logo、根路径、404返回用同一safeLanding；常量注册安全首页/自服务 | 空业务菜单仍进首页，零工单/任务请求；旧深链无权落403/安全页，无导航循环；账号切换清动态路由 |
| U02 | 单侧栏＋身份顶栏；PageHeader唯一主操作；列表FilterBar/ProTable、卡头/状态/SLA基础tokens冻结；首页责任数据样板 | 主副文字不塞实现术语；卡头只放短标题/必要数，规则算法移帮助详情；长中文、缩放、900高和窄宽不溢出；加载/错误不闪0 |
| U03 | 四视角、human_key展示与精确查找、普通组合筛选、详情双栏活动流、草稿、多选追加/批量标签 | 四视角均叠当前L2；“已办”取真实历史动作且当前仍可见，不把当前已关票当全量已办；详情主处理/管理动作分区；多动作没有默认破坏性主按钮 |
| U04 | 类型/字段/流程独立页，步骤导航及右属性抽屉；显隐/公式构建器产同一表达式；专家JSON次级；预览共用实际渲染器 | 保存草稿/发布分开；409保留本地草稿与服务器新值供重载/对比，不自动覆写；预览file默认不真正上传；未保存离开确认；旧col_span=1不批量改值 |
| U05 | 左侧本机最近条件、右侧输入/当前结果；历史仅回填，明确“重新查询”才请求；显示来源和本次获取时间 | 请求generation/Abort取消与迟到抑制；失败保留上次结果并标陈旧，首次失败不显示空结果；按用户隔离，不把本机历史当审计/跨端快照 |
| U06 | 按27页清单逐页迁现有首页/任务/AL/消息/系统管理；新增业务薄UI归其业务批 | 所有适用页面涵盖初读/刷新、成功空/筛选空、无权/撤权、失败重试、长文本、键盘、当前版本；改共用组件不等于全部页面已关闭 |
| U07 | 代表真实组件/页面和角色旅程基线；视觉/键盘/焦点/状态/权限请求及console异常验收 | 不只断pageerror；Vue errorHandler/console error也失败；对比度、聚焦、状态播报与真实栈关键链路同时核 |

U06系统管理用户页的当前安全修复须前移随C04交，不等待全站外观：角色/组织弹窗加载失败或pending不许整体保存；甲/乙迟到响应绑定目标和请求世代，组织失败不能沿用甲的选择。当前真实SFC函数已复现该问题，[第二轮复验](../review/reports/permission-boundary-recheck-2026-11-15.md)区分script替身证据与待补真实浏览器回归。加入`UserRoleEditor_LoadFailureNoReplacement/UserOrgEditor_LateResponseBoundToTarget`，不是只给按钮加loading。

个人首页候选 `GET /api/v1/workbench/summary` 仅在权限内返回domain小块 `{state:ok/unavailable,as_of,stale,counts,filter}`；无资格domain省略，服务端不查询，前端也不补发越权列表。已有消息/PAT自服务独立可用。工单主数=当前处理或具名领域复核的非终态票去重，原因分别展示；甲提交给乙复核但assigned_to仍甲时，乙计复核，甲不计当前处理，不改assigned_to凑数。类型运营视图须独立职责＋组织范围，卡片点击携同口径filter；`can_transition`或admin可操作范围不能代替“由我负责”。

所有资源页请求键包含 `user/authorization_epoch/resource/filters/definition` 的适用部分，写回和撤权使相关缓存失效；选择、自动保存、订阅和迟到结果核当前上下文。结果先到旧票不回贴新票，换账号不显示前一人的历史/选择。统计刷新失败标上次时间，不能把unknown/null当0；计数/列表暂不一致须显示as_of并可刷新，而非承诺跨HTTP读取事务完全同刻。

可访问性采用[WCAG2.2](https://www.w3.org/TR/WCAG22/)的适用基础项：正常文字对比度4.5:1、大字3:1；键盘可达、焦点可见不被sticky遮挡、错误关联字段、异步状态可播报，颜色之外有文字/图形。首期按代表页及逐页交付验证，不以一次自动扫描宣称全平台合规认证。

## 7. TDD 用例目录与验收出口

2026-11-15[权限专项](../review/reports/authorization-design-implementation-review-2026-11-15.md)将已有D03“全读出口一致”落到存量`ticket_relations`：A仍可见而B撤CC后不可见时，`ListRelations(A)`须按对端当前read资格过滤后再生成结果及任何后继分页/计数，不能回B的ID、关系创建人/时间或可识别占位；双向关系同样处理。当前main只验A，已隔离复现元数据泄露，尚未修。C04/D03正式启用前补`Relations_PeerRevokedNoMetadata`，同时验仍合法对端不误过滤；CreateRelation已有双端update检查继续保留。组织status=0后scope/本组与祖先委托撤权归底座FND-14同一修复，不重复造权限规则；BK-20历史档案例外单列。

下列名字是应新增/改造的行为测试候选，不是假称现有测试。先按当前获准逻辑批写全正向、负向、边界和权限红例，再实现到绿；并发缺陷用闩/通道固定竞争点及至少8次稳定红验牙，不靠随机sleep。只对当前消费范围开工，不一次写全远期红测试。

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
