# 方案总览

> 本文档是综合方案的总入口，结合旧系统 zhuzhao 的实践经验和业界优秀实践，整理一版完整的认证鉴权底座方案。
>
> 与 `design/architecture.md` 的区别：架构文档关注"框架怎么搭"（技术视角），本文档关注"业务怎么做"（方案视角，含场景、流程、演进路径）。
>
> 创建日期：2026-08-12
> 状态：方案设计阶段（设计考虑多种情况，实现分步进行，不追求一步到位）

---

## 1. 目标系统

### 1.1 系统定位

**通用办公管理后台的认证鉴权底座**。提供用户管理、组织架构、角色权限、动态路由等基础能力，支撑工单、审批、内容管理等业务模块。

### 1.2 典型场景

| 场景 | 描述 | 鉴权需求 |
|------|------|---------|
| 用户登录 | 账号密码登录，多设备在线 | 双 Token + 多设备管理 |
| 菜单渲染 | 登录后前端根据返回的菜单树渲染页面 | 动态路由 + 按钮权限码 |
| 数据管理 | 管理员管理用户/角色/组织/菜单 | 路由级 RBAC + 资源级鉴权 |
| 部门权限 | 每个部门/虚拟组织有对应权限 | 组织层级继承 + 资源级过滤 |
| 工单流转 | 用户只能看到本部门的工单 | 资源级列表过滤（数据级权限） |
| 跨部门协作 | 虚拟组成员可访问跨部门资源 | 虚拟组 + 组织关系遍历 |
| 部门内数据隔离 | 同部门成员可看本部门数据，不可看其他部门 | 资源级列表过滤（ltree 路径包含判断） |
| 多组织交叉权限 | 一个用户归属多个虚拟组织，各组织权限独立生效 | 多组织角色合并 + 取并集 |
| 组织管理层级 | 上级可查看下级部门数据，不可查看平级或上级 | ltree 路径前缀匹配 |
| 临时授权 | 用户临时加入虚拟组完成特定任务，到期自动退出 | 虚拟组成员有效期 + 过期清理 |
| 工单类型扩展 | 管理员新增工单类型（故障/请求/变更），配置自定义字段 | 配置驱动 + Hook 扩展（见 `modules/ticket.md`） |
| 事件触发 | 工单创建/状态变更触发通知、SLA 计时 | 事件驱动模块（见 `design-decisions.md` §16） |

### 1.3 设计原则

1. **设计考虑多种情况，实现分步进行**——目标定好，可以先不实现或不完整实现
2. **不过度设计**——Phase 1 用最简方案跑通，Phase 2/3 按需增强
3. **代码分层隔离**——为未来微服务化做准备，但不提前拆分
4. **幂等优先**——所有初始化和同步操作必须幂等

---

## 2. 架构全景

```
┌─────────────────────────────────────────────────────────────────┐
│                        客户端（前端）                             │
└────────────────────────────┬────────────────────────────────────┘
                             │ HTTPS
┌────────────────────────────▼────────────────────────────────────┐
│                     Phase 1：单体底座                             │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                    中间件层                              │    │
│  │  Recovery → Logger → RequestID → CORS → SecurityHeaders → JWT → Casbin → Audit │    │
│  └────────────────────────────┬────────────────────────────┘    │
│  ┌────────────────────────────▼────────────────────────────┐    │
│  │                   Handler 层                             │    │
│  │  Auth │ User │ Role │ Org │ Menu │ Audit │ Ticket...    │    │
│  └────────────────────────────┬────────────────────────────┘    │
│  ┌────────────────────────────▼────────────────────────────┐    │
│  │                   Service 层                              │    │
│  │  AuthService   │ UserService   │ RoleService             │    │
│  │  OrgService    │ MenuService   │ AuditService            │    │
│  │  TicketService │ ResourceRegistry（资源自注册）           │    │
│  └────────────────────────────┬────────────────────────────┘    │
│  ┌────────────────────────────▼────────────────────────────┐    │
│  │                  Repository 层                            │    │
│  └────────────────────────────┬────────────────────────────┘    │
│  ┌────────────────────────────▼────────────────────────────┐    │
│  │                   基础设施                                │    │
│  │  PostgreSQL 15  │  Redis 6.2  │  Casbin Enforcer(s)     │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                  Phase 2：IAM 独立部署                            │
│  ┌──────────┐     ┌──────────┐     ┌──────────────────────┐    │
│  │ Gateway  │────▶│ IAM 底座  │     │  业务服务（工单等）   │    │
│  │ 认证+路由 │     │ 用户/角色 │◀────│  资源级鉴权（内联）   │    │
│  │ 级鉴权   │     │ 组织/菜单 │     │  调用 IAM 获取身份    │    │
│  └──────────┘     └──────────┘     └──────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. 技术选型汇总

| 维度 | 选型 | 理由 |
|------|------|------|
| 语言 | Go 1.22+ | 标准库 slog 需 1.21+，项目实际用 1.26 |
| HTTP | Gin | 中间件生态成熟 |
| 数据库 | PostgreSQL 15 | 原生 ACID + ltree + JSONB，云托管 Cluster 高可用 |
| 缓存 | Redis 6.2 | 缓存/会话/限流/分布式锁 |
| 路由级鉴权 | Casbin v2（SyncedEnforcer） | 成熟的 RBAC 策略引擎 |
| 资源级鉴权 | 代码内联 + ltree SQL（Phase 1）→ OpenFGA/SpiceDB（Phase 2 按需） | Phase 1 零依赖，Phase 2 按需引入 PDP |
| 内部通信（Phase 2+） | gRPC + Protobuf | East-West 流量标准方案，详见 design-decisions.md §13 |
| JWT | golang-jwt/v5 | 标准库级 JWT 实现 |
| 密码 | bcrypt | 业界标准 |
| DI | Google Wire | 编译时生成，类型安全 |
| 日志 | slog + Lumberjack | 标准库零依赖 |
| 配置 | Viper | 文件+环境变量+热更新 |
| DB 迁移 | golang-migrate | SQL 文件 + CLI，生态成熟 |
| API 文档 | swag | 从代码注解生成 OpenAPI |
| 反向代理 | Nginx | Phase 1 无域名最简方案，有域名后可评估 Caddy |
| 前端 | Vue 3 + Element Plus / Ant Design Vue | Vue 3 生态成熟，组件库二选一（团队偏好决定） |

### 3.1 前后端协作约定

**前端技术方案**：Vue 3 + Element Plus 或 Ant Design Vue（具体组件库待定）。

**后端为前端配合的设计要点**：

| 要点 | 说明 |
|------|------|
| API 仅用 GET + POST | 前端所有请求只用这两个方法，更新/删除用 `POST /xxx/update`、`POST /xxx/delete` |
| 统一响应格式 | `{code, message, data, request_id}`，前端统一拦截器处理 |
| 菜单树接口 | `GET /user/menus` 返回树形结构，前端直接渲染路由和菜单 |
| 权限码接口 | `GET /user/permissions` 返回权限码列表，前端 `v-if` 控制按钮显隐 |
| 分页格式 | `{list, total, page, page_size}`，前端分页组件直接绑定 |
| 错误码分段 | 前端按模块段位区分错误来源，统一弹窗或跳转 |
| Token 管理 | 前端存储 AT + RT，AT 过期时自动用 RT 刷新，RT 失效则跳转登录 |
| 静态文件 | Phase 1 由 Nginx 托管前端 SPA 构建产物（`/var/www/zhuzhao`） |

---

## 4. 文档索引

| 文档 | 内容 |
|------|------|
| [auth-design.md](./auth-design.md) | 认证鉴权完整方案：AuthN + AuthZ + 资源抽象 |
| [data-init.md](./data-init.md) | 数据初始化与幂等性方案 |
| [resource-model.md](./resource-model.md) | 资源抽象与自注册机制方案 |
| [deployment-evolution.md](./deployment-evolution.md) | 部署演进：单体 → 微服务化路径 |

---

## 5. 遗漏项检查清单

> 结合业界实践和旧系统经验，以下事项已纳入考虑但可能尚未在方案中详细展开。

### 5.1 已考虑并记录

| 事项 | 文档位置 |
|------|---------|
| JWT 无状态 vs 权限实时生效 | design-decisions.md §1 |
| Redis 重启与数据恢复 | design-decisions.md §2 |
| 三层鉴权拆分理由 | design-decisions.md §3 |
| 通用工具包提取时机 | design-decisions.md §4 |
| 资源级鉴权架构（Gateway 下放） | design-decisions.md §5 |
| 资源抽象与自注册 | design-decisions.md §6, proposal/resource-model.md |
| 初始化幂等性 | design-decisions.md §7, proposal/data-init.md |
| Casbin 策略爆炸（每资源独立 enforcer） | design-decisions.md §8 |
| 旧系统对比与借鉴 | system-comparison.md |

### 5.2 已识别但需补充

| 事项 | 说明 | 优先级 |
|------|------|--------|
| 密码安全策略 | 旧系统有完整的 PasswordValidator（复杂度+bcrypt上限+first_login改密），新框架需对齐 | Phase 1 |
| 登录安全 | 旧系统有 LoginLocker（Lua 原子脚本+fail-close），新框架需对齐 | Phase 1 |
| 审计日志可靠性 | 异步写入 + channel 满降级策略 | Phase 2 |
| 审计日志过期清理 | PG 分区表或 cron 定期归档 | Phase 2 |
| 配置热更新 | Viper WatchConfig，哪些配置支持热更新需明确 | Phase 2 |
| 数据库高可用 | Phase 1 单节点 PG；Phase 2 迁移云托管 PG Cluster（2+VIP） | Phase 2 |
| ReBAC 引擎选型 | Phase 1 ltree+代码内联；Phase 2 按需评估 OpenFGA/SpiceDB | Phase 2 |
| 微服务通信协议 | Phase 2 gRPC 内部 + REST 外部（gRPC-Gateway） | Phase 2 |
| 多租户预留 | 表和模型预留 tenant_id，暂不实现 | Phase 3 |
| API 版本管理 | `/api/v1` 前缀，未来版本迁移策略 | Phase 2 |
| 服务间通信 | Phase 2 微服务化时的服务发现、负载均衡、熔断 | Phase 2 |
| 可观测性 | Prometheus metrics + OpenTelemetry tracing | Phase 3 |
| CI/CD | GitHub Actions / GitLab CI 配置 | Phase 2 |
| 灰度发布 | 多版本共存、流量切分 | Phase 3 |
| 工单模块 | Phase 2 开始，设计已完成（`modules/ticket.md`） | Phase 2 |
| 事件驱动模块 | Phase 1 进程内事件；Phase 2 Outbox + Asynq（`design-decisions.md` §16） | Phase 2 |
| 操作日志中间件 | 借鉴 ginfast：异步+脱敏+自动识别模块/类型（`design-decisions.md` §17） | Phase 1 |
| 授权引擎抽象 | 借鉴 go-wind-admin：Casbin/OPA/Zanzibar 可切换接口 | Phase 2 |

### 5.3 业界实践参考

| 实践 | 来源 | 新框架应用 |
|------|------|-----------|
| PEP/PDP 分层鉴权 | OWASP / NIST SP 800-204B | 路由级（PEP-1）+ 资源级（PEP-2）分层 |
| 资源注册表模式 | Altinn ResourceRegistry / DanDoeTech | Resource 接口 + 自注册机制 |
| 中央策略管理 + 本地执行 | Cerbos / Oso | Phase 1 代码内联，Phase 2 可选 PDP |
| 文档优先，策略后同步 | 旧系统 zhuzhao | DB 是 source of truth，Casbin 是 derived |
| desired-state sync | 旧系统 zhuzhao | 启动时同步系统数据，幂等 |
| 服务启动解耦 | NILUS bootstrap dependencies | 启动不依赖远程服务，本地配置+缓存 |
| 数据复制替代同步调用 | microservices.io fetch/replicate | Phase 2 IAM 数据复制到业务服务 |
