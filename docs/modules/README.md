# 模块设计文档

> 每个核心模块的详细设计，结合旧系统 zhuzhao 的经验和新框架的架构决策。
>
> 与 `design/architecture.md` 的区别：架构文档描述全局结构，本文档集描述每个模块的内部设计。
>
> **编码与分阶段边界以 [`phase1/`](../phase1/README.md)、[`roadmap.md`](../roadmap.md) 为准。** 本文档集偏跨阶段完整形态；主键用 `BIGINT`/`int64`（JSON `,string`），不用 UUID。若本节 schema 仍写 UUID，视为过时。
>
> 创建日期：2026-08-12

---

## 模块清单

| 模块 | 文档 | 核心职责 | 旧系统借鉴 |
|------|------|---------|-----------|
| 认证 | [auth.md](./auth.md) | 登录、双 Token、RT 轮换、登出、多设备 | authenticator.md + LoginLocker |
| 用户 | [user.md](./user.md) | 用户 CRUD、密码管理、角色绑定 | user.md + PasswordValidator |
| 角色 | [role.md](./role.md) | 角色 CRUD、菜单分配、Casbin 策略同步 | role.md + RolePresetProvider |
| 组织 | [organization.md](./organization.md) | 组织树、成员管理、组织角色 | organization.md + hierarchy |
| 菜单 | [menu.md](./menu.md) | 菜单树、菜单-API 绑定、前端权限 | menu.md + syncSystemMenus |
| 鉴权 | [authz.md](./authz.md) | 路由级 Casbin + 资源级 ResourceRegistry | authorizor.md + restrict.md + resource.md |
| 审计日志 | [audit.md](./audit.md) | 访问日志、操作审计、异步写入 | accesslog.md |
| 中间件 | [middleware.md](./middleware.md) | JWT、Casbin、CORS、限流、安全头 | middleware.md |
| 工单 | [ticket.md](./ticket.md) | IT 工单、状态机、三层鉴权、Scope 可见性 | - |

---

## 文档规范

每个模块文档包含以下章节：

1. **模块定位**：职责边界、与其他模块的关系
2. **数据模型**：表结构、索引、关系
3. **接口定义**：Service 接口方法
4. **核心流程**：关键业务流程的时序/流程图
5. **旧系统借鉴**：哪些设计直接采用，哪些修改，哪些不用
6. **分阶段实施**：Phase 1/2/3 各实现什么

---

## 模块依赖关系

```
auth ──────▶ user ──────▶ role ──────▶ menu
  │            │            │            │
  │            ▼            ▼            ▼
  │         organization  casbin_rule  menu_apis
  │            │
  ▼            ▼
middleware ──▶ authz (ResourceRegistry)
  │            │
  ▼            ▼
audit        ticket ← 第一个业务资源模块
```

- `auth` 依赖 `user`（验证密码）和 `middleware`（JWT 中间件）
- `user` 依赖 `role`（角色绑定）和 `organization`（组织归属）
- `role` 依赖 `menu`（菜单分配）和 `casbin_rule`（策略同步）
- `authz` 是横切模块，依赖 `ResourceRegistry`（各 Service 自注册）
- `audit` 是横切模块，中间件层记录
