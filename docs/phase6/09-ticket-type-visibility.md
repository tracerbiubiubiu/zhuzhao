# 09 — 类型级可见性策略（设计稿）

> **阅读状态（2026-11-08）**：本文的✅/“已拍”表示设计确认，不表示风险域或流程v2已经实现。本轮只校准文档；执行仍须Phase5检查/审计/修复收口，经后续授权合main并核验整合基线，再从main切Phase6。入口与分工见[阶段总纲](README.md)，执行顺序与量级总账见[05号](05-launch-batch.md)。

> **文档定位**：段级设计稿（2026-11-03 建档）——承接 [phase3/10 §10](../phase3/10-ticket-business.md) 类型级可见性策略蓝本（2026-09-20 翻案重启点登记）与 [phase5/10 §3.1](../phase5/10-s10-ticket-collaboration.md) 对表项（「两批谁先启动谁对表」——本批即后启动方，建档即触发销账义务，随登记批执行）。**状态 = 草案待拍（代拍项整体搁置）**：完整实施计划已过仓库级事实校验（§11 校验基线），拍板点 P1–P5 建议已记、待所有者拍板后转执行态；**拍板前不动代码与迁移**。**拍板时机（2026-11-03 所有者令）**：代拍项搁置至真正进入 phase6（切 `phase6` 分支后、开工前）一并拍——在此之前本稿仅作记录载体，不阻任何在途批。
> **量级**：W1 主仓 5.5 天 + W2 ui 仓 1.5 天 + W3 测试门禁 1.5 天 ≈ **8.5 天两仓**（原提案 ≈5 天；增量 = 谓词分组重写 / role 三源判定 / 抄送面隔离 / @ 提及移出）。
> **本稿与原提案五差异**（2026-11-03 修订版定稿）：①迁移默认值 participant-only、**同脚本回填存量 inherit**（阻断项 A 修法）；②role 判定改「role code + `GetEffectiveRoleCodes` 三源」；③件 4 剔除 @ 提及（入不做清单）；④L2 谓词改「策略分组」而非 JOIN（`buildTicketWhere` 无 JOIN 位）；⑤量级 5 → 8.5 天。

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
| **端点绑定种子（并入同对）** | 两只读 GET 挂页面行：`why-not-visible` → ticket_list 页面行（同受众先例 = 000065 导出绑定「查询类 GET 与列表同码同页面」）；影响预览 → 类型管理页（照 `FieldImpact` 先例）。**加参与人 POST 绑定二选一（实现期定、登记出处）**：**a 案（建议）**= 并入既有 `pageWriteAllow` 例外登记（ticket_list 族第二例；先例 = transition/000068「与既有 /tickets/close 等同受众——L3 属主面 + condition 细粒度服务端判」，AGENTS 现口径「唯一例外」随之改双例）；**b 案** = 详情页新增按钮行（000031 role_menus 绑 admin+superadmin 惯例）。a 案受众同类、少一行菜单种子 |
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
| 6 | 两个只读端点 | `why-not-visible`（逐分支解释）+ 影响预览（照 `FieldImpact` 先例）；**按 W1 B 案挂页面行（仅 GET）+ 审计留痕** | 0.5 |

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
