# 权限控制：现状、实现与后续计划专项复核

本页保留 11 月 15 日看到的问题和证据。要查某条权限问题后来修到哪一步，用 AUTH 原编号查看[项目控制台 §6](../11-project-control.md#6-当前健康状态遗留问题追踪)。例如 AUTH-01 要查停用组织后，经这条组织关系取得的权限是否失效，以及其它合法来源是否仍有效。

> **2026-11-15政策后继，不改本报告时点事实**：[第一包/F017](../../phase6/iam-first-batch-design.md)已确认父停用整子树组织派生资格暂停及SetRoles/AssignMenus本地审计失败回滚。当前产品缺陷仍开放；原净估作历史候选，IAM子估待扣重，不表示已开发或已验收。

日期：2026-11-15。状态：**审计及设计补充，产品缺陷仍开放**。主仓运行源码基线为 main `aa7451739bcecdea5afe288d2b7e19f39b55f142`；设计对照为本地候选 `c7759a27e50ada0d6085d06b5799984ee40bf97c`。UI `69fbad99`、AL `15e93682`、TR `b0906b83`、utils `de3cd0a6` 为本轮跨仓源码基线，未把本地设计稿当作已合 main 的实现。

结论：**保留 Casbin L1＋领域资源 L2/L3，先修具体缺陷，再按已有阶段接入统一调用和 UAA。** 确认6组当前问题（3组P1、3组P2），另有1项条件性PAT加固；AUTH编号不是当前可利用漏洞数量。当前门禁可以验证既有功能，却没有覆盖本轮所有撤权、动作职责和凭据故障反例；不能由历史矩阵、覆盖率或 acceptance 绿色推断权限没有问题。本轮不改业务源码、不迁移数据、不变更线上授权。

## 1. 范围和证据边界

- 本地登录、AT/RT/PAT/FileAuth、账号停用/会话撤销；直接角色、组织角色、继承角色、菜单和权限码投影。
- 组织任职、owner/admin 委托、scope；工单读写、CC、关联、列表/计数/导出、备注、附件、转换快照。
- 网关与 selfService、AL/TR 服务身份、任务动作/结果/维护操作、消息规则和审计职责。
- 对照 C04/C05/D03/D06/D12、CE01/CE03、X01、UAA-C/P/R 和 OBS 的现行设计候选。

采用源码追踪＋冻结 main 的隔离真实 PG18 探针＋原产品完整门禁。专属数据库只含合成数据；未操作共享 dev/test 账号、真实日志或公司系统。工单探针直达真实 service/repository，角色来源部分为 stub，不冒充完整浏览器或 JWT 路由验收；IAM 探针另验证真实 Casbin 中间件。真实公司 UAA/消息、浏览器所有交互、两实例撤权竞争和全部可能角色组合不在本轮动态证明范围内。

## 2. 当前实现需要修正的事项

### AUTH-01 · P1 · 停用组织后派生授权未停止

场景：乙仅因所属虚拟组获得功能角色，或因组管理员/owner 身份管理他人的票；管理员调用现有 `OrgService.Update(status=0)`。组织保留成员关系，乙仍得到原授权。

源码锚：`internal/repository/role_repo.go` 的 `GetEffectiveRoleCodes/ListRoleIDsByUserID`；`internal/service/ticket/scope_resolver.go` 的 `pgxScopeResolver.resolve`；`internal/service/ticket/resource.go` 的 `delegatedVisibilitySQL/Authorize/GetFilter`；`internal/service/org_delegation.go` 的组织资格 SQL。成员到组织的有效性判断缺组织状态条件，问题跨 L1、L2、L3，并非只缺一个前端隐藏。

隔离结果：真实停用前后，组织角色仍在，有该角色策略的合成路由均204；真实工单 service 仍允许组管理员读/更新他人票，列表及 total 仍为1。创建者依其独立身份仍可读，是正确对照。此项原已登记组织派生 L1 缺口，本轮扩展并动态确认 scope/委托路径，**不是重复新增三项缺陷**。

建议：C04/D06 与 D03 共用有效组织来源谓词，覆盖 group/all/透明读/本组与祖先委托，以及菜单和后台执行；停用撤掉该来源，不抹掉直接角色、其它有效组织或合法参与资格。**首次正式使用前修复**，不等真实 HR/UAA。软删组织的已结票档案权限是 BK-20 已接受的不同政策，不能顺手全局加 deleted_at 条件改掉；正式迁移该例外前须单独说明。

### AUTH-02 · P1 · 业务任务权与审计维护动作没有隔开

场景：按 Phase4 推荐预设给业务 operator 授 `task:submit`，且启用 TR/internal_jobs。该用户提交已注册的 `audit_archive`，并指定 `retention_days=1`。

源码锚：`internal/app/providers.go` 的 `provideJobsRegistry` 注册维护动作；`internal/handler/taskrunner_handler.go` 的 `Submit/failFastAction` 仅检查存在性；`internal/service/jobs_callback.go` 的 `JobsCallbackService.Execute` 校验快照/幂等后执行；`internal/service/audit_archive.go` 的 `AuditArchiveJob.Handle/archiveTable` 接受任意正保留天数，写 JSONL 后删除在线 `audit_logs/policy_evaluation_logs` 行。

这是获授业务任务权后的职责越界，**不是未认证访问，也不是永久抹除归档文件**。默认角色空权限和 internal 默认关闭限制了触发条件；原设计又明确审计/system 域由系统管理员管理，不能把普通任务提交权视为维护授权。

隔离实测：真实 CasbinAuth 仅准 POST tasks、审计读和 jobs 管理均403；真实 Submit/提交凭证/验签回调/生产注册表执行归档成功，两表各一条10天旧行从在线表移除、各一条新行保留，两个 JSONL 保存旧行。默认180天对照不移动旧行，重复回调不重复归档。只有 TR 队列由验签 httptest 上游代替，身份及角色 fetcher 为夹具；没有声称跑了真实调度部署或无 JWT 身份即可触发。

建议：先给维护 action 加明确允许主体和受控参数策略，Submit/CreateJob/UpdateJob/Trigger/Retry 及最终执行均验原动作资格；系统 cron 走固定维护身份，不能因无人值守省校验。保留期不得由普通任务参数任意降低。归 C04 与 OBS 必需保护，**开放业务任务入口前完成**；CE03 后续接共同通道，不能等完整能力平台才堵旧入口。

### AUTH-03 · P2 · 停用继承父角色后，菜单/按钮投影仍包含它

源码锚：`internal/repository/role_repo.go` 的 `ListRoleIDsByUserID` 递归及最终 SELECT，和 `GetEffectiveRoleCodes` 的父角色启用条件不一致；消费者为 `MenuService.GetUserMenus/GetUserPermissions`。

隔离结果：真实 `RBACService.UpdateRole` 停父角色后，L1 有效角色已排父，合成 API 204→403；菜单仍有该父角色贡献的页面及 `button:ticket:create`。这是**UI/权限投影与后端失配，不是该 API 仍可越权**。

建议：C04 复用同一当前有效角色集合生成角色码/菜单/权限码，补父角色停用、软删、组织停用/任职过期以及另一来源仍合法的反例；不用扩大客户端缓存来掩盖差异。核心试用前修正。

### AUTH-04 · P2 · 关联读取未复核对端当前可见性

源码锚：`internal/service/ticket/service.go` 的 `ListRelations` 只验当前票；`internal/repository/ticket_repo.go` 的 `ListRelations` 原样回双端 ID。

隔离结果：A/B 原均可见，建关联后撤用户对 B 的 CC；B 的详情/评论已404，但 A 的关联列表仍返回 B 的 ID、关系类型、创建人和时间。**不含 B 的标题正文**。`CreateRelation` 已检查双端 update，不能因此认为关联读取也安全。

建议：C04/D03 对现有 `ticket_relations` 按对端当前读资格过滤；不可见与不存在同形，不返回可识别占位或未过滤数量。以撤 CC/撤角色/跨组织、双向关系及合法关系仍可见作回归，不只保护未来风险关联。正式工单使用前修复。

### AUTH-05 · 条件性加固 · PAT 自定义随机源报错分支不安全

源码锚：`internal/service/pat_service.go` 的 `generateSecret/Create`：`rand.Int` 报错后逐位使用固定字母，仍落库并返回成功。

隔离故障注入：将专属探针进程的随机 Reader 替换为报错的自定义实现，真实 Create 成功，预测 secret 的 hash 可命中真实 PAT repo；真实 JWT(PAT)→SelfService→CasbinAuth→合成 handler 返回204。正常两次签发不同且不等于 fallback。**当前Go1.27.4默认 Reader 下 Int 不返回错误，未发现产品替换 Reader；底层随机故障也不能等同本探针的可返回错误。故不列当前默认路径下已证实可利用的凭据漏洞。** 此探针只证明可注入分支 fail-open，未声称执行真实业务 handler。

建议：C05 最小加固为删除固定回退；若保留可注入生成器则显式返回 error、失败拒签、零 token 行、无明文回包，不用弱随机替代。正常唯一性及注入错误各有测试。按 [Go crypto/rand.Int 的错误合同与默认 Reader 示例](https://pkg.go.dev/crypto/rand#Int)处理，随小修做即可，不拿条件性分支夸大紧急程度。

### AUTH-06 · P2 · 部分创建者字段由浏览器控制

源码锚：核心 `internal/handler/taskrunner_handler.go` 的 `TaskrunnerHandler.CreateJob` 与 TR `internal/handler/server.go` 的 `Deps.createJob`：透传 body.created_by 优先于已签名 Operator。TR 既有 `TestCreateJobCreatedByFallback` 正在断言该优先级。核心 `internal/handler/message_admin_handler.go` 的 `MessageAdminHandler.UpsertRule`、`internal/service/message_admin_service.go` 的 `MessageAdminService.UpsertRule` 也把请求 CreatedBy 用于新规则。

需要已具任务管理或消息写权；影响记录归因。**当前 HTTP 审计仍记录真实操作者，未证明可借此取得对象权限**。

建议：C04/CE01 在受信入口构造 DTO，created_by/operator 等服务端归因由可信 context 派生；TR 明确服务归因合同，不能把浏览器透传当服务声明。历史导入如需代录另设受控用例。OBS 消费可信归因，不能先凭这些旧字段构建“我的任务”安全边界。

### AUTH-07 · P1 · 组织管理粗门叠加组织赋角，能够取得更强角色

源码锚：`internal/service/org_delegation.go` 的 `HasOrgManagePermission` 将任意 `org:%` 菜单视为全局组织管理；`internal/service/org_service.go` 的 `isGlobalOrgAdmin/BindOrgRole` 复用此判定，未检查操作者可授角色档位；`internal/router/router.go` 的组织赋角路由在 selfService 组。`OrgRepo.BindOrgRole` 禁止内置系统角色，但允许其它更强 custom 角色。`UserService.SetRoles` 原有 `canAssignRole` 不能保护这条旁路。

隔离真实 HTTP：操作者 direct priority60，仅 `org:create`，普通 UserService.SetRoles 给他人授 custom priority5 被拒；其正常 PAT→JWT→Casbin→UserHandler.Update 起初403。PAT→selfService→OrgHandler.BindOrgRole 将 priority5 custom 角色绑到含自己的组织返回200；随后同一 PAT 的 UserHandler.Update 返回200，另一合成用户的 real_name 实际更新。绑定 admin/superadmin 的对照均400。**已证明 custom 角色提权，未声称能直接绑定内置系统角色或无需任何初始授权。**

原文档确有“org:* 即全局组织管理”的粗粒度选择，但组织赋角又声称“仅全局管理员”；本轮跨路径证明其组合超出正常角色授予边界，不能仅作为文案不一致待办。

建议：C04/D06 的小修先让 Bind/UnbindOrgRole 明确只允许现行内置系统级管理者，或经单独批准且具有同等授予边界的主体；单个 org 写权限不得替代 IAM 授予权。**所有获准主体（含内置admin）都须按直接角色复用现有 canAssignRole 的可授档位及目标角色状态，锁内复验；仅改 org 前缀或仅限admin仍不完整。** 系统角色仍禁绑；解绑须明确可管理对象的边界。禁止超出自身可授边界的自授、借成员/owner/继承来源绕过，不误禁合法管理员给自己所在组织绑定弱业务角色。其它组织自服务操作按动作区分全局功能权与本组织管理资格，移除 `LIKE 'org:%'` 的无限外延。**正式使用和向普通业务角色授组织写权前修复**，不等待 UAA。

## 3. 需要补清的设计边界

### 存量任务入口必须随敏感业务接线

旧 `phase3/16-external-integration.md` E-⑤明确选择共享队列全可见、dept 只是筛选、参数不含秘密；当前 Registry 实际只有 ticket。因而 GetTask/ListRuns/Jobs 等只有 L1 **本身符合旧选择，不直接报当前 IDOR**。

但首批风险/工单/AL 联动会携带受限参数与结果。D12 与 CE03 的首版本地子出口必须逐入口迁移：任务详情、runs、jobs/list/detail、cancel/retry、trigger/update、dead-letters、artifact/日志及 receipt；查询先过滤再分页/计数，写动作验当前职责。非敏感共享队列可以按批准 profile 保留；不能只保护新 operations 接口，旧深链仍全量透传。后台固定受限主体和执行前重验、首个敏感 AL 的 list/count/metadata/禁止未接线导出、消息预览收件权限已在现行详设，不重复建系统或算作新遗漏。

### 事务内“重新读角色”必须真的绕过请求缓存

`internal/service/rbac_service.go` 的 `GetRoleCodesByUserID` 会复用 `middleware.RolesFromContext`，这是现有同请求优化。C04 若在守卫后直接调用它，仍可能拿到守卫前角色。实现应有显式 fresh/DBTX 读取端口，同一事务展开三源；不得通过删一处 JWT 角色字段就宣称完成提交前重验。锁序、短事务、L1 revision 与 L2/L3 资源事实分开等现有方案保留。

### 已有计划中的失败窗口仍是首版本地必修项

`UserService.UpdateStatus` 先提交DB，再经`session_revoke.go`撤Redis；多次撤销失败时AT/FileAuth只检查Redis而非当前DB停用事实，PAT/Refresh另有DB状态读取。`RBACService.reloadPolicy`在DB提交后重载失败，也可能留下本进程旧策略；不必等多副本才出现。C05/FND-05与C04/FND-04已规划当前状态屏障、持久恢复和策略代际，**不是本轮新增发现，也未因普通门禁绿而修好**。需作为本地核心出口故障反例，不能只列“未来多实例增强”。本轮未重做这两类完整故障注入，按源码和既有计划承接。

### 当前授权与联邦来源不混用

独立模式继续本地账号/角色/菜单和三层校验；接 UAA 后仅受管范围由 adapter 输入身份及授权事实，核心仍执行 L1/L2/L3。外部同名 admin 不自动成为内置系统管理员。UAA 的账号/角色/菜单治理、单写主、三源撤权和失联行为已有候选，不因 mock 通过就声称企业兼容已验。真实公司合同、撤权新鲜度和激活配置仍在 P/R 窗口确认，不是现在新增的大改理由。

### 组织管理权限的剩余政策选择

具体场景：只授“创建组织”的甲，目前还可通过自服务 SetOwners 成为其它组织 owner；这与 AUTH-07 的提权根源相同。建议普通自定义角色的全局侧按现有 `org:update/member/delete/move` 对应动作逐项检查；本组织 owner/admin 仍按已批准职责管理，不另建一堆角色。是否保留某个“全局组织管理员”委托角色及其确切授予范围，在 C04/D06 实施前确认；**这项细节可以留待详设，不影响立即禁止组织写权替代任意赋角权**。`scope=all` 的授予资格同步明确，不能再以任意 org 前缀权限推导。

## 4. 保留的正确控制与渐进范围

- 工单 Authorize 先资源可见再动作；CC 只增加读与评论，不附办理权。List/Count/Stream 共同过滤，空谓词且非显式 Unscoped 会拒绝。
- `ListComments` 依 note 资格过滤内部备注；内部备注不进入公开回复通知路径。
- 当前附件下载 handler 注入工单授权；上传/确认/删除走 update。内部存储函数可接受 nil 回调是接线加固点，未发现当前 HTTP 因它绕过鉴权。
- 转换已有 `LockTransitionSnapshotTx` 校验票和类型版本；IAM/组织/CC 撤权竞争由 C04 补，不误报“完全没有并发保护”。
- AT/RT 密码纪元、Redis 依赖故障拒绝、内置账号/最后 superadmin 守卫保留。PAT 既定独立生命周期不因改密码自动撤销，不擅自改政策；账号停用及当前权限检查仍必须一致。
- AL/TR 生产空服务密钥拒启、核心 internal/compat 默认不挂；服务 HMAC 只证明调用服务。X01 的短期用户 ref＋属主在线 L2/L3 已规划，不能信任自填 X-Operator。
- 新类型仅相关人、已有类型原范围、独立参与人管理、业务管理员和字段 ACL 是已确认但未实现的后续能力，不能反推所有现有透明读取都违反现行设计。

首版建议顺序：**先小修已确认缺陷 → C04/C05 本地一致撤权及必要事实 → 风险/工单/任务/AL/消息启用入口同步保护 → UAA 管理来源及真实联调 → 启多实例前的双进程故障验收**。不引入另一套权限引擎、通用授权语言或全面微服务；共享数据库单行授权屏障先试点，观测等待后再讨论分片。

方法对照 [OWASP 授权检查建议](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)与 [API1 对象授权](https://api-security.owasp.org/editions/2023/en/0xa1-broken-object-level-authorization/)：每次请求/对象检查、默认拒绝、覆盖资源外围出口。[Casbin Watcher](https://casbin.apache.org/docs/watchers/)用于策略副本同步；现有 SyncedEnforcer 的进程内同步不代表多实例撤权已解决。本项目继续用已定的数据库代际和领域当前事实，不因此增加新基础设施。

## 5. 计划、待确认与收尾证据

以上问题挂回既有 C04/C05/D03/D06 与 CE/OBS 适用子出口；健康状态和设计成熟度机器表保持 **open/planned_not_implemented**。未新增顶层阶段、未修改76父包价格、68–104日候选净估或参考日期。实施前把这些已确认修复逐项与原包扣重补子估，不能将新增动态证据说成零成本或已经修好。

已定 mock、本地独立 IAM、公司允许本地先正式、三层校验、业务历史与专门审计分权均不重问。两操作 SetRoles/AssignMenus 的本地强审计失败政策仍是原待决项；真实人员、来源/组织范围、保留参数和企业连接条件按各自启用窗确认。不因审计补新场景就立刻阻塞所有无关详设。

原始证据仅在本机 ignored 审计区：工单 `.codex/audits/permissions-20261115/ticket-probe/`；IAM/PAT `.codex/audits/permission-authn-20261115/`；跨服务 `.codex/audits/permissions-20261115/cross-probe/`（均在 canonical main）；完整基线 `.codex/audits/permissions-review-20261115/`（在设计 worktree）。证据保留源码冻结值、探针、初始安全断言红与正确对照、执行日志及 owner 回收收据，不把合成凭据或私有配置放进 Git。

本页的审计完成不等于修复验收完成。后续小修先把对应探针移为产品回归，保留修前有效红，再跑真实路由和适用完整门禁；权限投影不能只测 UI，动作授权不能只测“action 已注册”，撤权不能只测直接角色。

本轮基线在设计worktree的独占栈通过lint（含Swagger）、race单元/集成、cover、migrate-up及完整acceptance：253通过/0失败/0跳过，81对迁移往返零漂移；默认标签覆盖率20.2%，未因文档审计增加。产品源码与main一致，因此这些是未修复产品的正常基线；另存的安全反例红/越界实际效果是缺陷证据，不能混成“安全测试已全绿”。UI/AL/TR/utils产品未改，本轮不重复宣称其全量E2E/CI新跑；专门TR归因及核心handler/认证测试仅按窄范围记录。

文档另过planning-check（19项闭合/31项排程测试及原DAG核对）、9文件316处本地链接/锚点和JSON状态计数；76父包/17子里程碑/13切片不变。新上下文独立复审核对动态证据、源码、BK-20/PAT边界和设计diff，修正“仅限admin但不验可授档位仍不足”及合法自授措辞；没有为了关审计把产品缺陷改成已完成。

临时9容器/项目卷及网络已按owner回收；16个既有共享容器的ID、镜像、状态/启动时间、重启数、挂载与端口指纹不变。各子探针专属PG资源亦已回收，证据文件保留；当前main干净，本批仅本地设计候选，不推送/合main或部署。
