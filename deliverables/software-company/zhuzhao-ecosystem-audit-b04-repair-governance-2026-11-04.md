# zhuzhao 全面审计 B04：修复治理与提交守卫

日期：2026-11-04。执行：Codex 当前会话。性质：所有者授权的治理补强，非新增业务模块审计。

## 范围与基线

主仓基线 `5c03581d5999139a3e8e5ead5972c026868aa181`，原验证在未提交候选上采集；现按所有者授权纳管已完成治理项。沿用 B01–B03 的五仓审计窗口，不把修复工作区纳入原始受审快照。B02 的 `ECO-20261104-B02-F01` 仍开放。

## 本批变更与挂点

- [standards §12](../../docs/standards.md)：明确 P0 独立及时修复、P1 按模块/批次收敛、P2/P3 挂账；原有开工前期限仍适用。审计派生 fix 必须 `Refs` 发现号，正文三节有实际内容，同类扫描保留；本笔完整 hash 在提交后写复验记录，不在自己的正文循环引用。
- [审计指南 §7.1/§9.1](../../docs/review/audit-playbook.md)：区分审计/授权修复窗口、正常门禁与环境边界、修复/复验/关闭和发布/部署状态；同 agent 可独立重跑，不强制新增并行会话。附可复用提交模板。
- [commit-lint.sh](../../scripts/commit-lint.sh) 的 fix 分支调用 [check-fix-body.awk](../../scripts/check-fix-body.awk)：检查「改动摘要、影响面、验证证据」存在、非空和重复，兼容有/无 scope、纯文本及 Markdown。新检查先 WARN，原有空正文硬失败不变；引用真实性、内容真实性和顺序仍由人工复核，不宣称机械检查能证明证据可信。
- [CI](../../.github/workflows/ci.yml) 增加 [黑盒反例](../../scripts/test-commit-lint.py)：临时仓运行，覆盖正常 scoped/scopeless、缺节、空节/Refs 尾注、伪标题、重复节、Markdown/ASCII 冒号、非 fix、旧空正文硬失败和同类扫描告警，共 10 例。

本批未给历史提交重写信息，未改兄弟仓实现、业务 API、数据库迁移或鉴权行为，未改变 B02 漏洞结论。

## 验证证据

[验证回执](evidence/ECO-20261104-b04/verification.json) 与 [SHA-256 清单](evidence/ECO-20261104-b04/manifest.json) 记录本轮实跑：

- `python3 scripts/test-commit-lint.py`：10/10 通过，见 [完整日志](evidence/ECO-20261104-b04/commit-lint-blackbox.log)。断言同时检查退出码及 WARN 内容，防止「只退出 0」假绿。
- `sh -n scripts/commit-lint.sh`、`git diff --check`：exit 0。
- `sh scripts/commit-lint.sh HEAD..HEAD`：空窗口 exit 0，仅验证入口可运行，**不代表已扫描不存在的修复提交**。

开发中发现 macOS awk 对中文冒号字符类的匹配偏差，已改为标题前缀逐字匹配，反例集在实际 macOS awk 下通过。另以现有 alpine:3.20 的 BusyBox awk 检查正例/空证据反例通过，该次仅有工具回执，未保存完整日志，不列作本清单内原始证据。

未跑 Go 业务测试、全量 CI 或 `make acceptance`：本批无业务实现改动，治理测试不能替代产品门禁；不将本批标作业务修复完成或漏洞关闭。B01–B03 完成项已独立提交，治理项在本次路径提交纳管；R01 未完成候选继续本地保留，未执行 push。

## 接续

R01 只针对 B02-F01，已登记独立工作树、双仓基线与授权；实现、依赖发布、消费者 pin 和独立复验分别给出实际状态。utils 的未发布 HEAD 不等于消费者 v0.4.2，不能以临时 `go.work` 的绿测宣称正式依赖可构建。修复进展另报，原 B02 报告保持时点事实。
