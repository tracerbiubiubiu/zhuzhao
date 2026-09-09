# 鉴权模块设计

> 模块代码：`internal/middleware/casbin.go` + `internal/pkg/resource/` + 各 Service 的 Resource 实现
>
> 旧系统参考：`doc/module-assessment-2026-08/authorizor.md` + `restrict.md` + `resource.md` + `interaction-auth-chain.md`
>
> **Phase 1 行为以 [phase1/03-authz.md](../phase1/03-authz.md) 为准**（直接角色、无 Redis 权限缓存、Registry 空接口）。

---

## 0. 权限文档地图（全域入口，2026-09-08 建）

> 权限相关文档分散在 design/modules/phase2/phase3/review 五处——**刻意不合并目录**（文件编号即索引、跨文档相对引用数百处，搬迁=断链；历史教训：曾因引用错文档版本引发误判）。以本节为唯一入口地图，按需取用。

### 0.1 按层分类

| 层 | 文档 | 内容 |
|---|------|------|
| **公约 SSOT**（跨生态约束） | [standards.md §7](../standards.md) | PDP/PEP 分工、三层定义、Q5 禁令、ReBAC 不演进、角色全链平台本职、权限码命名与声明式注册 |
| **决策 SSOT**（为什么这样定） | [design-decisions.md](../design/design-decisions.md) §12/§20/§25/§26 | §12 ReBAC 选型史、§20 SoD 延后、§25 权限架构定版（25.1 PDP/PEP、25.2 网关边界、25.3 策略库归属、25.4 ReBAC 不演进、25.5 前置清单）、§26 IAM 演进三步走/注册原则/支撑边界四墙 |
| **架构评审**（对照与触发表） | [phase2/11-authz-architecture-review.md](../phase2/11-authz-architecture-review.md) | §1.2 业界对照、§2 L2/L3 顺序拍板、§3 不变量（默认拒绝/fail-closed/Q5）、§5 ReBAC 转向触发表（六条）、§9 OPA/ReBAC 迁移复核（2026-09-08） |
| **模块详设**（怎么实现） | 本文 + [phase2/02-authz-resource.md](../phase2/02-authz-resource.md) + [phase2/04-org-delegation.md](../phase2/04-org-delegation.md) + [phase2/03-org-enhance.md](../phase2/03-org-enhance.md) | L1/L2/L3 详设、ScopeResolver/TicketResource、组织委托轴、组织可见性开关 |
| **执行与在册**（做到哪/欠什么） | [review/11-project-control.md](../review/11-project-control.md) §2/§6/§8 + [phase2/00 §9](../phase2/00-implementation-plan.md) + [phase3/00-startup-checklist.md](../phase3/00-startup-checklist.md) | 权限模型速查、健康状态（BK-19~22）、遗留分类、backlog 详情、启动检查单 |
| **Phase 3 权限配套** | [phase3/16-external-integration.md](../phase3/16-external-integration.md) §9 + [phase3/09-platform.md](../phase3/09-platform.md) + [phase3/02-multi-instance.md](../phase3/02-multi-instance.md) + [phase3/03-audit-l2.md](../phase3/03-audit-l2.md) + [phase3/13 §1](../phase3/13-implementation-plan.md) | 外部服务统一基线（AK/SK+断言）、批次 A/B 前置、L1 缓存、AK/SK 平台凭据、Casbin Watcher、判定日志/归档、主链排期 |
| **专项散节** | [ADR-003](../adr/ADR-003-activelist-integration-form.md)（activelist 零权限边界）、[phase1/05-role.md](../phase1/05-role.md)（角色 priority 与权限继承模型，§角色 priority 与权限继承模型）＋ [design/rbac-inheritance-and-cascade.md](../design/rbac-inheritance-and-cascade.md)（parent_id 继承与级联）、[phase1/02-auth.md](../phase1/02-auth.md)（认证详设）、[design/architecture.md §4](../design/architecture.md)（总体架构中的权限位） | 按需 |

### 0.2 按目的取用

| 你要做什么 | 读这里（顺序） |
|-----------|---------------|
| 5 分钟了解权限现状 | review/11 §2 权限模型速查 → standards §7 |
| 理解为什么这样设计 | design-decisions §25 → §26 → 11-authz §2/§3 |
| 改权限相关代码 | 本文 → phase2/02 → review/11 §6（先看在册债务避免重复踩） |
| 新服务接入（taskrunner/activelist 式） | 16 号 §9 基线 → ADR-003 → design-decisions §26.2（注册协议） |
| 判断「要不要引入 X / 能力该不该做」 | 11-authz §5 触发表 → §25.4 反触发评估 → §26.4 四墙 |
| 排查权限问题 | review/11 §2 速查 → §6 健康状态 → 11-authz §3 不变量（403/404/503 语义） |

### 0.3 引用口径警示

权限演进路径的引用 SSOT = **standards §7 + 11-authz §5 + design-decisions §12/§25/§26**。勿引：standards §11（是开发工作流）、ADR-001（是工单事件源）、review/11（是能力总览非设计源）。

---

## 1. 模块定位

**核心底座模块**。鉴权分为两层：

| 层级 | 位置 | Phase 1 | Phase 2+ |
|------|------|---------|----------|
| 路由级 RBAC | Casbin **中间件** | ✅ 直接角色 enforce | BFS 三源 + 可选缓存 |
| 资源级 | **Service** + ResourceRegistry | 空接口 | Authorize + GetFilter |

与其他模块的关系：
- `middleware/casbin.go`：路由级鉴权（查 `casbin_rule` + `user_roles`）
- 各 Service 在 Phase 2 自注册 Resource 到 ResourceRegistry
- Handler 调用 `registry.Authorize`（**不在 middleware**，无 `resource_authz.go`）

---

## 2. 路由级鉴权（Casbin RBAC）

### 2.1 Casbin 模型（g 表消除）

```ini
# configs/casbin_model.conf
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == "role::superadmin" || \
    r.sub == "role::admin" || \
    (r.sub == p.sub && (p.obj == "*" || keyMatch2(r.obj, p.obj)) && (r.act == p.act || p.act == "*"))
```

无 `[role_definition] g` 段。用户→角色在 `user_roles` 表，中间件 subject 为 `role::{code}`。

### 2.2 Phase 1 中间件流程（直接角色）

```go
func CasbinMiddleware(enforcer *casbin.SyncedEnforcer, fetcher RoleFetcher) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := c.GetInt64("userID")

        roles, err := fetcher.FetchRoleCodes(c.Request.Context(), userID)
        if err != nil {
            response.InternalError(c, errcode.ErrInternal)
            c.Abort()
            return
        }
        if len(roles) == 0 {
            response.Forbidden(c, errcode.ErrNoRoles)
            c.Abort()
            return
        }

        path := c.Request.URL.Path
        method := c.Request.Method
        allowed := false
        for _, role := range roles {
            if enforcer.Enforce("role::"+role, path, method) {
                allowed = true
                break
            }
        }
        if !allowed {
            response.Forbidden(c, errcode.ErrNoPermission)
            c.Abort()
            return
        }

        c.Set("roles", roles)
        c.Next()
    }
}
```

- superadmin/admin bypass 在 **matcher** 中完成，不在 Go 代码里硬编码循环
- Phase 1 **无** Redis `perm:user:{userId}` 缓存

### 2.2.1 自服务路由（业界做法 + 本项目决策）

**业界常见做法**（IAM / 后台管理系统）可归纳为三类：

| 做法 | 代表 | 特点 |
|------|------|------|
| **A. 鉴权层白名单 / `authenticated-only`** | Spring Security `permitAll`/`authenticated` 分区；API Gateway 对 `/me`、`/logout` 单独路由规则；OAuth2 **UserInfo** 与业务 Resource 分离 | 自服务接口**不进**业务权限码表；「已登录 + 有身份」即可，与菜单/RBAC 解耦 |
| **B. 隐式权限码** | Keycloak Account API、部分 SaaS 的 `profile:read` 默认绑 every user | 仍走统一 PDP，但给所有角色预置「基础包」 |
| **C. 绑进菜单/RBAC** | 部分 RuoYi 衍生把 `getInfo`/`getRouters` 写死放行或挂虚拟菜单 | 模型统一，但菜单表掺非 UI 路由，新角色易漏绑 |

**主流倾向 A**：自服务（当前用户 profile、会话 logout、动态 menus/permissions）属于 **AuthN 之后的基础能力**，不应与「用户管理」「工单列表」等业务权限同一套菜单策略；否则 `operator`/`viewer` 零菜单时连登录后拉菜单都会 403。

**本项目 Phase 1 已采纳 A**：Casbin 中间件在 `len(roles)>0` 之后、逐角色 `Enforce` 之前，对**自服务路由**直接放行（仍要求 JWT 已通过、且非 `mcp` 拦截场景下的非法路径）。
**实现方案（B4-2 回写）**：`SelfService()` 中间件在**路由注册期**打 context 标记（`SelfServiceContextKey`），`CasbinAuth` 检测标记放行——**非** method+path 路径匹配（原设计已废弃：路径匹配可被路径构造绕过且与路由注册解耦；标签方案注册期生效、请求侧不可伪造，router_test.go 行为测试守护注册顺序）。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/user/profile` | 当前用户资料 |
| POST | `/api/v1/user/profile/update` | 更新资料 |
| GET | `/api/v1/user/menus` | 前端动态路由 |
| GET | `/api/v1/user/permissions` | 按钮权限码 |
| POST | `/api/v1/auth/logout` | 登出（`mcp=true` 强制改密期间 **禁止**，仅允许改密） |
| POST | `/api/v1/auth/password/update` | 改密（含 `mcp` 场景唯一允许的业务写操作） |

- **零角色**用户：白名单**不**生效，仍 **403 + 70003**（未分配角色不能进系统）。
- **不写入** `menu_apis` / 菜单树；权限码列标 `—`（见 [07-menu](../phase1/07-menu.md)）。
- 实现 SSOT：[phase1/09-middleware §Casbin](../phase1/09-middleware.md#casbin-中间件g-表消除) 伪代码。

### 2.3 admin 路由 bypass ≠ 业务无约束

> **常见误解**：matcher 里 `role::admin` 与 `role::superadmin` 一样「全通」，是否意味着 admin 能改 superadmin / 其他 admin？  
> **否**。Casbin 只回答 **「这个 HTTP 请求能不能进 Handler」**；**「能不能动这个具体对象」** 在 **Service 业务层** 用 `roles.priority` + 影子超管规则拦截。

**两层分工**：

| 层 | 位置 | 回答什么 | superadmin | admin |
|----|------|----------|------------|-------|
| **L1 路由级** | Casbin 中间件 | 能否调用 `POST /users/update` 这类 API？ | matcher bypass | matcher bypass（**等价**） |
| **L2 业务防提权** | 各 Service 方法内 | 能否改**这个**用户/角色/菜单？ | 几乎无限制（兜底规则除外） | **priority + is_system + 影子超管** |

**为何 admin 仍做 Casbin bypass？**

- Phase 1 管理端是「全局管理员」模型：admin 应能调全部 IAM API，不必为每个路由维护 `p` 策略。
- 细粒度「对谁动手」不适合塞进 Casbin 策略表（策略爆炸 + 无法表达「目标用户是谁」）。
- 与若依/RuoYi 同类：**路由权限** 与 **数据/对象级防提权** 分离。

**L2 拦截点（Phase 1 实现位置）**：

| 场景 | 拦截位置 | 规则 | 失败 |
|------|----------|------|------|
| 重置密码 | `UserService.ResetPassword` | `actorP < targetP`（严格更强）；影子超管用户对 admin **404** | 403 + 30005 |
| 分配角色 | `UserService.SetRoles` | `actorP < targetP`；待分配角色 `priority >= actorP`（不能分 superadmin） | 403 + 30009 |
| 改/删/禁用用户 | `UserService.Update/Delete/SetStatus` | `actorP < targetP`；`is_system` 保护；不能删最后 superadmin | 403 / 404 |
| 改角色菜单 | `RoleService.AssignMenus` | admin **不能**改 `superadmin` 角色的菜单 | 403 |
| 删改系统资源 | `Role/Menu/Org Service` | `is_system=true` 时 admin 拒绝 | 403 |
| 列表/详情「看不见」 | `UserRepo.List` / `RoleRepo.List` / `GetByID` | 非 superadmin：**过滤** superadmin 角色及绑定用户 | 列表不含 / 详情 **404** |

**priority 计算**（与 [04-user §多角色](../phase1/04-user.md#多角色与有效-priority) 一致）：

```go
// EffectivePriority = min(用户全部角色的 priority)；越小越强
func CanManageUser(actorP, targetP int) bool {
    return actorP < targetP // 严格更强才能管对方（含 admin 不能动 admin）
}
```

**示例**：

| 操作者 | 目标 | L1 Casbin | L2 业务 |
|--------|------|-----------|---------|
| admin | operator 用户 | ✅ 放行 | ✅ `10 < 20` |
| admin | 另一 admin 用户 | ✅ 放行 | ❌ `10 < 10` 不成立 → 403 |
| admin | superadmin 用户 | ✅ 放行 | ❌ 列表不可见；直调 id → **404**；若强行写则 `10 < 1` → 403 |
| superadmin | admin 用户 | ✅ | ✅ `1 < 10` |

> SSOT 对照：[05-role §superadmin 与 admin](../phase1/05-role.md#superadmin-与-admin-的区别)、[04-user §角色分级与系统保护](../phase1/04-user.md#角色分级与系统保护业务校验非-casbin)、[resource-model §场景 2](../proposal/resource-model.md)。

### 2.4 Phase 2b+：BFS 三源合并 + 可选缓存

Phase 2 扩展 `RoleFetcher`：

```go
func (f *roleFetcher) FetchRoleCodes(ctx context.Context, userID int64) ([]string, error) {
    // Phase 2: 源1 直接角色 + 源2 组织角色 + 源3 继承角色，去重
    // Phase 3: 可选 Redis perm:user:{userId} 缓存
}
```

---

## 3. 资源级鉴权（ResourceRegistry）

Phase 1 只定义 `Resource` / `Registry` 接口，不注册业务 Resource。

Phase 2 工单等模块在 Service 构造函数中 `registry.Register(...)`，在 Service 方法内调用：

```go
ok, err := s.registry.Authorize(ctx, "ticket", resource.AuthorizeRequest{
    UserID: uid,
    Roles:  roles,
    Action: "update",
    ResourceID: ticketID,
})
```

列表过滤：`registry.GetFilter(ctx, "ticket", uid, "read")` → SQL WHERE。

详见 [proposal/resource-model.md](../proposal/resource-model.md)。

### 3.1 平台内置策略库（声明式接入，M-E 前置；design-decisions §25.3）——✅ 已实现（2026-09-04，`internal/pkg/resource/builtin.go`：`Builtin(code, OrgMember(table)|OwnerOnly(table)|RoleGated())`；单条/端点判定经 `Context` 行属性 + 注入 `Membership` 闭包；schema fail-fast = `RequireSchema`；单测 + 真 PG 集成测试正负向齐）

> 2026-09-03 定稿。目标：普通模块接入 L2 从「手写 Resource 实现」降为「一行声明」；工单类复杂策略（三轴+委托）保持手写，两条路并存、永不合流（工单策略随数据属主走，冻结期原地封存，详见 design-decisions §25.3 补充拍板）。

**内置策略**（`internal/pkg/resource/builtin.go`，判定构件全部复用现有：BFS 展开 / user_orgs 成员查询 / IW4 Unscoped 语义）：

> **适用前提与状态（2026-09-04 校准）**：内置策略要求**资源表在 zhuzhao 库**（谓词在库内下推）——跨库资源（数据在 taskrunner/activelist 等独立库）不适用，其过滤走**参数级组装**（E-⑤ 部门可见性模式，属手写路）。预设消费方已随 M-E 实际落地清零（E-④=L1 权限码 / E-②=AK/SK 验签 / E-⑤=参数级），**策略库整体降为触发条件驱动**（design-decisions §25.5）：zhuzhao 自有新资源需要 L2 时实施，设计保留有效。

| 策略 | 行级语义（GetFilter 谓词） | 单条语义（Authorize） | schema 约定 | 适用 |
|---|---|---|---|---|
| `org-member` | `org_id IN (SELECT org_id FROM user_orgs WHERE user_id=$1 AND (expires_at IS NULL OR expires_at > NOW()))` | 同款 EXISTS | 资源表必有 `org_id`（且在 zhuzhao 库） | zhuzhao 自有资源（触发驱动，暂无消费者） |
| `owner-only` | `created_by = $1` | 同款等值 | 资源表必有 `created_by` | 个人数据类 |
| `role-gated` | `Filter{Unscoped: true}`（无行级概念，IW4 显式豁免） | 恒 true（L1 权限码已挡） | 无 | 粗粒度模块 |

**接入形态**：

```go
// 模块 wire/启动处——模块的全部权限代码
reg.Register(resource.Builtin("task", resource.PolicyOrgMember))
```

**设计与边界**：

- **策略逻辑（代码）与策略数据（DB）分离**：事实（谁绑什么角色/权限）进 DB 由管理面运营——现状已如此；语义（模块用哪个行级策略）写死在模块注册处，**不进 DB/管理面**——它与模块代码同生命周期，「不发版换行级策略」无真实变更场景，进 DB 只制造代码与配置漂移面（K8s 内置 ClusterRole / AWS managed policy 同款取舍）。
- **fail-fast**：注册时校验资源表满足 schema 约定（缺 org_id/created_by 列即启动报错），不留到运行时 SQL 报错。
- **护栏**：每策略正/负向集成测试（成员可见非成员 404 / 属主可见他人 403 / role-gated 无码 403）+ IW4 哨兵对 Unscoped 路径的兼容验证；第二个带 L2 的资源出现时，`TestGuard_TicketRepoListCallSites` 泛化为「凡 Filter 参数的 repo.List 必经 Service」。
- **PEP 归属不变**：策略库是 PDP 复用构件，判定执行仍在各模块 Service 内（design-decisions §25.1——行级 PEP 必然在数据处）。

---

## 4. 旧系统借鉴

| 设计 | 决策 | 理由 |
|------|------|------|
| g 表消除 + BFS 展开 | ✅ Phase 2b 采用 | 简化 Casbin 模型 |
| expanded_roles 存 context | ✅ 采用 | Casbin 中间件写入 `roles` 供 handler 复用 |
| 三源合并 | ⏳ Phase 2b | Phase 1 仅直接角色 |
| 资源自注册 | ✅ Phase 2 工单验证 | Phase 1 空接口 |
| Restrict 9 种 ConditionType | ❌ 改为 ltree + 内联 | 降低抽象成本 |
| grants 内存缓存 | ❌ Phase 3 Redis 按需 | 多实例一致 |

---

## 5. 分阶段实施

### Phase 1

- Casbin 中间件（路由级 RBAC）
- g 表消除模型 + matcher superadmin/admin bypass
- `RoleFetcher` 查 `user_roles`（直接角色）；**多角色 OR 鉴权、业务分级用 priority（min）**，见 [phase1/03-authz §用户多角色](../phase1/03-authz.md#用户多角色phase-1)
- ResourceRegistry **空接口**
- Adapter：`noho-digital/casbin-pgx-adapter`（Casbin v3）
- **不做**组织范围数据过滤

### Phase 2

- BFS 三源合并
- OrgResource + ltree 关系
- 列表过滤（GetFilter → SQL WHERE）
- 代码内联资源级鉴权（不上独立 Enforcer）

### Phase 3

- 可选 `perm:user:{userId}` Redis 缓存 + Pub/Sub 失效
- 每资源独立 Enforcer / PDP 评估（按需）
