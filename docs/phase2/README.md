# Phase 2 实现计划：业务可用

> **核心目标**：让第一个业务模块（工单）带着资源级权限跑起来。安全底线（登录限流、会话吊销）已在 Phase 1 完成，Phase 2 不再堆平台能力。
>
> 创建日期：2026-08-12  
> 修订：2026-08-13 — 收窄范围；**拆为 Phase 2a / 2b**，避免一次改动过大。

---

## 0. 子阶段总览

| 子阶段 | 目标 | 典型交付 | 验收焦点 |
|--------|------|----------|----------|
| **2a** | 资源级鉴权 + 工单 MVP | Registry 实现、TicketResource、工单 CRUD+状态机 | **assigned** 范围：仅本人创建/被分派的工单 |
| **2b** | 组织范围 + 附件 + 体验 | 虚拟组/scope、对象存储、多设备 UI、密码策略 | **group/all** 部门隔离、附件上传、临时成员 |

**为什么拆**：

1. **org-enhance**（虚拟组、组织角色、scope、临时成员）与 **storage** 都是独立大块，和工单 CRUD 叠在一起难 review、难回滚。
2. 工单三层鉴权可以先在 **assigned** 范围跑通（属主 + 处理人），已能验证 ResourceRegistry 设计；ltree **group** 过滤放到 2b 不阻塞主线。
3. **auth-enhance**（设备 UI、密码复杂度）与工单无硬依赖，自然归入 2b。

**建议顺序**：Phase 1 验收 → **2a 合并** → 联调工单主路径 → 再开 **2b**。

---

## 1. Phase 2 边界

### 1.1 Phase 2a — 做什么

| 类别 | 模块 | 核心能力 | 文档 |
|------|------|---------|------|
| 资源级鉴权 | [authz-resource](./02-authz-resource.md) | ResourceRegistry 实现、属主判断、**assigned** 列表过滤、`resource_owners` 表 | 待编写 |
| 工单模块 | [ticket](./09-ticket.md) | 类型配置、状态机、TicketResource 注册；**无附件** | 待编写 |

**2a 刻意不做**：虚拟组、group/all scope、BFS 三源角色、对象存储、多设备 UI。

**2a 验收**（可独立演示）：

```
1. 有 ticket:list 权限的用户 A 只能看到 created_by=A 或 assigned_to=A 的工单
2. 对可见工单可 create / update / close（路由级 + 资源级）
3. 不可见工单详情返回 404（非 403）
4. ResourceRegistry.Authorize / GetFilter 有单元测试 + 工单集成测试
```

### 1.2 Phase 2b — 做什么

| 类别 | 模块 | 核心能力 | 文档 |
|------|------|---------|------|
| 组织增强 | [org-enhance](./03-org-enhance.md) | 虚拟组、组织角色、scope（all/group/assigned）、临时成员有效期、BFS 三源角色 | 待编写 |
| 文件存储 | [storage](./10-storage.md) | S3 兼容、预签名 URL、工单附件 | 待编写 |
| 工单增强 | [ticket](./09-ticket.md) §2b | 列表过滤升级为 group/all；附件关联 | 待编写 |
| 认证增强 | [auth-enhance](./01-auth-enhance.md) | 多设备列表/踢出、密码复杂度 | 待编写 |

**2b 验收**：

```
1. 主管（scope=group）可见本组织及子组织工单
2. 虚拟组成员按 expires_at 自动失效
3. 工单附件预签名上传 + 下载
4. 设备列表/踢出、密码复杂度策略生效
```

### 1.3 不做什么（整个 Phase 2）

| 不做 | 原因 | 阶段 |
|------|------|------|
| 每资源独立 Casbin Enforcer | 代码内联 + ltree 足够 | 策略需可配置时 |
| JWT RS256 / JWKS | 仍是单体 | Phase 3b |
| AK/SK | 无 M2M 调用方 | Phase 3b / 按需 |
| IAM 独立 / gRPC | 同进程 | Phase 3b |
| 缓存平台 | 工单跑通后再说 | Phase 3b |
| 审计异步 / Redis List | Phase 1 同步够用 | Phase 3a |
| API 级通用限流 | 登录限流在 Phase 1 | Phase 3a |

### 1.4 前置条件

**Phase 2a** 开始前，Phase 1 必须已完成：

- [ ] 认证鉴权框架可运行（含对抗路径验收）
- [ ] 所有 Phase 1 测试用例通过
- [ ] DB 迁移可重复执行（幂等）

**Phase 2b** 开始前，**Phase 2a** 必须已完成：

- [ ] 工单 MVP + assigned 范围验收通过
- [ ] TicketResource 已注册且测试覆盖主路径

---

## 2. 实施顺序

### 2.1 Phase 2a

```
Phase 1 完成
   │
   ├── Step 1: authz-resource（Registry + assigned GetFilter + resource_owners）
   │      │
   │      └── Step 2: ticket MVP（表结构、状态机、API、TicketResource）
   │
   └── Step 3: 2a 集成验收（assigned 范围 + 三层鉴权主路径）
```

| Step | 子阶段 | 模块 | 依赖 | 文档 |
|------|--------|------|------|------|
| 1 | 2a | authz-resource | Phase 1 | [02-authz-resource.md](./02-authz-resource.md) |
| 2 | 2a | ticket | Step 1 | [09-ticket.md](./09-ticket.md) |
| 3 | 2a | 集成验收 | Step 1–2 | 本文档 §1.1 |

### 2.2 Phase 2b

```
2a 验收通过
   │
   ├── Step 4: org-enhance（虚拟组 / scope / BFS / 临时成员）
   │      │
   │      └── Step 5: ticket 升级（group/all 过滤 + 404 语义不变）
   │
   ├── Step 6: storage（MinIO + 预签名 + 工单附件）
   │
   ├── Step 7: auth-enhance（可与 Step 6 并行）
   │
   └── Step 8: 2b 集成验收
```

| Step | 子阶段 | 模块 | 依赖 | 文档 |
|------|--------|------|------|------|
| 4 | 2b | org-enhance | 2a | [03-org-enhance.md](./03-org-enhance.md) |
| 5 | 2b | ticket（scope 升级） | Step 4 | [09-ticket.md](./09-ticket.md) |
| 6 | 2b | storage | 2a | [10-storage.md](./10-storage.md) |
| 7 | 2b | auth-enhance | Phase 1 | [01-auth-enhance.md](./01-auth-enhance.md) |
| 8 | 2b | 集成验收 | Step 4–7 | 本文档 §1.2 |

---

## 3. 待决策点

| 事项 | 说明 | 状态 |
|------|------|------|
| 2a 是否含 BFS 三源 | 建议 **不含**，assigned 只需直接角色；BFS 随 org-enhance 进 2b | ✅ 建议已采纳 |
| 工单状态机 | 自研简单状态机 | 建议自研，见 `modules/ticket.md` |
| 虚拟组与实体组织 | 统一建表 + org_type | ✅ 已决策 |
| 对象存储 | 开发 MinIO，生产按需 | 建议 MinIO |
| 附件 | 预签名 URL 直传 | 建议预签名，**2b 再做** |

---

## 4. 文档索引

| 文档 | 子阶段 | 状态 |
|------|--------|------|
| [02-authz-resource.md](./02-authz-resource.md) | 2a | 待编写 |
| [09-ticket.md](./09-ticket.md) | 2a + 2b | 待编写 |
| [03-org-enhance.md](./03-org-enhance.md) | 2b | 待编写 |
| [10-storage.md](./10-storage.md) | 2b | 待编写 |
| [01-auth-enhance.md](./01-auth-enhance.md) | 2b | 待编写 |

> cache / audit-async / RS256 / AK/SK 等已后移至 Phase 3，见 [phase3/README.md](../phase3/README.md)。
