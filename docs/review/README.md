# 审查与项目控制

- **首版读这两份即可**：[阶梯/日期与试用](../phase6/first-usable-release-plan.md)和[以前→现在/基础→增强对照](core-release-change-comparison.md)；原始源码重核在报告中，不要求所有者逐篇阅读。

> **用途**：区分当前控制/规划、审查方法和固定证据；本页仅作导航。**权威**：各机器台账、领域合同及准确候选收据；历史报告不承担当前状态。维护规则见[文档约定](../documentation.md)。

## 从这里开始

- **先理解交付边界**：[基础版与逐期演进](../design/foundation-release-boundaries.md)是2026-11-14最新范围说明：先现有底座试验、再真实扩展接缝；完整长期排程不是首批工期。[本轮复核](reports/foundation-boundaries-2026-11-14.md)说明依赖修正和未完成项。

- **核逐次要求是否处理**：原截至F003的65条输入/93点保留在[全会话再核](conversation-reaudit.md)及[矩阵](conversation-reaudit.json)，后继见[续源索引](planning-followups.json)。F006明确确认F004五组及Q01–Q06方向，新增公开UAA mock和统一日志审计；F005公开资产/测试资源前提保持。旧窗口不自动覆盖新输入，方向/样例不等于产品或真实激活。
- **看这次进度、详设深度和待决**：先读[规划收口](planning-closeout.md)、[剩余决策议程](owner-decision-agenda.md)与[按Phase深度概览](planning-closeout.md#各phase和模块细化到哪里)，交付时读[三节短审阅卡](../phase6/implementation-handoff.md#单维护者短审阅卡)。议程Q01–Q06方向已确认，Q07–Q13及生产实参按窗口绑定，不把13阅读组当新增门禁。
- **查已实现能力与开放债**：读[项目控制台](11-project-control.md)的能力矩阵、健康状态和迁移地图，再进入对应模块。
- **查后续范围/先后/新增差额**：读[总账](planning-backlog.md)、[执行排期](execution-schedule.md)与[互通增量](capability-interconnect-supplement.md)。业务T0未启动；CE五项已按有限范围计价，旧价和当前总价均不包干全部未来场景。
- **复核一个结论**：按下方审查方法与准确时点报告找证据，不需从全部历史流水推导当前状态。

## 全项目路线重规划

### 人读结论与实施交接

- [全项目路线](../roadmap.md)、[渐进演进](../design/project-evolution-strategy.md)、[新旧对照](roadmap-change-comparison-2026-11-12.md)：说明承接、范围和次序变化。
- [设计/交接状态](planning-closeout.md)、[总账摘要](planning-backlog.md)、[13切片已计价排期](execution-schedule.md)：说明可审候选与剩余冻结窗口，不等于全部详设或实施完成。
- [能力互通/一致性增量](capability-interconnect-supplement.md)：三条代表链、共同执行路径、全栈一致性及新增净估范围。

### 机器权威与来源追溯

- [工作包JSON](planning-backlog.json)：唯一工作包/依赖/既有粗估；[设计深度](planning-design-status.json)、[所有者决策](planning-decisions.json)分别负责成熟度和政策状态。
- [来源映射](planning-source-map.json)、[文档范围](planning-document-scope.json)及[能力连续性](roadmap-capability-continuity.md)/[逐原子JSON](roadmap-capability-continuity.json)：追溯旧模块价值及处置，来源数不等于独立功能数。
- [原始要求对账](user-requirements-reconciliation.json)负责截至本轮62条用户消息＋1明确目标的来源/证据索引；[旧32条关注点深度](focus-depth-verification.json)、[补研究窗口](research-supplement-status.json)保留其原截止和后继，不用有排期替代研究完成。
- [排程JSON](execution-schedule.json)、[互通增量JSON](capability-interconnect-supplement.json)：前者绑定冻结输入与已确认规划产能，后者保留新范围的未估状态。机器文件供校验/工具使用，日常阅读优先Markdown结论。

### 规划过程与核验出处

[正式重规划审查](reports/pre-phase6-replanning-2026-11-11.md)、[原始需求闭环审计](reports/pre-phase6-requirements-audit-2026-11-11.md)、[截至32条的深度核验](reports/concern-depth-verification-2026-11-12.md)和[补研究/排期记录](reports/near-term-research-and-schedule-2026-11-12.md)保留各轮来源、证据和限制；数量/状态按当时冻结窗口理解，当前结论回到上方权威入口。

## 开工前版本维护

[维护记录](reports/version-modernization-2026-11-12.md)汇总G07A/B隔离候选、数据恢复、首次失败与未执行边界；[组件清单](../../deployments/component-lock.json)和[恢复契约](../design/component-upgrade-data-contract.md)给版本/数据切换入口。本地候选验证不等于main或共享环境已切换。

## 企业UAA接入重核

[现行UAA合同](../design/uaa-governance-integration.md)负责完整本地IAM与私有adapter企业模式、来源管理权及撤权边界。E01-UAA-C/P/R各有公共合同、公共实现和条件真实出口；mock长期保留，真实企业激活单独验收，尚未接通不影响独立模式。

F006要求公司文档不可导出时先用公开实践和既有规划，[离线合成mock](../design/examples/uaa-reference-mock/README.md)保存可复验规范化合同，不实现真实IdP/MQ或替代产品P。统一运行日志、可信业务审计与领域事实的分工/缺口/分期见[日志审计设计](../design/unified-observability-audit.md)，其新增范围不能默塞旧G/I/UAA预算；[后继净估册](post-baseline-scope-estimates.json)登记OBS22–33日候选、F004未完净估；B1所选新增范围在B1开工前整合，增强按对应包开工窗口处理，不把旧基线加局部增量当完整新总价。

## UAA与完整双模式后的全路线再审

[再审报告](reports/uaa-dual-mode-roadmap-review-2026-11-12.md)和[逐项记录](uaa-dual-mode-review.json)保留当轮跨授权/AI/前端/生产/排期问题；后继[UAA补估](uaa-implementation-estimate.md)/[唯一数字注册表](uaa-implementation-estimate.json)补齐有界工程量，[真实条件](uaa-real-integration-conditions.md)/[条件JSON](uaa-real-integration-conditions.json)给进入、产出、pre_apply、收口和具体业务确认场景。

当前决策数量/未决项以决策册为准；资源以排程resource_assumptions和已确认问答为准。历史“P/R未计价、投入未绑定”不覆盖后继补估；新增互通/旧子范围及六项基价复估由净估册扣重并入当前排程，不混进历史UAA数字。

## 审查方法

- [全面审计指南](audit-playbook.md)：范围、覆盖、证据、复验与收口；每轮执行卡另建本地，不将当前基线写成永久方法。
- [BK11死锁复盘与dbtx决议](13-bk11-pool-deadlock-2026-10-26.md)：历史根因/修复与尚未实施的ticket试点分列。
- [真实前端首组](reports/frontend-first-baseline-2026-11-12.md)：实际观察、失败过程和未覆盖范围，不代表全站验收。

## 正式报告

[reports/README.md](reports/README.md)索引固定时点报告。文档归位及复核见[首次整理](reports/documentation-reorganization-2026-11-08.md)和[二次复核](reports/documentation-recheck-2026-11-08.md)；代码收尾从[11-08后续复验](reports/zhuzhao-report-followup-2026-11-08.md)、[B27收口](reports/zhuzhao-ecosystem-audit-b27-phase5-closeout-2026-11-07.md)进入，需要时再读[11-06全量报告](reports/zhuzhao-ecosystem-full-audit-2026-11-06.md)及原分批证据。

报告保留原hash、失败、NOT_RUN、变异和运行边界；12/14权限矩阵已归reports，仍为固定时点材料。B17并入B16、B22并入B18、B06为两份续批，不造空文件凑号，也不将旧绿色改成本轮验收。

## 历史档案与规划来源

[archive/README.md](archive/README.md)保留原Phase1/2审查、旧overview/计划及控制台历史；合并报告可能是原发现的唯一证据，不按日期批量删除。[安全运营平台规划](../proposal/secops-platform/README.md)保留已拍决策与未来提案来源，适用/替代关系回到现行合同/阶段入口。

## 材料归属

正式报告进reports；原始执行卡、日志、截图、快照和临时探针只在ignored本地目录。共享索引只链接纳管内容，不把本机原材料路径当他人可用入口。当前规划仍保留原文件位置，本轮只分清导航；将来若迁入planning目录，须同批核消费者、引用和冻结来源，历史输入不回写。

## 变更记录

- 2026-11-13：将当前结论和机器权威前置，方法/固定报告/历史后置；保留原文件及独有出处，减少重复计数和日期追加式摘要。


## 2026-11-13整会话补齐入口

[原话对账](user-requirements-reconciliation.md)说明此前哪些确实不足及本次补法；[闭环机器册](planning-closure.json)绑定来源、六组动作、证据和必需检查。[净估注册表](planning-scope-estimates.json)补CE/LS及既有包复估；[独立审查](reports/design-gap-independent-review-2026-11-13.md)保留实际反例、修订和独立资格边界。最终完整门禁与提交以当批准确输入/结果为准，不能从这些入口存在推断已全绿。


[审计窗口后的确认](planning-followups.json)承接原U63快照以后的明确答复；F001-B09已确认人工本地协作/自动暂停，原审计证据不扩写成覆盖后续全部消息。
