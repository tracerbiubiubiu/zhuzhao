# 生态审计 B09：临时成员过期与组织委托（2026-11-05）

## 范围与基线

- 受审快照：主仓 `62e40244b1667e47c3f729250b36c5edcbf445aa` 的独立 git archive；业务实现仍为原基线 `5c03581d5999139a3e8e5ead5972c026868aa181`，其间七笔只涉及审计记录、规范及验收工具。
- 窗口限定为成员有效期在组织委托、工单 L2/L3、列表过滤及相关成员归属查询中的贯彻；不代表 IAM 全面审计完成。
- 设计输入：`docs/phase2/03-org-enhance.md` 的临时成员能力、`docs/phase2/09-ticket.md` 的 T10、`docs/phase2/04-org-delegation.md` 的组内委托和独立 owner 权源。04 号的历史 SQL 同样漏写有效期，属于应随实现一并校准的设计镜像。
- 动态环境：自有 testcontainers PostgreSQL 15，真实迁移、组织/用户仓储、RBACService、OrgService、Ticket Resource；测试进程结束已清理临时 PG。没有连接 dev/test 常驻库，没有使用或修改 E000001。

## B09-F01 / P0：过期成员仍保留组织委托权限

**状态：已复现，修复与独立复验进行中，尚未关单。**

前提是 `user_orgs` 存在带非空 `expires_at` 的组内 admin/owner，时间自然到期。当前管理端请求本身没有设置有效期的字段；本发现针对项目已设计并支持读侧判定的存量临时成员数据，不声称普通用户能通过现有请求任意设置该列。显式 `organizations.owner_user_ids` 是另一个权源，不能因为成员行过期而一并撤销。

`scope_resolver.go` 的 `resolve` 与角色 BFS 已过滤过期行，但以下委托路径没有过滤。同一用户因此已无透明读锚点，却继续获得委托授权，直到关系/角色被显式移除。

| 锚点（均属于上述受审 commit） | 缺口与影响 |
|---|---|
| [org_delegation.go：EffectiveOrgPriority](../../internal/service/org_delegation.go:35) | LEFT JOIN 不检查成员有效期，过期 admin 仍为 priority 10；`ensureCanManageMember` 继续允许成员管理。 |
| [org_delegation.go：IsOrgAdminOrOwner](../../internal/service/org_delegation.go:56) | EXISTS 不检查有效期，名册读取及工单委托继续命中。 |
| [ticket/resource.go：delegatedVisibilitySQL](../../internal/service/ticket/resource.go:193) | 列表委托 SQL 同样命中过期成员；只修 Go 判定会留下列表读泄漏。 |

原快照确定性交错以外的普通串行验证，连续 **3/3** 得到同一结果：

1. 仅保留一条组内 admin 成员行，将其有效期设为数据库当前时间前一分钟；该用户不在 `owner_user_ids`，也没有全局组织管理权限。
2. 真实 scope resolver 返回空路径，但 priority=10、`IsOrgAdminOrOwner=true`。
3. `ListMembersRoster` 返回名册；`RemoveMember` 成功移除了另一普通成员。
4. 对另一用户创建的组内工单，真实 Resource 的 read/update/delete 均返回 allowed=true；真实列表 Filter 在 PG 中仍返回该工单一行。

以上直接证明 L2/L3 与组织委托层失效。工单每个 HTTP 动作仍受其各自 L1 Casbin 绑定限制；本批没有把 Resource 的放行夸大为所有角色都能调用所有写路由。名册/成员管理的自服务委托入口应在组合验收继续覆盖。

## 同类扫描与修复挂点

- `OrgRepo.IsInOrgBranch` 的建单归属判断也没有检查成员有效期；应与委托同步收口。[org_repo.go：IsInOrgBranch](../../internal/repository/org_repo.go:95)
- `OrgRepo.IsMember` 的通用成员判定与 `AssignmentRuleRepo.UserBelongsToOrg` 的规则写侧目标校验应采用相同的有效成员条件；后者不等于既有分派规则执行时持续重验全部身份。[assignment_rule_repo.go：UserBelongsToOrg](../../internal/repository/assignment_rule_repo.go:101)
- 角色 BFS、`HasOrgManagePermission`、scope resolver 及通用 `org-member` 资源过滤已经有有效期条件，保留其语义。
- 名册/管理列表仍可展示存量关系，不能把“展示关系行”与“授予有效成员权限”混同；本批授权修复不自动清除存量数据或更改 owner 数组。

修复建议：所有基于成员关系的授权查询统一要求 `expires_at IS NULL OR expires_at > NOW()`，并校准 04 号历史 SQL。覆盖永久/未来/已过期/边界时刻的 admin 与 member-role owner，单独保留显式 owner 数组的正向对照；同时检查单条授权与列表 SQL，避免只修一侧。移除每处修复后，针对性回归须连续八轮稳定失败。

## 验证证据与限制

原始证据按所有者要求仅本地保存，不纳入 Git/Docker：`.codex/audits/ECO-20261104/evidence/ECO-20261104-b09/`。

- `expired-delegation-baseline.log`：原始快照三轮实际结果及失败断言。
- `baseline-archive/internal/service/expired_delegation_probe_integration_test.go`：精确探针源；仅在实验归档中，不进入受审源仓编译。
- 修复提交、正常门禁、移除修复变异、固定提交独立重建及最终部署状态待后续增补；当前报告不以候选绿灯代替关单证据。

本批不新增迁移、不改变成员表数据。发现影响健康状态，控制台由主审统一回填；未完成的全角色 HTTP 矩阵及其他 IAM 模块继续按整体执行卡审计。
