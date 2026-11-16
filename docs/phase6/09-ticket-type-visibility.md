# 09 — 类型级可见性策略（设计稿）

> **2026-11-14 现行 D03 模块合同**：B01–B04 已由所有者确认，不再将“默认范围、CC、scope=all、谁可邀请”列待拍。准确 DTO/逻辑表/锁序/正常异常和用例见[工单模块详设](ticket-module-detail.md)，共同语义归[生命周期合同 §5](../design/business-lifecycle-contracts.md#5-d03类型可见性参与人与创建范围)。以下执行摘要取代折叠区历史P1–P5/000082候选/旧B波次，**尚未开发、未执行迁移或赋角**。

## 当前策略、职责与默认

规范值为 `participants_only / role_restricted / inherit`；旧草案`participant-only/role-restricted`只作显式导入归一化别名，不使核心形成两套分支。新类型默认participants_only，已有类型同批回填inherit，升级时已有可见集必须相同。收紧已有类型须受权管理者先预览影响再确认CAS应用，不静默全量收紧；不新增is_sensitive或启用单票override。

| 当前读来源 | participants_only | role_restricted | inherit |
| --- | --- | --- | --- |
| 创建人/当前主责、合法普通参与人、CC | 按L1和当前关系可读/评论 | 同左 | 保留现有读范围与协作语义 |
| 指定有效role_code | 不默认增加 | 仅显式配置命中 | 仍按现有范围 |
| 同部门透明读、普通scope=all、组织委托身份 | 不穿透 | 不穿透 | 保留原范围 |
| 类型业务管理职责 | 只有类型策略显式加入且处于所管组织交集才可读 | 同左，不因role字符串命中变全局 | 按既有及显式职责范围 |
| 系统级admin/superadmin | 保留既有受控例外 | 同左 | 同左 |

以上仍需路由L1；可见/CC/普通参与只提供读与评论资格，不附送改字段、转派、转换或管理参与人。内部备注、附件写和审计跨域检索分别受权。外部同名admin不获得内置系统级语义；真实身份/资源source沿UAA-C接口，不让D03直接依企业SDK。

添加/删除他人的participant或CC采用**按钮授权b案**：`ticket:participant:manage`＋类型/组织实例管理范围＋当前L2/L3。创建人不因创建身份拥有邀请权；自抄送不能用旧selfService入口绕过。本人退出自己手工CC可保留原有读可见退出语义，但不得删除domain/强制通知来源；普通退订也不能停掉既定强制风险通知。管理方因旧账号失去读权需处置时，通过已获权管理范围处理，不开放无权普通诊断。

## 接口与实现顺序

D03使用[生命周期 §5.3](../design/business-lifecycle-contracts.md#53-参与人与创建-api-目标)的participants list/change和creation-options候选。POST业务ID全部在body，携expected_revision及稳定幂等key；GET只读，写挂独立按钮，**不增加pageWriteAllow**。策略变更独立plan/apply绑定旧策略revision、组织/角色基线、影响摘要、确认者；不能靠编辑流程Schema自动修改实例可见范围。

1. 先消费C04/BK-21共同可见性/当前授权守卫；在同一谓词内按类型策略分组。`global_exception OR (inherit AND old_branches) OR (restricted AND restricted_branches)`；不能最后OR一个scope_all或CC逃出分组。列表/详情/关联入口/计数/导出/首页/任务结果/通知投影同源。
2. `role_restricted`使用有效三源role_code，但业务管理范围保留组织锚点，不以展开出的字符串扩大到全类型；未知策略/未知引用拒绝而非退回inherit。不存在或停用角色在写预检拒绝，读取时fail-closed并显式运维告警。
3. 参与来源按`ticket_member_sources`逻辑模型落地，`ticket_cc`仍为有效role投影；手工、创建人、主责和风险复核来源分开。移除一个来源不能误删另一个合法来源，转出复核权也不因仍可看票继续保留复核动作。
4. `TicketCCRepo.ListByTicket`当前尚未区分role，D03启用participant时须只将role=cc暴露到抄送面；IsCC不能扩大为任意成员。新参与/CC写、实例revision、领域事件及可靠通知intent同事务，不能先改变可见集再丢历史。
5. creation-options返回当前允许创建的类型/组织/模板与发布引用；现有同分支创建约束保留。普通读角色不自动可创建；风险组的受限派单按B05领域桥复用校验，不要求通用ticket:create、不允许任意created_by。

## 迁移、回退和数据访问

候选新增类型live策略revision及查看/创建配置，已有类型回填inherit；实例revision/定义版本共用D01，不重复各自加version。规范策略、稳定资源键、受管范围等准确存储见模块详设；实施当天核迁移地图后分配up/down号，本页原000082不占号。新页面GET和按钮POST种子需同时验证既定admin/superadmin及合法业务操作者可达，不只给系统管理员种子便称功能可用。

新增类型策略在本地模式可独立运行，无UAA配置仍完整使用本地账号/角色/菜单管理；企业受管权限以后通过adapter投影进同一三层执行路径，不能用本地org_roles补回中央已撤权限。D03不一次实现全UAA治理。

策略和参与变更实时影响新读写，不入实例业务定义快照；旧票不会永久保留已撤成员/字段读权。缓存绑定主体、策略revision及授权epoch，写前重验；异步导出下载、旧结果查询、深链、统计均重验当前资格。普通不可见/不存在同形404，不暴露票名、参与人或影响数量；受权管理预览另有审计，不给普通用户“为什么别人能看”的探针。

回退前停新受限写、备份策略/来源与新增历史、核旧代码能力；不能先删策略列再让live旧实现无声扩大已收紧数据。确需降级必须明确选择恢复范围并受控部署，未批准时保持保护；down结构可回不代表权限语义自动安全回退。

## 用例与开工出口

- `Visibility_AllReadSurfacesAgree`：三策略×创建人/主责/CC/普通participant/同部门非参与/角色命中/普通all/组织owner/业务管理员组织交集/系统管理员，所有读出口同集合。
- `Participants_GrantDoesNotOperate`：CC读评论、普通participant读评论；无处理权403，creator普通邀请403；无L1即使参与仍不绕功能闸。
- `Participants_SourceRevocation`：风险复核交接只删受管来源；另一manual来源保留读但无旧复核权；重复添加不重复通知，domain项普通删除拒绝。
- `Visibility_PolicyPlanConflict`：预览后角色/策略/组织范围变化409且零额外授权；未知策略拒绝，组织停用撤派生角色。
- `Visibility_BackfillPreservesLegacy`：升级前后inherit可见集逐角色diff为零；新类型缺省为participants_only；所有写绑定三道B案守卫通过。
- `Visibility_RevokeInFlight`：撤权先提交后旧事务不提交；排队任务/导出/消息/浏览器缓存不能继续泄露；原不可见诊断同形。

以上是待实现的行为测试目录；需真实PG/当前角色fixture与浏览器门禁，不由设计表推定通过。P1/P2/P3按B01/B02/B03已确认；P4取role_code稳定编码、P5消费C04/BK-21是现行技术/依赖选择，不再请所有者重复拍。仍需绑定具体类型、角色、业务管理组织与实际参与人管理授予，在首次启用前预览确认；日期/工日仍回唯一总账。

## 历史设计与裁决来源

<details>
<summary>展开2026-11-03至11-09原稿：旧量级、候选号与当时待拍状态</summary>

以下原文只保留来源，P1–P5“待拍”、a案pageWriteAllow、旧分支/门禁计数不再是现行执行约束。历史code/字段核验只证明当时现状，本次新增功能仍待实现。


> **2026-11-11现行排期**：本稿保留详细设计和历史裁决；范围、依赖、估算与条件项以[新路线](../roadmap.md)、[去重总账](../planning/planning-backlog.md)和[平台边界](../design/platform-evolution.md)为准。旧“合main待办/整包前置/触发未到不排期/所有代拍等切phase6”不再是当前执行顺序；未定语义在对应工作包开工前确认，不能据此认为代码已实现。

> **阅读状态（2026-11-08）**：本文的✅/“已拍”表示设计确认，不表示风险域或流程v2已经实现。本轮只校准文档；执行仍须Phase5检查/审计/修复收口，经后续授权合main并核验整合基线，再从main切Phase6。入口与分工见[阶段总纲](README.md)，执行顺序与量级总账见[05号](05-launch-batch.md)。

> **文档定位**：段级设计稿（2026-11-03 建档）——承接 [phase3/10 §10](../phase3/10-ticket-business.md) 类型级可见性策略蓝本（2026-09-20 翻案重启点登记）与 [phase5/10 §3.1](../phase5/10-s10-ticket-collaboration.md) 对表项（「两批谁先启动谁对表」——本批即后启动方，建档即触发销账义务，随登记批执行）。**状态 = 方向已定（2026-11-09 所有者确认入 phase6 完成，执行排期后定）；代拍项 P1–P5 维持搁置至 phase6 入口一并拍**：完整实施计划已过仓库级事实校验（§11 校验基线），拍板点 P1–P5 建议已记；**拍板前不动代码与迁移**。**拍板时机（2026-11-03 所有者令）**：代拍项搁置至真正进入 phase6（切 `phase6` 分支后、开工前）一并拍——在此之前本稿仅作记录载体，不阻任何在途批。
> **量级**：W1 主仓 5.5 天 + W2 ui 仓 1.5 天 + W3 测试门禁 1.5 天 ≈ **8.5 天两仓**（原提案 ≈5 天；增量 = 谓词分组重写 / role 三源判定 / 抄送面隔离 / @ 提及移出）。
> **本稿与原提案五差异**（2026-11-03 修订版定稿）：①迁移默认值 participant-only、**同脚本回填存量 inherit**（阻断项 A 修法）；②role 判定改「role code + `GetEffectiveRoleCodes` 三源」；③件 4 剔除 @ 提及（入不做清单）；④L2 谓词改「策略分组」而非 JOIN（`buildTicketWhere` 无 JOIN 位）；⑤量级 5 → 8.5 天。

> **D03当前建议**：加参与人采用显式按钮授权b案并覆盖预期受众；a案仅历史备选。普通诊断对不可见/不存在同形、不泄露参与人，管理影响预览须单独授权与审计。尚未改迁移或pageWriteAllow。

## 1. 策略语义矩阵（实施批必须写死）

| 可见来源 | `participant-only`（新类型默认） | `role-restricted`（params=role codes） | `inherit`（存量回填） |
|---|---|---|---|
| 属主（created_by / assigned_to） | 可见 | 可见 | 可见 |
| `ticket_cc` role='participant' | 可见 | 可见 | 可见 |
| org 锚点透明读（`org_path <@ 锚点`） | **不可见** | **不可见** | 可见 |
| 委托轴（org admin/owner、ancestor owner） | **不旁路**（铁律 2） | **不旁路** | 可见 |
| `ticket_scope=all` | **不生效**（待拍 P3） | **不生效**（待拍 P3） | 生效 |
| 抄送（`role='cc'`） | 可见（显式授予，非结构性越权，待拍 P2） | 可见（同左） | 可见 |
| 命中指定角色（有效角色三源） | — | 可见 | — |
| admin / superadmin | 恒可见（`Authorize` 既有 bypass，作为 fail-closed 的**显式例外**登记） | 同左 | 同左 |

**授予级别**沿用 phase3/10 §10 设计稿口径：可见即 read/comment，不自动进 `canOperate`（能看不能改）。

## 2. 数据模型与迁移（当时候选000082，一对up/down含列与绑定种子）

000082只表示建稿时可用候选号，不是预留号；实施前重新核migrations实际占用与review/11迁移地图。P1–P5及加参与人a/b绑定仍待拍，不能提前增加pageWriteAllow例外。

| 项 | 内容 |
|---|---|
| `ticket_types.visibility_policy` | `JSONB NOT NULL DEFAULT '{"policy":"participant-only"}'`，**同脚本内 `UPDATE ... SET = '{"policy":"inherit"}'` 回填存量**（阻断项 A 的修法），新类型走 DEFAULT |
| `tickets.visibility_override` | `JSONB NULL`，留缝不启用（phase3/10 §10.1 设计稿口径一致，Jira per-issue level 同构） |
| **端点绑定种子（并入同对）** | 两只读 GET 挂页面行：`why-not-visible` → ticket_list 页面行（同受众先例 = 000065 导出绑定「查询类 GET 与列表同码同页面」）；影响预览 → 类型管理页（照 `FieldImpact` 先例）。**加参与人 POST 绑定二选一（实现期定、登记出处）**：**a 案（历史备选，不作当前推荐）**= 并入既有 `pageWriteAllow` 例外登记（ticket_list 族第二例；先例 = transition/000068「与既有 /tickets/close 等同受众——L3 属主面 + condition 细粒度服务端判」，AGENTS 现口径「唯一例外」随之改双例）；**b 案（当前推荐）** = 详情页新增按钮行，明确预期业务受众的授权并验证其可达性，不能只给admin/superadmin种子后漏掉合法操作者。维持页面只读，不增加pageWriteAllow例外 |
| 索引 | 无需新增：`ticket_cc` 的 `UNIQUE(ticket_id,user_id,role)` 前缀覆盖 participant EXISTS；`tickets` 已有 `idx_tickets_type_status(type_code,status)` 支持分组分支；`user_roles` PK 支持角色反查 |
| down | 删两列 + 删绑定种子行；因回填在 up 内且 down 不回写类型策略，注释声明「down 仅回结构、策略语义丢失由代码回退兜底」。000075/76/77 型回滚耦合陷阱不适用（无写端点回挂页面行的 B 案对撞） |
| 编号核对 | 2026-11-03基线末位000081，当时000082可用；review/11缺行/81对162文件/下一编号勘误已于11-04先行，不再等待本批。新迁移编号在真正实施前重新核地图和目录，不长期占号。 |

## 3. 后端件（W1，主仓 ≈ 5.5 天）

| # | 件 | 关键落点 | 量级 |
|---|---|---|---|
| 1 | 迁移（重新核号，原候选000082） | 见 §2（列 + 绑定种子 + 勘误随行） | 0.5 |
| 2 | 策略库 | 命名策略注册（`participant-only`/`role-restricted`/`inherit` 实做，`org-anchored`/`hybrid` 占位报「未实现」）；写时 schema 校验 400、读时未知策略名拒绝（fail-closed）；**role 判定复用 `GetEffectiveRoleCodes` 三源**（`user_roles` ∪ `org_roles` ∪ `roles.parent_id`；`rbac_service.go` 的 `GetRoleCodesByUserID` 包装同源，BK-17 请求内缓存顺带复用）；**type→policy 映射不缓存——每请求现查**（ticket_types 小表单条 `SELECT type_code, visibility_policy` 成本可忽略；进程内缓存多实例失效不跨实例传播，2026-11-03 由「可缓存」修订为不缓存） | 1.5 |
| 3 | L2 双路径 | 每请求现查 type→policy 映射，按策略分组生成 `type_code = ANY($n)` 分支 + 相关子查询（`buildTicketWhere` 无 JOIN 位——相关子查询形态）；`Authorize` 同分派；**org 锚点透明读 / 委托轴 / ticket_scope=all 在非 inherit 策略下逐条条件化**（阻断项 D） | 1.5 |
| 4 | participants 维护点 | Create 写发起人、assignee 变更同步、显式「加参与人」端点（绑定方案见 §2 a/b 案）；**@ 提及移出本批**（不做清单）；与 `ticket_events` 同事务（依赖 [06 号](06-ticket-completeness.md) 件 1——转换主路径事件行补齐同批） | 1 |
| 5 | 抄送面隔离 | `TicketCCRepo.ListByTicket` 加 `role='cc'` 过滤（阻断项 C——现实现无 role 过滤，未来 participant 行会漏进抄送面；`IsCC` 已带 role='cc' 不动）；`AddCC` 返回值随之干净 | 0.5 |
| 6 | 两个只读端点 | `why-not-visible`仅给当前主体对已受权对象的安全说明，不可见/不存在同形且不泄露成员；逐分支/可见集影响限受权管理预览并审计（照 `FieldImpact` 先例）；**按 W1 B 案挂页面行（仅 GET）+ 审计留痕** | 0.5 |

## 4. 前端（W2，ui 仓 ≈ 1.5 天）

类型编辑抽屉新增策略段：策略下拉（源=后端注册表）、role-restricted 的角色多选（**提交 code**）、保存前调影响预览、存量类型显示「当前 inherit」提示；角色清单复用 `GET /roles`（注意其在管理面权限下，非全局管理员需权限说明）。`pnpm gate` 六件全绿。

## 5. 测试与门禁（W3，≈ 1.5 天）

| 项 | 断言 |
|---|---|
| 双路径一致性 | 3 策略 × 5 主体（属主/参与人/抄送/同 org 非参与/角色命中）Authorize 与 List 结论必须一致 |
| 天花板三条 | 委托 owner 看 participant-only 单被拒；`ticket_scope=all` 用户对 restricted 类型不多见；cc 人可见但改单 403 |
| fail-closed | 未配类型=默认策略；未知策略名读时拒绝写时 400 |
| **存量不变** | 升级前后同类型可见集快照 diff 为零（防护阻断项 A 回归） |
| 绑定守卫 | 三道守卫复跑：app 启动 `AuditPageWriteBindings` fail-fast + `TestCatalogPageRows_NoWriteBindings`（CI test-integration）+ acceptance phase1 D2 B 案；若加参与人走 pageWriteAllow 例外（§2 a 案），守卫白名单 +1 例并登记设计出处（本号 §2） |
| 门禁 | `gofmt`/`go vet -tags=integration`/单测+集成/build、`make migrate` 往返（82 对零漂移）、四档 acceptance、两仓 CI 转绿 |

## 6. 排期与依赖

- 全局前置：phase5 走查收口 → 合 main（⏸ 暂缓待测试收口，05 号 §6.1 口径）→ 切 `phase6`（当前 phase6 规划文档全在 `phase5` 分支）。
- 批内前置：[06 号](06-ticket-completeness.md) **件 1（B1，转换字段采集/事件行同事务）** + **件 3（B2，`cc_me` 消费面）**；排 B3 之后安全但非必需。
- **BK-21 计账（P5 待拍）+ 顺序咬合**：[05 号 §1.2](05-launch-batch.md) Week 0 已列 1–2 天且 §2 写死「BK-21 先于 R1」。二选一——消费 Week 0 那件（本批 0 增量，只补类型策略相关守卫用例）／提前到本批（Week 0 减 1–2 天并改依赖表述）。**顺序句（2026-11-03 补，随 P5 一并拍）**：建议 **09 号排 Week 0 之后**——若 09 号先动 `GetFilter` 引入策略分组新 pattern，BK-21 的「cc 读特例同模式」须吸收之，增量会塞回 Week 0、与「本批 0 增量」矛盾；备选 = 实现前对表一次、声明两者相互独立并行安全（取决于哨兵泛化实际形态）。

## 7. 未决拍板点（建议已记，待拍后转执行态）

> **拍板时机（2026-11-03 所有者令）**：本表及稿内全部代拍项（含 §6 BK-21 顺序句、§2 加参与人 a/b 案）整体搁置至真正进入 phase6 一并拍——建议列供届时比对，非既定结论；记录完整性 2026-11-03 已核对（五类在途项全覆盖，见变更记录）。

| # | 问题 | 建议 | 理由要点 | 状态 |
|---|---|---|---|---|
| P1 | 存量类型是否允许就地收紧 | 回填 `inherit`，不动存量 | 存量可见集零变化可测（§5「存量不变」项）——升级零风险 | 待拍 |
| P2 | restricted 类型下 cc 人是否可见 | 可见 | cc 是显式动作（非结构性推定），视为显式授予而非越权 | 待拍 |
| P3 | restricted 下 `ticket_scope=all` 是否生效 | 不生效 | scope=all 是结构性全量授权，与 restricted 的显式收窄语义相抵；与委托轴同性质 | 待拍 |
| P4 | role params 存 code 还是 ID | code | 随类型定义包跨环境导出；与权限码 `resource:verb` 惯例同源 | 待拍 |
| P5 | BK-21 计账归属 | 消费 Week 0，本批只补用例；顺序句见 §6 | GetFilter 泛化先落则 09 号纯消费；避免双向塞增量 | 待拍 |

## 8. 不做清单

沿用设计稿四条（DSL 入 DB / 外置 PDP / 读压前物化 / 微服务拆分）+ 本批新增五条：@ 提及、`visibility_override` 启用、角色组合（AND/差集）、`org-anchored`/`hybrid` 实现、**策略映射缓存**。

## 9. 文档登记清单（拍板后执行；纯 docs 批）

1. [phase3/10 §10](../phase3/10-ticket-business.md) 状态行 → 「随 phase6 09 号实施」
2. 本文件状态行「草案待拍」→ 执行态（含 P1–P5 拍板记录回填 §7）
3. [phase3/12 §3.6](../phase3/12-frontend.md) 状态行回写（类型可见性策略配置页）
4. [phase5/10](../phase5/10-s10-ticket-collaboration.md) §3.1 对表项销账 + 拍板点表 2 行 + §6 对表行（ticket_cc + role 语义演化，废弃 `ticket_participants` 名）
5. [README §2](README.md) **段级索引 +09 行 + 头注「段级稿 00–08 号」→ 00–09 + §7 变更记录行**（08 号「改 A 漏 B」外审同源教训——原登记清单漏此步，2026-11-03 补）——✅ 2026-11-04 指针微批先行（草案态标注）；拍板后仅需行内「草案待拍」语改执行态
6. [07 号](07-backlog-and-triggers.md) 加**索引行**（指针指向本号；07 号章程「不承载内容」）——✅ 2026-11-04 指针微批先行（§3.2 触发行）
7. 实施后回填 [11 §4](../review/11-project-control.md) 迁移地图 **+实际占用的新迁移行（原候选000082）** + §6 能力矩阵——其中勘误三项 stale + §3 规模行 ✅ 2026-11-04 指针微批先行（000081 补行 + 下一编号→000082 + 81 对/162 文件）
8. roadmap 同型检查（08 号批先例：roadmap 与 review/11 各有同型行——登记批随查随改）

## 10. 量级汇总

W1 5.5 + W2 1.5 + W3 1.5 ≈ **8.5 天两仓**（绑定种子并入 000082 计入件 1，不外加；原估 ≈5 天，增量来源见头注）。

## 11. 事实校验基线（2026-11-03 对称验证，15+ 锚点全属实）

- 迁移末位 000081（`settings_reset_bind`/0081f2d）→ 000082 可用；11 §4 三处 stale 实核（标题对数/缺 000081 行/两处下一编号过期）
- `ticket_cc` UNIQUE(ticket_id,user_id,role) + role 列注释预留 cc/participant + `idx_ticket_cc_user`（000062）
- `idx_tickets_type_status(type_code,status)`（000010）；`user_roles` PK
- `role_repo.GetEffectiveRoleCodes` 实存；`rbac_service.go` `GetRoleCodesByUserID` 包装（BK-17 请求内缓存复用位）
- `TicketCCRepo.ListByTicket` 无 role 过滤（阻断项 C 实锤）；`IsCC` 已滤 role='cc'
- `FieldImpact` 先例（field_impact_integration_test.go + ticket_handler）
- `Authorize` admin/superadmin bypass L2/L3（`resource.go`，L1 路由级 Casbin 仍走）
- `GetFilter` OR 支结构（delegatedVisibilitySQL + ccVisibilitySQL）+ ticket_scope=all 全量支 + `buildTicketWhere` 无 JOIN 位
- BK-21 在 05 号 §1.2（1–2 天）+ §2「先于 R1」；06 号件 1（§3.1 同事务）/件 3（§3.3 cc_me EXISTS 不旁路 L2）
- phase5/10 §3.1 对表项（:31/:67/:104 三处）；phase3/10 §10 蓝本（policy JSONB + override 留缝不启用）；phase3/12 §3.6 配置页登记
- 07 号章程「不承载内容」；README §2 段级索引 00–08（08 号「改 A 漏 B」外审实录）
- 绑定先例：000065 GET 挂页面行（导出）；000068 transition `pageWriteAllow` 例外（ticket_list 族）


## 整理保留的历史文本（2026-11-08）

<details>
<summary>展开查看被现行说明取代的旧估算、候选与时点口径</summary>

以下仅用于追溯，不是当前实施合同；有效说明与待定约束以正文标注为准。原有日期变更记录继续保留。

> | 编号核对 | 现末位 000081（`settings_reset_bind`，0081f2d），**000082 可用**；**勘误随行**：[11 §4](../review/11-project-control.md) 地图三项 stale——标题「80 对」vs 实际 81 对、**缺 000081 行**、两处「当前下一编号 000081」过期（§4 尾注 + IW2 行）——随本批登记/回填一并修正，下一编号口径改 000082；§3 规模行「160 个迁移文件（80 对）」同批刷新 |

</details>

## 变更记录

| 日期 | 批次 |
|---|---|
| 2026-11-03 | **建档（草案待拍）**：修订版完整计划落稿——§1 语义矩阵 / §2 迁移（绑定种子并入 000082 + 加参与人 a/b 案 + 勘误随行）/ §3 后端六件（策略库不缓存）/ §4 前端 / §5 测试门禁（绑定守卫行）/ §6 排期（BK-21 顺序句）/ §7 P1–P5 建议已记待拍 / §9 登记清单（+README §2 索引步——原清单漏项补）/ §11 校验基线；四处补强来源：①11 §4 勘误扩围（缺 000081 行+两处下一编号过期+规模行）②迁移对数修正（端点绑定种子需迁移，000052/000065/000068 先例）③策略缓存砍掉（多实例失效不跨实例+小表现查）④BK-21/09 号顺序咬合句 |
| 2026-11-03 | **搁置令落档**（所有者「代拍项先搁置、等真正进入 phase6 再拍板，先确认记录无遗漏」）：头注/§7 +拍板时机句；完整性核对五类在途项全在稿=P1–P5 拍板点·加参与人 a/b 案（实现期）·BK-21 归属+顺序句·§9 登记八步·11 §4 勘误随行 |
| 2026-11-04 | **指针微批入库**（所有者批「指针+勘误合成一笔先落」）：本号随批入库；README §2 索引+09 行（草案态）+头注 00–09；07 号 §3.2 触发行；review/11 编号勘误先行三项（000081 补行/下一编号→000082/81 对+规模行 162）+§7 树与 roadmap 同型 00–09；§9 步 5/6/7 标注先行态；拍板态其余步骤不变 |
| 2026-11-09 | **方向确认批**（所有者：「想把他在 phase6 完成，执行后面再说」）：头注状态改「方向已定」——P1–P5 与执行排期仍按 2026-11-03 搁置令至 phase6 入口一并拍；随附与本稿独立复算计划（另一会话同日产出）的三处一致性核对：①role-restricted 已在 §1 语义矩阵；②ticket_cc 定名保留已在 §9-4；③铁律 3（回溯 vs 快照）由 §3-2「策略映射不缓存——每请求现查」架构决策隐含解决（回溯生效语义，无独立拍板项）——无新增缺口，本批零内容变更 |


</details>

- 2026-11-14：按B01–B05及本地核心先行回填现行D03执行摘要；旧待拍状态整体保留为折叠历史，冻结b案、来源区分、三层全读出口与负例，未实施。
