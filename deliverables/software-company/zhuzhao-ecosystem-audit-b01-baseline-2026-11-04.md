# 生态全面审计 B01：基线、依赖与准备核验（2026-11-04）

## 范围与基线

批号 `ECO-20261104-B01`。首次全面审计的准备批，覆盖五仓版本、依赖发布/消费差集、运行产物来源、文档阅读覆盖登记及隔离验证准备。本报告不代表设计或实现全量审查完成。

固定提交如下（本批直接查询远端对应分支，均与本地相同；无新增差集）：

- zhuzhao / phase5：`5c03581d5999139a3e8e5ead5972c026868aa181`。
- taskrunner / main：`1dcc83185f24b8dc2bed7bba6786272a5fdfb19f`。
- activelist / main：`af0b350da650517df2c9d15cca4892cdcd51be88`。
- zhuzhao-ui / phase5：`b20594cc744a61e2bcb41535b0ea668a711aa4df`。
- zhuzhao-utils / main：`5a3900fa6013750e912eca958666831f0a6f2c9d`。

主仓仅本审计会话文档/证据在途，四个兄弟仓干净。UI 相对初盘 edaeeba 新增 b20594cc 一笔，已纳入当前基线；不再排除此前两文件。原始查询命令、完整 hash 与时点状态见 [五仓基线回执](evidence/ECO-20261104-b01/B01-five-repo-baseline.json)。

SSOT 入口：[VISION](../../docs/VISION.md)、[standards](../../docs/standards.md)、[项目控制台](../../docs/review/11-project-control.md)、[runbook](../../docs/runbook.md)及各阶段有效设计。`doc/` 未作为审计依据。

## 分批结论与覆盖

本批没有新增产品缺陷编号；认证初核的发现独立归入 [B02](zhuzhao-ecosystem-audit-b02-auth-revocation-2026-11-04.md)，避免重复统计。

### 依赖版本已核实

三个 Go 服务实际解析的 utils 均为 `v0.4.2`，`GOWORK` 为空，模块解析结果无 Replace；查询使用 `GOPROXY=off`，未改 go.mod/go.sum。tag 解引用为 `21155a98e87a944b20600833f940698d2759c2e5`，缓存模块 27 个 Go 文件与该 tag 快照逐件相同。

utils HEAD 不能替代消费版本。tag→HEAD 的四笔为：

- `cdb731799aea9705c5d931e57d9b7294c6e12e98`。
- `1c27ca2f909724f3f2b6d64de73a286dc61a68c4`。
- `78511a71ec180e7838e79f6e342b3d00159178c9`。
- `5a3900fa6013750e912eca958666831f0a6f2c9d`。

标题、实际文件集与缓存对照结果见 [发布差集](evidence/ECO-20261104-b01/B01-utils-release-diff.json)；消费者实际解析见 [模块回执](evidence/ECO-20261104-b01/B01-module-resolution.json)。本批未将这四笔视为已被消费者使用，也未完成公共库逐包审计。

### 运行环境存在版本限制

通过读取运行二进制的 `go version -m` 核实：宿主 dev 主应用为受审 `5c03581d...` 且 `vcs.modified=false`；当时 `zt-app` 为旧 `f3f2077e88e16f1e80b555d2e24d322ea4306bb1` 且 `vcs.modified=false`。宿主和测试栈 taskrunner/activelist 均能确认消费 utils v0.4.2，但未携带 VCS revision，不能确认完整源码提交；UI 产物来源未确认。

因此未使用这些未归因服务证明固定提交正确。本批记录的是验证限制，不直接将“服务未随最新提交重建”计为产品缺陷。镜像 ID、挂载及 buildinfo 见 [运行来源清单](evidence/ECO-20261104-b01/runtime-provenance-summary.json)与 [dev 来源清单](evidence/ECO-20261104-b01/dev-provenance-summary.json)。后续获授权重建的记录另列 B03，不回写本批历史观测。

### 文档阅读如实登记

建立五仓 `docs/**/*.md` 加根 README/AGENTS 共 145 份文件索引；该数字不含所有 deliverables 或其他位置的材料，不能称为全仓文档总数。主仓本批全文阅读 17 份，含九个模块设计、VISION/roadmap、Phase 7–9 总纲；其他文件分别标为部分章节或未开展本轮逐篇阅读。

早期模块文档包含完整形态草图与历史方案，仍须对照阶段文件、有效决策和代码裁定。Phase 6–9 用于演进前置核验，未实施规划不能自动判当前缺陷。逐篇状态、行数与文本 hash 见 [覆盖清单](evidence/ECO-20261104-b01/B01-doc-coverage.json)。

## 验证证据

六份临时快照来自五仓 HEAD 及 utils tag 的 `git archive`，不携带源仓未提交文档。原样执行：

```sh
# zhuzhao 固定提交临时副本
GOPROXY=off go test -race -count=1 -timeout=90s ./internal/...
# utils v0.4.2 临时副本
GOPROXY=off go test -race -count=1 -timeout=90s ./...
```

两条均 exit 0。完整初始 stdout 位于对话工具回执；[本地单测回执](evidence/ECO-20261104-b01/B01-unit-test-receipts.json)是明确标注来源的摘要，不冒充完整原始日志。[快照完整性](evidence/ECO-20261104-b01/snapshot-integrity.json)确认原有文件未变；主仓副本后来仅新增 B02 两个探针文件，受审仓实现未改。

证据文件 SHA-256 清单见 [manifest](evidence/ECO-20261104-b01/manifest.json)。未运行 acceptance、全量集成、浏览器 E2E 或共享环境数据实验。历史验收数字不计为本轮实跑。

## 接续与建议挂点

阶段 0 部分准备项已完成，全链隔离环境、完整角色账号矩阵与逐能力覆盖仍待补。阶段 1 继续 Phase 1–5 有效设计、演进决策、债台账及兄弟仓 SSOT；认证初核不抵扣其他模块覆盖。

本报告与证据为本地未提交审计产物；无实现修复、commit/push 或产品通过结论。后续提交变化须重新登记差集。
