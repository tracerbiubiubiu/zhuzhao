# 权限底座第一包：范围与A批接口、验收设计

日期：2026-11-15；基线main `610c1d76`。状态：**设计讨论与实施切片候选，未改业务代码、未实施修复**。所有者要求一起收口、先不要改代码；本页不构成开发、推送、合并或部署许可。后续只读本页、[底座详设](foundation-iam-module-detail.md)及所用源符号，具体冲突再回查历史，不再重审全部Phase6–9。

## 1. 本轮已确认的两项政策

### HIER-D01：停父组织，暂停子树的组织派生资格

所有者答复：**暂停整棵子树的组织派生授权**。B部门下C小组即使仍`status=1`，停用B后，通过B或C等子树组织得到的功能角色、scope和org owner/admin委托均暂停。不批量改子节点status、不删成员绑定与业务历史；重新启用B时按当前组织状态、任职期限和绑定重新求值，已撤/过期来源不复活。

独立直接角色、子树外有效组织，以及业务创建人/处理人/CC等关系仍按自身领域规则校验；保留来源不等于绕过L1、类型范围或动作约束。组织owner/admin不是可绕过停组的独立业务owner。父角色继承、父组织角色是否向子组授予，是另外两个问题：原“org_roles只授该节点直接有效成员”不变，不能把停用传播误作授予继承。BK-20软删已结票档案政策继续单列，不顺手改变。

停用前提供受影响组织/授权来源的有界预览；暂停是权限求值结果，不复制状态，不给每个子节点造额外revocation写主。HIER-07与FND-14复用父停用/子启用、直接/其它来源对照、重新启用前已撤或到期不复活的预期。

### IAM-AUDIT-D01：两条改权命令与本地成功审计原子提交

所有者答复：**两项均要求本地审计成功，否则回滚**。限`UserService.SetRoles`的直接角色授予/撤销、`RBACService.AssignMenus`的角色菜单/功能权限修改；本地必要成功事实写失败，本次业务变更不得提交。业务写失败不记录成功，拒绝尝试与成功事实分开。

本地业务与审计已提交，仅外部collector不可达时，保留本地事实、持久补送，不重做改权；策略重载/会话清理失败按原receipt/恢复设计显示待生效，不伪装已全实例生效。commit结果未知先查原operation/receipt，不能盲重做。此确认不把全部GET诊断日志改成强审计，也不自动批准更多改权操作、完整审计控制台或真实人员/保留期。

两答来源及对历史待答的后继见[后继索引F017](../review/planning-followups.json)。政策已定与产品已实现分别记录。

## 2. 三个IAM子批，先完成A的开工准备

| 子批 | 范围与当前证据 | 完成应证明 | 边界 |
|---|---|---|---|
| **A 角色授予和编辑正确性** | AUTH-07、RECHECK-01/02；`OrgService.Bind/UnbindOrgRole`、`UserService.GetUserRoleIDs/SetRoles`、UI用户角色/组织编辑弹窗 | 普通org:create不能借组织赋强角色；admin仍守可授档位。角色编辑只写直接绑定，组织/继承不固化direct，停用绑定往返不丢；pending/error/迟到响应不清空或串写 | 不新增委托赋角角色/全来源台，不改effective鉴权回直接角色；并发撤权窗口须等C验收才关闭 |
| **B 有效组织来源与任职保存** | AUTH-01/03、RECHECK-03、已定HIER-D01；`RoleRepo`两种有效查询、scope/组织委托、`OrgRepo.SetUserOrgsTx` | 父/本组停用跨菜单/API/scope/委托一致，独立来源保留；同org集合保存不重置admin/group/期限，停父角色不再贡献菜单 | 不做完整HR/UAA；组织集合差量保留既有primary缺省/null语义和owner双轨清理；Move/停用竞态由C共同写侧闭合 |
| **C 当前授权、提交及强审计最小闭环** | C04工单Create/Update/Transition及其repo邻域；相关IAM/组织/参与写方；两审计命令；FND-02/03/04/06/11/12/17 | fresh/DBTX不信守卫前context；撤权先提交旧决定零效果；L1 revision+enforcer配对原子安装，单实例失败/恢复可验；两改权审计写失败零提交、collector失败只补送 | 不能只给三个工单方法加读锁而漏对应改权写方；不扩全仓DBTX、独立PDP或完整HA。完整双副本、HR、UAA及其它领域按原包窗口 |

A接口/验收已细化；B的来源与差量算法见[底座§3.8](foundation-iam-module-detail.md#38-b有效组织来源与成员集合的准确接线)，C的写方、pair、两操作审计与旧wire方案见[§3.9](foundation-iam-module-detail.md#39-c共同屏障策略pair与两命令审计的准确纵切)，任务窄修复见[任务§8.3](task-activelist-message-detail.md#83-存量任务窄修复准确接线与参数归因)。这些均为源映射设计候选，未实现。A先作为第一可实施切片，B/C保持有界后继，不要求一次开发整个IAM。C05账号停用/会话收敛的本地必需出口仍按原包交付，不能因C完成就声称完整会话恢复链已验，亦不将它免费吞进C子估。两项政策先固定，具体SQL、fresh端口、锁序实现、错误兼容与测试夹具由维护者在既有合同内处理，不逐条询问所有者。

**现有任务入口的安全修复另列**：AUTH-02维护action资格/保留期、AUTH-06可信created_by归因归原任务/消息包，OBS只消费其可信actor，在开放业务任务前修；不等CE03/MCP。主仓Submit/CreateJob/UpdateJob/Trigger/Retry/最终执行逐项清单与TR归因合同另作窄批，不偷偷塞进IAM数据查询，也不把TR现有“body优先”测试当成可信归因证明。

## 3. A批的具体前后与验收

### 3.1 已核对的HTTP合同与兼容范围

只读基线：主仓`610c1d76`，UI `875df0bf`。下表是**现有wire及拟修语义**，不是已发布新接口；成功信封沿现有`response.OK`，表内只列`data`。客户端发送ID仍为十进制字符串，不用JS Number转换大ID。

| 现有入口 | 输入／成功data | A批处理与既有授权 |
|---|---|---|
| `GET /api/v1/users/:id/roles` | 路径用户ID；`{role_ids:["…"]}` | 从effective误用修为直接绑定编辑值，含仍绑定的停用、未软删角色；JWT＋L1页面授权＋原ensureVisible保持。**这是返回集合语义修正**，不能写成完全无行为变化 |
| `POST /api/v1/users/roles` | `{user_id:"…",role_ids:["…"]}`；`null` | 整体替换仅本地直接角色；保留`user:assign_role`及原目标管理/系统保护，按§3.2区分增保删 |
| `POST /api/v1/orgs/roles/bind`、`/api/v1/orgs/roles/delete` | 两者均`{org_id:"…",role_id:"…"}`；`null` | 现处orgDelegated selfService组，跳过Casbin路由enforce但仍有认证/非零有效角色检查，**必须Service核资格**；不能虚构已有按钮权限码替代修复 |
| `GET /api/v1/users/:id/orgs`、`POST /api/v1/users/orgs` | GET `{orgs:[…]}`；POST `{user_id:"…",org_ids:["…"],primary_org_id?:"…"}`，成功`null` | A只修弹窗读取/提交的状态正确性；B修成员集合差量。primary缺省/null及owner清理原语义不借此变更 |
| `GET /api/v1/roles` | 角色行数组，含`id/name/code/status/priority/parent_id` | 现为无分页未软删角色列表，含停用；非superadmin过滤superadmin，非无条件全系统目录。复用字典/停用tag，不要求来源台或新接口 |

HTTP锚点为`internal/router/router.go`的`New`、`internal/handler/user_handler.go`的`GetUserRoles/SetRoles/SetUserOrgs`及`internal/handler/org_handler.go`的`BindOrgRole/UnbindOrgRole`；DTO为`internal/model/user_request.go`的`SetUserRolesRequest/SetUserOrgsRequest`。actor取可信认证context，不接收body代填。现GET对未知数值用户可能回空集，不能写成已404；本批保留并补兼容断言，不借编辑语义修复统一重写全部对象存在性政策。

**保留输入兼容**：当前role_ids允许字符串/整数元素，缺失、null及`[]`均可能进入空集合替换；A不暗改服务端旧输入解析。UI始终显式发字符串数组，只有**成功读到有效数组**后才能明确清空。客户端收到缺role_ids/orgs、类型错误或不合法ID的200载荷是读取失败，不得以`?? []`构造成功空快照。若以后收紧旧请求的缺失/null，须另列行为变化及消费者回归。

正常无绑定的当前响应静态可证为`{role_ids:[]}`和`{orgs:[]}`：前者经`internal/handler/user_handler.go`的`int64IDsToStrings`初始化非nil数组，后者经`internal/repository/org_repo.go`的`GetUserOrgs`及固定pgx v5.13.0的`CollectRows`返回非nil空slice；严格读取校验不会把当前合法空集合挡掉。A-05仍须补真实空集合HTTP断言，不能把源码核对当运行证据。

### 3.2 直接角色编辑：保留不等于重新授予

三种查询分开：①直接绑定编辑集合；②有效direct/org/inherited鉴权投影；③IAM管理档位所用的启用直接角色。只新增/复用窄编辑查询，不把`UserRepo.GetRoles`改成包含停用，也不改effective鉴权回直接角色。

服务端以当前直接绑定`old`和请求`requested`计算`retained/added/removed`，仍为全量替换，不自动把缺项并回后声称保存成功。`old`须取完整user_roles绑定事实并辨认其角色状态，不能用过滤后的GET或客户端字典替代；发现历史软删孤儿且本次无法安全处理时，服务端整笔拒绝、零副作用，UI保留草稿，不让被过滤的旧ID在替换中悄悄消失：

- **新增**：角色存在、未软删、启用，且满足原`canAssignRole`；不能把别处派生或仅字典展示的角色当作已直接绑定。实际比较是role.priority≥actor有效直接档位，即可授等档；`canManageTarget`对非自身目标才要求严格更强。两者不得在文档或实现中混淆。
- **保留**：正常启用角色继续原可授档位检查。停用角色只允许保留服务端证实已存在的同一直接绑定，不贡献菜单/权限/管理档位，不得借此新增角色、换来源或恢复有效性；客户端声称“旧绑定”不是证据。
- **解除**：目标用户仍走原ensureCanManage（含self分支），当前非软删角色另走canAssignRole（允许等档），不套角色CRUD的严格管理比较。停用本身不禁止合法解除，也不意味着较低档位管理员可以删除高档位旧绑定。无权移除返回原有403并使整笔替换零效果；不是部分保存。**明确清空仅在每项解除均合法时成功**，最后superadmin事务守卫继续执行。当前SetRoles没有通用is_system/种子用户角色冻结检查，不虚标已有保护或借此新增冻结政策。
- 读取失败、字典不可见/未知或历史软删孤儿不能被称作“停用”，不能静默删关系。若不能证明可安全往返，阻止本次表单整体覆盖、保留当前草稿并提示受权核查；A不新建通用历史授权清理台。旧软删关系的识别/处置仍须按实际repo及原受审清理边界核验，不能以重新启用角色解锁。

例如admin10给目标保留既有停用custom5：保留仅维持原关系，不新增当前权限；若要删除该custom5，仍须更高资格处理。admin10新增custom5则拒绝；新增启用custom10还要通过对**目标用户**的管理检查，不能只凭等档可授就管理任意同级用户。此规则落实既有授予/管理边界，不把priority升级为通用委托模型。

### 3.3 组织赋角与解绑：两条路径都校验

仅为`BindOrgRole/UnbindOrgRole`使用明确的系统级管理资格，再按有效直接角色核既有档位；不全局改写`isGlobalOrgAdmin`而影响工单或其它组织动作。普通org:create、组织owner/admin任职或组织派生业务角色均不自动成为全局IAM授予权。若真实部署存在额外委托，先列差异再裁决，不阻塞当前提权路径修复。

绑定继续拒绝系统角色和停用新角色，重复绑定保持既有成功语义；解绑不能复用“新增必须启用”的检查，否则停用旧绑定无法合法解除。解绑不存在关系沿既有404，不发明幂等成功。组织停用与软删分开：暂停其授权贡献不等于禁止受权预配置，不能把status=0伪装成不存在。历史软删孤儿走受审清理，不以重新授予修复。

错误沿现映射：无资格403，超可授档位403／30009，目标不可管理403／30010，角色/组织不存在404，非法系统/停用新绑定400；若实施需新增稳定reason/码值，先在errcode账登记。删除/绑定竞争、权限读后被撤及角色并发变更由C共同屏障收口，A不能宣称已关这些窗口。

### 3.4 UI只需要成功快照与局部状态

实际消费方是UI的`src/api/system/user.ts`中`getUserRoleIdsApi/getUserOrgsApi/setUserRolesApi/setUserOrgsApi`、`src/api/system/role.ts`的`listRolesApi`及`src/views/system/user/index.vue`的`openRoles/submitRoles/openOrgs/submitOrgs`。

1. 每次打开/关闭递增局部请求世代，清旧草稿/错误、树勾选、primary；成功快照绑定`{userId,seq}`。角色读取和必要字典均有效、组织树和成员读取均有效且仍匹配，才进入ready。树赋数据后等渲染再回填，并再次核世代；已绑组织ID必须都能在树中定位，不能让树组件自动取交集丢项。
2. 字典载荷有效不等于当前操作者可授全部角色；最终资格在后端。字典至少验数组、唯一精确ID、name/code字符串和status=0/1；不能将不安全数值ID转字符串后冒称精确。字典失败（含失败后残留缓存）或已绑定ID无法正确回填时先禁整体保存、给重试/受权核查，不把缺项猜成停用或无绑定，不透露隐藏角色信息。角色界面说明“此处编辑直接授予；组织/继承来源另算”，A不凭现有ID猜来源明细，也不把来源管理台当必做。
3. 按钮与submit函数双守卫：当前可见、目标和快照匹配、已ready、无加载/确认/保存。首个await前取得提交锁，拍下目标ID/名称/世代和数组副本；组织确认返回后再核目标/世代。确认取消释放锁，不发请求。
4. POST只用拍下payload；迟到成功/失败只能更新同代弹窗，不能关闭新目标或重启其保存资格。GET/POST及确认的finally、取消和解锁也核原目标/世代/提交令牌，旧回调不能释放新目标的锁或改其loading。失败保留本代草稿并明确反馈；响应丢失/未知不等于零提交，不自动重发。无需新前端状态框架或为此增加幂等字段。

### 3.5 独立预期及真实测试入口

下列A-01–08为**待实现验收场景ID，不是已存在测试名**；复用FND-16/19/21，不再追加另一组全项目需求。先写业务预期、修前有效红，再修后绿。错序/并发/变异按原至少8轮纪律，真实HTTP、PG、浏览器与函数替身证据分列。

| 场景 | 独立预期 | 复用点与缺口 |
|---|---|---|
| A-01 三种角色分轨 | direct＋org＋parent读取后原样POST，direct集合不新增；撤组织/父边后派生资格消失，原direct仍在 | `internal/service/user_roles_route_integration_test.go`的`TestUserService_GetUserRoleIDs`实为Service测试，不是HTTP；需补真实GET→POST及PG来源断言 |
| A-02 停用绑定增保删 | 保留既有停用绑定不丢且不授当前权；新增停用/强角色拒绝；无权删除强旧绑定整笔拒绝，合法解除成功；软删孤儿不因GET过滤而静默丢失 | `internal/service/d2_integration_test.go`的`TestUserService_SetRolesHigherRejected`、`internal/repository/role_repo_integration_test.go`的`TestRoleRepo_DisabledRoleExcluded`及`internal/service/d2_more_integration_test.go`的`TestUserRepo_SetRolesTxDedupAndSoftDeleted`；新增delta与零效果对照 |
| A-03 组织入口越权 | custom60仅org:create→custom5、admin10→custom5均拒且无绑定/权限副作用；合法弱角色可绑且能合法解绑停用旧项 | `internal/service/org_delegation_integration_test.go`的`TestBK12_OrgRoleBinding/TestBK12_DirectVsExpanded/TestBK12_BindOrgRole_OrgGuard`；新增实际selfService路由组合 |
| A-04 原保护与wire | 最后superadmin/受保护目标不能越过；等档可授与目标严格管理分开；空集合、重复ID、字符串/整数旧兼容、大ID及未知用户GET原空语义分别验 | `internal/repository/user_repo_integration_test.go`的`TestUserRepo_SuperadminGuard/TestUserRepo_SuperadminGuardLockSerializes`与`scripts/acceptance-phase1.sh`的admin授superadmin用例；不能把后者当AUTH-07已覆盖 |
| A-05 加载/坏载荷 | 角色或字典、树或成员任一pending/error/缺字段/坏ID时零POST；重试全成功才恢复保存，成功显式空数组仍可按权限清空 | UI现无用户弹窗专项单测；补API载荷与局部状态测试，按钮拒绝由Playwright验 |
| A-06 换目标/树错序 | 甲慢乙快、甲迟到/乙失败、关闭重开均不串目标；树渲染后精确勾选，缺项不被静默覆盖 | `src/common/hooks/test/useCrud.test.ts`的deferred方法可借用；节点勾选仍需真实浏览器 |
| A-07 保存/确认重入 | 连点/直接重复submit/重复确认至多发一笔，payload目标与数组固定；旧POST结果不关新弹窗，旧finally不解锁新请求，失败保草稿 | `e2e/s3-user-admin.spec.ts`已有组织保存/二次回显骨架；`e2e/s5-storage-smoke.spec.ts`可借请求失败注入方法 |
| A-08 既有集合和有效性 | A不改变组织primary缺省/null；B未验收前不宣称组织任职原样保存无损；编辑修复不让停用角色进入effective/IAM档位 | 主仓成员/角色既有回归＋UI `src/api/ticket/idWire.test.ts`大ID方法；B、C的完成证据不可由A代销 |

既有测试符号的源文件由实施前`rg`定位；新用例须写入受控测试，ignored审计探针不能充当产品门禁。UI现Vitest为node环境，不预加jsdom或组件测试依赖；局部状态/适配器可用现有设施，真实按钮/树/提示由Playwright验。

### 3.6 实施路径与后继门

A预期允许路径仅主仓上述user/org service及其必要窄repo/handler/DTO文档和专项测试，UI上述user API/角色字典校验/用户页及专用测试；以实际PR声明为准，不授权改所有IAM、全部公共组件或已有工作流。现有`SetRolesTx`可复用但不能用DELETE重插掩盖来源/停用丢失；是否需要差量SQL由保留与零效果测试决定。A无已知必须DDL，不预占迁移号。

共同守卫、两操作强审计/receipt与恢复仍归C；A可独立评审和验证编辑修复，**不可单凭A合格宣布权限底座或正式发布门已完成**。B、C05原必要会话出口及任务窄修复仍各按原包验收。A-01–08属于下方既有4–6日WBS内部，不新增父包或把测试再计一笔；实际成本超界时先回填子估/扣重，不挤掉已确认审计门或按日历倒推通过。

## 4. 本包开工前交接清单

- 冻结A的允许路径、真实测试入口、数据/账号归属和当前五仓组合；工程隔离G01及原适用门禁不可省，不占共享dev/test或E000001做场景实验。
- 给A的源函数、独立预期与测试符号对应表；维护已有直接角色管理档位、self分支和最后superadmin事务守卫，遵守测试账号纪律，明确本批尚未关闭的C类竞态；不把其它命令的is_system保护误写成SetRoles已有检查。
- B只展开实际采用的祖先有效性与任职差量；C逐项列共同屏障的读/写入口、事务与审计接入点。若发现需改变已确认业务边界，给具体场景再问，不自动扩成通用权限平台。
- UAA-C先留稳定主体/资源/source边界，完整P/R后继；公共mock、独立模式、私有adapter隔离和测试先行保持。无需现在提供公司文档或真实凭据。

## 5. 估算及当前状态

基于main源码及既有测试的只读子估如下，单位为单席有效工程日，包含本批剩余设计、有效红例、实现、专用测试、评审和正常修正；置信中低，未按截止日倒推。

| 子批 | 有界子估候选 | 已有基础与新增成本 |
|---|---|---|
| A | **4–6日** | 复用SetRolesTx、可授档位及UI骨架；新增授予边界、直接/有效分轨、停用绑定往返、弹窗成功快照及真实回归 |
| B | **4–7日** | 复用组织事务/owner清理；新增祖先停用跨有效角色/scope/委托、恢复按当前关系计算、任职属性差量与对照 |
| C | **12–19日** | 复用工单/角色事务；当前授权多在Begin前持pool读取，Casbin尚无revision pair，请求末AuditLog写也不在业务事务内；新增相关写方闭合、fresh DBTX、快照加载/恢复与两命令本地强审计 |
| 现有任务窄修复 | **4–7日** | 动作Registry/提交凭证/回调栅栏已有；新增维护资格全入口、受控保留期与核心/TR/消息可信归因，含旧body优先合同/测试修正，不是完整CE03 |

**这些是有重叠待扣的包级子估，不能相加后追加到总工期。** A/B与C04/D06/必要薄UI分配对表；C与S1本地护栏、S2/R01及OBS02的事实/两命令/恢复部分只计一次；任务修复与原任务包扣重，Q1只整合接缝。OBS02后继查询/导出/治理UI等剩余范围不自动前移。

A的当前WBS为：授予边界/合同1–1.5日；直接绑定与停用往返1–1.5；必要弹窗0.75–1；真实路由/PG/浏览器回归0.75–1.25；评审和文档0.5–0.75，合4–6日。它是第一个待共同审阅的切片；不据此自动启动开发。

原S1的5–8日含UAA-C合同2–3日，本地护栏仅3–5日，**不足以继续代表这里已明确的全部IAM范围**。旧68–104日仍保留作历史候选参考，当前需要重估其IAM分配；尚未冻结C写方清单、同快照adapter/事实投递的最小纵切及原包扣重，不能发布一个貌似准确的新全版总数。下一步先收口A接口/预期与费用分配，用首批实际速度校准B/C，不重估整个Phase6–9后才行动。

目前只关闭上述两项政策待答。当前AUTH/RECHECK缺陷仍开放，A/B/C未开发，未运行新业务用例；子估不代表准确DTO/DDL、跨仓合同和运行证据已验收。没有新父包、引擎迁移或日历承诺；更细成本和实施计划只在实际消费窗口校准。
