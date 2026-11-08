# Phase 4：已收官的管理前端与配套能力

> 2026-11-08整理：本页先给当前阅读入口，下面保留原阶段说明/修订作为历史依据。本阶段前置为已收口的基础服务；交付UI壳层/system/ticket/al/task/audit、菜单B案与配套增强。后续业务深水与安全复验在Phase5推进；本阶段报告只证明其注明时点，不替代当前门禁。

## 阅读顺序与边界

1. [00-planning-inventory.md](00-planning-inventory.md)
2. [01-frontend-design.md](01-frontend-design.md)
3. [02-implementation-plan.md](02-implementation-plan.md)
4. [03-scenarios-and-tests.md](03-scenarios-and-tests.md)

当前模块接口看 [modules](../modules/README.md)，工程规则看 [standards](../standards.md)，操作看 [运维](../ops/README.md)。能力状态与迁移编号查 [项目控制台](../review/11-project-control.md)；当前业务段排期查 [Phase5](../phase5/README.md)。新批次仍需满足自身前置与门禁，不能从历史已完成推导所有未来能力已启用。

## 原阶段说明与修订记录

以下保留原时点的目标、结果与决策；旧分支名、数量、待办与凭据样例不覆盖现行约定。

# phase4/ — Phase 4 规划与交付文档

Phase 4 = 前端 zhuzhao-ui 落地主轴 + 后端穿插增强。

| 文件 | 内容 |
|---|---|
| [00-planning-inventory.md](./00-planning-inventory.md) | 总盘点（拍板/技术债/触发项/雷达表） |
| [01-frontend-design.md](./01-frontend-design.md) | **前端工程设计 SSOT**（技术栈/壳层/契约/门禁/里程碑） |
| [02-implementation-plan.md](./02-implementation-plan.md) | **实施计划 SSOT**（批次总览/编号分配/门禁纪律/完成定义） |
| [03-scenarios-and-tests.md](./03-scenarios-and-tests.md) | **场景×测试矩阵 SSOT**（S1–S18 用户旅程 + 覆盖映射） |

> 前端仓 zhuzhao-ui 的操作文档（README/AGENTS）不在此——各仓自理（standards §12-3）。
