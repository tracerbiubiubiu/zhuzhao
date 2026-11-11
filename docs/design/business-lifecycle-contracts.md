# 近期业务生命周期与领域接缝合同

日期：2026-11-11。状态：**实施前详细设计；不是已实现声明**。基线：`00494900c36fcb06e404cde6e04a5ac7da2bbf07`。本文细化 D01–D04、D06 及 D07–D10/D15 的跨域合同；工作量、实施波次和出口唯一维护于[排期总账](../review/planning-backlog.md)，不另报总工期。本文的技术推荐可供实施审查；§10 标为 owner 决策的业务策略未经批准不得据此上线。

本平台聚合已有专业系统结论并管理本地安全生命周期。HR、CMDB、SOC/SIEM、SOAR、威胁情报和企业知识/智能体供给类别已确认，具体接口仍待契约验证。保留长期合成 mock，分别验收 mock、contract、real；真实接口失败不得切成 mock 成功。模型仅通过已有模型 API 使用；本合同不增加模型部署、训练或推理基础设施。

## 1. 证据、裁决与适用范围

### 1.1 已核实现与目标的差别

| 范围 | 当前实现锚点 | 本文新增目标 |
|---|---|---|
| 类型与字段 | `internal/model/ticket.go` 的 `TicketType.Version`；`internal/service/ticket/type_admin.go` 的 `UpdateTicketType`、`ReplaceTicketTypeFields` 已用父配置 CAS | 独立的不可变发布定义、实例钉版、deploy 预检和显式实例迁版；现有 `version` 不冒充发布号 |
| 字段 HTTP | `internal/model/ticket_request.go` 的 `TicketTypeFieldInput` 留有八类 `oneof`，但 `ReplaceTypeFieldsRequest.Fields` 无 `dive`；`validateFieldInputs` 允许十一类 | 一致的元素校验、合法十一类往返、非法元素拒绝；追加 user/dept 时全链同步 |
| 求值 | `validateCustomData` 已统一显隐、公式重算、G2 和体积限制；`exprcalc` 有白名单和请求 context，但不证明 VM 可被抢占取消 | 同一发布定义解析入口、输入隔离、字段约束和服务端预览，不另造前端权威求值器 |
| 取数 | `Service.applyFetches`、`datafetcher.ApplyFetches` 是单 key/`trigger_by` 机制；仅缺键时填充，底层使用 `context.Background()`；生产注册不等于已有真实 HR/CMDB | 多参数、上下文传递、有界查询、编辑时预填及规范化公共 Query；不把旧 Go registry 当私有扩展接口 |
| 状态转换 | `TransitionTicketRequest` 为 `id/to/comment`；`AvailableTransitionsFor` 返回字符串数组 | 同事务字段采集、实例 revision 和结构化 capabilities；旧数组端点保留兼容 |
| 分派 | `Service.assignAuthorized` 已调用 `validateAssignTargetUser` 校验处理人和催办上级的存在及启用状态 | 保留已有检查；转办、user 字段和提交并发窗口补同口径校验，不能重复宣称 Assign 全缺 |
| HR | 来源字段、外部 ID、虚拟组和本地成员机制已有；`UserService.UpdateStatus` 已有禁用和会话撤销链；`OrgRepo.Move` 已级联工单/模板路径 | HR 同步、manager、完整快照/增量、离职交接和撤部处理均为目标，不能声称已对接真实 HR |
| 风险 | [R1](../phase6/01-r1-ingest-ledger.md)、[R2](../phase6/02-r2-assessment-treatment.md)、[R3](../phase6/03-r3-ticket-linkage.md) 是已拍业务输入，尚非风险域代码 | 固定风险状态机、指纹周期、接入/处置分权、N:1 桥和通知合同 |

实际路由核对于 `internal/router/router.go`；现有 `/api/v1` 路由继续服务旧客户端。新增路由、DTO 字段、表/索引/事件名均为本文目标，不是当前可调用入口。本文不预占迁移编号，不把表名当对外协议。

### 1.2 保留的已拍语义

- 工单定义发布后不可变，完整字段元数据随快照；旧单固定旧定义，live 权限不随快照冻结。[流程 v2](../phase6/00-flow-engine-v2.md)的同名状态最小迁版保留。
- 风险接受由当前 owner 申请，风险团队持 `risk:manage` 者确认/驳回，默认禁止申请人自确认；接受不是关闭，全局管理员不绕过此业务制衡。[R1 权限矩阵](../phase6/01-r1-ingest-ledger.md)仍有效。
- 同 owner 的 N 个风险可进入一张整改单，最多 20 个；每风险至多一条 active 工单链接。工单 `created_by` 为初始风险 owner，实际操作人单独审计；无人负责不能派单，审核资格跟当前 `risk_owner`，不跟历史 creator。
- 真回归在修复关闭后建新风险周期并回链；接受有效期内只增加观察计数，到期未续期才重开原风险评估。外部重推不得改本地处置状态。
- 资产以 CMDB 为权威，SOC/TI 提供其专业结论；本地只存引用、必要快照和生命周期事实，不建设第二资产账本、检测关联引擎或知识检索平台。

## 2. 通用身份、版本、事务与公共契约

### 2.1 五种版本互不代替

| 名称 | 目标含义与写主 | 变化后果 |
|---|---|---|
| `protocol_major=1` | 公共 API/事件/Query DTO 语义，由平台维护；沿用现有 `/api/v1` | 可选字段增量不换 major；删除、改类型、改必需语义需新 major 和兼容层 |
| `draft_revision` | 可编辑类型草稿的乐观锁；当前类型 `version` 的兼容别名 | 每次草稿原子写递增；不是运行时定义版本 |
| `definition_version` + `definition_hash` | 某 `type_code` 的不可变已发布业务定义 | 新发布仅影响以后创建的实例；旧实例/adapter 不自动迁版 |
| `revision` | 单张工单、单个风险、接受申请或链接集合的写序号，各领域独立维护 | 每次已提交业务变更递增；expected revision 不符返回冲突，不以 `updated_at` 代替 |
| `authorization_epoch` | IAM/角色/组织/参与人/可见性变化后的缓存失效线索 | 仅作失效/诊断，不是客户端授权凭证；每调用重新检查有效权限 |

字典、目录、源映射各有独立 revision。`source_sequence` 是某来源实体的排序令牌；`occurred_at` 不是全局事件顺序。协议版本、发布号、实例 revision、授权 epoch 和来源序号不得混成单一 `version`。

所有新 DTO 中的资源 ID 为十进制字符串，时间为带时区 RFC3339，空引用为 `null`，业务稳定键为有命名空间的字符串。字段显示名、工号、组织路径、外部人名都不是用户身份。输入未知的控制字段不得静默生效；`custom_data` 保留现有允许未知业务键的兼容口径，但未知键永不成为执行资格、授权锚点或脚本。

### 2.2 授权和调用上下文

调用上下文由网关/认证层构造：`principal_id/principal_kind`、`operator_user_id`（真实人，可空）、`request_id`、`correlation_id`、`delegation_ref`（未来 I02，当前交互可空）。客户端不得自填可信 actor、`created_by`、主 org、系统标记或审批结果。AK/SK 证明调用方身份，不直接授予人或资源权限。

每个业务动作均检查 L1 → 当前资源 L2 → 当前动作/字段 L3。不可见与不存在保持相同 404；已可见但动作禁止为 403。批量动作逐项检查，计数、导出、首页卡片、Query/MCP 复用同一行过滤器，不先查全量再裁剪。capabilities/预览只是当时结果，实际写入仍重验。

普通表单 `user` 字段可由授权编辑者修改，不因是 user 类型就赋权。**被 `field:<key>` 或领域资格引用的字段必须声明 `write_authority`**：`domain` 或受约束的 `delegated`。普通 update、模型、取数预填和结果映射都不能写 `domain` 字段。受约束 delegated 字段须在发布前绑定独立的设置资格、候选范围与审计；不得允许用同一边的输入把自己改成执行人。

### 2.3 事务、并发与重放

- 同主仓数据库的业务变更、领域历史、待交付 intent 使用同一事务；工单与风险跨域用显式用例端口/事务协调，禁止直接互调对方 repository。未来若服务拆分，则由持久命令+收据+对账保持最终一致，不承诺跨库原子提交。
- 本地高风险写的授权通过 `AuthorizeForMutation` 用例在事务中复验 actor 有效状态、live policy/参与关系和授权 epoch；C04 须给 IAM 撤权与业务提交一个共同的 epoch/授权守卫锁顺序，使撤权先提交后旧决定不能继续提交。不能仅在 handler 外读一次缓存、进事务后盲信。未来外部授权不承诺跨系统即时原子撤权，必须按 I02 的有效期/撤销合同与回执处理，不暗示本地锁能锁住提供方。
- 写接口支持 `Idempotency-Key`；作用域为调用主体、`operation_id`、资源和业务意图。相同 key+规范化请求 hash 重放首次收据；相同 key 不同 hash 返回 409。认证/当前可见性失效时不得泄露旧响应，也不得重新执行已提交副作用。幂等记录保留覆盖调用方重试/业务恢复窗口，按 I01/I03 统一政策配置，不在本文随意定 TTL。
- 新高风险写入要求 `expected_revision`。旧现行 REST DTO 暂保留无此字段兼容，由服务端锁定后进行状态及谓词 CAS；这种兼容只能防写交错，不能声称已防旧客户端的陈旧编辑。UI/SDK 分阶段补传，废弃前要有消费者清单和观测。
- 外部查询、模型、网络发送不持有工单/风险行锁。先在请求预算内求建议，再开短事务复验定义、revision、授权、引用和业务谓词；要求权威来源值的字段只能使用平台验证的引用/取数收据，不能拿客户端回传值代替。
- 锁顺序固定：先按稳定 key 排序取得本次需要的授权守卫，再取业务 group 锁，随后风险 ID、工单 ID、链接 ID 升序锁定。同域亦按资源 ID 升序；IAM 撤权在其短事务内只更新授权与 intent，业务交接提交后处理，不反向先拿业务锁再抢授权锁。任何路径都遵守同一顺序；唯一约束是最终防线，不以一次预查询代替。死锁/序列化失败只作有上限内部重试，业务冲突返回调用方。

错误复用 `errcode` 和现有 HTTP envelope，数值码在实施时登记，本文不杜撰现有数值。目标稳定 `reason` 包括 `REVISION_CONFLICT`、`DEFINITION_CHANGED`、`DEFINITION_UNAVAILABLE`、`POLICY_CHANGED`、`INVALID_FIELD`、`TARGET_INACTIVE`、`REFERENCE_UNRESOLVED`、`SOURCE_INCOMPLETE`、`SOURCE_ORDER_CONFLICT`、`IDEMPOTENCY_MISMATCH`、`RECOVERY_PENDING`。400 为输入/业务不满足，409 为竞态/基线变化，503 为权威依赖暂不可用；后台待处理不伪装最终成功。错误不回显 SQL、内部 URL、秘密或对不可见对象的诊断。

## 3. D01：发布定义、实例钉版和迁版

### 3.1 目标实体及不变量

`TicketTypeDraft` 仍由既有类型/字段管理用例维护，含 `type_code`、`draft_revision`、草稿图/元数据/字段、`updated_by`。父草稿 revision 覆盖图与全部字段；保存任一部分须以父 CAS 防止半图+旧字段组合。

`TicketTypeDefinition` 的逻辑键为 `(type_code, definition_version)`，含 `definition_hash`、`schema_format_version`、`states/transitions/transition_meta/fields`、发布人/时间、来源草稿 revision。完整字段含选项引用、公式、取数、显隐、布局、validator 和 qualification。发布版本仅追加，不 update；hash 针对规范化业务内容，排除时间/审计字段。规范化保留数组顺序和选项值类型，不能用无序排序抹掉流程优先级。

类型持有 `current_published_version` 和 `is_active`。工单新增 `definition_version`、`revision`、`source_type`；对外以 `type_code/definition_version/definition_hash` 引用，内部外键/存储形式不暴露。所有运行消费者——创建、编辑、详情 schema、G2、转换、SLA/提醒、终态计数、自动路由——经同一 `ResolveTicketDefinition` 端口取实例所钉定义；禁止各自回读最新草稿。

现有 `checkFieldLock` 在钉版切换前继续保护 live 字段，不能先松守卫再补快照。切换后字段“从新草稿移除”才可经 deploy 影响以后的新单，旧定义/旧单字段不删。相同 field_key 的值类型跨版本保持稳定；换型用新 key 并提供显式映射/迁版预览，不让旧 adapter 同名字段突然收到另一种类型。草稿保存的软提示应改按各 published version 受影响计数，不能声称草稿一次 required 加严已改变全部在途单。

以下仍为 live：类型可见性/创建策略、当前 IAM/人员状态、参与人/CC、标签、消息规则和模板、创建时分派规则、创建前填值的 ticket template、Action 实现/能力可用性。快照冻结字段中的字典**引用**，不冻结整个字典库；现有“启用选项供新选、停用旧值可原样保留”和公式可读停用字典项的口径保留。详情可回显保存时的 label 快照，不能伪装当前授权或选项状态。

### 3.2 目标 API 和原子发布

现有 `POST /api/v1/ticket-types/update` 和 `/ticket-types/fields/replace` 在 D01 切换后用于编辑草稿，现有 `version` 仍指草稿 CAS。**当前它们修改 live 配置，切换为草稿是可见管理行为变化**，必须随管理 UI/工具同批升级并给保存结果明确 `published=false/current_published_version`；登记管理端消费者、通知切换窗口，不对外声称是完全无语义变化的接口升级。不得用“兼容”偷偷自动 deploy，否则绕过已拍发布与预检。一般业务/内网 adapter 的 ticket API 不受这种管理工作流切换影响；若存在不能升级的旧配置客户端，先保留只读/显式拒绝写并给升级提示，不能让它误认已发布。新增以下**候选业务路由**，catalog 的稳定 operation ID 映射真实路由，不要求旧客户端转用通用 invoke：

| operation_id / 路由 | 输入要点 | 输出要点 / 权限 |
|---|---|---|
| `ticket.definition.preflight.v1` · `POST /api/v1/ticket-types/deploy/plan` | `code, expected_draft_revision, expected_published_version` | `plan_id, canonical_hash, diff, reference_revisions, in_flight_by_version, errors, warnings`；`ticket:type:manage` |
| `ticket.definition.deploy.v1` · `POST /api/v1/ticket-types/deploy` | `plan_id, code, expected_draft_revision, expected_published_version, acknowledged_warning_ids` + 幂等 key | `changed, definition_version, definition_hash, draft_revision`；同管理授权，写按钮绑定 |
| `ticket.definition.get.v1` · `GET /api/v1/ticket-types/:code/definitions/:version` | 明确版本；可选 `for_ticket_id` | 经该实例可见性或获准创建类型/管理权限后返回必要 schema；不借枚举版本读取受限业务配置 |
| `ticket.definition.migration.plan.v1` · `POST /api/v1/ticket-types/migrations/plan` | `type_code, from_version, to_version, selection/filter` | 候选清单+每单 revision+差异+阻断；限管理权且每单符合受管范围 |
| `ticket.definition.migration.apply.v1` · `POST /api/v1/ticket-types/migrations/apply` | `plan_id, selected_ids` + 幂等 key | 逐单 `migrated/conflict/skipped` 和恢复 receipt；不宣称整批全成 |

deploy plan 校验：图/字段完整性，状态和边引用，唯一字段 key，公式/fetch 引用，终态/入口，执行资格字段写权，模板/字典/能力引用，新增必填的消费者影响。plan 不是授权；apply 在锁定类型行后检查草稿 revision、当前发布号、plan 内容 hash、引用语义 revision 和警告确认。任一改变返回 409 要求重新预览。deploy 将新定义、current 指针和成功审计同事务提交；相同内容 hash 重放返回 `changed=false` 和现有发布号，不空增版本。

兼容性的“新增字段不强迫旧 adapter 升级”指公共 DTO 增量和已有实例继续可跑。对未来新单增加**无默认的必填字段**仍可能破坏旧生产者，不能用协议 v1 掩盖。preflight 核登记的稳定用例输入：平台可从可信上下文补值则由平台 adapter 适配；否则先交兼容默认/独立 capability revision 或迁移生产者，不静默编造值，不让原 adapter 自己依赖内部字段重构。泛型表单客户端应使用创建预览/定义 token，不能承诺永远无视业务 schema 变化。

### 3.3 创建、旧单和停用

现有 `POST /api/v1/tickets` 保留 `type_code,title,description,priority,org_id,template_code,custom_data,attachments`；`assigned_to` 仍禁止通过通用创建请求直写。新增可选 `definition_version`/`definition_hash` 作为**客户端预览期望值**，不让普通调用者随意创建任意历史版本。省略时服务端取当次 current；提供后 current 不同返回 409，预览刷新后重试。响应返回实际定义引用和实例 revision。没有已发布定义/类型停用不能新建；停用不阻断已有实例流转。

切换上线按“扩列与读兼容→每类型生成初始 v1→存量工单回填→核对零悬挂→读写切换→收紧非空/引用”实施，数据回填可重跑且不重复增版本。启动/健康检查暴露缺定义计数。缺版本/快照时写入 fail-closed；历史只读可按既定降级展示最新可用 schema，但响应必须 `definition_resolution=degraded`、明确不是历史定义，关闭动作/公式/附件写钮，不编造正确钉版证据。不能用此 fallback 接受新写。

任何存量工单（含终态历史）引用的定义禁止删除。没有引用但仍为 current 或晋级包/迁版计划引用者也先撤引用；“停用”不是删除。回滚发布指针只影响将来的新单，已创建新版本工单仍保留原定义；数据补偿、实例迁版和数据库灾备是三件事。

### 3.4 同名状态最小迁版

最小档只在新版本未删任何旧状态时提供，无状态映射、不重放旧 post_actions。plan 限非终态、指定 from 版本，列出每单当前状态/revision。apply 每单重验权限、版本/状态仍匹配、目标状态存在和字段引用可解析；更新定义引用+revision，写 `version_migrated`（旧/新定义、状态、字段差异、plan/actor）。删除字段数据保留但不渲染/不求值；新增 required 不在迁版瞬间逼填，下一次相关写入经过目标定义完整 G2，UI 提示需补哪些字段。不得自动把隐藏历史值删成无法回滚。

迁版保持 `status_changed_at`；不把迁版当进入节点重置时钟。在 plan 显示目标 timeout/reminder/SLA 策略变化；默认保留当前 `sla_due_at` 与已发水位，再按目标定义决定以后的入态策略。若要重算在途期限，必须另起显式调整操作、给旧/新值和事件，禁止隐式延长 SLA。删状态的完整映射工具归后续范围，不阻塞 D01。

## 4. D02：转换采集、资格与 capabilities

### 4.1 转换原子输入和目标字段

新增边元数据 `collect_fields` 为字段 key 列表及本边 required 标记，可附动作 label/说明；这只是复用已有 schema 的采集投影，不另建表单语言。服务端保存时检查字段存在、不可为 tips、不可把 domain/formula 字段变成可编辑输入。required 可比普通编辑严格，但不得弱化 schema/业务不变量。

现有 `POST /api/v1/tickets/transition` 兼容 `id,to,comment`，目标增量为：

```json
{
  "id": "101", "to": "pending_verify", "expected_revision": 8,
  "definition_version": 2,
  "custom_data_patch": {"remediation_note": "已按变更单完成修复"},
  "comment": "提交复核"
}
```

服务端顺序固定：认证/L1 → 锁工单并取 pinned definition → 当前可见性 → 当前身份/人工资格（使用**写前**权威字段）→ 合法边/非终态前提 → 允许字段 patch → 合并和来源字段保护 → 取数建议已取得的合格值/公式/G2 → collect_fields/业务 validator/证据 → CAS 写字段与状态、`status_changed_at`/SLA → `ticket_events` 与可靠后动作 intent 同事务。任一失败全部回滚，不能先存字段后转换失败。事件 payload 保留字段键级 diff，richtext/file 只记 key/hash/受权证据引用，不把全文或文件 URL 复制进通知。

`ValidatorContextV1` 明确内置词表：既有 `priority/title/status/type_code/custom_data` 保留原类型，增量加入 nullable string ID 的 `assigned_to/created_by/org_id` 及 `from_state/to_state/definition_version`。qualification 使用另一个可信 actor 上下文，不从 custom_data 同名键取值；未知系统字段保存期拒绝。撤回的 `assigned_to == null` 必须经该上下文可达，不能只把表达式写进 seed 却运行期拿不到值。词表增量和表达式/函数能力版本随定义 preflight 登记，VM 长度/复杂度限额与取消能力按真实实现说明。

通用 update 仍不自动给处理人全面编辑权；处理人仅能在其有资格的转换中写本边 `collect_fields` 的非受管字段。原 `close`、`assign`、创建进入初态和未来 transfer/revoke 都复用统一入态核，禁止绕开合法状态边、终态元数据、SLA 与审计。保留[已发现 K2](../phase5/12-scenarios-and-tests.md)的语义：进入配置了 SLA 的状态即起算，分派入态也算；进入无 SLA/终态按配置清理，不能只在 transition 一路生效。

### 4.2 `field:<user字段>` 不成为自授予通道

- 发布期：引用必须是同版本 `user` 字段、标量 ID，且 `write_authority` 已声明。未知 key/类型、值数组、formula 结果或无约束可自写的资格字段阻断发布。既有 assignee/creator/org_admin/admin 词表和 OR 语义保留。
- 执行期：从已锁定实例写前值或领域当前引用解析；用户须存在、启用、可在该实例执行该动作。缺值/悬挂/离职/解析失败均不能人工执行，并返回安全的阻断说明。不得先应用本次 patch 再判断资格。
- 它是 transition 的一种**动作级资格分支**，不能先被旧 `canOperate(transition)=creator∨assignee∨delegation` 永久挡住，也不能因此扩大 update/delete。实现将共同 L2 和状态前提与动作资格分开，统一可用动作和执行谓词。
- 字段资格本身不赠予 L1 或可见性。R3 经已授权领域用例维护 `risk_owner` 和受限 participant/CC，才能让当前 owner 进入复核页面；D03 策略未定导致当前 owner 无票可看时，阻断该策略启用并报告配置错误，不能给所有字段用户隐式全域读权。
- 管理员的普通工单管理例外按既有合同保留，但 domain validator、接受制衡、证据、幂等和处置状态不因管理员身份而跳过。自动路由只跳人工资格，仍执行以上业务不变量；system 不能借人类 `field:` token 自认管理员。

### 4.3 能力响应和兼容读取

`GET /api/v1/tickets/:id` 目标附加 `definition_ref`、`revision`、`viewer_capabilities`。已有 `GET /tickets/:id/transitions/available` 的 `[]string` 不破坏改形。新增候选 `GET /api/v1/tickets/:id/capabilities` 提供结构化版本：

```json
{
  "ticket_id": "101", "revision": 8,
  "definition_ref": {"type_code": "risk_remediation", "version": 2, "hash": "sha256:..."},
  "viewer_capabilities": {"update": false, "attach": false, "comment": true, "manage_participants": false},
  "transitions": [{"to": "pending_verify", "label": "提交复核", "collect_fields": ["remediation_note"], "required_fields": ["remediation_note"]}]
}
```

capabilities 只在对象已可见后生成，缺字段一律不显示写入口；读附带的 schema 不回显其他用户不可读的机密字段。`attach` 对齐 presign/confirm/delete 的真实 purpose 与 update/本边证据权限，不能因“可以评论”自动写附件。服务端仍是最终裁决；403/409 后刷新能力，保留本地输入并提示理由。只有已可见对象才能返回具体的缺权/失效说明；“为什么看不见”不提供枚举任意 ID 的探测接口。

转办和撤回输入使用 string ID、expected_revision、幂等 key；处理人转办目标须 active 且满足本单候选范围，候选列表受同一过滤。撤回仅创建人对未受理单，锁内验证未分派及 open 状态；并发分派二者最多一成。无候选/失效不自动选随机人或上级。Assign 已有 active 检查与 supervisor 检查应被复用，不删旧回归。

## 5. D03：类型可见性、参与人与创建范围

### 5.1 不混为一项的三类策略

目标类型 live 策略分别为 `read_policy`、`create_policy`、`participant_policy`，各含稳定 `role_codes`、`policy_revision`、变更人/时间。既有 `visibility_policy` 若实施可作为 read_policy 存储名，不强求重命名。**不预建实例 visibility_override**；真实例外须独立审批设计，不能留一列空白万能旁路。

面向用户统一称“查看范围”，前次问答中的“敏感类型”只是举例用词，不是is_sensitive字段或新的敏感等级体系。当前TicketType/迁移尚无这些类型查看字段；这是D03目标设计。UI选项为“沿用现有范围”“仅相关人员”“相关人员＋指定角色”。后者包含相关人员/合法CC，并非只有指定角色。标签、颜色或风险级别不自动改变权限。

read_policy 候选 enum 为 `inherit`、`participants_only`、`role_restricted`。create_policy 候选为 `inherit`、`role_restricted`、`disabled`，仍与用户可创建组织范围求交。participant_policy 表达“谁可增删普通参与者/CC，哪些具名领域用例可同步其受管参与者”；不是给第三方任意 actor 列表一个管理员执行口。

角色引用技术选择为**稳定 code**，不存环境特有 role ID；有效角色集合统一解析直接角色、组织角色与已定义继承。不存在/停用 role 不匹配，不回退 inherit。类型/角色 code 创建后不可改，显示名可改；role 删除须做策略引用预检。全局管理员既有例外仅在过 L1 后适用，风险接受等业务制衡另验。

### 5.2 可审阅授权矩阵（业务选择见 §10）

| 可见性来源 | `inherit` | `participants_only` 推荐候选 | `role_restricted` 推荐候选 | 是否自动新增写权 |
|---|---|---|---|---|
| creator / assignee | 保留当前 | 可见 | 可见 | 否；按具体 L3 |
| 显式普通 participant | 新增精确读分支 | 可见 | 可见 | 否；field 资格仍独立 |
| CC | 保留当前 | 已确认可见/评论，仍过入口码 | 已确认可见/评论，仍过入口码 | 否 |
| 当前 role_codes 命中 | 当前规则决定 | 不因角色扩大 | 可见 | 否 |
| 组织透明读/委托 | 保留当前 | 新类型默认相关人员范围已确认 | 类型切换须显式预览，非默认扩大 | 否 |
| `ticket_scope=all` | 保留当前 | 已确认不越过类型范围 | 已确认不越过类型范围 | 否 |
| global admin/superadmin | 当前全域例外 | 同当前 | 同当前 | 仅原有管理动作，不跳业务制衡 |

读可见允许 read/comment（仍过现有入口码），不自动允许 update/assign/delete/participant-manage。推荐新增 `ticket:participant:manage` **按钮码**（目标，非现有码），管理 API 绑定按钮，不挂页面写，不扩大现有唯一 pageWriteAllow。B04已确认由有独立授权的业务管理者管理，creator不因创建身份自动获邀请权。CC可保留现行路由形状，但新增/移除他人的写操作也必须执行同一管理资格，不能借selfService分组绕过；本人退出另按既有语义评估，领域桥只同步具名受管项。普通 participant 用独立管理写入口及 role 精确判定，不能把既有 `ticket_cc` 所有 role 混作 CC。

BK-21 技术顺序：先抽共用可见性谓词/参数输入，再在其内部按 type policy 分组，列表、详情、导出、计数、四视角、关联入口、首页同源。逻辑为 `global_exception OR (policy=inherit AND old_branches) OR (policy=restricted AND restricted_branches)`；不能最后拼一个 `OR scope_all/cc` 逃出 policy 分组。静态 SQL 模板+绑定参数，角色/路径不拼接 SQL。

### 查看范围的必要性与B03说明

Jira将项目访问权限与单张work item的可见性限制分开，能访问项目不等于能打开项目内每张票；这是可参考的分层模式，不要求本项目复制其完整security scheme或实例级安全等级。[Atlassian官方说明](https://support.atlassian.com/jira-cloud-administration/docs/create-a-new-work-item-security-scheme-and-security-levels/)

本项目先按类型配置固定查看范围，复用已有三层授权，不加is_sensitive标记、通用ABAC引擎或单票override。已有普通类型沿用现有组织/委托范围；新类型默认相关人员；确需团队共享用相关人员＋指定角色。B03已确认普通scope=all不穿透这一范围，避免参与人默认对该群体失效。系统级admin/superadmin既有例外另列，不将Jira的管理员规则等同本项目。此必要性属于本项目设计判断，方向已确认但代码尚未实现。

### 5.3 参与人与创建 API 目标

| operation_id / 路由 | 输入/输出 | 不变量 |
|---|---|---|
| `ticket.participants.list.v1` · `GET /api/v1/tickets/:id/participants` | 仅可见票，返回 `user_ref, role, origin, can_remove` | 历史失效用户可回显，不能当 active 候选 |
| `ticket.participants.change.v1` · `POST /api/v1/tickets/participants/change` | `ticket_id, expected_revision, add/remove user_ids` | L1 按钮+该票管理资格；新增 active；域受管项普通调用不可删；唯一 `(ticket_id,user_id,role)`；审计和 revision 同事务 |
| `ticket.type.creation-options.v1` · `GET /api/v1/ticket-types/creation-options` | `org_id` 可选，返回允许创建类型/模板/发布引用 | 按 create_policy×组织创建范围，读角色不自动可建 |
| `ticket.create.v1` · **现有** `POST /api/v1/tickets` | 兼容旧字段，增量预览期望版本 | L1 创建权∩类型创建策略∩目标 org 分支∩启用身份∩已发布类型；任一失败无票/无后动作 |

主仓 `Service.Create` 已有“本人任一组织与目标 org 同分支，或既有全局组织管理权”的创建约束，此处先保留，不擅自改为可向任意组织建单。`creation-options` 不返回被禁止类型名称用作列表线索；不能用 ListTicketTypes 管理列表直接当创建菜单。新写接口 `created_by` 恒由可信上下文/专用领域桥确定，普通 REST/Agent 不可代填。

### 5.4 虚拟风险组怎样取得业务能力

组织是人员归属/资源锚点，角色承载功能权限，两者通过现有 `org_roles` 绑定。虚拟组织不影响赋角：`RoleRepo.GetEffectiveRoleCodes` 从有效直接成员关系取得组织角色，再展开角色继承；它不沿HR部门树自动继承父部门角色。组织角色带来的是L1能力，风险主锚/副锚/owner及状态规则仍独立执行，不能以“已进风险组”替代全部授权。成员过期或移除撤去此来源，若仍有直接角色或别组角色等其它合法来源，不应声称全部权限均消失。

B05补充候选：若风险组全员都承担风险管理职责，可将现有风险管理业务角色绑定该虚拟组，统一获得风险录入、管理和受限派单能力，无须逐人额外授通用建票权或新增派单专属角色。若有只读/旁观成员，不能给全组绑定宽管理角色再指望给个只读角色抵消；应仅将管理角色授予获准人员/职责组，其他成员用只读角色。资源仍归风险组，派出的整改票可按批准映射进入业务组织，不因成员来自多个实体部门就允许任意目标组织。

当前R1矩阵保持：手工录入风险为 `risk:manage` 加主锚范围；单条派单为该码加主锚或当前owner；批量仍限主锚，无owner不派。角色从风险组或其它批准来源取得，不能让“owner身份”单独跳过L1。上述角色配置方案待所有者确认，不是本轮已经赋角。

术语需区分：旧R1/R3所称“风险事件”通常是风险台账记录 `risk_item`，不是工单；D14的攻击/应急安全事件是另一个协作对象，接SOC事件ID与结果，不另造SOC。两类都可复用组织/角色机制，但不因同属风险组就自动授予事件模块全部能力。所有者本次提问的具体含义正在确认，先不新增事件权限码或改变既有矩阵。[按组/角色授予权限的业界参照](https://support.atlassian.com/jira-cloud-administration/docs/grant-or-revoke-permissions-in-a-scheme/)。

类型策略变更须 plan 展示旧/新可见数量、创建影响及受管领域 owner 是否失去通路，管理员确认后 CAS 应用；影响预览需类型管理和相应数据范围，不向普通不可见票返回诊断细节。撤权立即影响后续读取/导出/检索/通知；浏览器缓存和搜索缓存失效，不能等实例升版。策略和参与人变更审计留 reason，现有参与不被无声清空。

## 6. D04 / D07：字段求值、约束与可替换数据查询

### 6.1 公共 schema、求值顺序及输入隔离

field schema 保留现有稳定 key/类型；schema format 与业务 definition 分开。当前十一类型不改原值类型；user/dept 为新增类型，规范值为 string ID（不得转 JS number），单用户/部门首版不承诺数组。旧非选项类型 `field_options` 的 absent/null/`[]` 规约为“无选项”，number/rating 的非空选项拒绝；select/radio/multi_select 保存仍须非空静态选项或合法字典引用。GET→原样 replace→GET 语义相等，先修 F11 再靠导出工具掩盖差异不可接受。

每次求值使用同一解析后的定义和同一业务输入快照：

1. 可信领域字段和原实例值装入上下文，合并允许修改的普通字段，去除伪造 formula/domain 字段输入；未知业务键保留存量兼容但不加入资格/表达式隐含变量。
2. 以请求 context 和固定预算完成 fetch 建议，多参数图最多三趟收敛；不把网络 I/O 放在持锁事务内。域受管字段不参加建议回填。
3. 显隐只依赖非公式业务字段和已允许的词典函数；同一版本的普通提交保留既有语义：隐藏字段不要求 required，并从本次有效 custom_data 投影剔除。显隐不是数据保密机制，机密读权限另验。
4. 求值环境剔除**所有** formula 字段的请求值和旧值。同类型 formula→formula 引用仍禁止，保存时按 AST 的“值位置”识别，不因与函数同名而漏检。C1/件26是前置正确性，不扩大表达式承诺。
5. 服务端计算可见 formula，G2 校验结果/普通值、collect_fields、业务 validator、最终体积。写入只采用服务器产物；预览的公式值不可作为写入权威。

必须区分同版本运行时 hidden prune 与 D01 迁版后字段移除：前者不改变已拍清理语义，后者按已拍合同保留旧字段数据且不渲染/不求值。实现可将迁版移除字段放入隔离的历史分区/旧版本快照，不让其混入目标 G2/公式环境；具体存储不对 adapter 暴露。不得在升级时顺手回刷所有工单，也不得为了保留历史把普通显隐清理悄悄改成继续回写隐藏值。

`field_meta.validation` 目标为 `{min_length,max_length,min,max}`，长度针对 input/textarea/richtext 的**Unicode code point 数量**（Go rune 与 JS `Array.from` 同义），不以字节或 JS UTF-16 code unit 混算；UTF-8 字节限制独立存在。上限缺省表示不加字段级长度限制，不以新默认误伤旧数据。已有 `custom_data` 总量 64 KiB 和 richtext 源文本 256 KiB 都保留，实际同时受更小整体限额；UI 必须展示能真正提交的剩余预算，不能宣称单富文本可无条件写 256 KiB。

添加/收紧 max_length 对旧定义无影响；字典 live 收窄保留现有旧值延续，新增/替换值须在当前启用集。明确空集 `[]` 表示“无新选项”，与依赖失败不同。依赖读取失败对旧纯展示/未改值可标 degraded；新写/修改选项/公式、资格或业务 validator 依赖该值时返回依赖不可用，不能把失败转成任意值放行。这是缩小既有 `expandDictOptions` 故障不校验路径，实施须增加故障回归而非只改注释。

旧 `col_span=1` 不能判断是显式窄列还是历史误写。保留原数据，输出疑似清单/预览；未确认前渲染可按 legacy layout 模式（单列，不解释旧列宽），用户重新确认保存为新草稿/新发布后启用栅格。不得批量把所有 1 改为 24。只修改布局也要按定义发布，不给旧单做“非结构 hotfix”。

### 6.2 目标求值与 prefill 接口

| operation_id / 候选路由 | 输入 | 输出和约束 |
|---|---|---|
| `ticket.fields.evaluate.v1` · `POST /api/v1/tickets/fields/evaluate` | 新建：`type_code,definition_version,org_id,values`；已有：`ticket_id,expected_revision,values` | `definition_ref,visible_keys,computed_values,validation_errors,dependency_states,evaluation_id`；无持久业务写；新建须可创建，已有须可见且只求被允许输入的字段；限制大小/表达式/查询次数 |
| `ticket.fields.prefill.v1` · `POST /api/v1/tickets/fields/prefill` | 同上上下文 + `fetch_rule_id,values,input_generation` | `suggested_values,source_ref,observed_at,status=ok/not_found/unavailable,echoed_input_generation`；不接受任意 URL/SQL/sourceConfig/secret；受同一源及字段白名单 |

设计器的未保存 schema 预览可用单独 `draft` 管理模式，须 `ticket:type:manage`，不向普通调用者开放上传任意表达式调用外网；字段渲染 preview 可纯本地，服务端真求值按上述预算。evaluation/prefill 不创建工单/派单/发消息。UI 防抖和取消旧请求，并比较 `input_generation` 丢弃迟到响应，不能让旧 IP 查询结果覆盖新 IP 的人工输入。

fetch 统一目标配置为 `{source_ref, query_ref, params, mapping}`；旧 `{source:{type,config},trigger_by,mapping}` 由平台兼容解析为单参数规则，不要求旧定义/私有 adapter 立刻改。新 params 用显式 `{field:"host_ip"}` / `{const:"cn-east"}` 表达引用，避免把字符串常量误当字段。保存检查入参契约、映射目标存在且不重复、源许可、禁止依赖 formula、引用无环；多次查询同参去重。一查多填优先，最多三跳；超过预算返回 missed/unavailable，不跑通用工作流。

建议预填仍允许人工修改。目标“补空”精确为字段不存在、null、空字符串、空数组；数值 0 和 false 是有效值，不补。旧字段若依赖“只补缺键”的行为，由 schema format 兼容解析保留，不能无版本地覆盖旧客户端显式空值。新规则的用户清空仍按已拍“总是补空”兜底，UI 明示；受管身份字段永远由领域命令而非 fetch 填写。

### 6.3 公共 Query 面与 CMDB 归属

对内网扩展发布的是带版本 DTO 的查询用例，而非 Go `DataFetcher`、`internal/model` 或 repository。目标 Query 请求为 `query_ref,parameters,resource_context,request_id`，调用方身份由认证确定；响应为 `status,records,source_ref,source_revision,observed_at,quality,warnings`，记录字段按 query_ref 允许集裁剪。普通用户无权自行切换成 service identity；服务查询要有明确受限数据集授权，`X-Operator` 仅归因不授予权限。

首批 query_ref：`directory.user.resolve.v1`（稳定人员映射/启用态）、`directory.org-head.resolve.v1`（直接/指定层级负责人）、`asset.owner.resolve.v1`（CMDB 资产引用/owner 建议/受影响 org）、`intel.conclusion.lookup.v1`（已有情报结论及引用）。具体 HTTP 封装与注册接缝由平台扩展合同维护，不能因本文命名就声称入口已实现。

CMDB 返回 `asset_ref={provider,external_id}`、可选本地映射 ref、`owner_external_ref`、`org_external_ref`、`observed_at`、深链和可展示快照。平台只做身份/组织解析与必要缓存；未知或歧义 owner 返回 unresolved，不按同名人工匹配。人工临时系统映射带 `origin=local_override`、责任人/理由/有效期，与真实 CMDB reference 分开；查询恢复不覆盖工单已确认的人工建议值，也不改变本地 risk owner，后者必须走改派命令。

## 7. D06：HR 公共规范化输入与组织生命周期

### 7.1 公私边界与目标 DTO

公共主仓维护版本化 DirectorySync 合同、验证/导入用例、身份映射及生命周期；私有 connector 独立仓/服务封装真实 HR SDK、私有字段、分页、endpoint、网络和凭据引用。内网私有代码不 import 公共仓 `internal`，不直写 IAM 表，不依赖 ltree/ID 生成算法。mock 和真实 connector 发送**同一规范化合同**；公开合成样例不含内部地址、工号或人员数据。

能力协商 `DirectoryProviderCapabilities` 目标字段：`provider_ref,protocol_major,full_snapshot,incremental,explicit_tombstones,stable_snapshot_token,ordered_entity_revision,manager_ref,department_owners,source_status_semantics`。只有 true 且契约验过的能力才能启用对应 reconcile。未提供 tombstone 不推断其已实现，增量缺失不等于离职。

```json
{
  "provider_ref": "mock-directory", "batch_id": "sample-full-001",
  "mode": "full", "snapshot_token": "sample-snapshot-7",
  "scope_ref": "all-authorized-records", "domain": "users", "complete": true,
  "records": [{
    "external_id": "person-001", "source_revision": "17", "observed_at": "2026-11-11T02:00:00Z",
    "lifecycle": "active", "employee_no": "DEMO001", "display_name": "合成人员甲",
    "primary_department_external_id": "dept-001", "manager_external_id": "person-002"
  }], "next_cursor": null
}
```

`DepartmentRecord` 含 `external_id,parent_external_id,code,display_name,lifecycle=active/retired,owner_external_ids[],source_revision,observed_at`；`UserRecord` 含上例、可选已核验域登录标识/联系字段/岗位 ref。`Tombstone` 为 `entity_kind,external_id,source_revision,effective_at,reason`，不复用空记录/查无结果；provider 可不支持，平台按能力降级。`complete=true` 必须是平台验证整个 domain/scope 的所有页后生成的完成标记，不能凭单页字段立即做缺失对账。scope 变化须新基线，不能将缩小授权范围当批量离职。

目标接入用例为 `directory.batch.stage.v1`、`directory.batch.validate.v1`、`directory.batch.apply.v1`、`directory.sync.receipt.v1`；候选 POST 输入有 batch_id/expected baseline/hash，GET receipt 只给同步运维权限和脱敏统计。认证 connector 只绑定某 provider/scope 的导入许可，不享用户/角色管理通配权；业务 HTTP 路由由平台注册合同最终确定，不把旧提案示意 `/admin/hr/sync` 当现行 API。

### 7.2 写主、映射与发布

- `(provider_ref,entity_kind,external_id)` 维护外部身份到本地 ID 的唯一绑定及 source revision，组织/用户活跃关系按此防并发串号。employee_no/domain 可辅助**发现冲突**，不自动把两个人合并；外部 ID 重用/复职必须显式核实身份世代，不能仅同工号就继承旧账号权限。现有用户外部 ID 未有该唯一数据库保证，迁移前先扫重复/冲突，隔离疑似记录。
- HR 写人员真实姓名、主部门、manager、岗位和来源生命周期等 provider-owned 字段；本地 IAM 写 username/密码/角色、本地虚拟组、临时成员、封禁及授权。同步不能重启本地已封禁账号；有效可登录条件为 source active ∩ local active ∩账户安全条件。当前单 `users.status` 的来源/本地状态拆分是目标，须数据迁移说明，不能直接拿 active HR 事件覆盖 status=0。
- 人工兜底不直接改 provider 原始事实。`DirectoryOverride` 记录 field/value/reason/actor/expiry/source_baseline；有效展示值可采用 override，权限锚点/禁用态不得被 override 静默放宽。HR 明确 active/terminated 状态优先决定源生命周期，角色仍以本地批准为准。
- 先完整 stage/验证组织，再应用无环父树；随后用户/manager 解析。manager 可能晚到：引用保留 unresolved，不报“已找到上级”，后续事件/重跑补解；负责人需 active，目标层级无负责人时返回 unavailable 不无限向上广播。
- 批处理状态：`staging→validated→applying→succeeded/partial/failed`，另有 `rejected`（合同/基线不符）。整 domain/scope 只有完整校验后才允许 absence reconcile；任一页失败/重复页冲突/父树环/大量异常缺失先停缺失处理，保留上次成功基线。partial 的正常 upsert 可保留并逐项报结果，**不得**顺带对缺失者禁用/撤部。
- 同 provider/scope 一次一个 apply，持久 run_id/基线/阶段水位，lease 失效不能并发推广同一批。短事务按完整顶层子树更新，失败子树回滚并标 partial；已提交子树不回滚为“全没发生”。reconcile 完成后才推进成功 baseline/cursor；同批重放返回原收据。

增量按 provider 声明的实体 revision 比较，旧序号不覆盖新值；相同序号不同内容隔离为 source conflict。无可比较序号时，只能用契约确认的 observed_at 和 tie-break 规则；不把本地 received_at 当真新鲜度。乱序 tombstone 不得取消更新后的 active 记录。只有 explicit tombstone 或已完整批准的 full absence 才触发生命周期动作。

### 7.3 离职、禁用与待交接

可信离职到达后**先禁登录/撤销会话**，不等待业务继任者，也不因名下未结票阻止安全动作。复用已有 disabled/RT 撤销链，写持久 security intent 重试 Redis/缓存失效；安全栅栏尚未完成时明确 `revocation_pending` 并告警，不能报“已完全踢出”。所有受保护请求不能只信旧 AT：正常可用受版本约束缓存加速，缓存不可用/失效未确认时检查本地权威状态；权威也不可核时拒绝受保护请求，不能放行离职者继续读。高风险写在事务中再次检查 active/epoch。不得改系统种子用户或最后管理员安全守卫；源误标系统账号为 terminated 时隔离高优先级异常，不自动突破 break-glass 规则。

同次 lifecycle command 写 `directory.user.deactivated` 和 `HandoverCase`：`case_id,departing_user_id,reason,source_ref,effective_at,status=open/in_progress/completed,revision,inventory_snapshot`。清单至少含当前处理工单、当前 risk owner、接受申请/复核待办、负责人/manager 引用；盘点失败可补扫，不丢案例。**owner/assignee 不自动置空抹历史，原 created_by 永不改写。**

交接动作逐项校验接管者 active、业务范围、目标实例 revision，成功写双边审计+通知；失败项可重试，案件只在所有在途项已移交或人工确认无须处理后完成。工单回池/继任者/组合属于 B06 策略；未批准时推荐保留原状态并标“待交接”、风险团队/组织管理者获精确处理入口，不能静默把 in_progress 回到 open 或选上级充数。人员重新启用不反向抢回已交接工作。

事件触发和周期补扫共同保证全量清单；禁用已发生而 HR 处理崩溃，补扫由 run_id/用户状态查补 HandoverCase。发通知失败只重试 intent，不回滚离职安全动作。跨仓 extension 可消费脱敏的用户失效事件清自己的执行缓存，但不得因此直接改主仓工单表。

### 7.4 部门移动、撤销与可见性

移动复用 `OrgRepo.Move` 的完整语义：组织子树（含本地虚拟组）path 与工单/模板的授权冗余 path 同一事务级联，并使授权缓存失效。未来风险主/副锚路径若冗余也走同一领域投影合同；扩展不保存 ltree 当永久业务 ID。树变更后按 live 组织关系授权，历史展示用快照名称/组织 ref，不拿历史路径继续授权。

撤部先生成 plan：撤销实体/后代、虚拟组、活跃成员、org_roles、存量工单/风险、负责人引用、可见性变动和目标接管锚点。不得硬删；不得将残留绑定隐式搬到更高范围导致权限扩张。旧提案给过“虚拟组最近存续 HR 祖先上挂”的候选默认，但相关撤部三项尚待确认；B07 明确 owner 选择。批准 A 时，移动保留虚拟组 ID/成员/角色，重算路径，**同时**预览祖先委托和 scope 可见集变化；scope 扩大需要同 plan 明示确认。

撤销后历史工单/风险仍可查、读者按现行管理锚点/参与分支，已撤组织只读展示不可作为新创建目标。是否迁移在途业务 `org_id` 或仅另设受管接管范围、历史报表归属采用 B07；未批准前保留部门 ID、禁新写组织结构，并暴露待处理清单，不用一次软删让记录“失踪”。root/system 不能退役；没有合法存续祖先或接管者不能落空父节点。

### 7.5 D06 合成验收集

固定样例覆盖：正常全量/重复批、组织先晚到、manager 延迟/自环/断链、员工换主部门但 local 虚拟组/角色保留、同工号不同外部人、external_id 重用、乱序 active/terminated/tombstone、分页失败/缩小scope/错误 complete、撤部带虚拟组和 org_roles、移树带既有工单、禁用后旧 AT 拒绝、撤会话暂失败补偿、离职交接部分成功/重跑/复职。每例声明预期业务状态/授权可见集和 run receipt；这证明 mock/contract，不证明真实 HR 身份或分页语义已验证。

## 8. D08–D10：风险来源、周期和处置闭环

### 8.1 规范化来源合同与唯一写主

私有 RiskSourceConnector 将已有专业发现/结论转换为公共 `FindingObservationV1`：

```json
{
  "source_ref": "mock-vulnerability", "source_item_key": "asset-01:vuln-01:443",
  "event_id": "sample-event-42", "source_sequence": "42",
  "observed_at": "2026-11-11T03:00:00Z", "observation_kind": "observed",
  "asset_ref": {"provider": "mock-cmdb", "external_id": "asset-01"},
  "finding_class": "vulnerability", "finding_key": "vuln-01", "port": 443,
  "source_severity": {"value": "5", "scheme": "provider-five-level"},
  "conclusion_ref": "sample-conclusion-42", "evidence_refs": ["sample-evidence-01"],
  "mapping_ref": "mapping:v1", "attributes": {"service": "https"}
}
```

来源强 schema 字段与允许扩展 attributes 分开；不接受 `local_status/owner/acceptance/created_by` 作为源写字段。SourceMapping 为版本化产物，声明稳定键选择、字段归属、严重性映射与归一 schema 版本。稳定键优先源不可变 ID；无可靠 ID 时选经契约确认的资产引用+发现标识+端口等，**禁止姓名/IP 漂移时静默改键或仅靠文本标题**。OCSF 只作为裁剪参考和映射来源，不要求复制全套检测平台。

目标接入操作 `risk.observation.report.v1` 对应[已规划](../phase6/01-r1-ingest-ledger.md) `POST /api/v1/risk-items/report`：源凭据限定 source_ref/mode/schema/上限，不以持 risk:manage 通配替代。返回每 observation 的 `accepted/duplicate/stale/quarantined`、ingest receipt、可受权查询的资源引用；accepted 代表已持久接入，不等于风险已评估/整改。任务拉取由唯一任务平台执行，平台状态登记和 adapter 分页不各开一套周期调度。

每 source 只启一个推/拉模式。输入先落可重试接入记录，再按版本映射/解析引用；本地风险处置是唯一写主，源重刷只更新源镜像/观察计数及 source 事件。重复 event_id 同 hash 不增加计数；同 event_id 不同 hash 隔离；相同 sequence 异内容隔离。有限重试超限进入受控待处置队列，有 receipt 可重放，不无限吞错。

`observation_kind=withdrawn/deleted` 表示源撤回记录，**不是修复证据、不是本地关单**。查无结果、某次全量未出现、拉取失败均不自动关风险。需要完整快照删除识别时同 HR 明确 complete/scope/cursor，但来源缺失仍只改镜像可用性。平台不执行 SOC 检测关联、漏洞验证扫描或 TI 评分，只消费外部带来源/时间的结论；本地复测可人工确认或引用专业工具的受权结果。

### 8.2 指纹族、周期与并发防重的技术定案

为满足已拍“同指纹在办归并，修复后真回归新单”，推荐逻辑分层：

- `RiskFingerprintFamily`：`family_id,fingerprint_scope,canonical_key_hash,key_schema_version,current_cycle_id,revision`；唯一 `(fingerprint_scope,canonical_key_hash,key_schema_version)`。scope 含 source/key命名空间；跨源同事件不默认自动合并，后续人工合并另命令。
- `RiskCycle`：`risk_id,human_key,family_id,generation,regression_of,status,revision,org_id,affected_org_id,owner_user_id,level,source_severity_snapshot,init_deadline,deadline,treatment,accepted_until,closure_kind,closed_at,attributes`，唯一 `(family_id,generation)`。对外风险 ID/人读编号是 cycle，不暴露 family 表主键算法。
- `FindingObservation` 以 `(source_ref,event_id)` 幂等，来源稳定记录映射独立于 risk cycle；`RiskEvent` 和 treatment 历史追加、与主状态变更同事务。已关闭单不删，归档 365 天/原始180天/risk_events随单保留继续依已拍 R1 执行。

同指纹新 observation 锁 family 行/当前 cycle：current 在办则只归并；accepted 未到期/误报抑制有效只计数；修复后经排序确认的**新观察**进入新 generation 并回链。关闭时保留相关源的 observation checkpoint（最后已消费 event/sequence/observed_at）；来源的 observed_at 必须表示专业系统再次实际观察，不是记录被改备注的更新时间。只有 checkpoint 之后的实际新观察能判回归；更大的传输序号但仍指旧观察不算。并发两条回归请求最多创建一代，family head 原子移动；不采用全表 `risk_items.fingerprint UNIQUE`。临界窗口旧序号、老 observed_at 或已接入过的迟到事件不能制造回归。真正新鲜度取经契约验证的 sequence/observed_at；不能判断就隔离待核，不以接收时间猜。

接受到期 cron 与新观察同取 cycle 锁：仍 accepted 且当前 acceptance 版本到期则重开该 cycle 为 assessed，写 expiry 事件；续期先成功则 cron 不重开。误报抑制过期后的新观察进入新的待评估 cycle，并标 `suppression_expired`，不记“修复后回归”；人工 merge 的从单保持回链，后续归并遵循已确认主族映射，不自动跨族混合。

这只是落实既有业务语义的并发/索引技术方案，不新拍“是否回归”。实施迁移须先查已有数据和来源稳定键，尚无风险代码不代表能预占全局编号。不同真实源若不能提供可判断的新鲜度，先用 mock/contract 跑明确定义的边界，把真实该源列 pending 而非降低全平台规则。

### 8.3 固定风险状态机和接受申请

目标状态枚举：`new`（待评估）、`assessed`（已评估/待处置）、`remediating`、`pending_verify`、`accepted`、`closed`。`closure_kind=fixed/false_positive/duplicate/transfer/avoid` 区分结论；“误报关闭”是 closed 的明确业务终点，不能与整改完成混算。`acceptance_pending` 由独立申请表示，不必挪用工单状态。固定 Go 领域状态机，无风险流程设计器。

| 业务命令 | 前置 / 字段 | 成功后及禁止事项 |
|---|---|---|
| assess | new/assessed；主锚管理者或 owner + risk:manage；level 四档、依据、deadline/treatment | assessed；源 severity 与本地处置 level 分列，不能宣称自研 CVSS/SOC 判定 |
| assign owner | 主锚管理者；新 owner active、身份明确，expected revision | owner 变更+交接；主风险团队 org 不跟 owner 漂移；链接处理见 §8.4 |
| dispatch | assessed/可整改状态；当前 owner 非空且 active；risk:manage+主锚/owner | remediating + active link；无 owner 400，不默认为操作者 |
| accept-request | assessed/remediating；当前 owner 本人+manage；理由、期望到期日、风险 revision、当前 owner | 创建 pending 申请；不直接 accepted，不自动关工单 |
| accept-confirm/reject | 当前主锚管理角色+manage；申请仍 pending、owner/风险 revision 与申请基线相符；默认 actor≠applicant | confirm→accepted；reject→申请 rejected，风险仍原处置态；无理由驳回拒绝 |
| accept-expire | 当前有效接受版本确已到期；系统限定用例 | accepted→assessed；原 cycle 重审；不自动续期，不自动建重复工单 |
| verify/close | 当前有资格的主锚/owner，合法状态；五档结论及证据 | fixed→closed；accept→申请；false_positive→closed+抑制；duplicate→merge；cancelled→assessed；“不修”不能冒充 fixed |

`RiskAcceptance` 含 `request_id,risk_id,applicant_id,owner_at_request,risk_revision,reason,valid_until,status=pending/approved/rejected/superseded/expired,revision,decided_by,decision_reason,decided_at`。owner、级别、关键范围或申请内容改变使旧 pending 申请 superseded，确认需新申请；不能确认旧 owner 的旧风险敞口。续期是新接受版本，保留历史；有效期必须未来，不接受无限期 null。默认制衡开关若已授权关闭须审计 reason/actor/策略版本；管理员不能通过自操作旁路关闭。

风险公共 DTO/命令使用 R1 已规划的 `/api/v1/risk-items` 路由族，以下均为待实现合同：

| operation_id / 路由 | 请求字段 | 响应字段 |
|---|---|---|
| `risk.get.v1` · `GET /risk-items/:id` | 路径 ID；正常认证 | `id,human_key,revision,status,closure_kind,owner_ref,org_ref,affected_org_ref,level,source_severity,deadline,acceptance_summary,source_refs,authorized_ticket_links,viewer_capabilities`；来源和关联对象逐项重验，隐藏不可读的深链 |
| `risk.assess.v1` · `POST /risk-items/assess` | `id,expected_revision,level,assessment_note,treatment,deadline?` | 实际 risk revision/状态、level/deadline、event receipt；changed/unchanged可区分 |
| `risk.owner.assign.v1` · `POST /risk-items/assign` | `id,expected_revision,owner_user_id,reason,link_expected_revisions?` | owner新旧ref、受影响link结果、handover_ref、risk revision |
| `risk.acceptance.request.v1` · `POST /risk-items/accept-request` | `id,expected_revision,reason,valid_until` | `acceptance_id,acceptance_revision,status=pending,risk_revision` |
| `risk.acceptance.decide.v1` · `POST /risk-items/accept-confirm` | `acceptance_id,expected_acceptance_revision,expected_risk_revision,decision=approve/reject,reason` | 申请状态/版本、risk当前状态/版本、valid_until及可靠通知 receipt |
| `risk.dispatch.v1` · `POST /risk-items/dispatch`；`risk.dispatch.batch.v1` · `POST /risk-items/batch-dispatch` | `plan_id,items[{risk_id,expected_revision}],target_type_code,expected_definition_version,append_ticket_id?,expected_ticket_revision?` | `groups[{ticket_ref,definition_ref,revision}],items[{risk_id,result=created/linked/already_linked/denied/invalid/conflict,receipt_ref,safe_reason}],pending_intents`；只对可见对象返回引用，不因批量报错泄漏另一张票 |
| `risk.link.change.v1` · `POST /risk-items/link-add` / `link-remove` | `risk_id,ticket_id,expected_risk_revision,expected_ticket_revision,expected_link_revision?,reason` | 新链接/实例revision、双方事件receipt；active唯一/同owner/上限锁内验证 |
| `risk.resolution.submit.v1` · `POST /risk-items/close`；领域 finish 的候选 `POST /risk-items/remediation-finish` | `risk_id或ticket_id,expected_revision,results[{link_id,expected_link_revision,resolution,evidence_refs,note,master_risk_id?}]` | 每项 `applied/pending_confirmation/conflict/rejected`、风险/链接版本和receipt；不是一个总success掩盖部分结果 |

上表只为表达路径后缀省略公共 `/api/v1` 前缀，POST 的资源标识仍全部在 body。新创建/上报/普通手工录入、误报/抑制和merge的权限沿R1矩阵，不能复用close让owner取得主锚专有的抑制或合并权限。恢复接口只允许查询或重放已授权的receipt，不接受任意新的“system result”。

deadline 默认自然日按 level policy 带出，`init_deadline` 保存首次值；owner/管理者的改期留旧新值及理由。未接真实工作日历时 `workday` 配置校验失败或保持不可启用，不能按自然日伪装工作日。transfer/avoid 第一档只登记决策与外部依据，不自建专业转移/退役流程；是否已可确认消除敞口须人工结论，不因策略字段变化自动 closed。

### 8.4 N:1 整改单桥、当前 owner 和结果消费

`RiskTicketLink` 目标含 `link_id,risk_id,ticket_id,status=active/resolved/detached,resolution,note,verified_by,verified_at,revision,created_at,ended_at,owner_at_link`。active 唯一约束继续为每 risk 至多一条；链接不复制 ticket status 当写主。分组键至少当前 owner、risk管理 org、工单目标 type/definition 和受管业务范围，≤20 条；不能只同 owner 就把不兼容组织/流程的风险强塞一单。

工单 priority 为 active risks 最高 level 映射，业务截止为其最早 deadline；新增、移出、改级和改期均在链接变更合同内重算，保留旧/新聚合值及来源risk revision。目标业务截止 `business_due_at` 与当前节点 SLA 的 `sla_due_at` 分开：前者是风险整改承诺，后者由入态规则计算，不能为实现最早deadline直接覆盖节点计时。UI同时标明两者，若统一“最先到期”排序可取非空最小值，但统计注明是业务超期还是节点SLA超时。无active links时聚合值清空/完成收据封存，不沿用已移出风险的截止。人工改变受管聚合字段须走有理由的专用调整而非普通update；风险事实的deadline不会被票聚合反写。

统计按风险数和每risk创建→该条合法结论计算MTTR，不能把一张20风险工单当一个风险完成。提醒可按票折叠，仍保留每riskdeadline和投递事实。初始模板 `default_sla_minutes` 的已拍补消费保留显式值>模板>类型次序，与上述业务deadline分开；模板仅创建前预填，不能改变旧单所钉状态SLA定义。

派单 plan 返回逐项可选、阻断、owner/风险 revision、目标类型发布引用、候选同 owner 在办单。apply 在 group 锁和逐项风险锁下复验，已有 active link 返回原票作为幂等成功；合法追加也锁目标票和链接集合 revision，竞争剩余名额时重新分组，不越过20。一次 group 的建票/links/风险状态/双域事件/CC intent 同事务；bulk 以 group 为原子单元，响应逐 risk 成败，允许修复后只重试失败项，不留“空壳票+无链接”。

B05简化候选：`risk.dispatch.v1` 是用例/operation_id，**不新增同名权限码或一套派单角色**。复用已批准的 `risk:manage` 与资源范围：单条仍为主锚管理者或当前owner，批量仍为主锚管理者，不能因owner可派自己的风险就扩大批量范围。界面只有“派发整改工单”；可用目标由受管配置限制为整改类型/组织/模板，类型须启用且已发布，字段/G2/公式、附件归属、初态和active owner等沿用工单领域规则。建议此入口不额外要求通用 `ticket:create`；普通自由建单仍执行原创建权限及类型/组织约束，查看权限不自动允许派单。用户正在确认此简化方案，不能把解释问题视为批准。

领域用例保留R3已批准的 `created_by=已核当前owner`、`operator_user_id=真实操作者`，无owner拒派；两种身份分别审计。普通REST/Agent不可任意传 `created_by` 或 `skip_auth`。实施应抽取共享创建校验与受限可信创建上下文，不能只以owner身份调用现有 `Service.Create`，因为该函数以actor派生创建人及组织检查；也不能为省事跳过类型和组织边界。未决不擅启扩大后的领域创建范围。

`risk_owner` 是 domain-managed user 字段，由桥维护为当前 active link 共同 owner；普通编辑/fetch/Agent 不能改。owner 交接时，先按被改风险锁与 link revision 处理：若一张票全部 active risks 同步移给同一新人，可原子更新受管字段/参与者并审计；若只转移其中部分，**detach 这些风险并让其进入待重新派单/交接**，保留旧票历史和剩余风险当前 owner，不把整票复核权送给新 owner。必要新票由授权交接者确认后派，不自动把历史 creator 改掉。旧 creator 的历史读权依 D03已批准策略仍可能保留，不能靠改 owner 假称已完全撤回旧作者阅读。

初始 owner 和后续 owner 的受管 participant/CC 来自同一可靠 bridge intent；receipt 可见 `collaboration_pending`，重试和对账修复。若当前 owner 未取得 L1/可见性，不能只靠 `field:` 强行复核，应报角色/策略阻断。受管 participant 只给该票必要读/相关转换资格，不附送 update/delete 或整个风险团队范围。

**P6.2 最小完成语义不等待整套 D16。** 目标 `risk.remediation.finish.v1` 为明确领域用例：持当前处理资格提交每 active link 的结论与证据；review_gate=on 时仅走已发布合法边 `in_progress→pending_verify`，off 时走已发布合法边 `in_progress→closed`。两种都执行 collect_fields/G2/证据/业务 validator/SLA/event；不伪造一个人类复核动作。发布预检要求这两条受管边与 gate 配置一致，缺边则阻断配置。D16 后续通用默认边、条件多命中、循环和恢复不得反向推翻此业务用例，可将其内部边选择委托给同一受限内核。此拆分保留“默认无额外复核直接闭环”，不建设临时通用编排器。

复核/完成结果按 risk 逐项，不能用 ticket closed 一刀切所有风险 fixed。固定结论需整改措施/证据 ref/复测人/时间；accept 必须由当前owner本人进入接受申请，且不先关风险，duplicate 指向受权可见 master，false_positive 的抑制由风险主锚管理者确认，cancelled 回 assessed。某 link 失败/被detach 不阻止其它合法风险，票的完成收据列每项结果与 pending，业务 terminal 的完成条件由已发布受管定义明确：所有 active links 已有**已授权且已生效**的合法结论或已detach，否则拒绝整票 terminal。

处理人不是owner时可在该link记录 `proposed_resolution/proposed_by/proposed_at` 及证据，但不能借finish代owner申请接受；需主锚权限的误报抑制/合并亦只产生建议。它是该link的待处理事实，不是通用审批引擎。owner合法提出accept后，link仍active且 `resolution_status=pending_confirmation`；确认批准才置resolved/accepted，驳回则清待确认标记并回remediating/待合法结论，不丢原建议历史。确认与link状态变更可靠协调；相关工单仍由合法完成用例关单，不能因风险接受/人工关闭在另一页发生就擅自关票。未完成的待owner/待团队项通过capabilities/receipt和通知显示，gate=off也不跳过这类不可省的业务确认。其余fixed项可逐risk结算，不能将部分pending报为全票完成。

票终态和持久 `risk-resolution` intent 同事务，风险消费按 `(ticket_id,ticket_revision,link_id,resolution_version)` 幂等。当前仍同库可由事务协调当次落实双域事实；若采用提交后消费，UI 明确 pending，不将票提交成功等同风险已关闭。R01 扫描未应用 intent、未结 link 和票终态差异，使用原始结论快照恢复，不能只猜最终 ticket status。风险被人工先关闭只通知工单，不自动关闭工单；迟到旧结果若 risk revision/owner/link 已变，进入 conflict 待人工，不复活/覆盖新生命周期。

物理删除不是风险产品操作；风险/链接/历史保留。现有管理员物理删票仍可能发生，桥必须保留不可变 ticket_ref/human_key/definition/结论审计快照及 deleted 引用（或受保护引用拒删），不能仅依赖 FK SET NULL 丢失来源。选择内部墓碑形式是技术实现，不对外暴露级联表结构。

### B05业界参照与简化理由

DefectDojo支持在发现页手动推送或按配置推送到Jira，也支持多个发现形成一张工单；其集成配置限定映射，连接账号仍需目标Jira的创建/更新权限。[DefectDojo开源版官方说明](https://docs.defectdojo.com/connectors/os_jira/os__jira_guide/)；Jira常规建单要求Create work items权限，[Atlassian官方说明](https://support.atlassian.com/jira-software-cloud/docs/create-a-work-item-and-a-subtask/)。这些例子说明业务派单入口与目标建单授权需协调，并不证明本项目必须复制两套用户角色。基于本平台同域建单与已有风险职责，选择复用风险权限、在共享工单用例限制目标范围是本项目设计建议。

## 9. D15：跨域事件、受权通知与扩展消费

### 9.1 稳定事件封套与权威归属

事件目标公共封套为 `event_id,event_type,schema_version,aggregate_ref,aggregate_revision,occurred_at,actor_ref,operator_ref,correlation_id,causation_id,payload,visibility_ref`。投递至少一次；消费者按 event_id 去重，按 aggregate revision 防回退，不依赖跨对象全局顺序。payload 增加可选字段不迫使旧订阅者升级；新终态/新结论是语义变更，需 capability 协商或新事件版本，未知 enum 不允许默认当成功。

| 事件族 / 公共消费面 | 写主 | 允许用途和限制 |
|---|---|---|
| `ticket.definition.published.v1` / definition.get | 工单定义域 | 平台兼容 adapter 刷 schema；不把发布当实例已迁版 |
| `ticket.state.changed.v1`、`ticket.fields.changed.v1` / ticket.get | 工单域 | 更新内部扩展投影；敏感字段只给变化键/受权引用，不广播 custom_data 全文 |
| `directory.user.deactivated.v1`、`directory.org.changed.v1` / directory.resolve | IAM | 失效与交接提示；只有被准许的最小身份字段，不能给下游改角色授权 |
| `risk.observation.accepted.v1`、`risk.cycle.created.v1` / risk.get | 风险接入/生命周期 | 区分镜像观察与新风险周期；来源不能反向写本地状态 |
| `risk.owner.changed.v1`、`risk.assessed.v1`、`risk.acceptance.*.v1`、`risk.state.changed.v1` | 风险域 | 双向协同/通知；每次 follow-up 用 public API 重验，不直接访问风险表 |
| `risk.ticket.link.changed.v1`、`risk.resolution.applied.v1` | 风险工单桥 | 准确双域对账和证据；link revision/结果版本用于去重，不猜票终态 |

HTTP REST、MCP、SDK和内部 extension 共用同一用例/查询授权。catalog 只将稳定 operation_id/capability_ref 绑定真实路由/门面，不另造万能 RPC，不要求旧 REST 客户端改调用。私有扩展只能用公开 DTO/版本化事件/受权 Query，禁止依赖 Go internal/model/repo、数据库表、ltree或反射序列化内部对象。业务 definition_version 的新增不会改 protocol_major；新流程状态和字段按实例 definition/schema 解析，旧 adapter 可忽略未知可选字段、只操作其声明支持的业务 capability。

持久订阅/投递权限撤销后终止后续消息和补发；事件含的数据不能比订阅主体受权 Query 更宽。事件可只给资源 ref，由消费时重新取数；修订授权后旧签名 payload 不能当读许可。平台升级由平台维护兼容映射，私有 HR/source SDK 与 URL 不进入公共主仓。通用扩展/内网迁移的安装、注册和版本兼容方案由对应平台设计维护，本文只定义领域不变量。

### 9.2 风险首通知最小合同

风险事件 payload 必含 `risk_ref/human_key,org_id,affected_org_id,current_owner_user_id,level,deadline,aggregate_revision,definition/source refs as applicable`，角色与源字段经过最小化；created_by=录入人或源服务不能代替 owner 收件人。S6 词表/模板变量、装配订阅、owner 解析和持久投递 intent 在首条风险通知启用前一并验，不等 D11 完整通知编辑器。

**已确认场景保持有效**：R1 §3.1/P1-6记录的高级别风险沿组织树抄送一级部门负责人，是2026-11-02所有者已给出的需求，本候选不能将其降为可选或再问是否需要。待B08冻结的是源五级→平台四档的触发阈值、从哪个受影响组织上溯、多个负责人/频次/时区/静默例外。负责人须当前active且具对应资源收件/读取资格；缺失记recipient_unresolved，无资格记recipient_unauthorized并进入配置待办，不广播、猜人或静默少发，也不以通知要求绕过三层授权。

| 触发 | 默认最小收件者 | 附加/去重边界 |
|---|---|---|
| 新风险未定 owner | 主锚内已配置且受权的风险管理队列/负责人 | 不广播受影响部门全员；无合法收件人留 `recipient_unresolved` 运维项 |
| 指派/交接 | 新 owner；旧 owner 只接自身交接告知 | 发送时 active/可见性重验；旧 owner 不附新状态机密详情 |
| 临期/超期/级别变化 | 当前 owner | 按映射后四档；高级别保留抄送一级部门负责人，具体阈值/触发时机B08冻结；找不到不无限上溯或静默降为可选 |
| 接受待确认 | 主锚受权管理者且排除申请人 | 至少一名有效确认人，不满足制衡报告组织配置问题 |
| 接受批准/驳回/到期重审 | 当前 owner；相关实际申请人仅按当前可见性 | 到期跟当前接受版本，旧提醒/旧owner intent 不发错人 |
| 工单提交/复核/风险结论 | 当前工单/风险受权接收者 | 相同 group 的提醒可折叠展示，逐风险事实和审计不折叠 |

收件人 intent 保存“解析规则+资源/revision”，发送时重新解析当前 owner/资格与偏好，不把 enqueue 时的旧 owner 永久冻结。level→负责人/渠道规则版本、组织负责人来源和重试失败可对账；主锚风险管理者通知需具体队列配置，不能 source 缺字段时默认发 admin/all。支持站内最低保障，外部渠道仅适配已有消息设施；偏好、静默窗、节流、投递失败分别记状态，不把“规则命中”当已读或送达。

首发四档矩阵配置不等于做完整通知编辑器。可审阅的 **B08 推荐候选**如下；它沿用原 P1-6 的示例方向，但不是已经批准的通知频率。指派/交接/接受待办/决定/到期均即时站内，下面只区分风险创建、级别和时限提醒：

| 平台 level | 创建/升到本档 | 临期/超期 | 可选额外收件人 |
|---|---|---|---|
| critical / priority1 | 即时 owner+受权协同者 | 即时 owner，重复按S6节流；每自然日最多一条同类自动提醒 | 一级部门负责人；平台哪些档触发由源级别映射与B08冻结，不取消该能力 |
| high / priority2 | 即时 owner+受权协同者 | 即时 owner，同类每日抑止 | 同上；阈值未定不将示例两档当已批准生产映射 |
| medium / priority3 | owner 每日汇总 | 临期进入每日汇总；首次超期即时 owner，此后同类每日抑止 | 默认无，需配置 |
| low / priority4 | owner 每日汇总 | 同medium | 默认无，需配置 |

每日汇总时区/时刻、临期窗口、映射阈值/受影响组织锚点/负责人集合和必要静默例外必须在B08收据填写，不能把空值猜成某公司习惯；已经确认的高级别上溯抄送不在是否建设的待决范围。已有人工催办独立遵守60s同单同人节流，不把自动每日抑止套给人工。未决可完成事件/解析/抑止/投递合同和合成验收，生产首规则不启用。

通知深链也要当前资源鉴权，正文最小化；无权已过期 intent 记 suppressed，真实不可用记 failed/retry，不能静默消失。风险级别平台四档不复制源五档通知词表，映射产物有 revision。通知指标区分待解析/抑止/入站/已发/失败/读回执，不以 inbox 插入成功证明企业 IM 真实送达。

## 10. 仅需 owner 选择的业务策略

以下不是已批准实现。默认是供拍板的推荐，替代与后果并列；独立技术细化、mock/contract样例和无破坏读逻辑可先完成，涉及可见集/授予/交接的写开关在决策前保持关闭。已确认的专业供给存在、mock 保留、模型仅 API、risk:manage 接受、N:1 和快照不在此重问。

| ID / 决策截止 | 推荐默认 | 可选方案 | 可观察后果 / 不决时的安全边界 |
|---|---|---|---|
| B01 · 已确认，D03实施 | 存量类型回填 `inherit` 保持当前可见集；新类型默认 participants_only，明确提示管理员；收紧旧类型单独预览确认 | 所有旧类型统一 participants_only；或新旧都 inherit | 统一收紧会立即影响老单/委托/报表；都 inherit 较宽。已确认新默认/旧兼容，收紧旧类型单独预览 |
| B02 · 已确认，D03实施 | CC 在受限类型中仍可见/评论，普通 participant 同样只读+评论；不附送写权 | CC 不突破受限策略，仅显式 participant 可见 | 后者使“抄送”与“可打开”分离，风险owner桥需显式 participant。已确认CC读/评论方向，R3 owner通路仍按具名领域桥和授予合同落地 |
| B03 · 已确认，D03实施 | `ticket_scope=all` 不突破 participants_only/role_restricted；global admin既有例外保留 | 将 scope=all 设为全类型通用可见 | 已确认普通all-scope不穿透；已有inherit行为保持，系统级管理员既有例外保留 |
| B04 · 已确认，D03实施 | `ticket:participant:manage` 按钮授给获准业务管理者；creator 不因创建身份自动获邀请权；领域桥可按具名合同同步受管 owner | creator 可邀请其候选范围内人员；或只有全局管理员可管 | 已确认独立授权的业务管理者管理，普通creator不自动获得邀请权；收件人范围和域受管项保护仍需校验，既有CC写也受该政策约束 |
| B05 · D03/P6.2 派单启用前 | 复用risk:manage与既有单条/批量范围；不另设派单角色，不额外要求通用ticket:create；受限领域用例仅向已配置整改类型/组织建单，复用工单校验与真实操作者审计 | 风险owner还必须单独具备通用ticket:create才能派单 | 用户询问必要性和业界后，已提出简化候选、尚待确认；未决不启扩大领域创建范围，不假扮owner或开放任意类型/组织 |
| B06 · D06 离职业务自动处置启用前 | 安全禁用即时执行；工作项进可见交接清单，有已批准继任映射才自动转，其余保持状态+待交接 | 全部回池；全部指定上级；全部人工逐项转 | 回池丢当前处理节奏，上级不一定合格；人工延迟较高。未决不自动回退业务状态/猜继任人，但绝不阻塞禁用和告警 |
| B07 · D06 撤部 apply 启用前 | 虚拟组上挂最近合法存续实体祖先但预览权限变化；在途业务显式接管；历史保留原组织展示/独立历史分组，不默默并入新部门统计 | 全部保留旧锚待人工；或将全部在途与历史一并转新部门 | 自动上挂可能扩大祖先委托，整体转历史会改报表；未决只禁新建并保留节点/历史ID，提供plan，不做破坏性撤部推广 |
| B08 · D15 首条生产风险通知启用前 | 保留已确认的高级别上溯抄送；冻结源五级到平台四档阈值、组织锚点/负责人、汇总时区/时刻、临期窗与静默例外 | 频次按四档候选、全即时或分层汇总；不是重问要不要负责人抄送 | 未决先交合同/mock，首条生产规则不启用；无合法负责人有显式待办，不猜人/广播/静默少发 |

P4“角色 code vs ID”和 P5“BK-21 顺序”已在 §5 给出技术推荐，无需 owner 重拍业务；实际角色授予名单和生产目标组织应在 B04/B05 收据中明确，不能以示例角色名当授权。长度单位、指纹族/代际、DTO兼容和锁序是技术设计，仍需实现评审，但不冒充用户已经逐字段批准。

## 11. 实施依赖、迁移与验收收据

### 11.1 交付分界

- D01 依赖 C01/C04；D04 与其同波合同协作，不能出现“先部署实例钉版，G2仍查 live 字段”。D02 依赖 D01+D04，R3的 user 字段/受管字段/field资格先完成，不等待完整设计器和 D16。
- D03 复用 D02统一动作谓词和 C04过滤器；B01–B05决定时不能只做菜单隐藏，需列表/单票/搜索/导出/计数同源反例。
- D06 可独立先交 mock 目录端口/最小导入与manager/负责人解析。full reconcile/撤部自动推广、真实认证分页、生产同步定时分别验；复杂组织自动化不得成为 risk核心必须前置。D08输入和D15首通知只依赖可重放的最小目录解析，不假定真实 HR 全量同步已交。
- D07 查询和 D08 来源 adapter共用稳定公共合同，提供方未知字段不进入核心类型；D09接受/责任交接、D10桥先消费确定性结论。R01可靠intent是跨域和通知恢复前置；D15在首条风险通知之前，不等D11完整编辑器。
- P6.2 领域完成用例先于 P6.3 D16通用自动路由。长期计划任务、后台代人、MCP副作用和Agent写动作须等I02委托与持续授权，不借本文的交互ticket.create开启长期grant。外部Agent与平台有界领域Agent均只能调用同一受权用例，模型不确定输出不绕开发布/精确确认/幂等。

### 11.2 迁移与恢复边界

实施前核实际迁移目录和控制台编号，全部 up/down 成对；此文不给占位编号。先加可兼容列/映射和幂等backfill，验证后收紧约束；旧源码/数据库回滚组合依既有 B案耦合规定，不能只 down DB。新增 published definitions、revision、source identity、risk family/cycle、接受申请/link/event/intents 的批次在实施任务中列清兼容代码最低版本。

需要恢复的四类情况分别演练：deploy响应丢失按key取收据；迁版部分成功按每单receipt重试；HR分阶段partial保留上一成功baseline和虚拟组；风险票终态消费崩溃靠intent/reconciler补齐。业务补偿必须是新命令/新历史，不删已发生事件伪装回滚。过期计划、权限撤销、provider键冲突和新revision都要求重新预览，不能为了重试成功绕过CAS。

### 11.3 最小验证矩阵

| 证据组 | 必须能识别的反例 | 正例/收据 |
|---|---|---|
| 定义/兼容 | 同时改字段/图CAS冲突；deploy预览后引用变化；旧单掉到最新字段；缺定义仍可写；重复deploy空增版；新增必填破坏已登记生产者 | 新单用新、旧单固定旧；模板/可见性仍live；hash等价重放；版本引用禁删；迁版逐单报告 |
| 字段/采集 | 处理人直接update未授权字段；patch把risk_owner改自己；函数同名值位绕过；伪造旧formula；dict失败当不限；隐藏字段漏required或读权 | 合法普通字段/函数；转换字段+状态同事务；emoji长度一致；空options往返；旧列宽不批量改写 |
| 鉴权/创建 | all-scope跨出受限policy；普通人枚举why-not-visible；CC混role；role读权限变创建权；伪造created_by；capabilities后撤权仍成功 | 列表/详情/导出/计数/首页同可见集；现有REST数组兼容；领域owner按批准路线读与复核 |
| HR/组织 | partial snapshot离职；external_id重用串号；乱序墓碑回退；HR active解本地封禁；撤部漏工单path；虚拟组/角色被对账清空 | 分页/批收据；manager延迟补解；禁用与会话恢复；交接逐项成功/失败；移树可见集回归 |
| 风险/来源 | 重推覆盖处置；同事件重试加计数；迟到旧观察造回归；并发两代；源删除当修复；accepted到期与续期竞争 | family当前cycle唯一；修复真回归新编号回链；接受有效期计数；到期原单重审 |
| N:1/恢复 | 第21风险挤同票；只转一risk把全票审核权交新人；ticketclosed一刀切fixed；consumer失败无追踪；旧结果覆盖改派后新状态 | 每risk唯一active；同owner同范围分组；双域审计；默认off完成/开启复核两条；逐项结果/重放/冲突收据 |
| 通知/扩展 | owner缺失广播all；离职旧owner收详情；event签名当读许可；私有connector直写库；provider失败切mock绿 | 首规则装配/词表/owner四档矩阵；受权Query；旧protocol消费者通过新增字段合同；mock/contract/real分层报告 |

实现交付必须提供准确五仓候选SHA/协议版本、迁移前后数据核对、相应单元/集成/跨仓合同/UI旅程及四档 `make acceptance` 结果；未跑不能标实现完成。文档本次只做内容/链接/差异检查，不把旧基线测试当这些新能力的证明。没有真实provider环境时保留明确责任人与总账决策窗口，mock/contract可独立关账，real不得代销。

## 12. 本文自审与仍未知道的事实

已逐项覆盖：定义/业务/协议/实例版本分离；旧REST和旧定义兼容；三层授权、field资格和受管字段；B01–B08业务选择；HR完整/增量/墓碑/manager/离职/撤部；风险稳定键/乱序/周期/接受/N:1/逐项结论；可靠intent/通知/补偿；私有adapter隔离和长期mock。本文不引入新业务代码、迁移、依赖、生产角色授予或外部消息发送。

真实未知仍包括：各HR/source提供方的稳定ID和身份重用语义、排序/分页/删除能力、真实目录负责人字段与组织深度规则；CMDB owner权威/更新频率；专业复测结论可供字段/权限/证据期限；企业消息通道和外部Query服务的主体授权。已确认“供给存在”不等于这些细节已核验。拿不到契约时用合成正反例和能力协商显式降级/阻断对应真实能力，不能再问一次有没有专业平台，也不能将私有接口细节写入公共仓。

## 13. E10：人工迁移导入与跨指纹合并的有限处置

三项旧挂账不是D05配置包导入、U03批量标签、同指纹去重或N:1整改关联的同义词。E10在**P6.1出口**分别给出范围/供给/价值裁决与合成样本，获准的有限增量在**P6.E**按独立子估算/子窗口验收；没有真实存量迁移或同事件多指纹场景时，可以明确退出本轮候选并说明重开所需证据，不能继续写“首诉求触发”而没有本次结论。该调查不依赖D08/D10已实施，后续增量写入才依赖领域合同。

| 子项 | 候选最小形态与合同 | 不能代销的验收/退出证据 |
|---|---|---|
| 风险Excel导入 | 固定模板下载、字段/来源命名空间/映射版本校验、逐行计划/错误报告；优先私有一次性迁移工具转标准feed，使用稳定record身份与核心提交回执 | 模板/必填/非法/重复/乱序/部分失败/重启回放与完整快照标记。只交API不算Excel和报告完成；不需要时明确退出工具，而非称API已替代需求 |
| 工单业务批量导入 | 固定类型发布版本和允许字段，通过工单用例逐行鉴权与提交；creator/owner不可自报提权，附件须合法对象引用；批次/行稳定幂等，逐项receipt | 权限变化、旧类型/schema、非法责任人/附件、同批重放、部分成功恢复；失败不重复已提交行。优先有限迁移CLI或受控导入页，不建通用任意表导入产品；不纳入时给退出理由 |
| 跨指纹人工merge | 主锚管理者预览主从风险及来源、状态/接受/活动links/owner冲突，以revision/hash确认；保留主从ID和merged_into回链、两侧审计与来源后续归属 | 不同owner/冲突接受/活动整改单不自动吞并；先给冲突处理方案，不能因merge提升读写权。合并后统计与回链可核；源再报归属明确，重复确认幂等。补偿是新命令，不删历史假装未发生 |

E10的2–4工作日仅计逐项调查、供给/字段证据与有限合成验证，不能声称足够实现三套产品。获准自建时交独立子包估算、准确API/schema/迁移、逐行/主从权限及恢复反例；若无法在P6.E交，必须在P6.1裁决时重排具体波次/依赖并记录影响，不在实施后才发现无限挂账。上述均未实现，不增加风险核心D08–D10的共同硬前置。
