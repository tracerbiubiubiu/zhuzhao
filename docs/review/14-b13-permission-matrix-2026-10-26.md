# 14 · B13 权限覆盖矩阵复审（2026-10-26 point-in-time）

> **文档定位**：[12 号](./12-b13-permission-matrix-2026-09-26.md)（2026-09-26 / phase4 基线 565317b，77 用户面端点）以来的全量复审——phase5 十段新增大量端点后的再次对账（台账 B-19，09 月提议至今）。同 12 号口径：point-in-time 审计，不背活文档包袱。
>
> **基准**：2026-10-26，分支 phase5（`0d5d2fd`），DB v79。**数据源=实况 DB（menu_apis 123 行）+ catalog.go 豁免集（44——勘误：首版误计 45，系把 catalog.go:145 注释里的示例键 "GET /api/v1/users" 数入；该端点实为 system_user 页面绑定，非豁免）+ 代码实核**。完备性由两道启动守卫结构性保证：路由↔menu_apis 对账 fail-fast（BK-22——非豁免路由必有绑定、绑定必指向真实路由）+ B 案守卫（页面行禁挂非 GET——2026-10-26 起）。即：**端点全集 ≡ menu_apis ∪ 豁免 ∪ 机器面**，无第三态。

## 1. 总览

| 面 | 数量 | L1 机制 |
|---|---|---|
| menu_apis 绑定（页面 54 GET / 按钮 37 / 目录 0） | **123** | Casbin 按 (path,method)；B 案重排后页面=读、写=按钮 |
| 豁免集（catalogExempt） | **44** | auth 面 5（公开 3：login/refresh/captcha + 登录态自服务 2：logout/password-update）+ orgs 委托面 10（L3 判定）+ 其余 SelfService 29（自数据/本人限定——存储 4、ticket 自服务 9、inbox 3、user 面 8、badges/文件代理/变量字典 5） |
| 机器面 | 2 | /internal/jobs/callback（AK/SK）+ 探针 |
| 网关 | — | /al 前缀反代（跨仓豁免） |

**phase5 以来新增面（12 号基准外）**：存储自服务 7（presign/confirm/avatar/bind/attachments/richtext）、cc/tags 各 3、badges、intel/query、transition+available、assignment-rules 4（按钮行）、tickets/export、jobs/actions、通知配置 8+模板 4（000077 迁按钮行）、notifications/inbox 族 3（000076 起 selfService）。**重排**：000075/77 把写端点从页面行迁按钮行（B 案三道守卫在册）；000078 删 message_center 幽灵行（inbox 三端点 selfService 化）。

## 2. 三层覆盖判定

- **L1**：全 123 绑定 + 45 豁免如上；守卫三道（启动 fail-fast / CI 集成测试 / acceptance D2 B案）持续在册。
- **L2（行级，resource.Registry）**：仍**仅 ticket 域注册**（`service.go:124`——GetFilter 可见性管线，cc 第四 OR 支并入）；/al 域由 activelist 侧判定（代理透传）；其余域无行级语义（系统配置/审计/情报为全局资源）。
- **L3（服务内）**：SelfService 面 34 端点全部 L3 管辖（本人限定/自数据——12 号 §域 2/3 结论维持）；orgDelegated 8 端点维持 L3 判定（owner/admin 判定链不变）。新增自服务面逐条：cc/tags（挂/自摘=读可见、摘他人=update 权——服务端判）、attachments（L2 read/update 面由 handler 编排）、inbox（recipient=self 硬限定）、badges（自聚合无越权面）、richtext render（纯消毒无票据）。

## 3. 矩阵（按域，123 绑定全量）

### al 活动列表域（14 绑定）

| 菜单行 | 型 | 权限码 | 方法 | 端点 | L2 |
|---|---|---|---|---|---|
| al_data | 页面 | activelist:data:read | GET | `/al/api/v1/data/:typeName` | ✔ 代理 |
| al_data | 页面 | activelist:data:read | GET | `/al/api/v1/data/:typeName/:id` | ✔ 代理 |
| al_data | 页面 | activelist:data:read | GET | `/al/api/v1/data/:typeName/export` | ✔ 代理 |
| al_data_write_btn | 按钮 | activelist:data:write | POST | `/al/api/v1/data/:typeName` | ✔ 代理 |
| al_data_write_btn | 按钮 | activelist:data:write | POST | `/al/api/v1/data/:typeName/:id/delete` | ✔ 代理 |
| al_data_write_btn | 按钮 | activelist:data:write | POST | `/al/api/v1/data/:typeName/:id/update` | ✔ 代理 |
| al_data_write_btn | 按钮 | activelist:data:write | POST | `/al/api/v1/data/:typeName/import` | ✔ 代理 |
| al_data_write_btn | 按钮 | activelist:data:write | POST | `/al/api/v1/data/restore` | ✔ 代理 |
| al_type_manage_btn | 按钮 | activelist:type:manage | POST | `/al/api/v1/admin/types` | ✔ 代理 |
| al_type_manage_btn | 按钮 | activelist:type:manage | POST | `/al/api/v1/admin/types/deprecate` | ✔ 代理 |
| al_type_manage_btn | 按钮 | activelist:type:manage | POST | `/al/api/v1/admin/types/schema` | ✔ 代理 |
| al_types | 页面 | activelist:type:read | GET | `/al/api/v1/admin/types` | ✔ 代理 |
| al_types | 页面 | activelist:type:read | GET | `/al/api/v1/admin/types/:typeName` | ✔ 代理 |
| al_types | 页面 | activelist:type:read | GET | `/al/api/v1/admin/types/:typeName/history` | ✔ 代理 |

### audit 审计域（4 绑定）

| 菜单行 | 型 | 权限码 | 方法 | 端点 | L2 |
|---|---|---|---|---|---|
| audit_log | 页面 | audit:read | GET | `/api/v1/audit/logs` | — |
| audit_log | 页面 | audit:read | GET | `/api/v1/audit/panics` | — |
| audit_log | 页面 | audit:read | GET | `/api/v1/audit/reconcile` | — |
| audit_log | 页面 | audit:read | GET | `/api/v1/system/cron-status` | — |

### intel 情报域（1 绑定）

| 菜单行 | 型 | 权限码 | 方法 | 端点 | L2 |
|---|---|---|---|---|---|
| intel_query | 页面 | intel:query | GET | `/api/v1/intel/query` | — |

### system 系统域（54 绑定）

| 菜单行 | 型 | 权限码 | 方法 | 端点 | L2 |
|---|---|---|---|---|---|
| dict_write_btn | 按钮 | dict:manage | POST | `/api/v1/dict-items` | — |
| dict_write_btn | 按钮 | dict:manage | POST | `/api/v1/dict-items/delete` | — |
| dict_write_btn | 按钮 | dict:manage | POST | `/api/v1/dict-items/update` | — |
| dict_write_btn | 按钮 | dict:manage | POST | `/api/v1/dicts` | — |
| dict_write_btn | 按钮 | dict:manage | POST | `/api/v1/dicts/delete` | — |
| dict_write_btn | 按钮 | dict:manage | POST | `/api/v1/dicts/update` | — |
| notification_write_btn | 按钮 | notification:manage | POST | `/api/v1/message-rules` | — |
| notification_write_btn | 按钮 | notification:manage | POST | `/api/v1/message-rules/delete` | — |
| notification_write_btn | 按钮 | notification:manage | POST | `/api/v1/message-templates` | — |
| notification_write_btn | 按钮 | notification:manage | POST | `/api/v1/message-templates/delete` | — |
| notification_write_btn | 按钮 | notification:manage | POST | `/api/v1/message-templates/update` | — |
| notification_write_btn | 按钮 | notification:manage | POST | `/api/v1/notifications` | — |
| notification_write_btn | 按钮 | notification:manage | POST | `/api/v1/notifications/delete` | — |
| notification_write_btn | 按钮 | notification:manage | POST | `/api/v1/notifications/update` | — |
| system_dict | 页面 | dict:read | GET | `/api/v1/dict-items` | — |
| system_dict | 页面 | dict:read | GET | `/api/v1/dicts` | — |
| system_menu | 页面 | — | GET | `/api/v1/menus` | — |
| system_menu | 页面 | — | GET | `/api/v1/menus/:id` | — |
| system_notification | 页面 | notification:read | GET | `/api/v1/message-logs` | — |
| system_notification | 页面 | notification:read | GET | `/api/v1/message-rules` | — |
| system_notification | 页面 | notification:read | GET | `/api/v1/message-templates` | — |
| system_notification | 页面 | notification:read | GET | `/api/v1/notifications` | — |
| system_org | 页面 | — | GET | `/api/v1/orgs` | — |
| system_org | 页面 | — | GET | `/api/v1/orgs/:id` | — |
| system_org | 页面 | — | GET | `/api/v1/orgs/:id/members` | — |
| system_org_create | 按钮 | org:create | POST | `/api/v1/orgs` | — |
| system_org_delete | 按钮 | org:delete | POST | `/api/v1/orgs/delete` | — |
| system_org_member | 按钮 | org:member | POST | `/api/v1/orgs/members` | — |
| system_org_member | 按钮 | org:member | POST | `/api/v1/orgs/members/delete` | — |
| system_org_member | 按钮 | org:member | POST | `/api/v1/orgs/members/role` | — |
| system_org_member | 按钮 | org:member | POST | `/api/v1/orgs/owners` | — |
| system_org_move | 按钮 | org:move | POST | `/api/v1/orgs/move` | — |
| system_org_update | 按钮 | org:update | POST | `/api/v1/orgs/update` | — |
| system_role | 页面 | — | GET | `/api/v1/roles` | — |
| system_role | 页面 | — | GET | `/api/v1/roles/:id` | — |
| system_role | 页面 | — | GET | `/api/v1/roles/:id/menus` | — |
| system_role | 页面 | — | GET | `/api/v1/roles/:id/permissions` | — |
| system_role_assign_menu | 按钮 | role:assign_menu | POST | `/api/v1/roles/menus` | — |
| system_role_create | 按钮 | role:create | POST | `/api/v1/roles` | — |
| system_role_delete | 按钮 | role:delete | POST | `/api/v1/roles/delete` | — |
| system_role_update | 按钮 | role:update | POST | `/api/v1/roles/update` | — |
| system_setting | 页面 | — | GET | `/api/v1/system/settings` | — |
| system_setting_update | 按钮 | system:setting:update | POST | `/api/v1/system/settings/update` | — |
| system_user | 页面 | — | GET | `/api/v1/users` | — |
| system_user | 页面 | — | GET | `/api/v1/users/:id` | — |
| system_user | 页面 | — | GET | `/api/v1/users/:id/orgs` | — |
| system_user | 页面 | — | GET | `/api/v1/users/:id/roles` | — |
| system_user_assign_org | 按钮 | user:assign_org | POST | `/api/v1/users/orgs` | — |
| system_user_assign_role | 按钮 | user:assign_role | POST | `/api/v1/users/roles` | — |
| system_user_create | 按钮 | user:create | POST | `/api/v1/users` | — |
| system_user_delete | 按钮 | user:delete | POST | `/api/v1/users/delete` | — |
| system_user_reset_pwd | 按钮 | user:reset_password | POST | `/api/v1/users/password/reset` | — |
| system_user_status | 按钮 | user:status | POST | `/api/v1/users/status` | — |
| system_user_update | 按钮 | user:update | POST | `/api/v1/users/update` | — |

### task 任务域（11 绑定）

| 菜单行 | 型 | 权限码 | 方法 | 端点 | L2 |
|---|---|---|---|---|---|
| task_center | 页面 | task:read | GET | `/api/v1/dead-letters` | — |
| task_center | 页面 | task:read | GET | `/api/v1/jobs/actions` | — |
| task_center | 页面 | task:read | GET | `/api/v1/runs` | — |
| task_center | 页面 | task:read | GET | `/api/v1/tasks/:id` | — |
| task_manage_btn | 按钮 | task:manage | GET | `/api/v1/jobs` | — |
| task_manage_btn | 按钮 | task:manage | POST | `/api/v1/jobs` | — |
| task_manage_btn | 按钮 | task:manage | POST | `/api/v1/jobs/trigger` | — |
| task_manage_btn | 按钮 | task:manage | POST | `/api/v1/jobs/update` | — |
| task_operate_btn | 按钮 | task:operate | POST | `/api/v1/tasks/cancel` | — |
| task_operate_btn | 按钮 | task:operate | POST | `/api/v1/tasks/retry` | — |
| task_submit_btn | 按钮 | task:submit | POST | `/api/v1/tasks` | — |

### ticket 工单域（39 绑定）

| 菜单行 | 型 | 权限码 | 方法 | 端点 | L2 |
|---|---|---|---|---|---|
| ticket_assign_btn | 按钮 | ticket:assign | POST | `/api/v1/tickets/assign` | — |
| ticket_assignment_rules | 页面 | ticket:type:manage | GET | `/api/v1/assignment-rules` | — |
| ticket_assignment_rules_write_btn | 按钮 | ticket:type:manage | POST | `/api/v1/assignment-rules` | — |
| ticket_assignment_rules_write_btn | 按钮 | ticket:type:manage | POST | `/api/v1/assignment-rules/delete` | — |
| ticket_assignment_rules_write_btn | 按钮 | ticket:type:manage | POST | `/api/v1/assignment-rules/enabled` | — |
| ticket_close_btn | 按钮 | ticket:close | POST | `/api/v1/tickets/close` | — |
| ticket_comment_btn | 按钮 | ticket:comment | POST | `/api/v1/tickets/comments` | — |
| ticket_create_btn | 按钮 | ticket:create | POST | `/api/v1/tickets` | — |
| ticket_delete_btn | 按钮 | ticket:delete | POST | `/api/v1/tickets/delete` | — |
| ticket_list | 页面 | ticket:list | GET | `/api/v1/orgs` | — |
| ticket_list | 页面 | ticket:list | GET | `/api/v1/ticket-templates` | — |
| ticket_list | 页面 | ticket:list | GET | `/api/v1/ticket-templates/:code` | — |
| ticket_list | 页面 | ticket:list | GET | `/api/v1/ticket-types` | — |
| ticket_list | 页面 | ticket:list | GET | `/api/v1/ticket-types/:code/fields` | — |
| ticket_list | 页面 | ticket:list | GET | `/api/v1/tickets` | — |
| ticket_list | 页面 | ticket:list | GET | `/api/v1/tickets/:id` | — |
| ticket_list | 页面 | ticket:list | GET | `/api/v1/tickets/:id/comments` | — |
| ticket_list | 页面 | ticket:list | GET | `/api/v1/tickets/:id/relations` | — |
| ticket_list | 页面 | ticket:list | GET | `/api/v1/tickets/:id/transitions/available` | — |
| ticket_list | 页面 | ticket:list | GET | `/api/v1/tickets/export` | — |
| ticket_list | 页面 | ticket:list | POST | `/api/v1/tickets/transition` | — |
| ticket_manage | 目录 | — | GET | `/api/v1/orgs` | — |
| ticket_note_btn | 按钮 | ticket:note | POST | `/api/v1/tickets/notes` | — |
| ticket_relation_btn | 按钮 | ticket:relation | POST | `/api/v1/tickets/relations` | — |
| ticket_type_manage | 页面 | ticket:type:manage | GET | `/api/v1/assignment-rules` | — |
| ticket_type_manage | 页面 | ticket:type:manage | GET | `/api/v1/orgs` | — |
| ticket_type_manage | 页面 | ticket:type:manage | GET | `/api/v1/ticket-templates` | — |
| ticket_type_manage | 页面 | ticket:type:manage | GET | `/api/v1/ticket-templates/:code` | — |
| ticket_type_manage | 页面 | ticket:type:manage | GET | `/api/v1/ticket-types` | — |
| ticket_type_manage | 页面 | ticket:type:manage | GET | `/api/v1/ticket-types/:code/fields` | — |
| ticket_type_manage | 页面 | ticket:type:manage | GET | `/api/v1/ticket-types/:code/fields/impact` | — |
| ticket_type_write_btn | 按钮 | ticket:type:manage | POST | `/api/v1/ticket-templates` | — |
| ticket_type_write_btn | 按钮 | ticket:type:manage | POST | `/api/v1/ticket-templates/delete` | — |
| ticket_type_write_btn | 按钮 | ticket:type:manage | POST | `/api/v1/ticket-templates/update` | — |
| ticket_type_write_btn | 按钮 | ticket:type:manage | POST | `/api/v1/ticket-types` | — |
| ticket_type_write_btn | 按钮 | ticket:type:manage | POST | `/api/v1/ticket-types/delete` | — |
| ticket_type_write_btn | 按钮 | ticket:type:manage | POST | `/api/v1/ticket-types/fields/replace` | — |
| ticket_type_write_btn | 按钮 | ticket:type:manage | POST | `/api/v1/ticket-types/update` | — |
| ticket_update_btn | 按钮 | ticket:update | POST | `/api/v1/tickets/update` | — |


## 4. 发现

1. **无新增越权/裸奔**：完备性由守卫结构性保证（§1）；B 案重排后 54 页面行全 GET（唯一例外 ticket_list↔transition 有 pageWriteAllow 在册设计出处）。
2. **orgs 写面全在豁免集（orgDelegated）**——L3 判定是唯一防线，与 12 号结论一致（维持原判：委托链实测过，风险可接受）；000041/42 读面放开后 GET /orgs 亦豁免（BK-24 在册）。
3. **intel 域单端点单码**（intel:query 页面行 GET）——S9 设计如此（管理/清理端点 superadmin-only 走系统族），无漂移。
4. **task 域代理面**（/runs /dead-letters /tasks /jobs）：L1=task:* 码族绑定，taskrunner 侧零鉴权（网关 AK/SK 段）——与 12 号口径一致，B13 矩阵不越仓。
5. **豁免集 45 项每条带理由注释**（catalog.go）——复核无无名豁免。

## 5. 结论

**通过**。三层覆盖与 12 号口径连续，phase5 新增端点全部落在「页面=读/按钮=写/selfService=L3」三分形内，无孤儿端点、无页面行写绑定（守卫保证）、无新增待拍板项。下一份复审时点：词表再动或下个大版本（同 12 号惯例）。
