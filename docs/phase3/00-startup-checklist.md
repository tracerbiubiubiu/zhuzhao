# Phase 3 启动检查单（Startup Checklist）

> **用途**：Phase 3 启动时的**唯一检查入口**。本文件是**快照 + 索引**，不是第二 SSOT——每行都注明权威登记处，启动时先按 §1 顺序逐项核对并刷新状态，防止与各 SSOT 漂移。
>
> **基线**：2026-08-31 ｜ 提交 `c389156` ｜ 分支 `feature/phase-2`
> **前提（2026-08-31 所有者确认）**：**不拆微服务**，以增强各模块能力为主——与 [roadmap](../roadmap.md) / [README §0](./README.md)「微服务整体推迟、Phase 3 按需启动子能力」既有决策一致，无需改决策文档。

---

## 0. 快速结论

| 结论 | 内容 |
|------|------|
| 启动前必须清空 | **A 档 7 项**（约 2 天人工 + 2 个拍板），见 §2.1 |
| 已触发待实施（不属 Phase 3，建议先做） | **W1：BK-13 + BK-14 多虚拟组可见性场景闭环**（1–1.5 天），见 §2.3 |
| 随行项 | **B 档**按启动的子能力对号入座，见 §2.2 |
| 启动时拍板 | §3 决策清单逐项过 |

## 1. 启动检查顺序（操作流程）

1. **刷新本表**：对照各登记处（00 §9 / 11 §6 / 11 §8 / phase3 README §4 / 09 合集）核对状态，更新本文件（~10 分钟）。
2. **清空 A 档**：§2.1 七项全部完成或显式豁免（含 A2/A3 两个拍板）。
3. **检查 W1**：多虚拟组场景闭环是否已实施？未做则决定「先行独立完成」或「并入启动批次」。
4. **选定启动子能力**（Step 1–7 任选，触发条件见 [README §0](./README.md)），映射 §2.2 B 档随行项；补编写对应文档（B9）。外部能力集成（activelist，ADR-003）按 README §2.1.0 W4 条目评估启动。
5. **过 §3 决策清单**剩余项。
6. **迁移号核对**：按 A2 规则（谁先启动谁占用，后者重排）确认新迁移编号。
7. **回填 [11-project-control](../review/11-project-control.md)** 能力矩阵，Phase 3 状态「暂缓」→「进行」。

## 2. 全量清单

### 2.1 A 档：Phase 3 启动前/时完成（门禁与拍板）

| # | 事项 | 权威出处 | 量级 |
|---|------|---------|------|
| A1 | ~~文档修正包：① phase3 README §1.4 前置矛盾、§5 状态行；② review/10 C1–C4 处置（C1 驳回维持 / C2 已修注记 / **C4 ScopeResolver 接口段重写（真开放）** / C3 → B10）；③ 09 合集抽查回注（F-01/02/03/18 核实已修）~~ | ✅ 已完成（2026-08-31） | 详见 11 §8 对应行 |
| A2 | ~~迁移编号 000017 归属拍板~~ | ✅ 已拍板（2026-08-31）：谁先启动谁占用，后者整体重排 | 已关闭 |
| A3 | ~~BK-11② `org_path` 数据结构拍板~~ | ✅ 已拍板（2026-08-31）：保留镜像列（FOR SHARE 兜底）；created_org_id 留 Step 7e 按需 | 已关闭 |
| A4 | ~~HC1：comment/note 补 `ticket_events`（Step 7 事件流地基）~~ | ✅ 已完成（2026-08-31） | 详见 11 §8 对应行 |
| A5 | ~~BK-5：relation 反向判重（报表数据质量）~~ | ✅ 已完成（2026-08-31） | 详见 11 §8 对应行 |
| A6 | ~~SoD 延后决策落档（动态 SoD 优先）+ phase2/12·13 断链修正 + 错误注入测试③~~ | ✅ 已完成（2026-08-31） | 详见 11 §8 对应行 |
| A7 | ~~TC1-Go：全局 admin 删单成功集成测试（回归基线进 CI）~~ | ✅ 已完成（2026-08-31） | 详见 11 §8 对应行 |

### 2.2 B 档：随 Phase 3 子能力一起（提前做无收益）

| # | 事项 | 随行位置 |
|---|------|---------|
| B1 | **Step 7 设计期拍板清单**：① SLA 暂停态语义 / 通知「主管」定义 / 邮件通知矩阵（对话评估 B1/B3/B6）；② **10 号文档设计深度缺口（2026-08-31 外部评审证实）**：§2.5「标记+Enqueue 同事务 vs 只 Enqueue」二选一（SLA 正确性核心，文档给主方案未拍板）、§7.2 signal 双写二选一（两条记录 vs 单条双标）、`responded_at` 是否含内部备注 note、`min_level` 职级数据源（**users 表无 level 列，决策悬空**）、§5 分派设计深度（keyword 匹配算法/同优先级冲突/target 从属校验/无命中兜底/分派 Hook 事务边界）、§6 报表设计深度（权限码/缓存失效/指标口径/分页）、§8 TB 负向用例补齐（§2.5 四必坑 0 覆盖）；③ 调研吸收（2026-08-31 eflow 对照）：发起人撤回（Revoke，eflow WITHDRAWING 栅栏模式）/workflow_definitions 版本发布快照/审批人 Assignee{rule,values} 策略模型（解 min_level 悬空） | **方向已认可（2026-08-31）**，细节归 7-0 设计期 |
| B2 | 权限码 seed：ticket:approve / notification:* / workflow:manage | Step 7 |
| B3 | in_progress / pending_verify 状态推进端点（BK-10 已拍板归 Phase 3） | Step 7 |
| B4 | BranchedStateEngine 引擎本体（**硬交付**；消费 A6 的 SoD 决策：互斥优先动态 SoD） | Step 7c |
| B5 | BK-11② 实施 | 随 A3 拍板，Step 7 动工前 |
| B6 | ~~BK-12：org_roles / parent_id 写侧~~ | ✅ 已实施（2026-08-31，IW3 附带）：绑定/解绑/列表端点 + parent_id 单调规则；HR 同步启动时无需再补写侧 |
| B7 | CORS AllowAll 转轨收紧（09 合集 F-21） | Step 5 security-enhance + 上线检查单 |
| B9 | 文档补齐状态：**已编写（2026-08-31）**：01 / 02-multi-instance / 10（7-0 决议已入）/ 11 / 12-frontend；**已编写（2026-09-02，原 5 份待编写全部补齐）**：03-audit-l2（替换占位，含 B11①②）/ 06-ha / 07-security-enhance / 08-ops + **ops/deployment.md**（B10）/ 09-platform；另 13-implementation-plan（执行计划）与 docs/ops/deployment.md 同日补齐 | 全部文档就绪，启动时按 Wave 取用；**M2 硬依赖 = Asynq 底座（2026-09-02 §22.1 修订，M1 降 🚦）**；**参考**：Watcher 移植 eiam `ioc/casbin.go`（redis-watcher+StartAutoLoadPolicy 双保险）、Asynq 任务建模仿 etask（RetryConfig 指数退避/补偿器） |
| B10 | `docs/ops/deployment.md` 补编写（骨架 README 已在，review/10 C3） | 随 Step 6 ops / 部署文档批 |
| B11 | **审计治理两件（2026-09-01 go-wind-admin 调研吸收）**：① **L2/L3 策略评估日志**——判定日志表 + `resource.Authorize`/`scope_resolver.resolve` 埋点（actor/资源/动作/scope 轴/结果/原因/trace_id），补 L2 拒绝无留痕盲区（现状：L3 路由拒绝有 slog Warn、审计行带 403/404；L2 scope 拒绝完全静默）；② **审计归档**——audit_logs + 判定日志表超期导出 JSONL、导出成功后删行（保留期默认 180 天等保口径、可配置）。暂缓期不提前建表：判定日志是天然大表，先建无归档=重蹈 audit_logs 覆辙 | ①随 **W1/M1**（03-audit-l2 文档范围，B9；写入管道「同步落库 vs Redis List 缓冲、失败容忍」随该文档拍板）；②随 **M-E**（Asynq 任务平台首个预置任务；~~SLA 扫描先用~~ 随工单暂缓，2026-09-02 §23） |

### 2.3 独立窗口 IW1–IW3（已触发 / 按需，Phase 2 范畴；「W」编号独占给 README Wave，本表用 IW 前缀）

| # | 事项 | 状态 |
|---|------|------|
| IW1 | ~~BK-13 + BK-14 多虚拟组可见性场景闭环~~ | ✅ **已实施（2026-08-31）**：000017 CHECK + org update 配置 API + L2 委托轴 + scope 配置面全链 + D12/委托轴/矩阵测试；验证 211/0 全链 + 13 包 `-race` |
| IW2 | 2b-ext 三件：附件（触发 A2）/ auth-enhance / ~~HR 同步~~（**已升 Phase 3 主链 2026-09-02**：M-HR（原 M2.5）预留接口版 HRFetcher+引擎+mock adapter，design-decisions §22.2/§23.2 / 13 号 §1 M-HR） | 附件 / auth-enhance 按需独立启动；HR 同步随 Phase 3 主链 |
| IW3 | ~~BK-18：类型/字段/模板管理闭环~~ | ✅ **后端已实施（2026-08-31）**：迁移 000018 + 7 管理端点 + G2 校验 + TestBK18×2；前端管理页/动态表单照 12-frontend 施工（另排期） |
| IW4 | ~~行级过滤护栏（fail-closed）~~（2026-09-01 go-wind-admin 调研吸收） | ✅ **已实施（2026-09-01）**：`resource.Filter` 增 `Unscoped` 显式豁免（admin bypass / ticket_scope=all 两处显式化）+ `ticket_repo.List` 入口 fail-closed 哨兵（无谓词且未豁免 → 报错）+ `TestGuard_TicketRepoListCallSites` AST 守护（调用点锁定 ticket 包）+ 测试 4 个（哨兵拒绝 / AllScope 豁免 / 常规谓词不误伤 / 集成双侧 `TestTicketRepoList_Iw4Guard`）；验证 = lint + 13 包单测/集成 `-race` 全绿 + acceptance 四档 FAIL=0（87+66+26+32 = 211，2026-09-01 实测） |

> **随手项（任意时点）**：BK-9（测试死代码）、09 F-31④（relation 越权负向用例）、09 F-32（audit/user service 分支级单测，低优）、TC-2（ListRelations/字段/模板 service 直测）、TC-3（非叶子节点 Move 并发）、TC-4（UpdateTicketType name/description patch 测试）、可选双删 404 回归断言、可选 Q5 组织赋角注记（11-authz §5 Q5 不变量补一句「组织赋角（org_roles/parent_id）使 token 快照原理上不可行」，doc-only，2026-09-01 登记）、P2-3（BFS CTE 双处一致性测试或抽共享 SQL）、**~~测试隔离债~~**（原口径仅「`-count=2` 下 R 系列二跑撞首跑」，8151a15 登记；063f5c9 复现证实为既有债务）。**根因定稿（2026-09-01 二批，口径改宽）**：两类随机红——① `idx_org_code` 23505：`setupTicket2a` 的 code 用 `to_char(clock_timestamp(),'MS')`（秒内毫秒位 000-999，周期 1s）+ 共享容器残留不清理 → 同 run 高负载下 setup 间隔漂移进整秒碰撞窗口、跨 run 对残留以 ~1/1000/行 累积撞车，flaky 单调恶化；**非并行竞争**（全仓零 `t.Parallel()` 实证）。② 跨包 TRUNCATE 踩踏：`org_repo:22` TRUNCATE organizations、`b4/rbac/auth` TRUNCATE users CASCADE，`./internal/...` 多二进制并发时端掉 ticket 包脚下的表（no-rows/FK 类随机红）。**全量清偿（2026-09-01 二批）**：`uniqueSuffix()`（完整 UnixNano）统一替换全仓 40 处截断点（ticket 28 + service 12；%1e9 属同模式隐患，10⁻⁹ 量级非碰撞源，统一化=一致性加固）；7 个建 org 函数全部 `t.Cleanup` 软删（`softDeleteOrg`，部分唯一索引软删即释放 code、FK 不看 deleted_at 无需清工单）；`2a_it_*` 用户改每 run 全新 + `createB2User` eno 唯一化（fixture 唯一化，R3/R8 计数与 `idx_user_orgs_single_primary` 随之修复）；`childOrgID` DO NOTHING→DO UPDATE RETURNING（消 no-rows 雷）+ 删 `p2a_it_sub` 死退化分支（不可达）；模板创建点补清理（保 `TestMeta_Templates_Empty`）；`make test-integration` 加 `-p 1`（跨包串行）。**验证**：ticket 包 `-count=2` 全绿（原必挂）+ 全量集成 `-p 1` 二连跑全绿 + lint/单测/acceptance 27+66+26+32 FAIL=0。user_orgs 孤儿行（软删 org 后残留）不清理：FK 不看 deleted_at、读侧按新 user_id 查询无串扰，彻底清偿可选。
> 已清：A6③（已落 02-authz §4 用例表）、BK-16（复核误报关闭）、BK-17（已随 IW1 落地）。
> **BK-19（中优，非随手）**：工单 handler 层 Go 测试（TC-1），见 00 §9。
> **BK-20（软删组织委托残留处置，2026-09-02 登记）**：禁删有未结工单的组织（两删除函数加计数守卫 + ErrOrgHasOpenTickets 409 + acceptance 删组织用例适配）；已结工单委托残留=档案连续性登记不修（显式断开=删除前 SetOwners 清空）；语义 SSOT = design-decisions §21，详见 00 §9。

> **IW3 备注**：BK-18 与 Phase 3 引擎零耦合——管理的是类型/字段/模板而非 workflow_definitions，Phase 3 落地后管理面原样复用，故不等 Phase 3。
> activelist（ADR-003）**不属独立窗口**——它是 Phase 3 范畴，即 README §2.1.0 的 Wave **W4**（入口 = Wave W2 完成 + 启用条件命中），勿在此表挂靠。

### 2.4 已闭环基线（启动时可假定已完成）

Phase 1 全模块 + Phase 2a/2b-core/2b-org/2c 四阶段已交付：`make acceptance` 四档链式 **211 PASS / 0 FAIL**（**2026-08-31 实测**：87+66+26+32；断言数随脚本演进漂移，复验以实跑为准）、13 包集成测试 `-race` 全绿（**2026-08-31 实测**）；BK-11①（org_path 快照竞态 FOR SHARE）、CC1–3/HC2/HC3/MC2/MC3/P0/EC1/OP1 等历史发现均已修复并有回归（详见 [11 §6](../review/11-project-control.md)）。

## 3. 决策清单（启动时逐项过，维护于 [phase3 README §4](./README.md)）

> **⚠ 执行结构已修订（2026-09-02，design-decisions §22 + §23）**：六项所有者拍板落 §22——M1 降 🚦、HR 同步升主链（预留接口版）、activelist 拆两半、签发模型=指派给人、7c 完整引擎维持硬交付、引擎可替换约束；**随后 §23 重定位（2026-09-02）**：工单自研暂缓（内部引擎优先），§22 中 7c 引擎硬交付相关拍板被推翻，主链改为 M-E/M-A/M-HR/M-Mig（详见 [13-implementation-plan](./13-implementation-plan.md) §1）。执行细节以 13 号为准。

| 事项 | 建议 | 状态 |
|------|------|------|
| A2 迁移号 000017 归属 | 谁先启动谁占用，后者重排 | ✅ 已拍板（2026-08-31） |
| A3 `org_path` 快照列 vs 运行时 JOIN | 保留镜像列（FOR SHARE 兜底） | ✅ 已拍板（2026-08-31） |
| K8s vs Docker Compose | Compose + Nginx 足够 | 待拍板（建议沿用） |
| Redis / PG 高可用 | Sentinel / 云托管 Cluster | 待拍板（建议沿用） |
| 部署级分离时机 | Phase 3 末按需验证 | 待拍板 |
| 审批流引擎选型 | 手写 BranchedStateEngine | ~~待拍板（建议沿用）~~ **随 §23 暂缓**（对接内部平台，自研不做；翻案恢复） |
| 可观测性栈 / 工单事件机制 | 已决策（应用内开关 / L1） | ✅ |

## 4. 对话记录依赖项与历史登记复核（2026-08-31 审计结论）

- Phase 3 计划评估：P1-P5 阻塞项 + S1-S10 缺口（大部分已由 A/B 档吸收，A1 落档时核对剩余）
- 文档一致性：~~C-1~C-4 断链/索引~~ 已明确化为 **review/10 C1–C4**（处置见 A1②：C1 驳回维持、C2 已修、C3 → B10、C4 真开放）
- ticket-business 设计歧义（**对话评估编号**，非本表 B 档编号）：评估-B1 SLA 暂停态 / 评估-B3 通知主管 / 评估-B6 邮件矩阵（已并入 §2.2 B1）
- **09 合集抽查结论**：F-01/02（VISION）、F-03（activelist 链）、F-18（Redis requirepass）核实已修（历史行未回标）；F-21 → B7；F-31④/F-32 → 随手项；其余条目按批次闭环记录未逐条重验
- **代码级复审（2026-08-31 第二轮，以代码为准）**：BK-7 两半项已在代码落地（priority→400、page 钳制回显，ticket_handler.go:43-52）→ 关闭、删 B8；testutil 迁移清单仅缺 000002/000007 纯数据 seed（刻意排除，DDL 全齐非缺口）；非测试代码零 TODO/FIXME、无请求路径 panic、关键错误码与 api/errcode.md 一致；「已修复」声明（CC1/2/3、P0、MC2/3、BK-1/2/3、BK-15、OP1）逐项代码复核属实
- **并发/多实例审计（2026-08-31）**：工单模块无进程内可变状态、共享层全在 Redis/DB（Lua 限流、advisory lock、乐观锁、DB 时间戳）→ 可水平扩展；唯一多实例缺口 = Casbin 策略传播无 Watcher（跨模块，Phase 3 Step 2，已在 B9）；并发防护（CC1/2、BK-3/11/15、竞态双测试）逐项验证在位
- **00 §9 BK 全量复核**：BK-1/2/3/4/6/8/10/15 已关闭（BK-4 行漏关本次修正）；开放项 BK-5/7/9/11②/12/13/14 全部已归纳于 A/B/W 档

## 5. 变更记录

| 日期 | 说明 |
|------|------|
| 2026-08-31 | 初版：基于全量文档扫描（phase1/2/3 + review）归拢 A/B/W 三档 + 决策清单；基线 `c389156` |
| 2026-08-31（A 档清零） | A1/A4/A5/A6/A7 全部完成：HC1 事件补全（TestHC1）、BK-5 反向判重（TestBK5）、TC1-Go（TestTicket_Delete_AdminSucceeds）、SoD 落 design-decisions §20、review/10 C1–C4 处置、14 号断链注记；验证：集成 13 包 `-race` 全绿、acceptance 211/0。**A 档清零 = Phase 2 收官** |
| 2026-08-31（IW3 实施） | BK-18 管理闭环后端落地（迁移 000018 + 7 端点 + G2 校验 + 测试）；211/0 全链绿；前端照 12-frontend 另排期 |
| 2026-08-31（外评核验二） | 6 条核验：04 表格 Step 编号 +1 偏移修正（8/9/10 对齐正文）；README §2.4 迁移表刷新；**BK-19 登记**（TC-1 handler 测试）；TC-2/3/4 入随手项；ARCH-1/2/3 确认为 W1/W2 既定 |
| 2026-08-31（BK-12 实施） | BK-12 落地：org_roles 绑定/解绑/列表三端点（仅全局管理员、系统角色禁绑）+ parent_id 写侧（单调校验 child ≤ parent + 环检测 + ClearParent）；TestBK12 双测试；B6 关闭 |
| 2026-08-31（编号治理） | 消除 W 编号撞车：README Wave（W0–W4）独占「W」；独立窗口改 **IW1–IW3**；activelist 从本表移出归位 Wave W4（此前误挂 Phase 2 范畴表，且与 IW 序号冲突）；BK-16/17/18 的"随 W1 批次"改指 IW1 |
| 2026-08-31（activelist 收录） | 用户核出遗漏：activelist（ADR-003 已决策 Phase 3 启动后）未进计划文档——README §1.1/Wave 表补 W4 条目（入口=W2 完成+启用条件，含 E13 前置），检查单 §1/§2.3 同步；SSOT 维持 roadmap/ADR-003，仅指针 |
| 2026-08-31（Phase 3 补版） | README 增 §2.1.0 Wave 结构提案（W0-W3 + 7-0 设计期 + 四项待确认：Wave 采纳/W2 硬前置/12-frontend 新文档/7-0 三项设计建议）；§1.1 与文档索引补 12-frontend；检查单 B9 同步（6→7 份 + 10 号修订提示） |
| 2026-08-31（生态调研） | Duke1616 生态调研：新增 W3/BK-18（管理闭环，W1 后批次）；B1 吸收三结论（撤回/快照/审批人策略模型）；B9 补 eiam/etask 参考指针 |
| 2026-08-31（外评核验） | 断言数修正为实跑值 **211 PASS / 0 FAIL**（87+66+26+32，标注实测日期；原 215 不准确，静态统计 213 亦非运行值）；§4 编号重名标注「对话评估编号」；A5 补反向判重测试要求；10 号文档 §0 前置勾选 + 必坑计数勘误 + 设计深度缺口并入 B1 |
| 2026-08-31（审计） | 完整性审计：BK-1..15 全量状态复核（BK-4 漏关修正）；review/10 C1–C4 逐条核验落 A1②（C4 真开放、C1 驳回维持、C2 已修、C3→B10 新增）；09 合集抽查（F-01/02/03/18 核实已修、F-31④/F-32 入随手项）；确认无其他遗漏 |
| 2026-09-01（go-wind-admin 调研吸收） | 登记 **IW4**（行级过滤护栏 fail-closed，建议与 BK-19 同批）+ **B11**（审计治理：① L2/L3 判定日志随 W1/03-audit-l2、② 归档随 W2/Asynq）；03-audit-l2 建范围占位；11 §8 独立窗口表同步编号治理（W1/W2→IW1/IW2、补 IW3）；11 §8 B6 回填关闭；随手项补可选 Q5 组织赋角注记。调研结论：Provider 抽象不排期（接缝=自有 ResourceAuthorizer，挂 ReBAC 触发器）；「快照 vs 实时」无需新记录（Q5 已覆盖） |
| 2026-09-01（IW4 实施） | IW4 当日落地：`resource.Filter.Unscoped` 显式豁免 + `ticket_repo.List` fail-closed 哨兵 + admin/scope=all 两处显式化 + AST 调用点守护（`TestGuard_TicketRepoListCallSites`）+ 测试 4 个；全门禁绿（lint / 13 包单测+集成 `-race` / acceptance 87+66+26+32 FAIL=0）。改动未提交，随批准批次落 |
| 2026-09-01（org_type 收敛 000019） | **org_is_virtual**：`org_type` 四值枚举（1/2/3 实体细分 + 4 虚拟）收敛为 `is_virtual` 布尔（0/1 讨论后定 bool——类型级杜绝"无消费细分复发"；未传=false=实体为安全缺省）；API 契约 `org_type` → `is_virtual`（前端暂缓期无消费方，12-frontend 零引用实证）；层级不设 tier（path/nlevel 已三份表达，HR 多维归属需求出现时另议标签字段）。改动：迁移 000019（含 CHECK 缺席说明——bool 天生两值）+ model/binding（去 required，bool 零值即合法）+ org_service 5 处 + scope_resolver 锚点 + 测试字面量全量 sweep + acceptance 四脚本适配 + 03-org-enhance/architecture 定义更新 + 11 迁移地图 + swag。全门禁绿，未提交 |
| 2026-09-02（软删委托残留处置登记） | 外评实证「软删 org 历史工单对原 owner/祖先 owner 保留委托读写」（三处委托 SQL 无 `deleted_at` + 删守卫不查工单/owner；普通 Delete 因双轨同步常规不可达，vg 删除路径系统性产生）。拍板两步划界：① **BK-20 登记**（禁删有未结工单的组织，待实施 ~半天，acceptance 删组织用例需核对适配）；② 已结工单委托残留=档案连续性**登记不修**（显式断开=删除前 SetOwners 清空；翻案条件见 §21）。语义 SSOT 落 design-decisions **§21**；11 §6 加 BK-20 行 |
| 2026-09-02（Phase 3 执行结构修订） | 排期评审六项拍板落 **design-decisions §22**（SSOT）+ [13 号](./13-implementation-plan.md)全量同步 + README §2.1.0 修订注记：① M1 降 🚦（修订决议②，M2 硬依赖=Asynq 底座，防重按 02 号约定写码）；② HR 同步升主链 M2.5（HRFetcher 预留接口版，IW2 行同步）；③ M4 activelist 拆两半（writer 管道随 W2 / E13·G1·G3 蓝图 🚦 随其独立项目；独立数据库拍板认可）；④ 签发模型=指派给人（转派端点不做，viewer 能力=AssignMenus 运营配置）；⑤ 7c 完整引擎维持硬交付；⑥ 引擎可替换约束（WorkflowEngine 接口 + 实例状态通用/私有分层）。7-0 新增拍板项三项（writer 契约/通知跟人 vs 跟单/引擎接口契约）；README §4 A3 漂移同步修掉 |
| 2026-09-02（**Phase 3 重定位：工单自研暂缓（内部引擎优先，自研兜底）**） | 所有者拍板落 **design-decisions §23**（推翻 §22.5）：项目将迁移公司内部、**对接内部工单平台/引擎**——工单 Phase 2 现状封版（仅保 BK-20 数据安全修复），7a–7e/引擎/前端暂缓自研，10/12 号转对接参考。Phase 3 主线重排（13 号 §1 修订表）：M0 收窄 → M-E 事件基建（Asynq + 审计归档首任务）→ M-A activelist 独立实现（外部事件契约由 activelist 侧定义）→ M-HR HR 同步（内网迁移前置）→ M-Mig 迁移准备；B11① 可并入 M-E；工单对接形态与 Phase 2 资产处置 = 迁移时拍板 🚦。BK-19/随手项随封版后置；README/10/12/13 均已加重定位注记 |
| 2026-09-02（M-E 扩展为任务平台） | 自定义任务需求（预置 + 用户 Python/Shell 脚本）并入 M-E：Asynq 底座 + 预置任务（审计归档）+ 自定义脚本任务（安全边界 = 仅全局管理员可配，design-decisions §23 补充拍板）；参照 ginfast scheduler / xxl-job GLUE；人日 3–4 → 6–8 |
| 2026-09-02（M-SSO 新增） | 公司 SSO 对接拍板 OAuth2.0（design-decisions §24）：SSOProvider 接口预留版 + callback 签发自有 JWT（鉴权三层零改动）；JIT 默认关；与 M-HR 对账键一致、分工明确（SSO 管认证/HR 管账号数据）；13 号 §1 M-SSO 行，2–3 人日 |
| 2026-09-02（M-SSO 降 🚦） | SSO 设计定稿（§24）但实施后置：现在零代码预留，进内网拿到公司 OAuth2.0 接入信息后按蓝图实施（13 号 §1 M-SSO 行标 🚦） |
