# 权限模型与引擎选择：2026-11-15复评

状态：**设计建议与旧论证勘误，未更换引擎、未修产品**。依据为当前main `aa745173`、本地详设后继 `3f0a1472` 及下列官方资料。承接所有者关于RBAC、三层校验、ReBAC/OpenFGA和PBAC的提问；不把历史“优”等评分、Star数或客户规模作为选型证据。公司资料、地址、人员和报文未用于外部查询。

<!-- local-core-source:start -->
当前首版本地核心净估与条件以[唯一净估册](../planning/local-core-release-estimate.json)为准；本文不另立总价。
<!-- local-core-source:end -->

## 1. 判断与推荐

**出发点正确，当前模型适合首个本地核心版；实现的一致性还不足，不能据此称为成熟的通用权限产品。** 保留角色管理及三层检查，复用现有Casbin，把领域授权收口到共同用例和资源策略；采用策略可审、可测、可解释的治理方式。当前不新增通用策略语言、策略编辑器或独立授权服务。

这并不是认定现有引擎永远最好。**规则治理成为主要成本时，优先比较Cerbos/OPA等策略引擎；对象关系复用成为主要成本时，优先比较OpenFGA/SpiceDB。** 两类也可以组合，但本项目没有同时引入多套引擎的当前需求。均先作有界对照验证，再决定是否迁移，不纳入本地首版前置。

## 2. 模型、三层与产品是三个问题

| 概念 | 本项目的实际问题 | 建议用法 |
|---|---|---|
| RBAC：角色组织功能资格 | 风险管理者是否有派单能力；普通成员是否有工单查看能力 | 继续使用，避免给每张工单新建一个角色。RBAC本身也有继承和约束，并非只能表达平面角色 |
| ABAC：主体、对象、动作及环境属性 | 授权是否到期、组织是否启用、对象类型策略是否允许 | 使用少量明确规则，不先开放任意表达式配置 |
| ReBAC：对象与人的关系参与决定 | 是否处理人/抄送人；是否目标组织获权管理者；未来证据是否继承工单可见性 | 当前已有关系型规则，但没有自建通用关系图引擎；复杂度增长时可交成熟引擎 |
| PBAC：以策略表达和管理规则 | 规则如何版本化、评审、测试、解释和分发 | 可以与前三者共存，不是“比RBAC高一代”的替代模型；本轮按Policy-Based含义使用 |
| L1/L2/L3：本项目检查职责 | 有没有功能资格／能否接触目标对象／能否对它办理当前动作 | 保留职责划分；更换决策引擎仍需这些检查。不是某项标准认证的“三层模型” |

概念依据：[NIST RBAC](https://csrc.nist.gov/projects/role-based-access-control/faqs)、[NIST ABAC](https://csrc.nist.gov/pubs/sp/800/162/upd2/final)、[Casbin对PBAC与RBAC/ABAC的区分](https://casbin.apache.org/docs/pbac/)。PBAC不要求网络调用；例如[OPA可嵌入Go](https://www.openpolicyagent.org/docs/integration)。

例：甲具备`ticket:read`，但不是某受限工单的相关人员，仍应拒绝；把甲加入抄送后，在当前类型范围允许且具备相应功能资格的前提下，他可查看、评论，不能因此转派或改字段。甲后来成为获权处理人，也须满足转换权限，提交时仍检查当前状态与必填证据。**身份可用、功能资格、对象范围、动作授权，以及业务不变量共同成立才执行**；“证据未填不能归档”等业务不变量留在领域用例，不需要全搬进权限DSL。这里的类型范围等补强是D03目标，不能当成当前main全部已实现。

## 3. 现有系统的价值与差距

当前证据锚点为`internal/middleware/casbin.go`的`CasbinAuth`、`internal/repository/role_repo.go`的`GetEffectiveRoleCodes/ListRoleIDsByUserID`、`internal/service/ticket/resource.go`的`Authorize/GetFilter/canOperate`及`internal/pkg/resource/registry.go`的`Resource/Filter`。实际生产Resource目前仅工单，不能从注册接口存在推定所有模块已经接入。

| 方面 | 现有价值 | 应补的基础能力，已有归属 |
|---|---|---|
| 功能、对象、动作分开 | 避免只隐藏菜单、只验接口或给每对象造角色 | HTTP、内部、worker及回调进入同一受控用例；稳定能力码与路由/菜单投影对应，C04及CE01登记/CE02接线、共同调用合同 |
| 组织/角色分开、多来源合并 | 能表达兼职、虚拟组、直接/组织/继承来源 | 直接绑定编辑与有效权限分轨，来源独立撤销、生命周期一致；C04/D06，原AUTH/RECHECK仍开放 |
| SQL在分页前过滤 | 列表/计数能与业务条件同库组合 | 单对象、List/Count/Search/Export及附件/引用出口不得漂移；C04/D03及所接资源，不能在分页后删无权行冒充正确结果 |
| 组织树与领域关系有边界 | 当前无需维护任意关系图与第二份授权关系库 | `org_roles`是全局功能角色来源，目标组织任职另算；菜单祖先不传递API权；按HIER-01–12逐批验，HIER-D01整子树暂停已确认、未实现 |
| 本地独立与UAA接管分开 | 满足本地先正式、内网私有adapter后接 | UAA受管写主、稳定主体、来源/撤权和新鲜度不能省；E01-UAA-C/P/R。外部角色同名不能自动映射全局admin |
| 默认拒绝及有界管理 | 已有Resource空过滤哨兵等机制 | `priority`只是本地管理档位，不是完整可授权/职责隔离证明；业务管理员的可授能力/对象范围须明确，不因组织owner自动获得系统管理权 |

这些差距主要是**授权事实和规则在不同出口的一致性**，并非证明RBAC或三层分工错误。已确认缺陷继续以[现状审计](../review/reports/authorization-design-implementation-review-2026-11-15.md)、[后继复验](../review/reports/permission-boundary-recheck-2026-11-15.md)为准，本轮不增加一份同名缺陷账，也不以换工具关闭它们。

## 4. 成熟工具如何选

| 方案 | 能复用什么 | 对本项目的代价与结论 |
|---|---|---|
| **继续Casbin＋领域资源策略** | 已有Go库、角色能力策略、当前PG事实与列表谓词 | 首版推荐。统一领域策略与输入，减少重复if；Casbin支持ABAC、PBAC和domain-scoped角色，不代表现有配置已启用这些模型 |
| **Cerbos PDP** | 面向应用资源/动作的YAML策略、角色/派生角色及属性条件；`PlanResources`生成条件计划，已有Go/pgx转换器 | 若规则需要独立治理，作为优先候选之一；当前ltree、关联查询、字段与NULL语义仍需映射/验证，不能假定转换器自动覆盖。开源PDP可自托管，Hub非必需；也有Go进程内启动PDP server的形式 |
| **OPA** | 通用Rego策略、策略测试、分发/嵌入；部分求值可生成SQL过滤 | 公司已有OPA治理或确有通用策略复用时更有吸引力。旧“只返回bool、一定双写策略”不成立；但SQL转换只支持相应策略子集，现有关系谓词能否覆盖须验证 |
| **Cedar** | 面向授权的语言、schema校验、角色/属性/层级实体、permit/forbid及决定原因，已有官方Go实现 | 值得借鉴模型与策略分析；仍需提供相关可信实体/上下文并解决列表查询及生命周期管理，不能因有Go库就视为现成IAM/关系存储；未验证稳定PG过滤接线 |
| **OpenFGA** | 声明对象关系、组/父子继承与条件；Check/ListObjects等关系查询 | 真实关系密集场景优先候选；需确定关系写主/投影同步、模型升级、撤权及搜索/计数组合。不是接入后自动消除所有业务SQL |
| **SpiceDB** | 关系模型、带条件关系、ZedToken和查询新鲜度选项 | 同属关系引擎候选，不要求达到某个巨量规模才评估。若需要把授权修订点与内容版本关联，具体比较其一致性协议及维护成本 |
| **Keycloak等IAM** | 登录、联合身份和账号管理，也确实有Authorization Services | 当前企业已指定UAA，不另建竞争身份平台。仅接OIDC/角色令牌不替代对象范围/业务动作；本地独立模式继续保留现有管理 |

上表是基于本项目约束的选型判断，不是性能排名。工具能力来源：[Casbin](https://casbin.apache.org/docs/supported-models/)、[domain角色](https://casbin.apache.org/docs/rbac-with-domains/)、[Cerbos资源策略](https://docs.cerbos.dev/cerbos/latest/policies/resource_policies.html)、[查询计划](https://docs.cerbos.dev/cerbos/latest/recipes/filtering-resources)、[pgx适配器](https://github.com/cerbos/query-plan-adapters/blob/main/pgx/README.md)、[PDP源码与部署](https://github.com/cerbos/cerbos)、[OPA数据过滤](https://www.openpolicyagent.org/docs/filtering)、[可转换子集](https://www.openpolicyagent.org/docs/filtering/fragment)、[Cedar概念](https://docs.cedarpolicy.com/overview/terminology.html)、[Keycloak授权架构](https://www.keycloak.org/docs/latest/authorization_services/#_architecture)。

部署/语言边界另见[Cerbos Go Serve](https://pkg.go.dev/github.com/cerbos/cerbos/pkg/cerbos)、[Cedar Go](https://github.com/cedar-policy/cedar-go)及[实体slicing职责](https://docs.cedarpolicy.com/policies/level-validation.html)。不将PBAC等同独立网络服务，也不因Cedar最初的Rust实现而排除它；采用时仍须核Go实现的功能边界，例如其schema validator当前在实验路径。

外置引擎与业务库可独立共存，不是架构冲突；代价在权威来源、同步与业务提交窗口。OpenFGA官方也区分[业务库与授权事实的写主](https://openfga.dev/docs/best-practices/source-of-truth)，并要求[搜索与授权结果组合](https://openfga.dev/docs/interacting/search-with-permissions)。它的`HIGHER_CONSISTENCY`跳过缓存查库，与[SpiceDB的ZedToken](https://authzed.com/docs/spicedb/concepts/consistency)不是同一协议；两者都不会自动把平台业务事务和远程关系写入变成一个原子事务。[OpenFGA一致性](https://openfga.dev/docs/interacting/consistency)。

策略翻译也不是只换语法：[Cedar](https://docs.cedarpolicy.com/auth/authorization.html)中forbid覆盖permit，但错误策略会被跳过并记入diagnostics；如果forbid出错而另一个permit成立，仍可能Allow。对照时须明确诊断错误是否由平台转为拒绝，不能把默认拒绝理解为任何求值错误均拒绝。[Cerbos资源策略](https://docs.cerbos.dev/cerbos/latest/policies/resource_policies.html)在同角色/动作内deny优先，多个角色之间某角色最终allow则整体可allow。迁移必须保留本项目明确的未知/错误处理和强制范围交集，不能用一个“只读角色”的deny试图抵消其它合法角色，更不能意外让宽角色绕过类型查看范围。

## 5. 可以直接借鉴的产品实践

- **Redmine**：角色功能权限、工单可见范围、按角色/类型的状态转换、字段权限分别设置；与本项目的职责拆分相近。借鉴边界和用户可理解的配置，不照搬整个矩阵。[角色](https://www.redmine.org/projects/redmine/wiki/RedmineRoles)、[工单流程与字段](https://www.redmine.org/projects/redmine/wiki/RedmineIssueTrackingSetup)。
- **Backstage**：统一权限名和决策协议，由资源插件执行对象规则并把条件下推数据查询。值得借鉴单对象规则与列表过滤的配对以及模块接入契约；不需要因此改成本项目之外的技术栈。[资源规则](https://backstage.io/docs/permissions/plugin-authors/03-adding-a-resource-permission-check/)、[分页过滤](https://backstage.io/docs/permissions/plugin-authors/04-authorizing-access-to-paginated-data/)。
- **DefectDojo**：可参考父级资源向下授权的边界；其当前官方OSS/Pro权限能力有区别，不能拿Pro细粒度RBAC证明开源版已提供整套能力。[OSS授权](https://docs.defectdojo.com/admin/user_management/os__authorized_users/)、[3.0升级](https://docs.defectdojo.com/releases/os_upgrading/3.0/)。

因此，保留自己的工单/风险领域规则并不等于重复造引擎。**应复用授权基础机制，平台自己定义安全业务语义**；避免另做通用图查询器、自创策略语言、全套身份治理产品。字段/流程/对象规则仍需项目自己负责，任何引擎都无法从代码自动推导正确业务边界。

## 6. 分阶段处理，保持既有核心排期

1. **P6.1及本地首版适用部分**：按[底座详设](../phase6/foundation-iam-module-detail.md)与[层级合同](rbac-inheritance-and-cascade.md#8-层级关系场景合同与实施归属2026-11-15候选)完成C04/D03/D06、必要薄UI和CE共同用例接线。复用C04已定代际、资源revision及安全原因码，收口可信事实来源和单对象/列表语义；不新增独立策略版本库/分发系统，不先建万能`AuthorizationPort`体系或把全部条件搬进Casbin表达式。
2. **逐模块详设检查成本**：出现多个模块复制相同规则、策略需独立发布/审查、类型/组织规则持续增加，或聚合查询开始难以保持一致时，就可评估策略引擎。出现跨对象授权继承/共享、嵌套组、跨服务共同关系决策时，评估关系引擎。无需等关系成链失控，也不以“第5种资源/第10个服务”机械定门槛；普通组织树本身不强制引擎迁移。
3. **命中需求后单资源对照**：用工单/风险中一个真实复杂资源比较当前实现与候选；复用独立的预期案例，包括只读抄送、组织/类型交集、多来源撤权、HIER场景、List/Count/Search/Export、分页、撤权可见时间和不可用行为。记录事实同步、关系变更与业务提交的竞争窗口、延迟与维护投入。可先影子比对后逐域迁移；未达标保留当前实现。

未来公共调用合同继续表达主体、能力、目标、受权范围与结果，不向私有adapter暴露Casbin/OpenFGA策略结构、tuple或业务库。`Resource.Authorize/GetFilter`是现有接缝，但`Filter.Where/Args`绑定SQL；换引擎仍可能改列表组合和关系同步，**不承诺零沉没成本或仅换一个adapter文件**。接口兼容目标与实际发布合同另按[UAA接入](uaa-governance-integration.md)及[共同调用合同](capability-invocation-contract.md)验收。

本轮不新增WBS父包、不调整工程日或日期，也不把外置引擎PoC暗含进C04。命中真实需求后，先明确对照范围、净估及替换/扣重再排入后续波次；现阶段业务政策待决仍按各模块登记，不要求所有者现在在模型名词中选一个。

## 7. 证据与效力

本轮完成官方文档/源码静态调研及现状设计对照，未部署候选引擎、未做真实数据规模基准、未联调公司UAA。不能声称已证实某工具在本项目最快、最安全或维护成本最低。当前AUTH/RECHECK等产品缺陷仍开放，旧产品测试通过不验收本页未来方案。

本页纠正`design-decisions.md` §12/25/26、`phase2/11-authz-architecture-review.md` §9及`standards.md` §7中有关“不演进/逻辑上不可能/无沉没成本”的绝对论证；保留当前部署与授权边界。旧评审和触发表保留其历史日期，选择工具以本页有界复评及后继实测为准。

## 8. 差距补齐动作与实施计划（所有者要求落档）

**差距判断**：当前组织/工单的规则表达没有明显代差；已确认的授权正确性和多出口一致性必须补齐；通用策略/关系引擎的工具与扩展能力差距明显，但本项目不以复刻通用引擎为目标。安全水平、吞吐与维护成本没有同条件对照，不能给百分比或“落后几年”。当前功能资格已复用Casbin，自研主要是领域规则、身份/组织治理和接线。

所有者本轮授权将建议纳入设计和计划，**没有授权开发或更换引擎**。下列是既有包的有界动作索引，不是新增7个父包；机器权威在[总账的authorization_gap_closure](../planning/planning-backlog.json)、实施证据状态在[成熟度](../planning/planning-design-status.json)。

本节编号按[文档命名空间](../documentation.md#引用编号与命名空间)限定：`AUTH-GAP-*`是本节改进动作，`AUTH-*`是权限专项的缺陷；`C04/D03/D06/CE01`等为总账父包。`S1/R2/TD03/TD05/MH02/Q1`等专指[本地净估册](../planning/local-core-release-estimate.json)的WBS行，映射到父包不重复计价；后文局部短名沿用此定义。

| 动作 | 补什么、怎样算补齐 | 主责与完成窗口 | 估算关系 |
|---|---|---|---|
| **AUTH-GAP-01 来源与撤权** | 修已在册的停组/父角色失效、直接与有效编辑分轨、任职往返及授予边界；撤一路不误撤其它合法来源，撤权先提交旧决定不得再提交 | C04/C05/D06；首版本地S1及各域实际启用前。UAA来源由P/R后继 | 复用FND/HIER/AUTH/RECHECK，不重复立修复项；开工拆子估 |
| **AUTH-GAP-02 多入口同规则** | CE01列能力、当前入口与唯一用例/例外；CE02实际接线，HTTP/内部/worker/PAT等有允许、拒绝和故障对照，签名不代替动作资格 | CE01/CE02；首版消费入口先验，S/D/L与后继MCP按原波次接入，新入口启用前必验 | 共用机制只计一次，不把全库存迁移塞S1或推到P9最后重写 |
| **AUTH-GAP-03 单条与集合一致** | 同一有限数据集验证详情可见集与List/Count/Search/Export、字段/关联投影一致，先过滤再分页；未启用出口显式拒绝，不留旁路 | D03及工单/风险/AL/消息所属包；首版实际出口、后续字段ACL随LS01等原包 | 分别归S1/R2/TD03/TD05/MH02等本域，Q1只整合接缝 |
| **AUTH-GAP-04 独立预期、回归及安全解释** | 复用既有场景ID，记主体/来源/对象/动作/独立预期/适用入口，先红后绿；规则变更关联这些ID。用安全reason与request/operation引用定位，授权管理面显示来源；普通人不获隐藏对象/成员信息 | C04/D03/G04及必要薄UI、既有OBS接缝；随每个授权变更批交，不等全站U06 | 最小索引/原因与既有用例扣重，不建新DSL/why图/独立规则分发或审计控制台 |
| **AUTH-GAP-05 单实例性能证据** | 记录合成/代表数据量、角色/组织扇出、并发、查询数、列表过滤、守卫等待/持锁和池占用；C05另按Bearer/Refresh/文件cookie/PAT对照认证安全读路径变更前后的QPS、p50/p95延迟及每请求DB/Redis查询数。模块开工定代表负载与预算，正式前按所选形态验，不以放松当前撤权判断换性能 | G04/C04/C05与E09适用硬出口；本地正式前 | 复用已有性能基线和生产形态验证；显著优化另拆净估，不预判性能差或先加跨请求安全缓存 |
| **AUTH-GAP-06 多实例一致性证据** | 实际启双副本前验落后策略、重载失败、撤权并发、重启/恢复及旧副本拒绝；单实例首发先验本进程失效与恢复 | E09/C04，UAA P/R按所选拓扑消费；P6.3列形态缺口，P7及实际多副本启用前复核 | 不提前建设HA；选择多副本时核专项子估，不假定E09原价覆盖任意拓扑 |
| **AUTH-GAP-07 引擎必要性复评** | 用重复规则/查询/治理或关系复用的实际证据裁定“保留当前”或“提出单资源验证”；若验证，只选一个资源、一类主要问题、一个候选，比较相同预期、列表、撤权/提交窗口、故障与维护投入 | 本地核心首次试用反馈安排一次；P6.3出口登记，P7出口给当前裁决；E07在P8/P9运行复评继续承接，不无限挂触发条件 | 这里仅排复评决策；实际PoC/迁移须另立净估和依赖后入总账，不暗含C04/E07，也不默认同时上策略/关系引擎 |

**最小质量实现**见[底座§3.7](../phase6/foundation-iam-module-detail.md#37-差距补齐的最小验收组织)。沿用当前测试设施，把预期数据、领域测试和入口测试关联起来即可；不新增通用策略管理模块。AUTH-GAP-05/06目前是证据缺口，不是已测出的性能/一致性差值。

**不追赶的通用产品范围**：任意图关系查询/全资源授权搜索、运行时通用策略编辑器、自创DSL、独立PDP集群与另一套完整IAM。业务场景将来需要时优先复用成熟工具；这不免除本平台必须定义的角色来源、动作职责和数据边界。

**工期处理**：多数动作细化已有正确性/测试/生产门，当前不新增父价、依赖或日历承诺。归包不等于零工作量，也不证明原C04或S1范围一定足够；第一批授权模块开工前，将适用动作与原WBS/测试逐项扣重、拆子估，如有净增先更新总账和首版候选再开工。当前工程量只读下方唯一来源，仍待实绩校准；未计实际外置引擎PoC/迁移。HIER-D01和两操作审计政策已由F017确认；真实名单、保留参数仍按原窗口绑定，不由测试矩阵暗中拍板。
