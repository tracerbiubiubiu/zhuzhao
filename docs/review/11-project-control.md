# 11 · 项目能力总览图（Project Control）

> **用途**：一张地图快速掌握整个项目的能力、关键细节与当前健康状态，用于对 AI 快速迭代保持掌控。**每次代码改动后应同步更新本文**（见 `AGENTS.md`）。
>
> 更新日期：2026-08-31 ｜ 分支：`feature/phase-2` ｜ 文档体系见 [docs/roadmap.md](../roadmap.md)

---

## 0. 项目一句话

Go 编写的**模块化单体 IAM + 工单系统**：三层鉴权（路由 RBAC → 资源级 ltree scope → 属主/委托）+ 资源级工单业务，单实例 Docker Compose（PG + Redis）部署，文档驱动开发（docs/ 与代码强同步）。

**技术栈**：Go + Gin + pgx + Casbin + Wire DI + Redis + PostgreSQL（ltree）+ Docker Compose。

**规模**：70 个非测试 Go 源文件 ｜ 46 个测试文件 ｜ 188 个测试函数 ｜ 32 个迁移文件（16 对）｜ 13 份 review 文档（含本文件；统计时点 2026-08-31，随代码漂移）。

---

## 1. 模块 × 能力 × 状态矩阵（核心）

| 模块 | 能力 | 状态 | 关键代码位置 |
|------|------|------|-------------|
| **认证** | 登录、双 Token（AT/RT）、RT 轮换、登出、黑名单、登录限流、会话吊销、改密 | ✅ Phase 1 | `internal/service/auth_service.go` `session_revoke.go` |
| **用户** | CRUD、启用/禁用、密码修改/重置、角色绑定、组织绑定、超管保护 | ✅ Phase 1 | `internal/service/user_service.go` |
| **角色** | CRUD、菜单分配、Casbin 策略同步、角色优先级（防提权） | ✅ Phase 1 | `internal/service/rbac_service.go` |
| **组织** | 树形 CRUD、ltree 路径、move、成员管理、owner/成员角色、虚拟组、scope | ✅ Phase 1+2 | `internal/service/org_service.go` `org_delegation.go` |
| **菜单** | CRUD、菜单树、权限码、前端数据 | ✅ Phase 1 | `internal/service/menu_service.go` |
| **审计** | 操作日志中间件、同步写、登录审计、事件表 FK 去 CASCADE | ✅ Phase 1 + 2c（000014 去 CASCADE + deleted 事件） | `internal/middleware/audit.go` `internal/service/audit_service.go` |
| **鉴权 L1** | 路由级 Casbin RBAC（BFS 角色展开 + 超管通配） | ✅ Phase 1 | `internal/middleware/casbin.go` `internal/casbin/enforcer.go` |
| **鉴权 L2/L3** | 资源级鉴权：属主/assigned/scope/虚拟组/BFS 三源/组织内委托 | ✅ Phase 2 | `internal/service/ticket/resource.go` `scope_resolver.go` |
| **工单** | CRUD、状态机（open/assigned/in_progress/pending_verify/closed/rejected）、分派、评论/备注、关联、类型/模板、可见性 | ✅ Phase 2 | `internal/service/ticket/service.go` `state_machine.go` |
| **工单模板/关联** | ticket_templates（org_path ltree）、ticket_relations | ✅ Phase 2a（迁移 000015/000016） | `migrations/000015*` `000016*` |
| **基础设施** | Wire DI、配置、优雅关闭、健康检查、迁移、限流、安全头 | ✅ Phase 1 | `internal/app/` `internal/pkg/` |

### 未实现 / 延后（明确不做）
- **附件**（file_objects/ticket_attachments）— 2b-ext 延后，迁移编号规划 000017（**与 Phase 3 SLA 编号冲突，待决策**，见下）
- **Phase 3 全部**：可观测性 / 多实例 / 审计 L2 / 高可用 / 安全增强 / 平台增强 — 暂缓，设计就绪（见 `docs/phase3/`）
- **微服务拆分 / gRPC / CQRS / RS256 / AK-SK** — 明确不做（无需求）

---

## 2. 权限模型速查

### 三层鉴权（从粗到细）

| 层 | 机制 | 说明 |
|----|------|------|
| **L1 路由级** | Casbin RBAC | 中间件 BFS 展开角色 → `role::superadmin` / `role::admin` 通配所有；否则匹配 `(role, path, method)` |
| **L2 资源级** | ltree scope | 工单 `org_path <@ ANY(锚点)`（包含于用户 scope 锚点集，锚点经组织表实时解析）；虚拟组/兄弟读写分离 |
| **L3 属主级** | 属主/委托 | created_by / assigned_to 直判；org owner / org_member_role=admin 组织内委托；组内防提权 |

### 权限码清单（Casbin seed 管理类）

`user:create/update/delete/status/assign_role/assign_org/reset_password` ｜ `role:create/update/delete/assign_menu` ｜ `org:create/update/delete/move/member` ｜ `menu:create/update/delete`

> 工单操作不走静态权限码，走 **L2/L3 资源级动态判定**（create/read/update/close/assign/delete/comment/note）。

---

## 3. API 地图（`/api/v1`）

| 分组 | 端点 | 说明 |
|------|------|------|
| `/auth` | login / refresh | 公开 |
| 自助（登录后） | logout、password/update、user/profile、user/menus、user/permissions | 本人 |
| `/orgs`（委托） | delete、members、members/role、owners、members/delete | org admin/owner 操作 |
| `/users` | CRUD + status + roles + password/reset + orgs | 管理端 |
| `/roles` | CRUD + menus + permissions | 管理端 |
| `/orgs` | 树 CRUD + move + members | 管理端 |
| `/menus` | CRUD | 管理端 |
| `/audit/logs` | 审计日志查询 | 管理端 |
| `/tickets` | CRUD + close + assign + comments + notes + relations | 工单 |
| `/ticket-types` `/ticket-templates` | 元数据（类型/字段/模板） | 只读 |

健康检查：`/health/live` `/health/ready`（Phase 1 起）。

---

## 4. 数据库迁移地图（16 对）

| 迁移 | 用途 | 阶段 |
|------|------|------|
| 000001 | 初始化：用户/角色/组织/菜单/审计等基础表 | P1 |
| 000002 | seed 初始数据 | P1 |
| 000003-000005 | Casbin 列调整 | P1 |
| 000006 | 唯一索引部分化（users/menus 等，软删友好） | P1 |
| 000007 | seed admin/MCP | P1 |
| 000008 | user_orgs 主键修正 | P1 |
| 000009 | Phase 1 硬化 | P1 |
| 000010 | **ticket 工单表** | 2a |
| 000011 | ticket_visibility 可见性 | 2b |
| 000012 | org_enhance 虚拟组/scope | 2b |
| 000013 | org_delegation owner/成员角色 | 2c |
| 000014 | 审计事件 FK 去 CASCADE | 2c |
| 000015 | ticket_templates | 2a |
| 000016 | ticket_relations | 2a |

> **编号冲突提示**：附件规划占 `000017`（phase2/README §2.4），但 Phase 3 SLA 文档（10-ticket-business §9）也用 `000017`——**启动 Phase 3 前需决策**（见 `docs/review/11` 与 Phase 3 评估）。

---

## 5. 测试与门禁体系（你的"一键验证"）

| 门禁 | 命令 | 覆盖 |
|------|------|------|
| 单元测试 | `make test-unit` | 182 个测试函数 |
| 集成测试 | `make test-integration` | 需 PG/Redis |
| 覆盖率 | `make test-cover` | 覆盖率报告 |
| **全量验收** | `make acceptance` | **四档链式门禁**（经 2c 脚本）：phase1（27 用例）→ 2a（R3-R8+T1-T7）→ 2b（R9-R10 虚拟组/scope/BFS）→ 2c（D1-D9 委托） |
| 分档入口 | `make acceptance-2a` / `-2b` / `-2c` | 单档运行（-2c 含全部上游回归） |
| 其他 | `make lint` `build` `swag` `benchmark` | 静态检查/构建/文档 |

**用法建议**：AI 迭代完成后，你只跑 `make acceptance` 看绿灯 + `make test-cover` 看覆盖率趋势，即可验收，无需读 diff。

---

## 6. 当前健康状态（遗留问题追踪）

> 状态基准：2026-08-29 静态验证；权威清单见 `docs/review/10-phase2-comprehensive-verification.md`。

| 编号 | 问题 | 状态 |
|------|------|------|
| CC1 | Close TOCTOU（UpdateStatusTx 无条件） | ✅ **已修复**（`WHERE id AND status=$2`） |
| CC2 | Assign TOCTOU（UpdateAssignedToTx 无条件） | ✅ **已修复**（`AND status<>'closed'`） |
| CC3 | DeleteOrgDelegated 非原子 | ✅ **已修复**（`DeleteVgWithOwnerCleanup` 单事务） |
| HC3 | CreateTx 未映射 FK 违规（raw 500） | ✅ **已修复**（`MapForeignKeyViolation`） |
| MC2 | Org Move 未级联 ticket_templates.org_path | ✅ **已修复**（Move 含 templates 级联） |
| MC3 | IsAncestorOwner 未用 ticketOrgPath | ✅ **已修复** |
| P0 | RemoveMember 不清理 owner_user_ids | ✅ **已修复**（owner 三处同步清理） |
| **HC1** | Comment/Note 不写 ticket_events（无审计事件） | ❌ **未修** |
| **TC1** | delete 成功路径断言 | ✅ **已补**（2c 脚本 TC1 建删删 404 + `TestD9` owner/pOwner 删单 200） |
| **TC2** | 缺 relation 集成测试 | ✅ **已补**（`TestD9_CreateRelation`：正向 / 同向 409 / 删后建联 400） |
| HC2 | Delete 无 "deleted" 事件 | ✅ **已修复**（000014 SET NULL + Delete 同事务写 deleted 事件，随库存活；回归断言通过） |
| EC1 | Swagger 未重新生成 | ✅ **已修复**（orgs/owners 等 2c 端点已入 docs.go/swagger.json） |
| **OP1** | org_path 快照竞态（Create×Move 并发 → 旧 path 快照） | 🟡 已确认、修复方案已定待实施（**已登记 BK-11**，见 00 §9）；Create 事务内 `FOR SHARE` 锁 org 行后读 path（与 Move 写锁串行化）；影响为 fail-safe 降级，无越权无损坏 |

---

## 7. 文档导航（docs/ 体系）

```
docs/
├── design/        # 为什么这样设计（决策与权衡）
├── proposal/      # 具体方案是什么
├── modules/       # 模块完整设计（跨阶段）
├── phase1/2/3/    # 每阶段实施计划（phase3 暂缓，设计就绪）
├── roadmap.md     # 三阶段总览
├── adr/           # 架构决策（001 L1 事件 / 002 Asynq / 003 activelist）
└── review/        # 验证报告（本文件 = 能力总览，01-10 = 历史 review）
```

**掌握流程**：遇到能力问题 → 查本文件矩阵定位模块 → 按需深入 `modules/` 对应文档 → 变更后回填本文件。

---

## 8. 下一步（规划中的 Phase 3，暂缓）

- **依赖空洞**：工单业务依赖多实例文档（02-multi-instance），未编写
- **迁移编号冲突**：附件（000017）vs SLA（000017）
- **权限码 seed**：ticket:approve / notification:* / workflow:manage 均未设计
- 完整评估见对话记录（Phase 3 计划评估：P1-P5 阻塞项 + S1-S10 缺口 + C1-C4 一致性）
