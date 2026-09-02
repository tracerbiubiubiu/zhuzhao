# 部署指南（deployment.md）

> **定位**：zhuzhao 单体服务的部署指南——环境要求、配置项、部署步骤、备份恢复、运维清单。对应 [phase3/08-ops](../phase3/08-ops.md)（Step 6）与 [phase3/11-deployment-split §2](../phase3/11-deployment-split.md)（档位 1：单体多副本为默认）。
> **状态**：**已编写（2026-09-02，B10 补齐）**。
> **标记约定**：`🚦` = 触发条件驱动；`⚠️` = 待拍板。

---

## 1. 环境要求

| 组件 | 版本 | 说明 |
|---|---|---|
| Go | ≥ 1.22 | 构建 |
| PostgreSQL | 15 | 业务数据 + L1 事件源 |
| Redis | 7 | 会话/黑名单/限流/Asynq 任务后端（辅助角色） |
| Docker + Compose | 最新 | 推荐部署方式 |

## 2. 部署形态（默认：档位 1 单体多副本）

```
┌──────────────┐   ┌──────────────┐
│  Nginx（可选） │──▶│  App 副本 ×N  │
└──────────────┘   └──────┬───────┘
                 ┌────────┴────────┐
                 ▼                 ▼
            PostgreSQL 15       Redis 7
```

- 默认单副本（内网/开发）；对外/SLO 时多副本 + Nginx（[06-ha](../phase3/06-ha.md)）。
- 观测栈（Metrics/Tracing）为可选 profile（[01-observability](../phase3/01-observability.md)）。

## 3. 配置项（configs/config.yaml + 环境变量）

| 段 | 关键项 | 环境变量示例 |
|---|---|---|
| server | port / mode / shutdown_timeout | `SERVER_PORT` / `SERVER_MODE` |
| postgres | dsn / pool_max_conns | `DATABASE_DSN` |
| redis | addr / password（compose 注入） | `REDIS_ADDR` / `REDIS_PASSWORD` |
| jwt | secret（HS256）/ 过期时长 | `JWT_SECRET`（必设，勿用默认） |
| observability | metrics/tracing/pprof enabled（W1 后） | `OBS_METRICS_ENABLED` 等 |
| audit | pipeline l1/l2（W1 后） | `AUDIT_PIPELINE` |

> 敏感项（JWT_SECRET / REDIS_PASSWORD / DATABASE_DSN）**必须**环境变量/密钥注入，不落代码仓库明文。

## 4. 部署步骤

```bash
# 1. 构建
make build

# 2. 拉起依赖（PG + Redis；观测栈按需）
docker compose up -d
docker compose --profile observability up -d   # 可选

# 3. 应用迁移（幂等，可脚本化）
make migrate-up

# 4. 启动应用（本地直跑）
./bin/zhuzhao server   # 或 docker compose 内 app 服务

# 5. 健康检查
curl -fsS localhost:8080/health/live
curl -fsS localhost:8080/health/ready
```

- **多副本（W1 后）**：`docker compose up -d --scale app=2`（或 deploy.replicas）+ Nginx upstream；探针供 LB 摘除。
- **配置即代码（W2 后）**：workflow/SLA 种子 `go run ./cmd/bootstrap --dry-run`（幂等载入，见 [10-ticket-business §4.9](../phase3/10-ticket-business.md)）。

## 5. 数据库迁移规范

- 迁移编号全局唯一；up/down 成对；启动前核对 `docs/phase2/README §2.4` 占用表（A2 规则）。
- 每次变更：`make migrate-up` + 相关 acceptance 回归。

## 6. 备份与恢复

| 项 | 建议 |
|---|---|
| 全量备份 | `pg_dump` 每日（或 PITR） |
| 恢复演练 | 季度一次，验证可恢复（Phase 3-min 也要求） |
| 审计归档（W2 后） | 判定日志/audit_logs 超期导出 JSONL 后删行（保留 180 天可配，见 [03-audit-l2](../phase3/03-audit-l2.md)） |

## 7. 上线检查单（摘要）

- [ ] 环境变量敏感项已注入（无默认明文）
- [ ] `make lint` + `make test-unit` + `make test-integration` + `make acceptance` 全绿
- [ ] 迁移已应用且可回滚
- [ ] `/health/live` `/health/ready` 正常
- [ ] CORS 白名单已收紧（B7，[07-security-enhance](../phase3/07-security-enhance.md)）；HTTPS（有域名时）
- [ ] ⚠️ 对外暴露前：API 限流启用、审计管道确认（l1/l2）、备份就绪

## 8. 故障排查入口

- 日志：结构化 slog + `request_id` 串联（Phase 1 基线）。
- 探针：live（存活）/ ready（依赖 PG/Redis）不健康时优先查依赖。
- 事件：L1 `ticket_events`（PG）为事实源；Asynq 任务失败可查队列/重试（W2 后）。
- 详细 runbook：`runbook.md`（Phase 3，见 ops/README）。

## 9. 变更记录

| 日期 | 说明 |
|---|---|
| 2026-09-02 | 已编写（B10）：环境要求 / 配置项 / 部署步骤 / 迁移 / 备份恢复 / 上线检查单 |
