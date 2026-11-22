# Phase 6 B2 第 4–5 项隔离验证

本轮从 D01 检查点 `fe29dc6` 接续，在 `zz-g04-it1` 用合成账号、工单、附件和数据库演练第 4–5 项。实际试用按 Q12 另行批准。第 1–3 项的原始结论见[前一轮验收](phase6-b2-stage3-acceptance-2026-11-22.md)。

## 候选检查

- 原检查点的既有类型建单正例返回 503。对照 v83 合成数据库应用迁移后，再检查发布指针、历史工单原记录与既有类型建单路由。定向用例为 `TestC04_CreateHTTPPreparationFence/allow`，调用 `internal/service/d01_create_integration_test.go` 的 `checkD01CreateAfterUpgrade`。
- `TestD01InstanceUsesBoundRules` 对照 A/B 发布、当前办理与旧规则。 `TestD01InstanceDegradedReadAndZeroWrites` 核对规则不明确的受权读与停写。补绑 CLI 的集成测试按可信映射逐票执行并保留原记录。
- `TestD02PinnedEvidenceAndTransitionCommitTogether` 检查缺材料、错附件和写历史失败时的零业务写入，随后以同一份证据提交。 `TestD02_HTTPVersionedTransitionRechecksCurrentAssignee` 经过真实 JWT、Casbin 和 HTTP handler，确认转派后的旧办理人被拒并无回执，以及新办理人提交、后续编辑后按原键重放。
- 浏览器正例按同类型发布 A/B 并分别建单。A 只收集整改说明和附件，B 另要求复测环境。独立回读字段、附件下载、状态、修订和状态历史。首轮 A 提交成功，B 缺复测环境返回 400，发现补填后表单未重新校验。回归先复现响应式字段缺陷，再修复。
- 独立复核由质量与交付审计者（DeepSeek）执行。另一位验证与验收者（Codex）编写浏览器夹具，按当前发布接口取精确规则。发布前证据字段检查、旧规则解析不一致和历史附件双关联为本轮重点复核项。

## 运行证据

候选 core 制品为 `sha256:2ac9009051e5ad5131a0829410ddc45adc516cbf0244683ffa324c05c9a44a33`，UI 制品为 `sha256:db2919e4bac8b885c363fd1420c19e646e6ecdfdff9fb833b169eacc773ce5a7`。运行前核对同一测试 Compose 的 app、UI、PostgreSQL、Redis 和浏览器回读连接。

- 恢复 v83 合成备份后重新执行 `make migrate-up`。迁移到 v88 且无未完成迁移，15 个启用类型均有发布指针，65 张历史工单仍为空引用，定义摘要重算差异为 0。旧合成数据库和旧 Redis 卷保留。新卷的会话验证先拒绝，再由 worker 重投合成账号已有的当前代际意图。保留原业务操作回执，未修改账号状态或口令。
- 第 4–5 项浏览器联合用例与大 ID 回归共 2 项通过。A 版工单一次提交后可下载原附件，B 版缺复测环境时，无字段、附件或历史变更，补齐后再一次写入。旧单补绑与不可信旧单读写边界另由 PostgreSQL 集成测试取证。
- 主仓 lint、单测、覆盖率、带竞态检测的完整集成测试及 Swagger 生成已通过。默认单测语句覆盖率为 22.6%。四档 `make acceptance` 共 251 项通过，失败和跳过均为 0，88 对迁移往返零漂移。前端 lint、类型检查、662 项单测、生产依赖审计、构建和 55 项完整浏览器用例通过，浏览器无失败、跳过或重试。
- 变更后的独立复核确认前一轮的发布检查、旧规则共用解码、迁移收口及证据转换关键守卫已经落地。运行结论以上述环境取证为准。广义按单按钮能力、字段级历史差异和持久跨域后继由其他工作包交付。

本地材料 ID 为 `g04-it1-20261122/stage4-5`。第 6–9 项由[开工计划](../../planning/phase6-b2-kickoff-plan.md#风险登记到全链验收)承接。生产制品部署与真实试用需另行确定窗口。
