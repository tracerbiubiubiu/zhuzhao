# 全项目规划入口

> 用途：集中查看当前范围、顺序、设计深度和待决；范围：五仓共同计划。目录迁移不改变业务状态、估算、依赖或所有者决定。产品是否实现以[项目控制台](../review/11-project-control.md)及实际源码/验证为准。

## 日常阅读

- 看当前结论、设计深度及实施出口：[规划收口](planning-closeout.md) → [模块合同入口](../phase6/module-detail-index.md) → [三节短审阅卡](../phase6/implementation-handoff.md#单维护者短审阅卡)。
- 看首版范围与变化：[首版计划](../phase6/first-usable-release-plan.md)、[前后对照](core-release-change-comparison.md)、[基础与逐期演进](../design/foundation-release-boundaries.md)。本地正式先行、完整UAA后继；准确状态和开工条件归各权威文档。
- 看工作包、依赖和时间：[总账摘要](planning-backlog.md)、[排期摘要](execution-schedule.md)、[首版净估](local-core-release-estimate.json)、[首版计价对账](core-release-accounting-reconciliation-2026-11-16.md)。相对排程不是实际开工或承诺日历。
- 看待决与输入承接：[决策议程](owner-decision-agenda.md)、[原话对账](user-requirements-reconciliation.md)、[后继输入](planning-followups.json)。简短确认只绑定明确前文，方向确认、合同冻结、实现和真实激活分别判断。
- 看长期路线及补充：[路线图](../roadmap.md)、[渐进演进](../design/project-evolution-strategy.md)、[路线变化](roadmap-change-comparison-2026-11-12.md)、[互通增量](capability-interconnect-supplement.md)、[旧范围处置](legacy-trigger-disposition.md)。Phase稿保留来源和验收映射，跨阶段规则归领域合同。

## 机器事实的归属

工作包/依赖归[planning-backlog.json](planning-backlog.json)，设计成熟度归[planning-design-status.json](planning-design-status.json)，政策状态归[planning-decisions.json](planning-decisions.json)，新增净估和扣重归[planning-scope-estimates.json](planning-scope-estimates.json)及[后继净估册](post-baseline-scope-estimates.json)。[闭环册](planning-closure.json)绑定冻结来源、动作和必要检查；[排程模型](execution-schedule-model.json)生成[排程JSON](execution-schedule.json)与人读摘要。不要手改生成数字或另建一套报价。

UAA完整双模式的边界、公共实现和条件真实出口见[补估说明](uaa-implementation-estimate.md)/[数字册](uaa-implementation-estimate.json)、[真实条件](uaa-real-integration-conditions.md)/[条件册](uaa-real-integration-conditions.json)。公开[合成mock](../design/examples/uaa-reference-mock/README.md)用于复验合同，不能证明真实IdP/MQ已接通。新增日志审计与其他后继范围按净估册、实际选项及开工窗口纳入。

来源追溯还可查[来源映射](planning-source-map.json)、[文档范围](planning-document-scope.json)、[能力连续性](roadmap-capability-continuity.md)/[逐项JSON](roadmap-capability-continuity.json)、[全会话再核](conversation-reaudit.md)/[矩阵](conversation-reaudit.json)、[关注点深度](focus-depth-verification.json)和[补研究窗口](research-supplement-status.json)。这些材料各有截止；有来源或排期不等于内容已审透。

## 迁移与历史

2026-11-16将39份原`docs/review/`根级规划材料归入本目录，原路径到新路径逐项见[迁移清单](../document-migration.json)。脚本、测试与五仓现行引用同步迁移；`make planning-check`继续校验。冻结JSON保留原字节及其旧来源字符串，固定commit的历史地址不解释成当前文件地址，闭环证据中的hash不随搬目录重算。

规划过程、独立反例、失败与验证边界仍保留在[正式报告](../review/reports/README.md)；旧Phase1–5和历史台账由[历史档案](../review/archive/README.md)承接。查看原始调研/日志时按[材料目录](../materials.md)定位本地归档，公共文档不依赖个人路径存在。
