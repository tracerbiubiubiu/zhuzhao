# 鉴权模块设计

> **2026-11-15后继**：[第一包收口](../phase6/iam-first-batch-design.md)记录HIER-D01及两操作本地强审计失败回滚已确认，产品未实现。先讨论A角色授予/编辑，B/C及任务修复分别扣重；不改业务代码、不将旧总价当完整子估。

> **2026-11-15模型/引擎复评**：[RBAC/ABAC/ReBAC/PBAC与成熟工具](../design/authorization-model-and-engine-choice.md)独立复核出发点与复用边界。首版保留Casbin＋领域资源策略、统一用例及事实语义；修正旧“PBAC/ReBAC不演进”“OPA只能bool”“换后端零成本”的论证。模型、检查层和产品分别选择；没有新引擎实施或当前缺陷关闭。

> **2026-11-15层级场景与阶段**：[RBAC/组织/菜单/范围合同§8](../design/rbac-inheritance-and-cascade.md#8-层级关系场景合同与实施归属2026-11-15候选)给HIER-01–12和HIER-D01整子树暂停已确认、未实现；P6.1 C04/D03/D06及首版本地适用部分先交，完整U06和UAA后继。场景表是测试设计，不等于产品验收完成。

> **2026-11-15后继复验（仍未修产品）**：[第二轮权限/资源边界](../review/reports/permission-boundary-recheck-2026-11-15.md)补角色编辑来源固化、加载/目标错配、任职属性往返及两个窄资源边界。下方原AUTH批次6组只代表原批，新增RECHECK状态看控制台§6和[底座§3.4](../phase6/foundation-iam-module-detail.md#34-授权编辑的来源与往返第二轮现状复验)；普通门禁绿不关闭这些问题。

> **2026-11-15现状专项（缺陷未修）**：[权限设计→实现→规划复核](../review/reports/authorization-design-implementation-review-2026-11-15.md)在main确认组织赋角提权、停组授权未撤、业务任务触发审计维护等6组当前问题，另列PAT条件性加固。现有三层架构保留；[底座§3.3](../phase6/foundation-iam-module-detail.md#33-现状缺陷先修与真实重读2026-11-15专项后继)及[任务§8.2](../phase6/task-activelist-message-detail.md#82-存量任务入口与维护动作的接线门)列最小修复/存量入口门。下方历史已实施表不表示这些问题已关闭；当前状态看控制台§6。

> **2026-11-12企业接入校准（未实现）**：[UAA认证与权限治理合同](../design/uaa-governance-integration.md)区分中央身份/受管授权与本地三层执行。现有授权能力不等于已接UAA；企业模式、来源追踪、三路同步与多实例撤权需E01-UAA-C/P/R建设验收。mock和独立开发入口保留，旧“鉴权零改动/换配置即可”不作当前保证。

> 当前阅读顺序：先看§0文档地图和§2.3业务保护，再看资源授权；标注Phase 1流程、缓存/Watcher与末尾阶段清单属历史设计或后续候选。实际注册仅工单；Builtin示例中的tasks表是演示，需真实模块表与schema守卫后才能使用。

> 模块代码：`internal/middleware/casbin.go` + `internal/pkg/resource/` + 各 Service 的 Resource 实现
>
> 旧系统参考：`doc/module-assessment-2026-08/authorizor.md` + `restrict.md` + `resource.md` + `interaction-auth-chain.md`
> 上述路径仅作历史来源标识；原件按[本地材料保管边界](../materials.md#仅限本机的受限资料)集中归档，不要求仓库内保留该路径。
>
> **Phase 1 行为以 [phase1/03-authz.md](../phase1/03-authz.md) 为准**（直接角色、无 Redis 权限缓存、Registry 空接口）。

---

## 0. 权限文档地图（全域入口，2026-09-08 建）

> 权限相关文档分散在 design/modules/phase2/phase3/review 五处——**刻意不合并目录**（文件编号即索引、跨文档相对引用数百处，搬迁=断链；历史教训：曾因引用错文档版本引发误判）。以本节为唯一入口地图，按需取用。

### 0.1 按层分类

| 层 | 文档 | 内容 |
|---|------|------|
| **公约 SSOT**（跨生态约束） | [standards.md §7](../standards.md) | PDP/PEP分工、三层职责、Q5实时约束、模型/引擎按需评估、角色全链平台本职、权限码与声明式注册 |
| **当前模型选型** | [权限模型与引擎选择](../design/authorization-model-and-engine-choice.md) | RBAC/ABAC/ReBAC/PBAC、Casbin/Cerbos/OPA/Cedar/OpenFGA/SpiceDB比较、产品实践及有界演进；旧绝对论证的后继 |
| **决策历史**（为什么这样定） | [design-decisions.md](../design/design-decisions.md) §12/§20/§25/§26 | ReBAC选型史、SoD、PDP/PEP、网关/策略归属、IAM注册与演进；模型结论及修订以本表当前复评为准 |
| **架构评审**（对照与触发表） | [phase2/11-authz-architecture-review.md](../phase2/11-authz-architecture-review.md) | §1.2 业界对照、§2 L2/L3 顺序拍板、§3 不变量（默认拒绝/fail-closed/Q5）、§5 ReBAC 转向触发表（六条）、§9 OPA/ReBAC 迁移复核（2026-09-08） |
| **模块详设**（怎么实现） | 本文 + [phase2/02-authz-resource.md](../phase2/02-authz-resource.md) + [phase2/04-org-delegation.md](../phase2/04-org-delegation.md) + [phase2/03-org-enhance.md](../phase2/03-org-enhance.md) | L1/L2/L3 详设、ScopeResolver/TicketResource、组织委托轴、组织可见性开关 |
| **执行与在册**（做到哪/欠什么） | [review/11-project-control.md](../review/11-project-control.md) §2/§6/§8 + [phase2/00 §9](../phase2/00-implementation-plan.md) + [phase3/00-startup-checklist.md](../phase3/00-startup-checklist.md) | 权限模型速查、健康状态（BK-19~22）、遗留分类、backlog 详情、启动检查单 |
| **Phase 3 权限配套** | [phase3/16-external-integration.md](../phase3/16-external-integration.md) §9 + [phase3/09-platform.md](../phase3/09-platform.md) + [phase3/02-multi-instance.md](../phase3/02-multi-instance.md) + [phase3/03-audit-l2.md](../phase3/03-audit-l2.md) + [phase3/13 §1](../phase3/13-implementation-plan.md) | 外部服务统一基线（AK/SK+断言）、批次 A/B 前置、L1 缓存、AK/SK 平台凭据、Casbin Watcher、判定日志/归档、主链排期 |
| **专项散节** | [ADR-003](../adr/adr-003-activelist-integration-form.md)（activelist 零权限边界）、[phase1/05-role.md](../phase1/05-role.md)（角色 priority 与权限继承模型，§角色 priority 与权限继承模型）＋ [design/rbac-inheritance-and-cascade.md](../design/rbac-inheritance-and-cascade.md)（parent_id 继承与级联）、[phase1/02-auth.md](../phase1/02-auth.md)（认证详设）、[design/architecture.md §4](../design/architecture.md)（总体架构中的权限位） | 按需 |

### 0.2 按目的取用

| 你要做什么 | 读这里（顺序） |
|-----------|---------------|
| 5 分钟了解权限现状 | [控制台 §2](../review/11-project-control.md#2-权限模型速查) → [工程公约](../standards.md) §7 |
| 理解为什么这样设计 | [当前模型复评](../design/authorization-model-and-engine-choice.md) → design-decisions §25/§26历史与后继 → 11-authz §2/§3 |
| 改权限相关代码 | 本文 → [资源授权](../phase2/02-authz-resource.md) → [健康状态](../review/11-project-control.md#6-当前健康状态遗留问题追踪)（先看在册债务） |
| 新服务接入（taskrunner/activelist 式） | 16 号 §9 基线 → ADR-003 → design-decisions §26.2（注册协议） |
| 判断「要不要引入 X / 能力该不该做」 | [模型复评§4/§6](../design/authorization-model-and-engine-choice.md#4-成熟工具如何选) → 逐模块真实成本/对照验证；旧触发表与四墙仅作历史参考 |
| 排查权限问题 | review/11 §2 速查 → §6 健康状态 → 11-authz §3 不变量（403/404/503 语义） |

### 0.3 引用口径警示

权限演进路径的引用 SSOT = **standards §7 + 当前模型复评**；11-authz §5及design-decisions §12/§25/§26保留历史/后继说明，不再单独用旧“不演进”作为否决理由。勿引：standards §11（开发工作流）、ADR-001（工单事件源）、review/11（能力状态而非模型论证）。

---

## 1. 模块定位

**核心底座模块**。实现位置分为路由中间件和业务 Service 两处；业务判定仍按 L1 路由、L2 资源可见性、L3 动作权限三层理解：

| 层级 | 位置 | Phase 1 | Phase 2+ |
|------|------|---------|----------|
| 路由级 RBAC | Casbin **中间件** | ✅ 直接角色 enforce | BFS 三源 + 可选缓存 |
| 资源级 | **Service** + ResourceRegistry | 空接口 | Authorize + GetFilter |

与其他模块的关系：
- `middleware/casbin.go`：路由级鉴权（查 `casbin_rule` + `user_roles`）
- 当前由 `internal/app/providers.go` 装配 Registry，`internal/service/ticket/service.go` 的 `NewTicketService` 自注册工单 Resource；其他资源不能因接口存在就视为已经接入。
- Handler 进入工单 Service 后，由 `internal/service/ticket/service.go` 的 `authorizeCheck → authorize` 调用 `registry.Authorize`；列表/导出共用 `resolveListFilter`：admin/superadmin 显式返回 `Unscoped`，其余调用 `registry.GetFilter`。资源检查不在通用 middleware，也没有 `resource_authz.go`。

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

### 角色菜单集合更新

角色菜单整体替换可提交 `original_menu_ids`，填写本次编辑前读到的菜单 ID 集合。空数组表示已读空集。服务在锁定角色后比较当前集合，发现变化返回 HTTP 409，原集合继续生效。比较基于此刻的全集，允许排序变化及曾还原至相同集合的情况。省略字段或传入 `null` 时沿用既有整体替换行为。

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

<a id="221-自服务路由业界做法-本项目决策"></a>

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
- 当前实现以 `internal/middleware/casbin.go` 的 `SelfService / CasbinAuth` 和 `internal/router/router.go` 的注册顺序为准；[Phase 1 伪代码](../phase1/09-middleware.md#casbin-中间件g-表消除)保留设计来源。

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
| 改/删/禁用用户 | `UserService.Update/Delete/UpdateStatus` | `actorP < targetP`；`is_system` 保护；不能删最后 superadmin | 403 / 404 |
| 改角色菜单 | `RBACService.AssignMenus` | admin **不能**改 `superadmin` 角色的菜单 | 403 |
| 删改系统资源 | `Role/Menu/Org Service` | `is_system=true` 时 admin 拒绝 | 403 |
| 列表/详情「看不见」 | `UserRepo.List` / `RoleRepo.List` / `GetByID` | 非 superadmin：**过滤** superadmin 角色及绑定用户 | 列表不含 / 详情 **404** |

**priority 计算**（与 [04-user §多角色](../phase1/04-user.md#多角色与有效-priority) 一致）：

```go
// 管理防提权使用直接角色的最小 priority；越小越强，不用组织派生角色升级管理权。
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

当前由 `RBACService.GetRoleCodesByUserID` 合并直接角色、有效组织成员所在节点的组织角色及父角色链；角色来源见[角色模块](role.md)。当前没有接线权限结果 Redis 缓存或跨实例 Watcher。下框是早期接口/缓存演进草图，符号不是现行 API，不可直接复制为实现：

```go
func (f *roleFetcher) FetchRoleCodes(ctx context.Context, userID int64) ([]string, error) {
    // Phase 2: 源1 直接角色 + 源2 组织角色 + 源3 继承角色，去重
    // Phase 3: 可选 Redis perm:user:{userId} 缓存
}
```

---

## 3. 资源级鉴权（ResourceRegistry）

Phase 1 只定义 `Resource` / `Registry` 接口，不注册业务 Resource。

当前工单在 `NewTicketService` 构造函数中 `registry.Register(...)`，在 Service 方法内调用。下面只演示调用形态，`uid / roles / ticketID` 由业务用例准备：

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
reg.Register(resource.Builtin("task", resource.OrgMember("tasks")))  // 接线示意，tasks只是本库表占位；跨库taskrunner不适用。生产Registry当前仅注册ticket。
```

**设计与边界**：

- **策略逻辑（代码）与策略数据（DB）分离**：事实（谁绑什么角色/权限）进 DB 由管理面运营——现状已如此；语义（模块用哪个行级策略）写死在模块注册处，**不进 DB/管理面**——它与模块代码同生命周期，「不发版换行级策略」无真实变更场景，进 DB 只制造代码与配置漂移面（K8s 内置 ClusterRole / AWS managed policy 同款取舍）。
- **schema 护栏能力**：`RequireSchema` 可校验资源表满足列约定；接入 Builtin 的模块须在启动路径显式调用并处理错误。`Registry.Register` 本身只注册资源，不自动查库；当前没有 Builtin 消费者，也没有生产装配调用 `RequireSchema`，不能把工具能力当作已经自动生效的启动守卫。
- **护栏**：每策略正/负向集成测试（成员可见非成员 404 / 属主可见他人 403 / role-gated 无码 403）+ IW4 哨兵对 Unscoped 路径的兼容验证；第二个带 L2 的资源出现时，`TestGuard_TicketRepoListCallSites` 泛化为「凡 Filter 参数的 repo.List 必经 Service」。
- **PEP 归属不变**：策略库是 PDP 复用构件，判定执行仍在各模块 Service 内（design-decisions §25.1——行级 PEP 必然在数据处）。

---

## 4. 旧系统借鉴

| 设计 | 决策 | 理由 |
|------|------|------|
| g 表消除 + BFS 展开 | ✅ Phase 2b 采用 | 简化 Casbin 模型 |
| expanded_roles 存 context | ✅ 采用 | Casbin 中间件写入 `roles` 供 handler 复用 |
| 三源合并 | ✅ 已实现 | 当前合并直接角色、组织角色、父角色链；Phase 1 仅直接角色是历史范围 |
| 资源自注册 | ✅ Phase 2 工单验证 | Phase 1 空接口 |
| Restrict 9 种 ConditionType | ❌ 改为 ltree + 内联 | 降低抽象成本 |
| grants 内存缓存 | ❌ Phase 3 Redis 按需 | 多实例一致 |

---

## 5. 分阶段实施（历史范围，不作当前完成清单）

### Phase 1

- Casbin 中间件（路由级 RBAC）
- g 表消除模型 + matcher superadmin/admin bypass
- `RoleFetcher` 查 `user_roles`（直接角色）；**多角色 OR 鉴权、业务分级用 priority（min）**，见 [phase1/03-authz §用户多角色](../phase1/03-authz.md#用户多角色phase-1)
- ResourceRegistry **空接口**
- Adapter：`noho-digital/casbin-pgx-adapter`（Casbin v3）
- **不做**组织范围数据过滤

### Phase 2

- BFS 三源合并
- 原拟 OrgResource + ltree 关系；**未按此形态交付**，当前仅工单注册 Resource，组织委托由 OrgDelegationService 实现
- 列表过滤（GetFilter → SQL WHERE）
- 代码内联资源级鉴权（不上独立 Enforcer）

### Phase 3

- 可选 `perm:user:{userId}` Redis 缓存 + Pub/Sub 失效
- 每资源独立 Enforcer / PDP 评估（按需）
