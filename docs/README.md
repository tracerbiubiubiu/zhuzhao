# zhuzhao 文档入口

> **2026-11-11现行入口**：Phase5已整合五仓main，后续以[新路线](roadmap.md)、[平台演进](design/platform-evolution.md)、[AI设计](design/ai-capabilities.md)及[排期总账](review/planning-backlog.md)为准；原阶段规划保留细节与历史，未开始功能实施。

先按任务选择入口。本文档树区分**当前实现、已批准后续方案、历史时点记录**；“已拍板”不等于已落码，“历史门禁通过”不等于当前候选通过。

## 第一次接手

1. 读 [产品边界](vision.md)，知道系统负责什么。
2. 读 [架构导览](design/architecture.md)，理解五仓、三层鉴权与可靠性边界。
3. 按 [开发指南](guides/development.md) 准备工具和隔离环境。
4. 查 [项目控制台](review/11-project-control.md) 的能力矩阵、健康状态和迁移地图，再进入具体模块。

## 按任务找文档

| 任务 | 入口 | 内容边界 |
|---|---|---|
| 本地开发、生成、测试 | [guides/](guides/README.md) | 可操作步骤及预期结果 |
| 理解模块 | [modules/](modules/README.md) | 当前调用链、数据与权限 |
| 理解架构与决策 | [design/](design/README.md)、[adr/](adr/README.md) | 架构解释；ADR 保留采纳/替代/实施状态 |
| 查 API | [api/](api/README.md) | 响应封装、错误码登记、Swagger |
| 部署、备份、故障处理 | [ops/](ops/README.md) | 操作手册；测试账号唯一台账在 runbook §4 |
| 当前状态、审计证据 | [review/](review/README.md) | 控制台、方法、正式日期报告和必要历史 |
| 查阶段设计/后续计划 | [路线图](roadmap.md)、[proposal/](proposal/README.md) | 阶段范围与设计，不能直接当作完成证明 |

## 阶段导航

> **2026-11-11 当前状态**。Phase 5 阶段交付与工作区收尾已确认，剩余问题和待验记录已移交 Phase 6。后续进行全项目重新设计规划，既有后续阶段方案、接口、波次和估算作为输入。交付、资源清理实际终态和最终收据见 [Phase 5 验收记录 §8](phase5/12-scenarios-and-tests.md#8-phase-5-收尾与-phase-6-承接2026-11-11-整理) 与 [运行手册](ops/runbook.md)。

[Phase 1](phase1/README.md) → [Phase 2](phase2/README.md) → [Phase 3](phase3/README.md) → [Phase 4](phase4/README.md) 是已交付阶段的设计与记录；[Phase 5](phase5/README.md) 是已合main的能力与历史债务入口。[Phase 6](phase6/README.md) 保留既有设计与待办，供 Phase 5 收尾后的全项目重规划；[Phase 7](phase7/README.md)、[Phase 8](phase8/README.md)、[Phase 9](phase9/README.md) 为未来方向，立项状态以各自页头为准。

Phase5合并已完成；当前代码基线见控制台，本轮规划在独立codex分支审阅。生产发布与后续功能启动另按对应验收记录，不从目录名推断。

## 五仓各自维护，结构统一

- **zhuzhao**：IAM、工单、权限、网关和主应用运维，本树为权威。
- **zhuzhao-ui**：[UI 文档](https://github.com/tracerbiubiubiu/zhuzhao-ui/tree/main/docs)，维护前端开发、组件、测试和发布。
- **zhuzhao-utils**：[公共库文档](https://github.com/tracerbiubiubiu/zhuzhao-utils/tree/main/docs)，维护包契约、使用和版本纪律。
- **taskrunner**：[任务服务文档](https://github.com/tracerbiubiubiu/taskrunner/tree/main/docs)，维护执行、调度、回调和服务部署。
- **activelist**：[动态数据服务文档](https://github.com/tracerbiubiubiu/activelist/tree/main/docs)，维护当前 PostgreSQL 模型、Schema、CRUD/导入和部署。

链接指向已整合的main，准确提交与工作区状态以各仓 Git 及本轮收据为准。跨仓只引用契约，不复制整份实现说明。

## 维护规则

[工程公约](standards.md) 管代码/API/权限/迁移与协作；[文档维护规则](documentation.md) 管分类、命名、状态、链接和迁移；[致谢](acknowledgments.md) 记录设计参考。根 `AGENTS.md` 是协作协议。

正式报告放 `review/reports/`，必要旧设计放 `review/archive/`；`doc/` 保持本地参考，执行卡、原始日志、截图与临时输出放 ignored `.codex/` 等目录。Swagger 的 `docs.go`、`swagger.json`、`swagger.yaml` 保留现有生成路径，勿手改。

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-11-08 | 五仓按各自 docs 维护，重写任务入口并区分当前/未来/历史；阶段目录和 Swagger 消费路径保留。 |
| 2026-11-11 | 同步本阶段工作区收尾与问题移交入口，后续阶段设计作为全项目重规划输入。 |
