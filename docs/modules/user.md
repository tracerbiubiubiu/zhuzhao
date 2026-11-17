# 用户模块

> 状态：当前实现说明。模型见 `internal/model/user.go` 的 `User`，用例见 `internal/service/user_service.go` 的 `UserService`。历史实现计划见 [Phase 1 用户稿](../phase1/04-user.md)。

> 2026-11-17状态：本批用户目标保护、组织新增授予与同事务fresh子出口已通过完整适用门禁及组合复验，见[正式修复报告](../review/reports/audit-baseline-repair-2026-11-17.md)；完整IAM-C/C05仍后继，未部署。

## 1. 模块定位

用户模块维护本地身份资料、账号状态、密码与角色绑定。认证模块消费它的身份数据，组织成员写入则统一委托 `OrgService`；不要在用户仓储中再实现一套组织授权规则。

全局管理权限由启用且未删的非空直接角色实体及合法priority（≥1）守卫判断；空集9999哨兵不授予actor管理资格，也不设置任意priority上限。无角色target继续是弱方。用户管理当前没有另行注册 `UserResource`；不能把旧接口草图当作已启用的通用资源过滤。

## 2. 数据模型

`username` 用于资料展示，可重复，不是账密登录键；登录键是 `employee_no`。迁移 000006 对非空工号及域账号组合使用活跃行部分唯一索引（deleted_at IS NULL）；软删记录不再占用唯一键，数据库允许新用户复用。复用标识不等于恢复原用户ID，HR对账和历史审计必须区分两者。

`users` 保存账号状态、强制改密标记、资料和版本。密码不序列化到 JSON，ID 为字符串。角色绑定在 `user_roles`，组织绑定在 `user_orgs`；完整列与索引查看实际迁移，不在此维护第二份建表 SQL。

头像当前由对象存储上传/绑定链路提供，并非只有“手填 URL”能力。资料更新不能回写旧读取得到的禁用状态、强制改密状态或头像代际，相关回归记录见 [当前健康状态](../review/11-project-control.md#6-当前健康状态遗留问题追踪)。

## 3. 接口定义

CRUD、状态、管理员重置密码和角色分配由 `UserService.Create / Update / Delete / UpdateStatus / ResetPassword / SetRoles` 等用例承接。当前资料入口是 `GetProfile / UpdateProfile`；自服务改密码由 `AuthService.UpdatePassword` 处理。

准确请求字段和 POST 路由查 Swagger。更新是 patch 语义：未传保持原值，允许清空的字段以显式空值表达；版本冲突应重新读取，不用旧快照反复覆盖。

## 4. 核心流程

### 4.1 创建用户

请求检查与密码哈希后，受共同IAM屏障保护的同一事务读取操作者启用的直接角色，校验初始直接角色和新增组织成员潜在授予的角色，再创建用户及初始角色/组织关系。手工创建不会调用真实 HR 接口。HR 同步目前仍是独立的后续能力，不能从 `source` 字段存在推导已经接入。

### 4.2 删除用户（级联）

先检查目标可管理性、系统账号、自我删除与最后一个有效 superadmin 等约束，再软删除并处理绑定关系，随后撤销会话。删除不会静默删除工单，也不自动代表名下事项已交接；活跃唯一键会释放，新建同工号仍是另一用户ID。

### 4.3 设置用户角色

`SetRoles` 全量覆盖直接角色绑定，数据库变更有事务保护。业务防提权使用直接角色的有效 priority；角色派生出的组织权限不能反过来当作全局账号管理权。

### 4.4 目标保护与提交时授权

`UserService.ensureTargetProtection`对管理员重置密码、删除、启停、直接角色/组织集合替换，以及实际变更工号、域账号或用户域的`Update`，读取目标当前有效直接/组织/父角色档位，作为原直接角色规则之外的附加拒绝条件。停用目标账号保留来源保护。本人、superadmin直通及系统/最后超管守卫沿原合同；展示资料不增加目标有效档位限制。

以上管理命令及普通管理`Update`在`repository.WithIAMGuard`中完成当前API资格、直接IAM档位、目标保护和写入。生产HTTP请求须有成功`CasbinAuth`签发的私有purpose；固定路由与POST、actor、SelfService性质均复核，随后用同事务当前p行和有效L1角色复用配置Casbin模型。缺少purpose拒绝；直接service夹具须显式`TrustedIAMInternal`，该能力不由handler或body默认授予。

`UpdateProfile`只调用`UserRepo.UpdateProfile`写展示字段并保留version CAS、持久安全状态回显；它不写身份/状态/强制改密，继续允许头像绑定的原有CAS并发。正常本人改密仍归`AuthService.UpdatePassword`。

### 4.5 列表查询

列表受路由权限、查询参数和 superadmin 隐藏规则约束。普通管理端列表没有“默认只看本部门员工”的承诺。工号精确过滤与用户名模糊过滤是不同用途，不能把同名资料字段当唯一身份。

## 5. 密码安全

当前请求约束与公共库 bcrypt 实现是准确依据。旧稿中的 `PasswordValidator` 四字符类、密码历史和到期策略不是现有完整策略的证明。

改密与重置会更新安全状态并撤销旧会话。superadmin 应急恢复使用受执行权限控制的 CLI，流程、审计与失败处置见 [runbook](../ops/runbook.md)；人工测试使用专属账号。

## 6. 验证与排查

修改资料、停用账号、角色分配和改密需要分别验证，不能用一次资料保存成功推导安全状态正确。典型排查顺序是：确认目标身份与版本，查看具体业务错误码，再按 request_id 查询审计；不要将内部数据库诊断直接返回前端。

## 7. 相关设计与变更记录

- [组织模块](organization.md)：成员、主组织、虚拟组与委托的写侧边界。
- [角色模块](role.md)：直接角色、展开角色与 priority。
- [HR 同步方案](../proposal/hr-directory-sync.md)：未实施同步的设计及待定项。
- 2026-11-17：补CF-06目标拒绝、组织新增授予与同事务fresh子出口；完整IAM-C的revision/receipt及其它资源消费者仍另行验收。
- 2026-11-08：移除重复且过时的 DDL/接口草图，校准软删唯一性、当前头像能力、密码策略及管理权限说明。
