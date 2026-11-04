# review/ — 审查记录

本目录三层结构；长期审计方法与时点报告分开维护：

- [audit-playbook.md](./audit-playbook.md) — **可复用全面审计指南**：设计→实现→关键实践，跨仓、前端功能/交互/视觉、七阶段、证据与报告模板；每次审计另建执行卡，不在指南保存当次基线或结论。
- [生态首次全面审计执行卡（2026-11-04）](../../deliverables/software-company/zhuzhao-ecosystem-audit-plan-2026-11-04.md) — 本次五仓范围、阶段安排与覆盖游标；审计进行中，不代表整体通过。
- 正式批报告：[B01 基线与准备](../../deliverables/software-company/zhuzhao-ecosystem-audit-b01-baseline-2026-11-04.md)、[B02 认证撤权初核](../../deliverables/software-company/zhuzhao-ecosystem-audit-b02-auth-revocation-2026-11-04.md)；发现与证据按时点归档，后续修复另行复审。
- [B03 测试环境重建](../../deliverables/software-company/zhuzhao-ecosystem-audit-b03-environment-2026-11-04.md) — 记录授权范围、产物来源、构建/迁移差异与冒烟；整体审计未完成，B02 问题未修。
- [B04 修复治理与提交守卫](../../deliverables/software-company/zhuzhao-ecosystem-audit-b04-repair-governance-2026-11-04.md) — 已完成的流程/提交结构检查与验证证据；R01 实现候选尚未提交，P0 仍开放。

- **顶层（活文档+近期快照）**：
  - [11-project-control.md](./11-project-control.md) — 项目控制台**活文档**：能力矩阵 / 健康状态（遗留问题 SSOT，§6）/ Phase 3 前置随行分类（§8）/ 迁移地图。**每次改动后回填**（AGENTS.md 铁律），引用惯例 = 直接按路径 `docs/review/11-project-control.md`。
  - [12-b13-permission-matrix-2026-09-26.md](./12-b13-permission-matrix-2026-09-26.md) — B13 权限覆盖矩阵 point-in-time 快照（79 端点/8 盲区定性/IAM 四平面边界）——W1 词表终态基线，词表再动时新生一份而非改旧。
  - [14-b13-permission-matrix-2026-10-26.md](./14-b13-permission-matrix-2026-10-26.md) — B13 权限覆盖矩阵复审（phase5 终态 point-in-time：123 menu_apis 绑定+45 豁免全量对账——通过；B-19 销账件）。
  - [13-bk11-pool-deadlock-2026-10-26.md](./13-bk11-pool-deadlock-2026-10-26.md) — BK11 连接池死锁复盘（三层根因/已交付修复 `039aca6`/业界三档对照/彻底方案 dbtx 连接解析待拍板）——事务窗口嵌套池取类缺陷的 SSOT。

- **[archive/](./archive/)**：Phase 1–2 历史快照（00–10 共 13 篇）——各阶段系统性审查/验收/文档体系审查报告，发现与处置已全部闭环（闭环证据见 [11 §6](./11-project-control.md)）。编号即时间序，04 有两篇系同期各自成文的历史现状（维持不重排）。

- **后续新报告**：带日期戳的定期审计（`NN-topic-YYYY-MM-DD.md`）直接放顶层；闭环后移入 archive/。

引用本文目录：[docs/README.md](../README.md) §review。
