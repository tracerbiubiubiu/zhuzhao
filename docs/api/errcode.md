# 错误码约定（已归并——本文件为指针）

> **权威登记 = [`api/errcode.md`](../../api/errcode.md)**（根文件——段随启用批登记；定义权威 = `internal/pkg/errcode/errcode.go`，HTTP 映射权威 = `internal/handler/errors.go`：表驱动 + 全码清单测试双锁）。
> 本文件原 303 行（Phase 1 验收对照版）已删除，历史内容见 git（`git log --follow -- docs/api/errcode.md`）。**2026-10-28 L-6 归一执行**（F-A-1 既定方向收口：phase5 起「新码只入根文件、本副本停更」的约定自此无第二份可漂移；登记 = [债台账 L-6](../phase5/S6-S5-debt-ledger.md)）。
> 响应体外层结构（`code`/`message`/`data`/`request_id`）SSOT = [`response.md`](./response.md)。
