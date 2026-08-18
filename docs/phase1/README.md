# Phase 1 实现计划：认证鉴权框架

> **核心目标**：为后续所有服务搭好认证鉴权框架。Phase 1 不做业务模块（工单等），聚焦于认证、鉴权、基础管理。
>
> 创建日期：2026-08-12

**编码前拍板**：路由 id 位置、无角色错误码等与骨架差异见 [00-pre-coding-decisions.md](./00-pre-coding-decisions.md)。  
**HTTP 响应体**：后端 SSOT 见 [../api/response.md](../api/response.md)。  
**部署与代码解耦**：全阶段原则见 [design-decisions §18](../design/design-decisions.md#18-部署与代码解耦一套代码多种部署)（一套代码，配置驱动多种部署；Phase 1 仅表示「先不实现 Watcher/多副本横切」，不是写死单节点）。

---

## 1. Phase 1 边界

### 1.1 做什么

| 类别 | 模块 | 核心能力 |
|------|------|---------|
| 基础设施 | [infra](./01-infra.md) | DB 迁移、配置加载、Wire DI、优雅关闭、健康检查 |
| 认证 | [auth](./02-auth.md) | 登录、双 Token、RT 轮换、登出、黑名单、登录限流、会话吊销 |
| 鉴权 | [authz](./03-authz.md) | 路由级 Casbin RBAC、ResourceRegistry 空接口 |
| 用户 | [user](./04-user.md) | 用户 CRUD、启用禁用、密码修改、超管保护 |
| 角色 | [role](./05-role.md) | 角色 CRUD、菜单分配、Casbin 策略同步 |
| 组织 | [organization](./06-organization.md) | 组织树 CRUD、ltree 路径、用户-组织关联 |
| 菜单 | [menu](./07-menu.md) | 菜单 CRUD、菜单树、前端权限数据、**权限码 SSOT**（各模块 API 表引用） |
| 审计日志 | [audit](./08-audit.md) | 操作日志中间件、同步写入、登录审计 |
| 中间件 | [middleware](./09-middleware.md) | JWT（fail-close）、Casbin、CORS、Recovery、RequestID、安全头 |
| 并发与事务 | [concurrency](./10-concurrency.md) | DB 事务、SyncedEnforcer、Redis 原子操作、乐观锁 |

### 1.2 不做什么

| 不做 | 原因 | 阶段 |
|------|------|------|
| 工单模块 | Phase 1 聚焦框架 | Phase 2 |
| 数据范围过滤 | 管理接口按「全局管理员」模型，不做组织范围过滤 | Phase 2 |
| 虚拟组 / 组织级权限 | Phase 1 只做实体组织树 CRUD | Phase 2b |
| 资源级鉴权完整实现 | Phase 1 只搭 ResourceRegistry 空接口，不实现 ltree 查询 | Phase 2 |
| 多设备管理 UI / 踢出 | 允许多设备登录，不提供设备列表 | Phase 2 |
| 密码复杂度策略 | 基础 bcrypt 即可 | Phase 2 |
| AK/SK | 无服务间调用方，不建表、不写中间件 | Phase 3b / 按需 |
| 文件存储 / 头像上传 | 无附件；`avatar` 仅 DB 存 URL 字符串 | Phase 2b storage |
| 事件驱动 / Asynq / Outbox | 无异步业务 | Phase 3 |
| 审计日志异步（channel / Redis List） | Phase 1 同步写入，保证不丢 | Phase 3a（Redis List L2；见 architecture §12.4） |
| 缓存体系 | Phase 1 无缓存，走 Casbin PG adapter | 工单跑通后按需 |
| JWT RS256 / JWKS | 单体无收益，拆服务时再换 | Phase 3 |
| 每资源独立 Enforcer | Phase 1 仅单全局 Enforcer | Phase 2+ / 策略需可配置时 |
| IAM 独立部署 / gRPC | Phase 1–2 模块化单体 | Phase 3 |
| 多实例 / 分布式锁 / Watcher | Phase 1 单实例 | Phase 3 |
| Metrics / 分布式追踪 | 可观测性 | Phase 3 |
| 多租户 | 预留 tenant_id 字段，不实现 | 按需 |

### 1.3 验收标准

**Step 11 全量验收**以本节为准。分阶段里程碑见 [§2.3](#23-里程碑验收推荐按此推进)。部分用例需到指定里程碑才能测，勿在过早 Step 误判失败。

Phase 1 完成后，以下流程能跑通：

```
主路径（Step 11 全量）
1. make docker-up / migrate-up / make dev
2. POST /auth/login              # employee_no + password，拿到双 Token
3. GET  /user/menus              # 菜单树（M4 起可测，见 §2.3）
4. GET  /user/permissions        # 权限码（M4 起可测）
5. GET  /users                   # 需路由级权限
6. POST /auth/refresh            # 轮换 Token
7. POST /auth/logout             # 登出
8. GET  /users                   # 登出后 401

对抗路径（必须覆盖，不是可选）
9.  错误密码 / 不存在用户          # 均返回同一文案，防枚举
10. 连续登录失败                   # 触发限流 429
11. 禁用用户后带旧 AT 访问         # 403 + 30003（会话吊销 · AT 路径）
11b.禁用用户后用旧 RT refresh      # 401 + 20004，不得返回新 AT/RT（会话吊销 · RT 路径）
12. 删除/禁用最后一个 superadmin   # 拒绝
13. admin 重置 superadmin 密码     # 403
14. 首次登录改密期间访问其它 API   # 403 + 20007
14b.mcp 期间 GET /user/menus       # 403 + 20007（JWT 层，非 Casbin）
15. 无角色用户访问鉴权路由         # 403 + 70003
15b.无角色用户 GET /user/menus     # 403 + 70003（自服务白名单不豁免零角色）
16. 并发两次 refresh               # 只有一次成功
17. Redis 不可用时访问鉴权路由     # 503 + 10008（fail-close）
18. 添加组织成员                   # POST /orgs/members → 200（M5）
19. 移除未加入组织的成员           # POST /orgs/members/delete → 404 + 50007（M5）
20. 用户侧分配组织                 # POST /users/orgs 全量覆盖 → 200（M5）
21. admin 给用户分配 superadmin 角色 # 403 + 30009（priority 防提权）
22. Bearer + X-AK 混用（预留）     # 400 + 20008，Abort
23. 按 username 模糊查用户         # GET /users?username=zhang → total 可 >1
24. 按 employee_no 精确查用户       # GET /users?employee_no=E… → total 为 0 或 1

自服务与 RBAC 区分（M3 起部分可测，M5 完整）
25. viewer（零 menu）GET /user/menus     # 200 + menus=[]（自服务白名单）
26. viewer GET /users                    # 403 + 70001（业务 API，无 p 策略）
27. admin 给 viewer 勾用户管理 menu 后   # GET /users → 200；未勾 POST → 仍 70001
```

### 1.4 已知限制（验收时不要误判为已实现）

| 限制 | 说明 |
|------|------|
| 无数据范围过滤 | 拥有 `GET /users` 权限的角色能看到**全部**用户，不做部门隔离 |
| 无虚拟组 | 只有实体组织树 |
| AT 存哪 | 前端自行存 AT/RT（建议内存 + 刷新），后端不设 Cookie |
| AK/SK 不可用 | 没有服务间认证 |
| 管理接口是全局模型 | `admin`/`superadmin` 路由级 bypass；`operator`/`viewer` 靠菜单策略，仍无行级过滤 |
| superadmin 对 admin 不可见 | 角色/用户列表过滤 superadmin；admin 眼中最高档为 **admin**（见 [05-role §影子超管](./05-role.md#影子超管superadmin-对-admin-不可见)） |
| 复杂继承 / org_roles / BFS / 数据 scope | **Phase 1 不做**；业界对照与级联矩阵已记录在 [rbac-inheritance-and-cascade.md](../design/rbac-inheritance-and-cascade.md)，Phase 2b+ 再实现 |
| 组织负责人 + 组内 admin/member | **Phase 2c**（非 2b）；依赖虚拟组+scope 后交付（见 [phase2/04-org-delegation.md](../phase2/04-org-delegation.md)） |

---

## 2. 实施顺序

### Git 分支策略

> **`docs/phase1/` 是文档目录，不是 Git 分支。** 已废弃长期 `phase1` Git 分支；编码按短 feature 分支从 `dev` 切出。

| 角色 | 分支 | 说明 |
|------|------|------|
| 集成主线 + 文档 SSOT | **`dev`** | 文档修复与代码最终合入点 |
| Phase 1 实现 | **`feature/step-N-简短描述`** | 从最新 `dev` 切出；PR base = `dev`；合入后删除 |

**开分支示例**（Step 1）：

```bash
git checkout dev && git pull origin dev
git checkout -b feature/step-1-infra
# ... 实现 Step 1 ...
git push -u origin feature/step-1-infra
# gh pr create --base dev
```

文档有更新时，在 feature 分支上 `git merge origin/dev`（或 `git rebase origin/dev`）后再继续编码。

### 2.1 主线（Step 1→5，必须串行）

AuthN / AuthZ 基座必须按序完成；**Step 4 与 Step 5 分工**见 [09-middleware](./09-middleware.md#step-4-vs-step-5-分工) 与 [03-authz](./03-authz.md)。

```
Step 1: infra（DB 迁移 + 种子 + 配置 + Wire + 路由骨架含 org 4 条）
   │
   ├── Step 2: user repo（用户数据访问 + RoleFetcher 所需 user_roles 查询）
   │      │
   │      └── Step 3: auth（login / refresh / AuthService；logout 逻辑，Handler 在 Step 4 挂载）
   │             │
   │             └── Step 4: middleware · JWT（黑名单 / mcp / 20008 / fail-close）
   │                    │
   │                    └── Step 5: authz（PG Casbin + isSelfServiceRoute + RoleFetcher + Registry 空壳）
```

| Step | 模块 | 依赖 | 文档 | 交付要点 |
|------|------|------|------|----------|
| 1 | infra | 无 | [01-infra.md](./01-infra.md) | migrations、种子 25 菜单、LoginLocker Lua、**router 注册 org 4 条** |
| 2 | user repo | 1 | [04-user.md](./04-user.md) | UserRepo；`FetchRoleCodes` 可在此或 Step 5 |
| 3 | auth | 2 | [02-auth.md](./02-auth.md) | 登录/刷新/登出 **Service**；公开路由可测 |
| 4 | middleware · JWT | 3 | [09-middleware.md](./09-middleware.md) | **仅 JWT 链**；Casbin 可先 stub 放行 |
| 5 | authz | 4 | [03-authz.md](./03-authz.md) | PG adapter、Casbin 中间件、自服务白名单、Registry |

**横切**：[10-concurrency.md](./10-concurrency.md) 贯穿 Step 5+（AssignMenus 事务、`ReloadPolicy` 在 commit 后）。

### 2.2 Step 5 之后（可并行）

Step 6–10 **无严格线性顺序**，按 [§2.3 里程碑](#23-里程碑验收推荐按此推进) 推荐组合。依赖关系：

```
                    Step 5（authz 就绪）
                           │
         ┌─────────────────┼─────────────────┬──────────────┐
         ▼                 ▼                 ▼              ▼
    Step 7 role       Step 8 menu       Step 9 org     Step 10 audit
    AssignMenus       CRUD +           OrgService +    （仅需 Step 4）
                      GetMenus/         user_orgs
                      GetPermissions
         │                 │                 │
         └────────┬────────┘                 │
                  ▼                          │
            Step 6 user                      │
            （见 04-user 分期）◄─────────────┘
                  │
                  └── Step 11 集成验收（§1.3 全量）
```

| Step | 模块 | 硬依赖 | 文档 | 说明 |
|------|------|--------|------|------|
| 6 | user service/handler | 5；**组织绑定另需 9** | [04-user.md](./04-user.md) | 6a：CRUD/profile/roles；6b：`org_ids`、`POST /users/orgs` 在 Step 9 后 |
| 7 | role | 5 | [05-role.md](./05-role.md) | 可与 8、9 **并行**；AssignMenus 依赖种子 menu_apis（Step 1） |
| 8 | menu | 5；**读侧可与 7 并行** | [07-menu.md](./07-menu.md) | `GET /user/menus` 在 **Step 8** 交付；不阻塞 Step 7 |
| 9 | organization | 5 | [06-organization.md](./06-organization.md) | 可与 7、8 **并行**；**应先于** Step 6b |
| 10 | audit | 4 | [08-audit.md](./08-audit.md) | 可与 6–9 **并行**；Step 11 前挂载即可 |
| 11 | 集成验收 | 6a+6b+7+8+9+10 | 本文 §1.3 | 全量对抗路径 |

> **旧版线性顺序**（6→7→8→9）易误导：验收 #3/#4 在 Step 8 末才可测；#18–#20 需 Step 9。以本节与 §2.3 为准。

### 2.3 里程碑验收（推荐按此推进）

> **关键原则**：每个里程碑只列 **该里程碑新增可测** 的用例。用例编号见 [§1.3](#13-验收标准)。部分用例需手动设置 Redis key / DB flag 才能在早期里程碑测（标注 `*`）。

| 里程碑 | 完成 Step | 新增可验证 | 说明 |
|--------|-----------|--------|------|
| **M1** 基座 | 1 | #1 | `migrate-up`、种子 25 菜单、ready 探针、router 含 org 4 条 |
| **M2** 能登录 | 2–4 | #2、#6–#10、#14\*、#14b\*、#16、#17、#22 | JWT 链可用；#14/#14b 需手动设 `must_change_password=true`（完整 admin 重置流程在 M5）；#14b 在 JWT 层拦截，不依赖 `/user/menus` handler 实现；#7/#8 需临时 stub CasbinAuth 全放行（见 [09-middleware §Step 4 vs Step 5 分工](./09-middleware.md)），仅用于 JWT 联调，M3 起换真实 Enforce |
| **M3** 能鉴权 | 5 | #15、#15b、#26 | Casbin 链可用；deny 路径（70001/70003）在中间件层返回，**不依赖** handler 实现；admin bypass 可验证「未 403」（handler 仍 stub 可能 500） |
| **M4** 能进前端 | 7+8 | #3、#4、#25 | `GetMenus`/`GetPermissions` 返回数据；`AssignMenus` + 种子 menu_apis 联调；#25 viewer 零 menu → `menus=[]` |
| **M5** 业务闭环 | 9+6a+6b | #5、#11、#11b、#12、#13、#14、#18–#21、#23–#24、#27 | 用户 CRUD + 组织 + priority 防提权 + mcp 完整流程；`OrgService` 注入 `UserService` |
| **M6** 可审计 | 10 | 审计中间件 | 操作日志写入；登录审计（Step 3 已有） |
| **M7** 全量 | 11 | §1.3 全量回归 | 含跨模块边界、级联、并发等 |

**推荐实施批次**（减少返工）：

1. **批次 A**：Step 1 → 2 → 3 → 4 → 5（→ **M3**）  
2. **批次 B**（并行）：Step 7 + Step 8 + Step 9（→ **M4** + 组织写路径就绪）  
3. **批次 C**：Step 6a（用户 CRUD，不含 org）→ Step 6b（接 OrgService）→ Step 10（→ **M5–M6**）  
4. **批次 D**：Step 11 全量回归（→ **M7**）

> **批次 B 说明**：Step 7/8/9 互相无依赖，可并行开发；Step 9 的 `OrgService` 是 Step 6b 的前置。若人手有限，优先 **9 → 6b**（组织闭环），再 **7+8**（前端菜单）。

---

## 3. 测试策略

### 3.1 核心原则：测试先行

每个模块的实现必须遵循"先写测试，再写实现"的节奏：

1. **接口定义先行** — 先定义 Service/Repository 接口方法签名
2. **测试用例先行** — 根据接口契约编写测试用例（含正常、边界、异常场景）
3. **实现代码** — 编写实现使测试通过
4. **重构** — 测试通过后重构代码，确保测试仍然通过

### 3.2 测试分层

| 层级 | 范围 | 工具 | 运行 |
|------|------|------|------|
| 单元测试 | Service 层（Mock Repository） | testing + testify + uber-go/mock | `make test-unit` |
| 集成测试 | Repository 层（testcontainers PG） | testcontainers-go + pgx | `make test-integration` |
| 单元测试 | Middleware（Mock JWT + Redis） | httptest + testify | `make test-unit` |
| 集成测试 | 端到端 API | httptest + testcontainers | CI |
| 基准测试 | 关键路径（JWT 解析、Casbin Enforce） | testing.B | 按需 |

### 3.3 Mock 策略

- **Service 层**：Mock Repository 接口（uber-go/mock 生成）
- **Handler 层**：Mock Service 接口 + 真实 Gin 路由（httptest）
- **Repository 层**：不 Mock，用 testcontainers 启动真实 PG
- **Middleware**：Mock JWT Manager + Redis Client

---

## 4. 待决策点

### 4.1 已确认（2026-08-17）

| # | 决策 |
|---|------|
| 1 | **自服务路由**：Casbin 中间件 **白名单**（方案 A），不进 `menu_apis`；见 [authz §2.2.1](../modules/authz.md#221-自服务路由业界做法-本项目决策) |
| 2 | **Create 成功 HTTP**：统一 **200** + `code: 0`（与 [response.md](../api/response.md) 一致） |
| 3 | **种子 role_menus**：`operator`/`viewer` **空**；`superadmin`/`admin` 绑定全部 IAM 菜单（必含用户/角色/组织；含菜单管理） |

> 其余骨架/文档不一致项见 [00-pre-coding-decisions §决策 3](./00-pre-coding-decisions.md#决策-3附带与上两项同批对齐的骨架项)。

### 4.2 历史已确认

| 事项 | 决策 | 状态 |
|------|------|------|
| 用户 ID 类型 | `BIGINT`/`int64`，JSON 加 `,string` tag | ✅ 已确认 |
| 组织 ID / 编码 | ID 为 `BIGINT`/`int64`（JSON `,string`）；业务编码 `code` 为 `VARCHAR`（ltree 路径用 code，只能字母数字下划线） | ✅ 已确认 |
| Casbin adapter | 直接上 PG adapter（`noho-digital/casbin-pgx-adapter`，Casbin v3） | ✅ 已确认 |
| 密码策略 | 仅 bcrypt cost=12，不增加复杂度校验 | ✅ 已确认 |
| 组织模块范围 | Phase 1 实现完整 CRUD | ✅ 已确认 |
| 审计日志写入方式 | Phase 1 同步写入（见下方分析） | ✅ 已确认 |
| 应用日志 | slog 同步写入文件，不异步（见下方分析） | ✅ 已确认 |
| ID 自增策略 | 用户/角色/菜单/日志/凭证 BIGSERIAL；组织 BIGSERIAL + 种子显式 ID | ✅ 已确认 |
| org_type 枚举 | 1=公司 2=部门 3=小组 4=虚拟组 | ✅ 已确认 |
| tenant_id | BIGINT DEFAULT 1，Phase 1 不过滤 | ✅ 已确认 |
| 数据访问层 | 每实体独立 Repository 接口 | ✅ 已确认 |
| superadmin 角色 | 新增，4 个系统角色 | ✅ 已确认 |
| 登录限流 | Phase 1 用 Redis **Lua**（INCR+EXPIRE 原子，15min/5 次） | ✅ 已确认 |
| 会话吊销 | 禁用/删除用户写 `user:disabled:{id}`，JWT 中间件检查 | ✅ 已确认 |
| Redis 故障 | 鉴权链路 fail-close，返回 503 | ✅ 已确认 |
| AuthN 非法/混用凭证 | 互斥、Abort、400/20008；SSOT [02-auth §非法认证](./02-auth.md#非法认证请求的处理实现必读) | ✅ 已确认 |
| 领域目录 | 新代码 `internal/{layer}/{domain}/`；跨域经 Service 接口 | ✅ 已确认（[architecture §3.5](../design/architecture.md#35-领域模块目录约定单仓可拆分)） |
| AK/SK | Phase 1 不做 | ✅ 已确认 |
| 数据范围 | Phase 1 不做组织范围过滤 | ✅ 已确认 |
| 错误码 `20008`/`30006`/`70003` | Phase 1 验收必需；Step 4 写入 `errcode.go`（见 [errcode.md](../api/errcode.md)） | 📋 实现项 |
| CORS | Phase 1 用 `gin-contrib/cors` **DefaultConfig + AllowAllOrigins**（全 Origin 放开，不做白名单）；生产上线前再改域名白名单 | ✅ 已确认 |
| 存量 qingtao/aksk | **仅自研 Canonical**（`X-AK-*`）；**不**双栈、不长期并存；存量调用方迁移到新 SDK | ✅ 已确认（[02-auth §存量迁移](./02-auth.md#已决策存量-qingtaoaksk-迁移策略)） |
| 部署与代码解耦 | 一套代码多种部署；拓扑只改配置/编排，不改业务逻辑；多副本横切 Phase 3 按配置启用 | ✅ 已确认（[design-decisions §18](../design/design-decisions.md#18-部署与代码解耦一套代码多种部署)） |

---

## 5. 文档索引

| 文档 | 模块 | 核心内容 |
|------|------|---------|
| [01-infra.md](./01-infra.md) | 基础设施 | DB 迁移、配置、Wire、优雅关闭、**领域目录约定** |
| [design/rbac-inheritance-and-cascade.md](../design/rbac-inheritance-and-cascade.md) | 横切 | **备忘**（Phase 1 不实现）：继承、业界对照、级联矩阵 |
| [02-auth.md](./02-auth.md) | 认证 | 登录、双 Token、RT 轮换、登出、**AuthN 拒绝原则** |
| [03-authz.md](./03-authz.md) | 鉴权 | Casbin RBAC、ResourceRegistry |
| [04-user.md](./04-user.md) | 用户 | CRUD、密码、角色绑定 |
| [05-role.md](./05-role.md) | 角色 | CRUD、菜单分配、策略同步 |
| [06-organization.md](./06-organization.md) | 组织 | 树形 CRUD、ltree、用户关联 |
| [07-menu.md](./07-menu.md) | 菜单 | CRUD、菜单树、权限码 |
| [08-audit.md](./08-audit.md) | 审计日志 | 操作日志中间件、同步写入、应用日志规划 |
| [09-middleware.md](./09-middleware.md) | 中间件 | JWT、Casbin、CORS、安全头 |
| [10-concurrency.md](./10-concurrency.md) | 并发与事务 | DB 事务、SyncedEnforcer、Redis 原子操作 |
