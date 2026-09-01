# Phase 2 全面验证报告

> 验证日期：2026-08-28
> 验证范围：Phase 2 全量代码 + 文档 + 测试 + 工程化配置
> 验证方法：5 路并行验证（代码实现 / 授权架构 / 文档一致性 / 测试覆盖 / 工程化配置）+ 直接代码校验
> 前置状态：Phase 2 已完成 2a/2b-core/2b-org/2c（Step 0-10），工作树干净

---

## 验证结论总览

| 维度 | 评级 | 关键发现 |
|------|------|----------|
| 代码实现 | ⚠️ 有待修 | 2 项 Critical（TOCTOU），3 项 High（审计断档/事务/Assign 缺条件更新） |
| 授权架构 | ✅ 良好 | 三层鉴权逻辑正确；1 项 High（文档伪代码误导） |
| 文档一致性 | ⚠️ 有待修 | 4 项 Medium（C1/C2/C4 + 04 §0 编号矛盾）+ 多项 Low |
| 测试覆盖 | ⚠️ 有待修 | 3 项 High（delete/relation/HR 隔离测试缺失） |
| 工程化配置 | ⚠️ 有待修 | Swagger 过期、DSN 硬编码、tickets 表无软删 |

**整体：Build 通过、`go vet` 无问题、gofmt 合规、单元测试全绿。** 核心架构设计正确，主要风险在并发安全（TOCTOU）、审计完整性、文档同步和测试覆盖缺口。

---

## 1. 代码实现验证

### 1.1 构建与测试状态

| 检查项 | 结果 |
|--------|------|
| `go build ./...` | ✅ 通过 |
| `go vet ./...` | ✅ 无问题 |
| `gofmt -l .` | ✅ 合规 |
| `go test -race -count=1 -short ./internal/...` | ✅ 全绿（20 包） |
| 迁移文件完整性 | ✅ 000001-000013/000015/000016 均有 up+down |

### 1.2 Critical 问题

| # | 问题 | 文件 | 风险 | 修复建议 |
|---|------|------|------|----------|
| **CC1** | **Close TOCTOU**：`Close` 读取工单（L275）→ 校验状态（L279）→ 事务内 `UpdateStatusTx`（L303），但 `UpdateStatusTx` 用 `WHERE id = $1` 无旧状态条件。两个并发 Close 均通过预检，第二个写入 `closed→closed` 事件 | `ticket_repo.go:144` / `service.go:275-326` | **Critical** | `UpdateStatusTx` 改为 `WHERE id = $1 AND status = $3`，0 行返回 `ErrConcurrentModification` |
| **CC2** | **Assign TOCTOU**：`Assign` 有 `closed` 预检（L340），但 `UpdateAssignedToTx` 用 `WHERE id = $1` 无状态条件。并发 Close+Assign 时，Assign 仍能修改已关闭工单的 assignee | `ticket_repo.go:161` / `service.go:330-396` | **Critical** | `UpdateAssignedToTx` 改为 `WHERE id = $1 AND status <> 'closed'`，0 行返回 `ErrTicketAlreadyClosed` |
| **CC3** | **DeleteOrgDelegated 非原子**：`CountNonOwnerMembers`（读）→ `ClearOwnerMemberships`（写，用 `r.db`）→ `Delete`（写，独立事务）三步非同事务。`ClearOwnerMemberships` 成功但 `Delete` 失败时，org 丢失 owner 但仍活跃 | `org_service.go:414-451` | **Critical** | 三步包裹在单个 `RunInTx` 中 |

### 1.3 High 问题

| # | 问题 | 文件 | 风险 | 修复建议 |
|---|------|------|------|----------|
| **HC1** | **CreateComment/CreateNote 无事件审计**：其他变更操作（Create/Update/Close/Assign）均写 `ticket_events`，但 comment 和 note 不写，留下审计断档 | `service.go:410-441` | **High** | 事务内补写 `CreateEventTx`（action=`"commented"`/`"noted"`） |
| **HC2** | **Delete 无事件审计 + 物理删除**：`Delete` 物理删除工单，`ticket_events` 因 CASCADE 全部丢失，无 "deleted" 事件记录谁删除了工单 | `service.go:400-405` / `ticket_repo.go:171` | **High** | 删除前在事务内写 "deleted" 事件；或改为软删除（加 `deleted_at`） |
| **HC3** | **CreateTx 不映射 FK 违规**：`type_code` 或 `org_id` 引用不存在时，FK 约束触发 23503 → raw 500，而非业务错误 400/404 | `ticket_repo.go:37-53` | **High** | 补 `MapForeignKeyViolation`，映射为 `ErrTicketTypeNotFound`/`ErrOrgNotFound` |

### 1.4 Medium 问题

| # | 问题 | 文件 | 风险 |
|---|------|------|------|
| MC1 | `CreateRelation` 未映射 FK 违规（并发删除 target 时 23503→500） | `ticket_repo.go` CreateRelation | Medium |
| MC2 | `OrgRepo.Move` 级联更新 `tickets.org_path` 但未更新 `ticket_templates.org_path` | `org_repo.go:419-430` | Medium |
| MC3 | `IsAncestorOwner` 接受 `ticketOrgPath` 参数但 SQL 未使用（dead code） | `org_delegation.go:77-91` | Medium |
| MC4 | `AddMember` 注释写"→ 500"但代码已修复为 409（注释过期） | `org_repo.go:97-98` | Low |

---

## 2. 授权架构验证

### 2.1 核心架构：正确 ✅

| 检查项 | 结论 |
|--------|------|
| L1 Casbin 门控先于 L2/L3 | ✅ 正确——中间件 abort 在前 |
| 不可见资源返回 404（非 403） | ✅ 正确——`Authorize` 返回 `(false, nil)` → 90001 → 404 |
| 可见无权返回 403 | ✅ 正确——`errDenied` → 70001 → 403 |
| admin bypass L2 列表过滤 | ✅ 正确——`List` 中 `HasRole(admin)` → 空 Filter |
| `close` 权限 = 处理人 OR 创建人 | ✅ 正确——`resource.go:182-186` |
| `update` 权限 = 仅创建人（RK-11） | ✅ 正确——`resource.go:176-181` |
| `delete` 权限 = admin bypass + org 委托 | ✅ 正确——`resource.go:193-195` |
| 关联创建双端 update 鉴权 | ✅ 正确——`service.go:464-474` |
| Registry 线程安全 | ✅ `sync.RWMutex` + 防御性拷贝 |

### 2.2 发现的问题

| # | 问题 | 文件 | 风险 | 修复建议 |
|---|------|------|------|----------|
| **AC1** | **04-org-delegation §4.3 canOperate 伪代码误导**：伪代码 `if t.CreatedBy == userID || t.AssignedTo == userID { return true }` 对所有 action 短路，意味着 assignee 可 update/delete。但实际代码正确实现了 per-action 矩阵（update 仅创建人） | `04-org-delegation.md:256-267` | **High** | 更新伪代码为 per-action 矩阵，或标注"简化版，实际见 resource.go" |
| AC2 | 02-authz §2.2a 表 `update=创建人或处理人` 未标注被 2b 收窄 | `02-authz-resource.md:172` | Medium | 加注 `(2b 收窄为仅创建人，见下方 2b 表)` |
| AC3 | 11-authz §Q3 写 DB 错误→503，代码实际返回 500 | `11-authz-architecture-review.md:93` vs `service.go:80` | Medium | 更新文档为 500（或标注实现选择了 500） |

---

## 3. 文档一致性验证

### 3.1 前序审查问题状态

| 前序编号 | 问题 | 当前状态 |
|----------|------|----------|
| C1 | `09-ticket §0 L3` "Step 9" 应为 "Step 8-10" | ⚪ **2026-08-31 复核：驳回维持**（"Step 9" 系 2c Authorize = Step 8–10 中的第 9 步，原文正确；三轮裁定不修） |
| C2 | `02-authz §2.5` NewResource 签名不匹配 | ✅ **2026-08-31 复核已修**：文档现为三参调用（与代码一致） |
| C3 | `docs/ops/` 缺 deployment.md | ⏳ 随 08-ops 编写落地（检查单 B10，Phase 3 W3 前） |
| C4 | `02-authz §2.2` ScopeResolver 接口不匹配 | ✅ **2026-08-31 已修**：§2.2 接口段按现行实现（`ReadAnchorPaths`/`ResolveScope` + `ResolvedScope` 三轴）重写；委托轴分支见 BK-13（IW1） |
| N4 | README §2.4 000013 缺 `file_objects` | ✅ **已修复**（000014 正确列出两张表） |
| N5 | VISION §4 migrations 现状 | ✅ **已修复** |

### 3.2 新发现的不一致

| # | 问题 | 文件 | 风险 | 修复建议 |
|---|------|------|------|----------|
| **DC1** | **04-org-delegation §0 表 Step 编号矛盾**：表用 9/10/11，但同文档 L32 用 8/9/10，00 §8 用 8/9/10 | `04-org-delegation.md:12-19` | **Medium** | 表改 8/9/10 |
| DC2 | `09-ticket.md L275` update 写"创建人或处理人"，与 L349/L395 的"仅创建人"(RK-11) 矛盾 | `09-ticket.md:275` | Medium | 改为"仅创建人（2b，RK-11）" |
| DC3 | `09-ticket.md L477` T5 行写"Step 8"但 2b scope 是 Step 4/5 | `09-ticket.md:477` | Low | 改为 "Step 4/5" |
| DC4 | `09-ticket.md L483` §8 2b header 写"Step 8 验收"但 Step 8 是 2c | `09-ticket.md:483` | Low | 改为 "Step 4/5 验收" |
| DC5 | `04-org-delegation.md L64/L280` 仍用 `0000xx_org_delegation.up.sql` 占位符 | `04-org-delegation.md` | Low | 改为 `000013_org_delegation.up.sql` |
| DC6 | `03-org-enhance.md L211` 仍用 `0000xx_hr_source.up.sql` 占位符 | `03-org-enhance.md` | Low | 改为 `000012_org_enhance.up.sql` |
| DC7 | `09-ticket.md L39/L512` 仍用 `0000xx_ticket.up.sql` 占位符 | `09-ticket.md` | Low | 改为 `000010_ticket.up.sql` |
| DC8 | 02-authz §2.3 TicketResource 结构体文档写 `{repo, scope}`，实际为 `{repo, resolver, delegation}` | `02-authz-resource.md:124-127` | Medium | 同步为实际结构体 |

### 3.3 errcode 一致性：✅ 通过

所有 Phase 2 文档引用的错误码（90001-90004、50008-50010、70001、70004）均在 `errcode.go` 中存在。Step 7/6 的前向码（20012/20013/91001-91004）按 P2-D4 规则随 Step 实现添加，符合预期。

---

## 4. 测试覆盖验证

### 4.1 已覆盖：良好 ✅

| 测试维度 | 状态 |
|----------|------|
| R1-R8（2a 授权） | ✅ 全部 PASS |
| R9-R12（2b 授权） | ✅ R9/R10/R12 PASS，R11 future |
| D1-D12（2c 委托） | ✅ D1-D9/D11 PASS，D10 静态检查，D12 future |
| T1-T7（工单 MVP） | ✅ 全部 PASS |
| 404/403/400/409 错误路径 | ✅ 覆盖 |
| 验收脚本链式回归 | ✅ 2c→2b→2a→Phase 1 |
| BK-1/2/3/6/8/10 | ✅ 已关闭 |
| testcontainers + 全迁移 | ✅ 14 个迁移加载 |

### 4.2 测试缺口

| # | 缺口 | 风险 | 建议补充 |
|---|------|------|----------|
| **TC1** | **无工单删除成功路径 Go 测试**——仅测 403 拒绝路径，未测 admin/owner 成功删除 | **High** | `TestTicket_Delete_AdminSucceeds` |
| **TC2** | **无工单关联 Go 测试（BK-5）**——零 Go 测试覆盖 CreateRelation/ListRelations | **High** | 新建 `relation_integration_test.go`：正向/反向判重/跨用户拒绝/自引用 |
| **TC3** | **D10 HR 隔离仅静态列检查**——无契约测试验证 HR Sync 不覆盖委托字段 | **High** | 定义契约测试 stub（2b-ext HR Sync 落地时实现） |
| TC4 | 无 500 错误路径测试（DB 错误 → 500 映射） | Medium | `TestTicket_AuthorizeDBError_Returns500` |
| TC5 | 无工单列表过滤测试（priority/status/type_code/pagination 组合） | Medium | `TestTicket_ListFilters` |
| TC6 | 无并发工单操作测试（并发 Close 同一工单） | Medium | `TestTicket_ConcurrentCloseRace` |
| TC7 | BK-7 handler 输入校验未测（invalid priority / page=0） | Medium | HTTP 级测试 |
| TC8 | BK-4/9 未关闭（action 常量化 / setupTicket2a 死代码） | Low | 顺手清理 |

---

## 5. 工程化配置验证

### 5.1 已验证：良好 ✅

| 检查项 | 状态 |
|--------|------|
| Makefile 目标齐全 | ✅ build/run/dev/wire/migrate/docker/swag/lint/test |
| Dockerfile 多阶段 + 非 root | ✅ CGO=0, alpine 3.20, appuser |
| docker-compose dev/prod 隔离 | ✅ 项目名/容器名/卷名分离 |
| PG/Redis 健康检查 | ✅ pg_isready + redis ping |
| JWT release 模式强制 | ✅ validate.go 拒绝默认 secret |
| Wire DI 完整 | ✅ 所有 provider 注册 |
| 迁移幂等 + down 可逆 | ✅ 2b/2c 新迁移均用 IF NOT EXISTS |
| 验收脚本三段链式 | ✅ phase2a/b/c 均可运行 |

### 5.2 发现的问题

| # | 问题 | 文件 | 风险 | 修复建议 |
|---|------|------|------|----------|
| **EC1** | **Swagger 过期**——2c 端点 `orgs/owners`、`orgs/members/role` 未生成到 swagger.yaml/docs.go | `docs/swagger.yaml` | **High** | 运行 `make swag` |
| **EC2** | **Makefile DSN 硬编码**——`migrate-up/down` 写死明文密码 | `Makefile:26,29` | **High** | `MIGRATE_DSN ?= ...` 变量化 |
| EC3 | `tickets` 表无 `deleted_at`——物理删除丢失审计记录（与 Phase 1 软删除模式不一致） | `migrations/000010_ticket.up.sql` | Medium | 产品决策：加 `deleted_at` 或文档标注"删除不可逆" |
| EC4 | `000014` 缺失无注释说明 | `migrations/` | Medium | README §2.4 000014 行加"（未执行，延后）" |
| EC5 | CORS `AllowAllOrigins=true` 仍未收紧 | `cors.go:12` | Medium | 已登记 D2-23（引入 cookie 前收紧），当前 Bearer 认证下可接受 |
| EC6 | `test` 目标不含集成测试 | `Makefile:56` | Low | 文档说明或改为 `test: test-unit test-integration` |
| EC7 | docker-compose v1 命令（`docker-compose` 应为 `docker compose`） | `Makefile` | Low | 迁移到 v2 语法 |
| EC8 | `000001_init.up.sql` `CREATE TABLE` 无 `IF NOT EXISTS` | `migrations/000001` | Low | golang-migrate 版本号机制可接受 |

---

## 6. 综合风险矩阵

| # | 风险 | 等级 | 类型 | 修复时机 |
|---|------|------|------|----------|
| CC1 | Close TOCTOU（`UpdateStatusTx` 无旧状态条件） | **Critical** | 并发安全 | 立即 |
| CC2 | Assign TOCTOU（`UpdateAssignedToTx` 无 `status<>'closed'`） | **Critical** | 并发安全 | 立即 |
| CC3 | DeleteOrgDelegated 三步非原子 | **Critical** | 事务安全 | 立即 |
| HC1 | Comment/Note 无事件审计 | **High** | 审计完整性 | 2b 收口 |
| HC2 | Delete 无事件 + 物理删除丢审计 | **High** | 审计完整性 | 2b 收口 |
| HC3 | CreateTx 不映射 FK 违规 → 500 | **High** | 错误处理 | 2b 收口 |
| AC1 | 04-org-delegation canOperate 伪代码误导 | **High** | 文档安全 | 立即 |
| EC1 | Swagger 过期缺 2c 端点 | **High** | 工程化 | 立即 |
| EC2 | Makefile DSN 硬编码 | **High** | 工程化 | 立即 |
| TC1 | 无 delete 成功路径 Go 测试 | **High** | 测试 | 2b 收口 |
| TC2 | 无 relation Go 测试（BK-5） | **High** | 测试 | 2b 收口 |
| DC1 | 04-org-delegation §0 Step 编号矛盾 | Medium | 文档 | P0 |
| C1 | 09-ticket §0 "Step 9" 未修 | Medium | 文档 | P0 |
| C2 | 02-authz NewResource 签名再次过时 | Medium | 文档 | P0 |
| C4 | 02-authz ScopeResolver 接口完全不匹配 | Medium | 文档 | P0 |
| DC8 | 02-authz TicketResource 结构体过时 | Medium | 文档 | P0 |
| DC2 | 09-ticket L275 update 权限描述矛盾 | Medium | 文档 | P0 |

---

## 7. 修复路线图

### P0 — 立即修复（安全 + 高频引用文档）

1. **CC1**：`UpdateStatusTx` 加 `AND status = $3`，0 行返回 `ErrConcurrentModification`
2. **CC2**：`UpdateAssignedToTx` 加 `AND status <> 'closed'`，0 行返回 `ErrTicketAlreadyClosed`
3. **CC3**：`DeleteOrgDelegated` 三步包裹 `RunInTx`
4. **AC1**：更新 `04-org-delegation §4.3` canOperate 伪代码为 per-action 版本
5. **EC1**：运行 `make swag` 重新生成 Swagger
6. **EC2**：Makefile DSN 外置为 `MIGRATE_DSN ?= ...`
7. **C1**：`09-ticket §0 L3` 改 "Step 8-10"
8. **C2+DC8**：`02-authz §2.3/§2.5` 同步为实际结构体和签名（3 参数）
9. **C4**：`02-authz §2.2` 替换为实际 ScopeResolver 接口（`ReadAnchorPaths`/`ResolveScope`）
10. **DC1**：`04-org-delegation §0` 表 Step 改 8/9/10
11. **DC2**：`09-ticket L275` 改"仅创建人（2b，RK-11）"

### P1 — 2b 收口前完成

12. **HC1**：Comment/Note 补写 `ticket_events`
13. **HC2**：Delete 前写 "deleted" 事件（或改软删除）
14. **HC3**：`CreateTx` 补 `MapForeignKeyViolation`
15. **TC1**：补 `TestTicket_Delete_AdminSucceeds`
16. **TC2**：补 `relation_integration_test.go`（BK-5）
17. **TC4**：补 500 错误路径测试
18. **TC6**：补并发 Close 种族测试
19. MC1：`CreateRelation` 补 FK 违规映射
20. MC2：`OrgRepo.Move` 级联更新 `ticket_templates.org_path`

### P2 — 文档打磨（可并行）

21. DC3-DC7：替换所有 `0000xx` 占位符为实际文件名
22. AC2：02-authz 2a 表 update 行加 2b 收窄标注
23. AC3：11-authz Q3 改 500
24. EC4：README §2.4 000014 加"未执行"标注
25. EC6-EC8：工程化小项
