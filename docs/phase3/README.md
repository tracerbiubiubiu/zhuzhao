# Phase 3 实现计划：生产加固

> **核心目标**：可观测性、多实例部署、高可用，使系统具备上线生产环境的条件。
>
> 创建日期：2026-08-12

---

## 1. Phase 3 边界

### 1.1 做什么

| 类别 | 模块 | 核心能力 | 文档 |
|------|------|---------|------|
| 可观测性 | [observability](./01-observability.md) | Prometheus Metrics、Grafana Dashboard、OpenTelemetry 分布式追踪 | 待编写 |
| 多实例部署 | [multi-instance](./02-multi-instance.md) | Casbin Watcher、跨实例事件广播、分布式锁 | 待编写 |
| 审计日志升级 | [audit-l2](./03-audit-l2.md) | Redis List 队列（L2），进程崩溃不丢日志 | 待编写 |
| 事件驱动 | [event-driven](./04-event-driven.md) | PostgreSQL Outbox + Asynq，可靠异步任务处理 | 待编写 |
| 微服务拆分 | [microservice](./05-microservice.md) | gRPC 内部通信、服务拆分策略、API Gateway | 待编写 |
| 高可用 | [ha](./06-ha.md) | PG Cluster、Redis Sentinel/Cluster、Nginx 负载均衡 | 待编写 |
| 安全增强 | [security-enhance](./07-security-enhance.md) | 异地登录检测、验证码、密码过期策略 | 待编写 |
| 运维工具 | [ops](./08-ops.md) | Swagger CI 生成、DB 迁移 CI 集成、集成测试自动化 | 待编写 |

### 1.2 不做什么

| 不做 | 原因 | 阶段 |
|------|------|------|
| 多租户 | 预留字段即可，按需启用 | 按需 |
| 第三方登录（OAuth/SSO） | 预留字段，按需启用 | 按需 |
| Kafka/RabbitMQ | Redis List + Asynq 足够 | 按需 |
| K8s 部署 | Phase 3 先用 Docker Compose + Nginx，K8s 后续 | 按需 |

### 1.3 前置条件

Phase 3 开始前，Phase 2 必须已完成：

- [ ] 资源级鉴权可用
- [ ] 缓存体系运行正常
- [ ] 安全加固完成（限流、锁定、密码策略）
- [ ] 工单模块可用
- [ ] 所有 Phase 2 测试用例通过

---

## 2. 实施顺序

```
Phase 2 完成
   │
   ├── Step 1: observability（可观测性，独立）
   │      │
   │      ├── Step 2: multi-instance（多实例，依赖 Watcher + 分布式锁）
   │      │      │
   │      │      └── Step 3: audit-l2（审计日志升级，多实例场景下必须）
   │      │
   │      ├── Step 4: event-driven（事件驱动，独立）
   │      │      │
   │      │      └── Step 5: microservice（微服务拆分，依赖事件驱动）
   │      │
   │      ├── Step 6: ha（高可用，依赖多实例）
   │      │
   │      ├── Step 7: security-enhance（安全增强，独立）
   │      │
   │      └── Step 8: ops（运维工具，独立）
   │
   └── Step 9: 生产验收
```

| Step | 模块 | 依赖 | 文档 |
|------|------|------|------|
| 1 | observability | Phase 2 | [01-observability.md](./01-observability.md) |
| 2 | multi-instance | Step 1 | [02-multi-instance.md](./02-multi-instance.md) |
| 3 | audit-l2 | Step 2 | [03-audit-l2.md](./03-audit-l2.md) |
| 4 | event-driven | Phase 2 | [04-event-driven.md](./04-event-driven.md) |
| 5 | microservice | Step 4 | [05-microservice.md](./05-microservice.md) |
| 6 | ha | Step 2 | [06-ha.md](./06-ha.md) |
| 7 | security-enhance | Phase 2 | [07-security-enhance.md](./07-security-enhance.md) |
| 8 | ops | Phase 2 | [08-ops.md](./08-ops.md) |
| 9 | 生产验收 | All | 本文档 §3 |

---

## 3. 生产验收标准

Phase 3 完成后，系统需满足：

| 维度 | 指标 |
|------|------|
| 可用性 | 单实例 99.5%，多实例 99.9% |
| 可观测性 | Metrics（QPS/延迟/错误率）+ 分布式追踪 + 结构化日志 |
| 安全性 | 限流 + 锁定 + 密码策略 + 异地登录检测 + HTTPS |
| 多实例 | Casbin 策略秒级同步、缓存跨实例失效、无脏数据 |
| 数据安全 | PG 定期备份 + Redis AOF 持久化 |
| 运维 | 一键部署、DB 迁移 CI 化、Swagger 自动生成 |

---

## 4. 待决策点

| 事项 | 说明 | 状态 |
|------|------|------|
| ⚠️ K8s vs Docker Compose | Phase 3 是否上 K8s？ | 建议 Docker Compose + Nginx 足够，K8s 按需 |
| ⚠️ 微服务拆分粒度 | 先拆哪个服务？ | 建议先拆工单服务（业务独立性强） |
| ⚠️ gRPC vs HTTP | 内部通信协议 | 架构文档已决策：gRPC 内部 + REST 外部 |
| ⚠️ Redis 高可用方案 | Sentinel vs Cluster | 建议 Sentinel（简单），Cluster 按需 |
| ⚠️ PG 高可用方案 | 自建 vs 云托管 | 已决策：云托管 Cluster（2+VIP） |
| ⚠️ KMS 密钥管理 | RS256 私钥是否上 KMS | Phase 2 用文件，Phase 3 评估云 KMS |

---

## 5. 文档索引

> 标注"待编写"的文档尚需创建，当前先占位。

| 文档 | 模块 | 状态 |
|------|------|------|
| [01-observability.md](./01-observability.md) | 可观测性 | 待编写 |
| [02-multi-instance.md](./02-multi-instance.md) | 多实例部署 | 待编写 |
| [03-audit-l2.md](./03-audit-l2.md) | 审计日志 L2 | 待编写 |
| [04-event-driven.md](./04-event-driven.md) | 事件驱动 | 待编写 |
| [05-microservice.md](./05-microservice.md) | 微服务拆分 | 待编写 |
| [06-ha.md](./06-ha.md) | 高可用 | 待编写 |
| [07-security-enhance.md](./07-security-enhance.md) | 安全增强 | 待编写 |
| [08-ops.md](./08-ops.md) | 运维工具 | 待编写 |
