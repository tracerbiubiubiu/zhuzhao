# 部署演进方案

> 单体底座 → IAM 独立部署 → 微服务化的演进路径。
>
> 创建日期：2026-08-12

---

## 1. 演进阶段总览

```
Phase 1：单体底座（当前）
  所有功能在一个进程内，Wire DI 注入

Phase 2：仍为单体，工单进进程
  资源级鉴权 + 工单 + 对象存储，不拆 IAM

Phase 3：IAM 独立 + 微服务化
  认证鉴权底座独立部署，业务服务拆分，gRPC 内部通信
```

> **一套代码多种部署**（全阶段）：同一二进制与业务代码；PG 单节点/Cluster、Redis 单实例/Sentinel、App 单副本/多副本 差异由 **配置 + 编排** 解决，不因换部署改 Handler/Service。详见 [design-decisions §18](../design/design-decisions.md#18-部署与代码解耦一套代码多种部署)。Phase 1 仅 **默认单 App 验收**，不实现 Watcher 等，**不是** 另一套代码。

---

## 2. Phase 1：单体底座（当前）

### 2.1 部署架构

```
┌─────────────────────────────────┐
│         单体进程（server）        │
│  ┌───────────────────────────┐  │
│  │     中间件层               │  │
│  │  JWT → Casbin → Audit     │  │
│  └───────────┬───────────────┘  │
│  ┌───────────▼───────────────┐  │
│  │     Service 层             │  │
│  │  Auth/User/Role/Org/Menu  │  │
│  │  Ticket...                │  │
│  │  ResourceRegistry         │  │
│  └───────────┬───────────────┘  │
│  ┌───────────▼───────────────┐  │
│  │     Repository 层          │  │
│  └───────────┬───────────────┘  │
│  ┌───────────▼───────────────┐  │
│  │     基础设施               │  │
│  │  PG 15  │  Redis 6.2      │  │
│  └───────────────────────────┘  │
└─────────────────────────────────┘
```

### 2.2 特点

- 所有 Service 在一个进程内
- ResourceRegistry 是内存级单例
- 各 Service 构造函数自注册资源
- Casbin enforcer 全局唯一（路由级）
- 代码分层清晰：middleware → handler → service → repository

### 2.3 代码分层隔离（为未来拆分准备）

> **领域目录 SSOT**：[architecture.md §3.5](../design/architecture.md#35-领域模块目录约定单仓可拆分)（横向分层 + 纵向 `{domain}` 子包、跨域调用规则、拆分清单）。

```
internal/
├── middleware/       # 中间件层（未来 → Gateway）
├── handler/{domain}/   # Handler 层（未来 → 各服务的 API 层）
├── service/{domain}/   # Service 层（未来 → 各服务的业务层）
├── repository/{domain}/# Repository 层（未来 → 各服务的数据层）
├── integration/      # 外部系统（HR 等，Phase 2b）
├── pkg/resource/     # 资源注册表框架（未来 → 共享库）
├── casbin/           # Casbin 封装（未来 → Gateway / IAM）
└── app/              # 应用编排（Wire DI；拆分时按领域块复制）
```

**关键原则**：Service 之间通过 **接口** 调用，不跨域直引 Repository；目录按 **领域** 划分，便于 Phase 3 整包迁移到新 `cmd/`。

---

## 3. Phase 2：单体进程内的工单（不拆 IAM）

Phase 2 工单、资源级鉴权、对象存储仍与认证底座**同一进程**。不引入 Gateway、不切 gRPC、不换 RS256。拆分发生在 Phase 3。

---

## 4. Phase 3：IAM 独立部署

> 原文档将本节标为 Phase 2，已与 roadmap 对齐：拆服务属于 Phase 3。
>
> **通信协议**：REST 对外，gRPC 对内。详见 [design-decisions.md#13](../design/design-decisions.md#13-微服务通信协议grpc-内部--rest-外部)。

### 4.1 部署架构

```
┌──────────┐     ┌──────────────────┐     ┌──────────────────┐
│  客户端   │────▶│   API Gateway    │────▶│  IAM 底座         │
│          │     │  认证+路由级鉴权  │     │  用户/角色/组织   │
└──────────┘     └────────┬─────────┘     │  菜单/Casbin策略  │
                          │               └──────────────────┘
                          │ direct
                          ▼
                 ┌──────────────────┐
                 │  业务服务（工单）  │
                 │  资源级鉴权（内联）│
                 │  调用 IAM 获取身份 │
                 └──────────────────┘
```

### 4.2 IAM 底座职责

| 职责 | 说明 |
|------|------|
| 用户管理 | CRUD + 启用禁用 + 密码策略 |
| 角色管理 | CRUD + 菜单分配 + 权限配置 |
| 组织管理 | 组织树 + 虚拟组 + 成员管理 |
| 菜单管理 | CRUD + 菜单-API 绑定 |
| 认证 | 登录 + 双 Token + 登出 + 多设备 |
| 路由级鉴权 | Casbin RBAC |
| 权限查询 API | 为业务服务提供用户角色/权限查询接口 |

### 4.3 业务服务职责

| 职责 | 说明 |
|------|------|
| 资源级鉴权 | 自己做（代码内联 + 独立 enforcer） |
| 调用 IAM | 获取用户身份、角色、组织信息 |
| 业务逻辑 | 工单流转、审批等 |

### 4.4 服务间通信

**协议：gRPC + Protobuf（内部 East-West 流量）**

对外仍用 REST（Gin + gRPC-Gateway 自动转换），内部服务间用 gRPC。

```
业务服务 → IAM 底座（gRPC）：
  rpc GetUser(GetUserRequest) → User
  rpc GetUserRoles(GetUserRolesRequest) → UserRolesResponse
  rpc GetUserOrgs(GetUserOrgsRequest) → UserOrgsResponse
  rpc GetUserPermissions(GetUserPermissionsRequest) → PermissionsResponse
```

> 详见 [design-decisions.md#13 微服务通信协议](../design/design-decisions.md#13-微服务通信协议grpc-内部--rest-外部)

### 4.5 数据一致性

**方案：数据复制（CQRS）**

IAM 底座发布事件，业务服务订阅并维护本地副本：

```
IAM 底座：
  管理员修改用户角色 → DB 写入 → 发布事件 user.role.changed

业务服务：
  订阅 user.role.changed → 更新本地 user_roles 副本表
  资源级鉴权时查本地副本，不实时调用 IAM
```

**优点**：无运行时同步调用，延迟低
**缺点**：数据有短暂延迟（秒级），需要事件基础设施（Redis Pub/Sub 或 MQ）

### 4.6 Phase 3 拆分的前提条件

- IAM 底座已稳定运行
- 业务模块已明确边界
- 服务间通信协议已定义
- 事件广播机制已就绪

---

## 5. Phase 3 续：微服务化与 PDP

### 5.1 部署架构

```
┌──────────┐     ┌──────────┐     ┌──────────────────┐
│  客户端   │────▶│ Gateway  │────▶│  IAM 服务        │
│          │     │ 认证+路由 │     │  用户/角色/组织   │
└──────────┘     │ 级鉴权   │     └──────────────────┘
                 └────┬─────┘     ┌──────────────────┐
                      │           │  工单服务         │
                      ├──────────▶│  资源级鉴权       │
                      │           └──────────────────┘
                      │           ┌──────────────────┐
                      ├──────────▶│  审批服务         │
                      │           │  资源级鉴权       │
                      │           └──────────────────┘
                      │           ┌──────────────────┐
                      └──────────▶│  PDP 服务（可选） │
                                  │  SpiceDB/Cerbos  │
                                  └──────────────────┘
```

### 5.2 PDP 服务评估

Phase 3 可选引入独立 PDP（Policy Decision Point）服务：

| 方案 | 适用场景 | 优点 | 缺点 |
|------|---------|------|------|
| OpenFGA | ReBAC（关系链） | CNCF 项目，REST 优先，Go SDK | 独立服务，运维成本 |
| SpiceDB | 复杂 ReBAC（多级关系链） | Zanzibar 论文级，Google 规模 | 独立服务，运维成本 |
| Cerbos | 策略可配置，YAML 定义策略 | 轻量，策略即代码 | 社区较小 |
| OPA | 通用策略引擎 | CNCF 项目，生态好 | 关系遍历不擅长 |
| 继续代码内联 | 简单场景 | 零依赖 | 策略散落各服务 |

**引入时机**：当多个业务服务的资源级鉴权策略变得复杂、需要统一管理时。

### 5.3 服务发现与通信

| 维度 | 方案 |
|------|------|
| 服务发现 | Consul / K8s DNS |
| 同步通信 | gRPC（内部）/ HTTP（对外） |
| 异步通信 | Redis Pub/Sub（轻量）/ Kafka（重量级） |
| 熔断限流 | go-zero / sentinel |
| 链路追踪 | OpenTelemetry |

---

## 6. 演进原则

1. **代码分层隔离**——Phase 1 的代码分层为未来拆分做准备
2. **接口优先**——Service 之间通过接口调用，便于替换为远程调用
3. **数据复制优于同步调用**——拆服务后用 CQRS 复制 IAM 数据到业务服务
4. **不提前拆分**——Phase 1–2 单体跑通，有真实需求才在 Phase 3 拆
5. **PDP 按需引入**——策略简单时代码内联，复杂时才引入 PDP
6. **事件驱动**——服务间通过事件广播状态变更，避免同步依赖

---

## 7. 旧系统的演进路线参考

旧系统 zhuzhao 的演进路线图也印证了这个方向：

```
阶段 1（当前）：单体底座（全部在一个进程）
  → 认证 + 路由级鉴权 + 资源级鉴权全在一个进程

阶段 3（中期）：网关 + RBAC 分离
  → 网关做路由级，底座做 RBAC 管理

阶段 4（长期）：微服务化
  → 各服务自己做资源级鉴权
```

新框架从 Phase 1 就在代码分层上隔离中间件层（路由级）和 Service 层（资源级），为未来微服务化天然做好准备。
