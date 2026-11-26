# 全项目规划入口

这页列出接下来要做的功能、交付顺序和开工前要确认的事项。已交付的功能见[项目控制台](../review/11-project-control.md)。

## 当前交付排期

- [首批人工漏洞整改（发布验收点 B2）](phase6-b2-kickoff-plan.md)演示同一漏洞影响同一责任人名下的多台资产时，怎样开一张单并逐台提交和复测。责任人不同则分单。[验收卡](phase6-b2-acceptance-cards.md)还写了两种容易遗漏的情况：停用账号后缓存没更新，其中一项整改被打回。
- [Phase 6 剩余开发顺序与验收](intranet-uaa-release-schedule.md#剩余开发顺序与验收)从人工整改第 6 项列到第 22 项的外部离线演练。[原工单需求 26 件](../phase6/06-ticket-completeness.md#26件当前对账2026-11-14)和[全站前端验收](../phase6/frontend-page-family-detail.md)逐项给出场景和判据。
- [入网、首发和待拍板](intranet-uaa-release-schedule.md#进内网后按什么顺序做)说明外部用 mock 验收所有公司内网依赖、离线演练后最后入网、以本地账号密码启用的顺序。[所有者议程](owner-decision-agenda.md#场景怎么选谁先准备)列出对应的选项和准备责任。原[机器排程](execution-schedule.md)供工日和依赖复算，首次进入和启用看当前排期。
- [开工计划](phase6-b2-kickoff-plan.md#本批实施与验收安排)列出获准在独立测试环境实施的九项工作，以及第 3、5、9 项的验收。实际开工时间随实施登记。真实试用单独报批，由所有者确认环境、人员、数据和停止办法。各模块还缺什么见[设计审查](../review/reports/phase6-detailed-design-audit-2026-11-20.md)。

## 按工作包找合同与交接

工作包是一组有明确交付内容的工作。派单时在任务卡上写清要做哪一组、参照哪段设计、用哪一版代码和通过哪些检查。历史阶段稿用于追查当初为何这么设计。

例如会话终结与认证增强收敛（工作包 C05）这次要解决的是：管理员已停用某账号，但 Redis 中缓存的登录状态还没更新，系统仍应拒绝旧登录。技术人员按[底座设计](../phase6/foundation-iam-module-detail.md#4-c05账号atrtpat-与停用恢复)查哪些登录凭据受影响，按[停用故障验收卡](phase6-b2-acceptance-cards.md#卡一-c05-停用已提交而撤销投影失败)检查旧登录被拒，并确认管理员能用原操作号查到停用是否成功。准备哪版程序和哪些测试服务见[首轮清单](phase6-b2-kickoff-plan.md#6-g04-隔离预检与-c05-首轮取证)。

- **找本单范围**：从[总账摘要](planning-backlog.md)按工作包或能力查范围和依赖；当前活跃项直接看同页的[合同、交接与准入](planning-backlog.md#当前活跃包的合同交接与准入)，再从[规划收口](planning-closeout.md)、[决策议程](owner-decision-agenda.md)核设计深度、待决及批准范围。
- **找有效设计**：先查[设计入口](../design/README.md)的共同合同，再顺着所属工作包查领域或阶段详设；当前行为另查[模块入口](../modules/README.md)、项目控制台及实际候选。找不到本包的有效合同或开工条件时，标为待确认，不借用其他阶段的详设补齐。
- **找实施、验收及发布交接**：按所属工作包读取该批交接与准入条件；通用评审格式复用[能力交付评审模板](../toolkit/templates/capability-delivery-review.md)，门禁和环境准备从[开发入口](../guides/README.md)和[运维入口](../ops/README.md)进入。当前活跃批次的具体链接见下方日常阅读。

## 常见编号怎么读

准备同一次真实试用时，会同时读到多个编号。**人工漏洞整改（B2）**是发布验收点，**会话终结与认证增强收敛（C05）**是其中一个工作包，**首发与恢复、保留政策（Q12）**是所有者议程。用下面的来源核对全文里的短称。

- **发布与决策**。隔离入网预演（B0）、基础扩展版（B1）、人工漏洞整改（B2）是[发布验收点](intranet-uaa-release-schedule.md#1-接下来怎么做)。合法抄送可查看及评论（B02）是[业务决策](planning-decisions.json)。P6.1 等编号指[技术分期](planning-backlog.md#波次与工作包)。B0-4 之类的条目沿用[旧启动批](../phase6/05-launch-batch.md)编号，跟读它对应的工作包时保留原出处。
- **工作包、实施子批与测试场景**。事务边界、组织删除与授权护栏（C04）和会话终结与认证增强收敛（C05）归[总账](planning-backlog.md#p61-受信业务底座)。本批分别做[业务提交时的共同授权守卫](../phase6/foundation-iam-module-detail.md#3-c04共同授权守卫与多实例)和[账号持久撤权与恢复](../phase6/foundation-iam-module-detail.md#4-c05账号atrtpat-与停用恢复)。[IAM 实施子批 C](../phase6/foundation-iam-module-detail.md#39-c共同屏障策略pair与两命令审计的准确纵切)负责共同屏障、策略投影与本地审计，测试场景 C-01 检查保存时如果发现权限已撤销，系统应拒绝这次操作，原业务记录保持不变。
- **缺陷与补齐动作**。停用组织后派生权限仍有效（AUTH-01）是[权限专项缺陷](../review/reports/authorization-design-implementation-review-2026-11-15.md)。来源与撤权补齐（AUTH-GAP-01）是[模型复评动作](../design/authorization-model-and-engine-choice.md#8-差距补齐动作与实施计划所有者要求落档)，覆盖多条缺陷和多个工作包。
- **议程、估算与时间**。真实试用窗口（Q12）查[所有者议程](owner-decision-agenda.md#q12-首发与恢复保留政策)。组合验收与兼容回归（Q1）、正式发布剩余工作（Q2）、共用操作回执基础（S2）查[首版净估册](local-core-release-estimate.json)。Phase 5 的流程图设计器用原段号 P5-S2。例子里的 D+1 表示样例业务日的次日，工单定义发布（D01）查[工作包总账](planning-backlog.md#p61-受信业务底座)。
- **净估行与排程切片**。本地身份和授权（S1）是[首版净估行](local-core-release-estimate.json)。隔离、有效检查与安全入口（S01）是一项[长期排程切片](execution-schedule.md)。查工作量时按各自的估算范围使用。
- **容易重名的 R 系**。业务意图持久交接（R01）和跨平台流程协同（R02）查[总账](planning-backlog.md)。同样的 R01、R02 在[真实 UAA 接入条件](uaa-real-integration-conditions.md#提供方与实施方要准备的条件)中分别指应用与认证合同、测试与正式环境资源。短号 R1、R2 在[验收卡](phase6-b2-acceptance-cards.md#卡二-b2-两条合成风险共票并逐项复测)里指同一模拟漏洞在两台资产上的受影响记录。在[旧阶段稿](../phase6/01-r1-ingest-ledger.md)里，R1 指风险接入与台账。[首版净估册](local-core-release-estimate.json)的 R2 指风险授权估算行。

## 日常阅读

- [规划结论](planning-closeout.md)列出已经决定和还需确认的事项。[模块设计](../phase6/module-detail-index.md)说明各模块要改什么，[审阅卡](../phase6/implementation-handoff.md#单维护者短审阅卡)供合并前逐项核对。
- 看全文复核后继的分工与验收：[11 月 20 日修复计划](audit-repair-followup-2026-11-20.md)。
- 看余项归属、前置和验收：[全文审计剩余待修项续修计划](audit-remainder-2026-11-20.md)与[逐项交接](../review/reports/audit-remainder-2026-11-20.md)。
- 看本轮存量问题的修复顺序和验收：[五仓审计整改实施计划](audit-repair-plan-2026-11-19.md)。
- 想在开工前弄懂系统怎样工作，读[系统理解与交付试行计划](system-understanding-and-delivery-plan.md)。先记录实际软件版本和功能，再选一件用户操作，从页面、权限检查到保存结果，分别讲清成功和被拒的情况。前三个交付小批再补使用故事和时间线，当前这些样例尚未整理。
- 看首版范围与变化：[首版计划](../phase6/first-usable-release-plan.md)、[前后对照](core-release-change-comparison.md)、[基础与逐期演进](../design/foundation-release-boundaries.md)。本地正式先行、完整UAA后继；准确状态和开工条件归各权威文档。
- 看工作包、依赖和时间：[总账摘要](planning-backlog.md)、[机器复算与历史底账](execution-schedule.md)、[首版净估](local-core-release-estimate.json)、[首版计价对账](core-release-accounting-reconciliation-2026-11-16.md)。相对排程不是实际开工或承诺日历。
- 看待决与输入承接：[决策议程](owner-decision-agenda.md)、[原话对账](user-requirements-reconciliation.md)、[后继输入](planning-followups.json)。简短确认只绑定明确前文，方向确认、合同冻结、实现和真实激活分别判断。
- 看存量修复的设计与当前承接：[2026-11-17修复计划](audit-repair-plan-2026-11-17.md) → [三仓合入及工作区收尾](../review/reports/audit-baseline-main-merge-2026-11-17.md)。目标账号保护等本批窄修已合main，未部署；完整IAM及其它后继仍按原包扣重，不叠加第二套总价。
- 看原计划的[全篇主线复核](../review/reports/audit-repair-ledger-recheck-2026-11-20.md)及[原始分批](audit-repair-plan-2026-11-17.md#6-2026-11-18-同类复核与分批承接)。先查主线已交切片，再核任务对账、撤权、模块校验、可靠通知等开放出口。
- 看长期路线及补充：[路线图](../roadmap.md)、[渐进演进](../design/project-evolution-strategy.md)、[路线变化](roadmap-change-comparison-2026-11-12.md)、[互通增量](capability-interconnect-supplement.md)、[旧范围处置](legacy-trigger-disposition.md)。Phase稿保留来源和验收映射，跨阶段规则归领域合同。

## 机器事实的归属

工作包/依赖归[planning-backlog.json](planning-backlog.json)，设计成熟度归[planning-design-status.json](planning-design-status.json)，政策状态归[planning-decisions.json](planning-decisions.json)，新增净估和扣重归[planning-scope-estimates.json](planning-scope-estimates.json)及[后继净估册](post-baseline-scope-estimates.json)。[闭环册](planning-closure.json)绑定冻结来源、动作和必要检查；[排程模型](execution-schedule-model.json)生成[排程JSON](execution-schedule.json)与人读摘要。不要手改生成数字或另建一套报价。

UAA完整双模式的边界、公共实现和条件真实出口见[补估说明](uaa-implementation-estimate.md)/[数字册](uaa-implementation-estimate.json)、[真实条件](uaa-real-integration-conditions.md)/[条件册](uaa-real-integration-conditions.json)。公开[合成mock](../design/examples/uaa-reference-mock/README.md)用于复验合同，不能证明真实IdP/MQ已接通。新增日志审计与其他后继范围按净估册、实际选项及开工窗口纳入。

来源追溯还可查[来源映射](planning-source-map.json)、[文档范围](planning-document-scope.json)、[能力连续性](roadmap-capability-continuity.md)/[逐项JSON](roadmap-capability-continuity.json)、[全会话再核](conversation-reaudit.md)/[矩阵](conversation-reaudit.json)、[关注点深度](focus-depth-verification.json)和[补研究窗口](research-supplement-status.json)。这些材料各有截止；有来源或排期不等于内容已审透。

## 迁移与历史

2026-11-16将39份原`docs/review/`根级规划材料归入本目录，原路径到新路径逐项见[迁移清单](../document-migration.json)。脚本、测试与五仓现行引用同步迁移；`make planning-check`继续校验。冻结JSON保留原字节及其旧来源字符串，固定commit的历史地址不解释成当前文件地址，闭环证据中的hash不随搬目录重算。

规划过程、独立反例、失败与验证边界仍保留在[正式报告](../review/reports/README.md)；旧Phase1–5和历史台账由[历史档案](../review/archive/README.md)承接。查看原始调研/日志时按[材料目录](../materials.md)定位本地归档，公共文档不依赖个人路径存在。
