# 审查与项目控制

> 用途：核实当前实现、复验结论和追溯历史。当前计划统一进入[planning](../planning/README.md)，本目录保存控制台、审查方法和正式报告。

## 核实当前状态

[项目控制台](11-project-control.md)维护实现能力、开放债和迁移地图。当前实现、已批准设计、运行验证和实际部署分别核实；文档落档或旧报告通过均不代表新候选已验收。

开工前范围、待决、估算和设计深度统一看[规划入口](../planning/README.md)。近期设计看[模块合同](../phase6/module-detail-index.md)，业务试用准入看[首版计划](../phase6/first-usable-release-plan.md)。这里不复制一份随时间漂移的规划摘要。

## 复核一个结论

- [审计方法](audit-playbook.md)：明确范围、覆盖、证据、复验与收口；本轮执行卡留本地，不将某次基线写成永久方法。
- [正式报告索引](reports/README.md)：按准确日期、源码/制品/环境与结果复核。报告保留原hash、首次失败、重试、NOT_RUN及运行限制，后继结论通过链接说明。
- [BK11复盘](13-bk11-pool-deadlock-2026-10-26.md)：历史根因、修复和未实施试点分列。
- [历史档案](archive/README.md)：原Phase1/2审查、旧控制台和计划；包含独有决策/未闭环问题的记录持续保留。

Phase5收尾从[B27收口](reports/zhuzhao-ecosystem-audit-b27-phase5-closeout-2026-11-07.md)和[后续复验](reports/zhuzhao-report-followup-2026-11-08.md)进入；版本维护、规划审查、授权与前端等专项从正式报告索引进入。旧绿色不改写成本轮验证，合并报告的批号沿用原解释。

## 材料维护

[文档维护约定](../documentation.md)决定Git保留范围；[材料目录](../materials.md)区分正式证据与本地原材料。正式结论保留在本仓；原始调研、日志、截图、备份和纯操作收据归本地档案。2026-11-16规划搬迁的逐项去向见[迁移清单](../document-migration.json)，冻结来源仍保留历史语义。
