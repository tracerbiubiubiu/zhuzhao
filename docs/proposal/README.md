# 方案来源与后续规划

这里同时保留早期方案来源和仍待实施的提案。**目录归属不等于“全部未实现”，提案被部分实现也不等于全文已成为当前规格。** 先看各文件头的状态，再进入对应模块或阶段文档。

| 文档 | 当前用途 | 阅读去向 |
|---|---|---|
| [overview](overview.md) | 2026-08 方案总览，历史入口 | 当前定位看 [VISION](../vision.md)，实现看 [模块入口](../modules/README.md) |
| [auth-design](auth-design.md) | 双 Token/分层鉴权的方案来源；部分旧缓存、设备与审计预案未实施 | [认证](../modules/auth.md)、[鉴权](../modules/authz.md) |
| [data-init](data-init.md) | 显式迁移、幂等种子的设计来源；示例不是执行手册 | [运行手册](../ops/runbook.md)、[迁移地图](../review/11-project-control.md) |
| [resource-model](resource-model.md) | Resource/Registry 已落地；其他资源和独立 Enforcer 是示例或备选 | [鉴权](../modules/authz.md)、实际工单 Resource |
| [deployment-evolution](deployment-evolution.md) | 部署档位参考与早期拆分备忘 | [现行架构](../design/architecture.md)、[部署](../ops/deployment.md) |
| [hr-directory-sync](hr-directory-sync.md) | M-HR 目标设计；来源字段已具备，完整同步未交付 | [Phase 6 启动批](../phase6/05-launch-batch.md)；未决项保留在原文 |
| [secops-platform](secops-platform/README.md) | 安全流程平台愿景与已拍决策来源；不代表全部已交付 | 按各篇状态进入 Phase 6–9 |

操作排障从 `docs/ops/` 进入；历史时点证据从 [review](../review/README.md) 进入。批准新范围仍遵循 VISION 与决策日志的级联纪律。
