# 01 - 可观测性（observability）

> Phase 3 Step 1。**应用内可选开关 + 外部栈部署可选**，与 [design-decisions §18](../design/design-decisions.md#18-部署与代码解耦一套代码多种部署) 一致。

---

## 1. 设计原则

| 原则 | 说明 |
|------|------|
| **App 不硬依赖观测栈** | 无 Prometheus/Grafana/Collector 时正常启动、正常服务 |
| **配置驱动** | `observability.*.enabled` 控制是否注册 endpoint / exporter |
| **关闭即零开销** | `enabled: false` 时用 noop tracer，不挂 `/metrics` |
| **Grafana 永远可选** | 仅可视化；缺 Grafana 不影响 App 与 Prometheus |
| **分档验收** | Phase 3-min 不要求 Metrics/追踪；Phase 3-full 或多实例建议开启 |

验收档位见 [phase3/README §3.1](./README.md#31-phase-3-验收两档)。

---

## 2. 配置约定（SSOT）

```yaml
# configs/config.yaml（Phase 3 扩展）
observability:
  metrics:
    enabled: false
    path: /metrics
  tracing:
    enabled: false
    exporter: noop          # noop | stdout | otlp
    otlp_endpoint: ""         # 有 Collector 时再填，如 localhost:4317
    sample_ratio: 1.0         # 生产可调低
  pprof:
    enabled: false
    addr: 127.0.0.1:6060      # 独立 admin 端口，不挂公网 HTTP 端口
```

| 开关 | `enabled: false` | `enabled: true` |
|------|------------------|-----------------|
| metrics | 不注册 `/metrics` | promhttp + Gin 中间件（QPS/延迟/错误率） |
| tracing | `noop.TracerProvider` | stdout 或 OTLP 导出 |
| pprof | 不监听 | 仅 `127.0.0.1` 或内网；release 需 Nginx 白名单 |

**dev 快捷方式**：`server.mode: debug` 时可默认 `pprof.enabled: true`（仍绑定 localhost），与 [01-infra](../phase1/01-infra.md) 一致。

---

## 3. 组件关系

```
App（config 开关）
  ├─ /metrics ──scrape──▶ Prometheus（部署可选）
  │                           └──▶ Grafana（部署可选）
  ├─ OTLP ──▶ OTel Collector（部署可选）──▶ Jaeger/Tempo（可选）
  └─ pprof ──▶ go tool pprof（按需，内网）
```

| 组件 | 角色 | 是否必须部署 |
|------|------|--------------|
| slog + `request_id` | 日志串联 | Phase 1 起已有 |
| `/health/live` + `/health/ready` | 探针 | Phase 1 必须 |
| Prometheus | 指标采集 | **可选** |
| Grafana | 大盘 | **可选** |
| OTel Collector | 追踪转发 | **可选**（dev 可用 stdout） |

---

## 4. 实现要点（Phase 3）

### 4.1 Wire 集成

- `internal/pkg/observability/`（或 `internal/middleware/metrics.go`）：根据 config 条件注册
- `enabled: false` 时 Provider 返回 noop / 空 middleware，Router 不挂载
- 不把 Prometheus client 初始化写死在 `main` 无条件路径

### 4.2 Metrics（开启时）

- 库：`prometheus/client_golang`、`gin-contrib/prometheus`（或自写轻量 counter/histogram）
- 建议指标：HTTP 请求数/延迟/状态码、Go runtime、pgxpool 状态（Phase 3 后期）
- `/metrics` 不对公网暴露；Nginx deny 或仅内网 scrape

### 4.3 Tracing（开启时）

- 库：`go.opentelemetry.io/otel`
- Gin / pgx / redis 中间件注入 trace context
- `exporter: otlp` 时 Collector 不可达应 **降级日志 Warn**，不阻塞启动（可配置 strict 模式用于 CI）

### 4.4 pprof

- Phase 1–2 开发：本地 `go test -bench` / debug 端口即可
- Phase 3 生产：仅 Phase 3-full 或排障时开；必须网络隔离

---

## 5. Docker Compose profile（示意）

```yaml
# docker-compose.yml
services:
  app:
    # 默认 profile，无 observability 依赖

  prometheus:
    profiles: ["observability"]
    # ...

  grafana:
    profiles: ["observability"]
    # ...

  otel-collector:
    profiles: ["observability"]
    # ...
```

```bash
docker compose up -d              # 仅 App + PG + Redis
docker compose --profile observability up -d   # 附带观测栈
```

---

## 6. 测试用例

| 用例 | 验证点 |
|------|--------|
| 全部 `enabled: false` | App 启动成功；无 `/metrics`；noop tracer |
| `metrics.enabled: true` | `GET /metrics` 200；Prometheus 未部署时 App 仍正常 |
| `tracing.exporter=stdout` | 有 span 输出；无 Collector |
| `tracing.exporter=otlp` 且 Collector  down | 默认 Warn 降级，不 crash（或 strict 模式失败，文档注明） |
| Phase 3-min 部署 | 无 Prometheus/Grafana 容器，验收通过 |

---

## 7. 涉及文件（规划）

```
configs/config.yaml                 # observability 段
internal/config/config.go           # ObservabilityConfig
internal/pkg/observability/         # metrics / tracing / pprof 装配
internal/router/router.go           # 条件挂载 /metrics
deploy/docker-compose.yml           # observability profile
deploy/prometheus.yml               # scrape 配置（可选）
```

---

## 8. 待决策点

- ✅ **栈是否必须**：否；应用可选 + 部署可选
- ✅ **Grafana**：永远可选
- 📋 **OTLP 不可达**：默认 Warn 降级 vs 启动失败（建议默认 Warn）
- 📋 **Sentry**：有 DSN 才启用，Phase 3 可选
