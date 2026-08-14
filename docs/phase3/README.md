# Phase 3 实现计划：生产加固

> **核心目标**：可观测性、多实例部署、高可用，使系统具备上线生产环境的条件。
>
> 创建日期：2026-08-12  
> 修订：2026-08-13 — 拆为 **Phase 3a**（先上生产）与 **Phase 3b**（按需拆服务/平台），避免一次做太多。

---

## 0. 子阶段总览

| 子阶段 | 目标 | 典型交付 |
|--------|------|----------|
| **3a** | 单/多实例可运维、可观测、可恢复 | Metrics、Watcher、审计 L2、HA、安全增强、ops CI |
| **3b** | 按需演进为分布式/拆服务 | Outbox+Asynq、IAM 拆分、gRPC、RS256、缓存平台、AK/SK |

**建议顺序**：先完成 **3a** 上线生产，有真实多服务或 M2M 需求再启动 **3b**。

---

## 1. Phase 3 边界

### 1.1 Phase 3a — 做什么

| 类别 | 模块 | 核心能力 | 文档 |
|------|------|---------|------|
| 可观测性 | [observability](./01-observability.md) | 应用内 **可选开关**（Metrics / OTel / pprof）；Prometheus / Grafana / Collector **部署可选** | 已编写 |
| 多实例部署 | [multi-instance](./02-multi-instance.md) | Casbin Watcher、跨实例事件、分布式锁 | 待编写 |
| 审计日志升级 | [audit-l2](./03-audit-l2.md) | Redis List L2，进程崩溃不丢日志 | 待编写 |
| 高可用 | [ha](./06-ha.md) | PG Cluster、Redis Sentinel、Nginx | 待编写 |
| 安全增强 | [security-enhance](./07-security-enhance.md) | 异地登录、验证码、密码过期、API 限流 | 待编写 |
| 运维工具 | [ops](./08-ops.md) | Swagger CI、迁移 CI、集成测试自动化 | 待编写 |

### 1.2 Phase 3b — 做什么

| 类别 | 模块 | 核心能力 | 文档 |
|------|------|---------|------|
| 事件驱动 | [event-driven](./04-event-driven.md) | PostgreSQL Outbox + Asynq | 待编写 |
| 微服务拆分 | [microservice](./05-microservice.md) | IAM 独立、Gateway、gRPC、RS256+JWKS | 待编写 |
| 平台增强 | [platform](./09-platform.md) | 权限/菜单缓存、AK/SK（有调用方时） | 待编写 |

### 1.3 不做什么

| 不做 | 原因 | 阶段 |
|------|------|------|
| 多租户 | 预留字段即可，按需启用 | 按需 |
| 第三方登录（OAuth/SSO） | 预留字段，按需启用 | 按需 |
| Kafka/RabbitMQ | Redis List + Asynq 足够 | 按需 |
| K8s 部署 | Phase 3 先用 Docker Compose + Nginx，K8s 后续 | 按需 |

### 1.4 前置条件

**Phase 3a** 开始前，**Phase 2b** 必须已完成（**2c 不阻塞 3a**——组织委托可与生产加固并行，但完整 Phase 2 产品能力仍以 2a→2b→2c 为准，见 [phase2/README §0](../phase2/README.md#0-子阶段总览)）：

- [ ] 2a 验收：TicketResource + **assigned** 范围
- [ ] 2b 验收：虚拟组 / scope / HR Sync、工单附件、auth-enhance
- [ ] 2c 验收：**不**作为 3a 硬前置；建议在对外上线前完成（D1–D11）

### 1.5 可观测性：应用可选、部署可选

> 与 [design-decisions §18](../design/design-decisions.md#18-部署与代码解耦一套代码多种部署) 一致：**App 不因未安装 Prometheus/Grafana/OTel Collector 而无法启动**。

| 层级 | 是否必须 | 说明 |
|------|----------|------|
| Phase 1–2 | 不要求 | `/health/*` + slog + `request_id` 即可 |
| 应用内埋点 | 有能力、**配置开关** | `observability.metrics/tracing/pprof.enabled` |
| Prometheus | **部署可选** | 采集端；无它 App 正常运行 |
| Grafana | **部署可选** | 纯展示，永远不是 App 依赖 |
| OTel Collector | **部署可选** | dev 可用 `noop` / `stdout` |

Docker Compose 建议用 **profile**（如 `observability`）拉起 Prometheus/Grafana/Collector；默认 `docker compose up` 可不包含。

详见 [01-observability.md](./01-observability.md)。

---

## 2. 实施顺序

### 2.1 Phase 3a（先上生产）

```
Phase 2b 验收通过（2c 可并行）
   │
   ├── Step 1: observability
   ├── Step 2: multi-instance → Step 3: audit-l2
   ├── Step 4: ha
   ├── Step 5: security-enhance（可与 3a 并行）
   └── Step 6: ops → Step 7: 3a 生产验收
```

### 2.2 Phase 3b（按需）

```
3a 稳定运行 + 有拆服务/M2M 需求
   │
   ├── Step 8: event-driven
   ├── Step 9: microservice（含 RS256+JWKS）
   ├── Step 10: platform（缓存、AK/SK）
   └── Step 11: 3b 验收
```

### 2.3 步骤对照表

| Step | 子阶段 | 模块 | 依赖 | 文档 |
|------|--------|------|------|------|
| 1 | 3a | observability | Phase 2 | [01-observability.md](./01-observability.md) |
| 2 | 3a | multi-instance | Step 1 | [02-multi-instance.md](./02-multi-instance.md) |
| 3 | 3a | audit-l2 | Step 2 | [03-audit-l2.md](./03-audit-l2.md) |
| 4 | 3a | ha | Step 2 | [06-ha.md](./06-ha.md) |
| 5 | 3a | security-enhance | Phase 2 | [07-security-enhance.md](./07-security-enhance.md) |
| 6 | 3a | ops | Phase 2 | [08-ops.md](./08-ops.md) |
| 7 | 3a | 生产验收 | Step 1–6 | 本文档 §3.1 |
| 8 | 3b | event-driven | 3a 稳定 | [04-event-driven.md](./04-event-driven.md) |
| 9 | 3b | microservice | Step 8 | [05-microservice.md](./05-microservice.md) |
| 10 | 3b | platform | 按需 | [09-platform.md](./09-platform.md) |
| 11 | 3b | 验收 | Step 8–10 | 本文档 §3.2 |

---

## 3. 生产验收标准

### 3.1 Phase 3a 验收（两档）

按部署场景选档；**不得**把 Grafana/Prometheus 未部署视为 App 启动失败。

#### 3a-min（单实例、内网、低 SLA）

| 维度 | 指标 |
|------|------|
| 可用性 | 单实例可恢复；live/ready 正常 |
| 可观测性 | 结构化 slog + `request_id`；`observability.*.enabled=false` 时零额外开销 |
| 安全性 | Phase 1 限流/锁定 + HTTPS（有域名时） |
| 数据安全 | PG 定期备份 |
| 运维 | 一键部署、DB 迁移可脚本化 |

#### 3a-full（多实例或需 SLO / 对外 SLA）

在 **3a-min** 基础上：

| 维度 | 指标 |
|------|------|
| 可用性 | 多实例 99.9% |
| 可观测性 | 开启 Metrics（QPS/延迟/错误率）+ 分布式追踪；Grafana 大盘 **可选** |
| 多实例 | Casbin Watcher、缓存跨实例失效 |
| 审计 | Redis List L2（进程崩溃不丢） |
| 安全性 | 密码策略、异地登录、API 限流等（见 security-enhance） |
| 运维 | Swagger CI、集成测试自动化 |

### 3.2 Phase 3b 验收（按需）

| 维度 | 指标 |
|------|------|
| 拆服务 | IAM 与工单可独立部署，gRPC 内部通信 |
| Token | RS256 + JWKS，业务服务仅持公钥验签 |
| 平台 | 权限缓存跨实例失效；有 M2M 时 AK/SK 可用 |

---

## 4. 待决策点

| 事项 | 说明 | 状态 |
|------|------|------|
| ⚠️ K8s vs Docker Compose | Phase 3 是否上 K8s？ | 建议 Docker Compose + Nginx 足够，K8s 按需 |
| ⚠️ 微服务拆分粒度 | 先拆哪个服务？ | 建议先拆工单服务（业务独立性强） |
| ⚠️ gRPC vs HTTP | 内部通信协议 | 架构文档已决策：gRPC 内部 + REST 外部 |
| ⚠️ Redis 高可用方案 | Sentinel vs Cluster | 建议 Sentinel（简单），Cluster 按需 |
| ⚠️ PG 高可用方案 | 自建 vs 云托管 | 已决策：云托管 Cluster（2+VIP） |
| ⚠️ KMS 密钥管理 | RS256 私钥是否上 KMS | Phase 3b 评估云 KMS |
| ✅ 可观测性栈 | Prometheus/Grafana/OTel | **应用内可选开关 + 部署可选**；3a-min 不要求全套栈 |

---

## 5. 文档索引

> 标注"待编写"的文档尚需创建，当前先占位。

| 文档 | 模块 | 状态 |
|------|------|------|
| [01-observability.md](./01-observability.md) | 可观测性 | 已编写 |
| [02-multi-instance.md](./02-multi-instance.md) | 多实例部署 | 待编写 |
| [03-audit-l2.md](./03-audit-l2.md) | 审计日志 L2 | 待编写 |
| [04-event-driven.md](./04-event-driven.md) | 事件驱动 | 待编写 |
| [05-microservice.md](./05-microservice.md) | 微服务拆分 | 待编写 |
| [06-ha.md](./06-ha.md) | 高可用 | 待编写 |
| [07-security-enhance.md](./07-security-enhance.md) | 安全增强 | 待编写 |
| [08-ops.md](./08-ops.md) | 运维工具 | 待编写 |
| [09-platform.md](./09-platform.md) | 平台增强（3b） | 待编写 |
