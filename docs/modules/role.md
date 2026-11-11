# 角色与策略同步

> 状态：当前实现说明。实际用例由 `internal/service/rbac_service.go` 的 `RBACService` 承接；原稿中的 `RoleService` 不是当前服务类型。

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

数据库提交与内存刷新不是一个原子动作。刷新失败会重试一次并返回 `ErrPolicyReloadFailed`，表示数据库可能已经生效；调用方应读回状态并排查，不能认为整个修改已自动回滚。

### 4.3 多实例边界

当前没有已接线的 Casbin Redis Watcher。一个实例的 LoadPolicy 不会自动同步另一个实例的内存；多副本上线前应按 [多实例方案](../phase3/02-multi-instance.md) 设计和验收，不能直接把旧 Pub/Sub 草图当作现有保证。

### 4.4 删除角色

系统角色与仍有直接user_roles用户绑定的角色受到保护，不级联删除用户。当前 `ErrRoleInUse` 对外 HTTP 为 403，准确错误码登记见 [错误码](../api/errcode.md)；旧设计中的 409 不是修改现行为的依据。

## 5. 角色继承

有效功能角色由直接角色、有效组织成员所在节点的组织角色及父角色链合并。父组织赋予的角色不会仅因组织父子关系自动授给子组织成员；数据范围沿组织树扩展是另一条规则。

L1/菜单使用展开角色集；全局用户管理守卫使用直接角色。组织角色不自动升级为全局管理员，这是有意的权限边界，不应在重构时统一成“到处都取 BFS 集合”。

## 6. 设计依据与验证

[RBAC 继承与级联](../design/rbac-inheritance-and-cascade.md)解释角色来源与数据范围的区别；[鉴权模块](authz.md)说明请求链。角色变更需要检查允许、拒绝和查询隐藏三个方向，不能只确认策略表多了一行。

## 7. 变更记录

- 2026-11-08：以实际 RBACService 解释角色、事务与策略刷新，校准系统角色保护、删除错误码和未实施的 Watcher 边界。
