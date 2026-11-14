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

首版采用**一个数据库授权代际行**，而非分布式锁服务：候选 `authorization_control(id=1, revision bigint NOT NULL CHECK revision>=0)`。本地业务量尚小，先以简单正确的全局策略变更屏障换取可证明撤权；普通业务共享锁可并行，只有授权变化排他。后续观测锁等待后再按资源/组织分片，不能在首版暗做全套权限平台。

1. 事务开始前按动作选择锁模式：普通受权读/写取该行 `FOR SHARE`；会改变有效权限的 IAM、组织任职/继承/数据范围、类型查看策略、参与/CC 管理取 `FOR UPDATE`。**禁止先共享后升级**。批量拆有界事务，但每批独立复验。
2. 取得守卫后读取 revision；每进程使用不可变的 `(enforcer, applied_revision)` 对。只有 applied_revision 与 DB revision 相等，才可使用该策略实例；否则先结束本次事务、触发重载并返回可恢复的 `POLICY_NOT_READY`，不得在持锁期间请求网络或把旧策略当准。
3. L1 走该 revision 的统一 PDP；L2/L3 从同一短事务的当前账号、组织、scope、参与及业务状态求值。写主锁业务行、核 expected_revision，再写效果、领域历史、必要 intent。守卫保持到 commit，避免“撤权已提交而旧允许决定随后仍提交”。外部调用只在提交后做。
4. 改权事务在排他守卫下复验操作者资格，保留最后 superadmin/种子号保护；更新所有适用直接/组织/继承来源、增加 revision，并同事务插入恢复 intent。提交释放锁后才重载/撤会话；旧内存实例因代际不等不能继续放行。
5. loader 使用**只读 REPEATABLE READ 事务**在同一数据库快照读取全部策略与 revision，校验后原子替换整个 pair；不能用多条 READ COMMITTED 查询拼出旧策略+新 revision。加载中又有新代际则允许安装旧 pair，但后续请求比较不等仍拒绝并再次加载。不能先置新 revision 再慢慢改原 enforcer。启动没有可证 pair 时受保护接口未就绪，健康存活与业务就绪分开。

固定锁序为授权守卫 → 来源/业务 group → 风险 ID → 工单 ID → 其它关联 ID（各集合排序），遵[生命周期合同](../design/business-lifecycle-contracts.md)。无反向先业务后 IAM，禁止持业务锁 HTTP。PG 死锁/序列化故障仅在未产生外部效果且相同 operation 下有界重试；业务冲突交调用者刷新。事务使用 READ COMMITTED 并在取锁后读当前事实，不依赖事务开始前缓存。

本设计是采用 [PostgreSQL 行锁及一致锁序](https://www.postgresql.org/docs/18/explicit-locking.html)的项目选择。借鉴 [OPA 状态模型](https://www.openpolicyagent.org/docs/management-status)区分“策略接收/存储”和“激活”，**继续使用现有 Casbin**；不引入 OPA 服务或以心跳代替代际检查。所有启用入口（含 PAT、worker、代理、批量和查询）接该公共守卫是开通前置；遗漏入口不得凭 L1 中间件已存在放行。

### 3.1 逻辑数据与恢复所有者

| 候选实体 | 约束/索引 | 写主与用途 |
| --- | --- | --- |
| authorization_control | 单行主键；revision 单调 | IAM 共同屏障；不可被私有 adapter 写 |
| iam_change_receipts | UNIQUE(caller_namespace, idempotency_key)；绑定 canonical_hash；target+created_at 索引 | IAM 原业务事务保存提交事实/当前 receipt revision |
| authorization_effect_intents | UNIQUE(operation_id, effect_kind, target_id, desired_revision)；(state,next_attempt_at)索引；lease_owner/until/fence | 复用 R01 worker 机制；一个 effect 一个重试主，过期 fence 不更新新执行 |
| identity_source_bindings | UNIQUE(source_id,entity_kind,external_id,identity_generation)；本地 ID 索引 | D06 目录映射；issuer+subject 映射在 UAA-P 增补，不用邮箱作主键 |
| governance_ownership | UNIQUE(application,resource_key,scope_key)；revision/source/allowed_write_port | 本地先登记 local，P/R 批次切批准范围的单一写主 |

表名是逻辑建议，实施前与已有 job submission/审计表对表复用，不复制第二套 outbox；迁移 up/down 成对、编号当天从迁移地图领取。初始代际为 0，对现有本地来源显式回填；回滚新增约束前停止新入口并处理在途 intent，不能删除正式历史以求 down 通过。

### 3.2 DBTX 试点、组织删除和事务预算

C04 原批准试点继续仅覆盖 `ticket.Service.Create/Update/Transition` 及其直接 repo 邻域：主仓内部 `DBTX={Query,QueryRow,Exec}`，`WithinTx(ctx,pool,fn(DBTX))` 由 service 开始、提交和回滚，repo 显式收同一 tx；不放任意 context、不隐式嵌套、不在 tx 内再从 pool 借连接。所需 schema/业务读也用该 tx，网络取数/HEAD/模型在外，锁内重新验可变引用。commit 返回错误先按 operation 查询 receipt，不整体盲重试或把“未知”报成“无效果”。DBTX 不下沉到 utils 的业务接口，不全仓重构。

`OrgRepo.Delete` 现有组织行锁与子组织/成员/未结工单检查保留，**增加 org_roles 存在检查并返回稳定冲突+受权计数**，要求显式解绑再删，不暗删授权。`BindOrgRole`、任职/相关组织依赖写均在授权守卫之后通过同一个 `lockAliveOrgForShareTx` 校验并锁存活组织；删除取该组织 `FOR UPDATE`，删除/绑定双方同序，避免“删前没绑定、提交时刚绑入”。虚拟组 `DeleteVgWithOwnerCleanup` 单独按同一来源/owner 清理政策验；历史软删孤儿只读盘点后受审清理，不自动恢复有效授权。

数据库连接池 max/min/lifetime、请求总 deadline、事务/statement/lock timeout 使用有界配置；初值以现有部署及隔离压测为依据，记录排队/持锁耗时，不凭空承诺生产 QPS。pool 上限为1的试点回归必须仍可完成，用来检出同 tx 内偷借第二连接；等待池或锁超预算在未提交时明确503/可恢复冲突，先回滚再重试。禁止长导出/HTTP等占授权守卫，分页导出每块重验当前权限并可中断。

## 4. C05：账号、AT/RT/PAT 与停用恢复

保留现有密码纪元、iat 地板、Redis Lua 和 fail-closed，不能将 authorization revision 冒充 password epoch。账号停用、角色剥夺与会话销毁分别表达：业务授权守卫立即以数据库当前状态/代际拒绝相关调用，后台持久 intent 继续撤 Redis 会话；PAT 也必须检查同一有效账号和当前权限，不只验证哈希。

禁用提交后 Redis 超时返回“账号已停用、会话清理待恢复”，不能返回“停用失败”诱发反复新操作，也不把已禁用状态回滚为 active。`Login`、`Refresh`、PAT 和 worker 准入均核 DB 状态及未完成的安全撤销屏障；**重新启用不自动清除未完成撤销**，须确认目标撤销已完成并按新 epoch 签发，避免旧 AT 复活。恢复 worker 重投原 intent，按期望状态/epoch CAS；较旧的 enable/clear 不能覆盖较新的 disable。密码修改的旧会话失效、同秒新旧 token、并发 Refresh、Redis 丢 key/不可用沿已有测试继续覆盖。

首版本地模式关闭 UAA adapter、UAA 网络不可达仍完整保留本地登录、用户/角色/菜单管理。完整 UAA-P 后才追加认证交换及中央纳管策略；OIDC 仅是参考 profile，不假定公司使用 OIDC。若采用它，adapter 按 [OIDC ID Token 验证](https://openid.net/specs/openid-connect-core-1_0.html#IDTokenValidation)及 [OAuth 安全实践](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.1)验证 issuer/audience/签名/时效/nonce、code flow 与 PKCE，核心只收规范化已验证身份；认证通过不等于获得 L1/L2/L3。

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

## 7. 工程与一致性切片

G01 任务 manifest 至少含 task_id、owner、base_sha、allowed_paths、worktree、runtime_owner、ports、db/redis/storage namespace、cleanup_allowlist；先装 hooks，再以显式路径提交，单会话推送。目录模板沿[文档约定](../documentation.md)，G06 用模板版本/同步检查维护共性，不从主仓复制带业务权限的 utils；新仓不造空目录。

G02/03 对当前真实 workflow 检查名、事件 SHA、候选组合、必需 job 始终生成、PR 权限/secret 边界逐项核验。现有 17 项 CI 绿色不等于分档/远端保护已配置；启用分档须按既有规则同步并用分类反例证明，治理/规则/spec/fixture/混合改动不归普通文档白名单。新增业务链有真实 E2E 后再登记为必需证据，不制造永远 success 的占位 job。

G04 先做高价值测试：表达式 AST 及输入投影 fuzz、文件引用跨单/GC 并发、IAM 撤权与提交竞态、任务 receipt/来源版本重放、schema 边界。PR 有界短 fuzz，定时长 fuzz；输入种子去敏，崩溃样本保留并转确定性回归。性能先测锁等待/列表过滤与有限流水线吞吐，记录数据量/机器/并发，不先承诺任意 QPS 或用覆盖率单值判定质量。

C01 只从发布字段定义投影可输入键；公式输出在服务端重算，名字相同的函数/变量按 AST 角色区分，非法公式链/资源耗尽要可解释拒绝。C02 上传默认追加、多选；字段引用与普通附件列表分别记录，删除引用不删除文件。真正删除须当前归属/权限且无有效字段或正文引用；绑定/解绑/删除/GC 共用对象锁和事务，不能信客户端文件名/大小/引用列表。详细工单合同见[工单详设](ticket-module-detail.md)。

## 8. 实施切片、TDD 与验收证据

先共同 Context/守卫/receipt，再接本地 IAM、目录生命周期及实际业务入口；同批薄 UI，随后全链角色验收。P/R 只在自身窗口完整实现，不因公共 C 已写就宣称企业联调完成。

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

测试先写业务正/负/边界/权限全部预期，目标行为有效红后实现；并发变异按 standards 至少 8 次稳定红验牙。最终需真实 PG/Redis、两进程故障注入、UI 普通/管理员/CC/只读旅程以及适用完整门禁；当前表格是测试设计，不能当已运行结果。真实公司 UAA、HR 和消息合同分列 real 待验；本地正式出口不等 UAA。

## 9. 决策和变更记录

现在需 owner 决定的是 §6 两类 IAM 操作的本地强审计失败政策；本轮已给具体场景，不重新询问已定 B01–B07、mock、设备人工登录或本地先正式。真实用户映射/通知配置、接管/退出计划及保留参数在相应启用前确认；不影响无关模块详设。

- 2026-11-14：按实际主线补三层共同守卫、代际/会话恢复、D06 typed 输入和逐项交接、UAA-C/P/R 分界、统一审计试点及工程出口。逻辑表未建、迁移未领号，估算仍按原包扣重。
