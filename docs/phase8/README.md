# Phase 8 — 自动化能力中心 = SOAR 能力迁移（总纲）

> **文档定位**：phase8 规划总纲（2026-10-29 建档）——**已拍决策汇编 + 范围索引**，蒸馏自愿景稿 [07-phased-plan §4.3](../../deliverables/software-company/secops-platform-vision-2026-10/07-phased-plan.md) 与 D 系拍板。细化设计稿随立项批拆建。
> **状态 = 规划期（未立项未排期）**——立项前置 = phase7 收口 + X3/X6/X7 输入。
> **量级口径**：6–9 周（07 号 §3；原 8–11 周去掉 DAG MVP 加 SOAR 集成）。

## 1. 定位与一句话

公司已有 SOAR（D3）→ 本阶段平台是**能力提供方 + 人机界面**：把 SOAR 的能力逐个迁为平台 Action（connector/script），流程调度**继续用 SOAR**；**不建编排引擎**（Stage 2 引擎 = phase9 既定排期，D1）。

## 2. 已拍决策输入

| D 系 | 内容 | 对本阶段 |
|---|---|---|
| D1 / D3 | Stage 1 编排留 SOAR（过渡态）；能力先迁、编排终态迁回（phase9/10 既定） | 本阶段形态收缩的根依据 |
| D24 | SOAR 双向集成归 phase8（phase7 仅注册表就绪） | 核心切片④ |
| D28 | sandbox-runner 出站三件 + 扩两件（设计节随 phase8 前置批） | 核心切片① |
| D2 | 脚本上传第一档 = 仅管理员 + 审批工单（复用自家工单流程） | 核心切片② |
| C-21+ / 04 号附录 B | 任务结果三通道定形：result 并入件①、日志/进度触发挂账、**phase8 拉取契约随 phase8** | 核心切片⑦ |
| D30 | 处置编排延伸：任务执行记录自动回流 R5 事件时间线 | 核心切片⑧ |

## 3. 核心切片（已确认模块）

1. **脚本契约 + sandbox-runner**：脚本契约已定稿（愿景稿 05 号，拍板 S1–S8——manifest / JSON Schema I/O / 日志工件分离 / secret 注入 / 退出码语义）；sandbox-runner 第一档隔离（03-A2；D28 出站白名单/回调超时/出站代理 + dispatch·report 协议/长任务租约）。
2. **脚本库管理面**：上传 / 审批工单 / 版本 / 预构建 / 运行历史（审批链 ticket_type = X7 立项拍）。
3. **预定义任务目录**：SOAR 现有 apps/actions 按使用频率逐个迁为 `actions(kind=connector|script)`，各带 JSON Schema I/O；迁移期两边并存能力以谁为准逐个拍（04-§4.4）。
4. **SOAR 双向集成**：入 = SOAR 以 service account 调 `/v1/action-invocations` 执行平台 Action、结果经出站 webhook 或轮询取回；出 = `soar.playbook.run` 连接器动作触发 SOAR playbook；SOAR 产出经统一接入层回流为事件。
5. **工单动作面板（F6）+ 自助任务门（F18）**：处置人不离工单页跑富化/处置，表单由 schema 生成。
6. **声明式 HTTP 连接器通用件**（04-§4.9）+ **凭据管理面（F13）**。
7. **job_runs.result 摘要列 + 拉取契约**（C-21+/附录 B——任务结果留存的历史挂账在此收口）。
8. **任务执行记录自动回流 R5 事件时间线**（D30）。

## 4. 补全切片（可滑）

更多 SOAR 能力迁移 · 凭据轮换 · 我们触发 SOAR playbook（出向全量）。

## 5. 边界与纪律

- **脚本内调用其它 Action 默认不开**（防隐式编排）——组合由编排层（Stage 1 = SOAR）负责；SDK `zh.actions.invoke()` 受 `platformAccess` 白名单约束。
- **自研 DAG 引擎 / playbook 设计器不做**——Stage 2 既定排期（phase9 引擎 MVP → phase10 迁移退役，D1），非触发驱动。

## 6. 外部输入（阻塞精估）

| # | 事项 | 用途 |
|---|---|---|
| X3 | SOAR playbook 清单（数量/频率/复杂度） | 能力迁移排序 + D23 phase10 检查点基线 |
| X6 | 内部 PyPI 镜像可用性 | 依赖策略 |
| X7 | 脚本审批用哪个 ticket_type、审批人 | 脚本库审批链 |

## 7. 出口标准（DoD，07 号 §3/§4.3）

管理员可上传脚本经审批运行；SOAR playbook 能调用至少一个平台 Action 并取回结果；处置人在工单页能跑富化/处置并回贴。

## 文档衔接

愿景稿详情 = 07-phased-plan §4.3；脚本契约 = 05 号（已拍 S1–S8）；roadmap §Phase 7–11；下游 = phase9 引擎 MVP（Stage 2 起点）。

## 变更记录

| 日期 | 记录 |
|---|---|
| 2026-10-29 | 建档（总纲蒸馏——已拍决策汇编，未立项） |
| 2026-11-01 | 指针修正：「文档衔接」行 roadmap 锚点 `Phase 8 段` → `§Phase 7–11`（原小节不存在，外审核出；实际小节 = roadmap §Phase 7–11 合并稿） |
