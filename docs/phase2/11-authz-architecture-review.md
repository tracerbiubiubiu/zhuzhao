# 三层权限架构评审与鉴权不变量（编码前决策记录）

> **文档定位**：本文档源自 2026-08-19 对 `docs/design/` 全部五份设计文档的系统评估（重点：architecture.md §4 鉴权体系、design-decisions.md §3/5/6/8/12、rbac-inheritance-and-cascade.md 全文），以及评审后与项目负责人的六点确认讨论。
>
> **性质**：评估结论存档 + 待落档决策清单。**design 目录下所有文档尚未修改**——落档动作见 §7 清单，等全部确认后执行。
>
> **服务对象**：Phase 2a 开工前的设计收口。若不变量（§3）不先收口，Phase 2a 编码时将转化为实现分歧。

---

## 1. 评估结论摘要

### 1.1 五维度评估

| 维度 | 结论 | 要点 |
|------|------|------|
| 层级划分合理性 | **优** | 与 OWASP / NIST SP 800-204B 分层鉴权对齐（PEP-1 边缘粗粒度 / PEP-2 Service 细粒度）；按判断成本升序短路；策略爆炸量化论证（300 万 → ~1,000 条）成立 |
| 职责边界清晰度 | **优** | 每层有精确的「回答的问题」定义；三条独立维度（功能权限/角色来源/数据范围）分开建模并有 6 产品业界对照；鉴权服务自身鉴权的循环依赖有数据源隔离解法 |
| 控制流程完整性 | **良** | 分期缺口（Phase 1 仅 L1）属计划内；但发现 **2 处文档内部不一致**（L2/L3 顺序矛盾、scope 枚举双轨），见 §2、§8 |
| 安全防护充分性 | **优** | Phase 1 经 12 项修复实证（令牌混淆/提权/TOCTOU/fail-close/审计断连）；前瞻注意点：allow-only 无显式 deny（Phase 2c 委托场景需评估）、属主短路前提会随工单流转漂移 |
| 业务需求匹配度 | **优** | 三层恰好映射工单三类权限问题；scope 三值对应主管-成员差异；跨部门派单有专门论证（父部门经理 ≠ 子部门自动扩权，与 AD/Keycloak 行为一致） |

### 1.2 与业界模型对比结论

| 模型 | 对齐度 | 说明 |
|------|--------|------|
| NIST RBAC | 高 | 角色继承不进 Casbin、应用层 BFS 展开，比 Keycloak composite 更可控 |
| XACML / NIST ABAC | 特例 | L3 仅单属性比对（creator_id），文档诚实自称「简单 ABAC」；PDP 演进有触发信号（§6） |
| Zanzibar ReBAC | 弱化实现 | design-decisions §12.2 主动承认「不是 ReBAC 引擎，是一条 SQL」——ltree 只覆盖组织树单种关系图 |
| 若依 data_scope | 子集 | 缺「自定义部门集合」级，Phase 2b 可按需补 |
| 混合模型（业界实态） | **高** | 纯模型不存在于大型系统，分层本质是按判断成本分流（AWS / GDrive 均为混合） |

**核心优势**：策略爆炸量化论证；对自身局限诚实并给出演进触发信号；三维分离有业界对照支撑。
**核心不足**：2 处文档内部不一致；**列表过滤与单点鉴权一致性**未成文（GetFilter 与 Authorize 若语义漂移 → 「列表可见、点开 403」或反向不一致）。

---

## 2. Q1：L2/L3 执行顺序（已拍板：L2 可见性在前，路径 A）

### 2.1 文档矛盾现状与决策

- [architecture.md §4（L558 附近）]原写：L1 → **L3 属主短路** → L2
- [design-decisions.md §3.4]原写：L1 → L2 → L3

两序语义不同：属主在前 = 属主豁免组织校验（转部门后仍能改旧工单）；组织在前 = 属主也必须在关系链上。

**决策（路径 A，对齐 Freshdesk/Jira 主流）**：取 **L2 可见性在前**。理由：项目引用的两个业界参考（[Freshdesk scope 模型](https://support.freshdesk.com/support/solutions/articles/97079-understanding-ticket-scope-and-agent-role)、[Jira SM permission + issue security](https://support.atlassian.com/jira-service-management-cloud/docs/manage-permissions-in-jira-cloud/)）**均无「属主短路」设计**——属主是可见性集合的一个特例（Freshdesk 的 assigned scope = 仅属主可见），不是独立短路层。属主豁免组织校验会弱化数据隔离，且「转部门改旧工单」可通过 scope 配置或重分派解决，不需以弱化隔离为代价。[02-authz-resource §2.3](./02-authz-resource.md#23-ticketresource) 的 `Authorize` 代码现状（`canRead` → `canOperate`）已符合此顺序，无需改动。

### 2.2 形式化语义

```
Allow ⟺ L1 通过 ∧ L2 可见性通过 ∧ canOperate 通过
```

- **L1 是必经门（合取项）**：角色无该 API 路由权限 → 直接 403，不进入资源级。属主不能绕过路由级——viewer 角色即使「拥有」某工单，调不了 update API 就是调不了
- **L2 可见性是数据访问边界门（合取项，先于属主）**：scope=all/group/assigned 判定。scope=assigned 的可见集即属主（对齐 Freshdesk）。不可见 → 404
- **属主不豁免 L2**：转部门后若 scope=group 且工单 `org_path` 不在新组织路径下，L2 fail，属主身份不短路。属主仅在 canOperate 内部用于决定动作权限
- **L3 属主降为 canOperate 的输入**：属主命中（`created_by`/`assigned_to` == uid）→ `read/comment/update/close` 可放行；`assign/delete` 属主也不放行，需 admin/scope 主管
- **拒绝点只有三个**：① L1 拒绝（路由级 403）；② L2 未命中（资源级 404）；③ L2 命中但 `canOperate` 未通过（动作级 403）。任何一层「无法判定」（DB 错误）也归入拒绝（见 §3 Q3）

### 2.3 转部门设计（实体部门 / 虚拟部门通用）

核心原则：**可见性以当前组织关系为准，属主不兜底**——

- L2 用当前组织关系（`org_path` ltree + `user_orgs`）判定可见性，转部门/组织 Move 后 `org_path` 变更即影响可见性
- **不设「属主豁免」兜底**：转出后若工单不在新组织路径下，L2 fail（404）。需保留访问的场景通过 scope 配置（如 scope=all）或重分派解决
- L3 属主判断只做资源行上的列比较（`created_by`/`assigned_to`），用于 canOperate 内部决定动作权限，不作为可见性豁免

两个注意点：

1. 工单的「属主」定义（仅创建人，还是创建人+处理人）以 09 PRD 为准，但 **Authorize 与 GetFilter 必须用同一个属主定义**，否则出现「单点能看、列表不可见」的反向不一致
2. 属主放行只解决「能做哪些动作」；工单关闭后能不能再改由状态机管（鉴权与状态机分层叠加，不混在一个判断里）

---

## 3. Q2 + Q3 + Q5：鉴权架构不变量（建议合并为一个不变量块落档）

三个缺口合并为 design 层的一个「鉴权不变量」小节（约 10 行 + 错误映射表），一次性解决。

### Q2：默认拒绝与合取/析取语义声明（优先级最高）

NIST / OWASP 要求鉴权失败必须 fail-closed。文档未明确「三层是 AND 还是 OR」——不写成公式，实现者可能写成「任一层通过即放行」（纯 OR）= **越权**。

落档内容：§2.2 公式 + 两处拒绝点 + 默认拒绝原则。

### Q3：资源级鉴权 fail-closed 行为

`Registry.Authorize` 若抛 DB 错误，Service 必须按拒绝处理。建议错误路径映射：

| 情形 | 行为 |
|------|------|
| 确定性拒绝（L2 未命中） | 403 |
| DB 错误、无法判定 | **503** + Error 日志（对齐项目既有 Redis fail-close 503 模式） |
| 未注册资源码 | **500** + Error 日志（编程错误，用 403 会掩盖代码缺陷） |

配套：phase2/02 测试用例补一条「DB 错误注入 → 拒绝」。

### Q5：缓存与鉴权一致性

声明不变量：**L2/L3 资源级判断不缓存，每请求实时查询**。Phase 3 的 perm 缓存仅覆盖 L1 的角色列表输入；任何资源级缓存提案必须连同失效级联方案评审：

| 变更 | 失效范围 |
|------|---------|
| org move | 子树全员 |
| 成员变更 | 单人 |
| scope 变更 | 该角色全员 |

理由：ltree 判定是一条索引 SQL（亚毫秒），缓存收益小、失效级联复杂度高——直接禁掉优于「允许缓存但要联级失效」。

---

## 4. Q4：职责分离（SoD）/ 约束 RBAC —— 延后决策

现状是 flat RBAC（角色→接口）。NIST RBAC 标准分 flat / hierarchical / constrained（含 SoD 静态/动态互斥）。

**结论：现在不建模，落一条「延后决策」记录。**

- 审批流 Phase 3 才来，届时真正的需求形态是**动态 SoD**（「不能审批自己发起的申请」——工作流规则）而非静态角色互斥（「不能同时持有审批人和申请人角色」——过粗且难配）
- 现在建互斥表是过度设计；schema 无需预留
- 但必须写明「延后 + 届时优先动态 SoD」，避免将来被当遗漏

---

## 5. Q6：ReBAC 转向触发场景清单

现有触发信号（design-decisions §12.5）偏技术侧，建议替换/扩充为业务侧触发表。**出现以下任一需求时启动评估**（2026-09-09 起本表带显式编号，**行序冻结**：禁止中间插行/重排，新增场景只许追加顺延——design-decisions §26.3/§26.4 按 #N 引用本表，样例 #1/#3/#5）：

| # | 触发场景 | 为什么 ltree + 内联撑不住 |
|---|----------|--------------------------|
| 1 | **跨资源关系链**：如「处理人 A 可见申请人 B 所在部门的全部工单」——权限要穿过 工单→人→部门→工单 的边 | ltree 只有一棵组织树；资源↔资源、人↔资源的图遍历表达不了 |
| 2 | **临时共享 / ACL**：「把这张工单共享给某外部人员一周」（GDrive 式） | 现模型无 per-resource 授权记录；手工加 ACL 表 ≈ 手写 relationship tuple，不如直接上引擎 |
| 3 | **策略运行时可配置**：管理员在 UI 配权限规则、不发版生效 | 引擎自带 schema/DSL；自研等于造规则引擎 |
| 4 | **多维组织**：项目线、产品线、部门三棵树同时参与可见性判定（AND/OR 组合） | 单棵 ltree 无法表达多维交集 |
| 5 | **微服务拆分后的统一 PDP**：N 个服务各自手写资源鉴权，语义漂移 | OpenFGA / SpiceDB 天然是中心化 PDP |
| 6 | **跨资源类型的「可见对象列表」成为核心功能** | ReBAC 有 ListObjects；手写多表 UNION + ltree 条件很快失控 |

**反触发**（现状，维持延后正确）：单一组织树、资源类型 ≤5、无共享需求、策略随代码走、单人团队。**补充评估（2026-09-03，design-decisions §25.4）**：§23 重定位后的新场景（统一网关 / taskrunner / activelist）逐一评估，**无一构成演进触发**——网关=路由级 RBAC（Casbin 已满足）、taskrunner=成员 EXISTS（策略库 `org-member`）、activelist=行级自治（Zanzibar 系要求关系集中注册，与独立库决策正面冲突）。授权关系是树形（ltree），PG+SQL 判定有一致性/索引/可测试优势。

**隐藏成本提醒**：上 ReBAC 不只是加一个服务——org move、成员变更、角色变更全都要双写到关系 tuple，这条同步管道才是长期负担，这正是现在不上的理由。迁移路径已预留（`ResourceAuthorizer` 接口换实现），无沉没成本。

---

## 6. 顺带发现的第 2 处不一致：scope 枚举双轨

（评审发现的另一处，未在六点确认范围内，一并记录）

- architecture.md §4.3 泛化模型：`org_permissions.scope ∈ {1 本组, 2 本组及子组, 3 本组及父级链}`
- Phase 2 落地模型：`roles.ticket_scope ∈ {all, group, assigned}`

scope=3（向上访问）在工单模型中无对应值，两套枚举缺映射表。§4.3 有 Phase 注释区分了路径，但枚举语义漂移未收口。**建议补映射表并决策 scope=3 是否保留（或标注 Phase 2b+ 泛化路径专用）。**

---

## 7. 待落档清单（当前全部未执行，等确认后操作）

| # | 内容 | 落点 |
|---|------|------|
| 1 | 鉴权不变量块（Q1 公式 + Q2 默认拒绝 + Q3 错误映射 + Q5 缓存禁令） | architecture.md §4.1 |
| 2 | L2/L3 顺序统一为「L1 → L2 可见性 → L3 属主」（路径 A，对齐 Freshdesk/Jira，见 §2 已拍板）+ 顺带修 §12.7 已声明要改的「ReBAC」旧称 | design-decisions.md §3.4 |
| 3 | ReBAC 业务侧触发表（替换/扩充现有四条技术信号） | design-decisions.md §12.5 |
| 4 | SoD 延后决策一句话（延后 + 届时优先动态 SoD） | design-decisions.md §14 或本计划 P2-D6 |
| 5 | scope 枚举映射表（§6） | architecture.md §4.3 |
| 6 | DB 错误注入 → 拒绝 测试用例 | phase2/02-authz-resource.md 测试用例节 |
| 7 | 本批决策挂入编码前决策清单（P2-D6：L2/L3 顺序 + 不变量；P2-D7：SoD 延后） | 00-implementation-plan.md §1 |

---

## 8. 与 00-implementation-plan.md 的衔接

本批内容建议编号：

- **P2-D6**：L2/L3 执行顺序 + 鉴权不变量 —— 状态：**方向已确认**（本文档 §2/§3），待正式拍板后写入 00 计划 §1 并执行 §7 落档
- **P2-D7**：SoD 延后决策 —— 状态：建议已给出（本文档 §4），待确认

> 截至本文档创建时，00-implementation-plan.md §1 仍为 P2-D1~D5，未写入 D6/D7（等确认）。

---

## 9. 2026-09-08 复核：OPA / ReBAC 迁移再评估（结论：维持不迁移）

**背景**：项目所有者提问「当前实现是否需要迁移至 OPA 或 ReBAC」。经文档（§5 触发表、design-decisions §12 / §25.4）与代码（`internal/` 三层鉴权实现）双线独立复核，**维持 §5 结论：不迁移、守触发表**。本节为对既有决策的再核验记录，非新决策。

### 9.1 复核理由摘要

- **OPA 不对口（形态层面）**：本项目核心判定是「在 SQL 里生成行级 WHERE」（ltree 锚点 + ticket_scope 三轴 + 委托子查询拼入查询），OPA 是布尔决策引擎、不生成 SQL——引入后 L2 过滤逻辑仍须留在 Go/PG 侧，等于策略写两份（Rego 一份、SQL 一份）。行级 PEP 必须在数据处（design-decisions §5 已论证，Zanzibar/K8s/AWS/OPA 无一例外）；OPA 的典型场景（admission control、无状态条件判定）本项目不存在。proposal 层对照表结论不变：「OPA | 关系遍历不擅长」。
- **ReBAC 不经济**：当前授权关系 = 单棵组织树（ltree）、资源类型 ≤5（挂 L2 仅 ticket）、关系链深度 2–3，§5 触发表六条**零命中**（含 2026-09-03 §25.4 补评估）。迁移真实成本不在跑一个服务，而在 **org move / 成员变更 / 角色变更的 tuple 双写同步管道** + 与业务事务失去同库一致性 + 与 activelist「行级跟数据走、关系不集中注册」的设计正面冲突（§25.4 原话）。
- **退路已铺**：`ResourceAuthorizer`（Check + ListFilter）接口 + Builtin 一行注册自 Phase 1 即在；命中触发表时换判定后端、L1 不动、无沉没成本。届时选型 OpenFGA（API 友好）优先，需 Zanzibar 级一致性再评估 SpiceDB（design-decisions §12.5 / §25.4）。

### 9.2 本次复核认为最现实的两条触发器

1. **跨资源关系链**：「处理人 A 可见申请人 B 所在部门的全部工单」类穿图判定（工单→人→部门→工单 的边，ltree 单树表达不了）；
2. **per-resource 临时共享 / ACL**：GDrive 式「把这张工单共享给某人一周」——手工加 ACL 表 ≈ 手写 relationship tuple，不如直接上引擎。

多维组织（第二棵树参与可见性）次之；「微服务拆分后统一 PDP」可不盯（微服务不拆已定，design-decisions §23 生态基调）。

### 9.3 代码侧清点出的三件权限债（均不需要外置引擎，迁移不解决它们）

| # | 债 | 代码证据 | 归口 |
|---|----|---------|------|
| 1 | **Casbin 无自动重载**：多实例下 AssignMenus 后其他实例策略陈旧依赖手动触发（失败窗口 = DB 已生效、内存陈旧） | `internal/casbin/enforcer.go` 仅启动期 LoadPolicy、未调 `StartAutoLoadPolicy`；cleanup 中 `StopAutoLoadPolicy()`（enforcer.go:35）为死代码 | 既有归口 **W1 多实例基座**（phase3/02-multi-instance.md，redis-watcher + StartAutoLoadPolicy 移植 eiam `ioc/casbin.go`）；实施时顺带清死代码 |
| 2 | **L1 权限缓存缺失**：每请求至少一次角色 BFS 递归 CTE + 工单每请求 ResolveScope，当前最实在的性能债（现仅请求级缓存，BK-17） | `casbin.go` 每请求 GetEffectiveRoleCodes | 既有归口 **phase3/09-platform.md**（`perm:user:{userId}` + Pub/Sub 失效）；实施须守 Q5 禁令——只许盖 L1 输入，L2/L3 保持实时 |
| 3 | **IW4 护栏未泛化**：fail-closed 哨兵 + AST 守护仅覆盖 ticket_repo 一处，「漏接 GetFilter 静默全量」对未来新资源/导出功能仍开放 | `ticket_repo.go` 入口哨兵 + `TestGuard_TicketRepoListCallSites` 只锁 ticket 包 | 新登记 **BK-21**（触发驱动，随首个新资源接 L2 / 导出功能实施；详见 phase2/00 §9、review/11 §6/§8 B12） |

> 附注（非债，设计内取舍）：Builtin 策略库零生产消费者 = §25.5 触发驱动降级的预期状态；task/jobs 走 L1 + 参数级组装（E-⑤）同理。`HasOrgManagePermission` 的 `LIKE 'org:%'` 过授权约束已落代码注释（P2-2），维持现口径。
>
> 护栏兜底预案（2026-09-08 注记）：若 BK-21 泛化后仍出现漏调事故，再评估 PG RLS 作 DB 层兜底（业界蓝本 Supabase；代价 = 谓词进 SQL 双维护 + Go 侧可测性降级）——定位触发驱动，现在不做。
