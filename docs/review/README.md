# 审查与项目控制

本目录把当前控制、审查方法和固定时点证据分开，避免从旧流水推导现状。维护规则见[文档维护约定](../documentation.md)。

## 从这里开始

- [项目控制台](11-project-control.md)：当前能力矩阵、健康状态、81对迁移地图及历史分流出处。每次改动后回填；当前开放债与历史来源的现行去向进入[排期总账](planning-backlog.md)。
- [全面审计指南](audit-playbook.md)：可复用的范围、覆盖、证据与复验方法。每次审计另建本地执行卡，不把当前基线写成永久方法。
- [BK11死锁复盘与dbtx决议](13-bk11-pool-deadlock-2026-10-26.md)：根因/修复历史与已批准ticket试点共存；试点尚未实施，保留原路径供设计引用。

## 正式报告

首次目录整理与验证见[五仓文档归位记录](reports/documentation-reorganization-2026-11-08.md)，按原始要求再次核对的遗漏、修正与范围见[二次复核记录](reports/documentation-recheck-2026-11-08.md)；旧审计结论按各自冻结时点保留。

[reports/README.md](reports/README.md)索引全部正式报告。追代码修复先读[11-08复验](reports/zhuzhao-report-followup-2026-11-08.md)、[B27最终收口](reports/zhuzhao-ecosystem-audit-b27-phase5-closeout-2026-11-07.md)，再按需要读[11-06全量总报告](reports/zhuzhao-ecosystem-full-audit-2026-11-06.md)及分批原证据。B17并入B16、B22并入B18、B06为两份续批，不造空文件凑号。

报告保留原hash、失败、NOT_RUN、变异验证与运行边界；固定窗口通过不等于main/生产已承接。12/14权限矩阵已归reports，仍保留point-in-time性质；当前L1全角色/路由验证以B11所述窗口为准。

## 历史档案与规划来源

[archive/README.md](archive/README.md)收录原Phase1/2审查12篇，以及旧overview、执行计划和从控制台抽离的日期流水。合并报告可能是已删除原稿的唯一证据，保留发现编号、误报裁定与后继关系，不按日期批量删除。

[安全运营平台规划](../proposal/secops-platform/README.md)的10篇资料含已拍决策与未来能力设计，属于proposal来源；当前实施状态回到各phase README。历史报告与未来方案承担不同职责。

## 材料归属

正式报告进入reports；原始执行卡、日志、截图、快照和临时探针只放ignored `.codex/audits/<轮次>/`。共享索引链接纳管内容，本机原始材料按报告说明恢复，不把本机路径当他人可用入口。

## 全项目路线重规划

[全会话研究深度核验](reports/concern-depth-verification-2026-11-12.md)回读32条真实用户输入，逐点核65项；51项实质规划、10项部分深入、4项合理后置。[逐点证据](focus-depth-verification.json)给短原话、来源及尚不足的具体维度，不以纳入/排期代替研究。

[正式审查报告](reports/pre-phase6-replanning-2026-11-11.md)给基线、发现、研究取舍和验证限制；[总账](planning-backlog.md)及其JSON/source-map负责71包与全部来源处置，旧台账保留追溯。

[审计/详设收口](planning-closeout.md)给当前五步位置；[逐包设计深度](planning-design-status.json)与[所有者决策](planning-decisions.json)区分候选/边界、确认/未决，不再将规划落档等同全部详细设计完成。

[原始需求闭环审计](reports/pre-phase6-requirements-audit-2026-11-11.md)记录逐项承接与本轮纠漏；当前71包/511来源，依赖以总账的完整包/子出口语义核对，旧报告数量保持原时点。

[原规划能力连续性](roadmap-capability-continuity.md)与其JSON保留169条来源原子的价值/问题/更小路径/新波次；主读[全项目路线](../roadmap.md)和[渐进演进](../design/project-evolution-strategy.md)，不把Phase6近期切片当整个路线。

[新旧对照](roadmap-change-comparison-2026-11-12.md)先说明变化；[新版执行排期](execution-schedule.md)给13个发布切片、首批PR及子出口依赖。[本轮补研究](reports/near-term-research-and-schedule-2026-11-12.md)记录FD01/02/04/05新增证据，[八组状态](research-supplement-status.json)保留尚未补齐的FD03/06/07/08窗口，不将有排期当作研究完成。


## 开工前版本维护

[版本维护记录](reports/version-modernization-2026-11-12.md)汇总G07A/G07B前置的五仓源码/镜像/数据验证、首次失败与未执行边界；[组件清单](../../deployments/component-lock.json)与[恢复契约](../design/component-upgrade-data-contract.md)是实际版本与数据切换入口。

## 企业UAA接入重核

[现行UAA合同](../design/uaa-governance-integration.md)将公司指南转为去敏的身份/资源/角色/数据范围/撤权要求；E01-UAA-C/P/R已纳入总账，mock保留，真实接入是公司UAA纳管的企业模式首次正式激活的独立门。文档核对不代表完整设计冻结或真实集成验收通过。

## UAA与完整双模式后的全路线再审

[再审报告](reports/uaa-dual-mode-roadmap-review-2026-11-12.md)及[逐项记录](uaa-dual-mode-review.json)区分UAA前基线与上一双模式候选，修订8组跨授权/AI/前端/生产/排期问题；仍71包/13切片，新增投入未齐、日期未绑定。决策册现31项：28方向确认、B08/P-RESOURCE待绑定、UAA-P01先核公司约束再审业务取舍；不重复询问已确认原则。
