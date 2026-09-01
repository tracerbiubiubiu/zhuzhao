# 11 · 项目能力总览图（Project Control）

> **用途**：一张地图快速掌握整个项目的能力、关键细节与当前健康状态，用于对 AI 快速迭代保持掌控。**每次代码改动后应同步更新本文**（见 `AGENTS.md`）。
>
> 更新日期：2026-08-31 ｜ 分支：`feature/phase-2` ｜ 文档体系见 [docs/roadmap.md](../roadmap.md)

---

## 0. 项目一句话

Go 编写的**模块化单体 IAM + 工单系统**：三层鉴权（路由 RBAC → 资源级 ltree scope → 属主/委托）+ 资源级工单业务，单实例 Docker Compose（PG + Redis）部署，文档驱动开发（docs/ 与代码强同步）。

**技术栈**：Go + Gin + pgx + Casbin + Wire DI + Redis + PostgreSQL（ltree）+ Docker Compose。

**规模**：70 个非测试 Go 源文件 ｜ 47 个测试文件 ｜ 190 个测试函数 ｜ 32 个迁移文件（16 对）｜ 13 份 review 文档（含本文件；统计时点 2026-08-31，随代码漂移）。

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
- **附件**（file_objects/ticket_attachments）— 2b-ext 延后，迁移编号规划 000017（归属已拍板：谁先启动谁占用、后者重排，见 §8 A2）
- **Phase 3 全部**：可观测性 / 多实例 / 审计 L2 / 高可用 / 安全增强 / ops / **前端工程** / 工单业务（SLA/通知/审批流/分派/报表）/ activelist 集成 — 暂缓；执行结构 = **Wave W0–W4**（README §2.1.0，2026-08-31 确认：W0 门禁 → W1 基座 → W2 工单业务 → W3 加固 → W4 activelist）。文档就绪：已编写 01 / 02-multi-instance / 10（含 7-0 决议）/ 11 / 12-frontend，待编写 5 份（03/06/07/08/09，见 §8 B9）
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
| **TC1** | delete 成功路径断言 | 🟡 **脚本层已覆盖**（2c 脚本 SAT 建单→删→GET 404）+ 委托删有 Go 测试（vg owner/vg admin/ancestor owner）；**Go 层全局 admin 删单成功测试仍缺** → 11 §8 A7 |
| **TC2** | 缺 relation 集成测试 | ✅ **已补**（`TestD9_CreateRelation`：正向 / 同向 409 / 删后建联 400） |
| HC2 | Delete 无 "deleted" 事件 | ✅ **已修复**（000014 SET NULL + Delete 同事务写 deleted 事件，随库存活；回归断言通过） |
| EC1 | Swagger 未重新生成 | ✅ **已修复**（orgs/owners 等 2c 端点已入 docs.go/swagger.json） |
| **OP1** | org_path 快照竞态（Create×Move 并发 → 旧 path 快照） | ✅ **已修复（BK-11 ①，2026-08-31）**：`OrgRepo.FindByIDForShareTx` 事务内 FOR SHARE 锁 org 行 + `ticket.Service.Create` 重构（org 读取移入事务）；回归：锁窗口阻塞验证 + Move×Create 锤击（变异验证去锁必失败）。② 已拍板（2026-08-31）保留镜像列，JOIN 备选弃用 |
| **BK-12** | `org_roles` / `roles.parent_id` 写侧管理接口缺失 | ⏳ **触发条件驱动**（① 真实进组赋角色诉求；② 2b-ext HR 同步启动 = 硬触发器）；机制符合业界实践，当前 fail-inert；详见 00 §9 |
| **BK-13** | 工单可见性默认方向：兄弟虚拟组透明可读 vs 业务要求「默认只看自己 + 可配置」（`project_isolated`） | 🔶 **已触发（2026-08-31 用户多虚拟组场景），待批准实施**（~0.5–1 天：约束 + 配置 API + D12 测试）；机制骨架已埋、默认值不动；详见 00 §9 |
| **BK-14** | 成员 scope（`ticket_scope`）无配置面（AddMember 仅收 org_member_role，无任何 API 写 scope） | 🔶 **已批准登记（2026-08-31）**：关系派拍板（scope 挂成员关系非角色）+ scope=all 仅全局管理员可授；与 BK-13 同批实施（~0.5 天）；详见 00 §9 |
| **BK-15** | 代码卫生：UpdateAssignedTo 死包装 + Delete/UpdateAssignedToTx 叠注释（旧「admin only」过时） | ✅ **已修复（2026-08-31）**：删死代码 + 注释合并更正 |
| **BK-16** | ~~Delete 不校验 RowsAffected~~ | ⚪ **误报关闭（2026-08-31）**：校验自 Phase 2a 即存在（66e2c39），审计读码截断所致；详见 00 §9 |
| **BK-17** | 角色展开每请求双查（中间件 + service 各一次 BFS SQL） | 🔶 **已登记（2026-08-31），随 IW1 批次修复**（request context 透传缓存）；详见 00 §9 |
| **BK-18** | 类型/字段/模板管理闭环（只读 API，增删改仅 SQL；custom_data 无 schema 校验） | 🔶 **已登记（2026-08-31，用户确认需求），IW1 后批次实施**（后端 2-3 天 + 前端 4-6 天，eflow 范式）；详见 00 §9 |

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

## 8. 遗留问题分类：Phase 3 前置 vs 随行（2026-08-31 整理）

> 本节取代原「下一步」清单，把全部已知未决项归入两档：**A 档 = Phase 3 启动前/启动时完成**（门禁与拍板，不做会让启动本身踩坑）；**B 档 = 随 Phase 3 对应子能力一起**（提前做无收益）。代码级 backlog 详情见 [phase2/00 §9](../phase2/00-implementation-plan.md)。**Phase 3 启动时从 [phase3/00-startup-checklist.md](../phase3/00-startup-checklist.md) 进入检查流程**（本节 + §6 是其数据源）。
>
> **Phase 3 执行结构（2026-08-31 确认）= Wave W0–W4**（详见 [phase3/README §2.1.0](../phase3/README.md)）：**W0** 启动门禁（本节 A 档 + 检查单 IW1/IW3）→ **W1** 可运维基座（Step 1/2/3；[02-multi-instance](../phase3/02-multi-instance.md) 已编写，含 Casbin Watcher 移植方案）→ **W2** 工单业务（Step 7；[10-ticket-business](../phase3/10-ticket-business.md) 已含 7-0 决议，前端规格见 [12-frontend](../phase3/12-frontend.md)；本节 B 档为其随行项）→ **W3** 加固收尾 → **W4** activelist 集成。

### A 档：Phase 3 启动前/启动时完成

| # | 事项 | 说明 | 量级 |
|---|------|------|------|
| A1 | **文档修正包** | ① phase3 README §1.4 前置矛盾（2b-ext 延后项列为「2b 验收」前置）、§5 状态行（10/11 已编写仍标待编写）；② **review/10 C1–C4 处置**（2026-08-31 逐条核验）：C1「Step 9 应为 8-10」三次裁定驳回维持（wontfix，注记防再犯）、C2 `NewResource` 三参签名已修（注记闭环）、**C4 §2.2 ScopeResolver 接口段与现行实现不符（真开放）**——按 `ReadAnchorPaths`/`ResolveScope` 重写、C3 `docs/ops/deployment.md` 缺 → 归 B10；③ 09 合集抽查回注：F-01/02（VISION 措辞/迁移现状）、F-03（activelist 链）、F-18（Redis requirepass 经 compose 注入）经核实**均已修**（09 行未回标属历史文档常态，此处记录防重复排查） | 半天 |
| A2 | **迁移编号 000017 归属拍板** | 2b-ext 附件（phase2/00 §Step 6）与 Phase 3 SLA（10-ticket-business §2，占用 000017–000021）都规划 000017。规则建议：**谁先启动谁占用，后者启动时整体重排**——✅ **已拍板（2026-08-31）**：谁先启动谁占用，后者整体重排 | 已关闭 | 决策 |
| A3 | **BK-11 ② 数据结构拍板** | `tickets.org_path` 保留镜像列（FOR SHARE 已兜底）vs 去列改运行时 JOIN + write-once `created_org_id`；已登记 [phase3/README §4](../phase3/README.md)，✅ **已拍板（2026-08-31）**：保留镜像列；`created_org_id` 留 Step 7e 按需 | 已关闭 | 决策 |
| A4 | **HC1：comment/note 补 ticket_events** | 事件流是 Step 7 SLA/通知/报表的统一输入，补全属地基（唯一遗留的 §6 未修项）；service 层两处 + 事件常量 + 测试 | ~半天 |
| A5 | **BK-5：relation 反向判重** | DB 唯一索引只挡同向，A→B 与 B→A 可共存；报表/SLA 引用关联数据前收口数据质量（应用层 EXISTS 检查） | ~1–2h |
| A6 | **散落决策落档与断链收口**（2026-08-31 全量扫描 phase1/2/3 新发现） | ① **SoD 延后决策未落档**：11-authz-architecture-review §4 已给结论（「延后 + 届时优先动态 SoD」）但从未写入 design-decisions；且其建议编号 P2-D7 已被 00 计划的三轨拆分复用——落档时用新编号并注明别名；② **phase2/12·13 号断链**：14 号文档 5 处引用 `12-phase1-backlog-and-phase2-review.md`、`13-project-plan-multi-round-verification.md`、`13-plan-remediation-actions.md`，文件从未入库（git 历史无删除记录）——修正引用为实际归宿（review/09 等）或加「已并入」注记；③ review §7 item6「DB 错误注入 → 拒绝」测试用例未落（顺手项） | ~1h |
| A7 | **TC1-Go：补全局 admin 删单成功集成测试** | 现有 Go 覆盖为委托删（vg owner/vg admin/ancestor owner）；全局 admin bypass 删单成功仅 2c 脚本覆盖（SAT 建删删）——补 Go 层测试进 `make test-integration` 基线（CI 可跑）。AGENTS.md 遗留节已同步校准（TC2/HC2 标已修） | ~15min |

> 其余待决策点（K8s vs Compose、Redis/PG HA、部署级分离时机、审批流引擎选型）已在 [phase3/README §4](../phase3/README.md) 维护，启动时逐项过表，此处不重复。

### B 档：随 Phase 3 对应子能力一起

| # | 事项 | 随哪个子能力 |
|---|------|-------------|
| B1 | **Step 7 设计期拍板清单**：① SLA 暂停态语义、通知「主管」定义、邮件通知矩阵（原评估 B1/B3/B6）；② **10 号文档设计深度缺口（2026-08-31 外评证实）**：§2.5「标记+Enqueue 同事务 vs 只 Enqueue」二选一（SLA 正确性核心）、§7.2 signal 双写二选一、responded_at 是否含内部备注、`min_level` 职级数据源（users 无 level 列，悬空）、§5 分派深度（keyword 算法/同优先级/target 从属/无命中兜底/Hook 事务边界）、§6 报表深度（权限码/缓存失效/指标口径/分页）、§8 TB 负向用例（§2.5 四必坑 0 覆盖）；③ **2026-08-31 生态调研吸收**：发起人撤回（Revoke）业务设计缺失（eflow WITHDRAWING 栅栏模式可对标）、`workflow_definitions` 版本/发布快照（编辑版/发布版分离）、审批人策略模型 `Assignee{rule,values}`（部门领导/分管规则替代职级，解 min_level 悬空） | Step 7 设计期逐项拍板 |
| B2 | 权限码 seed：ticket:approve / notification:* / workflow:manage 均未设计 | Step 7（与 7c/7d 同步设计 + seed 迁移） |
| B3 | in_progress / pending_verify 状态推进端点（BK-10 已拍板归 Phase 3） | Step 7 工单业务深化 |
| B4 | BranchedStateEngine 引擎本体 | Step 7c（**硬交付**；触发信号只决定流程定义数量，见 phase3/README §0）。设计期消费 A6 的 SoD 延后决策（审批流互斥优先**动态 SoD**） |
| B5 | BK-11 ② 实施（去列 JOIN 或保留快照的落地） | 随 A3 拍板结果，在 Step 7 动工前实施 |
| B6 | BK-12：org_roles / parent_id 写侧 | **不自动随 Phase 3**：触发器 = 2b-ext HR 同步启动或真实诉求（见 00 §9） |
| B7 | CORS AllowAll 转轨收紧（09 合集 F-21） | Step 5 security-enhance + 上线检查单 |
| B9 | 待编写 **5 份**：03-audit-l2（W1 前）/ 06-ha、07-security-enhance、08-ops+deployment.md（W3 前）/ 09-platform（L2 时）；**已编写（2026-08-31）**：01 / 02-multi-instance / 10（7-0 决议已入）/ 11 / 12-frontend；另 10 号待 7-0 细节修订 | 随启动的子能力/Wave 编写；**W2 以 W1 为硬前置**（2026-08-31 已确认，README §2.1.0）；**参考实现**：02 的 Casbin Watcher 直接移植 eiam `ioc/casbin.go`（redis-watcher + StartAutoLoadPolicy 双保险）、Asynq 任务建模仿 etask（RetryConfig 指数退避/补偿器/job 化 Wire 注册） |
| B10 | `docs/ops/deployment.md` 补编写（ops 骨架 README 已在，review/10 C3） | 随 Step 6 ops / 部署文档批 |

### 独立窗口（已触发，Phase 2 范畴，不属于 Phase 3 前置或随行）

| # | 事项 | 说明 |
|---|------|------|
| W1 | **多虚拟组可见性场景闭环：BK-13 + BK-14** | 用户场景触发（2026-08-31）：aa 看全子树 / bb 默认只看自己组 / scope 可配置放宽。aa 看全部、个人级放宽已可用；**BK-13** 交付「默认收紧」开关（CHECK 约束 + org update 配置 API + D12 测试，~0.5–1 天）；**BK-14** 交付成员 scope 配置面（AddMember 扩展 + scope 变更端点 + scope=all 仅全局管理员可授，~0.5 天）——bb「看自己组全部」依赖 BK-14 配 scope=group，**两者同批实施、同一场景验收**（合计 1–1.5 天）。详见 00 §9 |
| W2 | **2b-ext 三件**（附件 / auth-enhance / HR 同步） | 按需独立启动；附件若先于 Phase 3 启动 → 触发 A2 拍板；HR 同步启动 → 触发 BK-12（B6） |

> **随手项（任意时点）**：BK-9（测试死代码清理）、A6 ③（错误注入测试用例）、09 F-31④（relation 越权负向用例——现有 `TestD9_CreateRelation` 只覆盖正向/同向 409/删后 404）、09 F-32（audit/user service 分支级单测，低优——集成已兜底核心分支）。
