# 11 - 部署级分离方案（工单独立服务演进路径）

> **定位**：工单模块独立成服务的演进路径，按"代码不拆、部署先拆"原则分三档推进。微服务拆分（gRPC/CQRS/独立仓）推迟到 Phase 3+ 以后，本文档聚焦部署级分离。  
> **对应**：[phase3/README §1.3](./README.md#13-不做什么)、[deployment-evolution.md](../proposal/deployment-evolution.md)。  
> 创建日期：2026-08-25。

---

## 1. 演进三档

| 档位 | 阶段 | 代码改动 | 运维成本 | 扩缩容 | 适用 |
|---|---|---|---|---|---|
| **1 单体多副本** | Phase 2-3 | 零 | 低 | 整体 | **推荐 Phase 2-3 默认** |
| **2 部署级分离** | Phase 3 末可选 | Wire 分块 + 模块选择启动 | 中 | 工单独立 | 有独立扩缩需求时 |
| **3 真微服务** | Phase 3+ 按需 | gRPC + CQRS + 独立仓 | 高 | 完全独立 | 多团队/M2M 需求 |

> **核心原则**（[design-decisions §18](../design/design-decisions.md#18-部署与代码解耦一套代码多种部署)）：一套代码多种部署。同一二进制，配置和编排解决差异，不因换部署改 Handler/Service。

---

## 2. 档位 1：单体多副本（Phase 2-3 默认）

### 2.1 架构

```
┌──────────────────────────────────┐
│        单体进程（多副本）         │
│  中间件 → Service → Repository   │
│  Auth/User/Role/Org/Menu/Ticket  │  ← 工单在内，进程内调用
│  ResourceRegistry                │
└────────────────┬─────────────────┘
       ┌─────────┴─────────┐
       ▼                   ▼
   PG Cluster        Redis Sentinel
```

### 2.2 理由

- [deployment-evolution.md §3](../proposal/deployment-evolution.md#3-phase-2单体进程内的工单不拆-iam) 明确 Phase 2"不引入 Gateway、不切 gRPC、不换 RS256"
- 工单与三层鉴权强耦合（[ticket.md §2.2](../modules/ticket.md#22-三层鉴权在工单中的映射)），拆服务要引入 IAM 数据副本同步（CQRS），是 Phase 3+ 的活
- 代码已按 [§2.3](../proposal/deployment-evolution.md#23-代码分层隔离为未来拆分准备) 领域目录隔离（`internal/service/ticket/`），未来整包迁移零成本
- Phase 3 多实例靠 Casbin Watcher + 分布式锁，工单事件用 L1 机制（DB 持久化 + 轮询）

### 2.3 部署方式

```yaml
# docker-compose.yaml（当前模式，Phase 3 扩展为多副本）
services:
  app:
    image: zhuzhao:latest
    deploy:
      replicas: 2  # 多副本
    depends_on: [postgres, redis]
  postgres:
    image: postgres:15
  redis:
    image: redis:7
```

---

## 3. 档位 2：部署级分离（Phase 3 末可选）

### 3.1 触发条件

- 工单模块有独立扩缩容需求（工单流量大，IAM 流量小）
- 需验证服务边界是否合理，为未来真微服务化做准备
- **不满足则不拆**，保持档位 1

### 3.2 架构

```
┌──────────────────┐     ┌──────────────────┐
│  IAM 进程        │     │  工单进程        │
│  Auth/User/Role  │     │  Ticket/Storage  │
│  /Org/Menu/Casbin│     │  ResourceRegistry│
└────────┬─────────┘     └────────┬─────────┘
         │                        │
    ┌────┴────┐             ┌────┴────┐
    ▼         ▼             ▼         ▼
  PG Cluster  Redis Sentinel (共享)
```

### 3.3 实现方式：同二进制 + 模块选择启动

**代码改动最小**：同一二进制，通过 `APP_MODULES` 环境变量控制哪些 Service 注册到 Wire。

```yaml
# docker-compose.yaml（部署级分离）
services:
  iam-app:
    image: zhuzhao:latest
    environment:
      - APP_MODULES=auth,user,role,org,menu,casbin  # 只起 IAM 模块
      - PORT=33333
    deploy:
      replicas: 2
  ticket-app:
    image: zhuzhao:latest
    environment:
      - APP_MODULES=ticket,storage                    # 只起工单模块
      - PORT=33334
    deploy:
      replicas: 3  # 工单独立扩缩
  postgres:
    image: postgres:15  # 共享 DB
  redis:
    image: redis:7     # 共享 Redis
```

### 3.4 前置改动

1. **Wire Provider Set 按领域分块**（[§2.3](../proposal/deployment-evolution.md#23-代码分层隔离为未来拆分准备) 已有此设计）
2. **模块选择启动逻辑**：`internal/app/` 增加按 `APP_MODULES` 选择性注册 Provider 的编排
3. **工单进程的鉴权适配**：
   - L1 路由级：工单进程内嵌 Casbin enforcer（策略从 DB 加载，与 IAM 共享 PG）
   - L2/L3 资源级：工单进程读共享 DB 的 `user_orgs`/`organizations`（同一 PG，无 CQRS）
4. **健康检查**：`/health/live` 和 `/health/ready` 按启用的模块报告

### 3.5 优点

- 先享受部署独立（工单可单独扩容、单独重启）
- 不背代码拆分债（gRPC/protobuf/CQRS 全延后）
- 验证服务边界是否合理，再决定要不要真拆
- 共享 PG/Redis，无数据一致性问题

### 3.6 限制

- 同一 DB，工单故障可能影响 IAM（可通过连接池隔离缓解）
- 同一二进制，镜像体积不减小
- 模块选择启动需维护模块清单

---

## 4. 档位 3：真微服务化（Phase 3+ 按需）

### 4.1 触发条件

- 有真实多团队开发需求（不同团队维护不同服务）
- 有 M2M（机器对机器）调用需求
- 工单服务需要独立技术栈演进
- **不满足则不拆**，保持档位 1 或 2

### 4.2 架构

详见 [deployment-evolution.md §4-§5](../proposal/deployment-evolution.md#4-phase-3iam-独立部署)：

```
┌──────────┐     ┌──────────┐     ┌──────────────────┐
│  客户端   │────▶│ Gateway  │────▶│  IAM 服务        │
└──────────┘     │ 认证+路由 │     └──────────────────┘
                 └────┬─────┘     ┌──────────────────┐
                      ├──────────▶│  工单服务         │
                      │           └──────────────────┘
```

### 4.3 前置条件

- [ ] Phase 3+ event-driven 完成（CQRS 数据复制依赖事件基础设施）
- [ ] gRPC + Protobuf 通信协议定义
- [ ] IAM 数据 CQRS 复制管道就绪
- [ ] 服务发现/熔断/链路追踪基础设施就绪

### 4.4 鉴权适配（关键挑战）

工单独立成服务后，三层鉴权的实现变化：

| 层 | 单体（档位 1/2） | 微服务（档位 3） |
|---|---|---|
| L1 路由级 | Casbin 进程内 | Gateway 路由级 + IAM 提供策略 |
| L2 资源级 | 直接查 DB | **CQRS 复制 IAM 数据到工单本地副本**（秒级延迟） |
| L3 属主 | 直接查 DB | `created_by`/`assigned_to` 本地有；`org admin/owner` 查本地副本 |

> **鉴权不进 Port** 原则（[ticket.md §5.6](../modules/ticket.md#56-ticketengine-port引擎可替换边界)）在此体现：无论单体还是微服务，`TicketEngine` Port 实现不变，鉴权仍在 `TicketService` 层。

---

## 5. 决策建议

| 场景 | 推荐档位 |
|------|---------|
| Phase 2-3 生产部署 | **档位 1**（单体多副本） |
| Phase 3 末有工单独立扩缩需求 | **档位 2**（部署级分离，验证边界） |
| Phase 3+ 有多团队/M2M 需求 | **档位 3**（真微服务，需 event-driven 先就绪） |
| 无明确拆分需求 | 保持档位 1，不提前拆 |

> **核心原则**：[deployment-evolution.md §6](../proposal/deployment-evolution.md#6-演进原则)——不提前拆分。Phase 1-2 单体跑通，有真实需求才在 Phase 3 拆。代码分层已为拆分做好准备，但部署级拆分最早 Phase 3 末，真微服务化留 Phase 3+ 以后。
