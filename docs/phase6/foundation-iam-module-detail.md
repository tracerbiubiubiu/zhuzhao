# Phase6 底座、三层权限与身份接缝详细设计

日期：2026-11-14。状态：**模块详细设计候选，未实现**。范围为 G01–G04/G06、C01/C02/C04/C05、D06、E01-UAA-C 及统一审计在本期的适用部分。G07A/B 已合主线并有[新版主线复验](../review/reports/main-verification-2026-11-14.md)，不重新建设或重复计价；正式运行前仍验实际制品和数据恢复。本页将[模块交接](module-design-handoff.md)的第一包展开，业务范围、净估与远期出口继续归[首版计划](first-usable-release-plan.md)、[总账](../review/planning-backlog.json)。

本页所有新类型、逻辑表和端口均为实施目标；**不是新增现成路由、迁移号或已完成验收**。ID 对外用十进制字符串，时间用带时区 RFC3339；枚举拒绝未知控制值，空集、缺失和 null 不相互替代。既有 errcode/HTTP envelope 继续使用，稳定 reason 对应数值码在实现时登记。

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

Context 由可信认证层构造，body 的 user_id/operator/role 不替代它；`AuthorizationDecision` 只描述一次决定，不能当作长期 bearer token。新写用例要求 expected_revision；旧 REST 端点保留现行输入，由锁内业务谓词防交错，不能冒称已阻止旧客户端陈旧编辑。旧 wire 不静默换成 IAMReceipt：旧接口在现有 envelope 中兼容增加 operation 引用，提交后重载失败仍保持其已有明确错误，响应可关联原操作；新的状态查询用例才返回完整 receipt。不知道原结果时先查原操作，不重复授予/撤销。

L1 为当前页面/按钮/API 功能；L2 为该主体可访问的资源行及字段；L3 为此动作、对象状态、职责和字段约束。列表、详情、计数、聚合、导出、历史、附件、任务结果都用同一资源过滤器和字段投影；列表/详情中不可见与不存在同为 404，已可见但动作禁止为 403。依赖不可证明为 503，revision/同号异参为 409。不能因没有菜单而拒绝安全首页，也不能由“有首页”增加业务读权。

## 3. C04：共同授权守卫与多实例

首版采用**一个数据库授权守卫行**，而非分布式锁服务：候选 `authorization_control(id=1, revision bigint NOT NULL CHECK revision>=0)`。它承载短事务撤权屏障，`revision`仅表示需要同步的**L1 Casbin策略代际**，不是所有资源变化的全局版本。普通业务共享锁可并行，授权变化排他；后续观测锁等待后再按资源/组织分片，不在首版暗做全套权限平台。

1. 事务开始前按动作选择锁模式：普通受权读/写取该行 `FOR SHARE`；会改变有效权限的IAM、组织任职/继承/数据范围、类型查看策略、参与/CC管理取`FOR UPDATE`。Assign/Transfer或风险owner交接若改变既有职责/可见集，也按改权动作选择排他模式，不能因来源改为派生事实而漏守卫。**禁止先共享后升级**。批量拆有界事务，但每批独立复验。
2. 取得守卫后读取L1 revision；每进程使用不可变的 `(enforcer, applied_revision)` 对。只有 applied_revision 与 DB revision 相等，才可使用该策略实例；否则先结束本次事务、触发重载并返回可恢复的 `POLICY_NOT_READY`，不得在持锁期间请求网络或把旧策略当准。`AuthorizationDecision.policy_revision`仅标L1视图，不能证明L2/L3已判断。
3. L1走该revision的统一PDP，当前有效角色按直接/组织/继承来源重新求值，JWT中的旧角色不是授权真值；L2/L3从同一短事务的当前账号、组织、scope、参与及业务状态求值。写主锁业务行、核expected_revision，再写效果、领域历史、必要intent。守卫保持到commit，避免“撤权已提交而旧允许决定随后仍提交”。外部调用只在提交后做。
4. 改权事务在排他守卫下复验操作者资格，保留最后 superadmin/种子号保护；更新适用直接/组织/继承来源。**只有实际改变L1策略或其有效角色映射的操作**推进L1 revision并写policy_reload intent；会话安全变化按需写session_revoke intent。参与/CC、类型查看范围等只改L2/L3事实时更新其资源/策略revision，仍在同一守卫内提交并从当前数据库判断，**不触发全站LoadPolicy**。提交释放锁后才做对应恢复；不得为了省重载继续信旧L2缓存。
5. loader 使用**只读 REPEATABLE READ 事务**在同一数据库快照读取全部策略与 revision，校验后原子替换整个 pair；不能用多条 READ COMMITTED 查询拼出旧策略+新 revision。加载中又有新代际则允许安装旧 pair，但后续请求比较不等仍拒绝并再次加载。不能先置新 revision 再慢慢改原 enforcer。启动没有可证 pair 时受保护接口未就绪，健康存活与业务就绪分开。

固定锁序为授权守卫 → 来源/业务 group → 风险 ID → 工单 ID → 其它关联 ID（各集合排序），遵[生命周期合同](../design/business-lifecycle-contracts.md)。无反向先业务后 IAM，禁止持业务锁 HTTP。PG 死锁/序列化故障仅在未产生外部效果且相同 operation 下有界重试；业务冲突交调用者刷新。事务使用 READ COMMITTED 并在取锁后读当前事实，不依赖事务开始前缓存。

本设计是采用 [PostgreSQL 行锁及一致锁序](https://www.postgresql.org/docs/18/explicit-locking.html)的项目选择。借鉴 [OPA 状态模型](https://www.openpolicyagent.org/docs/management-status)区分“策略接收/存储”和“激活”，**继续使用现有 Casbin**；不引入 OPA 服务或以心跳代替代际检查。所有启用入口（含 PAT、worker、代理、批量和查询）接该公共守卫是开通前置；遗漏入口不得凭 L1 中间件已存在放行。

策略副本同步与本地资源事实分别处理也符合[Casbin watcher职责](https://casbin.apache.org/docs/watchers/)；这里选择数据库代际检查，不因此引入新的watcher组件。短期ref采用[Redis到期键](https://redis.io/docs/latest/commands/expire/)只承担可丢的请求上下文，绝不替代持久效果凭证。

### 3.1 逻辑数据与恢复所有者

| 候选实体 | 约束/索引 | 写主与用途 |
| --- | --- | --- |
| authorization_control | 单行主键；L1策略revision单调 | 共同事务屏障；L2/L3变化不推进Casbin代际；不可被私有 adapter 写 |
| iam_change_receipts | UNIQUE(caller_namespace, idempotency_key)；绑定 canonical_hash；target+created_at 索引 | IAM 原业务事务保存提交事实/当前 receipt revision |
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

[第二轮复验](../review/reports/permission-boundary-recheck-2026-11-15.md)新增RECHECK-01/02/03，当前产品仍未修。必须区分三种查询：**直接绑定编辑值、有效三源角色/菜单、用于IAM管理档位的有效直接角色**。`GetUserRoleIDs`作为直接授予编辑出口不能复用effective展开；原样保存不把组织/继承角色固化direct，也不丢弃尚有绑定但停用的角色；失效绑定只读保留不授予当前操作者新增高档位角色的权力，写侧仍守原档位/来源约束。组织/继承及未来UAA受管来源只读展示，不转为local写；现有role_ids wire可按正确语义修复，不要求先建全来源管理台。

`SetUserOrgsTx`的集合替换改为同事务差量：保留相同org的角色、scope、来源、时效及joined_at，is_primary仍按既有primary_org_id输入语义更新，不借差量修改缺省/null含义；新增按默认、移除按已准写主及现有owner双轨清理。旧Phase1 DELETE重插实现目前会把admin/group重置member/assigned，不能将仅修改组织ID集合等同重设全部任职属性；具体角色/scope仍走原专用命令。

用户角色/组织等整体替换编辑器必须先绑定当前user_id＋请求世代的成功快照。打开新目标先清旧草稿；pending/error时按钮与submit一起拒绝，显示重试；迟到响应不盖新目标，不能用空数组/前一用户的选择替代加载失败。成功后显式清空仍允许。上述是C04及薄UI/U06的当前数据正确性修复，不等全站视觉改造或真实UAA。

### 3.5 层级组合的首版出口与分工

按[层级场景合同§8](../design/rbac-inheritance-and-cascade.md#8-层级关系场景合同与实施归属2026-11-15候选)的HIER-01–12随包实施：C04处理三源/单父角色链、组织树合法性和Move/撤权、菜单导航与功能码分离、直接编辑视图；D03处理scope与类型/参与交集；D06处理多任职/来源及组织生命周期。组内任职不等于平台角色，整组绑定管理能力不能被个别viewer抵消；领域仍核具体目标职责。

首版本地S1先交实际启用规则及薄IAM正确性，D03/D06随其业务批交适用场景，不等完整U06/HR/UAA；HIER-07/08/11复用FND-03/14–16/19–21及既有AUTH/RECHECK反例，不重复建设。父节点停用对子树传播HIER-D01是C04详设前待确认政策，先保持独立切片推进。§8.4的阶段/净估边界和当前未实现状态同步总账，未将完整树编辑器或来源管理台塞入S1。

### 3.6 模型复用与引擎选择边界

按[模型复评](../design/authorization-model-and-engine-choice.md)，本期继续Casbin＋领域Resource/PG范围，不将L1/L2/L3误作与RBAC/ABAC/ReBAC竞争的模型。复用现有机制，先统一可信输入、直接/有效来源、对象/列表语义及共同用例检查；菜单/路由是稳定能力的执行/展示映射，内部与worker不能只靠HTTP中间件授权。业务状态和证据不变量仍在领域执行。

PBAC的策略可审/可测/可解释原则可随既有C04/CE接线采用，不新增通用DSL、规则分发/版本库或外置PDP。后续重复规则/独立治理或跨对象关系出现时，可按模型复评做Cerbos/OPA/Cedar或OpenFGA/SpiceDB的单资源对照；须另定范围/净估，不暗含进C04。当前缺陷、HIER-D01待决及三层授权新鲜度要求均保留。

## 4. C05：账号、AT/RT/PAT 与停用恢复

保留现有密码纪元、iat 地板、Redis Lua 和 fail-closed，不能将 authorization revision 冒充 password epoch。账号停用、角色剥夺与会话销毁分别表达：业务授权守卫立即以数据库当前状态/代际拒绝相关调用，后台持久 intent 继续撤 Redis 会话；PAT 也必须检查同一有效账号和当前权限，不只验证哈希。

禁用提交后 Redis 超时返回“账号已停用、会话清理待恢复”，不能返回“停用失败”诱发反复新操作，也不把已禁用状态回滚为 active。`Login`、`Refresh`、PAT 和 worker 准入均核 DB 状态及未完成的安全撤销屏障；**重新启用不自动清除未完成撤销**，须确认目标撤销已完成并按新 epoch 签发，避免旧 AT 复活。恢复 worker 重投原 intent，按期望状态/epoch CAS；较旧的 enable/clear 不能覆盖较新的 disable。密码修改的旧会话失效、同秒新旧 token、并发 Refresh、Redis 丢 key/不可用沿已有测试继续覆盖。

首版本地模式关闭 UAA adapter、UAA 网络不可达仍完整保留本地登录、用户/角色/菜单管理。完整 UAA-P 后才追加认证交换及中央纳管策略；OIDC 仅是参考 profile，不假定公司使用 OIDC。若采用它，adapter 按 [OIDC ID Token 验证](https://openid.net/specs/openid-connect-core-1_0.html#IDTokenValidation)及 [OAuth 安全实践](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.1)验证 issuer/audience/签名/时效/nonce、code flow 与 PKCE，核心只收规范化已验证身份；认证通过不等于获得 L1/L2/L3。

PAT条件性加固另见专项AUTH-05：当前Go1.27.4默认Reader不会回错误，不能把故障注入夸成正常签发可预测；仍应随C05去掉`generateSecret`固定回退。若可注入生成器返回错误，必须零落库/零secret回包，不改PAT独立生命周期政策。

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

SetRoles、AssignMenus 的“本地审计失败同事务回滚”仍是**待所有者确认的两操作试点**；本页不自行批准。选择通过后，业务写和成功审计同事务，外部 collector 故障由持久投递补送；选择未通过不得发布为已启用强审计。拒绝尝试与成功事实分开，失败后不补造成功。日志/审计不含密码、token、code、原公司报文或设备凭据。

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

两IAM试点经owner批准后才把成功AuditFact与业务同事务；拒绝/失败事实不是成功补写，collector失联由已存本地事实恢复。`TestAuditFacts_ScopeBeforeCountExport`、`TestAuditFacts_OrgMoveAndPlatformScope`、`TestAuditFacts_CursorRevokeAndCSV`、`TestAuditFacts_AtomicCommitAndDuplicate`分别验当前职责/字段与范围、历史组织锚、scope变更/导出预算、业务回滚和原事实去重。本段是详细候选，真实人员/保留参数、迁移号与产品红绿仍在OBS02启用前落实。

## 7. 工程与一致性切片

G01 任务 manifest 至少含 task_id、owner、base_sha、allowed_paths、worktree、runtime_owner、ports、db/redis/storage namespace、cleanup_allowlist；先装 hooks，再以显式路径提交，单会话推送。目录模板沿[文档约定](../documentation.md)，G06 用模板版本/同步检查维护共性，不从主仓复制带业务权限的 utils；新仓不造空目录。

G02/03 对当前真实 workflow 检查名、事件 SHA、候选组合、必需 job 始终生成、PR 权限/secret 边界逐项核验。现有 17 项 CI 绿色不等于分档/远端保护已配置；启用分档须按既有规则同步并用分类反例证明，治理/规则/spec/fixture/混合改动不归普通文档白名单。新增业务链有真实 E2E 后再登记为必需证据，不制造永远 success 的占位 job。

G04 先做高价值测试：表达式 AST 及输入投影 fuzz、文件引用跨单/GC 并发、IAM 撤权与提交竞态、任务 receipt/来源版本重放、schema 边界。PR 有界短 fuzz，定时长 fuzz；输入种子去敏，崩溃样本保留并转确定性回归。性能先测锁等待/列表过滤与有限流水线吞吐，记录数据量/机器/并发，不先承诺任意 QPS 或用覆盖率单值判定质量。

C01 只从发布字段定义投影可输入键；公式输出在服务端重算，名字相同的函数/变量按 AST 角色区分，非法公式链/资源耗尽要可解释拒绝。C02 上传默认追加、多选；字段引用与普通附件列表分别记录，删除引用不删除文件。真正删除须当前归属/权限且无有效字段或正文引用；绑定/解绑/删除/GC 共用对象锁和事务，不能信客户端文件名/大小/引用列表。详细工单合同见[工单详设](ticket-module-detail.md)。

## 8. 实施切片、TDD 与验收证据

先共同 Context/守卫/receipt，再接本地 IAM 及实际业务入口；目录生命周期按所启用feed交付，不让完整HR/UAA表阻塞人工风险。薄UI随批，随后全链角色验收。P/R只在自身窗口完整实现，不因公共C已写就宣称企业联调完成。

| 用例 ID | 可复现输入/故障 | 必须断言 |
| --- | --- | --- |
| FND-01 | 关闭 UAA、网络不可达，普通/空菜单/管理账号 | 本地完整旅程可用；空菜单无业务数据请求；原 IAM 可管理 |
| FND-02 | 两事务用 barrier：旧允许写 vs 撤角色/参与人 | 撤权先 commit 后旧决定零新增效果；反序仅撤权前提交那次有效 |
| FND-03 | direct/org/inherited 各一路，撤最后或其中一路 | 重算有效权限，移除一路不误删其它合法来源；外部同名 admin 无内置特权 |
| FND-04 | DB 改权成功，进程 A loader/Redis 失败，进程 B 旧 cache | stale pair 拒绝；原 intent 恢复，新 revision 生效；无跨实例旧授权放行 |
| FND-05 | disable→Redis 超时→enable 竞争；旧 AT/RT/PAT | 账号先安全禁用；未撤销完成不解除屏障；旧 clear 不复活旧 token |
| FND-06 | 审计插入失败/collector 不可达（试点批准后） | 前者业务与审计同回滚；后者本地成功事实仍在、只重投审计 |
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
| FND-19 | 角色编辑GET→相同POST→撤组织/父边；含停用直接角色 | 零新增direct来源，合法绑定不丢；撤派生后无固化权限，直接/有效/IAM档位视图分轨 |
| FND-20 | 相同org_ids/primary原样保存，已有admin/group及其它任职属性 | 保留项属性不被默认覆盖，新增/移除仍正确且同事务 |
| FND-21 | 角色读取pending/error；组织甲慢乙快/乙失败；提交按钮和函数 | 当前成功快照前零POST，迟到不串目标；成功读取后明确清空仍合法 |

测试先写业务正/负/边界/权限全部预期，目标行为有效红后实现；并发变异按standards至少8次稳定红验牙。首版本地验真实PG/Redis、双事务竞争、单实例重启/恢复、UI普通/管理员/CC/只读旅程及完整适用门禁；真正启用同服务多副本前再做两实例策略/租约/配额故障注入，不能把完整HA验收强塞单实例首发。当前表格是测试设计，不是已运行结果。真实公司UAA、HR和消息合同分列real待验；本地正式出口不等UAA。

## 9. 决策和变更记录

现在需 owner 决定的是 §6 两类 IAM 操作的本地强审计失败政策；本轮已给具体场景，不重新询问已定 B01–B07、mock、设备人工登录或本地先正式。真实用户映射/通知配置、接管/退出计划及保留参数在相应启用前确认；不影响无关模块详设。

- 2026-11-14：按实际主线补三层共同守卫、代际/会话恢复、D06 typed 输入和逐项交接、UAA-C/P/R 分界、统一审计试点及工程出口。逻辑表未建、迁移未领号，估算仍按原包扣重。
- 2026-11-15：将事务屏障与L1策略副本同步区分，去掉资源级变化引起的全站重载；明确身份/治理表按实际feed与P/R创建，补日志审计补充出口与首版所选最小保护的区别。
