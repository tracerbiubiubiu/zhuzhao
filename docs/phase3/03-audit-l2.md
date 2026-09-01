# Phase 3 · 审计日志 L2（audit-l2）

> **状态：待编写（W1 前，检查单 B9）。** 本文件当前为**范围占位**，锚定 2026-09-01 登记的范围（检查单 B11 / 11 §8），防止正式编写时漂移。W1 启动时以本文档正式编写为准。

## 范围（2026-09-01 登记）

1. **审计管道 L2**（原定范围）：Redis List 缓冲审计写入，进程崩溃不丢日志。
2. **B11① L2/L3 策略评估日志**（随 W1）：
   - 判定日志表（迁移编号启动时按 A2 规则核对，当前已占用至 000018）；
   - 埋点于 `resource.Authorize` / `scope_resolver.resolve`（字段：actor / 资源 / 动作 / scope 轴 / 结果 / 原因 / trace_id），补 L2 拒绝无留痕盲区（现状：L3 路由拒绝有 slog Warn、审计行带 403/404；L2 scope 拒绝完全静默）；
   - **写入管道随本文档拍板**：同步落库 vs 复用本模块 Redis List 缓冲、失败容忍度（fail-open 吞错不阻断业务 vs fail-close）。
3. **B11② 审计归档**（范围登记于此，实施随 W2 Asynq）：
   - audit_logs + 判定日志表超期导出 JSONL、**导出成功后才删行**；
   - Asynq periodic task（每日低峰）；保留期默认 180 天（等保 ≥6 个月口径）、配置可调；
   - 与 W1 的顺序天然成立（W2 硬前置 W1）：先建表埋点、Asynq 到位后接归档。

## 参考实现（go-wind-admin，2026-09-01 调研）

- **判定日志**：`sys_policy_evaluation_logs`——装饰器引擎包装 `IsAuthorized`，每次判定同步落一行（写失败吞错不阻断鉴权），字段含 result / effect_details（拒绝原因）/ evaluation_context（决策上下文 JSON 快照）/ trace_id / 命中 permission+policy。**注意其只盖 API 级鉴权（数据级 `scope_sql` 是预留空字段）**——zhuzhao 的痛点在资源级 L2，须埋在 `resource.Authorize`，不能照抄中间件装饰器模式。
- **归档**：asynq `audit_log_archive` 每日 03:30、保留 180 天（环境变量可调）、导出本地 JSONL（单批 5000 行、单表失败跳过）、导出成功才按同批 id 删行。**其归档仅落本地文件不上对象存储——zhuzhao 实施时需补上传一环**（对接 §oss 或 PG 备份体系）。
