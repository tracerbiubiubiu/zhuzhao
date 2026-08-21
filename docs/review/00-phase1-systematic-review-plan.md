# 系统性多轮代码检查计划（Phase 1 代码库全量）

> **制定日期**：2026-08-21
> **执行方式**：AI 检查 agent 编排（主会话统筹）+ 项目负责人人工复核并行推进
> **检查范围**：`internal/` 全部 Go 源码（约 50 文件）↔ `docs/` 五组文档体系（phase1 / modules / api / design / proposal）
> **定位**：继 [11-code-review.md](../phase1/11-code-review.md)（单轮审查，12 项已修复）与 [12-phase1-acceptance-report.md](../phase1/12-phase1-acceptance-report.md)（验收核查）之后的**第三轮系统性全库检查**，目标是零死角覆盖 + 结果交叉验证。

---

## 1. 检查目标与三大维度

每轮、每个 agent 均执行以下三个维度，缺一不可：

| 维度 | 编号 | 检查内容 | 方法 |
|------|------|----------|------|
| **文档一致性** | D1 | 代码实现与文档是否完全一致：API 路由/方法/参数/响应结构/错误码逐项比对；Service/Repo 接口签名；文档承诺的行为（事务、级联、排序、过滤）是否真实实现 | 逐接口「文档 ↔ handler ↔ service ↔ repo」四层对照 |
| **占位实现** | D2 | 空函数、空方法、返回 nil/假数据的 stub、TODO/FIXME/`panic("not implemented")`、注册但无实现的路由、写了文档但完全没有代码的功能 | 全量扫描 + 语义判断（区分「计划内预留」与「遗忘实现」） |
| **逻辑/边界/性能** | D3 | 逻辑错误、边界条件处理不当（nil/空列表/分页越界/并发竞争）、错误处理缺陷（吞错、fail-open）、性能隐患（N+1、无界查询、循环内查库）、并发安全 | 逐函数走查关键路径 + 并发场景推演 |

---

## 2. 严重程度分级标准

| 级别 | 名称 | 判定标准 |
|------|------|----------|
| **P0** | 阻断 | 安全漏洞可直接利用 / 数据损坏或丢失 / 核心功能不可用 |
| **P1** | 严重 | 逻辑错误导致错误行为 / 权限校验缺陷 / 文档与代码实质契约违背（前端会写错） |
| **P2** | 一般 | 边界条件处理不当、性能隐患、非关键文档不一致、防御性缺失 |
| **P3** | 建议 | 可读性、命名、注释、轻微冗余、风格 |

问题类型标签：`DOC`（文档不一致）/ `STUB`（占位实现）/ `LOGIC`（逻辑错误）/ `EDGE`（边界条件）/ `PERF`（性能）/ `ERR`（错误处理）/ `SEC`（安全）。

---

## 3. 模块优先级矩阵（检查顺序依据）

| 优先级 | 模块 | 涉及代码 | 风险理由 |
|--------|------|----------|----------|
| **P0** | 认证链路 | auth_service / session_revoke / pkg/jwt / middleware/jwt / pkg/redis(lua) / pkg/crypto | 全部安全的根基；RT 轮换、登录锁定、会话吊销任一缺陷即全库失守 |
| **P0** | 授权链路 | authz_service / rbac_service / middleware/casbin / casbin/enforcer / pkg/resource | 权限旁路风险；deny-by-default 不变量必须成立 |
| **P1** | 用户模块 | user_service / priority / user_repo / user_handler | 提权风险（优先级防提权、乐观锁、软删除） |
| **P1** | 组织模块 | org_service / org_repo（ltree、move 级联） | path 级联错误 → 静默丢工单（2b scope=group） |
| **P2** | 角色+菜单 | role_* / menu_* | 契约近期已修一轮，做回归确认 |
| **P2** | 基础设施+审计 | app/wire / router / config / middleware 其余 / pkg(response,errcode,validate…) / audit_* | 横切影响面广；审计断连丢失已修，回归确认 |

---

## 4. 多轮递进流程

```
R1 核心安全（3 agents 并行）──认证链路 │ 授权链路 │ 用户模块
        ↓ 汇总
R2 业务与基础设施（3 agents 并行）──角色+菜单 │ 组织模块 │ 基础设施+审计
        ↓ 汇总
R3 交叉验证（主会话执行）──P0/P1 逐条复核 │ 重叠区域冲突裁决 │ 误报剔除
        ↓
R4 汇总报告（主会话执行）──去重合并 │ 与历史发现对照 │ 产出 01-phase1-systematic-review-findings.md
```

递进逻辑：先安全后业务再横切——R1 发现的鉴权不变量问题会改变 R2 部分检查点的判定基准；R3 由主会话（非 agent）亲自读码复核，避免 agent 幻觉；R4 只收「已确认」发现。

---

## 5. Agent 配置表

| Agent | 轮次 | 检查范围（代码） | 对照文档 | 维度侧重 |
|-------|------|------------------|----------|----------|
| A1 认证 | R1 | auth_service.go、session_revoke.go、auth_handler.go、middleware/jwt.go、pkg/jwt/、pkg/redis/（含 login_lock.lua）、pkg/crypto/、model/token.go | phase1/02-auth.md、modules/auth.md、api/errcode.md（20xxx 段）、proposal/auth-design.md | D1+D3（token 语义/锁定原子性/吊销传播） |
| A2 授权 | R1 | authz_service.go、rbac_service.go、middleware/casbin.go、casbin/enforcer.go、pkg/resource/registry.go、handler/errors.go、router.go（权限绑定段） | phase1/03-authz.md、modules/authz.md、phase2/11-authz-architecture-review.md（§3 不变量）、design/architecture.md §4 | D2+D3（fail-closed/逐角色 enforce/g 表消除） |
| A3 用户 | R1 | user_service.go、priority.go、user_handler.go、user_repo.go、pgerr.go、model/user.go、user_request.go | phase1/04-user.md、modules/user.md、phase1/10-concurrency.md | D1+D3（防提权/乐观锁/唯一冲突映射） |
| B1 角色菜单 | R2 | role_service.go（rbac_service.go 角色段）、menu_service.go、role/menu handler+repo、model/role.go、menu.go、role_request.go、menu_request.go | phase1/05-role.md、07-menu.md、modules/role.md、menu.md | D1+D2（AssignMenus 事务/删除保护/权限码 SSOT） |
| B2 组织 | R2 | org_service.go、org_handler.go、org_repo.go、model/organization.go、org_request.go | phase1/06-organization.md、modules/organization.md、design/rbac-inheritance-and-cascade.md | D3（ltree path 级联/循环依赖/双入口单写） |
| B3 基础设施 | R2 | app/、router/、config/、middleware 其余（audit/body_limit/cors/logger/recovery/security）、pkg/response、errcode、validate、jsonutil、postgres、logger、audit 全链路 | phase1/01-infra.md、08-audit.md、09-middleware.md、api/response.md、api/errcode.md、modules/audit.md、middleware.md | D1+D3（envelope 唯一出口/优雅关停/审计不丢） |

---

## 6. 交叉验证机制

1. **主会话逐条复核**：R1/R2 全部 P0/P1 发现由主会话亲自读码验证（不接受 agent 单方面结论）；P2/P3 抽查约 30%。
2. **重叠区域双覆盖裁决**：router.go（A2/B3 都查）、middleware/jwt.go（A1/B3）、model/user.go（A3/B1 权限码）等重叠区，发现冲突时以「代码证据 + 文档原文」裁决，仍不确定的降级为「待人工复核」。
3. **置信度标注**：agent 每条发现自带置信度（高/中/低）；低置信度且主会话无法确证的 → 状态「待人工复核」，不进 P0/P1。
4. **历史对照防重复**：与 11-code-review.md 的 12 项已修复发现、近期提交（树组装/菜单按钮/响应契约）对照，已修复项不重复报告；确有回归的标记「回归」。
5. **误报剔除规则**：剔除必须给理由（如「文档实际写的是 X，agent 误读为 Y」，附文档行号）；被剔除项记入报告附录，保证可追溯。

---

## 7. 检查报告模板

### 7.1 单条发现模板（agent 与汇总报告通用）

```markdown
#### [R1-AUTH-01] P1 / DOC：RT 轮换未校验旧 token 状态
- **位置**：internal/service/auth_service.go:L120-L135
- **问题描述**：（现象 + 影响，1-3 句）
- **证据**：
  - 代码：`（关键片段，≤10 行）`
  - 文档：docs/phase1/02-auth.md §x.y（引用原文一句）
- **改进建议**：（具体可执行的修改方向）
- **置信度**：高
- **状态**：待确认 / 已确认 / 误报已剔除 / 待人工复核
```

### 7.2 汇总报告模板（R4 产出）

```markdown
# 系统性代码检查发现报告（docs/review/01-phase1-systematic-review-findings.md）
## 1. 执行摘要（检查范围、覆盖文件数、发现统计 P0/P1/P2/P3）
## 2. 发现总表（全量，按严重度→模块排序）
## 3. 分级详情（P0 → P3，逐条按 7.1 模板展开）
## 4. 交叉验证记录（复核结论、冲突裁决、剔除的误报及理由）
## 5. 与历史发现对照（11 号文档 12 项：已修复/回归/新增）
## 6. 负面确认清单（检查过且确认无问题的关键点——证明无死角）
```

---

## 8. 已知基线（不作为新发现）

| 事项 | 状态 | 依据 |
|------|------|------|
| buildOrgTree/buildMenuTree 递归树组装（随机丢孙子节点） | 已修复（commit 9f97483） | 单测 + `-count=30` 验证 |
| 管理端菜单树误滤按钮节点 | 已修复（同上） | 验收断言 #6c |
| `/user/menus`、`/user/permissions` 响应契约文档 | 已同步（commit 3b2655a） | phase1/07-menu.md |
| authz_service 的 Resource Registry | **Phase 2a 计划内预留** | phase2/02-authz-resource.md、11 号评审文档 §G-1 |
| RoleService.GetTree 角色继承树 | **Phase 2b 计划内预留** | modules/role.md 已标注 |
| 审计日志断连丢失、HTTP 超时配置 | 已修复（commit de470bf） | 11 号文档 |

> 判定规则：计划内预留**不算 STUB 问题**，但若文档无对应计划标注，则记 D2 发现。

## 9. 鉴权不变量（D3 检查基准，源自 phase2/11 号评审文档）

- deny-by-default：`Allow ⟺ L1 通过 ∧ (L3 属主命中 ∨ L2 组织命中)`，任何一层无法判定（DB 错误）= 拒绝（fail-closed，503）
- L2/L3 资源级鉴权**不得缓存**；未注册资源 = 500
- 优先级防提权：低优先级操作者不得变更高优先级目标（比较 actor 与 target 的最高角色优先级）

---

## 10. 人工检查配套建议（供项目负责人并行执行）

1. **顺序**：建议与 agent 同序（认证 → 授权 → 用户 → 组织 → 角色菜单 → 基础设施），可先看各 agent 报告的「负面确认清单」做抽查式复核。
2. **重点人工场景**（agent 难以完全模拟的）：并发登录锁定窗口、RT 并发刷新竞争、Casbin 策略热加载、ltree move 大子树事务时长、wire 依赖图与实际构造一致性。
3. **工具建议**：`go vet ./...`、`go test ./... -count=1 -race`、`gosec`（如可用）作为 agent 检查之外的第二通道。

## 11. 产出物清单

| 产出物 | 位置 | 状态 |
|--------|------|------|
| 本计划 | docs/review/00-phase1-systematic-review-plan.md | 本文档 |
| 各 agent 分模块发现（过程产物，在主会话汇总） | — | R1/R2 返回 |
| 汇总发现报告 | docs/review/01-phase1-systematic-review-findings.md | R4 产出 |
