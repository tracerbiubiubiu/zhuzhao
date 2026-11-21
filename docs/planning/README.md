# 全项目规划入口

从本页可以查到下一批交付什么、缺什么输入，以及哪些结果要由所有者或实跑确认。五仓的共同计划在这里维护，现有产品能力见[项目控制台](../review/11-project-control.md)。

## 当前交付排期

- **想知道第一批做成什么样**。打开[B2 开工计划](phase6-b2-kickoff-plan.md)，走读同一张整改票上的两项风险如何派单、办结和逐项复测，再看[两张验收卡](phase6-b2-acceptance-cards.md)里账号停用后 Redis 同步失败、以及一个风险项被打回时要观察什么。
- **想知道工作顺序和用时**。打开[交付排期](intranet-uaa-release-schedule.md)。先交账号撤权和提交时鉴权，再交工单定义、证据转换和人工风险整改，之后才是任务、清单和消息组合、本地正式运行与 UAA 纳管。
- **想知道还缺什么**。开发前查[需确认的输入](owner-decision-agenda.md#详设审计后的排产选择)，包括实际开工日和授权。真实试用还需单独确认环境、人、数据及停止手段。技术设计对照[模块缺口与验收](../review/reports/phase6-detailed-design-audit-2026-11-20.md)。

## 按工作包找合同与交接

这是跨阶段派单和交接的固定入口。具体的工作包、合同锚点和候选版本放本轮任务卡；阶段稿保留对应时期的详设和交接，不作为所有任务的默认输入。

例如接到 C05 时，先在[总账的可信底座](planning-backlog.md#p61-受信业务底座)查账号安全状态和依赖，再看[底座设计](../phase6/foundation-iam-module-detail.md#4-c05账号atrtpat-与停用恢复)规定哪些凭据受影响。安排验证时用[停用故障验收卡](phase6-b2-acceptance-cards.md#卡一-c05-停用已提交而撤销投影失败)核旧令牌是否被拒，以及管理员如何查同一次停用的结果。实际候选、隔离服务和取证输入由[首轮清单](phase6-b2-kickoff-plan.md#6-g04-隔离预检与-c05-首轮取证)逐项绑定。

- **找本单范围**：从[总账摘要](planning-backlog.md)按工作包或能力查范围和依赖；当前活跃项直接看同页的[合同、交接与准入](planning-backlog.md#当前活跃包的合同交接与准入)，再从[规划收口](planning-closeout.md)、[决策议程](owner-decision-agenda.md)核设计深度、待决及批准范围。
- **找有效设计**：先查[设计入口](../design/README.md)的共同合同，再顺着所属工作包查领域或阶段详设；当前行为另查[模块入口](../modules/README.md)、项目控制台及实际候选。找不到本包的有效合同或开工条件时，标为待确认，不借用其他阶段的详设补齐。
- **找实施、验收及发布交接**：按所属工作包读取该批交接与准入条件；通用评审格式复用[能力交付评审模板](../toolkit/templates/capability-delivery-review.md)，门禁和环境准备从[开发入口](../guides/README.md)和[运维入口](../ops/README.md)进入。当前活跃批次的具体链接见下方日常阅读。

## 日常阅读

- 看当前结论、设计深度及实施出口：[规划收口](planning-closeout.md) → [模块合同入口](../phase6/module-detail-index.md) → [三节短审阅卡](../phase6/implementation-handoff.md#单维护者短审阅卡)。
- 看全文复核后继的分工与验收：[11 月 20 日修复计划](audit-repair-followup-2026-11-20.md)。
- 看余项归属、前置和验收：[全文审计剩余待修项续修计划](audit-remainder-2026-11-20.md)与[逐项交接](../review/reports/audit-remainder-2026-11-20.md)。
- 看本轮存量问题的修复顺序和验收：[五仓审计整改实施计划](audit-repair-plan-2026-11-19.md)。
- 补齐开工前的系统理解、演进脉络及交付回顾：[系统理解与交付试行计划](system-understanding-and-delivery-plan.md)。先准备基线卡、能力地图、同旅程成功/拒绝纵切与启动讲解的最小样例；三个故事提纲及时间线随前三个实际交付小批整理。Git维护正式知识与空白模板，执行实例本地归档；实例尚未整理，不代表已掌握或新增技术门禁。
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
