# Phase 2 实现计划：业务可用

> **核心目标**：在 Phase 1 认证鉴权框架基础上，补齐资源级鉴权、安全加固、缓存体系，并启动第一个业务模块（工单）。
>
> 创建日期：2026-08-12

---

## 1. Phase 2 边界

### 1.1 做什么

| 类别 | 模块 | 核心能力 | 文档 |
|------|------|---------|------|
| 安全加固 | [auth-enhance](./01-auth-enhance.md) | 多设备管理、登录限流/锁定、密码复杂度、密码重置 | 待编写 |
| 资源级鉴权 | [authz-resource](./02-authz-resource.md) | ltree 组织关系查询、属主判断、每资源独立 Enforcer | 待编写 |
| 组织增强 | [org-enhance](./03-org-enhance.md) | 虚拟组、组织角色、组织级权限（scope） | 待编写 |
| 缓存体系 | [cache](./04-cache.md) | 权限缓存、菜单缓存、组织缓存、Cache-Aside + singleflight | 待编写 |
| 审计日志增强 | [audit-enhance](./05-audit-enhance.md) | channel + batch 异步写入、日志过期清理 | 待编写 |
| 限流中间件 | [ratelimit](./06-ratelimit.md) | Redis + 令牌桶/滑动窗口，API 级限流 | 待编写 |
| AK/SK 管理 | [m2m-aksk](./07-m2m-aksk.md) | 服务间认证完整实现、AK/SK 管理 API | 待编写 |
| JWT 升级 | [jwt-rs256](./08-jwt-rs256.md) | HS256 → RS256 + JWKS 公钥分发 | 待编写 |
| 工单模块 | [ticket](./09-ticket.md) | 工单类型配置、状态机、权限模型 | 待编写（参考 `docs/modules/ticket.md`） |
| 文件存储 | [storage](./10-storage.md) | S3 兼容对象存储、预签名 URL 直传、附件管理 | 待编写 |

### 1.2 不做什么

| 不做 | 原因 | 阶段 |
|------|------|------|
| 多实例部署 | Phase 2 仍单实例 | Phase 3 |
| Casbin Watcher | 多实例才需要 | Phase 3 |
| 分布式锁 | 多实例才需要 | Phase 3 |
| Metrics / 分布式追踪 | 可观测性 Phase 3 | Phase 3 |
| 事件驱动（Asynq/Outbox） | Phase 2 先用进程内 channel | Phase 3 |
| 多租户 | 预留字段即可 | Phase 3 |

### 1.3 前置条件

Phase 2 开始前，Phase 1 必须已完成并通过验收：

- [ ] 认证鉴权框架可运行（登录、鉴权、CRUD）
- [ ] 所有 Phase 1 测试用例通过
- [ ] DB 迁移脚本可重复执行（幂等）
- [ ] 健康检查接口正常

---

## 2. 实施顺序

```
Phase 1 完成
   │
   ├── Step 1: cache（缓存基础设施，后续模块依赖）
   │      │
   │      ├── Step 2: auth-enhance（安全加固，依赖 Redis）
   │      │
   │      ├── Step 3: authz-resource（资源级鉴权，依赖 ltree + cache）
   │      │      │
   │      │      └── Step 4: org-enhance（组织增强，依赖资源级鉴权框架）
   │      │
   │      ├── Step 5: ratelimit（限流中间件，独立）
   │      │
   │      ├── Step 6: audit-enhance（审计日志升级，独立）
   │      │
   │      ├── Step 7: m2m-aksk（AK/SK 完整实现，独立）
   │      │
   │      ├── Step 8: jwt-rs256（JWT 升级，独立）
   │      │
   │      ├── Step 9: storage（文件存储，工单附件依赖）
   │      │
   │      └── Step 10: ticket（工单模块，依赖 authz-resource + org-enhance + storage）
   │
   └── Step 11: 集成验收
```

| Step | 模块 | 依赖 | 文档 |
|------|------|------|------|
| 1 | cache | Phase 1 | [04-cache.md](./04-cache.md) |
| 2 | auth-enhance | Step 1 | [01-auth-enhance.md](./01-auth-enhance.md) |
| 3 | authz-resource | Step 1 | [02-authz-resource.md](./02-authz-resource.md) |
| 4 | org-enhance | Step 3 | [03-org-enhance.md](./03-org-enhance.md) |
| 5 | ratelimit | Phase 1 | [06-ratelimit.md](./06-ratelimit.md) |
| 6 | audit-enhance | Phase 1 | [05-audit-enhance.md](./05-audit-enhance.md) |
| 7 | m2m-aksk | Phase 1 | [07-m2m-aksk.md](./07-m2m-aksk.md) |
| 8 | jwt-rs256 | Phase 1 | [08-jwt-rs256.md](./08-jwt-rs256.md) |
| 9 | storage | Phase 1 | [10-storage.md](./10-storage.md) |
| 10 | ticket | Step 3+4+9 | [09-ticket.md](./09-ticket.md) |
| 11 | 集成验收 | All | 本文档 §1.3 |

---

## 3. 待决策点

| 事项 | 说明 | 状态 |
|------|------|------|
| ⚠️ 事件驱动方案 | Phase 2 用进程内 channel 还是提前引入 Asynq？ | 建议进程内 channel，Phase 3 再引入 Asynq |
| ⚠️ 工单状态机 | 自研简单状态机还是引入开源库？ | 建议自研，参考 `docs/modules/ticket.md` |
| ⚠️ 虚拟组与实体组织的关系 | 统一建表还是分表？ | 架构文档已决策：统一建表 + org_type 区分 |
| ⚠️ RS256 密钥管理 | 私钥存文件还是 KMS？ | Phase 2 用文件 + 环境变量，Phase 3 评估 KMS |
| ⚠️ 限流策略 | 全局限流还是按用户/按 IP 限流？ | 建议按 API 分组配置，支持按用户和按 IP |
| ⚠️ 对象存储方案 | 自建 MinIO 还是云服务（S3/OSS/R2）？ | 建议自建 MinIO 开发，生产按需选云服务 |
| ⚠️ 前端直传 vs 后端转发 | 文件上传走预签名 URL 直传还是后端转发？ | 建议预签名 URL 直传，省带宽 |

---

## 4. 文档索引

> 标注"待编写"的文档尚需创建，当前先占位。

| 文档 | 模块 | 状态 |
|------|------|------|
| [01-auth-enhance.md](./01-auth-enhance.md) | 安全加固 | 待编写 |
| [02-authz-resource.md](./02-authz-resource.md) | 资源级鉴权 | 待编写 |
| [03-org-enhance.md](./03-org-enhance.md) | 组织增强 | 待编写 |
| [04-cache.md](./04-cache.md) | 缓存体系 | 待编写 |
| [05-audit-enhance.md](./05-audit-enhance.md) | 审计日志增强 | 待编写 |
| [06-ratelimit.md](./06-ratelimit.md) | 限流中间件 | 待编写 |
| [07-m2m-aksk.md](./07-m2m-aksk.md) | AK/SK 管理 | 待编写 |
| [08-jwt-rs256.md](./08-jwt-rs256.md) | JWT 升级 | 待编写 |
| [09-ticket.md](./09-ticket.md) | 工单模块 | 待编写（参考 `docs/modules/ticket.md`） |
| [10-storage.md](./10-storage.md) | 文件存储 | 待编写 |
