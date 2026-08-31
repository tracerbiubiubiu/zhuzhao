# AGENTS.md — 项目协作协议

> 本文件对每次在本仓库工作的 AI/协作方生效。目标：**让代码改动对项目所有者始终可审、可验证、可掌控**。

## 项目速览

Go 模块化单体 **IAM + 工单** 系统（三层鉴权 + 资源级工单）。文档驱动：`docs/` 是权威设计源，改动必须与文档同步。项目能力总览见 `docs/review/11-project-control.md`（**每次改动后须回填该文件的能力矩阵/状态**）。

## 常用命令（交付前必须验证）

```bash
make lint              # 静态检查
make test-unit         # 单元测试
make test-integration  # 集成测试（需 PG/Redis）
make test-cover        # 覆盖率报告
make acceptance        # 【门禁】四档全量验收：phase1 → 2a → 2b → 2c（链式）
make migrate-up        # 应用迁移
make swag              # 重新生成 Swagger
```

**门禁纪律**：不得把未通过 `make acceptance` 的改动标记为"完成"；无法运行门禁时必须如实说明原因与影响。

## 铁律：每次代码改动必须附带"变更评审说明"

任何对 `internal/`、`migrations/`、`configs/`、`cmd/`、`docs/` 的改动，交付时在回复中固定附带三节：

1. **改动摘要**：改了哪些文件、为什么改、核心逻辑如何走（≤10 行，人话，不贴代码）。
2. **影响面**：涉及哪些模块 / 接口签名 / 数据库迁移 / 权限码 / 配置项 / API 路由；是否有破坏性变更。
3. **验证证据**：跑了哪些测试与门禁、结果、覆盖率变化；未覆盖的路径明确列出。

> 目的：项目所有者只 review 摘要即可审批，无需逐行 diff。摘要缺失或含糊 = 交付不合格。

## 数据库迁移规范

- 每个迁移 **up/down 成对**；编号**全局唯一**，新迁移前先核对 `migrations/` 与 `docs/phase2/README.md §2.4`、`docs/phase3/` 的编号占用表，**禁止编号冲突**。
- 修改已有表语义必须说明对存量数据的兼容处理。
- 变更后必须 `make migrate-up` 并跑相关 acceptance。

## 代码约定

- 遵循现有分层：`handler → service → repository`，工单在 `internal/service/ticket/`（领域隔离，为未来拆分准备）。
- 权限判定统一走三层鉴权，不在业务层手写绕过。
- 错误处理用 `internal/pkg/errcode`，DB 错误经 `pgerr.go` 映射，禁止 raw 500 泄漏内部细节。
- 新增依赖（go.mod）必须说明理由与替代方案。

## 文档同步

改动触及能力/接口/权限/迁移时，同步：
1. 更新 `docs/` 对应模块/阶段文档；
2. 回填 `docs/review/11-project-control.md`（能力矩阵、健康状态、迁移地图）。
3. 涉及遗留问题（见总览 §6）的修复，更新对应状态。

## 已知遗留问题（修改时优先关注）

> 权威清单：`docs/review/11-project-control.md` §6（健康状态）+ §8（Phase 3 前置/随行分类）+ phase2/00 §9（代码级 backlog）。2026-08-31 校准：

- **HC1**：Comment/Note 不写 `ticket_events`（审计缺口，§6 唯一未修项）
- **TC1**：Go 层缺「全局 admin 删单成功」集成测试（脚本层已覆盖：2c 脚本 SAT 建删删；待补见 11 §8 A7）
- ~~TC2 / HC2~~：已修复（`TestD9_CreateRelation`；000014 SET NULL + Delete 同事务写 deleted 事件）
- **BK-13 / BK-14**（已触发、待实施）：project_isolated 默认收紧开关 + 成员 scope 配置面（多虚拟组可见性场景闭环，见 00 §9）
- Phase 3 启动前待决策：迁移编号 000017 冲突（附件 vs SLA）；完整 A/B 档清单见 11 §8（SLA 暂停态等设计歧义归 B1 随 Step 7 拍板）

> 最新验证结果以 `docs/review/` 为准；修复后回填状态。
