# Phase6 底座、三层权限与身份接缝详细设计

日期：2026-11-14。状态：**模块详细设计候选，未实现**。范围为 G01–G04/G06、C01/C02/C04/C05、D06、E01-UAA-C 及统一审计在本期的适用部分。G07A/B 已合主线并有[新版主线复验](../review/reports/main-verification-2026-11-14.md)，不重新建设或重复计价；正式运行前仍验实际制品和数据恢复。本页将[模块交接](module-design-handoff.md)的第一包展开，业务范围、净估与远期出口继续归[首版计划](first-usable-release-plan.md)、[总账](../review/planning-backlog.json)。

本页所有新类型、逻辑表和端口均为实施目标；**不是新增现成路由、迁移号或已完成验收**。ID 对外用十进制字符串，时间用带时区 RFC3339；枚举拒绝未知控制值，空集、缺失和 null 不相互替代。既有 errcode/HTTP envelope 继续使用，稳定 reason 对应数值码在实现时登记。

> **当前第一包入口**：[范围、两项已确认政策与A/B/C子估](iam-first-batch-design.md)。本页是较宽底座候选，不把所有内容一次派入A；当前不开发。

## 1. 当前能力与本次补强

当前 `internal/middleware/jwt.go` 的 `JWT` 已检查 Redis blacklist/disabled/密码纪元且依赖故障拒绝；`internal/service/auth_service.go` 的 `Login`、`Refresh` 与 PAT 仍是本地认证链。`UserService.UpdateStatus` 已有系统号/最后 superadmin 保护及禁用后的会话撤销；`RBACService.AssignMenus` 已事务写权限并在提交后重载 Casbin。`reloadPolicy` 失败会报告错误，但尚不是持久、多实例生效证明。`GetEffectiveRoleCodes` 合并直接、组织、继承角色，不能只处理 user_roles 就声称撤权完成。

本次补的是共同当前授权守卫、提交后恢复与来源边界，不替换已有鉴权引擎，不把“统一调用”做成所有内部函数绕 HTTP。HTTP、内部用例、worker 和以后 MCP 均构造可信 Context，再调用同一应用用例；handler 与代理适配传输，领域 service 负责授权和效果，repository 只执行本用例的事务。

## 2. 首版冻结的端口与返回语义

```text
TrustedContext = {principal_id: ID, principal_kind: user|service,
  authenticated_service_id: string?, operator_user_id: ID?, request_id: string,
  correlation_id: string, delegation_ref: string?, authn_source: local|federated}
ResourceRef = {owner: string, resource_type: string, resource_id: ID}
AuthorizeQuery = {capability_id: string, contract_version: "1", resource: ResourceRef,
  action: string, requested_fields: string[]}
AuthorizationDecision = {allowed: bool, policy_revision: UInt64String,
  reason: string, allowed_fields: string[], filter_ref: string?}
ApplyIAMChange = {target_id: ID, expected_revision: UInt64String,
  command: SetRoles|AssignMenus|UpdateStatus, idempotency_key: string}
IAMReceipt = {operation_id: string, domain_revision: UInt64String,
  policy_revision: UInt64String, state: committed_pending_effect|effective|held,
  pending_effects: (policy_reload|session_revoke)[], reason: string?}
```

Context 由可信认证层构造，body 的 user_id/operator/role 不替代它；`AuthorizationDecision` 只描述一次决定，不能当作长期 bearer token。新写用例要求 expected_revision；旧 REST 端点保留现行输入，由锁内业务谓词防交错，不能冒称已阻止旧客户端陈旧编辑。旧wire不静默换成IAMReceipt，成功data:null保留，新增响应头关联operation，准确兼容见§3.9；状态查询才返回receipt。提交后重载失败可关联原操作，不能报成业务必然未提交；不知道原结果先查原操作，不重复授予/撤销。

L1 为当前页面/按钮/API 功能；L2 为该主体可访问的资源行及字段；L3 为此动作、对象状态、职责和字段约束。列表、详情、计数、聚合、导出、历史、附件、任务结果都用同一资源过滤器和字段投影；单对象不可见与不存在同形404，列表/计数/聚合先过滤无权行，空集正常返回，不泄露隐藏数量或因有隐藏行将整页改404。已可见但动作禁止为403。依赖不可证明为503，revision/同号异参为409。不能因没有菜单而拒绝安全首页，也不能由“有首页”增加业务读权。

## 3. C04：共同授权守卫与多实例

首版采用**一个数据库授权守卫行**，而非分布式锁服务：候选 `authorization_control(id=1, revision bigint NOT NULL CHECK revision>=0)`。它承载短事务撤权屏障，`revision`仅表示需要同步的**L1 Casbin策略代际**，不是所有资源变化的全局版本。普通业务共享锁可并行，授权变化排他；后续观测锁等待后再按资源/组织分片，不在首版暗做全套权限平台。

1. 事务开始前按动作选择锁模式：普通受权读/写取该行 `FOR SHARE`；会改变有效权限的IAM、组织任职/继承/数据范围、类型查看策略、参与/CC管理取`FOR UPDATE`。Assign/Transfer或风险owner交接若改变既有职责/可见集，也按改权动作选择排他模式，不能因来源改为派生事实而漏守卫。**禁止先共享后升级**。批量拆有界事务，但每批独立复验。
2. 取得守卫后读取L1 revision；每进程使用不可变的 `(enforcer, applied_revision)` 对。只有 applied_revision 与 DB revision 相等，才可使用该策略实例；否则先结束本次事务、触发重载并返回可恢复的 `POLICY_NOT_READY`，不得在持锁期间请求网络或把旧策略当准。`AuthorizationDecision.policy_revision`仅标L1视图，不能证明L2/L3已判断。
3. L1走该revision的统一PDP，当前有效角色按直接/组织/继承来源重新求值，JWT中的旧角色不是授权真值；L2/L3从同一短事务的当前账号、组织、scope、参与及业务状态求值。写主锁业务行、核expected_revision，再写效果、领域历史、必要intent。守卫保持到commit，避免“撤权已提交而旧允许决定随后仍提交”。外部调用只在提交后做。
4. 改权事务在排他守卫下复验操作者资格，保留各命令实际具有的最后superadmin/系统号保护；更新适用直接/组织/继承来源。**只有实际改变Casbin p策略内容**推进L1 revision并写policy_reload intent。当前`configs/casbin_model.conf`无g，直接角色、组织成员/org_roles和role parent/status由锁内fresh集合计算，改变这些关系本身不要求全站LoadPolicy；参与/CC、类型查看范围也只改相应资源/策略revision。会话安全变化另写适用session_revoke intent。提交释放锁后才做对应恢复，不得为省重载继续信旧角色或L2缓存。
5. loader 使用**只读 REPEATABLE READ 事务**在同一数据库快照读取全部策略与 revision，校验后原子替换整个 pair；不能用多条 READ COMMITTED 查询拼出旧策略+新 revision。加载中又有新代际则允许安装旧 pair，但后续请求比较不等仍拒绝并再次加载。不能先置新 revision 再慢慢改原 enforcer。启动没有可证 pair 时受保护接口未就绪，健康存活与业务就绪分开。

固定锁序为授权守卫 → 来源/业务 group → 风险 ID → 工单 ID → 其它关联 ID（各集合排序），遵[生命周期合同](../design/business-lifecycle-contracts.md)。无反向先业务后 IAM，禁止持业务锁 HTTP。PG 死锁/序列化故障仅在未产生外部效果且相同 operation 下有界重试；业务冲突交调用者刷新。事务使用 READ COMMITTED 并在取锁后读当前事实，不依赖事务开始前缓存。

本设计是采用 [PostgreSQL 行锁及一致锁序](https://www.postgresql.org/docs/18/explicit-locking.html)的项目选择。借鉴 [OPA 状态模型](https://www.openpolicyagent.org/docs/management-status)区分“策略接收/存储”和“激活”，**继续使用现有 Casbin**；不引入 OPA 服务或以心跳代替代际检查。所有启用入口（含 PAT、worker、代理、批量和查询）接该公共守卫是开通前置；遗漏入口不得凭 L1 中间件已存在放行。

策略副本同步与本地资源事实分别处理也符合[Casbin watcher职责](https://casbin.apache.org/docs/watchers/)；这里选择数据库代际检查，不因此引入新的watcher组件。短期ref采用[Redis到期键](https://redis.io/docs/latest/commands/expire/)只承担可丢的请求上下文，绝不替代持久效果凭证。

### 3.1 逻辑数据与恢复所有者

| 候选实体 | 约束/索引 | 写主与用途 |
| --- | --- | --- |
| authorization_control | 单行主键；L1策略revision单调 | 共同事务屏障；L2/L3变化不推进Casbin代际；不可被私有 adapter 写 |
| capability_operations中的IAM记录 | 沿共同operation唯一身份/输入hash/主体绑定；增加capability+target+domain_revision查询索引 | 两IAM命令复用同一逻辑operation store，不另建iam_change_receipts；IAMReceipt是所需状态的领域视图 |
| authorization_effect_intents | UNIQUE(operation_id, effect_kind, target_id, desired_revision)；(state,next_attempt_at)索引；lease_owner/until/fence | 复用 R01 worker 机制；一个 effect 一个重试主，过期 fence 不更新新执行 |
| identity_source_bindings | UNIQUE(source_id,entity_kind,external_id,identity_generation)；本地 ID 索引 | 启用实际D06目录feed时建设所需映射；issuer+subject映射在UAA-P增补。人工local账号/风险不以完整HR映射表为前置 |
| governance_ownership | UNIQUE(application,resource_key,scope_key)；revision/source/allowed_write_port | 完整受管投影表在P/R建设；本地先以既有来源字段和受审配置登记local写主，不先建全UAA治理库 |

表名是逻辑建议，实施前与已有 job submission/审计表对表复用，不复制第二套 outbox；迁移 up/down 成对、编号当天从迁移地图领取。初始代际为 0，对现有本地来源显式回填；回滚新增约束前停止新入口并处理在途 intent，不能删除正式历史以求 down 通过。

### 3.2 DBTX 试点、组织删除和事务预算

C04 原批准试点继续仅覆盖 `ticket.Service.Create/Update/Transition` 及其直接 repo 邻域：主仓内部 `DBTX={Query,QueryRow,Exec}`，`WithinTx(ctx,pool,fn(DBTX))` 由 service 开始、提交和回滚，repo 显式收同一 tx；不放任意 context、不隐式嵌套、不在 tx 内再从 pool 借连接。所需 schema/业务读也用该 tx，网络取数/HEAD/模型在外，锁内重新验可变引用。commit 返回错误先按 operation 查询 receipt，不整体盲重试或把“未知”报成“无效果”。DBTX 不下沉到 utils 的业务接口，不全仓重构。

`OrgRepo.Delete` 现有组织行锁与子组织/成员/未结工单检查保留，**增加 org_roles 存在检查并返回稳定冲突+受权计数**，要求显式解绑再删，不暗删授权。`BindOrgRole`、任职/相关组织依赖写均在授权守卫之后通过同一个 `lockAliveOrgForShareTx` 校验并锁存活组织；删除取该组织 `FOR UPDATE`，删除/绑定双方同序，避免“删前没绑定、提交时刚绑入”。虚拟组 `DeleteVgWithOwnerCleanup` 单独按同一来源/owner 清理政策验；历史软删孤儿只读盘点后受审清理，不自动恢复有效授权。

数据库连接池 max/min/lifetime、请求总 deadline、事务/statement/lock timeout 使用有界配置；初值以现有部署及隔离压测为依据，记录排队/持锁耗时，不凭空承诺生产 QPS。pool 上限为1的试点回归必须仍可完成，用来检出同 tx 内偷借第二连接；等待池或锁超预算在未提交时明确503/可恢复冲突，先回滚再重试。禁止长导出/HTTP等占授权守卫，分页导出每块重验当前权限并可中断。

### 3.3 现状缺陷先修与真实重读（2026-11-15专项后继）

[权限专项](../review/reports/authorization-design-implementation-review-2026-11-15.md)确认的 AUTH-01/03/07 在当前 main 仍开放，不等真实 UAA/HR 才处理。C04/D06 先补这些最小修复，不能以本页已有方向或普通门禁绿替代产品反例：

- 组织 `status=0` 停止该来源的 L1角色、菜单和 L2 scope/委托；其它直接/组织/参与来源仍独立有效。group/all、透明读、本组和祖先owner路径一并验。BK-20软删组织的已结票档案例外是另一政策，保留登记，不能顺手批量改变。
- `RoleRepo.ListRoleIDsByUserID` 与 `GetEffectiveRoleCodes` 使用同一有效角色语义，启用子→停用父不再下发父菜单/按钮；接口403与菜单消失分别断言。用户管理用直接角色判断可授档位的独立语义不随展示修复改变。
- 组织任职管理不等于IAM赋角权。`BindOrgRole/UnbindOrgRole` 不得复用任意`org:%`判定；首个小修候选以现行内置系统级管理者为界，所有获准主体（含admin）均按直接角色复用现有canAssignRole可授档位、目标状态并锁内复验；不能只限admin却仍容许其授更强custom角色。解绑另验可管理目标；未来委托赋角也不放宽此界。只授org:create的普通角色不能把更强custom角色绑到自己所在组织；系统角色禁绑保护仍保留。其它全局组织动作逐项映射现有功能码，owner/admin本组织职责与全局权限分开；是否保留额外全局委托角色在本包实施前确认，不延迟提权修复。
- `RBACService.GetRoleCodesByUserID` 会命中请求context缓存。守卫后使用显式fresh/DBTX读取端口，在同事务查当前直接/组织/继承角色和账号/组织状态，不能复用守卫前`RolesFromContext`结果；普通同请求展示缓存可保留。

小修也需后端真实路由及三源撤权反例，首版正式使用前完成；C04后续守卫试点仍按§3.2渐进，不因发现具体SQL缺陷扩大成全仓授权平台。服务端归因字段由可信context产生，浏览器不能在任务/消息新建中自填created_by；具体旧任务动作维护边界见[任务详设§8.2](task-activelist-message-detail.md#82-存量任务入口与维护动作的接线门)。

### 3.4 授权编辑的来源与往返（第二轮现状复验）

[第二轮复验](../review/reports/permission-boundary-recheck-2026-11-15.md)新增RECHECK-01/02/03，当前产品仍未修。必须区分三种查询：**直接绑定编辑值、有效三源角色/菜单、用于IAM管理档位的有效直接角色**。`GetUserRoleIDs`作为直接授予编辑出口不能复用effective展开；原样保存不把组织/继承角色固化direct，也不丢弃尚有绑定但停用的角色。停用同ID旧绑定可保留而不授当前权；解除仍核目标/角色管理资格，无权移除则整笔拒绝，不自动并回旧项。新增必须启用并守原可授档位；`canAssignRole`允许等档，`canManageTarget`管理他人才要求严格更强，不能混用。准确wire、增保删及测试见[第一包§3](iam-first-batch-design.md#3-a批的具体前后与验收)。组织/继承及未来UAA受管来源不转为local写；A先给直接编辑说明，不凭现有API编造全来源列表，不要求先建来源管理台。

`SetUserOrgsTx`的集合替换改为同事务差量：保留相同org的角色、scope、来源、时效及joined_at，is_primary仍按既有primary_org_id输入语义更新，不借差量修改缺省/null含义；新增按默认、移除按已准写主及现有owner双轨清理。旧Phase1 DELETE重插实现目前会把admin/group重置member/assigned，不能将仅修改组织ID集合等同重设全部任职属性；具体角色/scope仍走原专用命令。

用户角色/组织等整体替换编辑器必须先绑定当前user_id＋请求世代的成功快照。打开/关闭递增世代清草稿；pending/error、成功载荷缺字段或已绑ID无法正确回填时，按钮与submit一起拒绝并给重试，不能用空数组/前一用户的选择替代失败。首个await前锁提交并固定payload，迟到GET/POST均不操作新目标；组织树渲染后再回填。成功加载后的显式清空仍须每项解除合法。上述是C04及薄UI/U06的当前数据正确性修复，不等全站视觉改造或真实UAA，也不改变旧服务端缺省/null解析。

### 3.5 层级组合的首版出口与分工

按[层级场景合同§8](../design/rbac-inheritance-and-cascade.md#8-层级关系场景合同与实施归属2026-11-15候选)的HIER-01–12随包实施：C04处理三源/单父角色链、组织树合法性和Move/撤权、菜单导航与功能码分离、直接编辑视图；D03处理scope与类型/参与交集；D06处理多任职/来源及组织生命周期。组内任职不等于平台角色，整组绑定管理能力不能被个别viewer抵消；领域仍核具体目标职责。

首版本地S1先交实际启用规则及薄IAM正确性，D03/D06随其业务批交适用场景，不等完整U06/HR/UAA；HIER-07/08/11复用FND-03/14–16/19–21及既有AUTH/RECHECK反例，不重复建设。HIER-D01已确认整子树组织派生资格暂停、独立来源分别求值，独立切片继续推进。§8.4的阶段/净估边界和当前未实现状态同步总账，未将完整树编辑器或来源管理台塞入S1。

### 3.6 模型复用与引擎选择边界

按[模型复评](../design/authorization-model-and-engine-choice.md)，本期继续Casbin＋领域Resource/PG范围，不将L1/L2/L3误作与RBAC/ABAC/ReBAC竞争的模型。复用现有机制，先统一可信输入、直接/有效来源、对象/列表语义及共同用例检查；菜单/路由是稳定能力的执行/展示映射，内部与worker不能只靠HTTP中间件授权。业务状态和证据不变量仍在领域执行。

PBAC的策略可审/可测/可解释原则可随既有C04/CE接线采用，不新增通用DSL、规则分发/版本库或外置PDP。后续重复规则/独立治理或跨对象关系出现时，可按模型复评做Cerbos/OPA/Cedar或OpenFGA/SpiceDB的单资源对照；须另定范围/净估，不暗含进C04。当前缺陷、HIER-D01政策已定、待实施及三层授权新鲜度要求均保留。

### 3.7 差距补齐的最小验收组织

执行[AUTH-GAP-01–07](../design/authorization-model-and-engine-choice.md#8-差距补齐动作与实施计划所有者要求落档)，不重建引擎。C04主责共同事实/守卫，D03和各域主责资源/字段策略，D06主责目录来源，G04负责风险选择/回归口径；CE01登记实际入口，CE02逐批接线。先交当前必需正确性、共同路径及可验证性，再按实际拓扑和复评裁决演进。

- **独立预期索引**：复用FND/HIER/AUTH/RECHECK和领域用例ID，以当前测试夹具/数据表记录主体及有效来源、对象/动作、预期允许/拒绝、适用出口、实际测试符号；预期不调用被测授权函数产生。不预写所有条件的笛卡尔积、不新造策略DSL。HIER-D01已确认、产品仍未实现、BK-20例外另列，不以夹具偷定业务政策。
- **成对与跨入口回归**：同一有限数据集对详情与实际List/Count/Search/Export/关联/字段投影作独立断言，含第二页及来源/状态变化。HTTP、内部和已用worker等分别验证同一资格；移除关键守卫会失败的反例须真实有效。沿原TDD先红后绿、并发反例至少8轮纪律；规则/入口变更在PR列影响的既有场景ID，不新增恒绿检查。
- **有限解释**：复用当前`AuthorizationDecision.reason`候选、安全错误信封、request/operation引用、L1代际和适用资源revision。无功能资格、可见对象动作禁止、策略未就绪可定位；不可见与不存在仍同形，不能给任意ID提供“为何无权”的成员/字段探测器。受权IAM编辑面分别展示直接编辑值与组织/继承/受管来源；不做递归关系图解释台。
- **规模与恢复证据**：按实际部署画像记录AUTH-GAP-05基线，开工定预算，正式前测；性能不足再给有界优化，不提前分片/缓存。单实例先验重载/撤权失败恢复；P6.3列所选拓扑缺口，E09和适用P/R在实际双副本启用前交AUTH-GAP-06证据。引擎必要性按AUTH-GAP-07窗口裁决，非本地正式前置。

上述是原包细化的出口，不是已实现功能。子估需与S1/各域/薄UI/G04/E09扣重；Q1只整合非重复接缝。新增通用解释器、策略控制面、任意图或实际引擎PoC均不在此价内，获选时先补净估再排；当前不增加父包或承诺原估足够。

### 3.8 B：有效组织来源与成员集合的准确接线

只读基线`610c1d76`。HIER-D01已确认；本段将B的4–7日候选子估细化为来源谓词和保存算法，不新加组织API或预领迁移号，不将C并发闭环冒充B已完成。

**有效性顺序**：对某个组织来源先验证本节点至祖先链无停用，再判断该成员`expires_at IS NULL OR expires_at>当前时间`；等于当前时间已过期。然后求最近实体anchor及group/all等范围贡献。不能只在anchor选择中滤掉停用实体再向上找，否则会跳到更高实体而恢复本应暂停的透明权限。is_primary是展示/归属信息，不决定另一有效成员来源是否贡献。

角色两查询采用相同有效种子与启用、未删的单父闭包；中间父停用截断该路径，不能略过停父继续拿祖父。独立direct、其它有效组织、单独直接绑定的祖父等路径分别求并集。停用组织不增加对该用户的一刀切deny；创建人/处理人/CC仍按本域L1/L2/L3检查。组织有效时显式owner原本独立于成员期限的语义不改，但该owner的组织派生委托受本组织/祖先停用约束。

| 真实消费者 | B接线要求 |
|---|---|
| `internal/repository/role_repo.go`的`GetEffectiveRoleCodes/ListRoleIDsByUserID` | 共用组织种子有效性及父角色停止规则，菜单/API与有效角色一致；直接编辑查询另轨 |
| `internal/service/org_delegation.go`的`HasOrgManagePermission` | 内部另有三源递归SQL，须同步，不能让组织管理fallback继续拿停组grant；A的授予资格收紧不由此代销 |
| `internal/service/ticket/scope_resolver.go`的`pgxScopeResolver.resolve` | 源有效后再合并透明anchor/group/all；撤一路保留其它路，不因停组fallback更大范围 |
| 同org_delegation.go的`EffectiveOrgPriority/IsOrgAdminOrOwner/IsAncestorOwner`，`internal/service/ticket/resource.go`的`delegatedVisibilitySQL` | 单对象与列表/total/导出同规则；不能只修Authorize而漏集合SQL |
| `internal/repository/org_repo.go`的`IsMember/IsInOrgBranch`及实际分派成员检查 | 凡用于授予业务资格的成员查询使用相同有效来源；管理名册/绑定编辑GET仍回显停用/过期关系，不能为鉴权修复把它们隐藏 |

**BK-20独立画像**：已接受的软删组织已结票委托档案读写不在本轮收紧范围。现三处委托SQL未筛deleted_at有明确历史出处，不能顺手全加过滤。停用的status条件与软删档案例外分开表达；例外不给软删组织L1 grant或新建资格，角色组织种子仍须存活。测试必须同时包含live来源、停用来源、已结档案和独立参与对照。

**成员差量事务**沿`internal/service/user_service.go`的`UserService.SetUserOrgs`原ensureCanManage、`internal/service/org_service.go`的`SetUserOrgs/SetUserOrgsTx`，进入`internal/repository/org_repo.go`的同名方法；新建用户的初始组织也复用Tx入口：

1. 先去重请求org_ids，验证primary非nil时属于请求集合；外层主体/目标管理边界不改。同tx读原始user_orgs全部字段，不能依赖GET只投影的user_id/org_id/is_primary/joined_at计算属性。
2. retained保留org_member_role、ticket_scope、source、expires_at、joined_at；即使成员已过期也不重插续命。只按明确原输入语义更新primary，不由此更新专用任职字段。
3. added用原DB默认member/assigned/local/NULL期限/当前joined_at，不复制其它组织admin/group/owner。目标组织须未软删；status=0仍允许既有受权预配置，但不贡献当前授权。
4. removed删除合法移除的成员行；同tx保留原owner双轨清理：用户出现在**任何不在本次去重requested org_ids中的组织**的owner_user_ids都移除，不能只扫描旧成员集合，也不能把requested误作old∩requested而清掉新增组织已有的owner-only引用。请求保留组织不降owner/admin为member。
5. primary先清旧true，再设置唯一请求目标，保留`idx_user_orgs_single_primary`。**缺省和JSON null均为nil，沿现行为清空所有primary**；不能改成“保持旧primary”。非法primary整笔拒绝。
6. 按字段写主矩阵检查实际受管来源，不能由local集合覆盖暗删/改受管字段；保留非local行不重写其属性。来源尚无权威绑定的历史异常先hold受影响变化并登记受审处置，不从source字面猜管理员可绕过。
7. 任一步失败，成员、primary和owner数组一并回滚；相同合法集合重复保存不改joined_at/期限。旧软删孤儿等异常不借正常保存做隐式清理，先受权盘点处置。

继续现有wire：`GET /api/v1/users/:id/orgs`及`POST /api/v1/users/orgs`；组织状态仍经`POST /api/v1/orgs/update`的`id/version/name`及可选status，缺省保持现值；成员、role、scope、owners用原专用端点。当前无通用source/expires_at编辑API，不为B另开。UI成功快照已归A；不能靠回传完整隐含任职属性补救服务端DELETE重插。

锁协议归C：共同守卫之后统一组织/成员锁序，并覆盖Add/RemoveMember、SetOwners、角色/scope、Move/状态/删除和创建用户Tx入口。只锁旧成员行不能拦并发新成员phantom，也不能把串行提交说成防陈旧表单覆盖；旧wire没有expected_revision，此限制要保留。Move既有串行锁、存活组织FOR SHARE与owner写锁不能互换顺序产生反向等待。

**待新增独立场景**：B-01父停子启跨两有效查询/菜单/API/scope/委托暂停；B-02直接/其它组/参与来源及撤all保group；B-03最近实体停用不fallback更高anchor、虚拟透明/隔离对照；B-04停中间父角色断链但其它路径保留；B-05恢复按当前绑定/期限，已撤过期不复活、显式owner期限独立；B-06同集合保存全部属性不丢；B-07增保删/重复/空集及primary缺省/null/指定/非法；B-08owner双轨与故障零半套；B-09列表/详情/total/导出同源且BK-20对照；B-10用户Create原子绑定及C集成后的成员/owner/Move/停用交错。B-10并发成功证据须等C，不由本轮静态核对代替。

复用以下真实测试，现绿色不证明B新反例已验；真实非local写主/历史异常批准沿原Q07/Q12窗口绑定，不再重问HIER-D01：

- `internal/service/ticket/b2_org_integration_test.go`：`TestB2Org_BFSRoleExpansion`、`TestB2Org_ParentIdEdgeCases`、`TestB2Org_ScopeSupervisorAndAll`、`TestB2Org_ExpiredMemberLosesAnchor`。
- `internal/service/expired_membership_integration_test.go`：`TestExpiredMembership_AuthorizationBoundaries`、`TestExpiredMembership_ExplicitOwnerRemainsIndependent`。
- `internal/service/ticket/delegation_authorize_integration_test.go`：`TestD9_D7D8_OrgAdminOperatesTicket`、`TestD9_D9_AncestorOwnerSubtree`、`TestD9_AncestorOwnerAfterMove`、`TestD9_CreateOrgMembershipGuard`。
- `internal/repository/org_repo_integration_test.go`：`TestOrgRepo_SetUserOrgsDedup`、`TestOrgRepo_SinglePrimaryIndexEnforced`；`internal/service/org_delegation_integration_test.go`：`TestDelegation_SetUserOrgsCleansOwnerUserIDs`、`TestBK20_DeleteOrgWithOpenTickets`。

### 3.9 C：共同屏障、策略pair与两命令审计的准确纵切

基线`610c1d76`；本段范围是三个试点写用例及会改变其授权/验证输入的写方，不是全仓DBTX改造。所有新端口/表/HTTP头仍为设计候选。原12–19日是有界毛估，本次共用2–3归S2后C专用10–16，C05独立6–10；实际速度继续校准，不凭清单变长承诺免费包干。

| 读／写组与真实符号 | 锁与代际处理 |
|---|---|
| `internal/service/ticket/service.go`的`Create/Update`、`transition_service.go`的`Transition`及直接repo邻域 | 业务短事务先取守卫，再同tx fresh账号/角色/组织/scope/委托/参与和可变配置；只依赖中间件早期角色或旧context不合格 |
| `internal/service/user_service.go`的`Create/SetRoles/Delete/UpdateStatus`及创建用户复用的组织Tx入口 | 会改试点资格的路径先X守卫；初始绑定、删除关系和停用不能漏。普通头像/名称不机械加授权锁；会话效果归C05 |
| `internal/service/rbac_service.go`的`UpdateRole/DeleteRole/AssignMenus`及repo策略重建 | 角色状态/父边/priority用X守卫和fresh；只有实际p写入/删除（含AssignMenus、DeleteRole清策略）推进L1代际，纯名称/关系不全站重载 |
| `internal/service/org_service.go`的`AddMember/RemoveMember/SetUserOrgs/SetUserOrgsTx/SetMemberRole/SetMemberScope/SetOwners/BindOrgRole/UnbindOrgRole/Update/Move/DeleteOrgDelegated`及repo的`Delete/DeleteVgWithOwnerCleanup` | 覆盖成员、owner双轨、角色、scope、组织状态/祖先路径；X守卫在既有Move/组织行锁之前。不能只改公开入口漏Tx复用 |
| 工单`Assign/assignAuthorized`（含自动分派）、`AddCC/RemoveCC` | 改既有职责/可见性用X守卫，CC现pool写需窄Tx端；未来参与表不是已有实现。Close/Delete等同票写也先守卫后业务锁，避免反向锁序 |
| `internal/service/ticket/type_admin.go`的`UpdateTicketType/DeleteTicketType/ReplaceTicketTypeFields`及本批实际模板/字典依赖 | 先相应守卫/配置锁，保留类型版本与语义revision；纯类型/字段变化不推进Casbin代际。动态字典若被所选用例读取，必须接对应当前读取/写方，不能靠DBTX名字假定已冻结 |

当前生产菜单路由只有读；遗留`MenuService.Update`不构成开放菜单CRUD的理由。种子迁移/受控直库维护按部署窗口停写/推进适用版本，不能承诺任意直库SQL也受运行时屏障保护。

**事务及锁序**：动作登记时确定S/X，禁止拿S后升级。会在同一事务内改变授权关系的复合动作预先选X；未能证明所有分支只读权限事实时，不能按普通S路径执行到一半才调用赋权。固定顺序为授权守卫→适用既有superadmin/Move串行锁→组织/来源行→类型配置→风险ID→工单ID→关联对象（同类排序）。保留`LockTransitionSnapshotTx`先类型后票的相对次序；R01 claim先行短事务已提交，不带claim行锁去等业务守卫。

Service显式Begin/Commit，repo/Resource/ScopeResolver/OrgDelegationChecker使用同一DBTX（Query/QueryRow/Exec）；fresh读取端口不得fallback到pool或`RolesFromContext`。Create的组织/schema预取、Update的早期授权和Transition条件均在锁内对可变事实重核。只读外部取数在外，锁内验合法引用；禁止持锁HTTP/collector/LoadPolicy。未提交且无外部效果的死锁/序列化错误才能在同operation下有界重试，业务版本冲突交调用方刷新。

**策略pair**：当前`internal/casbin/enforcer.go`的`New`只启动LoadPolicy，`RBACService.reloadPolicy`只提交后短重试，不是持久配对。新增窄accessor持不可变enforcer+revision；loader在同一个只读REPEATABLE READ快照取DB p策略和revision，构造/校验新enforcer后原子安装。不能对共享enforcer原地LoadPolicy后再写revision；迟到旧loader不能覆盖较新pair。请求锁内比较DB revision，不匹配先结束事务、安排加载并安全拒绝；无可证pair时业务未就绪，健康存活可独立。role源变化已由fresh处理，不额外重建g映射或引入watcher服务。

**强审计纵切**仅SetRoles和AssignMenus：资格/原状态/请求与目标校验→业务变化→安全白名单diff的本地成功fact→必要补送intent→同一commit。任一步失败业务、fact、intent全回滚；请求诊断审计不能当fact。现`audit_logs`为HTTP method/path/status/duration/body等记录，`internal/middleware/audit.go`的`AuditLog`在请求结束/Flush后用pool写入，不具此原子性。复用窄Tx仓储/领域事实基础，不以原始body或HTTP200伪造业务成功，不扩大到所有IAM。

本地不可变fact与可变delivery分开；复用R01一个投递所有者，worker只补原fact ID，租约/fence防旧执行回写，外部失败不重做改权。collector无去重能力时仅承诺至少一次，不许诺外部恰好一次。query/export/UI完整版OBS02继续后继，不为C先造审计平台。

**operation与旧wire的明确兼容方案**：沿[共同operation/receipt](../design/capability-interconnect-detail.md#21-本地iam的两个严格profile)的身份/同号同hash/受权查询，存储归同一capability_operations；2026-11-15公共候选Schema已补IAM两个严格分支及63个形状例，仍非现成产品实现。capability_id为iam.user.roles.set/iam.role.menus.set、contract_version="1"，@1仅文稿简写。对外仍走原具名POST，不建IAM专属invoke/第二套receipt查询；领域提交与pending重载的收据语义沿该profile，不混collector状态。

旧POST body和成功data:null保留；可选`Idempotency-Key`采用`[A-Za-z0-9][A-Za-z0-9_.:-]{15,127}`，调用方发送前保存。唯一键为caller_namespace＋可信principal_id＋key，不能再把target放进命名空间以放过同key换目标；服务端映射到同一operation_id。缺省服务端生成operation，但旧客户端丢响应时可能不知道它。新增`X-Operation-ID`响应头用于关联；GET直接角色/角色菜单返回各自集合ETag，例如`"direct_roles:<target_id>:<revision>"`，新客户端用可选If-Match，typed命令仍必填expected_revision。只接该目标的强ETag，非法格式400；已知目标版本不符按[RFC 9110 If-Match](https://www.rfc-editor.org/rfc/rfc9110.html#section-13.1.1)返回412/稳定reason，原body版本冲突409继续区分。两头的允许/暴露CORS配置随同批适配，不改信封顶层。

目标domain_revision取同守卫下同capability+target的**已domain_committed最大单调版本**，不是按created_at/observed_at或runtime_effective排序；DB已提交但reload/collector pending也计入。索引覆盖该查询，实际变更递增，noop不伪造新集合版本。GET集合IDs与ETag须在同一S守卫/DBTX内取同快照，不能旧集合配新ETag；未知用户旧GET虽仍空集，不签发可编辑ETag。所有同集合写入口复用此写主，受控维护不能绕过。

旧历史首次初始化以当前DB集合为0；以后归档须保留每capability+target当前head或最小锚点，绝不能清空后回0使旧ETag复活。角色档位/状态另按当前事实检查，不借profile version、role.version或全局p代际替代集合版本。最小key→operation/hash/效果证明按原执行证据保留，不用普通短TTL过后重新执行旧改权；大payload/安全diff仍依自己的保留规则，不因此永久保存原始请求。

规范化先严格解析，role_ids/menu_ids按十进制ID去重并稳定排序；hash绑定命令版本、目标、集合、**原请求的expected_revision或明确未提供标记**和可信主体，不含请求追踪号，不采信客户端hash。key-only首次适配冻结缺省元数据，重放不得以当前head重生成expected_revision/hash。同operation先验当前安全读权和原绑定，再返回最小原receipt，不用旧If-Match重做，也不重放旧角色快照。无已完成效果才核当前写资格/版本、保留并写receipt；本库同步效果的唯一保留/成功receipt与业务同tx，第二个同key请求须等先行事务收敛或超时安全退出，不能并发再执行。

既有候选`GET /api/v1/operations/receipt`承接查询，选择器为operation_id或同一可信主体命名空间下的idempotency_key，二者互斥，不另建IAM GET。IAM仅投影原操作者仍有目标可见/所需管理资格的信息；猜ID/key不获读权。原效果事务仍在途或commit未证实时，暂未查到已提交receipt不证明无效果，UI继续待核实；只能原key安全重试/对账，不能换key重做。旧客户端无预存key时仅request_id受权核查，**没有自动去重或防陈旧编辑保证**。服务端commit未知按本次已知operation查询，不能依赖丢失响应里才返回的ID。

**待新增C-01–10**：①守卫前缓存允许、锁内撤权后零效果；②两事务双向提交次序与多来源对照；③表中各实际写方参与屏障、创建用户Tx/自动Assign/CC不旁路；④pool=1且无二次借连接；⑤p策略同快照pair、迟到loader不倒退；⑥SetRoles本地fact/intent失败全回滚；⑦AssignMenus同样失败回滚且不假成功；⑧commit未知、稳定键重放/异参及无键旧限制；⑨collector故障/恢复/重复/过期fence只补送；⑩成员/parent/status/CC/类型变化不触发不必要LoadPolicy。两事务及变异按至少8轮稳定红，HTTP/真实PG/进程恢复与替身证据分列。

C-08另含四个必要子例：GET/ETag交错、pending-effect仍推进集合head、归档后旧ETag不复活、只有key无If-Match成功重放不异参冲突。新412与旧409、head基线/未知用户无ETag分别核，不用HTTP状态成功掩盖版本错误。

复用`internal/repository/ticket_snapshot_pool_integration_test.go`的`TestTicketSnapshot_FullPoolUsesOnlyTransaction`、`internal/service/ticket/semantic_snapshot_integration_test.go`的`TestAuditValidatorFreshness/TestAuditConditionAfterReassignment/TestAuditTransitionConfigFreshness`、`internal/service/ticket/update_tx_hammer_integration_test.go`的`TestBK11_UpdateHammer`、`internal/repository/role_repo_integration_test.go`的`TestRoleRepo_AssignMenus_ConcurrentNoMix`、`internal/repository/org_delete_guard_integration_test.go`的`TestOrgRepo_DeleteAddMemberConcurrencyNoResidual`及`internal/repository/policy_eval_integration_test.go`的`TestRequestIDColumnsEndToEnd`；现门禁未覆盖新纵切。C只保证已接试点与对应写方，未接的业务必须在自身开通前接同端口，不冒称全模块或全部会话已恢复。

## 4. C05：账号、AT/RT/PAT 与停用恢复

保留密码纪元、iat地板、Redis Lua和fail-closed的安全目标，纪元真值迁到§4.2的DB写主，不能将authorization revision冒充password/session epoch。账号停用、角色剥夺与会话销毁分别表达：业务授权守卫以数据库当前状态/代际拒绝相关调用，后台持久intent投影Redis；PAT也必须核有效账号及当前权限，不只验哈希，但不接AT/RT的密码纪元门。

目标是禁用提交后Redis超时明确“账号已停用、会话清理待恢复”，不能诱发整笔重做或把禁用回滚为active。Login、Refresh、文件会话及适用worker准入核DB状态和安全屏障；PAT按其独立生命周期核当前账号/自身状态与业务授权，不因密码投影pending被误撤。**重新启用不自动清除未完成撤销**，须确认目标撤销完成，再按新epoch签发。恢复worker重投原intent并按状态/epoch CAS，旧enable/clear不能覆盖新disable；当前实际错误wire与未来变化见§4.1–4.3。

首版本地模式关闭 UAA adapter、UAA 网络不可达仍完整保留本地登录、用户/角色/菜单管理。完整 UAA-P 后才追加认证交换及中央纳管策略；OIDC 仅是参考 profile，不假定公司使用 OIDC。若采用它，adapter 按 [OIDC ID Token 验证](https://openid.net/specs/openid-connect-core-1_0.html#IDTokenValidation)及 [OAuth 安全实践](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.1)验证 issuer/audience/签名/时效/nonce、code flow 与 PKCE，核心只收规范化已验证身份；认证通过不等于获得 L1/L2/L3。

PAT条件性加固另见专项AUTH-05：当前Go1.27.4默认Reader不会回错误，不能把故障注入夸成正常签发可预测；仍应随C05去掉`generateSecret`固定回退。若可注入生成器返回错误，必须零落库/零secret回包，不改PAT独立生命周期政策。

### 4.1 C05现有wire与尚未持久化的部分

继续`POST /api/v1/auth/login`、`/auth/refresh`、`/auth/password/update`原TokenPair/cookie，`POST /users/status`的`{user_id,status}`、`/users/password/reset`的`{user_id,password}`原成功data:null，以及`GET/POST /user/pats`和`POST /user/pats/delete`。PAT只在创建时回一次secret。以下是源码现状，不是恢复能力已实现：

- `internal/service/auth_service.go`的`Login/Refresh`已有账号/密码/Redis密码epoch检查及防借用新epoch的重读，`internal/service/session_revoke.go`只有3次进程内重试和reconcile日志。当前永久epoch只存Redis，缺键为0；不能据此保证丢失后旧epoch0 AT不复活。
- `internal/middleware/jwt.go`的Bearer AT不读DB status，靠Redis blacklist/disabled/iat/epoch；文件cookie另有`internal/middleware/fileauth.go`入口，两者都需接新安全快照。PAT repo已join当前账号status/deleted及PAT自身状态，不靠AT/RT epoch。
- `UserService.UpdateStatus/Delete/ResetPassword`先提交DB，再撤Redis；最终Redis原错误经`internal/handler/errors.go`的`writeServiceError`可能为500，不能沿注释声称已有503/持久补偿。只有自服务`AuthService.UpdatePassword`显式映射503。启用当前直接清disabled，尚无本段安全屏障。
- `internal/pkg/admincli/reset_password.go`的`ResetPassword`也直接改密码再撤会话，必须纳入；普通用户metadata Update不含password字段，不扩无关写面。

### 4.2 C05的有限持久方案

技术候选选择DB为安全代际写主，Redis为投影/会话槽，不建设完整session平台：用户新增逻辑`session_epoch≥1`、`0≤session_projection_epoch≤session_epoch`；后者只表示该代Redis投影已确认，不代表全部业务/私有服务已生效。复用operation与authorization_effect_intents的`session_projection`，绑定目标/expected_epoch/desired_active/lease/fence，不再用重试INCR改变真值。本增量归C05单独子估及原包扣重，未含于C12–19日，不称现有小修免费完成。

初始值明确为epoch=1、projection_epoch=0。UserService.Create须同事务写初始投影intent；存量初始化同样逐批生成可恢复项，并允许worker从DB代际差值发现缺项、以稳定目标+epoch恢复唯一intent，不依赖一次性内存队列。原账号状态保持，初始化不能激活disabled用户；Login不得自行清disabled绕过pending。这样新用户与存量不会因没有恢复来源永远卡未就绪，首次启用须验证初始投影实际完成。

统一安全事务核由HTTP管理命令、自服务改密及受审CLI复用，原身份入口不混同：取C的适用守卫/保护锁与用户行，核当前状态/hash/epoch及各命令真实系统号/最后superadmin守卫，然后同tx写状态或密码、epoch+1、receipt和投影intent。disable保持status=0；Delete软删及既有关联清理同事务；ResetPassword保持原disabled状态、设must_change_password=true，不顺便启用。自服务bcrypt在锁外计算，锁内重核原hash/epoch/active，不能在管理员新重置后覆盖新密码。

enable先要求前一安全撤销已投影，未完成则明确冲突/待恢复且不先写active；获准后启用并推进新epoch，防旧disable/clear回调覆盖。重复相同目标状态不无意义推进，原operation重投不再改密码/代际。receipt/intent同tx是恢复必需，**不扩大两操作强审计政策**；口令及token不得落operation正文、弱可枚举hash或审计diff，秘密参数重放校验须用受控验证材料，准确私有输入摘要随C05消费合同收口。

worker仅按当前DB状态和epoch投影：旧代intent superseded，Lua单调安装目标epoch/状态，网络在DB锁外，完成按当前epoch＋原lease/fence CAS回写projection_epoch/receipt。失败保留本地安全事实及pending，不回滚禁用/改密，不改成新operation。RT槽按`refresh:db1:<uid>:<epoch>:<device>`隔代；旧worker不扫描删除新代槽。

新AT/RT显式带`session_schema=db_epoch_v1`与十进制session_epoch，文件cookie同AT。utils的`jwt/jwt.go`新增明确签发/claims能力，旧公共方法保持兼容，核心才要求新schema；不能让utils通用parser破坏其它消费者。Login验证的凭据/epoch快照必须与最终签发绑定，重读发现后来的新代只能拒绝，不能借新代签旧凭据。Refresh按原token epoch定位槽并原子比较旧hash/替换新hash，两次并发只一笔赢；不能GETDEL后无条件SET覆盖较新登录。最后检查后仍可能发生新的安全提交，已签旧代token由后续DB检查拒绝，不许诺网络输出与撤权跨库原子。

AT/RT及文件cookie核DB当前active/deleted/epoch与投影就绪，再核Redis会话约束；投影缺失/不可证503并恢复当前代，不能缺键回0。PAT只核当前DB账号、PAT自身状态及当前业务权，**不接AT/RT epoch或投影pending门**：密码重置不自动撤PAT，禁用/删除立即因账号拒绝，重新启用后未撤PAT按既有政策恢复。PAT DB故障由现统一401改为明确503若采用须列兼容用例，不虚标当前已区分。

新schema要求仅放JWT AT/RT分支，不能在Bearer统一入口抢先拒掉zpat_。业务最终fresh守卫仍核相应DB事实，不能用早期认证快照替代提交前资格。

### 4.3 C05切换、恢复与独立验收

当前测试环境可协调初始化DB epoch=1、切新schema并要求旧AT/RT重新登录；无需搬不可信Redis epoch，PAT及用户ID/业务历史不重建。未来正式环境只expand/校验/切读写并计划重登，down不可删安全代际后重新接受旧token；不理解新schema的旧代码须受控停会话/轮签名密钥后回退。

DB epoch解决全账号安全撤销，不持久化Redis单AT logout黑名单。Redis整体丢失或来源不可信恢复时，先拒绝会话服务、轮JWT签名key并重登录，不能重建同epoch后称已logout AT永不复活；只在可证明会话库其它状态可信时修复单个epoch投影。此运行门沿Q12恢复窗口绑定，不为首版造完整DB session/revocation表。Logout继续单设备/单AT语义，不顺手推进全账号epoch。

DB未提交零业务效果；commit未知查原operation；DB已提交而Redis失败明确pending并仅重投原效果。旧成功null/TokenPair不改，新响应头关联操作，受控503需给已提交事实说明，UI不能据此显示“没有修改”。旧token已失效的自服务查询须重新认证，响应丢失时用新密码重登核查，不临时放行旧token。新口令已被后续安全命令覆盖时不签后者epoch的token。

**UI实际接线**：UI仓`src/common/request/src/index.ts`现仅对401/20002做单飞refresh，401/20003清会话，refresh终态20004/20014/20015清会话；403/30003目前普通提示，不能称停用已完整退出。C05补账号停用与schema/epoch确定终态走`src/store/modules/user.ts`的resetState完整清理，过期仍按原单飞刷新；普通Redis/投影503不一律清所有会话。`changePassword`当前仅成功TokenPair时换token，故**已提交的自服务改密**须专用安全结果/operation引用，明确密码已变更、会话待恢复，停止复用旧token及自动重发，恢复后新密码登录；网络结果未知单列核查，不能显示必然未修改。请求/刷新/清理/迟到改密结果均核原session generation，不能清后来已登录的新账号。新增reason/码值与拦截器映射同批登记，不能让每个页面猜503含义。

**SESSION-01–10（待新增）**：DB commit＋Redis持续失败；commit后kill/恢复；disable/enable/迟到clear竞争；投影丢键与不可信全量恢复分开；Login不借新epoch；Refresh只一个赢家且不覆盖新登录；Bearer/cookie一致；PAT跨改密保持且禁用即拒；CLI/HTTP及新建/存量初始投影可恢复；未知结果/旧500与新pending兼容（含disabled完整退出、普通503不乱清、改密已提交停止旧session、迟到不清新账号）。至少8轮并发红例，真实PG/Redis/HTTP/进程恢复不由miniredis或旧绿色代替。

复用`internal/service/auth_epoch_race_integration_test.go`的`TestAuthEpoch_LoginCannotBorrowPostResetEpoch/TestAuthEpoch_RefreshBindsConsumptionEpoch/TestAuthEpoch_UserLifecycleCannotReviveOldAT`、`internal/service/auth_revocation_regression_integration_test.go`的`TestAudit_AdminResetThenLogin_OldATMustStayRevoked`、`internal/service/session_epoch_test.go`的`TestSessionEpoch_StrictReadAndConditionalClear`、`internal/service/session_revoke_test.go`的`TestRevokeUserSessionsWithRetry_RecoversAfterTransientFailure/TestRevokeUserSessionsWithRetry_PersistentFailure`及`internal/middleware/jwt_epoch_test.go`的`TestJWT_PasswordEpoch/TestFileSession_PasswordEpoch`。旧测试不证明新DB代际/持久intent已经存在；确切秘密摘要、claims与C05毛估见下节，公共IAM参考Schema见共同合同，真实维护者/恢复窗口沿原议程集中绑定。

### 4.4 C05秘密输入、claims与子估收口

口令写入不是前述两个公开IAM capability，不因共用operation存储就发布成工具。HTTP仍只接原password DTO；服务端在内存完成现行校验/bcrypt，把秘密输入与持久非秘密命令分开。规范operation只存命令种类/目标/原epoch/请求键及opaque secret_binding_ref，普通canonical_input_hash仍只对这份安全投影计算；原始/旧/新口令、bcrypt输入或私有校验digest不进receipt/审计/普通日志。该链限本地凭据写主；企业模式的受管凭据按UAA权威矩阵走外部端口，不借本地改密恢复受管身份的控制权。

私有重放校验固定`HMAC-SHA256`：服务端对purpose=C05-secret-input-v1、caller namespace、真实principal、命令/目标及**原始精确UTF-8秘密值**的封闭规范编码计算，常量时间比较。保留现密码字节语义，不Trim/Unicode归一化，也不存可离线枚举的裸SHA(password)。验证key来自独立只读secret_ref，数据库/制品/配置正文不含key；不复用JWT签名或服务AK/SK。初次绑定保存algorithm/key_id/digest于受限operation元数据，重放用原key_id和冻结默认值，不换当前active key重算。相同请求键换口令/目标/主体409且零新效果，秘密不可由客户端hash自证。

编码精确定为UTF-8、键排序/紧凑分隔/不做Unicode归一化的封闭JSON对象，键固定purpose/caller_namespace/principal_id/command/target_id/expected_session_epoch/secret_values；epoch为冻结正字符串，secret_values只含本命令已验证DTO的精确字符串值。HMAC消息仅内存使用，不将它当持久command正文。可复用[合成编码向量](../design/examples/capability-reference/c05-secret-binding-vector.json)核不同实现一致；其中零key和示例口令只为规范测试，严禁运行配置消费，未访问真实密钥/账号。公开receipt仅安全command hash，不回私有digest。

key轮换时旧验证key按去重窗口受控保留；若不可用，不盲认为同参、不重置口令或换新operation，返回稳定unverifiable/待核查，仍可在重新认证与当前目标权限允许后查询最小原receipt。最小效果证据不因普通TTL消失；永久留身份/效果不等于永久保存旧秘密验证材料。实际key托管/保留窗口沿Q12启用前绑定，技术方案不需要所有者选择哈希算法。自服务旧token失效后不能靠secret_binding_ref绕登录，使用新密码重登核查。

claims参考冻结为AT/RT新增可选`session_schema="db_epoch_v1"`、`session_epoch`规范十进制正字符串（int64范围）；核心对新schema分支必须校验，两字段缺一或非法均会话终态，不走旧epoch0回落。utils旧签发/解析入口对其它消费者保持兼容，新增显式安全代际签发能力，旧Pwe不能被当新DB字段；TokenPair/cookie外壳不改。projection intent只含operation/user/expected_epoch/desired_active/lease/fence及安全reason，不含token/口令/完整请求。USER创建/存量初始化/CLI也消费该profile，产品与公共库生成代码仍待实现。

源映射后的C05毛估候选为**6–10单席工程日**：DB代际/安全写方/初始化1.5–2.5；AT/RT/cookie/PAT及utils兼容1.5–2.5；投影恢复/安全重放1–1.5；薄UI/session generation/运维1–1.5；真实回归/评审1–2。原父包2–4日不足代表该持久切片，新增差异必须回原包扣重；此数不是追加到旧68–104日，也不混入C12–19日。S1/S2、C、G04、薄UI和OBS的同机制/同测试只计一次，完整DB session表/无感全量Redis灾难恢复明确不含。全版净额已按净分配整合为95–148候选，不能用已有回归免费包干。

SESSION-10补秘密变参/目标替换、key轮换/缺旧key、原默认值固定、receipt不泄digest及未认证secret_ref拒绝；SESSION-07/09补claims半字段/零/超int64及旧utils调用者保持原合同。这里是可写独立测试的候选，不是密码、密钥或数据已改。

## 5. D06 与 UAA-C：来源、目录和组织生命周期

```text
DirectoryEnvelopeV1 = {source_id: string, entity_kind: user|org,
  external_id: string, identity_generation: string, source_revision: string,
  ordering: authoritative_monotonic|authoritative_snapshot|unproven,
  observed_at: Timestamp, event_id: string, payload_hash: SHA256,
  operation: upsert|explicit_disable, payload: UserFacts|OrgFacts}
UserFacts = {employee_alias: string?, display_name: string, active: bool?,
  org_ref: ExternalRef?, manager_ref: ExternalRef?}
OrgFacts = {name: string, parent_ref: ExternalRef?, active: bool?}
HandoffPlan = {plan_id: string, subject_ref: ID, expected_revision: UInt64String,
  approved_mapping_ref: string, entries: [{kind: ticket|risk|membership,
  object_id: ID, object_revision: UInt64String, target_user_id: ID?, target_org_id: ID?}],
  plan_hash: SHA256, expires_at: Timestamp}
```

这些是公共领域输入，不是厂商报文透传。来源版本不一定可排序：`unproven` 下同身份/hash只能判重复，无法判新旧的冲突 hold；到达顺序不能伪造权威 revision。引用缺失可先 stage/held，依赖到齐后重验；姓名、工号、邮箱更改不改本地 ID；重用来源 ID 须明确新 generation，不自动接回旧人历史。完整快照必须有完整性证明，分页失败/局部缺失不能触发离职或撤部。HR 管目录任职，UAA 管批准范围账号/角色/资源；同字段只登记一个写主。

离职按已确认 B06：先安全停用，合法批准继任映射逐项检查目标在职/权限及对象 revision；合法项自动交接，无映射、冲突、不能直接转的项留 pending_handoff，不整批回滚安全停用。撤部按 B07：先预览虚拟组、在途业务和权限范围差异，plan 绑定 revision/hash/映射；批准后分批 CAS。新增/变化项重做预览，不偷纳入旧批准；保留历史归属，不把原部门角色复制给接管部门。计划/条目唯一 `(plan_id,kind,object_id)`，带 pending/applied/conflict/blocked 及原 receipt；恢复从未完成项续跑，目标离职即暂停该项。

UAA-C 现在冻结四个不能事后补的接口边界：稳定本地 ID 与认证 `issuer+subject` 显式映射；资源稳定 code 与页面/按钮 B 案；local/managed 单写主；三源角色撤销与代际传播。**三 feed 的持久投影、准确企业 wire、登录 UI/治理 UI 和切换台属于后续完整 P/R**，不是本地核心开工前置。后续 envelope 沿[port-map](../design/examples/uaa-reference-mock/port-map.json)的 application/source/kind/subject/revision/complete/payload，版本按对象范围而非应用全局比较；真实源不能证明规范化版本和 complete 时不得自动 apply。57 例离线模型继续作公共反例，不代替真实 PG/Redis 产品测试。

## 6. 统一日志、审计和管理 UI

复用[日志审计合同](../design/unified-observability-audit.md)，不自建日志检索平台。运行日志进现有采集器；领域事实/outbox 不采样；专门审计与业务历史分权。共同字段沿 record_id/kind、event_name、occurred_at/observed_at、service/instance、request/operation/invocation、可信 actor/resource/outcome；实际 trace 才写 trace_id，不把 request_id 改名。参考 [OpenTelemetry 日志模型](https://opentelemetry.io/docs/specs/otel/logs/data-model/)保留观察时间与实际 TraceId 的区分。

**2026-11-15所有者已确认IAM-AUDIT-D01，产品未实现**：仅SetRoles、AssignMenus两操作的业务写和本地成功审计同事务，本地审计写失败回滚；本地已提交、外部collector失败时持久补送，不重做改权。不扩大为其它日志/全部IAM均强审计。拒绝尝试与成功事实分开，失败后不补造成功。日志/审计不含密码、token、code、原公司报文或设备凭据。

薄 UI 同批展示本地/纳管来源、当前可写性、DB 已提交但运行时待生效、失败原因及获权重试/查询入口。普通业务管理员只能看受权对象历史；专门审计人员才能在指定范围跨模块检索/导出，导出自身留痕、大小有界、撤权停止后续读取。授权屏障异常、reload/revoke oldest age、失败次数、版本差距、拒绝原因只用有界标签；用户 ID 不作指标标签。首期保留天数/容量与真实审计名单在首次正式运行前确认，不编造合规要求。

### 6.1 OBS补充出口与首版本地保护

OBS01(P6.0)/OBS02(P6.1)位于[补充范围册](../review/post-baseline-scope-estimates.json)，**不在44个P6父包分母内**。本地核心S3先交已计价的敏感日志去除、可信request/operation/actor关联和必要本地事实；这不等于完整OBS01/02已实现，也不把OBS01–03完整22–33日追加到首版。完整跨仓外送、异步大导出、多worker判定管道与冷归档仍按原OBS03/P7或实际启用窗展开。

OBS01逐入口清单沿[统一日志合同§2/5](../design/unified-observability-audit.md)：core的AuditLog/body读取、AL查询/导入错误、TR回调/任务参数分别做允许字段投影、敏感值去除、字节预算及失败计数；不把截断当脱敏。缺采集器时可用受控stdout/既有本地审计，诊断采样不采掉领域事实。请求过大、非JSON/嵌套secret、CRLF、读body失败/handler仍可读、logger写盘或collector断开均有具名反例，三仓只对其实际敏感面接线，不造新logger平台。

完整OBS02的**准确候选**如下，只有本批启用对应审计读/写时建设，不强制本地核心先建审计总控台：

```text
AuditFactV1 = {record_id: ID, event_name: RegisteredAuditEvent,
  occurred_at: Timestamp, observed_at: Timestamp, producer: string,
  actor: {principal_id: ID?, service_id: string?, operator_user_id: ID?},
  resource: ResourceRef, scope: {kind: org|platform, org_id: ID?},
  outcome: domain_committed|runtime_effective|denied|failed,
  request_id: string?, operation_id: string?, source_event_id: string?,
  policy_revision: UInt64String?, diff: AllowedAuditDiff, reason: string?}
AuditQueryV1 = {start: Timestamp, end: Timestamp, event_name: string?,
  actor_id: ID?, resource_type: string?, resource_id: ID?, outcome: string?,
  operation_id: string?, cursor: string?, limit: int}
AuditPageV1 = {items: AuthorizedAuditFact[], next_cursor: string?, as_of: Timestamp}
```

查询候选固定为 `GET /api/v1/audit/facts`，Query字段使用有界query参数，不收客户端授权条件。导出候选 `POST /api/v1/audit/facts/export` 绑定独立按钮；body为同Query及固定允许列。不改旧`/audit/logs`请求诊断记录的含义，新领域成功事实不能伪造一条HTTP200日志替代。最小逻辑`security_audit_facts(record_id PK,event_name,occurred_at,observed_at,producer,actor_ref,resource_type/resource_id,scope_kind/org_id,outcome,operation_id/source_event_id,payload jsonb)`，索引(scope_kind,org_id,observed_at,record_id)、(operation_id)、(resource_type,resource_id,record_id)。对非空原事实使用稳定`(producer,source_event_id,event_name,resource_type,resource_id,outcome)`唯一键；不同生效阶段分别追加，不update旧成功事实。复用原领域事件+只读投影能达到同语义时优先复用，逻辑实体不强制多造表。

服务端从当前专门审计职责生成scope/字段谓词，**先过滤再分页/计数/导出**；客户端org/actor筛选只能收窄。历史fact保存发生时合法组织锚，读取按当前获准审计组织集合相交，不因actor已调岗按其现部门重归历史；平台级IAM事实须明确platform审计资格，不能给普通部门管理员。业务管理员查对象历史仍核当前对象/字段权，不自动得到跨模块audit查询。cursor绑定查询hash和当前授权，不是可移用的授权凭据；分页为普通历史列表，不用ID游标宣称可靠事件feed/全局提交顺序。

最小受权分页/受限导出复用既有audit页，起始窗口≤31日、limit≤100、同步导出≤1000条为可收紧技术候选；超限明确要求缩范围，不截断后冒称完整。CSV只输出白名单标量、做表格公式注入防护；不导出body/token/任意payload。大导出/快照artifact只有真实需要时按原异步方案接入，不作基础门。查询/导出只记安全访问元数据，避免审计递归；失败不回显不可见记录ID或全文。

两IAM试点按F017已确认政策、完成实现与验收后才把成功AuditFact与业务同事务；拒绝/失败事实不是成功补写，collector失联由已存本地事实恢复。`TestAuditFacts_ScopeBeforeCountExport`、`TestAuditFacts_OrgMoveAndPlatformScope`、`TestAuditFacts_CursorRevokeAndCSV`、`TestAuditFacts_AtomicCommitAndDuplicate`分别验当前职责/字段与范围、历史组织锚、scope变更/导出预算、业务回滚和原事实去重。本段是详细候选，真实人员/保留参数、迁移号与产品红绿仍在OBS02启用前落实。

## 7. 工程与一致性切片

G01 任务 manifest 至少含 task_id、owner、base_sha、allowed_paths、worktree、runtime_owner、ports、db/redis/storage namespace、cleanup_allowlist；先装 hooks，再以显式路径提交，单会话推送。目录模板沿[文档约定](../documentation.md)，G06 用模板版本/同步检查维护共性，不从主仓复制带业务权限的 utils；新仓不造空目录。

G02/03 对当前真实 workflow 检查名、事件 SHA、候选组合、必需 job 始终生成、PR 权限/secret 边界逐项核验。现有 17 项 CI 绿色不等于分档/远端保护已配置；启用分档须按既有规则同步并用分类反例证明，治理/规则/spec/fixture/混合改动不归普通文档白名单。新增业务链有真实 E2E 后再登记为必需证据，不制造永远 success 的占位 job。

G04 先做高价值测试：表达式 AST 及输入投影 fuzz、文件引用跨单/GC 并发、IAM 撤权与提交竞态、任务 receipt/来源版本重放、schema 边界。PR 有界短 fuzz，定时长 fuzz；输入种子去敏，崩溃样本保留并转确定性回归。性能先测锁等待/列表过滤与有限流水线吞吐，记录数据量/机器/并发，不先承诺任意 QPS 或用覆盖率单值判定质量。

C01从当前字段定义（D01后为钉版定义）投影可输入键；公式服务端重算，同名函数/变量按AST角色区分，非法链/资源耗尽可解释拒绝。C02上传默认追加、多选；字段内容引用与附件归属分别判断，去引用不删文件，无需为此新建引用表。真正删除须当前归属/权限且无有效字段/正文引用，绑定/解绑/逻辑删除/GC共用对象锁和事务，不能信客户端文件名/大小/引用列表。详细工单合同见[工单详设](ticket-module-detail.md)。

2026-11-15准确接线后继：现live配置与将来发布快照分别扫描；C01保formula显隐的一次服务端派生视图，不开放新DSL/preview；C02首期不加引用表/读字段，按同桶关联判归属，旧孤儿先盘点hold。具体wire、源符号、存量及C01-01–10/C02-01–08见[正确性施工合同](../design/engineering-execution-design.md#c01准确接线与一次快照显隐)。同页[五仓CI实际接线](../design/engineering-execution-design.md#2026-11-15实际入口与最小接线)复用现有检查并列ENG-01–08，不把新quality-gate/分档或私有runner写成已启用。

## 8. 实施切片、TDD 与验收证据

先共同 Context/守卫/receipt，再接本地 IAM 及实际业务入口；目录生命周期按所启用feed交付，不让完整HR/UAA表阻塞人工风险。薄UI随批，随后全链角色验收。P/R只在自身窗口完整实现，不因公共C已写就宣称企业联调完成。

| 用例 ID | 可复现输入/故障 | 必须断言 |
| --- | --- | --- |
| FND-01 | 关闭 UAA、网络不可达，普通/空菜单/管理账号 | 本地完整旅程可用；空菜单无业务数据请求；原 IAM 可管理 |
| FND-02 | 两事务用 barrier：旧允许写 vs 撤角色/参与人 | 撤权先 commit 后旧决定零新增效果；反序仅撤权前提交那次有效 |
| FND-03 | direct/org/inherited 各一路，撤最后或其中一路 | 重算有效权限，移除一路不误删其它合法来源；外部同名 admin 无内置特权 |
| FND-04 | DB 改权成功，进程 A loader/Redis 失败，进程 B 旧 cache | stale pair 拒绝；原 intent 恢复，新 revision 生效；无跨实例旧授权放行 |
| FND-05 | disable→Redis 超时→enable 竞争；旧 AT/RT/PAT | 账号先安全禁用；未撤销完成不解除屏障；旧 clear 不复活旧 token |
| FND-06 | SetRoles/AssignMenus审计插入失败/collector不可达（政策已批准、产品未交） | 前者业务与审计同回滚；后者本地成功事实仍在、只重投审计 |
| FND-07 | HR 重复/乱序/未知 org/同工号异人/缺页 | 去重、hold、不误并号、不因缺页停用；无未授权管理属性写入 |
| FND-08 | 批准交接后对象/继任者变化，worker 中断 | 变化项 conflict/blocked，其它合法项有独立 receipt，恢复不双转、不改历史 |
| FND-09 | 新资源漏 GetFilter，计数/导出/附件/结果深链 | fail-closed；与列表相同范围，不靠 UI 隐藏防泄露 |
| FND-10 | 错 owner 清理/混入索引/必需 job skip 或取消 | 精确清理拒绝，提交归属可核；门禁不被空 success 代替 |
| FND-11 | DBTX第二步 history/intent 插入失败；pool=1；commit响应断开 | 同事务零半套，不偷第二连接，未知提交查原receipt不重做 |
| FND-12 | 两真实事务 barrier：Org.Delete vs BindOrgRole/成员写 | 仅合法串行结果：已有绑定拒删，已删组织拒绑；无孤儿授权/半套清理 |
| FND-13 | 给票A加CC/改查看策略，同时读无关票B | A按新L2事实判定，B无Casbin重载/全站503；真正L1策略变化仍触发代际检查 |
| FND-14 | 停用实际组织；direct/org/inherited及本组/祖先委托、group/all/透明读 | 组织来源跨L1/L2/L3停止，列表/菜单/计数/导出一致；其它独立合法来源不误撤，BK-20另测 |
| FND-15 | 启用child→停用parent；UI菜单/权限码与真实Casbin路由 | 父角色投影消失且接口拒绝，不能只测后端403 |
| FND-16 | 仅org:create的custom60→selfService绑custom5；admin10→custom5；合法弱角色对照 | 两种越权均拒且零副作用，合法弱业务角色可按资格绑定；系统角色禁绑不回退，owner不自动获IAM授予权 |
| FND-17 | middleware已缓存允许角色，取得守卫前另一事务撤权 | 锁内fresh读取不命中旧context；不得以“再次调用RoleFetcher”替代真正重读 |
| FND-18 | 正常PAT与自定义失败随机Reader（条件性加固） | 正常签发不同；注入失败拒签、零落库；不冒称默认Reader故障可利用 |
| FND-19 | 角色编辑GET→相同POST→撤组织/父边；含停用直接角色及增保删 | 零新增direct来源，合法绑定不丢；停用不授当前权，无权解除强旧绑定整笔拒绝、合法可解除；撤派生后无固化权限，三种视图分轨 |
| FND-20 | 相同org_ids/primary原样保存，已有admin/group及其它任职属性 | 保留项属性不被默认覆盖，新增/移除仍正确且同事务 |
| FND-21 | pending/error/坏200/目录缺项；组织甲慢乙快/乙失败；重复确认/迟到POST | 当前成功快照前零POST，不串目标/丢绑定；提交payload固定且不重入，失败保草稿；成功读取后按解除资格明确清空 |

测试先写业务正/负/边界/权限全部预期，目标行为有效红后实现；并发变异按standards至少8次稳定红验牙。首版本地验真实PG/Redis、双事务竞争、单实例重启/恢复、UI普通/管理员/CC/只读旅程及完整适用门禁；真正启用同服务多副本前再做两实例策略/租约/配额故障注入，不能把完整HA验收强塞单实例首发。当前表格是测试设计，不是已运行结果。真实公司UAA、HR和消息合同分列real待验；本地正式出口不等UAA。

## 9. 决策和变更记录

HIER-D01子树暂停和IAM-AUDIT-D01两操作本地审计失败回滚已由F017确认，见[第一包收口](iam-first-batch-design.md)。本轮仅设计；A先作为4–6日子估候选，B/C和任务窄修复分开扣重，旧S1不能代表全部新明确范围。不重新询问已定B01–B07、mock、设备人工登录或本地先正式。真实用户映射/通知配置、接管/退出计划及保留参数在相应启用前确认；不影响无关模块详设。

- 2026-11-14：按实际主线补三层共同守卫、代际/会话恢复、D06 typed 输入和逐项交接、UAA-C/P/R 分界、统一审计试点及工程出口。逻辑表未建、迁移未领号，估算仍按原包扣重。
- 2026-11-15：将事务屏障与L1策略副本同步区分，去掉资源级变化引起的全站重载；明确身份/治理表按实际feed与P/R创建，补日志审计补充出口与首版所选最小保护的区别。
