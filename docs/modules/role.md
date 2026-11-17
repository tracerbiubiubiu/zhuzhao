# 角色与策略同步

> 状态：当前实现说明。实际用例由 `internal/service/rbac_service.go` 的 `RBACService` 承接；原稿中的 `RoleService` 不是当前服务类型。

> 2026-11-17状态：本批有效来源、CF-06目标保护及当前写命令fresh子出口已通过完整适用门禁和组合复验，见[正式修复报告](../review/reports/audit-baseline-repair-2026-11-17.md)；完整IAM-C/C05及HA仍未交付。

## 1. 模块定位

角色连接用户、菜单和后端 API 授权。先区分两个问题：角色决定能调用哪些接口；操作一个具体用户、角色或工单时，还要通过该用例自己的业务与资源检查。

`superadmin` 和 `admin` 在 Casbin 路由层有通配放行，但不等于 admin 可以管理同级或更强账号、修改所有系统角色。业务约束在 Service 中继续生效。

## 2. 数据模型

`roles` 保存 code、priority、状态、版本与可选父角色；`user_roles` 是直接赋角，`org_roles` 是组织赋角，`role_menus` 是菜单分配。Casbin 的 `casbin_rule` 是 API 策略派生结果。

priority 数字越小越强。创建或变更父链时校验环、父状态和 `child.priority ≤ parent.priority`，避免弱角色通过继承强角色绕开赋角检查。实际约束见 `RBACService.validateParentLink`。

## 3. 接口定义

`RBACService` 提供角色 CRUD、`AssignMenus`、菜单/策略查询及 `GetRoleCodesByUserID`。字段、版本和请求路由以 Swagger 为准；没有仅因设计了继承树就已经交付可视化树管理的承诺。

## 4. 核心流程

### 4.1 菜单和 API 策略

页面、按钮与 API 绑定由迁移维护。管理员给角色分配菜单后，后端从相关 `menu_apis` 计算 API 策略；前端的 `button:`、`route:` 权限码由菜单查询接口生成，不是另一份任意可编辑的后端策略体系。

### 4.2 策略同步流程

`RBACService.AssignMenus` 校验目标角色和菜单，再由 `internal/repository/role_repo.go` 的 `AssignMenus` 在同一事务修改菜单绑定与 API 策略。事务提交后刷新本实例 Casbin 内存。

数据库提交与内存刷新不是一个原子动作。custom角色p变更在最终commit前将本实例共享策略投影标为不可用；提交确认后先释放PG session锁/连接，保持本机串行再reload，避免单连接pool自等。刷新失败会重试一次并返回`ErrPolicyReloadFailed`，数据库可能已经生效。

不可用期间，`middleware.CasbinAuth`的缓存p消费者，以及SelfService路由内`PeopleService.Search`的assign/create/rule静态动作，在Enforce前后检查同一状态并返回503。本人SelfService读、People cc的原DB L3不消费该p缓存，保持原规则。pending之前已通过中间件的请求可能完成；本批管理写仍同Tx重验当前p。成功重载开闸；两次失败、取消或pending后的commit不确定保持关闭，运维排查后受控重启，启动完整LoadPolicy成功后恢复。没有新增恢复API；不得将已提交DB错误误作回滚，也不声称所有资源在途请求的revision协议已交付。

实际故障识别、核对数据库已提交状态及受控重启的步骤归[runbook §6.3](../ops/runbook.md#63-当前casbin策略投影失败的恢复)。健康探针成功不单独证明策略投影已恢复；须验证应保留权限的专属账号成功、已撤权账号仍拒绝。本批不新增后台无限重试、自动开闸或完整C05会话恢复机制。

### 4.3 多实例边界

当前没有已接线的 Casbin Redis Watcher。一个实例的 LoadPolicy 不会自动同步另一个实例的内存；多副本上线前应按 [多实例方案](../phase3/02-multi-instance.md) 设计和验收，不能直接把旧 Pub/Sub 草图当作现有保证。

### 4.4 删除角色

系统角色与仍有直接user_roles用户绑定的角色受到保护，不级联删除用户。当前 `ErrRoleInUse` 对外 HTTP 为 403，准确错误码登记见 [错误码](../api/errcode.md)；旧设计中的 409 不是修改现行为的依据。

## 5. 角色继承

有效功能角色由直接角色、有效组织成员所在节点的组织角色及父角色链合并。父组织赋予的角色不会仅因组织父子关系自动授给子组织成员；数据范围沿组织树扩展是另一条规则。

2026-11-17 IAM-B候选让`RoleRepo.GetEffectiveRoleCodes/ListRoleIDsByUserID`共用`EffectiveRoleCTE`：组织来源须本组与祖先均有效，任职未过期；角色链只沿启用、未删父节点继续，中间父停用即截断该路径，独立直接绑定的更高祖先仍可贡献。

`RoleRepo.GetEffectiveRolesForTargetProtection`沿同一CTE返回角色实体与priority，刻意不以users.status清空来源，供目标账号保护只增加拒绝；该查询接线及CF-06敏感写行为已通过本批专项和组合验收。它不替换操作者的直接IAM资格。实际接线、并发与组合验证范围见[正式修复报告](../review/reports/audit-baseline-repair-2026-11-17.md)。

L1/菜单使用展开角色集；全局用户管理操作者资格使用直接角色。组织角色不自动升级操作者的直接IAM管理档位，这是有意的权限边界，不应在重构时统一成“到处都取 BFS 集合”。

## 6. 当前写命令共同屏障

`internal/repository/iam_guard.go`的`WithIAMGuard`先在占用pool前等待本进程有界串行门，再在同一PG session取得advisory锁，取得锁后才Begin。该连接同时承载业务事务、所有保护查询和最终commit；session失效不能在另一连接继续提交。嵌套repo事务使用同Tx的savepoint。顺序固定为共同IAM屏障→既有最后超管/目标/组织锁。低层`*Tx`写辅助函数取得同键xact锁；外部事务所有者须在任何IAM行锁之前取得屏障，不能据此跳过service授权。

协作写方涵盖`UserRepo`身份/状态/口令/删除/直接角色，`RoleRepo`创建/档位/状态/父边/删除/菜单策略，`OrgRepo`创建/状态/移动/删除/成员集合/任职/scope/组织角色绑定及虚拟组owner清理；`OrgDelegationService`、角色和菜单查询在屏障内走context同Tx。CLI应急启用也复用repo源写，运维授权保持原边界。菜单词表运行时只读；受审repo/种子工具写同样协作。直接数据库维护和迁移应离线受控执行。

当前HTTP命令映射为：用户`POST /api/v1/users`及`/update /delete /status /roles /password/reset /orgs`；角色`POST /api/v1/roles`及`/update /delete /menus`；组织`POST /api/v1/orgs`及`/update /move /delete /members /members/delete /members/role /members/scope /owners /roles/bind /roles/delete`。组织委托路由继续使用原SelfService+组内/精确功能码/IAM资格规则，不额外要求静态API策略；当前角色来源仍会fresh重验。

数据库commit确认后才执行Redis会话投影和Casbin reload，并保留本机串行。投影失败仍明确表示数据库可能已提交。此处是当前已启用写命令的必要fresh子出口，未交付完整IAM-C的客户端expected revision、receipt、C05恢复协议或其它业务资源的并发屏障。

## 7. 设计依据与验证

[RBAC 继承与级联](../design/rbac-inheritance-and-cascade.md)解释角色来源与数据范围的区别；[鉴权模块](authz.md)说明请求链。角色变更需要检查允许、拒绝和查询隐藏三个方向，不能只确认策略表多了一行。

## 8. 变更记录

- 2026-11-17：共用有效来源/角色闭包，补CF-06目标保护及当前写命令共同屏障；本批门禁已通过，完整IAM-C/C05仍后继。
- 2026-11-08：以实际 RBACService 解释角色、事务与策略刷新，校准系统角色保护、删除错误码和未实施的 Watcher 边界。
