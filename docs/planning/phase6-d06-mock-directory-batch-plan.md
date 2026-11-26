# Phase 6 D06 模拟目录批次实施计划

> 用途：第 10 项的第一个获准实施切片。Core 基线为 `4e430308`，仅使用合成部门数据，在独占 PostgreSQL 上验收。
> 主线合同：[D06 规范化输入、写主和批次](../design/business-lifecycle-contracts.md#7-d06hr-公共规范化输入与组织生命周期)。按测试先行执行，实施与独立复核分工。

**目标**是通过本地命令处理模拟部门批次，持久化每次运行的结果，并可在重启后读取原回执。

**设计**采用 `go build ./cmd/directory-sync-mock` 生成的本地命令。命令显式使用 `D06_MOCK_DATABASE_URL` 指定的数据库，从标准输入读取一份限量 JSON，输出一份回执。专用投影表只存合成目录事实，业务 IAM 与组织表独立。一个 provider 固定为 `mock-directory`，domain 固定为 `departments`，mode 固定为 `full`。仅处理平面部门名称及其来源修订。不同 scope 独立，同 scope 的事务通过水位行串行化。事务内用每页保存点区分可持久化的部分结果和基础设施错误。

## 输入和回执

输入包含 `provider_ref`、`domain`、`mode`、`scope_ref`、`batch_id`、`expected_baseline`、`success_cursor` 和 `pages`。第一页 `cursor` 为空，后续页与上页 `next_cursor` 一致。最后一页显式 `done=true` 才有资格推进成功水位。回执的 `page_watermark` 为最后一个成功页的下一页游标，完整结束时为空。失败页给出 `error=fetch_failed`，不包含记录或下一页。每条部门记录使用 `external_id`、`display_name` 和正整数 `source_revision`。输入上限 1 MiB，每批最多 32 页，每页最多 100 条。

回执包含 `run_id`、`scope_ref`、`batch_id`、`status`、`applied_pages`、`page_watermark`、`baseline_before`、`baseline_after`、`cursor_after`、`records` 和适用的 `problem`。成功基线使用该次成功的 `batch_id`。回执逐条记录已提交页的部门外部 ID、来源修订和结果。读取同批次回执先比较规范化摘要，再判断与当前基线的关系，保证历史成功和部分完成均可原样重放。失败后修正输入用新批号。

## 任务与验收

- [x] **任务一，专项验收用例。** 在 `internal/service/directorymock/acceptance_integration_test.go` 通过真实命令与 PostgreSQL 先写失败用例。完整批次跨两页，核对部门名、批次和水位。后一页中断时保留已提交页，缺失部门无状态变化，水位保持。使用新的命令进程重放，比较原运行 ID、回执及持久行。同编号变更内容时拒绝且数据库零变化。
- [x] **任务二，批次处理。** 在 `internal/service/directorymock/` 实现固定合同、顺序和去重。在 `migrations/000097_directory_mock_batches.{up,down}.sql` 增加专用水位、目录投影和回执表。在 `cmd/directory-sync-mock/main.go` 接入受限命令、结构化回执和退出码。补验页缺失与错误终点、同批并发、修订倒退或同修订异值及事务回滚。
- [x] **任务三，文档与独立复核。** 在 D06 合同登记运行方式、输入与回执、事务和回退语义，回填项目控制台能力和迁移地图。不同助手复核范围与持久化并另验运行证据。
- [ ] **任务四，隔离交付门禁。** 将专项红绿、真实重放、迁移往返和普通档门禁绑定同一候选。按项目协议记录结果。本地材料 ID 为 `evidence/phase6-d06-mock-directory-batch-20261126.cJ156kn9/`，本项的实际门禁与候选身份以该收据为准。

## 复核重点

检查同号异内容在任何新写入前被拒，历史重放不受现行基线或先前失败页影响。检查错误页不会留下半页更新，且未知的数据库失败不被记成普通 partial。检查同 scope 并发运行遵守唯一回执与当前基线，合成事实与业务组织授权无交叉写。检查日志和命令输出只包含可安全报告的错误分类及合成回执。
