# 首版模块详设交接：接缝与权限/UAA首包

日期：2026-11-14。状态：**设计交接，未启动开发、未完成产品或企业验收**。现在进入按模块详设；先短冻结下列首版共同合同，不再以全仓审计、全项目或全部接口冻结为前置。
范围/日期以[首版计划](first-usable-release-plan.md)及[机器目标](../review/first-usable-release-target.json)为准：2027年1月试用、2月1日前核心可用；单席参考的按期可行性未证明，真实UAA/公司消息联调时间未定。
本页仅交接首批模块顺序与接缝，不另立设计规则、报价或状态总账；[工程公约](../standards.md)、[项目控制](../review/11-project-control.md)、[规划总账](../review/planning-backlog.md)及各领域合同继续各管其权威内容。

## 1. 先共同钉住的六条边

以下是待模块详设落实的首版语义，所有能力/DTO均是设计候选，未新增或宣称已上线API。共同基线复用[调用合同](../design/capability-invocation-contract.md)与[互通详设](../design/capability-interconnect-detail.md)；准确候选见[Schema](../design/capability-interconnect-detail.schema.json)的`command`、`receipt`、`event`、`subscription`、`admissionClaims`、`alExecutionRequest`。

1. **风险→工单**：风险域写评估、责任、派单关联与逐风险复核结论；工单域写票、流程及办理事实，保留有界N:1关联。派单需当前风险职责范围、功能许可、目标类型/组织及业务状态均允许；同operation重投不重复派票，任一边未确认不能显示闭环成功；工单完成不自动替所有关联风险下结论。准确事务所有者、关联键与补偿/对账由风险/工单包冻结。
2. **工单→TR**：工单域同事务写节点运行、稳定operation关联及可靠意图，TR写调度/运行事实；发布配置绑定能力/合同版本、固定输入及结果策略，提交和执行均验当前资格。意图失败不显示已安排，不持票锁调用网络；TR受理不等于业务成功，提交回包丢失查原operation，不另造效果。
3. **TR→结果/AL**：能力属主写效果与receipt，AL独占数据/revision/效果收据，核心只写原票运行的结果投影；TR不直写AL库。AL按获准行/字段、generation、expected_revision和短时准入执行，core发送前复验当前授权；同号异参拒绝、冲突可见，超时进入unknown先查原收据。结果回贴按operation/receipt revision核原节点版本，投影失败只补投影，不重做效果、不自动改票状态。
4. **AL→TR后继**：AL与受支持数据写同事务保存源事实/outbox；核心写持久inbox、受控订阅和派生意图，TR负责获准任务调度。先可靠收件再ACK，ACK只证明可恢复接收；稳定事件/订阅版本去重、当前grant复验、保留因果和抑制原因。首个后继选实际只读复核等用途，不再写回触发字段；重复、失联、积压、撤权不得漏事实或盲目重跑。
5. **业务→消息**：业务域独占业务事实，消息域写模板实例/发送记录，私有HTTP adapter处理公司协议。收件人/字段投影按当前授权与批准映射，事实、通知及任务结果分别记身份；受理/送达依真实合同。通知超时/失败显示待发、失败或待核实，不回滚已提交业务，不用通知成功代替整改完成。
6. **UAA→主体/角色/资源投影**：中央写其纳管账号/分配事实，公共核心写受限来源投影、本地会话及三层执行结果；私有adapter仅规范化和可靠投递。按来源/应用/主体/版本/完整范围准入，直接、组织、继承三源共同收敛；未知引用hold、完整空集才可撤该范围，收到/提交/策略会话生效/外部回传分别取证。失联不切回本地放权，外部同名admin不获内置管理员语义。

首版选择以11/14计划为准：固定受审流程/基本字段、一个AL结构字段组及一种订阅；监控AL类型仅开放普通insert/update，服务端拒绝该类型import/replace、未支持的delete/restore等写入口及破坏性schema修改，其他类型已有功能不因此退役。采用**单源outbox→push→核心持久inbox/ACK**，不把11/13较宽方案的分页多消费者feed、五写入口/批量import或通用回放整包当首版门；支持入口的事实完整性、幂等、恢复和保留窗仍须成立。首批实际场景确需扩展时，先补对应正确性合同再启用。

## 2. 第一包：权限/UAA详设的定向输入

先读本页、[UAA治理合同](../design/uaa-governance-integration.md)§3–5/7/10、[UAA源码重核](../review/reports/core-release-uaa-recheck-2026-11-14.md)；候选输入从[port-map.json](../design/examples/uaa-reference-mock/port-map.json)与[离线参考说明](../design/examples/uaa-reference-mock/README.md)取，不将57例内存模型当产品实现。
真实供给核验只读[联调条件册](../review/uaa-real-integration-conditions.json)；公司协议、地址、身份、凭据与原始回执留在内网私有侧，不要求导出后才能设计公共合同。
代码只定向核对下列真实符号及被改动的调用邻域，路径＋符号作锚点：

- 认证/会话：`internal/service/auth_service.go`的`AuthService.Login`/`Refresh`、`internal/middleware/jwt.go`的`JWT`、`internal/service/session_revoke.go`的`RevokeUserSessionsWithRetry`；已有本地JWT不是SSO认证器。
- L1/目录：`internal/middleware/casbin.go`的`CasbinAuth`、`internal/casbin/enforcer.go`的`New`、`internal/service/rbac_service.go`的`RBACService.AssignMenus`/`reloadPolicy`、`internal/model/menu.go`的`Menu`/`MenuAPI`；目录映射保留B案，页面GET、写端点按钮行。
- 来源/写侧：`internal/repository/role_repo.go`的`GetEffectiveRoleCodes`/`RoleRepo.AssignMenus`、`internal/service/user_service.go`的`UserService.SetRoles`/`UpdateStatus`及`internal/model/user.go`的`User`、`internal/model/role.go`的`Role`；保留原superadmin保护，不由adapter绕服务直写表。
- L2/L3：`internal/service/ticket/resource.go`的`Authorize`/`GetFilter`/`canOperate`、`internal/service/ticket/service.go`的`resolveListFilter`、`internal/service/ticket/transition_service.go`的`Transition`；风险/AL/TR需各自明确资源检查，不能用工单已有实现推定全链已有。

### 2.1 首包必须形成的状态与source映射

- 身份：已验证`issuer+subject`与本地ID显式映射；治理feed的`application/source/subject`单独映射，不假定与工号、邮箱或认证subject相等。改号/回聘不自动并号，JIT默认关闭。
- 可登录性：中央允许、本地未安全停用、必要身份/治理状态可证明才建立会话；完整空授权是有效的零权限状态，不因没有角色或菜单拒绝安全首页访问；投影未就绪只能按已确认合同给受限状态，不开放未知业务权。中央启用不能解除本地封禁，未知枚举隔离，停用联动AT/RT/PAT及受限委托。
- 管辖：逐资源键登记写主、source、纳管范围/数据条件、直接/组织/继承血缘、撤销优先级与本地允许入口；受管写入口只读/受控，非受管本地管理保留，旧本地绑定不得复活撤权。
- 三feed独立：`account`改账号状态，`user_roles`替换某用户的完整受管角色集合，`role_resources`替换某角色完整受管资源集合；三者不得误成全应用清表。候选envelope沿用`event_id/application/source/kind/subject/revision/complete/payload`，同版本冲突、缺页及未知依赖不可按到达顺序扩权或缺失删除。
- 生效：候选`committed_pending_effect/held_dependency/effective/superseded/ignore_stale`沿用参考语义，具体持久状态与迁移图在本包冻结；接收/领域提交/策略及会话生效/治理回传分阶段，重载、缓存或撤销失败须持久待恢复，不能只日志后报全链成功。
- 失联：F006已定先停受管写；公司允许且能证明授权有效时保留获准只读，无法证明或超窗停受管能力。恢复仅限已授权维护者修配置/查同步/对账；真实期限、人员、动作和接管/退出基线仍待实际条件与批准，mock时钟不是生产SLA。

### 2.2 L1/L2/L3用例矩阵与失败出口

矩阵列固定为“当前资源键/路由功能L1、数据集合及字段L2、具体动作及状态L3、预期结果/零额外效果、证据”；以下各行均需在列表/详情/计数/导出、任务结果、AL读写及消息投影的适用入口展开，不适用项写理由：

- 普通处理人：L1仅已授功能；L2仅当前可见票/数据；L3还验处理职责、节点、字段和版本，能看不自动能改。
- 风险组获授权管理者：L1按批准资源；L2限定风险职责范围；L3派单资格独立，不因组内owner/admin称谓获得系统admin或任意票处理权。
- 只读成员、CC分别成行：仅已授功能与当前可见范围；已获准CC可查看和评论，不附送改字段/转派/改状态等处理权。两者均不能由“可见”推导其它写权，消息/结果不得带越权字段。
- 同部门非参与人：即使有功能入口也按实际scope过滤；未获明确资源范围与动作资格时拒读/拒写，不因同部门兜底放行。
- 后台受限主体/委托：服务签名只鉴别调用服务，提交与执行分别核当前主体、grant、行字段/状态；排队后撤权hold，旧结果查询重验读权，不换key重做。
- 中央撤权/停用及外部同名admin：页面/按钮/API分别拒绝，直接/组织/继承旧来源不能补回；活跃AT/RT/PAT、未开始任务、迟到结果及重载/撤销故障均有反例。

### 2.3 本包准确冻结项与可验收出口

编码前冻结：认证交换/可信Context字段；三feed准确schema、对象动作枚举、来源版本及完整性证明；资源目录/数据条件映射；接收、提交、生效、冲突/held与回传DTO；状态迁移、幂等唯一性、重试/对账、权限epoch及新鲜度检查点。候选端口不等于新增现成HTTP路由；若需API遵GET/POST公约，POST业务信息入body。
DDL在本包核现行模型和[迁移地图](../review/11-project-control.md)后冻结：身份映射/唯一性、来源管辖及三源授权血缘、收件去重/版本、投影状态与policy revision、会话失效/重载待办、回传outbox及保留关系；不预占迁移号，不以“三表加source”替代撤权闭合证明。
公共出口：同一核心在无UAA配置/私有adapter/企业网络时完整验证本地登录、账号/角色/菜单分配和三层；再接合成HTTP身份/三feed验证持久投影、撤权、hold、最小来源只读/异常UI与运维恢复。离线模型只复用反例，不代替真实PG/Redis/浏览器及产品门禁。
企业出口另追加真实认证、三feed、目录/数据条件、撤权/故障、治理回执与适用公司激活门；真实条件未定时明确仅公共/隔离试用。固定公共合同下验**旧参考adapter×新核心**，内部model/DDL不泄露到adapter；破坏性升级显式新版本/转换或协调迁移，不承诺永久零改。
审计沿[统一日志/审计合同](../design/unified-observability-audit.md)：业务历史与专门审计资格分开已定；`UserService.SetRoles`和`RBACService.AssignMenus`两类高风险写的“本地必需审计失败同事务回滚”仍待所有者答复，不因本页、日期目标或详设推进自动批准。

## 3. 后续波次及每模块统一六交付物

第一波为本权限/UAA包，并行详设风险→工单人工闭环；第二波为工单→TR→原运行结果与AL受权写/源事实，可按已冻结DTO分别展开；第三波合上AL→TR实际后继、业务消息、薄UI及失败恢复。UAA持续贯穿，消息真实合同核验可先行；模块以所需子出口衔接，不等整个Phase或完整通用平台。设计并行不等于增加已确认有效席位。
每个模块仅在既有模块文档/合同补六项；不复制一套新框架或反复重读全仓：

1. 首版用户旅程、正常/失败用例及明确后置范围。
2. 模块写主与接缝、准确typed输入/输出/错误、版本与旧adapter兼容。
3. 领域状态/数据及事务、source/幂等/CAS/可靠交接与恢复；需迁移时列成对up/down和存量兼容。
4. L1/L2/L3及字段/结果权限矩阵、撤权/失联/重放反例和业务历史/审计边界。
5. 同批薄UI、运维配置/指标、可见失败与受权对账/恢复入口。
6. TDD正反例与验收出口、上下游输入/依赖、既有成果复用/净增待核及SSOT状态回填；不在本页另报总价。

每包最小上下文为本页、首版目标/计划中适用范围、该模块权威合同、相邻六边中实际消费的候选DTO/反例，以及上述定向代码符号；开始只核工作树状态和这些输入的差异。发现具体冲突或缺口再按文件＋符号查邻域/历史，不定期重扫所有仓库，不承诺token节省比例。共同合同修改只记录改动原因、受影响消费者及兼容决定，回填原SSOT并通知相邻包，未受影响模块继续详设。
实现仍须先写真实行为反例再使其变绿；保留现行完整适用门禁、迁移验证与`make acceptance`纪律，不启用分档或以合成模型替代。本文纯设计交接，不声称已跑产品门禁或首版按期可交付。
